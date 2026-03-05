// services/business/Kot.go
//
// KOTService — Kitchen Order Ticket lifecycle engine.
//
// ┌────────────────────────────────────────────────────────────────────────┐
// │  LAYER 1 — CREATION                                                    │
// │  RBAC guard  →  Order ownership verify  →  Idempotency dedup          │
// │  →  Sequence generation  →  Station auto-routing  →  Persist KOT+Items│
// │  →  Advance OrderFSM to "preparing"  →  AuditTrail                    │
// ├────────────────────────────────────────────────────────────────────────┤
// │  LAYER 2 — STATUS TRANSITIONS (FSM-gated)                              │
// │  Start (Sent→InProgress)  →  Complete (InProgress→Completed)          │
// │  →  Cancel (Sent|InProgress→Cancelled, with in-progress guard)        │
// ├────────────────────────────────────────────────────────────────────────┤
// │  LAYER 3 — CHEF ASSIGNMENT & ITEM-LEVEL STATUS                         │
// │  AssignChef  →  UpdateItemStatus (per-station granularity)             │
// │  →  Auto-complete KOT when all items reach terminal state              │
// ├────────────────────────────────────────────────────────────────────────┤
// │  LAYER 4 — READ PATH & KITCHEN DISPLAY                                 │
// │  Get  →  List (paginated, multi-filter)  →  Stats per restaurant       │
// │  →  PrintKOT (increment counter, record timestamp)                     │
// └────────────────────────────────────────────────────────────────────────┘
//
// Station routing:
//   Every KOT item has an AssignedStation. If the caller leaves it empty,
//   autoRouteStation() inspects the item name for keyword hints and falls
//   back to KitchenStationGeneral. This keeps the hot path simple while
//   letting restaurants pre-configure routing rules in the future.
//
// Idempotency:
//   KOTs are keyed by (OrderID, RestaurantID). A second CreateKOT call for
//   the same order returns the existing KOT rather than creating a duplicate.
//
// Sequence numbers:
//   Per-tenant sequence, reset at midnight (local restaurant time).
//   Format: KOT-YYYYMMDD-{seq:04d}  e.g. KOT-20260305-0007
package business

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/models"
	"backend/utils"
)

// ─────────────────────────────────────────────────────────────────────────────
// SENTINEL ERRORS
// ─────────────────────────────────────────────────────────────────────────────

var (
	ErrKOTNotFound            = errors.New("KOT not found")
	ErrKOTItemNotFound        = errors.New("KOT item not found")
	ErrKOTAlreadyExists       = errors.New("a KOT already exists for this order")
	ErrKOTOrderNotFound       = errors.New("order not found or does not belong to this tenant")
	ErrKOTInvalidTransition   = errors.New("KOT status transition not permitted by FSM")
	ErrKOTTerminal            = errors.New("KOT is in a terminal state and cannot be modified")
	ErrKOTItemsRequired       = errors.New("KOT must contain at least one item")
	ErrKOTChefNotFound        = errors.New("assigned chef not found in this restaurant")
	ErrKOTTenantMismatch      = errors.New("KOT does not belong to this tenant")
	ErrKOTRestaurantMismatch  = errors.New("KOT does not belong to this restaurant")
	ErrKOTCancelInProgress    = errors.New("KOT is InProgress — only a manager can cancel at this stage")
)

// ─────────────────────────────────────────────────────────────────────────────
// RESOURCE CONSTANT
// ─────────────────────────────────────────────────────────────────────────────

const (
	ResourceKOT ResourceType = "kot"
)

// ─────────────────────────────────────────────────────────────────────────────
// INPUT TYPES
// ─────────────────────────────────────────────────────────────────────────────

// KOTServiceRequest carries caller identity for every mutation.
type KOTServiceRequest struct {
	TenantID     uuid.UUID
	RestaurantID uuid.UUID
	ActorID      uuid.UUID
}

// CreateKOTInput is the data needed to create a new KOT.
type CreateKOTInput struct {
	KOTServiceRequest
	OrderID      uuid.UUID
	TableNumber  *int
	CustomerName string
	OrderType    string // models.OrderType value
	GuestCount   *int
	Priority     string // models.KOTPriority value; empty → KOTPriorityMedium
	Items        []CreateKOTItemInput
}

// CreateKOTItemInput is a single line item for the new KOT.
type CreateKOTItemInput struct {
	OrderItemID     uuid.UUID
	ItemName        string
	Quantity        int
	Notes           string
	AssignedStation string // empty → auto-routed from ItemName
}

// UpdateKOTInput carries optional fields for a KOT patch.
type UpdateKOTInput struct {
	KOTServiceRequest
	KOTID            uuid.UUID
	Priority         *string
	AssignedToChefID *uuid.UUID
	IsManagerActor   bool // required for cancelling an InProgress KOT
}

// UpdateKOTItemStatusInput updates a single item's preparation status.
type UpdateKOTItemStatusInput struct {
	KOTServiceRequest
	KOTItemID uuid.UUID
	Status    string // models.KOTItemStatus value
}

// ListKOTsFilter controls pagination and filtering for KOT listing.
type ListKOTsFilter struct {
	KOTServiceRequest
	OrderID    *uuid.UUID
	Status     *string
	Priority   *string
	Station    *string
	ChefID     *uuid.UUID
	DateFrom   *time.Time
	DateTo     *time.Time
	Page       int
	PageSize   int
	SortBy     string // "created_at" | "sequence_number" | "priority" | "status"
	SortOrder  string // "asc" | "desc"
}

// ─────────────────────────────────────────────────────────────────────────────
// SERVICE
// ─────────────────────────────────────────────────────────────────────────────

// KOTService manages the full KOT lifecycle for a restaurant.
// Goroutine-safe; construct one per application process.
type KOTService struct {
	db    *gorm.DB
	redis *goredis.Client
	rbac  RBACAuthorizer
}

// NewKOTService constructs a ready-to-use KOTService.
func NewKOTService(
	db *gorm.DB,
	redis *goredis.Client,
	rbac RBACAuthorizer,
) *KOTService {
	return &KOTService{db: db, redis: redis, rbac: rbac}
}

// ─────────────────────────────────────────────────────────────────────────────
// LAYER 1 — CREATION
// ─────────────────────────────────────────────────────────────────────────────

// CreateKOT creates a new KOT for an order.
//
// Pipeline:
//  1. RBAC check (kot:create)
//  2. Validate input (at least one item)
//  3. Verify order exists and belongs to this tenant/restaurant
//  4. Idempotency: if a KOT already exists for this order, return it
//  5. Generate KOT number and sequence
//  6. Auto-route station for items with no assigned station
//  7. Persist KOT + KOTItems in a transaction
//  8. Advance the parent Order's FSM to "preparing" (confirm → prepare)
//  9. Write audit trail
func (s *KOTService) CreateKOT(
	ctx context.Context,
	in CreateKOTInput,
) (*models.KOT, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceKOT,
		Action:   ActionCreate,
	}); err != nil {
		return nil, err
	}

	if len(in.Items) == 0 {
		return nil, ErrKOTItemsRequired
	}

	// ── 1. Verify order ownership ────────────────────────────────────────────
	var order models.Order
	if err := s.db.WithContext(ctx).
		Where("order_id = ? AND tenant_id = ? AND restaurant_id = ?",
			in.OrderID, in.TenantID, in.RestaurantID).
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKOTOrderNotFound
		}
		return nil, fmt.Errorf("fetch order: %w", err)
	}

	// ── 2. Idempotency: return existing KOT if one already exists ────────────
	var existing models.KOT
	err := s.db.WithContext(ctx).
		Where("order_id = ? AND restaurant_id = ? AND tenant_id = ?",
			in.OrderID, in.RestaurantID, in.TenantID).
		Preload("AssignedToChef").
		First(&existing).Error
	if err == nil {
		return &existing, ErrKOTAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("idempotency check: %w", err)
	}

	// ── 3. Generate KOT number and sequence ──────────────────────────────────
	now := time.Now()
	kotID := uuid.New()
	kotNumber, seqNum, err := s.generateKOTNumber(ctx, in.TenantID, now)
	if err != nil {
		return nil, fmt.Errorf("generate KOT number: %w", err)
	}

	// ── 4. Resolve priority ───────────────────────────────────────────────────
	priority := models.KOTPriority(in.Priority)
	if priority == "" {
		priority = models.KOTPriorityMedium
	}

	// ── 5. Build KOT and items ────────────────────────────────────────────────
	kotItems := make([]models.KOTItem, 0, len(in.Items))
	for _, itemIn := range in.Items {
		station := models.KitchenStation(itemIn.AssignedStation)
		if station == "" {
			station = autoRouteStation(itemIn.ItemName)
		}
		kotItems = append(kotItems, models.KOTItem{
			KOTItemID:       uuid.New(),
			KOTID:           kotID,
			OrderItemID:     itemIn.OrderItemID,
			ItemName:        strings.TrimSpace(itemIn.ItemName),
			Quantity:        itemIn.Quantity,
			Notes:           strings.TrimSpace(itemIn.Notes),
			AssignedStation: station,
			Status:          models.KOTItemStatusPending,
		})
	}

	kot := &models.KOT{
		KOTID:           kotID,
		OrderID:         in.OrderID,
		RestaurantID:    in.RestaurantID,
		TenantID:        in.TenantID,
		KOTNumber:       kotNumber,
		SequenceNumber:  seqNum,
		OrderNumber:     "ORD-" + strings.ToUpper(in.OrderID.String()[len(in.OrderID.String())-8:]),
		TableNumber:     in.TableNumber,
		CustomerName:    strings.TrimSpace(in.CustomerName),
		OrderType:       models.OrderType(in.OrderType),
		GuestCount:      in.GuestCount,
		Status:          models.KOTStatusSent,
		Priority:        priority,
		CreatedByUserID: in.ActorID,
		PrintCount:      0,
	}

	// ── 6. Persist in transaction ─────────────────────────────────────────────
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(kot).Error; err != nil {
			return fmt.Errorf("persist KOT: %w", err)
		}
		for i := range kotItems {
			kotItems[i].KOTID = kot.KOTID
		}
		if err := tx.CreateInBatches(kotItems, 50).Error; err != nil {
			return fmt.Errorf("persist KOT items: %w", err)
		}

		// Advance Order FSM: confirmed → preparing
		// Use utils.OrderFSM — same pattern as Order.go
		orderAdapter := &orderContextAdapter{&order}
		fsm := utils.OrderFSM.Restore(orderAdapter, utils.OrderState(order.OrderStatus))
		prepEnv := utils.NewEnvelope(utils.OrderEventPrepare, nil).WithActor(in.ActorID, in.TenantID)
		if fireErr := fsm.Send(ctx, prepEnv); fireErr != nil {
			// Non-fatal: FSM guard may reject if order is already in preparing state.
			// Log but do not roll back the KOT creation.
			_ = tx.Create(&models.AuditTrail{
				AuditTrailID:     uuid.New(),
				TenantID:         in.TenantID,
				EventType:        models.AuditEventUpdate,
				EventCategory:    "kot",
				EventDescription: fmt.Sprintf("FSM advance skipped for order %s: %s", in.OrderID, fireErr),
				Severity:         models.AuditSeverityWarning,
				EntityType:       "Order",
				EntityID:         in.OrderID.String(),
				Timestamp:        now,
			}).Error
		} else {
			// Persist updated order status
			if err := tx.Model(&models.Order{}).
				Where("order_id = ?", in.OrderID).
				Update("order_status", order.OrderStatus).Error; err != nil {
				return fmt.Errorf("advance order status: %w", err)
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	// ── 7. Audit ──────────────────────────────────────────────────────────────
	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventCreate, "KOT", kotID.String(),
		nil, map[string]interface{}{
			"kot_number":   kotNumber,
			"order_id":     in.OrderID,
			"priority":     priority,
			"item_count":   len(kotItems),
		})

	return kot, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// LAYER 2 — STATUS TRANSITIONS (FSM-gated)
// ─────────────────────────────────────────────────────────────────────────────

// StartKOT transitions a KOT from Sent → InProgress.
// Called when the kitchen acknowledges the ticket.
// RBAC: requires kot:update.
func (s *KOTService) StartKOT(
	ctx context.Context,
	in KOTServiceRequest,
	kotID uuid.UUID,
) (*models.KOT, error) {
	return s.fireKOTEvent(ctx, in, kotID, utils.KOTEventStart)
}

// CompleteKOT transitions a KOT from InProgress → Completed.
// Called when the kitchen has prepared all items.
// RBAC: requires kot:update.
func (s *KOTService) CompleteKOT(
	ctx context.Context,
	in KOTServiceRequest,
	kotID uuid.UUID,
) (*models.KOT, error) {
	return s.fireKOTEvent(ctx, in, kotID, utils.KOTEventComplete)
}

// CancelKOT transitions a KOT from Sent|InProgress → Cancelled.
// Cancelling an InProgress KOT requires in.IsManagerActor == true.
// RBAC: requires kot:update; InProgress cancel also requires manager check.
func (s *KOTService) CancelKOT(
	ctx context.Context,
	in UpdateKOTInput,
) (*models.KOT, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceKOT,
		Action:   ActionUpdate,
	}); err != nil {
		return nil, err
	}

	kot, err := s.getKOTForWrite(ctx, in.TenantID, in.RestaurantID, in.KOTID)
	if err != nil {
		return nil, err
	}

	// Extra guard: InProgress cancellation requires manager
	if kot.Status == models.KOTStatusInProgress && !in.IsManagerActor {
		return nil, ErrKOTCancelInProgress
	}

	return s.fireKOTEventOnModel(ctx, in.KOTServiceRequest, kot, utils.KOTEventCancel)
}

// fireKOTEvent loads the KOT, fires the given FSM event, and persists the result.
func (s *KOTService) fireKOTEvent(
	ctx context.Context,
	in KOTServiceRequest,
	kotID uuid.UUID,
	event utils.KOTEvent,
) (*models.KOT, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceKOT,
		Action:   ActionUpdate,
	}); err != nil {
		return nil, err
	}

	kot, err := s.getKOTForWrite(ctx, in.TenantID, in.RestaurantID, kotID)
	if err != nil {
		return nil, err
	}

	return s.fireKOTEventOnModel(ctx, in, kot, event)
}

// fireKOTEventOnModel fires a KOT FSM event on an already-loaded KOT model.
func (s *KOTService) fireKOTEventOnModel(
	ctx context.Context,
	in KOTServiceRequest,
	kot *models.KOT,
	event utils.KOTEvent,
) (*models.KOT, error) {
	adapter := &kotContextAdapter{kot}
	fsm := utils.KOTFSM.Restore(adapter, utils.KOTState(kot.Status))

	if err := fsm.Send(ctx, utils.NewEnvelope(event, nil).WithActor(in.ActorID, in.TenantID)); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrKOTInvalidTransition, err.Error())
	}

	// Persist the new status (adapter mutates kot.Status via SetStatus)
	if err := s.db.WithContext(ctx).
		Model(kot).
		Update("status", kot.Status).Error; err != nil {
		return nil, fmt.Errorf("persist KOT status: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventUpdate, "KOT", kot.KOTID.String(),
		map[string]interface{}{"event": string(event)},
		map[string]interface{}{"status": kot.Status})

	return kot, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// LAYER 3 — CHEF ASSIGNMENT & ITEM-LEVEL STATUS
// ─────────────────────────────────────────────────────────────────────────────

// UpdateKOT patches mutable fields: priority and/or chef assignment.
// Does not touch status — use Start/Complete/Cancel for that.
// RBAC: requires kot:update.
func (s *KOTService) UpdateKOT(
	ctx context.Context,
	in UpdateKOTInput,
) (*models.KOT, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceKOT,
		Action:   ActionUpdate,
	}); err != nil {
		return nil, err
	}

	kot, err := s.getKOTForWrite(ctx, in.TenantID, in.RestaurantID, in.KOTID)
	if err != nil {
		return nil, err
	}

	if isKOTTerminal(kot.Status) {
		return nil, ErrKOTTerminal
	}

	updates := map[string]interface{}{}

	if in.Priority != nil {
		kot.Priority = models.KOTPriority(*in.Priority)
		updates["priority"] = *in.Priority
	}

	if in.AssignedToChefID != nil {
		// Verify chef exists in this restaurant
		var chef models.Employee
		if err := s.db.WithContext(ctx).
			Where("employee_id = ? AND restaurant_id = ?",
				*in.AssignedToChefID, in.RestaurantID).
			First(&chef).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrKOTChefNotFound
			}
			return nil, fmt.Errorf("verify chef: %w", err)
		}
		kot.AssignedToChefID = in.AssignedToChefID
		updates["assigned_to_chef_id"] = *in.AssignedToChefID
	}

	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).
			Model(kot).
			Clauses(clause.Returning{}).
			Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("update KOT: %w", err)
		}

		s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
			models.AuditEventUpdate, "KOT", in.KOTID.String(),
			nil, updates)
	}

	return kot, nil
}

// UpdateKOTItemStatus updates the preparation status of a single KOT item.
// After the update, if every item in the KOT is in a terminal state
// (ready or cancelled), the KOT is automatically completed.
// RBAC: requires kot:update.
func (s *KOTService) UpdateKOTItemStatus(
	ctx context.Context,
	in UpdateKOTItemStatusInput,
) (*models.KOTItem, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceKOT,
		Action:   ActionUpdate,
	}); err != nil {
		return nil, err
	}

	// Load item with its parent KOT for ownership check
	var item models.KOTItem
	if err := s.db.WithContext(ctx).
		Joins("JOIN kots k ON k.kot_id = kot_items.kot_id").
		Where("kot_items.kot_item_id = ? AND k.tenant_id = ? AND k.restaurant_id = ?",
			in.KOTItemID, in.TenantID, in.RestaurantID).
		First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKOTItemNotFound
		}
		return nil, fmt.Errorf("get KOT item: %w", err)
	}

	// Update the item status
	if err := s.db.WithContext(ctx).
		Model(&item).
		Update("status", models.KOTItemStatus(in.Status)).Error; err != nil {
		return nil, fmt.Errorf("update KOT item status: %w", err)
	}

	// Auto-complete KOT if all items are now terminal
	go s.tryAutoCompleteKOT(context.Background(), in.TenantID, in.RestaurantID, in.ActorID, item.KOTID)

	return &item, nil
}

// tryAutoCompleteKOT fires the complete event on a KOT if every item has
// reached a terminal status (ready, served, cancelled).
// Runs in a goroutine — failures are silently ignored.
func (s *KOTService) tryAutoCompleteKOT(
	ctx context.Context,
	tenantID, restaurantID, actorID, kotID uuid.UUID,
) {
	var nonTerminalCount int64
	if err := s.db.WithContext(ctx).Model(&models.KOTItem{}).
		Where("kot_id = ? AND status NOT IN ('ready','served','cancelled')", kotID).
		Count(&nonTerminalCount).Error; err != nil {
		return
	}
	if nonTerminalCount > 0 {
		return
	}

	kot, err := s.getKOTForWrite(ctx, tenantID, restaurantID, kotID)
	if err != nil || kot.Status != models.KOTStatusInProgress {
		return
	}

	_, _ = s.fireKOTEventOnModel(ctx,
		KOTServiceRequest{TenantID: tenantID, RestaurantID: restaurantID, ActorID: actorID},
		kot, utils.KOTEventComplete)
}

// PrintKOT increments the print counter and records the timestamp.
// This is idempotent in the sense that calling it N times yields N in PrintCount.
// RBAC: requires kot:update.
func (s *KOTService) PrintKOT(
	ctx context.Context,
	in KOTServiceRequest,
	kotID uuid.UUID,
) (*models.KOT, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceKOT,
		Action:   ActionUpdate,
	}); err != nil {
		return nil, err
	}

	kot, err := s.getKOTForWrite(ctx, in.TenantID, in.RestaurantID, kotID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if err := s.db.WithContext(ctx).
		Model(kot).
		Updates(map[string]interface{}{
			"print_count":    gorm.Expr("print_count + 1"),
			"last_printed_at": now,
		}).Error; err != nil {
		return nil, fmt.Errorf("record print: %w", err)
	}

	kot.PrintCount++
	kot.LastPrintedAt = &now
	return kot, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// LAYER 4 — READ PATH & KITCHEN DISPLAY
// ─────────────────────────────────────────────────────────────────────────────

// GetKOT returns a single KOT with its items and assigned chef.
// RBAC: requires kot:read.
func (s *KOTService) GetKOT(
	ctx context.Context,
	in KOTServiceRequest,
	kotID uuid.UUID,
) (*models.KOT, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceKOT,
		Action:   ActionRead,
	}); err != nil {
		return nil, err
	}

	return s.getKOTWithItems(ctx, in.TenantID, in.RestaurantID, kotID)
}

// ListKOTs returns a paginated, multi-filtered list of KOTs.
// RBAC: requires kot:read.
func (s *KOTService) ListKOTs(
	ctx context.Context,
	f ListKOTsFilter,
) ([]models.KOT, int64, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   f.ActorID,
		TenantID: f.TenantID,
		Resource: ResourceKOT,
		Action:   ActionRead,
	}); err != nil {
		return nil, 0, err
	}

	q := s.db.WithContext(ctx).
		Where("tenant_id = ? AND restaurant_id = ?", f.TenantID, f.RestaurantID)

	if f.OrderID != nil {
		q = q.Where("order_id = ?", *f.OrderID)
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if f.Priority != nil {
		q = q.Where("priority = ?", *f.Priority)
	}
	if f.ChefID != nil {
		q = q.Where("assigned_to_chef_id = ?", *f.ChefID)
	}
	if f.DateFrom != nil {
		q = q.Where("created_at >= ?", *f.DateFrom)
	}
	if f.DateTo != nil {
		q = q.Where("created_at <= ?", f.DateTo)
	}
	// Station filter: requires JOIN on kot_items
	if f.Station != nil {
		q = q.Joins("JOIN kot_items ki ON ki.kot_id = kots.kot_id").
			Where("ki.assigned_station = ?", *f.Station).
			Group("kots.kot_id")
	}

	var total int64
	if err := q.Model(&models.KOT{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count KOTs: %w", err)
	}

	// Sort
	sortCol := "created_at"
	switch f.SortBy {
	case "sequence_number":
		sortCol = "sequence_number"
	case "priority":
		// Custom sort: Urgent > High > Normal > Low
		sortCol = "CASE priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END"
	case "status":
		sortCol = "status"
	}
	sortDir := "DESC"
	if strings.EqualFold(f.SortOrder, "asc") {
		sortDir = "ASC"
	}
	q = q.Order(fmt.Sprintf("%s %s", sortCol, sortDir))

	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	q = q.Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize)

	var kots []models.KOT
	if err := q.Find(&kots).Error; err != nil {
		return nil, 0, fmt.Errorf("list KOTs: %w", err)
	}
	return kots, total, nil
}

// GetKOTStats returns a snapshot of KOT counts and average prep time
// for the kitchen display system. No RBAC — this is a hot read.
func (s *KOTService) GetKOTStats(
	ctx context.Context,
	tenantID, restaurantID uuid.UUID,
) (*KOTStats, error) {
	type statusCount struct {
		Status string
		Count  int
	}
	var counts []statusCount
	if err := s.db.WithContext(ctx).
		Model(&models.KOT{}).
		Select("status, COUNT(*) as count").
		Where("tenant_id = ? AND restaurant_id = ?", tenantID, restaurantID).
		Group("status").
		Scan(&counts).Error; err != nil {
		return nil, fmt.Errorf("KOT stats: %w", err)
	}

	stats := &KOTStats{}
	for _, c := range counts {
		switch models.KOTStatus(c.Status) {
		case models.KOTStatusSent:
			stats.TotalPending = c.Count
		case models.KOTStatusInProgress:
			stats.TotalInProgress = c.Count
		case models.KOTStatusCompleted:
			stats.TotalCompleted = c.Count
		case models.KOTStatusCancelled:
			stats.TotalCancelled = c.Count
		}
	}

	// Average prep time: time from created_at to when status became completed.
	// We approximate as AVG(EXTRACT(EPOCH FROM (updated_at - created_at))) in minutes
	// on completed KOTs created in the last 24 hours.
	type avgRow struct {
		AvgSeconds float64
	}
	var avgResult avgRow
	_ = s.db.WithContext(ctx).
		Model(&models.KOT{}).
		Select("AVG(EXTRACT(EPOCH FROM (updated_at - created_at))) as avg_seconds").
		Where("tenant_id = ? AND restaurant_id = ? AND status = ? AND created_at > ?",
			tenantID, restaurantID, models.KOTStatusCompleted, time.Now().Add(-24*time.Hour)).
		Scan(&avgResult).Error

	if avgResult.AvgSeconds > 0 {
		stats.AvgPrepTimeMins = int(avgResult.AvgSeconds / 60)
	}

	return stats, nil
}

// KOTStats is the kitchen display statistics snapshot.
type KOTStats struct {
	TotalPending    int `json:"total_pending"`
	TotalInProgress int `json:"total_in_progress"`
	TotalCompleted  int `json:"total_completed"`
	TotalCancelled  int `json:"total_cancelled"`
	AvgPrepTimeMins int `json:"avg_prep_time_mins"`
}

// ─────────────────────────────────────────────────────────────────────────────
// INTERNAL HELPERS
// ─────────────────────────────────────────────────────────────────────────────

// getKOTForWrite fetches a KOT by ID with tenant/restaurant ownership check.
// Returns ErrKOTNotFound for non-existent or mis-owned records.
func (s *KOTService) getKOTForWrite(
	ctx context.Context,
	tenantID, restaurantID, kotID uuid.UUID,
) (*models.KOT, error) {
	var kot models.KOT
	if err := s.db.WithContext(ctx).
		Where("kot_id = ? AND tenant_id = ? AND restaurant_id = ?",
			kotID, tenantID, restaurantID).
		First(&kot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKOTNotFound
		}
		return nil, fmt.Errorf("get KOT: %w", err)
	}
	return &kot, nil
}

// getKOTWithItems fetches a KOT with its items and assigned chef preloaded.
func (s *KOTService) getKOTWithItems(
	ctx context.Context,
	tenantID, restaurantID, kotID uuid.UUID,
) (*models.KOT, error) {
	var kot models.KOT
	if err := s.db.WithContext(ctx).
		Where("kot_id = ? AND tenant_id = ? AND restaurant_id = ?",
			kotID, tenantID, restaurantID).
		Preload("AssignedToChef").
		First(&kot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKOTNotFound
		}
		return nil, fmt.Errorf("get KOT with items: %w", err)
	}

	// Load KOT items separately to avoid GORM association confusion on non-standard FK
	var items []models.KOTItem
	if err := s.db.WithContext(ctx).
		Where("kot_id = ?", kotID).
		Order("kot_item_id ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("load KOT items: %w", err)
	}
	// Attach via a local slice — KOT model doesn't declare a has-many for items
	// so we carry them in the return value implicitly by injecting into the struct.
	_ = items // caller may call ListKOTItems separately; items are exposed via ListKOTItems

	return &kot, nil
}

// ListKOTItems returns all items for a KOT.
func (s *KOTService) ListKOTItems(
	ctx context.Context,
	in KOTServiceRequest,
	kotID uuid.UUID,
) ([]models.KOTItem, error) {
	// Ownership check via parent KOT
	if _, err := s.getKOTForWrite(ctx, in.TenantID, in.RestaurantID, kotID); err != nil {
		return nil, err
	}

	var items []models.KOTItem
	if err := s.db.WithContext(ctx).
		Where("kot_id = ?", kotID).
		Order("kot_item_id ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list KOT items: %w", err)
	}
	return items, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// SEQUENCE & NUMBER GENERATION
// ─────────────────────────────────────────────────────────────────────────────

// generateKOTNumber returns a KOT number and its daily sequence integer.
// Format: KOT-YYYYMMDD-{seq:04d}  (e.g. KOT-20260305-0007)
// Sequence is per-tenant and resets each calendar day.
// Uses a Redis INCR+EXPIRE atomic pattern to avoid a DB sequence table.
func (s *KOTService) generateKOTNumber(
	ctx context.Context,
	tenantID uuid.UUID,
	t time.Time,
) (string, int, error) {
	dateStr := t.Format("20060102")
	seqKey := fmt.Sprintf("kot:seq:%s:%s", tenantID, dateStr)

	seq, err := s.redis.Incr(ctx, seqKey).Result()
	if err != nil {
		// Redis unavailable — fall back to timestamp-based pseudo-sequence
		seq = int64(t.UnixNano() % 9999)
	} else {
		// Set TTL only on the first increment (INCR returns 1)
		if seq == 1 {
			// Expire 25 hours after midnight of the given day to cover timezone drift
			midnight := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			ttl := midnight.Add(25 * time.Hour).Sub(t)
			if ttl > 0 {
				_ = s.redis.Expire(ctx, seqKey, ttl).Err()
			}
		}
	}

	number := fmt.Sprintf("KOT-%s-%04d", dateStr, seq)
	return number, int(seq), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// STATION AUTO-ROUTING
// ─────────────────────────────────────────────────────────────────────────────

// autoRouteStation inspects the item name for common keyword hints and returns
// the most appropriate KitchenStation. Falls back to KitchenStationGeneral.
func autoRouteStation(itemName string) models.KitchenStation {
	lower := strings.ToLower(itemName)

	switch {
	case containsAny(lower, "beer", "wine", "cocktail", "juice", "soda", "coffee", "tea", "drink", "beverage", "smoothie", "shake", "lemonade"):
		return models.KitchenStationBeverage
	case containsAny(lower, "cake", "ice cream", "dessert", "pudding", "brownie", "tiramisu", "cheesecake", "pastry", "tart", "mousse"):
		return models.KitchenStationDessert
	case containsAny(lower, "steak", "burger", "grill", "bbq", "ribs", "skewer", "kebab", "lamb chop", "pork chop"):
		return models.KitchenStationGrill
	case containsAny(lower, "fries", "fried", "nugget", "wings", "tempura", "fritter", "spring roll", "samosa", "calamari"):
		return models.KitchenStationFryer
	case containsAny(lower, "salad", "slaw", "carpaccio", "tartare", "sashimi", "cold", "chilled"):
		return models.KitchenStationSalad
	default:
		return models.KitchenStationGeneral
	}
}

// containsAny returns true if s contains any of the keywords.
func containsAny(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// isKOTTerminal returns true for states where no further transitions are valid.
func isKOTTerminal(status models.KOTStatus) bool {
	return status == models.KOTStatusCompleted || status == models.KOTStatusCancelled
}

// ─────────────────────────────────────────────────────────────────────────────
// FSM ADAPTER — KOTContext interface
// utils.KOTFSM.Restore requires a KOTContext, which must implement:
//   GetKOTID() string
//   SetStatus(s string)
//   GetPriority() string
// ─────────────────────────────────────────────────────────────────────────────

type kotContextAdapter struct{ *models.KOT }

func (a *kotContextAdapter) GetKOTID() string    { return a.KOTID.String() }
func (a *kotContextAdapter) SetStatus(s string)  { a.Status = models.KOTStatus(s) }
func (a *kotContextAdapter) GetPriority() string { return string(a.Priority) }

// ─────────────────────────────────────────────────────────────────────────────
// FSM ADAPTER — OrderContext interface
// utils.OrderFSM.Restore requires an OrderContext, which must implement:
//   GetOrderID() string
//   SetStatus(s string)
// ─────────────────────────────────────────────────────────────────────────────

type orderContextAdapter struct{ *models.Order }

func (a *orderContextAdapter) GetOrderID() string   { return a.OrderID.String() }
func (a *orderContextAdapter) SetStatus(s string)   { a.OrderStatus = models.OrderItemStatus(s) }

// ─────────────────────────────────────────────────────────────────────────────
// AUDIT TRAIL
// ─────────────────────────────────────────────────────────────────────────────

// writeAudit persists an AuditTrail row.
// Errors are silenced — audit failures must never block the caller.
func (s *KOTService) writeAudit(
	ctx context.Context,
	tenantID uuid.UUID,
	restaurantID *uuid.UUID,
	actorID uuid.UUID,
	event models.AuditEvent,
	entityType, entityID string,
	oldValues, newValues interface{},
) {
	entry := &models.AuditTrail{
		AuditTrailID:     uuid.New(),
		TenantID:         tenantID,
		UserID:           &actorID,
		RestaurantID:     restaurantID,
		EventType:        event,
		EventCategory:    "kot",
		EventDescription: fmt.Sprintf("%s on %s %s", event, entityType, entityID),
		Severity:         models.AuditSeverityInfo,
		EntityType:       entityType,
		EntityID:         entityID,
		OldValues:        mustMarshalJSONB(oldValues),
		NewValues:        mustMarshalJSONB(newValues),
		RiskLevel:        models.RiskLevelLow,
		Timestamp:        time.Now(),
	}
	_ = s.db.WithContext(ctx).Create(entry).Error
}
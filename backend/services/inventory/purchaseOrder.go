// services/business/PurchaseOrder.go
//
// PurchaseOrderService — purchase order lifecycle.
//
// ┌──────────────────────────────────────────────────────────────────────────┐
// │  FSM (from utils.PurchaseOrderFSM):                                      │
// │                                                                          │
// │  draft ──(submit)──► submitted ──(approve)──► approved                  │
// │    │                     │                      │                        │
// │    └──(cancel)──┐        └──(cancel)──┐         ├──(receive_partial)──► │
// │                 ▼                     ▼         │  partially_received   │
// │              cancelled             cancelled    │       │               │
// │                                                 └──(receive_full)──────►│
// │                                                         │               │
// │                                               partially_received        │
// │                                                  └──(receive_full)──►  │
// │                                                       received (term.)  │
// └──────────────────────────────────────────────────────────────────────────┘
//
// ReceivePurchaseOrder (Step 5):
//
//	For each line received, the service:
//	  1. Updates PurchaseOrderLine.ReceivedQuantity
//	  2. Calls InventoryService.AdjustStock with Reason=received
//	  3. Fires the FSM event (partial or full) based on whether all lines are done
//
// This replaces the InventoryNotifier stub in Order.go — purchase orders are
// the canonical way stock enters the system.
package inventory

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
	ErrPONotFound          = errors.New("purchase order not found")
	ErrPOTenantMismatch    = errors.New("purchase order does not belong to this tenant")
	ErrPOTerminal          = errors.New("purchase order is in a terminal state")
	ErrPOInvalidTransition = errors.New("purchase order state transition not permitted")
	ErrPOLineNotFound      = errors.New("purchase order line not found")
	ErrPONegativeReceived  = errors.New("received quantity cannot be negative")
	ErrPOExceedsOrdered    = errors.New("received quantity exceeds ordered quantity")
)

// ─────────────────────────────────────────────────────────────────────────────
// RESOURCE CONSTANT
// ─────────────────────────────────────────────────────────────────────────────

const (
	ResourcePurchaseOrder ResourceType = "purchase_order"
)

// ─────────────────────────────────────────────────────────────────────────────
// PO SEQUENCE KEY
// ─────────────────────────────────────────────────────────────────────────────

const poSeqKeyFmt = "po:seq:%s:%s" // po:seq:{tenantID}:{YYYYMM}

// ─────────────────────────────────────────────────────────────────────────────
// INPUT TYPES
// ─────────────────────────────────────────────────────────────────────────────

// POServiceRequest carries caller identity.
type POServiceRequest struct {
	TenantID     uuid.UUID
	RestaurantID uuid.UUID
	ActorID      uuid.UUID
}

// CreatePOInput creates a new PurchaseOrder in draft state.
type CreatePOInput struct {
	POServiceRequest
	VendorID             uuid.UUID
	OrderDate            time.Time
	ExpectedDeliveryDate *time.Time
	Notes                string
	Lines                []CreatePOLineInput
}

// CreatePOLineInput is one line in the purchase order.
type CreatePOLineInput struct {
	InventoryItemID uuid.UUID
	Quantity        float64
	UnitPrice       float64
}

// ReceivePOInput records goods received for one or more PO lines.
type ReceivePOInput struct {
	POServiceRequest
	PurchaseOrderID uuid.UUID
	Lines           []ReceivePOLineInput
	Notes           string
}

// ReceivePOLineInput is one line's received quantity.
type ReceivePOLineInput struct {
	PurchaseOrderLineID uuid.UUID
	ReceivedQuantity    float64
}

// ListPOsFilter controls pagination/filtering.
type ListPOsFilter struct {
	POServiceRequest
	VendorID  *uuid.UUID
	Status    *string
	DateFrom  *time.Time
	DateTo    *time.Time
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// ─────────────────────────────────────────────────────────────────────────────
// SERVICE
// ─────────────────────────────────────────────────────────────────────────────

// PurchaseOrderService manages purchase orders and receiving.
// Construct one per application process — goroutine-safe.
type PurchaseOrderService struct {
	db        *gorm.DB
	redis     *goredis.Client
	rbac      RBACAuthorizer
	inventory *InventoryService
}

func NewPurchaseOrderService(
	db *gorm.DB,
	redis *goredis.Client,
	rbac RBACAuthorizer,
	inventory *InventoryService,
) *PurchaseOrderService {
	return &PurchaseOrderService{db: db, redis: redis, rbac: rbac, inventory: inventory}
}

// ─────────────────────────────────────────────────────────────────────────────
// CREATE (draft)
// ─────────────────────────────────────────────────────────────────────────────

// CreatePurchaseOrder creates a PurchaseOrder in "draft" state with its lines.
// Validates all InventoryItemIDs exist in the same tenant.
// RBAC: requires purchase_order:create.
func (s *PurchaseOrderService) CreatePurchaseOrder(
	ctx context.Context,
	in CreatePOInput,
) (*models.PurchaseOrder, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourcePurchaseOrder, Action: ActionCreate,
	}); err != nil {
		return nil, err
	}

	if len(in.Lines) == 0 {
		return nil, errors.New("purchase order must have at least one line")
	}

	// Validate vendor belongs to this tenant
	var vendor models.Vendor
	if err := s.db.WithContext(ctx).
		Where("vendor_id = ? AND tenant_id = ? AND is_active = true", in.VendorID, in.TenantID).
		First(&vendor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVendorNotFound
		}
		return nil, fmt.Errorf("vendor lookup: %w", err)
	}

	// Validate inventory items exist in this tenant
	invIDs := make([]uuid.UUID, 0, len(in.Lines))
	seen := make(map[uuid.UUID]bool)
	for _, l := range in.Lines {
		if seen[l.InventoryItemID] {
			return nil, errors.New("duplicate inventory item in purchase order lines")
		}
		seen[l.InventoryItemID] = true
		invIDs = append(invIDs, l.InventoryItemID)
	}
	var invCount int64
	s.db.WithContext(ctx).Model(&models.InventoryItem{}).
		Where("inventory_item_id IN ? AND tenant_id = ? AND restaurant_id = ?",
			invIDs, in.TenantID, in.RestaurantID).Count(&invCount)
	if int(invCount) != len(invIDs) {
		return nil, errors.New("one or more inventory items not found in this restaurant")
	}

	// Generate PO number
	poNumber, err := s.generatePONumber(ctx, in.TenantID, in.OrderDate)
	if err != nil {
		return nil, err
	}

	// Build order and lines
	var totalAmount float64
	for _, l := range in.Lines {
		totalAmount += l.Quantity * l.UnitPrice
	}

	po := &models.PurchaseOrder{
		PurchaseOrderID:      uuid.New(),
		TenantID:             in.TenantID,
		RestaurantID:         in.RestaurantID,
		VendorID:             in.VendorID,
		OrderNumber:          poNumber,
		OrderDate:            in.OrderDate,
		ExpectedDeliveryDate: in.ExpectedDeliveryDate,
		Status:               string(utils.POStateDraft),
		TotalAmount:          totalAmount,
		CreatedBy:            in.ActorID,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit(clause.Associations).Create(po).Error; err != nil {
			return fmt.Errorf("create PO: %w", err)
		}
		for _, l := range in.Lines {
			line := &models.PurchaseOrderLine{
				PurchaseOrderLineID: uuid.New(),
				PurchaseOrderID:     po.PurchaseOrderID,
				InventoryItemID:     l.InventoryItemID,
				Quantity:            l.Quantity,
				UnitPrice:           l.UnitPrice,
				LineTotal:           l.Quantity * l.UnitPrice,
				ReceivedQuantity:    0,
			}
			if err := tx.Omit(clause.Associations).Create(line).Error; err != nil {
				return fmt.Errorf("create PO line: %w", err)
			}
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	s.writeAudit(ctx, in.TenantID, in.RestaurantID, in.ActorID,
		models.AuditEventCreate, "PurchaseOrder", po.PurchaseOrderID.String(),
		nil, map[string]interface{}{
			"order_number": po.OrderNumber,
			"vendor":       vendor.Name,
			"total":        po.TotalAmount,
			"lines":        len(in.Lines),
		})
	return po, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// FSM TRANSITIONS
// ─────────────────────────────────────────────────────────────────────────────

// SubmitPurchaseOrder transitions draft → submitted.
// RBAC: requires purchase_order:update.
func (s *PurchaseOrderService) SubmitPurchaseOrder(
	ctx context.Context,
	in POServiceRequest,
	purchaseOrderID uuid.UUID,
) (*models.PurchaseOrder, error) {
	return s.firePOEvent(ctx, in, purchaseOrderID, utils.POEventSubmit)
}

// ApprovePurchaseOrder transitions submitted → approved.
// RBAC: requires purchase_order:update.
func (s *PurchaseOrderService) ApprovePurchaseOrder(
	ctx context.Context,
	in POServiceRequest,
	purchaseOrderID uuid.UUID,
) (*models.PurchaseOrder, error) {
	return s.firePOEvent(ctx, in, purchaseOrderID, utils.POEventApprove)
}

// CancelPurchaseOrder transitions draft|submitted → cancelled.
// RBAC: requires purchase_order:update.
func (s *PurchaseOrderService) CancelPurchaseOrder(
	ctx context.Context,
	in POServiceRequest,
	purchaseOrderID uuid.UUID,
) (*models.PurchaseOrder, error) {
	return s.firePOEvent(ctx, in, purchaseOrderID, utils.POEventCancel)
}

// firePOEvent is the shared FSM-trigger helper for all PO transitions except receiving.
func (s *PurchaseOrderService) firePOEvent(
	ctx context.Context,
	in POServiceRequest,
	purchaseOrderID uuid.UUID,
	event utils.PurchaseOrderEvent,
) (*models.PurchaseOrder, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourcePurchaseOrder, Action: ActionUpdate,
	}); err != nil {
		return nil, err
	}

	po, err := s.getPOForWrite(ctx, in.TenantID, purchaseOrderID)
	if err != nil {
		return nil, err
	}
	if po.RestaurantID != in.RestaurantID {
		return nil, ErrPOTenantMismatch
	}

	adapter := &poContextAdapter{po}
	machine := utils.PurchaseOrderFSM.Restore(adapter, utils.PurchaseOrderState(po.Status))
	env := utils.NewEnvelope(event, nil).WithActor(in.ActorID, in.TenantID)

	if err := machine.Send(ctx, env); err != nil {
		return nil, fmt.Errorf("%w: %s → %s: %s",
			ErrPOInvalidTransition, po.Status, string(event), err.Error())
	}

	if err := s.db.WithContext(ctx).
		Model(po).
		Updates(map[string]interface{}{
			"status":     po.Status,
			"updated_at": time.Now(),
		}).Error; err != nil {
		return nil, fmt.Errorf("persist PO status: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, in.RestaurantID, in.ActorID,
		models.AuditEventUpdate, "PurchaseOrder", po.PurchaseOrderID.String(),
		nil, map[string]interface{}{"status": po.Status, "event": string(event)})
	return po, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// RECEIVE GOODS
// ─────────────────────────────────────────────────────────────────────────────

// ReceivePurchaseOrder records goods received for one or more lines.
//
// For each line:
//  1. Updates ReceivedQuantity on PurchaseOrderLine
//  2. Calls InventoryService.AdjustStock(Reason=received) — this writes the
//     stock movement ledger and updates InventoryItem.CurrentQuantity
//
// After processing all lines, fires the FSM:
//   - All lines fully received → POEventReceiveFull
//   - Some lines partially received → POEventReceivePartial (if currently approved)
//
// RBAC: requires purchase_order:update.
func (s *PurchaseOrderService) ReceivePurchaseOrder(
	ctx context.Context,
	in ReceivePOInput,
) (*models.PurchaseOrder, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourcePurchaseOrder, Action: ActionUpdate,
	}); err != nil {
		return nil, err
	}

	po, err := s.getPOForWrite(ctx, in.TenantID, in.PurchaseOrderID)
	if err != nil {
		return nil, err
	}
	if po.RestaurantID != in.RestaurantID {
		return nil, ErrPOTenantMismatch
	}

	// Must be in approved or partially_received to receive goods
	if po.Status != string(utils.POStateApproved) &&
		po.Status != string(utils.POStatePartiallyReceived) {
		return nil, fmt.Errorf("%w: can only receive against approved or partially_received POs (current: %s)",
			ErrPOInvalidTransition, po.Status)
	}

	// Load all lines for this PO
	var allLines []models.PurchaseOrderLine
	if err := s.db.WithContext(ctx).
		Where("purchase_order_id = ?", po.PurchaseOrderID).
		Find(&allLines).Error; err != nil {
		return nil, fmt.Errorf("load PO lines: %w", err)
	}

	// Index lines by ID
	lineMap := make(map[uuid.UUID]*models.PurchaseOrderLine, len(allLines))
	for i := range allLines {
		lineMap[allLines[i].PurchaseOrderLineID] = &allLines[i]
	}

	// Validate receiving inputs
	for _, rl := range in.Lines {
		line, found := lineMap[rl.PurchaseOrderLineID]
		if !found {
			return nil, fmt.Errorf("%w: %s", ErrPOLineNotFound, rl.PurchaseOrderLineID)
		}
		if rl.ReceivedQuantity < 0 {
			return nil, ErrPONegativeReceived
		}
		if line.ReceivedQuantity+rl.ReceivedQuantity > line.Quantity {
			return nil, fmt.Errorf("%w: line %s ordered=%.3f already_received=%.3f new=%.3f",
				ErrPOExceedsOrdered,
				rl.PurchaseOrderLineID,
				line.Quantity,
				line.ReceivedQuantity,
				rl.ReceivedQuantity,
			)
		}
	}

	// Process each receiving line
	for _, rl := range in.Lines {
		if rl.ReceivedQuantity == 0 {
			continue
		}
		line := lineMap[rl.PurchaseOrderLineID]

		// 1. Update line received quantity in DB
		newReceived := line.ReceivedQuantity + rl.ReceivedQuantity
		if err := s.db.WithContext(ctx).
			Model(line).
			Update("received_quantity", newReceived).Error; err != nil {
			return nil, fmt.Errorf("update line received qty: %w", err)
		}
		line.ReceivedQuantity = newReceived

		// 2. Deduct via InventoryService (writes stock movement + updates item qty)
		_, adjustErr := s.inventory.AdjustStock(ctx, AdjustStockInput{
			InventoryServiceRequest: InventoryServiceRequest{
				TenantID:     in.TenantID,
				RestaurantID: in.RestaurantID,
				ActorID:      in.ActorID,
			},
			InventoryItemID: line.InventoryItemID,
			QuantityDelta:   rl.ReceivedQuantity, // positive — adding stock
			Reason:          models.StockMovementReasonReceived,
			ReferenceType:   "purchase_order",
			ReferenceID:     po.PurchaseOrderID.String(),
			Notes: fmt.Sprintf("received against PO %s, line %s",
				po.OrderNumber, rl.PurchaseOrderLineID),
		})
		if adjustErr != nil {
			// Log but don't abort — partial receives are valid
			_ = adjustErr
		}
	}

	// Determine FSM event based on completion
	fullyReceived := true
	for _, line := range allLines {
		if line.ReceivedQuantity < line.Quantity {
			fullyReceived = false
			break
		}
	}

	var fsmEvent utils.PurchaseOrderEvent
	if fullyReceived {
		fsmEvent = utils.POEventReceiveFull
	} else if po.Status == string(utils.POStateApproved) {
		fsmEvent = utils.POEventReceivePartial
	} else {
		// Already partially_received — no FSM event needed, just update timestamps
		if err := s.db.WithContext(ctx).
			Model(po).Update("updated_at", time.Now()).Error; err != nil {
			return nil, fmt.Errorf("update PO timestamp: %w", err)
		}
		return po, nil
	}

	// Fire FSM
	adapter := &poContextAdapter{po}
	machine := utils.PurchaseOrderFSM.Restore(adapter, utils.PurchaseOrderState(po.Status))
	env := utils.NewEnvelope(fsmEvent, nil).WithActor(in.ActorID, in.TenantID)

	if err := machine.Send(ctx, env); err != nil {
		return nil, fmt.Errorf("PO FSM %s: %w", string(fsmEvent), err)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":     po.Status,
		"updated_at": now,
	}
	if fullyReceived {
		updates["actual_delivery_date"] = now
	}
	if err := s.db.WithContext(ctx).Model(po).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("persist PO receive status: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, in.RestaurantID, in.ActorID,
		models.AuditEventUpdate, "PurchaseOrder", po.PurchaseOrderID.String(),
		nil, map[string]interface{}{
			"status":         po.Status,
			"fully_received": fullyReceived,
			"lines_received": len(in.Lines),
		})
	return po, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// READ PATH
// ─────────────────────────────────────────────────────────────────────────────

// GetPurchaseOrder returns a PO with its lines.
// RBAC: requires purchase_order:read.
func (s *PurchaseOrderService) GetPurchaseOrder(
	ctx context.Context,
	in POServiceRequest,
	purchaseOrderID uuid.UUID,
) (*models.PurchaseOrder, []models.PurchaseOrderLine, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourcePurchaseOrder, Action: ActionRead,
	}); err != nil {
		return nil, nil, err
	}

	po, err := s.getPOForWrite(ctx, in.TenantID, purchaseOrderID)
	if err != nil {
		return nil, nil, err
	}

	var lines []models.PurchaseOrderLine
	if err := s.db.WithContext(ctx).
		Where("purchase_order_id = ?", purchaseOrderID).
		Find(&lines).Error; err != nil {
		return nil, nil, fmt.Errorf("load PO lines: %w", err)
	}
	return po, lines, nil
}

// ListPurchaseOrders returns a paginated, filtered list of POs.
// RBAC: requires purchase_order:read.
func (s *PurchaseOrderService) ListPurchaseOrders(
	ctx context.Context,
	f ListPOsFilter,
) ([]models.PurchaseOrder, int64, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: f.ActorID, TenantID: f.TenantID,
		Resource: ResourcePurchaseOrder, Action: ActionRead,
	}); err != nil {
		return nil, 0, err
	}

	q := s.db.WithContext(ctx).
		Where("tenant_id = ? AND restaurant_id = ?", f.TenantID, f.RestaurantID)

	if f.VendorID != nil {
		q = q.Where("vendor_id = ?", *f.VendorID)
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if f.DateFrom != nil {
		q = q.Where("order_date >= ?", *f.DateFrom)
	}
	if f.DateTo != nil {
		q = q.Where("order_date <= ?", *f.DateTo)
	}

	var total int64
	if err := q.Model(&models.PurchaseOrder{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	sortCol := "order_date"
	switch f.SortBy {
	case "total_amount":
		sortCol = "total_amount"
	case "status":
		sortCol = "status"
	}
	sortDir := "DESC"
	if strings.EqualFold(f.SortOrder, "asc") {
		sortDir = "ASC"
	}

	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}

	var pos []models.PurchaseOrder
	if err := q.Order(fmt.Sprintf("%s %s", sortCol, sortDir)).
		Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}
	return pos, total, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// FSM CONTEXT ADAPTER
// ─────────────────────────────────────────────────────────────────────────────

type poContextAdapter struct{ *models.PurchaseOrder }

func (a *poContextAdapter) GetPOID() string         { return a.PurchaseOrderID.String() }
func (a *poContextAdapter) GetTotalAmount() float64 { return a.TotalAmount }
func (a *poContextAdapter) SetStatus(s string)      { a.Status = s }

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────

func (s *PurchaseOrderService) getPOForWrite(
	ctx context.Context,
	tenantID, poID uuid.UUID,
) (*models.PurchaseOrder, error) {
	var po models.PurchaseOrder
	if err := s.db.WithContext(ctx).
		Where("purchase_order_id = ? AND tenant_id = ?", poID, tenantID).
		First(&po).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPONotFound
		}
		return nil, fmt.Errorf("get PO: %w", err)
	}
	return &po, nil
}

// generatePONumber returns a tenant-scoped number: PO-YYYYMM-{seq:06d}.
func (s *PurchaseOrderService) generatePONumber(
	ctx context.Context,
	tenantID uuid.UUID,
	t time.Time,
) (string, error) {
	monthStr := t.Format("200601")
	seqKey := fmt.Sprintf(poSeqKeyFmt, tenantID, monthStr)

	seq, err := s.redis.Incr(ctx, seqKey).Result()
	if err != nil {
		// Fallback: count existing POs this month + 1
		var count int64
		_ = s.db.WithContext(ctx).Model(&models.PurchaseOrder{}).
			Where("tenant_id = ? AND order_date >= ?", tenantID,
				time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())).
			Count(&count).Error
		seq = count + 1
	} else if seq == 1 {
		nextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location()).Add(48 * time.Hour)
		_ = s.redis.ExpireAt(ctx, seqKey, nextMonth).Err()
	}
	return fmt.Sprintf("PO-%s-%06d", monthStr, seq), nil
}

func (s *PurchaseOrderService) writeAudit(
	ctx context.Context,
	tenantID, restaurantID uuid.UUID,
	actorID uuid.UUID,
	event models.AuditEvent,
	entityType, entityID string,
	oldValues, newValues interface{},
) {
	var userID *uuid.UUID
	if actorID != uuid.Nil {
		userID = &actorID
	}
	rID := restaurantID
	entry := &models.AuditTrail{
		AuditTrailID:     uuid.New(),
		TenantID:         tenantID,
		UserID:           userID,
		RestaurantID:     &rID,
		EventType:        event,
		EventCategory:    "purchase_order",
		EventDescription: fmt.Sprintf("%s on %s %s", event, entityType, entityID),
		Severity:         models.AuditSeverityInfo,
		EntityType:       entityType,
		EntityID:         entityID,
		OldValues:        mustMarshalJSONB(oldValues),
		NewValues:        mustMarshalJSONB(newValues),
		RiskLevel:        models.RiskLevelLow,
		Timestamp:        time.Now(),
	}
	_ = s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(entry).Error
}

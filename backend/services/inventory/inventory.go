// services/business/Inventory.go
//
// InventoryService — stock management engine.
//
// ┌──────────────────────────────────────────────────────────────────────────┐
// │  CONCERNS                                                                │
// │                                                                          │
// │  1. Item Lifecycle    — Create / Get / List / Update / Delete           │
// │                         with Redis read-through cache (TTL 5m)          │
// │                                                                          │
// │  2. Stock Movements   — AdjustStock (all manual reasons)                │
// │                         DeductForOrder (called by InventoryNotifier)    │
// │                         Both write to stock_movements ledger atomically  │
// │                         with the CurrentQuantity update.                │
// │                                                                          │
// │  3. Low-Stock Alerts  — After every deduction, checkLowStock() fires    │
// │                         a goroutine that writes a Notification row      │
// │                         when CurrentQuantity ≤ ReorderPoint.            │
// │                                                                          │
// │  4. Vendor Lifecycle  — Create / Get / List / Update / Deactivate       │
// │                                                                          │
// │  5. Menu-Inventory Links — Create / Delete / ListByMenuItem             │
// │                         Enables real-time stock checks in OrderService. │
// │                                                                          │
// │  6. Stats             — GetInventoryStats (counts + total value)        │
// │                         StockMovementHistory (paginated ledger)         │
// └──────────────────────────────────────────────────────────────────────────┘
//
// Cache keys:
//
//	inventory:item:{tenantID}:{itemID} — models.InventoryItem, TTL 5m
//	inventory:stats:{tenantID}:{restaurantID} — InventoryStats, TTL 2m
//
// All mutating operations invalidate affected keys before returning.
package inventory

import (
	"context"
	"encoding/json"
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
	ErrInventoryItemNotFound    = errors.New("inventory item not found")
	ErrInventoryItemTenant      = errors.New("inventory item does not belong to this tenant")
	ErrInventoryItemInUse       = errors.New("inventory item is referenced by active menu-item links and cannot be deleted")
	ErrInventoryLinkNotFound    = errors.New("menu-item inventory link not found")
	ErrInventoryLinkExists      = errors.New("this menu-item → inventory-item link already exists")
	ErrInventoryQuantityInvalid = errors.New("quantity per unit must be greater than 0")
	ErrInsufficientInventory    = errors.New("insufficient inventory for this adjustment")
	ErrVendorNotFound           = errors.New("vendor not found")
	ErrVendorTenantMismatch     = errors.New("vendor does not belong to this tenant")
	ErrZeroAdjustment           = errors.New("quantity adjustment cannot be zero")
)

// ─────────────────────────────────────────────────────────────────────────────
// RESOURCE CONSTANTS (extend Order.go constants — same package)
// ─────────────────────────────────────────────────────────────────────────────

const (
	ResourceInventory ResourceType = "inventory"
	ResourceVendor    ResourceType = "vendor"
)

// ─────────────────────────────────────────────────────────────────────────────
// CACHE TTLs
// ─────────────────────────────────────────────────────────────────────────────

const (
	inventoryItemCacheTTL  = 5 * time.Minute
	inventoryStatsCacheTTL = 2 * time.Minute
)

// ─────────────────────────────────────────────────────────────────────────────
// INPUT TYPES
// ─────────────────────────────────────────────────────────────────────────────

// InventoryServiceRequest carries caller identity for all mutations.
type InventoryServiceRequest struct {
	TenantID     uuid.UUID
	RestaurantID uuid.UUID
	ActorID      uuid.UUID
}

// CreateItemInput creates a new InventoryItem.
type CreateItemInput struct {
	InventoryServiceRequest
	Name            string
	Description     string
	SKU             string
	Category        string
	UnitOfMeasure   string
	CurrentQuantity float64
	MinimumQuantity *float64
	MaximumQuantity *float64
	ReorderPoint    *float64
	UnitCost        *float64
}

// UpdateItemInput applies a partial patch to an InventoryItem.
// Only non-nil fields are written.
type UpdateItemInput struct {
	InventoryServiceRequest
	InventoryItemID uuid.UUID
	Name            *string
	Description     *string
	Category        *string
	MinimumQuantity *float64
	MaximumQuantity *float64
	ReorderPoint    *float64
	UnitCost        *float64
}

// AdjustStockInput is a manual stock adjustment.
type AdjustStockInput struct {
	InventoryServiceRequest
	InventoryItemID uuid.UUID
	// QuantityDelta is signed: positive = add, negative = remove.
	QuantityDelta float64
	Reason        models.StockMovementReason
	Notes         string
	// ReferenceType / ReferenceID are optional links to source entities
	// (e.g. "purchase_order", poID.String()).
	ReferenceType string
	ReferenceID   string
}

// DeductForOrderInput is called by InventoryNotifier when an order is placed.
type DeductForOrderInput struct {
	TenantID     uuid.UUID
	RestaurantID uuid.UUID
	OrderID      uuid.UUID
	// Items maps MenuItemID → quantity ordered.
	Items map[uuid.UUID]int
}

// ListItemsFilter controls the inventory list endpoint.
type ListItemsFilter struct {
	InventoryServiceRequest
	Category     *string
	SKU          *string
	Search       *string // name/description
	StockStatus  *string // in_stock | low_stock | out_of_stock
	NeedsReorder *bool
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
}

// CreateVendorInput creates a new Vendor.
type CreateVendorInput struct {
	InventoryServiceRequest
	Name         string
	ContactName  string
	Email        string
	Phone        string
	Address      string
	PaymentTerms string
}

// UpdateVendorInput patches a Vendor.
type UpdateVendorInput struct {
	InventoryServiceRequest
	VendorID     uuid.UUID
	Name         *string
	ContactName  *string
	Email        *string
	Phone        *string
	Address      *string
	PaymentTerms *string
	IsActive     *bool
}

// CreateLinkInput links a MenuItem to an InventoryItem.
type CreateLinkInput struct {
	InventoryServiceRequest
	MenuItemID      uuid.UUID
	InventoryItemID uuid.UUID
	QuantityPerUnit float64
}

// StockMovementFilter controls the movement history list.
type StockMovementFilter struct {
	InventoryServiceRequest
	InventoryItemID *uuid.UUID
	Reason          *models.StockMovementReason
	DateFrom        *time.Time
	DateTo          *time.Time
	Page            int
	PageSize        int
}

// ─────────────────────────────────────────────────────────────────────────────
// RESULT TYPES
// ─────────────────────────────────────────────────────────────────────────────

// InventoryStats is the dashboard summary for a restaurant.
type InventoryStats struct {
	TotalItems      int     `json:"total_items"`
	TotalValue      float64 `json:"total_value"`        // Σ(current_qty × unit_cost)
	LowStockCount   int     `json:"low_stock_count"`    // items where qty ≤ reorder_point
	OutOfStockCount int     `json:"out_of_stock_count"` // items where qty = 0
	ReorderRequired int     `json:"reorder_required"`   // items where qty ≤ reorder_point AND is_available
}

// ─────────────────────────────────────────────────────────────────────────────
// SERVICE
// ─────────────────────────────────────────────────────────────────────────────

// InventoryService manages the full inventory lifecycle.
// Goroutine-safe; construct one per application process.
type InventoryService struct {
	db    *gorm.DB
	redis *goredis.Client
	rbac  RBACAuthorizer
}

func NewInventoryService(db *gorm.DB, redis *goredis.Client, rbac RBACAuthorizer) *InventoryService {
	return &InventoryService{db: db, redis: redis, rbac: rbac}
}

// ─────────────────────────────────────────────────────────────────────────────
// CONCERN 1 — ITEM LIFECYCLE
// ─────────────────────────────────────────────────────────────────────────────

// CreateItem creates a new InventoryItem and an opening StockMovement if
// CurrentQuantity > 0.
// RBAC: requires inventory:create.
func (s *InventoryService) CreateItem(
	ctx context.Context,
	in CreateItemInput,
) (*models.InventoryItem, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceInventory, Action: ActionCreate,
	}); err != nil {
		return nil, err
	}

	item := &models.InventoryItem{
		InventoryItemID: uuid.New(),
		TenantID:        in.TenantID,
		RestaurantID:    in.RestaurantID,
		Name:            in.Name,
		Description:     in.Description,
		SKU:             in.SKU,
		Category:        in.Category,
		UnitOfMeasure:   in.UnitOfMeasure,
		CurrentQuantity: in.CurrentQuantity,
		MinimumQuantity: in.MinimumQuantity,
		MaximumQuantity: in.MaximumQuantity,
		ReorderPoint:    in.ReorderPoint,
		UnitCost:        in.UnitCost,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit(clause.Associations).Create(item).Error; err != nil {
			return fmt.Errorf("create inventory item: %w", err)
		}

		// Opening stock movement for traceability
		if in.CurrentQuantity > 0 {
			move := &models.StockMovement{
				TenantID:        in.TenantID,
				RestaurantID:    in.RestaurantID,
				InventoryItemID: item.InventoryItemID,
				Reason:          models.StockMovementReasonAdjustment,
				QuantityDelta:   in.CurrentQuantity,
				BalanceAfter:    in.CurrentQuantity,
				ReferenceType:   "opening_stock",
				Notes:           "opening stock at item creation",
				CreatedBy:       in.ActorID,
				CreatedAt:       time.Now(),
			}
			if err := tx.Create(move).Error; err != nil {
				return fmt.Errorf("create opening movement: %w", err)
			}
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	s.writeAudit(ctx, in.TenantID, in.RestaurantID, in.ActorID,
		models.AuditEventCreate, "InventoryItem", item.InventoryItemID.String(),
		nil, map[string]interface{}{"name": item.Name, "qty": item.CurrentQuantity})

	s.invalidateItemCache(ctx, in.TenantID, item.InventoryItemID)
	s.invalidateStatsCache(ctx, in.TenantID, in.RestaurantID)

	// Evaluate initial stock state via FSM
	s.evaluateStockTransition(ctx, in.TenantID, item)

	return item, nil
}

// GetItem returns an InventoryItem by ID, with Redis read-through.
// RBAC: requires inventory:read.
func (s *InventoryService) GetItem(
	ctx context.Context,
	in InventoryServiceRequest,
	inventoryItemID uuid.UUID,
) (*models.InventoryItem, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceInventory, Action: ActionRead,
	}); err != nil {
		return nil, err
	}

	// ── Redis read-through ────────────────────────────────────────────────────
	cacheKey := fmt.Sprintf("inventory:item:%s:%s", in.TenantID, inventoryItemID)
	if data, err := s.redis.Get(ctx, cacheKey).Bytes(); err == nil {
		var item models.InventoryItem
		if json.Unmarshal(data, &item) == nil {
			return &item, nil
		}
	}

	item, err := s.getItemForWrite(ctx, in.TenantID, inventoryItemID)
	if err != nil {
		return nil, err
	}

	if data, err := json.Marshal(item); err == nil {
		_ = s.redis.Set(ctx, cacheKey, data, inventoryItemCacheTTL).Err()
	}
	return item, nil
}

// ListItems returns a paginated, filtered inventory list.
// RBAC: requires inventory:read.
func (s *InventoryService) ListItems(
	ctx context.Context,
	f ListItemsFilter,
) ([]models.InventoryItem, int64, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: f.ActorID, TenantID: f.TenantID,
		Resource: ResourceInventory, Action: ActionRead,
	}); err != nil {
		return nil, 0, err
	}

	q := s.db.WithContext(ctx).
		Where("tenant_id = ? AND restaurant_id = ?", f.TenantID, f.RestaurantID)

	if f.Category != nil {
		q = q.Where("category = ?", *f.Category)
	}
	if f.SKU != nil {
		q = q.Where("sku = ?", *f.SKU)
	}
	if f.Search != nil {
		q = q.Where("name ILIKE ? OR description ILIKE ?",
			"%"+*f.Search+"%", "%"+*f.Search+"%")
	}
	if f.StockStatus != nil {
		switch *f.StockStatus {
		case "out_of_stock":
			q = q.Where("current_quantity = 0")
		case "low_stock":
			q = q.Where("reorder_point IS NOT NULL AND current_quantity > 0 AND current_quantity <= reorder_point")
		case "in_stock":
			q = q.Where("(reorder_point IS NULL OR current_quantity > reorder_point)")
		}
	}
	if f.NeedsReorder != nil && *f.NeedsReorder {
		q = q.Where("reorder_point IS NOT NULL AND current_quantity <= reorder_point")
	}

	var total int64
	if err := q.Model(&models.InventoryItem{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count items: %w", err)
	}

	sortCol := "name"
	switch f.SortBy {
	case "sku":
		sortCol = "sku"
	case "current_quantity":
		sortCol = "current_quantity"
	case "unit_cost":
		sortCol = "unit_cost"
	case "created_at":
		sortCol = "created_at"
	}
	sortDir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortDir = "DESC"
	}
	q = q.Order(fmt.Sprintf("%s %s", sortCol, sortDir))

	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	q = q.Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize)

	var items []models.InventoryItem
	if err := q.Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list inventory items: %w", err)
	}
	return items, total, nil
}

// UpdateItem applies a partial update to an InventoryItem.
// RBAC: requires inventory:update.
func (s *InventoryService) UpdateItem(
	ctx context.Context,
	in UpdateItemInput,
) (*models.InventoryItem, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceInventory, Action: ActionUpdate,
	}); err != nil {
		return nil, err
	}

	item, err := s.getItemForWrite(ctx, in.TenantID, in.InventoryItemID)
	if err != nil {
		return nil, err
	}
	if item.RestaurantID != in.RestaurantID {
		return nil, ErrInventoryItemTenant
	}

	old := snapshotItem(item)
	updates := map[string]interface{}{"updated_at": time.Now()}

	if in.Name != nil {
		item.Name = *in.Name
		updates["name"] = *in.Name
	}
	if in.Description != nil {
		item.Description = *in.Description
		updates["description"] = *in.Description
	}
	if in.Category != nil {
		item.Category = *in.Category
		updates["category"] = *in.Category
	}
	if in.MinimumQuantity != nil {
		item.MinimumQuantity = in.MinimumQuantity
		updates["minimum_quantity"] = *in.MinimumQuantity
	}
	if in.MaximumQuantity != nil {
		item.MaximumQuantity = in.MaximumQuantity
		updates["maximum_quantity"] = *in.MaximumQuantity
	}
	if in.ReorderPoint != nil {
		item.ReorderPoint = in.ReorderPoint
		updates["reorder_point"] = *in.ReorderPoint
	}
	if in.UnitCost != nil {
		item.UnitCost = in.UnitCost
		updates["unit_cost"] = *in.UnitCost
	}

	if err := s.db.WithContext(ctx).
		Model(item).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update inventory item: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, in.RestaurantID, in.ActorID,
		models.AuditEventUpdate, "InventoryItem", item.InventoryItemID.String(),
		old, snapshotItem(item))
	s.invalidateItemCache(ctx, in.TenantID, item.InventoryItemID)
	s.invalidateStatsCache(ctx, in.TenantID, in.RestaurantID)

	return item, nil
}

// DeleteItem removes an InventoryItem. Blocked if active MenuItemInventoryLinks exist.
// RBAC: requires inventory:delete.
func (s *InventoryService) DeleteItem(
	ctx context.Context,
	in InventoryServiceRequest,
	inventoryItemID uuid.UUID,
) error {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceInventory, Action: ActionDelete,
	}); err != nil {
		return err
	}

	item, err := s.getItemForWrite(ctx, in.TenantID, inventoryItemID)
	if err != nil {
		return err
	}
	if item.RestaurantID != in.RestaurantID {
		return ErrInventoryItemTenant
	}

	// Guard: block delete if active links exist
	var linkCount int64
	if err := s.db.WithContext(ctx).Model(&models.MenuItemInventoryLink{}).
		Where("inventory_item_id = ? AND is_active = true", inventoryItemID).
		Count(&linkCount).Error; err != nil {
		return fmt.Errorf("link count check: %w", err)
	}
	if linkCount > 0 {
		return ErrInventoryItemInUse
	}

	if err := s.db.WithContext(ctx).Delete(item).Error; err != nil {
		return fmt.Errorf("delete inventory item: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, in.RestaurantID, in.ActorID,
		models.AuditEventDelete, "InventoryItem", inventoryItemID.String(),
		snapshotItem(item), nil)
	s.invalidateItemCache(ctx, in.TenantID, inventoryItemID)
	s.invalidateStatsCache(ctx, in.TenantID, in.RestaurantID)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// CONCERN 2 — STOCK MOVEMENTS
// ─────────────────────────────────────────────────────────────────────────────

// AdjustStock applies a manual stock adjustment (any reason except "used").
// Both the InventoryItem.CurrentQuantity and the StockMovement row are written
// in a single transaction — they are always consistent.
// RBAC: requires inventory:update.
func (s *InventoryService) AdjustStock(
	ctx context.Context,
	in AdjustStockInput,
) (*models.StockMovement, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceInventory, Action: ActionUpdate,
	}); err != nil {
		return nil, err
	}

	if in.QuantityDelta == 0 {
		return nil, ErrZeroAdjustment
	}

	item, err := s.getItemForWrite(ctx, in.TenantID, in.InventoryItemID)
	if err != nil {
		return nil, err
	}
	if item.RestaurantID != in.RestaurantID {
		return nil, ErrInventoryItemTenant
	}

	newQty := item.CurrentQuantity + in.QuantityDelta
	if newQty < 0 {
		return nil, fmt.Errorf("%w: current=%.3f, delta=%.3f",
			ErrInsufficientInventory, item.CurrentQuantity, in.QuantityDelta)
	}

	var move *models.StockMovement
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Update item quantity
		if err := tx.Model(item).Updates(map[string]interface{}{
			"current_quantity": newQty,
			"updated_at":       time.Now(),
		}).Error; err != nil {
			return fmt.Errorf("update quantity: %w", err)
		}

		// 2. Write movement record
		move = &models.StockMovement{
			TenantID:        in.TenantID,
			RestaurantID:    in.RestaurantID,
			InventoryItemID: in.InventoryItemID,
			Reason:          in.Reason,
			QuantityDelta:   in.QuantityDelta,
			BalanceAfter:    newQty,
			ReferenceType:   in.ReferenceType,
			ReferenceID:     in.ReferenceID,
			Notes:           in.Notes,
			CreatedBy:       in.ActorID,
		}
		if err := tx.Create(move).Error; err != nil {
			return fmt.Errorf("create stock movement: %w", err)
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	item.CurrentQuantity = newQty
	s.invalidateItemCache(ctx, in.TenantID, in.InventoryItemID)
	s.invalidateStatsCache(ctx, in.TenantID, in.RestaurantID)

	// Evaluate stock state transition via FSM — replaces the old ad-hoc
	// checkLowStock goroutine with a formal state machine transition.
	s.evaluateStockTransition(ctx, in.TenantID, item)

	return move, nil
}

// DeductForOrder is called by InventoryNotifier when an order is placed.
// Resolves MenuItemInventoryLinks and deducts the consumed quantities.
// Fails silently on stock shortfalls (logged to audit) — orders are never
// blocked by inventory at the deduction stage (pre-check happens in OrderService).
func (s *InventoryService) DeductForOrder(ctx context.Context, in DeductForOrderInput) error {
	if len(in.Items) == 0 {
		return nil
	}

	menuItemIDs := make([]uuid.UUID, 0, len(in.Items))
	for id := range in.Items {
		menuItemIDs = append(menuItemIDs, id)
	}

	// Load all active links for these menu items
	var links []models.MenuItemInventoryLink
	if err := s.db.WithContext(ctx).
		Where("menu_item_id IN ? AND tenant_id = ? AND is_active = true",
			menuItemIDs, in.TenantID).
		Find(&links).Error; err != nil {
		return fmt.Errorf("load inventory links: %w", err)
	}
	if len(links) == 0 {
		return nil // no links configured — nothing to deduct
	}

	// Aggregate deductions per InventoryItem
	// One menu item can link to multiple inventory items
	type deduction struct {
		delta float64
	}
	deductions := make(map[uuid.UUID]*deduction)
	for _, link := range links {
		orderedQty := float64(in.Items[link.MenuItemID])
		delta := orderedQty * link.QuantityPerUnit
		if d, ok := deductions[link.InventoryItemID]; ok {
			d.delta += delta
		} else {
			deductions[link.InventoryItemID] = &deduction{delta: delta}
		}
	}

	// Deduct each item in its own mini-transaction
	for invItemID, ded := range deductions {
		// Fetch current qty — use SELECT FOR UPDATE to prevent race conditions
		var item models.InventoryItem
		if err := s.db.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("inventory_item_id = ? AND tenant_id = ?", invItemID, in.TenantID).
			First(&item).Error; err != nil {
			continue // item deleted — skip
		}

		newQty := item.CurrentQuantity - ded.delta
		if newQty < 0 {
			newQty = 0 // clamp — alert will fire below
		}

		txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&item).Updates(map[string]interface{}{
				"current_quantity": newQty,
				"updated_at":       time.Now(),
			}).Error; err != nil {
				return err
			}
			move := &models.StockMovement{
				TenantID:        in.TenantID,
				RestaurantID:    in.RestaurantID,
				InventoryItemID: invItemID,
				Reason:          models.StockMovementReasonUsed,
				QuantityDelta:   -ded.delta,
				BalanceAfter:    newQty,
				ReferenceType:   "order",
				ReferenceID:     in.OrderID.String(),
				Notes:           "auto-deducted on order creation",
				CreatedBy:       uuid.Nil, // system actor
			}
			return tx.Create(move).Error
		})
		if txErr != nil {
			// Log but don't fail the entire deduction batch
			_ = txErr
			continue
		}

		item.CurrentQuantity = newQty
		s.invalidateItemCache(ctx, in.TenantID, invItemID)

		// FSM-driven stock state evaluation
		s.evaluateStockTransition(ctx, in.TenantID, &item)
	}

	s.invalidateStatsCache(ctx, in.TenantID, in.RestaurantID)
	return nil
}

// StockMovementHistory returns a paginated audit ledger for an item or restaurant.
// RBAC: requires inventory:read.
func (s *InventoryService) StockMovementHistory(
	ctx context.Context,
	f StockMovementFilter,
) ([]models.StockMovement, int64, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: f.ActorID, TenantID: f.TenantID,
		Resource: ResourceInventory, Action: ActionRead,
	}); err != nil {
		return nil, 0, err
	}

	q := s.db.WithContext(ctx).
		Where("tenant_id = ? AND restaurant_id = ?", f.TenantID, f.RestaurantID)

	if f.InventoryItemID != nil {
		q = q.Where("inventory_item_id = ?", *f.InventoryItemID)
	}
	if f.Reason != nil {
		q = q.Where("reason = ?", *f.Reason)
	}
	if f.DateFrom != nil {
		q = q.Where("created_at >= ?", *f.DateFrom)
	}
	if f.DateTo != nil {
		q = q.Where("created_at <= ?", *f.DateTo)
	}

	var total int64
	if err := q.Model(&models.StockMovement{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if f.PageSize <= 0 {
		f.PageSize = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}

	var moves []models.StockMovement
	if err := q.Order("created_at DESC").
		Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize).
		Find(&moves).Error; err != nil {
		return nil, 0, err
	}
	return moves, total, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// CONCERN 3 — LOW-STOCK ALERTS
// ─────────────────────────────────────────────────────────────────────────────

// checkLowStock fires a Notification when CurrentQuantity ≤ ReorderPoint.
// Runs in a goroutine — uses a fresh background context.
// Debounced via Redis: one notification per item per 1 hour.
func (s *InventoryService) checkLowStock(
	ctx context.Context,
	tenantID uuid.UUID,
	item *models.InventoryItem,
) {
	if item.ReorderPoint == nil {
		return
	}
	if item.CurrentQuantity > *item.ReorderPoint {
		return
	}

	// Debounce: skip if we already sent an alert in the last hour
	debounceKey := fmt.Sprintf("inventory:lowstock:alert:%s", item.InventoryItemID)
	if _, err := s.redis.Get(ctx, debounceKey).Result(); err == nil {
		return // already alerted recently
	}
	_ = s.redis.Set(ctx, debounceKey, "1", time.Hour).Err()

	severity := "low_stock"
	if item.CurrentQuantity == 0 {
		severity = "out_of_stock"
	}

	msg := fmt.Sprintf(
		"[%s] %s: stock at %.2f %s (reorder point: %.2f)",
		strings.ToUpper(severity), item.Name,
		item.CurrentQuantity, item.UnitOfMeasure,
		*item.ReorderPoint,
	)

	notif := &models.Notification{
		NotificationID:   uuid.New(),
		TenantID:         tenantID,
		Message:          msg,
		NotificationType: string(models.NotificationTypeAlert),
		IsRead:           false,
		CreatedAt:        time.Now(),
	}
	_ = s.db.WithContext(ctx).Create(notif).Error
}

// ─────────────────────────────────────────────────────────────────────────────
// CONCERN 4 — VENDOR LIFECYCLE
// ─────────────────────────────────────────────────────────────────────────────

// CreateVendor creates a new Vendor.
// RBAC: requires vendor:create.
func (s *InventoryService) CreateVendor(
	ctx context.Context,
	in CreateVendorInput,
) (*models.Vendor, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceVendor, Action: ActionCreate,
	}); err != nil {
		return nil, err
	}

	vendor := &models.Vendor{
		VendorID:     uuid.New(),
		TenantID:     in.TenantID,
		Name:         in.Name,
		ContactName:  in.ContactName,
		Email:        in.Email,
		Phone:        in.Phone,
		Address:      in.Address,
		PaymentTerms: in.PaymentTerms,
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.db.WithContext(ctx).Omit(clause.Associations).Create(vendor).Error; err != nil {
		return nil, fmt.Errorf("create vendor: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, in.RestaurantID, in.ActorID,
		models.AuditEventCreate, "Vendor", vendor.VendorID.String(),
		nil, map[string]interface{}{"name": vendor.Name})
	return vendor, nil
}

// GetVendor returns a Vendor by ID.
// RBAC: requires vendor:read.
func (s *InventoryService) GetVendor(
	ctx context.Context,
	in InventoryServiceRequest,
	vendorID uuid.UUID,
) (*models.Vendor, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceVendor, Action: ActionRead,
	}); err != nil {
		return nil, err
	}
	return s.getVendorForWrite(ctx, in.TenantID, vendorID)
}

// ListVendors returns all vendors for a tenant (active only by default).
// RBAC: requires vendor:read.
func (s *InventoryService) ListVendors(
	ctx context.Context,
	in InventoryServiceRequest,
	includeInactive bool,
) ([]models.Vendor, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceVendor, Action: ActionRead,
	}); err != nil {
		return nil, err
	}

	q := s.db.WithContext(ctx).Where("tenant_id = ?", in.TenantID)
	if !includeInactive {
		q = q.Where("is_active = true")
	}

	var vendors []models.Vendor
	if err := q.Order("name ASC").Find(&vendors).Error; err != nil {
		return nil, fmt.Errorf("list vendors: %w", err)
	}
	return vendors, nil
}

// UpdateVendor applies a partial patch to a Vendor.
// RBAC: requires vendor:update.
func (s *InventoryService) UpdateVendor(
	ctx context.Context,
	in UpdateVendorInput,
) (*models.Vendor, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceVendor, Action: ActionUpdate,
	}); err != nil {
		return nil, err
	}

	vendor, err := s.getVendorForWrite(ctx, in.TenantID, in.VendorID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{"updated_at": time.Now()}
	if in.Name != nil {
		vendor.Name = *in.Name
		updates["name"] = *in.Name
	}
	if in.ContactName != nil {
		vendor.ContactName = *in.ContactName
		updates["contact_name"] = *in.ContactName
	}
	if in.Email != nil {
		vendor.Email = *in.Email
		updates["email"] = *in.Email
	}
	if in.Phone != nil {
		vendor.Phone = *in.Phone
		updates["phone"] = *in.Phone
	}
	if in.Address != nil {
		vendor.Address = *in.Address
		updates["address"] = *in.Address
	}
	if in.PaymentTerms != nil {
		vendor.PaymentTerms = *in.PaymentTerms
		updates["payment_terms"] = *in.PaymentTerms
	}
	if in.IsActive != nil {
		vendor.IsActive = *in.IsActive
		updates["is_active"] = *in.IsActive
	}

	if err := s.db.WithContext(ctx).Model(vendor).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update vendor: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, in.RestaurantID, in.ActorID,
		models.AuditEventUpdate, "Vendor", vendor.VendorID.String(), nil,
		map[string]interface{}{"name": vendor.Name, "is_active": vendor.IsActive})
	return vendor, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// CONCERN 5 — MENU-INVENTORY LINKS
// ─────────────────────────────────────────────────────────────────────────────

// CreateLink links a MenuItem to an InventoryItem.
// RBAC: requires inventory:update (it affects stock behaviour).
func (s *InventoryService) CreateLink(
	ctx context.Context,
	in CreateLinkInput,
) (*models.MenuItemInventoryLink, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceInventory, Action: ActionUpdate,
	}); err != nil {
		return nil, err
	}

	if in.QuantityPerUnit <= 0 {
		return nil, ErrInventoryQuantityInvalid
	}

	// Check for existing active link
	var existing models.MenuItemInventoryLink
	err := s.db.WithContext(ctx).
		Where("menu_item_id = ? AND inventory_item_id = ? AND tenant_id = ?",
			in.MenuItemID, in.InventoryItemID, in.TenantID).
		First(&existing).Error
	if err == nil {
		return nil, ErrInventoryLinkExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("link lookup: %w", err)
	}

	link := &models.MenuItemInventoryLink{
		TenantID:        in.TenantID,
		MenuItemID:      in.MenuItemID,
		InventoryItemID: in.InventoryItemID,
		QuantityPerUnit: in.QuantityPerUnit,
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := s.db.WithContext(ctx).Omit(clause.Associations).Create(link).Error; err != nil {
		return nil, fmt.Errorf("create link: %w", err)
	}
	return link, nil
}

// DeleteLink deactivates a MenuItemInventoryLink (soft delete).
// RBAC: requires inventory:update.
func (s *InventoryService) DeleteLink(
	ctx context.Context,
	in InventoryServiceRequest,
	linkID uuid.UUID,
) error {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceInventory, Action: ActionUpdate,
	}); err != nil {
		return err
	}

	result := s.db.WithContext(ctx).
		Model(&models.MenuItemInventoryLink{}).
		Where("link_id = ? AND tenant_id = ?", linkID, in.TenantID).
		Updates(map[string]interface{}{"is_active": false, "updated_at": time.Now()})
	if result.Error != nil {
		return fmt.Errorf("deactivate link: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrInventoryLinkNotFound
	}
	return nil
}

// ListLinksByMenuItem returns all active links for a given MenuItem.
// RBAC: requires inventory:read.
func (s *InventoryService) ListLinksByMenuItem(
	ctx context.Context,
	in InventoryServiceRequest,
	menuItemID uuid.UUID,
) ([]models.MenuItemInventoryLink, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceInventory, Action: ActionRead,
	}); err != nil {
		return nil, err
	}

	var links []models.MenuItemInventoryLink
	if err := s.db.WithContext(ctx).
		Where("menu_item_id = ? AND tenant_id = ? AND is_active = true",
			menuItemID, in.TenantID).
		Find(&links).Error; err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	return links, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// CONCERN 6 — STATS
// ─────────────────────────────────────────────────────────────────────────────

// GetInventoryStats returns summary stats for a restaurant.
// Cached in Redis with a 2-minute TTL.
// RBAC: requires inventory:read.
func (s *InventoryService) GetInventoryStats(
	ctx context.Context,
	in InventoryServiceRequest,
) (*InventoryStats, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID: in.ActorID, TenantID: in.TenantID,
		Resource: ResourceInventory, Action: ActionRead,
	}); err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("inventory:stats:%s:%s", in.TenantID, in.RestaurantID)
	if data, err := s.redis.Get(ctx, cacheKey).Bytes(); err == nil {
		var stats InventoryStats
		if json.Unmarshal(data, &stats) == nil {
			return &stats, nil
		}
	}

	type statsRow struct {
		TotalItems      int
		TotalValue      float64
		LowStockCount   int
		OutOfStockCount int
	}
	var row statsRow
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) AS total_items,
			COALESCE(SUM(current_quantity * COALESCE(unit_cost, 0)), 0) AS total_value,
			COUNT(*) FILTER (WHERE reorder_point IS NOT NULL
				AND current_quantity > 0
				AND current_quantity <= reorder_point) AS low_stock_count,
			COUNT(*) FILTER (WHERE current_quantity = 0) AS out_of_stock_count
		FROM inventory_items
		WHERE tenant_id = ? AND restaurant_id = ?
	`, in.TenantID, in.RestaurantID).Scan(&row).Error; err != nil {
		return nil, fmt.Errorf("stats query: %w", err)
	}

	stats := &InventoryStats{
		TotalItems:      row.TotalItems,
		TotalValue:      row.TotalValue,
		LowStockCount:   row.LowStockCount,
		OutOfStockCount: row.OutOfStockCount,
		ReorderRequired: row.LowStockCount + row.OutOfStockCount,
	}

	if data, err := json.Marshal(stats); err == nil {
		_ = s.redis.Set(ctx, cacheKey, data, inventoryStatsCacheTTL).Err()
	}
	return stats, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// INTERNAL HELPERS
// ─────────────────────────────────────────────────────────────────────────────

func (s *InventoryService) getItemForWrite(
	ctx context.Context,
	tenantID, itemID uuid.UUID,
) (*models.InventoryItem, error) {
	var item models.InventoryItem
	if err := s.db.WithContext(ctx).
		Where("inventory_item_id = ? AND tenant_id = ?", itemID, tenantID).
		First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInventoryItemNotFound
		}
		return nil, fmt.Errorf("get inventory item: %w", err)
	}
	return &item, nil
}

func (s *InventoryService) getVendorForWrite(
	ctx context.Context,
	tenantID, vendorID uuid.UUID,
) (*models.Vendor, error) {
	var vendor models.Vendor
	if err := s.db.WithContext(ctx).
		Where("vendor_id = ? AND tenant_id = ?", vendorID, tenantID).
		First(&vendor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVendorNotFound
		}
		return nil, fmt.Errorf("get vendor: %w", err)
	}
	return &vendor, nil
}

func (s *InventoryService) invalidateItemCache(ctx context.Context, tenantID, itemID uuid.UUID) {
	_ = s.redis.Del(ctx, fmt.Sprintf("inventory:item:%s:%s", tenantID, itemID)).Err()
}

func (s *InventoryService) invalidateStatsCache(ctx context.Context, tenantID, restaurantID uuid.UUID) {
	_ = s.redis.Del(ctx, fmt.Sprintf("inventory:stats:%s:%s", tenantID, restaurantID)).Err()
}

func snapshotItem(item *models.InventoryItem) map[string]interface{} {
	return map[string]interface{}{
		"name":             item.Name,
		"current_quantity": item.CurrentQuantity,
		"unit_cost":        item.UnitCost,
		"reorder_point":    item.ReorderPoint,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FSM INTEGRATION
// ─────────────────────────────────────────────────────────────────────────────

// inventoryItemAdapter wraps a models.InventoryItem to satisfy
// utils.InventoryItemContext. This keeps the FSM decoupled from the
// concrete model type.
type inventoryItemAdapter struct {
	item        *models.InventoryItem
	stockStatus string
}

func (a *inventoryItemAdapter) GetItemID() string           { return a.item.InventoryItemID.String() }
func (a *inventoryItemAdapter) GetCurrentQuantity() float64 { return a.item.CurrentQuantity }
func (a *inventoryItemAdapter) GetReorderPoint() *float64   { return a.item.ReorderPoint }
func (a *inventoryItemAdapter) SetStockStatus(s string)     { a.stockStatus = s }
func (a *inventoryItemAdapter) GetStockStatus() string      { return a.stockStatus }

// evaluateStockTransition derives the target FSM state from the item's current
// quantity and reorder point, then fires the appropriate FSM event. If the
// resulting state is low_stock or out_of_stock, a notification is generated
// via checkLowStock (which is now only called from here, not scattered across
// multiple call sites).
//
// This method is safe to call after any quantity mutation — it is idempotent
// with respect to the current state. If no transition is needed (the item
// is already in the correct state), the FSM Send() will return
// ErrInvalidTransition, which is silently ignored.
func (s *InventoryService) evaluateStockTransition(
	ctx context.Context,
	tenantID uuid.UUID,
	item *models.InventoryItem,
) {
	currentState := utils.DeriveInventoryState(item.CurrentQuantity, item.ReorderPoint)

	// Determine the appropriate event to reach the target state
	var event utils.InventoryStockEvent
	switch currentState {
	case utils.InventoryStateOutOfStock:
		event = utils.InventoryEventStockDepleted
	case utils.InventoryStateLowStock:
		event = utils.InventoryEventStockLow
	case utils.InventoryStateActive:
		event = utils.InventoryEventStockReplenished
	default:
		return
	}

	// We try the target state first; if the FSM is already there, the
	// transition lookup will fail and we silently skip.
	adapter := &inventoryItemAdapter{item: item, stockStatus: string(currentState)}

	// Restore from ALL possible source states — we do not know the previous
	// state (it is not persisted). We try each plausible source state.
	sourceStates := []utils.InventoryStockState{
		utils.InventoryStateActive,
		utils.InventoryStateLowStock,
		utils.InventoryStateOutOfStock,
	}

	for _, src := range sourceStates {
		if src == currentState {
			continue // already in target state — no transition needed
		}
		machine := utils.InventoryItemFSM.Restore(adapter, src)
		env := utils.NewEnvelope(event, nil)
		if err := machine.Send(ctx, env); err == nil {
			// Transition succeeded — fire notification if entering alert state
			if currentState == utils.InventoryStateLowStock || currentState == utils.InventoryStateOutOfStock {
				go s.checkLowStock(context.Background(), tenantID, item)
			}
			return
		}
	}
}

func (s *InventoryService) writeAudit(
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
		EventCategory:    "inventory",
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

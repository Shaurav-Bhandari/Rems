// services/business/inventory_notifier.go
//
// Drop-in replacement for the InventoryNotifier stub in Order.go.
// Copy this file into the same package — the stub's type definition will
// conflict, so delete these four declarations from Order.go:
//
//   type InventoryNotifier struct{ db *gorm.DB }
//   func NewInventoryNotifier(db *gorm.DB) *InventoryNotifier { ... }
//   func (n *InventoryNotifier) EventType() string { ... }
//   func (n *InventoryNotifier) Notify(_ context.Context, _ models.OrderCreatedPayload) error { ... }
//
// Then change the wire-up in NewDefaultOrderService from:
//   NewInventoryNotifier(db)
// to:
//   NewInventoryNotifier(inventorySvc)
package inventory

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"backend/models"
)

// InventoryNotifier deducts consumed stock when an order is placed.
// Satisfies DownstreamNotifier — registered with NewOrderService.
type InventoryNotifier struct {
	inventory *InventoryService
}

func NewInventoryNotifier(inventory *InventoryService) *InventoryNotifier {
	return &InventoryNotifier{inventory: inventory}
}

func (n *InventoryNotifier) EventType() string { return "OrderCreated" }

func (n *InventoryNotifier) Notify(ctx context.Context, payload models.OrderCreatedPayload) error {
	items := make(map[uuid.UUID]int, len(payload.Items))
	for _, item := range payload.Items {
		if item.MenuItemID != nil {
			items[*item.MenuItemID] += item.Quantity
		}
	}
	if len(items) == 0 {
		return nil
	}

	err := n.inventory.DeductForOrder(ctx, DeductForOrderInput{
		TenantID:     payload.TenantID,
		RestaurantID: payload.RestaurantID,
		OrderID:      payload.OrderID,
		Items:        items,
	})
	if err != nil {
		// Non-fatal: a deduction failure does not reverse the order.
		// Ops can correct the discrepancy via AdjustStock.
		_ = fmt.Errorf("inventory deduction for order %s: %w", payload.OrderID, err)
	}
	return nil
}
// models/inventory_extension.go
//
// Extends the base inventory models with:
//
//   StockMovement         — append-only ledger of every quantity change.
//                           Written inside the same transaction as the
//                           InventoryItem.CurrentQuantity update so the two
//                           are always consistent.
//
//   MenuItemInventoryLink — maps a MenuItem to one or more InventoryItems
//                           with a quantity_per_unit ratio. This is the
//                           table that Order.checkInventoryStock already
//                           queries — we're just finally defining the model.
//
// Table names (GORM snake_case convention):
//   stock_movements
//   menu_item_inventory_links
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────────────────────
// STOCK MOVEMENT
// Append-only ledger. Never update or delete rows here.
// ─────────────────────────────────────────────────────────────────────────────

// StockMovementReason is the reason code for a stock movement.
type StockMovementReason string

const (
	// StockMovementReasonReceived — goods received from a purchase order.
	StockMovementReasonReceived StockMovementReason = "received"
	// StockMovementReasonUsed — consumed by an order (auto-deducted).
	StockMovementReasonUsed StockMovementReason = "used"
	// StockMovementReasonDamaged — written off due to damage.
	StockMovementReasonDamaged StockMovementReason = "damaged"
	// StockMovementReasonExpired — expired stock removed.
	StockMovementReasonExpired StockMovementReason = "expired"
	// StockMovementReasonAdjustment — manual correction by a manager.
	StockMovementReasonAdjustment StockMovementReason = "adjustment"
	// StockMovementReasonCount — physical stock count override.
	StockMovementReasonCount StockMovementReason = "count"
)

// StockMovement records every quantity change for an InventoryItem.
// QuantityDelta is positive for increases (received, count-up) and negative
// for decreases (used, damaged, expired). BalanceAfter is a snapshot of
// CurrentQuantity immediately after the write — denormalised for fast
// audit queries without having to replay the entire ledger.
//
// Table: stock_movements
type StockMovement struct {
	StockMovementID uuid.UUID           `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"stock_movement_id"`
	TenantID        uuid.UUID           `gorm:"type:uuid;not null;index"                        json:"tenant_id"`
	RestaurantID    uuid.UUID           `gorm:"type:uuid;not null;index"                        json:"restaurant_id"`
	InventoryItemID uuid.UUID           `gorm:"type:uuid;not null;index"                        json:"inventory_item_id"`

	// Reason is why the quantity changed.
	Reason StockMovementReason `gorm:"type:varchar(50);not null;index" json:"reason"`

	// QuantityDelta is signed: +3.5 means +3.5 units added, -2.0 means 2 used.
	QuantityDelta float64 `gorm:"type:decimal(12,3);not null" json:"quantity_delta"`

	// BalanceAfter is the InventoryItem.CurrentQuantity snapshot after this move.
	// Written in the same transaction — consistent with the item row.
	BalanceAfter float64 `gorm:"type:decimal(12,3);not null" json:"balance_after"`

	// ReferenceType and ReferenceID link to the triggering entity.
	// Examples: ("purchase_order", "uuid"), ("order", "uuid"), ("manual", "")
	ReferenceType string `gorm:"type:varchar(100)" json:"reference_type,omitempty"`
	ReferenceID   string `gorm:"type:varchar(255)" json:"reference_id,omitempty"`

	Notes     string    `gorm:"type:text"         json:"notes,omitempty"`
	CreatedBy uuid.UUID `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt time.Time `gorm:"not null;default:now();index" json:"created_at"`

	// Relationships
	Tenant        Tenant        `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE"        json:"-"`
	Restaurant    Restaurant    `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE"    json:"-"`
	InventoryItem InventoryItem `gorm:"foreignKey:InventoryItemID;constraint:OnDelete:CASCADE"  json:"-"`
	CreatedByUser User          `gorm:"foreignKey:CreatedBy;constraint:OnDelete:RESTRICT"       json:"-"`
}

func (sm *StockMovement) BeforeCreate(tx *gorm.DB) error {
	if sm.StockMovementID == uuid.Nil {
		sm.StockMovementID = uuid.New()
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// MENU ITEM INVENTORY LINK
// Maps a MenuItem to its raw ingredients / inventory items.
// One MenuItem can consume multiple InventoryItems (e.g. a burger uses
// bun + patty + lettuce); one InventoryItem can be used by many MenuItems.
// ─────────────────────────────────────────────────────────────────────────────

// MenuItemInventoryLink maps a MenuItem to an InventoryItem and specifies
// how many inventory units are consumed per one unit of the menu item sold.
//
// Example: "Beef Burger" → patty (1.0 piece), bun (1.0 piece), lettuce (0.05 kg)
//
// Table: menu_item_inventory_links
// Unique: (menu_item_id, inventory_item_id)
type MenuItemInventoryLink struct {
	LinkID          uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"link_id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null;index"                        json:"tenant_id"`
	MenuItemID      uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_menu_inv_link" json:"menu_item_id"`
	InventoryItemID uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_menu_inv_link" json:"inventory_item_id"`

	// QuantityPerUnit is the amount of InventoryItem consumed when one unit of
	// the MenuItem is sold. Must be > 0.
	QuantityPerUnit float64 `gorm:"type:decimal(12,4);not null" json:"quantity_per_unit"`

	IsActive  bool      `gorm:"not null;default:true"        json:"is_active"`
	CreatedAt time.Time `gorm:"not null;default:now()"       json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:now()"       json:"updated_at"`

	// Relationships
	Tenant        Tenant        `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE"        json:"-"`
	MenuItem      MenuItem      `gorm:"foreignKey:MenuItemID;constraint:OnDelete:CASCADE"       json:"-"`
	InventoryItem InventoryItem `gorm:"foreignKey:InventoryItemID;constraint:OnDelete:CASCADE"  json:"-"`
}

func (l *MenuItemInventoryLink) BeforeCreate(tx *gorm.DB) error {
	if l.LinkID == uuid.Nil {
		l.LinkID = uuid.New()
	}
	return nil
}

// ── Migration SQL ─────────────────────────────────────────────────────────────
/*
CREATE TABLE IF NOT EXISTS stock_movements (
    stock_movement_id   UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id           UUID        NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    restaurant_id       UUID        NOT NULL REFERENCES restaurants(restaurant_id) ON DELETE CASCADE,
    inventory_item_id   UUID        NOT NULL REFERENCES inventory_items(inventory_item_id) ON DELETE CASCADE,
    reason              VARCHAR(50) NOT NULL,
    quantity_delta      DECIMAL(12,3) NOT NULL,
    balance_after       DECIMAL(12,3) NOT NULL,
    reference_type      VARCHAR(100),
    reference_id        VARCHAR(255),
    notes               TEXT,
    created_by          UUID        NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_stock_movements_item
    ON stock_movements(inventory_item_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_stock_movements_tenant
    ON stock_movements(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS menu_item_inventory_links (
    link_id             UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id           UUID        NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    menu_item_id        UUID        NOT NULL REFERENCES menu_items(menu_item_id) ON DELETE CASCADE,
    inventory_item_id   UUID        NOT NULL REFERENCES inventory_items(inventory_item_id) ON DELETE CASCADE,
    quantity_per_unit   DECIMAL(12,4) NOT NULL,
    is_active           BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT idx_menu_inv_link UNIQUE (menu_item_id, inventory_item_id)
);
*/
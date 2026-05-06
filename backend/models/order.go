package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Order represents a customer order.
// A single Order contains one or more OrderItems, each of which may have
// zero or more OrderItemModifiers. TotalAmount is always derived — use
// Order.RecalculateTotal() after mutating items; never set it by hand.
type Order struct {
	OrderID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"order_id"`
	UserID                uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	TenantID              uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	CustomerID            *uuid.UUID `gorm:"type:uuid;index" json:"customer_id"`
	ProcessedByEmployeeID *uuid.UUID `gorm:"type:uuid" json:"processed_by_employee_id"`
	TableID               *int       `gorm:"type:int;index" json:"table_id"`
	BranchID              *uuid.UUID `gorm:"type:uuid" json:"branch_id"`
	APIKeyID              *uuid.UUID `gorm:"type:uuid" json:"api_key_id"`

	SubTotal      float64 `gorm:"type:decimal(10,2)" json:"sub_total"`
    ServiceCharge float64 `gorm:"type:decimal(10,2)" json:"service_charge"`

	// OrderStatus is the single source of truth for order lifecycle.
	// Valid values are defined by the OrderState constants in fsm/fsm.go.
	// REMOVED: Status string — was a duplicate of this field.
	OrderStatus OrderItemStatus `gorm:"type:varchar(50);not null;default:'pending';index" json:"order_status"`

	// OrderType distinguishes dine-in, takeaway, delivery, online.
	OrderType OrderType `gorm:"type:varchar(50);not null;default:'dine-in'" json:"order_type"`

	// TotalAmount is the order-level sum: Σ(item.LineTotal) + Σ(modifier.AdditionalPrice * qty).
	// Always set via RecalculateTotal() — never write this field directly.
	// Tax and discounts live on Invoice (Payment.go) to keep order and billing concerns separate.
	TotalAmount float64 `gorm:"type:decimal(12,2);not null;default:0" json:"total_amount"`

	// Customer contact fields — denormalised for takeaway/delivery orders
	// where CustomerID may be nil (walk-in guests).
	CustomerName    string `gorm:"type:varchar(255)" json:"customer_name"`
	PhoneNumber     string `gorm:"type:varchar(50)" json:"phone_number"`
	DeliveryAddress string `gorm:"type:text" json:"delivery_address"`
	PickupTime      string `gorm:"type:varchar(50)" json:"pickup_time"`

	// Notes is a free-text field for kitchen / service instructions at the order level.
	// Item-level instructions live on OrderItem.Notes.
	Notes string `gorm:"type:text" json:"notes"`

	// Audit trail
	CreatedAt   time.Time  `gorm:"default:now();index" json:"created_at"`
	UpdatedAt   *time.Time `gorm:"default:now()" json:"updated_at"`
	CreatedBy   string     `gorm:"type:varchar(255)" json:"created_by"`
	UpdatedBy   string     `gorm:"type:varchar(255)" json:"updated_by"`
	CreatedByIP string     `gorm:"type:varchar(50)" json:"created_by_ip"`
	UpdatedByIP string     `gorm:"type:varchar(50)" json:"updated_by_ip"`
	IsDeleted   bool       `gorm:"default:false;index" json:"is_deleted"`
	DeletedAt   *time.Time `gorm:"type:timestamptz" json:"deleted_at"`
	DeletedBy   string     `gorm:"type:varchar(255)" json:"deleted_by"`
	DeletedByIP string     `gorm:"type:varchar(50)" json:"deleted_by_ip"`

	// Relationships
	User                User        `gorm:"foreignKey:UserID;constraint:OnDelete:RESTRICT" json:"-"`
	Tenant              Tenant      `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant          Restaurant  `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
	Customer            *Customer   `gorm:"foreignKey:CustomerID;constraint:OnDelete:SET NULL" json:"customer,omitempty"`
	ProcessedByEmployee *Employee   `gorm:"foreignKey:ProcessedByEmployeeID;constraint:OnDelete:SET NULL" json:"processed_by_employee,omitempty"`
	Table               *Table      `gorm:"foreignKey:TableID;constraint:OnDelete:SET NULL" json:"table,omitempty"`
	Branch              *Branch     `gorm:"foreignKey:BranchID;constraint:OnDelete:SET NULL" json:"branch,omitempty"`

	// Groups enables bill-splitting / round-based ordering within a single table order.
	// Load with: db.Preload("Groups.Items.Modifiers").First(&order, id)
	Groups []OrderGroup `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"groups,omitempty"`

	// Items is the has-many collection of line items belonging to this order.
	// Items may or may not belong to a group (GroupID is nullable).
	// Load with: db.Preload("Items.Modifiers").First(&order, id)
	Items []OrderItem `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"items,omitempty"`

	// Logs is the append-only audit trail for this order's lifecycle events.
	Logs []OrderLog `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"logs,omitempty"`
}

// RecalculateTotal sums all item line totals and writes the result into
// TotalAmount. Call this whenever items or modifiers are added or removed
// before persisting the order.
//
//	order.Items = append(order.Items, newItem)
//	order.RecalculateTotal()
//	db.Save(&order)
func (o *Order) RecalculateTotal() {
	var total float64
	for _, item := range o.Items {
		total += item.LineTotal()
	}
	o.TotalAmount = total
}

// FSMState returns the current OrderStatus cast to the fsm.OrderState type.
// Satisfies the fsm.OrderContext interface.
func (o *Order) GetOrderID() string      { return o.OrderID.String() }
func (o *Order) GetTotalAmount() float64 { return o.TotalAmount }
func (o *Order) GetStatus() string       { return string(o.OrderStatus) }
func (o *Order) SetStatus(s string)      { o.OrderStatus = OrderItemStatus(s) }

// OrderGroup represents a sub-group within an order. Enables bill-splitting
// (e.g. "Guest 1", "Guest 2") or round-based ordering ("Round 1", "Round 2").
// Each OrderItem may optionally belong to a group via its GroupID field.
type OrderGroup struct {
	GroupID   uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"group_id"`
	OrderID   uuid.UUID `gorm:"type:uuid;not null;index" json:"order_id"`
	GroupName string    `gorm:"type:varchar(255);not null" json:"group_name"` // e.g. "Guest 1", "Round 2"
	SortOrder int       `gorm:"not null;default:0" json:"sort_order"`
	Notes     string    `gorm:"type:text" json:"notes"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Relationships
	Order Order       `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"-"`
	Items []OrderItem `gorm:"foreignKey:GroupID;constraint:OnDelete:SET NULL" json:"items,omitempty"`
}

// OrderItem is a single line item within an Order.
// One Order contains one or more OrderItems; each OrderItem may have
// zero or more OrderItemModifiers (e.g. "extra cheese", "no onion").
type OrderItem struct {
	OrderItemID uuid.UUID       `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"order_item_id"`
	OrderID     uuid.UUID       `gorm:"type:uuid;not null;index" json:"order_id"`
	GroupID     *uuid.UUID      `gorm:"type:uuid;index" json:"group_id,omitempty"` // optional: assigns item to an OrderGroup
	Status      OrderItemStatus `gorm:"type:varchar(50);not null;default:'pending';index" json:"status"`
	Quantity    int             `gorm:"not null;default:1" json:"quantity"`

	// MenuItemID is a soft reference to the originating MenuItem.
	// Nullable (SET NULL on delete) so historical orders survive menu deletions.
	// ItemName and UnitPrice are snapshotted at order-creation time and must
	// never be updated after the order is placed.
	MenuItemID *uuid.UUID `gorm:"type:uuid;index" json:"menu_item_id"`

	// Snapshotted at order time — immutable after creation.
	ItemName  string  `gorm:"type:varchar(255);not null" json:"item_name"`
	UnitPrice float64 `gorm:"type:decimal(10,2);not null;default:0" json:"unit_price"`

	// Notes holds item-level kitchen instructions ("well done", "nut allergy").
	// Order-level instructions live on Order.Notes.
	Notes string `gorm:"type:text" json:"notes"`

	// Relationships
	Order     Order                `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"-"`
	Group     *OrderGroup          `gorm:"foreignKey:GroupID;constraint:OnDelete:SET NULL" json:"group,omitempty"`
	MenuItem  *MenuItem            `gorm:"foreignKey:MenuItemID;constraint:OnDelete:SET NULL" json:"menu_item,omitempty"`
	Modifiers []OrderItemModifier  `gorm:"foreignKey:OrderItemID;constraint:OnDelete:CASCADE" json:"modifiers,omitempty"`
}

// LineTotal returns the fully computed price for this line item:
//
//	(UnitPrice + Σ modifier.AdditionalPrice) × Quantity
//
// This is the canonical way to get item cost. Never store LineTotal in the DB —
// it is always derived. Order.RecalculateTotal() calls this on every item.
func (oi *OrderItem) LineTotal() float64 {
	modifierSum := 0.0
	for _, m := range oi.Modifiers {
		modifierSum += m.AdditionalPrice
	}
	return (oi.UnitPrice + modifierSum) * float64(oi.Quantity)
}

// OrderItemModifier is a customisation applied to a single OrderItem.
// Examples: "extra shot", "oat milk", "no onion", "large size".
//
// AdditionalPrice is the price delta for this modifier — it may be zero,
// positive (upcharge), or negative (discount modifier). It is snapshotted
// at order time just like OrderItem.UnitPrice.
type OrderItemModifier struct {
	OrderItemModifierID   uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"order_item_modifier_id"`
	OrderItemID           uuid.UUID  `gorm:"type:uuid;not null;index" json:"order_item_id"`

	// MenuItemModifierID is a soft reference to the source MenuItemModifier.
	// Nullable so historical modifiers survive menu changes.
	MenuItemModifierID *uuid.UUID `gorm:"type:uuid;index" json:"menu_item_modifier_id"`

	// Snapshotted at order time — immutable after creation.
	ModifierName    string  `gorm:"type:varchar(255);not null" json:"modifier_name"`
	AdditionalPrice float64 `gorm:"type:decimal(10,2);not null;default:0" json:"additional_price"`

	// Relationships
	OrderItem          OrderItem          `gorm:"foreignKey:OrderItemID;constraint:OnDelete:CASCADE" json:"-"`
	MenuItemModifier   *MenuItemModifier  `gorm:"foreignKey:MenuItemModifierID;constraint:OnDelete:SET NULL" json:"menu_item_modifier,omitempty"`
}

// OrderLog represents a log entry for an order
type OrderLog struct {
	LogID     uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"log_id"`
	OrderID   uuid.UUID `gorm:"type:uuid;not null;index" json:"order_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Action    string    `gorm:"type:varchar(255);not null" json:"action"`
	Details   string    `gorm:"type:text" json:"details"`
	Timestamp time.Time `gorm:"default:now();index" json:"timestamp"`

	// Relationships
	Order Order `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"-"`
	User  User  `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

// BeforeCreate hooks
func (o *Order) BeforeCreate(tx *gorm.DB) error {
	if o.OrderID == uuid.Nil {
		o.OrderID = uuid.New()
	}
	if o.OrderStatus == "" {
		o.OrderStatus = "pending"
	}
	return nil
}

func (og *OrderGroup) BeforeCreate(tx *gorm.DB) error {
	if og.GroupID == uuid.Nil {
		og.GroupID = uuid.New()
	}
	return nil
}

func (oi *OrderItem) BeforeCreate(tx *gorm.DB) error {
	if oi.OrderItemID == uuid.Nil {
		oi.OrderItemID = uuid.New()
	}
	// UnitPrice must be set by the caller before saving.
	// Read it from MenuItem.BasePrice at order creation time, not here.
	if oi.Quantity <= 0 {
		oi.Quantity = 1
	}
	return nil
}

func (oim *OrderItemModifier) BeforeCreate(tx *gorm.DB) error {
	if oim.OrderItemModifierID == uuid.Nil {
		oim.OrderItemModifierID = uuid.New()
	}
	return nil
}

func (ol *OrderLog) BeforeCreate(tx *gorm.DB) error {
	if ol.LogID == uuid.Nil {
		ol.LogID = uuid.New()
	}
	return nil
}
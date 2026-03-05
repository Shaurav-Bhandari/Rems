package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)


type OrderStatus string

const (
	OrderStatusDraft      OrderStatus = "draft"
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusConfirmed  OrderStatus = "confirmed"
	OrderStatusPreparing  OrderStatus = "preparing"
	OrderStatusReady      OrderStatus = "ready"
	OrderStatusDelivered  OrderStatus = "delivered"
	OrderStatusClosed     OrderStatus = "closed"
	OrderStatusCancelled  OrderStatus = "cancelled"
)

// NOTE: If your existing Order struct uses OrderStatus as the field type,
// change the field declaration from:
//     OrderStatus  string      `gorm:"..."`
// to:
//     OrderStatus  OrderStatus `gorm:"type:varchar(50);not null;default:'pending'"`
//
// The enhanced service references models.OrderStatusPending — this constant
// provides that.

// ─────────────────────────────────────────────────────────────────────────────
// ORDER TAX BREAKDOWN
// One row per order. Persisted inside the same transaction as the order row.
// Gives Finance a queryable tax ledger without parsing JSON columns.
// ─────────────────────────────────────────────────────────────────────────────

// OrderTaxBreakdown stores the fully-resolved tax figures for a single order.
// Written once at order creation — never updated (orders are immutable for tax).
//
// Table: order_tax_breakdowns
type OrderTaxBreakdown struct {
	TaxBreakdownID      uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"tax_breakdown_id"`
	OrderID             uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"order_id"` // one-to-one with orders

	// Tax amounts (all in the restaurant's configured currency)
	StandardTaxAmount   float64   `gorm:"type:decimal(12,4);not null;default:0" json:"standard_tax_amount"`
	AlcoholTaxAmount    float64   `gorm:"type:decimal(12,4);not null;default:0" json:"alcohol_tax_amount"`
	TotalTaxAmount      float64   `gorm:"type:decimal(12,4);not null;default:0" json:"total_tax_amount"`
	ServiceChargeAmount float64   `gorm:"type:decimal(12,4);not null;default:0" json:"service_charge_amount"`

	// Context at time of order (snapshot — not live values)
	TaxIncludedInPrice  bool      `gorm:"not null;default:false"              json:"tax_included_in_price"`
	TaxRate             float64   `gorm:"type:decimal(5,4);not null;default:0" json:"tax_rate"` // effective blended rate

	CreatedAt           time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Relationships
	Order Order `gorm:"foreignKey:OrderID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

func (otb *OrderTaxBreakdown) BeforeCreate(tx *gorm.DB) error {
	if otb.TaxBreakdownID == uuid.Nil {
		otb.TaxBreakdownID = uuid.New()
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// OUTBOX EVENT
// Moved from services/order_service.go into the models package so that
// order_service_enhanced.go can reference models.OutboxEvent.
// ─────────────────────────────────────────────────────────────────────────────

// OutboxStatus constants for the transactional outbox table.
const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusDelivered  = "delivered"
	OutboxStatusFailed     = "failed"
)

// OutboxEvent is the transactional outbox entry.
// Written inside the same DB transaction as the order — never committed separately.
// The relay worker (order_service_enhanced.go Layer 4) reads and fans out to notifiers.
//
// Table: outbox_events
type OutboxEvent struct {
	OutboxEventID uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"outbox_event_id"`
	TenantID      uuid.UUID  `gorm:"type:uuid;not null;index"                        json:"tenant_id"`
	AggregateType string     `gorm:"type:varchar(100);not null;index"                json:"aggregate_type"` // "order"
	AggregateID   string     `gorm:"type:varchar(255);not null;index"                json:"aggregate_id"`   // order UUID as string
	EventType     string     `gorm:"type:varchar(100);not null;index"                json:"event_type"`     // "OrderCreated"
	Payload       []byte     `gorm:"type:jsonb;not null"                             json:"payload"`
	Status        string     `gorm:"type:varchar(50);not null;default:'pending';index" json:"status"`
	Attempts      int        `gorm:"not null;default:0"                              json:"attempts"`
	LastAttemptAt *time.Time `gorm:"type:timestamptz"                                json:"last_attempt_at"`
	NextRetryAt   time.Time  `gorm:"type:timestamptz;not null;default:now();index"   json:"next_retry_at"`
	CreatedAt     time.Time  `gorm:"not null;default:now();index"                    json:"created_at"`
	ProcessedAt   *time.Time `gorm:"type:timestamptz"                                json:"processed_at"`
	Error         string     `gorm:"type:text"                                       json:"error"`
}

func (oe *OutboxEvent) BeforeCreate(tx *gorm.DB) error {
	if oe.OutboxEventID == uuid.Nil {
		oe.OutboxEventID = uuid.New()
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// ORDER CREATED PAYLOAD
// Moved from services/order_service.go into models so that both the service
// that writes it and the notifiers that read it share the same type.
// ─────────────────────────────────────────────────────────────────────────────

// OrderCreatedPayload is the JSON body of an "OrderCreated" outbox event.
// All downstream consumers (KOT, Inventory, Analytics) deserialise this.
type OrderCreatedPayload struct {
	OrderID       uuid.UUID                 `json:"order_id"`
	TenantID      uuid.UUID                 `json:"tenant_id"`
	RestaurantID  uuid.UUID                 `json:"restaurant_id"`
	TableID       *int                      `json:"table_id"`
	OrderType     string                    `json:"order_type"`
	KOTNumber     string                    `json:"kot_number"`
	ActorID       uuid.UUID                 `json:"actor_id"`
	OccurredAt    time.Time                 `json:"occurred_at"`
	Items         []OrderCreatedItemPayload `json:"items"`
}

// OrderCreatedItemPayload is the per-item slice of the outbox payload.
type OrderCreatedItemPayload struct {
	OrderItemID    uuid.UUID  `json:"order_item_id"`
	MenuItemID     *uuid.UUID `json:"menu_item_id"`
	ItemName       string     `json:"item_name"`
	Quantity       int        `json:"quantity"`
	UnitPrice      float64    `json:"unit_price"`
	KitchenStation string     `json:"kitchen_station"`
}

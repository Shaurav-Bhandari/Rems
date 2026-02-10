package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Order represents a customer order
type Order struct {
	OrderID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"order_id"`
	UserID                uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	TenantID              uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	CustomerID            *uuid.UUID `gorm:"type:uuid;index" json:"customer_id"`
	ProcessedByEmployeeID *uuid.UUID `gorm:"type:uuid" json:"processed_by_employee_id"`
	TableID               *int       `gorm:"type:int;index" json:"table_id"`
	OrderStatus           string     `gorm:"type:varchar(50);index" json:"order_status"`
	TotalAmount           float64    `gorm:"type:decimal(12,2);not null" json:"total_amount"`
	CustomerName          string     `gorm:"type:varchar(255)" json:"customer_name"`
	PhoneNumber           string     `gorm:"type:varchar(50)" json:"phone_number"`
	PickupTime            string     `gorm:"type:varchar(50)" json:"pickup_time"`
	DeliveryAddress       string     `gorm:"type:text" json:"delivery_address"`
	CreatedAt             time.Time  `gorm:"default:now();index" json:"created_at"`
	UpdatedAt             *time.Time `gorm:"default:now()" json:"updated_at"`
	CreatedBy             string     `gorm:"type:varchar(255)" json:"created_by"`
	UpdatedBy             string     `gorm:"type:varchar(255)" json:"updated_by"`
	CreatedByIP           string     `gorm:"type:varchar(50)" json:"created_by_ip"`
	UpdatedByIP           string     `gorm:"type:varchar(50)" json:"updated_by_ip"`
	IsDeleted             bool       `gorm:"default:false;index" json:"is_deleted"`
	DeletedAt             *time.Time `gorm:"type:timestamptz" json:"deleted_at"`
	DeletedBy             string     `gorm:"type:varchar(255)" json:"deleted_by"`
	DeletedByIP           string     `gorm:"type:varchar(50)" json:"deleted_by_ip"`
	Version               int        `gorm:"default:1" json:"version"`
	IsActive              bool       `gorm:"default:true" json:"is_active"`
	Status                string     `gorm:"type:varchar(50)" json:"status"`
	Environment           string     `gorm:"type:varchar(50)" json:"environment"`
	ChangeHistory         JSONB      `gorm:"type:jsonb;default:'[]'" json:"change_history"`
	APIKeyID              *uuid.UUID `gorm:"type:uuid" json:"api_key_id"`
	BranchID              *uuid.UUID `gorm:"type:uuid" json:"branch_id"`

	// Relationships
	User                User       `gorm:"foreignKey:UserID;constraint:OnDelete:RESTRICT" json:"-"`
	Tenant              Tenant     `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant          Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
	Customer            *Customer  `gorm:"foreignKey:CustomerID;constraint:OnDelete:SET NULL" json:"customer,omitempty"`
	ProcessedByEmployee *Employee  `gorm:"foreignKey:ProcessedByEmployeeID;constraint:OnDelete:SET NULL" json:"processed_by_employee,omitempty"`
	Table               *Table     `gorm:"foreignKey:TableID;constraint:OnDelete:SET NULL" json:"table,omitempty"`
	Branch              *Branch    `gorm:"foreignKey:BranchID;constraint:OnDelete:SET NULL" json:"branch,omitempty"`
}

// OrderItem represents an item in an order
type OrderItem struct {
	OrderItemID uuid.UUID       `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"order_item_id"`
	OrderID     uuid.UUID       `gorm:"type:uuid;not null;index" json:"order_id"`
	ItemName    string          `gorm:"type:varchar(255);not null" json:"item_name"`
	Quantity    int             `gorm:"default:1" json:"quantity"`
	Status      OrderItemStatus `gorm:"type:varchar(50);default:'Pending';index" json:"status"`

	// Relationships
	Order Order `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"-"`
}

// OrderItemModifier represents a modifier for an order item
type OrderItemModifier struct {
	OrderItemModifierID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"order_item_modifier_id"`
	OrderItemID         uuid.UUID `gorm:"type:uuid;not null;index" json:"order_item_id"`
	ModifierName        string    `gorm:"type:varchar(255);not null" json:"modifier_name"`
	AdditionalPrice     float64   `gorm:"type:decimal(10,2);default:0" json:"additional_price"`

	// Relationships
	OrderItem OrderItem `gorm:"foreignKey:OrderItemID;constraint:OnDelete:CASCADE" json:"-"`
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
	return nil
}

func (oi *OrderItem) BeforeCreate(tx *gorm.DB) error {
	if oi.OrderItemID == uuid.Nil {
		oi.OrderItemID = uuid.New()
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
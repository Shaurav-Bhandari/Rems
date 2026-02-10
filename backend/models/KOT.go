package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// KOT represents a Kitchen Order Ticket
type KOT struct {
	KOTID            uuid.UUID    `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"kot_id"`
	OrderID          uuid.UUID    `gorm:"type:uuid;not null;index" json:"order_id"`
	RestaurantID     uuid.UUID    `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	TenantID         uuid.UUID    `gorm:"type:uuid;not null;index;uniqueIndex:idx_tenant_kot_number" json:"tenant_id"`
	KOTNumber        string       `gorm:"type:varchar(50);not null;uniqueIndex:idx_tenant_kot_number" json:"kot_number"`
	SequenceNumber   int          `gorm:"not null" json:"sequence_number"`
	OrderNumber      string       `gorm:"type:varchar(50)" json:"order_number"`
	TableNumber      *int         `gorm:"type:int" json:"table_number"`
	CustomerName     string       `gorm:"type:varchar(255)" json:"customer_name"`
	OrderType        OrderType    `gorm:"type:varchar(50)" json:"order_type"`
	GuestCount       *int         `gorm:"type:int" json:"guest_count"`
	Status           KOTStatus    `gorm:"type:varchar(50);default:'Sent';index" json:"status"`
	Priority         KOTPriority  `gorm:"type:varchar(50);default:'Normal'" json:"priority"`
	CreatedByUserID  uuid.UUID    `gorm:"type:uuid;not null" json:"created_by_user_id"`
	CreatedByName    string       `gorm:"type:varchar(255)" json:"created_by_name"`
	AssignedToChefID *uuid.UUID   `gorm:"type:uuid" json:"assigned_to_chef_id"`
	PrintCount       int          `gorm:"default:0" json:"print_count"`
	LastPrintedAt    *time.Time   `gorm:"type:timestamptz" json:"last_printed_at"`
	CreatedAt        time.Time    `gorm:"default:now();index" json:"created_at"`

	// Relationships
	Order            Order      `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant       Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
	Tenant           Tenant     `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	CreatedByUser    User       `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:RESTRICT" json:"-"`
	AssignedToChef   *Employee  `gorm:"foreignKey:AssignedToChefID;constraint:OnDelete:SET NULL" json:"assigned_to_chef,omitempty"`
}

// KOTItem represents an item in a Kitchen Order Ticket
type KOTItem struct {
	KOTItemID       uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"kot_item_id"`
	KOTID           uuid.UUID      `gorm:"type:uuid;not null;index" json:"kot_id"`
	OrderItemID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"order_item_id"`
	ItemName        string         `gorm:"type:varchar(255);not null" json:"item_name"`
	Quantity        int            `gorm:"default:1" json:"quantity"`
	Notes           string         `gorm:"type:text" json:"notes"`
	AssignedStation KitchenStation `gorm:"type:varchar(50);default:'General';index" json:"assigned_station"`
	Status          KOTItemStatus  `gorm:"type:varchar(50);default:'Pending';index" json:"status"`

	// Relationships
	KOT       KOT       `gorm:"foreignKey:KOTID;constraint:OnDelete:CASCADE" json:"-"`
	OrderItem OrderItem `gorm:"foreignKey:OrderItemID;constraint:OnDelete:CASCADE" json:"-"`
}

// BeforeCreate hooks
func (k *KOT) BeforeCreate(tx *gorm.DB) error {
	if k.KOTID == uuid.Nil {
		k.KOTID = uuid.New()
	}
	return nil
}

func (ki *KOTItem) BeforeCreate(tx *gorm.DB) error {
	if ki.KOTItemID == uuid.Nil {
		ki.KOTItemID = uuid.New()
	}
	return nil
}
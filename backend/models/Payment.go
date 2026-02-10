package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PaymentRecord represents a payment transaction
type PaymentRecord struct {
	PaymentRecordID uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"payment_record_id"`
	TenantID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	OrderID         *uuid.UUID `gorm:"type:uuid;index" json:"order_id"`
	Amount          float64    `gorm:"type:decimal(12,2);not null" json:"amount"`
	PaymentMethod   string     `gorm:"type:varchar(50)" json:"payment_method"`
	TransactionID   string     `gorm:"type:varchar(255)" json:"transaction_id"`
	Status          string     `gorm:"type:varchar(50);index" json:"status"`
	PaymentDate     time.Time  `gorm:"not null;index" json:"payment_date"`
	CreatedAt       time.Time  `gorm:"not null;default:now()" json:"created_at"`

	// Relationships
	Tenant     Tenant      `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant Restaurant  `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
	Order      *Order      `gorm:"foreignKey:OrderID;constraint:OnDelete:SET NULL" json:"order,omitempty"`
}

// Invoice represents an invoice
type Invoice struct {
	InvoiceID      uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"invoice_id"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_tenant_invoice_number" json:"tenant_id"`
	RestaurantID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	OrderID        *uuid.UUID `gorm:"type:uuid;index" json:"order_id"`
	InvoiceNumber  string     `gorm:"type:varchar(100);not null;uniqueIndex:idx_tenant_invoice_number" json:"invoice_number"`
	InvoiceDate    time.Time  `gorm:"not null;index" json:"invoice_date"`
	DueDate        *time.Time `gorm:"type:timestamptz" json:"due_date"`
	Subtotal       float64    `gorm:"type:decimal(12,2);not null" json:"subtotal"`
	TaxAmount      float64    `gorm:"type:decimal(12,2);not null" json:"tax_amount"`
	DiscountAmount float64    `gorm:"type:decimal(12,2);default:0" json:"discount_amount"`
	TotalAmount    float64    `gorm:"type:decimal(12,2);not null" json:"total_amount"`
	Status         string     `gorm:"type:varchar(50);index" json:"status"`
	CreatedAt      time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"not null;default:now()" json:"updated_at"`

	// Relationships
	Tenant     Tenant      `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant Restaurant  `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
	Order      *Order      `gorm:"foreignKey:OrderID;constraint:OnDelete:SET NULL" json:"order,omitempty"`
}

// BeforeCreate hooks
func (pr *PaymentRecord) BeforeCreate(tx *gorm.DB) error {
	if pr.PaymentRecordID == uuid.Nil {
		pr.PaymentRecordID = uuid.New()
	}
	return nil
}

func (i *Invoice) BeforeCreate(tx *gorm.DB) error {
	if i.InvoiceID == uuid.Nil {
		i.InvoiceID = uuid.New()
	}
	return nil
}
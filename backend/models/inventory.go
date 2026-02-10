package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Vendor represents a supplier/vendor
type Vendor struct {
	VendorID     uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"vendor_id"`
	TenantID     uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name         string    `gorm:"type:varchar(255);not null" json:"name"`
	ContactName  string    `gorm:"type:varchar(255)" json:"contact_name"`
	Email        string    `gorm:"type:varchar(255)" json:"email"`
	Phone        string    `gorm:"type:varchar(50)" json:"phone"`
	Address      string    `gorm:"type:text" json:"address"`
	PaymentTerms string    `gorm:"type:text" json:"payment_terms"`
	IsActive     bool      `gorm:"not null;default:true;index" json:"is_active"`
	CreatedAt    time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt    time.Time `gorm:"not null;default:now()" json:"updated_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// InventoryItem represents an inventory item
type InventoryItem struct {
	InventoryItemID  uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"inventory_item_id"`
	TenantID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	Name             string     `gorm:"type:varchar(255);not null" json:"name"`
	Description      string     `gorm:"type:text" json:"description"`
	SKU              string     `gorm:"type:varchar(100);index" json:"sku"`
	Category         string     `gorm:"type:varchar(100);index" json:"category"`
	UnitOfMeasure    string     `gorm:"type:varchar(50)" json:"unit_of_measure"`
	CurrentQuantity  float64    `gorm:"type:decimal(12,3);not null;default:0" json:"current_quantity"`
	MinimumQuantity  *float64   `gorm:"type:decimal(12,3)" json:"minimum_quantity"`
	MaximumQuantity  *float64   `gorm:"type:decimal(12,3)" json:"maximum_quantity"`
	ReorderPoint     *float64   `gorm:"type:decimal(12,3)" json:"reorder_point"`
	UnitCost         *float64   `gorm:"type:decimal(10,2)" json:"unit_cost"`
	LastRestockDate  *time.Time `gorm:"type:timestamptz" json:"last_restock_date"`
	CreatedAt        time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"not null;default:now()" json:"updated_at"`

	// Relationships
	Tenant     Tenant     `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
}

// PurchaseOrder represents a purchase order
type PurchaseOrder struct {
	PurchaseOrderID      uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"purchase_order_id"`
	TenantID             uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_tenant_po_number" json:"tenant_id"`
	RestaurantID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	VendorID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"vendor_id"`
	OrderNumber          string     `gorm:"type:varchar(100);not null;uniqueIndex:idx_tenant_po_number" json:"order_number"`
	OrderDate            time.Time  `gorm:"not null" json:"order_date"`
	ExpectedDeliveryDate *time.Time `gorm:"type:timestamptz" json:"expected_delivery_date"`
	ActualDeliveryDate   *time.Time `gorm:"type:timestamptz" json:"actual_delivery_date"`
	Status               string     `gorm:"type:varchar(50);index" json:"status"`
	TotalAmount          float64    `gorm:"type:decimal(12,2);not null" json:"total_amount"`
	CreatedBy            uuid.UUID  `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt            time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt            time.Time  `gorm:"not null;default:now()" json:"updated_at"`

	// Relationships
	Tenant       Tenant     `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant   Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
	Vendor       Vendor     `gorm:"foreignKey:VendorID;constraint:OnDelete:RESTRICT" json:"-"`
	CreatedByUser User      `gorm:"foreignKey:CreatedBy;constraint:OnDelete:RESTRICT" json:"-"`
}

// PurchaseOrderLine represents a line item in a purchase order
type PurchaseOrderLine struct {
	PurchaseOrderLineID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"purchase_order_line_id"`
	PurchaseOrderID     uuid.UUID `gorm:"type:uuid;not null;index" json:"purchase_order_id"`
	InventoryItemID     uuid.UUID `gorm:"type:uuid;not null;index" json:"inventory_item_id"`
	Quantity            float64   `gorm:"type:decimal(12,3);not null" json:"quantity"`
	UnitPrice           float64   `gorm:"type:decimal(10,2);not null" json:"unit_price"`
	LineTotal           float64   `gorm:"type:decimal(12,2);not null" json:"line_total"`
	ReceivedQuantity    float64   `gorm:"type:decimal(12,3);default:0" json:"received_quantity"`

	// Relationships
	PurchaseOrder PurchaseOrder `gorm:"foreignKey:PurchaseOrderID;constraint:OnDelete:CASCADE" json:"-"`
	InventoryItem InventoryItem `gorm:"foreignKey:InventoryItemID;constraint:OnDelete:RESTRICT" json:"-"`
}

// BeforeCreate hooks
func (v *Vendor) BeforeCreate(tx *gorm.DB) error {
	if v.VendorID == uuid.Nil {
		v.VendorID = uuid.New()
	}
	return nil
}

func (ii *InventoryItem) BeforeCreate(tx *gorm.DB) error {
	if ii.InventoryItemID == uuid.Nil {
		ii.InventoryItemID = uuid.New()
	}
	return nil
}

func (po *PurchaseOrder) BeforeCreate(tx *gorm.DB) error {
	if po.PurchaseOrderID == uuid.Nil {
		po.PurchaseOrderID = uuid.New()
	}
	return nil
}

func (pol *PurchaseOrderLine) BeforeCreate(tx *gorm.DB) error {
	if pol.PurchaseOrderLineID == uuid.Nil {
		pol.PurchaseOrderLineID = uuid.New()
	}
	return nil
}
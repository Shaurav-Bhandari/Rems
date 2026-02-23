package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ===================================
// INVENTORY ITEM DTOs
// ===================================

// CreateInventoryItemRequest
type CreateInventoryItemRequest struct {
	RestaurantID    uuid.UUID `json:"restaurant_id" binding:"required"`
	Name            string    `json:"name" binding:"required,min=2,max=255"`
	Description     string    `json:"description" binding:"omitempty,max=1000"`
	SKU             string    `json:"sku" binding:"omitempty,max=100"`
	Category        string    `json:"category" binding:"omitempty,max=100"`
	UnitOfMeasure   string    `json:"unit_of_measure" binding:"required,max=50"` // kg, lbs, units, etc
	CurrentQuantity float64   `json:"current_quantity" binding:"required,min=0"`
	MinimumQuantity *float64  `json:"minimum_quantity" binding:"omitempty,min=0"`
	MaximumQuantity *float64  `json:"maximum_quantity" binding:"omitempty,min=0"`
	ReorderPoint    *float64  `json:"reorder_point" binding:"omitempty,min=0"`
	UnitCost        *float64  `json:"unit_cost" binding:"omitempty,min=0"`
}

// Validate validates CreateInventoryItemRequest
func (r *CreateInventoryItemRequest) Validate() error {
	if r.MinimumQuantity != nil && r.MaximumQuantity != nil {
		if *r.MinimumQuantity > *r.MaximumQuantity {
			return ErrMinQuantityGreaterThanMax
		}
	}
	if r.ReorderPoint != nil && r.MinimumQuantity != nil {
		if *r.ReorderPoint < *r.MinimumQuantity {
			return ErrReorderPointBelowMinimum
		}
	}
	return nil
}

// UpdateInventoryItemRequest
type UpdateInventoryItemRequest struct {
	Name            *string  `json:"name" binding:"omitempty,min=2,max=255"`
	Description     *string  `json:"description" binding:"omitempty,max=1000"`
	Category        *string  `json:"category" binding:"omitempty,max=100"`
	CurrentQuantity *float64 `json:"current_quantity" binding:"omitempty,min=0"`
	MinimumQuantity *float64 `json:"minimum_quantity" binding:"omitempty,min=0"`
	ReorderPoint    *float64 `json:"reorder_point" binding:"omitempty,min=0"`
	UnitCost        *float64 `json:"unit_cost" binding:"omitempty,min=0"`
}

// AdjustInventoryRequest - For stock adjustments
type AdjustInventoryRequest struct {
	Quantity    float64    `json:"quantity" binding:"required"`       // Can be negative
	Reason      string     `json:"reason" binding:"required,oneof=received used damaged expired adjustment count"`
	Notes       string     `json:"notes" binding:"omitempty,max=500"`
	ReferenceID *uuid.UUID `json:"reference_id,omitempty"` // Link to PO, waste log, etc
}

// Validate validates AdjustInventoryRequest
func (r *AdjustInventoryRequest) Validate() error {
	if r.Quantity == 0 {
		return ErrZeroQuantityAdjustment
	}
	return nil
}

// InventoryItemResponse
type InventoryItemResponse struct {
	InventoryItemID uuid.UUID  `json:"inventory_item_id"`
	TenantID        uuid.UUID  `json:"tenant_id"`
	RestaurantID    uuid.UUID  `json:"restaurant_id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	SKU             string     `json:"sku"`
	Category        string     `json:"category"`
	UnitOfMeasure   string     `json:"unit_of_measure"`
	CurrentQuantity float64    `json:"current_quantity"`
	MinimumQuantity *float64   `json:"minimum_quantity,omitempty"`
	MaximumQuantity *float64   `json:"maximum_quantity,omitempty"`
	ReorderPoint    *float64   `json:"reorder_point,omitempty"`
	UnitCost        *float64   `json:"unit_cost,omitempty"`
	StockValue      float64    `json:"stock_value"`      // CurrentQuantity * UnitCost
	NeedsReorder    bool       `json:"needs_reorder"`    // CurrentQuantity <= ReorderPoint
	StockStatus     string     `json:"stock_status"`     // in_stock, low_stock, out_of_stock
	LastRestockDate *time.Time `json:"last_restock_date,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// InventoryListResponse
type InventoryListResponse struct {
	Items      []InventoryItemResponse `json:"items"`
	Total      int64                   `json:"total"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"page_size"`
	TotalPages int                     `json:"total_pages"`
}

// InventoryFilterRequest
type InventoryFilterRequest struct {
	RestaurantID *uuid.UUID `form:"restaurant_id"`
	Category     *string    `form:"category"`
	SKU          *string    `form:"sku"`
	Search       *string    `form:"search"` // Search name/description
	StockStatus  *string    `form:"stock_status" binding:"omitempty,oneof=in_stock low_stock out_of_stock"`
	NeedsReorder *bool      `form:"needs_reorder"`
	Page         int        `form:"page" binding:"min=1"`
	PageSize     int        `form:"page_size" binding:"min=1,max=100"`
	SortBy       string     `form:"sort_by" binding:"omitempty,oneof=name sku current_quantity unit_cost created_at"`
	SortOrder    string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// InventoryStatsResponse
type InventoryStatsResponse struct {
	TotalItems      int     `json:"total_items"`
	TotalValue      float64 `json:"total_value"`
	LowStockCount   int     `json:"low_stock_count"`
	OutOfStockCount int     `json:"out_of_stock_count"`
	ReorderRequired int     `json:"reorder_required"`
}

// ===================================
// VENDOR DTOs
// ===================================

// CreateVendorRequest
type CreateVendorRequest struct {
	Name         string `json:"name" binding:"required,min=2,max=255"`
	ContactName  string `json:"contact_name" binding:"omitempty,max=255"`
	Email        string `json:"email" binding:"omitempty,email,max=255"`
	Phone        string `json:"phone" binding:"required,max=50"`
	Address      string `json:"address" binding:"omitempty,max=1000"`
	PaymentTerms string `json:"payment_terms" binding:"omitempty,max=500"`
}

// UpdateVendorRequest
type UpdateVendorRequest struct {
	Name         *string `json:"name" binding:"omitempty,min=2,max=255"`
	ContactName  *string `json:"contact_name" binding:"omitempty,max=255"`
	Email        *string `json:"email" binding:"omitempty,email,max=255"`
	Phone        *string `json:"phone" binding:"omitempty,max=50"`
	Address      *string `json:"address" binding:"omitempty,max=1000"`
	PaymentTerms *string `json:"payment_terms" binding:"omitempty,max=500"`
	IsActive     *bool   `json:"is_active"`
}

// VendorResponse
type VendorResponse struct {
	VendorID     uuid.UUID `json:"vendor_id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	Name         string    `json:"name"`
	ContactName  string    `json:"contact_name"`
	Email        string    `json:"email"`
	Phone        string    `json:"phone"`
	Address      string    `json:"address"`
	PaymentTerms string    `json:"payment_terms"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// VendorListResponse
type VendorListResponse struct {
	Vendors    []VendorResponse `json:"vendors"`
	Total      int64            `json:"total"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}

// VendorSummaryDTO
type VendorSummaryDTO struct {
	VendorID uuid.UUID `json:"vendor_id"`
	Name     string    `json:"name"`
	Phone    string    `json:"phone"`
}

// InventoryItemSummaryDTO
type InventoryItemSummaryDTO struct {
	InventoryItemID uuid.UUID `json:"inventory_item_id"`
	Name            string    `json:"name"`
	SKU             string    `json:"sku"`
	UnitOfMeasure   string    `json:"unit_of_measure"`
}

// ===================================
// VALIDATION ERRORS
// ===================================

var (
	ErrMinQuantityGreaterThanMax = NewValidationError("minimum quantity cannot be greater than maximum quantity")
	ErrReorderPointBelowMinimum  = NewValidationError("reorder point should be at or above minimum quantity")
	ErrZeroQuantityAdjustment    = NewValidationError("quantity adjustment cannot be zero")
)
package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ===================================
// PURCHASE ORDER DTOs
// ===================================

// CreatePurchaseOrderRequest
type CreatePurchaseOrderRequest struct {
	RestaurantID         uuid.UUID                    `json:"restaurant_id" binding:"required"`
	VendorID             uuid.UUID                    `json:"vendor_id" binding:"required"`
	OrderDate            time.Time                    `json:"order_date" binding:"required"`
	ExpectedDeliveryDate *time.Time                   `json:"expected_delivery_date,omitempty"`
	Lines                []CreatePurchaseOrderLineDTO `json:"lines" binding:"required,min=1,dive"`
	Notes                string                       `json:"notes" binding:"omitempty,max=500"`
}

// Validate validates CreatePurchaseOrderRequest
func (r *CreatePurchaseOrderRequest) Validate() error {
	if len(r.Lines) == 0 {
		return ErrNoPurchaseOrderLines
	}

	// Validate each line
	for i, line := range r.Lines {
		if err := line.Validate(); err != nil {
			return NewValidationError("line " + string(rune(i+1)) + ": " + err.Error())
		}
	}

	// Check for duplicate inventory items
	seen := make(map[uuid.UUID]bool)
	for _, line := range r.Lines {
		if seen[line.InventoryItemID] {
			return ErrDuplicateInventoryItems
		}
		seen[line.InventoryItemID] = true
	}

	return nil
}

// CreatePurchaseOrderLineDTO
type CreatePurchaseOrderLineDTO struct {
	InventoryItemID uuid.UUID `json:"inventory_item_id" binding:"required"`
	Quantity        float64   `json:"quantity" binding:"required,min=0.001"`
	UnitPrice       float64   `json:"unit_price" binding:"required,min=0"`
}

// Validate validates CreatePurchaseOrderLineDTO
func (r *CreatePurchaseOrderLineDTO) Validate() error {
	if r.Quantity <= 0 {
		return ErrInvalidQuantity
	}
	if r.UnitPrice < 0 {
		return ErrInvalidUnitPrice
	}
	return nil
}

// UpdatePurchaseOrderRequest
type UpdatePurchaseOrderRequest struct {
	ExpectedDeliveryDate *time.Time `json:"expected_delivery_date,omitempty"`
	ActualDeliveryDate   *time.Time `json:"actual_delivery_date,omitempty"`
	Status               *string    `json:"status" binding:"omitempty,oneof=draft pending approved received cancelled"`
}

// ReceivePurchaseOrderRequest
type ReceivePurchaseOrderRequest struct {
	Lines []ReceivePurchaseOrderLineDTO `json:"lines" binding:"required,min=1,dive"`
	Notes string                         `json:"notes" binding:"omitempty,max=500"`
}

// Validate validates ReceivePurchaseOrderRequest
func (r *ReceivePurchaseOrderRequest) Validate() error {
	if len(r.Lines) == 0 {
		return ErrNoReceivingLines
	}

	for i, line := range r.Lines {
		if err := line.Validate(); err != nil {
			return NewValidationError("line " + string(rune(i+1)) + ": " + err.Error())
		}
	}

	return nil
}

// ReceivePurchaseOrderLineDTO
type ReceivePurchaseOrderLineDTO struct {
	PurchaseOrderLineID uuid.UUID `json:"purchase_order_line_id" binding:"required"`
	ReceivedQuantity    float64   `json:"received_quantity" binding:"required,min=0"`
}

// Validate validates ReceivePurchaseOrderLineDTO
func (r *ReceivePurchaseOrderLineDTO) Validate() error {
	if r.ReceivedQuantity < 0 {
		return ErrNegativeReceivedQuantity
	}
	return nil
}

// PurchaseOrderResponse
type PurchaseOrderResponse struct {
	PurchaseOrderID      uuid.UUID                   `json:"purchase_order_id"`
	TenantID             uuid.UUID                   `json:"tenant_id"`
	RestaurantID         uuid.UUID                   `json:"restaurant_id"`
	VendorID             uuid.UUID                   `json:"vendor_id"`
	OrderNumber          string                      `json:"order_number"`
	OrderDate            time.Time                   `json:"order_date"`
	ExpectedDeliveryDate *time.Time                  `json:"expected_delivery_date,omitempty"`
	ActualDeliveryDate   *time.Time                  `json:"actual_delivery_date,omitempty"`
	Status               string                      `json:"status"`
	TotalAmount          float64                     `json:"total_amount"`
	CreatedBy            uuid.UUID                   `json:"created_by"`
	CreatedAt            time.Time                   `json:"created_at"`
	UpdatedAt            time.Time                   `json:"updated_at"`
	Vendor               *VendorSummaryDTO           `json:"vendor,omitempty"`
	Lines                []PurchaseOrderLineResponse `json:"lines,omitempty"`
}

// PurchaseOrderLineResponse
type PurchaseOrderLineResponse struct {
	PurchaseOrderLineID uuid.UUID                `json:"purchase_order_line_id"`
	PurchaseOrderID     uuid.UUID                `json:"purchase_order_id"`
	InventoryItemID     uuid.UUID                `json:"inventory_item_id"`
	Quantity            float64                  `json:"quantity"`
	UnitPrice           float64                  `json:"unit_price"`
	LineTotal           float64                  `json:"line_total"`
	ReceivedQuantity    float64                  `json:"received_quantity"`
	PendingQuantity     float64                  `json:"pending_quantity"` // Quantity - ReceivedQuantity
	InventoryItem       *InventoryItemSummaryDTO `json:"inventory_item,omitempty"`
}

// PurchaseOrderListResponse
type PurchaseOrderListResponse struct {
	PurchaseOrders []PurchaseOrderResponse `json:"purchase_orders"`
	Total          int64                   `json:"total"`
	Page           int                     `json:"page"`
	PageSize       int                     `json:"page_size"`
	TotalPages     int                     `json:"total_pages"`
}

// PurchaseOrderFilterRequest
type PurchaseOrderFilterRequest struct {
	RestaurantID *uuid.UUID `form:"restaurant_id"`
	VendorID     *uuid.UUID `form:"vendor_id"`
	Status       *string    `form:"status" binding:"omitempty,oneof=draft pending approved received cancelled"`
	DateFrom     *time.Time `form:"date_from" time_format:"2006-01-02"`
	DateTo       *time.Time `form:"date_to" time_format:"2006-01-02"`
	Page         int        `form:"page" binding:"min=1"`
	PageSize     int        `form:"page_size" binding:"min=1,max=100"`
	SortBy       string     `form:"sort_by" binding:"omitempty,oneof=order_date total_amount status"`
	SortOrder    string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// PurchaseOrderSummaryDTO
type PurchaseOrderSummaryDTO struct {
	PurchaseOrderID uuid.UUID `json:"purchase_order_id"`
	OrderNumber     string    `json:"order_number"`
	VendorName      string    `json:"vendor_name"`
	OrderDate       time.Time `json:"order_date"`
	Status          string    `json:"status"`
	TotalAmount     float64   `json:"total_amount"`
}

// ===================================
// VALIDATION ERRORS
// ===================================

var (
	ErrNoPurchaseOrderLines      = NewValidationError("purchase order must have at least one line item")
	ErrDuplicateInventoryItems   = NewValidationError("duplicate inventory items in purchase order")
	// ErrInvalidQuantity           = NewValidationError("quantity must be greater than 0")
	ErrInvalidUnitPrice          = NewValidationError("unit price cannot be negative")
	ErrNoReceivingLines          = NewValidationError("receiving must have at least one line item")
	ErrNegativeReceivedQuantity  = NewValidationError("received quantity cannot be negative")
)
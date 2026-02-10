package DTO

import (
	"time"

	"github.com/google/uuid"
)

// CreateOrderRequest represents the request to create a new order
type CreateOrderRequest struct {
	RestaurantID    uuid.UUID              `json:"restaurant_id" binding:"required"`
	CustomerID      *uuid.UUID             `json:"customer_id,omitempty"`
	TableID         *int                   `json:"table_id,omitempty"`
	BranchID        *uuid.UUID             `json:"branch_id,omitempty"`
	CustomerName    string                 `json:"customer_name,omitempty" binding:"max=255"`
	PhoneNumber     string                 `json:"phone_number,omitempty" binding:"max=50"`
	PickupTime      string                 `json:"pickup_time,omitempty" binding:"max=50"`
	DeliveryAddress string                 `json:"delivery_address,omitempty"`
	OrderItems      []CreateOrderItemDTO   `json:"order_items" binding:"required,min=1,dive"`
}

// CreateOrderItemDTO represents an order item in the create request
type CreateOrderItemDTO struct {
	ItemName  string                      `json:"item_name" binding:"required,max=255"`
	Quantity  int                         `json:"quantity" binding:"required,min=1"`
	Modifiers []CreateOrderItemModifierDTO `json:"modifiers,omitempty" binding:"dive"`
}

// CreateOrderItemModifierDTO represents a modifier in the create request
type CreateOrderItemModifierDTO struct {
	ModifierName    string  `json:"modifier_name" binding:"required,max=255"`
	AdditionalPrice float64 `json:"additional_price" binding:"min=0"`
}

// UpdateOrderRequest represents the request to update an order
type UpdateOrderRequest struct {
	OrderStatus           *string    `json:"order_status,omitempty" binding:"omitempty,max=50"`
	ProcessedByEmployeeID *uuid.UUID `json:"processed_by_employee_id,omitempty"`
	CustomerName          *string    `json:"customer_name,omitempty" binding:"omitempty,max=255"`
	PhoneNumber           *string    `json:"phone_number,omitempty" binding:"omitempty,max=50"`
	PickupTime            *string    `json:"pickup_time,omitempty" binding:"omitempty,max=50"`
	DeliveryAddress       *string    `json:"delivery_address,omitempty"`
	TableID               *int       `json:"table_id,omitempty"`
	IsActive              *bool      `json:"is_active,omitempty"`
}

// UpdateOrderItemStatusRequest represents the request to update order item status
type UpdateOrderItemStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=Pending InProgress Completed Cancelled"`
}

// OrderResponse represents the response for a single order
type OrderResponse struct {
	OrderID               uuid.UUID               `json:"order_id"`
	UserID                uuid.UUID               `json:"user_id"`
	TenantID              uuid.UUID               `json:"tenant_id"`
	RestaurantID          uuid.UUID               `json:"restaurant_id"`
	CustomerID            *uuid.UUID              `json:"customer_id,omitempty"`
	ProcessedByEmployeeID *uuid.UUID              `json:"processed_by_employee_id,omitempty"`
	TableID               *int                    `json:"table_id,omitempty"`
	BranchID              *uuid.UUID              `json:"branch_id,omitempty"`
	OrderStatus           string                  `json:"order_status"`
	TotalAmount           float64                 `json:"total_amount"`
	CustomerName          string                  `json:"customer_name"`
	PhoneNumber           string                  `json:"phone_number"`
	PickupTime            string                  `json:"pickup_time"`
	DeliveryAddress       string                  `json:"delivery_address"`
	IsActive              bool                    `json:"is_active"`
	Status                string                  `json:"status"`
	Version               int                     `json:"version"`
	CreatedAt             time.Time               `json:"created_at"`
	UpdatedAt             *time.Time              `json:"updated_at,omitempty"`
	OrderItems            []OrderItemResponse     `json:"order_items,omitempty"`
	Customer              *CustomerSummaryDTO     `json:"customer,omitempty"`
	ProcessedByEmployee   *EmployeeSummaryDTO     `json:"processed_by_employee,omitempty"`
	Table                 *TableSummaryDTO        `json:"table,omitempty"`
}

// OrderItemResponse represents an order item in the response
type OrderItemResponse struct {
	OrderItemID uuid.UUID                   `json:"order_item_id"`
	OrderID     uuid.UUID                   `json:"order_id"`
	ItemName    string                      `json:"item_name"`
	Quantity    int                         `json:"quantity"`
	Status      string                      `json:"status"`
	Modifiers   []OrderItemModifierResponse `json:"modifiers,omitempty"`
}

// OrderItemModifierResponse represents a modifier in the response
type OrderItemModifierResponse struct {
	OrderItemModifierID uuid.UUID `json:"order_item_modifier_id"`
	OrderItemID         uuid.UUID `json:"order_item_id"`
	ModifierName        string    `json:"modifier_name"`
	AdditionalPrice     float64   `json:"additional_price"`
}

// OrderListResponse represents a paginated list of orders
type OrderListResponse struct {
	Orders     []OrderResponse `json:"orders"`
	Total      int64           `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

// OrderSummaryDTO represents a minimal order summary
type OrderSummaryDTO struct {
	OrderID      uuid.UUID `json:"order_id"`
	OrderStatus  string    `json:"order_status"`
	TotalAmount  float64   `json:"total_amount"`
	CustomerName string    `json:"customer_name"`
	CreatedAt    time.Time `json:"created_at"`
}

// OrderLogResponse represents an order log entry
type OrderLogResponse struct {
	LogID     uuid.UUID `json:"log_id"`
	OrderID   uuid.UUID `json:"order_id"`
	UserID    uuid.UUID `json:"user_id"`
	Action    string    `json:"action"`
	Details   string    `json:"details"`
	Timestamp time.Time `json:"timestamp"`
}

// OrderFilterRequest represents filter options for listing orders
type OrderFilterRequest struct {
	RestaurantID  *uuid.UUID `form:"restaurant_id"`
	CustomerID    *uuid.UUID `form:"customer_id"`
	BranchID      *uuid.UUID `form:"branch_id"`
	OrderStatus   *string    `form:"order_status"`
	TableID       *int       `form:"table_id"`
	IsActive      *bool      `form:"is_active"`
	DateFrom      *time.Time `form:"date_from" time_format:"2006-01-02"`
	DateTo        *time.Time `form:"date_to" time_format:"2006-01-02"`
	CustomerName  *string    `form:"customer_name"`
	PhoneNumber   *string    `form:"phone_number"`
	Page          int        `form:"page" binding:"min=1"`
	PageSize      int        `form:"page_size" binding:"min=1,max=100"`
	SortBy        string     `form:"sort_by" binding:"omitempty,oneof=created_at updated_at total_amount order_status"`
	SortOrder     string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// OrderStatisticsResponse represents order statistics
type OrderStatisticsResponse struct {
	TotalOrders       int64   `json:"total_orders"`
	TotalRevenue      float64 `json:"total_revenue"`
	AverageOrderValue float64 `json:"average_order_value"`
	OrdersByStatus    map[string]int64 `json:"orders_by_status"`
	TodayOrders       int64   `json:"today_orders"`
	TodayRevenue      float64 `json:"today_revenue"`
}

// BulkUpdateOrderStatusRequest represents a bulk status update request
type BulkUpdateOrderStatusRequest struct {
	OrderIDs  []uuid.UUID `json:"order_ids" binding:"required,min=1"`
	NewStatus string      `json:"new_status" binding:"required,max=50"`
}

// CreateOrderLogRequest represents a request to create an order log entry
type CreateOrderLogRequest struct {
	OrderID uuid.UUID `json:"order_id" binding:"required"`
	Action  string    `json:"action" binding:"required,max=255"`
	Details string    `json:"details,omitempty"`
}

// Related entity summary DTOs (you may need to adjust these based on your actual models)

// CustomerSummaryDTO represents minimal customer information
type CustomerSummaryDTO struct {
	CustomerID   uuid.UUID `json:"customer_id"`
	CustomerName string    `json:"customer_name"`
	PhoneNumber  string    `json:"phone_number"`
	Email        string    `json:"email,omitempty"`
}

// EmployeeSummaryDTO represents minimal employee information
type EmployeeSummaryDTO struct {
	EmployeeID   uuid.UUID `json:"employee_id"`
	EmployeeName string    `json:"employee_name"`
	Role         string    `json:"role,omitempty"`
}

// TableSummaryDTO represents minimal table information
type TableSummaryDTO struct {
	TableID     int    `json:"table_id"`
	TableNumber string `json:"table_number"`
	TableName   string `json:"table_name,omitempty"`
	Section     string `json:"section,omitempty"`
}

// Validation helpers

// Validate validates the CreateOrderRequest
func (r *CreateOrderRequest) Validate() error {
	if len(r.OrderItems) == 0 {
		return ErrNoOrderItems
	}
	
	for _, item := range r.OrderItems {
		if item.Quantity < 1 {
			return ErrInvalidQuantity
		}
	}
	
	return nil
}

// Custom errors (define these in your errors package)
var (
	ErrNoOrderItems     = NewValidationError("order must have at least one item")
	ErrInvalidQuantity  = NewValidationError("item quantity must be at least 1")
)

// Helper function to create validation errors
func NewValidationError(message string) error {
	return &ValidationError{Message: message}
}

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}
package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ===================================
// KOT DTOs
// ===================================

// CreateKOTRequest
type CreateKOTRequest struct {
	OrderID      uuid.UUID  `json:"order_id" binding:"required"`
	RestaurantID uuid.UUID  `json:"restaurant_id" binding:"required"`
	TableNumber  *int       `json:"table_number,omitempty"`
	CustomerName string     `json:"customer_name" binding:"omitempty,max=255"`
	OrderType    string     `json:"order_type" binding:"required,oneof=DineIn Takeaway Delivery Online"`
	GuestCount   *int       `json:"guest_count" binding:"omitempty,min=1"`
	Priority     string     `json:"priority" binding:"omitempty,oneof=Low Normal High Urgent"`
	Items        []CreateKOTItemDTO `json:"items" binding:"required,min=1,dive"`
}

func (r *CreateKOTRequest) Validate() error {
	if len(r.Items) == 0 {
		return ErrNoKOTItems
	}
	if r.GuestCount != nil && *r.GuestCount < 1 {
		return ErrInvalidGuestCount
	}
	for i, item := range r.Items {
		if err := item.Validate(); err != nil {
			return NewValidationError("item " + string(rune('0'+i+1)) + ": " + err.Error())
		}
	}
	return nil
}

// CreateKOTItemDTO
type CreateKOTItemDTO struct {
	OrderItemID     uuid.UUID `json:"order_item_id" binding:"required"`
	ItemName        string    `json:"item_name" binding:"required,min=1,max=255"`
	Quantity        int       `json:"quantity" binding:"required,min=1"`
	Notes           string    `json:"notes" binding:"omitempty,max=500"`
	AssignedStation string    `json:"assigned_station" binding:"omitempty,oneof=General Grill Fry Saute Dessert Beverage Cold"`
}

func (r *CreateKOTItemDTO) Validate() error {
	if r.Quantity < 1 {
		return ErrInvalidKOTItemQuantity
	}
	return nil
}

// UpdateKOTRequest
type UpdateKOTRequest struct {
	Status           *string    `json:"status" binding:"omitempty,oneof=Sent InProgress Completed Cancelled"`
	Priority         *string    `json:"priority" binding:"omitempty,oneof=Low Normal High Urgent"`
	AssignedToChefID *uuid.UUID `json:"assigned_to_chef_id,omitempty"`
}

// UpdateKOTItemStatusRequest
type UpdateKOTItemStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=Pending InProgress Completed Cancelled"`
}

// PrintKOTRequest
type PrintKOTRequest struct {
	PrinterID string `json:"printer_id" binding:"omitempty,max=100"`
}

// KOTResponse
type KOTResponse struct {
	KOTID            uuid.UUID          `json:"kot_id"`
	OrderID          uuid.UUID          `json:"order_id"`
	RestaurantID     uuid.UUID          `json:"restaurant_id"`
	TenantID         uuid.UUID          `json:"tenant_id"`
	KOTNumber        string             `json:"kot_number"`
	SequenceNumber   int                `json:"sequence_number"`
	OrderNumber      string             `json:"order_number"`
	TableNumber      *int               `json:"table_number,omitempty"`
	CustomerName     string             `json:"customer_name"`
	OrderType        string             `json:"order_type"`
	GuestCount       *int               `json:"guest_count,omitempty"`
	Status           string             `json:"status"`
	Priority         string             `json:"priority"`
	CreatedByUserID  uuid.UUID          `json:"created_by_user_id"`
	CreatedByName    string             `json:"created_by_name"`
	AssignedToChefID *uuid.UUID         `json:"assigned_to_chef_id,omitempty"`
	PrintCount       int                `json:"print_count"`
	LastPrintedAt    *time.Time         `json:"last_printed_at,omitempty"`
	Items            []KOTItemResponse  `json:"items,omitempty"`
	AssignedToChef   *EmployeeSummaryDTO `json:"assigned_to_chef,omitempty"`
	CreatedAt        time.Time          `json:"created_at"`
}

// KOTItemResponse
type KOTItemResponse struct {
	KOTItemID       uuid.UUID `json:"kot_item_id"`
	KOTID           uuid.UUID `json:"kot_id"`
	OrderItemID     uuid.UUID `json:"order_item_id"`
	ItemName        string    `json:"item_name"`
	Quantity        int       `json:"quantity"`
	Notes           string    `json:"notes,omitempty"`
	AssignedStation string    `json:"assigned_station"`
	Status          string    `json:"status"`
}

// KOTListResponse
type KOTListResponse struct {
	KOTs       []KOTResponse `json:"kots"`
	Total      int64         `json:"total"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	TotalPages int           `json:"total_pages"`
}

// KOTFilterRequest
type KOTFilterRequest struct {
	RestaurantID *uuid.UUID `form:"restaurant_id"`
	OrderID      *uuid.UUID `form:"order_id"`
	Status       *string    `form:"status" binding:"omitempty,oneof=Sent InProgress Completed Cancelled"`
	Priority     *string    `form:"priority" binding:"omitempty,oneof=Low Normal High Urgent"`
	Station      *string    `form:"station" binding:"omitempty,oneof=General Grill Fry Saute Dessert Beverage Cold"`
	ChefID       *uuid.UUID `form:"chef_id"`
	DateFrom     *time.Time `form:"date_from" time_format:"2006-01-02"`
	DateTo       *time.Time `form:"date_to" time_format:"2006-01-02"`
	Page         int        `form:"page" binding:"min=1"`
	PageSize     int        `form:"page_size" binding:"min=1,max=100"`
	SortBy       string     `form:"sort_by" binding:"omitempty,oneof=created_at sequence_number priority status"`
	SortOrder    string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// KOTSummaryDTO - used in order responses
type KOTSummaryDTO struct {
	KOTID      uuid.UUID `json:"kot_id"`
	KOTNumber  string    `json:"kot_number"`
	Status     string    `json:"status"`
	Priority   string    `json:"priority"`
	CreatedAt  time.Time `json:"created_at"`
}

// KOTStatsResponse - kitchen display stats
type KOTStatsResponse struct {
	TotalPending    int `json:"total_pending"`
	TotalInProgress int `json:"total_in_progress"`
	TotalCompleted  int `json:"total_completed"`
	TotalCancelled  int `json:"total_cancelled"`
	AvgPrepTimeMins int `json:"avg_prep_time_mins"`
}

// ===================================
// VALIDATION ERRORS
// ===================================

var (
	ErrNoKOTItems             = NewValidationError("KOT must have at least one item")
	ErrInvalidGuestCount      = NewValidationError("guest count must be at least 1")
	ErrInvalidKOTItemQuantity = NewValidationError("KOT item quantity must be at least 1")
)
package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ===================================
// CUSTOMER DTOs
// ===================================

// CreateCustomerRequest
type CreateCustomerRequest struct {
	FirstName   string     `json:"first_name" binding:"required,min=2,max=255"`
	LastName    string     `json:"last_name" binding:"omitempty,max=255"`
	Email       string     `json:"email" binding:"omitempty,email,max=255"`
	Phone       string     `json:"phone" binding:"omitempty,max=50"`
	Address     string     `json:"address" binding:"omitempty,max=1000"`
	City        string     `json:"city" binding:"omitempty,max=100"`
	State       string     `json:"state" binding:"omitempty,max=100"`
	PostalCode  string     `json:"postal_code" binding:"omitempty,max=20"`
	DateOfBirth *time.Time `json:"date_of_birth,omitempty"`
}

func (r *CreateCustomerRequest) Validate() error {
	if r.Email == "" && r.Phone == "" {
		return ErrCustomerContactRequired
	}
	if r.DateOfBirth != nil && r.DateOfBirth.After(time.Now()) {
		return ErrFutureDateOfBirth
	}
	if r.DateOfBirth != nil && time.Since(*r.DateOfBirth).Hours() > 150*365*24 {
		return ErrInvalidDateOfBirth
	}
	return nil
}

// UpdateCustomerRequest
type UpdateCustomerRequest struct {
	FirstName   *string    `json:"first_name" binding:"omitempty,min=2,max=255"`
	LastName    *string    `json:"last_name" binding:"omitempty,max=255"`
	Email       *string    `json:"email" binding:"omitempty,email,max=255"`
	Phone       *string    `json:"phone" binding:"omitempty,max=50"`
	Address     *string    `json:"address" binding:"omitempty,max=1000"`
	City        *string    `json:"city" binding:"omitempty,max=100"`
	State       *string    `json:"state" binding:"omitempty,max=100"`
	PostalCode  *string    `json:"postal_code" binding:"omitempty,max=20"`
	DateOfBirth *time.Time `json:"date_of_birth,omitempty"`
	IsActive    *bool      `json:"is_active"`
}

// AdjustLoyaltyPointsRequest
type AdjustLoyaltyPointsRequest struct {
	Points int    `json:"points" binding:"required"`
	Reason string `json:"reason" binding:"required,max=255"`
}

func (r *AdjustLoyaltyPointsRequest) Validate() error {
	if r.Points == 0 {
		return ErrZeroLoyaltyPoints
	}
	return nil
}

// CustomerResponse
type CustomerResponse struct {
	CustomerID    uuid.UUID  `json:"customer_id"`
	TenantID      uuid.UUID  `json:"tenant_id"`
	FirstName     string     `json:"first_name"`
	LastName      string     `json:"last_name"`
	FullName      string     `json:"full_name"` // Computed: FirstName + LastName
	Email         string     `json:"email"`
	Phone         string     `json:"phone"`
	Address       string     `json:"address"`
	City          string     `json:"city"`
	State         string     `json:"state"`
	PostalCode    string     `json:"postal_code"`
	DateOfBirth   *time.Time `json:"date_of_birth,omitempty"`
	LoyaltyPoints int        `json:"loyalty_points"`
	TotalOrders   int        `json:"total_orders"`
	TotalSpent    float64    `json:"total_spent"`
	IsActive      bool       `json:"is_active"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CustomerListResponse
type CustomerListResponse struct {
	Customers  []CustomerResponse `json:"customers"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	TotalPages int                `json:"total_pages"`
}

// CustomerFilterRequest
type CustomerFilterRequest struct {
	Search    *string `form:"search"` // Search by name, email, phone
	City      *string `form:"city"`
	IsActive  *bool   `form:"is_active"`
	Page      int     `form:"page" binding:"min=1"`
	PageSize  int     `form:"page_size" binding:"min=1,max=100"`
	SortBy    string  `form:"sort_by" binding:"omitempty,oneof=first_name last_name total_spent total_orders created_at"`
	SortOrder string  `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// CustomerSummaryDTO - used in other responses (order, payment, etc)
// type CustomerSummaryDTO struct {
// 	CustomerID uuid.UUID `json:"customer_id"`
// 	FirstName  string    `json:"first_name"`
// 	LastName   string    `json:"last_name"`
// 	FullName   string    `json:"full_name"`
// 	Email      string    `json:"email"`
// 	Phone      string    `json:"phone"`
// }

// CustomerStatsResponse
type CustomerStatsResponse struct {
	TotalCustomers   int64   `json:"total_customers"`
	ActiveCustomers  int64   `json:"active_customers"`
	NewThisMonth     int64   `json:"new_this_month"`
	TotalLoyaltyPts  int64   `json:"total_loyalty_points"`
	AvgOrderValue    float64 `json:"avg_order_value"`
	AvgOrdersPerCust float64 `json:"avg_orders_per_customer"`
}

// ===================================
// VALIDATION ERRORS
// ===================================

var (
	ErrCustomerContactRequired = NewValidationError("at least one contact method (email or phone) is required")
	ErrFutureDateOfBirth       = NewValidationError("date of birth cannot be in the future")
	ErrInvalidDateOfBirth      = NewValidationError("date of birth is invalid")
	ErrZeroLoyaltyPoints       = NewValidationError("loyalty points adjustment cannot be zero")
)
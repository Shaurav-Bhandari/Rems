package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ===================================
// PLAN DTOs
// ===================================

// CreatePlanRequest
type CreatePlanRequest struct {
	Name           string   `json:"name" binding:"required,min=2,max=255"`
	Description    string   `json:"description" binding:"omitempty,max=1000"`
	Price          float64  `json:"price" binding:"required,min=0"`
	BillingCycle   string   `json:"billing_cycle" binding:"required,oneof=monthly yearly"`
	MaxRestaurants *int     `json:"max_restaurants" binding:"omitempty,min=1"`
	MaxUsers       *int     `json:"max_users" binding:"omitempty,min=1"`
	Features       []string `json:"features" binding:"omitempty"` // Feature names
}

// Validate validates CreatePlanRequest
func (r *CreatePlanRequest) Validate() error {
	if r.Price < 0 {
		return ErrNegativePrice
	}
	if r.MaxRestaurants != nil && *r.MaxRestaurants < 1 {
		return ErrInvalidMaxRestaurants
	}
	if r.MaxUsers != nil && *r.MaxUsers < 1 {
		return ErrInvalidMaxUsers
	}
	return nil
}

// UpdatePlanRequest
type UpdatePlanRequest struct {
	Name           *string  `json:"name" binding:"omitempty,min=2,max=255"`
	Description    *string  `json:"description" binding:"omitempty,max=1000"`
	Price          *float64 `json:"price" binding:"omitempty,min=0"`
	BillingCycle   *string  `json:"billing_cycle" binding:"omitempty,oneof=monthly yearly"`
	MaxRestaurants *int     `json:"max_restaurants" binding:"omitempty,min=1"`
	MaxUsers       *int     `json:"max_users" binding:"omitempty,min=1"`
	IsActive       *bool    `json:"is_active"`
}

// Validate validates UpdatePlanRequest
func (r *UpdatePlanRequest) Validate() error {
	if r.Price != nil && *r.Price < 0 {
		return ErrNegativePrice
	}
	if r.MaxRestaurants != nil && *r.MaxRestaurants < 1 {
		return ErrInvalidMaxRestaurants
	}
	if r.MaxUsers != nil && *r.MaxUsers < 1 {
		return ErrInvalidMaxUsers
	}
	return nil
}

// PlanResponse
type PlanResponse struct {
	PlanID         uuid.UUID        `json:"plan_id"`
	Name           string           `json:"name"`
	Description    string           `json:"description"`
	Price          float64          `json:"price"`
	BillingCycle   string           `json:"billing_cycle"`
	MaxRestaurants *int             `json:"max_restaurants,omitempty"`
	MaxUsers       *int             `json:"max_users,omitempty"`
	IsActive       bool             `json:"is_active"`
	Features       []PlanFeatureDTO `json:"features,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

// PlanFeatureDTO
type PlanFeatureDTO struct {
	PlanFeatureID uuid.UUID `json:"plan_feature_id"`
	FeatureName   string    `json:"feature_name"`
	FeatureValue  string    `json:"feature_value"`
	IsEnabled     bool      `json:"is_enabled"`
}

// PlanListResponse
type PlanListResponse struct {
	Plans      []PlanResponse `json:"plans"`
	Total      int64          `json:"total"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	TotalPages int            `json:"total_pages"`
}

// PlanSummaryDTO
type PlanSummaryDTO struct {
	PlanID       uuid.UUID `json:"plan_id"`
	Name         string    `json:"name"`
	Price        float64   `json:"price"`
	BillingCycle string    `json:"billing_cycle"`
}

// ===================================
// SUBSCRIPTION DTOs
// ===================================

// CreateSubscriptionRequest
type CreateSubscriptionRequest struct {
	TenantID  uuid.UUID `json:"tenant_id" binding:"required"`
	PlanID    uuid.UUID `json:"plan_id" binding:"required"`
	StartDate time.Time `json:"start_date" binding:"required"`
	AutoRenew bool      `json:"auto_renew"`
}

// Validate validates CreateSubscriptionRequest
func (r *CreateSubscriptionRequest) Validate() error {
	if r.StartDate.Before(time.Now().AddDate(0, 0, -1)) {
		return ErrPastStartDate
	}
	return nil
}

// UpdateSubscriptionRequest
type UpdateSubscriptionRequest struct {
	PlanID    *uuid.UUID `json:"plan_id"`
	EndDate   *time.Time `json:"end_date"`
	Status    *string    `json:"status" binding:"omitempty,oneof=active cancelled expired trial"`
	AutoRenew *bool      `json:"auto_renew"`
}

// Validate validates UpdateSubscriptionRequest
func (r *UpdateSubscriptionRequest) Validate() error {
	if r.EndDate != nil && r.EndDate.Before(time.Now()) {
		return ErrPastEndDate
	}
	return nil
}

// SubscriptionResponse
type SubscriptionResponse struct {
	SubscriptionID uuid.UUID      `json:"subscription_id"`
	TenantID       uuid.UUID      `json:"tenant_id"`
	PlanID         uuid.UUID      `json:"plan_id"`
	StartDate      time.Time      `json:"start_date"`
	EndDate        *time.Time     `json:"end_date,omitempty"`
	Status         string         `json:"status"`
	AutoRenew      bool           `json:"auto_renew"`
	DaysRemaining  int            `json:"days_remaining"` // Calculated
	Plan           *PlanSummaryDTO `json:"plan,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// SubscriptionListResponse
type SubscriptionListResponse struct {
	Subscriptions []SubscriptionResponse `json:"subscriptions"`
	Total         int64                  `json:"total"`
	Page          int                    `json:"page"`
	PageSize      int                    `json:"page_size"`
	TotalPages    int                    `json:"total_pages"`
}

// SubscriptionFilterRequest
type SubscriptionFilterRequest struct {
	TenantID  *uuid.UUID `form:"tenant_id"`
	PlanID    *uuid.UUID `form:"plan_id"`
	Status    *string    `form:"status" binding:"omitempty,oneof=active cancelled expired trial"`
	AutoRenew *bool      `form:"auto_renew"`
	Page      int        `form:"page" binding:"min=1"`
	PageSize  int        `form:"page_size" binding:"min=1,max=100"`
	SortBy    string     `form:"sort_by" binding:"omitempty,oneof=start_date end_date status"`
	SortOrder string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// ===================================
// ADDON DTOs
// ===================================

// CreateAddonRequest
type CreateAddonRequest struct {
	TenantID  uuid.UUID  `json:"tenant_id" binding:"required"`
	AddonName string     `json:"addon_name" binding:"required,min=2,max=255"`
	Price     float64    `json:"price" binding:"required,min=0"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Validate validates CreateAddonRequest
func (r *CreateAddonRequest) Validate() error {
	if r.Price < 0 {
		return ErrNegativePrice
	}
	if r.ExpiresAt != nil && r.ExpiresAt.Before(time.Now()) {
		return ErrPastExpiryDate
	}
	return nil
}

// UpdateAddonRequest
type UpdateAddonRequest struct {
	Price     *float64   `json:"price" binding:"omitempty,min=0"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	IsActive  *bool      `json:"is_active"`
}

// TenantAddonResponse
type TenantAddonResponse struct {
	TenantAddonID uuid.UUID  `json:"tenant_addon_id"`
	TenantID      uuid.UUID  `json:"tenant_id"`
	AddonName     string     `json:"addon_name"`
	Price         float64    `json:"price"`
	ActivatedAt   time.Time  `json:"activated_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	IsActive      bool       `json:"is_active"`
}

// TenantAddonListResponse
type TenantAddonListResponse struct {
	Addons     []TenantAddonResponse `json:"addons"`
	Total      int64                 `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
	TotalPages int                   `json:"total_pages"`
}

// ===================================
// VALIDATION ERRORS
// ===================================

var (
	ErrNegativePrice          = NewValidationError("price cannot be negative")
	ErrInvalidMaxRestaurants  = NewValidationError("max restaurants must be at least 1")
	ErrInvalidMaxUsers        = NewValidationError("max users must be at least 1")
	ErrPastStartDate          = NewValidationError("start date cannot be in the past")
	ErrPastEndDate            = NewValidationError("end date cannot be in the past")
	ErrPastExpiryDate         = NewValidationError("expiry date cannot be in the past")
)
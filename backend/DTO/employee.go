package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ===================================
// EMPLOYEE DTOs
// ===================================

// CreateEmployeeRequest
type CreateEmployeeRequest struct {
	RestaurantID    uuid.UUID  `json:"restaurant_id" binding:"required"`
	UserID          *uuid.UUID `json:"user_id,omitempty"`
	FirstName       string     `json:"first_name" binding:"required,min=2,max=255"`
	LastName        string     `json:"last_name" binding:"required,min=2,max=255"`
	Email           string     `json:"email" binding:"omitempty,email,max=255"`
	Phone           string     `json:"phone" binding:"omitempty,max=50"`
	Position        string     `json:"position" binding:"required,max=100"`
	Department      string     `json:"department" binding:"omitempty,max=100"`
	HireDate        *time.Time `json:"hire_date,omitempty"`
	HourlyRate      *float64   `json:"hourly_rate" binding:"omitempty,min=0"`
}

func (r *CreateEmployeeRequest) Validate() error {
	if r.HourlyRate != nil && *r.HourlyRate < 0 {
		return ErrNegativeHourlyRate
	}
	if r.HireDate != nil && r.HireDate.After(time.Now()) {
		return ErrFutureHireDate
	}
	return nil
}

// UpdateEmployeeRequest
type UpdateEmployeeRequest struct {
	FirstName       *string    `json:"first_name" binding:"omitempty,min=2,max=255"`
	LastName        *string    `json:"last_name" binding:"omitempty,min=2,max=255"`
	Email           *string    `json:"email" binding:"omitempty,email,max=255"`
	Phone           *string    `json:"phone" binding:"omitempty,max=50"`
	Position        *string    `json:"position" binding:"omitempty,max=100"`
	Department      *string    `json:"department" binding:"omitempty,max=100"`
	HourlyRate      *float64   `json:"hourly_rate" binding:"omitempty,min=0"`
	IsActive        *bool      `json:"is_active"`
	TerminationDate *time.Time `json:"termination_date,omitempty"`
}

func (r *UpdateEmployeeRequest) Validate() error {
	if r.HourlyRate != nil && *r.HourlyRate < 0 {
		return ErrNegativeHourlyRate
	}
	if r.TerminationDate != nil && r.TerminationDate.After(time.Now()) {
		return ErrFutureTerminationDate
	}
	return nil
}

// TerminateEmployeeRequest
type TerminateEmployeeRequest struct {
	TerminationDate time.Time `json:"termination_date" binding:"required"`
	Reason          string    `json:"reason" binding:"required,max=500"`
}

func (r *TerminateEmployeeRequest) Validate() error {
	if r.TerminationDate.After(time.Now().AddDate(0, 0, 1)) {
		return ErrFutureTerminationDate
	}
	return nil
}

// AssignEmployeeRoleRequest
type AssignEmployeeRoleRequest struct {
	RoleID uuid.UUID `json:"role_id" binding:"required"`
}

// EmployeeResponse
type EmployeeResponse struct {
	EmployeeID      uuid.UUID   `json:"employee_id"`
	TenantID        uuid.UUID   `json:"tenant_id"`
	RestaurantID    uuid.UUID   `json:"restaurant_id"`
	UserID          *uuid.UUID  `json:"user_id,omitempty"`
	FirstName       string      `json:"first_name"`
	LastName        string      `json:"last_name"`
	FullName        string      `json:"full_name"` // Computed: FirstName + LastName
	Email           string      `json:"email"`
	Phone           string      `json:"phone"`
	Position        string      `json:"position"`
	Department      string      `json:"department"`
	HireDate        *time.Time  `json:"hire_date,omitempty"`
	TerminationDate *time.Time  `json:"termination_date,omitempty"`
	HourlyRate      *float64    `json:"hourly_rate,omitempty"`
	IsActive        bool        `json:"is_active"`
	Roles           []RoleSummaryDTO `json:"roles,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// EmployeeListResponse
type EmployeeListResponse struct {
	Employees  []EmployeeResponse `json:"employees"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	TotalPages int                `json:"total_pages"`
}

// EmployeeFilterRequest
type EmployeeFilterRequest struct {
	RestaurantID *uuid.UUID `form:"restaurant_id"`
	Department   *string    `form:"department"`
	Position     *string    `form:"position"`
	IsActive     *bool      `form:"is_active"`
	Search       *string    `form:"search"` // Search by name, email, phone
	Page         int        `form:"page" binding:"min=1"`
	PageSize     int        `form:"page_size" binding:"min=1,max=100"`
	SortBy       string     `form:"sort_by" binding:"omitempty,oneof=first_name last_name position hire_date created_at"`
	SortOrder    string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// EmployeeSummaryDTO - used in other responses
// type EmployeeSummaryDTO struct {
// 	EmployeeID uuid.UUID `json:"employee_id"`
// 	FirstName  string    `json:"first_name"`
// 	LastName   string    `json:"last_name"`
// 	FullName   string    `json:"full_name"`
// 	Position   string    `json:"position"`
// }

// ===================================
// EMPLOYEE ROLE DTOs
// ===================================

// EmployeeRoleResponse
type EmployeeRoleResponse struct {
	EmployeeRoleID uuid.UUID          `json:"employee_role_id"`
	EmployeeID     uuid.UUID          `json:"employee_id"`
	RoleID         uuid.UUID          `json:"role_id"`
	AssignedAt     time.Time          `json:"assigned_at"`
	AssignedBy     *uuid.UUID         `json:"assigned_by,omitempty"`
	Role           *RoleSummaryDTO    `json:"role,omitempty"`
	Employee       *EmployeeSummaryDTO `json:"employee,omitempty"`
}

// ===================================
// VALIDATION ERRORS
// ===================================

var (
	ErrNegativeHourlyRate     = NewValidationError("hourly rate cannot be negative")
	ErrFutureHireDate         = NewValidationError("hire date cannot be in the future")
	ErrFutureTerminationDate  = NewValidationError("termination date cannot be in the future")
)
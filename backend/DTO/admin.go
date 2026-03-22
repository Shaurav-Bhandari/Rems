package DTO

import (
	"time"

	"backend/models"

	"github.com/google/uuid"
)

// ============================================================================
// REQUEST DTOs
// ============================================================================

// CreateAdminRequest — POST /api/v1/admins
type CreateAdminRequest struct {
	UserName string `json:"user_name" validate:"required,min=2,max=255"`
	FullName string `json:"full_name" validate:"required,min=2,max=255"`
	Email    string `json:"email"     validate:"required,email"`
	Phone    string `json:"phone"     validate:"omitempty"`
	Password string `json:"password"  validate:"required,min=6"`
}

// UpdateAdminRequest — PUT /api/v1/admins/:id
type UpdateAdminRequest struct {
	FullName *string `json:"full_name" validate:"omitempty,min=2,max=255"`
	Phone    *string `json:"phone"     validate:"omitempty"`
	IsActive *bool   `json:"is_active"`
}

// ChangeAdminPasswordRequest — PUT /api/v1/admins/:id/password
type ChangeAdminPasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password"     validate:"required,min=6"`
}

// ============================================================================
// RESPONSE DTOs
// ============================================================================

// AdminResponse — safe public representation of an Admin
type AdminResponse struct {
	AdminID         uuid.UUID  `json:"admin_id"`
	UserName        string     `json:"user_name"`
	FullName        string     `json:"full_name"`
	Email           string     `json:"email"`
	Phone           string     `json:"phone,omitempty"`
	IsActive        bool       `json:"is_active"`
	IsEmailVerified bool       `json:"is_email_verified"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ListAdminsResponse — paginated list
type ListAdminsResponse struct {
	Admins     []AdminResponse `json:"admins"`
	Total      int64           `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

// ============================================================================
// CONVERTERS
// ============================================================================

// ToModel converts CreateAdminRequest → Admin model (password must be pre-hashed)
func (req *CreateAdminRequest) ToModel(hashedPassword string) *models.Admin {
	return &models.Admin{
		UserName: req.UserName,
		FullName: req.FullName,
		Email:    req.Email,
		Phone:    req.Phone,
		Password: hashedPassword,
		IsActive: true,
	}
}

// FromAdmin converts Admin model → AdminResponse
func FromAdmin(a *models.Admin) *AdminResponse {
	return &AdminResponse{
		AdminID:         a.AdminID,
		UserName:        a.UserName,
		FullName:        a.FullName,
		Email:           a.Email,
		Phone:           a.Phone,
		IsActive:        a.IsActive,
		IsEmailVerified: a.IsEmailVerified,
		LastLoginAt:     a.LastLoginAt,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
	}
}

// FromAdmins converts a slice of Admin models → []AdminResponse
func FromAdmins(admins []models.Admin) []AdminResponse {
	out := make([]AdminResponse, len(admins))
	for i, a := range admins {
		out[i] = *FromAdmin(&a)
	}
	return out
}
package DTO

import (
    "time"
    "github.com/google/uuid"
    "backend/models"
)

// CreateUserRequest - Create new user
type CreateUserRequest struct {
    UserName       string    `json:"user_name" binding:"required,min=3,max=255"`
    FullName       string    `json:"full_name" binding:"required,min=2,max=255"`
    Email          string    `json:"email" binding:"required,email"`
    Phone          string    `json:"phone" binding:"omitempty,e164"`
    Password       string    `json:"password" binding:"required,min=8"`
    OrganizationID uuid.UUID `json:"organization_id" binding:"required,uuid"`
    BranchID       uuid.UUID `json:"branch_id" binding:"required,uuid"`
    DefaultRoleID  uuid.UUID `json:"default_role_id" binding:"required,uuid"`
}

// UpdateUserRequest - Update existing user
type UpdateUserRequest struct {
    FullName  *string `json:"full_name" binding:"omitempty,min=2,max=255"`
    Phone     *string `json:"phone" binding:"omitempty,e164"`
    IsActive  *bool   `json:"is_active"`
    BranchID  *uuid.UUID `json:"branch_id" binding:"omitempty,uuid"`
}

// UserResponse - User details response
type UserResponse struct {
    UserID         uuid.UUID  `json:"user_id"`
    UserName       string     `json:"user_name"`
    FullName       string     `json:"full_name"`
    Email          string     `json:"email"`
    Phone          string     `json:"phone,omitempty"`
    IsActive       bool       `json:"is_active"`
    OrganizationID uuid.UUID  `json:"organization_id"`
    BranchID       uuid.UUID  `json:"branch_id"`
    TenantID       uuid.UUID  `json:"tenant_id"`
    DefaultRole    RoleInfo   `json:"default_role"`
    Roles          []RoleInfo `json:"roles,omitempty"`
    CreatedAt      time.Time  `json:"created_at"`
}

type RoleInfo struct {
    RoleID   uuid.UUID `json:"role_id"`
    RoleName string    `json:"role_name"`
}

// ListUsersRequest - Query parameters
type ListUsersRequest struct {
    Page         int       `form:"page" binding:"omitempty,min=1"`
    PageSize     int       `form:"page_size" binding:"omitempty,min=1,max=100"`
    Search       string    `form:"search"`
    IsActive     *bool     `form:"is_active"`
    BranchID     uuid.UUID `form:"branch_id" binding:"omitempty,uuid"`
    RoleID       uuid.UUID `form:"role_id" binding:"omitempty,uuid"`
    SortBy       string    `form:"sort_by" binding:"omitempty,oneof=created_at full_name email"`
    SortOrder    string    `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// ListUsersResponse - Paginated user list
type ListUsersResponse struct {
    Users      []UserResponse `json:"users"`
    Total      int64          `json:"total"`
    Page       int            `json:"page"`
    PageSize   int            `json:"page_size"`
    TotalPages int            `json:"total_pages"`
}

// ToModel - Convert CreateUserRequest to User model
func (req *CreateUserRequest) ToModel(tenantID uuid.UUID, hashedPassword string) *models.User {
    return &models.User{
        TenantID:       tenantID,
        UserName:       req.UserName,
        FullName:       req.FullName,
        Email:          req.Email,
        Phone:          req.Phone,
        PasswordHash:   hashedPassword,
        OrganizationID: req.OrganizationID,
        BranchID:       req.BranchID,
        DefaultRoleId:  req.DefaultRoleID,
        IsActive:       true,
        IsDeleted:      false,
    }
}

// FromUser - Convert User model to UserResponse
func FromUser(user *models.User) *UserResponse {
    resp := &UserResponse{
        UserID:         user.UserID,
        UserName:       user.UserName,
        FullName:       user.FullName,
        Email:          user.Email,
        Phone:          user.Phone,
        IsActive:       user.IsActive,
        OrganizationID: user.OrganizationID,
        BranchID:       user.BranchID,
        TenantID:       user.TenantID,
        CreatedAt:      user.CreatedAt,
    }
    
    // Add role info if loaded
    if user.DefaultRole.RoleID != uuid.Nil {
        resp.DefaultRole = RoleInfo{
            RoleID:   user.DefaultRole.RoleID,
            RoleName: user.DefaultRole.RoleName,
        }
    }
    
    // Add roles if loaded
    if len(user.Roles) > 0 {
        resp.Roles = make([]RoleInfo, len(user.Roles))
        for i, role := range user.Roles {
            resp.Roles[i] = RoleInfo{
                RoleID:   role.RoleID,
                RoleName: role.RoleName,
            }
        }
    }
    
    return resp
}

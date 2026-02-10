package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// ROLE DTOs
// ============================================================================

// CreateRoleRequest represents the request to create a new role
type CreateRoleRequest struct {
	RoleName    string   `json:"role_name" binding:"required,min=2,max=255"`
	Description string   `json:"description" binding:"omitempty,max=1000"`
	Permissions []string `json:"permissions" binding:"omitempty"` // Optional: list of permission names
}

// UpdateRoleRequest represents the request to update a role
type UpdateRoleRequest struct {
	RoleName    *string   `json:"role_name" binding:"omitempty,min=2,max=255"`
	Description *string   `json:"description" binding:"omitempty,max=1000"`
	Permissions *[]string `json:"permissions" binding:"omitempty"` // Optional: update permissions
}

// RoleResponse represents the response for a single role
type RoleResponse struct {
	RoleID      uuid.UUID  `json:"role_id"`
	TenantID    uuid.UUID  `json:"tenant_id"`
	RoleName    string     `json:"role_name"`
	Description string     `json:"description"`
	UserCount   int        `json:"user_count,omitempty"` // Number of users with this role
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Permissions []string   `json:"permissions,omitempty"` // List of permission names
}

// RoleDetailResponse includes permissions and users
type RoleDetailResponse struct {
	RoleID      uuid.UUID       `json:"role_id"`
	TenantID    uuid.UUID       `json:"tenant_id"`
	RoleName    string          `json:"role_name"`
	Description string          `json:"description"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Permissions []PermissionDTO `json:"permissions,omitempty"`
	Users       []UserSummaryDTO `json:"users,omitempty"`
	UserCount   int             `json:"user_count"`
}

// RoleSummaryDTO represents minimal role information
type RoleSummaryDTO struct {
	RoleID   uuid.UUID `json:"role_id"`
	RoleName string    `json:"role_name"`
}

// RoleListResponse represents a paginated list of roles
type RoleListResponse struct {
	Roles      []RoleResponse `json:"roles"`
	Total      int64          `json:"total"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	TotalPages int            `json:"total_pages"`
}

// RoleFilterRequest represents filter options for listing roles
type RoleFilterRequest struct {
	Search    *string `form:"search"` // search by role name or description
	Page      int     `form:"page" binding:"min=1"`
	PageSize  int     `form:"page_size" binding:"min=1,max=100"`
	SortBy    string  `form:"sort_by" binding:"omitempty,oneof=role_name created_at user_count"`
	SortOrder string  `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// ============================================================================
// USER ROLE ASSIGNMENT DTOs
// ============================================================================

// AssignRoleToUserRequest represents assigning a role to a user
type AssignRoleToUserRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
	RoleID uuid.UUID `json:"role_id" binding:"required"`
}

// RemoveRoleFromUserRequest represents removing a role from a user
type RemoveRoleFromUserRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
	RoleID uuid.UUID `json:"role_id" binding:"required"`
}

// BulkAssignRolesRequest represents assigning multiple roles to a user
type BulkAssignRolesRequest struct {
	UserID  uuid.UUID   `json:"user_id" binding:"required"`
	RoleIDs []uuid.UUID `json:"role_ids" binding:"required,min=1"`
}

// BulkRemoveRolesRequest represents removing multiple roles from a user
type BulkRemoveRolesRequest struct {
	UserID  uuid.UUID   `json:"user_id" binding:"required"`
	RoleIDs []uuid.UUID `json:"role_ids" binding:"required,min=1"`
}

// AssignRolesToMultipleUsersRequest assigns one role to multiple users
type AssignRolesToMultipleUsersRequest struct {
	RoleID  uuid.UUID   `json:"role_id" binding:"required"`
	UserIDs []uuid.UUID `json:"user_ids" binding:"required,min=1"`
}

// UpdateUserDefaultRoleRequest updates a user's default role
type UpdateUserDefaultRoleRequest struct {
	DefaultRoleID uuid.UUID `json:"default_role_id" binding:"required"`
}

// UserRoleResponse represents the user-role relationship
type UserRoleResponse struct {
	UserRoleID uuid.UUID  `json:"user_role_id"`
	UserID     uuid.UUID  `json:"user_id"`
	RoleID     uuid.UUID  `json:"role_id"`
	AssignedAt time.Time  `json:"assigned_at"`
	User       *UserSummaryDTO `json:"user,omitempty"`
	Role       *RoleSummaryDTO `json:"role,omitempty"`
}

// UserRolesResponse represents all roles for a user
type UserRolesResponse struct {
	UserID      uuid.UUID       `json:"user_id"`
	UserName    string          `json:"user_name"`
	FullName    string          `json:"full_name"`
	DefaultRole RoleSummaryDTO  `json:"default_role"`
	Roles       []RoleSummaryDTO `json:"roles"`
}

// RoleUsersResponse represents all users with a specific role
type RoleUsersResponse struct {
	RoleID      uuid.UUID         `json:"role_id"`
	RoleName    string            `json:"role_name"`
	Description string            `json:"description"`
	Users       []UserSummaryDTO  `json:"users"`
	UserCount   int               `json:"user_count"`
}

// ============================================================================
// PERMISSION DTOs (for future permission-based access control)
// ============================================================================

// PermissionDTO represents a permission
type PermissionDTO struct {
	PermissionID   uuid.UUID `json:"permission_id,omitempty"`
	PermissionName string    `json:"permission_name"`
	Description    string    `json:"description,omitempty"`
	Resource       string    `json:"resource,omitempty"`       // e.g., "orders", "menu", "users"
	Action         string    `json:"action,omitempty"`         // e.g., "create", "read", "update", "delete"
	Category       string    `json:"category,omitempty"`       // e.g., "restaurant_management", "user_management"
}

// CreatePermissionRequest represents creating a new permission
type CreatePermissionRequest struct {
	PermissionName string `json:"permission_name" binding:"required,max=100"`
	Description    string `json:"description" binding:"omitempty,max=500"`
	Resource       string `json:"resource" binding:"required,max=50"`
	Action         string `json:"action" binding:"required,oneof=create read update delete manage"`
	Category       string `json:"category" binding:"omitempty,max=50"`
}

// AssignPermissionsToRoleRequest assigns permissions to a role
type AssignPermissionsToRoleRequest struct {
	RoleID        uuid.UUID `json:"role_id" binding:"required"`
	PermissionIDs []uuid.UUID `json:"permission_ids" binding:"required,min=1"`
}

// RemovePermissionsFromRoleRequest removes permissions from a role
type RemovePermissionsFromRoleRequest struct {
	RoleID        uuid.UUID `json:"role_id" binding:"required"`
	PermissionIDs []uuid.UUID `json:"permission_ids" binding:"required,min=1"`
}

// RolePermissionsResponse shows all permissions for a role
type RolePermissionsResponse struct {
	RoleID      uuid.UUID       `json:"role_id"`
	RoleName    string          `json:"role_name"`
	Permissions []PermissionDTO `json:"permissions"`
}

// ============================================================================
// USER SUMMARY DTO (referenced in role responses)
// ============================================================================

// UserSummaryDTO represents minimal user information
type UserSummaryDTO struct {
	UserID   uuid.UUID `json:"user_id"`
	UserName string    `json:"user_name"`
	FullName string    `json:"full_name"`
	Email    string    `json:"email"`
	IsActive bool      `json:"is_active"`
}

// ============================================================================
// PRE-DEFINED ROLE TEMPLATES
// ============================================================================

// RoleTemplate represents a pre-defined role template
type RoleTemplate struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// GetRoleTemplates returns common role templates for restaurant management
func GetRoleTemplates() []RoleTemplate {
	return []RoleTemplate{
		{
			Name:        "super_admin",
			Description: "Full system access - can manage tenants, organizations, and all resources",
			Permissions: []string{
				"tenant.create", "tenant.read", "tenant.update", "tenant.delete",
				"organization.create", "organization.read", "organization.update", "organization.delete",
				"branch.create", "branch.read", "branch.update", "branch.delete",
				"restaurant.create", "restaurant.read", "restaurant.update", "restaurant.delete",
				"user.create", "user.read", "user.update", "user.delete",
				"role.create", "role.read", "role.update", "role.delete",
				"order.create", "order.read", "order.update", "order.delete",
				"menu.create", "menu.read", "menu.update", "menu.delete",
				"inventory.create", "inventory.read", "inventory.update", "inventory.delete",
				"reports.read", "settings.manage",
			},
		},
		{
			Name:        "admin",
			Description: "Tenant-level admin - can manage organizations, branches, and restaurants within tenant",
			Permissions: []string{
				"organization.create", "organization.read", "organization.update", "organization.delete",
				"branch.create", "branch.read", "branch.update", "branch.delete",
				"restaurant.create", "restaurant.read", "restaurant.update", "restaurant.delete",
				"user.create", "user.read", "user.update",
				"role.read", "order.read", "menu.read", "inventory.read",
				"reports.read", "settings.manage",
			},
		},
		{
			Name:        "owner",
			Description: "Restaurant owner - full control over assigned restaurant(s)",
			Permissions: []string{
				"restaurant.read", "restaurant.update",
				"user.create", "user.read", "user.update",
				"employee.create", "employee.read", "employee.update", "employee.delete",
				"order.create", "order.read", "order.update", "order.delete",
				"menu.create", "menu.read", "menu.update", "menu.delete",
				"inventory.create", "inventory.read", "inventory.update", "inventory.delete",
				"table.create", "table.read", "table.update", "table.delete",
				"reports.read", "settings.manage",
			},
		},
		{
			Name:        "manager",
			Description: "Restaurant manager - day-to-day operations management",
			Permissions: []string{
				"restaurant.read",
				"employee.read", "employee.update",
				"order.create", "order.read", "order.update", "order.delete",
				"menu.read", "menu.update",
				"inventory.read", "inventory.update",
				"table.create", "table.read", "table.update", "table.delete",
				"reports.read",
			},
		},
		{
			Name:        "assistant_manager",
			Description: "Assistant manager - supports manager with limited permissions",
			Permissions: []string{
				"restaurant.read",
				"employee.read",
				"order.create", "order.read", "order.update",
				"menu.read",
				"inventory.read", "inventory.update",
				"table.read", "table.update",
				"reports.read",
			},
		},
		{
			Name:        "cashier",
			Description: "Cashier - handles orders and payments",
			Permissions: []string{
				"order.create", "order.read", "order.update",
				"payment.create", "payment.read",
				"menu.read",
				"customer.create", "customer.read",
			},
		},
		{
			Name:        "waiter",
			Description: "Waiter/Server - takes orders and manages tables",
			Permissions: []string{
				"order.create", "order.read", "order.update",
				"menu.read",
				"table.read", "table.update",
				"customer.read",
			},
		},
		{
			Name:        "chef",
			Description: "Kitchen staff - views and updates order status",
			Permissions: []string{
				"order.read", "order.update",
				"menu.read",
				"inventory.read",
				"kot.read", "kot.update",
			},
		},
		{
			Name:        "inventory_manager",
			Description: "Manages inventory and purchase orders",
			Permissions: []string{
				"inventory.create", "inventory.read", "inventory.update", "inventory.delete",
				"vendor.create", "vendor.read", "vendor.update",
				"purchase_order.create", "purchase_order.read", "purchase_order.update",
				"reports.read",
			},
		},
		{
			Name:        "viewer",
			Description: "Read-only access for reporting and analytics",
			Permissions: []string{
				"restaurant.read",
				"order.read",
				"menu.read",
				"inventory.read",
				"reports.read",
			},
		},
	}
}

// CreateRoleFromTemplateRequest creates a role from a template
type CreateRoleFromTemplateRequest struct {
	TemplateName string `json:"template_name" binding:"required,oneof=super_admin admin owner manager assistant_manager cashier waiter chef inventory_manager viewer"`
	CustomName   string `json:"custom_name" binding:"omitempty,max=255"` // Optional: override default name
}

// ============================================================================
// VALIDATION HELPERS
// ============================================================================

// Validate validates the CreateRoleRequest
func (r *CreateRoleRequest) Validate() error {
	// Add custom validation logic here if needed
	return nil
}

// Validate validates the AssignRoleToUserRequest
func (r *AssignRoleToUserRequest) Validate() error {
	// Add custom validation logic here if needed
	return nil
}
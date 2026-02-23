package DTO

import (
	"errors"
	"github.com/google/uuid"
)

// User roles constants (same as before)
const (
	RoleSuperAdmin       = "super_admin"
	RoleAdmin            = "admin"
	RoleOwner            = "owner"
	RoleManager          = "manager"
	RoleAssistantManager = "assistant_manager"
	RoleEmployee         = "employee"
	RoleWaiter           = "waiter"
	RoleCashier          = "cashier"
	RoleChef             = "chef"
	RoleInventoryManager = "inventory_manager"
	RoleViewer           = "viewer"
)

// Authorization errors
var (
	ErrUnauthorized                = errors.New("user is not authorized to perform this action")
	ErrInsufficientPermissions     = errors.New("insufficient permissions")
	ErrRestaurantAccessDenied      = errors.New("access denied to this restaurant")
	ErrSuperAdminRequired          = errors.New("super admin role required")
	ErrAdminRequired               = errors.New("admin or super admin role required")
	ErrManagementRoleRequired      = errors.New("management role required (owner/manager/assistant_manager)")
	ErrRestaurantOwnershipRequired = errors.New("user must be owner or manager of the restaurant")
	ErrSessionExpired              = errors.New("session has expired")
	ErrSessionRevoked              = errors.New("session has been revoked")
	ErrInvalidSession              = errors.New("invalid session")
	ErrPermissionDenied            = errors.New("permission denied for this action")
)

// AuthContext represents the authenticated user's context
// This can be populated from Redis cached session or JWT
type AuthContext struct {
	SessionID    uuid.UUID
	UserID       uuid.UUID
	TenantID     uuid.UUID
	Email        string
	FullName     string
	Role         string      // Default role name
	RoleIDs      []uuid.UUID // All role IDs
	Permissions  []string    // Flattened permissions list
	RestaurantID *uuid.UUID  // The restaurant the user is associated with (if any)
	BranchID     *uuid.UUID  // The branch the user is associated with (if any)
	IPAddress    string
	UserAgent    string
}

// NewAuthContextFromCache creates AuthContext from CachedUserSession
func NewAuthContextFromCache(session *CachedUserSession) *AuthContext {
	return &AuthContext{
		SessionID:    session.SessionID,
		UserID:       session.UserID,
		TenantID:     session.TenantID,
		Email:        session.Email,
		FullName:     session.FullName,
		Role:         session.Role,
		RoleIDs:      session.RoleIDs,
		Permissions:  session.Permissions,
		RestaurantID: session.RestaurantID,
		BranchID:     session.BranchID,
		IPAddress:    session.IPAddress,
		UserAgent:    session.UserAgent,
	}
}

// ============================================================================
// PERMISSION-BASED AUTHORIZATION
// ============================================================================

// HasPermission checks if user has a specific permission
func (ctx *AuthContext) HasPermission(permission string) bool {
	// Super admin has all permissions
	if ctx.Role == RoleSuperAdmin {
		return true
	}
	
	for _, p := range ctx.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// HasAnyPermission checks if user has any of the specified permissions
func (ctx *AuthContext) HasAnyPermission(permissions ...string) bool {
	for _, permission := range permissions {
		if ctx.HasPermission(permission) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks if user has all of the specified permissions
func (ctx *AuthContext) HasAllPermissions(permissions ...string) bool {
	for _, permission := range permissions {
		if !ctx.HasPermission(permission) {
			return false
		}
	}
	return true
}

// ============================================================================
// ROLE-BASED AUTHORIZATION
// ============================================================================

// HasRole checks if user has a specific role
func (ctx *AuthContext) HasRole(roleName string) bool {
	if ctx.Role == roleName {
		return true
	}
	// You can extend this to check RoleIDs if you have role name mapping
	return false
}

// HasAnyRole checks if user has any of the specified roles
func (ctx *AuthContext) HasAnyRole(roles ...string) bool {
	for _, role := range roles {
		if ctx.HasRole(role) {
			return true
		}
	}
	return false
}

// IsAdmin checks if user is admin or super admin
func (ctx *AuthContext) IsAdmin() bool {
	return ctx.Role == RoleSuperAdmin || ctx.Role == RoleAdmin
}

// IsManagement checks if user has management role
func (ctx *AuthContext) IsManagement() bool {
	managementRoles := []string{RoleSuperAdmin, RoleAdmin, RoleOwner, RoleManager, RoleAssistantManager}
	for _, role := range managementRoles {
		if ctx.Role == role {
			return true
		}
	}
	return false
}

// ============================================================================
// RESOURCE ACCESS AUTHORIZATION
// ============================================================================

// HasRestaurantAccess checks if user has access to a specific restaurant
func (ctx *AuthContext) HasRestaurantAccess(restaurantID uuid.UUID) bool {
	// Admins have access to all restaurants
	if ctx.IsAdmin() {
		return true
	}
	
	// Check if user belongs to this restaurant
	return ctx.RestaurantID != nil && *ctx.RestaurantID == restaurantID
}

// HasBranchAccess checks if user has access to a specific branch
func (ctx *AuthContext) HasBranchAccess(branchID uuid.UUID) bool {
	// Admins have access to all branches
	if ctx.IsAdmin() {
		return true
	}
	
	// Check if user belongs to this branch
	return ctx.BranchID != nil && *ctx.BranchID == branchID
}

// ============================================================================
// TENANT OPERATIONS AUTHORIZATION
// ============================================================================

// CanManageTenant checks if user can manage tenant settings
func (ctx *AuthContext) CanManageTenant() error {
	if !ctx.HasPermission("tenant.manage") && !ctx.IsAdmin() {
		return ErrSuperAdminRequired
	}
	return nil
}

// CanViewTenant checks if user can view tenant information
func (ctx *AuthContext) CanViewTenant(tenantID uuid.UUID) error {
	// Users can view their own tenant
	if ctx.TenantID == tenantID {
		return nil
	}
	
	// Super admins can view all tenants
	if ctx.Role == RoleSuperAdmin {
		return nil
	}
	
	return ErrUnauthorized
}

// ============================================================================
// ORGANIZATION & BRANCH AUTHORIZATION
// ============================================================================

// CanCreateOrganization checks if user can create organizations
func (ctx *AuthContext) CanCreateOrganization() error {
	if !ctx.HasPermission("organization.create") && !ctx.IsAdmin() {
		return ErrAdminRequired
	}
	return nil
}

// CanUpdateOrganization checks if user can update an organization
func (ctx *AuthContext) CanUpdateOrganization(organizationID uuid.UUID) error {
	if ctx.HasPermission("organization.update") || ctx.IsAdmin() {
		return nil
	}
	return ErrInsufficientPermissions
}

// CanCreateBranch checks if user can create branches
func (ctx *AuthContext) CanCreateBranch() error {
	if !ctx.HasPermission("branch.create") && !ctx.IsAdmin() {
		return ErrAdminRequired
	}
	return nil
}

// CanUpdateBranch checks if user can update a branch
func (ctx *AuthContext) CanUpdateBranch(branchID uuid.UUID) error {
	if ctx.IsAdmin() || ctx.HasPermission("branch.update") {
		return nil
	}
	
	// Branch managers can update their own branch
	if ctx.BranchID != nil && *ctx.BranchID == branchID && ctx.IsManagement() {
		return nil
	}
	
	return ErrInsufficientPermissions
}

// ============================================================================
// RESTAURANT AUTHORIZATION (Same as before, but permission-aware)
// ============================================================================

// CanCreateRestaurant checks if user can create a new restaurant
func (ctx *AuthContext) CanCreateRestaurant() error {
	if ctx.HasPermission("restaurant.create") || ctx.IsAdmin() {
		return nil
	}
	return ErrAdminRequired
}

// CanUpdateRestaurant checks if user can update a restaurant
func (ctx *AuthContext) CanUpdateRestaurant(restaurantID uuid.UUID) error {
	if ctx.IsAdmin() || ctx.HasPermission("restaurant.update") {
		return nil
	}
	
	// Owner and manager can update their own restaurant
	if (ctx.Role == RoleOwner || ctx.Role == RoleManager) && 
	   ctx.RestaurantID != nil && *ctx.RestaurantID == restaurantID {
		return nil
	}
	
	return ErrRestaurantAccessDenied
}

// CanDeleteRestaurant checks if user can delete a restaurant
func (ctx *AuthContext) CanDeleteRestaurant(restaurantID uuid.UUID) error {
	if ctx.HasPermission("restaurant.delete") || ctx.Role == RoleSuperAdmin {
		return nil
	}
	return ErrSuperAdminRequired
}

// CanViewRestaurant checks if user can view a restaurant
func (ctx *AuthContext) CanViewRestaurant(restaurantID uuid.UUID) error {
	if ctx.HasPermission("restaurant.read") || ctx.HasRestaurantAccess(restaurantID) {
		return nil
	}
	return ErrRestaurantAccessDenied
}

// ============================================================================
// TABLE AUTHORIZATION
// ============================================================================

// CanCreateTable checks if user can create a table for a specific restaurant
func (ctx *AuthContext) CanCreateTable(restaurantID uuid.UUID) error {
	if ctx.IsAdmin() || ctx.HasPermission("table.create") {
		return nil
	}
	
	if (ctx.Role == RoleOwner || ctx.Role == RoleManager || ctx.Role == RoleAssistantManager) &&
	   ctx.RestaurantID != nil && *ctx.RestaurantID == restaurantID {
		return nil
	}
	
	return ErrManagementRoleRequired
}

// CanUpdateTable checks if user can update a table
func (ctx *AuthContext) CanUpdateTable(restaurantID uuid.UUID) error {
	if ctx.IsAdmin() || ctx.HasPermission("table.update") {
		return nil
	}
	
	if ctx.IsManagement() && ctx.RestaurantID != nil && *ctx.RestaurantID == restaurantID {
		return nil
	}
	
	return ErrManagementRoleRequired
}

// CanUpdateTableStatus checks if user can update table status (more permissive)
func (ctx *AuthContext) CanUpdateTableStatus(restaurantID uuid.UUID) error {
	if ctx.IsAdmin() {
		return nil
	}
	
	// Management roles + waiters can update table status in their restaurant
	allowedRoles := []string{RoleOwner, RoleManager, RoleAssistantManager, RoleWaiter}
	if ctx.RestaurantID != nil && *ctx.RestaurantID == restaurantID {
		for _, role := range allowedRoles {
			if ctx.Role == role {
				return nil
			}
		}
	}
	
	return ErrRestaurantAccessDenied
}

// CanDeleteTable checks if user can delete a table
func (ctx *AuthContext) CanDeleteTable(restaurantID uuid.UUID) error {
	if ctx.IsAdmin() || ctx.HasPermission("table.delete") {
		return nil
	}
	
	if (ctx.Role == RoleOwner || ctx.Role == RoleManager) &&
	   ctx.RestaurantID != nil && *ctx.RestaurantID == restaurantID {
		return nil
	}
	
	return ErrRestaurantOwnershipRequired
}

// ============================================================================
// ORDER AUTHORIZATION
// ============================================================================

// CanCreateOrder checks if user can create an order
func (ctx *AuthContext) CanCreateOrder(restaurantID uuid.UUID) error {
	if ctx.IsAdmin() || ctx.HasPermission("order.create") {
		return nil
	}
	
	// Any employee of the restaurant can create orders
	if ctx.RestaurantID != nil && *ctx.RestaurantID == restaurantID {
		return nil
	}
	
	return ErrRestaurantAccessDenied
}

// CanUpdateOrder checks if user can update an order
func (ctx *AuthContext) CanUpdateOrder(restaurantID uuid.UUID) error {
	if ctx.IsAdmin() || ctx.HasPermission("order.update") {
		return nil
	}
	
	if ctx.RestaurantID != nil && *ctx.RestaurantID == restaurantID {
		return nil
	}
	
	return ErrRestaurantAccessDenied
}

// CanDeleteOrder checks if user can delete/cancel an order
func (ctx *AuthContext) CanDeleteOrder(restaurantID uuid.UUID) error {
	if ctx.IsAdmin() || ctx.HasPermission("order.delete") {
		return nil
	}
	
	if ctx.IsManagement() && ctx.RestaurantID != nil && *ctx.RestaurantID == restaurantID {
		return nil
	}
	
	return ErrManagementRoleRequired
}

// CanViewOrder checks if user can view an order
func (ctx *AuthContext) CanViewOrder(restaurantID uuid.UUID) error {
	if ctx.HasPermission("order.read") || ctx.HasRestaurantAccess(restaurantID) {
		return nil
	}
	return ErrRestaurantAccessDenied
}

// ============================================================================
// MENU AUTHORIZATION
// ============================================================================

// CanManageMenu checks if user can manage menu (create/update/delete)
func (ctx *AuthContext) CanManageMenu(restaurantID uuid.UUID) error {
	if ctx.IsAdmin() || ctx.HasAnyPermission("menu.create", "menu.update", "menu.delete") {
		return nil
	}
	
	if (ctx.Role == RoleOwner || ctx.Role == RoleManager) &&
	   ctx.RestaurantID != nil && *ctx.RestaurantID == restaurantID {
		return nil
	}
	
	return ErrManagementRoleRequired
}

// CanViewMenu checks if user can view menu
func (ctx *AuthContext) CanViewMenu(restaurantID uuid.UUID) error {
	// Most roles can view menu
	if ctx.HasPermission("menu.read") || ctx.HasRestaurantAccess(restaurantID) {
		return nil
	}
	return ErrRestaurantAccessDenied
}

// ============================================================================
// INVENTORY AUTHORIZATION
// ============================================================================

// CanManageInventory checks if user can manage inventory
func (ctx *AuthContext) CanManageInventory(restaurantID uuid.UUID) error {
	if ctx.IsAdmin() || ctx.HasAnyPermission("inventory.create", "inventory.update", "inventory.delete") {
		return nil
	}
	
	if (ctx.Role == RoleOwner || ctx.Role == RoleManager || ctx.Role == RoleInventoryManager) &&
	   ctx.RestaurantID != nil && *ctx.RestaurantID == restaurantID {
		return nil
	}
	
	return ErrInsufficientPermissions
}

// ============================================================================
// USER & ROLE MANAGEMENT AUTHORIZATION
// ============================================================================

// CanManageUsers checks if user can manage other users
func (ctx *AuthContext) CanManageUsers() error {
	if ctx.IsAdmin() || ctx.HasAnyPermission("user.create", "user.update", "user.delete") {
		return nil
	}
	return ErrAdminRequired
}

// CanManageRoles checks if user can manage roles
func (ctx *AuthContext) CanManageRoles() error {
	if ctx.IsAdmin() || ctx.HasAnyPermission("role.create", "role.update", "role.delete") {
		return nil
	}
	return ErrAdminRequired
}

// CanAssignRoles checks if user can assign roles to others
func (ctx *AuthContext) CanAssignRoles() error {
	if ctx.IsAdmin() || ctx.HasPermission("role.assign") {
		return nil
	}
	return ErrAdminRequired
}

// ============================================================================
// REPORTS & ANALYTICS AUTHORIZATION
// ============================================================================

// CanViewReports checks if user can view reports
func (ctx *AuthContext) CanViewReports(restaurantID uuid.UUID) error {
	if ctx.HasPermission("reports.read") {
		return nil
	}
	
	if ctx.IsManagement() && ctx.RestaurantID != nil && *ctx.RestaurantID == restaurantID {
		return nil
	}
	
	return ErrInsufficientPermissions
}

// ============================================================================
// VALIDATION WITH AUTHORIZATION HELPERS
// ============================================================================

// ValidateAndAuthorizeCreateRestaurant validates and authorizes restaurant creation
func ValidateAndAuthorizeCreateRestaurant(ctx *AuthContext, req *CreateRestaurantRequest) error {
	if err := ctx.CanCreateRestaurant(); err != nil {
		return err
	}
	
	if err := req.Validate(); err != nil {
		return err
	}
	
	return nil
}

// ValidateAndAuthorizeCreateTable validates and authorizes table creation
func ValidateAndAuthorizeCreateTable(ctx *AuthContext, req *CreateTableRequest, restaurantID uuid.UUID) error {
	if err := ctx.CanCreateTable(restaurantID); err != nil {
		return err
	}
	
	if err := req.Validate(); err != nil {
		return err
	}
	
	return nil
}
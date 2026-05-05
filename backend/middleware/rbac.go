package middleware

import (
	"backend/DTO"
	"backend/utils"
	"encoding/json"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// ============================================================================
// RBAC (ROLE & PERMISSION ENFORCEMENT) MIDDLEWARE
// Composable middleware functions that read the AuthContext from locals and
// enforce role/permission checks. Logs denied attempts as RBAC audit events
// with severity classification. Supports both simple checks and complex
// composable policies via AuthCheck functions.
// ============================================================================

// AuthCheck is a function that returns true if the check passes.
type AuthCheck func(auth *DTO.AuthContext) bool

// RequireRole returns middleware that requires the user to have at least
// one of the specified roles.
func RequireRole(roles ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		auth := GetAuthContext(c)
		if auth == nil {
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Authentication required", nil)
		}

		if auth.HasAnyRole(roles...) {
			return c.Next()
		}

		logRBACDenied(c, auth, "role_required", map[string]interface{}{
			"required_roles": roles,
			"user_role":      auth.Role,
		})

		return utils.SendResponse(c, fiber.StatusForbidden,
			"Insufficient role privileges", map[string]interface{}{
				"required": roles,
				"current":  auth.Role,
			})
	}
}

// RequirePermission returns middleware that requires the user to have
// permission to perform the given action on the given resource.
func RequirePermission(resource, action string) fiber.Handler {
	permStr := resource + "." + action

	return func(c fiber.Ctx) error {
		auth := GetAuthContext(c)
		if auth == nil {
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Authentication required", nil)
		}

		if auth.HasPermission(permStr) {
			return c.Next()
		}

		logRBACDenied(c, auth, "permission_denied", map[string]interface{}{
			"required_permission": permStr,
			"resource":            resource,
			"action":              action,
		})

		return utils.SendResponse(c, fiber.StatusForbidden,
			"You do not have permission to perform this action", map[string]interface{}{
				"required": permStr,
			})
	}
}

// RequireAnyPermission returns middleware that requires the user to have
// at least one of the specified permissions.
func RequireAnyPermission(permissions ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		auth := GetAuthContext(c)
		if auth == nil {
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Authentication required", nil)
		}

		if auth.HasAnyPermission(permissions...) {
			return c.Next()
		}

		logRBACDenied(c, auth, "permissions_denied", map[string]interface{}{
			"required_any": permissions,
		})

		return utils.SendResponse(c, fiber.StatusForbidden,
			"You do not have the required permissions", map[string]interface{}{
				"required_any": permissions,
			})
	}
}

// RequireAllPermissions returns middleware that requires the user to have
// ALL of the specified permissions.
func RequireAllPermissions(permissions ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		auth := GetAuthContext(c)
		if auth == nil {
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Authentication required", nil)
		}

		if auth.HasAllPermissions(permissions...) {
			return c.Next()
		}

		logRBACDenied(c, auth, "permissions_missing", map[string]interface{}{
			"required_all": permissions,
		})

		return utils.SendResponse(c, fiber.StatusForbidden,
			"You do not have all required permissions", map[string]interface{}{
				"required_all": permissions,
			})
	}
}

// RequireAny returns middleware that passes if ANY of the provided checks pass.
// This enables composable OR-logic:
//
//	RequireAny(
//	    RoleCheck("admin", "super_admin"),
//	    PermissionCheck("order.create"),
//	)
func RequireAny(checks ...AuthCheck) fiber.Handler {
	return func(c fiber.Ctx) error {
		auth := GetAuthContext(c)
		if auth == nil {
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Authentication required", nil)
		}

		for _, check := range checks {
			if check(auth) {
				return c.Next()
			}
		}

		logRBACDenied(c, auth, "composite_check_failed", map[string]interface{}{
			"check_count": len(checks),
			"logic":       "OR",
		})

		return utils.SendResponse(c, fiber.StatusForbidden,
			"Access denied", nil)
	}
}

// RequireAll returns middleware that passes only if ALL provided checks pass.
func RequireAll(checks ...AuthCheck) fiber.Handler {
	return func(c fiber.Ctx) error {
		auth := GetAuthContext(c)
		if auth == nil {
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Authentication required", nil)
		}

		for _, check := range checks {
			if !check(auth) {
				logRBACDenied(c, auth, "composite_check_failed", map[string]interface{}{
					"check_count": len(checks),
					"logic":       "AND",
				})
				return utils.SendResponse(c, fiber.StatusForbidden,
					"Access denied", nil)
			}
		}

		return c.Next()
	}
}

// ── AuthCheck Constructors ─────────────────────────────────────────────────

// RoleCheck returns an AuthCheck that passes if the user has any of the roles.
func RoleCheck(roles ...string) AuthCheck {
	return func(auth *DTO.AuthContext) bool {
		return auth.HasAnyRole(roles...)
	}
}

// PermissionCheck returns an AuthCheck that passes if the user has the permission.
func PermissionCheck(permission string) AuthCheck {
	return func(auth *DTO.AuthContext) bool {
		return auth.HasPermission(permission)
	}
}

// AdminCheck returns an AuthCheck that passes if the user is admin/super_admin.
func AdminCheck() AuthCheck {
	return func(auth *DTO.AuthContext) bool {
		return auth.IsAdmin()
	}
}

// ManagementCheck returns an AuthCheck that passes if the user has management role.
func ManagementCheck() AuthCheck {
	return func(auth *DTO.AuthContext) bool {
		return auth.IsManagement()
	}
}

// RequireReportsAccess returns middleware that validates the user can view reports
// for the restaurant_id query parameter. Uses CanViewReports() for proper authorization.
func RequireReportsAccess() fiber.Handler {
	return func(c fiber.Ctx) error {
		auth := GetAuthContext(c)
		if auth == nil {
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Authentication required", nil)
		}

		// Get restaurant_id from query parameter
		restaurantIDStr := c.Query("restaurant_id")
		if restaurantIDStr == "" {
			// If no restaurant_id provided, user must have global reports.read permission
			if !auth.HasPermission("reports.read") {
				logRBACDenied(c, auth, "reports_access_denied", map[string]interface{}{
					"reason": "restaurant_id required for non-admin users",
				})
				return utils.SendResponse(c, fiber.StatusForbidden,
					"restaurant_id parameter is required", nil)
			}
			return c.Next()
		}

		// Parse and validate restaurant_id
		restaurantID, err := uuid.Parse(restaurantIDStr)
		if err != nil {
			return utils.SendResponse(c, fiber.StatusBadRequest,
				"Invalid restaurant_id format", nil)
		}

		// Use CanViewReports to validate access
		if err := auth.CanViewReports(restaurantID); err != nil {
			logRBACDenied(c, auth, "reports_access_denied", map[string]interface{}{
				"restaurant_id": restaurantID.String(),
				"reason":        err.Error(),
			})
			return utils.SendResponse(c, fiber.StatusForbidden,
				"You do not have access to reports for this restaurant", nil)
		}

		return c.Next()
	}
}

// RequireInventoryAccess returns middleware that validates the user can manage inventory
// for the restaurant_id query parameter. Uses CanManageInventory() for proper authorization.
func RequireInventoryAccess() fiber.Handler {
	return func(c fiber.Ctx) error {
		auth := GetAuthContext(c)
		if auth == nil {
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Authentication required", nil)
		}

		// Get restaurant_id from query parameter
		restaurantIDStr := c.Query("restaurant_id")
		if restaurantIDStr == "" {
			// If no restaurant_id provided, user must have global inventory permissions
			if !auth.HasAnyPermission("inventory.create", "inventory.update", "inventory.delete") {
				logRBACDenied(c, auth, "inventory_access_denied", map[string]interface{}{
					"reason": "restaurant_id required for non-admin users",
				})
				return utils.SendResponse(c, fiber.StatusForbidden,
					"restaurant_id parameter is required", nil)
			}
			return c.Next()
		}

		// Parse and validate restaurant_id
		restaurantID, err := uuid.Parse(restaurantIDStr)
		if err != nil {
			return utils.SendResponse(c, fiber.StatusBadRequest,
				"Invalid restaurant_id format", nil)
		}

		// Use CanManageInventory to validate access
		if err := auth.CanManageInventory(restaurantID); err != nil {
			logRBACDenied(c, auth, "inventory_access_denied", map[string]interface{}{
				"restaurant_id": restaurantID.String(),
				"reason":        err.Error(),
			})
			return utils.SendResponse(c, fiber.StatusForbidden,
				"You do not have access to manage inventory for this restaurant", nil)
		}

		return c.Next()
	}
}

// RequireMenuAccess returns middleware that validates the user can manage menu
// for the restaurant_id query parameter. Uses CanManageMenu() for proper authorization.
func RequireMenuAccess() fiber.Handler {
	return func(c fiber.Ctx) error {
		auth := GetAuthContext(c)
		if auth == nil {
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Authentication required", nil)
		}

		// Get restaurant_id from query parameter
		restaurantIDStr := c.Query("restaurant_id")
		if restaurantIDStr == "" {
			// If no restaurant_id provided, user must have global menu permissions
			if !auth.HasAnyPermission("menu.create", "menu.update", "menu.delete") {
				logRBACDenied(c, auth, "menu_access_denied", map[string]interface{}{
					"reason": "restaurant_id required for non-admin users",
				})
				return utils.SendResponse(c, fiber.StatusForbidden,
					"restaurant_id parameter is required", nil)
			}
			return c.Next()
		}

		// Parse and validate restaurant_id
		restaurantID, err := uuid.Parse(restaurantIDStr)
		if err != nil {
			return utils.SendResponse(c, fiber.StatusBadRequest,
				"Invalid restaurant_id format", nil)
		}

		// Use CanManageMenu to validate access
		if err := auth.CanManageMenu(restaurantID); err != nil {
			logRBACDenied(c, auth, "menu_access_denied", map[string]interface{}{
				"restaurant_id": restaurantID.String(),
				"reason":        err.Error(),
			})
			return utils.SendResponse(c, fiber.StatusForbidden,
				"You do not have access to manage menu for this restaurant", nil)
		}

		return c.Next()
	}
}

// RequireAdmin is a convenience middleware that requires admin or super_admin.
func RequireAdmin() fiber.Handler {
	return RequireRole(DTO.RoleSuperAdmin, DTO.RoleAdmin)
}

// RequireManagement is a convenience middleware for management-level access.
func RequireManagement() fiber.Handler {
	return RequireRole(
		DTO.RoleSuperAdmin, DTO.RoleAdmin,
		DTO.RoleOwner, DTO.RoleManager, DTO.RoleAssistantManager,
	)
}

// ── RBAC Audit Logging ─────────────────────────────────────────────────────

// logRBACDenied logs an RBAC denial event with full context.
func logRBACDenied(c fiber.Ctx, auth *DTO.AuthContext, eventType string, metadata map[string]interface{}) {
	// Detect privilege escalation attempts
	severity := "warning"
	if eventType == "permission_denied" {
		// If a non-admin tries to access admin resources, escalate severity
		if metadata != nil {
			if resource, ok := metadata["resource"].(string); ok {
				criticalResources := map[string]bool{
					"*": true, "user": true, "role": true,
					"policy": true, "tenant": true, "security": true,
				}
				if criticalResources[resource] && !auth.IsAdmin() {
					severity = "critical"
					eventType = "privilege_escalation_attempt"
				}
			}
		}
	}

	entry := map[string]interface{}{
		"level":      "WARN",
		"event":      eventType,
		"severity":   severity,
		"user_id":    auth.UserID.String(),
		"tenant_id":  auth.TenantID.String(),
		"role":       auth.Role,
		"path":       c.Path(),
		"method":     c.Method(),
		"ip":         GetRealIP(c),
		"request_id": GetRequestID(c),
		"metadata":   metadata,
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
	}

	logJSON, _ := json.Marshal(entry)
	log.Printf("[RBAC] %s", string(logJSON))
}

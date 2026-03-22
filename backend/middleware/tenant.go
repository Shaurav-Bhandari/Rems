package middleware

import (
	"backend/DB"
	"backend/DTO"
	"backend/models"
	"backend/utils"
	"encoding/json"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// ============================================================================
// MULTI-TENANT ISOLATION MIDDLEWARE
// Ensures every authenticated request is scoped to the tenant from
// AuthContext.TenantID. Validates tenant existence and active status,
// resolves tenant metadata (name, subscription tier), and rejects
// cross-tenant access attempts with audit logging.
// ============================================================================

const (
	// LocalsTenantKey stores the resolved TenantContext in locals.
	LocalsTenantKey = "tenantCtx"
)

// TenantContext holds resolved tenant metadata for the current request.
type TenantContext struct {
	TenantID uuid.UUID `json:"tenant_id"`
	Name     string    `json:"name"`
	Domain   string    `json:"domain"`
	IsActive bool      `json:"is_active"`
}

// TenantIsolationConfig configures the tenant isolation middleware.
type TenantIsolationConfig struct {
	// SkipPaths are routes that bypass tenant validation (e.g. super-admin panel).
	SkipPaths []string

	// AllowCrossTenantRoles are roles that can access resources across tenants
	// (typically only super_admin).
	AllowCrossTenantRoles []string

	// EnforceTenantHeader requires X-Tenant-ID header even for authenticated routes.
	// When true, the header is cross-checked against the JWT tenant.
	EnforceTenantHeader bool
}

// DefaultTenantIsolationConfig returns production defaults.
func DefaultTenantIsolationConfig() TenantIsolationConfig {
	return TenantIsolationConfig{
		SkipPaths: []string{
			"/api/v1/auth",
			"/api/v1/health",
			"/api/v1/ping",
		},
		AllowCrossTenantRoles: []string{DTO.RoleSuperAdmin},
		EnforceTenantHeader:   false,
	}
}

// TenantIsolation returns the tenant isolation middleware with default config.
func TenantIsolation() fiber.Handler {
	return TenantIsolationWithConfig(DefaultTenantIsolationConfig())
}

// TenantIsolationWithConfig returns tenant isolation middleware with custom config.
func TenantIsolationWithConfig(cfg TenantIsolationConfig) fiber.Handler {
	skipSet := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skipSet[p] = struct{}{}
	}

	crossTenantSet := make(map[string]struct{}, len(cfg.AllowCrossTenantRoles))
	for _, r := range cfg.AllowCrossTenantRoles {
		crossTenantSet[r] = struct{}{}
	}

	return func(c fiber.Ctx) error {
		path := c.Path()

		// Skip for configured paths
		for skip := range skipSet {
			if pathHasPrefix(path, skip) {
				return c.Next()
			}
		}

		// ── Get auth context ───────────────────────────────────
		auth := GetAuthContext(c)
		if auth == nil {
			// No auth context = unauthenticated; let the auth middleware handle it
			return c.Next()
		}

		// ── Validate tenant ID ─────────────────────────────────
		if auth.TenantID == uuid.Nil {
			logTenantViolation(c, auth, "missing_tenant_id", "Token has no tenant_id")
			return utils.SendResponse(c, fiber.StatusForbidden,
				"Tenant context is required", nil)
		}

		// ── Cross-check X-Tenant-ID header if enforced ─────────
		if cfg.EnforceTenantHeader {
			headerTID := c.Get("X-Tenant-ID")
			if headerTID != "" {
				parsedTID, err := uuid.Parse(headerTID)
				if err != nil {
					return utils.SendResponse(c, fiber.StatusBadRequest,
						"Invalid X-Tenant-ID header", nil)
				}

				if parsedTID != auth.TenantID {
					// Check if user has cross-tenant access
					if _, allowed := crossTenantSet[auth.Role]; !allowed {
						logTenantViolation(c, auth, "cross_tenant_attempt",
							"Header tenant_id does not match token tenant_id")
						return utils.SendResponse(c, fiber.StatusForbidden,
							"Cross-tenant access denied", nil)
					}
				}
			}
		}

		// ── Resolve tenant from database ───────────────────────
		tenantCtx, err := resolveTenant(auth.TenantID)
		if err != nil {
			logTenantViolation(c, auth, "tenant_not_found", err.Error())
			return utils.SendResponse(c, fiber.StatusForbidden,
				"Tenant not found or inactive", nil)
		}

		// ── Check tenant is active ─────────────────────────────
		if !tenantCtx.IsActive {
			// Super admins can still access inactive tenants
			if _, allowed := crossTenantSet[auth.Role]; !allowed {
				logTenantViolation(c, auth, "tenant_inactive",
					"Tenant is deactivated")
				return utils.SendResponse(c, fiber.StatusForbidden,
					"Your organization's account has been deactivated. Contact support.", nil)
			}
		}

		// ── Inject tenant context into locals ──────────────────
		c.Locals(LocalsTenantKey, tenantCtx)

		return c.Next()
	}
}

// resolveTenant looks up a tenant by ID from the database.
func resolveTenant(tenantID uuid.UUID) (*TenantContext, error) {
	if DB.DB == nil {
		// Database not initialized — return minimal context
		return &TenantContext{
			TenantID: tenantID,
			IsActive: true,
		}, nil
	}

	var tenant models.Tenant
	if err := DB.DB.Where("tenant_id = ?", tenantID).First(&tenant).Error; err != nil {
		return nil, err
	}

	return &TenantContext{
		TenantID: tenant.TenantID,
		Name:     tenant.Name,
		Domain:   tenant.Domain,
		IsActive: tenant.IsActive,
	}, nil
}

// GetTenantContext extracts the TenantContext from fiber locals.
func GetTenantContext(c fiber.Ctx) *TenantContext {
	if tc, ok := c.Locals(LocalsTenantKey).(*TenantContext); ok {
		return tc
	}
	return nil
}

// logTenantViolation logs a tenant isolation violation.
func logTenantViolation(c fiber.Ctx, auth *DTO.AuthContext, eventType string, detail string) {
	entry := map[string]interface{}{
		"level":      "WARN",
		"event":      "tenant_violation",
		"type":       eventType,
		"detail":     detail,
		"user_id":    auth.UserID.String(),
		"tenant_id":  auth.TenantID.String(),
		"role":       auth.Role,
		"path":       c.Path(),
		"method":     c.Method(),
		"ip":         GetRealIP(c),
		"request_id": GetRequestID(c),
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
	}
	logJSON, _ := json.Marshal(entry)
	log.Printf("[TENANT] %s", string(logJSON))
}

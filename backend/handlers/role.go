package handlers

import (
	"backend/DTO"
	"backend/middleware"
	"backend/models"
	services "backend/services/core"
	"backend/utils"
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ============================================================================
// ROLE HANDLER — Admin-only role & user-role management
// ============================================================================

type RoleHandler struct {
	db   *gorm.DB
	rbac *services.RBACService
}

func NewRoleHandler(db *gorm.DB, rbac *services.RBACService) *RoleHandler {
	return &RoleHandler{db: db, rbac: rbac}
}

// ── List — GET /api/v1/roles ─────────────────────────────────────────────────
// Lists all roles for the authenticated user's tenant.
func (h *RoleHandler) List(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	var roles []models.Role
	query := h.db.Where("tenant_id = ?", auth.TenantID).Order("is_system DESC, role_name ASC")

	if search := c.Query("search"); search != "" {
		query = query.Where("role_name ILIKE ? OR description ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	if err := query.Find(&roles).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to list roles", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Roles retrieved", roles)
}

// ── Get — GET /api/v1/roles/:id ──────────────────────────────────────────────
// Gets a single role with its policies and permissions.
func (h *RoleHandler) Get(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid role ID", nil)
	}

	var role models.Role
	if err := h.db.
		Where("role_id = ? AND tenant_id = ?", id, auth.TenantID).
		Preload("Policies.Permissions").
		First(&role).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusNotFound, "Role not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Role retrieved", role)
}

// ── Create — POST /api/v1/roles ──────────────────────────────────────────────
// Creates a new custom (non-system) role.
func (h *RoleHandler) Create(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}
	if err := auth.CanManageRoles(); err != nil {
		return utils.SendResponse(c, fiber.StatusForbidden, err.Error(), nil)
	}

	type CreateRoleRequest struct {
		RoleName    string `json:"role_name" validate:"required"`
		Description string `json:"description"`
	}

	var req CreateRoleRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}
	if req.RoleName == "" {
		return utils.SendResponse(c, fiber.StatusBadRequest, "role_name is required", nil)
	}

	// Check for duplicates within tenant
	var existing models.Role
	if err := h.db.Where("role_name = ? AND tenant_id = ?", req.RoleName, auth.TenantID).
		First(&existing).Error; err == nil {
		return utils.SendResponse(c, fiber.StatusConflict, "Role with this name already exists", nil)
	}

	role := models.Role{
		RoleID:      uuid.New(),
		TenantID:    auth.TenantID,
		RoleName:    req.RoleName,
		Description: req.Description,
		IsSystem:    false,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := h.db.Create(&role).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to create role", nil)
	}

	return utils.SendResponse(c, fiber.StatusCreated, "Role created", role)
}

// ── Update — PUT /api/v1/roles/:id ───────────────────────────────────────────
// Updates a non-system role's name and/or description.
func (h *RoleHandler) Update(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}
	if err := auth.CanManageRoles(); err != nil {
		return utils.SendResponse(c, fiber.StatusForbidden, err.Error(), nil)
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid role ID", nil)
	}

	var role models.Role
	if err := h.db.Where("role_id = ? AND tenant_id = ?", id, auth.TenantID).First(&role).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusNotFound, "Role not found", nil)
	}
	if role.IsSystem {
		return utils.SendResponse(c, fiber.StatusForbidden, "System roles cannot be modified", nil)
	}

	type UpdateRoleRequest struct {
		RoleName    *string `json:"role_name"`
		Description *string `json:"description"`
	}

	var req UpdateRoleRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	updates := map[string]interface{}{}
	if req.RoleName != nil && *req.RoleName != "" {
		// Check for duplicate name
		var dup models.Role
		if err := h.db.Where("role_name = ? AND tenant_id = ? AND role_id != ?", *req.RoleName, auth.TenantID, id).
			First(&dup).Error; err == nil {
			return utils.SendResponse(c, fiber.StatusConflict, "Role with this name already exists", nil)
		}
		updates["role_name"] = *req.RoleName
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}

	if len(updates) == 0 {
		return utils.SendResponse(c, fiber.StatusBadRequest, "No fields to update", nil)
	}

	if err := h.db.Model(&role).Updates(updates).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to update role", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Role updated", nil)
}

// ── Delete — DELETE /api/v1/roles/:id ────────────────────────────────────────
// Deletes a non-system role. Fails if users are still assigned to it.
func (h *RoleHandler) Delete(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}
	if err := auth.CanManageRoles(); err != nil {
		return utils.SendResponse(c, fiber.StatusForbidden, err.Error(), nil)
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid role ID", nil)
	}

	var role models.Role
	if err := h.db.Where("role_id = ? AND tenant_id = ?", id, auth.TenantID).First(&role).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusNotFound, "Role not found", nil)
	}
	if role.IsSystem {
		return utils.SendResponse(c, fiber.StatusForbidden, "System roles cannot be deleted", nil)
	}

	// Check if any users are assigned to this role
	var count int64
	h.db.Model(&models.UserRole{}).Where("role_id = ? AND tenant_id = ?", id, auth.TenantID).Count(&count)
	if count > 0 {
		return utils.SendResponse(c, fiber.StatusConflict,
			"Cannot delete role — it is assigned to users. Revoke all assignments first.", nil)
	}

	if err := h.db.Delete(&role).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to delete role", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Role deleted", nil)
}

// ── AssignRole — POST /api/v1/users/:id/roles ────────────────────────────────
// Assigns a role to a user. Uses RBACService for privilege escalation checks.
func (h *RoleHandler) AssignRole(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}
	if err := auth.CanAssignRoles(); err != nil {
		return utils.SendResponse(c, fiber.StatusForbidden, err.Error(), nil)
	}

	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid user ID", nil)
	}

	type AssignRoleRequest struct {
		RoleID uuid.UUID `json:"role_id" validate:"required"`
	}

	var req AssignRoleRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}
	if req.RoleID == uuid.Nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "role_id is required", nil)
	}

	// Verify target user exists in same tenant
	var targetUser models.User
	if err := h.db.Where("user_id = ? AND tenant_id = ? AND is_deleted = false", targetUserID, auth.TenantID).
		First(&targetUser).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusNotFound, "User not found", nil)
	}

	ctx := context.Background()
	if err := h.rbac.AssignRole(ctx, auth.UserID, targetUserID, req.RoleID, auth.TenantID); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Role assigned successfully", nil)
}

// ── RevokeRole — DELETE /api/v1/users/:id/roles/:roleId ──────────────────────
// Revokes a role from a user.
func (h *RoleHandler) RevokeRole(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}
	if err := auth.CanAssignRoles(); err != nil {
		return utils.SendResponse(c, fiber.StatusForbidden, err.Error(), nil)
	}

	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid user ID", nil)
	}

	roleID, err := uuid.Parse(c.Params("roleId"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid role ID", nil)
	}

	ctx := context.Background()
	if err := h.rbac.RevokeRole(ctx, auth.UserID, targetUserID, roleID, auth.TenantID); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Role revoked successfully", nil)
}

// ── GetUserRoles — GET /api/v1/users/:id/roles ───────────────────────────────
// Lists all roles assigned to a specific user.
func (h *RoleHandler) GetUserRoles(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid user ID", nil)
	}

	// Users can view their own roles; admins can view anyone's
	if targetUserID != auth.UserID {
		if err := auth.CanManageUsers(); err != nil {
			return utils.SendResponse(c, fiber.StatusForbidden, err.Error(), nil)
		}
	}

	ctx := context.Background()
	roles, err := h.rbac.GetUserRoles(ctx, targetUserID, auth.TenantID)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to get user roles", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "User roles retrieved", roles)
}

// ── Unused import guard ──────────────────────────────────────────────────────
var _ = DTO.RoleSuperAdmin

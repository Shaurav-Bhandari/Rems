package handlers

import (
	"backend/middleware"
	"backend/utils"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
)

// ============================================================================
// USER HANDLER (Admin-only user management)
// ============================================================================

type UserHandler struct {
	db *gorm.DB
}

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{db: db}
}

// List — GET /api/v1/users
func (h *UserHandler) List(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	if err := auth.CanManageUsers(); err != nil {
		return utils.SendResponse(c, fiber.StatusForbidden, err.Error(), nil)
	}

	var users []map[string]interface{}
	query := h.db.Table("users").
		Select("user_id, user_name, full_name, email, phone, is_active, primary_role, created_at").
		Where("tenant_id = ? AND is_deleted = false", auth.TenantID).
		Order("created_at DESC")

	if search := c.Query("search"); search != "" {
		query = query.Where("full_name ILIKE ? OR email ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if role := c.Query("role"); role != "" {
		query = query.Where("primary_role = ?", role)
	}

	if err := query.Find(&users).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to list users", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Users retrieved", users)
}

// Get — GET /api/v1/users/:id
func (h *UserHandler) Get(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")
	var user map[string]interface{}

	if err := h.db.Table("users").
		Select("user_id, user_name, full_name, email, phone, is_active, primary_role, two_factor_enabled, is_email_verified, last_login_at, created_at, updated_at").
		Where("user_id = ? AND tenant_id = ? AND is_deleted = false", id, auth.TenantID).
		First(&user).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusNotFound, "User not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "User retrieved", user)
}

// Update — PUT /api/v1/users/:id
func (h *UserHandler) Update(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	if err := auth.CanManageUsers(); err != nil {
		return utils.SendResponse(c, fiber.StatusForbidden, err.Error(), nil)
	}

	id := c.Params("id")

	var body map[string]interface{}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	// Prevent updating sensitive fields via this endpoint
	delete(body, "password_hash")
	delete(body, "user_id")
	delete(body, "tenant_id")
	delete(body, "primary_role")
	delete(body, "is_active")
	delete(body, "is_email_verified")
	delete(body, "is_deleted")
	delete(body, "deleted_by")

	result := h.db.Table("users").
		Where("user_id = ? AND tenant_id = ? AND is_deleted = false", id, auth.TenantID).
		Updates(body)
	if result.Error != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to update user", nil)
	}
	if result.RowsAffected == 0 {
		return utils.SendResponse(c, fiber.StatusNotFound, "User not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "User updated", nil)
}

// Delete — DELETE /api/v1/users/:id (soft delete)
func (h *UserHandler) Delete(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	if err := auth.CanManageUsers(); err != nil {
		return utils.SendResponse(c, fiber.StatusForbidden, err.Error(), nil)
	}

	id := c.Params("id")

	result := h.db.Table("users").
		Where("user_id = ? AND tenant_id = ? AND is_deleted = false", id, auth.TenantID).
		Updates(map[string]interface{}{
			"is_deleted": true,
			"is_active":  false,
			"deleted_by": auth.UserID.String(),
		})
	if result.Error != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to delete user", nil)
	}
	if result.RowsAffected == 0 {
		return utils.SendResponse(c, fiber.StatusNotFound, "User not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "User deleted", nil)
}

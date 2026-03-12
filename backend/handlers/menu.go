package handlers

import (
	"backend/middleware"
	"backend/utils"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
)

// ============================================================================
// MENU HANDLER
// ============================================================================

type MenuHandler struct {
	db *gorm.DB
}

func NewMenuHandler(db *gorm.DB) *MenuHandler {
	return &MenuHandler{db: db}
}

// ── Categories ───────────────────────────────────────────────────────────────

// ListCategories — GET /api/v1/menu/categories
func (h *MenuHandler) ListCategories(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	var categories []map[string]interface{}
	query := h.db.Table("menu_categories").
		Where("tenant_id = ?", auth.TenantID).
		Order("display_order ASC")

	if restaurantID := c.Query("restaurant_id"); restaurantID != "" {
		query = query.Where("restaurant_id = ?", restaurantID)
	}

	if err := query.Find(&categories).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to list categories", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Categories retrieved", categories)
}

// CreateCategory — POST /api/v1/menu/categories
func (h *MenuHandler) CreateCategory(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	var body map[string]interface{}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	body["tenant_id"] = auth.TenantID.String()

	if err := h.db.Table("menu_categories").Create(&body).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to create category", nil)
	}

	return utils.SendResponse(c, fiber.StatusCreated, "Category created", body)
}

// ── Items ────────────────────────────────────────────────────────────────────

// ListItems — GET /api/v1/menu/items
func (h *MenuHandler) ListItems(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	var items []map[string]interface{}
	query := h.db.Table("menu_items").
		Where("tenant_id = ?", auth.TenantID).
		Order("name ASC")

	if categoryID := c.Query("category_id"); categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}
	if restaurantID := c.Query("restaurant_id"); restaurantID != "" {
		query = query.Where("restaurant_id = ?", restaurantID)
	}
	if available := c.Query("available"); available == "true" {
		query = query.Where("is_available = true")
	}

	if err := query.Find(&items).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to list items", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Menu items retrieved", items)
}

// CreateItem — POST /api/v1/menu/items
func (h *MenuHandler) CreateItem(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	var body map[string]interface{}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	body["tenant_id"] = auth.TenantID.String()

	if err := h.db.Table("menu_items").Create(&body).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to create menu item", nil)
	}

	return utils.SendResponse(c, fiber.StatusCreated, "Menu item created", body)
}

// UpdateItem — PUT /api/v1/menu/items/:id
func (h *MenuHandler) UpdateItem(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")

	var body map[string]interface{}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	result := h.db.Table("menu_items").
		Where("menu_item_id = ? AND tenant_id = ?", id, auth.TenantID).
		Updates(body)
	if result.Error != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to update menu item", nil)
	}
	if result.RowsAffected == 0 {
		return utils.SendResponse(c, fiber.StatusNotFound, "Menu item not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Menu item updated", nil)
}

// DeleteItem — DELETE /api/v1/menu/items/:id
func (h *MenuHandler) DeleteItem(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")

	result := h.db.Table("menu_items").
		Where("menu_item_id = ? AND tenant_id = ?", id, auth.TenantID).
		Delete(nil)
	if result.Error != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to delete menu item", nil)
	}
	if result.RowsAffected == 0 {
		return utils.SendResponse(c, fiber.StatusNotFound, "Menu item not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Menu item deleted", nil)
}

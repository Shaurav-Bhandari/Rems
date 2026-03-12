package handlers

import (
	"backend/middleware"
	"backend/utils"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
)

// ============================================================================
// INVENTORY HANDLER
// ============================================================================

type InventoryHandler struct {
	db *gorm.DB
}

func NewInventoryHandler(db *gorm.DB) *InventoryHandler {
	return &InventoryHandler{db: db}
}

// List — GET /api/v1/inventory
func (h *InventoryHandler) List(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	var items []map[string]interface{}
	query := h.db.Table("inventory_items").
		Where("tenant_id = ?", auth.TenantID).
		Order("name ASC")

	if restaurantID := c.Query("restaurant_id"); restaurantID != "" {
		query = query.Where("restaurant_id = ?", restaurantID)
	}
	if lowStock := c.Query("low_stock"); lowStock == "true" {
		query = query.Where("current_quantity <= reorder_point")
	}

	if err := query.Find(&items).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to list inventory", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Inventory retrieved", items)
}

// Get — GET /api/v1/inventory/:id
func (h *InventoryHandler) Get(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")
	var item map[string]interface{}

	if err := h.db.Table("inventory_items").
		Where("inventory_item_id = ? AND tenant_id = ?", id, auth.TenantID).
		First(&item).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusNotFound, "Inventory item not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Inventory item retrieved", item)
}

// Create — POST /api/v1/inventory
func (h *InventoryHandler) Create(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	var body map[string]interface{}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	body["tenant_id"] = auth.TenantID.String()

	if err := h.db.Table("inventory_items").Create(&body).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to create inventory item", nil)
	}

	return utils.SendResponse(c, fiber.StatusCreated, "Inventory item created", body)
}

// Update — PUT /api/v1/inventory/:id
func (h *InventoryHandler) Update(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")

	var body map[string]interface{}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	result := h.db.Table("inventory_items").
		Where("inventory_item_id = ? AND tenant_id = ?", id, auth.TenantID).
		Updates(body)
	if result.Error != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to update inventory item", nil)
	}
	if result.RowsAffected == 0 {
		return utils.SendResponse(c, fiber.StatusNotFound, "Inventory item not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Inventory item updated", nil)
}

// AdjustStock — POST /api/v1/inventory/:id/adjust
func (h *InventoryHandler) AdjustStock(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")

	var body struct {
		Adjustment float64 `json:"adjustment"`
		Reason     string  `json:"reason"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	result := h.db.Table("inventory_items").
		Where("inventory_item_id = ? AND tenant_id = ?", id, auth.TenantID).
		Update("current_quantity", gorm.Expr("current_quantity + ?", body.Adjustment))
	if result.Error != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to adjust stock", nil)
	}
	if result.RowsAffected == 0 {
		return utils.SendResponse(c, fiber.StatusNotFound, "Inventory item not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Stock adjusted", map[string]interface{}{
		"adjustment": body.Adjustment,
		"reason":     body.Reason,
	})
}

// Delete — DELETE /api/v1/inventory/:id
func (h *InventoryHandler) Delete(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")

	result := h.db.Table("inventory_items").
		Where("inventory_item_id = ? AND tenant_id = ?", id, auth.TenantID).
		Delete(nil)
	if result.Error != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to delete inventory item", nil)
	}
	if result.RowsAffected == 0 {
		return utils.SendResponse(c, fiber.StatusNotFound, "Inventory item not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Inventory item deleted", nil)
}

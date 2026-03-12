package handlers

import (
	"backend/middleware"
	"backend/utils"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
)

// ============================================================================
// ORDER HANDLER
// ============================================================================

type OrderHandler struct {
	db *gorm.DB
}

func NewOrderHandler(db *gorm.DB) *OrderHandler {
	return &OrderHandler{db: db}
}

// List — GET /api/v1/orders
func (h *OrderHandler) List(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	var orders []map[string]interface{}
	query := h.db.Table("orders").
		Where("tenant_id = ?", auth.TenantID).
		Order("created_at DESC").
		Limit(50)

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if restaurantID := c.Query("restaurant_id"); restaurantID != "" {
		query = query.Where("restaurant_id = ?", restaurantID)
	}

	if err := query.Find(&orders).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to list orders", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Orders retrieved", orders)
}

// Get — GET /api/v1/orders/:id
func (h *OrderHandler) Get(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")
	var order map[string]interface{}

	if err := h.db.Table("orders").
		Where("order_id = ? AND tenant_id = ?", id, auth.TenantID).
		First(&order).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusNotFound, "Order not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Order retrieved", order)
}

// Create — POST /api/v1/orders
func (h *OrderHandler) Create(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	var body map[string]interface{}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	body["tenant_id"] = auth.TenantID.String()
	body["user_id"] = auth.UserID.String()

	if err := h.db.Table("orders").Create(&body).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to create order", nil)
	}

	return utils.SendResponse(c, fiber.StatusCreated, "Order created", body)
}

// UpdateStatus — PUT /api/v1/orders/:id/status
func (h *OrderHandler) UpdateStatus(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")

	var body struct {
		Status string `json:"status"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	result := h.db.Table("orders").
		Where("order_id = ? AND tenant_id = ?", id, auth.TenantID).
		Update("status", body.Status)
	if result.Error != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to update order status", nil)
	}
	if result.RowsAffected == 0 {
		return utils.SendResponse(c, fiber.StatusNotFound, "Order not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Order status updated", nil)
}

// Delete — DELETE /api/v1/orders/:id
func (h *OrderHandler) Delete(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")

	result := h.db.Table("orders").
		Where("order_id = ? AND tenant_id = ?", id, auth.TenantID).
		Delete(nil)
	if result.Error != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to delete order", nil)
	}
	if result.RowsAffected == 0 {
		return utils.SendResponse(c, fiber.StatusNotFound, "Order not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Order deleted", nil)
}

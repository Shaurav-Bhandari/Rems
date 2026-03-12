package handlers

import (
	"backend/middleware"
	"backend/utils"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
)

// ============================================================================
// RESTAURANT HANDLER
// ============================================================================

type RestaurantHandler struct {
	db *gorm.DB
}

func NewRestaurantHandler(db *gorm.DB) *RestaurantHandler {
	return &RestaurantHandler{db: db}
}

// List — GET /api/v1/restaurants
func (h *RestaurantHandler) List(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	type restaurantRow struct {
		RestaurantID string `json:"restaurant_id"`
		Name         string `json:"name"`
		City         string `json:"city"`
		Phone        string `json:"phone"`
		IsActive     bool   `json:"is_active"`
	}

	var rows []restaurantRow
	query := h.db.Table("restaurants").Where("tenant_id = ?", auth.TenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	if err := query.Order("name ASC").Find(&rows).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to list restaurants", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Restaurants retrieved", rows)
}

// Get — GET /api/v1/restaurants/:id
func (h *RestaurantHandler) Get(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")
	var restaurant map[string]interface{}

	if err := h.db.Table("restaurants").
		Where("restaurant_id = ? AND tenant_id = ?", id, auth.TenantID).
		First(&restaurant).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusNotFound, "Restaurant not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Restaurant retrieved", restaurant)
}

// Create — POST /api/v1/restaurants
func (h *RestaurantHandler) Create(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	if err := auth.CanCreateRestaurant(); err != nil {
		return utils.SendResponse(c, fiber.StatusForbidden, err.Error(), nil)
	}

	var body map[string]interface{}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	body["tenant_id"] = auth.TenantID.String()

	if err := h.db.Table("restaurants").Create(&body).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to create restaurant", nil)
	}

	return utils.SendResponse(c, fiber.StatusCreated, "Restaurant created", body)
}

// Update — PUT /api/v1/restaurants/:id
func (h *RestaurantHandler) Update(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")

	var body map[string]interface{}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	result := h.db.Table("restaurants").
		Where("restaurant_id = ? AND tenant_id = ?", id, auth.TenantID).
		Updates(body)
	if result.Error != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to update restaurant", nil)
	}
	if result.RowsAffected == 0 {
		return utils.SendResponse(c, fiber.StatusNotFound, "Restaurant not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Restaurant updated", nil)
}

// Delete — DELETE /api/v1/restaurants/:id
func (h *RestaurantHandler) Delete(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	id := c.Params("id")

	result := h.db.Table("restaurants").
		Where("restaurant_id = ? AND tenant_id = ?", id, auth.TenantID).
		Delete(nil)
	if result.Error != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to delete restaurant", nil)
	}
	if result.RowsAffected == 0 {
		return utils.SendResponse(c, fiber.StatusNotFound, "Restaurant not found", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Restaurant deleted", nil)
}

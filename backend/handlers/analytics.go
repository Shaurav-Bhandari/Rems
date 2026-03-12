package handlers

import (
	"backend/middleware"
	"backend/utils"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AnalyticsHandler struct {
	db *gorm.DB
}

func NewAnalyticsHandler(db *gorm.DB) *AnalyticsHandler {
	return &AnalyticsHandler{db: db}
}

func (h *AnalyticsHandler) parseTimeRange(c fiber.Ctx) (time.Time, time.Time) {
	from := time.Now().AddDate(0, 0, -30)
	to := time.Now()
	if f := c.Query("from"); f != "" {
		if t, err := time.Parse("2006-01-02", f); err == nil {
			from = t
		}
	}
	if t := c.Query("to"); t != "" {
		if parsed, err := time.Parse("2006-01-02", t); err == nil {
			to = parsed.Add(24*time.Hour - time.Second)
		}
	}
	return from, to
}

func (h *AnalyticsHandler) parseRestaurantID(c fiber.Ctx) *uuid.UUID {
	if rid := c.Query("restaurant_id"); rid != "" {
		if parsed, err := uuid.Parse(rid); err == nil {
			return &parsed
		}
	}
	return nil
}

// RevenueOverview — GET /api/v1/analytics/revenue/overview
func (h *AnalyticsHandler) RevenueOverview(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}
	from, to := h.parseTimeRange(c)
	rid := h.parseRestaurantID(c)

	type Result struct {
		TotalRevenue float64 `json:"total_revenue"`
		OrderCount   int64   `json:"order_count"`
		AvgOrder     float64 `json:"avg_order_value"`
	}
	var r Result
	q := h.db.Table("orders").Select("COALESCE(SUM(total_amount),0) as total_revenue, COUNT(*) as order_count, COALESCE(AVG(total_amount),0) as avg_order").Where("tenant_id = ? AND created_at BETWEEN ? AND ?", auth.TenantID, from, to)
	if rid != nil {
		q = q.Where("restaurant_id = ?", *rid)
	}
	if err := q.Scan(&r).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed", nil)
	}
	return utils.SendResponse(c, fiber.StatusOK, "Revenue overview", r)
}

// RevenueTrend — GET /api/v1/analytics/revenue/trend
func (h *AnalyticsHandler) RevenueTrend(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}
	from, to := h.parseTimeRange(c)
	type P struct {
		Date    string  `json:"date"`
		Revenue float64 `json:"revenue"`
	}
	var pts []P
	if err := h.db.Table("orders").Select("DATE(created_at) as date, COALESCE(SUM(total_amount),0) as revenue").Where("tenant_id = ? AND created_at BETWEEN ? AND ?", auth.TenantID, from, to).Group("DATE(created_at)").Order("date ASC").Scan(&pts).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed", nil)
	}
	return utils.SendResponse(c, fiber.StatusOK, "Revenue trend", pts)
}

// OrderVolume — GET /api/v1/analytics/orders/volume
func (h *AnalyticsHandler) OrderVolume(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}
	from, to := h.parseTimeRange(c)
	type P struct {
		Date   string `json:"date"`
		Orders int64  `json:"orders"`
	}
	var pts []P
	if err := h.db.Table("orders").Select("DATE(created_at) as date, COUNT(*) as orders").Where("tenant_id = ? AND created_at BETWEEN ? AND ?", auth.TenantID, from, to).Group("DATE(created_at)").Order("date ASC").Scan(&pts).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed", nil)
	}
	return utils.SendResponse(c, fiber.StatusOK, "Order volume", pts)
}

// OrderStatusDistribution — GET /api/v1/analytics/orders/status
func (h *AnalyticsHandler) OrderStatusDistribution(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}
	type S struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	var counts []S
	if err := h.db.Table("orders").Select("status, COUNT(*) as count").Where("tenant_id = ?", auth.TenantID).Group("status").Scan(&counts).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed", nil)
	}
	return utils.SendResponse(c, fiber.StatusOK, "Order status distribution", counts)
}

// TopSellingItems — GET /api/v1/analytics/menu/top-items
func (h *AnalyticsHandler) TopSellingItems(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}
	from, to := h.parseTimeRange(c)
	type Item struct {
		Name     string  `json:"name"`
		Quantity int64   `json:"quantity_sold"`
		Revenue  float64 `json:"revenue"`
	}
	var items []Item
	if err := h.db.Table("order_items").Select("menu_items.name, SUM(order_items.quantity) as quantity, SUM(order_items.subtotal) as revenue").Joins("JOIN menu_items ON order_items.menu_item_id = menu_items.menu_item_id").Joins("JOIN orders ON order_items.order_id = orders.order_id").Where("orders.tenant_id = ? AND orders.created_at BETWEEN ? AND ?", auth.TenantID, from, to).Group("menu_items.name").Order("quantity DESC").Limit(20).Scan(&items).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed", nil)
	}
	return utils.SendResponse(c, fiber.StatusOK, "Top selling items", items)
}

// InventoryTurnover — GET /api/v1/analytics/inventory/turnover
func (h *AnalyticsHandler) InventoryTurnover(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}
	type I struct {
		Name         string  `json:"name"`
		CurrentQty   float64 `json:"current_quantity"`
		ReorderPoint float64 `json:"reorder_point"`
		LowStock     bool    `json:"is_low_stock"`
	}
	var items []I
	if err := h.db.Table("inventory_items").Select("name, current_quantity as current_qty, reorder_point, (current_quantity <= reorder_point) as low_stock").Where("tenant_id = ?", auth.TenantID).Order("current_quantity ASC").Scan(&items).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed", nil)
	}
	return utils.SendResponse(c, fiber.StatusOK, "Inventory turnover", items)
}

// ForecastDemand — GET /api/v1/analytics/forecast/demand
func (h *AnalyticsHandler) ForecastDemand(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}
	type F struct {
		Date     string  `json:"date"`
		Forecast float64 `json:"forecast"`
	}
	var pts []F
	if err := h.db.Table("orders").Select("DATE(created_at) as date, COUNT(*) as forecast").Where("tenant_id = ? AND created_at >= NOW() - INTERVAL '30 days'", auth.TenantID).Group("DATE(created_at)").Order("date ASC").Scan(&pts).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed", nil)
	}
	return utils.SendResponse(c, fiber.StatusOK, "Demand forecast", pts)
}

package handlers

import (
	"backend/config"
	"backend/utils"
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// ============================================================================
// HEALTH HANDLER
// ============================================================================

type HealthHandler struct {
	redis *redis.Client
	vault *config.ImmuVault
}

func NewHealthHandler(redisClient *redis.Client, vault *config.ImmuVault) *HealthHandler {
	return &HealthHandler{redis: redisClient, vault: vault}
}

// Health — GET /api/v1/health
func (h *HealthHandler) Health(c fiber.Ctx) error {
	return utils.SendResponse(c, fiber.StatusOK, "OK", map[string]string{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// RedisHealth — GET /api/v1/health/redis
func (h *HealthHandler) RedisHealth(c fiber.Ctx) error {
	if h.redis == nil {
		return utils.SendResponse(c, fiber.StatusServiceUnavailable, "Redis not configured", nil)
	}
	ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
	defer cancel()

	if err := h.redis.Ping(ctx).Err(); err != nil {
		return utils.SendResponse(c, fiber.StatusServiceUnavailable, "Redis unreachable", map[string]string{"error": err.Error()})
	}

	info, _ := h.redis.Info(ctx, "keyspace", "memory").Result()
	_ = info

	dbSize, _ := h.redis.DBSize(ctx).Result()

	return utils.SendResponse(c, fiber.StatusOK, "Redis healthy", map[string]interface{}{
		"status":     "connected",
		"total_keys": dbSize,
	})
}

// ImmuDBHealth — GET /api/v1/health/immudb
func (h *HealthHandler) ImmuDBHealth(c fiber.Ctx) error {
	if h.vault == nil {
		return utils.SendResponse(c, fiber.StatusServiceUnavailable, "ImmuDB not configured", nil)
	}

	// Try reading a known key as health check
	val := h.vault.GetString(c.Context(), "app.port", "")
	if val == "" {
		return utils.SendResponse(c, fiber.StatusServiceUnavailable, "ImmuDB unreachable or empty", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "ImmuDB healthy", map[string]string{
		"status": "connected",
	})
}

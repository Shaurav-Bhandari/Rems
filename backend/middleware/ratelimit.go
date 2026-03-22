package middleware

import (
	"backend/utils"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// ============================================================================
// ADAPTIVE RATE LIMITING MIDDLEWARE (REDIS-BACKED)
// Implements a sliding-window rate limiter with per-IP (unauthenticated),
// per-UserID (authenticated), and per-Tenant aggregate limiting. Uses
// token-bucket burst allowance. Configurable per-route and per-role limits.
// Returns RFC 6585 compliant 429 responses with Retry-After and X-RateLimit headers.
// ============================================================================

// RateLimitConfig configures the rate limiting middleware.
type RateLimitConfig struct {
	// RedisClient is the Redis connection for rate limit counters.
	RedisClient *redis.Client

	// DefaultLimit is the default max requests per window for unauthenticated users.
	DefaultLimit int

	// DefaultWindow is the duration of the sliding window.
	DefaultWindow time.Duration

	// BurstMultiplier allows N * DefaultLimit requests in short bursts.
	// Set to 1.0 for no burst allowance.
	BurstMultiplier float64

	// RoleLimits overrides the default limit for specific roles.
	// Higher-privilege roles typically get higher limits.
	RoleLimits map[string]int

	// RouteOverrides maps route prefixes to specific limits.
	RouteOverrides map[string]int

	// TenantLimit is the aggregate limit for all users within a single tenant.
	// Set to 0 to disable tenant-level limiting.
	TenantLimit int

	// TenantWindow is the window for tenant-level limiting.
	TenantWindow time.Duration

	// AbuseThreshold is the number of consecutive 429 responses before
	// a security event is logged for potential abuse.
	AbuseThreshold int

	// SkipPaths are paths exempt from rate limiting.
	SkipPaths []string

	// KeyPrefix is the Redis key prefix for rate limit counters.
	KeyPrefix string
}

// DefaultRateLimitConfig returns a production-ready default configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		DefaultLimit:    100,
		DefaultWindow:   1 * time.Minute,
		BurstMultiplier: 1.5,
		RoleLimits: map[string]int{
			"super_admin":       500,
			"admin":             300,
			"owner":             200,
			"manager":           200,
			"assistant_manager": 150,
			"employee":          100,
			"waiter":            120,
			"cashier":           120,
			"chef":              80,
			"inventory_manager": 100,
			"viewer":            60,
		},
		RouteOverrides: map[string]int{
			"/api/v1/auth/login":           10, // Strict: prevent brute force
			"/api/v1/auth/register":        5,  // Very strict: prevent spam
			"/api/v1/auth/forgot-password": 3,  // Very strict
			"/api/v1/export":               10, // Expensive operations
			"/api/v1/analytics":            30, // Moderately expensive
		},
		TenantLimit:    5000,
		TenantWindow:   1 * time.Minute,
		AbuseThreshold: 10,
		SkipPaths: []string{
			"/api/v1/health",
			"/api/v1/ping",
		},
		KeyPrefix: "ratelimit:",
	}
}

// RateLimit returns the rate limiting middleware with default config.
func RateLimit(redisClient *redis.Client) fiber.Handler {
	cfg := DefaultRateLimitConfig()
	cfg.RedisClient = redisClient
	return RateLimitWithConfig(cfg)
}

// RateLimitWithConfig returns rate limiting middleware with custom config.
func RateLimitWithConfig(cfg RateLimitConfig) fiber.Handler {
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "ratelimit:"
	}
	if cfg.DefaultWindow == 0 {
		cfg.DefaultWindow = 1 * time.Minute
	}
	if cfg.BurstMultiplier == 0 {
		cfg.BurstMultiplier = 1.0
	}

	skipSet := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skipSet[p] = struct{}{}
	}

	return func(c fiber.Ctx) error {
		path := c.Path()

		// Skip paths
		for skip := range skipSet {
			if pathHasPrefix(path, skip) {
				return c.Next()
			}
		}

		// ── Determine rate limit key and limit ─────────────────
		key, limit, window := resolveRateLimitParams(c, path, cfg)

		// ── Apply burst multiplier ─────────────────────────────
		burstLimit := int(float64(limit) * cfg.BurstMultiplier)

		// ── Check rate limit via Redis ─────────────────────────
		remaining, resetAt, err := checkRateLimit(cfg.RedisClient, key, burstLimit, window)
		if err != nil {
			// Redis error — fail open (allow the request but log)
			log.Printf("[WARN] Rate limiter Redis error: %v (failing open)", err)
			return c.Next()
		}

		// ── Set rate limit headers ─────────────────────────────
		c.Set("X-RateLimit-Limit", strconv.Itoa(burstLimit))
		c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Set("X-RateLimit-Reset", strconv.FormatInt(resetAt, 10))

		// ── Enforce limit ──────────────────────────────────────
		if remaining < 0 {
			retryAfter := resetAt - time.Now().Unix()
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Set("Retry-After", strconv.FormatInt(retryAfter, 10))

			// Track consecutive 429s for abuse detection
			trackAbuse(cfg, c, key)

			return utils.SendResponse(c, fiber.StatusTooManyRequests,
				"Rate limit exceeded. Please slow down.", map[string]interface{}{
					"retry_after_seconds": retryAfter,
					"limit":               burstLimit,
					"window_seconds":      window.Seconds(),
				})
		}

		// ── Tenant-level aggregate limit ───────────────────────
		if cfg.TenantLimit > 0 {
			auth := GetAuthContext(c)
			if auth != nil {
				tenantKey := cfg.KeyPrefix + "tenant:" + auth.TenantID.String()
				tRemaining, tResetAt, tErr := checkRateLimit(
					cfg.RedisClient, tenantKey, cfg.TenantLimit, cfg.TenantWindow)
				if tErr == nil && tRemaining < 0 {
					retryAfter := tResetAt - time.Now().Unix()
					if retryAfter < 1 {
						retryAfter = 1
					}
					c.Set("Retry-After", strconv.FormatInt(retryAfter, 10))
					return utils.SendResponse(c, fiber.StatusTooManyRequests,
						"Organization rate limit exceeded", map[string]interface{}{
							"retry_after_seconds": retryAfter,
						})
				}
			}
		}

		return c.Next()
	}
}

// resolveRateLimitParams determines the Redis key, limit, and window for this request.
func resolveRateLimitParams(c fiber.Ctx, path string, cfg RateLimitConfig) (string, int, time.Duration) {
	limit := cfg.DefaultLimit
	window := cfg.DefaultWindow

	// Check route-specific override (most specific prefix wins)
	bestRoutePrefix := ""
	for prefix, routeLimit := range cfg.RouteOverrides {
		if pathHasPrefix(path, prefix) && len(prefix) > len(bestRoutePrefix) {
			bestRoutePrefix = prefix
			limit = routeLimit
		}
	}

	// Build key based on authentication status
	auth := GetAuthContext(c)
	var key string
	if auth != nil {
		key = cfg.KeyPrefix + "user:" + auth.UserID.String()

		// Apply role-specific limits (only if no route override matched)
		if bestRoutePrefix == "" {
			if roleLimit, exists := cfg.RoleLimits[auth.Role]; exists {
				limit = roleLimit
			}
		}
	} else {
		// Unauthenticated — rate limit by IP
		key = cfg.KeyPrefix + "ip:" + GetRealIP(c)
	}

	// Scope key to route if there's a route override
	if bestRoutePrefix != "" {
		key += ":route:" + bestRoutePrefix
	}

	return key, limit, window
}

// checkRateLimit implements a sliding window counter in Redis.
// Returns (remaining requests, unix timestamp when window resets, error).
func checkRateLimit(client *redis.Client, key string, limit int, window time.Duration) (int, int64, error) {
	if client == nil {
		return limit, time.Now().Add(window).Unix(), nil
	}

	ctx := context.Background()
	now := time.Now()

	// Use Redis pipeline for atomic operations
	pipe := client.Pipeline()

	// Remove expired entries from the sorted set
	pipe.ZRemRangeByScore(ctx, key, "-inf",
		strconv.FormatInt(now.Add(-window).UnixMicro(), 10))

	// Add current request
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixMicro()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	// Count entries in window
	countCmd := pipe.ZCard(ctx, key)

	// Set TTL on the key
	pipe.Expire(ctx, key, window+time.Second)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, 0, err
	}

	count := int(countCmd.Val())
	remaining := limit - count
	resetAt := now.Add(window).Unix()

	return remaining, resetAt, nil
}

// trackAbuse increments a counter for consecutive 429s on the same key.
func trackAbuse(cfg RateLimitConfig, c fiber.Ctx, key string) {
	if cfg.RedisClient == nil || cfg.AbuseThreshold <= 0 {
		return
	}

	ctx := context.Background()
	abuseKey := cfg.KeyPrefix + "abuse:" + key

	count, err := cfg.RedisClient.Incr(ctx, abuseKey).Result()
	if err != nil {
		return
	}

	// Set expiry on first increment
	if count == 1 {
		cfg.RedisClient.Expire(ctx, abuseKey, 10*time.Minute)
	}

	if int(count) >= cfg.AbuseThreshold {
		entry := map[string]interface{}{
			"level":      "WARN",
			"event":      "rate_limit_abuse",
			"key":        key,
			"hit_count":  count,
			"threshold":  cfg.AbuseThreshold,
			"ip":         GetRealIP(c),
			"path":       c.Path(),
			"request_id": GetRequestID(c),
			"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		}
		logJSON, _ := json.Marshal(entry)
		log.Printf("[SECURITY] %s", string(logJSON))

		// Reset counter after alert
		cfg.RedisClient.Del(ctx, abuseKey)
	}
}

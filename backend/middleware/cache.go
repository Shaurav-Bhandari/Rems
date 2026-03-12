package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ============================================================================
// RESPONSE CACHING MIDDLEWARE (REDIS-BACKED)
// Caches GET responses in Redis with tenant-scoped keys. Supports ETag
// generation, conditional 304 Not Modified responses, configurable TTL per
// route group, and automatic cache invalidation on mutations.
// ============================================================================

// CacheConfig configures the response caching middleware.
type CacheConfig struct {
	// RedisClient is the Redis connection for cached responses.
	RedisClient *redis.Client

	// DefaultTTL is the default cache duration.
	DefaultTTL time.Duration

	// RouteTTLs maps route prefixes to specific TTLs.
	RouteTTLs map[string]time.Duration

	// SkipPaths are paths that should never be cached.
	SkipPaths []string

	// CacheableStatusCodes are HTTP status codes that can be cached.
	CacheableStatusCodes []int

	// KeyPrefix is the Redis key prefix for cached responses.
	KeyPrefix string

	// EnableETag generates and validates ETags for cached responses.
	EnableETag bool

	// CachePrivate controls whether responses with user-specific data
	// are cached. When true, cache keys include the user ID.
	CachePrivate bool
}

// cachedResponse represents a cached HTTP response stored in Redis.
type cachedResponse struct {
	StatusCode  int               `json:"status_code"`
	Body        []byte            `json:"body"`
	ContentType string            `json:"content_type"`
	ETag        string            `json:"etag"`
	CachedAt    time.Time         `json:"cached_at"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// DefaultCacheConfig returns a sensible default cache configuration.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		DefaultTTL: 5 * time.Minute,
		RouteTTLs: map[string]time.Duration{
			"/api/V1/menu":       15 * time.Minute, // Menu changes infrequently
			"/api/V1/restaurant": 10 * time.Minute, // Restaurant info is mostly static
			"/api/V1/analytics":  2 * time.Minute,  // Analytics data is semi-dynamic
			"/api/V1/forecast":   5 * time.Minute,  // Forecasts update periodically
		},
		SkipPaths: []string{
			"/api/V1/auth",
			"/api/V1/health",
			"/api/V1/ping",
			"/api/V1/orders", // Orders are highly dynamic
		},
		CacheableStatusCodes: []int{200, 203, 204, 206, 300, 301, 404, 405, 410, 414, 501},
		KeyPrefix:            "cache:",
		EnableETag:           true,
		CachePrivate:         true,
	}
}

// Cache returns the response caching middleware with default config.
func Cache(redisClient *redis.Client) fiber.Handler {
	cfg := DefaultCacheConfig()
	cfg.RedisClient = redisClient
	return CacheWithConfig(cfg)
}

// CacheWithConfig returns response caching middleware with custom config.
func CacheWithConfig(cfg CacheConfig) fiber.Handler {
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "cache:"
	}
	if cfg.DefaultTTL == 0 {
		cfg.DefaultTTL = 5 * time.Minute
	}

	skipSet := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skipSet[p] = struct{}{}
	}

	cacheableSet := make(map[int]struct{}, len(cfg.CacheableStatusCodes))
	for _, code := range cfg.CacheableStatusCodes {
		cacheableSet[code] = struct{}{}
	}

	return func(c fiber.Ctx) error {
		// Only cache GET and HEAD requests
		method := c.Method()
		if method != "GET" && method != "HEAD" {
			// On mutations, invalidate related cache entries
			if method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE" {
				go invalidateRelatedCache(cfg, c)
			}
			return c.Next()
		}

		path := c.Path()

		// Skip non-cacheable paths
		for skip := range skipSet {
			if pathHasPrefix(path, skip) {
				return c.Next()
			}
		}

		// Check Cache-Control: no-cache header from client
		if strings.Contains(c.Get("Cache-Control"), "no-cache") {
			return c.Next()
		}

		// ── Build cache key ────────────────────────────────────
		cacheKey := buildCacheKey(c, cfg)

		// ── Try to serve from cache ────────────────────────────
		if cfg.RedisClient != nil {
			if cached, err := getCachedResponse(cfg.RedisClient, cacheKey); err == nil && cached != nil {
				// ETag conditional response
				if cfg.EnableETag && cached.ETag != "" {
					if c.Get("If-None-Match") == cached.ETag {
						c.Set("ETag", cached.ETag)
						c.Set("X-Cache", "HIT-ETAG")
						return c.SendStatus(fiber.StatusNotModified)
					}
				}

				// Serve cached response
				c.Set("Content-Type", cached.ContentType)
				if cfg.EnableETag && cached.ETag != "" {
					c.Set("ETag", cached.ETag)
				}
				c.Set("X-Cache", "HIT")
				c.Set("X-Cache-Age", strconv.FormatInt(int64(time.Since(cached.CachedAt).Seconds()), 10))

				// Restore cached headers
				for k, v := range cached.Headers {
					c.Set(k, v)
				}

				return c.Status(cached.StatusCode).Send(cached.Body)
			}
		}

		// ── Cache MISS — execute handler ───────────────────────
		c.Set("X-Cache", "MISS")
		err := c.Next()
		if err != nil {
			return err
		}

		// ── Cache the response if eligible ─────────────────────
		statusCode := c.Response().StatusCode()
		if _, cacheable := cacheableSet[statusCode]; cacheable && cfg.RedisClient != nil {
			ttl := resolveCacheTTL(path, cfg)
			body := c.Response().Body()

			// Generate ETag
			var etag string
			if cfg.EnableETag {
				hash := sha256.Sum256(body)
				etag = fmt.Sprintf(`"%x"`, hash[:8])
				c.Set("ETag", etag)
			}

			// Store asynchronously to not block the response
			go func() {
				cr := &cachedResponse{
					StatusCode:  statusCode,
					Body:        body,
					ContentType: string(c.Response().Header.ContentType()),
					ETag:        etag,
					CachedAt:    time.Now(),
					Headers: map[string]string{
						"X-Request-ID": GetRequestID(c),
					},
				}
				setCachedResponse(cfg.RedisClient, cacheKey, cr, ttl)
			}()
		}

		return nil
	}
}

// buildCacheKey generates a tenant-scoped Redis key for the cached response.
func buildCacheKey(c fiber.Ctx, cfg CacheConfig) string {
	var sb strings.Builder
	sb.WriteString(cfg.KeyPrefix)

	// Scope by tenant
	auth := GetAuthContext(c)
	if auth != nil {
		sb.WriteString("t:")
		sb.WriteString(auth.TenantID.String())
		sb.WriteString(":")

		// Scope by user if private caching is enabled
		if cfg.CachePrivate {
			sb.WriteString("u:")
			sb.WriteString(auth.UserID.String())
			sb.WriteString(":")
		}
	} else {
		sb.WriteString("anon:")
	}

	// Path + query string
	sb.WriteString(c.Path())
	qs := c.Request().URI().QueryString()
	if len(qs) > 0 {
		sb.WriteString("?")
		sb.WriteString(string(qs))
	}

	return sb.String()
}

// resolveCacheTTL finds the TTL for a given path.
func resolveCacheTTL(path string, cfg CacheConfig) time.Duration {
	bestMatch := ""
	bestTTL := cfg.DefaultTTL

	for prefix, ttl := range cfg.RouteTTLs {
		if pathHasPrefix(path, prefix) && len(prefix) > len(bestMatch) {
			bestMatch = prefix
			bestTTL = ttl
		}
	}

	return bestTTL
}

// getCachedResponse retrieves a cached response from Redis.
func getCachedResponse(client *redis.Client, key string) (*cachedResponse, error) {
	ctx := context.Background()
	data, err := client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var cr cachedResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, err
	}

	return &cr, nil
}

// setCachedResponse stores a response in Redis.
func setCachedResponse(client *redis.Client, key string, cr *cachedResponse, ttl time.Duration) {
	ctx := context.Background()
	data, err := json.Marshal(cr)
	if err != nil {
		return
	}
	client.Set(ctx, key, data, ttl)
}

// invalidateRelatedCache removes cached entries that share the same
// resource prefix as the mutated path. For example, a POST to /api/V1/menu/items
// invalidates all cached /api/V1/menu/* responses.
func invalidateRelatedCache(cfg CacheConfig, c fiber.Ctx) {
	if cfg.RedisClient == nil {
		return
	}

	// Extract the resource prefix (first 3 segments of the path)
	path := c.Path()
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 3 {
		return
	}
	resourcePrefix := "/" + strings.Join(parts[:3], "/")

	ctx := context.Background()

	// Build pattern to match all cached keys for this resource
	var pattern string
	auth := GetAuthContext(c)
	if auth != nil {
		pattern = cfg.KeyPrefix + "t:" + auth.TenantID.String() + ":*" + resourcePrefix + "*"
	} else {
		pattern = cfg.KeyPrefix + "anon:*" + resourcePrefix + "*"
	}

	// Use SCAN to find and delete matching keys (non-blocking)
	iter := cfg.RedisClient.Scan(ctx, 0, pattern, 100).Iterator()
	var keysToDelete []string
	for iter.Next(ctx) {
		keysToDelete = append(keysToDelete, iter.Val())
	}

	if len(keysToDelete) > 0 {
		cfg.RedisClient.Del(ctx, keysToDelete...)
	}
}

// InvalidateCache explicitly invalidates cache for a specific resource.
// Useful when you need to bust the cache from a handler.
func InvalidateCache(client *redis.Client, tenantID uuid.UUID, resourcePath string) {
	if client == nil {
		return
	}

	ctx := context.Background()
	pattern := "cache:t:" + tenantID.String() + ":*" + resourcePath + "*"

	iter := client.Scan(ctx, 0, pattern, 100).Iterator()
	var keysToDelete []string
	for iter.Next(ctx) {
		keysToDelete = append(keysToDelete, iter.Val())
	}

	if len(keysToDelete) > 0 {
		client.Del(ctx, keysToDelete...)
	}
}

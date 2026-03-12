package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

// ============================================================================
// CORS MIDDLEWARE
// Configurable Cross-Origin Resource Sharing middleware. Supports allowlisted
// origins, per-tenant domain resolution, credential support, and proper
// preflight (OPTIONS) handling.
// ============================================================================

// CORSConfig holds configuration for the CORS middleware.
type CORSConfig struct {
	// AllowOrigins is a list of allowed origins.
	// Use "*" to allow all (not recommended with credentials).
	AllowOrigins []string

	// AllowMethods lists the HTTP methods allowed in CORS requests.
	AllowMethods []string

	// AllowHeaders lists the headers allowed in CORS requests.
	AllowHeaders []string

	// ExposeHeaders lists the headers that the browser is allowed to access.
	ExposeHeaders []string

	// AllowCredentials indicates whether the response can include credentials.
	AllowCredentials bool

	// MaxAge indicates how long (seconds) the preflight response is cached.
	MaxAge int
}

// DefaultCORSConfig returns a permissive-but-safe default config.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"http://localhost:3000", "http://localhost:5173", "http://localhost:8080"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowHeaders: []string{
			"Origin", "Content-Type", "Accept", "Authorization",
			"X-Request-ID", "X-Tenant-ID", "X-Restaurant-ID",
			"Cache-Control", "If-None-Match",
		},
		ExposeHeaders: []string{
			"X-Request-ID", "X-RateLimit-Limit", "X-RateLimit-Remaining",
			"X-RateLimit-Reset", "Retry-After", "ETag", "Content-Length",
		},
		AllowCredentials: true,
		MaxAge:           86400, // 24 hours
	}
}

// CORS returns the CORS middleware with default configuration.
func CORS() fiber.Handler {
	return CORSWithConfig(DefaultCORSConfig())
}

// CORSWithConfig returns CORS middleware with the given configuration.
func CORSWithConfig(cfg CORSConfig) fiber.Handler {
	// Pre-build joined strings for response headers
	allowMethods := strings.Join(cfg.AllowMethods, ", ")
	allowHeaders := strings.Join(cfg.AllowHeaders, ", ")
	exposeHeaders := strings.Join(cfg.ExposeHeaders, ", ")

	// Build origin lookup set for O(1) checks
	originSet := make(map[string]struct{}, len(cfg.AllowOrigins))
	allowAll := false
	for _, o := range cfg.AllowOrigins {
		if o == "*" {
			allowAll = true
		}
		originSet[strings.TrimRight(o, "/")] = struct{}{}
	}

	return func(c fiber.Ctx) error {
		origin := c.Get("Origin")

		// ── Determine if origin is allowed ─────────────────────
		originAllowed := false
		if origin != "" {
			normalizedOrigin := strings.TrimRight(origin, "/")
			if allowAll {
				originAllowed = true
			} else if _, exists := originSet[normalizedOrigin]; exists {
				originAllowed = true
			} else {
				// Wildcard subdomain matching: if config has "*.example.com",
				// match "app.example.com"
				for allowed := range originSet {
					if strings.HasPrefix(allowed, "*.") {
						domain := allowed[1:] // ".example.com"
						if strings.HasSuffix(normalizedOrigin, domain) {
							originAllowed = true
							break
						}
					}
				}
			}
		}

		// If origin is not allowed, still process the request but without CORS headers.
		// This prevents the browser from reading the response, but the request still executes.
		if !originAllowed && origin != "" {
			// For preflight, reject immediately
			if c.Method() == "OPTIONS" {
				return c.SendStatus(fiber.StatusForbidden)
			}
			// For simple requests, proceed without CORS headers (browser blocks response)
			return c.Next()
		}

		// ── Set CORS response headers ──────────────────────────
		if origin != "" {
			// Always echo back the specific origin (not *) when using credentials
			if cfg.AllowCredentials {
				c.Set("Access-Control-Allow-Origin", origin)
			} else if allowAll {
				c.Set("Access-Control-Allow-Origin", "*")
			} else {
				c.Set("Access-Control-Allow-Origin", origin)
			}
		}

		if cfg.AllowCredentials {
			c.Set("Access-Control-Allow-Credentials", "true")
		}

		if exposeHeaders != "" {
			c.Set("Access-Control-Expose-Headers", exposeHeaders)
		}

		// Vary header to ensure caches key on Origin
		c.Set("Vary", "Origin")

		// ── Handle preflight (OPTIONS) ─────────────────────────
		if c.Method() == "OPTIONS" {
			c.Set("Access-Control-Allow-Methods", allowMethods)
			c.Set("Access-Control-Allow-Headers", allowHeaders)

			if cfg.MaxAge > 0 {
				c.Set("Access-Control-Max-Age", strings.TrimSpace(
					strings.Replace(
						strings.Replace(
							formatInt(cfg.MaxAge), ",", "", -1),
						" ", "", -1)))
			}

			return c.SendStatus(fiber.StatusNoContent)
		}

		return c.Next()
	}
}

// formatInt converts an int to its string representation without importing strconv
// to keep the import list clean. For CORS max-age we only need positive integers.
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

package middleware

import (
	"backend/utils"
	"time"

	"github.com/gofiber/fiber/v3"
)

// ============================================================================
// REQUEST TIMEOUT MIDDLEWARE
// Enforces a maximum execution time for each request. If the handler
// exceeds the configured deadline, the middleware returns a 503 Service
// Unavailable response. Different route groups can have different timeouts.
// ============================================================================

// TimeoutConfig configures the timeout middleware.
type TimeoutConfig struct {
	// DefaultTimeout is the default request timeout.
	DefaultTimeout time.Duration

	// RouteTimeouts maps route prefixes to specific timeouts.
	// More specific prefixes take priority over less specific ones.
	RouteTimeouts map[string]time.Duration

	// OnTimeout is an optional callback invoked when a request times out.
	OnTimeout func(c fiber.Ctx, timeout time.Duration)
}

// DefaultTimeoutConfig returns a sensible default timeout configuration
// tailored for a restaurant management system.
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		DefaultTimeout: 30 * time.Second,
		RouteTimeouts: map[string]time.Duration{
			// Analytics and reports can be slow
			"/api/V1/analytics": 60 * time.Second,
			"/api/V1/reports":   60 * time.Second,
			"/api/V1/forecast":  60 * time.Second,
			"/api/V1/export":    120 * time.Second,

			// Auth operations should be fast
			"/api/V1/auth": 10 * time.Second,

			// File uploads may need more time
			"/api/V1/upload": 120 * time.Second,

			// Health checks should be instant
			"/api/V1/health": 5 * time.Second,
			"/api/V1/ping":   5 * time.Second,
		},
	}
}

// Timeout returns the timeout middleware with default config.
func Timeout() fiber.Handler {
	return TimeoutWithConfig(DefaultTimeoutConfig())
}

// TimeoutWithConfig returns the timeout middleware with custom config.
func TimeoutWithConfig(cfg TimeoutConfig) fiber.Handler {
	if cfg.DefaultTimeout == 0 {
		cfg.DefaultTimeout = 30 * time.Second
	}

	return func(c fiber.Ctx) error {
		timeout := resolveTimeout(c.Path(), cfg)

		// Create a done channel to signal handler completion
		done := make(chan error, 1)

		go func() {
			done <- c.Next()
		}()

		select {
		case err := <-done:
			return err
		case <-time.After(timeout):
			// Handler exceeded timeout
			if cfg.OnTimeout != nil {
				cfg.OnTimeout(c, timeout)
			}

			return utils.SendResponse(c, fiber.StatusServiceUnavailable,
				"Request timed out. Please try again.", map[string]interface{}{
					"timeout_seconds": timeout.Seconds(),
					"request_id":      GetRequestID(c),
				})
		}
	}
}

// resolveTimeout finds the most specific route timeout matching the path.
func resolveTimeout(path string, cfg TimeoutConfig) time.Duration {
	bestMatch := ""
	bestTimeout := cfg.DefaultTimeout

	for prefix, t := range cfg.RouteTimeouts {
		if len(prefix) > len(bestMatch) && pathHasPrefix(path, prefix) {
			bestMatch = prefix
			bestTimeout = t
		}
	}

	return bestTimeout
}

// pathHasPrefix checks if a path starts with the given prefix.
func pathHasPrefix(path, prefix string) bool {
	if len(path) < len(prefix) {
		return false
	}
	return path[:len(prefix)] == prefix
}

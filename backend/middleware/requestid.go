package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// ============================================================================
// REQUEST ID MIDDLEWARE
// Generates a UUID v4 for each request (or honors an incoming X-Request-ID
// header), injects it into Locals("requestId"), and sets it on the response
// X-Request-ID header for end-to-end distributed tracing.
// ============================================================================

const (
	HeaderRequestID = "X-Request-ID"
	LocalsRequestID = "requestId"
)

// RequestID returns middleware that injects a request ID into every request.
// If the client sends a valid X-Request-ID header, that value is reused
// (useful when an API gateway or load balancer already assigned one).
// Otherwise a fresh UUID v4 is generated.
func RequestID() fiber.Handler {
	return func(c fiber.Ctx) error {
		rid := c.Get(HeaderRequestID)

		// Validate incoming request ID — must be a valid UUID
		if rid != "" {
			if _, err := uuid.Parse(rid); err != nil {
				// Client sent a garbage request ID; replace it
				rid = ""
			}
		}

		// Generate a new one if missing or invalid
		if rid == "" {
			rid = uuid.New().String()
		}

		// Store in locals for downstream middleware/handlers
		c.Locals(LocalsRequestID, rid)

		// Set on response so the caller can correlate
		c.Set(HeaderRequestID, rid)

		return c.Next()
	}
}

// GetRequestID extracts the request ID from fiber context locals.
// Returns empty string if not set (i.e. RequestID middleware was not used).
func GetRequestID(c fiber.Ctx) string {
	if rid, ok := c.Locals(LocalsRequestID).(string); ok {
		return rid
	}
	return ""
}

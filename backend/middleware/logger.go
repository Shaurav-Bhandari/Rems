package middleware

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

// ============================================================================
// STRUCTURED REQUEST/RESPONSE LOGGING MIDDLEWARE
// Logs every request with method, path, status, latency, request ID, user ID,
// tenant ID, IP, user agent, and response size in JSON format. Includes
// slow-request detection, status-based log levels, and PII redaction.
// ============================================================================

// LoggerConfig configures the structured logging middleware.
type LoggerConfig struct {
	// SlowRequestThreshold is the latency above which a WARN is logged.
	SlowRequestThreshold time.Duration

	// SkipPaths are paths that should not be logged (e.g. health checks).
	SkipPaths []string

	// RedactedFields are JSON field names whose values are replaced with
	// "[REDACTED]" when logging request bodies.
	RedactedFields []string

	// LogRequestBody enables logging of POST/PUT/PATCH request bodies.
	LogRequestBody bool

	// MaxBodyLogSize is the maximum number of bytes of the request body to log.
	MaxBodyLogSize int
}

// DefaultLoggerConfig returns a sensible default logger configuration.
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		SlowRequestThreshold: 2 * time.Second,
		SkipPaths:            []string{"/api/v1/health", "/api/v1/ping"},
		RedactedFields: []string{
			"password", "password_confirm", "current_password",
			"new_password", "confirm_password", "token",
			"refresh_token", "secret", "credit_card", "card_number",
			"cvv", "ssn", "two_factor_secret", "backup_codes",
		},
		LogRequestBody: true,
		MaxBodyLogSize: 4096, // 4KB
	}
}

// Logger returns the structured logging middleware with default config.
func Logger() fiber.Handler {
	return LoggerWithConfig(DefaultLoggerConfig())
}

// LoggerWithConfig returns the logging middleware with custom config.
func LoggerWithConfig(cfg LoggerConfig) fiber.Handler {
	// Build skip-path set for O(1) lookups
	skipSet := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skipSet[p] = struct{}{}
	}

	// Build redacted field set
	redactSet := make(map[string]struct{}, len(cfg.RedactedFields))
	for _, f := range cfg.RedactedFields {
		redactSet[strings.ToLower(f)] = struct{}{}
	}

	return func(c fiber.Ctx) error {
		path := c.Path()

		// Skip logging for configured paths
		if _, skip := skipSet[path]; skip {
			return c.Next()
		}

		start := time.Now()

		// ── Process request ────────────────────────────────────
		err := c.Next()

		// ── Compute metrics ────────────────────────────────────
		latency := time.Since(start)
		status := c.Response().StatusCode()

		// ── Determine log level ────────────────────────────────
		level := "INFO"
		switch {
		case status >= 500:
			level = "ERROR"
		case status >= 400:
			level = "WARN"
		case status >= 300:
			level = "INFO"
		}

		// Slow request override
		isSlow := latency >= cfg.SlowRequestThreshold
		if isSlow && level == "INFO" {
			level = "WARN"
		}

		// ── Build log entry ────────────────────────────────────
		entry := map[string]interface{}{
			"level":         level,
			"event":         "http_request",
			"method":        c.Method(),
			"path":          path,
			"status":        status,
			"latency_ms":    float64(latency.Microseconds()) / 1000.0,
			"latency_human": latency.String(),
			"ip":            GetRealIP(c),
			"user_agent":    c.Get("User-Agent"),
			"request_id":    GetRequestID(c),
			"response_size": len(c.Response().Body()),
			"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
		}

		// Add auth context fields if available
		if auth := GetAuthContext(c); auth != nil {
			entry["user_id"] = auth.UserID.String()
			entry["tenant_id"] = auth.TenantID.String()
			entry["role"] = auth.Role
		}

		// Flag slow requests
		if isSlow {
			entry["slow_request"] = true
			entry["slow_threshold_ms"] = cfg.SlowRequestThreshold.Milliseconds()
		}

		// Log request body for mutation methods (with PII redaction)
		method := c.Method()
		if cfg.LogRequestBody && (method == "POST" || method == "PUT" || method == "PATCH") {
			body := c.Body()
			if len(body) > 0 && len(body) <= cfg.MaxBodyLogSize {
				redacted := redactBody(body, redactSet)
				entry["request_body"] = redacted
			} else if len(body) > cfg.MaxBodyLogSize {
				entry["request_body"] = "[TRUNCATED]"
				entry["request_body_size"] = len(body)
			}
		}

		// Add query params for GET requests if present
		if method == "GET" && c.Request().URI().QueryString() != nil {
			entry["query"] = string(c.Request().URI().QueryString())
		}

		// ── Output ─────────────────────────────────────────────
		logJSON, _ := json.Marshal(entry)
		log.Printf("[HTTP] %s", string(logJSON))

		return err
	}
}

// redactBody parses a JSON body and replaces sensitive fields with "[REDACTED]".
func redactBody(body []byte, redactSet map[string]struct{}) interface{} {
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		// Not valid JSON — return a truncated string
		s := string(body)
		if len(s) > 200 {
			s = s[:200] + "..."
		}
		return s
	}

	redactMap(parsed, redactSet)
	return parsed
}

// redactMap recursively redacts sensitive fields in a JSON object.
func redactMap(m map[string]interface{}, redactSet map[string]struct{}) {
	for key, val := range m {
		if _, sensitive := redactSet[strings.ToLower(key)]; sensitive {
			m[key] = "[REDACTED]"
			continue
		}
		// Recurse into nested objects
		switch v := val.(type) {
		case map[string]interface{}:
			redactMap(v, redactSet)
		case []interface{}:
			for _, item := range v {
				if nested, ok := item.(map[string]interface{}); ok {
					redactMap(nested, redactSet)
				}
			}
		}
	}
}

package middleware

import (
	"backend/utils"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

// ============================================================================
// RECOVERY MIDDLEWARE
// Catches panics, logs full stack traces, tracks repeated panics per route,
// and returns sanitized error responses. In dev mode, the stack trace is
// included in the response body; in production only a generic 500 is returned.
// ============================================================================

// RecoveryConfig holds configuration for the recovery middleware.
type RecoveryConfig struct {
	// EnableStackTrace includes the stack trace in the response body.
	// Should only be true in development environments.
	EnableStackTrace bool

	// StackTraceDepth is the max number of frames to capture.
	StackTraceDepth int

	// PanicThreshold is the number of panics on the same route within
	// PanicWindow before a critical alert is logged.
	PanicThreshold int

	// PanicWindow is the time window for counting repeated panics.
	PanicWindow time.Duration

	// OnPanic is an optional callback invoked after a panic is recovered.
	// Receives the route path, the recovered value, and the stack trace.
	OnPanic func(path string, err interface{}, stack string)
}

// DefaultRecoveryConfig returns a sensible default configuration.
func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{
		EnableStackTrace: os.Getenv("APP_ENV") != "production",
		StackTraceDepth:  32,
		PanicThreshold:   5,
		PanicWindow:      10 * time.Minute,
	}
}

// panicRecord tracks panics per route for threshold alerting.
type panicRecord struct {
	count     int
	firstSeen time.Time
}

// Recovery returns middleware that recovers from panics.
func Recovery() fiber.Handler {
	cfg := DefaultRecoveryConfig()
	return RecoveryWithConfig(cfg)
}

// RecoveryWithConfig returns panic-recovery middleware with the given config.
func RecoveryWithConfig(cfg RecoveryConfig) fiber.Handler {
	if cfg.StackTraceDepth == 0 {
		cfg.StackTraceDepth = 32
	}
	if cfg.PanicThreshold == 0 {
		cfg.PanicThreshold = 5
	}
	if cfg.PanicWindow == 0 {
		cfg.PanicWindow = 10 * time.Minute
	}

	var (
		mu      sync.Mutex
		tracker = make(map[string]*panicRecord)
	)

	return func(c fiber.Ctx) (err error) {
		defer func() {
			if r := recover(); r != nil {
				// ── Capture stack trace ─────────────────────────
				stack := captureStack(cfg.StackTraceDepth)
				path := c.Path()
				rid := GetRequestID(c)

				// ── Log the panic ──────────────────────────────
				logEntry := map[string]interface{}{
					"level":      "FATAL",
					"event":      "panic_recovered",
					"path":       path,
					"method":     c.Method(),
					"request_id": rid,
					"panic":      fmt.Sprintf("%v", r),
					"stack":      stack,
					"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
				}
				logJSON, _ := json.Marshal(logEntry)
				log.Printf("[PANIC] %s", string(logJSON))

				// ── Track repeated panics ──────────────────────
				mu.Lock()
				rec, exists := tracker[path]
				if !exists || time.Since(rec.firstSeen) > cfg.PanicWindow {
					tracker[path] = &panicRecord{count: 1, firstSeen: time.Now()}
				} else {
					rec.count++
					if rec.count >= cfg.PanicThreshold {
						log.Printf("[CRITICAL] Route %q has panicked %d times in %v — possible systemic failure",
							path, rec.count, cfg.PanicWindow)
					}
				}
				mu.Unlock()

				// ── Invoke optional callback ───────────────────
				if cfg.OnPanic != nil {
					go cfg.OnPanic(path, r, stack)
				}

				// ── Build response ─────────────────────────────
				errData := map[string]interface{}{
					"error":      "Internal Server Error",
					"request_id": rid,
				}
				if cfg.EnableStackTrace {
					errData["panic"] = fmt.Sprintf("%v", r)
					errData["stack_trace"] = strings.Split(stack, "\n")
				}

				err = utils.SendResponse(c, fiber.StatusInternalServerError,
					"An unexpected error occurred. Please try again later.", errData)
			}
		}()

		return c.Next()
	}
}

// captureStack returns a formatted stack trace string.
func captureStack(depth int) string {
	pcs := make([]uintptr, depth)
	n := runtime.Callers(4, pcs) // skip Callers, captureStack, deferred func, recover
	pcs = pcs[:n]

	var sb strings.Builder
	frames := runtime.CallersFrames(pcs)
	for {
		frame, more := frames.Next()
		fmt.Fprintf(&sb, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}
	return sb.String()
}

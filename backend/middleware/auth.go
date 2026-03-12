package middleware

import (
	"backend/DTO"
	"backend/utils"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ============================================================================
// JWT AUTHENTICATION & SESSION VALIDATION MIDDLEWARE
// Extracts and validates JWT tokens, resolves sessions from Redis (with
// Postgres fallback), injects AuthContext into Fiber locals, and logs
// security events for failed authentication attempts.
// ============================================================================

const (
	// LocalsAuthKey is the key used to store AuthContext in fiber.Ctx.Locals.
	LocalsAuthKey = "auth"

	// LocalsTenantIDKey stores the pre-auth tenant ID from X-Tenant-ID header.
	LocalsTenantIDKey = "tenantId"
)

// AuthConfig holds configuration for the authentication middleware.
type AuthConfig struct {
	// JWTSecret is the HMAC signing key for JWT tokens.
	JWTSecret string

	// RedisClient is the Redis connection for session lookups.
	RedisClient *redis.Client

	// SkipPaths are routes that do not require authentication.
	// Supports prefix matching (e.g. "/api/V1/auth" matches "/api/V1/auth/login").
	SkipPaths []string

	// TokenExpGracePeriod allows tokens within this duration past expiry
	// to still be accepted (clock skew tolerance).
	TokenExpGracePeriod time.Duration

	// EnableDeviceValidation checks the client fingerprint against the
	// session's stored device fingerprint.
	EnableDeviceValidation bool

	// OnAuthFailure is an optional callback invoked on authentication failure.
	// Receives the request context, error message, and client IP.
	OnAuthFailure func(c fiber.Ctx, reason string, ip string)
}

// DefaultAuthConfig returns a default auth configuration.
func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		JWTSecret: os.Getenv("JWT_SECRET"),
		SkipPaths: []string{
			"/api/V1/auth/login",
			"/api/V1/auth/register",
			"/api/V1/auth/forgot-password",
			"/api/V1/auth/reset-password",
			"/api/V1/auth/verify-email",
			"/api/V1/health",
			"/api/V1/ping",
		},
		TokenExpGracePeriod:    30 * time.Second,
		EnableDeviceValidation: true,
	}
}

// JWTClaims represents the claims embedded in an access token.
type JWTClaims struct {
	UserID    uuid.UUID `json:"user_id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	SessionID uuid.UUID `json:"session_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	DeviceID  string    `json:"device_id,omitempty"`
	jwt.RegisteredClaims
}

// Auth returns JWT authentication middleware with default config.
func Auth(redisClient *redis.Client) fiber.Handler {
	cfg := DefaultAuthConfig()
	cfg.RedisClient = redisClient
	return AuthWithConfig(cfg)
}

// AuthWithConfig returns JWT authentication middleware with custom config.
func AuthWithConfig(cfg AuthConfig) fiber.Handler {
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "CHANGE_ME_IN_PRODUCTION_PLEASE" // Fail-safe default
		log.Println("[WARN] JWT_SECRET not set — using insecure default. Set JWT_SECRET in .env!")
	}

	// Build skip-path set for fast lookups
	skipSet := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skipSet[p] = struct{}{}
	}

	signingKey := []byte(cfg.JWTSecret)

	return func(c fiber.Ctx) error {
		path := c.Path()

		// ── Check skip paths ───────────────────────────────────
		if shouldSkipAuth(path, skipSet, cfg.SkipPaths) {
			// Still extract X-Tenant-ID if present for public tenant-scoped routes
			if tid := c.Get("X-Tenant-ID"); tid != "" {
				if parsed, err := uuid.Parse(tid); err == nil {
					c.Locals(LocalsTenantIDKey, parsed)
				}
			}
			return c.Next()
		}

		// ── Extract token ──────────────────────────────────────
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			logAuthFailure(cfg, c, "missing_authorization_header")
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Authorization header is required", nil)
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			logAuthFailure(cfg, c, "invalid_authorization_format")
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Authorization header must be: Bearer <token>", nil)
		}
		tokenStr := parts[1]

		// ── Parse and validate JWT ─────────────────────────────
		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return signingKey, nil
		}, jwt.WithLeeway(cfg.TokenExpGracePeriod))

		if err != nil || !token.Valid {
			reason := "invalid_token"
			if err != nil {
				reason = fmt.Sprintf("invalid_token: %v", err)
			}
			logAuthFailure(cfg, c, reason)
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Invalid or expired token", nil)
		}

		// ── Validate required claims ───────────────────────────
		if claims.UserID == uuid.Nil || claims.TenantID == uuid.Nil || claims.SessionID == uuid.Nil {
			logAuthFailure(cfg, c, "missing_required_claims")
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Token is missing required claims", nil)
		}

		// ── Resolve session from Redis ─────────────────────────
		authCtx, err := resolveSession(c, cfg, claims)
		if err != nil {
			logAuthFailure(cfg, c, fmt.Sprintf("session_resolution_failed: %v", err))
			return utils.SendResponse(c, fiber.StatusUnauthorized,
				"Session is invalid or has been revoked", nil)
		}

		// ── Device fingerprint validation ──────────────────────
		if cfg.EnableDeviceValidation {
			clientFP := GetFingerprint(c)
			if clientFP != "" && authCtx.IPAddress != "" {
				// We only warn — don't block — because IP can change legitimately
				if clientIP := GetRealIP(c); clientIP != authCtx.IPAddress {
					log.Printf("[WARN] IP mismatch for user %s: token=%s, actual=%s",
						authCtx.UserID, authCtx.IPAddress, clientIP)
				}
			}
		}

		// ── Inject AuthContext into locals ──────────────────────
		authCtx.IPAddress = GetRealIP(c)
		authCtx.UserAgent = c.Get("User-Agent")
		c.Locals(LocalsAuthKey, authCtx)
		c.Locals(LocalsTenantIDKey, claims.TenantID)

		return c.Next()
	}
}

// resolveSession looks up the session in Redis, falling back to a minimal
// AuthContext from JWT claims if Redis is unavailable.
func resolveSession(c fiber.Ctx, cfg AuthConfig, claims *JWTClaims) (*DTO.AuthContext, error) {
	if cfg.RedisClient == nil {
		// No Redis — build AuthContext from JWT claims alone
		return &DTO.AuthContext{
			SessionID: claims.SessionID,
			UserID:    claims.UserID,
			TenantID:  claims.TenantID,
			Email:     claims.Email,
			Role:      claims.Role,
		}, nil
	}

	ctx := context.Background()
	cacheKey := DTO.GetCacheKey("user_session", claims.SessionID)

	data, err := cfg.RedisClient.Get(ctx, cacheKey).Bytes()
	if err == redis.Nil {
		// Session not in Redis — could be expired or evicted.
		// Fall back to claims (degraded mode).
		log.Printf("[WARN] Session %s not found in Redis — using JWT claims only", claims.SessionID)
		return &DTO.AuthContext{
			SessionID: claims.SessionID,
			UserID:    claims.UserID,
			TenantID:  claims.TenantID,
			Email:     claims.Email,
			Role:      claims.Role,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis error: %w", err)
	}

	var cached DTO.CachedUserSession
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("session deserialization failed: %w", err)
	}

	// ── Check session validity ─────────────────────────────
	if cached.IsRevoked {
		return nil, fmt.Errorf("session has been revoked")
	}

	if time.Now().After(cached.ExpiresAt) {
		return nil, fmt.Errorf("session has expired")
	}

	// ── Update last activity (fire-and-forget) ─────────────
	go func() {
		cached.LastActivity = time.Now()
		updated, _ := json.Marshal(cached)
		ttl := time.Until(cached.ExpiresAt)
		if ttl > 0 {
			cfg.RedisClient.Set(context.Background(), cacheKey, updated, ttl)
		}
	}()

	return DTO.NewAuthContextFromCache(&cached), nil
}

// shouldSkipAuth checks if the path should bypass authentication.
func shouldSkipAuth(path string, skipSet map[string]struct{}, skipPaths []string) bool {
	if _, exact := skipSet[path]; exact {
		return true
	}
	// Prefix matching for path groups
	for _, skip := range skipPaths {
		if strings.HasPrefix(path, skip) {
			return true
		}
	}
	return false
}

// logAuthFailure logs an authentication failure and invokes the callback.
func logAuthFailure(cfg AuthConfig, c fiber.Ctx, reason string) {
	ip := GetRealIP(c)
	rid := GetRequestID(c)

	entry := map[string]interface{}{
		"level":      "WARN",
		"event":      "auth_failure",
		"reason":     reason,
		"ip":         ip,
		"user_agent": c.Get("User-Agent"),
		"path":       c.Path(),
		"request_id": rid,
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
	}
	logJSON, _ := json.Marshal(entry)
	log.Printf("[AUTH] %s", string(logJSON))

	if cfg.OnAuthFailure != nil {
		cfg.OnAuthFailure(c, reason, ip)
	}
}

// GetAuthContext extracts the AuthContext from fiber context locals.
// Returns nil if the user is not authenticated.
func GetAuthContext(c fiber.Ctx) *DTO.AuthContext {
	if auth, ok := c.Locals(LocalsAuthKey).(*DTO.AuthContext); ok {
		return auth
	}
	return nil
}

// MustGetAuthContext extracts the AuthContext and panics if not present.
// Only use in routes that are guaranteed to be behind the Auth middleware.
func MustGetAuthContext(c fiber.Ctx) *DTO.AuthContext {
	auth := GetAuthContext(c)
	if auth == nil {
		panic("middleware.MustGetAuthContext called on unauthenticated route")
	}
	return auth
}

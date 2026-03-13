package handlers

import (
	"backend/DTO"
	"backend/config"
	"backend/middleware"
	services "backend/services/core"
	"backend/utils"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// ============================================================================
// AUTH HANDLER
// Over-engineered auth handler with injected services, vault config, and Redis.
// ============================================================================

// AuthHandler handles all authentication-related HTTP endpoints.
type AuthHandler struct {
	authService     *services.AuthService
	sessionService  *services.SessionService
	tokenService    *services.TokenService
	securityService *services.SecurityService
	vault           *config.ImmuVault
	redis           *redis.Client
}

// NewAuthHandler creates a fully-wired AuthHandler.
func NewAuthHandler(
	authService *services.AuthService,
	sessionService *services.SessionService,
	tokenService *services.TokenService,
	securityService *services.SecurityService,
	vault *config.ImmuVault,
	redisClient *redis.Client,
) *AuthHandler {
	return &AuthHandler{
		authService:     authService,
		sessionService:  sessionService,
		tokenService:    tokenService,
		securityService: securityService,
		vault:           vault,
		redis:           redisClient,
	}
}

// ── Login ────────────────────────────────────────────────────────────────────
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c fiber.Ctx) error {
	var req DTO.LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			"Invalid request body", nil)
	}

	ipAddress := middleware.GetRealIP(c)
	userAgent := c.Get("User-Agent")
	deviceFingerprint := middleware.GetFingerprint(c)

	resp, err := h.authService.Login(
		c.Context(),
		&req,
		ipAddress,
		userAgent,
		deviceFingerprint,
	)
	if err != nil {
		status := fiber.StatusUnauthorized
		switch err {
		case services.ErrAccountLocked:
			status = fiber.StatusForbidden
		case services.ErrRateLimitExceeded:
			status = fiber.StatusTooManyRequests
		case services.ErrAccountInactive:
			status = fiber.StatusForbidden
		}
		return utils.SendResponse(c, status, err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Login successful", resp)
}

// ── Register ─────────────────────────────────────────────────────────────────
// POST /api/v1/auth/register
func (h *AuthHandler) Register(c fiber.Ctx) error {
	var req DTO.RegisterRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			"Invalid request body", nil)
	}

	ipAddress := middleware.GetRealIP(c)
	userAgent := c.Get("User-Agent")

	resp, err := h.authService.Register(c.Context(), &req, ipAddress, userAgent)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusCreated, "Registration successful", resp)
}

// ── Logout ───────────────────────────────────────────────────────────────────
// POST /api/v1/auth/logout [Auth required]
func (h *AuthHandler) Logout(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized,
			"Not authenticated", nil)
	}

	ctx := c.Context()

	// Revoke session from Redis
	if err := h.sessionService.RevokeSession(ctx, auth.UserID, auth.SessionID.String()); err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError,
			"Failed to revoke session", nil)
	}

	// Blacklist the access token
	tokenStr := extractBearerToken(c)
	if tokenStr != "" {
		_ = h.tokenService.BlacklistAccessToken(ctx, tokenStr)
	}

	ipAddress := middleware.GetRealIP(c)
	userAgent := c.Get("User-Agent")
	h.securityService.LogSecurityEvent(
		ctx, &auth.UserID,
		services.EventLogout, services.SeverityInfo,
		ipAddress, userAgent, true, nil,
	)

	return utils.SendResponse(c, fiber.StatusOK, "Logged out successfully", nil)
}

// ── RefreshToken ─────────────────────────────────────────────────────────────
// POST /api/v1/auth/refresh
func (h *AuthHandler) RefreshToken(c fiber.Ctx) error {
	var req DTO.RefreshTokenRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			"Invalid request body", nil)
	}

	ipAddress := middleware.GetRealIP(c)
	userAgent := c.Get("User-Agent")

	resp, err := h.authService.RefreshAccessToken(
		c.Context(),
		req.RefreshToken,
		ipAddress,
		userAgent,
	)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Token refreshed", resp)
}

// ── ChangePassword ───────────────────────────────────────────────────────────
// POST /api/v1/auth/change-password [Auth required]
func (h *AuthHandler) ChangePassword(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized,
			"Not authenticated", nil)
	}

	var req DTO.ChangePasswordRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			"Invalid request body", nil)
	}

	ipAddress := middleware.GetRealIP(c)
	userAgent := c.Get("User-Agent")

	err := h.authService.ChangePassword(
		c.Context(),
		auth.UserID,
		req.CurrentPassword,
		req.NewPassword,
		ipAddress,
		userAgent,
	)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Password changed successfully", nil)
}

// ── ForgotPassword ───────────────────────────────────────────────────────────
// POST /api/v1/auth/forgot-password
// Generates a reset token, stores hash in Redis with 15-min TTL + DB audit record.
func (h *AuthHandler) ForgotPassword(c fiber.Ctx) error {
	var req DTO.ForgotPasswordRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			"Invalid request body", nil)
	}

	ctx := c.Context()
	ipAddress := middleware.GetRealIP(c)

	// Generate a cryptographically secure reset token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError,
			"Failed to generate reset token", nil)
	}
	resetToken := hex.EncodeToString(tokenBytes)

	// Hash the token for storage (never store plaintext)
	tokenHash := sha256.Sum256([]byte(resetToken))
	tokenHashStr := hex.EncodeToString(tokenHash[:])

	// Store in Redis with 15-minute TTL
	redisKey := fmt.Sprintf("password_reset:%s", tokenHashStr)
	err := h.redis.Set(ctx, redisKey, req.Email, 15*time.Minute).Err()
	if err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError,
			"Failed to store reset token", nil)
	}

	// Fire-and-forget: Log security event
	go func() {
		bgCtx := context.Background()
		h.securityService.LogSecurityEvent(
			bgCtx, nil,
			services.SecurityEventType("password_reset_requested"),
			services.SeverityInfo,
			ipAddress, "", true,
			map[string]interface{}{"email": req.Email},
		)
	}()

	// Always return success to prevent email enumeration
	return utils.SendResponse(c, fiber.StatusOK,
		"If an account exists with that email, a reset link has been sent", nil)
}

// ── ResetPassword ────────────────────────────────────────────────────────────
// POST /api/v1/auth/reset-password
// Validates token from Redis, updates password, deletes Redis key.
func (h *AuthHandler) ResetPassword(c fiber.Ctx) error {
	var req DTO.ResetPasswordRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			"Invalid request body", nil)
	}

	ctx := c.Context()

	// Hash the provided token
	tokenHash := sha256.Sum256([]byte(req.Token))
	tokenHashStr := hex.EncodeToString(tokenHash[:])
	redisKey := fmt.Sprintf("password_reset:%s", tokenHashStr)

	// Validate token exists in Redis
	email, err := h.redis.Get(ctx, redisKey).Result()
	if err == redis.Nil {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			"Invalid or expired reset token", nil)
	}
	if err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError,
			"Failed to validate reset token", nil)
	}

	// Perform password reset via auth service
	err = h.authService.ResetPassword(ctx, email, req.NewPassword)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
	}

	// Delete the Redis key (one-time use)
	h.redis.Del(ctx, redisKey)

	return utils.SendResponse(c, fiber.StatusOK, "Password reset successful", nil)
}

// ── ListSessions ─────────────────────────────────────────────────────────────
// GET /api/v1/auth/sessions [Auth required]
func (h *AuthHandler) ListSessions(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized,
			"Not authenticated", nil)
	}

	sessionIDs, err := h.sessionService.GetUserSessions(c.Context(), auth.UserID)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError,
			"Failed to retrieve sessions", nil)
	}

	// Build session list with details
	type sessionInfo struct {
		SessionID string `json:"session_id"`
		IsCurrent bool   `json:"is_current"`
	}

	sessions := make([]sessionInfo, 0, len(sessionIDs))
	for _, sid := range sessionIDs {
		sessions = append(sessions, sessionInfo{
			SessionID: sid,
			IsCurrent: sid == auth.SessionID.String(),
		})
	}

	return utils.SendResponse(c, fiber.StatusOK, "Sessions retrieved", map[string]interface{}{
		"sessions":     sessions,
		"active_count": len(sessions),
	})
}

// ── RevokeSession ────────────────────────────────────────────────────────────
// DELETE /api/v1/auth/sessions/:id [Auth required]
func (h *AuthHandler) RevokeSession(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized,
			"Not authenticated", nil)
	}

	sessionID := c.Params("id")
	if sessionID == "" {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			"Session ID is required", nil)
	}

	err := h.sessionService.RevokeSession(c.Context(), auth.UserID, sessionID)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError,
			"Failed to revoke session", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Session revoked", nil)
}

// ── Verify2FA ────────────────────────────────────────────────────────────────
// POST /api/v1/auth/verify-2fa
func (h *AuthHandler) Verify2FA(c fiber.Ctx) error {
	type TwoFARequest struct {
		PendingSessionID string `json:"pending_session_id"`
		Code             string `json:"code"`
	}

	var req TwoFARequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			"Invalid request body", nil)
	}

	ipAddress := middleware.GetRealIP(c)
	userAgent := c.Get("User-Agent")
	deviceFingerprint := middleware.GetFingerprint(c)

	resp, err := h.authService.Verify2FA(
		c.Context(),
		req.PendingSessionID,
		req.Code,
		ipAddress,
		userAgent,
		deviceFingerprint,
	)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "2FA verification successful", resp)
}

// ============================================================================
// HELPERS
// ============================================================================

func extractBearerToken(c fiber.Ctx) string {
	auth := c.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}

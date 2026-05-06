package handlers

import (
	"backend/config"
	services "backend/services/core"
	"backend/utils"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// ============================================================================
// GOOGLE OAUTH HANDLER
// Handles the Google OAuth2 login flow: redirect to consent screen and
// process the callback with authorization code exchange.
// ============================================================================

// OAuthHandler handles Google OAuth authentication endpoints.
type OAuthHandler struct {
	oauthService *services.OAuthService
	oauthConfig  config.OAuthConfig
	redis        *redis.Client
}

// NewOAuthHandler creates a new OAuthHandler.
func NewOAuthHandler(
	oauthService *services.OAuthService,
	oauthConfig config.OAuthConfig,
	redisClient *redis.Client,
) *OAuthHandler {
	return &OAuthHandler{
		oauthService: oauthService,
		oauthConfig:  oauthConfig,
		redis:        redisClient,
	}
}

// GoogleLogin — GET /api/v1/auth/google
// Redirects the user to Google's consent screen.
func (h *OAuthHandler) GoogleLogin(c fiber.Ctx) error {
	if !h.oauthService.IsEnabled() {
		return utils.SendResponse(c, fiber.StatusServiceUnavailable,
			"Google OAuth is not configured", nil)
	}

	// Generate a CSRF state token
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError,
			"Failed to generate state token", nil)
	}
	state := hex.EncodeToString(stateBytes)

	// Store state in Redis with 10-minute TTL for CSRF validation
	if h.redis != nil {
		stateKey := fmt.Sprintf("oauth_state:%s", state)
		h.redis.Set(c.Context(), stateKey, "1", 10*time.Minute)
	}

	url := h.oauthService.GetGoogleLoginURL(state)
	return c.Redirect().Status(fiber.StatusTemporaryRedirect).To(url)
}

// GoogleCallback — GET /api/v1/auth/google/callback
// Handles the OAuth callback from Google, exchanges the code for tokens.
func (h *OAuthHandler) GoogleCallback(c fiber.Ctx) error {
	if !h.oauthService.IsEnabled() {
		return utils.SendResponse(c, fiber.StatusServiceUnavailable,
			"Google OAuth is not configured", nil)
	}

	// Check for OAuth errors
	if errMsg := c.Query("error"); errMsg != "" {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			fmt.Sprintf("OAuth error: %s", errMsg), nil)
	}

	code := c.Query("code")
	if code == "" {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			"Missing authorization code", nil)
	}

	// Validate CSRF state token
	state := c.Query("state")
	if state == "" {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			"Missing state parameter", nil)
	}

	// CSRF validation is mandatory - Redis must be available
	if h.redis == nil {
		return utils.SendResponse(c, fiber.StatusServiceUnavailable,
			"CSRF validation unavailable", nil)
	}

	stateKey := fmt.Sprintf("oauth_state:%s", state)
	result, err := h.redis.Get(c.Context(), stateKey).Result()
	if err != nil || result != "1" {
		return utils.SendResponse(c, fiber.StatusBadRequest,
			"Invalid or expired state token", nil)
	}
	// Delete state token (one-time use)
	h.redis.Del(c.Context(), stateKey)

	// Extract client info
	ipAddress := c.Get("X-Forwarded-For")
	if ipAddress == "" {
		ipAddress = c.IP()
	}
	userAgent := c.Get("User-Agent")
	deviceFingerprint := c.Get("X-Device-Fingerprint")

	// Exchange code for tokens and authenticate user
	resp, err := h.oauthService.HandleGoogleCallback(
		c.Context(),
		code,
		ipAddress,
		userAgent,
		deviceFingerprint,
	)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized,
			err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusOK,
		"Google login successful", resp)
}

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"backend/DTO"
	"backend/config"
	"backend/models"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// ============================================================================
// GOOGLE OAUTH SERVICE
// Handles the Google OAuth2 flow: consent URL generation, code exchange,
// user lookup/creation, and JWT token issuance.
// ============================================================================

// OAuthService manages Google OAuth authentication flows.
type OAuthService struct {
	db             *gorm.DB
	redis          *redis.Client
	oauthConfig    *oauth2.Config
	tokenService   *TokenService
	sessionService *SessionService
	geoIPService   *GeoIPService
	ttlConfig      *config.TokenTTLConfig
	enabled        bool
}

// NewOAuthService creates a fully-wired OAuthService.
func NewOAuthService(
	db *gorm.DB,
	redisClient *redis.Client,
	oauthCfg config.OAuthConfig,
	tokenService *TokenService,
	sessionService *SessionService,
	geoIPService *GeoIPService,
	ttlCfg *config.TokenTTLConfig,
) *OAuthService {
	return &OAuthService{
		db:             db,
		redis:          redisClient,
		oauthConfig:    oauthCfg.GoogleOAuth2Config(),
		tokenService:   tokenService,
		sessionService: sessionService,
		geoIPService:   geoIPService,
		ttlConfig:      ttlCfg,
		enabled:        oauthCfg.Enabled,
	}
}

// IsEnabled returns whether Google OAuth is configured and enabled.
func (s *OAuthService) IsEnabled() bool {
	return s.enabled
}

// GetGoogleLoginURL generates the Google consent screen URL.
// The state parameter should be a CSRF-protection token stored in the client session.
func (s *OAuthService) GetGoogleLoginURL(state string) string {
	return s.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// HandleGoogleCallback exchanges the authorization code for tokens,
// fetches Google user info, and either logs in an existing user or
// creates a new one. Returns JWT tokens for the authenticated user.
func (s *OAuthService) HandleGoogleCallback(
	ctx context.Context,
	code string,
	ipAddress, userAgent, deviceFingerprint string,
) (*DTO.LoginResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// 1. Exchange authorization code for OAuth tokens
	oauthToken, err := s.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oauth: failed to exchange code: %w", err)
	}

	// 2. Fetch user info from Google
	googleUser, err := s.fetchGoogleUserInfo(ctx, oauthToken)
	if err != nil {
		return nil, fmt.Errorf("oauth: failed to fetch user info: %w", err)
	}

	if googleUser.Email == "" {
		return nil, fmt.Errorf("oauth: google did not return an email address")
	}

	// 3. Find or create the user in the database
	user, err := s.findOrCreateGoogleUser(ctx, googleUser)
	if err != nil {
		return nil, fmt.Errorf("oauth: user resolution failed: %w", err)
	}

	// 4. Check if the user account is active
	if !user.IsActive {
		return nil, ErrAccountInactive
	}

	// 5. Issue JWT tokens
	tokens, err := s.issueTokens(ctx, user, ipAddress, userAgent, deviceFingerprint)
	if err != nil {
		return nil, fmt.Errorf("oauth: token issuance failed: %w", err)
	}

	// 6. Update login metadata
	location := s.geoIPService.Lookup(ipAddress)
	s.db.WithContext(ctx).Model(user).Updates(map[string]interface{}{
		"last_login_at":       time.Now(),
		"last_login_ip":       ipAddress,
		"last_login_location": location,
	})

	log.Printf("[OAUTH] Google login successful for %s (user_id=%s)", user.Email, user.UserID)

	return tokens, nil
}

// fetchGoogleUserInfo calls Google's userinfo endpoint with the OAuth access token.
func (s *OAuthService) fetchGoogleUserInfo(ctx context.Context, token *oauth2.Token) (*DTO.GoogleUserInfo, error) {
	client := s.oauthConfig.Client(ctx, token)

	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo returned status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo DTO.GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo: %w", err)
	}

	return &userInfo, nil
}

// findOrCreateGoogleUser looks up a user by Google ID or email.
// If no user exists, a new account is created with the Google profile data.
func (s *OAuthService) findOrCreateGoogleUser(ctx context.Context, googleUser *DTO.GoogleUserInfo) (*models.User, error) {
	var user models.User

	// First, try to find by Google ID (already linked)
	err := s.db.WithContext(ctx).
		Where("google_id = ? AND is_deleted = false", googleUser.Sub).
		First(&user).Error
	if err == nil {
		// Update avatar URL if changed
		if user.AvatarURL != googleUser.Picture {
			s.db.WithContext(ctx).Model(&user).Update("avatar_url", googleUser.Picture)
		}
		return &user, nil
	}

	// Next, try to find by email (user exists but hasn't linked Google yet)
	err = s.db.WithContext(ctx).
		Where("email = ? AND is_deleted = false", strings.ToLower(googleUser.Email)).
		First(&user).Error
	if err == nil {
		// Link the Google account to the existing user
		s.db.WithContext(ctx).Model(&user).Updates(map[string]interface{}{
			"google_id":         googleUser.Sub,
			"avatar_url":        googleUser.Picture,
			"is_email_verified": true,
		})
		return &user, nil
	}

	// No existing user — create a new one
	// Generate a placeholder tenant for the new OAuth user
	tenantID := uuid.New()
	tenant := models.Tenant{
		TenantID:  tenantID,
		Name:      googleUser.Name + "'s Organization",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(&tenant).Error; err != nil {
		return nil, fmt.Errorf("failed to create tenant for OAuth user: %w", err)
	}

	newUser := models.User{
		UserID:          uuid.New(),
		TenantID:        tenantID,
		UserName:        s.generateUsername(googleUser),
		FullName:        googleUser.Name,
		Email:           strings.ToLower(googleUser.Email),
		PasswordHash:    "", // OAuth users don't have a password
		GoogleID:        googleUser.Sub,
		AvatarURL:       googleUser.Picture,
		IsActive:        true,
		IsEmailVerified: googleUser.EmailVerified,
		PrimaryRole:     "owner",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := s.db.WithContext(ctx).Create(&newUser).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	log.Printf("[OAUTH] Created new user via Google: %s (user_id=%s)", newUser.Email, newUser.UserID)
	return &newUser, nil
}

// generateUsername creates a username from the Google profile.
func (s *OAuthService) generateUsername(googleUser *DTO.GoogleUserInfo) string {
	// Use the part before @ in the email
	parts := strings.Split(googleUser.Email, "@")
	base := parts[0]

	// Check uniqueness
	var count int64
	s.db.Model(&models.User{}).Where("user_name LIKE ?", base+"%").Count(&count)
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s_%d", base, count+1)
}

// issueTokens creates a session and JWT tokens for the user.
func (s *OAuthService) issueTokens(
	ctx context.Context,
	user *models.User,
	ipAddress, userAgent, deviceFingerprint string,
) (*DTO.LoginResponse, error) {
	sessionID := uuid.New().String()
	deviceType := "web"

	// Determine TTL
	ttl := s.ttlConfig.Default
	if roleTTL, exists := s.ttlConfig.ByRole[user.PrimaryRole]; exists {
		ttl = roleTTL
	}

	// Generate access token
	accessToken, err := s.tokenService.GenerateAccessToken(
		user.UserID, user.TenantID, sessionID, ttl.AccessToken,
	)
	if err != nil {
		return nil, err
	}

	// Generate refresh token
	refreshTokenString, err := s.tokenService.GenerateSecureToken(64)
	if err != nil {
		return nil, err
	}

	// Store refresh token hash in DB
	tokenHash := s.tokenService.HashToken(refreshTokenString)
	refreshToken := &models.RefreshToken{
		TokenID:           uuid.New(),
		UserID:            user.UserID,
		TokenHash:         tokenHash,
		DeviceID:          sessionID,
		DeviceFingerprint: deviceFingerprint,
		DeviceType:        deviceType,
		UserRole:          user.PrimaryRole,
		IPAddress:         ipAddress,
		UserAgent:         userAgent,
		Location:          s.geoIPService.Lookup(ipAddress),
		ExpiresAt:         time.Now().Add(ttl.RefreshToken),
		CreatedAt:         time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(refreshToken).Error; err != nil {
		return nil, err
	}

	// Create session in Redis
	sessionData := &SessionData{
		UserID:     user.UserID,
		TenantID:   user.TenantID,
		Email:      user.Email,
		Role:       user.PrimaryRole,
		DeviceType: deviceType,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		LoginAt:    time.Now(),
	}
	if err := s.sessionService.CreateSession(ctx, user.UserID, sessionID, sessionData, ttl.AccessToken); err != nil {
		return nil, err
	}

	// Load tenant name
	var tenant models.Tenant
	s.db.WithContext(ctx).First(&tenant, user.TenantID)

	expiresAt := time.Now().Add(ttl.AccessToken)
	return &DTO.LoginResponse{
		Token:        &accessToken,
		RefreshToken: &refreshTokenString,
		ExpiresAt:    &expiresAt,
		User: &DTO.UserInfo{
			UserID:      user.UserID,
			Email:       user.Email,
			FullName:    user.FullName,
			TenantID:    user.TenantID,
			TenantName:  tenant.Name,
			DefaultRole: user.PrimaryRole,
		},
	}, nil
}

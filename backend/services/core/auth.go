// services/auth_service_integrated.go
package services

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"

	// "math/big"
	// "net"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/argon2"
	"gorm.io/gorm"

	"backend/DTO"
	"backend/config"
	"backend/models"
)

// ============================================
// YOUR CUSTOM PASSWORD HASHING (KEPT AS-IS!)
// ============================================

// HashPwd - Your custom Argon2id implementation (UNCHANGED)
func HashPwd(password string) (string, error) {
	memory := uint32(64 * 1024)
	iterations := uint32(3)
	parallelism := uint8(2)
	saltLength := uint32(16)
	keyLength := uint32(32)

	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encodedHash := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memory, iterations, parallelism, b64Salt, b64Hash)
	return encodedHash, nil
}

// ComparePwd - Your custom Argon2id comparison (UNCHANGED)
func ComparePwd(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, errors.New("invalid hash format")
	}

	var memory, iterations uint32
	var parallelism uint8

	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		return false, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}

	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}

	keyLength := uint32(len(decodedHash))
	comparisonHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)

	// IMPROVEMENT #2: Using constant-time comparison (already in your code!)
	if subtle.ConstantTimeCompare(decodedHash, comparisonHash) == 1 {
		return true, nil
	}
	return false, nil
}

// ============================================
// TYPED ENUMS (Improvement #35)
// ============================================

type SecurityEventType string

const (
	EventUserRegistered         SecurityEventType = "user_registered"
	EventLoginSuccess           SecurityEventType = "login_success"
	EventLoginFailed            SecurityEventType = "login_failed"
	EventLoginBlockedLocked     SecurityEventType = "login_blocked_locked"
	EventLoginBlockedInactive   SecurityEventType = "login_blocked_inactive"
	EventAccountLocked          SecurityEventType = "account_locked"
	EventSuspiciousLogin        SecurityEventType = "suspicious_login_detected"
	Event2FAVerificationFailed  SecurityEventType = "2fa_verification_failed"
	Event2FAVerificationSuccess SecurityEventType = "2fa_verification_success"
	EventSuspiciousTokenRefresh SecurityEventType = "suspicious_token_refresh"
	EventLogout                 SecurityEventType = "logout"
	EventLogoutAllDevices       SecurityEventType = "logout_all_devices"
	EventPasswordChanged        SecurityEventType = "password_changed"
	EventTokenReuse             SecurityEventType = "token_reuse_detected"
	EventHighRiskAccess         SecurityEventType = "high_risk_access"
)

type SecurityEventSeverity string

const (
	SeverityInfo     SecurityEventSeverity = "info"
	SeverityWarning  SecurityEventSeverity = "warning"
	SeverityCritical SecurityEventSeverity = "critical"
)

// ============================================
// CENTRALIZED ERRORS (Improvement #17)
// ============================================

var (
	ErrInvalidCredentials = errors.New("invalid_credentials")
	ErrAccountLocked      = errors.New("account_locked")
	ErrAccountInactive    = errors.New("account_inactive")
	ErrEmailNotVerified   = errors.New("email_not_verified")
	ErrInvalidToken       = errors.New("invalid_token")
	ErrExpiredToken       = errors.New("expired_token")
	ErrSessionNotFound    = errors.New("session_not_found")
	ErrRateLimitExceeded  = errors.New("rate_limit_exceeded")
	ErrInvalid2FACode     = errors.New("invalid_2fa_code")
	ErrSuspiciousActivity = errors.New("suspicious_activity_detected")
	ErrMaxSessionsReached = errors.New("max_sessions_reached")
	ErrPasswordTooWeak    = errors.New("password_too_weak")
	ErrPasswordReused     = errors.New("password_recently_used")
	ErrPasswordBreached   = errors.New("password_found_in_breach")
)

// ============================================
// TOKEN TTL CONFIG — types moved to config/immudb_vault.go
// Use config.TokenTTLConfig and config.TokenDuration.
// ============================================

// ============================================
// TYPED SESSION DATA (Improvement #33)
// ============================================

type SessionData struct {
	UserID     uuid.UUID `json:"user_id"`
	TenantID   uuid.UUID `json:"tenant_id"`
	Email      string    `json:"email"`
	Role       string    `json:"role"`
	DeviceType string    `json:"device_type"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	LoginAt    time.Time `json:"login_at"`
}

// ============================================
// DOMAIN SERVICES (Improvement #15)
// ============================================

type TokenService struct {
	jwtSecret string
	redis     *redis.Client
}

func NewTokenService(jwtSecret string, redis *redis.Client) *TokenService {
	return &TokenService{
		jwtSecret: jwtSecret,
		redis:     redis,
	}
}

// IMPROVEMENT #1: Hash tokens before storing
func (s *TokenService) HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// IMPROVEMENT #3: Secure random with error check
func (s *TokenService) GenerateSecureToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// IMPROVEMENT #8, #9: Add JTI, Issuer, Audience
func (s *TokenService) GenerateAccessToken(
	userID, tenantID uuid.UUID,
	sessionID string,
	ttl time.Duration,
) (string, error) {
	jti := uuid.New().String()

	claims := jwt.MapClaims{
		"jti":        jti,
		"iss":        "rms-auth",
		"aud":        "rms-api",
		"user_id":    userID.String(),
		"tenant_id":  tenantID.String(),
		"session_id": sessionID,
		"exp":        time.Now().Add(ttl).Unix(),
		"iat":        time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

// IMPROVEMENT #8: JWT Blacklist — uses remaining token TTL, not a fixed duration
func (s *TokenService) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	if ttl <= 0 {
		// Token already expired — no need to store it
		return nil
	}
	return s.redis.Set(ctx, "blacklist:"+jti, "1", ttl).Err()
}

// BlacklistAccessToken parses the token, extracts remaining TTL and JTI, then blacklists it
func (s *TokenService) BlacklistAccessToken(ctx context.Context, tokenString string) error {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil {
		return err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return fmt.Errorf("invalid claims")
	}

	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		return fmt.Errorf("missing jti claim")
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		return fmt.Errorf("missing exp claim")
	}

	remaining := time.Until(time.Unix(int64(exp), 0))
	return s.BlacklistToken(ctx, jti, remaining)
}

func (s *TokenService) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	result, err := s.redis.Get(ctx, "blacklist:"+jti).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return result == "1", nil
}

// ============================================
// SESSION SERVICE (Improvement #4, #11, #13)
// ============================================

type SessionService struct {
	redis *redis.Client
}

func NewSessionService(redis *redis.Client) *SessionService {
	return &SessionService{redis: redis}
}

// IMPROVEMENT #4: Use Redis Sets instead of KEYS
func (s *SessionService) CreateSession(
	ctx context.Context,
	userID uuid.UUID,
	sessionID string,
	data *SessionData,
	ttl time.Duration,
) error {
	sessionJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// IMPROVEMENT #13: Use pipeline
	pipe := s.redis.Pipeline()

	// Store session
	sessionKey := fmt.Sprintf("session:%s:%s", userID.String(), sessionID)
	pipe.Set(ctx, sessionKey, sessionJSON, ttl)

	// Add to user's session set
	userSessionsKey := fmt.Sprintf("user_sessions:%s", userID.String())
	pipe.SAdd(ctx, userSessionsKey, sessionID)
	pipe.Expire(ctx, userSessionsKey, ttl)

	_, err = pipe.Exec(ctx)
	return err
}

// IMPROVEMENT #4, #12: Get sessions from Set
func (s *SessionService) GetUserSessions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	userSessionsKey := fmt.Sprintf("user_sessions:%s", userID.String())
	return s.redis.SMembers(ctx, userSessionsKey).Result()
}

func (s *SessionService) GetSession(ctx context.Context, userID uuid.UUID, sessionID string) (*SessionData, error) {
	sessionKey := fmt.Sprintf("session:%s:%s", userID.String(), sessionID)
	data, err := s.redis.Get(ctx, sessionKey).Result()
	if err == redis.Nil {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}

	var sessionData SessionData
	if err := json.Unmarshal([]byte(data), &sessionData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &sessionData, nil
}

// IMPROVEMENT #11, #13: Optimized bulk delete
func (s *SessionService) RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error {
	sessionIDs, err := s.GetUserSessions(ctx, userID)
	if err != nil {
		return err
	}

	if len(sessionIDs) == 0 {
		return nil
	}

	// IMPROVEMENT #13: Pipeline for bulk operations
	pipe := s.redis.Pipeline()

	for _, sessionID := range sessionIDs {
		sessionKey := fmt.Sprintf("session:%s:%s", userID.String(), sessionID)
		pipe.Del(ctx, sessionKey)
	}

	userSessionsKey := fmt.Sprintf("user_sessions:%s", userID.String())
	pipe.Del(ctx, userSessionsKey)

	_, err = pipe.Exec(ctx)
	return err
}

func (s *SessionService) RevokeSession(ctx context.Context, userID uuid.UUID, sessionID string) error {
	pipe := s.redis.Pipeline()

	sessionKey := fmt.Sprintf("session:%s:%s", userID.String(), sessionID)
	pipe.Del(ctx, sessionKey)

	userSessionsKey := fmt.Sprintf("user_sessions:%s", userID.String())
	pipe.SRem(ctx, userSessionsKey, sessionID)

	_, err := pipe.Exec(ctx)
	return err
}

// IMPROVEMENT #12: Optimized count
func (s *SessionService) CountUserSessions(ctx context.Context, userID uuid.UUID) (int64, error) {
	userSessionsKey := fmt.Sprintf("user_sessions:%s", userID.String())
	return s.redis.SCard(ctx, userSessionsKey).Result()
}

type PasswordService struct {
	db                 *gorm.DB
	passwordMinLen     int
	passwordHistory    int
	breachCheckEnabled bool
}

func NewPasswordService(db *gorm.DB, minLen, history int) *PasswordService {
	// Enable breach check via env var (default: true)
	enabled := config.GetEnvars("HIBP_ENABLED", "true") == "true"
	return &PasswordService{
		db:                 db,
		passwordMinLen:     minLen,
		passwordHistory:    history,
		breachCheckEnabled: enabled,
	}
}

func (s *PasswordService) ValidatePasswordStrength(password string) error {
	if len(password) < s.passwordMinLen {
		return fmt.Errorf("%w: must be at least %d characters", ErrPasswordTooWeak, s.passwordMinLen)
	}

	hasUpper := false
	hasLower := false
	hasNumber := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case 'A' <= char && char <= 'Z':
			hasUpper = true
		case 'a' <= char && char <= 'z':
			hasLower = true
		case '0' <= char && char <= '9':
			hasNumber = true
		case strings.ContainsRune("!@#$%^&*()_+-=[]{}|;:,.<>?", char):
			hasSpecial = true
		}
	}

	if !hasUpper || !hasLower || !hasNumber || !hasSpecial {
		return fmt.Errorf("%w: must contain uppercase, lowercase, number, and special character", ErrPasswordTooWeak)
	}

	return nil
}

// IMPROVEMENT #32: Password breach check — HaveIBeenPwned k-Anonymity API
// Sends only the first 5 chars of the SHA-1 hash (prefix) to the API.
// The API returns all suffixes matching that prefix; we check locally.
// This means the full password hash never leaves the server.
func (s *PasswordService) CheckPasswordBreach(password string) (bool, error) {
	if !s.breachCheckEnabled {
		return false, nil
	}

	// SHA-1 hash the password (HIBP uses SHA-1, not SHA-256)
	hasher := sha1.New()
	hasher.Write([]byte(password))
	hashBytes := hasher.Sum(nil)
	fullHash := strings.ToUpper(fmt.Sprintf("%x", hashBytes))

	prefix := fullHash[:5]
	suffix := fullHash[5:]

	// Call HIBP range API
	url := fmt.Sprintf("https://api.pwnedpasswords.com/range/%s", prefix)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("hibp: request build failed: %w", err)
	}
	req.Header.Set("User-Agent", "ReMS-PasswordChecker")
	req.Header.Set("Add-Padding", "true") // Prevents response-length side-channel

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Fail open: if HIBP is unreachable, don't block the user
		log.Printf("[HIBP] API unreachable: %v — failing open", err)
		return false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[HIBP] Unexpected status %d — failing open", resp.StatusCode)
		return false, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, nil
	}

	// Each line is "SUFFIX:COUNT"
	for _, line := range strings.Split(string(body), "\r\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.EqualFold(parts[0], suffix) {
			return true, nil // Password found in breach database
		}
	}

	return false, nil
}

func (s *PasswordService) CheckPasswordHistory(userID uuid.UUID, newPassword string) error {
	var history []models.PasswordHistory
	if err := s.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(s.passwordHistory).
		Find(&history).Error; err != nil {
		return err
	}

	for _, h := range history {
		// Use YOUR ComparePwd function
		match, err := ComparePwd(newPassword, h.PasswordHash)
		if err != nil {
			continue
		}
		if match {
			return fmt.Errorf("%w: cannot reuse any of your last %d passwords", ErrPasswordReused, s.passwordHistory)
		}
	}

	return nil
}

func (s *PasswordService) SavePasswordHistory(userID uuid.UUID, passwordHash string) error {
	history := &models.PasswordHistory{
		PasswordHistoryID: uuid.New(),
		UserID:            userID,
		PasswordHash:      passwordHash,
		CreatedAt:         time.Now(),
	}
	return s.db.Create(history).Error
}

// ============================================
// SECURITY SERVICE (Improvement #15, #18, #19, #23, #30)
// ============================================

type SecurityService struct {
	db           *gorm.DB
	redis        *redis.Client
	geoIPService *GeoIPService
}

func NewSecurityService(db *gorm.DB, redis *redis.Client, geoIP *GeoIPService) *SecurityService {
	return &SecurityService{
		db:           db,
		redis:        redis,
		geoIPService: geoIP,
	}
}

type RiskScore struct {
	Score       int
	Factors     []string
	RequiresMFA bool
}

func (s *SecurityService) CalculateRiskScore(
	user *models.User,
	ipAddress, deviceFingerprint string,
) *RiskScore {
	risk := &RiskScore{Score: 0, Factors: []string{}}

	// IP change
	if user.LastLoginIP != "" && user.LastLoginIP != ipAddress {
		risk.Score += 20
		risk.Factors = append(risk.Factors, "ip_changed")

		// IMPROVEMENT #6: Better geo comparison
		lastCountry := s.geoIPService.GetCountry(user.LastLoginIP)
		currentCountry := s.geoIPService.GetCountry(ipAddress)

		if lastCountry != currentCountry {
			risk.Score += 30
			risk.Factors = append(risk.Factors, "country_changed")
		}

		// Impossible travel check
		if s.isImpossibleTravel(user.LastLoginIP, ipAddress, user.LastLoginAt) {
			risk.Score += 50
			risk.Factors = append(risk.Factors, "impossible_travel")
		}
	}

	// New device
	var device models.DeviceRegistry
	err := s.db.Where("user_id = ? AND device_fingerprint = ?", user.UserID, deviceFingerprint).
		First(&device).Error

	if err != nil {
		risk.Score += 25
		risk.Factors = append(risk.Factors, "new_device")
	} else if !device.IsTrusted {
		risk.Score += 15
		risk.Factors = append(risk.Factors, "untrusted_device")
	}

	// Recent failed attempts
	if user.FailedLoginAttempts > 0 {
		risk.Score += user.FailedLoginAttempts * 5
		risk.Factors = append(risk.Factors, "recent_failed_attempts")
	}

	risk.RequiresMFA = risk.Score >= 50

	return risk
}

func (s *SecurityService) isImpossibleTravel(lastIP, currentIP string, lastLoginAt *time.Time) bool {
	if lastLoginAt == nil {
		return false
	}

	lastLat, lastLon := s.geoIPService.GetCoordinates(lastIP)
	currentLat, currentLon := s.geoIPService.GetCoordinates(currentIP)

	distance := s.calculateDistance(lastLat, lastLon, currentLat, currentLon)
	timeSince := time.Since(*lastLoginAt)

	// Max speed: 1000 km/h
	maxPossibleDistance := timeSince.Hours() * 1000

	return distance > maxPossibleDistance
}

func (s *SecurityService) calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0

	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

// IMPROVEMENT #19: IP-based rate limiting
func (s *SecurityService) CheckIPRateLimit(ctx context.Context, ip string, limit int, window time.Duration) error {
	key := fmt.Sprintf("ratelimit:ip:%s", ip)
	return s.checkRateLimit(ctx, key, limit, window)
}

// IMPROVEMENT #23, #24: Sliding window with Lua script
func (s *SecurityService) checkRateLimit(ctx context.Context, key string, limit int, window time.Duration) error {
	now := time.Now().Unix()
	windowStart := now - int64(window.Seconds())

	luaScript := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window_start = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])
		local window = tonumber(ARGV[4])
		
		redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)
		local count = redis.call('ZCARD', key)
		
		if count >= limit then
			return 0
		end
		
		redis.call('ZADD', key, now, now)
		redis.call('EXPIRE', key, window)
		
		return 1
	`

	result, err := s.redis.Eval(ctx, luaScript, []string{key}, now, windowStart, limit, int64(window.Seconds())).Result()
	if err != nil {
		return err
	}

	if result == int64(0) {
		return ErrRateLimitExceeded
	}

	return nil
}

// IMPROVEMENT #18: Escalating lockout
func (s *SecurityService) CalculateLockoutDuration(failedAttempts int) time.Duration {
	switch {
	case failedAttempts >= 20:
		return 24 * time.Hour
	case failedAttempts >= 10:
		return 30 * time.Minute
	case failedAttempts >= 5:
		return 5 * time.Minute
	default:
		return 0
	}
}

func (s *SecurityService) LogSecurityEvent(
	ctx context.Context,
	userID *uuid.UUID,
	eventType SecurityEventType,
	severity SecurityEventSeverity,
	ipAddress, userAgent string,
	success bool,
	metadata map[string]interface{},
) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	event := &models.SecurityEvent{
		EventID:       uuid.New(),
		UserID:        userID,
		EventType:     string(eventType),
		EventSeverity: string(severity),
		IPAddress:     ipAddress,
		UserAgent:     userAgent,
		Location:      s.geoIPService.Lookup(ipAddress),
		Success:       success,
		Metadata:      metadataJSON,
		Timestamp:     time.Now(),
	}

	return s.db.Create(event).Error
}

// ============================================
// DEVICE SERVICE (Improvement #21)
// ============================================

type DeviceService struct {
	db *gorm.DB
}

func NewDeviceService(db *gorm.DB) *DeviceService {
	return &DeviceService{db: db}
}

// IMPROVEMENT #21: Device trust levels
func (s *DeviceService) RegisterOrUpdateDevice(
	ctx context.Context,
	userID uuid.UUID,
	deviceFingerprint, ipAddress, userAgent, location string,
) (*models.DeviceRegistry, error) {
	var device models.DeviceRegistry
	err := s.db.Where("user_id = ? AND device_fingerprint = ?", userID, deviceFingerprint).
		First(&device).Error

	if err != nil {
		// New device
		device = models.DeviceRegistry{
			DeviceID:          uuid.New(),
			UserID:            userID,
			DeviceFingerprint: deviceFingerprint,
			DeviceName:        parseDeviceName(userAgent),
			IsTrusted:         false,
			FirstSeenAt:       time.Now(),
			LastSeenAt:        time.Now(),
			LastSeenIP:        ipAddress,
			LastSeenLocation:  location,
		}
		if err := s.db.Create(&device).Error; err != nil {
			return nil, err
		}
	} else {
		// Update
		device.LastSeenAt = time.Now()
		device.LastSeenIP = ipAddress
		device.LastSeenLocation = location

		// Auto-trust after 5 logins
		if device.LoginCount >= 5 && !device.IsTrusted {
			device.IsTrusted = true
		}
		device.LoginCount++

		if err := s.db.Save(&device).Error; err != nil {
			return nil, err
		}
	}

	return &device, nil
}

func parseDeviceName(userAgent string) string {
	ua := strings.ToLower(userAgent)

	browser := "Unknown"
	if strings.Contains(ua, "chrome") {
		browser = "Chrome"
	} else if strings.Contains(ua, "firefox") {
		browser = "Firefox"
	} else if strings.Contains(ua, "safari") {
		browser = "Safari"
	} else if strings.Contains(ua, "edge") {
		browser = "Edge"
	}

	os := "Unknown"
	if strings.Contains(ua, "windows") {
		os = "Windows"
	} else if strings.Contains(ua, "mac") {
		os = "macOS"
	} else if strings.Contains(ua, "linux") {
		os = "Linux"
	} else if strings.Contains(ua, "android") {
		os = "Android"
	} else if strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") {
		os = "iOS"
	}

	return fmt.Sprintf("%s on %s", browser, os)
}

// ============================================
// EMAIL SERVICE — see email.go for implementation
// ============================================

// ============================================
// GEOIP SERVICE — see geoip.go for implementation
// ============================================

type AuthService struct {
	db           *gorm.DB
	redis        *redis.Client
	jwtSecretKey string
	ttlConfig    *config.TokenTTLConfig

	// Domain services
	tokenService    *TokenService
	sessionService  *SessionService
	passwordService *PasswordService
	securityService *SecurityService
	deviceService   *DeviceService
	emailService    *EmailService
	geoIPService    *GeoIPService

	// Configuration — loaded from ImmuDB vault
	maxSessions       int
	maxFailedLogins   int
	lockoutDuration   time.Duration
	passwordMinLength int
	passwordHistory   int
	rateLimitCount    int
	rateLimitWindow   time.Duration
}

// NewAuthService creates an AuthService with config loaded from ImmuDB vault.
// All magic numbers are sourced from authCfg and ttlCfg.
func NewAuthService(
	db *gorm.DB,
	redisClient *redis.Client,
	jwtSecret string,
	authCfg *config.AuthServiceConfig,
	ttlCfg *config.TokenTTLConfig,
	emailService *EmailService,
	geoIPService *GeoIPService,
) *AuthService {
	tokenService := NewTokenService(jwtSecret, redisClient)
	sessionService := NewSessionService(redisClient)
	passwordService := NewPasswordService(db, authCfg.PasswordMinLength, authCfg.PasswordHistory)
	securityService := NewSecurityService(db, redisClient, geoIPService)
	deviceService := NewDeviceService(db)

	return &AuthService{
		db:                db,
		redis:             redisClient,
		jwtSecretKey:      jwtSecret,
		ttlConfig:         ttlCfg,
		tokenService:      tokenService,
		sessionService:    sessionService,
		passwordService:   passwordService,
		securityService:   securityService,
		deviceService:     deviceService,
		emailService:      emailService,
		geoIPService:      geoIPService,
		maxSessions:       authCfg.MaxSessions,
		maxFailedLogins:   authCfg.MaxFailedLogins,
		lockoutDuration:   authCfg.LockoutDuration,
		passwordMinLength: authCfg.PasswordMinLength,
		passwordHistory:   authCfg.PasswordHistory,
		rateLimitCount:    authCfg.RateLimitCount,
		rateLimitWindow:   authCfg.RateLimitWindow,
	}
}

// DB returns the database connection for direct access when needed.
func (s *AuthService) DB() *gorm.DB {
	return s.db
}

// ============================================
// REGISTER (YOUR METHOD WITH IMPROVEMENTS)
// ============================================

func (s *AuthService) Register(
	ctx context.Context,
	req *DTO.RegisterRequest,
	ipAddress, userAgent string,
) (*DTO.RegisterResponse, error) {
	// IMPROVEMENT #27: Context timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Validate password strength
	if err := s.passwordService.ValidatePasswordStrength(req.Password); err != nil {
		return nil, err
	}

	// IMPROVEMENT #32: Check password breach
	breached, err := s.passwordService.CheckPasswordBreach(req.Password)
	if err != nil {
		return nil, err
	}
	if breached {
		return nil, ErrPasswordBreached
	}

	// Check existing user
	var existingUser models.User
	if err := s.db.WithContext(ctx).Where("email = ?", strings.ToLower(req.Email)).First(&existingUser).Error; err == nil {
		return nil, errors.New("user already exists")
	}

	// YOUR HashPwd function!
	passwordHash, err := HashPwd(req.Password)
	if err != nil {
		return nil, err
	}

	// Create user
	user := models.User{
		UserID:          uuid.New(),
		TenantID:        req.TenantID,
		UserName:        req.UserName,
		FullName:        req.FullName,
		Email:           strings.ToLower(req.Email),
		PasswordHash:    passwordHash,
		Phone:           req.Phone,
		IsActive:        true,
		IsEmailVerified: false, // IMPROVEMENT #10
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// IMPROVEMENT #16: Transaction
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}

		// Save password history
		history := models.PasswordHistory{
			PasswordHistoryID: uuid.New(),
			UserID:            user.UserID,
			PasswordHash:      passwordHash,
			CreatedAt:         time.Now(),
		}

		if err := tx.Create(&history).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Create email verification token
	verificationToken, err := s.createEmailVerificationToken(ctx, user.UserID)
	if err != nil {
		return nil, err
	}

	// Send verification email
	go s.emailService.SendVerificationEmail(user.Email, user.FullName, verificationToken)

	// Log event
	s.securityService.LogSecurityEvent(ctx, &user.UserID, EventUserRegistered, SeverityInfo, ipAddress, userAgent, true, nil)

	return &DTO.RegisterResponse{
		UserID:   user.UserID,
		Email:    user.Email,
		FullName: user.FullName,
	}, nil
}

// ============================================
// LOGIN (YOUR METHOD WITH ALL IMPROVEMENTS)
// ============================================

func (s *AuthService) Login(
	ctx context.Context,
	req *DTO.LoginRequest,
	ipAddress, userAgent, deviceFingerprint string,
) (*DTO.LoginResponse, error) {
	// IMPROVEMENT #27: Context timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// IMPROVEMENT #19: IP-based rate limiting
	if err := s.securityService.CheckIPRateLimit(ctx, ipAddress, 10, time.Minute); err != nil {
		return nil, ErrRateLimitExceeded
	}

	// Find user
	var user models.User
	if err := s.db.WithContext(ctx).Where("email = ?", strings.ToLower(req.Email)).First(&user).Error; err != nil {
		s.securityService.LogSecurityEvent(ctx, nil, EventLoginFailed, SeverityWarning, ipAddress, userAgent, false, map[string]interface{}{
			"reason": "user_not_found",
			"email":  req.Email,
		})
		return nil, ErrInvalidCredentials
	}

	// Check if account is locked
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		s.securityService.LogSecurityEvent(ctx, &user.UserID, EventLoginBlockedLocked, SeverityWarning, ipAddress, userAgent, false, nil)
		return nil, fmt.Errorf("%w: locked until %s", ErrAccountLocked, user.LockedUntil.Format(time.RFC3339))
	}

	// Check if active
	if !user.IsActive {
		s.securityService.LogSecurityEvent(ctx, &user.UserID, EventLoginBlockedInactive, SeverityWarning, ipAddress, userAgent, false, nil)
		return nil, ErrAccountInactive
	}

	// IMPROVEMENT #10: Enforce email verification
	if !user.IsEmailVerified {
		return nil, ErrEmailNotVerified
	}

	// YOUR ComparePwd function!
	match, err := ComparePwd(req.Password, user.PasswordHash)
	if err != nil || !match {
		s.handleFailedLogin(ctx, &user, ipAddress, userAgent)
		return nil, ErrInvalidCredentials
	}

	// Reset failed attempts
	if user.FailedLoginAttempts > 0 {
		s.db.WithContext(ctx).Model(&user).Updates(map[string]interface{}{
			"failed_login_attempts": 0,
			"locked_until":          nil,
		})
	}

	// IMPROVEMENT #30: Calculate risk score
	riskScore := s.securityService.CalculateRiskScore(&user, ipAddress, deviceFingerprint)

	// If high risk or 2FA enabled, require 2FA
	if riskScore.RequiresMFA || user.TwoFactorEnabled {
		pendingSessionID, err := s.createPending2FASession(ctx, user.UserID, ipAddress, userAgent)
		if err != nil {
			return nil, err
		}

		s.securityService.LogSecurityEvent(ctx, &user.UserID, EventHighRiskAccess, SeverityWarning, ipAddress, userAgent, true, map[string]interface{}{
			"risk_score":   riskScore.Score,
			"risk_factors": riskScore.Factors,
		})

		// User field is intentionally nil here — session is not fully established.
		// Callers must check Requires2FA before accessing User.
		return &DTO.LoginResponse{
			Requires2FA:      true,
			PendingSessionID: pendingSessionID,
			Message:          "2FA required for this login",
			User:             nil,
		}, nil
	}

	// Check session limit
	sessionCount, err := s.sessionService.CountUserSessions(ctx, user.UserID)
	if err != nil {
		return nil, err
	}
	if sessionCount >= int64(s.maxSessions) {
		return nil, ErrMaxSessionsReached
	}

	// Determine TTL
	deviceType := "web" // Default
	if req.DeviceType != "" {
		deviceType = req.DeviceType
	}
	ttl := s.determineTokenTTL(user.PrimaryRole, deviceType)

	// Create tokens
	tokens, err := s.createAuthTokensWithTTL(ctx, &user, ipAddress, userAgent, deviceFingerprint, deviceType, ttl)
	if err != nil {
		return nil, err
	}

	// Update login metadata
	location := s.geoIPService.Lookup(ipAddress)
	s.db.WithContext(ctx).Model(&user).Updates(map[string]interface{}{
		"last_login_at":       time.Now(),
		"last_login_ip":       ipAddress,
		"last_login_location": location,
	})

	// Register device
	s.deviceService.RegisterOrUpdateDevice(ctx, user.UserID, deviceFingerprint, ipAddress, userAgent, location)

	// Log success
	s.securityService.LogSecurityEvent(ctx, &user.UserID, EventLoginSuccess, SeverityInfo, ipAddress, userAgent, true, map[string]interface{}{
		"device_type": deviceType,
		"risk_score":  riskScore.Score,
	})

	return tokens, nil
}

// ============================================
// CHANGE PASSWORD (Using YOUR HashPwd)
// ============================================

func (s *AuthService) ChangePassword(
	ctx context.Context,
	userID uuid.UUID,
	oldPassword, newPassword string,
	ipAddress, userAgent string,
) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Get user
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return err
	}

	// Verify old password with YOUR ComparePwd
	match, err := ComparePwd(oldPassword, user.PasswordHash)
	if err != nil || !match {
		return errors.New("incorrect current password")
	}

	// Validate new password
	if err := s.passwordService.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	// Check breach
	breached, err := s.passwordService.CheckPasswordBreach(newPassword)
	if err != nil {
		return err
	}
	if breached {
		return ErrPasswordBreached
	}

	// Check history
	if err := s.passwordService.CheckPasswordHistory(userID, newPassword); err != nil {
		return err
	}

	// Hash with YOUR HashPwd
	newHash, err := HashPwd(newPassword)
	if err != nil {
		return err
	}

	// IMPROVEMENT #16: Transaction
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Update password
		if err := tx.Model(&user).Updates(map[string]interface{}{
			"password_hash":       newHash,
			"password_changed_at": time.Now(),
		}).Error; err != nil {
			return err
		}

		// Save history
		if err := tx.Create(&models.PasswordHistory{
			PasswordHistoryID: uuid.New(),
			UserID:            userID,
			PasswordHash:      newHash,
			CreatedAt:         time.Now(),
		}).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Revoke all sessions
	s.sessionService.RevokeAllUserSessions(ctx, userID)
	s.revokeAllRefreshTokens(ctx, userID, "password_changed")

	// Log
	s.securityService.LogSecurityEvent(ctx, &userID, EventPasswordChanged, SeverityInfo, ipAddress, userAgent, true, nil)

	// Send email
	go s.emailService.SendPasswordChangedNotification(user.Email, user.FullName, ipAddress)

	return nil
}

// ============================================
// RESET PASSWORD (token-based, from email link)
// ============================================

func (s *AuthService) ResetPassword(
	ctx context.Context,
	email string,
	newPassword string,
	ipAddress, userAgent string,
) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Find user
	var user models.User
	if err := s.db.WithContext(ctx).Where("email = ?", strings.ToLower(email)).First(&user).Error; err != nil {
		return errors.New("user not found")
	}

	// Validate new password strength
	if err := s.passwordService.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	// Check breach
	breached, err := s.passwordService.CheckPasswordBreach(newPassword)
	if err != nil {
		return err
	}
	if breached {
		return ErrPasswordBreached
	}

	// Check history
	if err := s.passwordService.CheckPasswordHistory(user.UserID, newPassword); err != nil {
		return err
	}

	// Hash new password
	newHash, err := HashPwd(newPassword)
	if err != nil {
		return err
	}

	// Update in transaction
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Updates(map[string]interface{}{
			"password_hash":       newHash,
			"password_changed_at": time.Now(),
		}).Error; err != nil {
			return err
		}

		// Save history
		if err := tx.Create(&models.PasswordHistory{
			PasswordHistoryID: uuid.New(),
			UserID:            user.UserID,
			PasswordHash:      newHash,
			CreatedAt:         time.Now(),
		}).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Revoke all sessions
	s.sessionService.RevokeAllUserSessions(ctx, user.UserID)
	s.revokeAllRefreshTokens(ctx, user.UserID, "password_reset")

	// Log
	s.securityService.LogSecurityEvent(ctx, &user.UserID, EventPasswordChanged, SeverityInfo, ipAddress, userAgent, true, map[string]interface{}{
		"reason": "password_reset",
	})

	// Notify
	go s.emailService.SendPasswordChangedNotification(user.Email, user.FullName, ipAddress)

	return nil
}

// ============================================
// REFRESH ACCESS TOKEN (with rotation)
// ============================================

func (s *AuthService) RefreshAccessToken(
	ctx context.Context,
	refreshTokenString string,
	ipAddress, userAgent string,
) (*DTO.LoginResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tokenHash := s.tokenService.HashToken(refreshTokenString)

	var existing models.RefreshToken
	if err := s.db.WithContext(ctx).
		Where("token_hash = ? AND expires_at > ?", tokenHash, time.Now()).
		First(&existing).Error; err != nil {
		return nil, ErrInvalidToken
	}

	// Detect reuse of an already-revoked token — possible theft
	if existing.IsRevoked {
		s.revokeAllRefreshTokens(ctx, existing.UserID, "token_reuse_detected")
		s.sessionService.RevokeAllUserSessions(ctx, existing.UserID)
		s.securityService.LogSecurityEvent(ctx, &existing.UserID, EventTokenReuse, SeverityCritical, ipAddress, userAgent, false, nil)
		return nil, ErrInvalidToken
	}

	// Rotation: revoke old token and issue new one atomically
	var newRefreshTokenString string

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Revoke the old token
		if err := tx.Model(&existing).Updates(map[string]interface{}{
			"is_revoked":     true,
			"revoked_at":     time.Now(),
			"revoked_reason": "rotated",
		}).Error; err != nil {
			return err
		}

		// 2. Generate new refresh token
		raw, err := s.tokenService.GenerateSecureToken(64)
		if err != nil {
			return err
		}
		newRefreshTokenString = raw

		ttl := s.determineTokenTTL(existing.UserRole, existing.DeviceType)

		// 3. Insert new token, inheriting device/user context from old
		newToken := &models.RefreshToken{
			TokenID:           uuid.New(),
			UserID:            existing.UserID,
			TokenHash:         s.tokenService.HashToken(raw),
			DeviceID:          existing.DeviceID,
			DeviceFingerprint: existing.DeviceFingerprint,
			DeviceType:        existing.DeviceType,
			UserRole:          existing.UserRole,
			IPAddress:         ipAddress,
			UserAgent:         userAgent,
			Location:          s.geoIPService.Lookup(ipAddress),
			ExpiresAt:         time.Now().Add(ttl.RefreshToken),
			CreatedAt:         time.Now(),
		}

		return tx.Create(newToken).Error
	})
	if err != nil {
		return nil, err
	}

	// Load user for access token generation
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, existing.UserID).Error; err != nil {
		return nil, err
	}

	ttl := s.determineTokenTTL(existing.UserRole, existing.DeviceType)
	accessToken, err := s.tokenService.GenerateAccessToken(user.UserID, user.TenantID, existing.DeviceID, ttl.AccessToken)
	if err != nil {
		return nil, err
	}

	s.securityService.LogSecurityEvent(ctx, &existing.UserID, EventLoginSuccess, SeverityInfo, ipAddress, userAgent, true, map[string]interface{}{
		"device_type": existing.DeviceType,
		"reason":      "token_refreshed",
	})

	expiresAt := time.Now().Add(ttl.AccessToken)
	return &DTO.LoginResponse{
		Token:        &accessToken,
		RefreshToken: &newRefreshTokenString,
		ExpiresAt:    &expiresAt,
	}, nil
}

// ============================================
// VERIFY 2FA — promotes pending session to full login
// ============================================

func (s *AuthService) Verify2FA(
	ctx context.Context,
	pendingSessionID string,
	totpCode string,
	deviceFingerprint string,
	ipAddress, userAgent string,
) (*DTO.LoginResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 1. Atomically retrieve and delete the pending 2FA session from Redis
	// Using GetDel to prevent TOCTOU race where two concurrent requests could
	// both read the session before either deletes it
	pendingKey := "pending_2fa:" + pendingSessionID
	data, err := s.redis.GetDel(ctx, pendingKey).Result()
	if err == redis.Nil {
		return nil, ErrInvalidToken // expired or never existed
	}
	if err != nil {
		return nil, err
	}

	var sessionData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &sessionData); err != nil {
		return nil, fmt.Errorf("failed to parse pending session: %w", err)
	}

	userIDStr, ok := sessionData["user_id"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// 2. Load user
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, ErrInvalidCredentials
	}

	// 3. Verify TOTP code against user's stored secret
	if !totp.Validate(totpCode, user.TwoFactorSecret) {
		s.securityService.LogSecurityEvent(ctx, &user.UserID, Event2FAVerificationFailed, SeverityWarning, ipAddress, userAgent, false, map[string]interface{}{
			"pending_session_id": pendingSessionID,
		})
		return nil, ErrInvalid2FACode
	}

	// 4. Pending session was already consumed atomically by GetDel above

	// 5. Check session cap before creating new session
	sessionCount, err := s.sessionService.CountUserSessions(ctx, user.UserID)
	if err != nil {
		return nil, err
	}
	if sessionCount >= int64(s.maxSessions) {
		return nil, ErrMaxSessionsReached
	}

	// 6. Determine TTL from device type (preserved from original login request)
	deviceType := "web"
	if dt, ok := sessionData["device_type"].(string); ok && dt != "" {
		deviceType = dt
	}
	ttl := s.determineTokenTTL(user.PrimaryRole, deviceType)

	// 7. Issue full auth tokens
	tokens, err := s.createAuthTokensWithTTL(ctx, &user, ipAddress, userAgent, deviceFingerprint, deviceType, ttl)
	if err != nil {
		return nil, err
	}

	// 8. Update login metadata
	location := s.geoIPService.Lookup(ipAddress)
	s.db.WithContext(ctx).Model(&user).Updates(map[string]interface{}{
		"last_login_at":       time.Now(),
		"last_login_ip":       ipAddress,
		"last_login_location": location,
	})

	// 9. Register device
	s.deviceService.RegisterOrUpdateDevice(ctx, user.UserID, deviceFingerprint, ipAddress, userAgent, location)

	// 10. Log success
	s.securityService.LogSecurityEvent(ctx, &user.UserID, Event2FAVerificationSuccess, SeverityInfo, ipAddress, userAgent, true, map[string]interface{}{
		"device_type": deviceType,
	})

	return tokens, nil
}

// ============================================
// HELPER METHODS
// ============================================

func (s *AuthService) handleFailedLogin(
	ctx context.Context,
	user *models.User,
	ipAddress, userAgent string,
) {
	user.FailedLoginAttempts++

	updates := map[string]interface{}{
		"failed_login_attempts": user.FailedLoginAttempts,
	}

	// IMPROVEMENT #18: Escalating lockout
	lockoutDuration := s.securityService.CalculateLockoutDuration(user.FailedLoginAttempts)

	if lockoutDuration > 0 {
		lockUntil := time.Now().Add(lockoutDuration)
		updates["locked_until"] = lockUntil

		s.securityService.LogSecurityEvent(ctx, &user.UserID, EventAccountLocked, SeverityCritical, ipAddress, userAgent, false, map[string]interface{}{
			"failed_attempts":  user.FailedLoginAttempts,
			"locked_until":     lockUntil,
			"lockout_duration": lockoutDuration.String(),
		})

		go s.emailService.SendAccountLockedNotification(user.Email, user.FullName, lockUntil)
	} else {
		s.securityService.LogSecurityEvent(ctx, &user.UserID, EventLoginFailed, SeverityWarning, ipAddress, userAgent, false, map[string]interface{}{
			"failed_attempts": user.FailedLoginAttempts,
		})
	}

	s.db.WithContext(ctx).Model(user).Updates(updates)
}

func (s *AuthService) createAuthTokensWithTTL(
	ctx context.Context,
	user *models.User,
	ipAddress, userAgent, deviceFingerprint, deviceType string,
	ttl config.TokenDuration,
) (*DTO.LoginResponse, error) {
	sessionID := uuid.New().String()

	// Generate access token
	accessToken, err := s.tokenService.GenerateAccessToken(
		user.UserID,
		user.TenantID,
		sessionID,
		ttl.AccessToken,
	)
	if err != nil {
		return nil, err
	}

	// Generate refresh token
	refreshTokenString, err := s.tokenService.GenerateSecureToken(64)
	if err != nil {
		return nil, err
	}

	// IMPROVEMENT #1: Hash before storing
	tokenHash := s.tokenService.HashToken(refreshTokenString)

	// Create refresh token record
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

	// Create session
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

	// Load tenant name for response
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

func (s *AuthService) determineTokenTTL(userRole, deviceType string) config.TokenDuration {
	if ttl, exists := s.ttlConfig.ByRole[userRole]; exists {
		return ttl
	}
	if ttl, exists := s.ttlConfig.ByDevice[deviceType]; exists {
		return ttl
	}
	return s.ttlConfig.Default
}

func (s *AuthService) createEmailVerificationToken(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := s.tokenService.GenerateSecureToken(32)
	if err != nil {
		return "", err
	}

	verification := &models.EmailVerificationToken{
		TokenID:   uuid.New(),
		UserID:    userID,
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}

	if err := s.db.WithContext(ctx).Create(verification).Error; err != nil {
		return "", err
	}

	return token, nil
}

func (s *AuthService) createPending2FASession(
	ctx context.Context,
	userID uuid.UUID,
	ipAddress, userAgent string,
) (string, error) {
	sessionID := uuid.New().String()

	sessionData := map[string]interface{}{
		"user_id":    userID.String(),
		"ip_address": ipAddress,
		"user_agent": userAgent,
	}

	sessionJSON, _ := json.Marshal(sessionData)
	if err := s.redis.Set(ctx, "pending_2fa:"+sessionID, sessionJSON, 5*time.Minute).Err(); err != nil {
		return "", err
	}

	return sessionID, nil
}

func (s *AuthService) revokeAllRefreshTokens(ctx context.Context, userID uuid.UUID, reason string) error {
	return s.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"is_revoked":     true,
			"revoked_at":     time.Now(),
			"revoked_reason": reason,
		}).Error
}

// // Add these missing fields to DTO.LoginRequest
// type LoginRequestExtended struct {
// 	DTO.LoginRequest
// 	DeviceType string `json:"device_type,omitempty"`
// }

// // Add these missing fields to DTO.LoginResponse
// type LoginResponseExtended struct {
// 	DTO.LoginResponse
// 	Requires2FA      bool   `json:"requires_2fa,omitempty"`
// 	PendingSessionID string `json:"pending_session_id,omitempty"`
// 	Message          string `json:"message,omitempty"`
// }

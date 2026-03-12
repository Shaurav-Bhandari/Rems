package config

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	immudb "github.com/codenotary/immudb/pkg/client"
)

// ============================================================================
// IMMUDB CONFIG VAULT
// Provides an immutable, tamper-proof configuration store. All PII, secrets,
// TTLs, and operational config are stored here instead of env vars or code.
// ============================================================================

// ImmuVault wraps the ImmuDB client to provide typed config access.
type ImmuVault struct {
	client immudb.ImmuClient
	ctx    context.Context
}

// NewImmuVault connects to ImmuDB and logs in.
// Returns a connected vault or an error. Caller must call Close().
func NewImmuVault(host string, port int, username, password, database string) (*ImmuVault, error) {
	ctx := context.Background()

	opts := immudb.DefaultOptions().
		WithAddress(host).
		WithPort(port)

	client := immudb.NewClient().WithOptions(opts)

	err := client.OpenSession(ctx, []byte(username), []byte(password), database)
	if err != nil {
		return nil, fmt.Errorf("immudb: failed to open session: %w", err)
	}

	log.Println("✓ ImmuDB vault connected")

	return &ImmuVault{
		client: client,
		ctx:    ctx,
	}, nil
}

// Close gracefully shuts down the ImmuDB session.
func (v *ImmuVault) Close() {
	if v.client != nil {
		v.client.CloseSession(v.ctx)
		log.Println("✓ ImmuDB vault closed")
	}
}

// ============================================================================
// TYPED READERS
// ============================================================================

// GetString reads a string config value. Returns fallback if key not found.
func (v *ImmuVault) GetString(ctx context.Context, key, fallback string) string {
	entry, err := v.client.Get(ctx, []byte(key))
	if err != nil {
		return fallback
	}
	return string(entry.Value)
}

// GetInt reads an integer config value. Returns fallback if key not found or unparseable.
func (v *ImmuVault) GetInt(ctx context.Context, key string, fallback int) int {
	raw := v.GetString(ctx, key, "")
	if raw == "" {
		return fallback
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return val
}

// GetDuration reads a duration config value (stored as seconds).
// Returns fallback if key not found or unparseable.
func (v *ImmuVault) GetDuration(ctx context.Context, key string, fallback time.Duration) time.Duration {
	raw := v.GetString(ctx, key, "")
	if raw == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

// SetConfig writes a config value to ImmuDB (immutable append).
func (v *ImmuVault) SetConfig(ctx context.Context, key, value string) error {
	_, err := v.client.Set(ctx, []byte(key), []byte(value))
	if err != nil {
		return fmt.Errorf("immudb: failed to set key %q: %w", key, err)
	}
	return nil
}

// ============================================================================
// DEFAULT SEEDER
// ============================================================================

// configDefault represents a single default config entry.
type configDefault struct {
	Key      string
	Value    string
	Category string
}

// SeedDefaults idempotently writes default config values.
// Values are only written if the key does NOT already exist.
// JWT secret is generated using crypto/rand (256-bit, base64-encoded).
func (v *ImmuVault) SeedDefaults(ctx context.Context) error {
	log.Println("🌱 Seeding ImmuDB config defaults...")

	// Generate a cryptographically secure JWT secret (256-bit)
	jwtSecret, err := generateSecureKey(32) // 256 bits
	if err != nil {
		return fmt.Errorf("immudb: failed to generate JWT secret: %w", err)
	}

	// Determine CORS origins based on environment
	appEnv := os.Getenv("APP_ENV")
	corsOrigins := "http://localhost:5173"
	if appEnv == "production" {
		corsOrigins = "" // Must be explicitly configured in production
		log.Println("[WARN] APP_ENV=production — cors.allowed_origins must be configured manually")
	}

	defaults := []configDefault{
		// Auth
		{"auth.max_sessions", "5", "auth"},
		{"auth.max_failed_logins", "5", "auth"},
		{"auth.lockout_duration_minutes", "30", "auth"},
		{"auth.password_min_length", "12", "auth"},
		{"auth.password_history_count", "5", "auth"},
		{"auth.rate_limit_count", "10", "auth"},
		{"auth.rate_limit_window_seconds", "60", "auth"},

		// JWT
		{"jwt.secret", jwtSecret, "jwt"},
		{"jwt.issuer", "rms-auth", "jwt"},
		{"jwt.audience", "rms-api", "jwt"},

		// Token TTLs (stored as seconds)
		{"ttl.access.admin", "900", "ttl"},         // 15 minutes
		{"ttl.refresh.admin", "7200", "ttl"},       // 2 hours
		{"ttl.access.manager", "1200", "ttl"},      // 20 minutes
		{"ttl.refresh.manager", "14400", "ttl"},    // 4 hours
		{"ttl.access.cashier", "7200", "ttl"},      // 2 hours
		{"ttl.refresh.cashier", "28800", "ttl"},    // 8 hours
		{"ttl.access.chef", "28800", "ttl"},        // 8 hours
		{"ttl.refresh.chef", "86400", "ttl"},       // 24 hours
		{"ttl.access.waiter", "14400", "ttl"},      // 4 hours
		{"ttl.refresh.waiter", "43200", "ttl"},     // 12 hours
		{"ttl.access.customer", "604800", "ttl"},   // 7 days
		{"ttl.refresh.customer", "7776000", "ttl"}, // 90 days
		{"ttl.access.default", "3600", "ttl"},      // 1 hour
		{"ttl.refresh.default", "86400", "ttl"},    // 24 hours

		// Device TTLs
		{"ttl.access.device.kds", "86400", "ttl"},           // 24 hours
		{"ttl.refresh.device.kds", "604800", "ttl"},         // 7 days
		{"ttl.access.device.pos", "28800", "ttl"},           // 8 hours
		{"ttl.refresh.device.pos", "86400", "ttl"},          // 24 hours
		{"ttl.access.device.mobile_app", "604800", "ttl"},   // 7 days
		{"ttl.refresh.device.mobile_app", "7776000", "ttl"}, // 90 days
		{"ttl.access.device.web", "3600", "ttl"},            // 1 hour
		{"ttl.refresh.device.web", "604800", "ttl"},         // 7 days

		// Redis
		{"redis.session_ttl_seconds", "3600", "redis"},

		// CORS
		{"cors.allowed_origins", corsOrigins, "cors"},

		// App
		{"app.port", "3000", "app"},
	}

	seeded := 0
	for _, d := range defaults {
		// Check if key already exists (idempotent)
		_, err := v.client.Get(ctx, []byte(d.Key))
		if err == nil {
			continue // Key exists, skip
		}

		if err := v.SetConfig(ctx, d.Key, d.Value); err != nil {
			log.Printf("[WARN] Failed to seed %s: %v", d.Key, err)
			continue
		}
		seeded++
	}

	log.Printf("✅ ImmuDB seeding complete — %d new keys written", seeded)
	return nil
}

// ============================================================================
// STRUCTURED CONFIG LOADERS
// ============================================================================

// AuthServiceConfig holds all auth-related configuration values.
type AuthServiceConfig struct {
	MaxSessions       int
	MaxFailedLogins   int
	LockoutDuration   time.Duration
	PasswordMinLength int
	PasswordHistory   int
	RateLimitCount    int
	RateLimitWindow   time.Duration
}

// GetAuthServiceConfig loads auth config from ImmuDB with safe defaults.
func (v *ImmuVault) GetAuthServiceConfig(ctx context.Context) *AuthServiceConfig {
	return &AuthServiceConfig{
		MaxSessions:       v.GetInt(ctx, "auth.max_sessions", 5),
		MaxFailedLogins:   v.GetInt(ctx, "auth.max_failed_logins", 5),
		LockoutDuration:   time.Duration(v.GetInt(ctx, "auth.lockout_duration_minutes", 30)) * time.Minute,
		PasswordMinLength: v.GetInt(ctx, "auth.password_min_length", 12),
		PasswordHistory:   v.GetInt(ctx, "auth.password_history_count", 5),
		RateLimitCount:    v.GetInt(ctx, "auth.rate_limit_count", 10),
		RateLimitWindow:   v.GetDuration(ctx, "auth.rate_limit_window_seconds", time.Minute),
	}
}

// TokenTTLConfigFromVault loads role-based and device-based TTL config from ImmuDB.
func (v *ImmuVault) GetTokenTTLConfig(ctx context.Context) *TokenTTLConfig {
	roles := []string{"admin", "manager", "cashier", "chef", "waiter", "customer"}
	devices := []string{"kds", "pos", "mobile_app", "web"}

	byRole := make(map[string]TokenDuration)
	for _, role := range roles {
		byRole[role] = TokenDuration{
			AccessToken:  v.GetDuration(ctx, fmt.Sprintf("ttl.access.%s", role), time.Hour),
			RefreshToken: v.GetDuration(ctx, fmt.Sprintf("ttl.refresh.%s", role), 24*time.Hour),
		}
	}

	byDevice := make(map[string]TokenDuration)
	for _, device := range devices {
		byDevice[device] = TokenDuration{
			AccessToken:  v.GetDuration(ctx, fmt.Sprintf("ttl.access.device.%s", device), time.Hour),
			RefreshToken: v.GetDuration(ctx, fmt.Sprintf("ttl.refresh.device.%s", device), 24*time.Hour),
		}
	}

	return &TokenTTLConfig{
		ByRole:   byRole,
		ByDevice: byDevice,
		Default: TokenDuration{
			AccessToken:  v.GetDuration(ctx, "ttl.access.default", time.Hour),
			RefreshToken: v.GetDuration(ctx, "ttl.refresh.default", 24*time.Hour),
		},
	}
}

// GetJWTSecret reads the JWT signing key from ImmuDB.
func (v *ImmuVault) GetJWTSecret(ctx context.Context) string {
	return v.GetString(ctx, "jwt.secret", "")
}

// GetCORSOrigins reads allowed CORS origins from ImmuDB.
// In production, rejects wildcard "*".
func (v *ImmuVault) GetCORSOrigins(ctx context.Context) []string {
	raw := v.GetString(ctx, "cors.allowed_origins", "http://localhost:5173")

	appEnv := os.Getenv("APP_ENV")
	if appEnv == "production" && (raw == "*" || raw == "") {
		log.Println("[FATAL] cors.allowed_origins cannot be '*' or empty in production")
		os.Exit(1)
	}

	return strings.Split(raw, ",")
}

// ============================================================================
// TOKEN TTL CONFIG STRUCTS (shared with auth service)
// ============================================================================

// TokenTTLConfig maps roles and devices to their token lifetimes.
type TokenTTLConfig struct {
	ByRole   map[string]TokenDuration
	ByDevice map[string]TokenDuration
	Default  TokenDuration
}

// TokenDuration holds access and refresh token lifetimes.
type TokenDuration struct {
	AccessToken  time.Duration
	RefreshToken time.Duration
}

// ============================================================================
// HELPERS
// ============================================================================

// generateSecureKey generates a cryptographically secure random key.
// Returns base64-encoded string. keyLen is in bytes (32 = 256-bit).
func generateSecureKey(keyLen int) (string, error) {
	key := make([]byte, keyLen)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("crypto/rand failed: %w", err)
	}
	return base64.URLEncoding.EncodeToString(key), nil
}

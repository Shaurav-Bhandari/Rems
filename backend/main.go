package main

import (
	"backend/DB"
	"backend/config"
	"backend/models"
	"backend/routes"
	services "backend/services/core"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v3"
	fiberlog "github.com/gofiber/fiber/v3/log"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func main() {
	fmt.Println("🚀 Starting ReMS Backend...")

	// ── Load base config ─────────────────────────────────────
	cfg := config.LoadDBConfig()

	// ── Initialize Database ──────────────────────────────────
	db, err := DB.InitDB(cfg)
	if err != nil {
		log.Fatalf("❌ Database initialization failed: %v", err)
	}
	DB.DB = db
	log.Println("✓ Database initialized")

	// ── Initialize Redis ─────────────────────────────────────
	redisCfg := config.LoadRedisConfig()
	redisClient, err := config.NewRedisClient(redisCfg)
	if err != nil {
		log.Printf("⚠️  Redis connection failed: %v — sessions will use JWT-only mode", err)
	}

	// ── Initialize ImmuDB Vault ──────────────────────────────
	var vault *config.ImmuVault
	immuHost := os.Getenv("IMMUDB_HOST")
	if immuHost == "" {
		immuHost = "localhost"
	}
	immuPass := os.Getenv("IMMUDB_PASSWORD")
	if immuPass == "" {
		immuPass = "immudb" // default immudb password
	}

	vault, err = config.NewImmuVault(immuHost, 3322, "immudb", immuPass, "defaultdb")
	if err != nil {
		log.Printf("⚠️  ImmuDB connection failed: %v — using env fallbacks", err)
	} else {
		// Seed default config values (idempotent)
		ctx := context.Background()
		if seedErr := vault.SeedDefaults(ctx); seedErr != nil {
			log.Printf("⚠️  ImmuDB seeding warning: %v", seedErr)
		}
		defer vault.Close()
	}

	// ── Load config from vault (with fallbacks) ──────────────
	var authCfg *config.AuthServiceConfig
	var ttlCfg *config.TokenTTLConfig
	var jwtSecret string

	if vault != nil {
		ctx := context.Background()
		authCfg = vault.GetAuthServiceConfig(ctx)
		ttlCfg = vault.GetTokenTTLConfig(ctx)
		jwtSecret = vault.GetJWTSecret(ctx)
	}

	// Fallback defaults if vault is unavailable
	if authCfg == nil {
		authCfg = &config.AuthServiceConfig{
			MaxSessions: 5, MaxFailedLogins: 5,
			LockoutDuration: 30 * 60e9, PasswordMinLength: 12,
			PasswordHistory: 5, RateLimitCount: 10,
			RateLimitWindow: 60e9,
		}
	}
	if ttlCfg == nil {
		ttlCfg = &config.TokenTTLConfig{
			ByRole:   map[string]config.TokenDuration{},
			ByDevice: map[string]config.TokenDuration{},
			Default:  config.TokenDuration{AccessToken: 3600e9, RefreshToken: 86400e9},
		}
	}
	if jwtSecret == "" {
		jwtSecret = os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			jwtSecret = "CHANGE_ME_IN_PRODUCTION"
			log.Println("⚠️  JWT_SECRET not configured — using insecure default!")
		}
	}

	// ── Initialize services ──────────────────────────────────
	smtpCfg := config.LoadSMTPConfig()
	emailService := services.NewEmailService(smtpCfg)
	if smtpCfg.Enabled {
		log.Println("✓ Email service enabled (SMTP)")
	} else {
		log.Println("⚠️  SMTP not configured — emails will be logged only")
	}

	geoIPService := services.NewGeoIPService()
	log.Println("✓ GeoIP service initialized (ip-api.com)")

	authService := services.NewAuthService(db, redisClient, jwtSecret, authCfg, ttlCfg, emailService, geoIPService)
	sessionService := services.NewSessionService(redisClient)
	tokenService := services.NewTokenService(jwtSecret, redisClient)
	securityService := services.NewSecurityService(db, redisClient, geoIPService)
	rbacService := services.NewRBACService(db, redisClient)
	log.Println("✓ All core services initialized")

	// ── Seed super admin from .env ───────────────────────────
	if err := seedSuperAdmin(db, rbacService); err != nil {
		log.Printf("⚠️  Super admin seed warning: %v", err)
	}

	// ── Initialize Google OAuth ───────────────────────────────
	oauthCfg := config.LoadOAuthConfig()
	var oauthService *services.OAuthService
	if oauthCfg.Enabled {
		oauthService = services.NewOAuthService(db, redisClient, oauthCfg, tokenService, sessionService, geoIPService, ttlCfg)
		log.Println("✓ Google OAuth enabled")
	} else {
		log.Println("⚠️  Google OAuth not configured (GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET missing)")
	}

	// ── Create Fiber app ─────────────────────────────────────
	app := fiber.New(fiber.Config{
		AppName: "ReMS API v1",
	})

	// ── Register routes ──────────────────────────────────────
	deps := &routes.Dependencies{
		DB:              db,
		Redis:           redisClient,
		Vault:           vault,
		AuthService:     authService,
		SessionService:  sessionService,
		TokenService:    tokenService,
		SecurityService: securityService,
		OAuthService:    oauthService,
		OAuthConfig:     oauthCfg,
		RBACService:     rbacService,
	}
	routes.RegisterRoutes(app, deps)

	// ── Start server ─────────────────────────────────────────
	port := "3000"
	if vault != nil {
		if p := vault.GetString(context.Background(), "app.port", ""); p != "" {
			port = p
		}
	}
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	log.Printf("✅ ReMS API ready on :%s", port)
	fiberlog.Fatal(app.Listen(":" + port))
}

// seedSuperAdmin bootstraps the system on first boot:
//  1. Creates a default "System" tenant if none exists
//  2. Seeds all system roles (superadmin, manager, cashier, waiter, chef, customer)
//  3. Creates the super admin user from SUPER_ADMIN_* env vars
//  4. Assigns the superadmin role to that user
//
// All operations are idempotent — safe to call on every startup.
func seedSuperAdmin(db *gorm.DB, rbacService *services.RBACService) error {
	email := os.Getenv("SUPER_ADMIN_EMAIL")
	password := os.Getenv("SUPER_ADMIN_PASSWORD")
	fullName := os.Getenv("SUPER_ADMIN_NAME")

	if email == "" || password == "" {
		log.Println("⚠️  SUPER_ADMIN_EMAIL or SUPER_ADMIN_PASSWORD not set — skipping super admin seed")
		return nil
	}

	if fullName == "" {
		fullName = "System Admin"
	}

	ctx := context.Background()

	// ── Step 1: Ensure a default tenant exists ────────────────
	var tenant models.Tenant
	if err := db.Where("name = ?", "System").First(&tenant).Error; err != nil {
		tenant = models.Tenant{
			TenantID: uuid.New(),
			Name:     "System",
			Status:   "active",
			IsActive: true,
		}
		if err := db.Create(&tenant).Error; err != nil {
			return fmt.Errorf("failed to create system tenant: %w", err)
		}
		log.Println("✓ System tenant created")
	}

	// ── Step 2: Seed system roles ─────────────────────────────
	if rbacService != nil {
		if err := rbacService.SeedSystemRoles(ctx, tenant.TenantID); err != nil {
			log.Printf("⚠️  Role seeding warning: %v", err)
		} else {
			log.Println("✓ System roles seeded")
		}
	}

	// ── Step 3: Create super admin user if not exists ─────────
	var existingUser models.User
	if err := db.Where("email = ? AND tenant_id = ?", email, tenant.TenantID).First(&existingUser).Error; err == nil {
		log.Printf("✓ Super admin already exists (%s)", email)
		// Ensure role is still assigned (Step 4 is idempotent)
		ensureSuperAdminRole(db, existingUser.UserID, tenant.TenantID)
		return nil
	}

	// Hash password using the same Argon2id hasher the login flow expects
	hashedPassword, err := services.HashPwd(password)
	if err != nil {
		return fmt.Errorf("failed to hash super admin password: %w", err)
	}

	now := time.Now()
	user := models.User{
		UserID:          uuid.New(),
		UserName:        "superadmin",
		FullName:        fullName,
		Email:           email,
		PasswordHash:    hashedPassword,
		IsActive:        true,
		IsEmailVerified: true,
		EmailVerifiedAt: &now,
		PrimaryRole:     "super_admin",
		TenantID:        tenant.TenantID,
	}

	if err := db.Create(&user).Error; err != nil {
		return fmt.Errorf("failed to create super admin user: %w", err)
	}
	log.Printf("✓ Super admin user created (%s)", email)

	// ── Step 4: Assign superadmin role ────────────────────────
	ensureSuperAdminRole(db, user.UserID, tenant.TenantID)

	return nil
}

// ensureSuperAdminRole assigns the superadmin system role to the user if not already assigned.
func ensureSuperAdminRole(db *gorm.DB, userID, tenantID uuid.UUID) {
	// Find the superadmin role
	var role models.Role
	if err := db.Where("role_name = ? AND tenant_id = ? AND is_system = true", "superadmin", tenantID).
		First(&role).Error; err != nil {
		log.Printf("⚠️  Could not find superadmin role for tenant %s: %v", tenantID, err)
		return
	}

	// Check if already assigned
	var existing models.UserRole
	if err := db.Where("user_id = ? AND role_id = ? AND tenant_id = ?", userID, role.RoleID, tenantID).
		First(&existing).Error; err == nil {
		return // already assigned
	}

	// Assign
	userRole := models.UserRole{
		UserRoleID: uuid.New(),
		UserID:     userID,
		RoleID:     role.RoleID,
		TenantID:   tenantID,
		AssignedBy: userID, // self-assigned on bootstrap
		AssignedAt: time.Now(),
	}
	if err := db.Create(&userRole).Error; err != nil {
		log.Printf("⚠️  Failed to assign superadmin role: %v", err)
		return
	}
	log.Println("✓ Superadmin role assigned")
}

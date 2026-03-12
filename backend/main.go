package main

import (
	"backend/DB"
	"backend/config"
	"backend/routes"
	services "backend/services/core"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v3"
	fiberlog "github.com/gofiber/fiber/v3/log"
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
	emailService := &services.EmailService{}
	geoIPService := &services.GeoIPService{}

	authService := services.NewAuthService(db, redisClient, jwtSecret, authCfg, ttlCfg, emailService, geoIPService)
	sessionService := services.NewSessionService(redisClient)
	tokenService := services.NewTokenService(jwtSecret, redisClient)
	securityService := services.NewSecurityService(db, redisClient, geoIPService)

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

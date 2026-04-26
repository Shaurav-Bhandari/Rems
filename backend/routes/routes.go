package routes

import (
	"backend/config"
	"backend/handlers"
	"backend/middleware"
	services "backend/services/core"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)


// Dependencies holds all service instances needed by route handlers.
type Dependencies struct {
	DB              *gorm.DB
	Redis           *redis.Client
	Vault           *config.ImmuVault
	AuthService     *services.AuthService
	SessionService  *services.SessionService
	TokenService    *services.TokenService
	SecurityService *services.SecurityService
	OAuthService    *services.OAuthService
	OAuthConfig     config.OAuthConfig
}

// RegisterRoutes wires all route groups with middleware chains.
// All routes use lowercase /api/v1/ prefix.
func RegisterRoutes(app *fiber.App, deps *Dependencies) {
	// ── Handlers ──────────────────────────────────────────────
	authH := handlers.NewAuthHandler(
		deps.AuthService,
		deps.SessionService,
		deps.TokenService,
		deps.SecurityService,
		deps.Vault,
		deps.Redis,
	)
	restaurantH := handlers.NewRestaurantHandler(deps.DB)
	orderH := handlers.NewOrderHandler(deps.DB)
	menuH := handlers.NewMenuHandler(deps.DB)
	inventoryH := handlers.NewInventoryHandler(deps.DB)
	userH := handlers.NewUserHandler(deps.DB)
	analyticsH := handlers.NewAnalyticsHandler(deps.DB)
	healthH := handlers.NewHealthHandler(deps.Redis, deps.Vault)

	// ── Global middleware (applied to ALL routes) ─────────────
	app.Use(middleware.RequestID())
	app.Use(middleware.Logger())
	app.Use(middleware.Recovery())
	app.Use(middleware.SecurityHeaders())

	api := app.Group("/api/v1")

	// ── Health (public) ──────────────────────────────────────
	health := api.Group("/health")
	health.Get("/", healthH.Health)
	health.Get("/redis", healthH.RedisHealth)
	health.Get("/immudb", healthH.ImmuDBHealth)

	// ── Auth (public + some authenticated) ───────────────────
	auth := api.Group("/auth")
	auth.Post("/login", authH.Login)
	auth.Post("/register", authH.Register)
	auth.Post("/refresh", authH.RefreshToken)
	auth.Post("/forgot-password", authH.ForgotPassword)
	auth.Post("/reset-password", authH.ResetPassword)
	auth.Post("/verify-2fa", authH.Verify2FA)

	// ── Google OAuth (public) ────────────────────────────────
	if deps.OAuthService != nil && deps.OAuthService.IsEnabled() {
		oauthH := handlers.NewOAuthHandler(deps.OAuthService, deps.OAuthConfig, deps.Redis)
		auth.Get("/google", oauthH.GoogleLogin)
		auth.Get("/google/callback", oauthH.GoogleCallback)
	}

	// Auth-protected auth routes
	authProtected := auth.Group("", middleware.Auth(deps.Redis))
	authProtected.Post("/logout", authH.Logout)
	authProtected.Post("/change-password", authH.ChangePassword)
	authProtected.Get("/sessions", authH.ListSessions)
	authProtected.Delete("/sessions/:id", authH.RevokeSession)

	// ── Authenticated + Tenant-scoped routes ─────────────────
	authenticated := api.Group("",
		middleware.Auth(deps.Redis),
		middleware.TenantIsolation(),
	)

	// Users (admin-only)
	users := authenticated.Group("/users")
	users.Get("/", userH.List)
	users.Get("/:id", userH.Get)
	users.Put("/:id", userH.Update)
	users.Delete("/:id", userH.Delete)

	// Restaurants
	restaurants := authenticated.Group("/restaurants")
	restaurants.Get("/", restaurantH.List)
	restaurants.Post("/", restaurantH.Create)
	restaurants.Get("/:id", restaurantH.Get)
	restaurants.Put("/:id", restaurantH.Update)
	restaurants.Delete("/:id", restaurantH.Delete)

	// Orders
	orders := authenticated.Group("/orders")
	orders.Get("/", orderH.List)
	orders.Post("/", orderH.Create)
	orders.Get("/:id", orderH.Get)
	orders.Put("/:id/status", orderH.UpdateStatus)
	orders.Delete("/:id", orderH.Delete)

	// Menu
	menu := authenticated.Group("/menu")
	menu.Get("/categories", menuH.ListCategories)
	menu.Post("/categories", menuH.CreateCategory)
	menu.Get("/items", menuH.ListItems)
	menu.Post("/items", menuH.CreateItem)
	menu.Put("/items/:id", menuH.UpdateItem)
	menu.Delete("/items/:id", menuH.DeleteItem)

	// Inventory
	inventory := authenticated.Group("/inventory")
	inventory.Get("/", inventoryH.List)
	inventory.Post("/", inventoryH.Create)
	inventory.Get("/:id", inventoryH.Get)
	inventory.Put("/:id", inventoryH.Update)
	inventory.Post("/:id/adjust", inventoryH.AdjustStock)
	inventory.Delete("/:id", inventoryH.Delete)

	// Analytics
	analytics := authenticated.Group("/analytics")
	analytics.Get("/revenue/overview", analyticsH.RevenueOverview)
	analytics.Get("/revenue/trend", analyticsH.RevenueTrend)
	analytics.Get("/orders/volume", analyticsH.OrderVolume)
	analytics.Get("/orders/status", analyticsH.OrderStatusDistribution)
	analytics.Get("/menu/top-items", analyticsH.TopSellingItems)
	analytics.Get("/inventory/turnover", analyticsH.InventoryTurnover)
	analytics.Get("/forecast/demand", analyticsH.ForecastDemand)
}

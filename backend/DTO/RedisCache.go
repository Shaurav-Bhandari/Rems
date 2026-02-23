package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// REDIS CACHE DTOs - For frequently accessed data
// ============================================================================

// CachedUserSession represents a user session stored in Redis
// Key format: "session:{session_id}"
// TTL: Based on JWT expiration
type CachedUserSession struct {
	SessionID    uuid.UUID   `json:"session_id"`
	UserID       uuid.UUID   `json:"user_id"`
	TenantID     uuid.UUID   `json:"tenant_id"`
	Email        string      `json:"email"`
	FullName     string      `json:"full_name"`
	Role         string      `json:"role"`           // Default role name
	RoleIDs      []uuid.UUID `json:"role_ids"`       // All role IDs
	Permissions  []string    `json:"permissions"`    // Flattened permissions list
	RestaurantID *uuid.UUID  `json:"restaurant_id,omitempty"`
	BranchID     *uuid.UUID  `json:"branch_id,omitempty"`
	IPAddress    string      `json:"ip_address"`
	UserAgent    string      `json:"user_agent"`
	DeviceInfo   DeviceInfo  `json:"device_info"`
	CreatedAt    time.Time   `json:"created_at"`
	ExpiresAt    time.Time   `json:"expires_at"`
	LastActivity time.Time   `json:"last_activity"`
	IsRevoked    bool        `json:"is_revoked"`
}

// DeviceInfo represents device information for session tracking
type DeviceInfo struct {
	DeviceType string `json:"device_type"` // mobile, desktop, tablet
	OS         string `json:"os"`          // iOS, Android, Windows, macOS, Linux
	Browser    string `json:"browser"`     // Chrome, Safari, Firefox, etc.
	DeviceID   string `json:"device_id"`   // Unique device identifier
}

// CachedUserProfile represents user profile data stored in Redis
// Key format: "user:profile:{user_id}"
// TTL: 1 hour (refresh on access)
type CachedUserProfile struct {
	UserID         uuid.UUID       `json:"user_id"`
	UserName       string          `json:"user_name"`
	FullName       string          `json:"full_name"`
	Email          string          `json:"email"`
	Phone          string          `json:"phone"`
	TenantID       uuid.UUID       `json:"tenant_id"`
	TenantName     string          `json:"tenant_name"`
	OrganizationID uuid.UUID       `json:"organization_id"`
	BranchID       uuid.UUID       `json:"branch_id"`
	DefaultRole    RoleSummaryDTO  `json:"default_role"`
	Roles          []RoleSummaryDTO `json:"roles"`
	Permissions    []string        `json:"permissions"`
	IsActive       bool            `json:"is_active"`
	CachedAt       time.Time       `json:"cached_at"`
}

// CachedRestaurantData represents restaurant data stored in Redis
// Key format: "restaurant:{restaurant_id}"
// TTL: 30 minutes (refresh on update)
type CachedRestaurantData struct {
	RestaurantID     uuid.UUID                `json:"restaurant_id"`
	TenantID         uuid.UUID                `json:"tenant_id"`
	BranchID         *uuid.UUID               `json:"branch_id,omitempty"`
	Name             string                   `json:"name"`
	City             string                   `json:"city"`
	Phone            string                   `json:"phone"`
	Email            string                   `json:"email"`
	IsActive         bool                     `json:"is_active"`
	RegionalSettings *RegionalSettingResponse `json:"regional_settings,omitempty"`
	Stats            *RestaurantStatsDTO      `json:"stats,omitempty"`
	CachedAt         time.Time                `json:"cached_at"`
	TTL              int64                    `json:"ttl"` // seconds
}

// CachedMenuItem represents menu item data stored in Redis
// Key format: "menu:item:{menu_item_id}"
// TTL: 15 minutes (highly accessed during ordering)
type CachedMenuItem struct {
	MenuItemID     uuid.UUID   `json:"menu_item_id"`
	RestaurantID   uuid.UUID   `json:"restaurant_id"`
	CategoryID     uuid.UUID   `json:"category_id"`
	CategoryName   string      `json:"category_name"`
	Name           string      `json:"name"`
	Description    string      `json:"description"`
	BasePrice      float64     `json:"base_price"`
	IsAvailable    bool        `json:"is_available"`
	PrepTime       *int        `json:"prep_time_minutes,omitempty"`
	ImageURL       string      `json:"image_url,omitempty"`
	Modifiers      []ModifierCache `json:"modifiers,omitempty"`
	DietaryFlags   map[string]interface{} `json:"dietary_flags,omitempty"`
	CachedAt       time.Time   `json:"cached_at"`
}

// ModifierCache represents a cached menu item modifier
type ModifierCache struct {
	ModifierID      uuid.UUID `json:"modifier_id"`
	Name            string    `json:"name"`
	PriceAdjustment float64   `json:"price_adjustment"`
	IsAvailable     bool      `json:"is_available"`
}

// CachedMenuCategory represents menu category with items
// Key format: "menu:category:{category_id}"
// TTL: 15 minutes
type CachedMenuCategory struct {
	CategoryID   uuid.UUID            `json:"category_id"`
	RestaurantID uuid.UUID            `json:"restaurant_id"`
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	DisplayOrder int                  `json:"display_order"`
	IsActive     bool                 `json:"is_active"`
	Items        []CachedMenuItem     `json:"items"`
	CachedAt     time.Time            `json:"cached_at"`
}

// CachedTableStatus represents real-time table status
// Key format: "table:status:{restaurant_id}:{table_id}"
// TTL: 5 minutes (highly dynamic)
type CachedTableStatus struct {
	TableID          int       `json:"table_id"`
	RestaurantID     uuid.UUID `json:"restaurant_id"`
	FloorID          int       `json:"floor_id"`
	TableNumber      string    `json:"table_number"`
	Capacity         int       `json:"capacity"`
	CurrentOccupancy int       `json:"current_occupancy"`
	Status           string    `json:"status"` // available, occupied, reserved, cleaning
	CurrentOrderID   *uuid.UUID `json:"current_order_id,omitempty"`
	LastUpdated      time.Time `json:"last_updated"`
}

// CachedOrderSummary represents order summary for quick access
// Key format: "order:summary:{order_id}"
// TTL: 10 minutes
type CachedOrderSummary struct {
	OrderID      uuid.UUID `json:"order_id"`
	RestaurantID uuid.UUID `json:"restaurant_id"`
	TableID      *int      `json:"table_id,omitempty"`
	CustomerName string    `json:"customer_name"`
	OrderStatus  string    `json:"order_status"`
	TotalAmount  float64   `json:"total_amount"`
	ItemCount    int       `json:"item_count"`
	CreatedAt    time.Time `json:"created_at"`
	CachedAt     time.Time `json:"cached_at"`
}

// CachedInventoryLevel represents inventory levels for quick checks
// Key format: "inventory:level:{inventory_item_id}"
// TTL: 5 minutes (dynamic)
type CachedInventoryLevel struct {
	InventoryItemID uuid.UUID `json:"inventory_item_id"`
	RestaurantID    uuid.UUID `json:"restaurant_id"`
	Name            string    `json:"name"`
	SKU             string    `json:"sku"`
	CurrentQuantity float64   `json:"current_quantity"`
	MinimumQuantity float64   `json:"minimum_quantity"`
	ReorderPoint    float64   `json:"reorder_point"`
	IsLowStock      bool      `json:"is_low_stock"`
	IsOutOfStock    bool      `json:"is_out_of_stock"`
	LastUpdated     time.Time `json:"last_updated"`
}

// ============================================================================
// SESSION MANAGEMENT DTOs
// ============================================================================

// CreateSessionRequest represents creating a new session (internal use)
type CreateSessionRequest struct {
	UserID     uuid.UUID  `json:"user_id"`
	IPAddress  string     `json:"ip_address"`
	UserAgent  string     `json:"user_agent"`
	DeviceInfo DeviceInfo `json:"device_info"`
}

// SessionResponse represents a user session
type SessionResponse struct {
	SessionID    uuid.UUID  `json:"session_id"`
	UserID       uuid.UUID  `json:"user_id"`
	IPAddress    string     `json:"ip_address"`
	DeviceInfo   DeviceInfo `json:"device_info"`
	CreatedAt    time.Time  `json:"created_at"`
	LastActivity time.Time  `json:"last_activity"`
	ExpiresAt    time.Time  `json:"expires_at"`
	IsRevoked    bool       `json:"is_revoked"`
	IsCurrent    bool       `json:"is_current"` // Is this the current session
}

// ListSessionsResponse represents all active sessions for a user
type ListSessionsResponse struct {
	Sessions      []SessionResponse `json:"sessions"`
	ActiveCount   int               `json:"active_count"`
	RevokedCount  int               `json:"revoked_count"`
	CurrentDevice string            `json:"current_device"`
}

// RevokeSessionRequest represents revoking a session
type RevokeSessionRequest struct {
	SessionID uuid.UUID `json:"session_id" binding:"required"`
}

// RevokeAllSessionsRequest revokes all sessions except current (optional)
type RevokeAllSessionsRequest struct {
	ExceptCurrent bool `json:"except_current"` // Don't revoke the current session
}

// RefreshTokenRequest represents a token refresh request
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// RefreshTokenResponse represents a token refresh response
type RefreshTokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// ValidateSessionRequest validates if a session is still active
type ValidateSessionRequest struct {
	SessionID uuid.UUID `json:"session_id" binding:"required"`
}

// ValidateSessionResponse returns session validity
type ValidateSessionResponse struct {
	IsValid      bool      `json:"is_valid"`
	SessionID    uuid.UUID `json:"session_id,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	LastActivity time.Time `json:"last_activity,omitempty"`
	Message      string    `json:"message,omitempty"`
}

// ============================================================================
// CACHE INVALIDATION DTOs
// ============================================================================

// InvalidateCacheRequest represents a cache invalidation request
type InvalidateCacheRequest struct {
	CacheType string      `json:"cache_type" binding:"required,oneof=user restaurant menu table order inventory all"`
	EntityID  *uuid.UUID  `json:"entity_id,omitempty"` // Optional: specific entity
	Pattern   string      `json:"pattern,omitempty"`   // Optional: Redis key pattern
}

// CacheStatsResponse represents Redis cache statistics
type CacheStatsResponse struct {
	TotalKeys       int64                  `json:"total_keys"`
	MemoryUsed      string                 `json:"memory_used"`
	HitRate         float64                `json:"hit_rate"`
	MissRate        float64                `json:"miss_rate"`
	KeysByType      map[string]int64       `json:"keys_by_type"`
	OldestKey       time.Time              `json:"oldest_key"`
	NewestKey       time.Time              `json:"newest_key"`
	ExpiringIn1Hour int64                  `json:"expiring_in_1_hour"`
}

// WarmCacheRequest represents a request to pre-populate cache
type WarmCacheRequest struct {
	CacheTypes   []string     `json:"cache_types" binding:"required,min=1"` // e.g., ["menu", "restaurant", "table"]
	RestaurantID *uuid.UUID   `json:"restaurant_id,omitempty"`
	Force        bool         `json:"force"` // Force refresh even if already cached
}

// ============================================================================
// REDIS PUB/SUB DTOs (for real-time updates)
// ============================================================================

// RedisPubSubMessage represents a message published to Redis pub/sub
type RedisPubSubMessage struct {
	Channel   string                 `json:"channel"`
	EventType string                 `json:"event_type"`
	EntityID  uuid.UUID              `json:"entity_id"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// TableStatusUpdate published when table status changes
// Channel: "table:updates:{restaurant_id}"
type TableStatusUpdate struct {
	TableID          int       `json:"table_id"`
	RestaurantID     uuid.UUID `json:"restaurant_id"`
	Status           string    `json:"status"`
	CurrentOccupancy int       `json:"current_occupancy"`
	UpdatedBy        uuid.UUID `json:"updated_by"`
	Timestamp        time.Time `json:"timestamp"`
}

// OrderStatusUpdate published when order status changes
// Channel: "order:updates:{restaurant_id}"
type OrderStatusUpdate struct {
	OrderID      uuid.UUID `json:"order_id"`
	RestaurantID uuid.UUID `json:"restaurant_id"`
	OldStatus    string    `json:"old_status"`
	NewStatus    string    `json:"new_status"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
	Timestamp    time.Time `json:"timestamp"`
}

// MenuItemUpdate published when menu availability changes
// Channel: "menu:updates:{restaurant_id}"
type MenuItemUpdate struct {
	MenuItemID   uuid.UUID `json:"menu_item_id"`
	RestaurantID uuid.UUID `json:"restaurant_id"`
	IsAvailable  bool      `json:"is_available"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
	Timestamp    time.Time `json:"timestamp"`
}

// InventoryAlert published when inventory is low/out
// Channel: "inventory:alerts:{restaurant_id}"
type InventoryAlert struct {
	InventoryItemID uuid.UUID `json:"inventory_item_id"`
	RestaurantID    uuid.UUID `json:"restaurant_id"`
	ItemName        string    `json:"item_name"`
	CurrentQuantity float64   `json:"current_quantity"`
	MinimumQuantity float64   `json:"minimum_quantity"`
	AlertType       string    `json:"alert_type"` // low_stock, out_of_stock, reorder_needed
	Timestamp       time.Time `json:"timestamp"`
}

// ============================================================================
// CACHE HELPER METHODS
// ============================================================================

// GetCacheKey returns the Redis key for different entity types
func GetCacheKey(cacheType string, entityID uuid.UUID) string {
	switch cacheType {
	case "user_profile":
		return "user:profile:" + entityID.String()
	case "user_session":
		return "session:" + entityID.String()
	case "restaurant":
		return "restaurant:" + entityID.String()
	case "menu_item":
		return "menu:item:" + entityID.String()
	case "menu_category":
		return "menu:category:" + entityID.String()
	case "order_summary":
		return "order:summary:" + entityID.String()
	case "inventory_level":
		return "inventory:level:" + entityID.String()
	default:
		return cacheType + ":" + entityID.String()
	}
}

// GetTableCacheKey returns the Redis key for table status
func GetTableCacheKey(restaurantID uuid.UUID, tableID int) string {
	return "table:status:" + restaurantID.String() + ":" + string(rune(tableID))
}

// GetDefaultTTL returns the default TTL for different cache types in seconds
func GetDefaultTTL(cacheType string) int64 {
	ttlMap := map[string]int64{
		"user_session":    3600,  // 1 hour
		"user_profile":    3600,  // 1 hour
		"restaurant":      1800,  // 30 minutes
		"menu_item":       900,   // 15 minutes
		"menu_category":   900,   // 15 minutes
		"table_status":    300,   // 5 minutes
		"order_summary":   600,   // 10 minutes
		"inventory_level": 300,   // 5 minutes
	}
	
	if ttl, exists := ttlMap[cacheType]; exists {
		return ttl
	}
	return 600 // Default 10 minutes
}
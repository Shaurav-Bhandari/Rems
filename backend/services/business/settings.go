package business

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"backend/models"
)

const (
	settingsCacheTTL = 2 * time.Minute
	settingsProfileTTL = 5 * time.Minute
)

// SettingsService provides read access to restaurant configuration.
// Goroutine-safe; construct one per application process.
type SettingsService struct {
	db    *gorm.DB
	redis *goredis.Client
}

func NewSettingsService(db *gorm.DB, redis *goredis.Client) *SettingsService {
	return &SettingsService{db: db, redis: redis}
}

// ─────────────────────────────────────────────────────────────────────────────
// PRIMARY: RestaurantProfile
// ─────────────────────────────────────────────────────────────────────────────

// GetRestaurantProfile returns the current RestaurantProfile, using Redis as
// a read-through cache. This is the hot-path call — every order submission
// hits this. immudb is never involved here.
func (s *SettingsService) GetRestaurantProfile(
	ctx context.Context,
	restaurantID uuid.UUID,
) (*models.RestaurantProfile, error) {
	cacheKey := fmt.Sprintf("profile:%s", restaurantID)

	if cached, err := s.redis.Get(ctx, cacheKey).Bytes(); err == nil {
		var p models.RestaurantProfile
		if json.Unmarshal(cached, &p) == nil {
			return &p, nil
		}
	}

	var profile models.RestaurantProfile
	if err := s.db.WithContext(ctx).
		Where("restaurant_id = ?", restaurantID).
		First(&profile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("restaurant profile not found: %s", restaurantID)
		}
		return nil, fmt.Errorf("GetRestaurantProfile: %w", err)
	}

	if b, err := json.Marshal(&profile); err == nil {
		_ = s.redis.Set(ctx, cacheKey, b, settingsProfileTTL).Err()
	}
	return &profile, nil
}

// InvalidateProfileCache evicts the profile from Redis.
// FinancialConfigService calls this after every tax rate update.
func (s *SettingsService) InvalidateProfileCache(ctx context.Context, restaurantID uuid.UUID) {
	_ = s.redis.Del(ctx, fmt.Sprintf("profile:%s", restaurantID)).Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// SECONDARY: Key/value setting reads
// Used for per-restaurant overrides that don't live in RestaurantProfile:
//   "tax_enabled"              — master toggle
//   "tax_label"                — display label ("VAT", "GST", "Sales Tax")
//   "takeaway_tax_rate"        — separate rate for delivery/takeaway orders
//   "service_charge_before_tax" — bool: apply service charge before tax?
// ─────────────────────────────────────────────────────────────────────────────

// GetSetting returns a setting value as a raw string.
// Resolution: SettingOverride (restaurant) → TenantSetting → error
func (s *SettingsService) GetSetting(
	ctx context.Context,
	tenantID uuid.UUID,
	restaurantID *uuid.UUID,
	key string,
) (string, error) {
	cacheKey := buildSettingCacheKey(tenantID, restaurantID, key)

	if cached, err := s.redis.Get(ctx, cacheKey).Result(); err == nil {
		return cached, nil
	}

	var value string
	found := false

	// 1. Restaurant-level override (highest priority)
	if restaurantID != nil {
		var override models.SettingOverride
		err := s.db.WithContext(ctx).
			Where("tenant_id = ? AND restaurant_id = ? AND key = ?", tenantID, restaurantID, key).
			Order("priority DESC").
			First(&override).Error
		if err == nil {
			value = override.Value
			found = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("setting override lookup: %w", err)
		}
	}

	// 2. Tenant-level default
	if !found {
		var ts models.TenantSetting
		err := s.db.WithContext(ctx).
			Where("tenant_id = ? AND key = ?", tenantID, key).
			First(&ts).Error
		if err == nil {
			value = ts.Value
			found = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("tenant setting lookup: %w", err)
		}
	}

	if !found {
		return "", fmt.Errorf("setting %q not found", key)
	}

	_ = s.redis.Set(ctx, cacheKey, value, settingsCacheTTL).Err()
	return value, nil
}

// GetBool returns a boolean setting. Returns an error if the setting is missing
// or not parseable. Callers use a default on error:
//
//	taxEnabled, err := svc.GetBool(ctx, tenantID, &rID, "tax_enabled")
//	if err != nil { taxEnabled = true } // safe default
func (s *SettingsService) GetBool(
	ctx context.Context,
	tenantID uuid.UUID,
	restaurantID *uuid.UUID,
	key string,
) (bool, error) {
	raw, err := s.GetSetting(ctx, tenantID, restaurantID, key)
	if err != nil {
		return false, err
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("setting %q: cannot parse %q as bool", key, raw)
	}
	return v, nil
}

// GetFloat returns a float64 setting.
func (s *SettingsService) GetFloat(
	ctx context.Context,
	tenantID uuid.UUID,
	restaurantID *uuid.UUID,
	key string,
) (float64, error) {
	raw, err := s.GetSetting(ctx, tenantID, restaurantID, key)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("setting %q: cannot parse %q as float64", key, raw)
	}
	return v, nil
}

// GetInt returns an integer setting.
func (s *SettingsService) GetInt(
	ctx context.Context,
	tenantID uuid.UUID,
	restaurantID *uuid.UUID,
	key string,
) (int, error) {
	raw, err := s.GetSetting(ctx, tenantID, restaurantID, key)
	if err != nil {
		return 0, err
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("setting %q: cannot parse %q as int", key, raw)
	}
	return v, nil
}

func buildSettingCacheKey(tenantID uuid.UUID, restaurantID *uuid.UUID, key string) string {
	if restaurantID != nil {
		return fmt.Sprintf("setting:%s:%s:%s", tenantID, *restaurantID, key)
	}
	return fmt.Sprintf("setting:%s:tenant:%s", tenantID, key)
}
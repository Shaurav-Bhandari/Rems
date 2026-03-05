// services/business/Menu.go
//
// MenuService — four-concern menu management engine.
//
// ┌────────────────────────────────────────────────────────────────────────┐
// │  CONCERN 1 — CATEGORY LIFECYCLE                                        │
// │  Create  →  Update  →  Reorder  →  Toggle active  →  Delete guard     │
// ├────────────────────────────────────────────────────────────────────────┤
// │  CONCERN 2 — ITEM LIFECYCLE                                            │
// │  Create  →  Update  →  Toggle availability  →  Delete guard           │
// │  Active pricing resolution  →  DietaryFlags JSONB read/write          │
// ├────────────────────────────────────────────────────────────────────────┤
// │  CONCERN 3 — PRICING ENGINE                                            │
// │  Time-bounded price bands  →  Active price resolution                 │
// │  Overlap validation  →  Batch price fetch  →  Price history           │
// ├────────────────────────────────────────────────────────────────────────┤
// │  CONCERN 4 — READ PATH (HOT)                                           │
// │  Redis read-through  →  Full menu assembly (no N+1)                   │
// │  Paginated item listing  →  Active price injection per item            │
// └────────────────────────────────────────────────────────────────────────┘
//
// Cache topology:
//   menu:full:{restaurantID}             — FullMenuAssembly, TTL 2m
//   menu:category:{categoryID}           — MenuCategory,     TTL 5m
//   menu:item:{itemID}                   — MenuItem,         TTL 5m
//   menu:price:{itemID}:{restaurantID}   — float64,          TTL 1m
//
// All mutating operations (Create/Update/Delete) call the relevant cache
// invalidators before returning so the next read is always consistent.
package business

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// SENTINEL ERRORS
// ─────────────────────────────────────────────────────────────────────────────

var (
	ErrMenuCategoryNotFound          = errors.New("menu category not found")
	ErrMenuItemNotFound              = errors.New("menu item not found")
	ErrMenuModifierNotFound          = errors.New("menu modifier not found")
	ErrMenuPricingNotFound           = errors.New("menu pricing record not found")
	ErrCategoryHasItems              = errors.New("cannot delete category that still has active items")
	ErrItemHasActiveOrders           = errors.New("cannot delete item referenced by open orders")
	ErrPriceOverlap                  = errors.New("pricing period overlaps with an existing active price band")
	ErrPriceEffectiveToBeforeFrom    = errors.New("effective_to cannot be before effective_from")
	ErrMenuTenantMismatch            = errors.New("resource does not belong to this tenant")
	ErrMenuRestaurantMismatch        = errors.New("resource does not belong to this restaurant")
)

// ─────────────────────────────────────────────────────────────────────────────
// RESOURCE / ACTION CONSTANTS
// (extend the set already declared in Order.go — same package, no collision)
// ─────────────────────────────────────────────────────────────────────────────

const (
	ResourceMenu     ResourceType = "menu"
	ResourceMenuItem ResourceType = "menu_item"

	ActionUpdate ActionType = "update"
	ActionDelete ActionType = "delete"
	ActionRead   ActionType = "read"
)

// ─────────────────────────────────────────────────────────────────────────────
// INPUT TYPES
// (service layer stays decoupled from the HTTP DTO layer)
// ─────────────────────────────────────────────────────────────────────────────

// MenuServiceRequest carries caller identity for every mutation.
type MenuServiceRequest struct {
	TenantID     uuid.UUID
	RestaurantID uuid.UUID
	ActorID      uuid.UUID // written to audit trail
}

// CreateCategoryInput is the minimal data needed to create a category.
type CreateCategoryInput struct {
	MenuServiceRequest
	Name         string
	Description  string
	DisplayOrder *int
}

// UpdateCategoryInput carries fields that may change. nil pointer = leave as-is.
type UpdateCategoryInput struct {
	MenuServiceRequest
	CategoryID   uuid.UUID
	Name         *string
	Description  *string
	DisplayOrder *int
	IsActive     *bool
}

// CreateItemInput is the minimal data needed to create a menu item.
type CreateItemInput struct {
	MenuServiceRequest
	CategoryID             uuid.UUID
	Name                   string
	Description            string
	BasePrice              float64
	PreparationTimeMinutes *int
	ImageURL               string
	AllergenInfo           string
	DietaryFlags           map[string]interface{} // stored as models.JSONB
}

// UpdateItemInput carries fields that may change on a menu item.
type UpdateItemInput struct {
	MenuServiceRequest
	ItemID                 uuid.UUID
	CategoryID             *uuid.UUID
	Name                   *string
	Description            *string
	BasePrice              *float64
	IsAvailable            *bool
	PreparationTimeMinutes *int
	ImageURL               *string
	AllergenInfo           *string
	DietaryFlags           map[string]interface{} // nil = do not change
}

// CreatePricingInput creates a time-bounded price band.
type CreatePricingInput struct {
	MenuServiceRequest
	ItemID        uuid.UUID
	Price         float64
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
}

// CreateModifierInput adds a modifier option to an existing item.
type CreateModifierInput struct {
	MenuServiceRequest
	ItemID          uuid.UUID
	Name            string
	PriceAdjustment float64
}

// UpdateModifierInput patches an existing modifier.
type UpdateModifierInput struct {
	MenuServiceRequest
	ModifierID      uuid.UUID
	Name            *string
	PriceAdjustment *float64
	IsAvailable     *bool
}

// ListItemsFilter controls pagination and filtering for item listing.
type ListItemsFilter struct {
	MenuServiceRequest
	CategoryID  *uuid.UUID
	IsAvailable *bool
	Search      *string
	MinPrice    *float64
	MaxPrice    *float64
	Page        int
	PageSize    int
	SortBy      string // "name" | "base_price" | "created_at"
	SortOrder   string // "asc" | "desc"
}

// ─────────────────────────────────────────────────────────────────────────────
// CACHE TTLs
// ─────────────────────────────────────────────────────────────────────────────

const (
	menuFullCacheTTL     = 2 * time.Minute
	menuCategoryCacheTTL = 5 * time.Minute
	menuItemCacheTTL     = 5 * time.Minute
	menuPriceCacheTTL    = 1 * time.Minute
)

// ─────────────────────────────────────────────────────────────────────────────
// SERVICE
// ─────────────────────────────────────────────────────────────────────────────

// MenuService manages the full menu lifecycle for a tenant/restaurant pair.
// Goroutine-safe; construct one per application process.
type MenuService struct {
	db    *gorm.DB
	redis *goredis.Client
	rbac  RBACAuthorizer
}

// NewMenuService constructs a ready-to-use MenuService.
func NewMenuService(
	db *gorm.DB,
	redis *goredis.Client,
	rbac RBACAuthorizer,
) *MenuService {
	return &MenuService{db: db, redis: redis, rbac: rbac}
}

// ─────────────────────────────────────────────────────────────────────────────
// CONCERN 1 — CATEGORY LIFECYCLE
// ─────────────────────────────────────────────────────────────────────────────

// CreateCategory creates a new menu category for the given restaurant.
// RBAC: requires menu:create.
func (s *MenuService) CreateCategory(
	ctx context.Context,
	in CreateCategoryInput,
) (*models.MenuCategory, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceMenu,
		Action:   ActionCreate,
	}); err != nil {
		return nil, err
	}

	category := &models.MenuCategory{
		MenuCategoryID: uuid.New(),
		TenantID:       in.TenantID,
		RestaurantID:   in.RestaurantID,
		Name:           strings.TrimSpace(in.Name),
		Description:    strings.TrimSpace(in.Description),
		DisplayOrder:   in.DisplayOrder,
		IsActive:       true,
	}

	if err := s.db.WithContext(ctx).Create(category).Error; err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventCreate, "MenuCategory", category.MenuCategoryID.String(),
		nil, map[string]interface{}{"name": category.Name})

	s.invalidateCategoryCache(ctx, category.MenuCategoryID)
	s.invalidateFullMenuCache(ctx, in.RestaurantID)

	return category, nil
}

// GetCategory returns a single category, using Redis read-through.
func (s *MenuService) GetCategory(
	ctx context.Context,
	tenantID, categoryID uuid.UUID,
) (*models.MenuCategory, error) {
	cacheKey := fmt.Sprintf("menu:category:%s", categoryID)
	if cached, err := s.redis.Get(ctx, cacheKey).Bytes(); err == nil {
		var c models.MenuCategory
		if json.Unmarshal(cached, &c) == nil {
			if c.TenantID != tenantID {
				return nil, ErrMenuTenantMismatch
			}
			return &c, nil
		}
	}

	var category models.MenuCategory
	if err := s.db.WithContext(ctx).
		Where("menu_category_id = ? AND tenant_id = ?", categoryID, tenantID).
		First(&category).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMenuCategoryNotFound
		}
		return nil, fmt.Errorf("get category: %w", err)
	}

	if b, err := json.Marshal(&category); err == nil {
		_ = s.redis.Set(ctx, cacheKey, b, menuCategoryCacheTTL).Err()
	}
	return &category, nil
}

// ListCategories returns all categories for a restaurant, ordered by DisplayOrder.
func (s *MenuService) ListCategories(
	ctx context.Context,
	tenantID, restaurantID uuid.UUID,
	activeOnly bool,
) ([]models.MenuCategory, error) {
	q := s.db.WithContext(ctx).
		Where("tenant_id = ? AND restaurant_id = ?", tenantID, restaurantID).
		Order("display_order ASC NULLS LAST, name ASC")

	if activeOnly {
		q = q.Where("is_active = true")
	}

	var categories []models.MenuCategory
	if err := q.Find(&categories).Error; err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	return categories, nil
}

// UpdateCategory applies a partial update to an existing category.
// RBAC: requires menu:update.
func (s *MenuService) UpdateCategory(
	ctx context.Context,
	in UpdateCategoryInput,
) (*models.MenuCategory, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceMenu,
		Action:   ActionUpdate,
	}); err != nil {
		return nil, err
	}

	category, err := s.GetCategory(ctx, in.TenantID, in.CategoryID)
	if err != nil {
		return nil, err
	}
	if category.RestaurantID != in.RestaurantID {
		return nil, ErrMenuRestaurantMismatch
	}

	old := snapshotCategory(category)
	updates := map[string]interface{}{"updated_at": time.Now()}

	if in.Name != nil {
		category.Name = strings.TrimSpace(*in.Name)
		updates["name"] = category.Name
	}
	if in.Description != nil {
		category.Description = strings.TrimSpace(*in.Description)
		updates["description"] = category.Description
	}
	if in.DisplayOrder != nil {
		category.DisplayOrder = in.DisplayOrder
		updates["display_order"] = in.DisplayOrder
	}
	if in.IsActive != nil {
		category.IsActive = *in.IsActive
		updates["is_active"] = *in.IsActive
	}

	if err := s.db.WithContext(ctx).
		Model(category).
		Clauses(clause.Returning{}).
		Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update category: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventUpdate, "MenuCategory", in.CategoryID.String(),
		old, snapshotCategory(category))

	s.invalidateCategoryCache(ctx, in.CategoryID)
	s.invalidateFullMenuCache(ctx, in.RestaurantID)

	return category, nil
}

// DeleteCategory hard-deletes a category after verifying no active items remain.
// RBAC: requires menu:delete.
func (s *MenuService) DeleteCategory(
	ctx context.Context,
	in MenuServiceRequest,
	categoryID uuid.UUID,
) error {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceMenu,
		Action:   ActionDelete,
	}); err != nil {
		return err
	}

	category, err := s.GetCategory(ctx, in.TenantID, categoryID)
	if err != nil {
		return err
	}
	if category.RestaurantID != in.RestaurantID {
		return ErrMenuRestaurantMismatch
	}

	// Guard: block delete if any available items still exist in this category
	var activeCount int64
	if err := s.db.WithContext(ctx).Model(&models.MenuItem{}).
		Where("menu_category_id = ? AND is_available = true", categoryID).
		Count(&activeCount).Error; err != nil {
		return fmt.Errorf("check category items: %w", err)
	}
	if activeCount > 0 {
		return ErrCategoryHasItems
	}

	if err := s.db.WithContext(ctx).Delete(category).Error; err != nil {
		return fmt.Errorf("delete category: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventDelete, "MenuCategory", categoryID.String(),
		snapshotCategory(category), nil)

	s.invalidateCategoryCache(ctx, categoryID)
	s.invalidateFullMenuCache(ctx, in.RestaurantID)
	return nil
}

// ReorderCategories bulk-updates DisplayOrder to match the supplied slice order.
// Runs in a single transaction. RBAC: requires menu:update.
func (s *MenuService) ReorderCategories(
	ctx context.Context,
	in MenuServiceRequest,
	orderedCategoryIDs []uuid.UUID,
) error {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceMenu,
		Action:   ActionUpdate,
	}); err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, catID := range orderedCategoryIDs {
			order := i + 1
			res := tx.Model(&models.MenuCategory{}).
				Where("menu_category_id = ? AND tenant_id = ? AND restaurant_id = ?",
					catID, in.TenantID, in.RestaurantID).
				Update("display_order", order)
			if res.Error != nil {
				return fmt.Errorf("reorder category %s: %w", catID, res.Error)
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("category %s: %w", catID, ErrMenuCategoryNotFound)
			}
		}
		s.invalidateFullMenuCache(ctx, in.RestaurantID)
		return nil
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// CONCERN 2 — ITEM LIFECYCLE
// ─────────────────────────────────────────────────────────────────────────────

// CreateItem creates a new menu item under the given category.
// RBAC: requires menu_item:create.
func (s *MenuService) CreateItem(
	ctx context.Context,
	in CreateItemInput,
) (*models.MenuItem, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceMenuItem,
		Action:   ActionCreate,
	}); err != nil {
		return nil, err
	}

	// Verify the category exists and belongs to this tenant/restaurant
	category, err := s.GetCategory(ctx, in.TenantID, in.CategoryID)
	if err != nil {
		return nil, err
	}
	if category.RestaurantID != in.RestaurantID {
		return nil, ErrMenuRestaurantMismatch
	}

	dietaryFlags := models.JSONB(in.DietaryFlags)
	if dietaryFlags == nil {
		dietaryFlags = models.JSONB{}
	}

	item := &models.MenuItem{
		MenuItemID:             uuid.New(),
		MenuCategoryID:         in.CategoryID,
		TenantID:               in.TenantID,
		RestaurantID:           in.RestaurantID,
		Name:                   strings.TrimSpace(in.Name),
		Description:            strings.TrimSpace(in.Description),
		BasePrice:              in.BasePrice,
		IsAvailable:            true,
		PreparationTimeMinutes: in.PreparationTimeMinutes,
		ImageURL:               in.ImageURL,
		AllergenInfo:           in.AllergenInfo,
		DietaryFlags:           dietaryFlags,
	}

	if err := s.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, fmt.Errorf("create menu item: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventCreate, "MenuItem", item.MenuItemID.String(),
		nil, snapshotItem(item))

	s.invalidateItemCache(ctx, item.MenuItemID)
	s.invalidateCategoryCache(ctx, in.CategoryID)
	s.invalidateFullMenuCache(ctx, in.RestaurantID)

	return item, nil
}

// GetItem returns a single menu item, using Redis read-through.
func (s *MenuService) GetItem(
	ctx context.Context,
	tenantID, itemID uuid.UUID,
) (*models.MenuItem, error) {
	cacheKey := fmt.Sprintf("menu:item:%s", itemID)
	if cached, err := s.redis.Get(ctx, cacheKey).Bytes(); err == nil {
		var item models.MenuItem
		if json.Unmarshal(cached, &item) == nil {
			if item.TenantID != tenantID {
				return nil, ErrMenuTenantMismatch
			}
			return &item, nil
		}
	}

	var item models.MenuItem
	if err := s.db.WithContext(ctx).
		Where("menu_item_id = ? AND tenant_id = ?", itemID, tenantID).
		First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMenuItemNotFound
		}
		return nil, fmt.Errorf("get menu item: %w", err)
	}

	if b, err := json.Marshal(&item); err == nil {
		_ = s.redis.Set(ctx, cacheKey, b, menuItemCacheTTL).Err()
	}
	return &item, nil
}

// ListItems returns a paginated, filtered list of items for a restaurant.
func (s *MenuService) ListItems(
	ctx context.Context,
	f ListItemsFilter,
) ([]models.MenuItem, int64, error) {
	q := s.db.WithContext(ctx).
		Where("tenant_id = ? AND restaurant_id = ?", f.TenantID, f.RestaurantID)

	if f.CategoryID != nil {
		q = q.Where("menu_category_id = ?", *f.CategoryID)
	}
	if f.IsAvailable != nil {
		q = q.Where("is_available = ?", *f.IsAvailable)
	}
	if f.Search != nil && strings.TrimSpace(*f.Search) != "" {
		pattern := "%" + strings.ToLower(strings.TrimSpace(*f.Search)) + "%"
		q = q.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", pattern, pattern)
	}
	if f.MinPrice != nil {
		q = q.Where("base_price >= ?", *f.MinPrice)
	}
	if f.MaxPrice != nil {
		q = q.Where("base_price <= ?", *f.MaxPrice)
	}

	var total int64
	if err := q.Model(&models.MenuItem{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count items: %w", err)
	}

	sortCol := "created_at"
	switch f.SortBy {
	case "name":
		sortCol = "name"
	case "base_price":
		sortCol = "base_price"
	}
	sortDir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortDir = "DESC"
	}
	q = q.Order(fmt.Sprintf("%s %s", sortCol, sortDir))

	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	q = q.Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize)

	var items []models.MenuItem
	if err := q.Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list items: %w", err)
	}
	return items, total, nil
}

// UpdateItem applies a partial update to an existing menu item.
// RBAC: requires menu_item:update.
func (s *MenuService) UpdateItem(
	ctx context.Context,
	in UpdateItemInput,
) (*models.MenuItem, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceMenuItem,
		Action:   ActionUpdate,
	}); err != nil {
		return nil, err
	}

	item, err := s.GetItem(ctx, in.TenantID, in.ItemID)
	if err != nil {
		return nil, err
	}
	if item.RestaurantID != in.RestaurantID {
		return nil, ErrMenuRestaurantMismatch
	}

	old := snapshotItem(item)
	updates := map[string]interface{}{"updated_at": time.Now()}

	if in.CategoryID != nil {
		// Verify new category belongs to this restaurant
		newCat, err := s.GetCategory(ctx, in.TenantID, *in.CategoryID)
		if err != nil {
			return nil, err
		}
		if newCat.RestaurantID != in.RestaurantID {
			return nil, ErrMenuRestaurantMismatch
		}
		item.MenuCategoryID = *in.CategoryID
		updates["menu_category_id"] = *in.CategoryID
	}
	if in.Name != nil {
		item.Name = strings.TrimSpace(*in.Name)
		updates["name"] = item.Name
	}
	if in.Description != nil {
		item.Description = strings.TrimSpace(*in.Description)
		updates["description"] = item.Description
	}
	if in.BasePrice != nil {
		item.BasePrice = *in.BasePrice
		updates["base_price"] = *in.BasePrice
	}
	if in.IsAvailable != nil {
		item.IsAvailable = *in.IsAvailable
		updates["is_available"] = *in.IsAvailable
	}
	if in.PreparationTimeMinutes != nil {
		item.PreparationTimeMinutes = in.PreparationTimeMinutes
		updates["preparation_time_minutes"] = in.PreparationTimeMinutes
	}
	if in.ImageURL != nil {
		item.ImageURL = *in.ImageURL
		updates["image_url"] = *in.ImageURL
	}
	if in.AllergenInfo != nil {
		item.AllergenInfo = *in.AllergenInfo
		updates["allergen_info"] = *in.AllergenInfo
	}
	if in.DietaryFlags != nil {
		item.DietaryFlags = models.JSONB(in.DietaryFlags)
		updates["dietary_flags"] = item.DietaryFlags
	}

	if err := s.db.WithContext(ctx).
		Model(item).
		Clauses(clause.Returning{}).
		Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update menu item: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventUpdate, "MenuItem", in.ItemID.String(),
		old, snapshotItem(item))

	s.invalidateItemCache(ctx, in.ItemID)
	s.invalidatePriceCache(ctx, in.ItemID, in.RestaurantID)
	s.invalidateCategoryCache(ctx, item.MenuCategoryID)
	s.invalidateFullMenuCache(ctx, in.RestaurantID)

	return item, nil
}

// SetItemAvailability is a focused toggle — kitchen staff use this to 86 an
// item mid-service. Intentionally granular: does not require full update perms.
// RBAC: requires menu_item:update.
func (s *MenuService) SetItemAvailability(
	ctx context.Context,
	in MenuServiceRequest,
	itemID uuid.UUID,
	available bool,
) error {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceMenuItem,
		Action:   ActionUpdate,
	}); err != nil {
		return err
	}

	item, err := s.GetItem(ctx, in.TenantID, itemID)
	if err != nil {
		return err
	}
	if item.RestaurantID != in.RestaurantID {
		return ErrMenuRestaurantMismatch
	}

	if err := s.db.WithContext(ctx).
		Model(item).
		Updates(map[string]interface{}{
			"is_available": available,
			"updated_at":   time.Now(),
		}).Error; err != nil {
		return fmt.Errorf("set item availability: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventUpdate, "MenuItem", itemID.String(),
		map[string]interface{}{"is_available": !available},
		map[string]interface{}{"is_available": available})

	s.invalidateItemCache(ctx, itemID)
	s.invalidateCategoryCache(ctx, item.MenuCategoryID)
	s.invalidateFullMenuCache(ctx, in.RestaurantID)
	return nil
}

// DeleteItem hard-deletes an item after verifying it is not referenced by open orders.
// RBAC: requires menu_item:delete.
func (s *MenuService) DeleteItem(
	ctx context.Context,
	in MenuServiceRequest,
	itemID uuid.UUID,
) error {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceMenuItem,
		Action:   ActionDelete,
	}); err != nil {
		return err
	}

	item, err := s.GetItem(ctx, in.TenantID, itemID)
	if err != nil {
		return err
	}
	if item.RestaurantID != in.RestaurantID {
		return ErrMenuRestaurantMismatch
	}

	// Guard: block delete if item appears in any non-terminal order item.
	// Join order_items → orders to check live order_status.
	var openCount int64
	if err := s.db.WithContext(ctx).
		Table("order_items oi").
		Joins("JOIN orders o ON o.order_id = oi.order_id").
		Where("oi.menu_item_id = ? AND o.order_status NOT IN ('cancelled','served','refunded')", itemID).
		Count(&openCount).Error; err != nil {
		return fmt.Errorf("check open orders for item: %w", err)
	}
	if openCount > 0 {
		return ErrItemHasActiveOrders
	}

	if err := s.db.WithContext(ctx).Delete(item).Error; err != nil {
		return fmt.Errorf("delete menu item: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventDelete, "MenuItem", itemID.String(),
		snapshotItem(item), nil)

	s.invalidateItemCache(ctx, itemID)
	s.invalidatePriceCache(ctx, itemID, in.RestaurantID)
	s.invalidateCategoryCache(ctx, item.MenuCategoryID)
	s.invalidateFullMenuCache(ctx, in.RestaurantID)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// CONCERN 3 — PRICING ENGINE
// ─────────────────────────────────────────────────────────────────────────────

// CreatePricing adds a time-bounded price band for a menu item.
// Validates that the new band does not overlap any existing active band.
// RBAC: requires menu_item:update (price changes are privileged writes).
func (s *MenuService) CreatePricing(
	ctx context.Context,
	in CreatePricingInput,
) (*models.MenuItemPricing, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceMenuItem,
		Action:   ActionUpdate,
	}); err != nil {
		return nil, err
	}

	if in.EffectiveTo != nil && in.EffectiveTo.Before(in.EffectiveFrom) {
		return nil, ErrPriceEffectiveToBeforeFrom
	}

	item, err := s.GetItem(ctx, in.TenantID, in.ItemID)
	if err != nil {
		return nil, err
	}
	if item.RestaurantID != in.RestaurantID {
		return nil, ErrMenuRestaurantMismatch
	}

	if err := s.checkPriceOverlap(ctx, in.ItemID, in.RestaurantID,
		in.EffectiveFrom, in.EffectiveTo, uuid.Nil); err != nil {
		return nil, err
	}

	pricing := &models.MenuItemPricing{
		MenuItemPricingID: uuid.New(),
		MenuItemID:        in.ItemID,
		RestaurantID:      in.RestaurantID,
		Price:             in.Price,
		EffectiveFrom:     in.EffectiveFrom,
		EffectiveTo:       in.EffectiveTo,
		IsActive:          true,
	}

	if err := s.db.WithContext(ctx).Create(pricing).Error; err != nil {
		return nil, fmt.Errorf("create pricing: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventCreate, "MenuItemPricing", pricing.MenuItemPricingID.String(),
		nil, map[string]interface{}{
			"price":          pricing.Price,
			"effective_from": pricing.EffectiveFrom,
			"effective_to":   pricing.EffectiveTo,
		})

	s.invalidatePriceCache(ctx, in.ItemID, in.RestaurantID)
	s.invalidateFullMenuCache(ctx, in.RestaurantID)

	return pricing, nil
}

// GetActivePrice returns the currently-active price for a menu item at a given
// restaurant. Falls back to item.BasePrice if no active price band exists.
// Result is cached for menuPriceCacheTTL.
func (s *MenuService) GetActivePrice(
	ctx context.Context,
	itemID, restaurantID uuid.UUID,
) (float64, error) {
	cacheKey := fmt.Sprintf("menu:price:%s:%s", itemID, restaurantID)
	if raw, err := s.redis.Get(ctx, cacheKey).Float64(); err == nil {
		return raw, nil
	}

	now := time.Now()
	var pricing models.MenuItemPricing
	err := s.db.WithContext(ctx).
		Where(`menu_item_id = ? AND restaurant_id = ?
			AND is_active = true
			AND effective_from <= ?
			AND (effective_to IS NULL OR effective_to > ?)`,
			itemID, restaurantID, now, now).
		Order("effective_from DESC").
		First(&pricing).Error

	var price float64
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, fmt.Errorf("get active price: %w", err)
		}
		// No active band — use BasePrice
		var item models.MenuItem
		if err2 := s.db.WithContext(ctx).
			Select("base_price").
			Where("menu_item_id = ?", itemID).
			First(&item).Error; err2 != nil {
			return 0, fmt.Errorf("fallback to base price: %w", err2)
		}
		price = item.BasePrice
	} else {
		price = pricing.Price
	}

	_ = s.redis.Set(ctx, cacheKey, price, menuPriceCacheTTL).Err()
	return price, nil
}

// ListPricingHistory returns all price bands for an item, newest first.
func (s *MenuService) ListPricingHistory(
	ctx context.Context,
	tenantID, itemID, restaurantID uuid.UUID,
) ([]models.MenuItemPricing, error) {
	if _, err := s.GetItem(ctx, tenantID, itemID); err != nil {
		return nil, err
	}

	var bands []models.MenuItemPricing
	if err := s.db.WithContext(ctx).
		Where("menu_item_id = ? AND restaurant_id = ?", itemID, restaurantID).
		Order("effective_from DESC").
		Find(&bands).Error; err != nil {
		return nil, fmt.Errorf("list pricing history: %w", err)
	}
	return bands, nil
}

// checkPriceOverlap returns ErrPriceOverlap when a conflicting band exists.
// excludeID skips a specific record — used in update flows.
func (s *MenuService) checkPriceOverlap(
	ctx context.Context,
	itemID, restaurantID uuid.UUID,
	from time.Time,
	to *time.Time,
	excludeID uuid.UUID,
) error {
	q := s.db.WithContext(ctx).Model(&models.MenuItemPricing{}).
		Where("menu_item_id = ? AND restaurant_id = ? AND is_active = true",
			itemID, restaurantID)

	if excludeID != uuid.Nil {
		q = q.Where("menu_item_pricing_id != ?", excludeID)
	}

	if to == nil {
		// New band is open-ended: conflicts with any active band ending after `from`
		q = q.Where("effective_to IS NULL OR effective_to > ?", from)
	} else {
		q = q.Where(
			"effective_from < ? AND (effective_to IS NULL OR effective_to > ?)",
			*to, from,
		)
	}

	var count int64
	if err := q.Count(&count).Error; err != nil {
		return fmt.Errorf("overlap check: %w", err)
	}
	if count > 0 {
		return ErrPriceOverlap
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// MODIFIER LIFECYCLE
// ─────────────────────────────────────────────────────────────────────────────

// CreateModifier adds a modifier option to an existing menu item.
// RBAC: requires menu_item:update.
func (s *MenuService) CreateModifier(
	ctx context.Context,
	in CreateModifierInput,
) (*models.MenuItemModifier, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceMenuItem,
		Action:   ActionUpdate,
	}); err != nil {
		return nil, err
	}

	item, err := s.GetItem(ctx, in.TenantID, in.ItemID)
	if err != nil {
		return nil, err
	}
	if item.RestaurantID != in.RestaurantID {
		return nil, ErrMenuRestaurantMismatch
	}

	mod := &models.MenuItemModifier{
		MenuItemModifierID: uuid.New(),
		MenuItemID:         in.ItemID,
		Name:               strings.TrimSpace(in.Name),
		PriceAdjustment:    in.PriceAdjustment,
		IsAvailable:        true,
	}

	if err := s.db.WithContext(ctx).Create(mod).Error; err != nil {
		return nil, fmt.Errorf("create modifier: %w", err)
	}

	s.invalidateItemCache(ctx, in.ItemID)
	s.invalidateFullMenuCache(ctx, in.RestaurantID)
	return mod, nil
}

// UpdateModifier patches an existing modifier.
// RBAC: requires menu_item:update.
func (s *MenuService) UpdateModifier(
	ctx context.Context,
	in UpdateModifierInput,
) (*models.MenuItemModifier, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourceMenuItem,
		Action:   ActionUpdate,
	}); err != nil {
		return nil, err
	}

	// Fetch modifier and verify tenant ownership via JOIN
	var mod models.MenuItemModifier
	if err := s.db.WithContext(ctx).
		Joins("JOIN menu_items mi ON mi.menu_item_id = menu_item_modifiers.menu_item_id").
		Where("menu_item_modifiers.menu_item_modifier_id = ? AND mi.tenant_id = ? AND mi.restaurant_id = ?",
			in.ModifierID, in.TenantID, in.RestaurantID).
		First(&mod).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMenuModifierNotFound
		}
		return nil, fmt.Errorf("get modifier: %w", err)
	}

	updates := map[string]interface{}{}
	if in.Name != nil {
		mod.Name = strings.TrimSpace(*in.Name)
		updates["name"] = mod.Name
	}
	if in.PriceAdjustment != nil {
		mod.PriceAdjustment = *in.PriceAdjustment
		updates["price_adjustment"] = *in.PriceAdjustment
	}
	if in.IsAvailable != nil {
		mod.IsAvailable = *in.IsAvailable
		updates["is_available"] = *in.IsAvailable
	}

	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(&mod).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("update modifier: %w", err)
		}
	}

	s.invalidateItemCache(ctx, mod.MenuItemID)
	s.invalidateFullMenuCache(ctx, in.RestaurantID)
	return &mod, nil
}

// ListModifiers returns all modifiers for a menu item.
func (s *MenuService) ListModifiers(
	ctx context.Context,
	tenantID, itemID uuid.UUID,
) ([]models.MenuItemModifier, error) {
	if _, err := s.GetItem(ctx, tenantID, itemID); err != nil {
		return nil, err
	}

	var mods []models.MenuItemModifier
	if err := s.db.WithContext(ctx).
		Where("menu_item_id = ?", itemID).
		Order("name ASC").
		Find(&mods).Error; err != nil {
		return nil, fmt.Errorf("list modifiers: %w", err)
	}
	return mods, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// CONCERN 4 — READ PATH: Full Menu Assembly
// ─────────────────────────────────────────────────────────────────────────────

// FullMenuAssembly is the service-layer view of a complete menu.
// Callers convert this to their DTO as needed.
type FullMenuAssembly struct {
	RestaurantID uuid.UUID          `json:"restaurant_id"`
	Categories   []CategoryAssembly `json:"categories"`
	TotalItems   int                `json:"total_items"`
	GeneratedAt  time.Time          `json:"generated_at"`
}

// CategoryAssembly holds a category and its hydrated items.
type CategoryAssembly struct {
	models.MenuCategory
	Items     []ItemAssembly `json:"items"`
	ItemCount int            `json:"item_count"`
}

// ItemAssembly holds a menu item, its active price, and its modifiers.
type ItemAssembly struct {
	models.MenuItem
	ActivePrice float64                   `json:"active_price"`
	Modifiers   []models.MenuItemModifier `json:"modifiers"`
}

// GetFullMenu returns the complete menu for a restaurant with active prices
// injected per item. Result is cached for menuFullCacheTTL.
// RBAC: requires menu:read.
func (s *MenuService) GetFullMenu(
	ctx context.Context,
	tenantID, restaurantID, actorID uuid.UUID,
) (*FullMenuAssembly, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   actorID,
		TenantID: tenantID,
		Resource: ResourceMenu,
		Action:   ActionRead,
	}); err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("menu:full:%s", restaurantID)
	if cached, err := s.redis.Get(ctx, cacheKey).Bytes(); err == nil {
		var menu FullMenuAssembly
		if json.Unmarshal(cached, &menu) == nil {
			return &menu, nil
		}
	}

	menu, err := s.assembleFullMenu(ctx, tenantID, restaurantID)
	if err != nil {
		return nil, err
	}

	if b, err := json.Marshal(menu); err == nil {
		_ = s.redis.Set(ctx, cacheKey, b, menuFullCacheTTL).Err()
	}
	return menu, nil
}

// assembleFullMenu fetches categories, items, modifiers, and active prices
// in three queries (no N+1).
func (s *MenuService) assembleFullMenu(
	ctx context.Context,
	tenantID, restaurantID uuid.UUID,
) (*FullMenuAssembly, error) {
	categories, err := s.ListCategories(ctx, tenantID, restaurantID, true)
	if err != nil {
		return nil, err
	}

	// Query 1: all available items for this restaurant
	var allItems []models.MenuItem
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND restaurant_id = ? AND is_available = true",
			tenantID, restaurantID).
		Order("name ASC").
		Find(&allItems).Error; err != nil {
		return nil, fmt.Errorf("load menu items: %w", err)
	}

	if len(allItems) == 0 {
		return &FullMenuAssembly{
			RestaurantID: restaurantID,
			Categories:   buildEmptyCategories(categories),
			TotalItems:   0,
			GeneratedAt:  time.Now(),
		}, nil
	}

	itemIDs := make([]uuid.UUID, len(allItems))
	for i, item := range allItems {
		itemIDs[i] = item.MenuItemID
	}

	// Query 2: all available modifiers for those items
	var allMods []models.MenuItemModifier
	if err := s.db.WithContext(ctx).
		Where("menu_item_id IN ? AND is_available = true", itemIDs).
		Find(&allMods).Error; err != nil {
		return nil, fmt.Errorf("load modifiers: %w", err)
	}

	// Query 3: batch-fetch active prices (DISTINCT ON per item)
	activePrices, err := s.batchGetActivePrices(ctx, itemIDs, restaurantID)
	if err != nil {
		activePrices = map[uuid.UUID]float64{} // non-fatal; fall back to BasePrice
	}

	// Index modifiers by item
	modsByItem := make(map[uuid.UUID][]models.MenuItemModifier, len(allItems))
	for _, mod := range allMods {
		modsByItem[mod.MenuItemID] = append(modsByItem[mod.MenuItemID], mod)
	}

	// Index items by category
	itemsByCategory := make(map[uuid.UUID][]ItemAssembly, len(categories))
	for _, item := range allItems {
		activePrice, ok := activePrices[item.MenuItemID]
		if !ok {
			activePrice = item.BasePrice
		}
		itemsByCategory[item.MenuCategoryID] = append(
			itemsByCategory[item.MenuCategoryID],
			ItemAssembly{
				MenuItem:    item,
				ActivePrice: activePrice,
				Modifiers:   modsByItem[item.MenuItemID],
			},
		)
	}

	assembled := &FullMenuAssembly{
		RestaurantID: restaurantID,
		GeneratedAt:  time.Now(),
	}
	for _, cat := range categories {
		items := itemsByCategory[cat.MenuCategoryID]
		if items == nil {
			items = []ItemAssembly{}
		}
		assembled.Categories = append(assembled.Categories, CategoryAssembly{
			MenuCategory: cat,
			Items:        items,
			ItemCount:    len(items),
		})
		assembled.TotalItems += len(items)
	}

	return assembled, nil
}

// batchGetActivePrices fetches the currently-active price for many items in
// one DISTINCT ON query. Items without an active band are absent from the map.
func (s *MenuService) batchGetActivePrices(
	ctx context.Context,
	itemIDs []uuid.UUID,
	restaurantID uuid.UUID,
) (map[uuid.UUID]float64, error) {
	if len(itemIDs) == 0 {
		return map[uuid.UUID]float64{}, nil
	}

	now := time.Now()
	type priceRow struct {
		MenuItemID uuid.UUID
		Price      float64
	}
	var rows []priceRow

	// DISTINCT ON (menu_item_id) picks the most recent effective band per item.
	if err := s.db.WithContext(ctx).
		Table("menu_item_pricings").
		Select("DISTINCT ON (menu_item_id) menu_item_id, price").
		Where(`menu_item_id IN ?
			AND restaurant_id = ?
			AND is_active = true
			AND effective_from <= ?
			AND (effective_to IS NULL OR effective_to > ?)`,
			itemIDs, restaurantID, now, now).
		Order("menu_item_id, effective_from DESC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("batch price query: %w", err)
	}

	result := make(map[uuid.UUID]float64, len(rows))
	for _, r := range rows {
		result[r.MenuItemID] = r.Price
	}
	return result, nil
}

// buildEmptyCategories creates CategoryAssembly values with no items.
func buildEmptyCategories(cats []models.MenuCategory) []CategoryAssembly {
	out := make([]CategoryAssembly, len(cats))
	for i, c := range cats {
		out[i] = CategoryAssembly{MenuCategory: c, Items: []ItemAssembly{}}
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// CACHE INVALIDATION
// ─────────────────────────────────────────────────────────────────────────────

func (s *MenuService) invalidateFullMenuCache(ctx context.Context, restaurantID uuid.UUID) {
	_ = s.redis.Del(ctx, fmt.Sprintf("menu:full:%s", restaurantID)).Err()
}

func (s *MenuService) invalidateCategoryCache(ctx context.Context, categoryID uuid.UUID) {
	_ = s.redis.Del(ctx, fmt.Sprintf("menu:category:%s", categoryID)).Err()
}

func (s *MenuService) invalidateItemCache(ctx context.Context, itemID uuid.UUID) {
	_ = s.redis.Del(ctx, fmt.Sprintf("menu:item:%s", itemID)).Err()
}

func (s *MenuService) invalidatePriceCache(ctx context.Context, itemID, restaurantID uuid.UUID) {
	_ = s.redis.Del(ctx, fmt.Sprintf("menu:price:%s:%s", itemID, restaurantID)).Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// AUDIT TRAIL
// ─────────────────────────────────────────────────────────────────────────────

// writeAudit persists an AuditTrail row.
// Errors are intentionally silenced: audit failures must never block the caller.
func (s *MenuService) writeAudit(
	ctx context.Context,
	tenantID uuid.UUID,
	restaurantID *uuid.UUID,
	actorID uuid.UUID,
	event models.AuditEvent,
	entityType, entityID string,
	oldValues, newValues interface{},
) {
	entry := &models.AuditTrail{
		AuditTrailID:     uuid.New(),
		TenantID:         tenantID,
		UserID:           &actorID,
		RestaurantID:     restaurantID,
		EventType:        event,
		EventCategory:    "menu",
		EventDescription: fmt.Sprintf("%s on %s %s", event, entityType, entityID),
		Severity:         models.AuditSeverityInfo,
		EntityType:       entityType,
		EntityID:         entityID,
		OldValues:        mustMarshalJSONB(oldValues),
		NewValues:        mustMarshalJSONB(newValues),
		RiskLevel:        models.RiskLevelLow,
		Timestamp:        time.Now(),
	}
	_ = s.db.WithContext(ctx).Create(entry).Error
}

// ─────────────────────────────────────────────────────────────────────────────
// SNAPSHOT HELPERS (used to build audit diff old/new values)
// ─────────────────────────────────────────────────────────────────────────────

func snapshotCategory(c *models.MenuCategory) map[string]interface{} {
	return map[string]interface{}{
		"name":          c.Name,
		"description":   c.Description,
		"display_order": c.DisplayOrder,
		"is_active":     c.IsActive,
	}
}

func snapshotItem(i *models.MenuItem) map[string]interface{} {
	return map[string]interface{}{
		"name":                     i.Name,
		"description":              i.Description,
		"base_price":               i.BasePrice,
		"is_available":             i.IsAvailable,
		"menu_category_id":         i.MenuCategoryID,
		"preparation_time_minutes": i.PreparationTimeMinutes,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PAGINATION HELPER
// ─────────────────────────────────────────────────────────────────────────────

// MenuTotalPages returns the number of pages for a given total and page size.
func MenuTotalPages(total int64, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}
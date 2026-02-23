package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ===================================
// MENU CATEGORY DTOs
// ===================================

// CreateMenuCategoryRequest
type CreateMenuCategoryRequest struct {
	RestaurantID uuid.UUID `json:"restaurant_id" binding:"required"`
	Name         string    `json:"name" binding:"required,min=2,max=255"`
	Description  string    `json:"description" binding:"omitempty,max=1000"`
	DisplayOrder *int      `json:"display_order" binding:"omitempty,min=0"`
}

// UpdateMenuCategoryRequest
type UpdateMenuCategoryRequest struct {
	Name         *string `json:"name" binding:"omitempty,min=2,max=255"`
	Description  *string `json:"description" binding:"omitempty,max=1000"`
	DisplayOrder *int    `json:"display_order" binding:"omitempty,min=0"`
	IsActive     *bool   `json:"is_active"`
}

// MenuCategoryResponse
type MenuCategoryResponse struct {
	MenuCategoryID uuid.UUID          `json:"menu_category_id"`
	TenantID       uuid.UUID          `json:"tenant_id"`
	RestaurantID   uuid.UUID          `json:"restaurant_id"`
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	DisplayOrder   *int               `json:"display_order,omitempty"`
	IsActive       bool               `json:"is_active"`
	ItemCount      int                `json:"item_count"` // Computed
	Items          []MenuItemResponse `json:"items,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// MenuCategoryListResponse
type MenuCategoryListResponse struct {
	Categories []MenuCategoryResponse `json:"categories"`
	Total      int64                  `json:"total"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"page_size"`
	TotalPages int                    `json:"total_pages"`
}

// MenuCategorySummaryDTO - used in other responses
type MenuCategorySummaryDTO struct {
	MenuCategoryID uuid.UUID `json:"menu_category_id"`
	Name           string    `json:"name"`
	DisplayOrder   *int      `json:"display_order,omitempty"`
}

// ===================================
// MENU ITEM DTOs
// ===================================

// CreateMenuItemRequest
type CreateMenuItemRequest struct {
	MenuCategoryID         uuid.UUID `json:"menu_category_id" binding:"required"`
	RestaurantID           uuid.UUID `json:"restaurant_id" binding:"required"`
	Name                   string    `json:"name" binding:"required,min=2,max=255"`
	Description            string    `json:"description" binding:"omitempty,max=1000"`
	BasePrice              float64   `json:"base_price" binding:"required,min=0"`
	PreparationTimeMinutes *int      `json:"preparation_time_minutes" binding:"omitempty,min=0"`
	ImageURL               string    `json:"image_url" binding:"omitempty,max=2000"`
	AllergenInfo           string    `json:"allergen_info" binding:"omitempty,max=1000"`
}

func (r *CreateMenuItemRequest) Validate() error {
	if r.BasePrice < 0 {
		return ErrNegativeBasePrice
	}
	if r.PreparationTimeMinutes != nil && *r.PreparationTimeMinutes < 0 {
		return ErrNegativePreparationTime
	}
	return nil
}

// UpdateMenuItemRequest
type UpdateMenuItemRequest struct {
	MenuCategoryID         *uuid.UUID `json:"menu_category_id"`
	Name                   *string    `json:"name" binding:"omitempty,min=2,max=255"`
	Description            *string    `json:"description" binding:"omitempty,max=1000"`
	BasePrice              *float64   `json:"base_price" binding:"omitempty,min=0"`
	IsAvailable            *bool      `json:"is_available"`
	PreparationTimeMinutes *int       `json:"preparation_time_minutes" binding:"omitempty,min=0"`
	ImageURL               *string    `json:"image_url" binding:"omitempty,max=2000"`
	AllergenInfo           *string    `json:"allergen_info" binding:"omitempty,max=1000"`
}

func (r *UpdateMenuItemRequest) Validate() error {
	if r.BasePrice != nil && *r.BasePrice < 0 {
		return ErrNegativeBasePrice
	}
	if r.PreparationTimeMinutes != nil && *r.PreparationTimeMinutes < 0 {
		return ErrNegativePreparationTime
	}
	return nil
}

// MenuItemResponse
type MenuItemResponse struct {
	MenuItemID             uuid.UUID              `json:"menu_item_id"`
	MenuCategoryID         uuid.UUID              `json:"menu_category_id"`
	TenantID               uuid.UUID              `json:"tenant_id"`
	RestaurantID           uuid.UUID              `json:"restaurant_id"`
	Name                   string                 `json:"name"`
	Description            string                 `json:"description"`
	BasePrice              float64                `json:"base_price"`
	IsAvailable            bool                   `json:"is_available"`
	PreparationTimeMinutes *int                   `json:"preparation_time_minutes,omitempty"`
	ImageURL               string                 `json:"image_url,omitempty"`
	AllergenInfo           string                 `json:"allergen_info,omitempty"`
	Category               *MenuCategorySummaryDTO `json:"category,omitempty"`
	Modifiers              []MenuItemModifierResponse `json:"modifiers,omitempty"`
	CurrentPrice           *float64               `json:"current_price,omitempty"` // From active pricing
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
}

// MenuItemListResponse
type MenuItemListResponse struct {
	Items      []MenuItemResponse `json:"items"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	TotalPages int                `json:"total_pages"`
}

// MenuItemFilterRequest
type MenuItemFilterRequest struct {
	RestaurantID   *uuid.UUID `form:"restaurant_id"`
	CategoryID     *uuid.UUID `form:"category_id"`
	IsAvailable    *bool      `form:"is_available"`
	Search         *string    `form:"search"` // Search by name, description
	MinPrice       *float64   `form:"min_price"`
	MaxPrice       *float64   `form:"max_price"`
	Page           int        `form:"page" binding:"min=1"`
	PageSize       int        `form:"page_size" binding:"min=1,max=100"`
	SortBy         string     `form:"sort_by" binding:"omitempty,oneof=name base_price created_at"`
	SortOrder      string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

func (r *MenuItemFilterRequest) Validate() error {
	if r.MinPrice != nil && r.MaxPrice != nil && *r.MinPrice > *r.MaxPrice {
		return ErrMinPriceGreaterThanMax
	}
	return nil
}

// MenuItemSummaryDTO - used in KOT, order responses
type MenuItemSummaryDTO struct {
	MenuItemID uuid.UUID `json:"menu_item_id"`
	Name       string    `json:"name"`
	BasePrice  float64   `json:"base_price"`
}

// ===================================
// MENU ITEM MODIFIER DTOs
// ===================================

// CreateMenuItemModifierRequest
type CreateMenuItemModifierRequest struct {
	MenuItemID      uuid.UUID `json:"menu_item_id" binding:"required"`
	Name            string    `json:"name" binding:"required,min=2,max=255"`
	PriceAdjustment float64   `json:"price_adjustment" binding:"required"` // Can be 0, positive, or negative
}

func (r *CreateMenuItemModifierRequest) Validate() error {
	if r.PriceAdjustment < -9999.99 || r.PriceAdjustment > 9999.99 {
		return ErrInvalidPriceAdjustment
	}
	return nil
}

// UpdateMenuItemModifierRequest
type UpdateMenuItemModifierRequest struct {
	Name            *string  `json:"name" binding:"omitempty,min=2,max=255"`
	PriceAdjustment *float64 `json:"price_adjustment"`
	IsAvailable     *bool    `json:"is_available"`
}

// MenuItemModifierResponse
type MenuItemModifierResponse struct {
	MenuItemModifierID uuid.UUID `json:"menu_item_modifier_id"`
	MenuItemID         uuid.UUID `json:"menu_item_id"`
	Name               string    `json:"name"`
	PriceAdjustment    float64   `json:"price_adjustment"`
	IsAvailable        bool      `json:"is_available"`
	CreatedAt          time.Time `json:"created_at"`
}

// ===================================
// MENU ITEM PRICING DTOs
// ===================================

// CreateMenuItemPricingRequest
type CreateMenuItemPricingRequest struct {
	MenuItemID    uuid.UUID  `json:"menu_item_id" binding:"required"`
	RestaurantID  uuid.UUID  `json:"restaurant_id" binding:"required"`
	Price         float64    `json:"price" binding:"required,min=0"`
	EffectiveFrom time.Time  `json:"effective_from" binding:"required"`
	EffectiveTo   *time.Time `json:"effective_to,omitempty"`
}

func (r *CreateMenuItemPricingRequest) Validate() error {
	if r.Price < 0 {
		return ErrNegativeBasePrice
	}
	if r.EffectiveTo != nil && r.EffectiveTo.Before(r.EffectiveFrom) {
		return ErrEffectiveToBeforeFrom
	}
	return nil
}

// MenuItemPricingResponse
type MenuItemPricingResponse struct {
	MenuItemPricingID uuid.UUID  `json:"menu_item_pricing_id"`
	MenuItemID        uuid.UUID  `json:"menu_item_id"`
	RestaurantID      uuid.UUID  `json:"restaurant_id"`
	Price             float64    `json:"price"`
	EffectiveFrom     time.Time  `json:"effective_from"`
	EffectiveTo       *time.Time `json:"effective_to,omitempty"`
	IsActive          bool       `json:"is_active"`
	CreatedAt         time.Time  `json:"created_at"`
}

// ===================================
// FULL MENU RESPONSE (All categories + items)
// ===================================

// FullMenuResponse - complete menu for a restaurant
type FullMenuResponse struct {
	RestaurantID uuid.UUID              `json:"restaurant_id"`
	Categories   []MenuCategoryResponse `json:"categories"`
	TotalItems   int                    `json:"total_items"`
}

// ===================================
// VALIDATION ERRORS
// ===================================

var (
	ErrNegativeBasePrice        = NewValidationError("base price cannot be negative")
	ErrNegativePreparationTime  = NewValidationError("preparation time cannot be negative")
	ErrMinPriceGreaterThanMax   = NewValidationError("min price cannot be greater than max price")
	ErrInvalidPriceAdjustment   = NewValidationError("price adjustment is out of valid range")
	ErrEffectiveToBeforeFrom    = NewValidationError("effective_to cannot be before effective_from")
)
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MenuCategory represents a category of menu items
type MenuCategory struct {
	MenuCategoryID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"menu_category_id"`
	TenantID       uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID   uuid.UUID `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	Name           string    `gorm:"type:varchar(255);not null" json:"name"`
	Description    string    `gorm:"type:text" json:"description"`
	DisplayOrder   *int      `gorm:"type:int" json:"display_order"`
	IsActive       bool      `gorm:"not null;default:true;index" json:"is_active"`
	CreatedAt      time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;default:now()" json:"updated_at"`

	// Relationships
	Tenant     Tenant     `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
}

// MenuItem represents a menu item
type MenuItem struct {
	MenuItemID            uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"menu_item_id"`
	MenuCategoryID        uuid.UUID `gorm:"type:uuid;not null;index" json:"menu_category_id"`
	TenantID              uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID          uuid.UUID `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	Name                  string    `gorm:"type:varchar(255);not null" json:"name"`
	Description           string    `gorm:"type:text" json:"description"`
	BasePrice             float64   `gorm:"type:decimal(10,2);not null" json:"base_price"`
	IsAvailable           bool      `gorm:"not null;default:true;index" json:"is_available"`
	PreparationTimeMinutes *int     `gorm:"type:int" json:"preparation_time_minutes"`
	ImageURL              string    `gorm:"type:text" json:"image_url"`
	AllergenInfo          string    `gorm:"type:text" json:"allergen_info"`
	DietaryFlags          JSONB     `gorm:"type:jsonb;default:'{}'" json:"dietary_flags"`
	CreatedAt             time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt             time.Time `gorm:"not null;default:now()" json:"updated_at"`

	// Relationships
	MenuCategory MenuCategory `gorm:"foreignKey:MenuCategoryID;constraint:OnDelete:CASCADE" json:"-"`
	Tenant       Tenant       `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant   Restaurant   `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
}

// MenuItemModifier represents a modifier for a menu item
type MenuItemModifier struct {
	MenuItemModifierID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"menu_item_modifier_id"`
	MenuItemID         uuid.UUID `gorm:"type:uuid;not null;index" json:"menu_item_id"`
	Name               string    `gorm:"type:varchar(255);not null" json:"name"`
	PriceAdjustment    float64   `gorm:"type:decimal(10,2);not null;default:0" json:"price_adjustment"`
	IsAvailable        bool      `gorm:"not null;default:true" json:"is_available"`
	CreatedAt          time.Time `gorm:"not null;default:now()" json:"created_at"`

	// Relationships
	MenuItem MenuItem `gorm:"foreignKey:MenuItemID;constraint:OnDelete:CASCADE" json:"-"`
}

// MenuItemPricing represents pricing for a menu item
type MenuItemPricing struct {
	MenuItemPricingID uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"menu_item_pricing_id"`
	MenuItemID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"menu_item_id"`
	RestaurantID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	Price             float64    `gorm:"type:decimal(10,2);not null" json:"price"`
	EffectiveFrom     time.Time  `gorm:"not null;index" json:"effective_from"`
	EffectiveTo       *time.Time `gorm:"type:timestamptz" json:"effective_to"`
	IsActive          bool       `gorm:"not null;default:true" json:"is_active"`
	CreatedAt         time.Time  `gorm:"not null;default:now()" json:"created_at"`

	// Relationships
	MenuItem   MenuItem   `gorm:"foreignKey:MenuItemID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
}

// BeforeCreate hooks
func (mc *MenuCategory) BeforeCreate(tx *gorm.DB) error {
	if mc.MenuCategoryID == uuid.Nil {
		mc.MenuCategoryID = uuid.New()
	}
	return nil
}

func (mi *MenuItem) BeforeCreate(tx *gorm.DB) error {
	if mi.MenuItemID == uuid.Nil {
		mi.MenuItemID = uuid.New()
	}
	return nil
}

func (mim *MenuItemModifier) BeforeCreate(tx *gorm.DB) error {
	if mim.MenuItemModifierID == uuid.Nil {
		mim.MenuItemModifierID = uuid.New()
	}
	return nil
}

func (mip *MenuItemPricing) BeforeCreate(tx *gorm.DB) error {
	if mip.MenuItemPricingID == uuid.Nil {
		mip.MenuItemPricingID = uuid.New()
	}
	return nil
}
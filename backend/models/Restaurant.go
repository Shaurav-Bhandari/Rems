	package models

	import (
		"database/sql/driver"
		"encoding/json"
		"time"

		"github.com/google/uuid"
		"gorm.io/gorm"
	)

	// Restaurant represents a restaurant location
	type Restaurant struct {
		RestaurantID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"restaurant_id"`
		TenantID     uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
		Name         string    `gorm:"type:varchar(255);not null" json:"name"`
		Address      string    `gorm:"type:text" json:"address"`
		City         string    `gorm:"type:varchar(100)" json:"city"`
		State        string    `gorm:"type:varchar(100)" json:"state"`
		Country      string    `gorm:"type:varchar(100)" json:"country"`
		PostalCode   string    `gorm:"type:varchar(20)" json:"postal_code"`
		Phone        string    `gorm:"type:varchar(50)" json:"phone"`
		Email        string    `gorm:"type:varchar(255)" json:"email"`
		IsActive     bool      `gorm:"not null;default:true;index" json:"is_active"`
		CreatedAt    time.Time `gorm:"not null;default:now()" json:"created_at"`
		UpdatedAt    time.Time `gorm:"not null;default:now()" json:"updated_at"`

		// Relationships
		Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	}

	// Region represents a geographical region
	type Region struct {
		RegionID       uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"region_id"`
		TenantID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
		Name           string     `gorm:"type:varchar(255);not null" json:"name"`
		Code           string     `gorm:"type:varchar(50)" json:"code"`
		ParentRegionID *uuid.UUID `gorm:"type:uuid;index" json:"parent_region_id"`
		Level          *int       `gorm:"type:int" json:"level"`
		IsActive       bool       `gorm:"not null;default:true" json:"is_active"`
		CreatedAt      time.Time  `gorm:"not null;default:now()" json:"created_at"`

		// Relationships
		Tenant       Tenant  `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
		ParentRegion *Region `gorm:"foreignKey:ParentRegionID;constraint:OnDelete:SET NULL" json:"parent_region,omitempty"`
	}

	// JSONB type for PostgreSQL JSONB columns
	type JSONB map[string]interface{}

	func (j JSONB) Value() (driver.Value, error) {
		return json.Marshal(j)
	}

	func (j *JSONB) Scan(value interface{}) error {
		if value == nil {
			*j = make(map[string]interface{})
			return nil
		}
		bytes, ok := value.([]byte)
		if !ok {
			return nil
		}
		return json.Unmarshal(bytes, j)
	}

	// RegionalSetting represents regional settings for a restaurant or region
	type RegionalSetting struct {
		RegionalSettingID uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"regional_setting_id"`
		TenantID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
		RegionID          *uuid.UUID `gorm:"type:uuid;index" json:"region_id"`
		RestaurantID      *uuid.UUID `gorm:"type:uuid;index" json:"restaurant_id"`
		Timezone          string     `gorm:"type:varchar(100)" json:"timezone"`
		CurrencyCode      string     `gorm:"type:varchar(10)" json:"currency_code"`
		DateFormat        string     `gorm:"type:varchar(50)" json:"date_format"`
		TimeFormat        string     `gorm:"type:varchar(50)" json:"time_format"`
		LanguageCode      string     `gorm:"type:varchar(10)" json:"language_code"`
		TaxRate           *float64   `gorm:"type:decimal(5,2)" json:"tax_rate"`
		CreatedAt         time.Time  `gorm:"not null;default:now()" json:"created_at"`
		UpdatedAt         time.Time  `gorm:"not null;default:now()" json:"updated_at"`

		// Relationships
		Tenant     Tenant      `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
		Region     *Region     `gorm:"foreignKey:RegionID;constraint:OnDelete:CASCADE" json:"region,omitempty"`
		Restaurant *Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"restaurant,omitempty"`
	}

	// Floor represents a floor in a restaurant
	type Floor struct {
		FloorID      int       `gorm:"primaryKey;autoIncrement" json:"floor_id"`
		RestaurantID uuid.UUID `gorm:"type:uuid;not null;index" json:"restaurant_id"`
		Name         string    `gorm:"type:varchar(255)" json:"name"`
		FloorNumber  int       `gorm:"not null" json:"floor_number"`
		TableCount   int       `gorm:"not null;default:0" json:"table_count"`
		CreatedAt    time.Time `gorm:"not null;default:now()" json:"created_at"`

		// Relationships
		Restaurant Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
	}

	// Table represents a dining table
	type Table struct {
		TableID          int          `gorm:"primaryKey;autoIncrement" json:"table_id"`
		RestaurantID     uuid.UUID    `gorm:"type:uuid;not null;index;uniqueIndex:idx_restaurant_table_number" json:"restaurant_id"`
		FloorID          int          `gorm:"not null;index" json:"floor_id"`
		TableNumber      string       `gorm:"type:varchar(50);not null;uniqueIndex:idx_restaurant_table_number" json:"table_number"`
		Capacity         int          `gorm:"not null" json:"capacity"`
		CurrentOccupancy int          `gorm:"not null;default:0" json:"current_occupancy"`
		Status           TableStatus  `gorm:"type:varchar(50);not null;default:'available';index" json:"status"`
		CreatedAt        time.Time    `gorm:"default:now()" json:"created_at"`
		LastReservedAt   *time.Time   `gorm:"type:timestamptz" json:"last_reserved_at"`
		LastCleanedAt    *time.Time   `gorm:"type:timestamptz" json:"last_cleaned_at"`
		UpdatedAt        *time.Time   `gorm:"type:timestamptz" json:"updated_at"`
		CreatedBy        *uuid.UUID   `gorm:"type:uuid" json:"created_by"`
		UpdatedBy        *uuid.UUID   `gorm:"type:uuid" json:"updated_by"`

		// Relationships
		Restaurant Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
		Floor      Floor      `gorm:"foreignKey:FloorID;constraint:OnDelete:CASCADE" json:"-"`
		CreatedByUser *User   `gorm:"foreignKey:CreatedBy;constraint:OnDelete:SET NULL" json:"created_by_user,omitempty"`
		UpdatedByUser *User   `gorm:"foreignKey:UpdatedBy;constraint:OnDelete:SET NULL" json:"updated_by_user,omitempty"`
	}

	// BeforeCreate hooks
	func (r *Restaurant) BeforeCreate(tx *gorm.DB) error {
		if r.RestaurantID == uuid.Nil {
			r.RestaurantID = uuid.New()
		}
		return nil
	}

	func (reg *Region) BeforeCreate(tx *gorm.DB) error {
		if reg.RegionID == uuid.Nil {
			reg.RegionID = uuid.New()
		}
		return nil
	}

	func (rs *RegionalSetting) BeforeCreate(tx *gorm.DB) error {
		if rs.RegionalSettingID == uuid.Nil {
			rs.RegionalSettingID = uuid.New()
		}
		return nil
	}
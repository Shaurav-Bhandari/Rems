package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DemandForecast represents a demand forecast
type DemandForecast struct {
	ForecastID      uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"forecast_id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	ForecastDate    time.Time `gorm:"not null;index" json:"forecast_date"`
	ForecastType    string    `gorm:"type:varchar(100);not null" json:"forecast_type"`
	PredictedValue  float64   `gorm:"type:decimal(12,2);not null" json:"predicted_value"`
	ConfidenceLevel *float64  `gorm:"type:decimal(5,2)" json:"confidence_level"`
	CreatedAt       time.Time `gorm:"not null;default:now()" json:"created_at"`

	// Relationships
	Tenant     Tenant     `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
}

// SalesForecast represents a sales forecast
type SalesForecast struct {
	SalesForecastID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"sales_forecast_id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	ForecastDate    time.Time `gorm:"not null;index" json:"forecast_date"`
	PredictedSales  *float64  `gorm:"type:decimal(12,2)" json:"predicted_sales"`
	PredictedOrders *int      `gorm:"type:int" json:"predicted_orders"`
	ConfidenceLevel *float64  `gorm:"type:decimal(5,2)" json:"confidence_level"`
	CreatedAt       time.Time `gorm:"default:now()" json:"created_at"`

	// Relationships
	Tenant     Tenant     `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
}

// InventoryForecast represents an inventory forecast
type InventoryForecast struct {
	InventoryForecastID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"inventory_forecast_id"`
	InventoryItemID     uuid.UUID `gorm:"type:uuid;not null;index" json:"inventory_item_id"`
	TenantID            uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	ForecastDate        time.Time `gorm:"not null;index" json:"forecast_date"`
	PredictedQuantity   *float64  `gorm:"type:decimal(12,3)" json:"predicted_quantity"`
	ConfidenceLevel     *float64  `gorm:"type:decimal(5,2)" json:"confidence_level"`
	CreatedAt           time.Time `gorm:"default:now()" json:"created_at"`

	// Relationships
	InventoryItem InventoryItem `gorm:"foreignKey:InventoryItemID;constraint:OnDelete:CASCADE" json:"-"`
	Tenant        Tenant        `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// ForecastAccuracy represents forecast accuracy metrics
type ForecastAccuracy struct {
	ForecastAccuracyID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"forecast_accuracy_id"`
	ForecastID         uuid.UUID `gorm:"type:uuid;not null;index" json:"forecast_id"`
	ActualValue        float64   `gorm:"type:decimal(12,2);not null" json:"actual_value"`
	AccuracyPercentage float64   `gorm:"type:decimal(5,2);not null" json:"accuracy_percentage"`
	EvaluatedAt        time.Time `gorm:"not null;default:now()" json:"evaluated_at"`

	// Relationships
	Forecast DemandForecast `gorm:"foreignKey:ForecastID;constraint:OnDelete:CASCADE" json:"-"`
}

// BeforeCreate hooks
func (df *DemandForecast) BeforeCreate(tx *gorm.DB) error {
	if df.ForecastID == uuid.Nil {
		df.ForecastID = uuid.New()
	}
	return nil
}

func (sf *SalesForecast) BeforeCreate(tx *gorm.DB) error {
	if sf.SalesForecastID == uuid.Nil {
		sf.SalesForecastID = uuid.New()
	}
	return nil
}

func (invf *InventoryForecast) BeforeCreate(tx *gorm.DB) error {
	if invf.InventoryForecastID == uuid.Nil {
		invf.InventoryForecastID = uuid.New()
	}
	return nil
}

func (fa *ForecastAccuracy) BeforeCreate(tx *gorm.DB) error {
	if fa.ForecastAccuracyID == uuid.Nil {
		fa.ForecastAccuracyID = uuid.New()
	}
	return nil
}
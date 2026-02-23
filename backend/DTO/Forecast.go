package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ===================================
// DEMAND FORECAST DTOs
// ===================================

// CreateDemandForecastRequest
type CreateDemandForecastRequest struct {
	RestaurantID    uuid.UUID `json:"restaurant_id" binding:"required"`
	ForecastDate    time.Time `json:"forecast_date" binding:"required"`
	ForecastType    string    `json:"forecast_type" binding:"required,max=100"` // orders, customers, revenue
	PredictedValue  float64   `json:"predicted_value" binding:"required"`
	ConfidenceLevel *float64  `json:"confidence_level" binding:"omitempty,min=0,max=100"`
}

// Validate validates CreateDemandForecastRequest
func (r *CreateDemandForecastRequest) Validate() error {
	if r.ForecastDate.Before(time.Now().AddDate(0, 0, -1)) {
		return ErrPastForecastDate
	}
	if r.PredictedValue < 0 {
		return ErrNegativePredictedValue
	}
	if r.ConfidenceLevel != nil {
		if *r.ConfidenceLevel < 0 || *r.ConfidenceLevel > 100 {
			return ErrInvalidConfidenceLevel
		}
	}
	return nil
}

// DemandForecastResponse
type DemandForecastResponse struct {
	ForecastID      uuid.UUID `json:"forecast_id"`
	TenantID        uuid.UUID `json:"tenant_id"`
	RestaurantID    uuid.UUID `json:"restaurant_id"`
	ForecastDate    time.Time `json:"forecast_date"`
	ForecastType    string    `json:"forecast_type"`
	PredictedValue  float64   `json:"predicted_value"`
	ConfidenceLevel *float64  `json:"confidence_level,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// DemandForecastListResponse
type DemandForecastListResponse struct {
	Forecasts  []DemandForecastResponse `json:"forecasts"`
	Total      int64                    `json:"total"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"page_size"`
	TotalPages int                      `json:"total_pages"`
}

// ===================================
// SALES FORECAST DTOs
// ===================================

// CreateSalesForecastRequest
type CreateSalesForecastRequest struct {
	RestaurantID    uuid.UUID `json:"restaurant_id" binding:"required"`
	ForecastDate    time.Time `json:"forecast_date" binding:"required"`
	PredictedSales  *float64  `json:"predicted_sales" binding:"omitempty,min=0"`
	PredictedOrders *int      `json:"predicted_orders" binding:"omitempty,min=0"`
	ConfidenceLevel *float64  `json:"confidence_level" binding:"omitempty,min=0,max=100"`
}

// Validate validates CreateSalesForecastRequest
func (r *CreateSalesForecastRequest) Validate() error {
	if r.ForecastDate.Before(time.Now().AddDate(0, 0, -1)) {
		return ErrPastForecastDate
	}
	if r.PredictedSales == nil && r.PredictedOrders == nil {
		return ErrNoForecastValues
	}
	if r.PredictedSales != nil && *r.PredictedSales < 0 {
		return ErrNegativePredictedSales
	}
	if r.PredictedOrders != nil && *r.PredictedOrders < 0 {
		return ErrNegativePredictedOrders
	}
	if r.ConfidenceLevel != nil {
		if *r.ConfidenceLevel < 0 || *r.ConfidenceLevel > 100 {
			return ErrInvalidConfidenceLevel
		}
	}
	return nil
}

// SalesForecastResponse
type SalesForecastResponse struct {
	SalesForecastID uuid.UUID `json:"sales_forecast_id"`
	TenantID        uuid.UUID `json:"tenant_id"`
	RestaurantID    uuid.UUID `json:"restaurant_id"`
	ForecastDate    time.Time `json:"forecast_date"`
	PredictedSales  *float64  `json:"predicted_sales,omitempty"`
	PredictedOrders *int      `json:"predicted_orders,omitempty"`
	ConfidenceLevel *float64  `json:"confidence_level,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// SalesForecastListResponse
type SalesForecastListResponse struct {
	Forecasts  []SalesForecastResponse `json:"forecasts"`
	Total      int64                   `json:"total"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"page_size"`
	TotalPages int                     `json:"total_pages"`
}

// ===================================
// INVENTORY FORECAST DTOs
// ===================================

// CreateInventoryForecastRequest
type CreateInventoryForecastRequest struct {
	InventoryItemID   uuid.UUID `json:"inventory_item_id" binding:"required"`
	ForecastDate      time.Time `json:"forecast_date" binding:"required"`
	PredictedQuantity *float64  `json:"predicted_quantity" binding:"omitempty,min=0"`
	ConfidenceLevel   *float64  `json:"confidence_level" binding:"omitempty,min=0,max=100"`
}

// Validate validates CreateInventoryForecastRequest
func (r *CreateInventoryForecastRequest) Validate() error {
	if r.ForecastDate.Before(time.Now().AddDate(0, 0, -1)) {
		return ErrPastForecastDate
	}
	if r.PredictedQuantity != nil && *r.PredictedQuantity < 0 {
		return ErrNegativePredictedQuantity
	}
	if r.ConfidenceLevel != nil {
		if *r.ConfidenceLevel < 0 || *r.ConfidenceLevel > 100 {
			return ErrInvalidConfidenceLevel
		}
	}
	return nil
}

// InventoryForecastResponse
type InventoryForecastResponse struct {
	InventoryForecastID uuid.UUID                `json:"inventory_forecast_id"`
	InventoryItemID     uuid.UUID                `json:"inventory_item_id"`
	TenantID            uuid.UUID                `json:"tenant_id"`
	ForecastDate        time.Time                `json:"forecast_date"`
	PredictedQuantity   *float64                 `json:"predicted_quantity,omitempty"`
	ConfidenceLevel     *float64                 `json:"confidence_level,omitempty"`
	InventoryItem       *InventoryItemSummaryDTO `json:"inventory_item,omitempty"`
	CreatedAt           time.Time                `json:"created_at"`
}

// InventoryForecastListResponse
type InventoryForecastListResponse struct {
	Forecasts  []InventoryForecastResponse `json:"forecasts"`
	Total      int64                       `json:"total"`
	Page       int                         `json:"page"`
	PageSize   int                         `json:"page_size"`
	TotalPages int                         `json:"total_pages"`
}

// ===================================
// FORECAST ACCURACY DTOs
// ===================================

// CreateForecastAccuracyRequest
type CreateForecastAccuracyRequest struct {
	ForecastID  uuid.UUID `json:"forecast_id" binding:"required"`
	ActualValue float64   `json:"actual_value" binding:"required"`
}

// Validate validates CreateForecastAccuracyRequest
func (r *CreateForecastAccuracyRequest) Validate() error {
	if r.ActualValue < 0 {
		return ErrNegativeActualValue
	}
	return nil
}

// ForecastAccuracyResponse
type ForecastAccuracyResponse struct {
	ForecastAccuracyID uuid.UUID `json:"forecast_accuracy_id"`
	ForecastID         uuid.UUID `json:"forecast_id"`
	ActualValue        float64   `json:"actual_value"`
	AccuracyPercentage float64   `json:"accuracy_percentage"`
	EvaluatedAt        time.Time `json:"evaluated_at"`
}

// ForecastAccuracyListResponse
type ForecastAccuracyListResponse struct {
	Accuracies []ForecastAccuracyResponse `json:"accuracies"`
	Total      int64                      `json:"total"`
	Page       int                        `json:"page"`
	PageSize   int                        `json:"page_size"`
	TotalPages int                        `json:"total_pages"`
}

// ===================================
// COMMON FORECAST DTOs
// ===================================

// ForecastFilterRequest - Common filter for all forecasts
type ForecastFilterRequest struct {
	RestaurantID *uuid.UUID `form:"restaurant_id"`
	DateFrom     *time.Time `form:"date_from" time_format:"2006-01-02"`
	DateTo       *time.Time `form:"date_to" time_format:"2006-01-02"`
	Page         int        `form:"page" binding:"min=1"`
	PageSize     int        `form:"page_size" binding:"min=1,max=100"`
	SortBy       string     `form:"sort_by" binding:"omitempty,oneof=forecast_date predicted_value confidence_level"`
	SortOrder    string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// ForecastSummaryDTO
type ForecastSummaryDTO struct {
	ForecastID      uuid.UUID `json:"forecast_id"`
	ForecastDate    time.Time `json:"forecast_date"`
	PredictedValue  float64   `json:"predicted_value"`
	ConfidenceLevel *float64  `json:"confidence_level,omitempty"`
}

// ===================================
// VALIDATION ERRORS
// ===================================

var (
	ErrPastForecastDate          = NewValidationError("forecast date cannot be in the past")
	ErrNegativePredictedValue    = NewValidationError("predicted value cannot be negative")
	ErrNegativePredictedSales    = NewValidationError("predicted sales cannot be negative")
	ErrNegativePredictedOrders   = NewValidationError("predicted orders cannot be negative")
	ErrNegativePredictedQuantity = NewValidationError("predicted quantity cannot be negative")
	ErrNegativeActualValue       = NewValidationError("actual value cannot be negative")
	ErrInvalidConfidenceLevel    = NewValidationError("confidence level must be between 0 and 100")
	ErrNoForecastValues          = NewValidationError("at least one forecast value (sales or orders) must be provided")
)
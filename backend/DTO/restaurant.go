package DTO

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New()

// CreateRestaurantRequest represents the request to create a new restaurant
type CreateRestaurantRequest struct {
	Name       string `json:"name" binding:"required,min=2,max=255"`
	Address    string `json:"address" binding:"required"`
	City       string `json:"city" binding:"required,max=100"`
	State      string `json:"state" binding:"omitempty,max=100"`
	Country    string `json:"country" binding:"required,max=100"`
	PostalCode string `json:"postal_code" binding:"omitempty,max=20"`
	Phone      string `json:"phone" binding:"required,max=50"`
	Email      string `json:"email" binding:"required,email,max=255"`
}

// UpdateRestaurantRequest represents the request to update a restaurant
type UpdateRestaurantRequest struct {
	Name       *string `json:"name" binding:"omitempty,min=2,max=255"`
	Address    *string `json:"address"`
	City       *string `json:"city" binding:"omitempty,max=100"`
	State      *string `json:"state" binding:"omitempty,max=100"`
	Country    *string `json:"country" binding:"omitempty,max=100"`
	PostalCode *string `json:"postal_code" binding:"omitempty,max=20"`
	Phone      *string `json:"phone" binding:"omitempty,max=50"`
	Email      *string `json:"email" binding:"omitempty,email,max=255"`
	IsActive   *bool   `json:"is_active"`
}

// RestaurantResponse represents the response for a single restaurant
type RestaurantResponse struct {
	RestaurantID uuid.UUID  `json:"restaurant_id"`
	TenantID     uuid.UUID  `json:"tenant_id"`
	Name         string     `json:"name"`
	Address      string     `json:"address"`
	City         string     `json:"city"`
	State        string     `json:"state"`
	Country      string     `json:"country"`
	PostalCode   string     `json:"postal_code"`
	Phone        string     `json:"phone"`
	Email        string     `json:"email"`
	IsActive     bool       `json:"is_active"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// RestaurantDetailResponse represents a detailed restaurant response with related data
type RestaurantDetailResponse struct {
	RestaurantID uuid.UUID            `json:"restaurant_id"`
	TenantID     uuid.UUID            `json:"tenant_id"`
	Name         string               `json:"name"`
	Address      string               `json:"address"`
	City         string               `json:"city"`
	State        string               `json:"state"`
	Country      string               `json:"country"`
	PostalCode   string               `json:"postal_code"`
	Phone        string               `json:"phone"`
	Email        string               `json:"email"`
	IsActive     bool                 `json:"is_active"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
	Floors       []FloorSummaryDTO    `json:"floors,omitempty"`
	Stats        *RestaurantStatsDTO  `json:"stats,omitempty"`
}

// RestaurantSummaryDTO represents minimal restaurant information
type RestaurantSummaryDTO struct {
	RestaurantID uuid.UUID `json:"restaurant_id"`
	Name         string    `json:"name"`
	City         string    `json:"city"`
	Phone        string    `json:"phone"`
	IsActive     bool      `json:"is_active"`
}

// RestaurantListResponse represents a paginated list of restaurants
type RestaurantListResponse struct {
	Restaurants []RestaurantResponse `json:"restaurants"`
	Total       int64                `json:"total"`
	Page        int                  `json:"page"`
	PageSize    int                  `json:"page_size"`
	TotalPages  int                  `json:"total_pages"`
}

// RestaurantFilterRequest represents filter options for listing restaurants
type RestaurantFilterRequest struct {
	City      *string `form:"city"`
	State     *string `form:"state"`
	Country   *string `form:"country"`
	IsActive  *bool   `form:"is_active"`
	Search    *string `form:"search"` // search by name, address, or city
	Page      int     `form:"page" binding:"min=1"`
	PageSize  int     `form:"page_size" binding:"min=1,max=100"`
	SortBy    string  `form:"sort_by" binding:"omitempty,oneof=name city created_at updated_at"`
	SortOrder string  `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// RestaurantStatsDTO represents restaurant statistics
type RestaurantStatsDTO struct {
	TotalFloors      int     `json:"total_floors"`
	TotalTables      int     `json:"total_tables"`
	AvailableTables  int     `json:"available_tables"`
	OccupiedTables   int     `json:"occupied_tables"`
	ActiveOrders     int     `json:"active_orders"`
	TodayOrders      int     `json:"today_orders"`
	TodayRevenue     float64 `json:"today_revenue"`
	WeekRevenue      float64 `json:"week_revenue"`
	MonthRevenue     float64 `json:"month_revenue"`
}

// Floor-related DTOs

// CreateFloorRequest represents the request to create a new floor
type CreateFloorRequest struct {
	Name        string `json:"name" binding:"required,max=255"`
	FloorNumber int    `json:"floor_number" binding:"required"`
}

// UpdateFloorRequest represents the request to update a floor
type UpdateFloorRequest struct {
	Name        *string `json:"name" binding:"omitempty,max=255"`
	FloorNumber *int    `json:"floor_number"`
}

// FloorResponse represents the response for a single floor
type FloorResponse struct {
	FloorID      int       `json:"floor_id"`
	RestaurantID uuid.UUID `json:"restaurant_id"`
	Name         string    `json:"name"`
	FloorNumber  int       `json:"floor_number"`
	TableCount   int       `json:"table_count"`
	CreatedAt    time.Time `json:"created_at"`
	Tables       []TableSummaryDTO `json:"tables,omitempty"`
}

// FloorSummaryDTO represents minimal floor information
type FloorSummaryDTO struct {
	FloorID     int    `json:"floor_id"`
	Name        string `json:"name"`
	FloorNumber int    `json:"floor_number"`
	TableCount  int    `json:"table_count"`
}

// FloorListResponse represents a paginated list of floors
type FloorListResponse struct {
	Floors     []FloorResponse `json:"floors"`
	Total      int64           `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

// Table-related DTOs

// CreateTableRequest represents the request to create a new table
type CreateTableRequest struct {
	FloorID     int    `json:"floor_id" binding:"required"`
	TableNumber string `json:"table_number" binding:"required,max=50"`
	Capacity    int    `json:"capacity" binding:"required,min=1"`
}

// UpdateTableRequest represents the request to update a table
type UpdateTableRequest struct {
	FloorID          *int    `json:"floor_id"`
	TableNumber      *string `json:"table_number" binding:"omitempty,max=50"`
	Capacity         *int    `json:"capacity" binding:"omitempty,min=1"`
	CurrentOccupancy *int    `json:"current_occupancy" binding:"omitempty,min=0"`
	Status           *string `json:"status" binding:"omitempty,oneof=available occupied reserved cleaning maintenance"`
}

// TableResponse represents the response for a single table
type TableResponse struct {
	TableID          int        `json:"table_id"`
	RestaurantID     uuid.UUID  `json:"restaurant_id"`
	FloorID          int        `json:"floor_id"`
	TableNumber      string     `json:"table_number"`
	Capacity         int        `json:"capacity"`
	CurrentOccupancy int        `json:"current_occupancy"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	LastReservedAt   *time.Time `json:"last_reserved_at,omitempty"`
	LastCleanedAt    *time.Time `json:"last_cleaned_at,omitempty"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
	Floor            *FloorSummaryDTO `json:"floor,omitempty"`
}

// // TableSummaryDTO represents minimal table information (used in other responses)
// type TableSummaryDTO struct {
// 	TableID     int    `json:"table_id"`
// 	TableNumber string `json:"table_number"`
// 	FloorID     int    `json:"floor_id"`
// 	Capacity    int    `json:"capacity"`
// 	Status      string `json:"status"`
// }

// TableListResponse represents a paginated list of tables
type TableListResponse struct {
	Tables     []TableResponse `json:"tables"`
	Total      int64           `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

// TableFilterRequest represents filter options for listing tables
type TableFilterRequest struct {
	FloorID   *int    `form:"floor_id"`
	Status    *string `form:"status" binding:"omitempty,oneof=available occupied reserved cleaning maintenance"`
	MinCapacity *int  `form:"min_capacity"`
	MaxCapacity *int  `form:"max_capacity"`
	Page      int     `form:"page" binding:"min=1"`
	PageSize  int     `form:"page_size" binding:"min=1,max=100"`
	SortBy    string  `form:"sort_by" binding:"omitempty,oneof=table_number capacity status created_at"`
	SortOrder string  `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// UpdateTableStatusRequest represents a request to update table status
type UpdateTableStatusRequest struct {
	Status           string `json:"status" binding:"required,oneof=available occupied reserved cleaning maintenance"`
	CurrentOccupancy *int   `json:"current_occupancy" binding:"omitempty,min=0"`
}

// BulkUpdateTableStatusRequest represents a bulk table status update
type BulkUpdateTableStatusRequest struct {
	TableIDs []int  `json:"table_ids" binding:"required,min=1"`
	Status   string `json:"status" binding:"required,oneof=available occupied reserved cleaning maintenance"`
}

// Region-related DTOs

// CreateRegionRequest represents the request to create a new region
type CreateRegionRequest struct {
	Name           string     `json:"name" binding:"required,max=255"`
	Code           string     `json:"code" binding:"omitempty,max=50"`
	ParentRegionID *uuid.UUID `json:"parent_region_id,omitempty"`
	Level          *int       `json:"level"`
}

// UpdateRegionRequest represents the request to update a region
type UpdateRegionRequest struct {
	Name           *string    `json:"name" binding:"omitempty,max=255"`
	Code           *string    `json:"code" binding:"omitempty,max=50"`
	ParentRegionID *uuid.UUID `json:"parent_region_id,omitempty"`
	Level          *int       `json:"level"`
	IsActive       *bool      `json:"is_active"`
}

// RegionResponse represents the response for a single region
type RegionResponse struct {
	RegionID       uuid.UUID       `json:"region_id"`
	TenantID       uuid.UUID       `json:"tenant_id"`
	Name           string          `json:"name"`
	Code           string          `json:"code"`
	ParentRegionID *uuid.UUID      `json:"parent_region_id,omitempty"`
	Level          *int            `json:"level,omitempty"`
	IsActive       bool            `json:"is_active"`
	CreatedAt      time.Time       `json:"created_at"`
	ParentRegion   *RegionSummaryDTO `json:"parent_region,omitempty"`
	SubRegions     []RegionSummaryDTO `json:"sub_regions,omitempty"`
}

// RegionSummaryDTO represents minimal region information
type RegionSummaryDTO struct {
	RegionID uuid.UUID `json:"region_id"`
	Name     string    `json:"name"`
	Code     string    `json:"code"`
	Level    *int      `json:"level,omitempty"`
}

// RegionListResponse represents a paginated list of regions
type RegionListResponse struct {
	Regions    []RegionResponse `json:"regions"`
	Total      int64            `json:"total"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}

// RegionalSetting-related DTOs

// CreateRegionalSettingRequest represents the request to create a regional setting
type CreateRegionalSettingRequest struct {
	RegionID     *uuid.UUID `json:"region_id,omitempty"`
	RestaurantID *uuid.UUID `json:"restaurant_id,omitempty"`
	Timezone     string     `json:"timezone" binding:"required,max=100"`
	CurrencyCode string     `json:"currency_code" binding:"required,len=3"` // ISO 4217
	DateFormat   string     `json:"date_format" binding:"required,max=50"`
	TimeFormat   string     `json:"time_format" binding:"required,max=50"`
	LanguageCode string     `json:"language_code" binding:"required,len=2"` // ISO 639-1
	TaxRate      *float64   `json:"tax_rate" binding:"omitempty,min=0,max=100"`
}

// UpdateRegionalSettingRequest represents the request to update a regional setting
type UpdateRegionalSettingRequest struct {
	Timezone     *string  `json:"timezone" binding:"omitempty,max=100"`
	CurrencyCode *string  `json:"currency_code" binding:"omitempty,len=3"`
	DateFormat   *string  `json:"date_format" binding:"omitempty,max=50"`
	TimeFormat   *string  `json:"time_format" binding:"omitempty,max=50"`
	LanguageCode *string  `json:"language_code" binding:"omitempty,len=2"`
	TaxRate      *float64 `json:"tax_rate" binding:"omitempty,min=0,max=100"`
}

// RegionalSettingResponse represents the response for a regional setting
type RegionalSettingResponse struct {
	RegionalSettingID uuid.UUID  `json:"regional_setting_id"`
	TenantID          uuid.UUID  `json:"tenant_id"`
	RegionID          *uuid.UUID `json:"region_id,omitempty"`
	RestaurantID      *uuid.UUID `json:"restaurant_id,omitempty"`
	Timezone          string     `json:"timezone"`
	CurrencyCode      string     `json:"currency_code"`
	DateFormat        string     `json:"date_format"`
	TimeFormat        string     `json:"time_format"`
	LanguageCode      string     `json:"language_code"`
	TaxRate           *float64   `json:"tax_rate,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	Region            *RegionSummaryDTO      `json:"region,omitempty"`
	Restaurant        *RestaurantSummaryDTO  `json:"restaurant,omitempty"`
}

// RegionalSettingListResponse represents a paginated list of regional settings
type RegionalSettingListResponse struct {
	Settings   []RegionalSettingResponse `json:"settings"`
	Total      int64                     `json:"total"`
	Page       int                       `json:"page"`
	PageSize   int                       `json:"page_size"`
	TotalPages int                       `json:"total_pages"`
}

// Validation helpers

// Validate validates the CreateRestaurantRequest
func (r *CreateRestaurantRequest) Validate() error {
	// TODO: Add custom validation logic here if needed

	return validate.Struct(r)
}

// Validate validates the CreateTableRequest
func (r *CreateTableRequest) Validate() error {
	if r.Capacity < 1 {
		return ErrInvalidTableCapacity
	}
	return nil
}

// Custom errors
var (
	ErrInvalidTableCapacity = NewValidationError("table capacity must be at least 1")
)
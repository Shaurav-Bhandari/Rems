// DTO/settings.go
package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ============================================
// TENANT SETTING DTOs
// ============================================

// CreateTenantSettingRequest - Create a new tenant setting
type CreateTenantSettingRequest struct {
	Key                string   `json:"key" binding:"required,min=1,max=100"`
	Value              string   `json:"value" binding:"required"`
	Domain             string   `json:"domain" binding:"required,oneof=financial operational display security compliance integration"`
	DataType           string   `json:"data_type" binding:"required,oneof=string int float bool json"`
	MinRoleLevel       int      `json:"min_role_level" binding:"min=1,max=99"`
	RequiredPermission string   `json:"required_permission" binding:"omitempty,max=100"`
	IsSystemLocked     bool     `json:"is_system_locked"`
	Description        string   `json:"description" binding:"omitempty,max=1000"`
	ValidationRule     string   `json:"validation_rule" binding:"omitempty"`
	DefaultValue       string   `json:"default_value" binding:"required"`
}

// UpdateTenantSettingRequest - Update a tenant setting
type UpdateTenantSettingRequest struct {
	Value              *string `json:"value"`
	MinRoleLevel       *int    `json:"min_role_level" binding:"omitempty,min=1,max=99"`
	RequiredPermission *string `json:"required_permission" binding:"omitempty,max=100"`
	IsSystemLocked     *bool   `json:"is_system_locked"`
	Description        *string `json:"description" binding:"omitempty,max=1000"`
	ValidationRule     *string `json:"validation_rule"`
	Reason             string  `json:"reason" binding:"omitempty,max=500"`
}

// SetSettingRequest - Set a setting value (with elevation support)
type SetSettingRequest struct {
	Key            string  `json:"key" binding:"required"`
	Value          string  `json:"value" binding:"required"`
	Reason         string  `json:"reason" binding:"omitempty,max=500"`
	ElevationToken *string `json:"elevation_token,omitempty"` // Manager PIN or token
}

// TenantSettingResponse - Single tenant setting
type TenantSettingResponse struct {
	SettingID          uuid.UUID  `json:"setting_id"`
	TenantID           uuid.UUID  `json:"tenant_id"`
	Key                string     `json:"key"`
	Value              string     `json:"value"`
	Domain             string     `json:"domain"`
	DataType           string     `json:"data_type"`
	MinRoleLevel       int        `json:"min_role_level"`
	RequiredPermission string     `json:"required_permission,omitempty"`
	IsSystemLocked     bool       `json:"is_system_locked"`
	Description        string     `json:"description,omitempty"`
	DefaultValue       string     `json:"default_value"`
	ModifiedBy         uuid.UUID  `json:"modified_by"`
	ModifiedAt         time.Time  `json:"modified_at"`
	ModifiedByName     string     `json:"modified_by_name,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// TenantSettingListResponse - Paginated list of tenant settings
type TenantSettingListResponse struct {
	Settings   []TenantSettingResponse `json:"settings"`
	Total      int64                   `json:"total"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"page_size"`
	TotalPages int                     `json:"total_pages"`
}

// TenantSettingFilterRequest - Filter options
type TenantSettingFilterRequest struct {
	Domain       *string `form:"domain" binding:"omitempty,oneof=financial operational display security compliance integration"`
	MinRoleLevel *int    `form:"min_role_level" binding:"omitempty,min=1,max=99"`
	Search       *string `form:"search"` // Search by key or description
	Page         int     `form:"page" binding:"min=1"`
	PageSize     int     `form:"page_size" binding:"min=1,max=100"`
	SortBy       string  `form:"sort_by" binding:"omitempty,oneof=key domain min_role_level modified_at"`
	SortOrder    string  `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// ============================================
// RESTAURANT PROFILE DTOs
// ============================================

// CreateRestaurantProfileRequest - Create restaurant profile
type CreateRestaurantProfileRequest struct {
	RestaurantID uuid.UUID `json:"restaurant_id" binding:"required"`
	
	// Financial
	CurrencyCode          string   `json:"currency_code" binding:"required,len=3"`
	TaxRegistrationNumber string   `json:"tax_registration_number" binding:"omitempty,max=100"`
	ServiceChargePct      *float64 `json:"service_charge_pct" binding:"omitempty,min=0,max=100"`
	TaxInclusivePricing   bool     `json:"tax_inclusive_pricing"`
	DefaultTaxRate        *float64 `json:"default_tax_rate" binding:"omitempty,min=0,max=100"`
	AlcoholTaxRate        *float64 `json:"alcohol_tax_rate" binding:"omitempty,min=0,max=100"`
	
	// Operational
	AutoKOTFiring          bool   `json:"auto_kot_firing"`
	TableExpirationMinutes int    `json:"table_expiration_minutes" binding:"omitempty,min=5,max=180"`
	StockWarningThreshold  int    `json:"stock_warning_threshold" binding:"omitempty,min=1"`
	EnableHappyHour        bool   `json:"enable_happy_hour"`
	HappyHourDiscount      *float64 `json:"happy_hour_discount" binding:"omitempty,min=0,max=100"`
	
	// Display
	LanguageISO         string `json:"language_iso" binding:"required,len=2"`
	BrandPrimaryColor   string `json:"brand_primary_color" binding:"required,len=7"` // Hex
	BrandSecondaryColor string `json:"brand_secondary_color" binding:"required,len=7"`
	LogoURL             string `json:"logo_url" binding:"omitempty,url"`
	ReceiptHeader       string `json:"receipt_header" binding:"omitempty,max=500"`
	ReceiptFooter       string `json:"receipt_footer" binding:"omitempty,max=500"`
}

// UpdateRestaurantProfileRequest - Update restaurant profile
type UpdateRestaurantProfileRequest struct {
	// Financial
	CurrencyCode          *string  `json:"currency_code" binding:"omitempty,len=3"`
	TaxRegistrationNumber *string  `json:"tax_registration_number" binding:"omitempty,max=100"`
	ServiceChargePct      *float64 `json:"service_charge_pct" binding:"omitempty,min=0,max=100"`
	TaxInclusivePricing   *bool    `json:"tax_inclusive_pricing"`
	DefaultTaxRate        *float64 `json:"default_tax_rate" binding:"omitempty,min=0,max=100"`
	AlcoholTaxRate        *float64 `json:"alcohol_tax_rate" binding:"omitempty,min=0,max=100"`
	
	// Operational
	AutoKOTFiring          *bool    `json:"auto_kot_firing"`
	TableExpirationMinutes *int     `json:"table_expiration_minutes" binding:"omitempty,min=5,max=180"`
	StockWarningThreshold  *int     `json:"stock_warning_threshold" binding:"omitempty,min=1"`
	EnableHappyHour        *bool    `json:"enable_happy_hour"`
	HappyHourStart         *string  `json:"happy_hour_start" binding:"omitempty"`        // HH:MM format
	HappyHourEnd           *string  `json:"happy_hour_end" binding:"omitempty"`          // HH:MM format
	HappyHourDiscount      *float64 `json:"happy_hour_discount" binding:"omitempty,min=0,max=100"`
	
	// Display
	LanguageISO         *string `json:"language_iso" binding:"omitempty,len=2"`
	BrandPrimaryColor   *string `json:"brand_primary_color" binding:"omitempty,len=7"`
	BrandSecondaryColor *string `json:"brand_secondary_color" binding:"omitempty,len=7"`
	LogoURL             *string `json:"logo_url" binding:"omitempty,url"`
	ReceiptHeader       *string `json:"receipt_header" binding:"omitempty,max=500"`
	ReceiptFooter       *string `json:"receipt_footer" binding:"omitempty,max=500"`
	
	// Security
	RequireManagerPINForVoid     *bool    `json:"require_manager_pin_for_void"`
	RequireManagerPINForDiscount *bool    `json:"require_manager_pin_for_discount"`
	MaxDiscountPctStaff          *float64 `json:"max_discount_pct_staff" binding:"omitempty,min=0,max=100"`
	MaxDiscountPctManager        *float64 `json:"max_discount_pct_manager" binding:"omitempty,min=0,max=100"`
	OffsiteLoginAllowed          *bool    `json:"offsite_login_allowed"`
}

// RestaurantProfileResponse - Complete restaurant profile
type RestaurantProfileResponse struct {
	ProfileID    uuid.UUID `json:"profile_id"`
	RestaurantID uuid.UUID `json:"restaurant_id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	
	// Financial Settings
	CurrencyCode          string  `json:"currency_code"`
	TaxRegistrationNumber string  `json:"tax_registration_number,omitempty"`
	ServiceChargePct      float64 `json:"service_charge_pct"`
	TaxInclusivePricing   bool    `json:"tax_inclusive_pricing"`
	DefaultTaxRate        float64 `json:"default_tax_rate"`
	AlcoholTaxRate        float64 `json:"alcohol_tax_rate"`
	
	// Operational Settings
	AutoKOTFiring          bool       `json:"auto_kot_firing"`
	TableExpirationMinutes int        `json:"table_expiration_minutes"`
	StockWarningThreshold  int        `json:"stock_warning_threshold"`
	EnableHappyHour        bool       `json:"enable_happy_hour"`
	HappyHourStart         *time.Time `json:"happy_hour_start,omitempty"`
	HappyHourEnd           *time.Time `json:"happy_hour_end,omitempty"`
	HappyHourDiscount      float64    `json:"happy_hour_discount"`
	
	// Display & Branding
	LanguageISO         string `json:"language_iso"`
	BrandPrimaryColor   string `json:"brand_primary_color"`
	BrandSecondaryColor string `json:"brand_secondary_color"`
	LogoURL             string `json:"logo_url,omitempty"`
	ReceiptHeader       string `json:"receipt_header,omitempty"`
	ReceiptFooter       string `json:"receipt_footer,omitempty"`
	
	// Security Settings
	RequireManagerPINForVoid     bool    `json:"require_manager_pin_for_void"`
	RequireManagerPINForDiscount bool    `json:"require_manager_pin_for_discount"`
	MaxDiscountPctStaff          float64 `json:"max_discount_pct_staff"`
	MaxDiscountPctManager        float64 `json:"max_discount_pct_manager"`
	OffsiteLoginAllowed          bool    `json:"offsite_login_allowed"`
	
	// Compliance
	GDPREnabled            bool `json:"gdpr_enabled"`
	DataRetentionDays      int  `json:"data_retention_days"`
	RequireAgeVerification bool `json:"require_age_verification"`
	
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ============================================
// USER PROFILE DTOs
// ============================================

// CreateUserProfileRequest - Create user profile
type CreateUserProfileRequest struct {
	UserID       uuid.UUID  `json:"user_id" binding:"required"`
	RestaurantID *uuid.UUID `json:"restaurant_id,omitempty"`
	
	// Profile Type
	ProfileType string `json:"profile_type" binding:"required,oneof=Owner Manager Staff"`
	RoleLevel   int    `json:"role_level" binding:"required,min=1,max=99"`
	
	// Employment
	EmployeeNumber   string     `json:"employee_number" binding:"omitempty,max=50"`
	Department       string     `json:"department" binding:"omitempty,max=100"`
	JobTitle         string     `json:"job_title" binding:"omitempty,max=100"`
	HireDate         *time.Time `json:"hire_date,omitempty"`
	EmploymentStatus string     `json:"employment_status" binding:"omitempty,oneof=active terminated suspended"`
	
	// Compensation (Optional - for HR access only)
	HourlyRate     *float64 `json:"hourly_rate" binding:"omitempty,min=0"`
	MonthlySalary  *float64 `json:"monthly_salary" binding:"omitempty,min=0"`
	CommissionRate *float64 `json:"commission_rate" binding:"omitempty,min=0,max=100"`
	
	// Schedule
	ShiftPattern string `json:"shift_pattern" binding:"omitempty,max=100"`
	DefaultShift string `json:"default_shift" binding:"omitempty,oneof=morning afternoon evening night"`
	WeeklyHours  int    `json:"weekly_hours" binding:"omitempty,min=1,max=168"`
	
	// Security
	CanLoginOffsite bool `json:"can_login_offsite"`
}

// UpdateUserProfileRequest - Update user profile
type UpdateUserProfileRequest struct {
	ProfileType      *string    `json:"profile_type" binding:"omitempty,oneof=Owner Manager Staff"`
	RoleLevel        *int       `json:"role_level" binding:"omitempty,min=1,max=99"`
	Department       *string    `json:"department" binding:"omitempty,max=100"`
	JobTitle         *string    `json:"job_title" binding:"omitempty,max=100"`
	EmploymentStatus *string    `json:"employment_status" binding:"omitempty,oneof=active terminated suspended"`
	HourlyRate       *float64   `json:"hourly_rate" binding:"omitempty,min=0"`
	MonthlySalary    *float64   `json:"monthly_salary" binding:"omitempty,min=0"`
	ShiftPattern     *string    `json:"shift_pattern" binding:"omitempty,max=100"`
	DefaultShift     *string    `json:"default_shift" binding:"omitempty,oneof=morning afternoon evening night"`
	WeeklyHours      *int       `json:"weekly_hours" binding:"omitempty,min=1,max=168"`
	CanLoginOffsite  *bool      `json:"can_login_offsite"`
	Notes            *string    `json:"notes" binding:"omitempty,max=2000"`
}

// UserProfileResponse - User profile (sanitized - no salary info unless authorized)
type UserProfileResponse struct {
	ProfileID    uuid.UUID  `json:"profile_id"`
	UserID       uuid.UUID  `json:"user_id"`
	TenantID     uuid.UUID  `json:"tenant_id"`
	RestaurantID *uuid.UUID `json:"restaurant_id,omitempty"`
	
	// Profile
	ProfileType string `json:"profile_type"`
	RoleLevel   int    `json:"role_level"`
	
	// Employment (Public)
	EmployeeNumber   string     `json:"employee_number,omitempty"`
	Department       string     `json:"department,omitempty"`
	JobTitle         string     `json:"job_title,omitempty"`
	HireDate         *time.Time `json:"hire_date,omitempty"`
	EmploymentStatus string     `json:"employment_status"`
	
	// Schedule
	ShiftPattern string `json:"shift_pattern,omitempty"`
	DefaultShift string `json:"default_shift,omitempty"`
	WeeklyHours  int    `json:"weekly_hours"`
	
	// Security
	CanLoginOffsite bool `json:"can_login_offsite"`
	HasManagerPIN   bool `json:"has_manager_pin"` // Whether PIN is set (not the actual PIN)
	
	// Performance (Public)
	TotalOrdersProcessed int       `json:"total_orders_processed"`
	AverageOrderValue    float64   `json:"average_order_value"`
	LastPerformanceReview *time.Time `json:"last_performance_review,omitempty"`
	PerformanceRating    *float64  `json:"performance_rating,omitempty"`
	
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserProfileDetailResponse - Full profile (with compensation - HR only)
type UserProfileDetailResponse struct {
	UserProfileResponse
	
	// Compensation (HR/Owner only)
	HourlyRate     *float64 `json:"hourly_rate,omitempty"`
	MonthlySalary  *float64 `json:"monthly_salary,omitempty"`
	CommissionRate *float64 `json:"commission_rate,omitempty"`
	
	// Full Details
	TerminationDate *time.Time `json:"termination_date,omitempty"`
	Notes           string     `json:"notes,omitempty"`
}

// ============================================
// MANAGER ELEVATION DTOs
// ============================================

// RequestElevationRequest - Request manager approval
type RequestElevationRequest struct {
	ManagerPIN     string     `json:"manager_pin" binding:"required,len=4"`
	Permission     string     `json:"permission" binding:"required,max=100"`
	Reason         string     `json:"reason" binding:"required,max=500"`
	OrderID        *uuid.UUID `json:"order_id,omitempty"`
	DiscountAmount *float64   `json:"discount_amount,omitempty"`
}

// RequestElevationResponse - Elevation token
type RequestElevationResponse struct {
	Token        string    `json:"token"`
	ExpiresAt    time.Time `json:"expires_at"`
	ExpiresIn    int       `json:"expires_in"` // Seconds
	ManagerName  string    `json:"manager_name"`
	ManagerLevel int       `json:"manager_level"`
}

// SetManagerPINRequest - Set or update manager PIN
type SetManagerPINRequest struct {
	PIN        string `json:"pin" binding:"required,len=4"` // Must be 4 digits
	ConfirmPIN string `json:"confirm_pin" binding:"required,eqfield=PIN"`
}

// ValidateElevationTokenRequest - Validate token before use
type ValidateElevationTokenRequest struct {
	Token string `json:"token" binding:"required"`
}

// ============================================
// SETTING OVERRIDE DTOs
// ============================================

// CreateSettingOverrideRequest - Create override
type CreateSettingOverrideRequest struct {
	RestaurantID *uuid.UUID `json:"restaurant_id,omitempty"` // Null = tenant-level
	Key          string     `json:"key" binding:"required,max=100"`
	Value        string     `json:"value" binding:"required"`
	Priority     int        `json:"priority" binding:"min=0,max=100"`
	
	// Temporal (Optional)
	ActiveFrom *time.Time `json:"active_from,omitempty"`
	ActiveUntil *time.Time `json:"active_until,omitempty"`
	DaysOfWeek []string   `json:"days_of_week,omitempty"` // ["Mon", "Tue", "Wed"]
	
	Reason string `json:"reason" binding:"omitempty,max=500"`
}

// SettingOverrideResponse - Override response
type SettingOverrideResponse struct {
	OverrideID   uuid.UUID  `json:"override_id"`
	TenantID     uuid.UUID  `json:"tenant_id"`
	RestaurantID *uuid.UUID `json:"restaurant_id,omitempty"`
	Key          string     `json:"key"`
	Value        string     `json:"value"`
	Priority     int        `json:"priority"`
	
	ActiveFrom *time.Time `json:"active_from,omitempty"`
	ActiveUntil *time.Time `json:"active_until,omitempty"`
	DaysOfWeek []string   `json:"days_of_week,omitempty"`
	
	CreatedBy     uuid.UUID `json:"created_by"`
	CreatedByName string    `json:"created_by_name,omitempty"`
	ModifiedBy    uuid.UUID `json:"modified_by"`
	Reason        string    `json:"reason,omitempty"`
	
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ============================================
// SETTING AUDIT DTOs
// ============================================

// SettingAuditLogResponse - Audit log entry
type SettingAuditLogResponse struct {
	AuditID      uuid.UUID  `json:"audit_id"`
	TenantID     uuid.UUID  `json:"tenant_id"`
	RestaurantID *uuid.UUID `json:"restaurant_id,omitempty"`
	
	SettingKey string `json:"setting_key"`
	Domain     string `json:"domain"`
	OldValue   string `json:"old_value"`
	NewValue   string `json:"new_value"`
	
	ChangedBy     uuid.UUID `json:"changed_by"`
	ChangedByName string    `json:"changed_by_name"`
	ChangedAt     time.Time `json:"changed_at"`
	
	Reason      string `json:"reason,omitempty"`
	RoleLevel   int    `json:"role_level"`
	WasElevated bool   `json:"was_elevated"`
	
	ApprovedBy     *uuid.UUID `json:"approved_by,omitempty"`
	ApprovedByName *string    `json:"approved_by_name,omitempty"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty"`
}

// SettingAuditListResponse - Paginated audit logs
type SettingAuditListResponse struct {
	AuditLogs  []SettingAuditLogResponse `json:"audit_logs"`
	Total      int64                     `json:"total"`
	Page       int                       `json:"page"`
	PageSize   int                       `json:"page_size"`
	TotalPages int                       `json:"total_pages"`
}

// SettingAuditFilterRequest - Filter audit logs
type SettingAuditFilterRequest struct {
	RestaurantID *uuid.UUID `form:"restaurant_id"`
	SettingKey   *string    `form:"setting_key"`
	Domain       *string    `form:"domain"`
	ChangedBy    *uuid.UUID `form:"changed_by"`
	DateFrom     *time.Time `form:"date_from" time_format:"2006-01-02"`
	DateTo       *time.Time `form:"date_to" time_format:"2006-01-02"`
	WasElevated  *bool      `form:"was_elevated"`
	Page         int        `form:"page" binding:"min=1"`
	PageSize     int        `form:"page_size" binding:"min=1,max=100"`
	SortBy       string     `form:"sort_by" binding:"omitempty,oneof=changed_at setting_key"`
	SortOrder    string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// ============================================
// BULK OPERATIONS DTOs
// ============================================

// BulkUpdateSettingsRequest - Update multiple settings at once
type BulkUpdateSettingsRequest struct {
	Settings []SetSettingRequest `json:"settings" binding:"required,min=1,dive"`
	Reason   string              `json:"reason" binding:"required,max=500"`
}

// BulkUpdateSettingsResponse - Bulk update result
type BulkUpdateSettingsResponse struct {
	SuccessCount int      `json:"success_count"`
	FailureCount int      `json:"failure_count"`
	Errors       []string `json:"errors,omitempty"`
}

// ============================================
// SETTINGS PRESET DTOs
// ============================================

// SettingsPresetRequest - Apply preset configuration
type SettingsPresetRequest struct {
	PresetName string `json:"preset_name" binding:"required,oneof=quick_service fine_dining fast_food cafe bar"`
}

// SettingsPreset - Predefined configuration
type SettingsPreset struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Settings    map[string]string `json:"settings"`
}

// GetPresetsResponse - Available presets
type GetPresetsResponse struct {
	Presets []SettingsPreset `json:"presets"`
}

// ============================================
// VALIDATION HELPERS
// ============================================

func (r *CreateUserProfileRequest) Validate() error {
	// Custom validation
	if r.RoleLevel < 1 || r.RoleLevel > 99 {
		return NewValidationError("role_level must be between 1 and 99")
	}
	return nil
}

func (r *SetSettingRequest) Validate() error {
	if r.Key == "" {
		return NewValidationError("key cannot be empty")
	}
	return nil
}

func (r *RequestElevationRequest) Validate() error {
	if len(r.ManagerPIN) != 4 {
		return NewValidationError("manager PIN must be 4 digits")
	}
	// Check if PIN is numeric
	for _, c := range r.ManagerPIN {
		if c < '0' || c > '9' {
			return NewValidationError("manager PIN must contain only digits")
		}
	}
	return nil
}

// ============================================
// CONSTANTS
// ============================================

const (
	// Role Levels
	RoleLevelStaff      = 1
	RoleLevelShiftLead  = 3
	RoleLevelManager    = 5
	RoleLevelSeniorMgr  = 7
	RoleLevelOwner      = 10
	RoleLevelSuperAdmin = 99
	
	// Domains
	DomainFinancial   = "financial"
	DomainOperational = "operational"
	DomainDisplay     = "display"
	DomainSecurity    = "security"
	DomainCompliance  = "compliance"
	
	// Profile Types
	ProfileTypeOwner   = "Owner"
	ProfileTypeManager = "Manager"
	ProfileTypeStaff   = "Staff"
)

// GetRoleLevelName returns human-readable role level name
func GetRoleLevelName(level int) string {
	switch {
	case level >= 99:
		return "SuperAdmin"
	case level >= 10:
		return "Owner"
	case level >= 7:
		return "Senior Manager"
	case level >= 5:
		return "Manager"
	case level >= 3:
		return "Shift Lead"
	default:
		return "Staff"
	}
}

// GetSettingPresets returns available presets
func GetSettingPresets() []SettingsPreset {
	return []SettingsPreset{
		{
			Name:        "quick_service",
			Description: "Fast-paced quick service restaurant (QSR)",
			Settings: map[string]string{
				"auto_kot_firing":            "true",
				"table_expiration_minutes":   "15",
				"service_charge_pct":         "0",
				"require_manager_pin_for_void": "false",
				"max_discount_pct_staff":     "5",
			},
		},
		{
			Name:        "fine_dining",
			Description: "Upscale fine dining establishment",
			Settings: map[string]string{
				"auto_kot_firing":            "false",
				"table_expiration_minutes":   "120",
				"service_charge_pct":         "18",
				"require_manager_pin_for_void": "true",
				"max_discount_pct_staff":     "0",
			},
		},
		{
			Name:        "fast_food",
			Description: "High-volume fast food operation",
			Settings: map[string]string{
				"auto_kot_firing":            "true",
				"table_expiration_minutes":   "10",
				"service_charge_pct":         "0",
				"require_manager_pin_for_void": "false",
				"max_discount_pct_staff":     "10",
			},
		},
		{
			Name:        "cafe",
			Description: "Casual cafe with table service",
			Settings: map[string]string{
				"auto_kot_firing":            "true",
				"table_expiration_minutes":   "45",
				"service_charge_pct":         "10",
				"require_manager_pin_for_void": "true",
				"max_discount_pct_staff":     "5",
			},
		},
		{
			Name:        "bar",
			Description: "Bar with food service",
			Settings: map[string]string{
				"auto_kot_firing":            "true",
				"table_expiration_minutes":   "90",
				"service_charge_pct":         "15",
				"enable_happy_hour":          "true",
				"happy_hour_discount":        "25",
				"require_age_verification":   "true",
			},
		},
	}
}
package models

import (
	"time"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ============================================
// TENANT SETTINGS (Hierarchy of Power)
// ============================================

type TenantSetting struct {
	SettingID   uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"setting_id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_tenant_key" json:"tenant_id"`
	Key         string    `gorm:"type:varchar(100);not null;uniqueIndex:idx_tenant_key;index" json:"key"`
	Value       string    `gorm:"type:text;not null" json:"value"`
	Domain      string    `gorm:"type:varchar(50);not null;index" json:"domain"` // Financial, Operational, Display, Security
	DataType    string    `gorm:"type:varchar(20);not null" json:"data_type"`    // string, int, float, bool, json
	
	// RBAC Integration
	MinRoleLevel  int         `gorm:"not null;default:1" json:"min_role_level"`  // 1=Staff, 5=Manager, 10=Owner, 99=SuperAdmin
	RequiredPermission string  `gorm:"type:varchar(100)" json:"required_permission"` // e.g., "setting:tax:write"
	IsSystemLocked bool       `gorm:"not null;default:false" json:"is_system_locked"` // Cannot be changed via UI
	
	// Audit Trail
	ModifiedBy  uuid.UUID  `gorm:"type:uuid;not null" json:"modified_by"`
	ModifiedAt  time.Time  `gorm:"not null;default:now()" json:"modified_at"`
	PreviousValue *string  `gorm:"type:text" json:"previous_value,omitempty"`
	
	// Metadata
	Description string     `gorm:"type:text" json:"description"`
	ValidationRule string  `gorm:"type:text" json:"validation_rule,omitempty"` // JSON schema or regex
	DefaultValue string    `gorm:"type:text" json:"default_value"`
	
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	
	// Relationships
	Tenant       Tenant `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	ModifiedByUser User `gorm:"foreignKey:ModifiedBy;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"-"`
}

func (ts *TenantSetting) BeforeCreate(tx *gorm.DB) error {
	if ts.SettingID == uuid.Nil {
		ts.SettingID = uuid.New()
	}
	return nil
}

// ============================================
// RESTAURANT PROFILE (Advanced Settings)
// ============================================

type RestaurantProfile struct {
	ProfileID    uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"profile_id"`
	RestaurantID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"restaurant_id"`
	TenantID     uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	
	// Financial Settings
	CurrencyCode           string  `gorm:"type:varchar(3);not null;default:'USD'" json:"currency_code"`           // ISO 4217
	TaxRegistrationNumber  string  `gorm:"type:varchar(100)" json:"tax_registration_number"`                      // VAT/PAN/TIN
	ServiceChargePct       float64 `gorm:"type:decimal(5,2);default:0" json:"service_charge_pct"`                 // Auto-tip %
	TaxInclusivePricing    bool    `gorm:"not null;default:false" json:"tax_inclusive_pricing"`                   // Owner-only setting
	DefaultTaxRate         float64 `gorm:"type:decimal(5,2);default:0" json:"default_tax_rate"`                   // Default tax %
	AlcoholTaxRate         float64 `gorm:"type:decimal(5,2);default:0" json:"alcohol_tax_rate"`                   // Separate alcohol tax
	
	// Operational Settings
	AutoKOTFiring          bool `gorm:"not null;default:true" json:"auto_kot_firing"`                             // Order → Kitchen automatically
	TableExpirationMinutes int  `gorm:"default:30" json:"table_expiration_minutes"`                               // Auto-release idle tables
	StockWarningThreshold  int  `gorm:"default:10" json:"stock_warning_threshold"`                                // Inventory alert level
	EnableHappyHour        bool `gorm:"not null;default:false" json:"enable_happy_hour"`                          // Manager can toggle
	HappyHourStart         *time.Time `json:"happy_hour_start,omitempty"`                                          // Time of day
	HappyHourEnd           *time.Time `json:"happy_hour_end,omitempty"`                                            // Time of day
	HappyHourDiscount      float64 `gorm:"type:decimal(5,2);default:0" json:"happy_hour_discount"`                // Discount %
	
	// Display & Branding
	LanguageISO          string `gorm:"type:varchar(5);default:'en'" json:"language_iso"`                         // ISO 639-1
	BrandPrimaryColor    string `gorm:"type:varchar(7);default:'#000000'" json:"brand_primary_color"`             // Hex color
	BrandSecondaryColor  string `gorm:"type:varchar(7);default:'#FFFFFF'" json:"brand_secondary_color"`           // Hex color
	LogoURL              string `gorm:"type:text" json:"logo_url,omitempty"`
	ReceiptHeader        string `gorm:"type:text" json:"receipt_header,omitempty"`                                // Custom receipt text
	ReceiptFooter        string `gorm:"type:text" json:"receipt_footer,omitempty"`                                // Thank you message
	
	// Security Settings
	RequireManagerPINForVoid     bool `gorm:"not null;default:true" json:"require_manager_pin_for_void"`
	RequireManagerPINForDiscount bool `gorm:"not null;default:true" json:"require_manager_pin_for_discount"`
	MaxDiscountPctStaff          float64 `gorm:"type:decimal(5,2);default:0" json:"max_discount_pct_staff"`        // Staff limit
	MaxDiscountPctManager        float64 `gorm:"type:decimal(5,2);default:20" json:"max_discount_pct_manager"`     // Manager limit
	OffsiteLoginAllowed          bool `gorm:"not null;default:false" json:"offsite_login_allowed"`                 // IP restriction toggle
	AllowedIPRanges              []byte `gorm:"type:jsonb" json:"allowed_ip_ranges,omitempty"`                      // Array of CIDR blocks
	
	// Compliance & Legal
	GDPREnabled              bool   `gorm:"not null;default:false" json:"gdpr_enabled"`
	DataRetentionDays        int    `gorm:"default:2555" json:"data_retention_days"`                              // 7 years default
	RequireAgeVerification   bool   `gorm:"not null;default:false" json:"require_age_verification"`               // For alcohol sales
	
	// Metadata
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	
	// Relationships
	Restaurant Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Tenant     Tenant     `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

func (rp *RestaurantProfile) BeforeCreate(tx *gorm.DB) error {
	if rp.ProfileID == uuid.Nil {
		rp.ProfileID = uuid.New()
	}
	return nil
}

// ============================================
// USER PROFILE (Role Metadata & Permissions)
// ============================================

type UserProfile struct {
	ProfileID       uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"profile_id"`
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID    *uuid.UUID `gorm:"type:uuid;index" json:"restaurant_id,omitempty"`                         // Null for tenant-level users
	
	// Profile Type & Hierarchy
	ProfileType     string `gorm:"type:varchar(50);not null;index" json:"profile_type"`                       // Owner, Manager, Staff
	RoleLevel       int    `gorm:"not null;default:1;index" json:"role_level"`                                // 1=Staff, 5=Manager, 10=Owner, 99=SuperAdmin
	
	// Employment Details
	EmployeeNumber  string    `gorm:"type:varchar(50);uniqueIndex:idx_tenant_employee" json:"employee_number,omitempty"`
	Department      string    `gorm:"type:varchar(100)" json:"department,omitempty"`                          // Kitchen, FrontOfHouse, Management
	JobTitle        string    `gorm:"type:varchar(100)" json:"job_title,omitempty"`
	HireDate        *time.Time `json:"hire_date,omitempty"`
	TerminationDate *time.Time `json:"termination_date,omitempty"`
	EmploymentStatus string   `gorm:"type:varchar(50);default:'active'" json:"employment_status"`            // active, terminated, suspended
	
	// Compensation (Encrypted in production)
	HourlyRate      *float64 `gorm:"type:decimal(10,2)" json:"hourly_rate,omitempty"`
	MonthlySalary   *float64 `gorm:"type:decimal(10,2)" json:"monthly_salary,omitempty"`
	CommissionRate  *float64 `gorm:"type:decimal(5,2)" json:"commission_rate,omitempty"`
	
	// Schedule & Attendance
	ShiftPattern    string    `gorm:"type:varchar(100)" json:"shift_pattern,omitempty"`                       // "Mon-Fri 9-5", "Rotating"
	DefaultShift    string    `gorm:"type:varchar(50)" json:"default_shift,omitempty"`                        // morning, afternoon, evening, night
	WeeklyHours     int       `gorm:"default:40" json:"weekly_hours"`
	
	// Security & Access Control
	CanLoginOffsite     bool   `gorm:"not null;default:false" json:"can_login_offsite"`                       // Owners=true, Staff=false
	RequiresPINForActions bool `gorm:"not null;default:false" json:"requires_pin_for_actions"`                // Manager PIN approval
	ManagerPIN          string `gorm:"type:varchar(255)" json:"-"`                                            // Hashed 4-digit PIN (hidden)
	
	// Permission Overrides (Temporary Elevation)
	TemporaryElevation       bool       `gorm:"not null;default:false" json:"temporary_elevation"`
	ElevationExpiresAt       *time.Time `json:"elevation_expires_at,omitempty"`
	ElevationGrantedBy       *uuid.UUID `gorm:"type:uuid" json:"elevation_granted_by,omitempty"`
	ElevationReason          string     `gorm:"type:varchar(500)" json:"elevation_reason,omitempty"`
	
	// Performance & Metrics
	TotalOrdersProcessed int       `gorm:"default:0" json:"total_orders_processed"`
	AverageOrderValue    float64   `gorm:"type:decimal(10,2);default:0" json:"average_order_value"`
	LastPerformanceReview *time.Time `json:"last_performance_review,omitempty"`
	PerformanceRating    *float64  `gorm:"type:decimal(3,2)" json:"performance_rating,omitempty"`             // 0.00 - 5.00
	
	// Preferences
	PreferredLanguage string `gorm:"type:varchar(5);default:'en'" json:"preferred_language"`
	NotificationPreferences []byte `gorm:"type:jsonb" json:"notification_preferences,omitempty"`
	
	// Metadata
	Notes     string    `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	
	// Relationships
	User       User        `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Tenant     Tenant      `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Restaurant *Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

func (up *UserProfile) BeforeCreate(tx *gorm.DB) error {
	if up.ProfileID == uuid.Nil {
		up.ProfileID = uuid.New()
	}
	return nil
}

// ============================================
// SETTING OVERRIDE (Hierarchical Override System)
// ============================================

type SettingOverride struct {
	OverrideID   uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"override_id"`
	TenantID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID *uuid.UUID `gorm:"type:uuid;index" json:"restaurant_id,omitempty"`                         // Null = tenant-level override
	Key          string     `gorm:"type:varchar(100);not null;uniqueIndex:idx_override_key" json:"key"`
	Value        string     `gorm:"type:text;not null" json:"value"`
	
	// RBAC Protection
	MinRoleLevel       int    `gorm:"not null;default:1" json:"min_role_level"`                              // Who can change this?
	RequiredPermission string `gorm:"type:varchar(100)" json:"required_permission,omitempty"`
	
	// Override Hierarchy (Restaurant > Tenant > System)
	Priority       int       `gorm:"not null;default:0" json:"priority"`                                     // Higher = wins
	InheritFromParent bool   `gorm:"not null;default:true" json:"inherit_from_parent"`                      // Use tenant default if no override
	
	// Temporal Settings (Time-based overrides)
	ActiveFrom     *time.Time `json:"active_from,omitempty"`                                                 // Happy hour start
	ActiveUntil    *time.Time `json:"active_until,omitempty"`                                                // Happy hour end
	DaysOfWeek     []byte     `gorm:"type:jsonb" json:"days_of_week,omitempty"`                              // ["Mon", "Tue", "Wed"]
	
	// Audit
	CreatedBy  uuid.UUID `gorm:"type:uuid;not null" json:"created_by"`
	ModifiedBy uuid.UUID `gorm:"type:uuid;not null" json:"modified_by"`
	Reason     string    `gorm:"type:text" json:"reason,omitempty"`                                         // Why was this changed?
	
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	
	// Relationships
	Tenant       Tenant      `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Restaurant   *Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Creator      User        `gorm:"foreignKey:CreatedBy;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"-"`
	Modifier     User        `gorm:"foreignKey:ModifiedBy;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"-"`
}

func (so *SettingOverride) BeforeCreate(tx *gorm.DB) error {
	if so.OverrideID == uuid.Nil {
		so.OverrideID = uuid.New()
	}
	return nil
}

// ============================================
// MANAGER ELEVATION TOKEN (Short-Lived Power)
// ============================================

type ElevationToken struct {
	TokenID      uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"token_id"`
	UserID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`                                // Staff who requested
	ManagerID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"manager_id"`                             // Manager who approved
	TenantID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID uuid.UUID  `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	
	// Token Details
	TokenHash        string    `gorm:"type:varchar(255);uniqueIndex;not null" json:"-"`                    // Hashed token (never store plaintext)
	Permission       string    `gorm:"type:varchar(100);not null" json:"permission"`                       // "manager:elevate", "void:approve"
	ExpiresAt        time.Time `gorm:"not null;index" json:"expires_at"`                                   // 60 seconds from creation
	IsUsed           bool      `gorm:"not null;default:false" json:"is_used"`
	UsedAt           *time.Time `json:"used_at,omitempty"`
	
	// Context
	RequestReason    string    `gorm:"type:text" json:"request_reason,omitempty"`                          // "Customer complaint - apply discount"
	OrderID          *uuid.UUID `gorm:"type:uuid" json:"order_id,omitempty"`                               // Which order needs elevation?
	DiscountAmount   *float64  `gorm:"type:decimal(10,2)" json:"discount_amount,omitempty"`
	
	// Audit
	IPAddress    string    `gorm:"type:varchar(45)" json:"ip_address"`
	DeviceID     string    `gorm:"type:varchar(255)" json:"device_id"`
	CreatedAt    time.Time `gorm:"autoCreateTime;index" json:"created_at"`
	
	// Relationships
	User       User       `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Manager    User       `gorm:"foreignKey:ManagerID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Tenant     Tenant     `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Restaurant Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

func (et *ElevationToken) BeforeCreate(tx *gorm.DB) error {
	if et.TokenID == uuid.Nil {
		et.TokenID = uuid.New()
	}
	return nil
}

// ============================================
// SETTING AUDIT LOG (Complete History)
// ============================================

type SettingAuditLog struct {
	AuditID      uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"audit_id"`
	TenantID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID *uuid.UUID `gorm:"type:uuid;index" json:"restaurant_id,omitempty"`
	
	// What Changed
	SettingKey    string `gorm:"type:varchar(100);not null;index" json:"setting_key"`
	Domain        string `gorm:"type:varchar(50);not null;index" json:"domain"`
	OldValue      string `gorm:"type:text" json:"old_value"`
	NewValue      string `gorm:"type:text" json:"new_value"`
	
	// Who & When
	ChangedBy     uuid.UUID `gorm:"type:uuid;not null;index" json:"changed_by"`
	ChangedAt     time.Time `gorm:"not null;index" json:"changed_at"`
	
	// Context
	Reason        string `gorm:"type:text" json:"reason,omitempty"`
	IPAddress     string `gorm:"type:varchar(45)" json:"ip_address"`
	UserAgent     string `gorm:"type:text" json:"user_agent"`
	RoleLevel     int    `gorm:"not null" json:"role_level"`                                               // What level was the actor?
	WasElevated   bool   `gorm:"not null;default:false" json:"was_elevated"`                               // Did they use a manager token?
	
	// Security
	RequiredApproval bool      `gorm:"not null;default:false" json:"required_approval"`
	ApprovedBy       *uuid.UUID `gorm:"type:uuid" json:"approved_by,omitempty"`
	ApprovedAt       *time.Time `json:"approved_at,omitempty"`
	
	// Relationships
	Tenant       Tenant      `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Restaurant   *Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Actor        User        `gorm:"foreignKey:ChangedBy;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"-"`
	Approver     *User       `gorm:"foreignKey:ApprovedBy;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"-"`
}

func (sal *SettingAuditLog) BeforeCreate(tx *gorm.DB) error {
	if sal.AuditID == uuid.Nil {
		sal.AuditID = uuid.New()
	}
	return nil
}

// ============================================
// ROLE LEVEL CONSTANTS (For Reference)
// ============================================

const (
	RoleLevelStaff      = 1
	RoleLevelShiftLead  = 3
	RoleLevelManager    = 5
	RoleLevelSeniorMgr  = 7
	RoleLevelOwner      = 10
	RoleLevelSuperAdmin = 99
)

// ============================================
// SETTING DOMAINS (For Reference)
// ============================================

const (
	DomainFinancial   = "financial"
	DomainOperational = "operational"
	DomainDisplay     = "display"
	DomainSecurity    = "security"
	DomainCompliance  = "compliance"
	DomainIntegration = "integration"
)
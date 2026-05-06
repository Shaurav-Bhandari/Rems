package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ============================================
// ROLE MODEL (UNCHANGED)
// ============================================

type Role struct {
	RoleID      uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"role_id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RoleName    string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_tenant_role_name" json:"role_name"`
	Description string    `gorm:"type:text" json:"description"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// ========== RBAC FIELDS ==========

	// Name is a read-only alias for RoleName — rbac_service.go uses role.Name
	Name string `gorm:"type:varchar(255);->" json:"name"`

	// IsSystem marks roles that cannot be deleted (superadmin, etc.)
	IsSystem bool `gorm:"not null;default:false" json:"is_system"`

	// ParentRoleIDs enables role inheritance — resolved recursively by RoleResolver
	ParentRoleIDs []uuid.UUID `gorm:"type:jsonb;serializer:json" json:"parent_role_ids,omitempty"`

	// Relationships
	Tenant   Tenant   `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Policies []Policy `gorm:"many2many:role_policies;joinForeignKey:RoleID;joinReferences:PolicyID" json:"policies,omitempty"`
}

// AfterFind syncs Name from RoleName so rbac_service.go can use role.Name
func (r *Role) AfterFind(tx *gorm.DB) error {
	r.Name = r.RoleName
	return nil
}

// ============================================
// USER MODEL (UPDATED WITH AUTH FIELDS)
// ============================================

type User struct {
	// ========== YOUR EXISTING FIELDS (UNCHANGED) ==========
	UserID         uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"user_id"`
	UserName       string         `gorm:"type:varchar(255);not null;uniqueIndex:idx_tenant_user_name" json:"user_name"`
	FullName       string         `gorm:"type:varchar(255)" json:"full_name"`
	Email          string         `gorm:"type:varchar(255);not null;uniqueIndex:idx_tenant_email" json:"email"`
	Phone          string         `gorm:"type:varchar(20)" json:"phone"`
	PasswordHash   string         `gorm:"type:varchar(255);not null" json:"-"`
	IsActive       bool           `gorm:"not null;default:true" json:"is_active"`
	IsDeleted      bool           `gorm:"not null;default:false" json:"is_deleted"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;index" json:"organization_id,omitempty"`
	BranchID       uuid.UUID      `gorm:"type:uuid;index" json:"branch_id,omitempty"`
	TenantID       uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_tenant_email,idx_tenant_username" json:"tenant_id"`
	DefaultRoleId  uuid.UUID      `gorm:"type:uuid;index" json:"default_role_id,omitempty"`
	CreatedAt      time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	DeletedBy      *uuid.UUID     `gorm:"type:uuid" json:"deleted_by,omitempty"`

	// ========== NEW AUTH SECURITY FIELDS ==========
	
	// Role-based token TTL
	PrimaryRole string `gorm:"type:varchar(50);index" json:"primary_role"`
	
	// Email verification (IMPROVEMENT #10)
	IsEmailVerified bool       `gorm:"not null;default:false" json:"is_email_verified"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	
	// Two-Factor Authentication
	TwoFactorEnabled bool   `gorm:"not null;default:false" json:"two_factor_enabled"`
	TwoFactorSecret  string `gorm:"type:varchar(255)" json:"-"`     // Hidden from JSON
	BackupCodes      []byte `gorm:"type:jsonb" json:"-"`            // Hidden from JSON
	
	// Account security (IMPROVEMENT #18: Escalating lockouts)
	FailedLoginAttempts int        `gorm:"default:0" json:"failed_login_attempts"`
	LockedUntil         *time.Time `json:"locked_until,omitempty"`
	
	// Login tracking
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP       string     `gorm:"type:varchar(45)" json:"last_login_ip,omitempty"`
	LastLoginLocation string     `gorm:"type:varchar(255)" json:"last_login_location,omitempty"`
	
	// Password management
	PasswordChangedAt  *time.Time `json:"password_changed_at,omitempty"`
	MustChangePassword bool       `gorm:"default:false" json:"must_change_password"`

	// ========== GOOGLE OAUTH FIELDS ==========
	GoogleID  string `gorm:"type:varchar(255);index" json:"google_id,omitempty"`
	AvatarURL string `gorm:"type:text" json:"avatar_url,omitempty"`

	// ========== YOUR EXISTING RELATIONSHIPS (UNCHANGED) ==========
	Tenant       Tenant       `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Organization Organization `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"-"`
	Branch       Branch       `gorm:"foreignKey:OrganizationID;references:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"-"`
	DefaultRole  Role         `gorm:"foreignKey:DefaultRoleId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"-"`
	Roles        []Role       `gorm:"many2many:user_roles;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"roles,omitempty"`
}

// ============================================
// USER ROLE (UNCHANGED)
// ============================================

type UserRole struct {
	UserRoleID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"user_role_id"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_role_tenant" json:"user_id"`
	RoleID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_role_tenant" json:"role_id"`
	AssignedAt time.Time `gorm:"autoCreateTime" json:"assigned_at"`

	// ========== RBAC FIELDS ==========

	// TenantID scopes role assignments — rbac_service queries on this
	TenantID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_role_tenant;index" json:"tenant_id"`

	// AssignedBy tracks the actor who granted this role (privilege escalation audit)
	AssignedBy uuid.UUID `gorm:"type:uuid" json:"assigned_by"`

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Role Role `gorm:"foreignKey:RoleID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

// ============================================
// NOTE: PasswordHistory is defined in auth.go
// ============================================

// ============================================
// NEW: REFRESH TOKEN MODEL
// ============================================

type RefreshToken struct {
	TokenID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"token_id"`
	UserID            uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	TokenHash         string     `gorm:"type:varchar(255);uniqueIndex;not null" json:"-"`
	DeviceID          string     `gorm:"type:varchar(255);index" json:"device_id"`
	DeviceFingerprint string     `gorm:"type:text" json:"device_fingerprint"`
	DeviceType        string     `gorm:"type:varchar(50)" json:"device_type"`
	UserRole          string     `gorm:"type:varchar(50)" json:"user_role"`
	IPAddress         string     `gorm:"type:varchar(45)" json:"ip_address"`
	UserAgent         string     `gorm:"type:text" json:"user_agent"`
	Location          string     `gorm:"type:varchar(255)" json:"location"`
	ExpiresAt         time.Time  `gorm:"not null;index" json:"expires_at"`
	LastUsedAt        *time.Time `json:"last_used_at,omitempty"`
	IsRevoked         bool       `gorm:"not null;default:false;index" json:"is_revoked"`
	IsRotated         bool       `gorm:"not null;default:false" json:"is_rotated"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	RevokedReason     string     `gorm:"type:varchar(255)" json:"revoked_reason,omitempty"`
	ParentTokenID     *uuid.UUID `gorm:"type:uuid" json:"parent_token_id,omitempty"`
	CreatedAt         time.Time  `gorm:"autoCreateTime" json:"created_at"`

	User User `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

// ============================================
// NEW: DEVICE REGISTRY MODEL
// ============================================

type DeviceRegistry struct {
	DeviceID          uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"device_id"`
	UserID            uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	DeviceFingerprint string    `gorm:"type:text;not null;index" json:"device_fingerprint"`
	DeviceName        string    `gorm:"type:varchar(255)" json:"device_name"`
	IsTrusted         bool      `gorm:"not null;default:false" json:"is_trusted"`
	LoginCount        int       `gorm:"default:0" json:"login_count"`
	FirstSeenAt       time.Time `gorm:"autoCreateTime" json:"first_seen_at"`
	LastSeenAt        time.Time `gorm:"not null" json:"last_seen_at"`
	LastSeenIP        string    `gorm:"type:varchar(45)" json:"last_seen_ip"`
	LastSeenLocation  string    `gorm:"type:varchar(255)" json:"last_seen_location"`

	User User `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

// ============================================
// NEW: SECURITY EVENT MODEL
// ============================================

type SecurityEvent struct {
	EventID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"event_id"`
	UserID            *uuid.UUID `gorm:"type:uuid;index" json:"user_id,omitempty"`
	EventType         string     `gorm:"type:varchar(100);not null;index" json:"event_type"`
	EventSeverity     string     `gorm:"type:varchar(50)" json:"event_severity"`
	IPAddress         string     `gorm:"type:varchar(45);index" json:"ip_address"`
	UserAgent         string     `gorm:"type:text" json:"user_agent"`
	Location          string     `gorm:"type:varchar(255)" json:"location"`
	DeviceFingerprint string     `gorm:"type:text" json:"device_fingerprint,omitempty"`
	Success           bool       `gorm:"not null" json:"success"`
	FailureReason     string     `gorm:"type:text" json:"failure_reason,omitempty"`
	Metadata          []byte     `gorm:"type:jsonb" json:"metadata,omitempty"`
	PreviousHash      string     `gorm:"type:varchar(255)" json:"previous_hash,omitempty"`
	CurrentHash       string     `gorm:"type:varchar(255)" json:"current_hash,omitempty"`
	Timestamp         time.Time  `gorm:"autoCreateTime;index" json:"timestamp"`

	User *User `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"-"`
}

// ============================================
// NEW: EMAIL VERIFICATION TOKEN MODEL
// ============================================

type EmailVerificationToken struct {
	TokenID   uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"token_id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Token     string     `gorm:"type:varchar(255);uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time  `gorm:"not null;index" json:"expires_at"`
	IsUsed    bool       `gorm:"not null;default:false" json:"is_used"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`

	User User `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

// ============================================
// NEW: PASSWORD RESET TOKEN MODEL
// ============================================

type PasswordResetToken struct {
	TokenID   uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"token_id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Token     string     `gorm:"type:varchar(255);uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time  `gorm:"not null;index" json:"expires_at"`
	IsUsed    bool       `gorm:"not null;default:false" json:"is_used"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	IPAddress string     `gorm:"type:varchar(45)" json:"ip_address,omitempty"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`

	User User `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

// ============================================
// GORM HOOKS (EXISTING + NEW)
// ============================================

func (r *Role) BeforeCreate(tx *gorm.DB) (err error) {
	if r.RoleID == uuid.Nil {
		r.RoleID = uuid.New()
	}
	return nil
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.UserID == uuid.Nil {
		u.UserID = uuid.New()
	}
	return nil
}

func (ur *UserRole) BeforeCreate(tx *gorm.DB) (err error) {
	if ur.UserRoleID == uuid.Nil {
		ur.UserRoleID = uuid.New()
	}
	return nil
}

// NEW HOOKS
// Note: PasswordHistory struct and its BeforeCreate hook live in auth.go

func (rt *RefreshToken) BeforeCreate(tx *gorm.DB) (err error) {
	if rt.TokenID == uuid.Nil {
		rt.TokenID = uuid.New()
	}
	return nil
}

func (dr *DeviceRegistry) BeforeCreate(tx *gorm.DB) (err error) {
	if dr.DeviceID == uuid.Nil {
		dr.DeviceID = uuid.New()
	}
	return nil
}

func (se *SecurityEvent) BeforeCreate(tx *gorm.DB) (err error) {
	if se.EventID == uuid.Nil {
		se.EventID = uuid.New()
	}
	return nil
}

func (evt *EmailVerificationToken) BeforeCreate(tx *gorm.DB) (err error) {
	if evt.TokenID == uuid.Nil {
		evt.TokenID = uuid.New()
	}
	return nil
}

func (prt *PasswordResetToken) BeforeCreate(tx *gorm.DB) (err error) {
	if prt.TokenID == uuid.Nil {
		prt.TokenID = uuid.New()
	}
	return nil
}

// func (ak *APIKey) BeforeCreate(tx *gorm.DB) (err error) {
// 	if ak.APIKeyID == uuid.Nil {
// 		ak.APIKeyID = uuid.New()
// 	}
// 	return nil
// }
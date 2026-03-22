package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Admin is a platform-level superadmin — no tenant, org, or branch.
// These are the humans who operate the ReMS platform itself.
type Admin struct {
	AdminID   uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"admin_id"`
	UserName  string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"user_name"`
	FullName  string         `gorm:"type:varchar(255);not null" json:"full_name"`
	Email     string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"email"`
	Phone     string         `gorm:"type:varchar(20)" json:"phone,omitempty"`
	Password  string         `gorm:"type:varchar(255);not null" json:"-"`
	IsActive  bool           `gorm:"not null;default:true" json:"is_active"`
	IsDeleted bool           `gorm:"not null;default:false" json:"is_deleted"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Security
	FailedLoginAttempts int        `gorm:"default:0" json:"failed_login_attempts"`
	LockedUntil         *time.Time `json:"locked_until,omitempty"`
	LastLoginAt         *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP         string     `gorm:"type:varchar(45)" json:"last_login_ip,omitempty"`
	IsEmailVerified     bool       `gorm:"not null;default:false" json:"is_email_verified"`
	EmailVerifiedAt     *time.Time `json:"email_verified_at,omitempty"`
	MustChangePassword  bool       `gorm:"default:false" json:"must_change_password"`
	PasswordChangedAt   *time.Time `json:"password_changed_at,omitempty"`
}

func (a *Admin) BeforeCreate(tx *gorm.DB) error {
	if a.AdminID == uuid.Nil {
		a.AdminID = uuid.New()
	}
	return nil
}
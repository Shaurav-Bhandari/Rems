package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Session struct {
	SessionID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"session_id"`
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	// Security metadata
	IP        string    `gorm:"type:inet" json:"ip"`
	UserAgent string    `gorm:"type:text" json:"user_agent"`
	Revoked   bool      `gorm:"not null;default:false" json:"revoked"`

	CreatedAt time.Time `gorm:"not null;default:now()" json:"created_at"`

	// Relations
	User   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

type PasswordReset struct {
	ResetID   uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	TokenHash string    `gorm:"type:char(64);not null;index"`
	ExpiresAt time.Time `gorm:"not null;index"`
	Used      bool      `gorm:"not null;default:false"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

// BeforeCreate hooks
func (s *Session) BeforeCreate(tx *gorm.DB) error {
	if s.SessionID == uuid.Nil {
		s.SessionID = uuid.New()
	}
	return nil
}

func (pr *PasswordReset) BeforeCreate(tx *gorm.DB) error {
	if pr.ResetID == uuid.Nil {
		pr.ResetID = uuid.New()
	}
	return nil
}
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// APIKey represents an API key for external access
type APIKey struct {
	APIKeyID    uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"api_key_id"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Key         string     `gorm:"type:varchar(500);not null;uniqueIndex" json:"key"`
	Description string     `gorm:"type:text" json:"description"`
	IsActive    bool       `gorm:"not null;default:true;index" json:"is_active"`
	CreatedAt   time.Time  `gorm:"not null;default:now()" json:"created_at"`
	ExpiresAt   *time.Time `gorm:"type:timestamptz" json:"expires_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// BeforeCreate hook
func (ak *APIKey) BeforeCreate(tx *gorm.DB) error {
	if ak.APIKeyID == uuid.Nil {
		ak.APIKeyID = uuid.New()
	}
	return nil
}
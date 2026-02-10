package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DataAccessControl represents fine-grained data access control
type DataAccessControl struct {
	DataAccessControlID  uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"data_access_control_id"`
	TenantID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	ResourceType         string     `gorm:"type:varchar(100)" json:"resource_type"`
	ResourceID           string     `gorm:"type:varchar(255)" json:"resource_id"`
	AccessLevel          string     `gorm:"type:varchar(50);not null" json:"access_level"`
	UserID               *uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	RoleID               *uuid.UUID `gorm:"type:uuid;index" json:"role_id"`
	RestaurantID         *uuid.UUID `gorm:"type:uuid;index" json:"restaurant_id"`
	AccessConditions     JSONB      `gorm:"type:jsonb;default:'{}'" json:"access_conditions"`
	ExpiresAt            *time.Time `gorm:"type:timestamptz" json:"expires_at"`
	IsActive             bool       `gorm:"not null;default:true;index" json:"is_active"`
	AllowedFields        JSONB      `gorm:"type:jsonb;default:'[]'" json:"allowed_fields"`
	DeniedFields         JSONB      `gorm:"type:jsonb;default:'[]'" json:"denied_fields"`
	AllowPIIAccess       bool       `gorm:"not null;default:false" json:"allow_pii_access"`
	AllowFinancialAccess bool       `gorm:"not null;default:false" json:"allow_financial_access"`
	CreatedAt            time.Time  `gorm:"not null;default:now()" json:"created_at"`
	CreatedBy            uuid.UUID  `gorm:"type:uuid;not null" json:"created_by"`

	// Relationships
	Tenant       Tenant      `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	User         *User       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
	Role         *Role       `gorm:"foreignKey:RoleID;constraint:OnDelete:CASCADE" json:"role,omitempty"`
	Restaurant   *Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"restaurant,omitempty"`
	CreatedByUser User       `gorm:"foreignKey:CreatedBy;constraint:OnDelete:RESTRICT" json:"-"`
}

// BeforeCreate hook
func (dac *DataAccessControl) BeforeCreate(tx *gorm.DB) error {
	if dac.DataAccessControlID == uuid.Nil {
		dac.DataAccessControlID = uuid.New()
	}
	return nil
}
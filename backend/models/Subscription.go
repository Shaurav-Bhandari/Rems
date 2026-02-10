package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Plan represents a subscription plan
type Plan struct {
	PlanID         uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"plan_id"`
	Name           string    `gorm:"type:varchar(255);not null" json:"name"`
	Description    string    `gorm:"type:text" json:"description"`
	Price          float64   `gorm:"type:decimal(10,2);not null" json:"price"`
	BillingCycle   string    `gorm:"type:varchar(50)" json:"billing_cycle"`
	MaxRestaurants *int      `gorm:"type:int" json:"max_restaurants"`
	MaxUsers       *int      `gorm:"type:int" json:"max_users"`
	IsActive       bool      `gorm:"not null;default:true;index" json:"is_active"`
	CreatedAt      time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

// PlanFeature represents a feature of a subscription plan
type PlanFeature struct {
	PlanFeatureID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"plan_feature_id"`
	PlanID        uuid.UUID `gorm:"type:uuid;not null;index" json:"plan_id"`
	FeatureName   string    `gorm:"type:varchar(255);not null" json:"feature_name"`
	FeatureValue  string    `gorm:"type:text" json:"feature_value"`
	IsEnabled     bool      `gorm:"not null;default:true" json:"is_enabled"`

	// Relationships
	Plan Plan `gorm:"foreignKey:PlanID;constraint:OnDelete:CASCADE" json:"-"`
}

// Subscription represents a tenant's subscription
type Subscription struct {
	SubscriptionID uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"subscription_id"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	PlanID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"plan_id"`
	StartDate      time.Time  `gorm:"not null" json:"start_date"`
	EndDate        *time.Time `gorm:"type:timestamptz" json:"end_date"`
	Status         string     `gorm:"type:varchar(50);index" json:"status"`
	AutoRenew      bool       `gorm:"not null;default:true" json:"auto_renew"`
	CreatedAt      time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"not null;default:now()" json:"updated_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Plan   Plan   `gorm:"foreignKey:PlanID;constraint:OnDelete:RESTRICT" json:"-"`
}

// TenantAddon represents an addon for a tenant
type TenantAddon struct {
	TenantAddonID uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"tenant_addon_id"`
	TenantID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	AddonName     string     `gorm:"type:varchar(255);not null" json:"addon_name"`
	Price         float64    `gorm:"type:decimal(10,2);not null" json:"price"`
	ActivatedAt   time.Time  `gorm:"not null" json:"activated_at"`
	ExpiresAt     *time.Time `gorm:"type:timestamptz" json:"expires_at"`
	IsActive      bool       `gorm:"not null;default:true;index" json:"is_active"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// BeforeCreate hooks
func (p *Plan) BeforeCreate(tx *gorm.DB) error {
	if p.PlanID == uuid.Nil {
		p.PlanID = uuid.New()
	}
	return nil
}

func (pf *PlanFeature) BeforeCreate(tx *gorm.DB) error {
	if pf.PlanFeatureID == uuid.Nil {
		pf.PlanFeatureID = uuid.New()
	}
	return nil
}

func (s *Subscription) BeforeCreate(tx *gorm.DB) error {
	if s.SubscriptionID == uuid.Nil {
		s.SubscriptionID = uuid.New()
	}
	return nil
}

func (ta *TenantAddon) BeforeCreate(tx *gorm.DB) error {
	if ta.TenantAddonID == uuid.Nil {
		ta.TenantAddonID = uuid.New()
	}
	return nil
}
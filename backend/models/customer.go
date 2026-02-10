package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Customer represents a restaurant customer
type Customer struct {
	CustomerID   uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"customer_id"`
	TenantID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	FirstName    string     `gorm:"type:varchar(255)" json:"first_name"`
	LastName     string     `gorm:"type:varchar(255)" json:"last_name"`
	Email        string     `gorm:"type:varchar(255);index" json:"email"`
	Phone        string     `gorm:"type:varchar(50);index" json:"phone"`
	Address      string     `gorm:"type:text" json:"address"`
	City         string     `gorm:"type:varchar(100)" json:"city"`
	State        string     `gorm:"type:varchar(100)" json:"state"`
	PostalCode   string     `gorm:"type:varchar(20)" json:"postal_code"`
	DateOfBirth  *time.Time `gorm:"type:timestamptz" json:"date_of_birth"`
	LoyaltyPoints int       `gorm:"default:0" json:"loyalty_points"`
	TotalOrders  int        `gorm:"default:0" json:"total_orders"`
	TotalSpent   float64    `gorm:"type:decimal(12,2);default:0" json:"total_spent"`
	IsActive     bool       `gorm:"not null;default:true;index" json:"is_active"`
	CreatedAt    time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"not null;default:now()" json:"updated_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// BeforeCreate hook
func (c *Customer) BeforeCreate(tx *gorm.DB) error {
	if c.CustomerID == uuid.Nil {
		c.CustomerID = uuid.New()
	}
	return nil
}
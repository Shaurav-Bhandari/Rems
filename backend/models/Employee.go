package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Employee represents a restaurant employee
type Employee struct {
	EmployeeID     uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"employee_id"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"restaurant_id"`
	UserID         *uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	FirstName      string     `gorm:"type:varchar(255);not null" json:"first_name"`
	LastName       string     `gorm:"type:varchar(255);not null" json:"last_name"`
	Email          string     `gorm:"type:varchar(255)" json:"email"`
	Phone          string     `gorm:"type:varchar(50)" json:"phone"`
	HireDate       *time.Time `gorm:"type:timestamptz" json:"hire_date"`
	TerminationDate *time.Time `gorm:"type:timestamptz" json:"termination_date"`
	Position       string     `gorm:"type:varchar(100)" json:"position"`
	Department     string     `gorm:"type:varchar(100)" json:"department"`
	HourlyRate     *float64   `gorm:"type:decimal(10,2)" json:"hourly_rate"`
	IsActive       bool       `gorm:"not null;default:true;index" json:"is_active"`
	CreatedAt      time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"not null;default:now()" json:"updated_at"`

	// Relationships
	Tenant     Tenant     `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
	User       *User      `gorm:"foreignKey:UserID;constraint:OnDelete:SET NULL" json:"user,omitempty"`
	Roles      []Role     `gorm:"many2many:employee_roles;" json:"roles,omitempty"`
}

// EmployeeRole represents the many-to-many relationship between employees and roles
type EmployeeRole struct {
	EmployeeRoleID uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"employee_role_id"`
	EmployeeID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"employee_id"`
	RoleID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"role_id"`
	AssignedAt     time.Time  `gorm:"not null;default:now()" json:"assigned_at"`
	AssignedBy     *uuid.UUID `gorm:"type:uuid" json:"assigned_by"`

	// Relationships
	Employee   Employee `gorm:"foreignKey:EmployeeID;constraint:OnDelete:CASCADE" json:"-"`
	Role       Role     `gorm:"foreignKey:RoleID;constraint:OnDelete:CASCADE" json:"-"`
	AssignedByUser *User `gorm:"foreignKey:AssignedBy;constraint:OnDelete:SET NULL" json:"assigned_by_user,omitempty"`
}

// BeforeCreate hooks
func (e *Employee) BeforeCreate(tx *gorm.DB) error {
	if e.EmployeeID == uuid.Nil {
		e.EmployeeID = uuid.New()
	}
	return nil
}

func (er *EmployeeRole) BeforeCreate(tx *gorm.DB) error {
	if er.EmployeeRoleID == uuid.Nil {
		er.EmployeeRoleID = uuid.New()
	}
	return nil
}
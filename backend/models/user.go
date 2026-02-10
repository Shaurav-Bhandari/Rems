package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Role struct {
	RoleID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"role_id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RoleName string `gorm:"type:varchar(255);not null;uniqueIndex:idx_tenant_role_name" json:"role_name"`
	Description string `gorm:"type:text" json:"description"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	//Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

type User struct {
	UserID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"user_id"`
	UserName string `gorm:"type:varchar(255);not null;uniqueIndex:idx_tenant_user_name" json:"user_name"`
	FullName string `gorm:"type:varchar(255)" json:"full_name"`
	Email string `gorm:"type:varchar(255);not null;uniqueIndex:idx_tenant_email" json:"email"`
	Phone string `gorm:"type:varchar(20)" json:"phone"`
	PasswordHash string `gorm:"type:varchar(255);not null" json:"-"`
	IsActive bool `gorm:"not null;default:true" json:"is_active"`
	IsDeleted bool `gorm:"not null;default:false" json:"is_deleted"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty"`
	BranchID uuid.UUID `gorm:"type:uuid;index" json:"branch_id,omitempty"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_tenant_email,idx_tenant_username" json:"tenant_id"`
	DefaultRoleId uuid.UUID `gorm:"type:uuid;index" json:"default_role_id,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	DeletedBy *uuid.UUID `gorm:"type:uuid" json:"deleted_by,omitempty"`

	//Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Organization Organization `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"-"`
	Branch Branch `gorm:"foreignKey:OrganizationID;references:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"-"`
	DefaultRole Role `gorm:"foreignKey:DefaultRoleId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"-"`
	Roles []Role `gorm:"many2many:user_roles;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"roles,omitempty"`
}


type UserRole struct {
	UserRoleID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"user_role_id"`
	UserID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_role" json:"user_id"`
	RoleID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_role" json:"role_id"`
	AssignedAt time.Time `gorm:"autoCreateTime" json:"assigned_at"`

	//Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Role Role `gorm:"foreignKey:RoleID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

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

package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Tenant struct {
	TenantID 	uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"tenant_id"`
	Name    	string    `gorm:"type:varchar(255);not null" json:"name"`
	Status  	string    `gorm:"type:varchar(50);uniqueIndex" json:"status"`
	Domain 		string    `gorm:"type:varchar(255);" json:"domain"`
	IsActive 	bool      `gorm:"not null;default:true;index" json:"is_active"`
	CreatedAt 	time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt 	time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	CreatedBy 	*uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy 	*uuid.UUID `gorm:"type:uuid" json:"updated_by"`
}


type Organization struct {
	OrganizationID 	uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"organization_id"`
	TenantID 		uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name    		string    `gorm:"type:varchar(255);not null" json:"name"`
	Description 	string    `gorm:"type:text" json:"description"`
	CreatedAt 		time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt 		time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	//Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

type Branch struct {
	BranchID 		uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"branch_id"`
	OrganizationID 	uuid.UUID `gorm:"type:uuid;not null;index" json:"organization_id"`
	TenantID 		uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name    		string    `gorm:"type:varchar(255);not null" json:"name"`
	Address 		string    `gorm:"type:text" json:"address"`
	City 			string    `gorm:"type:varchar(100)" json:"city"`
	State 			string    `gorm:"type:varchar(100)" json:"state"`
	Country 		string    `gorm:"type:varchar(100)" json:"country"`
	ZipCode 		string    `gorm:"type:varchar(20)" json:"zip_code"`
	PhoneNumber 	string    `gorm:"type:varchar(20)" json:"phone_number"`
	Email 			string    `gorm:"type:varchar(255)" json:"email"`
	CreatedAt 		time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt 		time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	//Relationships
	Organization Organization `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Tenant       Tenant       `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

func (t *Tenant) BeforeCreate(tx *gorm.DB) (err error) {
	if t.TenantID == uuid.Nil {
		t.TenantID = uuid.New()
	}
	return nil
}

func (o *Organization) BeforeCreate(tx *gorm.DB) (err error) {
	if o.OrganizationID == uuid.Nil {
		o.OrganizationID = uuid.New()
	}
	return nil
}

func (b *Branch) BeforeCreate(tx *gorm.DB) (err error) {
	if b.BranchID == uuid.Nil {
		b.BranchID = uuid.New()
	}
	return nil
}
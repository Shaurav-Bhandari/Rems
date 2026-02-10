package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Notification represents a user notification
type Notification struct {
	NotificationID   uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"notification_id"`
	TenantID         uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	UserID           *uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	Message          string    `gorm:"type:text;not null" json:"message"`
	NotificationType string    `gorm:"type:varchar(50)" json:"notification_type"`
	IsRead           bool      `gorm:"not null;default:false;index" json:"is_read"`
	CreatedAt        time.Time `gorm:"not null;default:now();index" json:"created_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	User   *User  `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
}

// Webhook represents a webhook configuration
type Webhook struct {
	WebhookID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"webhook_id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	URL       string    `gorm:"type:text;not null" json:"url"`
	EventType string    `gorm:"type:varchar(100);not null;index" json:"event_type"`
	Secret    string    `gorm:"type:varchar(255)" json:"secret"`
	IsActive  bool      `gorm:"not null;default:true;index" json:"is_active"`
	CreatedAt time.Time `gorm:"not null;default:now()" json:"created_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// BeforeCreate hooks
func (n *Notification) BeforeCreate(tx *gorm.DB) error {
	if n.NotificationID == uuid.Nil {
		n.NotificationID = uuid.New()
	}
	return nil
}

func (w *Webhook) BeforeCreate(tx *gorm.DB) error {
	if w.WebhookID == uuid.Nil {
		w.WebhookID = uuid.New()
	}
	return nil
}
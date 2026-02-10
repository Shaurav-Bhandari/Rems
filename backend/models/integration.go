package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Integration represents a third-party integration
type Integration struct {
	IntegrationID uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"integration_id"`
	TenantID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Provider      string     `gorm:"type:varchar(255)" json:"provider"`
	APIKey        string     `gorm:"type:varchar(500)" json:"api_key"`
	Endpoint      string     `gorm:"type:text" json:"endpoint"`
	IsActive      bool       `gorm:"default:true;index" json:"is_active"`
	CreatedAt     time.Time  `gorm:"default:now()" json:"created_at"`
	UpdatedAt     *time.Time `gorm:"default:now()" json:"updated_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// IntegrationLog represents a log entry for integration events
type IntegrationLog struct {
	LogID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"log_id"`
	TenantID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	BranchID        *uuid.UUID `gorm:"type:uuid;index" json:"branch_id"`
	IntegrationType string     `gorm:"type:varchar(100);not null;index" json:"integration_type"`
	Provider        string     `gorm:"type:varchar(255);not null" json:"provider"`
	EventType       string     `gorm:"type:varchar(100);index" json:"event_type"`
	EventTime       *time.Time `gorm:"type:timestamptz;index" json:"event_time"`
	Status          string     `gorm:"type:varchar(50);index" json:"status"`
	Request         string     `gorm:"type:text" json:"request"`
	Response        string     `gorm:"type:text" json:"response"`
	ErrorMessage    string     `gorm:"type:text" json:"error_message"`
	CorrelationID   string     `gorm:"type:varchar(255)" json:"correlation_id"`
	RetryCount      int        `gorm:"default:0" json:"retry_count"`
	NextRetryTime   *time.Time `gorm:"type:timestamptz" json:"next_retry_time"`
	Metadata        JSONB      `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	Duration        *int64     `gorm:"type:bigint" json:"duration"` // Duration in milliseconds
	IPAddress       string     `gorm:"type:varchar(50)" json:"ip_address"`
	CreatedAt       time.Time  `gorm:"default:now()" json:"created_at"`
	CreatedBy       string     `gorm:"type:varchar(255)" json:"created_by"`

	// Relationships
	Tenant Tenant  `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Branch *Branch `gorm:"foreignKey:BranchID;constraint:OnDelete:SET NULL" json:"branch,omitempty"`
}

// WebhookSubscription represents a webhook subscription
type WebhookSubscription struct {
	WebhookSubscriptionID uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"webhook_subscription_id"`
	TenantID              uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	URL                   string     `gorm:"type:text;not null" json:"url"`
	EventType             string     `gorm:"type:varchar(100);index" json:"event_type"`
	IsActive              bool       `gorm:"default:true;index" json:"is_active"`
	CreatedAt             time.Time  `gorm:"default:now()" json:"created_at"`
	LastSentAt            *time.Time `gorm:"type:timestamptz" json:"last_sent_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// WorkflowRule represents an automation rule
type WorkflowRule struct {
	RuleID    uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"rule_id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name      string    `gorm:"type:varchar(255);not null" json:"name"`
	RuleType  RuleType  `gorm:"type:varchar(50);index" json:"rule_type"`
	Condition string    `gorm:"type:text" json:"condition"`
	Action    string    `gorm:"type:text" json:"action"`
	IsActive  bool      `gorm:"default:true;index" json:"is_active"`
	CreatedAt time.Time `gorm:"default:now()" json:"created_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// BeforeCreate hooks
func (i *Integration) BeforeCreate(tx *gorm.DB) error {
	if i.IntegrationID == uuid.Nil {
		i.IntegrationID = uuid.New()
	}
	return nil
}

func (il *IntegrationLog) BeforeCreate(tx *gorm.DB) error {
	if il.LogID == uuid.Nil {
		il.LogID = uuid.New()
	}
	return nil
}

func (ws *WebhookSubscription) BeforeCreate(tx *gorm.DB) error {
	if ws.WebhookSubscriptionID == uuid.Nil {
		ws.WebhookSubscriptionID = uuid.New()
	}
	return nil
}

func (wr *WorkflowRule) BeforeCreate(tx *gorm.DB) error {
	if wr.RuleID == uuid.Nil {
		wr.RuleID = uuid.New()
	}
	return nil
}
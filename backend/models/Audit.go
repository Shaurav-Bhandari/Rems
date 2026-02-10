package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ActivityLog represents a user activity log
type ActivityLog struct {
	ActivityID uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"activity_id"`
	UserID     *uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	Action     string     `gorm:"type:varchar(255);not null" json:"action"`
	EntityName string     `gorm:"type:varchar(255);index" json:"entity_name"`
	EntityID   *uuid.UUID `gorm:"type:uuid" json:"entity_id"`
	Details    string     `gorm:"type:text" json:"details"`
	Timestamp  time.Time  `gorm:"not null;default:now();index" json:"timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID;constraint:OnDelete:SET NULL" json:"user,omitempty"`
}

// AuditTrail represents a comprehensive audit trail entry
type AuditTrail struct {
	AuditTrailID     uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"audit_trail_id"`
	TenantID         uuid.UUID      `gorm:"type:uuid;not null;index" json:"tenant_id"`
	UserID           *uuid.UUID     `gorm:"type:uuid;index" json:"user_id"`
	RestaurantID     *uuid.UUID     `gorm:"type:uuid;index" json:"restaurant_id"`
	EventType        AuditEvent 	`gorm:"type:varchar(50);index" json:"event_type"`
	EventCategory    string         `gorm:"type:varchar(100)" json:"event_category"`
	EventDescription string         `gorm:"type:text" json:"event_description"`
	Severity         AuditSeverity  `gorm:"type:varchar(50);not null;default:'Info';index" json:"severity"`
	EntityType       string         `gorm:"type:varchar(100)" json:"entity_type"`
	EntityID         string         `gorm:"type:varchar(255)" json:"entity_id"`
	OldValues        JSONB          `gorm:"type:jsonb;default:'{}'" json:"old_values"`
	NewValues        JSONB          `gorm:"type:jsonb;default:'{}'" json:"new_values"`
	RequestURL       string         `gorm:"type:text" json:"request_url"`
	HTTPMethod       string         `gorm:"type:varchar(10)" json:"http_method"`
	IPAddress        string         `gorm:"type:varchar(50)" json:"ip_address"`
	UserAgent        string         `gorm:"type:text" json:"user_agent"`
	SessionID        string         `gorm:"type:varchar(255)" json:"session_id"`
	Geolocation      JSONB          `gorm:"type:jsonb;default:'{}'" json:"geolocation"`
	RiskLevel        RiskLevel      `gorm:"type:varchar(50);not null;default:'Low';index" json:"risk_level"`
	RequiresReview   bool           `gorm:"not null;default:false;index" json:"requires_review"`
	IsAnomalous      bool           `gorm:"not null;default:false" json:"is_anomalous"`
	AnomalyReason    string         `gorm:"type:text" json:"anomaly_reason"`
	ComplianceFlags  JSONB          `gorm:"type:jsonb;default:'[]'" json:"compliance_flags"`
	IsPCIRelevant    bool           `gorm:"not null;default:false;index" json:"is_pci_relevant"`
	IsGDPRRelevant   bool           `gorm:"not null;default:false;index" json:"is_gdpr_relevant"`
	Timestamp        time.Time      `gorm:"not null;default:now();index" json:"timestamp"`
	ReviewedAt       *time.Time     `gorm:"type:timestamptz" json:"reviewed_at"`
	ReviewedBy       *uuid.UUID     `gorm:"type:uuid" json:"reviewed_by"`

	// Relationships
	Tenant         Tenant      `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	User           *User       `gorm:"foreignKey:UserID;constraint:OnDelete:SET NULL" json:"user,omitempty"`
	Restaurant     *Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:SET NULL" json:"restaurant,omitempty"`
	ReviewedByUser *User       `gorm:"foreignKey:ReviewedBy;constraint:OnDelete:SET NULL" json:"reviewed_by_user,omitempty"`
}

// AnomalyRecord represents a detected anomaly
type AnomalyRecord struct {
	AnomalyRecordID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"anomaly_record_id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Description     string    `gorm:"type:text" json:"description"`
	DetectedAt      time.Time `gorm:"not null;default:now();index" json:"detected_at"`
	AnomalyType     string    `gorm:"type:varchar(100);index" json:"anomaly_type"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// BeforeCreate hooks
func (al *ActivityLog) BeforeCreate(tx *gorm.DB) error {
	if al.ActivityID == uuid.Nil {
		al.ActivityID = uuid.New()
	}
	return nil
}

func (at *AuditTrail) BeforeCreate(tx *gorm.DB) error {
	if at.AuditTrailID == uuid.Nil {
		at.AuditTrailID = uuid.New()
	}
	return nil
}

func (ar *AnomalyRecord) BeforeCreate(tx *gorm.DB) error {
	if ar.AnomalyRecordID == uuid.Nil {
		ar.AnomalyRecordID = uuid.New()
	}
	return nil
}

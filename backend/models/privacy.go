package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DataPrivacyRecord represents privacy compliance settings
type DataPrivacyRecord struct {
	DataPrivacyRecordID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"data_privacy_record_id"`
	TenantID                     uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	GDPRApplicable               *bool      `gorm:"type:boolean" json:"gdpr_applicable"`
	LawfulBasisForProcessing     string     `gorm:"type:text" json:"lawful_basis_for_processing"`
	HasPrivacyPolicy             *bool      `gorm:"type:boolean" json:"has_privacy_policy"`
	PrivacyPolicyLastUpdated     *time.Time `gorm:"type:timestamptz" json:"privacy_policy_last_updated"`
	HasCookieConsent             *bool      `gorm:"type:boolean" json:"has_cookie_consent"`
	HasDataProcessingAgreements  *bool      `gorm:"type:boolean" json:"has_data_processing_agreements"`
	CCPAApplicable               *bool      `gorm:"type:boolean" json:"ccpa_applicable"`
	AllowsDataDeletion           *bool      `gorm:"type:boolean" json:"allows_data_deletion"`
	AllowsDataPortability        *bool      `gorm:"type:boolean" json:"allows_data_portability"`
	AllowsOptOut                 *bool      `gorm:"type:boolean" json:"allows_opt_out"`
	CustomerDataRetentionDays    *int       `gorm:"type:int" json:"customer_data_retention_days"`
	PaymentDataRetentionDays     *int       `gorm:"type:int" json:"payment_data_retention_days"`
	AuditLogRetentionDays        *int       `gorm:"type:int" json:"audit_log_retention_days"`
	BackupRetentionDays          *int       `gorm:"type:int" json:"backup_retention_days"`
	DataCategories               JSONB      `gorm:"type:jsonb;default:'[]'" json:"data_categories"`
	ProcessingPurposes           JSONB      `gorm:"type:jsonb;default:'[]'" json:"processing_purposes"`
	DataRecipients               JSONB      `gorm:"type:jsonb;default:'[]'" json:"data_recipients"`
	InternationalTransfers       JSONB      `gorm:"type:jsonb;default:'[]'" json:"international_transfers"`
	CreatedAt                    time.Time  `gorm:"default:now()" json:"created_at"`
	UpdatedAt                    *time.Time `gorm:"default:now()" json:"updated_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// DataSubjectRequest represents a GDPR/CCPA data subject request
type DataSubjectRequest struct {
	DataSubjectRequestID  uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"data_subject_request_id"`
	DataPrivacyRecordID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"data_privacy_record_id"`
	CustomerID            *uuid.UUID `gorm:"type:uuid;index" json:"customer_id"`
	RequestType           string     `gorm:"type:varchar(50)" json:"request_type"`
	RequesterName         string     `gorm:"type:varchar(255)" json:"requester_name"`
	RequesterEmail        string     `gorm:"type:varchar(255)" json:"requester_email"`
	RequesterPhone        string     `gorm:"type:varchar(50)" json:"requester_phone"`
	RequestDescription    string     `gorm:"type:text" json:"request_description"`
	IdentityVerified      bool       `gorm:"default:false" json:"identity_verified"`
	VerificationMethod    string     `gorm:"type:varchar(100)" json:"verification_method"`
	VerifiedAt            *time.Time `gorm:"type:timestamptz" json:"verified_at"`
	VerifiedBy            *uuid.UUID `gorm:"type:uuid" json:"verified_by"`
	Status                string     `gorm:"type:varchar(50);index" json:"status"`
	RequestDate           *time.Time `gorm:"type:timestamptz;index" json:"request_date"`
	DueDate               *time.Time `gorm:"type:timestamptz" json:"due_date"`
	CompletedDate         *time.Time `gorm:"type:timestamptz" json:"completed_date"`
	ResponseNotes         string     `gorm:"type:text" json:"response_notes"`
	DataExportPath        string     `gorm:"type:text" json:"data_export_path"`
	ProcessedBy           *uuid.UUID `gorm:"type:uuid" json:"processed_by"`

	// Relationships
	DataPrivacyRecord DataPrivacyRecord `gorm:"foreignKey:DataPrivacyRecordID;constraint:OnDelete:CASCADE" json:"-"`
	Customer          *Customer         `gorm:"foreignKey:CustomerID;constraint:OnDelete:SET NULL" json:"customer,omitempty"`
	VerifiedByUser    *User             `gorm:"foreignKey:VerifiedBy;constraint:OnDelete:SET NULL" json:"verified_by_user,omitempty"`
	ProcessedByUser   *User             `gorm:"foreignKey:ProcessedBy;constraint:OnDelete:SET NULL" json:"processed_by_user,omitempty"`
}

// DataBreach represents a data breach incident
type DataBreach struct {
	DataBreachID                 uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"data_breach_id"`
	DataPrivacyRecordID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"data_privacy_record_id"`
	TenantID                     uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	IncidentTitle                string     `gorm:"type:varchar(255)" json:"incident_title"`
	Description                  string     `gorm:"type:text" json:"description"`
	BreachType                   string     `gorm:"type:varchar(100)" json:"breach_type"`
	Severity                     string     `gorm:"type:varchar(50);index" json:"severity"`
	IncidentDate                 *time.Time `gorm:"type:timestamptz;index" json:"incident_date"`
	DiscoveredDate               *time.Time `gorm:"type:timestamptz" json:"discovered_date"`
	ContainedDate                *time.Time `gorm:"type:timestamptz" json:"contained_date"`
	ResolvedDate                 *time.Time `gorm:"type:timestamptz" json:"resolved_date"`
	AffectedCustomers            int        `gorm:"default:0" json:"affected_customers"`
	DataTypesAffected            JSONB      `gorm:"type:jsonb;default:'[]'" json:"data_types_affected"`
	RequiresRegulatorNotification bool      `gorm:"default:false" json:"requires_regulator_notification"`
	RequiresCustomerNotification bool       `gorm:"default:false" json:"requires_customer_notification"`
	RegulatorsNotifiedDate       *time.Time `gorm:"type:timestamptz" json:"regulators_notified_date"`
	CustomersNotifiedDate        *time.Time `gorm:"type:timestamptz" json:"customers_notified_date"`
	ImmediateActions             string     `gorm:"type:text" json:"immediate_actions"`
	PreventiveActions            string     `gorm:"type:text" json:"preventive_actions"`
	LessonsLearned               string     `gorm:"type:text" json:"lessons_learned"`
	ReportedBy                   uuid.UUID  `gorm:"type:uuid;not null" json:"reported_by"`
	AssignedTo                   *uuid.UUID `gorm:"type:uuid" json:"assigned_to"`

	// Relationships
	DataPrivacyRecord DataPrivacyRecord `gorm:"foreignKey:DataPrivacyRecordID;constraint:OnDelete:CASCADE" json:"-"`
	Tenant            Tenant            `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	ReportedByUser    User              `gorm:"foreignKey:ReportedBy;constraint:OnDelete:RESTRICT" json:"-"`
	AssignedToUser    *User             `gorm:"foreignKey:AssignedTo;constraint:OnDelete:SET NULL" json:"assigned_to_user,omitempty"`
}

// DataArchiveSetting represents archive settings for data types
type DataArchiveSetting struct {
	ArchiveSettingID       uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"archive_setting_id"`
	TenantID               uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	DataType               string     `gorm:"type:varchar(100);not null;index" json:"data_type"`
	RetentionPeriodMonths  *int       `gorm:"type:int" json:"retention_period_months"`
	ArchiveFrequency       string     `gorm:"type:varchar(50)" json:"archive_frequency"`
	IsEnabled              bool       `gorm:"default:true" json:"is_enabled"`
	ArchiveLocation        JSONB      `gorm:"type:jsonb;default:'{}'" json:"archive_location"`
	RetentionRules         JSONB      `gorm:"type:jsonb;default:'{}'" json:"retention_rules"`
	CompressArchive        bool       `gorm:"default:true" json:"compress_archive"`
	EncryptArchive         bool       `gorm:"default:true" json:"encrypt_archive"`
	EncryptionKeyIdentifier string    `gorm:"type:varchar(255)" json:"encryption_key_identifier"`
	DataSelectionCriteria  JSONB      `gorm:"type:jsonb;default:'{}'" json:"data_selection_criteria"`
	LastArchiveDate        *time.Time `gorm:"type:timestamptz" json:"last_archive_date"`
	NextScheduledArchive   *time.Time `gorm:"type:timestamptz" json:"next_scheduled_archive"`
	LastArchiveStatus      string     `gorm:"type:varchar(50)" json:"last_archive_status"`
	LastArchiveRecordCount *int       `gorm:"type:int" json:"last_archive_record_count"`
	NotificationSettings   JSONB      `gorm:"type:jsonb;default:'{}'" json:"notification_settings"`
	CreatedAt              time.Time  `gorm:"default:now()" json:"created_at"`
	UpdatedAt              *time.Time `gorm:"default:now()" json:"updated_at"`
	CreatedBy              string     `gorm:"type:varchar(255)" json:"created_by"`
	UpdatedBy              string     `gorm:"type:varchar(255)" json:"updated_by"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// AuditTrailArchive represents archived audit trail entries
type AuditTrailArchive struct {
	AuditTrailID     uuid.UUID      `gorm:"type:uuid;primaryKey" json:"audit_trail_id"`
	TenantID         uuid.UUID      `gorm:"type:uuid;not null;index" json:"tenant_id"`
	UserID           *uuid.UUID     `gorm:"type:uuid" json:"user_id"`
	RestaurantID     *uuid.UUID     `gorm:"type:uuid" json:"restaurant_id"`
	EventType        AuditEvent		`gorm:"type:varchar(50)" json:"event_type"`
	EventCategory    string         `gorm:"type:varchar(100)" json:"event_category"`
	EventDescription string         `gorm:"type:text" json:"event_description"`
	Severity         AuditSeverity  `gorm:"type:varchar(50)" json:"severity"`
	EntityType       string         `gorm:"type:varchar(100)" json:"entity_type"`
	EntityID         string         `gorm:"type:varchar(255)" json:"entity_id"`
	OldValues        JSONB          `gorm:"type:jsonb" json:"old_values"`
	NewValues        JSONB          `gorm:"type:jsonb" json:"new_values"`
	RequestURL       string         `gorm:"type:text" json:"request_url"`
	HTTPMethod       string         `gorm:"type:varchar(10)" json:"http_method"`
	IPAddress        string         `gorm:"type:varchar(50)" json:"ip_address"`
	UserAgent        string         `gorm:"type:text" json:"user_agent"`
	SessionID        string         `gorm:"type:varchar(255)" json:"session_id"`
	Geolocation      JSONB          `gorm:"type:jsonb" json:"geolocation"`
	RiskLevel        RiskLevel      `gorm:"type:varchar(50)" json:"risk_level"`
	RequiresReview   bool           `gorm:"type:boolean" json:"requires_review"`
	IsAnomalous      bool           `gorm:"type:boolean" json:"is_anomalous"`
	AnomalyReason    string         `gorm:"type:text" json:"anomaly_reason"`
	ComplianceFlags  JSONB          `gorm:"type:jsonb" json:"compliance_flags"`
	IsPCIRelevant    bool           `gorm:"type:boolean" json:"is_pci_relevant"`
	IsGDPRRelevant   bool           `gorm:"type:boolean" json:"is_gdpr_relevant"`
	Timestamp        time.Time      `gorm:"type:timestamptz;index" json:"timestamp"`
	ReviewedAt       *time.Time     `gorm:"type:timestamptz" json:"reviewed_at"`
	ReviewedBy       *uuid.UUID     `gorm:"type:uuid" json:"reviewed_by"`
}

// BeforeCreate hooks
func (dpr *DataPrivacyRecord) BeforeCreate(tx *gorm.DB) error {
	if dpr.DataPrivacyRecordID == uuid.Nil {
		dpr.DataPrivacyRecordID = uuid.New()
	}
	return nil
}

func (dsr *DataSubjectRequest) BeforeCreate(tx *gorm.DB) error {
	if dsr.DataSubjectRequestID == uuid.Nil {
		dsr.DataSubjectRequestID = uuid.New()
	}
	return nil
}

func (db *DataBreach) BeforeCreate(tx *gorm.DB) error {
	if db.DataBreachID == uuid.Nil {
		db.DataBreachID = uuid.New()
	}
	return nil
}

func (das *DataArchiveSetting) BeforeCreate(tx *gorm.DB) error {
	if das.ArchiveSettingID == uuid.Nil {
		das.ArchiveSettingID = uuid.New()
	}
	return nil
}
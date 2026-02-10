package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PCIComplianceRecord represents PCI-DSS compliance status
type PCIComplianceRecord struct {
	PCIComplianceRecordID    uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"pci_compliance_record_id"`
	TenantID                 uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RestaurantID             *uuid.UUID `gorm:"type:uuid;index" json:"restaurant_id"`
	ComplianceLevel          string     `gorm:"type:varchar(50)" json:"compliance_level"`
	Status                   string     `gorm:"type:varchar(50);index" json:"status"`
	LastAssessment           time.Time  `gorm:"not null" json:"last_assessment"`
	NextAssessmentDue        *time.Time `gorm:"type:timestamptz" json:"next_assessment_due"`
	SAQType                  string     `gorm:"type:varchar(50)" json:"saq_type"`
	SAQCompleted             bool       `gorm:"not null;default:false" json:"saq_completed"`
	SAQCompletedDate         *time.Time `gorm:"type:timestamptz" json:"saq_completed_date"`
	SAQDocumentPath          string     `gorm:"type:text" json:"saq_document_path"`
	HasFirewall              bool       `gorm:"not null;default:false" json:"has_firewall"`
	HasUpdatedPasswords      bool       `gorm:"not null;default:false" json:"has_updated_passwords"`
	HasDataEncryption        bool       `gorm:"not null;default:false" json:"has_data_encryption"`
	HasAntiVirus             bool       `gorm:"not null;default:false" json:"has_anti_virus"`
	HasSecureNetworks        bool       `gorm:"not null;default:false" json:"has_secure_networks"`
	HasAccessControls        bool       `gorm:"not null;default:false" json:"has_access_controls"`
	HasUniqueUserIDs         bool       `gorm:"not null;default:false" json:"has_unique_user_ids"`
	HasDataAccessRestriction bool       `gorm:"not null;default:false" json:"has_data_access_restriction"`
	HasPhysicalSecurity      bool       `gorm:"not null;default:false" json:"has_physical_security"`
	HasNetworkMonitoring     bool       `gorm:"not null;default:false" json:"has_network_monitoring"`
	HasVulnerabilityTesting  bool       `gorm:"not null;default:false" json:"has_vulnerability_testing"`
	HasSecurityPolicy        bool       `gorm:"not null;default:false" json:"has_security_policy"`
	QSACompany               string     `gorm:"type:varchar(255)" json:"qsa_company"`
	CertificateNumber        string     `gorm:"type:varchar(255)" json:"certificate_number"`
	CertificateExpiry        *time.Time `gorm:"type:timestamptz" json:"certificate_expiry"`
	CreatedAt                time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt                time.Time  `gorm:"not null;default:now()" json:"updated_at"`

	// Relationships
	Tenant     Tenant      `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	Restaurant *Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:SET NULL" json:"restaurant,omitempty"`
}

// SecurityAssessment represents a security assessment
type SecurityAssessment struct {
	SecurityAssessmentID  uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"security_assessment_id"`
	TenantID              uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	AssessmentType        string     `gorm:"type:varchar(100)" json:"assessment_type"`
	AssessmentName        string     `gorm:"type:varchar(255)" json:"assessment_name"`
	Vendor                string     `gorm:"type:varchar(255)" json:"vendor"`
	ScheduledDate         *time.Time `gorm:"type:timestamptz" json:"scheduled_date"`
	StartedDate           *time.Time `gorm:"type:timestamptz" json:"started_date"`
	CompletedDate         *time.Time `gorm:"type:timestamptz;index" json:"completed_date"`
	Status                string     `gorm:"type:varchar(50);index" json:"status"`
	ScopeDescription      string     `gorm:"type:text" json:"scope_description"`
	SystemsIncluded       JSONB      `gorm:"type:jsonb;default:'[]'" json:"systems_included"`
	IncludesWebApplication bool      `gorm:"not null;default:false" json:"includes_web_application"`
	IncludesNetwork       bool       `gorm:"not null;default:false" json:"includes_network"`
	IncludesPhysical      bool       `gorm:"not null;default:false" json:"includes_physical"`
	IncludesSocial        bool       `gorm:"not null;default:false" json:"includes_social"`
	CriticalFindings      int        `gorm:"not null;default:0" json:"critical_findings"`
	HighFindings          int        `gorm:"not null;default:0" json:"high_findings"`
	MediumFindings        int        `gorm:"not null;default:0" json:"medium_findings"`
	LowFindings           int        `gorm:"not null;default:0" json:"low_findings"`
	ReportPath            string     `gorm:"type:text" json:"report_path"`
	ExecutiveSummary      string     `gorm:"type:text" json:"executive_summary"`
	CreatedAt             time.Time  `gorm:"not null;default:now()" json:"created_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
}

// SecurityFinding represents a security finding from an assessment
type SecurityFinding struct {
	SecurityFindingID    uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"security_finding_id"`
	SecurityAssessmentID uuid.UUID  `gorm:"type:uuid;not null;index" json:"security_assessment_id"`
	Title                string     `gorm:"type:varchar(500)" json:"title"`
	Description          string     `gorm:"type:text" json:"description"`
	Severity             string     `gorm:"type:varchar(50);index" json:"severity"`
	CVENumber            string     `gorm:"type:varchar(50)" json:"cve_number"`
	CVSSScore            float64    `gorm:"type:decimal(3,1);not null;default:0.0" json:"cvss_score"`
	Category             string     `gorm:"type:varchar(100)" json:"category"`
	AffectedSystem       string     `gorm:"type:varchar(255)" json:"affected_system"`
	ProofOfConcept       string     `gorm:"type:text" json:"proof_of_concept"`
	Recommendation       string     `gorm:"type:text" json:"recommendation"`
	Status               string     `gorm:"type:varchar(50);index" json:"status"`
	RemediatedDate       *time.Time `gorm:"type:timestamptz" json:"remediated_date"`
	RemediationNotes     string     `gorm:"type:text" json:"remediation_notes"`
	AssignedTo           *uuid.UUID `gorm:"type:uuid" json:"assigned_to"`

	// Relationships
	SecurityAssessment SecurityAssessment `gorm:"foreignKey:SecurityAssessmentID;constraint:OnDelete:CASCADE" json:"-"`
	AssignedToUser     *User              `gorm:"foreignKey:AssignedTo;constraint:OnDelete:SET NULL" json:"assigned_to_user,omitempty"`
}

// SecurityIncident represents a security incident
type SecurityIncident struct {
	SecurityIncidentID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"security_incident_id"`
	TenantID                  uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	IncidentType              string     `gorm:"type:varchar(100)" json:"incident_type"`
	Severity                  string     `gorm:"type:varchar(50);index" json:"severity"`
	Title                     string     `gorm:"type:varchar(500)" json:"title"`
	Description               string     `gorm:"type:text" json:"description"`
	DetectedAt                time.Time  `gorm:"not null;default:now();index" json:"detected_at"`
	DetectionMethod           string     `gorm:"type:varchar(100)" json:"detection_method"`
	Status                    string     `gorm:"type:varchar(50);index" json:"status"`
	AffectsCustomerData       bool       `gorm:"not null;default:false" json:"affects_customer_data"`
	AffectsPaymentData        bool       `gorm:"not null;default:false" json:"affects_payment_data"`
	AffectsSystemAvailability bool       `gorm:"not null;default:false" json:"affects_system_availability"`
	EstimatedAffectedUsers    int        `gorm:"not null;default:0" json:"estimated_affected_users"`
	AcknowledgedAt            *time.Time `gorm:"type:timestamptz" json:"acknowledged_at"`
	MitigatedAt               *time.Time `gorm:"type:timestamptz" json:"mitigated_at"`
	ResolvedAt                *time.Time `gorm:"type:timestamptz" json:"resolved_at"`
	ReportedBy                uuid.UUID  `gorm:"type:uuid;not null" json:"reported_by"`
	AssignedTo                *uuid.UUID `gorm:"type:uuid" json:"assigned_to"`
	ResponseActions           string     `gorm:"type:text" json:"response_actions"`
	RootCause                 string     `gorm:"type:text" json:"root_cause"`
	PreventionMeasures        string     `gorm:"type:text" json:"prevention_measures"`

	// Relationships
	Tenant         Tenant `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE" json:"-"`
	ReportedByUser User   `gorm:"foreignKey:ReportedBy;constraint:OnDelete:RESTRICT" json:"-"`
	AssignedToUser *User  `gorm:"foreignKey:AssignedTo;constraint:OnDelete:SET NULL" json:"assigned_to_user,omitempty"`
}

// BeforeCreate hooks
func (pcir *PCIComplianceRecord) BeforeCreate(tx *gorm.DB) error {
	if pcir.PCIComplianceRecordID == uuid.Nil {
		pcir.PCIComplianceRecordID = uuid.New()
	}
	return nil
}

func (sa *SecurityAssessment) BeforeCreate(tx *gorm.DB) error {
	if sa.SecurityAssessmentID == uuid.Nil {
		sa.SecurityAssessmentID = uuid.New()
	}
	return nil
}

func (sf *SecurityFinding) BeforeCreate(tx *gorm.DB) error {
	if sf.SecurityFindingID == uuid.Nil {
		sf.SecurityFindingID = uuid.New()
	}
	return nil
}

func (si *SecurityIncident) BeforeCreate(tx *gorm.DB) error {
	if si.SecurityIncidentID == uuid.Nil {
		si.SecurityIncidentID = uuid.New()
	}
	return nil
}
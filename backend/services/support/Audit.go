// services/support/Audit.go
//
// AuditService -- comprehensive audit trail management, compliance reporting,
// anomaly detection, and security event queries. All operations are
// tenant-scoped. This is a read-heavy service -- write operations are
// typically performed by other services using direct model inserts.
package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"backend/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// SENTINEL ERRORS
// ─────────────────────────────────────────────────────────────────────────────

var (
	ErrAuditEntryNotFound = errors.New("audit entry not found")
	ErrAnomalyNotFound    = errors.New("anomaly record not found")
)

// ─────────────────────────────────────────────────────────────────────────────
// SERVICE
// ─────────────────────────────────────────────────────────────────────────────

// AuditService provides comprehensive audit trail querying, anomaly
// detection, and compliance reporting.
type AuditService struct {
	db    *gorm.DB
	redis *goredis.Client
}

// NewAuditService constructs a new AuditService.
func NewAuditService(db *gorm.DB, redis *goredis.Client) *AuditService {
	return &AuditService{db: db, redis: redis}
}

// ─────────────────────────────────────────────────────────────────────────────
// INPUT / OUTPUT TYPES
// ─────────────────────────────────────────────────────────────────────────────

// AuditFilter controls pagination and filtering for audit queries.
type AuditFilter struct {
	TenantID       uuid.UUID
	RestaurantID   *uuid.UUID
	UserID         *uuid.UUID
	EventType      *models.AuditEvent
	Severity       *models.AuditSeverity
	RiskLevel      *models.RiskLevel
	EntityType     *string
	EntityID       *string
	RequiresReview *bool
	IsAnomalous    *bool
	IsPCIRelevant  *bool
	IsGDPRRelevant *bool
	From           *time.Time
	To             *time.Time
	Search         *string
	Page           int
	PageSize       int
	SortOrder      string // "asc" or "desc"
}

// AuditEntryResponse is a read-optimised view of an audit trail entry.
type AuditEntryResponse struct {
	AuditTrailID     uuid.UUID            `json:"audit_trail_id"`
	TenantID         uuid.UUID            `json:"tenant_id"`
	UserID           *uuid.UUID           `json:"user_id,omitempty"`
	UserName         string               `json:"user_name,omitempty"`
	RestaurantID     *uuid.UUID           `json:"restaurant_id,omitempty"`
	EventType        models.AuditEvent    `json:"event_type"`
	EventCategory    string               `json:"event_category"`
	EventDescription string               `json:"event_description"`
	Severity         models.AuditSeverity `json:"severity"`
	EntityType       string               `json:"entity_type"`
	EntityID         string               `json:"entity_id"`
	OldValues        json.RawMessage      `json:"old_values,omitempty"`
	NewValues        json.RawMessage      `json:"new_values,omitempty"`
	IPAddress        string               `json:"ip_address,omitempty"`
	RiskLevel        models.RiskLevel     `json:"risk_level"`
	RequiresReview   bool                 `json:"requires_review"`
	IsAnomalous      bool                 `json:"is_anomalous"`
	AnomalyReason    string               `json:"anomaly_reason,omitempty"`
	IsPCIRelevant    bool                 `json:"is_pci_relevant"`
	IsGDPRRelevant   bool                 `json:"is_gdpr_relevant"`
	Timestamp        time.Time            `json:"timestamp"`
	ReviewedAt       *time.Time           `json:"reviewed_at,omitempty"`
	ReviewedBy       *uuid.UUID           `json:"reviewed_by,omitempty"`
}

// AuditStats holds aggregate audit metrics.
type AuditStats struct {
	TotalEntries      int            `json:"total_entries"`
	BySeverity        map[string]int `json:"by_severity"`
	ByEventType       map[string]int `json:"by_event_type"`
	ByRiskLevel       map[string]int `json:"by_risk_level"`
	PendingReviews    int            `json:"pending_reviews"`
	AnomaliesDetected int            `json:"anomalies_detected"`
	PCIRelevantCount  int            `json:"pci_relevant_count"`
	GDPRRelevantCount int            `json:"gdpr_relevant_count"`
}

// ComplianceReport is a summary of compliance-relevant audit activity.
type ComplianceReport struct {
	TenantID          uuid.UUID `json:"tenant_id"`
	PeriodFrom        time.Time `json:"period_from"`
	PeriodTo          time.Time `json:"period_to"`
	TotalEvents       int       `json:"total_events"`
	PCIEvents         int       `json:"pci_events"`
	GDPREvents        int       `json:"gdpr_events"`
	HighRiskEvents    int       `json:"high_risk_events"`
	CriticalEvents    int       `json:"critical_events"`
	UnreviewedCount   int       `json:"unreviewed_count"`
	AnomalyCount      int       `json:"anomaly_count"`
	DataExportEvents  int       `json:"data_export_events"`
	PermissionChanges int       `json:"permission_changes"`
	LoginEvents       int       `json:"login_events"`
	SecurityEvents    int       `json:"security_events"`
	ComplianceScore   float64   `json:"compliance_score"`
}

// SecuritySnapshot holds recent security event statistics.
type SecuritySnapshot struct {
	TotalEvents         int         `json:"total_events"`
	FailedLogins        int         `json:"failed_logins"`
	SuccessfulLogins    int         `json:"successful_logins"`
	PasswordChanges     int         `json:"password_changes"`
	HighRiskActions     int         `json:"high_risk_actions"`
	AnomalousActivities int         `json:"anomalous_activities"`
	TopRiskyUsers       []RiskyUser `json:"top_risky_users"`
	EventsByHour        map[int]int `json:"events_by_hour"`
}

// RiskyUser identifies a user with elevated risk activity.
type RiskyUser struct {
	UserID      uuid.UUID `json:"user_id"`
	EventCount  int       `json:"event_count"`
	HighestRisk string    `json:"highest_risk"`
}

// ─────────────────────────────────────────────────────────────────────────────
// WRITE OPERATIONS
// ─────────────────────────────────────────────────────────────────────────────

// RecordAuditEntry creates a new audit trail entry.
func (s *AuditService) RecordAuditEntry(
	ctx context.Context,
	entry *models.AuditTrail,
) error {
	if entry.AuditTrailID == uuid.Nil {
		entry.AuditTrailID = uuid.New()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if entry.Severity == "" {
		entry.Severity = models.AuditSeverityInfo
	}
	if entry.RiskLevel == "" {
		entry.RiskLevel = models.RiskLevelLow
	}

	// Auto-flag as requiring review if high risk or critical severity
	if entry.RiskLevel == models.RiskLevelHigh || entry.RiskLevel == models.RiskLevelCritical ||
		entry.Severity == models.AuditSeverityCritical {
		entry.RequiresReview = true
	}

	// Auto-flag PCI relevance for payment events
	if entry.EventType == models.AuditEventPaymentProcessed {
		entry.IsPCIRelevant = true
	}

	// Auto-flag GDPR relevance for data export and user operations
	if entry.EventType == models.AuditEventDataExport ||
		(entry.EntityType == "User" && (entry.EventType == models.AuditEventDelete || entry.EventType == models.AuditEventUpdate)) {
		entry.IsGDPRRelevant = true
	}

	if err := s.db.WithContext(ctx).Create(entry).Error; err != nil {
		return fmt.Errorf("record audit entry: %w", err)
	}
	return nil
}

// RecordAnomaly records a detected anomaly.
func (s *AuditService) RecordAnomaly(
	ctx context.Context,
	tenantID uuid.UUID,
	anomalyType, description string,
) error {
	record := &models.AnomalyRecord{
		AnomalyRecordID: uuid.New(),
		TenantID:        tenantID,
		AnomalyType:     anomalyType,
		Description:     description,
		DetectedAt:      time.Now(),
	}
	return s.db.WithContext(ctx).Create(record).Error
}

// ─────────────────────────────────────────────────────────────────────────────
// READ: AUDIT TRAIL
// ─────────────────────────────────────────────────────────────────────────────

// GetAuditEntry returns a single audit trail entry by ID.
func (s *AuditService) GetAuditEntry(
	ctx context.Context,
	tenantID uuid.UUID,
	auditTrailID uuid.UUID,
) (*AuditEntryResponse, error) {
	var entry models.AuditTrail
	if err := s.db.WithContext(ctx).
		Preload("User").
		Where("audit_trail_id = ? AND tenant_id = ?", auditTrailID, tenantID).
		First(&entry).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAuditEntryNotFound
		}
		return nil, fmt.Errorf("get audit entry: %w", err)
	}
	return s.toResponse(&entry), nil
}

// ListAuditTrail returns a paginated, filtered list of audit entries.
func (s *AuditService) ListAuditTrail(
	ctx context.Context,
	f AuditFilter,
) ([]AuditEntryResponse, int64, error) {
	q := s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where("tenant_id = ?", f.TenantID)

	q = s.applyAuditFilters(q, f)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	sortDir := "DESC"
	if strings.EqualFold(f.SortOrder, "asc") {
		sortDir = "ASC"
	}

	if f.PageSize <= 0 {
		f.PageSize = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}

	var entries []models.AuditTrail
	if err := s.db.WithContext(ctx).
		Preload("User").
		Where("tenant_id = ?", f.TenantID).
		Scopes(func(db *gorm.DB) *gorm.DB {
			return s.applyAuditFilters(db, f)
		}).
		Order(fmt.Sprintf("timestamp %s", sortDir)).
		Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize).
		Find(&entries).Error; err != nil {
		return nil, 0, err
	}

	results := make([]AuditEntryResponse, len(entries))
	for i := range entries {
		results[i] = *s.toResponse(&entries[i])
	}
	return results, total, nil
}

// GetAuditHistory returns all audit entries for a specific entity.
func (s *AuditService) GetAuditHistory(
	ctx context.Context,
	tenantID uuid.UUID,
	entityType, entityID string,
) ([]AuditEntryResponse, error) {
	var entries []models.AuditTrail
	if err := s.db.WithContext(ctx).
		Preload("User").
		Where("tenant_id = ? AND entity_type = ? AND entity_id = ?",
			tenantID, entityType, entityID).
		Order("timestamp DESC").
		Find(&entries).Error; err != nil {
		return nil, err
	}

	results := make([]AuditEntryResponse, len(entries))
	for i := range entries {
		results[i] = *s.toResponse(&entries[i])
	}
	return results, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// REVIEW WORKFLOW
// ─────────────────────────────────────────────────────────────────────────────

// GetPendingReviews returns all audit entries that require review.
func (s *AuditService) GetPendingReviews(
	ctx context.Context,
	tenantID uuid.UUID,
	page, pageSize int,
) ([]AuditEntryResponse, int64, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	if page <= 0 {
		page = 1
	}

	var total int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where("tenant_id = ? AND requires_review = true AND reviewed_at IS NULL", tenantID).
		Count(&total)

	var entries []models.AuditTrail
	if err := s.db.WithContext(ctx).
		Preload("User").
		Where("tenant_id = ? AND requires_review = true AND reviewed_at IS NULL", tenantID).
		Order("timestamp DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&entries).Error; err != nil {
		return nil, 0, err
	}

	results := make([]AuditEntryResponse, len(entries))
	for i := range entries {
		results[i] = *s.toResponse(&entries[i])
	}
	return results, total, nil
}

// MarkReviewed marks an audit entry as reviewed by a specific user.
func (s *AuditService) MarkReviewed(
	ctx context.Context,
	tenantID uuid.UUID,
	auditTrailID, reviewerID uuid.UUID,
) error {
	now := time.Now()
	result := s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where("audit_trail_id = ? AND tenant_id = ?", auditTrailID, tenantID).
		Updates(map[string]interface{}{
			"reviewed_at": now,
			"reviewed_by": reviewerID,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAuditEntryNotFound
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// STATISTICS
// ─────────────────────────────────────────────────────────────────────────────

// GetAuditStats returns aggregate audit metrics for a given time window.
func (s *AuditService) GetAuditStats(
	ctx context.Context,
	tenantID uuid.UUID,
	from, to time.Time,
) (*AuditStats, error) {
	stats := &AuditStats{
		BySeverity:  make(map[string]int),
		ByEventType: make(map[string]int),
		ByRiskLevel: make(map[string]int),
	}

	baseQ := s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where("tenant_id = ? AND timestamp >= ? AND timestamp <= ?", tenantID, from, to)

	// Total
	var total int64
	baseQ.Count(&total)
	stats.TotalEntries = int(total)

	// By severity
	type kv struct {
		Key   string
		Count int
	}
	var severities []kv
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Select("severity AS key, COUNT(*) AS count").
		Where("tenant_id = ? AND timestamp >= ? AND timestamp <= ?", tenantID, from, to).
		Group("severity").Scan(&severities)
	for _, sv := range severities {
		stats.BySeverity[sv.Key] = sv.Count
	}

	// By event type
	var eventTypes []kv
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Select("event_type AS key, COUNT(*) AS count").
		Where("tenant_id = ? AND timestamp >= ? AND timestamp <= ?", tenantID, from, to).
		Group("event_type").Scan(&eventTypes)
	for _, et := range eventTypes {
		stats.ByEventType[et.Key] = et.Count
	}

	// By risk level
	var riskLevels []kv
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Select("risk_level AS key, COUNT(*) AS count").
		Where("tenant_id = ? AND timestamp >= ? AND timestamp <= ?", tenantID, from, to).
		Group("risk_level").Scan(&riskLevels)
	for _, rl := range riskLevels {
		stats.ByRiskLevel[rl.Key] = rl.Count
	}

	// Pending reviews
	var pending int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where("tenant_id = ? AND requires_review = true AND reviewed_at IS NULL AND timestamp >= ? AND timestamp <= ?",
			tenantID, from, to).Count(&pending)
	stats.PendingReviews = int(pending)

	// Anomalies
	var anomalies int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where("tenant_id = ? AND is_anomalous = true AND timestamp >= ? AND timestamp <= ?",
			tenantID, from, to).Count(&anomalies)
	stats.AnomaliesDetected = int(anomalies)

	// PCI/GDPR counts
	var pci, gdpr int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where("tenant_id = ? AND is_pci_relevant = true AND timestamp >= ? AND timestamp <= ?",
			tenantID, from, to).Count(&pci)
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where("tenant_id = ? AND is_gdpr_relevant = true AND timestamp >= ? AND timestamp <= ?",
			tenantID, from, to).Count(&gdpr)
	stats.PCIRelevantCount = int(pci)
	stats.GDPRRelevantCount = int(gdpr)

	return stats, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// COMPLIANCE REPORTING
// ─────────────────────────────────────────────────────────────────────────────

// GenerateComplianceReport produces a compliance summary for the given period.
func (s *AuditService) GenerateComplianceReport(
	ctx context.Context,
	tenantID uuid.UUID,
	from, to time.Time,
) (*ComplianceReport, error) {
	report := &ComplianceReport{
		TenantID:   tenantID,
		PeriodFrom: from,
		PeriodTo:   to,
	}

	baseWhere := "tenant_id = ? AND timestamp >= ? AND timestamp <= ?"
	args := []interface{}{tenantID, from, to}

	// Total events
	var total int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere, args...).Count(&total)
	report.TotalEvents = int(total)

	// PCI events
	var pci int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND is_pci_relevant = true", args...).Count(&pci)
	report.PCIEvents = int(pci)

	// GDPR events
	var gdpr int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND is_gdpr_relevant = true", args...).Count(&gdpr)
	report.GDPREvents = int(gdpr)

	// High risk events
	var highRisk int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND risk_level IN ('high', 'critical')", args...).Count(&highRisk)
	report.HighRiskEvents = int(highRisk)

	// Critical severity
	var critical int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND severity = 'critical'", args...).Count(&critical)
	report.CriticalEvents = int(critical)

	// Unreviewed
	var unreviewed int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND requires_review = true AND reviewed_at IS NULL", args...).Count(&unreviewed)
	report.UnreviewedCount = int(unreviewed)

	// Anomalies
	var anomalies int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND is_anomalous = true", args...).Count(&anomalies)
	report.AnomalyCount = int(anomalies)

	// Data exports
	var exports int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND event_type = 'data_export'", args...).Count(&exports)
	report.DataExportEvents = int(exports)

	// Permission changes
	var permChanges int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND event_type = 'permissions_change'", args...).Count(&permChanges)
	report.PermissionChanges = int(permChanges)

	// Login events
	var logins int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND event_type IN ('login', 'logout')", args...).Count(&logins)
	report.LoginEvents = int(logins)

	// Security events
	var security int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND event_type = 'security_event'", args...).Count(&security)
	report.SecurityEvents = int(security)

	// Compliance score: 100 - penalty
	// Penalties: unreviewed high-risk events, anomalies, excessive critical events
	penalty := 0.0
	if report.TotalEvents > 0 {
		unreviewedRatio := float64(report.UnreviewedCount) / float64(report.TotalEvents)
		penalty += unreviewedRatio * 30 // max 30pt penalty for unreviewed

		if report.CriticalEvents > 0 {
			criticalRatio := float64(report.CriticalEvents) / float64(report.TotalEvents)
			penalty += criticalRatio * 20 // max 20pt penalty for criticals
		}

		anomalyRatio := float64(report.AnomalyCount) / float64(report.TotalEvents)
		penalty += anomalyRatio * 20 // max 20pt penalty for anomalies
	}
	report.ComplianceScore = math.Max(0, math.Round((100-penalty)*100)/100)

	return report, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// SECURITY SNAPSHOT
// ─────────────────────────────────────────────────────────────────────────────

// GetSecuritySnapshot returns a summary of recent security-relevant activity.
func (s *AuditService) GetSecuritySnapshot(
	ctx context.Context,
	tenantID uuid.UUID,
	from, to time.Time,
) (*SecuritySnapshot, error) {
	snap := &SecuritySnapshot{
		EventsByHour: make(map[int]int),
	}

	baseWhere := "tenant_id = ? AND timestamp >= ? AND timestamp <= ?"
	args := []interface{}{tenantID, from, to}

	// Total events
	var total int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere, args...).Count(&total)
	snap.TotalEvents = int(total)

	// Failed logins
	var failed int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND event_type = 'security_event' AND event_category = 'failed_login'", args...).
		Count(&failed)
	snap.FailedLogins = int(failed)

	// Successful logins
	var successful int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND event_type = 'login'", args...).Count(&successful)
	snap.SuccessfulLogins = int(successful)

	// Password changes
	var pwChanges int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND event_type = 'password_change'", args...).Count(&pwChanges)
	snap.PasswordChanges = int(pwChanges)

	// High risk actions
	var highRisk int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND risk_level IN ('high', 'critical')", args...).Count(&highRisk)
	snap.HighRiskActions = int(highRisk)

	// Anomalous activities
	var anomalous int64
	s.db.WithContext(ctx).Model(&models.AuditTrail{}).
		Where(baseWhere+" AND is_anomalous = true", args...).Count(&anomalous)
	snap.AnomalousActivities = int(anomalous)

	// Top risky users (up to 10)
	type riskyRow struct {
		UserID  uuid.UUID
		Count   int
		MaxRisk string
	}
	var riskyUsers []riskyRow
	s.db.WithContext(ctx).Raw(`
		SELECT user_id, COUNT(*) as count,
			MAX(CASE
				WHEN risk_level = 'critical' THEN 4
				WHEN risk_level = 'high' THEN 3
				WHEN risk_level = 'medium' THEN 2
				ELSE 1
			END) as max_risk_num,
			MAX(risk_level) as max_risk
		FROM audit_trails
		WHERE tenant_id = ? AND timestamp >= ? AND timestamp <= ?
			AND user_id IS NOT NULL
			AND risk_level IN ('medium', 'high', 'critical')
		GROUP BY user_id
		ORDER BY max_risk_num DESC, count DESC
		LIMIT 10
	`, tenantID, from, to).Scan(&riskyUsers)
	for _, ru := range riskyUsers {
		snap.TopRiskyUsers = append(snap.TopRiskyUsers, RiskyUser{
			UserID:      ru.UserID,
			EventCount:  ru.Count,
			HighestRisk: ru.MaxRisk,
		})
	}

	// Events by hour
	type hourCount struct {
		Hour  int
		Count int
	}
	var hourly []hourCount
	s.db.WithContext(ctx).Raw(`
		SELECT EXTRACT(HOUR FROM timestamp)::int AS hour, COUNT(*) AS count
		FROM audit_trails
		WHERE tenant_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY EXTRACT(HOUR FROM timestamp)
		ORDER BY hour
	`, tenantID, from, to).Scan(&hourly)
	for _, h := range hourly {
		snap.EventsByHour[h.Hour] = h.Count
	}

	return snap, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// ANOMALY QUERIES
// ─────────────────────────────────────────────────────────────────────────────

// GetAnomalies returns recent anomaly records for a tenant.
func (s *AuditService) GetAnomalies(
	ctx context.Context,
	tenantID uuid.UUID,
	from, to time.Time,
	limit int,
) ([]models.AnomalyRecord, error) {
	if limit <= 0 {
		limit = 50
	}

	var records []models.AnomalyRecord
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND detected_at >= ? AND detected_at <= ?",
			tenantID, from, to).
		Order("detected_at DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// GetUserActivity returns recent audit activity for a specific user.
func (s *AuditService) GetUserActivity(
	ctx context.Context,
	tenantID, userID uuid.UUID,
	from, to time.Time,
	limit int,
) ([]AuditEntryResponse, error) {
	if limit <= 0 {
		limit = 50
	}

	var entries []models.AuditTrail
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND user_id = ? AND timestamp >= ? AND timestamp <= ?",
			tenantID, userID, from, to).
		Order("timestamp DESC").
		Limit(limit).
		Find(&entries).Error; err != nil {
		return nil, err
	}

	results := make([]AuditEntryResponse, len(entries))
	for i := range entries {
		results[i] = *s.toResponse(&entries[i])
	}
	return results, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────

func (s *AuditService) applyAuditFilters(q *gorm.DB, f AuditFilter) *gorm.DB {
	if f.RestaurantID != nil {
		q = q.Where("restaurant_id = ?", *f.RestaurantID)
	}
	if f.UserID != nil {
		q = q.Where("user_id = ?", *f.UserID)
	}
	if f.EventType != nil {
		q = q.Where("event_type = ?", *f.EventType)
	}
	if f.Severity != nil {
		q = q.Where("severity = ?", *f.Severity)
	}
	if f.RiskLevel != nil {
		q = q.Where("risk_level = ?", *f.RiskLevel)
	}
	if f.EntityType != nil {
		q = q.Where("entity_type = ?", *f.EntityType)
	}
	if f.EntityID != nil {
		q = q.Where("entity_id = ?", *f.EntityID)
	}
	if f.RequiresReview != nil {
		q = q.Where("requires_review = ?", *f.RequiresReview)
	}
	if f.IsAnomalous != nil {
		q = q.Where("is_anomalous = ?", *f.IsAnomalous)
	}
	if f.IsPCIRelevant != nil {
		q = q.Where("is_pci_relevant = ?", *f.IsPCIRelevant)
	}
	if f.IsGDPRRelevant != nil {
		q = q.Where("is_gdpr_relevant = ?", *f.IsGDPRRelevant)
	}
	if f.From != nil {
		q = q.Where("timestamp >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("timestamp <= ?", *f.To)
	}
	if f.Search != nil && *f.Search != "" {
		search := "%" + strings.ToLower(*f.Search) + "%"
		q = q.Where("LOWER(event_description) LIKE ? OR LOWER(entity_type) LIKE ? OR entity_id LIKE ?",
			search, search, "%"+*f.Search+"%")
	}
	return q
}

func (s *AuditService) toResponse(entry *models.AuditTrail) *AuditEntryResponse {
	resp := &AuditEntryResponse{
		AuditTrailID:     entry.AuditTrailID,
		TenantID:         entry.TenantID,
		UserID:           entry.UserID,
		RestaurantID:     entry.RestaurantID,
		EventType:        entry.EventType,
		EventCategory:    entry.EventCategory,
		EventDescription: entry.EventDescription,
		Severity:         entry.Severity,
		EntityType:       entry.EntityType,
		EntityID:         entry.EntityID,
		IPAddress:        entry.IPAddress,
		RiskLevel:        entry.RiskLevel,
		RequiresReview:   entry.RequiresReview,
		IsAnomalous:      entry.IsAnomalous,
		AnomalyReason:    entry.AnomalyReason,
		IsPCIRelevant:    entry.IsPCIRelevant,
		IsGDPRRelevant:   entry.IsGDPRRelevant,
		Timestamp:        entry.Timestamp,
		ReviewedAt:       entry.ReviewedAt,
		ReviewedBy:       entry.ReviewedBy,
	}

	if entry.User != nil {
		resp.UserName = entry.User.FullName
	}

	// Serialize JSONB values
	if entry.OldValues != nil {
		if data, err := json.Marshal(entry.OldValues); err == nil {
			resp.OldValues = data
		}
	}
	if entry.NewValues != nil {
		if data, err := json.Marshal(entry.NewValues); err == nil {
			resp.NewValues = data
		}
	}

	return resp
}

// services/people/Employee.go
//
// EmployeeService -- full lifecycle management for restaurant employees.
// Handles CRUD, role assignment/revocation, scheduling metadata, performance
// tracking, and department-level queries. All operations are tenant-scoped
// and restaurant-scoped.
package people

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// SENTINEL ERRORS
// ─────────────────────────────────────────────────────────────────────────────

var (
	ErrEmployeeNotFound       = errors.New("employee not found")
	ErrEmployeeTenantMismatch = errors.New("employee does not belong to this tenant")
	ErrEmployeeDuplicate      = errors.New("employee with this email already exists in this restaurant")
	ErrEmployeeTerminated     = errors.New("employee is already terminated")
	ErrEmployeeRoleNotFound   = errors.New("role not found")
	ErrEmployeeRoleExists     = errors.New("employee already has this role")
)

// ─────────────────────────────────────────────────────────────────────────────
// SERVICE
// ─────────────────────────────────────────────────────────────────────────────

// EmployeeService manages the employee lifecycle.
// Construct one per application process -- goroutine-safe.
type EmployeeService struct {
	db    *gorm.DB
	redis *goredis.Client
}

// NewEmployeeService constructs a new EmployeeService.
func NewEmployeeService(db *gorm.DB, redis *goredis.Client) *EmployeeService {
	return &EmployeeService{db: db, redis: redis}
}

// ─────────────────────────────────────────────────────────────────────────────
// INPUT / OUTPUT TYPES
// ─────────────────────────────────────────────────────────────────────────────

// EmployeeServiceRequest scopes every call to a tenant + restaurant + actor.
type EmployeeServiceRequest struct {
	TenantID     uuid.UUID
	RestaurantID uuid.UUID
	ActorID      uuid.UUID
}

// CreateEmployeeInput contains the fields required to create an employee.
type CreateEmployeeInput struct {
	EmployeeServiceRequest
	FirstName  string
	LastName   string
	Email      string
	Phone      string
	Position   string
	Department string
	HireDate   *time.Time
	HourlyRate *float64
	UserID     *uuid.UUID // optional: link to a User account
}

// UpdateEmployeeInput contains the fields that can be updated on an employee.
type UpdateEmployeeInput struct {
	EmployeeServiceRequest
	EmployeeID uuid.UUID
	FirstName  *string
	LastName   *string
	Email      *string
	Phone      *string
	Position   *string
	Department *string
	HourlyRate *float64
}

// ListEmployeesFilter controls pagination and filtering.
type ListEmployeesFilter struct {
	EmployeeServiceRequest
	Department *string
	Position   *string
	IsActive   *bool
	Search     *string
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
}

// EmployeeResponse is a read-optimised view of an employee.
type EmployeeResponse struct {
	EmployeeID      uuid.UUID  `json:"employee_id"`
	TenantID        uuid.UUID  `json:"tenant_id"`
	RestaurantID    uuid.UUID  `json:"restaurant_id"`
	UserID          *uuid.UUID `json:"user_id,omitempty"`
	FirstName       string     `json:"first_name"`
	LastName        string     `json:"last_name"`
	FullName        string     `json:"full_name"`
	Email           string     `json:"email"`
	Phone           string     `json:"phone"`
	Position        string     `json:"position"`
	Department      string     `json:"department"`
	HireDate        *time.Time `json:"hire_date,omitempty"`
	TerminationDate *time.Time `json:"termination_date,omitempty"`
	HourlyRate      *float64   `json:"hourly_rate,omitempty"`
	IsActive        bool       `json:"is_active"`
	TenureDays      int        `json:"tenure_days"`
	Roles           []string   `json:"roles,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// EmployeeStats holds aggregate employee metrics.
type EmployeeStats struct {
	TotalEmployees  int            `json:"total_employees"`
	ActiveCount     int            `json:"active_count"`
	TerminatedCount int            `json:"terminated_count"`
	ByDepartment    map[string]int `json:"by_department"`
	ByPosition      map[string]int `json:"by_position"`
	AvgTenureDays   float64        `json:"avg_tenure_days"`
}

// ─────────────────────────────────────────────────────────────────────────────
// CREATE
// ─────────────────────────────────────────────────────────────────────────────

// CreateEmployee creates a new employee record.
func (s *EmployeeService) CreateEmployee(
	ctx context.Context,
	in CreateEmployeeInput,
) (*models.Employee, error) {
	// Check for duplicate email within the same restaurant
	if in.Email != "" {
		var count int64
		s.db.WithContext(ctx).Model(&models.Employee{}).
			Where("tenant_id = ? AND restaurant_id = ? AND email = ? AND is_active = true",
				in.TenantID, in.RestaurantID, in.Email).
			Count(&count)
		if count > 0 {
			return nil, ErrEmployeeDuplicate
		}
	}

	employee := &models.Employee{
		EmployeeID:   uuid.New(),
		TenantID:     in.TenantID,
		RestaurantID: in.RestaurantID,
		UserID:       in.UserID,
		FirstName:    in.FirstName,
		LastName:     in.LastName,
		Email:        in.Email,
		Phone:        in.Phone,
		Position:     in.Position,
		Department:   in.Department,
		HireDate:     in.HireDate,
		HourlyRate:   in.HourlyRate,
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.db.WithContext(ctx).Omit(clause.Associations).Create(employee).Error; err != nil {
		return nil, fmt.Errorf("create employee: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, in.RestaurantID, in.ActorID,
		models.AuditEventCreate, "Employee", employee.EmployeeID.String(),
		nil, map[string]interface{}{
			"name":       employee.FirstName + " " + employee.LastName,
			"position":   employee.Position,
			"department": employee.Department,
		})

	return employee, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// READ
// ─────────────────────────────────────────────────────────────────────────────

// GetEmployee returns a single employee by ID.
func (s *EmployeeService) GetEmployee(
	ctx context.Context,
	req EmployeeServiceRequest,
	employeeID uuid.UUID,
) (*EmployeeResponse, error) {
	var emp models.Employee
	if err := s.db.WithContext(ctx).
		Preload("Roles").
		Where("employee_id = ? AND tenant_id = ? AND restaurant_id = ?",
			employeeID, req.TenantID, req.RestaurantID).
		First(&emp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEmployeeNotFound
		}
		return nil, fmt.Errorf("get employee: %w", err)
	}

	return s.toResponse(&emp), nil
}

// ListEmployees returns a paginated, filtered list of employees.
func (s *EmployeeService) ListEmployees(
	ctx context.Context,
	f ListEmployeesFilter,
) ([]EmployeeResponse, int64, error) {
	q := s.db.WithContext(ctx).
		Where("tenant_id = ? AND restaurant_id = ?", f.TenantID, f.RestaurantID)

	if f.Department != nil {
		q = q.Where("department = ?", *f.Department)
	}
	if f.Position != nil {
		q = q.Where("position = ?", *f.Position)
	}
	if f.IsActive != nil {
		q = q.Where("is_active = ?", *f.IsActive)
	}
	if f.Search != nil && *f.Search != "" {
		search := "%" + strings.ToLower(*f.Search) + "%"
		q = q.Where("LOWER(first_name || ' ' || last_name) LIKE ? OR LOWER(email) LIKE ?",
			search, search)
	}

	var total int64
	if err := q.Model(&models.Employee{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	sortCol := "created_at"
	switch f.SortBy {
	case "name":
		sortCol = "first_name"
	case "position":
		sortCol = "position"
	case "department":
		sortCol = "department"
	case "hire_date":
		sortCol = "hire_date"
	}
	sortDir := "DESC"
	if strings.EqualFold(f.SortOrder, "asc") {
		sortDir = "ASC"
	}

	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}

	var employees []models.Employee
	if err := q.Preload("Roles").
		Order(fmt.Sprintf("%s %s", sortCol, sortDir)).
		Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize).
		Find(&employees).Error; err != nil {
		return nil, 0, err
	}

	results := make([]EmployeeResponse, len(employees))
	for i := range employees {
		results[i] = *s.toResponse(&employees[i])
	}
	return results, total, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// UPDATE
// ─────────────────────────────────────────────────────────────────────────────

// UpdateEmployee updates mutable fields on an employee.
func (s *EmployeeService) UpdateEmployee(
	ctx context.Context,
	in UpdateEmployeeInput,
) (*models.Employee, error) {
	emp, err := s.getEmployeeForWrite(ctx, in.TenantID, in.RestaurantID, in.EmployeeID)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})
	if in.FirstName != nil {
		updates["first_name"] = *in.FirstName
	}
	if in.LastName != nil {
		updates["last_name"] = *in.LastName
	}
	if in.Email != nil {
		updates["email"] = *in.Email
	}
	if in.Phone != nil {
		updates["phone"] = *in.Phone
	}
	if in.Position != nil {
		updates["position"] = *in.Position
	}
	if in.Department != nil {
		updates["department"] = *in.Department
	}
	if in.HourlyRate != nil {
		updates["hourly_rate"] = *in.HourlyRate
	}
	updates["updated_at"] = time.Now()

	if err := s.db.WithContext(ctx).Model(emp).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update employee: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, in.RestaurantID, in.ActorID,
		models.AuditEventUpdate, "Employee", emp.EmployeeID.String(),
		nil, updates)
	return emp, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// TERMINATE / REACTIVATE
// ─────────────────────────────────────────────────────────────────────────────

// TerminateEmployee marks an employee as terminated.
func (s *EmployeeService) TerminateEmployee(
	ctx context.Context,
	req EmployeeServiceRequest,
	employeeID uuid.UUID,
) error {
	emp, err := s.getEmployeeForWrite(ctx, req.TenantID, req.RestaurantID, employeeID)
	if err != nil {
		return err
	}
	if !emp.IsActive {
		return ErrEmployeeTerminated
	}

	now := time.Now()
	if err := s.db.WithContext(ctx).Model(emp).Updates(map[string]interface{}{
		"is_active":        false,
		"termination_date": now,
		"updated_at":       now,
	}).Error; err != nil {
		return fmt.Errorf("terminate employee: %w", err)
	}

	s.writeAudit(ctx, req.TenantID, req.RestaurantID, req.ActorID,
		models.AuditEventUpdate, "Employee", emp.EmployeeID.String(),
		map[string]interface{}{"is_active": true},
		map[string]interface{}{"is_active": false, "termination_date": now})
	return nil
}

// ReactivateEmployee restores a terminated employee.
func (s *EmployeeService) ReactivateEmployee(
	ctx context.Context,
	req EmployeeServiceRequest,
	employeeID uuid.UUID,
) error {
	emp, err := s.getEmployeeForWrite(ctx, req.TenantID, req.RestaurantID, employeeID)
	if err != nil {
		return err
	}
	if emp.IsActive {
		return errors.New("employee is already active")
	}

	if err := s.db.WithContext(ctx).Model(emp).Updates(map[string]interface{}{
		"is_active":        true,
		"termination_date": nil,
		"updated_at":       time.Now(),
	}).Error; err != nil {
		return fmt.Errorf("reactivate employee: %w", err)
	}

	s.writeAudit(ctx, req.TenantID, req.RestaurantID, req.ActorID,
		models.AuditEventUpdate, "Employee", emp.EmployeeID.String(),
		map[string]interface{}{"is_active": false},
		map[string]interface{}{"is_active": true})
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// ROLE ASSIGNMENT
// ─────────────────────────────────────────────────────────────────────────────

// AssignRole adds a role to an employee.
func (s *EmployeeService) AssignRole(
	ctx context.Context,
	req EmployeeServiceRequest,
	employeeID, roleID uuid.UUID,
) error {
	// Verify employee exists
	if _, err := s.getEmployeeForWrite(ctx, req.TenantID, req.RestaurantID, employeeID); err != nil {
		return err
	}

	// Verify role exists
	var role models.Role
	if err := s.db.WithContext(ctx).
		Where("role_id = ? AND tenant_id = ?", roleID, req.TenantID).
		First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrEmployeeRoleNotFound
		}
		return fmt.Errorf("verify role: %w", err)
	}

	// Check for existing assignment
	var count int64
	s.db.WithContext(ctx).Model(&models.EmployeeRole{}).
		Where("employee_id = ? AND role_id = ?", employeeID, roleID).
		Count(&count)
	if count > 0 {
		return ErrEmployeeRoleExists
	}

	assignment := &models.EmployeeRole{
		EmployeeRoleID: uuid.New(),
		EmployeeID:     employeeID,
		RoleID:         roleID,
		AssignedAt:     time.Now(),
		AssignedBy:     &req.ActorID,
	}
	if err := s.db.WithContext(ctx).Create(assignment).Error; err != nil {
		return fmt.Errorf("assign role: %w", err)
	}

	s.writeAudit(ctx, req.TenantID, req.RestaurantID, req.ActorID,
		models.AuditEventPermissionsChange, "EmployeeRole", employeeID.String(),
		nil, map[string]interface{}{"role_added": role.RoleName})
	return nil
}

// RevokeRole removes a role from an employee.
func (s *EmployeeService) RevokeRole(
	ctx context.Context,
	req EmployeeServiceRequest,
	employeeID, roleID uuid.UUID,
) error {
	result := s.db.WithContext(ctx).
		Where("employee_id = ? AND role_id = ?", employeeID, roleID).
		Delete(&models.EmployeeRole{})
	if result.Error != nil {
		return fmt.Errorf("revoke role: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("role assignment not found")
	}

	s.writeAudit(ctx, req.TenantID, req.RestaurantID, req.ActorID,
		models.AuditEventPermissionsChange, "EmployeeRole", employeeID.String(),
		nil, map[string]interface{}{"role_removed": roleID.String()})
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// STATISTICS
// ─────────────────────────────────────────────────────────────────────────────

// GetEmployeeStats returns aggregate employee metrics for a restaurant.
func (s *EmployeeService) GetEmployeeStats(
	ctx context.Context,
	req EmployeeServiceRequest,
) (*EmployeeStats, error) {
	stats := &EmployeeStats{
		ByDepartment: make(map[string]int),
		ByPosition:   make(map[string]int),
	}

	// Total and active counts
	s.db.WithContext(ctx).Model(&models.Employee{}).
		Where("tenant_id = ? AND restaurant_id = ?", req.TenantID, req.RestaurantID).
		Count(func() *int64 { v := int64(0); return &v }())

	var total, active int64
	s.db.WithContext(ctx).Model(&models.Employee{}).
		Where("tenant_id = ? AND restaurant_id = ?", req.TenantID, req.RestaurantID).
		Count(&total)
	s.db.WithContext(ctx).Model(&models.Employee{}).
		Where("tenant_id = ? AND restaurant_id = ? AND is_active = true", req.TenantID, req.RestaurantID).
		Count(&active)

	stats.TotalEmployees = int(total)
	stats.ActiveCount = int(active)
	stats.TerminatedCount = stats.TotalEmployees - stats.ActiveCount

	// By department
	type deptCount struct {
		Department string
		Count      int
	}
	var depts []deptCount
	s.db.WithContext(ctx).Model(&models.Employee{}).
		Select("department, COUNT(*) as count").
		Where("tenant_id = ? AND restaurant_id = ? AND is_active = true", req.TenantID, req.RestaurantID).
		Group("department").Scan(&depts)
	for _, d := range depts {
		if d.Department != "" {
			stats.ByDepartment[d.Department] = d.Count
		}
	}

	// By position
	type posCount struct {
		Position string
		Count    int
	}
	var positions []posCount
	s.db.WithContext(ctx).Model(&models.Employee{}).
		Select("position, COUNT(*) as count").
		Where("tenant_id = ? AND restaurant_id = ? AND is_active = true", req.TenantID, req.RestaurantID).
		Group("position").Scan(&positions)
	for _, p := range positions {
		if p.Position != "" {
			stats.ByPosition[p.Position] = p.Count
		}
	}

	// Average tenure
	var avgDays *float64
	s.db.WithContext(ctx).Model(&models.Employee{}).
		Select("AVG(EXTRACT(EPOCH FROM (COALESCE(termination_date, now()) - COALESCE(hire_date, created_at))) / 86400)").
		Where("tenant_id = ? AND restaurant_id = ?", req.TenantID, req.RestaurantID).
		Scan(&avgDays)
	if avgDays != nil {
		stats.AvgTenureDays = *avgDays
	}

	return stats, nil
}

// GetEmployeesByDepartment returns all active employees in a department.
func (s *EmployeeService) GetEmployeesByDepartment(
	ctx context.Context,
	req EmployeeServiceRequest,
	department string,
) ([]EmployeeResponse, error) {
	var employees []models.Employee
	if err := s.db.WithContext(ctx).
		Preload("Roles").
		Where("tenant_id = ? AND restaurant_id = ? AND department = ? AND is_active = true",
			req.TenantID, req.RestaurantID, department).
		Order("last_name ASC").
		Find(&employees).Error; err != nil {
		return nil, err
	}

	results := make([]EmployeeResponse, len(employees))
	for i := range employees {
		results[i] = *s.toResponse(&employees[i])
	}
	return results, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────

func (s *EmployeeService) getEmployeeForWrite(
	ctx context.Context,
	tenantID, restaurantID, employeeID uuid.UUID,
) (*models.Employee, error) {
	var emp models.Employee
	if err := s.db.WithContext(ctx).
		Where("employee_id = ? AND tenant_id = ? AND restaurant_id = ?",
			employeeID, tenantID, restaurantID).
		First(&emp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEmployeeNotFound
		}
		return nil, fmt.Errorf("get employee: %w", err)
	}
	return &emp, nil
}

func (s *EmployeeService) toResponse(emp *models.Employee) *EmployeeResponse {
	resp := &EmployeeResponse{
		EmployeeID:      emp.EmployeeID,
		TenantID:        emp.TenantID,
		RestaurantID:    emp.RestaurantID,
		UserID:          emp.UserID,
		FirstName:       emp.FirstName,
		LastName:        emp.LastName,
		FullName:        emp.FirstName + " " + emp.LastName,
		Email:           emp.Email,
		Phone:           emp.Phone,
		Position:        emp.Position,
		Department:      emp.Department,
		HireDate:        emp.HireDate,
		TerminationDate: emp.TerminationDate,
		HourlyRate:      emp.HourlyRate,
		IsActive:        emp.IsActive,
		CreatedAt:       emp.CreatedAt,
		UpdatedAt:       emp.UpdatedAt,
	}

	// Compute tenure
	start := emp.CreatedAt
	if emp.HireDate != nil {
		start = *emp.HireDate
	}
	end := time.Now()
	if emp.TerminationDate != nil {
		end = *emp.TerminationDate
	}
	resp.TenureDays = int(end.Sub(start).Hours() / 24)

	// Extract role names
	for _, r := range emp.Roles {
		resp.Roles = append(resp.Roles, r.RoleName)
	}

	return resp
}

func (s *EmployeeService) writeAudit(
	ctx context.Context,
	tenantID, restaurantID, actorID uuid.UUID,
	event models.AuditEvent,
	entityType, entityID string,
	oldValues, newValues interface{},
) {
	rID := restaurantID
	var userID *uuid.UUID
	if actorID != uuid.Nil {
		userID = &actorID
	}
	entry := &models.AuditTrail{
		AuditTrailID:     uuid.New(),
		TenantID:         tenantID,
		UserID:           userID,
		RestaurantID:     &rID,
		EventType:        event,
		EventCategory:    "people",
		EventDescription: fmt.Sprintf("%s on %s %s", event, entityType, entityID),
		Severity:         models.AuditSeverityInfo,
		EntityType:       entityType,
		EntityID:         entityID,
		RiskLevel:        models.RiskLevelLow,
		Timestamp:        time.Now(),
	}
	_ = s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(entry).Error
}

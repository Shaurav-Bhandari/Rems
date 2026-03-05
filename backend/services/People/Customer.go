// services/people/Customer.go
//
// CustomerService -- full lifecycle management for restaurant customers.
// Handles CRUD, loyalty points, order history aggregation, customer
// segmentation, and search. All operations are tenant-scoped.
package people

import (
	"context"
	"errors"
	"fmt"
	"math"
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
	ErrCustomerNotFound  = errors.New("customer not found")
	ErrCustomerDuplicate = errors.New("customer with this email or phone already exists")
	ErrInvalidPoints     = errors.New("insufficient loyalty points")
)

// ─────────────────────────────────────────────────────────────────────────────
// SERVICE
// ─────────────────────────────────────────────────────────────────────────────

// CustomerService manages customer records and loyalty programs.
type CustomerService struct {
	db    *gorm.DB
	redis *goredis.Client
}

// NewCustomerService constructs a new CustomerService.
func NewCustomerService(db *gorm.DB, redis *goredis.Client) *CustomerService {
	return &CustomerService{db: db, redis: redis}
}

// ─────────────────────────────────────────────────────────────────────────────
// INPUT / OUTPUT TYPES
// ─────────────────────────────────────────────────────────────────────────────

// CreateCustomerInput contains fields for creating a customer.
type CreateCustomerInput struct {
	TenantID    uuid.UUID
	FirstName   string
	LastName    string
	Email       string
	Phone       string
	Address     string
	City        string
	State       string
	PostalCode  string
	DateOfBirth *time.Time
}

// UpdateCustomerInput contains fields for updating a customer.
type UpdateCustomerInput struct {
	TenantID   uuid.UUID
	CustomerID uuid.UUID
	FirstName  *string
	LastName   *string
	Email      *string
	Phone      *string
	Address    *string
	City       *string
	State      *string
	PostalCode *string
}

// ListCustomersFilter controls pagination and filtering.
type ListCustomersFilter struct {
	TenantID  uuid.UUID
	Search    *string
	IsActive  *bool
	Segment   *string // "vip", "regular", "at_risk", "new"
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// CustomerResponse is a read-optimised view of a customer.
type CustomerResponse struct {
	CustomerID    uuid.UUID  `json:"customer_id"`
	TenantID      uuid.UUID  `json:"tenant_id"`
	FirstName     string     `json:"first_name"`
	LastName      string     `json:"last_name"`
	FullName      string     `json:"full_name"`
	Email         string     `json:"email"`
	Phone         string     `json:"phone"`
	Address       string     `json:"address,omitempty"`
	City          string     `json:"city,omitempty"`
	State         string     `json:"state,omitempty"`
	PostalCode    string     `json:"postal_code,omitempty"`
	DateOfBirth   *time.Time `json:"date_of_birth,omitempty"`
	LoyaltyPoints int        `json:"loyalty_points"`
	TotalOrders   int        `json:"total_orders"`
	TotalSpent    float64    `json:"total_spent"`
	AvgOrderValue float64    `json:"avg_order_value"`
	Segment       string     `json:"segment"`
	IsActive      bool       `json:"is_active"`
	MemberSince   time.Time  `json:"member_since"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CustomerStats holds aggregate customer metrics.
type CustomerStats struct {
	TotalCustomers  int     `json:"total_customers"`
	ActiveCustomers int     `json:"active_customers"`
	TotalRevenue    float64 `json:"total_revenue"`
	AvgLifetimeVal  float64 `json:"avg_lifetime_value"`
	AvgOrderValue   float64 `json:"avg_order_value"`
	VIPCount        int     `json:"vip_count"`
	AtRiskCount     int     `json:"at_risk_count"`
	NewThisMonth    int     `json:"new_this_month"`
}

// LoyaltyTransaction records a loyalty point change.
type LoyaltyTransaction struct {
	CustomerID uuid.UUID `json:"customer_id"`
	Points     int       `json:"points"`
	Reason     string    `json:"reason"`
	Balance    int       `json:"balance"`
	Timestamp  time.Time `json:"timestamp"`
}

// ─────────────────────────────────────────────────────────────────────────────
// SEGMENTATION THRESHOLDS
// ─────────────────────────────────────────────────────────────────────────────

const (
	vipMinOrders    = 10
	vipMinSpent     = 500.0
	atRiskMaxOrders = 2
	newMaxDays      = 30
)

// ─────────────────────────────────────────────────────────────────────────────
// CREATE
// ─────────────────────────────────────────────────────────────────────────────

// CreateCustomer creates a new customer record.
func (s *CustomerService) CreateCustomer(
	ctx context.Context,
	in CreateCustomerInput,
) (*models.Customer, error) {
	// Check duplicate by email or phone
	if in.Email != "" || in.Phone != "" {
		q := s.db.WithContext(ctx).Model(&models.Customer{}).
			Where("tenant_id = ? AND is_active = true", in.TenantID)
		if in.Email != "" && in.Phone != "" {
			q = q.Where("email = ? OR phone = ?", in.Email, in.Phone)
		} else if in.Email != "" {
			q = q.Where("email = ?", in.Email)
		} else {
			q = q.Where("phone = ?", in.Phone)
		}
		var count int64
		q.Count(&count)
		if count > 0 {
			return nil, ErrCustomerDuplicate
		}
	}

	customer := &models.Customer{
		CustomerID:    uuid.New(),
		TenantID:      in.TenantID,
		FirstName:     in.FirstName,
		LastName:      in.LastName,
		Email:         in.Email,
		Phone:         in.Phone,
		Address:       in.Address,
		City:          in.City,
		State:         in.State,
		PostalCode:    in.PostalCode,
		DateOfBirth:   in.DateOfBirth,
		LoyaltyPoints: 0,
		TotalOrders:   0,
		TotalSpent:    0,
		IsActive:      true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.db.WithContext(ctx).Omit(clause.Associations).Create(customer).Error; err != nil {
		return nil, fmt.Errorf("create customer: %w", err)
	}
	return customer, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// READ
// ─────────────────────────────────────────────────────────────────────────────

// GetCustomer returns a single customer by ID.
func (s *CustomerService) GetCustomer(
	ctx context.Context,
	tenantID uuid.UUID,
	customerID uuid.UUID,
) (*CustomerResponse, error) {
	var cust models.Customer
	if err := s.db.WithContext(ctx).
		Where("customer_id = ? AND tenant_id = ?", customerID, tenantID).
		First(&cust).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCustomerNotFound
		}
		return nil, fmt.Errorf("get customer: %w", err)
	}
	return s.toResponse(&cust), nil
}

// ListCustomers returns a paginated, filtered list of customers.
func (s *CustomerService) ListCustomers(
	ctx context.Context,
	f ListCustomersFilter,
) ([]CustomerResponse, int64, error) {
	q := s.db.WithContext(ctx).Model(&models.Customer{}).
		Where("tenant_id = ?", f.TenantID)

	if f.IsActive != nil {
		q = q.Where("is_active = ?", *f.IsActive)
	}
	if f.Search != nil && *f.Search != "" {
		search := "%" + strings.ToLower(*f.Search) + "%"
		q = q.Where("LOWER(first_name || ' ' || last_name) LIKE ? OR LOWER(email) LIKE ? OR phone LIKE ?",
			search, search, "%"+*f.Search+"%")
	}

	// Segment filter applied post-query on full set
	// For large datasets, this could be optimized with a materialized view
	applySegmentFilter := f.Segment != nil && *f.Segment != ""

	var total int64
	if !applySegmentFilter {
		if err := q.Count(&total).Error; err != nil {
			return nil, 0, err
		}
	}

	sortCol := "created_at"
	switch f.SortBy {
	case "name":
		sortCol = "first_name"
	case "total_spent":
		sortCol = "total_spent"
	case "total_orders":
		sortCol = "total_orders"
	case "loyalty_points":
		sortCol = "loyalty_points"
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

	var customers []models.Customer
	baseQ := q.Order(fmt.Sprintf("%s %s", sortCol, sortDir))

	if applySegmentFilter {
		// Load all, filter by segment, then paginate
		if err := baseQ.Find(&customers).Error; err != nil {
			return nil, 0, err
		}
		var filtered []models.Customer
		for i := range customers {
			if s.computeSegment(&customers[i]) == *f.Segment {
				filtered = append(filtered, customers[i])
			}
		}
		total = int64(len(filtered))
		start := (f.Page - 1) * f.PageSize
		end := start + f.PageSize
		if start > len(filtered) {
			start = len(filtered)
		}
		if end > len(filtered) {
			end = len(filtered)
		}
		customers = filtered[start:end]
	} else {
		if err := baseQ.Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize).
			Find(&customers).Error; err != nil {
			return nil, 0, err
		}
	}

	results := make([]CustomerResponse, len(customers))
	for i := range customers {
		results[i] = *s.toResponse(&customers[i])
	}
	return results, total, nil
}

// SearchCustomers performs a quick search by name, email, or phone.
func (s *CustomerService) SearchCustomers(
	ctx context.Context,
	tenantID uuid.UUID,
	query string,
	limit int,
) ([]CustomerResponse, error) {
	if limit <= 0 {
		limit = 10
	}
	search := "%" + strings.ToLower(query) + "%"

	var customers []models.Customer
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND is_active = true", tenantID).
		Where("LOWER(first_name || ' ' || last_name) LIKE ? OR LOWER(email) LIKE ? OR phone LIKE ?",
			search, search, "%"+query+"%").
		Order("total_orders DESC").
		Limit(limit).
		Find(&customers).Error; err != nil {
		return nil, err
	}

	results := make([]CustomerResponse, len(customers))
	for i := range customers {
		results[i] = *s.toResponse(&customers[i])
	}
	return results, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// UPDATE
// ─────────────────────────────────────────────────────────────────────────────

// UpdateCustomer updates mutable fields on a customer.
func (s *CustomerService) UpdateCustomer(
	ctx context.Context,
	in UpdateCustomerInput,
) (*models.Customer, error) {
	var cust models.Customer
	if err := s.db.WithContext(ctx).
		Where("customer_id = ? AND tenant_id = ?", in.CustomerID, in.TenantID).
		First(&cust).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCustomerNotFound
		}
		return nil, fmt.Errorf("get customer: %w", err)
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
	if in.Address != nil {
		updates["address"] = *in.Address
	}
	if in.City != nil {
		updates["city"] = *in.City
	}
	if in.State != nil {
		updates["state"] = *in.State
	}
	if in.PostalCode != nil {
		updates["postal_code"] = *in.PostalCode
	}
	updates["updated_at"] = time.Now()

	if err := s.db.WithContext(ctx).Model(&cust).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update customer: %w", err)
	}

	return &cust, nil
}

// DeactivateCustomer soft-deactivates a customer.
func (s *CustomerService) DeactivateCustomer(
	ctx context.Context,
	tenantID uuid.UUID,
	customerID uuid.UUID,
) error {
	result := s.db.WithContext(ctx).Model(&models.Customer{}).
		Where("customer_id = ? AND tenant_id = ? AND is_active = true", customerID, tenantID).
		Updates(map[string]interface{}{
			"is_active":  false,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCustomerNotFound
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// LOYALTY POINTS
// ─────────────────────────────────────────────────────────────────────────────

// AwardPoints adds loyalty points to a customer.
func (s *CustomerService) AwardPoints(
	ctx context.Context,
	tenantID uuid.UUID,
	customerID uuid.UUID,
	points int,
	reason string,
) (*LoyaltyTransaction, error) {
	if points <= 0 {
		return nil, errors.New("points must be positive")
	}

	var cust models.Customer
	if err := s.db.WithContext(ctx).
		Where("customer_id = ? AND tenant_id = ?", customerID, tenantID).
		First(&cust).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCustomerNotFound
		}
		return nil, err
	}

	newBalance := cust.LoyaltyPoints + points
	if err := s.db.WithContext(ctx).Model(&cust).Updates(map[string]interface{}{
		"loyalty_points": newBalance,
		"updated_at":     time.Now(),
	}).Error; err != nil {
		return nil, err
	}

	return &LoyaltyTransaction{
		CustomerID: customerID,
		Points:     points,
		Reason:     reason,
		Balance:    newBalance,
		Timestamp:  time.Now(),
	}, nil
}

// RedeemPoints deducts loyalty points from a customer.
func (s *CustomerService) RedeemPoints(
	ctx context.Context,
	tenantID uuid.UUID,
	customerID uuid.UUID,
	points int,
	reason string,
) (*LoyaltyTransaction, error) {
	if points <= 0 {
		return nil, errors.New("points must be positive")
	}

	var cust models.Customer
	if err := s.db.WithContext(ctx).
		Where("customer_id = ? AND tenant_id = ?", customerID, tenantID).
		First(&cust).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCustomerNotFound
		}
		return nil, err
	}

	if cust.LoyaltyPoints < points {
		return nil, ErrInvalidPoints
	}

	newBalance := cust.LoyaltyPoints - points
	if err := s.db.WithContext(ctx).Model(&cust).Updates(map[string]interface{}{
		"loyalty_points": newBalance,
		"updated_at":     time.Now(),
	}).Error; err != nil {
		return nil, err
	}

	return &LoyaltyTransaction{
		CustomerID: customerID,
		Points:     -points,
		Reason:     reason,
		Balance:    newBalance,
		Timestamp:  time.Now(),
	}, nil
}

// RecordOrderForCustomer updates the customer's order count and total spent.
// Called by the order service after order completion.
func (s *CustomerService) RecordOrderForCustomer(
	ctx context.Context,
	tenantID uuid.UUID,
	customerID uuid.UUID,
	orderAmount float64,
) error {
	result := s.db.WithContext(ctx).Model(&models.Customer{}).
		Where("customer_id = ? AND tenant_id = ?", customerID, tenantID).
		Updates(map[string]interface{}{
			"total_orders": gorm.Expr("total_orders + 1"),
			"total_spent":  gorm.Expr("total_spent + ?", orderAmount),
			"updated_at":   time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCustomerNotFound
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// STATISTICS
// ─────────────────────────────────────────────────────────────────────────────

// GetCustomerStats returns aggregate customer metrics for a tenant.
func (s *CustomerService) GetCustomerStats(
	ctx context.Context,
	tenantID uuid.UUID,
) (*CustomerStats, error) {
	stats := &CustomerStats{}

	var total, active int64
	s.db.WithContext(ctx).Model(&models.Customer{}).Where("tenant_id = ?", tenantID).Count(&total)
	s.db.WithContext(ctx).Model(&models.Customer{}).
		Where("tenant_id = ? AND is_active = true", tenantID).Count(&active)

	stats.TotalCustomers = int(total)
	stats.ActiveCustomers = int(active)

	// Revenue and averages
	type agg struct {
		TotalRev    float64
		AvgSpent    float64
		AvgOrderVal float64
	}
	var a agg
	s.db.WithContext(ctx).Model(&models.Customer{}).
		Select(`COALESCE(SUM(total_spent), 0) AS total_rev,
			COALESCE(AVG(total_spent), 0) AS avg_spent,
			COALESCE(AVG(CASE WHEN total_orders > 0 THEN total_spent / total_orders ELSE 0 END), 0) AS avg_order_val`).
		Where("tenant_id = ? AND is_active = true", tenantID).
		Scan(&a)
	stats.TotalRevenue = math.Round(a.TotalRev*100) / 100
	stats.AvgLifetimeVal = math.Round(a.AvgSpent*100) / 100
	stats.AvgOrderValue = math.Round(a.AvgOrderVal*100) / 100

	// VIP count
	var vipCount int64
	s.db.WithContext(ctx).Model(&models.Customer{}).
		Where("tenant_id = ? AND is_active = true AND total_orders >= ? AND total_spent >= ?",
			tenantID, vipMinOrders, vipMinSpent).
		Count(&vipCount)
	stats.VIPCount = int(vipCount)

	// At-risk count
	var atRiskCount int64
	s.db.WithContext(ctx).Model(&models.Customer{}).
		Where("tenant_id = ? AND is_active = true AND total_orders <= ?",
			tenantID, atRiskMaxOrders).
		Count(&atRiskCount)
	stats.AtRiskCount = int(atRiskCount)

	// New this month
	monthStart := time.Now().Truncate(24*time.Hour).AddDate(0, 0, -time.Now().Day()+1)
	var newCount int64
	s.db.WithContext(ctx).Model(&models.Customer{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, monthStart).
		Count(&newCount)
	stats.NewThisMonth = int(newCount)

	return stats, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────

func (s *CustomerService) computeSegment(c *models.Customer) string {
	daysSinceCreation := time.Since(c.CreatedAt).Hours() / 24
	if daysSinceCreation <= newMaxDays {
		return "new"
	}
	if c.TotalOrders >= vipMinOrders && c.TotalSpent >= vipMinSpent {
		return "vip"
	}
	if c.TotalOrders <= atRiskMaxOrders {
		return "at_risk"
	}
	return "regular"
}

func (s *CustomerService) toResponse(c *models.Customer) *CustomerResponse {
	resp := &CustomerResponse{
		CustomerID:    c.CustomerID,
		TenantID:      c.TenantID,
		FirstName:     c.FirstName,
		LastName:      c.LastName,
		FullName:      c.FirstName + " " + c.LastName,
		Email:         c.Email,
		Phone:         c.Phone,
		Address:       c.Address,
		City:          c.City,
		State:         c.State,
		PostalCode:    c.PostalCode,
		DateOfBirth:   c.DateOfBirth,
		LoyaltyPoints: c.LoyaltyPoints,
		TotalOrders:   c.TotalOrders,
		TotalSpent:    c.TotalSpent,
		IsActive:      c.IsActive,
		MemberSince:   c.CreatedAt,
		Segment:       s.computeSegment(c),
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
	if c.TotalOrders > 0 {
		resp.AvgOrderValue = math.Round(c.TotalSpent/float64(c.TotalOrders)*100) / 100
	}
	return resp
}

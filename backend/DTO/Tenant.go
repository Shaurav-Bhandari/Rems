package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// TENANT DTOs
// ============================================================================

// CreateTenantRequest represents the request to create a new tenant
// This is used for simple single-outlet registration
type CreateTenantRequest struct {
	Name   string `json:"name" binding:"required,min=2,max=255"`
	Domain string `json:"domain" binding:"omitempty,max=255"`
	
	// Auto-create organization and branch with same info
	AutoCreateOrgAndBranch bool `json:"auto_create_org_and_branch" binding:"omitempty"`
}

// CreateTenantWithDetailsRequest represents full tenant setup with org and branch
// Use this when you want explicit control over organization and branch
type CreateTenantWithDetailsRequest struct {
	TenantName         string `json:"tenant_name" binding:"required,min=2,max=255"`
	Domain             string `json:"domain" binding:"omitempty,max=255"`
	
	// Organization details
	OrganizationName   string `json:"organization_name" binding:"required,min=2,max=255"`
	OrganizationDesc   string `json:"organization_description" binding:"omitempty,max=1000"`
	
	// Branch/Location details
	BranchName         string `json:"branch_name" binding:"required,min=2,max=255"`
	Address            string `json:"address" binding:"required"`
	City               string `json:"city" binding:"required,max=100"`
	State              string `json:"state" binding:"omitempty,max=100"`
	Country            string `json:"country" binding:"required,max=100"`
	ZipCode            string `json:"zip_code" binding:"omitempty,max=20"`
	PhoneNumber        string `json:"phone_number" binding:"required,max=20"`
	Email              string `json:"email" binding:"required,email,max=255"`
}

// UpdateTenantRequest represents the request to update a tenant
type UpdateTenantRequest struct {
	Name     *string `json:"name" binding:"omitempty,min=2,max=255"`
	Domain   *string `json:"domain" binding:"omitempty,max=255"`
	Status   *string `json:"status" binding:"omitempty,oneof=active inactive suspended"`
	IsActive *bool   `json:"is_active"`
}

// TenantResponse represents the response for a single tenant
type TenantResponse struct {
	TenantID  uuid.UUID  `json:"tenant_id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	Domain    string     `json:"domain"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// TenantDetailResponse includes organization and branch info
type TenantDetailResponse struct {
	TenantID      uuid.UUID                 `json:"tenant_id"`
	Name          string                    `json:"name"`
	Status        string                    `json:"status"`
	Domain        string                    `json:"domain"`
	IsActive      bool                      `json:"is_active"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
	Organizations []OrganizationSummaryDTO  `json:"organizations,omitempty"`
	Subscription  *SubscriptionSummaryDTO   `json:"subscription,omitempty"`
	Stats         *TenantStatsDTO           `json:"stats,omitempty"`
}

// TenantSummaryDTO represents minimal tenant information
type TenantSummaryDTO struct {
	TenantID uuid.UUID `json:"tenant_id"`
	Name     string    `json:"name"`
	Status   string    `json:"status"`
	IsActive bool      `json:"is_active"`
}

// TenantListResponse represents a paginated list of tenants
type TenantListResponse struct {
	Tenants    []TenantResponse `json:"tenants"`
	Total      int64            `json:"total"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}

// TenantFilterRequest represents filter options for listing tenants
type TenantFilterRequest struct {
	Status    *string `form:"status" binding:"omitempty,oneof=active inactive suspended"`
	IsActive  *bool   `form:"is_active"`
	Search    *string `form:"search"` // search by name or domain
	Page      int     `form:"page" binding:"min=1"`
	PageSize  int     `form:"page_size" binding:"min=1,max=100"`
	SortBy    string  `form:"sort_by" binding:"omitempty,oneof=name created_at updated_at status"`
	SortOrder string  `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// TenantStatsDTO represents tenant statistics
type TenantStatsDTO struct {
	TotalOrganizations int     `json:"total_organizations"`
	TotalBranches      int     `json:"total_branches"`
	TotalRestaurants   int     `json:"total_restaurants"`
	TotalUsers         int     `json:"total_users"`
	TotalOrders        int64   `json:"total_orders"`
	TotalRevenue       float64 `json:"total_revenue"`
	MonthlyRevenue     float64 `json:"monthly_revenue"`
}

// ============================================================================
// ORGANIZATION DTOs
// ============================================================================

// CreateOrganizationRequest represents the request to create a new organization
type CreateOrganizationRequest struct {
	Name        string `json:"name" binding:"required,min=2,max=255"`
	Description string `json:"description" binding:"omitempty,max=1000"`
	
	// Auto-create first branch with same info
	AutoCreateBranch bool   `json:"auto_create_branch" binding:"omitempty"`
	BranchName       string `json:"branch_name" binding:"omitempty,max=255"`
	Address          string `json:"address" binding:"omitempty"`
	City             string `json:"city" binding:"omitempty,max=100"`
	State            string `json:"state" binding:"omitempty,max=100"`
	Country          string `json:"country" binding:"omitempty,max=100"`
	ZipCode          string `json:"zip_code" binding:"omitempty,max=20"`
	PhoneNumber      string `json:"phone_number" binding:"omitempty,max=20"`
	Email            string `json:"email" binding:"omitempty,email,max=255"`
}

// UpdateOrganizationRequest represents the request to update an organization
type UpdateOrganizationRequest struct {
	Name        *string `json:"name" binding:"omitempty,min=2,max=255"`
	Description *string `json:"description" binding:"omitempty,max=1000"`
}

// OrganizationResponse represents the response for a single organization
type OrganizationResponse struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// OrganizationDetailResponse includes branches
type OrganizationDetailResponse struct {
	OrganizationID uuid.UUID          `json:"organization_id"`
	TenantID       uuid.UUID          `json:"tenant_id"`
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Branches       []BranchSummaryDTO `json:"branches,omitempty"`
	Stats          *OrganizationStatsDTO `json:"stats,omitempty"`
}

// OrganizationSummaryDTO represents minimal organization information
type OrganizationSummaryDTO struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	Name           string    `json:"name"`
	BranchCount    int       `json:"branch_count,omitempty"`
}

// OrganizationListResponse represents a paginated list of organizations
type OrganizationListResponse struct {
	Organizations []OrganizationResponse `json:"organizations"`
	Total         int64                  `json:"total"`
	Page          int                    `json:"page"`
	PageSize      int                    `json:"page_size"`
	TotalPages    int                    `json:"total_pages"`
}

// OrganizationStatsDTO represents organization statistics
type OrganizationStatsDTO struct {
	TotalBranches    int     `json:"total_branches"`
	TotalRestaurants int     `json:"total_restaurants"`
	TotalEmployees   int     `json:"total_employees"`
	TotalOrders      int64   `json:"total_orders"`
	TotalRevenue     float64 `json:"total_revenue"`
}

// ============================================================================
// BRANCH DTOs
// ============================================================================

// CreateBranchRequest represents the request to create a new branch
type CreateBranchRequest struct {
	OrganizationID uuid.UUID `json:"organization_id" binding:"required"`
	Name           string    `json:"name" binding:"required,min=2,max=255"`
	Address        string    `json:"address" binding:"required"`
	City           string    `json:"city" binding:"required,max=100"`
	State          string    `json:"state" binding:"omitempty,max=100"`
	Country        string    `json:"country" binding:"required,max=100"`
	ZipCode        string    `json:"zip_code" binding:"omitempty,max=20"`
	PhoneNumber    string    `json:"phone_number" binding:"required,max=20"`
	Email          string    `json:"email" binding:"required,email,max=255"`
	
	// Auto-create restaurant for this branch
	AutoCreateRestaurant bool `json:"auto_create_restaurant" binding:"omitempty"`
}

// UpdateBranchRequest represents the request to update a branch
type UpdateBranchRequest struct {
	Name        *string `json:"name" binding:"omitempty,min=2,max=255"`
	Address     *string `json:"address"`
	City        *string `json:"city" binding:"omitempty,max=100"`
	State       *string `json:"state" binding:"omitempty,max=100"`
	Country     *string `json:"country" binding:"omitempty,max=100"`
	ZipCode     *string `json:"zip_code" binding:"omitempty,max=20"`
	PhoneNumber *string `json:"phone_number" binding:"omitempty,max=20"`
	Email       *string `json:"email" binding:"omitempty,email,max=255"`
}

// BranchResponse represents the response for a single branch
type BranchResponse struct {
	BranchID       uuid.UUID `json:"branch_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	Name           string    `json:"name"`
	Address        string    `json:"address"`
	City           string    `json:"city"`
	State          string    `json:"state"`
	Country        string    `json:"country"`
	ZipCode        string    `json:"zip_code"`
	PhoneNumber    string    `json:"phone_number"`
	Email          string    `json:"email"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// BranchDetailResponse includes restaurants and stats
type BranchDetailResponse struct {
	BranchID       uuid.UUID                `json:"branch_id"`
	OrganizationID uuid.UUID                `json:"organization_id"`
	TenantID       uuid.UUID                `json:"tenant_id"`
	Name           string                   `json:"name"`
	Address        string                   `json:"address"`
	City           string                   `json:"city"`
	State          string                   `json:"state"`
	Country        string                   `json:"country"`
	ZipCode        string                   `json:"zip_code"`
	PhoneNumber    string                   `json:"phone_number"`
	Email          string                   `json:"email"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
	Restaurants    []RestaurantSummaryDTO   `json:"restaurants,omitempty"`
	Stats          *BranchStatsDTO          `json:"stats,omitempty"`
}

// BranchSummaryDTO represents minimal branch information
type BranchSummaryDTO struct {
	BranchID    uuid.UUID `json:"branch_id"`
	Name        string    `json:"name"`
	City        string    `json:"city"`
	PhoneNumber string    `json:"phone_number"`
}

// BranchListResponse represents a paginated list of branches
type BranchListResponse struct {
	Branches   []BranchResponse `json:"branches"`
	Total      int64            `json:"total"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}

// BranchFilterRequest represents filter options for listing branches
type BranchFilterRequest struct {
	OrganizationID *uuid.UUID `form:"organization_id"`
	City           *string    `form:"city"`
	State          *string    `form:"state"`
	Country        *string    `form:"country"`
	Search         *string    `form:"search"` // search by name, address, or city
	Page           int        `form:"page" binding:"min=1"`
	PageSize       int        `form:"page_size" binding:"min=1,max=100"`
	SortBy         string     `form:"sort_by" binding:"omitempty,oneof=name city created_at"`
	SortOrder      string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// BranchStatsDTO represents branch statistics
type BranchStatsDTO struct {
	TotalRestaurants int     `json:"total_restaurants"`
	TotalEmployees   int     `json:"total_employees"`
	TotalOrders      int64   `json:"total_orders"`
	TotalRevenue     float64 `json:"total_revenue"`
	TodayOrders      int     `json:"today_orders"`
	TodayRevenue     float64 `json:"today_revenue"`
}

// ============================================================================
// SUBSCRIPTION SUMMARY DTO (referenced in TenantDetailResponse)
// ============================================================================

// SubscriptionSummaryDTO represents minimal subscription information
type SubscriptionSummaryDTO struct {
	SubscriptionID uuid.UUID  `json:"subscription_id"`
	PlanName       string     `json:"plan_name"`
	Status         string     `json:"status"`
	StartDate      time.Time  `json:"start_date"`
	EndDate        *time.Time `json:"end_date,omitempty"`
	AutoRenew      bool       `json:"auto_renew"`
}

// ============================================================================
// SIMPLIFIED SETUP DTO (For Single Restaurant Scenario)
// ============================================================================

// SimplifiedSetupRequest is for simple single-restaurant registration
// This auto-creates: Tenant -> Organization -> Branch -> Restaurant
type SimplifiedSetupRequest struct {
	// Business Info (used for Tenant and Organization)
	BusinessName string `json:"business_name" binding:"required,min=2,max=255"`
	Domain       string `json:"domain" binding:"omitempty,max=255"`
	
	// Location Info (used for Branch and Restaurant)
	LocationName string `json:"location_name" binding:"required,min=2,max=255"` // e.g., "Main Branch" or restaurant name
	Address      string `json:"address" binding:"required"`
	City         string `json:"city" binding:"required,max=100"`
	State        string `json:"state" binding:"omitempty,max=100"`
	Country      string `json:"country" binding:"required,max=100"`
	ZipCode      string `json:"zip_code" binding:"omitempty,max=20"`
	PhoneNumber  string `json:"phone_number" binding:"required,max=20"`
	Email        string `json:"email" binding:"required,email,max=255"`
	
	// Owner/Admin User Info
	OwnerFullName string `json:"owner_full_name" binding:"required,min=2,max=255"`
	OwnerEmail    string `json:"owner_email" binding:"required,email,max=255"`
	OwnerPassword string `json:"owner_password" binding:"required,min=8"`
}

// SimplifiedSetupResponse returns all created entities
type SimplifiedSetupResponse struct {
	Tenant       TenantResponse       `json:"tenant"`
	Organization OrganizationResponse `json:"organization"`
	Branch       BranchResponse       `json:"branch"`
	Restaurant   RestaurantResponse   `json:"restaurant"`
	Owner        UserResponse         `json:"owner"`
	AccessToken  string               `json:"access_token"`
	RefreshToken string               `json:"refresh_token"`
	Message      string               `json:"message"`
}

// ============================================================================
// VALIDATION HELPERS
// ============================================================================

// Validate validates the CreateTenantWithDetailsRequest
func (r *CreateTenantWithDetailsRequest) Validate() error {
	// Add custom validation logic here if needed
	return nil
}

// Validate validates the SimplifiedSetupRequest
func (r *SimplifiedSetupRequest) Validate() error {
	// Add custom validation logic here if needed
	return nil
}
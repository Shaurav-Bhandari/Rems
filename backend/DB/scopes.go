
package DB

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ============================================================================
// TENANT ISOLATION SCOPES - CRITICAL FOR MULTI-TENANCY
// ============================================================================

// WithTenantID applies tenant isolation to queries
// CRITICAL: This MUST be applied to ALL tenant-scoped queries to prevent data leaks
func WithTenantID(tenantID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("tenant_id = ?", tenantID)
	}
}

// WithTenantIDOrFail applies tenant isolation and returns error if tenantID is nil
// Use this when tenantID is absolutely required
func WithTenantIDOrFail(tenantID *uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if tenantID == nil || *tenantID == uuid.Nil {
			db.AddError(fmt.Errorf("tenant_id is required but was not provided"))
			return db
		}
		return db.Where("tenant_id = ?", *tenantID)
	}
}

// WithRestaurantID applies restaurant-level isolation
func WithRestaurantID(restaurantID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("restaurant_id = ?", restaurantID)
	}
}

// WithBranchID applies branch-level isolation
func WithBranchID(branchID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("branch_id = ?", branchID)
	}
}

// WithOrganizationID applies organization-level isolation
func WithOrganizationID(organizationID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("organization_id = ?", organizationID)
	}
}

// WithTenantAndRestaurant applies both tenant and restaurant isolation
// Use this for restaurant-scoped resources
func WithTenantAndRestaurant(tenantID, restaurantID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("tenant_id = ? AND restaurant_id = ?", tenantID, restaurantID)
	}
}

// WithTenantAndBranch applies both tenant and branch isolation
func WithTenantAndBranch(tenantID, branchID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("tenant_id = ? AND branch_id = ?", tenantID, branchID)
	}
}

// ============================================================================
// SOFT DELETE SCOPES
// ============================================================================

// WithoutDeleted excludes soft-deleted records (is_deleted = false)
func WithoutDeleted() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_deleted = ?", false)
	}
}

// WithDeleted includes soft-deleted records
func WithDeleted() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Unscoped()
	}
}

// OnlyDeleted returns only soft-deleted records
func OnlyDeleted() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_deleted = ?", true)
	}
}

// ============================================================================
// ACTIVE/INACTIVE SCOPES
// ============================================================================

// WithActive returns only active records (is_active = true)
func WithActive() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_active = ?", true)
	}
}

// WithInactive returns only inactive records
func WithInactive() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_active = ?", false)
	}
}

// WithIsActive filters by active status (allows dynamic filtering)
func WithIsActive(isActive bool) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_active = ?", isActive)
	}
}

// ============================================================================
// STATUS SCOPES
// ============================================================================

// WithStatus filters by status field
func WithStatus(status string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("status = ?", status)
	}
}

// WithOrderStatus filters orders by status
func WithOrderStatus(status string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("order_status = ?", status)
	}
}

// WithTableStatus filters tables by status
func WithTableStatus(status string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("status = ?", status)
	}
}

// WithStatusIn filters by multiple statuses
func WithStatusIn(statuses []string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if len(statuses) == 0 {
			return db
		}
		return db.Where("status IN ?", statuses)
	}
}

// ============================================================================
// DATE RANGE SCOPES
// ============================================================================

// WithDateRange filters by date range on created_at
func WithDateRange(from, to time.Time) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("created_at BETWEEN ? AND ?", from, to)
	}
}

// WithDateFrom filters records created after a date
func WithDateFrom(from time.Time) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("created_at >= ?", from)
	}
}

// WithDateTo filters records created before a date
func WithDateTo(to time.Time) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("created_at <= ?", to)
	}
}

// WithCustomDateRange filters by date range on a custom field
func WithCustomDateRange(field string, from, to *time.Time) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if from != nil && to != nil {
			return db.Where(fmt.Sprintf("%s BETWEEN ? AND ?", field), from, to)
		} else if from != nil {
			return db.Where(fmt.Sprintf("%s >= ?", field), from)
		} else if to != nil {
			return db.Where(fmt.Sprintf("%s <= ?", field), to)
		}
		return db
	}
}

// WithToday filters records created today
func WithToday() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		now := time.Now()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		endOfDay := startOfDay.Add(24 * time.Hour)
		return db.Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay)
	}
}

// WithThisWeek filters records created this week
func WithThisWeek() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		now := time.Now()
		// Start of week (Monday)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday becomes 7
		}
		startOfWeek := now.AddDate(0, 0, -weekday+1)
		startOfWeek = time.Date(startOfWeek.Year(), startOfWeek.Month(), startOfWeek.Day(), 0, 0, 0, 0, startOfWeek.Location())
		endOfWeek := startOfWeek.AddDate(0, 0, 7)
		return db.Where("created_at >= ? AND created_at < ?", startOfWeek, endOfWeek)
	}
}

// WithThisMonth filters records created this month
func WithThisMonth() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		now := time.Now()
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		endOfMonth := startOfMonth.AddDate(0, 1, 0)
		return db.Where("created_at >= ? AND created_at < ?", startOfMonth, endOfMonth)
	}
}

// ============================================================================
// SEARCH SCOPES
// ============================================================================

// WithSearch performs case-insensitive search on specified fields
func WithSearch(search string, fields ...string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if search == "" || len(fields) == 0 {
			return db
		}
		
		query := db
		searchPattern := "%" + search + "%"
		
		// Build OR conditions for all fields
		for i, field := range fields {
			if i == 0 {
				query = query.Where(fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", field), searchPattern)
			} else {
				query = query.Or(fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", field), searchPattern)
			}
		}
		
		return query
	}
}

// WithExactMatch performs exact match on a field
func WithExactMatch(field string, value interface{}) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(fmt.Sprintf("%s = ?", field), value)
	}
}

// ============================================================================
// PAGINATION SCOPES
// ============================================================================

// WithPagination applies limit and offset for pagination
func WithPagination(page, pageSize int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if page < 1 {
			page = 1
		}
		if pageSize < 1 {
			pageSize = 10
		}
		if pageSize > 100 {
			pageSize = 100 // Max page size
		}
		
		offset := (page - 1) * pageSize
		return db.Offset(offset).Limit(pageSize)
	}
}

// WithLimit applies a limit
func WithLimit(limit int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if limit < 1 {
			limit = 10
		}
		return db.Limit(limit)
	}
}

// WithOffset applies an offset
func WithOffset(offset int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if offset < 0 {
			offset = 0
		}
		return db.Offset(offset)
	}
}

// ============================================================================
// SORTING SCOPES
// ============================================================================

// WithOrderBy applies ordering
func WithOrderBy(field, order string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if field == "" {
			return db
		}
		
		// Validate order direction
		if order != "asc" && order != "desc" {
			order = "desc"
		}
		
		return db.Order(fmt.Sprintf("%s %s", field, order))
	}
}

// WithDefaultOrder applies default ordering (created_at desc)
func WithDefaultOrder() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at DESC")
	}
}

// ============================================================================
// RELATIONSHIP LOADING SCOPES
// ============================================================================

// WithPreload preloads specified associations
func WithPreload(associations ...string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		for _, assoc := range associations {
			db = db.Preload(assoc)
		}
		return db
	}
}

// WithPreloadAll preloads all associations
func WithPreloadAll() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Preload("*")
	}
}

// ============================================================================
// RESOURCE-SPECIFIC SCOPES
// ============================================================================

// Order Scopes

// WithCustomerID filters orders by customer
func WithCustomerID(customerID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("customer_id = ?", customerID)
	}
}

// WithTableID filters orders by table
func WithTableID(tableID int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("table_id = ?", tableID)
	}
}

// WithActiveOrders filters for active orders (not completed/cancelled)
func WithActiveOrders() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("order_status NOT IN ?", []string{"completed", "cancelled"})
	}
}

// Table Scopes

// WithAvailableTables filters for available tables
func WithAvailableTables() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("status = ?", "available")
	}
}

// WithFloorID filters tables by floor
func WithFloorID(floorID int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("floor_id = ?", floorID)
	}
}

// WithCapacity filters tables by minimum capacity
func WithCapacity(minCapacity int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("capacity >= ?", minCapacity)
	}
}

// Menu Scopes

// WithMenuCategoryID filters menu items by category
func WithMenuCategoryID(categoryID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("menu_category_id = ?", categoryID)
	}
}

// WithAvailableMenuItems filters for available menu items
func WithAvailableMenuItems() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_available = ?", true)
	}
}

// Inventory Scopes

// WithLowStock filters inventory items that are low on stock
func WithLowStock() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("current_quantity <= reorder_point")
	}
}

// WithOutOfStock filters inventory items that are out of stock
func WithOutOfStock() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("current_quantity = 0")
	}
}

// User Scopes

// WithUserID filters by user ID
func WithUserID(userID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("user_id = ?", userID)
	}
}

// WithEmail filters by email
func WithEmail(email string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("email = ?", email)
	}
}

// WithRoleID filters users by role
func WithRoleID(roleID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("default_role_id = ?", roleID)
	}
}

// ============================================================================
// AGGREGATION SCOPES
// ============================================================================

// WithCount adds a count to the query
func WithCount(field string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Select(fmt.Sprintf("COUNT(%s) as count", field))
	}
}

// WithSum adds a sum to the query
func WithSum(field string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Select(fmt.Sprintf("SUM(%s) as total", field))
	}
}

// WithAverage adds an average to the query
func WithAverage(field string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Select(fmt.Sprintf("AVG(%s) as average", field))
	}
}

// ============================================================================
// COMBINED COMMON SCOPES
// ============================================================================

// DefaultTenantScope combines common tenant query requirements
// Use this as the base for most tenant-scoped queries
func DefaultTenantScope(tenantID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Scopes(
			WithTenantID(tenantID),
			WithoutDeleted(),
			WithActive(),
		)
	}
}

// DefaultRestaurantScope combines common restaurant query requirements
func DefaultRestaurantScope(tenantID, restaurantID uuid.UUID) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Scopes(
			WithTenantAndRestaurant(tenantID, restaurantID),
			WithoutDeleted(),
			WithActive(),
		)
	}
}

// ListScope combines pagination and ordering for list queries
func ListScope(page, pageSize int, sortBy, sortOrder string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Scopes(
			WithPagination(page, pageSize),
			WithOrderBy(sortBy, sortOrder),
		)
	}
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

// ApplyScopesIf applies scopes conditionally
func ApplyScopesIf(condition bool, scopes ...func(*gorm.DB) *gorm.DB) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if condition {
			return db.Scopes(scopes...)
		}
		return db
	}
}

// CalculateTotalPages calculates total pages for pagination
func CalculateTotalPages(total int64, pageSize int) int {
	if pageSize <= 0 {
		pageSize = 10
	}
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}
	return totalPages
}
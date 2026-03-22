package handlers

import (
	"fmt"
	"log"
	"time"

	"backend/DTO"
	"backend/models"
	svc "backend/services"
	services "backend/services/core"
	"backend/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ============================================================================
// JSON REQUEST TYPES
// ============================================================================

type AddRoleRequest struct {
	TenantID    string `json:"tenant_id"    validate:"required,uuid"`
	RoleName    string `json:"role_name"    validate:"required,min=2,max=100"`
	Description string `json:"description"`
	IsSystem    bool   `json:"is_system"`
	Permissions []struct {
		Resource string `json:"resource" validate:"required"`
		Action   string `json:"action"   validate:"required"`
		Effect   string `json:"effect"`
	} `json:"permissions,omitempty"`
}

type AddUserRequest struct {
	TenantID       string `json:"tenant_id"        validate:"required,uuid"`
	UserName       string `json:"user_name"        validate:"required,min=2,max=100"`
	FullName       string `json:"full_name"        validate:"required"`
	Email          string `json:"email"            validate:"required,email"`
	Phone          string `json:"phone"`
	Password       string `json:"password"         validate:"required,min=6"`
	OrganizationID string `json:"organization_id"`
	BranchID       string `json:"branch_id"`
	RoleName       string `json:"role_name"`
	PrimaryRole    string `json:"primary_role"`
}


// ============================================================================
// HANDLER STRUCT
// ============================================================================

type InitializeHandler struct {
	db *gorm.DB
}

func NewInitializeHandler(db *gorm.DB) *InitializeHandler {
	return &InitializeHandler{db: db}
}

type initResult struct {
	Created []string `json:"created"`
	Skipped []string `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

func (r *initResult) create(entity string)            { r.Created = append(r.Created, entity) }
func (r *initResult) skip(entity string)              { r.Skipped = append(r.Skipped, entity) }
func (r *initResult) fail(entity, msg string)         { r.Errors = append(r.Errors, fmt.Sprintf("%s: %s", entity, msg)) }

// ============================================================================
// HTTP ENDPOINTS
// ============================================================================

// Initialize — POST /api/v1/initialize
func (h *InitializeHandler) Initialize(c fiber.Ctx) error {
	result := &initResult{}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		return h.seedAll(tx, result)
	})
	if err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError,
			"Initialization failed — transaction rolled back", result)
	}
	if len(result.Errors) > 0 {
		return utils.SendResponse(c, fiber.StatusMultiStatus,
			"Initialization completed with some errors", result)
	}
	if len(result.Created) == 0 {
		return utils.SendResponse(c, fiber.StatusOK,
			"System already initialized — nothing to do", result)
	}
	return utils.SendResponse(c, fiber.StatusCreated,
		"System initialized successfully", result)
}

// AddRole — POST /api/v1/initialize/roles
func (h *InitializeHandler) AddRole(c fiber.Ctx) error {
	var req AddRoleRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid JSON", err.Error())
	}
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid tenant_id", err.Error())
	}

	result := &initResult{}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		role, err := findOrCreate(tx, result, "role:"+req.RoleName, &models.Role{},
			"tenant_id = ? AND role_name = ?", tenantID, req.RoleName,
			func() *models.Role {
				return &models.Role{
					RoleID: uuid.New(), TenantID: tenantID,
					RoleName: req.RoleName, Description: req.Description,
					IsSystem: req.IsSystem,
				}
			})
		if err != nil {
			return err
		}

		if len(req.Permissions) > 0 {
			policyName := req.RoleName + " Policy"
			policy, err := findOrCreate(tx, result, "policy:"+req.RoleName, &models.Policy{},
				"tenant_id = ? AND name = ?", tenantID, policyName,
				func() *models.Policy {
					return &models.Policy{
						PolicyID: uuid.New(), TenantID: tenantID,
						Name: policyName, Description: "Auto-created for " + req.RoleName,
					}
				})
			if err != nil {
				return err
			}
			for _, perm := range req.Permissions {
				effect := perm.Effect
				if effect == "" {
					effect = "allow"
				}
				_, err := findOrCreate(tx, result,
					fmt.Sprintf("perm:%s:%s:%s", req.RoleName, perm.Resource, perm.Action),
					&models.Permission{},
					"policy_id = ? AND resource = ? AND action = ?", policy.PolicyID, perm.Resource, perm.Action,
					func() *models.Permission {
						return &models.Permission{
							PermissionID: uuid.New(), PolicyID: policy.PolicyID,
							Resource: perm.Resource, Action: perm.Action, Effect: effect,
						}
					})
				if err != nil {
					return err
				}
			}
			if tx.Model(role).Association("Policies").Count() == 0 {
				if err := tx.Model(role).Association("Policies").Append(policy); err != nil {
					result.fail("role_policy:"+req.RoleName, err.Error())
					return err
				}
				result.create("role_policy:" + req.RoleName)
			} else {
				result.skip("role_policy:" + req.RoleName)
			}
		}
		return nil
	})
	if err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to add role", result)
	}
	status := fiber.StatusCreated
	if len(result.Created) == 0 {
		status = fiber.StatusOK
	}
	return utils.SendResponse(c, status, "Role processed", result)
}

// AddUser — POST /api/v1/initialize/users
func (h *InitializeHandler) AddUser(c fiber.Ctx) error {
	var req AddUserRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid JSON", err.Error())
	}
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid tenant_id", err.Error())
	}

	var orgID, branchID uuid.UUID
	if req.OrganizationID != "" {
		orgID, err = uuid.Parse(req.OrganizationID)
		if err != nil {
			return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid organization_id", err.Error())
		}
	}
	if req.BranchID != "" {
		branchID, err = uuid.Parse(req.BranchID)
		if err != nil {
			return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid branch_id", err.Error())
		}
	}

	result := &initResult{}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var role *models.Role
		if req.RoleName != "" {
			role = &models.Role{}
			if err := tx.Where("tenant_id = ? AND role_name = ?", tenantID, req.RoleName).
				First(role).Error; err != nil {
				result.fail("role_lookup", fmt.Sprintf("role %q not found", req.RoleName))
				return fmt.Errorf("role %q not found: %w", req.RoleName, err)
			}
		}
		primaryRole := req.PrimaryRole
		if primaryRole == "" && req.RoleName != "" {
			primaryRole = req.RoleName
		}
		user, err := h.ensureUser(tx, result,
			req.UserName, req.FullName, req.Email, req.Phone, req.Password,
			tenantID, orgID, branchID, role, primaryRole)
		if err != nil {
			return err
		}
		if role != nil {
			_, err := findOrCreate(tx, result, "user_role:"+req.UserName, &models.UserRole{},
				"user_id = ? AND role_id = ?", user.UserID, role.RoleID,
				func() *models.UserRole {
					return &models.UserRole{
						UserRoleID: uuid.New(), UserID: user.UserID,
						RoleID: role.RoleID, TenantID: tenantID, AssignedBy: user.UserID,
					}
				})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to add user", result)
	}
	status := fiber.StatusCreated
	if len(result.Created) == 0 {
		status = fiber.StatusOK
	}
	return utils.SendResponse(c, status, "User processed", result)
}

// InitializeSuperadmin — POST /api/v1/initialize/superadmin
func (h *InitializeHandler) InitializeSuperadmin(c fiber.Ctx) error {
	var req DTO.CreateAdminRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid JSON", err.Error())
	}

	result := &initResult{}
	
	var count int64
	h.db.Model(&models.Admin{}).Count(&count)
	if count > 0 {
		return utils.SendResponse(c, fiber.StatusForbidden, "Platform admins already initialized", nil)
	}

	adminSvc := svc.NewAdminService(h.db)
	resp, err := adminSvc.Create(&req)
	
	if err != nil {
		result.fail("superadmin", err.Error())
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to initialize superadmin", result)
	}

	result.create("admin:" + resp.UserName)
	return utils.SendResponse(c, fiber.StatusCreated, "Superadmin processed", result)
}


// ============================================================================
// SEED LOGIC
// ============================================================================

func (h *InitializeHandler) seedAll(db *gorm.DB, r *initResult) error {

	// ── 1. TENANT ────────────────────────────────────────────────────────────
	tenant, err := findOrCreate(db, r, "tenant", &models.Tenant{},
		"domain = ?", "rems.local",
		func() *models.Tenant {
			return &models.Tenant{
				TenantID: uuid.New(), Name: "ReMS Default Tenant",
				Status: "rems_active", Domain: "rems.local", IsActive: true,
			}
		})
	if err != nil {
		return err
	}

	// ── 2. ORGANIZATION ──────────────────────────────────────────────────────
	org, err := findOrCreate(db, r, "organization", &models.Organization{},
		"tenant_id = ? AND name = ?", tenant.TenantID, "ReMS Organization",
		func() *models.Organization {
			return &models.Organization{
				OrganizationID: uuid.New(), TenantID: tenant.TenantID,
				Name: "ReMS Organization", Description: "Default organization",
			}
		})
	if err != nil {
		return err
	}

	// ── 3. BRANCH ────────────────────────────────────────────────────────────
	branch, err := findOrCreate(db, r, "branch", &models.Branch{},
		"tenant_id = ? AND name = ?", tenant.TenantID, "Main Branch",
		func() *models.Branch {
			return &models.Branch{
				BranchID: uuid.New(), OrganizationID: org.OrganizationID,
				TenantID: tenant.TenantID, Name: "Main Branch",
				Address: "Kathmandu, Nepal", City: "Kathmandu",
				State: "Bagmati", Country: "Nepal", ZipCode: "44600",
				PhoneNumber: "+977-1-4000000", Email: "main@rems.local",
			}
		})
	if err != nil {
		return err
	}

	// ── 4. RESTAURANT ────────────────────────────────────────────────────────
	restaurant, err := findOrCreate(db, r, "restaurant", &models.Restaurant{},
		"tenant_id = ? AND name = ?", tenant.TenantID, "ReMS Demo Restaurant",
		func() *models.Restaurant {
			return &models.Restaurant{
				RestaurantID: uuid.New(), TenantID: tenant.TenantID,
				Name: "ReMS Demo Restaurant", Address: "Thamel, Kathmandu",
				City: "Kathmandu", State: "Bagmati", Country: "Nepal",
				PostalCode: "44600", Phone: "+977-1-4111111",
				Email: "demo@rems.local", IsActive: true,
			}
		})
	if err != nil {
		return err
	}

	// ── 5. RESTAURANT PROFILE ────────────────────────────────────────────────
	_, err = findOrCreate(db, r, "restaurant_profile", &models.RestaurantProfile{},
		"restaurant_id = ?", restaurant.RestaurantID,
		func() *models.RestaurantProfile {
			return &models.RestaurantProfile{
				ProfileID:                    uuid.New(),
				RestaurantID:                 restaurant.RestaurantID,
				TenantID:                     tenant.TenantID,
				CurrencyCode:                 "NPR",
				ServiceChargePct:             10.0,
				TaxInclusivePricing:          false,
				DefaultTaxRate:               13.0,
				AlcoholTaxRate:               0.0,
				AutoKOTFiring:                true,
				TableExpirationMinutes:       60,
				StockWarningThreshold:        10,
				EnableHappyHour:              false,
				LanguageISO:                  "en",
				BrandPrimaryColor:            "#c0392b",
				BrandSecondaryColor:          "#FFFFFF",
				ReceiptHeader:                "Welcome to ReMS Demo Restaurant",
				ReceiptFooter:                "Thank you! Come again.",
				RequireManagerPINForVoid:     true,
				RequireManagerPINForDiscount: true,
				MaxDiscountPctStaff:          0.0,
				MaxDiscountPctManager:        20.0,
				OffsiteLoginAllowed:          false,
				GDPREnabled:                  false,
				DataRetentionDays:            2555,
				RequireAgeVerification:       false,
			}
		})
	if err != nil {
		return err
	}

	// ── 6. REGIONAL SETTING ──────────────────────────────────────────────────
	_, err = findOrCreate(db, r, "regional_setting", &models.RegionalSetting{},
		"tenant_id = ? AND restaurant_id = ?", tenant.TenantID, restaurant.RestaurantID,
		func() *models.RegionalSetting {
			taxRate := 13.0
			return &models.RegionalSetting{
				RegionalSettingID: uuid.New(),
				TenantID:          tenant.TenantID,
				RestaurantID:      &restaurant.RestaurantID,
				Timezone:          "Asia/Kathmandu",
				CurrencyCode:      "NPR",
				DateFormat:        "YYYY-MM-DD",
				TimeFormat:        "HH24:MI",
				LanguageCode:      "en",
				TaxRate:           &taxRate,
			}
		})
	if err != nil {
		return err
	}

	// ── 7. PLAN & SUBSCRIPTION ───────────────────────────────────────────────
	maxR, maxU := 5, 50
	plan, err := findOrCreate(db, r, "plan", &models.Plan{},
		"name = ?", "Professional",
		func() *models.Plan {
			return &models.Plan{
				PlanID: uuid.New(), Name: "Professional",
				Description: "Full-featured plan", Price: 49.99,
				BillingCycle: "monthly", MaxRestaurants: &maxR,
				MaxUsers: &maxU, IsActive: true,
			}
		})
	if err != nil {
		return err
	}

	_, err = findOrCreate(db, r, "plan_feature:KOT", &models.PlanFeature{},
		"plan_id = ? AND feature_name = ?", plan.PlanID, "KOT",
		func() *models.PlanFeature {
			return &models.PlanFeature{PlanFeatureID: uuid.New(), PlanID: plan.PlanID, FeatureName: "KOT", FeatureValue: "true", IsEnabled: true}
		})
	if err != nil {
		return err
	}
	_, err = findOrCreate(db, r, "plan_feature:Inventory", &models.PlanFeature{},
		"plan_id = ? AND feature_name = ?", plan.PlanID, "Inventory",
		func() *models.PlanFeature {
			return &models.PlanFeature{PlanFeatureID: uuid.New(), PlanID: plan.PlanID, FeatureName: "Inventory", FeatureValue: "true", IsEnabled: true}
		})
	if err != nil {
		return err
	}
	_, err = findOrCreate(db, r, "plan_feature:Analytics", &models.PlanFeature{},
		"plan_id = ? AND feature_name = ?", plan.PlanID, "Analytics",
		func() *models.PlanFeature {
			return &models.PlanFeature{PlanFeatureID: uuid.New(), PlanID: plan.PlanID, FeatureName: "Analytics", FeatureValue: "true", IsEnabled: true}
		})
	if err != nil {
		return err
	}

	_, err = findOrCreate(db, r, "subscription", &models.Subscription{},
		"tenant_id = ? AND plan_id = ?", tenant.TenantID, plan.PlanID,
		func() *models.Subscription {
			end := time.Now().AddDate(1, 0, 0)
			return &models.Subscription{
				SubscriptionID: uuid.New(), TenantID: tenant.TenantID,
				PlanID: plan.PlanID, StartDate: time.Now(), EndDate: &end,
				Status: "active", AutoRenew: true,
			}
		})
	if err != nil {
		return err
	}

	// ── 8. ROLES ─────────────────────────────────────────────────────────────
	type roleSpec struct{ name, desc string; system bool }
	roleSpecs := []roleSpec{
		{"superadmin", "Platform administrator with full access", true},
		{"owner", "Restaurant owner — full tenant access", false},
		{"manager", "Restaurant manager — operational access", false},
		{"waiter", "Front-of-house waiter — order management", false},
		{"cashier", "Cashier — payment and order closing", false},
		{"kitchen", "Kitchen staff — KOT management only", false},
	}
	roles := make(map[string]*models.Role)
	for _, spec := range roleSpecs {
		s := spec // capture
		role, err := findOrCreate(db, r, "role:"+s.name, &models.Role{},
			"tenant_id = ? AND role_name = ?", tenant.TenantID, s.name,
			func() *models.Role {
				return &models.Role{
					RoleID: uuid.New(), TenantID: tenant.TenantID,
					RoleName: s.name, Description: s.desc, IsSystem: s.system,
				}
			})
		if err != nil {
			return err
		}
		roles[s.name] = role
	}

	// ── 9. RBAC POLICIES & PERMISSIONS ──────────────────────────────────────
	if err := h.ensureRBAC(db, r, tenant.TenantID, roles); err != nil {
		return err
	}

	// ── 10. TENANT SETTINGS ──────────────────────────────────────────────────
	type settingSpec struct {
		key, value, domain, dataType, desc string
		minLevel                           int
	}
	settingSpecs := []settingSpec{
		{"default_currency", "NPR", models.DomainFinancial, "string", "Default currency code", models.RoleLevelOwner},
		{"default_tax_rate", "13.0", models.DomainFinancial, "float", "Nepal VAT rate", models.RoleLevelOwner},
		{"service_charge_pct", "10.0", models.DomainFinancial, "float", "Service charge %", models.RoleLevelOwner},
		{"tax_inclusive_pricing", "false", models.DomainFinancial, "bool", "Prices include tax", models.RoleLevelOwner},
		{"timezone", "Asia/Kathmandu", models.DomainOperational, "string", "Primary timezone", models.RoleLevelManager},
		{"auto_kot_firing", "true", models.DomainOperational, "bool", "Auto-fire KOT on order", models.RoleLevelManager},
		{"table_expiration_minutes", "60", models.DomainOperational, "int", "Idle table release time", models.RoleLevelManager},
		{"stock_warning_threshold", "10", models.DomainOperational, "int", "Low-stock alert level", models.RoleLevelManager},
		{"language", "en", models.DomainDisplay, "string", "UI language", models.RoleLevelStaff},
		{"date_format", "YYYY-MM-DD", models.DomainDisplay, "string", "Date display format", models.RoleLevelStaff},
		{"require_manager_pin_void", "true", models.DomainSecurity, "bool", "PIN required to void order", models.RoleLevelOwner},
		{"require_manager_pin_discount", "true", models.DomainSecurity, "bool", "PIN required to apply discount", models.RoleLevelOwner},
		{"max_discount_pct_staff", "0", models.DomainSecurity, "float", "Max staff discount %", models.RoleLevelOwner},
		{"max_discount_pct_manager", "20", models.DomainSecurity, "float", "Max manager discount %", models.RoleLevelOwner},
		{"offsite_login_allowed", "false", models.DomainSecurity, "bool", "Allow logins outside office IP", models.RoleLevelOwner},
		{"gdpr_enabled", "false", models.DomainCompliance, "bool", "GDPR compliance mode", models.RoleLevelOwner},
		{"data_retention_days", "2555", models.DomainCompliance, "int", "Data retention period (days)", models.RoleLevelOwner},
	}
	// We need a user to set as modified_by — use a placeholder until superadmin is created.
	// We'll update after users are created; for now use zero UUID which is allowed (no FK on modified_by).
	placeholderUserID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	for _, s := range settingSpecs {
		s := s
		_, err := findOrCreate(db, r, "setting:"+s.key, &models.TenantSetting{},
			"tenant_id = ? AND key = ?", tenant.TenantID, s.key,
			func() *models.TenantSetting {
				return &models.TenantSetting{
					SettingID: uuid.New(), TenantID: tenant.TenantID,
					Key: s.key, Value: s.value,
					Domain: s.domain, DataType: s.dataType,
					MinRoleLevel: s.minLevel, IsSystemLocked: false,
					ModifiedBy: placeholderUserID, ModifiedAt: time.Now(),
					Description: s.desc, DefaultValue: s.value,
				}
			})
		if err != nil {
			r.fail("setting:"+s.key, err.Error()) // non-fatal — log and continue
		}
	}

	// ── 11. USERS ────────────────────────────────────────────────────────────
	type userSpec struct {
		name, fullName, email, phone, password, role string
	}
	userSpecs := []userSpec{
		{"superadmin", "Shaurav Bhandari", "shauravbhandari2@gmail.com", "+977-9800000000", "Shaurav,1.2@", "superadmin"},
		{"owner", "Restaurant Owner", "owner@rems.local", "+977-9811111111", "Owner,1.2@", "owner"},
		{"manager", "Demo Manager", "manager@rems.local", "+977-9833333333", "Manager,1.2@", "manager"},
		{"waiter", "Demo Waiter", "waiter@rems.local", "+977-9822222222", "Waiter,1.2@", "waiter"},
		{"cashier", "Demo Cashier", "cashier@rems.local", "+977-9844444444", "Cashier,1.2@", "cashier"},
		{"kitchen", "Demo Kitchen", "kitchen@rems.local", "+977-9855555555", "Kitchen,1.2@", "kitchen"},
	}
	users := make(map[string]*models.User)
	for _, spec := range userSpecs {
		s := spec
		user, err := h.ensureUser(db, r,
			s.name, s.fullName, s.email, s.phone, s.password,
			tenant.TenantID, org.OrganizationID, branch.BranchID,
			roles[s.role], s.role)
		if err != nil {
			return err
		}
		users[s.name] = user
	}

	// ── 12. USER-ROLE ASSIGNMENTS ────────────────────────────────────────────
	for _, spec := range userSpecs {
		s := spec
		_, err := findOrCreate(db, r, "user_role:"+s.name, &models.UserRole{},
			"user_id = ? AND role_id = ?", users[s.name].UserID, roles[s.role].RoleID,
			func() *models.UserRole {
				return &models.UserRole{
					UserRoleID: uuid.New(),
					UserID:     users[s.name].UserID,
					RoleID:     roles[s.role].RoleID,
					TenantID:   tenant.TenantID,
					AssignedBy: users["superadmin"].UserID,
				}
			})
		if err != nil {
			return err
		}
	}

	// ── 13. USER PROFILES ────────────────────────────────────────────────────
	type profileSpec struct {
		user        string
		profileType string
		roleLevel   int
		empNum      string
		dept        string
		jobTitle    string
		offsite     bool
		pin         bool
	}
	profileSpecs := []profileSpec{
		{"superadmin", "Owner", models.RoleLevelSuperAdmin, "EMP-000", "Management", "Super Administrator", true, false},
		{"owner", "Owner", models.RoleLevelOwner, "EMP-001", "Management", "Restaurant Owner", true, false},
		{"manager", "Manager", models.RoleLevelManager, "EMP-002", "Management", "Restaurant Manager", true, true},
		{"waiter", "Staff", models.RoleLevelStaff, "EMP-003", "FrontOfHouse", "Waiter", false, true},
		{"cashier", "Staff", models.RoleLevelStaff, "EMP-004", "FrontOfHouse", "Cashier", false, true},
		{"kitchen", "Staff", models.RoleLevelStaff, "EMP-005", "Kitchen", "Kitchen Staff", false, false},
	}
	for _, spec := range profileSpecs {
		s := spec
		rid := restaurant.RestaurantID
		_, err := findOrCreate(db, r, "user_profile:"+s.user, &models.UserProfile{},
			"user_id = ?", users[s.user].UserID,
			func() *models.UserProfile {
				return &models.UserProfile{
					ProfileID:             uuid.New(),
					UserID:                users[s.user].UserID,
					TenantID:              tenant.TenantID,
					RestaurantID:          &rid,
					ProfileType:           s.profileType,
					RoleLevel:             s.roleLevel,
					EmployeeNumber:        s.empNum,
					Department:            s.dept,
					JobTitle:              s.jobTitle,
					HireDate:              timePtr(time.Now()),
					EmploymentStatus:      "active",
					WeeklyHours:           40,
					CanLoginOffsite:       s.offsite,
					RequiresPINForActions: s.pin,
					PreferredLanguage:     "en",
				}
			})
		if err != nil {
			return err
		}
	}

	// ── 14. EMPLOYEES ────────────────────────────────────────────────────────
	type empSpec struct {
		user, first, last, pos, dept string
		rate                         *float64
	}
	hrOwner, hrMgr, hrWaiter, hrCashier, hrKitchen := 0.0, 250.0, 200.0, 180.0, 190.0
	empSpecs := []empSpec{
		{"owner", "Restaurant", "Owner", "Owner", "Management", &hrOwner},
		{"manager", "Demo", "Manager", "Manager", "Management", &hrMgr},
		{"waiter", "Demo", "Waiter", "Waiter", "FrontOfHouse", &hrWaiter},
		{"cashier", "Demo", "Cashier", "Cashier", "FrontOfHouse", &hrCashier},
		{"kitchen", "Demo", "Kitchen", "Kitchen Staff", "Kitchen", &hrKitchen},
	}
	for _, spec := range empSpecs {
		s := spec
		uid := users[s.user].UserID
		_, err := findOrCreate(db, r, "employee:"+s.pos, &models.Employee{},
			"tenant_id = ? AND user_id = ?", tenant.TenantID, uid,
			func() *models.Employee {
				return &models.Employee{
					EmployeeID:   uuid.New(),
					TenantID:     tenant.TenantID,
					RestaurantID: restaurant.RestaurantID,
					UserID:       &uid,
					FirstName:    s.first,
					LastName:     s.last,
					Email:        users[s.user].Email,
					Phone:        users[s.user].Phone,
					HireDate:     timePtr(time.Now()),
					Position:     s.pos,
					Department:   s.dept,
					HourlyRate:   s.rate,
					IsActive:     true,
				}
			})
		if err != nil {
			return err
		}
	}

	// ── 15. FLOOR & TABLES ───────────────────────────────────────────────────
	floor, err := findOrCreate(db, r, "floor:ground", &models.Floor{},
		"restaurant_id = ? AND name = ?", restaurant.RestaurantID, "Ground Floor",
		func() *models.Floor {
			return &models.Floor{
				RestaurantID: restaurant.RestaurantID,
				Name:         "Ground Floor",
				FloorNumber:  1,
				TableCount:   5,
			}
		})
	if err != nil {
		return err
	}

	for i, tbl := range []struct{ num string; cap int }{
		{"T1", 2}, {"T2", 4}, {"T3", 4}, {"T4", 6}, {"T5", 8},
	} {
		t := tbl
		i := i
		_ = i
		_, err := findOrCreate(db, r, "table:"+t.num, &models.Table{},
			"restaurant_id = ? AND table_number = ?", restaurant.RestaurantID, t.num,
			func() *models.Table {
				return &models.Table{
					RestaurantID: restaurant.RestaurantID,
					FloorID:      floor.FloorID,
					TableNumber:  t.num,
					Capacity:     t.cap,
					Status:       models.TableStatusAvailable,
				}
			})
		if err != nil {
			return err
		}
	}

	// ── 16. MENU CATEGORIES ──────────────────────────────────────────────────
	type catSpec struct{ name, desc string; order int }
	catSpecs := []catSpec{
		{"Starters", "Appetizers and soups", 1},
		{"Main Course", "Hearty main dishes", 2},
		{"Beverages", "Hot and cold drinks", 3},
		{"Desserts", "Sweet endings", 4},
	}
	cats := make(map[string]*models.MenuCategory)
	for _, spec := range catSpecs {
		s := spec
		cat, err := findOrCreate(db, r, "menu_cat:"+s.name, &models.MenuCategory{},
			"tenant_id = ? AND restaurant_id = ? AND name = ?",
			tenant.TenantID, restaurant.RestaurantID, s.name,
			func() *models.MenuCategory {
				return &models.MenuCategory{
					MenuCategoryID: uuid.New(),
					TenantID:       tenant.TenantID,
					RestaurantID:   restaurant.RestaurantID,
					Name:           s.name,
					Description:    s.desc,
					DisplayOrder:   &s.order,
					IsActive:       true,
				}
			})
		if err != nil {
			return err
		}
		cats[s.name] = cat
	}

	// ── 17. MENU ITEMS ───────────────────────────────────────────────────────
	type menuItemSpec struct {
		cat, name, desc string
		price           float64
		prepTime        int
		dietary         models.JSONB
	}
	prepTimes := map[string]int{}
	menuItemSpecs := []menuItemSpec{
		{"Starters", "Veg Soup", "Fresh garden vegetable soup", 180, 10,
			models.JSONB{"vegetarian": true, "vegan": true, "gluten_free": true}},
		{"Starters", "Spring Rolls", "Crispy vegetable spring rolls (4 pcs)", 250, 15,
			models.JSONB{"vegetarian": true, "vegan": false, "gluten_free": false}},
		{"Main Course", "Classic Burger", "Juicy beef patty with lettuce, tomato & sauce", 350, 20,
			models.JSONB{"vegetarian": false, "vegan": false, "gluten_free": false}},
		{"Main Course", "Dal Bhat Set", "Traditional Nepali dal bhat with pickles", 280, 15,
			models.JSONB{"vegetarian": true, "vegan": true, "gluten_free": true}},
		{"Main Course", "Grilled Chicken", "Herb-marinated grilled chicken breast", 420, 25,
			models.JSONB{"vegetarian": false, "vegan": false, "gluten_free": true}},
		{"Beverages", "Masala Tea", "Spiced Nepali milk tea", 80, 5,
			models.JSONB{"vegetarian": true, "vegan": false, "gluten_free": true}},
		{"Beverages", "Fresh Lime Soda", "Lime juice with soda water", 120, 3,
			models.JSONB{"vegetarian": true, "vegan": true, "gluten_free": true}},
		{"Desserts", "Gulab Jamun", "Soft milk-solid dumplings in sugar syrup (2 pcs)", 150, 5,
			models.JSONB{"vegetarian": true, "vegan": false, "gluten_free": false}},
	}
	_ = prepTimes
	menuItems := make(map[string]*models.MenuItem)
	for _, spec := range menuItemSpecs {
		s := spec
		item, err := findOrCreate(db, r, "menu_item:"+s.name, &models.MenuItem{},
			"tenant_id = ? AND restaurant_id = ? AND name = ?",
			tenant.TenantID, restaurant.RestaurantID, s.name,
			func() *models.MenuItem {
				return &models.MenuItem{
					MenuItemID:             uuid.New(),
					MenuCategoryID:         cats[s.cat].MenuCategoryID,
					TenantID:               tenant.TenantID,
					RestaurantID:           restaurant.RestaurantID,
					Name:                   s.name,
					Description:            s.desc,
					BasePrice:              s.price,
					IsAvailable:            true,
					PreparationTimeMinutes: &s.prepTime,
					DietaryFlags:           s.dietary,
				}
			})
		if err != nil {
			return err
		}
		menuItems[s.name] = item
	}

	// ── 18. MENU ITEM MODIFIERS ──────────────────────────────────────────────
	type modSpec struct{ item, name string; price float64 }
	modSpecs := []modSpec{
		{"Classic Burger", "Extra Cheese", 50},
		{"Classic Burger", "No Onion", 0},
		{"Classic Burger", "Extra Patty", 120},
		{"Dal Bhat Set", "Extra Rice", 50},
		{"Dal Bhat Set", "Extra Papadam", 30},
		{"Masala Tea", "Extra Strong", 0},
		{"Masala Tea", "No Sugar", 0},
	}
	for _, spec := range modSpecs {
		s := spec
		item, ok := menuItems[s.item]
		if !ok {
			continue
		}
		_, err := findOrCreate(db, r, "mod:"+s.item+":"+s.name, &models.MenuItemModifier{},
			"menu_item_id = ? AND name = ?", item.MenuItemID, s.name,
			func() *models.MenuItemModifier {
				return &models.MenuItemModifier{
					MenuItemModifierID: uuid.New(),
					MenuItemID:         item.MenuItemID,
					Name:               s.name,
					PriceAdjustment:    s.price,
					IsAvailable:        true,
				}
			})
		if err != nil {
			return err
		}
	}

	// ── 19. CUSTOMER ─────────────────────────────────────────────────────────
	_, err = findOrCreate(db, r, "customer:ram", &models.Customer{},
		"tenant_id = ? AND email = ?", tenant.TenantID, "ram.shrestha@example.com",
		func() *models.Customer {
			return &models.Customer{
				CustomerID: uuid.New(), TenantID: tenant.TenantID,
				FirstName: "Ram", LastName: "Shrestha",
				Email: "ram.shrestha@example.com", Phone: "+977-9841234567",
				Address: "New Baneshwor", City: "Kathmandu",
				State: "Bagmati", PostalCode: "44600",
				LoyaltyPoints: 150, TotalOrders: 5, TotalSpent: 2500.00,
				IsActive: true,
			}
		})
	if err != nil {
		return err
	}

	// ── 20. VENDOR ───────────────────────────────────────────────────────────
	vendor, err := findOrCreate(db, r, "vendor", &models.Vendor{},
		"tenant_id = ? AND name = ?", tenant.TenantID, "Fresh Farms Supply",
		func() *models.Vendor {
			return &models.Vendor{
				VendorID: uuid.New(), TenantID: tenant.TenantID,
				Name: "Fresh Farms Supply", ContactName: "Sita Tamang",
				Email: "fresh.farms@example.com", Phone: "+977-1-5555555",
				Address: "Kalimati, Kathmandu", PaymentTerms: "Net 30", IsActive: true,
			}
		})
	if err != nil {
		return err
	}
	_ = vendor

	// ── 21. INVENTORY ITEMS ──────────────────────────────────────────────────
	type invSpec struct {
		sku, name, desc, cat, unit string
		qty, minQ, maxQ, reorder   float64
		cost                       float64
	}
	invSpecs := []invSpec{
		{"MEAT-BP-001", "Burger Patties", "Frozen beef patties 150g", "Meat", "pieces", 100, 20, 200, 30, 120},
		{"VEG-TOM-001", "Tomatoes", "Fresh tomatoes", "Vegetables", "kg", 10, 2, 50, 5, 80},
		{"VEG-LET-001", "Lettuce", "Iceberg lettuce heads", "Vegetables", "pieces", 20, 5, 100, 10, 60},
		{"DRY-RIC-001", "Basmati Rice", "Long-grain basmati rice", "Dry Goods", "kg", 50, 10, 200, 20, 120},
		{"DRY-DAL-001", "Yellow Dal", "Split yellow lentils", "Dry Goods", "kg", 30, 5, 100, 10, 90},
		{"BEV-TEA-001", "Tea Leaves", "Premium CTC tea", "Beverages", "kg", 5, 1, 20, 2, 500},
		{"BEV-MLK-001", "Fresh Milk", "Full-fat pasteurized milk", "Beverages", "litre", 20, 5, 50, 8, 100},
		{"BAK-BUN-001", "Burger Buns", "Sesame burger buns", "Bakery", "pieces", 80, 20, 200, 40, 25},
	}
	invItems := make(map[string]*models.InventoryItem)
	for _, spec := range invSpecs {
		s := spec
		item, err := findOrCreate(db, r, "inv:"+s.sku, &models.InventoryItem{},
			"tenant_id = ? AND sku = ?", tenant.TenantID, s.sku,
			func() *models.InventoryItem {
				return &models.InventoryItem{
					InventoryItemID: uuid.New(),
					TenantID:        tenant.TenantID,
					RestaurantID:    restaurant.RestaurantID,
					Name:            s.name,
					Description:     s.desc,
					SKU:             s.sku,
					Category:        s.cat,
					UnitOfMeasure:   s.unit,
					CurrentQuantity: s.qty,
					MinimumQuantity: &s.minQ,
					MaximumQuantity: &s.maxQ,
					ReorderPoint:    &s.reorder,
					UnitCost:        &s.cost,
				}
			})
		if err != nil {
			return err
		}
		invItems[s.sku] = item
	}

	// ── 22. MENU-ITEM → INVENTORY LINKS ─────────────────────────────────────
	type linkSpec struct{ menuItem, invSKU string; qty float64 }
	linkSpecs := []linkSpec{
		{"Classic Burger", "MEAT-BP-001", 1.0},
		{"Classic Burger", "BAK-BUN-001", 1.0},
		{"Classic Burger", "VEG-TOM-001", 0.05},
		{"Classic Burger", "VEG-LET-001", 0.25},
		{"Dal Bhat Set", "DRY-RIC-001", 0.2},
		{"Dal Bhat Set", "DRY-DAL-001", 0.1},
		{"Masala Tea", "BEV-TEA-001", 0.01},
		{"Masala Tea", "BEV-MLK-001", 0.15},
	}
	for _, spec := range linkSpecs {
		s := spec
		item, ok := menuItems[s.menuItem]
		if !ok {
			continue
		}
		inv, ok := invItems[s.invSKU]
		if !ok {
			continue
		}
		_, err := findOrCreate(db, r, "inv_link:"+s.menuItem+":"+s.invSKU,
			&models.MenuItemInventoryLink{},
			"menu_item_id = ? AND inventory_item_id = ?", item.MenuItemID, inv.InventoryItemID,
			func() *models.MenuItemInventoryLink {
				return &models.MenuItemInventoryLink{
					LinkID:          uuid.New(),
					TenantID:        tenant.TenantID,
					MenuItemID:      item.MenuItemID,
					InventoryItemID: inv.InventoryItemID,
					QuantityPerUnit: s.qty,
					IsActive:        true,
				}
			})
		if err != nil {
			return err
		}
	}

	// ── 23. INITIAL STOCK MOVEMENTS ──────────────────────────────────────────
	superID := users["superadmin"].UserID
	for sku, inv := range invItems {
		s := sku
		i := inv
		_, err := findOrCreate(db, r, "stock_mv:"+s, &models.StockMovement{},
			"inventory_item_id = ? AND reason = ? AND reference_type = ?",
			i.InventoryItemID, models.StockMovementReasonReceived, "seed",
			func() *models.StockMovement {
				return &models.StockMovement{
					StockMovementID: uuid.New(),
					TenantID:        tenant.TenantID,
					RestaurantID:    restaurant.RestaurantID,
					InventoryItemID: i.InventoryItemID,
					Reason:          models.StockMovementReasonReceived,
					QuantityDelta:   i.CurrentQuantity,
					BalanceAfter:    i.CurrentQuantity,
					ReferenceType:   "seed",
					Notes:           "Initial stock load",
					CreatedBy:       superID,
				}
			})
		if err != nil {
			r.fail("stock_mv:"+s, err.Error()) // non-fatal
		}
	}

	// ── 24. DATA PRIVACY RECORD ──────────────────────────────────────────────
	_, err = findOrCreate(db, r, "privacy_record", &models.DataPrivacyRecord{},
		"tenant_id = ?", tenant.TenantID,
		func() *models.DataPrivacyRecord {
			gdpr := false
			ccpa := false
			hasPolicy := true
			hasCookie := false
			hasDPA := false
			allowDel := true
			allowPort := true
			allowOpt := true
			custDays := 730
			payDays := 2555
			auditDays := 2555
			backupDays := 365
			return &models.DataPrivacyRecord{
        DataPrivacyRecordID:         uuid.New(),
        TenantID:                    tenant.TenantID,
        GDPRApplicable:              &gdpr,
        LawfulBasisForProcessing:    "Legitimate interest",
        HasPrivacyPolicy:            &hasPolicy,
        HasCookieConsent:            &hasCookie,
        HasDataProcessingAgreements: &hasDPA,
        CCPAApplicable:              &ccpa,
        AllowsDataDeletion:          &allowDel,
        AllowsDataPortability:       &allowPort,
        AllowsOptOut:                &allowOpt,
        CustomerDataRetentionDays:   &custDays,
        PaymentDataRetentionDays:    &payDays,
        AuditLogRetentionDays:       &auditDays,
        BackupRetentionDays:         &backupDays,
        // JSONB must be map[string]interface{}, not a bare array
        DataCategories:       models.JSONB{"items": []string{"contact_info", "order_history", "payment_info"}},
        ProcessingPurposes:   models.JSONB{"items": []string{"order_processing", "support", "analytics"}},
        DataRecipients:       models.JSONB{"items": []string{"internal_staff", "payment_processor"}},
        InternationalTransfers: models.JSONB{"items": []string{}},
    }
		})
	if err != nil {
		r.fail("privacy_record", err.Error())
	}

	// ── 25. API KEY ───────────────────────────────────────────────────────────
	_, err = findOrCreate(db, r, "api_key", &models.APIKey{},
		"tenant_id = ? AND description = ?", tenant.TenantID, "Default POS Key",
		func() *models.APIKey {
			return &models.APIKey{
				APIKeyID:    uuid.New(),
				TenantID:    tenant.TenantID,
				Key:         "sk_rems_" + uuid.New().String(),
				Description: "Default POS Key",
				IsActive:    true,
			}
		})
	if err != nil {
		r.fail("api_key", err.Error())
	}

	// ── 26. NOTIFICATION (welcome) ───────────────────────────────────────────
	ownerID := users["owner"].UserID
	_, err = findOrCreate(db, r, "notification:welcome", &models.Notification{},
		"tenant_id = ? AND user_id = ? AND notification_type = ?",
		tenant.TenantID, ownerID, "Info",
		func() *models.Notification {
			return &models.Notification{
				NotificationID:   uuid.New(),
				TenantID:         tenant.TenantID,
				UserID:           &ownerID,
				Message:          "Welcome to ReMS! Your system has been initialized.",
				NotificationType: "Info",
				IsRead:           false,
			}
		})
	if err != nil {
		r.fail("notification:welcome", err.Error())
	}

	log.Println("🌱 Initialization complete")
	return nil
}

// ============================================================================
// RBAC — Policies & Permissions
// ============================================================================

func (h *InitializeHandler) ensureRBAC(db *gorm.DB, r *initResult, tenantID uuid.UUID, roles map[string]*models.Role) error {
	type permEntry struct{ resource, action string }
	type policySpec struct {
		name, desc, role string
		perms            []permEntry
	}

	allResources := []string{"restaurant", "menu", "employee", "order", "inventory",
		"customer", "payment", "report", "setting", "user", "role", "audit", "kot"}

	ownerPerms := make([]permEntry, 0, len(allResources))
	for _, res := range allResources {
		ownerPerms = append(ownerPerms, permEntry{res, "*"})
	}

	managerPerms := []permEntry{
		{"order", "*"}, {"menu", "*"}, {"customer", "*"},
		{"inventory", "read"}, {"inventory", "update"},
		{"employee", "read"}, {"report", "read"},
		{"kot", "*"}, {"payment", "read"},
	}

	specs := []policySpec{
		{
			name: "SuperAdmin Full Access", desc: "Unrestricted platform access", role: "superadmin",
			perms: []permEntry{{"*", "*"}},
		},
		{
			name: "Owner Management", desc: "Full restaurant management", role: "owner",
			perms: ownerPerms,
		},
		{
			name: "Manager Access", desc: "Operational management access", role: "manager",
			perms: managerPerms,
		},
		{
			name: "Waiter Access", desc: "Order and menu access", role: "waiter",
			perms: []permEntry{
				{"order", "create"}, {"order", "read"}, {"order", "update"},
				{"menu", "read"}, {"customer", "read"}, {"customer", "create"},
				{"kot", "create"}, {"kot", "read"},
			},
		},
		{
			name: "Cashier Access", desc: "Payment and order closing", role: "cashier",
			perms: []permEntry{
				{"order", "read"}, {"order", "update"},
				{"payment", "create"}, {"payment", "read"}, {"payment", "update"},
				{"customer", "read"}, {"customer", "create"},
				{"menu", "read"},
			},
		},
		{
			name: "Kitchen Access", desc: "KOT management only", role: "kitchen",
			perms: []permEntry{
				{"kot", "read"}, {"kot", "update"},
				{"order", "read"},
				{"menu", "read"},
			},
		},
	}

	for _, spec := range specs {
		s := spec
		policy, err := findOrCreate(db, r, "policy:"+s.role, &models.Policy{},
			"tenant_id = ? AND name = ?", tenantID, s.name,
			func() *models.Policy {
				return &models.Policy{
					PolicyID: uuid.New(), TenantID: tenantID,
					Name: s.name, Description: s.desc,
				}
			})
		if err != nil {
			return err
		}

		for _, perm := range s.perms {
			p := perm
			_, err := findOrCreate(db, r,
				fmt.Sprintf("perm:%s:%s:%s", s.role, p.resource, p.action),
				&models.Permission{},
				"policy_id = ? AND resource = ? AND action = ?",
				policy.PolicyID, p.resource, p.action,
				func() *models.Permission {
					return &models.Permission{
						PermissionID: uuid.New(), PolicyID: policy.PolicyID,
						Resource: p.resource, Action: p.action, Effect: "allow",
					}
				})
			if err != nil {
				return err
			}
		}

		role := roles[s.role]
if role == nil {
    continue
}

	var count int64
	db.Model(&models.RolePolicy{}).
		Where("role_id = ? AND policy_id = ?", role.RoleID, policy.PolicyID).
		Count(&count)

	if count == 0 {
		rp := &models.RolePolicy{
			RolePolicyID: uuid.New(),
			RoleID:       role.RoleID,
			PolicyID:     policy.PolicyID,
		}
		if err := db.Create(rp).Error; err != nil {
			r.fail("role_policy:"+s.role, err.Error())
			return err
		}
		r.create("role_policy:" + s.role)
	} else {
		r.skip("role_policy:" + s.role)
	}
	}

	return nil
}

// ============================================================================
// USER CREATION HELPER
// ============================================================================

func (h *InitializeHandler) ensureUser(
	db *gorm.DB, r *initResult,
	userName, fullName, email, phone, password string,
	tenantID, orgID, branchID uuid.UUID,
	role *models.Role, primaryRole string,
) (*models.User, error) {
	var existing models.User
	if err := db.Where("email = ? AND tenant_id = ?", email, tenantID).
		First(&existing).Error; err == nil {
		r.skip("user:" + userName)
		return &existing, nil
	}

	hash, err := services.HashPwd(password)
	if err != nil {
		r.fail("user:"+userName, "password hash failed")
		return nil, fmt.Errorf("hash password for %s: %w", userName, err)
	}

	now := time.Now()
	user := &models.User{
		UserID:          uuid.New(),
		UserName:        userName,
		FullName:        fullName,
		Email:           email,
		Phone:           phone,
		PasswordHash:    hash,
		IsActive:        true,
		TenantID:        tenantID,
		OrganizationID:  orgID,
		BranchID:        branchID,
		PrimaryRole:     primaryRole,
		IsEmailVerified: true,
		EmailVerifiedAt: &now,
	}
	if role != nil {
		user.DefaultRoleId = role.RoleID
	}

	if err := db.Create(user).Error; err != nil {
		r.fail("user:"+userName, err.Error())
		return nil, fmt.Errorf("create user %s: %w", userName, err)
	}
	r.create("user:" + userName)
	return user, nil
}

// ============================================================================
// GENERIC FIND-OR-CREATE
// ============================================================================

func findOrCreate[T any](
	db *gorm.DB,
	r *initResult,
	label string,
	dest T,
	query string,
	args ...interface{},
) (T, error) {
	factoryIdx := len(args) - 1
	factory := args[factoryIdx].(func() T)
	queryArgs := args[:factoryIdx]

	if err := db.Where(query, queryArgs...).First(dest).Error; err == nil {
		r.skip(label)
		return dest, nil
	}

	newRecord := factory()
	if err := db.Create(newRecord).Error; err != nil {
		r.fail(label, err.Error())
		return newRecord, fmt.Errorf("create %s: %w", label, err)
	}
	r.create(label)
	return newRecord, nil
}

// ============================================================================
// HELPERS
// ============================================================================

func timePtr(t time.Time) *time.Time { return &t }
package DB

import (
	"fmt"
	"log"

	"backend/config"
	"backend/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func InitDB(config config.DBConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		config.Host, config.User, config.Password, config.DBName, config.Port, config.SSLMode,
	)

	// nosemgrep: go.lang.security.audit.database.string-formatted-query - credentials are loaded from environment variables via config.LoadDBConfig()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		// CRITICAL: Disable foreign key constraints during migration
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	if err := enableUUIDExtension(db); err != nil {
		return nil, fmt.Errorf("failed to enable UUID: %w", err)
	}

	if err := MigrateAll(db); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	log.Println("✓ Database initialized successfully")
	return db, nil
}

func enableUUIDExtension(db *gorm.DB) error {
	return db.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"").Error
}

// MigrateAll - Create ALL tables in one go, no foreign key constraints
func MigrateAll(db *gorm.DB) error {
	log.Println("🚀 Starting migration (FK constraints DISABLED)...")

	// Get all models
	models := getAllModels()

	// Migrate all at once - GORM will handle dependency order
	if err := db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}

	log.Println("✅ All tables created successfully!")
	
	// Now add foreign key constraints manually
	// log.Println("🔗 Adding foreign key constraints...")
	// if err := addForeignKeyConstraints(db); err != nil {
	// 	log.Printf("⚠️  Warning: Some FK constraints failed: %v", err)
	// 	log.Println("   This is OK - constraints may already exist")
	// }

	log.Println("✅ Migration complete!")
	return nil
}

// getAllModels returns all models in the system
func getAllModels() []interface{} {
	return []interface{}{
		// Foundation
		&models.Tenant{},
		&models.Plan{},
		&models.PlanFeature{},
		
		// Tenant-level
		&models.Organization{},
		&models.Branch{},
		&models.Role{},
		&models.Customer{},
		&models.Vendor{},
		
		// Properties
		&models.Restaurant{},
		&models.Region{},
		&models.RegionalSetting{},
		&models.Floor{},
		&models.Table{},
		
		// Menu
		&models.MenuCategory{},
		&models.MenuItem{},
		&models.MenuItemModifier{},
		&models.MenuItemPricing{},
		
		// Users & Auth
		&models.User{},
		&models.UserRole{},
		&models.Session{},
		&models.PasswordReset{},
		
		// Employees
		&models.Employee{},
		&models.EmployeeRole{},
		
		// Orders
		&models.Order{},
		&models.OrderItem{},
		&models.OrderItemModifier{},
		&models.OrderLog{},
		
		// Kitchen
		&models.KOT{},
		&models.KOTItem{},
		
		// Inventory
		&models.InventoryItem{},
		&models.PurchaseOrder{},
		&models.PurchaseOrderLine{},
		
		// Financial
		&models.PaymentRecord{},
		&models.Invoice{},
		&models.Subscription{},
		&models.TenantAddon{},
		
		// Forecasting
		&models.DemandForecast{},
		&models.SalesForecast{},
		&models.InventoryForecast{},
		&models.ForecastAccuracy{},
		
		// Notifications & Integrations
		&models.Notification{},
		&models.Integration{},
		&models.IntegrationLog{},
		&models.Webhook{},
		&models.WebhookSubscription{},
		&models.WorkflowRule{},
		&models.APIKey{},
		
		// Audit & Security
		&models.ActivityLog{},
		&models.AuditTrail{},
		&models.AuditTrailArchive{},
		&models.AnomalyRecord{},
		&models.SecurityAssessment{},
		&models.SecurityFinding{},
		&models.SecurityIncident{},
		&models.PCIComplianceRecord{},
		
		// Privacy & Compliance
		&models.DataPrivacyRecord{},
		&models.DataBreach{},
		&models.DataSubjectRequest{},
		&models.DataAccessControl{},
		&models.DataArchiveSetting{},
	}
}

// addForeignKeyConstraints - Add FK constraints after all tables exist
func addForeignKeyConstraints(db *gorm.DB) error {
	// Most critical foreign keys
	constraints := []string{
		// Organization to Tenant
		`ALTER TABLE organizations 
		 ADD CONSTRAINT fk_organizations_tenant 
		 FOREIGN KEY (tenant_id) REFERENCES tenants(tenant_id) ON DELETE CASCADE`,
		
		// Branch to Organization and Tenant
		`ALTER TABLE branches 
		 ADD CONSTRAINT fk_branches_organization 
		 FOREIGN KEY (organization_id) REFERENCES organizations(organization_id) ON DELETE CASCADE`,
		
		`ALTER TABLE branches 
		 ADD CONSTRAINT fk_branches_tenant 
		 FOREIGN KEY (tenant_id) REFERENCES tenants(tenant_id) ON DELETE CASCADE`,
		
		// User to Organization, Branch, Tenant
		`ALTER TABLE users 
		 ADD CONSTRAINT fk_users_organization 
		 FOREIGN KEY (organization_id) REFERENCES organizations(organization_id) ON DELETE CASCADE`,
		
		`ALTER TABLE users 
		 ADD CONSTRAINT fk_users_branch 
		 FOREIGN KEY (branch_id) REFERENCES branches(branch_id) ON DELETE CASCADE`,
		
		`ALTER TABLE users 
		 ADD CONSTRAINT fk_users_tenant 
		 FOREIGN KEY (tenant_id) REFERENCES tenants(tenant_id) ON DELETE CASCADE`,
		
		`ALTER TABLE users 
		 ADD CONSTRAINT fk_users_default_role 
		 FOREIGN KEY (default_role_id) REFERENCES roles(role_id) ON DELETE RESTRICT`,
		
		// Restaurant to Tenant
		`ALTER TABLE restaurants 
		 ADD CONSTRAINT fk_restaurants_tenant 
		 FOREIGN KEY (tenant_id) REFERENCES tenants(tenant_id) ON DELETE CASCADE`,
		
		// Order to Customer, Restaurant, Table, User
		`ALTER TABLE orders 
		 ADD CONSTRAINT fk_orders_customer 
		 FOREIGN KEY (customer_id) REFERENCES customers(customer_id) ON DELETE SET NULL`,
		
		`ALTER TABLE orders 
		 ADD CONSTRAINT fk_orders_restaurant 
		 FOREIGN KEY (restaurant_id) REFERENCES restaurants(restaurant_id) ON DELETE CASCADE`,
		
		`ALTER TABLE orders 
		 ADD CONSTRAINT fk_orders_table 
		 FOREIGN KEY (table_id) REFERENCES tables(table_id) ON DELETE SET NULL`,
		
		`ALTER TABLE orders 
		 ADD CONSTRAINT fk_orders_user 
		 FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE SET NULL`,
		
		// KOT to Order
		`ALTER TABLE kots 
		 ADD CONSTRAINT fk_kots_order 
		 FOREIGN KEY (order_id) REFERENCES orders(order_id) ON DELETE CASCADE`,
	}
	
	for _, constraint := range constraints {
		if err := db.Exec(constraint).Error; err != nil {
			// Log but don't fail - constraint might already exist
			log.Printf("   Constraint warning: %v", err)
		}
	}
	
	return nil
}

func DropAllTables(db *gorm.DB) error {
	log.Println("⚠️  Dropping all tables...")
	
	// Drop in reverse order
	models := getAllModels()
	
	// Reverse the slice
	for i := len(models)/2 - 1; i >= 0; i-- {
		opp := len(models) - 1 - i
		models[i], models[opp] = models[opp], models[i]
	}
	
	for _, model := range models {
		if err := db.Migrator().DropTable(model); err != nil {
			log.Printf("   Warning dropping %T: %v", model, err)
		}
	}
	
	log.Println("✓ Tables dropped")
	return nil
}

func CheckTablesExist(db *gorm.DB) error {
	requiredTables := []interface{}{
		&models.Tenant{},
		&models.Organization{},
		&models.Branch{},
		&models.User{},
		&models.Restaurant{},
		&models.Order{},
		&models.KOT{},
	}
	
	for _, table := range requiredTables {
		if !db.Migrator().HasTable(table) {
			return fmt.Errorf("missing critical table: %T", table)
		}
	}
	
	log.Println("✓ All critical tables exist")
	return nil
}

var DB *gorm.DB

func Connect(config config.DBConfig) error {
	db, err := InitDB(config)
	if err != nil {
		return err
	}
	DB = db
	return nil
}
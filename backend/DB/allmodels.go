package DB

import (
	"backend/models"
)
func AllModels() []interface{} {
	return []interface{}{
		// Core
		&models.Tenant{},
		&models.Organization{},
		&models.Branch{},
		&models.Role{},
		&models.User{},
		&models.UserRole{},
		
		// Authentication
		&models.Session{},
		&models.PasswordReset{},
		
		// Restaurant & Location
		&models.Restaurant{},
		&models.Region{},
		&models.RegionalSetting{},
		&models.Floor{},
		&models.Table{},
		
		// Employees
		&models.Employee{},
		&models.EmployeeRole{},
		
		// Customers
		&models.Customer{},
		
		// Menu
		&models.MenuCategory{},
		&models.MenuItem{},
		&models.MenuItemModifier{},
		&models.MenuItemPricing{},
		
		// Orders & KOT
		&models.Order{},
		&models.OrderItem{},
		&models.OrderItemModifier{},
		&models.OrderLog{},
		&models.KOT{},
		&models.KOTItem{},
		
		// Inventory
		&models.Vendor{},
		&models.InventoryItem{},
		&models.PurchaseOrder{},
		&models.PurchaseOrderLine{},
		
		// Forecasting
		&models.DemandForecast{},
		&models.SalesForecast{},
		&models.InventoryForecast{},
		&models.ForecastAccuracy{},
		
		// Payments & Invoicing
		&models.PaymentRecord{},
		&models.Invoice{},
		
		// Subscriptions
		&models.Plan{},
		&models.PlanFeature{},
		&models.Subscription{},
		&models.TenantAddon{},
		
		// Notifications & Webhooks
		&models.Notification{},
		&models.Webhook{},
		
		// Integrations & Workflows
		&models.Integration{},
		&models.IntegrationLog{},
		&models.WebhookSubscription{},
		&models.WorkflowRule{},
		
		// API Keys
		&models.APIKey{},
		
		// Audit & Compliance
		&models.ActivityLog{},
		&models.AuditTrail{},
		&models.AnomalyRecord{},
		
		// Data Privacy & Compliance
		&models.DataPrivacyRecord{},
		&models.DataSubjectRequest{},
		&models.DataBreach{},
		&models.DataArchiveSetting{},
		&models.AuditTrailArchive{},
		
		// Access Control
		&models.DataAccessControl{},
		
		// Security
		&models.PCIComplianceRecord{},
		&models.SecurityAssessment{},
		&models.SecurityFinding{},
		&models.SecurityIncident{},
	}
}
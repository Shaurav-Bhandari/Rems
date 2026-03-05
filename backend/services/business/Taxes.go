// services/order_service_enhanced.go
package business

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"backend/models"
)

// ============================================
// TAX CALCULATION ENGINE (Settings-Driven)
// ============================================

// TaxCalculator handles all tax computation using settings service
type TaxCalculator struct {
	settingsService *SettingsService
}

func NewTaxCalculator(settingsService *SettingsService) *TaxCalculator {
	return &TaxCalculator{settingsService: settingsService}
}

// TaxConfiguration holds all tax-related settings for a restaurant
type TaxConfiguration struct {
	// Core Settings
	TaxEnabled           bool    // Master toggle - can disable tax entirely
	TaxInclusivePricing  bool    // If true, prices already include tax
	DefaultTaxRate       float64 // Base tax rate (as decimal, e.g., 0.13 for 13%)
	
	// Category-Specific Rates
	AlcoholTaxRate       float64 // Separate rate for alcohol items
	TakeawayTaxRate      float64 // Different rate for takeaway orders
	
	// Tax Application Rules
	ServiceChargeBeforeTax bool  // Apply service charge before or after tax
	RoundTaxAmount         bool  // Round tax to 2 decimals
	
	// Display Settings
	ShowTaxBreakdown     bool    // Show detailed tax breakdown in receipt
	TaxLabel             string  // Custom tax label (e.g., "VAT", "GST", "Sales Tax")
}

// GetTaxConfiguration loads all tax settings for a restaurant
func (tc *TaxCalculator) GetTaxConfiguration(
	ctx context.Context,
	tenantID uuid.UUID,
	restaurantID uuid.UUID,
) (*TaxConfiguration, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Load restaurant profile (has most settings)
	profile, err := tc.settingsService.GetRestaurantProfile(ctx, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("load restaurant profile: %w", err)
	}

	// Check if tax is globally enabled for this restaurant
	taxEnabled, err := tc.settingsService.GetBool(ctx, tenantID, &restaurantID, "tax_enabled")
	if err != nil {
		taxEnabled = true // Default to enabled if setting doesn't exist
	}

	// Get custom tax label
	taxLabel, err := tc.settingsService.GetSetting(ctx, tenantID, &restaurantID, "tax_label")
	if err != nil {
		taxLabel = "Tax" // Default label
	}

	// Get takeaway tax rate (may differ from dine-in)
	takeawayTaxRate, err := tc.settingsService.GetFloat(ctx, tenantID, &restaurantID, "takeaway_tax_rate")
	if err != nil {
		takeawayTaxRate = profile.DefaultTaxRate // Fall back to default
	}

	// Service charge timing
	serviceChargeBeforeTax, err := tc.settingsService.GetBool(ctx, tenantID, &restaurantID, "service_charge_before_tax")
	if err != nil {
		serviceChargeBeforeTax = true // Default: service charge before tax
	}

	return &TaxConfiguration{
		TaxEnabled:             taxEnabled,
		TaxInclusivePricing:    profile.TaxInclusivePricing,
		DefaultTaxRate:         profile.DefaultTaxRate / 100.0,     // Convert % to decimal
		AlcoholTaxRate:         profile.AlcoholTaxRate / 100.0,     // Convert % to decimal
		TakeawayTaxRate:        takeawayTaxRate / 100.0,
		ServiceChargeBeforeTax: serviceChargeBeforeTax,
		RoundTaxAmount:         true,
		ShowTaxBreakdown:       true,
		TaxLabel:               taxLabel,
	}, nil
}

// TaxBreakdown provides detailed tax calculation
type TaxBreakdown struct {
	// Amounts
	SubtotalBeforeTax    float64            `json:"subtotal_before_tax"`
	ServiceChargeAmount  float64            `json:"service_charge_amount"`
	TaxableAmount        float64            `json:"taxable_amount"`
	TotalTaxAmount       float64            `json:"total_tax_amount"`
	GrandTotal           float64            `json:"grand_total"`
	
	// Tax Details
	StandardTaxAmount    float64            `json:"standard_tax_amount"`
	AlcoholTaxAmount     float64            `json:"alcohol_tax_amount"`
	
	// Metadata
	TaxIncludedInPrice   bool               `json:"tax_included_in_price"`
	TaxRate              float64            `json:"tax_rate"` // Effective rate used
	ItemTaxBreakdown     []ItemTaxDetail    `json:"item_tax_breakdown,omitempty"`
}

// ItemTaxDetail shows per-item tax calculation
type ItemTaxDetail struct {
	ItemName       string  `json:"item_name"`
	Quantity       int     `json:"quantity"`
	UnitPrice      float64 `json:"unit_price"`
	LineTotal      float64 `json:"line_total"`
	TaxRate        float64 `json:"tax_rate"`
	TaxAmount      float64 `json:"tax_amount"`
	TaxCategory    string  `json:"tax_category"` // "standard", "alcohol", "exempt"
}

// CalculateTax computes all tax amounts for an order
func (tc *TaxCalculator) CalculateTax(
	ctx context.Context,
	tenantID uuid.UUID,
	restaurantID uuid.UUID,
	orderType string,
	items []OrderItemForTax,
	serviceChargePct float64,
) (*TaxBreakdown, error) {
	config, err := tc.GetTaxConfiguration(ctx, tenantID, restaurantID)
	if err != nil {
		return nil, err
	}

	breakdown := &TaxBreakdown{
		TaxIncludedInPrice: config.TaxInclusivePricing,
		ItemTaxBreakdown:   make([]ItemTaxDetail, 0, len(items)),
	}

	// Step 1: Calculate subtotal (sum of all line items)
	var subtotal float64
	for _, item := range items {
		lineTotal := item.UnitPrice * float64(item.Quantity)
		
		// Add modifier costs
		for _, mod := range item.Modifiers {
			lineTotal += mod.AdditionalPrice * float64(item.Quantity)
		}
		
		subtotal += lineTotal
		
		// Track per-item for detailed breakdown
		breakdown.ItemTaxBreakdown = append(breakdown.ItemTaxBreakdown, ItemTaxDetail{
			ItemName:    item.ItemName,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			LineTotal:   lineTotal,
			TaxCategory: item.TaxCategory,
		})
	}

	breakdown.SubtotalBeforeTax = subtotal

	// Step 2: Handle tax-inclusive pricing
	if config.TaxInclusivePricing {
		// Prices already include tax - extract the tax component
		return tc.extractIncludedTax(breakdown, items, config)
	}

	// Step 3: Tax NOT included - calculate tax on top
	if !config.TaxEnabled {
		// Tax disabled - no tax to calculate
		breakdown.TaxableAmount = subtotal
		breakdown.GrandTotal = subtotal
		return breakdown, nil
	}

	// Step 4: Calculate service charge (if applicable)
	if serviceChargePct > 0 {
		breakdown.ServiceChargeAmount = subtotal * (serviceChargePct / 100.0)
		
		if config.ServiceChargeBeforeTax {
			// Service charge is taxable
			breakdown.TaxableAmount = subtotal + breakdown.ServiceChargeAmount
		} else {
			// Service charge added after tax
			breakdown.TaxableAmount = subtotal
		}
	} else {
		breakdown.TaxableAmount = subtotal
	}

	// Step 5: Calculate tax per item based on category
	for i, item := range items {
		var itemTaxRate float64
		
		switch item.TaxCategory {
		case "alcohol":
			itemTaxRate = config.AlcoholTaxRate
		case "takeaway":
			if orderType == "takeaway" || orderType == "delivery" {
				itemTaxRate = config.TakeawayTaxRate
			} else {
				itemTaxRate = config.DefaultTaxRate
			}
		case "exempt":
			itemTaxRate = 0
		default:
			itemTaxRate = config.DefaultTaxRate
		}

		lineTotal := breakdown.ItemTaxBreakdown[i].LineTotal
		itemTax := lineTotal * itemTaxRate

		breakdown.ItemTaxBreakdown[i].TaxRate = itemTaxRate
		breakdown.ItemTaxBreakdown[i].TaxAmount = itemTax

		// Accumulate by category
		if item.TaxCategory == "alcohol" {
			breakdown.AlcoholTaxAmount += itemTax
		} else {
			breakdown.StandardTaxAmount += itemTax
		}

		breakdown.TotalTaxAmount += itemTax
	}

	// Step 6: Tax on service charge (if applicable)
	if config.ServiceChargeBeforeTax && serviceChargePct > 0 {
		serviceChargeTax := breakdown.ServiceChargeAmount * config.DefaultTaxRate
		breakdown.StandardTaxAmount += serviceChargeTax
		breakdown.TotalTaxAmount += serviceChargeTax
	}

	// Step 7: Round tax if configured
	if config.RoundTaxAmount {
		breakdown.TotalTaxAmount = roundTo2Decimals(breakdown.TotalTaxAmount)
		breakdown.StandardTaxAmount = roundTo2Decimals(breakdown.StandardTaxAmount)
		breakdown.AlcoholTaxAmount = roundTo2Decimals(breakdown.AlcoholTaxAmount)
	}

	// Step 8: Calculate grand total
	breakdown.GrandTotal = breakdown.SubtotalBeforeTax + breakdown.TotalTaxAmount
	
	if !config.ServiceChargeBeforeTax && serviceChargePct > 0 {
		// Add service charge after tax
		breakdown.GrandTotal += breakdown.ServiceChargeAmount
	}

	breakdown.GrandTotal = roundTo2Decimals(breakdown.GrandTotal)

	return breakdown, nil
}

// extractIncludedTax extracts tax component from tax-inclusive prices
func (tc *TaxCalculator) extractIncludedTax(
	breakdown *TaxBreakdown,
	items []OrderItemForTax,
	config *TaxConfiguration,
) (*TaxBreakdown, error) {
	// For tax-inclusive pricing: price = base + (base * tax_rate)
	// Therefore: base = price / (1 + tax_rate)
	// And: tax = price - base

	for i, item := range items {
		var taxRate float64
		switch item.TaxCategory {
		case "alcohol":
			taxRate = config.AlcoholTaxRate
		case "exempt":
			taxRate = 0
		default:
			taxRate = config.DefaultTaxRate
		}

		lineTotal := breakdown.ItemTaxBreakdown[i].LineTotal
		
		// Extract tax component
		baseAmount := lineTotal / (1 + taxRate)
		taxAmount := lineTotal - baseAmount

		breakdown.ItemTaxBreakdown[i].TaxRate = taxRate
		breakdown.ItemTaxBreakdown[i].TaxAmount = taxAmount

		if item.TaxCategory == "alcohol" {
			breakdown.AlcoholTaxAmount += taxAmount
		} else {
			breakdown.StandardTaxAmount += taxAmount
		}

		breakdown.TotalTaxAmount += taxAmount
	}

	// Round
	breakdown.TotalTaxAmount = roundTo2Decimals(breakdown.TotalTaxAmount)
	breakdown.StandardTaxAmount = roundTo2Decimals(breakdown.StandardTaxAmount)
	breakdown.AlcoholTaxAmount = roundTo2Decimals(breakdown.AlcoholTaxAmount)

	breakdown.TaxableAmount = breakdown.SubtotalBeforeTax - breakdown.TotalTaxAmount
	breakdown.GrandTotal = breakdown.SubtotalBeforeTax // Already includes tax

	return breakdown, nil
}

// OrderItemForTax is the minimal item data needed for tax calculation
type OrderItemForTax struct {
	ItemName    string
	UnitPrice   float64
	Quantity    int
	TaxCategory string // "standard", "alcohol", "exempt", "takeaway"
	Modifiers   []ModifierForTax
}

type ModifierForTax struct {
	Name            string
	AdditionalPrice float64
}

// ============================================
// HELPER FUNCTIONS
// ============================================

func roundTo2Decimals(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}

// DetermineTaxCategory examines menu item attributes to determine tax category.
// models.JSONB is map[string]interface{} — already parsed by GORM's Scan hook,
// so we read it directly without json.Unmarshal.
func DetermineTaxCategory(menuItem *models.MenuItem) string {
	flags := map[string]interface{}(menuItem.DietaryFlags)

	if isFlagTrue(flags, "alcohol") {
		return "alcohol"
	}
	if isFlagTrue(flags, "tax_exempt") {
		return "exempt"
	}
	if isFlagTrue(flags, "takeaway_eligible") {
		return "takeaway"
	}
	return "standard"
}

// isFlagTrue returns true when a JSONB flag key is present and its value is
// the boolean true or the string "true".
func isFlagTrue(flags map[string]interface{}, key string) bool {
	v, ok := flags[key]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true"
	default:
		return fmt.Sprintf("%v", v) == "true"
	}
}
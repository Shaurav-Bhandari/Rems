package handlers

import (
	"backend/models"
	"backend/services/printing"
	"backend/utils"
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ============================================================================
// RECEIPT HANDLER
// ============================================================================

// ReceiptHandler handles receipt printing endpoints.
type ReceiptHandler struct {
	db      *gorm.DB
	printer *printing.PrinterService
}

// NewReceiptHandler creates a ReceiptHandler.
func NewReceiptHandler(db *gorm.DB, printer *printing.PrinterService) *ReceiptHandler {
	return &ReceiptHandler{db: db, printer: printer}
}

// PrintReceipt prints a receipt for the given order.
// POST /api/v1/receipts/print
func (h *ReceiptHandler) PrintReceipt(c fiber.Ctx) error {
	type PrintRequest struct {
		OrderID uuid.UUID `json:"order_id"`
	}

	var req PrintRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	// Load order with items
	var order models.Order
	if err := h.db.Preload("Items").Where("order_id = ?", req.OrderID).First(&order).Error; err != nil {
		return utils.SendResponse(c, fiber.StatusNotFound, "Order not found", nil)
	}

	// Load restaurant
	var restaurant models.Restaurant
	h.db.Where("restaurant_id = ?", order.RestaurantID).First(&restaurant)

	// Build receipt
	receipt := &printing.Receipt{
		RestaurantName:    restaurant.Name,
		RestaurantAddress: restaurant.Address,
		RestaurantPhone:   restaurant.Phone,
		OrderID:           order.OrderID,
		OrderNumber:       order.OrderID.String()[:8],
		OrderType:         string(order.OrderType),
		CustomerName:      order.CustomerName,
		CustomerPhone:     order.PhoneNumber,
		GrandTotal:        order.TotalAmount,
		FooterMessage:     "Thank you for your visit!",
	}

	if order.TableID != nil {
		receipt.TableNumber = fmt.Sprintf("%d", *order.TableID)
	}

	// Build line items
	for _, item := range order.Items {
		lineTotal := item.LineTotal()
		receipt.Items = append(receipt.Items, printing.ReceiptItem{
			Name:      item.ItemName,
			Quantity:  item.Quantity,
			UnitPrice: item.UnitPrice,
			Total:     lineTotal,
			Notes:     item.Notes,
		})
		receipt.Subtotal += lineTotal
	}

	// Load tax breakdown if available
	var taxBreakdown models.OrderTaxBreakdown
	if err := h.db.Where("order_id = ?", order.OrderID).First(&taxBreakdown).Error; err == nil {
		receipt.TaxAmount = taxBreakdown.TotalTaxAmount
		receipt.TaxRate = taxBreakdown.TaxRate
		receipt.ServiceCharge = taxBreakdown.ServiceChargeAmount
	}

	// Print!
	if err := h.printer.PrintReceipt(receipt); err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError,
			fmt.Sprintf("Print failed: %v", err), nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Receipt printed successfully", map[string]interface{}{
		"order_id": order.OrderID,
		"status":   "printed",
	})
}

// TestPrinter tests printer connectivity.
// GET /api/v1/receipts/test
func (h *ReceiptHandler) TestPrinter(c fiber.Ctx) error {
	err := h.printer.TestConnection()
	if err != nil {
		return utils.SendResponse(c, fiber.StatusServiceUnavailable,
			fmt.Sprintf("Printer test failed: %v", err), map[string]interface{}{
				"printer_type":    h.printer.GetConfig().Type,
				"printer_address": h.printer.GetConfig().Address,
				"status":          "unreachable",
			})
	}

	return utils.SendResponse(c, fiber.StatusOK, "Printer is reachable", map[string]interface{}{
		"printer_type":    h.printer.GetConfig().Type,
		"printer_address": h.printer.GetConfig().Address,
		"paper_width":     h.printer.GetConfig().PaperWidth,
		"status":          "connected",
	})
}

// PrinterConfig returns the current printer configuration.
// GET /api/v1/receipts/config
func (h *ReceiptHandler) PrinterConfig(c fiber.Ctx) error {
	cfg := h.printer.GetConfig()
	return utils.SendResponse(c, fiber.StatusOK, "Printer configuration", map[string]interface{}{
		"type":        cfg.Type,
		"address":     cfg.Address,
		"paper_width": cfg.PaperWidth,
	})
}

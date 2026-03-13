package printing

import (
	"bytes"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// RECEIPT DATA STRUCTURES
// ============================================================================

// Receipt represents a printable receipt.
type Receipt struct {
	// Header
	RestaurantName    string    `json:"restaurant_name"`
	RestaurantAddress string    `json:"restaurant_address"`
	RestaurantPhone   string    `json:"restaurant_phone"`
	RestaurantPAN     string    `json:"restaurant_pan"`       // PAN/VAT number

	// Order info
	OrderID     uuid.UUID `json:"order_id"`
	OrderNumber string    `json:"order_number"`
	OrderType   string    `json:"order_type"`   // dine-in, takeaway, delivery
	TableNumber string    `json:"table_number"` // for dine-in
	ServerName  string    `json:"server_name"`

	// Customer
	CustomerName  string `json:"customer_name"`
	CustomerPhone string `json:"customer_phone"`

	// Items
	Items []ReceiptItem `json:"items"`

	// Totals
	Subtotal      float64 `json:"subtotal"`
	TaxAmount     float64 `json:"tax_amount"`
	TaxRate       float64 `json:"tax_rate"`        // e.g., 13.0 for 13%
	ServiceCharge float64 `json:"service_charge"`
	DiscountAmount float64 `json:"discount_amount"`
	GrandTotal    float64 `json:"grand_total"`

	// Payment
	PaymentMethod string  `json:"payment_method"` // cash, card, online
	AmountPaid    float64 `json:"amount_paid"`
	ChangeAmount  float64 `json:"change_amount"`

	// Footer
	FooterMessage string    `json:"footer_message"`
	PrintedAt     time.Time `json:"printed_at"`
}

// ReceiptItem represents one line item on the receipt.
type ReceiptItem struct {
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
	Total     float64 `json:"total"`
	Notes     string  `json:"notes"`
}

// ============================================================================
// RECEIPT FORMATTER — generates ESC/POS byte output
// ============================================================================

// FormatReceipt converts a Receipt into ESC/POS bytes ready for printing.
func FormatReceipt(receipt *Receipt, paperWidth int) []byte {
	lineWidth := CharsPerLine(paperWidth)
	var buf bytes.Buffer

	// Initialize printer
	buf.Write(CmdInit)
	buf.Write(CmdDefaultLineSpacing)

	// ── Header (centered, bold, double-width) ────────────────────────────
	buf.Write(CmdAlignCenter)
	buf.Write(CmdDoubleHW)
	buf.WriteString(receipt.RestaurantName)
	buf.Write(CmdLF)
	buf.Write(CmdNormalSize)

	if receipt.RestaurantAddress != "" {
		buf.WriteString(receipt.RestaurantAddress)
		buf.Write(CmdLF)
	}
	if receipt.RestaurantPhone != "" {
		buf.WriteString("Tel: " + receipt.RestaurantPhone)
		buf.Write(CmdLF)
	}
	if receipt.RestaurantPAN != "" {
		buf.WriteString("PAN: " + receipt.RestaurantPAN)
		buf.Write(CmdLF)
	}

	buf.Write(CmdAlignLeft)
	buf.WriteString(DoubleDividerLine(paperWidth))
	buf.Write(CmdLF)

	// ── Order info ───────────────────────────────────────────────────────
	printTime := receipt.PrintedAt
	if printTime.IsZero() {
		printTime = time.Now()
	}

	buf.WriteString(FormatTwoColumn("Order: "+receipt.OrderNumber, printTime.Format("2006-01-02"), lineWidth))
	buf.Write(CmdLF)
	buf.WriteString(FormatTwoColumn("Type: "+receipt.OrderType, printTime.Format("15:04:05"), lineWidth))
	buf.Write(CmdLF)

	if receipt.TableNumber != "" {
		buf.WriteString("Table: " + receipt.TableNumber)
		buf.Write(CmdLF)
	}
	if receipt.ServerName != "" {
		buf.WriteString("Server: " + receipt.ServerName)
		buf.Write(CmdLF)
	}
	if receipt.CustomerName != "" {
		buf.WriteString("Customer: " + receipt.CustomerName)
		buf.Write(CmdLF)
	}

	buf.WriteString(DividerLine(paperWidth))
	buf.Write(CmdLF)

	// ── Column header ────────────────────────────────────────────────────
	buf.Write(CmdBoldOn)
	// Item  Qty  Price  Total
	itemColWidth := lineWidth - 5 - 9 - 10 // qty(5) + price(9) + total(10)
	if itemColWidth < 10 {
		itemColWidth = 10
	}
	header := PadRight("Item", itemColWidth) +
		PadLeft("Qty", 5) +
		PadLeft("Price", 9) +
		PadLeft("Total", 10)
	buf.WriteString(header)
	buf.Write(CmdLF)
	buf.Write(CmdBoldOff)
	buf.WriteString(DividerLine(paperWidth))
	buf.Write(CmdLF)

	// ── Items ────────────────────────────────────────────────────────────
	for _, item := range receipt.Items {
		name := item.Name
		if len(name) > itemColWidth {
			name = name[:itemColWidth-1] + "."
		}

		line := PadRight(name, itemColWidth) +
			PadLeft(fmt.Sprintf("%d", item.Quantity), 5) +
			PadLeft(fmt.Sprintf("%.2f", item.UnitPrice), 9) +
			PadLeft(fmt.Sprintf("%.2f", item.Total), 10)
		buf.WriteString(line)
		buf.Write(CmdLF)

		if item.Notes != "" {
			buf.WriteString("  * " + item.Notes)
			buf.Write(CmdLF)
		}
	}

	buf.WriteString(DividerLine(paperWidth))
	buf.Write(CmdLF)

	// ── Totals ───────────────────────────────────────────────────────────
	buf.WriteString(FormatTwoColumn("Subtotal:", fmt.Sprintf("%.2f", receipt.Subtotal), lineWidth))
	buf.Write(CmdLF)

	if receipt.DiscountAmount > 0 {
		buf.WriteString(FormatTwoColumn("Discount:", fmt.Sprintf("-%.2f", receipt.DiscountAmount), lineWidth))
		buf.Write(CmdLF)
	}

	if receipt.TaxAmount > 0 {
		taxLabel := fmt.Sprintf("Tax (%.1f%%):", receipt.TaxRate)
		buf.WriteString(FormatTwoColumn(taxLabel, fmt.Sprintf("%.2f", receipt.TaxAmount), lineWidth))
		buf.Write(CmdLF)
	}

	if receipt.ServiceCharge > 0 {
		buf.WriteString(FormatTwoColumn("Service Charge:", fmt.Sprintf("%.2f", receipt.ServiceCharge), lineWidth))
		buf.Write(CmdLF)
	}

	buf.WriteString(DoubleDividerLine(paperWidth))
	buf.Write(CmdLF)

	buf.Write(CmdBoldOn)
	buf.Write(CmdDoubleH)
	buf.WriteString(FormatTwoColumn("GRAND TOTAL:", fmt.Sprintf("%.2f", receipt.GrandTotal), lineWidth))
	buf.Write(CmdLF)
	buf.Write(CmdNormalSize)
	buf.Write(CmdBoldOff)

	buf.WriteString(DoubleDividerLine(paperWidth))
	buf.Write(CmdLF)

	// ── Payment ──────────────────────────────────────────────────────────
	if receipt.PaymentMethod != "" {
		buf.WriteString(FormatTwoColumn("Payment:", receipt.PaymentMethod, lineWidth))
		buf.Write(CmdLF)
	}
	if receipt.AmountPaid > 0 {
		buf.WriteString(FormatTwoColumn("Paid:", fmt.Sprintf("%.2f", receipt.AmountPaid), lineWidth))
		buf.Write(CmdLF)
	}
	if receipt.ChangeAmount > 0 {
		buf.WriteString(FormatTwoColumn("Change:", fmt.Sprintf("%.2f", receipt.ChangeAmount), lineWidth))
		buf.Write(CmdLF)
	}

	buf.Write(CmdLF)

	// ── Footer ───────────────────────────────────────────────────────────
	buf.Write(CmdAlignCenter)
	if receipt.FooterMessage != "" {
		buf.WriteString(receipt.FooterMessage)
	} else {
		buf.WriteString("Thank you for dining with us!")
	}
	buf.Write(CmdLF)
	buf.WriteString("Powered by ReMS")
	buf.Write(CmdLF)

	// Feed and cut
	buf.Write(CmdFeed5)
	buf.Write(CmdPartialCut)

	return buf.Bytes()
}

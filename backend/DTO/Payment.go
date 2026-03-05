// DTO/Payment.go
//
// Payment DTOs — extended for Fonepay QR dynamic payment flow.
//
// Status values (must match PaymentState constants in utils/fsm.go):
//   pending_qr | awaiting_confirm | confirmed | failed | refunded
package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ===================================
// PAYMENT RECORD DTOs
// ===================================

// InitiateQRPaymentRequest starts a Fonepay QR payment for an order.
// This is the primary entry point for QR-based checkout at the POS.
type InitiateQRPaymentRequest struct {
	RestaurantID uuid.UUID  `json:"restaurant_id" binding:"required"`
	OrderID      *uuid.UUID `json:"order_id,omitempty"`
	// Amount in NPR (whole rupees — the lib takes int64, we convert in service).
	Amount      float64 `json:"amount" binding:"required,min=1"`
	Description string  `json:"description" binding:"omitempty,max=255"`
	// Remarks2 is an optional second remark line shown on the Fonepay screen.
	Remarks2 string `json:"remarks2" binding:"omitempty,max=255"`
}

func (r *InitiateQRPaymentRequest) Validate() error {
	if r.Amount <= 0 {
		return ErrInvalidPaymentAmount
	}
	return nil
}

// QRPaymentResponse is returned by InitiateQRPayment.
// The caller displays QRImageBase64 as an <img src="data:image/png;base64,...">
// and polls VerifyQRPayment until status is confirmed or failed.
type QRPaymentResponse struct {
	PaymentRecordID      uuid.UUID  `json:"payment_record_id"`
	QRImageBase64        string     `json:"qr_image_base64"`         // data to render in UI
	FonepayTransactionID string     `json:"fonepay_transaction_id"`  // PRN for correlation
	ExpiresAt            time.Time  `json:"expires_at"`              // client should refresh after this
	Status               string     `json:"status"`                  // "pending_qr"
	Amount               float64    `json:"amount"`
	VerifyToken          string     `json:"verify_token"`            // sent back on callback
}

// FonepayCallbackRequest is the body/query the Fonepay gateway sends to
// our callback endpoint after the customer scans and pays.
// The service reads EncodedParams and passes it to provider.VerifyPayment.
type FonepayCallbackRequest struct {
	// VerifyToken is the secret we embedded when generating the QR.
	// Validated server-side before calling Fonepay to prevent replays.
	VerifyToken string `form:"verify_token" json:"verify_token" binding:"required"`

	// EncodedParams is the raw callback string from Fonepay
	// (format: "PRN=...&BID=...&AMT=...&UID=...&...").
	// Passed verbatim to provider.VerifyPayment as core.VerifyRequest.EncodedParams.
	EncodedParams string `form:"encoded_params" json:"encoded_params" binding:"required"`
}

// RegenerateQRRequest asks for a fresh QR when the previous one expired.
type RegenerateQRRequest struct {
	RestaurantID uuid.UUID `json:"restaurant_id" binding:"required"`
}

// CreatePaymentRecordRequest covers non-QR payment methods (cash, card).
type CreatePaymentRecordRequest struct {
	RestaurantID  uuid.UUID  `json:"restaurant_id" binding:"required"`
	OrderID       *uuid.UUID `json:"order_id,omitempty"`
	Amount        float64    `json:"amount" binding:"required,min=0"`
	// PaymentMethod: "cash" | "card" | "fonepay" | "wallet" | "online"
	PaymentMethod string    `json:"payment_method" binding:"required,oneof=cash card fonepay wallet online"`
	TransactionID string    `json:"transaction_id" binding:"omitempty,max=255"`
	PaymentDate   time.Time `json:"payment_date" binding:"required"`
}

func (r *CreatePaymentRecordRequest) Validate() error {
	if r.Amount <= 0 {
		return ErrInvalidPaymentAmount
	}
	if r.PaymentDate.After(time.Now().Add(5 * time.Minute)) {
		return ErrFuturePaymentDate
	}
	return nil
}

// UpdatePaymentRecordRequest is used by admins for manual status corrections.
// Normal status transitions happen through the FSM methods, not here.
type UpdatePaymentRecordRequest struct {
	// Status must be one of the FSM states.
	Status        *string `json:"status" binding:"omitempty,oneof=pending_qr awaiting_confirm confirmed failed refunded"`
	TransactionID *string `json:"transaction_id" binding:"omitempty,max=255"`
	FailureReason *string `json:"failure_reason" binding:"omitempty,max=1000"`
}

// PaymentRecordResponse is the standard read response for a PaymentRecord.
type PaymentRecordResponse struct {
	PaymentRecordID      uuid.UUID        `json:"payment_record_id"`
	TenantID             uuid.UUID        `json:"tenant_id"`
	RestaurantID         uuid.UUID        `json:"restaurant_id"`
	OrderID              *uuid.UUID       `json:"order_id,omitempty"`
	Amount               float64          `json:"amount"`
	PaymentMethod        string           `json:"payment_method"`
	TransactionID        string           `json:"transaction_id,omitempty"`
	FonepayTransactionID string           `json:"fonepay_transaction_id,omitempty"`
	Status               string           `json:"status"`
	PaymentDate          time.Time        `json:"payment_date"`
	// QRExpiresAt is only populated for fonepay payments.
	QRExpiresAt          *time.Time       `json:"qr_expires_at,omitempty"`
	// QRExpired is a convenience flag the client can use to decide whether to
	// show a "Refresh QR" button without having to parse timestamps.
	QRExpired            bool             `json:"qr_expired,omitempty"`
	FailureReason        string           `json:"failure_reason,omitempty"`
	CreatedAt            time.Time        `json:"created_at"`
	Order                *OrderSummaryDTO `json:"order,omitempty"`
}

// PaymentRecordListResponse is the paginated list wrapper.
type PaymentRecordListResponse struct {
	Payments   []PaymentRecordResponse `json:"payments"`
	Total      int64                   `json:"total"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"page_size"`
	TotalPages int                     `json:"total_pages"`
}

// PaymentFilterRequest controls the list endpoint.
type PaymentFilterRequest struct {
	RestaurantID  *uuid.UUID `form:"restaurant_id"`
	OrderID       *uuid.UUID `form:"order_id"`
	PaymentMethod *string    `form:"payment_method" binding:"omitempty,oneof=cash card fonepay wallet online"`
	Status        *string    `form:"status" binding:"omitempty,oneof=pending_qr awaiting_confirm confirmed failed refunded"`
	DateFrom      *time.Time `form:"date_from" time_format:"2006-01-02"`
	DateTo        *time.Time `form:"date_to"   time_format:"2006-01-02"`
	MinAmount     *float64   `form:"min_amount"`
	MaxAmount     *float64   `form:"max_amount"`
	Page          int        `form:"page"      binding:"min=1"`
	PageSize      int        `form:"page_size" binding:"min=1,max=100"`
	SortBy        string     `form:"sort_by"   binding:"omitempty,oneof=payment_date amount status"`
	SortOrder     string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

func (r *PaymentFilterRequest) Validate() error {
	if r.MinAmount != nil && r.MaxAmount != nil && *r.MinAmount > *r.MaxAmount {
		return ErrMinAmountGreaterThanMax
	}
	if r.DateFrom != nil && r.DateTo != nil && r.DateFrom.After(*r.DateTo) {
		return ErrDateFromAfterDateTo
	}
	return nil
}

// PaymentStatsResponse is the dashboard summary.
type PaymentStatsResponse struct {
	TotalRevenue      float64            `json:"total_revenue"`
	TotalTransactions int64              `json:"total_transactions"`
	AvgTransactionVal float64            `json:"avg_transaction_value"`
	RevenueByMethod   map[string]float64 `json:"revenue_by_method"`
	TodayRevenue      float64            `json:"today_revenue"`
	WeekRevenue       float64            `json:"week_revenue"`
	MonthRevenue      float64            `json:"month_revenue"`
}

// OrderSummaryDTO is used inside PaymentRecordResponse.
// type OrderSummaryDTO struct {
// 	OrderID      uuid.UUID `json:"order_id"`
// 	OrderStatus  string    `json:"order_status"`
// 	TotalAmount  float64   `json:"total_amount"`
// 	CustomerName string    `json:"customer_name"`
// 	CreatedAt    time.Time `json:"created_at"`
// }

// ===================================
// INVOICE DTOs (unchanged)
// ===================================

type CreateInvoiceRequest struct {
	RestaurantID   uuid.UUID  `json:"restaurant_id" binding:"required"`
	OrderID        *uuid.UUID `json:"order_id,omitempty"`
	InvoiceDate    time.Time  `json:"invoice_date" binding:"required"`
	DueDate        *time.Time `json:"due_date,omitempty"`
	Subtotal       float64    `json:"subtotal" binding:"required,min=0"`
	TaxAmount      float64    `json:"tax_amount" binding:"required,min=0"`
	DiscountAmount float64    `json:"discount_amount" binding:"omitempty,min=0"`
}

func (r *CreateInvoiceRequest) Validate() error {
	if r.Subtotal < 0 {
		return ErrNegativeSubtotal
	}
	if r.TaxAmount < 0 {
		return ErrNegativeTaxAmount
	}
	if r.DiscountAmount < 0 {
		return ErrNegativeDiscountAmount
	}
	if r.DiscountAmount > r.Subtotal {
		return ErrDiscountExceedsSubtotal
	}
	if r.DueDate != nil && r.DueDate.Before(r.InvoiceDate) {
		return ErrDueDateBeforeInvoiceDate
	}
	return nil
}

type UpdateInvoiceRequest struct {
	Status  *string    `json:"status" binding:"omitempty,oneof=draft issued paid overdue cancelled"`
	DueDate *time.Time `json:"due_date,omitempty"`
}

type InvoiceResponse struct {
	InvoiceID      uuid.UUID        `json:"invoice_id"`
	TenantID       uuid.UUID        `json:"tenant_id"`
	RestaurantID   uuid.UUID        `json:"restaurant_id"`
	OrderID        *uuid.UUID       `json:"order_id,omitempty"`
	InvoiceNumber  string           `json:"invoice_number"`
	InvoiceDate    time.Time        `json:"invoice_date"`
	DueDate        *time.Time       `json:"due_date,omitempty"`
	Subtotal       float64          `json:"subtotal"`
	TaxAmount      float64          `json:"tax_amount"`
	DiscountAmount float64          `json:"discount_amount"`
	TotalAmount    float64          `json:"total_amount"`
	Status         string           `json:"status"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	Order          *OrderSummaryDTO `json:"order,omitempty"`
}

type InvoiceListResponse struct {
	Invoices   []InvoiceResponse `json:"invoices"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalPages int               `json:"total_pages"`
}

type InvoiceFilterRequest struct {
	RestaurantID *uuid.UUID `form:"restaurant_id"`
	OrderID      *uuid.UUID `form:"order_id"`
	Status       *string    `form:"status" binding:"omitempty,oneof=draft issued paid overdue cancelled"`
	DateFrom     *time.Time `form:"date_from" time_format:"2006-01-02"`
	DateTo       *time.Time `form:"date_to"   time_format:"2006-01-02"`
	Page         int        `form:"page"      binding:"min=1"`
	PageSize     int        `form:"page_size" binding:"min=1,max=100"`
	SortBy       string     `form:"sort_by"   binding:"omitempty,oneof=invoice_date due_date total_amount status"`
	SortOrder    string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// ===================================
// VALIDATION ERRORS
// ===================================

var (
	ErrInvalidPaymentAmount    = NewValidationError("payment amount must be greater than 0")
	ErrFuturePaymentDate       = NewValidationError("payment date cannot be in the future")
	ErrMinAmountGreaterThanMax = NewValidationError("min amount cannot be greater than max amount")
	ErrDateFromAfterDateTo     = NewValidationError("date_from cannot be after date_to")
	ErrNegativeSubtotal        = NewValidationError("subtotal cannot be negative")
	ErrNegativeTaxAmount       = NewValidationError("tax amount cannot be negative")
	ErrNegativeDiscountAmount  = NewValidationError("discount amount cannot be negative")
	ErrDiscountExceedsSubtotal = NewValidationError("discount amount cannot exceed subtotal")
	ErrDueDateBeforeInvoiceDate = NewValidationError("due date cannot be before invoice date")
)
// models/payment.go
//
// PaymentRecord — FSM-aware payment model with Fonepay QR support.
//
// Status lifecycle (mirrors PaymentFSM in utils/fsm.go):
//
//   pending_qr ──(qr_scanned)──► awaiting_confirm
//       │                              │
//       │                     (payment_received)
//       │                              │
//       │                              ▼
//       │                          confirmed ──(refund)──► refunded (terminal)
//       │                              │
//       │                          (fail)
//       │                              │
//       └──(expire/fail)──────────► failed (terminal)
//
// The VerifyToken field is a server-generated secret stored at QR generation
// time and checked on the Fonepay callback to prevent replay attacks.
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PaymentMethod constants — single source of truth used by service + DTO.
const (
	PaymentMethodCash    = "cash"
	PaymentMethodCard    = "card"
	PaymentMethodFonepay = "fonepay"
	PaymentMethodWallet  = "wallet"
	PaymentMethodOnline  = "online"
)

// PaymentRecord represents a payment transaction, extended for Fonepay QR flow.
type PaymentRecord struct {
	PaymentRecordID uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"payment_record_id"`
	TenantID        uuid.UUID  `gorm:"type:uuid;not null;index"                        json:"tenant_id"`
	RestaurantID    uuid.UUID  `gorm:"type:uuid;not null;index"                        json:"restaurant_id"`
	OrderID         *uuid.UUID `gorm:"type:uuid;index"                                 json:"order_id,omitempty"`

	Amount        float64 `gorm:"type:decimal(12,2);not null"  json:"amount"`
	PaymentMethod string  `gorm:"type:varchar(50);not null"    json:"payment_method"`

	// Status mirrors PaymentState constants in utils/fsm.go:
	//   pending_qr | awaiting_confirm | confirmed | failed | refunded
	Status string `gorm:"type:varchar(50);not null;default:'pending_qr';index" json:"status"`

	// TransactionID is the provider's reference — populated on confirmation.
	TransactionID string `gorm:"type:varchar(255)" json:"transaction_id,omitempty"`

	PaymentDate time.Time `gorm:"not null;index" json:"payment_date"`

	// ── Fonepay QR fields ────────────────────────────────────────────────────
	// QRImageData holds the base64-encoded PNG returned by fonepay.GenerateQR.
	// Stored so the POS can re-display without re-calling the API.
	QRImageData string `gorm:"type:text"          json:"qr_image_data,omitempty"`

	// QRExpiresAt is set to now+30m at generation time.
	// The service refuses to verify against an expired QR.
	QRExpiresAt *time.Time `gorm:"type:timestamptz;index" json:"qr_expires_at,omitempty"`

	// FonepayTransactionID is the PRN echoed back in the QRResponse and used
	// as the correlator when calling VerifyPayment.
	FonepayTransactionID string `gorm:"type:varchar(255)" json:"fonepay_transaction_id,omitempty"`

	// VerifyToken is a server-generated random secret stored at QR generation
	// and checked on the callback to prevent replay / CSRF attacks.
	VerifyToken string `gorm:"type:varchar(128);index" json:"verify_token,omitempty"`

	// EncodedParams is the raw callback query string from Fonepay.
	// Stored for audit / manual re-verification.
	EncodedParams string `gorm:"type:text" json:"encoded_params,omitempty"`
	// ─────────────────────────────────────────────────────────────────────────

	// FailureReason is populated when Status = "failed".
	FailureReason string `gorm:"type:text" json:"failure_reason,omitempty"`

	CreatedAt time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt *time.Time `gorm:"default:now()"          json:"updated_at,omitempty"`

	// Relationships
	Tenant     Tenant     `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE"     json:"-"`
	Restaurant Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
	Order      *Order     `gorm:"foreignKey:OrderID;constraint:OnDelete:SET NULL"     json:"order,omitempty"`
}

func (pr *PaymentRecord) BeforeCreate(tx *gorm.DB) error {
	if pr.PaymentRecordID == uuid.Nil {
		pr.PaymentRecordID = uuid.New()
	}
	return nil
}

// IsQRExpired returns true when the QR code is past its expiry time.
func (pr *PaymentRecord) IsQRExpired() bool {
	if pr.QRExpiresAt == nil {
		return false
	}
	return time.Now().After(*pr.QRExpiresAt)
}

// IsFonepayQR returns true when this record is a Fonepay QR payment.
func (pr *PaymentRecord) IsFonepayQR() bool {
	return pr.PaymentMethod == PaymentMethodFonepay
}

// Invoice model is unchanged.
type Invoice struct {
	InvoiceID      uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"invoice_id"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_tenant_invoice_number" json:"tenant_id"`
	RestaurantID   uuid.UUID  `gorm:"type:uuid;not null;index"                        json:"restaurant_id"`
	OrderID        *uuid.UUID `gorm:"type:uuid;index"                                 json:"order_id,omitempty"`
	InvoiceNumber  string     `gorm:"type:varchar(100);not null;uniqueIndex:idx_tenant_invoice_number" json:"invoice_number"`
	InvoiceDate    time.Time  `gorm:"not null;index"          json:"invoice_date"`
	DueDate        *time.Time `gorm:"type:timestamptz"        json:"due_date,omitempty"`
	Subtotal       float64    `gorm:"type:decimal(12,2);not null" json:"subtotal"`
	TaxAmount      float64    `gorm:"type:decimal(12,2);not null" json:"tax_amount"`
	DiscountAmount float64    `gorm:"type:decimal(12,2);default:0" json:"discount_amount"`
	TotalAmount    float64    `gorm:"type:decimal(12,2);not null" json:"total_amount"`
	Status         string     `gorm:"type:varchar(50);index"  json:"status"`
	CreatedAt      time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"not null;default:now()" json:"updated_at"`

	Tenant     Tenant     `gorm:"foreignKey:TenantID;constraint:OnDelete:CASCADE"     json:"-"`
	Restaurant Restaurant `gorm:"foreignKey:RestaurantID;constraint:OnDelete:CASCADE" json:"-"`
	Order      *Order     `gorm:"foreignKey:OrderID;constraint:OnDelete:SET NULL"     json:"order,omitempty"`
}

func (i *Invoice) BeforeCreate(tx *gorm.DB) error {
	if i.InvoiceID == uuid.Nil {
		i.InvoiceID = uuid.New()
	}
	return nil
}

// ── Migration SQL ─────────────────────────────────────────────────────────────
/*
-- Run this if upgrading from the old payment_records schema.

ALTER TABLE payment_records
	ADD COLUMN IF NOT EXISTS qr_image_data          TEXT,
	ADD COLUMN IF NOT EXISTS qr_expires_at          TIMESTAMPTZ,
	ADD COLUMN IF NOT EXISTS fonepay_transaction_id VARCHAR(255),
	ADD COLUMN IF NOT EXISTS verify_token           VARCHAR(128),
	ADD COLUMN IF NOT EXISTS encoded_params         TEXT,
	ADD COLUMN IF NOT EXISTS failure_reason         TEXT,
	ADD COLUMN IF NOT EXISTS updated_at             TIMESTAMPTZ DEFAULT now();

-- Rename old columns if they exist (safe to run twice with IF EXISTS)
ALTER TABLE payment_records
	RENAME COLUMN fonepay_qr_data   TO qr_image_data;
ALTER TABLE payment_records
	RENAME COLUMN fonepay_qr_string TO fonepay_transaction_id;
ALTER TABLE payment_records
	RENAME COLUMN fonepay_expires_at TO qr_expires_at;

-- Update old status values to new FSM states
UPDATE payment_records SET status = 'pending_qr'        WHERE status = 'pending'   AND payment_method = 'fonepay';
UPDATE payment_records SET status = 'confirmed'         WHERE status = 'completed';
UPDATE payment_records SET status = 'failed'            WHERE status = 'failed';   -- no-op
UPDATE payment_records SET status = 'refunded'          WHERE status = 'refunded'; -- no-op

-- Indexes
CREATE INDEX IF NOT EXISTS idx_payment_records_qr_expires
	ON payment_records(qr_expires_at) WHERE qr_expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_payment_records_verify_token
	ON payment_records(verify_token)  WHERE verify_token IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_payment_records_method
	ON payment_records(payment_method);
*/
// services/business/Payment.go
//
// PaymentService — Fonepay dynamic QR payment engine.
//
// ┌──────────────────────────────────────────────────────────────────────────┐
// │  FLOW                                                                    │
// │                                                                          │
// │  1. POS calls InitiateQRPayment                                          │
// │     → generates PaymentRecord(status=pending_qr)                        │
// │     → calls fonepay.GenerateQR → gets PNG bytes + TransactionID         │
// │     → stores QRImageData (base64), QRExpiresAt (+30m), VerifyToken      │
// │     → returns QRPaymentResponse to POS                                  │
// │                                                                          │
// │  2. POS displays QR image to customer                                   │
// │                                                                          │
// │  3. Customer scans in Fonepay app → Fonepay calls our callback URL      │
// │     → HandleFonepayCallback validates VerifyToken                       │
// │     → fires PaymentFSM: pending_qr → awaiting_confirm                  │
// │     → calls fonepay.VerifyPayment(EncodedParams)                        │
// │     → fires PaymentFSM:                                                 │
// │         ✓ Success  → awaiting_confirm → confirmed                       │
// │         ✗ Failure  → awaiting_confirm → failed                          │
// │                                                                          │
// │  4. Background worker (ExpireStaleQRs) sweeps QRs past their TTL       │
// │     → fires PaymentFSM: pending_qr → failed (expire)                   │
// │                                                                          │
// │  5. Manager calls RefundPayment                                          │
// │     → fires PaymentFSM: confirmed → refunded (guard: manager_approved)  │
// └──────────────────────────────────────────────────────────────────────────┘
//
// Idempotency:
//   InitiateQRPayment is keyed by (OrderID, RestaurantID). A second call while
//   a pending_qr or awaiting_confirm record exists returns the existing QR
//   rather than billing Fonepay twice.
//   A new QR is only issued when the previous record is terminal (failed/refunded)
//   or via the explicit RegenerateQR call.
//
// VerifyToken:
//   A 32-byte random hex string stored in PaymentRecord.VerifyToken and
//   returned to the POS. The POS must include it in the callback URL as
//   ?verify_token=... so our handler can authenticate the callback without
//   shared secrets in URLs.
package business

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/voidarchive/nepal-payment-go/core"
	"github.com/voidarchive/nepal-payment-go/providers/fonepay"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/models"
	"backend/utils"
)

// ─────────────────────────────────────────────────────────────────────────────
// SENTINEL ERRORS
// ─────────────────────────────────────────────────────────────────────────────

var (
	ErrPaymentNotFound        = errors.New("payment record not found")
	ErrPaymentTenantMismatch  = errors.New("payment record does not belong to this tenant")
	ErrQRAlreadyPending       = errors.New("an active QR payment already exists for this order — use RegenerateQR if the previous one expired")
	ErrQRExpired              = errors.New("QR code has expired — call RegenerateQR")
	ErrInvalidVerifyToken     = errors.New("verify_token is invalid or does not match this payment")
	ErrPaymentNotFonepay      = errors.New("this payment record is not a Fonepay QR payment")
	ErrPaymentTerminal        = errors.New("payment is in a terminal state and cannot be modified")
	ErrPaymentInvalidFSMEvent = errors.New("payment state transition not permitted")
	ErrRefundNotApproved      = errors.New("refund requires manager_approved=true in the request context")
	ErrInvoiceNotFound        = errors.New("invoice not found")
)

// ─────────────────────────────────────────────────────────────────────────────
// RESOURCE CONSTANT
// ─────────────────────────────────────────────────────────────────────────────

const (
	ResourcePayment ResourceType = "payment"
)

// ─────────────────────────────────────────────────────────────────────────────
// QR TTL
// ─────────────────────────────────────────────────────────────────────────────

const (
	fonepayQRTTL = 30 * time.Minute
)

// ─────────────────────────────────────────────────────────────────────────────
// INPUT TYPES
// ─────────────────────────────────────────────────────────────────────────────

// PaymentServiceRequest carries caller identity for every mutation.
type PaymentServiceRequest struct {
	TenantID     uuid.UUID
	RestaurantID uuid.UUID
	ActorID      uuid.UUID
}

// InitiateQRInput is the data needed to start a Fonepay QR payment.
type InitiateQRInput struct {
	PaymentServiceRequest
	OrderID     *uuid.UUID
	Amount      float64 // NPR, whole rupees
	Description string
	Remarks2    string
}

// HandleCallbackInput carries the raw Fonepay callback data.
type HandleCallbackInput struct {
	// VerifyToken authenticates the callback — matched against PaymentRecord.VerifyToken.
	VerifyToken string
	// EncodedParams is the raw Fonepay callback query string.
	EncodedParams string
}

// RefundInput requests a refund for a confirmed payment.
type RefundInput struct {
	PaymentServiceRequest
	PaymentRecordID uuid.UUID
	// ManagerApproved must be true — enforced both here and in the FSM guard.
	ManagerApproved bool
}

// CreateCashCardInput creates a non-QR payment record (cash/card).
type CreateCashCardInput struct {
	PaymentServiceRequest
	OrderID       *uuid.UUID
	Amount        float64
	PaymentMethod string // "cash" | "card" | "wallet" | "online"
	TransactionID string
	PaymentDate   time.Time
}

// ListPaymentsFilter controls pagination/filtering for payment listing.
type ListPaymentsFilter struct {
	PaymentServiceRequest
	OrderID       *uuid.UUID
	PaymentMethod *string
	Status        *string
	DateFrom      *time.Time
	DateTo        *time.Time
	MinAmount     *float64
	MaxAmount     *float64
	Page          int
	PageSize      int
	SortBy        string
	SortOrder     string
}

// ─────────────────────────────────────────────────────────────────────────────
// QR PAYMENT RESULT
// ─────────────────────────────────────────────────────────────────────────────

// QRPaymentResult is returned by InitiateQRPayment and RegenerateQR.
type QRPaymentResult struct {
	PaymentRecordID      uuid.UUID
	QRImageBase64        string // PNG bytes base64-encoded for direct <img> embedding
	FonepayTransactionID string
	ExpiresAt            time.Time
	VerifyToken          string
	Amount               float64
}

// ─────────────────────────────────────────────────────────────────────────────
// SERVICE
// ─────────────────────────────────────────────────────────────────────────────

// PaymentService manages the full payment lifecycle including Fonepay QR.
// Goroutine-safe; construct one per application process.
type PaymentService struct {
	db              *gorm.DB
	redis           *goredis.Client
	rbac            RBACAuthorizer
	fonepayProvider *fonepay.Provider
}

// NewPaymentService constructs a ready-to-use PaymentService.
// fonepayProvider may be nil if the deployment does not use Fonepay —
// calls to QR methods will return ErrPaymentNotFonepay in that case.
func NewPaymentService(
	db *gorm.DB,
	redis *goredis.Client,
	rbac RBACAuthorizer,
	fonepayProvider *fonepay.Provider,
) *PaymentService {
	return &PaymentService{
		db:              db,
		redis:           redis,
		rbac:            rbac,
		fonepayProvider: fonepayProvider,
	}
}

// NewPaymentServiceWithFonepay is a convenience constructor that builds the
// Fonepay provider from environment-sourced config.
func NewPaymentServiceWithFonepay(
	db *gorm.DB,
	redis *goredis.Client,
	rbac RBACAuthorizer,
	cfg fonepay.Config,
) (*PaymentService, error) {
	provider, err := fonepay.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("init fonepay provider: %w", err)
	}
	return NewPaymentService(db, redis, rbac, provider), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// LAYER 1 — QR GENERATION
// ─────────────────────────────────────────────────────────────────────────────

// InitiateQRPayment generates a Fonepay dynamic QR and creates a PaymentRecord
// in pending_qr state.
//
// Idempotency: if an active (pending_qr or awaiting_confirm) record already
// exists for the same order, the call returns ErrQRAlreadyPending instead of
// creating a duplicate.
//
// RBAC: requires payment:create.
func (s *PaymentService) InitiateQRPayment(
	ctx context.Context,
	in InitiateQRInput,
) (*QRPaymentResult, error) {
	if s.fonepayProvider == nil {
		return nil, ErrPaymentNotFonepay
	}

	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourcePayment,
		Action:   ActionCreate,
	}); err != nil {
		return nil, err
	}

	// ── Idempotency check ────────────────────────────────────────────────────
	if in.OrderID != nil {
		var existing models.PaymentRecord
		err := s.db.WithContext(ctx).
			Where("order_id = ? AND restaurant_id = ? AND tenant_id = ? AND status IN (?,?)",
				*in.OrderID, in.RestaurantID, in.TenantID,
				utils.PaymentStatePendingQR, utils.PaymentStateAwaitingConfirm).
			First(&existing).Error
		if err == nil {
			return nil, ErrQRAlreadyPending
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("idempotency check: %w", err)
		}
	}

	// ── Generate VerifyToken ─────────────────────────────────────────────────
	verifyToken, err := generateSecureToken()
	if err != nil {
		return nil, fmt.Errorf("generate verify token: %w", err)
	}

	// ── Call Fonepay GenerateQR ──────────────────────────────────────────────
	// Amount: lib takes int64 in whole NPR. We round down from float.
	correlationID := uuid.New().String() // PRN / OrderID for Fonepay
	qrResp, err := s.fonepayProvider.GenerateQR(ctx, &fonepay.QRRequest{
		Amount:      int64(in.Amount),
		OrderID:     correlationID,
		Description: in.Description,
		Remarks2:    in.Remarks2,
	})
	if err != nil {
		return nil, fmt.Errorf("fonepay GenerateQR: %w", err)
	}

	// ── Build PaymentRecord ───────────────────────────────────────────────────
	now := time.Now()
	expiresAt := now.Add(fonepayQRTTL)
	qrBase64 := base64.StdEncoding.EncodeToString(qrResp.QRImageData)

	record := &models.PaymentRecord{
		PaymentRecordID:      uuid.New(),
		TenantID:             in.TenantID,
		RestaurantID:         in.RestaurantID,
		OrderID:              in.OrderID,
		Amount:               in.Amount,
		PaymentMethod:        models.PaymentMethodFonepay,
		Status:               string(utils.PaymentStatePendingQR),
		PaymentDate:          now,
		QRImageData:          qrBase64,
		QRExpiresAt:          &expiresAt,
		FonepayTransactionID: qrResp.TransactionID,
		VerifyToken:          verifyToken,
	}

	if err := s.db.WithContext(ctx).Create(record).Error; err != nil {
		return nil, fmt.Errorf("persist payment record: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventCreate, "PaymentRecord", record.PaymentRecordID.String(),
		nil, map[string]interface{}{
			"method":   models.PaymentMethodFonepay,
			"amount":   in.Amount,
			"order_id": in.OrderID,
			"status":   record.Status,
		})

	return &QRPaymentResult{
		PaymentRecordID:      record.PaymentRecordID,
		QRImageBase64:        qrBase64,
		FonepayTransactionID: qrResp.TransactionID,
		ExpiresAt:            expiresAt,
		VerifyToken:          verifyToken,
		Amount:               in.Amount,
	}, nil
}

// RegenerateQR issues a fresh QR for a payment that is in failed state
// (e.g., because the previous QR expired). Creates a new PaymentRecord;
// the old one remains for audit purposes.
//
// RBAC: requires payment:create.
func (s *PaymentService) RegenerateQR(
	ctx context.Context,
	in PaymentServiceRequest,
	paymentRecordID uuid.UUID,
) (*QRPaymentResult, error) {
	if s.fonepayProvider == nil {
		return nil, ErrPaymentNotFonepay
	}

	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourcePayment,
		Action:   ActionCreate,
	}); err != nil {
		return nil, err
	}

	old, err := s.getPaymentForWrite(ctx, in.TenantID, paymentRecordID)
	if err != nil {
		return nil, err
	}
	if !old.IsFonepayQR() {
		return nil, ErrPaymentNotFonepay
	}
	if old.Status != string(utils.PaymentStateFailed) {
		return nil, fmt.Errorf("can only regenerate QR for failed payments (current status: %s)", old.Status)
	}

	return s.InitiateQRPayment(ctx, InitiateQRInput{
		PaymentServiceRequest: in,
		OrderID:               old.OrderID,
		Amount:                old.Amount,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// LAYER 2 — FONEPAY CALLBACK (FSM: pending_qr → awaiting_confirm → confirmed/failed)
// ─────────────────────────────────────────────────────────────────────────────

// HandleFonepayCallback processes the Fonepay payment gateway callback.
//
// Pipeline:
//  1. Look up PaymentRecord by VerifyToken
//  2. Validate token matches
//  3. Fire FSM: pending_qr → awaiting_confirm
//  4. Call provider.VerifyPayment(EncodedParams)
//  5. Fire FSM: awaiting_confirm → confirmed  (success)
//               awaiting_confirm → failed     (failure)
//  6. Persist updated record + store EncodedParams for audit
//  7. If confirmed, advance Order FSM → served
func (s *PaymentService) HandleFonepayCallback(
	ctx context.Context,
	in HandleCallbackInput,
) (*models.PaymentRecord, error) {
	if s.fonepayProvider == nil {
		return nil, ErrPaymentNotFonepay
	}

	// ── 1. Find record by VerifyToken ────────────────────────────────────────
	var record models.PaymentRecord
	if err := s.db.WithContext(ctx).
		Where("verify_token = ?", in.VerifyToken).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidVerifyToken
		}
		return nil, fmt.Errorf("lookup by verify_token: %w", err)
	}

	// ── 2. Guard: only act on pending_qr records ─────────────────────────────
	if record.Status != string(utils.PaymentStatePendingQR) {
		// Already processed (could be a duplicate callback). Return current state.
		return &record, nil
	}

	// ── 3. Guard: QR must not be expired ─────────────────────────────────────
	if record.IsQRExpired() {
		return s.failPayment(ctx, &record, "QR code expired before callback received")
	}

	// ── 4. FSM: pending_qr → awaiting_confirm ────────────────────────────────
	if err := s.firePaymentFSM(ctx, &record, utils.PaymentEventQRScanned, nil); err != nil {
		return nil, err
	}

	// ── 5. Store EncodedParams and persist awaiting_confirm state ────────────
	record.EncodedParams = in.EncodedParams
	if err := s.db.WithContext(ctx).
		Model(&record).
		Updates(map[string]interface{}{
			"status":         record.Status,
			"encoded_params": in.EncodedParams,
			"updated_at":     time.Now(),
		}).Error; err != nil {
		return nil, fmt.Errorf("persist awaiting_confirm: %w", err)
	}

	// ── 6. Call Fonepay VerifyPayment ─────────────────────────────────────────
	// core.VerifyRequest.EncodedParams is map[string]string — parse the raw
	// query string (e.g. "PRN=x&BID=y&AMT=z") before handing it to the provider.
	parsedParams, parseErr := url.ParseQuery(in.EncodedParams)
	if parseErr != nil {
		return s.failPayment(ctx, &record, fmt.Sprintf("invalid callback params: %s", parseErr.Error()))
	}
	paramMap := make(map[string]string, len(parsedParams))
	for k, vs := range parsedParams {
		if len(vs) > 0 {
			paramMap[k] = vs[0]
		}
	}

	verifyResp, err := s.fonepayProvider.VerifyPayment(ctx, &core.VerifyRequest{
		EncodedParams: paramMap,
	})
	if err != nil {
		// Network/provider error — mark as failed
		return s.failPayment(ctx, &record, fmt.Sprintf("VerifyPayment error: %s", err.Error()))
	}

	// ── 7. FSM: awaiting_confirm → confirmed / failed ─────────────────────────
	if !verifyResp.Success {
		reason := fmt.Sprintf("Fonepay verification failed (status: %s)", verifyResp.Status)
		return s.failPayment(ctx, &record, reason)
	}

	// Success path
	if err := s.firePaymentFSM(ctx, &record, utils.PaymentEventReceived, nil); err != nil {
		return nil, err
	}
	record.TransactionID = verifyResp.TransactionID

	if err := s.db.WithContext(ctx).
		Model(&record).
		Updates(map[string]interface{}{
			"status":         record.Status,
			"transaction_id": verifyResp.TransactionID,
			"updated_at":     time.Now(),
		}).Error; err != nil {
		return nil, fmt.Errorf("persist confirmed: %w", err)
	}

	// ── 8. Advance Order FSM → served (best-effort, non-fatal) ───────────────
	if record.OrderID != nil {
		go s.tryAdvanceOrderToServed(context.Background(), record.TenantID, *record.OrderID)
	}

	s.writeAudit(ctx, record.TenantID, &record.RestaurantID, uuid.Nil,
		models.AuditEventUpdate, "PaymentRecord", record.PaymentRecordID.String(),
		map[string]interface{}{"status": string(utils.PaymentStateAwaitingConfirm)},
		map[string]interface{}{"status": record.Status, "transaction_id": record.TransactionID})

	return &record, nil
}

// failPayment fires the FSM fail event and persists the failure reason.
func (s *PaymentService) failPayment(
	ctx context.Context,
	record *models.PaymentRecord,
	reason string,
) (*models.PaymentRecord, error) {
	meta := map[string]string{"failure_reason": reason}
	if err := s.firePaymentFSM(ctx, record, utils.PaymentEventFail, meta); err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).
		Model(record).
		Updates(map[string]interface{}{
			"status":         record.Status,
			"failure_reason": reason,
			"updated_at":     time.Now(),
		}).Error; err != nil {
		return nil, fmt.Errorf("persist failure: %w", err)
	}
	return record, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// LAYER 3 — REFUND
// ─────────────────────────────────────────────────────────────────────────────

// RefundPayment fires confirmed → refunded. Requires ManagerApproved.
// RBAC: requires payment:update.
func (s *PaymentService) RefundPayment(
	ctx context.Context,
	in RefundInput,
) (*models.PaymentRecord, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourcePayment,
		Action:   ActionUpdate,
	}); err != nil {
		return nil, err
	}

	if !in.ManagerApproved {
		return nil, ErrRefundNotApproved
	}

	record, err := s.getPaymentForWrite(ctx, in.TenantID, in.PaymentRecordID)
	if err != nil {
		return nil, err
	}
	if record.RestaurantID != in.RestaurantID {
		return nil, ErrPaymentTenantMismatch
	}

	meta := map[string]string{"manager_approved": "true"}
	if err := s.firePaymentFSM(ctx, record, utils.PaymentEventRefund, meta); err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).
		Model(record).
		Update("status", record.Status).Error; err != nil {
		return nil, fmt.Errorf("persist refund: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventUpdate, "PaymentRecord", record.PaymentRecordID.String(),
		map[string]interface{}{"status": string(utils.PaymentStateConfirmed)},
		map[string]interface{}{"status": record.Status})

	return record, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// LAYER 4 — NON-QR PAYMENTS (cash / card)
// ─────────────────────────────────────────────────────────────────────────────

// RecordCashOrCard creates a payment record for cash/card methods that are
// confirmed immediately (no async verification needed).
// RBAC: requires payment:create.
func (s *PaymentService) RecordCashOrCard(
	ctx context.Context,
	in CreateCashCardInput,
) (*models.PaymentRecord, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourcePayment,
		Action:   ActionCreate,
	}); err != nil {
		return nil, err
	}

	record := &models.PaymentRecord{
		PaymentRecordID: uuid.New(),
		TenantID:        in.TenantID,
		RestaurantID:    in.RestaurantID,
		OrderID:         in.OrderID,
		Amount:          in.Amount,
		PaymentMethod:   in.PaymentMethod,
		// Cash/card are confirmed immediately — skip the QR states entirely.
		Status:        string(utils.PaymentStateConfirmed),
		TransactionID: in.TransactionID,
		PaymentDate:   in.PaymentDate,
	}

	if err := s.db.WithContext(ctx).Create(record).Error; err != nil {
		return nil, fmt.Errorf("persist cash/card payment: %w", err)
	}

	s.writeAudit(ctx, in.TenantID, &in.RestaurantID, in.ActorID,
		models.AuditEventCreate, "PaymentRecord", record.PaymentRecordID.String(),
		nil, map[string]interface{}{"method": in.PaymentMethod, "amount": in.Amount})

	return record, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// LAYER 5 — READ PATH
// ─────────────────────────────────────────────────────────────────────────────

// GetPayment returns a single PaymentRecord.
// RBAC: requires payment:read.
func (s *PaymentService) GetPayment(
	ctx context.Context,
	in PaymentServiceRequest,
	paymentRecordID uuid.UUID,
) (*models.PaymentRecord, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourcePayment,
		Action:   ActionRead,
	}); err != nil {
		return nil, err
	}

	return s.getPaymentForWrite(ctx, in.TenantID, paymentRecordID)
}

// ListPayments returns a paginated, filtered list of payment records.
// RBAC: requires payment:read.
func (s *PaymentService) ListPayments(
	ctx context.Context,
	f ListPaymentsFilter,
) ([]models.PaymentRecord, int64, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   f.ActorID,
		TenantID: f.TenantID,
		Resource: ResourcePayment,
		Action:   ActionRead,
	}); err != nil {
		return nil, 0, err
	}

	q := s.db.WithContext(ctx).
		Where("tenant_id = ? AND restaurant_id = ?", f.TenantID, f.RestaurantID)

	if f.OrderID != nil {
		q = q.Where("order_id = ?", *f.OrderID)
	}
	if f.PaymentMethod != nil {
		q = q.Where("payment_method = ?", *f.PaymentMethod)
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if f.DateFrom != nil {
		q = q.Where("payment_date >= ?", *f.DateFrom)
	}
	if f.DateTo != nil {
		q = q.Where("payment_date <= ?", *f.DateTo)
	}
	if f.MinAmount != nil {
		q = q.Where("amount >= ?", *f.MinAmount)
	}
	if f.MaxAmount != nil {
		q = q.Where("amount <= ?", *f.MaxAmount)
	}

	var total int64
	if err := q.Model(&models.PaymentRecord{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count payments: %w", err)
	}

	sortCol := "payment_date"
	switch f.SortBy {
	case "amount":
		sortCol = "amount"
	case "status":
		sortCol = "status"
	}
	sortDir := "DESC"
	if strings.EqualFold(f.SortOrder, "asc") {
		sortDir = "ASC"
	}
	q = q.Order(fmt.Sprintf("%s %s", sortCol, sortDir))

	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	q = q.Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize)

	var records []models.PaymentRecord
	if err := q.Find(&records).Error; err != nil {
		return nil, 0, fmt.Errorf("list payments: %w", err)
	}
	return records, total, nil
}

// GetPaymentStats returns revenue summary for a restaurant.
// RBAC: requires payment:read.
func (s *PaymentService) GetPaymentStats(
	ctx context.Context,
	in PaymentServiceRequest,
) (*PaymentStats, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourcePayment,
		Action:   ActionRead,
	}); err != nil {
		return nil, err
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekAgo := today.AddDate(0, 0, -7)
	monthAgo := today.AddDate(0, -1, 0)

	base := s.db.WithContext(ctx).Model(&models.PaymentRecord{}).
		Where("tenant_id = ? AND restaurant_id = ? AND status = ?",
			in.TenantID, in.RestaurantID, utils.PaymentStateConfirmed)

	type aggRow struct {
		Total float64
		Count int64
	}

	var overall aggRow
	_ = base.Select("SUM(amount) as total, COUNT(*) as count").Scan(&overall).Error

	var todayTotal float64
	_ = base.Where("payment_date >= ?", today).Select("COALESCE(SUM(amount),0)").Scan(&todayTotal).Error

	var weekTotal float64
	_ = base.Where("payment_date >= ?", weekAgo).Select("COALESCE(SUM(amount),0)").Scan(&weekTotal).Error

	var monthTotal float64
	_ = base.Where("payment_date >= ?", monthAgo).Select("COALESCE(SUM(amount),0)").Scan(&monthTotal).Error

	type methodRow struct {
		PaymentMethod string
		Total         float64
	}
	var byMethod []methodRow
	_ = base.Select("payment_method, COALESCE(SUM(amount),0) as total").
		Group("payment_method").Scan(&byMethod).Error

	revenueByMethod := make(map[string]float64)
	for _, r := range byMethod {
		revenueByMethod[r.PaymentMethod] = r.Total
	}

	avg := 0.0
	if overall.Count > 0 {
		avg = overall.Total / float64(overall.Count)
	}

	return &PaymentStats{
		TotalRevenue:      overall.Total,
		TotalTransactions: overall.Count,
		AvgTransactionVal: avg,
		RevenueByMethod:   revenueByMethod,
		TodayRevenue:      todayTotal,
		WeekRevenue:       weekTotal,
		MonthRevenue:      monthTotal,
	}, nil
}

// PaymentStats is the revenue summary.
type PaymentStats struct {
	TotalRevenue      float64            `json:"total_revenue"`
	TotalTransactions int64              `json:"total_transactions"`
	AvgTransactionVal float64            `json:"avg_transaction_value"`
	RevenueByMethod   map[string]float64 `json:"revenue_by_method"`
	TodayRevenue      float64            `json:"today_revenue"`
	WeekRevenue       float64            `json:"week_revenue"`
	MonthRevenue      float64            `json:"month_revenue"`
}

// ─────────────────────────────────────────────────────────────────────────────
// LAYER 6 — BACKGROUND WORKER: expire stale QRs
// ─────────────────────────────────────────────────────────────────────────────

// ExpireStaleQRs scans for pending_qr records whose QRExpiresAt has passed
// and fires the PaymentEventExpire transition on each one.
//
// Call this from a cron job or a ticker goroutine — recommended interval: 1m.
//
//	go func() {
//	    t := time.NewTicker(time.Minute)
//	    for range t.C {
//	        _ = svc.ExpireStaleQRs(context.Background())
//	    }
//	}()
func (s *PaymentService) ExpireStaleQRs(ctx context.Context) error {
	var stale []models.PaymentRecord
	if err := s.db.WithContext(ctx).
		Where("status = ? AND qr_expires_at < ?",
			utils.PaymentStatePendingQR, time.Now()).
		Find(&stale).Error; err != nil {
		return fmt.Errorf("query stale QRs: %w", err)
	}

	for i := range stale {
		record := &stale[i]
		if _, err := s.failPayment(ctx, record, "QR code expired"); err != nil {
			// Log but continue — don't let one bad record block the rest
			_ = err
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// FSM INTEGRATION
// ─────────────────────────────────────────────────────────────────────────────

// paymentContextAdapter bridges models.PaymentRecord to utils.PaymentContext.
type paymentContextAdapter struct{ *models.PaymentRecord }

func (a *paymentContextAdapter) GetPaymentID() string       { return a.PaymentRecordID.String() }
func (a *paymentContextAdapter) SetStatus(s string)         { a.Status = s }
func (a *paymentContextAdapter) GetAmount() float64         { return a.Amount }
func (a *paymentContextAdapter) GetFailureReason() string   { return a.FailureReason }
func (a *paymentContextAdapter) SetFailureReason(r string)  { a.FailureReason = r }

// firePaymentFSM fires a PaymentEvent on a record, mutating its Status in-place.
// The caller is responsible for persisting the updated status to DB.
func (s *PaymentService) firePaymentFSM(
	ctx context.Context,
	record *models.PaymentRecord,
	event utils.PaymentEvent,
	meta map[string]string,
) error {
	adapter := &paymentContextAdapter{record}
	machine := utils.PaymentFSM.Restore(adapter, utils.PaymentState(record.Status))

	env := utils.NewEnvelope(event, nil)
	for k, v := range meta {
		env = env.WithMeta(k, v)
	}

	if err := machine.Send(ctx, env); err != nil {
		return fmt.Errorf("%w: %s → %s: %s",
			ErrPaymentInvalidFSMEvent, record.Status, string(event), err.Error())
	}
	return nil
}

// tryAdvanceOrderToServed fires OrderEventServe on the parent order when a
// payment is confirmed. Non-fatal — runs in a goroutine.
func (s *PaymentService) tryAdvanceOrderToServed(
	ctx context.Context,
	tenantID, orderID uuid.UUID,
) {
	var order models.Order
	if err := s.db.WithContext(ctx).
		Where("order_id = ? AND tenant_id = ?", orderID, tenantID).
		First(&order).Error; err != nil {
		return
	}

	adapter := &orderContextAdapter{&order}
	machine := utils.OrderFSM.Restore(adapter, utils.OrderState(order.OrderStatus))
	env := utils.NewEnvelope(utils.OrderEventServe, nil)
	if err := machine.Send(ctx, env); err != nil {
		return // already served or wrong state — fine
	}

	_ = s.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("order_id = ?", orderID).
		Update("order_status", order.OrderStatus).Error
}

// ─────────────────────────────────────────────────────────────────────────────
// INVOICE LIFECYCLE (thin layer — unchanged from original design)
// ─────────────────────────────────────────────────────────────────────────────

// CreateInvoice generates an Invoice for an order.
// RBAC: requires payment:create.
func (s *PaymentService) CreateInvoice(
	ctx context.Context,
	in PaymentServiceRequest,
	orderID *uuid.UUID,
	subtotal, taxAmount, discountAmount float64,
	invoiceDate time.Time,
	dueDate *time.Time,
) (*models.Invoice, error) {
	if err := s.rbac.Require(ctx, &AccessRequest{
		UserID:   in.ActorID,
		TenantID: in.TenantID,
		Resource: ResourcePayment,
		Action:   ActionCreate,
	}); err != nil {
		return nil, err
	}

	invoiceNumber, err := s.generateInvoiceNumber(ctx, in.TenantID, invoiceDate)
	if err != nil {
		return nil, err
	}

	inv := &models.Invoice{
		InvoiceID:      uuid.New(),
		TenantID:       in.TenantID,
		RestaurantID:   in.RestaurantID,
		OrderID:        orderID,
		InvoiceNumber:  invoiceNumber,
		InvoiceDate:    invoiceDate,
		DueDate:        dueDate,
		Subtotal:       subtotal,
		TaxAmount:      taxAmount,
		DiscountAmount: discountAmount,
		TotalAmount:    subtotal + taxAmount - discountAmount,
		Status:         "issued",
	}

	if err := s.db.WithContext(ctx).Create(inv).Error; err != nil {
		return nil, fmt.Errorf("create invoice: %w", err)
	}
	return inv, nil
}

// generateInvoiceNumber returns a tenant-scoped invoice number:
// INV-YYYYMM-{seq:06d}
func (s *PaymentService) generateInvoiceNumber(
	ctx context.Context,
	tenantID uuid.UUID,
	t time.Time,
) (string, error) {
	monthStr := t.Format("200601")
	seqKey := fmt.Sprintf("invoice:seq:%s:%s", tenantID, monthStr)

	seq, err := s.redis.Incr(ctx, seqKey).Result()
	if err != nil {
		// Fallback: count existing invoices this month + 1
		var count int64
		_ = s.db.WithContext(ctx).Model(&models.Invoice{}).
			Where("tenant_id = ? AND invoice_date >= ?", tenantID,
				time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())).
			Count(&count).Error
		seq = count + 1
	} else if seq == 1 {
		// First of the month — set TTL to end of next month
		nextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location()).Add(48 * time.Hour)
		_ = s.redis.ExpireAt(ctx, seqKey, nextMonth).Err()
	}

	return fmt.Sprintf("INV-%s-%06d", monthStr, seq), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// INTERNAL HELPERS
// ─────────────────────────────────────────────────────────────────────────────

// getPaymentForWrite fetches a PaymentRecord by ID with tenant ownership check.
func (s *PaymentService) getPaymentForWrite(
	ctx context.Context,
	tenantID, paymentRecordID uuid.UUID,
) (*models.PaymentRecord, error) {
	var record models.PaymentRecord
	if err := s.db.WithContext(ctx).
		Where("payment_record_id = ? AND tenant_id = ?", paymentRecordID, tenantID).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("get payment: %w", err)
	}
	return &record, nil
}

// generateSecureToken returns a 32-byte random hex string.
func generateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// writeAudit persists an AuditTrail row. Errors are silenced.
func (s *PaymentService) writeAudit(
	ctx context.Context,
	tenantID uuid.UUID,
	restaurantID *uuid.UUID,
	actorID uuid.UUID,
	event models.AuditEvent,
	entityType, entityID string,
	oldValues, newValues interface{},
) {
	var userID *uuid.UUID
	if actorID != uuid.Nil {
		userID = &actorID
	}
	entry := &models.AuditTrail{
		AuditTrailID:     uuid.New(),
		TenantID:         tenantID,
		UserID:           userID,
		RestaurantID:     restaurantID,
		EventType:        event,
		EventCategory:    "payment",
		EventDescription: fmt.Sprintf("%s on %s %s", event, entityType, entityID),
		Severity:         models.AuditSeverityInfo,
		EntityType:       entityType,
		EntityID:         entityID,
		OldValues:        mustMarshalJSONB(oldValues),
		NewValues:        mustMarshalJSONB(newValues),
		RiskLevel:        models.RiskLevelLow,
		IsPCIRelevant:    true, // payment events are always PCI-relevant
		Timestamp:        time.Now(),
	}
	_ = s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(entry).Error
}
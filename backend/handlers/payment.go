package handlers

import (
	"strconv"
	"time"

	"backend/middleware"
	"backend/services/business"
	"backend/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// ============================================================================
// PAYMENT HANDLER
// ============================================================================

type PaymentHandler struct {
	svc *business.PaymentService
}

func NewPaymentHandler(svc *business.PaymentService) *PaymentHandler {
	return &PaymentHandler{svc: svc}
}

// ── QR Payments ──────────────────────────────────────────────────────────────

// InitiateQR — POST /api/v1/payments/qr
func (h *PaymentHandler) InitiateQR(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	var body struct {
		RestaurantID string  `json:"restaurant_id"`
		OrderID      string  `json:"order_id"`
		Amount       float64 `json:"amount"`
		Description  string  `json:"description"`
		Remarks2     string  `json:"remarks2"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	restaurantID, err := uuid.Parse(body.RestaurantID)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid restaurant_id", nil)
	}

	var orderID *uuid.UUID
	if body.OrderID != "" {
		parsed, err := uuid.Parse(body.OrderID)
		if err != nil {
			return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid order_id", nil)
		}
		orderID = &parsed
	}

	result, err := h.svc.InitiateQRPayment(c.Context(), business.InitiateQRInput{
		PaymentServiceRequest: business.PaymentServiceRequest{
			TenantID:     auth.TenantID,
			RestaurantID: restaurantID,
			ActorID:      auth.UserID,
		},
		OrderID:     orderID,
		Amount:      body.Amount,
		Description: body.Description,
		Remarks2:    body.Remarks2,
	})
	if err != nil {
		return mapPaymentError(c, err)
	}

	return utils.SendResponse(c, fiber.StatusCreated, "QR payment initiated", result)
}

// RegenerateQR — POST /api/v1/payments/:id/regenerate-qr
func (h *PaymentHandler) RegenerateQR(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	paymentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid payment ID", nil)
	}

	var body struct {
		RestaurantID string `json:"restaurant_id"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	restaurantID, err := uuid.Parse(body.RestaurantID)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid restaurant_id", nil)
	}

	result, err := h.svc.RegenerateQR(c.Context(), business.PaymentServiceRequest{
		TenantID:     auth.TenantID,
		RestaurantID: restaurantID,
		ActorID:      auth.UserID,
	}, paymentID)
	if err != nil {
		return mapPaymentError(c, err)
	}

	return utils.SendResponse(c, fiber.StatusCreated, "QR regenerated", result)
}

// FonepayCallback — POST /api/v1/payments/fonepay/callback (public)
func (h *PaymentHandler) FonepayCallback(c fiber.Ctx) error {
	verifyToken := c.Query("verify_token")
	if verifyToken == "" {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Missing verify_token", nil)
	}

	record, err := h.svc.HandleFonepayCallback(c.Context(), business.HandleCallbackInput{
		VerifyToken:   verifyToken,
		EncodedParams: string(c.Body()),
	})
	if err != nil {
		return mapPaymentError(c, err)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Payment callback processed", record)
}

// ── Cash / Card ──────────────────────────────────────────────────────────────

// RecordPayment — POST /api/v1/payments
func (h *PaymentHandler) RecordPayment(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	var body struct {
		RestaurantID  string  `json:"restaurant_id"`
		OrderID       string  `json:"order_id"`
		Amount        float64 `json:"amount"`
		PaymentMethod string  `json:"payment_method"`
		TransactionID string  `json:"transaction_id"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	restaurantID, err := uuid.Parse(body.RestaurantID)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid restaurant_id", nil)
	}

	var orderID *uuid.UUID
	if body.OrderID != "" {
		parsed, err := uuid.Parse(body.OrderID)
		if err != nil {
			return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid order_id", nil)
		}
		orderID = &parsed
	}

	record, err := h.svc.RecordCashOrCard(c.Context(), business.CreateCashCardInput{
		PaymentServiceRequest: business.PaymentServiceRequest{
			TenantID:     auth.TenantID,
			RestaurantID: restaurantID,
			ActorID:      auth.UserID,
		},
		OrderID:       orderID,
		Amount:        body.Amount,
		PaymentMethod: body.PaymentMethod,
		TransactionID: body.TransactionID,
		PaymentDate:   time.Now(),
	})
	if err != nil {
		return mapPaymentError(c, err)
	}

	return utils.SendResponse(c, fiber.StatusCreated, "Payment recorded", record)
}

// ── Read Path ────────────────────────────────────────────────────────────────

// Get — GET /api/v1/payments/:id
func (h *PaymentHandler) Get(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	paymentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid payment ID", nil)
	}

	restaurantID, err := uuid.Parse(c.Query("restaurant_id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "restaurant_id query param required", nil)
	}

	record, err := h.svc.GetPayment(c.Context(), business.PaymentServiceRequest{
		TenantID:     auth.TenantID,
		RestaurantID: restaurantID,
		ActorID:      auth.UserID,
	}, paymentID)
	if err != nil {
		return mapPaymentError(c, err)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Payment retrieved", record)
}

// List — GET /api/v1/payments
func (h *PaymentHandler) List(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	restaurantID, err := uuid.Parse(c.Query("restaurant_id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "restaurant_id query param required", nil)
	}

	page := 1
	if v := c.Query("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			page = p
		}
	}
	pageSize := 20
	if v := c.Query("page_size"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			pageSize = p
		}
	}

	filter := business.ListPaymentsFilter{
		PaymentServiceRequest: business.PaymentServiceRequest{
			TenantID:     auth.TenantID,
			RestaurantID: restaurantID,
			ActorID:      auth.UserID,
		},
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.Query("sort_by", "payment_date"),
		SortOrder: c.Query("sort_order", "desc"),
	}

	if method := c.Query("payment_method"); method != "" {
		filter.PaymentMethod = &method
	}
	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}
	if oid := c.Query("order_id"); oid != "" {
		if parsed, err := uuid.Parse(oid); err == nil {
			filter.OrderID = &parsed
		}
	}

	records, total, err := h.svc.ListPayments(c.Context(), filter)
	if err != nil {
		return mapPaymentError(c, err)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Payments retrieved", fiber.Map{
		"payments": records,
		"total":    total,
		"page":     filter.Page,
		"pageSize": filter.PageSize,
	})
}

// Stats — GET /api/v1/payments/stats
func (h *PaymentHandler) Stats(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	restaurantID, err := uuid.Parse(c.Query("restaurant_id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "restaurant_id query param required", nil)
	}

	stats, err := h.svc.GetPaymentStats(c.Context(), business.PaymentServiceRequest{
		TenantID:     auth.TenantID,
		RestaurantID: restaurantID,
		ActorID:      auth.UserID,
	})
	if err != nil {
		return mapPaymentError(c, err)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Payment stats retrieved", stats)
}

// ── Refund ────────────────────────────────────────────────────────────────────

// Refund — POST /api/v1/payments/:id/refund
func (h *PaymentHandler) Refund(c fiber.Ctx) error {
	auth := middleware.GetAuthContext(c)
	if auth == nil {
		return utils.SendResponse(c, fiber.StatusUnauthorized, "Not authenticated", nil)
	}

	paymentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid payment ID", nil)
	}

	var body struct {
		RestaurantID    string `json:"restaurant_id"`
		ManagerApproved bool   `json:"manager_approved"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid request body", nil)
	}

	restaurantID, err := uuid.Parse(body.RestaurantID)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid restaurant_id", nil)
	}

	record, err := h.svc.RefundPayment(c.Context(), business.RefundInput{
		PaymentServiceRequest: business.PaymentServiceRequest{
			TenantID:     auth.TenantID,
			RestaurantID: restaurantID,
			ActorID:      auth.UserID,
		},
		PaymentRecordID: paymentID,
		ManagerApproved: body.ManagerApproved,
	})
	if err != nil {
		return mapPaymentError(c, err)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Payment refunded", record)
}

// ── Error Mapping ────────────────────────────────────────────────────────────

func mapPaymentError(c fiber.Ctx, err error) error {
	switch err {
	case business.ErrPaymentNotFound:
		return utils.SendResponse(c, fiber.StatusNotFound, err.Error(), nil)
	case business.ErrPaymentTenantMismatch:
		return utils.SendResponse(c, fiber.StatusForbidden, err.Error(), nil)
	case business.ErrQRAlreadyPending:
		return utils.SendResponse(c, fiber.StatusConflict, err.Error(), nil)
	case business.ErrQRExpired:
		return utils.SendResponse(c, fiber.StatusGone, err.Error(), nil)
	case business.ErrInvalidVerifyToken:
		return utils.SendResponse(c, fiber.StatusUnauthorized, err.Error(), nil)
	case business.ErrPaymentNotFonepay:
		return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
	case business.ErrRefundNotApproved:
		return utils.SendResponse(c, fiber.StatusForbidden, err.Error(), nil)
	case business.ErrPaymentTerminal:
		return utils.SendResponse(c, fiber.StatusConflict, err.Error(), nil)
	default:
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Payment operation failed", nil)
	}
}

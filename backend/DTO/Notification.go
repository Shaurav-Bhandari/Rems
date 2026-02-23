package DTO

import (
	"time"

	"github.com/google/uuid"
)

// ===================================
// NOTIFICATION DTOs
// ===================================

// CreateNotificationRequest
type CreateNotificationRequest struct {
	UserID           *uuid.UUID `json:"user_id,omitempty"`     // nil = broadcast to all tenant users
	Message          string     `json:"message" binding:"required,min=1,max=1000"`
	NotificationType string     `json:"notification_type" binding:"required,oneof=Info Warning Error Success Alert"`
}

func (r *CreateNotificationRequest) Validate() error {
	if len(r.Message) == 0 {
		return ErrEmptyNotificationMessage
	}
	return nil
}

// MarkNotificationReadRequest
type MarkNotificationReadRequest struct {
	NotificationIDs []uuid.UUID `json:"notification_ids" binding:"required,min=1"`
}

func (r *MarkNotificationReadRequest) Validate() error {
	if len(r.NotificationIDs) == 0 {
		return ErrNoNotificationIDs
	}
	return nil
}

// NotificationResponse
type NotificationResponse struct {
	NotificationID   uuid.UUID  `json:"notification_id"`
	TenantID         uuid.UUID  `json:"tenant_id"`
	UserID           *uuid.UUID `json:"user_id,omitempty"`
	Message          string     `json:"message"`
	NotificationType string     `json:"notification_type"`
	IsRead           bool       `json:"is_read"`
	CreatedAt        time.Time  `json:"created_at"`
}

// NotificationListResponse
type NotificationListResponse struct {
	Notifications []NotificationResponse `json:"notifications"`
	Total         int64                  `json:"total"`
	UnreadCount   int64                  `json:"unread_count"`
	Page          int                    `json:"page"`
	PageSize      int                    `json:"page_size"`
	TotalPages    int                    `json:"total_pages"`
}

// NotificationFilterRequest
type NotificationFilterRequest struct {
	UserID           *uuid.UUID `form:"user_id"`
	NotificationType *string    `form:"notification_type" binding:"omitempty,oneof=Info Warning Error Success Alert"`
	IsRead           *bool      `form:"is_read"`
	DateFrom         *time.Time `form:"date_from" time_format:"2006-01-02"`
	DateTo           *time.Time `form:"date_to" time_format:"2006-01-02"`
	Page             int        `form:"page" binding:"min=1"`
	PageSize         int        `form:"page_size" binding:"min=1,max=100"`
	SortBy           string     `form:"sort_by" binding:"omitempty,oneof=created_at notification_type is_read"`
	SortOrder        string     `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// ===================================
// WEBHOOK DTOs
// ===================================

// CreateWebhookRequest
type CreateWebhookRequest struct {
	URL       string `json:"url" binding:"required,url,max=2000"`
	EventType string `json:"event_type" binding:"required,max=100"`
	Secret    string `json:"secret" binding:"omitempty,min=16,max=255"`
}

func (r *CreateWebhookRequest) Validate() error {
	validEventTypes := []string{
		"order.created",
		"order.updated",
		"order.cancelled",
		"payment.completed",
		"payment.failed",
		"inventory.low_stock",
		"inventory.out_of_stock",
		"kot.created",
		"kot.completed",
		"subscription.expiring",
		"subscription.expired",
	}
	for _, valid := range validEventTypes {
		if r.EventType == valid {
			return nil
		}
	}
	return ErrInvalidWebhookEventType
}

// UpdateWebhookRequest
type UpdateWebhookRequest struct {
	URL       *string `json:"url" binding:"omitempty,url,max=2000"`
	EventType *string `json:"event_type" binding:"omitempty,max=100"`
	Secret    *string `json:"secret" binding:"omitempty,min=16,max=255"`
	IsActive  *bool   `json:"is_active"`
}

// WebhookResponse
type WebhookResponse struct {
	WebhookID uuid.UUID `json:"webhook_id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	URL       string    `json:"url"`
	EventType string    `json:"event_type"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	// NOTE: Secret is intentionally excluded from response for security
}

// WebhookListResponse
type WebhookListResponse struct {
	Webhooks   []WebhookResponse `json:"webhooks"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalPages int               `json:"total_pages"`
}

// WebhookFilterRequest
type WebhookFilterRequest struct {
	EventType *string `form:"event_type"`
	IsActive  *bool   `form:"is_active"`
	Page      int     `form:"page" binding:"min=1"`
	PageSize  int     `form:"page_size" binding:"min=1,max=100"`
}

// WebhookTestRequest - manually trigger a test webhook
type WebhookTestRequest struct {
	WebhookID uuid.UUID `json:"webhook_id" binding:"required"`
}

// ===================================
// VALIDATION ERRORS
// ===================================

var (
	ErrEmptyNotificationMessage = NewValidationError("notification message cannot be empty")
	ErrNoNotificationIDs        = NewValidationError("at least one notification ID is required")
	ErrInvalidWebhookEventType  = NewValidationError("invalid webhook event type")
)
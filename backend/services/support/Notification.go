// services/support/Notification.go
//
// NotificationService -- in-app notification delivery, read/unread tracking,
// webhook management, and bulk operations. All operations are tenant-scoped.
// Notifications are stored in PostgreSQL with a Redis cache for badge counts.
package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// SENTINEL ERRORS
// ─────────────────────────────────────────────────────────────────────────────

var (
	ErrNotificationNotFound = errors.New("notification not found")
	ErrWebhookNotFound      = errors.New("webhook not found")
	ErrWebhookDuplicate     = errors.New("webhook for this event type already exists")
)

// ─────────────────────────────────────────────────────────────────────────────
// CACHE
// ─────────────────────────────────────────────────────────────────────────────

const (
	notifBadgeCacheTTL    = 2 * time.Minute
	notifBadgeCachePrefix = "notif:badge:"
)

// ─────────────────────────────────────────────────────────────────────────────
// SERVICE
// ─────────────────────────────────────────────────────────────────────────────

// NotificationService manages in-app notifications and webhook subscriptions.
type NotificationService struct {
	db    *gorm.DB
	redis *goredis.Client
}

// NewNotificationService constructs a new NotificationService.
func NewNotificationService(db *gorm.DB, redis *goredis.Client) *NotificationService {
	return &NotificationService{db: db, redis: redis}
}

// ─────────────────────────────────────────────────────────────────────────────
// INPUT / OUTPUT TYPES
// ─────────────────────────────────────────────────────────────────────────────

// SendNotificationInput contains the fields for creating a notification.
type SendNotificationInput struct {
	TenantID         uuid.UUID
	UserID           *uuid.UUID
	Message          string
	NotificationType string // maps to models.NotificationType
}

// ListNotificationsFilter controls pagination and filtering.
type ListNotificationsFilter struct {
	TenantID         uuid.UUID
	UserID           *uuid.UUID
	IsRead           *bool
	NotificationType *string
	From             *time.Time
	To               *time.Time
	Page             int
	PageSize         int
}

// NotificationResponse is a read-optimised view of a notification.
type NotificationResponse struct {
	NotificationID   uuid.UUID  `json:"notification_id"`
	TenantID         uuid.UUID  `json:"tenant_id"`
	UserID           *uuid.UUID `json:"user_id,omitempty"`
	Message          string     `json:"message"`
	NotificationType string     `json:"notification_type"`
	IsRead           bool       `json:"is_read"`
	CreatedAt        time.Time  `json:"created_at"`
	TimeAgo          string     `json:"time_ago"`
}

// NotificationBadge holds unread counts for a user.
type NotificationBadge struct {
	UnreadCount int            `json:"unread_count"`
	ByType      map[string]int `json:"by_type"`
}

// CreateWebhookInput contains the fields for registering a webhook.
type CreateWebhookInput struct {
	TenantID  uuid.UUID
	URL       string
	EventType string
	Secret    string
}

// WebhookResponse is a read-optimised view of a webhook.
type WebhookResponse struct {
	WebhookID uuid.UUID `json:"webhook_id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	URL       string    `json:"url"`
	EventType string    `json:"event_type"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// SEND NOTIFICATION
// ─────────────────────────────────────────────────────────────────────────────

// Send creates and stores a new notification.
func (s *NotificationService) Send(
	ctx context.Context,
	in SendNotificationInput,
) (*models.Notification, error) {
	notif := &models.Notification{
		NotificationID:   uuid.New(),
		TenantID:         in.TenantID,
		UserID:           in.UserID,
		Message:          in.Message,
		NotificationType: in.NotificationType,
		IsRead:           false,
		CreatedAt:        time.Now(),
	}

	if err := s.db.WithContext(ctx).Omit(clause.Associations).Create(notif).Error; err != nil {
		return nil, fmt.Errorf("send notification: %w", err)
	}

	// Invalidate badge cache for the target user
	if in.UserID != nil {
		s.invalidateBadgeCache(ctx, in.TenantID, *in.UserID)
	}

	return notif, nil
}

// SendBulk sends a notification to multiple users.
func (s *NotificationService) SendBulk(
	ctx context.Context,
	tenantID uuid.UUID,
	userIDs []uuid.UUID,
	message string,
	notificationType string,
) (int, error) {
	notifications := make([]models.Notification, len(userIDs))
	now := time.Now()
	for i, uid := range userIDs {
		u := uid
		notifications[i] = models.Notification{
			NotificationID:   uuid.New(),
			TenantID:         tenantID,
			UserID:           &u,
			Message:          message,
			NotificationType: notificationType,
			IsRead:           false,
			CreatedAt:        now,
		}
	}

	result := s.db.WithContext(ctx).Create(&notifications)
	if result.Error != nil {
		return 0, fmt.Errorf("send bulk notifications: %w", result.Error)
	}

	// Invalidate badge caches
	for _, uid := range userIDs {
		s.invalidateBadgeCache(ctx, tenantID, uid)
	}

	return int(result.RowsAffected), nil
}

// SendToAll sends a tenant-wide notification (UserID = nil).
func (s *NotificationService) SendToAll(
	ctx context.Context,
	tenantID uuid.UUID,
	message string,
	notificationType string,
) (*models.Notification, error) {
	return s.Send(ctx, SendNotificationInput{
		TenantID:         tenantID,
		UserID:           nil,
		Message:          message,
		NotificationType: notificationType,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// READ NOTIFICATIONS
// ─────────────────────────────────────────────────────────────────────────────

// GetNotification returns a single notification.
func (s *NotificationService) GetNotification(
	ctx context.Context,
	tenantID uuid.UUID,
	notificationID uuid.UUID,
) (*NotificationResponse, error) {
	var notif models.Notification
	if err := s.db.WithContext(ctx).
		Where("notification_id = ? AND tenant_id = ?", notificationID, tenantID).
		First(&notif).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotificationNotFound
		}
		return nil, fmt.Errorf("get notification: %w", err)
	}
	return s.toNotifResponse(&notif), nil
}

// ListNotifications returns a paginated, filtered list of notifications.
// Includes both user-specific and tenant-wide (UserID IS NULL) notifications.
func (s *NotificationService) ListNotifications(
	ctx context.Context,
	f ListNotificationsFilter,
) ([]NotificationResponse, int64, error) {
	q := s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("tenant_id = ?", f.TenantID)

	if f.UserID != nil {
		// Include user-specific + tenant-wide
		q = q.Where("user_id = ? OR user_id IS NULL", *f.UserID)
	}
	if f.IsRead != nil {
		q = q.Where("is_read = ?", *f.IsRead)
	}
	if f.NotificationType != nil {
		q = q.Where("notification_type = ?", *f.NotificationType)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at <= ?", *f.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}

	var notifications []models.Notification
	if err := q.Order("created_at DESC").
		Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize).
		Find(&notifications).Error; err != nil {
		return nil, 0, err
	}

	results := make([]NotificationResponse, len(notifications))
	for i := range notifications {
		results[i] = *s.toNotifResponse(&notifications[i])
	}
	return results, total, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// READ / UNREAD / DELETE
// ─────────────────────────────────────────────────────────────────────────────

// MarkAsRead marks a single notification as read.
func (s *NotificationService) MarkAsRead(
	ctx context.Context,
	tenantID uuid.UUID,
	notificationID uuid.UUID,
) error {
	result := s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("notification_id = ? AND tenant_id = ?", notificationID, tenantID).
		Update("is_read", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

// MarkAllAsRead marks all notifications for a user as read.
func (s *NotificationService) MarkAllAsRead(
	ctx context.Context,
	tenantID uuid.UUID,
	userID uuid.UUID,
) (int, error) {
	result := s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("tenant_id = ? AND (user_id = ? OR user_id IS NULL) AND is_read = false",
			tenantID, userID).
		Update("is_read", true)
	if result.Error != nil {
		return 0, result.Error
	}
	s.invalidateBadgeCache(ctx, tenantID, userID)
	return int(result.RowsAffected), nil
}

// DeleteNotification removes a single notification.
func (s *NotificationService) DeleteNotification(
	ctx context.Context,
	tenantID uuid.UUID,
	notificationID uuid.UUID,
) error {
	result := s.db.WithContext(ctx).
		Where("notification_id = ? AND tenant_id = ?", notificationID, tenantID).
		Delete(&models.Notification{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

// DeleteOlderThan removes all notifications older than the given duration.
// Returns the number of deleted records.
func (s *NotificationService) DeleteOlderThan(
	ctx context.Context,
	tenantID uuid.UUID,
	olderThan time.Duration,
) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	result := s.db.WithContext(ctx).
		Where("tenant_id = ? AND created_at < ?", tenantID, cutoff).
		Delete(&models.Notification{})
	return int(result.RowsAffected), result.Error
}

// ─────────────────────────────────────────────────────────────────────────────
// BADGE / UNREAD COUNT
// ─────────────────────────────────────────────────────────────────────────────

// GetBadge returns the unread notification count for a user, using Redis
// as a read-through cache.
func (s *NotificationService) GetBadge(
	ctx context.Context,
	tenantID uuid.UUID,
	userID uuid.UUID,
) (*NotificationBadge, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("%s%s:%s", notifBadgeCachePrefix, tenantID, userID)
	if data, err := s.redis.Get(ctx, cacheKey).Bytes(); err == nil {
		var badge NotificationBadge
		if json.Unmarshal(data, &badge) == nil {
			return &badge, nil
		}
	}

	badge := &NotificationBadge{
		ByType: make(map[string]int),
	}

	// Total unread
	var total int64
	s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("tenant_id = ? AND (user_id = ? OR user_id IS NULL) AND is_read = false",
			tenantID, userID).
		Count(&total)
	badge.UnreadCount = int(total)

	// By type
	type kv struct {
		NotificationType string
		Count            int
	}
	var byType []kv
	s.db.WithContext(ctx).Model(&models.Notification{}).
		Select("notification_type, COUNT(*) AS count").
		Where("tenant_id = ? AND (user_id = ? OR user_id IS NULL) AND is_read = false",
			tenantID, userID).
		Group("notification_type").Scan(&byType)
	for _, t := range byType {
		badge.ByType[t.NotificationType] = t.Count
	}

	// Cache
	if data, err := json.Marshal(badge); err == nil {
		_ = s.redis.Set(ctx, cacheKey, data, notifBadgeCacheTTL).Err()
	}

	return badge, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// WEBHOOK MANAGEMENT
// ─────────────────────────────────────────────────────────────────────────────

// RegisterWebhook creates a new webhook subscription.
func (s *NotificationService) RegisterWebhook(
	ctx context.Context,
	in CreateWebhookInput,
) (*models.Webhook, error) {
	// Check duplicate
	var count int64
	s.db.WithContext(ctx).Model(&models.Webhook{}).
		Where("tenant_id = ? AND event_type = ? AND url = ? AND is_active = true",
			in.TenantID, in.EventType, in.URL).
		Count(&count)
	if count > 0 {
		return nil, ErrWebhookDuplicate
	}

	webhook := &models.Webhook{
		WebhookID: uuid.New(),
		TenantID:  in.TenantID,
		URL:       in.URL,
		EventType: in.EventType,
		Secret:    in.Secret,
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	if err := s.db.WithContext(ctx).Omit(clause.Associations).Create(webhook).Error; err != nil {
		return nil, fmt.Errorf("register webhook: %w", err)
	}
	return webhook, nil
}

// ListWebhooks returns all webhooks for a tenant.
func (s *NotificationService) ListWebhooks(
	ctx context.Context,
	tenantID uuid.UUID,
	activeOnly bool,
) ([]WebhookResponse, error) {
	q := s.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID)
	if activeOnly {
		q = q.Where("is_active = true")
	}

	var webhooks []models.Webhook
	if err := q.Order("created_at DESC").Find(&webhooks).Error; err != nil {
		return nil, err
	}

	results := make([]WebhookResponse, len(webhooks))
	for i, w := range webhooks {
		results[i] = WebhookResponse{
			WebhookID: w.WebhookID,
			TenantID:  w.TenantID,
			URL:       w.URL,
			EventType: w.EventType,
			IsActive:  w.IsActive,
			CreatedAt: w.CreatedAt,
		}
	}
	return results, nil
}

// GetWebhooksByEvent returns all active webhooks for a specific event type.
// Used internally by other services to fire webhooks on events.
func (s *NotificationService) GetWebhooksByEvent(
	ctx context.Context,
	tenantID uuid.UUID,
	eventType string,
) ([]models.Webhook, error) {
	var webhooks []models.Webhook
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND event_type = ? AND is_active = true",
			tenantID, eventType).
		Find(&webhooks).Error; err != nil {
		return nil, err
	}
	return webhooks, nil
}

// DeactivateWebhook marks a webhook as inactive.
func (s *NotificationService) DeactivateWebhook(
	ctx context.Context,
	tenantID uuid.UUID,
	webhookID uuid.UUID,
) error {
	result := s.db.WithContext(ctx).Model(&models.Webhook{}).
		Where("webhook_id = ? AND tenant_id = ?", webhookID, tenantID).
		Update("is_active", false)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWebhookNotFound
	}
	return nil
}

// DeleteWebhook permanently removes a webhook.
func (s *NotificationService) DeleteWebhook(
	ctx context.Context,
	tenantID uuid.UUID,
	webhookID uuid.UUID,
) error {
	result := s.db.WithContext(ctx).
		Where("webhook_id = ? AND tenant_id = ?", webhookID, tenantID).
		Delete(&models.Webhook{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWebhookNotFound
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────

func (s *NotificationService) invalidateBadgeCache(ctx context.Context, tenantID, userID uuid.UUID) {
	_ = s.redis.Del(ctx, fmt.Sprintf("%s%s:%s", notifBadgeCachePrefix, tenantID, userID)).Err()
}

func (s *NotificationService) toNotifResponse(n *models.Notification) *NotificationResponse {
	resp := &NotificationResponse{
		NotificationID:   n.NotificationID,
		TenantID:         n.TenantID,
		UserID:           n.UserID,
		Message:          n.Message,
		NotificationType: n.NotificationType,
		IsRead:           n.IsRead,
		CreatedAt:        n.CreatedAt,
		TimeAgo:          s.computeTimeAgo(n.CreatedAt),
	}
	return resp
}

func (s *NotificationService) computeTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		weeks := int(d.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// EVENT HELPERS (for use by other services)
// ─────────────────────────────────────────────────────────────────────────────

// NotifyLowStock sends a low-stock alert notification.
func (s *NotificationService) NotifyLowStock(
	ctx context.Context,
	tenantID uuid.UUID,
	userID *uuid.UUID,
	itemName string,
	currentQty float64,
) error {
	message := fmt.Sprintf("Low stock alert: %s is at %.1f units", itemName, currentQty)
	_, err := s.Send(ctx, SendNotificationInput{
		TenantID:         tenantID,
		UserID:           userID,
		Message:          message,
		NotificationType: string(models.NotificationTypeWarning),
	})
	return err
}

// NotifyOrderReady sends an order-ready notification.
func (s *NotificationService) NotifyOrderReady(
	ctx context.Context,
	tenantID uuid.UUID,
	userID *uuid.UUID,
	orderNumber string,
) error {
	message := fmt.Sprintf("Order %s is ready for pickup", orderNumber)
	_, err := s.Send(ctx, SendNotificationInput{
		TenantID:         tenantID,
		UserID:           userID,
		Message:          message,
		NotificationType: string(models.NotificationTypeSuccess),
	})
	return err
}

// NotifySecurityAlert sends a security-related notification.
func (s *NotificationService) NotifySecurityAlert(
	ctx context.Context,
	tenantID uuid.UUID,
	userID *uuid.UUID,
	alertMessage string,
) error {
	_, err := s.Send(ctx, SendNotificationInput{
		TenantID:         tenantID,
		UserID:           userID,
		Message:          alertMessage,
		NotificationType: string(models.NotificationTypeAlert),
	})
	return err
}

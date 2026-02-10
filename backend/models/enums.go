package models

type TableStatus string

const (
	TableStatusAvailable     TableStatus = "available"
	TableStatusOccupied      TableStatus = "occupied"
	TableStatusReserved      TableStatus = "reserved"
	TableStatusCleaning      TableStatus = "cleaning"
	TableStatusOutOfServices TableStatus = "out_of_services"
)

type AuditEvent string

const (
	AuditEventCreate            AuditEvent = "create"
	AuditEventRead              AuditEvent = "read"
	AuditEventUpdate            AuditEvent = "update"
	AuditEventDelete            AuditEvent = "delete"
	AuditEventLogin             AuditEvent = "login"
	AuditEventLogout            AuditEvent = "logout"
	AuditEventPasswordChange    AuditEvent = "password_change"
	AuditEventPermissionsChange AuditEvent = "permissions_change"
	AuditEventPaymentProcessed  AuditEvent = "payment_processed"
	AuditEventDataExport        AuditEvent = "data_export"
	AuditEventConfigChange      AuditEvent = "config_change"
	AuditEventSecurityEvent     AuditEvent = "security_event"
)

type AuditSeverity string

const (
	AuditSeverityInfo     AuditSeverity = "info"
	AuditSeverityWarning  AuditSeverity = "warning"
	AuditSeverityError    AuditSeverity = "error"
	AuditSeverityCritical AuditSeverity = "critical"
)

type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelNone     RiskLevel = "none"
	RiskLevelCritical RiskLevel = "critical"
)

type OrderItemStatus string

const (
	OrderItemStatusPending   OrderItemStatus = "pending"
	OrderItemStatusComplete  OrderItemStatus = "complete"
	OrderItemStatusPreparing OrderItemStatus = "preparing"
	OrderItemStatusServed    OrderItemStatus = "served"
	OrderItemStatusCancelled OrderItemStatus = "cancelled"
)

type OrderType string

const (
	OrderTypeDineIn   OrderType = "dine-in"
	OrderTypeTakeaway OrderType = "takeaway"
	OrderTypeDelivery OrderType = "delivery"
	OrderTypeOnline   OrderType = "online"
)

type KOTStatus string

const (
	KOTStatusSent    KOTStatus = "sent"
	KOTStatusInProgress KOTStatus = "in_progress"
	KOTStatusCompleted KOTStatus = "completed"
	KOTStatusCancelled KOTStatus = "cancelled"
)

type KOTPriority string

const (
	KKOTPriorityLow    KOTPriority = "low"
	KOTPriorityMedium KOTPriority = "medium"
	KOTPriorityHigh   KOTPriority = "high"
	KOTPriorityUrgent KOTPriority = "urgent"
)

type KOTItemStatus string

const (
	KOTItemStatusPending   KOTItemStatus = "pending"
	KOTItemStatusPreparing KOTItemStatus = "preparing"
	KOTItemStatusReady     KOTItemStatus = "ready"
	KOTItemStatusServed    KOTItemStatus = "served"
	KOTItemStatusCancelled KOTItemStatus = "cancelled"
)

type KitchenStation string

const (
	KitchenStationGeneral   KitchenStation = "general"
	KitchenStationGrill     KitchenStation = "grill"
	KitchenStationFryer     KitchenStation = "fryer"
	KitchenStationSalad    KitchenStation = "salad"
	KitchenStationDessert  KitchenStation = "dessert"
	KitchenStationBeverage KitchenStation = "beverage"
)

type RuleType string

const (
	RuleTypeDiscount       RuleType = "Discount"
	RuleTypeKdsRouting     RuleType = "KdsRouting"
	RuleTypeNotification   RuleType = "Notification"
	RuleTypeAutoCloseTable RuleType = "AutoCloseTable"
)

type NotificationType string

const (
	NotificationTypeInfo    NotificationType = "Info"
	NotificationTypeWarning NotificationType = "Warning"
	NotificationTypeError   NotificationType = "Error"
	NotificationTypeSuccess NotificationType = "Success"
	NotificationTypeAlert   NotificationType = "Alert"
)

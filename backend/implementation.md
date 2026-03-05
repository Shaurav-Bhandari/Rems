# Implementation Reference

This document describes the service implementations added to the ReMS backend, including FSM integrations (inventory, analytics, forecast), people services (employee, customer), and support services (audit, notification). All changes compile cleanly and require no database schema modifications.

---

## Table of Contents

1. [Files Modified](#files-modified)
2. [Inventory Item FSM](#inventory-item-fsm)
3. [Inventory Service Integration](#inventory-service-integration)
4. [Analytics Pipeline FSM and Service](#analytics-pipeline-fsm-and-service)
5. [Forecast Pipeline FSM and Service](#forecast-pipeline-fsm-and-service)
6. [Employee Service](#employee-service)
7. [Customer Service](#customer-service)
8. [Audit Service](#audit-service)
9. [Notification Service](#notification-service)
10. [Usage Patterns](#usage-patterns)
11. [Design Decisions](#design-decisions)

---

## Files Modified

| File | Change Type | Lines Added |
|------|-------------|-------------|
| `backend/utils/FSM.go` | Modified | ~530 |
| `backend/services/inventory/inventory.go` | Modified | ~80 |
| `backend/services/analytics/analytics.go` | Rewritten | ~620 |
| `backend/services/analytics/Forecast.go` | Rewritten | ~520 |
| `backend/services/people/Employee.go` | Rewritten | ~470 |
| `backend/services/people/Customer.go` | Rewritten | ~490 |
| `backend/services/support/Audit.go` | Rewritten | ~610 |
| `backend/services/support/Notification.go` | Rewritten | ~530 |

---

## Inventory Item FSM

**Location**: `backend/utils/FSM.go` (appended after `PaymentFSM`)

### States

| State | Type | Description |
|-------|------|-------------|
| `active` | Initial | Item is in stock above its reorder point |
| `low_stock` |  | Stock is at or below the reorder point but above zero |
| `out_of_stock` |  | No stock remaining (quantity is zero or negative) |
| `discontinued` | Terminal | Item permanently removed from active inventory |

### Events

| Event | Description |
|-------|-------------|
| `stock_low` | Quantity dropped to or below reorder point |
| `stock_depleted` | Quantity reached zero |
| `stock_replenished` | Quantity restored above reorder point |
| `discontinue` | Item taken out of service |
| `reactivate` | Reserved for future use |

### Transition Table

| From | Event | To | Guard |
|------|-------|----|-------|
| `active` | `stock_low` | `low_stock` | qty > 0 AND qty <= reorder_point |
| `active` | `stock_depleted` | `out_of_stock` | qty <= 0 |
| `low_stock` | `stock_depleted` | `out_of_stock` | qty <= 0 |
| `low_stock` | `stock_replenished` | `active` | qty > reorder_point |
| `out_of_stock` | `stock_replenished` | `active` | qty > 0 AND qty > reorder_point |
| `out_of_stock` | `stock_low` | `low_stock` | qty > 0 AND qty <= reorder_point |
| `active` | `discontinue` | `discontinued` | none |
| `low_stock` | `discontinue` | `discontinued` | none |
| `out_of_stock` | `discontinue` | `discontinued` | none |

### Helper Function

```go
func DeriveInventoryState(currentQty float64, reorderPoint *float64) InventoryStockState
```

Computes the correct FSM state from an item's current quantity and reorder point. Used at restore time since the state is not persisted in the database.

---

## Inventory Service Integration

**Location**: `backend/services/inventory/inventory.go`

### Context Adapter

The `inventoryItemAdapter` struct wraps `*models.InventoryItem` to satisfy the `utils.InventoryItemContext` interface:

```go
type inventoryItemAdapter struct {
    item        *models.InventoryItem
    stockStatus string
}
```

It exposes `GetItemID()`, `GetCurrentQuantity()`, `GetReorderPoint()`, `SetStockStatus()`, and `GetStockStatus()` -- all delegating to the underlying model fields.

### Transition Evaluation

The `evaluateStockTransition()` method is the central integration point. After any quantity mutation, it:

1. Derives the target state using `utils.DeriveInventoryState()`.
2. Determines the FSM event needed to reach that state.
3. Attempts the transition from each plausible source state (since the previous state is not persisted).
4. On successful transition into `low_stock` or `out_of_stock`, fires the existing `checkLowStock()` notification asynchronously.

This method is idempotent -- if the item is already in the correct state, all transition attempts fail silently and no notification is sent.

### Integration Points

The FSM is wired into three methods:

- **`CreateItem()`** -- evaluates stock state immediately after inserting the new item.
- **`AdjustStock()`** -- replaces the previous ad-hoc `go s.checkLowStock()` call with `evaluateStockTransition()`.
- **`DeductForOrder()`** -- same replacement, ensuring FSM-driven evaluation after each per-item deduction.

---

## Analytics Pipeline FSM and Service

### Pipeline FSM

**Location**: `backend/utils/FSM.go`

The `AnalyticsPipelineFSM` defines a six-stage linear pipeline:

```
idle -> collecting -> aggregating -> enriching -> delivering -> completed
                                                                   |
  (any in-flight state) ------- fail --------> failed (terminal)
```

The `collecting -> aggregating` transition has a guard requiring at least one collected record (`GetRecordCount() > 0`).

### Service

**Location**: `backend/services/analytics/analytics.go`

The `AnalyticsService` provides nine public methods, each executing the full FSM pipeline:

**Revenue**:
- `GetRevenueOverview()` -- total revenue, order count, average order value, tax, discount, net revenue, period-over-period delta.
- `GetRevenueTrend()` -- daily revenue time series.
- `GetRevenueByCategory()` -- revenue breakdown by menu category with percentage shares.

**Orders**:
- `GetOrderVolume()` -- daily order count time series.
- `GetOrderStatusDistribution()` -- count and percentage per order status.
- `GetPeakHours()` -- order and revenue concentration per hour of day.

**Menu Performance**:
- `GetTopSellingItems()` -- top N items by quantity sold with average price and rank.

**Inventory**:
- `GetInventoryTurnover()` -- turnover rate and days-of-supply per item.
- `GetWastageReport()` -- damage and expiry quantities with cost valuation.

Each method follows the same pattern:
1. Check Redis cache (5-minute TTL) for a precomputed result.
2. Create an `analyticsPipelineRun` adapter.
3. Define four `pipelineStage` closures (collected, aggregated, enriched, delivered).
4. Call `runPipeline()` which drives the FSM through each transition.
5. Cache and return the result.

---

## Forecast Pipeline FSM and Service

### Pipeline FSM

**Location**: `backend/utils/FSM.go`

The `ForecastPipelineFSM` defines an eight-stage pipeline:

```
idle -> ingesting -> feature_engineering -> model_training -> scoring
  -> post_processing -> publishing -> completed
```

Guards:
- `ingesting -> feature_engineering`: requires at least 7 data points.
- `model_training -> scoring`: requires non-negative model accuracy.

### Service

**Location**: `backend/services/analytics/Forecast.go`

The `ForecastService` provides three public methods:

**`ForecastDemand()`** -- per-menu-item daily demand prediction.
- Uses Holt-Winters exponential smoothing with additive day-of-week seasonality.
- Smoothing parameters: alpha=0.3 (level), beta=0.1 (trend), gamma=0.2 (seasonal).
- Confidence intervals computed from residual standard deviation scaled by forecast horizon.
- Output sorted by historical average demand (highest first).

**`ForecastRevenue()`** -- total daily revenue prediction.
- Same Holt-Winters model applied to aggregate daily revenue.
- Includes trend direction (up/down/flat) and trend strength (percent change per day).

**`ForecastReorderDates()`** -- predicts when each inventory item hits its reorder point.
- Uses a linear consumption model: days_until_reorder = (current_stock - reorder_point) / daily_consumption.
- Assigns urgency levels: `critical` (<=3 days), `warning` (<=7 days), `normal` (>7 days).
- Output sorted by urgency (most urgent first).

### Internal Model

The `forecastModel` struct encapsulates the Holt-Winters fitting and prediction logic:

- `fit()` -- initializes seasonal indices from the first week of data, then iteratively updates level, trend, and seasonal components. Computes residual standard deviation for confidence intervals.
- `predict()` -- generates point forecasts with configurable confidence level (80%, 90%, 95%, 99%). Applies a floor constraint of zero and widens intervals proportionally to the square root of steps ahead.
- `accuracy()` -- returns R-squared of the fitted model.

---

## Employee Service

**Location**: `backend/services/people/Employee.go`

`EmployeeService` manages the full employee lifecycle -- CRUD, termination/reactivation, role assignment/revocation, and aggregate statistics. All operations are scoped to a tenant + restaurant.

### Public Methods

| Method | Description |
|--------|-------------|
| `CreateEmployee()` | Create employee with duplicate email check |
| `GetEmployee()` | Single employee by ID with roles preloaded |
| `ListEmployees()` | Paginated list with department/position/search filtering |
| `UpdateEmployee()` | Partial update on mutable fields |
| `TerminateEmployee()` | Set `is_active = false`, record `termination_date` |
| `ReactivateEmployee()` | Restore a terminated employee |
| `AssignRole()` | Add a role to an employee (with duplicate check) |
| `RevokeRole()` | Remove a role from an employee |
| `GetEmployeeStats()` | Aggregate counts by department, position, avg tenure |
| `GetEmployeesByDepartment()` | All active employees in a given department |

All write operations produce audit trail entries via `writeAudit()`.

---

## Customer Service

**Location**: `backend/services/people/Customer.go`

`CustomerService` manages customer records and the loyalty program. Operations are scoped to a tenant.

### Public Methods

| Method | Description |
|--------|-------------|
| `CreateCustomer()` | Create customer with email/phone dedup |
| `GetCustomer()` | Single customer by ID |
| `ListCustomers()` | Paginated list with segment/search filtering |
| `SearchCustomers()` | Quick name/email/phone search (top N) |
| `UpdateCustomer()` | Partial update on mutable fields |
| `DeactivateCustomer()` | Soft-deactivate |
| `AwardPoints()` | Add loyalty points with reason |
| `RedeemPoints()` | Deduct loyalty points (with balance check) |
| `RecordOrderForCustomer()` | Increment total_orders and total_spent |
| `GetCustomerStats()` | Aggregate metrics: revenue, LTV, VIP/at-risk counts |

### Customer Segmentation

Customers are classified into segments based on order history:

| Segment | Criteria |
|---------|----------|
| `new` | Created within the last 30 days |
| `vip` | 10+ orders AND $500+ total spent |
| `at_risk` | 2 or fewer orders (after new period) |
| `regular` | Everyone else |

---

## Audit Service

**Location**: `backend/services/support/Audit.go`

`AuditService` provides comprehensive audit trail management, compliance reporting, anomaly querying, and security snapshots. All operations are tenant-scoped.

### Public Methods

| Method | Description |
|--------|-------------|
| `RecordAuditEntry()` | Create audit entry (auto-flags PCI/GDPR/risk) |
| `RecordAnomaly()` | Record a detected anomaly |
| `GetAuditEntry()` | Single entry by ID with user preloaded |
| `ListAuditTrail()` | Paginated list with 14+ filter dimensions |
| `GetAuditHistory()` | All entries for a specific entity |
| `GetPendingReviews()` | Entries requiring review, newest first |
| `MarkReviewed()` | Mark an entry as reviewed |
| `GetAuditStats()` | Breakdown by severity, event type, risk level |
| `GenerateComplianceReport()` | PCI/GDPR/risk summary with compliance score |
| `GetSecuritySnapshot()` | Login/password/risk/anomaly summary with hourly breakdown |
| `GetAnomalies()` | Recent anomaly records |
| `GetUserActivity()` | Audit entries for a specific user |

### Auto-Flagging Rules

When `RecordAuditEntry()` is called, the following flags are set automatically:

- **`requires_review`**: Set when `risk_level` is `high` or `critical`, or `severity` is `critical`.
- **`is_pci_relevant`**: Set when `event_type` is `payment_processed`.
- **`is_gdpr_relevant`**: Set when `event_type` is `data_export`, or when `entity_type` is `User` with a `delete` or `update` event.

### Compliance Score

Computed as `100 - penalty`, where penalties are weighted:
- Unreviewed ratio x 30 (max 30pt penalty)
- Critical event ratio x 20 (max 20pt penalty)
- Anomaly ratio x 20 (max 20pt penalty)

---

## Notification Service

**Location**: `backend/services/support/Notification.go`

`NotificationService` manages in-app notifications and webhook subscriptions. Notifications are tenant-scoped. Badge counts are cached in Redis with a 2-minute TTL.

### Public Methods -- Notifications

| Method | Description |
|--------|-------------|
| `Send()` | Create a single notification |
| `SendBulk()` | Send to multiple users at once |
| `SendToAll()` | Tenant-wide notification (UserID = nil) |
| `GetNotification()` | Single notification by ID |
| `ListNotifications()` | Paginated list with type/read/date filters |
| `MarkAsRead()` | Mark one notification as read |
| `MarkAllAsRead()` | Mark all for a user as read |
| `DeleteNotification()` | Remove a single notification |
| `DeleteOlderThan()` | Bulk cleanup by age |
| `GetBadge()` | Unread count + breakdown by type (Redis-cached) |

### Public Methods -- Webhooks

| Method | Description |
|--------|-------------|
| `RegisterWebhook()` | Register a webhook with dedup check |
| `ListWebhooks()` | All webhooks for a tenant |
| `GetWebhooksByEvent()` | Active webhooks for a specific event type |
| `DeactivateWebhook()` | Soft-disable a webhook |
| `DeleteWebhook()` | Permanently remove a webhook |

### Domain Helpers

Convenience methods for common notification scenarios:
- `NotifyLowStock()` -- sends a warning notification for low inventory.
- `NotifyOrderReady()` -- sends a success notification when an order is ready.
- `NotifySecurityAlert()` -- sends an alert notification for security events.

---

## Usage Patterns

### Restoring an Inventory FSM Machine

```go
adapter := &inventoryItemAdapter{item: &item, stockStatus: string(currentState)}
machine := utils.InventoryItemFSM.Restore(adapter, currentState)
env := utils.NewEnvelope(utils.InventoryEventStockLow, nil)
err := machine.Send(ctx, env)
```

### Running an Analytics Pipeline

```go
svc := analytics.NewAnalyticsService(db, redis)
overview, err := svc.GetRevenueOverview(ctx, analytics.AnalyticsRequest{
    TenantID:     tenantID,
    RestaurantID: restaurantID,
    From:         time.Now().AddDate(0, 0, -30),
    To:           time.Now(),
})
```

### Running a Forecast Pipeline

```go
svc := analytics.NewForecastService(db, redis)
demand, err := svc.ForecastDemand(ctx, analytics.ForecastRequest{
    TenantID:     tenantID,
    RestaurantID: restaurantID,
    LookbackDays: 30,
    HorizonDays:  7,
})
```

### Using the Employee Service

```go
svc := people.NewEmployeeService(db, redis)
emp, err := svc.CreateEmployee(ctx, people.CreateEmployeeInput{
    EmployeeServiceRequest: people.EmployeeServiceRequest{
        TenantID: tenantID, RestaurantID: restaurantID, ActorID: actorID,
    },
    FirstName: "Jane", LastName: "Doe", Position: "Server", Department: "Front of House",
})
```

### Using the Notification Service

```go
svc := support.NewNotificationService(db, redis)
_ = svc.NotifyLowStock(ctx, tenantID, &managerUserID, "Olive Oil", 2.5)
badge, _ := svc.GetBadge(ctx, tenantID, managerUserID)
```

---

## Design Decisions

**No schema migration for inventory state.** The FSM state is derived at runtime from `CurrentQuantity` and `ReorderPoint` using `DeriveInventoryState()`. This avoids adding a column and an associated migration while keeping the FSM fully functional. The tradeoff is that the previous state is unknown at restore time, requiring the `evaluateStockTransition()` method to try all plausible source states.

**Pipeline FSMs as orchestration, not persistence.** The analytics and forecast pipeline FSMs are ephemeral -- each request creates a new machine, drives it to completion, and discards it. The FSM enforces stage ordering, guards prevent invalid transitions (such as aggregating zero records), and effects track timing metadata. This is intentionally over-engineered to demonstrate the FSM framework's applicability to data pipelines.

**Holt-Winters over ARIMA.** Exponential smoothing with additive seasonality was chosen over more complex models because it works well with limited data (as few as 7 data points), has no external library dependencies, and produces reasonable forecasts for the typical restaurant demand pattern (strong day-of-week seasonality, gentle trend).

**Cache-first analytics.** All analytics queries check Redis before hitting the database. The 5-minute TTL balances freshness against load. The cache is keyed on tenant, restaurant, and time range so concurrent dashboards do not interfere.

**Computed customer segments.** Segments (vip, regular, at_risk, new) are derived at read time from `total_orders`, `total_spent`, and `created_at` rather than stored as a column. This avoids stale classifications and removes the need for batch segment-recomputation jobs. The tradeoff is that segment-based filtering on large customer tables requires a full scan; this can be optimized with a materialized view if needed.

**Auto-flagging in audit entries.** PCI, GDPR, risk, and review flags are set automatically in `RecordAuditEntry()` based on event type and severity. This ensures consistent compliance tagging without requiring each calling service to manually set flags.

**Redis-cached notification badges.** Badge counts are cached in Redis with a 2-minute TTL to avoid hitting the database on every page load. The cache is invalidated on send and mark-all-as-read operations.


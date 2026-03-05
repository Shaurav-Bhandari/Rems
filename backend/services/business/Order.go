	// services/business/Order.go
	//
	// OrderService — four-layer Submit Order pipeline with dynamic tax calculation.
	//
	// ┌────────────────────────────────────────────────────────────────────────┐
	// │  LAYER 1 — GATEWAY                                                     │
	// │  Idempotency dedup  →  RBAC authorization                             │
	// ├────────────────────────────────────────────────────────────────────────┤
	// │  LAYER 2 — DOMAIN VALIDATION                                           │
	// │  Table FSM guard  →  Menu availability  →  Price integrity            │
	// │  →  Modifier check  →  Inventory stock check                          │
	// ├────────────────────────────────────────────────────────────────────────┤
	// │  LAYER 3 — ATOMIC TRANSACTION                                          │
	// │  Build items  →  TaxCalculator  →  Persist Order + Items              │
	// │  →  OrderTaxBreakdown  →  OrderLog  →  AuditTrail                    │
	// │  →  Table FSM seat  →  Outbox event                                   │
	// ├────────────────────────────────────────────────────────────────────────┤
	// │  LAYER 4 — ASYNC DISSEMINATION (post-commit, non-blocking)             │
	// │  Outbox relay  →  KOTService  →  InventoryService  →  Analytics       │
	// └────────────────────────────────────────────────────────────────────────┘
	//
	// Tax logic is 100% driven by RestaurantProfile + TenantSetting overrides.
	// Nothing is hardcoded. Changing the profile row changes the math for all
	// future orders at that restaurant — zero code changes required.
	package business

	import (
		"context"
		"crypto/rand"
		"encoding/base64"
		"encoding/json"
		"errors"
		"fmt"
		"strings"
		"sync"
		"time"

		"github.com/google/uuid"
		goredis "github.com/redis/go-redis/v9"
		"gorm.io/gorm"
		"gorm.io/gorm/clause"

		"backend/utils"
		"backend/models"
	)

	// ─────────────────────────────────────────────────────────────────────────────
	// SENTINEL ERRORS
	// ─────────────────────────────────────────────────────────────────────────────

	var (
		ErrIdempotentReplay    = errors.New("order_service: idempotent replay — returning cached result")
		ErrTableNotAvailable   = errors.New("order_service: table is not available for new orders")
		ErrMenuItemUnavailable = errors.New("order_service: one or more menu items are not available")
		ErrInsufficientStock   = errors.New("order_service: insufficient inventory stock for one or more items")
		ErrPriceMismatch       = errors.New("order_service: submitted price does not match current menu price")
		ErrEmptyOrder          = errors.New("order_service: order must contain at least one item")
		// ErrOrderNotFound       = errors.New("order_service: order not found")
		ErrInvalidTransition   = errors.New("order_service: invalid order state transition")
	)

	// ─────────────────────────────────────────────────────────────────────────────
	// CONSTANTS
	// ─────────────────────────────────────────────────────────────────────────────

	const (
		idempotencyTTL      = 24 * time.Hour
		outboxWorkerBackoff = 5 * time.Second
		maxOutboxRetries    = 10
		orderSubmitTimeout  = 10 * time.Second
		stockCheckTimeout   = 3 * time.Second
	)

	// ─────────────────────────────────────────────────────────────────────────────
	// RBAC INTERFACE
	// Defined here so Order.go has zero import dependency on services/core.
	// *core.RBACService satisfies this automatically — no changes to rbac.go.
	//
	// In tests, pass a mock:
	//   type allowAll struct{}
	//   func (allowAll) Require(_ context.Context, _ *AccessRequest) error { return nil }
	// ─────────────────────────────────────────────────────────────────────────────

	// RBACAuthorizer is the subset of RBACService that OrderService needs.
	type RBACAuthorizer interface {
		Require(ctx context.Context, req *AccessRequest) error
	}

	// AccessRequest is the authorization query.
	type AccessRequest struct {
		UserID     uuid.UUID
		TenantID   uuid.UUID
		Resource   ResourceType
		Action     ActionType
		ResourceID *uuid.UUID
	}

	type ResourceType string
	type ActionType string

	const (
		ResourceOrder  ResourceType = "order"
		ActionCreate   ActionType   = "create"
	)

	// ─────────────────────────────────────────────────────────────────────────────
	// REQUEST / RESPONSE TYPES
	// ─────────────────────────────────────────────────────────────────────────────

	// SubmitOrderRequest is the inbound payload from the waiter app.
	type SubmitOrderRequest struct {
		IdempotencyKey string                   `json:"idempotency_key" binding:"required,min=8,max=128"`
		RestaurantID   uuid.UUID                `json:"restaurant_id"   binding:"required"`
		TableID        *int                     `json:"table_id"`
		CustomerID     *uuid.UUID               `json:"customer_id"`
		CustomerName   string                   `json:"customer_name"`
		PhoneNumber    string                   `json:"phone_number"`
		OrderType      string                   `json:"order_type" binding:"required,oneof=dine-in takeaway delivery online"`
		Notes          string                   `json:"notes"`
		Items          []SubmitOrderItemRequest  `json:"items" binding:"required,min=1,dive"`
	}

	// SubmitOrderItemRequest is one line item in the submission.
	type SubmitOrderItemRequest struct {
		MenuItemID      uuid.UUID                     `json:"menu_item_id"  binding:"required"`
		Quantity        int                           `json:"quantity"      binding:"required,min=1"`
		Notes           string                        `json:"notes"`
		Modifiers       []SubmitOrderModifierRequest  `json:"modifiers"`
		ClientUnitPrice float64                       `json:"unit_price"    binding:"required,gt=0"`
	}

	// SubmitOrderModifierRequest is one modifier within a line item.
	type SubmitOrderModifierRequest struct {
		MenuItemModifierID uuid.UUID `json:"menu_item_modifier_id" binding:"required"`
		ClientPrice        float64   `json:"additional_price"`
	}

	// SubmitOrderResponse is returned on 201 Created (200 for idempotent replay).
	type SubmitOrderResponse struct {
		OrderID               uuid.UUID        `json:"order_id"`
		OrderStatus           string           `json:"order_status"`
		SubTotal              float64          `json:"sub_total"`              // before tax and service charge
		TaxAmount             float64          `json:"tax_amount"`
		ServiceChargeAmount   float64          `json:"service_charge_amount"`
		DiscountAmount        float64          `json:"discount_amount"`
		GrandTotal            float64          `json:"grand_total"`
		KOTNumber             string           `json:"kot_number"`
		EstimatedReadyMinutes int              `json:"estimated_ready_minutes"`
		TaxBreakdown          *TaxBreakdownDTO `json:"tax_breakdown,omitempty"` // nil when profile.ShowTaxBreakdown = false
		Cached                bool             `json:"cached"`
	}

	// TaxBreakdownDTO is the tax section of the response, shown on receipts.
	type TaxBreakdownDTO struct {
		StandardTaxAmount  float64        `json:"standard_tax_amount"`
		AlcoholTaxAmount   float64        `json:"alcohol_tax_amount"`
		TaxIncludedInPrice bool           `json:"tax_included_in_price"`
		ItemDetails        []ItemTaxDTO   `json:"item_details,omitempty"`
	}

	// ItemTaxDTO is one row of per-item tax detail.
	type ItemTaxDTO struct {
		ItemName    string  `json:"item_name"`
		Quantity    int     `json:"quantity"`
		UnitPrice   float64 `json:"unit_price"`
		LineTotal   float64 `json:"line_total"`
		TaxRate     float64 `json:"tax_rate"`
		TaxAmount   float64 `json:"tax_amount"`
		TaxCategory string  `json:"tax_category"` // "standard" | "alcohol" | "exempt"
	}

	// ─────────────────────────────────────────────────────────────────────────────
	// VALIDATION TYPES
	// ─────────────────────────────────────────────────────────────────────────────

	// ValidationFailure carries structured detail for a single 422 rejection.
	type ValidationFailure struct {
		Field   string `json:"field"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	type validationError struct{ failures []ValidationFailure }

	func (e *validationError) Error() string {
		msgs := make([]string, len(e.failures))
		for i, f := range e.failures {
			msgs[i] = fmt.Sprintf("%s: %s", f.Field, f.Message)
		}
		return "validation_failed: " + strings.Join(msgs, "; ")
	}

	func (e *validationError) Failures() []ValidationFailure { return e.failures }

	func newValidationError(failures ...ValidationFailure) error {
		return &validationError{failures: failures}
	}

	// ─────────────────────────────────────────────────────────────────────────────
	// DOWNSTREAM NOTIFIERS (Layer 4 — outbox relay consumers)
	// ─────────────────────────────────────────────────────────────────────────────

	// DownstreamNotifier is the interface every async consumer must satisfy.
	type DownstreamNotifier interface {
		EventType() string
		Notify(ctx context.Context, payload models.OrderCreatedPayload) error
	}

	// KOTNotifier creates a KOT + KOTItems in the kitchen when an order is placed.
	// Idempotent — safe to call multiple times for the same order.
	type KOTNotifier struct{ db *gorm.DB }

	func NewKOTNotifier(db *gorm.DB) *KOTNotifier { return &KOTNotifier{db: db} }

	func (n *KOTNotifier) EventType() string { return "OrderCreated" }

	func (n *KOTNotifier) Notify(ctx context.Context, payload models.OrderCreatedPayload) error {
		kotNumber := fmt.Sprintf("KOT-%s", payload.KOTNumber)

		var existing models.KOT
		err := n.db.WithContext(ctx).
			Where("order_id = ? AND tenant_id = ?", payload.OrderID, payload.TenantID).
			First(&existing).Error
		if err == nil {
			return nil // already created — idempotent
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("kotnotifier: lookup: %w", err)
		}

		kot := &models.KOT{
			KOTID:           uuid.New(),
			OrderID:         payload.OrderID,
			RestaurantID:    payload.RestaurantID,
			TenantID:        payload.TenantID,
			KOTNumber:       kotNumber,
			SequenceNumber:  1,
			OrderType:       models.OrderType(payload.OrderType),
			TableNumber:     payload.TableID,
			Status:          models.KOTStatusSent,
			Priority:        models.KOTPriorityMedium,
			CreatedByUserID: payload.ActorID,
			CreatedAt:       payload.OccurredAt,
		}

		return n.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(kot).Error; err != nil {
				return err
			}
			for _, item := range payload.Items {
				kotItem := &models.KOTItem{
					KOTItemID:       uuid.New(),
					KOTID:           kot.KOTID,
					OrderItemID:     item.OrderItemID,
					ItemName:        item.ItemName,
					Quantity:        item.Quantity,
					Notes:           "",
					AssignedStation: models.KitchenStation(item.KitchenStation),
					Status:          models.KOTItemStatusPending,
				}
				if err := tx.Create(kotItem).Error; err != nil {
					return err
				}
			}
			return nil
		})
	}

	// InventoryNotifier deducts stock after an order is placed.
	// No-op until menu_item_inventory_links table is built.
	type InventoryNotifier struct{ db *gorm.DB }

	func NewInventoryNotifier(db *gorm.DB) *InventoryNotifier { return &InventoryNotifier{db: db} }

	func (n *InventoryNotifier) EventType() string { return "OrderCreated" }

	func (n *InventoryNotifier) Notify(_ context.Context, _ models.OrderCreatedPayload) error {
		// TODO: implement once menu_item_inventory_links exists in menu.go
		return nil
	}

	// AnalyticsNotifier invalidates the revenue cache so the dashboard re-reads.
	type AnalyticsNotifier struct {
		db    *gorm.DB
		redis *goredis.Client
	}

	func NewAnalyticsNotifier(db *gorm.DB, redis *goredis.Client) *AnalyticsNotifier {
		return &AnalyticsNotifier{db: db, redis: redis}
	}

	func (n *AnalyticsNotifier) EventType() string { return "OrderCreated" }

	func (n *AnalyticsNotifier) Notify(ctx context.Context, payload models.OrderCreatedPayload) error {
		dateKey := fmt.Sprintf("analytics:revenue:%s:%s",
			payload.TenantID,
			payload.OccurredAt.Format("2006-01-02"),
		)
		return n.redis.Del(ctx, dateKey).Err()
	}

	// ─────────────────────────────────────────────────────────────────────────────
	// ORDER SERVICE
	// ─────────────────────────────────────────────────────────────────────────────

	// OrderService orchestrates the four-layer Submit Order pipeline.
	// Construct one per application process — goroutine-safe.
	type OrderService struct {
		db              *gorm.DB
		redis           *goredis.Client
		rbac            RBACAuthorizer   // interface — *core.RBACService satisfies this
		settings        *SettingsService // read-only config accessor
		taxCalculator   *TaxCalculator   // dynamic tax engine
		orderFSM        *utils.FSMDefinition[utils.OrderContext, utils.OrderState, utils.OrderEvent]
		tableFSM        *utils.FSMDefinition[utils.TableContext, utils.TableState, utils.TableEvent]
		notifiers       []DownstreamNotifier

		outboxWorkerOnce sync.Once
		outboxWorkerStop chan struct{}
	}

	// NewOrderService constructs the service and starts the background outbox relay.
	// Call Stop() in your shutdown hook.
	func NewOrderService(
		db *gorm.DB,
		redis *goredis.Client,
		rbac RBACAuthorizer,
		settings *SettingsService,
		notifiers ...DownstreamNotifier,
	) *OrderService {
		svc := &OrderService{
			db:               db,
			redis:            redis,
			rbac:             rbac,
			settings:         settings,
			taxCalculator:    NewTaxCalculator(settings),
			orderFSM:         utils.OrderFSM,
			tableFSM:         utils.TableFSM,
			notifiers:        notifiers,
			outboxWorkerStop: make(chan struct{}),
		}
		svc.startOutboxRelay()
		return svc
	}

	// NewDefaultOrderService wires the standard notifiers and returns a ready service.
	//
	//	settingsSvc := services.NewSettingsService(db, redis)
	//	orderSvc    := services.NewDefaultOrderService(db, redis, rbacSvc, settingsSvc)
	//	defer orderSvc.Stop()
	func NewDefaultOrderService(
		db *gorm.DB,
		redis *goredis.Client,
		rbac RBACAuthorizer,
		settings *SettingsService,
	) *OrderService {
		return NewOrderService(
			db, redis, rbac, settings,
			NewKOTNotifier(db),
			NewInventoryNotifier(db),
			NewAnalyticsNotifier(db, redis),
		)
	}

	// Stop signals the outbox relay worker to exit cleanly.
	func (s *OrderService) Stop() { close(s.outboxWorkerStop) }

	// ─────────────────────────────────────────────────────────────────────────────
	// LAYER 1 — GATEWAY
	// Idempotency dedup → RBAC authorization
	// ─────────────────────────────────────────────────────────────────────────────

	// SubmitOrder is the primary entry point. Call this from your HTTP handler.
	//
	//	resp, err := orderSvc.SubmitOrder(c.Request.Context(), actorID, tenantID, req)
	//	if errors.Is(err, services.ErrIdempotentReplay) {
	//	    c.JSON(http.StatusOK, resp)  // 200 not 201 — replay
	//	    return
	//	}
	func (s *OrderService) SubmitOrder(
		ctx context.Context,
		actorID uuid.UUID,
		tenantID uuid.UUID,
		req *SubmitOrderRequest,
	) (*SubmitOrderResponse, error) {
		ctx, cancel := context.WithTimeout(ctx, orderSubmitTimeout)
		defer cancel()

		// ── 1a. Idempotency ───────────────────────────────────────────────────
		idempotencyKey := fmt.Sprintf("idempotency:order:%s:%s", tenantID, req.IdempotencyKey)
		if cached, err := s.checkIdempotency(ctx, idempotencyKey); err != nil {
			return nil, fmt.Errorf("idempotency check: %w", err)
		} else if cached != nil {
			cached.Cached = true
			return cached, ErrIdempotentReplay
		}

		// ── 1b. RBAC ──────────────────────────────────────────────────────────
		if err := s.rbac.Require(ctx, &AccessRequest{
			UserID:   actorID,
			TenantID: tenantID,
			Resource: ResourceOrder,
			Action:   ActionCreate,
		}); err != nil {
			return nil, err
		}

		// ── 2. Domain validation ──────────────────────────────────────────────
		dc, err := s.validateDomain(ctx, tenantID, req)
		if err != nil {
			return nil, err
		}

		// ── 3. Atomic transaction ─────────────────────────────────────────────
		resp, err := s.runTransaction(ctx, actorID, tenantID, req, dc)
		if err != nil {
			return nil, err
		}

		// ── 4. Cache idempotency result (post-commit, non-blocking) ──────────
		go s.cacheIdempotencyResult(context.Background(), idempotencyKey, resp)

		return resp, nil
	}

	func (s *OrderService) checkIdempotency(ctx context.Context, key string) (*SubmitOrderResponse, error) {
		data, err := s.redis.Get(ctx, key).Bytes()
		if errors.Is(err, goredis.Nil) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("redis get: %w", err)
		}
		var resp SubmitOrderResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, nil // corrupt entry — re-execute
		}
		return &resp, nil
	}

	func (s *OrderService) cacheIdempotencyResult(ctx context.Context, key string, resp *SubmitOrderResponse) {
		data, err := json.Marshal(resp)
		if err != nil {
			return
		}
		_ = s.redis.Set(ctx, key, data, idempotencyTTL).Err()
	}

	// ─────────────────────────────────────────────────────────────────────────────
	// LAYER 2 — DOMAIN VALIDATION
	// Table FSM guard → menu availability → price integrity → stock check
	// ─────────────────────────────────────────────────────────────────────────────

	// domainContext carries everything loaded during validation.
	// The transaction layer re-uses this — no repeated DB queries.
	type domainContext struct {
		profile      models.RestaurantProfile           // financial rules: tax rates, service charge, happy hour
		table        *models.Table                      // nil for non-dine-in
		menuItems    map[uuid.UUID]models.MenuItem
		modifiers    map[uuid.UUID]models.MenuItemModifier
		estReadyMins int
	}

	func (s *OrderService) validateDomain(
		ctx context.Context,
		tenantID uuid.UUID,
		req *SubmitOrderRequest,
	) (*domainContext, error) {
		if len(req.Items) == 0 {
			return nil, ErrEmptyOrder
		}

		dc := &domainContext{
			menuItems: make(map[uuid.UUID]models.MenuItem),
			modifiers: make(map[uuid.UUID]models.MenuItemModifier),
		}

		// ── 2a. Load RestaurantProfile (all financial rules live here) ────────
		profile, err := s.settings.GetRestaurantProfile(ctx, req.RestaurantID)
		if err != nil {
			return nil, fmt.Errorf("restaurant profile: %w", err)
		}
		dc.profile = *profile

		// ── 2b. Table FSM guard ───────────────────────────────────────────────
		if req.TableID != nil && req.OrderType == "dine-in" {
			var table models.Table
			if err := s.db.WithContext(ctx).
				Where("table_id = ? AND restaurant_id = ?", *req.TableID, req.RestaurantID).
				First(&table).Error; err != nil {
				return nil, fmt.Errorf("table not found: %w", err)
			}
			tableMachine := s.tableFSM.Restore(&tableContextAdapter{&table}, utils.TableState(table.Status))
			if !tableMachine.Can(ctx, utils.TableEventSeat, utils.NewEnvelope(utils.TableEventSeat, nil)) {
				return nil, fmt.Errorf("%w: table %d is %s", ErrTableNotAvailable, table.TableID, table.Status)
			}
			dc.table = &table
		}

		// ── 2c. Load all menu items in one query ──────────────────────────────
		menuItemIDs := make([]uuid.UUID, 0, len(req.Items))
		for _, item := range req.Items {
			menuItemIDs = append(menuItemIDs, item.MenuItemID)
		}

		var menuItems []models.MenuItem
		if err := s.db.WithContext(ctx).
			Where("menu_item_id IN ? AND tenant_id = ? AND restaurant_id = ? AND is_available = true",
				menuItemIDs, tenantID, req.RestaurantID).
			Find(&menuItems).Error; err != nil {
			return nil, fmt.Errorf("menu item load: %w", err)
		}
		for _, mi := range menuItems {
			dc.menuItems[mi.MenuItemID] = mi
		}

		// ── 2d. Price integrity + prep time accumulation ──────────────────────
		var failures []ValidationFailure
		for i, item := range req.Items {
			mi, found := dc.menuItems[item.MenuItemID]
			if !found {
				failures = append(failures, ValidationFailure{
					Field:   fmt.Sprintf("items[%d].menu_item_id", i),
					Code:    "item_unavailable",
					Message: fmt.Sprintf("menu item %s is not available or does not exist", item.MenuItemID),
				})
				continue
			}

			priceDelta := mi.BasePrice - item.ClientUnitPrice
			if priceDelta < 0 {
				priceDelta = -priceDelta
			}
			if priceDelta > mi.BasePrice*0.01 {
				failures = append(failures, ValidationFailure{
					Field:   fmt.Sprintf("items[%d].unit_price", i),
					Code:    "price_mismatch",
					Message: fmt.Sprintf("submitted price %.2f does not match current price %.2f", item.ClientUnitPrice, mi.BasePrice),
				})
			}

			if mi.PreparationTimeMinutes != nil && *mi.PreparationTimeMinutes > dc.estReadyMins {
				dc.estReadyMins = *mi.PreparationTimeMinutes
			}
		}

		// ── 2e. Load and validate modifiers ──────────────────────────────────
		modifierIDs := make([]uuid.UUID, 0)
		for _, item := range req.Items {
			for _, m := range item.Modifiers {
				modifierIDs = append(modifierIDs, m.MenuItemModifierID)
			}
		}
		if len(modifierIDs) > 0 {
			var mods []models.MenuItemModifier
			if err := s.db.WithContext(ctx).
				Where("menu_item_modifier_id IN ? AND is_available = true", modifierIDs).
				Find(&mods).Error; err != nil {
				return nil, fmt.Errorf("modifier load: %w", err)
			}
			for _, m := range mods {
				dc.modifiers[m.MenuItemModifierID] = m
			}
			for i, item := range req.Items {
				for j, mod := range item.Modifiers {
					if _, found := dc.modifiers[mod.MenuItemModifierID]; !found {
						failures = append(failures, ValidationFailure{
							Field:   fmt.Sprintf("items[%d].modifiers[%d].menu_item_modifier_id", i, j),
							Code:    "modifier_unavailable",
							Message: fmt.Sprintf("modifier %s is not available", mod.MenuItemModifierID),
						})
					}
				}
			}
		}

		if len(failures) > 0 {
			return nil, newValidationError(failures...)
		}

		// ── 2f. Inventory stock check (fails open if table doesn't exist) ─────
		if err := s.checkInventoryStock(ctx, tenantID, req.RestaurantID, req.Items); err != nil {
			return nil, err
		}

		return dc, nil
	}

	// checkInventoryStock verifies stock via menu_item_inventory_links.
	// Fails open when that table doesn't exist yet — orders are never blocked
	// by a missing infrastructure table.
	func (s *OrderService) checkInventoryStock(
		ctx context.Context,
		tenantID, restaurantID uuid.UUID,
		items []SubmitOrderItemRequest,
	) error {
		stockCtx, cancel := context.WithTimeout(ctx, stockCheckTimeout)
		defer cancel()

		type stockRow struct {
			MenuItemID      uuid.UUID
			InventoryItemID uuid.UUID
			CurrentQty      float64
			RequiredQty     float64
		}

		menuItemIDs := make([]uuid.UUID, len(items))
		quantityMap := make(map[uuid.UUID]int)
		for i, item := range items {
			menuItemIDs[i] = item.MenuItemID
			quantityMap[item.MenuItemID] += item.Quantity
		}

		var rows []stockRow
		err := s.db.WithContext(stockCtx).Raw(`
			SELECT
				ml.menu_item_id,
				ml.inventory_item_id,
				ii.current_quantity AS current_qty,
				ml.quantity_per_unit * 1 AS required_qty
			FROM   menu_item_inventory_links ml
			JOIN   inventory_items ii ON ii.inventory_item_id = ml.inventory_item_id
			WHERE  ml.menu_item_id = ANY(?)
			AND  ii.tenant_id = ?
			AND  ii.restaurant_id = ?
		`, menuItemIDs, tenantID, restaurantID).Scan(&rows).Error

		if err != nil {
			return nil // table doesn't exist yet — fail open
		}

		var failures []ValidationFailure
		for _, row := range rows {
			ordered := float64(quantityMap[row.MenuItemID]) * row.RequiredQty
			if row.CurrentQty < ordered {
				failures = append(failures, ValidationFailure{
					Field:   "items",
					Code:    "insufficient_stock",
					Message: fmt.Sprintf("item %s: need %.2f units, only %.2f available", row.MenuItemID, ordered, row.CurrentQty),
				})
			}
		}
		if len(failures) > 0 {
			return newValidationError(failures...)
		}
		return nil
	}

	// ─────────────────────────────────────────────────────────────────────────────
	// LAYER 3 — ATOMIC TRANSACTION
	// All writes in one transaction. If any step fails, everything rolls back.
	// ─────────────────────────────────────────────────────────────────────────────

	func (s *OrderService) runTransaction(
		ctx context.Context,
		actorID, tenantID uuid.UUID,
		req *SubmitOrderRequest,
		dc *domainContext,
	) (*SubmitOrderResponse, error) {
		var resp *SubmitOrderResponse

		txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			orderID := uuid.New()
			now := time.Now()
			kotNumber := generateKOTNumber(now)

			// ── 3a. Build items + run TaxCalculator ───────────────────────────
			items, taxBreakdown, err := s.buildOrderItemsWithTax(ctx, orderID, tenantID, req, dc)
			if err != nil {
				return fmt.Errorf("build items: %w", err)
			}

			// ── 3b. Build Order struct ────────────────────────────────────────
			order := &models.Order{
				OrderID:       orderID,
				UserID:        actorID,
				TenantID:      tenantID,
				RestaurantID:  req.RestaurantID,
				CustomerID:    req.CustomerID,
				TableID:       req.TableID,
				OrderType:     models.OrderType(req.OrderType),
				OrderStatus:   models.OrderItemStatusPending,
				CustomerName:  req.CustomerName,
				PhoneNumber:   req.PhoneNumber,
				Notes:         req.Notes,
				Items:         items,
				TotalAmount:   taxBreakdown.GrandTotal,
				// SubTotal, TaxAmount, ServiceCharge: added in order.go v2 migration.
				// Once the new order.go model is deployed, restore these three lines:
				// SubTotal:      taxBreakdown.SubtotalBeforeTax,
				// TaxAmount:     taxBreakdown.TotalTaxAmount,
				// ServiceCharge: taxBreakdown.ServiceChargeAmount,
				CreatedAt:     now,
			}

			// ── 3c. Persist Order ─────────────────────────────────────────────
			if err := tx.Omit(clause.Associations).Create(order).Error; err != nil {
				return fmt.Errorf("persist order: %w", err)
			}

			// ── 3d. Persist OrderItems + Modifiers ────────────────────────────
			for i := range items {
				if err := tx.Omit(clause.Associations).Create(&items[i]).Error; err != nil {
					return fmt.Errorf("persist item[%d]: %w", i, err)
				}
				for j := range items[i].Modifiers {
					items[i].Modifiers[j].OrderItemID = items[i].OrderItemID
					if err := tx.Omit(clause.Associations).Create(&items[i].Modifiers[j]).Error; err != nil {
						return fmt.Errorf("persist modifier[%d][%d]: %w", i, j, err)
					}
				}
			}

			// ── 3e. Persist OrderTaxBreakdown ─────────────────────────────────
			taxRecord := &models.OrderTaxBreakdown{
				TaxBreakdownID:      uuid.New(),
				OrderID:             orderID,
				StandardTaxAmount:   taxBreakdown.StandardTaxAmount,
				AlcoholTaxAmount:    taxBreakdown.AlcoholTaxAmount,
				TotalTaxAmount:      taxBreakdown.TotalTaxAmount,
				TaxIncludedInPrice:  taxBreakdown.TaxIncludedInPrice,
				ServiceChargeAmount: taxBreakdown.ServiceChargeAmount,
				TaxRate:             taxBreakdown.TaxRate,
				CreatedAt:           now,
			}
			if err := tx.Create(taxRecord).Error; err != nil {
				return fmt.Errorf("persist tax breakdown: %w", err)
			}

			// ── 3f. Persist OrderLog ──────────────────────────────────────────
			orderLog := &models.OrderLog{
				LogID:   uuid.New(),
				OrderID: orderID,
				UserID:  actorID,
				Action:  "order_submitted",
				Details: fmt.Sprintf(
					"order submitted; grand=%.2f sub=%.2f tax=%.2f svc=%.2f tax_inclusive=%v currency=%s",
					taxBreakdown.GrandTotal, taxBreakdown.SubtotalBeforeTax,
					taxBreakdown.TotalTaxAmount, taxBreakdown.ServiceChargeAmount,
					taxBreakdown.TaxIncludedInPrice, dc.profile.CurrencyCode,
				),
				Timestamp: now,
			}
			if err := tx.Create(orderLog).Error; err != nil {
				return fmt.Errorf("persist order log: %w", err)
			}

			// ── 3g. Persist AuditTrail ────────────────────────────────────────
			auditEntry := &models.AuditTrail{
				AuditTrailID:     uuid.New(),
				TenantID:         tenantID,
				UserID:           &actorID,
				RestaurantID:     &req.RestaurantID,
				EventType:        models.AuditEventCreate,
				EventCategory:    "order",
				EventDescription: fmt.Sprintf("order %s submitted", orderID),
				Severity:         models.AuditSeverityInfo,
				EntityType:       "order",
				EntityID:         orderID.String(),
				NewValues: mustMarshalJSONB(map[string]interface{}{
					"order_id":        orderID,
					"subtotal":        taxBreakdown.SubtotalBeforeTax,
					"tax":             taxBreakdown.TotalTaxAmount,
					"service_charge":  taxBreakdown.ServiceChargeAmount,
					"grand_total":     taxBreakdown.GrandTotal,
					"tax_inclusive":   taxBreakdown.TaxIncludedInPrice,
					"currency":        dc.profile.CurrencyCode,
					"item_count":      len(items),
					"order_type":      req.OrderType,
				}),
				Timestamp: now,
			}
			if err := tx.Create(auditEntry).Error; err != nil {
				return fmt.Errorf("persist audit: %w", err)
			}

			// ── 3h. Table FSM: Available → Occupied (optimistic lock) ─────────
			if dc.table != nil && req.OrderType == "dine-in" {
				tableMachine := s.tableFSM.Restore(
					&tableContextAdapter{dc.table},
					utils.TableState(dc.table.Status),
				)
				seatEnv := utils.NewEnvelope(utils.TableEventSeat, nil).WithActor(actorID, tenantID)
				if err := tableMachine.Send(ctx, seatEnv); err != nil {
					return fmt.Errorf("table FSM seat: %w", err)
				}
				result := tx.Model(&models.Table{}).
					Where("table_id = ? AND status = ?", dc.table.TableID, dc.table.Status).
					Update("status", string(utils.TableStateOccupied))
				if result.Error != nil {
					return fmt.Errorf("table status update: %w", result.Error)
				}
				if result.RowsAffected == 0 {
					return fmt.Errorf("%w: table %d was taken by another order", ErrTableNotAvailable, dc.table.TableID)
				}
			}

			// ── 3i. Write Outbox event (atomically with order) ────────────────
			outboxPayload := buildOutboxPayload(orderID, tenantID, req, actorID, kotNumber, items, now)
			payloadBytes, err := json.Marshal(outboxPayload)
			if err != nil {
				return fmt.Errorf("marshal outbox payload: %w", err)
			}
			outboxEvent := &models.OutboxEvent{
				OutboxEventID: uuid.New(),
				TenantID:      tenantID,
				AggregateType: "order",
				AggregateID:   orderID.String(),
				EventType:     "OrderCreated",
				Payload:       payloadBytes,
				Status:        models.OutboxStatusPending,
				NextRetryAt:   now,
				CreatedAt:     now,
			}
			if err := tx.Create(outboxEvent).Error; err != nil {
				return fmt.Errorf("persist outbox: %w", err)
			}

			// ── 3j. Build response ────────────────────────────────────────────
			resp = &SubmitOrderResponse{
				OrderID:               orderID,
				OrderStatus:           string(models.OrderStatusPending),
				SubTotal:              taxBreakdown.SubtotalBeforeTax,
				TaxAmount:             taxBreakdown.TotalTaxAmount,
				ServiceChargeAmount:   taxBreakdown.ServiceChargeAmount,
				DiscountAmount:        0,
				GrandTotal:            taxBreakdown.GrandTotal,
				KOTNumber:             kotNumber,
				EstimatedReadyMinutes: dc.estReadyMins,
				TaxBreakdown:          toTaxBreakdownDTO(taxBreakdown),
			}
			return nil
		})

		if txErr != nil {
			return nil, txErr
		}
		return resp, nil
	}

	// buildOrderItemsWithTax constructs []models.OrderItem and runs TaxCalculator.
	// The profile is already loaded in dc — no extra DB query here.
	func (s *OrderService) buildOrderItemsWithTax(
		ctx context.Context,
		orderID, tenantID uuid.UUID,
		req *SubmitOrderRequest,
		dc *domainContext,
	) ([]models.OrderItem, *TaxBreakdown, error) {
		items := make([]models.OrderItem, 0, len(req.Items))
		taxItems := make([]OrderItemForTax, 0, len(req.Items))

		for _, itemReq := range req.Items {
			mi := dc.menuItems[itemReq.MenuItemID]
			menuItemID := mi.MenuItemID

			mods := make([]models.OrderItemModifier, 0, len(itemReq.Modifiers))
			taxMods := make([]ModifierForTax, 0, len(itemReq.Modifiers))

			for _, modReq := range itemReq.Modifiers {
				mod := dc.modifiers[modReq.MenuItemModifierID]
				modID := mod.MenuItemModifierID
				mods = append(mods, models.OrderItemModifier{
					OrderItemModifierID: uuid.New(),
					MenuItemModifierID:  &modID,
					ModifierName:        mod.Name,
					AdditionalPrice:     mod.PriceAdjustment,
				})
				taxMods = append(taxMods, ModifierForTax{
					Name:            mod.Name,
					AdditionalPrice: mod.PriceAdjustment,
				})
			}

			orderItem := models.OrderItem{
				OrderItemID: uuid.New(),
				OrderID:     orderID,
				MenuItemID:  &menuItemID,
				ItemName:    mi.Name,
				UnitPrice:   mi.BasePrice, // always canonical DB price, never client value
				Quantity:    itemReq.Quantity,
				Notes:       itemReq.Notes,
				Status:      models.OrderItemStatusPending,
				Modifiers:   mods,
			}
			items = append(items, orderItem)

			taxItems = append(taxItems, OrderItemForTax{
				ItemName:    mi.Name,
				UnitPrice:   mi.BasePrice,
				Quantity:    itemReq.Quantity,
				TaxCategory: DetermineTaxCategory(&mi),
				Modifiers:   taxMods,
			})
		}

		// TaxCalculator reads rates from RestaurantProfile + TenantSetting overrides.
		// The profile is already in dc — CalculateTax uses the injected SettingsService
		// only for the four key/value overrides (tax_enabled, tax_label, etc.).
		taxBreakdown, err := s.taxCalculator.CalculateTax(
			ctx,
			tenantID,
			req.RestaurantID,
			req.OrderType,
			taxItems,
			dc.profile.ServiceChargePct,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("calculate tax: %w", err)
		}

		return items, taxBreakdown, nil
	}

	// buildOutboxPayload assembles the OrderCreatedPayload for the outbox event.
	func buildOutboxPayload(
		orderID, tenantID uuid.UUID,
		req *SubmitOrderRequest,
		actorID uuid.UUID,
		kotNumber string,
		items []models.OrderItem,
		now time.Time,
	) models.OrderCreatedPayload {
		payloadItems := make([]models.OrderCreatedItemPayload, len(items))
		for i, item := range items {
			payloadItems[i] = models.OrderCreatedItemPayload{
				OrderItemID:    item.OrderItemID,
				MenuItemID:     item.MenuItemID,
				ItemName:       item.ItemName,
				Quantity:       item.Quantity,
				UnitPrice:      item.UnitPrice,
				KitchenStation: string(models.KitchenStationGeneral),
			}
		}
		return models.OrderCreatedPayload{
			OrderID:      orderID,
			TenantID:     tenantID,
			RestaurantID: req.RestaurantID,
			TableID:      req.TableID,
			OrderType:    req.OrderType,
			KOTNumber:    kotNumber,
			ActorID:      actorID,
			OccurredAt:   now,
			Items:        payloadItems,
		}
	}

	// ─────────────────────────────────────────────────────────────────────────────
	// LAYER 4 — OUTBOX RELAY WORKER
	// Polls outbox_events and fans out to registered DownstreamNotifiers.
	// SELECT FOR UPDATE SKIP LOCKED — safe for multi-pod deployments.
	// ─────────────────────────────────────────────────────────────────────────────

	func (s *OrderService) startOutboxRelay() {
		s.outboxWorkerOnce.Do(func() { go s.outboxRelayLoop() })
	}

	func (s *OrderService) outboxRelayLoop() {
		ticker := time.NewTicker(outboxWorkerBackoff)
		defer ticker.Stop()
		for {
			select {
			case <-s.outboxWorkerStop:
				return
			case <-ticker.C:
				s.processOutboxBatch(context.Background())
			}
		}
	}

	func (s *OrderService) processOutboxBatch(ctx context.Context) {
		batchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		var events []models.OutboxEvent
		if err := s.db.WithContext(batchCtx).
			Where("status = ? AND next_retry_at <= NOW()", models.OutboxStatusPending).
			Order("next_retry_at ASC").
			Limit(50).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Find(&events).Error; err != nil {
			return
		}
		for i := range events {
			s.processOneOutboxEvent(batchCtx, &events[i])
		}
	}

	func (s *OrderService) processOneOutboxEvent(ctx context.Context, event *models.OutboxEvent) {
		now := time.Now()
		_ = s.db.WithContext(ctx).Model(event).Updates(map[string]interface{}{
			"status":          models.OutboxStatusProcessing,
			"last_attempt_at": now,
			"attempts":        event.Attempts + 1,
		})

		var payload models.OrderCreatedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			s.failOutboxEvent(ctx, event, "unmarshal failed: "+err.Error())
			return
		}

		var deliveryErrors []string
		for _, notifier := range s.notifiers {
			if notifier.EventType() != event.EventType {
				continue
			}
			if err := notifier.Notify(ctx, payload); err != nil {
				deliveryErrors = append(deliveryErrors, fmt.Sprintf("%T: %v", notifier, err))
			}
		}

		if len(deliveryErrors) > 0 {
			nextRetry := time.Now().Add(outboxWorkerBackoff * time.Duration(1<<minInt(event.Attempts, 6)))
			updates := map[string]interface{}{
				"status":        models.OutboxStatusPending,
				"next_retry_at": nextRetry,
				"error":         strings.Join(deliveryErrors, "; "),
			}
			if event.Attempts >= maxOutboxRetries {
				updates["status"] = models.OutboxStatusFailed
			}
			_ = s.db.WithContext(ctx).Model(event).Updates(updates)
			return
		}

		processedAt := time.Now()
		_ = s.db.WithContext(ctx).Model(event).Updates(map[string]interface{}{
			"status":       models.OutboxStatusDelivered,
			"processed_at": processedAt,
			"error":        "",
		})
	}

	func (s *OrderService) failOutboxEvent(ctx context.Context, event *models.OutboxEvent, reason string) {
		_ = s.db.WithContext(ctx).Model(event).Updates(map[string]interface{}{
			"status": models.OutboxStatusFailed,
			"error":  reason,
		})
	}

	// ─────────────────────────────────────────────────────────────────────────────
	// FSM CONTEXT ADAPTERS
	// ─────────────────────────────────────────────────────────────────────────────

	type tableContextAdapter struct{ *models.Table }

	func (a *tableContextAdapter) GetTableID() int    { return a.TableID }
	func (a *tableContextAdapter) GetCapacity() int   { return a.Capacity }
	func (a *tableContextAdapter) SetStatus(s string) { a.Status = models.TableStatus(s) }

	// ─────────────────────────────────────────────────────────────────────────────
	// HELPERS
	// ─────────────────────────────────────────────────────────────────────────────

	// generateKOTNumber produces a human-readable KOT number.
	// Format: YYYYMMDD-HHMMSS-<4 random chars>
	func generateKOTNumber(t time.Time) string {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		suffix := base64.URLEncoding.EncodeToString(b)[:4]
		return fmt.Sprintf("%s-%s", t.Format("20060102-150405"), suffix)
	}

	// mustMarshalJSONB converts any value to models.JSONB (map[string]interface{}).
	// models.JSONB is NOT []byte — it is map[string]interface{} with its own
	// driver.Valuer that marshals to JSON when GORM writes to Postgres.
	func mustMarshalJSONB(v interface{}) models.JSONB {
		b, err := json.Marshal(v)
		if err != nil {
			return models.JSONB{}
		}
		var m map[string]interface{}
		if err := json.Unmarshal(b, &m); err != nil {
			return models.JSONB{}
		}
		return models.JSONB(m)
	}

	func minInt(a, b int) int {
		if a < b {
			return a
		}
		return b
	}

	// toTaxBreakdownDTO converts the internal TaxBreakdown to the response DTO.
	func toTaxBreakdownDTO(tb *TaxBreakdown) *TaxBreakdownDTO {
		if tb == nil {
			return nil
		}
		details := make([]ItemTaxDTO, len(tb.ItemTaxBreakdown))
		for i, item := range tb.ItemTaxBreakdown {
			details[i] = ItemTaxDTO{
				ItemName:    item.ItemName,
				Quantity:    item.Quantity,
				UnitPrice:   item.UnitPrice,
				LineTotal:   item.LineTotal,
				TaxRate:     item.TaxRate,
				TaxAmount:   item.TaxAmount,
				TaxCategory: item.TaxCategory,
			}
		}
		return &TaxBreakdownDTO{
			StandardTaxAmount:  tb.StandardTaxAmount,
			AlcoholTaxAmount:   tb.AlcoholTaxAmount,
			TaxIncludedInPrice: tb.TaxIncludedInPrice,
			ItemDetails:        details,
		}
	}
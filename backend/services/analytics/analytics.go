// services/analytics/analytics.go
// AnalyticsService -- FSM-driven analytics pipeline for revenue, order,
// menu-item, and inventory analytics. Each public method executes an
// internal pipeline that transitions through the AnalyticsPipelineFSM
// stages: collecting -> aggregating -> enriching -> delivering -> completed.
// All queries are tenant- and restaurant-scoped. Results are cached in
// Redis with configurable TTLs to avoid hammering the database on
// dashboard refreshes.
package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"backend/utils"
)


// CACHE TTLs


const (
	analyticsCacheTTL    = 5 * time.Minute
	analyticsCachePrefix = "analytics:"
)


// SERVICE


// AnalyticsService provides read-only analytics queries driven by the
// AnalyticsPipelineFSM. Each query method creates a pipeline run,
// transitions through the FSM stages, and returns the result.
type AnalyticsService struct {
	db    *gorm.DB
	redis *goredis.Client
}

// NewAnalyticsService constructs a new AnalyticsService.
func NewAnalyticsService(db *gorm.DB, redis *goredis.Client) *AnalyticsService {
	return &AnalyticsService{db: db, redis: redis}
}


// REQUEST / RESPONSE TYPES


// AnalyticsRequest scopes every analytics query to a tenant + restaurant
// within a time range.
type AnalyticsRequest struct {
	TenantID     uuid.UUID
	RestaurantID uuid.UUID
	From         time.Time
	To           time.Time
}

// RevenueOverview contains top-level revenue metrics.
type RevenueOverview struct {
	TotalRevenue        float64 `json:"total_revenue"`
	OrderCount          int     `json:"order_count"`
	AverageOrderValue   float64 `json:"average_order_value"`
	TotalTax            float64 `json:"total_tax"`
	TotalDiscount       float64 `json:"total_discount"`
	NetRevenue          float64 `json:"net_revenue"`
	PreviousPeriodDelta float64 `json:"previous_period_delta_pct"`
}

// RevenueTrendPoint is a single data point in a revenue time series.
type RevenueTrendPoint struct {
	Date    string  `json:"date"`
	Revenue float64 `json:"revenue"`
	Orders  int     `json:"orders"`
}

// CategoryRevenue breaks revenue down by menu category.
type CategoryRevenue struct {
	Category   string  `json:"category"`
	Revenue    float64 `json:"revenue"`
	OrderCount int     `json:"order_count"`
	Percentage float64 `json:"percentage"`
}

// OrderVolumePoint is a single data point in an order volume time series.
type OrderVolumePoint struct {
	Date       string `json:"date"`
	OrderCount int    `json:"order_count"`
}

// OrderStatusDistribution shows count per order status.
type OrderStatusDistribution struct {
	Status string  `json:"status"`
	Count  int     `json:"count"`
	Pct    float64 `json:"percentage"`
}

// PeakHourEntry shows order concentration per hour.
type PeakHourEntry struct {
	Hour       int     `json:"hour"`
	OrderCount int     `json:"order_count"`
	Revenue    float64 `json:"revenue"`
	Percentage float64 `json:"percentage"`
}

// MenuItemPerformance shows per-item sales metrics.
type MenuItemPerformance struct {
	MenuItemID uuid.UUID `json:"menu_item_id"`
	Name       string    `json:"name"`
	Category   string    `json:"category"`
	Quantity   int       `json:"quantity_sold"`
	Revenue    float64   `json:"revenue"`
	AvgPrice   float64   `json:"avg_price"`
	Rank       int       `json:"rank"`
}

// InventoryTurnoverEntry shows turnover metrics for an inventory item.
type InventoryTurnoverEntry struct {
	InventoryItemID uuid.UUID `json:"inventory_item_id"`
	Name            string    `json:"name"`
	TotalConsumed   float64   `json:"total_consumed"`
	CurrentStock    float64   `json:"current_stock"`
	TurnoverRate    float64   `json:"turnover_rate"`
	DaysOfSupply    float64   `json:"days_of_supply"`
}

// WastageEntry shows waste/damage/expiry metrics for an inventory item.
type WastageEntry struct {
	InventoryItemID uuid.UUID `json:"inventory_item_id"`
	Name            string    `json:"name"`
	DamagedQty      float64   `json:"damaged_qty"`
	ExpiredQty      float64   `json:"expired_qty"`
	TotalWaste      float64   `json:"total_waste"`
	WasteCost       float64   `json:"waste_cost"`
}


// PIPELINE CONTEXT ADAPTER


// analyticsPipelineRun implements utils.AnalyticsPipelineContext.
type analyticsPipelineRun struct {
	pipelineID  string
	stage       string
	recordCount int
	startedAt   time.Time
	completedAt time.Time
	errMsg      string
}

func (r *analyticsPipelineRun) GetPipelineID() string      { return r.pipelineID }
func (r *analyticsPipelineRun) SetStage(s string)          { r.stage = s }
func (r *analyticsPipelineRun) GetRecordCount() int        { return r.recordCount }
func (r *analyticsPipelineRun) SetStartedAt(t time.Time)   { r.startedAt = t }
func (r *analyticsPipelineRun) SetCompletedAt(t time.Time) { r.completedAt = t }
func (r *analyticsPipelineRun) SetError(err string)        { r.errMsg = err }


// PIPELINE RUNNER


// pipelineStage is a function that executes one stage of the pipeline.
type pipelineStage struct {
	event utils.AnalyticsPipelineEvent
	run   func(ctx context.Context) error
}

// runPipeline drives the AnalyticsPipelineFSM through a sequence of stages.
// If any stage fails, the FSM transitions to the failed state and the error
// is returned. On success, the FSM reaches the completed state.
func (s *AnalyticsService) runPipeline(
	ctx context.Context,
	run *analyticsPipelineRun,
	stages []pipelineStage,
) error {
	machine := utils.AnalyticsPipelineFSM.New(run, utils.APStateIdle)

	// Start: idle -> collecting
	startEnv := utils.NewEnvelope(utils.APEventStartCollect, nil)
	if err := machine.Send(ctx, startEnv); err != nil {
		return fmt.Errorf("pipeline start: %w", err)
	}

	for _, stage := range stages {
		if err := stage.run(ctx); err != nil {
			// Transition to failed state
			failEnv := utils.NewEnvelope(utils.APEventFail, nil).
				WithMeta("error", err.Error())
			_ = machine.Send(ctx, failEnv)
			return fmt.Errorf("pipeline stage %s: %w", stage.event, err)
		}

		env := utils.NewEnvelope(stage.event, nil)
		if err := machine.Send(ctx, env); err != nil {
			return fmt.Errorf("pipeline transition %s: %w", stage.event, err)
		}
	}

	return nil
}


// REVENUE ANALYTICS


// GetRevenueOverview returns top-level revenue metrics for the given period.
// Runs through the full AnalyticsPipelineFSM lifecycle.
func (s *AnalyticsService) GetRevenueOverview(
	ctx context.Context,
	req AnalyticsRequest,
) (*RevenueOverview, error) {
	cacheKey := fmt.Sprintf("%srevenue:overview:%s:%s:%d:%d",
		analyticsCachePrefix, req.TenantID, req.RestaurantID,
		req.From.Unix(), req.To.Unix())

	if data, err := s.redis.Get(ctx, cacheKey).Bytes(); err == nil {
		var result RevenueOverview
		if json.Unmarshal(data, &result) == nil {
			return &result, nil
		}
	}

	var result RevenueOverview
	run := &analyticsPipelineRun{pipelineID: uuid.NewString()}

	stages := []pipelineStage{
		{
			event: utils.APEventCollected,
			run: func(ctx context.Context) error {
				// Collecting stage: query raw order data
				type row struct {
					TotalRevenue  float64
					OrderCount    int
					TotalTax      float64
					TotalDiscount float64
				}
				var r row
				err := s.db.WithContext(ctx).Raw(`
					SELECT
						COALESCE(SUM(total_amount), 0) AS total_revenue,
						COUNT(*) AS order_count,
						COALESCE(SUM(tax), 0) AS total_tax,
						COALESCE(SUM(discount), 0) AS total_discount
					FROM orders
					WHERE tenant_id = ? AND restaurant_id = ?
						AND created_at >= ? AND created_at <= ?
						AND status NOT IN ('cancelled', 'refunded')
				`, req.TenantID, req.RestaurantID, req.From, req.To).Scan(&r).Error
				if err != nil {
					return err
				}
				result.TotalRevenue = r.TotalRevenue
				result.OrderCount = r.OrderCount
				result.TotalTax = r.TotalTax
				result.TotalDiscount = r.TotalDiscount
				run.recordCount = r.OrderCount
				return nil
			},
		},
		{
			event: utils.APEventAggregated,
			run: func(ctx context.Context) error {
				// Aggregating stage: compute derived metrics
				if result.OrderCount > 0 {
					result.AverageOrderValue = result.TotalRevenue / float64(result.OrderCount)
				}
				result.NetRevenue = result.TotalRevenue - result.TotalDiscount
				return nil
			},
		},
		{
			event: utils.APEventEnriched,
			run: func(ctx context.Context) error {
				// Enriching stage: compute period-over-period delta
				periodDuration := req.To.Sub(req.From)
				prevFrom := req.From.Add(-periodDuration)
				prevTo := req.From

				var prevRevenue float64
				s.db.WithContext(ctx).Raw(`
					SELECT COALESCE(SUM(total_amount), 0)
					FROM orders
					WHERE tenant_id = ? AND restaurant_id = ?
						AND created_at >= ? AND created_at <= ?
						AND status NOT IN ('cancelled', 'refunded')
				`, req.TenantID, req.RestaurantID, prevFrom, prevTo).Scan(&prevRevenue)

				if prevRevenue > 0 {
					result.PreviousPeriodDelta = ((result.TotalRevenue - prevRevenue) / prevRevenue) * 100
				}
				result.PreviousPeriodDelta = math.Round(result.PreviousPeriodDelta*100) / 100
				return nil
			},
		},
		{
			event: utils.APEventDelivered,
			run: func(ctx context.Context) error {
				// Delivering stage: cache the result
				if data, err := json.Marshal(result); err == nil {
					_ = s.redis.Set(ctx, cacheKey, data, analyticsCacheTTL).Err()
				}
				return nil
			},
		},
	}

	if err := s.runPipeline(ctx, run, stages); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetRevenueTrend returns daily revenue data points for the given period.
func (s *AnalyticsService) GetRevenueTrend(
	ctx context.Context,
	req AnalyticsRequest,
) ([]RevenueTrendPoint, error) {
	cacheKey := fmt.Sprintf("%srevenue:trend:%s:%s:%d:%d",
		analyticsCachePrefix, req.TenantID, req.RestaurantID,
		req.From.Unix(), req.To.Unix())

	if data, err := s.redis.Get(ctx, cacheKey).Bytes(); err == nil {
		var result []RevenueTrendPoint
		if json.Unmarshal(data, &result) == nil {
			return result, nil
		}
	}

	var points []RevenueTrendPoint
	run := &analyticsPipelineRun{pipelineID: uuid.NewString()}

	stages := []pipelineStage{
		{
			event: utils.APEventCollected,
			run: func(ctx context.Context) error {
				return s.db.WithContext(ctx).Raw(`
					SELECT
						TO_CHAR(DATE(created_at), 'YYYY-MM-DD') AS date,
						COALESCE(SUM(total_amount), 0) AS revenue,
						COUNT(*) AS orders
					FROM orders
					WHERE tenant_id = ? AND restaurant_id = ?
						AND created_at >= ? AND created_at <= ?
						AND status NOT IN ('cancelled', 'refunded')
					GROUP BY DATE(created_at)
					ORDER BY DATE(created_at)
				`, req.TenantID, req.RestaurantID, req.From, req.To).Scan(&points).Error
			},
		},
		{
			event: utils.APEventAggregated,
			run: func(ctx context.Context) error {
				run.recordCount = len(points)
				return nil
			},
		},
		{
			event: utils.APEventEnriched,
			run: func(ctx context.Context) error {
				// No additional enrichment needed for trend data
				return nil
			},
		},
		{
			event: utils.APEventDelivered,
			run: func(ctx context.Context) error {
				if data, err := json.Marshal(points); err == nil {
					_ = s.redis.Set(ctx, cacheKey, data, analyticsCacheTTL).Err()
				}
				return nil
			},
		},
	}

	if err := s.runPipeline(ctx, run, stages); err != nil {
		return nil, err
	}
	return points, nil
}

// GetRevenueByCategory breaks revenue down by menu category.
func (s *AnalyticsService) GetRevenueByCategory(
	ctx context.Context,
	req AnalyticsRequest,
) ([]CategoryRevenue, error) {
	var categories []CategoryRevenue
	run := &analyticsPipelineRun{pipelineID: uuid.NewString()}

	stages := []pipelineStage{
		{
			event: utils.APEventCollected,
			run: func(ctx context.Context) error {
				return s.db.WithContext(ctx).Raw(`
					SELECT
						COALESCE(mi.category, 'Uncategorized') AS category,
						COALESCE(SUM(oi.price * oi.quantity), 0) AS revenue,
						COUNT(DISTINCT o.order_id) AS order_count
					FROM order_items oi
					JOIN orders o ON o.order_id = oi.order_id
					LEFT JOIN menu_items mi ON mi.menu_item_id = oi.menu_item_id
					WHERE o.tenant_id = ? AND o.restaurant_id = ?
						AND o.created_at >= ? AND o.created_at <= ?
						AND o.status NOT IN ('cancelled', 'refunded')
					GROUP BY mi.category
					ORDER BY revenue DESC
				`, req.TenantID, req.RestaurantID, req.From, req.To).Scan(&categories).Error
			},
		},
		{
			event: utils.APEventAggregated,
			run: func(ctx context.Context) error {
				run.recordCount = len(categories)
				return nil
			},
		},
		{
			event: utils.APEventEnriched,
			run: func(ctx context.Context) error {
				var total float64
				for _, c := range categories {
					total += c.Revenue
				}
				if total > 0 {
					for i := range categories {
						categories[i].Percentage = math.Round((categories[i].Revenue/total)*10000) / 100
					}
				}
				return nil
			},
		},
		{
			event: utils.APEventDelivered,
			run:   func(ctx context.Context) error { return nil },
		},
	}

	if err := s.runPipeline(ctx, run, stages); err != nil {
		return nil, err
	}
	return categories, nil
}


// ORDER ANALYTICS


// GetOrderVolume returns daily order counts for the given period.
func (s *AnalyticsService) GetOrderVolume(
	ctx context.Context,
	req AnalyticsRequest,
) ([]OrderVolumePoint, error) {
	var points []OrderVolumePoint
	run := &analyticsPipelineRun{pipelineID: uuid.NewString()}

	stages := []pipelineStage{
		{
			event: utils.APEventCollected,
			run: func(ctx context.Context) error {
				return s.db.WithContext(ctx).Raw(`
					SELECT
						TO_CHAR(DATE(created_at), 'YYYY-MM-DD') AS date,
						COUNT(*) AS order_count
					FROM orders
					WHERE tenant_id = ? AND restaurant_id = ?
						AND created_at >= ? AND created_at <= ?
					GROUP BY DATE(created_at)
					ORDER BY DATE(created_at)
				`, req.TenantID, req.RestaurantID, req.From, req.To).Scan(&points).Error
			},
		},
		{
			event: utils.APEventAggregated,
			run: func(ctx context.Context) error {
				run.recordCount = len(points)
				return nil
			},
		},
		{
			event: utils.APEventEnriched,
			run:   func(ctx context.Context) error { return nil },
		},
		{
			event: utils.APEventDelivered,
			run:   func(ctx context.Context) error { return nil },
		},
	}

	if err := s.runPipeline(ctx, run, stages); err != nil {
		return nil, err
	}
	return points, nil
}

// GetOrderStatusDistribution returns order counts grouped by status.
func (s *AnalyticsService) GetOrderStatusDistribution(
	ctx context.Context,
	req AnalyticsRequest,
) ([]OrderStatusDistribution, error) {
	var dist []OrderStatusDistribution
	run := &analyticsPipelineRun{pipelineID: uuid.NewString()}

	stages := []pipelineStage{
		{
			event: utils.APEventCollected,
			run: func(ctx context.Context) error {
				return s.db.WithContext(ctx).Raw(`
					SELECT status, COUNT(*) AS count
					FROM orders
					WHERE tenant_id = ? AND restaurant_id = ?
						AND created_at >= ? AND created_at <= ?
					GROUP BY status
					ORDER BY count DESC
				`, req.TenantID, req.RestaurantID, req.From, req.To).Scan(&dist).Error
			},
		},
		{
			event: utils.APEventAggregated,
			run: func(ctx context.Context) error {
				run.recordCount = len(dist)
				return nil
			},
		},
		{
			event: utils.APEventEnriched,
			run: func(ctx context.Context) error {
				var total int
				for _, d := range dist {
					total += d.Count
				}
				if total > 0 {
					for i := range dist {
						dist[i].Pct = math.Round(float64(dist[i].Count)/float64(total)*10000) / 100
					}
				}
				return nil
			},
		},
		{
			event: utils.APEventDelivered,
			run:   func(ctx context.Context) error { return nil },
		},
	}

	if err := s.runPipeline(ctx, run, stages); err != nil {
		return nil, err
	}
	return dist, nil
}

// GetPeakHours returns order distribution by hour of day.
func (s *AnalyticsService) GetPeakHours(
	ctx context.Context,
	req AnalyticsRequest,
) ([]PeakHourEntry, error) {
	var hours []PeakHourEntry
	run := &analyticsPipelineRun{pipelineID: uuid.NewString()}

	stages := []pipelineStage{
		{
			event: utils.APEventCollected,
			run: func(ctx context.Context) error {
				return s.db.WithContext(ctx).Raw(`
					SELECT
						EXTRACT(HOUR FROM created_at)::int AS hour,
						COUNT(*) AS order_count,
						COALESCE(SUM(total_amount), 0) AS revenue
					FROM orders
					WHERE tenant_id = ? AND restaurant_id = ?
						AND created_at >= ? AND created_at <= ?
						AND status NOT IN ('cancelled', 'refunded')
					GROUP BY EXTRACT(HOUR FROM created_at)
					ORDER BY hour
				`, req.TenantID, req.RestaurantID, req.From, req.To).Scan(&hours).Error
			},
		},
		{
			event: utils.APEventAggregated,
			run: func(ctx context.Context) error {
				run.recordCount = len(hours)
				return nil
			},
		},
		{
			event: utils.APEventEnriched,
			run: func(ctx context.Context) error {
				var total int
				for _, h := range hours {
					total += h.OrderCount
				}
				if total > 0 {
					for i := range hours {
						hours[i].Percentage = math.Round(float64(hours[i].OrderCount)/float64(total)*10000) / 100
					}
				}
				return nil
			},
		},
		{
			event: utils.APEventDelivered,
			run:   func(ctx context.Context) error { return nil },
		},
	}

	if err := s.runPipeline(ctx, run, stages); err != nil {
		return nil, err
	}
	return hours, nil
}


// MENU PERFORMANCE


// GetTopSellingItems returns the top N menu items by quantity sold.
func (s *AnalyticsService) GetTopSellingItems(
	ctx context.Context,
	req AnalyticsRequest,
	limit int,
) ([]MenuItemPerformance, error) {
	if limit <= 0 {
		limit = 10
	}

	var items []MenuItemPerformance
	run := &analyticsPipelineRun{pipelineID: uuid.NewString()}

	stages := []pipelineStage{
		{
			event: utils.APEventCollected,
			run: func(ctx context.Context) error {
				return s.db.WithContext(ctx).Raw(`
					SELECT
						oi.menu_item_id,
						COALESCE(mi.name, 'Unknown') AS name,
						COALESCE(mi.category, 'Uncategorized') AS category,
						COALESCE(SUM(oi.quantity), 0) AS quantity,
						COALESCE(SUM(oi.price * oi.quantity), 0) AS revenue
					FROM order_items oi
					JOIN orders o ON o.order_id = oi.order_id
					LEFT JOIN menu_items mi ON mi.menu_item_id = oi.menu_item_id
					WHERE o.tenant_id = ? AND o.restaurant_id = ?
						AND o.created_at >= ? AND o.created_at <= ?
						AND o.status NOT IN ('cancelled', 'refunded')
					GROUP BY oi.menu_item_id, mi.name, mi.category
					ORDER BY quantity DESC
					LIMIT ?
				`, req.TenantID, req.RestaurantID, req.From, req.To, limit).Scan(&items).Error
			},
		},
		{
			event: utils.APEventAggregated,
			run: func(ctx context.Context) error {
				run.recordCount = len(items)
				return nil
			},
		},
		{
			event: utils.APEventEnriched,
			run: func(ctx context.Context) error {
				for i := range items {
					items[i].Rank = i + 1
					if items[i].Quantity > 0 {
						items[i].AvgPrice = items[i].Revenue / float64(items[i].Quantity)
						items[i].AvgPrice = math.Round(items[i].AvgPrice*100) / 100
					}
				}
				return nil
			},
		},
		{
			event: utils.APEventDelivered,
			run:   func(ctx context.Context) error { return nil },
		},
	}

	if err := s.runPipeline(ctx, run, stages); err != nil {
		return nil, err
	}
	return items, nil
}


// INVENTORY ANALYTICS


// GetInventoryTurnover computes turnover rates for inventory items.
// Turnover rate = total consumed / current stock over the period.
// Days of supply = current stock / (daily consumption rate).
func (s *AnalyticsService) GetInventoryTurnover(
	ctx context.Context,
	req AnalyticsRequest,
) ([]InventoryTurnoverEntry, error) {
	var entries []InventoryTurnoverEntry
	run := &analyticsPipelineRun{pipelineID: uuid.NewString()}
	periodDays := req.To.Sub(req.From).Hours() / 24
	if periodDays < 1 {
		periodDays = 1
	}

	stages := []pipelineStage{
		{
			event: utils.APEventCollected,
			run: func(ctx context.Context) error {
				return s.db.WithContext(ctx).Raw(`
					SELECT
						ii.inventory_item_id,
						ii.name,
						COALESCE(ABS(SUM(CASE WHEN sm.quantity_delta < 0 THEN sm.quantity_delta ELSE 0 END)), 0) AS total_consumed,
						ii.current_quantity AS current_stock
					FROM inventory_items ii
					LEFT JOIN stock_movements sm
						ON sm.inventory_item_id = ii.inventory_item_id
						AND sm.created_at >= ? AND sm.created_at <= ?
					WHERE ii.tenant_id = ? AND ii.restaurant_id = ?
					GROUP BY ii.inventory_item_id, ii.name, ii.current_quantity
					ORDER BY total_consumed DESC
				`, req.From, req.To, req.TenantID, req.RestaurantID).Scan(&entries).Error
			},
		},
		{
			event: utils.APEventAggregated,
			run: func(ctx context.Context) error {
				run.recordCount = len(entries)
				return nil
			},
		},
		{
			event: utils.APEventEnriched,
			run: func(ctx context.Context) error {
				for i := range entries {
					if entries[i].CurrentStock > 0 {
						entries[i].TurnoverRate = math.Round((entries[i].TotalConsumed/entries[i].CurrentStock)*100) / 100
					}
					dailyRate := entries[i].TotalConsumed / periodDays
					if dailyRate > 0 {
						entries[i].DaysOfSupply = math.Round((entries[i].CurrentStock/dailyRate)*10) / 10
					}
				}
				return nil
			},
		},
		{
			event: utils.APEventDelivered,
			run:   func(ctx context.Context) error { return nil },
		},
	}

	if err := s.runPipeline(ctx, run, stages); err != nil {
		return nil, err
	}
	return entries, nil
}

// GetWastageReport returns waste (damage + expiry) metrics per inventory item.
func (s *AnalyticsService) GetWastageReport(
	ctx context.Context,
	req AnalyticsRequest,
) ([]WastageEntry, error) {
	var entries []WastageEntry
	run := &analyticsPipelineRun{pipelineID: uuid.NewString()}

	stages := []pipelineStage{
		{
			event: utils.APEventCollected,
			run: func(ctx context.Context) error {
				return s.db.WithContext(ctx).Raw(`
					SELECT
						ii.inventory_item_id,
						ii.name,
						COALESCE(ABS(SUM(CASE WHEN sm.reason = 'damaged' THEN sm.quantity_delta ELSE 0 END)), 0) AS damaged_qty,
						COALESCE(ABS(SUM(CASE WHEN sm.reason = 'expired' THEN sm.quantity_delta ELSE 0 END)), 0) AS expired_qty
					FROM inventory_items ii
					LEFT JOIN stock_movements sm
						ON sm.inventory_item_id = ii.inventory_item_id
						AND sm.reason IN ('damaged', 'expired')
						AND sm.created_at >= ? AND sm.created_at <= ?
					WHERE ii.tenant_id = ? AND ii.restaurant_id = ?
					GROUP BY ii.inventory_item_id, ii.name
					HAVING SUM(CASE WHEN sm.reason IN ('damaged', 'expired') THEN 1 ELSE 0 END) > 0
					ORDER BY (ABS(SUM(sm.quantity_delta))) DESC
				`, req.From, req.To, req.TenantID, req.RestaurantID).Scan(&entries).Error
			},
		},
		{
			event: utils.APEventAggregated,
			run: func(ctx context.Context) error {
				run.recordCount = len(entries)
				return nil
			},
		},
		{
			event: utils.APEventEnriched,
			run: func(ctx context.Context) error {
				for i := range entries {
					entries[i].TotalWaste = entries[i].DamagedQty + entries[i].ExpiredQty
					// Fetch unit cost for waste valuation
					var unitCost *float64
					s.db.WithContext(ctx).Raw(
						"SELECT unit_cost FROM inventory_items WHERE inventory_item_id = ?",
						entries[i].InventoryItemID,
					).Scan(&unitCost)
					if unitCost != nil {
						entries[i].WasteCost = math.Round(entries[i].TotalWaste*(*unitCost)*100) / 100
					}
				}
				return nil
			},
		},
		{
			event: utils.APEventDelivered,
			run:   func(ctx context.Context) error { return nil },
		},
	}

	if err := s.runPipeline(ctx, run, stages); err != nil {
		return nil, err
	}
	return entries, nil
}

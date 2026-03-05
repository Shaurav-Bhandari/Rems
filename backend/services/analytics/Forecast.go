// services/analytics/Forecast.go
//
// ForecastService -- FSM-driven demand and revenue forecasting using
// exponential smoothing with additive seasonality. Each forecast method
// executes the full ForecastPipelineFSM lifecycle:
//
//	ingesting -> feature_engineering -> model_training -> scoring
//	  -> post_processing -> publishing -> completed
//
// The model uses a weighted moving average with day-of-week seasonal
// indices and a trend component extracted via simple linear regression.
// Confidence intervals are computed from the residual standard deviation.
package analytics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"backend/utils"
)

// ─────────────────────────────────────────────────────────────────────────────
// SERVICE
// ─────────────────────────────────────────────────────────────────────────────

// ForecastService provides demand and revenue forecasting driven by the
// ForecastPipelineFSM. Each method runs the full pipeline and returns
// structured results with confidence intervals.
type ForecastService struct {
	db    *gorm.DB
	redis *goredis.Client
}

// NewForecastService constructs a new ForecastService.
func NewForecastService(db *gorm.DB, redis *goredis.Client) *ForecastService {
	return &ForecastService{db: db, redis: redis}
}

// ─────────────────────────────────────────────────────────────────────────────
// TYPES
// ─────────────────────────────────────────────────────────────────────────────

// ForecastRequest scopes a forecast to a tenant + restaurant and specifies
// how far back to look (lookback) and how far forward to predict (horizon).
type ForecastRequest struct {
	TenantID     uuid.UUID
	RestaurantID uuid.UUID
	LookbackDays int // number of days of historical data to use
	HorizonDays  int // number of days to forecast forward
}

// DemandForecastItem contains per-menu-item demand predictions.
type DemandForecastItem struct {
	MenuItemID    uuid.UUID           `json:"menu_item_id"`
	Name          string              `json:"name"`
	Category      string              `json:"category"`
	HistoricalAvg float64             `json:"historical_avg_daily"`
	Predictions   []ForecastDataPoint `json:"predictions"`
}

// ForecastDataPoint is a single forecast output with confidence bounds.
type ForecastDataPoint struct {
	Date       string  `json:"date"`
	Predicted  float64 `json:"predicted"`
	LowerBound float64 `json:"lower_bound"`
	UpperBound float64 `json:"upper_bound"`
	Confidence float64 `json:"confidence"` // e.g. 0.95
}

// RevenueForecast contains overall revenue predictions.
type RevenueForecast struct {
	Predictions    []ForecastDataPoint `json:"predictions"`
	TotalPredicted float64             `json:"total_predicted"`
	TrendDirection string              `json:"trend_direction"` // up, down, flat
	TrendStrength  float64             `json:"trend_strength"`  // percent per day
}

// ReorderForecast predicts when an inventory item will hit its reorder point.
type ReorderForecast struct {
	InventoryItemID  uuid.UUID `json:"inventory_item_id"`
	Name             string    `json:"name"`
	CurrentStock     float64   `json:"current_stock"`
	ReorderPoint     float64   `json:"reorder_point"`
	DailyConsumption float64   `json:"daily_consumption"`
	DaysUntilReorder float64   `json:"days_until_reorder"`
	PredictedDate    string    `json:"predicted_reorder_date"`
	UrgencyLevel     string    `json:"urgency_level"` // critical, warning, normal
}

// ─────────────────────────────────────────────────────────────────────────────
// PIPELINE CONTEXT ADAPTER
// ─────────────────────────────────────────────────────────────────────────────

// forecastPipelineRun implements utils.ForecastPipelineContext.
type forecastPipelineRun struct {
	pipelineID     string
	stage          string
	dataPointCount int
	modelAccuracy  float64
	startedAt      time.Time
	completedAt    time.Time
	errMsg         string
}

func (r *forecastPipelineRun) GetPipelineID() string      { return r.pipelineID }
func (r *forecastPipelineRun) SetStage(s string)          { r.stage = s }
func (r *forecastPipelineRun) GetDataPointCount() int     { return r.dataPointCount }
func (r *forecastPipelineRun) GetModelAccuracy() float64  { return r.modelAccuracy }
func (r *forecastPipelineRun) SetStartedAt(t time.Time)   { r.startedAt = t }
func (r *forecastPipelineRun) SetCompletedAt(t time.Time) { r.completedAt = t }
func (r *forecastPipelineRun) SetError(err string)        { r.errMsg = err }

// ─────────────────────────────────────────────────────────────────────────────
// INTERNAL MODEL
// ─────────────────────────────────────────────────────────────────────────────

// forecastModel holds the fitted parameters for exponential smoothing
// with additive day-of-week seasonality.
type forecastModel struct {
	level           float64          // smoothed level
	trend           float64          // trend per day
	seasonalIndices [7]float64       // additive seasonal index per weekday (0=Sun)
	alpha           float64          // level smoothing factor
	beta            float64          // trend smoothing factor
	gamma           float64          // seasonal smoothing factor
	residualStdDev  float64          // standard deviation of residuals
	dataPoints      []dailyDataPoint // historical data used for fitting
}

// dailyDataPoint is one day of historical observations.
type dailyDataPoint struct {
	date  time.Time
	value float64
}

// fit trains the model on historical data using Holt-Winters additive
// seasonality with a 7-day seasonal period.
func (m *forecastModel) fit(data []dailyDataPoint) {
	n := len(data)
	if n < 7 {
		// Not enough data — use simple average
		var sum float64
		for _, d := range data {
			sum += d.value
		}
		m.level = sum / float64(n)
		m.trend = 0
		for i := range m.seasonalIndices {
			m.seasonalIndices[i] = 0
		}
		m.residualStdDev = 0
		return
	}

	// Set smoothing parameters
	m.alpha = 0.3 // level
	m.beta = 0.1  // trend
	m.gamma = 0.2 // seasonal

	// Initialize seasonal indices from the first full week
	var weekAvg float64
	firstWeek := min(7, n)
	for i := 0; i < firstWeek; i++ {
		weekAvg += data[i].value
	}
	weekAvg /= float64(firstWeek)
	if weekAvg == 0 {
		weekAvg = 1
	}

	for i := 0; i < firstWeek; i++ {
		dow := int(data[i].date.Weekday())
		m.seasonalIndices[dow] = data[i].value - weekAvg
	}

	// Initialize level and trend
	m.level = weekAvg
	if n >= 14 {
		var secondWeekAvg float64
		for i := 7; i < 14; i++ {
			secondWeekAvg += data[i].value
		}
		secondWeekAvg /= 7
		m.trend = (secondWeekAvg - weekAvg) / 7
	}

	// Fit: iterate over all data points
	residuals := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		dow := int(data[i].date.Weekday())
		predicted := m.level + m.trend + m.seasonalIndices[dow]
		residual := data[i].value - predicted
		residuals = append(residuals, residual)

		prevLevel := m.level
		m.level = m.alpha*(data[i].value-m.seasonalIndices[dow]) + (1-m.alpha)*(m.level+m.trend)
		m.trend = m.beta*(m.level-prevLevel) + (1-m.beta)*m.trend
		m.seasonalIndices[dow] = m.gamma*(data[i].value-m.level) + (1-m.gamma)*m.seasonalIndices[dow]
	}

	// Compute residual standard deviation
	if len(residuals) > 1 {
		var sumSq float64
		for _, r := range residuals {
			sumSq += r * r
		}
		m.residualStdDev = math.Sqrt(sumSq / float64(len(residuals)-1))
	}
	m.dataPoints = data
}

// predict generates forecasts for the next horizonDays.
func (m *forecastModel) predict(startDate time.Time, horizonDays int, confidence float64) []ForecastDataPoint {
	// z-score for the given confidence level (using normal approximation)
	z := 1.96 // default 95%
	switch {
	case confidence >= 0.99:
		z = 2.576
	case confidence >= 0.95:
		z = 1.96
	case confidence >= 0.90:
		z = 1.645
	case confidence >= 0.80:
		z = 1.282
	}

	predictions := make([]ForecastDataPoint, horizonDays)
	for h := 0; h < horizonDays; h++ {
		date := startDate.AddDate(0, 0, h+1)
		dow := int(date.Weekday())
		stepsAhead := float64(h + 1)

		pointForecast := m.level + m.trend*stepsAhead + m.seasonalIndices[dow]
		if pointForecast < 0 {
			pointForecast = 0 // floor constraint
		}

		// Confidence interval widens with forecast horizon
		intervalWidth := z * m.residualStdDev * math.Sqrt(stepsAhead)

		predictions[h] = ForecastDataPoint{
			Date:       date.Format("2006-01-02"),
			Predicted:  math.Round(pointForecast*100) / 100,
			LowerBound: math.Max(0, math.Round((pointForecast-intervalWidth)*100)/100),
			UpperBound: math.Round((pointForecast+intervalWidth)*100) / 100,
			Confidence: confidence,
		}
	}
	return predictions
}

// accuracy returns the R-squared of the fitted model.
func (m *forecastModel) accuracy() float64 {
	if len(m.dataPoints) < 2 {
		return 0
	}

	var mean, ssTot, ssRes float64
	for _, d := range m.dataPoints {
		mean += d.value
	}
	mean /= float64(len(m.dataPoints))

	for i, d := range m.dataPoints {
		dow := int(d.date.Weekday())
		predicted := m.level + m.trend*float64(i) + m.seasonalIndices[dow]
		ssRes += (d.value - predicted) * (d.value - predicted)
		ssTot += (d.value - mean) * (d.value - mean)
	}

	if ssTot == 0 {
		return 1.0
	}
	r2 := 1 - ssRes/ssTot
	if r2 < 0 {
		r2 = 0
	}
	return math.Round(r2*10000) / 10000
}

// ─────────────────────────────────────────────────────────────────────────────
// PIPELINE RUNNER
// ─────────────────────────────────────────────────────────────────────────────

// forecastStage is a function that executes one stage of the forecast pipeline.
type forecastStage struct {
	event utils.ForecastPipelineEvent
	run   func(ctx context.Context) error
}

// runForecastPipeline drives the ForecastPipelineFSM through all stages.
func (s *ForecastService) runForecastPipeline(
	ctx context.Context,
	run *forecastPipelineRun,
	stages []forecastStage,
) error {
	machine := utils.ForecastPipelineFSM.New(run, utils.FPStateIdle)

	// Start: idle -> ingesting
	startEnv := utils.NewEnvelope(utils.FPEventStartIngest, nil)
	if err := machine.Send(ctx, startEnv); err != nil {
		return fmt.Errorf("forecast pipeline start: %w", err)
	}

	for _, stage := range stages {
		if err := stage.run(ctx); err != nil {
			failEnv := utils.NewEnvelope(utils.FPEventFail, nil).
				WithMeta("error", err.Error())
			_ = machine.Send(ctx, failEnv)
			return fmt.Errorf("forecast stage %s: %w", stage.event, err)
		}

		env := utils.NewEnvelope(stage.event, nil)
		if err := machine.Send(ctx, env); err != nil {
			return fmt.Errorf("forecast transition %s: %w", stage.event, err)
		}
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// DEMAND FORECAST
// ─────────────────────────────────────────────────────────────────────────────

// ForecastDemand predicts per-menu-item daily demand using Holt-Winters
// exponential smoothing with day-of-week seasonality.
func (s *ForecastService) ForecastDemand(
	ctx context.Context,
	req ForecastRequest,
) ([]DemandForecastItem, error) {
	if req.LookbackDays <= 0 {
		req.LookbackDays = 30
	}
	if req.HorizonDays <= 0 {
		req.HorizonDays = 7
	}

	now := time.Now()
	lookbackStart := now.AddDate(0, 0, -req.LookbackDays)

	var results []DemandForecastItem
	run := &forecastPipelineRun{pipelineID: uuid.NewString()}

	// Raw data container
	type dailySales struct {
		MenuItemID uuid.UUID
		Name       string
		Category   string
		SaleDate   time.Time
		Quantity   int
	}
	var rawData []dailySales
	var models map[uuid.UUID]*forecastModel

	stages := []forecastStage{
		// Stage 1: Ingesting -- load historical daily sales per item
		{
			event: utils.FPEventIngested,
			run: func(ctx context.Context) error {
				err := s.db.WithContext(ctx).Raw(`
					SELECT
						oi.menu_item_id,
						COALESCE(mi.name, 'Unknown') AS name,
						COALESCE(mi.category, 'Uncategorized') AS category,
						DATE(o.created_at) AS sale_date,
						COALESCE(SUM(oi.quantity), 0) AS quantity
					FROM order_items oi
					JOIN orders o ON o.order_id = oi.order_id
					LEFT JOIN menu_items mi ON mi.menu_item_id = oi.menu_item_id
					WHERE o.tenant_id = ? AND o.restaurant_id = ?
						AND o.created_at >= ?
						AND o.status NOT IN ('cancelled', 'refunded')
					GROUP BY oi.menu_item_id, mi.name, mi.category, DATE(o.created_at)
					ORDER BY oi.menu_item_id, DATE(o.created_at)
				`, req.TenantID, req.RestaurantID, lookbackStart).Scan(&rawData).Error
				if err != nil {
					return err
				}
				run.dataPointCount = len(rawData)
				return nil
			},
		},
		// Stage 2: Feature Engineering -- organize data per item
		{
			event: utils.FPEventFeaturesReady,
			run: func(ctx context.Context) error {
				// Group by menu item
				itemData := make(map[uuid.UUID]struct {
					name     string
					category string
					points   []dailyDataPoint
				})
				for _, row := range rawData {
					entry := itemData[row.MenuItemID]
					entry.name = row.Name
					entry.category = row.Category
					entry.points = append(entry.points, dailyDataPoint{
						date:  row.SaleDate,
						value: float64(row.Quantity),
					})
					itemData[row.MenuItemID] = entry
				}

				// Sort each item's data by date
				for id, entry := range itemData {
					sort.Slice(entry.points, func(i, j int) bool {
						return entry.points[i].date.Before(entry.points[j].date)
					})
					itemData[id] = entry
				}

				// Store for next stage
				models = make(map[uuid.UUID]*forecastModel, len(itemData))
				results = make([]DemandForecastItem, 0, len(itemData))
				for id, entry := range itemData {
					m := &forecastModel{}
					models[id] = m
					var sum float64
					for _, p := range entry.points {
						sum += p.value
					}
					results = append(results, DemandForecastItem{
						MenuItemID:    id,
						Name:          entry.name,
						Category:      entry.category,
						HistoricalAvg: math.Round(sum/float64(len(entry.points))*100) / 100,
					})
					m.dataPoints = entry.points
				}
				return nil
			},
		},
		// Stage 3: Model Training -- fit Holt-Winters on each item
		{
			event: utils.FPEventModelTrained,
			run: func(ctx context.Context) error {
				var totalAccuracy float64
				for id, m := range models {
					m.fit(m.dataPoints)
					totalAccuracy += m.accuracy()
					_ = id
				}
				if len(models) > 0 {
					run.modelAccuracy = totalAccuracy / float64(len(models))
				}
				return nil
			},
		},
		// Stage 4: Scoring -- generate predictions
		{
			event: utils.FPEventScored,
			run: func(ctx context.Context) error {
				for i := range results {
					m := models[results[i].MenuItemID]
					if m != nil {
						lastDate := now
						if len(m.dataPoints) > 0 {
							lastDate = m.dataPoints[len(m.dataPoints)-1].date
						}
						results[i].Predictions = m.predict(lastDate, req.HorizonDays, 0.95)
					}
				}
				return nil
			},
		},
		// Stage 5: Post-processing -- apply business rules
		{
			event: utils.FPEventPostProcessed,
			run: func(ctx context.Context) error {
				for i := range results {
					for j := range results[i].Predictions {
						// Round to whole units for menu item demand
						results[i].Predictions[j].Predicted = math.Round(results[i].Predictions[j].Predicted)
						results[i].Predictions[j].LowerBound = math.Floor(results[i].Predictions[j].LowerBound)
						results[i].Predictions[j].UpperBound = math.Ceil(results[i].Predictions[j].UpperBound)
					}
				}
				// Sort results by historical avg descending (highest demand first)
				sort.Slice(results, func(i, j int) bool {
					return results[i].HistoricalAvg > results[j].HistoricalAvg
				})
				return nil
			},
		},
		// Stage 6: Publishing -- done
		{
			event: utils.FPEventPublished,
			run: func(ctx context.Context) error {
				return nil
			},
		},
	}

	if err := s.runForecastPipeline(ctx, run, stages); err != nil {
		return nil, err
	}
	return results, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// REVENUE FORECAST
// ─────────────────────────────────────────────────────────────────────────────

// ForecastRevenue predicts total daily revenue using Holt-Winters on
// historical daily revenue totals.
func (s *ForecastService) ForecastRevenue(
	ctx context.Context,
	req ForecastRequest,
) (*RevenueForecast, error) {
	if req.LookbackDays <= 0 {
		req.LookbackDays = 30
	}
	if req.HorizonDays <= 0 {
		req.HorizonDays = 7
	}

	now := time.Now()
	lookbackStart := now.AddDate(0, 0, -req.LookbackDays)

	result := &RevenueForecast{}
	run := &forecastPipelineRun{pipelineID: uuid.NewString()}
	model := &forecastModel{}

	stages := []forecastStage{
		// Ingest
		{
			event: utils.FPEventIngested,
			run: func(ctx context.Context) error {
				type dailyRev struct {
					Date    time.Time
					Revenue float64
				}
				var rows []dailyRev
				err := s.db.WithContext(ctx).Raw(`
					SELECT
						DATE(created_at) AS date,
						COALESCE(SUM(total_amount), 0) AS revenue
					FROM orders
					WHERE tenant_id = ? AND restaurant_id = ?
						AND created_at >= ?
						AND status NOT IN ('cancelled', 'refunded')
					GROUP BY DATE(created_at)
					ORDER BY DATE(created_at)
				`, req.TenantID, req.RestaurantID, lookbackStart).Scan(&rows).Error
				if err != nil {
					return err
				}
				for _, r := range rows {
					model.dataPoints = append(model.dataPoints, dailyDataPoint{
						date:  r.Date,
						value: r.Revenue,
					})
				}
				run.dataPointCount = len(model.dataPoints)
				return nil
			},
		},
		// Feature Engineering (no-op for univariate)
		{
			event: utils.FPEventFeaturesReady,
			run:   func(ctx context.Context) error { return nil },
		},
		// Model Training
		{
			event: utils.FPEventModelTrained,
			run: func(ctx context.Context) error {
				model.fit(model.dataPoints)
				run.modelAccuracy = model.accuracy()
				return nil
			},
		},
		// Scoring
		{
			event: utils.FPEventScored,
			run: func(ctx context.Context) error {
				lastDate := now
				if len(model.dataPoints) > 0 {
					lastDate = model.dataPoints[len(model.dataPoints)-1].date
				}
				result.Predictions = model.predict(lastDate, req.HorizonDays, 0.95)
				return nil
			},
		},
		// Post-processing
		{
			event: utils.FPEventPostProcessed,
			run: func(ctx context.Context) error {
				for _, p := range result.Predictions {
					result.TotalPredicted += p.Predicted
				}
				result.TotalPredicted = math.Round(result.TotalPredicted*100) / 100

				// Determine trend direction from the model
				if model.trend > 0.01 {
					result.TrendDirection = "up"
				} else if model.trend < -0.01 {
					result.TrendDirection = "down"
				} else {
					result.TrendDirection = "flat"
				}
				if model.level > 0 {
					result.TrendStrength = math.Round((model.trend/model.level)*10000) / 100
				}
				return nil
			},
		},
		// Publishing
		{
			event: utils.FPEventPublished,
			run:   func(ctx context.Context) error { return nil },
		},
	}

	if err := s.runForecastPipeline(ctx, run, stages); err != nil {
		return nil, err
	}
	return result, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// INVENTORY REORDER FORECAST
// ─────────────────────────────────────────────────────────────────────────────

// ForecastReorderDates predicts when each inventory item will hit its
// reorder point based on recent consumption velocity.
func (s *ForecastService) ForecastReorderDates(
	ctx context.Context,
	req ForecastRequest,
) ([]ReorderForecast, error) {
	if req.LookbackDays <= 0 {
		req.LookbackDays = 30
	}

	now := time.Now()
	lookbackStart := now.AddDate(0, 0, -req.LookbackDays)

	var forecasts []ReorderForecast
	run := &forecastPipelineRun{pipelineID: uuid.NewString()}

	type itemConsumption struct {
		InventoryItemID uuid.UUID
		Name            string
		CurrentStock    float64
		ReorderPoint    *float64
		TotalConsumed   float64
	}
	var items []itemConsumption

	stages := []forecastStage{
		// Ingest
		{
			event: utils.FPEventIngested,
			run: func(ctx context.Context) error {
				err := s.db.WithContext(ctx).Raw(`
					SELECT
						ii.inventory_item_id,
						ii.name,
						ii.current_quantity AS current_stock,
						ii.reorder_point,
						COALESCE(ABS(SUM(CASE WHEN sm.quantity_delta < 0 THEN sm.quantity_delta ELSE 0 END)), 0) AS total_consumed
					FROM inventory_items ii
					LEFT JOIN stock_movements sm
						ON sm.inventory_item_id = ii.inventory_item_id
						AND sm.created_at >= ?
					WHERE ii.tenant_id = ? AND ii.restaurant_id = ?
						AND ii.reorder_point IS NOT NULL
					GROUP BY ii.inventory_item_id, ii.name, ii.current_quantity, ii.reorder_point
				`, lookbackStart, req.TenantID, req.RestaurantID).Scan(&items).Error
				if err != nil {
					return err
				}
				run.dataPointCount = len(items)
				return nil
			},
		},
		// Feature Engineering
		{
			event: utils.FPEventFeaturesReady,
			run:   func(ctx context.Context) error { return nil },
		},
		// Model Training (trivial for linear consumption model)
		{
			event: utils.FPEventModelTrained,
			run: func(ctx context.Context) error {
				run.modelAccuracy = 1.0 // linear model
				return nil
			},
		},
		// Scoring
		{
			event: utils.FPEventScored,
			run: func(ctx context.Context) error {
				periodDays := float64(req.LookbackDays)
				if periodDays < 1 {
					periodDays = 1
				}

				for _, item := range items {
					rp := 0.0
					if item.ReorderPoint != nil {
						rp = *item.ReorderPoint
					}

					dailyConsumption := item.TotalConsumed / periodDays
					stockAboveReorder := item.CurrentStock - rp

					var daysUntilReorder float64
					var predictedDate string
					if dailyConsumption > 0 && stockAboveReorder > 0 {
						daysUntilReorder = stockAboveReorder / dailyConsumption
						predictedDate = now.AddDate(0, 0, int(math.Ceil(daysUntilReorder))).Format("2006-01-02")
					} else if stockAboveReorder <= 0 {
						daysUntilReorder = 0
						predictedDate = now.Format("2006-01-02")
					} else {
						daysUntilReorder = 999
						predictedDate = "N/A"
					}

					urgency := "normal"
					switch {
					case daysUntilReorder <= 3:
						urgency = "critical"
					case daysUntilReorder <= 7:
						urgency = "warning"
					}

					forecasts = append(forecasts, ReorderForecast{
						InventoryItemID:  item.InventoryItemID,
						Name:             item.Name,
						CurrentStock:     item.CurrentStock,
						ReorderPoint:     rp,
						DailyConsumption: math.Round(dailyConsumption*1000) / 1000,
						DaysUntilReorder: math.Round(daysUntilReorder*10) / 10,
						PredictedDate:    predictedDate,
						UrgencyLevel:     urgency,
					})
				}
				return nil
			},
		},
		// Post-processing: sort by urgency
		{
			event: utils.FPEventPostProcessed,
			run: func(ctx context.Context) error {
				sort.Slice(forecasts, func(i, j int) bool {
					return forecasts[i].DaysUntilReorder < forecasts[j].DaysUntilReorder
				})
				return nil
			},
		},
		// Publishing
		{
			event: utils.FPEventPublished,
			run:   func(ctx context.Context) error { return nil },
		},
	}

	if err := s.runForecastPipeline(ctx, run, stages); err != nil {
		return nil, err
	}
	return forecasts, nil
}

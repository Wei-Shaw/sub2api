package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	AccountWindowKeyFiveHour  = "five_hour"
	AccountWindowKeySevenDay  = "seven_day"
	AccountWindowKeyThirtyDay = "thirty_day"

	AccountWindowPeriodCurrent  = "current"
	AccountWindowPeriodPrevious = "previous"

	SuccessRateStatusAvailable          = "available"
	SuccessRateStatusNoData             = "no_data"
	SuccessRateStatusMonitoringDisabled = "monitoring_disabled"
	SuccessRateStatusRetentionLimited   = "retention_limited"

	maxAccountWindowUsageTargets  = 6
	maxAccountWindowUsageRange    = 32 * 24 * time.Hour
	maxAccountWindowUsageLookback = 65 * 24 * time.Hour
	accountWindowFutureTolerance  = time.Minute
)

// AccountWindowUsageTarget is the wire-level target. Times stay as strings so
// the service, rather than the HTTP binding layer, owns RFC3339 validation.
type AccountWindowUsageTarget struct {
	WindowKey string `json:"window_key"`
	Period    string `json:"period"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type AccountWindowUsageRequest struct {
	Windows []AccountWindowUsageTarget `json:"windows"`
}

// AccountWindowUsageQuery is the normalized repository input.
type AccountWindowUsageQuery struct {
	WindowKey string
	Period    string
	StartTime time.Time
	EndTime   time.Time
}

// AccountWindowUsageAggregate contains facts available from local logs.
type AccountWindowUsageAggregate struct {
	WindowKey    string
	Period       string
	StartTime    time.Time
	EndTime      time.Time
	SuccessCalls int64
	FailureCalls int64
	TotalTokens  int64
	AccountCost  float64
	StandardCost float64
	UserCost     float64
}

type AccountWindowUsageCoverage struct {
	MonitoringEnabled       bool
	ErrorLogRetentionCutoff *time.Time
}

type AccountWindowUsageItem struct {
	WindowKey         string    `json:"window_key"`
	Period            string    `json:"period"`
	StartTime         time.Time `json:"start_time"`
	EndTime           time.Time `json:"end_time"`
	Matched           bool      `json:"matched"`
	TotalRequests     int64     `json:"total_requests"`
	SuccessCalls      int64     `json:"success_calls"`
	FailureCalls      int64     `json:"failure_calls"`
	TotalTokens       int64     `json:"total_tokens"`
	AccountCost       float64   `json:"account_cost"`
	StandardCost      float64   `json:"standard_cost"`
	UserCost          float64   `json:"user_cost"`
	SuccessRate       *float64  `json:"success_rate"`
	SuccessRateStatus string    `json:"success_rate_status"`
}

type AccountWindowUsageResponse struct {
	GeneratedAt time.Time                `json:"generated_at"`
	Items       []AccountWindowUsageItem `json:"items"`
}

type accountWindowUsageReader interface {
	GetAccountWindowUsage(
		ctx context.Context,
		accountID int64,
		queries []AccountWindowUsageQuery,
	) ([]AccountWindowUsageAggregate, error)
}

// GetWindowUsage validates all ranges, verifies account ownership, and performs
// one capability-based repository call. The main UsageLogRepository interface
// deliberately remains unchanged so unrelated test doubles do not grow.
func (s *AccountUsageService) GetWindowUsage(
	ctx context.Context,
	accountID int64,
	targets []AccountWindowUsageTarget,
	coverage AccountWindowUsageCoverage,
) (*AccountWindowUsageResponse, error) {
	now := time.Now().UTC()
	queries, err := validateAccountWindowUsageTargets(targets, now)
	if err != nil {
		return nil, err
	}
	if accountID <= 0 {
		return nil, infraerrors.BadRequest("ACCOUNT_WINDOW_USAGE_INVALID_ACCOUNT", "account id must be greater than zero")
	}
	if _, err := s.accountRepo.GetByID(ctx, accountID); err != nil {
		return nil, err
	}

	reader, ok := s.usageLogRepo.(accountWindowUsageReader)
	if !ok {
		return nil, fmt.Errorf("usage log repository does not support account window usage")
	}
	aggregates, err := reader.GetAccountWindowUsage(ctx, accountID, queries)
	if err != nil {
		return nil, fmt.Errorf("get account window usage: %w", err)
	}
	if len(aggregates) != len(queries) {
		return nil, fmt.Errorf("get account window usage: expected %d rows, got %d", len(queries), len(aggregates))
	}

	aggregatesByTarget := make(map[string]AccountWindowUsageAggregate, len(aggregates))
	for _, aggregate := range aggregates {
		identity := accountWindowUsageIdentity(aggregate.WindowKey, aggregate.Period)
		if _, exists := aggregatesByTarget[identity]; exists {
			return nil, fmt.Errorf("get account window usage: duplicate aggregate for %s", identity)
		}
		aggregatesByTarget[identity] = aggregate
	}

	items := make([]AccountWindowUsageItem, 0, len(queries))
	for _, query := range queries {
		identity := accountWindowUsageIdentity(query.WindowKey, query.Period)
		aggregate, ok := aggregatesByTarget[identity]
		if !ok {
			return nil, fmt.Errorf("get account window usage: missing aggregate for %s", identity)
		}
		if aggregate.StartTime.UTC().UnixMicro() != query.StartTime.UTC().UnixMicro() ||
			aggregate.EndTime.UTC().UnixMicro() != query.EndTime.UTC().UnixMicro() {
			return nil, fmt.Errorf("get account window usage: aggregate range mismatch for %s", identity)
		}
		totalRequests := aggregate.SuccessCalls + aggregate.FailureCalls
		item := AccountWindowUsageItem{
			WindowKey:         query.WindowKey,
			Period:            query.Period,
			StartTime:         query.StartTime,
			EndTime:           query.EndTime,
			Matched:           totalRequests > 0,
			TotalRequests:     totalRequests,
			SuccessCalls:      aggregate.SuccessCalls,
			FailureCalls:      aggregate.FailureCalls,
			TotalTokens:       aggregate.TotalTokens,
			AccountCost:       aggregate.AccountCost,
			StandardCost:      aggregate.StandardCost,
			UserCost:          aggregate.UserCost,
			SuccessRateStatus: successRateStatus(query, totalRequests, coverage),
		}
		if item.SuccessRateStatus == SuccessRateStatusAvailable {
			rate := float64(item.SuccessCalls) / float64(totalRequests) * 100
			item.SuccessRate = &rate
		}
		items = append(items, item)
	}

	return &AccountWindowUsageResponse{GeneratedAt: now, Items: items}, nil
}

func accountWindowUsageIdentity(windowKey, period string) string {
	return windowKey + ":" + period
}

func validateAccountWindowUsageTargets(targets []AccountWindowUsageTarget, now time.Time) ([]AccountWindowUsageQuery, error) {
	if len(targets) == 0 {
		return nil, infraerrors.BadRequest("ACCOUNT_WINDOW_USAGE_EMPTY", "windows must contain at least one target")
	}
	if len(targets) > maxAccountWindowUsageTargets {
		return nil, infraerrors.BadRequest("ACCOUNT_WINDOW_USAGE_TOO_MANY", "windows must contain at most 6 targets")
	}

	allowedKeys := map[string]struct{}{
		AccountWindowKeyFiveHour: {}, AccountWindowKeySevenDay: {}, AccountWindowKeyThirtyDay: {},
	}
	allowedPeriods := map[string]struct{}{
		AccountWindowPeriodCurrent: {}, AccountWindowPeriodPrevious: {},
	}
	seen := make(map[string]struct{}, len(targets))
	queries := make([]AccountWindowUsageQuery, 0, len(targets))
	oldestAllowed := now.Add(-maxAccountWindowUsageLookback)
	latestAllowed := now.Add(accountWindowFutureTolerance)

	for i, target := range targets {
		key := strings.TrimSpace(target.WindowKey)
		period := strings.TrimSpace(target.Period)
		if _, ok := allowedKeys[key]; !ok {
			return nil, invalidAccountWindowUsageTarget(i, "window_key must be one of five_hour, seven_day, thirty_day")
		}
		if _, ok := allowedPeriods[period]; !ok {
			return nil, invalidAccountWindowUsageTarget(i, "period must be current or previous")
		}
		identity := key + ":" + period
		if _, ok := seen[identity]; ok {
			return nil, invalidAccountWindowUsageTarget(i, "window_key and period must be unique")
		}
		seen[identity] = struct{}{}

		start, err := time.Parse(time.RFC3339, strings.TrimSpace(target.StartTime))
		if err != nil {
			return nil, invalidAccountWindowUsageTarget(i, "start_time must be RFC3339")
		}
		end, err := time.Parse(time.RFC3339, strings.TrimSpace(target.EndTime))
		if err != nil {
			return nil, invalidAccountWindowUsageTarget(i, "end_time must be RFC3339")
		}
		start = start.UTC()
		end = end.UTC()
		if !start.Before(end) {
			return nil, invalidAccountWindowUsageTarget(i, "start_time must be before end_time")
		}
		if end.Sub(start) > maxAccountWindowUsageRange {
			return nil, invalidAccountWindowUsageTarget(i, "time range must not exceed 32 days")
		}
		if start.Before(oldestAllowed) {
			return nil, invalidAccountWindowUsageTarget(i, "start_time must not be more than 65 days old")
		}
		if end.After(latestAllowed) {
			return nil, invalidAccountWindowUsageTarget(i, "end_time must not be in the future")
		}
		queries = append(queries, AccountWindowUsageQuery{
			WindowKey: key, Period: period, StartTime: start, EndTime: end,
		})
	}
	return queries, nil
}

func invalidAccountWindowUsageTarget(index int, message string) error {
	return infraerrors.BadRequest(
		"ACCOUNT_WINDOW_USAGE_INVALID_TARGET",
		fmt.Sprintf("windows[%d]: %s", index, message),
	)
}

func successRateStatus(query AccountWindowUsageQuery, totalRequests int64, coverage AccountWindowUsageCoverage) string {
	if !coverage.MonitoringEnabled {
		return SuccessRateStatusMonitoringDisabled
	}
	if cutoff := coverage.ErrorLogRetentionCutoff; cutoff != nil && query.StartTime.Before(*cutoff) {
		return SuccessRateStatusRetentionLimited
	}
	if totalRequests == 0 {
		return SuccessRateStatusNoData
	}
	return SuccessRateStatusAvailable
}

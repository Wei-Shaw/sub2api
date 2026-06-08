package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

var (
	dashboardTrendCache        = newSnapshotCache(30 * time.Second)
	dashboardModelStatsCache   = newSnapshotCache(30 * time.Second)
	dashboardGroupStatsCache   = newSnapshotCache(30 * time.Second)
	dashboardUsersTrendCache   = newSnapshotCache(30 * time.Second)
	dashboardAPIKeysTrendCache = newSnapshotCache(30 * time.Second)
)

type dashboardTrendCacheKey struct {
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Granularity string `json:"granularity"`
	UserID      int64  `json:"user_id"`
	APIKeyID    int64  `json:"api_key_id"`
	AccountID   int64  `json:"account_id"`
	GroupID     int64  `json:"group_id"`
	Model       string `json:"model"`
	RequestType *int16 `json:"request_type"`
	Stream      *bool  `json:"stream"`
	BillingType *int8  `json:"billing_type"`
}

type dashboardModelGroupCacheKey struct {
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	UserID      int64  `json:"user_id"`
	APIKeyID    int64  `json:"api_key_id"`
	AccountID   int64  `json:"account_id"`
	GroupID     int64  `json:"group_id"`
	ModelSource string `json:"model_source,omitempty"`
	RequestType *int16 `json:"request_type"`
	Stream      *bool  `json:"stream"`
	BillingType *int8  `json:"billing_type"`
}

type dashboardEntityTrendCacheKey struct {
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Granularity string `json:"granularity"`
	Limit       int    `json:"limit"`
}

func cacheStatusValue(hit bool) string {
	if hit {
		return "hit"
	}
	return "miss"
}

func mustMarshalDashboardCacheKey(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func snapshotPayloadAs[T any](payload any) (T, error) {
	typed, ok := payload.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("unexpected cache payload type %T", payload)
	}
	return typed, nil
}

func (h *DashboardHandler) getUsageTrendCached(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userID, apiKeyID, accountID, groupID int64,
	model string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.TrendDataPoint, bool, error) {
	key := mustMarshalDashboardCacheKey(dashboardTrendCacheKey{
		StartTime:   startTime.UTC().Format(time.RFC3339),
		EndTime:     endTime.UTC().Format(time.RFC3339),
		Granularity: granularity,
		UserID:      userID,
		APIKeyID:    apiKeyID,
		AccountID:   accountID,
		GroupID:     groupID,
		Model:       model,
		RequestType: requestType,
		Stream:      stream,
		BillingType: billingType,
	})
	entry, hit, err := dashboardTrendCache.GetOrLoad(key, func() (any, error) {
		return h.dashboardService.GetUsageTrendWithFilters(ctx, startTime, endTime, granularity, userID, apiKeyID, accountID, groupID, model, requestType, stream, billingType)
	})
	if err != nil {
		return nil, hit, err
	}
	trend, err := snapshotPayloadAs[[]usagestats.TrendDataPoint](entry.Payload)
	return trend, hit, err
}

func (h *DashboardHandler) getModelStatsCached(
	ctx context.Context,
	startTime, endTime time.Time,
	userID, apiKeyID, accountID, groupID int64,
	modelSource string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.ModelStat, bool, error) {
	key := mustMarshalDashboardCacheKey(dashboardModelGroupCacheKey{
		StartTime:   startTime.UTC().Format(time.RFC3339),
		EndTime:     endTime.UTC().Format(time.RFC3339),
		UserID:      userID,
		APIKeyID:    apiKeyID,
		AccountID:   accountID,
		GroupID:     groupID,
		ModelSource: usagestats.NormalizeModelSource(modelSource),
		RequestType: requestType,
		Stream:      stream,
		BillingType: billingType,
	})
	entry, hit, err := dashboardModelStatsCache.GetOrLoad(key, func() (any, error) {
		return h.dashboardService.GetModelStatsWithFiltersBySource(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType, modelSource)
	})
	if err != nil {
		return nil, hit, err
	}
	stats, err := snapshotPayloadAs[[]usagestats.ModelStat](entry.Payload)
	return stats, hit, err
}

func (h *DashboardHandler) getGroupStatsCached(
	ctx context.Context,
	startTime, endTime time.Time,
	userID, apiKeyID, accountID, groupID int64,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.GroupStat, bool, error) {
	key := mustMarshalDashboardCacheKey(dashboardModelGroupCacheKey{
		StartTime:   startTime.UTC().Format(time.RFC3339),
		EndTime:     endTime.UTC().Format(time.RFC3339),
		UserID:      userID,
		APIKeyID:    apiKeyID,
		AccountID:   accountID,
		GroupID:     groupID,
		RequestType: requestType,
		Stream:      stream,
		BillingType: billingType,
	})
	entry, hit, err := dashboardGroupStatsCache.GetOrLoad(key, func() (any, error) {
		return h.dashboardService.GetGroupStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType)
	})
	if err != nil {
		return nil, hit, err
	}
	stats, err := snapshotPayloadAs[[]usagestats.GroupStat](entry.Payload)
	return stats, hit, err
}

func (h *DashboardHandler) getAPIKeyUsageTrendCached(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.APIKeyUsageTrendPoint, bool, error) {
	key := mustMarshalDashboardCacheKey(dashboardEntityTrendCacheKey{
		StartTime:   startTime.UTC().Format(time.RFC3339),
		EndTime:     endTime.UTC().Format(time.RFC3339),
		Granularity: granularity,
		Limit:       limit,
	})
	entry, hit, err := dashboardAPIKeysTrendCache.GetOrLoad(key, func() (any, error) {
		return h.dashboardService.GetAPIKeyUsageTrend(ctx, startTime, endTime, granularity, limit)
	})
	if err != nil {
		return nil, hit, err
	}
	trend, err := snapshotPayloadAs[[]usagestats.APIKeyUsageTrendPoint](entry.Payload)
	return trend, hit, err
}

func (h *DashboardHandler) getUserUsageTrendCached(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.UserUsageTrendPoint, bool, error) {
	key := mustMarshalDashboardCacheKey(dashboardEntityTrendCacheKey{
		StartTime:   startTime.UTC().Format(time.RFC3339),
		EndTime:     endTime.UTC().Format(time.RFC3339),
		Granularity: granularity,
		Limit:       limit,
	})
	entry, hit, err := dashboardUsersTrendCache.GetOrLoad(key, func() (any, error) {
		return h.dashboardService.GetUserUsageTrend(ctx, startTime, endTime, granularity, limit)
	})
	if err != nil {
		return nil, hit, err
	}
	trend, err := snapshotPayloadAs[[]usagestats.UserUsageTrendPoint](entry.Payload)
	return trend, hit, err
}

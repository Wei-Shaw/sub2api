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
	StartTime             string `json:"start_time"`
	EndTime               string `json:"end_time"`
	Granularity           string `json:"granularity"`
	UserID                int64  `json:"user_id"`
	APIKeyID              int64  `json:"api_key_id"`
	AccountID             int64  `json:"account_id"`
	GroupID               int64  `json:"group_id"`
	Model                 string `json:"model"`
	RequestType           *int16 `json:"request_type"`
	Stream                *bool  `json:"stream"`
	NativeCompactionV2    *bool  `json:"native_compaction_v2"`
	BillingType           *int8  `json:"billing_type"`
	UpstreamModelMismatch *bool  `json:"upstream_model_mismatch"`
	// ProfitExcludedUserIDs 参与缓存键，理由见 getUsageTrendCached 里的注释：
	// 不把它算进键，「改完名单看到的还是旧数字」会伪装成功能失效。
	ProfitExcludedUserIDs []int64 `json:"profit_excluded_user_ids"`
}

type dashboardModelGroupCacheKey struct {
	StartTime             string `json:"start_time"`
	EndTime               string `json:"end_time"`
	UserID                int64  `json:"user_id"`
	APIKeyID              int64  `json:"api_key_id"`
	AccountID             int64  `json:"account_id"`
	GroupID               int64  `json:"group_id"`
	ModelSource           string `json:"model_source,omitempty"`
	RequestType           *int16 `json:"request_type"`
	Stream                *bool  `json:"stream"`
	NativeCompactionV2    *bool  `json:"native_compaction_v2"`
	BillingType           *int8  `json:"billing_type"`
	UpstreamModelMismatch *bool  `json:"upstream_model_mismatch"`
}

type dashboardEntityTrendCacheKey struct {
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Granularity string `json:"granularity"`
	Limit       int    `json:"limit"`
	Metric      string `json:"metric,omitempty"`
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
	nativeCompactionV2 *bool,
	billingType *int8,
	upstreamModelMismatch *bool,
) ([]usagestats.TrendDataPoint, bool, error) {
	// 排除名单必须先取到，因为它要进缓存键。
	//
	// 若只把它塞进下面的 filters 而不进缓存键，就会出现这样一幕：管理员把某人
	// 加进名单、刷新页面，看到的还是旧数字——因为 30 秒内的缓存条目是按「不含名单」
	// 的键存下的。要等 TTL 过期才对，而界面上没有任何提示，看起来就像功能没生效。
	// 名单本身在 service 层有 30 秒缓存，这里取一次不会打库。
	excludedUserIDs, err := h.dashboardService.ProfitExcludedUserIDs(ctx)
	if err != nil {
		return nil, false, err
	}
	key := mustMarshalDashboardCacheKey(dashboardTrendCacheKey{
		StartTime:             startTime.UTC().Format(time.RFC3339),
		EndTime:               endTime.UTC().Format(time.RFC3339),
		Granularity:           granularity,
		UserID:                userID,
		APIKeyID:              apiKeyID,
		AccountID:             accountID,
		GroupID:               groupID,
		Model:                 model,
		RequestType:           requestType,
		Stream:                stream,
		NativeCompactionV2:    nativeCompactionV2,
		BillingType:           billingType,
		UpstreamModelMismatch: upstreamModelMismatch,
		ProfitExcludedUserIDs: excludedUserIDs,
	})
	entry, hit, err := dashboardTrendCache.GetOrLoad(key, func() (any, error) {
		return h.dashboardService.GetUsageTrendWithUsageFilters(ctx, startTime, endTime, granularity, usagestats.UsageLogFilters{
			UserID: userID, APIKeyID: apiKeyID, AccountID: accountID, GroupID: groupID,
			Model: model, RequestType: requestType, Stream: stream, NativeCompactionV2: nativeCompactionV2, BillingType: billingType,
			UpstreamModelMismatch: upstreamModelMismatch,
			ProfitExcludedUserIDs: excludedUserIDs,
		})
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
	nativeCompactionV2 *bool,
	billingType *int8,
	upstreamModelMismatch *bool,
) ([]usagestats.ModelStat, bool, error) {
	key := mustMarshalDashboardCacheKey(dashboardModelGroupCacheKey{
		StartTime:             startTime.UTC().Format(time.RFC3339),
		EndTime:               endTime.UTC().Format(time.RFC3339),
		UserID:                userID,
		APIKeyID:              apiKeyID,
		AccountID:             accountID,
		GroupID:               groupID,
		ModelSource:           usagestats.NormalizeModelSource(modelSource),
		RequestType:           requestType,
		Stream:                stream,
		NativeCompactionV2:    nativeCompactionV2,
		BillingType:           billingType,
		UpstreamModelMismatch: upstreamModelMismatch,
	})
	entry, hit, err := dashboardModelStatsCache.GetOrLoad(key, func() (any, error) {
		return h.dashboardService.GetModelStatsWithUsageFiltersBySource(ctx, startTime, endTime, usagestats.UsageLogFilters{
			UserID: userID, APIKeyID: apiKeyID, AccountID: accountID, GroupID: groupID,
			RequestType: requestType, Stream: stream, NativeCompactionV2: nativeCompactionV2, BillingType: billingType,
			UpstreamModelMismatch: upstreamModelMismatch,
		}, modelSource)
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
	nativeCompactionV2 *bool,
	billingType *int8,
	upstreamModelMismatch *bool,
) ([]usagestats.GroupStat, bool, error) {
	key := mustMarshalDashboardCacheKey(dashboardModelGroupCacheKey{
		StartTime:             startTime.UTC().Format(time.RFC3339),
		EndTime:               endTime.UTC().Format(time.RFC3339),
		UserID:                userID,
		APIKeyID:              apiKeyID,
		AccountID:             accountID,
		GroupID:               groupID,
		RequestType:           requestType,
		Stream:                stream,
		NativeCompactionV2:    nativeCompactionV2,
		BillingType:           billingType,
		UpstreamModelMismatch: upstreamModelMismatch,
	})
	entry, hit, err := dashboardGroupStatsCache.GetOrLoad(key, func() (any, error) {
		return h.dashboardService.GetGroupStatsWithUsageFilters(ctx, startTime, endTime, usagestats.UsageLogFilters{
			UserID: userID, APIKeyID: apiKeyID, AccountID: accountID, GroupID: groupID,
			RequestType: requestType, Stream: stream, NativeCompactionV2: nativeCompactionV2, BillingType: billingType,
			UpstreamModelMismatch: upstreamModelMismatch,
		})
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

func (h *DashboardHandler) getUserUsageTrendCached(ctx context.Context, startTime, endTime time.Time, granularity string, limit int, metric string) ([]usagestats.UserUsageTrendPoint, bool, error) {
	key := mustMarshalDashboardCacheKey(dashboardEntityTrendCacheKey{
		StartTime:   startTime.UTC().Format(time.RFC3339),
		EndTime:     endTime.UTC().Format(time.RFC3339),
		Granularity: granularity,
		Limit:       limit,
		Metric:      metric,
	})
	entry, hit, err := dashboardUsersTrendCache.GetOrLoad(key, func() (any, error) {
		return h.dashboardService.GetUserUsageTrend(ctx, startTime, endTime, granularity, limit, metric)
	})
	if err != nil {
		return nil, hit, err
	}
	trend, err := snapshotPayloadAs[[]usagestats.UserUsageTrendPoint](entry.Payload)
	return trend, hit, err
}

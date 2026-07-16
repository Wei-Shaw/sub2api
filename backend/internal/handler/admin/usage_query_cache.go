package admin

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// 与 dashboard 查询缓存同款:30s TTL 进程内缓存,仅服务 /admin/usage/stats 读路径。
var usageStatsCache = newSnapshotCache(30 * time.Second)

type usageStatsCacheKeyData struct {
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	UserID      int64  `json:"user_id"`
	APIKeyID    int64  `json:"api_key_id"`
	AccountID   int64  `json:"account_id"`
	GroupID     int64  `json:"group_id"`
	Model       string `json:"model"`
	BillingMode string `json:"billing_mode"`
	RequestType *int16 `json:"request_type"`
	Stream      *bool  `json:"stream"`
	BillingType *int8  `json:"billing_type"`
}

func usageStatsCacheKey(filters usagestats.UsageLogFilters) string {
	start := ""
	if filters.StartTime != nil {
		start = filters.StartTime.UTC().Format(time.RFC3339)
	}
	end := ""
	if filters.EndTime != nil {
		end = filters.EndTime.UTC().Format(time.RFC3339)
	}
	return mustMarshalDashboardCacheKey(usageStatsCacheKeyData{
		StartTime:   start,
		EndTime:     end,
		UserID:      filters.UserID,
		APIKeyID:    filters.APIKeyID,
		AccountID:   filters.AccountID,
		GroupID:     filters.GroupID,
		Model:       filters.Model,
		BillingMode: filters.BillingMode,
		RequestType: filters.RequestType,
		Stream:      filters.Stream,
		BillingType: filters.BillingType,
	})
}

// getStatsCached 命中则返回缓存,未命中则回源 usageService 并写缓存。
func (h *UsageHandler) getStatsCached(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, bool, error) {
	key := usageStatsCacheKey(filters)
	entry, hit, err := usageStatsCache.GetOrLoad(key, func() (any, error) {
		return h.usageService.GetStatsWithFilters(ctx, filters)
	})
	if err != nil {
		return nil, hit, err
	}
	stats, err := snapshotPayloadAs[*usagestats.UsageStats](entry.Payload)
	return stats, hit, err
}

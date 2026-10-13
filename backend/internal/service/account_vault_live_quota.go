package service

import (
	"context"
	"encoding/json"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"math"
	"time"
)

type VaultQuotaSnapshot struct {
	Primary   *OpenAIRateLimitWindow `json:"primary,omitempty"`
	Secondary *OpenAIRateLimitWindow `json:"secondary,omitempty"`
	FetchedAt time.Time              `json:"fetched_at"`
}

func validQuotaWindow(w *OpenAIRateLimitWindow) bool {
	return w == nil || (w.LimitWindowSeconds > 0 && w.UsedPercent >= 0 && !math.IsNaN(w.UsedPercent) && !math.IsInf(w.UsedPercent, 0))
}
func (s *AccountUsageService) queryLiveQuota(ctx context.Context, accountID int64) (*VaultQuotaSnapshot, error) {
	q, err := s.openAIQuotaService.QueryUsageSnapshot(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if q == nil || q.RateLimit == nil || (q.RateLimit.PrimaryWindow == nil && q.RateLimit.SecondaryWindow == nil) || !validQuotaWindow(q.RateLimit.PrimaryWindow) || !validQuotaWindow(q.RateLimit.SecondaryWindow) {
		return nil, infraerrors.New(502, "OPENAI_QUOTA_INVALID_RESPONSE", "额度接口未返回有效窗口，剩余额度未知")
	}
	snapshot := &VaultQuotaSnapshot{Primary: q.RateLimit.PrimaryWindow, Secondary: q.RateLimit.SecondaryWindow, FetchedAt: time.Unix(q.FetchedAt, 0).UTC()}
	// Store only quota metadata. Neither identity nor credentials are persisted here.
	if err = s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{"openai_live_quota": snapshot}); err != nil {
		return nil, infraerrors.ServiceUnavailable("OPENAI_QUOTA_STORAGE_FAILED", "实时额度已获取，但保存失败")
	}
	return snapshot, nil
}
func usageFromLiveQuota(q *VaultQuotaSnapshot, state string) *UsageInfo {
	info := &UsageInfo{Source: "active", QuotaQueryStatus: state, QuotaPrimary: q.Primary, QuotaSecondary: q.Secondary, UpdatedAt: &q.FetchedAt}
	if state == "cached" {
		info.Source = "passive"
	}
	for _, w := range []*OpenAIRateLimitWindow{q.Primary, q.Secondary} {
		if w == nil {
			continue
		}
		reset := time.Unix(w.ResetAt, 0)
		progress := &UsageProgress{Utilization: w.UsedPercent, ResetsAt: &reset, RemainingSeconds: int(w.ResetAfterSeconds)}
		switch w.LimitWindowSeconds {
		case 18000:
			info.FiveHour = progress
		case 604800:
			info.SevenDay = progress
		case 2592000:
			info.ThirtyDay = progress
		}
	}
	return info
}
func (s *AccountUsageService) getOpenAILiveUsage(ctx context.Context, account *Account, force bool) (*UsageInfo, error) {
	var cached VaultQuotaSnapshot
	var hasCached bool
	if account.Extra != nil {
		raw, _ := json.Marshal(account.Extra["openai_live_quota"])
		hasCached = json.Unmarshal(raw, &cached) == nil && !cached.FetchedAt.IsZero() &&
			(cached.Primary != nil || cached.Secondary != nil) && validQuotaWindow(cached.Primary) && validQuotaWindow(cached.Secondary)
	}
	// The account table requests many rows at once. Refreshing every old
	// snapshot here can exceed the browser's batch timeout and hide all rows,
	// even though their last successful readings are still available. Only an
	// explicit refresh (force=true) should wait for the upstream quota API.
	if !force && hasCached {
		state := "stale"
		if age := time.Since(cached.FetchedAt); age >= 0 && age < time.Minute {
			state = "cached"
		}
		return usageFromLiveQuota(&cached, state), nil
	}
	q, err := s.queryLiveQuota(ctx, account.ID)
	if err != nil {
		if hasCached && infraerrors.Code(err) != 401 && revokedCredentialError(err) == "" {
			info := usageFromLiveQuota(&cached, "stale")
			info.Error = "额度查询失败，显示上次成功查询的快照"
			return info, nil
		}
		// Upstream credential rejection is not rejection of the administrator's
		// JWT. Do not return local HTTP 401 and trigger a frontend logout/refresh.
		if infraerrors.Code(err) == 401 {
			return nil, infraerrors.New(502, "OPENAI_QUOTA_UPSTREAM_ERROR", "上游额度查询凭证失效，当前额度未知").WithMetadata(map[string]string{"upstream_status": "401", "upstream_code": revokedCredentialError(err)})
		}
		return nil, err
	}
	return usageFromLiveQuota(q, "live"), nil
}

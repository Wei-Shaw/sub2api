package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

func (s *AccountUsageService) getGeminiUsage(ctx context.Context, account *Account) (*UsageInfo, error) {
	now := time.Now()
	usage := &UsageInfo{
		UpdatedAt: &now,
	}

	if s.geminiQuotaService == nil || s.usageLogRepo == nil {
		return usage, nil
	}

	quota, ok := s.geminiQuotaService.QuotaForAccount(ctx, account)
	if !ok {
		return usage, nil
	}

	dayStart := geminiDailyWindowStart(now)
	stats, err := s.usageLogRepo.GetModelStatsWithFilters(ctx, dayStart, now, 0, 0, account.ID, 0, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get gemini usage stats failed: %w", err)
	}

	dayTotals := geminiAggregateUsage(stats)
	dailyResetAt := geminiDailyResetTime(now)

	if quota.SharedRPD > 0 {
		totalReq := dayTotals.ProRequests + dayTotals.FlashRequests
		totalTokens := dayTotals.ProTokens + dayTotals.FlashTokens
		totalCost := dayTotals.ProCost + dayTotals.FlashCost
		usage.GeminiSharedDaily = buildGeminiUsageProgress(totalReq, quota.SharedRPD, dailyResetAt, totalTokens, totalCost, now)
	} else {
		usage.GeminiProDaily = buildGeminiUsageProgress(dayTotals.ProRequests, quota.ProRPD, dailyResetAt, dayTotals.ProTokens, dayTotals.ProCost, now)
		usage.GeminiFlashDaily = buildGeminiUsageProgress(dayTotals.FlashRequests, quota.FlashRPD, dailyResetAt, dayTotals.FlashTokens, dayTotals.FlashCost, now)
	}

	minuteStart := now.Truncate(time.Minute)
	minuteResetAt := minuteStart.Add(time.Minute)
	minuteStats, err := s.usageLogRepo.GetModelStatsWithFilters(ctx, minuteStart, now, 0, 0, account.ID, 0, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get gemini minute usage stats failed: %w", err)
	}
	minuteTotals := geminiAggregateUsage(minuteStats)

	if quota.SharedRPM > 0 {
		totalReq := minuteTotals.ProRequests + minuteTotals.FlashRequests
		totalTokens := minuteTotals.ProTokens + minuteTotals.FlashTokens
		totalCost := minuteTotals.ProCost + minuteTotals.FlashCost
		usage.GeminiSharedMinute = buildGeminiUsageProgress(totalReq, quota.SharedRPM, minuteResetAt, totalTokens, totalCost, now)
	} else {
		usage.GeminiProMinute = buildGeminiUsageProgress(minuteTotals.ProRequests, quota.ProRPM, minuteResetAt, minuteTotals.ProTokens, minuteTotals.ProCost, now)
		usage.GeminiFlashMinute = buildGeminiUsageProgress(minuteTotals.FlashRequests, quota.FlashRPM, minuteResetAt, minuteTotals.FlashTokens, minuteTotals.FlashCost, now)
	}

	return usage, nil
}

func buildGeminiUsageProgress(used, limit int64, resetAt time.Time, tokens int64, cost float64, now time.Time) *UsageProgress {
	if limit <= 0 {
		return nil
	}
	utilization := (float64(used) / float64(limit)) * 100
	remainingSeconds := int(resetAt.Sub(now).Seconds())
	if remainingSeconds < 0 {
		remainingSeconds = 0
	}
	resetCopy := resetAt
	return &UsageProgress{
		Utilization:      utilization,
		ResetsAt:         &resetCopy,
		RemainingSeconds: remainingSeconds,
		UsedRequests:     used,
		LimitRequests:    limit,
		WindowStats: &WindowStats{
			Requests: used,
			Tokens:   tokens,
			Cost:     cost,
		},
	}
}

// getAntigravityUsage 获取 Antigravity 账户额度及本地 5h / 7d 窗口统计
func (s *AccountUsageService) getAntigravityUsage(ctx context.Context, account *Account, force ...bool) (*UsageInfo, error) {
	forceProbe := len(force) > 0 && force[0]
	if s.antigravityQuotaFetcher == nil || !s.antigravityQuotaFetcher.CanFetch(account) {
		now := time.Now()
		usage := &UsageInfo{UpdatedAt: &now, AntigravityQuotaSource: "unavailable"}
		s.attachAntigravityWindowStats(ctx, account, usage)
		return usage, nil
	}

	var prevCachedUsage *UsageInfo
	identity := antigravityUsageIdentity(account)
	if s.cache != nil {
		if cached, ok := s.cache.antigravityCache.Load(account.ID); ok {
			if cache, ok := cached.(*antigravityUsageCache); ok && cache != nil && cache.identity == identity {
				prevCachedUsage = cache.usageInfo
				if !forceProbe {
					ttl := antigravityCacheTTL(cache.usageInfo)
					if time.Since(cache.timestamp) < ttl {
						usage := cloneAntigravityUsageInfo(cache.usageInfo)
						recalcAntigravityRemainingSeconds(usage)
						s.attachAntigravityWindowStats(ctx, account, usage)
						return usage, nil
					}
				}
			}
		}
	}

	flightKey := fmt.Sprintf("ag-usage:%d:%x", account.ID, identity)
	result, flightErr, _ := s.cache.antigravityFlight.Do(flightKey, func() (any, error) {
		if !forceProbe && s.cache != nil {
			if cached, ok := s.cache.antigravityCache.Load(account.ID); ok {
				if cache, ok := cached.(*antigravityUsageCache); ok && cache != nil && cache.identity == identity {
					ttl := antigravityCacheTTL(cache.usageInfo)
					if time.Since(cache.timestamp) < ttl {
						usage := cloneAntigravityUsageInfo(cache.usageInfo)
						recalcAntigravityRemainingSeconds(usage)
						return usage, nil
					}
				}
			}
		}

		fetchCtx, fetchCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer fetchCancel()

		proxyURL := s.antigravityQuotaFetcher.GetProxyURL(fetchCtx, account)
		fetchResult, err := s.antigravityQuotaFetcher.FetchQuota(fetchCtx, account, proxyURL)
		if err != nil {
			degraded := buildAntigravityDegradedUsage(err)
			retainAntigravityQuotaSnapshot(prevCachedUsage, degraded)
			enrichUsageWithAccountError(degraded, account)
			if s.cache != nil {
				s.cache.antigravityCache.Store(account.ID, &antigravityUsageCache{
					usageInfo: degraded,
					timestamp: time.Now(),
					identity:  identity,
				})
			}
			return degraded, nil
		}

		if fetchResult.UsageInfo.AntigravityQuotaError != "" {
			retainAntigravityQuotaSnapshot(prevCachedUsage, fetchResult.UsageInfo)
		}
		enrichUsageWithAccountError(fetchResult.UsageInfo, account)
		if s.cache != nil {
			s.cache.antigravityCache.Store(account.ID, &antigravityUsageCache{
				usageInfo: fetchResult.UsageInfo,
				timestamp: time.Now(),
				identity:  identity,
			})
		}
		return fetchResult.UsageInfo, nil
	})

	if flightErr != nil {
		return nil, flightErr
	}
	rawUsage, ok := result.(*UsageInfo)
	if !ok || rawUsage == nil {
		now := time.Now()
		usage := &UsageInfo{UpdatedAt: &now}
		s.attachAntigravityWindowStats(ctx, account, usage)
		return usage, nil
	}
	usage := cloneAntigravityUsageInfo(rawUsage)
	recalcAntigravityRemainingSeconds(usage)
	s.attachAntigravityWindowStats(ctx, account, usage)
	return usage, nil
}

// cloneAntigravityUsageInfo copies mutable local statistics. Quota groups and
// model metadata are immutable snapshots and may be shared safely.
func cloneAntigravityUsageInfo(src *UsageInfo) *UsageInfo {
	if src == nil {
		return nil
	}
	dst := *src
	if src.FiveHour != nil {
		fh := *src.FiveHour
		if src.FiveHour.WindowStats != nil {
			ws := *src.FiveHour.WindowStats
			fh.WindowStats = &ws
		}
		dst.FiveHour = &fh
	}
	if src.SevenDay != nil {
		sd := *src.SevenDay
		if src.SevenDay.WindowStats != nil {
			ws := *src.SevenDay.WindowStats
			sd.WindowStats = &ws
		}
		dst.SevenDay = &sd
	}
	return &dst
}

// attachAntigravityWindowStats 为 Antigravity 账户附加 5 小时与 7 天本地窗口统计（req / tokens / A $ / U $）
func (s *AccountUsageService) attachAntigravityWindowStats(ctx context.Context, account *Account, usage *UsageInfo) {
	if s == nil || s.usageLogRepo == nil || account == nil || account.ID <= 0 || usage == nil {
		return
	}
	now := time.Now()
	if stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, now.Add(-5*time.Hour)); err == nil && stats != nil {
		if usage.FiveHour == nil {
			usage.FiveHour = &UsageProgress{}
		}
		usage.FiveHour.WindowStats = windowStatsFromAccountStats(stats)
	}
	if stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, now.Add(-7*24*time.Hour)); err == nil && stats != nil {
		if usage.SevenDay == nil {
			usage.SevenDay = &UsageProgress{}
		}
		usage.SevenDay.WindowStats = windowStatsFromAccountStats(stats)
	}
}

func recalcAntigravityRemainingSeconds(info *UsageInfo) {
	if info == nil {
		return
	}
	if info.FiveHour != nil && info.FiveHour.ResetsAt != nil {
		remaining := int(time.Until(*info.FiveHour.ResetsAt).Seconds())
		if remaining < 0 {
			remaining = 0
		}
		info.FiveHour.RemainingSeconds = remaining
	}
	if info.SevenDay != nil && info.SevenDay.ResetsAt != nil {
		remaining := int(time.Until(*info.SevenDay.ResetsAt).Seconds())
		if remaining < 0 {
			remaining = 0
		}
		info.SevenDay.RemainingSeconds = remaining
	}
}

func antigravityCacheTTL(info *UsageInfo) time.Duration {
	if info == nil {
		return antigravityErrorTTL
	}
	if info.IsForbidden {
		return apiCacheTTL
	}
	if info.ErrorCode != "" || info.Error != "" || info.AntigravityQuotaError != "" {
		return antigravityErrorTTL
	}
	return apiCacheTTL
}

func buildAntigravityDegradedUsage(err error) *UsageInfo {
	now := time.Now()
	errMsg := fmt.Sprintf("usage API error: %v", err)
	slog.Warn("antigravity usage fetch failed, returning degraded response", "error", err)

	info := &UsageInfo{
		UpdatedAt: &now,
		Error:     errMsg,
	}

	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "HTTP 401") ||
		strings.Contains(errStr, "UNAUTHENTICATED") ||
		strings.Contains(errStr, "invalid_grant"):
		info.ErrorCode = errorCodeUnauthenticated
		info.NeedsReauth = true
	case strings.Contains(errStr, "HTTP 429"):
		info.ErrorCode = errorCodeRateLimited
	default:
		info.ErrorCode = errorCodeNetworkError
	}

	return info
}

func enrichUsageWithAccountError(info *UsageInfo, account *Account) {
	if info == nil || account == nil || account.Status != StatusError {
		return
	}
	msg := strings.ToLower(account.ErrorMessage)
	if !strings.Contains(msg, "403") && !strings.Contains(msg, "forbidden") &&
		!strings.Contains(msg, "violation") && !strings.Contains(msg, "validation") {
		return
	}
	fbType := classifyForbiddenType(account.ErrorMessage)
	info.IsForbidden = true
	info.ForbiddenType = fbType
	info.ForbiddenReason = account.ErrorMessage
	info.NeedsVerify = fbType == forbiddenTypeValidation
	info.IsBanned = fbType == forbiddenTypeViolation
	info.ValidationURL = extractValidationURL(account.ErrorMessage)
	info.ErrorCode = errorCodeForbidden
	info.NeedsReauth = false
}

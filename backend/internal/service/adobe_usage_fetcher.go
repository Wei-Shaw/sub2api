package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
)

// adobeCreditsExhaustedFallback 是「available=0 但上游没给 availableUntil」时的
// 兜底冷却时长——半小时足够熬过一次短暂上游延迟，又不会长到把付费账号闲置。
const adobeCreditsExhaustedFallback = 30 * time.Minute

// getAdobeUsage 拉取 Adobe 账号的 credits/balance，写进 UsageInfo。
//
// 与 kiro_usage_fetcher.go:76 getKiroUsage 同结构：cache + singleflight + 错误降级。
// available <= 0 时同步 SetTempUnschedulable 到 availableUntil——这条比 handler 层
// 撞 429 后进 adobeQuotaCooldown 精准得多（免费账号第 11 次不用先撞 429 才知道）。
func (s *AccountUsageService) getAdobeUsage(ctx context.Context, account *Account, source string, forceRefresh bool) (*UsageInfo, error) {
	now := time.Now()
	if account == nil {
		return &UsageInfo{
			Source:    source,
			UpdatedAt: &now,
			Error:     "account is nil",
			ErrorCode: errorCodeNetworkError,
		}, nil
	}
	if account.Platform != domain.PlatformAdobe {
		return &UsageInfo{Source: source, UpdatedAt: &now}, nil
	}
	if isAdobeRelayAccount(account) {
		return &UsageInfo{Source: source, UpdatedAt: &now}, nil
	}

	cached, hasCached := s.getCachedAdobeUsage(account.ID)
	if hasCached && (cached.ErrorCode != "" || cached.Error != "") {
		cached.Source = source
		return cached, nil
	}
	if !forceRefresh && hasCached {
		cached.Source = source
		return cached, nil
	}

	flightKey := fmt.Sprintf("adobe-usage:%d", account.ID)
	result, fetchErr, _ := s.cache.adobeUsageFlight.Do(flightKey, func() (any, error) {
		if !forceRefresh {
			if usage, ok := s.getCachedAdobeUsage(account.ID); ok {
				return usage, nil
			}
		}
		usage, err := s.fetchAndCacheAdobeUsage(ctx, account, source)
		if err != nil {
			return nil, err
		}
		return usage, nil
	})
	if fetchErr == nil {
		if usage, ok := result.(*UsageInfo); ok && usage != nil {
			usage.Source = source
			if source == "active" {
				s.tryClearRecoverableAccountError(ctx, account)
			}
			return usage, nil
		}
	}

	degraded := buildAdobeDegradedUsage(fetchErr)
	degraded.Source = source
	if hasCached {
		cached.Error = degraded.Error
		cached.ErrorCode = degraded.ErrorCode
		cached.NeedsReauth = degraded.NeedsReauth
		cached.Source = source
		return cached, nil
	}
	s.storeAdobeUsageSnapshot(account.ID, degraded)
	return degraded, nil
}

func (s *AccountUsageService) fetchAndCacheAdobeUsage(ctx context.Context, account *Account, source string) (*UsageInfo, error) {
	token, err := resolveAdobeAccessToken(ctx, s.accountRepo, s.adobeTokenRefresher, account)
	if err != nil {
		return nil, err
	}

	client := s.adobeUsageClient(account)
	if client == nil {
		return nil, errors.New("adobe client unavailable")
	}

	bal, err := client.FetchCreditsBalance(ctx, token)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	info := &UsageInfo{
		UpdatedAt:    &now,
		AdobePlanCap: bal.PlanCap,
	}
	if progress := adobeProgressFromInt64s(bal.Total, bal.Used, bal.Available); progress != nil {
		info.AdobeCredit = progress
	}
	if len(bal.CreditPools) > 0 {
		pools := make([]AdobeCreditPool, 0, len(bal.CreditPools))
		for _, pool := range bal.CreditPools {
			pools = append(pools, AdobeCreditPool{
				Name:      pool.Name,
				Total:     pool.Total,
				Used:      pool.Used,
				Available: pool.Available,
			})
		}
		info.AdobeCreditPools = pools
	}
	if reset := parseAdobeResetTime(bal.AvailableUntil); reset != nil {
		info.AdobeCreditResetAt = reset
	}

	s.applyAdobeCreditsCooldown(ctx, account, bal, info.AdobeCreditResetAt)
	s.storeAdobeUsageSnapshot(account.ID, info)
	return info, nil
}

// applyAdobeCreditsCooldown 在 credits 耗尽时把账号临时摆出调度，直到 availableUntil。
//
// 与 handler 层的 adobeQuotaCooldown(30 min) 是互补关系：这里主动、精准；
// 那边被动、兜底。两条并存 = 即便本轮拉取失败或上游数据延迟，429 仍会摆号。
func (s *AccountUsageService) applyAdobeCreditsCooldown(
	ctx context.Context,
	account *Account,
	bal *adobe.CreditsBalance,
	resetAt *time.Time,
) {
	if s == nil || s.accountRepo == nil || account == nil || bal == nil {
		return
	}
	if bal.Available == nil || *bal.Available > 0 {
		return
	}
	until := time.Now().Add(adobeCreditsExhaustedFallback)
	if resetAt != nil && resetAt.After(time.Now()) {
		until = *resetAt
	}
	_ = s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, "adobe_credits_exhausted")
}

func (s *AccountUsageService) storeAdobeUsageSnapshot(accountID int64, usage *UsageInfo) {
	if s == nil || s.cache == nil || accountID <= 0 || usage == nil {
		return
	}
	now := time.Now()
	if usage.UpdatedAt == nil {
		usage.UpdatedAt = &now
	}
	s.cache.adobeUsageCache.Store(accountID, &adobeUsageCache{
		usageInfo: cloneUsageInfo(usage),
		timestamp: now,
	})
}

func (s *AccountUsageService) getCachedAdobeUsage(accountID int64) (*UsageInfo, bool) {
	if s == nil || s.cache == nil || accountID <= 0 {
		return nil, false
	}
	cached, ok := s.cache.adobeUsageCache.Load(accountID)
	if !ok {
		return nil, false
	}
	cache, ok := cached.(*adobeUsageCache)
	if !ok || cache == nil || cache.usageInfo == nil {
		return nil, false
	}
	if time.Since(cache.timestamp) >= adobeCacheTTL(cache.usageInfo) {
		return nil, false
	}
	return cloneUsageInfo(cache.usageInfo), true
}

func adobeCacheTTL(info *UsageInfo) time.Duration {
	if info == nil {
		return adobeUsageErrorTTL
	}
	if info.ErrorCode != "" || info.Error != "" {
		return adobeUsageErrorTTL
	}
	return apiCacheTTL
}

// buildAdobeDegradedUsage 把 pkg 抛出的类型化错误映射成 UsageInfo 的错误码。
// 与 Kiro 侧同码位，方便前端复用错误处理。
func buildAdobeDegradedUsage(err error) *UsageInfo {
	now := time.Now()
	info := &UsageInfo{
		UpdatedAt: &now,
		Error:     "",
		ErrorCode: errorCodeNetworkError,
	}
	if err == nil {
		return info
	}
	info.Error = err.Error()
	var (
		authErr     *adobe.AuthError
		quotaErr    *adobe.QuotaExhaustedError
		entitledErr *adobe.NotEntitledError
		tempErr     *adobe.UpstreamTemporaryError
	)
	switch {
	case errors.As(err, &authErr):
		info.ErrorCode = errorCodeUnauthenticated
		info.NeedsReauth = true
	case errors.As(err, &quotaErr):
		info.ErrorCode = errorCodeRateLimited
	case errors.As(err, &entitledErr):
		// 套餐不够不是 cookie 坏了，不能标 NeedsReauth 去诱导重导。
		info.ErrorCode = errorCodeNetworkError
	case errors.As(err, &tempErr):
		info.ErrorCode = errorCodeNetworkError
	default:
		info.ErrorCode = errorCodeNetworkError
	}
	return info
}

// adobeUsageClients 是用量查询常驻的 TLS 客户端池。缓存未命中时仍要出站，
// 每次 new adobeClientCache 会丢掉连接复用。不和出图服务共用：用量查询不该
// 继承出图过程中被 IMS Set-Cookie 改过的 jar。
var adobeUsageClients = &adobeClientCache{}

// adobeUsageClientFactory 让测试能替换掉真实的 adobe.Client。
var adobeUsageClientFactory = func(account *Account) adobeCreditsClient {
	if account == nil {
		return nil
	}
	return adobeUsageClients.clientForAccount(account)
}

// adobeCreditsClient 只暴露 credits/balance 一条方法——足以让测试用一个纯 fake 替换。
type adobeCreditsClient interface {
	FetchCreditsBalance(ctx context.Context, accessToken string) (*adobe.CreditsBalance, error)
}

func (s *AccountUsageService) adobeUsageClient(account *Account) adobeCreditsClient {
	return adobeUsageClientFactory(account)
}

// adobeProgressFromInt64s 把 total/used/available 三元组打成 CreditProgress，
// 缺任何一项就返回 nil——避免前端看到「用了 0 / 上限 0」这种误导。
func adobeProgressFromInt64s(total, used, available *int64) *CreditProgress {
	if total == nil || used == nil || available == nil {
		return nil
	}
	limit := float64(*total)
	current := float64(*used)
	percent := 0.0
	if limit > 0 {
		percent = math.Min(100, math.Max(0, (current/limit)*100))
	}
	return &CreditProgress{
		CurrentUsage:   current,
		UsageLimit:     limit,
		PercentageUsed: percent,
	}
}

// parseAdobeResetTime 解析 availableUntil，兼容 RFC3339 与 unix 秒/毫秒。
// 无法识别时返回 nil——比给一个错的时间更好。
func parseAdobeResetTime(raw any) *time.Time {
	switch v := raw.(type) {
	case string:
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999Z"} {
				if t, err := time.Parse(layout, trimmed); err == nil {
					return &t
				}
			}
		}
	case float64:
		return unixishFloatToTime(v)
	case int64:
		return unixishToTime(v)
	case int:
		return unixishToTime(int64(v))
	}
	return nil
}

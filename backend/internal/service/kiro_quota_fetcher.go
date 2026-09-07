package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// kiroCreditsExhaustedKey 是 model_rate_limits 中标记 credits 耗尽的特殊 key。
// 与 Antigravity 的 "AICredits" 并列，各平台独立。
const kiroCreditsExhaustedKey = "KiroCredits"

// kiroCreditsFallbackCooldown 是拿不到可信重置时间时的保守冷却窗口。
const kiroCreditsFallbackCooldown = time.Hour

// kiroQuotaTimeout 是额度查询的超时。
const kiroQuotaTimeout = 20 * time.Second

// kiroQuotaBodyLimit 限制额度响应的读取量。
const kiroQuotaBodyLimit = 1 << 20

// KiroQuotaFetcher 实现 QuotaFetcher，通过 getUsageLimits 拉取账号额度。
type KiroQuotaFetcher struct {
	// qHostFor 可被测试替换以指向 httptest.Server。
	qHostFor func(account *Account) string
}

// NewKiroQuotaFetcher 创建额度获取器。
func NewKiroQuotaFetcher() *KiroQuotaFetcher {
	return &KiroQuotaFetcher{
		qHostFor: func(account *Account) string {
			return fmt.Sprintf("https://q.%s.amazonaws.com", account.KiroRegion())
		},
	}
}

// CanFetch 判断账号是否具备查询额度的凭证。
func (f *KiroQuotaFetcher) CanFetch(account *Account) bool {
	if account == nil || account.Platform != PlatformKiro {
		return false
	}
	return strings.TrimSpace(account.KiroBearerToken()) != ""
}

// fetchUsageLimits 发起 getUsageLimits 请求并解析响应，不做 UsageInfo 映射——
// 单独拆出来是因为 finishWithAction（kiro_gateway_service.go）的 credits
// 冷却逻辑也需要原始的 *kiro.UsageLimits（取 AgenticRequest() 拿到
// nextDateReset），而不是映射后的服务层 UsageInfo。同时把响应体原样带出，
// 供 FetchQuota 填充 QuotaResult.Raw，避免调用两次上游。
func (f *KiroQuotaFetcher) fetchUsageLimits(ctx context.Context, account *Account, proxyURL string) (*kiro.UsageLimits, []byte, error) {
	endpoint := kiro.BuildUsageLimitsURL(f.qHostFor(account), account.KiroProfileArn())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	// 复用转发路径的指纹头，避免额度探测与转发呈现两种客户端形态。
	machineID, _ := EnsureKiroMachineID(account.Credentials)
	req.Header = kiro.BuildHeaders(kiro.HeaderOptions{
		Endpoint:    kiro.Endpoint{Origin: "AI_EDITOR"},
		BearerToken: account.KiroBearerToken(),
		MachineID:   machineID,
		IsAPIKey:    account.IsKiroAPIKeyAccount(),
		Profile:     kiro.DefaultClientProfile(),
	})

	hc, err := httpclient.GetClient(httpclient.Options{ProxyURL: proxyURL, Timeout: kiroQuotaTimeout})
	if err != nil {
		return nil, nil, err
	}

	resp, err := hc.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, kiroQuotaBodyLimit))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("kiro: getUsageLimits returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	limits, err := kiro.ParseUsageLimits(body)
	if err != nil {
		return nil, nil, err
	}

	return limits, body, nil
}

// FetchQuota 查询并映射账号额度。
func (f *KiroQuotaFetcher) FetchQuota(ctx context.Context, account *Account, proxyURL string) (*QuotaResult, error) {
	if !f.CanFetch(account) {
		return nil, fmt.Errorf("kiro: account %d has no usable credential for quota lookup", account.ID)
	}

	limits, body, err := f.fetchUsageLimits(ctx, account, proxyURL)
	if err != nil {
		return nil, err
	}

	return &QuotaResult{
		UsageInfo: kiroUsageInfo(limits),
		Raw:       rawJSONMap(body),
	}, nil
}

// rawJSONMap 把响应体解析成通用 map，失败时返回 nil（不阻断主流程）。
func rawJSONMap(body []byte) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil
	}
	return m
}

// kiroUsageInfo 把解析结果映射成账号额度视图。
func kiroUsageInfo(limits *kiro.UsageLimits) *UsageInfo {
	now := time.Now()
	info := &UsageInfo{
		Source:                "active",
		UpdatedAt:             &now,
		KiroSubscriptionTitle: limits.SubscriptionTitle,
		KiroOverageStatus:     limits.OverageStatus,
	}

	b := limits.AgenticRequest()
	if b == nil {
		return info
	}

	progress := &UsageProgress{
		Utilization:   b.UtilizationPercent(),
		UsedRequests:  int64(b.CurrentUsage),
		LimitRequests: int64(b.EffectiveLimit()),
		ResetsAt:      b.NextDateReset,
	}
	if b.NextDateReset != nil {
		if remaining := int(time.Until(*b.NextDateReset).Seconds()); remaining > 0 {
			progress.RemainingSeconds = remaining
		}
	}
	info.KiroCredits = progress
	return info
}

// kiroCreditsCooldownUntil 返回 credits 耗尽时应冷却到的时间点。
//
// 优先用上游给出的真实 nextDateReset（比 Antigravity 的固定 5 小时准确）；
// 缺失或已过期时退回保守窗口 —— 直接用过期时间会导致立刻解冻并反复打上游。
func kiroCreditsCooldownUntil(b *kiro.UsageBreakdown, now time.Time) (time.Time, bool) {
	if b == nil || !b.Exhausted() {
		return time.Time{}, false
	}
	if b.NextDateReset != nil && b.NextDateReset.After(now) {
		return *b.NextDateReset, true
	}
	return now.Add(kiroCreditsFallbackCooldown), true
}

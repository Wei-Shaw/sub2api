package service

// AccountCodingPlanQuotaService 定期为国产 coding-plan 账号刷新套餐配额快照。
//
// 这些账号在库里以 platform=openai + type=apikey + credentials.base_url=厂商URL
// 形态存在，并通过 Extra["coding_plan_provider"]（deepseek/glm/kimi）标识厂商。
// 本服务复用 channel monitor 的 coding-plan 探针（QueryCodingPlanQuota），
// 把配额快照翻译成 account.Extra 的一组 coding_plan_* 键（供账号页展示），
// 并在窗口配额耗尽时写 temp_unschedulable_until 实现自动冷却——
// Account.IsSchedulable 会据此排除该账号，窗口 ResetsAt 到期后自动还原。
//
// 与 channel monitor 的区别：monitor 是独立探针（按 monitor 行配置，仅展示）；
// 本服务按 account 探针（per-account proxy/凭据），并驱动调度冷却。

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// CodingPlanQuotaAccountStore 是账号配额刷新服务依赖的账号存储子集（依赖倒置，
// 避免扩充庞大的 AccountRepository 接口及其全部测试 stub）。*accountRepository 天然满足。
type CodingPlanQuotaAccountStore interface {
	ListCodingPlanQuotaAccounts(ctx context.Context) ([]*Account, error)
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
	SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error
}

// AccountCodingPlanQuotaService 周期性刷新 coding-plan 账号配额。
type AccountCodingPlanQuotaService struct {
	repo     CodingPlanQuotaAccountStore
	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewAccountCodingPlanQuotaService 构造服务。interval<=0 时 Start 为空操作，
// 便于在 wire 层用功能开关关闭。
func NewAccountCodingPlanQuotaService(repo CodingPlanQuotaAccountStore, interval time.Duration) *AccountCodingPlanQuotaService {
	return &AccountCodingPlanQuotaService{
		repo:     repo,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (s *AccountCodingPlanQuotaService) Start() {
	if s == nil || s.repo == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *AccountCodingPlanQuotaService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

const (
	// codingPlanQuotaPerAccountTimeout 单个账号探针总超时。
	codingPlanQuotaPerAccountTimeout = 20 * time.Second
	// codingPlanCooldownReason 写入 temp_unschedulable_reason 的标识。
	codingPlanCooldownReason = "coding_plan_exhausted"
	// codingPlanExhaustedUtilization 视为耗尽的利用率阈值（含，百分比）。
	codingPlanExhaustedUtilization = 100.0
)

func (s *AccountCodingPlanQuotaService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	accounts, err := s.repo.ListCodingPlanQuotaAccounts(ctx)
	if err != nil {
		slog.Warn("coding_plan_quota: list accounts failed", "error", err)
		return
	}
	now := time.Now()
	for _, account := range accounts {
		if account == nil {
			continue
		}
		s.refreshOne(account, now)
	}
}

// refreshOne 探针单个账号并回写 Extra / 冷却。失败只记日志，不覆盖既有快照。
func (s *AccountCodingPlanQuotaService) refreshOne(account *Account, now time.Time) {
	provider := account.CodingPlanProvider()
	if provider == "" {
		return
	}
	apiKey := account.GetCredential("api_key")
	if apiKey == "" {
		slog.Debug("coding_plan_quota: skip account without api_key", "account_id", account.ID)
		return
	}
	endpoint := account.GetCredential("base_url")
	if endpoint == "" {
		endpoint = codingPlanDefaultEndpoint(provider)
	}

	client := newCodingPlanAccountHTTPClient(accountProxyURL(account))
	defer client.CloseIdleConnections()

	ctx, cancel := context.WithTimeout(context.Background(), codingPlanQuotaPerAccountTimeout)
	defer cancel()

	snapshot, errMsg, authErr := QueryCodingPlanQuota(ctx, client, provider, endpoint, apiKey)
	if errMsg != "" {
		if authErr {
			slog.Warn("coding_plan_quota: auth failed", "account_id", account.ID, "provider", provider, "msg", errMsg)
		} else {
			slog.Debug("coding_plan_quota: probe failed", "account_id", account.ID, "provider", provider, "msg", errMsg)
		}
		return
	}

	updates, exhaustedResetAt := codingPlanExtraUpdates(provider, snapshot, now)
	if err := s.repo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("coding_plan_quota: update extra failed", "account_id", account.ID, "error", err)
		return
	}

	// 窗口配额耗尽（GLM/Kimi 的 5h/周）→ 冷却到最早重置时间。DeepSeek 余额耗尽
	// 无 reset（需充值），不在此处做时间冷却——其请求失败会走既有 429 冷却链路。
	//
	// 这里只做"硬冷却"（耗尽即排除）。未实现"软降权"（接近耗尽时降低调度权重）：
	// 这些账号经 GatewayService.SelectAccountWithLoadAwareness（generic 调度）选号，
	// 该路径按 priority→最低负载→LRU 选择，无 quota-headroom 维度（仅 OpenAI 专用
	// 调度器 openAIQuotaHeadroomFactor 有，且不经过本路径）。在 generic 热路径新增
	// 配额权重属高风险改动、收益有限，故暂不实现；硬冷却已能让耗尽账号被排除。
	if exhaustedResetAt != nil && exhaustedResetAt.After(now) {
		if account.TempUnschedulableUntil == nil || account.TempUnschedulableUntil.Before(*exhaustedResetAt) {
			if err := s.repo.SetTempUnschedulable(ctx, account.ID, *exhaustedResetAt, codingPlanCooldownReason); err != nil {
				slog.Warn("coding_plan_quota: set temp unschedulable failed", "account_id", account.ID, "error", err)
			}
		}
	}
}

// codingPlanExtraUpdates 把配额快照翻译成 account.Extra 更新，并返回耗尽窗口的
// 最早未来重置时间（nil 表示无窗口耗尽）。
func codingPlanExtraUpdates(provider string, s *MonitorQuotaSnapshot, now time.Time) (map[string]any, *time.Time) {
	updates := make(map[string]any)
	updates["coding_plan_provider"] = provider
	updates["coding_plan_usage_updated_at"] = now.UTC().Format(time.RFC3339)
	if s.PlanLevel != "" {
		updates["coding_plan_plan_level"] = s.PlanLevel
	}
	if s.Available != nil {
		updates["coding_plan_available"] = *s.Available
	}

	var earliestExhaustedReset *time.Time
	for _, t := range s.Tiers {
		switch t.Name {
		case QuotaTierFiveHour:
			if t.Utilization != nil {
				updates["coding_plan_5h_used_percent"] = *t.Utilization
			}
			if t.ResetsAt != nil {
				updates["coding_plan_5h_reset_at"] = t.ResetsAt.UTC().Format(time.RFC3339)
				if t.Utilization != nil && *t.Utilization >= codingPlanExhaustedUtilization {
					earliestExhaustedReset = earliestFutureReset(earliestExhaustedReset, t.ResetsAt, now)
				}
			}
		case QuotaTierWeekly:
			if t.Utilization != nil {
				updates["coding_plan_weekly_used_percent"] = *t.Utilization
			}
			if t.ResetsAt != nil {
				updates["coding_plan_weekly_reset_at"] = t.ResetsAt.UTC().Format(time.RFC3339)
				if t.Utilization != nil && *t.Utilization >= codingPlanExhaustedUtilization {
					earliestExhaustedReset = earliestFutureReset(earliestExhaustedReset, t.ResetsAt, now)
				}
			}
		case QuotaTierBalance:
			// 多币种取首条作为展示用的主余额。
			if _, ok := updates["coding_plan_balance"]; ok {
				continue
			}
			if t.Balance != nil {
				updates["coding_plan_balance"] = *t.Balance
			}
			if t.Currency != nil {
				updates["coding_plan_currency"] = *t.Currency
			}
		}
	}

	exhausted := earliestExhaustedReset != nil
	if s.Available != nil && !*s.Available {
		exhausted = true
	}
	updates["coding_plan_exhausted"] = exhausted
	return updates, earliestExhaustedReset
}

// earliestFutureReset 返回 base 与 candidate 中更早的未来时间；仅 candidate 在未来时参与。
func earliestFutureReset(base, candidate *time.Time, now time.Time) *time.Time {
	if candidate == nil || !candidate.After(now) {
		return base
	}
	if base == nil || candidate.Before(*base) {
		return candidate
	}
	return base
}

// codingPlanDefaultEndpoint 返回厂商默认 endpoint（账号未设 base_url 时兜底）。
func codingPlanDefaultEndpoint(provider string) string {
	switch provider {
	case MonitorProviderDeepseek:
		return MonitorDefaultDeepseekEndpoint
	case MonitorProviderGLM:
		return MonitorDefaultGLMEndpoint
	case MonitorProviderKimi:
		return MonitorDefaultKimiEndpoint
	default:
		return ""
	}
}

// accountProxyURL 返回账号绑定的代理 URL（无代理返回空串）。
func accountProxyURL(account *Account) string {
	if account == nil || account.Proxy == nil {
		return ""
	}
	return account.Proxy.URL()
}

// newCodingPlanAccountHTTPClient 构建一个尊重账号代理的 HTTP client。
// 复用 monitor 的 SSRF-safe dialer 与超时配置；仅当提供合法代理 URL 时启用代理。
func newCodingPlanAccountHTTPClient(proxyURL string) *http.Client {
	tr := &http.Transport{
		DialContext:           safeDialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          4,
		IdleConnTimeout:       monitorIdleConnTimeout,
		TLSHandshakeTimeout:   monitorTLSHandshakeTimeout,
		ResponseHeaderTimeout: monitorResponseHeaderTimeout,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Timeout: monitorRequestTimeout, Transport: tr}
}

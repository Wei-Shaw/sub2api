package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/gjson"
)

// Cline 积分余额与 ClinePass 用量来自账号接口（Cline 控制台与客户端同源），账号 API Key
// 可直接访问（Bearer <key>；实测带 workos: 前缀反而 401）：
//
//	GET /api/v1/users/me                     -> {id, email, ...}
//	GET /api/v1/users/{id}/balance           -> {balance, userId}
//	GET /api/v1/users/me/plan/usage-limits   -> {limits:[{type, percentUsed, resetsAt?}]}
//
// 响应包在 {success, error, data} 信封里。balance 单位为百万分之一积分（1 积分 =
// 1 美元；Cline CLI 的 normalizeCreditBalance 除以 1e6，扩展先除 100 再除 10000），
// 按 Cline 的展示精度保留 4 位小数。usage-limits 的 type 为 five_hour / weekly /
// monthly（套餐里对应最近 5 小时 / 7 天 / 30 天的滚动用量上限），percentUsed 为 0-100，
// 窗口未开始时没有 resetsAt；未订阅时返回 404（no plan history found for user）或空列表。

const clineAPIBase = "https://" + clineAPIHost

var errClineInvalidResponse = errors.New("invalid cline account response")

// clineHTTPError 是账号接口的非 2xx 响应。
type clineHTTPError struct {
	status int
	body   string
}

func (e *clineHTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.status, e.body)
}

type clineAccountClient struct {
	cfg          *config.Config
	httpUpstream HTTPUpstream
	account      *Account
	apiKey       string
	proxyURL     string
	// errPrefix 是应用错误码前缀（CN_BALANCE / CN_QUOTA），与调用方服务的错误码一致。
	errPrefix string
}

func newClineAccountClient(cfg *config.Config, httpUpstream HTTPUpstream, account *Account, proxyURL, errPrefix string) *clineAccountClient {
	return &clineAccountClient{
		cfg:          cfg,
		httpUpstream: httpUpstream,
		account:      account,
		apiKey:       strings.TrimSpace(account.GetCNAPIKey()),
		proxyURL:     proxyURL,
		errPrefix:    errPrefix,
	}
}

// get 请求一个账号接口，返回拆掉 {success, error, data} 信封后的 JSON。
func (c *clineAccountClient) get(ctx context.Context, path string) (gjson.Result, error) {
	validatedURL, err := cnValidateProbeURL(c.cfg, clineAPIBase+path)
	if err != nil {
		return gjson.Result{}, infraerrors.New(http.StatusForbidden, c.errPrefix+"_URL_REJECTED", err.Error())
	}
	callCtx, cancel := context.WithTimeout(ctx, cnQuotaUpstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, validatedURL, nil)
	if err != nil {
		return gjson.Result{}, infraerrors.Newf(http.StatusInternalServerError, c.errPrefix+"_REQUEST_BUILD_FAILED", "build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	c.account.ApplyHeaderOverrides(req.Header)

	resp, err := c.httpUpstream.Do(req, c.proxyURL, c.account.ID, maxInt(c.account.Concurrency, 1))
	if err != nil {
		return gjson.Result{}, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, cnQuotaMaxBodyBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return gjson.Result{}, &clineHTTPError{status: resp.StatusCode, body: truncate(strings.TrimSpace(string(body)), 240)}
	}
	if !gjson.ValidBytes(body) {
		return gjson.Result{}, fmt.Errorf("%w: %s", errClineInvalidResponse, path)
	}
	parsed := gjson.ParseBytes(body)
	if success := parsed.Get("success"); success.Exists() {
		if !success.Bool() {
			return gjson.Result{}, &clineHTTPError{status: resp.StatusCode, body: truncate(parsed.Get("error").String(), 240)}
		}
		return parsed.Get("data"), nil
	}
	return parsed, nil
}

// balance 查询积分余额（美元）。
func (c *clineAccountClient) balance(ctx context.Context) (float64, error) {
	me, err := c.get(ctx, "/api/v1/users/me")
	if err != nil {
		return 0, err
	}
	userID := strings.TrimSpace(me.Get("id").String())
	if userID == "" {
		return 0, fmt.Errorf("%w: missing user id", errClineInvalidResponse)
	}
	balance, err := c.get(ctx, "/api/v1/users/"+url.PathEscape(userID)+"/balance")
	if err != nil {
		return 0, err
	}
	micro := balance.Get("balance")
	if !micro.Exists() || (micro.Type != gjson.Number && micro.Type != gjson.String) {
		return 0, fmt.Errorf("%w: missing balance", errClineInvalidResponse)
	}
	return math.Round(micro.Float()/100) / 10000, nil
}

// passLimits 查询 ClinePass 三档窗口；subscribed=false 表示没有订阅。
func (c *clineAccountClient) passLimits(ctx context.Context) (tiers []CNQuotaTier, subscribed bool, err error) {
	data, err := c.get(ctx, "/api/v1/users/me/plan/usage-limits")
	if err != nil {
		var httpErr *clineHTTPError
		if errors.As(err, &httpErr) && httpErr.status == http.StatusNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	limits := data.Get("limits")
	if !limits.IsArray() {
		return nil, false, fmt.Errorf("%w: missing limits", errClineInvalidResponse)
	}
	windows := map[string]string{"five_hour": "5h", "weekly": "weekly", "monthly": "monthly"}
	for _, item := range limits.Array() {
		window, ok := windows[strings.ToLower(strings.TrimSpace(item.Get("type").String()))]
		used := item.Get("percentUsed")
		if !ok || used.Type != gjson.Number {
			continue
		}
		tier := CNQuotaTier{Window: window, UsedPercent: used.Float()}
		if resetAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(item.Get("resetsAt").String())); err == nil {
			tier.ResetAt = resetAt.UTC().Format(time.RFC3339)
		}
		tiers = append(tiers, tier)
	}
	return tiers, len(tiers) > 0, nil
}

// clineFetchFailure 把取数错误转换为探测失败的状态码与文案；ok=false 表示应作为错误返回。
func clineFetchFailure(err error) (status int, message string, ok bool) {
	var httpErr *clineHTTPError
	switch {
	case errors.As(err, &httpErr):
		return httpErr.status, fmt.Sprintf("API error (HTTP %d): %s", httpErr.status, httpErr.body), true
	case errors.Is(err, errClineInvalidResponse):
		return http.StatusOK, "Invalid response", true
	default:
		return 0, "", false
	}
}

func clineApplicationError(err error, errPrefix string) error {
	if isApplicationError(err) {
		return err
	}
	return infraerrors.Newf(http.StatusBadGateway, errPrefix+"_REQUEST_FAILED", "%v", err)
}

func clineBalanceResult(usd float64, now time.Time) *CNProviderBalanceResult {
	return &CNProviderBalanceResult{
		Provider:   PlatformCline,
		Success:    true,
		Balance:    usd,
		Currency:   "USD",
		Balances:   []CNProviderBalanceEntry{{Currency: "USD", Balance: usd}},
		Available:  true,
		StatusCode: http.StatusOK,
		FetchedAt:  now.Unix(),
	}
}

// queryClineBalance 只查积分余额（美元）并落余额快照。
func (s *CNProviderBalanceService) queryClineBalance(ctx context.Context, account *Account) (*CNProviderBalanceResult, error) {
	client := newClineAccountClient(s.cfg, s.httpUpstream, account, s.resolveProxyURL(ctx, account), "CN_BALANCE")
	if client.apiKey == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_BALANCE_NO_APIKEY", "account api_key is empty")
	}
	now := time.Now().UTC()
	usd, err := client.balance(ctx)
	if err != nil {
		status, message, ok := clineFetchFailure(err)
		if !ok {
			return nil, clineApplicationError(err, "CN_BALANCE")
		}
		return &CNProviderBalanceResult{Provider: PlatformCline, StatusCode: status, FetchedAt: now.Unix(), Available: true,
			Error: "Balance query failed: " + message}, nil
	}
	result := clineBalanceResult(usd, now)
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, cnBalanceExtraUpdates(PlatformCline, result, now)); err != nil {
		slog.Warn("cn_balance_persist_failed", "account_id", account.ID, "provider", PlatformCline, "error", err)
	} else {
		result.Persisted = true
	}
	return result, nil
}

// queryClineUsage 查询 ClinePass 窗口与积分余额，落快照，并按结果设置或解除两个钱包的
// 冷却（见 applyClineWallets）。余额查询失败不影响窗口结果。
func (s *CNProviderQuotaService) queryClineUsage(ctx context.Context, account *Account) (*CNProviderQuotaProbeResult, error) {
	client := newClineAccountClient(s.cfg, s.httpUpstream, account, s.resolveProxyURL(ctx, account), "CN_QUOTA")
	if client.apiKey == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NO_APIKEY", "account api_key is empty")
	}
	now := time.Now().UTC()
	result := &CNProviderQuotaProbeResult{
		Provider:  PlatformCline,
		Source:    "cline_usage_limits",
		FetchedAt: now.Unix(),
	}
	tiers, subscribed, err := client.passLimits(ctx)
	if err != nil {
		status, message, ok := clineFetchFailure(err)
		if !ok {
			return nil, clineApplicationError(err, "CN_QUOTA")
		}
		result.StatusCode = status
		result.Error = message
		result.CredentialValid = status != http.StatusUnauthorized && status != http.StatusForbidden
		return result, nil
	}
	result.StatusCode = http.StatusOK
	result.Success = true
	result.CredentialValid = true
	result.Tiers = tiers
	if subscribed {
		result.PlanLevel = "ClinePass"
	}

	updates := cnQuotaExtraUpdates(PlatformCline, tiers, now)
	present := make(map[string]CNQuotaTier, len(tiers))
	for _, tier := range tiers {
		present[tier.Window] = tier
	}
	// 缺席的窗口写 nil：取消订阅后不沿用旧快照（clinePassSubscribed 据此判断）；窗口尚未
	// 开始时没有重置时间，同样清掉上一次的值。
	for _, window := range []struct{ name, used, reset string }{
		{"5h", cnExtraSuffix5hUsed, cnExtraSuffix5hReset},
		{"weekly", cnExtraSuffixWeeklyUsed, cnExtraSuffixWeeklyReset},
		{"monthly", cnExtraSuffixMonthlyUsed, cnExtraSuffixMonthlyReset},
	} {
		tier, ok := present[window.name]
		if !ok {
			updates[cnExtraKey(PlatformCline, window.used)] = nil
		}
		if tier.ResetAt == "" {
			updates[cnExtraKey(PlatformCline, window.reset)] = nil
		}
	}
	if usd, err := client.balance(ctx); err != nil {
		slog.Warn("cline_balance_probe_failed", "account_id", account.ID, "error", err)
	} else {
		result.Balance = clineBalanceResult(usd, now)
		for key, value := range cnBalanceExtraUpdates(PlatformCline, result.Balance, now) {
			updates[key] = value
		}
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("cn_quota_persist_failed", "account_id", account.ID, "provider", PlatformCline, "error", err)
	} else {
		result.Persisted = true
		if result.Balance != nil {
			result.Balance.Persisted = true
		}
	}
	s.applyClineWallets(ctx, account, subscribed, tiers, result.Balance, now)
	return result, nil
}

const (
	clineCreditsLowReason = "cline_credits_low"
	// clinePassNoSubscriptionReason 是探测发现未订阅时的原因；与请求侧的未订阅 / 组织账户
	// 共用 clinePassUnavailableReason 前缀，单独区分出探测能证明已恢复的这一种。
	clinePassNoSubscriptionReason = clinePassUnavailableReason + ": no ClinePass subscription"
)

// applyClineWallets 按探测结果设置或解除两个钱包的冷却：
//   - ClinePass：未订阅，或有窗口用满（≥100%）时冷却到该窗口的重置时间；可用时只解除
//     窗口用满与未订阅的冷却（组织账户、所需订阅档位与推理额度上限反映不到用量接口里，
//     仍等到期后由请求重新确认）；
//   - 积分：余额低于阈值时冷却，恢复后只解除积分类原因的冷却（组织花费上限无从探测，
//     仍等到期后由请求重新确认）。
//
// 没有重置时间时冷却 2 个检测周期，由下一次探测续期或解除。
func (s *CNProviderQuotaService) applyClineWallets(ctx context.Context, account *Account, subscribed bool, tiers []CNQuotaTier, balance *CNProviderBalanceResult, now time.Time) {
	fallback := now.Add(cnBalanceCheckCooldown(s.cfg))

	switch until, reason := clinePassWalletState(subscribed, tiers, now, fallback); {
	case until != nil:
		s.setClineWalletLimit(ctx, account, clinePassRateLimitKey, *until, reason, now)
	case account.isRateLimitActiveForKey(clinePassRateLimitKey):
		reason := account.modelRateLimitReason(clinePassRateLimitKey)
		if strings.HasPrefix(reason, clinePassLimitReason) || strings.HasPrefix(reason, clinePassNoSubscriptionReason) {
			s.setClineWalletLimit(ctx, account, clinePassRateLimitKey, now, "", now)
		}
	}

	if balance == nil {
		return
	}
	threshold := 0.0
	if s.cfg != nil {
		threshold = s.cfg.Gateway.CNProviders.BalanceThreshold
	}
	switch low := balance.Balance <= 0 || balance.Balance < threshold; {
	case low:
		s.setClineWalletLimit(ctx, account, clineCreditsRateLimitKey, fallback,
			fmt.Sprintf("%s: balance %.4g USD below threshold %.2f", clineCreditsLowReason, balance.Balance, threshold), now)
	case account.isRateLimitActiveForKey(clineCreditsRateLimitKey):
		reason := account.modelRateLimitReason(clineCreditsRateLimitKey)
		if strings.HasPrefix(reason, clineCreditsReason) || strings.HasPrefix(reason, clineCreditsLowReason) {
			s.setClineWalletLimit(ctx, account, clineCreditsRateLimitKey, now, "", now)
		}
	}
}

// clinePassWalletState 返回 ClinePass 钱包应冷却到的时间；nil 表示可用。
func clinePassWalletState(subscribed bool, tiers []CNQuotaTier, now, fallback time.Time) (*time.Time, string) {
	if !subscribed {
		return &fallback, clinePassNoSubscriptionReason
	}
	var until *time.Time
	var exhausted []string
	for _, tier := range tiers {
		if tier.UsedPercent < 100 {
			continue
		}
		exhausted = append(exhausted, tier.Window)
		resetAt := fallback
		if parsed := parseSchedulingResetAt(tier.ResetAt); parsed != nil && parsed.After(now) && parsed.Sub(now) <= clinePassMaxCooldown {
			resetAt = *parsed
		}
		if until == nil || resetAt.After(*until) {
			until = &resetAt
		}
	}
	if until == nil {
		return nil, ""
	}
	return until, clinePassLimitReason + ": " + strings.Join(exhausted, ", ") + " window used up"
}

// setClineWalletLimit 写钱包冷却；until 不晚于 now 时即解除。
func (s *CNProviderQuotaService) setClineWalletLimit(ctx context.Context, account *Account, scope string, until time.Time, reason string, now time.Time) {
	setAccountModelRateLimitSnapshot(account, scope, until, reason, now)
	if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, scope, until, reason); err != nil {
		slog.Warn("cline_wallet_limit_set_failed", "account_id", account.ID, "scope", scope, "error", err)
		return
	}
	slog.Info("cline_wallet_limit", "account_id", account.ID, "scope", scope, "until", until.UTC(), "cleared", !until.After(now))
}

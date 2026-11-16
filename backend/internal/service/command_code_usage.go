package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Command Code 的积分与用量窗口来自 /alpha 下四个只读接口（官方 CLI /usage 同源，
// 社区插件实测复核），全部使用账号 API Key（Bearer）：
//
//	GET /alpha/whoami                          -> {user, org}；个人账户 org 为 null
//	GET /alpha/billing/credits[?orgId=]        -> {credits:{monthlyCredits, purchasedCredits, freeCredits},
//	                                               windowLimits:{limited, fiveHour, weekly}}
//	GET /alpha/billing/subscriptions[?orgId=]  -> {data:{planId, currentPeriodStart, currentPeriodEnd}}
//	GET /alpha/usage/summary[?orgId=][&since=] -> {totalCost, totalMonthlyCredits}
//
// 积分单位为美元。windowLimits 与 credits 平级；fiveHour / weekly 的 used、cap 为积分，
// resetAt 为 epoch 毫秒；limited=false（按量计费）时没有滚动窗口。月度窗口上游只给
// 剩余量（credits.monthlyCredits），上限按「本周期已用 + 剩余」还原。个人账户不能带
// orgId 参数（空值返回 400），只有组织账户才带。
//
// 订阅套餐的滚动窗口只限制套餐内积分：有充值积分时请求跳过窗口检查，此时窗口快照
// 不参与阈值停调与 429 冷却（见 commandCodeWindowsBinding），已有的窗口冷却在探测到
// 充值积分后解除（见 clearCommandCodeWindowCooldown）。

const (
	commandCodeAPIBase = "https://api.commandcode.ai"

	// commandCodeExtraSuffixPurchasedCredits 记录充值积分，供阈值停调与 429 冷却判断
	// 滚动窗口是否生效。
	commandCodeExtraSuffixPurchasedCredits = "purchased_credits"
)

var errCommandCodeInvalidResponse = errors.New("invalid command code usage response")

// commandCodePlanNames 与官方 CLI 的 planId 展示名一致；未收录的原样展示。
var commandCodePlanNames = map[string]string{
	"individual-go":       "Go",
	"individual-goat":     "GOAT",
	"individual-pro":      "Pro",
	"individual-pro-v1":   "Pro",
	"individual-provider": "Provider",
	"individual-max":      "Max",
	"individual-ultra":    "Ultra",
	"teams-pro":           "Teams Pro",
}

// commandCodeUsageSupported 报告账号能否查询 Command Code 用量：API Key 账号且推理
// 端点指向官方主机。自定义中转的 Key 不发往官方用量接口。
func (a *Account) commandCodeUsageSupported() bool {
	return a.IsCommandCode() && a.Type == AccountTypeAPIKey && isOfficialCommandCodeHost(a.GetOpenAIBaseURL())
}

// commandCodeNumber 兼容数字与数字字符串。
type commandCodeNumber float64

func (n *commandCodeNumber) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*n = 0
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return err
		}
		*n = commandCodeNumber(v)
		return nil
	}
	var v float64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*n = commandCodeNumber(v)
	return nil
}

type commandCodeWindow struct {
	Used    commandCodeNumber `json:"used"`
	Cap     commandCodeNumber `json:"cap"`
	ResetAt commandCodeNumber `json:"resetAt"`
}

type commandCodeCredits struct {
	MonthlyCredits   commandCodeNumber `json:"monthlyCredits"`
	PurchasedCredits commandCodeNumber `json:"purchasedCredits"`
	FreeCredits      commandCodeNumber `json:"freeCredits"`
}

type commandCodeCreditsPayload struct {
	Credits      *commandCodeCredits `json:"credits"`
	WindowLimits struct {
		Limited  bool               `json:"limited"`
		FiveHour *commandCodeWindow `json:"fiveHour"`
		Weekly   *commandCodeWindow `json:"weekly"`
	} `json:"windowLimits"`
}

type commandCodeSubscriptionPayload struct {
	Data struct {
		PlanID             string `json:"planId"`
		CurrentPeriodStart string `json:"currentPeriodStart"`
		CurrentPeriodEnd   string `json:"currentPeriodEnd"`
	} `json:"data"`
}

type commandCodeSummaryPayload struct {
	TotalCost           *commandCodeNumber `json:"totalCost"`
	TotalMonthlyCredits *commandCodeNumber `json:"totalMonthlyCredits"`
}

// commandCodeUsage 是一次取数的汇总；subscription / summary 取数失败时为 nil，并置
// periodFetchFailed：此时无从判断月度窗口是否存在，不能当作没有月度窗口。
type commandCodeUsage struct {
	credits           commandCodeCreditsPayload
	subscription      *commandCodeSubscriptionPayload
	summary           *commandCodeSummaryPayload
	periodFetchFailed bool
}

// commandCodeHTTPError 是 /alpha 接口的非 2xx 响应。
type commandCodeHTTPError struct {
	status int
	body   string
}

func (e *commandCodeHTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.status, e.body)
}

func (e *commandCodeHTTPError) unauthorized() bool {
	return e.status == http.StatusUnauthorized || e.status == http.StatusForbidden
}

func isCommandCodeAuthError(err error) bool {
	var httpErr *commandCodeHTTPError
	return errors.As(err, &httpErr) && httpErr.unauthorized()
}

type commandCodeUsageClient struct {
	cfg          *config.Config
	httpUpstream HTTPUpstream
	account      *Account
	apiKey       string
	proxyURL     string
}

// get 请求一个 /alpha 接口并解码 JSON。
func (c *commandCodeUsageClient) get(ctx context.Context, path string, query url.Values, out any) error {
	target := commandCodeAPIBase + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	validatedURL, err := cnValidateProbeURL(c.cfg, target)
	if err != nil {
		return infraerrors.New(http.StatusForbidden, "CN_QUOTA_URL_REJECTED", err.Error())
	}
	callCtx, cancel := context.WithTimeout(ctx, cnQuotaUpstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, validatedURL, nil)
	if err != nil {
		return infraerrors.Newf(http.StatusInternalServerError, "CN_QUOTA_REQUEST_BUILD_FAILED", "build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	c.account.ApplyHeaderOverrides(req.Header)

	resp, err := c.httpUpstream.Do(req, c.proxyURL, c.account.ID, maxInt(c.account.Concurrency, 1))
	if err != nil {
		return fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, cnQuotaMaxBodyBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &commandCodeHTTPError{status: resp.StatusCode, body: truncate(strings.TrimSpace(string(body)), 240)}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: %s: %v", errCommandCodeInvalidResponse, path, err)
	}
	return nil
}

// fetch 取积分与窗口；withPeriod 时再取订阅周期与本周期用量以还原月度窗口。
// whoami 用来取 orgId，任何失败都整体返回：无从判断是否组织账户时，按个人账户查会把
// 组织账号查成成员的个人钱包，并据此按余额停调。
func (c *commandCodeUsageClient) fetch(ctx context.Context, withPeriod bool) (*commandCodeUsage, error) {
	var whoami struct {
		Org *struct {
			ID string `json:"id"`
		} `json:"org"`
	}
	query := url.Values{}
	if err := c.get(ctx, "/alpha/whoami", nil, &whoami); err != nil {
		return nil, err
	}
	if whoami.Org != nil && strings.TrimSpace(whoami.Org.ID) != "" {
		query.Set("orgId", strings.TrimSpace(whoami.Org.ID))
	}

	usage := &commandCodeUsage{}
	if err := c.get(ctx, "/alpha/billing/credits", query, &usage.credits); err != nil {
		return nil, err
	}
	if usage.credits.Credits == nil {
		return nil, fmt.Errorf("%w: credits missing", errCommandCodeInvalidResponse)
	}
	if !withPeriod {
		return usage, nil
	}

	var subscription commandCodeSubscriptionPayload
	if err := c.get(ctx, "/alpha/billing/subscriptions", query, &subscription); err != nil {
		if isCommandCodeAuthError(err) {
			return nil, err
		}
		slog.Warn("command_code_subscription_failed", "account_id", c.account.ID, "error", err)
		usage.periodFetchFailed = true
		return usage, nil
	}
	usage.subscription = &subscription
	periodStart := strings.TrimSpace(subscription.Data.CurrentPeriodStart)
	if periodStart == "" {
		return usage, nil
	}
	summaryQuery := url.Values{}
	for key, values := range query {
		summaryQuery[key] = values
	}
	summaryQuery.Set("since", periodStart)
	var summary commandCodeSummaryPayload
	if err := c.get(ctx, "/alpha/usage/summary", summaryQuery, &summary); err != nil {
		if isCommandCodeAuthError(err) {
			return nil, err
		}
		slog.Warn("command_code_usage_summary_failed", "account_id", c.account.ID, "error", err)
		usage.periodFetchFailed = true
		return usage, nil
	}
	usage.summary = &summary
	return usage, nil
}

func isApplicationError(err error) bool {
	var appErr *infraerrors.ApplicationError
	return errors.As(err, &appErr)
}

func commandCodePercent(used, limit float64) float64 {
	percent := used / limit * 100
	if percent < 0 {
		percent = 0
	}
	return math.Round(percent*100) / 100
}

// tiers 返回 5h / weekly（限额套餐）与月度窗口。
func (u *commandCodeUsage) tiers() []CNQuotaTier {
	var tiers []CNQuotaTier
	limits := u.credits.WindowLimits
	if limits.Limited {
		for _, item := range []struct {
			window string
			limit  *commandCodeWindow
		}{
			{window: "5h", limit: limits.FiveHour},
			{window: "weekly", limit: limits.Weekly},
		} {
			if item.limit == nil || item.limit.Cap <= 0 {
				continue
			}
			tiers = append(tiers, CNQuotaTier{
				Window:      item.window,
				UsedPercent: commandCodePercent(float64(item.limit.Used), float64(item.limit.Cap)),
				ResetAt:     cnMillisToRFC3339(int64(item.limit.ResetAt)),
			})
		}
	}
	if u.summary != nil && u.subscription != nil {
		used := 0.0
		switch {
		case u.summary.TotalMonthlyCredits != nil:
			used = float64(*u.summary.TotalMonthlyCredits)
		case u.summary.TotalCost != nil:
			used = float64(*u.summary.TotalCost)
		}
		if limit := used + float64(u.credits.Credits.MonthlyCredits); limit > 0 {
			tiers = append(tiers, CNQuotaTier{
				Window:      "monthly",
				UsedPercent: commandCodePercent(used, limit),
				ResetAt:     cnNormalizeResetTime(u.subscription.Data.CurrentPeriodEnd),
			})
		}
	}
	return tiers
}

func (u *commandCodeUsage) planName() string {
	if u.subscription == nil {
		return ""
	}
	planID := strings.TrimSpace(u.subscription.Data.PlanID)
	if name, ok := commandCodePlanNames[planID]; ok {
		return name
	}
	return planID
}

// balance 返回可用积分合计（套餐剩余 + 充值 + 赠送，美元）。
func (u *commandCodeUsage) balance(now time.Time) *CNProviderBalanceResult {
	credits := u.credits.Credits
	total := float64(credits.MonthlyCredits + credits.PurchasedCredits + credits.FreeCredits)
	total = math.Round(total*10000) / 10000
	return &CNProviderBalanceResult{
		Provider:   PlatformCommandCode,
		Success:    true,
		Balance:    total,
		Currency:   "USD",
		Balances:   []CNProviderBalanceEntry{{Currency: "USD", Balance: total}},
		Available:  true,
		StatusCode: http.StatusOK,
		FetchedAt:  now.Unix(),
	}
}

// clearCommandCodeWindowCooldown 在探测到充值积分时解除窗口用满的临时停调：充值积分
// 不受滚动窗口限制，上游已允许继续请求。只清除 commandCodeUsageLimitReason 前缀的停调，
// 积分不足、花费上限、管理员规则等其他原因的停调不受影响。
func clearCommandCodeWindowCooldown(ctx context.Context, repo AccountRepository, account *Account, usage *commandCodeUsage, now time.Time) bool {
	if usage == nil || usage.credits.Credits == nil || usage.credits.Credits.PurchasedCredits <= 0 {
		return false
	}
	if account.TempUnschedulableUntil == nil || !now.Before(*account.TempUnschedulableUntil) ||
		!strings.HasPrefix(account.TempUnschedulableReason, commandCodeUsageLimitReason) {
		return false
	}
	if err := repo.ClearTempUnschedulable(ctx, account.ID); err != nil {
		slog.Warn("command_code_usage_limit_clear_failed", "account_id", account.ID, "error", err)
		return false
	}
	slog.Info("command_code_usage_limit_cleared", "account_id", account.ID,
		"purchased_credits", float64(usage.credits.Credits.PurchasedCredits))
	return true
}

// commandCodeWindowsBinding 报告账号快照中的滚动 / 月度窗口是否限制调度：
// 有充值积分时上游跳过窗口检查，窗口用满也不会被拒绝。
func commandCodeWindowsBinding(extra map[string]any) bool {
	purchased, ok := cnParseF64(extra[cnExtraKey(PlatformCommandCode, commandCodeExtraSuffixPurchasedCredits)])
	return !ok || purchased <= 0
}

// commandCodeUsageExtraUpdates 构造窗口 + 余额快照。缺席的窗口写 nil，避免套餐
// 变化后沿用旧快照；窗口尚未开始时没有重置时间，同样清掉上一个窗口的值；月度窗口
// 取数失败时保留旧快照；传 nil tiers 时只写余额。
func commandCodeUsageExtraUpdates(usage *commandCodeUsage, tiers []CNQuotaTier, balance *CNProviderBalanceResult, now time.Time, withTiers bool) map[string]any {
	updates := cnBalanceExtraUpdates(PlatformCommandCode, balance, now)
	updates[cnExtraKey(PlatformCommandCode, commandCodeExtraSuffixPurchasedCredits)] = float64(usage.credits.Credits.PurchasedCredits)
	if !withTiers {
		return updates
	}
	for key, value := range cnQuotaExtraUpdates(PlatformCommandCode, tiers, now) {
		updates[key] = value
	}
	present := make(map[string]CNQuotaTier, len(tiers))
	for _, tier := range tiers {
		present[tier.Window] = tier
	}
	for _, window := range []struct {
		name, used, reset string
	}{
		{"5h", cnExtraSuffix5hUsed, cnExtraSuffix5hReset},
		{"weekly", cnExtraSuffixWeeklyUsed, cnExtraSuffixWeeklyReset},
		{"monthly", cnExtraSuffixMonthlyUsed, cnExtraSuffixMonthlyReset},
	} {
		tier, ok := present[window.name]
		if !ok && window.name == "monthly" && usage.periodFetchFailed {
			continue
		}
		if !ok {
			updates[cnExtraKey(PlatformCommandCode, window.used)] = nil
		}
		if tier.ResetAt == "" {
			updates[cnExtraKey(PlatformCommandCode, window.reset)] = nil
		}
	}
	return updates
}

func newCommandCodeUsageClient(cfg *config.Config, httpUpstream HTTPUpstream, account *Account, proxyURL string) *commandCodeUsageClient {
	return &commandCodeUsageClient{
		cfg:          cfg,
		httpUpstream: httpUpstream,
		account:      account,
		apiKey:       strings.TrimSpace(account.GetCNAPIKey()),
		proxyURL:     proxyURL,
	}
}

// commandCodeFetchFailure 把取数错误转换为探测失败文案；ok=false 表示应作为错误返回。
func commandCodeFetchFailure(err error) (status int, message string, ok bool) {
	var httpErr *commandCodeHTTPError
	switch {
	case errors.As(err, &httpErr) && httpErr.unauthorized():
		return httpErr.status, fmt.Sprintf("Authentication failed (HTTP %d)", httpErr.status), true
	case errors.As(err, &httpErr):
		return httpErr.status, fmt.Sprintf("API error (HTTP %d): %s", httpErr.status, httpErr.body), true
	case errors.Is(err, errCommandCodeInvalidResponse):
		return http.StatusOK, "Invalid usage response", true
	default:
		return 0, "", false
	}
}

// queryCommandCodeUsage 查询 Command Code 积分与窗口，落窗口与余额快照。
func (s *CNProviderQuotaService) queryCommandCodeUsage(ctx context.Context, account *Account) (*CNProviderQuotaProbeResult, error) {
	client := newCommandCodeUsageClient(s.cfg, s.httpUpstream, account, s.resolveProxyURL(ctx, account))
	if client.apiKey == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NO_APIKEY", "account api_key is empty")
	}
	now := time.Now().UTC()
	result := &CNProviderQuotaProbeResult{
		Provider:  PlatformCommandCode,
		Source:    "command_code_usage",
		FetchedAt: now.Unix(),
	}
	usage, err := client.fetch(ctx, true)
	if err != nil {
		status, message, ok := commandCodeFetchFailure(err)
		if !ok {
			if isApplicationError(err) {
				return nil, err
			}
			return nil, infraerrors.Newf(http.StatusBadGateway, "CN_QUOTA_REQUEST_FAILED", "%v", err)
		}
		result.StatusCode = status
		result.Error = message
		return result, nil
	}

	tiers := usage.tiers()
	balance := usage.balance(now)
	result.StatusCode = http.StatusOK
	result.Tiers = tiers
	result.PlanLevel = usage.planName()
	result.Balance = balance
	result.Success = true
	result.CredentialValid = true

	updates := commandCodeUsageExtraUpdates(usage, tiers, balance, now, true)
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("cn_quota_persist_failed", "account_id", account.ID, "provider", PlatformCommandCode, "error", err)
	} else {
		result.Persisted = true
		balance.Persisted = true
	}
	clearCommandCodeWindowCooldown(ctx, s.accountRepo, account, usage, now)
	return result, nil
}

// queryCommandCodeBalance 只查积分余额（whoami + credits），落余额快照。
func (s *CNProviderBalanceService) queryCommandCodeBalance(ctx context.Context, account *Account) (*CNProviderBalanceResult, error) {
	client := newCommandCodeUsageClient(s.cfg, s.httpUpstream, account, s.resolveProxyURL(ctx, account))
	if client.apiKey == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_BALANCE_NO_APIKEY", "account api_key is empty")
	}
	now := time.Now().UTC()
	usage, err := client.fetch(ctx, false)
	if err != nil {
		status, message, ok := commandCodeFetchFailure(err)
		if !ok {
			if isApplicationError(err) {
				return nil, err
			}
			return nil, infraerrors.Newf(http.StatusBadGateway, "CN_BALANCE_REQUEST_FAILED", "%v", err)
		}
		return &CNProviderBalanceResult{
			Provider:   PlatformCommandCode,
			StatusCode: status,
			FetchedAt:  now.Unix(),
			Available:  true,
			Error:      message,
		}, nil
	}
	result := usage.balance(now)
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, commandCodeUsageExtraUpdates(usage, nil, result, now, false)); err != nil {
		slog.Warn("cn_balance_persist_failed", "account_id", account.ID, "provider", PlatformCommandCode, "error", err)
	} else {
		result.Persisted = true
	}
	clearCommandCodeWindowCooldown(ctx, s.accountRepo, account, usage, now)
	return result, nil
}

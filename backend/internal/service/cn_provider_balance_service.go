package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/singleflight"
)

// 国产供应商 payg（按量付费）账号余额探测服务。
//
// 仅覆盖有公开余额端点的供应商：
//   - Kimi/Moonshot：GET https://api.moonshot.cn/v1/users/me/balance (Bearer) → data.available_balance
//   - DeepSeek：     GET https://api.deepseek.com/user/balance (Bearer) → balance_infos[].total_balance + is_available
//
// 智谱（zhipu）无公开余额端点（OpenAPI 规格验证），仅靠响应式 429/402（见
// ratelimit_cn_providers.go）。解析逻辑对齐 cc-switch services/balance.rs::query_deepseek。
const (
	cnBalanceUpstreamTimeout = 15 * time.Second
	cnBalanceMaxBodyBytes    = 256 * 1024

	// Extra 余额快照键后缀（加 provider 前缀）。
	cnBalanceExtraSuffixBalance   = "balance"
	cnBalanceExtraSuffixCurrency  = "balance_currency"
	cnBalanceExtraSuffixAvailable = "balance_available" // deepseek is_available 健康标记
	cnBalanceExtraSuffixUpdated   = "balance_updated_at"
	cnBalanceExtraSuffixBalances  = "balances" // 多币种明细（deepseek USD+CNY）
)

// CNProviderBalanceEntry 是单一币种的余额明细。
type CNProviderBalanceEntry struct {
	Currency string  `json:"currency"`
	Balance  float64 `json:"balance"`
}

// CNProviderBalanceResult 是余额探测的返回结构（管理端 + UI 消费）。
type CNProviderBalanceResult struct {
	Provider string `json:"provider"`
	Success  bool   `json:"success"`
	// Balance/Currency 为主币种（balance_infos 首条，兼容单币种消费方）；
	// 完整明细见 Balances（deepseek 双币种账号含 CNY + USD 两条）。
	Balance    float64                  `json:"balance"`
	Currency   string                   `json:"currency,omitempty"`
	Balances   []CNProviderBalanceEntry `json:"balances,omitempty"`
	Available  bool                     `json:"available"` // 健康标记（deepseek is_available；kimi 无此概念恒 true）
	StatusCode int                      `json:"status_code,omitempty"`
	FetchedAt  int64                    `json:"fetched_at"`
	Persisted  bool                     `json:"persisted"`
	Error      string                   `json:"error,omitempty"`
}

// CNProviderBalanceService 探测 Kimi / DeepSeek payg 账号的账户余额。
type CNProviderBalanceService struct {
	accountRepo  AccountRepository
	proxyRepo    ProxyRepository
	httpUpstream HTTPUpstream
	cfg          *config.Config
	flight       singleflight.Group
}

// NewCNProviderBalanceService 构造余额探测服务。
func NewCNProviderBalanceService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
) *CNProviderBalanceService {
	return &CNProviderBalanceService{
		accountRepo:  accountRepo,
		proxyRepo:    proxyRepo,
		httpUpstream: httpUpstream,
		cfg:          cfg,
	}
}

// QueryBalance 探测指定 payg 账号的余额并落 Extra 快照。
func (s *CNProviderBalanceService) QueryBalance(ctx context.Context, accountID int64) (*CNProviderBalanceResult, error) {
	account, err := s.loadPayGAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return s.QueryBalanceForAccount(ctx, account)
}

// QueryBalanceForAccount 探测已加载账号（配额监控 fetcher / 周期余额检测复用，
// 避免二次 GetByID）。singleflight key 与 QueryBalance 相同，按账号 ID 合并。
func (s *CNProviderBalanceService) QueryBalanceForAccount(ctx context.Context, account *Account) (*CNProviderBalanceResult, error) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "CN_BALANCE_NOT_CONFIGURED", "cn provider balance service is not configured")
	}
	if err := validatePayGAccount(account); err != nil {
		return nil, err
	}
	key := "cn_balance:" + strconv.FormatInt(account.ID, 10)
	resultCh := s.flight.DoChan(key, func() (any, error) {
		probeCtx, cancel := context.WithTimeout(context.Background(), cnBalanceUpstreamTimeout+5*time.Second)
		defer cancel()
		return s.queryBalanceForAccount(probeCtx, account)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case flightResult := <-resultCh:
		if flightResult.Err != nil {
			return nil, flightResult.Err
		}
		result, ok := flightResult.Val.(*CNProviderBalanceResult)
		if !ok || result == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "CN_BALANCE_PROBE_RESULT_INVALID", "invalid cn provider balance probe result")
		}
		cloned := *result
		return &cloned, nil
	}
}

func (s *CNProviderBalanceService) queryBalanceForAccount(ctx context.Context, account *Account) (*CNProviderBalanceResult, error) {
	provider := account.Platform
	if provider != PlatformKimi && provider != PlatformDeepseek {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_BALANCE_NO_ENDPOINT", "account provider has no balance endpoint")
	}

	apiKey := strings.TrimSpace(account.GetCNAPIKey())
	if apiKey == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_BALANCE_NO_APIKEY", "account api_key is empty")
	}

	targetURL := cnBalanceURL(account)
	// 探测发起前过出站 URL 安全策略（与网关转发/Grok 探测同一套校验）：
	// DeepSeek 端点由账号 base_url 衍生，不得把 API key 发往策略外主机。
	validatedURL, err := cnValidateProbeURL(s.cfg, targetURL)
	if err != nil {
		return nil, infraerrors.New(http.StatusForbidden, "CN_BALANCE_URL_REJECTED", err.Error())
	}
	targetURL = validatedURL
	proxyURL := s.resolveProxyURL(ctx, account)

	statusCode, bodyBytes, err := s.doCNBalanceGET(ctx, account, proxyURL, targetURL, apiKey)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	result := &CNProviderBalanceResult{
		Provider:   provider,
		FetchedAt:  now.Unix(),
		StatusCode: statusCode,
		Available:  true,
	}

	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		result.Error = fmt.Sprintf("Authentication failed (HTTP %d)", statusCode)
		return result, nil
	}

	entries, available, parseErr := parseCNProviderBalanceBody(provider, bodyBytes)
	// DeepSeek 官方 /user/balance；中转站（NewAPI/OneAPI 等）通常没有该接口，
	// 会回 HTML/404。回退到 NewAPI 的 sk- 可读余额：GET /api/usage/token/。
	if provider == PlatformDeepseek && (parseErr != "" || len(entries) == 0) {
		if fbEntries, fbAvailable, fbStatus, fbOK := s.tryNewAPITokenUsageBalance(ctx, account, proxyURL, apiKey); fbOK {
			entries = fbEntries
			available = fbAvailable
			result.StatusCode = fbStatus
			parseErr = ""
		}
	}
	if len(entries) == 0 {
		if statusCode < 200 || statusCode >= 300 {
			result.Error = fmt.Sprintf("API error (HTTP %d): %s", statusCode, truncate(strings.TrimSpace(string(bodyBytes)), 240))
			return result, nil
		}
		if parseErr != "" {
			result.Error = parseErr
			return result, nil
		}
		result.Error = "Invalid balance response: no valid balance entries"
		return result, nil
	}

	result.Balances = entries
	result.Balance = entries[0].Balance
	result.Currency = entries[0].Currency
	result.Available = available
	result.Success = true

	balanceUpdates := make([]any, 0, len(entries))
	for _, entry := range entries {
		balanceUpdates = append(balanceUpdates, map[string]any{
			"currency": entry.Currency,
			"balance":  entry.Balance,
		})
	}
	updates := map[string]any{
		cnExtraKey(provider, cnBalanceExtraSuffixBalance):   result.Balance,
		cnExtraKey(provider, cnBalanceExtraSuffixCurrency):  result.Currency,
		cnExtraKey(provider, cnBalanceExtraSuffixAvailable): available,
		cnExtraKey(provider, cnBalanceExtraSuffixUpdated):   now.Format(time.RFC3339),
		cnExtraKey(provider, cnBalanceExtraSuffixBalances):  balanceUpdates,
		// 余额探测成功即清除响应式 402/429 写下的 balance_low 标记。
		cnExtraKey(provider, cnBalanceExtraSuffixLow): false,
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("cn_balance_persist_failed", "account_id", account.ID, "provider", provider, "error", err)
	} else {
		result.Persisted = true
	}
	return result, nil
}

func (s *CNProviderBalanceService) doCNBalanceGET(
	ctx context.Context,
	account *Account,
	proxyURL string,
	targetURL string,
	apiKey string,
) (int, []byte, error) {
	callCtx, cancel := context.WithTimeout(ctx, cnBalanceUpstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, nil, infraerrors.Newf(http.StatusInternalServerError, "CN_BALANCE_REQUEST_BUILD_FAILED", "build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(req.Header)

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		return 0, nil, infraerrors.Newf(http.StatusBadGateway, "CN_BALANCE_REQUEST_FAILED", "upstream request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, cnBalanceMaxBodyBytes))
	return resp.StatusCode, bodyBytes, nil
}

func (s *CNProviderBalanceService) tryNewAPITokenUsageBalance(
	ctx context.Context,
	account *Account,
	proxyURL string,
	apiKey string,
) ([]CNProviderBalanceEntry, bool, int, bool) {
	usageURL := cnNewAPITokenUsageURL(account)
	if usageURL == "" {
		return nil, false, 0, false
	}
	validatedURL, err := cnValidateProbeURL(s.cfg, usageURL)
	if err != nil {
		return nil, false, 0, false
	}
	statusCode, bodyBytes, err := s.doCNBalanceGET(ctx, account, proxyURL, validatedURL, apiKey)
	if err != nil || statusCode < 200 || statusCode >= 300 {
		return nil, false, statusCode, false
	}
	entries, available, ok := parseNewAPITokenUsageBalance(bodyBytes)
	if !ok || len(entries) == 0 {
		return nil, false, statusCode, false
	}
	return entries, available, statusCode, true
}

func parseCNProviderBalanceBody(provider string, bodyBytes []byte) ([]CNProviderBalanceEntry, bool, string) {
	available := true
	switch provider {
	case PlatformKimi:
		balance, ok := cnParseF64(gjson.GetBytes(bodyBytes, "data.available_balance").Value())
		if !ok {
			return nil, false, "Invalid balance response: missing available_balance"
		}
		return []CNProviderBalanceEntry{{Currency: "CNY", Balance: balance}}, true, ""
	case PlatformDeepseek:
		if v := gjson.GetBytes(bodyBytes, "is_available"); v.Exists() {
			available = v.Bool()
		}
		balanceInfos := gjson.GetBytes(bodyBytes, "balance_infos")
		if !balanceInfos.Exists() || !balanceInfos.IsArray() {
			return nil, available, "Invalid balance response: missing balance_infos"
		}
		var entries []CNProviderBalanceEntry
		balanceInfos.ForEach(func(_, info gjson.Result) bool {
			currency := strings.ToUpper(strings.TrimSpace(info.Get("currency").String()))
			totalBalance := info.Get("total_balance")
			balance, ok := cnParseF64(totalBalance.Value())
			if !totalBalance.Exists() || !ok {
				return true
			}
			if currency == "" {
				currency = "CNY"
			}
			entries = append(entries, CNProviderBalanceEntry{Currency: currency, Balance: balance})
			return true
		})
		if len(entries) == 0 {
			return nil, available, "Invalid balance response: no valid balance entries"
		}
		return entries, available, ""
	default:
		return nil, false, "account provider has no balance endpoint"
	}
}

// parseNewAPITokenUsageBalance 解析 NewAPI/OneAPI GET /api/usage/token/ 响应。
// sheapi 等站点在 data.remaining + data.unit 中返回钱包余额；上游原版则主要给
// data.total_available（内部 quota，默认 500000 = $1）。
func parseNewAPITokenUsageBalance(bodyBytes []byte) ([]CNProviderBalanceEntry, bool, bool) {
	data := gjson.GetBytes(bodyBytes, "data")
	if !data.Exists() || !data.IsObject() {
		return nil, false, false
	}
	available := true
	if v := data.Get("is_active"); v.Exists() {
		available = v.Bool()
	}
	if rem := data.Get("remaining"); rem.Exists() {
		balance, ok := cnParseF64(rem.Value())
		if !ok {
			return nil, false, false
		}
		currency := strings.ToUpper(strings.TrimSpace(data.Get("unit").String()))
		if currency == "" {
			currency = "USD"
		}
		return []CNProviderBalanceEntry{{Currency: currency, Balance: balance}}, available, true
	}
	if avail := data.Get("total_available"); avail.Exists() {
		quota, ok := cnParseF64(avail.Value())
		if !ok {
			return nil, false, false
		}
		// NewAPI 内部额度：500000 quota ≈ 1 USD。
		return []CNProviderBalanceEntry{{Currency: "USD", Balance: quota / 500000.0}}, available, true
	}
	return nil, false, false
}

// cnNewAPITokenUsageURL 把 OpenAI 风格 base_url 映射到 NewAPI 站点根的
// /api/usage/token/（sk- 可读，无需控制台 Access Token）。
func cnNewAPITokenUsageURL(account *Account) string {
	if account == nil {
		return ""
	}
	base := strings.TrimSpace(account.GetOpenAIFormatBaseURL())
	if base == "" {
		return ""
	}
	return buildNewAPISiteAPIURL(base, "/api/usage/token/")
}

// loadPayGAccount 加载 payg 模式的国产供应商账号（余额仅对 payg 有意义；coding 走额度）。
func (s *CNProviderBalanceService) loadPayGAccount(ctx context.Context, accountID int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusNotFound, "CN_BALANCE_ACCOUNT_NOT_FOUND", "account not found: %v", err)
	}
	if err := validatePayGAccount(account); err != nil {
		return nil, err
	}
	return account, nil
}

// validatePayGAccount 加载后的非 DB 校验（ForAccount 入口同样复用，
// 保证直传 account 也不绕过平台/模式检查）。
func validatePayGAccount(account *Account) error {
	if account == nil {
		return infraerrors.New(http.StatusNotFound, "CN_BALANCE_ACCOUNT_NOT_FOUND", "account not found")
	}
	if !account.IsCNProvider() {
		return infraerrors.New(http.StatusBadRequest, "CN_BALANCE_INVALID_PLATFORM", "account is not a CN provider account")
	}
	// coding 账号走额度探测，余额端点不适用。
	if account.IsCodingPlan() {
		return infraerrors.New(http.StatusBadRequest, "CN_BALANCE_CODING_PLAN", "coding plan account has no balance endpoint; use quota probe")
	}
	return nil
}

func (s *CNProviderBalanceService) resolveProxyURL(ctx context.Context, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	if account.Proxy != nil {
		return account.Proxy.URL()
	}
	if s != nil && s.proxyRepo != nil {
		if proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
			account.Proxy = proxy
			return proxy.URL()
		}
	}
	return ""
}

// cnBalanceURL 解析账号的余额端点。
//
//   - Kimi：固定 https://api.moonshot.cn/v1/users/me/balance（与 base_url 无关，Moonshot 仅此一处）
//   - DeepSeek：基于 base_url 拼接 /user/balance（支持自定义域名）
func cnBalanceURL(account *Account) string {
	switch account.Platform {
	case PlatformKimi:
		return "https://api.moonshot.cn/v1/users/me/balance"
	case PlatformDeepseek:
		// Anthropic 协议账号的凭证 base_url 指向 /anthropic 端点，余额探测需回退
		// 到 OpenAI 格式 base（协议感知）再拼接 /user/balance。
		return strings.TrimRight(account.GetOpenAIFormatBaseURL(), "/") + "/user/balance"
	default:
		return ""
	}
}

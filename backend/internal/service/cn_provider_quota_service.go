package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

// 国产供应商 Coding Plan 滚动窗口额度探测服务（Kimi For Coding / 智谱 GLM Coding Plan）。
//
// 与 grok_quota_service 不同：CN 供应商走数据面 API Key（无 OAuth token provider），
// 额度端点为只读 GET，解析 5h + weekly 两档滚动窗口并落 account.Extra 快照，
// 供账号调度阈值评估（account_scheduling_threshold_eval.go）做主动停调。
//
// 解析逻辑对齐 cc-switch（farion1231/cc-switch）services/coding_plan.rs 的
// query_kimi / query_zhipu，包括智谱 unit 字段优先分类与 reset 兜底启发式。
const (
	cnQuotaUpstreamTimeout = 15 * time.Second
	cnQuotaMaxBodyBytes    = 256 * 1024

	// Extra 快照键后缀（加 provider 前缀，如 kimi_5h_used_percent）。
	cnExtraSuffix5hUsed       = "5h_used_percent"
	cnExtraSuffix5hReset      = "5h_reset_at"
	cnExtraSuffixWeeklyUsed   = "weekly_used_percent"
	cnExtraSuffixWeeklyReset  = "weekly_reset_at"
	cnExtraSuffixUsageUpdated = "usage_updated_at"
)

// cnExtraKey 拼接 provider 维度的 extra 键。
func cnExtraKey(provider, suffix string) string { return provider + "_" + suffix }

// CNQuotaTier 表示一个滚动用量窗口档位（5h / weekly）。
type CNQuotaTier struct {
	Window      string  `json:"window"`             // "5h" | "weekly"
	UsedPercent float64 `json:"used_percent"`       // 已用百分比（0-100+，不做裁剪）
	ResetAt     string  `json:"reset_at,omitempty"` // RFC3339，空表示无重置时间
}

// CNProviderQuotaProbeResult 是 Coding Plan 额度探测的返回结构（管理端 + UI 消费）。
type CNProviderQuotaProbeResult struct {
	Provider        string        `json:"provider"`
	Source          string        `json:"source"`
	Success         bool          `json:"success"`
	CredentialValid bool          `json:"credential_valid"` // false = 401/403 鉴权失败
	Tiers           []CNQuotaTier `json:"tiers,omitempty"`
	PlanLevel       string        `json:"plan_level,omitempty"` // 智谱套餐等级
	StatusCode      int           `json:"status_code,omitempty"`
	FetchedAt       int64         `json:"fetched_at"`
	Persisted       bool          `json:"persisted"`
	Error           string        `json:"error,omitempty"`
}

// CNProviderQuotaService 探测 Kimi / Zhipu Coding Plan 的滚动窗口用量。
type CNProviderQuotaService struct {
	accountRepo  AccountRepository
	proxyRepo    ProxyRepository
	httpUpstream HTTPUpstream
	cfg          *config.Config
	flight       singleflight.Group
}

// NewCNProviderQuotaService 构造 Coding Plan 额度探测服务。
func NewCNProviderQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
) *CNProviderQuotaService {
	return &CNProviderQuotaService{
		accountRepo:  accountRepo,
		proxyRepo:    proxyRepo,
		httpUpstream: httpUpstream,
		cfg:          cfg,
	}
}

// QueryUsage 探测指定账号的 Coding Plan 滚动窗口用量并落 Extra 快照。
// 同一账号的并发探测会被 singleflight 合并。
func (s *CNProviderQuotaService) QueryUsage(ctx context.Context, accountID int64) (*CNProviderQuotaProbeResult, error) {
	account, err := s.loadCodingPlanAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return s.QueryUsageForAccount(ctx, account)
}

// QueryUsageForAccount 探测已加载账号（配额监控 fetcher 复用，避免二次 GetByID）。
// singleflight key 与 QueryUsage 相同，按账号 ID 与 admin 侧并发探测合并。
func (s *CNProviderQuotaService) QueryUsageForAccount(ctx context.Context, account *Account) (*CNProviderQuotaProbeResult, error) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_NOT_CONFIGURED", "cn provider quota service is not configured")
	}
	if err := validateCodingPlanAccount(account); err != nil {
		return nil, err
	}
	key := "cn_quota:" + strconv.FormatInt(account.ID, 10)
	resultCh := s.flight.DoChan(key, func() (any, error) {
		probeCtx, cancel := context.WithTimeout(context.Background(), cnQuotaUpstreamTimeout+5*time.Second)
		defer cancel()
		return s.queryUsageForAccount(probeCtx, account)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case flightResult := <-resultCh:
		if flightResult.Err != nil {
			return nil, flightResult.Err
		}
		result, ok := flightResult.Val.(*CNProviderQuotaProbeResult)
		if !ok || result == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_PROBE_RESULT_INVALID", "invalid cn provider quota probe result")
		}
		cloned := *result
		return &cloned, nil
	}
}

func (s *CNProviderQuotaService) queryUsageForAccount(ctx context.Context, account *Account) (*CNProviderQuotaProbeResult, error) {
	provider := account.GetCodingPlanProvider()
	spec, ok := GetCNProviderSpec(provider)
	if !ok || spec.QuotaProbe == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NOT_CODING_PLAN", "account is not a coding plan account with quota endpoint")
	}

	apiKey := strings.TrimSpace(account.GetCNAPIKey())
	if apiKey == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NO_APIKEY", "account api_key is empty")
	}

	targetURL := spec.QuotaProbe.QuotaURL(account)
	authHeader := spec.QuotaProbe.QuotaAuthHeader(apiKey)

	// 探测发起前过出站 URL 安全策略（与网关转发/Grok 探测同一套校验）：
	// 端点多由账号 base_url 衍生，不得把 API key 发往策略外主机。
	validatedURL, err := cnValidateProbeURL(s.cfg, targetURL)
	if err != nil {
		return nil, infraerrors.New(http.StatusForbidden, "CN_QUOTA_URL_REJECTED", err.Error())
	}
	targetURL = validatedURL

	proxyURL := s.resolveProxyURL(ctx, account)
	callCtx, cancel := context.WithTimeout(ctx, cnQuotaUpstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "CN_QUOTA_REQUEST_BUILD_FAILED", "build request: %v", err)
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/json")
	spec.QuotaProbe.SetQuotaHeaders(req)
	// 探测与真实转发保持同一套账号级请求头覆写，避免探测通过但转发失败。
	account.ApplyHeaderOverrides(req.Header)

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CN_QUOTA_REQUEST_FAILED", "upstream request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, cnQuotaMaxBodyBytes))

	now := time.Now().UTC()
	result := &CNProviderQuotaProbeResult{
		Provider:   provider,
		Source:     "coding_plan",
		FetchedAt:  now.Unix(),
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		// 鉴权失败：不落快照（不覆盖之前的有效值），仅返回失败结果供前端提示。
		result.Error = fmt.Sprintf("Authentication failed (HTTP %d)", resp.StatusCode)
		return result, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("API error (HTTP %d): %s", resp.StatusCode, truncate(strings.TrimSpace(string(bodyBytes)), 240))
		return result, nil
	}

	// 平台钩子负责业务级错误判定（HTTP 2xx 但 success=false 之类）与窗口解析。
	tiers, planLevel, bizErrMsg := spec.QuotaProbe.ParseQuota(bodyBytes)
	if bizErrMsg != "" {
		result.Error = bizErrMsg
		return result, nil
	}
	result.Tiers = tiers
	result.PlanLevel = planLevel
	result.Success = true
	result.CredentialValid = true

	updates := cnQuotaExtraUpdates(provider, tiers, now)
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("cn_quota_persist_failed", "account_id", account.ID, "provider", provider, "error", err)
	} else {
		result.Persisted = true
	}
	return result, nil
}

func (s *CNProviderQuotaService) loadCodingPlanAccount(ctx context.Context, accountID int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusNotFound, "CN_QUOTA_ACCOUNT_NOT_FOUND", "account not found: %v", err)
	}
	if err := validateCodingPlanAccount(account); err != nil {
		return nil, err
	}
	return account, nil
}

// validateCodingPlanAccount 加载后的非 DB 校验（ForAccount 入口同样复用，
// 保证直传 account 也不绕过平台/模式检查）。
func validateCodingPlanAccount(account *Account) error {
	if account == nil {
		return infraerrors.New(http.StatusNotFound, "CN_QUOTA_ACCOUNT_NOT_FOUND", "account not found")
	}
	if !account.IsCNProvider() {
		return infraerrors.New(http.StatusBadRequest, "CN_QUOTA_INVALID_PLATFORM", "account is not a CN provider account")
	}
	if !account.IsCodingPlan() {
		return infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NOT_CODING_PLAN", "account is not a coding plan account")
	}
	return nil
}

func (s *CNProviderQuotaService) resolveProxyURL(ctx context.Context, account *Account) string {
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

// cnQuotaExtraUpdates 根据 tier 列表构造 provider 维度的 Extra 快照更新。
func cnQuotaExtraUpdates(provider string, tiers []CNQuotaTier, now time.Time) map[string]any {
	updates := map[string]any{
		cnExtraKey(provider, cnExtraSuffixUsageUpdated): now.Format(time.RFC3339),
	}
	for _, t := range tiers {
		switch t.Window {
		case "5h":
			updates[cnExtraKey(provider, cnExtraSuffix5hUsed)] = t.UsedPercent
			if t.ResetAt != "" {
				updates[cnExtraKey(provider, cnExtraSuffix5hReset)] = t.ResetAt
			}
		case "weekly":
			updates[cnExtraKey(provider, cnExtraSuffixWeeklyUsed)] = t.UsedPercent
			if t.ResetAt != "" {
				updates[cnExtraKey(provider, cnExtraSuffixWeeklyReset)] = t.ResetAt
			}
		}
	}
	return updates
}

// cnParseF64 把 JSON 数值或字符串解析为 float64（兼容 "100" 与 100）。
func cnParseF64(raw any) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// cnNormalizeResetTime 把上游重置时间（ISO8601 字符串 / 秒级 / 毫秒级数字）归一化为
// RFC3339 字符串；无法识别或非正时间戳返回空串。
func cnNormalizeResetTime(raw any) string {
	switch v := raw.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return ""
		}
		if ts, err := parseSchedulingTime(s); err == nil {
			return ts.UTC().Format(time.RFC3339)
		}
		return ""
	case float64:
		return cnMillisToRFC3339(int64(v))
	case int:
		return cnMillisToRFC3339(int64(v))
	case int64:
		return cnMillisToRFC3339(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return cnMillisToRFC3339(n)
		}
		return ""
	default:
		return ""
	}
}

// cnMillisToRFC3339 把秒级（<1e12）或毫秒级时间戳转为 RFC3339 字符串；非正返回空串。
func cnMillisToRFC3339(n int64) string {
	if n <= 0 {
		return ""
	}
	var ms int64
	if n < 1_000_000_000_000 {
		ms = n * 1000
	} else {
		ms = n
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

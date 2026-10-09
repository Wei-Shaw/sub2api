package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/gjson"
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
	cnExtraSuffixMonthlyUsed  = "monthly_used_percent"
	cnExtraSuffixMonthlyReset = "monthly_reset_at"
	cnExtraSuffixUsageUpdated = "usage_updated_at"
)

// cnExtraKey 拼接 provider 维度的 extra 键。
func cnExtraKey(provider, suffix string) string { return provider + "_" + suffix }

// CNQuotaTier 表示一个滚动用量窗口档位（5h / weekly）。
type CNQuotaTier struct {
	Window      string  `json:"window"`             // "5h" | "weekly" | "monthly"
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

	// Balance 为同一次探测得到的余额（Command Code 积分与窗口同源），其余供应商为空。
	Balance *CNProviderBalanceResult `json:"balance,omitempty"`
}

// CNProviderQuotaService 探测 Kimi / Zhipu / MiniMax Coding Plan、OpenCode Go、
// Command Code 与 Cline（ClinePass）的滚动窗口用量。
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
	if provider == PlatformCommandCode {
		return s.queryCommandCodeUsage(ctx, account)
	}
	if provider == PlatformCline {
		return s.queryClineUsage(ctx, account)
	}
	if provider != PlatformKimi && provider != PlatformZhipu && provider != PlatformMiniMax && provider != PlatformOpenCodeGo {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NOT_CODING_PLAN", "account is not a kimi/zhipu/minimax coding plan or opencode go account")
	}

	apiKey := strings.TrimSpace(account.GetCNAPIKey())
	if apiKey == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NO_APIKEY", "account api_key is empty")
	}

	baseURL := account.GetOpenAIBaseURL()
	var (
		targetURL  string
		authHeader string
		zhipuOrg   string
	)
	switch provider {
	case PlatformKimi:
		targetURL = kimiQuotaURL(baseURL)
		authHeader = "Bearer " + apiKey
	case PlatformOpenCodeGo:
		targetURL = openCodeGoQuotaURL(baseURL)
		authHeader = "Bearer " + apiKey
	case PlatformZhipu:
		targetURL = zhipuQuotaURL(baseURL)
		authHeader = apiKey // 智谱额度端点鉴权不加 Bearer 前缀
		// 团队版 GLM Coding Plan：额度端点需 ?type=2 + 组织/项目请求头，
		// 否则官方 API 回「当前用户不存在coding plan」。组织 ID 存在即视为
		// 团队版（个人版凭据不含该字段，走原个人版查询路径）。
		zhipuOrg = strings.TrimSpace(account.GetCredential("zhipu_organization"))
		if zhipuOrg != "" {
			targetURL += "?type=2"
		}
	case PlatformMiniMax:
		targetURL = minimaxQuotaURL(baseURL)
		authHeader = "Bearer " + apiKey
	}

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
	if provider == PlatformZhipu || provider == PlatformMiniMax {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept-Language", "en-US,en")
		if zhipuOrg != "" {
			req.Header.Set("bigmodel-organization", zhipuOrg)
			if project := strings.TrimSpace(account.GetCredential("zhipu_project")); project != "" {
				req.Header.Set("bigmodel-project", project)
			}
		}
	}
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

	// HTTP 2xx 但响应体不是合法 JSON：中间网关的错误页、登录跳转、WAF 拦截页都长这样。
	// 这类响应绝不能当成「成功但窗口全没了」——cnQuotaExtraUpdates 会把缺席的窗口置空，
	// 等于用一个垃圾响应抹掉有效快照（阈值停调也会跟着解除）。
	if !gjson.ValidBytes(bodyBytes) {
		result.Error = cnQuotaInvalidResponseError("response body is not valid JSON")
		return result, nil
	}

	// 智谱业务级错误（HTTP 2xx 但 success=false）。
	if provider == PlatformZhipu {
		if success := gjson.GetBytes(bodyBytes, "success"); success.Exists() && !success.Bool() {
			msg := strings.TrimSpace(gjson.GetBytes(bodyBytes, "msg").String())
			if msg == "" {
				msg = "unknown zhipu quota error"
			}
			result.Error = "API error: " + msg
			return result, nil
		}
	}

	// MiniMax 业务级错误（HTTP 2xx 但 base_resp.status_code != 0）。
	// 与智谱一样放在结构校验之前：业务错误响应本来就没有 model_remains，
	// 应该回原始错误文案而不是笼统的「结构不对」。
	if provider == PlatformMiniMax {
		if status := gjson.GetBytes(bodyBytes, "base_resp.status_code"); status.Exists() && status.Int() != 0 {
			msg := strings.TrimSpace(gjson.GetBytes(bodyBytes, "base_resp.status_msg").String())
			if msg == "" {
				msg = "unknown minimax quota error"
			}
			result.Error = fmt.Sprintf("API error (%d): %s", status.Int(), msg)
			return result, nil
		}
	}

	// 合法 JSON 但没有解析器要读的容器：同样不可信，不落快照。
	if issue := cnQuotaResponseShapeIssue(provider, bodyBytes); issue != "" {
		result.Error = cnQuotaInvalidResponseError(issue)
		return result, nil
	}

	var (
		tiers    []CNQuotaTier
		parseErr error
	)
	switch provider {
	case PlatformKimi:
		tiers, parseErr = parseKimiUsageTiers(bodyBytes)
	case PlatformOpenCodeGo:
		tiers, parseErr = parseOpenCodeGoUsageTiers(bodyBytes)
		result.PlanLevel = "OpenCode Go"
	case PlatformZhipu:
		tiers, parseErr = parseZhipuTokenTiers(gjson.GetBytes(bodyBytes, "data"))
		result.PlanLevel = strings.TrimSpace(gjson.GetBytes(bodyBytes, "data.level").String())
	case PlatformMiniMax:
		tiers, parseErr = parseMiniMaxUsageTiers(bodyBytes)
		result.PlanLevel = strings.TrimSpace(gjson.GetBytes(bodyBytes, "current_subscribe_title").String())
	}
	// 窗口已声明但字段读不出来：与「结构不对」同等对待——报错、不落快照。
	// 缺席的窗口（节点根本没声明）不会走到这里，仍然按正常成功路径被清理。
	if parseErr != nil {
		result.Error = cnQuotaInvalidResponseError(parseErr.Error())
		return result, nil
	}

	// 兜底：结构看着对，却一个窗口都没解析出来。Coding Plan 账号不存在「零窗口」
	// 这种正常状态（额度窗口与用量无关，0% 也会下发），所以这多半是上游改了字段名
	// 或回了空壳。这种响应不落快照，避免把有效快照清成空。
	// 真正需要支持的「窗口被取消」场景（如 MiniMax 关掉周限额）仍然有其它档位，
	// tiers 非空，照常走 cnQuotaExtraUpdates 的显式清理。
	if len(tiers) == 0 {
		result.Error = cnQuotaInvalidResponseError("no usage window parsed from the response")
		return result, nil
	}

	result.Tiers = tiers
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

// cnQuotaInvalidResponseError 拼接「响应不可信」的错误文案（带机器可读前缀）。
func cnQuotaInvalidResponseError(detail string) string {
	msg := "CN_QUOTA_INVALID_RESPONSE: unexpected upstream payload"
	if detail == "" {
		return msg
	}
	return msg + " (" + detail + ")"
}

// cnQuotaResponseShapeIssue 校验 2xx 响应体是否具备该供应商解析器要读的容器，
// 返回空串表示结构可用。路径与各 parse*UsageTiers 保持一一对应。
func cnQuotaResponseShapeIssue(provider string, body []byte) string {
	switch provider {
	case PlatformKimi:
		// parseKimiUsageTiers：limits[].detail → 5h，usage → 周窗口，有其一即可解析。
		if gjson.GetBytes(body, "limits").IsArray() || gjson.GetBytes(body, "usage").IsObject() {
			return ""
		}
		return `missing "limits" array and "usage" object`
	case PlatformOpenCodeGo:
		// parseOpenCodeGoUsageTiers：usage.{rolling,weekly,monthly}。
		if gjson.GetBytes(body, "usage").IsObject() {
			return ""
		}
		return `missing "usage" object`
	case PlatformZhipu:
		// parseZhipuTokenTiers：data.limits[]。
		if gjson.GetBytes(body, "data.limits").IsArray() {
			return ""
		}
		return `missing "data.limits" array`
	case PlatformMiniMax:
		// parseMiniMaxUsageTiers：model_remains[]。
		if gjson.GetBytes(body, "model_remains").IsArray() {
			return ""
		}
		return `missing "model_remains" array`
	default:
		return "unsupported coding plan provider"
	}
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
	if account.IsOpenCodeGoPlan() {
		return nil
	}
	if account.IsCommandCode() {
		if !account.commandCodeUsageSupported() {
			return infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NOT_CODING_PLAN", "command code usage is only available for api key accounts on the official host")
		}
		return nil
	}
	if account.IsCline() {
		if !account.clineAccountAPISupported() {
			return infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NOT_CODING_PLAN", "cline usage is only available for api key accounts on the official host")
		}
		return nil
	}
	if account.IsOpenCodeGo() {
		return infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NOT_CODING_PLAN", "opencode zen accounts have no subscription quota window")
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

// zhipuQuotaURL 根据 base_url 解析智谱额度端点（与数据面推理域名同主机）。
func zhipuQuotaURL(baseURL string) string {
	return zhipuQuotaHost(baseURL) + "/api/monitor/usage/quota/limit"
}

// kimiQuotaURL 根据 base_url 解析 Kimi For Coding 额度端点。
// cc-switch query_kimi 固定探测 https://api.kimi.com/coding/v1/usages
// （实测 /coding/usages 无 /v1 → 404）。coding/v1（CC 协议默认）与
// coding（Anthropic 协议默认）两种 base 统一剥掉尾部后拼回 /v1/usages，
// 协议切换不影响额度探测端点。
func kimiQuotaURL(baseURL string) string {
	base := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
	return base + "/v1/usages"
}

// minimaxQuotaURL 根据推理域名选择 Token Plan / Coding Plan 额度主机。
// 官方 FAQ 写 www.minimax.io / www.minimaxi.com，实际以 Bearer Key 打 api.*。
// 国际站 api.minimax.io；国内站 api.minimaxi.com（含 api.minimax.com 与自定义回落）。
func minimaxQuotaURL(baseURL string) string {
	if strings.Contains(strings.ToLower(baseURL), "minimax.io") {
		return "https://api.minimax.io/v1/api/openplatform/coding_plan/remains"
	}
	return "https://api.minimaxi.com/v1/api/openplatform/coding_plan/remains"
}

func zhipuQuotaHost(baseURL string) string {
	switch u := strings.ToLower(baseURL); {
	case strings.Contains(u, "bigmodel.cn"):
		return "https://open.bigmodel.cn"
	case strings.Contains(u, "z.ai"):
		return "https://api.z.ai"
	default:
		// 国产优先：未知域名回落国内站（与前端 zhipu 预设一致）。
		return "https://open.bigmodel.cn"
	}
}

// cnQuotaParseError 表示「窗口已声明，但必需字段缺失/非法」。
//
// 关键区分：
//   - 窗口压根没声明 → 视为缺席，正常返回，cnQuotaExtraUpdates 会把该窗口清掉；
//   - 窗口声明了但字段读不出来 → 整次探测判失败，不落快照，保留旧快照。
//
// 否则一个字段缺失就会被当成「0% 或窗口消失」，把 90% 的有效快照抹掉、
// 顺带解除阈值停调。
type cnQuotaParseError struct {
	Provider string
	Window   string
	Field    string
	Reason   string
}

func (e *cnQuotaParseError) Error() string {
	scope := e.Provider + " payload"
	if e.Window != "" {
		scope = fmt.Sprintf("%s %s window", e.Provider, e.Window)
	}
	return fmt.Sprintf("%s: field %q %s", scope, e.Field, e.Reason)
}

func cnQuotaFieldError(provider, window, field, reason string) error {
	return &cnQuotaParseError{Provider: provider, Window: window, Field: field, Reason: reason}
}

const (
	cnQuotaReasonPositiveNumber = "is missing or not a positive number"
	cnQuotaReasonNonNegative    = "is missing or not a non-negative number"
)

// cnRequiredNonNegative 读取一个必须存在且 >= 0 的数值字段。
func cnRequiredNonNegative(node gjson.Result, provider, window, field string) (float64, error) {
	value, ok := cnParseF64(node.Get(field).Value())
	if !ok || value < 0 {
		return 0, cnQuotaFieldError(provider, window, field, cnQuotaReasonNonNegative)
	}
	return value, nil
}

// parseKimiUsageTiers 解析 Kimi For Coding 的 /usages 响应。
//
//   - limits[].detail.{limit,remaining,resetTime} → 5h 窗口（取首个 detail）
//   - usage.{limit,remaining,resetTime} → 周窗口
//
// utilization = (limit-remaining)/limit*100。
// detail / usage 未声明 → 该窗口缺席；声明了但 limit/remaining 读不出来 → 解析失败。
func parseKimiUsageTiers(body []byte) ([]CNQuotaTier, error) {
	var tiers []CNQuotaTier

	var fiveHourErr error
	if limits := gjson.GetBytes(body, "limits"); limits.IsArray() {
		limits.ForEach(func(_, item gjson.Result) bool {
			detail := item.Get("detail")
			if !detail.Exists() {
				return true // 没有 detail 的条目跳过，继续找下一个
			}
			tier, err := kimiWindowTier(detail, "5h")
			if err != nil {
				fiveHourErr = err
				return false
			}
			tiers = append(tiers, tier)
			return false // 取首个 detail 作为 5h 窗口
		})
	}
	if fiveHourErr != nil {
		return nil, fiveHourErr
	}

	if usage := gjson.GetBytes(body, "usage"); usage.Exists() {
		tier, err := kimiWindowTier(usage, "weekly")
		if err != nil {
			return nil, err
		}
		tiers = append(tiers, tier)
	}

	return tiers, nil
}

// kimiWindowTier 从 {limit, remaining, resetTime} 构造窗口。
// limit 必须 > 0：limit<=0 没有可用的分母，旧实现会静默产出一条 0% 的假窗口。
// remaining > limit 仍按 0 已用夹住（上游偶发的超发值，不当错误）。
func kimiWindowTier(node gjson.Result, window string) (CNQuotaTier, error) {
	limit, ok := cnParseF64(node.Get("limit").Value())
	if !ok || limit <= 0 {
		return CNQuotaTier{}, cnQuotaFieldError(PlatformKimi, window, "limit", cnQuotaReasonPositiveNumber)
	}
	remaining, err := cnRequiredNonNegative(node, PlatformKimi, window, "remaining")
	if err != nil {
		return CNQuotaTier{}, err
	}
	used := limit - remaining
	if used < 0 {
		used = 0
	}
	return CNQuotaTier{
		Window:      window,
		UsedPercent: used / limit * 100,
		ResetAt:     cnNormalizeResetTime(node.Get("resetTime").Value()),
	}, nil
}

// parseMiniMaxUsageTiers 解析 MiniMax Token Plan / Coding Plan remains 响应。
//
// 只取 model_name == "general"（编程套餐），跳过 video。字段是剩余百分比，
// 展示已用 = 100 - remaining：
//   - 5h：current_interval_remaining_percent + end_time
//   - 周限额：仅 current_weekly_status == 1 时用 current_weekly_remaining_percent + weekly_end_time
//
// current_weekly_status != 1 → 周限额未开启（窗口缺席，不是错误）；
// 开启了却读不到 current_weekly_remaining_percent → 解析失败，不落快照。
func parseMiniMaxUsageTiers(body []byte) ([]CNQuotaTier, error) {
	remains := gjson.GetBytes(body, "model_remains")
	if !remains.IsArray() {
		return nil, cnQuotaFieldError(PlatformMiniMax, "", "model_remains", "is missing or not an array")
	}
	var general gjson.Result
	remains.ForEach(func(_, item gjson.Result) bool {
		if strings.EqualFold(strings.TrimSpace(item.Get("model_name").String()), "general") {
			general = item
			return false
		}
		return true
	})
	// 容器校验已确认这是一份额度响应，却没有编程套餐条目 → 结构不可信。
	if !general.Exists() {
		return nil, cnQuotaFieldError(PlatformMiniMax, "", `model_remains[model_name="general"]`, "is missing")
	}

	var tiers []CNQuotaTier
	remaining, err := cnRequiredNonNegative(general, PlatformMiniMax, "5h", "current_interval_remaining_percent")
	if err != nil {
		return nil, err
	}
	tiers = append(tiers, minimaxTier("5h", remaining, general.Get("end_time")))

	if general.Get("current_weekly_status").Int() == 1 {
		weeklyRemaining, err := cnRequiredNonNegative(general, PlatformMiniMax, "weekly", "current_weekly_remaining_percent")
		if err != nil {
			return nil, err
		}
		tiers = append(tiers, minimaxTier("weekly", weeklyRemaining, general.Get("weekly_end_time")))
	}
	return tiers, nil
}

// minimaxTier 把「剩余百分比」换算成已用百分比（上游偶发 >100 的剩余值夹到 0 已用）。
func minimaxTier(window string, remainingPercent float64, resetNode gjson.Result) CNQuotaTier {
	used := 100 - remainingPercent
	if used < 0 {
		used = 0
	}
	return CNQuotaTier{Window: window, UsedPercent: used, ResetAt: minimaxResetTime(resetNode)}
}

func minimaxResetTime(v gjson.Result) string {
	if !v.Exists() {
		return ""
	}
	ms := v.Int()
	if ms <= 0 {
		return cnNormalizeResetTime(v.Value())
	}
	if ms < 1_000_000_000_000 {
		return time.Unix(ms, 0).UTC().Format(time.RFC3339)
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// cnZhipuWindow 标识智谱 TOKENS_LIMIT 条目所属窗口。
type cnZhipuWindow int

const (
	cnZhipuWindowUnknown cnZhipuWindow = iota
	cnZhipuWindow5h
	cnZhipuWindowWeekly
)

// classifyZhipuWindowUnit 按 unit 字段判定窗口类型（3=5h，6=weekly）。
// unit 缺失或未识别时返回 Unknown，由调用方走 reset 时间启发式兜底。
func classifyZhipuWindowUnit(unit int64) cnZhipuWindow {
	switch unit {
	case 3:
		return cnZhipuWindow5h
	case 6:
		return cnZhipuWindowWeekly
	default:
		return cnZhipuWindowUnknown
	}
}

// parseZhipuTokenTiers 解析智谱额度响应 data.limits 为 5h + weekly 两档。
//
// 分类优先级（对齐 cc-switch parse_zhipu_token_tiers，issue #3036）：
//  1. 显式 unit 字段（3=5h / 6=weekly）——不能用 reset 排序代替，周期末尾
//     周窗口会比 5h 更早重置，时间排序必然标反。
//  2. unit 缺失/未识别：无 nextResetTime 的条目优先归 5h（0% 状态下 5h 桶可能
//     没有 reset），其余按 reset 升序依次填入仍空缺的槽位。
//
// CREDIT_LIMIT（信用额度）与 TOKENS_LIMIT（token 窗口）度量不同：两者同时返回时
// 只让 TOKENS_LIMIT 参与 5h/weekly 槽位竞争，避免信用额度百分比污染阈值停调
// 快照；仅当无任何 TOKENS_LIMIT 条目时才降级用 CREDIT_LIMIT 展示。
// 老套餐只回 1 条 TOKENS_LIMIT，自然降级为仅 5h；新套餐回 2 条。
func parseZhipuTokenTiers(data gjson.Result) ([]CNQuotaTier, error) {
	type entry struct {
		resetMs    int64
		hasReset   bool
		percentage float64
		resetISO   string
	}
	var (
		fiveHour     entry
		fiveHourSet  bool
		weekly       entry
		weeklySet    bool
		unclassified []entry
	)

	classify := func(item gjson.Result, e entry) {
		switch classifyZhipuWindowUnit(item.Get("unit").Int()) {
		case cnZhipuWindow5h:
			if !fiveHourSet {
				fiveHour, fiveHourSet = e, true
			} else {
				unclassified = append(unclassified, e)
			}
		case cnZhipuWindowWeekly:
			if !weeklySet {
				weekly, weeklySet = e, true
			} else {
				unclassified = append(unclassified, e)
			}
		default:
			unclassified = append(unclassified, e)
		}
	}
	var creditFallback []entry
	hasTokensLimit := false

	var parseErr error
	data.Get("limits").ForEach(func(_, item gjson.Result) bool {
		limitType := strings.ToUpper(strings.TrimSpace(item.Get("type").String()))
		if limitType != "TOKENS_LIMIT" && limitType != "CREDIT_LIMIT" {
			return true // 其它 limit 类型本来就不参与窗口，跳过不算错
		}
		// 条目已经声明了窗口，percentage 读不出来就不能当 0% 用：那会把真实用量抹平。
		percentage, err := cnRequiredNonNegative(item, PlatformZhipu, zhipuWindowLabel(item), "percentage")
		if err != nil {
			parseErr = err
			return false
		}
		var (
			resetMs  int64
			hasReset bool
			resetISO string
		)
		if nr := item.Get("nextResetTime"); nr.Exists() {
			switch nr.Type {
			case gjson.Number:
				resetMs = nr.Int()
				hasReset = resetMs > 0
				resetISO = cnMillisToRFC3339(resetMs)
			case gjson.String:
				resetISO = cnNormalizeResetTime(nr.String())
				hasReset = resetISO != ""
			}
		}
		e := entry{resetMs: resetMs, hasReset: hasReset, percentage: percentage, resetISO: resetISO}
		if limitType == "TOKENS_LIMIT" {
			hasTokensLimit = true
			classify(item, e)
		} else {
			creditFallback = append(creditFallback, e)
		}
		return true
	})
	if parseErr != nil {
		return nil, parseErr
	}

	// 无任何 TOKENS_LIMIT 条目（部分套餐只报信用额度）：降级用 CREDIT_LIMIT 展示。
	if !hasTokensLimit {
		unclassified = append(unclassified, creditFallback...)
	}

	// 无 reset 的条目排前，再按 reset 升序，依次填入仍空缺的槽位。
	sort.SliceStable(unclassified, func(i, j int) bool {
		if unclassified[i].hasReset != unclassified[j].hasReset {
			return !unclassified[i].hasReset
		}
		return unclassified[i].resetMs < unclassified[j].resetMs
	})
	for _, e := range unclassified {
		switch {
		case !fiveHourSet:
			fiveHour, fiveHourSet = e, true
		case !weeklySet:
			weekly, weeklySet = e, true
		}
	}

	var tiers []CNQuotaTier
	if fiveHourSet {
		tiers = append(tiers, CNQuotaTier{Window: "5h", UsedPercent: fiveHour.percentage, ResetAt: fiveHour.resetISO})
	}
	if weeklySet {
		tiers = append(tiers, CNQuotaTier{Window: "weekly", UsedPercent: weekly.percentage, ResetAt: weekly.resetISO})
	}
	return tiers, nil
}

// zhipuWindowLabel 给错误信息用的窗口名：unit 能判就报 5h/weekly，
// 判不出来就留空（此时条目还没进槽位，报「payload」更诚实）。
func zhipuWindowLabel(item gjson.Result) string {
	switch classifyZhipuWindowUnit(item.Get("unit").Int()) {
	case cnZhipuWindow5h:
		return "5h"
	case cnZhipuWindowWeekly:
		return "weekly"
	default:
		return ""
	}
}

// cnQuotaExtraUpdates 根据 tier 列表构造 provider 维度的 Extra 快照更新。
//
// 探测成功后的快照必须与本次返回的窗口集合完全一致：先把 5h / weekly / monthly
// 三组键一律置空，再用本次 tiers 覆盖。否则上游停发某个窗口
// （如 MiniMax current_weekly_status != 1、智谱换套餐）后，旧值会永远留在 extra 里，
// 继续被展示、继续参与阈值停调。
//
// 这里用 nil 而不是删键：UpdateExtra 走 JSONB `extra || $1::jsonb` 合并，删不掉键，
// nil 会写成 JSON null。读侧（cnThresholdCandidate / cnProviderQuotaSnapshotReset /
// 前端 CNProviderQuotaCell）统一把 null 当作「窗口不存在」。
// 注意：只有探测成功才会走到这里，失败路径不落快照，旧值原样保留。
func cnQuotaExtraUpdates(provider string, tiers []CNQuotaTier, now time.Time) map[string]any {
	updates := map[string]any{
		cnExtraKey(provider, cnExtraSuffixUsageUpdated): now.Format(time.RFC3339),
		cnExtraKey(provider, cnExtraSuffix5hUsed):       nil,
		cnExtraKey(provider, cnExtraSuffix5hReset):      nil,
		cnExtraKey(provider, cnExtraSuffixWeeklyUsed):   nil,
		cnExtraKey(provider, cnExtraSuffixWeeklyReset):  nil,
		cnExtraKey(provider, cnExtraSuffixMonthlyUsed):  nil,
		cnExtraKey(provider, cnExtraSuffixMonthlyReset): nil,
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
		case "monthly":
			updates[cnExtraKey(provider, cnExtraSuffixMonthlyUsed)] = t.UsedPercent
			if t.ResetAt != "" {
				updates[cnExtraKey(provider, cnExtraSuffixMonthlyReset)] = t.ResetAt
			}
		}
	}
	return updates
}

// parseOpenCodeGoUsageTiers 解析 OpenCode Go GET /usage 响应。
//
// 结构对齐 cc-switch extractor：
//
//	{ "usage": { "rolling": {percent, resetsAt}, "weekly": {...}, "monthly": {...} } }
//
// percent 为已用百分比（0-100）；rolling 映射为 5h 窗口。
// 窗口节点不存在 → 该窗口缺席；节点在但 percent 读不出来 → 解析失败，不落快照。
func parseOpenCodeGoUsageTiers(body []byte) ([]CNQuotaTier, error) {
	usage := gjson.GetBytes(body, "usage")
	if !usage.Exists() {
		return nil, cnQuotaFieldError(PlatformOpenCodeGo, "", "usage", "is missing")
	}
	var tiers []CNQuotaTier
	for _, item := range []struct {
		key    string
		window string
	}{
		{key: "rolling", window: "5h"},
		{key: "weekly", window: "weekly"},
		{key: "monthly", window: "monthly"},
	} {
		node := usage.Get(item.key)
		if !node.Exists() {
			continue
		}
		used, err := cnRequiredNonNegative(node, PlatformOpenCodeGo, item.window, "percent")
		if err != nil {
			return nil, err
		}
		tiers = append(tiers, CNQuotaTier{
			Window:      item.window,
			UsedPercent: used,
			ResetAt:     cnNormalizeResetTime(node.Get("resetsAt").Value()),
		})
	}
	return tiers, nil
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

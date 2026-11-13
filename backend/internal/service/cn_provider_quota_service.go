package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
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

	var tiers []CNQuotaTier
	switch provider {
	case PlatformKimi:
		tiers = parseKimiUsageTiers(bodyBytes)
	case PlatformOpenCodeGo:
		tiers = parseOpenCodeGoUsageTiers(bodyBytes)
		result.PlanLevel = "OpenCode Go"
	case PlatformZhipu:
		tiers = parseZhipuTokenTiers(gjson.GetBytes(bodyBytes, "data"))
		result.PlanLevel = strings.TrimSpace(gjson.GetBytes(bodyBytes, "data.level").String())
	case PlatformMiniMax:
		if status := gjson.GetBytes(bodyBytes, "base_resp.status_code"); status.Exists() && status.Int() != 0 {
			msg := strings.TrimSpace(gjson.GetBytes(bodyBytes, "base_resp.status_msg").String())
			if msg == "" {
				msg = "unknown minimax quota error"
			}
			result.Error = fmt.Sprintf("API error (%d): %s", status.Int(), msg)
			return result, nil
		}
		tiers = parseMiniMaxUsageTiers(bodyBytes)
		result.PlanLevel = strings.TrimSpace(gjson.GetBytes(bodyBytes, "current_subscribe_title").String())
	}
	result.Tiers = tiers
	// 2xx 但一个可用窗口都解析不出来，通常意味着上游报文改版/字段改名，而不是
	// 「额度无限」。必须按失败处理：保留旧快照、不写成功时间；否则下面的缺席
	// 清理会把 5h/weekly/monthly 全写 nil，调度层看到「没有窗口」就会解除预停调，
	// 让仍处于耗尽的账号回池撞 429。
	if len(tiers) == 0 {
		result.Error = fmt.Sprintf(
			"quota response for %s contains no recognized usage window (upstream schema may have changed); keeping the previous snapshot",
			provider)
		slog.Warn("cn_quota_no_recognized_window",
			"account_id", account.ID, "provider", provider, "http_status", resp.StatusCode)
		return result, nil
	}
	result.Success = true
	result.CredentialValid = true

	updates := cnQuotaExtraUpdates(provider, tiers, now)
	// 只对 5h/weekly/monthly 三档通吃的供应商清理缺席窗口；Command Code 与
	// Cline 走各自的取舍（抓取失败时保留月度），不在这里处理。
	if provider == PlatformKimi || provider == PlatformZhipu || provider == PlatformMiniMax || provider == PlatformOpenCodeGo {
		clearAbsentCNQuotaWindows(provider, tiers, updates)
	}
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

// parseKimiUsageTiers 解析 Kimi For Coding 的 /usages 响应。
//
// Kimi 目前并存两种套餐形态：
//
//   - 老套餐（5h + 7d）：limits[].detail.{limit,remaining,resetTime} → 5h
//     （取首个 detail），顶层 usage.{limit,remaining,resetTime} → 周窗口，
//     utilization = (limit-remaining)/limit*100。
//   - 新套餐（5h + 月度）：不再有顶层 usage，改由 usages.limit_5h /
//     usages.limit_7d / usages.limit_month_total / usages.limit_month_code
//     以 used_ratio（0-1）+ reset_time 上报。月度 total 与 code 任一耗尽都会
//     限流，因此 monthly 取两者较大用量、较早重置时间。
//
// 两者同时出现时 used_ratio 优先（比 counts 更贴近官方口径），顶层 usage 仅兜底；
// usages.limit_5h 只在 limits[] 没有 detail 时补位，避免重复产出 5h 档。
func parseKimiUsageTiers(body []byte) []CNQuotaTier {
	var tiers []CNQuotaTier

	hasFiveHour := false
	if limits := gjson.GetBytes(body, "limits"); limits.IsArray() {
		limits.ForEach(func(_, item gjson.Result) bool {
			detail := item.Get("detail")
			if !detail.Exists() {
				return true
			}
			// 字段节点存在不等于数值可信：limit/remaining 必须可解析、limit>0、
			// remaining 落在 [0, limit]，否则跳过该条（`{"detail":{}}` 或 limit=0
			// 不能产出一个「已用 0%」的伪窗口，那会掩盖真实的额度耗尽状态）。
			limit, okLimit := cnParseF64(detail.Get("limit").Value())
			remaining, okRemaining := cnParseF64(detail.Get("remaining").Value())
			if !okLimit || !okRemaining || limit <= 0 || remaining < 0 || remaining > limit {
				return true
			}
			util := (limit - remaining) / limit * 100
			tiers = append(tiers, CNQuotaTier{
				Window:      "5h",
				UsedPercent: util,
				ResetAt:     cnNormalizeResetTime(detail.Get("resetTime").Value()),
			})
			hasFiveHour = true
			return false // 取首个有效 detail 作为 5h 窗口
		})
	}
	if !hasFiveHour {
		if tier, ok := kimiUsedRatioTier(body, "limit_5h", "5h"); ok {
			tiers = append(tiers, tier)
			hasFiveHour = true
		}
	}

	// 周窗口：usages.limit_7d 优先，顶层 usage（老套餐单数）兜底。
	if tier, ok := kimiUsedRatioTier(body, "limit_7d", "weekly"); ok {
		tiers = append(tiers, tier)
	} else if usage := gjson.GetBytes(body, "usage"); usage.Exists() {
		limit, okLimit := cnParseF64(usage.Get("limit").Value())
		remaining, okRemaining := cnParseF64(usage.Get("remaining").Value())
		if okLimit && okRemaining && limit > 0 && remaining >= 0 && remaining <= limit {
			tiers = append(tiers, CNQuotaTier{
				Window:      "weekly",
				UsedPercent: (limit - remaining) / limit * 100,
				ResetAt:     cnNormalizeResetTime(usage.Get("resetTime").Value()),
			})
		}
	}

	// 月度限额（新套餐）：total 与 code 是两个**独立**的限流窗口，任一耗尽都会限流。
	// 用量取两者较大值；重置时间取**较晚**者——取较早会在另一个窗口仍耗尽时提前
	// 解除预停调（例如 total 到 11-26 才恢复、code 到 11-20 恢复）。宁可晚回池，
	// 不可提前撞 429。
	total, hasTotal := kimiUsedRatioTier(body, "limit_month_total", "monthly")
	code, hasCode := kimiUsedRatioTier(body, "limit_month_code", "monthly")
	if hasTotal || hasCode {
		monthly := total
		if !hasTotal {
			monthly = code
		} else if hasCode {
			if code.UsedPercent > monthly.UsedPercent {
				monthly.UsedPercent = code.UsedPercent
			}
			if code.ResetAt != "" && (monthly.ResetAt == "" || code.ResetAt > monthly.ResetAt) {
				monthly.ResetAt = code.ResetAt
			}
		}
		tiers = append(tiers, monthly)
	}

	return tiers
}

// kimiUsedRatioTier 解析 usages.<key>.{used_ratio,reset_time} 为窗口档位。
// used_ratio 是 0-1 比例，输出换算为百分比；键缺失、不可解析或越界返回 false。
func kimiUsedRatioTier(body []byte, key, window string) (CNQuotaTier, bool) {
	node := gjson.GetBytes(body, "usages."+key)
	if !node.Exists() {
		return CNQuotaTier{}, false
	}
	ratioNode := node.Get("used_ratio")
	if !ratioNode.Exists() {
		return CNQuotaTier{}, false
	}
	ratio, ok := cnParseF64(ratioNode.Value())
	if !ok {
		return CNQuotaTier{}, false
	}
	// 必须落在上游声明区间 [0,1]：越界值（例如把百分数 65 直接塞进 used_ratio）
	// 会算出 6500%，NaN/Inf 同理不可信。
	if ratio < 0 || ratio > 1 {
		return CNQuotaTier{}, false
	}
	return CNQuotaTier{
		Window:      window,
		UsedPercent: ratio * 100,
		ResetAt:     cnNormalizeResetTime(node.Get("reset_time").Value()),
	}, true
}

// parseMiniMaxUsageTiers 解析 MiniMax Token Plan / Coding Plan remains 响应。
//
// 只取 model_name == "general"（编程套餐），跳过 video。字段是剩余百分比，
// 展示已用 = 100 - remaining：
//   - 5h：current_interval_remaining_percent + end_time
//   - 周限额：仅 current_weekly_status == 1 时用 current_weekly_remaining_percent + weekly_end_time
func parseMiniMaxUsageTiers(body []byte) []CNQuotaTier {
	remains := gjson.GetBytes(body, "model_remains")
	if !remains.IsArray() {
		return nil
	}
	var general gjson.Result
	remains.ForEach(func(_, item gjson.Result) bool {
		if strings.EqualFold(strings.TrimSpace(item.Get("model_name").String()), "general") {
			general = item
			return false
		}
		return true
	})
	if !general.Exists() {
		return nil
	}

	var tiers []CNQuotaTier
	if remaining, ok := cnParseF64(general.Get("current_interval_remaining_percent").Value()); ok {
		used := 100 - remaining
		if used < 0 {
			used = 0
		}
		tiers = append(tiers, CNQuotaTier{
			Window:      "5h",
			UsedPercent: used,
			ResetAt:     minimaxResetTime(general.Get("end_time")),
		})
	}
	if general.Get("current_weekly_status").Int() == 1 {
		if remaining, ok := cnParseF64(general.Get("current_weekly_remaining_percent").Value()); ok {
			used := 100 - remaining
			if used < 0 {
				used = 0
			}
			tiers = append(tiers, CNQuotaTier{
				Window:      "weekly",
				UsedPercent: used,
				ResetAt:     minimaxResetTime(general.Get("weekly_end_time")),
			})
		}
	}
	return tiers
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

// cnZhipuWindow 标识智谱额度条目（TOKENS_LIMIT / CREDIT_LIMIT）所属窗口。
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
// 快照；仅当无任何 TOKENS_LIMIT 条目时才降级用 CREDIT_LIMIT 展示 —— 此时
// CREDIT_LIMIT 同样按 unit 分类（它也携带 unit），不依赖 reset 时间排序。
// 老套餐只回 1 条 TOKENS_LIMIT，自然降级为仅 5h；新套餐回 2 条。
func parseZhipuTokenTiers(data gjson.Result) []CNQuotaTier {
	type entry struct {
		resetMs    int64
		hasReset   bool
		percentage float64
		resetISO   string
		unit       int64
	}
	var (
		fiveHour     entry
		fiveHourSet  bool
		weekly       entry
		weeklySet    bool
		unclassified []entry
	)

	classify := func(unit int64, e entry) {
		switch classifyZhipuWindowUnit(unit) {
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

	data.Get("limits").ForEach(func(_, item gjson.Result) bool {
		limitType := strings.ToUpper(strings.TrimSpace(item.Get("type").String()))
		if limitType != "TOKENS_LIMIT" && limitType != "CREDIT_LIMIT" {
			return true
		}
		// percentage 缺失/非数字/NaN/越界时不能默认 0：那会产出一个「已用 0%」的
		// 合法窗口，既掩盖真实耗尽状态，也会被缺席清理当成有效快照落库。
		percentage, okPercentage := cnParseF64(item.Get("percentage").Value())
		if !okPercentage || percentage < 0 || percentage > 100 {
			return true
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
		e := entry{resetMs: resetMs, hasReset: hasReset, percentage: percentage, resetISO: resetISO, unit: item.Get("unit").Int()}
		if limitType == "TOKENS_LIMIT" {
			hasTokensLimit = true
			classify(e.unit, e)
		} else {
			creditFallback = append(creditFallback, e)
		}
		return true
	})

	// 无任何 TOKENS_LIMIT 条目（部分套餐只报信用额度）：降级用 CREDIT_LIMIT 展示。
	// CREDIT_LIMIT 条目同样带 unit（3=5h / 6=weekly；实测 GLM Coding Plan 两条窗口
	// 都是 CREDIT_LIMIT），必须和 TOKENS_LIMIT 一样先按 unit 显式分类 —— 否则会退化
	// 成 reset 时间启发式，而周期末尾周窗口比 5h 更早重置，时间排序必然把两档标反。
	if !hasTokensLimit {
		for _, e := range creditFallback {
			if classifyZhipuWindowUnit(e.unit) != cnZhipuWindowUnknown {
				classify(e.unit, e)
				continue
			}
			unclassified = append(unclassified, e)
		}
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
	return tiers
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
		case "monthly":
			updates[cnExtraKey(provider, cnExtraSuffixMonthlyUsed)] = t.UsedPercent
			if t.ResetAt != "" {
				updates[cnExtraKey(provider, cnExtraSuffixMonthlyReset)] = t.ResetAt
			}
		}
	}
	return updates
}

// clearAbsentCNQuotaWindows 把本次探测缺席的窗口显式写 nil（JSONB 合并语义下等于
// 移除按键）：账号 extra 的更新走 `extra || $1`，只"不写"会保留旧值。上游改套餐
// （Kimi 老套餐 7d → 新套餐月度）后旧窗口不再上报，不清理就会把切换前的用量永久
// 冻结在快照里（生产实测冻结 13 天，列表一直显示过期的 7D 用量）。
//
// 仅供 5h/weekly/monthly 三档通吃的供应商调用；Command Code / Cline 有自己的
// 取舍（周期端点抓取失败时保留月度值），不在此列。
func clearAbsentCNQuotaWindows(provider string, tiers []CNQuotaTier, updates map[string]any) {
	present := make(map[string]CNQuotaTier, len(tiers))
	for _, tier := range tiers {
		present[tier.Window] = tier
	}
	for _, window := range []struct{ name, used, reset string }{
		{"5h", cnExtraSuffix5hUsed, cnExtraSuffix5hReset},
		{"weekly", cnExtraSuffixWeeklyUsed, cnExtraSuffixWeeklyReset},
		{"monthly", cnExtraSuffixMonthlyUsed, cnExtraSuffixMonthlyReset},
	} {
		tier, ok := present[window.name]
		if !ok {
			updates[cnExtraKey(provider, window.used)] = nil
			updates[cnExtraKey(provider, window.reset)] = nil
			continue
		}
		if tier.ResetAt == "" {
			updates[cnExtraKey(provider, window.reset)] = nil
		}
	}
}

// parseOpenCodeGoUsageTiers 解析 OpenCode Go GET /usage 响应。
//
// 结构对齐 cc-switch extractor：
//
//	{ "usage": { "rolling": {percent, resetsAt}, "weekly": {...}, "monthly": {...} } }
//
// percent 为已用百分比（0-100）；rolling 映射为 5h 窗口。
func parseOpenCodeGoUsageTiers(body []byte) []CNQuotaTier {
	usage := gjson.GetBytes(body, "usage")
	if !usage.Exists() {
		return nil
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
		percentNode := node.Get("percent")
		if !percentNode.Exists() {
			continue
		}
		used, ok := cnParseF64(percentNode.Value())
		if !ok {
			continue
		}
		if used < 0 {
			used = 0
		}
		tiers = append(tiers, CNQuotaTier{
			Window:      item.window,
			UsedPercent: used,
			ResetAt:     cnNormalizeResetTime(node.Get("resetsAt").Value()),
		})
	}
	return tiers
}

// cnParseF64 把 JSON 数值或字符串解析为 float64（兼容 "100" 与 100）。
func cnParseF64(raw any) (float64, bool) {
	var value float64
	switch v := raw.(type) {
	case float64:
		value = v
	case float32:
		value = float64(v)
	case int:
		value = float64(v)
	case int64:
		value = float64(v)
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		value = f
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		value = f
	default:
		return 0, false
	}
	// strconv.ParseFloat 接受 "NaN"/"Inf"，JSON 的 1e999 也会得到 +Inf；
	// 这类值参与百分比与阈值比较会让停调判定失效，一律视为不可解析。
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
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

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

const (
	forbiddenTypeValidation = "validation"
	forbiddenTypeViolation  = "violation"
	forbiddenTypeForbidden  = "forbidden"

	// 机器可读的错误码
	errorCodeForbidden       = "forbidden"
	errorCodeUnauthenticated = "unauthenticated"
	errorCodeRateLimited     = "rate_limited"
	errorCodeNetworkError    = "network_error"
)

// AntigravityQuotaFetcher 从 Antigravity API 获取额度
type AntigravityQuotaFetcher struct {
	proxyRepo ProxyRepository
	cfg       *config.Config
}

// NewAntigravityQuotaFetcher 创建 AntigravityQuotaFetcher
func NewAntigravityQuotaFetcher(proxyRepo ProxyRepository, cfg *config.Config) *AntigravityQuotaFetcher {
	return &AntigravityQuotaFetcher{proxyRepo: proxyRepo, cfg: cfg}
}

// CanFetch 检查是否可以获取此账户的额度
func (f *AntigravityQuotaFetcher) CanFetch(account *Account) bool {
	if account.Platform != PlatformAntigravity {
		return false
	}
	accessToken := account.GetCredential("access_token")
	return accessToken != ""
}

// resolveAntigravityQuotaBaseURLs 返回用量查询使用的有序端点队列。
//
// 与转发侧保持一致（参考 Antigravity Tools / Antigravity-Manager 实现）：
// 1. 显式环境变量 GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL (daily/prod) 优先级最高；
// 2. 默认采用 antigravity.ForwardBaseURLs() 顺序（优先 daily，其次 prod），
//    确保配额探测端点与实际请求转发端点一致，避免因生产端点配额不同步导致配额恒为 0 或限流感知失效。
func resolveAntigravityQuotaBaseURLs(account *Account) []string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(antigravityForwardBaseURLEnv)))
	if mode == "prod" {
		return append([]string(nil), antigravity.BaseURLs...)
	}
	if mode == "daily" || mode == "sandbox" {
		return antigravity.ForwardBaseURLs()
	}

	// 默认使用 ForwardBaseURLs()（daily 优先），与实际运行时转发端点保持完全一致
	return antigravity.ForwardBaseURLs()
}

// FetchQuota 获取 Antigravity 账户额度信息
func (f *AntigravityQuotaFetcher) FetchQuota(ctx context.Context, account *Account, proxyURL string) (*QuotaResult, error) {
	accessToken := account.GetCredential("access_token")
	projectID := account.GetCredential("project_id")

	client, err := antigravity.NewClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create antigravity client failed: %w", err)
	}

	baseURLs := resolveAntigravityQuotaBaseURLs(account)
	bodyLimit := resolveModelsListReadLimit(f.cfg)

	// 调用 API 获取模型目录与配额；Google One / 付费账号优先 daily
	modelsResp, modelsRaw, err := client.FetchAvailableModelsWithURLs(ctx, baseURLs, accessToken, projectID, bodyLimit)
	if err != nil {
		// 403 Forbidden: 不报错，返回 is_forbidden 标记
		var forbiddenErr *antigravity.ForbiddenError
		if errors.As(err, &forbiddenErr) {
			now := time.Now()
			fbType := classifyForbiddenType(forbiddenErr.Body)
			return &QuotaResult{
				UsageInfo: &UsageInfo{
					UpdatedAt:       &now,
					IsForbidden:     true,
					ForbiddenReason: forbiddenErr.Body,
					ForbiddenType:   fbType,
					ValidationURL:   extractValidationURL(forbiddenErr.Body),
					NeedsVerify:     fbType == forbiddenTypeValidation,
					IsBanned:        fbType == forbiddenTypeViolation,
					ErrorCode:       errorCodeForbidden,
				},
			}, nil
		}
		return nil, err
	}

	// 调用 LoadCodeAssist 获取订阅等级和 AI Credits 余额（非关键路径，失败不影响主流程）
	tierRaw, tierNormalized, loadResp := f.fetchSubscriptionTier(ctx, client, baseURLs, accessToken)

	// 官方按模型组返回的 5h / weekly 配额摘要（best-effort，失败不影响目录数据）。
	// fetchAvailableModels 对免费与付费账号都返回同一份全量静态目录且 remainingFraction 恒为 1，
	// 真实用量只在配额摘要里；缺少该步时 Google One / 付费账号会永远显示 0% 用量。
	var quotaSummary *antigravity.QuotaSummaryResponse
	if summary, summaryErr := client.RetrieveUserQuotaSummaryWithURLs(ctx, baseURLs, accessToken, projectID, bodyLimit); summaryErr != nil {
		slog.Warn("failed to fetch antigravity quota summary", "error", summaryErr)
	} else {
		quotaSummary = summary
	}

	// 转换为 UsageInfo
	usageInfo := f.buildUsageInfoWithSummary(modelsResp, tierRaw, tierNormalized, loadResp, quotaSummary)

	return &QuotaResult{
		UsageInfo: usageInfo,
		Raw:       modelsRaw,
	}, nil
}

// fetchSubscriptionTier 获取账号订阅等级，失败返回空字符串。
// 同时返回 LoadCodeAssistResponse，以便提取 AI Credits 余额。
func (f *AntigravityQuotaFetcher) fetchSubscriptionTier(ctx context.Context, client *antigravity.Client, baseURLs []string, accessToken string) (raw, normalized string, loadResp *antigravity.LoadCodeAssistResponse) {
	loadResp, _, err := client.LoadCodeAssistWithURLs(ctx, baseURLs, accessToken)
	if err != nil {
		slog.Warn("failed to fetch subscription tier", "error", err)
		return "", "", nil
	}
	if loadResp == nil {
		return "", "", nil
	}

	raw = loadResp.GetTier() // 已有方法：paidTier > currentTier
	normalized = normalizeTier(raw)
	return raw, normalized, loadResp
}

// normalizeTier 将原始 tier 字符串归一化为 FREE/PRO/ULTRA/UNKNOWN。
//
// 与官方 Antigravity Tools 的口径对齐：
//   - g1-ultra-tier / "Google AI Ultra" / 内部代号 helium (GOOGLE_ONE_HELIUM / GDP_HELIUM) → ULTRA
//   - g1-pro-tier / "Google AI Pro" / Gemini Advanced / premium → PRO
//   - free-tier / "Antigravity Starter Quota" → FREE
func normalizeTier(raw string) string {
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "ultra"), strings.Contains(lower, "helium"):
		return "ULTRA"
	case strings.Contains(lower, "pro"), strings.Contains(lower, "premium"), strings.Contains(lower, "advanced"):
		return "PRO"
	case strings.Contains(lower, "free"), strings.Contains(lower, "starter"):
		return "FREE"
	default:
		return "UNKNOWN"
	}
}

// buildUsageInfo 将 API 响应转换为 UsageInfo。
func (f *AntigravityQuotaFetcher) buildUsageInfo(modelsResp *antigravity.FetchAvailableModelsResponse, tierRaw, tierNormalized string, loadResp *antigravity.LoadCodeAssistResponse) *UsageInfo {
	return f.buildUsageInfoWithSummary(modelsResp, tierRaw, tierNormalized, loadResp, nil)
}

// buildUsageInfoWithSummary 将 API 响应转换为 UsageInfo，并用官方配额摘要
// （retrieveUserQuotaSummary）修正真实用量。summary 为 nil 时与历史行为一致。
func (f *AntigravityQuotaFetcher) buildUsageInfoWithSummary(modelsResp *antigravity.FetchAvailableModelsResponse, tierRaw, tierNormalized string, loadResp *antigravity.LoadCodeAssistResponse, summary *antigravity.QuotaSummaryResponse) *UsageInfo {
	now := time.Now()
	info := &UsageInfo{
		UpdatedAt:               &now,
		AntigravityQuota:        make(map[string]*AntigravityModelQuota),
		AntigravityQuotaDetails: make(map[string]*AntigravityModelDetail),
		SubscriptionTier:        tierNormalized,
		SubscriptionTierRaw:     tierRaw,
	}

	// 遍历所有模型，填充 AntigravityQuota 和 AntigravityQuotaDetails
	for modelName, modelInfo := range modelsResp.Models {
		if modelInfo.QuotaInfo == nil {
			continue
		}

		// remainingFraction 是剩余比例 (0.0-1.0)，转换为使用率百分比（四舍五入，
		// 避免 0.8 这类比例因浮点误差截断成 19%）。
		utilization := antigravityUtilizationFromRemaining(modelInfo.QuotaInfo.RemainingFraction)

		info.AntigravityQuota[modelName] = &AntigravityModelQuota{
			Utilization: utilization,
			ResetTime:   modelInfo.QuotaInfo.ResetTime,
		}

		// 填充模型详细能力信息
		detail := &AntigravityModelDetail{
			DisplayName:        modelInfo.DisplayName,
			SupportsImages:     modelInfo.SupportsImages,
			SupportsThinking:   modelInfo.SupportsThinking,
			ThinkingBudget:     modelInfo.ThinkingBudget,
			Recommended:        modelInfo.Recommended,
			MaxTokens:          modelInfo.MaxTokens,
			MaxOutputTokens:    modelInfo.MaxOutputTokens,
			SupportedMimeTypes: modelInfo.SupportedMimeTypes,
		}
		info.AntigravityQuotaDetails[modelName] = detail
	}

	// 废弃模型转发规则
	if len(modelsResp.DeprecatedModelIDs) > 0 {
		info.ModelForwardingRules = make(map[string]string, len(modelsResp.DeprecatedModelIDs))
		for oldID, deprecated := range modelsResp.DeprecatedModelIDs {
			info.ModelForwardingRules[oldID] = deprecated.NewModelID
		}
	}

	// 用官方 5h / weekly 配额摘要覆盖目录里恒为 1 的 remainingFraction。
	// 必须在推导 FiveHour 之前执行，否则窗口会沿用未修正的目录用量。
	applyAntigravityQuotaSummaryToModels(info, summary)

	// 记录活跃的共享配额桶（gemini-5h / gemini-weekly / 3p-5h / 3p-weekly），
	// 供账号用量单元格按配额组直接展示 5h / 7d 两个窗口。
	// 必须放在模型融合之后，避免合成条目被当成模型再次归类覆盖。
	applyAntigravityQuotaSummaryBuckets(info, summary)

	// 同时设置 FiveHour 用于兼容展示（取主要模型）。
	// 读取已融合的 AntigravityQuota，避免复用目录里对 Google One / 付费账号恒为 1 的假配额。
	priorityModels := []string{"claude-sonnet-4-20250514", "claude-sonnet-4", "gemini-2.5-pro"}
	for _, modelName := range priorityModels {
		quota, ok := info.AntigravityQuota[modelName]
		if !ok || quota == nil {
			continue
		}
		progress := &UsageProgress{
			Utilization: float64(quota.Utilization),
		}
		if quota.ResetTime != "" {
			if resetTime, err := time.Parse(time.RFC3339, quota.ResetTime); err == nil {
				progress.ResetsAt = &resetTime
				progress.RemainingSeconds = int(time.Until(resetTime).Seconds())
			}
		}
		info.FiveHour = progress
		break
	}

	if loadResp != nil {
		for _, credit := range loadResp.GetAvailableCredits() {
			info.AICredits = append(info.AICredits, AICredit{
				CreditType:     credit.CreditType,
				Amount:         credit.GetAmount(),
				MinimumBalance: credit.GetMinimumAmount(),
			})
		}
	}

	// 摘要中的 5h / weekly 桶补齐兼容窗口（上游没有对应窗口时保持原值）。
	applyAntigravityQuotaSummaryWindows(info, summary)

	return info
}

// ── 官方配额摘要（retrieveUserQuotaSummary）融合 ──────────────────────

// antigravityQuotaSummaryExhaustedEpsilon 周配额视为已耗尽的剩余比例阈值。
const antigravityQuotaSummaryExhaustedEpsilon = 0.001

// applyAntigravityQuotaSummaryToModels 用官方按模型组返回的 5h / weekly 配额，
// 覆盖 fetchAvailableModels 目录中恒为 1 的 remainingFraction。
//
// 官方 Antigravity Tools 的实测结论：fetchAvailableModels 对免费号与付费号返回完全相同的
// 全量静态目录，quotaInfo.remainingFraction 恒为 1；若不做覆盖，Google One / Pro 账号的
// 用量会永远显示 0%，这正是「无法正确查询模型用量」的根因。
func applyAntigravityQuotaSummaryToModels(info *UsageInfo, summary *antigravity.QuotaSummaryResponse) {
	if info == nil || summary == nil || len(summary.Groups) == 0 {
		return
	}
	for modelName, quota := range info.AntigravityQuota {
		if quota == nil {
			continue
		}
		group := matchAntigravityQuotaGroup(summary.Groups, modelName)
		if group == nil {
			continue
		}
		bucket := selectAntigravityQuotaBucket(group.Buckets)
		if bucket == nil || bucket.RemainingFraction == nil {
			continue
		}
		quota.Utilization = antigravityUtilizationFromRemaining(*bucket.RemainingFraction)
		if resetTime := strings.TrimSpace(bucket.ResetTime); resetTime != "" {
			quota.ResetTime = resetTime
		}
	}
}

// applyAntigravityQuotaSummaryBuckets 把官方摘要中的活跃配额桶按 bucketId 记录为
// AntigravityQuota 的合成条目（gemini-5h / gemini-weekly / 3p-5h / 3p-weekly），
// 供前端按配额组展示 5h / 7d 两个窗口。
//
// 上游在周配额耗尽时会把对应的 5h 桶标记为 disabled=true 且 remainingFraction=1，
// 这类桶必须跳过，否则会展示一条假的 0% 已用。
func applyAntigravityQuotaSummaryBuckets(info *UsageInfo, summary *antigravity.QuotaSummaryResponse) {
	if info == nil || summary == nil {
		return
	}
	if info.AntigravityQuota == nil {
		info.AntigravityQuota = make(map[string]*AntigravityModelQuota)
	}
	for _, bucket := range allAntigravityQuotaSummaryBuckets(summary) {
		bucketID := strings.TrimSpace(bucket.BucketID)
		if bucketID == "" || bucket.Disabled || bucket.RemainingFraction == nil {
			continue
		}
		info.AntigravityQuota[bucketID] = &AntigravityModelQuota{
			Utilization: antigravityUtilizationFromRemaining(*bucket.RemainingFraction),
			ResetTime:   strings.TrimSpace(bucket.ResetTime),
		}
	}
}

// allAntigravityQuotaSummaryBuckets 汇总顶层 buckets 与各分组内的 buckets。
func allAntigravityQuotaSummaryBuckets(summary *antigravity.QuotaSummaryResponse) []antigravity.QuotaSummaryBucket {
	if summary == nil {
		return nil
	}
	buckets := make([]antigravity.QuotaSummaryBucket, 0, len(summary.Buckets)+len(summary.Groups))
	buckets = append(buckets, summary.Buckets...)
	for i := range summary.Groups {
		buckets = append(buckets, summary.Groups[i].Buckets...)
	}
	return buckets
}

// applyAntigravityQuotaSummaryWindows 在目录没有给出对应窗口时，
// 用配额摘要中最受限的 5h / weekly 桶补齐 FiveHour / SevenDay 兼容窗口，
// 供渠道监控等消费方使用。已有窗口（由优先模型推导，且已在融合后重算）保持不变。
func applyAntigravityQuotaSummaryWindows(info *UsageInfo, summary *antigravity.QuotaSummaryResponse) {
	if info == nil || summary == nil {
		return
	}
	buckets := allAntigravityQuotaSummaryBuckets(summary)
	if len(buckets) == 0 {
		return
	}
	if info.FiveHour == nil {
		if bucket := mostConstrainedAntigravityQuotaBucket(buckets, false); bucket != nil {
			info.FiveHour = buildAntigravityQuotaProgressFromBucket(bucket)
		}
	}
	if info.SevenDay == nil {
		if bucket := mostConstrainedAntigravityQuotaBucket(buckets, true); bucket != nil {
			info.SevenDay = buildAntigravityQuotaProgressFromBucket(bucket)
		}
	}
}

// matchAntigravityQuotaGroup 按模型名匹配官方配额分组（argon / claude+gpt+3p / gemini）。
func matchAntigravityQuotaGroup(groups []antigravity.QuotaSummaryGroup, modelName string) *antigravity.QuotaSummaryGroup {
	kind := antigravityQuotaModelKind(modelName)
	if kind == "" {
		return nil
	}
	for i := range groups {
		if antigravityQuotaGroupKind(groups[i].DisplayName) == kind {
			return &groups[i]
		}
	}
	return nil
}

// antigravityQuotaModelKind 归一化模型所属分组；无法归类时返回空字符串。
func antigravityQuotaModelKind(modelName string) string {
	name := normalizeAntigravityModelName(modelName)
	// 合成配额桶 key（gemini-5h / 3p-weekly 等）不是模型，避免被再次归类覆盖。
	if strings.HasSuffix(name, "-5h") || strings.HasSuffix(name, "-weekly") {
		return ""
	}
	switch {
	case strings.Contains(name, "argon"):
		return "argon"
	case strings.HasPrefix(name, "claude"), strings.HasPrefix(name, "gpt"):
		return "3p"
	case strings.HasPrefix(name, "gemini"):
		return "gemini"
	default:
		return ""
	}
}

// antigravityQuotaGroupKind 归一化官方分组名称；空名称返回空字符串。
func antigravityQuotaGroupKind(displayName string) string {
	name := strings.ToLower(strings.TrimSpace(displayName))
	switch {
	case name == "":
		return ""
	case strings.Contains(name, "argon"):
		return "argon"
	case strings.Contains(name, "claude"), strings.Contains(name, "gpt"), strings.Contains(name, "3p"):
		return "3p"
	default:
		// 官方其余分组（Gemini Models 等）都归属 Gemini 系配额。
		return "gemini"
	}
}

// selectAntigravityQuotaBucket 在单个分组内挑选真正生效的配额桶。
//
// 规则与官方客户端一致：
//  1. 周配额耗尽时模型直接受限于周配额（重置时间取周重置）；
//  2. 否则取 5h 与 weekly 中剩余更少（更受限）的一个。
func selectAntigravityQuotaBucket(buckets []antigravity.QuotaSummaryBucket) *antigravity.QuotaSummaryBucket {
	var fiveHour, weekly *antigravity.QuotaSummaryBucket
	for i := range buckets {
		bucket := &buckets[i]
		// disabled 的窗口当前不生效（如周配额耗尽后的 5h 桶），不参与选择。
		if bucket.RemainingFraction == nil || bucket.Disabled {
			continue
		}
		window := strings.ToLower(strings.TrimSpace(bucket.Window + " " + bucket.BucketID))
		switch {
		case fiveHour == nil && (strings.Contains(window, "5h") || strings.Contains(window, "hour")):
			fiveHour = bucket
		case weekly == nil && (strings.Contains(window, "week") || strings.Contains(window, "7d")):
			weekly = bucket
		}
	}

	switch {
	case fiveHour != nil && weekly != nil:
		if *weekly.RemainingFraction <= antigravityQuotaSummaryExhaustedEpsilon {
			return weekly
		}
		if *fiveHour.RemainingFraction <= *weekly.RemainingFraction {
			return fiveHour
		}
		return weekly
	case fiveHour != nil:
		return fiveHour
	case weekly != nil:
		return weekly
	}

	// 兜底：窗口标记缺失时取第一个带剩余比例的桶。
	for i := range buckets {
		if buckets[i].RemainingFraction != nil {
			return &buckets[i]
		}
	}
	return nil
}

// mostConstrainedAntigravityQuotaBucket 返回所有候选桶中最受限（剩余最少）的指定窗口桶。
func mostConstrainedAntigravityQuotaBucket(buckets []antigravity.QuotaSummaryBucket, weekly bool) *antigravity.QuotaSummaryBucket {
	var best *antigravity.QuotaSummaryBucket
	for i := range buckets {
		bucket := &buckets[i]
		if bucket.RemainingFraction == nil || bucket.Disabled {
			continue
		}
		window := strings.ToLower(strings.TrimSpace(bucket.Window + " " + bucket.BucketID))
		isWeekly := strings.Contains(window, "week") || strings.Contains(window, "7d")
		isFiveHour := strings.Contains(window, "5h") || strings.Contains(window, "hour")
		if weekly {
			if !isWeekly {
				continue
			}
		} else if !isFiveHour {
			continue
		}
		if best == nil || *bucket.RemainingFraction < *best.RemainingFraction {
			best = bucket
		}
	}
	return best
}

// antigravityUtilizationFromRemaining 将剩余比例 (0-1) 转换为使用率百分比 (0-100)。
func antigravityUtilizationFromRemaining(remaining float64) int {
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 1 {
		remaining = 1
	}
	return int(math.Round((1 - remaining) * 100))
}

// buildAntigravityQuotaProgressFromBucket 将配额桶转换为兼容的 UsageProgress。
func buildAntigravityQuotaProgressFromBucket(bucket *antigravity.QuotaSummaryBucket) *UsageProgress {
	if bucket == nil || bucket.RemainingFraction == nil {
		return nil
	}
	progress := &UsageProgress{
		Utilization: float64(antigravityUtilizationFromRemaining(*bucket.RemainingFraction)),
	}
	if resetTime, err := time.Parse(time.RFC3339, strings.TrimSpace(bucket.ResetTime)); err == nil {
		progress.ResetsAt = &resetTime
		progress.RemainingSeconds = int(time.Until(resetTime).Seconds())
	}
	return progress
}

// GetProxyURL 获取账户的代理 URL
func (f *AntigravityQuotaFetcher) GetProxyURL(ctx context.Context, account *Account) string {
	if account.ProxyID == nil || f.proxyRepo == nil {
		return ""
	}
	proxy, err := f.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil || proxy == nil {
		return ""
	}
	return proxy.URL()
}

// classifyForbiddenType 根据 403 响应体判断禁止类型
func classifyForbiddenType(body string) string {
	lower := strings.ToLower(body)
	switch {
	case strings.Contains(lower, "validation_required") ||
		strings.Contains(lower, "verify your account") ||
		strings.Contains(lower, "validation_url"):
		return forbiddenTypeValidation
	case strings.Contains(lower, "terms of service") ||
		strings.Contains(lower, "violation"):
		return forbiddenTypeViolation
	default:
		return forbiddenTypeForbidden
	}
}

// urlPattern 用于从 403 响应体中提取 URL（降级方案）
var urlPattern = regexp.MustCompile(`https://[^\s"'\\]+`)

// extractValidationURL 从 403 响应 JSON 中提取验证/申诉链接
func extractValidationURL(body string) string {
	// 1. 尝试结构化 JSON 提取: /error/details[*]/metadata/validation_url 或 appeal_url
	var parsed struct {
		Error struct {
			Details []struct {
				Metadata map[string]string `json:"metadata"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(body), &parsed) == nil {
		for _, detail := range parsed.Error.Details {
			if u := detail.Metadata["validation_url"]; u != "" {
				return u
			}
			if u := detail.Metadata["appeal_url"]; u != "" {
				return u
			}
		}
	}

	// 2. 降级：正则匹配 URL
	lower := strings.ToLower(body)
	if !strings.Contains(lower, "validation") &&
		!strings.Contains(lower, "verify") &&
		!strings.Contains(lower, "appeal") {
		return ""
	}
	// 先解码常见转义再匹配
	normalized := strings.ReplaceAll(body, `\u0026`, "&")
	if m := urlPattern.FindString(normalized); m != "" {
		return m
	}
	return ""
}

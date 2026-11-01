package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
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

// FetchQuota 获取 Antigravity 账户额度信息
func (f *AntigravityQuotaFetcher) FetchQuota(ctx context.Context, account *Account, proxyURL string) (*QuotaResult, error) {
	accessToken := account.GetCredential("access_token")
	projectID := account.GetCredential("project_id")

	client, err := antigravity.NewClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create antigravity client failed: %w", err)
	}

	bodyLimit := resolveModelsListReadLimit(f.cfg)

	// 调用 API 获取配额 (旧端点)
	modelsResp, modelsRaw, modelsErr := client.FetchAvailableModels(ctx, accessToken, projectID, bodyLimit)
	if modelsErr != nil {
		// 403 Forbidden: 明确禁止访问，立即返回 is_forbidden 标记
		var forbiddenErr *antigravity.ForbiddenError
		if errors.As(modelsErr, &forbiddenErr) {
			return buildAntigravityForbiddenResult(forbiddenErr.Body), nil
		}
	}

	// 调用 LoadCodeAssist 获取订阅等级和 AI Credits 余额（非关键路径，失败不影响主流程）
	tierRaw, tierNormalized, loadResp := f.fetchSubscriptionTier(ctx, client, accessToken)

	// 调用 RetrieveUserQuotaSummary 获取真实额度桶摘要（对应 agy /usage，非关键路径，平滑降级）
	quotaSummaryResp, summaryRaw, summaryErr := client.RetrieveUserQuotaSummary(ctx, accessToken, projectID, bodyLimit)
	if summaryErr != nil {
		slog.Warn("failed to fetch antigravity user quota summary, falling back to available models",
			"account_id", account.ID,
			"error", logredact.RedactText(summaryErr.Error()),
		)
	}

	if summaryErr != nil {
		var forbiddenErr *antigravity.ForbiddenError
		if errors.As(summaryErr, &forbiddenErr) {
			return buildAntigravityForbiddenResult(forbiddenErr.Body), nil
		}
	}
	// 降级与容错策略：
	// 1. 若两者均失败：返回错误（优先返回 modelsErr，附带脱敏后的 summaryErr）
	if modelsErr != nil && !hasUsableQuotaSummary(quotaSummaryResp) {
		if summaryErr != nil {
			return nil, newSafeAntigravityFetchError(modelsErr, summaryErr)
		}
		return nil, newSafeAntigravityFetchError(modelsErr)
	}

	// 2. 若 modelsErr != nil 但 quotaSummaryResp != nil：记录警告并使用 quotaSummary 进行软降级
	if modelsErr != nil {
		slog.Warn("failed to fetch available models, recovering from quota summary",
			"account_id", account.ID,
			"error", logredact.RedactText(modelsErr.Error()),
		)
	}

	// 原始数据 raw：优先使用 modelsRaw，若不可用则回退至 summaryRaw
	raw := modelsRaw
	if len(raw) == 0 {
		raw = summaryRaw
	}

	// 转换为 UsageInfo
	if quotaSummaryResp != nil && !hasUsableQuotaSummary(quotaSummaryResp) {
		quotaSummaryResp = nil
	}
	usageInfo := f.buildUsageInfo(modelsResp, tierRaw, tierNormalized, loadResp, quotaSummaryResp)

	return &QuotaResult{
		UsageInfo: usageInfo,
		Raw:       raw,
	}, nil
}

// fetchSubscriptionTier 获取账号订阅等级，失败返回空字符串。
// 同时返回 LoadCodeAssistResponse，以便提取 AI Credits 余额。
func (f *AntigravityQuotaFetcher) fetchSubscriptionTier(ctx context.Context, client *antigravity.Client, accessToken string) (raw, normalized string, loadResp *antigravity.LoadCodeAssistResponse) {
	loadResp, _, err := client.LoadCodeAssist(ctx, accessToken)
	if err != nil {
		slog.Warn("failed to fetch subscription tier", "error", logredact.RedactText(err.Error()))
		return "", "", nil
	}
	if loadResp == nil {
		return "", "", nil
	}

	raw = loadResp.GetTier() // 已有方法：paidTier > currentTier
	normalized = normalizeTier(raw)
	return raw, normalized, loadResp
}

// quota summary 必须包含至少一个 bucket 才能作为可用的降级数据。
// normalizeTier 将原始 tier 字符串归一化为 FREE/PRO/ULTRA/UNKNOWN
func hasUsableQuotaSummary(summary *antigravity.RetrieveUserQuotaSummaryResponse) bool {
	if summary == nil {
		return false
	}
	if len(summary.Buckets) > 0 {
		return true
	}
	for _, group := range summary.Groups {
		if len(group.Buckets) > 0 {
			return true
		}
	}
	return false
}

func newSafeAntigravityFetchError(primary error, secondary ...error) error {
	parts := []string{"antigravity quota fetch failed"}
	for _, err := range append([]error{primary}, secondary...) {
		if err == nil {
			continue
		}
		message := logredact.RedactText(err.Error())
		if match := antigravityHTTPStatusPattern.FindStringSubmatch(message); len(match) == 2 {
			parts = append(parts, "HTTP "+match[1])
		} else {
			parts = append(parts, message)
		}
	}
	return errors.New(strings.Join(parts, "; "))
}
func normalizeTier(raw string) string {
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "ultra"):
		return "ULTRA"
	case strings.Contains(lower, "pro"):
		return "PRO"
	case strings.Contains(lower, "free"):
		return "FREE"
	default:
		return "UNKNOWN"
	}
}

// buildUsageInfo 将 API 响应转换为 UsageInfo。
func (f *AntigravityQuotaFetcher) buildUsageInfo(
	modelsResp *antigravity.FetchAvailableModelsResponse,
	tierRaw, tierNormalized string,
	loadResp *antigravity.LoadCodeAssistResponse,
	optQuotaSummary ...*antigravity.RetrieveUserQuotaSummaryResponse,
) *UsageInfo {
	var quotaSummaryResp *antigravity.RetrieveUserQuotaSummaryResponse
	if len(optQuotaSummary) > 0 {
		quotaSummaryResp = optQuotaSummary[0]
	}

	now := time.Now()
	info := &UsageInfo{
		UpdatedAt:               &now,
		AntigravityQuota:        make(map[string]*AntigravityModelQuota),
		AntigravityQuotaDetails: make(map[string]*AntigravityModelDetail),
		AntigravityQuotaSummary: quotaSummaryResp,
		SubscriptionTier:        tierNormalized,
		SubscriptionTierRaw:     tierRaw,
	}

	// 1. 遍历 modelsResp，填充 AntigravityQuota 和 AntigravityQuotaDetails
	if modelsResp != nil {
		for modelName, modelInfo := range modelsResp.Models {
			if modelInfo.QuotaInfo == nil {
				continue
			}

			// remainingFraction 是剩余比例 (0.0-1.0)，转换为使用率百分比
			utilization := quotaUtilization(modelInfo.QuotaInfo.RemainingFraction)

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
	}

	// 2. 如果存在 QuotaSummary，将以模型 ID 命名的桶补充到 AntigravityQuota。
	// AntigravityQuota 的键约定为模型名（渠道监控会逐项输出为模型档位），因此：
	//   - 仅接受形如模型 ID 的 BucketID，DisplayName 或 "weekly" 等汇总桶只保留在 AntigravityQuotaSummary 中；
	//   - 不覆盖 fetchAvailableModels 已返回的模型数据，也不让后出现的同名桶覆盖先出现的桶。
	if quotaSummaryResp != nil {
		applyBucket := func(b antigravity.QuotaSummaryBucket) {
			if b.Disabled || b.RemainingFraction < 0 || !isAntigravityModelBucketID(b.BucketID) {
				return
			}
			if _, exists := info.AntigravityQuota[b.BucketID]; exists {
				return
			}
			utilization := quotaUtilization(b.RemainingFraction)

			info.AntigravityQuota[b.BucketID] = &AntigravityModelQuota{
				Utilization: utilization,
				ResetTime:   b.ResetTime,
			}
		}
		for _, b := range quotaSummaryResp.Buckets {
			applyBucket(b)
		}
		for _, g := range quotaSummaryResp.Groups {
			for _, b := range g.Buckets {
				applyBucket(b)
			}
		}
	}

	// 废弃模型转发规则
	if modelsResp != nil && len(modelsResp.DeprecatedModelIDs) > 0 {
		info.ModelForwardingRules = make(map[string]string, len(modelsResp.DeprecatedModelIDs))
		for oldID, deprecated := range modelsResp.DeprecatedModelIDs {
			info.ModelForwardingRules[oldID] = deprecated.NewModelID
		}
	}

	// 同时设置 FiveHour 用于兼容展示（取主要模型，优先考虑 Gemini 3.8 / 3 Flash）
	priorityModels := []string{
		"gemini-3.8-flash", "gemini-3.8-flash-high",
		"gemini-3-flash", "claude-sonnet-4-6",
		"claude-sonnet-4-20250514", "claude-sonnet-4", "gemini-2.5-pro",
	}
	for _, modelName := range priorityModels {
		if modelsResp != nil {
			if modelInfo, ok := modelsResp.Models[modelName]; ok && modelInfo.QuotaInfo != nil {
				utilization := (1.0 - modelInfo.QuotaInfo.RemainingFraction) * 100
				progress := &UsageProgress{
					Utilization: utilization,
				}
				if modelInfo.QuotaInfo.ResetTime != "" {
					if resetTime, err := time.Parse(time.RFC3339, modelInfo.QuotaInfo.ResetTime); err == nil {
						progress.ResetsAt = &resetTime
						progress.RemainingSeconds = int(time.Until(resetTime).Seconds())
						if progress.RemainingSeconds < 0 {
							progress.RemainingSeconds = 0
						}
					}
				}
				info.FiveHour = progress
				break
			}
		}
		if q, ok := info.AntigravityQuota[modelName]; ok && q != nil {
			progress := &UsageProgress{
				Utilization: float64(q.Utilization),
			}
			if q.ResetTime != "" {
				if resetTime, err := time.Parse(time.RFC3339, q.ResetTime); err == nil {
					progress.ResetsAt = &resetTime
					progress.RemainingSeconds = int(time.Until(resetTime).Seconds())
					if progress.RemainingSeconds < 0 {
						progress.RemainingSeconds = 0
					}
				}
			}
			info.FiveHour = progress
			break
		}
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

	return info
}

func quotaUtilization(remainingFraction float64) int {
	utilization := int(math.Round((1.0 - remainingFraction) * 100))
	if utilization < 0 {
		return 0
	}
	if utilization > 100 {
		return 100
	}
	return utilization
}

// antigravityModelBucketPrefixes 为 Antigravity 模型 ID 的已知前缀，用于区分模型桶与汇总桶
var antigravityModelBucketPrefixes = []string{"gemini-", "claude-", "gpt-"}

// isAntigravityModelBucketID 判断 quota summary 的 BucketID 是否为模型 ID
func isAntigravityModelBucketID(bucketID string) bool {
	if bucketID == "" || strings.ContainsAny(bucketID, " \t") {
		return false
	}
	lower := strings.ToLower(bucketID)
	for _, prefix := range antigravityModelBucketPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
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

var antigravityHTTPStatusPattern = regexp.MustCompile(`(?i)HTTP (\d{3})`)

func buildAntigravityForbiddenResult(body string) *QuotaResult {
	now := time.Now()
	fbType := classifyForbiddenType(body)
	return &QuotaResult{
		UsageInfo: &UsageInfo{
			UpdatedAt:       &now,
			IsForbidden:     true,
			ForbiddenReason: body,
			ForbiddenType:   fbType,
			ValidationURL:   extractValidationURL(body),
			NeedsVerify:     fbType == forbiddenTypeValidation,
			IsBanned:        fbType == forbiddenTypeViolation,
			ErrorCode:       errorCodeForbidden,
		},
	}
}

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

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

// FetchQuota 获取 Antigravity 账户额度信息
func (f *AntigravityQuotaFetcher) FetchQuota(ctx context.Context, account *Account, proxyURL string) (*QuotaResult, error) {
	accessToken := account.GetCredential("access_token")
	projectID := account.GetCredential("project_id")

	client, err := antigravity.NewClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create antigravity client failed: %w", err)
	}

	// 调用 API 获取配额
	bodyLimit := resolveModelsListReadLimit(f.cfg)
	modelsResp, modelsRaw, err := client.FetchAvailableModels(ctx, accessToken, projectID, bodyLimit)
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

	// 调用 retrieveUserQuotaSummary 获取 5h / Weekly 共享配额桶（非关键路径，失败不影响主流程）
	quotaSummaryResp, _, summaryErr := client.RetrieveUserQuotaSummary(ctx, accessToken, projectID, bodyLimit)
	if summaryErr != nil {
		slog.Warn("failed to fetch antigravity user quota summary", "error", summaryErr)
	}

	// 调用 LoadCodeAssist 获取订阅等级和 AI Credits 余额（非关键路径，失败不影响主流程）
	tierRaw, tierNormalized, loadResp := f.fetchSubscriptionTier(ctx, client, accessToken)

	// 转换为 UsageInfo
	usageInfo := f.buildUsageInfo(modelsResp, tierRaw, tierNormalized, loadResp)
	f.applyQuotaSummary(usageInfo, quotaSummaryResp)

	return &QuotaResult{
		UsageInfo: usageInfo,
		Raw:       modelsRaw,
	}, nil
}

// fetchSubscriptionTier 获取账号订阅等级，失败返回空字符串。
// 同时返回 LoadCodeAssistResponse，以便提取 AI Credits 余额。
func (f *AntigravityQuotaFetcher) fetchSubscriptionTier(ctx context.Context, client *antigravity.Client, accessToken string) (raw, normalized string, loadResp *antigravity.LoadCodeAssistResponse) {
	loadResp, _, err := client.LoadCodeAssist(ctx, accessToken)
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

// normalizeTier 将原始 tier 字符串归一化为 FREE/PRO/ULTRA/UNKNOWN
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
func (f *AntigravityQuotaFetcher) buildUsageInfo(modelsResp *antigravity.FetchAvailableModelsResponse, tierRaw, tierNormalized string, loadResp *antigravity.LoadCodeAssistResponse) *UsageInfo {
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

		// remainingFraction 是剩余比例 (0.0-1.0)，转换为使用率百分比
		utilization := int((1.0 - modelInfo.QuotaInfo.RemainingFraction) * 100)

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

	// 同时设置 FiveHour 用于兼容展示（取主要模型）
	priorityModels := []string{
		"claude-sonnet-5-5-high",
		"claude-opus-5-5-high",
		"claude-sonnet-4-6",
		"claude-opus-4-6",
		"claude-sonnet-4-20250514",
		"claude-sonnet-4",
		"gemini-4-argon-high",
		"gemini-3.1-pro-high",
		"gemini-2.5-pro",
	}
	for _, modelName := range priorityModels {
		if modelInfo, ok := modelsResp.Models[modelName]; ok && modelInfo.QuotaInfo != nil {
			utilization := (1.0 - modelInfo.QuotaInfo.RemainingFraction) * 100
			progress := &UsageProgress{
				Utilization: utilization,
			}
			if modelInfo.QuotaInfo.ResetTime != "" {
				if resetTime, err := time.Parse(time.RFC3339, modelInfo.QuotaInfo.ResetTime); err == nil {
					progress.ResetsAt = &resetTime
					progress.RemainingSeconds = int(time.Until(resetTime).Seconds())
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

// applyQuotaSummary 将 retrieveUserQuotaSummary 的共享配额桶（gemini-5h、gemini-weekly、
// 3p-5h、3p-weekly）合并到 UsageInfo。当某个窗口因周限额已耗尽而被标记为 disabled: true
// 时跳过该桶，避免把上游填充的 remainingFraction: 1 误展示为 0% 已用。
func (f *AntigravityQuotaFetcher) applyQuotaSummary(info *UsageInfo, summary *antigravity.RetrieveUserQuotaSummaryResponse) {
	if info == nil || summary == nil {
		return
	}
	if info.AntigravityQuota == nil {
		info.AntigravityQuota = make(map[string]*AntigravityModelQuota)
	}

	allBuckets := make([]antigravity.QuotaSummaryBucket, 0, len(summary.Buckets)+4)
	allBuckets = append(allBuckets, summary.Buckets...)
	for _, g := range summary.Groups {
		allBuckets = append(allBuckets, g.Buckets...)
	}

	bucketByID := make(map[string]antigravity.QuotaSummaryBucket, len(allBuckets))
	for _, b := range allBuckets {
		id := strings.TrimSpace(b.BucketID)
		if id == "" || b.Disabled {
			continue
		}
		remaining := b.RemainingFraction
		if remaining < 0 {
			remaining = 0
		} else if remaining > 1 {
			remaining = 1
		}
		utilization := int((1.0 - remaining) * 100)
		info.AntigravityQuota[id] = &AntigravityModelQuota{
			Utilization: utilization,
			ResetTime:   b.ResetTime,
		}
		bucketByID[id] = b
	}

	// 优先用活跃的 5h / weekly 共享桶更新 FiveHour 和 SevenDay。
	for _, id := range []string{"3p-5h", "gemini-5h"} {
		if b, ok := bucketByID[id]; ok {
			info.FiveHour = quotaSummaryBucketToProgress(b)
			break
		}
	}
	for _, id := range []string{"3p-weekly", "gemini-weekly"} {
		if b, ok := bucketByID[id]; ok {
			info.SevenDay = quotaSummaryBucketToProgress(b)
			break
		}
	}
}

func quotaSummaryBucketToProgress(b antigravity.QuotaSummaryBucket) *UsageProgress {
	remaining := b.RemainingFraction
	if remaining < 0 {
		remaining = 0
	} else if remaining > 1 {
		remaining = 1
	}
	progress := &UsageProgress{
		Utilization: (1.0 - remaining) * 100,
	}
	if b.ResetTime != "" {
		if resetTime, err := time.Parse(time.RFC3339, b.ResetTime); err == nil {
			progress.ResetsAt = &resetTime
			progress.RemainingSeconds = int(time.Until(resetTime).Seconds())
		}
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

package service

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

const (
	modelRateLimitsKey                 = "model_rate_limits"
	antigravityGeminiModelRateLimitKey = "antigravity:gemini"
	openAIImageGenerationRateLimitKey  = "openai:image_generation"
	openAICodexSparkRateLimitReason    = "openai_codex_spark_rate_limit"
	// anthropicFableRateLimitKey 是 Anthropic 7d_oi（Fable 专属 7d 窗口）限流的
	// 家族级 scope：命中后所有 Fable 变体（含 [1m] 等后缀）都不再调度到该账号。
	anthropicFableRateLimitKey = "claude-fable-5"
)

// isRateLimitActiveForKey 检查指定 key 的限流是否生效
func (a *Account) isRateLimitActiveForKey(key string) bool {
	resetAt := a.modelRateLimitResetAt(key)
	return resetAt != nil && time.Now().Before(*resetAt)
}

// getRateLimitRemainingForKey 获取指定 key 的限流剩余时间，0 表示未限流或已过期
func (a *Account) getRateLimitRemainingForKey(key string) time.Duration {
	resetAt := a.modelRateLimitResetAt(key)
	if resetAt == nil {
		return 0
	}
	remaining := time.Until(*resetAt)
	if remaining > 0 {
		return remaining
	}
	return 0
}

func (a *Account) isModelRateLimitedWithContext(ctx context.Context, requestedModel string) bool {
	for _, key := range a.modelRateLimitKeysForRequest(ctx, requestedModel) {
		if a.isRateLimitActiveForKey(key) {
			return true
		}
	}
	return false
}

// isModelRateLimitedForFinalKeyWithContext 直接按「实际会发往上游的最终模型名」查
// model_rate_limits，不再做任何账号级映射。
//
// 为什么要有这条不映射的路径：调用方（WS 每轮账号资格复核）拿到的已经是最终上游
// 模型名，再走一次 modelRateLimitKeysForRequest 会被 GetMappedModel 二次映射——
// 账号配了 `*` 通配或链式映射时，最终模型名会被映回别的名字，记在最终模型名上的
// 限流直接漏检，反过来也可能误拦正常模型。
//
// HTTP 调度路径拿到的是「客户端请求模型」，必须保留那一次映射，所以
// modelRateLimitKeysForRequest 的行为不动，这里只补一条平行入口。
func (a *Account) isModelRateLimitedForFinalKeyWithContext(ctx context.Context, finalKey string) bool {
	for _, key := range a.modelRateLimitKeysForFinalKey(ctx, finalKey) {
		if a.isRateLimitActiveForKey(key) {
			return true
		}
	}
	return false
}

// modelRateLimitKeysForFinalKey 返回「最终模型键本身 + 平台家族级 key」。
// 家族 key 的判定规则与 modelRateLimitKeysForRequest 共用 modelRateLimitFamilyKeys，
// 唯一的差别是这里不做 GetMappedModel / Antigravity thinking 后缀等任何映射。
func (a *Account) modelRateLimitKeysForFinalKey(ctx context.Context, finalKey string) []string {
	if a == nil {
		return nil
	}
	finalKey = strings.TrimSpace(finalKey)
	if finalKey == "" {
		return nil
	}
	// requestedModel 传最终键本身：家族判定只看这一个键，不引入映射前的模型名。
	return a.modelRateLimitFamilyKeys(ctx, finalKey, finalKey)
}

// GetModelRateLimitRemainingTime 获取模型限流剩余时间
// 返回 0 表示未限流或已过期
func (a *Account) GetModelRateLimitRemainingTime(requestedModel string) time.Duration {
	return a.GetModelRateLimitRemainingTimeWithContext(context.Background(), requestedModel)
}

func (a *Account) GetModelRateLimitRemainingTimeWithContext(ctx context.Context, requestedModel string) time.Duration {
	remaining := time.Duration(0)
	for _, key := range a.modelRateLimitKeysForRequest(ctx, requestedModel) {
		if keyRemaining := a.getRateLimitRemainingForKey(key); keyRemaining > remaining {
			remaining = keyRemaining
		}
	}
	return remaining
}

func (a *Account) modelRateLimitKeysForRequest(ctx context.Context, requestedModel string) []string {
	if a == nil {
		return nil
	}

	modelKey := a.GetMappedModel(requestedModel)
	if a.Platform == PlatformAntigravity {
		modelKey = resolveFinalAntigravityModelKey(ctx, a, requestedModel)
	}
	modelKey = strings.TrimSpace(modelKey)
	if modelKey == "" {
		return nil
	}

	return a.modelRateLimitFamilyKeys(ctx, requestedModel, modelKey)
}

// modelRateLimitFamilyKeys 在已经定好的 modelKey 之上补平台家族级 scope。
// requestedModel 只参与 OpenAI 生图家族的入站意图判定，不影响 modelKey 本身。
func (a *Account) modelRateLimitFamilyKeys(ctx context.Context, requestedModel, modelKey string) []string {
	keys := []string{modelKey}
	switch a.Platform {
	case PlatformAntigravity:
		if isAntigravityGeminiModel(modelKey) && modelKey != antigravityGeminiModelRateLimitKey {
			keys = append(keys, antigravityGeminiModelRateLimitKey)
		}
	case PlatformOpenAI:
		if openAIImageGenerationRateLimitApplies(ctx, requestedModel, modelKey) && modelKey != openAIImageGenerationRateLimitKey {
			keys = append(keys, openAIImageGenerationRateLimitKey)
		}
	case PlatformAnthropic:
		if isAnthropicFableModel(modelKey) && modelKey != anthropicFableRateLimitKey {
			keys = append(keys, anthropicFableRateLimitKey)
		}
	}
	return keys
}

// isAnthropicFableModel 判断是否为 Fable 模型家族（claude-fable-5、claude-fable-5[1m] 等变体）
func isAnthropicFableModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "fable")
}

func openAIImageGenerationRateLimitApplies(ctx context.Context, requestedModel, modelKey string) bool {
	if isOpenAIImageGenerationModel(requestedModel) || isOpenAIImageGenerationModel(modelKey) {
		return true
	}
	return OpenAIImageGenerationIntentFromContext(ctx)
}

func WithOpenAIImageGenerationIntent(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxkey.OpenAIImageGenerationIntent, true)
}

func OpenAIImageGenerationIntentFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, ok := ctx.Value(ctxkey.OpenAIImageGenerationIntent).(bool)
	return ok && enabled
}

// WithOpenAIImagesEndpoint 标记请求从 /v1/images/* 专用生图端点入站。
func WithOpenAIImagesEndpoint(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxkey.OpenAIImagesEndpoint, true)
}

// OpenAIImagesEndpointFromContext 报告请求是否来自 /v1/images/*。
func OpenAIImagesEndpointFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, ok := ctx.Value(ctxkey.OpenAIImagesEndpoint).(bool)
	return ok && enabled
}

func resolveFinalAntigravityModelKey(ctx context.Context, account *Account, requestedModel string) string {
	modelKey := mapAntigravityModel(account, requestedModel)
	if modelKey == "" {
		return ""
	}
	// thinking 会影响 Antigravity 最终模型名（例如 claude-sonnet-4-5 -> claude-sonnet-4-5-thinking）
	if enabled, ok := ThinkingEnabledFromContext(ctx); ok {
		modelKey = applyThinkingModelSuffix(modelKey, enabled)
	}
	return modelKey
}

func isAntigravityGeminiModel(model string) bool {
	return strings.HasPrefix(normalizeAntigravityModelName(model), "gemini-")
}

func antigravityModelRateLimitKeys(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	keys := []string{model}
	if isAntigravityGeminiModel(model) && model != antigravityGeminiModelRateLimitKey {
		keys = append(keys, antigravityGeminiModelRateLimitKey)
	}
	return keys
}

func (a *Account) modelRateLimitResetAt(scope string) *time.Time {
	if a == nil || a.Extra == nil || scope == "" {
		return nil
	}
	rawLimits, ok := a.Extra[modelRateLimitsKey].(map[string]any)
	if !ok {
		return nil
	}
	rawLimit, ok := rawLimits[scope].(map[string]any)
	if !ok {
		return nil
	}
	resetAtRaw, ok := rawLimit["rate_limit_reset_at"].(string)
	if !ok || strings.TrimSpace(resetAtRaw) == "" {
		return nil
	}
	resetAt, err := time.Parse(time.RFC3339, resetAtRaw)
	if err != nil {
		return nil
	}
	return &resetAt
}

func setAccountModelRateLimitSnapshot(account *Account, scope string, resetAt time.Time, reason string, now time.Time) {
	if account == nil || strings.TrimSpace(scope) == "" {
		return
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	limits, ok := account.Extra[modelRateLimitsKey].(map[string]any)
	if !ok {
		limits = make(map[string]any)
		account.Extra[modelRateLimitsKey] = limits
	}
	payload := map[string]any{
		"rate_limited_at":     now.UTC().Format(time.RFC3339),
		"rate_limit_reset_at": resetAt.UTC().Format(time.RFC3339),
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		payload["reason"] = reason
	}
	limits[scope] = payload
}

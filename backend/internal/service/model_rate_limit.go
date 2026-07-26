package service

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

const (
	modelRateLimitsKey                 = domain.ModelRateLimitsKey
	antigravityGeminiModelRateLimitKey = "antigravity:gemini"
	openAIImageGenerationRateLimitKey  = "openai:image_generation"
	// anthropicFableRateLimitKey 是 Anthropic 7d_oi（Fable 专属 7d 窗口）限流的
	// 家族级 scope：命中后所有 Fable 变体（含 [1m] 等后缀）都不再调度到该账号。
	anthropicFableRateLimitKey = "claude-fable-5"
)

// isRateLimitActiveForKey / getRateLimitRemainingForKey / modelRateLimitResetAt
// moved to domain as Account methods (IsRateLimitActiveForKey etc.). Thin
// lowercase wrappers keep the many service-internal call sites compiling.
func isRateLimitActiveForKey(a *Account, key string) bool {
	return a.IsRateLimitActiveForKey(key)
}

func getRateLimitRemainingForKey(a *Account, key string) time.Duration {
	return a.GetRateLimitRemainingForKey(key)
}

func modelRateLimitResetAt(a *Account, scope string) *time.Time {
	return a.ModelRateLimitResetAt(scope)
}

// AccountIsModelRateLimitedWithContext is the free-function form of the former
// (a *Account) isModelRateLimitedWithContext method. Stays in service because
// it cascades into request-metadata / ThinkingEnabledFromContext.
func AccountIsModelRateLimitedWithContext(a *Account, ctx context.Context, requestedModel string) bool {
	for _, key := range accountModelRateLimitKeysForRequest(a, ctx, requestedModel) {
		if isRateLimitActiveForKey(a, key) {
			return true
		}
	}
	return false
}

// AccountGetModelRateLimitRemainingTime is the free-function form of the former
// (a *Account) GetModelRateLimitRemainingTime method.
func AccountGetModelRateLimitRemainingTime(a *Account, requestedModel string) time.Duration {
	return AccountGetModelRateLimitRemainingTimeWithContext(a, context.Background(), requestedModel)
}

// AccountGetModelRateLimitRemainingTimeWithContext is the free-function form of
// the former (a *Account) GetModelRateLimitRemainingTimeWithContext method.
func AccountGetModelRateLimitRemainingTimeWithContext(a *Account, ctx context.Context, requestedModel string) time.Duration {
	remaining := time.Duration(0)
	for _, key := range accountModelRateLimitKeysForRequest(a, ctx, requestedModel) {
		if keyRemaining := getRateLimitRemainingForKey(a, key); keyRemaining > remaining {
			remaining = keyRemaining
		}
	}
	return remaining
}

func accountModelRateLimitKeysForRequest(a *Account, ctx context.Context, requestedModel string) []string {
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

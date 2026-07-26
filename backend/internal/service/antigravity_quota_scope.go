package service

import (
	"context"
	"strings"
	"time"
)

func normalizeAntigravityModelName(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if idx := strings.LastIndex(normalized, "/publishers/google/models/"); idx != -1 {
		normalized = normalized[idx+len("/publishers/google/models/"):]
	} else if idx := strings.LastIndex(normalized, "/publishers/anthropic/models/"); idx != -1 {
		normalized = normalized[idx+len("/publishers/anthropic/models/"):]
	} else if idx := strings.LastIndex(normalized, "/models/"); idx != -1 {
		normalized = normalized[idx+len("/models/"):]
	} else {
		normalized = strings.TrimPrefix(normalized, "publishers/google/models/")
		normalized = strings.TrimPrefix(normalized, "publishers/anthropic/models/")
		normalized = strings.TrimPrefix(normalized, "models/")
	}
	return normalized
}

// resolveAntigravityModelKey 根据请求的模型名解析限流 key
// 返回空字符串表示无法解析
func resolveAntigravityModelKey(requestedModel string) string {
	return normalizeAntigravityModelName(requestedModel)
}

// AccountIsSchedulableForModel is the free-function form of the former
// (a *Account) IsSchedulableForModel method. Stays in service (ctx cascade).
func AccountIsSchedulableForModel(a *Account, requestedModel string) bool {
	return AccountIsSchedulableForModelWithContext(a, context.Background(), requestedModel)
}

// AccountIsSchedulableForModelWithContext is the free-function form of the
// former (a *Account) IsSchedulableForModelWithContext method.
func AccountIsSchedulableForModelWithContext(a *Account, ctx context.Context, requestedModel string) bool {
	if a == nil {
		return false
	}
	if !a.IsSchedulable() {
		return false
	}
	if AccountIsModelRateLimitedWithContext(a, ctx, requestedModel) {
		// Antigravity + overages 启用 + 积分未耗尽 → 放行（有积分可用）
		if a.Platform == PlatformAntigravity && a.IsOveragesEnabled() && !a.IsCreditsExhausted() {
			return true
		}
		return false
	}
	return true
}

// AccountGetRateLimitRemainingTime is the free-function form of the former
// (a *Account) GetRateLimitRemainingTime method.
func AccountGetRateLimitRemainingTime(a *Account, requestedModel string) time.Duration {
	return AccountGetRateLimitRemainingTimeWithContext(a, context.Background(), requestedModel)
}

// AccountGetRateLimitRemainingTimeWithContext is the free-function form of the
// former (a *Account) GetRateLimitRemainingTimeWithContext method.
func AccountGetRateLimitRemainingTimeWithContext(a *Account, ctx context.Context, requestedModel string) time.Duration {
	if a == nil {
		return 0
	}
	return AccountGetModelRateLimitRemainingTimeWithContext(a, ctx, requestedModel)
}

package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func supportsGroupModelRouting(platform string) bool {
	switch platform {
	case PlatformAnthropic, PlatformOpenAI, PlatformGemini:
		return true
	default:
		return false
	}
}

func estimatedInputTokensFromContext(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	switch v := ctx.Value(ctxkey.EstimatedInputTokens).(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func contextLengthFilterAllowsAccount(ctx context.Context, account *Account, sessionHash string) bool {
	if account == nil {
		return false
	}
	estimatedInputTokens := estimatedInputTokensFromContext(ctx)
	if estimatedInputTokens <= 0 {
		return true
	}
	return account.ContextLengthFilterAllows(ctx, estimatedInputTokens, sessionHash)
}

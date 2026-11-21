package service

import (
	"context"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	OpenAICompactStrategyKey     = "openai_compact_strategy"
	OpenAICompactStrategyInherit = "inherit"
	OpenAICompactStrategySummary = "summary"
)

type summaryCompactionRequestKey struct{}

func WithOpenAISummaryCompactionRequest(ctx context.Context) context.Context {
	return context.WithValue(ctx, summaryCompactionRequestKey{}, true)
}

func isSummaryCompactionRequest(ctx context.Context, account *Account) bool {
	if ctx == nil {
		return false
	}
	requested, _ := ctx.Value(summaryCompactionRequestKey{}).(bool)
	return requested && account.UsesOpenAISummaryCompaction()
}

// UsesOpenAISummaryCompaction is opt-in per API-key account. Unknown/missing
// values deliberately retain the existing forwarding behavior.
func (a *Account) UsesOpenAISummaryCompaction() bool {
	return a != nil && a.Platform == PlatformOpenAI && a.Type == AccountTypeAPIKey &&
		a.GetExtraString(OpenAICompactStrategyKey) == OpenAICompactStrategySummary
}

func ValidateOpenAICompactStrategy(platform, accountType string, extra map[string]any) error {
	value, exists := extra[OpenAICompactStrategyKey]
	if !exists || value == nil {
		return nil
	}
	strategy, ok := value.(string)
	if !ok || (strategy != OpenAICompactStrategyInherit && strategy != OpenAICompactStrategySummary) {
		return infraerrors.BadRequest("INVALID_COMPACT_STRATEGY", "compact strategy must be inherit or summary")
	}
	if strategy == OpenAICompactStrategySummary && (platform != PlatformOpenAI || accountType != AccountTypeAPIKey) {
		return infraerrors.BadRequest("INVALID_COMPACT_STRATEGY", "summary compaction requires an OpenAI API-key account")
	}
	return nil
}

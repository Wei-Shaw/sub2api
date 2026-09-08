package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"
)

// openAIRateLimitGeneration identifies the account-level cooldown observed
// immediately before an upstream quota probe. Recovery is conditional on this
// exact generation so a newer 429 that arrives while the probe is in flight
// cannot be erased by a stale successful response.
type openAIRateLimitGeneration struct {
	accountID     int64
	rateLimitedAt time.Time
	resetAt       time.Time
}

type openAIRateLimitRecoveryRepository interface {
	ClearOpenAIRateLimitIfObserved(ctx context.Context, id int64, observedLimitedAt, observedResetAt time.Time) (bool, error)
}

// observeOpenAIRateLimitGeneration only observes ordinary OpenAI OAuth rows.
// Spark shadows have an independent quota dimension and their /wham/usage
// response must not clear the parent's global account cooldown.
func observeOpenAIRateLimitGeneration(account *Account) *openAIRateLimitGeneration {
	if account == nil || !account.IsOpenAIOAuth() || account.IsShadow() {
		return nil
	}
	if account.RateLimitedAt == nil || account.RateLimitResetAt == nil {
		return nil
	}
	return &openAIRateLimitGeneration{
		accountID:     account.ID,
		rateLimitedAt: *account.RateLimitedAt,
		resetAt:       *account.RateLimitResetAt,
	}
}

func observeOpenAIRateLimitGenerationByID(ctx context.Context, repo AccountRepository, accountID int64) *openAIRateLimitGeneration {
	if repo == nil || accountID <= 0 {
		return nil
	}
	account, err := repo.GetByID(ctx, accountID)
	if err != nil {
		return nil
	}
	return observeOpenAIRateLimitGeneration(account)
}

func clearObservedOpenAIRateLimitIfRecovered(
	ctx context.Context,
	repo AccountRepository,
	observed *openAIRateLimitGeneration,
	recovered bool,
) (bool, error) {
	if !recovered || observed == nil || repo == nil {
		return false, nil
	}
	recoveryRepo, ok := repo.(openAIRateLimitRecoveryRepository)
	if !ok {
		return false, nil
	}
	return recoveryRepo.ClearOpenAIRateLimitIfObserved(ctx, observed.accountID, observed.rateLimitedAt, observed.resetAt)
}

func logOpenAIRateLimitRecoveryResult(
	ctx context.Context,
	repo AccountRepository,
	observed *openAIRateLimitGeneration,
	recovered bool,
	source string,
) {
	cleared, err := clearObservedOpenAIRateLimitIfRecovered(ctx, repo, observed, recovered)
	if err != nil {
		slog.Warn("openai_quota_rate_limit_recovery_clear_failed", "account_id", observedAccountID(observed), "source", source, "error", err)
		return
	}
	if cleared {
		slog.Info("openai_quota_rate_limit_recovered", "account_id", observed.accountID, "source", source, "reset_at", observed.resetAt.UTC())
	}
}

func observedAccountID(observed *openAIRateLimitGeneration) int64 {
	if observed == nil {
		return 0
	}
	return observed.accountID
}

// openAIQuotaUsageRecovered requires an affirmative, internally consistent
// quota response. The allowed/limit_reached flags alone are insufficient:
// callers also need every returned window to be below exhaustion, or to have
// already crossed its reset boundary.
func openAIQuotaUsageRecovered(usage *OpenAIQuotaUsage, now time.Time) bool {
	if usage == nil || usage.RateLimit == nil || !usage.RateLimit.Allowed || usage.RateLimit.LimitReached {
		return false
	}

	seenWindow := false
	for _, window := range []*OpenAIRateLimitWindow{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow} {
		if window == nil {
			continue
		}
		seenWindow = true
		if !openAIQuotaWindowRecovered(window, now) {
			return false
		}
	}
	return seenWindow
}

func openAIQuotaWindowRecovered(window *OpenAIRateLimitWindow, now time.Time) bool {
	if window == nil || math.IsNaN(window.UsedPercent) || math.IsInf(window.UsedPercent, 0) || window.UsedPercent < 0 {
		return false
	}
	if window.UsedPercent < 100 {
		return true
	}
	if window.ResetAfterSeconds <= 0 {
		return true
	}
	return window.ResetAt > 0 && !time.Unix(window.ResetAt, 0).After(now)
}

// openAICodexProbeQuotaRecovered applies the same recovery rule to the
// canonical 5h/7d fields produced by the ordinary /responses probe. Both
// windows are required so a partial header set cannot prematurely re-enable an
// account whose other window is still exhausted.
func openAICodexProbeQuotaRecovered(updates map[string]any, now time.Time) bool {
	return openAICodexProbeWindowRecovered(updates, "codex_5h", now) &&
		openAICodexProbeWindowRecovered(updates, "codex_7d", now)
}

func openAICodexProbeWindowRecovered(updates map[string]any, prefix string, now time.Time) bool {
	used, ok := recoveryFloat(updates, prefix+"_used_percent")
	if !ok || used < 0 {
		return false
	}
	if used < 100 {
		return true
	}

	if resetAfter, ok := recoveryInt(updates, prefix+"_reset_after_seconds"); ok && resetAfter <= 0 {
		return true
	}
	if resetAt, ok := recoveryTime(updates, prefix+"_reset_at"); ok && !resetAt.After(now) {
		return true
	}
	return false
}

func recoveryFloat(values map[string]any, key string) (float64, bool) {
	raw, ok := values[key]
	if !ok {
		return 0, false
	}
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
		parsed, err := v.Float64()
		if err != nil {
			return 0, false
		}
		value = parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		value = parsed
	default:
		return 0, false
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

func recoveryInt(values map[string]any, key string) (int64, bool) {
	raw, ok := values[key]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case int32:
		return int64(v), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return int64(v), true
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func recoveryTime(values map[string]any, key string) (time.Time, bool) {
	raw, ok := values[key]
	if !ok {
		return time.Time{}, false
	}
	value, ok := raw.(string)
	if !ok {
		return time.Time{}, false
	}
	parsed, err := parseTime(strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

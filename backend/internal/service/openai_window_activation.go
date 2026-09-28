package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const openAIWindowActivationExtraKey = "openai_window_activation"
const openAIWindowActivationRetry = 5 * time.Minute

// Account-specific daily schedule in the configured server timezone; 00:00 is midnight.
func activationMinute(value string) (int, bool) {
	if len(value) != 5 || value[2] != ':' {
		return 0, false
	}
	hour, errH := strconv.Atoi(value[:2])
	minute, errM := strconv.Atoi(value[3:])
	return hour*60 + minute, errH == nil && errM == nil && hour >= 0 && hour < 24 && minute >= 0 && minute < 60
}

func normalizeOpenAIWindowActivationExtra(platform, accountType string, isShadow bool, extra map[string]any) error {
	raw, exists := extra[openAIWindowActivationExtraKey]
	if !exists {
		return nil
	}
	if platform != PlatformOpenAI || accountType != AccountTypeOAuth || isShadow {
		return infraerrors.New(http.StatusBadRequest, "OPENAI_WINDOW_ACTIVATION_ACCOUNT_INVALID", "window activation requires an OpenAI OAuth parent account")
	}
	value, ok := raw.(map[string]any)
	if !ok || len(value) != 3 {
		return infraerrors.New(http.StatusBadRequest, "OPENAI_WINDOW_ACTIVATION_CONFIG_INVALID", "window activation requires enabled, start and end")
	}
	enabled, okEnabled := value["enabled"].(bool)
	start, okStart := value["start"].(string)
	end, okEnd := value["end"].(string)
	a, validStart := activationMinute(start)
	b, validEnd := activationMinute(end)
	if !okEnabled || !okStart || !okEnd || !validStart || !validEnd || a == b {
		return infraerrors.New(http.StatusBadRequest, "OPENAI_WINDOW_ACTIVATION_CONFIG_INVALID", "invalid window activation schedule")
	}
	extra[openAIWindowActivationExtraKey] = map[string]any{"enabled": enabled, "start": start, "end": end}
	return nil
}

func shouldActivateOpenAIWindow(account *Account, now time.Time, loc *time.Location) bool {
	if !isOpenAIAutoResetCreditAccount(account) || !account.IsActive() || account.Extra == nil {
		return false
	}
	raw, ok := account.Extra[openAIWindowActivationExtraKey].(map[string]any)
	if !ok || raw["enabled"] != true {
		return false
	}
	start, okStart := raw["start"].(string)
	end, okEnd := raw["end"].(string)
	if !okStart || !okEnd {
		return false
	}
	a, goodStart := activationMinute(start)
	b, goodEnd := activationMinute(end)
	if !goodStart || !goodEnd || a == b {
		return false
	}
	if loc == nil {
		return false
	}
	local := now.In(loc)
	current := local.Hour()*60 + local.Minute()
	if a < b {
		return current >= a && current < b
	}
	return current >= a || current < b
}

// The usage endpoint is observational: only a real /responses request starts a
// new upstream window. Require a 4–6h duration and a future reset time.
func openAI5hCountdownMissing(usage *OpenAIQuotaUsage, now time.Time) bool {
	if usage == nil || usage.RateLimit == nil {
		return true
	}
	return openAI5hResetAt(usage) <= now.Unix()
}

// Do not hammer an exhausted window when upstream omits its reset time. A
// recorded reset time in the past is different: that cycle has expired and a
// probe may start a fresh one.
func shouldProbeOpenAI5hActivation(usage *OpenAIQuotaUsage, now time.Time) bool {
	if !openAI5hCountdownMissing(usage, now) {
		return false
	}
	if usage == nil || usage.RateLimit == nil {
		return true
	}
	for _, window := range []*OpenAIRateLimitWindow{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow} {
		if window == nil || window.LimitWindowSeconds < 4*3600 || window.LimitWindowSeconds > 6*3600 || window.UsedPercent < 100 {
			continue
		}
		if window.ResetAt == 0 && (window.ResetAfterSeconds <= 0 || usage.FetchedAt <= 0) {
			return false
		}
	}
	return true
}

type openAIWindowActivationTester interface {
	RunTestBackground(context.Context, int64, string) (*ScheduledTestResult, error)
}

func (s *OpenAIQuotaAutoResetService) activateWindow(ctx context.Context, account *Account) {
	if s.accountTest == nil || !shouldActivateOpenAIWindow(account, time.Now(), s.activationLocation) {
		return
	}
	// The lock is deliberately not released: it throttles retries across workers
	// and across instances, including unsuccessful requests and restarts.
	if s.leaderLock == nil {
		return
	}
	lockKey := fmt.Sprintf("jobs:openai-window-activation:%d", account.ID)
	acquired, err := s.leaderLock.TryAcquireLeaderLock(ctx, lockKey, s.owner, openAIWindowActivationRetry)
	if err != nil || !acquired {
		return
	}
	usage, err := s.quota.QueryUsage(ctx, account.ID)
	if err != nil {
		slog.Warn("openai_window_activation_usage_failed", "account_id", account.ID, "error", err)
		return
	}
	if !shouldActivateOpenAIWindow(account, time.Now(), s.activationLocation) || !shouldProbeOpenAI5hActivation(usage, time.Now()) {
		return
	}
	result, err := s.accountTest.RunTestBackground(ctx, account.ID, "")
	if err != nil || result == nil || result.Status != "success" {
		slog.Warn("openai_window_activation_test_failed", "account_id", account.ID, "error", err)
		return
	}
	usage, err = s.quota.QueryUsage(ctx, account.ID)
	if err != nil {
		slog.Warn("openai_window_activation_recheck_failed", "account_id", account.ID, "error", err)
		return
	}
	if !openAI5hCountdownMissing(usage, time.Now()) {
		slog.Info("openai_window_activation_success", "account_id", account.ID, "reset_at", openAI5hResetAt(usage))
	} else {
		slog.Info("openai_window_activation_pending", "account_id", account.ID)
	}
}

func openAI5hResetAt(usage *OpenAIQuotaUsage) int64 {
	if usage == nil || usage.RateLimit == nil {
		return 0
	}
	var latest int64
	for _, w := range []*OpenAIRateLimitWindow{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow} {
		if w == nil || w.LimitWindowSeconds < 4*3600 || w.LimitWindowSeconds > 6*3600 {
			continue
		}
		resetAt := w.ResetAt
		if resetAt <= 0 && usage.FetchedAt > 0 && w.ResetAfterSeconds > 0 {
			resetAt = usage.FetchedAt + w.ResetAfterSeconds
		}
		if resetAt > latest {
			latest = resetAt
		}
	}
	return latest
}

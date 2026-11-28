package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"time"
)

func (w *ClaudeUsageWindow) UnmarshalJSON(raw []byte) error {
	var decoded struct {
		Utilization *float64 `json:"utilization"`
		ResetsAt    string   `json:"resets_at"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*w = ClaudeUsageWindow{ResetsAt: decoded.ResetsAt}
	if decoded.Utilization != nil {
		w.Utilization = *decoded.Utilization
		w.utilizationPresent = true
	}
	return nil
}

func (resp *ClaudeUsageResponse) fableWindow() ClaudeUsageWindow {
	if resp == nil {
		return ClaudeUsageWindow{}
	}
	if resp.SevenDayOverageIncluded.ResetsAt != "" {
		return resp.SevenDayOverageIncluded
	}
	// percent is already a percentage. A missing/null value is not zero.
	for _, limit := range resp.Limits {
		if limit.Kind != "weekly_scoped" || limit.Group != "weekly" ||
			limit.Scope.Model.DisplayName != "Fable" || limit.Percent == nil || *limit.Percent < 0 {
			continue
		}
		if surface := bytes.TrimSpace(limit.Scope.Surface); len(surface) > 0 && !bytes.Equal(surface, []byte("null")) {
			continue
		}
		if _, err := parseTime(limit.ResetsAt); err != nil {
			continue
		}
		return ClaudeUsageWindow{Utilization: *limit.Percent, ResetsAt: limit.ResetsAt, utilizationPresent: true}
	}
	return ClaudeUsageWindow{}
}

type modelQuotaRecoveryRepository interface {
	RecoverModelRateLimitIfUnchanged(context.Context, int64, string, json.RawMessage, time.Time, map[string]any) (bool, error)
}

func (s *AccountUsageService) recoverAnthropicFableModelQuota(ctx context.Context, account *Account, resp *ClaudeUsageResponse) {
	if account == nil || account.Platform != PlatformAnthropic || account.Type != AccountTypeOAuth ||
		account.UpdatedAt.IsZero() || account.modelRateLimitReason(anthropicFableRateLimitKey) != anthropicFableWindowReason ||
		!account.isRateLimitActiveForKey(anthropicFableRateLimitKey) {
		return
	}
	window := resp.fableWindow()
	if !window.utilizationPresent || window.Utilization < 0 || window.Utilization/100 >= anthropicFableWindowRecoveredUtilization ||
		math.IsNaN(window.Utilization) || math.IsInf(window.Utilization, 0) {
		return
	}
	reset, err := parseTime(window.ResetsAt)
	if err != nil || !reset.After(time.Now()) {
		return
	}
	repo, ok := s.accountRepo.(modelQuotaRecoveryRepository)
	if !ok {
		return
	}
	limits, _ := account.Extra[modelRateLimitsKey].(map[string]any)
	expected, err := json.Marshal(limits[anthropicFableRateLimitKey])
	if err != nil {
		return
	}
	updates := map[string]any{
		"passive_usage_7d_oi_utilization": window.Utilization / 100,
		"passive_usage_7d_oi_reset":       reset.Unix(),
		"passive_usage_sampled_at":        time.Now().UTC().Format(time.RFC3339),
	}
	// The row version also protects a concurrent 429 whose JSON happens to be
	// identical (model cooldown timestamps currently have second precision).
	cleared, err := repo.RecoverModelRateLimitIfUnchanged(ctx, account.ID, anthropicFableRateLimitKey, expected, account.UpdatedAt, updates)
	if err != nil {
		slog.Warn("anthropic_fable_quota_recovery_failed", "account_id", account.ID, "error", err)
		return
	}
	if cleared {
		clearAccountModelRateLimitSnapshot(account, anthropicFableRateLimitKey)
	}
}

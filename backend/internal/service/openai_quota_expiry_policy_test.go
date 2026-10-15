package service

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAssessExpiringUsefulReset(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		change func(*OpenAIQuotaUsage, *Account, *OpenAIAutoResetCreditConfig)
		want   bool
	}{
		{"useful expiring credit", func(*OpenAIQuotaUsage, *Account, *OpenAIAutoResetCreditConfig) {}, true},
		{"fully unused", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimit.PrimaryWindow.UsedPercent = 0
		}, false},
		{"below benefit", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimit.PrimaryWindow.UsedPercent = 24.99
		}, false},
		{"at minimum", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimit.PrimaryWindow.UsedPercent = 25
		}, true},
		{"later credit", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.autoResetCandidates[0].ExpiresAt = now.Add(2 * time.Hour).Format(time.RFC3339)
		}, false},
		{"expired credit", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.autoResetCandidates[0].ExpiresAt = now.Format(time.RFC3339)
		}, false},
		{"no credits", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimitResetCredits.AvailableCount = 0
		}, false},
		{"incomplete list", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimitResetCredits.AvailableCount = 2
		}, false},
		{"missing id", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.autoResetCandidates[0].ID = ""
		}, false},
		{"invalid expiry", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.autoResetCandidates[0].ExpiresAt = "bad"
		}, false},
		{"elapsed natural reset", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimit.PrimaryWindow.ResetAt = now.Unix()
		}, false},
		{"relative natural reset", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimit.PrimaryWindow.ResetAt = 0
			u.RateLimit.PrimaryWindow.ResetAfterSeconds = 600
		}, true},
		{"unknown natural reset", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimit.PrimaryWindow.ResetAt = 0
		}, false},
		{"unknown window", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimit.PrimaryWindow.LimitWindowSeconds = 7200
		}, false},
		{"weekly benefit", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimit.PrimaryWindow.LimitWindowSeconds = 7 * 24 * 3600
		}, true},
		{"nan", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimit.PrimaryWindow.UsedPercent = math.NaN()
		}, false},
		{"infinite", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimit.PrimaryWindow.UsedPercent = math.Inf(1)
		}, false},
		{"invalid usage", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.RateLimit.PrimaryWindow.UsedPercent = 101
		}, false},
		{"stale", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.FetchedAt = now.Add(-openAIAutoResetSnapshotTTL).Unix()
		}, false},
		{"future", func(u *OpenAIQuotaUsage, _ *Account, _ *OpenAIAutoResetCreditConfig) {
			u.FetchedAt = now.Add(time.Second).Unix()
		}, false},
		{"disabled", func(_ *OpenAIQuotaUsage, _ *Account, c *OpenAIAutoResetCreditConfig) { c.Enabled = false }, false},
		{"invalid horizon", func(_ *OpenAIQuotaUsage, _ *Account, c *OpenAIAutoResetCreditConfig) { c.ExpiryHorizonSeconds = 0 }, false},
		{"zero benefit config", func(_ *OpenAIQuotaUsage, _ *Account, c *OpenAIAutoResetCreditConfig) { c.ExpiryMinUtilization = 0 }, false},
		{"other provider", func(_ *OpenAIQuotaUsage, a *Account, _ *OpenAIAutoResetCreditConfig) { a.Platform = PlatformAnthropic }, false},
		{"previous ambiguous other card", func(u *OpenAIQuotaUsage, a *Account, _ *OpenAIAutoResetCreditConfig) {
			a.Extra = map[string]any{OpenAIAutoResetCreditStateExtraKey: &OpenAIAutoResetCreditState{Status: OpenAIAutoResetStatusFailed, AttemptCycleHash: shortOpenAIAutoResetHash(openAIAutoResetCycleSeed(u)), AttemptCreditHash: shortOpenAIAutoResetHash("missing")}}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &OpenAIQuotaUsage{FetchedAt: now.Unix(), RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 50, LimitWindowSeconds: 5 * 3600, ResetAt: now.Add(time.Hour).Unix()}}, RateLimitResetCredits: &OpenAIRateLimitResetCredits{AvailableCount: 1}, autoResetCandidates: []openAIAutoResetCreditCandidate{{ID: "credit-a", ExpiresAt: now.Add(30 * time.Minute).Format(time.RFC3339)}}}
			a := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			c := OpenAIAutoResetCreditConfig{Enabled: true, ExpiryHorizonSeconds: 3600, ExpiryMinUtilization: .25}
			tt.change(u, a, &c)
			require.Equal(t, tt.want, assessExpiringUsefulReset(u, a, c, now))
		})
	}
}

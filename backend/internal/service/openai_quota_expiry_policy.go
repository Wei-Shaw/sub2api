package service

import (
	"math"
	"strings"
	"time"
)

// assessExpiringUsefulReset evaluates only the expiry exception. It never
// redeems a credit or changes the worker's per-credit/per-cycle idempotency key.
// Call with a fresh native usage response inside the worker's existing lock.
func assessExpiringUsefulReset(usage *OpenAIQuotaUsage, account *Account, config OpenAIAutoResetCreditConfig, now time.Time) bool {
	if !isOpenAIAutoResetCreditAccount(account) || !config.Enabled || usage == nil || usage.RateLimit == nil || usage.RateLimitResetCredits == nil {
		return false
	}
	if config.ExpiryHorizonSeconds < 60 || config.ExpiryHorizonSeconds > 604800 || !isValidOpenAIAutoResetThreshold(config.ExpiryMinUtilization) {
		return false
	}
	fetched := time.Unix(usage.FetchedAt, 0)
	if usage.FetchedAt <= 0 || fetched.After(now) || now.Sub(fetched) >= openAIAutoResetSnapshotTTL {
		return false
	}
	// Reuse the same selection and retry affinity as the redemption path. A
	// previous ambiguous attempt must never become permission to try another card.
	candidate, err := selectOpenAIAutoResetCandidate(usage.autoResetCandidates, usage.RateLimitResetCredits.AvailableCount, openAIAutoResetStateFromExtra(account.Extra), shortOpenAIAutoResetHash(openAIAutoResetCycleSeed(usage)))
	if err != nil || strings.TrimSpace(candidate.ID) == "" {
		return false
	}
	expiry, err := time.Parse(time.RFC3339, candidate.ExpiresAt)
	if err != nil || !expiry.After(now) || expiry.After(now.Add(time.Duration(config.ExpiryHorizonSeconds)*time.Second)) {
		return false
	}
	// Require a measured benefit in an active standard window. Unknown/custom
	// windows, invalid utilization, and naturally elapsed windows prove nothing.
	for _, window := range []*OpenAIRateLimitWindow{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow} {
		if window == nil || (window.LimitWindowSeconds != 5*60*60 && window.LimitWindowSeconds != 7*24*60*60) {
			continue
		}
		used := window.UsedPercent / 100
		if math.IsNaN(used) || math.IsInf(used, 0) || used <= 0 || used > 1 || used < config.ExpiryMinUtilization {
			continue
		}
		resetAt := window.ResetAt
		if resetAt <= 0 && window.ResetAfterSeconds > 0 {
			resetAt = usage.FetchedAt + window.ResetAfterSeconds
		}
		if resetAt > now.Unix() {
			return true
		}
	}
	return false
}

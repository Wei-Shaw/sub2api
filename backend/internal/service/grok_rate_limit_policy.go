package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// grokRateLimitPolicy controls the account-level cooldown installed after a
// Grok upstream 429 (gateway.grok.rate_limit.*). The zero-value config maps to
// the built-in defaults, so deployments that never set these keys keep the
// historical 2m → 10m → 30m → 1h behavior.
//
// Typical operator profiles:
//   - small pool / single account: disable adaptive backoff so a bare 429 only
//     ever costs fallback_cooldown, instead of escalating the whole platform to 1h;
//   - large free-tier pool: raise min_cooldown (e.g. 24h) so a 429'd account
//     stays out of rotation even when xAI only sends a short Retry-After;
//   - manual operations: manual_recovery turns scheduling off on 429 and waits
//     for an admin to switch it back on.
type grokRateLimitPolicy struct {
	// fallbackCooldown applies when a 429 carries neither Retry-After nor an
	// exhausted quota window with a reset time.
	fallbackCooldown time.Duration
	// adaptiveBackoff escalates repeated OAuth 429s that arrive within
	// backoffQuietPeriod after the previous cooldown ended.
	adaptiveBackoff     bool
	repeatCooldown      time.Duration
	sustainedCooldown   time.Duration
	maxAdaptiveCooldown time.Duration
	backoffQuietPeriod  time.Duration
	// minCooldown is a floor for every 429 cooldown, including upstream
	// Retry-After and quota-window resets. Zero disables the floor.
	minCooldown time.Duration
	// manualRecovery turns account scheduling off after a gateway 429.
	manualRecovery bool
}

func defaultGrokRateLimitPolicy() grokRateLimitPolicy {
	return grokRateLimitPolicy{
		fallbackCooldown:    grokRateLimitFallbackCooldown,
		adaptiveBackoff:     true,
		repeatCooldown:      grokRateLimitRepeatCooldown,
		sustainedCooldown:   grokRateLimitSustainedCooldown,
		maxAdaptiveCooldown: grokRateLimitMaxAdaptiveCooldown,
		backoffQuietPeriod:  grokRateLimitBackoffQuietPeriod,
	}
}

// grokRateLimitPolicyFromConfig resolves the policy from config. Non-positive
// durations fall back to the built-in defaults so partially populated configs
// (tests, older config files) never produce a zero-length cooldown.
func grokRateLimitPolicyFromConfig(cfg *config.Config) grokRateLimitPolicy {
	policy := defaultGrokRateLimitPolicy()
	if cfg == nil {
		return policy
	}
	rl := cfg.Gateway.Grok.RateLimit
	seconds := func(v int, fallback time.Duration) time.Duration {
		if v <= 0 {
			return fallback
		}
		return time.Duration(v) * time.Second
	}
	policy.fallbackCooldown = seconds(rl.FallbackCooldownSeconds, policy.fallbackCooldown)
	policy.adaptiveBackoff = !rl.DisableAdaptiveBackoff
	policy.repeatCooldown = seconds(rl.RepeatCooldownSeconds, policy.repeatCooldown)
	policy.sustainedCooldown = seconds(rl.SustainedCooldownSeconds, policy.sustainedCooldown)
	policy.maxAdaptiveCooldown = seconds(rl.MaxAdaptiveCooldownSeconds, policy.maxAdaptiveCooldown)
	policy.backoffQuietPeriod = seconds(rl.BackoffQuietPeriodSeconds, policy.backoffQuietPeriod)
	if rl.MinCooldownSeconds > 0 {
		policy.minCooldown = time.Duration(rl.MinCooldownSeconds) * time.Second
	}
	policy.manualRecovery = rl.ManualRecovery
	return policy
}

func (s *OpenAIGatewayService) grokRateLimitPolicy() grokRateLimitPolicy {
	if s == nil {
		return defaultGrokRateLimitPolicy()
	}
	return grokRateLimitPolicyFromConfig(s.cfg)
}

// adaptiveCooldown picks the next escalation step from the previous cooldown
// length: anything shorter than repeat → repeat, ≥ repeat → sustained,
// ≥ sustained → max.
func (p grokRateLimitPolicy) adaptiveCooldown(previousCooldown time.Duration) time.Duration {
	switch {
	case previousCooldown >= p.sustainedCooldown:
		return p.maxAdaptiveCooldown
	case previousCooldown >= p.repeatCooldown:
		return p.sustainedCooldown
	default:
		return p.repeatCooldown
	}
}

func (p grokRateLimitPolicy) applyMinCooldown(resetAt, now time.Time) time.Time {
	if p.minCooldown <= 0 {
		return resetAt
	}
	if floor := now.Add(p.minCooldown); floor.After(resetAt) {
		return floor
	}
	return resetAt
}

// pauseGrokAccountForManualRecovery turns scheduling off so the account stays
// out of rotation after its cooldown expires, until an admin re-enables it.
// The cooldown itself is still installed, so re-enabling before it ends keeps
// honoring the upstream reset.
func (s *OpenAIGatewayService) pauseGrokAccountForManualRecovery(ctx context.Context, account *Account) {
	if s == nil || s.accountRepo == nil || account == nil || account.ID <= 0 || !account.Schedulable {
		return
	}
	if err := s.accountRepo.SetSchedulable(ctx, account.ID, false); err != nil {
		slog.Warn("grok_rate_limit_manual_recovery_pause_failed", "account_id", account.ID, "error", err)
		return
	}
	account.Schedulable = false
	slog.Warn("grok_rate_limit_manual_recovery_paused", "account_id", account.ID, "platform", account.Platform)
}

package service

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Real Ollama Cloud usage accounts (API-key accounts whose base_url points at
// ollama.com) have quota-driven 429s; their headers must not be re-read as an
// OpenAI codex / Anthropic / CN limit. handleOllamaCloudUsage429 applies a
// never-shrinking immediate cooldown (valid Retry-After, else the seconds
// fallback), announces runtime scheduling from the authoritative row, then
// schedules an async usage probe (ollama_cloud_usage_rate_limit_probe.go) to
// learn the true window reset. The probe callback only writes back while the
// account still matches the trigger's fingerprint and rate-limit generation,
// enforced atomically in the repository CAS, so a stale async result cannot
// overwrite a newer 429, an admin clear/re-arm, a disabled account, or a key
// swap. No in-memory per-account guard is kept; each pending probe carries its
// own version in its callback closure.

const ollamaCloudUsageProbeWritebackTimeout = 10 * time.Second

// ollamaCloudUsageProbeScheduler is the single-method surface RateLimitService
// needs from the Ollama Cloud usage service. It is optional.
type ollamaCloudUsageProbeScheduler interface {
	ScheduleOllamaCloudUsageRateLimitProbe(accountID int64, onExhausted OllamaCloudUsageRateLimitProbeCallback) bool
}

// ollamaCloudUsageRateLimitExtender extends (never shrinks) an account-level
// rate limit via the concrete repository (SetRateLimitedIfLater).
type ollamaCloudUsageRateLimitExtender interface {
	SetRateLimitedIfLater(ctx context.Context, id int64, resetAt time.Time) error
}

// ollamaCloudUsageRateLimitSetterIfGeneration atomically writes a new reset only
// while the account still carries the exact generation the caller observed
// (UpdatedAt == expectedUpdatedAt AND RateLimitedAt == expectedLimitedAt AND
// RateLimitResetAt == expectedResetAt, where nil means unset), returning whether
// it updated. Provided by the concrete repository (SetRateLimitedIfUnchanged).
type ollamaCloudUsageRateLimitSetterIfGeneration interface {
	SetRateLimitedIfUnchanged(ctx context.Context, id int64, expectedUpdatedAt time.Time, expectedLimitedAt, expectedResetAt *time.Time, newResetAt time.Time) (bool, error)
}

// SetOllamaCloudUsageProbeScheduler injects the optional probe scheduler. When
// nil, handle429 still cools the account and announces runtime scheduling; only
// the async probe is skipped. Wire calls this once at startup.
func (s *RateLimitService) SetOllamaCloudUsageProbeScheduler(scheduler ollamaCloudUsageProbeScheduler) {
	s.ollamaCloudUsageProbe = scheduler
}

func (s *RateLimitService) handleOllamaCloudUsage429(ctx context.Context, account *Account, headers http.Header) {
	if s == nil || account == nil || account.ID <= 0 || s.accountRepo == nil {
		return
	}

	var shortReset time.Time
	now := time.Now()
	if d := retryAfter(headers, now); d > 0 {
		shortReset = now.Add(d)
	} else if cooldown, enabled := s.get429FallbackCooldown(ctx, account); enabled {
		shortReset = now.Add(cooldown)
	} else {
		slog.Info("rate_limit_ollama_429_fallback_ignored", "account_id", account.ID, "platform", account.Platform)
	}

	if !shortReset.IsZero() {
		s.applyOllamaCloudUsageImmediateCooldown(ctx, account, shortReset)
	}

	// Read the authoritative row (IfLater may have kept a longer pre-existing
	// reset) and announce runtime scheduling from it. This is independent of the
	// optional probe scheduler.
	authoritative, err := s.accountRepo.GetByID(ctx, account.ID)
	if err != nil || authoritative == nil {
		slog.Warn("ollama_cloud_usage_authoritative_load_failed", "account_id", account.ID, "error", err)
		return
	}
	if authoritative.RateLimitResetAt != nil && authoritative.RateLimitResetAt.After(now) {
		s.notifyAccountSchedulingBlocked(authoritative, *authoritative.RateLimitResetAt, "ollama_429")
	}

	if s.ollamaCloudUsageProbe == nil {
		return
	}
	s.scheduleOllamaCloudUsageProbe(authoritative)
}

// applyOllamaCloudUsageImmediateCooldown persists the short cooldown with the
// atomic never-shrink IfLater write when available. Runtime announcement is done
// later from the authoritative row, never here, so an IfLater no-op never sends a
// shorter runtime-block notification.
func (s *RateLimitService) applyOllamaCloudUsageImmediateCooldown(ctx context.Context, account *Account, shortReset time.Time) {
	if extender, ok := s.accountRepo.(ollamaCloudUsageRateLimitExtender); ok {
		if err := extender.SetRateLimitedIfLater(ctx, account.ID, shortReset); err != nil {
			slog.Warn("rate_limit_ollama_429_iflater_failed", "account_id", account.ID, "error", err)
		}
		return
	}
	reset := shortReset
	if account.RateLimitResetAt != nil && account.RateLimitResetAt.After(reset) {
		reset = *account.RateLimitResetAt
	}
	if err := s.accountRepo.SetRateLimited(ctx, account.ID, reset); err != nil {
		slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
	}
}

// scheduleOllamaCloudUsageProbe captures the trigger fingerprint and rate-limit
// generation from the authoritative row and schedules the async probe bound to
// them.
func (s *RateLimitService) scheduleOllamaCloudUsageProbe(account *Account) {
	if s == nil || account == nil || s.ollamaCloudUsageProbe == nil {
		return
	}
	if !IsOllamaCloudUsageAccount(account) {
		return
	}
	fingerprint, valid := ollamaCloudUsageGroupFingerprint(account)
	if !valid {
		return
	}
	expectedLimitedAt := cloneTimePtr(account.RateLimitedAt)
	expectedResetAt := cloneTimePtr(account.RateLimitResetAt)
	accepted := s.ollamaCloudUsageProbe.ScheduleOllamaCloudUsageRateLimitProbe(
		account.ID,
		func(accountID int64, resetAt time.Time) {
			s.applyOllamaCloudUsageProbeReset(accountID, fingerprint, expectedLimitedAt, expectedResetAt, resetAt)
		},
	)
	if !accepted {
		// Service stopped or the probe queue is full; the account is already
		// cooled by the immediate cooldown, so no retry or other action is needed.
		slog.Debug("ollama_cloud_usage_probe_schedule_rejected", "account_id", account.ID)
	}
}

// applyOllamaCloudUsageProbeReset writes the reset learned by an async usage
// probe, but only while the account is still the same active Ollama Cloud usage
// account with the same rate-limit generation observed at scheduling time. It
// runs on the probe coordinator with a detached, timeout-bounded context and does
// at most one bounded re-read plus one bounded atomic CAS.
//
// The result is dropped when any of the following holds:
//   - the reset is not in the future, or the account is gone;
//   - the account is no longer an active/schedulable Ollama Cloud usage account
//     (disabled/errored since the trigger) or its group fingerprint changed;
//   - the reset does not extend the trigger's cooldown floor (an explicit
//     Retry-After or an already-confirmed cooldown must never be shortened);
//   - the CAS reports no update (the rate-limit generation moved on: a newer 429,
//     an admin clear, a re-arm, or a concurrent update between the re-read and
//     this write, guarded by the row's UpdatedAt).
//
// On success the CAS already updated the DB; only the runtime scheduling
// notification is sent here.
func (s *RateLimitService) applyOllamaCloudUsageProbeReset(
	accountID int64,
	expectedFingerprint string,
	expectedLimitedAt, expectedResetAt *time.Time,
	resetAt time.Time,
) {
	now := time.Now()
	if s == nil || accountID <= 0 || s.accountRepo == nil || !resetAt.After(now) {
		return
	}
	// Never write an earlier reset than the trigger floor captured at schedule.
	if expectedResetAt != nil && !resetAt.After(*expectedResetAt) {
		return
	}

	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), ollamaCloudUsageProbeWritebackTimeout)
	defer cancel()

	account, err := s.accountRepo.GetByID(bgCtx, accountID)
	if err != nil || account == nil {
		slog.Warn("ollama_cloud_usage_probe_reset_load_failed", "account_id", accountID, "error", err)
		return
	}
	if !IsOllamaCloudUsageAccount(account) {
		return
	}
	if !account.IsActive() || !account.Schedulable {
		return
	}
	currentFingerprint, valid := ollamaCloudUsageGroupFingerprint(account)
	if !valid || currentFingerprint != expectedFingerprint {
		return
	}

	setter, ok := s.accountRepo.(ollamaCloudUsageRateLimitSetterIfGeneration)
	if !ok {
		return
	}
	// Bind the CAS to the version just re-read (not the scheduling-time version,
	// which the probe's own snapshot persist legitimately bumps via UpdatedAt).
	// Any change between this re-read and the CAS therefore fails the write.
	updated, err := setter.SetRateLimitedIfUnchanged(bgCtx, accountID, account.UpdatedAt, expectedLimitedAt, expectedResetAt, resetAt)
	if err != nil {
		slog.Warn("rate_limit_set_failed", "account_id", accountID, "error", err)
		return
	}
	if !updated {
		// The generation moved on (a newer 429, admin clear/re-arm, or an
		// unrelated write bumped UpdatedAt): conservatively skip, never override.
		// No credentials are logged. A later 429 for the same account schedules a
		// fresh probe with a fresh generation, so no retry is needed here.
		slog.Debug("ollama_cloud_usage_probe_reset_skipped_stale", "account_id", accountID)
		return
	}

	s.notifyAccountSchedulingBlocked(account, resetAt, "ollama_cloud_usage_429_probe")
	slog.Info("ollama_cloud_account_rate_limited_probe",
		"account_id", accountID,
		"reset_at", resetAt.UTC(),
		"reset_in", time.Until(resetAt).Truncate(time.Second),
	)
}

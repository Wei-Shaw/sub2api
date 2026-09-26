package service

import (
	"context"
	"log/slog"
	"time"
)

// This file adds the event-triggered rate-limit probe to OllamaCloudUsageService:
// a model-429 handler calls ScheduleOllamaCloudUsageRateLimitProbe to prompt a
// controlled usage refresh now (instead of waiting for the regular cycle) and to
// learn the recovery horizon when the account's usage is actually exhausted.
//
// The probe reuses refreshAccount wholesale (singleflight, bounded concurrency
// slot, group resolution, settings-HTML fetch); no HTTP/parsing is duplicated.
// Work runs on a single background coordinator owned by Start/Stop with a
// detached, timeout-bounded context, so a model request never blocks on a probe
// and no unbounded goroutines are introduced.

const (
	// ollamaCloudUsageProbeTimeout bounds one detached probe task. It must exceed
	// the underlying settings-HTML request timeout.
	ollamaCloudUsageProbeTimeout = 45 * time.Second

	// ollamaCloudUsageProbeMaxQueue bounds queued (not yet serviced) probe
	// requests. A duplicate request for an already-queued account still merges
	// into the latest callback even when the queue is full.
	ollamaCloudUsageProbeMaxQueue = 256

	// ollamaCloudUsageProbeGroupRetention bounds how long finished probe results
	// are retained for coalescing before being pruned.
	ollamaCloudUsageProbeGroupRetention = 2 * ollamaCloudUsageManualRefreshInterval
)

// OllamaCloudUsageRateLimitProbeCallback is invoked (on the coordinator goroutine)
// when an account's Ollama Cloud usage is confirmed exhausted from a fresh
// successful snapshot; resetAt is the earliest wall-clock rollover of the
// exhausted window(s). It is never invoked for accounts that are not configured
// Ollama usage accounts, lack a web session, or whose most recent snapshot is not
// exhausted.
//
// The callback runs on the single coordinator, so it must not block indefinitely:
// the next serialized probe (and Stop) would be held back. It MAY do bounded
// persistence — e.g. a limited DB write-back under its own timeout context — and
// must not spawn unbounded goroutines. Stop waits the coordinator, so it may wait
// out an in-flight callback's own bounded write-back timeout (shorter than the
// probe task timeout) rather than a strict probe timeout; keep any persistence
// bounded so Stop returns promptly. The 429-wiring author is responsible for how
// the callback maps to a rate-limit write-back.
type OllamaCloudUsageRateLimitProbeCallback func(accountID int64, resetAt time.Time)

// ollamaCloudUsageProbeRequest is one queued, per-account probe request.
type ollamaCloudUsageProbeRequest struct {
	accountID   int64
	onExhausted OllamaCloudUsageRateLimitProbeCallback
}

// ollamaCloudUsageProbeGroupEntry is the most recent probe fetch outcome for an
// api_key group, kept briefly so coalesced members reuse the same upstream result
// instead of each issuing its own fetch.
type ollamaCloudUsageProbeGroupEntry struct {
	attemptAt time.Time
	snapshot  *OllamaCloudUsageSnapshot
}

// ScheduleOllamaCloudUsageRateLimitProbe schedules an asynchronous probe of the
// account's Ollama Cloud usage and reports whether the request was accepted. It
// never blocks on network or DB work: it only records the request for the
// background coordinator and returns, so it is safe to call from a model-429 path.
// The probe runs under a detached, timeout-bounded context derived from the
// service lifecycle.
//
// Accepted == true means the request was queued, not that a result will follow:
// an ineligible account quietly yields no callback. Accepted == false means the
// service is not running (not started / stopped) or the queue is full for a brand
// new account; a duplicate for an already-queued account is always merged.
func (s *OllamaCloudUsageService) ScheduleOllamaCloudUsageRateLimitProbe(
	accountID int64,
	onExhausted OllamaCloudUsageRateLimitProbeCallback,
) bool {
	if s == nil || onExhausted == nil || accountID <= 0 {
		return false
	}
	s.mu.Lock()
	running := s.started && !s.stopped
	s.mu.Unlock()
	if !running {
		return false
	}

	s.probeMu.Lock()
	// Coalesce a duplicate for the same account first (keep the latest callback)
	// so a full queue still accepts a merge.
	for i := range s.probeQueue {
		if s.probeQueue[i].accountID == accountID {
			s.probeQueue[i].onExhausted = onExhausted
			s.probeMu.Unlock()
			s.wakeProbeLoop()
			return true
		}
	}
	if len(s.probeQueue) >= ollamaCloudUsageProbeMaxQueue {
		s.probeMu.Unlock()
		return false
	}
	s.probeQueue = append(s.probeQueue, ollamaCloudUsageProbeRequest{
		accountID:   accountID,
		onExhausted: onExhausted,
	})
	s.probeMu.Unlock()

	s.wakeProbeLoop()
	return true
}

// wakeProbeLoop nudges the coordinator; a dropped wake is safe because the
// coordinator re-checks the queue after every wake.
func (s *OllamaCloudUsageService) wakeProbeLoop() {
	if s == nil {
		return
	}
	select {
	case s.probeWake <- struct{}{}:
	default:
	}
}

// probeLoop is the single background worker. It is added to the service WaitGroup
// in Start and returns when parentCtx is cancelled in Stop, so Stop waits for any
// in-flight probe (including a bounded callback) to finish under its own timeout.
func (s *OllamaCloudUsageService) probeLoop() {
	defer s.wg.Done()
	for {
		s.probeMu.Lock()
		if len(s.probeQueue) == 0 {
			s.probeMu.Unlock()
			select {
			case <-s.probeWake:
			case <-s.parentCtx.Done():
				return
			}
			continue
		}
		batch := s.probeQueue
		s.probeQueue = nil
		s.probeMu.Unlock()

		for _, req := range batch {
			if s.parentCtx.Err() != nil {
				return
			}
			ctx, cancel := context.WithTimeout(s.parentCtx, ollamaCloudUsageProbeTimeout)
			s.runOllamaCloudUsageProbe(ctx, req.accountID, req.onExhausted)
			cancel()
		}
	}
}

func (s *OllamaCloudUsageService) probeGroupResult(key string) (ollamaCloudUsageProbeGroupEntry, bool) {
	if s == nil {
		return ollamaCloudUsageProbeGroupEntry{}, false
	}
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	entry, ok := s.probeGroups[key]
	return entry, ok
}

func (s *OllamaCloudUsageService) storeProbeGroupResult(key string, entry ollamaCloudUsageProbeGroupEntry) {
	if s == nil {
		return
	}
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if s.probeGroups == nil {
		s.probeGroups = make(map[string]ollamaCloudUsageProbeGroupEntry)
	}
	pruneAt := entry.attemptAt.Add(-ollamaCloudUsageProbeGroupRetention)
	for groupKey, existing := range s.probeGroups {
		if existing.attemptAt.Before(pruneAt) {
			delete(s.probeGroups, groupKey)
		}
	}
	s.probeGroups[key] = entry
}

// runOllamaCloudUsageProbe resolves the account and, when appropriate, performs a
// controlled group refresh and reports exhaustion. All decisions are best-effort;
// accounts that cannot be probed at all are skipped with a single Debug line, and
// every executed probe reports its outcome (fetch failure, non-exhaustion, or an
// exhausted window without a usable reset) exactly once.
//
// This is a 429-event recovery query: it deliberately runs regardless of the
// periodic auto_refresh switch (a model was just refused, so a one-off read is
// warranted even when auto-refresh is off) and regardless of a scrape-429
// failure backoff recorded on the snapshot — the model 429 is itself fresh
// evidence that the account still needs a recovery horizon, and the auto-refresh
// chain keeps enforcing its own backoff unchanged. It still respects the
// presence of a configured session/cookie and the 30s group cooldown.
func (s *OllamaCloudUsageService) runOllamaCloudUsageProbe(
	ctx context.Context,
	accountID int64,
	onExhausted OllamaCloudUsageRateLimitProbeCallback,
) {
	if s == nil || s.accountRepo == nil || onExhausted == nil || ctx.Err() != nil {
		return
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		logOllamaCloudUsageProbeSkipped(accountID, "account_unavailable", err)
		return
	}
	if !IsOllamaCloudUsageAccount(account) {
		logOllamaCloudUsageProbeSkipped(accountID, "account_ineligible", nil)
		return
	}
	// Resolve the api_key group so the account sees the group's managed session
	// and most recent snapshot even when it is not the row that last refreshed.
	if err := s.ResolveAccounts(ctx, []*Account{account}); err != nil {
		logOllamaCloudUsageProbeSkipped(accountID, "group_resolve_failed", err)
		return
	}
	if !ollamaCloudUsageConfigured(account) {
		logOllamaCloudUsageProbeSkipped(accountID, "session_missing", nil)
		return
	}
	key, valid := ollamaCloudUsageGroupFingerprint(account)
	if !valid {
		logOllamaCloudUsageProbeSkipped(accountID, "group_fingerprint_invalid", nil)
		return
	}

	window := ollamaCloudUsageManualRefreshInterval
	accountSnapshot := decodeOllamaCloudUsageSnapshot(account.Extra)
	cached, hasCached := s.probeGroupResult(key)

	// The most recent observation wins, so an older success never masks a newer
	// one (success or failure) and never drives a stale exhaustion report.
	now := s.currentTime()
	if newest := ollamaCloudUsageProbeNewestSuccess(accountSnapshot, cached, hasCached, now, window); newest != nil {
		maybeOllamaCloudUsageProbeExhaustion(ctx, accountID, newest, s.currentTime(), window, onExhausted)
		return
	}

	// A model 429 is itself fresh evidence that the account still needs a
	// recovery horizon, so this probe deliberately ignores any scrape-429
	// failure backoff recorded on the snapshot (the auto-refresh chain keeps
	// enforcing its own NextRefreshAt). The 30s group cooldown after the most
	// recent group attempt remains the only throttle here.
	if hasCached && now.Before(cached.attemptAt.Add(window)) {
		return
	}

	settings, settingsErr := s.GetSettings(ctx)
	if settingsErr != nil {
		logOllamaCloudUsageProbeSkipped(accountID, "settings_unavailable", settingsErr)
		return
	}
	fetched, refreshErr := s.refreshAccount(ctx, accountID, settings, false)
	// A nil success means refreshAccount piggybacked on a singleflight run owned
	// by another request (a timed-cycle refresh or a sibling probe). That owner
	// already stored the group result, so this no-op must not be recorded as a
	// fresh attempt (it would wrongly start a 30s group cooldown or mask state).
	if refreshErr == nil && fetched == nil {
		return
	}
	// Evaluate against when the fetch actually completed: a reset that rolled over
	// during a slow fetch must not be reported as still-future, and the group
	// cooldown must not start before the attempt finished.
	doneNow := s.currentTime()
	s.storeProbeGroupResult(key, ollamaCloudUsageProbeGroupEntry{attemptAt: doneNow, snapshot: fetched})
	if refreshErr != nil {
		// No snapshot came back (transport/pre-check failure): the error itself
		// is the only failure evidence.
		slog.Warn("ollama_cloud_usage_probe_fetch_failed",
			"account_id", accountID,
			"reason", refreshErr.Error())
		return
	}
	if fetched.Status != OllamaCloudUsageStatusOK {
		// persistFailure does not return an error: a failed scrape is reported
		// through the returned snapshot's status, reason and counters.
		slog.Warn("ollama_cloud_usage_probe_fetch_failed",
			"account_id", accountID,
			"reason", fetched.LastError,
			"http_status", fetched.HTTPStatus,
			"failure_count", fetched.FailureCount)
		return
	}
	maybeOllamaCloudUsageProbeExhaustion(ctx, accountID, fetched, doneNow, window, onExhausted)
}

// ollamaCloudUsageProbeObservedAt returns the wall-clock time at which a snapshot
// was actually observed: FetchedAt for a success (a failure carries a stale
// FetchedAt copied from a prior success), otherwise LastAttemptAt.
func ollamaCloudUsageProbeObservedAt(snapshot *OllamaCloudUsageSnapshot) (time.Time, bool) {
	if snapshot == nil {
		return time.Time{}, false
	}
	if snapshot.Status == OllamaCloudUsageStatusOK && snapshot.FetchedAt != nil && !snapshot.FetchedAt.IsZero() {
		return snapshot.FetchedAt.UTC(), true
	}
	if !snapshot.LastAttemptAt.IsZero() {
		return snapshot.LastAttemptAt.UTC(), true
	}
	return time.Time{}, false
}

// ollamaCloudUsageProbeNewestSuccess returns the most recent observation when it
// is a fresh, successful snapshot that may feed the exhaustion decision, or nil
// otherwise. The candidate with the latest actual observation time is picked, so
// neither the account row nor the coordinator cache is unconditionally preferred,
// and a newer failure (or an unknown recent attempt) suppresses an older success
// rather than letting it drive a stale exhaustion report.
func ollamaCloudUsageProbeNewestSuccess(
	accountSnapshot *OllamaCloudUsageSnapshot,
	cached ollamaCloudUsageProbeGroupEntry,
	hasCached bool,
	now time.Time,
	window time.Duration,
) *OllamaCloudUsageSnapshot {
	var newest *OllamaCloudUsageSnapshot
	var newestAt time.Time
	consider := func(snapshot *OllamaCloudUsageSnapshot) {
		if snapshot == nil {
			return
		}
		at, ok := ollamaCloudUsageProbeObservedAt(snapshot)
		if !ok {
			return
		}
		if newest == nil || at.After(newestAt) {
			newest = snapshot
			newestAt = at
		}
	}
	consider(accountSnapshot)
	if hasCached {
		if cached.snapshot != nil {
			consider(cached.snapshot)
		} else if !cached.attemptAt.IsZero() && (newest == nil || cached.attemptAt.After(newestAt)) {
			// The most recent group attempt produced no snapshot (an errored
			// fetch); it supersedes any older success.
			newest = nil
			newestAt = cached.attemptAt
		}
	}
	if newest == nil || newest.Status != OllamaCloudUsageStatusOK {
		return nil
	}
	if newest.FetchedAt == nil || newest.FetchedAt.IsZero() || newest.FetchedAt.Before(now.Add(-window)) {
		return nil
	}
	return newest
}

// ollamaCloudUsageSnapshotHasExhaustedWindow reports whether any usage window is
// at or above 100% used, mirroring the window rule of
// ollamaCloudUsageExhaustionResetAt. It only classifies snapshots that failed to
// yield a recovery horizon.
func ollamaCloudUsageSnapshotHasExhaustedWindow(snapshot *OllamaCloudUsageSnapshot) bool {
	if snapshot == nil || snapshot.Data == nil {
		return false
	}
	for _, window := range []*OllamaCloudUsageWindow{snapshot.Data.FiveHour, snapshot.Data.SevenDay} {
		if window != nil && window.UsedPercent >= 100 {
			return true
		}
	}
	return false
}

// logOllamaCloudUsageProbeSkipped reports a probe request that could not run at
// all (missing/ineligible account, missing session, group or settings failure).
// Each scheduled probe emits at most one such Debug line, so these stay quiet
// while remaining diagnosable.
func logOllamaCloudUsageProbeSkipped(accountID int64, reason string, err error) {
	attrs := []any{"account_id", accountID, "reason", reason}
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	slog.Debug("ollama_cloud_usage_probe_skipped", attrs...)
}

// logOllamaCloudUsageProbeOutcome reports a fresh, successful probe observation
// that does not yield a recovery horizon. A window at or above 100% used whose
// reset could not be determined is a Warn (the account is still blocked upstream
// with no known recovery time); a genuinely not-exhausted observation is an Info.
func logOllamaCloudUsageProbeOutcome(accountID int64, snapshot *OllamaCloudUsageSnapshot) {
	attrs := []any{"account_id", accountID}
	if data := snapshot.Data; data != nil {
		if data.FiveHour != nil {
			attrs = append(attrs, "five_hour_used_percent", data.FiveHour.UsedPercent)
		}
		if data.SevenDay != nil {
			attrs = append(attrs, "seven_day_used_percent", data.SevenDay.UsedPercent)
		}
	}
	if ollamaCloudUsageSnapshotHasExhaustedWindow(snapshot) {
		slog.Warn("ollama_cloud_usage_probe_reset_unknown", attrs...)
		return
	}
	slog.Info("ollama_cloud_usage_probe_not_exhausted", attrs...)
}

// maybeOllamaCloudUsageProbeExhaustion invokes onExhausted only when the snapshot
// is a fresh, successful, genuinely exhausted one (per
// ollamaCloudUsageExhaustionResetAt). Non-exhausted or unusable snapshots yield
// no callback but are logged once per probe (see logOllamaCloudUsageProbeOutcome).
func maybeOllamaCloudUsageProbeExhaustion(
	ctx context.Context,
	accountID int64,
	snapshot *OllamaCloudUsageSnapshot,
	now time.Time,
	window time.Duration,
	onExhausted OllamaCloudUsageRateLimitProbeCallback,
) {
	if snapshot == nil || onExhausted == nil {
		return
	}
	resetAt, exhausted := ollamaCloudUsageExhaustionResetAt(snapshot, now, now.Add(-window))
	if !exhausted {
		logOllamaCloudUsageProbeOutcome(accountID, snapshot)
		return
	}
	if ctx.Err() != nil {
		// Probe was cancelled/stopped: do not deliver a stale callback.
		return
	}
	onExhausted(accountID, resetAt)
}

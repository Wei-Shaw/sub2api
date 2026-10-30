package service

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ollamaCloudProbeUsageBody renders a minimal settings page exposing only the
// five-hour usage window at the given percent and an absolute reset time.
func ollamaCloudProbeUsageBody(percent int, resetRFC3339 string) []byte {
	return []byte(fmt.Sprintf(`
		<section>
			<div><span>5 hour usage</span><span>%d%% used</span></div>
			<time datetime="%s"></time>
		</section>`, percent, resetRFC3339))
}

// TestOllamaCloudProbeUsageBodiesParse pins the crafted HTML fixtures used by the
// probe tests: 100% usage is exhausted and must carry the future reset.
func TestOllamaCloudProbeUsageBodiesParse(t *testing.T) {
	reset := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	exhausted, err := parseOllamaCloudUsageHTML(ollamaCloudProbeUsageBody(100, reset.Format(time.RFC3339)))
	require.NoError(t, err)
	require.NotNil(t, exhausted.FiveHour)
	require.Equal(t, float64(100), exhausted.FiveHour.UsedPercent)
	require.NotNil(t, exhausted.FiveHour.ResetAt)
	require.True(t, exhausted.FiveHour.ResetAt.Equal(reset), "reset = %v", exhausted.FiveHour.ResetAt)

	mild, err := parseOllamaCloudUsageHTML(ollamaCloudProbeUsageBody(5, reset.Format(time.RFC3339)))
	require.NoError(t, err)
	require.NotNil(t, mild.FiveHour)
	require.Equal(t, float64(5), mild.FiveHour.UsedPercent)
}

// ollamaCloudProbeFixture builds a started service with a fixed clock so exhaustion
// and reset decisions are deterministic.
func ollamaCloudProbeFixture(t *testing.T, repo AccountRepository, upstream HTTPUpstream, fixedNow time.Time) *OllamaCloudUsageService {
	t.Helper()
	svc := NewOllamaCloudUsageService(repo, upstream, NewSettingService(&upstreamBillingProbeSettingRepo{}, nil), ollamaUsageTestEncryptor{}, true)
	svc.now = func() time.Time { return fixedNow }
	svc.Start()
	t.Cleanup(svc.Stop)
	return svc
}

func ollamaCloudProbeReset(t *testing.T, fixedNow time.Time) time.Time {
	t.Helper()
	return fixedNow.Add(2 * time.Hour).UTC()
}

func TestOllamaCloudUsageScheduleRateLimitProbeReturnsWhileProbeRuns(t *testing.T) {
	// A gated GetByID proves ScheduleRateLimitProbe does not synchronously block
	// on the probe's database/network work.
	fixedNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	account := ollamaUsageAccount(1)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	base := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{
		accounts: map[int64]*Account{account.ID: account},
	}}
	repo := &ollamaCloudProbeGateRepo{ollamaUsageTestRepo: base, entered: make(chan struct{}), release: make(chan struct{})}
	repo.gating.Store(true)
	upstream := &ollamaUsageHTTPStub{body: ollamaCloudProbeUsageBody(100, ollamaCloudProbeReset(t, fixedNow).Format(time.RFC3339))}
	svc := ollamaCloudProbeFixture(t, repo, upstream, fixedNow)

	called := make(chan time.Time, 1)
	accepted := svc.ScheduleOllamaCloudUsageRateLimitProbe(account.ID, func(id int64, reset time.Time) {
		called <- reset
	})
	require.True(t, accepted, "running service must accept the probe")

	// The fetch is still blocked on the gate, yet Schedule already returned:
	// this is only possible if the probe runs asynchronously on the coordinator.
	require.Eventually(t, func() bool {
		select {
		case <-repo.entered:
			return true
		default:
			return false
		}
	}, 5*time.Second, time.Millisecond, "coordinator must reach the gated GetByID")
	require.Zero(t, upstream.calls.Load(), "no upstream fetch while the probe is gated")
	select {
	case <-called:
		t.Fatal("callback must not fire before the probe completes")
	default:
	}

	repo.gating.Store(false)
	close(repo.release)
	select {
	case reset := <-called:
		require.True(t, reset.Equal(ollamaCloudProbeReset(t, fixedNow)))
	case <-time.After(5 * time.Second):
		t.Fatal("expected an exhaustion callback after the probe completed")
	}
	require.Equal(t, int64(1), upstream.calls.Load())
}

func TestOllamaCloudUsageRateLimitProbeExhaustedReportsReset(t *testing.T) {
	fixedNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	account := ollamaUsageAccount(11)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{
		accounts: map[int64]*Account{account.ID: account},
	}}
	wantReset := ollamaCloudProbeReset(t, fixedNow)
	upstream := &ollamaUsageHTTPStub{body: ollamaCloudProbeUsageBody(100, wantReset.Format(time.RFC3339))}
	svc := ollamaCloudProbeFixture(t, repo, upstream, fixedNow)

	called := make(chan time.Time, 1)
	require.True(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(account.ID, func(id int64, reset time.Time) {
		require.Equal(t, account.ID, id)
		called <- reset
	}))
	select {
	case reset := <-called:
		require.True(t, reset.Equal(wantReset))
	case <-time.After(5 * time.Second):
		t.Fatal("expected a callback for an exhausted account")
	}
	require.Equal(t, int64(1), upstream.calls.Load())
}

func TestOllamaCloudUsageRateLimitProbeNotExhaustedNoCallback(t *testing.T) {
	fixedNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	account := ollamaUsageAccount(12)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{
		accounts: map[int64]*Account{account.ID: account},
	}}
	upstream := &ollamaUsageHTTPStub{body: ollamaCloudProbeUsageBody(5, ollamaCloudProbeReset(t, fixedNow).Format(time.RFC3339))}
	svc := ollamaCloudProbeFixture(t, repo, upstream, fixedNow)

	called := make(chan struct{}, 1)
	require.True(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(account.ID, func(int64, time.Time) {
		called <- struct{}{}
	}))
	select {
	case <-called:
		t.Fatal("not-exhausted usage must not invoke the callback")
	case <-time.After(300 * time.Millisecond):
	}
	require.Equal(t, int64(1), upstream.calls.Load(), "a fresh probe must still refresh once")
}

func TestOllamaCloudUsageRateLimitProbeSameGroupCoalescesIntoOneFetchPerAccount(t *testing.T) {
	fixedNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	first := ollamaUsageAccount(21)
	first.Credentials["api_key"] = "shared-key"
	first.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=shared"
	second := ollamaUsageAccount(22)
	second.Platform = PlatformAnthropic
	second.Credentials = map[string]any{"base_url": "https://www.ollama.com/v1", "api_key": "shared-key"}
	second.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=shared"
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{
		accounts: map[int64]*Account{first.ID: first, second.ID: second},
	}}
	wantReset := ollamaCloudProbeReset(t, fixedNow)
	upstream := &ollamaUsageHTTPStub{body: ollamaCloudProbeUsageBody(100, wantReset.Format(time.RFC3339))}
	svc := ollamaCloudProbeFixture(t, repo, upstream, fixedNow)

	results := make(map[int64]time.Time)
	var mu sync.Mutex
	got := make(chan int64, 2)
	cb := func(id int64, reset time.Time) {
		mu.Lock()
		results[id] = reset
		mu.Unlock()
		got <- id
	}
	require.True(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(first.ID, cb))
	require.True(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(second.ID, cb))

	for range 2 {
		select {
		case <-got:
		case <-time.After(5 * time.Second):
			t.Fatal("both same-group accounts must each receive their own callback")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, results, first.ID)
	require.Contains(t, results, second.ID)
	for _, reset := range results {
		require.True(t, reset.Equal(wantReset))
	}
	require.Equal(t, int64(1), upstream.calls.Load(), "one shared group must fetch at most once for the coalesced probes")
}

func TestOllamaCloudUsageRateLimitProbeSkipsMissingCookieAndNonOllamaAccount(t *testing.T) {
	fixedNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	noCookie := ollamaUsageAccount(31)
	noCookie.Extra = map[string]any{} // no session
	nonOllama := ollamaUsageAccount(32)
	nonOllama.Credentials["base_url"] = "https://api.openai.com"
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{
		accounts: map[int64]*Account{noCookie.ID: noCookie, nonOllama.ID: nonOllama},
	}}
	upstream := &ollamaUsageHTTPStub{body: ollamaCloudProbeUsageBody(100, ollamaCloudProbeReset(t, fixedNow).Format(time.RFC3339))}
	svc := ollamaCloudProbeFixture(t, repo, upstream, fixedNow)

	called := make(chan struct{}, 1)
	cb := func(int64, time.Time) { called <- struct{}{} }
	require.True(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(noCookie.ID, cb))
	require.True(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(nonOllama.ID, cb))

	select {
	case <-called:
		t.Fatal("ineligible accounts must be skipped quietly")
	case <-time.After(300 * time.Millisecond):
	}
	require.Zero(t, upstream.calls.Load(), "no upstream fetch for ineligible accounts")
}

func TestOllamaCloudUsageRateLimitProbeRespectsFailureBackoff(t *testing.T) {
	fixedNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	account := ollamaUsageAccount(41)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	backoffUntil := fixedNow.Add(30 * time.Minute)
	account.Extra[OllamaCloudUsageSnapshotExtraKey] = &OllamaCloudUsageSnapshot{
		Status:        OllamaCloudUsageStatusFailed,
		LastAttemptAt: fixedNow.Add(-time.Minute),
		NextRefreshAt: backoffUntil,
		FailureCount:  2,
	}
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{
		accounts: map[int64]*Account{account.ID: account},
	}}
	upstream := &ollamaUsageHTTPStub{body: ollamaCloudProbeUsageBody(100, ollamaCloudProbeReset(t, fixedNow).Format(time.RFC3339))}
	svc := ollamaCloudProbeFixture(t, repo, upstream, fixedNow)

	called := make(chan struct{}, 1)
	require.True(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(account.ID, func(int64, time.Time) {
		called <- struct{}{}
	}))
	select {
	case <-called:
		t.Fatal("a model 429 must not bypass scrape-429 backoff")
	case <-time.After(300 * time.Millisecond):
	}
	require.Zero(t, upstream.calls.Load(), "no fetch while the snapshot is in failure backoff")
}

func TestOllamaCloudUsageRateLimitProbeStopCancelsInFlightAndRejectsNew(t *testing.T) {
	fixedNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	account := ollamaUsageAccount(51)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	base := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{
		accounts: map[int64]*Account{account.ID: account},
	}}
	repo := &ollamaCloudProbeGateRepo{ollamaUsageTestRepo: base, entered: make(chan struct{}), release: make(chan struct{})}
	repo.gating.Store(true)
	svc := ollamaCloudProbeFixture(t, repo, &ollamaUsageHTTPStub{body: ollamaCloudProbeUsageBody(100, ollamaCloudProbeReset(t, fixedNow).Format(time.RFC3339))}, fixedNow)

	called := make(chan struct{}, 1)
	require.True(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(account.ID, func(int64, time.Time) {
		called <- struct{}{}
	}))
	require.Eventually(t, func() bool {
		select {
		case <-repo.entered:
			return true
		default:
			return false
		}
	}, 5*time.Second, time.Millisecond, "coordinator must be blocked in the probe")

	// Stop cancels parentCtx; the in-flight probe's detached context is derived
	// from it, so the gated GetByID is aborted and Stop returns promptly.
	done := make(chan struct{})
	go func() { svc.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop must not hang while a probe is in flight")
	}
	select {
	case <-called:
		t.Fatal("no callback may be delivered for a probe cancelled by Stop")
	default:
	}

	// A stopped service rejects further probes and performs no further fetch.
	require.False(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(account.ID, func(int64, time.Time) {
		called <- struct{}{}
	}), "a stopped service must reject new probes")
}

// ollamaCloudProbeGateRepo blocks GetByID until released (or its context is
// cancelled), letting tests prove probes run asynchronously and that Stop aborts
// in-flight work.
type ollamaCloudProbeGateRepo struct {
	*ollamaUsageTestRepo
	entered chan struct{}
	release chan struct{}
	gating  atomic.Bool
	once    sync.Once
}

func (r *ollamaCloudProbeGateRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if r.gating.Load() {
		r.once.Do(func() { close(r.entered) })
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return r.ollamaUsageTestRepo.GetByID(ctx, id)
}

// TestOllamaCloudUsageRateLimitProbeSlowFetchDoesNotReportExpiredReset checks that
// an exhaustion decision is evaluated at fetch completion time: when the reset
// rolls over during a slow fetch, the probe must not report a horizon that is
// already in the past (which would otherwise trigger a needless long-lived ban).
func TestOllamaCloudUsageRateLimitProbeSlowFetchDoesNotReportExpiredReset(t *testing.T) {
	base := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	account := ollamaUsageAccount(61)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{
		accounts: map[int64]*Account{account.ID: account},
	}}

	var mu sync.Mutex
	cur := base
	reset := base.Add(time.Second) // in the future at start, expired by completion
	advanced := false
	upstream := &ollamaUsageHTTPStub{body: ollamaCloudProbeUsageBody(100, reset.Format(time.RFC3339)), beforeResponse: func(*http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !advanced {
			cur = base.Add(2 * time.Second) // fetch completes after the reset
			advanced = true
		}
	}}
	svc := NewOllamaCloudUsageService(repo, upstream, NewSettingService(&upstreamBillingProbeSettingRepo{}, nil), ollamaUsageTestEncryptor{}, true)
	svc.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return cur }
	svc.Start()
	t.Cleanup(svc.Stop)

	called := make(chan struct{}, 1)
	require.True(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(account.ID, func(int64, time.Time) {
		called <- struct{}{}
	}))
	select {
	case <-called:
		t.Fatal("a reset that already rolled over during the fetch must not be reported")
	case <-time.After(400 * time.Millisecond):
	}
	require.Equal(t, int64(1), upstream.calls.Load(), "the slow probe must still fetch once")

	// The snapshot really was exhausted; only the expired reset suppressed the
	// callback, which is what this test pins. Read it through the repo (which
	// clones under its lock) to avoid racing the coordinator's snapshot write.
	require.Eventually(t, func() bool {
		loaded, err := repo.GetByID(context.Background(), account.ID)
		if err != nil || loaded == nil {
			return false
		}
		snapshot := decodeOllamaCloudUsageSnapshot(loaded.Extra)
		return snapshot != nil && snapshot.Status == OllamaCloudUsageStatusOK &&
			snapshot.Data != nil && snapshot.Data.FiveHour != nil && snapshot.Data.FiveHour.UsedPercent == 100
	}, 5*time.Second, 5*time.Millisecond, "coordinator must persist the exhausted snapshot")
}

// TestOllamaCloudUsageRateLimitProbeQueueFullStillMergesExistingAccount verifies
// that a full queue still accepts (and merges into the latest callback) a probe
// for an account that is already queued, while a brand-new account is rejected.
func TestOllamaCloudUsageRateLimitProbeQueueFullStillMergesExistingAccount(t *testing.T) {
	base := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	repo := &ollamaCloudProbeGateRepo{ollamaUsageTestRepo: &ollamaUsageTestRepo{
		upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{}},
	}, entered: make(chan struct{}), release: make(chan struct{})}
	repo.gating.Store(true)
	svc := NewOllamaCloudUsageService(repo, &ollamaUsageHTTPStub{body: ollamaCloudProbeUsageBody(100, base.Add(time.Hour).Format(time.RFC3339))}, NewSettingService(&upstreamBillingProbeSettingRepo{}, nil), ollamaUsageTestEncryptor{}, true)
	svc.now = func() time.Time { return base }
	svc.Start()
	t.Cleanup(svc.Stop)

	noop := func(int64, time.Time) {}
	// The first probe blocks the single coordinator on its gated GetByID.
	require.True(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(9001, noop))
	require.Eventually(t, func() bool {
		select {
		case <-repo.entered:
			return true
		default:
			return false
		}
	}, 5*time.Second, time.Millisecond, "coordinator must be blocked")

	// Fill the pending queue to capacity (item 9001 is in flight, not pending).
	for id := int64(9002); id <= 9001+ollamaCloudUsageProbeMaxQueue; id++ {
		require.True(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(id, noop), "filling the queue must be accepted")
	}
	// Queue is now full: a brand-new account is rejected, but an already-queued
	// account is still merged into its latest callback.
	require.False(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(9999, noop), "a new account must be rejected when the queue is full")
	require.True(t, svc.ScheduleOllamaCloudUsageRateLimitProbe(9002, noop), "an already-queued account must still merge when the queue is full")
}

func probeSnap(status string, fetchedAt, lastAttempt time.Time) *OllamaCloudUsageSnapshot {
	s := &OllamaCloudUsageSnapshot{Status: status, LastAttemptAt: lastAttempt}
	if !fetchedAt.IsZero() {
		s.FetchedAt = &fetchedAt
	}
	return s
}

// TestOllamaCloudUsageProbeNewestSuccess pins the "most recent observation wins"
// rule: neither the account row nor the coordinator cache is preferred outright,
// an older success never masks a newer success, and a newer failure suppresses an
// older success so it cannot drive a stale exhaustion report.
func TestOllamaCloudUsageProbeNewestSuccess(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	window := ollamaCloudUsageManualRefreshInterval
	fresh := now.Add(-5 * time.Second)
	newer := now.Add(-time.Second)
	stale := now.Add(-time.Hour)
	failed := now.Add(-2 * time.Second) // LastAttemptAt for a failure
	entry := func(snapshot *OllamaCloudUsageSnapshot, at time.Time) ollamaCloudUsageProbeGroupEntry {
		return ollamaCloudUsageProbeGroupEntry{attemptAt: at, snapshot: snapshot}
	}

	tests := []struct {
		name            string
		accountSnapshot *OllamaCloudUsageSnapshot
		cached          ollamaCloudUsageProbeGroupEntry
		hasCached       bool
		wantFetchedAt   time.Time // zero means nil result
	}{
		{
			name:          "no snapshots",
			wantFetchedAt: time.Time{},
		},
		{
			name:            "cached newer success wins over account success",
			accountSnapshot: probeSnap(OllamaCloudUsageStatusOK, fresh, fresh),
			cached:          entry(probeSnap(OllamaCloudUsageStatusOK, newer, newer), newer),
			hasCached:       true,
			wantFetchedAt:   newer,
		},
		{
			name:            "account newer success wins over cached success",
			accountSnapshot: probeSnap(OllamaCloudUsageStatusOK, newer, newer),
			cached:          entry(probeSnap(OllamaCloudUsageStatusOK, fresh, fresh), fresh),
			hasCached:       true,
			wantFetchedAt:   newer,
		},
		{
			name:            "newer account failure suppresses older cached success",
			accountSnapshot: probeSnap(OllamaCloudUsageStatusFailed, time.Time{}, failed),
			cached:          entry(probeSnap(OllamaCloudUsageStatusOK, fresh, fresh), fresh),
			hasCached:       true,
			wantFetchedAt:   time.Time{},
		},
		{
			name:            "newer cached failure suppresses older account success",
			accountSnapshot: probeSnap(OllamaCloudUsageStatusOK, fresh, fresh),
			cached:          entry(probeSnap(OllamaCloudUsageStatusFailed, time.Time{}, failed), failed),
			hasCached:       true,
			wantFetchedAt:   time.Time{},
		},
		{
			name:            "stale success alone is not usable",
			accountSnapshot: probeSnap(OllamaCloudUsageStatusOK, stale, stale),
			wantFetchedAt:   time.Time{},
		},
		{
			name:            "only a fresh account success is usable",
			accountSnapshot: probeSnap(OllamaCloudUsageStatusOK, fresh, fresh),
			wantFetchedAt:   fresh,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ollamaCloudUsageProbeNewestSuccess(tt.accountSnapshot, tt.cached, tt.hasCached, now, window)
			if tt.wantFetchedAt.IsZero() {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.NotNil(t, got.FetchedAt)
			require.True(t, got.FetchedAt.Equal(tt.wantFetchedAt), "FetchedAt = %v, want %v", got.FetchedAt, tt.wantFetchedAt)
		})
	}
}

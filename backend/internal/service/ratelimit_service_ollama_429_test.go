//go:build unit

package service

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This file unit-tests the Ollama Cloud usage 429 branch added to
// RateLimitService.handle429 (see ratelimit_service_ollama_429.go): early
// platform diversion, immediate never-shrink cooldown (Retry-After vs seconds
// fallback), scheduling rejection / scheduler-absent runtime notification, and,
// for the async probe callback, that a stale result can never overwrite a newer
// 429, admin clear, key swap, disabled account, or a shorter cooldown floor,
// guarded atomically via the repository CAS.

func ollama429Account(id int64, platform string) *Account {
	return &Account{
		ID:          id,
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"base_url": "https://www.ollama.com",
			"api_key":  "ollama429-key",
		},
	}
}

// ollama429SchedulerStub records Schedule calls so tests can drive the recorded
// probe callback (as the background coordinator would) deterministically.
type ollama429SchedulerStub struct {
	mu        sync.Mutex
	accept    bool
	scheduled []int64
	callbacks map[int64]OllamaCloudUsageRateLimitProbeCallback
}

func newOllama429SchedulerStub(accept bool) *ollama429SchedulerStub {
	return &ollama429SchedulerStub{accept: accept, callbacks: map[int64]OllamaCloudUsageRateLimitProbeCallback{}}
}

func (p *ollama429SchedulerStub) ScheduleOllamaCloudUsageRateLimitProbe(accountID int64, cb OllamaCloudUsageRateLimitProbeCallback) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.accept {
		return false
	}
	p.scheduled = append(p.scheduled, accountID)
	p.callbacks[accountID] = cb // latest wins, mirroring the coordinator coalescing
	return true
}

func (p *ollama429SchedulerStub) fire(accountID int64, resetAt time.Time) {
	p.mu.Lock()
	cb := p.callbacks[accountID]
	p.mu.Unlock()
	if cb != nil {
		cb(accountID, resetAt)
	}
}

func (p *ollama429SchedulerStub) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.scheduled)
}

// ollama429BlockRec records an in-memory runtime scheduling block.
type ollama429BlockRec struct {
	accountID int64
	until     time.Time
	reason    string
}

type ollama429BlockerStub struct {
	mu     sync.Mutex
	blocks []ollama429BlockRec
}

func (b *ollama429BlockerStub) BlockAccountScheduling(account *Account, until time.Time, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if account != nil {
		b.blocks = append(b.blocks, ollama429BlockRec{accountID: account.ID, until: until, reason: reason})
	}
}

func (b *ollama429BlockerStub) ClearAccountSchedulingBlock(int64) {}

func (b *ollama429BlockerStub) last() (ollama429BlockRec, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.blocks) == 0 {
		return ollama429BlockRec{}, false
	}
	return b.blocks[len(b.blocks)-1], true
}

// ollama429Repo is a controllable AccountRepository fake implementing the narrow
// IfLater + generation-CAS contracts the production *accountRepository provides.
// updatedAt is simulated by bumping Account.UpdatedAt forward on every write.
type ollama429Repo struct {
	mockAccountRepoForGemini
	mu             sync.Mutex
	accounts       map[int64]*Account
	casUpdated     int
	casSkipped     int
	ifLaterWrites  []time.Time
	uncondWrites   []time.Time
	versionCounter int64
}

func newOllama429Repo(acct *Account) *ollama429Repo {
	r := &ollama429Repo{accounts: map[int64]*Account{}}
	r.accounts[acct.ID] = acct
	return r
}

func (r *ollama429Repo) bump(acct *Account) {
	r.versionCounter++
	acct.UpdatedAt = time.Unix(0, r.versionCounter).UTC()
}

func (r *ollama429Repo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.accounts[id], nil
}

func (r *ollama429Repo) currentReset(id int64) *time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.accounts[id]
	if a == nil || a.RateLimitResetAt == nil {
		return nil
	}
	v := *a.RateLimitResetAt
	return &v
}

// mutate lets a test simulate an external change (admin clear / re-arm / key swap).
func (r *ollama429Repo) mutate(id int64, fn func(*Account)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.accounts[id]; ok {
		fn(a)
		r.bump(a)
	}
}

// bumpVersion simulates the real probe persisting its usage snapshot: it advances
// the row's UpdatedAt (TimeMixin bump) without touching rate-limit fields, which
// is exactly what updateSnapshot does between scheduling and callback delivery.
func (r *ollama429Repo) bumpVersion(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.accounts[id]; ok {
		r.bump(a)
	}
}

// SetRateLimitedIfLater implements the never-shrink narrow extender.
func (r *ollama429Repo) SetRateLimitedIfLater(_ context.Context, id int64, resetAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.accounts[id]
	if a == nil {
		return nil
	}
	cur := a.RateLimitResetAt
	if cur == nil || resetAt.After(*cur) {
		a.RateLimitResetAt = ollama429TimePtr(resetAt)
		now := time.Now()
		a.RateLimitedAt = &now
		r.bump(a)
		r.ifLaterWrites = append(r.ifLaterWrites, resetAt)
	}
	return nil
}

func (r *ollama429Repo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.accounts[id]
	if a == nil {
		return nil
	}
	a.RateLimitResetAt = ollama429TimePtr(resetAt)
	now := time.Now()
	a.RateLimitedAt = &now
	r.bump(a)
	r.uncondWrites = append(r.uncondWrites, resetAt)
	return nil
}

func (r *ollama429Repo) SetRateLimitedIfUnchanged(
	_ context.Context, id int64,
	expectedUpdatedAt time.Time,
	expectedLimitedAt, expectedResetAt *time.Time,
	newResetAt time.Time,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.accounts[id]
	if a == nil {
		r.casSkipped++
		return false, nil
	}
	limitedMatch := timePtrEqual(expectedLimitedAt, a.RateLimitedAt)
	resetMatch := timePtrEqual(expectedResetAt, a.RateLimitResetAt)
	if !a.UpdatedAt.Equal(expectedUpdatedAt) || !limitedMatch || !resetMatch {
		r.casSkipped++
		return false, nil
	}
	a.RateLimitedAt = ollama429TimePtr(time.Now())
	a.RateLimitResetAt = ollama429TimePtr(newResetAt)
	r.bump(a)
	r.casUpdated++
	return true, nil
}

func ollama429TimePtr(t time.Time) *time.Time { return &t }

func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// ollama429Fixture builds a started-ish RateLimitService over the given repo and
// scheduler. runtime blocker is injected so scheduling notifications are visible.
func ollama429Fixture(t *testing.T, repo *ollama429Repo, scheduler *ollama429SchedulerStub) (*RateLimitService, *ollama429BlockerStub) {
	t.Helper()
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	blocker := &ollama429BlockerStub{}
	svc.SetAccountRuntimeBlocker(blocker)
	svc.SetOllamaCloudUsageProbeScheduler(scheduler)
	return svc, blocker
}

func TestHandle429_OllamaEarlyBranchAcrossPlatforms(t *testing.T) {
	platforms := []string{PlatformOpenAI, PlatformAnthropic, PlatformKimi, PlatformZhipu, PlatformDeepseek}
	for _, platform := range platforms {
		t.Run(platform, func(t *testing.T) {
			acct := ollama429Account(101, platform)
			repo := newOllama429Repo(acct)
			scheduler := newOllama429SchedulerStub(true)
			svc, _ := ollama429Fixture(t, repo, scheduler)

			svc.handle429(context.Background(), acct, http.Header{}, nil)

			require.Equal(t, 1, scheduler.count(), "real-Ollama %s account must schedule a probe", platform)
			require.Greater(t, len(repo.ifLaterWrites), 0, "immediate never-shrink cooldown must be applied")
		})
	}
}

func TestHandle429_NonOllamaUnchanged(t *testing.T) {
	acct := &Account{ID: 202, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://api.anthropic.com", "api_key": "k"}}
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(true)
	svc, _ := ollama429Fixture(t, repo, scheduler)

	svc.handle429(context.Background(), acct, http.Header{}, nil)

	require.Zero(t, scheduler.count(), "non-Ollama account must not schedule an Ollama probe")
	require.Zero(t, repo.casUpdated)
}

func TestHandle429_OllamaUsesValidRetryAfter(t *testing.T) {
	acct := ollama429Account(301, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(true)
	svc, _ := ollama429Fixture(t, repo, scheduler)

	before := time.Now()
	svc.handle429(context.Background(), acct, http.Header{"Retry-After": []string{"120"}}, nil)
	after := time.Now()

	require.Equal(t, 1, scheduler.count())
	reset := repo.currentReset(acct.ID)
	require.NotNil(t, reset)
	require.False(t, reset.Before(before.Add(120*time.Second)), "reset %v < now+120s", reset)
	require.False(t, reset.After(after.Add(120*time.Second)), "reset %v > now+120s", reset)
}

func TestHandle429_OllamaFallsBackToSecondsCooldown(t *testing.T) {
	acct := ollama429Account(302, PlatformAnthropic)
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(true)
	svc, _ := ollama429Fixture(t, repo, scheduler)

	before := time.Now()
	svc.handle429(context.Background(), acct, http.Header{}, nil) // no Retry-After
	after := time.Now()

	require.Equal(t, 1, scheduler.count())
	reset := repo.currentReset(acct.ID)
	require.NotNil(t, reset)
	def := defaultRateLimit429CooldownSeconds
	require.False(t, reset.Before(before.Add(time.Duration(def)*time.Second)))
	require.False(t, reset.After(after.Add(time.Duration(def)*time.Second)))
}

func TestHandle429_OllamaScheduleRejectedStillCooled(t *testing.T) {
	acct := ollama429Account(303, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(false) // queue full / service stopped
	svc, _ := ollama429Fixture(t, repo, scheduler)

	svc.handle429(context.Background(), acct, http.Header{}, nil)

	require.Zero(t, scheduler.count())
	require.Greater(t, len(repo.ifLaterWrites), 0, "immediate cooldown must still be applied when scheduling is rejected")
}

func TestHandle429_OllamaDoesNotShrinkConfirmedLongCooldown(t *testing.T) {
	acct := ollama429Account(304, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	long := time.Now().Add(time.Hour)
	repo.mutate(acct.ID, func(a *Account) { a.RateLimitResetAt = &long })
	scheduler := newOllama429SchedulerStub(true)
	svc, _ := ollama429Fixture(t, repo, scheduler)

	// A 429 with no useful Retry-After arrives while a +1h reset is already set;
	// the 5s fallback must not shrink it.
	svc.handle429(context.Background(), acct, http.Header{}, nil)

	reset := repo.currentReset(acct.ID)
	require.NotNil(t, reset)
	require.GreaterOrEqual(t, reset.Unix(), long.Unix(), "already-confirmed long cooldown must not be shrunk to fallback seconds")
	require.Equal(t, 1, scheduler.count())
}

func TestOllamaProbeCallback_ValidUpdatesDBAndMemory(t *testing.T) {
	acct := ollama429Account(401, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(true)
	svc, blocker := ollama429Fixture(t, repo, scheduler)

	svc.handle429(context.Background(), acct, http.Header{}, nil) // schedules; immediate 5s
	require.Equal(t, 1, scheduler.count())

	probeReset := time.Now().Add(2 * time.Hour)
	scheduler.fire(acct.ID, probeReset)

	require.Equal(t, 1, repo.casUpdated, "valid callback must update DB once")
	require.Zero(t, repo.casSkipped)
	reset := repo.currentReset(acct.ID)
	require.NotNil(t, reset)
	require.True(t, reset.Equal(probeReset), "DB reset = %v, want %v", reset, probeReset)
	rec, ok := blocker.last()
	require.True(t, ok)
	require.Equal(t, "ollama_cloud_usage_429_probe", rec.reason)
	require.True(t, rec.until.Equal(probeReset))
}

func TestOllamaProbeCallback_DuplicateNoDoubleUpdate(t *testing.T) {
	acct := ollama429Account(402, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(true)
	svc, blocker := ollama429Fixture(t, repo, scheduler)

	svc.handle429(context.Background(), acct, http.Header{}, nil)
	probeReset := time.Now().Add(2 * time.Hour)
	scheduler.fire(acct.ID, probeReset)
	require.Equal(t, 1, repo.casUpdated)

	// Same generation fired again (a duplicate/out-of-order delivery): the CAS
	// generation has moved on, so no second update and no second notification.
	scheduler.fire(acct.ID, probeReset.Add(time.Hour))
	require.Equal(t, 1, repo.casUpdated)
	rec, _ := blocker.last()
	require.Equal(t, "ollama_cloud_usage_429_probe", rec.reason)
	require.True(t, rec.until.Equal(probeReset))
}

func TestOllamaProbeCallback_StaleLongDoesNotOverrideNewShort(t *testing.T) {
	acct := ollama429Account(403, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(true)
	svc, blocker := ollama429Fixture(t, repo, scheduler)

	svc.handle429(context.Background(), acct, http.Header{}, nil) // schedules gen @5s
	require.Equal(t, 1, scheduler.count())

	// An admin / newer policy re-arms the account to a fresh SHORT cooldown.
	newShort := time.Now().Add(5 * time.Second)
	repo.mutate(acct.ID, func(a *Account) { a.RateLimitResetAt = ollama429TimePtr(newShort) })

	// The old async result reports a long 7d reset; it must not override.
	oldLong := time.Now().Add(7 * 24 * time.Hour)
	scheduler.fire(acct.ID, oldLong)

	require.Zero(t, repo.casUpdated, "stale long callback must not pass the CAS")
	require.Greater(t, repo.casSkipped, 0)
	reset := repo.currentReset(acct.ID)
	require.NotNil(t, reset)
	require.True(t, reset.Equal(newShort), "current reset must remain the new short %v, got %v", newShort, reset)
	rec, ok := blocker.last()
	// The only scheduling notification should be the immediate cooldown one, not
	// an ollama_cloud_usage_429_probe block for the stale 7d reset.
	if ok && rec.reason == "ollama_cloud_usage_429_probe" {
		t.Fatalf("stale long callback must not announce scheduling; got block to %v", rec.until)
	}
}

func TestOllamaProbeCallback_AdminClearSkips(t *testing.T) {
	acct := ollama429Account(404, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(true)
	svc, blocker := ollama429Fixture(t, repo, scheduler)

	svc.handle429(context.Background(), acct, http.Header{}, nil)
	// Admin clears the rate limit entirely.
	repo.mutate(acct.ID, func(a *Account) {
		a.RateLimitedAt = nil
		a.RateLimitResetAt = nil
	})

	scheduler.fire(acct.ID, time.Now().Add(2*time.Hour))
	require.Zero(t, repo.casUpdated, "cleared account must not be re-limited by a stale callback")
	require.Nil(t, repo.currentReset(acct.ID))
	rec, ok := blocker.last()
	if ok && rec.reason == "ollama_cloud_usage_429_probe" {
		t.Fatalf("cleared account must not be re-blocked by a stale callback")
	}
}

func TestOllamaProbeCallback_KeySwapSkips(t *testing.T) {
	acct := ollama429Account(405, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(true)
	svc, _ := ollama429Fixture(t, repo, scheduler)

	svc.handle429(context.Background(), acct, http.Header{}, nil) // captures fingerprint of k1
	// The api_key is swapped to another key (still on ollama.com).
	repo.mutate(acct.ID, func(a *Account) { a.Credentials["api_key"] = "ollama429-key-2" })

	scheduler.fire(acct.ID, time.Now().Add(2*time.Hour))
	require.Zero(t, repo.casUpdated, "key-swapped account must not be limited by the old key's probe")
	// The fingerprint guard returns before any CAS is attempted, so the current
	// reset stays at the immediate cooldown, not the probe's far-future reset.
	reset := repo.currentReset(acct.ID)
	require.NotNil(t, reset)
	require.False(t, reset.After(time.Now().Add(time.Hour)), "key-swapped account must keep only its immediate cooldown, got %v", reset)
}

func TestOllamaProbeCallback_ResetAlreadyPastSkips(t *testing.T) {
	acct := ollama429Account(406, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(true)
	svc, _ := ollama429Fixture(t, repo, scheduler)

	svc.handle429(context.Background(), acct, http.Header{}, nil)
	repo.mutate(acct.ID, func(a *Account) {
		// keep same reset/state, just ensure callback reset is in the past
	})
	scheduler.fire(acct.ID, time.Now().Add(-time.Minute))

	require.Zero(t, repo.casUpdated)
	require.Zero(t, repo.casSkipped, "an already-passed reset is dropped before any CAS is attempted")
}

func TestOllamaProbeCallback_SnapshotPersistBumpStillApplies(t *testing.T) {
	// The real probe persists its usage snapshot between scheduling and callback
	// delivery, which advances the row's UpdatedAt (TimeMixin) without touching
	// the rate-limit generation. The write-back must still succeed: it binds the
	// CAS to the version re-read in the callback, not the scheduling-time one.
	acct := ollama429Account(501, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(true)
	svc, blocker := ollama429Fixture(t, repo, scheduler)

	svc.handle429(context.Background(), acct, http.Header{}, nil)
	require.Equal(t, 1, scheduler.count())

	repo.bumpVersion(acct.ID) // simulate updateSnapshot persisting the snapshot

	probeReset := time.Now().Add(2 * time.Hour)
	scheduler.fire(acct.ID, probeReset)

	require.Equal(t, 1, repo.casUpdated, "snapshot persist bumping UpdatedAt must not fail the exhaustion CAS")
	require.Zero(t, repo.casSkipped)
	reset := repo.currentReset(acct.ID)
	require.NotNil(t, reset)
	require.True(t, reset.Equal(probeReset), "DB reset = %v, want %v", reset, probeReset)
	rec, ok := blocker.last()
	require.True(t, ok)
	require.Equal(t, "ollama_cloud_usage_429_probe", rec.reason)
}

func TestOllamaProbeCallback_NeverShortensCooldownFloor(t *testing.T) {
	// An explicit Retry-After sets a 120s floor. A later async probe that reports
	// a shorter (still-future) reset must not shrink that floor back down.
	acct := ollama429Account(502, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(true)
	svc, _ := ollama429Fixture(t, repo, scheduler)

	before := time.Now()
	svc.handle429(context.Background(), acct, http.Header{"Retry-After": []string{"120"}}, nil)
	after := time.Now()
	floor := repo.currentReset(acct.ID)
	require.NotNil(t, floor)
	require.False(t, floor.Before(before.Add(120*time.Second)))
	require.False(t, floor.After(after.Add(120*time.Second)))

	// Probe reports a reset still in the future but sooner than the 120s floor.
	scheduler.fire(acct.ID, time.Now().Add(30*time.Second))

	require.Zero(t, repo.casUpdated, "a probe reset sooner than the explicit floor must be dropped")
	still := repo.currentReset(acct.ID)
	require.True(t, still.Equal(*floor), "the explicit Retry-After floor must be preserved")
}

func TestOllamaProbeCallback_DisabledSkips(t *testing.T) {
	acct := ollama429Account(503, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	scheduler := newOllama429SchedulerStub(true)
	svc, _ := ollama429Fixture(t, repo, scheduler)

	svc.handle429(context.Background(), acct, http.Header{}, nil)
	repo.mutate(acct.ID, func(a *Account) { a.Status = StatusDisabled })

	scheduler.fire(acct.ID, time.Now().Add(2*time.Hour))
	require.Zero(t, repo.casUpdated, "a disabled account must not be re-limited by a stale callback")
}

func TestHandle429_OllamaSchedulerAbsentStillNotifiesRuntime(t *testing.T) {
	acct := ollama429Account(504, PlatformOpenAI)
	repo := newOllama429Repo(acct)
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	blocker := &ollama429BlockerStub{}
	svc.SetAccountRuntimeBlocker(blocker)
	// No probe scheduler injected.

	before := time.Now()
	svc.handle429(context.Background(), acct, http.Header{}, nil)
	after := time.Now()

	// Immediate cooldown applied and runtime scheduling announced even without a
	// scheduler; only the async probe is skipped.
	require.Greater(t, len(repo.ifLaterWrites), 0)
	rec, ok := blocker.last()
	require.True(t, ok)
	require.Equal(t, "ollama_429", rec.reason)
	require.False(t, rec.until.Before(before.Add(defaultRateLimit429CooldownSeconds*time.Second)))
	require.False(t, rec.until.After(after.Add(defaultRateLimit429CooldownSeconds*time.Second)))
}

// ollama429LinkRepo drives a real probe->callback linkage through the actual
// OllamaCloudUsageService. It wraps the shared ollama feature test repo, adds the
// narrow IfLater/CAS surface RateLimitService needs, and makes the real snapshot
// persist (UpdateOllamaCloudUsageSnapshot) advance the row's UpdatedAt exactly as
// the production TimeMixin-backed repository does.
type ollama429LinkRepo struct {
	*ollamaUsageTestRepo
	mu             sync.Mutex
	versionCounter int64
	casUpdated     int
	linkIfLater    []time.Time
}

func (r *ollama429LinkRepo) bump(a *Account) {
	r.versionCounter++
	a.UpdatedAt = time.Unix(0, r.versionCounter).UTC()
}

func (r *ollama429LinkRepo) UpdateOllamaCloudUsageSnapshot(ctx context.Context, expected *Account, snapshot *OllamaCloudUsageSnapshot) error {
	if err := r.ollamaUsageTestRepo.UpdateOllamaCloudUsageSnapshot(ctx, expected, snapshot); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.upstreamBillingProbeAccountRepo.accounts[expected.ID]; ok {
		r.bump(a)
	}
	return nil
}

func (r *ollama429LinkRepo) SetRateLimitedIfLater(_ context.Context, id int64, resetAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.upstreamBillingProbeAccountRepo.accounts[id]
	if a == nil {
		return nil
	}
	cur := a.RateLimitResetAt
	if cur == nil || resetAt.After(*cur) {
		a.RateLimitResetAt = ollama429TimePtr(resetAt)
		now := time.Now()
		a.RateLimitedAt = &now
		r.bump(a)
		r.linkIfLater = append(r.linkIfLater, resetAt)
	}
	return nil
}

func (r *ollama429LinkRepo) SetRateLimitedIfUnchanged(
	_ context.Context, id int64,
	expectedUpdatedAt time.Time,
	expectedLimitedAt, expectedResetAt *time.Time,
	newResetAt time.Time,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.upstreamBillingProbeAccountRepo.accounts[id]
	if a == nil {
		return false, nil
	}
	if !a.UpdatedAt.Equal(expectedUpdatedAt) ||
		!timePtrEqual(expectedLimitedAt, a.RateLimitedAt) ||
		!timePtrEqual(expectedResetAt, a.RateLimitResetAt) {
		return false, nil
	}
	a.RateLimitedAt = ollama429TimePtr(time.Now())
	a.RateLimitResetAt = ollama429TimePtr(newResetAt)
	r.bump(a)
	r.casUpdated++
	return true, nil
}

func (r *ollama429LinkRepo) currentReset(id int64) *time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.upstreamBillingProbeAccountRepo.accounts[id]
	if a == nil || a.RateLimitResetAt == nil {
		return nil
	}
	v := *a.RateLimitResetAt
	return &v
}

func (r *ollama429LinkRepo) casUpdatedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.casUpdated
}

// TestOllama429RealProbeLinkage_SnapshotPersistThenWriteBack drives the full real
// chain: handle429 schedules the async probe on a real started
// OllamaCloudUsageService; the probe fetches an exhausted settings page, persists
// its snapshot (which advances UpdatedAt through the link repo, mirroring the
// production TimeMixin bump), and the exhaustion callback then writes the reset
// back through the generation CAS. The CAS must still succeed even though the
// snapshot persist moved UpdatedAt, because it is bound to the version re-read at
// callback time, not the scheduling-time version.
func TestOllama429RealProbeLinkage_SnapshotPersistThenWriteBack(t *testing.T) {
	account := ollamaUsageAccount(701)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	// The probe is a 429-event recovery query and must run even when the periodic
	// auto_refresh switch is off (it still needs the configured cookie/session).
	account.Extra[OllamaCloudUsageAutoRefreshExtraKey] = false

	reset := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	body := []byte(`
		<section>
			<div><span>5 hour usage</span><span>100% used</span></div>
			<time datetime="` + reset + `"></time>
		</section>`)

	repo := &ollama429LinkRepo{ollamaUsageTestRepo: &ollamaUsageTestRepo{
		upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{
			accounts: map[int64]*Account{account.ID: account},
		},
	}}
	upstream := &ollamaUsageHTTPStub{body: body}

	usageSvc := NewOllamaCloudUsageService(repo, upstream, NewSettingService(&upstreamBillingProbeSettingRepo{}, nil), ollamaUsageTestEncryptor{}, true)

	// With auto_refresh off, a requireEnabled refresh is a (nil,nil) no-op. When
	// the 429-event probe piggybacks such a timed-cycle no-op through the shared
	// singleflight, runOllamaCloudUsageProbe must not record it as a real attempt.
	noop, noopErr := usageSvc.refreshAccount(context.Background(), account.ID, defaultOllamaCloudUsageSettings(), true)
	require.NoError(t, noopErr)
	require.Nil(t, noop, "auto_refresh-disabled refresh is a nil,nil no-op")

	usageSvc.Start()
	t.Cleanup(usageSvc.Stop)

	blocker := &ollama429BlockerStub{}
	rlSvc := NewRateLimitService(repo, nil, nil, nil, nil)
	rlSvc.SetAccountRuntimeBlocker(blocker)
	rlSvc.SetOllamaCloudUsageProbeScheduler(usageSvc)

	rlSvc.handle429(context.Background(), account, http.Header{}, nil)
	require.Greater(t, len(repo.linkIfLater), 0, "immediate cooldown must be applied")

	// The real probe coordinator (async) fetches, persists (bumping UpdatedAt),
	// reports exhaustion, and the write-back must then succeed. Reads go through
	// lock-protected accessors because the coordinator mutates the shared repo.
	require.Eventually(t, func() bool {
		if repo.casUpdatedCount() != 1 {
			return false
		}
		rec, ok := blocker.last()
		return ok && rec.reason == "ollama_cloud_usage_429_probe"
	}, 10*time.Second, 5*time.Millisecond, "exhaustion write-back through the real probe must update once and notify")

	// The 429-event probe actually performed a usage fetch despite auto_refresh=off.
	require.Greater(t, upstream.calls.Load(), int64(0), "probe must query usage even with auto_refresh disabled")

	rec, ok := blocker.last()
	require.True(t, ok)
	require.Equal(t, "ollama_cloud_usage_429_probe", rec.reason)
	require.True(t, rec.until.After(time.Now()), "probe reset must be in the future")
	resetAfter := repo.currentReset(account.ID)
	require.NotNil(t, resetAfter)
	require.True(t, resetAfter.After(time.Now()), "DB reset must be extended by the real probe write-back")
}

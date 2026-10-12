package service

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func transientPoolAccount(id int64) Account {
	return Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 100,
		Credentials: map[string]any{"pool_mode": true}}
}

func TestOpenAIAccountTransient_BackoffAndRecovery(t *testing.T) {
	state := newOpenAIAccountTransientState()
	now := time.Now()
	for _, duration := range []time.Duration{10 * time.Second, 45 * time.Second, 2 * time.Minute, 5 * time.Minute, 15 * time.Minute, 15 * time.Minute} {
		decision := state.recordFailure(42, now)
		require.Equal(t, duration, decision.Cooldown)
		require.True(t, state.isBlocked(42, now.Add(time.Second)))
		for i := range 100 {
			duplicate := state.recordFailure(42, now.Add(time.Duration(i)*time.Millisecond))
			require.Equal(t, decision.FailureStreak, duplicate.FailureStreak)
			require.Equal(t, decision.BlockUntil, duplicate.BlockUntil)
		}
		state.recordSuccess(42, now.Add(time.Second))
		require.True(t, state.isBlocked(42, now.Add(time.Second)), "late success must not cancel a new cooldown")
		now = decision.BlockUntil
	}
	blocked, probe := state.reserveProbe(42, now)
	require.False(t, blocked)
	require.NotZero(t, probe)
	require.False(t, state.recordSuccess(42, now.Add(time.Millisecond)))
	require.True(t, state.recordSuccess(42, now.Add(time.Millisecond), probe))
	require.True(t, state.hasEntry(42, now.Add(time.Millisecond)))
	_, next := state.reserveProbe(42, now.Add(time.Millisecond))
	state.recordSuccess(42, now.Add(2*time.Millisecond), next)
	require.False(t, state.hasEntry(42, now.Add(time.Millisecond)))
	state.releaseProbe(42, probe)
	require.Equal(t, 10*time.Second, state.recordFailure(42, now.Add(time.Second)).Cooldown)
}

func TestOpenAIAccountTransient_ConcurrentProbeAndStaleRelease(t *testing.T) {
	state := newOpenAIAccountTransientState()
	now := time.Now()
	state.recordFailure(42, now)
	now = now.Add(10 * time.Second)
	var winners atomic.Int32
	var winningProbe atomic.Uint64
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			blocked, probe := state.reserveProbe(42, now)
			if !blocked && probe != 0 {
				winners.Add(1)
				winningProbe.Store(probe)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), winners.Load())
	failure := state.recordFailure(42, now.Add(time.Second))
	blocked, nextProbe := state.reserveProbe(42, failure.BlockUntil)
	require.False(t, blocked)
	require.NotZero(t, nextProbe)
	state.releaseProbe(42, winningProbe.Load())
	require.True(t, state.isBlocked(42, failure.BlockUntil), "old release cannot open a newer probe")
	state.releaseProbe(42, nextProbe)
	require.False(t, state.isBlocked(42, failure.BlockUntil))
}

func TestOpenAIAccountTransient_ExpiresAbandonedState(t *testing.T) {
	state := newOpenAIAccountTransientState()
	now := time.Now()
	state.recordFailure(42, now)
	require.False(t, state.isBlocked(42, now.Add(openAIAccountTransientStreakTTL+time.Second)))
	require.Equal(t, 1, state.recordFailure(42, now.Add(time.Hour)).FailureStreak)
}

func TestOpenAIAccountTransient_SelectionProbeReleaseAndWait(t *testing.T) {
	svc := &OpenAIGatewayService{}
	state := svc.getOpenAIAccountTransientState()
	account := transientPoolAccount(42)
	state.recordFailure(42, time.Now().Add(-20*time.Second))
	releases := 0
	selection, ok := svc.finalizeOpenAIAccountTransientSelection(&AccountSelectionResult{
		Account: &account, Acquired: true, ReleaseFunc: func() { releases++ },
	})
	require.True(t, ok)
	require.True(t, state.isBlocked(42, time.Now()))
	_, ok = svc.finalizeOpenAIAccountTransientSelection(&AccountSelectionResult{Account: &account, Acquired: true})
	require.False(t, ok)
	_, ok = svc.finalizeOpenAIAccountTransientSelection(&AccountSelectionResult{Account: &account, WaitPlan: &AccountWaitPlan{AccountID: 42}})
	require.False(t, ok)
	selection.CompleteTransient(true)
	selection.ReleaseFunc()
	selection.ReleaseFunc()
	require.Equal(t, 1, releases)
	svc.ReportOpenAIAccountScheduleResult(&account, "gpt-5.1", true, nil)
	require.False(t, state.hasEntry(42, time.Now()), "the configured account circuit recovers after one admitted successful probe")
	selection, ok = svc.finalizeOpenAIAccountTransientSelection(&AccountSelectionResult{Account: &account, Acquired: true})
	require.True(t, ok)
	selection.CompleteTransient(true)
	selection.ReleaseFunc()
	require.False(t, state.hasEntry(42, time.Now()))
}

func TestOpenAIAccountTransient_OnlyPoolTransportAndServerFailures(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500, 502, 503, 504, 529} {
		svc := &OpenAIGatewayService{}
		account := transientPoolAccount(42)
		svc.recordOpenAIAccountTransientFailure(&account, status, []byte("upstream failed"), time.Now())
		require.Equal(t, status >= 500 && status != 529, svc.isOpenAIAccountTransientBlocked(&account))
	}
	for _, platform := range []string{PlatformOpenAI, PlatformGrok, PlatformAnthropic} {
		for _, pool := range []bool{true, false} {
			svc := &OpenAIGatewayService{}
			account := transientPoolAccount(42)
			account.Platform = platform
			account.Credentials["pool_mode"] = pool
			svc.recordOpenAIAccountTransientFailure(&account, 502, nil, time.Now())
			require.Equal(t, platform == PlatformOpenAI && pool, svc.isOpenAIAccountTransientBlocked(&account))
		}
	}
	for _, cause := range []error{nil, context.Canceled, bufio.ErrTooLong, ErrUpstreamResponseBodyTooLarge, io.ErrUnexpectedEOF, &net.OpError{Op: "dial", Err: io.EOF}} {
		svc := &OpenAIGatewayService{}
		account := transientPoolAccount(42)
		svc.recordOpenAIAvailabilityTransportFailure(context.Background(), nil, &account, "model", cause)
		_, isDial := cause.(*net.OpError)
		require.Equal(t, isDial, svc.isOpenAIAccountTransientBlocked(&account))
		if cause == io.ErrUnexpectedEOF {
			require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(&account, "model"))
		}
	}
}

func TestOpenAIAccountTransient_HTTPFailureTripsPool(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, &config.Config{}, nil, nil)
	account := transientPoolAccount(42)
	require.False(t, svc.handleOpenAIAccountUpstreamError(context.Background(), &account, http.StatusBadGateway, http.Header{}, []byte("upstream failed"), "gpt-5.1"))
	require.False(t, svc.isOpenAIAccountTransientBlocked(&account), "one model failure must not block other models")
	require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(&account, "gpt-5.1"))
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(&account), "no permanent account disable")
	snapshot := svc.SnapshotOpenAIAccountSchedulerState()
	found := false
	for _, item := range snapshot.ModelCooldowns {
		if item.AccountID == account.ID && item.Model == "gpt-5.1" {
			found = true
			require.Positive(t, item.RemainingSecs)
		}
	}
	require.True(t, found, "account cooldown remains visible in the existing scheduler monitor")
}

func TestOpenAIAccountTransient_QueuedRequestRechecksAndReleases(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := transientPoolAccount(42)
	svc.recordOpenAIAccountTransientFailure(&account, 502, nil, time.Now())
	releases := 0
	release, admitted := svc.AdmitOpenAIAccountAfterWait(&account, func() { releases++ })
	require.False(t, admitted)
	require.Nil(t, release)
	require.Equal(t, 1, releases)
	require.True(t, svc.isOpenAIAccountTransientBlocked(&account))
}

func TestOpenAIAccountTransient_SchedulerSwitchesAndRecovers(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		for _, loadBatch := range []bool{false, true} {
			t.Run(map[bool]string{true: "advanced", false: "legacy"}[advanced]+map[bool]string{true: "-batch", false: "-direct"}[loadBatch], func(t *testing.T) {
				resetOpenAIAdvancedSchedulerSettingCacheForTest()
				defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
				groupID := int64(901)
				accounts := []Account{transientPoolAccount(41), transientPoolAccount(42)}
				for i := range accounts {
					accounts[i].GroupIDs = []int64{groupID}
				}
				cfg := newSchedulerTestSubscriptionPriorityConfig()
				cfg.Gateway.Scheduling.LoadBatchEnabled = loadBatch
				svc := &OpenAIGatewayService{
					accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
					cache:       &schedulerTestGatewayCache{}, cfg: cfg,
					concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
					rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(map[bool]string{true: "true", false: "false"}[advanced]),
				}
				selectAccount := func() (*AccountSelectionResult, error) {
					selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "pool-recovery", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
					return selection, err
				}
				state := svc.getOpenAIAccountTransientState()
				state.recordFailure(41, time.Now())
				selection, err := selectAccount()
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.Equal(t, int64(42), selection.Account.ID)
				selection.ReleaseFunc()
				state.recordFailure(42, time.Now())
				selection, err = selectAccount()
				require.Error(t, err)
				require.Nil(t, selection)
				// Advance the failed account to its recovery window without sleeps.
				state.mu.Lock()
				entry := state.entries[41]
				entry.blockedUntil = time.Now().Add(-time.Second)
				state.entries[41] = entry
				state.mu.Unlock()
				selection, err = selectAccount()
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.Equal(t, int64(41), selection.Account.ID)
				next, nextErr := selectAccount()
				require.Error(t, nextErr)
				require.Nil(t, next)
				selection.CompleteTransient(true)
				selection.ReleaseFunc()
				svc.ReportOpenAIAccountScheduleResult(selection.Account, "gpt-5.1", true, nil)
				next, nextErr = selectAccount()
				require.NoError(t, nextErr)
				require.NotNil(t, next)
				next.ReleaseFunc()
			})
		}
	}
}

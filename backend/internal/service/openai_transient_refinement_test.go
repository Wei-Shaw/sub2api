package service

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAITransientRefinement_CrossModelEvidence(t *testing.T) {
	s := newOpenAIAccountTransientState()
	now := time.Now()
	for i := 0; i < 4; i++ {
		d := s.observeModelFailure(1, "model-a", 502, now.Add(time.Duration(i)*10*time.Second))
		require.Zero(t, d.Cooldown)
	}
	d := s.observeModelFailure(1, "model-b", 503, now.Add(35*time.Second))
	require.Equal(t, 10*time.Second, d.Cooldown)
	s = newOpenAIAccountTransientState()
	s.observeModelFailure(1, "a", 502, now)
	s.observeModelFailure(1, "b", 502, now.Add(time.Second))
	s.observeSuccess(1)
	require.Zero(t, s.observeModelFailure(1, "c", 502, now.Add(2*time.Second)).Cooldown)
}

func TestOpenAITransientRefinement_ModelIsolationAndBurstDedup(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, &config.Config{}, nil, nil)
	account := transientPoolAccount(1)
	for range 100 {
		svc.handleOpenAIAccountUpstreamError(context.Background(), &account, 502, http.Header{}, []byte("upstream failure"), "model-a")
	}
	require.False(t, svc.isOpenAIAccountTransientBlocked(&account))
	require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(&account, "model-a"))
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(&account, "model-b"))
	snapshot := svc.SnapshotOpenAIAccountSchedulerState()
	require.Len(t, snapshot.ModelCooldowns, 1)
	require.Equal(t, 1, snapshot.ModelCooldowns[0].FailureStreak)
	require.Equal(t, "upstream_502", snapshot.ModelCooldowns[0].Reason)
}

func TestOpenAITransientRefinement_StrictModelProbeGeneration(t *testing.T) {
	s := newOpenAIAccountModelTransientState(10)
	now := time.Now()
	options := openAITransientFailureOptions{requireProbe: true}
	d := s.recordFailureWithInitialCooldown(1, "a", now, time.Second, options)
	blocked, first := s.reserveProbe(1, "a", d.BlockUntil)
	require.False(t, blocked)
	s.recordSuccess(1, "a")
	require.True(t, s.isBlocked(1, "a", d.BlockUntil))
	d2 := s.recordFailureWithInitialCooldown(1, "a", d.BlockUntil.Add(time.Second), time.Second, options)
	_, second := s.reserveProbe(1, "a", d2.BlockUntil)
	require.False(t, s.completeProbe(1, "a", first, d2.BlockUntil))
	s.releaseProbe(1, "a", first)
	require.True(t, s.isBlocked(1, "a", d2.BlockUntil))
	require.True(t, s.completeProbe(1, "a", second, d2.BlockUntil))
	_, third := s.reserveProbe(1, "a", d2.BlockUntil)
	require.True(t, s.completeProbe(1, "a", third, d2.BlockUntil))
	require.Empty(t, s.snapshot(d2.BlockUntil))
}

func TestOpenAITransientRefinement_SlowProbeBoundAndSnapshot(t *testing.T) {
	s := newOpenAIAccountTransientState()
	now := time.Now()
	d := s.recordFailure(1, now)
	_, first := s.reserveProbe(1, d.BlockUntil)
	require.Equal(t, "probing", s.snapshot(d.BlockUntil)[0].State)
	blocked, _ := s.reserveProbe(1, d.BlockUntil.Add(time.Second))
	require.True(t, blocked)
	require.True(t, s.recordSuccess(1, d.BlockUntil.Add(31*time.Second), first))
	require.Empty(t, s.snapshot(d.BlockUntil.Add(31*time.Second)))
}

func TestOpenAITransientRefinement_ConcurrentModelProbe(t *testing.T) {
	s := newOpenAIAccountModelTransientState(10)
	now := time.Now()
	d := s.recordFailureWithInitialCooldown(1, "a", now, time.Second, openAITransientFailureOptions{requireProbe: true})
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			blocked, id := s.reserveProbe(1, "a", d.BlockUntil)
			if !blocked && id != 0 {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), winners.Load())
}

func TestOpenAITransientRefinement_AdmissionReleasesBothGates(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := transientPoolAccount(1)
	now := time.Now()
	svc.getOpenAIAccountTransientState().recordFailure(1, now.Add(-20*time.Second))
	svc.recordOpenAIAccountModelTransientFailureWithInitialCooldown(&account, "a", now, 10*time.Second)
	releases := 0
	selection, ok := svc.finalizeOpenAIAccountTransientSelection(&AccountSelectionResult{Account: &account, Acquired: true, ReleaseFunc: func() { releases++ }}, "a")
	require.False(t, ok)
	require.Nil(t, selection)
	require.Zero(t, releases, "caller owns concurrency release on veto")
	require.False(t, svc.isOpenAIAccountTransientBlocked(&account), "model veto must release the account probe")
}

func TestOpenAITransientRefinement_SmallGroupWaitAndCancellation(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	groupID := int64(901)
	account := transientPoolAccount(1)
	account.GroupIDs = []int64{groupID}
	svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		cache: &schedulerTestGatewayCache{}, cfg: newSchedulerTestSubscriptionPriorityConfig(),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true")}
	svc.getOpenAIAccountTransientState().recordFailure(1, time.Now().Add(-9900*time.Millisecond))
	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	selection.ReleaseFunc()
	require.Zero(t, svc.getOpenAIAccountTransientState().waiterTotal)
	svc.getOpenAIAccountTransientState().recordFailure(1, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.Error(t, err)
	require.Zero(t, svc.getOpenAIAccountTransientState().waiterTotal)
}

func TestOpenAITransientRefinement_WaiterLimit(t *testing.T) {
	s := newOpenAIAccountTransientState()
	var releases []func()
	for range 8 {
		release, ok := s.reserveWait(1)
		require.True(t, ok)
		releases = append(releases, release)
	}
	_, ok := s.reserveWait(1)
	require.False(t, ok)
	for _, release := range releases {
		release()
		release()
	}
	require.Zero(t, s.waiterTotal)
}

func TestOpenAITransientRefinement_ModelProbeDoesNotExtendAccountCooldown(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := transientPoolAccount(1)
	state := svc.getOpenAIAccountTransientState()
	d := state.recordFailure(1, time.Now().Add(-20*time.Second))
	selection := &AccountSelectionResult{Account: &account, Acquired: true}
	selection, ok := svc.finalizeOpenAIAccountTransientSelection(selection, "model-a")
	require.True(t, ok)
	selection.CompleteTransient(false, &UpstreamFailoverError{StatusCode: 503, ResponseBody: []byte("model unavailable")})
	selection.ReleaseFunc()
	rows := state.snapshot(time.Now())
	require.Len(t, rows, 1)
	require.Equal(t, d.BlockUntil.UTC(), rows[0].BlockUntil)
	require.Equal(t, 1, rows[0].FailureStreak)
	require.Equal(t, 1, rows[0].RecoveryRequired)
	require.Nil(t, rows[0].ProbeStartedAt)
	require.False(t, state.isBlocked(1, time.Now()), "another model may validate recovery")
}

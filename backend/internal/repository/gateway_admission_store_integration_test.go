//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type gatewayAdmissionIntegrationCapacity struct {
	accountID int64
}

func (c gatewayAdmissionIntegrationCapacity) AdmissionCapacity(context.Context, string) (service.AdmissionCapacitySnapshot, error) {
	return service.AdmissionCapacitySnapshot{
		TotalConcurrency: 1,
		AccountConcurrency: map[int64]int{
			c.accountID: 1,
		},
	}, nil
}

func beginRedisBackedAdmissionTarget(
	t *testing.T,
	ctx context.Context,
	leaseTTL time.Duration,
	userID int64,
	accountID int64,
) (service.GatewayAdmissionStore, *service.GatewayAdmissionSession, *service.AdmittedTarget) {
	t.Helper()

	store := NewGatewayAdmissionStore(testRedis(t), leaseTTL)
	admission := service.NewGatewayAdmission(
		store,
		nil,
		gatewayAdmissionIntegrationCapacity{accountID: accountID},
	)
	session, err := admission.Begin(ctx, service.GatewayAdmissionRequest{
		UserID:        userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: service.ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 2,
		},
	})
	require.NoError(t, err)

	account := &service.Account{
		ID:          accountID,
		Platform:    service.PlatformAnthropic,
		Concurrency: 1,
	}
	target, err := session.NextTarget(ctx, service.GatewayTargetRequest{
		Selector: service.GatewayTargetSelectorFunc(func(ctx context.Context, claimer service.TargetClaimer) (*service.AccountSelectionResult, error) {
			release, claimed, claimErr := claimer.TryClaim(ctx, service.TargetClaimRequest{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountConcurrency: account.Concurrency,
			})
			if claimErr != nil {
				return nil, claimErr
			}
			return &service.AccountSelectionResult{
				Account:     account,
				Acquired:    claimed,
				ReleaseFunc: release,
			}, nil
		}),
	})
	require.NoError(t, err)
	return store, session, target
}

func acquireCompetingAdmission(
	t *testing.T,
	store service.GatewayAdmissionStore,
	requestID string,
	userID int64,
	accountID int64,
) (service.UserLeaseResult, service.TargetLeaseResult) {
	t.Helper()

	userLease, err := store.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     requestID,
		UserID:        userID,
		StandardLimit: 1,
		ExtraLimit:    0,
		MaxWaiting:    1,
		WaitTimeout:   2 * time.Second,
	})
	require.NoError(t, err)

	targetLease, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        requestID,
		Platform:         service.PlatformAnthropic,
		AccountID:        accountID,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
		WaitTimeout:      2 * time.Second,
	})
	require.NoError(t, err)
	return userLease, targetLease
}

func assertCompetingAdmissionBlocked(
	t *testing.T,
	store service.GatewayAdmissionStore,
	requestID string,
	userID int64,
	accountID int64,
) {
	t.Helper()
	userLease, targetLease := acquireCompetingAdmission(t, store, requestID, userID, accountID)
	require.False(t, userLease.Acquired)
	require.False(t, targetLease.Acquired)
}

func assertCompetingAdmissionAcquired(
	t *testing.T,
	store service.GatewayAdmissionStore,
	requestID string,
	userID int64,
	accountID int64,
) {
	t.Helper()
	userLease, targetLease := acquireCompetingAdmission(t, store, requestID, userID, accountID)
	require.True(t, userLease.Acquired)
	require.Equal(t, service.AdmissionClassStandard, userLease.Class)
	require.True(t, targetLease.Acquired)

	t.Cleanup(func() {
		_ = store.ReleaseTargetLease(context.Background(), service.PlatformAnthropic, accountID, requestID)
		_ = store.ReleaseUserLease(context.Background(), userID, requestID)
	})
}

func TestGatewayAdmissionStoreAllocatesStandardBeforeExtraAcrossInstances(t *testing.T) {
	rdb := testRedis(t)
	firstStore := NewGatewayAdmissionStore(rdb, time.Minute)
	secondStore := NewGatewayAdmissionStore(rdb, time.Minute)

	standard, err := firstStore.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     "standard-request",
		UserID:        42,
		StandardLimit: 1,
		ExtraLimit:    1,
	})
	require.NoError(t, err)
	require.True(t, standard.Acquired)
	require.Equal(t, service.AdmissionClassStandard, standard.Class)

	extra, err := secondStore.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     "extra-request",
		UserID:        42,
		StandardLimit: 1,
		ExtraLimit:    1,
	})
	require.NoError(t, err)
	require.True(t, extra.Acquired)
	require.Equal(t, service.AdmissionClassExtra, extra.Class)

	blocked, err := secondStore.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     "next-request",
		UserID:        42,
		StandardLimit: 1,
		ExtraLimit:    1,
	})
	require.NoError(t, err)
	require.False(t, blocked.Acquired)

	require.NoError(t, firstStore.ReleaseUserLease(t.Context(), 42, "standard-request"))

	promoted, err := secondStore.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     "extra-request",
		UserID:        42,
		StandardLimit: 1,
		ExtraLimit:    1,
	})
	require.NoError(t, err)
	require.True(t, promoted.Acquired)
	require.Equal(t, service.AdmissionClassStandard, promoted.Class)
}

func TestGatewayAdmissionStoreDisablingExtraRequeuesWithOriginalOrder(t *testing.T) {
	store := NewGatewayAdmissionStore(testRedis(t), time.Minute)
	request := service.UserLeaseRequest{
		UserID:        43,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    2,
		WaitTimeout:   time.Second,
	}

	request.RequestID = "active-standard"
	active, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, active.Acquired)
	require.Equal(t, service.AdmissionClassStandard, active.Class)

	request.RequestID = "earlier-extra"
	earlier, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, earlier.Acquired)
	require.Equal(t, service.AdmissionClassExtra, earlier.Class)

	request.RequestID = "later-standard"
	request.ExtraLimit = 0
	later, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, later.Acquired)

	request.RequestID = "earlier-extra"
	converted, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, converted.Acquired)

	require.NoError(t, store.ReleaseUserLease(t.Context(), request.UserID, "active-standard"))

	request.RequestID = "later-standard"
	later, err = store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, later.Acquired)

	request.RequestID = "earlier-extra"
	earlier, err = store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, earlier.Acquired)
	require.Equal(t, service.AdmissionClassStandard, earlier.Class)
}

func TestGatewayAdmissionStoreExtraTargetPreservesPlatformReserve(t *testing.T) {
	rdb := testRedis(t)
	firstStore := NewGatewayAdmissionStore(rdb, time.Minute)
	secondStore := NewGatewayAdmissionStore(rdb, time.Minute)

	first, err := firstStore.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "first-extra-request",
		Platform:         service.PlatformAnthropic,
		AccountID:        101,
		AccountLimit:     2,
		PlatformCapacity: 2,
		ReservedSlots:    1,
		Class:            service.AdmissionClassExtra,
	})
	require.NoError(t, err)
	require.True(t, first.Acquired)

	second, err := secondStore.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "second-extra-request",
		Platform:         service.PlatformAnthropic,
		AccountID:        101,
		AccountLimit:     2,
		PlatformCapacity: 2,
		ReservedSlots:    1,
		Class:            service.AdmissionClassExtra,
	})
	require.NoError(t, err)
	require.False(t, second.Acquired)

	require.NoError(t, firstStore.ReleaseTargetLease(
		t.Context(),
		service.PlatformAnthropic,
		101,
		"first-extra-request",
	))

	retried, err := secondStore.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "second-extra-request",
		Platform:         service.PlatformAnthropic,
		AccountID:        101,
		AccountLimit:     2,
		PlatformCapacity: 2,
		ReservedSlots:    1,
		Class:            service.AdmissionClassExtra,
	})
	require.NoError(t, err)
	require.True(t, retried.Acquired)
}

func TestGatewayAdmissionStoreTracksUnlimitedTargetWithoutCapacityLimit(t *testing.T) {
	rdb := testRedis(t)
	store := NewGatewayAdmissionStore(rdb, time.Minute)

	result, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID: "unlimited-target",
		Platform:  service.PlatformAnthropic,
		AccountID: 108,
		Class:     service.AdmissionClassExtra,
		Unlimited: true,
	})

	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.NoError(t, store.ReleaseTargetLease(t.Context(), service.PlatformAnthropic, 108, "unlimited-target"))
}

func TestGatewayAdmissionStoreSharesAccountCapacityWithLegacyConcurrency(t *testing.T) {
	rdb := testRedis(t)
	legacy := NewConcurrencyCache(rdb, 1, 60)
	store := NewGatewayAdmissionStore(rdb, time.Minute)
	const accountID int64 = 109

	acquired, err := legacy.AcquireAccountSlot(t.Context(), accountID, 1, "legacy-request")
	require.NoError(t, err)
	require.True(t, acquired)

	request := service.TargetLeaseRequest{
		RequestID:        "gateway-admission-request",
		Platform:         service.PlatformAnthropic,
		AccountID:        accountID,
		AccountLimit:     1,
		PlatformCapacity: 2,
		Class:            service.AdmissionClassStandard,
	}
	blocked, err := store.TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, blocked.Acquired)

	require.NoError(t, legacy.ReleaseAccountSlot(t.Context(), accountID, "legacy-request"))
	admitted, err := store.TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, admitted.Acquired)

	competing, err := legacy.AcquireAccountSlot(t.Context(), accountID, 1, "legacy-competitor")
	require.NoError(t, err)
	require.False(t, competing)
	require.NoError(t, store.ReleaseTargetLease(t.Context(), request.Platform, accountID, request.RequestID))
}

func TestGatewayAdmissionStoreSharesUserCapacityWithLegacyConcurrencyAcrossClients(t *testing.T) {
	clients := testRedisClients(t, 2)
	legacy := NewConcurrencyCache(clients[0], 1, 60)
	gateway := NewGatewayAdmissionStore(clients[1], time.Minute)
	const userID int64 = 110

	legacyAcquired, err := legacy.AcquireUserSlot(t.Context(), userID, 1, "legacy-standard")
	require.NoError(t, err)
	require.True(t, legacyAcquired)

	first, err := gateway.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     "stale-enabled-first",
		UserID:        userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   time.Minute,
	})
	require.NoError(t, err)
	require.True(t, first.Acquired)
	require.Equal(t, service.AdmissionClassExtra, first.Class)

	second, err := gateway.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     "stale-enabled-second",
		UserID:        userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   time.Minute,
	})
	require.NoError(t, err)
	require.False(t, second.Acquired)
	require.False(t, second.QueueFull)
}

func TestGatewayAdmissionStoreStandardWaiterBlocksEarlierExtraWaiter(t *testing.T) {
	store := NewGatewayAdmissionStore(testRedis(t), time.Minute)
	baseRequest := service.TargetLeaseRequest{
		Platform:         service.PlatformAnthropic,
		AccountID:        202,
		AccountLimit:     1,
		PlatformCapacity: 1,
	}

	activeRequest := baseRequest
	activeRequest.RequestID = "active-standard"
	activeRequest.Class = service.AdmissionClassStandard
	active, err := store.TryAcquireTargetLease(t.Context(), activeRequest)
	require.NoError(t, err)
	require.True(t, active.Acquired)

	extraRequest := baseRequest
	extraRequest.RequestID = "earlier-extra-waiter"
	extraRequest.Class = service.AdmissionClassExtra
	extra, err := store.TryAcquireTargetLease(t.Context(), extraRequest)
	require.NoError(t, err)
	require.False(t, extra.Acquired)

	standardRequest := baseRequest
	standardRequest.RequestID = "later-standard-waiter"
	standardRequest.Class = service.AdmissionClassStandard
	standard, err := store.TryAcquireTargetLease(t.Context(), standardRequest)
	require.NoError(t, err)
	require.False(t, standard.Acquired)

	require.NoError(t, store.ReleaseTargetLease(
		t.Context(),
		baseRequest.Platform,
		baseRequest.AccountID,
		activeRequest.RequestID,
	))

	extra, err = store.TryAcquireTargetLease(t.Context(), extraRequest)
	require.NoError(t, err)
	require.False(t, extra.Acquired)

	standard, err = store.TryAcquireTargetLease(t.Context(), standardRequest)
	require.NoError(t, err)
	require.True(t, standard.Acquired)
}

func TestGatewayAdmissionStoreExpiredStandardWaiterStopsBlockingExtra(t *testing.T) {
	store := NewGatewayAdmissionStore(testRedis(t), time.Minute)
	baseRequest := service.TargetLeaseRequest{
		Platform:         service.PlatformAnthropic,
		AccountID:        303,
		AccountLimit:     1,
		PlatformCapacity: 1,
	}

	activeRequest := baseRequest
	activeRequest.RequestID = "active-standard"
	activeRequest.Class = service.AdmissionClassStandard
	active, err := store.TryAcquireTargetLease(t.Context(), activeRequest)
	require.NoError(t, err)
	require.True(t, active.Acquired)

	standardRequest := baseRequest
	standardRequest.RequestID = "expired-standard-waiter"
	standardRequest.Class = service.AdmissionClassStandard
	standardRequest.WaitTimeout = 20 * time.Millisecond
	standard, err := store.TryAcquireTargetLease(t.Context(), standardRequest)
	require.NoError(t, err)
	require.False(t, standard.Acquired)

	extraRequest := baseRequest
	extraRequest.RequestID = "extra-waiter"
	extraRequest.Class = service.AdmissionClassExtra
	extraRequest.WaitTimeout = time.Second
	extra, err := store.TryAcquireTargetLease(t.Context(), extraRequest)
	require.NoError(t, err)
	require.False(t, extra.Acquired)

	time.Sleep(40 * time.Millisecond)
	require.NoError(t, store.ReleaseTargetLease(
		t.Context(),
		baseRequest.Platform,
		baseRequest.AccountID,
		activeRequest.RequestID,
	))

	extra, err = store.TryAcquireTargetLease(t.Context(), extraRequest)
	require.NoError(t, err)
	require.True(t, extra.Acquired)
}

func TestGatewayAdmissionStoreRenewKeepsUserAndTargetLeasesAlive(t *testing.T) {
	store := NewGatewayAdmissionStore(testRedis(t), 2*time.Second)
	requestID := "long-running-request"

	userLease, err := store.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     requestID,
		UserID:        404,
		StandardLimit: 1,
		ExtraLimit:    1,
	})
	require.NoError(t, err)
	require.True(t, userLease.Acquired)
	require.Equal(t, service.AdmissionClassStandard, userLease.Class)

	targetRequest := service.TargetLeaseRequest{
		RequestID:        requestID,
		Platform:         service.PlatformAnthropic,
		AccountID:        404,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
	}
	targetLease, err := store.TryAcquireTargetLease(t.Context(), targetRequest)
	require.NoError(t, err)
	require.True(t, targetLease.Acquired)

	time.Sleep(1200 * time.Millisecond)
	renewed, err := store.RenewUserLease(
		t.Context(),
		404,
		requestID,
		service.AdmissionClassStandard,
	)
	require.NoError(t, err)
	require.True(t, renewed)
	renewed, err = store.RenewTargetLease(
		t.Context(),
		service.PlatformAnthropic,
		404,
		requestID,
	)
	require.NoError(t, err)
	require.True(t, renewed)

	time.Sleep(1200 * time.Millisecond)
	competingUser, err := store.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     "competing-request",
		UserID:        404,
		StandardLimit: 1,
	})
	require.NoError(t, err)
	require.False(t, competingUser.Acquired)

	competingTarget := targetRequest
	competingTarget.RequestID = "competing-request"
	competingTarget.Class = service.AdmissionClassStandard
	competingLease, err := store.TryAcquireTargetLease(t.Context(), competingTarget)
	require.NoError(t, err)
	require.False(t, competingLease.Acquired)
}

func TestGatewayAdmissionStoreBeginDispatchRejectsExpiredMemberKeptAliveByPeer(t *testing.T) {
	const leaseTTL = 500 * time.Millisecond
	store := NewGatewayAdmissionStore(testRedis(t), leaseTTL)
	request := service.TargetLeaseRequest{
		Platform:         service.PlatformAnthropic,
		AccountID:        405,
		AccountLimit:     2,
		PlatformCapacity: 2,
		Class:            service.AdmissionClassStandard,
	}

	request.RequestID = "expired-before-dispatch"
	first, err := store.TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, first.Acquired)
	request.RequestID = "live-peer"
	second, err := store.TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, second.Acquired)

	time.Sleep(300 * time.Millisecond)
	renewed, err := store.RenewTargetLease(
		t.Context(),
		request.Platform,
		request.AccountID,
		"live-peer",
	)
	require.NoError(t, err)
	require.True(t, renewed)
	time.Sleep(300 * time.Millisecond)
	renewed, err = store.RenewTargetLease(
		t.Context(),
		request.Platform,
		request.AccountID,
		"expired-before-dispatch",
	)
	require.NoError(t, err)
	require.False(t, renewed, "renewal must not resurrect an already expired target member")

	expired, err := store.BeginTargetDispatch(t.Context(), service.TargetDispatchRequest{
		RequestID: "expired-before-dispatch",
		Platform:  request.Platform,
		AccountID: request.AccountID,
		Class:     request.Class,
	})
	require.NoError(t, err)
	require.False(t, expired.Started, "an expired sorted-set member must not be resurrected at dispatch")

	live, err := store.BeginTargetDispatch(t.Context(), service.TargetDispatchRequest{
		RequestID: "live-peer",
		Platform:  request.Platform,
		AccountID: request.AccountID,
		Class:     request.Class,
	})
	require.NoError(t, err)
	require.True(t, live.Started, "the peer renewal must keep the shared keys alive for this assertion")
}

func TestGatewayAdmissionDispatchAutomaticallyRenewsRedisLeasesUntilCompletion(t *testing.T) {
	const (
		userID    int64 = 901
		accountID int64 = 801
	)
	store, session, target := beginRedisBackedAdmissionTarget(
		t,
		t.Context(),
		300*time.Millisecond,
		userID,
		accountID,
	)
	t.Cleanup(session.Close)

	upstreamStarted := make(chan struct{})
	finishUpstream := make(chan struct{}, 1)
	t.Cleanup(func() {
		select {
		case finishUpstream <- struct{}{}:
		default:
		}
	})
	dispatchDone := make(chan error, 1)
	go func() {
		dispatchDone <- target.Dispatch(
			t.Context(),
			nil,
			func(context.Context, *service.Account) error {
				close(upstreamStarted)
				<-finishUpstream
				return nil
			},
		)
	}()
	<-upstreamStarted

	// The shared account lease has second-level compatibility TTL semantics.
	// Holding past one second ensures a non-renewing implementation would have
	// expired every user, platform, and account lease before this assertion.
	time.Sleep(1500 * time.Millisecond)
	assertCompetingAdmissionBlocked(t, store, "dispatch-competitor", userID, accountID)

	finishUpstream <- struct{}{}
	require.NoError(t, <-dispatchDone)
	session.Close()
	session.Close()

	assertCompetingAdmissionAcquired(t, store, "dispatch-competitor", userID, accountID)
}

func TestGatewayAdmissionContextCancellationReleasesHeldRedisLeasesIdempotently(t *testing.T) {
	const (
		userID    int64 = 902
		accountID int64 = 802
	)
	requestCtx, cancelRequest := context.WithCancel(t.Context())
	store, session, target := beginRedisBackedAdmissionTarget(
		t,
		requestCtx,
		300*time.Millisecond,
		userID,
		accountID,
	)
	t.Cleanup(session.Close)

	upstreamStarted := make(chan struct{})
	dispatchDone := make(chan error, 1)
	go func() {
		dispatchDone <- target.Dispatch(
			requestCtx,
			nil,
			func(ctx context.Context, _ *service.Account) error {
				close(upstreamStarted)
				<-ctx.Done()
				return ctx.Err()
			},
		)
	}()
	<-upstreamStarted

	cancelRequest()
	require.ErrorIs(t, <-dispatchDone, context.Canceled)
	session.Close()
	session.Close()

	assertCompetingAdmissionAcquired(t, store, "cancel-competitor", userID, accountID)
}

func TestGatewayAdmissionUpstreamErrorReleasesHeldRedisLeasesIdempotently(t *testing.T) {
	const (
		userID    int64 = 903
		accountID int64 = 803
	)
	store, session, target := beginRedisBackedAdmissionTarget(
		t,
		t.Context(),
		300*time.Millisecond,
		userID,
		accountID,
	)
	t.Cleanup(session.Close)

	upstreamErr := fmt.Errorf("upstream stream read failed")
	err := target.Dispatch(
		t.Context(),
		nil,
		func(context.Context, *service.Account) error {
			return upstreamErr
		},
	)
	require.ErrorIs(t, err, upstreamErr)
	session.Close()
	session.Close()

	assertCompetingAdmissionAcquired(t, store, "error-competitor", userID, accountID)
}

func TestGatewayAdmissionStoreUserLeaseContentionAcrossRedisClients(t *testing.T) {
	clients := testRedisClients(t, 2)
	stores := []service.GatewayAdmissionStore{
		NewGatewayAdmissionStore(clients[0], time.Minute),
		NewGatewayAdmissionStore(clients[1], time.Minute),
	}

	const attempts = 32
	start := make(chan struct{})
	results := make(chan struct {
		lease service.UserLeaseResult
		err   error
	}, attempts)
	for i := range attempts {
		go func(attempt int) {
			<-start
			lease, err := stores[attempt%len(stores)].TryAcquireUserLease(
				t.Context(),
				service.UserLeaseRequest{
					RequestID:     fmt.Sprintf("request-%02d", attempt),
					UserID:        505,
					StandardLimit: 1,
					ExtraLimit:    1,
				},
			)
			results <- struct {
				lease service.UserLeaseResult
				err   error
			}{lease: lease, err: err}
		}(i)
	}
	close(start)

	standardCount := 0
	extraCount := 0
	for range attempts {
		result := <-results
		require.NoError(t, result.err)
		if !result.lease.Acquired {
			continue
		}
		switch result.lease.Class {
		case service.AdmissionClassStandard:
			standardCount++
		case service.AdmissionClassExtra:
			extraCount++
		default:
			t.Fatalf("unexpected admission class %q", result.lease.Class)
		}
	}

	require.Equal(t, 1, standardCount)
	require.Equal(t, 1, extraCount)
}

func TestUserLoadBatchSplitsGatewayStandardAndExtraWithCombinedWaiting(t *testing.T) {
	ctx := t.Context()
	clients := testRedisClients(t, 2)
	legacy := NewConcurrencyCache(clients[0], 1, 60)
	gateway := NewGatewayAdmissionStore(clients[1], time.Minute)
	const userID int64 = 506

	legacyAcquired, err := legacy.AcquireUserSlot(ctx, userID, 2, "legacy-standard")
	require.NoError(t, err)
	require.True(t, legacyAcquired)
	require.True(t, mustIncrementWaitCount(t, legacy, ctx, userID, 20))

	standard, err := gateway.TryAcquireUserLease(ctx, service.UserLeaseRequest{
		RequestID:     "gateway-standard",
		UserID:        userID,
		StandardLimit: 2,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   time.Minute,
	})
	require.NoError(t, err)
	require.True(t, standard.Acquired)
	require.Equal(t, service.AdmissionClassStandard, standard.Class)

	extra, err := gateway.TryAcquireUserLease(ctx, service.UserLeaseRequest{
		RequestID:     "gateway-extra",
		UserID:        userID,
		StandardLimit: 2,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   time.Minute,
	})
	require.NoError(t, err)
	require.True(t, extra.Acquired)
	require.Equal(t, service.AdmissionClassExtra, extra.Class)

	waiting, err := gateway.TryAcquireUserLease(ctx, service.UserLeaseRequest{
		RequestID:     "gateway-waiting",
		UserID:        userID,
		StandardLimit: 2,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   time.Minute,
	})
	require.NoError(t, err)
	require.False(t, waiting.Acquired)

	loads, err := legacy.GetUsersLoadBatch(ctx, []service.UserWithConcurrency{{
		ID:               userID,
		MaxConcurrency:   2,
		ExtraConcurrency: 1,
	}})
	require.NoError(t, err)
	load := loads[userID]
	require.NotNil(t, load)
	require.Equal(t, 2, load.StandardConcurrency)
	require.Equal(t, 1, load.ExtraConcurrency)
	require.Equal(t, 3, load.CurrentConcurrency)
	require.Equal(t, 2, load.WaitingCount)
}

func mustIncrementWaitCount(t *testing.T, cache service.ConcurrencyCache, ctx context.Context, userID int64, maxWait int) bool {
	t.Helper()
	ok, err := cache.IncrementWaitCount(ctx, userID, maxWait)
	require.NoError(t, err)
	return ok
}

func TestGatewayAdmissionStoreUnlimitedStandardNeverConsumesExtra(t *testing.T) {
	store := NewGatewayAdmissionStore(testRedis(t), time.Minute)

	for i := range 3 {
		lease, err := store.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
			RequestID:     fmt.Sprintf("unlimited-request-%d", i),
			UserID:        606,
			StandardLimit: 0,
			ExtraLimit:    1,
		})
		require.NoError(t, err)
		require.True(t, lease.Acquired)
		require.Equal(t, service.AdmissionClassStandard, lease.Class)
		require.True(t, lease.Unlimited)
	}
}

func TestGatewayAdmissionStoreUserWaitersAreFIFO(t *testing.T) {
	store := NewGatewayAdmissionStore(testRedis(t), time.Minute)
	request := service.UserLeaseRequest{
		UserID:        707,
		StandardLimit: 1,
	}

	request.RequestID = "active-request"
	active, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, active.Acquired)

	request.RequestID = "earlier-waiter"
	earlier, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, earlier.Acquired)

	request.RequestID = "later-waiter"
	later, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, later.Acquired)

	require.NoError(t, store.ReleaseUserLease(t.Context(), 707, "active-request"))

	later, err = store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, later.Acquired)

	request.RequestID = "earlier-waiter"
	earlier, err = store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, earlier.Acquired)
	require.Equal(t, service.AdmissionClassStandard, earlier.Class)
}

func TestGatewayAdmissionStoreRejectsUserWaiterWhenMaxWaitingReached(t *testing.T) {
	store := NewGatewayAdmissionStore(testRedis(t), time.Minute)
	request := service.UserLeaseRequest{
		UserID:        708,
		StandardLimit: 1,
		MaxWaiting:    1,
	}

	request.RequestID = "active-request"
	active, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, active.Acquired)

	request.RequestID = "accepted-waiter"
	accepted, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, accepted.Acquired)
	require.False(t, accepted.QueueFull)

	request.RequestID = "rejected-waiter"
	rejected, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, rejected.Acquired)
	require.True(t, rejected.QueueFull)
}

func TestGatewayAdmissionStoreExpiredUserWaiterDoesNotBlockQueue(t *testing.T) {
	store := NewGatewayAdmissionStore(testRedis(t), time.Minute)
	request := service.UserLeaseRequest{
		UserID:        709,
		StandardLimit: 1,
		MaxWaiting:    2,
		WaitTimeout:   time.Second,
	}

	request.RequestID = "active-request"
	active, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, active.Acquired)

	request.RequestID = "crashed-waiter"
	request.WaitTimeout = 50 * time.Millisecond
	crashed, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, crashed.Acquired)

	request.RequestID = "live-waiter"
	request.WaitTimeout = time.Second
	live, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, live.Acquired)

	time.Sleep(100 * time.Millisecond)
	require.NoError(t, store.ReleaseUserLease(t.Context(), 709, "active-request"))

	live, err = store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, live.Acquired)
	require.Equal(t, service.AdmissionClassStandard, live.Class)
}

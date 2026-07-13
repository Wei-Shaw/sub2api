//go:build unit

package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type gatewayAdmissionSessionStoreStub struct {
	GatewayAdmissionStore
	userRequest           UserLeaseRequest
	userResult            UserLeaseResult
	userResults           []UserLeaseResult
	targetRequest         TargetLeaseRequest
	targetResult          TargetLeaseResult
	targetResults         []TargetLeaseResult
	targetDispatch        TargetDispatchResult
	targetDispatchResults []TargetDispatchResult
	targetDispatchFunc    func(TargetDispatchRequest) (TargetDispatchResult, error)
	targetBlocked         bool
	targetWaitForDone     bool
	targetAttempted       chan struct{}
	targetAcquireReady    chan struct{}
	targetAcquireStart    chan struct{}
	userAcquireReady      chan struct{}
	userAcquireStart      chan struct{}
	userAcquireCalls      atomic.Int32
	targetAcquireCalls    atomic.Int32
	releaseCalls          atomic.Int32
	targetReleaseCalls    atomic.Int32
	targetDispatchCalls   atomic.Int32
	renewUserCalls        atomic.Int32
	renewTargetCalls      atomic.Int32
}

type targetConcurrencyAdmissionStore struct {
	*gatewayAdmissionSessionStoreStub
	expectedAccountConcurrency int
}

func (s *targetConcurrencyAdmissionStore) TryAcquireTargetLease(_ context.Context, request TargetLeaseRequest) (TargetLeaseResult, error) {
	if request.AccountLimit != s.expectedAccountConcurrency {
		return TargetLeaseResult{}, errors.New("target account concurrency was not preserved")
	}
	return TargetLeaseResult{Acquired: true}, nil
}

type extraConcurrencyRuntimeSettingsSourceFunc func(context.Context) ExtraConcurrencyRuntimeSettings

func (f extraConcurrencyRuntimeSettingsSourceFunc) GetExtraConcurrencyRuntimeSettings(ctx context.Context) ExtraConcurrencyRuntimeSettings {
	return f(ctx)
}

func (s *gatewayAdmissionSessionStoreStub) TryAcquireUserLease(_ context.Context, request UserLeaseRequest) (UserLeaseResult, error) {
	call := s.userAcquireCalls.Add(1)
	s.userRequest = request
	if call > 1 && s.userAcquireReady != nil {
		if s.userAcquireStart != nil {
			select {
			case s.userAcquireStart <- struct{}{}:
			default:
			}
		}
		<-s.userAcquireReady
	}
	if index := int(call) - 1; index < len(s.userResults) {
		return s.userResults[index], nil
	}
	return s.userResult, nil
}

func (s *gatewayAdmissionSessionStoreStub) ReleaseUserLease(context.Context, int64, string) error {
	s.releaseCalls.Add(1)
	return nil
}

func (s *gatewayAdmissionSessionStoreStub) RenewUserLease(context.Context, int64, string, AdmissionClass) (bool, error) {
	s.renewUserCalls.Add(1)
	return true, nil
}

func (s *gatewayAdmissionSessionStoreStub) TryAcquireTargetLease(ctx context.Context, request TargetLeaseRequest) (TargetLeaseResult, error) {
	call := s.targetAcquireCalls.Add(1)
	s.targetRequest = request
	if s.targetWaitForDone {
		<-ctx.Done()
		return TargetLeaseResult{}, ctx.Err()
	}
	if s.targetAcquireReady != nil {
		if s.targetAcquireStart != nil {
			select {
			case s.targetAcquireStart <- struct{}{}:
			default:
			}
		}
		<-s.targetAcquireReady
	}
	if s.targetBlocked {
		if s.targetAttempted != nil {
			select {
			case s.targetAttempted <- struct{}{}:
			default:
			}
		}
		return TargetLeaseResult{}, nil
	}
	if index := int(call) - 1; index < len(s.targetResults) {
		return s.targetResults[index], nil
	}
	if s.targetResult != (TargetLeaseResult{}) {
		return s.targetResult, nil
	}
	return TargetLeaseResult{Acquired: true}, nil
}

func TestGatewayAdmissionStaleDrainRefreshWaitsForStandardInsteadOfLosingUserLease(t *testing.T) {
	const accountID int64 = 73
	settings := ExtraConcurrencyRuntimeSettings{
		Enabled:            true,
		WaitTimeoutSeconds: 1,
		PlatformReserves:   map[string]ExtraConcurrencyPlatformReserve{},
	}
	store := &gatewayAdmissionSessionStoreStub{
		userResults: []UserLeaseResult{
			{Acquired: true, Class: AdmissionClassExtra},
			{}, // Redis drain removed the extra lease and queued it for standard capacity.
			{Acquired: true, Class: AdmissionClassStandard},
		},
		targetResults: []TargetLeaseResult{
			{},
			{Acquired: true},
		},
	}
	admission := NewGatewayAdmission(
		store,
		nil,
		admissionCapacitySourceFunc(func(context.Context, string) (AdmissionCapacitySnapshot, error) {
			return AdmissionCapacitySnapshot{
				TotalConcurrency:   1,
				AccountConcurrency: map[int64]int{accountID: 1},
			}, nil
		}),
	)
	// Model a subscriber/cache lag: Redis already drains, while this instance still reads enabled.
	admission.SetExtraConcurrencyRuntimeSettingsSource(extraConcurrencyRuntimeSettingsSourceFunc(
		func(context.Context) ExtraConcurrencyRuntimeSettings { return settings },
	))
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        919,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings:      settings,
	})
	require.NoError(t, err)
	defer session.Close()

	target, err := session.NextTarget(context.Background(), GatewayTargetRequest{
		Selector: GatewayTargetSelectorFunc(func(ctx context.Context, claimer TargetClaimer) (*AccountSelectionResult, error) {
			account := &Account{ID: accountID, Platform: PlatformAnthropic, Concurrency: 1}
			release, acquired, claimErr := claimer.TryClaim(ctx, TargetClaimRequest{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountConcurrency: account.Concurrency,
			})
			return &AccountSelectionResult{Account: account, Acquired: acquired, ReleaseFunc: release}, claimErr
		}),
	})

	require.NoError(t, err)
	require.Equal(t, AdmissionClassStandard, target.Class)
	require.Equal(t, AdmissionClassStandard, session.Class())
	require.Equal(t, int32(3), store.userAcquireCalls.Load())
	require.Equal(t, int32(2), store.targetAcquireCalls.Load())
	require.Zero(t, store.userRequest.ExtraLimit)
}

func (s *gatewayAdmissionSessionStoreStub) ReleaseTargetLease(context.Context, string, int64, string) error {
	s.targetReleaseCalls.Add(1)
	return nil
}

func (s *gatewayAdmissionSessionStoreStub) BeginTargetDispatch(_ context.Context, request TargetDispatchRequest) (TargetDispatchResult, error) {
	call := s.targetDispatchCalls.Add(1)
	if s.targetDispatchFunc != nil {
		return s.targetDispatchFunc(request)
	}
	if index := int(call) - 1; index < len(s.targetDispatchResults) {
		return s.targetDispatchResults[index], nil
	}
	if s.targetDispatch != (TargetDispatchResult{}) {
		return s.targetDispatch, nil
	}
	return TargetDispatchResult{Started: true}, nil
}

func (s *gatewayAdmissionSessionStoreStub) RenewTargetLease(context.Context, string, int64, string) (bool, error) {
	s.renewTargetCalls.Add(1)
	return true, nil
}

func TestGatewayAdmissionBeginOwnsAndReleasesUserLease(t *testing.T) {
	store := &gatewayAdmissionSessionStoreStub{
		userResult: UserLeaseResult{Acquired: true, Class: AdmissionClassExtra},
	}
	admission := NewGatewayAdmission(store, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())

	session, err := admission.Begin(ctx, GatewayAdmissionRequest{
		UserID:        808,
		StandardLimit: 1,
		ExtraLimit:    2,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, AdmissionClassExtra, session.Class())
	require.Equal(t, int64(808), store.userRequest.UserID)
	require.Equal(t, 1, store.userRequest.StandardLimit)
	require.Equal(t, 2, store.userRequest.ExtraLimit)
	require.NotEmpty(t, store.userRequest.RequestID)

	cancel()
	require.Eventually(t, func() bool {
		return store.releaseCalls.Load() == 1
	}, time.Second, 10*time.Millisecond)
	session.Close()
	session.Close()
	require.Equal(t, int32(1), store.releaseCalls.Load())
}

func TestGatewayAdmissionBeginReturnsQueueFullWithoutWaiting(t *testing.T) {
	store := &gatewayAdmissionSessionStoreStub{
		userResult: UserLeaseResult{QueueFull: true},
	}
	admission := NewGatewayAdmission(store, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := admission.Begin(ctx, GatewayAdmissionRequest{
		UserID:        809,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})

	var queueFull *GatewayAdmissionQueueFullError
	require.ErrorAs(t, err, &queueFull)
	require.Equal(t, int32(1), store.releaseCalls.Load())
}

func TestGatewayAdmissionBeginReturnsDrainingSignalWithoutWaiting(t *testing.T) {
	store := &gatewayAdmissionSessionStoreStub{
		userResult: UserLeaseResult{Draining: true},
	}
	admission := NewGatewayAdmission(store, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := admission.Begin(ctx, GatewayAdmissionRequest{
		UserID:        812,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})

	require.ErrorIs(t, err, ErrGatewayAdmissionDraining)
	require.Equal(t, int32(1), store.userAcquireCalls.Load())
	require.Equal(t, int32(1), store.releaseCalls.Load())
}

func TestGatewayAdmissionBeginStandardOnlyTimeoutUsesStandardError(t *testing.T) {
	store := &gatewayAdmissionSessionStoreStub{}
	admission := NewGatewayAdmission(store, nil, nil)

	_, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        810,
		StandardLimit: 1,
		ExtraLimit:    0,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})

	var timeout *GatewayAdmissionTimeoutError
	require.ErrorAs(t, err, &timeout)
	require.Equal(t, "user", timeout.SlotType)
}

func TestGatewayAdmissionSessionNextTargetOwnsAtomicTargetLease(t *testing.T) {
	account := Account{
		ID:          42,
		Platform:    PlatformAnthropic,
		Priority:    1,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 3,
	}
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{account},
		accountsByID: map[int64]*Account{account.ID: &account},
	}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	gatewayService := &GatewayService{
		accountRepo:        repo,
		cache:              &mockGatewayCacheForPlatform{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(&mockConcurrencyCache{}),
	}
	store := &gatewayAdmissionSessionStoreStub{
		userResult: UserLeaseResult{Acquired: true, Class: AdmissionClassStandard},
	}
	admission := NewGatewayAdmission(store, gatewayService, gatewayService)
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        909,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)

	target, err := session.NextTarget(context.Background(), GatewayTargetRequest{
		Model: "claude-3-5-sonnet-20241022",
	})

	require.NoError(t, err)
	require.NotNil(t, target)
	require.NotNil(t, target.Account)
	require.Equal(t, int64(42), target.Account.ID)
	require.Equal(t, AdmissionClassStandard, target.Class)
	require.Equal(t, int64(42), store.targetRequest.AccountID)
	require.Equal(t, 3, store.targetRequest.AccountLimit)

	session.ReleaseTarget()
	session.ReleaseTarget()
	require.Equal(t, int32(1), store.targetReleaseCalls.Load())
	session.Close()
	session.Close()
	require.Equal(t, int32(1), store.releaseCalls.Load())
	require.Equal(t, int32(1), store.targetReleaseCalls.Load())
}

func TestGatewayAdmissionSessionDisabledExtraFallsBackToStandardBeforeTarget(t *testing.T) {
	store := &gatewayAdmissionSessionStoreStub{
		userResults: []UserLeaseResult{
			{Acquired: true, Class: AdmissionClassExtra},
			{Acquired: true, Class: AdmissionClassStandard},
		},
	}
	admission := NewGatewayAdmission(store, nil, nil)
	admission.SetExtraConcurrencyRuntimeSettingsSource(extraConcurrencyRuntimeSettingsSourceFunc(
		func(context.Context) ExtraConcurrencyRuntimeSettings {
			return ExtraConcurrencyRuntimeSettings{Enabled: false, WaitTimeoutSeconds: 1}
		},
	))
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        910,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			Enabled:            true,
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)

	target, err := session.NextTarget(context.Background(), GatewayTargetRequest{
		Selector: GatewayTargetSelectorFunc(func(ctx context.Context, claimer TargetClaimer) (*AccountSelectionResult, error) {
			account := &Account{ID: 91, Platform: PlatformAnthropic, Concurrency: 1}
			release, acquired, claimErr := claimer.TryClaim(ctx, TargetClaimRequest{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountConcurrency: account.Concurrency,
			})
			return &AccountSelectionResult{Account: account, Acquired: acquired, ReleaseFunc: release}, claimErr
		}),
	})

	require.NoError(t, err)
	require.NotNil(t, target)
	require.Equal(t, AdmissionClassStandard, target.Class)
	require.Equal(t, int32(2), store.userAcquireCalls.Load())
	require.Zero(t, store.userRequest.ExtraLimit)
	require.Equal(t, AdmissionClassStandard, store.targetRequest.Class)
}

func TestGatewayAdmissionSessionNextTargetUsesInjectedSelector(t *testing.T) {
	store := &gatewayAdmissionSessionStoreStub{
		userResult: UserLeaseResult{Acquired: true, Class: AdmissionClassStandard},
	}
	admission := NewGatewayAdmission(store, nil, nil)
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        910,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)
	defer session.Close()

	selectorCalled := false
	target, err := session.NextTarget(context.Background(), GatewayTargetRequest{
		Selector: GatewayTargetSelectorFunc(func(ctx context.Context, claimer TargetClaimer) (*AccountSelectionResult, error) {
			selectorCalled = true
			account := &Account{
				ID:          52,
				Platform:    PlatformOpenAI,
				Concurrency: 4,
			}
			claim, err := tryClaimTarget(ctx, claimer, account)
			if err != nil {
				return nil, err
			}
			return &AccountSelectionResult{
				Account:     account,
				Acquired:    claim.Acquired,
				ReleaseFunc: claim.ReleaseFunc,
			}, nil
		}),
	})

	require.NoError(t, err)
	require.True(t, selectorCalled)
	require.Equal(t, int64(52), target.Account.ID)
	require.Equal(t, PlatformOpenAI, store.targetRequest.Platform)
	require.Equal(t, 4, store.targetRequest.AccountLimit)
}

func TestGatewayAdmissionSessionImmediateTargetDoesNotRefreshUserLease(t *testing.T) {
	target, session, store := newAdmittedTargetForDispatchTest(t)
	defer session.Close()

	require.NotNil(t, target)
	require.Equal(t, int32(1), store.userAcquireCalls.Load())
}

func TestGatewayAdmissionSessionPromotesWaitedExtraBeforeImmediateTarget(t *testing.T) {
	account := Account{
		ID:          49,
		Platform:    PlatformAnthropic,
		Priority:    1,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{account},
		accountsByID: map[int64]*Account{account.ID: &account},
	}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	gatewayService := &GatewayService{
		accountRepo:        repo,
		cache:              &mockGatewayCacheForPlatform{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(&mockConcurrencyCache{}),
	}
	store := &gatewayAdmissionSessionStoreStub{
		userResults: []UserLeaseResult{
			{},
			{Acquired: true, Class: AdmissionClassExtra},
			{Acquired: true, Class: AdmissionClassStandard},
		},
	}
	admission := NewGatewayAdmission(store, gatewayService, gatewayService)
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        916,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)
	defer session.Close()
	require.True(t, session.Waited())
	require.Equal(t, AdmissionClassExtra, session.Class())

	target, err := session.NextTarget(context.Background(), GatewayTargetRequest{
		Model: "claude-3-5-sonnet-20241022",
	})

	require.NoError(t, err)
	require.Equal(t, AdmissionClassStandard, target.Class)
	require.Equal(t, AdmissionClassStandard, store.targetRequest.Class)
	require.Equal(t, int32(3), store.userAcquireCalls.Load())
}

func TestGatewayAdmissionSessionReturnsExtraTimeoutFromTargetStore(t *testing.T) {
	account := Account{
		ID:          43,
		Platform:    PlatformAnthropic,
		Priority:    1,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{account},
		accountsByID: map[int64]*Account{account.ID: &account},
	}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	gatewayService := &GatewayService{
		accountRepo:        repo,
		cache:              &mockGatewayCacheForPlatform{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(&mockConcurrencyCache{}),
	}
	store := &gatewayAdmissionSessionStoreStub{
		userResult:   UserLeaseResult{Acquired: true, Class: AdmissionClassExtra},
		targetResult: TargetLeaseResult{Expired: true},
	}
	admission := NewGatewayAdmission(store, gatewayService, gatewayService)
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        910,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = session.NextTarget(ctx, GatewayTargetRequest{Model: "claude-3-5-sonnet-20241022"})

	var unavailable *ExtraConcurrencyUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.True(t, unavailable.Timeout)
	require.False(t, errors.Is(err, context.DeadlineExceeded))
}

func TestGatewayAdmissionSessionNormalizesInternalTargetDeadlineForExtraRequest(t *testing.T) {
	const accountID int64 = 53
	store := &gatewayAdmissionSessionStoreStub{
		userResult:        UserLeaseResult{Acquired: true, Class: AdmissionClassExtra},
		targetWaitForDone: true,
	}
	admission := NewGatewayAdmission(
		store,
		nil,
		admissionCapacitySourceFunc(func(context.Context, string) (AdmissionCapacitySnapshot, error) {
			return AdmissionCapacitySnapshot{
				TotalConcurrency:   1,
				AccountConcurrency: map[int64]int{accountID: 1},
			}, nil
		}),
	)
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        917,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)
	defer session.Close()

	_, err = session.NextTarget(context.Background(), GatewayTargetRequest{
		Selector: GatewayTargetSelectorFunc(func(ctx context.Context, claimer TargetClaimer) (*AccountSelectionResult, error) {
			release, acquired, claimErr := claimer.TryClaim(ctx, TargetClaimRequest{
				Platform:           PlatformGemini,
				AccountID:          accountID,
				AccountConcurrency: 1,
			})
			return &AccountSelectionResult{
				Account:     &Account{ID: accountID, Platform: PlatformGemini, Concurrency: 1},
				Acquired:    acquired,
				ReleaseFunc: release,
			}, claimErr
		}),
	})

	var unavailable *ExtraConcurrencyUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.True(t, unavailable.Timeout)
	require.False(t, errors.Is(err, context.DeadlineExceeded))
}

func TestGatewayAdmissionSessionExtraCapacityFailureStillHonorsWaitTimeout(t *testing.T) {
	account := Account{
		ID:          46,
		Platform:    PlatformAnthropic,
		Priority:    1,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{account},
		accountsByID: map[int64]*Account{account.ID: &account},
	}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	gatewayService := &GatewayService{
		accountRepo:        repo,
		cache:              &mockGatewayCacheForPlatform{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(&mockConcurrencyCache{}),
	}
	store := &gatewayAdmissionSessionStoreStub{
		userResult: UserLeaseResult{Acquired: true, Class: AdmissionClassExtra},
	}
	admission := NewGatewayAdmission(
		store,
		gatewayService,
		admissionCapacitySourceFunc(func(context.Context, string) (AdmissionCapacitySnapshot, error) {
			return AdmissionCapacitySnapshot{}, errors.New("capacity snapshot unavailable")
		}),
	)
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        913,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	_, err = session.NextTarget(ctx, GatewayTargetRequest{Model: "claude-3-5-sonnet-20241022"})

	var unavailable *ExtraConcurrencyUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.False(t, errors.Is(err, context.DeadlineExceeded))
}

func TestGatewayAdmissionSessionStandardUsesTargetConcurrencyWhenCapacityUnavailable(t *testing.T) {
	const (
		accountID          int64 = 47
		accountConcurrency       = 3
	)
	store := &targetConcurrencyAdmissionStore{
		gatewayAdmissionSessionStoreStub: &gatewayAdmissionSessionStoreStub{
			userResult: UserLeaseResult{Acquired: true, Class: AdmissionClassStandard},
		},
		expectedAccountConcurrency: accountConcurrency,
	}
	admission := NewGatewayAdmission(
		store,
		nil,
		admissionCapacitySourceFunc(func(context.Context, string) (AdmissionCapacitySnapshot, error) {
			return AdmissionCapacitySnapshot{}, errors.New("capacity snapshot unavailable")
		}),
	)
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        914,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	target, err := session.NextTarget(ctx, GatewayTargetRequest{
		Selector: GatewayTargetSelectorFunc(func(ctx context.Context, claimer TargetClaimer) (*AccountSelectionResult, error) {
			account := &Account{ID: accountID, Platform: PlatformAnthropic, Concurrency: accountConcurrency}
			claim, claimErr := tryClaimTarget(ctx, claimer, account)
			if claimErr != nil {
				return nil, claimErr
			}
			return &AccountSelectionResult{
				Account:     account,
				Acquired:    claim.Acquired,
				ReleaseFunc: claim.ReleaseFunc,
			}, nil
		}),
	})

	require.NoError(t, err)
	require.NotNil(t, target)
	require.Equal(t, accountID, target.Account.ID)
	require.Equal(t, AdmissionClassStandard, target.Class)
}

func TestGatewayAdmissionSessionCancelWaitingTargetRemovesQueueEntry(t *testing.T) {
	account := Account{
		ID:          45,
		Platform:    PlatformAnthropic,
		Priority:    1,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{account},
		accountsByID: map[int64]*Account{account.ID: &account},
	}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	gatewayService := &GatewayService{
		accountRepo:        repo,
		cache:              &mockGatewayCacheForPlatform{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(&mockConcurrencyCache{}),
	}
	store := &gatewayAdmissionSessionStoreStub{
		userResult:      UserLeaseResult{Acquired: true, Class: AdmissionClassStandard},
		targetBlocked:   true,
		targetAttempted: make(chan struct{}, 1),
	}
	admission := NewGatewayAdmission(store, gatewayService, gatewayService)
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        912,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, nextErr := session.NextTarget(ctx, GatewayTargetRequest{
			Model: "claude-3-5-sonnet-20241022",
		})
		done <- nextErr
	}()
	<-store.targetAttempted
	cancel()

	require.ErrorIs(t, <-done, context.Canceled)
	require.Equal(t, int32(1), store.targetReleaseCalls.Load())
}

func TestGatewayAdmissionSessionCloseReleasesTargetAcquiredConcurrently(t *testing.T) {
	account := Account{
		ID:          47,
		Platform:    PlatformAnthropic,
		Priority:    1,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{account},
		accountsByID: map[int64]*Account{account.ID: &account},
	}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	gatewayService := &GatewayService{
		accountRepo:        repo,
		cache:              &mockGatewayCacheForPlatform{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(&mockConcurrencyCache{}),
	}
	store := &gatewayAdmissionSessionStoreStub{
		userResult:         UserLeaseResult{Acquired: true, Class: AdmissionClassStandard},
		targetAcquireReady: make(chan struct{}),
		targetAcquireStart: make(chan struct{}, 1),
	}
	admission := NewGatewayAdmission(store, gatewayService, gatewayService)
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        914,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)

	targets := make(chan *AdmittedTarget, 1)
	errs := make(chan error, 1)
	go func() {
		target, nextErr := session.NextTarget(context.Background(), GatewayTargetRequest{
			Model: "claude-3-5-sonnet-20241022",
		})
		targets <- target
		errs <- nextErr
	}()
	<-store.targetAcquireStart

	session.Close()
	close(store.targetAcquireReady)

	require.Error(t, <-errs)
	require.Nil(t, <-targets)
	require.Equal(t, int32(1), store.targetReleaseCalls.Load())
	require.Equal(t, int32(1), store.releaseCalls.Load())
}

func TestGatewayAdmissionSessionCloseReleasesUserLeaseRefreshedConcurrently(t *testing.T) {
	account := Account{
		ID:          48,
		Platform:    PlatformAnthropic,
		Priority:    1,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{account},
		accountsByID: map[int64]*Account{account.ID: &account},
	}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	gatewayService := &GatewayService{
		accountRepo:        repo,
		cache:              &mockGatewayCacheForPlatform{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(&mockConcurrencyCache{}),
	}
	store := &gatewayAdmissionSessionStoreStub{
		userResult:       UserLeaseResult{Acquired: true, Class: AdmissionClassStandard},
		targetBlocked:    true,
		userAcquireReady: make(chan struct{}),
		userAcquireStart: make(chan struct{}, 1),
	}
	admission := NewGatewayAdmission(store, gatewayService, gatewayService)
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        915,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, nextErr := session.NextTarget(context.Background(), GatewayTargetRequest{
			Model: "claude-3-5-sonnet-20241022",
		})
		done <- nextErr
	}()
	<-store.userAcquireStart

	session.Close()
	close(store.userAcquireReady)

	require.ErrorIs(t, <-done, context.Canceled)
	require.Equal(t, int32(2), store.releaseCalls.Load())
}

func newAdmittedTargetForDispatchTest(t *testing.T) (*AdmittedTarget, *GatewayAdmissionSession, *gatewayAdmissionSessionStoreStub) {
	t.Helper()

	account := Account{
		ID:          44,
		Platform:    PlatformAnthropic,
		Priority:    1,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{account},
		accountsByID: map[int64]*Account{account.ID: &account},
	}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	gatewayService := &GatewayService{
		accountRepo:        repo,
		cache:              &mockGatewayCacheForPlatform{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(&mockConcurrencyCache{}),
	}
	store := &gatewayAdmissionSessionStoreStub{
		userResult: UserLeaseResult{Acquired: true, Class: AdmissionClassExtra},
	}
	admission := NewGatewayAdmission(store, gatewayService, gatewayService)
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        911,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)
	target, err := session.NextTarget(context.Background(), GatewayTargetRequest{
		Model: "claude-3-5-sonnet-20241022",
	})
	require.NoError(t, err)
	return target, session, store
}

func TestAdmittedTargetDispatchRechecksBeforeUpstreamAndReleases(t *testing.T) {
	target, session, store := newAdmittedTargetForDispatchTest(t)
	defer session.Close()
	session.waited.Store(true)

	eligibilityErr := errors.New("balance changed while waiting")
	recheckCalls := 0
	upstreamCalls := 0
	err := target.Dispatch(
		context.Background(),
		func(context.Context) error {
			recheckCalls++
			return eligibilityErr
		},
		func(context.Context, *Account) error {
			upstreamCalls++
			return nil
		},
	)

	require.ErrorIs(t, err, eligibilityErr)
	require.Equal(t, 1, recheckCalls)
	require.Zero(t, upstreamCalls)
	require.Equal(t, int32(1), store.targetReleaseCalls.Load())
}

func TestAdmittedTargetDispatchPreparedRechecksWhenPreparationRequiresIt(t *testing.T) {
	target, session, _ := newAdmittedTargetForDispatchTest(t)
	defer session.Close()

	eligibilityErr := errors.New("eligibility changed during target preparation")
	recheckCalls := 0
	upstreamCalls := 0
	handled, err := target.DispatchPrepared(
		context.Background(),
		func(context.Context, *Account) (GatewayTargetPreparation, error) {
			return GatewayTargetPreparation{Recheck: true}, nil
		},
		func(context.Context) error {
			recheckCalls++
			return eligibilityErr
		},
		func(context.Context, *Account) error {
			upstreamCalls++
			return nil
		},
	)

	require.ErrorIs(t, err, eligibilityErr)
	require.False(t, handled)
	require.Equal(t, 1, recheckCalls)
	require.Zero(t, upstreamCalls)
}

func TestAdmittedTargetDispatchPreparedRetargetsWhenDrainStartsDuringRecheck(t *testing.T) {
	target, session, store := newAdmittedTargetForDispatchTest(t)
	defer session.Close()
	session.waited.Store(true)
	store.userResults = []UserLeaseResult{
		{Acquired: true, Class: AdmissionClassExtra},
		{Acquired: true, Class: AdmissionClassStandard},
	}

	var draining atomic.Bool
	store.targetDispatchFunc = func(request TargetDispatchRequest) (TargetDispatchResult, error) {
		if request.Class == AdmissionClassExtra && draining.Load() {
			return TargetDispatchResult{Draining: true}, nil
		}
		return TargetDispatchResult{Started: true}, nil
	}
	prepareCalls := 0
	recheckCalls := 0
	upstreamCalls := 0
	handled, err := target.DispatchPrepared(
		context.Background(),
		func(context.Context, *Account) (GatewayTargetPreparation, error) {
			prepareCalls++
			return GatewayTargetPreparation{}, nil
		},
		func(context.Context) error {
			recheckCalls++
			draining.Store(true)
			return nil
		},
		func(context.Context, *Account) error {
			upstreamCalls++
			return nil
		},
	)

	require.NoError(t, err)
	require.False(t, handled)
	require.Equal(t, 2, prepareCalls)
	require.Equal(t, 2, recheckCalls)
	require.Equal(t, 1, upstreamCalls)
	require.Equal(t, int32(2), store.targetDispatchCalls.Load())
	require.Equal(t, AdmissionClassStandard, target.Class)
}

func TestAdmittedTargetDispatchPreparedCleansPreparationWhenRecheckPanics(t *testing.T) {
	target, session, _ := newAdmittedTargetForDispatchTest(t)
	defer session.Close()
	session.waited.Store(true)

	cleanupCalls := 0
	upstreamCalls := 0
	require.PanicsWithValue(t, "recheck panic", func() {
		_, _ = target.DispatchPrepared(
			context.Background(),
			func(context.Context, *Account) (GatewayTargetPreparation, error) {
				return GatewayTargetPreparation{Cleanup: func() { cleanupCalls++ }}, nil
			},
			func(context.Context) error {
				panic("recheck panic")
			},
			func(context.Context, *Account) error {
				upstreamCalls++
				return nil
			},
		)
	})

	require.Equal(t, 1, cleanupCalls)
	require.Zero(t, upstreamCalls)
}

func TestAdmittedTargetDispatchCanOnlyRunOnce(t *testing.T) {
	target, session, store := newAdmittedTargetForDispatchTest(t)
	defer session.Close()

	upstreamCalls := 0
	dispatch := func(context.Context, *Account) error {
		upstreamCalls++
		return nil
	}
	require.NoError(t, target.Dispatch(context.Background(), nil, dispatch))

	err := target.Dispatch(context.Background(), nil, dispatch)

	require.Error(t, err)
	require.Equal(t, 1, upstreamCalls)
	require.Equal(t, int32(1), store.targetReleaseCalls.Load())
}

func TestAdmittedTargetStandardDispatchRejectsLostTargetLeaseBeforeUpstream(t *testing.T) {
	const accountID int64 = 46
	store := &gatewayAdmissionSessionStoreStub{
		userResult:            UserLeaseResult{Acquired: true, Class: AdmissionClassStandard},
		targetDispatchResults: []TargetDispatchResult{{}},
	}
	admission := NewGatewayAdmission(
		store,
		nil,
		admissionCapacitySourceFunc(func(context.Context, string) (AdmissionCapacitySnapshot, error) {
			return AdmissionCapacitySnapshot{
				TotalConcurrency:   1,
				AccountConcurrency: map[int64]int{accountID: 1},
			}, nil
		}),
	)
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        921,
		StandardLimit: 1,
		Settings: ExtraConcurrencyRuntimeSettings{
			Enabled:            true,
			WaitTimeoutSeconds: 1,
		},
	})
	require.NoError(t, err)
	defer session.Close()
	target, err := session.NextTarget(context.Background(), GatewayTargetRequest{
		Selector: GatewayTargetSelectorFunc(func(ctx context.Context, claimer TargetClaimer) (*AccountSelectionResult, error) {
			account := &Account{ID: accountID, Platform: PlatformAnthropic, Concurrency: 1}
			release, acquired, claimErr := claimer.TryClaim(ctx, TargetClaimRequest{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountConcurrency: account.Concurrency,
			})
			return &AccountSelectionResult{Account: account, Acquired: acquired, ReleaseFunc: release}, claimErr
		}),
	})
	require.NoError(t, err)

	upstreamCalls := 0
	err = target.Dispatch(context.Background(), nil, func(context.Context, *Account) error {
		upstreamCalls++
		return nil
	})

	require.ErrorContains(t, err, "target lease was lost before dispatch")
	require.Zero(t, upstreamCalls)
	require.Equal(t, int32(1), store.targetDispatchCalls.Load())
}

func TestAdmittedTargetBeginAttemptRenewsUntilFinishedWithoutReleasingTarget(t *testing.T) {
	target, session, store := newAdmittedTargetForDispatchTest(t)
	defer session.Close()
	session.admission.renewInterval = 10 * time.Millisecond

	finish, err := target.BeginAttempt(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, finish)
	require.Eventually(t, func() bool {
		return store.renewUserCalls.Load() > 0 && store.renewTargetCalls.Load() > 0
	}, time.Second, 10*time.Millisecond)
	require.Zero(t, store.targetReleaseCalls.Load())

	finish()
	finish()
	userRenewals := store.renewUserCalls.Load()
	targetRenewals := store.renewTargetCalls.Load()
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, userRenewals, store.renewUserCalls.Load())
	require.Equal(t, targetRenewals, store.renewTargetCalls.Load())
	require.Zero(t, store.targetReleaseCalls.Load())

	session.ReleaseTarget()
	require.Equal(t, int32(1), store.targetReleaseCalls.Load())
	_, err = target.BeginAttempt(context.Background(), nil)
	require.Error(t, err)
}

func TestAdmittedTargetDispatchFallsBackDisabledExtraBeforeUpstream(t *testing.T) {
	target, session, store := newAdmittedTargetForDispatchTest(t)
	defer session.Close()
	session.admission.SetExtraConcurrencyRuntimeSettingsSource(extraConcurrencyRuntimeSettingsSourceFunc(
		func(context.Context) ExtraConcurrencyRuntimeSettings {
			return ExtraConcurrencyRuntimeSettings{Enabled: false, WaitTimeoutSeconds: 1}
		},
	))
	store.userResults = []UserLeaseResult{
		{Acquired: true, Class: AdmissionClassExtra},
		{Acquired: true, Class: AdmissionClassStandard},
	}
	store.userAcquireReady = make(chan struct{})
	store.userAcquireStart = make(chan struct{}, 1)
	defer func() {
		select {
		case <-store.userAcquireReady:
		default:
			close(store.userAcquireReady)
		}
	}()
	upstreamStarted := make(chan struct{}, 1)
	dispatchDone := make(chan error, 1)
	go func() {
		dispatchDone <- target.Dispatch(
			context.Background(),
			nil,
			func(context.Context, *Account) error {
				upstreamStarted <- struct{}{}
				return nil
			},
		)
	}()

	select {
	case <-store.userAcquireStart:
	case <-upstreamStarted:
		t.Fatal("upstream started before disabled extra admission fell back to standard")
	case err := <-dispatchDone:
		t.Fatalf("dispatch finished before disabled extra admission fell back to standard: %v", err)
	case <-time.After(time.Second):
		t.Fatal("dispatch did not attempt to fall back disabled extra admission")
	}
	require.Equal(t, int32(1), store.targetReleaseCalls.Load(),
		"disabled extra admission must release its target before waiting for standard concurrency")
	select {
	case <-upstreamStarted:
		t.Fatal("upstream started while standard fallback was blocked")
	default:
	}
	close(store.userAcquireReady)
	require.NoError(t, <-dispatchDone)
	select {
	case <-upstreamStarted:
	default:
		t.Fatal("upstream did not start after standard fallback completed")
	}
	require.Equal(t, AdmissionClassStandard, session.Class())
	require.Equal(t, AdmissionClassStandard, target.Class)
	require.Equal(t, int32(2), store.userAcquireCalls.Load())
	require.Equal(t, int32(2), store.targetAcquireCalls.Load(),
		"the converted request must reacquire a target under standard admission")
	require.Equal(t, int32(2), store.targetReleaseCalls.Load())
	require.Zero(t, store.userRequest.ExtraLimit)
}

func TestAdmittedTargetDispatchFallsBackRedisDrainWithStaleLocalSettings(t *testing.T) {
	target, session, store := newAdmittedTargetForDispatchTest(t)
	defer session.Close()
	store.targetDispatch = TargetDispatchResult{Draining: true}
	store.userResults = []UserLeaseResult{
		{Acquired: true, Class: AdmissionClassExtra},
		{Acquired: true, Class: AdmissionClassStandard},
	}
	upstreamCalls := 0

	err := target.Dispatch(context.Background(), nil, func(context.Context, *Account) error {
		upstreamCalls++
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 1, upstreamCalls)
	require.Equal(t, AdmissionClassStandard, session.Class())
	require.Equal(t, AdmissionClassStandard, target.Class)
	require.Equal(t, int32(2), store.userAcquireCalls.Load())
	require.Equal(t, int32(2), store.targetAcquireCalls.Load())
	require.Zero(t, store.userRequest.ExtraLimit)
}

func TestAdmittedTargetDispatchRevalidatesUpdatedReserveBeforeUpstream(t *testing.T) {
	const accountID int64 = 44
	currentSettings := ExtraConcurrencyRuntimeSettings{
		Enabled:            true,
		WaitTimeoutSeconds: 1,
		ReservePercent:     0,
		PlatformReserves:   map[string]ExtraConcurrencyPlatformReserve{},
	}
	store := &gatewayAdmissionSessionStoreStub{
		userResult: UserLeaseResult{Acquired: true, Class: AdmissionClassExtra},
	}
	admission := NewGatewayAdmission(
		store,
		nil,
		admissionCapacitySourceFunc(func(context.Context, string) (AdmissionCapacitySnapshot, error) {
			return AdmissionCapacitySnapshot{
				TotalConcurrency:   10,
				AccountConcurrency: map[int64]int{accountID: 10},
			}, nil
		}),
	)
	admission.SetExtraConcurrencyRuntimeSettingsSource(extraConcurrencyRuntimeSettingsSourceFunc(
		func(context.Context) ExtraConcurrencyRuntimeSettings {
			return currentSettings
		},
	))
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        918,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings:      currentSettings,
	})
	require.NoError(t, err)
	defer session.Close()

	target, err := session.NextTarget(context.Background(), GatewayTargetRequest{
		Selector: GatewayTargetSelectorFunc(func(ctx context.Context, claimer TargetClaimer) (*AccountSelectionResult, error) {
			account := &Account{ID: accountID, Platform: PlatformAnthropic, Concurrency: 10}
			release, acquired, claimErr := claimer.TryClaim(ctx, TargetClaimRequest{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountConcurrency: account.Concurrency,
			})
			return &AccountSelectionResult{Account: account, Acquired: acquired, ReleaseFunc: release}, claimErr
		}),
	})
	require.NoError(t, err)
	require.Zero(t, store.targetRequest.ReservedSlots)
	require.Equal(t, int32(1), store.targetAcquireCalls.Load())

	currentSettings.ReservePercent = 100
	store.targetBlocked = true
	upstreamCalls := 0
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	err = target.Dispatch(ctx, nil, func(context.Context, *Account) error {
		upstreamCalls++
		return nil
	})

	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Zero(t, upstreamCalls)
	require.Greater(t, store.targetAcquireCalls.Load(), int32(1))
	require.Equal(t, 10, store.targetRequest.ReservedSlots)
}

func TestAdmittedTargetDispatchReconfirmsReplacementExtraAtDispatchBoundary(t *testing.T) {
	const accountID int64 = 45
	currentSettings := ExtraConcurrencyRuntimeSettings{
		Enabled:            true,
		WaitTimeoutSeconds: 1,
		ReservePercent:     0,
		PlatformReserves:   map[string]ExtraConcurrencyPlatformReserve{},
	}
	store := &gatewayAdmissionSessionStoreStub{
		userResult: UserLeaseResult{Acquired: true, Class: AdmissionClassExtra},
	}
	admission := NewGatewayAdmission(
		store,
		nil,
		admissionCapacitySourceFunc(func(context.Context, string) (AdmissionCapacitySnapshot, error) {
			return AdmissionCapacitySnapshot{
				TotalConcurrency:   10,
				AccountConcurrency: map[int64]int{accountID: 10},
			}, nil
		}),
	)
	admission.SetExtraConcurrencyRuntimeSettingsSource(extraConcurrencyRuntimeSettingsSourceFunc(
		func(context.Context) ExtraConcurrencyRuntimeSettings { return currentSettings },
	))
	session, err := admission.Begin(context.Background(), GatewayAdmissionRequest{
		UserID:        920,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings:      currentSettings,
	})
	require.NoError(t, err)
	defer session.Close()

	target, err := session.NextTarget(context.Background(), GatewayTargetRequest{
		Selector: GatewayTargetSelectorFunc(func(ctx context.Context, claimer TargetClaimer) (*AccountSelectionResult, error) {
			account := &Account{ID: accountID, Platform: PlatformAnthropic, Concurrency: 10}
			release, acquired, claimErr := claimer.TryClaim(ctx, TargetClaimRequest{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountConcurrency: account.Concurrency,
			})
			return &AccountSelectionResult{Account: account, Acquired: acquired, ReleaseFunc: release}, claimErr
		}),
	})
	require.NoError(t, err)

	currentSettings.ReservePercent = 20
	upstreamCalls := 0
	err = target.Dispatch(context.Background(), nil, func(context.Context, *Account) error {
		upstreamCalls++
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 1, upstreamCalls)
	require.Equal(t, int32(2), store.targetAcquireCalls.Load())
	require.Equal(t, int32(2), store.targetDispatchCalls.Load(),
		"the replacement extra target must establish its own drain ordering point")
}

func TestAdmittedTargetDispatchPreparedCleansAndRepreparesAfterDrainRetarget(t *testing.T) {
	target, session, store := newAdmittedTargetForDispatchTest(t)
	defer session.Close()
	store.targetDispatchResults = []TargetDispatchResult{
		{Draining: true},
		{Started: true},
	}
	store.userResults = []UserLeaseResult{
		{Acquired: true, Class: AdmissionClassExtra},
		{Acquired: true, Class: AdmissionClassStandard},
	}

	prepareCalls := 0
	cleanupCalls := 0
	upstreamCalls := 0
	handled, err := target.DispatchPrepared(
		context.Background(),
		func(context.Context, *Account) (GatewayTargetPreparation, error) {
			prepareCalls++
			return GatewayTargetPreparation{Cleanup: func() { cleanupCalls++ }}, nil
		},
		nil,
		func(context.Context, *Account) error {
			upstreamCalls++
			return nil
		},
	)

	require.NoError(t, err)
	require.False(t, handled)
	require.Equal(t, 2, prepareCalls)
	require.Equal(t, 2, cleanupCalls)
	require.Equal(t, 1, upstreamCalls)
	require.Equal(t, int32(2), store.targetDispatchCalls.Load())
	require.Equal(t, AdmissionClassStandard, target.Class)
}

func TestAdmittedTargetDispatchRenewsLeasesWhileUpstreamIsRunning(t *testing.T) {
	target, session, store := newAdmittedTargetForDispatchTest(t)
	defer session.Close()
	session.admission.renewInterval = 10 * time.Millisecond

	upstreamStarted := make(chan struct{})
	finishUpstream := make(chan struct{})
	dispatchDone := make(chan error, 1)
	go func() {
		dispatchDone <- target.Dispatch(
			context.Background(),
			nil,
			func(context.Context, *Account) error {
				close(upstreamStarted)
				<-finishUpstream
				return nil
			},
		)
	}()
	<-upstreamStarted

	require.Eventually(t, func() bool {
		return store.renewUserCalls.Load() > 0 && store.renewTargetCalls.Load() > 0
	}, time.Second, 10*time.Millisecond)
	close(finishUpstream)
	require.NoError(t, <-dispatchDone)
	require.Equal(t, int32(1), store.targetReleaseCalls.Load())
}

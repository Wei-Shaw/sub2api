//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type admissionCapacitySourceFunc func(context.Context, string) (AdmissionCapacitySnapshot, error)

func (f admissionCapacitySourceFunc) AdmissionCapacity(ctx context.Context, platform string) (AdmissionCapacitySnapshot, error) {
	return f(ctx, platform)
}

type recordingGatewayAdmissionStore struct {
	GatewayAdmissionStore
	targetRequest TargetLeaseRequest
	targetResult  TargetLeaseResult
	releaseCalls  int
}

type switchingGatewayAdmissionStore struct {
	GatewayAdmissionStore
	results        map[string]TargetLeaseResult
	accountResults map[int64]TargetLeaseResult
	released       []TargetClaimRequest
}

func (s *switchingGatewayAdmissionStore) TryAcquireTargetLease(_ context.Context, request TargetLeaseRequest) (TargetLeaseResult, error) {
	if result, ok := s.accountResults[request.AccountID]; ok {
		return result, nil
	}
	return s.results[request.Platform], nil
}

func (s *switchingGatewayAdmissionStore) ReleaseTargetLease(_ context.Context, platform string, accountID int64, _ string) error {
	s.released = append(s.released, TargetClaimRequest{Platform: platform, AccountID: accountID})
	return nil
}

func (s *recordingGatewayAdmissionStore) TryAcquireTargetLease(_ context.Context, request TargetLeaseRequest) (TargetLeaseResult, error) {
	s.targetRequest = request
	if s.targetResult != (TargetLeaseResult{}) {
		return s.targetResult, nil
	}
	return TargetLeaseResult{Acquired: true}, nil
}

func (s *recordingGatewayAdmissionStore) ReleaseTargetLease(context.Context, string, int64, string) error {
	s.releaseCalls++
	return nil
}

func TestGatewayAdmissionTargetClaimerUsesAuthoritativeCapacityAndReserveOverride(t *testing.T) {
	store := &recordingGatewayAdmissionStore{}
	reservePercent := 30.0
	minReservedSlots := 2
	claimer := &gatewayAdmissionTargetClaimer{
		store: store,
		capacitySource: admissionCapacitySourceFunc(func(_ context.Context, platform string) (AdmissionCapacitySnapshot, error) {
			require.Equal(t, PlatformAnthropic, platform)
			return AdmissionCapacitySnapshot{
				TotalConcurrency:   10,
				AccountConcurrency: map[int64]int{42: 3},
			}, nil
		}),
		requestID: "request-42",
		class:     AdmissionClassExtra,
		settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 45,
			ReservePercent:     10,
			MinReservedSlots:   1,
			PlatformReserves: map[string]ExtraConcurrencyPlatformReserve{
				PlatformAnthropic: {
					ReservePercent:   &reservePercent,
					MinReservedSlots: &minReservedSlots,
				},
			},
		},
	}

	release, claimed, err := claimer.TryClaim(context.Background(), TargetClaimRequest{
		Platform:           PlatformAnthropic,
		AccountID:          42,
		AccountConcurrency: 99,
	})

	require.NoError(t, err)
	require.True(t, claimed)
	require.NotNil(t, release)
	require.Equal(t, TargetLeaseRequest{
		RequestID:        "request-42",
		Platform:         PlatformAnthropic,
		AccountID:        42,
		AccountLimit:     3,
		PlatformCapacity: 10,
		ReservedSlots:    3,
		Class:            AdmissionClassExtra,
		WaitTimeout:      45 * time.Second,
	}, store.targetRequest)

	release()
	require.Equal(t, 1, store.releaseCalls)
}

func TestGatewayAdmissionTargetClaimerPreservesUnlimitedAccountSemantics(t *testing.T) {
	store := &recordingGatewayAdmissionStore{}
	capacityCalls := 0
	claimer := &gatewayAdmissionTargetClaimer{
		store: store,
		capacitySource: admissionCapacitySourceFunc(func(context.Context, string) (AdmissionCapacitySnapshot, error) {
			capacityCalls++
			return AdmissionCapacitySnapshot{}, nil
		}),
		requestID: "unlimited-account",
		class:     AdmissionClassExtra,
	}

	release, claimed, err := claimer.TryClaim(context.Background(), TargetClaimRequest{
		Platform:           PlatformAnthropic,
		AccountID:          99,
		AccountConcurrency: 0,
	})

	require.NoError(t, err)
	require.True(t, claimed)
	require.NotNil(t, release)
	require.Zero(t, capacityCalls)
	require.Equal(t, "unlimited-account", store.targetRequest.RequestID)
	require.Equal(t, int64(99), store.targetRequest.AccountID)
	require.True(t, store.targetRequest.Unlimited)

	release()
	require.Equal(t, 1, store.releaseCalls)
}

func TestGatewayAdmissionTargetClaimerStandardTimeoutUsesStandardError(t *testing.T) {
	store := &recordingGatewayAdmissionStore{
		targetResult: TargetLeaseResult{Expired: true},
	}
	claimer := &gatewayAdmissionTargetClaimer{
		store: store,
		capacitySource: admissionCapacitySourceFunc(func(context.Context, string) (AdmissionCapacitySnapshot, error) {
			return AdmissionCapacitySnapshot{
				TotalConcurrency:   1,
				AccountConcurrency: map[int64]int{42: 1},
			}, nil
		}),
		requestID: "standard-timeout",
		class:     AdmissionClassStandard,
		settings: ExtraConcurrencyRuntimeSettings{
			WaitTimeoutSeconds: 1,
		},
	}

	_, claimed, err := claimer.TryClaim(context.Background(), TargetClaimRequest{
		Platform:           PlatformAnthropic,
		AccountID:          42,
		AccountConcurrency: 1,
	})

	require.NoError(t, err)
	require.False(t, claimed)
	var timeout *GatewayAdmissionTimeoutError
	require.ErrorAs(t, claimer.Err(), &timeout)
	require.Equal(t, "account", timeout.SlotType)
}

func TestGatewayAdmissionTargetClaimerReleasesPendingTargetBeforeSwitchingPlatform(t *testing.T) {
	store := &switchingGatewayAdmissionStore{
		results: map[string]TargetLeaseResult{
			PlatformAnthropic:   {},
			PlatformAntigravity: {Acquired: true},
		},
	}
	claimer := &gatewayAdmissionTargetClaimer{
		store: store,
		capacitySource: admissionCapacitySourceFunc(func(_ context.Context, platform string) (AdmissionCapacitySnapshot, error) {
			accountID := int64(1)
			if platform == PlatformAntigravity {
				accountID = 2
			}
			return AdmissionCapacitySnapshot{
				TotalConcurrency:   1,
				AccountConcurrency: map[int64]int{accountID: 1},
			}, nil
		}),
		requestID: "cross-platform-request",
		class:     AdmissionClassStandard,
	}

	_, claimed, err := claimer.TryClaim(context.Background(), TargetClaimRequest{
		Platform:           PlatformAnthropic,
		AccountID:          1,
		AccountConcurrency: 1,
	})
	require.NoError(t, err)
	require.False(t, claimed)

	release, claimed, err := claimer.TryClaim(context.Background(), TargetClaimRequest{
		Platform:           PlatformAntigravity,
		AccountID:          2,
		AccountConcurrency: 1,
	})
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, []TargetClaimRequest{{Platform: PlatformAnthropic, AccountID: 1}}, store.released)

	release()
	require.Equal(t, []TargetClaimRequest{
		{Platform: PlatformAnthropic, AccountID: 1},
		{Platform: PlatformAntigravity, AccountID: 2},
	}, store.released)
}

func TestGatewayAdmissionTargetClaimerReleasesPendingWhenNewPlatformHasNoCapacity(t *testing.T) {
	store := &switchingGatewayAdmissionStore{
		results: map[string]TargetLeaseResult{
			PlatformAnthropic: {},
		},
	}
	claimer := &gatewayAdmissionTargetClaimer{
		store: store,
		capacitySource: admissionCapacitySourceFunc(func(_ context.Context, platform string) (AdmissionCapacitySnapshot, error) {
			if platform == PlatformAnthropic {
				return AdmissionCapacitySnapshot{
					TotalConcurrency:   1,
					AccountConcurrency: map[int64]int{1: 1},
				}, nil
			}
			return AdmissionCapacitySnapshot{}, nil
		}),
		requestID: "cross-platform-no-capacity",
		class:     AdmissionClassExtra,
	}

	_, claimed, err := claimer.TryClaim(context.Background(), TargetClaimRequest{
		Platform:           PlatformAnthropic,
		AccountID:          1,
		AccountConcurrency: 1,
	})
	require.NoError(t, err)
	require.False(t, claimed)

	_, claimed, err = claimer.TryClaim(context.Background(), TargetClaimRequest{
		Platform:           PlatformAntigravity,
		AccountID:          2,
		AccountConcurrency: 1,
	})
	require.NoError(t, err)
	require.False(t, claimed)
	require.Equal(t, []TargetClaimRequest{{Platform: PlatformAnthropic, AccountID: 1}}, store.released)
}

func TestGatewayAdmissionTargetClaimerPreservesPendingQueueWhenSwitchingAccountOnSamePlatform(t *testing.T) {
	store := &switchingGatewayAdmissionStore{
		accountResults: map[int64]TargetLeaseResult{
			1: {},
			2: {Acquired: true},
		},
	}
	claimer := &gatewayAdmissionTargetClaimer{
		store: store,
		capacitySource: admissionCapacitySourceFunc(func(context.Context, string) (AdmissionCapacitySnapshot, error) {
			return AdmissionCapacitySnapshot{
				TotalConcurrency:   2,
				AccountConcurrency: map[int64]int{1: 1, 2: 1},
			}, nil
		}),
		requestID: "same-platform-request",
		class:     AdmissionClassStandard,
	}

	_, claimed, err := claimer.TryClaim(context.Background(), TargetClaimRequest{
		Platform:           PlatformAnthropic,
		AccountID:          1,
		AccountConcurrency: 1,
	})
	require.NoError(t, err)
	require.False(t, claimed)

	release, claimed, err := claimer.TryClaim(context.Background(), TargetClaimRequest{
		Platform:           PlatformAnthropic,
		AccountID:          2,
		AccountConcurrency: 1,
	})
	require.NoError(t, err)
	require.True(t, claimed)
	require.Empty(t, store.released)

	release()
	require.Equal(t, []TargetClaimRequest{{Platform: PlatformAnthropic, AccountID: 2}}, store.released)
}

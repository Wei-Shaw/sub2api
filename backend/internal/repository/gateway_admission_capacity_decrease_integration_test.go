//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type mutableAdmissionCapacity struct {
	mu       sync.RWMutex
	snapshot service.AdmissionCapacitySnapshot
}

func (c *mutableAdmissionCapacity) AdmissionCapacity(context.Context, string) (service.AdmissionCapacitySnapshot, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snapshot, nil
}

func (c *mutableAdmissionCapacity) set(snapshot service.AdmissionCapacitySnapshot) {
	c.mu.Lock()
	c.snapshot = snapshot
	c.mu.Unlock()
}

func TestGatewayAdmissionCapacityDecreaseKeepsInflightExtraAndRejectsNewExtra(t *testing.T) {
	const (
		accountID int64 = 7331
		userID    int64 = 8101
	)
	store := NewGatewayAdmissionStore(testRedis(t), time.Minute)
	capacity := &mutableAdmissionCapacity{snapshot: service.AdmissionCapacitySnapshot{
		TotalConcurrency:   3,
		AccountConcurrency: map[int64]int{accountID: 3},
	}}
	admission := service.NewGatewayAdmission(store, nil, capacity)
	settings := service.ExtraConcurrencyRuntimeSettings{
		Enabled:            true,
		WaitTimeoutSeconds: 1,
		MinReservedSlots:   1,
		PlatformReserves:   map[string]service.ExtraConcurrencyPlatformReserve{},
	}
	account := &service.Account{
		ID:          accountID,
		Platform:    service.PlatformAnthropic,
		Concurrency: 3,
	}
	selector := service.GatewayTargetSelectorFunc(func(ctx context.Context, claimer service.TargetClaimer) (*service.AccountSelectionResult, error) {
		release, acquired, err := claimer.TryClaim(ctx, service.TargetClaimRequest{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountConcurrency: account.Concurrency,
		})
		return &service.AccountSelectionResult{Account: account, Acquired: acquired, ReleaseFunc: release}, err
	})

	standardHolder, err := admission.Begin(t.Context(), service.GatewayAdmissionRequest{
		UserID:        userID,
		StandardLimit: 1,
		ExtraLimit:    3,
		Settings:      settings,
	})
	require.NoError(t, err)
	require.Equal(t, service.AdmissionClassStandard, standardHolder.Class())
	t.Cleanup(standardHolder.Close)

	beginExtra := func() (*service.GatewayAdmissionSession, *service.AdmittedTarget) {
		t.Helper()
		session, err := admission.Begin(t.Context(), service.GatewayAdmissionRequest{
			UserID:        userID,
			StandardLimit: 1,
			ExtraLimit:    3,
			Settings:      settings,
		})
		require.NoError(t, err)
		t.Cleanup(session.Close)
		target, err := session.NextTarget(t.Context(), service.GatewayTargetRequest{Selector: selector})
		require.NoError(t, err)
		return session, target
	}

	_, first := beginExtra()
	_, second := beginExtra()
	releaseInflight := make(chan struct{})
	started := make(chan struct{}, 2)
	canceled := make(chan error, 2)
	dispatch := func(target *service.AdmittedTarget) <-chan error {
		done := make(chan error, 1)
		go func() {
			done <- target.Dispatch(t.Context(), nil, func(ctx context.Context, _ *service.Account) error {
				started <- struct{}{}
				select {
				case <-releaseInflight:
					return nil
				case <-ctx.Done():
					canceled <- ctx.Err()
					return ctx.Err()
				}
			})
		}()
		return done
	}
	firstDone := dispatch(first)
	secondDone := dispatch(second)
	<-started
	<-started

	capacity.set(service.AdmissionCapacitySnapshot{
		TotalConcurrency:   1,
		AccountConcurrency: map[int64]int{accountID: 1},
	})
	thirdSession, err := admission.Begin(t.Context(), service.GatewayAdmissionRequest{
		UserID:        userID,
		StandardLimit: 1,
		ExtraLimit:    3,
		Settings:      settings,
	})
	require.NoError(t, err)
	t.Cleanup(thirdSession.Close)
	_, err = thirdSession.NextTarget(t.Context(), service.GatewayTargetRequest{Selector: selector})
	var unavailable *service.ExtraConcurrencyUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.True(t, unavailable.Timeout)
	select {
	case err := <-canceled:
		t.Fatalf("capacity decrease canceled an in-flight extra request: %v", err)
	default:
	}

	close(releaseInflight)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)
}

//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type admissionCapacityTTLCache struct {
	*snapshotHydrationCache
	lockTTL  time.Duration
	snapshot AdmissionCapacitySnapshot
}

func (c *admissionCapacityTTLCache) TryLockBucket(_ context.Context, _ SchedulerBucket, ttl time.Duration) (bool, error) {
	c.lockTTL = ttl
	return true, nil
}

func (c *admissionCapacityTTLCache) GetAdmissionCapacity(_ context.Context, _ string) (AdmissionCapacitySnapshot, bool, error) {
	return c.snapshot, true, nil
}

func (c *admissionCapacityTTLCache) SetAdmissionCapacity(_ context.Context, _ string, snapshot AdmissionCapacitySnapshot) error {
	c.snapshot = snapshot
	return nil
}

func TestSchedulerSnapshotAdmissionCapacityLockOutlivesRebuildTimeout(t *testing.T) {
	cache := &admissionCapacityTTLCache{snapshotHydrationCache: &snapshotHydrationCache{}}
	repo := stubOpenAIAccountRepo{accounts: []Account{{
		ID:          7,
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 3,
	}}}
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)

	err := svc.rebuildAdmissionCapacity(context.Background(), PlatformOpenAI, "test")

	require.NoError(t, err)
	require.Equal(t, time.Minute, cache.lockTTL)
	require.Greater(t, cache.lockTTL, schedulerAdmissionCapacityRebuildTimeout)
	require.Equal(t, 3, cache.snapshot.TotalConcurrency)
}

func TestSchedulerSnapshotAdmissionCapacityFailsClosedAfterAccountAutoExpiresWithoutOutbox(t *testing.T) {
	expiresAt := time.Now().Add(200 * time.Millisecond)
	cache := &admissionCapacityTTLCache{snapshotHydrationCache: &snapshotHydrationCache{}}
	repo := stubOpenAIAccountRepo{accounts: []Account{{
		ID:                 7,
		Platform:           PlatformOpenAI,
		Status:             StatusActive,
		Schedulable:        true,
		Concurrency:        3,
		AutoPauseOnExpired: true,
		ExpiresAt:          &expiresAt,
	}}}
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)

	require.NoError(t, svc.rebuildAdmissionCapacity(context.Background(), PlatformOpenAI, "test"))
	beforeExpiry, err := svc.AdmissionCapacity(context.Background(), PlatformOpenAI)
	require.NoError(t, err)
	require.Equal(t, 3, beforeExpiry.TotalConcurrency)

	require.Eventually(t, func() bool {
		afterExpiry, admissionErr := svc.AdmissionCapacity(context.Background(), PlatformOpenAI)
		return errors.Is(admissionErr, ErrSchedulerCacheNotReady) && afterExpiry.TotalConcurrency == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestSchedulerSnapshotAdmissionCapacityRejectsLegacySnapshotWithoutValidityMetadata(t *testing.T) {
	cache := &admissionCapacityTTLCache{
		snapshotHydrationCache: &snapshotHydrationCache{},
		snapshot: AdmissionCapacitySnapshot{
			TotalConcurrency:   3,
			AccountConcurrency: map[int64]int{7: 3},
		},
	}
	svc := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)

	snapshot, err := svc.AdmissionCapacity(context.Background(), PlatformOpenAI)

	require.ErrorIs(t, err, ErrSchedulerCacheNotReady)
	require.Zero(t, snapshot.TotalConcurrency)
}

func TestSchedulerSnapshotAdmissionCapacityRejectsStaleBuiltAtWithoutValidUntil(t *testing.T) {
	cache := &admissionCapacityTTLCache{
		snapshotHydrationCache: &snapshotHydrationCache{},
		snapshot: AdmissionCapacitySnapshot{
			TotalConcurrency:   3,
			AccountConcurrency: map[int64]int{7: 3},
			BuiltAt:            time.Now().Add(-11 * time.Minute),
		},
	}
	svc := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)

	snapshot, err := svc.AdmissionCapacity(context.Background(), PlatformOpenAI)

	require.ErrorIs(t, err, ErrSchedulerCacheNotReady)
	require.Zero(t, snapshot.TotalConcurrency)
}

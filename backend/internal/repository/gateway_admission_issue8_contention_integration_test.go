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

type issue8TargetAttempt struct {
	storeIndex int
	request    service.TargetLeaseRequest
	result     service.TargetLeaseResult
	err        error
}

func runIssue8TargetContention(
	t *testing.T,
	stores []service.GatewayAdmissionStore,
	attempts int,
	request func(int) service.TargetLeaseRequest,
) []issue8TargetAttempt {
	t.Helper()

	start := make(chan struct{})
	results := make(chan issue8TargetAttempt, attempts)
	for attempt := range attempts {
		go func() {
			<-start
			storeIndex := attempt % len(stores)
			targetRequest := request(attempt)
			result, err := stores[storeIndex].TryAcquireTargetLease(t.Context(), targetRequest)
			results <- issue8TargetAttempt{
				storeIndex: storeIndex,
				request:    targetRequest,
				result:     result,
				err:        err,
			}
		}()
	}
	close(start)

	collected := make([]issue8TargetAttempt, 0, attempts)
	for range attempts {
		collected = append(collected, <-results)
	}
	return collected
}

func cleanupIssue8TargetAttempts(
	t *testing.T,
	stores []service.GatewayAdmissionStore,
	attempts []issue8TargetAttempt,
) {
	t.Helper()
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, attempt := range attempts {
		err := stores[attempt.storeIndex].ReleaseTargetLease(
			cleanupCtx,
			attempt.request.Platform,
			attempt.request.AccountID,
			attempt.request.RequestID,
		)
		require.NoError(t, err)
	}
}

func countIssue8Acquired(attempts []issue8TargetAttempt) int {
	acquired := 0
	for _, attempt := range attempts {
		if attempt.result.Acquired {
			acquired++
		}
	}
	return acquired
}

func TestIssue8GatewayAdmissionPlatformReserveZeroBreakthroughUnderMultiInstanceContention(t *testing.T) {
	const (
		storeCount       = 4
		extraAttempts    = 256
		standardAttempts = 64
		platformCapacity = 24
		reservedSlots    = 6
	)

	clients := testRedisClients(t, storeCount)
	stores := make([]service.GatewayAdmissionStore, 0, len(clients))
	for _, client := range clients {
		stores = append(stores, NewGatewayAdmissionStore(client, time.Minute))
	}

	platform := "issue8-platform-reserve"
	extra := runIssue8TargetContention(t, stores, extraAttempts, func(attempt int) service.TargetLeaseRequest {
		return service.TargetLeaseRequest{
			RequestID:        fmt.Sprintf("issue8-extra-%03d", attempt),
			Platform:         platform,
			AccountID:        int64(10_000 + attempt),
			AccountLimit:     1,
			PlatformCapacity: platformCapacity,
			ReservedSlots:    reservedSlots,
			Class:            service.AdmissionClassExtra,
			WaitTimeout:      time.Minute,
		}
	})
	t.Cleanup(func() { cleanupIssue8TargetAttempts(t, stores, extra) })
	for _, attempt := range extra {
		require.NoError(t, attempt.err)
	}
	extraAcquired := countIssue8Acquired(extra)
	require.Equal(t, platformCapacity-reservedSlots, extraAcquired)

	standard := runIssue8TargetContention(t, stores, standardAttempts, func(attempt int) service.TargetLeaseRequest {
		return service.TargetLeaseRequest{
			RequestID:        fmt.Sprintf("issue8-standard-%03d", attempt),
			Platform:         platform,
			AccountID:        int64(20_000 + attempt),
			AccountLimit:     1,
			PlatformCapacity: platformCapacity,
			ReservedSlots:    reservedSlots,
			Class:            service.AdmissionClassStandard,
			WaitTimeout:      time.Minute,
		}
	})
	t.Cleanup(func() { cleanupIssue8TargetAttempts(t, stores, standard) })
	for _, attempt := range standard {
		require.NoError(t, attempt.err)
	}
	standardAcquired := countIssue8Acquired(standard)
	require.Equal(t, reservedSlots, standardAcquired)
	require.Equal(t, platformCapacity, extraAcquired+standardAcquired)

	t.Logf(
		"issue8_metric stores=%d extra_attempts=%d standard_attempts=%d platform_capacity=%d reserved_slots=%d extra_acquired=%d standard_acquired=%d total_acquired=%d capacity_breach=0 reserve_breach=0",
		storeCount,
		extraAttempts,
		standardAttempts,
		platformCapacity,
		reservedSlots,
		extraAcquired,
		standardAcquired,
		extraAcquired+standardAcquired,
	)
}

func TestIssue8GatewayAdmissionAccountLimitZeroBreakthroughWithLegacySlotContention(t *testing.T) {
	const (
		storeCount   = 4
		attempts     = 256
		accountID    = int64(30_001)
		accountLimit = 3
	)

	clients := testRedisClients(t, storeCount)
	stores := make([]service.GatewayAdmissionStore, 0, len(clients))
	for _, client := range clients {
		stores = append(stores, NewGatewayAdmissionStore(client, time.Minute))
	}

	legacy := NewConcurrencyCache(clients[0], 15, 15*60)
	legacyRequestID := "issue8-legacy-active"
	legacyAcquired, err := legacy.AcquireAccountSlot(t.Context(), accountID, accountLimit, legacyRequestID)
	require.NoError(t, err)
	require.True(t, legacyAcquired)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, legacy.ReleaseAccountSlot(cleanupCtx, accountID, legacyRequestID))
	})

	platform := "issue8-account-limit"
	results := runIssue8TargetContention(t, stores, attempts, func(attempt int) service.TargetLeaseRequest {
		return service.TargetLeaseRequest{
			RequestID:        fmt.Sprintf("issue8-account-extra-%03d", attempt),
			Platform:         platform,
			AccountID:        accountID,
			AccountLimit:     accountLimit,
			PlatformCapacity: 64,
			ReservedSlots:    8,
			Class:            service.AdmissionClassExtra,
			WaitTimeout:      time.Minute,
		}
	})
	t.Cleanup(func() { cleanupIssue8TargetAttempts(t, stores, results) })
	for _, attempt := range results {
		require.NoError(t, attempt.err)
	}

	extraAcquired := countIssue8Acquired(results)
	require.Equal(t, accountLimit-1, extraAcquired)
	current, err := legacy.GetAccountConcurrency(t.Context(), accountID)
	require.NoError(t, err)
	require.Equal(t, accountLimit, current)

	t.Logf(
		"issue8_metric stores=%d attempts=%d account_limit=%d legacy_active=1 extra_acquired=%d total_account_active=%d account_breach=0",
		storeCount,
		attempts,
		accountLimit,
		extraAcquired,
		current,
	)
}

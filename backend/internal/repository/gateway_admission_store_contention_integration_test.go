//go:build integration

package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type targetLeaseAttemptResult struct {
	request service.TargetLeaseRequest
	lease   service.TargetLeaseResult
	err     error
}

func runConcurrentTargetLeaseAttempts(
	t *testing.T,
	stores []service.GatewayAdmissionStore,
	attempts int,
	request func(int) service.TargetLeaseRequest,
) []targetLeaseAttemptResult {
	t.Helper()

	start := make(chan struct{})
	results := make(chan targetLeaseAttemptResult, attempts)
	for i := range attempts {
		go func(attempt int) {
			<-start
			targetRequest := request(attempt)
			lease, err := stores[attempt%len(stores)].TryAcquireTargetLease(t.Context(), targetRequest)
			results <- targetLeaseAttemptResult{request: targetRequest, lease: lease, err: err}
		}(i)
	}
	close(start)

	collected := make([]targetLeaseAttemptResult, 0, attempts)
	for range attempts {
		collected = append(collected, <-results)
	}
	return collected
}

func TestGatewayAdmissionStoreTargetAccountContentionAcrossRedisClients(t *testing.T) {
	clients := testRedisClients(t, 2)
	stores := []service.GatewayAdmissionStore{
		NewGatewayAdmissionStore(clients[0], time.Minute),
		NewGatewayAdmissionStore(clients[1], time.Minute),
	}

	results := runConcurrentTargetLeaseAttempts(t, stores, 32, func(attempt int) service.TargetLeaseRequest {
		return service.TargetLeaseRequest{
			RequestID:        fmt.Sprintf("account-contention-%02d", attempt),
			Platform:         service.PlatformAnthropic,
			AccountID:        801,
			AccountLimit:     1,
			PlatformCapacity: 32,
			Class:            service.AdmissionClassStandard,
			WaitTimeout:      time.Minute,
		}
	})

	acquired := 0
	for _, result := range results {
		require.NoError(t, result.err)
		if result.lease.Acquired {
			acquired++
		}
	}
	require.Equal(t, 1, acquired)
}

func TestGatewayAdmissionStoreTargetPlatformContentionAcrossRedisClients(t *testing.T) {
	clients := testRedisClients(t, 2)
	stores := []service.GatewayAdmissionStore{
		NewGatewayAdmissionStore(clients[0], time.Minute),
		NewGatewayAdmissionStore(clients[1], time.Minute),
	}

	results := runConcurrentTargetLeaseAttempts(t, stores, 2, func(attempt int) service.TargetLeaseRequest {
		return service.TargetLeaseRequest{
			RequestID:        fmt.Sprintf("platform-contention-%02d", attempt),
			Platform:         service.PlatformAnthropic,
			AccountID:        int64(901 + attempt),
			AccountLimit:     1,
			PlatformCapacity: 1,
			Class:            service.AdmissionClassStandard,
			WaitTimeout:      time.Minute,
		}
	})

	acquired := 0
	for _, result := range results {
		require.NoError(t, result.err)
		if result.lease.Acquired {
			acquired++
		}
	}
	require.Equal(t, 1, acquired)
}

func TestGatewayAdmissionStoreStandardTargetWaitersAreFIFO(t *testing.T) {
	clients := testRedisClients(t, 2)
	stores := []service.GatewayAdmissionStore{
		NewGatewayAdmissionStore(clients[0], time.Minute),
		NewGatewayAdmissionStore(clients[1], time.Minute),
	}
	request := service.TargetLeaseRequest{
		Platform:         service.PlatformAnthropic,
		AccountID:        1001,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
		WaitTimeout:      time.Minute,
	}

	request.RequestID = "active-request"
	active, err := stores[0].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, active.Acquired)

	request.RequestID = "earlier-waiter"
	earlier, err := stores[0].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, earlier.Acquired)

	request.RequestID = "later-waiter"
	later, err := stores[1].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, later.Acquired)

	require.NoError(t, stores[0].ReleaseTargetLease(t.Context(), request.Platform, request.AccountID, "active-request"))

	later, err = stores[1].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, later.Acquired)

	request.RequestID = "earlier-waiter"
	earlier, err = stores[0].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, earlier.Acquired)
}

func TestGatewayAdmissionStoreExtraTargetWaitersAreFIFO(t *testing.T) {
	clients := testRedisClients(t, 2)
	stores := []service.GatewayAdmissionStore{
		NewGatewayAdmissionStore(clients[0], time.Minute),
		NewGatewayAdmissionStore(clients[1], time.Minute),
	}
	request := service.TargetLeaseRequest{
		Platform:         service.PlatformAnthropic,
		AccountID:        1003,
		AccountLimit:     1,
		PlatformCapacity: 2,
		ReservedSlots:    1,
		Class:            service.AdmissionClassExtra,
		WaitTimeout:      time.Minute,
	}

	request.RequestID = "active-extra"
	active, err := stores[0].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, active.Acquired)

	request.RequestID = "earlier-extra"
	earlier, err := stores[0].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, earlier.Acquired)

	request.RequestID = "later-extra"
	later, err := stores[1].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, later.Acquired)

	require.NoError(t, stores[0].ReleaseTargetLease(t.Context(), request.Platform, request.AccountID, "active-extra"))

	later, err = stores[1].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, later.Acquired)

	request.RequestID = "earlier-extra"
	earlier, err = stores[0].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, earlier.Acquired)
}

func TestGatewayAdmissionStorePromotedExtraKeepsOriginalTargetOrder(t *testing.T) {
	clients := testRedisClients(t, 2)
	stores := []service.GatewayAdmissionStore{
		NewGatewayAdmissionStore(clients[0], time.Minute),
		NewGatewayAdmissionStore(clients[1], time.Minute),
	}
	request := service.TargetLeaseRequest{
		Platform:         service.PlatformAnthropic,
		AccountID:        1002,
		AccountLimit:     1,
		PlatformCapacity: 2,
		ReservedSlots:    1,
		WaitTimeout:      time.Minute,
	}

	request.RequestID = "active-request"
	request.Class = service.AdmissionClassStandard
	active, err := stores[0].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, active.Acquired)

	request.RequestID = "earlier-extra"
	request.Class = service.AdmissionClassExtra
	earlier, err := stores[0].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, earlier.Acquired)

	request.RequestID = "later-standard"
	request.Class = service.AdmissionClassStandard
	later, err := stores[1].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, later.Acquired)

	request.RequestID = "earlier-extra"
	request.Class = service.AdmissionClassStandard
	earlier, err = stores[0].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, earlier.Acquired)

	require.NoError(t, stores[0].ReleaseTargetLease(t.Context(), request.Platform, request.AccountID, "active-request"))

	request.RequestID = "later-standard"
	later, err = stores[1].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, later.Acquired)

	request.RequestID = "earlier-extra"
	earlier, err = stores[0].TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, earlier.Acquired)
}

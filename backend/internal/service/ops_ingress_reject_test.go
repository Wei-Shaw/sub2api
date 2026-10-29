package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type ingressRejectRepoStub struct {
	mu        sync.Mutex
	failCount int
	calls     int
	requests  int64
}

func (r *ingressRejectRepoStub) BatchUpsertIngressRejects(_ context.Context, items []*OpsIngressRejectAggregate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.failCount > 0 {
		r.failCount--
		return errors.New("temporary database failure")
	}
	for _, item := range items {
		if item != nil {
			r.requests += item.RequestCount
		}
	}
	return nil
}

func (r *ingressRejectRepoStub) ListIngressRejects(context.Context, *OpsIngressRejectFilter) (*OpsIngressRejectList, error) {
	return &OpsIngressRejectList{}, nil
}

func TestOpsIngressRejectAggregatorUsesGlobalBucketCapacity(t *testing.T) {
	repo := &ingressRejectRepoStub{}
	a := NewOpsIngressRejectAggregator(repo)
	a.Start()

	// Concentrating all dimensions in one shard must not waste capacity in the others.
	inserted := 0
	for i := 0; inserted < 600; i++ {
		ip := fmt.Sprintf("target-%d", i)
		key := ingressRejectKey{reason: "invalid_api_key", routeFamily: "messages", protocol: "anthropic", clientIP: ip}
		if ingressRejectHash(key)%ingressRejectShardCount == 0 {
			a.RecordIngressReject(key.reason, key.routeFamily, key.protocol, ip, 0, 0)
			inserted++
		}
	}
	require.Equal(t, int64(600), a.Health().Cardinality)

	for i := 0; i < ingressRejectMaxEntries; i++ {
		a.RecordIngressReject("invalid_api_key", "messages", "anthropic", fmt.Sprintf("rotating-%d", i), 0, 0)
	}
	health := a.Health()
	require.Equal(t, int64(ingressRejectMaxEntries), health.Cardinality)
	require.Greater(t, health.Overflowed, uint64(0))

	a.snapshotAndEnqueue(false)
	require.Equal(t, int64(ingressRejectMaxEntries), a.Health().Cardinality, "periodic flush must retain the minute budget")
	a.Stop()
}

func TestOpsIngressRejectAggregatorConcurrentCountAndStopFlush(t *testing.T) {
	repo := &ingressRejectRepoStub{}
	a := NewOpsIngressRejectAggregator(repo)
	a.Start()

	const goroutines = 32
	const perGoroutine = 200
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				a.RecordIngressReject("invalid_api_key", "responses", "openai", "192.0.2.10", 0, 0)
			}
		}()
	}
	wg.Wait()
	a.Stop()

	repo.mu.Lock()
	require.Equal(t, int64(goroutines*perGoroutine), repo.requests)
	repo.mu.Unlock()
	require.False(t, a.Health().Accepting)
	a.RecordIngressReject("invalid_api_key", "responses", "openai", "192.0.2.10", 0, 0)
}

func TestOpsIngressRejectAggregatorRetriesBoundedPendingBatch(t *testing.T) {
	repo := &ingressRejectRepoStub{failCount: 1}
	a := NewOpsIngressRejectAggregator(repo)
	a.accepting.Store(true)
	a.RecordIngressReject("group_deleted", "messages", "anthropic", "192.0.2.20", 1, 2)
	a.snapshotAndEnqueue(false)
	a.flushPending()
	health := a.Health()
	require.Equal(t, 1, health.PendingBatches)
	require.Equal(t, uint64(1), health.FlushFailures)
	require.Equal(t, uint64(0), health.Dropped)

	a.flushPending()
	require.Equal(t, 0, a.Health().PendingBatches)
	a.Stop()
	repo.mu.Lock()
	require.Equal(t, int64(1), repo.requests)
	require.GreaterOrEqual(t, repo.calls, 2)
	repo.mu.Unlock()
}

package securityaudit

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAtomicMetricsExposeCountsLatencyDistributionAndAsyncDelivery(t *testing.T) {
	metrics := NewAtomicMetrics()
	latencies := []time.Duration{10, 20, 30, 40, 100}
	kinds := []DecisionKind{DecisionAllow, DecisionFlag, DecisionBlock, DecisionUnavailable, DecisionInvalid}
	for index := range latencies {
		metrics.Observe(kinds[index], latencies[index]*time.Millisecond)
	}
	metrics.IncTimeout()
	metrics.IncFailover()
	metrics.IncBulkheadFull()
	metrics.IncRecordFailed()
	metrics.IncEnqueued()
	metrics.IncDropped()

	snapshot := metrics.Snapshot()
	require.Equal(t, int64(5), snapshot.Total)
	require.Equal(t, int64(5), snapshot.LatencyCount)
	require.Equal(t, int64(40), snapshot.LatencyAvgMS)
	require.Equal(t, int64(30), snapshot.LatencyP50MS)
	require.Equal(t, int64(40), snapshot.LatencyP95MS)
	require.Equal(t, int64(40), snapshot.LatencyP99MS)
	require.Equal(t, int64(100), snapshot.LatencyMaxMS)
	require.Equal(t, AuditMetricsSnapshot{Enqueued: 1, Dropped: 1}, metrics.AuditSnapshot())
}

func TestAtomicMetricsConcurrentObservationIsBoundedAndRaceSafe(t *testing.T) {
	metrics := NewAtomicMetrics()
	const observations = 4096
	var wg sync.WaitGroup
	for index := 0; index < observations; index++ {
		wg.Add(1)
		go func(value int) {
			defer wg.Done()
			metrics.Observe(DecisionAllow, time.Duration(value%250)*time.Millisecond)
		}(index)
	}
	wg.Wait()
	require.Equal(t, int64(observations), metrics.Snapshot().Total)
	metrics.latencyMu.RLock()
	require.LessOrEqual(t, len(metrics.latencies), latencySampleCapacity)
	metrics.latencyMu.RUnlock()
}

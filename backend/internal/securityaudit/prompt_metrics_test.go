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
	metrics.IncGenericSampledOut()
	metrics.IncGenericSchemaFailure()
	metrics.IncGenericFailOpen()
	metrics.IncGenericFailClosed()
	metrics.ObserveGeneric(&NormalizedResult{EngineType: EngineGenericLLM, PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}, 55*time.Millisecond)

	snapshot := metrics.Snapshot()
	require.Equal(t, int64(5), snapshot.Total)
	require.Equal(t, int64(5), snapshot.LatencyCount)
	require.Equal(t, int64(40), snapshot.LatencyAvgMS)
	require.Equal(t, int64(30), snapshot.LatencyP50MS)
	require.Equal(t, int64(40), snapshot.LatencyP95MS)
	require.Equal(t, int64(40), snapshot.LatencyP99MS)
	require.Equal(t, int64(100), snapshot.LatencyMaxMS)
	require.Equal(t, AuditMetricsSnapshot{Enqueued: 1, Dropped: 1}, metrics.AuditSnapshot())
	require.Equal(t, int64(1), snapshot.GenericRequests)
	require.Equal(t, int64(1), snapshot.GenericSampledOut)
	require.Equal(t, int64(1), snapshot.GenericSchemaFailures)
	require.Equal(t, int64(1), snapshot.GenericFailOpen)
	require.Equal(t, int64(1), snapshot.GenericFailClosed)
	require.Equal(t, int64(11), snapshot.GenericPromptTokens)
	require.Equal(t, int64(7), snapshot.GenericCompletionTokens)
	require.Equal(t, int64(18), snapshot.GenericTotalTokens)
	require.Equal(t, int64(55), snapshot.GenericLatencyAvgMS)
	require.Equal(t, int64(55), snapshot.GenericLatencyMaxMS)
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

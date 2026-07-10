package service

import (
	"sync"
	"testing"
)

func TestReliabilityMetricsRecorderIsThreadSafeAndLabelAware(t *testing.T) {
	metrics := NewReliabilityMetrics()
	const workers = 64
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			metrics.Add("video_finalization_total", 1, map[string]string{"status": VideoStatusSucceeded})
			metrics.Add("domain_outbox_pending_total", 1, map[string]string{"event_type": VideoOutboxEventArchive})
		}()
	}
	wg.Wait()
	metrics.Set("billing_reservation_active_total", 7, nil)

	requireReliabilityMetricValue(t, metrics.Snapshot(), "video_finalization_total", map[string]string{"status": VideoStatusSucceeded}, workers)
	requireReliabilityMetricValue(t, metrics.Snapshot(), "domain_outbox_pending_total", map[string]string{"event_type": VideoOutboxEventArchive}, workers)
	requireReliabilityMetricValue(t, metrics.Snapshot(), "billing_reservation_active_total", nil, 7)
}

func TestDefaultReliabilityMetricsRecorderCanBeInstalledAndRestored(t *testing.T) {
	metrics := NewReliabilityMetrics()
	restore := InstallReliabilityMetricsRecorder(metrics)
	t.Cleanup(restore)

	RecordReliabilityMetricAdd("video_dispatch_unknown_total", 1, map[string]string{"provider": VideoProviderSeedance})
	RecordReliabilityMetricSet("domain_outbox_oldest_age_seconds", 12, nil)

	requireReliabilityMetricValue(t, metrics.Snapshot(), "video_dispatch_unknown_total", map[string]string{"provider": VideoProviderSeedance}, 1)
	requireReliabilityMetricValue(t, metrics.Snapshot(), "domain_outbox_oldest_age_seconds", nil, 12)
}

func requireReliabilityMetricValue(t *testing.T, samples []ReliabilityMetricSample, name string, labels map[string]string, want float64) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name && sameReliabilityMetricLabels(sample.Labels, labels) {
			if sample.Value != want {
				t.Fatalf("metric %s%v = %v, want %v", name, labels, sample.Value, want)
			}
			return
		}
	}
	t.Fatalf("metric %s%v not found in %#v", name, labels, samples)
}

func sameReliabilityMetricLabels(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

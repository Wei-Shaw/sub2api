package observability

import (
	"errors"
	"testing"
	"time"
)

func TestChatSessionCaptureStats(t *testing.T) {
	old := chatSessionCaptureMetrics
	t.Cleanup(func() { chatSessionCaptureMetrics = old })
	chatSessionCaptureMetrics = &chatSessionCaptureMetricsState{}

	SetChatSessionCaptureQueueStats(3, 2048)
	IncChatSessionCaptureEnqueued()
	IncChatSessionCaptureDropped()
	ObserveChatSessionCaptureSuccess(250 * time.Millisecond)
	ObserveChatSessionCaptureSuccess(750 * time.Millisecond)
	ObserveChatSessionCaptureFailure(125*time.Millisecond, errors.New("db failed"))
	ObserveChatSessionCaptureSlow(1500 * time.Millisecond)

	stats := ChatSessionCaptureSnapshot(time.Second)
	if stats.QueueLength != 3 || stats.QueueCapacity != 2048 {
		t.Fatalf("queue stats = %d/%d, want 3/2048", stats.QueueLength, stats.QueueCapacity)
	}
	if stats.EnqueuedTotal != 1 || stats.DroppedTotal != 1 || stats.SuccessTotal != 2 || stats.FailureTotal != 1 || stats.SlowTotal != 1 {
		t.Fatalf("counters = %#v", stats)
	}
	if stats.AverageDurationMS != 500 {
		t.Fatalf("average duration = %f, want 500", stats.AverageDurationMS)
	}
	if stats.MaxDurationMS != 750 || stats.LastDurationMS != 125 || stats.LastSlowDurationMS != 1500 {
		t.Fatalf("duration stats = %#v", stats)
	}
	if stats.LastFailureMessage != "db failed" {
		t.Fatalf("last failure = %q", stats.LastFailureMessage)
	}
}

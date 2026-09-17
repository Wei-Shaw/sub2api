package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStabilizeMonitorStatus(t *testing.T) {
	now := time.Now()
	entry := func(status string) *ChannelMonitorHistoryEntry {
		return &ChannelMonitorHistoryEntry{Status: status, CheckedAt: now}
	}

	if got := stabilizeMonitorStatus(MonitorStatusError, []*ChannelMonitorHistoryEntry{entry(MonitorStatusError)}); got != MonitorStatusError {
		t.Fatalf("single error should remain raw before history smoothing, got %q", got)
	}
	if got := stabilizeMonitorStatus(MonitorStatusError, []*ChannelMonitorHistoryEntry{entry(MonitorStatusError), entry(MonitorStatusOperational)}); got != MonitorStatusDegraded {
		t.Fatalf("one-off error should be degraded, got %q", got)
	}
	if got := stabilizeMonitorStatus(MonitorStatusError, []*ChannelMonitorHistoryEntry{entry(MonitorStatusError), entry(MonitorStatusFailed)}); got != MonitorStatusError {
		t.Fatalf("two consecutive failures should remain error, got %q", got)
	}
	if got := stabilizeMonitorStatus(MonitorStatusOperational, []*ChannelMonitorHistoryEntry{entry(MonitorStatusOperational), entry(MonitorStatusError)}); got != MonitorStatusDegraded {
		t.Fatalf("first recovery should be degraded, got %q", got)
	}
}

func TestIsRetryableMonitorResult(t *testing.T) {
	if !isRetryableMonitorResult(0, &testMonitorNetError{}) {
		t.Fatal("network errors should be retryable")
	}
	if !isRetryableMonitorResult(503, nil) || !isRetryableMonitorResult(429, nil) {
		t.Fatal("transient upstream statuses should be retryable")
	}
	if isRetryableMonitorResult(401, nil) || isRetryableMonitorResult(400, errors.New("invalid request")) {
		t.Fatal("auth and configuration errors should not be retryable")
	}
	if isRetryableMonitorResult(0, context.Canceled) {
		t.Fatal("canceled checks should not be retried")
	}
}

type testMonitorNetError struct{}

func (*testMonitorNetError) Error() string   { return "temporary network error" }
func (*testMonitorNetError) Timeout() bool   { return true }
func (*testMonitorNetError) Temporary() bool { return true }

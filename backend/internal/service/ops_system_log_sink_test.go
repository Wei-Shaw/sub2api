package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

func TestOpsSystemLogSink_ShouldIndex(t *testing.T) {
	sink := &OpsSystemLogSink{}

	cases := []struct {
		name  string
		event *logger.LogEvent
		want  bool
	}{
		{
			name:  "warn level",
			event: &logger.LogEvent{Level: "warn", Component: "app"},
			want:  true,
		},
		{
			name:  "error level",
			event: &logger.LogEvent{Level: "error", Component: "app"},
			want:  true,
		},
		{
			name:  "access component",
			event: &logger.LogEvent{Level: "info", Component: "http.access"},
			want:  true,
		},
		{
			name: "access component from fields (real zap path)",
			event: &logger.LogEvent{
				Level:     "info",
				Component: "",
				Fields:    map[string]any{"component": "http.access"},
			},
			want: true,
		},
		{
			name:  "audit component",
			event: &logger.LogEvent{Level: "info", Component: "audit.log_config_change"},
			want:  true,
		},
		{
			name: "audit component from fields (real zap path)",
			event: &logger.LogEvent{
				Level:     "info",
				Component: "",
				Fields:    map[string]any{"component": "audit.log_config_change"},
			},
			want: true,
		},
		{
			name:  "plain info",
			event: &logger.LogEvent{Level: "info", Component: "app"},
			want:  false,
		},
	}

	for _, tc := range cases {
		if got := sink.shouldIndex(tc.event); got != tc.want {
			t.Fatalf("%s: shouldIndex()=%v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestOpsSystemLogSink_WriteLogEvent_ShouldDropWhenQueueFull(t *testing.T) {
	sink := &OpsSystemLogSink{
		queue: make(chan *logger.LogEvent, 1),
	}

	sink.WriteLogEvent(&logger.LogEvent{Level: "warn", Component: "app"})
	sink.WriteLogEvent(&logger.LogEvent{Level: "warn", Component: "app"})

	if got := len(sink.queue); got != 1 {
		t.Fatalf("queue len = %d, want 1", got)
	}
	if dropped := atomic.LoadUint64(&sink.droppedCount); dropped != 1 {
		t.Fatalf("droppedCount = %d, want 1", dropped)
	}
}

func TestOpsSystemLogSink_Health(t *testing.T) {
	sink := &OpsSystemLogSink{
		queue: make(chan *logger.LogEvent, 10),
	}
	sink.lastError.Store("db timeout")
	atomic.StoreUint64(&sink.droppedCount, 3)
	atomic.StoreUint64(&sink.writeFailed, 2)
	atomic.StoreUint64(&sink.writtenCount, 5)
	atomic.StoreUint64(&sink.totalDelayNs, uint64(5000000)) // 5ms total -> avg 1ms
	sink.queue <- &logger.LogEvent{Level: "warn", Component: "app"}
	sink.queue <- &logger.LogEvent{Level: "warn", Component: "app"}

	health := sink.Health()
	if health.QueueDepth != 2 {
		t.Fatalf("queue depth = %d, want 2", health.QueueDepth)
	}
	if health.QueueCapacity != 10 {
		t.Fatalf("queue capacity = %d, want 10", health.QueueCapacity)
	}
	if health.DroppedCount != 3 {
		t.Fatalf("dropped = %d, want 3", health.DroppedCount)
	}
	if health.WriteFailed != 2 {
		t.Fatalf("write failed = %d, want 2", health.WriteFailed)
	}
	if health.WrittenCount != 5 {
		t.Fatalf("written = %d, want 5", health.WrittenCount)
	}
	if health.AvgWriteDelayMs != 1 {
		t.Fatalf("avg delay ms = %d, want 1", health.AvgWriteDelayMs)
	}
	if health.LastError != "db timeout" {
		t.Fatalf("last error = %q, want db timeout", health.LastError)
	}
}

func TestOpsSystemLogSink_StartStopAndFlushSuccess(t *testing.T) {
	done := make(chan struct{}, 1)
	var captured []*OpsInsertSystemLogInput
	repo := &opsRepoMock{
		BatchInsertSystemLogsFn: func(_ context.Context, inputs []*OpsInsertSystemLogInput) (int64, error) {
			captured = append(captured, inputs...)
			select {
			case done <- struct{}{}:
			default:
			}
			return int64(len(inputs)), nil
		},
	}

	sink := NewOpsSystemLogSink(repo)
	sink.batchSize = 1
	sink.flushInterval = 10 * time.Millisecond
	sink.Start()
	defer sink.Stop()

	sink.WriteLogEvent(&logger.LogEvent{
		Time:      time.Now().UTC(),
		Level:     "warn",
		Component: "http.access",
		Message:   `authorization="Bearer sk-test-123"`,
		Fields: map[string]any{
			"component":         "http.access",
			"request_id":        "req-1",
			"client_request_id": "creq-1",
			"user_id":           "12",
			"account_id":        json.Number("34"),
			"platform":          "openai",
			"model":             "gpt-5",
		},
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for sink flush")
	}

	if len(captured) != 1 {
		t.Fatalf("captured len = %d, want 1", len(captured))
	}
	item := captured[0]
	if item.RequestID != "req-1" || item.ClientRequestID != "creq-1" {
		t.Fatalf("unexpected request ids: %+v", item)
	}
	if item.UserID == nil || *item.UserID != 12 {
		t.Fatalf("unexpected user_id: %+v", item.UserID)
	}
	if item.AccountID == nil || *item.AccountID != 34 {
		t.Fatalf("unexpected account_id: %+v", item.AccountID)
	}
	if strings.TrimSpace(item.Message) == "" {
		t.Fatalf("message should not be empty")
	}
	// writtenCount is incremented after BatchInsertSystemLogsFn returns,
	// so poll briefly to avoid a race between the done signal and the atomic add.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if sink.Health().WrittenCount > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	health := sink.Health()
	if health.WrittenCount == 0 {
		t.Fatalf("written_count should be >0")
	}
}

func TestOpsSystemLogSink_FlushFailureUpdatesHealth(t *testing.T) {
	repo := &opsRepoMock{
		BatchInsertSystemLogsFn: func(_ context.Context, inputs []*OpsInsertSystemLogInput) (int64, error) {
			return 0, errors.New("db unavailable")
		},
	}
	sink := NewOpsSystemLogSink(repo)
	sink.batchSize = 1
	sink.flushInterval = 10 * time.Millisecond
	sink.Start()
	defer sink.Stop()

	sink.WriteLogEvent(&logger.LogEvent{
		Time:      time.Now().UTC(),
		Level:     "warn",
		Component: "app",
		Message:   "boom",
		Fields:    map[string]any{},
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		health := sink.Health()
		if health.WriteFailed > 0 {
			if !strings.Contains(health.LastError, "db unavailable") {
				t.Fatalf("unexpected last error: %s", health.LastError)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("write_failed_count not updated")
}

func TestOpsSystemLogSink_StopFlushUsesActiveContextAndDrainsQueue(t *testing.T) {
	var inserted int64
	var canceledCtxCalls int64
	repo := &opsRepoMock{
		BatchInsertSystemLogsFn: func(ctx context.Context, inputs []*OpsInsertSystemLogInput) (int64, error) {
			if err := ctx.Err(); err != nil {
				atomic.AddInt64(&canceledCtxCalls, 1)
				return 0, err
			}
			atomic.AddInt64(&inserted, int64(len(inputs)))
			return int64(len(inputs)), nil
		},
	}

	sink := NewOpsSystemLogSink(repo)
	sink.batchSize = 200
	sink.flushInterval = time.Hour
	sink.Start()

	sink.WriteLogEvent(&logger.LogEvent{
		Time:      time.Now().UTC(),
		Level:     "warn",
		Component: "app",
		Message:   "pending-on-shutdown",
		Fields:    map[string]any{"component": "http.access"},
	})

	sink.Stop()

	if got := atomic.LoadInt64(&inserted); got != 1 {
		t.Fatalf("inserted = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&canceledCtxCalls); got != 0 {
		t.Fatalf("canceled ctx calls = %d, want 0", got)
	}
	health := sink.Health()
	if health.WrittenCount != 1 {
		t.Fatalf("written_count = %d, want 1", health.WrittenCount)
	}
}

type stringerValue string

func (s stringerValue) String() string { return string(s) }

func TestOpsSystemLogSink_HelperFunctions(t *testing.T) {
	src := map[string]any{"a": 1}
	cloned := copyMap(src)
	src["a"] = 2
	v, ok := cloned["a"].(int)
	if !ok || v != 1 {
		t.Fatalf("copyMap should create copy")
	}
	if got := asString(stringerValue(" hello ")); got != "hello" {
		t.Fatalf("asString stringer = %q", got)
	}
	if got := asString(fmt.Errorf("x")); got != "" {
		t.Fatalf("asString error should be empty, got %q", got)
	}
	if got := asString(123); got != "" {
		t.Fatalf("asString non-string should be empty, got %q", got)
	}

	cases := []struct {
		in   any
		want int64
		ok   bool
	}{
		{in: 5, want: 5, ok: true},
		{in: int64(6), want: 6, ok: true},
		{in: float64(7), want: 7, ok: true},
		{in: json.Number("8"), want: 8, ok: true},
		{in: "9", want: 9, ok: true},
		{in: "0", ok: false},
		{in: -1, ok: false},
		{in: "abc", ok: false},
	}
	for _, tc := range cases {
		got := asInt64Ptr(tc.in)
		if tc.ok {
			if got == nil || *got != tc.want {
				t.Fatalf("asInt64Ptr(%v) = %+v, want %d", tc.in, got, tc.want)
			}
		} else if got != nil {
			t.Fatalf("asInt64Ptr(%v) should be nil, got %d", tc.in, *got)
		}
	}
}

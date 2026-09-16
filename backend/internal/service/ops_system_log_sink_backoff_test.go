package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

func opsSystemLogBackoffEvent() *logger.LogEvent {
	return &logger.LogEvent{
		Time:      time.Now().UTC(),
		Level:     "warn",
		Component: "app",
		Message:   "boom",
		Fields:    map[string]any{},
	}
}

// pumpOpsSystemLogEvents 持续投递日志事件，模拟故障期间业务侧不断产生 WARN/ERROR。
func pumpOpsSystemLogEvents(t *testing.T, sink *OpsSystemLogSink) {
	t.Helper()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			sink.WriteLogEvent(opsSystemLogBackoffEvent())
			time.Sleep(2 * time.Millisecond)
		}
	}()
	t.Cleanup(func() {
		close(stop)
		<-done
	})
}

func waitForOpsSystemLogCondition(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func TestOpsSystemLogSinkFlushBackoffFor(t *testing.T) {
	sink := &OpsSystemLogSink{flushBackoff: time.Second, flushBackoffMax: 8 * time.Second}

	cases := []struct {
		name     string
		failures int
		want     time.Duration
	}{
		{"first_failure", 1, time.Second},
		{"second_failure", 2, 2 * time.Second},
		{"third_failure", 3, 4 * time.Second},
		{"fourth_failure", 4, 8 * time.Second},
		{"capped", 9, 8 * time.Second},
		{"large_streak_does_not_overflow", 1000, 8 * time.Second},
		{"zero_streak_uses_base", 0, time.Second},
		{"negative_streak_uses_base", -1, time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sink.flushBackoffFor(tc.failures); got != tc.want {
				t.Fatalf("flushBackoffFor(%d) = %s, want %s", tc.failures, got, tc.want)
			}
		})
	}
}

// 未显式配置时回落到默认值；max 小于 base 时以 base 为准，不产生比基准还短的退避。
func TestOpsSystemLogSinkFlushBackoffForFallbacks(t *testing.T) {
	zero := &OpsSystemLogSink{}
	if got := zero.flushBackoffFor(1); got != defaultOpsSystemLogFlushBackoff {
		t.Fatalf("zero-value base = %s, want %s", got, defaultOpsSystemLogFlushBackoff)
	}
	if got := zero.flushBackoffFor(100); got != defaultOpsSystemLogFlushBackoffMax {
		t.Fatalf("zero-value cap = %s, want %s", got, defaultOpsSystemLogFlushBackoffMax)
	}

	inverted := &OpsSystemLogSink{flushBackoff: 5 * time.Second, flushBackoffMax: time.Second}
	if got := inverted.flushBackoffFor(3); got != 5*time.Second {
		t.Fatalf("inverted bounds = %s, want 5s", got)
	}
}

// issue #5265：写入失败后如果按 flushInterval 继续每秒重试，每一轮都会占用并取消
// 一条池内连接（远程 PG 上 COPY 取消会让连接协议失步而被销毁），小连接池会被日志
// 通道长期占满，业务侧最终报 Billing 503。失败后必须退避。
func TestOpsSystemLogSinkSuppressesRetriesDuringBackoff(t *testing.T) {
	var calls int64
	repo := &opsRepoMock{
		BatchInsertSystemLogsFn: func(_ context.Context, _ []*OpsInsertSystemLogInput) (int64, error) {
			atomic.AddInt64(&calls, 1)
			return 0, errors.New("db unavailable")
		},
	}

	sink := NewOpsSystemLogSink(repo)
	sink.batchSize = 1
	sink.flushInterval = 5 * time.Millisecond
	sink.flushBackoff = 800 * time.Millisecond
	sink.flushBackoffMax = 800 * time.Millisecond
	sink.Start()
	defer sink.Stop()
	pumpOpsSystemLogEvents(t, sink)

	if !waitForOpsSystemLogCondition(t, 2*time.Second, func() bool { return atomic.LoadInt64(&calls) >= 1 }) {
		t.Fatalf("first flush never happened")
	}

	// 退避窗口内不得再打上游：修复前 5ms 的 flushInterval 会在这 300ms 里打出上百次。
	time.Sleep(300 * time.Millisecond)
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("upstream calls during backoff = %d, want 1", got)
	}

	// 被抑制的批次记为 dropped，而不是 write_failed —— 没有尝试过就不算写失败。
	health := sink.Health()
	if health.DroppedCount == 0 {
		t.Fatalf("dropped_count should grow while flushing is suppressed")
	}
	if health.WriteFailed == 0 || health.LastError == "" {
		t.Fatalf("failed flush should still surface in health: %+v", health)
	}
}

// 退避到期后必须自动恢复，不能变成永久停写。
func TestOpsSystemLogSinkResumesAfterBackoffWindow(t *testing.T) {
	var calls int64
	repo := &opsRepoMock{
		BatchInsertSystemLogsFn: func(_ context.Context, _ []*OpsInsertSystemLogInput) (int64, error) {
			atomic.AddInt64(&calls, 1)
			return 0, errors.New("db unavailable")
		},
	}

	sink := NewOpsSystemLogSink(repo)
	sink.batchSize = 1
	sink.flushInterval = 5 * time.Millisecond
	sink.flushBackoff = 150 * time.Millisecond
	sink.flushBackoffMax = 150 * time.Millisecond
	sink.Start()
	defer sink.Stop()
	pumpOpsSystemLogEvents(t, sink)

	if !waitForOpsSystemLogCondition(t, 3*time.Second, func() bool { return atomic.LoadInt64(&calls) >= 3 }) {
		t.Fatalf("sink did not resume flushing after backoff, calls=%d", atomic.LoadInt64(&calls))
	}
}

// 一次成功必须清空失败计数与抑制窗口，否则短暂抖动后写入速率无法恢复。
func TestOpsSystemLogSinkSuccessClearsSuppression(t *testing.T) {
	var calls int64
	repo := &opsRepoMock{
		BatchInsertSystemLogsFn: func(_ context.Context, inputs []*OpsInsertSystemLogInput) (int64, error) {
			if atomic.AddInt64(&calls, 1) == 1 {
				return 0, errors.New("db unavailable")
			}
			return int64(len(inputs)), nil
		},
	}

	sink := NewOpsSystemLogSink(repo)
	sink.batchSize = 1
	sink.flushInterval = 5 * time.Millisecond
	sink.flushBackoff = 150 * time.Millisecond
	sink.flushBackoffMax = 150 * time.Millisecond
	sink.Start()
	defer sink.Stop()
	pumpOpsSystemLogEvents(t, sink)

	// 第 2 次调用（恢复后的首次成功）之后不应再有抑制：调用次数需要快速爬升。
	if !waitForOpsSystemLogCondition(t, 3*time.Second, func() bool { return atomic.LoadInt64(&calls) >= 2 }) {
		t.Fatalf("sink never retried after the first failure")
	}
	recovered := atomic.LoadInt64(&calls)
	time.Sleep(200 * time.Millisecond)
	if got := atomic.LoadInt64(&calls) - recovered; got < 3 {
		t.Fatalf("calls after recovery = %d in 200ms, want >=3 (suppression not cleared)", got)
	}
	if sink.Health().WrittenCount == 0 {
		t.Fatalf("written_count should grow after recovery")
	}
}

// 健康路径不受影响：一直成功就永远不进入退避。
func TestOpsSystemLogSinkHealthyPathNeverSuppressed(t *testing.T) {
	var calls int64
	repo := &opsRepoMock{
		BatchInsertSystemLogsFn: func(_ context.Context, inputs []*OpsInsertSystemLogInput) (int64, error) {
			atomic.AddInt64(&calls, 1)
			return int64(len(inputs)), nil
		},
	}

	sink := NewOpsSystemLogSink(repo)
	sink.batchSize = 1
	sink.flushInterval = 5 * time.Millisecond
	sink.flushBackoff = time.Hour
	sink.flushBackoffMax = time.Hour
	sink.Start()
	defer sink.Stop()
	pumpOpsSystemLogEvents(t, sink)

	if !waitForOpsSystemLogCondition(t, 3*time.Second, func() bool { return atomic.LoadInt64(&calls) >= 10 }) {
		t.Fatalf("healthy sink should keep flushing, calls=%d", atomic.LoadInt64(&calls))
	}
	if got := sink.Health().DroppedCount; got != 0 {
		t.Fatalf("healthy sink dropped_count = %d, want 0", got)
	}
}

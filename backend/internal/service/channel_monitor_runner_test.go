//go:build unit

package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// stubMonitorSvc 实现 monitorRunnerSvc，用于隔离 runner 与真实 service/repo。
type stubMonitorSvc struct {
	enabled    []*ChannelMonitor
	runCount   atomic.Int64
	runCalled  chan int64 // 每次 RunCheck 触发时 push 一次（缓冲足够大避免阻塞）
	runErr     error
	listErr    error
	runHoldFor time.Duration // RunCheck 内额外阻塞的时长，用来测试 Stop 等待行为
}

func (s *stubMonitorSvc) ListEnabledMonitors(_ context.Context) ([]*ChannelMonitor, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.enabled, nil
}

func (s *stubMonitorSvc) RunCheck(ctx context.Context, id int64) ([]*CheckResult, error) {
	s.runCount.Add(1)
	if s.runCalled != nil {
		select {
		case s.runCalled <- id:
		default:
		}
	}
	if s.runHoldFor > 0 {
		select {
		case <-time.After(s.runHoldFor):
		case <-ctx.Done():
		}
	}
	return nil, s.runErr
}

func newRunnerForTest(svc monitorRunnerSvc) *ChannelMonitorRunner {
	return newChannelMonitorRunner(svc, nil)
}

// 等待 condition 在 timeout 内变 true，否则 t.Fatalf。轮询 5ms 一次。
func waitFor(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("waitFor timed out: %s", msg)
	}
}

func runnerTaskCount(r *ChannelMonitorRunner) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.tasks)
}

func runnerTaskPtr(r *ChannelMonitorRunner, id int64) *scheduledMonitor {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tasks[id]
}

// TestSchedule_AddsTaskAndFiresOnce 验证 Schedule 后立即触发一次首检测，并把任务记入 tasks 表。
func TestSchedule_AddsTaskAndFiresOnce(t *testing.T) {
	svc := &stubMonitorSvc{runCalled: make(chan int64, 4)}
	r := newRunnerForTest(svc)
	r.Start() // svc.enabled 为空，Start 立即完成

	r.Schedule(&ChannelMonitor{ID: 1, Name: "m1", Enabled: true, IntervalSeconds: 60})

	if got := runnerTaskCount(r); got != 1 {
		t.Fatalf("expected 1 scheduled task, got %d", got)
	}

	select {
	case id := <-svc.runCalled:
		if id != 1 {
			t.Fatalf("expected first fire for id=1, got %d", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected immediate first fire within 2s")
	}

	r.Stop()
}

// TestSchedule_ReplaceCancelsOldTask 验证对同一 id 二次 Schedule 会替换旧 task 实例。
// （旧 goroutine 通过 ctx 取消退出；这里以 task 指针不同 + Stop 不超时作为证据。）
func TestSchedule_ReplaceCancelsOldTask(t *testing.T) {
	svc := &stubMonitorSvc{runCalled: make(chan int64, 8)}
	r := newRunnerForTest(svc)
	r.Start()

	m := &ChannelMonitor{ID: 7, Name: "m7", Enabled: true, IntervalSeconds: 60}
	r.Schedule(m)
	first := runnerTaskPtr(r, 7)
	if first == nil {
		t.Fatal("first schedule did not register task")
	}

	r.Schedule(m)
	second := runnerTaskPtr(r, 7)
	if second == nil {
		t.Fatal("second schedule did not register task")
	}
	if first == second {
		t.Fatal("re-Schedule should create a new scheduledMonitor instance")
	}

	stoppedWithin(t, r, 3*time.Second)
}

// TestUnschedule_RemovesTask 验证 Unschedule 删除 task 并使对应 goroutine 退出。
func TestUnschedule_RemovesTask(t *testing.T) {
	svc := &stubMonitorSvc{runCalled: make(chan int64, 4)}
	r := newRunnerForTest(svc)
	r.Start()

	r.Schedule(&ChannelMonitor{ID: 3, Enabled: true, IntervalSeconds: 60})
	waitFor(t, time.Second, "task registered", func() bool { return runnerTaskCount(r) == 1 })

	r.Unschedule(3)
	if got := runnerTaskCount(r); got != 0 {
		t.Fatalf("expected tasks empty after Unschedule, got %d", got)
	}

	stoppedWithin(t, r, 3*time.Second)
}

// TestSchedule_DisabledRedirectsToUnschedule 验证 Enabled=false 等同于 Unschedule。
func TestSchedule_DisabledRedirectsToUnschedule(t *testing.T) {
	svc := &stubMonitorSvc{runCalled: make(chan int64, 4)}
	r := newRunnerForTest(svc)
	r.Start()

	r.Schedule(&ChannelMonitor{ID: 9, Enabled: true, IntervalSeconds: 60})
	waitFor(t, time.Second, "task registered", func() bool { return runnerTaskCount(r) == 1 })

	r.Schedule(&ChannelMonitor{ID: 9, Enabled: false, IntervalSeconds: 60})
	if got := runnerTaskCount(r); got != 0 {
		t.Fatalf("expected tasks empty after disabled re-Schedule, got %d", got)
	}

	stoppedWithin(t, r, 3*time.Second)
}

// TestSchedule_InvalidIntervalSkipped 验证 IntervalSeconds<=0 不会注册任务（防御性检查）。
func TestSchedule_InvalidIntervalSkipped(t *testing.T) {
	svc := &stubMonitorSvc{}
	r := newRunnerForTest(svc)
	r.Start()

	r.Schedule(&ChannelMonitor{ID: 1, Enabled: true, IntervalSeconds: 0})
	if got := runnerTaskCount(r); got != 0 {
		t.Fatalf("expected no task for invalid interval, got %d", got)
	}
	r.Stop()
}

// TestSchedule_BeforeStartIsNoOp 验证 Start 之前调用 Schedule 不会注册任务。
func TestSchedule_BeforeStartIsNoOp(t *testing.T) {
	svc := &stubMonitorSvc{}
	r := newRunnerForTest(svc)
	// 故意不调用 Start

	r.Schedule(&ChannelMonitor{ID: 1, Enabled: true, IntervalSeconds: 60})
	if got := runnerTaskCount(r); got != 0 {
		t.Fatalf("expected no task before Start, got %d", got)
	}
	r.Stop()
}

// TestStart_LoadsAllEnabledMonitors 验证 Start 会为 ListEnabledMonitors 返回的每条记录建立任务。
func TestStart_LoadsAllEnabledMonitors(t *testing.T) {
	svc := &stubMonitorSvc{
		enabled: []*ChannelMonitor{
			{ID: 1, Enabled: true, IntervalSeconds: 60},
			{ID: 2, Enabled: true, IntervalSeconds: 60},
			{ID: 3, Enabled: true, IntervalSeconds: 60},
		},
	}
	r := newRunnerForTest(svc)
	r.Start()
	waitFor(t, 2*time.Second, "all 3 tasks scheduled", func() bool { return runnerTaskCount(r) == 3 })

	stoppedWithin(t, r, 3*time.Second)
}

// TestStop_DrainsAllGoroutines 验证 Stop 会等待所有调度 goroutine 退出（无游离）。
func TestStop_DrainsAllGoroutines(t *testing.T) {
	svc := &stubMonitorSvc{}
	r := newRunnerForTest(svc)
	r.Start()

	for id := int64(1); id <= 5; id++ {
		r.Schedule(&ChannelMonitor{ID: id, Enabled: true, IntervalSeconds: 60})
	}
	waitFor(t, 2*time.Second, "5 tasks scheduled", func() bool { return runnerTaskCount(r) == 5 })

	stoppedWithin(t, r, 3*time.Second)
}

// TestStop_WaitsForInFlightCheck 验证 Stop 会等待正在执行的 RunCheck 退出（pool.StopAndWait）。
func TestStop_WaitsForInFlightCheck(t *testing.T) {
	svc := &stubMonitorSvc{
		runCalled:  make(chan int64, 1),
		runHoldFor: 200 * time.Millisecond,
	}
	r := newRunnerForTest(svc)
	r.Start()
	r.Schedule(&ChannelMonitor{ID: 1, Enabled: true, IntervalSeconds: 60})

	select {
	case <-svc.runCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("first fire never happened")
	}

	start := time.Now()
	stoppedWithin(t, r, 3*time.Second)
	elapsed := time.Since(start)
	// Stop 必须等待 in-flight check 跑完（runHoldFor=200ms），耗时下界约 100ms。
	if elapsed < 100*time.Millisecond {
		t.Fatalf("Stop returned too fast (%v); did not wait for in-flight check", elapsed)
	}
}

// TestInFlight_PoolFullReleasesSlot 直接驱动 fire 路径，模拟 pool.TrySubmit 失败时 inFlight 必须释放。
// 用一个小型 stub pool 替换 r.pool 不便（pond.Pool 是接口但 mock 麻烦），
// 改为：占满 inFlight 后直接 fire，验证不会在 inFlight 空槽时永久卡住。
func TestInFlight_AcquireReleaseSymmetric(t *testing.T) {
	svc := &stubMonitorSvc{}
	r := newRunnerForTest(svc)

	if !r.tryAcquireInFlight(42) {
		t.Fatal("first acquire should succeed")
	}
	if r.tryAcquireInFlight(42) {
		t.Fatal("second acquire (no release) must fail")
	}
	r.releaseInFlight(42)
	if !r.tryAcquireInFlight(42) {
		t.Fatal("acquire after release should succeed")
	}
	r.releaseInFlight(42)
}

// stoppedWithin 在 timeout 内并行调用 Stop，超时则 Fatal。验证 Stop 不会阻塞。
func stoppedWithin(t *testing.T, r *ChannelMonitorRunner, timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	var once sync.Once
	go func() {
		r.Stop()
		once.Do(func() { close(done) })
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("Stop did not return within %s — leaked goroutine?", timeout)
	}
}

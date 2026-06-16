package service

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zeromicro/go-zero/core/collection"
)

func TestNewTimingWheelService_InitFail_NoPanicAndReturnError(t *testing.T) {
	original := newTimingWheel
	t.Cleanup(func() { newTimingWheel = original })

	newTimingWheel = func(_ time.Duration, _ int, _ collection.Execute) (*collection.TimingWheel, error) {
		return nil, errors.New("boom")
	}

	svc, err := NewTimingWheelService()
	if err == nil {
		t.Fatalf("期望返回 error，但得到 nil")
	}
	if svc != nil {
		t.Fatalf("期望返回 nil svc，但得到非空")
	}
}

func TestNewTimingWheelService_Success(t *testing.T) {
	svc, err := NewTimingWheelService()
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	if svc == nil {
		t.Fatalf("期望 svc 非空，但得到 nil")
	}
	svc.Stop()
}

func TestNewTimingWheelService_ExecuteCallbackRunsFunc(t *testing.T) {
	original := newTimingWheel
	t.Cleanup(func() { newTimingWheel = original })

	var captured collection.Execute
	newTimingWheel = func(interval time.Duration, numSlots int, execute collection.Execute) (*collection.TimingWheel, error) {
		captured = execute
		return original(interval, numSlots, execute)
	}

	svc, err := NewTimingWheelService()
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	if captured == nil {
		t.Fatalf("期望 captured 非空，但得到 nil")
	}

	called := false
	captured("k", func() { called = true })
	if !called {
		t.Fatalf("期望 execute 回调触发传入函数执行")
	}

	svc.Stop()
}

func TestTimingWheelService_Schedule_ExecutesOnce(t *testing.T) {
	original := newTimingWheel
	t.Cleanup(func() { newTimingWheel = original })

	newTimingWheel = func(_ time.Duration, _ int, execute collection.Execute) (*collection.TimingWheel, error) {
		return original(10*time.Millisecond, 128, execute)
	}

	svc, err := NewTimingWheelService()
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	defer svc.Stop()

	ch := make(chan struct{}, 1)
	svc.Schedule("once", 30*time.Millisecond, func() { ch <- struct{}{} })

	select {
	case <-ch:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("等待任务执行超时")
	}

	select {
	case <-ch:
		t.Fatalf("任务不应重复执行")
	case <-time.After(80 * time.Millisecond):
	}
}

func TestTimingWheelService_Cancel_PreventsExecution(t *testing.T) {
	original := newTimingWheel
	t.Cleanup(func() { newTimingWheel = original })

	newTimingWheel = func(_ time.Duration, _ int, execute collection.Execute) (*collection.TimingWheel, error) {
		return original(10*time.Millisecond, 128, execute)
	}

	svc, err := NewTimingWheelService()
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	defer svc.Stop()

	ch := make(chan struct{}, 1)
	svc.Schedule("cancel", 80*time.Millisecond, func() { ch <- struct{}{} })
	svc.Cancel("cancel")

	select {
	case <-ch:
		t.Fatalf("任务已取消，不应执行")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestTimingWheelService_Schedule_AfterStop_LogsError(t *testing.T) {
	svc, err := NewTimingWheelService()
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	svc.Stop()

	// Stop 后调用 Schedule 应走 error 日志路径，不应 panic
	svc.Schedule("after-stop", 100*time.Millisecond, func() {
		t.Fatal("不应被执行")
	})
}

func TestTimingWheelService_ScheduleRecurring_AfterStop_LogsError(t *testing.T) {
	svc, err := NewTimingWheelService()
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	svc.Stop()

	// Stop 后调用 ScheduleRecurring 应走 error 日志路径，不应 panic
	svc.ScheduleRecurring("after-stop-rec", 100*time.Millisecond, func() {
		t.Fatal("不应被执行")
	})
}

func TestTimingWheelService_Stop_Idempotent(t *testing.T) {
	svc, err := NewTimingWheelService()
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	svc.Stop()
	svc.Stop() // 第二次调用不应 panic
}

func TestTimingWheelService_ScheduleRecurring_ExecutesMultipleTimes(t *testing.T) {
	original := newTimingWheel
	t.Cleanup(func() { newTimingWheel = original })

	newTimingWheel = func(_ time.Duration, _ int, execute collection.Execute) (*collection.TimingWheel, error) {
		return original(10*time.Millisecond, 128, execute)
	}

	svc, err := NewTimingWheelService()
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	defer svc.Stop()

	var count int32
	svc.ScheduleRecurring("rec", 30*time.Millisecond, func() { atomic.AddInt32(&count, 1) })

	deadline := time.Now().Add(500 * time.Millisecond)
	for atomic.LoadInt32(&count) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&count) < 2 {
		t.Fatalf("期望周期任务至少执行 2 次，但只执行了 %d 次", atomic.LoadInt32(&count))
	}
}

package handler

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// TestWrapReleaseOnDone_NoGoroutineLeak 验证 wrapReleaseOnDone 修复后不会泄露 goroutine
func TestWrapReleaseOnDone_NoGoroutineLeak(t *testing.T) {
	// 记录测试开始时的 goroutine 数量
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var releaseCount int32
	release := wrapReleaseOnDone(ctx, func() {
		atomic.AddInt32(&releaseCount, 1)
	})

	// 正常释放
	release()

	// 等待足够时间确保 goroutine 退出
	time.Sleep(200 * time.Millisecond)

	// 验证只释放一次
	if count := atomic.LoadInt32(&releaseCount); count != 1 {
		t.Errorf("expected release count to be 1, got %d", count)
	}

	// 强制 GC，清理已退出的 goroutine
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	// 验证 goroutine 数量没有增加（允许±2的误差，考虑到测试框架本身可能创建的 goroutine）
	finalGoroutines := runtime.NumGoroutine()
	if finalGoroutines > initialGoroutines+2 {
		t.Errorf("goroutine leak detected: initial=%d, final=%d, leaked=%d",
			initialGoroutines, finalGoroutines, finalGoroutines-initialGoroutines)
	}
}

// TestWrapReleaseOnDone_ContextCancellation 验证 context 取消时也能正确释放
func TestWrapReleaseOnDone_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var releaseCount int32
	_ = wrapReleaseOnDone(ctx, func() {
		atomic.AddInt32(&releaseCount, 1)
	})

	// 取消 context，应该触发释放
	cancel()

	// 等待释放完成
	time.Sleep(100 * time.Millisecond)

	// 验证释放被调用
	if count := atomic.LoadInt32(&releaseCount); count != 1 {
		t.Errorf("expected release count to be 1, got %d", count)
	}
}

// TestWrapReleaseOnDone_MultipleCallsOnlyReleaseOnce 验证多次调用 release 只释放一次
func TestWrapReleaseOnDone_MultipleCallsOnlyReleaseOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var releaseCount int32
	release := wrapReleaseOnDone(ctx, func() {
		atomic.AddInt32(&releaseCount, 1)
	})

	// 调用多次
	release()
	release()
	release()

	// 等待执行完成
	time.Sleep(100 * time.Millisecond)

	// 验证只释放一次
	if count := atomic.LoadInt32(&releaseCount); count != 1 {
		t.Errorf("expected release count to be 1, got %d", count)
	}
}

// TestWrapReleaseOnDone_NilReleaseFunc 验证 nil releaseFunc 不会 panic
func TestWrapReleaseOnDone_NilReleaseFunc(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	release := wrapReleaseOnDone(ctx, nil)

	if release != nil {
		t.Error("expected nil release function when releaseFunc is nil")
	}
}

// TestWrapReleaseOnDone_ConcurrentCalls 验证并发调用的安全性
func TestWrapReleaseOnDone_ConcurrentCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var releaseCount int32
	release := wrapReleaseOnDone(ctx, func() {
		atomic.AddInt32(&releaseCount, 1)
	})

	// 并发调用 release
	const numGoroutines = 10
	for i := 0; i < numGoroutines; i++ {
		go release()
	}

	// 等待所有 goroutine 完成
	time.Sleep(200 * time.Millisecond)

	// 验证只释放一次
	if count := atomic.LoadInt32(&releaseCount); count != 1 {
		t.Errorf("expected release count to be 1, got %d", count)
	}
}

// BenchmarkWrapReleaseOnDone 性能基准测试
func BenchmarkWrapReleaseOnDone(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		release := wrapReleaseOnDone(ctx, func() {})
		release()
	}
}

//go:build unit

package safe

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// withCapturedLogger 临时把全局 slog 默认 logger 替换为写入 buffer 的
// text handler，返回 buffer 与还原函数。便于在测试中断言日志输出。
func withCapturedLogger(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	buf := &bytes.Buffer{}
	handler := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	prev := slog.Default()
	slog.SetDefault(slog.New(handler))
	return buf, func() { slog.SetDefault(prev) }
}

// TestRun_NoPanic 验证 fn 在无 panic 情况下被正常执行，且不写入 panic 日志。
func TestRun_NoPanic(t *testing.T) {
	buf, restore := withCapturedLogger(t)
	defer restore()

	called := false
	Run("test.normal", []slog.Attr{slog.String("k", "v")}, func() {
		called = true
	})

	if !called {
		t.Fatal("expected fn to be called")
	}
	if strings.Contains(buf.String(), ".panic") {
		t.Fatalf("expected no panic log, got: %q", buf.String())
	}
}

// TestRun_RecoversPanic 验证 Run 同步捕获 panic，并写出包含
// event.panic / panic=boom / account_id=42 / stack= 的结构化日志。
func TestRun_RecoversPanic(t *testing.T) {
	buf, restore := withCapturedLogger(t)
	defer restore()

	// 不应该向上传播 panic
	Run("test.event", []slog.Attr{slog.Int64("account_id", 42)}, func() {
		panic("boom")
	})

	output := buf.String()
	wantSubstrs := []string{
		"test.event.panic",
		"panic=boom",
		"account_id=42",
		"stack=",
	}
	for _, want := range wantSubstrs {
		if !strings.Contains(output, want) {
			t.Errorf("expected log to contain %q, full output:\n%s", want, output)
		}
	}
}

// TestGo_RecoversPanicAsync 验证 Go 在 goroutine 中捕获 panic，
// goroutine 能正常退出（不会让测试进程 crash），且日志被写入。
func TestGo_RecoversPanicAsync(t *testing.T) {
	buf, restore := withCapturedLogger(t)
	defer restore()

	done := make(chan struct{})
	// wg 用于确保 fn 已被调用（Go 立刻返回，需要手动同步退出信号）
	var wg sync.WaitGroup
	wg.Add(1)

	Go("test.async", []slog.Attr{slog.String("worker", "w1")}, func() {
		defer wg.Done()
		defer close(done)
		panic("async-boom")
	})

	select {
	case <-done:
		// goroutine 完成了 panic（done close 在 panic 之前，所以一定会触发）
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not finish within 2s")
	}

	// 等待 panic recover + log 写入完成
	wg.Wait()

	// 给 slog handler 一点点时间 flush（text handler 是同步的，但 defer 顺序保险起见）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "test.async.panic") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	output := buf.String()
	if !strings.Contains(output, "test.async.panic") {
		t.Errorf("expected log to contain %q, full output:\n%s", "test.async.panic", output)
	}
	if !strings.Contains(output, "panic=async-boom") {
		t.Errorf("expected log to contain %q, full output:\n%s", "panic=async-boom", output)
	}
}

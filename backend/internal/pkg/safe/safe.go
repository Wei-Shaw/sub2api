// Package safe 提供 panic-recovering 的 goroutine 辅助函数。
//
// 设计目的：
//   - 业务代码中常见 `go func() { ... }()` 模式，一旦 panic 会导致整个进程退出。
//   - 本包统一捕获 panic，记录结构化日志（含调用方上下文 attrs + panic 值 + 堆栈），
//     避免单个 goroutine 崩溃影响主流程。
package safe

import (
	"log/slog"
	"runtime/debug"
)

// panicSuffix 是 panic 事件名后缀，便于日志检索。
const panicSuffix = ".panic"

// Go 在新 goroutine 中执行 fn，捕获 panic 并以 event+".panic" 名字记录到 slog。
//
// attrs 在调用点求值并以切片形式传入，避免 goroutine 闭包陷阱
// （调用方变量被异步求值时可能已被修改）。
//
// 用法：
//
//	safe.Go("sidecar_probe.startup", []slog.Attr{
//	    slog.Int64("account_id", acc.ID),
//	}, func() {
//	    probeAccount(acc)
//	})
func Go(event string, attrs []slog.Attr, fn func()) {
	go Run(event, attrs, fn)
}

// Run 同步执行 fn，同样捕获 panic。给已经自管 goroutine 的调用方使用
// （waitgroup/errgroup pattern）。
//
// 用法：
//
//	wg.Add(1)
//	go func() {
//	    defer wg.Done()
//	    safe.Run("worker.tick", []slog.Attr{slog.Int("worker_id", id)}, func() {
//	        doWork()
//	    })
//	}()
func Run(event string, attrs []slog.Attr, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			logPanic(event, attrs, r)
		}
	}()
	fn()
}

// logPanic 以结构化方式记录 panic，附加调用方 attrs + panic 值 + 堆栈。
// 单独提取以便测试覆盖与未来扩展（如指标上报）。
func logPanic(event string, attrs []slog.Attr, panicValue any) {
	// 复制一份 attrs 避免修改调用方切片
	logAttrs := make([]slog.Attr, 0, len(attrs)+2)
	logAttrs = append(logAttrs, attrs...)
	logAttrs = append(logAttrs,
		slog.Any("panic", panicValue),
		slog.String("stack", string(debug.Stack())),
	)
	slog.LogAttrs(nil, slog.LevelError, event+panicSuffix, logAttrs...)
}

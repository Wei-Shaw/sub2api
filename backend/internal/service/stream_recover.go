package service

// 流式处理协程的 panic 兜底。
//
// 背景：gin 的 Recovery() 中间件只包裹**请求协程**，不覆盖请求内派生的子协程。
// 网关里大量“泵”协程（SSE 行泵、io.Pipe 转换、WebSocket 读泵）处理的是完全由
// 上游/客户端控制的字节流，一旦解析代码越界 panic，整个进程会退出，所有在途请求
// 一起被丢弃——而不是只失败这一条请求。
//
// 仅仅 recover 是不够的：这些协程的消费方通常正阻塞在一个 channel / io.Pipe /
// errCh 上，靠协程自己在退出前投递结果来解除阻塞。如果 recover 之后不做任何事，
// “进程崩溃”就变成了“请求永久挂起”，并不更好。因此本文件提供的兜底一律要求调用方
// 给出 onPanic 清理逻辑，用来把 panic 转换成消费方能观测到的错误。
//
// defer 顺序（LIFO）很关键：recover 必须**晚于**“解除阻塞”的 defer 注册
// （例如 `defer close(events)` 之后再 `defer recoverStreamGoroutine(...)`），
// 这样 panic 时先投递错误事件、再关闭通道，消费方才能看到错误而不是一个“正常结束”。
// 参见 bedrock_stream.go 中的同类写法。

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// streamRecoverScope 是流处理协程 panic 的统一日志 scope。
const streamRecoverScope = "service.gateway"

// recoverStreamGoroutine 是流处理协程的通用 panic 兜底，必须以 defer 直接调用
// （recover 只有被 defer 的函数直接调用才生效，不能再包一层）。
//
//	go func() {
//	    defer close(events)                                   // 先注册：最后执行
//	    defer recoverStreamGoroutine("xxx pump", func(err error) {
//	        _ = sendEvent(scanEvent{err: err})                 // 后注册：先执行
//	    })
//	    ...
//	}()
//
// onPanic 只在真的发生 panic 时被调用，正常返回路径完全不受影响；它拿到的 err
// 已经包含 name 与 panic 值，调用方可以直接把它交给消费方。
func recoverStreamGoroutine(name string, onPanic func(err error)) {
	if recovered := recover(); recovered != nil {
		err := reportStreamPanic(name, recovered)
		if onPanic != nil {
			onPanic(err)
		}
	}
}

// recoverStreamPipeWriter 是 io.Pipe 转换协程专用的兜底：panic 时必须用
// CloseWithError 关闭写端，否则读端（通常是 http.Response.Body 的消费者）
// 会永远阻塞在 Read 上。io.Pipe 的错误是“先写先得”，因此即便调用方还有
// `defer writer.Close()`，这里投递的错误也不会被后续的 EOF 覆盖。
func recoverStreamPipeWriter(name string, writer *io.PipeWriter) {
	if recovered := recover(); recovered != nil {
		err := reportStreamPanic(name, recovered)
		if writer != nil {
			_ = writer.CloseWithError(err)
		}
	}
}

// reportStreamPanic 记录 panic 与调用栈，并把它转换成可以投递给消费方的 error。
func reportStreamPanic(name string, recovered any) error {
	logger.LegacyPrintf(streamRecoverScope,
		"[StreamRecover] panic in %s: %v\n%s", name, recovered, debug.Stack())
	return fmt.Errorf("panic in %s: %v", name, recovered)
}

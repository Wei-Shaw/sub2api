package openai_ws_v2

// WebSocket 中继协程的 panic 兜底。
//
// 语义与 internal/service/stream_recover.go 完全一致，但本包位于 service 的下游
// （service 导入 openai_ws_v2），无法反向导入，因此在这里保留一份最小实现。
//
// 与 service 侧同理：gin 的 Recovery() 不覆盖这些子协程，而中继的消费方阻塞在
// exitCh 上，靠协程退出前投递 relayExitSignal 解除阻塞。所以 recover 之后必须把
// panic 转成一次退出信号，否则 panic 会从“进程退出”变成“连接永久挂起”。

import (
	"fmt"
	"runtime/debug"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// relayRecoverScope 是中继协程 panic 的统一日志 scope。
const relayRecoverScope = "service.openai_ws_v2"

// recoverRelayGoroutine 必须以 defer 直接调用；onPanic 只在发生 panic 时执行，
// 用于向消费方投递错误（通常是 exitCh <- relayExitSignal{...}）。
func recoverRelayGoroutine(name string, onPanic func(err error)) {
	if recovered := recover(); recovered != nil {
		logger.LegacyPrintf(relayRecoverScope,
			"[StreamRecover] panic in %s: %v\n%s", name, recovered, debug.Stack())
		if onPanic != nil {
			onPanic(fmt.Errorf("panic in %s: %v", name, recovered))
		}
	}
}

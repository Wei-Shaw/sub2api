// stream.go 是上游 Connect 流的消费侧：泵协程把阻塞的帧读取归一化为
// channel 序列，responseStream 在其上运行 start 扣留 / 静默看门狗 /
// pre-content 重开 / 空轮续传。语义逐段对齐 devin2api responseStream。
package adapter

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// upstreamStallTimeout 是相邻两个上游帧之间允许的最长静默；超时即判定
// 传输层已死（半开连接、上游挂死），按传输错误收尾而不是无限等待。
// 取值需高于上游首批帧的实测延迟（长思考可达 45s+）。
// var 而非 const：测试临时缩短它来覆盖超时路径。
var upstreamStallTimeout = 120 * time.Second

// startHoldTimeout 是 start 事件（message_start/response.created）允许被
// 扣留的最长时间。扣留的目的是给上游「产出内容前就失败」留一个返回真实
// HTTP 状态码的窗口——实测这类失败全部在 ~9s 内落定；而下游客户端在
// ~30s 无数据时弃连，且中间网关只在首个协议事件后才向客户端放通字节
// （保活注释行不算）。15s 位于两者之间：快速失败仍拿到真实状态码，
// 长思考则先把 start 发出去让客户端保持存活。
var startHoldTimeout = 15 * time.Second

// upstreamFrameBuffer 是泵协程可超前读取的帧数：上游生产与客户端
// 消费解耦，同时保留对上游的背压上限。
const upstreamFrameBuffer = 64

// upstreamFrame 是泵协程的一次产出：response 为正常数据帧；
// response 为 nil 表示流终止，err 为终止错误（正常 EOF 时为 nil）。
type upstreamFrame struct {
	response *devin.ChatResponseFrame
	err      error
}

// pumpUpstream 把阻塞的帧读取归一化为 channel 帧序列：Reader.Next 只能被
// ctx 取消打断，交给协程后 Recv 才能在等待期间响应静默看门狗与客户端断开。
// 流终止时终止错误作为最后一帧无条件投递：终帧只有一帧，阻塞等消费方
// 排空缓冲，丢弃它会以「正常 EOF」的形态吃掉真实流错误（含静默截断）。
// 所有发送带 ctx.Done 分支：消费方放弃后协程必须能退出。
func pumpUpstream(ctx context.Context, conn streamConn) <-chan upstreamFrame {
	frames := make(chan upstreamFrame, upstreamFrameBuffer)
	go func() {
		defer close(frames)
		defer conn.body.Close()
		for conn.reader.Next() {
			frame := conn.reader.Frame()
			if frame.End {
				// Connect end 帧：trailer 可携带 {"error":{...}}。
				terminal := devin.ParseEndTrailer(frame.Payload)
				select {
				case frames <- upstreamFrame{err: terminal}:
				case <-ctx.Done():
				}
				return
			}
			decoded := devin.DecodeChatResponseFrame(frame.Payload)
			select {
			case frames <- upstreamFrame{response: decoded}:
			case <-ctx.Done():
				return
			}
		}
		select {
		case frames <- upstreamFrame{err: conn.reader.Err()}:
		case <-ctx.Done():
		}
	}()
	return frames
}

// responseStream 从泵协程读取上游帧并依次返回 decoder 生成的事件。
type responseStream struct {
	// frames 是泵协程产出的上游帧通道；终止帧 response 为 nil。
	frames <-chan upstreamFrame
	// cancel 中止上游流：看门狗判死、客户端 ctx 取消或流正常结束时调用，
	// 打断泵协程内可能仍阻塞的读取。
	cancel context.CancelFunc
	// decoder 将一个 Devin protobuf 帧转换为零个或多个中间响应事件。
	decoder *responseDecoder
	// started 表示是否已经请求 decoder 产生 start 事件。
	started bool
	// pendingStart 保存 decoder.start() 生成但尚未下发的事件。
	// start 推迟到第一批真实事件前发出：上游在产出内容前报错时，
	// 首个对外事件是 error，HTTP 层才能返回真实错误状态码，
	// 而不是已提交的 200 + SSE error（下游网关会把后者误判为渠道故障）。
	// 扣留上限由 startHold 控制：超时后 start 单独下发。
	pendingStart []llm.ResponseEvent
	// startHold 在 pendingStart 填充时武装，到期释放扣留的 start。
	startHold *time.Timer
	// startReleased 标记 start 已下发给客户端：pre-content 重试重建
	// 解码器后必须丢弃新 start，否则客户端会收到第二个 message_start。
	startReleased bool
	// finished 表示 decoder 已经生成最终事件，不再读取上游。
	finished bool
	// queue 保存已经转换、等待调用方读取的中间响应事件。
	queue []llm.ResponseEvent
	// producedEvents 表示上游帧已产出过任何事件：一旦为真说明内容已
	// 开始对外流动，此后失败只能透传，不能整体重发。
	producedEvents bool
	// retried 表示已经做过一次 pre-content 整体重试（上限 1 次）。
	retried bool
	// reopen 在可重试的 pre-content 失败（传输断裂、静默看门狗判死）
	// 时重发请求并返回新泵；continueEmpty 表示空 end_turn 续传：
	// 追加 "continue" 用户消息。
	reopen func(cause error, continueEmpty bool) (<-chan upstreamFrame, context.CancelFunc, error)
	// newDecoder 重建响应解码器供重试使用；nil 时不可重试。
	newDecoder func() *responseDecoder
	// stall 是跨 Recv 复用的静默看门狗计时器；首次等待时创建。
	stall *time.Timer
}

// Recv 返回下一个中间响应事件；流尽返回 io.EOF。
func (stream *responseStream) Recv(ctx context.Context) (llm.ResponseEvent, error) {
	// 静默计时器挂在流上跨 Recv 复用：每次入等待循环前 Reset 覆盖
	// 帧间隔。Go 1.23+ 计时器通道无缓冲，Stop/Reset 后不会投递陈旧触发，
	// 已触发（stall.C 分支）的计时器 Reset 重新武装即可。
	stall := stream.stall
	if stall == nil {
		stall = time.NewTimer(upstreamStallTimeout)
		stream.stall = stall
	} else {
		stall.Reset(upstreamStallTimeout)
	}
	defer stall.Stop()
	for len(stream.queue) == 0 && !stream.finished {
		if err := ctx.Err(); err != nil {
			return llm.ResponseEvent{}, err
		}
		if !stream.started {
			// start() 初始化 decoder.partial，必须先于 decode 调用；
			// 事件本身扣留在 pendingStart，等待第一批真实事件一起下发。
			stream.started = true
			stream.pendingStart = stream.decoder.start()
			if stream.startReleased {
				// 重试流上客户端已见过一个 start，重复下发会违反协议。
				stream.pendingStart = nil
			} else if stream.startHold == nil {
				stream.startHold = time.NewTimer(startHoldTimeout)
			} else {
				stream.startHold.Reset(startHoldTimeout)
			}
			continue
		}
		stall.Reset(upstreamStallTimeout)
		var startHold <-chan time.Time
		if stream.startHold != nil {
			startHold = stream.startHold.C
		}
		select {
		case <-startHold:
			// 上游建流后静默超时：先把扣留的 start 发出去——对客户端
			// 这是首个可见字节，链路各段的空闲计时器随之刷新。
			if len(stream.pendingStart) == 0 {
				continue
			}
			stream.queue = stream.pendingStart
			stream.pendingStart = nil
			stream.startReleased = true
			continue
		case frame, ok := <-stream.frames:
			stall.Stop()
			if !ok || frame.response == nil {
				var upstreamErr error
				if ok {
					upstreamErr = frame.err
				} else if ctxErr := context.Cause(ctx); ctxErr != nil {
					// ok==false 只剩「泵协程随 ctx 取消退出」一种来源
					//（终帧无条件投递）。把取消透传给 finish，避免以
					// 正常 EOF 的形态吞掉被截断的流。
					upstreamErr = ctxErr
				}
				if upstreamErr != nil && stream.tryReopen(upstreamErr, false) {
					continue
				}
				events := stream.release(stream.decoder.finish(upstreamErr))
				if upstreamErr == nil && emptyEndTurn(events) && stream.tryReopen(nil, true) {
					continue
				}
				stream.queue = events
				stream.finished = true
				continue
			}
			events := stream.decoder.decode(frame.response)
			if len(events) > 0 {
				stream.producedEvents = true
			}
			stream.queue = stream.release(events)
			stream.finished = stream.decoder.finished
		case <-stall.C:
			// 上游静默超时：取消底层流打断泵协程；已缓冲未消费的帧
			// 丢弃，然后按传输错误收尾。
			stream.cancel()
			stallErr := fmt.Errorf("devin stream stalled: no frames for %s", upstreamStallTimeout)
			if stream.tryReopen(stallErr, false) {
				continue
			}
			stream.queue = stream.release(stream.decoder.finish(stallErr))
			stream.finished = true
		case <-ctx.Done():
			stall.Stop()
			stream.cancel()
			stream.queue = stream.release(stream.decoder.finish(context.Cause(ctx)))
			stream.finished = true
		}
	}
	if stream.finished {
		stream.cancel()
	}
	if len(stream.queue) > 0 {
		event := stream.queue[0]
		stream.queue = stream.queue[1:]
		return event, nil
	}
	return llm.ResponseEvent{}, io.EOF
}

// tryReopen 在「上游已失败但尚未产出任何内容」时整体重发请求一次：
// 此时客户端只见过扣留的 start 事件，重发没有可见副作用。返回 true
// 表示新流已接管，调用方重置解码器后继续消费。
// continueEmpty 为空 end_turn 续传：流正常结束但零内容时重发并
// 追加 "continue" 用户消息（空轮是上游实测退化形态）。
func (stream *responseStream) tryReopen(cause error, continueEmpty bool) bool {
	if stream.retried || stream.producedEvents || stream.reopen == nil {
		return false
	}
	if cause == nil && !continueEmpty {
		return false
	}
	frames, cancel, err := stream.reopen(cause, continueEmpty)
	if err != nil {
		return false
	}
	stream.retried = true
	stream.frames = frames
	stream.cancel = cancel
	if stream.newDecoder != nil {
		stream.decoder = stream.newDecoder()
	}
	stream.started = false
	stream.pendingStart = nil
	stream.finished = false
	stream.queue = nil
	return true
}

// emptyEndTurn 判断 finish 产出的事件是否构成「正常 stop 但零内容」：
// 上游偶发直接以 stopReason 收尾且不带任何 delta。StopSequence 不算——
// 零内容命中停止序列更可能是预期的截断而非退化轮。
func emptyEndTurn(events []llm.ResponseEvent) bool {
	for _, event := range events {
		if event.Type != llm.ResponseEventDone {
			continue
		}
		return event.Message != nil &&
			event.Message.StopReason == llm.StopReasonStop &&
			len(event.Message.Content) == 0
	}
	return false
}

// release 把 decoder 产出的第一批事件交给调用方：非错误批次前置扣留的
// start 事件；若首批就是错误事件（上游在产出内容前失败），丢弃 start，
// 让错误成为流的第一个对外事件。
func (stream *responseStream) release(events []llm.ResponseEvent) []llm.ResponseEvent {
	if len(events) == 0 || len(stream.pendingStart) == 0 {
		return events
	}
	start := stream.pendingStart
	stream.pendingStart = nil
	stream.startReleased = true
	if events[0].Type == llm.ResponseEventError {
		return events
	}
	return append(start, events...)
}

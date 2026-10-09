package service

// 国产供应商 Anthropic 协议转换路径的上游 SSE 行泵。
//
// 这两条转换链（CC×anthropic / Responses×anthropic）的上游 ctx 是
// WithoutCancel（detachStreamUpstreamContext）、http.Client 无整体 Timeout，
// 客户端断开后的排水阶段若上游挂住 SSE（不发数据也不断连），
// scanner.Scan() 将永久阻塞：goroutine 钉死、resp.Body 不归还、连接池位
// 被占用、usage 永不落库。
//
// 与 handleAnthropicStreamingResponse / readOpenAICompatBufferedTerminal 的
// 同类排水一致，本泵用 gateway.stream_data_interval_timeout（默认 180s）作为
// 逐行读间隔上限，超时即向调用方返回 errAnthropicNativeStreamIdle，由调用方
// 关闭 resp.Body 解除阻塞的读并结束排水。

import (
	"bufio"
	"context"
	"errors"
	"io"
	"time"
)

// errAnthropicNativeStreamIdle 表示上游流读间隔超时（见上方文件注释）。
var errAnthropicNativeStreamIdle = errors.New("stream data interval timeout")

// A disconnected client still needs a bounded drain even when the operator
// disabled the normal stream interval timeout. Keep the fallback aligned with
// the documented default so slow thinking streams have time to finish while a
// stalled upstream cannot pin the request forever.
const defaultAnthropicNativeDisconnectDrainTimeout = 180 * time.Second

// anthropicNativeLineEvent 是行泵交付的单次读取结果：line 为一行 SSE 文本，
// err 为 scanner 读错误（流自然结束时 next 返回 io.EOF，不经过本字段）。
type anthropicNativeLineEvent struct {
	line string
	err  error
}

// anthropicNativeLinePump 以独立 goroutine 泵送 scanner 的行，并对逐行到达
// 间隔施加 interval 上限（<=0 表示禁用，保持无界读的旧行为）。
type anthropicNativeLinePump struct {
	events   chan anthropicNativeLineEvent
	done     chan struct{}
	timer    *time.Timer
	interval time.Duration
	// clientDisconnectSeen is only accessed by the handler goroutine through
	// nextWithContext; the scanner goroutine never touches it.
	clientDisconnectSeen bool
}

// newAnthropicNativeLinePump 启动泵 goroutine；调用方 defer pump.stop()。
func newAnthropicNativeLinePump(scanner *bufio.Scanner, interval time.Duration) *anthropicNativeLinePump {
	p := &anthropicNativeLinePump{
		events:   make(chan anthropicNativeLineEvent, 16),
		done:     make(chan struct{}),
		interval: interval,
	}
	if interval > 0 {
		p.timer = time.NewTimer(interval)
	}
	go func() {
		defer close(p.events)
		for scanner.Scan() {
			select {
			case p.events <- anthropicNativeLineEvent{line: scanner.Text()}:
			case <-p.done:
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case p.events <- anthropicNativeLineEvent{err: err}:
			case <-p.done:
			}
		}
	}()
	return p
}

// next 阻塞返回下一行。返回 io.EOF 表示上游正常收流；errAnthropicNativeStreamIdle
// 表示 interval 内无任何数据到达（计时从收到上一行时起算，事件处理耗时不算入，
// 与 readOpenAICompatBufferedTerminal 的 resetTimeout 语义一致）。
func (p *anthropicNativeLinePump) next() (string, error) {
	var timeoutCh <-chan time.Time
	if p.timer != nil {
		timeoutCh = p.timer.C
	}
	select {
	case ev, ok := <-p.events:
		if !ok {
			return "", io.EOF
		}
		p.resetTimer()
		return ev.line, ev.err
	case <-timeoutCh:
		return "", errAnthropicNativeStreamIdle
	}
}

// nextWithContext is the context-aware variant used by compatibility handlers.
// A canceled client context marks the drain as disconnected and enables the
// disconnect-only fallback interval before continuing to consume upstream
// events. This keeps a silent detached upstream from blocking forever when the
// normal stream interval timeout is disabled.
func (p *anthropicNativeLinePump) nextWithContext(ctx context.Context, onDisconnect func()) (string, error) {
	if ctx == nil || p.clientDisconnectSeen {
		return p.next()
	}
	markDisconnected := func() {
		p.clientDisconnectSeen = true
		p.enableInterval(defaultAnthropicNativeDisconnectDrainTimeout)
		if onDisconnect != nil {
			onDisconnect()
		}
	}
	clientDone := ctx.Done()
	if ctx.Err() != nil {
		markDisconnected()
		return p.next()
	}

	var timeoutCh <-chan time.Time
	if p.timer != nil {
		timeoutCh = p.timer.C
	}
	select {
	case ev, ok := <-p.events:
		if ctx.Err() != nil {
			markDisconnected()
		}
		if !ok {
			return "", io.EOF
		}
		p.resetTimer()
		return ev.line, ev.err
	case <-timeoutCh:
		if ctx.Err() != nil {
			markDisconnected()
		}
		return "", errAnthropicNativeStreamIdle
	case <-clientDone:
		markDisconnected()
		// Continue the same line read, including when cancellation occurs between
		// an SSE event line and its data line, so final usage is never skipped.
		return p.next()
	}
}

// resetTimer 在收到一行后重启间隔计时器。
func (p *anthropicNativeLinePump) resetTimer() {
	if p.timer == nil {
		return
	}
	if !p.timer.Stop() {
		select {
		case <-p.timer.C:
		default:
		}
	}
	p.timer.Reset(p.interval)
}

// enableInterval turns on an idle timeout after a downstream disconnect. It
// is called by the handler goroutine, which is also the only goroutine that
// reads or resets p.timer.
func (p *anthropicNativeLinePump) enableInterval(interval time.Duration) {
	if p.timer != nil || interval <= 0 {
		return
	}
	p.interval = interval
	p.timer = time.NewTimer(interval)
}

// stop 终止泵 goroutine。注意：goroutine 若正阻塞在 scanner.Read 上，需由
// 调用方关闭 resp.Body（间隔超时分支已做）才能真正退出。
func (p *anthropicNativeLinePump) stop() {
	close(p.done)
	if p.timer != nil {
		if !p.timer.Stop() {
			select {
			case <-p.timer.C:
			default:
			}
		}
	}
}

// anthropicNativeStreamInterval 返回本组转换路径适用的读间隔上限；
// gateway.stream_data_interval_timeout <= 0 时视为禁用。
func (s *OpenAIGatewayService) anthropicNativeStreamInterval() time.Duration {
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		return time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	return 0
}

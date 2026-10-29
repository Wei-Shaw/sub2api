package service

// Anthropic 协议转换路径的上游 SSE 行泵，用于 OpenAI 侧国产供应商的
// CC×anthropic / Responses×anthropic 转换链，以及 Anthropic 账号的
// chat/responses 兼容入口（anthropicCompatStream）。
//
// 这些转换链的上游 ctx 是
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
	"errors"
	"io"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// errAnthropicNativeStreamIdle 表示上游流读间隔超时（见上方文件注释）。
var errAnthropicNativeStreamIdle = errors.New("stream data interval timeout")

// errAnthropicNativeClientGone 表示等待上游数据期间客户端已离开；泵状态不变，
// 调用方可继续读取。
var errAnthropicNativeClientGone = errors.New("client gone")

// anthropicNativeLineEvent 是行泵交付的单次读取结果：line 为一行 SSE 文本，
// err 为 scanner 读错误（流自然结束时 next 返回 io.EOF，不经过本字段）。
type anthropicNativeLineEvent struct {
	line string
	err  error
}

// anthropicNativeLinePump 以独立 goroutine 泵送 scanner 的行，并对逐行到达
// 间隔施加 interval 上限（<=0 表示禁用，可经 enableInterval 启用）。
type anthropicNativeLinePump struct {
	events   chan anthropicNativeLineEvent
	done     chan struct{}
	timer    *time.Timer
	interval time.Duration
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
// 表示自交付上一行起已过 interval 且没有可交付的行；计时器到期时若已有缓冲的行，
// 优先交付该行，调用方处理事件期间上游送达的数据不会被判为上游空闲。
func (p *anthropicNativeLinePump) next() (string, error) {
	return p.nextOrClientGone(nil)
}

// nextOrClientGone 与 next 相同，另在等待期间 clientGone 关闭时返回
// errAnthropicNativeClientGone。clientGone 为 nil 时不监听。
func (p *anthropicNativeLinePump) nextOrClientGone(clientGone <-chan struct{}) (string, error) {
	var timeoutCh <-chan time.Time
	if p.timer != nil {
		timeoutCh = p.timer.C
	}
	select {
	case ev, ok := <-p.events:
		return p.deliver(ev, ok)
	case <-timeoutCh:
		select {
		case ev, ok := <-p.events:
			return p.deliver(ev, ok)
		default:
			return "", errAnthropicNativeStreamIdle
		}
	case <-clientGone:
		return "", errAnthropicNativeClientGone
	}
}

// deliver 交付一次读取结果；ok=false 表示泵 goroutine 已结束。
func (p *anthropicNativeLinePump) deliver(ev anthropicNativeLineEvent, ok bool) (string, error) {
	if !ok {
		return "", io.EOF
	}
	p.resetTimer()
	return ev.line, ev.err
}

// enableInterval 在读间隔上限未启用时以 interval 启用，计时从调用时起算；
// 已启用时不变。只能在调用 next 的 goroutine 中调用。
func (p *anthropicNativeLinePump) enableInterval(interval time.Duration) {
	if p.timer != nil || interval <= 0 {
		return
	}
	p.interval = interval
	p.timer = time.NewTimer(interval)
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

// anthropicNativeStreamInterval 返回 Anthropic 上游协议转换路径适用的读间隔上限；
// gateway.stream_data_interval_timeout <= 0 时视为禁用。
func anthropicNativeStreamInterval(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.Gateway.StreamDataIntervalTimeout > 0 {
		return time.Duration(cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	return 0
}

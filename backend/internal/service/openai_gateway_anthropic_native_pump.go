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
	"errors"
	"io"
	"time"
)

// errAnthropicNativeStreamIdle 表示上游流读间隔超时（见上方文件注释）。
var errAnthropicNativeStreamIdle = errors.New("stream data interval timeout")

// anthropicNativeLineEvent 是行泵交付的单次读取结果：line 为一行 SSE 文本，
// err 为 scanner 读错误（流自然结束时 next 返回 io.EOF，不经过本字段）。
type anthropicNativeLineEvent struct {
	line string
	err  error
REDACTED

// anthropicNativeLinePump 以独立 goroutine 泵送 scanner 的行，并对逐行到达
// 间隔施加 interval 上限（<=0 表示禁用，保持无界读的旧行为）。
type anthropicNativeLinePump struct {
	events   chan anthropicNativeLineEvent
	done     chan struct{REDACTED
	timer    *time.Timer
	interval time.Duration
REDACTED

// newAnthropicNativeLinePump 启动泵 goroutine；调用方 defer pump.stop()。
func newAnthropicNativeLinePump(scanner *bufio.Scanner, interval time.Duration) *anthropicNativeLinePump {
	p := &anthropicNativeLinePump{
		events:   make(chan anthropicNativeLineEvent, 16),
		done:     make(chan struct{REDACTED),
		interval: interval,
REDACTED
	if interval > 0 {
		p.timer = time.NewTimer(interval)
REDACTED
	go func() {
		defer close(p.events)
		for scanner.Scan() {
			select {
			case p.events <- anthropicNativeLineEvent{line: scanner.Text()REDACTED:
			case <-p.done:
				return
		REDACTED
	REDACTED
		if err := scanner.Err(); err != nil {
			select {
			case p.events <- anthropicNativeLineEvent{err: errREDACTED:
			case <-p.done:
		REDACTED
	REDACTED
REDACTED()
	return p
REDACTED

// next 阻塞返回下一行。返回 io.EOF 表示上游正常收流；errAnthropicNativeStreamIdle
// 表示 interval 内无任何数据到达（计时从收到上一行时起算，事件处理耗时不算入，
// 与 readOpenAICompatBufferedTerminal 的 resetTimeout 语义一致）。
func (p *anthropicNativeLinePump) next() (string, error) {
	var timeoutCh <-chan time.Time
	if p.timer != nil {
		timeoutCh = p.timer.C
REDACTED
	select {
	case ev, ok := <-p.events:
		if !ok {
			return "", io.EOF
	REDACTED
		p.resetTimer()
		return ev.line, ev.err
	case <-timeoutCh:
		return "", errAnthropicNativeStreamIdle
REDACTED
REDACTED

// resetTimer 在收到一行后重启间隔计时器。
func (p *anthropicNativeLinePump) resetTimer() {
	if p.timer == nil {
		return
REDACTED
	if !p.timer.Stop() {
		select {
		case <-p.timer.C:
		default:
	REDACTED
REDACTED
	p.timer.Reset(p.interval)
REDACTED

// stop 终止泵 goroutine。注意：goroutine 若正阻塞在 scanner.Read 上，需由
// 调用方关闭 resp.Body（间隔超时分支已做）才能真正退出。
func (p *anthropicNativeLinePump) stop() {
	close(p.done)
	if p.timer != nil {
		if !p.timer.Stop() {
			select {
			case <-p.timer.C:
			default:
		REDACTED
	REDACTED
REDACTED
REDACTED

// anthropicNativeStreamInterval 返回本组转换路径适用的读间隔上限；
// gateway.stream_data_interval_timeout <= 0 时视为禁用。
func (s *OpenAIGatewayService) anthropicNativeStreamInterval() time.Duration {
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		return time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
REDACTED
	return 0
REDACTED

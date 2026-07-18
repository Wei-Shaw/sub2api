package inbox

import "sync/atomic"

// Metrics 是信箱模块的可观测性打点接口。默认使用 noop 实现；部署时可注入真实的
// 指标后端（Prometheus / 内部 metrics）而无需改动业务代码。
type Metrics interface {
	// IncPublished 记录一条消息成功发布（created=true）。
	IncPublished(scope Scope)
	// IncSeqAllocRetry 记录一次 seq 分配后落库遇到主键冲突而重试（正常应始终为 0）。
	IncSeqAllocRetry()
	// IncCatchup 记录一次 catchup 拉取，n 为返回的消息条数。
	IncCatchup(n int)
	// IncAck 记录一次 ack。
	IncAck()
	// IncWSConnected 记录一次 WebSocket 成功建连。
	IncWSConnected()
	// IncWSKicked 记录一次因单连接策略而踢出旧连接。
	IncWSKicked()
	// IncPushDropped 记录一次实时推送被丢弃（连接不在线或发送缓冲满）。
	IncPushDropped()
}

// noopMetrics 是不做任何事的 Metrics 实现。
type noopMetrics struct{}

// NewNoopMetrics 返回一个空操作的 Metrics 实现。
func NewNoopMetrics() Metrics { return noopMetrics{} }

func (noopMetrics) IncPublished(Scope) {}
func (noopMetrics) IncSeqAllocRetry()  {}
func (noopMetrics) IncCatchup(int)     {}
func (noopMetrics) IncAck()            {}
func (noopMetrics) IncWSConnected()    {}
func (noopMetrics) IncWSKicked()       {}
func (noopMetrics) IncPushDropped()    {}

// MetricsSnapshot 是 CountingMetrics 的只读快照，便于管理端展示 / 结构化日志 / 未来
// 桥接到 Prometheus 导出器。
type MetricsSnapshot struct {
	PublishedDirect    int64 `json:"published_direct"`
	PublishedBroadcast int64 `json:"published_broadcast"`
	SeqAllocRetries    int64 `json:"seq_alloc_retries"`
	Catchups           int64 `json:"catchups"`
	CatchupMessages    int64 `json:"catchup_messages"`
	Acks               int64 `json:"acks"`
	WSConnected        int64 `json:"ws_connected"`
	WSKicked           int64 `json:"ws_kicked"`
	PushDropped        int64 `json:"push_dropped"`
}

// CountingMetrics 是基于原子计数器的进程内 Metrics 实现，线程安全、零依赖。它把
// 各类事件累加到计数器，可通过 Snapshot 读取。适合默认启用（开销可忽略），也可作为
// 后续 Prometheus 导出器的数据源（导出器读取 Snapshot 即可，无需改业务打点）。
type CountingMetrics struct {
	publishedDirect    atomic.Int64
	publishedBroadcast atomic.Int64
	seqAllocRetries    atomic.Int64
	catchups           atomic.Int64
	catchupMessages    atomic.Int64
	acks               atomic.Int64
	wsConnected        atomic.Int64
	wsKicked           atomic.Int64
	pushDropped        atomic.Int64
}

// NewCountingMetrics 构造一个进程内计数器 Metrics 实现。
func NewCountingMetrics() *CountingMetrics { return &CountingMetrics{} }

func (m *CountingMetrics) IncPublished(scope Scope) {
	if scope == ScopeBroadcast {
		m.publishedBroadcast.Add(1)
		return
	}
	m.publishedDirect.Add(1)
}
func (m *CountingMetrics) IncSeqAllocRetry() { m.seqAllocRetries.Add(1) }
func (m *CountingMetrics) IncCatchup(n int) {
	m.catchups.Add(1)
	m.catchupMessages.Add(int64(n))
}
func (m *CountingMetrics) IncAck()         { m.acks.Add(1) }
func (m *CountingMetrics) IncWSConnected() { m.wsConnected.Add(1) }
func (m *CountingMetrics) IncWSKicked()    { m.wsKicked.Add(1) }
func (m *CountingMetrics) IncPushDropped() { m.pushDropped.Add(1) }

// Snapshot 返回当前计数快照。
func (m *CountingMetrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		PublishedDirect:    m.publishedDirect.Load(),
		PublishedBroadcast: m.publishedBroadcast.Load(),
		SeqAllocRetries:    m.seqAllocRetries.Load(),
		Catchups:           m.catchups.Load(),
		CatchupMessages:    m.catchupMessages.Load(),
		Acks:               m.acks.Load(),
		WSConnected:        m.wsConnected.Load(),
		WSKicked:           m.wsKicked.Load(),
		PushDropped:        m.pushDropped.Load(),
	}
}

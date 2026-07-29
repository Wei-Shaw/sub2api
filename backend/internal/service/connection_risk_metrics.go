package service

import "sync/atomic"

// ConnectionRiskMetrics holds process-local counters for connection-risk
// observability (aligned with securityaudit AtomicMetrics — no fictional Prom API).
type ConnectionRiskMetrics struct {
	EmitOK        atomic.Int64
	EmitError     atomic.Int64
	EmitTimeout   atomic.Int64
	WorkerTicks   atomic.Int64
	EventsCreated atomic.Int64
	SubjectsScan  atomic.Int64
	Degraded      atomic.Bool
	LastTickUnix  atomic.Int64
}

// ConnectionRiskMetricsSnapshot is a stable export for /runtime.
type ConnectionRiskMetricsSnapshot struct {
	EmitOK        int64 `json:"emit_ok"`
	EmitError     int64 `json:"emit_error"`
	EmitTimeout   int64 `json:"emit_timeout"`
	WorkerTicks   int64 `json:"worker_ticks"`
	EventsCreated int64 `json:"events_created"`
	SubjectsScan  int64 `json:"subjects_scanned"`
	Degraded      bool  `json:"degraded"`
	LastTickUnix  int64 `json:"last_tick_unix"`
}

// Snapshot returns a point-in-time copy of counters.
func (m *ConnectionRiskMetrics) Snapshot() ConnectionRiskMetricsSnapshot {
	if m == nil {
		return ConnectionRiskMetricsSnapshot{}
	}
	return ConnectionRiskMetricsSnapshot{
		EmitOK:        m.EmitOK.Load(),
		EmitError:     m.EmitError.Load(),
		EmitTimeout:   m.EmitTimeout.Load(),
		WorkerTicks:   m.WorkerTicks.Load(),
		EventsCreated: m.EventsCreated.Load(),
		SubjectsScan:  m.SubjectsScan.Load(),
		Degraded:      m.Degraded.Load(),
		LastTickUnix:  m.LastTickUnix.Load(),
	}
}

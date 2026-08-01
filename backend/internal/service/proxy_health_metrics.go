package service

import "sync/atomic"

// ProxyHealthMetrics holds process-local counters for the proxy health poller.
type ProxyHealthMetrics struct {
	Ticks          atomic.Int64
	ProbedTotal    atomic.Int64
	IsolatedTotal  atomic.Int64
	RecoveredTotal atomic.Int64
	ErrorTotal     atomic.Int64
	SkippedTotal   atomic.Int64
	LastTickUnix   atomic.Int64
	LastScanUnix   atomic.Int64 // last manual or worker scan
}

// ProxyHealthMetricsSnapshot is a stable export for ops/runtime.
type ProxyHealthMetricsSnapshot struct {
	Ticks          int64 `json:"ticks"`
	ProbedTotal    int64 `json:"probed_total"`
	IsolatedTotal  int64 `json:"isolated_total"`
	RecoveredTotal int64 `json:"recovered_total"`
	ErrorTotal     int64 `json:"error_total"`
	SkippedTotal   int64 `json:"skipped_total"`
	LastTickUnix   int64 `json:"last_tick_unix"`
	LastScanUnix   int64 `json:"last_scan_unix"`
}

// Snapshot returns a point-in-time copy of counters.
func (m *ProxyHealthMetrics) Snapshot() ProxyHealthMetricsSnapshot {
	if m == nil {
		return ProxyHealthMetricsSnapshot{}
	}
	return ProxyHealthMetricsSnapshot{
		Ticks:          m.Ticks.Load(),
		ProbedTotal:    m.ProbedTotal.Load(),
		IsolatedTotal:  m.IsolatedTotal.Load(),
		RecoveredTotal: m.RecoveredTotal.Load(),
		ErrorTotal:     m.ErrorTotal.Load(),
		SkippedTotal:   m.SkippedTotal.Load(),
		LastTickUnix:   m.LastTickUnix.Load(),
		LastScanUnix:   m.LastScanUnix.Load(),
	}
}

// ProvideProxyHealthMetrics returns a process-wide metrics holder.
func ProvideProxyHealthMetrics() *ProxyHealthMetrics {
	return &ProxyHealthMetrics{}
}

func (m *ProxyHealthMetrics) recordRun(res *ProxyHealthRunResult, nowUnix int64) {
	if m == nil {
		return
	}
	m.Ticks.Add(1)
	m.LastTickUnix.Store(nowUnix)
	m.LastScanUnix.Store(nowUnix)
	if res == nil {
		return
	}
	m.ProbedTotal.Add(int64(res.Probed))
	m.IsolatedTotal.Add(int64(res.Isolated))
	m.RecoveredTotal.Add(int64(res.Recovered))
	m.ErrorTotal.Add(int64(res.Errors))
	m.SkippedTotal.Add(int64(res.Skipped))
}

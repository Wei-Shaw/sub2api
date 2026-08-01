package service

import "context"

// ProxyHealthIsolatedByHealth marks auto-isolation by the health poller.
const ProxyHealthIsolatedByHealth = "health"

// ProxyHealthMeta is Redis-backed probe state for a single proxy.
type ProxyHealthMeta struct {
	FailCount     int    `json:"fail_count"`
	SuccessCount  int    `json:"success_count"`
	LastCheckedAt int64  `json:"last_checked_at"`
	LastOKAt      int64  `json:"last_ok_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	LatencyMs     int64  `json:"latency_ms,omitempty"`
	ExitIP        string `json:"exit_ip,omitempty"`
	// IsolatedBy is "health" when auto-isolated; empty means manual/other (no auto-recover).
	IsolatedBy string `json:"isolated_by,omitempty"`
	IsolatedAt int64  `json:"isolated_at,omitempty"`
}

// ProxyHealthCache stores per-proxy health metadata in Redis.
type ProxyHealthCache interface {
	GetProxyHealth(ctx context.Context, proxyID int64) (*ProxyHealthMeta, error)
	SetProxyHealth(ctx context.Context, proxyID int64, meta *ProxyHealthMeta) error
}

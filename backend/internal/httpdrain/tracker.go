package httpdrain

import "sync/atomic"

type Snapshot struct {
	Supported           bool  `json:"supported"`
	ActiveRequests      int64 `json:"active_requests"`
	HijackedConnections int64 `json:"hijacked_connections"`
	Blockers            int64 `json:"blockers"`
}

var activeRequests atomic.Int64
var hijackedConnections atomic.Int64

func BeginRequest(path string) func() {
	if path == "/health" {
		return func() {}
	}
	activeRequests.Add(1)
	var finished atomic.Bool
	return func() {
		if finished.CompareAndSwap(false, true) {
			activeRequests.Add(-1)
		}
	}
}

func BeginHijackedConnection() {
	hijackedConnections.Add(1)
}

func EndHijackedConnection() {
	hijackedConnections.Add(-1)
}

func Status() Snapshot {
	requests := activeRequests.Load()
	hijacked := hijackedConnections.Load()
	return Snapshot{
		Supported:           true,
		ActiveRequests:      requests,
		HijackedConnections: hijacked,
		Blockers:            requests + hijacked,
	}
}

package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	portproxy "github.com/Wei-Shaw/sub2api/internal/port/proxy"
)

// Type aliases keep existing service call sites compiling while ports live in
// internal/port/proxy.
type ProxyLatencyInfo = domain.ProxyLatencyInfo
type ProxyLatencyCache = portproxy.LatencyCache

package service

import (
	"context"
	"time"
)

type ProxyLatencyInfo struct {
	Success   bool      `json:"success"`
	LatencyMs *int64    `json:"latency_ms,omitempty"`
	Message   string    `json:"message,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
REDACTED

type ProxyLatencyCache interface {
	GetProxyLatencies(ctx context.Context, proxyIDs []int64) (map[int64]*ProxyLatencyInfo, error)
	SetProxyLatency(ctx context.Context, proxyID int64, info *ProxyLatencyInfo) error
REDACTED

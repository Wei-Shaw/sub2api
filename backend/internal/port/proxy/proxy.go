// Package proxy contains the port interfaces (repository/cache/probe
// abstractions) for the proxy bounded context. DTO/value types live in
// internal/domain; this package only owns the infrastructure contracts.
package proxy

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// Repository persists proxies and related projections.
type Repository interface {
	Create(ctx context.Context, proxy *domain.Proxy) error
	GetByID(ctx context.Context, id int64) (*domain.Proxy, error)
	ListByIDs(ctx context.Context, ids []int64) ([]domain.Proxy, error)
	Update(ctx context.Context, proxy *domain.Proxy) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]domain.Proxy, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]domain.Proxy, *pagination.PaginationResult, error)
	ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]domain.ProxyWithAccountCount, *pagination.PaginationResult, error)
	ListActive(ctx context.Context) ([]domain.Proxy, error)
	ListActiveWithAccountCount(ctx context.Context) ([]domain.ProxyWithAccountCount, error)

	ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error)
	CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error)
	ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]domain.ProxyAccountSummary, error)

	SweepExpiredProxies(ctx context.Context, now time.Time) (changed int64, err error)
	ListAllForFallback(ctx context.Context) ([]domain.Proxy, error)
	CountExpired(ctx context.Context) (int64, error)
	CountExpiringSoon(ctx context.Context, now time.Time) (int64, error)
}

// LatencyCache stores per-proxy latency/quality snapshots.
type LatencyCache interface {
	GetProxyLatencies(ctx context.Context, proxyIDs []int64) (map[int64]*domain.ProxyLatencyInfo, error)
	SetProxyLatency(ctx context.Context, proxyID int64, info *domain.ProxyLatencyInfo) error
}

// ExitInfoProber tests proxy connectivity and retrieves exit information.
type ExitInfoProber interface {
	ProbeProxy(ctx context.Context, proxyURL string) (*domain.ProxyExitInfo, int64, error)
}

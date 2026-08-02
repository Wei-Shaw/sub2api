package service

import (
	"context"
	"fmt"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrProxyNotFound = infraerrors.NotFound("PROXY_NOT_FOUND", "proxy not found")
	ErrProxyInUse    = infraerrors.Conflict("PROXY_IN_USE", "proxy is in use by accounts")
	// ErrProxyHealthScanBusy is returned when a process-local or cluster-wide
	// proxy health scan is already in flight (singleflight / peer holds leader lock) — 409.
	ErrProxyHealthScanBusy = infraerrors.Conflict("PROXY_HEALTH_SCAN_BUSY", "proxy health scan already running")
	// ErrProxyHealthLockUnavailable is returned when the leader-lock backend
	// (Redis) errors on acquire — peer may still hold the lock; do not run — 503.
	ErrProxyHealthLockUnavailable = infraerrors.ServiceUnavailable(
		"PROXY_HEALTH_LOCK_UNAVAILABLE", "proxy health leader lock backend unavailable")
)

type ProxyRepository interface {
	Create(ctx context.Context, proxy *Proxy) error
	GetByID(ctx context.Context, id int64) (*Proxy, error)
	ListByIDs(ctx context.Context, ids []int64) ([]Proxy, error)
	Update(ctx context.Context, proxy *Proxy) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]Proxy, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]Proxy, *pagination.PaginationResult, error)
	ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error)
	ListActive(ctx context.Context) ([]Proxy, error)
	ListActiveWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error)

	ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error)
	CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error)
	ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]ProxyAccountSummary, error)

	SweepExpiredProxies(ctx context.Context, now time.Time) (changed int64, err error)
	ListAllForFallback(ctx context.Context) ([]Proxy, error)
	CountExpired(ctx context.Context) (int64, error)
	CountExpiringSoon(ctx context.Context, now time.Time) (int64, error)

	// 代理组成员查询（供 ProxyGroupResolver / 管理端使用）
	ListByGroupID(ctx context.Context, groupID int64) ([]Proxy, error)
	CountByGroupID(ctx context.Context, groupID int64) (int64, error)

	// Health audit snapshot (Phase 3 DB fields; independent of Redis meta).
	UpdateHealthAudit(ctx context.Context, proxyID int64, failCount int, lastHealthAt *time.Time, isolatedBy string) error
	GetHealthAudit(ctx context.Context, proxyID int64) (failCount int, lastHealthAt *time.Time, isolatedBy string, err error)
	// Health isolation monitoring (admin dashboard).
	CountHealthIsolated(ctx context.Context) (int64, error)
	ListHealthIsolated(ctx context.Context, limit int) ([]Proxy, error)
	// ListHealthIsolatedByID returns inactive health-isolated proxies with id > afterID
	// ordered by id ASC for rotating AutoRecover probes (avoids last_health_at starvation).
	ListHealthIsolatedByID(ctx context.Context, afterID int64, limit int) ([]Proxy, error)

	// ClearAccountProxyBindings nulls accounts.proxy_id (and backup_proxy_id
	// self-refs) so a proxy can be deleted/orphaned without dangling FKs.
	// Returns the number of accounts unbound.
	ClearAccountProxyBindings(ctx context.Context, proxyID int64) (int64, error)

	// UpdateStatusWithHealthIsolation updates status+audit in one SQL.
	// onlyIfStatus: if non-empty, require current status equals this.
	// onlyIfIsolatedBy: if non-nil, require COALESCE(health_isolated_by,'') equals *onlyIfIsolatedBy.
	// Returns updated=false when 0 rows (condition failed) — caller must NOT force Redis meta.
	UpdateStatusWithHealthIsolation(ctx context.Context, proxyID int64, status string, failCount int, lastHealthAt *time.Time, isolatedBy string, onlyIfStatus string, onlyIfIsolatedBy *string, updateHealthCounters bool) (updated bool, err error)
}

// CreateProxyRequest 创建代理请求
type CreateProxyRequest struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// UpdateProxyRequest 更新代理请求
type UpdateProxyRequest struct {
	Name     *string `json:"name"`
	Protocol *string `json:"protocol"`
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	Username *string `json:"username"`
	Password *string `json:"password"`
	Status   *string `json:"status"`
}

// ProxyService 代理管理服务
type ProxyService struct {
	proxyRepo ProxyRepository
	prober    ProxyExitInfoProber // optional; used by TestConnection
}

// NewProxyService 创建代理服务实例
func NewProxyService(proxyRepo ProxyRepository) *ProxyService {
	return &ProxyService{
		proxyRepo: proxyRepo,
	}
}

// NewProxyServiceWithProber wires an exit-info prober for TestConnection.
func NewProxyServiceWithProber(proxyRepo ProxyRepository, prober ProxyExitInfoProber) *ProxyService {
	return &ProxyService{
		proxyRepo: proxyRepo,
		prober:    prober,
	}
}

// SetProber attaches a probe implementation (for DI after construction).
func (s *ProxyService) SetProber(prober ProxyExitInfoProber) {
	if s == nil {
		return
	}
	s.prober = prober
}

// Create 创建代理
func (s *ProxyService) Create(ctx context.Context, req CreateProxyRequest) (*Proxy, error) {
	// 创建代理
	proxy := &Proxy{
		Name:     req.Name,
		Protocol: req.Protocol,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
		Status:   StatusActive,
	}

	if err := s.proxyRepo.Create(ctx, proxy); err != nil {
		return nil, fmt.Errorf("create proxy: %w", err)
	}

	return proxy, nil
}

// GetByID 根据ID获取代理
func (s *ProxyService) GetByID(ctx context.Context, id int64) (*Proxy, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get proxy: %w", err)
	}
	return proxy, nil
}

// List 获取代理列表
func (s *ProxyService) List(ctx context.Context, params pagination.PaginationParams) ([]Proxy, *pagination.PaginationResult, error) {
	proxies, pagination, err := s.proxyRepo.List(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list proxies: %w", err)
	}
	return proxies, pagination, nil
}

// ListActive 获取活跃代理列表
func (s *ProxyService) ListActive(ctx context.Context) ([]Proxy, error) {
	proxies, err := s.proxyRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active proxies: %w", err)
	}
	return proxies, nil
}

// Update 更新代理
func (s *ProxyService) Update(ctx context.Context, id int64, req UpdateProxyRequest) (*Proxy, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get proxy: %w", err)
	}

	// 更新字段
	if req.Name != nil {
		proxy.Name = *req.Name
	}

	if req.Protocol != nil {
		proxy.Protocol = *req.Protocol
	}

	if req.Host != nil {
		proxy.Host = *req.Host
	}

	if req.Port != nil {
		proxy.Port = *req.Port
	}

	if req.Username != nil {
		proxy.Username = *req.Username
	}

	if req.Password != nil {
		proxy.Password = *req.Password
	}

	if req.Status != nil {
		proxy.Status = *req.Status
	}

	if err := s.proxyRepo.Update(ctx, proxy); err != nil {
		return nil, fmt.Errorf("update proxy: %w", err)
	}

	return proxy, nil
}

// Delete 删除代理
func (s *ProxyService) Delete(ctx context.Context, id int64) error {
	// 检查代理是否存在
	_, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get proxy: %w", err)
	}

	if err := s.proxyRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete proxy: %w", err)
	}

	return nil
}

// TestConnection probes the proxy exit via the shared exit-info prober when configured.
// Without a prober it still validates the proxy row exists and builds a URL.
func (s *ProxyService) TestConnection(ctx context.Context, id int64) error {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get proxy: %w", err)
	}
	if s.prober == nil {
		if proxy.URL() == "" {
			return fmt.Errorf("proxy url is empty")
		}
		return fmt.Errorf("proxy prober not configured")
	}
	_, _, err = s.prober.ProbeProxy(ctx, proxy.URL())
	if err != nil {
		return fmt.Errorf("probe proxy: %w", err)
	}
	return nil
}

// GetURL 获取代理URL
func (s *ProxyService) GetURL(ctx context.Context, id int64) (string, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get proxy: %w", err)
	}

	return proxy.URL(), nil
}

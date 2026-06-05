package service

import (
	"context"
	"fmt"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrProxyNotFound = infraerrors.NotFound("PROXY_NOT_FOUND", "proxy not found")
	ErrProxyInUse    = infraerrors.Conflict("PROXY_IN_USE", "proxy is in use by accounts")
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
}

// NewProxyService 创建代理服务实例
func NewProxyService(proxyRepo ProxyRepository) *ProxyService {
	return &ProxyService{
		proxyRepo: proxyRepo,
	}
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

// TestConnection 测试代理连接（需要实现具体测试逻辑）
func (s *ProxyService) TestConnection(ctx context.Context, id int64) error {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get proxy: %w", err)
	}

	// TODO: 实现代理连接测试逻辑
	// 可以尝试通过代理发送测试请求
	_ = proxy

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

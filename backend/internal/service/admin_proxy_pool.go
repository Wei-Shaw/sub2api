package service

import (
	"context"
	"errors"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// CreateProxyPoolInput 创建代理池入参。
type CreateProxyPoolInput struct {
	Name                  string
	Description           *string
	Status                string
	HealthIntervalSeconds int
	FailureThreshold      int
	AutoRebind            *bool
}

// UpdateProxyPoolInput 更新代理池入参（零值字段不修改）。
type UpdateProxyPoolInput struct {
	Name                  *string
	Description           *string
	Status                *string
	HealthIntervalSeconds *int
	FailureThreshold      *int
	AutoRebind            *bool
}

var (
	ErrProxyPoolNotFound      = infraerrors.NotFound("PROXY_POOL_NOT_FOUND", "proxy pool not found")
	ErrProxyPoolRunInProgress = infraerrors.Conflict("PROXY_POOL_RUN_IN_PROGRESS", "proxy pool sweep is already in progress")
)

// ListProxyPools 返回全部池（含代理统计）。
func (s *adminServiceImpl) ListProxyPools(ctx context.Context) ([]ProxyPoolWithStats, error) {
	if s.poolRepo == nil {
		return nil, nil
	}
	return s.poolRepo.ListPoolsWithStats(ctx)
}

// GetProxyPool 获取单个池。
func (s *adminServiceImpl) GetProxyPool(ctx context.Context, id int64) (*ProxyPool, error) {
	if s.poolRepo == nil {
		return nil, ErrProxyPoolNotFound
	}
	pool, err := s.poolRepo.GetPoolByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return pool, nil
}

// GetProxyPoolProxies 返回池内代理（含账号数与延迟/质量展示字段）。
func (s *adminServiceImpl) GetProxyPoolProxies(ctx context.Context, poolID int64) ([]ProxyWithAccountCount, error) {
	if s.poolRepo == nil {
		return nil, nil
	}
	proxies, err := s.poolRepo.ListPoolProxies(ctx, poolID)
	if err != nil {
		return nil, err
	}
	out := make([]ProxyWithAccountCount, 0, len(proxies))
	ids := make([]int64, 0, len(proxies))
	for i := range proxies {
		ids = append(ids, proxies[i].ID)
	}
	counts, err := s.poolRepo.CountAccountsByProxyIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range proxies {
		out = append(out, ProxyWithAccountCount{
			Proxy:        proxies[i],
			AccountCount: counts[proxies[i].ID],
		})
	}
	s.attachProxyLatency(ctx, out)
	return out, nil
}

// GetProxyPoolAccounts 分页返回池绑定账号及其当前实际代理。
func (s *adminServiceImpl) GetProxyPoolAccounts(ctx context.Context, poolID int64, page, pageSize int) ([]ProxyPoolAccountSummary, int64, error) {
	if s.poolRepo == nil {
		return nil, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	} else if pageSize > 100 {
		pageSize = 100
	}
	return s.poolRepo.ListPoolAccounts(ctx, poolID, (page-1)*pageSize, pageSize)
}

// CreateProxyPool 创建代理池。
func (s *adminServiceImpl) CreateProxyPool(ctx context.Context, input *CreateProxyPoolInput) (*ProxyPool, error) {
	if s.poolRepo == nil {
		return nil, errors.New("proxy pool repository unavailable")
	}
	if input == nil {
		return nil, errors.New("invalid proxy pool input")
	}
	if input.Name == "" {
		return nil, errors.New("pool name is required")
	}
	existing, err := s.poolRepo.ListPools(ctx)
	if err != nil {
		return nil, err
	}
	for i := range existing {
		if strings.EqualFold(existing[i].Name, input.Name) {
			return nil, errors.New("proxy pool name already exists")
		}
	}
	pool := &ProxyPool{
		Name:                  input.Name,
		Description:           input.Description,
		Status:                StatusActive,
		HealthIntervalSeconds: input.HealthIntervalSeconds,
		FailureThreshold:      input.FailureThreshold,
		AutoRebind:            true,
	}
	if input.Status != "" {
		pool.Status = input.Status
	}
	if pool.HealthIntervalSeconds <= 0 {
		pool.HealthIntervalSeconds = 300
	}
	if pool.FailureThreshold <= 0 {
		pool.FailureThreshold = 2
	}
	if input.AutoRebind != nil {
		pool.AutoRebind = *input.AutoRebind
	}
	created, err := s.poolRepo.CreatePool(ctx, pool)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// UpdateProxyPool 更新代理池（指针字段为 nil 时不修改）。
func (s *adminServiceImpl) UpdateProxyPool(ctx context.Context, id int64, input *UpdateProxyPoolInput) (*ProxyPool, error) {
	if s.poolRepo == nil {
		return nil, ErrProxyPoolNotFound
	}
	existing, err := s.poolRepo.GetPoolByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if input == nil {
		return existing, nil
	}
	if input.Name != nil {
		if !strings.EqualFold(existing.Name, *input.Name) {
			all, listErr := s.poolRepo.ListPools(ctx)
			if listErr != nil {
				return nil, listErr
			}
			for i := range all {
				if all[i].ID != existing.ID && strings.EqualFold(all[i].Name, *input.Name) {
					return nil, errors.New("proxy pool name already exists")
				}
			}
		}
		existing.Name = *input.Name
	}
	if input.Description != nil {
		existing.Description = input.Description
	}
	if input.Status != nil {
		existing.Status = *input.Status
	}
	if input.HealthIntervalSeconds != nil {
		existing.HealthIntervalSeconds = *input.HealthIntervalSeconds
	}
	if input.FailureThreshold != nil {
		existing.FailureThreshold = *input.FailureThreshold
	}
	if input.AutoRebind != nil {
		existing.AutoRebind = *input.AutoRebind
	}
	if existing.HealthIntervalSeconds <= 0 {
		existing.HealthIntervalSeconds = 300
	}
	if existing.FailureThreshold <= 0 {
		existing.FailureThreshold = 2
	}
	if err := s.poolRepo.UpdatePool(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteProxyPool 硬删除池；外键会把代理和账号的 pool_id 置空，并级联清理重绑日志。
func (s *adminServiceImpl) DeleteProxyPool(ctx context.Context, id int64) error {
	if s.poolRepo == nil {
		return ErrProxyPoolNotFound
	}
	return s.poolRepo.DeletePool(ctx, id)
}

// AssignProxiesToPool 把代理加入池。
func (s *adminServiceImpl) AssignProxiesToPool(ctx context.Context, poolID int64, proxyIDs []int64) (int64, error) {
	if s.poolRepo == nil {
		return 0, errors.New("proxy pool repository unavailable")
	}
	if _, err := s.poolRepo.GetPoolByID(ctx, poolID); err != nil {
		return 0, err
	}
	if len(proxyIDs) == 0 {
		return 0, nil
	}
	return s.poolRepo.AssignProxiesToPool(ctx, poolID, proxyIDs)
}

// RemoveProxiesFromPool 把代理移出池。
func (s *adminServiceImpl) RemoveProxiesFromPool(ctx context.Context, poolID int64, proxyIDs []int64) (int64, error) {
	if s.poolRepo == nil {
		return 0, errors.New("proxy pool repository unavailable")
	}
	// 只移出属于该池的代理，避免误动其它池成员
	proxies, err := s.poolRepo.ListPoolProxies(ctx, poolID)
	if err != nil {
		return 0, err
	}
	inPool := make(map[int64]struct{}, len(proxies))
	for i := range proxies {
		inPool[proxies[i].ID] = struct{}{}
	}
	filtered := make([]int64, 0, len(proxyIDs))
	for _, pid := range proxyIDs {
		if _, ok := inPool[pid]; ok {
			filtered = append(filtered, pid)
		}
	}
	if len(filtered) == 0 {
		return 0, nil
	}
	return s.poolRepo.RemoveProxiesFromPool(ctx, filtered)
}

// RunProxyPoolRebind 手动触发一轮池健康探测 + 自动重绑。
// 返回被改投的账号数。
func (s *adminServiceImpl) RunProxyPoolRebind(ctx context.Context, poolID int64) (int64, error) {
	if s.poolRepo == nil {
		return 0, ErrProxyPoolNotFound
	}
	pool, err := s.poolRepo.GetPoolByID(ctx, poolID)
	if err != nil {
		return 0, err
	}
	if pool == nil {
		return 0, ErrProxyPoolNotFound
	}
	if !pool.IsActive() {
		return 0, errors.New("proxy pool is disabled; enable it first")
	}
	if s.poolService == nil {
		return 0, errors.New("proxy pool service unavailable")
	}
	rebound, err := s.poolService.runPoolManually(ctx, pool)
	return int64(rebound), err
}

// ListProxyPoolRebindLogs 返回池内最近的重绑日志。
func (s *adminServiceImpl) ListProxyPoolRebindLogs(ctx context.Context, poolID int64, limit int) ([]ProxyPoolRebindLog, error) {
	if s.poolRepo == nil {
		return nil, nil
	}
	return s.poolRepo.ListRebindLogs(ctx, poolID, limit)
}

// assignPoolHealthyProxy 为账号分配池内账号数最少的健康代理（负载均衡）。
// 返回 nil 表示池内暂无健康代理（由池服务周期探测后补齐）。
func (s *adminServiceImpl) assignPoolHealthyProxy(ctx context.Context, poolID int64) (*int64, error) {
	if s.poolRepo == nil {
		return nil, errors.New("proxy pool repository unavailable")
	}
	proxies, err := s.poolRepo.ListPoolProxies(ctx, poolID)
	if err != nil {
		return nil, err
	}
	healthy := make([]*Proxy, 0, len(proxies))
	for i := range proxies {
		if proxies[i].PoolHealth == PoolHealthHealthy {
			healthy = append(healthy, &proxies[i])
		}
	}
	if len(healthy) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(healthy))
	for _, p := range healthy {
		ids = append(ids, p.ID)
	}
	counts, err := s.poolRepo.CountAccountsByProxyIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	best := healthy[0]
	bestCount := counts[best.ID]
	for _, p := range healthy[1:] {
		if counts[p.ID] < bestCount {
			best = p
			bestCount = counts[p.ID]
		}
	}
	id := best.ID
	return &id, nil
}

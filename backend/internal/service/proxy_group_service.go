package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrProxyGroupNotFound = infraerrors.NotFound("PROXY_GROUP_NOT_FOUND", "proxy group not found")
	ErrProxyGroupInUse    = infraerrors.Conflict("PROXY_GROUP_IN_USE", "proxy group is in use by accounts")
	ErrProxyGroupEmpty    = infraerrors.BadRequest("PROXY_GROUP_EMPTY", "proxy group must contain at least one proxy")
	ErrProxyGroupNoProxy  = infraerrors.ServiceUnavailable("PROXY_GROUP_NO_AVAILABLE_PROXY", "proxy group has no available proxy")
)

type ProxyGroupRepository interface {
	List(ctx context.Context, page, pageSize int, status, search string) ([]ProxyGroup, int64, error)
	ListAll(ctx context.Context) ([]ProxyGroup, error)
	GetByID(ctx context.Context, id int64) (*ProxyGroup, error)
	Create(ctx context.Context, group *ProxyGroup, proxyIDs []int64) error
	Update(ctx context.Context, group *ProxyGroup, proxyIDs []int64) error
	Delete(ctx context.Context, id int64) error
	CountAccountsByGroupID(ctx context.Context, id int64) (int64, error)
	SelectRandomAvailableProxy(ctx context.Context, groupID int64) (*Proxy, error)
}

type ProxyGroupService struct {
	repo ProxyGroupRepository
}

var defaultProxyGroupResolver struct {
	sync.RWMutex
	resolver ProxyGroupResolver
}

func NewProxyGroupService(repo ProxyGroupRepository) *ProxyGroupService {
	svc := &ProxyGroupService{repo: repo}
	SetDefaultProxyGroupResolver(svc)
	return svc
}

func SetDefaultProxyGroupResolver(resolver ProxyGroupResolver) {
	defaultProxyGroupResolver.Lock()
	defaultProxyGroupResolver.resolver = resolver
	defaultProxyGroupResolver.Unlock()
}

func resolveDefaultProxyGroupAccount(ctx context.Context, account *Account) error {
	if account == nil || account.ProxyGroupID == nil || account.proxyGroupResolved {
		return nil
	}
	defaultProxyGroupResolver.RLock()
	resolver := defaultProxyGroupResolver.resolver
	defaultProxyGroupResolver.RUnlock()
	if resolver == nil {
		return infraerrors.ServiceUnavailable("PROXY_GROUP_UNAVAILABLE", "proxy group resolver is not available")
	}
	return resolver.ResolveAccountProxy(ctx, account)
}

func (s *ProxyGroupService) List(ctx context.Context, page, pageSize int, status, search string) ([]ProxyGroup, int64, error) {
	return s.repo.List(ctx, page, pageSize, status, strings.TrimSpace(search))
}

func (s *ProxyGroupService) ListAll(ctx context.Context) ([]ProxyGroup, error) {
	return s.repo.ListAll(ctx)
}

func (s *ProxyGroupService) Get(ctx context.Context, id int64) (*ProxyGroup, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ProxyGroupService) Create(ctx context.Context, input ProxyGroupInput) (*ProxyGroup, error) {
	group, err := normalizeProxyGroupInput(input)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, group, group.ProxyIDs); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, group.ID)
}

func (s *ProxyGroupService) Update(ctx context.Context, id int64, input ProxyGroupInput) (*ProxyGroup, error) {
	group, err := normalizeProxyGroupInput(input)
	if err != nil {
		return nil, err
	}
	group.ID = id
	if err := s.repo.Update(ctx, group, group.ProxyIDs); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, id)
}

func (s *ProxyGroupService) Delete(ctx context.Context, id int64) error {
	count, err := s.repo.CountAccountsByGroupID(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrProxyGroupInUse.WithMetadata(map[string]string{"account_count": fmt.Sprintf("%d", count)})
	}
	return s.repo.Delete(ctx, id)
}

func (s *ProxyGroupService) SelectRandomAvailableProxy(ctx context.Context, groupID int64) (*Proxy, error) {
	proxy, err := s.repo.SelectRandomAvailableProxy(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if proxy == nil {
		return nil, ErrProxyGroupNoProxy
	}
	return proxy, nil
}

func normalizeProxyGroupInput(input ProxyGroupInput) (*ProxyGroup, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, infraerrors.BadRequest("PROXY_GROUP_NAME_REQUIRED", "proxy group name is required")
	}
	status := input.Status
	if status == "" {
		status = ProxyGroupStatusActive
	}
	if status != ProxyGroupStatusActive && status != ProxyGroupStatusInactive {
		return nil, infraerrors.BadRequest("PROXY_GROUP_STATUS_INVALID", "proxy group status must be active or inactive")
	}
	proxyIDs := uniquePositiveIDs(input.ProxyIDs)
	if len(proxyIDs) == 0 {
		return nil, ErrProxyGroupEmpty
	}
	return &ProxyGroup{Name: name, Description: input.Description, Status: status, ProxyIDs: proxyIDs}, nil
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// ProxyGroupResolver is intentionally narrow so gateway services can resolve
// a request-scoped proxy without depending on admin CRUD operations.
type ProxyGroupResolver interface {
	ResolveAccountProxy(ctx context.Context, account *Account) error
}

func (s *ProxyGroupService) ResolveAccountProxy(ctx context.Context, account *Account) error {
	if account == nil || account.ProxyGroupID == nil {
		return nil
	}
	proxy, err := s.SelectRandomAvailableProxy(ctx, *account.ProxyGroupID)
	if err != nil {
		return err
	}
	proxyID := proxy.ID
	account.ProxyID = &proxyID
	account.Proxy = proxy
	account.proxyGroupResolved = true
	return nil
}

// SelectNextProxy selects a different active, unexpired member for a first-serve rotation.
func (s *ProxyGroupService) SelectNextProxy(ctx context.Context, groupID, previousID int64, allowedIDs []int64) (*Proxy, error) {
	selector, ok := s.repo.(interface {
		SelectAvailableProxyExcluding(context.Context, int64, int64, []int64) (*Proxy, error)
	})
	if !ok {
		return nil, ErrProxyGroupNoProxy
	}
	proxy, err := selector.SelectAvailableProxyExcluding(ctx, groupID, previousID, allowedIDs)
	if err != nil {
		return nil, err
	}
	if proxy == nil {
		return nil, ErrProxyGroupNoProxy
	}
	return proxy, nil
}

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ProxyGroupService 代理组管理服务（独立于 AdminService）。
type ProxyGroupService struct {
	groupRepo ProxyGroupRepository
	proxyRepo ProxyRepository
	resolver  ProxyGroupResolver
}

type atomicProxyGroupRepository interface {
	CreateWithMembers(ctx context.Context, group *ProxyGroup, proxyIDs []int64) error
	UpdateWithMembers(ctx context.Context, group *ProxyGroup, proxyIDs *[]int64) error
	DeleteIfUnused(ctx context.Context, groupID int64) error
}

// NewProxyGroupService 构造代理组服务。
func NewProxyGroupService(
	groupRepo ProxyGroupRepository,
	proxyRepo ProxyRepository,
	resolver ProxyGroupResolver,
) *ProxyGroupService {
	return &ProxyGroupService{
		groupRepo: groupRepo,
		proxyRepo: proxyRepo,
		resolver:  resolver,
	}
}

type CreateProxyGroupInput struct {
	Name                   string
	Description            string
	Strategy               string
	StickyByAccount        bool
	Status                 string
	ProxyIDs               []int64
	HealthFailThreshold    *int
	HealthSuccessThreshold *int
}

type UpdateProxyGroupInput struct {
	Name                   *string
	Description            *string
	Strategy               *string
	StickyByAccount        *bool
	Status                 *string
	ProxyIDs               *[]int64 // nil = 不改成员；非 nil（含空切片）= 替换成员集
	HealthFailThreshold    *int     // nil=不改；0=清回全局默认
	HealthSuccessThreshold *int
}

func (s *ProxyGroupService) Create(ctx context.Context, input CreateProxyGroupInput) (*ProxyGroupWithProxies, error) {
	if s == nil || s.groupRepo == nil {
		return nil, fmt.Errorf("proxy group service not configured")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	strategy := normalizeProxyGroupStrategy(input.Strategy)
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = StatusActive
	}
	group := &ProxyGroup{
		Name:                   name,
		Description:            strings.TrimSpace(input.Description),
		Strategy:               strategy,
		StickyByAccount:        input.StickyByAccount,
		Status:                 status,
		HealthFailThreshold:    normalizeHealthThresholdPtr(input.HealthFailThreshold),
		HealthSuccessThreshold: normalizeHealthThresholdPtr(input.HealthSuccessThreshold),
	}
	oldGroupIDs := s.collectSourceGroupIDs(ctx, 0, input.ProxyIDs)
	if atomicRepo, ok := s.groupRepo.(atomicProxyGroupRepository); ok {
		if err := atomicRepo.CreateWithMembers(ctx, group, input.ProxyIDs); err != nil {
			return nil, err
		}
		s.invalidate(group.ID)
		for oldID := range oldGroupIDs {
			s.invalidate(oldID)
		}
	} else {
		if err := s.groupRepo.Create(ctx, group); err != nil {
			return nil, err
		}
		if len(input.ProxyIDs) > 0 {
			if err := s.replaceMembers(ctx, group.ID, input.ProxyIDs); err != nil {
				return nil, err
			}
		}
	}
	return s.GetByID(ctx, group.ID)
}

func (s *ProxyGroupService) GetByID(ctx context.Context, id int64) (*ProxyGroupWithProxies, error) {
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	members, err := s.proxyRepo.ListByGroupID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &ProxyGroupWithProxies{
		ProxyGroup: *group,
		Proxies:    members,
		ProxyCount: int64(len(members)),
	}, nil
}

func (s *ProxyGroupService) List(ctx context.Context, params pagination.PaginationParams) ([]ProxyGroupWithProxies, *pagination.PaginationResult, error) {
	groups, page, err := s.groupRepo.List(ctx, params)
	if err != nil {
		return nil, nil, err
	}
	out := make([]ProxyGroupWithProxies, 0, len(groups))
	for i := range groups {
		count, err := s.groupRepo.CountProxiesByGroupID(ctx, groups[i].ID)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, ProxyGroupWithProxies{
			ProxyGroup: groups[i],
			ProxyCount: count,
		})
	}
	return out, page, nil
}

func (s *ProxyGroupService) ListActive(ctx context.Context) ([]ProxyGroup, error) {
	return s.groupRepo.ListActive(ctx)
}

func (s *ProxyGroupService) Update(ctx context.Context, id int64, input UpdateProxyGroupInput) (*ProxyGroupWithProxies, error) {
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("name is required")
		}
		group.Name = name
	}
	if input.Description != nil {
		group.Description = strings.TrimSpace(*input.Description)
	}
	if input.Strategy != nil {
		group.Strategy = normalizeProxyGroupStrategy(*input.Strategy)
	}
	if input.StickyByAccount != nil {
		group.StickyByAccount = *input.StickyByAccount
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if status != "" {
			group.Status = status
		}
	}
	if input.HealthFailThreshold != nil {
		group.HealthFailThreshold = normalizeHealthThresholdPtr(input.HealthFailThreshold)
	}
	if input.HealthSuccessThreshold != nil {
		group.HealthSuccessThreshold = normalizeHealthThresholdPtr(input.HealthSuccessThreshold)
	}
	if atomicRepo, ok := s.groupRepo.(atomicProxyGroupRepository); ok {
		oldGroupIDs := map[int64]struct{}{}
		if input.ProxyIDs != nil {
			oldGroupIDs = s.collectSourceGroupIDs(ctx, id, *input.ProxyIDs)
		}
		if err := atomicRepo.UpdateWithMembers(ctx, group, input.ProxyIDs); err != nil {
			return nil, err
		}
		s.invalidate(id)
		for oldID := range oldGroupIDs {
			s.invalidate(oldID)
		}
	} else {
		if err := s.groupRepo.Update(ctx, group); err != nil {
			return nil, err
		}
		if input.ProxyIDs != nil {
			if err := s.replaceMembers(ctx, id, *input.ProxyIDs); err != nil {
				return nil, err
			}
		} else {
			s.invalidate(id)
		}
	}
	return s.GetByID(ctx, id)
}

func (s *ProxyGroupService) Delete(ctx context.Context, id int64) error {
	if atomicRepo, ok := s.groupRepo.(atomicProxyGroupRepository); ok {
		if err := atomicRepo.DeleteIfUnused(ctx, id); err != nil {
			return err
		}
		s.invalidate(id)
		return nil
	}
	proxyCount, err := s.groupRepo.CountProxiesByGroupID(ctx, id)
	if err != nil {
		return err
	}
	accountCount, err := s.groupRepo.CountAccountsByGroupID(ctx, id)
	if err != nil {
		return err
	}
	if proxyCount > 0 || accountCount > 0 {
		return ErrProxyGroupInUse
	}
	if err := s.groupRepo.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidate(id)
	return nil
}

func (s *ProxyGroupService) SetMembers(ctx context.Context, id int64, proxyIDs []int64) (*ProxyGroupWithProxies, error) {
	if _, err := s.groupRepo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	if err := s.replaceMembers(ctx, id, proxyIDs); err != nil {
		return nil, err
	}
	return s.GetByID(ctx, id)
}

func (s *ProxyGroupService) replaceMembers(ctx context.Context, groupID int64, proxyIDs []int64) error {
	if s.groupRepo == nil {
		return fmt.Errorf("proxy group repository not configured")
	}

	// Proxies being assigned may currently belong to other groups; those source
	// groups must be invalidated after membership moves (not only the target).
	oldGroupIDs := s.collectSourceGroupIDs(ctx, groupID, proxyIDs)

	if err := s.groupRepo.SetGroupMembers(ctx, groupID, proxyIDs); err != nil {
		return err
	}
	s.invalidate(groupID)
	for oldID := range oldGroupIDs {
		s.invalidate(oldID)
	}
	return nil
}

// collectSourceGroupIDs returns unique group IDs that currently own any of
// proxyIDs and differ from target groupID. Best-effort: if proxyRepo is nil
// or ListByIDs fails, returns an empty set (target still invalidated by caller).
func (s *ProxyGroupService) collectSourceGroupIDs(ctx context.Context, groupID int64, proxyIDs []int64) map[int64]struct{} {
	out := make(map[int64]struct{})
	if s == nil || s.proxyRepo == nil || len(proxyIDs) == 0 {
		return out
	}
	proxies, err := s.proxyRepo.ListByIDs(ctx, proxyIDs)
	if err != nil {
		// Do not block membership update on a pre-query failure; target
		// invalidation still runs. Source-group staleness is bounded by TTL.
		return out
	}
	for i := range proxies {
		gid := proxies[i].GroupID
		if gid == nil || *gid <= 0 || *gid == groupID {
			continue
		}
		out[*gid] = struct{}{}
	}
	return out
}

func (s *ProxyGroupService) invalidate(groupID int64) {
	if s != nil && s.resolver != nil {
		s.resolver.InvalidateGroup(groupID)
	}
}

func normalizeHealthThresholdPtr(v *int) *int {
	if v == nil {
		return nil
	}
	if *v <= 0 {
		return nil
	}
	n := *v
	return &n
}

func normalizeProxyGroupStrategy(strategy string) string {
	switch strings.TrimSpace(strings.ToLower(strategy)) {
	case ProxyGroupStrategyRandom:
		return ProxyGroupStrategyRandom
	case ProxyGroupStrategySticky:
		return ProxyGroupStrategySticky
	case ProxyGroupStrategyRoundRobin, "":
		return ProxyGroupStrategyRoundRobin
	default:
		return ProxyGroupStrategyRoundRobin
	}
}

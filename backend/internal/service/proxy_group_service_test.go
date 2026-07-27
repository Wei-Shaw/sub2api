//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type memProxyGroupRepo struct {
	nextID  int64
	groups  map[int64]*ProxyGroup
	members map[int64][]int64
}

func newMemProxyGroupRepo() *memProxyGroupRepo {
	return &memProxyGroupRepo{
		nextID:  1,
		groups:  map[int64]*ProxyGroup{},
		members: map[int64][]int64{},
	}
}

func (r *memProxyGroupRepo) Create(_ context.Context, group *ProxyGroup) error {
	id := r.nextID
	r.nextID++
	copy := *group
	copy.ID = id
	r.groups[id] = &copy
	group.ID = id
	return nil
}
func (r *memProxyGroupRepo) GetByID(_ context.Context, id int64) (*ProxyGroup, error) {
	g, ok := r.groups[id]
	if !ok {
		return nil, ErrProxyGroupNotFound
	}
	copy := *g
	return &copy, nil
}
func (r *memProxyGroupRepo) Update(_ context.Context, group *ProxyGroup) error {
	if _, ok := r.groups[group.ID]; !ok {
		return ErrProxyGroupNotFound
	}
	copy := *group
	r.groups[group.ID] = &copy
	return nil
}
func (r *memProxyGroupRepo) Delete(_ context.Context, id int64) error {
	if len(r.members[id]) > 0 {
		return ErrProxyGroupInUse
	}
	if _, ok := r.groups[id]; !ok {
		return ErrProxyGroupNotFound
	}
	delete(r.groups, id)
	return nil
}
func (r *memProxyGroupRepo) List(context.Context, pagination.PaginationParams) ([]ProxyGroup, *pagination.PaginationResult, error) {
	out := make([]ProxyGroup, 0, len(r.groups))
	for _, g := range r.groups {
		out = append(out, *g)
	}
	return out, &pagination.PaginationResult{Total: int64(len(out)), Page: 1, PageSize: 20}, nil
}
func (r *memProxyGroupRepo) ListActive(context.Context) ([]ProxyGroup, error) {
	out := make([]ProxyGroup, 0)
	for _, g := range r.groups {
		if g.Status == StatusActive {
			out = append(out, *g)
		}
	}
	return out, nil
}
func (r *memProxyGroupRepo) CountProxiesByGroupID(_ context.Context, groupID int64) (int64, error) {
	return int64(len(r.members[groupID])), nil
}
func (r *memProxyGroupRepo) CountAccountsByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (r *memProxyGroupRepo) SetGroupMembers(_ context.Context, groupID int64, proxyIDs []int64) error {
	cp := append([]int64(nil), proxyIDs...)
	r.members[groupID] = cp
	return nil
}

type memProxyRepoForGroupSvc struct {
	ProxyRepository
	byID map[int64]Proxy
}

func (r *memProxyRepoForGroupSvc) ListByGroupID(_ context.Context, groupID int64) ([]Proxy, error) {
	// not used by Create path until GetByID; implement via members lookup outside
	out := make([]Proxy, 0)
	for _, p := range r.byID {
		if p.GroupID != nil && *p.GroupID == groupID {
			out = append(out, p)
		}
	}
	return out, nil
}

type memResolver struct {
	invalidated []int64
}

func (r *memResolver) ResolveProxy(context.Context, int64, int64) (*Proxy, error) {
	return nil, nil
}
func (r *memResolver) InvalidateGroup(groupID int64) {
	r.invalidated = append(r.invalidated, groupID)
}

func TestProxyGroupService_CreateAndDeleteProtection(t *testing.T) {
	t.Parallel()
	groupRepo := newMemProxyGroupRepo()
	proxyRepo := &memProxyRepoForGroupSvc{byID: map[int64]Proxy{}}
	resolver := &memResolver{}
	svc := NewProxyGroupService(groupRepo, proxyRepo, resolver)

	created, err := svc.Create(context.Background(), CreateProxyGroupInput{
		Name:     "pool-a",
		Strategy: "random",
		ProxyIDs: []int64{1, 2},
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, "pool-a", created.Name)
	require.Equal(t, ProxyGroupStrategyRandom, created.Strategy)
	require.Equal(t, []int64{1, 2}, groupRepo.members[created.ID])
	require.Contains(t, resolver.invalidated, created.ID)

	// 有成员时禁止删除
	err = svc.Delete(context.Background(), created.ID)
	require.ErrorIs(t, err, ErrProxyGroupInUse)

	// 清空成员后可删
	_, err = svc.SetMembers(context.Background(), created.ID, nil)
	require.NoError(t, err)
	err = svc.Delete(context.Background(), created.ID)
	require.NoError(t, err)
}

func TestProxyGroupService_NormalizeStrategy(t *testing.T) {
	t.Parallel()
	require.Equal(t, ProxyGroupStrategyRoundRobin, normalizeProxyGroupStrategy(""))
	require.Equal(t, ProxyGroupStrategySticky, normalizeProxyGroupStrategy("STICKY"))
	require.Equal(t, ProxyGroupStrategyRoundRobin, normalizeProxyGroupStrategy("weird"))
}

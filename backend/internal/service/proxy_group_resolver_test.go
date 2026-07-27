//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type stubProxyGroupRepo struct {
	group *ProxyGroup
	err   error
}

func (s *stubProxyGroupRepo) Create(context.Context, *ProxyGroup) error { panic("unused") }
func (s *stubProxyGroupRepo) GetByID(context.Context, int64) (*ProxyGroup, error) {
	return s.group, s.err
}
func (s *stubProxyGroupRepo) Update(context.Context, *ProxyGroup) error { panic("unused") }
func (s *stubProxyGroupRepo) Delete(context.Context, int64) error       { panic("unused") }
func (s *stubProxyGroupRepo) List(context.Context, pagination.PaginationParams) ([]ProxyGroup, *pagination.PaginationResult, error) {
	panic("unused")
}
func (s *stubProxyGroupRepo) ListActive(context.Context) ([]ProxyGroup, error) { panic("unused") }
func (s *stubProxyGroupRepo) CountProxiesByGroupID(context.Context, int64) (int64, error) {
	panic("unused")
}
func (s *stubProxyGroupRepo) CountAccountsByGroupID(context.Context, int64) (int64, error) {
	panic("unused")
}
func (s *stubProxyGroupRepo) SetGroupMembers(context.Context, int64, []int64) error {
	panic("unused")
}

type stubProxyRepoForGroup struct {
	ProxyRepository
	members []Proxy
}

func (s *stubProxyRepoForGroup) ListByGroupID(context.Context, int64) ([]Proxy, error) {
	return s.members, nil
}

func TestDefaultProxyGroupResolver_RoundRobinAndCache(t *testing.T) {
	t.Parallel()
	now := time.Now()
	group := &ProxyGroup{ID: 9, Name: "pool", Strategy: ProxyGroupStrategyRoundRobin, Status: StatusActive}
	members := []Proxy{
		{ID: 1, Status: StatusActive},
		{ID: 2, Status: StatusActive},
		{ID: 3, Status: StatusActive},
	}
	r := NewDefaultProxyGroupResolver(
		&stubProxyGroupRepo{group: group},
		&stubProxyRepoForGroup{members: members},
	)
	r.now = func() time.Time { return now }

	seen := map[int64]int{}
	for i := 0; i < 6; i++ {
		p, err := r.ResolveProxy(context.Background(), 9, 100)
		require.NoError(t, err)
		require.NotNil(t, p)
		seen[p.ID]++
	}
	require.Equal(t, 2, seen[1])
	require.Equal(t, 2, seen[2])
	require.Equal(t, 2, seen[3])
}

func TestDefaultProxyGroupResolver_StickyDoesNotWriteProxyID(t *testing.T) {
	t.Parallel()
	group := &ProxyGroup{ID: 1, StickyByAccount: true, Status: StatusActive}
	members := []Proxy{
		{ID: 10, Status: StatusActive},
		{ID: 20, Status: StatusActive},
	}
	r := NewDefaultProxyGroupResolver(
		&stubProxyGroupRepo{group: group},
		&stubProxyRepoForGroup{members: members},
	)
	first, err := r.ResolveProxy(context.Background(), 1, 42)
	require.NoError(t, err)
	require.NotNil(t, first)
	for i := 0; i < 5; i++ {
		p, err := r.ResolveProxy(context.Background(), 1, 42)
		require.NoError(t, err)
		require.Equal(t, first.ID, p.ID)
	}
}

func TestDefaultProxyGroupResolver_NoHealthyMembers(t *testing.T) {
	t.Parallel()
	now := time.Now()
	expired := now.Add(-time.Hour)
	group := &ProxyGroup{ID: 1, Status: StatusActive, Strategy: ProxyGroupStrategyRoundRobin}
	r := NewDefaultProxyGroupResolver(
		&stubProxyGroupRepo{group: group},
		&stubProxyRepoForGroup{members: []Proxy{{ID: 1, Status: StatusActive, ExpiresAt: &expired}}},
	)
	r.now = func() time.Time { return now }
	p, err := r.ResolveProxy(context.Background(), 1, 1)
	require.NoError(t, err)
	require.Nil(t, p)
}

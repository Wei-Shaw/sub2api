package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type memHealthCache struct {
	mu   sync.Mutex
	data map[int64]*ProxyHealthMeta
}

func (c *memHealthCache) GetProxyHealth(_ context.Context, proxyID int64) (*ProxyHealthMeta, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		return nil, nil
	}
	m, ok := c.data[proxyID]
	if !ok {
		return nil, nil
	}
	cp := *m
	return &cp, nil
}

func (c *memHealthCache) SetProxyHealth(_ context.Context, proxyID int64, meta *ProxyHealthMeta) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = make(map[int64]*ProxyHealthMeta)
	}
	cp := *meta
	c.data[proxyID] = &cp
	return nil
}

type healthProxyRepoStub struct {
	mu      sync.Mutex
	proxies map[int64]*Proxy
	groups  map[int64][]int64
}

func (r *healthProxyRepoStub) Create(context.Context, *Proxy) error { panic("unused") }
func (r *healthProxyRepoStub) GetByID(_ context.Context, id int64) (*Proxy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.proxies[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *p
	return &cp, nil
}
func (r *healthProxyRepoStub) ListByIDs(context.Context, []int64) ([]Proxy, error) {
	panic("unused")
}
func (r *healthProxyRepoStub) Update(_ context.Context, proxy *Proxy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *proxy
	r.proxies[proxy.ID] = &cp
	return nil
}
func (r *healthProxyRepoStub) Delete(context.Context, int64) error { panic("unused") }
func (r *healthProxyRepoStub) List(context.Context, pagination.PaginationParams) ([]Proxy, *pagination.PaginationResult, error) {
	panic("unused")
}
func (r *healthProxyRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]Proxy, *pagination.PaginationResult, error) {
	panic("unused")
}
func (r *healthProxyRepoStub) ListWithFiltersAndAccountCount(context.Context, pagination.PaginationParams, string, string, string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error) {
	panic("unused")
}
func (r *healthProxyRepoStub) ListActive(context.Context) ([]Proxy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Proxy, 0)
	for _, p := range r.proxies {
		if p.Status == StatusActive {
			out = append(out, *p)
		}
	}
	return out, nil
}
func (r *healthProxyRepoStub) ListActiveWithAccountCount(context.Context) ([]ProxyWithAccountCount, error) {
	panic("unused")
}
func (r *healthProxyRepoStub) ExistsByHostPortAuth(context.Context, string, int, string, string) (bool, error) {
	panic("unused")
}
func (r *healthProxyRepoStub) CountAccountsByProxyID(context.Context, int64) (int64, error) {
	panic("unused")
}
func (r *healthProxyRepoStub) ListAccountSummariesByProxyID(context.Context, int64) ([]ProxyAccountSummary, error) {
	panic("unused")
}
func (r *healthProxyRepoStub) SweepExpiredProxies(context.Context, time.Time) (int64, error) {
	panic("unused")
}
func (r *healthProxyRepoStub) ListAllForFallback(context.Context) ([]Proxy, error) { panic("unused") }
func (r *healthProxyRepoStub) CountExpired(context.Context) (int64, error)         { panic("unused") }
func (r *healthProxyRepoStub) CountExpiringSoon(context.Context, time.Time) (int64, error) {
	panic("unused")
}
func (r *healthProxyRepoStub) ListByGroupID(_ context.Context, groupID int64) ([]Proxy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := r.groups[groupID]
	out := make([]Proxy, 0, len(ids))
	for _, id := range ids {
		if p, ok := r.proxies[id]; ok {
			out = append(out, *p)
		}
	}
	return out, nil
}
func (r *healthProxyRepoStub) CountByGroupID(context.Context, int64) (int64, error) { panic("unused") }
func (r *healthProxyRepoStub) UpdateHealthAudit(context.Context, int64, int, *time.Time, string) error {
	return nil
}
func (r *healthProxyRepoStub) GetHealthAudit(context.Context, int64) (int, *time.Time, string, error) {
	return 0, nil, "", nil
}
func (r *healthProxyRepoStub) CountHealthIsolated(context.Context) (int64, error) { return 0, nil }
func (r *healthProxyRepoStub) ListHealthIsolated(context.Context, int) ([]Proxy, error) { return nil, nil }


type healthGroupRepoStub struct {
	groups []ProxyGroup
}

func (r *healthGroupRepoStub) Create(context.Context, *ProxyGroup) error { panic("unused") }
func (r *healthGroupRepoStub) GetByID(context.Context, int64) (*ProxyGroup, error) {
	panic("unused")
}
func (r *healthGroupRepoStub) Update(context.Context, *ProxyGroup) error { panic("unused") }
func (r *healthGroupRepoStub) Delete(context.Context, int64) error       { panic("unused") }
func (r *healthGroupRepoStub) List(context.Context, pagination.PaginationParams) ([]ProxyGroup, *pagination.PaginationResult, error) {
	panic("unused")
}
func (r *healthGroupRepoStub) ListActive(context.Context) ([]ProxyGroup, error) {
	return r.groups, nil
}
func (r *healthGroupRepoStub) CountProxiesByGroupID(context.Context, int64) (int64, error) {
	panic("unused")
}
func (r *healthGroupRepoStub) CountAccountsByGroupID(context.Context, int64) (int64, error) {
	panic("unused")
}
func (r *healthGroupRepoStub) SetGroupMembers(context.Context, int64, []int64) error {
	panic("unused")
}

type healthProberStub struct{}

func (p *healthProberStub) ProbeProxy(_ context.Context, proxyURL string) (*ProxyExitInfo, int64, error) {
	if proxyURL == "" {
		return nil, 0, errors.New("empty")
	}
	// Convention: host "bad.example" fails
	if containsHost(proxyURL, "bad.example") {
		return nil, 0, errors.New("dial timeout")
	}
	return &ProxyExitInfo{IP: "1.2.3.4", Country: "US", CountryCode: "US"}, 50, nil
}

func containsHost(u, host string) bool {
	return len(u) >= len(host) && (u == host || (len(u) > 0 && (indexOf(u, host) >= 0)))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

type healthResolverStub struct {
	invalidated []int64
}

func (r *healthResolverStub) ResolveProxy(context.Context, int64, int64) (*Proxy, error) {
	return nil, nil
}
func (r *healthResolverStub) InvalidateGroup(groupID int64) {
	r.invalidated = append(r.invalidated, groupID)
}

func TestProxyHealthService_RunOnceIsolatesAndSkipsWarp(t *testing.T) {
	gid := int64(7)
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			1: {ID: 1, Name: "good", Protocol: "socks5", Host: "good.example", Port: 1080, Status: StatusActive, GroupID: &gid},
			2: {ID: 2, Name: "bad", Protocol: "socks5", Host: "bad.example", Port: 1080, Status: StatusActive, GroupID: &gid},
			3: {ID: 3, Name: "warp-x", Protocol: "socks5", Host: "bad.example", Port: 1081, Status: StatusActive, GroupID: &gid},
		},
		groups: map[int64][]int64{7: {1, 2, 3}},
	}
	groups := &healthGroupRepoStub{groups: []ProxyGroup{{ID: 7, Name: "pool", Status: StatusActive}}}
	health := &memHealthCache{data: map[int64]*ProxyHealthMeta{}}
	resolver := &healthResolverStub{}
	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		Enabled:          true,
		FailThreshold:    2,
		SuccessThreshold: 2,
		ProbeScope:       "group_members",
		AutoRecover:      true,
		Concurrency:      2,
		BatchSize:        100,
		TimeoutMS:        1000,
		SkipNamePrefix:   []string{"warp-"},
	}}
	svc := NewProxyHealthService(cfg, repo, groups, &healthProberStub{}, health, nil, resolver, ProvideProxyHealthMetrics(), nil)

	// First fail — not isolated yet
	res, err := svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, res.Isolated)
	require.Equal(t, StatusActive, repo.proxies[2].Status)

	// Second fail — isolate
	res, err = svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, res.Isolated)
	require.Equal(t, StatusInactive, repo.proxies[2].Status)
	require.Equal(t, StatusActive, repo.proxies[1].Status)
	require.Equal(t, StatusActive, repo.proxies[3].Status) // warp skipped, never isolated by this worker
	require.Contains(t, resolver.invalidated, gid)

	meta, _ := health.GetProxyHealth(context.Background(), 2)
	require.NotNil(t, meta)
	require.Equal(t, ProxyHealthIsolatedByHealth, meta.IsolatedBy)
}

func TestProxyHealthService_RunOnceRecoversHealthIsolated(t *testing.T) {
	gid := int64(3)
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			9: {ID: 9, Name: "was-bad", Protocol: "http", Host: "good.example", Port: 8080, Status: StatusInactive, GroupID: &gid},
		},
		groups: map[int64][]int64{3: {9}},
	}
	groups := &healthGroupRepoStub{groups: []ProxyGroup{{ID: 3, Name: "g", Status: StatusActive}}}
	health := &memHealthCache{data: map[int64]*ProxyHealthMeta{
		9: {IsolatedBy: ProxyHealthIsolatedByHealth, FailCount: 3},
	}}
	// Manual inactive without isolated_by must not recover
	repo.proxies[10] = &Proxy{ID: 10, Name: "manual", Protocol: "http", Host: "good.example", Port: 8081, Status: StatusInactive, GroupID: &gid}
	repo.groups[3] = append(repo.groups[3], 10)

	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		FailThreshold:    3,
		SuccessThreshold: 2,
		ProbeScope:       "group_members",
		AutoRecover:      true,
		Concurrency:      2,
		BatchSize:        50,
		TimeoutMS:        1000,
		SkipNamePrefix:   []string{"warp-"},
	}}
	svc := NewProxyHealthService(cfg, repo, groups, &healthProberStub{}, health, nil, &healthResolverStub{}, nil, nil)

	_, err := svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusInactive, repo.proxies[9].Status) // need 2 successes

	_, err = svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusActive, repo.proxies[9].Status)
	require.Equal(t, StatusInactive, repo.proxies[10].Status) // manual stays down
}

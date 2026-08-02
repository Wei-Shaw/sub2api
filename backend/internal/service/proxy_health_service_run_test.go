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

func (c *memHealthCache) CompareAndSetProxyHealth(_ context.Context, proxyID int64, expectedVersion int64, meta *ProxyHealthMeta) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = make(map[int64]*ProxyHealthMeta)
	}
	cur, ok := c.data[proxyID]
	var curVer int64
	if ok && cur != nil {
		curVer = cur.Version
	}
	if curVer != expectedVersion {
		return false, nil
	}
	cp := *meta
	c.data[proxyID] = &cp
	return true, nil
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
func (r *healthProxyRepoStub) UpdateHealthAudit(_ context.Context, proxyID int64, failCount int, lastHealthAt *time.Time, isolatedBy string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.proxies[proxyID]; ok {
		p.HealthFailCount = failCount
		p.HealthIsolatedBy = isolatedBy
		p.LastHealthAt = lastHealthAt
	}
	return nil
}
func (r *healthProxyRepoStub) GetHealthAudit(_ context.Context, proxyID int64) (int, *time.Time, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.proxies[proxyID]
	if !ok {
		return 0, nil, "", nil
	}
	return p.HealthFailCount, p.LastHealthAt, p.HealthIsolatedBy, nil
}
func (r *healthProxyRepoStub) CountHealthIsolated(context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, p := range r.proxies {
		if p.HealthIsolatedBy == ProxyHealthIsolatedByHealth {
			n++
		}
	}
	return n, nil
}
func (r *healthProxyRepoStub) ListHealthIsolated(_ context.Context, limit int) ([]Proxy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Proxy, 0)
	for _, p := range r.proxies {
		if p.HealthIsolatedBy == ProxyHealthIsolatedByHealth {
			out = append(out, *p)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (r *healthProxyRepoStub) ListHealthIsolatedByID(_ context.Context, afterID int64, limit int) ([]Proxy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Proxy, 0)
	for _, p := range r.proxies {
		if p.ID <= afterID {
			continue
		}
		if p.Status != StatusInactive {
			continue
		}
		if p.HealthIsolatedBy != ProxyHealthIsolatedByHealth {
			continue
		}
		out = append(out, *p)
	}
	// id ASC for rotating cursor (map iteration order is nondeterministic).
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].ID < out[i].ID {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (r *healthProxyRepoStub) UpdateStatusWithHealthIsolation(_ context.Context, proxyID int64, status string, failCount int, lastHealthAt *time.Time, isolatedBy string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.proxies[proxyID]
	if !ok {
		return errors.New("not found")
	}
	p.Status = status
	p.HealthFailCount = failCount
	p.HealthIsolatedBy = isolatedBy
	p.LastHealthAt = lastHealthAt
	return nil
}
func (r *healthProxyRepoStub) ClearAccountProxyBindings(context.Context, int64) (int64, error) {
	return 0, nil
}

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

func TestProxyHealthService_AllActiveRecoversHealthIsolated(t *testing.T) {
	// probe_scope=all_active historically only listed StatusActive, so AutoRecover
	// could never re-probe health-isolated rows. This regression covers that path.
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			1: {
				ID: 1, Name: "isolated", Protocol: "http", Host: "good.example", Port: 8080,
				Status: StatusInactive, HealthIsolatedBy: ProxyHealthIsolatedByHealth,
			},
			2: {
				ID: 2, Name: "manual", Protocol: "http", Host: "good.example", Port: 8081,
				Status: StatusInactive, // no health mark
			},
			3: {
				ID: 3, Name: "live", Protocol: "http", Host: "good.example", Port: 8082,
				Status: StatusActive,
			},
		},
	}
	health := &memHealthCache{data: map[int64]*ProxyHealthMeta{
		1: {IsolatedBy: ProxyHealthIsolatedByHealth, FailCount: 5, Version: 1},
	}}
	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		FailThreshold:    3,
		SuccessThreshold: 2,
		ProbeScope:       "all_active",
		AutoRecover:      true,
		Concurrency:      2,
		BatchSize:        50,
		TimeoutMS:        1000,
		SkipNamePrefix:   []string{"warp-"},
	}}
	svc := NewProxyHealthService(cfg, repo, nil, &healthProberStub{}, health, nil, &healthResolverStub{}, nil, nil)

	_, err := svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusInactive, repo.proxies[1].Status) // need 2 successes

	res, err := svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, res.Recovered)
	require.Equal(t, StatusActive, repo.proxies[1].Status)
	require.Equal(t, StatusInactive, repo.proxies[2].Status)
	require.Equal(t, StatusActive, repo.proxies[3].Status)
}

func TestProxyHealthService_RunOnceProcessSingleflight(t *testing.T) {
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			1: {ID: 1, Name: "slow", Protocol: "http", Host: "good.example", Port: 9, Status: StatusActive},
		},
	}
	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		FailThreshold: 3, SuccessThreshold: 2, ProbeScope: "all_active",
		AutoRecover: true, Concurrency: 1, BatchSize: 10, TimeoutMS: 500,
	}}
	svc := NewProxyHealthService(cfg, repo, nil, &healthProberStub{}, &memHealthCache{}, nil, nil, nil, nil)

	// Hold the process lock as if a scan is in flight.
	svc.runMu.Lock()
	svc.runActive = true
	svc.runMu.Unlock()

	_, err := svc.RunOnce(context.Background())
	require.ErrorIs(t, err, ErrProxyHealthScanBusy)

	svc.runMu.Lock()
	svc.runActive = false
	svc.runMu.Unlock()

	res, err := svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestProxyHealthService_RunOnceLeaderLockContended(t *testing.T) {
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			1: {ID: 1, Name: "p", Protocol: "http", Host: "good.example", Port: 1, Status: StatusActive},
		},
	}
	lock := &fakeLeaderLockCache{}
	// Peer already holds the shared key.
	ok, err := lock.TryAcquireLeaderLock(context.Background(), proxyHealthWorkerLockKey, "peer", time.Minute)
	require.NoError(t, err)
	require.True(t, ok)

	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		FailThreshold: 3, SuccessThreshold: 2, ProbeScope: "all_active",
		AutoRecover: true, Concurrency: 1, BatchSize: 10, TimeoutMS: 500,
		IntervalSec: 60, LeaderLockTTLSec: 50,
	}}
	svc := NewProxyHealthService(cfg, repo, nil, &healthProberStub{}, &memHealthCache{}, nil, nil, nil, nil)
	svc.SetLeaderLock(lock, "local", nil)

	_, err = svc.RunOnce(context.Background())
	require.ErrorIs(t, err, ErrProxyHealthScanBusy)
}

func TestProxyHealthService_LoadMetaFillsIsolatedByFromDB(t *testing.T) {
	// Redis miss (no key); DB still has health mark → seed from audit so recover works.
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			5: {
				ID: 5, Name: "redis-lost", Protocol: "http", Host: "good.example", Port: 8080,
				Status: StatusInactive, HealthIsolatedBy: ProxyHealthIsolatedByHealth, HealthFailCount: 4,
			},
		},
	}
	// Empty Redis cache — key absent, loadMeta seeds from DB only.
	health := &memHealthCache{}
	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		FailThreshold: 3, SuccessThreshold: 2, ProbeScope: "all_active",
		AutoRecover: true, Concurrency: 1, BatchSize: 10, TimeoutMS: 500,
		SkipNamePrefix: []string{"warp-"},
	}}
	svc := NewProxyHealthService(cfg, repo, nil, &healthProberStub{}, health, nil, &healthResolverStub{}, nil, nil)

	meta, err := svc.loadMeta(context.Background(), 5)
	require.NoError(t, err)
	require.Equal(t, ProxyHealthIsolatedByHealth, meta.IsolatedBy)
	require.Equal(t, 4, meta.FailCount)

	_, runErr := svc.RunOnce(context.Background())
	require.NoError(t, runErr)
	require.Equal(t, StatusInactive, repo.proxies[5].Status) // need 2 successes

	res, runErr := svc.RunOnce(context.Background())
	require.NoError(t, runErr)
	require.Equal(t, 1, res.Recovered)
	require.Equal(t, StatusActive, repo.proxies[5].Status)
}

func TestProxyHealthService_LoadMetaTrustsRedisOverDB(t *testing.T) {
	// Redis hit with FailCount=0 must not be overridden by stale DB fail_count.
	lastChecked := time.Now().Add(-time.Minute)
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			8: {
				ID: 8, Name: "recovered", Protocol: "http", Host: "good.example", Port: 8080,
				Status: StatusActive, HealthFailCount: 5, LastHealthAt: &lastChecked,
			},
		},
	}
	health := &memHealthCache{data: map[int64]*ProxyHealthMeta{
		8: {FailCount: 0, LastCheckedAt: time.Now().Unix(), Version: 3},
	}}
	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		FailThreshold: 3, SuccessThreshold: 2, ProbeScope: "all_active",
		AutoRecover: true, Concurrency: 1, BatchSize: 10, TimeoutMS: 500,
	}}
	svc := NewProxyHealthService(cfg, repo, nil, &healthProberStub{}, health, nil, nil, nil, nil)

	meta, err := svc.loadMeta(context.Background(), 8)
	require.NoError(t, err)
	require.Equal(t, 0, meta.FailCount, "Redis hit must trust FailCount=0; no DB overlay")
	require.Equal(t, int64(3), meta.Version)
}

func TestProxyHealthService_IsolatedRecoverCursorRotation(t *testing.T) {
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			10: {ID: 10, Name: "a", Protocol: "http", Host: "good.example", Port: 1, Status: StatusInactive, HealthIsolatedBy: ProxyHealthIsolatedByHealth},
			20: {ID: 20, Name: "b", Protocol: "http", Host: "good.example", Port: 2, Status: StatusInactive, HealthIsolatedBy: ProxyHealthIsolatedByHealth},
			30: {ID: 30, Name: "c", Protocol: "http", Host: "good.example", Port: 3, Status: StatusInactive, HealthIsolatedBy: ProxyHealthIsolatedByHealth},
		},
	}
	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		FailThreshold: 3, SuccessThreshold: 99, // never recover — just exercise listing
		ProbeScope: "all_active", AutoRecover: true, Concurrency: 1, BatchSize: 50, TimeoutMS: 500,
	}}
	svc := NewProxyHealthService(cfg, repo, nil, &healthProberStub{}, &memHealthCache{}, nil, nil, nil, nil)

	// Page size 2: first tick gets 10,20 and advances cursor to 20.
	first, err := svc.listHealthIsolatedForRecover(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, first, 2)
	require.Equal(t, int64(10), first[0].ID)
	require.Equal(t, int64(20), first[1].ID)
	require.Equal(t, int64(20), svc.isolatedRecoverCursor)

	// Second tick gets 30 only.
	second, err := svc.listHealthIsolatedForRecover(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Equal(t, int64(30), second[0].ID)
	require.Equal(t, int64(30), svc.isolatedRecoverCursor)

	// Third tick wraps to start.
	third, err := svc.listHealthIsolatedForRecover(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, third, 2)
	require.Equal(t, int64(10), third[0].ID)
	require.Equal(t, int64(20), third[1].ID)
}

func TestMemHealthCache_CompareAndSet(t *testing.T) {
	c := &memHealthCache{}
	ok, err := c.CompareAndSetProxyHealth(context.Background(), 7, 0, &ProxyHealthMeta{FailCount: 1, Version: 1})
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = c.CompareAndSetProxyHealth(context.Background(), 7, 0, &ProxyHealthMeta{FailCount: 99, Version: 1})
	require.NoError(t, err)
	require.False(t, ok, "stale expected version must fail")

	ok, err = c.CompareAndSetProxyHealth(context.Background(), 7, 1, &ProxyHealthMeta{FailCount: 2, Version: 2})
	require.NoError(t, err)
	require.True(t, ok)

	got, err := c.GetProxyHealth(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, 2, got.FailCount)
	require.Equal(t, int64(2), got.Version)
}

// H1: admin deactivated between CAS and isolate → recheck skips.
func TestProxyHealthService_RecheckBeforeIsolate_SkipsWhenAdminInactive(t *testing.T) {
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			1: {ID: 1, Name: "p", Protocol: "http", Host: "bad.example", Port: 1, Status: StatusInactive},
		},
	}
	health := &memHealthCache{data: map[int64]*ProxyHealthMeta{
		1: {FailCount: 5, Version: 2},
	}}
	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		FailThreshold: 3, SuccessThreshold: 2, ProbeScope: "all_active",
		AutoRecover: true, Concurrency: 1, BatchSize: 10, TimeoutMS: 500,
	}}
	svc := NewProxyHealthService(cfg, repo, nil, &healthProberStub{}, health, nil, nil, nil, nil)

	_, _, ok := svc.recheckBeforeIsolate(context.Background(), 1, 3)
	require.False(t, ok, "admin-deactivated proxy must not be isolated again")
}

// H1: admin cleared fail counters below threshold → recheck skips isolate.
func TestProxyHealthService_RecheckBeforeIsolate_SkipsWhenFailCountCleared(t *testing.T) {
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			1: {ID: 1, Name: "p", Protocol: "http", Host: "bad.example", Port: 1, Status: StatusActive},
		},
	}
	health := &memHealthCache{data: map[int64]*ProxyHealthMeta{
		1: {FailCount: 0, Version: 9}, // admin clear
	}}
	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		FailThreshold: 3, SuccessThreshold: 2,
	}}
	svc := NewProxyHealthService(cfg, repo, nil, &healthProberStub{}, health, nil, nil, nil, nil)

	_, meta, ok := svc.recheckBeforeIsolate(context.Background(), 1, 3)
	require.False(t, ok)
	require.NotNil(t, meta)
	require.Equal(t, 0, meta.FailCount)
}

// H1: happy path isolate recheck.
func TestProxyHealthService_RecheckBeforeIsolate_OK(t *testing.T) {
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			1: {ID: 1, Name: "p", Protocol: "http", Host: "bad.example", Port: 1, Status: StatusActive},
		},
	}
	health := &memHealthCache{data: map[int64]*ProxyHealthMeta{
		1: {FailCount: 4, Version: 2},
	}}
	svc := NewProxyHealthService(&config.Config{}, repo, nil, &healthProberStub{}, health, nil, nil, nil, nil)

	p, meta, ok := svc.recheckBeforeIsolate(context.Background(), 1, 3)
	require.True(t, ok)
	require.Equal(t, StatusActive, p.Status)
	require.Equal(t, 4, meta.FailCount)
}

// H1: admin cleared IsolatedBy → recheck skips recover.
func TestProxyHealthService_RecheckBeforeRecover_SkipsWhenIsolatedByCleared(t *testing.T) {
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			9: {ID: 9, Name: "p", Protocol: "http", Host: "good.example", Port: 1, Status: StatusInactive},
		},
	}
	health := &memHealthCache{data: map[int64]*ProxyHealthMeta{
		9: {IsolatedBy: "", SuccessCount: 5, Version: 4}, // admin cleared mark
	}}
	svc := NewProxyHealthService(&config.Config{}, repo, nil, &healthProberStub{}, health, nil, nil, nil, nil)

	_, meta, ok := svc.recheckBeforeRecover(context.Background(), 9, 2)
	require.False(t, ok, "admin-cleared IsolatedBy must block recover")
	require.Equal(t, "", meta.IsolatedBy)
}

// H1: status already active → skip recover.
func TestProxyHealthService_RecheckBeforeRecover_SkipsWhenNotInactive(t *testing.T) {
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			9: {ID: 9, Name: "p", Protocol: "http", Host: "good.example", Port: 1, Status: StatusActive},
		},
	}
	health := &memHealthCache{data: map[int64]*ProxyHealthMeta{
		9: {IsolatedBy: ProxyHealthIsolatedByHealth, SuccessCount: 5},
	}}
	svc := NewProxyHealthService(&config.Config{}, repo, nil, &healthProberStub{}, health, nil, nil, nil, nil)

	_, _, ok := svc.recheckBeforeRecover(context.Background(), 9, 2)
	require.False(t, ok)
}

// H1: probeAndEvaluate does not isolate when admin sets inactive after counter CAS
// (UpdateHealthAudit hook flips status before recheckBeforeIsolate).
func TestProxyHealthService_ProbeAndEvaluate_SkipsIsolateAfterAdminInactive(t *testing.T) {
	base := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			2: {ID: 2, Name: "race", Protocol: "socks5", Host: "bad.example", Port: 1080, Status: StatusActive},
		},
	}
	repo := &casDeactivateRepo{healthProxyRepoStub: base, deactivateAfterAudit: true}
	health := &memHealthCache{data: map[int64]*ProxyHealthMeta{
		2: {FailCount: 1, Version: 1}, // one fail already; threshold=2 → this fail would isolate
	}}
	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		FailThreshold: 2, SuccessThreshold: 2, AutoRecover: true, TimeoutMS: 500,
	}}
	svc := NewProxyHealthService(cfg, repo, nil, &healthProberStub{}, health, nil, &healthResolverStub{}, nil, nil)

	proxy := *base.proxies[2]
	isolated, recovered, err := svc.probeAndEvaluate(context.Background(), proxy)
	require.NoError(t, err)
	require.False(t, isolated, "must skip isolate when admin deactivated after CAS")
	require.False(t, recovered)
	require.Equal(t, StatusInactive, base.proxies[2].Status)
	meta, _ := health.GetProxyHealth(context.Background(), 2)
	require.NotNil(t, meta)
	require.NotEqual(t, ProxyHealthIsolatedByHealth, meta.IsolatedBy)
}

// H1: probeAndEvaluate does not recover when admin clears IsolatedBy after counter CAS.
func TestProxyHealthService_ProbeAndEvaluate_SkipsRecoverAfterAdminClear(t *testing.T) {
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			9: {
				ID: 9, Name: "was-bad", Protocol: "http", Host: "good.example", Port: 8080,
				Status: StatusInactive, HealthIsolatedBy: ProxyHealthIsolatedByHealth,
			},
		},
	}
	inner := &memHealthCache{data: map[int64]*ProxyHealthMeta{
		9: {IsolatedBy: ProxyHealthIsolatedByHealth, SuccessCount: 1, FailCount: 0, Version: 1},
	}}
	health := &casClearHealth{memHealthCache: inner, clearAfterCAS: true}
	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		FailThreshold: 3, SuccessThreshold: 2, AutoRecover: true, TimeoutMS: 500,
	}}
	svc := NewProxyHealthService(cfg, repo, nil, &healthProberStub{}, health, nil, &healthResolverStub{}, nil, nil)

	proxy := *repo.proxies[9]
	isolated, recovered, err := svc.probeAndEvaluate(context.Background(), proxy)
	require.NoError(t, err)
	require.False(t, isolated)
	require.False(t, recovered, "must skip recover when admin cleared IsolatedBy after CAS")
	require.Equal(t, StatusInactive, repo.proxies[9].Status)
}

// A3: Redis hit with empty IsolatedBy must NOT fall back to DB health mark.
func TestProxyHealthService_ListGroupMembers_TrustsRedisEmptyIsolatedBy(t *testing.T) {
	gid := int64(1)
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			// DB still says health-isolated, but Redis was cleared by admin.
			11: {
				ID: 11, Name: "cleared", Protocol: "http", Host: "good.example", Port: 1,
				Status: StatusInactive, HealthIsolatedBy: ProxyHealthIsolatedByHealth, GroupID: &gid,
			},
			// Redis still marks health — should be included.
			12: {
				ID: 12, Name: "health-iso", Protocol: "http", Host: "good.example", Port: 2,
				Status: StatusInactive, HealthIsolatedBy: ProxyHealthIsolatedByHealth, GroupID: &gid,
			},
			// Active always included.
			13: {
				ID: 13, Name: "live", Protocol: "http", Host: "good.example", Port: 3,
				Status: StatusActive, GroupID: &gid,
			},
		},
		groups: map[int64][]int64{1: {11, 12, 13}},
	}
	groups := &healthGroupRepoStub{groups: []ProxyGroup{{ID: 1, Name: "g", Status: StatusActive}}}
	health := &memHealthCache{data: map[int64]*ProxyHealthMeta{
		11: {IsolatedBy: "", FailCount: 0, Version: 5}, // admin clear — trust Redis
		12: {IsolatedBy: ProxyHealthIsolatedByHealth, FailCount: 3, Version: 2},
	}}
	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		FailThreshold: 3, SuccessThreshold: 2, ProbeScope: "group_members",
		AutoRecover: true, Concurrency: 1, BatchSize: 50, TimeoutMS: 500,
	}}
	svc := NewProxyHealthService(cfg, repo, groups, &healthProberStub{}, health, nil, nil, nil, nil)

	cands, err := svc.listGroupMemberCandidates(context.Background())
	require.NoError(t, err)
	ids := make([]int64, 0, len(cands))
	for _, p := range cands {
		ids = append(ids, p.ID)
	}
	require.NotContains(t, ids, int64(11), "Redis empty IsolatedBy must exclude despite DB health mark")
	require.Contains(t, ids, int64(12))
	require.Contains(t, ids, int64(13))
}

// A3: true Redis miss still falls back to DB audit for candidates.
func TestProxyHealthService_ListGroupMembers_DBFallbackOnRedisMiss(t *testing.T) {
	gid := int64(2)
	repo := &healthProxyRepoStub{
		proxies: map[int64]*Proxy{
			21: {
				ID: 21, Name: "db-only", Protocol: "http", Host: "good.example", Port: 1,
				Status: StatusInactive, HealthIsolatedBy: ProxyHealthIsolatedByHealth, GroupID: &gid,
			},
		},
		groups: map[int64][]int64{2: {21}},
	}
	groups := &healthGroupRepoStub{groups: []ProxyGroup{{ID: 2, Name: "g", Status: StatusActive}}}
	health := &memHealthCache{} // miss
	cfg := &config.Config{ProxyHealth: config.ProxyHealthConfig{
		ProbeScope: "group_members", AutoRecover: true,
	}}
	svc := NewProxyHealthService(cfg, repo, groups, &healthProberStub{}, health, nil, nil, nil, nil)

	cands, err := svc.listGroupMemberCandidates(context.Background())
	require.NoError(t, err)
	require.Len(t, cands, 1)
	require.Equal(t, int64(21), cands[0].ID)
}

// casClearHealth wraps memHealthCache and clears IsolatedBy after a successful CAS
// (simulates admin clear racing after counter write, before recheck).
type casClearHealth struct {
	*memHealthCache
	clearAfterCAS bool
}

func (c *casClearHealth) CompareAndSetProxyHealth(ctx context.Context, proxyID int64, expectedVersion int64, meta *ProxyHealthMeta) (bool, error) {
	ok, err := c.memHealthCache.CompareAndSetProxyHealth(ctx, proxyID, expectedVersion, meta)
	if ok && c.clearAfterCAS {
		c.mu.Lock()
		if m, exists := c.data[proxyID]; exists && m != nil {
			cp := *m
			cp.IsolatedBy = ""
			cp.IsolatedAt = 0
			cp.Version++ // admin bump
			c.data[proxyID] = &cp
		}
		c.mu.Unlock()
	}
	return ok, err
}

// casDeactivateRepo flips proxy status to inactive after UpdateHealthAudit
// (simulates admin deactivate racing after counter write, before recheck).
type casDeactivateRepo struct {
	*healthProxyRepoStub
	deactivateAfterAudit bool
}

func (r *casDeactivateRepo) UpdateHealthAudit(ctx context.Context, proxyID int64, failCount int, lastHealthAt *time.Time, isolatedBy string) error {
	err := r.healthProxyRepoStub.UpdateHealthAudit(ctx, proxyID, failCount, lastHealthAt, isolatedBy)
	if r.deactivateAfterAudit {
		r.mu.Lock()
		if p, ok := r.proxies[proxyID]; ok {
			p.Status = StatusInactive
			p.HealthIsolatedBy = ""
		}
		r.mu.Unlock()
	}
	return err
}

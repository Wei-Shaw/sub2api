package service

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type proxyPoolServiceTestRepo struct {
	mu            sync.Mutex
	pool          *ProxyPool
	proxies       map[int64]ProxyPoolProxy
	counts        map[int64]int64
	accountProxy  map[int64]int64
	unassigned    []int64
	assignments   []ProxyPoolAccountAssignment
	pending       []int64
	healthUpdates []proxyPoolHealthUpdate
	logs          []ProxyPoolRebindLog
}

type proxyPoolHealthUpdate struct {
	proxyID    int64
	health     string
	failures   int
	grokStatus string
}

func newProxyPoolServiceTestRepo(pool *ProxyPool, proxies ...ProxyPoolProxy) *proxyPoolServiceTestRepo {
	repo := &proxyPoolServiceTestRepo{
		pool:         pool,
		proxies:      make(map[int64]ProxyPoolProxy, len(proxies)),
		counts:       make(map[int64]int64),
		accountProxy: make(map[int64]int64),
	}
	for _, proxy := range proxies {
		repo.proxies[proxy.ID] = proxy
	}
	return repo
}

func (r *proxyPoolServiceTestRepo) CreatePool(_ context.Context, pool *ProxyPool) (*ProxyPool, error) {
	r.pool = pool
	return pool, nil
}

func (r *proxyPoolServiceTestRepo) UpdatePool(_ context.Context, pool *ProxyPool) error {
	r.pool = pool
	return nil
}

func (r *proxyPoolServiceTestRepo) DeletePool(_ context.Context, _ int64) error { return nil }

func (r *proxyPoolServiceTestRepo) GetPoolByID(_ context.Context, id int64) (*ProxyPool, error) {
	if r.pool == nil || r.pool.ID != id {
		return nil, ErrProxyPoolNotFound
	}
	copy := *r.pool
	return &copy, nil
}

func (r *proxyPoolServiceTestRepo) ListPools(_ context.Context) ([]ProxyPool, error) {
	if r.pool == nil {
		return []ProxyPool{}, nil
	}
	return []ProxyPool{*r.pool}, nil
}

func (r *proxyPoolServiceTestRepo) ListPoolsWithStats(_ context.Context) ([]ProxyPoolWithStats, error) {
	if r.pool == nil {
		return []ProxyPoolWithStats{}, nil
	}
	return []ProxyPoolWithStats{{ProxyPool: *r.pool}}, nil
}

func (r *proxyPoolServiceTestRepo) ListPoolProxies(_ context.Context, poolID int64) ([]ProxyPoolProxy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]ProxyPoolProxy, 0, len(r.proxies))
	for _, proxy := range r.proxies {
		if proxy.PoolID == poolID {
			result = append(result, proxy)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (r *proxyPoolServiceTestRepo) AssignProxiesToPool(_ context.Context, poolID int64, proxyIDs []int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var changed int64
	for _, id := range proxyIDs {
		proxy, ok := r.proxies[id]
		if !ok {
			continue
		}
		proxy.PoolID = poolID
		r.proxies[id] = proxy
		changed++
	}
	return changed, nil
}

func (r *proxyPoolServiceTestRepo) RemoveProxiesFromPool(_ context.Context, poolID int64, proxyIDs []int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var changed int64
	for _, id := range proxyIDs {
		proxy, ok := r.proxies[id]
		if !ok || proxy.PoolID != poolID {
			continue
		}
		proxy.PoolID = 0
		r.proxies[id] = proxy
		changed++
	}
	return changed, nil
}

func (r *proxyPoolServiceTestRepo) UpdateProxyPoolHealth(_ context.Context, poolID, proxyID int64, snapshot ProxyPoolHealthSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	proxy := r.proxies[proxyID]
	if proxy.PoolID != poolID {
		return nil
	}
	proxy.PoolHealth = snapshot.Health
	proxy.PoolFailures = snapshot.Failures
	proxy.PoolCheckedAt = &snapshot.CheckedAt
	proxy.GrokQualityStatus = snapshot.GrokQualityStatus
	proxy.GrokQualityCheckedAt = snapshot.GrokQualityCheckedAt
	proxy.GrokQualityHTTPStatus = snapshot.GrokQualityHTTPStatus
	proxy.GrokQualityMessage = snapshot.GrokQualityMessage
	r.proxies[proxyID] = proxy
	r.healthUpdates = append(r.healthUpdates, proxyPoolHealthUpdate{
		proxyID: proxyID, health: snapshot.Health, failures: snapshot.Failures, grokStatus: snapshot.GrokQualityStatus,
	})
	return nil
}

func (r *proxyPoolServiceTestRepo) ListPoolUnassignedAccountIDs(_ context.Context, _ int64) ([]int64, error) {
	return append([]int64(nil), r.unassigned...), nil
}

func (r *proxyPoolServiceTestRepo) ListAccountIDsByProxy(_ context.Context, _ int64, proxyID int64) ([]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]int64, 0)
	for accountID, assignedProxyID := range r.accountProxy {
		if assignedProxyID == proxyID {
			ids = append(ids, accountID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (r *proxyPoolServiceTestRepo) CountAccountsByProxyIDs(_ context.Context, proxyIDs []int64) (map[int64]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[int64]int64, len(proxyIDs))
	for _, proxyID := range proxyIDs {
		result[proxyID] = r.counts[proxyID]
	}
	return result, nil
}

func (r *proxyPoolServiceTestRepo) BindAccountsToPool(_ context.Context, _ int64, assignments []ProxyPoolAccountAssignment) ([]ProxyPoolAccountAssignment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, assignment := range assignments {
		if previous, ok := r.accountProxy[assignment.AccountID]; ok && r.counts[previous] > 0 {
			r.counts[previous]--
		}
		r.accountProxy[assignment.AccountID] = assignment.ProxyID
		r.counts[assignment.ProxyID]++
		r.assignments = append(r.assignments, assignment)
	}
	return append([]ProxyPoolAccountAssignment(nil), assignments...), nil
}

func (r *proxyPoolServiceTestRepo) MarkAccountsPendingInPool(_ context.Context, _ int64, accountIDs []int64) ([]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = append(r.pending, accountIDs...)
	return append([]int64(nil), accountIDs...), nil
}

func (r *proxyPoolServiceTestRepo) UnbindAccountsFromPool(_ context.Context, _ int64, _ []int64) (int64, error) {
	return 0, nil
}

func (r *proxyPoolServiceTestRepo) RecordRebindLog(_ context.Context, entry *ProxyPoolRebindLog) error {
	if entry != nil {
		r.logs = append(r.logs, *entry)
	}
	return nil
}

func (r *proxyPoolServiceTestRepo) ListRebindLogs(_ context.Context, _ int64, _ int) ([]ProxyPoolRebindLog, error) {
	return append([]ProxyPoolRebindLog(nil), r.logs...), nil
}

type proxyPoolServiceTestProber struct {
	results     map[string]error
	grokResults map[string]ProxyQualityCheckItem
	grokErrors  map[string]error
}

func (p *proxyPoolServiceTestProber) ProbeProxy(_ context.Context, proxyURL string) (*ProxyExitInfo, int64, error) {
	if err := p.results[proxyURL]; err != nil {
		return nil, 0, err
	}
	return &ProxyExitInfo{IP: "203.0.113.10"}, 10, nil
}

func (p *proxyPoolServiceTestProber) ProbeGrokQuality(_ context.Context, proxyURL string) (ProxyQualityCheckItem, error) {
	if err := p.grokErrors[proxyURL]; err != nil {
		return ProxyQualityCheckItem{Target: "grok", Status: "fail", Message: err.Error()}, err
	}
	if item, ok := p.grokResults[proxyURL]; ok {
		return item, nil
	}
	return ProxyQualityCheckItem{Target: "grok", Status: "pass", HTTPStatus: 401, Message: "reachable"}, nil
}

type blockingProxyPoolServiceTestProber struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingProxyPoolServiceTestProber) ProbeProxy(ctx context.Context, _ string) (*ProxyExitInfo, int64, error) {
	p.once.Do(func() { close(p.started) })
	select {
	case <-p.release:
		return &ProxyExitInfo{IP: "203.0.113.10"}, 10, nil
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
}

func (p *blockingProxyPoolServiceTestProber) ProbeGrokQuality(ctx context.Context, _ string) (ProxyQualityCheckItem, error) {
	if err := ctx.Err(); err != nil {
		return ProxyQualityCheckItem{Target: "grok", Status: "fail", Message: err.Error()}, err
	}
	return ProxyQualityCheckItem{Target: "grok", Status: "pass", HTTPStatus: 401}, nil
}

type selectiveBlockingProxyPoolServiceTestProber struct {
	blockedURL string
	started    chan struct{}
	release    chan struct{}
	once       sync.Once
}

func (p *selectiveBlockingProxyPoolServiceTestProber) ProbeProxy(ctx context.Context, proxyURL string) (*ProxyExitInfo, int64, error) {
	if proxyURL != p.blockedURL {
		return &ProxyExitInfo{IP: "203.0.113.10"}, 10, nil
	}
	p.once.Do(func() { close(p.started) })
	select {
	case <-p.release:
		return &ProxyExitInfo{IP: "203.0.113.11"}, 11, nil
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
}

func (p *selectiveBlockingProxyPoolServiceTestProber) ProbeGrokQuality(ctx context.Context, _ string) (ProxyQualityCheckItem, error) {
	if err := ctx.Err(); err != nil {
		return ProxyQualityCheckItem{Target: "grok", Status: "fail", Message: err.Error()}, err
	}
	return ProxyQualityCheckItem{Target: "grok", Status: "pass", HTTPStatus: 401}, nil
}

type countingProxyPoolServiceTestProber struct {
	calls        int
	qualityCalls int
}

func (p *countingProxyPoolServiceTestProber) ProbeProxy(_ context.Context, _ string) (*ProxyExitInfo, int64, error) {
	p.calls++
	return &ProxyExitInfo{IP: "203.0.113.10"}, 10, nil
}

func (p *countingProxyPoolServiceTestProber) ProbeGrokQuality(_ context.Context, _ string) (ProxyQualityCheckItem, error) {
	p.qualityCalls++
	return ProxyQualityCheckItem{Target: "grok", Status: "pass", HTTPStatus: 401}, nil
}

type genericOnlyProxyPoolServiceTestProber struct{}

func (p *genericOnlyProxyPoolServiceTestProber) ProbeProxy(_ context.Context, _ string) (*ProxyExitInfo, int64, error) {
	return &ProxyExitInfo{IP: "203.0.113.10"}, 10, nil
}

type proxyPoolServiceTestLatencyCache struct {
	values map[int64]*ProxyLatencyInfo
	err    error
}

func (c *proxyPoolServiceTestLatencyCache) GetProxyLatencies(_ context.Context, _ []int64) (map[int64]*ProxyLatencyInfo, error) {
	return c.values, c.err
}

func (c *proxyPoolServiceTestLatencyCache) SetProxyLatency(_ context.Context, proxyID int64, info *ProxyLatencyInfo) error {
	if c.values == nil {
		c.values = make(map[int64]*ProxyLatencyInfo)
	}
	c.values[proxyID] = info
	return c.err
}

func proxyPoolServiceTestProxy(id, poolID int64, health string, checkedAt *time.Time) ProxyPoolProxy {
	grokStatus := "unknown"
	if health == ProxyPoolHealthHealthy {
		grokStatus = proxyPoolGrokQualityPass
	}
	return ProxyPoolProxy{
		Proxy: Proxy{
			ID: id, Name: "proxy", Protocol: "http", Host: "proxy" + string(rune('a'+id-1)) + ".test", Port: 8080, Status: StatusActive,
		},
		PoolID: poolID, PoolHealth: health, PoolCheckedAt: checkedAt, GrokQualityStatus: grokStatus,
	}
}

func TestProxyPoolBindAccountsUsesLeastLoadedHealthyProxy(t *testing.T) {
	now := time.Now()
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive, HealthIntervalSeconds: 300, FailureThreshold: 2, AutoRebind: true}
	repo := newProxyPoolServiceTestRepo(
		pool,
		proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthHealthy, &now),
		proxyPoolServiceTestProxy(2, 1, ProxyPoolHealthHealthy, &now),
	)
	repo.counts[1] = 2
	repo.counts[2] = 0
	service := NewProxyPoolService(repo, nil, nil, nil, nil)

	result, err := service.BindAccounts(context.Background(), pool.ID, []int64{100, 101, 102})

	require.NoError(t, err)
	require.Equal(t, 3, result.Assigned)
	require.Zero(t, result.Failed)
	require.Equal(t, []ProxyPoolAccountAssignment{
		{AccountID: 100, ProxyID: 2},
		{AccountID: 101, ProxyID: 2},
		{AccountID: 102, ProxyID: 1},
	}, repo.assignments)
}

func TestProxyPoolBindAccountsQueuesUntilAHealthyProxyIsAvailable(t *testing.T) {
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive, HealthIntervalSeconds: 300, FailureThreshold: 2, AutoRebind: true}
	repo := newProxyPoolServiceTestRepo(pool, proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthUnknown, nil))
	service := NewProxyPoolService(repo, nil, nil, nil, nil)

	result, err := service.BindAccounts(context.Background(), pool.ID, []int64{100, 101, 100, 0})

	require.NoError(t, err)
	require.Equal(t, 2, result.Assigned)
	require.Equal(t, 2, result.Pending)
	require.Zero(t, result.Failed)
	require.Equal(t, []int64{100, 101}, repo.pending)
	require.Equal(t, []ProxyPoolAccountAssignment{
		{AccountID: 100, ProxyID: 0},
		{AccountID: 101, ProxyID: 0},
	}, result.Results)
	require.Empty(t, repo.assignments)
}

func TestProxyPoolRunPoolRebindsAfterFailureThreshold(t *testing.T) {
	now := time.Now()
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive, HealthIntervalSeconds: 30, FailureThreshold: 2, AutoRebind: true}
	healthy := proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthHealthy, &now)
	failing := proxyPoolServiceTestProxy(2, 1, ProxyPoolHealthUnknown, nil)
	repo := newProxyPoolServiceTestRepo(pool, healthy, failing)
	repo.counts[2] = 2
	repo.accountProxy[200] = 2
	repo.accountProxy[201] = 2
	prober := &proxyPoolServiceTestProber{results: map[string]error{failing.URL(): errors.New("dial failed")}}
	service := NewProxyPoolService(repo, prober, nil, nil, nil)

	require.Zero(t, service.RunPool(context.Background(), pool))
	require.Equal(t, ProxyPoolHealthUnknown, repo.proxies[2].PoolHealth)
	require.Equal(t, 1, repo.proxies[2].PoolFailures)
	require.Empty(t, repo.assignments)

	stale := time.Now().Add(-time.Minute)
	proxy := repo.proxies[2]
	proxy.PoolCheckedAt = &stale
	repo.proxies[2] = proxy

	require.Equal(t, 2, service.RunPool(context.Background(), pool))
	require.Equal(t, ProxyPoolHealthUnhealthy, repo.proxies[2].PoolHealth)
	require.Equal(t, 2, repo.proxies[2].PoolFailures)
	require.Equal(t, int64(1), repo.accountProxy[200])
	require.Equal(t, int64(1), repo.accountProxy[201])
	require.Len(t, repo.logs, 1)
	require.Equal(t, 2, repo.logs[0].AccountCount)
}

func TestProxyPoolRunPoolDoesNotRebindBelowFailureThreshold(t *testing.T) {
	now := time.Now()
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive, HealthIntervalSeconds: 30, FailureThreshold: 3, AutoRebind: true}
	target := proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthHealthy, &now)
	failing := proxyPoolServiceTestProxy(2, 1, ProxyPoolHealthHealthy, nil)
	failing.PoolFailures = 1
	repo := newProxyPoolServiceTestRepo(pool, target, failing)
	repo.counts[2] = 1
	repo.accountProxy[300] = 2
	prober := &proxyPoolServiceTestProber{results: map[string]error{failing.URL(): errors.New("timeout")}}
	service := NewProxyPoolService(repo, prober, nil, nil, nil)

	require.Zero(t, service.RunPool(context.Background(), pool))
	require.Equal(t, ProxyPoolHealthHealthy, repo.proxies[2].PoolHealth)
	require.Equal(t, 2, repo.proxies[2].PoolFailures)
	require.Equal(t, int64(2), repo.accountProxy[300])
	require.Empty(t, repo.assignments)
}

func TestProxyPoolRunPoolNowCoalescesConcurrentCheck(t *testing.T) {
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive, HealthIntervalSeconds: 30, FailureThreshold: 2, AutoRebind: true}
	repo := newProxyPoolServiceTestRepo(pool, proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthUnknown, nil))
	prober := &blockingProxyPoolServiceTestProber{started: make(chan struct{}), release: make(chan struct{})}
	service := NewProxyPoolService(repo, prober, nil, nil, nil)
	released := false
	defer func() {
		if !released {
			close(prober.release)
		}
	}()

	started, err := service.RunPoolNow(context.Background(), pool.ID)
	require.NoError(t, err)
	require.True(t, started)
	<-prober.started

	started, err = service.RunPoolNow(context.Background(), pool.ID)
	require.NoError(t, err)
	require.False(t, started)

	close(prober.release)
	released = true
	require.Eventually(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.healthUpdates) == 1
	}, time.Second, 10*time.Millisecond)
}

func TestProxyPoolBindAccountsDuringHealthCheck(t *testing.T) {
	now := time.Now()
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive, HealthIntervalSeconds: 30, FailureThreshold: 2, AutoRebind: true}
	repo := newProxyPoolServiceTestRepo(
		pool,
		proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthHealthy, &now),
		proxyPoolServiceTestProxy(2, 1, ProxyPoolHealthUnknown, nil),
	)
	prober := &blockingProxyPoolServiceTestProber{started: make(chan struct{}), release: make(chan struct{})}
	service := NewProxyPoolService(repo, prober, nil, nil, nil)
	released := false
	defer func() {
		if !released {
			close(prober.release)
		}
	}()

	started, err := service.RunPoolNow(context.Background(), pool.ID)
	require.NoError(t, err)
	require.True(t, started)
	<-prober.started

	result, err := service.BindAccounts(context.Background(), pool.ID, []int64{100})
	require.NoError(t, err)
	require.Equal(t, 1, result.Assigned)
	require.Equal(t, int64(1), repo.accountProxy[100])

	close(prober.release)
	released = true
}

func TestProxyPoolPersistsCompletedProbeBeforeWholePoolFinishes(t *testing.T) {
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive, HealthIntervalSeconds: 30, FailureThreshold: 2, AutoRebind: true}
	fast := proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthUnknown, nil)
	blocked := proxyPoolServiceTestProxy(2, 1, ProxyPoolHealthUnknown, nil)
	repo := newProxyPoolServiceTestRepo(pool, fast, blocked)
	prober := &selectiveBlockingProxyPoolServiceTestProber{
		blockedURL: blocked.URL(),
		started:    make(chan struct{}),
		release:    make(chan struct{}),
	}
	service := NewProxyPoolService(repo, prober, nil, nil, nil)
	done := make(chan struct{})
	go func() {
		service.RunPool(context.Background(), pool)
		close(done)
	}()
	<-prober.started

	require.Eventually(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return repo.proxies[fast.ID].PoolHealth == ProxyPoolHealthHealthy
	}, time.Second, 10*time.Millisecond)
	select {
	case <-done:
		t.Fatal("pool run completed before the blocked proxy was released")
	default:
	}

	close(prober.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pool run did not complete")
	}
}

func TestProxyPoolReusesFreshBatchTestCache(t *testing.T) {
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive, HealthIntervalSeconds: 300, FailureThreshold: 2, AutoRebind: true}
	proxy := proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthUnknown, nil)
	repo := newProxyPoolServiceTestRepo(pool, proxy)
	latency := int64(18)
	qualityCheckedAt := time.Now()
	httpStatus := 401
	cache := &proxyPoolServiceTestLatencyCache{values: map[int64]*ProxyLatencyInfo{
		proxy.ID: {
			Success: true, LatencyMs: &latency, UpdatedAt: qualityCheckedAt,
			GrokQualityStatus: proxyPoolGrokQualityPass, GrokQualityCheckedAt: &qualityCheckedAt,
			GrokQualityHTTPStatus: &httpStatus,
		},
	}}
	prober := &countingProxyPoolServiceTestProber{}
	service := NewProxyPoolService(repo, prober, cache, nil, nil)

	service.RunPool(context.Background(), pool)

	require.Zero(t, prober.calls)
	require.Zero(t, prober.qualityCalls)
	require.Equal(t, ProxyPoolHealthHealthy, repo.proxies[proxy.ID].PoolHealth)
	require.Equal(t, proxyPoolGrokQualityPass, repo.proxies[proxy.ID].GrokQualityStatus)
	require.Len(t, repo.healthUpdates, 1)
}

func TestProxyPoolGenericCacheCannotBypassGrokQuality(t *testing.T) {
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive, HealthIntervalSeconds: 300, FailureThreshold: 1, AutoRebind: true}
	proxy := proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthUnknown, nil)
	repo := newProxyPoolServiceTestRepo(pool, proxy)
	cache := &proxyPoolServiceTestLatencyCache{values: map[int64]*ProxyLatencyInfo{
		proxy.ID: {Success: true, UpdatedAt: time.Now()},
	}}
	prober := &proxyPoolServiceTestProber{
		results: map[string]error{},
		grokResults: map[string]ProxyQualityCheckItem{
			proxy.URL(): {Target: "grok", Status: "fail", HTTPStatus: 403, Message: "forbidden"},
		},
	}
	service := NewProxyPoolService(repo, prober, cache, nil, nil)

	service.RunPool(context.Background(), pool)

	require.Equal(t, ProxyPoolHealthUnhealthy, repo.proxies[proxy.ID].PoolHealth)
	require.Equal(t, "fail", repo.proxies[proxy.ID].GrokQualityStatus)
	require.NotNil(t, repo.proxies[proxy.ID].GrokQualityHTTPStatus)
	require.Equal(t, 403, *repo.proxies[proxy.ID].GrokQualityHTTPStatus)
}

func TestProxyPoolRequiresGrokQualityProber(t *testing.T) {
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive, HealthIntervalSeconds: 300, FailureThreshold: 1, AutoRebind: true}
	proxy := proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthUnknown, nil)
	repo := newProxyPoolServiceTestRepo(pool, proxy)
	prober := &genericOnlyProxyPoolServiceTestProber{}
	service := NewProxyPoolService(repo, prober, nil, nil, nil)

	service.RunPool(context.Background(), pool)

	require.Equal(t, ProxyPoolHealthUnhealthy, repo.proxies[proxy.ID].PoolHealth)
	require.Equal(t, proxyPoolGrokQualityFail, repo.proxies[proxy.ID].GrokQualityStatus)
}

func TestProxyPoolCancelledProbeDoesNotCountAsFailure(t *testing.T) {
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive, HealthIntervalSeconds: 30, FailureThreshold: 1, AutoRebind: true}
	proxy := proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthHealthy, nil)
	repo := newProxyPoolServiceTestRepo(pool, proxy)
	prober := &blockingProxyPoolServiceTestProber{started: make(chan struct{}), release: make(chan struct{})}
	service := NewProxyPoolService(repo, prober, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		service.RunPool(ctx, pool)
		close(done)
	}()
	<-prober.started
	cancel()
	<-done

	require.Equal(t, ProxyPoolHealthHealthy, repo.proxies[1].PoolHealth)
	require.Zero(t, repo.proxies[1].PoolFailures)
	require.Empty(t, repo.healthUpdates)
}

func TestProxyPoolGetPoolProxiesAttachesCachedExitInfo(t *testing.T) {
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive}
	repo := newProxyPoolServiceTestRepo(pool, proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthHealthy, nil))
	latency := int64(42)
	cache := &proxyPoolServiceTestLatencyCache{values: map[int64]*ProxyLatencyInfo{
		1: {
			LatencyMs:   &latency,
			IPAddress:   "203.0.113.25",
			Country:     "Example",
			CountryCode: "EX",
		},
	}}
	service := NewProxyPoolService(repo, nil, cache, nil, nil)

	proxies, err := service.GetPoolProxies(context.Background(), pool.ID)

	require.NoError(t, err)
	require.Len(t, proxies, 1)
	require.Equal(t, int64(42), *proxies[0].LatencyMs)
	require.Equal(t, "203.0.113.25", proxies[0].IPAddress)
	require.Equal(t, "Example", proxies[0].Country)
	require.Equal(t, "EX", proxies[0].CountryCode)
}

func TestProxyPoolGetPoolProxiesDegradesWhenLatencyCacheFails(t *testing.T) {
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive}
	repo := newProxyPoolServiceTestRepo(pool, proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthHealthy, nil))
	cache := &proxyPoolServiceTestLatencyCache{err: errors.New("redis unavailable")}
	service := NewProxyPoolService(repo, nil, cache, nil, nil)

	proxies, err := service.GetPoolProxies(context.Background(), pool.ID)

	require.NoError(t, err)
	require.Len(t, proxies, 1)
	require.Nil(t, proxies[0].LatencyMs)
}

func TestProxyPoolProbePreservesCachedQualityReport(t *testing.T) {
	pool := &ProxyPool{ID: 1, Status: ProxyPoolStatusActive, FailureThreshold: 2}
	proxy := proxyPoolServiceTestProxy(1, 1, ProxyPoolHealthUnknown, nil)
	repo := newProxyPoolServiceTestRepo(pool, proxy)
	score := 88
	checkedAt := int64(1234)
	grokCheckedAt := time.Now().Add(-10 * time.Minute)
	httpStatus := 401
	cache := &proxyPoolServiceTestLatencyCache{values: map[int64]*ProxyLatencyInfo{
		1: {
			QualityStatus:         "success",
			QualityScore:          &score,
			QualityGrade:          "A",
			QualitySummary:        "stable",
			QualityCheckedAt:      &checkedAt,
			QualityCFRay:          "ray-id",
			GrokQualityStatus:     proxyPoolGrokQualityPass,
			GrokQualityCheckedAt:  &grokCheckedAt,
			GrokQualityHTTPStatus: &httpStatus,
		},
	}}
	prober := &proxyPoolServiceTestProber{results: map[string]error{}}
	service := NewProxyPoolService(repo, prober, cache, nil, nil)

	service.RunPool(context.Background(), pool)

	stored := cache.values[1]
	require.NotNil(t, stored)
	require.True(t, stored.Success)
	require.Equal(t, "A", stored.QualityGrade)
	require.Equal(t, &score, stored.QualityScore)
	require.Equal(t, &checkedAt, stored.QualityCheckedAt)
	require.Equal(t, "ray-id", stored.QualityCFRay)
	require.Equal(t, proxyPoolGrokQualityPass, stored.GrokQualityStatus)
	require.NotNil(t, stored.GrokQualityCheckedAt)
}

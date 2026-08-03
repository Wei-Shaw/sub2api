//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakePoolRepo 内存版 ProxyPoolRepository，用于单元测试池调度逻辑。
type fakePoolRepo struct {
	mu           sync.Mutex
	pools        map[int64]*ProxyPool
	proxies      map[int64]*Proxy // proxyID -> proxy
	rebinds      [][2]*int64      // [from, to(nil=direct)]
	rebindErr    error
	accountCount map[int64]int64
	unassigned   []int64 // 池分配补全的待分配账号
	assignments [][2]int64 // [accountID, proxyID] 池服务分配记录
	logs         []ProxyPoolRebindLog
}

func newFakePoolRepo() *fakePoolRepo {
	return &fakePoolRepo{
		pools:        map[int64]*ProxyPool{},
		proxies:      map[int64]*Proxy{},
		accountCount: map[int64]int64{},
	}
}

func (f *fakePoolRepo) CreatePool(ctx context.Context, pool *ProxyPool) (*ProxyPool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pool.ID = int64(len(f.pools) + 1)
	f.pools[pool.ID] = pool
	return pool, nil
}

func (f *fakePoolRepo) GetPoolByID(ctx context.Context, id int64) (*ProxyPool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.pools[id]
	if !ok {
		return nil, ErrProxyPoolNotFound
	}
	cp := *p
	return &cp, nil
}

func (f *fakePoolRepo) ListPools(ctx context.Context) ([]ProxyPool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ProxyPool, 0, len(f.pools))
	for _, p := range f.pools {
		out = append(out, *p)
	}
	return out, nil
}

func (f *fakePoolRepo) ListPoolsWithStats(ctx context.Context) ([]ProxyPoolWithStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ProxyPoolWithStats, 0, len(f.pools))
	for _, p := range f.pools {
		out = append(out, ProxyPoolWithStats{ProxyPool: *p})
	}
	return out, nil
}

func (f *fakePoolRepo) UpdatePool(ctx context.Context, pool *ProxyPool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pools[pool.ID] = pool
	return nil
}

func (f *fakePoolRepo) DeletePool(ctx context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.pools, id)
	return nil
}

func (f *fakePoolRepo) ListPoolProxies(ctx context.Context, poolID int64) ([]Proxy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Proxy, 0)
	for _, p := range f.proxies {
		if p.PoolID != nil && *p.PoolID == poolID {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (f *fakePoolRepo) AssignProxiesToPool(ctx context.Context, poolID int64, proxyIDs []int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := int64(0)
	for _, pid := range proxyIDs {
		if p, ok := f.proxies[pid]; ok {
			id := poolID
			p.PoolID = &id
			n++
		}
	}
	return n, nil
}

func (f *fakePoolRepo) RemoveProxiesFromPool(ctx context.Context, proxyIDs []int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := int64(0)
	for _, pid := range proxyIDs {
		if p, ok := f.proxies[pid]; ok {
			p.PoolID = nil
			n++
		}
	}
	return n, nil
}

func (f *fakePoolRepo) UpdateProxyPoolHealth(ctx context.Context, proxyID int64, health string, failures int, checkedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.proxies[proxyID]; ok {
		p.PoolHealth = health
		p.PoolFailures = failures
		p.PoolCheckedAt = &checkedAt
	}
	return nil
}

func (f *fakePoolRepo) CountAccountsByProxyIDs(ctx context.Context, proxyIDs []int64) (map[int64]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[int64]int64, len(proxyIDs))
	for _, id := range proxyIDs {
		out[id] = f.accountCount[id]
	}
	return out, nil
}

func (f *fakePoolRepo) RebindAccountsOffProxy(ctx context.Context, fromProxyID int64, toProxyID *int64) ([]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rebindErr != nil {
		return nil, f.rebindErr
	}
	f.rebinds = append(f.rebinds, [2]*int64{&fromProxyID, toProxyID})
	count := f.accountCount[fromProxyID]
	ids := make([]int64, 0, count)
	for i := int64(0); i < count; i++ {
		ids = append(ids, i+1)
	}
	// 改投后 from 不再有账号
	f.accountCount[fromProxyID] = 0
	if toProxyID != nil {
		f.accountCount[*toProxyID] += count
	}
	return ids, nil
}

func (f *fakePoolRepo) RecordRebindLog(ctx context.Context, entry *ProxyPoolRebindLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, *entry)
	return nil
}

func (f *fakePoolRepo) ListRebindLogs(ctx context.Context, poolID int64, limit int) ([]ProxyPoolRebindLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ProxyPoolRebindLog, 0)
	for i := len(f.logs) - 1; i >= 0 && len(out) < limit; i-- {
		if f.logs[i].PoolID == poolID {
			out = append(out, f.logs[i])
		}
	}
	return out, nil
}

func (f *fakePoolRepo) ListPoolUnassignedAccountIDs(ctx context.Context, poolID int64) ([]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int64, len(f.unassigned))
	copy(out, f.unassigned)
	return out, nil
}

func (f *fakePoolRepo) AssignAccountToProxy(ctx context.Context, accountID int64, proxyID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.assignments = append(f.assignments, [2]int64{accountID, proxyID})
	return nil
}

// fakeProberURL 基于 URL 判定成功/延迟。

type fakeProberURL struct {
	mu      sync.Mutex
	ok      map[string]bool
	latency map[string]int64
}

func newFakeProberURL() *fakeProberURL {
	return &fakeProberURL{ok: map[string]bool{}, latency: map[string]int64{}}
}

func (f *fakeProberURL) set(url string, ok bool, latencyMs int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ok[url] = ok
	f.latency[url] = latencyMs
}

func (f *fakeProberURL) ProbeProxy(ctx context.Context, proxyURL string) (*ProxyExitInfo, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.ok[proxyURL] {
		return nil, 0, errors.New("probe failed: " + proxyURL)
	}
	return &ProxyExitInfo{IP: "1.2.3.4"}, f.latency[proxyURL], nil
}

func mkPoolProxy(id int64, poolID int64) *Proxy {
	pid := poolID
	return &Proxy{
		ID:           id,
		Name:         "p" + itoa(id),
		Protocol:     "http",
		Host:         "host-" + itoa(id),
		Port:         8080,
		Status:       StatusActive,
		PoolID:       &pid,
		PoolHealth:   PoolHealthUnknown,
		PoolFailures: 0,
	}
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func TestApplyProbeResultStateMachine(t *testing.T) {
	repo := newFakePoolRepo()
	svc := NewProxyPoolService(repo, nil, nil, time.Minute)
	now := time.Now()
	pool := &ProxyPool{ID: 1, Name: "pool", Status: StatusActive, HealthIntervalSeconds: 60, FailureThreshold: 2}

	t.Run("first failure below threshold keeps previous health", func(t *testing.T) {
		repo.proxies[1] = mkPoolProxy(1, 1)
		pp := repo.proxies[1]
		svc.applyProbeResult(context.Background(), pool, pp, false, 2, now)
		require.Equal(t, PoolHealthUnknown, pp.PoolHealth) // 未达阈值保留原状态
		require.Equal(t, 1, pp.PoolFailures)
		require.Equal(t, PoolHealthUnknown, repo.proxies[1].PoolHealth)
	})

	t.Run("second consecutive failure marks unhealthy", func(t *testing.T) {
		repo.proxies[1] = &Proxy{PoolID: i64p(1), PoolHealth: PoolHealthUnknown, PoolFailures: 1}
		pp := repo.proxies[1]
		svc.applyProbeResult(context.Background(), pool, pp, false, 2, now)
		require.Equal(t, PoolHealthUnhealthy, pp.PoolHealth)
		require.Equal(t, 2, pp.PoolFailures)
	})

	t.Run("success resets failures and marks healthy", func(t *testing.T) {
		repo.proxies[1] = &Proxy{PoolID: i64p(1), PoolHealth: PoolHealthUnhealthy, PoolFailures: 2}
		pp := repo.proxies[1]
		svc.applyProbeResult(context.Background(), pool, pp, true, 2, now)
		require.Equal(t, PoolHealthHealthy, pp.PoolHealth)
		require.Equal(t, 0, pp.PoolFailures)
	})
}

// i64p 已在 openai_gateway_record_usage_test.go 定义，直接复用（同包）。

func TestRebindUnhealthyDistribution(t *testing.T) {
	repo := newFakePoolRepo()
	now := time.Now()
	svc := NewProxyPoolService(repo, nil, nil, time.Minute)

	// 池内 3 个代理：1 健康（host-a），2 健康（host-b），3 不健康（有账号）
	p1 := mkPoolProxy(1, 1)
	p1.PoolHealth = PoolHealthHealthy
	p2 := mkPoolProxy(2, 1)
	p2.PoolHealth = PoolHealthHealthy
	p3 := mkPoolProxy(3, 1)
	p3.PoolHealth = PoolHealthUnhealthy
	p3.PoolFailures = 2
	repo.proxies[1] = p1
	repo.proxies[2] = p2
	repo.proxies[3] = p3
	repo.accountCount[3] = 5

	pool := &ProxyPool{ID: 1, Name: "pool", Status: StatusActive, HealthIntervalSeconds: 60, FailureThreshold: 2}
	rebound := svc.rebindUnhealthy(context.Background(), pool, []*Proxy{p1, p2, p3}, now)

	require.Equal(t, 5, rebound)
	require.Len(t, repo.rebinds, 1)
	require.Equal(t, int64(3), *repo.rebinds[0][0])
	require.NotNil(t, repo.rebinds[0][1])
	require.Equal(t, int64(1), *repo.rebinds[0][1]) // 候选按 ID 升序，第一个候选为 1
}

func TestAssignUnassignedBalancesAcrossHealthyProxies(t *testing.T) {
	repo := newFakePoolRepo()
	now := time.Now()
	svc := NewProxyPoolService(repo, nil, nil, time.Minute)

	// 池内 2 个健康代理；3 个待分配账号
	p1 := mkPoolProxy(1, 1)
	p1.PoolHealth = PoolHealthHealthy
	p2 := mkPoolProxy(2, 1)
	p2.PoolHealth = PoolHealthHealthy
	repo.proxies[1] = p1
	repo.proxies[2] = p2
	repo.accountCount[1] = 5 // proxy 1 已有 5 个账号 → 应优先分到 proxy 2
	repo.unassigned = []int64{10, 11, 12}

	pool := &ProxyPool{ID: 1, Name: "pool", Status: StatusActive, HealthIntervalSeconds: 60, FailureThreshold: 2}
	svc.assignUnassigned(context.Background(), pool, []*Proxy{p1, p2})

	require.Len(t, repo.assignments, 3)
	// 全部账号分配到账号数较少的 proxy 2
	for _, a := range repo.assignments {
		require.Equal(t, int64(2), a[1])
	}
	// 无健康代理时不分配
	repo.unassigned = []int64{13}
	repo.proxies[1].PoolHealth = PoolHealthUnhealthy
	repo.proxies[2].PoolHealth = PoolHealthUnhealthy
	svc.assignUnassigned(context.Background(), pool, []*Proxy{p1, p2})
	require.Len(t, repo.assignments, 3)
	_ = now
}

func TestRebindUnhealthyNoCandidateKeepsAccounts(t *testing.T) {
	repo := newFakePoolRepo()
	now := time.Now()
	svc := NewProxyPoolService(repo, nil, nil, time.Minute)

	// 池内唯一代理不健康 → 无候选 → 不改投
	p1 := mkPoolProxy(1, 1)
	p1.PoolHealth = PoolHealthUnhealthy
	p1.PoolFailures = 3
	repo.proxies[1] = p1
	repo.accountCount[1] = 5

	pool := &ProxyPool{ID: 1, Name: "pool", Status: StatusActive, HealthIntervalSeconds: 60, FailureThreshold: 2}
	rebound := svc.rebindUnhealthy(context.Background(), pool, []*Proxy{p1}, now)

	require.Equal(t, 0, rebound)
	require.Empty(t, repo.rebinds)
}

func TestRebindUnhealthySkipsZeroAccount(t *testing.T) {
	repo := newFakePoolRepo()
	now := time.Now()
	svc := NewProxyPoolService(repo, nil, nil, time.Minute)

	p1 := mkPoolProxy(1, 1)
	p1.PoolHealth = PoolHealthHealthy
	p2 := mkPoolProxy(2, 1)
	p2.PoolHealth = PoolHealthUnhealthy // 无账号
	repo.proxies[1] = p1
	repo.proxies[2] = p2
	repo.accountCount[2] = 0

	pool := &ProxyPool{ID: 1, Name: "pool", Status: StatusActive, HealthIntervalSeconds: 60, FailureThreshold: 2}
	rebound := svc.rebindUnhealthy(context.Background(), pool, []*Proxy{p1, p2}, now)

	require.Equal(t, 0, rebound)
	require.Empty(t, repo.rebinds)
}

func TestRunPoolEndToEnd(t *testing.T) {
	repo := newFakePoolRepo()
	prober := newFakeProberURL()
	svc := NewProxyPoolService(repo, prober, nil, time.Minute)

	pool := &ProxyPool{ID: 1, Name: "pool", Status: StatusActive, HealthIntervalSeconds: 60, FailureThreshold: 1, AutoRebind: true}
	repo.pools[1] = pool

	p1 := mkPoolProxy(1, 1)
	p2 := mkPoolProxy(2, 1)
	repo.proxies[1] = p1
	repo.proxies[2] = p2
	repo.accountCount[2] = 3

	// host-1 健康、host-2 不健康
	prober.set("http://host-1:8080", true, 100)
	prober.set("http://host-2:8080", false, 0)

	rebound := svc.RunPool(context.Background(), pool)

	require.Equal(t, 3, rebound)
	require.Equal(t, PoolHealthHealthy, repo.proxies[1].PoolHealth)
	require.Equal(t, PoolHealthUnhealthy, repo.proxies[2].PoolHealth)
	require.Equal(t, 1, repo.proxies[2].PoolFailures) // threshold=1：单次失败即判不健康
	require.Len(t, repo.rebinds, 1)
	require.Equal(t, int64(2), *repo.rebinds[0][0])
	require.Equal(t, int64(1), *repo.rebinds[0][1])
	require.Len(t, repo.logs, 1)
	require.Equal(t, 3, repo.logs[0].AccountCount)
	require.Equal(t, "unhealthy", repo.logs[0].Reason)

	// 第二轮：不健康代理恢复 → 不再改投
	prober.set("http://host-2:8080", true, 90)
	rebound2 := svc.RunPool(context.Background(), pool)
	require.Equal(t, 0, rebound2)
	require.Equal(t, PoolHealthHealthy, repo.proxies[2].PoolHealth)
}

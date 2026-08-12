package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// ProxyPoolService 代理池调度服务：
// 周期探测池内代理健康度，并把绑定到不健康代理的账号自动改投到池内健康代理。
// 请求热路径不变（仍读 account.proxy_id），仅通过数据库绑定变更实现故障转移。
type ProxyPoolService struct {
	repo         ProxyPoolRepository
	prober       ProxyExitInfoProber
	latencyCache ProxyLatencyCache
	interval     time.Duration
	lockCache    LeaderLockCache
	db           *sql.DB
	stopCh       chan struct{}
	stopOnce     sync.Once
	wg           sync.WaitGroup
}

const (
	poolProbeConcurrency    = 4
	proxyPoolSweepTimeout   = 10 * time.Minute
	proxyPoolSweepLockTTL   = 15 * time.Minute
	proxyPoolSweepLockKey   = "proxy_pool_sweep"
	proxyPoolSweepLockOwner = "pool"
)

// ProxyPoolRunError 表示一轮扫描至少有一个代理重绑失败；成功账号数由调用方单独返回。
type ProxyPoolRunError struct {
	FailedProxies int
	Err           error
}

func (e *ProxyPoolRunError) Error() string {
	if e == nil || e.Err == nil {
		return "proxy pool sweep failed"
	}
	return "proxy pool sweep failed: " + e.Err.Error()
}

func (e *ProxyPoolRunError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NewProxyPoolService 创建代理池调度服务。
// latencyCache 可为 nil（仅影响管理端延迟展示，不影响健康判定）。
func NewProxyPoolService(repo ProxyPoolRepository, prober ProxyExitInfoProber, latencyCache ProxyLatencyCache, interval time.Duration) *ProxyPoolService {
	return &ProxyPoolService{
		repo:         repo,
		prober:       prober,
		latencyCache: latencyCache,
		interval:     interval,
		stopCh:       make(chan struct{}),
	}
}

func (s *ProxyPoolService) Start() {
	if s == nil || s.repo == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *ProxyPoolService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

// SetLeaderLock 注入领导锁（多实例部署时保证同一时间只有一个实例执行探测/重绑）。
// 不注入时（单实例/测试）照常运行。
func (s *ProxyPoolService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

// runOnce 扫描所有 active 池。
func (s *ProxyPoolService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), proxyPoolSweepTimeout)
	defer cancel()

	// 领导锁 TTL 需大于单轮最坏耗时（含探测）。
	release, acquired := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, proxyPoolSweepLockKey, proxyPoolSweepLockOwner, proxyPoolSweepLockTTL)
	if !acquired {
		return
	}
	defer release()

	pools, err := s.repo.ListPools(ctx)
	if err != nil {
		log.Printf("[ProxyPool] list pools failed: %v", err)
		return
	}
	for i := range pools {
		pool := &pools[i]
		if !pool.IsActive() {
			continue
		}
		if _, runErr := s.RunPool(ctx, pool); runErr != nil {
			log.Printf("[ProxyPool] pool %d sweep failed: %v", pool.ID, runErr)
		}
	}
}

// RunPool 对一个池执行一轮「探测健康度 + 自动重绑」。
// 供调度器周期调用，也供管理端手动触发（同步返回）。
// 返回本轮被改投的账号总数；部分代理重绑失败时同时返回 ProxyPoolRunError。
func (s *ProxyPoolService) RunPool(ctx context.Context, pool *ProxyPool) (int, error) {
	if pool == nil || !pool.IsActive() || s.repo == nil {
		return 0, nil
	}
	proxies, err := s.repo.ListPoolProxies(ctx, pool.ID)
	if err != nil {
		return 0, fmt.Errorf("list pool %d proxies: %w", pool.ID, err)
	}
	now := time.Now()
	interval := pool.HealthInterval()
	threshold := pool.FailureThresholdValue()

	// 1. 待探测集合：从未探测 / 超过间隔 / 已不健康（快速恢复探测）
	needCheck := make([]*Proxy, 0, len(proxies))
	activeProxies := make([]*Proxy, 0, len(proxies))
	for i := range proxies {
		pp := &proxies[i]
		if pp.Status != StatusActive || pp.IsExpired(now) {
			continue
		}
		activeProxies = append(activeProxies, pp)
		checkedAt := pp.PoolCheckedAt
		if checkedAt == nil ||
			now.Sub(*checkedAt) >= interval ||
			pp.PoolHealth == PoolHealthUnhealthy {
			needCheck = append(needCheck, pp)
		}
	}

	// 2. 并发探测 → 更新内存快照 + 数据库健康字段
	results := s.probeAll(ctx, needCheck)
	for _, pp := range needCheck {
		res, ok := results[pp.ID]
		if !ok {
			continue
		}
		s.applyProbeResult(ctx, pool, pp, res.ok, threshold, now)
	}

	// 3. 自动重绑（可手动禁用）
	rebound := 0
	var rebindErr error
	if pool.AutoRebind {
		rebound, rebindErr = s.rebindUnhealthy(ctx, pool, activeProxies, now)
	}

	// 4. 为绑定池但尚未分配到池内代理的账号补分配（负载均衡到健康代理）。
	//    发生在探测之后，确保拿到最新健康状态。
	s.assignUnassigned(ctx, pool, activeProxies)

	return rebound, rebindErr
}

// runPoolManually 在与周期扫描相同的跨实例锁下执行一轮，并与发起请求的取消信号解耦。
// 手动任务一旦开始就应在有界超时内完成，避免客户端断开留下半轮健康状态和重绑结果。
func (s *ProxyPoolService) runPoolManually(ctx context.Context, pool *ProxyPool) (int, error) {
	if s == nil || s.repo == nil {
		return 0, errors.New("proxy pool service unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), proxyPoolSweepTimeout)
	defer cancel()

	release, acquired := tryAcquireSingletonLeaderLock(runCtx, s.lockCache, s.db, proxyPoolSweepLockKey, proxyPoolSweepLockOwner, proxyPoolSweepLockTTL)
	if !acquired {
		return 0, ErrProxyPoolRunInProgress
	}
	defer release()
	return s.RunPool(runCtx, pool)
}

type poolProbeResult struct {
	ok        bool
	latencyMs int64
}

// probeAll 并发探测代理，返回 proxyID -> 结果（仅包含探测过的代理）。
// assignUnassigned 为绑定池但 proxy_id 为空或不属于本池的账号分配健康代理。
// 按账号数最少的健康代理优先（负载均衡）；无健康代理时跳过，下轮再试。
func (s *ProxyPoolService) assignUnassigned(ctx context.Context, pool *ProxyPool, active []*Proxy) {
	healthy := make([]*Proxy, 0, len(active))
	for _, pp := range active {
		if pp.PoolHealth == PoolHealthHealthy {
			healthy = append(healthy, pp)
		}
	}
	if len(healthy) == 0 {
		return
	}
	accountIDs, err := s.repo.ListPoolUnassignedAccountIDs(ctx, pool.ID)
	if err != nil {
		log.Printf("[ProxyPool] pool %d list unassigned accounts failed: %v", pool.ID, err)
		return
	}
	if len(accountIDs) == 0 {
		return
	}
	ids := make([]int64, 0, len(healthy))
	for _, p := range healthy {
		ids = append(ids, p.ID)
	}
	counts, err := s.repo.CountAccountsByProxyIDs(ctx, ids)
	if err != nil {
		log.Printf("[ProxyPool] pool %d count accounts failed: %v", pool.ID, err)
		return
	}
	for _, accountID := range accountIDs {
		sort.SliceStable(healthy, func(i, j int) bool {
			left, right := counts[healthy[i].ID], counts[healthy[j].ID]
			if left == right {
				return healthy[i].ID < healthy[j].ID
			}
			return left < right
		})
		assigned := false
		for _, p := range healthy {
			if err := s.repo.AssignAccountToProxy(ctx, accountID, p.ID); err == nil {
				counts[p.ID]++
				assigned = true
				log.Printf("[ProxyPool] pool %d assigned account %d to proxy %d", pool.ID, accountID, p.ID)
				break
			}
		}
		_ = assigned
	}
}

func (s *ProxyPoolService) probeAll(ctx context.Context, proxies []*Proxy) map[int64]poolProbeResult {
	results := make(map[int64]poolProbeResult, len(proxies))
	if s.prober == nil || len(proxies) == 0 {
		return results
	}

	sem := make(chan struct{}, poolProbeConcurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, pp := range proxies {
		pp := pp
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ok, latencyMs := s.probeOne(ctx, pp)
			mu.Lock()
			results[pp.ID] = poolProbeResult{ok: ok, latencyMs: latencyMs}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results
}

// probeOne 探测单个代理并写入延迟缓存（供管理端展示）。
func (s *ProxyPoolService) probeOne(ctx context.Context, pp *Proxy) (bool, int64) {
	if s.prober == nil {
		return false, 0
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	info := &ProxyLatencyInfo{UpdatedAt: time.Now()}
	exitInfo, latencyMs, err := s.prober.ProbeProxy(probeCtx, pp.URL())
	if err != nil {
		info.Success = false
		info.Message = "pool probe failed"
		if s.latencyCache != nil {
			_ = s.latencyCache.SetProxyLatency(ctx, pp.ID, info)
		}
		return false, 0
	}
	info.Success = true
	info.LatencyMs = &latencyMs
	info.Message = "Proxy is accessible"
	info.IPAddress = exitInfo.IP
	info.Country = exitInfo.Country
	info.CountryCode = exitInfo.CountryCode
	info.Region = exitInfo.Region
	info.City = exitInfo.City
	if s.latencyCache != nil {
		_ = s.latencyCache.SetProxyLatency(ctx, pp.ID, info)
	}
	return true, latencyMs
}

// applyProbeResult 把单次探测结果落到内存快照与数据库健康字段。
// 连续失败达到阈值才判为 unhealthy；成功即恢复 healthy 并清零失败计数。
func (s *ProxyPoolService) applyProbeResult(ctx context.Context, pool *ProxyPool, pp *Proxy, ok bool, threshold int, now time.Time) {
	health := PoolHealthUnhealthy
	failures := pp.PoolFailures
	if ok {
		failures = 0
		health = PoolHealthHealthy
	} else {
		failures++
		if failures < threshold {
			// 未达阈值：保留原状态，仅累计失败次数
			health = pp.PoolHealth
		}
	}
	pp.PoolHealth = health
	pp.PoolFailures = failures
	pp.PoolCheckedAt = &now
	if err := s.repo.UpdateProxyPoolHealth(ctx, pp.ID, health, failures, now); err != nil {
		log.Printf("[ProxyPool] pool %d update health for proxy %d failed: %v", pool.ID, pp.ID, err)
	}
}

// rebindUnhealthy 把绑定在不健康代理上的账号改投到池内健康代理。
// 候选按 ID 升序，跨不健康代理轮询分配以分散负载；
// 无健康候选时保持原样（等待下一轮探测恢复），避免误改投直连。
// 返回被改投的账号总数。
func (s *ProxyPoolService) rebindUnhealthy(ctx context.Context, pool *ProxyPool, active []*Proxy, now time.Time) (int, error) {
	total := 0
	var rebindErrors []error
	candidates := make([]*Proxy, 0, len(active))
	for _, pp := range active {
		if pp.PoolHealth == PoolHealthHealthy {
			candidates = append(candidates, pp)
		}
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })

	unhealthy := make([]*Proxy, 0, len(active))
	for _, pp := range active {
		if pp.PoolHealth == PoolHealthUnhealthy {
			unhealthy = append(unhealthy, pp)
		}
	}
	if len(unhealthy) == 0 {
		return 0, nil
	}
	sort.SliceStable(unhealthy, func(i, j int) bool { return unhealthy[i].ID < unhealthy[j].ID })

	unhealthyIDs := make([]int64, 0, len(unhealthy))
	for _, pp := range unhealthy {
		unhealthyIDs = append(unhealthyIDs, pp.ID)
	}
	// 仅处理有绑定账号的不健康代理，避免无谓 UPDATE
	accountCounts, err := s.repo.CountAccountsByProxyIDs(ctx, unhealthyIDs)
	if err != nil {
		return 0, fmt.Errorf("count pool %d unhealthy proxy accounts: %w", pool.ID, err)
	}

	cursor := 0
	for _, pp := range unhealthy {
		if accountCounts[pp.ID] == 0 {
			continue
		}
		target := candidates[cursor%len(candidates)]
		cursor++
		targetID := target.ID
		changed, rebindErr := s.repo.RebindAccountsOffProxy(ctx, pp.ID, &targetID)
		if rebindErr != nil {
			log.Printf("[ProxyPool] pool %d rebind proxy %d -> %d failed: %v", pool.ID, pp.ID, target.ID, rebindErr)
			rebindErrors = append(rebindErrors, fmt.Errorf("rebind proxy %d to %d: %w", pp.ID, target.ID, rebindErr))
			continue
		}
		if len(changed) > 0 {
			total += len(changed)
			log.Printf("[ProxyPool] pool %d rebound %d accounts from proxy %d to proxy %d", pool.ID, len(changed), pp.ID, target.ID)
			if logErr := s.repo.RecordRebindLog(ctx, &ProxyPoolRebindLog{
				PoolID:       pool.ID,
				FromProxyID:  &pp.ID,
				ToProxyID:    &targetID,
				AccountCount: len(changed),
				Reason:       "unhealthy",
			}); logErr != nil {
				log.Printf("[ProxyPool] record rebind log failed: %v", logErr)
			}
		}
	}
	if len(rebindErrors) > 0 {
		return total, &ProxyPoolRunError{FailedProxies: len(rebindErrors), Err: errors.Join(rebindErrors...)}
	}
	return total, nil
}

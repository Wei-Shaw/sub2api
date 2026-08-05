package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ProxyPoolService owns proxy-pool CRUD and the periodic health/failover loop.
// The failover loop only changes accounts.proxy_id, so existing request paths
// immediately use the replacement without a gateway code change.
type ProxyPoolService struct {
	repo         ProxyPoolRepository
	prober       ProxyExitInfoProber
	latencyCache ProxyLatencyCache
	rdb          *redis.Client
	db           *sql.DB
	interval     time.Duration
	stopCh       chan struct{}
	stopOnce     sync.Once
	wg           sync.WaitGroup
	poolRuns     sync.Map
	bindRuns     sync.Map
}

const (
	proxyPoolSweepInterval           = 30 * time.Second
	proxyPoolRunTimeout              = 10 * time.Minute
	proxyPoolProbeLimit              = 32
	proxyPoolBindHealthyWait         = 20 * time.Second
	proxyPoolBindHealthyPollInterval = 250 * time.Millisecond
	proxyPoolLockAcquireTimeout      = 2 * time.Second
	proxyPoolLockTTL                 = 15 * time.Minute
	proxyPoolHealthWriteTimeout      = 5 * time.Second
	proxyPoolPostProcessTimeout      = 30 * time.Second
)

func NewProxyPoolService(repo ProxyPoolRepository, prober ProxyExitInfoProber, latencyCache ProxyLatencyCache, rdb *redis.Client, db *sql.DB) *ProxyPoolService {
	return &ProxyPoolService{
		repo:         repo,
		prober:       prober,
		latencyCache: latencyCache,
		rdb:          rdb,
		db:           db,
		interval:     proxyPoolSweepInterval,
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

func (s *ProxyPoolService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), proxyPoolRunTimeout)
	defer cancel()

	pools, err := s.repo.ListPools(ctx)
	if err != nil {
		log.Printf("[ProxyPool] list pools failed: %v", err)
		return
	}
	for i := range pools {
		if pools[i].IsActive() {
			s.RunPool(ctx, &pools[i])
		}
	}
}

func (s *ProxyPoolService) acquireDistributedLock(ctx context.Context, key string) (func(), bool) {
	if s.rdb == nil && s.db == nil {
		return func() {}, true
	}
	// Every production instance shares PostgreSQL. Using it as the primary lock
	// keeps healthy and Redis-degraded instances in the same lock domain.
	if s.db != nil {
		return tryAcquireDBAdvisoryLock(ctx, s.db, hashAdvisoryLockID(key))
	}

	// Redis remains a fallback for callers that intentionally construct the
	// service without a database handle.
	token := fmt.Sprintf("%d-%p", time.Now().UnixNano(), s)
	if s.rdb != nil {
		ok, err := s.rdb.SetNX(ctx, key, token, proxyPoolLockTTL).Result()
		if err == nil {
			if !ok {
				return func() {}, false
			}
			return func() {
				releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_, _ = s.rdb.Eval(releaseCtx, `
					if redis.call('get', KEYS[1]) == ARGV[1] then
						return redis.call('del', KEYS[1])
					end
					return 0
				`, []string{key}, token).Result()
			}, true
		}
		log.Printf("[ProxyPool] Redis leader lock failed: %v", err)
	}
	return nil, false
}

func (s *ProxyPoolService) acquireScopedPoolRun(ctx context.Context, runs *sync.Map, scope string, poolID int64) (func(), bool) {
	value, _ := runs.LoadOrStore(poolID, &sync.Mutex{})
	local := value.(*sync.Mutex)
	if !local.TryLock() {
		return nil, false
	}

	releaseDistributed, ok := s.acquireDistributedLock(ctx, fmt.Sprintf("sub2api:proxy-pool:%s:%d", scope, poolID))
	if !ok {
		local.Unlock()
		return nil, false
	}
	return func() {
		releaseDistributed()
		local.Unlock()
	}, true
}

func (s *ProxyPoolService) acquirePoolRun(ctx context.Context, poolID int64) (func(), bool) {
	return s.acquireScopedPoolRun(ctx, &s.poolRuns, "run", poolID)
}

func (s *ProxyPoolService) acquirePoolBind(ctx context.Context, poolID int64) (func(), bool) {
	return s.acquireScopedPoolRun(ctx, &s.bindRuns, "bind", poolID)
}

// RunPool executes one health-probe and failover round synchronously.
func (s *ProxyPoolService) RunPool(ctx context.Context, pool *ProxyPool) int {
	changed, _ := s.runPool(ctx, pool, false)
	return changed
}

func (s *ProxyPoolService) runPool(ctx context.Context, pool *ProxyPool, forceProbe bool) (int, bool) {
	if s == nil || s.repo == nil || pool == nil || !pool.IsActive() {
		return 0, true
	}
	release, ok := s.acquirePoolRun(ctx, pool.ID)
	if !ok {
		return 0, false
	}
	defer release()
	return s.runPoolLocked(ctx, pool, forceProbe), true
}

// startPoolRun acquires the health-check lock before returning, then performs
// the potentially long scan independently of the admin request lifetime.
func (s *ProxyPoolService) startPoolRun(pool *ProxyPool, forceProbe bool) bool {
	if s == nil || s.repo == nil || pool == nil || !pool.IsActive() {
		return false
	}
	select {
	case <-s.stopCh:
		return false
	default:
	}

	lockCtx, lockCancel := context.WithTimeout(context.Background(), proxyPoolLockAcquireTimeout)
	release, ok := s.acquirePoolRun(lockCtx, pool.ID)
	lockCancel()
	if !ok {
		return false
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer release()

		runCtx, cancel := context.WithTimeout(context.Background(), proxyPoolRunTimeout)
		defer cancel()
		watchDone := make(chan struct{})
		defer close(watchDone)
		go func() {
			select {
			case <-s.stopCh:
				cancel()
			case <-watchDone:
			}
		}()
		s.runPoolLocked(runCtx, pool, forceProbe)
	}()
	return true
}

func (s *ProxyPoolService) runPoolLocked(ctx context.Context, pool *ProxyPool, forceProbe bool) int {
	proxies, err := s.repo.ListPoolProxies(ctx, pool.ID)
	if err != nil {
		log.Printf("[ProxyPool] list pool %d proxies failed: %v", pool.ID, err)
		return 0
	}
	now := time.Now()
	s.applyCachedPoolHealth(ctx, pool, proxies, now)
	needCheck := poolProxiesDueForProbe(proxies, pool, forceProbe, now)
	s.probePoolMembers(ctx, pool, needCheck)

	healthy := make([]*ProxyPoolProxy, 0, len(proxies))
	for i := range proxies {
		if proxies[i].Status == StatusActive && proxies[i].PoolHealth == ProxyPoolHealthHealthy {
			healthy = append(healthy, &proxies[i])
		}
	}
	if len(healthy) == 0 {
		return 0
	}
	sort.SliceStable(healthy, func(i, j int) bool { return healthy[i].ID < healthy[j].ID })
	postCtx, cancel := context.WithTimeout(context.Background(), proxyPoolPostProcessTimeout)
	defer cancel()
	releaseBind, ok := s.acquirePoolBind(postCtx, pool.ID)
	if !ok {
		return 0
	}
	defer releaseBind()

	changed := 0
	if pool.AutoRebind {
		changed += s.rebindUnhealthy(postCtx, pool, proxies, healthy)
	}
	changed += s.assignUnassigned(postCtx, pool, healthy)
	return changed
}

func poolProxiesDueForProbe(proxies []ProxyPoolProxy, pool *ProxyPool, forceProbe bool, now time.Time) []*ProxyPoolProxy {
	needCheck := make([]*ProxyPoolProxy, 0, len(proxies))
	for i := range proxies {
		proxy := &proxies[i]
		if proxy.Status != StatusActive {
			continue
		}
		if forceProbe || proxy.PoolCheckedAt == nil || now.Sub(*proxy.PoolCheckedAt) >= pool.HealthInterval() || proxy.PoolHealth == ProxyPoolHealthUnhealthy {
			needCheck = append(needCheck, proxy)
		}
	}
	sort.SliceStable(needCheck, func(i, j int) bool {
		left, right := needCheck[i], needCheck[j]
		if left.PoolCheckedAt == nil && right.PoolCheckedAt != nil {
			return true
		}
		if left.PoolCheckedAt != nil && right.PoolCheckedAt == nil {
			return false
		}
		if left.PoolCheckedAt != nil && right.PoolCheckedAt != nil && !left.PoolCheckedAt.Equal(*right.PoolCheckedAt) {
			return left.PoolCheckedAt.Before(*right.PoolCheckedAt)
		}
		return left.ID < right.ID
	})
	return needCheck
}

func (s *ProxyPoolService) probePoolMembers(ctx context.Context, pool *ProxyPool, proxies []*ProxyPoolProxy) {
	s.probeAll(ctx, proxies, func(proxy *ProxyPoolProxy, result poolProbeResult, checkedAt time.Time) {
		s.applyPoolProbeResult(pool, proxy, result, checkedAt)
	})
}

func (s *ProxyPoolService) applyPoolProbeResult(pool *ProxyPool, proxy *ProxyPoolProxy, result poolProbeResult, checkedAt time.Time) {
	if pool == nil || proxy == nil {
		return
	}
	failures := proxy.PoolFailures
	health := proxy.PoolHealth
	if result.ok {
		failures = 0
		health = ProxyPoolHealthHealthy
	} else {
		failures++
		if failures >= pool.FailureThresholdValue() {
			health = ProxyPoolHealthUnhealthy
		}
	}
	proxy.PoolFailures = failures
	proxy.PoolHealth = health
	proxy.PoolCheckedAt = &checkedAt

	writeCtx, cancel := context.WithTimeout(context.Background(), proxyPoolHealthWriteTimeout)
	defer cancel()
	if err := s.repo.UpdateProxyPoolHealth(writeCtx, pool.ID, proxy.ID, health, failures, checkedAt); err != nil {
		log.Printf("[ProxyPool] update health for proxy %d failed: %v", proxy.ID, err)
	}
}

// applyCachedPoolHealth consumes fresh results produced by the existing proxy
// batch-test UI before scheduling any additional network probes.
func (s *ProxyPoolService) applyCachedPoolHealth(ctx context.Context, pool *ProxyPool, proxies []ProxyPoolProxy, now time.Time) {
	if s == nil || s.latencyCache == nil || pool == nil || len(proxies) == 0 {
		return
	}
	ids := make([]int64, 0, len(proxies))
	for i := range proxies {
		if proxies[i].Status == StatusActive {
			ids = append(ids, proxies[i].ID)
		}
	}
	latencies, err := s.latencyCache.GetProxyLatencies(ctx, ids)
	if err != nil {
		log.Printf("[ProxyPool] load cached proxy test results failed: %v", err)
		return
	}
	for i := range proxies {
		proxy := &proxies[i]
		info := latencies[proxy.ID]
		if proxy.Status != StatusActive || info == nil || info.UpdatedAt.IsZero() {
			continue
		}
		if info.UpdatedAt.After(now.Add(time.Minute)) || now.Sub(info.UpdatedAt) > pool.HealthInterval() {
			continue
		}
		if proxy.PoolCheckedAt != nil && !info.UpdatedAt.After(*proxy.PoolCheckedAt) {
			continue
		}
		s.applyPoolProbeResult(pool, proxy, poolProbeResult{ok: info.Success}, info.UpdatedAt)
	}
}

type poolProbeResult struct {
	ok bool
}

type poolProbeOutcome struct {
	proxy     *ProxyPoolProxy
	result    poolProbeResult
	checkedAt time.Time
}

func (s *ProxyPoolService) probeAll(ctx context.Context, proxies []*ProxyPoolProxy, onResult func(*ProxyPoolProxy, poolProbeResult, time.Time)) {
	if s.prober == nil || len(proxies) == 0 {
		return
	}
	workerCount := min(proxyPoolProbeLimit, len(proxies))
	jobs := make(chan *ProxyPoolProxy)
	outcomes := make(chan poolProbeOutcome, workerCount)
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for proxy := range jobs {
				if ctx.Err() != nil {
					return
				}
				probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				info, latency, err := s.prober.ProbeProxy(probeCtx, proxy.URL())
				cancel()
				if err != nil && ctx.Err() != nil {
					return
				}
				checkedAt := time.Now()
				result := poolProbeResult{ok: err == nil}
				cacheInfo := &ProxyLatencyInfo{Success: err == nil, UpdatedAt: checkedAt}
				if err != nil {
					cacheInfo.Message = err.Error()
				} else {
					cacheInfo.LatencyMs = &latency
					cacheInfo.Message = "Proxy is accessible"
					if info != nil {
						cacheInfo.IPAddress = info.IP
						cacheInfo.Country = info.Country
						cacheInfo.CountryCode = info.CountryCode
						cacheInfo.Region = info.Region
						cacheInfo.City = info.City
					}
				}
				outcomes <- poolProbeOutcome{proxy: proxy, result: result, checkedAt: checkedAt}
				cacheCtx, cacheCancel := context.WithTimeout(context.Background(), proxyPoolHealthWriteTimeout)
				s.saveProxyLatency(cacheCtx, proxy.ID, cacheInfo)
				cacheCancel()
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, proxy := range proxies {
			select {
			case jobs <- proxy:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(outcomes)
	}()
	for outcome := range outcomes {
		onResult(outcome.proxy, outcome.result, outcome.checkedAt)
	}
}

func (s *ProxyPoolService) saveProxyLatency(ctx context.Context, proxyID int64, info *ProxyLatencyInfo) {
	if s == nil || s.latencyCache == nil || info == nil {
		return
	}
	merged := *info
	if latencies, err := s.latencyCache.GetProxyLatencies(ctx, []int64{proxyID}); err == nil {
		if existing := latencies[proxyID]; existing != nil {
			merged.QualityStatus = existing.QualityStatus
			merged.QualityScore = existing.QualityScore
			merged.QualityGrade = existing.QualityGrade
			merged.QualitySummary = existing.QualitySummary
			merged.QualityCheckedAt = existing.QualityCheckedAt
			merged.QualityCFRay = existing.QualityCFRay
		}
	}
	if err := s.latencyCache.SetProxyLatency(ctx, proxyID, &merged); err != nil {
		log.Printf("[ProxyPool] store proxy latency cache failed: %v", err)
	}
}

func (s *ProxyPoolService) rebindUnhealthy(ctx context.Context, pool *ProxyPool, proxies []ProxyPoolProxy, healthy []*ProxyPoolProxy) int {
	counts, err := s.repo.CountAccountsByProxyIDs(ctx, proxyIDs(healthy))
	if err != nil {
		log.Printf("[ProxyPool] count healthy proxy accounts failed: %v", err)
		return 0
	}
	changed := 0
	for i := range proxies {
		from := &proxies[i]
		if from.PoolHealth != ProxyPoolHealthUnhealthy && from.Status == StatusActive {
			continue
		}
		accountIDs, err := s.repo.ListAccountIDsByProxy(ctx, pool.ID, from.ID)
		if err != nil || len(accountIDs) == 0 {
			continue
		}
		assignments := make([]ProxyPoolAccountAssignment, 0, len(accountIDs))
		for _, accountID := range accountIDs {
			target := leastLoadedProxy(healthy, counts, from.ID)
			if target == nil {
				break
			}
			assignments = append(assignments, ProxyPoolAccountAssignment{AccountID: accountID, ProxyID: target.ID})
			counts[target.ID]++
		}
		applied, bindErr := s.repo.BindAccountsToPool(ctx, pool.ID, assignments)
		if bindErr != nil {
			log.Printf("[ProxyPool] rebind proxy %d failed: %v", from.ID, bindErr)
			continue
		}
		if len(applied) > 0 {
			changed += len(applied)
			byTarget := make(map[int64]int)
			for _, assignment := range applied {
				byTarget[assignment.ProxyID]++
			}
			targetIDs := make([]int64, 0, len(byTarget))
			for targetID := range byTarget {
				targetIDs = append(targetIDs, targetID)
			}
			sort.Slice(targetIDs, func(i, j int) bool { return targetIDs[i] < targetIDs[j] })
			for _, targetID := range targetIDs {
				to := targetID
				_ = s.repo.RecordRebindLog(ctx, &ProxyPoolRebindLog{
					PoolID: pool.ID, FromProxyID: &from.ID, ToProxyID: &to,
					AccountCount: byTarget[targetID], Reason: "unhealthy",
				})
			}
		}
	}
	return changed
}

func (s *ProxyPoolService) assignUnassigned(ctx context.Context, pool *ProxyPool, healthy []*ProxyPoolProxy) int {
	accountIDs, err := s.repo.ListPoolUnassignedAccountIDs(ctx, pool.ID)
	if err != nil || len(accountIDs) == 0 {
		return 0
	}
	counts, err := s.repo.CountAccountsByProxyIDs(ctx, proxyIDs(healthy))
	if err != nil {
		return 0
	}
	assignments := make([]ProxyPoolAccountAssignment, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		target := leastLoadedProxy(healthy, counts, 0)
		if target == nil {
			break
		}
		assignments = append(assignments, ProxyPoolAccountAssignment{AccountID: accountID, ProxyID: target.ID})
		counts[target.ID]++
	}
	applied, err := s.repo.BindAccountsToPool(ctx, pool.ID, assignments)
	if err != nil {
		return 0
	}
	return len(applied)
}

func proxyIDs(proxies []*ProxyPoolProxy) []int64 {
	ids := make([]int64, 0, len(proxies))
	for _, proxy := range proxies {
		ids = append(ids, proxy.ID)
	}
	return ids
}

func leastLoadedProxy(proxies []*ProxyPoolProxy, counts map[int64]int64, exclude int64) *ProxyPoolProxy {
	var best *ProxyPoolProxy
	for _, proxy := range proxies {
		if proxy.ID == exclude {
			continue
		}
		if best == nil || counts[proxy.ID] < counts[best.ID] || (counts[proxy.ID] == counts[best.ID] && proxy.ID < best.ID) {
			best = proxy
		}
	}
	return best
}

// BindAccounts binds every selected account to the pool. Healthy members are
// assigned immediately; otherwise the binding remains pending until a health
// check finds an available member.
func (s *ProxyPoolService) BindAccounts(ctx context.Context, poolID int64, accountIDs []int64) (*ProxyPoolBindResult, error) {
	pool, err := s.repo.GetPoolByID(ctx, poolID)
	if err != nil {
		return nil, err
	}
	if !pool.IsActive() {
		return nil, ErrProxyPoolDisabled
	}
	uniqueAccountIDs := uniquePositiveIDs(accountIDs)
	if len(uniqueAccountIDs) == 0 {
		return &ProxyPoolBindResult{Results: []ProxyPoolAccountAssignment{}}, nil
	}
	release, ok := s.acquirePoolBind(ctx, poolID)
	if !ok {
		return nil, ErrProxyPoolBindBusy
	}
	defer release()

	proxies, err := s.repo.ListPoolProxies(ctx, poolID)
	if err != nil {
		return nil, err
	}
	s.applyCachedPoolHealth(ctx, pool, proxies, time.Now())
	healthy := healthyPoolProxies(proxies)
	if len(healthy) == 0 && s.prober != nil {
		// A new pool may not have a snapshot yet. Start (or join) the background
		// scan and wait briefly for its first incrementally persisted healthy IP.
		s.startPoolRun(pool, false)
		healthy, err = s.waitForHealthyPoolProxies(ctx, poolID)
		if err != nil {
			return nil, err
		}
	}
	if len(healthy) == 0 {
		pendingIDs, err := s.repo.MarkAccountsPendingInPool(ctx, poolID, uniqueAccountIDs)
		if err != nil {
			return nil, err
		}
		results := make([]ProxyPoolAccountAssignment, 0, len(pendingIDs))
		for _, accountID := range pendingIDs {
			results = append(results, ProxyPoolAccountAssignment{AccountID: accountID})
		}
		return &ProxyPoolBindResult{
			Assigned: len(pendingIDs),
			Pending:  len(pendingIDs),
			Failed:   len(uniqueAccountIDs) - len(pendingIDs),
			Results:  results,
		}, nil
	}
	counts, err := s.repo.CountAccountsByProxyIDs(ctx, proxyIDs(healthy))
	if err != nil {
		return nil, err
	}
	assignments := make([]ProxyPoolAccountAssignment, 0, len(uniqueAccountIDs))
	for _, accountID := range uniqueAccountIDs {
		target := leastLoadedProxy(healthy, counts, 0)
		assignments = append(assignments, ProxyPoolAccountAssignment{AccountID: accountID, ProxyID: target.ID})
		counts[target.ID]++
	}
	applied, err := s.repo.BindAccountsToPool(ctx, poolID, assignments)
	if err != nil {
		return nil, err
	}
	return &ProxyPoolBindResult{Assigned: len(applied), Failed: len(assignments) - len(applied), Results: applied}, nil
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func healthyPoolProxies(proxies []ProxyPoolProxy) []*ProxyPoolProxy {
	healthy := make([]*ProxyPoolProxy, 0, len(proxies))
	for i := range proxies {
		if proxies[i].Status == StatusActive && proxies[i].PoolHealth == ProxyPoolHealthHealthy {
			healthy = append(healthy, &proxies[i])
		}
	}
	sort.SliceStable(healthy, func(i, j int) bool { return healthy[i].ID < healthy[j].ID })
	return healthy
}

func (s *ProxyPoolService) waitForHealthyPoolProxies(ctx context.Context, poolID int64) ([]*ProxyPoolProxy, error) {
	waitCtx, cancel := context.WithTimeout(ctx, proxyPoolBindHealthyWait)
	defer cancel()
	ticker := time.NewTicker(proxyPoolBindHealthyPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, nil
		case <-ticker.C:
			proxies, err := s.repo.ListPoolProxies(waitCtx, poolID)
			if err != nil {
				return nil, err
			}
			if healthy := healthyPoolProxies(proxies); len(healthy) > 0 {
				return healthy, nil
			}
		}
	}
}

// CRUD and pool membership operations used by the admin handler.
func (s *ProxyPoolService) ListPools(ctx context.Context) ([]ProxyPoolWithStats, error) {
	return s.repo.ListPoolsWithStats(ctx)
}

func (s *ProxyPoolService) GetPool(ctx context.Context, id int64) (*ProxyPool, error) {
	return s.repo.GetPoolByID(ctx, id)
}

func (s *ProxyPoolService) GetPoolProxies(ctx context.Context, id int64) ([]ProxyPoolProxy, error) {
	if _, err := s.repo.GetPoolByID(ctx, id); err != nil {
		return nil, err
	}
	proxies, err := s.repo.ListPoolProxies(ctx, id)
	if err != nil {
		return nil, err
	}
	s.attachProxyLatency(ctx, proxies)
	return proxies, nil
}

func (s *ProxyPoolService) attachProxyLatency(ctx context.Context, proxies []ProxyPoolProxy) {
	if s == nil || s.latencyCache == nil || len(proxies) == 0 {
		return
	}
	ids := make([]int64, 0, len(proxies))
	for i := range proxies {
		ids = append(ids, proxies[i].ID)
	}
	latencies, err := s.latencyCache.GetProxyLatencies(ctx, ids)
	if err != nil {
		log.Printf("[ProxyPool] load proxy latency cache failed: %v", err)
		return
	}
	for i := range proxies {
		info := latencies[proxies[i].ID]
		if info == nil {
			continue
		}
		proxies[i].LatencyMs = info.LatencyMs
		proxies[i].IPAddress = info.IPAddress
		proxies[i].Country = info.Country
		proxies[i].CountryCode = info.CountryCode
	}
}

func (s *ProxyPoolService) CreatePool(ctx context.Context, input *CreateProxyPoolInput) (*ProxyPool, error) {
	if input == nil || strings.TrimSpace(input.Name) == "" {
		return nil, ErrProxyPoolNameRequired
	}
	pool := &ProxyPool{
		Name:                  strings.TrimSpace(input.Name),
		Description:           input.Description,
		Status:                input.Status,
		HealthIntervalSeconds: input.HealthIntervalSeconds,
		FailureThreshold:      input.FailureThreshold,
		AutoRebind:            input.AutoRebind,
	}
	if pool.Status == "" {
		pool.Status = ProxyPoolStatusActive
	}
	if pool.Status != ProxyPoolStatusActive && pool.Status != ProxyPoolStatusDisabled {
		return nil, ErrProxyPoolInvalidStatus
	}
	if pool.HealthIntervalSeconds <= 0 {
		pool.HealthIntervalSeconds = 300
	}
	if pool.FailureThreshold <= 0 {
		pool.FailureThreshold = 2
	}
	if !input.AutoRebind {
		pool.AutoRebind = false
	}
	return s.repo.CreatePool(ctx, pool)
}

func (s *ProxyPoolService) UpdatePool(ctx context.Context, id int64, input *UpdateProxyPoolInput) (*ProxyPool, error) {
	pool, err := s.repo.GetPoolByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if input != nil {
		if input.Name != nil {
			if strings.TrimSpace(*input.Name) == "" {
				return nil, ErrProxyPoolNameRequired
			}
			pool.Name = strings.TrimSpace(*input.Name)
		}
		if input.Description != nil {
			pool.Description = input.Description
		}
		if input.Status != nil {
			pool.Status = *input.Status
		}
		if input.HealthIntervalSeconds != nil {
			pool.HealthIntervalSeconds = *input.HealthIntervalSeconds
		}
		if input.FailureThreshold != nil {
			pool.FailureThreshold = *input.FailureThreshold
		}
		if input.AutoRebind != nil {
			pool.AutoRebind = *input.AutoRebind
		}
	}
	if pool.HealthIntervalSeconds <= 0 {
		pool.HealthIntervalSeconds = 300
	}
	if pool.FailureThreshold <= 0 {
		pool.FailureThreshold = 2
	}
	if pool.Status != ProxyPoolStatusActive && pool.Status != ProxyPoolStatusDisabled {
		return nil, ErrProxyPoolInvalidStatus
	}
	if err := s.repo.UpdatePool(ctx, pool); err != nil {
		return nil, err
	}
	return pool, nil
}

func (s *ProxyPoolService) DeletePool(ctx context.Context, id int64) error {
	return s.repo.DeletePool(ctx, id)
}

func (s *ProxyPoolService) AssignProxies(ctx context.Context, poolID int64, proxyIDs []int64) (int64, error) {
	if _, err := s.repo.GetPoolByID(ctx, poolID); err != nil {
		return 0, err
	}
	return s.repo.AssignProxiesToPool(ctx, poolID, proxyIDs)
}

func (s *ProxyPoolService) RemoveProxies(ctx context.Context, poolID int64, proxyIDs []int64) (int64, error) {
	if _, err := s.repo.GetPoolByID(ctx, poolID); err != nil {
		return 0, err
	}
	return s.repo.RemoveProxiesFromPool(ctx, poolID, proxyIDs)
}

func (s *ProxyPoolService) RunPoolNow(ctx context.Context, poolID int64) (bool, error) {
	pool, err := s.repo.GetPoolByID(ctx, poolID)
	if err != nil {
		return false, err
	}
	if !pool.IsActive() {
		return false, ErrProxyPoolDisabled
	}
	return s.startPoolRun(pool, true), nil
}

func (s *ProxyPoolService) RebindLogs(ctx context.Context, poolID int64, limit int) ([]ProxyPoolRebindLog, error) {
	if _, err := s.repo.GetPoolByID(ctx, poolID); err != nil {
		return nil, err
	}
	return s.repo.ListRebindLogs(ctx, poolID, limit)
}

func (s *ProxyPoolService) UnbindAccounts(ctx context.Context, poolID int64, accountIDs []int64) (int64, error) {
	if _, err := s.repo.GetPoolByID(ctx, poolID); err != nil {
		return 0, err
	}
	return s.repo.UnbindAccountsFromPool(ctx, poolID, accountIDs)
}

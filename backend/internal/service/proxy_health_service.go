package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// ProxyHealthRunResult summarizes one RunOnce tick.
type ProxyHealthRunResult struct {
	Probed    int
	Isolated  int
	Recovered int
	Skipped   int
	Errors    int
}

// ProxyHealthService probes proxies and isolates/recovers by consecutive thresholds.
type ProxyHealthService struct {
	cfg       *config.Config
	proxyRepo ProxyRepository
	groupRepo ProxyGroupRepository
	prober    ProxyExitInfoProber
	health    ProxyHealthCache
	latency   ProxyLatencyCache
	resolver  ProxyGroupResolver
	log       *slog.Logger
	now       func() time.Time
}

// NewProxyHealthService constructs the domain service (not started).
func NewProxyHealthService(
	cfg *config.Config,
	proxyRepo ProxyRepository,
	groupRepo ProxyGroupRepository,
	prober ProxyExitInfoProber,
	health ProxyHealthCache,
	latency ProxyLatencyCache,
	resolver ProxyGroupResolver,
) *ProxyHealthService {
	return &ProxyHealthService{
		cfg:       cfg,
		proxyRepo: proxyRepo,
		groupRepo: groupRepo,
		prober:    prober,
		health:    health,
		latency:   latency,
		resolver:  resolver,
		log:       slog.Default().With("component", "proxy_health"),
		now:       time.Now,
	}
}

func (s *ProxyHealthService) conf() config.ProxyHealthConfig {
	if s == nil || s.cfg == nil {
		return config.ProxyHealthConfig{
			IntervalSec:      60,
			TimeoutMS:        10000,
			Concurrency:      8,
			FailThreshold:    3,
			SuccessThreshold: 2,
			ProbeScope:       "group_members",
			AutoRecover:      true,
			SkipNamePrefix:   []string{"warp-"},
			BatchSize:        100,
		}
	}
	return s.cfg.ProxyHealth
}

// RunOnce selects candidates, probes concurrently, and applies isolate/recover.
func (s *ProxyHealthService) RunOnce(ctx context.Context) (*ProxyHealthRunResult, error) {
	if s == nil || s.proxyRepo == nil || s.prober == nil {
		return &ProxyHealthRunResult{}, nil
	}
	cfg := s.conf()
	candidates, err := s.listCandidates(ctx)
	if err != nil {
		return nil, err
	}
	result := &ProxyHealthRunResult{}
	if len(candidates) == 0 {
		return result, nil
	}
	if cfg.BatchSize > 0 && len(candidates) > cfg.BatchSize {
		candidates = candidates[:cfg.BatchSize]
	}

	type job struct {
		proxy Proxy
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 8
	}
	if concurrency > len(candidates) {
		concurrency = len(candidates)
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if s.shouldSkip(j.proxy) {
				mu.Lock()
				result.Skipped++
				mu.Unlock()
				continue
			}
			isolated, recovered, err := s.probeAndEvaluate(ctx, j.proxy)
			mu.Lock()
			result.Probed++
			if err != nil {
				result.Errors++
			}
			if isolated {
				result.Isolated++
			}
			if recovered {
				result.Recovered++
			}
			mu.Unlock()
		}
	}

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go worker()
	}
	for _, p := range candidates {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return result, ctx.Err()
		case jobs <- job{proxy: p}:
		}
	}
	close(jobs)
	wg.Wait()
	return result, nil
}

func (s *ProxyHealthService) shouldSkip(p Proxy) bool {
	cfg := s.conf()
	name := p.Name
	for _, prefix := range cfg.SkipNamePrefix {
		if prefix != "" && strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func (s *ProxyHealthService) listCandidates(ctx context.Context) ([]Proxy, error) {
	cfg := s.conf()
	switch cfg.ProbeScope {
	case "all_active":
		return s.listAllActiveCandidates(ctx)
	default:
		return s.listGroupMemberCandidates(ctx)
	}
}

func (s *ProxyHealthService) listAllActiveCandidates(ctx context.Context) ([]Proxy, error) {
	active, err := s.proxyRepo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Proxy, 0, len(active))
	for _, p := range active {
		if !s.shouldSkip(p) {
			out = append(out, p)
		}
	}
	if !s.conf().AutoRecover || s.health == nil {
		return out, nil
	}
	// Recovery path for all_active is limited: we only re-probe inactive
	// proxies that already carry isolated_by=health via group membership
	// listing is not available; scan is skipped for inactive outside groups.
	// Operators who need recover on non-group proxies can switch to group_members
	// or re-enable manually. (Phase 2 may add ListByStatus.)
	return out, nil
}

func (s *ProxyHealthService) listGroupMemberCandidates(ctx context.Context) ([]Proxy, error) {
	if s.groupRepo == nil {
		return s.listAllActiveCandidates(ctx)
	}
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{})
	out := make([]Proxy, 0)
	cfg := s.conf()
	for _, g := range groups {
		members, err := s.proxyRepo.ListByGroupID(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		for _, p := range members {
			if _, ok := seen[p.ID]; ok {
				continue
			}
			if s.shouldSkip(p) {
				continue
			}
			switch p.Status {
			case StatusActive:
				seen[p.ID] = struct{}{}
				out = append(out, p)
			case StatusInactive:
				if !cfg.AutoRecover {
					continue
				}
				// Only recover health-isolated; check meta cheaply.
				if s.health == nil {
					continue
				}
				meta, err := s.health.GetProxyHealth(ctx, p.ID)
				if err != nil || meta == nil || meta.IsolatedBy != ProxyHealthIsolatedByHealth {
					continue
				}
				seen[p.ID] = struct{}{}
				out = append(out, p)
			}
		}
	}
	return out, nil
}

func (s *ProxyHealthService) probeAndEvaluate(ctx context.Context, proxy Proxy) (isolated, recovered bool, err error) {
	cfg := s.conf()
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	exitInfo, latencyMs, probeErr := s.prober.ProbeProxy(probeCtx, proxy.URL())
	now := s.now()
	meta := s.loadMeta(ctx, proxy.ID)

	if probeErr != nil {
		meta.FailCount++
		meta.SuccessCount = 0
		meta.LastCheckedAt = now.Unix()
		meta.LastError = truncateErr(probeErr.Error(), 256)
		s.saveMeta(ctx, proxy.ID, meta)
		s.writeLatencyFail(ctx, proxy.ID, meta.LastError, now)
		if proxy.Status == StatusActive && meta.FailCount >= cfg.FailThreshold {
			if isoErr := s.isolate(ctx, proxy, meta, now); isoErr != nil {
				return false, false, isoErr
			}
			return true, false, nil
		}
		return false, false, nil
	}

	meta.SuccessCount++
	meta.FailCount = 0
	meta.LastCheckedAt = now.Unix()
	meta.LastOKAt = now.Unix()
	meta.LastError = ""
	meta.LatencyMs = latencyMs
	if exitInfo != nil {
		meta.ExitIP = exitInfo.IP
	}
	s.saveMeta(ctx, proxy.ID, meta)
	s.writeLatencyOK(ctx, proxy.ID, exitInfo, latencyMs, now)

	if cfg.AutoRecover &&
		proxy.Status == StatusInactive &&
		meta.IsolatedBy == ProxyHealthIsolatedByHealth &&
		meta.SuccessCount >= cfg.SuccessThreshold {
		if recErr := s.recover(ctx, proxy, meta, now); recErr != nil {
			return false, false, recErr
		}
		return false, true, nil
	}
	return false, false, nil
}

func (s *ProxyHealthService) isolate(ctx context.Context, proxy Proxy, meta *ProxyHealthMeta, now time.Time) error {
	proxy.Status = StatusInactive
	if err := s.proxyRepo.Update(ctx, &proxy); err != nil {
		return fmt.Errorf("isolate proxy %d: %w", proxy.ID, err)
	}
	meta.IsolatedBy = ProxyHealthIsolatedByHealth
	meta.IsolatedAt = now.Unix()
	s.saveMeta(ctx, proxy.ID, meta)
	s.invalidateGroup(proxy.GroupID)
	s.log.Info("proxy health isolated",
		"proxy_id", proxy.ID,
		"name", proxy.Name,
		"fail_count", meta.FailCount,
	)
	return nil
}

func (s *ProxyHealthService) recover(ctx context.Context, proxy Proxy, meta *ProxyHealthMeta, now time.Time) error {
	proxy.Status = StatusActive
	if err := s.proxyRepo.Update(ctx, &proxy); err != nil {
		return fmt.Errorf("recover proxy %d: %w", proxy.ID, err)
	}
	meta.IsolatedBy = ""
	meta.IsolatedAt = 0
	meta.FailCount = 0
	s.saveMeta(ctx, proxy.ID, meta)
	s.invalidateGroup(proxy.GroupID)
	s.log.Info("proxy health recovered",
		"proxy_id", proxy.ID,
		"name", proxy.Name,
		"success_count", meta.SuccessCount,
		"at", now.Unix(),
	)
	return nil
}

func (s *ProxyHealthService) invalidateGroup(groupID *int64) {
	if s.resolver == nil || groupID == nil || *groupID <= 0 {
		return
	}
	s.resolver.InvalidateGroup(*groupID)
}

func (s *ProxyHealthService) loadMeta(ctx context.Context, proxyID int64) *ProxyHealthMeta {
	if s.health != nil {
		if meta, err := s.health.GetProxyHealth(ctx, proxyID); err == nil && meta != nil {
			return meta
		}
	}
	return &ProxyHealthMeta{}
}

func (s *ProxyHealthService) saveMeta(ctx context.Context, proxyID int64, meta *ProxyHealthMeta) {
	if s.health == nil || meta == nil {
		return
	}
	if err := s.health.SetProxyHealth(ctx, proxyID, meta); err != nil {
		s.log.Warn("proxy health meta save failed", "proxy_id", proxyID, "err", err)
	}
}

func (s *ProxyHealthService) writeLatencyFail(ctx context.Context, proxyID int64, message string, now time.Time) {
	if s.latency == nil {
		return
	}
	_ = s.latency.SetProxyLatency(ctx, proxyID, &ProxyLatencyInfo{
		Success:   false,
		Message:   message,
		UpdatedAt: now,
	})
}

func (s *ProxyHealthService) writeLatencyOK(ctx context.Context, proxyID int64, exit *ProxyExitInfo, latencyMs int64, now time.Time) {
	if s.latency == nil {
		return
	}
	lat := latencyMs
	info := &ProxyLatencyInfo{
		Success:   true,
		LatencyMs: &lat,
		Message:   "Proxy is accessible",
		UpdatedAt: now,
	}
	if exit != nil {
		info.IPAddress = exit.IP
		info.Country = exit.Country
		info.CountryCode = exit.CountryCode
		info.Region = exit.Region
		info.City = exit.City
	}
	_ = s.latency.SetProxyLatency(ctx, proxyID, info)
}

func truncateErr(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

// ApplyProbeResult is a pure helper for unit tests: update meta + decide action.
// status is current proxy status; returns next status and updated meta flags.
func ApplyProbeResult(
	status string,
	meta ProxyHealthMeta,
	ok bool,
	failThreshold, successThreshold int,
	autoRecover bool,
	nowUnix int64,
) (nextStatus string, nextMeta ProxyHealthMeta, isolated, recovered bool) {
	nextMeta = meta
	nextStatus = status
	if failThreshold <= 0 {
		failThreshold = 3
	}
	if successThreshold <= 0 {
		successThreshold = 2
	}
	if !ok {
		nextMeta.FailCount++
		nextMeta.SuccessCount = 0
		nextMeta.LastCheckedAt = nowUnix
		if status == StatusActive && nextMeta.FailCount >= failThreshold {
			nextStatus = StatusInactive
			nextMeta.IsolatedBy = ProxyHealthIsolatedByHealth
			nextMeta.IsolatedAt = nowUnix
			isolated = true
		}
		return nextStatus, nextMeta, isolated, recovered
	}
	nextMeta.SuccessCount++
	nextMeta.FailCount = 0
	nextMeta.LastCheckedAt = nowUnix
	nextMeta.LastOKAt = nowUnix
	nextMeta.LastError = ""
	if autoRecover &&
		status == StatusInactive &&
		nextMeta.IsolatedBy == ProxyHealthIsolatedByHealth &&
		nextMeta.SuccessCount >= successThreshold {
		nextStatus = StatusActive
		nextMeta.IsolatedBy = ""
		nextMeta.IsolatedAt = 0
		nextMeta.FailCount = 0
		recovered = true
	}
	return nextStatus, nextMeta, isolated, recovered
}

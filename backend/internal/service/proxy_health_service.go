package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

// ProxyHealthRunResult summarizes one RunOnce tick.
type ProxyHealthRunResult struct {
	Probed    int `json:"probed"`
	Isolated  int `json:"isolated"`
	Recovered int `json:"recovered"`
	Skipped   int `json:"skipped"`
	Errors    int `json:"errors"`
}

// ProxyHealthDetail is the admin health detail payload.
type ProxyHealthDetail struct {
	ProxyID       int64  `json:"proxy_id"`
	Status        string `json:"status"`
	FailCount     int    `json:"fail_count"`
	SuccessCount  int    `json:"success_count"`
	LastCheckedAt int64  `json:"last_checked_at,omitempty"`
	LastOKAt      int64  `json:"last_ok_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	LatencyMs     int64  `json:"latency_ms,omitempty"`
	ExitIP        string `json:"exit_ip,omitempty"`
	IsolatedBy    string `json:"isolated_by,omitempty"`
	IsolatedAt    int64  `json:"isolated_at,omitempty"`
	// DB audit mirror
	DBFailCount      int    `json:"db_fail_count"`
	DBIsolatedBy     string `json:"db_isolated_by,omitempty"`
	DBLastHealthAt   *int64 `json:"db_last_health_at,omitempty"`
	FailThreshold    int    `json:"fail_threshold"`
	SuccessThreshold int    `json:"success_threshold"`
	ProbeMode        string `json:"probe_mode"`
	AutoRecover      bool   `json:"auto_recover"`
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
	metrics   *ProxyHealthMetrics
	log       *slog.Logger
	now       func() time.Time

	// group threshold cache for current RunOnce
	groupFailTh map[int64]int
	groupSuccTh map[int64]int

	// runtime override (DB settings)
	runtimeMu sync.RWMutex
	runtime   *ProxyHealthSettings

	// worker reference (for Apply after update)
	workerMu sync.Mutex
	worker   *ProxyHealthWorker

	// optional settings store (for DB persistence)
	settingRepo SettingRepository

	// yamlBaselineEnabled is the process YAML value before panel overrides.
	yamlBaselineEnabled bool

	// batchCursor rotates which slice of candidates is probed when BatchSize caps a large pool.
	batchMu     sync.Mutex
	batchCursor int
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
	metrics *ProxyHealthMetrics,
	settingRepo SettingRepository,
) *ProxyHealthService {
	s := &ProxyHealthService{
		cfg:       cfg,
		proxyRepo: proxyRepo,
		groupRepo: groupRepo,
		prober:    prober,
		health:    health,
		latency:   latency,
		resolver:  resolver,
		metrics:   metrics,
		log:       slog.Default().With("component", "proxy_health"),
		now:       time.Now,
		settingRepo: settingRepo,
	}
	if cfg != nil {
		s.yamlBaselineEnabled = cfg.ProxyHealth.Enabled
	}
	s.bootstrapRuntimeSettings()
	return s
}

func (s *ProxyHealthService) conf() config.ProxyHealthConfig {
	if s == nil {
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
			ProbeMode:        "connectivity",
		}
	}
	// Prefer applied runtime settings (DB/panel), fall back to YAML.
	s.runtimeMu.RLock()
	if s.runtime != nil {
		cfg := s.runtime.toConfig()
		s.runtimeMu.RUnlock()
		return cfg
	}
	s.runtimeMu.RUnlock()
	if s.cfg != nil {
		return s.cfg.ProxyHealth
	}
	return DefaultProxyHealthSettingsFromYAML(nil).toConfig()
}

// Metrics returns the process-local metrics holder (may be nil).
func (s *ProxyHealthService) Metrics() *ProxyHealthMetrics {
	if s == nil {
		return nil
	}
	return s.metrics
}

// GetHealth returns Redis meta + DB audit for one proxy.
func (s *ProxyHealthService) GetHealth(ctx context.Context, proxyID int64) (*ProxyHealthDetail, error) {
	if s == nil || s.proxyRepo == nil {
		return nil, fmt.Errorf("proxy health service not configured")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	cfg := s.conf()
	failTh, succTh := cfg.FailThreshold, cfg.SuccessThreshold
	if proxy.GroupID != nil && s.groupRepo != nil {
		if g, gerr := s.groupRepo.GetByID(ctx, *proxy.GroupID); gerr == nil && g != nil {
			failTh, succTh = s.thresholdsForGroup(g, cfg)
		}
	}
	meta := s.loadMeta(ctx, proxyID)
	detail := &ProxyHealthDetail{
		ProxyID:          proxy.ID,
		Status:           proxy.Status,
		FailCount:        meta.FailCount,
		SuccessCount:     meta.SuccessCount,
		LastCheckedAt:    meta.LastCheckedAt,
		LastOKAt:         meta.LastOKAt,
		LastError:        meta.LastError,
		LatencyMs:        meta.LatencyMs,
		ExitIP:           meta.ExitIP,
		IsolatedBy:       meta.IsolatedBy,
		IsolatedAt:       meta.IsolatedAt,
		FailThreshold:    failTh,
		SuccessThreshold: succTh,
		ProbeMode:        cfg.ProbeMode,
		AutoRecover:      cfg.AutoRecover,
	}
	if fc, lha, iso, aerr := s.proxyRepo.GetHealthAudit(ctx, proxyID); aerr == nil {
		detail.DBFailCount = fc
		detail.DBIsolatedBy = iso
		if lha != nil {
			u := lha.Unix()
			detail.DBLastHealthAt = &u
		}
	}
	return detail, nil
}

// RunScan is the admin-facing alias for a single full probe round.
func (s *ProxyHealthService) RunScan(ctx context.Context) (*ProxyHealthRunResult, error) {
	return s.RunOnce(ctx)
}

// RunOnce selects candidates, probes concurrently, and applies isolate/recover.
func (s *ProxyHealthService) RunOnce(ctx context.Context) (*ProxyHealthRunResult, error) {
	if s == nil || s.proxyRepo == nil || s.prober == nil {
		return &ProxyHealthRunResult{}, nil
	}
	cfg := s.conf()
	s.loadGroupThresholdIndex(ctx)
	candidates, err := s.listCandidates(ctx)
	if err != nil {
		return nil, err
	}
	result := &ProxyHealthRunResult{}
	if len(candidates) == 0 {
		s.metrics.recordRun(result, s.now().Unix())
		return result, nil
	}
	if cfg.BatchSize > 0 && len(candidates) > cfg.BatchSize {
		candidates = s.takeBatchWindow(candidates, cfg.BatchSize)
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
			s.metrics.recordRun(result, s.now().Unix())
			return result, ctx.Err()
		case jobs <- job{proxy: p}:
		}
	}
	close(jobs)
	wg.Wait()
	s.metrics.recordRun(result, s.now().Unix())
	return result, nil
}

func (s *ProxyHealthService) loadGroupThresholdIndex(ctx context.Context) {
	s.groupFailTh = map[int64]int{}
	s.groupSuccTh = map[int64]int{}
	if s.groupRepo == nil {
		return
	}
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return
	}
	for _, g := range groups {
		if g.HealthFailThreshold != nil && *g.HealthFailThreshold > 0 {
			s.groupFailTh[g.ID] = *g.HealthFailThreshold
		}
		if g.HealthSuccessThreshold != nil && *g.HealthSuccessThreshold > 0 {
			s.groupSuccTh[g.ID] = *g.HealthSuccessThreshold
		}
	}
}

func (s *ProxyHealthService) thresholdsForProxy(p Proxy, cfg config.ProxyHealthConfig) (failTh, succTh int) {
	failTh, succTh = cfg.FailThreshold, cfg.SuccessThreshold
	if failTh <= 0 {
		failTh = 3
	}
	if succTh <= 0 {
		succTh = 2
	}
	if p.GroupID == nil {
		return failTh, succTh
	}
	if v, ok := s.groupFailTh[*p.GroupID]; ok && v > 0 {
		failTh = v
	}
	if v, ok := s.groupSuccTh[*p.GroupID]; ok && v > 0 {
		succTh = v
	}
	return failTh, succTh
}

func (s *ProxyHealthService) thresholdsForGroup(g *ProxyGroup, cfg config.ProxyHealthConfig) (failTh, succTh int) {
	failTh, succTh = cfg.FailThreshold, cfg.SuccessThreshold
	if failTh <= 0 {
		failTh = 3
	}
	if succTh <= 0 {
		succTh = 2
	}
	if g == nil {
		return failTh, succTh
	}
	if g.HealthFailThreshold != nil && *g.HealthFailThreshold > 0 {
		failTh = *g.HealthFailThreshold
	}
	if g.HealthSuccessThreshold != nil && *g.HealthSuccessThreshold > 0 {
		succTh = *g.HealthSuccessThreshold
	}
	return failTh, succTh
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

// takeBatchWindow returns the next BatchSize candidates using a rotating cursor
// so large pools eventually probe every member instead of always the first N.
func (s *ProxyHealthService) takeBatchWindow(candidates []Proxy, batchSize int) []Proxy {
	if batchSize <= 0 || len(candidates) <= batchSize {
		return candidates
	}
	s.batchMu.Lock()
	start := s.batchCursor % len(candidates)
	s.batchCursor = (start + batchSize) % len(candidates)
	s.batchMu.Unlock()

	out := make([]Proxy, 0, batchSize)
	for i := 0; i < batchSize; i++ {
		out = append(out, candidates[(start+i)%len(candidates)])
	}
	return out
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
				if s.health == nil {
					continue
				}
				meta, err := s.health.GetProxyHealth(ctx, p.ID)
				if err != nil || meta == nil || meta.IsolatedBy != ProxyHealthIsolatedByHealth {
					// Fall back to DB audit mark.
					if _, _, iso, aerr := s.proxyRepo.GetHealthAudit(ctx, p.ID); aerr != nil || iso != ProxyHealthIsolatedByHealth {
						continue
					}
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
	failTh, succTh := s.thresholdsForProxy(proxy, cfg)
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	// Quality mode needs more headroom for multi-target HTTP.
	if cfg.ProbeMode == "quality" && timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	exitInfo, latencyMs, probeErr := s.prober.ProbeProxy(probeCtx, proxy.URL())
	now := s.now()
	meta := s.loadMeta(ctx, proxy.ID)

	if probeErr == nil && cfg.ProbeMode == "quality" {
		if qerr := s.probeQuality(probeCtx, proxy.URL()); qerr != nil {
			probeErr = fmt.Errorf("quality: %w", qerr)
		}
	}

	if probeErr != nil {
		meta.FailCount++
		meta.SuccessCount = 0
		meta.LastCheckedAt = now.Unix()
		meta.LastError = truncateErr(probeErr.Error(), 256)
		s.saveMeta(ctx, proxy.ID, meta)
		s.persistAudit(ctx, proxy.ID, meta, now)
		s.writeLatencyFail(ctx, proxy.ID, meta.LastError, now)
		if proxy.Status == StatusActive && meta.FailCount >= failTh {
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
	s.persistAudit(ctx, proxy.ID, meta, now)
	s.writeLatencyOK(ctx, proxy.ID, exitInfo, latencyMs, now)

	if cfg.AutoRecover &&
		proxy.Status == StatusInactive &&
		meta.IsolatedBy == ProxyHealthIsolatedByHealth &&
		meta.SuccessCount >= succTh {
		if recErr := s.recover(ctx, proxy, meta, now); recErr != nil {
			return false, false, recErr
		}
		return false, true, nil
	}
	return false, false, nil
}

// probeQuality runs AI-target quality checks (shared with admin quality check).
// Any hard fail (status=fail) counts as unhealthy; warn/challenge do not isolate.
func (s *ProxyHealthService) probeQuality(ctx context.Context, proxyURL string) error {
	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:              proxyURL,
		Timeout:               proxyQualityRequestTimeout,
		ResponseHeaderTimeout: proxyQualityResponseHeaderTimeout,
	})
	if err != nil {
		return err
	}
	// Use the first quality target as a lightweight gate (full multi-target is expensive in poller).
	if len(proxyQualityTargets) == 0 {
		return nil
	}
	item := runProxyQualityTarget(ctx, client, proxyQualityTargets[0])
	if item.Status == "fail" {
		return fmt.Errorf("%s: %s", item.Target, item.Message)
	}
	return nil
}

func (s *ProxyHealthService) isolate(ctx context.Context, proxy Proxy, meta *ProxyHealthMeta, now time.Time) error {
	proxy.Status = StatusInactive
	if err := s.proxyRepo.Update(ctx, &proxy); err != nil {
		return fmt.Errorf("isolate proxy %d: %w", proxy.ID, err)
	}
	meta.IsolatedBy = ProxyHealthIsolatedByHealth
	meta.IsolatedAt = now.Unix()
	s.saveMeta(ctx, proxy.ID, meta)
	s.persistAudit(ctx, proxy.ID, meta, now)
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
	s.persistAudit(ctx, proxy.ID, meta, now)
	s.invalidateGroup(proxy.GroupID)
	s.log.Info("proxy health recovered",
		"proxy_id", proxy.ID,
		"name", proxy.Name,
		"success_count", meta.SuccessCount,
		"at", now.Unix(),
	)
	return nil
}

func (s *ProxyHealthService) persistAudit(ctx context.Context, proxyID int64, meta *ProxyHealthMeta, now time.Time) {
	if s.proxyRepo == nil || meta == nil {
		return
	}
	t := now
	if err := s.proxyRepo.UpdateHealthAudit(ctx, proxyID, meta.FailCount, &t, meta.IsolatedBy); err != nil {
		// Missing migration should not break the poller.
		s.log.Debug("proxy health audit persist skipped/failed", "proxy_id", proxyID, "err", err)
	}
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
	// Seed from DB audit when Redis empty.
	meta := &ProxyHealthMeta{}
	if s.proxyRepo != nil {
		if fc, lha, iso, err := s.proxyRepo.GetHealthAudit(ctx, proxyID); err == nil {
			meta.FailCount = fc
			meta.IsolatedBy = iso
			if lha != nil {
				meta.LastCheckedAt = lha.Unix()
			}
		}
	}
	return meta
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
	info := &ProxyLatencyInfo{
		Success:   false,
		Message:   message,
		UpdatedAt: now,
	}
	s.mergeAndSaveLatency(ctx, proxyID, info)
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
	s.mergeAndSaveLatency(ctx, proxyID, info)
}

// mergeAndSaveLatency preserves quality_* snapshot fields when the health
// poller only updates connectivity latency (mirrors admin saveProxyLatency).
func (s *ProxyHealthService) mergeAndSaveLatency(ctx context.Context, proxyID int64, info *ProxyLatencyInfo) {
	if s.latency == nil || info == nil {
		return
	}
	merged := *info
	if latencies, err := s.latency.GetProxyLatencies(ctx, []int64{proxyID}); err == nil {
		if existing := latencies[proxyID]; existing != nil {
			if merged.QualityCheckedAt == nil &&
				merged.QualityScore == nil &&
				merged.QualityGrade == "" &&
				merged.QualityStatus == "" &&
				merged.QualitySummary == "" &&
				merged.QualityCFRay == "" {
				merged.QualityStatus = existing.QualityStatus
				merged.QualityScore = existing.QualityScore
				merged.QualityGrade = existing.QualityGrade
				merged.QualitySummary = existing.QualitySummary
				merged.QualityCheckedAt = existing.QualityCheckedAt
				merged.QualityCFRay = existing.QualityCFRay
			}
		}
	}
	if err := s.latency.SetProxyLatency(ctx, proxyID, &merged); err != nil {
		s.log.Warn("proxy health latency save failed", "proxy_id", proxyID, "err", err)
	}
}

func truncateErr(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

// ApplyProbeResult is a pure helper for unit tests: update meta + decide action.
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

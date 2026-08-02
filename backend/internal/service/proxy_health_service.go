package service

import (
	"context"
	"database/sql"
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

	// isolatedRecoverCursor rotates AutoRecover probes over health-isolated rows
	// under probe_scope=all_active (id > cursor ORDER BY id ASC, wrap on empty).
	isolatedRecoverCursor int64

	// Cross-instance + process singleflight for RunOnce / admin RunScan.
	lock       LeaderLockCache
	instanceID string
	db         *sql.DB
	runMu      sync.Mutex
	runActive  bool
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
		cfg:         cfg,
		proxyRepo:   proxyRepo,
		groupRepo:   groupRepo,
		prober:      prober,
		health:      health,
		latency:     latency,
		resolver:    resolver,
		metrics:     metrics,
		log:         slog.Default().With("component", "proxy_health"),
		now:         time.Now,
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
	meta, _ := s.loadMeta(ctx, proxyID)
	if meta == nil {
		meta = &ProxyHealthMeta{}
	}
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

// SetLeaderLock wires the shared Redis leader lock (and optional DB advisory
// fallback) used by both the background worker and admin RunScan so only one
// probe round runs cluster-wide.
func (s *ProxyHealthService) SetLeaderLock(lock LeaderLockCache, instanceID string, db *sql.DB) {
	if s == nil {
		return
	}
	s.lock = lock
	if instanceID != "" {
		s.instanceID = instanceID
	}
	s.db = db
}

// RunScan is the admin-facing alias for a single full probe round.
func (s *ProxyHealthService) RunScan(ctx context.Context) (*ProxyHealthRunResult, error) {
	return s.RunOnce(ctx)
}

// RunOnce selects candidates, probes concurrently, and applies isolate/recover.
// Process-local singleflight + shared leader lock gate both the worker tick and
// admin RunScan so concurrent rounds cannot thrash health meta / isolate state.
func (s *ProxyHealthService) RunOnce(ctx context.Context) (*ProxyHealthRunResult, error) {
	if s == nil || s.proxyRepo == nil || s.prober == nil {
		return &ProxyHealthRunResult{}, nil
	}

	// Process-local singleflight: admin Scan + worker tick must not overlap.
	s.runMu.Lock()
	if s.runActive {
		s.runMu.Unlock()
		return nil, ErrProxyHealthScanBusy
	}
	s.runActive = true
	s.runMu.Unlock()
	defer func() {
		s.runMu.Lock()
		s.runActive = false
		s.runMu.Unlock()
	}()

	owner := s.instanceID
	if owner == "" {
		owner = "proxy-health-scan"
	}
	// Short dedicated lock ctx so a long parent timeout does not block acquire,
	// and a dead parent does not force ungated fall-through via cache errors.
	ttl := s.scanLockTTL()
	lockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	release, ok := tryAcquireSingletonLeaderLock(lockCtx, s.lock, s.db, proxyHealthWorkerLockKey, owner, ttl)
	cancel()
	if !ok {
		// Peer instance already running a scan, or lock backend timed out / Redis error.
		// Both map to Busy for admin 409; worker logs Debug. Redis-down = cluster-wide skip
		// (no DB fallthrough — see tryAcquireSingletonLeaderLock).
		s.log.Debug("proxy health run skipped: leader lock not acquired")
		return nil, ErrProxyHealthScanBusy
	}
	if release != nil {
		defer release()
	}
	// Heartbeat ctx cancels when leadership is lost mid-scan so workers abort.
	hbCtx, stopHB := startLeaderLockHeartbeat(ctx, s.lock, proxyHealthWorkerLockKey, owner, ttl)
	defer stopHB()

	return s.runOnceLocked(hbCtx)
}

func (s *ProxyHealthService) scanLockTTL() time.Duration {
	cfg := s.conf()
	sec := 50
	if cfg.LeaderLockTTLSec > 0 {
		sec = cfg.LeaderLockTTLSec
	}
	ttl := time.Duration(sec) * time.Second
	// Keep lock longer than one interval so multi-instance ticks do not pile up.
	interval := cfg.IntervalSec
	if interval <= 0 {
		interval = 60
	}
	minTTL := time.Duration(interval) * time.Second
	if ttl < minTTL {
		ttl = minTTL
	}
	// Also cover worst-case probe wall time.
	batch := cfg.BatchSize
	if batch <= 0 {
		batch = 100
	}
	conc := cfg.Concurrency
	if conc <= 0 {
		conc = 8
	}
	perProxyMS := cfg.TimeoutMS
	if perProxyMS <= 0 {
		perProxyMS = 10000
	}
	if cfg.ProbeMode == "quality" && perProxyMS < 30000 {
		perProxyMS = 30000
	}
	waves := (batch + conc - 1) / conc
	est := time.Duration(waves*perProxyMS)*time.Millisecond + 30*time.Second
	if est > ttl {
		ttl = est
	}
	if ttl > 15*time.Minute {
		ttl = 15 * time.Minute
	}
	return ttl
}

// runOnceLocked performs the actual probe round; caller holds process + leader locks.
func (s *ProxyHealthService) runOnceLocked(ctx context.Context) (*ProxyHealthRunResult, error) {
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
	seen := make(map[int64]struct{}, len(active))
	out := make([]Proxy, 0, len(active))
	for _, p := range active {
		if s.shouldSkip(p) {
			continue
		}
		seen[p.ID] = struct{}{}
		out = append(out, p)
	}

	// AutoRecover needs health-isolated inactive rows; ListActive alone never
	// re-probes them so they stay down forever under probe_scope=all_active.
	cfg := s.conf()
	if !cfg.AutoRecover {
		return out, nil
	}
	isolated, err := s.listHealthIsolatedForRecover(ctx, 100)
	if err != nil {
		s.log.Debug("list health-isolated for all_active recover failed", "err", err)
		return out, nil
	}
	for _, partial := range isolated {
		if _, ok := seen[partial.ID]; ok {
			continue
		}
		if s.shouldSkip(partial) {
			continue
		}
		// List returns a slim projection; load full row for URL/auth.
		full, gerr := s.proxyRepo.GetByID(ctx, partial.ID)
		if gerr != nil || full == nil {
			continue
		}
		if full.Status != StatusInactive {
			continue
		}
		// Prefer Redis meta; fall back to DB audit mark already implied by list.
		if s.health != nil {
			if meta, merr := s.health.GetProxyHealth(ctx, full.ID); merr == nil && meta != nil &&
				meta.IsolatedBy != "" && meta.IsolatedBy != ProxyHealthIsolatedByHealth {
				continue
			}
		}
		seen[full.ID] = struct{}{}
		out = append(out, *full)
	}
	return out, nil
}

// listHealthIsolatedForRecover pages inactive health-isolated proxies by id ASC
// with a rotating cursor so large isolation sets are eventually fully probed.
func (s *ProxyHealthService) listHealthIsolatedForRecover(ctx context.Context, limit int) ([]Proxy, error) {
	if s == nil || s.proxyRepo == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	s.batchMu.Lock()
	cursor := s.isolatedRecoverCursor
	s.batchMu.Unlock()

	rows, err := s.proxyRepo.ListHealthIsolatedByID(ctx, cursor, limit)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 && cursor > 0 {
		// Wrap to the start of the id space.
		s.batchMu.Lock()
		s.isolatedRecoverCursor = 0
		s.batchMu.Unlock()
		rows, err = s.proxyRepo.ListHealthIsolatedByID(ctx, 0, limit)
		if err != nil {
			return nil, err
		}
	}
	if len(rows) > 0 {
		s.batchMu.Lock()
		s.isolatedRecoverCursor = rows[len(rows)-1].ID
		s.batchMu.Unlock()
	}
	return rows, nil
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
				if err != nil {
					// Redis error: skip rather than trust stale DB (consistent with loadMeta).
					continue
				}
				if meta != nil {
					// Redis hit: trust Redis only. Empty IsolatedBy means admin cleared —
					// do NOT fall back to DB health mark.
					if meta.IsolatedBy != ProxyHealthIsolatedByHealth {
						continue
					}
				} else {
					// True miss: seed eligibility from DB audit.
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

	if probeErr == nil && cfg.ProbeMode == "quality" {
		if qerr := s.probeQuality(probeCtx, proxy.URL()); qerr != nil {
			probeErr = fmt.Errorf("quality: %w", qerr)
		}
	}

	// CAS retry: concurrent admin/worker (or multi-instance lock miss) must not
	// clobber each other's fail/success counters via blind GET-MODIFY-SET.
	// On exhaustion we skip counter advance + isolate/recover for this round
	// (no force-write that would defeat CAS).
	const maxCASAttempts = 3
	var meta *ProxyHealthMeta
	saved := false
	for attempt := 0; attempt < maxCASAttempts; attempt++ {
		loaded, lerr := s.loadMeta(ctx, proxy.ID)
		if lerr != nil {
			// Redis read error: do not seed Version=0 and CAS (would fight real key).
			s.log.Warn("proxy health meta load failed; skip counter/isolate this round",
				"proxy_id", proxy.ID, "err", lerr)
			return false, false, nil
		}
		meta = loaded
		if meta == nil {
			meta = &ProxyHealthMeta{}
		}
		baseVersion := meta.Version
		if probeErr != nil {
			meta.FailCount++
			meta.SuccessCount = 0
			meta.LastCheckedAt = now.Unix()
			meta.LastError = truncateErr(probeErr.Error(), 256)
		} else {
			meta.SuccessCount++
			meta.FailCount = 0
			meta.LastCheckedAt = now.Unix()
			meta.LastOKAt = now.Unix()
			meta.LastError = ""
			meta.LatencyMs = latencyMs
			if exitInfo != nil {
				meta.ExitIP = exitInfo.IP
			}
		}
		if s.saveMetaCAS(ctx, proxy.ID, baseVersion, meta) {
			saved = true
			break
		}
	}
	if !saved {
		s.log.Warn("proxy health meta CAS exhausted; skip counter/isolate this round",
			"proxy_id", proxy.ID)
		return false, false, nil
	}

	s.persistAudit(ctx, proxy.ID, meta, now)
	if probeErr != nil {
		s.writeLatencyFail(ctx, proxy.ID, meta.LastError, now)
		if proxy.Status == StatusActive && meta.FailCount >= failTh {
			// Re-read status/meta after CAS so concurrent admin deactivate/clear wins.
			cur, curMeta, ok := s.recheckBeforeIsolate(ctx, proxy.ID, failTh)
			if !ok {
				return false, false, nil
			}
			if isoErr := s.isolate(ctx, cur, curMeta, now); isoErr != nil {
				return false, false, isoErr
			}
			return true, false, nil
		}
		return false, false, nil
	}

	s.writeLatencyOK(ctx, proxy.ID, exitInfo, latencyMs, now)
	if cfg.AutoRecover &&
		proxy.Status == StatusInactive &&
		meta.IsolatedBy == ProxyHealthIsolatedByHealth &&
		meta.SuccessCount >= succTh {
		// Re-read status/meta after CAS so concurrent admin clear/activate wins.
		cur, curMeta, ok := s.recheckBeforeRecover(ctx, proxy.ID, succTh)
		if !ok {
			return false, false, nil
		}
		if recErr := s.recover(ctx, cur, curMeta, now); recErr != nil {
			return false, false, recErr
		}
		return false, true, nil
	}
	return false, false, nil
}

// recheckBeforeIsolate re-fetches proxy + meta after counter CAS succeeds and
// before writing isolate. Returns ok=false when admin already deactivated the
// proxy or cleared fail counters below the threshold.
func (s *ProxyHealthService) recheckBeforeIsolate(ctx context.Context, proxyID int64, failTh int) (Proxy, *ProxyHealthMeta, bool) {
	if s.proxyRepo == nil {
		return Proxy{}, nil, false
	}
	p, err := s.proxyRepo.GetByID(ctx, proxyID)
	if err != nil || p == nil {
		return Proxy{}, nil, false
	}
	if p.Status != StatusActive {
		// Admin already deactivated (or another path isolated) — do not re-isolate.
		return *p, nil, false
	}
	meta, lerr := s.loadMeta(ctx, proxyID)
	if lerr != nil {
		return *p, nil, false
	}
	if meta == nil {
		meta = &ProxyHealthMeta{}
	}
	// Admin clear bumps Version and zeros FailCount — skip if below threshold.
	if meta.FailCount < failTh {
		return *p, meta, false
	}
	return *p, meta, true
}

// recheckBeforeRecover re-fetches proxy + meta after counter CAS succeeds and
// before writing recover. Returns ok=false when status is no longer inactive,
// IsolatedBy is no longer "health" (admin cleared mark), or success count
// dropped below threshold.
func (s *ProxyHealthService) recheckBeforeRecover(ctx context.Context, proxyID int64, succTh int) (Proxy, *ProxyHealthMeta, bool) {
	if s.proxyRepo == nil {
		return Proxy{}, nil, false
	}
	p, err := s.proxyRepo.GetByID(ctx, proxyID)
	if err != nil || p == nil {
		return Proxy{}, nil, false
	}
	if p.Status != StatusInactive {
		return *p, nil, false
	}
	meta, lerr := s.loadMeta(ctx, proxyID)
	if lerr != nil {
		return *p, nil, false
	}
	if meta == nil {
		meta = &ProxyHealthMeta{}
	}
	if meta.IsolatedBy != ProxyHealthIsolatedByHealth {
		// Admin cleared the health mark — do not auto-recover.
		return *p, meta, false
	}
	if meta.SuccessCount < succTh {
		return *p, meta, false
	}
	return *p, meta, true
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
	meta.IsolatedBy = ProxyHealthIsolatedByHealth
	meta.IsolatedAt = now.Unix()
	// Atomic status + health_isolated_by so a crash mid-path cannot leave
	// inactive without the audit mark (which would block AutoRecover forever).
	t := now
	if err := s.proxyRepo.UpdateStatusWithHealthIsolation(ctx, proxy.ID, StatusInactive, meta.FailCount, &t, ProxyHealthIsolatedByHealth); err != nil {
		// Fallback for older stubs / missing SQL columns: full Update + audit.
		proxy.Status = StatusInactive
		if uerr := s.proxyRepo.Update(ctx, &proxy); uerr != nil {
			return fmt.Errorf("isolate proxy %d: %w", proxy.ID, uerr)
		}
		s.persistAudit(ctx, proxy.ID, meta, now)
	} else {
		proxy.Status = StatusInactive
	}
	// Status change must stick in Redis: CAS with one reload retry, then force Set
	// only for isolate/recover path (counter path never force-writes).
	s.persistStatusMeta(ctx, proxy.ID, meta, func(m *ProxyHealthMeta) {
		m.IsolatedBy = ProxyHealthIsolatedByHealth
		m.IsolatedAt = now.Unix()
	})
	s.invalidateGroup(proxy.GroupID)
	s.log.Info("proxy health isolated",
		"proxy_id", proxy.ID,
		"name", proxy.Name,
		"fail_count", meta.FailCount,
	)
	return nil
}

func (s *ProxyHealthService) recover(ctx context.Context, proxy Proxy, meta *ProxyHealthMeta, now time.Time) error {
	meta.IsolatedBy = ""
	meta.IsolatedAt = 0
	meta.FailCount = 0
	t := now
	if err := s.proxyRepo.UpdateStatusWithHealthIsolation(ctx, proxy.ID, StatusActive, 0, &t, ""); err != nil {
		proxy.Status = StatusActive
		if uerr := s.proxyRepo.Update(ctx, &proxy); uerr != nil {
			return fmt.Errorf("recover proxy %d: %w", proxy.ID, uerr)
		}
		s.persistAudit(ctx, proxy.ID, meta, now)
	} else {
		proxy.Status = StatusActive
	}
	s.persistStatusMeta(ctx, proxy.ID, meta, func(m *ProxyHealthMeta) {
		m.IsolatedBy = ""
		m.IsolatedAt = 0
		m.FailCount = 0
	})
	s.invalidateGroup(proxy.GroupID)
	s.log.Info("proxy health recovered",
		"proxy_id", proxy.ID,
		"name", proxy.Name,
		"success_count", meta.SuccessCount,
		"at", now.Unix(),
	)
	return nil
}

// persistStatusMeta writes isolate/recover fields with CAS + one reload retry.
// If both CAS attempts fail, force SetProxyHealth so DB status and Redis mark
// stay aligned (only used on isolate/recover, never on counter updates).
func (s *ProxyHealthService) persistStatusMeta(ctx context.Context, proxyID int64, meta *ProxyHealthMeta, apply func(*ProxyHealthMeta)) {
	if meta == nil {
		return
	}
	baseVersion := meta.Version
	if s.saveMetaCAS(ctx, proxyID, baseVersion, meta) {
		return
	}
	// Reload and retry CAS once with re-applied status fields.
	// Redis read error: do not seed Version=0; force-write local meta so the
	// isolate/recover mark still lands when Redis recovers momentarily on SET.
	fresh, lerr := s.loadMeta(ctx, proxyID)
	if lerr != nil {
		s.log.Warn("proxy health meta reload failed during status persist; force local",
			"proxy_id", proxyID, "err", lerr)
		s.saveMeta(ctx, proxyID, meta)
		return
	}
	if fresh == nil {
		fresh = &ProxyHealthMeta{}
	}
	apply(fresh)
	// Preserve success/fail counters already decided this round when present.
	if meta.SuccessCount > fresh.SuccessCount {
		fresh.SuccessCount = meta.SuccessCount
	}
	if meta.FailCount > fresh.FailCount {
		fresh.FailCount = meta.FailCount
	}
	if meta.LastCheckedAt > fresh.LastCheckedAt {
		fresh.LastCheckedAt = meta.LastCheckedAt
		fresh.LastOKAt = meta.LastOKAt
		fresh.LastError = meta.LastError
		fresh.LatencyMs = meta.LatencyMs
		fresh.ExitIP = meta.ExitIP
	}
	baseVersion = fresh.Version
	*meta = *fresh
	if s.saveMetaCAS(ctx, proxyID, baseVersion, meta) {
		return
	}
	s.saveMeta(ctx, proxyID, meta)
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

// loadMeta returns Redis health meta, or seeds from DB audit on true miss.
// On Redis read error it returns (nil, err) — callers must skip CAS with
// Version=0 rather than treating errors as miss (aligned with leader-lock
// "Redis error → SKIP" philosophy).
func (s *ProxyHealthService) loadMeta(ctx context.Context, proxyID int64) (*ProxyHealthMeta, error) {
	// Redis hit: trust Redis fully (even FailCount==0 / IsolatedBy=="").
	// Do not overlay DB fields — that reintroduces stale counters after recover.
	if s.health != nil {
		m, err := s.health.GetProxyHealth(ctx, proxyID)
		if err != nil {
			return nil, err
		}
		if m != nil {
			return m, nil
		}
	}
	// Redis miss only: seed from DB audit so AutoRecover still works after a
	// Redis flush / key expiry.
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
	return meta, nil
}

func (s *ProxyHealthService) saveMeta(ctx context.Context, proxyID int64, meta *ProxyHealthMeta) {
	if s.health == nil || meta == nil {
		return
	}
	// Unconditional write path (force after failed CAS / seed). Still bump version.
	if meta.Version < 1 {
		meta.Version = 1
	} else {
		meta.Version++
	}
	if err := s.health.SetProxyHealth(ctx, proxyID, meta); err != nil {
		s.log.Warn("proxy health meta save failed", "proxy_id", proxyID, "err", err)
	}
}

// saveMetaCAS bumps Version and writes via CompareAndSet. expectedVersion is the
// Version observed before local mutation (0 when key was missing).
// On CAS error: return false without force Set so the counter path in
// probeAndEvaluate skips rather than clobbering peers. isolate/recover may still
// force via persistStatusMeta → saveMeta after CAS retries.
func (s *ProxyHealthService) saveMetaCAS(ctx context.Context, proxyID int64, expectedVersion int64, meta *ProxyHealthMeta) bool {
	if s.health == nil || meta == nil {
		return true
	}
	meta.Version = expectedVersion + 1
	ok, err := s.health.CompareAndSetProxyHealth(ctx, proxyID, expectedVersion, meta)
	if err != nil {
		s.log.Warn("proxy health meta CAS failed", "proxy_id", proxyID, "err", err)
		return false
	}
	return ok
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

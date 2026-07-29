package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	connectionRiskWorkerLockKey = "connection_risk_worker"
	connectionRiskWorkerJobName = "connection_risk_worker"
)

// ConnectionRiskWorker periodically scores active keys and writes risk events.
type ConnectionRiskWorker struct {
	cfg         *config.Config
	settings    *SettingService
	signals     ConnectionSignalCache
	events      ConnectionRiskEventRepository
	lock        LeaderLockCache
	concurrency ConcurrencyCache
	users       UserRepository
	apiKeys     *APIKeyService
	metrics     *ConnectionRiskMetrics
	policy      *RiskActionPolicy

	instanceID string
	stopCh     chan struct{}
	wg         sync.WaitGroup
	started    bool
	mu         sync.Mutex
}

// NewConnectionRiskWorker constructs the worker (call Start separately or via Provide).
func NewConnectionRiskWorker(
	cfg *config.Config,
	settings *SettingService,
	signals ConnectionSignalCache,
	events ConnectionRiskEventRepository,
	lock LeaderLockCache,
	concurrency ConcurrencyCache,
	users UserRepository,
	apiKeys *APIKeyService,
	metrics *ConnectionRiskMetrics,
	policy *RiskActionPolicy,
) *ConnectionRiskWorker {
	if metrics == nil {
		metrics = &ConnectionRiskMetrics{}
	}
	host, _ := os.Hostname()
	return &ConnectionRiskWorker{
		cfg:         cfg,
		settings:    settings,
		signals:     signals,
		events:      events,
		lock:        lock,
		concurrency: concurrency,
		users:       users,
		apiKeys:     apiKeys,
		metrics:     metrics,
		policy:      policy,
		instanceID:  fmt.Sprintf("%s-%d", host, os.Getpid()),
		stopCh:      make(chan struct{}),
	}
}

// ProvideConnectionRiskMetrics returns a process-wide metrics holder.
func ProvideConnectionRiskMetrics() *ConnectionRiskMetrics {
	return &ConnectionRiskMetrics{}
}

// ProvideConnectionRiskWorker constructs and starts the worker for wire DI.
func ProvideConnectionRiskWorker(
	cfg *config.Config,
	settings *SettingService,
	signals ConnectionSignalCache,
	events ConnectionRiskEventRepository,
	lock LeaderLockCache,
	concurrency ConcurrencyCache,
	users UserRepository,
	apiKeys *APIKeyService,
	metrics *ConnectionRiskMetrics,
	policy *RiskActionPolicy,
) *ConnectionRiskWorker {
	w := NewConnectionRiskWorker(cfg, settings, signals, events, lock, concurrency, users, apiKeys, metrics, policy)
	w.Start()
	return w
}

// Start begins the background loop (idempotent).
func (w *ConnectionRiskWorker) Start() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return
	}
	w.started = true
	w.wg.Add(1)
	go w.run()
}

// Stop halts the background loop.
func (w *ConnectionRiskWorker) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
	w.mu.Unlock()
	w.wg.Wait()
}

func (w *ConnectionRiskWorker) run() {
	defer w.wg.Done()
	// Initial delay so boot storms don't all evaluate at once.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case <-timer.C:
			w.evaluateOnce()
			timer.Reset(w.interval())
		}
	}
}

func (w *ConnectionRiskWorker) interval() time.Duration {
	sec := 120
	if w.settings != nil {
		s := w.settings.GetConnectionRiskSettingsCached(context.Background())
		if s.WorkerIntervalSeconds >= 60 && s.WorkerIntervalSeconds <= 300 {
			sec = s.WorkerIntervalSeconds
		}
	}
	return time.Duration(sec) * time.Second
}

func (w *ConnectionRiskWorker) evaluateOnce() {
	if w == nil {
		return
	}
	if w.cfg == nil || !w.cfg.ConnectionRisk.Enabled {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	s := w.settings.GetConnectionRiskSettingsCached(ctx)
	if !s.EffectiveWorkerEnabled(true) {
		// master already true; settings layers:
		if !s.Enabled || !s.WorkerEnabled {
			return
		}
	}

	// Always prune active sets when worker runs (even if no subjects) so emit can be safe.
	if w.signals != nil {
		_ = w.signals.PruneActive(ctx, s.MaxActiveMembers, 24*time.Hour)
	}

	ttl := 2 * w.interval()
	if ttl < 90*time.Second {
		ttl = 90 * time.Second
	}
	if w.lock != nil {
		ok, err := w.lock.TryAcquireLeaderLock(ctx, connectionRiskWorkerLockKey, w.instanceID, ttl)
		if err != nil {
			slog.Warn("connection risk worker lock error, skip tick", "error", err)
			return
		}
		if !ok {
			return
		}
		defer func() {
			_ = w.lock.ReleaseLeaderLock(context.Background(), connectionRiskWorkerLockKey, w.instanceID)
		}()
	}

	tick := w.metrics.WorkerTicks.Add(1)
	w.metrics.LastTickUnix.Store(time.Now().Unix())

	if w.signals == nil || w.events == nil {
		return
	}

	// Retention: every ~12 ticks (~24 min at default interval)
	if tick%12 == 0 && w.events != nil {
		days := s.RetentionDays
		if days <= 0 {
			days = 120
		}
		cutoff := time.Now().UTC().AddDate(0, 0, -days)
		if n, err := w.events.DeleteOlderThan(ctx, cutoff); err != nil {
			slog.Warn("connection risk retention failed", "error", err)
		} else if n > 0 {
			slog.Info("connection risk retention deleted events", "count", n, "cutoff", cutoff)
		}
	}

	keys, err := w.signals.ListActiveKeys(ctx, 2000)
	if err != nil {
		slog.Warn("connection risk list active keys failed", "error", err)
		return
	}

	now := time.Now().Unix()
	for _, keyID := range keys {
		select {
		case <-ctx.Done():
			return
		default:
		}
		w.scoreKey(ctx, keyID, now, s)
	}
}

func (w *ConnectionRiskWorker) scoreKey(ctx context.Context, keyID, nowUnix int64, s ConnectionRiskSettings) {
	w.metrics.SubjectsScan.Add(1)

	// We need userID — read metrics requires it. List from signals doesn't carry user;
	// ReadKeyWindowMetrics still needs userID for user-dimension rules. Look up via a
	// lightweight approach: pass 0 first for key-only metrics then fill user from exempt check.
	// Better: store user on active set is key-only; fetch user via optional side channel.
	// For MVP, use userID=0 for key-only rules and skip R4/R5/R7 user parts unless we can resolve.

	// Resolve user from recent usage is too heavy. Convention: active users ZSET is scored
	// separately for user-scoped rules; key scoring uses userID from exempt list miss.
	// Practical approach: read key metrics with userID=0, then if we have concurrency reader
	// we still need user. Store mapping cr:k:{id}:uid — not in design.
	//
	// Design lists active keys and reads per key with user_id from API key — we don't have
	// APIKey repo injected. Inject optional resolver:
	userID := w.lookupUserForKey(ctx, keyID)

	if userID > 0 {
		if containsInt64(s.ExemptUserIDs, userID) {
			return
		}
		if ok, _ := w.signals.IsExempt(ctx, "u", userID); ok {
			return
		}
	}
	if containsInt64(s.ExemptAPIKeyIDs, keyID) {
		return
	}
	if ok, _ := w.signals.IsExempt(ctx, "k", keyID); ok {
		return
	}

	metrics, err := w.signals.ReadKeyWindowMetrics(ctx, keyID, userID, nowUnix)
	if err != nil || metrics == nil {
		return
	}
	if userID > 0 && w.concurrency != nil {
		if n, err := w.concurrency.GetUserConcurrency(ctx, userID); err == nil {
			metrics.UserConcurrency = n
		}
		if w.users != nil {
			if u, err := w.users.GetByID(ctx, userID); err == nil && u != nil {
				metrics.UserConcurrencyLimit = u.Concurrency
				metrics.EffectiveRPMLimit = u.RPMLimit
			}
		}
	}

	// Phase B: daily baseline snapshot + R3 inputs
	w.maybeWriteBaseline(ctx, keyID, metrics.IPHll24h)
	if p95, days, ok, _ := w.signals.GetBaselineP95(ctx, keyID); ok {
		metrics.BaselineP95 = p95
		metrics.BaselineSampleDays = days
		metrics.BaselineFactor = 3
	}

	result := ScoreConnectionRisk(metrics, s)
	if !result.ShouldOpen {
		return
	}

	// Stable open-event dedupe key (rule family + 30m bucket). PG partial unique
	// + UpsertOpen refresh last_seen; Redis SETNX is best-effort cross-instance hint only.
	dedupe := fmt.Sprintf("k:%d:%s:%d", keyID, primaryRule(result), nowUnix/1800)
	_, _ = w.signals.TryDedupe(ctx, dedupe, 30*time.Minute)

	uid := userID
	kid := keyID
	prefix := metrics.APIKeyPrefix
	if prefix == "" && w.signals != nil {
		if p, err := w.signals.GetKeyPrefix(ctx, keyID); err == nil && p != "" {
			prefix = p
		}
	}
	if prefix == "" {
		prefix = w.lookupKeyPrefix(ctx, keyID)
	}
	ev := &ConnectionRiskEvent{
		SubjectType:  ConnectionRiskSubjectAPIKey,
		UserID:       nil,
		APIKeyID:     &kid,
		APIKeyPrefix: prefix,
		RulesFired:   result.RulesFired,
		Severity:     result.Severity,
		Score:        result.Score,
		Status:       ConnectionRiskStatusOpen,
		Title:        result.Title,
		Summary:      result.Summary,
		Evidence:     BuildConnectionRiskEvidence(metrics),
		Metrics:      BuildConnectionRiskEvidence(metrics),
		DedupeKey:    dedupe,
		ActionTaken:  ConnectionRiskActionNone,
		FirstSeenAt:  time.Now().UTC(),
		LastSeenAt:   time.Now().UTC(),
	}
	if uid > 0 {
		ev.UserID = &uid
	}

	saved, err := w.events.UpsertOpen(ctx, ev)
	if err != nil {
		slog.Warn("connection risk upsert failed", "error", err, "key_id", keyID)
		return
	}
	w.metrics.EventsCreated.Add(1)

	if w.policy != nil && saved != nil {
		w.policy.HandleNewEvent(ctx, saved, s)
	}
}

func (w *ConnectionRiskWorker) maybeWriteBaseline(ctx context.Context, keyID int64, hll24h int64) {
	if w == nil || w.signals == nil || keyID <= 0 {
		return
	}
	day := time.Now().UTC().Format("20060102")
	_ = w.signals.SnapshotBaselineDay(ctx, keyID, day, hll24h)
	// Recompute p95 from last 7 days when we have enough samples.
	days := make([]string, 0, 7)
	now := time.Now().UTC()
	for i := 0; i < 7; i++ {
		days = append(days, now.AddDate(0, 0, -i).Format("20060102"))
	}
	samples, err := w.signals.LoadBaselineSamples(ctx, keyID, days)
	if err != nil || len(samples) < 3 {
		return
	}
	p95 := percentileApprox(samples, 95)
	_ = w.signals.SetBaselineP95(ctx, keyID, p95, len(samples))
}

func percentileApprox(samples []int64, p int) float64 {
	if len(samples) == 0 {
		return 0
	}
	cp := append([]int64(nil), samples...)
	// insertion sort (N≤7)
	for i := 1; i < len(cp); i++ {
		for j := i; j > 0 && cp[j] < cp[j-1]; j-- {
			cp[j], cp[j-1] = cp[j-1], cp[j]
		}
	}
	if p >= 100 {
		return float64(cp[len(cp)-1])
	}
	// nearest-rank
	idx := (p*len(cp) + 99) / 100
	if idx < 1 {
		idx = 1
	}
	if idx > len(cp) {
		idx = len(cp)
	}
	return float64(cp[idx-1])
}

func (w *ConnectionRiskWorker) lookupUserForKey(ctx context.Context, keyID int64) int64 {
	if w.signals != nil {
		if id, err := w.signals.GetKeyOwner(ctx, keyID); err == nil && id > 0 {
			return id
		}
	}
	if w.apiKeys != nil {
		if key, err := w.apiKeys.GetByID(ctx, keyID); err == nil && key != nil {
			return key.UserID
		}
	}
	return 0
}

func (w *ConnectionRiskWorker) lookupKeyPrefix(ctx context.Context, keyID int64) string {
	if w.apiKeys == nil {
		return ""
	}
	key, err := w.apiKeys.GetByID(ctx, keyID)
	if err != nil || key == nil {
		return ""
	}
	return maskAPIKeyPrefix(key.Key)
}

// maskAPIKeyPrefix returns a short non-secret display prefix (e.g. sk-ab12…).
func maskAPIKeyPrefix(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) <= 10 {
		return key
	}
	return key[:8] + "…"
}

func primaryRule(res ConnectionRiskScoreResult) string {
	if len(res.RulesFired) == 0 {
		return "none"
	}
	// Prefer highest weight
	best := res.RulesFired[0]
	for _, h := range res.RulesFired[1:] {
		if h.Weight*h.Confidence > best.Weight*best.Confidence {
			best = h
		}
	}
	return best.RuleID
}

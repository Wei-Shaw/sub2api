package service

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
)

const (
	channelMonitorV2AggregatorLockKey = "channel-monitor-v2-aggregator"
	// Retention walks back to the longest stored tier (1d rollup = 90d). Per-tier
	// prune in the repository drops short-lived 1m/user/hist facts earlier.
	channelMonitorV2RetentionMax = 90 * 24 * time.Hour
	// First tick after upgrade prioritizes the default 90m view (with small padding).
	channelMonitorV2BootstrapFirst = 2 * time.Hour
	// Always refresh a small trailing window so late writes land without
	// re-aggregating large history every tick.
	channelMonitorV2RecentOverlap = 10 * time.Minute

	// Gentle backfill: small adaptive chunks, never default 24h hammering.
	// Initial historical chunk after the 2h seed.
	channelMonitorV2BackfillChunkInit = time.Hour
	channelMonitorV2MinBackfillChunk  = 15 * time.Minute
	// Depth-based ceilings (product phases 90m → 1d → 7d → 30d → 90d).
	channelMonitorV2MaxChunkNear1d = 2 * time.Hour
	channelMonitorV2MaxChunkNear7d = 4 * time.Hour
	channelMonitorV2MaxChunkFar    = 6 * time.Hour

	// Soft adaptive timing: grow only when a recompute is clearly cheap.
	channelMonitorV2GrowChunkUnder = 15 * time.Second
	channelMonitorV2MaxBackoff     = 10 * time.Minute
)

// channelMonitorRuntimeSubscriber is the optional settings hook that lets the
// aggregator wake immediately when channel_monitor_enabled / mode flips.
type channelMonitorRuntimeSubscriber interface {
	SubscribeChannelMonitorRuntime(listener func()) (unsubscribe func())
}

type ChannelMonitorV2Aggregator struct {
	repo       ChannelMonitorV2Repository
	db         *sql.DB
	settings   channelMonitorRuntimeReader
	instanceID string
	stopCh     chan struct{}
	// kickCh wakes the loop early after a settings change (buffered 1).
	kickCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	mu        sync.Mutex
	// backfillAt is the earliest minute already recomputed (mirrors DB cursor).
	// Zero means "not yet loaded from durable watermark this process".
	backfillAt       time.Time
	backfillChunk    time.Duration
	backfillFailures int
	// nextWaitFloor is applied after runOnce when failures require backoff.
	nextWaitFloor time.Duration
	// cursorLoaded is true after the first successful watermark read (or init).
	cursorLoaded bool
	// hasAggregated is true once any recompute in this process (or durable data) exists.
	hasAggregated bool
	unsub         func()
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewChannelMonitorV2Aggregator(repo ChannelMonitorV2Repository, db *sql.DB, settings channelMonitorRuntimeReader) *ChannelMonitorV2Aggregator {
	return &ChannelMonitorV2Aggregator{
		repo:          repo,
		db:            db,
		settings:      settings,
		instanceID:    uuid.NewString(),
		stopCh:        make(chan struct{}),
		kickCh:        make(chan struct{}, 1),
		backfillChunk: channelMonitorV2BackfillChunkInit,
	}
}

func (s *ChannelMonitorV2Aggregator) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.startOnce.Do(func() {
		s.mu.Lock()
		s.ctx, s.cancel = context.WithCancel(context.Background())
		s.mu.Unlock()
		if sub, ok := s.settings.(channelMonitorRuntimeSubscriber); ok && sub != nil {
			unsub := sub.SubscribeChannelMonitorRuntime(func() {
				s.kick()
			})
			s.mu.Lock()
			stopped := s.ctx == nil
			if !stopped {
				select {
				case <-s.ctx.Done():
					stopped = true
				default:
				}
			}
			if !stopped {
				s.unsub = unsub
			}
			s.mu.Unlock()
			if stopped && unsub != nil {
				unsub()
			}
		}
		go s.loop()
	})
}

func (s *ChannelMonitorV2Aggregator) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.mu.Lock()
		cancel := s.cancel
		unsub := s.unsub
		s.cancel = nil
		s.unsub = nil
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if unsub != nil {
			unsub()
		}
		close(s.stopCh)
	})
}

// kick wakes the aggregation loop so mode flips take effect without waiting
// for the next refresh interval.
func (s *ChannelMonitorV2Aggregator) kick() {
	if s == nil {
		return
	}
	select {
	case s.kickCh <- struct{}{}:
	default:
	}
}

func (s *ChannelMonitorV2Aggregator) loop() {
	for {
		interval := time.Minute
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if !s.passiveAggregationAllowed(ctx) {
			cancel()
			if !s.wait(interval) {
				return
			}
			continue
		}
		if cfg, err := s.repo.GetConfig(ctx); err == nil {
			if !cfg.Enabled {
				cancel()
				if !s.wait(interval) {
					return
				}
				continue
			}
			if cfg.RefreshIntervalSeconds > 0 {
				interval = time.Duration(cfg.RefreshIntervalSeconds) * time.Second
			}
		}
		cancel()
		s.runOnce()
		// Hard gate: never compress bootstrap to multi-Hz ticks. Soft gate: on
		// repeated failures raise the wait floor (exponential backoff).
		s.mu.Lock()
		floor := s.nextWaitFloor
		s.mu.Unlock()
		if floor > interval {
			interval = floor
		}
		if !s.wait(interval) {
			return
		}
	}
}

func (s *ChannelMonitorV2Aggregator) passiveAggregationAllowed(ctx context.Context) bool {
	if s == nil || s.settings == nil {
		// Fail closed without settings: do not aggregate under ambiguous mode.
		return false
	}
	return s.settings.GetChannelMonitorRuntime(ctx).PassiveAggregationAllowed()
}

func (s *ChannelMonitorV2Aggregator) wait(interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-s.kickCh:
		// Drain any coalesced kicks so a burst of settings writes only wakes once.
		for {
			select {
			case <-s.kickCh:
			default:
				return true
			}
		}
	case <-s.stopCh:
		return false
	}
}

func (s *ChannelMonitorV2Aggregator) runOnce() {
	s.mu.Lock()
	parent := s.ctx
	s.mu.Unlock()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 55*time.Second)
	defer cancel()
	release, acquired := tryAcquireSingletonLeaderLock(ctx, nil, s.db, channelMonitorV2AggregatorLockKey, s.instanceID, 2*time.Minute)
	if !acquired {
		return
	}
	if release != nil {
		defer release()
	}

	now := time.Now().UTC().Truncate(time.Minute)
	if err := s.ensureCursor(ctx, now); err != nil {
		logger.LegacyPrintf("service.channel_monitor_v2", "[ChannelMonitorV2] load watermark failed: %v", err)
		return
	}

	s.mu.Lock()
	cursor := s.backfillAt
	hasData := s.hasAggregated
	s.mu.Unlock()

	// Phase 1 (first upgrade / empty): seed the default 90m UI window quickly.
	if !hasData || cursor.IsZero() {
		start := now.Add(-channelMonitorV2BootstrapFirst)
		started := time.Now()
		if err := s.repo.RecomputeRange(ctx, start, now); err != nil {
			logger.LegacyPrintf("service.channel_monitor_v2", "[ChannelMonitorV2] bootstrap recent aggregation failed: %v", err)
			s.recordBackfillFailure(now, cursor)
			return
		}
		s.recordBackfillSuccess(start, time.Since(started), now)
		return
	}

	// Always refresh the trailing overlap so late usage/error writes land in 1m facts.
	if err := s.repo.RecomputeRange(ctx, now.Add(-channelMonitorV2RecentOverlap), now); err != nil {
		logger.LegacyPrintf("service.channel_monitor_v2", "[ChannelMonitorV2] overlap aggregation failed: %v", err)
		return
	}

	// Phase 2: walk history backward at most one chunk per tick until retention max (90d).
	// Product UI (30d) fills first; remaining 30–90d continues silently.
	retentionCutoff := now.Add(-channelMonitorV2RetentionMax)
	if !cursor.After(retentionCutoff) {
		return
	}
	end := cursor
	s.mu.Lock()
	chunk := s.backfillChunk
	s.mu.Unlock()
	if chunk <= 0 {
		chunk = channelMonitorV2BackfillChunkInit
	}
	maxChunk := channelMonitorV2MaxChunkForDepth(now, end)
	if chunk > maxChunk {
		chunk = maxChunk
	}
	if chunk < channelMonitorV2MinBackfillChunk {
		chunk = channelMonitorV2MinBackfillChunk
	}
	start := end.Add(-chunk)
	// Once bootstrap reaches historical data, keep chunks on day boundaries so
	// daily rollups never depend on 1m rows from two independently pruned chunks.
	if end.Before(now.Add(-7 * 24 * time.Hour)) {
		aligned := end.Add(-chunk).Truncate(24 * time.Hour)
		if aligned.Before(end) {
			start = aligned
		}
	}
	if start.Before(retentionCutoff) {
		start = retentionCutoff
	}
	if !start.Before(end) {
		return
	}
	started := time.Now()
	if err := s.repo.RecomputeRange(ctx, start, end); err != nil {
		logger.LegacyPrintf("service.channel_monitor_v2", "[ChannelMonitorV2] backfill failed %s..%s: %v", start, end, err)
		s.recordBackfillFailure(now, end)
		return
	}
	s.recordBackfillSuccess(start, time.Since(started), now)
}

// channelMonitorV2MaxChunkForDepth returns the hard ceiling for a historical
// chunk ending at `end` (earliest already covered / next walk end).
func channelMonitorV2MaxChunkForDepth(now, end time.Time) time.Duration {
	age := now.Sub(end)
	switch {
	case age < 24*time.Hour:
		return channelMonitorV2MaxChunkNear1d
	case age < 7*24*time.Hour:
		return channelMonitorV2MaxChunkNear7d
	default:
		return channelMonitorV2MaxChunkFar
	}
}

func (s *ChannelMonitorV2Aggregator) recordBackfillSuccess(coveredFrom time.Time, elapsed time.Duration, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backfillAt = coveredFrom
	s.hasAggregated = true
	s.backfillFailures = 0
	s.nextWaitFloor = 0
	maxChunk := channelMonitorV2MaxChunkForDepth(now, coveredFrom)
	// Grow slowly only when the recompute was clearly cheap.
	if elapsed > 0 && elapsed < channelMonitorV2GrowChunkUnder {
		next := s.backfillChunk
		if next <= 0 {
			next = channelMonitorV2BackfillChunkInit
		}
		next = time.Duration(float64(next) * 1.5)
		if next > maxChunk {
			next = maxChunk
		}
		if next < channelMonitorV2MinBackfillChunk {
			next = channelMonitorV2MinBackfillChunk
		}
		s.backfillChunk = next
		return
	}
	// Keep a healthy chunk within the depth ceiling after success.
	if s.backfillChunk <= 0 || s.backfillChunk > maxChunk {
		s.backfillChunk = channelMonitorV2BackfillChunkInit
		if s.backfillChunk > maxChunk {
			s.backfillChunk = maxChunk
		}
	}
}

func (s *ChannelMonitorV2Aggregator) recordBackfillFailure(now, end time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backfillFailures++
	// Shrink chunk toward the minimum so large DBs self-throttle.
	if s.backfillChunk <= 0 {
		s.backfillChunk = channelMonitorV2BackfillChunkInit
	}
	s.backfillChunk /= 2
	if s.backfillChunk < channelMonitorV2MinBackfillChunk {
		s.backfillChunk = channelMonitorV2MinBackfillChunk
	}
	maxChunk := channelMonitorV2MaxChunkForDepth(now, end)
	if s.backfillChunk > maxChunk {
		s.backfillChunk = maxChunk
	}
	// Exponential backoff on wait floor: 1m, 2m, 4m… capped at 10m.
	floor := time.Minute << uint(s.backfillFailures-1)
	if floor > channelMonitorV2MaxBackoff {
		floor = channelMonitorV2MaxBackoff
	}
	if floor < time.Minute {
		floor = time.Minute
	}
	s.nextWaitFloor = floor
}

// ensureCursor restores durable backfill_cursor after process restart so progress
// and historical walk continue instead of re-seeding only the last 2h.
func (s *ChannelMonitorV2Aggregator) ensureCursor(ctx context.Context, now time.Time) error {
	s.mu.Lock()
	loaded := s.cursorLoaded
	s.mu.Unlock()
	if loaded {
		return nil
	}
	wm, err := s.repo.GetAggregationWatermark(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cursorLoaded {
		return nil
	}
	if wm != nil {
		if !wm.BackfillCursor.IsZero() {
			s.backfillAt = wm.BackfillCursor.UTC().Truncate(time.Minute)
		}
		if wm.HasData || !wm.DataThrough.IsZero() {
			s.hasAggregated = true
			// Legacy rows may have data_through but null backfill_cursor (older workers).
			// Infer cursor from data_through − initial window so we do not re-bootstrap
			// only 2h and claim zero progress forever.
			if s.backfillAt.IsZero() && !wm.DataThrough.IsZero() {
				inferred := wm.DataThrough.UTC().Truncate(time.Minute).Add(-channelMonitorV2BootstrapFirst)
				if inferred.After(now) {
					inferred = now.Add(-channelMonitorV2BootstrapFirst)
				}
				s.backfillAt = inferred
			}
		}
	}
	s.cursorLoaded = true
	return nil
}

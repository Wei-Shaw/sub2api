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
	// Product bootstrap goal: fill the UI ranges 90m / 24h / 7d / 30d first.
	// Banner hides once coveredFrom reaches this depth; 90d retention continues silently.
	channelMonitorV2BootstrapWindow = ChannelMonitorV2BootstrapProductWindow
	// First tick after upgrade prioritizes the default 90m view (with small padding).
	channelMonitorV2BootstrapFirst = 2 * time.Hour
	// Subsequent bootstrap chunks grow so 24h/7d/30d fill without waiting full 90d pace.
	// Order after first tick: 22h → full 1d chunks until 30d, then 1d toward 90d.
	channelMonitorV2RecentOverlap    = 10 * time.Minute
	channelMonitorV2BackfillChunk    = 24 * time.Hour
	channelMonitorV2MinBackfillChunk = time.Hour
)

// channelMonitorRuntimeSubscriber is the optional settings hook that lets the
// aggregator wake immediately when channel_monitor_enabled / mode flips.
type channelMonitorRuntimeSubscriber interface {
	SubscribeChannelMonitorRuntime(listener func()) (unsubscribe func())
REDACTED

type ChannelMonitorV2Aggregator struct {
	repo       ChannelMonitorV2Repository
	db         *sql.DB
	settings   channelMonitorRuntimeReader
	instanceID string
	stopCh     chan struct{REDACTED
	// kickCh wakes the loop early after a settings change (buffered 1).
	kickCh    chan struct{REDACTED
	startOnce sync.Once
	stopOnce  sync.Once
	mu        sync.Mutex
	// backfillAt is the earliest minute already recomputed (mirrors DB cursor).
	// Zero means "not yet loaded from durable watermark this process".
	backfillAt       time.Time
	backfillChunk    time.Duration
	backfillFailures int
	// cursorLoaded is true after the first successful watermark read (or init).
	cursorLoaded bool
	// hasAggregated is true once any recompute in this process (or durable data) exists.
	hasAggregated bool
	unsub         func()
	ctx           context.Context
	cancel        context.CancelFunc
REDACTED

func NewChannelMonitorV2Aggregator(repo ChannelMonitorV2Repository, db *sql.DB, settings channelMonitorRuntimeReader) *ChannelMonitorV2Aggregator {
	return &ChannelMonitorV2Aggregator{
		repo:          repo,
		db:            db,
		settings:      settings,
		instanceID:    uuid.NewString(),
		stopCh:        make(chan struct{REDACTED),
		kickCh:        make(chan struct{REDACTED, 1),
		backfillChunk: channelMonitorV2BackfillChunk,
REDACTED
REDACTED

func (s *ChannelMonitorV2Aggregator) Start() {
	if s == nil || s.repo == nil {
		return
REDACTED
	s.startOnce.Do(func() {
		s.mu.Lock()
		s.ctx, s.cancel = context.WithCancel(context.Background())
		s.mu.Unlock()
		if sub, ok := s.settings.(channelMonitorRuntimeSubscriber); ok && sub != nil {
			unsub := sub.SubscribeChannelMonitorRuntime(func() {
				s.kick()
		REDACTED)
			s.mu.Lock()
			stopped := s.ctx == nil
			if !stopped {
				select {
				case <-s.ctx.Done():
					stopped = true
				default:
			REDACTED
		REDACTED
			if !stopped {
				s.unsub = unsub
		REDACTED
			s.mu.Unlock()
			if stopped && unsub != nil {
				unsub()
		REDACTED
	REDACTED
		go s.loop()
REDACTED)
REDACTED

func (s *ChannelMonitorV2Aggregator) Stop() {
	if s == nil {
		return
REDACTED
	s.stopOnce.Do(func() {
		s.mu.Lock()
		cancel := s.cancel
		unsub := s.unsub
		s.cancel = nil
		s.unsub = nil
		s.mu.Unlock()
		if cancel != nil {
			cancel()
	REDACTED
		if unsub != nil {
			unsub()
	REDACTED
		close(s.stopCh)
REDACTED)
REDACTED

// kick wakes the aggregation loop so mode flips take effect without waiting
// for the next refresh interval.
func (s *ChannelMonitorV2Aggregator) kick() {
	if s == nil {
		return
REDACTED
	select {
	case s.kickCh <- struct{REDACTED{REDACTED:
	default:
REDACTED
REDACTED

func (s *ChannelMonitorV2Aggregator) loop() {
	for {
		interval := time.Minute
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if !s.passiveAggregationAllowed(ctx) {
			cancel()
			if !s.wait(interval) {
				return
		REDACTED
			continue
	REDACTED
		if cfg, err := s.repo.GetConfig(ctx); err == nil {
			if !cfg.Enabled {
				cancel()
				if !s.wait(interval) {
					return
			REDACTED
				continue
		REDACTED
			if cfg.RefreshIntervalSeconds > 0 {
				interval = time.Duration(cfg.RefreshIntervalSeconds) * time.Second
		REDACTED
	REDACTED
		cancel()
		s.runOnce()
		// While product bootstrap is incomplete, tick faster so 90m/24h/7d/30d
		// fill without waiting a full refresh_interval between historical chunks.
		if s.bootstrapActive() {
			if interval > 5*time.Second {
				interval = 5 * time.Second
		REDACTED
	REDACTED
		if !s.wait(interval) {
			return
	REDACTED
REDACTED
REDACTED

func (s *ChannelMonitorV2Aggregator) bootstrapActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.cursorLoaded {
		return true
REDACTED
	if !s.hasAggregated {
		return true
REDACTED
	if s.backfillFailures >= 3 {
		return false
REDACTED
	now := time.Now().UTC().Truncate(time.Minute)
	target := now.Add(-channelMonitorV2BootstrapWindow)
	return s.backfillAt.IsZero() || s.backfillAt.After(target)
REDACTED

func (s *ChannelMonitorV2Aggregator) passiveAggregationAllowed(ctx context.Context) bool {
	if s == nil || s.settings == nil {
		// Fail closed without settings: do not aggregate under ambiguous mode.
		return false
REDACTED
	return s.settings.GetChannelMonitorRuntime(ctx).PassiveAggregationAllowed()
REDACTED

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
		REDACTED
	REDACTED
	case <-s.stopCh:
		return false
REDACTED
REDACTED

func (s *ChannelMonitorV2Aggregator) runOnce() {
	s.mu.Lock()
	parent := s.ctx
	s.mu.Unlock()
	if parent == nil {
		parent = context.Background()
REDACTED
	ctx, cancel := context.WithTimeout(parent, 55*time.Second)
	defer cancel()
	release, acquired := tryAcquireSingletonLeaderLock(ctx, nil, s.db, channelMonitorV2AggregatorLockKey, s.instanceID, 2*time.Minute)
	if !acquired {
		return
REDACTED
	if release != nil {
		defer release()
REDACTED

	now := time.Now().UTC().Truncate(time.Minute)
	if err := s.ensureCursor(ctx, now); err != nil {
		logger.LegacyPrintf("service.channel_monitor_v2", "[ChannelMonitorV2] load watermark failed: %v", err)
		return
REDACTED

	s.mu.Lock()
	cursor := s.backfillAt
	hasData := s.hasAggregated
	s.mu.Unlock()

	// Phase 1 (first upgrade / empty): seed the default 90m UI window quickly.
	if !hasData || cursor.IsZero() {
		start := now.Add(-channelMonitorV2BootstrapFirst)
		if err := s.repo.RecomputeRange(ctx, start, now); err != nil {
			logger.LegacyPrintf("service.channel_monitor_v2", "[ChannelMonitorV2] bootstrap recent aggregation failed: %v", err)
			return
	REDACTED
		s.mu.Lock()
		s.backfillAt = start
		s.hasAggregated = true
		s.mu.Unlock()
		return
REDACTED

	// Always refresh the trailing overlap so late usage/error writes land in 1m facts.
	if err := s.repo.RecomputeRange(ctx, now.Add(-channelMonitorV2RecentOverlap), now); err != nil {
		logger.LegacyPrintf("service.channel_monitor_v2", "[ChannelMonitorV2] overlap aggregation failed: %v", err)
		return
REDACTED

	// Phase 2: walk history backward in chunks until retention max (90d).
	// Product UI (30d) fills first; remaining 30–90d continues silently.
	retentionCutoff := now.Add(-channelMonitorV2RetentionMax)
	if !cursor.After(retentionCutoff) {
		return
REDACTED
	// Walk backward in 1d chunks. After the 2h seed, the next chunk reaches
	// past 24h so the default + daily UI ranges fill within a few ticks; 7d/30d
	// follow as more chunks complete (banner tracks progress against 30d).
	end := cursor
	s.mu.Lock()
	chunk := s.backfillChunk
	s.mu.Unlock()
	if chunk <= 0 {
		chunk = channelMonitorV2BackfillChunk
REDACTED
	start := end.Add(-chunk)
	// Once bootstrap reaches historical data, keep chunks on day boundaries so
	// daily rollups never depend on 1m rows from two independently pruned chunks.
	if end.Before(now.Add(-7 * 24 * time.Hour)) {
		start = end.Add(-chunk).Truncate(24 * time.Hour)
REDACTED
	if start.Before(retentionCutoff) {
		start = retentionCutoff
REDACTED
	if !start.Before(end) {
		return
REDACTED
	if err := s.repo.RecomputeRange(ctx, start, end); err != nil {
		logger.LegacyPrintf("service.channel_monitor_v2", "[ChannelMonitorV2] backfill failed %s..%s: %v", start, end, err)
		s.mu.Lock()
		s.backfillFailures++
		if s.backfillChunk > channelMonitorV2MinBackfillChunk {
			s.backfillChunk /= 2
			if s.backfillChunk < channelMonitorV2MinBackfillChunk {
				s.backfillChunk = channelMonitorV2MinBackfillChunk
		REDACTED
	REDACTED
		s.mu.Unlock()
		return
REDACTED
	s.mu.Lock()
	s.backfillChunk = channelMonitorV2BackfillChunk
	s.backfillFailures = 0
	s.backfillAt = start
	s.hasAggregated = true
	s.mu.Unlock()
REDACTED

// ensureCursor restores durable backfill_cursor after process restart so progress
// and historical walk continue instead of re-seeding only the last 2h.
func (s *ChannelMonitorV2Aggregator) ensureCursor(ctx context.Context, now time.Time) error {
	s.mu.Lock()
	loaded := s.cursorLoaded
	s.mu.Unlock()
	if loaded {
		return nil
REDACTED
	wm, err := s.repo.GetAggregationWatermark(ctx)
	if err != nil {
		return err
REDACTED
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cursorLoaded {
		return nil
REDACTED
	if wm != nil {
		if !wm.BackfillCursor.IsZero() {
			s.backfillAt = wm.BackfillCursor.UTC().Truncate(time.Minute)
	REDACTED
		if wm.HasData || !wm.DataThrough.IsZero() {
			s.hasAggregated = true
			// Legacy rows may have data_through but null backfill_cursor (older workers).
			// Infer cursor from data_through − initial window so we do not re-bootstrap
			// only 2h and claim zero progress forever.
			if s.backfillAt.IsZero() && !wm.DataThrough.IsZero() {
				inferred := wm.DataThrough.UTC().Truncate(time.Minute).Add(-channelMonitorV2BootstrapFirst)
				if inferred.After(now) {
					inferred = now.Add(-channelMonitorV2BootstrapFirst)
			REDACTED
				s.backfillAt = inferred
		REDACTED
	REDACTED
REDACTED
	s.cursorLoaded = true
	return nil
REDACTED

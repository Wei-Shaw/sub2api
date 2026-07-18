package service

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const (
	authInvalidationBatchSize    = 100
	authInvalidationPollInterval = 500 * time.Millisecond
	authInvalidationLease        = 30 * time.Second
	authInvalidationRedisTimeout = 2 * time.Second
	authInvalidationSafetyDelay  = 30 * time.Second
	authInvalidationConcurrency  = 16
)

type AuthCacheInvalidationEvent struct {
	ID        int64
	CacheKey  string
	Attempts  int
	Stage     int
	CreatedAt time.Time
REDACTED

type AuthCacheInvalidationOutboxStats struct {
	Pending         int64
	OldestCreatedAt *time.Time
	MaxAttempts     int
	LastError       string
REDACTED

type AuthCacheInvalidationOutboxRepository interface {
	Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]AuthCacheInvalidationEvent, error)
	DeleteClaimed(ctx context.Context, id int64, workerID string) error
	ScheduleSecondPass(ctx context.Context, id int64, workerID string, availableAt time.Time) error
	RetryClaimed(ctx context.Context, id int64, workerID string, availableAt time.Time, lastError string) error
	Stats(ctx context.Context) (AuthCacheInvalidationOutboxStats, error)
REDACTED

type AuthCacheInvalidationHealth struct {
	Running    bool          `json:"running"`
	Processed  uint64        `json:"processed"`
	Failures   uint64        `json:"failures"`
	Pending    int64         `json:"pending"`
	OldestLag  time.Duration `json:"oldest_lag"`
	LastError  string        `json:"last_error,omitempty"`
	StatsError string        `json:"stats_error,omitempty"`
	// HealthySLA includes the delayed safety pass. RecoverySLA is the maximum
	// convergence time after Redis becomes healthy, including capped backoff.
	HealthySLA  time.Duration `json:"healthy_sla"`
	RecoverySLA time.Duration `json:"recovery_sla"`
	MaxAttempts int           `json:"max_attempts"`
REDACTED

type OpsAuthCacheInvalidationHealth struct {
	Outbox       AuthCacheInvalidationHealth           `json:"outbox"`
	Subscriber   AuthCacheInvalidationSubscriberHealth `json:"subscriber"`
	Lookup       APIKeyAuthLookupMetrics               `json:"lookup"`
	InvalidAbuse InvalidAuthAbuseHealth                `json:"invalid_abuse"`
REDACTED

func (s *OpsService) GetAuthCacheInvalidationHealth(ctx context.Context) OpsAuthCacheInvalidationHealth {
	if s == nil {
		return OpsAuthCacheInvalidationHealth{REDACTED
REDACTED
	health := OpsAuthCacheInvalidationHealth{REDACTED
	if s.authCacheInvalidationWorker != nil {
		health.Outbox = s.authCacheInvalidationWorker.Health(ctx)
REDACTED
	if s.apiKeyService != nil {
		health.Subscriber = s.apiKeyService.AuthCacheInvalidationSubscriberHealth()
		health.Lookup = s.apiKeyService.AuthLookupMetrics()
		health.InvalidAbuse = s.apiKeyService.InvalidAuthAbuseHealth()
REDACTED
	return health
REDACTED

type AuthCacheInvalidationWorker struct {
	repo      AuthCacheInvalidationOutboxRepository
	cache     APIKeyCache
	local     *APIKeyService
	workerID  string
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	start     sync.Once
	stop      sync.Once
	running   atomic.Bool
	processed atomic.Uint64
	failures  atomic.Uint64
	lastError atomic.Value
REDACTED

func NewAuthCacheInvalidationWorker(repo AuthCacheInvalidationOutboxRepository, cache APIKeyCache, local ...*APIKeyService) *AuthCacheInvalidationWorker {
	ctx, cancel := context.WithCancel(context.Background())
	w := &AuthCacheInvalidationWorker{
		repo: repo, cache: cache, workerID: uuid.NewString(), ctx: ctx, cancel: cancel,
REDACTED
	if len(local) > 0 {
		w.local = local[0]
REDACTED
	w.lastError.Store("")
	return w
REDACTED

func (w *AuthCacheInvalidationWorker) Start() {
	if w == nil || w.repo == nil || w.cache == nil {
		return
REDACTED
	w.start.Do(func() {
		w.running.Store(true)
		w.wg.Add(1)
		go w.run()
REDACTED)
REDACTED

func (w *AuthCacheInvalidationWorker) Stop() {
	if w == nil {
		return
REDACTED
	w.stop.Do(func() {
		w.cancel()
		w.wg.Wait()
		w.running.Store(false)
REDACTED)
REDACTED

func (w *AuthCacheInvalidationWorker) run() {
	defer w.wg.Done()
	defer w.running.Store(false)
	ticker := time.NewTicker(authInvalidationPollInterval)
	defer ticker.Stop()
	for {
		if err := w.processBatch(w.ctx); err != nil && w.ctx.Err() == nil {
			w.recordFailure(err)
	REDACTED
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
	REDACTED
REDACTED
REDACTED

func (w *AuthCacheInvalidationWorker) processBatch(ctx context.Context) error {
	events, err := w.repo.Claim(ctx, w.workerID, authInvalidationBatchSize, authInvalidationLease)
	if err != nil {
		return fmt.Errorf("claim auth cache invalidations: %w", err)
REDACTED
	semaphore := make(chan struct{REDACTED, authInvalidationConcurrency)
	var wg sync.WaitGroup
	for i := range events {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		case semaphore <- struct{REDACTED{REDACTED:
	REDACTED
		wg.Add(1)
		go func(event AuthCacheInvalidationEvent) {
			defer wg.Done()
			defer func() { <-semaphore REDACTED()
			w.processEvent(ctx, event)
	REDACTED(events[i])
REDACTED
	wg.Wait()
	return nil
REDACTED

func (w *AuthCacheInvalidationWorker) processEvent(parent context.Context, event AuthCacheInvalidationEvent) {
	if w.local != nil {
		w.local.invalidateLocalAuthCache(event.CacheKey)
REDACTED
	ctx, cancel := context.WithTimeout(parent, authInvalidationRedisTimeout)
	err := w.cache.DeleteAuthCache(ctx, event.CacheKey)
	if err == nil {
		err = w.cache.PublishAuthCacheInvalidation(ctx, event.CacheKey)
REDACTED
	cancel()
	if err != nil {
		w.recordFailure(err)
		retryAt := time.Now().UTC().Add(authInvalidationRetryDelay(event.Attempts + 1))
		retryCtx, retryCancel := context.WithTimeout(context.Background(), 2*time.Second)
		retryErr := w.repo.RetryClaimed(retryCtx, event.ID, w.workerID, retryAt, boundedAuthInvalidationError(err))
		retryCancel()
		if retryErr != nil {
			w.recordFailure(fmt.Errorf("release failed auth invalidation %d: %w", event.ID, retryErr))
	REDACTED
		return
REDACTED
	if event.Stage == 0 {
		nextCtx, nextCancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = w.repo.ScheduleSecondPass(nextCtx, event.ID, w.workerID, time.Now().UTC().Add(authInvalidationSafetyDelay))
		nextCancel()
		if err != nil {
			w.recordFailure(fmt.Errorf("schedule second auth invalidation pass %d: %w", event.ID, err))
			return
	REDACTED
		w.processed.Add(1)
		w.lastError.Store("")
		return
REDACTED

	ackCtx, ackCancel := context.WithTimeout(context.Background(), 2*time.Second)
	err = w.repo.DeleteClaimed(ackCtx, event.ID, w.workerID)
	ackCancel()
	if err != nil {
		w.recordFailure(fmt.Errorf("ack auth invalidation %d: %w", event.ID, err))
		return
REDACTED
	w.processed.Add(1)
	w.lastError.Store("")
REDACTED

func authInvalidationRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
REDACTED
	if attempt > 9 {
		attempt = 9
REDACTED
	base := time.Second * time.Duration(1<<(attempt-1))
	return time.Duration(float64(base) * (0.8 + rand.Float64()*0.4))
REDACTED

func boundedAuthInvalidationError(err error) string {
	if err == nil {
		return ""
REDACTED
	message := err.Error()
	if len(message) > 1024 {
		return message[:1024]
REDACTED
	return message
REDACTED

func (w *AuthCacheInvalidationWorker) recordFailure(err error) {
	if err == nil {
		return
REDACTED
	w.failures.Add(1)
	w.lastError.Store(boundedAuthInvalidationError(err))
	slog.Warn("auth cache invalidation outbox processing failed", "error", err)
REDACTED

func (w *AuthCacheInvalidationWorker) Health(ctx context.Context) AuthCacheInvalidationHealth {
	health := AuthCacheInvalidationHealth{
		HealthySLA:  authInvalidationSafetyDelay + 5*time.Second,
		RecoverySLA: 6 * time.Minute,
REDACTED
	if w == nil {
		return health
REDACTED
	health.Running = w.running.Load()
	health.Processed = w.processed.Load()
	health.Failures = w.failures.Load()
	if value := w.lastError.Load(); value != nil {
		health.LastError, _ = value.(string)
REDACTED
	if w.repo == nil {
		return health
REDACTED
	stats, err := w.repo.Stats(ctx)
	if err != nil {
		health.StatsError = boundedAuthInvalidationError(err)
		return health
REDACTED
	health.Pending = stats.Pending
	health.MaxAttempts = stats.MaxAttempts
	if health.LastError == "" {
		health.LastError = stats.LastError
REDACTED
	if stats.OldestCreatedAt != nil {
		health.OldestLag = time.Since(*stats.OldestCreatedAt)
		if health.OldestLag < 0 {
			health.OldestLag = 0
	REDACTED
REDACTED
	return health
REDACTED

func ProvideAuthCacheInvalidationWorker(repo AuthCacheInvalidationOutboxRepository, cache APIKeyCache, apiKeyService *APIKeyService) *AuthCacheInvalidationWorker {
	worker := NewAuthCacheInvalidationWorker(repo, cache, apiKeyService)
	worker.Start()
	return worker
REDACTED

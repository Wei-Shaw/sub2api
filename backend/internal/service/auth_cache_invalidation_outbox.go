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
}

type AuthCacheInvalidationOutboxStats struct {
	Pending         int64
	OldestCreatedAt *time.Time
	MaxAttempts     int
	LastError       string
}

type AuthCacheInvalidationOutboxRepository interface {
	Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]AuthCacheInvalidationEvent, error)
	DeleteClaimed(ctx context.Context, id int64, workerID string) error
	ScheduleSecondPass(ctx context.Context, id int64, workerID string, availableAt time.Time) error
	RetryClaimed(ctx context.Context, id int64, workerID string, availableAt time.Time, lastError string) error
	Stats(ctx context.Context) (AuthCacheInvalidationOutboxStats, error)
}

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
}

type OpsAuthCacheInvalidationHealth struct {
	Outbox       AuthCacheInvalidationHealth           `json:"outbox"`
	Subscriber   AuthCacheInvalidationSubscriberHealth `json:"subscriber"`
	Lookup       APIKeyAuthLookupMetrics               `json:"lookup"`
	InvalidAbuse InvalidAuthAbuseHealth                `json:"invalid_abuse"`
}

func (s *OpsService) GetAuthCacheInvalidationHealth(ctx context.Context) OpsAuthCacheInvalidationHealth {
	if s == nil {
		return OpsAuthCacheInvalidationHealth{}
	}
	health := OpsAuthCacheInvalidationHealth{}
	if s.authCacheInvalidationWorker != nil {
		health.Outbox = s.authCacheInvalidationWorker.Health(ctx)
	}
	if s.apiKeyService != nil {
		health.Subscriber = s.apiKeyService.AuthCacheInvalidationSubscriberHealth()
		health.Lookup = s.apiKeyService.AuthLookupMetrics()
		health.InvalidAbuse = s.apiKeyService.InvalidAuthAbuseHealth()
	}
	return health
}

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
}

func NewAuthCacheInvalidationWorker(repo AuthCacheInvalidationOutboxRepository, cache APIKeyCache, local ...*APIKeyService) *AuthCacheInvalidationWorker {
	ctx, cancel := context.WithCancel(context.Background())
	w := &AuthCacheInvalidationWorker{
		repo: repo, cache: cache, workerID: uuid.NewString(), ctx: ctx, cancel: cancel,
	}
	if len(local) > 0 {
		w.local = local[0]
	}
	w.lastError.Store("")
	return w
}

func (w *AuthCacheInvalidationWorker) Start() {
	if w == nil || w.repo == nil || w.cache == nil {
		return
	}
	w.start.Do(func() {
		w.running.Store(true)
		w.wg.Add(1)
		go w.run()
	})
}

func (w *AuthCacheInvalidationWorker) Stop() {
	if w == nil {
		return
	}
	w.stop.Do(func() {
		w.cancel()
		w.wg.Wait()
		w.running.Store(false)
	})
}

func (w *AuthCacheInvalidationWorker) run() {
	defer w.wg.Done()
	defer w.running.Store(false)
	ticker := time.NewTicker(authInvalidationPollInterval)
	defer ticker.Stop()
	for {
		if err := w.processBatch(w.ctx); err != nil && w.ctx.Err() == nil {
			w.recordFailure(err)
		}
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *AuthCacheInvalidationWorker) processBatch(ctx context.Context) error {
	events, err := w.repo.Claim(ctx, w.workerID, authInvalidationBatchSize, authInvalidationLease)
	if err != nil {
		return fmt.Errorf("claim auth cache invalidations: %w", err)
	}
	semaphore := make(chan struct{}, authInvalidationConcurrency)
	var wg sync.WaitGroup
	for i := range events {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		case semaphore <- struct{}{}:
		}
		wg.Add(1)
		go func(event AuthCacheInvalidationEvent) {
			defer wg.Done()
			defer func() { <-semaphore }()
			w.processEvent(ctx, event)
		}(events[i])
	}
	wg.Wait()
	return nil
}

func (w *AuthCacheInvalidationWorker) processEvent(parent context.Context, event AuthCacheInvalidationEvent) {
	if w.local != nil {
		w.local.invalidateLocalAuthCache(event.CacheKey)
	}
	ctx, cancel := context.WithTimeout(parent, authInvalidationRedisTimeout)
	err := w.cache.DeleteAuthCache(ctx, event.CacheKey)
	if err == nil {
		err = w.cache.PublishAuthCacheInvalidation(ctx, event.CacheKey)
	}
	cancel()
	if err != nil {
		w.recordFailure(err)
		retryAt := time.Now().UTC().Add(authInvalidationRetryDelay(event.Attempts + 1))
		retryCtx, retryCancel := context.WithTimeout(context.Background(), 2*time.Second)
		retryErr := w.repo.RetryClaimed(retryCtx, event.ID, w.workerID, retryAt, boundedAuthInvalidationError(err))
		retryCancel()
		if retryErr != nil {
			w.recordFailure(fmt.Errorf("release failed auth invalidation %d: %w", event.ID, retryErr))
		}
		return
	}
	if event.Stage == 0 {
		nextCtx, nextCancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = w.repo.ScheduleSecondPass(nextCtx, event.ID, w.workerID, time.Now().UTC().Add(authInvalidationSafetyDelay))
		nextCancel()
		if err != nil {
			w.recordFailure(fmt.Errorf("schedule second auth invalidation pass %d: %w", event.ID, err))
			return
		}
		w.processed.Add(1)
		w.lastError.Store("")
		return
	}

	ackCtx, ackCancel := context.WithTimeout(context.Background(), 2*time.Second)
	err = w.repo.DeleteClaimed(ackCtx, event.ID, w.workerID)
	ackCancel()
	if err != nil {
		w.recordFailure(fmt.Errorf("ack auth invalidation %d: %w", event.ID, err))
		return
	}
	w.processed.Add(1)
	w.lastError.Store("")
}

func authInvalidationRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 9 {
		attempt = 9
	}
	base := time.Second * time.Duration(1<<(attempt-1))
	return time.Duration(float64(base) * (0.8 + rand.Float64()*0.4))
}

func boundedAuthInvalidationError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 1024 {
		return message[:1024]
	}
	return message
}

func (w *AuthCacheInvalidationWorker) recordFailure(err error) {
	if err == nil {
		return
	}
	w.failures.Add(1)
	w.lastError.Store(boundedAuthInvalidationError(err))
	slog.Warn("auth cache invalidation outbox processing failed", "error", err)
}

func (w *AuthCacheInvalidationWorker) Health(ctx context.Context) AuthCacheInvalidationHealth {
	health := AuthCacheInvalidationHealth{
		HealthySLA:  authInvalidationSafetyDelay + 5*time.Second,
		RecoverySLA: 6 * time.Minute,
	}
	if w == nil {
		return health
	}
	health.Running = w.running.Load()
	health.Processed = w.processed.Load()
	health.Failures = w.failures.Load()
	if value := w.lastError.Load(); value != nil {
		health.LastError, _ = value.(string)
	}
	if w.repo == nil {
		return health
	}
	stats, err := w.repo.Stats(ctx)
	if err != nil {
		health.StatsError = boundedAuthInvalidationError(err)
		return health
	}
	health.Pending = stats.Pending
	health.MaxAttempts = stats.MaxAttempts
	if health.LastError == "" {
		health.LastError = stats.LastError
	}
	if stats.OldestCreatedAt != nil {
		health.OldestLag = time.Since(*stats.OldestCreatedAt)
		if health.OldestLag < 0 {
			health.OldestLag = 0
		}
	}
	return health
}

func ProvideAuthCacheInvalidationWorker(repo AuthCacheInvalidationOutboxRepository, cache APIKeyCache, apiKeyService *APIKeyService) *AuthCacheInvalidationWorker {
	worker := NewAuthCacheInvalidationWorker(repo, cache, apiKeyService)
	worker.Start()
	return worker
}

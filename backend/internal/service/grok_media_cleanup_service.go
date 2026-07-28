package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	defaultGrokMediaCleanupInterval = 60 * time.Second
	defaultGrokMediaCleanupTimeout  = 10 * time.Second
)

// GrokMediaExpiredCleaner removes one bounded batch of each durable Grok
// media record type. OpenAIGatewayService implements this interface.
type GrokMediaExpiredCleaner interface {
	CleanupGrokMediaExpiredRecords(context.Context) GrokMediaExpiredCleanupStats
}

// GrokMediaCleanupService runs durable Grok media expiry cleanup independently
// from request traffic. A single loop prevents overlapping cleanup batches.
type GrokMediaCleanupService struct {
	cleaner        GrokMediaExpiredCleaner
	interval       time.Duration
	cleanupTimeout time.Duration

	mu      sync.Mutex
	started bool
	stopped bool
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewGrokMediaCleanupService(cleaner GrokMediaExpiredCleaner, cfg *config.Config) *GrokMediaCleanupService {
	interval := defaultGrokMediaCleanupInterval
	if cfg != nil && cfg.Idempotency.CleanupIntervalSeconds > 0 {
		interval = time.Duration(cfg.Idempotency.CleanupIntervalSeconds) * time.Second
	}
	return &GrokMediaCleanupService{
		cleaner:        cleaner,
		interval:       interval,
		cleanupTimeout: defaultGrokMediaCleanupTimeout,
	}
}

// Start launches one cleanup immediately and then repeats it on the configured
// interval. Start after Stop is intentionally a no-op.
func (s *GrokMediaCleanupService) Start() {
	if s == nil || s.cleaner == nil {
		return
	}

	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.started = true
	s.cancel = cancel
	s.done = done
	interval := s.interval
	s.mu.Unlock()

	logger.LegacyPrintf("service.grok_media_cleanup", "[GrokMediaCleanup] started interval=%s", interval)
	go s.runLoop(ctx, done)
}

// Stop cancels an in-flight cleanup and waits for the runner to exit. It is
// safe to call more than once or before Start.
func (s *GrokMediaCleanupService) Stop() {
	if s == nil {
		return
	}

	s.mu.Lock()
	if !s.stopped {
		s.stopped = true
		if s.cancel != nil {
			s.cancel()
		}
	}
	started := s.started
	done := s.done
	s.mu.Unlock()

	if started && done != nil {
		<-done
	}
}

func (s *GrokMediaCleanupService) runLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	defer logger.LegacyPrintf("service.grok_media_cleanup", "[GrokMediaCleanup] stopped")

	if ctx.Err() != nil {
		return
	}
	s.cleanupOnce(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanupOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *GrokMediaCleanupService) cleanupOnce(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, s.cleanupTimeout)
	defer cancel()
	s.cleaner.CleanupGrokMediaExpiredRecords(ctx)
}

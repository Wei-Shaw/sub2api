package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

var outboxLogURLPattern = regexp.MustCompile(`(?i)\bhttps?://[^\s]+`)
var outboxLogCredentialPattern = regexp.MustCompile(`(?i)(authorization|proxy-authorization|x-api-key|api-key|token|secret|credential|cookie)\s*[:=]\s*[^\s,;]+`)

const (
	defaultDomainOutboxPollInterval = time.Second
	defaultDomainOutboxClaimBatch   = 50
	defaultDomainOutboxLease        = 2 * time.Minute
	defaultDomainOutboxMaxAttempts  = 8
)

var ErrDomainOutboxUnknownEvent = errors.New("unknown domain outbox event")

// DomainOutboxRetryError lets a handler distinguish a transient failure from
// a permanent payload/contract failure. A plain error is treated as retryable
// to preserve at-least-once delivery for existing handlers.
type DomainOutboxRetryError struct{ Err error }

func (e *DomainOutboxRetryError) Error() string {
	if e == nil || e.Err == nil {
		return "domain outbox retryable error"
	}
	return e.Err.Error()
}
func (e *DomainOutboxRetryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type DomainOutboxDeadError struct{ Err error }

func (e *DomainOutboxDeadError) Error() string {
	if e == nil || e.Err == nil {
		return "domain outbox non-retryable error"
	}
	return e.Err.Error()
}
func (e *DomainOutboxDeadError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func RetryableDomainOutboxError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*DomainOutboxRetryError); ok {
		return err
	}
	return &DomainOutboxRetryError{Err: err}
}

func NonRetryableDomainOutboxError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*DomainOutboxDeadError); ok {
		return err
	}
	return &DomainOutboxDeadError{Err: err}
}

type DomainOutboxHandler interface {
	Handle(context.Context, *DomainOutboxEvent) error
}

type DomainOutboxCompletionHandler interface {
	Complete(context.Context, *DomainOutboxEvent, string, time.Time) (bool, error)
	Dead(context.Context, *DomainOutboxEvent, string, time.Time, string) (bool, error)
}

type DomainOutboxHandlerRegistry map[string]DomainOutboxHandler

func (r DomainOutboxHandlerRegistry) Handle(ctx context.Context, event *DomainOutboxEvent) error {
	if event == nil {
		return NonRetryableDomainOutboxError(errors.New("nil domain outbox event"))
	}
	handler, ok := r[event.EventType]
	if !ok || handler == nil {
		return NonRetryableDomainOutboxError(fmt.Errorf("%w: %s", ErrDomainOutboxUnknownEvent, event.EventType))
	}
	return handler.Handle(ctx, event)
}

type DomainOutboxWorker struct {
	repo       DomainOutboxRepository
	handlers   DomainOutboxHandler
	workerID   string
	poll       time.Duration
	batch      int
	lease      time.Duration
	maxAttempt int
	backoff    []time.Duration
	now        func() time.Time
	startOnce  sync.Once
	stopOnce   sync.Once
	stopCh     chan struct{}
	doneCh     chan struct{}
	cancel     context.CancelFunc
	started    atomic.Bool
}

type DomainOutboxWorkerOptions struct {
	WorkerID string
	Now      func() time.Time
}

func NewDomainOutboxWorker(repo DomainOutboxRepository, handlers DomainOutboxHandler, cfg *config.Config, opts ...DomainOutboxWorkerOptions) *DomainOutboxWorker {
	poll, batch, lease, maxAttempts := defaultDomainOutboxPollInterval, defaultDomainOutboxClaimBatch, defaultDomainOutboxLease, defaultDomainOutboxMaxAttempts
	backoff := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second, 160 * time.Second, 300 * time.Second}
	if cfg != nil {
		oc := cfg.ReliabilityCore.Outbox
		if oc.PollIntervalSeconds > 0 {
			poll = time.Duration(oc.PollIntervalSeconds) * time.Second
		}
		if oc.ClaimBatchSize > 0 {
			batch = oc.ClaimBatchSize
		}
		if oc.LeaseSeconds > 0 {
			lease = time.Duration(oc.LeaseSeconds) * time.Second
		}
		if oc.MaxAttempts > 0 {
			maxAttempts = oc.MaxAttempts
		}
		if len(oc.RetryBackoffSeconds) > 0 {
			backoff = make([]time.Duration, len(oc.RetryBackoffSeconds))
			for i, seconds := range oc.RetryBackoffSeconds {
				backoff[i] = time.Duration(seconds) * time.Second
			}
		}
	}
	workerID := "domain-outbox-worker"
	now := func() time.Time { return time.Now().UTC() }
	if len(opts) > 0 {
		if strings.TrimSpace(opts[0].WorkerID) != "" {
			workerID = strings.TrimSpace(opts[0].WorkerID)
		}
		if opts[0].Now != nil {
			now = opts[0].Now
		}
	}
	return &DomainOutboxWorker{repo: repo, handlers: handlers, workerID: workerID, poll: poll, batch: batch, lease: lease, maxAttempt: maxAttempts, backoff: backoff, now: now, stopCh: make(chan struct{}), doneCh: make(chan struct{})}
}

func (w *DomainOutboxWorker) RunOnce(ctx context.Context) error {
	if w == nil || w.repo == nil {
		return nil
	}
	now := w.now().UTC()
	if _, err := w.repo.ReapExpiredLeases(ctx, now, w.batch); err != nil {
		return fmt.Errorf("reap expired domain outbox leases: %w", err)
	}
	events, err := w.repo.ClaimBatch(ctx, w.workerID, now, w.batch, w.lease)
	if err != nil {
		return fmt.Errorf("claim domain outbox: %w", err)
	}
	oldestAgeSeconds := -1.0
	for _, event := range events {
		if event == nil {
			continue
		}
		if !event.CreatedAt.IsZero() {
			age := now.Sub(event.CreatedAt)
			if age < 0 {
				age = 0
			}
			if age.Seconds() > oldestAgeSeconds {
				oldestAgeSeconds = age.Seconds()
			}
		}
		w.processOne(ctx, event, now)
	}
	if oldestAgeSeconds >= 0 {
		RecordReliabilityMetricSet("domain_outbox_oldest_age_seconds", oldestAgeSeconds, nil)
	}
	return nil
}

func (w *DomainOutboxWorker) processOne(ctx context.Context, event *DomainOutboxEvent, now time.Time) {
	var err error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = RetryableDomainOutboxError(fmt.Errorf("handler panic: %v", recovered))
			}
		}()
		if w.handlers == nil {
			err = NonRetryableDomainOutboxError(errors.New("domain outbox handler registry is required"))
			return
		}
		err = w.handlers.Handle(ctx, event)
	}()
	if err == nil {
		if completion, ok := w.handlers.(DomainOutboxCompletionHandler); ok {
			if handled, completionErr := completion.Complete(ctx, event, w.workerID, now); handled {
				if completionErr != nil {
					slog.Warn("domain outbox completion failed", "event_id", event.ID, "event_type", event.EventType, "error", sanitizeOutboxLogError(completionErr))
					return
				}
				RecordReliabilityMetricSet("domain_outbox_pending_total", 0, map[string]string{"event_type": event.EventType})
				RecordReliabilityMetricSet("domain_outbox_dead_total", 0, map[string]string{"event_type": event.EventType})
				return
			}
		}
		if _, completeErr := w.repo.Complete(ctx, event.ID, w.workerID, now); completeErr != nil {
			slog.Warn("domain outbox completion failed", "event_id", event.ID, "event_type", event.EventType, "error", sanitizeOutboxLogError(completeErr))
			return
		}
		RecordReliabilityMetricSet("domain_outbox_pending_total", 0, map[string]string{"event_type": event.EventType})
		RecordReliabilityMetricSet("domain_outbox_dead_total", 0, map[string]string{"event_type": event.EventType})
		return
	}
	dead := isDomainOutboxDead(err) || event.AttemptCount >= w.maxAttempt
	next := now
	if !dead {
		delay := w.retryDelay(event.AttemptCount)
		next = now.Add(delay)
	}
	if dead {
		if completion, ok := w.handlers.(DomainOutboxCompletionHandler); ok {
			if handled, deadErr := completion.Dead(ctx, event, w.workerID, next, err.Error()); handled {
				if deadErr != nil {
					slog.Warn("domain outbox dead update failed", "event_id", event.ID, "event_type", event.EventType, "error", sanitizeOutboxLogError(deadErr))
					return
				}
				RecordReliabilityMetricSet("domain_outbox_dead_total", 1, map[string]string{"event_type": event.EventType})
				RecordReliabilityMetricSet("domain_outbox_pending_total", 0, map[string]string{"event_type": event.EventType})
				return
			}
		}
	}
	if _, retryErr := w.repo.Retry(ctx, event.ID, w.workerID, next, dead, err.Error()); retryErr != nil {
		slog.Warn("domain outbox retry update failed", "event_id", event.ID, "event_type", event.EventType, "attempt", event.AttemptCount, "error", sanitizeOutboxLogError(retryErr))
		return
	}
	if dead {
		RecordReliabilityMetricSet("domain_outbox_dead_total", 1, map[string]string{"event_type": event.EventType})
		RecordReliabilityMetricSet("domain_outbox_pending_total", 0, map[string]string{"event_type": event.EventType})
	} else {
		RecordReliabilityMetricSet("domain_outbox_pending_total", 1, map[string]string{"event_type": event.EventType})
	}
	if dead {
		slog.Error("domain outbox event dead-lettered", "event_type", event.EventType, "aggregate_id", event.AggregateID, "attempt", event.AttemptCount, "error", sanitizeOutboxLogError(err))
	}
}

func (w *DomainOutboxWorker) retryDelay(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	index := attempt - 1
	if index >= len(w.backoff) {
		index = len(w.backoff) - 1
	}
	if index < 0 || len(w.backoff) == 0 {
		return time.Second
	}
	return w.backoff[index]
}

func isDomainOutboxDead(err error) bool {
	var dead *DomainOutboxDeadError
	return errors.As(err, &dead) || errors.Is(err, ErrDomainOutboxUnknownEvent)
}

func sanitizeOutboxLogError(err error) string {
	if err == nil {
		return ""
	}
	value := outboxLogURLPattern.ReplaceAllStringFunc(strings.TrimSpace(err.Error()), func(raw string) string {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil || parsed.Host == "" {
			return "<redacted-url>"
		}
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		return parsed.String()
	})
	value = outboxLogCredentialPattern.ReplaceAllString(value, "$1=***")
	if len(value) > DomainOutboxMaxErrorSummaryBytes {
		value = value[:DomainOutboxMaxErrorSummaryBytes]
	}
	return value
}

func (w *DomainOutboxWorker) Start() {
	if w == nil {
		return
	}
	w.startOnce.Do(func() {
		w.started.Store(true)
		if w.repo == nil || w.handlers == nil {
			close(w.doneCh)
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		w.cancel = cancel
		go w.loop(ctx)
	})
}

func (w *DomainOutboxWorker) Stop() {
	if w == nil {
		return
	}
	if !w.started.Load() {
		return
	}
	w.stopOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
		close(w.stopCh)
		<-w.doneCh
	})
}

func (w *DomainOutboxWorker) loop(ctx context.Context) {
	defer close(w.doneCh)
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("domain outbox worker tick failed", "error", sanitizeOutboxLogError(err))
			}
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

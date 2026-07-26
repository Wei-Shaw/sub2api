package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
)

type NotificationOutboxMessage struct {
	ID           int64
	Event        string
	Recipient    string
	Locale       string
	Variables    map[string]string
	AttemptCount int
}

type NotificationOutboxStats struct {
	Pending         int64      `json:"pending"`
	OldestCreatedAt *time.Time `json:"oldest_created_at,omitempty"`
	Failed          int64      `json:"failed"`
}

type NotificationOutboxRepository interface {
	Claim(ctx context.Context, workerID string, limit, maxAttempts int, lease time.Duration) ([]NotificationOutboxMessage, error)
	MarkDelivered(ctx context.Context, id int64, workerID string) error
	MarkRetry(ctx context.Context, id int64, workerID string, nextAttempt time.Time, lastError string, terminal bool) error
	Stats(ctx context.Context, maxAttempts int) (NotificationOutboxStats, error)
}

type NotificationOutboxSender interface {
	Send(ctx context.Context, input NotificationEmailSendInput) error
}

type NotificationOutboxWorker struct {
	repo        NotificationOutboxRepository
	emailer     NotificationOutboxSender
	workerID    string
	poll        time.Duration
	maxAttempts int
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	start       sync.Once
	stop        sync.Once
	running     atomic.Bool
	delivered   atomic.Uint64
	failures    atomic.Uint64
}

func NewNotificationOutboxWorker(repo NotificationOutboxRepository, emailer *NotificationEmailService, cfg *config.Config) *NotificationOutboxWorker {
	poll := 5 * time.Second
	maxAttempts := 10
	if cfg != nil {
		if cfg.Company.OutboxPollSeconds > 0 {
			poll = time.Duration(cfg.Company.OutboxPollSeconds) * time.Second
		}
		if cfg.Company.OutboxMaxAttempts > 0 {
			maxAttempts = cfg.Company.OutboxMaxAttempts
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &NotificationOutboxWorker{
		repo: repo, emailer: emailer, workerID: uuid.NewString(), poll: poll,
		maxAttempts: maxAttempts, ctx: ctx, cancel: cancel,
	}
}

func (w *NotificationOutboxWorker) Start() {
	if w == nil || w.repo == nil || w.emailer == nil {
		return
	}
	w.start.Do(func() {
		w.running.Store(true)
		w.wg.Add(1)
		go w.run()
	})
}

func (w *NotificationOutboxWorker) Stop() {
	if w == nil {
		return
	}
	w.stop.Do(func() {
		w.cancel()
		w.wg.Wait()
		w.running.Store(false)
	})
}

func (w *NotificationOutboxWorker) run() {
	defer w.wg.Done()
	defer w.running.Store(false)
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		if err := w.ProcessOnce(w.ctx); err != nil && w.ctx.Err() == nil {
			w.failures.Add(1)
			slog.Warn("notification outbox batch failed", "error", err)
		}
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *NotificationOutboxWorker) ProcessOnce(ctx context.Context) error {
	if w == nil || w.repo == nil || w.emailer == nil {
		return nil
	}
	messages, err := w.repo.Claim(ctx, w.workerID, 50, w.maxAttempts, 2*w.poll+30*time.Second)
	if err != nil {
		return fmt.Errorf("claim notification outbox: %w", err)
	}
	for _, message := range messages {
		sendErr := w.emailer.Send(ctx, NotificationEmailSendInput{
			Event: message.Event, Locale: message.Locale, RecipientEmail: message.Recipient,
			SourceType: "notification_outbox", SourceID: strconv.FormatInt(message.ID, 10),
			Variables: message.Variables,
		})
		if sendErr == nil {
			if err := w.repo.MarkDelivered(ctx, message.ID, w.workerID); err != nil {
				return fmt.Errorf("ack notification outbox %d: %w", message.ID, err)
			}
			w.delivered.Add(1)
			continue
		}
		w.failures.Add(1)
		terminal := message.AttemptCount >= w.maxAttempts
		nextAttempt := time.Now().UTC().Add(notificationOutboxRetryDelay(message.AttemptCount))
		if err := w.repo.MarkRetry(ctx, message.ID, w.workerID, nextAttempt, boundedNotificationError(sendErr), terminal); err != nil {
			return fmt.Errorf("retry notification outbox %d: %w", message.ID, err)
		}
	}
	return nil
}

func notificationOutboxRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	delay := time.Second * time.Duration(1<<uint(attempt-1))
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

func boundedNotificationError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	if len(value) > 1000 {
		value = value[:1000]
	}
	return value
}

type NotificationOutboxHealth struct {
	Running   bool   `json:"running"`
	Delivered uint64 `json:"delivered"`
	Failures  uint64 `json:"failures"`
	Pending   int64  `json:"pending"`
	Failed    int64  `json:"failed"`
	OldestLag int64  `json:"oldest_lag_seconds"`
}

func (w *NotificationOutboxWorker) Health(ctx context.Context) NotificationOutboxHealth {
	health := NotificationOutboxHealth{Running: w != nil && w.running.Load()}
	if w == nil {
		return health
	}
	health.Delivered = w.delivered.Load()
	health.Failures = w.failures.Load()
	if w.repo != nil {
		if stats, err := w.repo.Stats(ctx, w.maxAttempts); err == nil {
			health.Pending = stats.Pending
			health.Failed = stats.Failed
			if stats.OldestCreatedAt != nil {
				lag := int64(time.Since(*stats.OldestCreatedAt).Seconds())
				if lag > 0 {
					health.OldestLag = lag
				}
			}
		}
	}
	return health
}

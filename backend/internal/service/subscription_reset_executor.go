package service

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SubscriptionResetExecutionRepository interface {
	ListReadyResetEvents(context.Context, int) ([]string, error)
	ExecuteResetEvent(context.Context, string) (*RotateGroupQuotaResult, error)
}

// SubscriptionResetExecutor only consumes freshly confirmed events belonging
// to the current automatic policy. Publication is atomic with its audit row;
// duplicate delivery cannot grant quota twice.
type SubscriptionResetExecutor struct {
	repo             SubscriptionResetExecutionRepository
	leader           LeaderLockCache
	db               *sql.DB
	owner            string
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.Mutex
	started, stopped bool
	wg               sync.WaitGroup
}

func NewSubscriptionResetExecutor(repo SubscriptionResetExecutionRepository, leader LeaderLockCache, db *sql.DB) *SubscriptionResetExecutor {
	ctx, cancel := context.WithCancel(context.Background())
	return &SubscriptionResetExecutor{repo: repo, leader: leader, db: db, owner: uuid.NewString(), ctx: ctx, cancel: cancel}
}
func (e *SubscriptionResetExecutor) Start() {
	if e == nil || e.repo == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started || e.stopped {
		return
	}
	e.started = true
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-e.ctx.Done():
				return
			case <-ticker.C:
				e.runOnce(e.ctx)
			}
		}
	}()
}
func (e *SubscriptionResetExecutor) Stop() {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.stopped = true
	e.cancel()
	e.mu.Unlock()
	e.wg.Wait()
}
func (e *SubscriptionResetExecutor) runOnce(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	release, ok := tryAcquireSingletonLeaderLock(ctx, e.leader, e.db, "jobs:subscription-reset-executor", e.owner, 20*time.Second)
	if !ok {
		return
	}
	defer release()
	ids, err := e.repo.ListReadyResetEvents(ctx, 20)
	if err != nil {
		slog.Warn("subscription_reset: list confirmed events failed", "error", err)
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		if _, err := e.repo.ExecuteResetEvent(ctx, id); err != nil {
			slog.Warn("subscription_reset: event remains pending", "event_id", id, "error", err)
		}
	}
}

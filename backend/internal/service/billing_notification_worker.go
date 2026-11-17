package service

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alitto/pond/v2"
)

const (
	usageNotificationWorkers   = 4
	usageNotificationQueueSize = 128
	usageNotificationTimeout   = 30 * time.Second
)

// Notification work is best effort and must not create an unbounded queue or
// compete with committed-charge persistence for usage workers. A task includes
// both settings reads and email delivery, including all quota dimensions.
type usageNotificationWorker struct {
	pool        pond.Pool
	ctx         context.Context
	cancel      context.CancelFunc
	stopOnce    sync.Once
	dropped     atomic.Uint64
	lastDropLog atomic.Int64
}

func newUsageNotificationWorker() *usageNotificationWorker {
	ctx, cancel := context.WithCancel(context.Background())
	return &usageNotificationWorker{
		pool:   pond.NewPool(usageNotificationWorkers, pond.WithQueueSize(usageNotificationQueueSize)),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (w *usageNotificationWorker) submit(task func(context.Context)) bool {
	if w == nil || task == nil {
		return false
	}
	_, accepted := w.pool.TrySubmit(func() {
		if w.ctx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(w.ctx, usageNotificationTimeout)
		defer cancel()
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("panic in usage notification", "recover", recovered)
			}
		}()
		task(ctx)
	})
	if !accepted {
		count := w.dropped.Add(1)
		now := time.Now().UnixNano()
		last := w.lastDropLog.Load()
		if now-last >= int64(5*time.Second) && w.lastDropLog.CompareAndSwap(last, now) {
			slog.Warn("usage notification queue unavailable; notification skipped", "dropped_total", count)
		}
	}
	return accepted
}

func (w *usageNotificationWorker) stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		// Cancel settings reads and discard queued notifications during shutdown.
		// Active SMTP calls also have their own dial and I/O deadlines.
		w.cancel()
		w.pool.StopAndWait()
	})
}

func (s *BalanceNotifyService) submitUsageNotifications(p *postUsageBillingParams, deps *billingDeps, result *UsageBillingApplyResult) {
	if s == nil || s.usageNotifications == nil || p == nil || p.Cost == nil {
		return
	}
	balanceEnabled := !p.IsSubscriptionBill && p.Cost.ActualCost > 0 && s.canNotifyBalance(p.User)
	quotaEnabled := p.Cost.TotalCost > 0 && p.Account != nil && p.Account.SupportsAccountQuota() && p.Account.HasAnyQuotaLimit()
	if !balanceEnabled && !quotaEnabled {
		return
	}
	s.usageNotifications.submit(func(ctx context.Context) {
		notifyBalanceLow(ctx, p, deps, result)
		if ctx.Err() == nil {
			notifyAccountQuota(ctx, p, deps, result)
		}
	})
}

// Stop cancels notification work and waits for the bounded workers to exit.
// Billing persistence belongs to usage tasks and is never dropped here.
func (s *BalanceNotifyService) Stop() {
	if s != nil {
		s.usageNotifications.stop()
	}
}

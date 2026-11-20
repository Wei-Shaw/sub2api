package openaiasync

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Executor func(context.Context, *Job, *Item) Result

type Worker struct {
	store       *Store
	execute     Executor
	concurrency int
	maxOutput   int64
	timeout     time.Duration
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewWorker(store *Store, execute Executor, concurrency int, maxOutput int64, timeout time.Duration) *Worker {
	return &Worker{store: store, execute: execute, concurrency: concurrency, maxOutput: maxOutput, timeout: timeout}
}

func (w *Worker) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.wg.Add(1)
	go func() { defer w.wg.Done(); w.maintenance(ctx) }()
	for n := 0; n < w.concurrency; n++ {
		w.wg.Add(1)
		go func() { defer w.wg.Done(); w.loop(ctx) }()
	}
}

func (w *Worker) maintenance(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.store.Cleanup(ctx); err != nil && ctx.Err() == nil {
				slog.Error("file cleanup failed", "error", err)
			}
		}
	}
}

func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}

func (w *Worker) loop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		j, err := w.store.Claim(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Error("batch claim failed", "error", err)
		}
		if j != nil {
			w.run(ctx, j)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) run(parent context.Context, j *Job) {
	ctx, cancel := context.WithCancel(parent)
	// Keep ownership for long upstream calls. On uncertainty, cancel the call
	// and leave its running item for interruption recovery, without replaying it.
	done := make(chan struct{})
	renewDone := make(chan struct{})
	// The main loop replaces j.Batch as counts and status change. Keep the
	// lease identity immutable so renewal never reads that shared state.
	renewal := &Job{Batch: Batch{ID: j.ID}, Lease: j.Lease}
	go func() {
		defer close(renewDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				renewCtx, c := context.WithTimeout(ctx, 5*time.Second)
				err := w.store.Renew(renewCtx, renewal)
				c()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { close(done); cancel(); <-renewDone }()
	for ctx.Err() == nil {
		i, err := w.store.Next(ctx, j)
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("batch next request failed", "batch_id", j.ID, "error", err)
			}
			return
		}
		if i == nil {
			if err = w.store.Finish(ctx, j, w.maxOutput); err != nil && ctx.Err() == nil {
				slog.Error("batch finalize failed", "batch_id", j.ID, "error", err)
			}
			return
		}
		remaining := time.Until(time.Unix(j.ExpiresAt, 0))
		timeout := w.timeout
		if remaining < timeout {
			timeout = remaining
		}
		callCtx, callCancel := context.WithTimeout(ctx, timeout)
		result := w.invoke(callCtx, j, i)
		callCancel()
		if ctx.Err() != nil {
			return
		}
		if err = w.store.SaveResult(ctx, j, i, result, w.maxOutput); err != nil {
			slog.Error("batch result persistence failed", "batch_id", j.ID, "error", err)
			return
		}
	}
}

func (w *Worker) invoke(ctx context.Context, j *Job, i *Item) (result Result) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("batch executor panicked", "batch_id", j.ID)
			result = Failure(i.CustomID, "internal_error", "Batch execution failed")
		}
	}()
	return w.execute(ctx, j, i)
}

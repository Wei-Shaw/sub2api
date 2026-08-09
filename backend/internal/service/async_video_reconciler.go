package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	defaultAsyncVideoReconcileInterval = 30 * time.Second
	defaultAsyncVideoReconcileBatch    = 100
)

// AsyncVideoReconciler 周期性扫描未终结的视频任务，兜底推进到终态。
// 与 AsyncMediaReconciler 语义完全一致，但作用于 async_video_tasks 表。
type AsyncVideoReconciler struct {
	taskRepo    AsyncVideoTaskRepository
	exec        *AsyncVideoService
	accountRepo AccountRepository

	interval  time.Duration
	batchSize int

	parentCtx    context.Context
	parentCancel context.CancelFunc

	reloadCh chan struct{}

	mu      sync.Mutex
	started bool
	stopped bool
	wg      sync.WaitGroup
}

// NewAsyncVideoReconciler 构造视频对账 worker。
func NewAsyncVideoReconciler(
	taskRepo AsyncVideoTaskRepository,
	exec *AsyncVideoService,
	accountRepo AccountRepository,
) *AsyncVideoReconciler {
	ctx, cancel := context.WithCancel(context.Background())
	return &AsyncVideoReconciler{
		taskRepo:     taskRepo,
		exec:         exec,
		accountRepo:  accountRepo,
		interval:     defaultAsyncVideoReconcileInterval,
		batchSize:    defaultAsyncVideoReconcileBatch,
		parentCtx:    ctx,
		parentCancel: cancel,
		reloadCh:     make(chan struct{}, 1),
	}
}

// SetInterval 配置扫描间隔（可动态热更新）。
func (r *AsyncVideoReconciler) SetInterval(d time.Duration) {
	if d <= 0 {
		return
	}
	r.mu.Lock()
	r.interval = d
	r.mu.Unlock()
	select {
	case r.reloadCh <- struct{}{}:
	default:
	}
}

func (r *AsyncVideoReconciler) Interval() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.interval
}

// SetBatchSize 配置单轮扫描批量大小。
func (r *AsyncVideoReconciler) SetBatchSize(n int) {
	if n > 0 {
		r.batchSize = n
	}
}

// Start 启动后台扫描循环（幂等）。
func (r *AsyncVideoReconciler) Start() {
	if r == nil || r.taskRepo == nil || r.exec == nil {
		return
	}
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.mu.Unlock()

	r.wg.Add(1)
	go r.loop()
}

// Stop 停止后台扫描。
func (r *AsyncVideoReconciler) Stop() {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	r.mu.Unlock()
	r.parentCancel()
	r.wg.Wait()
}

func (r *AsyncVideoReconciler) loop() {
	defer r.wg.Done()
	ticker := time.NewTicker(r.Interval())
	defer ticker.Stop()
	for {
		select {
		case <-r.parentCtx.Done():
			return
		case <-r.reloadCh:
			ticker.Reset(r.Interval())
		case <-ticker.C:
			r.runOnce(r.parentCtx)
		}
	}
}

func (r *AsyncVideoReconciler) runOnce(ctx context.Context) {
	tasks, err := r.taskRepo.ListUnfinished(ctx, r.batchSize)
	if err != nil {
		logger.L().Warn("async_video.reconcile_list_failed", zap.Error(err))
		return
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		r.reconcileOne(ctx, task)
	}
}

func (r *AsyncVideoReconciler) reconcileOne(ctx context.Context, task *AsyncVideoTask) {
	defer func() {
		if rec := recover(); rec != nil {
			logger.L().Error("async_video.reconcile_panic",
				zap.Int64("task_id", task.ID), zap.Any("recover", rec))
		}
	}()

	var account *Account
	if task.AccountID != nil && r.accountRepo != nil {
		acc, err := r.accountRepo.GetByID(ctx, *task.AccountID)
		if err != nil {
			logger.L().Warn("async_video.reconcile_account_load_failed",
				zap.Int64("task_id", task.ID), zap.Int64p("account_id", task.AccountID), zap.Error(err))
		} else {
			account = acc
		}
	}
	if err := r.exec.ReconcileTask(ctx, task, account); err != nil {
		logger.L().Warn("async_video.reconcile_task_failed",
			zap.Int64("task_id", task.ID), zap.Error(err))
	}
}

package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	// defaultAsyncMediaReconcileInterval 是 reconciler 默认扫描间隔。
	defaultAsyncMediaReconcileInterval = 30 * time.Second
	// defaultAsyncMediaReconcileBatch 是单轮扫描处理的最大任务数。
	defaultAsyncMediaReconcileBatch = 100
)

// AsyncMediaReconciler 周期性扫描未终结（pending/running）的异步媒体任务，
// 通过 AsyncMediaService 补完成（取结果+转存+结算）或补退费（明确失败/超期 expired）。
//
// 幂等性由 AsyncMediaService 内的 MarkSucceeded/MarkRefunded（status 终态守卫）保证，
// 不会与伪同步等待路径产生重复退费 / 重复 usage_log。
type AsyncMediaReconciler struct {
	taskRepo    AsyncMediaTaskRepository
	exec        *AsyncMediaService
	accountRepo AccountRepository

	interval  time.Duration
	batchSize int

	parentCtx    context.Context
	parentCancel context.CancelFunc

	// reloadCh 用于在运行时通知 loop 以新的 interval 重置 ticker（动态配置 9.4）。
	reloadCh chan struct{}

	mu      sync.Mutex
	started bool
	stopped bool
	wg      sync.WaitGroup
}

// NewAsyncMediaReconciler 构造对账 worker。
func NewAsyncMediaReconciler(
	taskRepo AsyncMediaTaskRepository,
	exec *AsyncMediaService,
	accountRepo AccountRepository,
) *AsyncMediaReconciler {
	ctx, cancel := context.WithCancel(context.Background())
	return &AsyncMediaReconciler{
		taskRepo:     taskRepo,
		exec:         exec,
		accountRepo:  accountRepo,
		interval:     defaultAsyncMediaReconcileInterval,
		batchSize:    defaultAsyncMediaReconcileBatch,
		parentCtx:    ctx,
		parentCancel: cancel,
		reloadCh:     make(chan struct{}, 1),
	}
}

// SetInterval 配置扫描间隔（可配置项 9.4）。
// 支持运行时调整：若 loop 已启动，会通过 reloadCh 通知其以新间隔重置 ticker。
func (r *AsyncMediaReconciler) SetInterval(d time.Duration) {
	if d <= 0 {
		return
	}
	r.mu.Lock()
	r.interval = d
	r.mu.Unlock()
	// 非阻塞通知 loop 重置 ticker；reloadCh 容量为 1，重复通知会被合并。
	select {
	case r.reloadCh <- struct{}{}:
	default:
	}
}

// Interval 返回当前生效的扫描间隔。
func (r *AsyncMediaReconciler) Interval() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.interval
}

// SetBatchSize 配置单轮扫描批量大小。
func (r *AsyncMediaReconciler) SetBatchSize(n int) {
	if n > 0 {
		r.batchSize = n
	}
}

// Start 启动后台扫描循环（幂等：仅生效一次）。
func (r *AsyncMediaReconciler) Start() {
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

// Stop 停止后台扫描并等待退出。
func (r *AsyncMediaReconciler) Stop() {
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

func (r *AsyncMediaReconciler) loop() {
	defer r.wg.Done()
	ticker := time.NewTicker(r.Interval())
	defer ticker.Stop()
	for {
		select {
		case <-r.parentCtx.Done():
			return
		case <-r.reloadCh:
			// 运行时间隔变更：以最新 interval 重置 ticker。
			ticker.Reset(r.Interval())
		case <-ticker.C:
			r.runOnce(r.parentCtx)
		}
	}
}

// runOnce 扫描并推进一批未终结任务。
func (r *AsyncMediaReconciler) runOnce(ctx context.Context) {
	tasks, err := r.taskRepo.ListUnfinished(ctx, r.batchSize)
	if err != nil {
		logger.L().Warn("async_media.reconcile_list_failed", zap.Error(err))
		return
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		r.reconcileOne(ctx, task)
	}
}

func (r *AsyncMediaReconciler) reconcileOne(ctx context.Context, task *AsyncMediaTask) {
	defer func() {
		if rec := recover(); rec != nil {
			logger.L().Error("async_media.reconcile_panic",
				zap.Int64("task_id", task.ID), zap.Any("recover", rec))
		}
	}()

	var account *Account
	if task.AccountID != nil && r.accountRepo != nil {
		acc, err := r.accountRepo.GetByID(ctx, *task.AccountID)
		if err != nil {
			logger.L().Warn("async_media.reconcile_account_load_failed",
				zap.Int64("task_id", task.ID), zap.Int64p("account_id", task.AccountID), zap.Error(err))
		} else {
			account = acc
		}
	}
	if err := r.exec.ReconcileTask(ctx, task, account); err != nil {
		logger.L().Warn("async_media.reconcile_task_failed",
			zap.Int64("task_id", task.ID), zap.Error(err))
	}
}

package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/alitto/pond/v2"
)

// ChannelMonitorRunner 渠道监控调度器。
//
// 职责：
//   - 每 monitorTickerInterval 扫描一次"到期需要检测"的监控
//   - 通过 pond 池（容量 monitorWorkerConcurrency）异步执行检测
//   - 每小时检查一次时钟，到 monitorCleanupHour 点时执行历史清理
//   - Stop 时优雅关闭：池 drain + ticker.Stop + wg.Wait
//
// 不引入 cron 库；清理调度通过"每小时检查时间"实现，足够 MVP。
//
// 定时任务维护：删除/创建/编辑 monitor 无需显式 reload，每个 tick 都会重新查 DB
// （ListEnabled + listDueForCheck），新 monitor 的 LastCheckedAt 为 nil 天然立即到期，
// 被删除的 monitor 自然不再返回，interval 变化下次 tick 自动按新值判定。
type ChannelMonitorRunner struct {
	svc            *ChannelMonitorService
	settingService *SettingService

	pool   pond.Pool
	stopCh chan struct{}
	once   sync.Once
	wg     sync.WaitGroup

	// inFlight 跟踪正在执行的 monitor.ID。tickDueChecks 调度前会检查避免重复提交，
	// 防止单次检测耗时 > interval 时同一 monitor 被并发执行。
	inFlight   map[int64]struct{}
	inFlightMu sync.Mutex

	// 清理状态：lastCleanupDay 记录上次清理的"年-月-日"，避免同一天重复跑。
	lastCleanupDay string
	cleanupMu      sync.Mutex
}

// NewChannelMonitorRunner 构造调度器。Start 在 wire 中调用。
// settingService 用于在每次 tick 前读取功能开关；传 nil 时视为总是启用（兼容测试）。
func NewChannelMonitorRunner(svc *ChannelMonitorService, settingService *SettingService) *ChannelMonitorRunner {
	return &ChannelMonitorRunner{
		svc:            svc,
		settingService: settingService,
		stopCh:         make(chan struct{}),
		inFlight:       make(map[int64]struct{}),
	}
}

// Start 启动 ticker + worker pool + cleanup loop。
// 调用方需保证只调一次（wire ProvideChannelMonitorRunner 内只调一次）。
func (r *ChannelMonitorRunner) Start() {
	if r == nil || r.svc == nil {
		return
	}
	// 容量 5 的 pond 池：超出时调用方等待，避免调度堆积无限增长。
	r.pool = pond.NewPool(monitorWorkerConcurrency)

	r.wg.Add(2)
	go r.dueCheckLoop()
	go r.cleanupLoop()
}

// Stop 优雅停止：close stopCh -> 等待两个 loop 退出 -> 池 drain。
func (r *ChannelMonitorRunner) Stop() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		close(r.stopCh)
	})
	r.wg.Wait()
	if r.pool != nil {
		r.pool.StopAndWait()
	}
}

// dueCheckLoop 每 monitorTickerInterval 扫描一次"到期监控"，提交到池。
func (r *ChannelMonitorRunner) dueCheckLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(monitorTickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.tickDueChecks()
		}
	}
}

// tickDueChecks 一次扫描：查询到期监控并逐个提交到池。
// 已在执行的 monitor 会被跳过（防止单次检测耗时 > interval 时重复调度）。
// 池满时使用 TrySubmit 跳过（不能阻塞 ticker），同时立即释放已占用的 inFlight 槽。
// 当功能开关关闭时直接返回——管理员可以动态禁用模块，runner 不会拉取 DB。
func (r *ChannelMonitorRunner) tickDueChecks() {
	ctx, cancel := context.WithTimeout(context.Background(), monitorListDueTimeout)
	defer cancel()

	if r.settingService != nil && !r.settingService.GetChannelMonitorRuntime(ctx).Enabled {
		return
	}

	due, err := r.svc.listDueForCheck(ctx)
	if err != nil {
		slog.Warn("channel_monitor: list due failed", "error", err)
		return
	}
	for _, m := range due {
		monitor := m
		if !r.tryAcquireInFlight(monitor.ID) {
			slog.Debug("channel_monitor: skip already in-flight",
				"monitor_id", monitor.ID, "name", monitor.Name)
			continue
		}
		if _, ok := r.pool.TrySubmit(func() {
			r.runOne(monitor.ID, monitor.Name)
		}); !ok {
			// 池满：丢弃本次检测，但必须释放已占用的 inFlight 槽，否则该 monitor 会被永久卡住。
			r.releaseInFlight(monitor.ID)
			slog.Warn("channel_monitor: worker pool full, skip submission",
				"monitor_id", monitor.ID, "name", monitor.Name)
		}
	}
}

// tryAcquireInFlight 原子地占用 monitor 的 in-flight 槽。
// 已被占用返回 false（调用方应跳过本次提交）。
func (r *ChannelMonitorRunner) tryAcquireInFlight(id int64) bool {
	r.inFlightMu.Lock()
	defer r.inFlightMu.Unlock()
	if _, exists := r.inFlight[id]; exists {
		return false
	}
	r.inFlight[id] = struct{}{}
	return true
}

// releaseInFlight 释放 in-flight 槽。runOne 完成（含 panic recover）后必须调用。
func (r *ChannelMonitorRunner) releaseInFlight(id int64) {
	r.inFlightMu.Lock()
	delete(r.inFlight, id)
	r.inFlightMu.Unlock()
}

// runOne 执行单个监控的检测。所有错误只记日志，不熔断。
// 任务结束时（含 panic recover）必须释放 in-flight 槽。
//
// 单次解密路径：调 RunCheckByID，内部统一 Get + APIKeyDecryptFailed 判定 + 跑检测，
// 避免 runner 自己再 Get 一次造成密文二次解密。
func (r *ChannelMonitorRunner) runOne(id int64, name string) {
	// 单次任务上限 = 请求超时 + ping + 一些缓冲。
	ctx, cancel := context.WithTimeout(context.Background(), monitorRequestTimeout+monitorPingTimeout+monitorRunOneBuffer)
	defer cancel()

	defer r.releaseInFlight(id)

	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("channel_monitor: runner panic",
				"monitor_id", id, "name", name, "panic", rec)
		}
	}()

	if _, err := r.svc.RunCheck(ctx, id); err != nil {
		// ErrChannelMonitorAPIKeyDecryptFailed 是预期可恢复错误，降为 Warn 即可。
		slog.Warn("channel_monitor: run check failed",
			"monitor_id", id, "name", name, "error", err)
	}
}

// cleanupLoop 每小时检查当前时间，到 monitorCleanupHour 点（且当天还没清理过）则跑一次清理。
// 启动时立即检查一次，避免长时间运行才跑首次清理。
func (r *ChannelMonitorRunner) cleanupLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(monitorCleanupCheckInterval)
	defer ticker.Stop()

	r.maybeRunCleanup()
	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.maybeRunCleanup()
		}
	}
}

// maybeRunCleanup 如果当前小时是 monitorCleanupHour 且当天未跑过，则执行清理。
func (r *ChannelMonitorRunner) maybeRunCleanup() {
	now := time.Now()
	if now.Hour() != monitorCleanupHour {
		return
	}
	day := now.Format(monitorCleanupDayLayout)

	r.cleanupMu.Lock()
	if r.lastCleanupDay == day {
		r.cleanupMu.Unlock()
		return
	}
	r.lastCleanupDay = day
	r.cleanupMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()
	if err := r.svc.cleanupOldHistory(ctx); err != nil {
		slog.Warn("channel_monitor: cleanup history failed", "error", err)
	}
}

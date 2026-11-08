package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/google/uuid"
)

// channelMonitorProbeLockKeyPrefix 是多实例选主键前缀：同一条监控同一时刻
// 只允许一个实例真正发探针。探针会真花钱、真写使用记录，重复执行就是重复消费
// （2026-10-09 演练容器残留事故：两个实例各探一遍，记录翻倍）。
const channelMonitorProbeLockKeyPrefix = "channel:monitor:probe:"

// channelMonitorProbeLockTTL 是选主锁的崩溃兜底 TTL：必须大于单次探针最坏耗时
// （runOne 的 ctx 超时 = monitorRequestTimeout + monitorPingTimeout + buffer），
// 正常路径探针一结束就释放，不靠 TTL 续期。
const channelMonitorProbeLockTTL = 3 * time.Minute

// MonitorScheduler 调度器接口，供 ChannelMonitorService 在 CRUD 时回调，
// 用 setter 注入避免 service ↔ runner 的 wire 依赖环。
type MonitorScheduler interface {
	// Schedule 为指定监控创建（或重置）独立定时任务。
	// 当 m.Enabled=false 时等同于 Unschedule(m.ID)。
	Schedule(m *ChannelMonitor)
	// Unschedule 取消指定监控的定时任务（若存在）。
	Unschedule(id int64)
}

// monitorRunnerSvc 抽出 runner 实际依赖的两个 service 方法：
//   - 启动时加载 enabled monitor
//   - 每次 ticker 触发执行检测
//
// 用接口而非 *ChannelMonitorService 是为了让 runner 单元测试可注入轻量 stub，
// 避免依赖完整的 repo + encryptor 链路。生产实现 *ChannelMonitorService 自然满足。
type monitorRunnerSvc interface {
	ListEnabledMonitors(ctx context.Context) ([]*ChannelMonitor, error)
	RunCheck(ctx context.Context, id int64) ([]*CheckResult, error)
}

// ChannelMonitorRunner 渠道监控调度器。
//
// 设计：
//   - 每个 enabled monitor 对应一个独立 goroutine + ticker（按各自 IntervalSeconds）
//   - Start 时一次性加载所有 enabled monitor 并为每个建立任务
//   - Service 在 Create/Update/Delete 后通过 MonitorScheduler 接口回调，
//     即时重建/取消对应任务（无需轮询 DB）
//   - 实际 HTTP 检测交给 pond 池（容量 monitorWorkerConcurrency），
//     防止突发并发拖垮上游
//
// 历史清理与日聚合维护由 OpsCleanupService 的 cron 触发
// ChannelMonitorService.RunDailyMaintenance（复用 leader lock + heartbeat），
// 不在 runner 职责内。
type ChannelMonitorRunner struct {
	svc            monitorRunnerSvc
	settingService *SettingService

	pool         pond.Pool
	parentCtx    context.Context
	parentCancel context.CancelFunc

	mu      sync.Mutex
	tasks   map[int64]*scheduledMonitor
	wg      sync.WaitGroup
	started bool
	stopped bool

	// inFlight 跟踪正在执行的 monitor.ID。fire 调度前会检查避免重复提交，
	// 防止单次检测耗时 > interval 时同一 monitor 被并发执行。
	inFlight   map[int64]struct{}
	inFlightMu sync.Mutex

	// activeWorkers 当前在跑的探测数。pool 容量固定为允许的最大并发，
	// 真正的并发上限由 activeWorkers 与运行时配置共同把关，使后台改并发可即时生效。
	activeWorkers atomic.Int64

	// 多实例选主（由 wire 通过 SetLeaderLock 注入）。三者缺席时
	// tryAcquireSingletonLeaderLock 会退化为「不设闸门」，单实例/单测照常跑。
	lockCache  LeaderLockCache
	lockDB     *sql.DB
	instanceID string
}

// scheduledMonitor 单个监控的运行时上下文。
type scheduledMonitor struct {
	id       int64
	name     string
	interval time.Duration
	jitter   time.Duration // 每轮 ± [0, jitter] 的均匀随机偏移；0 = 固定间隔
	cancel   context.CancelFunc
}

// nextDelay 计算下一次触发的等待时长：interval ± [0, jitter] 的均匀随机偏移。
// 校验链路已保证 interval - jitter >= monitorMinIntervalSeconds，
// 这里仍 clamp 一次下限，兜底数据库中违反约束的脏数据。
func (t *scheduledMonitor) nextDelay() time.Duration {
	if t.jitter <= 0 {
		return t.interval
	}
	offset := time.Duration(rand.Int64N(int64(2*t.jitter) + 1)) // [0, 2*jitter]
	d := t.interval - t.jitter + offset
	if floor := monitorMinIntervalSeconds * time.Second; d < floor {
		d = floor
	}
	return d
}

// NewChannelMonitorRunner 构造调度器。Start 在 wire 中调用一次。
// settingService 用于在每次 fire 前读取功能开关；传 nil 时视为总是启用（兼容测试）。
//
// pool 在构造时即建好：避免 Start 在 mu 内赋值、fire/Stop 在 mu 外读取的竞态隐患，
// 且 pond.NewPool 创建本身近似零开销，提前建池不会浪费资源。
func NewChannelMonitorRunner(svc *ChannelMonitorService, settingService *SettingService) *ChannelMonitorRunner {
	return newChannelMonitorRunner(svc, settingService)
}

// newChannelMonitorRunner 内部构造，接受最小化接口，便于单元测试注入 stub。
func newChannelMonitorRunner(svc monitorRunnerSvc, settingService *SettingService) *ChannelMonitorRunner {
	ctx, cancel := context.WithCancel(context.Background())
	return &ChannelMonitorRunner{
		svc:            svc,
		settingService: settingService,
		pool:           pond.NewPool(ChannelMonitorWorkerConcurrencyMax),
		parentCtx:      ctx,
		parentCancel:   cancel,
		tasks:          make(map[int64]*scheduledMonitor),
		inFlight:       make(map[int64]struct{}),
		instanceID:     uuid.NewString(),
	}
}

// SetLeaderLock 注入多实例选主依赖（与智力检测、上游成本采集同一套机制）。
// 走 setter 注入的理由与其它服务一致：lockCache/db 在 wire 图中较晚就绪。
func (r *ChannelMonitorRunner) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if r == nil {
		return
	}
	r.lockCache = lockCache
	r.lockDB = db
}

// Start 加载所有 enabled monitor 并为每个建立独立定时任务。
// 调用方需保证只调一次（wire ProvideChannelMonitorRunner 内只调一次）。
func (r *ChannelMonitorRunner) Start() {
	if r == nil || r.svc == nil {
		return
	}
	r.mu.Lock()
	if r.started || r.stopped {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), monitorStartupLoadTimeout)
	defer cancel()
	enabled, err := r.svc.ListEnabledMonitors(ctx)
	if err != nil {
		slog.Error("channel_monitor: load enabled monitors failed at startup", "error", err)
		return
	}
	for _, m := range enabled {
		r.Schedule(m)
	}
	slog.Info("channel_monitor: runner started", "scheduled_tasks", len(enabled))
}

// Schedule 为指定监控创建（或重置）独立定时任务。
//   - m.Enabled=false 或 APIKeyDecryptFailed=true → 等同于 Unschedule(m.ID)
//   - 已存在的任务会先被取消再重建（适用于 IntervalSeconds 变更场景）
//   - 新任务立即触发首次检测，之后按 IntervalSeconds 周期触发
func (r *ChannelMonitorRunner) Schedule(m *ChannelMonitor) {
	if r == nil || m == nil {
		return
	}
	if !m.Enabled || m.APIKeyDecryptFailed {
		r.Unschedule(m.ID)
		return
	}
	interval := time.Duration(m.IntervalSeconds) * time.Second
	if interval <= 0 {
		// Create/Update 已通过 validateInterval 校验区间，正常路径不可能到这里。
		// 真触发说明数据库中存在违反约束的数据或校验链路有 bug，记 Error 暴露问题。
		slog.Error("channel_monitor: skip schedule for invalid interval",
			"monitor_id", m.ID, "interval_seconds", m.IntervalSeconds)
		return
	}
	jitter := time.Duration(m.JitterSeconds) * time.Second
	if jitter < 0 {
		jitter = 0
	}

	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	if !r.started {
		// Start 之前调用 Schedule 通常意味着 wire 顺序错乱：
		// 当前 wire 顺序是 SetScheduler → Start，CRUD 钩子最早也只能在请求到达时触发，
		// 此时 Start 早已完成。出现此分支时把 monitor 信息打出来便于排查，
		// 不入队、不缓存——交给运维通过重启或修复 wire 解决。
		r.mu.Unlock()
		slog.Warn("channel_monitor: schedule before runner started, skip",
			"monitor_id", m.ID, "name", m.Name)
		return
	}
	if existing, ok := r.tasks[m.ID]; ok {
		existing.cancel()
	}
	ctx, cancel := context.WithCancel(r.parentCtx)
	task := &scheduledMonitor{
		id:       m.ID,
		name:     m.Name,
		interval: interval,
		jitter:   jitter,
		cancel:   cancel,
	}
	r.tasks[m.ID] = task
	r.wg.Add(1)
	r.mu.Unlock()

	go r.runScheduled(ctx, task)
}

// Unschedule 取消指定监控的定时任务（若存在）。
// 已经在执行中的检测会通过 ctx 取消信号传递。
func (r *ChannelMonitorRunner) Unschedule(id int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	task, ok := r.tasks[id]
	if ok {
		delete(r.tasks, id)
	}
	r.mu.Unlock()
	if ok {
		task.cancel()
	}
}

// Stop 优雅停止：取消所有任务、关闭池。
func (r *ChannelMonitorRunner) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	r.parentCancel()
	r.tasks = nil
	r.mu.Unlock()

	r.wg.Wait()
	r.pool.StopAndWait()
}

// runScheduled 单个监控的循环：立即触发首次（满足"新建/启用即跑"），
// 之后按 interval ± jitter 周期触发；ctx 取消即退出。
// 用 timer 而非 ticker：jitter > 0 时每轮等待时长都需要重新随机化。
func (r *ChannelMonitorRunner) runScheduled(ctx context.Context, task *scheduledMonitor) {
	defer r.wg.Done()

	r.fire(ctx, task)

	timer := time.NewTimer(task.nextDelay())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			r.fire(ctx, task)
			timer.Reset(task.nextDelay())
		}
	}
}

// fire 提交一次检测到 worker 池。功能开关关闭时跳过本次（不取消任务，
// 重新启用时立即恢复）；超过当前并发上限或重复在飞时也跳过。
//
// 【可调优】并发上限不再取自固定常量：每次 fire 都重读 ChannelMonitorRuntime，
// 因此管理员在后台把并发从 5 改到 10 后，无需重启进程，下一次触发即按新值执行。
func (r *ChannelMonitorRunner) fire(ctx context.Context, task *scheduledMonitor) {
	tuning := DefaultChannelMonitorTuning()
	if r.settingService != nil {
		rt := r.settingService.GetChannelMonitorRuntime(ctx)
		if !rt.ActiveProbesAllowed() {
			return
		}
		// 把本次生效的超时/保活参数推给 checker，供其按需重建 http client。
		tuning = ChannelMonitorTuning{
			WorkerConcurrency:            rt.WorkerConcurrency,
			ResponseHeaderTimeoutSeconds: int(rt.ResponseHeaderTimeout / time.Second),
			IdleConnTimeoutSeconds:       int(rt.IdleConnTimeout / time.Second),
			MaxIdleConnsPerHost:          rt.MaxIdleConnsPerHost,
			RequestTimeoutSeconds:        int(rt.RequestTimeout / time.Second),
		}
	}
	tuning = NormalizeChannelMonitorTuning(tuning)
	ApplyMonitorTuning(tuning)

	// 并发闸门：pool 容量固定为允许的最大值，实际并发由这里按当前配置把关。
	// 超限时直接跳过本轮（而非排队等待）——调度器每 interval 会再次触发，
	// 排队只会让"上次检测时间"失真。
	if !r.tryAcquireWorkerSlot(tuning.WorkerConcurrency) {
		slog.Debug("channel_monitor: worker concurrency reached, skip this round",
			"monitor_id", task.id, "name", task.name, "limit", tuning.WorkerConcurrency)
		return
	}

	if !r.tryAcquireInFlight(task.id) {
		r.releaseWorkerSlot()
		slog.Debug("channel_monitor: skip already in-flight",
			"monitor_id", task.id, "name", task.name)
		return
	}
	if _, ok := r.pool.TrySubmit(func() {
		defer r.releaseWorkerSlot()
		r.runOne(task.id, task.name)
	}); !ok {
		// 池满：丢弃本次检测，但必须释放已占用的槽，否则该 monitor 会被永久卡住。
		r.releaseWorkerSlot()
		r.releaseInFlight(task.id)
		slog.Warn("channel_monitor: worker pool full, skip submission",
			"monitor_id", task.id, "name", task.name)
	}
}

// tryAcquireWorkerSlot 在不超过 limit 的前提下占用一个并发槽。
func (r *ChannelMonitorRunner) tryAcquireWorkerSlot(limit int) bool {
	if limit < 1 {
		limit = ChannelMonitorWorkerConcurrencyDefault
	}
	for {
		cur := r.activeWorkers.Load()
		if int(cur) >= limit {
			return false
		}
		if r.activeWorkers.CompareAndSwap(cur, cur+1) {
			return true
		}
	}
}

// releaseWorkerSlot 释放并发槽。
func (r *ChannelMonitorRunner) releaseWorkerSlot() {
	r.activeWorkers.Add(-1)
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

// runOne 执行单个监控的检测。普通错误只记日志；API key 解密失败会撤销任务。
// 任务结束时（含 panic recover）必须释放 in-flight 槽。
func (r *ChannelMonitorRunner) runOne(id int64, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorRequestTimeout+monitorPingTimeout+monitorRunOneBuffer)
	defer cancel()

	defer r.releaseInFlight(id)

	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("channel_monitor: runner panic",
				"monitor_id", id, "name", name, "panic", rec)
		}
	}()

	// 多实例选主：同一条监控同一时刻只允许一个实例真正发探针。
	// 探针会真花钱、真写使用记录，两个实例各探一遍就是重复消费。
	release, acquired := tryAcquireSingletonLeaderLock(ctx, r.lockCache, r.lockDB,
		channelMonitorProbeLockKeyPrefix+strconv.FormatInt(id, 10), r.instanceID, channelMonitorProbeLockTTL)
	if !acquired {
		slog.Debug("channel_monitor: probe lock held by another instance, skip",
			"monitor_id", id, "name", name)
		return
	}
	defer release()

	if _, err := r.svc.RunCheck(ctx, id); err != nil {
		if errors.Is(err, ErrChannelMonitorAPIKeyDecryptFailed) {
			r.Unschedule(id)
		}
		slog.Warn("channel_monitor: run check failed",
			"monitor_id", id, "name", name, "error", err)
	}
}

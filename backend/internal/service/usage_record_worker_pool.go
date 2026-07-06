package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/alitto/pond/v2"
	"go.uber.org/zap"
)

const (
	defaultUsageRecordWorkerCount        = 128
	defaultUsageRecordQueueSize          = 16384
	defaultUsageRecordTaskTimeoutSeconds = 5
	// 默认 sync：溢出时提交方内联执行，保证计费任务不被静默丢弃（issue #3656）。
	defaultUsageRecordOverflowPolicy       = config.UsageRecordOverflowPolicySync
	defaultUsageRecordOverflowSampleRatio  = 10
	defaultUsageRecordAutoScaleEnabled     = true
	defaultUsageRecordAutoScaleMinWorkers  = 128
	defaultUsageRecordAutoScaleMaxWorkers  = 512
	defaultUsageRecordAutoScaleUpPercent   = 70
	defaultUsageRecordAutoScaleDownPercent = 15
	defaultUsageRecordAutoScaleUpStep      = 32
	defaultUsageRecordAutoScaleDownStep    = 16
	defaultUsageRecordAutoScaleInterval    = 3 * time.Second
	defaultUsageRecordAutoScaleCooldown    = 10 * time.Second
	usageRecordDropLogInterval             = 5 * time.Second
)

// UsageRecordTask 是提交到使用量记录池的任务。
// 任务实现应自行处理业务错误日志；池本身只负责调度与超时控制。
type UsageRecordTask func(ctx context.Context)

// UsageRecordSubmitMode 表示任务提交结果。
type UsageRecordSubmitMode string

const (
	UsageRecordSubmitModeEnqueued UsageRecordSubmitMode = "enqueued"
	UsageRecordSubmitModeDropped  UsageRecordSubmitMode = "dropped"
	UsageRecordSubmitModeSync     UsageRecordSubmitMode = "sync_fallback"
)

// UsageRecordWorkerPoolOptions 使用量记录池配置。
type UsageRecordWorkerPoolOptions struct {
	WorkerCount           int
	QueueSize             int
	TaskTimeout           time.Duration
	OverflowPolicy        string
	OverflowSamplePercent int
	AutoScaleEnabled      bool
	AutoScaleMinWorkers   int
	AutoScaleMaxWorkers   int
	AutoScaleUpPercent    int
	AutoScaleDownPercent  int
	AutoScaleUpStep       int
	AutoScaleDownStep     int
	AutoScaleInterval     time.Duration
	AutoScaleCooldown     time.Duration
REDACTED

// UsageRecordWorkerPoolStats 使用量记录池运行时统计。
type UsageRecordWorkerPoolStats struct {
	MaxConcurrency     int
	RunningWorkers     int64
	WaitingTasks       uint64
	SubmittedTasks     uint64
	CompletedTasks     uint64
	SuccessfulTasks    uint64
	FailedTasks        uint64
	DroppedTasks       uint64
	DroppedQueueFull   uint64
	DroppedPoolStopped uint64
	SyncFallbackTasks  uint64
REDACTED

// UsageRecordWorkerPool 提供“有界队列 + 固定 worker”的异步执行器。
// 用于替代请求路径里的直接 goroutine，避免高并发时无界堆积。
type UsageRecordWorkerPool struct {
	pool                  pond.Pool
	taskTimeout           time.Duration
	overflowPolicy        string
	overflowSamplePercent int
	overflowCounter       atomic.Uint64
	droppedQueueFull      atomic.Uint64
	droppedPoolStopped    atomic.Uint64
	syncFallback          atomic.Uint64
	lastDropLogNanos      atomic.Int64
	autoScaleEnabled      bool
	autoScaleMinWorkers   int
	autoScaleMaxWorkers   int
	autoScaleUpPercent    int
	autoScaleDownPercent  int
	autoScaleUpStep       int
	autoScaleDownStep     int
	autoScaleInterval     time.Duration
	autoScaleCooldown     time.Duration
	lastScaleNanos        atomic.Int64
	autoScaleCancel       context.CancelFunc
	lifecycleWg           sync.WaitGroup
	stopOnce              sync.Once
REDACTED

// NewUsageRecordWorkerPool 从配置构建使用量记录池。
func NewUsageRecordWorkerPool(cfg *config.Config) *UsageRecordWorkerPool {
	opts := usageRecordPoolOptionsFromConfig(cfg)
	return NewUsageRecordWorkerPoolWithOptions(opts)
REDACTED

// NewUsageRecordWorkerPoolWithOptions 根据给定参数构建使用量记录池。
func NewUsageRecordWorkerPoolWithOptions(opts UsageRecordWorkerPoolOptions) *UsageRecordWorkerPool {
	opts = normalizeUsageRecordPoolOptions(opts)

	p := &UsageRecordWorkerPool{
		taskTimeout:           opts.TaskTimeout,
		overflowPolicy:        opts.OverflowPolicy,
		overflowSamplePercent: opts.OverflowSamplePercent,
		autoScaleEnabled:      opts.AutoScaleEnabled,
		autoScaleMinWorkers:   opts.AutoScaleMinWorkers,
		autoScaleMaxWorkers:   opts.AutoScaleMaxWorkers,
		autoScaleUpPercent:    opts.AutoScaleUpPercent,
		autoScaleDownPercent:  opts.AutoScaleDownPercent,
		autoScaleUpStep:       opts.AutoScaleUpStep,
		autoScaleDownStep:     opts.AutoScaleDownStep,
		autoScaleInterval:     opts.AutoScaleInterval,
		autoScaleCooldown:     opts.AutoScaleCooldown,
REDACTED

	p.pool = pond.NewPool(
		opts.WorkerCount,
		pond.WithQueueSize(opts.QueueSize),
	)
	if p.autoScaleEnabled {
		p.startAutoScaler()
REDACTED
	return p
REDACTED

// Submit 提交一个使用量记录任务。
// 提交失败（队列满）时按 overflowPolicy 执行降级策略：drop/sample/sync。
func (p *UsageRecordWorkerPool) Submit(task UsageRecordTask) UsageRecordSubmitMode {
	if p == nil || task == nil {
		return UsageRecordSubmitModeDropped
REDACTED
	if p.pool == nil || p.pool.Stopped() {
		p.droppedPoolStopped.Add(1)
		p.logDrop("stopped")
		return UsageRecordSubmitModeDropped
REDACTED

	_, ok := p.pool.TrySubmit(func() {
		p.execute(task)
REDACTED)
	if ok {
		return UsageRecordSubmitModeEnqueued
REDACTED

	if p.pool.Stopped() {
		p.droppedPoolStopped.Add(1)
		p.logDrop("stopped")
		return UsageRecordSubmitModeDropped
REDACTED

	switch p.overflowPolicy {
	case config.UsageRecordOverflowPolicySync:
		p.syncFallback.Add(1)
		p.execute(task)
		return UsageRecordSubmitModeSync
	case config.UsageRecordOverflowPolicySample:
		if p.shouldSyncFallback() {
			p.syncFallback.Add(1)
			p.execute(task)
			return UsageRecordSubmitModeSync
	REDACTED
REDACTED

	p.droppedQueueFull.Add(1)
	p.logDrop("full")
	return UsageRecordSubmitModeDropped
REDACTED

// Stats 返回当前池状态与计数器。
func (p *UsageRecordWorkerPool) Stats() UsageRecordWorkerPoolStats {
	if p == nil || p.pool == nil {
		return UsageRecordWorkerPoolStats{REDACTED
REDACTED
	return UsageRecordWorkerPoolStats{
		MaxConcurrency:     p.pool.MaxConcurrency(),
		RunningWorkers:     p.pool.RunningWorkers(),
		WaitingTasks:       p.pool.WaitingTasks(),
		SubmittedTasks:     p.pool.SubmittedTasks(),
		CompletedTasks:     p.pool.CompletedTasks(),
		SuccessfulTasks:    p.pool.SuccessfulTasks(),
		FailedTasks:        p.pool.FailedTasks(),
		DroppedTasks:       p.pool.DroppedTasks(),
		DroppedQueueFull:   p.droppedQueueFull.Load(),
		DroppedPoolStopped: p.droppedPoolStopped.Load(),
		SyncFallbackTasks:  p.syncFallback.Load(),
REDACTED
REDACTED

// Stop 停止池并等待队列任务完成。
func (p *UsageRecordWorkerPool) Stop() {
	if p == nil || p.pool == nil {
		return
REDACTED
	p.stopOnce.Do(func() {
		if p.autoScaleCancel != nil {
			p.autoScaleCancel()
	REDACTED
		p.lifecycleWg.Wait()
		p.pool.StopAndWait()
REDACTED)
REDACTED

func (p *UsageRecordWorkerPool) startAutoScaler() {
	ctx, cancel := context.WithCancel(context.Background())
	p.autoScaleCancel = cancel

	p.lifecycleWg.Add(1)
	go func() {
		defer p.lifecycleWg.Done()

		ticker := time.NewTicker(p.autoScaleInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.autoScaleTick()
		REDACTED
	REDACTED
REDACTED()
REDACTED

func (p *UsageRecordWorkerPool) autoScaleTick() {
	if p == nil || p.pool == nil || p.pool.Stopped() {
		return
REDACTED
	queueSize := p.pool.QueueSize()
	if queueSize <= 0 {
		return
REDACTED
	current := p.pool.MaxConcurrency()
	waiting := int(p.pool.WaitingTasks())
	running := int(p.pool.RunningWorkers())
	if current <= 0 || waiting < 0 {
		return
REDACTED
	queuePercent := waiting * 100 / queueSize
	runningPercent := 0
	if current > 0 {
		runningPercent = running * 100 / current
REDACTED

	now := time.Now()
	lastScaleNanos := p.lastScaleNanos.Load()
	if lastScaleNanos > 0 && now.Sub(time.Unix(0, lastScaleNanos)) < p.autoScaleCooldown {
		return
REDACTED

	// 扩容优先：队列占用率超过阈值时，按步长提升并发上限。
	if queuePercent >= p.autoScaleUpPercent && current < p.autoScaleMaxWorkers {
		target := current + p.autoScaleUpStep
		if target > p.autoScaleMaxWorkers {
			target = p.autoScaleMaxWorkers
	REDACTED
		p.resizePool(current, target, queuePercent, waiting, runningPercent, queueSize, "scale_up")
		return
REDACTED

	// 缩容：仅在队列为空且运行利用率低时收缩，避免高负载下“无排队误缩容”导致震荡。
	if queuePercent <= p.autoScaleDownPercent && waiting == 0 &&
		runningPercent <= p.autoScaleDownPercent &&
		current > p.autoScaleMinWorkers {
		target := current - p.autoScaleDownStep
		if target < p.autoScaleMinWorkers {
			target = p.autoScaleMinWorkers
	REDACTED
		p.resizePool(current, target, queuePercent, waiting, runningPercent, queueSize, "scale_down")
REDACTED
REDACTED

func (p *UsageRecordWorkerPool) resizePool(current, target, queuePercent, waiting, runningPercent, queueSize int, action string) {
	if target == current {
		return
REDACTED
	p.pool.Resize(target)
	p.lastScaleNanos.Store(time.Now().UnixNano())

	logger.L().With(
		zap.String("component", "service.usage_record_worker_pool"),
		zap.String("action", action),
		zap.Int("from_workers", current),
		zap.Int("to_workers", target),
		zap.Int("queue_percent", queuePercent),
		zap.Int("waiting_tasks", waiting),
		zap.Int("running_percent", runningPercent),
		zap.Int("queue_size", queueSize),
	).Info("usage_record.auto_scale")
REDACTED

func (p *UsageRecordWorkerPool) shouldSyncFallback() bool {
	if p.overflowSamplePercent <= 0 {
		return false
REDACTED
	n := p.overflowCounter.Add(1)
	return int((n-1)%100) < p.overflowSamplePercent
REDACTED

func (p *UsageRecordWorkerPool) execute(task UsageRecordTask) {
	ctx, cancel := context.WithTimeout(context.Background(), p.taskTimeout)
	defer cancel()

	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().With(
				zap.String("component", "service.usage_record_worker_pool"),
				zap.Any("panic", recovered),
			).Error("usage_record.task_panic")
	REDACTED
REDACTED()

	task(ctx)
REDACTED

func (p *UsageRecordWorkerPool) logDrop(reason string) {
	now := time.Now().UnixNano()
	last := p.lastDropLogNanos.Load()
	if now-last < int64(usageRecordDropLogInterval) {
		return
REDACTED
	if !p.lastDropLogNanos.CompareAndSwap(last, now) {
		return
REDACTED

	stats := p.Stats()
	logger.L().With(
		zap.String("component", "service.usage_record_worker_pool"),
		zap.String("reason", reason),
		zap.String("overflow_policy", p.overflowPolicy),
		zap.Int64("running_workers", stats.RunningWorkers),
		zap.Uint64("waiting_tasks", stats.WaitingTasks),
		zap.Uint64("dropped_queue_full", stats.DroppedQueueFull),
		zap.Uint64("dropped_pool_stopped", stats.DroppedPoolStopped),
		zap.Uint64("sync_fallback_tasks", stats.SyncFallbackTasks),
	).Warn("usage_record.task_dropped")
REDACTED

func usageRecordPoolOptionsFromConfig(cfg *config.Config) UsageRecordWorkerPoolOptions {
	opts := UsageRecordWorkerPoolOptions{
		WorkerCount:           defaultUsageRecordWorkerCount,
		QueueSize:             defaultUsageRecordQueueSize,
		TaskTimeout:           time.Duration(defaultUsageRecordTaskTimeoutSeconds) * time.Second,
		OverflowPolicy:        defaultUsageRecordOverflowPolicy,
		OverflowSamplePercent: defaultUsageRecordOverflowSampleRatio,
		AutoScaleEnabled:      defaultUsageRecordAutoScaleEnabled,
		AutoScaleMinWorkers:   defaultUsageRecordAutoScaleMinWorkers,
		AutoScaleMaxWorkers:   defaultUsageRecordAutoScaleMaxWorkers,
		AutoScaleUpPercent:    defaultUsageRecordAutoScaleUpPercent,
		AutoScaleDownPercent:  defaultUsageRecordAutoScaleDownPercent,
		AutoScaleUpStep:       defaultUsageRecordAutoScaleUpStep,
		AutoScaleDownStep:     defaultUsageRecordAutoScaleDownStep,
		AutoScaleInterval:     defaultUsageRecordAutoScaleInterval,
		AutoScaleCooldown:     defaultUsageRecordAutoScaleCooldown,
REDACTED
	if cfg == nil {
		return opts
REDACTED
	if cfg.Gateway.UsageRecord.WorkerCount > 0 {
		opts.WorkerCount = cfg.Gateway.UsageRecord.WorkerCount
REDACTED
	if cfg.Gateway.UsageRecord.QueueSize > 0 {
		opts.QueueSize = cfg.Gateway.UsageRecord.QueueSize
REDACTED
	if cfg.Gateway.UsageRecord.TaskTimeoutSeconds > 0 {
		opts.TaskTimeout = time.Duration(cfg.Gateway.UsageRecord.TaskTimeoutSeconds) * time.Second
REDACTED
	if policy := strings.TrimSpace(strings.ToLower(cfg.Gateway.UsageRecord.OverflowPolicy)); policy != "" {
		opts.OverflowPolicy = policy
REDACTED
	if cfg.Gateway.UsageRecord.OverflowSamplePercent >= 0 {
		opts.OverflowSamplePercent = cfg.Gateway.UsageRecord.OverflowSamplePercent
REDACTED
	opts.AutoScaleEnabled = cfg.Gateway.UsageRecord.AutoScaleEnabled
	if cfg.Gateway.UsageRecord.AutoScaleMinWorkers > 0 {
		opts.AutoScaleMinWorkers = cfg.Gateway.UsageRecord.AutoScaleMinWorkers
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleMaxWorkers > 0 {
		opts.AutoScaleMaxWorkers = cfg.Gateway.UsageRecord.AutoScaleMaxWorkers
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleUpQueuePercent > 0 {
		opts.AutoScaleUpPercent = cfg.Gateway.UsageRecord.AutoScaleUpQueuePercent
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleDownQueuePercent >= 0 {
		opts.AutoScaleDownPercent = cfg.Gateway.UsageRecord.AutoScaleDownQueuePercent
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleUpStep > 0 {
		opts.AutoScaleUpStep = cfg.Gateway.UsageRecord.AutoScaleUpStep
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleDownStep > 0 {
		opts.AutoScaleDownStep = cfg.Gateway.UsageRecord.AutoScaleDownStep
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds > 0 {
		opts.AutoScaleInterval = time.Duration(cfg.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds) * time.Second
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleCooldownSeconds >= 0 {
		opts.AutoScaleCooldown = time.Duration(cfg.Gateway.UsageRecord.AutoScaleCooldownSeconds) * time.Second
REDACTED
	return normalizeUsageRecordPoolOptions(opts)
REDACTED

func normalizeUsageRecordPoolOptions(opts UsageRecordWorkerPoolOptions) UsageRecordWorkerPoolOptions {
	if opts.WorkerCount <= 0 {
		opts.WorkerCount = defaultUsageRecordWorkerCount
REDACTED
	if opts.QueueSize <= 0 {
		opts.QueueSize = defaultUsageRecordQueueSize
REDACTED
	if opts.TaskTimeout <= 0 {
		opts.TaskTimeout = time.Duration(defaultUsageRecordTaskTimeoutSeconds) * time.Second
REDACTED
	switch strings.ToLower(strings.TrimSpace(opts.OverflowPolicy)) {
	case config.UsageRecordOverflowPolicyDrop,
		config.UsageRecordOverflowPolicySample,
		config.UsageRecordOverflowPolicySync:
		opts.OverflowPolicy = strings.ToLower(strings.TrimSpace(opts.OverflowPolicy))
	default:
		opts.OverflowPolicy = defaultUsageRecordOverflowPolicy
REDACTED
	if opts.OverflowSamplePercent < 0 {
		opts.OverflowSamplePercent = 0
REDACTED
	if opts.OverflowSamplePercent > 100 {
		opts.OverflowSamplePercent = 100
REDACTED
	if opts.OverflowPolicy == config.UsageRecordOverflowPolicySample && opts.OverflowSamplePercent == 0 {
		opts.OverflowSamplePercent = defaultUsageRecordOverflowSampleRatio
REDACTED
	if opts.AutoScaleEnabled {
		if opts.AutoScaleMinWorkers <= 0 {
			opts.AutoScaleMinWorkers = defaultUsageRecordAutoScaleMinWorkers
	REDACTED
		if opts.AutoScaleMaxWorkers <= 0 {
			opts.AutoScaleMaxWorkers = defaultUsageRecordAutoScaleMaxWorkers
	REDACTED
		if opts.AutoScaleMaxWorkers < opts.AutoScaleMinWorkers {
			opts.AutoScaleMaxWorkers = opts.AutoScaleMinWorkers
	REDACTED
		if opts.WorkerCount < opts.AutoScaleMinWorkers {
			opts.WorkerCount = opts.AutoScaleMinWorkers
	REDACTED
		if opts.WorkerCount > opts.AutoScaleMaxWorkers {
			opts.WorkerCount = opts.AutoScaleMaxWorkers
	REDACTED
		if opts.AutoScaleUpPercent <= 0 || opts.AutoScaleUpPercent > 100 {
			opts.AutoScaleUpPercent = defaultUsageRecordAutoScaleUpPercent
	REDACTED
		if opts.AutoScaleDownPercent < 0 || opts.AutoScaleDownPercent >= 100 {
			opts.AutoScaleDownPercent = defaultUsageRecordAutoScaleDownPercent
	REDACTED
		if opts.AutoScaleDownPercent >= opts.AutoScaleUpPercent {
			opts.AutoScaleDownPercent = max(0, opts.AutoScaleUpPercent/2)
	REDACTED
		if opts.AutoScaleUpStep <= 0 {
			opts.AutoScaleUpStep = defaultUsageRecordAutoScaleUpStep
	REDACTED
		if opts.AutoScaleDownStep <= 0 {
			opts.AutoScaleDownStep = defaultUsageRecordAutoScaleDownStep
	REDACTED
		if opts.AutoScaleInterval <= 0 {
			opts.AutoScaleInterval = defaultUsageRecordAutoScaleInterval
	REDACTED
		if opts.AutoScaleCooldown < 0 {
			opts.AutoScaleCooldown = defaultUsageRecordAutoScaleCooldown
	REDACTED
REDACTED else {
		opts.AutoScaleMinWorkers = opts.WorkerCount
		opts.AutoScaleMaxWorkers = opts.WorkerCount
REDACTED
	return opts
REDACTED

func (m UsageRecordSubmitMode) String() string {
	return string(m)
REDACTED

func (s UsageRecordWorkerPoolStats) String() string {
	return fmt.Sprintf("running=%d waiting=%d submitted=%d dropped=%d", s.RunningWorkers, s.WaitingTasks, s.SubmittedTasks, s.DroppedTasks)
REDACTED

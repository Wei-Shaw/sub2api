//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 本文件覆盖「兜底补账」这一层：窗口与上限怎么算、错误怎么传、
// 以及采集器把它接进 cron 时不会因为配置写错而把取数主循环拖停。
//
// 补账本身不做任何查询、不碰任何金额，它唯一的动作就是「把用尽的记录
// 重新排回队列」。所以这里断言的重点是**传给仓储的参数**：
// 窗口算错会漏掉或误捞记录，上限算错会把还在重试的进度清掉。

func newRequeueService(repo UpstreamCostRepository, cfg UpstreamCostCollectorConfig) *UpstreamCostService {
	return NewUpstreamCostService(repo, nil, nil, cfg)
}

func TestUpstreamCostRequeueExhaustedPassesLookbackWindow(t *testing.T) {
	repo := newUpstreamCostRepoStub()
	repo.requeueResult = 16

	cfg := UpstreamCostCollectorConfig{SweepLookback: 72 * time.Hour, SweepBatchSize: 500}
	svc := newRequeueService(repo, cfg)

	now := time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC)
	requeued, err := svc.RequeueExhausted(context.Background(), now)

	require.NoError(t, err)
	require.EqualValues(t, 16, requeued)

	calls := repo.requeueCallsSnapshot()
	require.Len(t, calls, 1)
	require.Equal(t, now, calls[0].until, "窗口右端必须是本次执行时刻")
	require.Equal(t, now.Add(-72*time.Hour), calls[0].since, "窗口左端必须是执行时刻减回看长度")
	require.Equal(t, 500, calls[0].limit, "上限必须透传到仓储")
}

// 上限与判定阈值都必须与取数时用的是**同一个** MaxAttempts：
// 若两者不一致，重排出来的记录会立刻又被判定为「已用尽」，
// 补账看起来跑了、实际什么都没发生。
func TestUpstreamCostRequeueExhaustedUsesSameMaxAttemptsAsCollection(t *testing.T) {
	repo := newUpstreamCostRepoStub()
	cfg := UpstreamCostCollectorConfig{}
	svc := newRequeueService(repo, cfg)

	_, err := svc.RequeueExhausted(context.Background(), time.Now().UTC())
	require.NoError(t, err)

	calls := repo.requeueCallsSnapshot()
	require.Len(t, calls, 1)
	require.Equal(t, cfg.Normalize().MaxAttempts, calls[0].maxAttempts,
		"补账的「已用尽」阈值必须与取数一致")
	require.Equal(t, 60, calls[0].maxAttempts, "默认阈值应为 60 次")
}

func TestUpstreamCostRequeueExhaustedPropagatesError(t *testing.T) {
	repo := newUpstreamCostRepoStub()
	repo.requeueErr = errors.New("db down")

	svc := newRequeueService(repo, UpstreamCostCollectorConfig{})
	requeued, err := svc.RequeueExhausted(context.Background(), time.Now().UTC())

	require.Error(t, err, "补账失败必须上报，不能伪装成「补了 0 条」")
	require.Zero(t, requeued)
}

// lookback / batch 为 0 意味着「补账参数不自洽」：宁可不做，也不能拿
// 0 长度窗口或 0 上限去跑一条注定筛不出东西的语句——那会伪装成
// 「补账跑了但没东西可补」，让配置写错变得难以发现。
//
// 注意这里刻意**绕过构造函数**直接组装：构造函数内部会调 cfg.Normalize()，
// 它保证「构造出来的调度参数一定可用」，所以从构造函数走永远看不到 0。
// 这条守卫防的是「有人直接组装了一个 UpstreamCostService」这条旁路。
func TestUpstreamCostRequeueExhaustedSkipsWhenUnconfigured(t *testing.T) {
	now := time.Now().UTC()

	for name, cfg := range map[string]UpstreamCostCollectorConfig{
		"窗口为零":  {SweepLookback: 0, SweepBatchSize: 500, MaxAttempts: 60},
		"上限为零":  {SweepLookback: 72 * time.Hour, SweepBatchSize: 0, MaxAttempts: 60},
		"两者都为零": {SweepLookback: 0, SweepBatchSize: 0, MaxAttempts: 60},
	} {
		t.Run(name, func(t *testing.T) {
			repo := newUpstreamCostRepoStub()
			svc := &UpstreamCostService{repo: repo, cfg: cfg}

			requeued, err := svc.RequeueExhausted(context.Background(), now)
			require.NoError(t, err)
			require.Zero(t, requeued)
			require.Empty(t, repo.requeueCallsSnapshot(), "参数不自洽时一个仓储调用都不该发出去")
		})
	}
}

// 反向钉住上一条：走构造函数时 Normalize 会把缺省值补齐，
// 于是补账**一定**会跑起来——「没配置」不应该被误解成「补账关闭」。
// 关闭补账的唯一方式是显式把 SweepSchedule 置空，而不是把窗口留成 0。
func TestUpstreamCostRequeueExhaustedRunsWithNormalizedDefaults(t *testing.T) {
	repo := newUpstreamCostRepoStub()
	svc := NewUpstreamCostService(repo, nil, nil, UpstreamCostCollectorConfig{})

	requeued, err := svc.RequeueExhausted(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	require.Zero(t, requeued)

	calls := repo.requeueCallsSnapshot()
	require.Len(t, calls, 1, "构造函数路径必须补齐缺省值并正常发起补账")
	require.Equal(t, 72*time.Hour, calls[0].until.Sub(calls[0].since))
	require.Equal(t, 500, calls[0].limit)
}

func TestUpstreamCostRequeueExhaustedIsNilSafe(t *testing.T) {
	var svc *UpstreamCostService
	requeued, err := svc.RequeueExhausted(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	require.Zero(t, requeued)

	// 仓储为 nil 时同样直接返回，不 panic。
	svcNoRepo := &UpstreamCostService{cfg: UpstreamCostCollectorConfig{}.Normalize()}
	requeued, err = svcNoRepo.RequeueExhausted(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	require.Zero(t, requeued)
}

// Normalize 的默认值是这个机制的「出厂设定」，改它等于改线上行为，
// 所以逐个钉住：重试预算 60 次（≈31.5 分钟）、回看 3 天、单次封顶 500 条。
func TestUpstreamCostCollectorConfigSweepDefaults(t *testing.T) {
	cfg := UpstreamCostCollectorConfig{}.Normalize()

	require.Equal(t, 60, cfg.MaxAttempts, "重试预算默认 60 次（≈31.5 分钟）")
	require.Equal(t, 30*time.Second, cfg.Interval)
	require.Equal(t, 90*time.Second, cfg.FirstDelay)
	require.Equal(t, 200, cfg.BatchSize)
	require.Equal(t, 72*time.Hour, cfg.SweepLookback, "补账默认回看 3 天")
	require.Equal(t, 500, cfg.SweepBatchSize, "补账单次默认封顶 500 条")
	require.Equal(t, 5*time.Minute, cfg.SweepStartupDelay)

	// 空串是「关闭补账」的显式表达，Normalize 绝不能把它补成默认值，
	// 否则「显式关掉」这件事将变得不可能。
	require.Empty(t, cfg.SweepSchedule, "Normalize 不得为补账时刻补默认值")
	require.Equal(t, "0 6 * * *", upstreamCostDefaultSweepSchedule, "默认补账时刻为每天 06:00")
}

// TestUpstreamCostCollectorRunSweepInvokesService 覆盖 cron 真正调用的那个函数。
func TestUpstreamCostCollectorRunSweepInvokesService(t *testing.T) {
	repo := newUpstreamCostRepoStub()
	repo.requeueResult = 3

	svc := newRequeueService(repo, UpstreamCostCollectorConfig{})
	collector := NewUpstreamCostCollector(svc, UpstreamCostCollectorConfig{})

	// 未注入 leader 锁依赖时（单机/测试）恒为可执行，不需要额外打桩。
	collector.runSweep()

	calls := repo.requeueCallsSnapshot()
	require.Len(t, calls, 1, "runSweep 必须恰好触发一次补账")
	require.Equal(t, 72*time.Hour, calls[0].until.Sub(calls[0].since), "回看长度应为 3 天")
}

// Stop 之后再触发补账必须什么都不做：cron 已停，但万一有在途回调，
// 也不能在进程关停途中继续写库。
func TestUpstreamCostCollectorRunSweepNoopAfterStop(t *testing.T) {
	repo := newUpstreamCostRepoStub()
	svc := newRequeueService(repo, UpstreamCostCollectorConfig{})

	collector := NewUpstreamCostCollector(svc, UpstreamCostCollectorConfig{Enabled: true})
	collector.Stop()
	collector.runSweep()

	require.Empty(t, repo.requeueCallsSnapshot(), "关停后不得再动数据库")
}

// 时刻表达式写错时只记日志、不建 cron，且绝不能 panic——
// 补账是兜底层，它配错了可以暂时不生效，但不能把取数主循环一起带走。
func TestUpstreamCostCollectorStartSweepCronRejectsBadSchedule(t *testing.T) {
	for name, schedule := range map[string]string{
		"表达式非法": "不是 cron",
		"段数不足":  "0 6 *",
	} {
		t.Run(name, func(t *testing.T) {
			collector := NewUpstreamCostCollector(nil, UpstreamCostCollectorConfig{SweepSchedule: schedule})
			require.NotPanics(t, func() { collector.startSweepCronLocked() })
			require.Nil(t, collector.sweepCron, "非法表达式不得建立 cron")
		})
	}
}

func TestUpstreamCostCollectorStartSweepCronDisabledWhenEmpty(t *testing.T) {
	collector := NewUpstreamCostCollector(nil, UpstreamCostCollectorConfig{SweepSchedule: "  "})
	require.NotPanics(t, func() { collector.startSweepCronLocked() })
	require.Nil(t, collector.sweepCron, "空白表达式等于关闭补账")
}

func TestUpstreamCostCollectorStartSweepCronAndStop(t *testing.T) {
	collector := NewUpstreamCostCollector(nil, UpstreamCostCollectorConfig{SweepSchedule: "0 6 * * *"})

	collector.mu.Lock()
	collector.startSweepCronLocked()
	collector.mu.Unlock()
	require.NotNil(t, collector.sweepCron, "合法表达式应建立 cron")

	collector.started = true
	require.NotPanics(t, collector.Stop, "Stop 必须能干净地关掉补账 cron")
	require.Nil(t, collector.sweepCron, "Stop 后 cron 引用必须清空")
}

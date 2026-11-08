package service

import (
	"database/sql"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// ProvideUpstreamCostCollector 构造并启动进程级的上游成本取数采集器。
//
// 开关沿用对账配置里的总开关（cfg.Reconciliation.Enabled）：它决定的正是
// 「要不要向 A6 取数」，而取数凭据、站点地址、换算汇率也都来自同一套 A6 设置。
// 刻意不再引入第二个开关——多一个开关只会多出一种「配置都填了但功能没开」的困惑。
//
// 调度参数（扫描间隔、首次延迟、重试上限、每轮处理上限、补账窗口与封顶）不给配置项，
// 直接用实测数据定下的缺省值：账单落库延迟 P50≈13s / P90≈66s / P99≈599s / 最大 600s，
// 对应「90 秒首查 + 每 30 秒扫一轮 + 最多 60 次（≈31.5 分钟）+ 每轮最多 200 条」，
// 外加「每天 06:00 补一次账 + 回看 3 天 + 单次最多重排 500 条」。
// 这些数字将来要改只有一个理由——上游落库延迟变了；那时应该连同实测一起改，
// 而不是让运维去面板上凭感觉调一个说不出所以然的秒数。
// 实际生效值会在启动日志里留痕，便于事后核对。
//
// 唯一在这一层显式给出的是**补账时刻**：它属于运维偏好（几点跑不打扰业务），
// 不是从实测延迟推导出来的量，所以不藏在 Normalize 的缺省值里，
// 而是写到这个一眼能看见的地方，改时刻时也只改这一处。
func ProvideUpstreamCostCollector(
	repo UpstreamCostRepository,
	client *A6Client,
	settings *ReconciliationA6SettingsService,
	lockCache LeaderLockCache,
	db *sql.DB,
	cfg *config.Config,
) *UpstreamCostCollector {
	collectorCfg := UpstreamCostCollectorConfig{
		Enabled:       cfg.Reconciliation.Enabled,
		SweepSchedule: upstreamCostDefaultSweepSchedule,
	}
	svc := NewUpstreamCostService(repo, client, settings, collectorCfg)
	collector := NewUpstreamCostCollector(svc, collectorCfg)
	collector.SetLeaderLock(lockCache, db)
	collector.Start()
	return collector
}

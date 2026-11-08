package service

import (
	"context"
	"math"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// 本文件保存「经营对账」模块删除后，A6 生效凭据服务仍然需要的几个最小定义。
//
// 这些东西原先散落在 reconciliation_types.go（仓储接口）与 reconciliation_ops.go
// （汇率覆盖的读写助手）里，随模块一起被删；但 A6 设置的读取路径依赖它们：
// 汇率允许由管理面板写进数据库做运行时覆盖，覆盖值就存在 reconciliation_sync_state
// 表的 fx_usd_cny_rate_override 键上。
//
// 只搬运不重写——行为与被删版本逐字保持一致，避免「清理顺手改了行为」这类
// 无法从 diff 中一眼看出的回归。唯一的变化是把它们集中到本文件并补上说明。
//
// 注意本文件**不包含** ReconciliationSyncService 上的 EffectiveFxRate 方法：
// 那是对账同步服务的对外能力，随模块删除；上游成本取数用的是
// ReconciliationA6SettingsService.Effective(ctx).FxUSDCNYRate，走的是下面这套同样的
// 覆盖逻辑，因此汇率语义没有任何变化。

// ReconciliationSyncStateRepository 同步状态的读写。
//
// 表名与接口名沿用历史命名（该表本是为对账模块建的），现在实际承载两类数据：
// A6 连接凭据的面板覆盖值（a6_*_override），以及汇率的运行时覆盖值。
type ReconciliationSyncStateRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetMultiple(ctx context.Context, keys []string) (map[string]string, error)
}

// ReconciliationStateKeyFxRateOverride 是汇率运行时覆盖值在状态表里的键名。
const ReconciliationStateKeyFxRateOverride = "fx_usd_cny_rate_override"

// normalizeReconciliationFxDefault 归一化配置层的汇率默认值。
//
// 非法值（0、负数）一律退化为 1，等价于「不做换算」。这里刻意不报错也不 panic：
// 汇率配错只应让成本数字失真，不应让整个取数链路起不来。
func normalizeReconciliationFxDefault(rate float64) float64 {
	if rate <= 0 {
		return 1
	}
	return rate
}

// parseReconciliationFxRate 解析面板写入的汇率覆盖值。
//
// 空串、非数字、NaN/Inf、非正数都判为「无效」并返回 false，由调用方回落到配置默认值；
// 同时打一条告警，否则面板上填了个错的汇率会静默失效，排查时无从下手。
func parseReconciliationFxRate(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed <= 0 {
		logger.LegacyPrintf("service.reconciliation_sync", "fx_override_invalid: raw=%q", raw)
		return 0, false
	}
	return parsed, true
}

// setReconciliationFxRateOverride 写入汇率覆盖值；非正数等价于「清除覆盖」。
func setReconciliationFxRateOverride(ctx context.Context, stateRepo ReconciliationSyncStateRepository, rate float64) error {
	if rate <= 0 {
		return stateRepo.Set(ctx, ReconciliationStateKeyFxRateOverride, "")
	}
	return stateRepo.Set(ctx, ReconciliationStateKeyFxRateOverride, strconv.FormatFloat(rate, 'f', -1, 64))
}

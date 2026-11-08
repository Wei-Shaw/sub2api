package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// 本文件是「经营对账」模块删除后**故意保留**的最小接线。
//
// 保留原因：A6 连接的凭据（base_url / access_token / user_id）与汇率，除了配置文件
// 之外还允许由管理面板写进数据库做运行时覆盖，读取入口就是
// ReconciliationA6SettingsService；而「按上游请求 ID 反查真实扣费」这条新链路
// 每一次取数前都要用「配置默认值 + 面板覆盖」解析出的生效配置去设置 A6 客户端
// （见 upstream_cost.go 的 resolveOne）。因此这两个 provider 不是对账模块的残留，
// 而是上游成本取数的必需依赖，不要随其他对账代码一起删掉。
//
// 注意：面板覆盖值存在 reconciliation_sync_state 表里（键名 a6_*_override）。
// 删掉那张表就等于删掉面板上配好的 A6 连接，新功能的取数会全部失败——这也是
// 该表在本次清理中被保留的唯一原因。表名沿用历史命名，未做重命名，避免为了
// 改名字去动生产数据。

// ProvideReconciliationA6Config 从应用配置构造 A6 客户端配置。
//
// 这里返回的是「配置层默认值」：面板上的覆盖值由 ReconciliationA6SettingsService
// 在这一层之上解析，因此本函数不需要知道面板的存在。
func ProvideReconciliationA6Config(cfg *config.Config) ReconciliationA6Config {
	return ReconciliationA6Config{
		BaseURL:     cfg.Reconciliation.A6BaseURL,
		AccessToken: cfg.Reconciliation.A6AccessToken,
		UserID:      cfg.Reconciliation.A6UserID,
		Timeout:     time.Duration(cfg.Reconciliation.TimeoutSeconds) * time.Second,
	}
}

// ProvideReconciliationA6Settings 构造 A6 生效凭据服务。
//
// defaults 是配置层（环境变量 / 配置文件）注入的默认值，面板覆盖优先于它。
// 汇率默认值直接取自应用配置：历史上有过一个中间的 syncConfig 结构体专门翻译
// 「小时/秒 → time.Duration」并顺带携带汇率，对账同步服务删除后它已无存在必要，
// 汇率就地读取，不再经过中转，免得留下一个只有单个字段的搬运结构体。
func ProvideReconciliationA6Settings(
	stateRepo ReconciliationSyncStateRepository,
	encryptor SecretEncryptor,
	defaults ReconciliationA6Config,
	cfg *config.Config,
) *ReconciliationA6SettingsService {
	return NewReconciliationA6SettingsService(stateRepo, encryptor, defaults, cfg.Reconciliation.FxUSDCNYRate)
}

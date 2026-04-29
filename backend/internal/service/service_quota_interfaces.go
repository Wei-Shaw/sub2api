package service

import (
	"context"
	"time"
)

type ServiceQuotaRuleRepository interface {
	List(ctx context.Context, filter ServiceQuotaListFilter) ([]*ServiceQuotaRule, error)
	Create(ctx context.Context, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	Update(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	Delete(ctx context.Context, id int64) error
	FetchAccountScope(ctx context.Context, accountID int64) (*AccountScopeInfo, error)
	FetchGroupScope(ctx context.Context, groupID int64) (*GroupScopeInfo, error)
	// FetchChannelScope 返回 channel 服务的 platform 集合与 group 集合，用于 validatePathLinkage
	// 校验 path 中 channel↔platform、channel↔group 的隶属关系。channel 不存在时返回 nil（非 error），
	// 与 FetchAccountScope/FetchGroupScope 行为一致——让 caller 用 nil 判断"未找到"。
	FetchChannelScope(ctx context.Context, channelID int64) (*ChannelScopeInfo, error)
	// FetchPathIDsByOwner 返回 service_quota_paths 中所有引用指定 owner（channel/group/account/user）的 path_id。
	//
	// 用于 admin 删除资源前快照受影响的 path_ids，再异步清 Redis 残留 counter key。
	// CASCADE 删完后查不到，所以必须 "先快照 → 再 DELETE"。
	//
	// owner 为 ServiceQuotaPathOwner* 中的常量。
	FetchPathIDsByOwner(ctx context.Context, owner string, ownerID int64) ([]int64, error)
}

// ServiceQuotaUserChecker 是 service quota 在校验 target_user_ids 时使用的最小化用户存在性检查接口。
//
// 单独抽出小接口（接口隔离原则）避免侵入庞大的 UserRepository，同时让 *userRepository
// 自动满足该接口（duck typing）。返回值与 group_repo.ExistsByIDs 对齐：map[id]exists，
// 缺失 ID 由调用方按需收集。
type ServiceQuotaUserChecker interface {
	ExistsByIDs(ctx context.Context, ids []int64) (map[int64]bool, error)
}

type ServiceQuotaLimiter interface {
	Current(ctx context.Context, key string, window time.Duration, mode string) (float64, error)
	Increment(ctx context.Context, key string, delta float64, window time.Duration, mode string) (float64, error)
	Acquire(ctx context.Context, key, member string, limit int64) (bool, error)
	Release(ctx context.Context, key, member string) error
	// Snapshot 只读返回单个 key 的当前计数，不执行任何写入（区别于 Current 的 rolling
	// 模式会顺带 ZRemRangeByScore）。监控页用它读取规则当前用量，必须保证多次调用幂等。
	//
	// mode 取值：ServiceQuotaWindowFixed / ServiceQuotaWindowRolling / ""（concurrency
	// 路径走 SnapshotKey.IsConcurrency，mode 字段被忽略）。key 不存在时返回
	// LimiterSnapshot{Current: 0, Exists: false}, nil；底层 Redis 错误以 error 返回。
	Snapshot(ctx context.Context, key string, window time.Duration, mode string) (LimiterSnapshot, error)
	// SnapshotMany 用单条 Redis Pipeline 批量获取多个 key 的快照，返回切片顺序
	// 与入参 keys 一一对应。单 key 失败（除 redis.Nil 外）降级为 {0, false} 并打 warn 日志，
	// 不让单条命令拖垮整个批次。
	SnapshotMany(ctx context.Context, keys []SnapshotKey) ([]LimiterSnapshot, error)
	// Reset 直接删除 limiter key，让计数立即归零。用于管理员手动重置场景。
	// fixed/rolling/concurrency 三种实现共用同一 DEL；concurrency 已在飞请求的
	// Release 是幂等的，无副作用。
	Reset(ctx context.Context, key string) error
	// ResetPattern 通过 SCAN+DEL 批量清理符合 pattern 的所有 limiter key（counter key 公式见
	// BuildServiceQuotaCounterKey）。用于以下两类管理员操作的"事务后善后"路径：
	//   - 规则被删除（DeleteRule） → pattern = svcquota:v2:<rule_id>:*
	//   - limiter window_mode 切换 fixed↔rolling → pattern = svcquota:v2:<rule_id>:*:<limiter_type>:*
	//
	// 不写入 counterKey 公式（避免破坏 BuildServiceQuotaCounterKey 单点实现），
	// 而是依赖 SCAN 在持有该前缀语义的 service 层调用方组装 pattern 后传入。
	//
	// 失败按 best-effort 语义返回 error 给调用方：调用方通常 log warn 但不阻塞业务
	// （Redis TTL 仍会兜底清理，只是窗口更长）。pattern 为空字符串时直接返回 nil 不做任何动作。
	ResetPattern(ctx context.Context, pattern string) error
}

// ServiceQuotaCache 抽象服务限额规则与开关的缓存层。
//
// 读路径（GetRules / GetEnabled）未命中时由 service 层自行通过 singleflight
// 加载并调用 SetRules / SetEnabled 回填，写路径只调 Invalidate* 把 key 删掉,
// 让其他实例下次读时重新拉取，避免多实例间陈旧数据的不一致窗口。
type ServiceQuotaCache interface {
	GetRules(ctx context.Context) ([]*ServiceQuotaRule, bool, error)
	SetRules(ctx context.Context, rules []*ServiceQuotaRule) error
	InvalidateRules(ctx context.Context) error
	GetEnabled(ctx context.Context) (*bool, error)
	SetEnabled(ctx context.Context, enabled bool) error
	InvalidateEnabled(ctx context.Context) error
	Invalidate(ctx context.Context) error
}

type ServiceQuotaService interface {
	ListRules(ctx context.Context, filter ServiceQuotaListFilter) ([]*ServiceQuotaRule, error)
	CreateRule(ctx context.Context, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	UpdateRule(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	DeleteRule(ctx context.Context, id int64) error
	InvalidateEnabledCache(ctx context.Context)
	// ReloadCache 失效规则缓存，让所有实例下次读时重新拉取数据库。
	// 用于 handler 层在删除 channel/group/account/user 等关联实体后
	// 主动让服务限额规则缓存失效。
	ReloadCache(ctx context.Context) error
	// PreCheck 为兼容旧 caller 保留的一阶段方法：内部等价于 PreCheckSelect + PreCheckAcquire，
	// 但 ChannelID/AccountID 取自传入 req（caller 路由前调用时这两字段为 0）。
	//
	// 新代码请走 BillingCacheService.PrepareBillingCheck 两阶段路径。
	PreCheck(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaLease, error)
	// PreCheckSelect 在路由前调用，仅做规则级过滤（enabled / target_user_ids），返回候选规则与
	// 计划上下文。不抢 concurrency、不增 RPM。
	PreCheckSelect(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaPreCheckPlan, error)
	// PreCheckAcquire 在 caller 选定 channel + account 后调用，基于 plan 候选规则做 path 匹配
	// （含 channel/account 维度），按 concurrency → rpm → deferred 三轮判定，命中超限返回错误，
	// 否则返回已 acquire 的 *ServiceQuotaLease（caller 必须 defer Release）。
	//
	// channelID/accountID == 0 表示 caller 未选定该维度（例如某些 fallback 路径或仅 group/platform
	// 限定的请求）；此时只命中那些对应字段为 nil 的 path。
	PreCheckAcquire(ctx context.Context, plan *ServiceQuotaPreCheckPlan, channelID, accountID int64) (*ServiceQuotaLease, error)
	// IsPreCheckTwoPhase 返回是否启用两阶段 PreCheck（对应 setting service_quota.precheck_two_phase）。
	// BillingTicket.Consume 据此决定是否在 caller 侧真正抢槽位。
	IsPreCheckTwoPhase(ctx context.Context) bool
	Record(ctx context.Context, req ServiceQuotaRecordRequest)
	// ResetLimiterCounter 直接清空指定 (rule_id, path_id, limiter_type, scope_user_id) 的 Redis 计数器。
	// 用于管理员手动重置：路径不存在或 limiter 缺失时返回 404 让 handler 透传 HTTP 状态。
	// scopeUserID == nil 对应 shared / per_user 未限定用户的情况；非 nil 对应 user 模式或 per_user 指定用户。
	ResetLimiterCounter(ctx context.Context, ruleID, pathID int64, limiterType string, scopeUserID *int64) error
	// ResetCountersForUser 清理某用户在所有规则下的残留 counter key（counter_mode=user / per_user 写出的 user-scoped 计数）。
	// 用于 admin 删除 user 时 fire-and-forget 调用：DB CASCADE 已清 service_quota_rule_users，
	// 这里把 Redis 中以该 user_id 结尾的 counter key 也清掉，避免复用 user_id 时新用户继承旧用户的计数。
	// 失败仅 warn 不抛（与其他 reset 一致），TTL 自然兜底。
	ResetCountersForUser(ctx context.Context, userID int64)
	// ResetCountersForPaths 清理一批 path_id 在所有规则/limiter 下的残留 counter key。
	// 用于 admin 删除 channel/group/account 前先快照受影响的 path_ids，再异步调用此方法清 Redis；
	// 调用顺序必须是 "先快照 → 再删 DB"，否则 CASCADE 删完查不到 path_ids。
	// 失败仅 warn 不抛，TTL 自然兜底。
	ResetCountersForPaths(ctx context.Context, pathIDs []int64)
	// SnapshotForAccounts 在选号阶段批量预测哪些候选账号已被 service_quota 限额阻塞。
	//
	// 仅识别 account-scope（path 含 account_id）/ channel-scope（path 不含 account_id 但含 channel_id）
	// 命中：这两类切号才有意义，调度循环可直接跳过对应账号。其他 scope（group / platform / user）
	// 命中后切号无解，留给 Consume 阶段返回 429。
	//
	// base 是请求快照（user_id / platform / group_id / model 必填，channel_id / account_id 由
	// candidates 维度展开），plan 是 PreCheckSelect 选出的候选规则集合，candidates 是当前可参与
	// 调度的账号 ID 列表。
	//
	// 三层 short-circuit：feature disabled / plan == nil / candidates 空时全部返回 (nil, nil)。
	// Redis 故障：fail-open 返回空 map + nil error（log warn + metric），不阻塞调度——
	// 错放由 Consume 阶段兜底。
	//
	// 返回 map[accountID]*ServiceQuotaPredictedBlock：仅 account/channel scope 命中的账号才进 map。
	SnapshotForAccounts(
		ctx context.Context,
		base ServiceQuotaCheckRequest,
		plan *ServiceQuotaPreCheckPlan,
		candidates []int64,
	) (map[int64]*ServiceQuotaPredictedBlock, error)
	// SnapshotPathIDsByOwner 同步查询当前 service_quota_paths 中引用指定 owner（channel/group/account）的 path_id 列表。
	//
	// 必须在 admin 调 DeleteChannel/Group/Account 之前调用——CASCADE 删完后 paths 行就消失了。
	// 返回 nil/empty 列表时调用方可以省去后续 ResetCountersForPaths 调用。
	//
	// owner 必须是 ServiceQuotaPathOwnerChannel / Group / Account 之一。失败返回 error（与 reset 系列的 fail-open 不同：
	// 这是"读快照"路径，错误不能吞，否则后续 reset 拿到的就是空列表 → 静默漏清）。
	SnapshotPathIDsByOwner(ctx context.Context, owner string, ownerID int64) ([]int64, error)
}

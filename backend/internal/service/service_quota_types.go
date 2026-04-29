package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SettingKeyServiceQuotaEnabled = "service_quota_enabled"

	// SettingKeyServiceQuotaPreCheckTwoPhase 控制 PreCheck 是否走两阶段路径
	// （PreCheckSelect 在路由前选规则、PreCheckAcquire 在选定 channel/account 后抢槽位）。
	//
	// 默认 false（关闭）：CheckBillingEligibility 行为与历史版本完全一致，PreCheck 在 channel/account
	// 还未确定时被一次性调用，account/channel scope 的 concurrency / rpm 限流器仍然失效。
	//
	// true（开启）：BillingTicket.Prepare 只调 PreCheckSelect 选规则，BillingTicket.Consume 在
	// caller 选定 channel + account 后调用 PreCheckAcquire 才真正抢 concurrency / 增 rpm，
	// account/channel scope 限流器从此生效。详见 BillingCacheService.PrepareBillingCheck。
	SettingKeyServiceQuotaPreCheckTwoPhase = "service_quota.precheck_two_phase"

	ServiceQuotaLimiterRPM         = "rpm"
	ServiceQuotaLimiterTPM         = "tpm"
	ServiceQuotaLimiterTPD         = "tpd"
	ServiceQuotaLimiterDailyUSD    = "daily_usd"
	ServiceQuotaLimiterConcurrency = "concurrency"

	// ServiceQuotaCounterMode* 决定规则 Redis 计数 key 的分片方式：
	//   - user     → 规则只对关联表中列出的用户生效，每个用户独立计数
	//   - per_user → 对 scope 内所有用户生效，按 user_id 分片
	//   - shared   → 对 scope 内所有用户生效，共享同一计数器
	ServiceQuotaCounterModeUser    = "user"
	ServiceQuotaCounterModePerUser = "per_user"
	ServiceQuotaCounterModeShared  = "shared"

	ServiceQuotaWindowFixed   = "fixed"
	ServiceQuotaWindowRolling = "rolling"

	// ServiceQuotaPathOwner* 是 FetchPathIDsByOwner 的 owner 维度，对应 service_quota_paths 表的字段名。
	// 用常量代替魔法字符串避免拼错（service 层与 repo 层共享）。
	ServiceQuotaPathOwnerChannel = "channel_id"
	ServiceQuotaPathOwnerGroup   = "group_id"
	ServiceQuotaPathOwnerAccount = "account_id"

	// ServiceQuotaTokenComponent* 决定 TPM/TPD 限流器把哪几种 token 计入计数。
	// 仅 TPM/TPD 使用此配置；RPM/concurrency/daily_usd 忽略。
	//
	// 默认值（ServiceQuotaTokenComponentsDefault）= {input, output, cache_creation}：
	//   - 与 Anthropic 3.7+ ITPM(input+cache_creation) + OTPM(output) 双桶之和一致
	//   - 与 Bedrock Claude 3.7+ 公式 InputTokenCount + CacheWriteInputTokens + OutputTokenCount 一致
	//   - 剔除 cache_read：鼓励 prompt cache，与 Anthropic 3.7+/Bedrock 3.7+/Groq 一致
	//   - 与 OpenAI/Gemini 单桶相比略松（它们也算 cache_read）
	ServiceQuotaTokenComponentInput         = "input"
	ServiceQuotaTokenComponentOutput        = "output"
	ServiceQuotaTokenComponentCacheCreation = "cache_creation"
	ServiceQuotaTokenComponentCacheRead     = "cache_read"
)

// ServiceQuotaTokenComponentsDefault 是 TPM/TPD 限流器 token_components 字段的默认值，
// 与 DB DEFAULT 一致。新建限流器若未传该字段则采用此默认。
var ServiceQuotaTokenComponentsDefault = []string{
	ServiceQuotaTokenComponentInput,
	ServiceQuotaTokenComponentOutput,
	ServiceQuotaTokenComponentCacheCreation,
}

// ServiceQuotaTokenComponentsAll 是全部合法 token component 值，用于校验 input。
var ServiceQuotaTokenComponentsAll = []string{
	ServiceQuotaTokenComponentInput,
	ServiceQuotaTokenComponentOutput,
	ServiceQuotaTokenComponentCacheCreation,
	ServiceQuotaTokenComponentCacheRead,
}

// IsValidServiceQuotaTokenComponent 判断字符串是否是合法 token component。
func IsValidServiceQuotaTokenComponent(s string) bool {
	switch s {
	case ServiceQuotaTokenComponentInput,
		ServiceQuotaTokenComponentOutput,
		ServiceQuotaTokenComponentCacheCreation,
		ServiceQuotaTokenComponentCacheRead:
		return true
	}
	return false
}

// ServiceQuotaLimiterTypeUsesTokenComponents 判断 limiter type 是否使用 token_components 字段。
// 仅 TPM/TPD；其他类型应忽略此字段（service 层会强制置 nil）。
func ServiceQuotaLimiterTypeUsesTokenComponents(kind string) bool {
	return kind == ServiceQuotaLimiterTPM || kind == ServiceQuotaLimiterTPD
}

var (
	ErrServiceQuotaExceeded = infraerrors.TooManyRequests(
		"SERVICE_QUOTA_EXCEEDED",
		"service quota exceeded",
	)
	ErrServiceQuotaRuleNotFound = infraerrors.NotFound(
		"SERVICE_QUOTA_RULE_NOT_FOUND",
		"service quota rule not found",
	)
)

// ServiceQuotaLimiterDef 单个限流器：一条规则可以挂多种类型（RPM、TPD 等）。
//
// TokenComponents 仅对 TPM/TPD 有效，决定哪几种 token 计入计数（input/output/cache_creation/cache_read）。
// 其他 limiter type 该字段为 nil；service 层会强制清洗。
//
// CountOnArrival 仅对 RPM 有效：true 表示请求到达即 +1（旧语义，迁移 137 把现存 RPM 行迁移成此值），
// false 表示仅在请求成功后由 Record 阶段 +1（默认）。其他 limiter type 该字段恒为 false 且无意义。
type ServiceQuotaLimiterDef struct {
	ID              int64    `json:"id"`
	RuleID          int64    `json:"rule_id"`
	LimiterType     string   `json:"limiter_type"`
	WindowMode      string   `json:"window_mode"`
	LimitValue      float64  `json:"limit_value"`
	TokenComponents []string `json:"token_components,omitempty"`
	CountOnArrival  bool     `json:"count_on_arrival,omitempty"`
}

type ServiceQuotaLimiterInput struct {
	LimiterType     string   `json:"limiter_type"`
	WindowMode      string   `json:"window_mode"`
	LimitValue      float64  `json:"limit_value"`
	TokenComponents []string `json:"token_components,omitempty"`
	CountOnArrival  *bool    `json:"count_on_arrival,omitempty"`
}

// ServiceQuotaLimiterTypeUsesCountOnArrival 判断 limiter type 是否使用 count_on_arrival 字段。
// 仅 RPM 使用；其他类型 service 层会强制置零，避免管理员误填影响判定。
func ServiceQuotaLimiterTypeUsesCountOnArrival(kind string) bool {
	return kind == ServiceQuotaLimiterRPM
}

// ServiceQuotaLimiterTypeIsInteger 判断 limiter type 的 LimitValue 业务上是否必须为整数。
//
// rpm / tpm / tpd / concurrency 都是"次数 / 个数"语义，本就是整数；
// daily_usd 是金额必须保留小数（精度由项目 CLAUDE.md 约定的 decimal 处理，本字段是 float64
// 历史遗留，等彻底切 string-based 再大改）。
//
// service 层用此判定在 normalize 阶段对整数型做 math.Round，截断浮点漂移
// （前端 NumericInput integer prop 是 UX 屏障；后端 round 是数据底线）。
func ServiceQuotaLimiterTypeIsInteger(kind string) bool {
	switch kind {
	case ServiceQuotaLimiterRPM,
		ServiceQuotaLimiterTPM,
		ServiceQuotaLimiterTPD,
		ServiceQuotaLimiterConcurrency:
		return true
	default:
		return false
	}
}

// ShouldIncrementOnArrival 决定 PreCheckAcquire 阶段是否需要对该 limiter +1：
//   - 仅 RPM 受 CountOnArrival 控制：true → arrival 阶段 +1（旧语义，Record 不再重复）
//   - 其他 limiter 走各自既有路径（concurrency=Acquire / TPM/TPD/daily_usd=Record），
//     这里返回 false 表示 arrival 阶段不需要额外写入
//
// 抽到 receiver 是为了让 PreCheckAcquire/Record 共用同一判定，避免规则分散漂移。
func (l ServiceQuotaLimiterDef) ShouldIncrementOnArrival() bool {
	return l.LimiterType == ServiceQuotaLimiterRPM && l.CountOnArrival
}

// ShouldIncrementOnRecord 决定 Record 阶段（请求成功落账后）是否需要对该 limiter +1：
//   - RPM 仅当 CountOnArrival=false 时由 Record 兜底 +1（"成功才计"语义）
//   - TPM/TPD/daily_usd 天然由 Record 计数（与 CountOnArrival 无关）
//   - concurrency 不通过 Record 计数
func (l ServiceQuotaLimiterDef) ShouldIncrementOnRecord() bool {
	switch l.LimiterType {
	case ServiceQuotaLimiterRPM:
		return !l.CountOnArrival
	case ServiceQuotaLimiterTPM, ServiceQuotaLimiterTPD, ServiceQuotaLimiterDailyUSD:
		return true
	default:
		return false
	}
}

// ServiceQuotaPathDef 路径定义：单向递进链 平台→渠道→分组→账号→模型，
// 每层非 nil 才参与匹配，nil 视作不限制该维度。
type ServiceQuotaPathDef struct {
	ID           int64   `json:"id"`
	RuleID       int64   `json:"rule_id"`
	Platform     *string `json:"platform,omitempty"`
	ChannelID    *int64  `json:"channel_id,omitempty"`
	GroupID      *int64  `json:"group_id,omitempty"`
	AccountID    *int64  `json:"account_id,omitempty"`
	ModelPattern *string `json:"model_pattern,omitempty"`
}

type ServiceQuotaPathInput struct {
	Platform     *string `json:"platform"`
	ChannelID    *int64  `json:"channel_id"`
	GroupID      *int64  `json:"group_id"`
	AccountID    *int64  `json:"account_id"`
	ModelPattern *string `json:"model_pattern"`
}

// ServiceQuotaRuleUserRef 是绑定用户的轻量引用，用于前端 chip 展示（规则只在 counter_mode=user 时携带此字段）。
type ServiceQuotaRuleUserRef struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

// ServiceQuotaRule 一条层级规则。
//
// 一条规则 = N 个限流器 × M 条路径 × 用户绑定。每个 path×limiter 组合在 Redis 中
// 维护独立的计数器。CounterMode 与 IsFallback 正交：
//   - CounterMode 决定计数 key 的分片方式
//   - IsFallback=true 表示兜底规则：同一 limiter_type 有其他非 fallback 规则命中时该 limiter 自动让位
type ServiceQuotaRule struct {
	ID            int64                     `json:"id"`
	Enabled       bool                      `json:"enabled"`
	Name          *string                   `json:"name,omitempty"`
	CounterMode   string                    `json:"counter_mode"`
	IsFallback    bool                      `json:"is_fallback"`
	Limiters      []ServiceQuotaLimiterDef  `json:"limiters"`
	Paths         []ServiceQuotaPathDef     `json:"paths"`
	TargetUserIDs []int64                   `json:"target_user_ids,omitempty"`
	TargetUsers   []ServiceQuotaRuleUserRef `json:"target_users,omitempty"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
}

type ServiceQuotaRuleInput struct {
	Enabled       *bool                      `json:"enabled"`
	Name          *string                    `json:"name"`
	CounterMode   string                     `json:"counter_mode"`
	IsFallback    bool                       `json:"is_fallback"`
	Limiters      []ServiceQuotaLimiterInput `json:"limiters"`
	Paths         []ServiceQuotaPathInput    `json:"paths"`
	TargetUserIDs []int64                    `json:"target_user_ids"`
}

type ServiceQuotaListFilter struct {
	Enabled     *bool
	LimiterType string
}

type ServiceQuotaCheckRequest struct {
	UserID    int64
	Platform  string
	ChannelID int64
	GroupID   int64
	AccountID int64
	Model     string
}

// ServiceQuotaRecordRequest 一次 LLM 调用结束后的限额计费快照。
//
// 拆四个 token 字段（不再用单一 Tokens int64）让 TPM/TPD 限流器按各自 token_components
// 配置自由选择计入项。Cost 单位美元，仅 daily_usd 限流器使用。
type ServiceQuotaRecordRequest struct {
	ServiceQuotaCheckRequest
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	Cost                float64
}

// SumTokens 按指定 components 求和。components 为空或 nil 时返回 0；
// 未识别值跳过（向前兼容老配置）。
func (r ServiceQuotaRecordRequest) SumTokens(components []string) int64 {
	var sum int64
	for _, c := range components {
		switch c {
		case ServiceQuotaTokenComponentInput:
			sum += r.InputTokens
		case ServiceQuotaTokenComponentOutput:
			sum += r.OutputTokens
		case ServiceQuotaTokenComponentCacheCreation:
			sum += r.CacheCreationTokens
		case ServiceQuotaTokenComponentCacheRead:
			sum += r.CacheReadTokens
		}
	}
	return sum
}

type ServiceQuotaLease struct {
	Release func()
}

// ServiceQuotaPreCheckPlan 是两阶段 PreCheck 的中间态。
//
// 阶段 A（PreCheckSelect）只做"必到字段"过滤——is_enabled、counter_mode=user 时 target_user_ids 命中——
// 不做 path 匹配（channel_id / account_id 此时还未选定），返回的候选规则集合保留完整 path 列表，
// 由阶段 B 在补全 channel/account 后再次 findMatchingPaths。
//
// Plan 持有原始 req 副本（不含 channel/account），让阶段 B 可基于同一 req 重建 ServiceQuotaCheckRequest；
// rulesVersion 则记录读取时的规则 snapshot 序号，方便后续观测/测试。Plan 一定是只读的，跨 goroutine 共享安全。
type ServiceQuotaPreCheckPlan struct {
	// Rules 是阶段 A 选出的候选规则集合，包含全部 path（未按 channel/account 过滤）。
	// 阶段 B 会按补全后的 req 再次做 path 匹配。
	Rules []*ServiceQuotaRule
	// Req 是阶段 A 调用时的请求快照。阶段 B 会在此基础上补 ChannelID/AccountID。
	Req ServiceQuotaCheckRequest
	// PreparedAt 记录 plan 创建时间，用于观测/调试，不参与匹配语义。
	PreparedAt time.Time
}

// AccountScopeInfo / GroupScopeInfo / ChannelScopeInfo 用于 path 链路一致性校验：
// 下级必须是上级的子孙（account ⊂ group ⊂ platform；channel 服务的 platform/group 必须包含 path 指定值）。
type AccountScopeInfo struct {
	Platform string
	GroupIDs []int64
}

type GroupScopeInfo struct {
	Platform string
}

// ChannelScopeInfo 用于 path 校验 channel 的服务关系：
//   - Platforms：channel 的 model_pricing 中出现过的所有 platform 集合（小写归一化）
//   - GroupIDs：channel 通过 channel_groups 关联表绑定的所有 group_id
//
// path.channel_id 配合 path.platform 时，platform 必须出现在 ChannelScopeInfo.Platforms；
// path.channel_id 配合 path.group_id 时，group_id 必须出现在 ChannelScopeInfo.GroupIDs。
type ChannelScopeInfo struct {
	Platforms []string
	GroupIDs  []int64
}

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

// SnapshotKey 描述一个 limiter 的快照读取目标。
//
// 区分 IsConcurrency 与普通 fixed/rolling：concurrency 走 ZSET ZCount + 泄漏窗口，
// 不依赖 mode 字符串；fixed / rolling 走 GET 或 ZRangeByScoreWithScores，由 Mode 决定。
// Window 仅对 rolling 起作用（fixed/concurrency 自带固定窗口语义）。
type SnapshotKey struct {
	Key           string
	Window        time.Duration
	Mode          string
	IsConcurrency bool
}

// LimiterSnapshot 单个 limiter 的只读快照。
//
// Exists=false 表示 key 在 Redis 中不存在（首次接入或已过期），调用方据此区分
// "未启用" 与 "已启用但用量 0"。Current 始终是有效数值（不存在时为 0）。
//
// ResetAtUnixMs 表示该计数器"自然清零"的 Unix 毫秒时间戳：
//   - fixed window：key 真实 PTTL 推算（now + PTTL）；PTTL=-1（永久）按 0 处理
//   - rolling window：now + 窗口长度（窗口右端是固定的相对当前时刻）
//   - concurrency / key 不存在：0
//
// 前端展示"X 秒后重置"用：max(0, ceil((ResetAtUnixMs - now)/1000))。
type LimiterSnapshot struct {
	Current       float64
	Exists        bool
	ResetAtUnixMs int64
}

// ServiceQuotaCache 抽象服务限额规则与开关的缓存层。
//
// 读路径（GetRules / GetEnabled）未命中时由 service 层自行通过 singleflight
// 加载并调用 SetRules / SetEnabled 回填，写路径只调 Invalidate* 把 key 删掉，
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
	// SnapshotPathIDsByOwner 同步查询当前 service_quota_paths 中引用指定 owner（channel/group/account）的 path_id 列表。
	//
	// 必须在 admin 调 DeleteChannel/Group/Account 之前调用——CASCADE 删完后 paths 行就消失了。
	// 返回 nil/empty 列表时调用方可以省去后续 ResetCountersForPaths 调用。
	//
	// owner 必须是 ServiceQuotaPathOwnerChannel / Group / Account 之一。失败返回 error（与 reset 系列的 fail-open 不同：
	// 这是"读快照"路径，错误不能吞，否则后续 reset 拿到的就是空列表 → 静默漏清）。
	SnapshotPathIDsByOwner(ctx context.Context, owner string, ownerID int64) ([]int64, error)
}

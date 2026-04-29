package service

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

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

// ServiceQuotaPredictedBlock 是调度阶段批量预测出的"该账号已被服务限额阻塞"结果。
//
// 仅当 path 含 account_id（最高优先级）或 channel_id 时才会被标记 —— 这两类 scope
// 切号才有意义；user / group / platform scope 命中后切号无解，留给 Consume 阶段返回 429。
//
// 模型维度天然支持：path.ModelPattern 与 req.Model 的匹配在 findMatchingPaths 已完成，
// 此结构只描述"哪个规则的哪个 limiter 让该账号当前不可调度"，不再持有 path/model 信息。
type ServiceQuotaPredictedBlock struct {
	RuleID      int64
	LimiterType string // rpm / tpm / tpd / daily_usd / concurrency
	ScopeKind   string // account / channel
	Used        float64
	Limit       float64
}

// ServiceQuotaPredictedBlockScope* 是 ServiceQuotaPredictedBlock.ScopeKind 的取值集合。
// 仅 account / channel 会出现在 SnapshotForAccounts 返回的 map；其他 scope 命中后切号无解，
// 留给 Consume 阶段返回 429（与原生限流命中所有可调度账号 → 503 链路保持一致）。
const (
	ServiceQuotaPredictedBlockScopeAccount  = "account"
	ServiceQuotaPredictedBlockScopeChannel  = "channel"
	ServiceQuotaPredictedBlockScopeGroup    = "group"
	ServiceQuotaPredictedBlockScopePlatform = "platform"
	ServiceQuotaPredictedBlockScopeUser     = "user"
)

// serviceQuotaPathScopeKind 判定 path 的 scope 维度。
//
// 优先级（最高优先级铁律）：
//  1. AccountID != nil       → account（**最高**，无视是否同时含 channel/group/platform/model）
//  2. ChannelID != nil       → channel
//  3. GroupID != nil         → group
//  4. Platform != ""         → platform
//  5. 否则                   → user
//
// 切号语义只对 account / channel 有意义：含 account → 跳过该账号；只含 channel → 跳过
// 该 channel 的全部账号；其他 scope 命中后没有"切到另一个就好"的解，必须落到 Consume 429。
func serviceQuotaPathScopeKind(path ServiceQuotaPathDef) string {
	if path.AccountID != nil {
		return ServiceQuotaPredictedBlockScopeAccount
	}
	if path.ChannelID != nil {
		return ServiceQuotaPredictedBlockScopeChannel
	}
	if path.GroupID != nil {
		return ServiceQuotaPredictedBlockScopeGroup
	}
	if path.Platform != nil && *path.Platform != "" {
		return ServiceQuotaPredictedBlockScopePlatform
	}
	return ServiceQuotaPredictedBlockScopeUser
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

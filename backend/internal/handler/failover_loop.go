package handler

import (
	"context"
	"crypto/rand"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

// TempUnscheduler 用于 HandleFailoverError 中同账号重试耗尽后的临时封禁。
// GatewayService 隐式实现此接口。
type TempUnscheduler interface {
	TempUnscheduleRetryableError(ctx context.Context, accountID int64, failoverErr *service.UpstreamFailoverError)
}

// StickySessionUnbinder 用于 HandleFailoverError 中配额耗尽时清除粘性会话绑定。
// GatewayService 隐式实现此接口。
type StickySessionUnbinder interface {
	UnbindStickySession(ctx context.Context, groupID *int64, sessionHash string) error
}

// SessionFanoutLimiter 扩展 TempUnscheduler，添加 P0-3 反扫荡能力。
// GatewayService 隐式实现此接口。
type SessionFanoutLimiter interface {
	TempUnscheduler
	RecordSessionFanout(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error
	IsSessionFanoutExhausted(ctx context.Context, groupID *int64, sessionHash string) (bool, int)
}

type sessionFanoutConfigProvider interface {
	GetSessionFanoutConfig() (limit int, boundJitterMin, boundJitterMax time.Duration)
}

// FailoverAction 表示 failover 错误处理后的下一步动作
type FailoverAction int

const (
	// FailoverContinue 继续循环（同账号重试或切换账号，调用方统一 continue）
	FailoverContinue FailoverAction = iota
	// FailoverExhausted 切换次数耗尽（调用方应返回错误响应）
	FailoverExhausted
	// FailoverCanceled context 已取消（调用方应直接 return）
	FailoverCanceled
)

const (
	// maxSameAccountRetries 同账号重试次数上限（针对 RetryableOnSameAccount 错误）
	maxSameAccountRetries = 2
	// sameAccountRetryDelay 同账号重试间隔
	sameAccountRetryDelay = 500 * time.Millisecond
	// singleAccountBackoffDelay 单账号分组 503 退避重试固定延时。
	// Service 层在 SingleAccountRetry 模式下已做充分原地重试（最多 3 次、总等待 30s），
	// Handler 层只需短暂间隔后重新进入 Service 层即可。
	singleAccountBackoffDelay = 2 * time.Second
)

// FailoverState 跨循环迭代共享的 failover 状态
type FailoverState struct {
	SwitchCount           int
	MaxSwitches           int
	FailedAccountIDs      map[int64]struct{}
	SameAccountRetryCount map[int64]int
	LastFailoverErr       *service.UpstreamFailoverError
	ForceCacheBilling     bool
	hasBoundSession       bool

	// P0-3: Session fanout limiting
	sessionHash    string
	groupID        *int64
	fanoutLimit    int
	boundJitterMin time.Duration
	boundJitterMax time.Duration
}

// NewFailoverState 创建 failover 状态
func NewFailoverState(maxSwitches int, hasBoundSession bool) *FailoverState {
	return &FailoverState{
		MaxSwitches:           maxSwitches,
		FailedAccountIDs:      make(map[int64]struct{}),
		SameAccountRetryCount: make(map[int64]int),
		hasBoundSession:       hasBoundSession,
	}
}

// NewFailoverStateWithFanout 创建带有 fanout 限制的 failover 状态 (P0-3)
func NewFailoverStateWithFanout(
	maxSwitches int,
	hasBoundSession bool,
	sessionHash string,
	groupID *int64,
	fanoutLimit int,
	boundJitterMin, boundJitterMax time.Duration,
) *FailoverState {
	return &FailoverState{
		MaxSwitches:           maxSwitches,
		FailedAccountIDs:      make(map[int64]struct{}),
		SameAccountRetryCount: make(map[int64]int),
		hasBoundSession:       hasBoundSession,
		sessionHash:           sessionHash,
		groupID:               groupID,
		fanoutLimit:           fanoutLimit,
		boundJitterMin:        boundJitterMin,
		boundJitterMax:        boundJitterMax,
	}
}

func newFailoverStateWithGatewayFanout(
	maxSwitches int,
	hasBoundSession bool,
	sessionHash string,
	groupID *int64,
	provider sessionFanoutConfigProvider,
) *FailoverState {
	if provider == nil {
		return NewFailoverState(maxSwitches, hasBoundSession)
	}
	fanoutLimit, boundJitterMin, boundJitterMax := provider.GetSessionFanoutConfig()
	return NewFailoverStateWithFanout(
		maxSwitches,
		hasBoundSession,
		sessionHash,
		groupID,
		fanoutLimit,
		boundJitterMin,
		boundJitterMax,
	)
}

// HandleFailoverError 处理 UpstreamFailoverError，返回下一步动作。
// 包含：缓存计费判断、同账号重试、临时封禁、切换计数、Antigravity 延时。
func (s *FailoverState) HandleFailoverError(
	ctx context.Context,
	gatewayService TempUnscheduler,
	accountID int64,
	platform string,
	failoverErr *service.UpstreamFailoverError,
) FailoverAction {
	s.LastFailoverErr = failoverErr

	// 缓存计费判断
	if needForceCacheBilling(s.hasBoundSession, failoverErr) {
		s.ForceCacheBilling = true
	}

	// 配额耗尽时清除粘性会话绑定，避免后续请求继续粘到已耗尽的账号
	if failoverErr.QuotaExhausted && s.hasBoundSession {
		if unbinder, ok := gatewayService.(StickySessionUnbinder); ok && s.sessionHash != "" {
			_ = unbinder.UnbindStickySession(ctx, s.groupID, s.sessionHash)
			logger.FromContext(ctx).Warn("gateway.failover_quota_exhausted_unbind_sticky",
				zap.Int64("account_id", accountID),
				zap.String("session_hash", shortSessionHash(s.sessionHash)),
			)
		}
	}

	// 同账号重试：对 RetryableOnSameAccount 的临时性错误，先在同一账号上重试
	if failoverErr.RetryableOnSameAccount && s.SameAccountRetryCount[accountID] < maxSameAccountRetries {
		s.SameAccountRetryCount[accountID]++
		logger.FromContext(ctx).Warn("gateway.failover_same_account_retry",
			zap.Int64("account_id", accountID),
			zap.Int("upstream_status", failoverErr.StatusCode),
			zap.Int("same_account_retry_count", s.SameAccountRetryCount[accountID]),
			zap.Int("same_account_retry_max", maxSameAccountRetries),
		)
		if !sleepWithContext(ctx, sameAccountRetryDelay) {
			return FailoverCanceled
		}
		return FailoverContinue
	}

	// 同账号重试用尽，执行临时封禁
	if failoverErr.RetryableOnSameAccount {
		gatewayService.TempUnscheduleRetryableError(ctx, accountID, failoverErr)
	}

	// 加入失败列表
	s.FailedAccountIDs[accountID] = struct{}{}

	// P0-3: 检查 session fanout 限制（需要切号前先记录当前账号）
	if limiter, ok := gatewayService.(SessionFanoutLimiter); ok && s.fanoutLimit > 0 && s.sessionHash != "" {
		// 记录当前失败账号到 fanout set
		_ = limiter.RecordSessionFanout(ctx, s.groupID, s.sessionHash, accountID)

		// 检查是否超限
		if exhausted, count := limiter.IsSessionFanoutExhausted(ctx, s.groupID, s.sessionHash); exhausted {
			logger.FromContext(ctx).Warn("gateway.failover_fanout_exhausted",
				zap.Int64("account_id", accountID),
				zap.String("session_hash", shortSessionHash(s.sessionHash)),
				zap.Int("fanout_count", count),
				zap.Int("fanout_limit", s.fanoutLimit),
				zap.Int("upstream_status", failoverErr.StatusCode),
			)
			return FailoverExhausted
		}
	}

	// 检查是否耗尽
	if s.SwitchCount >= s.MaxSwitches {
		return FailoverExhausted
	}

	// 递增切换计数
	s.SwitchCount++
	logger.FromContext(ctx).Warn("gateway.failover_switch_account",
		zap.Int64("account_id", accountID),
		zap.Int("upstream_status", failoverErr.StatusCode),
		zap.Int("switch_count", s.SwitchCount),
		zap.Int("max_switches", s.MaxSwitches),
	)

	// Antigravity 平台换号线性递增延时
	if platform == service.PlatformAntigravity {
		delay := time.Duration(s.SwitchCount-1) * time.Second
		if !sleepWithContext(ctx, delay) {
			return FailoverCanceled
		}
	} else if s.SwitchCount >= 2 {
		// P0-3: 绑定会话切号时使用更长的抖动延迟（2-10s），普通会话维持 0-600ms
		var jitterDelay time.Duration
		if s.hasBoundSession && s.boundJitterMax > 0 {
			jitterDelay = boundSessionJitterDelay(s.boundJitterMin, s.boundJitterMax)
			logger.FromContext(ctx).Debug("gateway.failover_bound_session_jitter",
				zap.Duration("jitter_delay", jitterDelay),
			)
		} else {
			// 非 Antigravity 平台的跨账号抖动（2026-04 加固）：
			//
			// 真实 Claude Code CLI 在收到 4xx/5xx 后的重试节奏是几百毫秒到几秒级
			// 的"自然"间隔（受 Node fetch + 用户操作影响）。sub2api 网关在多账号
			// 共享池里如果接到 429/limit 后立刻打到下一个账号，上游能看到：
			//   - 同一上游 prompt / sessionHash
			//   - 在 A 账号 429 后 < 50ms 出现在 B 账号
			// 这是"扫荡式切号"的标志性信号，直接帮 Anthropic 把账号关联起来。
			//
			// 我们对第 2 次起的切换加 0-600ms 抖动（首次切换不加延迟，避免一个简单
			// 的 token 失效就拖慢正常用户）。和 Antigravity 的线性退避不冲突。
			jitterDelay = crossAccountJitterDelay()
		}
		if !sleepWithContext(ctx, jitterDelay) {
			return FailoverCanceled
		}
	}

	return FailoverContinue
}

// shortSessionHash returns at most first 8 chars of sessionHash for logging
func shortSessionHash(sessionHash string) string {
	if len(sessionHash) <= 8 {
		return sessionHash
	}
	return sessionHash[:8]
}

// crossAccountJitterDelay 返回 0-600ms 之间的随机抖动延迟，用于跨账号 failover。
// 使用 crypto/rand 是因为本进程其它路径已普遍依赖它，不引入新的 PRNG 状态；
// 调用频率低（仅 failover 切号路径），开销可忽略。
//
// 选型 600ms 上限：足以打散"切号即重试"的薄尾分布特征，又不会显著影响用户体感
// （对端在等待一次失败重试时，0-600ms 抖动 + 网络往返 ≈ 1-2s 总体）。
func crossAccountJitterDelay() time.Duration {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 极罕见：crypto/rand 失败时退化为固定 200ms（仍优于零延迟）
		return 200 * time.Millisecond
	}
	// 0..65535 -> 0..600ms
	n := int(b[0])<<8 | int(b[1])
	return time.Duration(n*600/65536) * time.Millisecond
}

// recordOpenAIFanoutAndCheckExhausted 是为 OpenAI 原生 failover loop 提供的"轻量
// 版" P0-3 反扫荡入口（瑕疵 2 修复）。
//
// OpenAI 三条原生路径（ChatCompletions / Responses / Messages）没有走 FailoverState，
// 而是手写循环（failedAccountIDs map + lastFailoverErr + switchCount）。这个 helper
// 在 handler 决定切号前记录一次失败账号，并检查 sessionHash 维度的 fanout 是否超限。
//
// 行为与 FailoverState.HandleFailoverError 中的 fanout 段保持一致：
//   - limiter 为 nil 或 sessionHash 为空 → 退化为 noop（不阻断）。
//   - 配置侧 fanout limit ≤ 0 → 同样 noop（向后兼容）。
//   - 超限 → 返回 true，调用方应改走 failover-exhausted 错误响应。
//
// 返回 true 表示"已达 fanout 上限，请勿继续切号"。
func recordOpenAIFanoutAndCheckExhausted(
	ctx context.Context,
	limiter SessionFanoutLimiter,
	provider sessionFanoutConfigProvider,
	accountID int64,
	sessionHash string,
	groupID *int64,
	upstreamStatus int,
	reqLog *zap.Logger,
) bool {
	if limiter == nil || sessionHash == "" || provider == nil {
		return false
	}
	fanoutLimit, _, _ := provider.GetSessionFanoutConfig()
	if fanoutLimit <= 0 {
		return false
	}
	_ = limiter.RecordSessionFanout(ctx, groupID, sessionHash, accountID)
	exhausted, count := limiter.IsSessionFanoutExhausted(ctx, groupID, sessionHash)
	if !exhausted {
		return false
	}
	if reqLog != nil {
		reqLog.Warn("openai.failover_fanout_exhausted",
			zap.Int64("account_id", accountID),
			zap.String("session_hash", shortSessionHash(sessionHash)),
			zap.Int("fanout_count", count),
			zap.Int("fanout_limit", fanoutLimit),
			zap.Int("upstream_status", upstreamStatus),
		)
	}
	return true
}

// applyOpenAIFailoverJitter 是 OpenAI 原生 failover loop 用的抖动延迟入口
// （瑕疵 2 修复）。语义对齐 FailoverState.HandleFailoverError 中的抖动段：
//
//   - 第 1 次切换不引入抖动（避免单纯 token 失效拖慢正常用户）。
//   - 第 2 次起：绑定会话使用 boundJitter[min,max]（默认 2-10s），普通会话使用
//     0-600ms 的 crossAccountJitterDelay。
//
// switchCountAfterIncrement 是 switchCount++ 之后的值（即"本次切换是第几次"）。
// 返回 false 表示 ctx 已取消，调用方应直接 return。
func applyOpenAIFailoverJitter(
	ctx context.Context,
	provider sessionFanoutConfigProvider,
	hasBoundSession bool,
	switchCountAfterIncrement int,
	reqLog *zap.Logger,
) bool {
	if switchCountAfterIncrement < 2 {
		return true
	}
	var boundJitterMin, boundJitterMax time.Duration
	if provider != nil {
		_, boundJitterMin, boundJitterMax = provider.GetSessionFanoutConfig()
	}
	var jitterDelay time.Duration
	if hasBoundSession && boundJitterMax > 0 {
		jitterDelay = boundSessionJitterDelay(boundJitterMin, boundJitterMax)
		if reqLog != nil {
			reqLog.Debug("openai.failover_bound_session_jitter",
				zap.Duration("jitter_delay", jitterDelay),
			)
		}
	} else {
		jitterDelay = crossAccountJitterDelay()
	}
	return sleepWithContext(ctx, jitterDelay)
}

// boundSessionJitterDelay 返回 min-max 范围内的随机抖动延迟，用于绑定会话切号 (P0-3)。
// 绑定会话切号需要更长的延迟（默认 2-10s）以进一步打散内容关联信号。
func boundSessionJitterDelay(min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		// fallback: 返回中间值
		return (min + max) / 2
	}
	// 0..65535 -> min..max
	n := int(b[0])<<8 | int(b[1])
	rangeMs := int64(max-min) / int64(time.Millisecond)
	offsetMs := int64(n) * rangeMs / 65536
	return min + time.Duration(offsetMs)*time.Millisecond
}

// HandleSelectionExhausted 处理选号失败（所有候选账号都在排除列表中）时的退避重试决策。
// 针对 Antigravity 单账号分组的 503 (MODEL_CAPACITY_EXHAUSTED) 场景：
// 清除排除列表、等待退避后重新选号。
//
// 返回 FailoverContinue 时，调用方应设置 SingleAccountRetry context 并 continue。
// 返回 FailoverExhausted 时，调用方应返回错误响应。
// 返回 FailoverCanceled 时，调用方应直接 return。
func (s *FailoverState) HandleSelectionExhausted(ctx context.Context) FailoverAction {
	if s.LastFailoverErr != nil &&
		s.LastFailoverErr.StatusCode == http.StatusServiceUnavailable &&
		s.SwitchCount <= s.MaxSwitches {

		logger.FromContext(ctx).Warn("gateway.failover_single_account_backoff",
			zap.Duration("backoff_delay", singleAccountBackoffDelay),
			zap.Int("switch_count", s.SwitchCount),
			zap.Int("max_switches", s.MaxSwitches),
		)
		if !sleepWithContext(ctx, singleAccountBackoffDelay) {
			return FailoverCanceled
		}
		logger.FromContext(ctx).Warn("gateway.failover_single_account_retry",
			zap.Int("switch_count", s.SwitchCount),
			zap.Int("max_switches", s.MaxSwitches),
		)
		s.FailedAccountIDs = make(map[int64]struct{})
		return FailoverContinue
	}
	return FailoverExhausted
}

// needForceCacheBilling 判断 failover 时是否需要强制缓存计费。
// 粘性会话切换账号、或上游明确标记时，将 input_tokens 转为 cache_read 计费。
func needForceCacheBilling(hasBoundSession bool, failoverErr *service.UpstreamFailoverError) bool {
	return hasBoundSession || (failoverErr != nil && failoverErr.ForceCacheBilling)
}

// sleepWithContext 等待指定时长，返回 false 表示 context 已取消。
func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

// GatewayStickySuccessBinding 是通用路径可迁移会话的「成功偏好」。
//
// 它按 分组 / 会话 / 模型 三维隔离（键见 repository 层），只在业务请求确认成功
// 后写入，因此与旧的 sticky_session 绑定语义不同：旧绑定表示「上一次用过谁」，
// 成功偏好表示「上一次真正成功的是谁」。Revision 用于 CAS 围栏，使晚完成的旧
// 请求无法覆盖另一个请求已提交的更新偏好。
type GatewayStickySuccessBinding struct {
	AccountID int64  `json:"account_id"`
	Revision  string `json:"revision"`
}

type gatewayStickySuccessKey struct{}

type gatewayStickyIdentityModelKey struct{}

// WithGatewayStickyIdentityModel 记录本请求的**粘性身份模型**。
//
// 成功偏好按 (分组, 会话, 模型) 隔离，这里的「模型」必须是客户端请求的、渠道
// 映射前的模型，与下列**调度模型**严格区分：
//
//   - 身份模型：只决定成功偏好键。客户端两个别名即使映射到同一个上游模型，也
//     必须各自学习、各自复用，一个别名的 failover 不得覆盖另一个的偏好。
//   - 调度模型：账号资格判定、渠道/账号模型映射与真实出站请求所用的模型。它在
//     调度栈里会被渠道映射与 composite 路由改写，因此不能兼任粘性键。
//
// 通用入口在进入选号循环前设置一次；调度器装配点优先读它，读不到才回落到自己
// 收到的调度模型（web search 等没有显式设置身份的调用方行为逐字不变）。
func WithGatewayStickyIdentityModel(ctx context.Context, model string) context.Context {
	if ctx == nil {
		return ctx
	}
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return ctx
	}
	if existing, ok := ctx.Value(gatewayStickyIdentityModelKey{}).(string); ok && existing == trimmed {
		return ctx
	}
	return context.WithValue(ctx, gatewayStickyIdentityModelKey{}, trimmed)
}

// gatewayStickyIdentityModel 返回本请求已登记的身份模型，未登记时返回空串。
func gatewayStickyIdentityModel(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	model, _ := ctx.Value(gatewayStickyIdentityModelKey{}).(string)
	return model
}

// gatewayStickySuccessModel 解析装配应使用的粘性键模型：请求级身份模型优先，
// 没有登记时回落到调度器收到的调度模型。
//
// 回落分支保证未接入身份登记的调用方（例如 gateway_web_search.go 的选号）与
// 登记前逐字同行为。
func gatewayStickySuccessModel(ctx context.Context, schedulingModel string) string {
	if identity := gatewayStickyIdentityModel(ctx); identity != "" {
		return identity
	}
	return strings.TrimSpace(schedulingModel)
}

type gatewayStickySuccessState struct {
	groupID     int64
	sessionHash string
	// model 是粘性身份模型（渠道映射前的客户端请求模型），不是出站调度模型。
	model string
	// expected 是本请求开始时观察到的偏好版本，提交时作为 CAS 的比较基准。
	expected GatewayStickySuccessBinding
	// originalID 是本请求的粘性候选：成功偏好优先，没有成功偏好时兼容读旧绑定。
	// 旧绑定只作软候选，不视为「已确认成功」，因此替代账号成功后可以直接接替。
	originalID int64
	// legacyWritesManaged 只由 OpenAI 旧调度入口使用：true 表示旧 sticky 键的
	// 写入 / 续期 / 删除也由成功偏好接管（利润控制分组，门下选号内部本就不做
	// eager 绑定，旧键唯一写入点是终检后绑定）；false 表示只接管候选来源，旧键
	// 语义逐字不变（普通分组）。通用路径只在利润门下装配，不读这个字段。
	legacyWritesManaged bool
}

// armGatewayStickySuccess 在调度器解析出**实际生效的分组**之后装配请求级成功
// 偏好状态。它必须在 checkClaudeCodeRestriction → withGroupContext → 装利润门
// 之后调用：那时 ctx 上的门、候选过滤与粘性键三者同源，都指向同一个有效分组。
//
// 无门（含降级后的分组本身没开利润控制）时写入清除值，保证上一轮/上一分组的
// 状态不会残留。
//
// schedulingModel 是调度器本轮收到的**调度模型**，只在 ctx 没有登记身份模型时
// 才被当作粘性键使用，见 gatewayStickySuccessModel。
func (s *GatewayService) armGatewayStickySuccess(ctx context.Context, groupID *int64, sessionHash, schedulingModel string) context.Context {
	if !gatewayProfitControlGateActive(ctx) {
		return clearGatewayStickySuccess(ctx)
	}
	return s.armGatewayStickySuccessGated(ctx, groupID, sessionHash, schedulingModel)
}

// armGatewayStickySuccessGated 是装配主体，调用方必须已确认本请求受利润控制。
//
// 粘性键的模型维度取身份模型（渠道映射前的客户端请求模型），只有没登记身份时
// 才回落到 schedulingModel。身份模型在整条调度栈上恒定，因此 composite 路由改写
// requestedModel、Gemini 原生入口传入映射后 modelName 都不会换键。
//
// 同一请求内同一 (有效分组, 会话, 身份模型) 的 expected 只读一次：ctx 里已带同键
// 状态时原样复用。这让 CAS 的期望版本固定在本轮 failover 开始之前，成功提交时
// 才能真正挡住「另一个请求已经提交了更新偏好」的晚到覆盖；外层 Select 与旧引擎
// 内层重复选号也因此共用同一份身份与同一份 expected。
func (s *GatewayService) armGatewayStickySuccessGated(ctx context.Context, groupID *int64, sessionHash, schedulingModel string) context.Context {
	stickyModel := gatewayStickySuccessModel(ctx, schedulingModel)
	if s == nil || s.cache == nil || sessionHash == "" || stickyModel == "" {
		return clearGatewayStickySuccess(ctx)
	}
	effectiveGroupID := derefGroupID(groupID)
	if existing := gatewayStickySuccessFromContext(ctx); existing != nil &&
		existing.groupID == effectiveGroupID && existing.sessionHash == sessionHash && existing.model == stickyModel {
		return ctx
	}
	state := &gatewayStickySuccessState{
		groupID:     effectiveGroupID,
		sessionHash: sessionHash,
		model:       stickyModel,
	}
	binding, err := s.cache.GetGatewayStickySuccess(ctx, state.groupID, state.sessionHash, state.model)
	if err != nil && !errors.Is(err, ErrStickySessionNotFound) {
		// 读失败只影响软偏好：不装配状态，本请求按门下既有候选逻辑走，
		// 不影响响应、计费或重试。
		slog.Warn("gateway.sticky_success_read_failed",
			"group_id", state.groupID, "session", shortSessionHash(state.sessionHash), "error", err)
		return clearGatewayStickySuccess(ctx)
	}
	state.expected = binding
	state.originalID = binding.AccountID
	if state.originalID == 0 {
		state.originalID, _ = s.GetCachedSessionAccountID(ctx, groupID, state.sessionHash)
	}
	return context.WithValue(ctx, gatewayStickySuccessKey{}, state)
}

// BeginGatewayStickySuccess 是 armGatewayStickySuccess 的薄封装，供不经调度栈的
// 调用方直接装配或清除状态：它自行判定本请求是否受利润控制（判定条件与装门点
// 逐字一致，但不安装门、不记录安装观测），再走同一套装配主体。
//
// 线上五个入口不再调用它——装配已移进调度器，只有那里才知道 Claude Code 限制
// 降级后真正生效的分组（gateway_scheduling.go:115-123）；它们改为在选号前用
// WithGatewayStickyIdentityModel 登记身份模型。
//
// model 与装配主体同义：ctx 已登记身份模型时以身份模型为准，这里的入参只作回落。
func (s *GatewayService) BeginGatewayStickySuccess(ctx context.Context, groupID *int64, sessionHash, model string) context.Context {
	if s == nil || !s.gatewayProfitControlConfigured(ctx, groupID) {
		return clearGatewayStickySuccess(ctx)
	}
	return s.armGatewayStickySuccessGated(ctx, groupID, sessionHash, model)
}

func gatewayStickySuccessFromContext(ctx context.Context) *gatewayStickySuccessState {
	if ctx == nil {
		return nil
	}
	// 清除标记存的是 typed-nil，断言成功但值为 nil，因此这里统一返回 nil。
	state, _ := ctx.Value(gatewayStickySuccessKey{}).(*gatewayStickySuccessState)
	return state
}

// clearGatewayStickySuccess 用 typed-nil 覆盖上一轮的成功偏好状态，只影响本
// context key，其余 context value 原样保留。没有残留时直接返回原 ctx，避免在
// 无利润门的常规流量上多包一层。
func clearGatewayStickySuccess(ctx context.Context) context.Context {
	if ctx == nil || gatewayStickySuccessFromContext(ctx) == nil {
		return ctx
	}
	return context.WithValue(ctx, gatewayStickySuccessKey{}, (*gatewayStickySuccessState)(nil))
}

// GatewayStickySuccessActive 报告本请求是否已装配成功偏好状态。handler 用它
// 区分「门下按成功维护粘性」与「无门保持官方原行为」两条出口。分组切换后由
// 新一轮 BeginGatewayStickySuccess 重建或清除状态，因此它总是反映当前这一轮。
func GatewayStickySuccessActive(ctx context.Context) bool {
	return gatewayStickySuccessFromContext(ctx) != nil
}

// gatewayStickySuccessManaged 报告本会话的粘性是否由成功偏好接管。
//
// 它是本次修复的最小边界：只有显式调用过 BeginGatewayStickySuccess 且会话哈希
// 一致的请求才走新语义。共用的绑定 helper 与抢槽续期点因此对没有接入的调用方
// （例如 OpenAI 家族自带调度器的入口）逐字保持原行为。
func gatewayStickySuccessManaged(ctx context.Context, groupID *int64, sessionHash string) bool {
	_, managed := gatewayStickySuccessCandidate(ctx, groupID, sessionHash)
	return managed
}

// gatewayStickySuccessCandidate 返回门下本请求应优先复用的粘性候选账号。
//
// 第二个返回值为 true 表示「成功偏好已接管这条（分组, 会话）的候选来源」：调用
// 方此时不得再回头读旧绑定，否则会出现「读成功偏好、下一分支又被旧绑定压回去」
// 的双来源矛盾。候选为 0 表示既无成功偏好也无旧绑定，本次不做粘性。
//
// 分组必须逐字一致：调度器可能因 Claude Code 限制把 groupID 换成降级分组
// （gateway_scheduling.go:115），handler 的兜底分组循环也会换分组；此时原分组
// 的状态一律不得作为候选来源。
func gatewayStickySuccessCandidate(ctx context.Context, groupID *int64, sessionHash string) (int64, bool) {
	state := gatewayStickySuccessFromContext(ctx)
	if state == nil || sessionHash == "" || state.sessionHash != sessionHash || state.groupID != derefGroupID(groupID) {
		return 0, false
	}
	return state.originalID, true
}

// stickySessionCandidateID 是通用选号路径读取粘性候选的唯一入口：门下取成功
// 偏好（兼容回落旧绑定），无门时逐字保持原来的旧绑定读取。
func (s *GatewayService) stickySessionCandidateID(ctx context.Context, groupID *int64, sessionHash string) (int64, error) {
	if candidate, ok := gatewayStickySuccessCandidate(ctx, groupID, sessionHash); ok {
		if candidate <= 0 {
			return 0, ErrStickySessionNotFound
		}
		return candidate, nil
	}
	if s == nil || s.cache == nil {
		return 0, ErrStickySessionNotFound
	}
	return s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
}

// SucceededForScheduling 是通用路径的调度用成功判据，与观测层的追踪状态完全
// 独立（追踪开关、采样、异步健康队列都不参与判定）。
//
// 语义：Forward 返回结果且客户端没有在流式传输过程中断开。调用方必须另行确认
// err == nil 与 ctx.Err() == nil——流中断且上游已计量的失败在通用路径走的是
// err != nil 分支，取消走 ctx.Err()，两者都不算成功。
func (r *ForwardResult) SucceededForScheduling() bool {
	return r != nil && !r.ClientDisconnect
}

// CommitGatewayStickySuccess 在业务请求确认成功后提交/续期成功偏好。
//
// 首次选号、抢到槽位、终检通过都不算成功，因此都不会走到这里；失败尝试、
// failover 中被换掉的账号、客户端取消同样不提交，也就不会给坏账号续命。同一
// 账号再次成功也走一次 CAS 换 revision，即「按成功规则续期」。
func (s *GatewayService) CommitGatewayStickySuccess(ctx context.Context, selection *AccountSelectionResult, account *Account, result *ForwardResult, err error) {
	// 状态优先取自选号结果：它就是调度器在实际生效分组上装配的那一份，
	// 因此 success key、旧键与 CAS 的 expected 天然都落在有效分组上。
	state := selection.stickySuccessState()
	if state == nil {
		state = gatewayStickySuccessFromContext(ctx)
	}
	if state == nil || account == nil || account.ID <= 0 || err != nil || ctx.Err() != nil || !result.SucceededForScheduling() {
		return
	}
	if s == nil || s.cache == nil {
		return
	}
	// 防御性校验：装配已经在有效分组上完成，这里不该再出现不一致。真出现就是
	// 程序缺陷（例如状态没有随选号结果传递），此时宁可不写，不写错组。
	if effective, known := selection.EffectiveGroupID(); known && effective != state.groupID {
		slog.Warn("gateway.sticky_success_commit_group_mismatch",
			"state_group_id", state.groupID, "effective_group_id", effective,
			"account_id", account.ID, "session", shortSessionHash(state.sessionHash))
		return
	}
	next := GatewayStickySuccessBinding{AccountID: account.ID, Revision: uuid.NewString()}
	updated, casErr := s.cache.CompareAndSwapGatewayStickySuccess(ctx, state.groupID, state.sessionHash, state.model, state.expected, next, stickySessionTTL)
	if casErr != nil {
		slog.Warn("gateway.sticky_success_commit_failed",
			"group_id", state.groupID, "account_id", account.ID,
			"session", shortSessionHash(state.sessionHash), "error", casErr)
		return
	}
	if !updated {
		// 本请求开始时观察到的版本已被另一个请求换掉：较新的成功偏好胜出，
		// 这里不重试、不回写，也不影响已经完成的响应。
		slog.Info("gateway.sticky_success_commit_stale",
			"group_id", state.groupID, "account_id", account.ID,
			"previous_account_id", state.originalID,
			"session", shortSessionHash(state.sessionHash), "model", state.model)
		return
	}
	// 旧键同步指向最后一次成功的账号：请求开始处的预取与其他只读旧键的路径
	// 因此不会把已被取代的账号再压回来。只在门下这样做，无门路径不受影响。
	groupID := state.groupID
	if bindErr := s.BindStickySession(ctx, &groupID, state.sessionHash, account.ID); bindErr != nil {
		slog.Warn("gateway.sticky_success_legacy_bind_failed",
			"group_id", state.groupID, "account_id", account.ID,
			"session", shortSessionHash(state.sessionHash), "error", bindErr)
	}
	if state.originalID != account.ID {
		slog.Info("gateway.sticky_success_rebound",
			"group_id", state.groupID,
			"previous_account_id", state.originalID,
			"account_id", account.ID,
			"session", shortSessionHash(state.sessionHash),
			"model", state.model)
	}
}

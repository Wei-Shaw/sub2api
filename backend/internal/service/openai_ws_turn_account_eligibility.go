package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	coderws "github.com/coder/websocket"
)

// WS 入口每轮账号资格复核的否决原因。原因串会同时出现在服务端告警日志和
// 发给客户端的 close reason 里，故保持稳定、可 grep。
const (
	// OpenAIWSTurnAccountIneligibleNotSchedulable 账号整体不可调度：status 非
	// active、schedulable=false、temp_unschedulable_until / rate_limit_reset_at /
	// overload_until 未到期，或被调度侧门控（调度阈值、Grok 免费额度）拦下等。
	OpenAIWSTurnAccountIneligibleNotSchedulable = "not_schedulable"
	// OpenAIWSTurnAccountIneligibleModelRateLimited 账号本身可调度，但本轮请求
	// 模型命中 model_rate_limits 限流窗口。
	OpenAIWSTurnAccountIneligibleModelRateLimited = "model_rate_limited"
	// OpenAIWSTurnAccountIneligibleRuntimeBlocked 进程内 runtime blocker 判定该
	// 账号（或该账号的该模型）当前不可用。
	OpenAIWSTurnAccountIneligibleRuntimeBlocked = "runtime_blocked"
	// OpenAIWSTurnAccountIneligibleAccountMissing 账号在快照/库里已不存在。
	OpenAIWSTurnAccountIneligibleAccountMissing = "account_missing"
)

// openAIWSTurnForwardModel 由客户端本轮模型算出实际转发的渠道模型，与 WS 入口
// 每轮走的 hooks.MapRequestModel（handler 里返回 ResolveChannelMappingAndRestrict
// 的 MappedModel）以及入口 parseClientPayload 的 requestModel 回退逐字一致：
// 渠道映射结果非空就用映射结果，否则沿用客户端模型。
//
// 资格复核必须自己算一遍而不是复用上一轮存下来的值：各 WS 入口里 MapRequestModel
// 与 BeforeTurn 的先后顺序并不一致（ctx_pool / http_bridge 入口在上一轮末尾解析
// 下一帧时就调了 MapRequestModel，passthrough 入口则在 BeforeTurn 之后才调），共用
// 同一个解析函数才是各条路都对得上的口径。
func (s *OpenAIGatewayService) openAIWSTurnForwardModel(ctx context.Context, groupID *int64, clientModel string) string {
	model := strings.TrimSpace(clientModel)
	if s == nil || model == "" {
		return model
	}
	mapping, _ := s.ResolveChannelMappingAndRestrict(ctx, groupID, model)
	if mapped := strings.TrimSpace(mapping.MappedModel); mapped != "" {
		return mapped
	}
	return model
}

// openAIWSTurnEligibilityModels 是本轮资格复核查 model_rate_limits 的「最终上游
// 模型键」集合。返回的每一项都必须是「某种 WS 入口模式下实际会写进上游 payload 的
// 模型名」，也就是 WS 入口上游报错时（handleOpenAIWSFailureAccountSideEffects 等）
// 写按模型限流用的那个键。调用方必须把这些键直接拿去查限流，不得再经过任何一次映射。
//
// 为什么返回两项而不是一项：最终模型名在不同入口模式下口径不同，而 BeforeTurn 这个
// 调用点拿不到本连接最终生效的入口模式（模式由协议解析器按账号 extra、配置默认值、
// HTTP bridge 阈值等一路裁决，且可能在连接中途回落）：
//
//   - passthrough：只替换认证，原样转发「渠道映射后的客户端模型」，即 forwardModel
//     （openai_ws_v2_passthrough_adapter.go 用 hooks.MapRequestModel 的结果
//     ReplaceModelInBody，不走账号 model_mapping、不做别名归一）。
//   - ctx_pool / http_bridge：在 forwardModel 之上再过账号 model_mapping 和上游别名
//     归一，即 openAIWSUpstreamModelForAccount(account, forwardModel)，例如 Codex
//     协议账号上 gpt-5.1 → gpt-5.4。
//
// 第二项必须走 openAIWSUpstreamModelForAccount（与 WS 各入口逐字同源），不能用
// canonicalOpenAIAccountSchedulingModel：后者带着 HTTP Forward 的 passthrough 分叉
// （resolveOpenAIAccountUpstreamModelForRequest 在 IsOpenAIPassthroughEnabled()
// 时原样返回），而 WS 入口无视该开关一律做账号映射 + 别名归一。openai_passthrough
// 账号走 ctx_pool / http_bridge 时，入口把 gpt-5.1 归一成 gpt-5.4 发上游、限流也记在
// gpt-5.4 上，若这里用 canonical 则两个候选键都是 gpt-5.1，限流直接漏检。
//
// 拿不到模式就取并集，任一候选被限流即判不合格。资格闸门的作用是断开让客户端重连换号，
// 宁可多断一次也不能漏掉已经不可用的号。
func openAIWSTurnEligibilityModels(account *Account, forwardModel string) []string {
	forwardModel = strings.TrimSpace(forwardModel)
	if forwardModel == "" {
		return nil
	}
	// passthrough 模式的最终键。
	models := []string{forwardModel}
	// ctx_pool / http_bridge 模式的最终键。
	if upstream := strings.TrimSpace(openAIWSUpstreamModelForAccount(account, forwardModel)); upstream != "" && upstream != forwardModel {
		models = append(models, upstream)
	}
	return models
}

// openAIWSTurnRuntimeBlockModels 是查模型级 runtime blocker 的键集合。它与
// openAIWSTurnEligibilityModels 分开，因为 runtime blocker 的键必须与**写入侧**
// 对齐，而写入侧用的不全是「实际发往上游的模型」口径：
//
//   - ctx_pool 入口拨号失败（openai_ws_forwarder_ingress.go 的 acquireTurnLease）记的是
//     canonicalOpenAIAccountSchedulingModel(account, ingressSessionOriginalModel)，
//     即「客户端本轮声明的模型」过 canonical。
//   - passthrough 入口（openai_ws_v2_passthrough_adapter.go）记的是
//     capturedSessionModel，即「渠道映射后的模型」= forwardModel。
//   - ctx_pool turn 内上游错误（ingress.go 的 mappedModel）与 http_bridge
//     （openai_ws_http_bridge.go 的 handleOpenAIWSFailureAccountSideEffects）记的是
//     实际发往上游的模型，即 openAIWSUpstreamModelForAccount(account, forwardModel)。
//
// 所以这里取这几种写入键的并集（去重）。非 passthrough 账号上 canonical 与
// openAIWSUpstreamModelForAccount 本来就相等，实际只会多出一两项。
//
// 已知限制（本改动不扩大范围修）：写入侧的 canonicalOpenAIAccountSchedulingModel
// 同样带着 HTTP Forward 的 passthrough 分叉，passthrough 账号走 ctx_pool 时拨号失败
// 会把 blocker 记在客户端模型上，而不是实际发往上游的归一后模型，于是同一个账号的
// blocker 会散落在两个键上。检查侧取并集所以判定仍然完整；要彻底收口得改写入侧，
// 届时本函数应收敛成只用 openAIWSUpstreamModelForAccount 一项。
//
// 永远至少返回一项（算不出模型名时返回空串）：账号级 runtime blocker 与模型无关，
// 不能因为拿不到模型名就整条跳过。
func openAIWSTurnRuntimeBlockModels(account *Account, clientModel, forwardModel string) []string {
	models := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	add := func(model string) {
		if model = strings.TrimSpace(model); model == "" {
			return
		}
		if _, ok := seen[model]; ok {
			return
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	add(canonicalOpenAIAccountSchedulingModel(account, clientModel))
	add(canonicalOpenAIAccountSchedulingModel(account, forwardModel))
	add(forwardModel)
	if strings.TrimSpace(forwardModel) != "" {
		add(openAIWSUpstreamModelForAccount(account, forwardModel))
	}
	if len(models) == 0 {
		return []string{""}
	}
	return models
}

// openAIWSTurnAccountIneligibleReason 复核 WS 长连接绑定账号对本轮转发模型是否
// 仍可调度，可调度返回空串。forwardModel 必须是渠道映射后的转发模型
// （openAIWSTurnForwardModel 的结果）。
//
// 账号状态必须是新鲜的（管理员刚停调度，下一轮就要生效），但长连接每轮打一次 DB
// 不可接受，故统一走 getSchedulableAccount：优先读调度器的 Redis 账号快照
// （outbox worker 默认 1s 轮询同步写库事件），快照缺失才回源 accountRepo；与粘性
// 会话命中时的读取口径一致（含调度阈值、Grok 免费额度等门控）。此外 runtime
// blocker 是进程内状态，写入即刻可见，不受快照延迟影响。
//
// 读取结果分三类：
//   - 仓储返回 ErrAccountNotFound（Redis 快照缺失也会回源到这条路上）说明号已被
//     删除，判不合格。
//   - 返回 (nil, nil) 说明账号被 getSchedulableAccount 的调度门控拦下（或查无此号），
//     同样判不合格。
//   - 其余读失败（缓存抖动、超时）fail-open，避免把正在跑的长连接批量踢掉，下一轮
//     还会再复核一次。
func (s *OpenAIGatewayService) openAIWSTurnAccountIneligibleReason(ctx context.Context, account *Account, clientModel string, forwardModel string) string {
	if s == nil || account == nil {
		return ""
	}
	latest := account
	if s.schedulerSnapshot != nil || s.accountRepo != nil {
		current, err := s.getSchedulableAccount(ctx, account.ID)
		switch {
		case errors.Is(err, ErrAccountNotFound):
			return OpenAIWSTurnAccountIneligibleAccountMissing
		case err != nil:
			slog.Warn("openai_ws_turn_account_refresh_failed",
				"account_id", account.ID,
				"forward_model", forwardModel,
				"error", err,
			)
		case current == nil:
			return OpenAIWSTurnAccountIneligibleNotSchedulable
		default:
			latest = current
		}
	}
	if !latest.IsSchedulable() {
		return OpenAIWSTurnAccountIneligibleNotSchedulable
	}
	// 模型级判定一律按候选键直接查，不再二次映射：openAIWSTurnEligibilityModels /
	// openAIWSTurnRuntimeBlockModels 给出的已经是最终键，若再走
	// IsSchedulableForModelWithContext / isOpenAIAccountRequestRuntimeBlocked
	// （内部各自还有一次 GetMappedModel / canonicalOpenAIAccountSchedulingModel），
	// 账号配 `*` 通配或链式映射时最终键会被映回别的模型名，记在最终键上的限制直接
	// 漏检，反过来也可能误拦本来正常的模型。
	for _, model := range openAIWSTurnEligibilityModels(latest, forwardModel) {
		if latest.isModelRateLimitedForFinalKeyWithContext(ctx, model) {
			return OpenAIWSTurnAccountIneligibleModelRateLimited
		}
	}
	// runtime blocker 单独一轮：它的键与写入侧对齐，和 model_rate_limits 的口径不同。
	for _, model := range openAIWSTurnRuntimeBlockModels(latest, clientModel, forwardModel) {
		if s.isOpenAIAccountRequestRuntimeBlockedForFinalModel(latest, model) {
			return OpenAIWSTurnAccountIneligibleRuntimeBlocked
		}
	}
	return ""
}

// releaseOpenAIWSTurnStickyBinding 解除会话与该账号的粘性绑定，保证客户端重连时
// 不会又粘回同一个已不可用的号。tryStickySessionHit 本身也会做资格检查，这里是双保险。
// 绑定若已经指向别的账号（别处已换号），不动它。
func (s *OpenAIGatewayService) releaseOpenAIWSTurnStickyBinding(ctx context.Context, groupID *int64, sessionHash string, accountID int64) {
	if s == nil || sessionHash == "" {
		return
	}
	if accountID > 0 {
		bound, err := s.getStickySessionAccountID(ctx, groupID, sessionHash)
		if err == nil && bound > 0 && bound != accountID {
			return
		}
	}
	if err := s.deleteStickySessionAccountID(ctx, groupID, sessionHash); err != nil && !errors.Is(err, ErrStickySessionNotFound) {
		slog.Warn("openai_ws_turn_account_sticky_release_failed",
			"account_id", accountID,
			"group_id", derefGroupID(groupID),
			"error", err,
		)
	}
}

// EnforceOpenAIWSTurnAccountEligibility 是 WS 入口非首轮 turn 的账号资格闸门。
//
// 背景：WS 入口（ctx_pool / http_bridge / passthrough）只在握手时选一次号，之后每轮
// 只做利润复核和并发槽位，不看账号资格。账号被管理员停调度、status 变为非 active、
// 临时下线、按模型限流（model_rate_limits）或命中 runtime blocker 后，上游往往仍能
// 正常回答，已建立的连接会一直用这个号跑到客户端自己断开；HTTP 路径没有这个问题
// （粘性会话每次请求都过资格检查）。
//
// 本函数在 BeforeTurn（客户端下一轮 response.create 已到、尚未写上游）判定：不合格
// 就解除粘性绑定、记一次换号计数，并返回一个 TryAgainLater 的客户端 close error。
// Codex CLI 在两轮之间收到服务端关闭后会自动重试并新建 WS，握手时重新选号，该轮
// 正常完成，对用户无感。
//
// 合格返回 ("", nil)。首轮由握手准入负责，调用方不应对 turn==1 调用本函数。
//
// clientModel 传客户端本轮 response.create 声明的模型（省略 model 时传会话实际
// 生效的模型）。渠道映射、账号级 model_mapping 和上游别名归一都在本函数内部按与
// 转发相同的口径复算，调用方不要自己先映射一遍。
func (s *OpenAIGatewayService) EnforceOpenAIWSTurnAccountEligibility(
	ctx context.Context,
	account *Account,
	groupID *int64,
	sessionHash string,
	clientModel string,
) (string, error) {
	if s == nil || account == nil {
		return "", nil
	}
	forwardModel := s.openAIWSTurnForwardModel(ctx, groupID, clientModel)
	reason := s.openAIWSTurnAccountIneligibleReason(ctx, account, clientModel, forwardModel)
	if reason == "" {
		return "", nil
	}
	// 断开前先解除粘性：重连的握手选号必须有机会换号。
	s.releaseOpenAIWSTurnStickyBinding(ctx, groupID, sessionHash, account.ID)
	// 客户端重连后会重新选号，计入调度器的换号计数，与 failover 换号口径一致。
	s.RecordOpenAIAccountSwitch()
	return reason, NewOpenAIWSClientCloseError(
		coderws.StatusTryAgainLater,
		fmt.Sprintf("account no longer schedulable (%s); please reconnect", reason),
		nil,
	)
}

package service

import (
	"context"
	"strings"
)

// 分组模型路由（Group.ModelRouting）在 OpenAI 系调度栈上的接线。
//
// 规则的存储与查表（Group.GetRoutingAccountIDs）本身与平台无关，但 openai / grok /
// kimi / zhipu / deepseek 分组的 Responses、Chat Completions 与 Messages 全部由
// OpenAIGatewayService 承接，那套调度独立于通用网关，此前从不查表——规则可以保存、
// 可以展示，却对调度没有任何影响。
//
// 语义与 Anthropic 侧保持一致：
//   - 路由是"优先账号集合"，不是硬限定。路由账号在全部资格门之后仍无一可用时，
//     回落普通调度，而不是让请求失败。
//   - 普通会话粘性不得压过有效路由：粘性账号落在路由集合之外时该绑定让位。
//   - previous_response_id、guardian parent 与 WebSocket 续话绑定不在此列。
//     这些是会话正确性约束而非调度偏好，按既有可迁移条件处理。

type openAIModelRoutingContextKey struct{}

// openAIModelRoutingSnapshot 缓存单次请求解析出的路由账号集合，避免同一请求链路
// （粘性 gate、候选过滤、failover 重入）重复读取分组。
type openAIModelRoutingSnapshot struct {
	groupID    int64
	model      string
	accountIDs []int64
}

// WithOpenAIModelRouting 在调度入口解析一次分组模型路由并缓存到 context。
func (s *OpenAIGatewayService) WithOpenAIModelRouting(ctx context.Context, groupID *int64, platform string, requestedModel string) context.Context {
	if s == nil || ctx == nil {
		return ctx
	}
	model := modelRoutingLookupModel(ctx, requestedModel)
	if existing, ok := ctx.Value(openAIModelRoutingContextKey{}).(openAIModelRoutingSnapshot); ok &&
		existing.groupID == derefGroupID(groupID) && existing.model == model {
		return ctx
	}
	return context.WithValue(ctx, openAIModelRoutingContextKey{}, openAIModelRoutingSnapshot{
		groupID:    derefGroupID(groupID),
		model:      model,
		accountIDs: s.resolveOpenAIRoutedAccountIDs(ctx, groupID, platform, model),
	})
}

// openAIRoutedAccountIDs 读取本次请求的路由账号集合，未装门时按需解析。
func (s *OpenAIGatewayService) openAIRoutedAccountIDs(ctx context.Context, groupID *int64, platform string, requestedModel string) []int64 {
	if s == nil || ctx == nil {
		return nil
	}
	model := modelRoutingLookupModel(ctx, requestedModel)
	if cached, ok := ctx.Value(openAIModelRoutingContextKey{}).(openAIModelRoutingSnapshot); ok &&
		cached.groupID == derefGroupID(groupID) && cached.model == model {
		return cached.accountIDs
	}
	return s.resolveOpenAIRoutedAccountIDs(ctx, groupID, platform, model)
}

// modelRoutingLookupModel 决定用哪个模型名查表。两套调度栈共用。
//
// 管理员在后台按"客户端书写的公开别名"配置规则。Anthropic 的 /v1/messages 恰好把
// 该名字直接传给调度（parsedReq.Model），但这只是巧合：OpenAI 系 handler 传的是渠道
// 映射后的 forwardModel，Gemini handler 会用 channelMapping.MappedModel 覆盖 modelName，
// composite 中间件还会改写请求体里的模型名。任何一条这样的路径直接拿调度参数查表，
// 规则都会漏配，因此统一优先使用请求链路上记录的公开别名，拿不到才回落调度参数。
func modelRoutingLookupModel(ctx context.Context, requestedModel string) string {
	if publicModel, ok := RequestedPublicModelFromContext(ctx); ok {
		return publicModel
	}
	return strings.TrimSpace(requestedModel)
}

func (s *OpenAIGatewayService) resolveOpenAIRoutedAccountIDs(ctx context.Context, groupID *int64, platform string, model string) []int64 {
	if groupID == nil || model == "" || s.schedulerSnapshot == nil {
		return nil
	}
	group, err := s.schedulerSnapshot.GetGroupByIDLite(ctx, *groupID)
	if err != nil || group == nil {
		return nil
	}
	if !openAIModelRoutingAppliesToGroup(group, platform) {
		return nil
	}
	return group.GetRoutingAccountIDs(model)
}

// openAIModelRoutingAppliesToGroup 判断分组的路由规则是否适用于本次调度。
//
// 本函数只放行"分组自身平台就是本次目标平台"以及 composite——后者的目标平台已由
// composite 路由解析确定。它不额外放宽跨平台调度权限：候选账号仍由既有的分组归属与
// 平台过滤决定，路由只能在这些候选之内表达偏好。
func openAIModelRoutingAppliesToGroup(group *Group, targetPlatform string) bool {
	if group == nil {
		return false
	}
	if group.Platform == PlatformComposite {
		return true
	}
	return group.Platform == targetPlatform
}

type openAINonMovableContinuationContextKey struct{}

// withOpenAINonMovableContinuation 标记本次请求带有不可迁移的续话绑定
// （previous_response_id 且 previousResponseCanMove=false）。
//
// 高级调度有独立的 previous_response 层，能自己识别这类绑定；legacy 调度没有，
// 该绑定完全由会话粘性承载。若不加区分地让粘性为路由让步，legacy 路径会把不可
// 迁移的续话踢到另一个账号上，续话直接失败。
func withOpenAINonMovableContinuation(ctx context.Context) context.Context {
	if ctx == nil {
		return ctx
	}
	if openAIHasNonMovableContinuation(ctx) {
		return ctx
	}
	return context.WithValue(ctx, openAINonMovableContinuationContextKey{}, true)
}

func openAIHasNonMovableContinuation(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	flagged, _ := ctx.Value(openAINonMovableContinuationContextKey{}).(bool)
	return flagged
}

// openAIStickyBindingBlockedByRouting 判断一条会话粘性绑定是否应为路由让位。
//
// 必须以真正解析出来的绑定账号 ID 调用：部分入口把 stickyAccountID 传 0，由
// tryStickySessionHit 自己从缓存读出绑定，只看入参会漏掉这些请求。
func (s *OpenAIGatewayService) openAIStickyBindingBlockedByRouting(ctx context.Context, routedIDs []int64, accountID int64) bool {
	if accountID <= 0 || len(routedIDs) == 0 {
		return false
	}
	if openAIHasNonMovableContinuation(ctx) {
		return false
	}
	return !openAIRoutingAllowsAccount(routedIDs, accountID)
}

// openAIRoutingAllowsAccount 判断账号是否落在路由集合内。空集合表示未命中路由，
// 此时不施加任何约束。
func openAIRoutingAllowsAccount(routedIDs []int64, accountID int64) bool {
	if len(routedIDs) == 0 {
		return true
	}
	for _, id := range routedIDs {
		if id == accountID {
			return true
		}
	}
	return false
}

// openAIAllAccountsAtCapacity 判断一组候选是否全部已知满载。
//
// 用于路由优先轮：路由账号确实占满时应把请求让给空闲的备用账号，而不是排队等它，
// 这也是 Anthropic 侧在同样条件下的行为。"负载读数未满、只是抢槽失败"不属于此列，
// 那种情况仍返回等待计划以保持账号亲和。
//
// 读数缺失（并发服务不可用或批量查询失败）时返回 false：无从判断就不抑制等待，
// 保持既有行为，不因为一次读数失败把请求推到别的账号上。
func openAIAllAccountsAtCapacity(accounts []*Account, loadMap map[int64]*AccountLoadInfo) bool {
	if len(accounts) == 0 || len(loadMap) == 0 {
		return false
	}
	for _, account := range accounts {
		if account == nil {
			return false
		}
		info, ok := loadMap[account.ID]
		if !ok || info == nil || info.LoadRate < 100 {
			return false
		}
	}
	return true
}

// openAIRoutedAccountSubset 返回候选中落在路由集合内的子集（保持原有顺序）。
// 返回空表示没有可用的路由账号，调用方据此回落普通调度。
func openAIRoutedAccountSubset(accounts []*Account, routedIDs []int64) []*Account {
	if len(routedIDs) == 0 || len(accounts) == 0 {
		return nil
	}
	subset := make([]*Account, 0, len(accounts))
	for _, account := range accounts {
		if account != nil && openAIRoutingAllowsAccount(routedIDs, account.ID) {
			subset = append(subset, account)
		}
	}
	return subset
}

// openAIRoutedAccountValueSubset 是 openAIRoutedAccountSubset 的值切片版本，
// 供仍按 []Account 传递候选的 legacy 选择路径使用。
func openAIRoutedAccountValueSubset(accounts []Account, routedIDs []int64) []Account {
	if len(routedIDs) == 0 || len(accounts) == 0 {
		return nil
	}
	subset := make([]Account, 0, len(accounts))
	for i := range accounts {
		if openAIRoutingAllowsAccount(routedIDs, accounts[i].ID) {
			subset = append(subset, accounts[i])
		}
	}
	return subset
}

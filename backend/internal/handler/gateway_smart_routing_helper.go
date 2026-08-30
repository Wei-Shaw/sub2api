package handler

import (
	"context"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ResolveSmartRoutingCandidates implements middleware.SmartRoutingResolver.
// 供路由层的 SmartRoutingMiddleware 在分流前解析候选分组。
func (h *GatewayHandler) ResolveSmartRoutingCandidates(ctx context.Context, apiKey *service.APIKey, requestedModel string) ([]*service.Group, error) {
	if h == nil || h.gatewayService == nil {
		return nil, nil
	}
	return h.gatewayService.ResolveSmartRoutingCandidates(ctx, apiKey, requestedModel)
}

// resolveSmartRoutingGroup 为本次请求应用智能路由：
//   - 未启用智能路由：返回 nil（保持 apiKey.GroupID 原样）。
//   - 已启用：根据请求模型解析首选目标分组，若失败返回 error；成功后把解析出的分组写入
//     apiKey.GroupID / apiKey.Group，并把分组放入请求上下文（ctxkey.Group），
//     使后续计费、选号、用量等全部按该分组走。
//
// 返回的 *service.Group 为解析后的首选分组（未启用时为 nil）。
func (h *GatewayHandler) resolveSmartRoutingGroup(c *gin.Context, apiKey *service.APIKey, reqModel string) (*service.Group, error) {
	candidates, err := h.resolveSmartRoutingCandidates(c, apiKey, reqModel)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	return candidates[0], nil
}

// resolveSmartRoutingCandidates 为本次请求解析智能路由候选分组序列。
//   - 未启用智能路由：返回 nil（保持 apiKey.GroupID 原样）。
//   - 已启用：优先复用路由层 SmartRoutingMiddleware 已解析并写入 context 的候选序列
//     （避免重复解析）；否则自行解析，并把首选分组写入
//     apiKey.GroupID / apiKey.Group 与请求上下文（ctxkey.Group）。
//
// 调用方可在分组级 failover 时按序列依次降级（见 applySmartRoutingGroup）。
func (h *GatewayHandler) resolveSmartRoutingCandidates(c *gin.Context, apiKey *service.APIKey, reqModel string) ([]*service.Group, error) {
	if apiKey == nil || !apiKey.SmartRoutingEnabled {
		return nil, nil
	}
	// 优先复用路由层中间件已解析的候选序列
	if candidates := smartRoutingCandidatesFromRequestContext(c); len(candidates) > 0 {
		return candidates, nil
	}
	candidates, err := h.gatewayService.ResolveSmartRoutingCandidates(c.Request.Context(), apiKey, reqModel)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, errors.New("smart routing: resolved candidate list is empty")
	}
	apiKey.GroupID = &candidates[0].ID
	apiKey.Group = candidates[0]
	if service.IsGroupContextValid(candidates[0]) {
		ctx := context.WithValue(c.Request.Context(), ctxkey.Group, candidates[0])
		c.Request = c.Request.WithContext(ctx)
	}
	return candidates, nil
}

// smartRoutingCandidatesFromRequestContext 从请求上下文读取路由层中间件解析的候选序列。
func smartRoutingCandidatesFromRequestContext(c *gin.Context) []*service.Group {
	if c == nil || c.Request == nil {
		return nil
	}
	candidates, _ := c.Request.Context().Value(ctxkey.SmartRoutingCandidates).([]*service.Group)
	return candidates
}

// applySmartRoutingGroup 在分组级 failover 时把当前生效分组切换到候选序列中的下一分组：
// 更新 apiKey.GroupID / apiKey.Group 与请求上下文（ctxkey.Group）。
// 返回 false 表示候选序列已耗尽（所有候选分组均失败）。
func applySmartRoutingGroup(c *gin.Context, apiKey *service.APIKey, candidates []*service.Group, currentIndex int) (*service.Group, int, bool) {
	if apiKey == nil || len(candidates) == 0 {
		return nil, currentIndex, false
	}
	next := currentIndex + 1
	if next >= len(candidates) {
		return nil, currentIndex, false
	}
	group := candidates[next]
	apiKey.GroupID = &group.ID
	apiKey.Group = group
	if service.IsGroupContextValid(group) {
		ctx := context.WithValue(c.Request.Context(), ctxkey.Group, group)
		c.Request = c.Request.WithContext(ctx)
	}
	return group, next, true
}

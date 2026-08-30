package middleware

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// SmartRoutingResolver 智能路由预解析接口，由路由层注入。
type SmartRoutingResolver interface {
	ResolveSmartRoutingCandidates(ctx context.Context, apiKey *service.APIKey, requestedModel string) ([]*service.Group, error)
}

// SmartRoutingMiddleware 为启用了智能路由的 API Key 在路由分流前解析首选分组。
//
// 背景：路由层根据 getGroupPlatform(c) 决定走 OpenAI 网关还是通用网关，
// 而智能路由 key 认证后 apiKey.Group 为 nil，导致 getGroupPlatform 返回 "",
// 请求被错误路由到 Anthropic 协议路径，deepseek/openai 等 OpenAI 兼容账号
// 收到 Anthropic 格式请求而 401。
//
// 本中间件：
//  1. 读取请求体中的 model（仅当 body 是 JSON 时）；
//  2. 调用 resolver 解析候选分组序列；
//  3. 把首选分组写入 apiKey.GroupID / apiKey.Group（内存中，不落库），
//     使 getGroupPlatform 返回正确平台、路由分流正确；
//  4. 把完整候选序列写入 context（ctxkey.SmartRoutingCandidates），
//     供 handler 分组级 failover 使用；
//  5. 恢复请求体，保证下游 handler 可正常读取。
func SmartRoutingMiddleware(resolver SmartRoutingResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, ok := GetAPIKeyFromContext(c)
		if !ok || apiKey == nil || !apiKey.SmartRoutingEnabled {
			c.Next()
			return
		}
		if resolver == nil {
			c.Next()
			return
		}

		// 读取请求体（读后恢复）
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Next()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		// 仅对 JSON body 提取 model
		model := ""
		if len(body) > 0 && body[0] == '{' {
			if gjson.ValidBytes(body) {
				if m := gjson.GetBytes(body, "model"); m.Exists() && m.Type == gjson.String {
					model = m.String()
				}
			}
		}
		// 部分端点（如 /v1beta/models/:model:generateContent）model 在 URL 中，
		// 此时解析不到 model 直接放行，由 handler 内嵌的 resolveSmartRoutingCandidates 处理。
		if model == "" {
			c.Next()
			return
		}

		candidates, err := resolver.ResolveSmartRoutingCandidates(c.Request.Context(), apiKey, model)
		if err != nil {
			// 解析失败：无候选分组，按未分组处理（由 requireGroup 中间件或 handler 报错）
			c.Next()
			return
		}
		if len(candidates) == 0 {
			c.Next()
			return
		}

		// 把首选分组写入 apiKey（内存中），使路由分流正确
		group := candidates[0]
		apiKey.GroupID = &group.ID
		apiKey.Group = group
		// 写入 context（ctxkey.Group 供下游选号/计费使用）
		if service.IsGroupContextValid(group) {
			ctx := context.WithValue(c.Request.Context(), ctxkey.Group, group)
			c.Request = c.Request.WithContext(ctx)
		}
		// 写入候选序列供 handler 分组级 failover 使用
		ctx := context.WithValue(c.Request.Context(), ctxkey.SmartRoutingCandidates, candidates)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// SmartRoutingCandidatesFromContext 从请求上下文读取智能路由候选分组序列。
func SmartRoutingCandidatesFromContext(ctx context.Context) []*service.Group {
	if ctx == nil {
		return nil
	}
	candidates, _ := ctx.Value(ctxkey.SmartRoutingCandidates).([]*service.Group)
	return candidates
}

// ApplySmartRoutingGroupFromContext 应用智能路由候选分组到 apiKey 与上下文。
// 返回 false 表示候选序列耗尽。
func ApplySmartRoutingGroupFromContext(c *gin.Context, apiKey *service.APIKey, currentIndex int) (int, bool) {
	candidates := SmartRoutingCandidatesFromContext(c.Request.Context())
	if apiKey == nil || len(candidates) == 0 {
		return currentIndex, false
	}
	next := currentIndex + 1
	if next >= len(candidates) {
		return currentIndex, false
	}
	group := candidates[next]
	apiKey.GroupID = &group.ID
	apiKey.Group = group
	if service.IsGroupContextValid(group) {
		ctx := context.WithValue(c.Request.Context(), ctxkey.Group, group)
		c.Request = c.Request.WithContext(ctx)
	}
	return next, true
}

// ErrSmartRoutingNoCandidates 表示智能路由无候选分组（供 handler 直接报错）。
var ErrSmartRoutingNoCandidates = errors.New("smart routing: no eligible group can serve the requested model")

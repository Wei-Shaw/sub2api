package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// resolveMessagesRequestPlatform 判定 /v1/messages 这次请求该按哪个平台的协议转发。
//
// 优先级：强制平台（/antigravity 路由）> composite 解析出的目标平台 > 跨组模型路由
// （issue #82）命中的目标分组平台 > key 绑定的源分组平台。
//
// 跨组那一条不能少：分发组（platform=anthropic 的聚合组）里发 gemini-* 命中路由后，账号
// 调度已按目标分组选出 gemini 账号；若这里仍按源组判成 anthropic，就会拿 gemini 的
// access_token 去请求 Anthropic 上游，稳定返回 401 invalid bearer token。同一判定在
// openAICompatibleRequestPlatform（OpenAI 兼容端点）里也有一份，两处要保持一致。
func resolveMessagesRequestPlatform(c *gin.Context, apiKey *service.APIKey) string {
	if c == nil {
		if apiKey != nil && apiKey.Group != nil {
			return apiKey.Group.Platform
		}
		return ""
	}
	if forcePlatform, ok := middleware2.GetForcePlatformFromContext(c); ok {
		return forcePlatform
	}
	if c.Request != nil {
		ctx := c.Request.Context()
		if resolvedPlatform, ok := service.ResolvedTargetPlatformFromContext(ctx); ok {
			return resolvedPlatform
		}
		if effectivePlatform, ok := ctx.Value(ctxkey.EffectiveGroupPlatform).(string); ok && effectivePlatform != "" {
			return effectivePlatform
		}
	}
	if apiKey != nil && apiKey.Group != nil {
		return apiKey.Group.Platform
	}
	return ""
}

// Package routes — support.go 注册客服工单系统路由。
//
// 用户端路由挂在 `/api/v1/support` 子组下，统一走 JWTAuthMiddleware + BackendModeUserGuard，
// 与 user.go 内其他用户接口保持一致；
// admin 端路由挂在 `/api/v1/admin/support` 子组下，由 admin.go 的 admin 组承载（自带
// AdminAuthMiddleware 鉴权）。
//
// 之所以单独抽到 support.go：工单是横跨用户端 + admin 端的一个独立 capability，
// 集中注册让上下游一目了然，也避免在 user.go / admin.go 主入口里继续堆函数。
//
// 客服浮窗（add-support-chat-widget）也挂在这里：
//   - POST /api/v1/support/chat        ── auth 由 handler 内部根据 anonymous_llm 决定
//   - GET  /api/v1/support/chat/faqs   ── 公开端点 + IP 限流
package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	corerl "github.com/Wei-Shaw/sub2api/internal/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterSupportRoutes 注册用户端工单 + 客服浮窗路由。
//
// 路由列表（前缀 /api/v1/support）：
//   - POST   /tickets               创建工单（需登录）
//   - GET    /tickets               分页拉取自己的工单（需登录）
//   - GET    /tickets/:id           工单详情（需登录）
//   - POST   /tickets/:id/replies   追加回复（需登录）
//   - POST   /tickets/:id/close     关闭工单（需登录）
//   - GET    /categories            分类下拉与默认优先级（需登录）
//   - POST   /chat                  客服浮窗 SSE 对话（auth 在 handler 内根据 anonymous_llm 决定）
//   - GET    /chat/faqs             客服浮窗 FAQ 列表（公开 + IP 限流）
//
// 工单类路由用 jwtAuth + BackendModeUserGuard 中间件链；
// chat 路由因为可能匿名访问，单独挂在不带 jwtAuth 的子组上，**但仍要挂
// optionalJWTAuth**：让 handler 内部 GetAuthSubjectFromContext 在已登录访客上也能拿到
// AuthSubject，否则 anonymous_llm=false 时会把所有人（含登录用户）当匿名 401。
func RegisterSupportRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth servermiddleware.JWTAuthMiddleware,
	optionalJWTAuth servermiddleware.OptionalJWTAuthMiddleware,
	settingService *service.SettingService,
	rateLimiter *corerl.RateLimiter,
) {
	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(servermiddleware.BackendModeUserGuard(settingService))
	{
		support := authenticated.Group("/support")
		{
			support.GET("/categories", h.SupportTicket.ListCategories)

			tickets := support.Group("/tickets")
			{
				tickets.POST("", h.SupportTicket.Create)
				tickets.GET("", h.SupportTicket.List)
				tickets.GET("/:id", h.SupportTicket.Get)
				tickets.POST("/:id/replies", h.SupportTicket.AppendReply)
				tickets.POST("/:id/close", h.SupportTicket.Close)
			}
		}
	}

	// 客服浮窗：chat / faqs 挂在不带强制 jwtAuth 的子组上。
	// PostChat handler 自己根据 settings.support_chat_anonymous_llm 决定是否要求登录。
	// 仍然挂 BackendModeUserGuard，避免后端模式下匿名用户绕过这层保护。
	publicSupport := v1.Group("/support")
	publicSupport.Use(servermiddleware.BackendModeUserGuard(settingService))
	{
		chat := publicSupport.Group("/chat")
		// optionalJWTAuth：带 token 就把 AuthSubject 写入 context，没带则放行。**关键**：
		// 没有这个中间件时，登录用户的请求也走不到 GetAuthSubjectFromContext 的成功分支，
		// anonymous_llm=false 的配置下会把所有访问当匿名 401（哪怕用户已经登录）。
		chat.Use(gin.HandlerFunc(optionalJWTAuth))
		// 60 req/min/IP 公共限流：覆盖 chat/faqs 这种匿名 GET，POST /chat 内部还有按 user/IP 三层限流。
		if rateLimiter != nil {
			chat.Use(rateLimiter.Limit("support_chat_endpoint", 60, time.Minute))
		}
		{
			chat.POST("", h.SupportChat.PostChat)
			chat.GET("/faqs", h.SupportChat.GetFaqs)
		}
	}
}

// registerAdminSupportRoutes 注册 admin 端工单 + FAQ 路由。
//
// 由 admin.go 的 RegisterAdminRoutes 统一调用，admin 子组已经携带 AdminAuthMiddleware。
//
// 路由列表（前缀 /api/v1/admin/support）：
//   - GET    /tickets                 分页 + 多维过滤
//   - GET    /tickets/:id             详情（含 chat_context）
//   - POST   /tickets/:id/replies     admin 回复（自动 open → in_progress）
//   - PATCH  /tickets/:id             修改状态/优先级/分类（拒绝 reopen closed）
//   - GET    /faqs                    FAQ 列表（含 indexed 标记）
//   - POST   /faqs                    新建 FAQ（同步重算 embedding）
//   - GET    /faqs/:id                FAQ 详情
//   - PUT    /faqs/:id                更新 FAQ（部分字段；改文本时重算 embedding）
//   - DELETE /faqs/:id                删除 FAQ
//   - POST   /faqs/reindex            批量重算（?mode=all|missing）
//   - POST   /doc-index/rebuild       异步触发文档抓取/重建索引
//   - GET    /doc-index/status        查询最近一次 pipeline 状态
//   - POST   /doc-index/purge         清空全部 doc chunks
//   - POST   /chat/test-llm-connection 探测 admin 录入的外部 LLM base_url+api_key
func registerAdminSupportRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	support := admin.Group("/support")
	{
		tickets := support.Group("/tickets")
		{
			tickets.GET("", h.Admin.SupportTicket.List)
			tickets.GET("/:id", h.Admin.SupportTicket.Get)
			tickets.POST("/:id/replies", h.Admin.SupportTicket.AppendReply)
			tickets.PATCH("/:id", h.Admin.SupportTicket.Patch)
		}

		faqs := support.Group("/faqs")
		{
			faqs.GET("", h.Admin.SupportFaq.List)
			faqs.POST("", h.Admin.SupportFaq.Create)
			faqs.POST("/reindex", h.Admin.SupportFaq.Reindex)
			faqs.GET("/:id", h.Admin.SupportFaq.Get)
			faqs.PUT("/:id", h.Admin.SupportFaq.Update)
			faqs.DELETE("/:id", h.Admin.SupportFaq.Delete)
		}

		docIndex := support.Group("/doc-index")
		{
			docIndex.POST("/rebuild", h.Admin.SupportDocIndex.Rebuild)
			docIndex.GET("/status", h.Admin.SupportDocIndex.Status)
			docIndex.POST("/purge", h.Admin.SupportDocIndex.Purge)
		}

		// 客服 LLM 凭据探活（change-support-chat-external-llm §4）：
		// 复用用户端 SupportChatHandler（共享 SettingService + masked-sentinel 解析逻辑），
		// 该方法本身不读用户上下文，只做出站 HTTP 探测；admin 鉴权由 admin 路由组中间件保证。
		chat := support.Group("/chat")
		{
			chat.POST("/test-llm-connection", h.SupportChat.TestLLMConnection)
		}
	}
}

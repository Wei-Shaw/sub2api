package inbox

import (
	"encoding/json"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// UserIDFunc 从 gin 上下文提取当前登录用户 id。由路由层注入（适配鉴权中间件），
// 使 inbox 模块不直接依赖 server/middleware，避免 service→inbox 反向依赖形成环。
type UserIDFunc func(c *gin.Context) (userID int64, ok bool)

// Handler 提供信箱的 REST 端点。
type Handler struct {
	svc       *Service
	publisher Publisher
	getUserID UserIDFunc
}

// NewHandler 构造 REST handler。
func NewHandler(svc *Service, publisher Publisher, getUserID UserIDFunc) *Handler {
	return &Handler{svc: svc, publisher: publisher, getUserID: getUserID}
}

// Catchup 处理 GET /inbox/catchup?since=<seq>，返回自 since 之后的消息与权威水位。
func (h *Handler) Catchup(c *gin.Context) {
	userID, ok := h.getUserID(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}
	var since int64
	if raw := c.Query("since"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			response.BadRequest(c, "invalid since")
			return
		}
		since = v
	}
	res, err := h.svc.Catchup(c.Request.Context(), userID, since)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, res)
}

// ackRequest 是 ack 端点的请求体。
type ackRequest struct {
	Seq int64 `json:"seq"`
}

// Ack 处理 POST /inbox/ack，把用户已读水位抬升到 seq。
func (h *Handler) Ack(c *gin.Context) {
	userID, ok := h.getUserID(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}
	var req ackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	if err := h.svc.Ack(c.Request.Context(), userID, req.Seq); response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, gin.H{"acked_seq": req.Seq})
}

// UnreadCount 处理 GET /inbox/unread-count，返回未读数与是否被截断。
func (h *Handler) UnreadCount(c *gin.Context) {
	userID, ok := h.getUserID(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}
	count, truncated, err := h.svc.UnreadCount(c.Request.Context(), userID)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, gin.H{"count": count, "truncated": truncated})
}

// broadcastRequest 是管理端发布广播的请求体。
type broadcastRequest struct {
	Namespace string          `json:"namespace"`
	DedupKey  string          `json:"dedup_key"`
	Targeting json.RawMessage `json:"targeting"`
	Payload   json.RawMessage `json:"payload"`
}

// Broadcast 处理管理端 POST /admin/inbox/broadcast，发布一条广播消息。
func (h *Handler) Broadcast(c *gin.Context) {
	var req broadcastRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	created, seq, err := h.publisher.PublishBroadcast(c.Request.Context(), PublishBroadcastInput(req))
	if response.ErrorFrom(c, err) {
		return
	}
	// 管理端操作审计（tasks 12.4）：记录发布者、命名空间、dedup_key 与分配的 seq。
	// 仅记录元数据，不打印 payload / targeting 内容（脱敏）。
	adminID, _ := h.getUserID(c)
	logger.LegacyPrintf("inbox.admin",
		"[Inbox][Audit] admin=%d broadcast namespace=%s dedup_key=%s seq=%d created=%t",
		adminID, req.Namespace, req.DedupKey, seq, created)
	response.Success(c, gin.H{"seq": seq, "created": created})
}

// parsePageParams 解析 ?page= & ?page_size= 查询参数（缺省交给 service 归一化）。
func parsePageParams(c *gin.Context) (page, pageSize int) {
	if v, err := strconv.Atoi(c.Query("page")); err == nil {
		page = v
	}
	if v, err := strconv.Atoi(c.Query("page_size")); err == nil {
		pageSize = v
	}
	return page, pageSize
}

// AdminListBroadcasts 处理管理端 GET /admin/inbox/broadcasts?namespace=&page=&page_size=。
func (h *Handler) AdminListBroadcasts(c *gin.Context) {
	page, pageSize := parsePageParams(c)
	res, err := h.svc.ListBroadcasts(c.Request.Context(), c.Query("namespace"), page, pageSize)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, res)
}

// AdminListDirectMessages 处理管理端 GET /admin/inbox/direct-messages?namespace=&user_id=&page=&page_size=。
func (h *Handler) AdminListDirectMessages(c *gin.Context) {
	var userID int64
	if raw := c.Query("user_id"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			response.BadRequest(c, "invalid user_id")
			return
		}
		userID = v
	}
	page, pageSize := parsePageParams(c)
	res, err := h.svc.ListDirectMessages(c.Request.Context(), c.Query("namespace"), userID, page, pageSize)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, res)
}

package admin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type zhipuResetPollRequest struct {
	AttemptID string `json:"attempt_id" binding:"required,max=128"` // 发起接口返回的本地标识。
}

type zhipuResetActionRequest struct {
	BindingID string `json:"binding_id" binding:"required,max=128"`               // 目标绑定的当前版本。
	ResetType string `json:"reset_type" binding:"omitempty,oneof=FIVE_HOUR WEEK"` // 用卡窗口；领取时不填。
}

func (h *AccountHandler) zhipuResetActor(c *gin.Context) (id, actor int64, ok bool) {
	// 路由继承管理员鉴权与审计；用户浏览器授权必须由完整管理员会话操作。
	subject, signedIn := middleware.GetAuthSubjectFromContext(c)
	if !signedIn || subject.UserID <= 0 || adminaccess.FromContext(c.Request.Context()) != nil ||
		c.GetString("auth_method") != service.AuditAuthMethodJWT {
		response.Forbidden(c, "Administrator browser session required")
		return 0, 0, false
	}
	if h.zhipuResetCards == nil {
		response.ErrorFrom(c, service.ErrZhipuResetUnavailable)
		return 0, 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, 0, false
	}
	c.Header("Cache-Control", "no-store")
	return id, subject.UserID, true
}

// GetZhipuResetCards 读取目标账号的个人套餐重置卡，不触发领取或使用。
func (h *AccountHandler) GetZhipuResetCards(c *gin.Context) {
	id, _, ok := h.zhipuResetActor(c)
	if !ok {
		return
	}
	result, err := h.zhipuResetCards.Status(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// StartZhipuResetAuthorization 返回在智谱官方页面批准本次绑定的地址。
func (h *AccountHandler) StartZhipuResetAuthorization(c *gin.Context) {
	id, actor, ok := h.zhipuResetActor(c)
	if !ok {
		return
	}
	result, err := h.zhipuResetCards.Start(c.Request.Context(), id, actor)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// PollZhipuResetAuthorization 检查已发起的授权，仅保存经官方卡片查询验证的会话。
func (h *AccountHandler) PollZhipuResetAuthorization(c *gin.Context) {
	id, actor, ok := h.zhipuResetActor(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req zhipuResetPollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid GLM authorization attempt")
		return
	}
	result, err := h.zhipuResetCards.Poll(c.Request.Context(), id, actor, req.AttemptID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if result.Status == "pending" {
		middleware.SkipAudit(c)
	}
	response.Success(c, result)
}

// UnbindZhipuResetAuthorization 删除本地重置卡授权，不改推理 Key。
func (h *AccountHandler) UnbindZhipuResetAuthorization(c *gin.Context) {
	id, _, ok := h.zhipuResetActor(c)
	if !ok {
		return
	}
	if err := h.zhipuResetCards.Unbind(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

// ClaimZhipuResetCard 手动向智谱申请一次领取机会。
func (h *AccountHandler) ClaimZhipuResetCard(c *gin.Context) {
	h.actZhipuResetCard(c, "opportunity")
}

// UseZhipuResetCard 手动消耗指定窗口的一张卡，使用上游与本地双层幂等。
func (h *AccountHandler) UseZhipuResetCard(c *gin.Context) {
	h.actZhipuResetCard(c, "use")
}

func (h *AccountHandler) actZhipuResetCard(c *gin.Context, op string) {
	id, _, ok := h.zhipuResetActor(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req zhipuResetActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid GLM reset-card action")
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		response.ErrorFrom(c, service.ErrIdempotencyKeyRequired)
		return
	}
	if service.DefaultIdempotencyCoordinator() == nil {
		response.ErrorFrom(c, service.ErrIdempotencyStoreUnavail)
		return
	}
	payload := struct {
		AccountID int64
		Request   zhipuResetActionRequest
	}{id, req}
	executeAdminIdempotentJSONWithTimeout(c, "admin.accounts.zhipu_reset_cards."+op, payload,
		service.DefaultWriteIdempotencyTTL(), 35*time.Second, func(ctx context.Context) (any, error) {
			return h.zhipuResetCards.Act(ctx, id, req.BindingID, op, req.ResetType, key)
		})
}

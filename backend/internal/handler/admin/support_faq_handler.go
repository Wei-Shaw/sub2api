// Package admin — support_faq_handler.go
//
// admin 端客服 FAQ 管理 HTTP handler。覆盖：
//
//   - GET    /api/v1/admin/support/faqs          列表（含 Indexed 标记）
//   - POST   /api/v1/admin/support/faqs          新建
//   - GET    /api/v1/admin/support/faqs/:id      详情
//   - PUT    /api/v1/admin/support/faqs/:id      更新（部分字段）
//   - DELETE /api/v1/admin/support/faqs/:id      删除
//   - POST   /api/v1/admin/support/faqs/reindex  批量重新嵌入
package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SupportFaqHandler 处理 admin 客服 FAQ 接口。
type SupportFaqHandler struct {
	service *service.SupportFaqService
}

// NewSupportFaqHandler 构造 handler。
func NewSupportFaqHandler(svc *service.SupportFaqService) *SupportFaqHandler {
	return &SupportFaqHandler{service: svc}
}

// List 处理 GET /api/v1/admin/support/faqs。
//
// 支持 query: ?only_enabled=true 仅返回启用项（默认 false，admin 视图全量）。
func (h *SupportFaqHandler) List(c *gin.Context) {
	onlyEnabled := strings.EqualFold(c.Query("only_enabled"), "true")
	items, err := h.service.List(c.Request.Context(), onlyEnabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.SupportFaqItem, 0, len(items))
	for i := range items {
		out = append(out, *dto.SupportFaqItemFromService(&items[i]))
	}
	response.Success(c, gin.H{"items": out, "total": len(out)})
}

// Get 处理 GET /api/v1/admin/support/faqs/:id。
func (h *SupportFaqHandler) Get(c *gin.Context) {
	id, err := parseAdminFaqID(c)
	if err != nil {
		response.BadRequest(c, "Invalid faq ID")
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportFaqItemFromService(item))
}

// Create 处理 POST /api/v1/admin/support/faqs。
func (h *SupportFaqHandler) Create(c *gin.Context) {
	var req dto.SupportFaqCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	res, err := h.service.Create(c.Request.Context(), req.ToService())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportFaqMutationFromService(res))
}

// Update 处理 PUT /api/v1/admin/support/faqs/:id。
func (h *SupportFaqHandler) Update(c *gin.Context) {
	id, err := parseAdminFaqID(c)
	if err != nil {
		response.BadRequest(c, "Invalid faq ID")
		return
	}
	var req dto.SupportFaqUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	res, err := h.service.Update(c.Request.Context(), id, req.ToService())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportFaqMutationFromService(res))
}

// Delete 处理 DELETE /api/v1/admin/support/faqs/:id。
func (h *SupportFaqHandler) Delete(c *gin.Context) {
	id, err := parseAdminFaqID(c)
	if err != nil {
		response.BadRequest(c, "Invalid faq ID")
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

// Reindex 处理 POST /api/v1/admin/support/faqs/reindex。
//
// query: ?mode=all 全量重算；默认 missing 仅补 embedding=NULL 的 row。
func (h *SupportFaqHandler) Reindex(c *gin.Context) {
	mode := strings.ToLower(strings.TrimSpace(c.Query("mode")))
	var (
		ok     int
		failed int
		err    error
	)
	if mode == "all" {
		ok, failed, err = h.service.ReindexAll(c.Request.Context())
	} else {
		ok, failed, err = h.service.ReindexMissing(c.Request.Context())
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportFaqReindexResponse{Succeeded: ok, Failed: failed})
}

// parseAdminFaqID 解析 :id 路径参数。
func parseAdminFaqID(c *gin.Context) (int64, error) {
	v, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || v <= 0 {
		return 0, err
	}
	return v, nil
}

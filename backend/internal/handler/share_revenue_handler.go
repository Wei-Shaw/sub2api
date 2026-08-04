package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ShareRevenueHandler 用户贡献收益查询。
type ShareRevenueHandler struct {
	svc *service.ShareRevenueService
}

// NewShareRevenueHandler 构造。
func NewShareRevenueHandler(svc *service.ShareRevenueService) *ShareRevenueHandler {
	return &ShareRevenueHandler{svc: svc}
}

// GetSummary GET /api/v1/user/share-revenue/summary
func (h *ShareRevenueHandler) GetSummary(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if h.svc == nil {
		response.Success(c, &service.ShareRevenueSummary{})
		return
	}
	sum, err := h.svc.GetMySummary(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, sum)
}

// ListLedgers GET /api/v1/user/share-revenue/ledgers?page=1&page_size=20
func (h *ShareRevenueHandler) ListLedgers(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if h.svc == nil {
		response.Success(c, gin.H{
			"items": []any{},
			"total": 0,
			"page":  page,
			"page_size": pageSize,
		})
		return
	}
	items, total, err := h.svc.ListMyLedgers(c.Request.Context(), subject.UserID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if items == nil {
		items = []service.ShareRevenueLedgerItem{}
	}
	pages := int64(0)
	if pageSize > 0 {
		pages = (total + int64(pageSize) - 1) / int64(pageSize)
	}
	response.Success(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"pages":     pages,
	})
}

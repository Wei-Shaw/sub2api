package admin

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AuditHandler struct {
	service *service.AuditService
}

func NewAuditHandler(svc *service.AuditService) *AuditHandler {
	return &AuditHandler{service: svc}
}

func (h *AuditHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := service.AuditLogFilter{
		Pagination: pagination.PaginationParams{
			Page:      page,
			PageSize:  pageSize,
			SortOrder: pagination.SortOrderDesc,
		},
		Search:   c.Query("search"),
		Platform: c.Query("platform"),
		Model:    c.Query("model"),
		Endpoint: c.Query("endpoint"),
	}
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "Invalid from")
			return
		}
		filter.From = &t
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "Invalid to")
			return
		}
		filter.To = &t
	}
	items, pageResult, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, pageResult.Total, pageResult.Page, pageResult.PageSize)
}

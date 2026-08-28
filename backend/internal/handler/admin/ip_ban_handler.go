package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type IPBanHandler struct {
	service *service.IPBanService
}

func NewIPBanHandler(ipBanService *service.IPBanService) *IPBanHandler {
	return &IPBanHandler{service: ipBanService}
}

type createIPBanRequest struct {
	IPAddress string `json:"ip_address" binding:"required"`
}

func (h *IPBanHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	bans, result, err := h.service.List(c.Request.Context(), pagination.PaginationParams{Page: page, PageSize: pageSize})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, bans, result.Total, page, pageSize)
}

func (h *IPBanHandler) Create(c *gin.Context) {
	var req createIPBanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	ban, err := h.service.Create(c.Request.Context(), req.IPAddress)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, ban)
}

func (h *IPBanHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid IP ban ID")
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

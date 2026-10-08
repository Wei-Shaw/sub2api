package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ProxyGroupHandler struct{ service *service.ProxyGroupService }

func NewProxyGroupHandler(svc *service.ProxyGroupService) *ProxyGroupHandler {
	return &ProxyGroupHandler{service: svc}
}

type proxyGroupRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
	ProxyIDs    []int64 `json:"proxy_ids"`
}

func (h *ProxyGroupHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	groups, total, err := h.service.List(c.Request.Context(), page, pageSize, c.Query("status"), c.Query("search"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, groups, total, page, pageSize)
}

func (h *ProxyGroupHandler) GetAll(c *gin.Context) {
	groups, err := h.service.ListAll(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, groups)
}

func (h *ProxyGroupHandler) GetByID(c *gin.Context) {
	id, err := parseProxyGroupID(c)
	if err != nil {
		response.BadRequest(c, "Invalid proxy group ID")
		return
	}
	group, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, group)
}

func (h *ProxyGroupHandler) Create(c *gin.Context) {
	var req proxyGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	group, err := h.service.Create(c.Request.Context(), service.ProxyGroupInput{Name: req.Name, Description: req.Description, Status: req.Status, ProxyIDs: req.ProxyIDs})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, group)
}

func (h *ProxyGroupHandler) Update(c *gin.Context) {
	id, err := parseProxyGroupID(c)
	if err != nil {
		response.BadRequest(c, "Invalid proxy group ID")
		return
	}
	var req proxyGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	group, err := h.service.Update(c.Request.Context(), id, service.ProxyGroupInput{Name: req.Name, Description: req.Description, Status: req.Status, ProxyIDs: req.ProxyIDs})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, group)
}

func (h *ProxyGroupHandler) Delete(c *gin.Context) {
	id, err := parseProxyGroupID(c)
	if err != nil {
		response.BadRequest(c, "Invalid proxy group ID")
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

func parseProxyGroupID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

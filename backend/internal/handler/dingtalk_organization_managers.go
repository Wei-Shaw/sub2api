package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func dingTalkManagerID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("manager"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid manager ID")
		return 0, false
	}
	return id, true
}
func (h *DingTalkOrganizationHandler) ManagerPage(c *gin.Context) {
	if !dingTalkRequireAdmin(c) {
		return
	}
	page, _ := response.ParsePagination(c)
	managers, total, err := h.organization.ManagerPage(c.Request.Context(), page)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, managers, total, page, service.DingTalkManagerPageSize)
}
func (h *DingTalkOrganizationHandler) CreateManager(c *gin.Context) {
	if !dingTalkRequireAdmin(c) {
		return
	}
	req := service.DingTalkManager{LimitCents: 50000, Enabled: true}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid manager configuration")
		return
	}
	if err := h.organization.CreateManager(c.Request.Context(), req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, gin.H{"saved": true})
}
func (h *DingTalkOrganizationHandler) PatchManager(c *gin.Context) {
	if !dingTalkRequireAdmin(c) {
		return
	}
	id, ok := dingTalkManagerID(c)
	if !ok {
		return
	}
	var req service.DingTalkManagerPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid manager changes")
		return
	}
	if err := h.organization.PatchManager(c.Request.Context(), id, req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"saved": true})
}
func (h *DingTalkOrganizationHandler) IncreaseManagerBudget(c *gin.Context) {
	if !dingTalkRequireAdmin(c) {
		return
	}
	id, ok := dingTalkManagerID(c)
	if !ok {
		return
	}
	var req struct {
		Amount    float64 `json:"amount"`
		RequestID string  `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid budget increase")
		return
	}
	cents, err := service.DingTalkQuotaCents(req.Amount)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result, err := h.organization.IncreaseManagerBudget(c.Request.Context(), mustDingTalkActorID(c), id, cents, req.RequestID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
func (h *DingTalkOrganizationHandler) ManagerGrants(c *gin.Context) {
	if !dingTalkRequireAdmin(c) {
		return
	}
	id, ok := dingTalkManagerID(c)
	if !ok {
		return
	}
	page, _ := response.ParsePagination(c)
	grants, total, err := h.organization.ManagerGrants(c.Request.Context(), id, page)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, grants, total, page, service.DingTalkManagerPageSize)
}

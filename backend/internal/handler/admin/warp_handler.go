package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// WarpHandler admin REST for Cloudflare WARP gateway sync.
type WarpHandler struct {
	svc *service.WarpSyncService
}

func NewWarpHandler(svc *service.WarpSyncService) *WarpHandler {
	return &WarpHandler{svc: svc}
}

// Status GET /api/v1/admin/warp/status
func (h *WarpHandler) Status(c *gin.Context) {
	enabled := h.svc != nil && h.svc.Enabled()
	response.Success(c, gin.H{"enabled": enabled})
}

// Snapshot GET /api/v1/admin/warp/snapshot
func (h *WarpHandler) Snapshot(c *gin.Context) {
	if h.svc == nil || !h.svc.Enabled() {
		response.BadRequest(c, "warp gateway is disabled")
		return
	}
	snap, err := h.svc.Snapshot(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, snap)
}

// ListInstances GET /api/v1/admin/warp/instances
func (h *WarpHandler) ListInstances(c *gin.Context) {
	if h.svc == nil || !h.svc.Enabled() {
		response.BadRequest(c, "warp gateway is disabled")
		return
	}
	list, err := h.svc.ListInstances(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"instances": list})
}

// Sync POST /api/v1/admin/warp/sync
// Body optional: { "group_name": "warp-pool" }
func (h *WarpHandler) Sync(c *gin.Context) {
	if h.svc == nil || !h.svc.Enabled() {
		response.BadRequest(c, "warp gateway is disabled")
		return
	}
	var req struct {
		GroupName string `json:"group_name"`
	}
	_ = c.ShouldBindJSON(&req)
	result, err := h.svc.SyncFromGateway(c.Request.Context(), strings.TrimSpace(req.GroupName))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// CreatePool POST /api/v1/admin/warp/pools
// { "name_prefix": "warp", "count": 3, "group_name": "warp-pool", "register": true }
func (h *WarpHandler) CreatePool(c *gin.Context) {
	if h.svc == nil || !h.svc.Enabled() {
		response.BadRequest(c, "warp gateway is disabled")
		return
	}
	var req struct {
		NamePrefix string `json:"name_prefix"`
		Count      int    `json:"count"`
		GroupName  string `json:"group_name"`
		Register   bool   `json:"register"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if req.Count <= 0 {
		response.BadRequest(c, "count must be > 0")
		return
	}
	if req.NamePrefix == "" {
		req.NamePrefix = "warp"
	}
	result, err := h.svc.CreatePoolAndSyncEx(c.Request.Context(), req.NamePrefix, req.Count, strings.TrimSpace(req.GroupName), req.Register)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// RegisterPool POST /api/v1/admin/warp/register-pool
// One-click: register free WARP accounts + create pool + sync.
// { "name_prefix": "warp", "count": 3, "group_name": "warp-pool" }
func (h *WarpHandler) RegisterPool(c *gin.Context) {
	if h.svc == nil || !h.svc.Enabled() {
		response.BadRequest(c, "warp gateway is disabled")
		return
	}
	var req struct {
		NamePrefix string `json:"name_prefix"`
		Count      int    `json:"count"`
		GroupName  string `json:"group_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if req.Count <= 0 {
		response.BadRequest(c, "count must be > 0")
		return
	}
	if req.NamePrefix == "" {
		req.NamePrefix = "warp"
	}
	result, err := h.svc.RegisterProfilesAndSync(c.Request.Context(), req.NamePrefix, req.Count, strings.TrimSpace(req.GroupName))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// HealthSync POST /api/v1/admin/warp/health-sync
func (h *WarpHandler) HealthSync(c *gin.Context) {
	if h.svc == nil || !h.svc.Enabled() {
		response.BadRequest(c, "warp gateway is disabled")
		return
	}
	var req struct {
		GroupName string `json:"group_name"`
	}
	_ = c.ShouldBindJSON(&req)
	result, err := h.svc.HealthAllAndSync(c.Request.Context(), strings.TrimSpace(req.GroupName))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// Rotate POST /api/v1/admin/warp/instances/:id/rotate
func (h *WarpHandler) Rotate(c *gin.Context) {
	if h.svc == nil || !h.svc.Enabled() {
		response.BadRequest(c, "warp gateway is disabled")
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "instance id required")
		return
	}
	var req struct {
		GroupName string `json:"group_name"`
	}
	_ = c.ShouldBindJSON(&req)
	result, err := h.svc.RotateAndSync(c.Request.Context(), id, strings.TrimSpace(req.GroupName))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// DeleteInstance DELETE /api/v1/admin/warp/instances/:id
// Query/body: deregister_cloudflare (default true), group_name
func (h *WarpHandler) DeleteInstance(c *gin.Context) {
	if h.svc == nil || !h.svc.Enabled() {
		response.BadRequest(c, "warp gateway is disabled")
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "instance id required")
		return
	}
	deregister := true
	if v := c.Query("deregister_cloudflare"); v != "" {
		// keep true unless explicitly disabled
		deregister = v != "0" && !strings.EqualFold(v, "false") && !strings.EqualFold(v, "no")
	}
	groupName := strings.TrimSpace(c.Query("group_name"))
	var body struct {
		DeregisterCloudflare *bool  `json:"deregister_cloudflare"`
		GroupName            string `json:"group_name"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.DeregisterCloudflare != nil {
		deregister = *body.DeregisterCloudflare
	}
	if strings.TrimSpace(body.GroupName) != "" {
		groupName = strings.TrimSpace(body.GroupName)
	}
	result, err := h.svc.DeleteInstanceAndSync(c.Request.Context(), id, groupName, deregister)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// BindAccounts POST /api/v1/admin/warp/bind-accounts
// { "account_ids": [1,2], "group_name": "warp-pool", "bind_all_active": false }
func (h *WarpHandler) BindAccounts(c *gin.Context) {
	if h.svc == nil || !h.svc.Enabled() {
		response.BadRequest(c, "warp gateway is disabled")
		return
	}
	var req struct {
		AccountIDs    []int64 `json:"account_ids"`
		GroupName     string  `json:"group_name"`
		BindAllActive bool    `json:"bind_all_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if len(req.AccountIDs) == 0 && !req.BindAllActive {
		response.BadRequest(c, "account_ids or bind_all_active required")
		return
	}
	result, err := h.svc.BindAccountsToGroup(c.Request.Context(), req.AccountIDs, strings.TrimSpace(req.GroupName), req.BindAllActive)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// PreviewPlan GET /api/v1/admin/warp/attach-plan?group_name=
// Dry-run BuildAttachPlan without writing DB.
func (h *WarpHandler) PreviewPlan(c *gin.Context) {
	if h.svc == nil || !h.svc.Enabled() {
		response.BadRequest(c, "warp gateway is disabled")
		return
	}
	snap, err := h.svc.Snapshot(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	group := strings.TrimSpace(c.Query("group_name"))
	plan := service.BuildAttachPlan(snap, group)
	response.Success(c, gin.H{"snapshot": snap, "plan": plan})
}

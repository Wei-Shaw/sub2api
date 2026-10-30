package admin

import (
	"context"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountSchedulingPoolAdminService interface {
	List(context.Context) ([]service.AccountSchedulingPool, error)
	Create(context.Context, string, bool) (*service.AccountSchedulingPool, error)
	Update(context.Context, int64, string, bool) (*service.AccountSchedulingPool, error)
	Delete(context.Context, int64) error
	Members(context.Context, *int64) ([]service.AccountSchedulingMember, error)
	SetMembers(context.Context, *int64, []int64) error
	GetPolicy(context.Context) (service.AccountSchedulingPolicy, error)
	UpdatePolicy(context.Context, service.AccountSchedulingPolicy) error
}

type AccountSchedulingPoolHandler struct {
	service accountSchedulingPoolAdminService
}

func NewAccountSchedulingPoolHandler(s *service.AccountSchedulingPoolService) *AccountSchedulingPoolHandler {
	return &AccountSchedulingPoolHandler{service: s}
}

type accountSchedulingPoolRequest struct {
	Name    string `json:"name" binding:"required,max=100"`
	Enabled bool   `json:"enabled"`
}

func applyAccountSchedulingPoolFilter(c *gin.Context) bool {
	if raw := c.Query("pool_id"); raw != "" {
		poolID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || poolID < 0 {
			response.BadRequest(c, "Invalid account pool filter")
			return false
		}
		c.Request = c.Request.WithContext(service.WithAccountSchedulingPoolFilter(c.Request.Context(), &poolID))
	}
	return true
}

func (h *AccountSchedulingPoolHandler) List(c *gin.Context) {
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if items == nil {
		items = []service.AccountSchedulingPool{}
	}
	response.Success(c, items)
}

func (h *AccountSchedulingPoolHandler) Create(c *gin.Context) {
	var req accountSchedulingPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid account pool: "+err.Error())
		return
	}
	executeAdminIdempotentJSON(c, "admin.account-scheduling-pools.create", req, 24*time.Hour, func(ctx context.Context) (any, error) {
		return h.service.Create(ctx, req.Name, req.Enabled)
	})
}

func accountSchedulingPoolID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account pool ID")
		return 0, false
	}
	return id, true
}

func (h *AccountSchedulingPoolHandler) Update(c *gin.Context) {
	id, ok := accountSchedulingPoolID(c)
	if !ok {
		return
	}
	var req accountSchedulingPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid account pool: "+err.Error())
		return
	}
	item, err := h.service.Update(c.Request.Context(), id, req.Name, req.Enabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *AccountSchedulingPoolHandler) Delete(c *gin.Context) {
	id, ok := accountSchedulingPoolID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *AccountSchedulingPoolHandler) Members(c *gin.Context) {
	var poolID *int64
	if raw := c.Query("pool_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid account pool ID")
			return
		}
		poolID = &id
	}
	items, err := h.service.Members(c.Request.Context(), poolID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if items == nil {
		items = []service.AccountSchedulingMember{}
	}
	response.Success(c, items)
}

func (h *AccountSchedulingPoolHandler) SetMembers(c *gin.Context) {
	var req struct {
		PoolID     *int64  `json:"pool_id" binding:"omitempty,gt=0"`
		AccountIDs []int64 `json:"account_ids" binding:"required,min=1,max=1000,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid pool members: "+err.Error())
		return
	}
	if err := h.service.SetMembers(c.Request.Context(), req.PoolID, req.AccountIDs); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"updated": true})
}

func (h *AccountSchedulingPoolHandler) GetPolicy(c *gin.Context) {
	policy, err := h.service.GetPolicy(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, policy)
}

func (h *AccountSchedulingPoolHandler) UpdatePolicy(c *gin.Context) {
	var policy service.AccountSchedulingPolicy
	if err := c.ShouldBindJSON(&policy); err != nil {
		response.BadRequest(c, "Invalid scheduling policy: "+err.Error())
		return
	}
	if err := h.service.UpdatePolicy(c.Request.Context(), policy); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, policy)
}

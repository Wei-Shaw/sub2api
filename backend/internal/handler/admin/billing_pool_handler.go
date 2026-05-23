package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// BillingPoolHandler handles admin billing pool management.
type BillingPoolHandler struct {
	billingPoolService service.BillingPoolManager
}

// NewBillingPoolHandler creates a new admin billing pool handler.
func NewBillingPoolHandler(billingPoolService service.BillingPoolManager) *BillingPoolHandler {
	return &BillingPoolHandler{billingPoolService: billingPoolService}
}

type billingPoolMemberRequest struct {
	GroupID       int64 `json:"group_id" binding:"required"`
	ChainOrder    int   `json:"chain_order"`
	CanBePrimary  bool  `json:"can_be_primary"`
	CanBeFallback bool  `json:"can_be_fallback"`
}

type CreateBillingPoolRequest struct {
	Name                       string                     `json:"name" binding:"required"`
	Code                       string                     `json:"code" binding:"required"`
	Description                string                     `json:"description"`
	Status                     string                     `json:"status" binding:"omitempty,oneof=active inactive"`
	PlatformScope              string                     `json:"platform_scope" binding:"omitempty,oneof=same_platform mixed_platform"`
	AllowUserReorder           bool                       `json:"allow_user_reorder"`
	RequirePrimarySubscription *bool                      `json:"require_primary_subscription"`
	AllowBalanceFallback       *bool                      `json:"allow_balance_fallback"`
	Groups                     []billingPoolMemberRequest `json:"groups"`
}

type UpdateBillingPoolRequest struct {
	Name                       *string `json:"name"`
	Code                       *string `json:"code"`
	Description                *string `json:"description"`
	Status                     *string `json:"status" binding:"omitempty,oneof=active inactive"`
	PlatformScope              *string `json:"platform_scope" binding:"omitempty,oneof=same_platform mixed_platform"`
	AllowUserReorder           *bool   `json:"allow_user_reorder"`
	RequirePrimarySubscription *bool   `json:"require_primary_subscription"`
	AllowBalanceFallback       *bool   `json:"allow_balance_fallback"`
}

type ReplaceBillingPoolMembersRequest struct {
	Groups []billingPoolMemberRequest `json:"groups"`
}

// List handles listing billing pools.
// GET /api/v1/admin/billing-pools
func (h *BillingPoolHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filters := service.BillingPoolListFilters{
		Status:        strings.TrimSpace(c.Query("status")),
		Search:        strings.TrimSpace(c.Query("search")),
		PlatformScope: strings.TrimSpace(c.Query("platform_scope")),
	}
	if len(filters.Search) > 100 {
		filters.Search = filters.Search[:100]
	}

	pools, paginationResult, err := h.billingPoolService.List(
		c.Request.Context(),
		page,
		pageSize,
		filters,
		c.DefaultQuery("sort_by", "updated_at"),
		c.DefaultQuery("sort_order", "desc"),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.BillingPool, 0, len(pools))
	for i := range pools {
		item := dto.BillingPoolFromService(&pools[i])
		if item != nil {
			out = append(out, *item)
		}
	}
	response.Paginated(c, out, paginationResult.Total, page, pageSize)
}

// Lookup handles lightweight billing pool lookup.
// GET /api/v1/admin/billing-pools/lookup
func (h *BillingPoolHandler) Lookup(c *gin.Context) {
	items, err := h.billingPoolService.ListLookup(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.BillingPoolLookup, 0, len(items))
	for i := range items {
		item := dto.BillingPoolLookupFromService(&items[i])
		if item != nil {
			out = append(out, *item)
		}
	}
	response.Success(c, out)
}

// GetByID handles billing pool detail.
// GET /api/v1/admin/billing-pools/:id
func (h *BillingPoolHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid billing pool ID")
		return
	}

	pool, err := h.billingPoolService.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.BillingPoolFromService(pool))
}

// Create handles billing pool creation.
// POST /api/v1/admin/billing-pools
func (h *BillingPoolHandler) Create(c *gin.Context) {
	var req CreateBillingPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	input := &service.CreateBillingPoolInput{
		Name:             req.Name,
		Code:             req.Code,
		Description:      req.Description,
		Status:           req.Status,
		PlatformScope:    req.PlatformScope,
		AllowUserReorder: req.AllowUserReorder,
		Members:          toBillingPoolMemberInputs(req.Groups),
	}
	if req.RequirePrimarySubscription != nil {
		input.RequirePrimarySubscription = *req.RequirePrimarySubscription
	} else {
		input.RequirePrimarySubscription = true
	}
	if req.AllowBalanceFallback != nil {
		input.AllowBalanceFallback = *req.AllowBalanceFallback
	} else {
		input.AllowBalanceFallback = true
	}

	pool, err := h.billingPoolService.Create(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.BillingPoolFromService(pool))
}

// Update handles billing pool update.
// PUT /api/v1/admin/billing-pools/:id
func (h *BillingPoolHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid billing pool ID")
		return
	}

	var req UpdateBillingPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	pool, err := h.billingPoolService.Update(c.Request.Context(), id, &service.UpdateBillingPoolInput{
		Name:                       req.Name,
		Code:                       req.Code,
		Description:                req.Description,
		Status:                     req.Status,
		PlatformScope:              req.PlatformScope,
		AllowUserReorder:           req.AllowUserReorder,
		RequirePrimarySubscription: req.RequirePrimarySubscription,
		AllowBalanceFallback:       req.AllowBalanceFallback,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.BillingPoolFromService(pool))
}

// Delete handles billing pool deletion.
// DELETE /api/v1/admin/billing-pools/:id
func (h *BillingPoolHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid billing pool ID")
		return
	}

	if err := h.billingPoolService.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Billing pool deleted successfully"})
}

// ReplaceMembers handles full billing pool member replacement.
// PUT /api/v1/admin/billing-pools/:id/members
func (h *BillingPoolHandler) ReplaceMembers(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid billing pool ID")
		return
	}

	var req ReplaceBillingPoolMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	pool, err := h.billingPoolService.ReplaceMembers(c.Request.Context(), id, toBillingPoolMemberInputs(req.Groups))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.BillingPoolFromService(pool))
}

func toBillingPoolMemberInputs(groups []billingPoolMemberRequest) []service.BillingPoolMemberInput {
	if groups == nil {
		return nil
	}
	out := make([]service.BillingPoolMemberInput, 0, len(groups))
	for i := range groups {
		out = append(out, service.BillingPoolMemberInput{
			GroupID:       groups[i].GroupID,
			ChainOrder:    groups[i].ChainOrder,
			CanBePrimary:  groups[i].CanBePrimary,
			CanBeFallback: groups[i].CanBeFallback,
		})
	}
	return out
}

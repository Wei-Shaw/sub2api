package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UserPoolHandler handles admin user pool management.
type UserPoolHandler struct {
	poolService *service.UserPoolService
}

// NewUserPoolHandler constructs a UserPoolHandler.
func NewUserPoolHandler(poolService *service.UserPoolService) *UserPoolHandler {
	return &UserPoolHandler{poolService: poolService}
}

// parsePathInt64 reads a path parameter and validates it as a positive int64.
// On failure it writes a 400 response and returns (0, false).
func parsePathInt64(c *gin.Context, name string) (int64, bool) {
	raw := c.Param(name)
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		response.BadRequest(c, "invalid "+name)
		return 0, false
	}
	return v, true
}

// ── request/response types ────────────────────────────────────────────────────

type createPoolRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Status      string `json:"status" binding:"omitempty,oneof=active disabled"`
}

type updatePoolRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status" binding:"omitempty,oneof=active disabled"`
}

type addMembersRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required"`
}

type addMembersByFilterRequest struct {
	Search     string           `json:"search"`
	Status     string           `json:"status" binding:"omitempty,oneof=active disabled"`
	Role       string           `json:"role" binding:"omitempty,oneof=admin user"`
	GroupName  string           `json:"group_name"`
	Attributes map[int64]string `json:"attributes"`
}

type removeMembersRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required"`
}

type poolGroupGrantInput struct {
	GroupID        int64    `json:"group_id"`
	RateMultiplier *float64 `json:"rate_multiplier"`
	RPMOverride    *int     `json:"rpm_override"`
}

type replaceGroupGrantsRequest struct {
	Grants []poolGroupGrantInput `json:"grants" binding:"required"`
}

// ── pool DTO ──────────────────────────────────────────────────────────────────

type poolResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toPoolResponse(p service.Pool) poolResponse {
	return poolResponse{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Status:      p.Status,
		CreatedAt:   p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type memberResponse struct {
	PoolID    int64  `json:"pool_id"`
	UserID    int64  `json:"user_id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

func toMemberResponse(m service.PoolMember) memberResponse {
	return memberResponse{
		PoolID:    m.PoolID,
		UserID:    m.UserID,
		Email:     m.Email,
		Username:  m.Username,
		CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type grantResponse struct {
	PoolID         int64    `json:"pool_id"`
	GroupID        int64    `json:"group_id"`
	RateMultiplier *float64 `json:"rate_multiplier"`
	RPMOverride    *int     `json:"rpm_override"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

func toGrantResponse(g service.PoolGroupGrant) grantResponse {
	return grantResponse{
		PoolID:         g.PoolID,
		GroupID:        g.GroupID,
		RateMultiplier: g.RateMultiplier,
		RPMOverride:    g.RPMOverride,
		CreatedAt:      g.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      g.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ── handlers ──────────────────────────────────────────────────────────────────

// Create creates a new user pool. POST /admin/user-pools
func (h *UserPoolHandler) Create(c *gin.Context) {
	var req createPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	p, err := h.poolService.Create(c.Request.Context(), req.Name, req.Description, req.Status)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Created(c, toPoolResponse(p))
}

// List returns a paginated list of user pools. GET /admin/user-pools
func (h *UserPoolHandler) List(c *gin.Context) {
	page := parseIntQuery(c, "page", 1)
	limit := parseIntQuery(c, "limit", 20)
	status := c.Query("status")

	pools, total, err := h.poolService.List(c.Request.Context(), service.ListPoolsOptions{
		Page:   page,
		Limit:  limit,
		Status: status,
	})
	if response.ErrorFrom(c, err) {
		return
	}

	items := make([]poolResponse, len(pools))
	for i, p := range pools {
		items[i] = toPoolResponse(p)
	}
	response.Success(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": limit,
	})
}

// GetByID returns a pool by ID. GET /admin/user-pools/:id
func (h *UserPoolHandler) GetByID(c *gin.Context) {
	id, ok := parsePathInt64(c, "id")
	if !ok {
		return
	}
	p, err := h.poolService.GetByID(c.Request.Context(), id)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, toPoolResponse(p))
}

// Update updates a pool. PUT /admin/user-pools/:id
func (h *UserPoolHandler) Update(c *gin.Context) {
	id, ok := parsePathInt64(c, "id")
	if !ok {
		return
	}
	var req updatePoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	p, err := h.poolService.Update(c.Request.Context(), id, req.Name, req.Description, req.Status)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, toPoolResponse(p))
}

// Delete soft-deletes a pool. DELETE /admin/user-pools/:id
func (h *UserPoolHandler) Delete(c *gin.Context) {
	id, ok := parsePathInt64(c, "id")
	if !ok {
		return
	}
	if err := h.poolService.Delete(c.Request.Context(), id); response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, nil)
}

// AddMembers adds users to a pool. POST /admin/user-pools/:id/members
func (h *UserPoolHandler) AddMembers(c *gin.Context) {
	id, ok := parsePathInt64(c, "id")
	if !ok {
		return
	}
	var req addMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	added, skipped, err := h.poolService.AddMembers(c.Request.Context(), id, req.UserIDs)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, gin.H{
		"added":   added,
		"skipped": skipped,
	})
}

// AddMembersByFilter adds users matching a filter to a pool. POST /admin/user-pools/:id/members/by-filter
func (h *UserPoolHandler) AddMembersByFilter(c *gin.Context) {
	id, ok := parsePathInt64(c, "id")
	if !ok {
		return
	}
	var req addMembersByFilterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Search == "" && req.Status == "" && req.Role == "" && req.GroupName == "" && len(req.Attributes) == 0 {
		response.BadRequest(c, "at least one filter required, refusing to bulk-add all users")
		return
	}
	filters := service.UserListFilters{
		Search:     req.Search,
		Status:     req.Status,
		Role:       req.Role,
		GroupName:  req.GroupName,
		Attributes: req.Attributes,
	}
	added, skipped, matched, err := h.poolService.AddMembersByFilter(c.Request.Context(), id, filters)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, gin.H{
		"added":   added,
		"skipped": skipped,
		"matched": matched,
	})
}

// RemoveMembers removes users from a pool. POST /admin/user-pools/:id/members/remove
func (h *UserPoolHandler) RemoveMembers(c *gin.Context) {
	id, ok := parsePathInt64(c, "id")
	if !ok {
		return
	}
	var req removeMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	removed, err := h.poolService.RemoveMembers(c.Request.Context(), id, req.UserIDs)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, gin.H{"removed": removed})
}

// ListMembers lists members of a pool. GET /admin/user-pools/:id/members
func (h *UserPoolHandler) ListMembers(c *gin.Context) {
	id, ok := parsePathInt64(c, "id")
	if !ok {
		return
	}
	page := parseIntQuery(c, "page", 1)
	limit := parseIntQuery(c, "limit", 20)

	members, total, err := h.poolService.ListMembers(c.Request.Context(), id, service.ListMembersOptions{
		Page:  page,
		Limit: limit,
	})
	if response.ErrorFrom(c, err) {
		return
	}

	items := make([]memberResponse, len(members))
	for i, m := range members {
		items[i] = toMemberResponse(m)
	}
	response.Success(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": limit,
	})
}

// ReplaceGroupGrants atomically replaces all group grants. PUT /admin/user-pools/:id/allowed-groups
func (h *UserPoolHandler) ReplaceGroupGrants(c *gin.Context) {
	id, ok := parsePathInt64(c, "id")
	if !ok {
		return
	}
	var req replaceGroupGrantsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	grants := make([]service.PoolGroupGrant, len(req.Grants))
	for i, g := range req.Grants {
		grants[i] = service.PoolGroupGrant{
			PoolID:         id,
			GroupID:        g.GroupID,
			RateMultiplier: g.RateMultiplier,
			RPMOverride:    g.RPMOverride,
		}
	}
	if err := h.poolService.ReplaceGroupGrants(c.Request.Context(), id, grants); response.ErrorFrom(c, err) {
		return
	}

	// Return updated grants list.
	updatedGrants, err := h.poolService.ListGroupGrants(c.Request.Context(), id)
	if response.ErrorFrom(c, err) {
		return
	}
	items := make([]grantResponse, len(updatedGrants))
	for i, g := range updatedGrants {
		items[i] = toGrantResponse(g)
	}
	response.Success(c, gin.H{"grants": items})
}

// DeleteGroupGrant removes a specific grant. DELETE /admin/user-pools/:id/allowed-groups/:group_id
func (h *UserPoolHandler) DeleteGroupGrant(c *gin.Context) {
	poolID, ok := parsePathInt64(c, "id")
	if !ok {
		return
	}
	groupID, ok := parsePathInt64(c, "group_id")
	if !ok {
		return
	}
	if err := h.poolService.DeleteGroupGrant(c.Request.Context(), poolID, groupID); response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, nil)
}

// GetUserPools lists pools a user belongs to. GET /admin/users/:id/pools
func (h *UserPoolHandler) GetUserPools(c *gin.Context) {
	userID, ok := parsePathInt64(c, "id")
	if !ok {
		return
	}
	pools, err := h.poolService.GetUserPools(c.Request.Context(), userID)
	if response.ErrorFrom(c, err) {
		return
	}
	items := make([]poolResponse, len(pools))
	for i, p := range pools {
		items[i] = toPoolResponse(p)
	}
	response.Success(c, gin.H{"pools": items})
}

// ListGroupGrants lists all group grants for a pool. GET /admin/user-pools/:id/allowed-groups
func (h *UserPoolHandler) ListGroupGrants(c *gin.Context) {
	id, ok := parsePathInt64(c, "id")
	if !ok {
		return
	}
	grants, err := h.poolService.ListGroupGrants(c.Request.Context(), id)
	if response.ErrorFrom(c, err) {
		return
	}
	items := make([]grantResponse, len(grants))
	for i, g := range grants {
		items[i] = toGrantResponse(g)
	}
	response.Success(c, gin.H{"grants": items})
}

// ── internal helpers ──────────────────────────────────────────────────────────

// parseIntQuery reads a query parameter as int, returning defaultVal on error or absence.
func parseIntQuery(c *gin.Context, name string, defaultVal int) int {
	raw := c.Query(name)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return defaultVal
	}
	return v
}

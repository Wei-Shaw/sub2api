package admin

import (
	"context"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// toResponsePagination converts pagination.PaginationResult to response.PaginationResult
func toResponsePagination(p *pagination.PaginationResult) *response.PaginationResult {
	if p == nil {
		return nil
	}
	return &response.PaginationResult{
		Total:    p.Total,
		Page:     p.Page,
		PageSize: p.PageSize,
		Pages:    p.Pages,
	}
}

// SubscriptionHandler handles admin subscription management
type SubscriptionHandler struct {
	subscriptionService *service.SubscriptionService
}

// NewSubscriptionHandler creates a new admin subscription handler
func NewSubscriptionHandler(subscriptionService *service.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{
		subscriptionService: subscriptionService,
	}
}

// AssignSubscriptionRequest represents assign subscription request
type AssignSubscriptionRequest struct {
	UserID       int64  `json:"user_id" binding:"required"`
	GroupID      int64  `json:"group_id" binding:"required"`
	ValidityDays int    `json:"validity_days" binding:"omitempty,max=36500"` // max 100 years
	Notes        string `json:"notes"`
}

// BulkAssignSubscriptionRequest represents bulk assign subscription request
type BulkAssignSubscriptionRequest struct {
	UserIDs      []int64 `json:"user_ids" binding:"required,min=1"`
	GroupID      int64   `json:"group_id" binding:"required"`
	ValidityDays int     `json:"validity_days" binding:"omitempty,max=36500"` // max 100 years
	Notes        string  `json:"notes"`
}

// AdjustSubscriptionRequest represents adjust subscription request (extend or shorten)
type AdjustSubscriptionRequest struct {
	Days int `json:"days" binding:"required,min=-36500,max=36500"` // negative to shorten, positive to extend
}

type BenefitPackageRequest struct {
	Name        string `json:"name" binding:"required,max=100"`
	Description string `json:"description"`
	GroupID     int64  `json:"group_id" binding:"required"`
	LeaseDays   int    `json:"lease_days" binding:"required,min=1,max=36500"`
}

type BenefitPlanRequest struct {
	Name        string  `json:"name" binding:"required,max=100"`
	Description string  `json:"description"`
	PackageIDs  []int64 `json:"package_ids" binding:"required,min=1"`
}

type AssignUserBenefitPlanRequest struct {
	PlanID *int64 `json:"plan_id"`
}

type BenefitPlanBulkUsersRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required,min=1"`
}

type BenefitPackageResponse struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	GroupID     int64     `json:"group_id"`
	GroupName   string    `json:"group_name"`
	LeaseDays   int       `json:"lease_days"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type BenefitPlanPackageResponse struct {
	PackageID int64  `json:"package_id"`
	SortOrder int    `json:"sort_order"`
	Name      string `json:"name"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	LeaseDays int    `json:"lease_days"`
}

type BenefitPlanResponse struct {
	ID                int64                        `json:"id"`
	Name              string                       `json:"name"`
	Description       string                       `json:"description"`
	Packages          []BenefitPlanPackageResponse `json:"packages"`
	AssignedUserCount int64                        `json:"assigned_user_count"`
	CreatedAt         time.Time                    `json:"created_at"`
	UpdatedAt         time.Time                    `json:"updated_at"`
}

type UserBenefitPlanAssignmentResponse struct {
	UserID     int64     `json:"user_id"`
	PlanID     int64     `json:"plan_id"`
	PlanName   string    `json:"plan_name"`
	Version    int64     `json:"version"`
	AssignedBy *int64    `json:"assigned_by"`
	AssignedAt time.Time `json:"assigned_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type BenefitPlanMemberResponse struct {
	UserID     int64     `json:"user_id"`
	Email      string    `json:"email"`
	Role       string    `json:"role"`
	Status     string    `json:"status"`
	Version    int64     `json:"version"`
	AssignedAt time.Time `json:"assigned_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type BenefitPlanUserBulkResultResponse struct {
	SuccessCount   int               `json:"success_count"`
	FailedCount    int               `json:"failed_count"`
	AssignedCount  int               `json:"assigned_count"`
	RemovedCount   int               `json:"removed_count"`
	UnchangedCount int               `json:"unchanged_count"`
	SkippedCount   int               `json:"skipped_count"`
	Errors         []string          `json:"errors"`
	Statuses       map[string]string `json:"statuses,omitempty"`
}

// List handles listing all subscriptions with pagination and filters
// GET /api/v1/admin/subscriptions
func (h *SubscriptionHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)

	// Parse optional filters
	var userID, groupID *int64
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if id, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = &id
		}
	}
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if id, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil {
			groupID = &id
		}
	}
	status := c.Query("status")
	platform := c.Query("platform")

	// Parse sorting parameters
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")

	subscriptions, pagination, err := h.subscriptionService.List(c.Request.Context(), page, pageSize, userID, groupID, status, platform, sortBy, sortOrder)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.AdminUserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromServiceAdmin(&subscriptions[i]))
	}
	response.PaginatedWithResult(c, out, toResponsePagination(pagination))
}

// GetByID handles getting a subscription by ID
// GET /api/v1/admin/subscriptions/:id
func (h *SubscriptionHandler) GetByID(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	subscription, err := h.subscriptionService.GetByID(c.Request.Context(), subscriptionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.UserSubscriptionFromServiceAdmin(subscription))
}

// GetProgress handles getting subscription usage progress
// GET /api/v1/admin/subscriptions/:id/progress
func (h *SubscriptionHandler) GetProgress(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	progress, err := h.subscriptionService.GetSubscriptionProgress(c.Request.Context(), subscriptionID)
	if err != nil {
		response.NotFound(c, "Subscription not found")
		return
	}

	response.Success(c, progress)
}

// Assign handles assigning a subscription to a user
// POST /api/v1/admin/subscriptions/assign
func (h *SubscriptionHandler) Assign(c *gin.Context) {
	var req AssignSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Get admin user ID from context
	adminID := getAdminIDFromContext(c)

	subscription, err := h.subscriptionService.AssignSubscription(c.Request.Context(), &service.AssignSubscriptionInput{
		UserID:       req.UserID,
		GroupID:      req.GroupID,
		ValidityDays: req.ValidityDays,
		AssignedBy:   adminID,
		Notes:        req.Notes,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.UserSubscriptionFromServiceAdmin(subscription))
}

// BulkAssign handles bulk assigning subscriptions to multiple users
// POST /api/v1/admin/subscriptions/bulk-assign
func (h *SubscriptionHandler) BulkAssign(c *gin.Context) {
	var req BulkAssignSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Get admin user ID from context
	adminID := getAdminIDFromContext(c)

	result, err := h.subscriptionService.BulkAssignSubscription(c.Request.Context(), &service.BulkAssignSubscriptionInput{
		UserIDs:      req.UserIDs,
		GroupID:      req.GroupID,
		ValidityDays: req.ValidityDays,
		AssignedBy:   adminID,
		Notes:        req.Notes,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.BulkAssignResultFromService(result))
}

// Extend handles adjusting a subscription (extend or shorten)
// POST /api/v1/admin/subscriptions/:id/extend
func (h *SubscriptionHandler) Extend(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	var req AdjustSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	idempotencyPayload := struct {
		SubscriptionID int64                     `json:"subscription_id"`
		Body           AdjustSubscriptionRequest `json:"body"`
	}{
		SubscriptionID: subscriptionID,
		Body:           req,
	}
	executeAdminIdempotentJSON(c, "admin.subscriptions.extend", idempotencyPayload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		subscription, execErr := h.subscriptionService.ExtendSubscription(ctx, subscriptionID, req.Days)
		if execErr != nil {
			return nil, execErr
		}
		return dto.UserSubscriptionFromServiceAdmin(subscription), nil
	})
}

// ResetSubscriptionQuotaRequest represents the reset quota request
type ResetSubscriptionQuotaRequest struct {
	Daily   bool `json:"daily"`
	Weekly  bool `json:"weekly"`
	Monthly bool `json:"monthly"`
}

// ResetQuota resets daily, weekly, and/or monthly usage for a subscription.
// POST /api/v1/admin/subscriptions/:id/reset-quota
func (h *SubscriptionHandler) ResetQuota(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}
	var req ResetSubscriptionQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !req.Daily && !req.Weekly && !req.Monthly {
		response.BadRequest(c, "At least one of 'daily', 'weekly', or 'monthly' must be true")
		return
	}
	sub, err := h.subscriptionService.AdminResetQuota(c.Request.Context(), subscriptionID, req.Daily, req.Weekly, req.Monthly)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.UserSubscriptionFromServiceAdmin(sub))
}

// Revoke handles revoking a subscription.
// POST /api/v1/admin/subscriptions/:id/revoke
// DELETE /api/v1/admin/subscriptions/:id is kept for backward compatibility.
func (h *SubscriptionHandler) Revoke(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	err = h.subscriptionService.RevokeSubscription(c.Request.Context(), subscriptionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Subscription revoked successfully"})
}

// Restore handles restoring a revoked subscription.
// POST /api/v1/admin/subscriptions/:id/restore
func (h *SubscriptionHandler) Restore(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	subscription, err := h.subscriptionService.RestoreSubscription(c.Request.Context(), subscriptionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.UserSubscriptionFromServiceAdmin(subscription))
}

// ListByGroup handles listing subscriptions for a specific group
// GET /api/v1/admin/groups/:id/subscriptions
func (h *SubscriptionHandler) ListByGroup(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	page, pageSize := response.ParsePagination(c)

	subscriptions, pagination, err := h.subscriptionService.ListGroupSubscriptions(c.Request.Context(), groupID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.AdminUserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromServiceAdmin(&subscriptions[i]))
	}
	response.PaginatedWithResult(c, out, toResponsePagination(pagination))
}

// ListByUser handles listing subscriptions for a specific user
// GET /api/v1/admin/users/:id/subscriptions
func (h *SubscriptionHandler) ListByUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	subscriptions, err := h.subscriptionService.ListUserSubscriptions(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.AdminUserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromServiceAdmin(&subscriptions[i]))
	}
	response.Success(c, out)
}

// ListBenefitPackages handles listing benefit packages
// GET /api/v1/admin/subscriptions/benefit-packages
func (h *SubscriptionHandler) ListBenefitPackages(c *gin.Context) {
	packages, err := h.subscriptionService.ListBenefitPackages(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]BenefitPackageResponse, 0, len(packages))
	for _, item := range packages {
		out = append(out, benefitPackageResponseFromService(item))
	}
	response.Success(c, out)
}

// CreateBenefitPackage handles creating a benefit package
// POST /api/v1/admin/subscriptions/benefit-packages
func (h *SubscriptionHandler) CreateBenefitPackage(c *gin.Context) {
	var req BenefitPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.subscriptionService.CreateBenefitPackage(c.Request.Context(), &service.CreateBenefitPackageInput{
		Name:        req.Name,
		Description: req.Description,
		GroupID:     req.GroupID,
		LeaseDays:   req.LeaseDays,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, benefitPackageResponseFromService(*item))
}

// UpdateBenefitPackage handles updating a benefit package
// PUT /api/v1/admin/subscriptions/benefit-packages/:id
func (h *SubscriptionHandler) UpdateBenefitPackage(c *gin.Context) {
	packageID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid package ID")
		return
	}
	var req BenefitPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.subscriptionService.UpdateBenefitPackage(c.Request.Context(), packageID, &service.UpdateBenefitPackageInput{
		Name:        req.Name,
		Description: req.Description,
		GroupID:     req.GroupID,
		LeaseDays:   req.LeaseDays,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, benefitPackageResponseFromService(*item))
}

// DeleteBenefitPackage handles deleting a benefit package
// DELETE /api/v1/admin/subscriptions/benefit-packages/:id
func (h *SubscriptionHandler) DeleteBenefitPackage(c *gin.Context) {
	packageID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid package ID")
		return
	}
	if err := h.subscriptionService.DeleteBenefitPackage(c.Request.Context(), packageID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Benefit package deleted successfully"})
}

// ListBenefitPlans handles listing benefit plans
// GET /api/v1/admin/subscriptions/benefit-plans
func (h *SubscriptionHandler) ListBenefitPlans(c *gin.Context) {
	plans, err := h.subscriptionService.ListBenefitPlans(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]BenefitPlanResponse, 0, len(plans))
	for _, item := range plans {
		out = append(out, benefitPlanResponseFromService(item))
	}
	response.Success(c, out)
}

// CreateBenefitPlan handles creating a benefit plan
// POST /api/v1/admin/subscriptions/benefit-plans
func (h *SubscriptionHandler) CreateBenefitPlan(c *gin.Context) {
	var req BenefitPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.subscriptionService.CreateBenefitPlan(c.Request.Context(), &service.CreateBenefitPlanInput{
		Name:        req.Name,
		Description: req.Description,
		PackageIDs:  req.PackageIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, benefitPlanResponseFromService(*item))
}

// UpdateBenefitPlan handles updating a benefit plan
// PUT /api/v1/admin/subscriptions/benefit-plans/:id
func (h *SubscriptionHandler) UpdateBenefitPlan(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID")
		return
	}
	var req BenefitPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.subscriptionService.UpdateBenefitPlan(c.Request.Context(), planID, &service.UpdateBenefitPlanInput{
		Name:        req.Name,
		Description: req.Description,
		PackageIDs:  req.PackageIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, benefitPlanResponseFromService(*item))
}

// DeleteBenefitPlan handles deleting a benefit plan
// DELETE /api/v1/admin/subscriptions/benefit-plans/:id
func (h *SubscriptionHandler) DeleteBenefitPlan(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID")
		return
	}
	if err := h.subscriptionService.DeleteBenefitPlan(c.Request.Context(), planID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Benefit plan deleted successfully"})
}

// ListBenefitPlanMembers handles listing users assigned to a benefit plan
// GET /api/v1/admin/subscriptions/benefit-plans/:id/users
func (h *SubscriptionHandler) ListBenefitPlanMembers(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID")
		return
	}

	members, err := h.subscriptionService.ListBenefitPlanMembers(c.Request.Context(), planID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]BenefitPlanMemberResponse, 0, len(members))
	for _, item := range members {
		out = append(out, benefitPlanMemberResponseFromService(item))
	}
	response.Success(c, out)
}

// BulkAssignBenefitPlanUsers handles assigning multiple users to a benefit plan
// POST /api/v1/admin/subscriptions/benefit-plans/:id/users/bulk-assign
func (h *SubscriptionHandler) BulkAssignBenefitPlanUsers(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID")
		return
	}

	var req BenefitPlanBulkUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	adminID := getAdminIDFromContext(c)
	var assignedBy *int64
	if adminID > 0 {
		assignedBy = &adminID
	}

	result, err := h.subscriptionService.BulkAssignUsersToBenefitPlan(c.Request.Context(), &service.BulkAssignUsersBenefitPlanInput{
		PlanID:     planID,
		UserIDs:    req.UserIDs,
		AssignedBy: assignedBy,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, benefitPlanUserBulkResultResponseFromService(result))
}

// BulkRemoveBenefitPlanUsers handles removing multiple users from a benefit plan
// POST /api/v1/admin/subscriptions/benefit-plans/:id/users/bulk-remove
func (h *SubscriptionHandler) BulkRemoveBenefitPlanUsers(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID")
		return
	}

	var req BenefitPlanBulkUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	adminID := getAdminIDFromContext(c)
	var assignedBy *int64
	if adminID > 0 {
		assignedBy = &adminID
	}

	result, err := h.subscriptionService.BulkRemoveUsersFromBenefitPlan(c.Request.Context(), &service.BulkRemoveUsersBenefitPlanInput{
		PlanID:     planID,
		UserIDs:    req.UserIDs,
		AssignedBy: assignedBy,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, benefitPlanUserBulkResultResponseFromService(result))
}

// GetUserBenefitPlan handles getting one user benefit plan assignment for compatibility.
// When multiple plans exist, returns the most recently assigned one.
// GET /api/v1/admin/users/:id/benefit-plan
func (h *SubscriptionHandler) GetUserBenefitPlan(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	assignment, err := h.subscriptionService.GetUserBenefitPlanAssignment(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if assignment == nil {
		response.Success(c, nil)
		return
	}
	response.Success(c, userBenefitPlanAssignmentResponseFromService(*assignment))
}

// AssignUserBenefitPlan handles assigning/clearing user benefit plan assignments.
// plan_id != nil appends/refreshes one plan, plan_id == nil clears all assignments.
// PUT /api/v1/admin/users/:id/benefit-plan
func (h *SubscriptionHandler) AssignUserBenefitPlan(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	var req AssignUserBenefitPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.PlanID != nil && *req.PlanID <= 0 {
		response.BadRequest(c, "plan_id must be positive or null")
		return
	}

	adminID := getAdminIDFromContext(c)
	var assignedBy *int64
	if adminID > 0 {
		assignedBy = &adminID
	}
	assignment, err := h.subscriptionService.AssignUserBenefitPlan(c.Request.Context(), &service.AssignUserBenefitPlanInput{
		UserID:     userID,
		PlanID:     req.PlanID,
		AssignedBy: assignedBy,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if assignment == nil {
		response.Success(c, nil)
		return
	}
	response.Success(c, userBenefitPlanAssignmentResponseFromService(*assignment))
}

func benefitPackageResponseFromService(item service.BenefitPackage) BenefitPackageResponse {
	return BenefitPackageResponse{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		GroupID:     item.GroupID,
		GroupName:   item.GroupName,
		LeaseDays:   item.LeaseDays,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

func benefitPlanResponseFromService(item service.BenefitPlan) BenefitPlanResponse {
	packages := make([]BenefitPlanPackageResponse, 0, len(item.Packages))
	for _, pkg := range item.Packages {
		packages = append(packages, BenefitPlanPackageResponse{
			PackageID: pkg.PackageID,
			SortOrder: pkg.SortOrder,
			Name:      pkg.Name,
			GroupID:   pkg.GroupID,
			GroupName: pkg.GroupName,
			LeaseDays: pkg.LeaseDays,
		})
	}
	return BenefitPlanResponse{
		ID:                item.ID,
		Name:              item.Name,
		Description:       item.Description,
		Packages:          packages,
		AssignedUserCount: item.AssignedUserCount,
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
	}
}

func userBenefitPlanAssignmentResponseFromService(item service.UserBenefitPlanAssignment) UserBenefitPlanAssignmentResponse {
	return UserBenefitPlanAssignmentResponse{
		UserID:     item.UserID,
		PlanID:     item.PlanID,
		PlanName:   item.PlanName,
		Version:    item.Version,
		AssignedBy: item.AssignedBy,
		AssignedAt: item.AssignedAt,
		UpdatedAt:  item.UpdatedAt,
	}
}

func benefitPlanMemberResponseFromService(item service.BenefitPlanMember) BenefitPlanMemberResponse {
	return BenefitPlanMemberResponse{
		UserID:     item.UserID,
		Email:      item.Email,
		Role:       item.Role,
		Status:     item.Status,
		Version:    item.Version,
		AssignedAt: item.AssignedAt,
		UpdatedAt:  item.UpdatedAt,
	}
}

func benefitPlanUserBulkResultResponseFromService(item *service.BenefitPlanUserBulkResult) *BenefitPlanUserBulkResultResponse {
	if item == nil {
		return nil
	}

	statuses := make(map[string]string, len(item.Statuses))
	for userID, status := range item.Statuses {
		statuses[strconv.FormatInt(userID, 10)] = status
	}

	return &BenefitPlanUserBulkResultResponse{
		SuccessCount:   item.SuccessCount,
		FailedCount:    item.FailedCount,
		AssignedCount:  item.AssignedCount,
		RemovedCount:   item.RemovedCount,
		UnchangedCount: item.UnchangedCount,
		SkippedCount:   item.SkippedCount,
		Errors:         item.Errors,
		Statuses:       statuses,
	}
}

// Helper function to get admin ID from context
func getAdminIDFromContext(c *gin.Context) int64 {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		return 0
	}
	return subject.UserID
}

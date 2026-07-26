package handler

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type OrganizationHandler struct {
	organization *service.OrganizationService
	auth         *service.AuthService
	operations   *service.CompanyOperationsMonitor
}

const OrganizationContextKey = "organization_context"

func NewOrganizationHandler(organization *service.OrganizationService, auth *service.AuthService, operations *service.CompanyOperationsMonitor) *OrganizationHandler {
	return &OrganizationHandler{organization: organization, auth: auth, operations: operations}
}

// RequireOrganization derives organization scope exclusively from the
// authenticated subject. Handlers and repositories still enforce their
// action-specific owner/policy checks against current database state.
func (h *OrganizationHandler) RequireOrganization(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		c.Abort()
		return
	}
	organization, err := h.organization.Context(c.Request.Context(), userID)
	if err != nil || organization == nil || !organization.Active() {
		if err == nil {
			err = service.ErrOrganizationPermission
		}
		response.ErrorFrom(c, err)
		c.Abort()
		return
	}
	c.Set(OrganizationContextKey, organization)
	c.Next()
}

func organizationSubject(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return 0, false
	}
	return subject.UserID, true
}

func parseOrganizationIDParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid identifier")
		return 0, false
	}
	return id, true
}

func (h *OrganizationHandler) Context(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	org, err := h.organization.Context(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	finance, _ := h.organization.FinanceSummary(c.Request.Context(), userID)
	response.Success(c, gin.H{"organization": org, "finance": finance})
}

func (h *OrganizationHandler) CurrentApplication(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	application, err := h.organization.CurrentApplication(c.Request.Context(), userID)
	if err != nil {
		if err == service.ErrApplicationNotFound {
			response.Success(c, gin.H{"application": nil})
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"application": application})
}

func (h *OrganizationHandler) UpgradeEligibility(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	eligibility, err := h.organization.UpgradeEligibility(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, eligibility)
}

func (h *OrganizationHandler) SubmitApplication(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	var req struct {
		CompanyName    string `json:"company_name" binding:"required"`
		IdempotencyKey string `json:"idempotency_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid company application")
		return
	}
	application, err := h.organization.SubmitApplication(c.Request.Context(), userID, req.CompanyName, req.IdempotencyKey)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, application)
}

func (h *OrganizationHandler) WithdrawApplication(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	applicationID, ok := parseOrganizationIDParam(c, "application_id")
	if !ok {
		return
	}
	application, err := h.organization.WithdrawApplication(c.Request.Context(), userID, applicationID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, application)
}

func (h *OrganizationHandler) RequestNameChange(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	var req struct {
		CompanyName string `json:"company_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid company name")
		return
	}
	if err := h.organization.RequestNameChange(c.Request.Context(), userID, req.CompanyName); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, gin.H{"status": "pending"})
}

func (h *OrganizationHandler) CreateMember(c *gin.Context) {
	ownerID, ok := organizationSubject(c)
	if !ok {
		return
	}
	var req struct {
		LoginName          string `json:"login_name" binding:"required"`
		RecoveryEmail      string `json:"recovery_email"`
		Password           string `json:"password" binding:"required"`
		MustChangePassword *bool  `json:"must_change_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid IAM member")
		return
	}
	mustChangePassword := true
	if req.MustChangePassword != nil {
		mustChangePassword = *req.MustChangePassword
	}
	member, password, err := h.organization.CreateIAMMember(c.Request.Context(), ownerID, req.LoginName, req.RecoveryEmail, req.Password, mustChangePassword)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, gin.H{"member": member, "initial_password": password})
}

func (h *OrganizationHandler) ListMembers(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	members, limit, err := h.organization.ListIAMMembers(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": members, "member_limit": limit, "used_slots": countIAMSlots(members)})
}

func (h *OrganizationHandler) GetMember(c *gin.Context) {
	ownerID, ok := organizationSubject(c)
	if !ok {
		return
	}
	memberID, ok := parseOrganizationIDParam(c, "member_id")
	if !ok {
		return
	}
	member, err := h.organization.GetIAMMember(c.Request.Context(), ownerID, memberID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, member)
}

func countIAMSlots(members []service.IAMMember) int {
	count := 0
	for _, member := range members {
		if member.Status != service.MembershipStatusArchived {
			count++
		}
	}
	return count
}

func (h *OrganizationHandler) SetMemberStatus(c *gin.Context) {
	ownerID, ok := organizationSubject(c)
	if !ok {
		return
	}
	memberID, ok := parseOrganizationIDParam(c, "member_id")
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid member status")
		return
	}
	if err := h.organization.SetIAMMemberStatus(c.Request.Context(), ownerID, memberID, req.Status); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if req.Status != service.MembershipStatusActive && h.auth != nil {
		_ = h.auth.RevokeAllUserSessions(c.Request.Context(), memberID)
	}
	response.Success(c, gin.H{"status": req.Status})
}

func (h *OrganizationHandler) ResetMemberPassword(c *gin.Context) {
	ownerID, ok := organizationSubject(c)
	if !ok {
		return
	}
	memberID, ok := parseOrganizationIDParam(c, "member_id")
	if !ok {
		return
	}
	password, err := h.organization.ResetIAMPassword(c.Request.Context(), ownerID, memberID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.auth != nil {
		_ = h.auth.RevokeAllUserSessions(c.Request.Context(), memberID)
	}
	response.Success(c, gin.H{"initial_password": password, "must_change_password": true})
}

func (h *OrganizationHandler) ChangePassword(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	var req struct {
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid password")
		return
	}
	user, err := h.organization.ChangeIAMPassword(c.Request.Context(), userID, req.NewPassword)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.auth == nil {
		response.InternalError(c, "Authentication service unavailable")
		return
	}
	_ = h.auth.RevokeAllUserSessions(c.Request.Context(), userID)
	pair, err := h.auth.GenerateTokenPair(c.Request.Context(), user, "")
	if err != nil {
		response.InternalError(c, "Failed to generate token")
		return
	}
	response.Success(c, gin.H{
		"access_token": pair.AccessToken, "refresh_token": pair.RefreshToken,
		"expires_in": pair.ExpiresIn, "token_type": "Bearer", "user": dto.UserFromService(user),
	})
}

func (h *OrganizationHandler) SendRecoveryEmailCode(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid recovery email")
		return
	}
	if err := h.auth.SendIAMRecoveryEmailCode(c.Request.Context(), userID, req.Email, c.GetHeader("Accept-Language")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, gin.H{"status": "verification_sent"})
}

func (h *OrganizationHandler) VerifyRecoveryEmail(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email" binding:"required"`
		Code  string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid verification code")
		return
	}
	if err := h.auth.VerifyIAMRecoveryEmail(c.Request.Context(), userID, req.Email, req.Code); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"verified": true})
}

func (h *OrganizationHandler) IAMLogin(c *gin.Context) {
	var req struct {
		Principal string `json:"principal" binding:"required"`
		Password  string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, service.ErrInvalidCredentials)
		return
	}
	user, org, err := h.organization.AuthenticateIAM(c.Request.Context(), req.Principal, req.Password)
	if err != nil {
		response.ErrorFrom(c, service.ErrInvalidCredentials)
		return
	}
	pair, err := h.auth.GenerateTokenPair(c.Request.Context(), user, "")
	if err != nil {
		response.InternalError(c, "Failed to generate token")
		return
	}
	h.auth.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	response.Success(c, gin.H{
		"access_token": pair.AccessToken, "refresh_token": pair.RefreshToken,
		"expires_in": pair.ExpiresIn, "token_type": "Bearer", "user": dto.UserFromService(user),
		"organization": org,
	})
}

func (h *OrganizationHandler) ListPolicies(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	policies, err := h.organization.ListPolicies(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": policies})
}

func (h *OrganizationHandler) ListMemberPolicies(c *gin.Context) {
	ownerID, ok := organizationSubject(c)
	if !ok {
		return
	}
	memberID, ok := parseOrganizationIDParam(c, "member_id")
	if !ok {
		return
	}
	policies, err := h.organization.ListMemberPolicyAttachments(c.Request.Context(), ownerID, memberID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": policies})
}

func (h *OrganizationHandler) SetPolicy(c *gin.Context) {
	ownerID, ok := organizationSubject(c)
	if !ok {
		return
	}
	memberID, ok := parseOrganizationIDParam(c, "member_id")
	if !ok {
		return
	}
	var req struct {
		PolicyKey string `json:"policy_key" binding:"required"`
		Attached  bool   `json:"attached"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid policy attachment")
		return
	}
	if err := h.organization.SetPolicyAttachment(c.Request.Context(), ownerID, memberID, req.PolicyKey, req.Attached, c.GetHeader("X-Request-ID")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"attached": req.Attached})
}

func (h *OrganizationHandler) TransferBalance(c *gin.Context) {
	ownerID, ok := organizationSubject(c)
	if !ok {
		return
	}
	memberID, ok := parseOrganizationIDParam(c, "member_id")
	if !ok {
		return
	}
	var req struct {
		Amount         string `json:"amount" binding:"required"`
		IdempotencyKey string `json:"idempotency_key" binding:"required"`
		Operation      string `json:"operation" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Operation != "allocate" && req.Operation != "reclaim") {
		response.BadRequest(c, "Invalid balance transfer")
		return
	}
	if err := h.organization.TransferBalance(c.Request.Context(), ownerID, memberID, req.Amount, req.IdempotencyKey, req.Operation == "reclaim"); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"operation": req.Operation})
}

func (h *OrganizationHandler) Finance(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	summary, err := h.organization.FinanceSummary(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, summary)
}

func (h *OrganizationHandler) Usage(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	filter := service.OrganizationUsageFilter{
		Model: c.Query("model"), Endpoint: c.Query("endpoint"), Status: c.Query("status"),
		Page: parsePositiveQuery(c.Query("page"), 1), PageSize: parsePositiveQuery(c.Query("page_size"), 20),
	}
	if value := c.Query("start"); value != "" {
		filter.Start, _ = time.Parse(time.RFC3339, value)
	}
	if value := c.Query("end"); value != "" {
		filter.End, _ = time.Parse(time.RFC3339, value)
	}
	if value := c.Query("member_id"); value != "" {
		if id, err := strconv.ParseInt(value, 10, 64); err == nil && id > 0 {
			filter.MemberID = &id
		}
	}
	if value := c.Query("api_key_id"); value != "" {
		if id, err := strconv.ParseInt(value, 10, 64); err == nil && id > 0 {
			filter.APIKeyID = &id
		}
	}
	items, total, err := h.organization.ListUsage(c.Request.Context(), userID, filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, response.PaginatedData{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize, Pages: int((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))})
}

func organizationUsageTimeFilter(c *gin.Context) service.OrganizationUsageFilter {
	filter := service.OrganizationUsageFilter{
		Model: c.Query("model"), Endpoint: c.Query("endpoint"), Status: c.Query("status"),
	}
	if value := c.Query("start"); value != "" {
		filter.Start, _ = time.Parse(time.RFC3339, value)
	}
	if value := c.Query("end"); value != "" {
		filter.End, _ = time.Parse(time.RFC3339, value)
	}
	if value := c.Query("member_id"); value != "" {
		if id, err := strconv.ParseInt(value, 10, 64); err == nil && id > 0 {
			filter.MemberID = &id
		}
	}
	if value := c.Query("api_key_id"); value != "" {
		if id, err := strconv.ParseInt(value, 10, 64); err == nil && id > 0 {
			filter.APIKeyID = &id
		}
	}
	return filter
}

func (h *OrganizationHandler) UsageStats(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	stats, err := h.organization.UsageStats(c.Request.Context(), userID, organizationUsageTimeFilter(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

func (h *OrganizationHandler) UsageTrend(c *gin.Context) {
	userID, ok := organizationSubject(c)
	if !ok {
		return
	}
	trend, err := h.organization.UsageTrend(c.Request.Context(), userID, organizationUsageTimeFilter(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": trend})
}

func parsePositiveQuery(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func (h *OrganizationHandler) AdminListApplications(c *gin.Context) {
	page, pageSize := parsePositiveQuery(c.Query("page"), 1), parsePositiveQuery(c.Query("page_size"), 20)
	items, total, err := h.organization.ListApplications(c.Request.Context(), c.Query("status"), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, response.PaginatedData{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: int((total + int64(pageSize) - 1) / int64(pageSize))})
}

func (h *OrganizationHandler) AdminGetApplication(c *gin.Context) {
	applicationID, ok := parseOrganizationIDParam(c, "application_id")
	if !ok {
		return
	}
	detail, err := h.organization.GetApplication(c.Request.Context(), applicationID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, detail)
}

func (h *OrganizationHandler) AdminListNameChanges(c *gin.Context) {
	page, pageSize := parsePositiveQuery(c.Query("page"), 1), parsePositiveQuery(c.Query("page_size"), 20)
	items, total, err := h.organization.ListNameChangeRequests(c.Request.Context(), c.Query("status"), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, response.PaginatedData{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: int((total + int64(pageSize) - 1) / int64(pageSize))})
}

func (h *OrganizationHandler) AdminGetNameChange(c *gin.Context) {
	requestID, ok := parseOrganizationIDParam(c, "request_id")
	if !ok {
		return
	}
	request, err := h.organization.GetNameChangeRequest(c.Request.Context(), requestID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, request)
}

func (h *OrganizationHandler) AdminListOrganizations(c *gin.Context) {
	actorID, ok := organizationSubject(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := h.organization.ListOrganizations(c.Request.Context(), actorID, c.Query("status"), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (h *OrganizationHandler) AdminOperations(c *gin.Context) {
	if h.operations == nil {
		response.InternalError(c, "Company operations monitor is unavailable")
		return
	}
	snapshot, err := h.operations.Collect(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, snapshot)
}

func (h *OrganizationHandler) AdminGetOrganization(c *gin.Context) {
	actorID, ok := organizationSubject(c)
	if !ok {
		return
	}
	organizationID, ok := parseOrganizationIDParam(c, "organization_id")
	if !ok {
		return
	}
	detail, err := h.organization.GetOrganization(c.Request.Context(), actorID, organizationID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, detail)
}

func (h *OrganizationHandler) AdminDecideApplication(c *gin.Context) {
	reviewerID, ok := organizationSubject(c)
	if !ok {
		return
	}
	applicationID, ok := parseOrganizationIDParam(c, "application_id")
	if !ok {
		return
	}
	var req struct {
		Decision string `json:"decision" binding:"required"`
		Reason   string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Decision != "approve" && req.Decision != "reject") {
		response.BadRequest(c, "Invalid review decision")
		return
	}
	application, err := h.organization.DecideApplication(c.Request.Context(), reviewerID, applicationID, req.Decision == "approve", req.Reason)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, application)
}

func (h *OrganizationHandler) AdminDecideNameChange(c *gin.Context) {
	reviewerID, ok := organizationSubject(c)
	if !ok {
		return
	}
	requestID, ok := parseOrganizationIDParam(c, "request_id")
	if !ok {
		return
	}
	var req struct {
		Decision string `json:"decision" binding:"required"`
		Reason   string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Decision != "approve" && req.Decision != "reject") {
		response.BadRequest(c, "Invalid review decision")
		return
	}
	if err := h.organization.DecideNameChange(c.Request.Context(), reviewerID, requestID, req.Decision == "approve", req.Reason); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"status": req.Decision + "d"})
}

func (h *OrganizationHandler) AdminSetOrganizationStatus(c *gin.Context) {
	actorID, ok := organizationSubject(c)
	if !ok {
		return
	}
	organizationID, ok := parseOrganizationIDParam(c, "organization_id")
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid organization status")
		return
	}
	if err := h.organization.SetOrganizationStatus(c.Request.Context(), actorID, organizationID, req.Status); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"status": req.Status})
}

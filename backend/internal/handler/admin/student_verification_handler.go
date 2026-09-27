package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// StudentVerificationHandler manages student verification records for admins.
type StudentVerificationHandler struct {
	studentVerificationService *service.StudentVerificationService
}

func NewStudentVerificationHandler(studentVerificationService *service.StudentVerificationService) *StudentVerificationHandler {
	return &StudentVerificationHandler{studentVerificationService: studentVerificationService}
}

type studentVerificationAdminView struct {
	ID              int64    `json:"id"`
	UserID          int64    `json:"user_id"`
	Email           string   `json:"email"`
	Status          string   `json:"status"`
	VerifiedAt      string   `json:"verified_at"`
	ExpiresAt       string   `json:"expires_at"`
	RevokedAt       *string  `json:"revoked_at,omitempty"`
	RevokedBy       *int64   `json:"revoked_by,omitempty"`
	RevokeReason    *string  `json:"revoke_reason,omitempty"`
	GrantedGroupIDs []int64  `json:"granted_group_ids"`
	RebateRate      *float64 `json:"rebate_rate_applied,omitempty"`
	CreatedAt       string   `json:"created_at"`
}

func studentVerificationAdminViewOf(v *service.StudentVerification) studentVerificationAdminView {
	var revokedAt *string
	if v.RevokedAt != nil {
		s := v.RevokedAt.Format("2006-01-02T15:04:05Z07:00")
		revokedAt = &s
	}
	return studentVerificationAdminView{
		ID:              v.ID,
		UserID:          v.UserID,
		Email:           service.MaskEmail(v.EmailRaw),
		Status:          v.Status,
		VerifiedAt:      v.VerifiedAt.Format("2006-01-02T15:04:05Z07:00"),
		ExpiresAt:       v.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		RevokedAt:       revokedAt,
		RevokedBy:       v.RevokedBy,
		RevokeReason:    v.RevokeReason,
		GrantedGroupIDs: v.GrantedGroupIDs,
		RebateRate:      v.RebateRateApplied,
		CreatedAt:       v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// List GET /api/v1/admin/student-verifications?status=&keyword=&page=&page_size=
func (h *StudentVerificationHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortOrder: "desc"}
	records, pag, err := h.studentVerificationService.AdminList(
		c.Request.Context(), params, c.Query("status"), c.Query("keyword"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	views := make([]studentVerificationAdminView, 0, len(records))
	for i := range records {
		views = append(views, studentVerificationAdminViewOf(&records[i]))
	}
	response.Paginated(c, views, pag.Total, page, pageSize)
}

type studentVerificationRevokeRequest struct {
	Reason string `json:"reason"`
}

// Revoke POST /api/v1/admin/student-verifications/:id/revoke
func (h *StudentVerificationHandler) Revoke(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid id")
		return
	}
	var req studentVerificationRevokeRequest
	_ = c.ShouldBindJSON(&req) // reason is optional
	if err := h.studentVerificationService.AdminRevoke(c.Request.Context(), id, subject.UserID, req.Reason); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id, "status": service.StudentVerificationStatusRevoked})
}

// Delete DELETE /api/v1/admin/student-verifications/:id — removes the record
// and releases the school email for a new claim.
func (h *StudentVerificationHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid id")
		return
	}
	if err := h.studentVerificationService.AdminDelete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id, "deleted": true})
}

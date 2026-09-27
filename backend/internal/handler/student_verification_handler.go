package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// StudentVerificationHandler handles the logged-in user's student verification.
type StudentVerificationHandler struct {
	studentVerificationService *service.StudentVerificationService
}

func NewStudentVerificationHandler(studentVerificationService *service.StudentVerificationService) *StudentVerificationHandler {
	return &StudentVerificationHandler{studentVerificationService: studentVerificationService}
}

type studentVerificationView struct {
	Status     string `json:"status"`
	Email      string `json:"email"`
	VerifiedAt string `json:"verified_at"`
	ExpiresAt  string `json:"expires_at"`
}

type studentVerificationStatusResponse struct {
	Enabled      bool                     `json:"enabled"`
	Verification *studentVerificationView `json:"verification,omitempty"`
	RebateRate   float64                  `json:"rebate_rate"`
	GroupCount   int                      `json:"group_count"`
}

func studentVerificationToView(v *service.StudentVerification) *studentVerificationView {
	if v == nil {
		return nil
	}
	return &studentVerificationView{
		Status:     v.Status,
		Email:      service.MaskEmail(v.EmailRaw),
		VerifiedAt: v.VerifiedAt.Format("2006-01-02T15:04:05Z07:00"),
		ExpiresAt:  v.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// GetStatus GET /api/v1/user/student-verification
func (h *StudentVerificationHandler) GetStatus(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	result, err := h.studentVerificationService.GetStatus(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, studentVerificationStatusResponse{
		Enabled:      result.Enabled,
		Verification: studentVerificationToView(result.Verification),
		RebateRate:   result.RebateRate,
		GroupCount:   result.GroupCount,
	})
}

type studentVerificationSendCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// SendCode POST /api/v1/user/student-verification/send-code
func (h *StudentVerificationHandler) SendCode(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req studentVerificationSendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	locale := c.GetHeader("Accept-Language")
	if err := h.studentVerificationService.SendCode(c.Request.Context(), subject.UserID, req.Email, locale); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"sent": true})
}

type studentVerificationVerifyRequest struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required"`
}

// Verify POST /api/v1/user/student-verification/verify
func (h *StudentVerificationHandler) Verify(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req studentVerificationVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	v, err := h.studentVerificationService.Verify(c.Request.Context(), subject.UserID, req.Email, req.Code)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, studentVerificationStatusResponse{
		Enabled:      true,
		Verification: studentVerificationToView(v),
	})
}

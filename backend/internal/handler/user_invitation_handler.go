package handler

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UserInvitationHandler 处理已注册用户自助生成邀请码的请求。
type UserInvitationHandler struct {
	invitationService *service.UserInvitationService
}

// NewUserInvitationHandler creates a new UserInvitationHandler
func NewUserInvitationHandler(invitationService *service.UserInvitationService) *UserInvitationHandler {
	return &UserInvitationHandler{invitationService: invitationService}
}

// 邀请码对用户展示的状态。
const (
	userInvitationStatusUnused  = "unused"
	userInvitationStatusUsed    = "used"
	userInvitationStatusExpired = "expired"
)

// UserInvitationCode 用户邀请码（不暴露内部 ID 与备注，被邀请人邮箱脱敏）。
type UserInvitationCode struct {
	Code            string     `json:"code"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	ExpiresAt       *time.Time `json:"expires_at"`
	UsedAt          *time.Time `json:"used_at"`
	UsedByEmailMask string     `json:"used_by_email_mask,omitempty"`
}

// UserInvitationOverviewResponse 邀请页面数据。
type UserInvitationOverviewResponse struct {
	Enabled          bool                 `json:"enabled"`
	MaxCodesPerUser  int                  `json:"max_codes_per_user"` // 0 = 不限
	UsedCount        int                  `json:"used_count"`         // 已占用名额（已使用 + 未过期未使用）
	Remaining        int                  `json:"remaining"`          // -1 = 不限
	CodeValidityDays int                  `json:"code_validity_days"` // 0 = 永不过期
	Codes            []UserInvitationCode `json:"codes"`
}

func userInvitationCodeFromService(code *service.RedeemCode, now time.Time) UserInvitationCode {
	out := UserInvitationCode{
		Code:      code.Code,
		Status:    userInvitationStatusUnused,
		CreatedAt: code.CreatedAt,
		ExpiresAt: code.ExpiresAt,
		UsedAt:    code.UsedAt,
	}
	switch {
	case code.IsUsed():
		out.Status = userInvitationStatusUsed
		if code.User != nil {
			out.UsedByEmailMask = service.MaskEmail(code.User.Email)
		}
	case code.IsExpiredAt(now):
		out.Status = userInvitationStatusExpired
	}
	return out
}

// GetOverview 返回当前用户的邀请名额与邀请记录
// GET /api/v1/invitations
func (h *UserInvitationHandler) GetOverview(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	overview, err := h.invitationService.GetOverview(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	now := time.Now()
	codes := make([]UserInvitationCode, 0, len(overview.Codes))
	for i := range overview.Codes {
		codes = append(codes, userInvitationCodeFromService(&overview.Codes[i], now))
	}
	response.Success(c, UserInvitationOverviewResponse{
		Enabled:          overview.Available,
		MaxCodesPerUser:  overview.MaxCodesPerUser,
		UsedCount:        overview.UsedQuota,
		Remaining:        overview.Remaining(),
		CodeValidityDays: overview.CodeValidityDays,
		Codes:            codes,
	})
}

// Create 为当前用户生成一个一次性邀请码
// POST /api/v1/invitations
func (h *UserInvitationHandler) Create(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	code, err := h.invitationService.CreateInvitation(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, userInvitationCodeFromService(code, time.Now()))
}

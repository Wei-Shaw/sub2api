package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RedeemHandler handles redeem code-related requests
type RedeemHandler struct {
	redeemService *service.RedeemService
	promoService  *service.PromoService
	userRepo      service.UserRepository
}

// NewRedeemHandler creates a new RedeemHandler
func NewRedeemHandler(redeemService *service.RedeemService, promoService *service.PromoService, userRepo service.UserRepository) *RedeemHandler {
	return &RedeemHandler{
		redeemService: redeemService,
		promoService:  promoService,
		userRepo:      userRepo,
	}
}

// RedeemRequest represents the redeem code request payload
type RedeemRequest struct {
	Code string `json:"code" binding:"required"`
}

// RedeemResponse represents the redeem response
type RedeemResponse struct {
	Message        string   `json:"message"`
	Type           string   `json:"type"`
	Value          float64  `json:"value"`
	NewBalance     *float64 `json:"new_balance,omitempty"`
	NewConcurrency *int     `json:"new_concurrency,omitempty"`
}

type PromoRedeemRequest struct {
	Code string `json:"code" binding:"required"`
}

type PromoRedeemResponse struct {
	Message     string  `json:"message"`
	BonusAmount float64 `json:"bonus_amount"`
	NewBalance  float64 `json:"new_balance"`
}

// Redeem handles redeeming a code
// POST /api/v1/redeem
func (h *RedeemHandler) Redeem(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req RedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.redeemService.Redeem(c.Request.Context(), subject.UserID, req.Code)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.RedeemCodeFromService(result))
}

// RedeemPromoCode handles logged-in promo code redemption.
// POST /api/v1/redeem/promo
func (h *RedeemHandler) RedeemPromoCode(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if h.promoService == nil || h.userRepo == nil {
		response.BadRequest(c, "Promo code redemption is not available")
		return
	}

	var req PromoRedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	promoCode, err := h.promoService.ValidatePromoCode(c.Request.Context(), req.Code)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if promoCode == nil {
		response.BadRequest(c, "Promo code is required")
		return
	}

	if err := h.promoService.ApplyPromoCode(c.Request.Context(), subject.UserID, req.Code); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	user, err := h.userRepo.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, PromoRedeemResponse{
		Message:     "优惠码领取成功，余额已更新",
		BonusAmount: promoCode.BonusAmount,
		NewBalance:  user.Balance,
	})
}

// GetHistory returns the user's redemption history
// GET /api/v1/redeem/history
func (h *RedeemHandler) GetHistory(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	// Default limit is 25
	limit := 25

	codes, err := h.redeemService.GetUserHistory(c.Request.Context(), subject.UserID, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.RedeemCode, 0, len(codes))
	for i := range codes {
		out = append(out, *dto.RedeemCodeFromService(&codes[i]))
	}
	response.Success(c, out)
}

package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication-related requests
type AuthHandler struct {
	cfg          *config.Config
	authService  *service.AuthService
	userService  *service.UserService
	settingSvc   *service.SettingService
	promoService *service.PromoService
REDACTED

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(cfg *config.Config, authService *service.AuthService, userService *service.UserService, settingService *service.SettingService, promoService *service.PromoService) *AuthHandler {
	return &AuthHandler{
		cfg:          cfg,
		authService:  authService,
		userService:  userService,
		settingSvc:   settingService,
		promoService: promoService,
REDACTED
REDACTED

// RegisterRequest represents the registration request payload
type RegisterRequest struct {
	Email          string `json:"email" binding:"required,email"`
	Password       string `json:"password" binding:"required,min=6"`
	VerifyCode     string `json:"verify_code"`
	TurnstileToken string `json:"turnstile_token"`
	PromoCode      string `json:"promo_code"` // 注册优惠码
REDACTED

// SendVerifyCodeRequest 发送验证码请求
type SendVerifyCodeRequest struct {
	Email          string `json:"email" binding:"required,email"`
	TurnstileToken string `json:"turnstile_token"`
REDACTED

// SendVerifyCodeResponse 发送验证码响应
type SendVerifyCodeResponse struct {
	Message   string `json:"message"`
	Countdown int    `json:"countdown"` // 倒计时秒数
REDACTED

// LoginRequest represents the login request payload
type LoginRequest struct {
	Email          string `json:"email" binding:"required,email"`
	Password       string `json:"password" binding:"required"`
	TurnstileToken string `json:"turnstile_token"`
REDACTED

// AuthResponse 认证响应格式（匹配前端期望）
type AuthResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	User        *dto.User `json:"user"`
REDACTED

// Register handles user registration
// POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	// Turnstile 验证（当提供了邮箱验证码时跳过，因为发送验证码时已验证过）
	if req.VerifyCode == "" {
		if err := h.authService.VerifyTurnstile(c.Request.Context(), req.TurnstileToken, ip.GetClientIP(c)); err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
REDACTED

	token, user, err := h.authService.RegisterWithVerification(c.Request.Context(), req.Email, req.Password, req.VerifyCode, req.PromoCode)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, AuthResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		User:        dto.UserFromService(user),
REDACTED)
REDACTED

// SendVerifyCode 发送邮箱验证码
// POST /api/v1/auth/send-verify-code
func (h *AuthHandler) SendVerifyCode(c *gin.Context) {
	var req SendVerifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	// Turnstile 验证
	if err := h.authService.VerifyTurnstile(c.Request.Context(), req.TurnstileToken, ip.GetClientIP(c)); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	result, err := h.authService.SendVerifyCodeAsync(c.Request.Context(), req.Email)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, SendVerifyCodeResponse{
		Message:   "Verification code sent successfully",
		Countdown: result.Countdown,
REDACTED)
REDACTED

// Login handles user login
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	// Turnstile 验证
	if err := h.authService.VerifyTurnstile(c.Request.Context(), req.TurnstileToken, ip.GetClientIP(c)); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	token, user, err := h.authService.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, AuthResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		User:        dto.UserFromService(user),
REDACTED)
REDACTED

// GetCurrentUser handles getting current authenticated user
// GET /api/v1/auth/me
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	user, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	type UserResponse struct {
		*dto.User
		RunMode string `json:"run_mode"`
REDACTED

	runMode := config.RunModeStandard
	if h.cfg != nil {
		runMode = h.cfg.RunMode
REDACTED

	response.Success(c, UserResponse{User: dto.UserFromService(user), RunMode: runModeREDACTED)
REDACTED

// ValidatePromoCodeRequest 验证优惠码请求
type ValidatePromoCodeRequest struct {
	Code string `json:"code" binding:"required"`
REDACTED

// ValidatePromoCodeResponse 验证优惠码响应
type ValidatePromoCodeResponse struct {
	Valid       bool    `json:"valid"`
	BonusAmount float64 `json:"bonus_amount,omitempty"`
	ErrorCode   string  `json:"error_code,omitempty"`
	Message     string  `json:"message,omitempty"`
REDACTED

// ValidatePromoCode 验证优惠码（公开接口，注册前调用）
// POST /api/v1/auth/validate-promo-code
func (h *AuthHandler) ValidatePromoCode(c *gin.Context) {
	// 检查优惠码功能是否启用
	if h.settingSvc != nil && !h.settingSvc.IsPromoCodeEnabled(c.Request.Context()) {
		response.Success(c, ValidatePromoCodeResponse{
			Valid:     false,
			ErrorCode: "PROMO_CODE_DISABLED",
	REDACTED)
		return
REDACTED

	var req ValidatePromoCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	promoCode, err := h.promoService.ValidatePromoCode(c.Request.Context(), req.Code)
	if err != nil {
		// 根据错误类型返回对应的错误码
		errorCode := "PROMO_CODE_INVALID"
		switch err {
		case service.ErrPromoCodeNotFound:
			errorCode = "PROMO_CODE_NOT_FOUND"
		case service.ErrPromoCodeExpired:
			errorCode = "PROMO_CODE_EXPIRED"
		case service.ErrPromoCodeDisabled:
			errorCode = "PROMO_CODE_DISABLED"
		case service.ErrPromoCodeMaxUsed:
			errorCode = "PROMO_CODE_MAX_USED"
		case service.ErrPromoCodeAlreadyUsed:
			errorCode = "PROMO_CODE_ALREADY_USED"
	REDACTED

		response.Success(c, ValidatePromoCodeResponse{
			Valid:     false,
			ErrorCode: errorCode,
	REDACTED)
		return
REDACTED

	if promoCode == nil {
		response.Success(c, ValidatePromoCodeResponse{
			Valid:     false,
			ErrorCode: "PROMO_CODE_INVALID",
	REDACTED)
		return
REDACTED

	response.Success(c, ValidatePromoCodeResponse{
		Valid:       true,
		BonusAmount: promoCode.BonusAmount,
REDACTED)
REDACTED

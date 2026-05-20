package handler

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UserHandler handles user-related requests
type UserHandler struct {
	userService      *service.UserService
	authService      *service.AuthService
	emailService     *service.EmailService
	emailCache       service.EmailCache
	affiliateService *service.AffiliateService
REDACTED

// NewUserHandler creates a new UserHandler
func NewUserHandler(
	userService *service.UserService,
	authService *service.AuthService,
	emailService *service.EmailService,
	emailCache service.EmailCache,
	affiliateService *service.AffiliateService,
) *UserHandler {
	return &UserHandler{
		userService:      userService,
		authService:      authService,
		emailService:     emailService,
		emailCache:       emailCache,
		affiliateService: affiliateService,
REDACTED
REDACTED

// ChangePasswordRequest represents the change password request payload
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
REDACTED

// UpdateProfileRequest represents the update profile request payload
type UpdateProfileRequest struct {
	Username               *string  `json:"username"`
	AvatarURL              *string  `json:"avatar_url"`
	BalanceNotifyEnabled   *bool    `json:"balance_notify_enabled"`
	BalanceNotifyThreshold *float64 `json:"balance_notify_threshold"`
REDACTED

type userProfileResponse struct {
	dto.User
	AvatarURL         string                                 `json:"avatar_url,omitempty"`
	AvatarSource      *userProfileSourceContext              `json:"avatar_source,omitempty"`
	UsernameSource    *userProfileSourceContext              `json:"username_source,omitempty"`
	DisplayNameSource *userProfileSourceContext              `json:"display_name_source,omitempty"`
	NicknameSource    *userProfileSourceContext              `json:"nickname_source,omitempty"`
	ProfileSources    map[string]*userProfileSourceContext   `json:"profile_sources,omitempty"`
	Identities        service.UserIdentitySummarySet         `json:"identities"`
	AuthBindings      map[string]service.UserIdentitySummary `json:"auth_bindings"`
	IdentityBindings  map[string]service.UserIdentitySummary `json:"identity_bindings"`
	EmailBound        bool                                   `json:"email_bound"`
	LinuxDoBound      bool                                   `json:"linuxdo_bound"`
	OIDCBound         bool                                   `json:"oidc_bound"`
	WeChatBound       bool                                   `json:"wechat_bound"`
	DingTalkBound     bool                                   `json:"dingtalk_bound"`
REDACTED

type userProfileSourceContext struct {
	Provider string `json:"provider,omitempty"`
	Source   string `json:"source,omitempty"`
REDACTED

// GetProfile handles getting user profile
// GET /api/v1/users/me
func (h *UserHandler) GetProfile(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	userData, err := h.userService.GetProfile(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	profileResp, err := h.buildUserProfileResponse(c.Request.Context(), subject.UserID, userData)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, profileResp)
REDACTED

// ChangePassword handles changing user password
// POST /api/v1/users/me/password
func (h *UserHandler) ChangePassword(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	svcReq := service.ChangePasswordRequest{
		CurrentPassword: req.OldPassword,
		NewPassword:     req.NewPassword,
REDACTED
	err := h.userService.ChangePassword(c.Request.Context(), subject.UserID, svcReq)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{"message": "Password changed successfully"REDACTED)
REDACTED

// UpdateProfile handles updating user profile
// PUT /api/v1/users/me
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	svcReq := service.UpdateProfileRequest{
		Username:               req.Username,
		AvatarURL:              req.AvatarURL,
		BalanceNotifyEnabled:   req.BalanceNotifyEnabled,
		BalanceNotifyThreshold: req.BalanceNotifyThreshold,
REDACTED
	updatedUser, err := h.userService.UpdateProfile(c.Request.Context(), subject.UserID, svcReq)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	profileResp, err := h.buildUserProfileResponse(c.Request.Context(), subject.UserID, updatedUser)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, profileResp)
REDACTED

// GetAffiliate returns the current user's affiliate details.
// GET /api/v1/user/aff
func (h *UserHandler) GetAffiliate(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	detail, err := h.affiliateService.GetAffiliateDetail(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, detail)
REDACTED

// TransferAffiliateQuota transfers all available affiliate quota into current balance.
// POST /api/v1/user/aff/transfer
func (h *UserHandler) TransferAffiliateQuota(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	transferred, balance, err := h.affiliateService.TransferAffiliateQuota(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{
		"transferred_quota": transferred,
		"balance":           balance,
REDACTED)
REDACTED

type StartIdentityBindingRequest struct {
	Provider   string `json:"provider" binding:"required"`
	RedirectTo string `json:"redirect_to"`
REDACTED

type BindEmailIdentityRequest struct {
	Email      string `json:"email" binding:"required,email"`
	VerifyCode string `json:"verify_code" binding:"required"`
	Password   string `json:"password" binding:"required"`
REDACTED

type SendEmailBindingCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
REDACTED

// StartIdentityBinding returns the backend authorize URL for starting a third-party identity bind flow.
// POST /api/v1/user/auth-identities/bind/start
func (h *UserHandler) StartIdentityBinding(c *gin.Context) {
	if _, ok := middleware2.GetAuthSubjectFromContext(c); !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	var req StartIdentityBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	result, err := h.userService.PrepareIdentityBindingStart(c.Request.Context(), service.StartUserIdentityBindingRequest{
		Provider:   req.Provider,
		RedirectTo: req.RedirectTo,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, result)
REDACTED

// BindEmailIdentity verifies and binds a local email identity for the current user.
// POST /api/v1/user/account-bindings/email
func (h *UserHandler) BindEmailIdentity(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED
	if h.authService == nil {
		response.InternalError(c, "Auth service not configured")
		return
REDACTED

	var req BindEmailIdentityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	updatedUser, err := h.authService.BindEmailIdentity(
		c.Request.Context(),
		subject.UserID,
		req.Email,
		req.VerifyCode,
		req.Password,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	profileResp, err := h.buildUserProfileResponse(c.Request.Context(), subject.UserID, updatedUser)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, profileResp)
REDACTED

// UnbindIdentity removes a third-party sign-in provider from the current user.
// DELETE /api/v1/user/account-bindings/:provider
func (h *UserHandler) UnbindIdentity(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	updatedUser, unbound, err := h.userService.UnbindUserAuthProviderWithResult(
		c.Request.Context(),
		subject.UserID,
		c.Param("provider"),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if unbound && h.authService != nil {
		if err := h.authService.RevokeAllUserTokens(c.Request.Context(), subject.UserID); err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
REDACTED

	profileResp, err := h.buildUserProfileResponse(c.Request.Context(), subject.UserID, updatedUser)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, profileResp)
REDACTED

// SendEmailBindingCode sends a verification code for the current user's email binding flow.
// POST /api/v1/user/account-bindings/email/send-code
func (h *UserHandler) SendEmailBindingCode(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED
	if h.authService == nil {
		response.InternalError(c, "Auth service not configured")
		return
REDACTED

	var req SendEmailBindingCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if err := h.authService.SendEmailIdentityBindCode(c.Request.Context(), subject.UserID, req.Email, c.GetHeader("Accept-Language")); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{"message": "Verification code sent successfully"REDACTED)
REDACTED

// SendNotifyEmailCodeRequest represents the request to send notify email verification code
type SendNotifyEmailCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
REDACTED

// SendNotifyEmailCode sends verification code to extra notification email
// POST /api/v1/user/notify-email/send-code
func (h *UserHandler) SendNotifyEmailCode(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	var req SendNotifyEmailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	err := h.userService.SendNotifyEmailCode(c.Request.Context(), subject.UserID, req.Email, h.emailService, h.emailCache, c.GetHeader("Accept-Language"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{"message": "Verification code sent successfully"REDACTED)
REDACTED

// VerifyNotifyEmailRequest represents the request to verify and add notify email
type VerifyNotifyEmailRequest struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required,len=6"`
REDACTED

// VerifyNotifyEmail verifies code and adds email to notification list
// POST /api/v1/user/notify-email/verify
func (h *UserHandler) VerifyNotifyEmail(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	var req VerifyNotifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	err := h.userService.VerifyAndAddNotifyEmail(c.Request.Context(), subject.UserID, req.Email, req.Code, h.emailCache)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	// Return updated user
	updatedUser, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	profileResp, err := h.buildUserProfileResponse(c.Request.Context(), subject.UserID, updatedUser)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, profileResp)
REDACTED

// RemoveNotifyEmailRequest represents the request to remove a notify email
type RemoveNotifyEmailRequest struct {
	Email string `json:"email" binding:"required,email"`
REDACTED

// RemoveNotifyEmail removes email from notification list
// DELETE /api/v1/user/notify-email
func (h *UserHandler) RemoveNotifyEmail(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	var req RemoveNotifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	err := h.userService.RemoveNotifyEmail(c.Request.Context(), subject.UserID, req.Email)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	// Return updated user
	updatedUser, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	profileResp, err := h.buildUserProfileResponse(c.Request.Context(), subject.UserID, updatedUser)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, profileResp)
REDACTED

// ToggleNotifyEmailRequest represents the request to toggle a notify email's disabled state
type ToggleNotifyEmailRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Disabled bool   `json:"disabled"`
REDACTED

// ToggleNotifyEmail toggles the disabled state of a notification email
// PUT /api/v1/user/notify-email/toggle
func (h *UserHandler) ToggleNotifyEmail(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	var req ToggleNotifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	err := h.userService.ToggleNotifyEmail(c.Request.Context(), subject.UserID, req.Email, req.Disabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	updatedUser, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	profileResp, err := h.buildUserProfileResponse(c.Request.Context(), subject.UserID, updatedUser)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, profileResp)
REDACTED

func (h *UserHandler) buildUserProfileResponse(ctx context.Context, userID int64, user *service.User) (userProfileResponse, error) {
	identities, err := h.userService.GetProfileIdentitySummaries(ctx, userID, user)
	if err != nil {
		return userProfileResponse{REDACTED, err
REDACTED
	return userProfileResponseFromService(user, identities), nil
REDACTED

func userProfileResponseFromService(user *service.User, identities service.UserIdentitySummarySet) userProfileResponse {
	base := dto.UserFromService(user)
	if base == nil {
		return userProfileResponse{REDACTED
REDACTED
	bindings := userProfileBindingMap(identities)
	profileSources, avatarSource, usernameSource := inferUserProfileSources(user, identities)
	return userProfileResponse{
		User:              *base,
		AvatarURL:         user.AvatarURL,
		AvatarSource:      avatarSource,
		UsernameSource:    usernameSource,
		DisplayNameSource: usernameSource,
		NicknameSource:    usernameSource,
		ProfileSources:    profileSources,
		Identities:        identities,
		AuthBindings:      bindings,
		IdentityBindings:  bindings,
		EmailBound:        identities.Email.Bound,
		LinuxDoBound:      identities.LinuxDo.Bound,
		OIDCBound:         identities.OIDC.Bound,
		WeChatBound:       identities.WeChat.Bound,
		DingTalkBound:     identities.DingTalk.Bound,
REDACTED
REDACTED

func userProfileBindingMap(identities service.UserIdentitySummarySet) map[string]service.UserIdentitySummary {
	return map[string]service.UserIdentitySummary{
		"email":    identities.Email,
		"linuxdo":  identities.LinuxDo,
		"oidc":     identities.OIDC,
		"wechat":   identities.WeChat,
		"dingtalk": identities.DingTalk,
REDACTED
REDACTED

func inferUserProfileSources(user *service.User, identities service.UserIdentitySummarySet) (
	map[string]*userProfileSourceContext,
	*userProfileSourceContext,
	*userProfileSourceContext,
) {
	if user == nil {
		return nil, nil, nil
REDACTED

	thirdParty := thirdPartyIdentityProviders(identities)
	var avatarSource *userProfileSourceContext
	avatarValue := strings.TrimSpace(user.AvatarURL)
	for _, summary := range thirdParty {
		if avatarValue != "" && avatarValue == strings.TrimSpace(summary.AvatarURL) {
			avatarSource = buildUserProfileSourceContext(summary.Provider)
			break
	REDACTED
REDACTED

	usernameValue := strings.TrimSpace(user.Username)
	var usernameSource *userProfileSourceContext
	for _, summary := range thirdParty {
		if usernameValue != "" && usernameValue == strings.TrimSpace(summary.DisplayName) {
			usernameSource = buildUserProfileSourceContext(summary.Provider)
			break
	REDACTED
REDACTED

	profileSources := map[string]*userProfileSourceContext{REDACTED
	if avatarSource != nil {
		profileSources["avatar"] = avatarSource
REDACTED
	if usernameSource != nil {
		profileSources["username"] = usernameSource
		profileSources["display_name"] = usernameSource
		profileSources["nickname"] = usernameSource
REDACTED
	if len(profileSources) == 0 {
		return nil, avatarSource, usernameSource
REDACTED
	return profileSources, avatarSource, usernameSource
REDACTED

func thirdPartyIdentityProviders(identities service.UserIdentitySummarySet) []service.UserIdentitySummary {
	out := make([]service.UserIdentitySummary, 0, 3)
	for _, summary := range []service.UserIdentitySummary{identities.LinuxDo, identities.OIDC, identities.WeChat, identities.DingTalkREDACTED {
		if summary.Bound {
			out = append(out, summary)
	REDACTED
REDACTED
	return out
REDACTED

func buildUserProfileSourceContext(provider string) *userProfileSourceContext {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil
REDACTED
	return &userProfileSourceContext{
		Provider: provider,
		Source:   provider,
REDACTED
REDACTED

package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UserHandler handles user-related requests
type UserHandler struct {
	userService  *service.UserService
	emailService *service.EmailService
	emailCache   service.EmailCache
REDACTED

// NewUserHandler creates a new UserHandler
func NewUserHandler(userService *service.UserService, emailService *service.EmailService, emailCache service.EmailCache) *UserHandler {
	return &UserHandler{
		userService:  userService,
		emailService: emailService,
		emailCache:   emailCache,
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
	BalanceNotifyEnabled   *bool    `json:"balance_notify_enabled"`
	BalanceNotifyThreshold *float64 `json:"balance_notify_threshold"`
REDACTED

// GetProfile handles getting user profile
// GET /api/v1/users/me
func (h *UserHandler) GetProfile(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	userData, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.UserFromService(userData))
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
		BalanceNotifyEnabled:   req.BalanceNotifyEnabled,
		BalanceNotifyThreshold: req.BalanceNotifyThreshold,
REDACTED
	updatedUser, err := h.userService.UpdateProfile(c.Request.Context(), subject.UserID, svcReq)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.UserFromService(updatedUser))
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

	err := h.userService.SendNotifyEmailCode(c.Request.Context(), subject.UserID, req.Email, h.emailService, h.emailCache)
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

	response.Success(c, dto.UserFromService(updatedUser))
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

	response.Success(c, gin.H{"message": "Email removed successfully"REDACTED)
REDACTED

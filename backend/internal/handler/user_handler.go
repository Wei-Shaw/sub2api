package handler

import (
	"sub2api/internal/model"
	"sub2api/internal/pkg/response"
	"sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UserHandler handles user-related requests
type UserHandler struct {
	userService *service.UserService
REDACTED

// NewUserHandler creates a new UserHandler
func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
REDACTED
REDACTED

// ChangePasswordRequest represents the change password request payload
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
REDACTED

// UpdateProfileRequest represents the update profile request payload
type UpdateProfileRequest struct {
	Username *string `json:"username"`
	Wechat   *string `json:"wechat"`
REDACTED

// GetProfile handles getting user profile
// GET /api/v1/users/me
func (h *UserHandler) GetProfile(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	user, ok := userValue.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user context")
		return
REDACTED

	userData, err := h.userService.GetByID(c.Request.Context(), user.ID)
	if err != nil {
		response.InternalError(c, "Failed to get user profile: "+err.Error())
		return
REDACTED

	// 清空notes字段，普通用户不应看到备注
	userData.Notes = ""

	response.Success(c, userData)
REDACTED

// ChangePassword handles changing user password
// POST /api/v1/users/me/password
func (h *UserHandler) ChangePassword(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	user, ok := userValue.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user context")
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
	err := h.userService.ChangePassword(c.Request.Context(), user.ID, svcReq)
	if err != nil {
		response.BadRequest(c, "Failed to change password: "+err.Error())
		return
REDACTED

	response.Success(c, gin.H{"message": "Password changed successfully"REDACTED)
REDACTED

// UpdateProfile handles updating user profile
// PUT /api/v1/users/me
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	user, ok := userValue.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user context")
		return
REDACTED

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	svcReq := service.UpdateProfileRequest{
		Username: req.Username,
		Wechat:   req.Wechat,
REDACTED
	updatedUser, err := h.userService.UpdateProfile(c.Request.Context(), user.ID, svcReq)
	if err != nil {
		response.BadRequest(c, "Failed to update profile: "+err.Error())
		return
REDACTED

	// 清空notes字段，普通用户不应看到备注
	updatedUser.Notes = ""

	response.Success(c, updatedUser)
REDACTED

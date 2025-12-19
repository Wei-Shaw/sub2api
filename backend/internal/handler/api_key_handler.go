package handler

import (
	"strconv"

	"sub2api/internal/model"
	"sub2api/internal/pkg/pagination"
	"sub2api/internal/pkg/response"
	"sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// APIKeyHandler handles API key-related requests
type APIKeyHandler struct {
	apiKeyService *service.ApiKeyService
REDACTED

// NewAPIKeyHandler creates a new APIKeyHandler
func NewAPIKeyHandler(apiKeyService *service.ApiKeyService) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyService: apiKeyService,
REDACTED
REDACTED

// CreateAPIKeyRequest represents the create API key request payload
type CreateAPIKeyRequest struct {
	Name      string  `json:"name" binding:"required"`
	GroupID   *int64  `json:"group_id"`   // nullable
	CustomKey *string `json:"custom_key"` // 可选的自定义key
REDACTED

// UpdateAPIKeyRequest represents the update API key request payload
type UpdateAPIKeyRequest struct {
	Name    string `json:"name"`
	GroupID *int64 `json:"group_id"`
	Status  string `json:"status" binding:"omitempty,oneof=active inactive"`
REDACTED

// List handles listing user's API keys with pagination
// GET /api/v1/api-keys
func (h *APIKeyHandler) List(c *gin.Context) {
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

	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED

	keys, result, err := h.apiKeyService.List(c.Request.Context(), user.ID, params)
	if err != nil {
		response.InternalError(c, "Failed to list API keys: "+err.Error())
		return
REDACTED

	response.Paginated(c, keys, result.Total, page, pageSize)
REDACTED

// GetByID handles getting a single API key
// GET /api/v1/api-keys/:id
func (h *APIKeyHandler) GetByID(c *gin.Context) {
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

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid key ID")
		return
REDACTED

	key, err := h.apiKeyService.GetByID(c.Request.Context(), keyID)
	if err != nil {
		response.NotFound(c, "API key not found")
		return
REDACTED

	// 验证所有权
	if key.UserID != user.ID {
		response.Forbidden(c, "Not authorized to access this key")
		return
REDACTED

	response.Success(c, key)
REDACTED

// Create handles creating a new API key
// POST /api/v1/api-keys
func (h *APIKeyHandler) Create(c *gin.Context) {
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

	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	svcReq := service.CreateApiKeyRequest{
		Name:      req.Name,
		GroupID:   req.GroupID,
		CustomKey: req.CustomKey,
REDACTED
	key, err := h.apiKeyService.Create(c.Request.Context(), user.ID, svcReq)
	if err != nil {
		response.InternalError(c, "Failed to create API key: "+err.Error())
		return
REDACTED

	response.Success(c, key)
REDACTED

// Update handles updating an API key
// PUT /api/v1/api-keys/:id
func (h *APIKeyHandler) Update(c *gin.Context) {
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

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid key ID")
		return
REDACTED

	var req UpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	svcReq := service.UpdateApiKeyRequest{REDACTED
	if req.Name != "" {
		svcReq.Name = &req.Name
REDACTED
	svcReq.GroupID = req.GroupID
	if req.Status != "" {
		svcReq.Status = &req.Status
REDACTED

	key, err := h.apiKeyService.Update(c.Request.Context(), keyID, user.ID, svcReq)
	if err != nil {
		response.InternalError(c, "Failed to update API key: "+err.Error())
		return
REDACTED

	response.Success(c, key)
REDACTED

// Delete handles deleting an API key
// DELETE /api/v1/api-keys/:id
func (h *APIKeyHandler) Delete(c *gin.Context) {
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

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid key ID")
		return
REDACTED

	err = h.apiKeyService.Delete(c.Request.Context(), keyID, user.ID)
	if err != nil {
		response.InternalError(c, "Failed to delete API key: "+err.Error())
		return
REDACTED

	response.Success(c, gin.H{"message": "API key deleted successfully"REDACTED)
REDACTED

// GetAvailableGroups 获取用户可以绑定的分组列表
// GET /api/v1/groups/available
func (h *APIKeyHandler) GetAvailableGroups(c *gin.Context) {
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

	groups, err := h.apiKeyService.GetAvailableGroups(c.Request.Context(), user.ID)
	if err != nil {
		response.InternalError(c, "Failed to get available groups: "+err.Error())
		return
REDACTED

	response.Success(c, groups)
REDACTED

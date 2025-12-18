package admin

import (
	"strconv"

	"sub2api/internal/pkg/response"
	"sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OAuthHandler handles OAuth-related operations for accounts
type OAuthHandler struct {
	oauthService *service.OAuthService
	adminService service.AdminService
REDACTED

// NewOAuthHandler creates a new OAuth handler
func NewOAuthHandler(oauthService *service.OAuthService, adminService service.AdminService) *OAuthHandler {
	return &OAuthHandler{
		oauthService: oauthService,
		adminService: adminService,
REDACTED
REDACTED

// AccountHandler handles admin account management
type AccountHandler struct {
	adminService        service.AdminService
	oauthService        *service.OAuthService
	rateLimitService    *service.RateLimitService
	accountUsageService *service.AccountUsageService
	accountTestService  *service.AccountTestService
REDACTED

// NewAccountHandler creates a new admin account handler
func NewAccountHandler(adminService service.AdminService, oauthService *service.OAuthService, rateLimitService *service.RateLimitService, accountUsageService *service.AccountUsageService, accountTestService *service.AccountTestService) *AccountHandler {
	return &AccountHandler{
		adminService:        adminService,
		oauthService:        oauthService,
		rateLimitService:    rateLimitService,
		accountUsageService: accountUsageService,
		accountTestService:  accountTestService,
REDACTED
REDACTED

// CreateAccountRequest represents create account request
type CreateAccountRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Platform    string                 `json:"platform" binding:"required"`
	Type        string                 `json:"type" binding:"required,oneof=oauth setup-token apikey"`
	Credentials map[string]interface{REDACTED `json:"credentials" binding:"required"`
	Extra       map[string]interface{REDACTED `json:"extra"`
	ProxyID     *int64                 `json:"proxy_id"`
	Concurrency int                    `json:"concurrency"`
	Priority    int                    `json:"priority"`
	GroupIDs    []int64                `json:"group_ids"`
REDACTED

// UpdateAccountRequest represents update account request
// 使用指针类型来区分"未提供"和"设置为0"
type UpdateAccountRequest struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type" binding:"omitempty,oneof=oauth setup-token apikey"`
	Credentials map[string]interface{REDACTED `json:"credentials"`
	Extra       map[string]interface{REDACTED `json:"extra"`
	ProxyID     *int64                 `json:"proxy_id"`
	Concurrency *int                   `json:"concurrency"`
	Priority    *int                   `json:"priority"`
	Status      string                 `json:"status" binding:"omitempty,oneof=active inactive"`
	GroupIDs    *[]int64               `json:"group_ids"`
REDACTED

// List handles listing all accounts with pagination
// GET /api/v1/admin/accounts
func (h *AccountHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	platform := c.Query("platform")
	accountType := c.Query("type")
	status := c.Query("status")
	search := c.Query("search")

	accounts, total, err := h.adminService.ListAccounts(c.Request.Context(), page, pageSize, platform, accountType, status, search)
	if err != nil {
		response.InternalError(c, "Failed to list accounts: "+err.Error())
		return
REDACTED

	response.Paginated(c, accounts, total, page, pageSize)
REDACTED

// GetByID handles getting an account by ID
// GET /api/v1/admin/accounts/:id
func (h *AccountHandler) GetByID(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.NotFound(c, "Account not found")
		return
REDACTED

	response.Success(c, account)
REDACTED

// Create handles creating a new account
// POST /api/v1/admin/accounts
func (h *AccountHandler) Create(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	account, err := h.adminService.CreateAccount(c.Request.Context(), &service.CreateAccountInput{
		Name:        req.Name,
		Platform:    req.Platform,
		Type:        req.Type,
		Credentials: req.Credentials,
		Extra:       req.Extra,
		ProxyID:     req.ProxyID,
		Concurrency: req.Concurrency,
		Priority:    req.Priority,
		GroupIDs:    req.GroupIDs,
REDACTED)
	if err != nil {
		response.BadRequest(c, "Failed to create account: "+err.Error())
		return
REDACTED

	response.Success(c, account)
REDACTED

// Update handles updating an account
// PUT /api/v1/admin/accounts/:id
func (h *AccountHandler) Update(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED

	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	account, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{
		Name:        req.Name,
		Type:        req.Type,
		Credentials: req.Credentials,
		Extra:       req.Extra,
		ProxyID:     req.ProxyID,
		Concurrency: req.Concurrency, // 指针类型，nil 表示未提供
		Priority:    req.Priority,    // 指针类型，nil 表示未提供
		Status:      req.Status,
		GroupIDs:    req.GroupIDs,
REDACTED)
	if err != nil {
		response.InternalError(c, "Failed to update account: "+err.Error())
		return
REDACTED

	response.Success(c, account)
REDACTED

// Delete handles deleting an account
// DELETE /api/v1/admin/accounts/:id
func (h *AccountHandler) Delete(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED

	err = h.adminService.DeleteAccount(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, "Failed to delete account: "+err.Error())
		return
REDACTED

	response.Success(c, gin.H{"message": "Account deleted successfully"REDACTED)
REDACTED

// Test handles testing account connectivity with SSE streaming
// POST /api/v1/admin/accounts/:id/test
func (h *AccountHandler) Test(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED

	// Use AccountTestService to test the account with SSE streaming
	if err := h.accountTestService.TestAccountConnection(c, accountID); err != nil {
		// Error already sent via SSE, just log
		return
REDACTED
REDACTED

// Refresh handles refreshing account credentials
// POST /api/v1/admin/accounts/:id/refresh
func (h *AccountHandler) Refresh(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED

	// Get account
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.NotFound(c, "Account not found")
		return
REDACTED

	// Only refresh OAuth-based accounts (oauth and setup-token)
	if !account.IsOAuth() {
		response.BadRequest(c, "Cannot refresh non-OAuth account credentials")
		return
REDACTED

	// Use OAuth service to refresh token
	tokenInfo, err := h.oauthService.RefreshAccountToken(c.Request.Context(), account)
	if err != nil {
		response.InternalError(c, "Failed to refresh credentials: "+err.Error())
		return
REDACTED

	// Update account credentials
	newCredentials := map[string]interface{REDACTED{
		"access_token":  tokenInfo.AccessToken,
		"token_type":    tokenInfo.TokenType,
		"expires_in":    tokenInfo.ExpiresIn,
		"expires_at":    tokenInfo.ExpiresAt,
		"refresh_token": tokenInfo.RefreshToken,
		"scope":         tokenInfo.Scope,
REDACTED

	updatedAccount, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{
		Credentials: newCredentials,
REDACTED)
	if err != nil {
		response.InternalError(c, "Failed to update account credentials: "+err.Error())
		return
REDACTED

	response.Success(c, updatedAccount)
REDACTED

// GetStats handles getting account statistics
// GET /api/v1/admin/accounts/:id/stats
func (h *AccountHandler) GetStats(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED

	// Return mock data for now
	_ = accountID
	response.Success(c, gin.H{
		"total_requests":        0,
		"successful_requests":   0,
		"failed_requests":       0,
		"total_tokens":          0,
		"average_response_time": 0,
REDACTED)
REDACTED

// ClearError handles clearing account error
// POST /api/v1/admin/accounts/:id/clear-error
func (h *AccountHandler) ClearError(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED

	account, err := h.adminService.ClearAccountError(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, "Failed to clear error: "+err.Error())
		return
REDACTED

	response.Success(c, account)
REDACTED

// BatchCreate handles batch creating accounts
// POST /api/v1/admin/accounts/batch
func (h *AccountHandler) BatchCreate(c *gin.Context) {
	var req struct {
		Accounts []CreateAccountRequest `json:"accounts" binding:"required,min=1"`
REDACTED
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	// Return mock data for now
	response.Success(c, gin.H{
		"success": len(req.Accounts),
		"failed":  0,
		"results": []gin.H{REDACTED,
REDACTED)
REDACTED

// ========== OAuth Handlers ==========

// GenerateAuthURLRequest represents the request for generating auth URL
type GenerateAuthURLRequest struct {
	ProxyID *int64 `json:"proxy_id"`
REDACTED

// GenerateAuthURL generates OAuth authorization URL with full scope
// POST /api/v1/admin/accounts/generate-auth-url
func (h *OAuthHandler) GenerateAuthURL(c *gin.Context) {
	var req GenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body
		req = GenerateAuthURLRequest{REDACTED
REDACTED

	result, err := h.oauthService.GenerateAuthURL(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.InternalError(c, "Failed to generate auth URL: "+err.Error())
		return
REDACTED

	response.Success(c, result)
REDACTED

// GenerateSetupTokenURL generates OAuth authorization URL for setup token (inference only)
// POST /api/v1/admin/accounts/generate-setup-token-url
func (h *OAuthHandler) GenerateSetupTokenURL(c *gin.Context) {
	var req GenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body
		req = GenerateAuthURLRequest{REDACTED
REDACTED

	result, err := h.oauthService.GenerateSetupTokenURL(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.InternalError(c, "Failed to generate setup token URL: "+err.Error())
		return
REDACTED

	response.Success(c, result)
REDACTED

// ExchangeCodeRequest represents the request for exchanging auth code
type ExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	Code      string `json:"code" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
REDACTED

// ExchangeCode exchanges authorization code for tokens
// POST /api/v1/admin/accounts/exchange-code
func (h *OAuthHandler) ExchangeCode(c *gin.Context) {
	var req ExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	tokenInfo, err := h.oauthService.ExchangeCode(c.Request.Context(), &service.ExchangeCodeInput{
		SessionID: req.SessionID,
		Code:      req.Code,
		ProxyID:   req.ProxyID,
REDACTED)
	if err != nil {
		response.BadRequest(c, "Failed to exchange code: "+err.Error())
		return
REDACTED

	response.Success(c, tokenInfo)
REDACTED

// ExchangeSetupTokenCode exchanges authorization code for setup token
// POST /api/v1/admin/accounts/exchange-setup-token-code
func (h *OAuthHandler) ExchangeSetupTokenCode(c *gin.Context) {
	var req ExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	tokenInfo, err := h.oauthService.ExchangeCode(c.Request.Context(), &service.ExchangeCodeInput{
		SessionID: req.SessionID,
		Code:      req.Code,
		ProxyID:   req.ProxyID,
REDACTED)
	if err != nil {
		response.BadRequest(c, "Failed to exchange code: "+err.Error())
		return
REDACTED

	response.Success(c, tokenInfo)
REDACTED

// CookieAuthRequest represents the request for cookie-based authentication
type CookieAuthRequest struct {
	SessionKey string `json:"code" binding:"required"` // Using 'code' field as sessionKey (frontend sends it this way)
	ProxyID    *int64 `json:"proxy_id"`
REDACTED

// CookieAuth performs OAuth using sessionKey (cookie-based auto-auth)
// POST /api/v1/admin/accounts/cookie-auth
func (h *OAuthHandler) CookieAuth(c *gin.Context) {
	var req CookieAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	tokenInfo, err := h.oauthService.CookieAuth(c.Request.Context(), &service.CookieAuthInput{
		SessionKey: req.SessionKey,
		ProxyID:    req.ProxyID,
		Scope:      "full",
REDACTED)
	if err != nil {
		response.BadRequest(c, "Cookie auth failed: "+err.Error())
		return
REDACTED

	response.Success(c, tokenInfo)
REDACTED

// SetupTokenCookieAuth performs OAuth using sessionKey for setup token (inference only)
// POST /api/v1/admin/accounts/setup-token-cookie-auth
func (h *OAuthHandler) SetupTokenCookieAuth(c *gin.Context) {
	var req CookieAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	tokenInfo, err := h.oauthService.CookieAuth(c.Request.Context(), &service.CookieAuthInput{
		SessionKey: req.SessionKey,
		ProxyID:    req.ProxyID,
		Scope:      "inference",
REDACTED)
	if err != nil {
		response.BadRequest(c, "Cookie auth failed: "+err.Error())
		return
REDACTED

	response.Success(c, tokenInfo)
REDACTED

// GetUsage handles getting account usage information
// GET /api/v1/admin/accounts/:id/usage
func (h *AccountHandler) GetUsage(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED

	usage, err := h.accountUsageService.GetUsage(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, "Failed to get usage: "+err.Error())
		return
REDACTED

	response.Success(c, usage)
REDACTED

// ClearRateLimit handles clearing account rate limit status
// POST /api/v1/admin/accounts/:id/clear-rate-limit
func (h *AccountHandler) ClearRateLimit(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED

	err = h.rateLimitService.ClearRateLimit(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, "Failed to clear rate limit: "+err.Error())
		return
REDACTED

	response.Success(c, gin.H{"message": "Rate limit cleared successfully"REDACTED)
REDACTED

// GetTodayStats handles getting account today statistics
// GET /api/v1/admin/accounts/:id/today-stats
func (h *AccountHandler) GetTodayStats(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED

	stats, err := h.accountUsageService.GetTodayStats(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, "Failed to get today stats: "+err.Error())
		return
REDACTED

	response.Success(c, stats)
REDACTED

// SetSchedulableRequest represents the request body for setting schedulable status
type SetSchedulableRequest struct {
	Schedulable bool `json:"schedulable"`
REDACTED

// SetSchedulable handles toggling account schedulable status
// POST /api/v1/admin/accounts/:id/schedulable
func (h *AccountHandler) SetSchedulable(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED

	var req SetSchedulableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	account, err := h.adminService.SetAccountSchedulable(c.Request.Context(), accountID, req.Schedulable)
	if err != nil {
		response.InternalError(c, "Failed to update schedulable status: "+err.Error())
		return
REDACTED

	response.Success(c, account)
REDACTED

package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OAuthHandler handles OAuth-related operations for accounts
type OAuthHandler struct {
	oauthService *service.OAuthService
REDACTED

// NewOAuthHandler creates a new OAuth handler
func NewOAuthHandler(oauthService *service.OAuthService) *OAuthHandler {
	return &OAuthHandler{
		oauthService: oauthService,
REDACTED
REDACTED

// AccountHandler handles admin account management
type AccountHandler struct {
	adminService        service.AdminService
	oauthService        *service.OAuthService
	openaiOAuthService  *service.OpenAIOAuthService
	rateLimitService    *service.RateLimitService
	accountUsageService *service.AccountUsageService
	accountTestService  *service.AccountTestService
	concurrencyService  *service.ConcurrencyService
	crsSyncService      *service.CRSSyncService
REDACTED

// NewAccountHandler creates a new admin account handler
func NewAccountHandler(
	adminService service.AdminService,
	oauthService *service.OAuthService,
	openaiOAuthService *service.OpenAIOAuthService,
	rateLimitService *service.RateLimitService,
	accountUsageService *service.AccountUsageService,
	accountTestService *service.AccountTestService,
	concurrencyService *service.ConcurrencyService,
	crsSyncService *service.CRSSyncService,
) *AccountHandler {
	return &AccountHandler{
		adminService:        adminService,
		oauthService:        oauthService,
		openaiOAuthService:  openaiOAuthService,
		rateLimitService:    rateLimitService,
		accountUsageService: accountUsageService,
		accountTestService:  accountTestService,
		concurrencyService:  concurrencyService,
		crsSyncService:      crsSyncService,
REDACTED
REDACTED

// CreateAccountRequest represents create account request
type CreateAccountRequest struct {
	Name        string         `json:"name" binding:"required"`
	Platform    string         `json:"platform" binding:"required"`
	Type        string         `json:"type" binding:"required,oneof=oauth setup-token apikey"`
	Credentials map[string]any `json:"credentials" binding:"required"`
	Extra       map[string]any `json:"extra"`
	ProxyID     *int64         `json:"proxy_id"`
	Concurrency int            `json:"concurrency"`
	Priority    int            `json:"priority"`
	GroupIDs    []int64        `json:"group_ids"`
REDACTED

// UpdateAccountRequest represents update account request
// 使用指针类型来区分"未提供"和"设置为0"
type UpdateAccountRequest struct {
	Name        string         `json:"name"`
	Type        string         `json:"type" binding:"omitempty,oneof=oauth setup-token apikey"`
	Credentials map[string]any `json:"credentials"`
	Extra       map[string]any `json:"extra"`
	ProxyID     *int64         `json:"proxy_id"`
	Concurrency *int           `json:"concurrency"`
	Priority    *int           `json:"priority"`
	Status      string         `json:"status" binding:"omitempty,oneof=active inactive"`
	GroupIDs    *[]int64       `json:"group_ids"`
REDACTED

// AccountWithConcurrency extends Account with real-time concurrency info
type AccountWithConcurrency struct {
	*model.Account
	CurrentConcurrency int `json:"current_concurrency"`
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

	// Get current concurrency counts for all accounts
	accountIDs := make([]int64, len(accounts))
	for i, acc := range accounts {
		accountIDs[i] = acc.ID
REDACTED

	concurrencyCounts, err := h.concurrencyService.GetAccountConcurrencyBatch(c.Request.Context(), accountIDs)
	if err != nil {
		// Log error but don't fail the request, just use 0 for all
		concurrencyCounts = make(map[int64]int)
REDACTED

	// Build response with concurrency info
	result := make([]AccountWithConcurrency, len(accounts))
	for i := range accounts {
		result[i] = AccountWithConcurrency{
			Account:            &accounts[i],
			CurrentConcurrency: concurrencyCounts[accounts[i].ID],
	REDACTED
REDACTED

	response.Paginated(c, result, total, page, pageSize)
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

// TestAccountRequest represents the request body for testing an account
type TestAccountRequest struct {
	ModelID string `json:"model_id"`
REDACTED

type SyncFromCRSRequest struct {
	BaseURL     string `json:"base_url" binding:"required"`
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	SyncProxies *bool  `json:"sync_proxies"`
REDACTED

// Test handles testing account connectivity with SSE streaming
// POST /api/v1/admin/accounts/:id/test
func (h *AccountHandler) Test(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED

	var req TestAccountRequest
	// Allow empty body, model_id is optional
	_ = c.ShouldBindJSON(&req)

	// Use AccountTestService to test the account with SSE streaming
	if err := h.accountTestService.TestAccountConnection(c, accountID, req.ModelID); err != nil {
		// Error already sent via SSE, just log
		return
REDACTED
REDACTED

// SyncFromCRS handles syncing accounts from claude-relay-service (CRS)
// POST /api/v1/admin/accounts/sync/crs
func (h *AccountHandler) SyncFromCRS(c *gin.Context) {
	var req SyncFromCRSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	// Default to syncing proxies (can be disabled by explicitly setting false)
	syncProxies := true
	if req.SyncProxies != nil {
		syncProxies = *req.SyncProxies
REDACTED

	result, err := h.crsSyncService.SyncFromCRS(c.Request.Context(), service.SyncFromCRSInput{
		BaseURL:     req.BaseURL,
		Username:    req.Username,
		Password:    req.Password,
		SyncProxies: syncProxies,
REDACTED)
	if err != nil {
		response.BadRequest(c, "Sync failed: "+err.Error())
		return
REDACTED

	response.Success(c, result)
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

	var newCredentials map[string]any

	if account.IsOpenAI() {
		// Use OpenAI OAuth service to refresh token
		tokenInfo, err := h.openaiOAuthService.RefreshAccountToken(c.Request.Context(), account)
		if err != nil {
			response.InternalError(c, "Failed to refresh credentials: "+err.Error())
			return
	REDACTED

		// Build new credentials from token info
		newCredentials = h.openaiOAuthService.BuildAccountCredentials(tokenInfo)

		// Preserve non-token settings from existing credentials
		for k, v := range account.Credentials {
			if _, exists := newCredentials[k]; !exists {
				newCredentials[k] = v
		REDACTED
	REDACTED
REDACTED else {
		// Use Anthropic/Claude OAuth service to refresh token
		tokenInfo, err := h.oauthService.RefreshAccountToken(c.Request.Context(), account)
		if err != nil {
			response.InternalError(c, "Failed to refresh credentials: "+err.Error())
			return
	REDACTED

		// Copy existing credentials to preserve non-token settings (e.g., intercept_warmup_requests)
		newCredentials = make(map[string]any)
		for k, v := range account.Credentials {
			newCredentials[k] = v
	REDACTED

		// Update token-related fields
		newCredentials["access_token"] = tokenInfo.AccessToken
		newCredentials["token_type"] = tokenInfo.TokenType
		newCredentials["expires_in"] = tokenInfo.ExpiresIn
		newCredentials["expires_at"] = tokenInfo.ExpiresAt
		newCredentials["refresh_token"] = tokenInfo.RefreshToken
		newCredentials["scope"] = tokenInfo.Scope
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

	// Parse days parameter (default 30)
	days := 30
	if daysStr := c.Query("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 90 {
			days = d
	REDACTED
REDACTED

	// Calculate time range
	now := timezone.Now()
	endTime := timezone.StartOfDay(now.AddDate(0, 0, 1))
	startTime := timezone.StartOfDay(now.AddDate(0, 0, -days+1))

	stats, err := h.accountUsageService.GetAccountUsageStats(c.Request.Context(), accountID, startTime, endTime)
	if err != nil {
		response.InternalError(c, "Failed to get account stats: "+err.Error())
		return
REDACTED

	response.Success(c, stats)
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

// GetAvailableModels handles getting available models for an account
// GET /api/v1/admin/accounts/:id/models
func (h *AccountHandler) GetAvailableModels(c *gin.Context) {
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

	// Handle OpenAI accounts
	if account.IsOpenAI() {
		// For OAuth accounts: return default OpenAI models
		if account.IsOAuth() {
			response.Success(c, openai.DefaultModels)
			return
	REDACTED

		// For API Key accounts: check model_mapping
		mapping := account.GetModelMapping()
		if len(mapping) == 0 {
			response.Success(c, openai.DefaultModels)
			return
	REDACTED

		// Return mapped models
		var models []openai.Model
		for requestedModel := range mapping {
			var found bool
			for _, dm := range openai.DefaultModels {
				if dm.ID == requestedModel {
					models = append(models, dm)
					found = true
					break
			REDACTED
		REDACTED
			if !found {
				models = append(models, openai.Model{
					ID:          requestedModel,
					Object:      "model",
					Type:        "model",
					DisplayName: requestedModel,
			REDACTED)
		REDACTED
	REDACTED
		response.Success(c, models)
		return
REDACTED

	// Handle Claude/Anthropic accounts
	// For OAuth and Setup-Token accounts: return default models
	if account.IsOAuth() {
		response.Success(c, claude.DefaultModels)
		return
REDACTED

	// For API Key accounts: return models based on model_mapping
	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		// No mapping configured, return default models
		response.Success(c, claude.DefaultModels)
		return
REDACTED

	// Return mapped models (keys of the mapping are the available model IDs)
	var models []claude.Model
	for requestedModel := range mapping {
		// Try to find display info from default models
		var found bool
		for _, dm := range claude.DefaultModels {
			if dm.ID == requestedModel {
				models = append(models, dm)
				found = true
				break
		REDACTED
	REDACTED
		// If not found in defaults, create a basic entry
		if !found {
			models = append(models, claude.Model{
				ID:          requestedModel,
				Type:        "model",
				DisplayName: requestedModel,
				CreatedAt:   "",
		REDACTED)
	REDACTED
REDACTED

	response.Success(c, models)
REDACTED

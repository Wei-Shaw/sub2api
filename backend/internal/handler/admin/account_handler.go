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
}

// NewOAuthHandler creates a new OAuth handler
func NewOAuthHandler(oauthService *service.OAuthService) *OAuthHandler {
	return &OAuthHandler{
		oauthService: oauthService,
	}
}

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
}

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
	}
}

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
}

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
}

// BulkUpdateAccountsRequest represents the payload for bulk editing accounts
type BulkUpdateAccountsRequest struct {
	AccountIDs  []int64        `json:"account_ids" binding:"required,min=1"`
	Name        string         `json:"name"`
	ProxyID     *int64         `json:"proxy_id"`
	Concurrency *int           `json:"concurrency"`
	Priority    *int           `json:"priority"`
	Status      string         `json:"status" binding:"omitempty,oneof=active inactive error"`
	GroupIDs    *[]int64       `json:"group_ids"`
	Credentials map[string]any `json:"credentials"`
	Extra       map[string]any `json:"extra"`
}

// AccountWithConcurrency extends Account with real-time concurrency info
type AccountWithConcurrency struct {
	*model.Account
	CurrentConcurrency int `json:"current_concurrency"`
}

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
	}

	// Get current concurrency counts for all accounts
	accountIDs := make([]int64, len(accounts))
	for i, acc := range accounts {
		accountIDs[i] = acc.ID
	}

	concurrencyCounts, err := h.concurrencyService.GetAccountConcurrencyBatch(c.Request.Context(), accountIDs)
	if err != nil {
		// Log error but don't fail the request, just use 0 for all
		concurrencyCounts = make(map[int64]int)
	}

	// Build response with concurrency info
	result := make([]AccountWithConcurrency, len(accounts))
	for i := range accounts {
		result[i] = AccountWithConcurrency{
			Account:            &accounts[i],
			CurrentConcurrency: concurrencyCounts[accounts[i].ID],
		}
	}

	response.Paginated(c, result, total, page, pageSize)
}

// GetByID handles getting an account by ID
// GET /api/v1/admin/accounts/:id
func (h *AccountHandler) GetByID(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.NotFound(c, "Account not found")
		return
	}

	response.Success(c, account)
}

// Create handles creating a new account
// POST /api/v1/admin/accounts
func (h *AccountHandler) Create(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

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
	})
	if err != nil {
		response.BadRequest(c, "Failed to create account: "+err.Error())
		return
	}

	response.Success(c, account)
}

// Update handles updating an account
// PUT /api/v1/admin/accounts/:id
func (h *AccountHandler) Update(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

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
	})
	if err != nil {
		response.InternalError(c, "Failed to update account: "+err.Error())
		return
	}

	response.Success(c, account)
}

// Delete handles deleting an account
// DELETE /api/v1/admin/accounts/:id
func (h *AccountHandler) Delete(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	err = h.adminService.DeleteAccount(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, "Failed to delete account: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Account deleted successfully"})
}

// TestAccountRequest represents the request body for testing an account
type TestAccountRequest struct {
	ModelID string `json:"model_id"`
}

type SyncFromCRSRequest struct {
	BaseURL     string `json:"base_url" binding:"required"`
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	SyncProxies *bool  `json:"sync_proxies"`
}

// Test handles testing account connectivity with SSE streaming
// POST /api/v1/admin/accounts/:id/test
func (h *AccountHandler) Test(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	var req TestAccountRequest
	// Allow empty body, model_id is optional
	_ = c.ShouldBindJSON(&req)

	// Use AccountTestService to test the account with SSE streaming
	if err := h.accountTestService.TestAccountConnection(c, accountID, req.ModelID); err != nil {
		// Error already sent via SSE, just log
		return
	}
}

// SyncFromCRS handles syncing accounts from claude-relay-service (CRS)
// POST /api/v1/admin/accounts/sync/crs
func (h *AccountHandler) SyncFromCRS(c *gin.Context) {
	var req SyncFromCRSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Default to syncing proxies (can be disabled by explicitly setting false)
	syncProxies := true
	if req.SyncProxies != nil {
		syncProxies = *req.SyncProxies
	}

	result, err := h.crsSyncService.SyncFromCRS(c.Request.Context(), service.SyncFromCRSInput{
		BaseURL:     req.BaseURL,
		Username:    req.Username,
		Password:    req.Password,
		SyncProxies: syncProxies,
	})
	if err != nil {
		response.BadRequest(c, "Sync failed: "+err.Error())
		return
	}

	response.Success(c, result)
}

// Refresh handles refreshing account credentials
// POST /api/v1/admin/accounts/:id/refresh
func (h *AccountHandler) Refresh(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	// Get account
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.NotFound(c, "Account not found")
		return
	}

	// Only refresh OAuth-based accounts (oauth and setup-token)
	if !account.IsOAuth() {
		response.BadRequest(c, "Cannot refresh non-OAuth account credentials")
		return
	}

	var newCredentials map[string]any

	if account.IsOpenAI() {
		// Use OpenAI OAuth service to refresh token
		tokenInfo, err := h.openaiOAuthService.RefreshAccountToken(c.Request.Context(), account)
		if err != nil {
			response.InternalError(c, "Failed to refresh credentials: "+err.Error())
			return
		}

		// Build new credentials from token info
		newCredentials = h.openaiOAuthService.BuildAccountCredentials(tokenInfo)

		// Preserve non-token settings from existing credentials
		for k, v := range account.Credentials {
			if _, exists := newCredentials[k]; !exists {
				newCredentials[k] = v
			}
		}
	} else {
		// Use Anthropic/Claude OAuth service to refresh token
		tokenInfo, err := h.oauthService.RefreshAccountToken(c.Request.Context(), account)
		if err != nil {
			response.InternalError(c, "Failed to refresh credentials: "+err.Error())
			return
		}

		// Copy existing credentials to preserve non-token settings (e.g., intercept_warmup_requests)
		newCredentials = make(map[string]any)
		for k, v := range account.Credentials {
			newCredentials[k] = v
		}

		// Update token-related fields
		newCredentials["access_token"] = tokenInfo.AccessToken
		newCredentials["token_type"] = tokenInfo.TokenType
		newCredentials["expires_in"] = tokenInfo.ExpiresIn
		newCredentials["expires_at"] = tokenInfo.ExpiresAt
		newCredentials["refresh_token"] = tokenInfo.RefreshToken
		newCredentials["scope"] = tokenInfo.Scope
	}

	updatedAccount, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{
		Credentials: newCredentials,
	})
	if err != nil {
		response.InternalError(c, "Failed to update account credentials: "+err.Error())
		return
	}

	response.Success(c, updatedAccount)
}

// GetStats handles getting account statistics
// GET /api/v1/admin/accounts/:id/stats
func (h *AccountHandler) GetStats(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	// Parse days parameter (default 30)
	days := 30
	if daysStr := c.Query("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 90 {
			days = d
		}
	}

	// Calculate time range
	now := timezone.Now()
	endTime := timezone.StartOfDay(now.AddDate(0, 0, 1))
	startTime := timezone.StartOfDay(now.AddDate(0, 0, -days+1))

	stats, err := h.accountUsageService.GetAccountUsageStats(c.Request.Context(), accountID, startTime, endTime)
	if err != nil {
		response.InternalError(c, "Failed to get account stats: "+err.Error())
		return
	}

	response.Success(c, stats)
}

// ClearError handles clearing account error
// POST /api/v1/admin/accounts/:id/clear-error
func (h *AccountHandler) ClearError(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	account, err := h.adminService.ClearAccountError(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, "Failed to clear error: "+err.Error())
		return
	}

	response.Success(c, account)
}

// BatchCreate handles batch creating accounts
// POST /api/v1/admin/accounts/batch
func (h *AccountHandler) BatchCreate(c *gin.Context) {
	var req struct {
		Accounts []CreateAccountRequest `json:"accounts" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Return mock data for now
	response.Success(c, gin.H{
		"success": len(req.Accounts),
		"failed":  0,
		"results": []gin.H{},
	})
}

// BatchUpdateCredentialsRequest represents batch credentials update request
type BatchUpdateCredentialsRequest struct {
	AccountIDs []int64 `json:"account_ids" binding:"required,min=1"`
	Field      string  `json:"field" binding:"required,oneof=account_uuid org_uuid intercept_warmup_requests"`
	Value      any     `json:"value"`
}

// BatchUpdateCredentials handles batch updating credentials fields
// POST /api/v1/admin/accounts/batch-update-credentials
func (h *AccountHandler) BatchUpdateCredentials(c *gin.Context) {
	var req BatchUpdateCredentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Validate value type based on field
	if req.Field == "intercept_warmup_requests" {
		// Must be boolean
		if _, ok := req.Value.(bool); !ok {
			response.BadRequest(c, "intercept_warmup_requests must be boolean")
			return
		}
	} else {
		// account_uuid and org_uuid can be string or null
		if req.Value != nil {
			if _, ok := req.Value.(string); !ok {
				response.BadRequest(c, req.Field+" must be string or null")
				return
			}
		}
	}

	ctx := c.Request.Context()
	success := 0
	failed := 0
	results := []gin.H{}

	for _, accountID := range req.AccountIDs {
		// Get account
		account, err := h.adminService.GetAccount(ctx, accountID)
		if err != nil {
			failed++
			results = append(results, gin.H{
				"account_id": accountID,
				"success":    false,
				"error":      "Account not found",
			})
			continue
		}

		// Update credentials field
		if account.Credentials == nil {
			account.Credentials = make(map[string]any)
		}

		account.Credentials[req.Field] = req.Value

		// Update account
		updateInput := &service.UpdateAccountInput{
			Credentials: account.Credentials,
		}

		_, err = h.adminService.UpdateAccount(ctx, accountID, updateInput)
		if err != nil {
			failed++
			results = append(results, gin.H{
				"account_id": accountID,
				"success":    false,
				"error":      err.Error(),
			})
			continue
		}

		success++
		results = append(results, gin.H{
			"account_id": accountID,
			"success":    true,
		})
	}

	response.Success(c, gin.H{
		"success": success,
		"failed":  failed,
		"results": results,
	})
}

// BulkUpdate handles bulk updating accounts with selected fields/credentials.
// POST /api/v1/admin/accounts/bulk-update
func (h *AccountHandler) BulkUpdate(c *gin.Context) {
	var req BulkUpdateAccountsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	hasUpdates := req.Name != "" ||
		req.ProxyID != nil ||
		req.Concurrency != nil ||
		req.Priority != nil ||
		req.Status != "" ||
		req.GroupIDs != nil ||
		len(req.Credentials) > 0 ||
		len(req.Extra) > 0

	if !hasUpdates {
		response.BadRequest(c, "No updates provided")
		return
	}

	result, err := h.adminService.BulkUpdateAccounts(c.Request.Context(), &service.BulkUpdateAccountsInput{
		AccountIDs:  req.AccountIDs,
		Name:        req.Name,
		ProxyID:     req.ProxyID,
		Concurrency: req.Concurrency,
		Priority:    req.Priority,
		Status:      req.Status,
		GroupIDs:    req.GroupIDs,
		Credentials: req.Credentials,
		Extra:       req.Extra,
	})
	if err != nil {
		response.InternalError(c, "Failed to bulk update accounts: "+err.Error())
		return
	}

	response.Success(c, result)
}

// ========== OAuth Handlers ==========

// GenerateAuthURLRequest represents the request for generating auth URL
type GenerateAuthURLRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

// GenerateAuthURL generates OAuth authorization URL with full scope
// POST /api/v1/admin/accounts/generate-auth-url
func (h *OAuthHandler) GenerateAuthURL(c *gin.Context) {
	var req GenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body
		req = GenerateAuthURLRequest{}
	}

	result, err := h.oauthService.GenerateAuthURL(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.InternalError(c, "Failed to generate auth URL: "+err.Error())
		return
	}

	response.Success(c, result)
}

// GenerateSetupTokenURL generates OAuth authorization URL for setup token (inference only)
// POST /api/v1/admin/accounts/generate-setup-token-url
func (h *OAuthHandler) GenerateSetupTokenURL(c *gin.Context) {
	var req GenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body
		req = GenerateAuthURLRequest{}
	}

	result, err := h.oauthService.GenerateSetupTokenURL(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.InternalError(c, "Failed to generate setup token URL: "+err.Error())
		return
	}

	response.Success(c, result)
}

// ExchangeCodeRequest represents the request for exchanging auth code
type ExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	Code      string `json:"code" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
}

// ExchangeCode exchanges authorization code for tokens
// POST /api/v1/admin/accounts/exchange-code
func (h *OAuthHandler) ExchangeCode(c *gin.Context) {
	var req ExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	tokenInfo, err := h.oauthService.ExchangeCode(c.Request.Context(), &service.ExchangeCodeInput{
		SessionID: req.SessionID,
		Code:      req.Code,
		ProxyID:   req.ProxyID,
	})
	if err != nil {
		response.BadRequest(c, "Failed to exchange code: "+err.Error())
		return
	}

	response.Success(c, tokenInfo)
}

// ExchangeSetupTokenCode exchanges authorization code for setup token
// POST /api/v1/admin/accounts/exchange-setup-token-code
func (h *OAuthHandler) ExchangeSetupTokenCode(c *gin.Context) {
	var req ExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	tokenInfo, err := h.oauthService.ExchangeCode(c.Request.Context(), &service.ExchangeCodeInput{
		SessionID: req.SessionID,
		Code:      req.Code,
		ProxyID:   req.ProxyID,
	})
	if err != nil {
		response.BadRequest(c, "Failed to exchange code: "+err.Error())
		return
	}

	response.Success(c, tokenInfo)
}

// CookieAuthRequest represents the request for cookie-based authentication
type CookieAuthRequest struct {
	SessionKey string `json:"code" binding:"required"` // Using 'code' field as sessionKey (frontend sends it this way)
	ProxyID    *int64 `json:"proxy_id"`
}

// CookieAuth performs OAuth using sessionKey (cookie-based auto-auth)
// POST /api/v1/admin/accounts/cookie-auth
func (h *OAuthHandler) CookieAuth(c *gin.Context) {
	var req CookieAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	tokenInfo, err := h.oauthService.CookieAuth(c.Request.Context(), &service.CookieAuthInput{
		SessionKey: req.SessionKey,
		ProxyID:    req.ProxyID,
		Scope:      "full",
	})
	if err != nil {
		response.BadRequest(c, "Cookie auth failed: "+err.Error())
		return
	}

	response.Success(c, tokenInfo)
}

// SetupTokenCookieAuth performs OAuth using sessionKey for setup token (inference only)
// POST /api/v1/admin/accounts/setup-token-cookie-auth
func (h *OAuthHandler) SetupTokenCookieAuth(c *gin.Context) {
	var req CookieAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	tokenInfo, err := h.oauthService.CookieAuth(c.Request.Context(), &service.CookieAuthInput{
		SessionKey: req.SessionKey,
		ProxyID:    req.ProxyID,
		Scope:      "inference",
	})
	if err != nil {
		response.BadRequest(c, "Cookie auth failed: "+err.Error())
		return
	}

	response.Success(c, tokenInfo)
}

// GetUsage handles getting account usage information
// GET /api/v1/admin/accounts/:id/usage
func (h *AccountHandler) GetUsage(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	usage, err := h.accountUsageService.GetUsage(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, "Failed to get usage: "+err.Error())
		return
	}

	response.Success(c, usage)
}

// ClearRateLimit handles clearing account rate limit status
// POST /api/v1/admin/accounts/:id/clear-rate-limit
func (h *AccountHandler) ClearRateLimit(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	err = h.rateLimitService.ClearRateLimit(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, "Failed to clear rate limit: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Rate limit cleared successfully"})
}

// GetTodayStats handles getting account today statistics
// GET /api/v1/admin/accounts/:id/today-stats
func (h *AccountHandler) GetTodayStats(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	stats, err := h.accountUsageService.GetTodayStats(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, "Failed to get today stats: "+err.Error())
		return
	}

	response.Success(c, stats)
}

// SetSchedulableRequest represents the request body for setting schedulable status
type SetSchedulableRequest struct {
	Schedulable bool `json:"schedulable"`
}

// SetSchedulable handles toggling account schedulable status
// POST /api/v1/admin/accounts/:id/schedulable
func (h *AccountHandler) SetSchedulable(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	var req SetSchedulableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	account, err := h.adminService.SetAccountSchedulable(c.Request.Context(), accountID, req.Schedulable)
	if err != nil {
		response.InternalError(c, "Failed to update schedulable status: "+err.Error())
		return
	}

	response.Success(c, account)
}

// GetAvailableModels handles getting available models for an account
// GET /api/v1/admin/accounts/:id/models
func (h *AccountHandler) GetAvailableModels(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.NotFound(c, "Account not found")
		return
	}

	// Handle OpenAI accounts
	if account.IsOpenAI() {
		// For OAuth accounts: return default OpenAI models
		if account.IsOAuth() {
			response.Success(c, openai.DefaultModels)
			return
		}

		// For API Key accounts: check model_mapping
		mapping := account.GetModelMapping()
		if len(mapping) == 0 {
			response.Success(c, openai.DefaultModels)
			return
		}

		// Return mapped models
		var models []openai.Model
		for requestedModel := range mapping {
			var found bool
			for _, dm := range openai.DefaultModels {
				if dm.ID == requestedModel {
					models = append(models, dm)
					found = true
					break
				}
			}
			if !found {
				models = append(models, openai.Model{
					ID:          requestedModel,
					Object:      "model",
					Type:        "model",
					DisplayName: requestedModel,
				})
			}
		}
		response.Success(c, models)
		return
	}

	// Handle Claude/Anthropic accounts
	// For OAuth and Setup-Token accounts: return default models
	if account.IsOAuth() {
		response.Success(c, claude.DefaultModels)
		return
	}

	// For API Key accounts: return models based on model_mapping
	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		// No mapping configured, return default models
		response.Success(c, claude.DefaultModels)
		return
	}

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
			}
		}
		// If not found in defaults, create a basic entry
		if !found {
			models = append(models, claude.Model{
				ID:          requestedModel,
				Type:        "model",
				DisplayName: requestedModel,
				CreatedAt:   "",
			})
		}
	}

	response.Success(c, models)
}

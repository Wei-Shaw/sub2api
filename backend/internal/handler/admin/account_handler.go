// Package admin provides HTTP handlers for administrative operations.
package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
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
	adminService            service.AdminService
	oauthService            *service.OAuthService
	openaiOAuthService      *service.OpenAIOAuthService
	geminiOAuthService      *service.GeminiOAuthService
	antigravityOAuthService *service.AntigravityOAuthService
	rateLimitService        *service.RateLimitService
	accountUsageService     *service.AccountUsageService
	accountTestService      *service.AccountTestService
	concurrencyService      *service.ConcurrencyService
	crsSyncService          *service.CRSSyncService
	sessionLimitCache       service.SessionLimitCache
	rpmCache                service.RPMCache
	tokenCacheInvalidator   service.TokenCacheInvalidator
}

// NewAccountHandler creates a new admin account handler
func NewAccountHandler(
	adminService service.AdminService,
	oauthService *service.OAuthService,
	openaiOAuthService *service.OpenAIOAuthService,
	geminiOAuthService *service.GeminiOAuthService,
	antigravityOAuthService *service.AntigravityOAuthService,
	rateLimitService *service.RateLimitService,
	accountUsageService *service.AccountUsageService,
	accountTestService *service.AccountTestService,
	concurrencyService *service.ConcurrencyService,
	crsSyncService *service.CRSSyncService,
	sessionLimitCache service.SessionLimitCache,
	rpmCache service.RPMCache,
	tokenCacheInvalidator service.TokenCacheInvalidator,
) *AccountHandler {
	return &AccountHandler{
		adminService:            adminService,
		oauthService:            oauthService,
		openaiOAuthService:      openaiOAuthService,
		geminiOAuthService:      geminiOAuthService,
		antigravityOAuthService: antigravityOAuthService,
		rateLimitService:        rateLimitService,
		accountUsageService:     accountUsageService,
		accountTestService:      accountTestService,
		concurrencyService:      concurrencyService,
		crsSyncService:          crsSyncService,
		sessionLimitCache:       sessionLimitCache,
		rpmCache:                rpmCache,
		tokenCacheInvalidator:   tokenCacheInvalidator,
	}
}

// CreateAccountRequest represents create account request
type CreateAccountRequest struct {
	Name                    string         `json:"name" binding:"required"`
	Notes                   *string        `json:"notes"`
	Platform                string         `json:"platform" binding:"required"`
	Type                    string         `json:"type" binding:"required,oneof=oauth setup-token apikey upstream bedrock"`
	Credentials             map[string]any `json:"credentials" binding:"required"`
	Extra                   map[string]any `json:"extra"`
	ProxyID                 *int64         `json:"proxy_id"`
	Concurrency             int            `json:"concurrency"`
	Priority                int            `json:"priority"`
	RateMultiplier          *float64       `json:"rate_multiplier"`
	LoadFactor              *int           `json:"load_factor"`
	GroupIDs                []int64        `json:"group_ids"`
	ExpiresAt               *int64         `json:"expires_at"`
	AutoPauseOnExpired      *bool          `json:"auto_pause_on_expired"`
	ConfirmMixedChannelRisk *bool          `json:"confirm_mixed_channel_risk"` // 用户确认混合渠道风险
}

// UpdateAccountRequest represents update account request
// 使用指针类型来区分"未提供"和"设置为0"
type UpdateAccountRequest struct {
	Name                    string         `json:"name"`
	Notes                   *string        `json:"notes"`
	Type                    string         `json:"type" binding:"omitempty,oneof=oauth setup-token apikey upstream bedrock"`
	Credentials             map[string]any `json:"credentials"`
	Extra                   map[string]any `json:"extra"`
	ProxyID                 *int64         `json:"proxy_id"`
	Concurrency             *int           `json:"concurrency"`
	Priority                *int           `json:"priority"`
	RateMultiplier          *float64       `json:"rate_multiplier"`
	LoadFactor              *int           `json:"load_factor"`
	Status                  string         `json:"status" binding:"omitempty,oneof=active inactive error"`
	GroupIDs                *[]int64       `json:"group_ids"`
	ExpiresAt               *int64         `json:"expires_at"`
	AutoPauseOnExpired      *bool          `json:"auto_pause_on_expired"`
	ConfirmMixedChannelRisk *bool          `json:"confirm_mixed_channel_risk"` // 用户确认混合渠道风险
}

// BulkUpdateAccountsRequest represents the payload for bulk editing accounts
type BulkUpdateAccountsRequest struct {
	AccountIDs              []int64        `json:"account_ids" binding:"required,min=1"`
	Name                    string         `json:"name"`
	ProxyID                 *int64         `json:"proxy_id"`
	Concurrency             *int           `json:"concurrency"`
	Priority                *int           `json:"priority"`
	RateMultiplier          *float64       `json:"rate_multiplier"`
	LoadFactor              *int           `json:"load_factor"`
	Status                  string         `json:"status" binding:"omitempty,oneof=active inactive error"`
	Schedulable             *bool          `json:"schedulable"`
	GroupIDs                *[]int64       `json:"group_ids"`
	Credentials             map[string]any `json:"credentials"`
	Extra                   map[string]any `json:"extra"`
	ConfirmMixedChannelRisk *bool          `json:"confirm_mixed_channel_risk"` // 用户确认混合渠道风险
}

// CheckMixedChannelRequest represents check mixed channel risk request
type CheckMixedChannelRequest struct {
	Platform  string  `json:"platform" binding:"required"`
	GroupIDs  []int64 `json:"group_ids"`
	AccountID *int64  `json:"account_id"`
}

// AccountWithConcurrency extends Account with real-time concurrency info
type AccountWithConcurrency struct {
	*dto.Account
	CurrentConcurrency int `json:"current_concurrency"`
	// 以下字段仅对 Anthropic OAuth/SetupToken 账号有效，且仅在启用相应功能时返回
	CurrentWindowCost *float64 `json:"current_window_cost,omitempty"` // 当前窗口费用
	ActiveSessions    *int     `json:"active_sessions,omitempty"`     // 当前活跃会话数
	CurrentRPM        *int     `json:"current_rpm,omitempty"`         // 当前分钟 RPM 计数
}

const accountListGroupUngroupedQueryValue = "ungrouped"

func (h *AccountHandler) buildAccountResponseWithRuntime(ctx context.Context, account *service.Account) AccountWithConcurrency {
	item := AccountWithConcurrency{
		Account:            dto.AccountFromService(account),
		CurrentConcurrency: 0,
	}
	if account == nil {
		return item
	}

	if h.concurrencyService != nil {
		if counts, err := h.concurrencyService.GetAccountConcurrencyBatch(ctx, []int64{account.ID}); err == nil {
			item.CurrentConcurrency = counts[account.ID]
		}
	}

	if account.IsAnthropicOAuthOrSetupToken() {
		if h.accountUsageService != nil && account.GetWindowCostLimit() > 0 {
			startTime := account.GetCurrentWindowStartTime()
			if stats, err := h.accountUsageService.GetAccountWindowStats(ctx, account.ID, startTime); err == nil && stats != nil {
				cost := stats.StandardCost
				item.CurrentWindowCost = &cost
			}
		}

		if h.sessionLimitCache != nil && account.GetMaxSessions() > 0 {
			idleTimeout := time.Duration(account.GetSessionIdleTimeoutMinutes()) * time.Minute
			idleTimeouts := map[int64]time.Duration{account.ID: idleTimeout}
			if sessions, err := h.sessionLimitCache.GetActiveSessionCountBatch(ctx, []int64{account.ID}, idleTimeouts); err == nil {
				if count, ok := sessions[account.ID]; ok {
					item.ActiveSessions = &count
				}
			}
		}

		if h.rpmCache != nil && account.GetBaseRPM() > 0 {
			if rpm, err := h.rpmCache.GetRPM(ctx, account.ID); err == nil {
				item.CurrentRPM = &rpm
			}
		}
	}

	return item
}

// List handles listing all accounts with pagination
// GET /api/v1/admin/accounts
func (h *AccountHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	platform := c.Query("platform")
	accountType := c.Query("type")
	status := c.Query("status")
	search := c.Query("search")
	privacyMode := strings.TrimSpace(c.Query("privacy_mode"))
	// 标准化和验证 search 参数
	search = strings.TrimSpace(search)
	if len(search) > 100 {
		search = search[:100]
	}
	lite := parseBoolQueryWithDefault(c.Query("lite"), false)

	var groupID int64
	if groupIDStr := c.Query("group"); groupIDStr != "" {
		if groupIDStr == accountListGroupUngroupedQueryValue {
			groupID = service.AccountListGroupUngrouped
		} else {
			parsedGroupID, parseErr := strconv.ParseInt(groupIDStr, 10, 64)
			if parseErr != nil {
				response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_FILTER", "invalid group filter"))
				return
			}
			if parsedGroupID < 0 {
				response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_FILTER", "invalid group filter"))
				return
			}
			groupID = parsedGroupID
		}
	}

	accounts, total, err := h.adminService.ListAccounts(c.Request.Context(), page, pageSize, platform, accountType, status, search, groupID, privacyMode)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Get current concurrency counts for all accounts
	accountIDs := make([]int64, len(accounts))
	for i, acc := range accounts {
		accountIDs[i] = acc.ID
	}

	concurrencyCounts := make(map[int64]int)
	var windowCosts map[int64]float64
	var activeSessions map[int64]int
	var rpmCounts map[int64]int

	// 始终获取并发数（Redis ZCARD，极低开销）
	if h.concurrencyService != nil {
		if cc, ccErr := h.concurrencyService.GetAccountConcurrencyBatch(c.Request.Context(), accountIDs); ccErr == nil && cc != nil {
			concurrencyCounts = cc
		}
	}

	// 识别需要查询窗口费用、会话数和 RPM 的账号（Anthropic OAuth/SetupToken 且启用了相应功能）
	windowCostAccountIDs := make([]int64, 0)
	sessionLimitAccountIDs := make([]int64, 0)
	rpmAccountIDs := make([]int64, 0)
	sessionIdleTimeouts := make(map[int64]time.Duration) // 各账号的会话空闲超时配置
	for i := range accounts {
		acc := &accounts[i]
		if acc.IsAnthropicOAuthOrSetupToken() {
			if acc.GetWindowCostLimit() > 0 {
				windowCostAccountIDs = append(windowCostAccountIDs, acc.ID)
			}
			if acc.GetMaxSessions() > 0 {
				sessionLimitAccountIDs = append(sessionLimitAccountIDs, acc.ID)
				sessionIdleTimeouts[acc.ID] = time.Duration(acc.GetSessionIdleTimeoutMinutes()) * time.Minute
			}
			if acc.GetBaseRPM() > 0 {
				rpmAccountIDs = append(rpmAccountIDs, acc.ID)
			}
		}
	}

	// 始终获取 RPM 计数（Redis GET，极低开销）
	if len(rpmAccountIDs) > 0 && h.rpmCache != nil {
		rpmCounts, _ = h.rpmCache.GetRPMBatch(c.Request.Context(), rpmAccountIDs)
		if rpmCounts == nil {
			rpmCounts = make(map[int64]int)
		}
	}

	// 始终获取活跃会话数（Redis ZCARD，低开销）
	if len(sessionLimitAccountIDs) > 0 && h.sessionLimitCache != nil {
		activeSessions, _ = h.sessionLimitCache.GetActiveSessionCountBatch(c.Request.Context(), sessionLimitAccountIDs, sessionIdleTimeouts)
		if activeSessions == nil {
			activeSessions = make(map[int64]int)
		}
	}

	// 始终获取窗口费用（PostgreSQL 聚合查询）
	if len(windowCostAccountIDs) > 0 {
		windowCosts = make(map[int64]float64)
		var mu sync.Mutex
		g, gctx := errgroup.WithContext(c.Request.Context())
		g.SetLimit(10) // 限制并发数

		for i := range accounts {
			acc := &accounts[i]
			if !acc.IsAnthropicOAuthOrSetupToken() || acc.GetWindowCostLimit() <= 0 {
				continue
			}
			accCopy := acc // 闭包捕获
			g.Go(func() error {
				// 使用统一的窗口开始时间计算逻辑（考虑窗口过期情况）
				startTime := accCopy.GetCurrentWindowStartTime()
				stats, err := h.accountUsageService.GetAccountWindowStats(gctx, accCopy.ID, startTime)
				if err == nil && stats != nil {
					mu.Lock()
					windowCosts[accCopy.ID] = stats.StandardCost // 使用标准费用
					mu.Unlock()
				}
				return nil // 不返回错误，允许部分失败
			})
		}
		_ = g.Wait()
	}

	// Build response with concurrency info
	result := make([]AccountWithConcurrency, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		item := AccountWithConcurrency{
			Account:            dto.AccountFromService(acc),
			CurrentConcurrency: concurrencyCounts[acc.ID],
		}

		// 添加窗口费用（仅当启用时）
		if windowCosts != nil {
			if cost, ok := windowCosts[acc.ID]; ok {
				item.CurrentWindowCost = &cost
			}
		}

		// 添加活跃会话数（仅当启用时）
		if activeSessions != nil {
			if count, ok := activeSessions[acc.ID]; ok {
				item.ActiveSessions = &count
			}
		}

		// 添加 RPM 计数（仅当启用时）
		if rpmCounts != nil {
			if rpm, ok := rpmCounts[acc.ID]; ok {
				item.CurrentRPM = &rpm
			}
		}

		result[i] = item
	}

	etag := buildAccountsListETag(result, total, page, pageSize, platform, accountType, status, search, lite)
	if etag != "" {
		c.Header("ETag", etag)
		c.Header("Vary", "If-None-Match")
		if ifNoneMatchMatched(c.GetHeader("If-None-Match"), etag) {
			c.Status(http.StatusNotModified)
			return
		}
	}

	response.Paginated(c, result, total, page, pageSize)
}

func buildAccountsListETag(
	items []AccountWithConcurrency,
	total int64,
	page, pageSize int,
	platform, accountType, status, search string,
	lite bool,
) string {
	payload := struct {
		Total       int64                    `json:"total"`
		Page        int                      `json:"page"`
		PageSize    int                      `json:"page_size"`
		Platform    string                   `json:"platform"`
		AccountType string                   `json:"type"`
		Status      string                   `json:"status"`
		Search      string                   `json:"search"`
		Lite        bool                     `json:"lite"`
		Items       []AccountWithConcurrency `json:"items"`
	}{
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
		Platform:    platform,
		AccountType: accountType,
		Status:      status,
		Search:      search,
		Lite:        lite,
		Items:       items,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "\"" + hex.EncodeToString(sum[:]) + "\""
}

func ifNoneMatchMatched(ifNoneMatch, etag string) bool {
	if etag == "" || ifNoneMatch == "" {
		return false
	}
	for _, token := range strings.Split(ifNoneMatch, ",") {
		candidate := strings.TrimSpace(token)
		if candidate == "*" {
			return true
		}
		if candidate == etag {
			return true
		}
		if strings.HasPrefix(candidate, "W/") && strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
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
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account))
}

// CheckMixedChannel handles checking mixed channel risk for account-group binding.
// POST /api/v1/admin/accounts/check-mixed-channel
func (h *AccountHandler) CheckMixedChannel(c *gin.Context) {
	var req CheckMixedChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if len(req.GroupIDs) == 0 {
		response.Success(c, gin.H{"has_risk": false})
		return
	}

	accountID := int64(0)
	if req.AccountID != nil {
		accountID = *req.AccountID
	}

	err := h.adminService.CheckMixedChannelRisk(c.Request.Context(), accountID, req.Platform, req.GroupIDs)
	if err != nil {
		var mixedErr *service.MixedChannelError
		if errors.As(err, &mixedErr) {
			response.Success(c, gin.H{
				"has_risk": true,
				"error":    "mixed_channel_warning",
				"message":  mixedErr.Error(),
				"details": gin.H{
					"group_id":         mixedErr.GroupID,
					"group_name":       mixedErr.GroupName,
					"current_platform": mixedErr.CurrentPlatform,
					"other_platform":   mixedErr.OtherPlatform,
				},
			})
			return
		}

		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"has_risk": false})
}

// Create handles creating a new account
// POST /api/v1/admin/accounts
func (h *AccountHandler) Create(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.RateMultiplier != nil && *req.RateMultiplier < 0 {
		response.BadRequest(c, "rate_multiplier must be >= 0")
		return
	}
	// base_rpm 输入校验：负值归零，超过 10000 截断
	sanitizeExtraBaseRPM(req.Extra)

	// 确定是否跳过混合渠道检查
	skipCheck := req.ConfirmMixedChannelRisk != nil && *req.ConfirmMixedChannelRisk

	result, err := executeAdminIdempotent(c, "admin.accounts.create", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		account, execErr := h.adminService.CreateAccount(ctx, &service.CreateAccountInput{
			Name:                  req.Name,
			Notes:                 req.Notes,
			Platform:              req.Platform,
			Type:                  req.Type,
			Credentials:           req.Credentials,
			Extra:                 req.Extra,
			ProxyID:               req.ProxyID,
			Concurrency:           req.Concurrency,
			Priority:              req.Priority,
			RateMultiplier:        req.RateMultiplier,
			LoadFactor:            req.LoadFactor,
			GroupIDs:              req.GroupIDs,
			ExpiresAt:             req.ExpiresAt,
			AutoPauseOnExpired:    req.AutoPauseOnExpired,
			SkipMixedChannelCheck: skipCheck,
		})
		if execErr != nil {
			return nil, execErr
		}
		// Antigravity OAuth: 新账号直接设置隐私
		h.adminService.ForceAntigravityPrivacy(ctx, account)
		// OpenAI OAuth: 新账号直接设置隐私
		h.adminService.ForceOpenAIPrivacy(ctx, account)
		return h.buildAccountResponseWithRuntime(ctx, account), nil
	})
	if err != nil {
		// 检查是否为混合渠道错误
		var mixedErr *service.MixedChannelError
		if errors.As(err, &mixedErr) {
			// 创建接口仅返回最小必要字段，详细信息由专门检查接口提供
			c.JSON(409, gin.H{
				"error":   "mixed_channel_warning",
				"message": mixedErr.Error(),
			})
			return
		}

		if retryAfter := service.RetryAfterSecondsFromError(err); retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		response.ErrorFrom(c, err)
		return
	}

	if result != nil && result.Replayed {
		c.Header("X-Idempotency-Replayed", "true")
	}
	response.Success(c, result.Data)
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
	if req.RateMultiplier != nil && *req.RateMultiplier < 0 {
		response.BadRequest(c, "rate_multiplier must be >= 0")
		return
	}
	// base_rpm 输入校验：负值归零，超过 10000 截断
	sanitizeExtraBaseRPM(req.Extra)

	// 确定是否跳过混合渠道检查
	skipCheck := req.ConfirmMixedChannelRisk != nil && *req.ConfirmMixedChannelRisk

	account, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{
		Name:                  req.Name,
		Notes:                 req.Notes,
		Type:                  req.Type,
		Credentials:           req.Credentials,
		Extra:                 req.Extra,
		ProxyID:               req.ProxyID,
		Concurrency:           req.Concurrency, // 指针类型，nil 表示未提供
		Priority:              req.Priority,    // 指针类型，nil 表示未提供
		RateMultiplier:        req.RateMultiplier,
		LoadFactor:            req.LoadFactor,
		Status:                req.Status,
		GroupIDs:              req.GroupIDs,
		ExpiresAt:             req.ExpiresAt,
		AutoPauseOnExpired:    req.AutoPauseOnExpired,
		SkipMixedChannelCheck: skipCheck,
	})
	if err != nil {
		// 检查是否为混合渠道错误
		var mixedErr *service.MixedChannelError
		if errors.As(err, &mixedErr) {
			// 更新接口仅返回最小必要字段，详细信息由专门检查接口提供
			c.JSON(409, gin.H{
				"error":   "mixed_channel_warning",
				"message": mixedErr.Error(),
			})
			return
		}

		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account))
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
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Account deleted successfully"})
}

// TestAccountRequest represents the request body for testing an account
type TestAccountRequest struct {
	ModelID string `json:"model_id"`
	Prompt  string `json:"prompt"`
}

type SyncFromCRSRequest struct {
	BaseURL            string   `json:"base_url" binding:"required"`
	Username           string   `json:"username" binding:"required"`
	Password           string   `json:"password" binding:"required"`
	SyncProxies        *bool    `json:"sync_proxies"`
	SelectedAccountIDs []string `json:"selected_account_ids"`
}

type PreviewFromCRSRequest struct {
	BaseURL  string `json:"base_url" binding:"required"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
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
	if err := h.accountTestService.TestAccountConnection(c, accountID, req.ModelID, req.Prompt); err != nil {
		// Error already sent via SSE, just log
		return
	}

	if h.rateLimitService != nil {
		if _, err := h.rateLimitService.RecoverAccountAfterSuccessfulTest(c.Request.Context(), accountID); err != nil {
			_ = c.Error(err)
		}
	}
}

// RecoverState handles unified recovery of recoverable account runtime state.
// POST /api/v1/admin/accounts/:id/recover-state
func (h *AccountHandler) RecoverState(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	if h.rateLimitService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Rate limit service unavailable")
		return
	}

	if _, err := h.rateLimitService.RecoverAccountState(c.Request.Context(), accountID, service.AccountRecoveryOptions{
		InvalidateToken: true,
	}); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account))
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
		BaseURL:            req.BaseURL,
		Username:           req.Username,
		Password:           req.Password,
		SyncProxies:        syncProxies,
		SelectedAccountIDs: req.SelectedAccountIDs,
	})
	if err != nil {
		// Provide detailed error message for CRS sync failures
		response.InternalError(c, "CRS sync failed: "+err.Error())
		return
	}

	response.Success(c, result)
}

// PreviewFromCRS handles previewing accounts from CRS before sync
// POST /api/v1/admin/accounts/sync/crs/preview
func (h *AccountHandler) PreviewFromCRS(c *gin.Context) {
	var req PreviewFromCRSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.crsSyncService.PreviewFromCRS(c.Request.Context(), service.SyncFromCRSInput{
		BaseURL:  req.BaseURL,
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		response.InternalError(c, "CRS preview failed: "+err.Error())
		return
	}

	response.Success(c, result)
}

// refreshSingleAccount refreshes credentials for a single OAuth account.
// Returns (updatedAccount, warning, error) where warning is used for Antigravity ProjectIDMissing scenario.
func (h *AccountHandler) refreshSingleAccount(ctx context.Context, account *service.Account) (*service.Account, string, error) {
	if !account.IsOAuth() {
		return nil, "", infraerrors.BadRequest("NOT_OAUTH", "cannot refresh non-OAuth account")
	}

	var newCredentials map[string]any

	if account.IsOpenAI() {
		tokenInfo, err := h.openaiOAuthService.RefreshAccountToken(ctx, account)
		if err != nil {
			// 刷新失败但 access_token 可能仍有效，尝试设置隐私
			h.adminService.EnsureOpenAIPrivacy(ctx, account)
			return nil, "", err
		}

		newCredentials = h.openaiOAuthService.BuildAccountCredentials(tokenInfo)
		for k, v := range account.Credentials {
			if _, exists := newCredentials[k]; !exists {
				newCredentials[k] = v
			}
		}
	} else if account.Platform == service.PlatformGemini {
		tokenInfo, err := h.geminiOAuthService.RefreshAccountToken(ctx, account)
		if err != nil {
			return nil, "", fmt.Errorf("failed to refresh credentials: %w", err)
		}

		newCredentials = h.geminiOAuthService.BuildAccountCredentials(tokenInfo)
		for k, v := range account.Credentials {
			if _, exists := newCredentials[k]; !exists {
				newCredentials[k] = v
			}
		}
	} else if account.Platform == service.PlatformAntigravity {
		tokenInfo, err := h.antigravityOAuthService.RefreshAccountToken(ctx, account)
		if err != nil {
			return nil, "", err
		}

		newCredentials = h.antigravityOAuthService.BuildAccountCredentials(tokenInfo)
		for k, v := range account.Credentials {
			if _, exists := newCredentials[k]; !exists {
				newCredentials[k] = v
			}
		}

		// 特殊处理 project_id：如果新值为空但旧值非空，保留旧值
		// 这确保了即使 LoadCodeAssist 失败，project_id 也不会丢失
		if newProjectID, _ := newCredentials["project_id"].(string); newProjectID == "" {
			if oldProjectID := strings.TrimSpace(account.GetCredential("project_id")); oldProjectID != "" {
				newCredentials["project_id"] = oldProjectID
			}
		}

		// 如果 project_id 获取失败，更新凭证但不标记为 error
		if tokenInfo.ProjectIDMissing {
			updatedAccount, updateErr := h.adminService.UpdateAccount(ctx, account.ID, &service.UpdateAccountInput{
				Credentials: newCredentials,
			})
			if updateErr != nil {
				return nil, "", fmt.Errorf("failed to update credentials: %w", updateErr)
			}
			h.adminService.EnsureAntigravityPrivacy(ctx, updatedAccount)
			return updatedAccount, "missing_project_id_temporary", nil
		}

		// 成功获取到 project_id，如果之前是 missing_project_id 错误则清除
		if account.Status == service.StatusError && strings.Contains(account.ErrorMessage, "missing_project_id:") {
			if _, clearErr := h.adminService.ClearAccountError(ctx, account.ID); clearErr != nil {
				return nil, "", fmt.Errorf("failed to clear account error: %w", clearErr)
			}
		}
	} else {
		// Use Anthropic/Claude OAuth service to refresh token
		tokenInfo, err := h.oauthService.RefreshAccountToken(ctx, account)
		if err != nil {
			return nil, "", err
		}

		// Copy existing credentials to preserve non-token settings (e.g., intercept_warmup_requests)
		newCredentials = make(map[string]any)
		for k, v := range account.Credentials {
			newCredentials[k] = v
		}

		// Update token-related fields
		newCredentials["access_token"] = tokenInfo.AccessToken
		newCredentials["token_type"] = tokenInfo.TokenType
		newCredentials["expires_in"] = strconv.FormatInt(tokenInfo.ExpiresIn, 10)
		newCredentials["expires_at"] = strconv.FormatInt(tokenInfo.ExpiresAt, 10)
		if strings.TrimSpace(tokenInfo.RefreshToken) != "" {
			newCredentials["refresh_token"] = tokenInfo.RefreshToken
		}
		if strings.TrimSpace(tokenInfo.Scope) != "" {
			newCredentials["scope"] = tokenInfo.Scope
		}
	}

	updatedAccount, err := h.adminService.UpdateAccount(ctx, account.ID, &service.UpdateAccountInput{
		Credentials: newCredentials,
	})
	if err != nil {
		return nil, "", err
	}

	// 刷新成功后，清除 token 缓存，确保下次请求使用新 token
	if h.tokenCacheInvalidator != nil {
		if invalidateErr := h.tokenCacheInvalidator.InvalidateToken(ctx, updatedAccount); invalidateErr != nil {
			log.Printf("[WARN] Failed to invalidate token cache for account %d: %v", updatedAccount.ID, invalidateErr)
		}
	}

	// OpenAI OAuth: 刷新成功后检查并设置 privacy_mode
	h.adminService.EnsureOpenAIPrivacy(ctx, updatedAccount)
	// Antigravity OAuth: 刷新成功后检查并设置 privacy_mode
	h.adminService.EnsureAntigravityPrivacy(ctx, updatedAccount)

	return updatedAccount, "", nil
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

	updatedAccount, warning, err := h.refreshSingleAccount(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if warning == "missing_project_id_temporary" {
		response.Success(c, gin.H{
			"message": "Token refreshed successfully, but project_id could not be retrieved (will retry automatically)",
			"warning": "missing_project_id_temporary",
		})
		return
	}

	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), updatedAccount))
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
		response.ErrorFrom(c, err)
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
		response.ErrorFrom(c, err)
		return
	}

	// 清除错误后，同时清除 token 缓存，确保下次请求会获取最新的 token（触发刷新或从 DB 读取）
	// 这解决了管理员重置账号状态后，旧的失效 token 仍在缓存中导致立即再次 401 的问题
	if h.tokenCacheInvalidator != nil && account.IsOAuth() {
		if invalidateErr := h.tokenCacheInvalidator.InvalidateToken(c.Request.Context(), account); invalidateErr != nil {
			log.Printf("[WARN] Failed to invalidate token cache for account %d: %v", accountID, invalidateErr)
		}
	}

	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account))
}

// BatchClearError handles batch clearing account errors
// POST /api/v1/admin/accounts/batch-clear-error
func (h *AccountHandler) BatchClearError(c *gin.Context) {
	var req struct {
		AccountIDs []int64 `json:"account_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if len(req.AccountIDs) == 0 {
		response.BadRequest(c, "account_ids is required")
		return
	}

	ctx := c.Request.Context()

	const maxConcurrency = 10
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrency)

	var mu sync.Mutex
	var successCount, failedCount int
	var errors []gin.H

	// 注意：所有 goroutine 必须 return nil，避免 errgroup cancel 其他并发任务
	for _, id := range req.AccountIDs {
		accountID := id // 闭包捕获
		g.Go(func() error {
			account, err := h.adminService.ClearAccountError(gctx, accountID)
			if err != nil {
				mu.Lock()
				failedCount++
				errors = append(errors, gin.H{
					"account_id": accountID,
					"error":      err.Error(),
				})
				mu.Unlock()
				return nil
			}

			// 清除错误后，同时清除 token 缓存
			if h.tokenCacheInvalidator != nil && account.IsOAuth() {
				if invalidateErr := h.tokenCacheInvalidator.InvalidateToken(gctx, account); invalidateErr != nil {
					log.Printf("[WARN] Failed to invalidate token cache for account %d: %v", accountID, invalidateErr)
				}
			}

			mu.Lock()
			successCount++
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"total":   len(req.AccountIDs),
		"success": successCount,
		"failed":  failedCount,
		"errors":  errors,
	})
}

// BatchRefresh handles batch refreshing account credentials
// POST /api/v1/admin/accounts/batch-refresh
func (h *AccountHandler) BatchRefresh(c *gin.Context) {
	var req struct {
		AccountIDs []int64 `json:"account_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if len(req.AccountIDs) == 0 {
		response.BadRequest(c, "account_ids is required")
		return
	}

	ctx := c.Request.Context()

	accounts, err := h.adminService.GetAccountsByIDs(ctx, req.AccountIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 建立已获取账号的 ID 集合，检测缺失的 ID
	foundIDs := make(map[int64]bool, len(accounts))
	for _, acc := range accounts {
		if acc != nil {
			foundIDs[acc.ID] = true
		}
	}

	const maxConcurrency = 10
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrency)

	var mu sync.Mutex
	var successCount, failedCount int
	var errors []gin.H
	var warnings []gin.H

	// 将不存在的账号 ID 标记为失败
	for _, id := range req.AccountIDs {
		if !foundIDs[id] {
			failedCount++
			errors = append(errors, gin.H{
				"account_id": id,
				"error":      "account not found",
			})
		}
	}

	// 注意：所有 goroutine 必须 return nil，避免 errgroup cancel 其他并发任务
	for _, account := range accounts {
		acc := account // 闭包捕获
		if acc == nil {
			continue
		}
		g.Go(func() error {
			_, warning, err := h.refreshSingleAccount(gctx, acc)
			mu.Lock()
			if err != nil {
				failedCount++
				errors = append(errors, gin.H{
					"account_id": acc.ID,
					"error":      err.Error(),
				})
			} else {
				successCount++
				if warning != "" {
					warnings = append(warnings, gin.H{
						"account_id": acc.ID,
						"warning":    warning,
					})
				}
			}
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"total":    len(req.AccountIDs),
		"success":  successCount,
		"failed":   failedCount,
		"errors":   errors,
		"warnings": warnings,
	})
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

	executeAdminIdempotentJSON(c, "admin.accounts.batch_create", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		success := 0
		failed := 0
		results := make([]gin.H, 0, len(req.Accounts))
		// 收集需要异步设置隐私的 OAuth 账号
		var antigravityPrivacyAccounts []*service.Account
		var openaiPrivacyAccounts []*service.Account

		for _, item := range req.Accounts {
			if item.RateMultiplier != nil && *item.RateMultiplier < 0 {
				failed++
				results = append(results, gin.H{
					"name":    item.Name,
					"success": false,
					"error":   "rate_multiplier must be >= 0",
				})
				continue
			}

			// base_rpm 输入校验：负值归零，超过 10000 截断
			sanitizeExtraBaseRPM(item.Extra)

			skipCheck := item.ConfirmMixedChannelRisk != nil && *item.ConfirmMixedChannelRisk

			account, err := h.adminService.CreateAccount(ctx, &service.CreateAccountInput{
				Name:                  item.Name,
				Notes:                 item.Notes,
				Platform:              item.Platform,
				Type:                  item.Type,
				Credentials:           item.Credentials,
				Extra:                 item.Extra,
				ProxyID:               item.ProxyID,
				Concurrency:           item.Concurrency,
				Priority:              item.Priority,
				RateMultiplier:        item.RateMultiplier,
				GroupIDs:              item.GroupIDs,
				ExpiresAt:             item.ExpiresAt,
				AutoPauseOnExpired:    item.AutoPauseOnExpired,
				SkipMixedChannelCheck: skipCheck,
			})
			if err != nil {
				failed++
				results = append(results, gin.H{
					"name":    item.Name,
					"success": false,
					"error":   err.Error(),
				})
				continue
			}
			// 收集需要异步设置隐私的 OAuth 账号
			if account.Type == service.AccountTypeOAuth {
				switch account.Platform {
				case service.PlatformAntigravity:
					antigravityPrivacyAccounts = append(antigravityPrivacyAccounts, account)
				case service.PlatformOpenAI:
					openaiPrivacyAccounts = append(openaiPrivacyAccounts, account)
				}
			}
			success++
			results = append(results, gin.H{
				"name":    item.Name,
				"id":      account.ID,
				"success": true,
			})
		}

		// 异步设置隐私，避免批量创建时阻塞请求
		adminSvc := h.adminService
		if len(antigravityPrivacyAccounts) > 0 {
			accounts := antigravityPrivacyAccounts
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("batch_create_antigravity_privacy_panic", "recover", r)
					}
				}()
				bgCtx := context.Background()
				for _, acc := range accounts {
					adminSvc.ForceAntigravityPrivacy(bgCtx, acc)
				}
			}()
		}
		if len(openaiPrivacyAccounts) > 0 {
			accounts := openaiPrivacyAccounts
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("batch_create_openai_privacy_panic", "recover", r)
					}
				}()
				bgCtx := context.Background()
				for _, acc := range accounts {
					adminSvc.ForceOpenAIPrivacy(bgCtx, acc)
				}
			}()
		}

		return gin.H{
			"success": success,
			"failed":  failed,
			"results": results,
		}, nil
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

	// 阶段一：预验证所有账号存在，收集 credentials
	type accountUpdate struct {
		ID          int64
		Credentials map[string]any
	}
	updates := make([]accountUpdate, 0, len(req.AccountIDs))
	for _, accountID := range req.AccountIDs {
		account, err := h.adminService.GetAccount(ctx, accountID)
		if err != nil {
			response.Error(c, 404, fmt.Sprintf("Account %d not found", accountID))
			return
		}
		if account.Credentials == nil {
			account.Credentials = make(map[string]any)
		}
		account.Credentials[req.Field] = req.Value
		updates = append(updates, accountUpdate{ID: accountID, Credentials: account.Credentials})
	}

	// 阶段二：依次更新，返回每个账号的成功/失败明细，便于调用方重试
	success := 0
	failed := 0
	successIDs := make([]int64, 0, len(updates))
	failedIDs := make([]int64, 0, len(updates))
	results := make([]gin.H, 0, len(updates))
	for _, u := range updates {
		updateInput := &service.UpdateAccountInput{Credentials: u.Credentials}
		if _, err := h.adminService.UpdateAccount(ctx, u.ID, updateInput); err != nil {
			failed++
			failedIDs = append(failedIDs, u.ID)
			results = append(results, gin.H{
				"account_id": u.ID,
				"success":    false,
				"error":      err.Error(),
			})
			continue
		}
		success++
		successIDs = append(successIDs, u.ID)
		results = append(results, gin.H{
			"account_id": u.ID,
			"success":    true,
		})
	}

	response.Success(c, gin.H{
		"success":     success,
		"failed":      failed,
		"success_ids": successIDs,
		"failed_ids":  failedIDs,
		"results":     results,
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
	if req.RateMultiplier != nil && *req.RateMultiplier < 0 {
		response.BadRequest(c, "rate_multiplier must be >= 0")
		return
	}
	// base_rpm 输入校验：负值归零，超过 10000 截断
	sanitizeExtraBaseRPM(req.Extra)

	// 确定是否跳过混合渠道检查
	skipCheck := req.ConfirmMixedChannelRisk != nil && *req.ConfirmMixedChannelRisk

	hasUpdates := req.Name != "" ||
		req.ProxyID != nil ||
		req.Concurrency != nil ||
		req.Priority != nil ||
		req.RateMultiplier != nil ||
		req.LoadFactor != nil ||
		req.Status != "" ||
		req.Schedulable != nil ||
		req.GroupIDs != nil ||
		len(req.Credentials) > 0 ||
		len(req.Extra) > 0

	if !hasUpdates {
		response.BadRequest(c, "No updates provided")
		return
	}

	result, err := h.adminService.BulkUpdateAccounts(c.Request.Context(), &service.BulkUpdateAccountsInput{
		AccountIDs:            req.AccountIDs,
		Name:                  req.Name,
		ProxyID:               req.ProxyID,
		Concurrency:           req.Concurrency,
		Priority:              req.Priority,
		RateMultiplier:        req.RateMultiplier,
		LoadFactor:            req.LoadFactor,
		Status:                req.Status,
		Schedulable:           req.Schedulable,
		GroupIDs:              req.GroupIDs,
		Credentials:           req.Credentials,
		Extra:                 req.Extra,
		SkipMixedChannelCheck: skipCheck,
	})
	if err != nil {
		var mixedErr *service.MixedChannelError
		if errors.As(err, &mixedErr) {
			c.JSON(409, gin.H{
				"error":   "mixed_channel_warning",
				"message": mixedErr.Error(),
			})
			return
		}
		response.ErrorFrom(c, err)
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
		response.ErrorFrom(c, err)
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
		response.ErrorFrom(c, err)
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
		response.ErrorFrom(c, err)
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
		response.ErrorFrom(c, err)
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
		response.ErrorFrom(c, err)
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
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, tokenInfo)
}

// GetUsage handles getting account usage information
// GET /api/v1/admin/accounts/:id/usage?source=passive|active
func (h *AccountHandler) GetUsage(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	source := c.DefaultQuery("source", "active")

	var usage *service.UsageInfo
	if source == "passive" {
		usage, err = h.accountUsageService.GetPassiveUsage(c.Request.Context(), accountID)
	} else {
		usage, err = h.accountUsageService.GetUsage(c.Request.Context(), accountID)
	}
	if err != nil {
		response.ErrorFrom(c, err)
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
		response.ErrorFrom(c, err)
		return
	}

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account))
}

// ResetQuota handles resetting account quota usage
// POST /api/v1/admin/accounts/:id/reset-quota
func (h *AccountHandler) ResetQuota(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	if err := h.adminService.ResetAccountQuota(c.Request.Context(), accountID); err != nil {
		response.InternalError(c, "Failed to reset account quota: "+err.Error())
		return
	}

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account))
}

// GetTempUnschedulable handles getting temporary unschedulable status
// GET /api/v1/admin/accounts/:id/temp-unschedulable
func (h *AccountHandler) GetTempUnschedulable(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	state, err := h.rateLimitService.GetTempUnschedStatus(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if state == nil || state.UntilUnix <= time.Now().Unix() {
		response.Success(c, gin.H{"active": false})
		return
	}

	response.Success(c, gin.H{
		"active": true,
		"state":  state,
	})
}

// ClearTempUnschedulable handles clearing temporary unschedulable status
// DELETE /api/v1/admin/accounts/:id/temp-unschedulable
func (h *AccountHandler) ClearTempUnschedulable(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	if err := h.rateLimitService.ClearTempUnschedulable(c.Request.Context(), accountID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Temp unschedulable cleared successfully"})
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
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, stats)
}

// BatchTodayStatsRequest 批量今日统计请求体。
type BatchTodayStatsRequest struct {
	AccountIDs []int64 `json:"account_ids" binding:"required"`
}

// GetBatchTodayStats 批量获取多个账号的今日统计。
// POST /api/v1/admin/accounts/today-stats/batch
func (h *AccountHandler) GetBatchTodayStats(c *gin.Context) {
	var req BatchTodayStatsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	accountIDs := normalizeInt64IDList(req.AccountIDs)
	if len(accountIDs) == 0 {
		response.Success(c, gin.H{"stats": map[string]any{}})
		return
	}

	cacheKey := buildAccountTodayStatsBatchCacheKey(accountIDs)
	if cached, ok := accountTodayStatsBatchCache.Get(cacheKey); ok {
		if cached.ETag != "" {
			c.Header("ETag", cached.ETag)
			c.Header("Vary", "If-None-Match")
			if ifNoneMatchMatched(c.GetHeader("If-None-Match"), cached.ETag) {
				c.Status(http.StatusNotModified)
				return
			}
		}
		c.Header("X-Snapshot-Cache", "hit")
		response.Success(c, cached.Payload)
		return
	}

	stats, err := h.accountUsageService.GetTodayStatsBatch(c.Request.Context(), accountIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	payload := gin.H{"stats": stats}
	cached := accountTodayStatsBatchCache.Set(cacheKey, payload)
	if cached.ETag != "" {
		c.Header("ETag", cached.ETag)
		c.Header("Vary", "If-None-Match")
	}
	c.Header("X-Snapshot-Cache", "miss")
	response.Success(c, payload)
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
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account))
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
		// OpenAI 自动透传会绕过常规模型改写，测试/模型列表也应回落到默认模型集。
		if account.IsOpenAIPassthroughEnabled() {
			response.Success(c, openai.DefaultModels)
			return
		}

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

	// Handle Gemini accounts
	if account.IsGemini() {
		// For OAuth accounts: return default Gemini models
		if account.IsOAuth() {
			response.Success(c, geminicli.DefaultModels)
			return
		}

		// For API Key accounts: return models based on model_mapping
		mapping := account.GetModelMapping()
		if len(mapping) == 0 {
			response.Success(c, geminicli.DefaultModels)
			return
		}

		var models []geminicli.Model
		for requestedModel := range mapping {
			var found bool
			for _, dm := range geminicli.DefaultModels {
				if dm.ID == requestedModel {
					models = append(models, dm)
					found = true
					break
				}
			}
			if !found {
				models = append(models, geminicli.Model{
					ID:          requestedModel,
					Type:        "model",
					DisplayName: requestedModel,
					CreatedAt:   "",
				})
			}
		}
		response.Success(c, models)
		return
	}

	// Handle Antigravity accounts: return Claude + Gemini models
	if account.Platform == service.PlatformAntigravity {
		// 直接复用 antigravity.DefaultModels()，与 /v1/models 端点保持同步
		response.Success(c, antigravity.DefaultModels())
		return
	}

	// Handle Sora accounts
	if account.Platform == service.PlatformSora {
		response.Success(c, service.DefaultSoraModels(nil))
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

// SetPrivacy handles setting privacy for a single OpenAI/Antigravity OAuth account
// POST /api/v1/admin/accounts/:id/set-privacy
func (h *AccountHandler) SetPrivacy(c *gin.Context) {
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
	if account.Type != service.AccountTypeOAuth {
		response.BadRequest(c, "Only OAuth accounts support privacy setting")
		return
	}
	var mode string
	switch account.Platform {
	case service.PlatformOpenAI:
		mode = h.adminService.ForceOpenAIPrivacy(c.Request.Context(), account)
	case service.PlatformAntigravity:
		mode = h.adminService.ForceAntigravityPrivacy(c.Request.Context(), account)
	default:
		response.BadRequest(c, "Only OpenAI and Antigravity OAuth accounts support privacy setting")
		return
	}
	if mode == "" {
		response.BadRequest(c, "Cannot set privacy: missing access_token")
		return
	}
	// 从 DB 重新读取以确保返回最新状态
	updated, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		// 隐私已设置成功但读取失败，回退到内存更新
		if account.Extra == nil {
			account.Extra = make(map[string]any)
		}
		account.Extra["privacy_mode"] = mode
		response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account))
		return
	}
	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), updated))
}

// RefreshTier handles refreshing Google One tier for a single account
// POST /api/v1/admin/accounts/:id/refresh-tier
func (h *AccountHandler) RefreshTier(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	ctx := c.Request.Context()
	account, err := h.adminService.GetAccount(ctx, accountID)
	if err != nil {
		response.NotFound(c, "Account not found")
		return
	}

	if account.Platform != service.PlatformGemini || account.Type != service.AccountTypeOAuth {
		response.BadRequest(c, "Only Gemini OAuth accounts support tier refresh")
		return
	}

	oauthType, _ := account.Credentials["oauth_type"].(string)
	if oauthType != "google_one" {
		response.BadRequest(c, "Only google_one OAuth accounts support tier refresh")
		return
	}

	tierID, extra, creds, err := h.geminiOAuthService.RefreshAccountGoogleOneTier(ctx, account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	_, updateErr := h.adminService.UpdateAccount(ctx, accountID, &service.UpdateAccountInput{
		Credentials: creds,
		Extra:       extra,
	})
	if updateErr != nil {
		response.ErrorFrom(c, updateErr)
		return
	}

	response.Success(c, gin.H{
		"tier_id":             tierID,
		"storage_info":        extra,
		"drive_storage_limit": extra["drive_storage_limit"],
		"drive_storage_usage": extra["drive_storage_usage"],
		"updated_at":          extra["drive_tier_updated_at"],
	})
}

// BatchRefreshTierRequest represents batch tier refresh request
type BatchRefreshTierRequest struct {
	AccountIDs []int64 `json:"account_ids"`
}

// BatchRefreshTier handles batch refreshing Google One tier
// POST /api/v1/admin/accounts/batch-refresh-tier
func (h *AccountHandler) BatchRefreshTier(c *gin.Context) {
	var req BatchRefreshTierRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = BatchRefreshTierRequest{}
	}

	ctx := c.Request.Context()
	accounts := make([]*service.Account, 0)

	if len(req.AccountIDs) == 0 {
		allAccounts, _, err := h.adminService.ListAccounts(ctx, 1, 10000, "gemini", "oauth", "", "", 0, "")
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		for i := range allAccounts {
			acc := &allAccounts[i]
			oauthType, _ := acc.Credentials["oauth_type"].(string)
			if oauthType == "google_one" {
				accounts = append(accounts, acc)
			}
		}
	} else {
		fetched, err := h.adminService.GetAccountsByIDs(ctx, req.AccountIDs)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}

		for _, acc := range fetched {
			if acc == nil {
				continue
			}
			if acc.Platform != service.PlatformGemini || acc.Type != service.AccountTypeOAuth {
				continue
			}
			oauthType, _ := acc.Credentials["oauth_type"].(string)
			if oauthType != "google_one" {
				continue
			}
			accounts = append(accounts, acc)
		}
	}

	const maxConcurrency = 10
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrency)

	var mu sync.Mutex
	var successCount, failedCount int
	var errors []gin.H

	for _, account := range accounts {
		acc := account // 闭包捕获
		g.Go(func() error {
			_, extra, creds, err := h.geminiOAuthService.RefreshAccountGoogleOneTier(gctx, acc)
			if err != nil {
				mu.Lock()
				failedCount++
				errors = append(errors, gin.H{
					"account_id": acc.ID,
					"error":      err.Error(),
				})
				mu.Unlock()
				return nil
			}

			_, updateErr := h.adminService.UpdateAccount(gctx, acc.ID, &service.UpdateAccountInput{
				Credentials: creds,
				Extra:       extra,
			})

			mu.Lock()
			if updateErr != nil {
				failedCount++
				errors = append(errors, gin.H{
					"account_id": acc.ID,
					"error":      updateErr.Error(),
				})
			} else {
				successCount++
			}
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	results := gin.H{
		"total":   len(accounts),
		"success": successCount,
		"failed":  failedCount,
		"errors":  errors,
	}

	response.Success(c, results)
}

// GetAntigravityDefaultModelMapping 获取 Antigravity 平台的默认模型映射
// GET /api/v1/admin/accounts/antigravity/default-model-mapping
func (h *AccountHandler) GetAntigravityDefaultModelMapping(c *gin.Context) {
	response.Success(c, domain.DefaultAntigravityModelMapping)
}

// sanitizeExtraBaseRPM 对 extra map 中的 base_rpm 值进行范围校验和归一化。
// 负值归零，超过 10000 截断为 10000。extra 为 nil 或不含 base_rpm 时无操作。
func sanitizeExtraBaseRPM(extra map[string]any) {
	if extra == nil {
		return
	}
	raw, ok := extra["base_rpm"]
	if !ok {
		return
	}
	v := service.ParseExtraInt(raw)
	if v < 0 {
		v = 0
	} else if v > 10000 {
		v = 10000
	}
	extra["base_rpm"] = v
}

package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

func (h *AccountHandler) usageViewerAllowedAccountIDs(c *gin.Context) ([]int64, bool, error) {
	role, ok := middleware.GetUserRoleFromContext(c)
	if !ok || role != service.RoleUsageViewer {
		return nil, false, nil
	}

	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return nil, true, infraerrors.Unauthorized("UNAUTHORIZED", "user not found in context")
	}

	user, err := h.adminService.GetUser(c.Request.Context(), subject.UserID)
	if err != nil {
		return nil, true, err
	}
	return normalizeInt64IDList(user.AllowedAccounts), true, nil
}

func filterAllowedAccountIDs(requestedIDs []int64, allowedIDs []int64) []int64 {
	if len(requestedIDs) == 0 || len(allowedIDs) == 0 {
		return []int64{}
	}
	allowed := make(map[int64]struct{}, len(allowedIDs))
	for _, id := range allowedIDs {
		if id > 0 {
			allowed[id] = struct{}{}
		}
	}
	out := make([]int64, 0, len(requestedIDs))
	for _, id := range requestedIDs {
		if _, ok := allowed[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

func (h *AccountHandler) ensureUsageViewerAccountAllowed(c *gin.Context, accountID int64) bool {
	allowedIDs, scoped, err := h.usageViewerAllowedAccountIDs(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	if !scoped {
		return true
	}
	if usageViewerAccountIDAllowed(allowedIDs, accountID) {
		return true
	}
	response.ErrorFrom(c, errUsageViewerAccountForbidden)
	return false
}

func usageViewerAccountIDAllowed(allowedIDs []int64, accountID int64) bool {
	for _, id := range allowedIDs {
		if id == accountID {
			return true
		}
	}
	return false
}

// List handles listing all accounts with pagination.
// GET /api/v1/admin/accounts
func (h *AccountHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	platform := c.Query("platform")
	accountType := c.Query("type")
	status := c.Query("status")
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 100 {
		search = search[:100]
	}
	privacyMode := strings.TrimSpace(c.Query("privacy_mode"))
	sortBy := c.DefaultQuery("sort_by", "name")
	sortOrder := c.DefaultQuery("sort_order", "asc")
	lite := parseBoolQueryWithDefault(c.Query("lite"), false)

	groupID, ok := parseAccountListGroupID(c)
	if !ok {
		return
	}

	allowedIDs, scoped, err := h.usageViewerAllowedAccountIDs(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if scoped {
		h.listUsageViewerAccounts(c, allowedIDs, usageViewerAccountListParams{
			page: page, pageSize: pageSize, platform: platform, accountType: accountType,
			status: status, search: search, groupID: groupID, privacyMode: privacyMode,
			sortBy: sortBy, sortOrder: sortOrder, lite: lite,
		})
		return
	}

	accounts, total, err := h.adminService.ListAccounts(c.Request.Context(), page, pageSize, platform, accountType, status, search, groupID, privacyMode, sortBy, sortOrder)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.respondAccountList(c, accounts, total, page, pageSize, platform, accountType, status, search, lite)
}

type usageViewerAccountListParams struct {
	page        int
	pageSize    int
	platform    string
	accountType string
	status      string
	search      string
	groupID     int64
	privacyMode string
	sortBy      string
	sortOrder   string
	lite        bool
}

func parseAccountListGroupID(c *gin.Context) (int64, bool) {
	groupIDStr := c.Query("group")
	if groupIDStr == "" {
		return 0, true
	}
	if groupIDStr == accountListGroupUngroupedQueryValue {
		return service.AccountListGroupUngrouped, true
	}
	groupID, err := parsePositiveAccountListGroupID(groupIDStr)
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_FILTER", "invalid group filter"))
		return 0, false
	}
	return groupID, true
}

func parsePositiveAccountListGroupID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 0 {
		return 0, fmt.Errorf("invalid group filter")
	}
	return id, nil
}

func (h *AccountHandler) listUsageViewerAccounts(c *gin.Context, allowedIDs []int64, params usageViewerAccountListParams) {
	if len(allowedIDs) == 0 {
		response.Paginated(c, []AccountWithConcurrency{}, 0, params.page, params.pageSize)
		return
	}
	accounts, err := h.adminService.GetAccountsByIDs(c.Request.Context(), allowedIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	filtered := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		if accountMatchesUsageViewerListFilters(account, params) {
			filtered = append(filtered, *account)
		}
	}
	pageAccounts, total := paginateUsageViewerAccounts(filtered, params.page, params.pageSize, params.sortBy, params.sortOrder)
	h.respondUsageViewerAccountList(c, pageAccounts, total, params)
}

func (h *AccountHandler) respondAccountList(
	c *gin.Context,
	accounts []service.Account,
	total int64,
	page int,
	pageSize int,
	platform string,
	accountType string,
	status string,
	search string,
	lite bool,
) {
	accountIDs := make([]int64, len(accounts))
	for i, acc := range accounts {
		accountIDs[i] = acc.ID
	}

	concurrencyCounts := map[int64]int{}
	if h.concurrencyService != nil {
		if cc, err := h.concurrencyService.GetAccountConcurrencyBatch(c.Request.Context(), accountIDs); err == nil && cc != nil {
			concurrencyCounts = cc
		}
	}

	windowCosts := h.accountWindowCosts(c, accounts)
	activeSessions := h.accountActiveSessions(c, accounts)
	rpmCounts := h.accountRPMCounts(c, accounts)

	result := make([]AccountWithConcurrency, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		result[i] = AccountWithConcurrency{
			Account:            dto.AccountFromService(acc),
			CurrentConcurrency: concurrencyCounts[acc.ID],
		}
		if cost, ok := windowCosts[acc.ID]; ok {
			result[i].CurrentWindowCost = &cost
		}
		if count, ok := activeSessions[acc.ID]; ok {
			result[i].ActiveSessions = &count
		}
		if rpm, ok := rpmCounts[acc.ID]; ok {
			result[i].CurrentRPM = &rpm
		}
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

func (h *AccountHandler) respondUsageViewerAccountList(c *gin.Context, accounts []service.Account, total int64, params usageViewerAccountListParams) {
	accountIDs := make([]int64, len(accounts))
	for i, acc := range accounts {
		accountIDs[i] = acc.ID
	}

	concurrencyCounts := map[int64]int{}
	if h.concurrencyService != nil {
		if cc, err := h.concurrencyService.GetAccountConcurrencyBatch(c.Request.Context(), accountIDs); err == nil && cc != nil {
			concurrencyCounts = cc
		}
	}

	windowCosts := h.accountWindowCosts(c, accounts)
	activeSessions := h.accountActiveSessions(c, accounts)
	rpmCounts := h.accountRPMCounts(c, accounts)

	result := make([]UsageViewerAccountWithConcurrency, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		result[i] = h.buildUsageViewerAccountResponse(acc, concurrencyCounts, windowCosts, activeSessions, rpmCounts)
	}

	etag := buildUsageViewerAccountsListETag(result, total, params)
	if etag != "" {
		c.Header("ETag", etag)
		c.Header("Vary", "If-None-Match")
		if ifNoneMatchMatched(c.GetHeader("If-None-Match"), etag) {
			c.Status(http.StatusNotModified)
			return
		}
	}
	response.Paginated(c, result, total, params.page, params.pageSize)
}

func (h *AccountHandler) buildUsageViewerAccountResponseWithRuntime(ctx context.Context, account *service.Account) UsageViewerAccountWithConcurrency {
	item := UsageViewerAccountWithConcurrency{
		UsageViewerAccount: dto.UsageViewerAccountFromService(account),
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
			if rpms, err := h.rpmCache.GetRPMBatch(ctx, []int64{account.ID}); err == nil {
				if rpm, ok := rpms[account.ID]; ok {
					item.CurrentRPM = &rpm
				}
			}
		}
	}
	return item
}

func (h *AccountHandler) buildUsageViewerAccountResponse(
	account *service.Account,
	concurrencyCounts map[int64]int,
	windowCosts map[int64]float64,
	activeSessions map[int64]int,
	rpmCounts map[int64]int,
) UsageViewerAccountWithConcurrency {
	item := UsageViewerAccountWithConcurrency{
		UsageViewerAccount: dto.UsageViewerAccountFromService(account),
	}
	if account == nil {
		return item
	}
	item.CurrentConcurrency = concurrencyCounts[account.ID]
	if cost, ok := windowCosts[account.ID]; ok {
		item.CurrentWindowCost = &cost
	}
	if count, ok := activeSessions[account.ID]; ok {
		item.ActiveSessions = &count
	}
	if rpm, ok := rpmCounts[account.ID]; ok {
		item.CurrentRPM = &rpm
	}
	return item
}

func buildUsageViewerAccountsListETag(items []UsageViewerAccountWithConcurrency, total int64, params usageViewerAccountListParams) string {
	payload := struct {
		Total       int64                               `json:"total"`
		Page        int                                 `json:"page"`
		PageSize    int                                 `json:"page_size"`
		Platform    string                              `json:"platform"`
		AccountType string                              `json:"type"`
		Status      string                              `json:"status"`
		Search      string                              `json:"search"`
		Lite        bool                                `json:"lite"`
		Items       []UsageViewerAccountWithConcurrency `json:"items"`
	}{
		Total:       total,
		Page:        params.page,
		PageSize:    params.pageSize,
		Platform:    params.platform,
		AccountType: params.accountType,
		Status:      params.status,
		Search:      params.search,
		Lite:        params.lite,
		Items:       items,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "\"" + hex.EncodeToString(sum[:]) + "\""
}

func (h *AccountHandler) accountWindowCosts(c *gin.Context, accounts []service.Account) map[int64]float64 {
	if h.accountUsageService == nil {
		return nil
	}
	out := make(map[int64]float64)
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(c.Request.Context())
	g.SetLimit(10)
	for i := range accounts {
		acc := &accounts[i]
		if !acc.IsAnthropicOAuthOrSetupToken() || acc.GetWindowCostLimit() <= 0 {
			continue
		}
		g.Go(func() error {
			stats, err := h.accountUsageService.GetAccountWindowStats(gctx, acc.ID, acc.GetCurrentWindowStartTime())
			if err == nil && stats != nil {
				mu.Lock()
				out[acc.ID] = stats.StandardCost
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()
	return out
}

func (h *AccountHandler) accountActiveSessions(c *gin.Context, accounts []service.Account) map[int64]int {
	if h.sessionLimitCache == nil {
		return nil
	}
	accountIDs := make([]int64, 0)
	idleTimeouts := make(map[int64]time.Duration)
	for i := range accounts {
		acc := &accounts[i]
		if acc.IsAnthropicOAuthOrSetupToken() && acc.GetMaxSessions() > 0 {
			accountIDs = append(accountIDs, acc.ID)
			idleTimeouts[acc.ID] = time.Duration(acc.GetSessionIdleTimeoutMinutes()) * time.Minute
		}
	}
	if len(accountIDs) == 0 {
		return nil
	}
	out, _ := h.sessionLimitCache.GetActiveSessionCountBatch(c.Request.Context(), accountIDs, idleTimeouts)
	return out
}

func (h *AccountHandler) accountRPMCounts(c *gin.Context, accounts []service.Account) map[int64]int {
	if h.rpmCache == nil {
		return nil
	}
	accountIDs := make([]int64, 0)
	for i := range accounts {
		acc := &accounts[i]
		if acc.IsAnthropicOAuthOrSetupToken() && acc.GetBaseRPM() > 0 {
			accountIDs = append(accountIDs, acc.ID)
		}
	}
	if len(accountIDs) == 0 {
		return nil
	}
	out, _ := h.rpmCache.GetRPMBatch(c.Request.Context(), accountIDs)
	return out
}

func accountMatchesUsageViewerListFilters(account *service.Account, params usageViewerAccountListParams) bool {
	if account == nil {
		return false
	}
	if params.platform != "" && account.Platform != params.platform {
		return false
	}
	if params.accountType != "" && account.Type != params.accountType {
		return false
	}
	if !accountMatchesStatusFilter(account, params.status) {
		return false
	}
	if params.search != "" && !strings.Contains(strings.ToLower(account.Name), strings.ToLower(params.search)) {
		return false
	}
	if !accountMatchesGroupFilter(account, params.groupID) {
		return false
	}
	return accountMatchesPrivacyFilter(account, params.privacyMode)
}

func accountMatchesStatusFilter(account *service.Account, status string) bool {
	if status == "" {
		return true
	}
	now := time.Now()
	switch status {
	case service.StatusActive:
		return account.Status == service.StatusActive &&
			account.Schedulable &&
			(account.RateLimitResetAt == nil || !account.RateLimitResetAt.After(now)) &&
			(account.TempUnschedulableUntil == nil || !account.TempUnschedulableUntil.After(now))
	case "rate_limited":
		return account.Status == service.StatusActive &&
			account.RateLimitResetAt != nil &&
			account.RateLimitResetAt.After(now) &&
			(account.TempUnschedulableUntil == nil || !account.TempUnschedulableUntil.After(now))
	case "temp_unschedulable":
		return account.Status == service.StatusActive &&
			account.TempUnschedulableUntil != nil &&
			account.TempUnschedulableUntil.After(now)
	case "unschedulable":
		return account.Status == service.StatusActive &&
			!account.Schedulable &&
			(account.RateLimitResetAt == nil || !account.RateLimitResetAt.After(now)) &&
			(account.TempUnschedulableUntil == nil || !account.TempUnschedulableUntil.After(now))
	default:
		return account.Status == status
	}
}

func accountMatchesGroupFilter(account *service.Account, groupID int64) bool {
	switch {
	case groupID == 0:
		return true
	case groupID == service.AccountListGroupUngrouped:
		return len(account.GroupIDs) == 0
	case groupID > 0:
		for _, id := range account.GroupIDs {
			if id == groupID {
				return true
			}
		}
	}
	return false
}

func accountMatchesPrivacyFilter(account *service.Account, privacyMode string) bool {
	if privacyMode == "" {
		return true
	}
	value := ""
	if account.Extra != nil {
		if raw, ok := account.Extra["privacy_mode"]; ok && raw != nil {
			value = strings.TrimSpace(fmt.Sprint(raw))
		}
	}
	if privacyMode == service.AccountPrivacyModeUnsetFilter {
		return value == ""
	}
	return value == privacyMode
}

func paginateUsageViewerAccounts(accounts []service.Account, page, pageSize int, sortBy, sortOrder string) ([]service.Account, int64) {
	sortUsageViewerAccounts(accounts, sortBy, sortOrder)
	total := int64(len(accounts))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= len(accounts) {
		return []service.Account{}, total
	}
	end := start + pageSize
	if end > len(accounts) {
		end = len(accounts)
	}
	return accounts[start:end], total
}

func sortUsageViewerAccounts(accounts []service.Account, sortBy, sortOrder string) {
	desc := strings.EqualFold(sortOrder, "desc")
	sort.SliceStable(accounts, func(i, j int) bool {
		if desc {
			return usageViewerAccountLess(accounts[j], accounts[i], sortBy)
		}
		return usageViewerAccountLess(accounts[i], accounts[j], sortBy)
	})
}

func usageViewerAccountLess(a, b service.Account, sortBy string) bool {
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "id":
		return a.ID < b.ID
	case "status":
		if a.Status == b.Status {
			return a.ID < b.ID
		}
		return a.Status < b.Status
	case "schedulable":
		if a.Schedulable == b.Schedulable {
			return a.ID < b.ID
		}
		return !a.Schedulable && b.Schedulable
	case "priority":
		if a.Priority == b.Priority {
			return a.ID < b.ID
		}
		return a.Priority < b.Priority
	case "rate_multiplier":
		av, bv := a.BillingRateMultiplier(), b.BillingRateMultiplier()
		if av == bv {
			return a.ID < b.ID
		}
		return av < bv
	case "last_used_at":
		return timePtrLess(a.LastUsedAt, b.LastUsedAt, a.ID, b.ID)
	case "expires_at":
		return timePtrLess(a.ExpiresAt, b.ExpiresAt, a.ID, b.ID)
	case "created_at":
		if a.CreatedAt.Equal(b.CreatedAt) {
			return a.ID < b.ID
		}
		return a.CreatedAt.Before(b.CreatedAt)
	default:
		if a.Name == b.Name {
			return a.ID < b.ID
		}
		return a.Name < b.Name
	}
}

func timePtrLess(a, b *time.Time, aID, bID int64) bool {
	switch {
	case a == nil && b == nil:
		return aID < bID
	case a == nil:
		return false
	case b == nil:
		return true
	case a.Equal(*b):
		return aID < bID
	default:
		return a.Before(*b)
	}
}

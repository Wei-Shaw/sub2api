package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UsageHandler handles usage-related requests
type UsageHandler struct {
	usageService   *service.UsageService
	usageReadService *service.UsageReadService
	apiKeyService  *service.APIKeyService
	opsService     *service.OpsService
	settingService *service.SettingService
}

// NewUsageHandler creates a new UsageHandler
func NewUsageHandler(
	usageService *service.UsageService,
	usageReadService *service.UsageReadService,
	apiKeyService *service.APIKeyService,
	opsService *service.OpsService,
	settingService *service.SettingService,
) *UsageHandler {
	if usageReadService == nil {
		usageReadService = service.NewUsageReadService(usageService)
	}
	return &UsageHandler{
		usageService:   usageService,
		usageReadService: usageReadService,
		apiKeyService:  apiKeyService,
		opsService:     opsService,
		settingService: settingService,
	}
}

func (h *UsageHandler) resolveReadUserID(c *gin.Context, localUserID int64) (int64, bool, error) {
	readUserID, ok, err := h.usageReadService.ResolveReadUserID(c.Request.Context(), localUserID)
	if err != nil {
		return 0, false, err
	}
	return readUserID, ok, nil
}

func (h *UsageHandler) resolveReadAPIKeyID(c *gin.Context, localAPIKey *service.APIKey, readUserID int64) (int64, bool, error) {
	readAPIKeyID, ok, err := h.usageReadService.ResolveReadAPIKeyID(c.Request.Context(), localAPIKey, readUserID)
	if err != nil {
		return 0, false, err
	}
	return readAPIKeyID, ok, nil
}

// List handles listing usage records with pagination
// GET /api/v1/usage
func (h *UsageHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	page, pageSize := response.ParsePagination(c)

	var apiKeyID int64
	var localAPIKey *service.APIKey
	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		id, err := strconv.ParseInt(apiKeyIDStr, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid api_key_id")
			return
		}

		// [Security Fix] Verify API Key ownership to prevent horizontal privilege escalation
		apiKey, err := h.apiKeyService.GetByID(c.Request.Context(), id)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if apiKey.UserID != subject.UserID {
			response.Forbidden(c, "Not authorized to access this API key's usage records")
			return
		}

		localAPIKey = apiKey
		apiKeyID = id
	}

	// Parse additional filters
	model := c.Query("model")

	var requestType *int16
	var stream *bool
	if requestTypeStr := strings.TrimSpace(c.Query("request_type")); requestTypeStr != "" {
		parsed, err := service.ParseUsageRequestType(requestTypeStr)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		value := int16(parsed)
		requestType = &value
	} else if streamStr := c.Query("stream"); streamStr != "" {
		val, err := strconv.ParseBool(streamStr)
		if err != nil {
			response.BadRequest(c, "Invalid stream value, use true or false")
			return
		}
		stream = &val
	}

	var billingType *int8
	if billingTypeStr := c.Query("billing_type"); billingTypeStr != "" {
		val, err := strconv.ParseInt(billingTypeStr, 10, 8)
		if err != nil {
			response.BadRequest(c, "Invalid billing_type")
			return
		}
		bt := int8(val)
		billingType = &bt
	}

	// Parse date range
	var startTime, endTime *time.Time
	userTZ := c.Query("timezone") // Get user's timezone from request
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		t, err := timezone.ParseInUserLocation("2006-01-02", startDateStr, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid start_date format, use YYYY-MM-DD")
			return
		}
		startTime = &t
	}

	if endDateStr := c.Query("end_date"); endDateStr != "" {
		t, err := timezone.ParseInUserLocation("2006-01-02", endDateStr, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
			return
		}
		// Use half-open range [start, end), move to next calendar day start (DST-safe).
		t = t.AddDate(0, 0, 1)
		endTime = &t
	}

	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}

	readUserID, ok, err := h.resolveReadUserID(c, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !ok {
		response.Paginated(c, []dto.UsageLog{}, 0, page, pageSize)
		return
	}
	if localAPIKey != nil {
		readAPIKeyID, found, err := h.resolveReadAPIKeyID(c, localAPIKey, readUserID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if !found {
			response.Paginated(c, []dto.UsageLog{}, 0, page, pageSize)
			return
		}
		apiKeyID = readAPIKeyID
	}

	filters := usagestats.UsageLogFilters{
		UserID:      readUserID, // Always filter by current user for security
		APIKeyID:    apiKeyID,
		Model:       model,
		RequestType: requestType,
		Stream:      stream,
		BillingType: billingType,
		StartTime:   startTime,
		EndTime:     endTime,
	}

	records, result, err := h.usageReadService.ListWithFilters(c.Request.Context(), params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.UsageLog, 0, len(records))
	for i := range records {
		out = append(out, *dto.UsageLogFromService(&records[i]))
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}

// ListErrors handles listing the current user's failed requests (redacted).
// GET /api/v1/usage/errors
func (h *UsageHandler) ListErrors(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	// Visibility switch (fail-closed). Defense-in-depth: frontend also hides the tab.
	if h.settingService == nil || !h.settingService.IsUserErrorViewAllowed(c.Request.Context()) {
		response.Forbidden(c, "Error requests view is disabled")
		return
	}
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}

	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		pageSize = 100
	}

	filter := &service.OpsErrorLogFilter{Page: page, PageSize: pageSize}

	// Date range (half-open [start, end)), reuse usage-list semantics.
	userTZ := c.Query("timezone")
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		t, err := timezone.ParseInUserLocation("2006-01-02", startDateStr, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid start_date format, use YYYY-MM-DD")
			return
		}
		filter.StartTime = &t
	}
	if endDateStr := c.Query("end_date"); endDateStr != "" {
		t, err := timezone.ParseInUserLocation("2006-01-02", endDateStr, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
			return
		}
		t = t.AddDate(0, 0, 1)
		filter.EndTime = &t
	}

	filter.Model = strings.TrimSpace(c.Query("model"))

	if k := strings.TrimSpace(c.Query("api_key_id")); k != "" {
		n, err := strconv.ParseInt(k, 10, 64)
		if err != nil || n < 0 {
			response.BadRequest(c, "Invalid api_key_id")
			return
		}
		if n > 0 {
			filter.APIKeyID = &n
		}
	}

	if sc := strings.TrimSpace(c.Query("status_code")); sc != "" {
		n, err := strconv.Atoi(sc)
		if err != nil || n < 0 {
			response.BadRequest(c, "Invalid status_code")
			return
		}
		filter.StatusCodes = []int{n}
	}

	if cat := strings.TrimSpace(c.Query("category")); cat != "" {
		phases, types := service.CategoryToFilter(cat)
		filter.ErrorPhasesAny = phases
		filter.ErrorTypesAny = types
	}

	result, err := h.opsService.ListUserErrorRequests(c.Request.Context(), subject.UserID, filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, result.Items, int64(result.Total), result.Page, result.PageSize)
}

// GetErrorDetail handles fetching one of the current user's failed-request details (redacted).
// GET /api/v1/usage/errors/:id
func (h *UsageHandler) GetErrorDetail(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if h.settingService == nil || !h.settingService.IsUserErrorViewAllowed(c.Request.Context()) {
		response.Forbidden(c, "Error requests view is disabled")
		return
	}
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid id")
		return
	}
	detail, err := h.opsService.GetUserErrorRequestDetail(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, detail)
}

// GetByID handles getting a single usage record
// GET /api/v1/usage/:id
func (h *UsageHandler) GetByID(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	usageID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid usage ID")
		return
	}

	record, err := h.usageReadService.GetByID(c.Request.Context(), usageID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 验证所有权
	readUserID, ok, err := h.resolveReadUserID(c, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !ok || record.UserID != readUserID {
		response.Forbidden(c, "Not authorized to access this record")
		return
	}

	response.Success(c, dto.UsageLogFromService(record))
}

// Stats handles getting usage statistics
// GET /api/v1/usage/stats
func (h *UsageHandler) Stats(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var apiKeyID int64
	var localAPIKey *service.APIKey
	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		id, err := strconv.ParseInt(apiKeyIDStr, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid api_key_id")
			return
		}

		// [Security Fix] Verify API Key ownership to prevent horizontal privilege escalation
		apiKey, err := h.apiKeyService.GetByID(c.Request.Context(), id)
		if err != nil {
			response.NotFound(c, "API key not found")
			return
		}
		if apiKey.UserID != subject.UserID {
			response.Forbidden(c, "Not authorized to access this API key's statistics")
			return
		}

		localAPIKey = apiKey
		apiKeyID = id
	}

	// 获取时间范围参数
	userTZ := c.Query("timezone") // Get user's timezone from request
	now := timezone.NowInUserLocation(userTZ)
	var startTime, endTime time.Time

	// 优先使用 start_date 和 end_date 参数
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	if startDateStr != "" && endDateStr != "" {
		// 使用自定义日期范围
		var err error
		startTime, err = timezone.ParseInUserLocation("2006-01-02", startDateStr, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid start_date format, use YYYY-MM-DD")
			return
		}
		endTime, err = timezone.ParseInUserLocation("2006-01-02", endDateStr, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
			return
		}
		// 与 SQL 条件 created_at < end 对齐，使用次日 00:00 作为上边界（DST-safe）。
		endTime = endTime.AddDate(0, 0, 1)
	} else {
		// 使用 period 参数
		period := c.DefaultQuery("period", "today")
		switch period {
		case "today":
			startTime = timezone.StartOfDayInUserLocation(now, userTZ)
		case "week":
			startTime = now.AddDate(0, 0, -7)
		case "month":
			startTime = now.AddDate(0, -1, 0)
		default:
			startTime = timezone.StartOfDayInUserLocation(now, userTZ)
		}
		endTime = now
	}

	var stats *service.UsageStats
	var err error
	readUserID, ok, err := h.resolveReadUserID(c, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !ok {
		response.Success(c, &service.UsageStats{})
		return
	}
	if localAPIKey != nil {
		readAPIKeyID, found, err := h.resolveReadAPIKeyID(c, localAPIKey, readUserID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if !found {
			response.Success(c, &service.UsageStats{})
			return
		}
		apiKeyID = readAPIKeyID
	}
	if apiKeyID > 0 {
		stats, err = h.usageReadService.GetStatsByAPIKey(c.Request.Context(), apiKeyID, startTime, endTime)
	} else {
		stats, err = h.usageReadService.GetStatsByUser(c.Request.Context(), readUserID, startTime, endTime)
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, stats)
}

// parseUserTimeRange parses start_date, end_date query parameters for user dashboard
// Uses user's timezone if provided, otherwise falls back to server timezone
func parseUserTimeRange(c *gin.Context) (time.Time, time.Time) {
	userTZ := c.Query("timezone") // Get user's timezone from request
	now := timezone.NowInUserLocation(userTZ)
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	var startTime, endTime time.Time

	if startDate != "" {
		if t, err := timezone.ParseInUserLocation("2006-01-02", startDate, userTZ); err == nil {
			startTime = t
		} else {
			startTime = timezone.StartOfDayInUserLocation(now.AddDate(0, 0, -7), userTZ)
		}
	} else {
		startTime = timezone.StartOfDayInUserLocation(now.AddDate(0, 0, -7), userTZ)
	}

	if endDate != "" {
		if t, err := timezone.ParseInUserLocation("2006-01-02", endDate, userTZ); err == nil {
			endTime = t.Add(24 * time.Hour) // Include the end date
		} else {
			endTime = timezone.StartOfDayInUserLocation(now.AddDate(0, 0, 1), userTZ)
		}
	} else {
		endTime = timezone.StartOfDayInUserLocation(now.AddDate(0, 0, 1), userTZ)
	}

	return startTime, endTime
}

const (
	defaultAPIKeyDailyUsageDays = 30
	maxAPIKeyDailyUsageDays     = 90
)

func parseAPIKeyDailyUsageDays(raw string) (int, bool) {
	if strings.TrimSpace(raw) == "" {
		return defaultAPIKeyDailyUsageDays, true
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days <= 0 || days > maxAPIKeyDailyUsageDays {
		return 0, false
	}
	return days, true
}

func apiKeyDailyUsageRange(days int, userTZ string) (time.Time, time.Time) {
	now := timezone.NowInUserLocation(userTZ)
	startTime := timezone.StartOfDayInUserLocation(now.AddDate(0, 0, -(days-1)), userTZ)
	endTime := timezone.StartOfDayInUserLocation(now.AddDate(0, 0, 1), userTZ)
	return startTime, endTime
}

// DashboardStats handles getting user dashboard statistics
// GET /api/v1/usage/dashboard/stats
func (h *UsageHandler) DashboardStats(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	readUserID, ok, err := h.resolveReadUserID(c, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !ok {
		response.Success(c, &usagestats.UserDashboardStats{})
		return
	}

	stats, err := h.usageReadService.GetUserDashboardStats(c.Request.Context(), readUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, stats)
}

// DashboardTrend handles getting user usage trend data
// GET /api/v1/usage/dashboard/trend
func (h *UsageHandler) DashboardTrend(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	startTime, endTime := parseUserTimeRange(c)
	granularity := c.DefaultQuery("granularity", "day")

	readUserID, ok, err := h.resolveReadUserID(c, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !ok {
		response.Success(c, gin.H{
			"trend":       []usagestats.TrendDataPoint{},
			"start_date":  startTime.Format("2006-01-02"),
			"end_date":    endTime.Add(-24 * time.Hour).Format("2006-01-02"),
			"granularity": granularity,
		})
		return
	}

	trend, err := h.usageReadService.GetUserUsageTrendByUserID(c.Request.Context(), readUserID, startTime, endTime, granularity)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"trend":       trend,
		"start_date":  startTime.Format("2006-01-02"),
		"end_date":    endTime.Add(-24 * time.Hour).Format("2006-01-02"),
		"granularity": granularity,
	})
}

// DashboardModels handles getting user model usage statistics
// GET /api/v1/usage/dashboard/models
func (h *UsageHandler) DashboardModels(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	startTime, endTime := parseUserTimeRange(c)

	readUserID, ok, err := h.resolveReadUserID(c, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !ok {
		response.Success(c, gin.H{
			"models":     []usagestats.ModelStat{},
			"start_date": startTime.Format("2006-01-02"),
			"end_date":   endTime.Add(-24 * time.Hour).Format("2006-01-02"),
		})
		return
	}

	stats, err := h.usageReadService.GetUserModelStats(c.Request.Context(), readUserID, startTime, endTime)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"models":     stats,
		"start_date": startTime.Format("2006-01-02"),
		"end_date":   endTime.Add(-24 * time.Hour).Format("2006-01-02"),
	})
}

// BatchAPIKeysUsageRequest represents the request for batch API keys usage
type BatchAPIKeysUsageRequest struct {
	APIKeyIDs []int64 `json:"api_key_ids" binding:"required"`
}

type todayUsageLeaderboardItem struct {
	Rank        int     `json:"rank"`
	MaskedEmail string  `json:"masked_email"`
	Requests    int64   `json:"requests"`
	Tokens      int64   `json:"tokens"`
	ActualCost  float64 `json:"actual_cost"`
}

type todayUsageLeaderboardResponse struct {
	Items           []todayUsageLeaderboardItem `json:"items"`
	TotalActualCost float64                     `json:"total_actual_cost"`
	TotalRequests   int64                       `json:"total_requests"`
	TotalTokens     int64                       `json:"total_tokens"`
	StartDate       string                      `json:"start_date"`
	EndDate         string                      `json:"end_date"`
}

// TodayLeaderboard handles getting today's public user usage leaderboard.
// GET /api/v1/usage/leaderboard/today
func (h *UsageHandler) TodayLeaderboard(c *gin.Context) {
	if _, ok := middleware2.GetAuthSubjectFromContext(c); !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	userTZ := c.Query("timezone")
	now := timezone.NowInUserLocation(userTZ)
	startTime := timezone.StartOfDayInUserLocation(now, userTZ)
	endTime := startTime.AddDate(0, 0, 1)

	ranking, err := h.usageReadService.GetUserSpendingRanking(c.Request.Context(), startTime, endTime, 10)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	rows := ranking.Ranking
	if len(rows) > 10 {
		rows = rows[:10]
	}
	items := make([]todayUsageLeaderboardItem, 0, len(rows))
	for i, row := range rows {
		items = append(items, todayUsageLeaderboardItem{
			Rank:        i + 1,
			MaskedEmail: maskUsageLeaderboardEmail(row.Email),
			Requests:    row.Requests,
			Tokens:      row.Tokens,
			ActualCost:  row.ActualCost,
		})
	}

	response.Success(c, todayUsageLeaderboardResponse{
		Items:           items,
		TotalActualCost: ranking.TotalActualCost,
		TotalRequests:   ranking.TotalRequests,
		TotalTokens:     ranking.TotalTokens,
		StartDate:       startTime.Format("2006-01-02"),
		EndDate:         startTime.Format("2006-01-02"),
	})
}

func maskUsageLeaderboardEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return "hidden"
	}

	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return maskUsageLeaderboardIdentifier(email)
	}

	return maskUsageLeaderboardIdentifier(parts[0]) + "@" + parts[1]
}

func maskUsageLeaderboardIdentifier(value string) string {
	const mask = "*****"

	value = strings.TrimSpace(value)
	runes := []rune(value)
	switch n := len(runes); {
	case n == 0:
		return mask
	case n == 1:
		return string(runes[:1]) + mask
	case n <= 6:
		return string(runes[:1]) + mask + string(runes[n-1:])
	default:
		return string(runes[:3]) + mask + string(runes[n-3:])
	}
}

// DashboardAPIKeysUsage handles getting usage stats for user's own API keys
// POST /api/v1/usage/dashboard/api-keys-usage
func (h *UsageHandler) DashboardAPIKeysUsage(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req BatchAPIKeysUsageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if len(req.APIKeyIDs) == 0 {
		response.Success(c, gin.H{"stats": map[string]any{}})
		return
	}

	// Limit the number of API key IDs to prevent SQL parameter overflow
	if len(req.APIKeyIDs) > 100 {
		response.BadRequest(c, "Too many API key IDs (maximum 100 allowed)")
		return
	}

	validAPIKeyIDs, err := h.apiKeyService.VerifyOwnership(c.Request.Context(), subject.UserID, req.APIKeyIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if len(validAPIKeyIDs) == 0 {
		response.Success(c, gin.H{"stats": map[string]any{}})
		return
	}

	readAPIKeyIDs := validAPIKeyIDs
	readToLocalAPIKeyID := make(map[int64]int64, len(validAPIKeyIDs))
	if h.usageReadService.IsRemoteUsageSource() {
		readUserID, ok, err := h.resolveReadUserID(c, subject.UserID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if !ok {
			response.Success(c, gin.H{"stats": map[string]any{}})
			return
		}
		readAPIKeyIDs = make([]int64, 0, len(validAPIKeyIDs))
		for _, localID := range validAPIKeyIDs {
			localAPIKey, err := h.apiKeyService.GetByID(c.Request.Context(), localID)
			if err != nil {
				response.ErrorFrom(c, err)
				return
			}
			readID, ok, err := h.resolveReadAPIKeyID(c, localAPIKey, readUserID)
			if err != nil {
				response.ErrorFrom(c, err)
				return
			}
			if !ok {
				continue
			}
			readAPIKeyIDs = append(readAPIKeyIDs, readID)
			readToLocalAPIKeyID[readID] = localID
		}
		if len(readAPIKeyIDs) == 0 {
			response.Success(c, gin.H{"stats": map[string]any{}})
			return
		}
	}

	stats, err := h.usageReadService.GetBatchAPIKeyUsageStats(c.Request.Context(), readAPIKeyIDs, time.Time{}, time.Time{})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if len(readToLocalAPIKeyID) > 0 {
		remapped := make(map[int64]*usagestats.BatchAPIKeyUsageStats, len(stats))
		for readID, stat := range stats {
			localID, ok := readToLocalAPIKeyID[readID]
			if !ok || stat == nil {
				continue
			}
			copyStat := *stat
			copyStat.APIKeyID = localID
			remapped[localID] = &copyStat
		}
		stats = remapped
	}

	response.Success(c, gin.H{"stats": stats})
}

// GetMyAPIKeyDailyUsage handles getting daily usage details for the current user's API key.
// GET /api/v1/user/api-keys/:id/usage/daily?days=30
func (h *UsageHandler) GetMyAPIKeyDailyUsage(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	apiKeyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}

	days, ok := parseAPIKeyDailyUsageDays(c.DefaultQuery("days", ""))
	if !ok {
		response.BadRequest(c, "Invalid days, allowed range is 1-90")
		return
	}

	if h.apiKeyService == nil {
		response.InternalError(c, "API key service is not configured")
		return
	}

	apiKey, err := h.apiKeyService.GetByID(c.Request.Context(), apiKeyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if apiKey.UserID != subject.UserID {
		response.Forbidden(c, "Not authorized to access this API key's usage")
		return
	}

	userTZ := c.Query("timezone")
	startTime, endTime := apiKeyDailyUsageRange(days, userTZ)
	readUserID, ok, err := h.resolveReadUserID(c, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !ok {
		response.Success(c, gin.H{
			"items":      []usagestats.APIKeyDailyUsagePoint{},
			"days":       days,
			"start_date": startTime.Format("2006-01-02"),
			"end_date":   endTime.AddDate(0, 0, -1).Format("2006-01-02"),
		})
		return
	}
	readAPIKeyID, ok, err := h.resolveReadAPIKeyID(c, apiKey, readUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !ok {
		response.Success(c, gin.H{
			"items":      []usagestats.APIKeyDailyUsagePoint{},
			"days":       days,
			"start_date": startTime.Format("2006-01-02"),
			"end_date":   endTime.AddDate(0, 0, -1).Format("2006-01-02"),
		})
		return
	}

	items, err := h.usageReadService.GetAPIKeyDailyUsage(c.Request.Context(), readUserID, readAPIKeyID, startTime, endTime)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"items":      items,
		"days":       days,
		"start_date": startTime.Format("2006-01-02"),
		"end_date":   endTime.AddDate(0, 0, -1).Format("2006-01-02"),
	})
}

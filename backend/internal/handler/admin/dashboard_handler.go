package admin

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// DashboardHandler handles admin dashboard statistics
type DashboardHandler struct {
	dashboardService   *service.DashboardService
	aggregationService *service.DashboardAggregationService
	startTime          time.Time // Server start time for uptime calculation
}

// NewDashboardHandler creates a new admin dashboard handler
func NewDashboardHandler(dashboardService *service.DashboardService, aggregationService *service.DashboardAggregationService) *DashboardHandler {
	return &DashboardHandler{
		dashboardService:   dashboardService,
		aggregationService: aggregationService,
		startTime:          time.Now(),
	}
}

// parseTimeRange parses start_date, end_date query parameters
// Uses user's timezone if provided, otherwise falls back to server timezone
func parseTimeRange(c *gin.Context) (time.Time, time.Time) {
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

// GetStats handles getting dashboard statistics
// GET /api/v1/admin/dashboard/stats
func (h *DashboardHandler) GetStats(c *gin.Context) {
	stats, err := h.dashboardService.GetDashboardStats(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "Failed to get dashboard statistics")
		return
	}

	// Calculate uptime in seconds
	uptime := int64(time.Since(h.startTime).Seconds())

	response.Success(c, gin.H{
		// 用户统计
		"total_users":     stats.TotalUsers,
		"today_new_users": stats.TodayNewUsers,
		"active_users":    stats.ActiveUsers,

		// API Key 统计
		"total_api_keys":  stats.TotalAPIKeys,
		"active_api_keys": stats.ActiveAPIKeys,

		// 账户统计
		"total_accounts":     stats.TotalAccounts,
		"normal_accounts":    stats.NormalAccounts,
		"error_accounts":     stats.ErrorAccounts,
		"ratelimit_accounts": stats.RateLimitAccounts,
		"overload_accounts":  stats.OverloadAccounts,

		// 累计 Token 使用统计
		"total_requests":              stats.TotalRequests,
		"total_input_tokens":          stats.TotalInputTokens,
		"total_output_tokens":         stats.TotalOutputTokens,
		"total_cache_creation_tokens": stats.TotalCacheCreationTokens,
		"total_cache_read_tokens":     stats.TotalCacheReadTokens,
		"total_tokens":                stats.TotalTokens,
		"total_cost":                  stats.TotalCost,       // 标准计费
		"total_actual_cost":           stats.TotalActualCost, // 实际扣除

		// 今日 Token 使用统计
		"today_requests":              stats.TodayRequests,
		"today_input_tokens":          stats.TodayInputTokens,
		"today_output_tokens":         stats.TodayOutputTokens,
		"today_cache_creation_tokens": stats.TodayCacheCreationTokens,
		"today_cache_read_tokens":     stats.TodayCacheReadTokens,
		"today_tokens":                stats.TodayTokens,
		"today_cost":                  stats.TodayCost,       // 今日标准计费
		"today_actual_cost":           stats.TodayActualCost, // 今日实际扣除

		// 系统运行统计
		"average_duration_ms": stats.AverageDurationMs,
		"uptime":              uptime,

		// 性能指标
		"rpm": stats.Rpm,
		"tpm": stats.Tpm,

		// 预聚合新鲜度
		"hourly_active_users": stats.HourlyActiveUsers,
		"stats_updated_at":    stats.StatsUpdatedAt,
		"stats_stale":         stats.StatsStale,
	})
}

type DashboardAggregationBackfillRequest struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// BackfillAggregation handles triggering aggregation backfill
// POST /api/v1/admin/dashboard/aggregation/backfill
func (h *DashboardHandler) BackfillAggregation(c *gin.Context) {
	if h.aggregationService == nil {
		response.InternalError(c, "Aggregation service not available")
		return
	}

	var req DashboardAggregationBackfillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	start, err := time.Parse(time.RFC3339, req.Start)
	if err != nil {
		response.BadRequest(c, "Invalid start time")
		return
	}
	end, err := time.Parse(time.RFC3339, req.End)
	if err != nil {
		response.BadRequest(c, "Invalid end time")
		return
	}

	if err := h.aggregationService.TriggerBackfill(start, end); err != nil {
		if errors.Is(err, service.ErrDashboardBackfillDisabled) {
			response.Forbidden(c, "Backfill is disabled")
			return
		}
		if errors.Is(err, service.ErrDashboardBackfillTooLarge) {
			response.BadRequest(c, "Backfill range too large")
			return
		}
		response.InternalError(c, "Failed to trigger backfill")
		return
	}

	response.Success(c, gin.H{
		"status": "accepted",
	})
}

// GetRealtimeMetrics handles getting real-time system metrics
// GET /api/v1/admin/dashboard/realtime
func (h *DashboardHandler) GetRealtimeMetrics(c *gin.Context) {
	// Return mock data for now
	response.Success(c, gin.H{
		"active_requests":       0,
		"requests_per_minute":   0,
		"average_response_time": 0,
		"error_rate":            0.0,
	})
}

// GetUsageTrend handles getting usage trend data
// GET /api/v1/admin/dashboard/trend
// Query params: start_date, end_date (YYYY-MM-DD), granularity (day/hour), user_id, api_key_id, model, account_id, group_id, request_type, stream, billing_type
func (h *DashboardHandler) GetUsageTrend(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)
	granularity := c.DefaultQuery("granularity", "day")

	// Parse optional filter params
	var userID, apiKeyID, accountID, groupID int64
	var model string
	var requestType *int16
	var stream *bool
	var billingType *int8

	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if id, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = id
		}
	}
	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		if id, err := strconv.ParseInt(apiKeyIDStr, 10, 64); err == nil {
			apiKeyID = id
		}
	}
	if accountIDStr := c.Query("account_id"); accountIDStr != "" {
		if id, err := strconv.ParseInt(accountIDStr, 10, 64); err == nil {
			accountID = id
		}
	}
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if id, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil {
			groupID = id
		}
	}
	if modelStr := c.Query("model"); modelStr != "" {
		model = modelStr
	}
	if requestTypeStr := strings.TrimSpace(c.Query("request_type")); requestTypeStr != "" {
		parsed, err := service.ParseUsageRequestType(requestTypeStr)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		value := int16(parsed)
		requestType = &value
	} else if streamStr := c.Query("stream"); streamStr != "" {
		if streamVal, err := strconv.ParseBool(streamStr); err == nil {
			stream = &streamVal
		} else {
			response.BadRequest(c, "Invalid stream value, use true or false")
			return
		}
	}
	if billingTypeStr := c.Query("billing_type"); billingTypeStr != "" {
		if v, err := strconv.ParseInt(billingTypeStr, 10, 8); err == nil {
			bt := int8(v)
			billingType = &bt
		} else {
			response.BadRequest(c, "Invalid billing_type")
			return
		}
	}

	trend, hit, err := h.getUsageTrendCached(c.Request.Context(), startTime, endTime, granularity, userID, apiKeyID, accountID, groupID, model, requestType, stream, billingType)
	if err != nil {
		response.Error(c, 500, "Failed to get usage trend")
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))

	response.Success(c, gin.H{
		"trend":       trend,
		"start_date":  startTime.Format("2006-01-02"),
		"end_date":    endTime.Add(-24 * time.Hour).Format("2006-01-02"),
		"granularity": granularity,
	})
}

// GetModelStats handles getting model usage statistics
// GET /api/v1/admin/dashboard/models
// Query params: start_date, end_date (YYYY-MM-DD), user_id, api_key_id, account_id, group_id, request_type, stream, billing_type
func (h *DashboardHandler) GetModelStats(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)

	// Parse optional filter params
	var userID, apiKeyID, accountID, groupID int64
	modelSource := usagestats.ModelSourceRequested
	var requestType *int16
	var stream *bool
	var billingType *int8

	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if id, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = id
		}
	}
	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		if id, err := strconv.ParseInt(apiKeyIDStr, 10, 64); err == nil {
			apiKeyID = id
		}
	}
	if accountIDStr := c.Query("account_id"); accountIDStr != "" {
		if id, err := strconv.ParseInt(accountIDStr, 10, 64); err == nil {
			accountID = id
		}
	}
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if id, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil {
			groupID = id
		}
	}
	if rawModelSource := strings.TrimSpace(c.Query("model_source")); rawModelSource != "" {
		if !usagestats.IsValidModelSource(rawModelSource) {
			response.BadRequest(c, "Invalid model_source, use requested/upstream/mapping")
			return
		}
		modelSource = rawModelSource
	}
	if requestTypeStr := strings.TrimSpace(c.Query("request_type")); requestTypeStr != "" {
		parsed, err := service.ParseUsageRequestType(requestTypeStr)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		value := int16(parsed)
		requestType = &value
	} else if streamStr := c.Query("stream"); streamStr != "" {
		if streamVal, err := strconv.ParseBool(streamStr); err == nil {
			stream = &streamVal
		} else {
			response.BadRequest(c, "Invalid stream value, use true or false")
			return
		}
	}
	if billingTypeStr := c.Query("billing_type"); billingTypeStr != "" {
		if v, err := strconv.ParseInt(billingTypeStr, 10, 8); err == nil {
			bt := int8(v)
			billingType = &bt
		} else {
			response.BadRequest(c, "Invalid billing_type")
			return
		}
	}

	stats, hit, err := h.getModelStatsCached(c.Request.Context(), startTime, endTime, userID, apiKeyID, accountID, groupID, modelSource, requestType, stream, billingType)
	if err != nil {
		response.Error(c, 500, "Failed to get model statistics")
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))

	response.Success(c, gin.H{
		"models":     stats,
		"start_date": startTime.Format("2006-01-02"),
		"end_date":   endTime.Add(-24 * time.Hour).Format("2006-01-02"),
	})
}

// GetGroupStats handles getting group usage statistics
// GET /api/v1/admin/dashboard/groups
// Query params: start_date, end_date (YYYY-MM-DD), user_id, api_key_id, account_id, group_id, request_type, stream, billing_type
func (h *DashboardHandler) GetGroupStats(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)

	var userID, apiKeyID, accountID, groupID int64
	var requestType *int16
	var stream *bool
	var billingType *int8

	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if id, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = id
		}
	}
	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		if id, err := strconv.ParseInt(apiKeyIDStr, 10, 64); err == nil {
			apiKeyID = id
		}
	}
	if accountIDStr := c.Query("account_id"); accountIDStr != "" {
		if id, err := strconv.ParseInt(accountIDStr, 10, 64); err == nil {
			accountID = id
		}
	}
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if id, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil {
			groupID = id
		}
	}
	if requestTypeStr := strings.TrimSpace(c.Query("request_type")); requestTypeStr != "" {
		parsed, err := service.ParseUsageRequestType(requestTypeStr)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		value := int16(parsed)
		requestType = &value
	} else if streamStr := c.Query("stream"); streamStr != "" {
		if streamVal, err := strconv.ParseBool(streamStr); err == nil {
			stream = &streamVal
		} else {
			response.BadRequest(c, "Invalid stream value, use true or false")
			return
		}
	}
	if billingTypeStr := c.Query("billing_type"); billingTypeStr != "" {
		if v, err := strconv.ParseInt(billingTypeStr, 10, 8); err == nil {
			bt := int8(v)
			billingType = &bt
		} else {
			response.BadRequest(c, "Invalid billing_type")
			return
		}
	}

	stats, hit, err := h.getGroupStatsCached(c.Request.Context(), startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType)
	if err != nil {
		response.Error(c, 500, "Failed to get group statistics")
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))

	response.Success(c, gin.H{
		"groups":     stats,
		"start_date": startTime.Format("2006-01-02"),
		"end_date":   endTime.Add(-24 * time.Hour).Format("2006-01-02"),
	})
}

// GetAPIKeyUsageTrend handles getting API key usage trend data
// GET /api/v1/admin/dashboard/api-keys-trend
// Query params: start_date, end_date (YYYY-MM-DD), granularity (day/hour), limit (default 5)
func (h *DashboardHandler) GetAPIKeyUsageTrend(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)
	granularity := c.DefaultQuery("granularity", "day")
	limitStr := c.DefaultQuery("limit", "5")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 5
	}

	trend, hit, err := h.getAPIKeyUsageTrendCached(c.Request.Context(), startTime, endTime, granularity, limit)
	if err != nil {
		response.Error(c, 500, "Failed to get API key usage trend")
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))

	response.Success(c, gin.H{
		"trend":       trend,
		"start_date":  startTime.Format("2006-01-02"),
		"end_date":    endTime.Add(-24 * time.Hour).Format("2006-01-02"),
		"granularity": granularity,
	})
}

// GetUserUsageTrend handles getting user usage trend data
// GET /api/v1/admin/dashboard/users-trend
// Query params: start_date, end_date (YYYY-MM-DD), granularity (day/hour), limit (default 12)
func (h *DashboardHandler) GetUserUsageTrend(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)
	granularity := c.DefaultQuery("granularity", "day")
	limitStr := c.DefaultQuery("limit", "12")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 12
	}

	trend, hit, err := h.getUserUsageTrendCached(c.Request.Context(), startTime, endTime, granularity, limit)
	if err != nil {
		response.Error(c, 500, "Failed to get user usage trend")
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))

	response.Success(c, gin.H{
		"trend":       trend,
		"start_date":  startTime.Format("2006-01-02"),
		"end_date":    endTime.Add(-24 * time.Hour).Format("2006-01-02"),
		"granularity": granularity,
	})
}

// BatchUsersUsageRequest represents the request body for batch user usage stats
type BatchUsersUsageRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required"`
}

var dashboardUsersRankingCache = newSnapshotCache(5 * time.Minute)
var dashboardBatchUsersUsageCache = newSnapshotCache(30 * time.Second)
var dashboardBatchAPIKeysUsageCache = newSnapshotCache(30 * time.Second)

func parseRankingLimit(raw string) int {
	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || limit <= 0 {
		return 12
	}
	if limit > 50 {
		return 50
	}
	return limit
}

// GetUserSpendingRanking handles getting user spending ranking data.
// GET /api/v1/admin/dashboard/users-ranking
func (h *DashboardHandler) GetUserSpendingRanking(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)
	limit := parseRankingLimit(c.DefaultQuery("limit", "12"))

	keyRaw, _ := json.Marshal(struct {
		Start string `json:"start"`
		End   string `json:"end"`
		Limit int    `json:"limit"`
	}{
		Start: startTime.UTC().Format(time.RFC3339),
		End:   endTime.UTC().Format(time.RFC3339),
		Limit: limit,
	})
	cacheKey := string(keyRaw)
	if cached, ok := dashboardUsersRankingCache.Get(cacheKey); ok {
		c.Header("X-Snapshot-Cache", "hit")
		response.Success(c, cached.Payload)
		return
	}

	ranking, err := h.dashboardService.GetUserSpendingRanking(c.Request.Context(), startTime, endTime, limit)
	if err != nil {
		response.Error(c, 500, "Failed to get user spending ranking")
		return
	}

	payload := gin.H{
		"ranking":           ranking.Ranking,
		"total_actual_cost": ranking.TotalActualCost,
		"total_requests":    ranking.TotalRequests,
		"total_tokens":      ranking.TotalTokens,
		"start_date":        startTime.Format("2006-01-02"),
		"end_date":          endTime.Add(-24 * time.Hour).Format("2006-01-02"),
	}
	dashboardUsersRankingCache.Set(cacheKey, payload)
	c.Header("X-Snapshot-Cache", "miss")
	response.Success(c, payload)
}

// GetBatchUsersUsage handles getting usage stats for multiple users
// POST /api/v1/admin/dashboard/users-usage
func (h *DashboardHandler) GetBatchUsersUsage(c *gin.Context) {
	var req BatchUsersUsageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userIDs := normalizeInt64IDList(req.UserIDs)
	if len(userIDs) == 0 {
		response.Success(c, gin.H{"stats": map[string]any{}})
		return
	}

	keyRaw, _ := json.Marshal(struct {
		UserIDs []int64 `json:"user_ids"`
	}{
		UserIDs: userIDs,
	})
	cacheKey := string(keyRaw)
	if cached, ok := dashboardBatchUsersUsageCache.Get(cacheKey); ok {
		c.Header("X-Snapshot-Cache", "hit")
		response.Success(c, cached.Payload)
		return
	}

	stats, err := h.dashboardService.GetBatchUserUsageStats(c.Request.Context(), userIDs, time.Time{}, time.Time{})
	if err != nil {
		response.Error(c, 500, "Failed to get user usage stats")
		return
	}

	payload := gin.H{"stats": stats}
	dashboardBatchUsersUsageCache.Set(cacheKey, payload)
	c.Header("X-Snapshot-Cache", "miss")
	response.Success(c, payload)
}

// BatchAPIKeysUsageRequest represents the request body for batch api key usage stats
type BatchAPIKeysUsageRequest struct {
	APIKeyIDs []int64 `json:"api_key_ids" binding:"required"`
}

// GetBatchAPIKeysUsage handles getting usage stats for multiple API keys
// POST /api/v1/admin/dashboard/api-keys-usage
func (h *DashboardHandler) GetBatchAPIKeysUsage(c *gin.Context) {
	var req BatchAPIKeysUsageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	apiKeyIDs := normalizeInt64IDList(req.APIKeyIDs)
	if len(apiKeyIDs) == 0 {
		response.Success(c, gin.H{"stats": map[string]any{}})
		return
	}

	keyRaw, _ := json.Marshal(struct {
		APIKeyIDs []int64 `json:"api_key_ids"`
	}{
		APIKeyIDs: apiKeyIDs,
	})
	cacheKey := string(keyRaw)
	if cached, ok := dashboardBatchAPIKeysUsageCache.Get(cacheKey); ok {
		c.Header("X-Snapshot-Cache", "hit")
		response.Success(c, cached.Payload)
		return
	}

	stats, err := h.dashboardService.GetBatchAPIKeyUsageStats(c.Request.Context(), apiKeyIDs, time.Time{}, time.Time{})
	if err != nil {
		response.Error(c, 500, "Failed to get API key usage stats")
		return
	}

	payload := gin.H{"stats": stats}
	dashboardBatchAPIKeysUsageCache.Set(cacheKey, payload)
	c.Header("X-Snapshot-Cache", "miss")
	response.Success(c, payload)
}

// GetUserBreakdown handles getting per-user usage breakdown within a dimension.
// GET /api/v1/admin/dashboard/user-breakdown
// Query params: start_date, end_date, group_id, model, endpoint, endpoint_type, limit
func (h *DashboardHandler) GetUserBreakdown(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)

	dim := usagestats.UserBreakdownDimension{}
	if v := c.Query("group_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			dim.GroupID = id
		}
	}
	dim.Model = c.Query("model")
	rawModelSource := strings.TrimSpace(c.DefaultQuery("model_source", usagestats.ModelSourceRequested))
	if !usagestats.IsValidModelSource(rawModelSource) {
		response.BadRequest(c, "Invalid model_source, use requested/upstream/mapping")
		return
	}
	dim.ModelType = rawModelSource
	dim.Endpoint = c.Query("endpoint")
	dim.EndpointType = c.DefaultQuery("endpoint_type", "inbound")

	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	stats, err := h.dashboardService.GetUserBreakdownStats(
		c.Request.Context(), startTime, endTime, dim, limit,
	)
	if err != nil {
		response.Error(c, 500, "Failed to get user breakdown stats")
		return
	}

	response.Success(c, gin.H{
		"users":      stats,
		"start_date": startTime.Format("2006-01-02"),
		"end_date":   endTime.Add(-24 * time.Hour).Format("2006-01-02"),
	})
}

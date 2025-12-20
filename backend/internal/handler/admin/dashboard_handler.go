package admin

import (
	"strconv"
	"sub2api/internal/pkg/response"
	"sub2api/internal/pkg/timezone"
	"sub2api/internal/repository"
	"time"

	"github.com/gin-gonic/gin"
)

// DashboardHandler handles admin dashboard statistics
type DashboardHandler struct {
	usageRepo *repository.UsageLogRepository
	startTime time.Time // Server start time for uptime calculation
REDACTED

// NewDashboardHandler creates a new admin dashboard handler
func NewDashboardHandler(usageRepo *repository.UsageLogRepository) *DashboardHandler {
	return &DashboardHandler{
		usageRepo: usageRepo,
		startTime: time.Now(),
REDACTED
REDACTED

// parseTimeRange parses start_date, end_date query parameters
func parseTimeRange(c *gin.Context) (time.Time, time.Time) {
	now := timezone.Now()
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	var startTime, endTime time.Time

	if startDate != "" {
		if t, err := timezone.ParseInLocation("2006-01-02", startDate); err == nil {
			startTime = t
	REDACTED else {
			startTime = timezone.StartOfDay(now.AddDate(0, 0, -7))
	REDACTED
REDACTED else {
		startTime = timezone.StartOfDay(now.AddDate(0, 0, -7))
REDACTED

	if endDate != "" {
		if t, err := timezone.ParseInLocation("2006-01-02", endDate); err == nil {
			endTime = t.Add(24 * time.Hour) // Include the end date
	REDACTED else {
			endTime = timezone.StartOfDay(now.AddDate(0, 0, 1))
	REDACTED
REDACTED else {
		endTime = timezone.StartOfDay(now.AddDate(0, 0, 1))
REDACTED

	return startTime, endTime
REDACTED

// GetStats handles getting dashboard statistics
// GET /api/v1/admin/dashboard/stats
func (h *DashboardHandler) GetStats(c *gin.Context) {
	stats, err := h.usageRepo.GetDashboardStats(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "Failed to get dashboard statistics")
		return
REDACTED

	// Calculate uptime in seconds
	uptime := int64(time.Since(h.startTime).Seconds())

	response.Success(c, gin.H{
		// 用户统计
		"total_users":     stats.TotalUsers,
		"today_new_users": stats.TodayNewUsers,
		"active_users":    stats.ActiveUsers,

		// API Key 统计
		"total_api_keys":  stats.TotalApiKeys,
		"active_api_keys": stats.ActiveApiKeys,

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
REDACTED)
REDACTED

// GetRealtimeMetrics handles getting real-time system metrics
// GET /api/v1/admin/dashboard/realtime
func (h *DashboardHandler) GetRealtimeMetrics(c *gin.Context) {
	// Return mock data for now
	response.Success(c, gin.H{
		"active_requests":       0,
		"requests_per_minute":   0,
		"average_response_time": 0,
		"error_rate":            0.0,
REDACTED)
REDACTED

// GetUsageTrend handles getting usage trend data
// GET /api/v1/admin/dashboard/trend
// Query params: start_date, end_date (YYYY-MM-DD), granularity (day/hour), user_id, api_key_id
func (h *DashboardHandler) GetUsageTrend(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)
	granularity := c.DefaultQuery("granularity", "day")

	// Parse optional filter params
	var userID, apiKeyID int64
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if id, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = id
	REDACTED
REDACTED
	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		if id, err := strconv.ParseInt(apiKeyIDStr, 10, 64); err == nil {
			apiKeyID = id
	REDACTED
REDACTED

	trend, err := h.usageRepo.GetUsageTrendWithFilters(c.Request.Context(), startTime, endTime, granularity, userID, apiKeyID)
	if err != nil {
		response.Error(c, 500, "Failed to get usage trend")
		return
REDACTED

	response.Success(c, gin.H{
		"trend":       trend,
		"start_date":  startTime.Format("2006-01-02"),
		"end_date":    endTime.Add(-24 * time.Hour).Format("2006-01-02"),
		"granularity": granularity,
REDACTED)
REDACTED

// GetModelStats handles getting model usage statistics
// GET /api/v1/admin/dashboard/models
// Query params: start_date, end_date (YYYY-MM-DD), user_id, api_key_id
func (h *DashboardHandler) GetModelStats(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)

	// Parse optional filter params
	var userID, apiKeyID int64
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if id, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = id
	REDACTED
REDACTED
	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		if id, err := strconv.ParseInt(apiKeyIDStr, 10, 64); err == nil {
			apiKeyID = id
	REDACTED
REDACTED

	stats, err := h.usageRepo.GetModelStatsWithFilters(c.Request.Context(), startTime, endTime, userID, apiKeyID)
	if err != nil {
		response.Error(c, 500, "Failed to get model statistics")
		return
REDACTED

	response.Success(c, gin.H{
		"models":     stats,
		"start_date": startTime.Format("2006-01-02"),
		"end_date":   endTime.Add(-24 * time.Hour).Format("2006-01-02"),
REDACTED)
REDACTED

// GetApiKeyUsageTrend handles getting API key usage trend data
// GET /api/v1/admin/dashboard/api-keys-trend
// Query params: start_date, end_date (YYYY-MM-DD), granularity (day/hour), limit (default 5)
func (h *DashboardHandler) GetApiKeyUsageTrend(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)
	granularity := c.DefaultQuery("granularity", "day")
	limitStr := c.DefaultQuery("limit", "5")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 5
REDACTED

	trend, err := h.usageRepo.GetApiKeyUsageTrend(c.Request.Context(), startTime, endTime, granularity, limit)
	if err != nil {
		response.Error(c, 500, "Failed to get API key usage trend")
		return
REDACTED

	response.Success(c, gin.H{
		"trend":       trend,
		"start_date":  startTime.Format("2006-01-02"),
		"end_date":    endTime.Add(-24 * time.Hour).Format("2006-01-02"),
		"granularity": granularity,
REDACTED)
REDACTED

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
REDACTED

	trend, err := h.usageRepo.GetUserUsageTrend(c.Request.Context(), startTime, endTime, granularity, limit)
	if err != nil {
		response.Error(c, 500, "Failed to get user usage trend")
		return
REDACTED

	response.Success(c, gin.H{
		"trend":       trend,
		"start_date":  startTime.Format("2006-01-02"),
		"end_date":    endTime.Add(-24 * time.Hour).Format("2006-01-02"),
		"granularity": granularity,
REDACTED)
REDACTED

// BatchUsersUsageRequest represents the request body for batch user usage stats
type BatchUsersUsageRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required"`
REDACTED

// GetBatchUsersUsage handles getting usage stats for multiple users
// POST /api/v1/admin/dashboard/users-usage
func (h *DashboardHandler) GetBatchUsersUsage(c *gin.Context) {
	var req BatchUsersUsageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if len(req.UserIDs) == 0 {
		response.Success(c, gin.H{"stats": map[string]any{REDACTEDREDACTED)
		return
REDACTED

	stats, err := h.usageRepo.GetBatchUserUsageStats(c.Request.Context(), req.UserIDs)
	if err != nil {
		response.Error(c, 500, "Failed to get user usage stats")
		return
REDACTED

	response.Success(c, gin.H{"stats": statsREDACTED)
REDACTED

// BatchApiKeysUsageRequest represents the request body for batch api key usage stats
type BatchApiKeysUsageRequest struct {
	ApiKeyIDs []int64 `json:"api_key_ids" binding:"required"`
REDACTED

// GetBatchApiKeysUsage handles getting usage stats for multiple API keys
// POST /api/v1/admin/dashboard/api-keys-usage
func (h *DashboardHandler) GetBatchApiKeysUsage(c *gin.Context) {
	var req BatchApiKeysUsageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if len(req.ApiKeyIDs) == 0 {
		response.Success(c, gin.H{"stats": map[string]any{REDACTEDREDACTED)
		return
REDACTED

	stats, err := h.usageRepo.GetBatchApiKeyUsageStats(c.Request.Context(), req.ApiKeyIDs)
	if err != nil {
		response.Error(c, 500, "Failed to get API key usage stats")
		return
REDACTED

	response.Success(c, gin.H{"stats": statsREDACTED)
REDACTED

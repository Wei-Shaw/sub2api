package admin

import (
	"strconv"
	"time"

	"sub2api/internal/pkg/pagination"
	"sub2api/internal/pkg/response"
	"sub2api/internal/pkg/timezone"
	"sub2api/internal/repository"
	"sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UsageHandler handles admin usage-related requests
type UsageHandler struct {
	usageRepo    *repository.UsageLogRepository
	apiKeyRepo   *repository.ApiKeyRepository
	usageService *service.UsageService
	adminService service.AdminService
REDACTED

// NewUsageHandler creates a new admin usage handler
func NewUsageHandler(
	usageRepo *repository.UsageLogRepository,
	apiKeyRepo *repository.ApiKeyRepository,
	usageService *service.UsageService,
	adminService service.AdminService,
) *UsageHandler {
	return &UsageHandler{
		usageRepo:    usageRepo,
		apiKeyRepo:   apiKeyRepo,
		usageService: usageService,
		adminService: adminService,
REDACTED
REDACTED

// List handles listing all usage records with filters
// GET /api/v1/admin/usage
func (h *UsageHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)

	// Parse filters
	var userID, apiKeyID int64
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		id, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid user_id")
			return
	REDACTED
		userID = id
REDACTED

	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		id, err := strconv.ParseInt(apiKeyIDStr, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid api_key_id")
			return
	REDACTED
		apiKeyID = id
REDACTED

	// Parse date range
	var startTime, endTime *time.Time
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		t, err := timezone.ParseInLocation("2006-01-02", startDateStr)
		if err != nil {
			response.BadRequest(c, "Invalid start_date format, use YYYY-MM-DD")
			return
	REDACTED
		startTime = &t
REDACTED

	if endDateStr := c.Query("end_date"); endDateStr != "" {
		t, err := timezone.ParseInLocation("2006-01-02", endDateStr)
		if err != nil {
			response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
			return
	REDACTED
		// Set end time to end of day
		t = t.Add(24*time.Hour - time.Nanosecond)
		endTime = &t
REDACTED

	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	filters := repository.UsageLogFilters{
		UserID:    userID,
		ApiKeyID:  apiKeyID,
		StartTime: startTime,
		EndTime:   endTime,
REDACTED

	records, result, err := h.usageRepo.ListWithFilters(c.Request.Context(), params, filters)
	if err != nil {
		response.InternalError(c, "Failed to list usage records: "+err.Error())
		return
REDACTED

	response.Paginated(c, records, result.Total, page, pageSize)
REDACTED

// Stats handles getting usage statistics with filters
// GET /api/v1/admin/usage/stats
func (h *UsageHandler) Stats(c *gin.Context) {
	// Parse filters
	var userID, apiKeyID int64
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		id, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid user_id")
			return
	REDACTED
		userID = id
REDACTED

	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		id, err := strconv.ParseInt(apiKeyIDStr, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid api_key_id")
			return
	REDACTED
		apiKeyID = id
REDACTED

	// Parse date range
	now := timezone.Now()
	var startTime, endTime time.Time

	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	if startDateStr != "" && endDateStr != "" {
		var err error
		startTime, err = timezone.ParseInLocation("2006-01-02", startDateStr)
		if err != nil {
			response.BadRequest(c, "Invalid start_date format, use YYYY-MM-DD")
			return
	REDACTED
		endTime, err = timezone.ParseInLocation("2006-01-02", endDateStr)
		if err != nil {
			response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
			return
	REDACTED
		endTime = endTime.Add(24*time.Hour - time.Nanosecond)
REDACTED else {
		period := c.DefaultQuery("period", "today")
		switch period {
		case "today":
			startTime = timezone.StartOfDay(now)
		case "week":
			startTime = now.AddDate(0, 0, -7)
		case "month":
			startTime = now.AddDate(0, -1, 0)
		default:
			startTime = timezone.StartOfDay(now)
	REDACTED
		endTime = now
REDACTED

	if apiKeyID > 0 {
		stats, err := h.usageService.GetStatsByApiKey(c.Request.Context(), apiKeyID, startTime, endTime)
		if err != nil {
			response.InternalError(c, "Failed to get usage statistics: "+err.Error())
			return
	REDACTED
		response.Success(c, stats)
		return
REDACTED

	if userID > 0 {
		stats, err := h.usageService.GetStatsByUser(c.Request.Context(), userID, startTime, endTime)
		if err != nil {
			response.InternalError(c, "Failed to get usage statistics: "+err.Error())
			return
	REDACTED
		response.Success(c, stats)
		return
REDACTED

	// Get global stats
	stats, err := h.usageRepo.GetGlobalStats(c.Request.Context(), startTime, endTime)
	if err != nil {
		response.InternalError(c, "Failed to get usage statistics: "+err.Error())
		return
REDACTED

	response.Success(c, stats)
REDACTED

// SearchUsers handles searching users by email keyword
// GET /api/v1/admin/usage/search-users
func (h *UsageHandler) SearchUsers(c *gin.Context) {
	keyword := c.Query("q")
	if keyword == "" {
		response.Success(c, []interface{REDACTED{REDACTED)
		return
REDACTED

	// Limit to 30 results
	users, _, err := h.adminService.ListUsers(c.Request.Context(), 1, 30, "", "", keyword)
	if err != nil {
		response.InternalError(c, "Failed to search users: "+err.Error())
		return
REDACTED

	// Return simplified user list (only id and email)
	type SimpleUser struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
REDACTED

	result := make([]SimpleUser, len(users))
	for i, u := range users {
		result[i] = SimpleUser{
			ID:    u.ID,
			Email: u.Email,
	REDACTED
REDACTED

	response.Success(c, result)
REDACTED

// SearchApiKeys handles searching API keys by user
// GET /api/v1/admin/usage/search-api-keys
func (h *UsageHandler) SearchApiKeys(c *gin.Context) {
	userIDStr := c.Query("user_id")
	keyword := c.Query("q")

	var userID int64
	if userIDStr != "" {
		id, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid user_id")
			return
	REDACTED
		userID = id
REDACTED

	keys, err := h.apiKeyRepo.SearchApiKeys(c.Request.Context(), userID, keyword, 30)
	if err != nil {
		response.InternalError(c, "Failed to search API keys: "+err.Error())
		return
REDACTED

	// Return simplified API key list (only id and name)
	type SimpleApiKey struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		UserID int64  `json:"user_id"`
REDACTED

	result := make([]SimpleApiKey, len(keys))
	for i, k := range keys {
		result[i] = SimpleApiKey{
			ID:     k.ID,
			Name:   k.Name,
			UserID: k.UserID,
	REDACTED
REDACTED

	response.Success(c, result)
REDACTED

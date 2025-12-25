package admin

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UsageHandler handles admin usage-related requests
type UsageHandler struct {
	usageService  *service.UsageService
	apiKeyService *service.ApiKeyService
	adminService  service.AdminService
}

// NewUsageHandler creates a new admin usage handler
func NewUsageHandler(
	usageService *service.UsageService,
	apiKeyService *service.ApiKeyService,
	adminService service.AdminService,
) *UsageHandler {
	return &UsageHandler{
		usageService:  usageService,
		apiKeyService: apiKeyService,
		adminService:  adminService,
	}
}

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
		}
		userID = id
	}

	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		id, err := strconv.ParseInt(apiKeyIDStr, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid api_key_id")
			return
		}
		apiKeyID = id
	}

	// Parse date range
	var startTime, endTime *time.Time
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		t, err := timezone.ParseInLocation("2006-01-02", startDateStr)
		if err != nil {
			response.BadRequest(c, "Invalid start_date format, use YYYY-MM-DD")
			return
		}
		startTime = &t
	}

	if endDateStr := c.Query("end_date"); endDateStr != "" {
		t, err := timezone.ParseInLocation("2006-01-02", endDateStr)
		if err != nil {
			response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
			return
		}
		// Set end time to end of day
		t = t.Add(24*time.Hour - time.Nanosecond)
		endTime = &t
	}

	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	filters := usagestats.UsageLogFilters{
		UserID:    userID,
		ApiKeyID:  apiKeyID,
		StartTime: startTime,
		EndTime:   endTime,
	}

	records, result, err := h.usageService.ListWithFilters(c.Request.Context(), params, filters)
	if err != nil {
		response.InternalError(c, "Failed to list usage records: "+err.Error())
		return
	}

	response.Paginated(c, records, result.Total, page, pageSize)
}

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
		}
		userID = id
	}

	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		id, err := strconv.ParseInt(apiKeyIDStr, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid api_key_id")
			return
		}
		apiKeyID = id
	}

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
		}
		endTime, err = timezone.ParseInLocation("2006-01-02", endDateStr)
		if err != nil {
			response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
			return
		}
		endTime = endTime.Add(24*time.Hour - time.Nanosecond)
	} else {
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
		}
		endTime = now
	}

	if apiKeyID > 0 {
		stats, err := h.usageService.GetStatsByApiKey(c.Request.Context(), apiKeyID, startTime, endTime)
		if err != nil {
			response.InternalError(c, "Failed to get usage statistics: "+err.Error())
			return
		}
		response.Success(c, stats)
		return
	}

	if userID > 0 {
		stats, err := h.usageService.GetStatsByUser(c.Request.Context(), userID, startTime, endTime)
		if err != nil {
			response.InternalError(c, "Failed to get usage statistics: "+err.Error())
			return
		}
		response.Success(c, stats)
		return
	}

	// Get global stats
	stats, err := h.usageService.GetGlobalStats(c.Request.Context(), startTime, endTime)
	if err != nil {
		response.InternalError(c, "Failed to get usage statistics: "+err.Error())
		return
	}

	response.Success(c, stats)
}

// SearchUsers handles searching users by email keyword
// GET /api/v1/admin/usage/search-users
func (h *UsageHandler) SearchUsers(c *gin.Context) {
	keyword := c.Query("q")
	if keyword == "" {
		response.Success(c, []any{})
		return
	}

	// Limit to 30 results
	users, _, err := h.adminService.ListUsers(c.Request.Context(), 1, 30, "", "", keyword)
	if err != nil {
		response.InternalError(c, "Failed to search users: "+err.Error())
		return
	}

	// Return simplified user list (only id and email)
	type SimpleUser struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
	}

	result := make([]SimpleUser, len(users))
	for i, u := range users {
		result[i] = SimpleUser{
			ID:    u.ID,
			Email: u.Email,
		}
	}

	response.Success(c, result)
}

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
		}
		userID = id
	}

	keys, err := h.apiKeyService.SearchApiKeys(c.Request.Context(), userID, keyword, 30)
	if err != nil {
		response.InternalError(c, "Failed to search API keys: "+err.Error())
		return
	}

	// Return simplified API key list (only id and name)
	type SimpleApiKey struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		UserID int64  `json:"user_id"`
	}

	result := make([]SimpleApiKey, len(keys))
	for i, k := range keys {
		result[i] = SimpleApiKey{
			ID:     k.ID,
			Name:   k.Name,
			UserID: k.UserID,
		}
	}

	response.Success(c, result)
}

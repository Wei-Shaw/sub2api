package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetDashboardOverview returns vNext ops dashboard overview (raw path).
// GET /api/v1/admin/ops/dashboard/overview
func (h *OpsHandler) GetDashboardOverview(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
	}
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	data, err := h.opsService.GetDashboardOverview(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

// GetDashboardThroughputTrend returns throughput time series (raw path).
// GET /api/v1/admin/ops/dashboard/throughput-trend
func (h *OpsHandler) GetDashboardThroughputTrend(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
	}
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	bucketSeconds := pickThroughputBucketSeconds(endTime.Sub(startTime))
	data, err := h.opsService.GetThroughputTrend(c.Request.Context(), filter, bucketSeconds)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

// GetDashboardLatencyHistogram returns the latency distribution histogram (success requests).
// GET /api/v1/admin/ops/dashboard/latency-histogram
func (h *OpsHandler) GetDashboardLatencyHistogram(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
	}
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	data, err := h.opsService.GetLatencyHistogram(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

// GetDashboardErrorTrend returns error counts time series (raw path).
// GET /api/v1/admin/ops/dashboard/error-trend
func (h *OpsHandler) GetDashboardErrorTrend(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
	}
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	bucketSeconds := pickThroughputBucketSeconds(endTime.Sub(startTime))
	data, err := h.opsService.GetErrorTrend(c.Request.Context(), filter, bucketSeconds)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

// GetDashboardErrorDistribution returns error distribution by status code (raw path).
// GET /api/v1/admin/ops/dashboard/error-distribution
func (h *OpsHandler) GetDashboardErrorDistribution(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
	}
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	data, err := h.opsService.GetErrorDistribution(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

// GetDashboardOpenAITokenStats returns OpenAI token efficiency stats grouped by model.
// GET /api/v1/admin/ops/dashboard/openai-token-stats
func (h *OpsHandler) GetDashboardOpenAITokenStats(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	filter, err := parseOpsOpenAITokenStatsFilter(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	data, err := h.opsService.GetOpenAITokenStats(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

func parseOpsOpenAITokenStatsFilter(c *gin.Context) (*service.OpsOpenAITokenStatsFilter, error) {
	if c == nil {
		return nil, fmt.Errorf("invalid request")
	}

	timeRange := strings.TrimSpace(c.Query("time_range"))
	if timeRange == "" {
		timeRange = "30d"
	}
	dur, ok := parseOpsOpenAITokenStatsDuration(timeRange)
	if !ok {
		return nil, fmt.Errorf("invalid time_range")
	}
	end := time.Now().UTC()
	start := end.Add(-dur)

	filter := &service.OpsOpenAITokenStatsFilter{
		TimeRange: timeRange,
		StartTime: start,
		EndTime:   end,
		Platform:  strings.TrimSpace(c.Query("platform")),
	}

	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid group_id")
		}
		filter.GroupID = &id
	}

	topNRaw := strings.TrimSpace(c.Query("top_n"))
	pageRaw := strings.TrimSpace(c.Query("page"))
	pageSizeRaw := strings.TrimSpace(c.Query("page_size"))
	if topNRaw != "" && (pageRaw != "" || pageSizeRaw != "") {
		return nil, fmt.Errorf("invalid query: top_n cannot be used with page/page_size")
	}

	if topNRaw != "" {
		topN, err := strconv.Atoi(topNRaw)
		if err != nil || topN < 1 || topN > 100 {
			return nil, fmt.Errorf("invalid top_n")
		}
		filter.TopN = topN
		return filter, nil
	}

	filter.Page = 1
	filter.PageSize = 20
	if pageRaw != "" {
		page, err := strconv.Atoi(pageRaw)
		if err != nil || page < 1 {
			return nil, fmt.Errorf("invalid page")
		}
		filter.Page = page
	}
	if pageSizeRaw != "" {
		pageSize, err := strconv.Atoi(pageSizeRaw)
		if err != nil || pageSize < 1 || pageSize > 100 {
			return nil, fmt.Errorf("invalid page_size")
		}
		filter.PageSize = pageSize
	}
	return filter, nil
}

func parseOpsOpenAITokenStatsDuration(v string) (time.Duration, bool) {
	switch strings.TrimSpace(v) {
	case "30m":
		return 30 * time.Minute, true
	case "1h":
		return time.Hour, true
	case "1d":
		return 24 * time.Hour, true
	case "15d":
		return 15 * 24 * time.Hour, true
	case "30d":
		return 30 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

func pickThroughputBucketSeconds(window time.Duration) int {
	// Keep buckets predictable and avoid huge responses.
	switch {
	case window <= 2*time.Hour:
		return 60
	case window <= 24*time.Hour:
		return 300
	default:
		return 3600
	}
}

func parseOpsQueryMode(c *gin.Context) service.OpsQueryMode {
	if c == nil {
		return ""
	}
	raw := strings.TrimSpace(c.Query("mode"))
	if raw == "" {
		// Empty means "use server default" (DB setting ops_query_mode_default).
		return ""
	}
	return service.ParseOpsQueryMode(raw)
}

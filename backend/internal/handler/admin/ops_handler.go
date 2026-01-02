package admin

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// OpsHandler handles ops dashboard endpoints.
type OpsHandler struct {
	opsService *service.OpsService
REDACTED

// NewOpsHandler creates a new OpsHandler.
func NewOpsHandler(opsService *service.OpsService) *OpsHandler {
	return &OpsHandler{opsService: opsServiceREDACTED
REDACTED

// GetMetrics returns the latest ops metrics snapshot.
// GET /api/v1/admin/ops/metrics
func (h *OpsHandler) GetMetrics(c *gin.Context) {
	metrics, err := h.opsService.GetLatestMetrics(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get ops metrics")
		return
REDACTED
	response.Success(c, metrics)
REDACTED

// ListMetricsHistory returns a time-range slice of metrics for charts.
// GET /api/v1/admin/ops/metrics/history
//
// Query params:
// - window_minutes: int (default 1)
// - minutes: int (lookback; optional)
// - start_time/end_time: RFC3339 timestamps (optional; overrides minutes when provided)
// - limit: int (optional; max 100, default 300 for backward compatibility)
func (h *OpsHandler) ListMetricsHistory(c *gin.Context) {
	windowMinutes := 1
	if v := c.Query("window_minutes"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			windowMinutes = parsed
	REDACTED else {
			response.BadRequest(c, "Invalid window_minutes")
			return
	REDACTED
REDACTED

	limit := 300
	limitProvided := false
	if v := c.Query("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 || parsed > 5000 {
			response.BadRequest(c, "Invalid limit (must be 1-5000)")
			return
	REDACTED
		limit = parsed
		limitProvided = true
REDACTED

	endTime := time.Now()
	startTime := time.Time{REDACTED

	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		parsed, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			response.BadRequest(c, "Invalid start_time format (RFC3339)")
			return
	REDACTED
		startTime = parsed
REDACTED
	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		parsed, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			response.BadRequest(c, "Invalid end_time format (RFC3339)")
			return
	REDACTED
		endTime = parsed
REDACTED

	// If explicit range not provided, use lookback minutes.
	if startTime.IsZero() {
		if v := c.Query("minutes"); v != "" {
			minutes, err := strconv.Atoi(v)
			if err != nil || minutes <= 0 {
				response.BadRequest(c, "Invalid minutes")
				return
		REDACTED
			if minutes > 60*24*7 {
				minutes = 60 * 24 * 7
		REDACTED
			startTime = endTime.Add(-time.Duration(minutes) * time.Minute)
	REDACTED
REDACTED

	// Default time range: last 24 hours.
	if startTime.IsZero() {
		startTime = endTime.Add(-24 * time.Hour)
		if !limitProvided {
			// Metrics are collected at 1-minute cadence; 24h requires ~1440 points.
			limit = 24 * 60
	REDACTED
REDACTED

	if startTime.After(endTime) {
		response.BadRequest(c, "Invalid time range: start_time must be <= end_time")
		return
REDACTED

	items, err := h.opsService.ListMetricsHistory(c.Request.Context(), windowMinutes, startTime, endTime, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to list ops metrics history")
		return
REDACTED
	response.Success(c, gin.H{"items": itemsREDACTED)
REDACTED

// ListErrorLogs lists recent error logs with optional filters.
// GET /api/v1/admin/ops/error-logs
//
// Query params:
// - start_time/end_time: RFC3339 timestamps (optional)
// - platform: string (optional)
// - phase: string (optional)
// - severity: string (optional)
// - q: string (optional; fuzzy match)
// - limit: int (optional; default 100; max 500)
func (h *OpsHandler) ListErrorLogs(c *gin.Context) {
	var filters service.OpsErrorLogFilters

	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			response.BadRequest(c, "Invalid start_time format (RFC3339)")
			return
	REDACTED
		filters.StartTime = &startTime
REDACTED
	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			response.BadRequest(c, "Invalid end_time format (RFC3339)")
			return
	REDACTED
		filters.EndTime = &endTime
REDACTED

	if filters.StartTime != nil && filters.EndTime != nil && filters.StartTime.After(*filters.EndTime) {
		response.BadRequest(c, "Invalid time range: start_time must be <= end_time")
		return
REDACTED

	filters.Platform = c.Query("platform")
	filters.Phase = c.Query("phase")
	filters.Severity = c.Query("severity")
	filters.Query = c.Query("q")

	filters.Limit = 100
	if limitStr := c.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 500 {
			response.BadRequest(c, "Invalid limit (must be 1-500)")
			return
	REDACTED
		filters.Limit = limit
REDACTED

	items, total, err := h.opsService.ListErrorLogs(c.Request.Context(), filters)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to list error logs")
		return
REDACTED

	response.Success(c, gin.H{
		"items": items,
		"total": total,
REDACTED)
REDACTED

// GetDashboardOverview returns realtime ops dashboard overview.
// GET /api/v1/admin/ops/dashboard/overview
//
// Query params:
// - time_range: string (optional; default "1h") one of: 5m, 30m, 1h, 6h, 24h
func (h *OpsHandler) GetDashboardOverview(c *gin.Context) {
	timeRange := c.Query("time_range")
	if timeRange == "" {
		timeRange = "1h"
REDACTED

	switch timeRange {
	case "5m", "30m", "1h", "6h", "24h":
	default:
		response.BadRequest(c, "Invalid time_range (supported: 5m, 30m, 1h, 6h, 24h)")
		return
REDACTED

	data, err := h.opsService.GetDashboardOverview(c.Request.Context(), timeRange)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get dashboard overview")
		return
REDACTED
	response.Success(c, data)
REDACTED

// GetProviderHealth returns upstream provider health comparison data.
// GET /api/v1/admin/ops/dashboard/providers
//
// Query params:
// - time_range: string (optional; default "1h") one of: 5m, 30m, 1h, 6h, 24h
func (h *OpsHandler) GetProviderHealth(c *gin.Context) {
	timeRange := c.Query("time_range")
	if timeRange == "" {
		timeRange = "1h"
REDACTED

	switch timeRange {
	case "5m", "30m", "1h", "6h", "24h":
	default:
		response.BadRequest(c, "Invalid time_range (supported: 5m, 30m, 1h, 6h, 24h)")
		return
REDACTED

	providers, err := h.opsService.GetProviderHealth(c.Request.Context(), timeRange)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get provider health")
		return
REDACTED

	var totalRequests int64
	var weightedSuccess float64
	var bestProvider string
	var worstProvider string
	var bestRate float64
	var worstRate float64
	hasRate := false

	for _, p := range providers {
		if p == nil {
			continue
	REDACTED
		totalRequests += p.RequestCount
		weightedSuccess += (p.SuccessRate / 100) * float64(p.RequestCount)

		if p.RequestCount <= 0 {
			continue
	REDACTED
		if !hasRate {
			bestProvider = p.Name
			worstProvider = p.Name
			bestRate = p.SuccessRate
			worstRate = p.SuccessRate
			hasRate = true
			continue
	REDACTED

		if p.SuccessRate > bestRate {
			bestProvider = p.Name
			bestRate = p.SuccessRate
	REDACTED
		if p.SuccessRate < worstRate {
			worstProvider = p.Name
			worstRate = p.SuccessRate
	REDACTED
REDACTED

	avgSuccessRate := 0.0
	if totalRequests > 0 {
		avgSuccessRate = (weightedSuccess / float64(totalRequests)) * 100
		avgSuccessRate = math.Round(avgSuccessRate*100) / 100
REDACTED

	response.Success(c, gin.H{
		"providers": providers,
		"summary": gin.H{
			"total_requests":   totalRequests,
			"avg_success_rate": avgSuccessRate,
			"best_provider":    bestProvider,
			"worst_provider":   worstProvider,
	REDACTED,
REDACTED)
REDACTED

// GetErrorLogs returns a paginated error log list with multi-dimensional filters.
// GET /api/v1/admin/ops/errors
func (h *OpsHandler) GetErrorLogs(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)

	filter := &service.ErrorLogFilter{
		Page:     page,
		PageSize: pageSize,
REDACTED

	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			response.BadRequest(c, "Invalid start_time format (RFC3339)")
			return
	REDACTED
		filter.StartTime = &startTime
REDACTED
	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			response.BadRequest(c, "Invalid end_time format (RFC3339)")
			return
	REDACTED
		filter.EndTime = &endTime
REDACTED

	if filter.StartTime != nil && filter.EndTime != nil && filter.StartTime.After(*filter.EndTime) {
		response.BadRequest(c, "Invalid time range: start_time must be <= end_time")
		return
REDACTED

	if errorCodeStr := c.Query("error_code"); errorCodeStr != "" {
		code, err := strconv.Atoi(errorCodeStr)
		if err != nil || code < 0 {
			response.BadRequest(c, "Invalid error_code")
			return
	REDACTED
		filter.ErrorCode = &code
REDACTED

	// Keep both parameter names for compatibility: provider (docs) and platform (legacy).
	filter.Provider = c.Query("provider")
	if filter.Provider == "" {
		filter.Provider = c.Query("platform")
REDACTED

	if accountIDStr := c.Query("account_id"); accountIDStr != "" {
		accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
		if err != nil || accountID <= 0 {
			response.BadRequest(c, "Invalid account_id")
			return
	REDACTED
		filter.AccountID = &accountID
REDACTED

	out, err := h.opsService.GetErrorLogs(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get error logs")
		return
REDACTED

	response.Success(c, gin.H{
		"errors":    out.Errors,
		"total":     out.Total,
		"page":      out.Page,
		"page_size": out.PageSize,
REDACTED)
REDACTED

// GetLatencyHistogram returns the latency distribution histogram.
// GET /api/v1/admin/ops/dashboard/latency-histogram
func (h *OpsHandler) GetLatencyHistogram(c *gin.Context) {
	timeRange := c.Query("time_range")
	if timeRange == "" {
		timeRange = "1h"
REDACTED

	buckets, err := h.opsService.GetLatencyHistogram(c.Request.Context(), timeRange)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get latency histogram")
		return
REDACTED

	totalRequests := int64(0)
	for _, b := range buckets {
		totalRequests += b.Count
REDACTED

	response.Success(c, gin.H{
		"buckets":                buckets,
		"total_requests":         totalRequests,
		"slow_request_threshold": 1000,
REDACTED)
REDACTED

// GetErrorDistribution returns the error distribution.
// GET /api/v1/admin/ops/dashboard/errors/distribution
func (h *OpsHandler) GetErrorDistribution(c *gin.Context) {
	timeRange := c.Query("time_range")
	if timeRange == "" {
		timeRange = "1h"
REDACTED

	items, err := h.opsService.GetErrorDistribution(c.Request.Context(), timeRange)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get error distribution")
		return
REDACTED

	response.Success(c, gin.H{
		"items": items,
REDACTED)
REDACTED

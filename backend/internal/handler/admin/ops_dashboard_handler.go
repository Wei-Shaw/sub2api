package admin

import (
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
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
REDACTED
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
	REDACTED
		filter.GroupID = &id
REDACTED

	data, err := h.opsService.GetDashboardOverview(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, data)
REDACTED

// GetDashboardThroughputTrend returns throughput time series (raw path).
// GET /api/v1/admin/ops/dashboard/throughput-trend
func (h *OpsHandler) GetDashboardThroughputTrend(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
REDACTED
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
	REDACTED
		filter.GroupID = &id
REDACTED

	bucketSeconds := pickThroughputBucketSeconds(endTime.Sub(startTime))
	data, err := h.opsService.GetThroughputTrend(c.Request.Context(), filter, bucketSeconds)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, data)
REDACTED

// GetDashboardLatencyHistogram returns the latency distribution histogram (success requests).
// GET /api/v1/admin/ops/dashboard/latency-histogram
func (h *OpsHandler) GetDashboardLatencyHistogram(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
REDACTED
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
	REDACTED
		filter.GroupID = &id
REDACTED

	data, err := h.opsService.GetLatencyHistogram(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, data)
REDACTED

// GetDashboardErrorTrend returns error counts time series (raw path).
// GET /api/v1/admin/ops/dashboard/error-trend
func (h *OpsHandler) GetDashboardErrorTrend(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
REDACTED
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
	REDACTED
		filter.GroupID = &id
REDACTED

	bucketSeconds := pickThroughputBucketSeconds(endTime.Sub(startTime))
	data, err := h.opsService.GetErrorTrend(c.Request.Context(), filter, bucketSeconds)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, data)
REDACTED

// GetDashboardErrorDistribution returns error distribution by status code (raw path).
// GET /api/v1/admin/ops/dashboard/error-distribution
func (h *OpsHandler) GetDashboardErrorDistribution(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
REDACTED
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
	REDACTED
		filter.GroupID = &id
REDACTED

	data, err := h.opsService.GetErrorDistribution(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, data)
REDACTED

func pickThroughputBucketSeconds(window time.Duration) int {
	// Keep buckets predictable and avoid huge responses.
	switch {
	case window <= 2*time.Hour:
		return 60
	case window <= 24*time.Hour:
		return 300
	default:
		return 3600
REDACTED
REDACTED

func parseOpsQueryMode(c *gin.Context) service.OpsQueryMode {
	if c == nil {
		return ""
REDACTED
	raw := strings.TrimSpace(c.Query("mode"))
	if raw == "" {
		// Empty means "use server default" (DB setting ops_query_mode_default).
		return ""
REDACTED
	return service.ParseOpsQueryMode(raw)
REDACTED

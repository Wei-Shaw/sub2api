package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetConcurrencyStats returns real-time concurrency usage aggregated by platform/group/account.
// GET /api/v1/admin/ops/concurrency
func (h *OpsHandler) GetConcurrencyStats(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	if !h.opsService.IsRealtimeMonitoringEnabled(c.Request.Context()) {
		response.Success(c, gin.H{
			"enabled":   false,
			"platform":  map[string]*service.PlatformConcurrencyInfo{REDACTED,
			"group":     map[int64]*service.GroupConcurrencyInfo{REDACTED,
			"account":   map[int64]*service.AccountConcurrencyInfo{REDACTED,
			"timestamp": time.Now().UTC(),
	REDACTED)
		return
REDACTED

	platformFilter := strings.TrimSpace(c.Query("platform"))
	var groupID *int64
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
	REDACTED
		groupID = &id
REDACTED

	platform, group, account, collectedAt, err := h.opsService.GetConcurrencyStats(c.Request.Context(), platformFilter, groupID)
	if err != nil {
		if isOpsRealtimeRequestCanceled(c, err) {
			return
	REDACTED
		response.ErrorFrom(c, err)
		return
REDACTED

	payload := gin.H{
		"enabled":  true,
		"platform": platform,
		"group":    group,
		"account":  account,
REDACTED
	if collectedAt != nil {
		payload["timestamp"] = collectedAt.UTC()
REDACTED
	response.Success(c, payload)
REDACTED

// GetUserConcurrencyStats returns real-time concurrency usage for all active users.
// GET /api/v1/admin/ops/user-concurrency
func (h *OpsHandler) GetUserConcurrencyStats(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	if !h.opsService.IsRealtimeMonitoringEnabled(c.Request.Context()) {
		response.Success(c, gin.H{
			"enabled":   false,
			"user":      map[int64]*service.UserConcurrencyInfo{REDACTED,
			"timestamp": time.Now().UTC(),
	REDACTED)
		return
REDACTED

	users, collectedAt, err := h.opsService.GetUserConcurrencyStats(c.Request.Context())
	if err != nil {
		if isOpsRealtimeRequestCanceled(c, err) {
			return
	REDACTED
		response.ErrorFrom(c, err)
		return
REDACTED

	payload := gin.H{
		"enabled": true,
		"user":    users,
REDACTED
	if collectedAt != nil {
		payload["timestamp"] = collectedAt.UTC()
REDACTED
	response.Success(c, payload)
REDACTED

// GetAccountAvailability returns account availability statistics.
// GET /api/v1/admin/ops/account-availability
//
// Query params:
// - platform: optional
// - group_id: optional
func (h *OpsHandler) GetAccountAvailability(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	if !h.opsService.IsRealtimeMonitoringEnabled(c.Request.Context()) {
		response.Success(c, gin.H{
			"enabled":   false,
			"platform":  map[string]*service.PlatformAvailability{REDACTED,
			"group":     map[int64]*service.GroupAvailability{REDACTED,
			"account":   map[int64]*service.AccountAvailability{REDACTED,
			"timestamp": time.Now().UTC(),
	REDACTED)
		return
REDACTED

	platform := strings.TrimSpace(c.Query("platform"))
	var groupID *int64
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
	REDACTED
		groupID = &id
REDACTED

	platformStats, groupStats, accountStats, collectedAt, err := h.opsService.GetAccountAvailabilityStats(c.Request.Context(), platform, groupID)
	if err != nil {
		if isOpsRealtimeRequestCanceled(c, err) {
			return
	REDACTED
		response.ErrorFrom(c, err)
		return
REDACTED

	payload := gin.H{
		"enabled":  true,
		"platform": platformStats,
		"group":    groupStats,
		"account":  accountStats,
REDACTED
	if collectedAt != nil {
		payload["timestamp"] = collectedAt.UTC()
REDACTED
	response.Success(c, payload)
REDACTED

func isOpsRealtimeRequestCanceled(c *gin.Context, err error) bool {
	if err == nil {
		return false
REDACTED
	if errors.Is(err, context.Canceled) {
		return true
REDACTED
	if c != nil && c.Request != nil && errors.Is(c.Request.Context().Err(), context.Canceled) {
		return true
REDACTED
	return strings.Contains(err.Error(), "canceling statement due to user request")
REDACTED

func parseOpsRealtimeWindow(v string) (time.Duration, string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "1min", "1m":
		return 1 * time.Minute, "1min", true
	case "5min", "5m":
		return 5 * time.Minute, "5min", true
	case "30min", "30m":
		return 30 * time.Minute, "30min", true
	case "1h", "60m", "60min":
		return 1 * time.Hour, "1h", true
	default:
		return 0, "", false
REDACTED
REDACTED

// GetRealtimeTrafficSummary returns QPS/TPS current/peak/avg for the selected window.
// GET /api/v1/admin/ops/realtime-traffic
//
// Query params:
// - window: 1min|5min|30min|1h (default: 1min)
// - platform: optional
// - group_id: optional
func (h *OpsHandler) GetRealtimeTrafficSummary(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	windowDur, windowLabel, ok := parseOpsRealtimeWindow(c.Query("window"))
	if !ok {
		response.BadRequest(c, "Invalid window")
		return
REDACTED

	platform := strings.TrimSpace(c.Query("platform"))
	var groupID *int64
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
	REDACTED
		groupID = &id
REDACTED

	endTime := time.Now().UTC()
	startTime := endTime.Add(-windowDur)

	if !h.opsService.IsRealtimeMonitoringEnabled(c.Request.Context()) {
		disabledSummary := &service.OpsRealtimeTrafficSummary{
			Window:    windowLabel,
			StartTime: startTime,
			EndTime:   endTime,
			Platform:  platform,
			GroupID:   groupID,
			QPS:       service.OpsRateSummary{REDACTED,
			TPS:       service.OpsRateSummary{REDACTED,
	REDACTED
		response.Success(c, gin.H{
			"enabled":   false,
			"summary":   disabledSummary,
			"timestamp": endTime,
	REDACTED)
		return
REDACTED

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  platform,
		GroupID:   groupID,
		QueryMode: service.OpsQueryModeRaw,
REDACTED

	summary, err := h.opsService.GetRealtimeTrafficSummary(c.Request.Context(), filter)
	if err != nil {
		if isOpsRealtimeRequestCanceled(c, err) {
			return
	REDACTED
		response.ErrorFrom(c, err)
		return
REDACTED
	if summary != nil {
		summary.Window = windowLabel
REDACTED
	response.Success(c, gin.H{
		"enabled":   true,
		"summary":   summary,
		"timestamp": endTime,
REDACTED)
REDACTED

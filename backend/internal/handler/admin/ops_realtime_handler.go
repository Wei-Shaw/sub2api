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

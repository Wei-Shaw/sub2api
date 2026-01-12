package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetEmailNotificationConfig returns Ops email notification config (DB-backed).
// GET /api/v1/admin/ops/email-notification/config
func (h *OpsHandler) GetEmailNotificationConfig(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	cfg, err := h.opsService.GetEmailNotificationConfig(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get email notification config")
		return
REDACTED
	response.Success(c, cfg)
REDACTED

// UpdateEmailNotificationConfig updates Ops email notification config (DB-backed).
// PUT /api/v1/admin/ops/email-notification/config
func (h *OpsHandler) UpdateEmailNotificationConfig(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	var req service.OpsEmailNotificationConfigUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
REDACTED

	updated, err := h.opsService.UpdateEmailNotificationConfig(c.Request.Context(), &req)
	if err != nil {
		// Most failures here are validation errors from request payload; treat as 400.
		response.Error(c, http.StatusBadRequest, err.Error())
		return
REDACTED
	response.Success(c, updated)
REDACTED

// GetAlertRuntimeSettings returns Ops alert evaluator runtime settings (DB-backed).
// GET /api/v1/admin/ops/runtime/alert
func (h *OpsHandler) GetAlertRuntimeSettings(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	cfg, err := h.opsService.GetOpsAlertRuntimeSettings(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get alert runtime settings")
		return
REDACTED
	response.Success(c, cfg)
REDACTED

// UpdateAlertRuntimeSettings updates Ops alert evaluator runtime settings (DB-backed).
// PUT /api/v1/admin/ops/runtime/alert
func (h *OpsHandler) UpdateAlertRuntimeSettings(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	var req service.OpsAlertRuntimeSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
REDACTED

	updated, err := h.opsService.UpdateOpsAlertRuntimeSettings(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
REDACTED
	response.Success(c, updated)
REDACTED

// GetAdvancedSettings returns Ops advanced settings (DB-backed).
// GET /api/v1/admin/ops/advanced-settings
func (h *OpsHandler) GetAdvancedSettings(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	cfg, err := h.opsService.GetOpsAdvancedSettings(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get advanced settings")
		return
REDACTED
	response.Success(c, cfg)
REDACTED

// UpdateAdvancedSettings updates Ops advanced settings (DB-backed).
// PUT /api/v1/admin/ops/advanced-settings
func (h *OpsHandler) UpdateAdvancedSettings(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	var req service.OpsAdvancedSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
REDACTED

	updated, err := h.opsService.UpdateOpsAdvancedSettings(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
REDACTED
	response.Success(c, updated)
REDACTED

// GetMetricThresholds returns Ops metric thresholds (DB-backed).
// GET /api/v1/admin/ops/settings/metric-thresholds
func (h *OpsHandler) GetMetricThresholds(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	cfg, err := h.opsService.GetMetricThresholds(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get metric thresholds")
		return
REDACTED
	response.Success(c, cfg)
REDACTED

// UpdateMetricThresholds updates Ops metric thresholds (DB-backed).
// PUT /api/v1/admin/ops/settings/metric-thresholds
func (h *OpsHandler) UpdateMetricThresholds(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	var req service.OpsMetricThresholds
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
REDACTED

	updated, err := h.opsService.UpdateMetricThresholds(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
REDACTED
	response.Success(c, updated)
REDACTED


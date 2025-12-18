package admin

import (
	"net/http"

	"sub2api/internal/pkg/response"
	"sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// SystemHandler handles system-related operations
type SystemHandler struct {
	updateSvc *service.UpdateService
REDACTED

// NewSystemHandler creates a new SystemHandler
func NewSystemHandler(rdb *redis.Client, version, buildType string) *SystemHandler {
	return &SystemHandler{
		updateSvc: service.NewUpdateService(rdb, version, buildType),
REDACTED
REDACTED

// GetVersion returns the current version
// GET /api/v1/admin/system/version
func (h *SystemHandler) GetVersion(c *gin.Context) {
	info, _ := h.updateSvc.CheckUpdate(c.Request.Context(), false)
	response.Success(c, gin.H{
		"version": info.CurrentVersion,
REDACTED)
REDACTED

// CheckUpdates checks for available updates
// GET /api/v1/admin/system/check-updates
func (h *SystemHandler) CheckUpdates(c *gin.Context) {
	force := c.Query("force") == "true"
	info, err := h.updateSvc.CheckUpdate(c.Request.Context(), force)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
REDACTED
	response.Success(c, info)
REDACTED

// PerformUpdate downloads and applies the update
// POST /api/v1/admin/system/update
func (h *SystemHandler) PerformUpdate(c *gin.Context) {
	if err := h.updateSvc.PerformUpdate(c.Request.Context()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
REDACTED
	response.Success(c, gin.H{
		"message":      "Update completed. Please restart the service.",
		"need_restart": true,
REDACTED)
REDACTED

// Rollback restores the previous version
// POST /api/v1/admin/system/rollback
func (h *SystemHandler) Rollback(c *gin.Context) {
	if err := h.updateSvc.Rollback(); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
REDACTED
	response.Success(c, gin.H{
		"message":      "Rollback completed. Please restart the service.",
		"need_restart": true,
REDACTED)
REDACTED

// RestartService restarts the systemd service
// POST /api/v1/admin/system/restart
func (h *SystemHandler) RestartService(c *gin.Context) {
	if err := h.updateSvc.RestartService(); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
REDACTED
	response.Success(c, gin.H{
		"message": "Service restart initiated",
REDACTED)
REDACTED

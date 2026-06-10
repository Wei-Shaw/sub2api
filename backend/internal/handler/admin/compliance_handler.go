package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ComplianceHandler struct {
	settingService *service.SettingService
REDACTED

func NewComplianceHandler(settingService *service.SettingService) *ComplianceHandler {
	return &ComplianceHandler{settingService: settingServiceREDACTED
REDACTED

type AcceptAdminComplianceRequest struct {
	Phrase   string `json:"phrase" binding:"required"`
	Language string `json:"language"`
REDACTED

func (h *ComplianceHandler) GetStatus(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	status, err := h.settingService.GetAdminComplianceStatus(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, status)
REDACTED

func (h *ComplianceHandler) Accept(c *gin.Context) {
	var req AcceptAdminComplianceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	status, err := h.settingService.AcceptAdminCompliance(c.Request.Context(), service.AdminComplianceAcceptInput{
		AdminUserID: subject.UserID,
		Phrase:      req.Phrase,
		Language:    req.Language,
		IPAddress:   ip.GetClientIP(c),
		UserAgent:   strings.TrimSpace(c.GetHeader("User-Agent")),
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, status)
REDACTED

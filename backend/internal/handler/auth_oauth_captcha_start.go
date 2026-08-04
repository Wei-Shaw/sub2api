package handler

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type oauthStartCaptchaRequest struct {
	TencentCaptchaTicket  string `json:"tencent_captcha_ticket"`
	TencentCaptchaRandstr string `json:"tencent_captcha_randstr"`
REDACTED

type oauthStartResponse struct {
	AuthorizeURL string `json:"authorize_url"`
REDACTED

func (h *AuthHandler) requireTencentCaptchaForOAuthLoginStart(c *gin.Context) bool {
	if strings.HasSuffix(strings.TrimRight(c.Request.URL.Path, "/"), "/bind/start") {
		return true
REDACTED

	var req oauthStartCaptchaRequest
	if c.Request.Method == http.MethodPost {
		_ = c.ShouldBindJSON(&req)
REDACTED
	if err := h.authService.VerifyTencentCaptchaIfEnabled(c.Request.Context(), service.CaptchaProof{
		TencentTicket:  req.TencentCaptchaTicket,
		TencentRandstr: req.TencentCaptchaRandstr,
REDACTED, ip.GetClientIP(c)); err != nil {
		response.ErrorFrom(c, err)
		return false
REDACTED
	return true
REDACTED

func respondOAuthStart(c *gin.Context, authorizeURL string) {
	if c.Request.Method == http.MethodPost {
		response.Success(c, oauthStartResponse{AuthorizeURL: authorizeURLREDACTED)
		return
REDACTED
	c.Redirect(http.StatusFound, authorizeURL)
REDACTED

package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// CodexModels serves the Codex models manifest for Codex clients.
//
// Codex CLI and the Codex desktop app refresh their model picker from
// GET {base_urlREDACTED/models?client_version=... (custom provider mode) or
// GET /backend-api/codex/models (chatgpt_base_url mode). Both routes land
// here. ChatGPT manifests are proxied verbatim; custom API key manifests receive
// provider-compatibility normalization and use a short-lived, asynchronously
// revalidated cache to tolerate canceled client requests.
func (h *OpenAIGatewayHandler) CodexModels(c *gin.Context) {
	if c.Request.Context().Err() != nil {
		return
REDACTED
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "invalid_request_error", "API key group is required")
		return
REDACTED
	if apiKey.Group.Platform != service.PlatformOpenAI {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Codex models manifest is only available for OpenAI groups")
		return
REDACTED

	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
REDACTED
	failedAccountIDs := make(map[int64]struct{REDACTED)
	switchCount := 0
	var lastUpstreamErr error

	for {
		account, err := h.gatewayService.SelectAccountForModelWithExclusions(c.Request.Context(), apiKey.GroupID, "", "", failedAccountIDs)
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
		REDACTED
			if lastUpstreamErr != nil {
				h.errorResponse(c, infraerrors.Code(lastUpstreamErr), "upstream_error", infraerrors.Message(lastUpstreamErr))
				return
		REDACTED
			h.errorResponse(c, http.StatusServiceUnavailable, "upstream_error", "No available OpenAI accounts")
			return
	REDACTED
		// 让 ops 错误日志携带实际选中的上游账号，便于定位失效账号（#4544）。
		setOpsSelectedAccount(c, account.ID, account.Platform)

		manifest, err := h.gatewayService.FetchCodexModelsManifest(c.Request.Context(), account, c.Query("client_version"), c.GetHeader("If-None-Match"))
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
		REDACTED
			if service.IsRetryableCodexModelsManifestError(err) && switchCount < maxAccountSwitches {
				failedAccountIDs[account.ID] = struct{REDACTED{REDACTED
				switchCount++
				lastUpstreamErr = err
				continue
		REDACTED
			h.errorResponse(c, infraerrors.Code(err), "upstream_error", infraerrors.Message(err))
			return
	REDACTED
		if c.Request.Context().Err() != nil {
			return
	REDACTED

		if manifest.ETag != "" {
			c.Header("ETag", manifest.ETag)
	REDACTED
		if manifest.NotModified {
			c.Status(http.StatusNotModified)
			return
	REDACTED
		c.Data(http.StatusOK, "application/json", manifest.Body)
		return
REDACTED
REDACTED

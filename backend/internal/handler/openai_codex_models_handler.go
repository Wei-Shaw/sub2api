package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

// CodexModels serves the Codex models manifest for Codex clients.
//
// Codex CLI and the Codex desktop app refresh their model picker from
// GET {base_url}/models?client_version=... (custom provider mode) or
// GET /backend-api/codex/models (chatgpt_base_url mode). Both routes land
// here. The manifest is proxied verbatim from the ChatGPT backend with a
// schedulable OAuth account's credentials, so clients pointed at the gateway
// see the account's real, always-current model entitlements instead of a
// frozen local cache.
func (h *OpenAIGatewayHandler) CodexModels(c *gin.Context) {
	setOpsRequestContext(c, "", false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "invalid_request_error", "API key group is required")
		return
	}
	if apiKey.Group.Platform != service.PlatformOpenAI {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Codex models manifest is only available for OpenAI groups")
		return
	}

	requestCtx := c.Request.Context()
	clientVersion := c.Query("client_version")
	ifNoneMatch := c.GetHeader("If-None-Match")
	failedAccountIDs := make(map[int64]struct{})
	maxSwitches := h.maxAccountSwitches
	if maxSwitches <= 0 {
		maxSwitches = 3
	}
	switchCount := 0
	var lastErr error
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.codex_models",
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
		zap.String("client_version", truncateString(clientVersion, 64)),
	)

	for {
		account, err := h.gatewayService.SelectCodexModelsAccountWithExclusions(requestCtx, apiKey.GroupID, failedAccountIDs)
		if err != nil {
			if lastErr != nil {
				h.errorResponse(c, infraerrors.Code(lastErr), "upstream_error", infraerrors.Message(lastErr))
				return
			}
			h.errorResponse(c, http.StatusServiceUnavailable, "upstream_error", "No available OpenAI OAuth accounts")
			return
		}

		setOpsSelectedAccount(c, account.ID, account.Platform)
		service.SetActualOpenAIUpstreamEndpoint(c, EndpointCodexModels)
		attemptETag := ifNoneMatch
		if len(failedAccountIDs) > 0 {
			attemptETag = ""
		}

		manifest, err := h.gatewayService.FetchCodexModelsManifestForRequest(requestCtx, c, account, clientVersion, attemptETag)
		if err == nil {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, nil)
			if manifest.ETag != "" {
				c.Header("ETag", manifest.ETag)
			}
			if manifest.NotModified {
				c.Status(http.StatusNotModified)
				return
			}
			c.Data(http.StatusOK, "application/json", manifest.Body)
			return
		}

		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
		if requestCtx.Err() != nil || errors.Is(err, context.Canceled) {
			return
		}
		if !service.IsCodexModelsManifestRetryable(err) {
			h.errorResponse(c, infraerrors.Code(err), "upstream_error", infraerrors.Message(err))
			return
		}

		failedAccountIDs[account.ID] = struct{}{}
		lastErr = err
		if switchCount >= maxSwitches {
			h.errorResponse(c, infraerrors.Code(err), "upstream_error", infraerrors.Message(err))
			return
		}
		switchCount++
		h.gatewayService.RecordOpenAIAccountSwitch()
		reqLog.Warn("openai_codex_models.upstream_failover_switching",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
			zap.Int("max_switches", maxSwitches),
		)
	}
}

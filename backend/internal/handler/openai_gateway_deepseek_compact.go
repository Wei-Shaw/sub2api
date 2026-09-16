package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func isExactDeepSeekResponsesCompactPath(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	switch strings.TrimSpace(c.Request.URL.Path) {
	case "/v1/responses/compact", "/openai/v1/responses/compact", "/responses/compact", "/backend-api/codex/responses/compact":
		return true
	default:
		return false
	}
}

func isExactDeepSeekResponsesPath(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	switch strings.TrimSpace(c.Request.URL.Path) {
	case "/v1/responses", "/openai/v1/responses", "/responses", "/backend-api/codex/responses":
		return true
	default:
		return false
	}
}

func isDeepSeekRemoteCompactionV2Request(c *gin.Context, body []byte) bool {
	if !isOpenAIRemoteCompactionV2Request(body) || c == nil || c.Request == nil {
		return false
	}
	for _, header := range c.Request.Header.Values("x-codex-beta-features") {
		for _, feature := range strings.Split(header, ",") {
			if strings.TrimSpace(feature) == "remote_compaction_v2" {
				return true
			}
		}
	}
	return false
}

func classifyDeepSeekCompactionRequest(c *gin.Context, body []byte, requestPlatform string) service.DeepSeekCompactionMode {
	if requestPlatform != service.PlatformDeepSeek {
		return service.DeepSeekCompactionModeNone
	}
	if isExactDeepSeekResponsesCompactPath(c) {
		return service.DeepSeekCompactionModeLegacyUnary
	}
	if !isExactDeepSeekResponsesPath(c) || !service.HasCompactionTriggerInInput(body) {
		return service.DeepSeekCompactionModeNone
	}
	stream, valid := parseOpenAICompatibleStream(body)
	if !valid {
		return service.DeepSeekCompactionModeNone
	}
	if !stream {
		return service.DeepSeekCompactionModeLegacyUnary
	}
	if isDeepSeekRemoteCompactionV2Request(c, body) {
		return service.DeepSeekCompactionModeRemoteV2SSE
	}
	return service.DeepSeekCompactionModeLegacyBodySSE
}

func markDeepSeekRemoteCompactionV2Request(c *gin.Context, reqLog *zap.Logger, body []byte, requestPlatform string) bool {
	mode := classifyDeepSeekCompactionRequest(c, body, requestPlatform)
	if mode == service.DeepSeekCompactionModeNone {
		return false
	}
	service.MarkDeepSeekCompaction(c, mode)
	if reqLog != nil {
		reqLog.Info("deepseek.compact_bridge.detected", zap.String("compact_mode", string(mode)))
	}
	return true
}

func (h *OpenAIGatewayHandler) restoreDeepSeekCompactInputBeforeAudit(c *gin.Context, body []byte, requestPlatform string) ([]byte, bool) {
	if h == nil || h.gatewayService == nil {
		return body, true
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	restoredBody, changed, err := h.gatewayService.RestoreDeepSeekCompactInputForTarget(ctx, body, requestPlatform)
	if err != nil {
		if errors.Is(err, service.ErrDeepSeekCompactRequestTooLarge) {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", err.Error())
			return nil, false
		}
		message := "invalid DeepSeek compact encrypted_content"
		if errors.Is(err, service.ErrDeepSeekResponsesDuplicateJSONKey) ||
			errors.Is(err, service.ErrDeepSeekResponsesNonCanonicalJSONKey) {
			message = err.Error()
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", message)
		return nil, false
	}
	if requestPlatform == service.PlatformDeepSeek {
		service.MarkDeepSeekResponsesInputValidated(c)
	}
	if changed {
		return restoredBody, true
	}
	return body, true
}

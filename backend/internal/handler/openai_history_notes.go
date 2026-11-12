package handler

import (
	"errors"
	"net/http"
	"strings"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// HistoryNotes proxies the private model-only History/Notes API used by Codex
// experimental context management. Requests are pinned to the same upstream
// account as context.session_id; no cross-account failover is attempted.
func (h *OpenAIGatewayHandler) HistoryNotes(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if apiKey.Group.Platform != service.PlatformOpenAI && apiKey.Group.Platform != service.PlatformComposite {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Codex history/notes is only available for OpenAI and Composite groups")
		return
	}

	namespace := strings.TrimSpace(c.Param("state_namespace"))
	action := strings.TrimSpace(c.Param("action"))
	if !service.IsCodexHistoryNotesEndpoint(namespace, action) {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Unsupported Codex history/notes endpoint")
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if !gjson.ValidBytes(body) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	if strings.TrimSpace(gjson.GetBytes(body, "context.session_id").String()) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "context.session_id is required")
		return
	}

	result, err := h.gatewayService.ForwardCodexHistoryNotes(
		c.Request.Context(),
		c,
		apiKey.GroupID,
		namespace,
		action,
		body,
	)
	if err != nil {
		if errors.Is(err, service.ErrCodexHistoryNotesSessionUnbound) && namespace == "notes" && action == "thread_hint" {
			// A fresh Codex thread asks for a hint before its first Responses call,
			// so no sticky account exists yet. Empty hint is the safe no-state result.
			c.JSON(http.StatusOK, gin.H{"text": ""})
			return
		}
		switch {
		case errors.Is(err, service.ErrCodexHistoryNotesSessionUnbound):
			h.errorResponse(c, http.StatusConflict, "session_unavailable", "Codex history session is not pinned to an upstream account")
		case errors.Is(err, service.ErrCodexHistoryNotesUnsupported):
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Pinned account does not support Codex history/notes")
		default:
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Codex history/notes request failed")
		}
		return
	}

	for _, key := range []string{"Content-Type", "X-Request-Id", "X-Oai-Request-Id"} {
		if value := result.Header.Get(key); value != "" {
			c.Header(key, value)
		}
	}
	contentType := result.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(result.StatusCode, contentType, result.Body)
}

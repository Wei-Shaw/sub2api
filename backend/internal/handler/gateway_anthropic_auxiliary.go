package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// tryAnthropicUpstreamModels preserves native model metadata for a single
// strict upstream. Explicit group allowlists retain their local catalog policy.
func (h *GatewayHandler) tryAnthropicUpstreamModels(c *gin.Context, apiKey *service.APIKey, platform string) bool {
	if platform != service.PlatformAnthropic || apiKey == nil || apiKey.Group == nil || apiKey.Group.ModelAllowlistEnabled() {
		return false
	}
	account, err := h.gatewayService.SingleAnthropicPassthroughAccount(c.Request.Context(), apiKey.GroupID)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Native upstream unavailable")
		return true
	}
	if account == nil {
		return false
	}
	endpoint := "/models"
	if id := c.Param("model"); id != "" {
		endpoint += "/" + url.PathEscape(id)
	}
	if err := h.gatewayService.ForwardAnthropicAuxiliary(c.Request.Context(), c, account, endpoint); err != nil && !c.Writer.Written() {
		h.errorResponse(c, http.StatusBadGateway, "api_error", "Failed to fetch upstream models")
	}
	return true
}

// AnthropicAuxiliary handles native model detail and Files endpoints behind the
// same API-key and group middleware as Messages. No provider key is exposed.
func (h *GatewayHandler) AnthropicAuxiliary(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformAnthropic {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Native Anthropic endpoint unavailable for this group")
		return
	}
	account, err := h.gatewayService.SingleAnthropicPassthroughAccount(c.Request.Context(), apiKey.GroupID)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Native upstream unavailable")
		return
	}
	if account == nil {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "This endpoint requires one strict Anthropic API key upstream")
		return
	}
	endpoint := "/files"
	if id := c.Param("model_id"); id != "" {
		endpoint = "/models/" + url.PathEscape(id)
	} else if id := c.Param("file_id"); id != "" {
		endpoint += "/" + url.PathEscape(id)
		if strings.HasSuffix(c.FullPath(), "/content") {
			endpoint += "/content"
		}
	}
	if err := h.gatewayService.ForwardAnthropicAuxiliary(c.Request.Context(), c, account, endpoint); err != nil && !c.Writer.Written() {
		h.errorResponse(c, http.StatusBadGateway, "api_error", "Upstream request failed")
	}
}

// TryAnthropicModels lets the route preserve native queries before applying
// client-specific catalog formats used by other gateway platforms.
func (h *GatewayHandler) TryAnthropicModels(c *gin.Context) bool {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil {
		return false
	}
	return h.tryAnthropicUpstreamModels(c, apiKey, apiKey.Group.Platform)
}

// Retain the decoded client body before any compatibility repair. Once an
// account is selected, strict mode rejects changes and compatibility keeps them.
func readAnthropicGatewayRequestBody(req *http.Request, cfg *config.Config, apiKey *service.APIKey) ([]byte, error) {
	if apiKey != nil && apiKey.Group != nil && apiKey.Group.Platform == service.PlatformAnthropic {
		return pkghttputil.ReadRequestBodyWithPrealloc(req)
	}
	return readLenientJSONRequestBodyWithPrealloc(req, cfg)
}

func parseAnthropicGatewayRequest(body *service.RequestBodyRef, cfg *config.Config, apiKey *service.APIKey) (*service.ParsedRequest, error) {
	if apiKey != nil && apiKey.Group != nil && apiKey.Group.Platform == service.PlatformAnthropic {
		normalized, err := pkghttputil.NormalizeLenientJSONRequestBody(body.Bytes(), gatewayMaxBodySize(cfg))
		if err != nil {
			return nil, err
		}
		body.Replace(normalized)
		return service.ParseGatewayRequestPreservingBody(body, service.PlatformAnthropic)
	}
	return service.ParseGatewayRequest(body, service.PlatformAnthropic)
}

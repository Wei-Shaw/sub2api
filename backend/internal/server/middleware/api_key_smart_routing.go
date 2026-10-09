package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func resolveSmartRoutingKey(c *gin.Context, keys *service.APIKeyService, gateway *service.OpenAIGatewayService, key *service.APIKey) (*service.APIKey, bool) {
	if gateway == nil {
		AbortWithError(c, 503, "SMART_ROUTING_UNAVAILABLE", "Smart routing is temporarily unavailable")
		return nil, false
	}
	path := c.Request.URL.Path
	for _, prefix := range []string{"/backend-api/codex", "/v1"} {
		path = strings.TrimPrefix(path, prefix)
	}
	discovery := c.Request.Method == http.MethodGet && path == "/models"
	inspection := c.Request.Method == http.MethodGet && (path == "/usage" || path == "/sub2api/billing")
	textRequest := c.Request.Method == http.MethodPost && (path == "/chat/completions" || path == "/responses" || path == "/responses/compact")
	if !discovery && !inspection && !textRequest {
		AbortWithError(c, 400, "SMART_ROUTING_ENDPOINT_UNSUPPORTED", "Smart routing supports HTTP Chat Completions and Responses; use a fixed-group key for other endpoints")
		return nil, false
	}
	model, previousResponseID := "", ""
	compact := path == "/responses/compact"
	needsResponses := compact
	imageIntent := false
	if textRequest {
		body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			status := http.StatusBadRequest
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				status = http.StatusRequestEntityTooLarge
			}
			AbortWithError(c, status, "INVALID_REQUEST", "Unable to read request body")
			return nil, false
		}
		requestmodel.ResetRequestBody(c.Request, body)
		value := gjson.GetBytes(body, "model")
		if !gjson.ValidBytes(body) || value.Type != gjson.String || strings.TrimSpace(value.String()) == "" {
			AbortWithError(c, 400, "INVALID_REQUEST", "A non-empty model is required for smart routing")
			return nil, false
		}
		model = value.String()
		// Reject ambiguous model fields before choosing any billing group.
		for _, candidate := range requestmodel.FromBodyCandidates(c.FullPath(), c.GetHeader("Content-Type"), body) {
			if candidate != model {
				AbortWithError(c, 400, "INVALID_REQUEST", "Conflicting model fields")
				return nil, false
			}
		}
		previousResponseID = strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String())
		imageIntent = service.IsExplicitImageGenerationIntent(path, model, body)
		if imageIntent {
			AbortWithError(c, 400, "SMART_ROUTING_ENDPOINT_UNSUPPORTED", "Use a fixed-group key for image generation")
			return nil, false
		}
		if path == "/responses" && service.HasCompactionTriggerInInput(body) {
			needsResponses = true
			compact = !gjson.GetBytes(body, "stream").Bool()
		}
	}
	candidates, err := keys.SmartRoutingKeys(c.Request.Context(), key)
	if err != nil {
		AbortWithError(c, 503, "SMART_ROUTING_UNAVAILABLE", "Unable to load routing groups")
		return nil, false
	}
	if len(candidates) == 0 {
		AbortWithError(c, 403, "SMART_ROUTING_NO_ACCESS", "No accessible routing groups")
		return nil, false
	}
	if inspection {
		if path == "/sub2api/billing" && len(candidates) > 1 {
			AbortWithError(c, 400, "SMART_ROUTING_BILLING_UNSUPPORTED", "Billing introspection requires a fixed-group API key")
			return nil, false
		}
		return candidates[0], true
	}
	if discovery {
		models := make([]string, 0)
		for _, candidate := range candidates {
			ids, err := gateway.SmartRoutingModels(c.Request.Context(), candidate)
			if err != nil {
				AbortWithError(c, 503, "SMART_ROUTING_UNAVAILABLE", "Unable to load routing models")
				return nil, false
			}
			models = append(models, ids...)
		}
		c.Request = c.Request.WithContext(service.WithSmartRoutingModels(c.Request.Context(), models))
		return candidates[0], true
	}
	for _, candidate := range candidates {
		if previousResponseID != "" {
			owned, err := gateway.ValidateOpenAIHTTPResponseOwner(c.Request.Context(), *candidate.GroupID, previousResponseID, key.UserID, key.ID)
			if err != nil {
				AbortWithError(c, 503, "SMART_ROUTING_UNAVAILABLE", "Unable to verify response ownership")
				return nil, false
			}
			if !owned {
				continue
			}
		}
		capability := service.OpenAIEndpointCapabilityChatCompletions
		if needsResponses && candidate.Group.Platform == service.PlatformOpenAI {
			capability = service.OpenAIEndpointCapabilityResponses
		}
		available, err := gateway.SmartRoutingAvailable(c.Request.Context(), candidate, model, capability, compact)
		if err != nil {
			AbortWithError(c, 503, "SMART_ROUTING_UNAVAILABLE", "Unable to check routing availability")
			return nil, false
		}
		if available {
			prepared, selected, err := gateway.PrepareSmartRouting(c.Request.Context(), candidate, model, previousResponseID, capability, compact)
			if err != nil {
				AbortWithError(c, 503, "SMART_ROUTING_UNAVAILABLE", "Unable to select a routing account")
				return nil, false
			}
			if !selected {
				continue
			}
			c.Request = c.Request.WithContext(prepared)
			return candidate, true
		}
	}
	AbortWithError(c, 503, "SMART_ROUTING_NO_AVAILABLE_GROUP", "No routing group currently has an available account for this model")
	return nil, false
}

package handler

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountSelectionErrorTemplateDataFunc func(context.Context, string, *int64) service.AccountSelectionErrorTemplateData

func openAIAccountSelectionErrorResponse(
	c *gin.Context,
	errorPassthroughService *service.ErrorPassthroughService,
	templateData accountSelectionErrorTemplateDataFunc,
	platform string,
	groupID *int64,
	matchMessage string,
	defaultMessage string,
) (int, string, string) {
	status := http.StatusServiceUnavailable
	errType := "api_error"
	message := defaultMessage
	if errorPassthroughService == nil {
		return status, errType, message
	}

	body := []byte(matchMessage)
	if platform == "" {
		platform = service.PlatformOpenAI
	}
	rule := errorPassthroughService.MatchRule(platform, service.OpenAIAccountSelectionNoAvailableStatus, body)
	if rule == nil && platform != service.PlatformOpenAI {
		rule = errorPassthroughService.MatchRule(service.PlatformOpenAI, service.OpenAIAccountSelectionNoAvailableStatus, body)
	}
	if rule == nil {
		return status, errType, message
	}

	if !rule.PassthroughCode && rule.ResponseCode != nil {
		status = *rule.ResponseCode
	}
	if !rule.PassthroughBody && rule.CustomMessage != nil {
		data := service.AccountSelectionErrorTemplateData{}
		if templateData != nil {
			ctx := context.Background()
			if c != nil && c.Request != nil {
				ctx = c.Request.Context()
			}
			data = templateData(ctx, platform, groupID)
		}
		message = service.RenderAccountSelectionErrorMessage(*rule.CustomMessage, data)
	} else {
		message = matchMessage
	}
	if rule.SkipMonitoring && c != nil {
		c.Set(service.OpsSkipPassthroughKey, true)
	}
	return status, "upstream_error", message
}

func (h *OpenAIGatewayHandler) openAIAccountSelectionErrorResponse(c *gin.Context, groupID *int64, matchMessage string, defaultMessage string) (int, string, string) {
	var templateData accountSelectionErrorTemplateDataFunc
	if h != nil && h.gatewayService != nil {
		templateData = h.gatewayService.AccountSelectionErrorTemplateData
	}
	var errorPassthroughService *service.ErrorPassthroughService
	if h != nil {
		errorPassthroughService = h.errorPassthroughService
	}
	return openAIAccountSelectionErrorResponse(c, errorPassthroughService, templateData, service.PlatformOpenAI, groupID, matchMessage, defaultMessage)
}

func (h *GatewayHandler) openAIAccountSelectionErrorResponse(c *gin.Context, platform string, groupID *int64, matchMessage string, defaultMessage string) (int, string, string) {
	var templateData accountSelectionErrorTemplateDataFunc
	if h != nil && h.gatewayService != nil {
		templateData = h.gatewayService.AccountSelectionErrorTemplateData
	}
	var errorPassthroughService *service.ErrorPassthroughService
	if h != nil {
		errorPassthroughService = h.errorPassthroughService
	}
	return openAIAccountSelectionErrorResponse(c, errorPassthroughService, templateData, platform, groupID, matchMessage, defaultMessage)
}

func apiKeyGroupPlatform(apiKey *service.APIKey) string {
	if apiKey == nil || apiKey.Group == nil {
		return ""
	}
	return apiKey.Group.Platform
}

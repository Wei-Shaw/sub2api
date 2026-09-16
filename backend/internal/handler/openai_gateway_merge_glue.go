package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func copyFailoverResponseHeaders(c *gin.Context, headers http.Header) {
	copyFailoverRetryAfter(c, headers)
	if c == nil || headers == nil {
		return
	}
	for _, key := range []string{"x-request-id", "x-deepseek-request-id"} {
		value := strings.TrimSpace(headers.Get(key))
		if value != "" && len(value) <= 256 && !strings.ContainsAny(value, "\r\n") {
			c.Header(key, value)
		}
	}
}

func validateResponsesWebSocketTurnPlatform(
	c *gin.Context,
	apiKey *service.APIKey,
	ctx context.Context,
	connectionPlatform string,
	model string,
) error {
	if apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite {
		return nil
	}
	_, targetPlatform, err := resolveResponsesWebSocketTarget(c, apiKey, ctx, model)
	if err != nil {
		return service.NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"Responses WebSocket model route could not be resolved",
			err,
		)
	}
	if targetPlatform == connectionPlatform {
		return nil
	}
	cause := fmt.Errorf(
		"%w: connection platform %q cannot serve target platform %q for model %q",
		errOpenAIWSUnsupportedModelSwitch,
		strings.TrimSpace(connectionPlatform),
		strings.TrimSpace(targetPlatform),
		strings.TrimSpace(model),
	)
	return service.NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "model switch requires reconnect", cause)
}

func shouldRecordDeepSeekPartialUsage(account *service.Account, result *service.OpenAIForwardResult, forwardErr error) bool {
	if account == nil || !account.IsDeepSeekAPIKey() || result == nil || forwardErr == nil ||
		result.ImageCount > 0 || result.UpstreamTerminalEvent == "response.failed" || !result.HasBillableTokenUsage() {
		return false
	}
	var failoverErr *service.UpstreamFailoverError
	return !errors.As(forwardErr, &failoverErr)
}

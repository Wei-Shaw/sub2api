package service

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
)

type openAICompactionSummaryServiceContextKey struct{}

// WithOpenAICompactionSummaryService carries the existing OpenAI gateway helper
// into another protocol forwarder. The selected protocol still owns auth,
// transport, and billing; only summary conversion/sealing is shared.
func WithOpenAICompactionSummaryService(ctx context.Context, service *OpenAIGatewayService) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAICompactionSummaryServiceContextKey{}, service)
}

func MarkOpenAICompactionSummaryExecutionForRequest(c *gin.Context, protocol string) {
	markOpenAICompactionSummaryExecution(c, protocol)
}

func finalizeOpenAICompactionSummaryBody(c *gin.Context, body []byte) ([]byte, error) {
	if _, ok := openAICompactionSummaryExecutionFromContext(c); !ok {
		return body, nil
	}
	if c == nil || c.Request == nil {
		return body, fmt.Errorf("%w: request context is unavailable", ErrOpenAICompactionSummaryInvalid)
	}
	svc, _ := c.Request.Context().Value(openAICompactionSummaryServiceContextKey{}).(*OpenAIGatewayService)
	if svc == nil {
		return body, fmt.Errorf("%w: summary sealing service is unavailable", ErrOpenAICompactionSummaryKey)
	}
	return svc.finalizeOpenAICompactionSummaryResponse(c, 200, body)
}

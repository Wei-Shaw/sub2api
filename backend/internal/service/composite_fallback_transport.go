package service

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func OpenAIStreamKeepaliveAdjustedWrittenSize(c *gin.Context) int {
	size := OpenAICompactKeepaliveAdjustedWrittenSize(c)
	if size == 0 {
		return -1
	}
	return size
}

func compositeFallbackPreOutputReadError(ctx context.Context, c *gin.Context, err error) *UpstreamFailoverError {
	if _, active := CompositeFallbackDecisionFromContext(ctx); !active || err == nil || c == nil || c.Request == nil {
		return nil
	}
	if c.Request.Context().Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, ErrUpstreamResponseBodyTooLarge) || OpenAIStreamKeepaliveAdjustedWrittenSize(c) > -1 {
		return nil
	}
	return &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: openAITransportFailoverBody}
}

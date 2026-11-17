package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIForwardRequestBodyMemoryOverloadReturns503(t *testing.T) {
	c, recorder := newGinContextForEndpoint(t, EndpointResponses)
	h := &OpenAIGatewayHandler{}
	err := fmt.Errorf("materialize rewritten body: %w", pkghttputil.ErrRequestBodyMemoryBudget)

	require.True(t, h.handleForwardRequestBodyMemoryOverload(c, err, false))
	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Equal(t, "1", recorder.Result().Header.Get("Retry-After"))
	require.Equal(t, "service_unavailable", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Contains(t, gjson.GetBytes(recorder.Body.Bytes(), "error.message").String(), "retry later")
}

func TestOpenAIForwardRequestBodyMemoryOverloadTerminatesCommittedStream(t *testing.T) {
	c, recorder := newGinContextForEndpoint(t, EndpointResponses)
	c.Header("Content-Type", "text/event-stream")
	_, err := c.Writer.Write([]byte(": keepalive\n\n"))
	require.NoError(t, err)
	c.Writer.Flush()
	h := &OpenAIGatewayHandler{}

	// A heartbeat may commit HTTP 200 before the caller updates streamStarted.
	require.True(t, h.handleForwardRequestBodyMemoryOverload(c, pkghttputil.ErrRequestBodyMemoryBudget, false))
	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusOK, recorder.Code)
	_, errorObject := parseResponsesFailedSSE(t, strings.TrimPrefix(recorder.Body.String(), ": keepalive\n\n"))
	require.Contains(t, errorObject["message"], "retry later")
	require.NotContains(t, recorder.Body.String(), "response.completed")
	streamError, ok := service.GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, http.StatusServiceUnavailable, streamError.IntendedStatus)
}

func TestOpenAIForwardRequestBodyMemoryOverloadLeavesOtherErrorsUntouched(t *testing.T) {
	for _, err := range []error{nil, errors.New("upstream failed")} {
		c, recorder := newGinContextForEndpoint(t, EndpointResponses)
		h := &OpenAIGatewayHandler{}
		require.False(t, h.handleForwardRequestBodyMemoryOverload(c, err, false))
		require.False(t, c.IsAborted())
		require.False(t, c.Writer.Written())
		require.Empty(t, recorder.Body.String())
		require.Empty(t, recorder.Header().Get("Retry-After"))
	}
}

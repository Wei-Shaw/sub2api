package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAnthropicJSONKeepalivePreservesValidSuccessJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	originalWriter := c.Writer

	stop := StartAnthropicJSONKeepalive(c, 5*time.Millisecond)
	require.Eventually(t, func() bool {
		return AnthropicJSONKeepaliveCommitted(c)
	}, time.Second, time.Millisecond)
	require.Equal(t, -1, AnthropicDownstreamAdjustedWrittenSize(c))

	c.JSON(http.StatusOK, gin.H{"type": "message", "content": []any{}})
	stop()

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, rec.Flushed)
	require.True(t, strings.HasPrefix(rec.Body.String(), " \n"), rec.Body.String())
	require.True(t, json.Valid(rec.Body.Bytes()), rec.Body.String())
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "type").String())
	require.Greater(t, AnthropicDownstreamAdjustedWrittenSize(c), 0)
	require.Same(t, originalWriter, c.Writer)
}

func TestAnthropicJSONKeepaliveFastErrorPreservesStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	stop := StartAnthropicJSONKeepalive(c, time.Hour)
	writeAnthropicGatewayErrorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "slow down")
	stop()

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.False(t, strings.HasPrefix(rec.Body.String(), " \n"))
	require.True(t, json.Valid(rec.Body.Bytes()), rec.Body.String())
	require.Equal(t, "slow down", gjson.Get(rec.Body.String(), "error.message").String())
	_, marked := GetOpsStreamError(c)
	require.False(t, marked)
}

func TestAnthropicJSONKeepaliveLateErrorRemainsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	stop := StartAnthropicJSONKeepalive(c, 5*time.Millisecond)
	defer stop()
	require.Eventually(t, func() bool {
		return AnthropicJSONKeepaliveCommitted(c)
	}, time.Second, time.Millisecond)

	writeAnthropicGatewayErrorResponse(c, http.StatusBadGateway, "upstream_error", "upstream failed")

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, json.Valid(rec.Body.Bytes()), rec.Body.String())
	require.NotContains(t, rec.Body.String(), "event:")
	require.NotContains(t, rec.Body.String(), "data:")
	require.Equal(t, "upstream failed", gjson.Get(rec.Body.String(), "error.message").String())
	marked, ok := GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, http.StatusBadGateway, marked.IntendedStatus)
}

func TestAnthropicJSONKeepalivePaddingDoesNotBlockFailoverSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	stop := StartAnthropicJSONKeepalive(c, 5*time.Millisecond)
	defer stop()
	before := AnthropicDownstreamAdjustedWrittenSize(c)
	require.Eventually(t, func() bool {
		return AnthropicJSONKeepaliveCommitted(c)
	}, time.Second, time.Millisecond)

	require.Equal(t, before, AnthropicDownstreamAdjustedWrittenSize(c))
	require.True(t, c.Writer.Written())
	require.Empty(t, strings.TrimSpace(rec.Body.String()))
}

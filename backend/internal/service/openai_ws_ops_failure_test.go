package service

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMarkOpenAIWSClientVisibleFailure_ResponseFailedNestedError(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	markOpenAIWSClientVisibleFailure(c, "response.failed", []byte(`{"type":"response.failed","response":{"error":{"type":"invalid_request_error","code":"context_length_exceeded","message":"too long","status_code":400}}}`))

	got, ok := GetOpsStreamError(c)
	require.True(t, ok)
	require.True(t, got.CountTowardsSLA)
	require.Equal(t, http.StatusBadRequest, got.IntendedStatus)
	require.Equal(t, "invalid_request_error", got.ErrType)
	require.Equal(t, "context_length_exceeded", got.Code)
	require.Equal(t, "too long", got.Message)
}

func TestMarkOpenAIWSClientVisibleFailure_ErrorAndSuccessBoundary(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		markOpenAIWSClientVisibleFailure(c, "error", []byte(`{"type":"error","error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"slow down"}}`))
		got, ok := GetOpsStreamError(c)
		require.True(t, ok)
		require.Equal(t, http.StatusTooManyRequests, got.IntendedStatus)
	})

	t.Run("completed does not mark", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		markOpenAIWSClientVisibleFailure(c, "response.completed", []byte(`{"type":"response.completed"}`))
		_, ok := GetOpsStreamError(c)
		require.False(t, ok)
	})
}

func TestSetOpsUpstreamModelStoresOnlyTrimmedModelSlug(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	SetOpsUpstreamModel(c, "  gpt-5.6-sol  ")
	value, ok := c.Get(OpsUpstreamModelKey)
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-sol", value)

	SetOpsUpstreamModel(c, "  ")
	value, ok = c.Get(OpsUpstreamModelKey)
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-sol", value)

	ClearOpsUpstreamModel(c)
	value, ok = c.Get(OpsUpstreamModelKey)
	require.True(t, ok)
	require.Equal(t, "", value)
}

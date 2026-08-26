package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResponsesInputTokensIsClassifiedAsTokenCountRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/input_tokens", nil)

	require.True(t, isCountTokensRequest(c))
	require.True(t, isTokenCountRequestPath("/responses/input_tokens"))
	require.False(t, isTokenCountRequestPath("/v1/responses"))
}

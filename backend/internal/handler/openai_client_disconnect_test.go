package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIEnsureForwardErrorResponse_SkipsCanceledClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, nil).WithContext(ctx)
	cancel()

	h := &OpenAIGatewayHandler{}
	require.False(t, h.ensureForwardErrorResponse(c, true))
	require.Equal(t, statusClientClosedRequest, c.Writer.Status())
	require.Empty(t, recorder.Body.String())
}

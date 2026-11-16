package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMuseForwardErrorDoesNotAppendFallback(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	gateway := &service.OpenAIGatewayService{}
	_, err := gateway.ForwardAsChatCompletions(c.Request.Context(), c, &service.Account{Platform: service.PlatformMuse}, nil, "", "")
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	original := recorder.Body.String()
	handler := &OpenAIGatewayHandler{}
	require.False(t, handler.ensureForwardErrorResponse(c, false), "Muse already wrote the complete protocol error")
	require.Equal(t, original, recorder.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body), "clients must receive one valid JSON document")
	require.Contains(t, original, "muse_transport_unqualified")
}

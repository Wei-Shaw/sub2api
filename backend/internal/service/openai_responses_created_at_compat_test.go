package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func requirePositiveResponseCreatedAt(t *testing.T, body []byte) {
	t.Helper()
	require.True(t, gjson.GetBytes(body, "response.created_at").Exists())
	require.Positive(t, gjson.GetBytes(body, "response.created_at").Int())
}

func TestResponsesCreatedAtCompatibility_SyntheticFailures(t *testing.T) {
	t.Run("generic SSE failure", func(t *testing.T) {
		requirePositiveResponseCreatedAt(t, []byte(buildOpenAIResponseFailedSSE("resp_1", "model", nil, "failed")))
	})
	t.Run("WS HTTP failure", func(t *testing.T) {
		requirePositiveResponseCreatedAt(t, buildOpenAIWSHTTPBridgeFailedEvent("resp_1", "model", nil, "failed"))
	})
	t.Run("compact failure", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
		writeOpenAICompactSSEFailureMessage(ctx, http.StatusBadGateway, "upstream_error", "failed")
		requirePositiveResponseCreatedAt(t, []byte(recorder.Body.String()))
	})
	t.Run("buffered terminal failure", func(t *testing.T) {
		out := openAICompatTerminalResponse(&apicompat.ResponsesStreamEvent{Type: "error", Code: "upstream_error"}, []byte(`{"error":{"message":"failed"}}`))
		require.NotNil(t, out)
		require.Positive(t, out.CreatedAt)
	})
}

func TestResponsesCreatedAtCompatibility_CompactPayload(t *testing.T) {
	for name, source := range map[string][]byte{
		"missing":  []byte(`{"id":"resp_1","output":[]}`),
		"invalid":  []byte(`{"id":"resp_1","created_at":"bad","output":[]}`),
		"zero":     []byte(`{"id":"resp_1","created_at":0,"output":[]}`),
		"negative": []byte(`{"id":"resp_1","created_at":-1,"output":[]}`),
	} {
		t.Run(name, func(t *testing.T) {
			payload, ok := buildOpenAICompactSSEPayload(source)
			require.True(t, ok)
			requirePositiveResponseCreatedAt(t, payload)
		})
	}
	t.Run("preserves positive source", func(t *testing.T) {
		payload, ok := buildOpenAICompactSSEPayload([]byte(`{"id":"resp_1","created_at":123,"output":[]}`))
		require.True(t, ok)
		require.Equal(t, int64(123), gjson.GetBytes(payload, "response.created_at").Int())
	})
}

func TestResponsesCreatedAtCompatibility_FailedPayloadsAreJSON(t *testing.T) {
	body := buildOpenAIWSHTTPBridgeFailedEvent("resp_1", "", nil, "failed")
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
}

package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func requirePositiveResponseCreatedAt(t *testing.T, body []byte) {
	t.Helper()
	createdAt := gjson.GetBytes(body, "response.created_at")
	require.True(t, createdAt.Exists())
	value, err := strconv.ParseInt(createdAt.Raw, 10, 64)
	require.NoError(t, err)
	require.Positive(t, value)
}

func requireSyntheticResponseFields(t *testing.T, body []byte, expectedID string) {
	t.Helper()
	response := gjson.GetBytes(body, "response")
	require.Equal(t, expectedID, response.Get("id").String())
	require.Equal(t, "response", response.Get("object").String())
	require.Equal(t, "failed", response.Get("status").String())
	require.True(t, response.Get("output").IsArray())
	require.NotEmpty(t, response.Get("error.code").String())
	require.NotEmpty(t, response.Get("error.message").String())
	requirePositiveResponseCreatedAt(t, body)
}

func TestResponsesCreatedAtCompatibility_SyntheticFailures(t *testing.T) {
	t.Run("generic SSE failure", func(t *testing.T) {
		requireSyntheticResponseFields(t, []byte(buildOpenAIResponseFailedSSE("resp_1", "model", nil, "failed")), "resp_1")
	})
	t.Run("WS HTTP failure", func(t *testing.T) {
		requireSyntheticResponseFields(t, buildOpenAIWSHTTPBridgeFailedEvent("resp_1", "model", nil, "failed"), "resp_1")
	})
	t.Run("WS HTTP failure repairs empty response ID", func(t *testing.T) {
		body := buildOpenAIWSHTTPBridgeFailedEvent("", "model", nil, "failed")
		responseID := gjson.GetBytes(body, "response.id").String()
		require.True(t, len(responseID) > len("resp_"))
		requireSyntheticResponseFields(t, body, responseID)
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
		"missing":         []byte(`{"id":"resp_1","output":[]}`),
		"string":          []byte(`{"id":"resp_1","created_at":"bad","output":[]}`),
		"zero":            []byte(`{"id":"resp_1","created_at":0,"output":[]}`),
		"negative":        []byte(`{"id":"resp_1","created_at":-1,"output":[]}`),
		"negative zero":   []byte(`{"id":"resp_1","created_at":-0,"output":[]}`),
		"zero decimal":    []byte(`{"id":"resp_1","created_at":0.0,"output":[]}`),
		"integer decimal": []byte(`{"id":"resp_1","created_at":1.0,"output":[]}`),
		"fractional":      []byte(`{"id":"resp_1","created_at":0.5,"output":[]}`),
		"exponent":        []byte(`{"id":"resp_1","created_at":1e3,"output":[]}`),
		"overflow":        []byte(`{"id":"resp_1","created_at":9223372036854775808,"output":[]}`),
	} {
		t.Run(name, func(t *testing.T) {
			payload, ok := buildOpenAICompactSSEPayload(source)
			require.True(t, ok)
			requirePositiveResponseCreatedAt(t, payload)
		})
	}
	for name, raw := range map[string]string{
		"one":       "1",
		"ordinary":  "123",
		"max int64": "9223372036854775807",
	} {
		t.Run("preserves "+name, func(t *testing.T) {
			payload, ok := buildOpenAICompactSSEPayload([]byte(`{"id":"resp_1","created_at":` + raw + `,"output":[]}`))
			require.True(t, ok)
			require.Equal(t, raw, gjson.GetBytes(payload, "response.created_at").Raw)
		})
	}
}

func TestResponsesCreatedAtCompatibility_FailedPayloadsAreJSON(t *testing.T) {
	body := buildOpenAIWSHTTPBridgeFailedEvent("resp_1", "", nil, "failed")
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))

	responseID := "resp_\x01\"\n{"
	fallback := buildOpenAIResponseFailedFallbackPayload(responseID)
	require.True(t, gjson.ValidBytes(fallback))
	requireSyntheticResponseFields(t, fallback, responseID)
}

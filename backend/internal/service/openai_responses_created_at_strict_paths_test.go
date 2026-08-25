package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const responsesCreatedAtLargeVendorInteger = "900719925474099312345"

func requireStrictPositiveIntegerJSONPath(t *testing.T, body []byte, path string) string {
	t.Helper()
	value := gjson.GetBytes(body, path)
	require.True(t, value.Exists(), path)
	parsed, err := strconv.ParseInt(value.Raw, 10, 64)
	require.NoError(t, err, value.Raw)
	require.Positive(t, parsed)
	return value.Raw
}

func TestNormalizeOpenAIResponsesCreatedAt_StrictPositiveIntegers(t *testing.T) {
	invalid := map[string][]byte{
		"missing":         []byte(`{"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}`),
		"string":          []byte(`{"created_at":"123","vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}`),
		"zero":            []byte(`{"created_at":0,"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}`),
		"negative":        []byte(`{"created_at":-1,"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}`),
		"negative zero":   []byte(`{"created_at":-0,"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}`),
		"zero decimal":    []byte(`{"created_at":0.0,"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}`),
		"integer decimal": []byte(`{"created_at":1.0,"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}`),
		"fractional":      []byte(`{"created_at":0.5,"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}`),
		"exponent":        []byte(`{"created_at":1e3,"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}`),
		"overflow":        []byte(`{"created_at":9223372036854775808,"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}`),
	}
	for name, payload := range invalid {
		t.Run(name, func(t *testing.T) {
			normalized, ok := normalizeOpenAIResponsesCreatedAt(payload, "created_at")
			require.True(t, ok)
			requireStrictPositiveIntegerJSONPath(t, normalized, "created_at")
			require.Equal(t, responsesCreatedAtLargeVendorInteger, gjson.GetBytes(normalized, "vendor_sequence").Raw)
		})
	}

	for name, raw := range map[string]string{
		"one":       "1",
		"ordinary":  "123",
		"max int64": "9223372036854775807",
	} {
		t.Run("preserves "+name, func(t *testing.T) {
			payload := []byte(`{"created_at":` + raw + `,"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}`)
			normalized, ok := normalizeOpenAIResponsesCreatedAt(payload, "created_at")
			require.True(t, ok)
			require.Equal(t, string(payload), string(normalized))
		})
	}
}

func TestNormalizeOpenAIResponsesEventCreatedAt_AllResponseEnvelopes(t *testing.T) {
	for _, eventType := range []string{
		"response.created",
		"response.in_progress",
		"response.completed",
		"response.done",
		"response.incomplete",
		"response.cancelled",
		"response.canceled",
		"response.failed",
	} {
		t.Run(eventType, func(t *testing.T) {
			payload := []byte(`{"type":"` + eventType + `","response":{"id":"resp_1","created_at":0.5,"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `},"root_sequence":` + responsesCreatedAtLargeVendorInteger + `}`)
			normalized, ok := normalizeOpenAIResponsesEventCreatedAt(payload, eventType)
			require.True(t, ok)
			requireStrictPositiveIntegerJSONPath(t, normalized, "response.created_at")
			require.Equal(t, responsesCreatedAtLargeVendorInteger, gjson.GetBytes(normalized, "response.vendor_sequence").Raw)
			require.Equal(t, responsesCreatedAtLargeVendorInteger, gjson.GetBytes(normalized, "root_sequence").Raw)
		})
	}

	delta := []byte(`{"type":"response.output_text.delta","response":{"id":"resp_1"},"delta":"ok"}`)
	normalized, ok := normalizeOpenAIResponsesEventCreatedAt(delta, "response.output_text.delta")
	require.True(t, ok)
	require.Equal(t, string(delta), string(normalized))
}

func TestNormalizeOpenAIResponsesEventCreatedAt_DoesNotCreateResponseObject(t *testing.T) {
	for name, payload := range map[string][]byte{
		"missing":         []byte(`{"type":"response.completed"}`),
		"null":            []byte(`{"type":"response.completed","response":null}`),
		"array":           []byte(`{"type":"response.completed","response":[]}`),
		"string":          []byte(`{"type":"response.completed","response":"invalid"}`),
		"top-level error": []byte(`{"type":"response.failed","error":{"code":"upstream_error","message":"failed"}}`),
	} {
		t.Run(name, func(t *testing.T) {
			normalized, ok := normalizeOpenAIResponsesEventCreatedAt(payload, gjson.GetBytes(payload, "type").String())
			require.True(t, ok)
			require.Equal(t, string(payload), string(normalized))
		})
	}
}

func TestNormalizeOpenAIResponsesEventCreatedAt_SurvivesFinalToolNameRestore(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	setCodexToolNameReverse(c, map[string]string{codexPythonToolAlias: "python"})
	payload := []byte(`{"type":"response.completed","response":{"id":"resp_1","created_at":0.5,"output":[{"type":"function_call","name":"python__sub2api"}],"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}}`)

	normalized, ok := normalizeOpenAIResponsesEventCreatedAt(payload, "response.completed")
	require.True(t, ok)
	restored := restoreCodexToolNamesFromContext(c, normalized)

	requireStrictPositiveIntegerJSONPath(t, restored, "response.created_at")
	require.Equal(t, "python", gjson.GetBytes(restored, "response.output.0.name").String())
	require.Equal(t, responsesCreatedAtLargeVendorInteger, gjson.GetBytes(restored, "response.vendor_sequence").Raw)
}

func responsesCreatedAtStrictSSE(responseID, model string) string {
	return strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"` + responseID + `","object":"response","created_at":0.5,"model":` + strconv.Quote(model) + `,"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `}}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"ok"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"` + responseID + `","object":"response","created_at":1e3,"model":` + strconv.Quote(model) + `,"status":"completed","output":[],"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `,"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`,
		"",
	}, "\n")
}

func requireStrictCreatedAtSSELifecycle(t *testing.T, body, expectedModel string) {
	t.Helper()
	events := make(map[string][]byte)
	forEachOpenAISSEFrame(body, func(eventType string, data []byte) {
		eventType = effectiveOpenAISSEEventType(data, eventType)
		if eventType == "response.created" || eventType == "response.completed" {
			events[eventType] = append([]byte(nil), data...)
		}
	})
	for _, eventType := range []string{"response.created", "response.completed"} {
		payload := events[eventType]
		require.NotEmpty(t, payload, eventType)
		requireStrictPositiveIntegerJSONPath(t, payload, "response.created_at")
		require.Equal(t, expectedModel, gjson.GetBytes(payload, "response.model").String())
		require.Equal(t, responsesCreatedAtLargeVendorInteger, gjson.GetBytes(payload, "response.vendor_sequence").Raw)
	}
}

func newResponsesCreatedAtStrictService() *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		}},
		toolCorrector: NewCodexToolCorrector(),
	}
}

func TestResponsesCreatedAtCompatibility_HTTPStreamingPaths(t *testing.T) {
	account := &Account{ID: 1, Name: "account", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	t.Run("main", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(responsesCreatedAtStrictSSE("resp_http_main", "mapped-model")))}
		_, err := newResponsesCreatedAtStrictService().handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "client-model", "mapped-model")
		require.NoError(t, err)
		requireStrictCreatedAtSSELifecycle(t, recorder.Body.String(), "client-model")
	})

	t.Run("passthrough", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(responsesCreatedAtStrictSSE("resp_http_passthrough", "mapped-model")))}
		_, err := newResponsesCreatedAtStrictService().handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "client-model", "mapped-model")
		require.NoError(t, err)
		requireStrictCreatedAtSSELifecycle(t, recorder.Body.String(), "client-model")
	})
}

func TestResponsesCreatedAtCompatibility_NonStreamingPaths(t *testing.T) {
	account := &Account{ID: 1, Name: "account", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := `{"id":"resp_json","object":"response","created_at":1e3,"model":"model","status":"completed","output":[],"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `,"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`

	t.Run("main", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
		_, err := newResponsesCreatedAtStrictService().handleNonStreamingResponse(context.Background(), resp, c, account, "model", "model")
		require.NoError(t, err)
		requireStrictPositiveIntegerJSONPath(t, recorder.Body.Bytes(), "created_at")
		require.Equal(t, responsesCreatedAtLargeVendorInteger, gjson.Get(recorder.Body.String(), "vendor_sequence").Raw)
	})

	t.Run("passthrough", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
		_, err := newResponsesCreatedAtStrictService().handleNonStreamingResponsePassthrough(context.Background(), resp, c, account, "model", "model")
		require.NoError(t, err)
		requireStrictPositiveIntegerJSONPath(t, recorder.Body.Bytes(), "created_at")
		require.Equal(t, responsesCreatedAtLargeVendorInteger, gjson.Get(recorder.Body.String(), "vendor_sequence").Raw)
	})
}

func TestResponsesCreatedAtCompatibility_SSEToJSONPaths(t *testing.T) {
	account := &Account{ID: 1, Name: "account", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(responsesCreatedAtStrictSSE("resp_sse_json", "mapped-model"))

	t.Run("main", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
		_, err := newResponsesCreatedAtStrictService().handleSSEToJSON(resp, c, account, body, "client-model", "mapped-model")
		require.NoError(t, err)
		requireStrictPositiveIntegerJSONPath(t, recorder.Body.Bytes(), "created_at")
		require.Equal(t, "client-model", gjson.GetBytes(recorder.Body.Bytes(), "model").String())
		require.Equal(t, responsesCreatedAtLargeVendorInteger, gjson.GetBytes(recorder.Body.Bytes(), "vendor_sequence").Raw)
	})

	t.Run("passthrough", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
		_, err := newResponsesCreatedAtStrictService().handlePassthroughSSEToJSON(resp, c, account, body, "client-model", "mapped-model")
		require.NoError(t, err)
		requireStrictPositiveIntegerJSONPath(t, recorder.Body.Bytes(), "created_at")
		require.Equal(t, "client-model", gjson.GetBytes(recorder.Body.Bytes(), "model").String())
		require.Equal(t, responsesCreatedAtLargeVendorInteger, gjson.GetBytes(recorder.Body.Bytes(), "vendor_sequence").Raw)
	})
}

func TestResponsesTerminalOutputCompatibility_SSEToJSONThinTerminalPaths(t *testing.T) {
	account := &Account{ID: 1, Name: "account", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(strings.Join([]string{
		"event: response.output_item.done",
		`data: {"output_index":0,"item":{"type":"function_call","id":"fc_exec","call_id":"call_exec","name":"exec","arguments":"{\"input\":\"echo hi\"}","status":"completed","vendor_counter":` + responsesCreatedAtLargeVendorInteger + `,"vendor_extension":{"trace":"keep"}}}`,
		"",
		"event: response.completed",
		`data: {"response":{"id":"resp_thin","object":"response","created_at":1e3,"model":"mapped-model","status":"completed","output":[{"type":"function_call","id":"fc_exec","call_id":"call_exec","name":"exec"}],"vendor_sequence":` + responsesCreatedAtLargeVendorInteger + `,"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`,
		"",
	}, "\n"))

	for _, tc := range []struct {
		name string
		run  func(*OpenAIGatewayService, *http.Response, *gin.Context) error
	}{
		{
			name: "main",
			run: func(svc *OpenAIGatewayService, resp *http.Response, c *gin.Context) error {
				_, err := svc.handleSSEToJSON(resp, c, account, body, "client-model", "mapped-model")
				return err
			},
		},
		{
			name: "passthrough",
			run: func(svc *OpenAIGatewayService, resp *http.Response, c *gin.Context) error {
				_, err := svc.handlePassthroughSSEToJSON(resp, c, account, body, "client-model", "mapped-model")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}

			require.NoError(t, tc.run(newResponsesCreatedAtStrictService(), resp, c))
			output := recorder.Body.Bytes()
			require.Equal(t, "client-model", gjson.GetBytes(output, "model").String())
			requireStrictPositiveIntegerJSONPath(t, output, "created_at")
			require.Equal(t, `{"input":"echo hi"}`, gjson.GetBytes(output, "output.0.arguments").String())
			require.Equal(t, "completed", gjson.GetBytes(output, "output.0.status").String())
			require.Equal(t, "keep", gjson.GetBytes(output, "output.0.vendor_extension.trace").String())
			require.Equal(t, responsesCreatedAtLargeVendorInteger, gjson.GetBytes(output, "output.0.vendor_counter").Raw)
			require.Equal(t, responsesCreatedAtLargeVendorInteger, gjson.GetBytes(output, "vendor_sequence").Raw)
		})
	}
}

func TestResponsesCreatedAtCompatibility_WSHTTPBridgePath(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(responsesCreatedAtStrictSSE("resp_ws_bridge", "mapped-model"))),
	}}
	svc := newResponsesCreatedAtStrictService()
	svc.httpUpstream = upstream
	account := &Account{ID: 7, Name: "api-key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Status: StatusActive}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	var writes [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "sk-test",
		[]byte(`{"type":"response.create","model":"mapped-model","input":"hi","stream":true}`),
		len(`{"type":"response.create","model":"mapped-model","input":"hi","stream":true}`),
		"client-model", "", "", "", "", 1,
		func(message []byte) error {
			writes = append(writes, append([]byte(nil), message...))
			return nil
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, writes, 3)
	for _, index := range []int{0, 2} {
		requireStrictPositiveIntegerJSONPath(t, writes[index], "response.created_at")
		require.Equal(t, "client-model", gjson.GetBytes(writes[index], "response.model").String())
		require.Equal(t, responsesCreatedAtLargeVendorInteger, gjson.GetBytes(writes[index], "response.vendor_sequence").Raw)
	}
}

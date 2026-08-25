//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const webSearchActionEntryLargeVendorInteger = "900719925474099312345"

func webSearchActionEntryFrame(eventType, payload string) string {
	return "event: " + eventType + "\n" + "data: " + payload + "\n\n"
}

func webSearchActionEntrySSE() string {
	action := `{"type":"search","query":"weather","sources":[{"url":"https://example.test","vendor_rank":` + webSearchActionEntryLargeVendorInteger + `}],"vendor_extension":{"trace":"keep"}}`
	return strings.Join([]string{
		webSearchActionEntryFrame("response.output_item.added", `{"sequence_number":0,"output_index":0,"item":{"type":"web_search_call","id":"ws_1","call_id":"c_ws","status":"in_progress"}}`),
		webSearchActionEntryFrame("response.output_item.added", `{"sequence_number":1,"output_index":1,"item":{"type":"x_search_call","id":"xs_1","call_id":"c_x","status":"in_progress"}}`),
		webSearchActionEntryFrame("response.output_item.added", `{"sequence_number":2,"output_index":2,"item":{"type":"tool_search_call","id":"ts_1","call_id":"c_tool","status":"in_progress"}}`),
		webSearchActionEntryFrame("response.output_item.added", `{"sequence_number":3,"output_index":3,"item":{"type":"function_call","id":"fc_1","call_id":"c_fn","name":"lookup","status":"in_progress"}}`),
		webSearchActionEntryFrame("response.output_item.added", `{"sequence_number":4,"output_index":4,"item":{"type":"custom_tool_call","id":"ctc_1","call_id":"c_custom","name":"lookup","status":"in_progress"}}`),
		webSearchActionEntryFrame("response.web_search_call.completed", `{"sequence_number":5,"output_index":0,"item_id":"ws_1"}`),
		webSearchActionEntryFrame("response.output_item.done", `{"sequence_number":6,"output_index":0,"item":{"type":"web_search_call","id":"ws_1","call_id":"c_ws","status":"completed","action":`+action+`}}`),
		webSearchActionEntryFrame("response.output_item.done", `{"sequence_number":7,"output_index":1,"item":{"type":"x_search_call","id":"xs_1","call_id":"c_x","status":"completed","vendor_extension":{"trace":"x-keep"}}}`),
		webSearchActionEntryFrame("response.output_item.done", `{"sequence_number":8,"output_index":2,"item":{"type":"tool_search_call","id":"ts_1","call_id":"c_tool","status":"completed","vendor_extension":{"trace":"tool-keep"}}}`),
		webSearchActionEntryFrame("response.output_item.done", `{"sequence_number":9,"output_index":3,"item":{"type":"function_call","id":"fc_1","call_id":"c_fn","name":"lookup","arguments":"{}","status":"completed","vendor_extension":{"trace":"function-keep"}}}`),
		webSearchActionEntryFrame("response.output_item.done", `{"sequence_number":10,"output_index":4,"item":{"type":"custom_tool_call","id":"ctc_1","call_id":"c_custom","name":"lookup","input":"opaque","status":"completed","vendor_extension":{"trace":"custom-keep"}}}`),
		webSearchActionEntryFrame("response.completed", `{"sequence_number":11,"response":{"id":"resp_search_entries","object":"response","created_at":1e3,"model":"mapped-model","status":"completed","output":[{"type":"web_search_call","id":"ws_1","call_id":"c_ws"},{"type":"x_search_call","id":"xs_1","call_id":"c_x"},{"type":"tool_search_call","id":"ts_1","call_id":"c_tool"},{"type":"function_call","id":"fc_1","call_id":"c_fn","name":"lookup"},{"type":"custom_tool_call","id":"ctc_1","call_id":"c_custom","name":"lookup"}],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`),
	}, "")
}

func collectWebSearchActionEntryEvents(t *testing.T, body string) [][]byte {
	t.Helper()
	var events [][]byte
	forEachOpenAISSEFrame(body, func(eventType string, data []byte) {
		if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
			return
		}
		payload := []byte(openAICompatPayloadWithEventType(string(data), eventType))
		require.True(t, gjson.ValidBytes(payload), "client SSE data must remain valid JSON: %s", data)
		events = append(events, append([]byte(nil), payload...))
	})
	return events
}

func assertWebSearchActionEntryEvents(t *testing.T, events [][]byte) {
	t.Helper()
	expectedTypes := []string{
		"response.output_item.added",
		"response.output_item.added",
		"response.output_item.added",
		"response.output_item.added",
		"response.output_item.added",
		"response.web_search_call.completed",
		"response.output_item.done",
		"response.output_item.done",
		"response.output_item.done",
		"response.output_item.done",
		"response.output_item.done",
		"response.completed",
	}
	require.Len(t, events, len(expectedTypes))
	for index, eventType := range expectedTypes {
		require.Equal(t, eventType, gjson.GetBytes(events[index], "type").String(), "unexpected event order at index %d", index)
	}

	for _, assertion := range []struct {
		payload []byte
		path    string
	}{
		{events[0], "item.action"},
		{events[5], "action"},
		{events[6], "item.action"},
		{events[11], "response.output.0.action"},
	} {
		require.Equal(t, "search", gjson.GetBytes(assertion.payload, assertion.path+".type").String())
		require.Equal(t, "weather", gjson.GetBytes(assertion.payload, assertion.path+".query").String())
		require.Equal(t, "keep", gjson.GetBytes(assertion.payload, assertion.path+".vendor_extension.trace").String())
		require.Equal(t, webSearchActionEntryLargeVendorInteger, gjson.GetBytes(assertion.payload, assertion.path+".sources.0.vendor_rank").Raw)
	}

	for index := 1; index <= 4; index++ {
		require.False(t, gjson.GetBytes(events[index], "item.action").Exists(), "non-hosted added item %d must remain transparent", index)
	}
	for index := 7; index <= 10; index++ {
		require.False(t, gjson.GetBytes(events[index], "item.action").Exists(), "non-hosted done item %d must remain transparent", index)
	}
	for index := 1; index <= 4; index++ {
		require.False(t, gjson.GetBytes(events[11], "response.output."+strconv.Itoa(index)+".action").Exists(), "non-hosted terminal item %d must remain transparent", index)
	}

	require.Equal(t, "completed", gjson.GetBytes(events[11], "response.output.0.status").String())
	require.Equal(t, "x-keep", gjson.GetBytes(events[11], "response.output.1.vendor_extension.trace").String())
	require.Equal(t, "tool-keep", gjson.GetBytes(events[11], "response.output.2.vendor_extension.trace").String())
	require.Equal(t, `{}`, gjson.GetBytes(events[11], "response.output.3.arguments").String())
	require.Equal(t, "function-keep", gjson.GetBytes(events[11], "response.output.3.vendor_extension.trace").String())
	require.Equal(t, "opaque", gjson.GetBytes(events[11], "response.output.4.input").String())
	require.Equal(t, "custom-keep", gjson.GetBytes(events[11], "response.output.4.vendor_extension.trace").String())
	requireStrictPositiveIntegerJSONPath(t, events[11], "response.created_at")
}

func TestResponsesWebSearchActionCompatibility_HTTPStreamingEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name              string
		body              []byte
		account           func() *Account
		run               func(*OpenAIGatewayService, *gin.Context, *Account, []byte) (*OpenAIForwardResult, error)
		expectedSearchCnt int
	}{
		{
			name: "standard",
			body: []byte(`{"model":"gpt-5.4","input":"weather","stream":true,"tools":[{"type":"web_search"}]}`),
			account: func() *Account {
				return &Account{
					ID:       5701,
					Platform: PlatformDeepseek,
					Type:     AccountTypeAPIKey,
					Credentials: map[string]any{
						"api_key":      "test-key",
						"api_protocol": APIProtocolResponses,
						"base_url":     "https://relay.example",
					},
				}
			},
			run: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
				return svc.Forward(context.Background(), c, account, body)
			},
		},
		{
			name: "grok",
			body: []byte(`{"model":"grok","input":"weather","stream":true,"tools":[{"type":"web_search"}]}`),
			account: func() *Account {
				return &Account{
					ID:          5702,
					Name:        "grok-api-key",
					Platform:    PlatformGrok,
					Type:        AccountTypeAPIKey,
					Concurrency: 1,
					Credentials: map[string]any{
						"api_key":  "xai-test-key",
						"base_url": "https://api.x.ai/v1",
					},
				}
			},
			run: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
				return svc.forwardGrokResponses(context.Background(), c, account, body, "grok", true, time.Now())
			},
			expectedSearchCnt: 3,
		},
		{
			name: "passthrough",
			body: []byte(`{"model":"gpt-5.4","input":"weather","stream":true,"tools":[{"type":"web_search"}]}`),
			account: func() *Account {
				return &Account{
					ID:          5703,
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Concurrency: 1,
					Credentials: map[string]any{"api_key": "test-key"},
				}
			},
			run: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
				return svc.forwardOpenAIPassthrough(context.Background(), c, account, body, body, "gpt-5.4", false, nil, true, time.Now())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(webSearchActionEntrySSE())),
			}}
			svc := openAIClientToolsTestService(upstream)
			result, err := tc.run(svc, c, tc.account(), tc.body)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.Stream)
			require.Equal(t, "resp_search_entries", result.ResponseID)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
			if tc.expectedSearchCnt > 0 {
				require.Equal(t, tc.expectedSearchCnt, result.SearchCount)
			}
			assertWebSearchActionEntryEvents(t, collectWebSearchActionEntryEvents(t, recorder.Body.String()))
		})
	}
}

func TestHandleStreamingResponsePassthroughCompletesEventOnlyThinTerminalOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	action := `{"type":"search","query":"weather","sources":[{"url":"https://example.test","vendor_rank":` + webSearchActionEntryLargeVendorInteger + `}],"vendor_extension":{"trace":"keep"}}`
	sse := strings.Join([]string{
		`event: response.output_item.done`,
		`data: {"output_index":0,"item":{"type":"function_call","id":"fc_exec","call_id":"call_exec","name":"exec","arguments":"{\"input\":\"echo hi\"}","status":"completed","vendor_counter":` + webSearchActionEntryLargeVendorInteger + `}}`,
		``,
		`event: response.output_item.done`,
		`data: {"output_index":1,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":` + action + `,"vendor_counter":` + webSearchActionEntryLargeVendorInteger + `}}`,
		``,
		`event: response.completed`,
		`data: {"response":{"id":"resp_event_only","object":"response","created_at":1e3,"model":"mapped-model","status":"completed","output":[{"type":"function_call","id":"fc_exec","call_id":"call_exec","name":"exec"},{"type":"web_search_call","id":"ws_1"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
		``,
	}, "\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}
	account := &Account{ID: 5704, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}

	result, err := openAIClientToolsTestService(nil).handleStreamingResponsePassthrough(
		context.Background(), resp, c, account, time.Now(), "client-model", "mapped-model",
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_event_only", result.responseID)
	terminalCount := 0
	var terminal []byte
	forEachOpenAISSEFrame(recorder.Body.String(), func(eventType string, data []byte) {
		if eventType == "response.completed" {
			terminalCount++
			terminal = append([]byte(nil), data...)
		}
	})
	require.Equal(t, 1, terminalCount)
	require.NotEmpty(t, terminal)
	require.Equal(t, "response.completed", gjson.GetBytes(terminal, "type").String())
	require.Equal(t, "client-model", gjson.GetBytes(terminal, "response.model").String())
	require.JSONEq(t, `{"input":"echo hi"}`, gjson.GetBytes(terminal, "response.output.0.arguments").String())
	require.Equal(t, "completed", gjson.GetBytes(terminal, "response.output.0.status").String())
	require.Equal(t, webSearchActionEntryLargeVendorInteger, gjson.GetBytes(terminal, "response.output.0.vendor_counter").Raw)
	require.Equal(t, "search", gjson.GetBytes(terminal, "response.output.1.action.type").String())
	require.Equal(t, "keep", gjson.GetBytes(terminal, "response.output.1.action.vendor_extension.trace").String())
	require.Equal(t, webSearchActionEntryLargeVendorInteger, gjson.GetBytes(terminal, "response.output.1.action.sources.0.vendor_rank").Raw)
	require.Equal(t, "completed", gjson.GetBytes(terminal, "response.output.1.status").String())
	require.Equal(t, webSearchActionEntryLargeVendorInteger, gjson.GetBytes(terminal, "response.output.1.vendor_counter").Raw)
	requireStrictPositiveIntegerJSONPath(t, terminal, "response.created_at")
}

func TestHandleStreamingResponsePassthroughKeepsThinTerminalUnchangedWhenDoneItemCollectorDisables(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fullDoneData := `{"output_index":0,"item":{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"keep"}],"vendor_extension":{"trace":"keep"}}}`
	thinTerminalData := `{"response":{"id":"resp_collector_disabled","object":"response","created_at":1000,"model":"same-model","status":"completed","output":[{"type":"message","id":"msg_1"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`
	tests := []struct {
		name           string
		secondDoneData string
	}{
		{
			name:           "malformed output index",
			secondDoneData: `{"output_index":"1","item":{"type":"message","id":"msg_bad","status":"completed"}}`,
		},
		{
			name:           "conflicting output index",
			secondDoneData: `{"output_index":0,"item":{"type":"message","id":"msg_conflict","status":"completed"}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sse := strings.Join([]string{
				"event: response.output_item.done",
				"data: " + fullDoneData,
				"",
				"event: response.output_item.done",
				"data: " + tc.secondDoneData,
				"",
				"event: response.completed",
				"data: " + thinTerminalData,
				"",
			}, "\n")

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(sse)),
			}
			account := &Account{ID: 5705, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}

			result, err := openAIClientToolsTestService(nil).handleStreamingResponsePassthrough(
				context.Background(), resp, c, account, time.Now(), "same-model", "same-model",
			)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "resp_collector_disabled", result.responseID)

			emittedTypes := make([]string, 0, 3)
			emittedData := make([]string, 0, 3)
			terminalCount := 0
			var terminal []byte
			forEachOpenAISSEFrame(recorder.Body.String(), func(eventType string, data []byte) {
				emittedTypes = append(emittedTypes, eventType)
				emittedData = append(emittedData, string(data))
				if eventType == "response.completed" {
					terminalCount++
					terminal = append([]byte(nil), data...)
				}
			})
			require.Equal(t, []string{
				"response.output_item.done",
				"response.output_item.done",
				"response.completed",
			}, emittedTypes)
			require.Equal(t, []string{fullDoneData, tc.secondDoneData, thinTerminalData}, emittedData)
			require.Equal(t, 1, terminalCount)
			require.Equal(t, `{"type":"message","id":"msg_1"}`, gjson.GetBytes(terminal, "response.output.0").Raw)
			require.False(t, gjson.GetBytes(terminal, "response.output.0.status").Exists())
			require.False(t, gjson.GetBytes(terminal, "response.output.0.role").Exists())
			require.False(t, gjson.GetBytes(terminal, "response.output.0.content").Exists())
			require.False(t, gjson.GetBytes(terminal, "response.output.0.vendor_extension").Exists())
			require.False(t, gjson.GetBytes(terminal, "type").Exists(), "event-line type must remain probe-only when no terminal supplementation occurs")
		})
	}
}

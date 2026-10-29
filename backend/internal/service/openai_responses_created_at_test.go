package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// issue #8118：grok-build（Rust serde）把 Response.created_at 当必填字段，缺失即
// `missing field 'created_at'` 整轮读不出响应。网关送出的每个 Response 对象都必须带它。

// sseDataPayloads 按顺序取出 SSE 文本里的全部 data 负载（跳过 [DONE]）。
func sseDataPayloads(body string) []string {
	var payloads []string
	for _, line := range strings.Split(body, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || strings.TrimSpace(data) == "[DONE]" {
			continue
		}
		payloads = append(payloads, data)
	}
	return payloads
}

func TestResponsesCreatedAtStamper_StampsMissingAndKeepsStreamStable(t *testing.T) {
	var stamper responsesCreatedAtStamper

	created, changed := stamper.Stamp([]byte(`{"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`), "response.created")
	require.True(t, changed)
	first := gjson.GetBytes(created, "response.created_at")
	require.Equal(t, gjson.Number, first.Type)
	require.Positive(t, first.Int())

	completed, changed := stamper.Stamp([]byte(`{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[]}}`), "response.completed")
	require.True(t, changed)
	require.Equal(t, first.Int(), gjson.GetBytes(completed, "response.created_at").Int(), "同一条流的 created_at 必须一致")
}

func TestResponsesCreatedAtStamper_PreservesUpstreamValue(t *testing.T) {
	var stamper responsesCreatedAtStamper
	upstream := []byte(`{"type":"response.created","response":{"id":"resp_1","created_at":1700000000}}`)

	out, changed := stamper.Stamp(upstream, "response.created")
	require.False(t, changed)
	require.Equal(t, string(upstream), string(out), "上游已带 created_at 时字节不变")

	completed, changed := stamper.Stamp([]byte(`{"type":"response.completed","response":{"id":"resp_1"}}`), "response.completed")
	require.True(t, changed)
	require.EqualValues(t, 1700000000, gjson.GetBytes(completed, "response.created_at").Int(), "后续缺失时沿用上游值")
	require.EqualValues(t, 1700000000, stamper.Value())
}

func TestResponsesCreatedAtStamper_NullIsTreatedAsMissing(t *testing.T) {
	var stamper responsesCreatedAtStamper
	out, changed := stamper.Stamp([]byte(`{"type":"response.created","response":{"id":"resp_1","created_at":null}}`), "response.created")
	require.True(t, changed)
	require.Equal(t, gjson.Number, gjson.GetBytes(out, "response.created_at").Type)
}

func TestResponsesCreatedAtStamper_LeavesOtherEventsUntouched(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		payload   string
	}{
		{"delta", "response.output_text.delta", `{"type":"response.output_text.delta","delta":"hi"}`},
		{"bare_error", "error", `{"type":"error","error":{"message":"boom"}}`},
		{"error_with_response_field", "error", `{"type":"error","response":{"id":"resp_1"}}`},
		{"response_not_object", "response.created", `{"type":"response.created","response":"resp_1"}`},
		{"non_numeric_created_at", "response.created", `{"type":"response.created","response":{"created_at":"2026-10-16T00:00:00Z"}}`},
		{"invalid_json", "response.created", `not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stamper responsesCreatedAtStamper
			out, changed := stamper.Stamp([]byte(tc.payload), tc.eventType)
			require.False(t, changed)
			require.Equal(t, tc.payload, string(out))
		})
	}
}

func TestResponsesCreatedAtStamper_ObserveLearnsWithoutStamping(t *testing.T) {
	var stamper responsesCreatedAtStamper
	stamper.Observe([]byte(`{"type":"error","response":{"created_at":1600000000}}`), "error")
	stamper.Observe([]byte(`{"type":"response.created","response":{"created_at":1700000000}}`), "response.created")
	stamper.Observe([]byte(`{"type":"response.in_progress","response":{"created_at":1800000000}}`), "response.in_progress")
	require.EqualValues(t, 1700000000, stamper.Value(), "记首个生命周期事件的上游值")
}

func TestEnsureOpenAIResponseObjectCreatedAt(t *testing.T) {
	stamped := ensureOpenAIResponseObjectCreatedAt([]byte(`{"id":"resp_1","object":"response","status":"completed"}`))
	require.Equal(t, gjson.Number, gjson.GetBytes(stamped, "created_at").Type)
	require.Positive(t, gjson.GetBytes(stamped, "created_at").Int())

	for _, body := range []string{
		`{"id":"resp_1","object":"response","created_at":1700000000}`,
		`{"id":"cmp_1","object":"response.compaction","output":[]}`,
		`{"id":"resp_1","output":[]}`,
		`not json`,
	} {
		require.Equal(t, body, string(ensureOpenAIResponseObjectCreatedAt([]byte(body))))
	}
}

func TestBuildOpenAIResponseFailedSSE_CarriesCreatedAtAndSequenceNumber(t *testing.T) {
	frame := buildOpenAIResponseFailedSSE("resp_1", "gpt-5.5", 1700000000,
		[]byte(`{"type":"error","sequence_number":7,"error":{"code":"server_is_overloaded","message":"busy"}}`), "")
	payloads := sseDataPayloads(frame)
	require.Len(t, payloads, 1)
	event := payloads[0]
	require.Equal(t, "response.failed", gjson.Get(event, "type").String())
	require.EqualValues(t, 7, gjson.Get(event, "sequence_number").Int(), "沿用被替换的裸 error 帧序号")
	require.EqualValues(t, 1700000000, gjson.Get(event, "response.created_at").Int())
	require.Equal(t, "resp_1", gjson.Get(event, "response.id").String())
	require.Equal(t, "failed", gjson.Get(event, "response.status").String())

	frame = buildOpenAIResponseFailedSSE("", "", 0, []byte(`{"type":"error","error":{"message":"boom"}}`), "")
	event = sseDataPayloads(frame)[0]
	require.True(t, gjson.Get(event, "sequence_number").Exists(), "sequence_number 未知时也必须写出")
	require.EqualValues(t, 0, gjson.Get(event, "sequence_number").Int())
	require.Positive(t, gjson.Get(event, "response.created_at").Int(), "未知 created_at 时取当前时间")
}

func TestBuildOpenAIWSHTTPBridgeFailedEvent_CarriesCreatedAtAndSequenceNumber(t *testing.T) {
	event := buildOpenAIWSHTTPBridgeFailedEvent("resp_1", "gpt-5.5", 1700000000,
		[]byte(`{"type":"error","sequence_number":4,"error":{"message":"boom"}}`), "")
	require.Equal(t, "response.failed", gjson.GetBytes(event, "type").String())
	require.EqualValues(t, 4, gjson.GetBytes(event, "sequence_number").Int())
	require.EqualValues(t, 1700000000, gjson.GetBytes(event, "response.created_at").Int())

	event = buildOpenAIWSHTTPBridgeFailedEvent("resp_1", "", 0, []byte(`{"type":"error","error":{"message":"boom"}}`), "")
	require.EqualValues(t, 0, gjson.GetBytes(event, "sequence_number").Int())
	require.Positive(t, gjson.GetBytes(event, "response.created_at").Int())
}

// Grok 账号挂在不发 created_at 的上游（第三方中转）后面：每个生命周期事件都要补齐，
// 且同一条流取同一个值；增量事件字节不变。
func TestHandleStreamingResponse_GrokStampsMissingCreatedAt(t *testing.T) {
	delta := `{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"hi"}`
	body := "event: response.created\n" +
		`data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_grok","object":"response","model":"grok-4.5","status":"in_progress","output":[]}}` + "\n\n" +
		"event: response.in_progress\n" +
		`data: {"type":"response.in_progress","sequence_number":1,"response":{"id":"resp_grok","object":"response","model":"grok-4.5","status":"in_progress","output":[]}}` + "\n\n" +
		"event: response.output_text.delta\n" +
		"data: " + delta + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","sequence_number":3,"response":{"id":"resp_grok","object":"response","model":"grok-4.5","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hi","annotations":[]}]}],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}}` + "\n\n"
	recorder := newOpenAIResponseFlushRecorder()
	account := &Account{ID: 8118, Platform: PlatformGrok, Type: AccountTypeAPIKey}

	result, err := runOpenAIResponseFlushTestWithAccount(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{}, account)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 3, result.usage.InputTokens)
	gotBody, _ := recorder.snapshot()
	payloads := sseDataPayloads(gotBody)
	require.Len(t, payloads, 4)
	createdAt := gjson.Get(payloads[0], "response.created_at")
	require.Equal(t, gjson.Number, createdAt.Type)
	require.Positive(t, createdAt.Int())
	for _, index := range []int{1, 3} {
		require.Equal(t, createdAt.Int(), gjson.Get(payloads[index], "response.created_at").Int(), "payload %d", index)
	}
	require.Equal(t, delta, payloads[2], "非生命周期事件不得改动")
	require.Equal(t, "hi", gjson.Get(payloads[3], "response.output.0.content.0.text").String())
}

// 官方上游自带 created_at：网关输出必须与上游逐字节一致（不引入任何改写）。
func TestHandleStreamingResponse_GrokKeepsUpstreamCreatedAtBytes(t *testing.T) {
	body := "event: response.created\n" +
		`data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_grok","object":"response","created_at":1774274151,"model":"grok-4.5","status":"in_progress","output":[]}}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","sequence_number":1,"response":{"id":"resp_grok","object":"response","created_at":1774274151,"model":"grok-4.5","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hi","annotations":[]}]}],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}}` + "\n\n"
	recorder := newOpenAIResponseFlushRecorder()
	account := &Account{ID: 8118, Platform: PlatformGrok, Type: AccountTypeOAuth}

	_, err := runOpenAIResponseFlushTestWithAccount(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{}, account)

	require.NoError(t, err)
	gotBody, _ := recorder.snapshot()
	require.Equal(t, body, gotBody)
}

// OpenAI OAuth 流以裸 error 结束：合成的 response.failed 必须带 created_at
// （与本流 response.created 一致）和 sequence_number。
func TestHandleStreamingResponse_BareErrorSynthesizedFailedCarriesCreatedAt(t *testing.T) {
	body := `data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_bare","object":"response","created_at":1700000000,"status":"in_progress","output":[]}}` + "\n\n" +
		`data: {"type":"response.output_text.delta","sequence_number":1,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"partial"}` + "\n\n" +
		`data: {"type":"error","sequence_number":2,"error":{"code":"server_is_overloaded","message":"busy"}}` + "\n\n" +
		"data: [DONE]\n\n"
	recorder := newOpenAIResponseFlushRecorder()

	result, err := runOpenAIResponseFlushTest(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{})

	require.Error(t, err)
	require.NotNil(t, result)
	gotBody, _ := recorder.snapshot()
	require.NotContains(t, gotBody, `"type":"error"`)
	var failed string
	for _, payload := range sseDataPayloads(gotBody) {
		if gjson.Get(payload, "type").String() == "response.failed" {
			failed = payload
		}
	}
	require.NotEmpty(t, failed, "必须合成 response.failed: %s", gotBody)
	require.EqualValues(t, 1700000000, gjson.Get(failed, "response.created_at").Int())
	require.EqualValues(t, 2, gjson.Get(failed, "sequence_number").Int())
	require.Equal(t, "resp_bare", gjson.Get(failed, "response.id").String())
}

func TestHandleStreamingResponsePassthrough_BareErrorSynthesizedFailedCarriesCreatedAt(t *testing.T) {
	upstream := `data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_pt","object":"response","created_at":1700000000,"status":"in_progress","output":[]}}` + "\n\n" +
		"event: error\n" +
		`data: {"type":"error","sequence_number":1,"error":{"code":"content_policy","message":"request blocked by policy"}}` + "\n\n" +
		"data: [DONE]\n\n"

	_, recorder, _, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

	require.Error(t, err)
	var failed string
	for _, payload := range sseDataPayloads(recorder.Body.String()) {
		if gjson.Get(payload, "type").String() == "response.failed" {
			failed = payload
		}
	}
	require.NotEmpty(t, failed, "必须合成 response.failed: %s", recorder.Body.String())
	require.EqualValues(t, 1700000000, gjson.Get(failed, "response.created_at").Int())
	require.EqualValues(t, 1, gjson.Get(failed, "sequence_number").Int())
}

func TestProxyOpenAIWSHTTPBridgeTurn_GrokStampsMissingCreatedAt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_ws","object":"response","model":"grok-4.5","status":"in_progress"}}`,
			"",
			`data: {"type":"response.completed","sequence_number":1,"response":{"id":"resp_ws","object":"response","model":"grok-4.5","status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
		}, "\n"))),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: upstream,
	}
	account := &Account{ID: 8118, Platform: PlatformGrok, Type: AccountTypeOAuth, Concurrency: 1}
	payload := []byte(`{"type":"response.create","model":"grok-4.5","stream":true,"input":"hi"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	var events [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "access-token", payload, len(payload),
		"grok-4.5", "", "", "", "", 1,
		func(message []byte) error {
			events = append(events, append([]byte(nil), message...))
			return nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, events, 2)
	createdAt := gjson.GetBytes(events[0], "response.created_at")
	require.Equal(t, gjson.Number, createdAt.Type)
	require.Positive(t, createdAt.Int())
	require.Equal(t, createdAt.Int(), gjson.GetBytes(events[1], "response.created_at").Int())
}

func TestProxyOpenAIWSHTTPBridgeTurn_BareErrorSynthesizedFailedCarriesCreatedAt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_eof","created_at":1700000000,"status":"in_progress"}}` + "\n\n" +
		`data: {"type":"error","sequence_number":1,"error":{"code":"invalid_request","message":"bad request"}}` + "\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 112, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
	var writes [][]byte

	_, err := svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "sk-test", payload, len(payload), "gpt-5", "", "", "", "", 2, func(message []byte) error {
		writes = append(writes, append([]byte(nil), message...))
		return nil
	})

	require.EqualError(t, err, "bad request")
	require.Len(t, writes, 2)
	require.Equal(t, "response.failed", gjson.GetBytes(writes[1], "type").String())
	require.EqualValues(t, 1700000000, gjson.GetBytes(writes[1], "response.created_at").Int())
	require.EqualValues(t, 1, gjson.GetBytes(writes[1], "sequence_number").Int())
}

func TestHandleNonStreamingResponse_StampsMissingCreatedAt(t *testing.T) {
	run := func(t *testing.T, upstreamBody string) string {
		t.Helper()
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		svc := &OpenAIGatewayService{cfg: &config.Config{}, toolCorrector: NewCodexToolCorrector()}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(upstreamBody)),
		}
		result, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 8118, Platform: PlatformGrok, Type: AccountTypeAPIKey}, "grok-4.5", "grok-4.5")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, 3, result.usage.InputTokens)
		return rec.Body.String()
	}

	missing := run(t, `{"id":"resp_1","object":"response","model":"grok-4.5","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}`)
	require.Equal(t, gjson.Number, gjson.Get(missing, "created_at").Type)
	require.Positive(t, gjson.Get(missing, "created_at").Int())

	present := `{"id":"resp_1","object":"response","created_at":1700000000,"model":"grok-4.5","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}`
	require.Equal(t, present, run(t, present))
}

func TestHandleSSEToJSON_StampsMissingCreatedAt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, toolCorrector: NewCodexToolCorrector()}
	body := `data: {"type":"response.completed","response":{"id":"resp_1","object":"response","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hi","annotations":[]}]}],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}}` + "\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	result, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 8118, Platform: PlatformGrok, Type: AccountTypeOAuth}, "grok-4.5", "grok-4.5")

	require.NoError(t, err)
	require.NotNil(t, result)
	out := rec.Body.String()
	require.Equal(t, "resp_1", gjson.Get(out, "id").String())
	require.Positive(t, gjson.Get(out, "created_at").Int())
}

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func deepSeekResponsesTestAccount() *Account {
	return &Account{
		ID:          11,
		Name:        "deepseek-apikey",
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"api_protocol": APIProtocolResponses,
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func deepSeekResponsesTestContext(t *testing.T, body []byte) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func deepSeekResponsesTestUpstream() *httpUpstreamRecorder {
	return &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`,
		)),
	}}
}

// TestForwardDeepSeekResponsesInjectsReasoningPlaceholders 覆盖真实转发链路：
// Codex 历史里 assistant 消息缺少 reasoning 明文时，DeepSeek 原生 Responses 端点会以
// 400 "reasoning_text ... must be passed back" 拒绝，网关必须在上游收到请求前补非空占位。
func TestForwardDeepSeekResponsesInjectsReasoningPlaceholders(t *testing.T) {
	body := []byte(`{"model":"deepseek-flash","stream":false,` + deepSeekResponsesToolsFragment + `,"input":[` +
		`{"type":"message","role":"user","content":"go"},` +
		`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":""}]},` +
		`{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"c1","output":"ok"}]}`)

	upstream := deepSeekResponsesTestUpstream()
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c := deepSeekResponsesTestContext(t, body)

	_, err := svc.Forward(context.Background(), c, deepSeekResponsesTestAccount(), body)
	require.NoError(t, err)
	require.Equal(t, "https://api.deepseek.com/responses", upstream.lastReq.URL.String())
	assertAssistantMessagesGuarded(t, upstream.lastBody)
	require.Equal(t, "rs_ph_msg_1", gjson.GetBytes(upstream.lastBody, "input.1.id").String())
}

// TestForwardDeepSeekResponsesSkipsPlaceholderWithoutTools 确认补齐只发生在带 tools 的请求上：
// 不带 tools 时 DeepSeek 会忽略 reasoning item，改写请求体反而会破坏上下文缓存前缀。
func TestForwardDeepSeekResponsesSkipsPlaceholderWithoutTools(t *testing.T) {
	body := []byte(`{"model":"deepseek-flash","stream":false,"input":[` +
		`{"type":"message","role":"user","content":"go"},` +
		`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":""}]}]}`)

	upstream := deepSeekResponsesTestUpstream()
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c := deepSeekResponsesTestContext(t, body)

	_, err := svc.Forward(context.Background(), c, deepSeekResponsesTestAccount(), body)
	require.NoError(t, err)
	// store=false 等无状态字段归一化仍会改写请求体，这里只断言没有插入占位 reasoning。
	require.Len(t, gjson.GetBytes(upstream.lastBody, "input").Array(), 2)
	require.Equal(t, "message", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
	require.False(t, strings.Contains(string(upstream.lastBody), responsesReasoningPlaceholderIDPrefix))
}

// TestForwardDeepSeekResponsesInjectsPlaceholderForTypeLessAssistantMessage 覆盖线上真实形态：
// Responses 允许 message 省略 type 字段，DeepSeek 按 role 识别它并要求 reasoning 明文，
// 补齐必须同样认得出来，否则请求仍会被 400 拒绝。
func TestForwardDeepSeekResponsesInjectsPlaceholderForTypeLessAssistantMessage(t *testing.T) {
	body := []byte(`{"model":"deepseek-flash","stream":false,` + deepSeekResponsesToolsFragment + `,"input":[` +
		`{"role":"user","content":"go"},` +
		`{"role":"assistant","content":[{"type":"output_text","text":""}]},` +
		`{"type":"function_call","id":"fc_1","call_id":"c1","name":"shell","arguments":"{}","status":"completed"},` +
		`{"type":"function_call_output","id":"fco_1","call_id":"c1","output":"ok","status":"completed"}]}`)

	upstream := deepSeekResponsesTestUpstream()
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c := deepSeekResponsesTestContext(t, body)

	_, err := svc.Forward(context.Background(), c, deepSeekResponsesTestAccount(), body)
	require.NoError(t, err)
	require.Equal(t, "https://api.deepseek.com/responses", upstream.lastReq.URL.String())
	require.Len(t, gjson.GetBytes(upstream.lastBody, "input").Array(), 5)
	require.Equal(t, "reasoning", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
	require.Equal(t, " ", gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String())
	assertAssistantMessagesGuarded(t, upstream.lastBody)
}

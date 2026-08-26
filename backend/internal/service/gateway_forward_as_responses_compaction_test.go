package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// anthropicSummarySSE 构造一条上游 Anthropic 流：模型把摘要作为普通文本输出。
func anthropicSummarySSE(text string) string {
	block, _ := json.Marshal(text)
	return strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_cmp","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":244812,"output_tokens":0}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":` + string(block) + `}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1200}}`,
		``,
	}, "\n")
}

// compactionTestUpstream 是本文件自用的 HTTPUpstream 桩。不复用
// account_test_service_openai_test.go 的 queuedHTTPUpstream：那个文件在
// //go:build unit 标签下，而该标签的构建目前被 wire_merge_preservation_test.go
// 的既有签名不匹配打断，会让这些测试根本跑不起来。
type compactionTestUpstream struct {
	responses []*http.Response
	requests  []*http.Request
	bodies    [][]byte
}

func (u *compactionTestUpstream) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, concurrency, nil)
}

func (u *compactionTestUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.requests = append(u.requests, req)
	var captured []byte
	if req != nil && req.Body != nil {
		captured, _ = io.ReadAll(req.Body)
	}
	u.bodies = append(u.bodies, captured)
	if len(u.responses) == 0 {
		return nil, fmt.Errorf("no mocked response")
	}
	resp := u.responses[0]
	u.responses = u.responses[1:]
	return resp, nil
}

func newCompactionUpstream(body string) *http.Response {
	return &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cmp"}},
		Body:   io.NopCloser(strings.NewReader(body)),
	}
}

// 流式客户端必须收到恰好一个 compaction item 与 response.completed，
// 且 encrypted_content 非空——这两条是 Codex compact_remote_v2 的硬要求。
func TestHandleResponsesCompactionResponse_StreamEmitsSingleCompactionItem(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	const summary = "<summary>\n1. Primary Request and Intent: 修复 compaction\n</summary>"
	svc := &GatewayService{}
	result, err := svc.handleResponsesCompactionResponse(
		newCompactionUpstream(anthropicSummarySSE(summary)),
		c, "gpt-5.6-sol", "claude-sonnet-4.5", nil, time.Now(), true,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)

	out := rec.Body.String()
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Contains(t, out, "event: response.output_item.done")
	require.Contains(t, out, "event: response.completed")

	// Codex 只从 output_item.done 收集 item，且要求恰好一个 compaction。
	// （completed 事件里嵌的完整 response 对象会重复出现同一 item，不计入。）
	items := collectSSEItems(t, out)
	require.Len(t, items, 1)
	require.Equal(t, "compaction", gjson.Get(items[0], "type").String())
	require.Equal(t, "completed", gjson.Get(items[0], "status").String())

	envelope := gjson.Get(items[0], "encrypted_content").String()
	require.NotEmpty(t, envelope)
	decoded, ok := apicompat.DecodeCompactionEnvelope(envelope)
	require.True(t, ok, "网关必须能解回自己写的信封，否则下一轮前文全丢")
	require.Equal(t, summary, decoded)

	// 明文 summary 同时保留，便于回放时免解码。
	require.Equal(t, summary, gjson.Get(items[0], "summary.0.text").String())

	// response.completed 必须带可被 Codex 解析的 usage（三个整数字段）。
	completed := lastSSEData(t, out)
	require.Equal(t, float64(1200), gjson.Get(completed, "response.usage.output_tokens").Num)
	require.Equal(t, 1, len(gjson.Get(completed, "response.output").Array()))
}

// 非流式客户端拿 JSON，output 同样恰好一项。
func TestHandleResponsesCompactionResponse_NonStreamEmitsJSON(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GatewayService{}
	result, err := svc.handleResponsesCompactionResponse(
		newCompactionUpstream(anthropicSummarySSE("摘要正文")),
		c, "gpt-5.6-sol", "claude-sonnet-4.5", nil, time.Now(), false,
	)
	require.NoError(t, err)
	require.False(t, result.Stream)

	body := rec.Body.String()
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	require.NotContains(t, body, "event:")

	output := gjson.Get(body, "output").Array()
	require.Len(t, output, 1)
	require.Equal(t, "compaction", output[0].Get("type").String())
	require.NotEmpty(t, output[0].Get("encrypted_content").String())
}

// 上游没产出任何文本时不能发空 encrypted_content（Codex 会拒收整个 payload），
// 必须显式失败。流式走 response.failed——普通 error 帧会被当成断流并盲重连。
func TestHandleResponsesCompactionResponse_EmptySummaryFailsAsResponseFailed(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	emptySSE := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_empty","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":5}}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`,
		``,
	}, "\n")

	svc := &GatewayService{}
	_, err := svc.handleResponsesCompactionResponse(
		newCompactionUpstream(emptySSE),
		c, "gpt-5.6-sol", "claude-sonnet-4.5", nil, time.Now(), true,
	)
	require.Error(t, err)

	out := rec.Body.String()
	require.Contains(t, out, "response.failed")
	require.NotContains(t, out, `"type":"compaction"`)
}

// 上游流未产出 message_start 时同样失败，且不得合成空 compaction item。
func TestHandleResponsesCompactionResponse_NoUpstreamResponseFails(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GatewayService{}
	_, err := svc.handleResponsesCompactionResponse(
		newCompactionUpstream(""),
		c, "gpt-5.6-sol", "claude-sonnet-4.5", nil, time.Now(), false,
	)
	require.Error(t, err)
	require.NotContains(t, rec.Body.String(), `"type":"compaction"`)
}

// thinking 块不得混入摘要正文：摘要只应来自模型的可见输出。
func TestAnthropicResponseTextIgnoresThinking(t *testing.T) {
	t.Parallel()

	resp := &apicompat.AnthropicResponse{
		Content: []apicompat.AnthropicContentBlock{
			{Type: "thinking", Thinking: "内部推理，不应出现"},
			{Type: "text", Text: "第一段"},
			{Type: "text", Text: "第二段"},
		},
	}
	got := anthropicResponseText(resp)
	require.Equal(t, "第一段\n第二段", got)
	require.NotContains(t, got, "内部推理")
	require.Empty(t, anthropicResponseText(nil))
}

// 端到端请求侧：带 compaction_trigger 的请求必须被改写成"摘要轮次"——
// 摘要指令入 messages、tool_choice=none、max_tokens 拉高、thinking 关闭、
// 且启用账号的 compact 专用模型映射。tools 必须保留（历史 tool_use 引用它）。
func TestForwardAsResponses_CompactionRewritesUpstreamRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"gpt-5.6-sol",
		"stream":true,
		"max_output_tokens":8192,
		"tools":[{"type":"function","name":"exec","parameters":{"type":"object"}}],
		"tool_choice":"auto",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"帮我改代码"}]},
			{"type":"compaction_trigger"}
		]
	}`)

	upstream := &compactionTestUpstream{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(anthropicSummarySSE("<summary>摘要</summary>"))),
	}}}
	svc := &GatewayService{
		cfg:                 &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream:        upstream,
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}
	account := &Account{
		ID:          901,
		Name:        "anthropic-compact",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":               "sk-test",
			"model_mapping":         map[string]any{"gpt-5.6-sol": "claude-sonnet-4-6"},
			"compact_model_mapping": map[string]any{"gpt-5.6-sol": "claude-haiku-4-5"},
		},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsResponses(c.Request.Context(), c, account, body, &ParsedRequest{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 1)

	upstreamBody := string(upstream.bodies[0])
	require.NotEmpty(t, upstreamBody)

	// compact 专用模型映射优先于普通映射。
	require.Equal(t, "claude-haiku-4-5", gjson.Get(upstreamBody, "model").String())
	require.Equal(t, "claude-haiku-4-5", result.UpstreamModel)

	// 摘要指令必须真的进了 messages，否则上游会当普通对话继续干活。
	require.Contains(t, upstreamBody, "produce a faithful, concise summary")

	// 禁止工具调用，但工具声明保留。
	require.Equal(t, "none", gjson.Get(upstreamBody, "tool_choice.type").String())
	require.Len(t, gjson.Get(upstreamBody, "tools").Array(), 1)

	// max_tokens 拉到下限之上，thinking 关闭。
	require.GreaterOrEqual(t, int(gjson.Get(upstreamBody, "max_tokens").Int()), compactionMinMaxTokens)
	require.False(t, gjson.Get(upstreamBody, "thinking").Exists())

	// 客户端拿到的是 compaction item，而不是普通 message。
	out := rec.Body.String()
	items := collectSSEItems(t, out)
	require.Len(t, items, 1)
	require.Equal(t, "compaction", gjson.Get(items[0], "type").String())
}

// 普通请求（无 compaction_trigger）不得被 compact 门控影响。
func TestForwardAsResponses_NonCompactionRequestUnaffected(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"gpt-5.6-sol",
		"stream":false,
		"tools":[{"type":"function","name":"exec","parameters":{"type":"object"}}],
		"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"你好"}]}]
	}`)

	upstream := &compactionTestUpstream{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(anthropicSummarySSE("普通回答"))),
	}}}
	svc := &GatewayService{
		cfg:                 &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream:        upstream,
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}
	account := &Account{
		ID:          902,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":               "sk-test",
			"model_mapping":         map[string]any{"gpt-5.6-sol": "claude-sonnet-4-6"},
			"compact_model_mapping": map[string]any{"gpt-5.6-sol": "claude-haiku-4-5"},
		},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")

	_, err := svc.ForwardAsResponses(c.Request.Context(), c, account, body, &ParsedRequest{})
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)

	upstreamBody := string(upstream.bodies[0])
	require.NotEmpty(t, upstreamBody)

	// 普通映射生效，compact 映射不得介入。
	require.Equal(t, "claude-sonnet-4-6", gjson.Get(upstreamBody, "model").String())
	require.NotContains(t, upstreamBody, "produce a faithful, concise summary")
	require.False(t, gjson.Get(upstreamBody, "tool_choice").Exists())
	require.Equal(t, 8192, int(gjson.Get(upstreamBody, "max_tokens").Int()))

	// 输出仍是普通 message，不是 compaction。
	require.NotContains(t, rec.Body.String(), `"type":"compaction"`)
}

func collectSSEItems(t *testing.T, stream string) []string {
	t.Helper()
	var items []string
	for _, line := range strings.Split(stream, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if gjson.Get(payload, "type").String() != "response.output_item.done" {
			continue
		}
		items = append(items, gjson.Get(payload, "item").Raw)
	}
	return items
}

func lastSSEData(t *testing.T, stream string) string {
	t.Helper()
	var last string
	for _, line := range strings.Split(stream, "\n") {
		if strings.HasPrefix(line, "data: ") {
			last = strings.TrimPrefix(line, "data: ")
		}
	}
	require.NotEmpty(t, last)
	return last
}

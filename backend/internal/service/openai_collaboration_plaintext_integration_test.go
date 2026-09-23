package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func collabPlaintextTestContext(body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "curl/8.0")
	return c, recorder
}

// 走真实 Forward 入口（而非只测 helper）证明请求侧接线：
// collaboration namespace 的 send_message 被降级为普通 function 且去掉
// encrypted:true，control 工具 wait 保留在 namespace，input 里的历史
// namespaced 调用被改写为别名。
func assertCollabPlaintextLowered(t *testing.T, upstreamBody []byte) {
	t.Helper()
	flat := gjson.GetBytes(upstreamBody, `tools.#(name=="collaboration__send_message")`)
	require.True(t, flat.Exists(), "send_message must be lowered to a flat function tool")
	require.Equal(t, "function", flat.Get("type").String())
	require.False(t, flat.Get("parameters.properties.message.encrypted").Exists(), "encrypted:true marker must be stripped from the message schema")
	require.Equal(t, "note", flat.Get("parameters.properties.message.description").String())
	require.Equal(t, "collaboration__send_message", gjson.GetBytes(upstreamBody, "input.0.name").String())
	require.False(t, gjson.GetBytes(upstreamBody, "input.0.namespace").Exists())
	require.Equal(t, "go", gjson.GetBytes(upstreamBody, "input.1.content.0.text").String())
}

func assertCollabPlaintextRestored(t *testing.T, body string) {
	t.Helper()
	require.NotContains(t, body, "collaboration__send_message", "lowered alias must never reach the client")
	require.Contains(t, body, `"name":"send_message"`)
	require.Contains(t, body, `"namespace":"collaboration"`)
	require.Contains(t, body, `"encrypted_function_args":[]`)
}

func collabPlaintextEnabledAccount() *Account {
	account := newOpenAIRejectedFieldTestAccount()
	account.Extra["openai_responses_plaintext_collaboration"] = true
	return account
}

func collabPlaintextNonstreamUpstream(aliasName string) *httpUpstreamRecorder {
	return &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"id":"resp_collab","status":"completed","output":[
			{"type":"function_call","id":"i1","call_id":"c1","name":"` + aliasName + `","arguments":"{\"message\":\"done\"}"}],
			"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
}

// HTTP 非流式：Forward 全链路（adapt → upstream → restore）。
func TestOpenAIGatewayService_ForwardCollaborationPlaintextNonStreaming(t *testing.T) {
	body := collabPlaintextRequestBody(false)
	c, recorder := collabPlaintextTestContext(body)
	upstream := collabPlaintextNonstreamUpstream("collaboration__send_message")

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), c, collabPlaintextEnabledAccount(), body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	forwarded := upstream.bodies[0]
	assertCollabPlaintextLowered(t, forwarded)
	// API Key 未开 flatten flag：剩余 control 工具仍在原 namespace。
	ns := gjson.GetBytes(forwarded, `tools.#(type=="namespace")`)
	require.True(t, ns.Exists())
	require.Equal(t, "collaboration", ns.Get("name").String())
	require.Equal(t, "wait", ns.Get("tools.0.name").String())

	out := recorder.Body.String()
	assertCollabPlaintextRestored(t, out)
	require.Equal(t, "collaboration", gjson.Get(out, "output.0.namespace").String())
	require.Equal(t, "send_message", gjson.Get(out, "output.0.name").String())
	require.Len(t, gjson.Get(out, "output.0.encrypted_function_args").Array(), 0)
}

func TestOpenAIGatewayService_ForwardCollaborationPlaintextEscapedNames(t *testing.T) {
	body := bytes.Replace(collabPlaintextRequestBody(false), []byte(`"name":"collaboration"`), []byte(`"name":"collabor\u0061tion"`), 1)
	c, recorder := collabPlaintextTestContext(body)
	upstream := collabPlaintextNonstreamUpstream(`collaboration\u005f\u005fsend_message`)

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), c, collabPlaintextEnabledAccount(), body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	flat := gjson.GetBytes(upstream.bodies[0], `tools.#(name=="collaboration__send_message")`)
	require.True(t, flat.Exists(), "escaped namespace must be lowered at the HTTP entry")
	require.False(t, flat.Get("parameters.properties.message.encrypted").Exists())
	require.Equal(t, "collaboration", gjson.Get(recorder.Body.String(), "output.0.namespace").String())
	require.Equal(t, "send_message", gjson.Get(recorder.Body.String(), "output.0.name").String())
}

// HTTP SSE：逐事件还原别名。
func TestOpenAIGatewayService_ForwardCollaborationPlaintextStreaming(t *testing.T) {
	body := collabPlaintextRequestBody(true)
	c, recorder := collabPlaintextTestContext(body)

	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"type":"function_call","id":"i1","call_id":"c1","name":"collaboration__send_message","status":"in_progress"}}`,
		`data: {"type":"response.function_call_arguments.done","sequence_number":1,"item_id":"i1","call_id":"c1","name":"collaboration__send_message","arguments":"{\"message\":\"ok\"}"}`,
		`data: {"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"function_call","id":"i1","call_id":"c1","name":"collaboration__send_message","arguments":"{\"message\":\"ok\"}","status":"completed"}}`,
		`data: {"type":"response.completed","sequence_number":3,"response":{"id":"resp_collab_sse","status":"completed","output":[{"type":"function_call","id":"i1","call_id":"c1","name":"collaboration__send_message","arguments":"{\"message\":\"ok\"}"}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
	}, "\n\n") + "\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), c, collabPlaintextEnabledAccount(), body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assertCollabPlaintextLowered(t, upstream.bodies[0])
	assertCollabPlaintextRestored(t, recorder.Body.String())
}

// SSE-to-JSON：客户端 stream:false 但上游回 SSE，最终 JSON 同样还原别名。
func TestOpenAIGatewayService_ForwardCollaborationPlaintextSSEToJSON(t *testing.T) {
	body := collabPlaintextRequestBody(false)
	c, recorder := collabPlaintextTestContext(body)

	sse := `data: {"type":"response.completed","response":{"id":"resp_collab_s2j","status":"completed","output":[{"type":"function_call","id":"i1","call_id":"c1","name":"collaboration__send_message","arguments":"{\"message\":\"ok\"}"}],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), c, collabPlaintextEnabledAccount(), body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	out := recorder.Body.String()
	assertCollabPlaintextRestored(t, out)
	require.Equal(t, "send_message", gjson.Get(out, "output.0.name").String())
}

// API-key passthrough：请求经 Forward adapt 后进入透传分支，响应同样还原。
func TestOpenAIPassthroughCollaborationPlaintextNonStreaming(t *testing.T) {
	body := collabPlaintextRequestBody(false)
	c, recorder := collabPlaintextTestContext(body)
	upstream := collabPlaintextNonstreamUpstream("collaboration__send_message")
	account := collabPlaintextEnabledAccount()
	account.Extra["openai_passthrough"] = true

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), c, account, body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	forwarded := upstream.bodies[0]
	// 请求里没有 custom/tool_search 项时透传分支不跑 client-tool 适配：
	// send_message 由本适配器降级（encrypted 去掉），wait 留在原 namespace。
	flat := gjson.GetBytes(forwarded, `tools.#(name=="collaboration__send_message")`)
	require.True(t, flat.Exists())
	require.False(t, flat.Get("parameters.properties.message.encrypted").Exists())
	ns := gjson.GetBytes(forwarded, `tools.#(type=="namespace")`)
	require.Equal(t, "collaboration", ns.Get("name").String())
	require.Equal(t, "wait", ns.Get("tools.0.name").String())
	require.Equal(t, "collaboration__send_message", gjson.GetBytes(forwarded, "input.0.name").String())

	assertCollabPlaintextRestored(t, recorder.Body.String())
}

func TestOpenAIPassthroughCollaborationPlaintextStreaming(t *testing.T) {
	body := collabPlaintextRequestBody(true)
	c, recorder := collabPlaintextTestContext(body)

	sse := strings.Join([]string{
		`data: {"type":"response.output_item.done","sequence_number":0,"output_index":0,"item":{"type":"function_call","id":"i1","call_id":"c1","name":"collaboration__send_message","arguments":"{\"message\":\"ok\"}","status":"completed"}}`,
		`data: {"type":"response.completed","sequence_number":1,"response":{"id":"resp_collab_pt_sse","status":"completed","output":[{"type":"function_call","id":"i1","call_id":"c1","name":"collaboration__send_message","arguments":"{\"message\":\"ok\"}"}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
	}, "\n\n") + "\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}}
	account := collabPlaintextEnabledAccount()
	account.Extra["openai_passthrough"] = true

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), c, account, body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assertCollabPlaintextRestored(t, recorder.Body.String())
}

// default-off：同样的 body 不开 flag 时必须与既有行为一致——namespace
// 声明原样转发、encrypted:true 保留、input 调用项保留 namespace。
func TestOpenAIGatewayService_ForwardCollaborationPlaintextOptOutPreservesBody(t *testing.T) {
	body := collabPlaintextRequestBody(false)
	c, recorder := collabPlaintextTestContext(body)
	upstream := collabPlaintextNonstreamUpstream("collaboration__send_message")
	account := newOpenAIRejectedFieldTestAccount() // no flag

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), c, account, body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	forwarded := upstream.bodies[0]
	ns := gjson.GetBytes(forwarded, `tools.#(type=="namespace")`)
	require.True(t, ns.Exists(), "opt-out must forward the namespace declaration untouched")
	require.Equal(t, "send_message", ns.Get("tools.0.name").String())
	require.Equal(t, true, ns.Get("tools.0.parameters.properties.message.encrypted").Bool())
	require.Equal(t, "wait", ns.Get("tools.1.name").String())
	require.False(t, gjson.GetBytes(forwarded, `tools.#(name=="collaboration__send_message")`).Exists())
	require.Equal(t, "send_message", gjson.GetBytes(forwarded, "input.0.name").String())
	require.Equal(t, "collaboration", gjson.GetBytes(forwarded, "input.0.namespace").String())

	// 无 mapping → 上游别名（如果有）原样透传，不还原。
	out := recorder.Body.String()
	require.Contains(t, out, "collaboration__send_message")
	_, ok := openAIResponsesCollabPlaintextMapping(c)
	require.False(t, ok)
}

// opt-in 但出站模式不支持 → 明确 400，不静默改道。
func TestOpenAIGatewayService_ForwardCollaborationPlaintextRejectsUnsupportedOutbound(t *testing.T) {
	newForceCCAccount := func() *Account {
		account := collabPlaintextEnabledAccount()
		account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeForceChatCompletions)
		return account
	}
	// OpenAI 平台账号不会是 Anthropic 协议（GetAPIProtocol 仅对多协议
	// API-key 生效），可达的 unsupported 组合是 force_chat_completions 与
	// 非 APIKey/OAuthLike 账号类型。
	newUpstreamTypeAccount := func() *Account {
		account := collabPlaintextEnabledAccount()
		account.Type = AccountTypeUpstream
		return account
	}

	for name, account := range map[string]*Account{
		"force_chat_completions": newForceCCAccount(),
		"upstream_account_type":  newUpstreamTypeAccount(),
	} {
		t.Run(name, func(t *testing.T) {
			body := collabPlaintextRequestBody(false)
			c, recorder := collabPlaintextTestContext(body)
			upstream := collabPlaintextNonstreamUpstream("collaboration__send_message")

			result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
				context.Background(), c, account, body,
			)

			require.ErrorIs(t, err, errOpenAIResponsesCollabPlaintextUnsupported)
			require.Nil(t, result)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
			require.Nil(t, upstream.lastReq, "unsupported outbound must be rejected before any upstream call")
		})
	}
}

// compact 路径不是新 collaboration 调用生成路径：opt-in 下本适配器跳过，
// 既有 namespace 摊平照常执行（encrypted:true 原样保留 → 证明没走降级）。
func TestOpenAIGatewayService_CompactSkipsCollaborationPlaintextAdaptation(t *testing.T) {
	body := collabPlaintextRequestBody(false)
	c, _ := collabPlaintextTestContext(body)
	c.Request.URL.Path = "/v1/responses/compact"
	upstream := collabPlaintextNonstreamUpstream("collaboration__send_message")
	// namespace 摊平仅对 OAuth-like 账号生效；用 OAuth 账号才能让 compact
	// 的既有摊平与本适配器的跳过形成可区分的断言。
	account := newOpenAIOAuthNamespaceTestAccount()
	account.Extra = map[string]any{"openai_responses_plaintext_collaboration": true}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), c, account, body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	forwarded := upstream.bodies[0]
	// 既有 compact 摊平仍把 send_message 变成同名平铺形式，但 encrypted
	// 标记必须原样保留——证明走的是 flatten 而不是 plaintext 降级。
	flat := gjson.GetBytes(forwarded, `tools.#(name=="collaboration__send_message")`)
	require.True(t, flat.Exists())
	require.Equal(t, true, flat.Get("parameters.properties.message.encrypted").Bool(),
		"compact path must keep the original encrypted marker; plaintext lowering is skipped")
	_, ok := openAIResponsesCollabPlaintextMapping(c)
	require.False(t, ok, "compact path must not install a plaintext collaboration mapping")
}

// Forward 在同一 gin context 上按账号重试：新 attempt 开始时清掉上一个
// 账号留下的 mapping，opt-out 账号不得沿用旧 mapping 还原响应。
func TestOpenAIGatewayService_ForwardClearsStaleCollabPlaintextMapping(t *testing.T) {
	body := collabPlaintextRequestBody(false)
	c, recorder := collabPlaintextTestContext(body)
	setOpenAIResponsesCollabPlaintextMapping(c, apicompat.ResponsesCollaborationPlaintextMapping{
		Calls: map[string]apicompat.ResponsesNamespaceName{
			"collaboration__send_message": {Namespace: "collaboration", Name: "send_message"},
		},
	})
	upstream := collabPlaintextNonstreamUpstream("collaboration__send_message")

	_, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), c, newOpenAIRejectedFieldTestAccount(), body, // flag off
	)

	require.NoError(t, err)
	_, ok := openAIResponsesCollabPlaintextMapping(c)
	require.False(t, ok)
	// flag-off 账号的上游别名不应被旧 mapping 还原。
	require.Contains(t, recorder.Body.String(), "collaboration__send_message")
}

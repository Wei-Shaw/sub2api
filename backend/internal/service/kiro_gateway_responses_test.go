//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func kiroTestResponsesContext(body string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/backend-api/codex/responses", bytes.NewReader([]byte(body)))
	return recorder, c
}

// kiroTestFakeUpstreamCapturingBody 跟 kiroTestFakeUpstream 一样起一个假
// 上游，额外把每次收到的请求体记下来——用于断言 Kiro 上游真的收到了经
// MapModel 处理过的模型名（转换链路是否正确接上的直接证据）。
func kiroTestFakeUpstreamCapturingBody(t *testing.T, status int, respBody []byte) (*httptest.Server, *int32, *[][]byte) {
	t.Helper()
	var calls int32
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		bodies = append(bodies, buf)
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(status)
		_, _ = w.Write(respBody)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &bodies
}

// TestKiroForwardAsResponsesNonStreamingReturnsResponsesShapedJSON 覆盖非
// 流式路径的整条链路：Responses 请求 → apicompat 转 Anthropic → 走
// forwardUpstream 这同一套核心引擎打假上游 → 折叠出的 AnthropicResponse 再
// 转回 Responses 形态。断言：(1) Kiro 上游收到的是经 MapModel 处理后的模型名
// （证明转换链路真的接上了，不是巧合地什么都没转就侥幸能跑）；(2) 写给
// 客户端的是 Responses 的信封（"object":"response"），不是 Anthropic 的
// 信封（"type":"message"）。
func TestKiroForwardAsResponsesNonStreamingReturnsResponsesShapedJSON(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"Hello from gpt"}`),
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
	)
	srv, calls, bodies := kiroTestFakeUpstreamCapturingBody(t, http.StatusOK, frames)

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(601)
	reqBody := `{"model":"gpt-5.6-sol","input":"hi","max_output_tokens":100,"stream":false}`
	recorder, c := kiroTestResponsesContext(reqBody)

	result, err := svc.ForwardAsResponses(context.Background(), c, account, []byte(reqBody), nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))
	require.Len(t, *bodies, 1)
	require.Contains(t, string((*bodies)[0]), `"gpt-5.6-sol"`, "Kiro 上游必须收到经 MapModel 处理后的模型名")

	out := recorder.Body.String()
	require.Contains(t, out, `"object":"response"`, "必须是 Responses 信封（Anthropic 响应没有这个字段）")
	require.NotContains(t, out, `"stop_reason"`, "不应该混入 Anthropic 顶层信封才有的字段（Responses 用 status 表达终止原因）")
	require.Contains(t, out, `"status":"completed"`)
	require.Contains(t, out, "Hello from gpt")
}

// TestKiroForwardAsResponsesStreamingWritesResponsesSSEEventTypes 覆盖流式
// 路径：写给客户端的 SSE 事件类型必须是 Responses 的（response.created /
// response.output_text.delta / response.completed），不能是 Anthropic 的
// （message_start / content_block_delta）——这两套事件类型名字空间完全不
// 重叠，只要断言其中一个出现、另一个不出现就足以证明走对了转换分支。
func TestKiroForwardAsResponsesStreamingWritesResponsesSSEEventTypes(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"Hi"}`),
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
	)
	srv, calls, _ := kiroTestFakeUpstreamCapturingBody(t, http.StatusOK, frames)

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(602)
	reqBody := `{"model":"gpt-5.6-terra","input":"hi","stream":true}`
	recorder, c := kiroTestResponsesContext(reqBody)

	result, err := svc.ForwardAsResponses(context.Background(), c, account, []byte(reqBody), nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))

	out := recorder.Body.String()
	require.Contains(t, out, "event: response.created")
	require.Contains(t, out, "event: response.completed")
	require.NotContains(t, out, "event: message_start", "不应该混入 Anthropic 的事件类型")
	require.NotContains(t, out, "event: content_block_delta", "不应该混入 Anthropic 的事件类型")
}

// TestKiroEnforceModelProtocolRejectsClaudeOverResponses 覆盖协议归属隔离
// 的一半：Claude 系模型走 Responses 协议（Codex 端点）必须被拒绝——用户
// 明确要求"claude走Anthropic协议，gpt走openai协议"。
func TestKiroEnforceModelProtocolRejectsClaudeOverResponses(t *testing.T) {
	srv, calls, _ := kiroTestFakeUpstreamCapturingBody(t, http.StatusOK, nil)

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(603)
	reqBody := `{"model":"claude-sonnet-4-6","input":"hi","stream":false}`
	recorder, c := kiroTestResponsesContext(reqBody)

	result, err := svc.ForwardAsResponses(context.Background(), c, account, []byte(reqBody), nil)
	require.Error(t, err)
	require.Nil(t, result)
	require.Zero(t, atomic.LoadInt32(calls), "协议不匹配必须在打上游之前就拒绝")
	require.Contains(t, recorder.Body.String(), "Anthropic Messages API")
}

// TestKiroEnforceModelProtocolRejectsGPTOverMessages 覆盖协议归属隔离的
// 另一半：非 Claude 系模型（gpt-5.6-*）走 Anthropic 协议（/v1/messages）
// 必须被拒绝。
func TestKiroEnforceModelProtocolRejectsGPTOverMessages(t *testing.T) {
	srv, calls, _ := kiroTestFakeUpstreamCapturingBody(t, http.StatusOK, nil)

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(604)
	reqBody := `{"model":"gpt-5.6-sol","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":false}`
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader([]byte(reqBody)))

	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(reqBody))
	require.Error(t, err)
	require.Nil(t, result)
	require.Zero(t, atomic.LoadInt32(calls), "协议不匹配必须在打上游之前就拒绝")
	require.Contains(t, recorder.Body.String(), "OpenAI Responses API")
}

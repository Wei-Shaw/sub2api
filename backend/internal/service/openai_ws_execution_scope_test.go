package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAIWSExecutionScopeTestContext(headers map[string]string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	for k, v := range headers {
		c.Request.Header.Set(k, v)
	}
	return c
}

func TestResolveOpenAIWSClientThreadID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name    string
		headers map[string]string
		body    string
		want    string
	}{
		{
			name: "thread-id header wins",
			headers: map[string]string{
				openAIWSThreadIDHeader:     " t-plain ",
				openAIWSTurnMetadataHeader: `{"session_id":"s","thread_id":"t-header"}`,
				openAIWSWindowIDHeader:     "t-window:0",
			},
			body: `{"client_metadata":{"thread_id":"t-body"}}`,
			want: "t-plain",
		},
		{
			name: "turn metadata header second",
			headers: map[string]string{
				openAIWSTurnMetadataHeader: `{"session_id":"s","thread_id":"t-header"}`,
				openAIWSWindowIDHeader:     "t-window:0",
			},
			body: `{"client_metadata":{"thread_id":"t-body"}}`,
			want: "t-header",
		},
		{
			name:    "window id fallback",
			headers: map[string]string{openAIWSWindowIDHeader: " t-window:0 "},
			body:    `{"client_metadata":{"thread_id":"t-body"}}`,
			want:    "t-window",
		},
		{
			name: "body client_metadata.thread_id",
			body: `{"client_metadata":{"thread_id":"t-body"}}`,
			want: "t-body",
		},
		{
			name: "body embedded turn metadata",
			body: `{"client_metadata":{"x-codex-turn-metadata":"{\"thread_id\":\"t-embedded\"}"}}`,
			want: "t-embedded",
		},
		{
			name:    "malformed header ignored",
			headers: map[string]string{openAIWSTurnMetadataHeader: "{not json"},
			body:    `{}`,
			want:    "",
		},
		{
			name: "none",
			body: `{"input":"hi"}`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveOpenAIWSClientThreadID(newOpenAIWSExecutionScopeTestContext(tc.headers), []byte(tc.body))
			require.Equal(t, tc.want, got)
		})
	}
	require.Equal(t, "t", resolveOpenAIWSClientThreadID(nil, []byte(`{"client_metadata":{"thread_id":"t"}}`)))
}

func TestResolveOpenAIWSExecutionLane(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name    string
		headers map[string]string
		body    string
		want    string
	}{
		{
			name:    "turn is the main lane",
			headers: map[string]string{openAIWSTurnMetadataHeader: `{"thread_id":"t","request_kind":"turn"}`},
			want:    "",
		},
		{
			name:    "prewarm shares the turn lane",
			headers: map[string]string{openAIWSTurnMetadataHeader: `{"thread_id":"t","request_kind":"prewarm","thread_source":"user"}`},
			want:    "",
		},
		{
			name:    "compaction shares the turn lane",
			headers: map[string]string{openAIWSTurnMetadataHeader: `{"thread_id":"t","request_kind":"compaction"}`},
			want:    "",
		},
		{
			name:    "missing request_kind is the main lane",
			headers: map[string]string{openAIWSTurnMetadataHeader: `{"thread_id":"t"}`},
			want:    "",
		},
		{
			name:    "memory consolidation gets its own lane",
			headers: map[string]string{openAIWSTurnMetadataHeader: `{"thread_id":"t","request_kind":"memory","thread_source":"memory_consolidation"}`},
			want:    "kind=memory",
		},
		{
			name:    "request_kind is normalized",
			headers: map[string]string{openAIWSTurnMetadataHeader: `{"thread_id":"t","request_kind":" Memory "}`},
			want:    "kind=memory",
		},
		{
			name:    "unknown kind never joins the turn lane",
			headers: map[string]string{openAIWSTurnMetadataHeader: `{"thread_id":"t","request_kind":"future"}`},
			want:    "kind=future",
		},
		{
			name:    "header metadata wins over body metadata",
			headers: map[string]string{openAIWSTurnMetadataHeader: `{"thread_id":"t","request_kind":"turn"}`},
			body:    `{"client_metadata":{"x-codex-turn-metadata":"{\"thread_id\":\"t\",\"request_kind\":\"memory\"}"}}`,
			want:    "",
		},
		{
			name: "body embedded metadata used when header absent",
			body: `{"client_metadata":{"x-codex-turn-metadata":"{\"thread_id\":\"t\",\"request_kind\":\"memory\"}"}}`,
			want: "kind=memory",
		},
		{
			name:    "malformed header falls back to body",
			headers: map[string]string{openAIWSTurnMetadataHeader: "{not json"},
			body:    `{"client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"memory\"}"}}`,
			want:    "kind=memory",
		},
		{
			name:    "subagent without thread metadata gets its own lane",
			headers: map[string]string{openAIWSThreadIDHeader: "t", openAISubagentHeader: "Guardian"},
			want:    "subagent=guardian",
		},
		{
			name: "subagent thread with its own metadata stays in the turn lane",
			headers: map[string]string{
				openAIWSThreadIDHeader:     "child",
				openAISubagentHeader:       "guardian",
				openAIWSTurnMetadataHeader: `{"thread_id":"child","request_kind":"turn","subagent_kind":"guardian"}`,
			},
			want: "",
		},
		{
			name: "subagent declared in body client_metadata",
			body: `{"client_metadata":{"x-openai-subagent":"guardian"}}`,
			want: "subagent=guardian",
		},
		{
			name: "none",
			body: `{"input":"hi"}`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveOpenAIWSExecutionLane(newOpenAIWSExecutionScopeTestContext(tc.headers), []byte(tc.body))
			require.Equal(t, tc.want, got)
		})
	}
	require.Equal(t, "kind=memory", resolveOpenAIWSExecutionLane(nil, []byte(`{"client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"memory\"}"}}`)))
}

func TestResolveOpenAIWSExecutionScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	threadHeaders := map[string]string{
		"session-id":               "root-session",
		openAIWSTurnMetadataHeader: `{"session_id":"root-session","thread_id":"thread-a"}`,
	}
	bodyFirst := []byte(`{"type":"response.create","model":"gpt-5.1","instructions":"sys","input":[{"role":"user","content":"first"}]}`)
	bodyRetry := []byte(`{"type":"response.create","model":"gpt-5.1","instructions":"sys","input":[{"role":"user","content":"first"},{"role":"assistant","content":"x"},{"role":"user","content":"second"}]}`)

	scopeOf := func(headers map[string]string, body []byte, apiKeyID int64) string {
		scope, _ := resolveOpenAIWSExecutionScope(newOpenAIWSExecutionScopeTestContext(headers), body, apiKeyID)
		return scope
	}

	byThread, threadID := resolveOpenAIWSExecutionScope(newOpenAIWSExecutionScopeTestContext(threadHeaders), bodyFirst, 11)
	require.Len(t, byThread, 16)
	require.Equal(t, "thread-a", threadID)
	require.Equal(t, byThread, scopeOf(threadHeaders, bodyRetry, 11), "同线程重连的作用域不得随请求内容变化")

	otherThread := map[string]string{
		"session-id":               "root-session",
		openAIWSTurnMetadataHeader: `{"session_id":"root-session","thread_id":"thread-b"}`,
	}
	require.NotEqual(t, byThread, scopeOf(otherThread, bodyFirst, 11))
	require.NotEqual(t, byThread, scopeOf(threadHeaders, bodyFirst, 12), "API key 必须参与隔离")

	memoryHeaders := map[string]string{
		"session-id":               "root-session",
		openAIWSTurnMetadataHeader: `{"session_id":"root-session","thread_id":"thread-a","request_kind":"memory","thread_source":"memory_consolidation"}`,
	}
	byMemory, memoryThread := resolveOpenAIWSExecutionScope(newOpenAIWSExecutionScopeTestContext(memoryHeaders), bodyFirst, 11)
	require.Len(t, byMemory, 16)
	require.Equal(t, "thread-a", memoryThread)
	require.NotEqual(t, byThread, byMemory, "同线程的记忆整理请求不得与用户 turn 同键")
	require.Equal(t, byMemory, scopeOf(memoryHeaders, bodyRetry, 11), "记忆整理重连仍命中同一键")
	for _, kind := range []string{"turn", "prewarm", "compaction"} {
		sameLane := map[string]string{
			"session-id":               "root-session",
			openAIWSTurnMetadataHeader: `{"session_id":"root-session","thread_id":"thread-a","request_kind":"` + kind + `"}`,
		}
		require.Equal(t, byThread, scopeOf(sameLane, bodyFirst, 11), kind+" 必须与用户 turn 同键")
	}
	require.Equal(t, byThread, scopeOf(map[string]string{openAIWSThreadIDHeader: "thread-a"}, bodyFirst, 11), "只有 thread-id 头时仍是主道")
	byScorer := scopeOf(map[string]string{openAIWSThreadIDHeader: "thread-a", openAISubagentHeader: "guardian"}, bodyFirst, 11)
	require.Len(t, byScorer, 16)
	require.NotEqual(t, byThread, byScorer, "无线程元数据的子智能体请求不得与用户 turn 同键")
	require.NotEqual(t, byMemory, byScorer)

	sessionOnly := map[string]string{"session-id": "root-session"}
	bySession, threadID := resolveOpenAIWSExecutionScope(newOpenAIWSExecutionScopeTestContext(sessionOnly), bodyFirst, 11)
	require.Len(t, bySession, 16)
	require.Equal(t, "", threadID)
	require.NotEqual(t, byThread, bySession, "线程标识优先于会话标识")
	require.Equal(t, bySession, scopeOf(sessionOnly, bodyRetry, 11))
	require.NotEqual(t, bySession, scopeOf(sessionOnly, bodyFirst, 12))
	sessionMemory := []byte(`{"type":"response.create","input":"hi","client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"memory\"}"}}`)
	require.NotEqual(t, bySession, scopeOf(sessionOnly, sessionMemory, 11), "只有会话标识时记忆整理也单独成道")

	byCacheKey := scopeOf(nil, []byte(`{"type":"response.create","prompt_cache_key":"cache-1","input":"hi"}`), 11)
	require.Len(t, byCacheKey, 16)

	require.Equal(t, "", scopeOf(nil, bodyFirst, 11), "只有请求内容时不得产生执行作用域")
	nilScope, nilThread := resolveOpenAIWSExecutionScope(nil, bodyFirst, 11)
	require.Equal(t, "", nilScope)
	require.Equal(t, "", nilThread)
}

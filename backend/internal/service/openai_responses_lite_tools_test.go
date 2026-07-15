//go:build unit

package service

import (
	"bytes"
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

func TestNormalizeOpenAIResponsesLiteTools_MovesNamespacesAndKeepsSupportedTools(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.6-terra",
		"tools": []any{
			map[string]any{"type": "function", "name": "shell"REDACTED,
			map[string]any{"type": "custom", "name": "exec"REDACTED,
			map[string]any{"type": "tool_search"REDACTED,
			map[string]any{"type": "namespace", "name": "collaboration", "tools": []any{
				map[string]any{"type": "function", "name": "spawn_agent"REDACTED,
	REDACTED
	REDACTED,
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "hello"REDACTED,
			map[string]any{"type": "additional_tools", "role": "developer", "tools": []any{
				map[string]any{"type": "namespace", "name": "image_gen"REDACTED,
				map[string]any{"type": "namespace", "name": "collaboration", "tools": []any{
					map[string]any{"type": "function", "name": "spawn_agent"REDACTED,
		REDACTED
	REDACTED
	REDACTED,
		"tool_choice": map[string]any{"type": "namespace", "name": "collaboration"REDACTED,
REDACTED

	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

REDACTED
	require.True(t, changed)
	tools := reqBody["tools"].([]any)
	require.Len(t, tools, 3)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "custom", tools[1].(map[string]any)["type"])
	require.Equal(t, "tool_search", tools[2].(map[string]any)["type"])
	input := reqBody["input"].([]any)
	require.Len(t, input, 2)
	additional := input[1].(map[string]any)["tools"].([]any)
	require.Len(t, additional, 2)
	require.Equal(t, "image_gen", additional[0].(map[string]any)["name"])
	require.Equal(t, "collaboration", additional[1].(map[string]any)["name"], "existing namespace must not be duplicated")
	require.Equal(t, map[string]any{"type": "namespace", "name": "collaboration"REDACTED, reqBody["tool_choice"])
REDACTED

func TestNormalizeOpenAIResponsesLiteTools_RejectsConflictingAdditionalTool(t *testing.T) {
	reqBody := map[string]any{
		"tools": []any{map[string]any{
			"type":  "namespace",
			"name":  "collaboration",
			"tools": []any{map[string]any{"type": "function", "name": "spawn_agent"REDACTEDREDACTED,
REDACTED
		"input": []any{map[string]any{
			"type": "additional_tools",
			"tools": []any{map[string]any{
				"type":  "namespace",
				"name":  "collaboration",
				"tools": []any{map[string]any{"type": "function", "name": "send_message"REDACTEDREDACTED,
	REDACTED
REDACTED
REDACTED

	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

	require.ErrorContains(t, err, `conflicts with migrated tool type "namespace" name "collaboration"`)
	require.False(t, changed)
	require.Len(t, reqBody["tools"], 1, "conflicts must not partially remove top-level tools")
REDACTED

func TestNormalizeOpenAIResponsesLiteTools_DeduplicatesAcrossAdditionalToolItems(t *testing.T) {
	namespace := map[string]any{
		"type":  "namespace",
		"name":  "collaboration",
		"tools": []any{map[string]any{"type": "function", "name": "spawn_agent"REDACTEDREDACTED,
REDACTED
	reqBody := map[string]any{
		"tools": []any{namespaceREDACTED,
		"input": []any{
			map[string]any{
				"type":  "additional_tools",
				"tools": []any{map[string]any{"type": "custom", "name": "exec"REDACTEDREDACTED,
		REDACTED,
			map[string]any{
				"type":  "additional_tools",
				"tools": []any{namespaceREDACTED,
		REDACTED,
	REDACTED,
REDACTED

	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

REDACTED
	require.True(t, changed)
	require.NotContains(t, reqBody, "tools")
	input := reqBody["input"].([]any)
	require.Len(t, input[0].(map[string]any)["tools"], 1)
	require.Len(t, input[1].(map[string]any)["tools"], 1)
REDACTED

func TestNormalizeOpenAIResponsesLiteTools_ConvertsStringInput(t *testing.T) {
	reqBody := map[string]any{
		"input": "hello",
		"tools": []any{map[string]any{
			"type": "namespace",
			"name": "collaboration",
REDACTED
REDACTED

	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

REDACTED
	require.True(t, changed)
	require.NotContains(t, reqBody, "tools")
	input := reqBody["input"].([]any)
	require.Len(t, input, 2)
	require.Equal(t, "message", input[0].(map[string]any)["type"])
	require.Equal(t, "hello", input[0].(map[string]any)["content"])
	require.Equal(t, "additional_tools", input[1].(map[string]any)["type"])
REDACTED

func TestNormalizeOpenAIResponsesLiteTools_KeepsSupportedTopLevelTools(t *testing.T) {
	reqBody := map[string]any{
		"tools": []any{
			map[string]any{"type": "function", "name": "shell"REDACTED,
			map[string]any{"type": "custom", "name": "exec"REDACTED,
			map[string]any{"type": "tool_search"REDACTED,
			"custom shorthand",
	REDACTED,
REDACTED

	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

REDACTED
	require.False(t, changed)
	require.Len(t, reqBody["tools"], 4)
REDACTED

func TestNormalizeOpenAIResponsesLiteTools_RejectsUnsupportedTools(t *testing.T) {
	tests := []struct {
		name string
		tool map[string]any
		want string
REDACTED{
		{name: "hosted web search", tool: map[string]any{"type": "web_search"REDACTED, want: `top-level tool type "web_search"`REDACTED,
		{name: "hosted image generation", tool: map[string]any{"type": "image_generation"REDACTED, want: `top-level tool type "image_generation"`REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := map[string]any{"tools": []any{tt.toolREDACTEDREDACTED
			changed, err := normalizeOpenAIResponsesLiteTools(reqBody)
			require.ErrorContains(t, err, tt.want)
			require.False(t, changed)
			require.Len(t, reqBody["tools"], 1, "validation errors must not partially mutate tools")
	REDACTED)
REDACTED
REDACTED

func TestNormalizeOpenAIResponsesLiteToolsPayload_PreservesResponseCreateShape(t *testing.T) {
	body := []byte(`{
		"type":"response.create",
		"model":"gpt-5.6-terra",
		"client_metadata":{"ws_request_header_x_openai_internal_codex_responses_lite":"true"REDACTED,
		"input":[{"type":"message","role":"user","content":"hello"REDACTED],
		"tools":[{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent"REDACTED]REDACTED],
		"tool_choice":{"type":"namespace","name":"collaboration"REDACTED
REDACTED`)

	updated, changed, err := normalizeOpenAIResponsesLiteToolsPayload(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, "response.create", gjson.GetBytes(updated, "type").String())
	require.False(t, gjson.GetBytes(updated, "tools").Exists())
	require.Equal(t, "collaboration", gjson.GetBytes(updated, `input.#(type=="additional_tools").tools.0.name`).String())
	require.Equal(t, "namespace", gjson.GetBytes(updated, "tool_choice.type").String())
REDACTED

func TestApplyCodexOAuthTransform_PreservesLiteNamespaceToolChoice(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.6-terra",
		"input": []any{map[string]any{
			"type": "additional_tools",
			"tools": []any{map[string]any{
				"type": "namespace",
				"name": "collaboration",
	REDACTED
REDACTED
		"tool_choice": map[string]any{"type": "namespace", "name": "collaboration"REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, true, false)

	require.Equal(t, map[string]any{"type": "namespace", "name": "collaboration"REDACTED, reqBody["tool_choice"])
REDACTED

func TestOpenAIGatewayServiceForward_NormalizesResponsesLiteToolsForOAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, passthrough := range []bool{false, trueREDACTED {
		name := "managed"
		if passthrough {
			name = "passthrough"
	REDACTED
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
			c.Request.Header.Set(responsesLiteHeader, "true")
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
				Body: io.NopCloser(strings.NewReader(
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_lite\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1REDACTEDREDACTEDREDACTED\n\n" +
						"data: [DONE]\n\n",
				)),
		REDACTEDREDACTED
			svc := &OpenAIGatewayService{cfg: &config.Config{REDACTED, httpUpstream: upstreamREDACTED
			account := &Account{
				ID: 501, Name: "responses-lite", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Concurrency: 1, Status: StatusActive, Schedulable: true, RateMultiplier: f64p(1),
		REDACTED"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"REDACTED,
				Extra:       map[string]any{"openai_passthrough": passthroughREDACTED,
		REDACTED
			body := []byte(`{
				"model":"gpt-5.6-terra","stream":true,"instructions":"test",
				"tools":[
					{"type":"function","name":"shell","parameters":{"type":"object"REDACTEDREDACTED,
					{"type":"custom","name":"exec"REDACTED,
					{"type":"tool_search"REDACTED,
					{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"REDACTEDREDACTED]REDACTED
				],
				"input":[{"type":"message","role":"user","content":"hello"REDACTED],
				"tool_choice":{"type":"namespace","name":"collaboration"REDACTED
		REDACTED`)

			result, err := svc.Forward(context.Background(), c, account, body)

		REDACTED
			require.NotNil(t, result)
			require.Equal(t, "true", upstream.lastReq.Header.Get(responsesLiteHeader))
			require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="namespace")`).Exists())
			require.Equal(t, "shell", gjson.GetBytes(upstream.lastBody, `tools.#(type=="function").name`).String())
			require.Equal(t, "exec", gjson.GetBytes(upstream.lastBody, `tools.#(type=="custom").name`).String())
			require.True(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="tool_search")`).Exists())
			require.Equal(t, "collaboration", gjson.GetBytes(upstream.lastBody, `input.#(type=="additional_tools").tools.0.name`).String())
			require.Equal(t, "namespace", gjson.GetBytes(upstream.lastBody, "tool_choice.type").String())
			require.Equal(t, "collaboration", gjson.GetBytes(upstream.lastBody, "tool_choice.name").String())
	REDACTED)
REDACTED
REDACTED

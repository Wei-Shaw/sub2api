package service

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestGPT62CacheableInstructionsForward(t *testing.T) {
	for _, tt := range []struct {
		name         string
		body         string
		mapping      map[string]any
		defaultModel string
	}{
		{
			name:         "sol missing with existing developer",
			body:         `{"model":"gpt-6.2-sol","input":[{"role":"developer","content":"Client rules."},{"role":"user","content":"Hi"}]}`,
			defaultModel: "gpt-6.2-sol",
		},
		{
			name:         "astra blank with string input",
			body:         `{"model":"gpt-6.2-astra","instructions":" \t","input":"Hi"}`,
			defaultModel: "gpt-6.2-astra",
		},
		{
			name:         "null instructions",
			body:         `{"model":"gpt-6.2-sol","instructions":null,"input":[{"role":"user","content":"Hi"}]}`,
			defaultModel: "gpt-6.2-sol",
		},
		{
			name:         "invalid instructions",
			body:         `{"model":"gpt-6.2-sol","instructions":{"invalid":true},"input":[{"role":"user","content":"Hi"}]}`,
			defaultModel: "gpt-6.2-sol",
		},
		{
			name:         "empty input",
			body:         `{"model":"gpt-6.2-sol","instructions":"","input":[]}`,
			defaultModel: "gpt-6.2-sol",
		},
		{
			name:         "legacy prompt array",
			body:         `{"model":"gpt-6.2-sol","prompt":[{"role":"user","content":"Hi"}]}`,
			defaultModel: "gpt-6.2-sol",
		},
		{
			name:         "mapped astra",
			body:         `{"model":"public-model","input":[{"role":"user","content":"Hi"}]}`,
			mapping:      map[string]any{"public-model": "gpt-6.2-astra"},
			defaultModel: "gpt-6.2-astra",
		},
		{
			name:         "mapped older name to sol",
			body:         `{"model":"gpt-6.1-sol","input":[{"role":"user","content":"Hi"}]}`,
			mapping:      map[string]any{"gpt-6.1-sol": "gpt-6.2-sol"},
			defaultModel: "gpt-6.2-sol",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account := instructionsEchoOAuthAccount(false)
			account.Credentials["model_mapping"] = tt.mapping
			upstream := instructionsEchoUpstream(`""`)
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.body), &body))
			body["stream"] = true
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			result, rec, err := instructionsEchoForward(t, account, upstream, string(encoded))
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "", gjson.GetBytes(upstream.lastBody, "instructions").String())
			require.True(t, gjson.GetBytes(upstream.lastBody, "instructions").Exists())
			input := gjson.GetBytes(upstream.lastBody, "input").Array()
			require.NotEmpty(t, input)
			require.Equal(t, "developer", input[0].Get("role").String())
			require.Equal(t, defaultCodexSynthInstructions(tt.defaultModel), input[0].Get("content").String())
			count := 0
			for _, item := range input {
				if item.Get("role").String() == "developer" && item.Get("content").String() == defaultCodexSynthInstructions(tt.defaultModel) {
					count++
				}
			}
			require.Equal(t, 1, count, "both forwarding passes must share a single default")
			require.Equal(t, gjson.Get(tt.body, "input").IsArray() && len(gjson.Get(tt.body, "input").Array()) == 0,
				len(input) == 1)
			if len(input) > 1 {
				require.Equal(t, "Hi", input[len(input)-1].Get("content").String())
			}
			echo := sseResponseInstructions(t, rec.Body.String(), "response.completed")
			if original := gjson.Get(tt.body, "instructions"); original.Exists() {
				require.JSONEq(t, original.Raw, echo.Raw)
			} else {
				require.Equal(t, gjson.Null, echo.Type)
			}
		})
	}
}

func TestGPT62CacheableInstructionsPreservesExplicitAndSystemGuidance(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "default and system", true: "explicit and system"}[explicit], func(t *testing.T) {
			body := map[string]any{
				"model": "gpt-6.2-sol", "stream": false,
				"input": []any{
					map[string]any{"role": "system", "content": "Keep this JSON guidance."},
					map[string]any{"role": "developer", "content": "Client developer rules."},
					map[string]any{"role": "user", "content": "Hi"},
				},
				"text": map[string]any{"format": map[string]any{"type": "json_object"}},
			}
			if explicit {
				body["instructions"] = "Turn-scoped instruction."
			}
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			upstream := instructionsEchoUpstream(`"upstream echo"`)
			result, _, err := instructionsEchoForward(t, instructionsEchoOAuthAccount(false), upstream, string(encoded))
			require.NoError(t, err)
			require.NotNil(t, result)
			offset := 1
			if explicit {
				offset = 0
				require.Equal(t, body["instructions"], gjson.GetBytes(upstream.lastBody, "instructions").String())
			} else {
				require.Empty(t, gjson.GetBytes(upstream.lastBody, "instructions").String())
			}
			input := gjson.GetBytes(upstream.lastBody, "input").Array()
			require.Len(t, input, 3+offset)
			require.Equal(t, "developer", input[offset].Get("role").String())
			require.Equal(t, "Keep this JSON guidance.", input[offset].Get("content").String())
			require.Equal(t, "Client developer rules.", input[offset+1].Get("content").String())
			require.Equal(t, "Hi", input[offset+2].Get("content").String())
		})
	}
}

func TestGPT62CacheableInstructionsTransformReplayAndTools(t *testing.T) {
	body := map[string]any{
		"model": "openai/GPT-6.2-SOL-max",
		"input": []any{
			map[string]any{"type": "message", "role": "system", "content": []any{
				map[string]any{"type": "input_text", "text": "System guidance."},
				map[string]any{"type": "input_image", "image_url": "https://example.com/image.png"},
			}},
			map[string]any{"type": "function_call", "id": "fc_existing", "call_id": "call_existing", "name": "lookup", "arguments": "{}"},
			map[string]any{"type": "function_call_output", "id": "fco_existing", "call_id": "call_existing", "output": "OK"},
		},
	}
	opts := codexOAuthTransformOptions{OmitPromotedSystemMessagesFromInput: true, PreserveToolCallIDs: true}
	result := applyCodexOAuthTransformWithOptions(body, opts)
	require.NoError(t, result.Error)
	require.Empty(t, body["instructions"])
	first, err := json.Marshal(body["input"])
	require.NoError(t, err)
	input, ok := body["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 4)
	systemMessage, ok := input[1].(map[string]any)
	require.True(t, ok)
	functionCall, ok := input[2].(map[string]any)
	require.True(t, ok)
	functionOutput, ok := input[3].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "developer", systemMessage["role"])
	require.Len(t, systemMessage["content"], 2)
	require.Equal(t, "fc_existing", functionCall["id"])
	require.Equal(t, "fco_existing", functionOutput["id"])
	require.Equal(t, "call_existing", functionCall["call_id"])
	require.Equal(t, "call_existing", functionOutput["call_id"])
	result = applyCodexOAuthTransformWithOptions(body, opts)
	require.NoError(t, result.Error)
	second, err := json.Marshal(body["input"])
	require.NoError(t, err)
	require.JSONEq(t, string(first), string(second))
}

func TestGPT62CacheableInstructionsPreservesSkipAndInvalidInput(t *testing.T) {
	t.Run("compatibility bridge does not gain a default", func(t *testing.T) {
		body := map[string]any{
			"model": "gpt-6.2-astra",
			"input": []any{map[string]any{"role": "system", "content": "Client rules."}},
		}
		result := applyCodexOAuthTransformWithOptions(body, codexOAuthTransformOptions{
			SkipDefaultInstructions: true, OmitPromotedSystemMessagesFromInput: true,
		})
		require.NoError(t, result.Error)
		input, ok := body["input"].([]any)
		require.True(t, ok)
		require.Len(t, input, 1)
		message, ok := input[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "developer", message["role"])
		require.Equal(t, "Client rules.", message["content"])
		require.NotContains(t, body, "instructions")
	})
	t.Run("invalid input is not discarded", func(t *testing.T) {
		input := map[string]any{"invalid": "input"}
		body := map[string]any{"input": input}
		require.False(t, applyGPT62DefaultInstructions(body, "gpt-6.2-sol"))
		require.Equal(t, input, body["input"])
	})
}

func TestGPT62CacheableInstructionsCompatibilityEntrypoints(t *testing.T) {
	for _, tt := range []struct {
		name        string
		path        string
		body        string
		wantDefault bool
	}{
		{
			name: "Chat system", path: "/v1/chat/completions",
			body: `{"model":"public-model","stream":false,"messages":[{"role":"system","content":"Client system rules."},{"role":"user","content":"Hi"}]}`,
		},
		{
			name: "Messages system", path: "/v1/messages",
			body: `{"model":"public-model","max_tokens":16,"stream":false,"system":"Client system rules.","messages":[{"role":"user","content":"Hi"}]}`,
		},
		{
			name: "Responses shape through Chat", path: "/v1/chat/completions", wantDefault: true,
			body: `{"model":"public-model","stream":false,"input":[{"role":"system","content":"Client system rules."},{"role":"user","content":"Hi"}]}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			account := instructionsEchoOAuthAccount(false)
			account.Credentials["model_mapping"] = map[string]any{"public-model": "gpt-6.2-sol"}
			upstream := instructionsEchoUpstream(`""`)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			var result *OpenAIForwardResult
			var err error
			if tt.path == "/v1/messages" {
				result, err = svc.ForwardAsAnthropic(context.Background(), c, account, []byte(tt.body), "", "")
			} else {
				result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(tt.body), "", "")
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Empty(t, gjson.GetBytes(upstream.lastBody, "instructions").String())
			input := gjson.GetBytes(upstream.lastBody, "input").Array()
			offset := 0
			if tt.wantDefault {
				offset = 1
				require.Equal(t, defaultCodexSynthInstructions("gpt-6.2-sol"), input[0].Get("content").String())
			}
			require.Equal(t, "developer", input[offset].Get("role").String())
			require.Contains(t, input[offset].Get("content").Raw, "Client system rules.")
			require.Equal(t, 1, strings.Count(string(upstream.lastBody), "Client system rules."))
		})
	}
}

func TestGPT62CacheableInstructionsKeepsImageBridgeSeparate(t *testing.T) {
	body := map[string]any{
		"model": "gpt-6.2-sol",
		"input": []any{map[string]any{"role": "user", "content": "Create an image."}},
		"tools": []any{map[string]any{"type": "image_generation"}},
	}
	require.True(t, applyGPT62DefaultInstructions(body, "gpt-6.2-sol"))
	require.True(t, applyCodexImageGenerationBridgeInstructions(body))
	result := applyCodexOAuthTransformWithOptions(body, codexOAuthTransformOptions{})
	require.NoError(t, result.Error)
	require.Equal(t, codexImageGenerationBridgeText, body["instructions"])
	input, ok := body["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)
	message, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, defaultCodexSynthInstructions("gpt-6.2-sol"), message["content"])
}

func TestGPT62CacheableInstructionsPreservesOtherRoutes(t *testing.T) {
	for _, tt := range []struct {
		name        string
		model       string
		accountType string
		passthrough bool
		compact     bool
		mapping     map[string]any
		wantDefault bool
	}{
		{name: "older sol", model: "gpt-6.1-sol", accountType: AccountTypeOAuth, wantDefault: true},
		{name: "6.2 name mapped to older sol", model: "gpt-6.2-sol", accountType: AccountTypeOAuth, mapping: map[string]any{"gpt-6.2-sol": "gpt-6.1-sol"}, wantDefault: true},
		{name: "passthrough", model: "gpt-6.2-sol", accountType: AccountTypeOAuth, passthrough: true},
		{name: "native API key", model: "gpt-6.2-sol", accountType: AccountTypeAPIKey},
		{name: "compact", model: "gpt-6.2-sol", accountType: AccountTypeOAuth, compact: true, wantDefault: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			account := instructionsEchoOAuthAccount(tt.passthrough)
			account.Type = tt.accountType
			account.Credentials["api_key"] = "test-key"
			account.Credentials["model_mapping"] = tt.mapping
			upstream := instructionsEchoUpstream(`"echo"`)
			path := "/v1/responses"
			if tt.compact {
				path += "/compact"
				upstream.resp.Header.Set("Content-Type", "application/json")
				upstream.resp.Body = io.NopCloser(strings.NewReader(`{"id":"resp_compact","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`))
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(nil))
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			body, err := json.Marshal(map[string]any{
				"model": tt.model, "stream": !tt.compact,
				"input": []any{map[string]any{"role": "user", "content": "Hi"}},
			})
			require.NoError(t, err)
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			_, err = svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.Equal(t, "user", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
			require.Equal(t, tt.wantDefault, gjson.GetBytes(upstream.lastBody, "instructions").String() != "")
		})
	}
}

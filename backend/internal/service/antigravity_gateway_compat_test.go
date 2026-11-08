package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type antigravityCompatTokenCache struct {
	token string
}

type antigravityCompatErrorReader struct {
	data []byte
	off  int
	err  error
}

type antigravityCompatNotifyingWriter struct {
	gin.ResponseWriter
	wrote chan struct{}
}

func (w *antigravityCompatNotifyingWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	select {
	case w.wrote <- struct{}{}:
	default:
	}
	return n, err
}

func (r *antigravityCompatErrorReader) Read(p []byte) (int, error) {
	if r.off < len(r.data) {
		n := copy(p, r.data[r.off:])
		r.off += n
		return n, nil
	}
	return 0, r.err
}

func (r *antigravityCompatErrorReader) Close() error { return nil }

func (c *antigravityCompatTokenCache) GetAccessToken(context.Context, string) (string, error) {
	return c.token, nil
}

func (c *antigravityCompatTokenCache) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *antigravityCompatTokenCache) DeleteAccessToken(context.Context, string) error {
	return nil
}

func (c *antigravityCompatTokenCache) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}

func (c *antigravityCompatTokenCache) ReleaseRefreshLock(context.Context, string) error {
	return nil
}

func newAntigravityCompatService(cfg config.GatewayConfig, upstream HTTPUpstream) *AntigravityGatewayService {
	tokenProvider := NewAntigravityTokenProvider(
		nil,
		&antigravityCompatTokenCache{token: "fresh-oauth-token"},
		nil,
	)
	return NewAntigravityGatewayService(
		nil,
		nil,
		nil,
		tokenProvider,
		nil,
		upstream,
		NewSettingService(&antigravitySettingRepoStub{}, &config.Config{Gateway: cfg}),
		nil,
	)
}

func newAntigravityCompatAccount(accountType string) *Account {
	return &Account{
		ID:          3757,
		Name:        "antigravity-compat",
		Platform:    PlatformAntigravity,
		Type:        accountType,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "stale-account-token",
			"project_id":   "project-3757",
			"model_mapping": map[string]any{
				"gemini-3.1-pro-high":      "gemini-3.1-pro-high",
				"claude-sonnet-4-5":        "claude-sonnet-4-5",
				"claude-sonnet-4-6":        "claude-sonnet-4-6",
				"claude-opus-4-6-thinking": "claude-opus-4-6-thinking",
			},
		},
	}
}

func newAntigravityCompatContext(method, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, bytes.NewReader(body))
	return c, recorder
}

func antigravityCompatSuccessResponse() *http.Response {
	body := `data: {"response":{"responseId":"resp_3757","candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3}}}` + "\n\n"
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"request-3757"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func TestAntigravitySystemIdentityAcrossProtocols(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const task = "\nKeep GPT-5.6, gpt-6 and Claude model ids unchanged."
	for _, identity := range []string{"You are Codex, based on GPT-5.6.", "You are Codex, an agent based on GPT-6.", "You are GPT-6 running in the Codex CLI.", "You are Claude Code, Anthropic's official CLI for Claude."} {
		for _, model := range []string{"gemini-3.1-pro-high", "claude-sonnet-4-6"} {
			for _, protocol := range []string{"responses", "chat", "messages"} {
				for _, arraySystem := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%s/array=%t", identity, model, protocol, arraySystem), func(t *testing.T) {
						text := "x-anthropic-billing-header: cc_version=example;\n" + identity + task
						var system any = text
						if arraySystem {
							system = []map[string]any{{"type": "text", "text": text}}
						}
						request := map[string]any{"model": model, "max_tokens": 128}
						switch protocol {
						case "responses":
							request["input"] = identity
							request["instructions"] = text
							if arraySystem {
								delete(request, "instructions")
								request["input"] = []map[string]any{
									{"type": "message", "role": "developer", "content": []map[string]string{{"type": "input_text", "text": text}}},
									{"type": "message", "role": "user", "content": identity},
								}
							}
						case "chat":
							request["messages"] = []map[string]any{{"role": "system", "content": system}, {"role": "user", "content": identity}}
						case "messages":
							request["system"] = system
							request["messages"] = []map[string]any{{"role": "user", "content": identity}}
						}
						body, err := json.Marshal(request)
						require.NoError(t, err)
						var forwarded []byte
						upstream := &queuedHTTPUpstreamStub{
							responses: []*http.Response{antigravityCompatSuccessResponse()},
							onCall: func(req *http.Request, _ *queuedHTTPUpstreamStub) {
								forwarded, err = io.ReadAll(req.Body)
								require.NoError(t, err)
							},
						}
						svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
						account := newAntigravityCompatAccount(AccountTypeOAuth)
						c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/"+protocol, body)
						switch protocol {
						case "responses":
							_, err = svc.ForwardAsResponses(context.Background(), c, account, body, nil)
						case "chat":
							_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
						case "messages":
							_, err = svc.Forward(context.Background(), c, account, body, false)
						}
						require.NoError(t, err)
						systemText := gjson.GetBytes(forwarded, "request.systemInstruction.parts").Raw
						require.Contains(t, systemText, "You are an AI agent.")
						require.Contains(t, systemText, "Keep GPT-5.6, gpt-6 and Claude model ids unchanged.")
						require.NotContains(t, systemText, identity)
						require.NotContains(t, systemText, "x-anthropic-billing-header:")
						require.Equal(t, identity, gjson.GetBytes(forwarded, "request.contents.0.parts.0.text").String())
						require.Equal(t, model, gjson.GetBytes(forwarded, "model").String())
					})
				}
			}
		}
	}
}

func TestAntigravitySystemIdentityMultipleBlocksForward(test *testing.T) {
	gin.SetMode(gin.TestMode)
	const model = "gemini-3.1-pro-high"
	const stale = "You are Codex, based on GPT-6."
	const otherStale = "You are Claude Code, Anthropic's official CLI for Claude."
	const benign = "Keep GPT-5.6, gpt-6 and Claude model ids unchanged."
	const quoted = `"You are Codex, based on GPT-6."`
	const embedded = "Example identity:\nYou are Codex, based on GPT-6."
	const userText = stale + "\n" + otherStale
	for _, scenario := range []struct {
		name   string
		blocks []string
	}{
		{"benign_then_stale", []string{benign, stale}},
		{"native_then_stale", []string{"You are Antigravity, a helpful assistant.", stale}},
		{"multiple_stale", []string{stale, otherStale}},
	} {
		for _, protocol := range []string{"responses", "chat", "messages"} {
			for _, separateMessages := range []bool{false, true} {
				if protocol == "messages" && separateMessages {
					continue
				}
				test.Run(fmt.Sprintf("%s/%s/separate=%t", scenario.name, protocol, separateMessages), func(test *testing.T) {
					texts := append(append([]string{}, scenario.blocks...), quoted, embedded, benign)
					blocks := make([]map[string]any, 0, len(texts))
					for _, text := range texts {
						blocks = append(blocks, map[string]any{"type": "text", "text": text})
					}
					request := map[string]any{"model": model, "max_tokens": 128}
					tool := map[string]any{"name": "example", "description": userText, "parameters": map[string]any{"type": "object"}}
					switch protocol {
					case "responses", "chat":
						if protocol == "responses" {
							for _, block := range blocks {
								block["type"] = "input_text"
							}
							tool["type"] = "function"
							request["tools"] = []any{tool}
						} else {
							request["tools"] = []any{map[string]any{"type": "function", "function": tool}}
						}
						messages := []map[string]any{{"role": "system", "content": blocks}}
						if separateMessages {
							messages = nil
							for index, text := range texts {
								role := "system"
								if index%2 == 1 {
									role = "developer"
								}
								if protocol == "responses" && index == 0 {
									request["instructions"] = text
									continue
								}
								messages = append(messages, map[string]any{"role": role, "content": text})
							}
						}
						messages = append(messages, map[string]any{"role": "user", "content": userText})
						if protocol == "responses" {
							request["input"] = messages
						} else {
							request["messages"] = messages
						}
					case "messages":
						request["system"] = blocks
						request["messages"] = []map[string]any{{"role": "user", "content": userText}}
						request["tools"] = []any{map[string]any{"name": "example", "description": userText, "input_schema": map[string]any{"type": "object"}}}
					}
					body, err := json.Marshal(request)
					require.NoError(test, err)
					var forwarded []byte
					upstream := &queuedHTTPUpstreamStub{
						responses: []*http.Response{antigravityCompatSuccessResponse()},
						onCall: func(req *http.Request, _ *queuedHTTPUpstreamStub) {
							forwarded, err = io.ReadAll(req.Body)
							require.NoError(test, err)
						},
					}
					svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
					account := newAntigravityCompatAccount(AccountTypeOAuth)
					requestContext, _ := newAntigravityCompatContext(http.MethodPost, "/v1/"+protocol, body)
					switch protocol {
					case "responses":
						_, err = svc.ForwardAsResponses(context.Background(), requestContext, account, body, nil)
					case "chat":
						_, err = svc.ForwardAsChatCompletions(context.Background(), requestContext, account, body, nil)
					case "messages":
						_, err = svc.Forward(context.Background(), requestContext, account, body, false)
					}
					require.NoError(test, err)
					parts := gjson.GetBytes(forwarded, "request.systemInstruction.parts").Array()
					var systemTexts []string
					for _, part := range parts {
						systemTexts = append(systemTexts, part.Get("text").String())
					}
					require.NotEmpty(test, systemTexts)
					wantTexts := append([]string{}, texts...)
					for index, text := range wantTexts {
						if text == stale || text == otherStale {
							wantTexts[index] = "You are an AI agent."
						}
					}
					if scenario.name != "native_then_stale" {
						require.Contains(test, systemTexts[0], "You are Antigravity")
						systemTexts = systemTexts[1:]
						if protocol == "messages" {
							require.GreaterOrEqual(test, len(systemTexts), 2)
							require.Contains(test, systemTexts[0], "Below are your system instructions.")
							require.Equal(test, "\n--- [SYSTEM_PROMPT_END] ---", systemTexts[len(systemTexts)-1])
							systemTexts = systemTexts[1 : len(systemTexts)-1]
						}
					}
					separator := "\n\n"
					if protocol == "messages" {
						separator = "\n"
					}
					require.Equal(test, strings.Join(wantTexts, separator), strings.Join(systemTexts, "\n"))
					require.Equal(test, userText, gjson.GetBytes(forwarded, "request.contents.0.parts.0.text").String())
					require.Equal(test, userText, gjson.GetBytes(forwarded, "request.tools.0.functionDeclarations.0.description").String())
					require.Equal(test, model, gjson.GetBytes(forwarded, "model").String())
				})
			}
		}
	}
}

func TestInjectAntigravityGeminiSystemIdentity(t *testing.T) {
	const identity = "You are Codex, based on GPT-6."
	const mention = "Example: You are GPT-5.6 running in the Codex CLI."
	for _, existingIdentity := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing identity=%t", existingIdentity), func(t *testing.T) {
			parts := []map[string]any{
				{"text": "x-anthropic-billing-header: cc_version=example;"},
				{"text": identity, "custom": "preserved"},
				{"text": mention},
				{"text": `"You are Codex, based on GPT-6."`},
			}
			if existingIdentity {
				parts = append([]map[string]any{{"text": "You are Antigravity."}}, parts...)
			}
			body, err := json.Marshal(map[string]any{
				"systemInstruction": map[string]any{"parts": parts},
				"contents":          []map[string]any{{"role": "user", "parts": []map[string]string{{"text": identity}}}},
				"tools":             []map[string]any{{"functionDeclarations": []map[string]string{{"name": "example", "description": identity}}}},
			})
			require.NoError(t, err)
			got, err := injectIdentityPatchToGeminiRequest(body)
			require.NoError(t, err)
			require.Equal(t, "You are an AI agent.", gjson.GetBytes(got, "systemInstruction.parts.2.text").String())
			require.Equal(t, "preserved", gjson.GetBytes(got, "systemInstruction.parts.2.custom").String())
			require.Equal(t, mention, gjson.GetBytes(got, "systemInstruction.parts.3.text").String())
			require.Equal(t, `"You are Codex, based on GPT-6."`, gjson.GetBytes(got, "systemInstruction.parts.4.text").String())
			for _, field := range []string{"contents", "tools"} {
				require.JSONEq(t, gjson.GetBytes(body, field).Raw, gjson.GetBytes(got, field).Raw)
			}
			again, err := injectIdentityPatchToGeminiRequest(got)
			require.NoError(t, err)
			require.Equal(t, string(got), string(again))
		})
	}
}

func TestAntigravityCompatOAuthUsesNativeTokenAndRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
		body []byte
		call func(*AntigravityGatewayService, context.Context, *gin.Context, *Account, []byte) (*ForwardResult, error)
	}{
		{
			name: "chat completions",
			path: "/v1/chat/completions",
			body: []byte(`{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"Reply exactly: ok"}]}`),
			call: func(svc *AntigravityGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsChatCompletions(ctx, c, account, body, nil)
			},
		},
		{
			name: "responses",
			path: "/v1/responses",
			body: []byte(`{"model":"gemini-3.1-pro-high","input":"Reply exactly: ok"}`),
			call: func(svc *AntigravityGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsResponses(ctx, c, account, body, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var authorization string
			var upstreamPath string
			var upstreamAlt string
			upstream := &queuedHTTPUpstreamStub{
				responses: []*http.Response{antigravityCompatSuccessResponse()},
				onCall: func(req *http.Request, _ *queuedHTTPUpstreamStub) {
					authorization = req.Header.Get("Authorization")
					upstreamPath = req.URL.Path
					upstreamAlt = req.URL.Query().Get("alt")
				},
			}
			svc := newAntigravityCompatService(
				config.GatewayConfig{MaxLineSize: defaultMaxLineSize},
				upstream,
			)
			c, recorder := newAntigravityCompatContext(http.MethodPost, tt.path, tt.body)

			result, err := tt.call(svc, context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), tt.body)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "Bearer fresh-oauth-token", authorization)
			require.Equal(t, "/v1internal:streamGenerateContent", upstreamPath)
			require.Equal(t, "sse", upstreamAlt)
			require.Equal(t, "request-3757", result.RequestID)
			require.Equal(t, 8, result.Usage.InputTokens)
			require.Equal(t, 3, result.Usage.OutputTokens)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), "ok")
			if tt.name == "chat completions" {
				require.Equal(t, "stop", gjson.Get(recorder.Body.String(), "choices.0.finish_reason").String())
				require.Equal(t, int64(8), gjson.Get(recorder.Body.String(), "usage.prompt_tokens").Int())
				require.Equal(t, int64(3), gjson.Get(recorder.Body.String(), "usage.completion_tokens").Int())
			}
		})
	}
}

func TestAntigravityCompatRejectsUnsupportedAccountType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		path        string
		accountType string
		call        func(*AntigravityGatewayService, context.Context, *gin.Context, *Account, []byte) (*ForwardResult, error)
	}{
		{
			name:        "chat completions upstream",
			path:        "/v1/chat/completions",
			accountType: AccountTypeUpstream,
			call: func(svc *AntigravityGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsChatCompletions(ctx, c, account, body, nil)
			},
		},
		{
			name:        "responses setup token",
			path:        "/v1/responses",
			accountType: AccountTypeSetupToken,
			call: func(svc *AntigravityGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsResponses(ctx, c, account, body, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gemini-3.1-pro-high"}`)
			c, recorder := newAntigravityCompatContext(http.MethodPost, tt.path, body)

			result, err := tt.call(&AntigravityGatewayService{}, context.Background(), c, newAntigravityCompatAccount(tt.accountType), body)

			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), "native OAuth account required for antigravity compatibility mode")
		})
	}
}

func TestBuildAntigravityCompatGeminiBody_ConfiguresMixedToolInvocations(t *testing.T) {
	svc := &AntigravityGatewayService{}
	tests := []struct {
		name           string
		tools          string
		wantFuncs      bool
		wantGoogle     bool
		wantServerFlag bool
	}{
		{
			name:       "mixed server and client tools prefer client tools",
			tools:      `[{"name":"get_weather","input_schema":{"type":"object"}},{"type":"web_search_20250305","name":"web_search"}]`,
			wantFuncs:  true,
			wantGoogle: false,
		},
		{
			name:      "client tools only",
			tools:     `[{"name":"get_weather","input_schema":{"type":"object"}}]`,
			wantFuncs: true,
		},
		{
			name:       "server tools only",
			tools:      `[{"type":"web_search_20250305","name":"web_search"}]`,
			wantGoogle: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claudeBody := []byte(`{"messages":[{"role":"user","content":"hello"}],"tools":` + tt.tools + `}`)
			claudeBody = bytes.ReplaceAll(claudeBody, []byte{92}, nil)
			body, err := svc.buildAntigravityCompatGeminiBody(context.Background(), claudeBody, nil, "project-1", "gemini-2.5-flash")
			require.NoError(t, err)

			var wrapped map[string]any
			require.NoError(t, json.Unmarshal(body, &wrapped))
			request, ok := wrapped["request"].(map[string]any)
			require.True(t, ok)

			tools, _ := request["tools"].([]any)
			hasFuncs, hasGoogle := false, false
			for _, raw := range tools {
				tool, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				if decls, ok := tool["functionDeclarations"].([]any); ok && len(decls) > 0 {
					hasFuncs = true
				}
				if _, ok := tool["googleSearch"]; ok {
					hasGoogle = true
				}
			}
			require.Equal(t, tt.wantFuncs, hasFuncs)
			require.Equal(t, tt.wantGoogle, hasGoogle)

			toolConfig, exists := request["toolConfig"].(map[string]any)
			if !tt.wantServerFlag {
				if exists {
					require.NotContains(t, toolConfig, "includeServerSideToolInvocations")
				}
				return
			}
			require.True(t, exists)
			require.Equal(t, true, toolConfig["includeServerSideToolInvocations"])
		})
	}
}

func TestAntigravityCompatChatMixedBuiltInToolsPreferClientFunctions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	body := []byte(`{
		"model":"claude-opus-4-6-thinking",
		"messages":[{"role":"user","content":"hello"}],
		"stream":true,
		"tools":[
			{"type":"function","function":{"name":"read_file","parameters":{"type":"object","properties":{"path":{"type":"string"}}}}},
			{"type":"function","function":{"name":"terminal","parameters":{"type":"object","properties":{"command":{"type":"string"}}}}},
			{"type":"web_search"},
			{"type":"code_execution"}
		]
	}`)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 1)
	requestBody := upstream.requestBodies[0]
	require.False(t, gjson.GetBytes(requestBody, "request.toolConfig.includeServerSideToolInvocations").Exists())
	require.Len(t, gjson.GetBytes(requestBody, "request.tools.0.functionDeclarations").Array(), 2)
	// No built-in tool entries should remain once client functions are present.
	for _, tool := range gjson.GetBytes(requestBody, "request.tools").Array() {
		require.False(t, tool.Get("googleSearch").Exists())
		require.False(t, tool.Get("codeExecution").Exists())
	}
}

func TestAntigravityCompatResponsesCodexWebSearchMixedWithFunctionsDropsBuiltins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"input":"Reply with exactly: pong",
		"stream":true,
		"tools":[
			{"type":"function","name":"shell","description":"Run a shell command","parameters":{"type":"object","properties":{"command":{"type":"array","items":{"type":"string"}}},"required":["command"],"additionalProperties":false},"strict":true},
			{"type":"web_search"}
		]
	}`)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)

	result, err := svc.ForwardAsResponses(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 1)
	requestBody := upstream.requestBodies[0]
	require.Equal(t, "agent", gjson.GetBytes(requestBody, "requestType").String())
	require.NotEqual(t, "gemini-2.5-flash", gjson.GetBytes(requestBody, "model").String())
	require.True(t, gjson.GetBytes(requestBody, "request.tools.0.functionDeclarations.#(name==\"shell\")").Exists())
	for _, tool := range gjson.GetBytes(requestBody, "request.tools").Array() {
		require.False(t, tool.Get("googleSearch").Exists())
	}
	require.False(t, gjson.GetBytes(requestBody, "request.toolConfig.includeServerSideToolInvocations").Exists())
}

func TestAntigravityCompatPreservesChatTokenLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		body string
		want int64
	}{
		{
			name: "legacy max_tokens below bridge floor",
			body: `{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"ok"}],"max_tokens":8}`,
			want: 8,
		},
		{
			name: "max_completion_tokens takes precedence",
			body: `{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"ok"}],"max_tokens":8,"max_completion_tokens":13}`,
			want: 13,
		},
		{
			name: "max_tokens at safe ceiling is preserved",
			body: `{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"ok"}],"max_tokens":64000}`,
			want: 64000,
		},
		{
			name: "max_tokens above safe ceiling is clamped",
			body: `{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"ok"}],"max_tokens":64001}`,
			want: 64000,
		},
		{
			name: "precedence applies before clamping",
			body: `{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"ok"}],"max_tokens":8,"max_completion_tokens":64001}`,
			want: 64000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
			body := []byte(tt.body)
			c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

			result, err := svc.ForwardAsChatCompletions(
				context.Background(),
				c,
				newAntigravityCompatAccount(AccountTypeOAuth),
				body,
				nil,
			)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requestBodies, 1)
			require.Equal(t, tt.want, gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.maxOutputTokens").Int())
		})
	}
}

func TestPreserveChatCompletionTokenLimitIgnoresAbsentAndNonPositiveValues(t *testing.T) {
	tests := []struct {
		name    string
		request apicompat.ChatCompletionsRequest
	}{
		{name: "absent"},
		{name: "zero max_tokens", request: apicompat.ChatCompletionsRequest{MaxTokens: antigravityCompatIntPtr(0)}},
		{name: "negative max_completion_tokens takes precedence", request: apicompat.ChatCompletionsRequest{MaxTokens: antigravityCompatIntPtr(12), MaxCompletionTokens: antigravityCompatIntPtr(-1)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claudeRequest := &apicompat.AnthropicRequest{MaxTokens: 99}
			preserveChatCompletionTokenLimit(&tt.request, claudeRequest)
			require.Equal(t, 99, claudeRequest.MaxTokens)
		})
	}
}

func antigravityCompatIntPtr(v int) *int { return &v }

func TestAntigravityCompatRoutesByMappedModelFamily(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		model         string
		wantSessionID bool
	}{
		{model: "gemini-3.1-pro-high", wantSessionID: false},
		{model: "claude-sonnet-4-5", wantSessionID: true},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
			body := []byte(`{"model":"` + tt.model + `","messages":[{"role":"user","content":"ok"}],"max_tokens":8}`)
			c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

			result, err := svc.ForwardAsChatCompletions(
				context.Background(),
				c,
				newAntigravityCompatAccount(AccountTypeOAuth),
				body,
				nil,
			)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requestBodies, 1)
			require.Equal(t, tt.model, gjson.GetBytes(upstream.requestBodies[0], "model").String())
			require.Equal(t, tt.wantSessionID, gjson.GetBytes(upstream.requestBodies[0], "request.sessionId").Exists())
		})
	}
}

func TestAntigravityCompatUnauthorizedIsCredentialFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusUnauthorized,
		Header:     http.Header{"X-Request-Id": []string{"auth-3757"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"Invalid bearer token"}}`)),
	}}}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	body := []byte(`{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"ok"}]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	result, err := svc.ForwardAsChatCompletions(
		context.Background(),
		c,
		newAntigravityCompatAccount(AccountTypeOAuth),
		body,
		nil,
	)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GatewayFailureStageAccountAuth, failoverErr.Stage)
	require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
	require.Equal(t, AntigravityCredentialRejectedReason, failoverErr.Reason)
	require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
	require.Equal(t, http.StatusBadGateway, failoverErr.ClientStatusCode)
	require.Equal(t, AntigravityCredentialRejectedClientMessage, failoverErr.ClientMessage)
	require.Equal(t, "auth-3757", failoverErr.ResponseHeaders.Get("X-Request-Id"))
	require.Empty(t, recorder.Body.String())
}

func TestAntigravityCompatEmptyStreamTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		run  func(*AntigravityGatewayService, *gin.Context, *http.Response) (*antigravityStreamResult, error)
	}{
		{
			name: "chat completions",
			run: func(svc *AntigravityGatewayService, c *gin.Context, resp *http.Response) (*antigravityStreamResult, error) {
				return svc.handleChatCompletionsStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high", true)
			},
		},
		{
			name: "responses",
			run: func(svc *AntigravityGatewayService, c *gin.Context, resp *http.Response) (*antigravityStreamResult, error) {
				return svc.handleResponsesStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader("data: malformed\n\ndata: [DONE]\n\n")),
			}

			result, err := tt.run(svc, c, resp)

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.True(t, failoverErr.RetryableOnSameAccount)
			require.Empty(t, recorder.Body.String())
			require.Empty(t, recorder.Header().Get("Content-Type"))
		})
	}
}

func TestAntigravityCompatUsageOnlyStreamTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		run  func(*AntigravityGatewayService, *gin.Context, *http.Response) (*antigravityStreamResult, error)
	}{
		{
			name: "chat completions",
			run: func(svc *AntigravityGatewayService, c *gin.Context, resp *http.Response) (*antigravityStreamResult, error) {
				return svc.handleChatCompletionsStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high", true)
			},
		},
		{
			name: "responses",
			run: func(svc *AntigravityGatewayService, c *gin.Context, resp *http.Response) (*antigravityStreamResult, error) {
				return svc.handleResponsesStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(
					`data: {"response":{"responseId":"resp_3757","usageMetadata":{"promptTokenCount":8}}}` + "\n\n",
				)),
			}

			result, err := tt.run(svc, c, resp)

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.True(t, failoverErr.RetryableOnSameAccount)
			require.Empty(t, recorder.Body.String())
			require.Empty(t, recorder.Header().Get("Content-Type"))
		})
	}
}

func TestAntigravityCompatUsageOnlyNonStreamingTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
		body []byte
		call func(*AntigravityGatewayService, context.Context, *gin.Context, *Account, []byte) (*ForwardResult, error)
	}{
		{
			name: "chat completions",
			path: "/v1/chat/completions",
			body: []byte(`{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"ok"}]}`),
			call: func(svc *AntigravityGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsChatCompletions(ctx, c, account, body, nil)
			},
		},
		{
			name: "responses",
			path: "/v1/responses",
			body: []byte(`{"model":"gemini-3.1-pro-high","input":"ok"}`),
			call: func(svc *AntigravityGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsResponses(ctx, c, account, body, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(
					`data: {"response":{"responseId":"resp_3757","usageMetadata":{"promptTokenCount":8}}}` + "\n\n",
				)),
			}}}
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
			c, recorder := newAntigravityCompatContext(http.MethodPost, tt.path, tt.body)

			result, err := tt.call(
				svc,
				context.Background(),
				c,
				newAntigravityCompatAccount(AccountTypeOAuth),
				tt.body,
			)

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.True(t, failoverErr.RetryableOnSameAccount)
			require.Empty(t, recorder.Body.String())
			require.Empty(t, recorder.Header().Get("Content-Type"))
		})
	}
}

func TestAntigravityCompatChatStreamMapsToolCallAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", nil)
	body := `data: {"response":{"responseId":"resp_3757","candidates":[{"content":{"parts":[{"functionCall":{"id":"call_3757","name":"get_weather","args":{"city":"Tokyo"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3}}}` + "\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	result, err := svc.handleChatCompletionsStreamingFromAntigravity(
		c,
		resp,
		time.Now(),
		"gemini-3.1-pro-high",
		true,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 8, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	require.Contains(t, recorder.Body.String(), `"tool_calls"`)
	require.Contains(t, recorder.Body.String(), `"get_weather"`)
	require.Contains(t, recorder.Body.String(), `"finish_reason":"tool_calls"`)
	require.Contains(t, recorder.Body.String(), `"prompt_tokens":8`)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "data: [DONE]"))
}

func TestAntigravityCompatFirstEventTimeoutTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(
		config.GatewayConfig{MaxLineSize: defaultMaxLineSize, StreamDataIntervalTimeout: 1},
		nil,
	)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", nil)
	reader, writer := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: reader}
	type outcome struct {
		result *antigravityStreamResult
		err    error
	}
	done := make(chan outcome, 1)

	go func() {
		result, err := svc.handleChatCompletionsStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high", false)
		done <- outcome{result: result, err: err}
	}()

	select {
	case got := <-done:
		require.Nil(t, got.result)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, got.err, &failoverErr)
		require.True(t, failoverErr.RetryableOnSameAccount)
		require.Empty(t, recorder.Header().Get("Content-Type"))
	case <-time.After(2 * time.Second):
		_ = writer.Close()
		_ = reader.Close()
		t.Fatal("compat stream ignored StreamDataIntervalTimeout")
	}
	_ = writer.Close()
	_ = reader.Close()
}

func TestAntigravityCompatClientDisconnectDrainsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", nil)
	c.Writer = &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
	body := strings.Join([]string{
		`data: {"response":{"responseId":"resp_3757","candidates":[{"content":{"parts":[{"text":"partial"}]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":1}}}`,
		"",
		`data: {"response":{"responseId":"resp_3757","candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":15}}}`,
		"",
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	result, err := svc.handleChatCompletionsStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high", false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.Equal(t, 15, result.usage.OutputTokens)
}

func TestAntigravityCompatStreamErrorCommitsSingleTerminalFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	body := []byte(`data: {"response":{"responseId":"resp_3757","candidates":[{"content":{"parts":[{"text":"partial"}]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":1}}}` + "\n\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &antigravityCompatErrorReader{
			data: body,
			err:  io.ErrUnexpectedEOF,
		},
	}

	result, err := svc.handleResponsesStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high")

	require.Error(t, err)
	require.Nil(t, result)
	require.True(t, IsResponseCommitted(c))
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: error"))
}

func TestAntigravityCompatKeepaliveAfterFirstEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(
		config.GatewayConfig{MaxLineSize: defaultMaxLineSize, StreamKeepaliveInterval: 1},
		nil,
	)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	reader, writer := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: reader}
	done := make(chan error, 1)

	go func() {
		_, err := svc.handleResponsesStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high")
		done <- err
	}()
	_, err := io.WriteString(
		writer,
		`data: {"response":{"responseId":"resp_3757","candidates":[{"content":{"parts":[{"text":"partial"}]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":1}}}`+"\n\n",
	)
	require.NoError(t, err)
	time.Sleep(1200 * time.Millisecond)
	require.NoError(t, writer.Close())
	require.NoError(t, <-done)
	require.Contains(t, recorder.Body.String(), ": ping\n\n")
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
	require.NoError(t, reader.Close())
}

func TestAntigravityCompatPreContentKeepalive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name    string
		adapter antigravityCompatStreamAdapter
		want    string
	}{
		{"chat completions", newAntigravityChatStreamAdapter("gemini-3.1-pro", false), "data: "},
		{"responses", newAntigravityResponsesStreamAdapter("gemini-3.1-pro"), "event: "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/", nil)
			writer := newAntigravityClientWriter(c.Writer, c.Writer, "test")
			writer.beforeFirstWrite = func() { c.Header("Content-Type", "text/event-stream") }
			start := time.Now().Add(-20 * time.Second)
			session := newAntigravityCompatStreamSession("gemini-3.1-pro", start, tt.adapter, writer)
			session.writePreContentKeepalive(start.Add(14 * time.Second))
			require.Empty(t, recorder.Body.String())
			session.consumeClaudeData("message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant"}}`)
			require.False(t, session.hasMeaningfulData())
			session.writePreContentKeepalive(start.Add(14 * time.Second))
			require.Empty(t, recorder.Body.String())
			session.writePreContentKeepalive(start.Add(15 * time.Second))
			require.Equal(t, ": ping\n\n", recorder.Body.String())
			require.Nil(t, session.firstTokenMs)
			session.consumeClaudeData("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`)
			require.True(t, session.hasMeaningfulData())
			require.NotNil(t, session.firstTokenMs)
			require.GreaterOrEqual(t, *session.firstTokenMs, 20000)
			require.Contains(t, recorder.Body.String(), tt.want)
			require.Greater(t, strings.Index(recorder.Body.String(), tt.want), strings.Index(recorder.Body.String(), ": ping"))
			before := recorder.Body.String()
			session.writePreContentKeepalive(start.Add(30 * time.Second))
			require.Equal(t, before, recorder.Body.String())
		})
	}
}

func TestAntigravityCompatHandlerPreContentKeepalive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name    string
		adapter func() antigravityCompatStreamAdapter
		line    string
		want    string
	}{
		{"silent chat", func() antigravityCompatStreamAdapter { return newAntigravityChatStreamAdapter("gemini-3.1-pro", false) }, "", `"upstream_error"`},
		{"signature-only responses", func() antigravityCompatStreamAdapter { return newAntigravityResponsesStreamAdapter("gemini-3.1-pro") }, `data: {"response":{"candidates":[{"content":{"parts":[{"thoughtSignature":"sig","text":""}]}}]}}` + "\n\n", "event: error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize, StreamDataIntervalTimeout: 30}, nil)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/", nil)
			notifier := &antigravityCompatNotifyingWriter{ResponseWriter: c.Writer, wrote: make(chan struct{}, 1)}
			c.Writer = notifier
			reader, pipeWriter := io.Pipe()
			defer func() { _ = reader.Close() }()
			resp := &http.Response{StatusCode: http.StatusOK, Body: reader}
			done := make(chan error, 1)
			go func() {
				_, err := svc.handleAntigravityCompatStream(c, resp, time.Now().Add(-antigravityCompatPreContentKeepaliveInterval+200*time.Millisecond), "gemini-3.1-pro", tt.adapter(), "test")
				done <- err
			}()
			if tt.line != "" {
				_, err := io.WriteString(pipeWriter, tt.line)
				require.NoError(t, err)
			}
			select {
			case <-notifier.wrote:
			case <-time.After(2 * time.Second):
				_ = pipeWriter.Close()
				t.Fatal("no pre-content keepalive before read timeout")
			}
			require.Equal(t, ": ping\n\n", recorder.Body.String())
			require.NoError(t, pipeWriter.Close())
			require.Error(t, <-done)
			require.Contains(t, recorder.Body.String(), tt.want)
			require.True(t, IsResponseCommitted(c))
		})
	}
}

func TestAntigravityCompatHandlerRepeatsPreContentKeepalive(t *testing.T) {
	svc := newAntigravityCompatService(config.GatewayConfig{
		MaxLineSize: defaultMaxLineSize, StreamDataIntervalTimeout: 30, StreamKeepaliveInterval: 0,
	}, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/", nil)
	reader, pipeWriter := io.Pipe()
	defer func() { _ = reader.Close() }()
	resp := &http.Response{StatusCode: http.StatusOK, Body: reader}
	done := make(chan error, 1)
	go func() {
		_, err := svc.handleAntigravityCompatStreamWithKeepaliveInterval(
			c, resp, time.Now().Add(-15*time.Millisecond), "gemini-3.1-pro",
			newAntigravityResponsesStreamAdapter("gemini-3.1-pro"), "test", 15*time.Millisecond, time.Second,
		)
		done <- err
	}()
	time.Sleep(55 * time.Millisecond)
	require.NoError(t, pipeWriter.Close())
	require.Error(t, <-done)
	require.GreaterOrEqual(t, strings.Count(recorder.Body.String(), ": ping\n\n"), 3)
	require.Contains(t, recorder.Body.String(), "event: error")
	require.True(t, IsResponseCommitted(c))
}

func TestAntigravityCompatHandlerPreContentDeadlineWithCommentOnlyStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize, StreamDataIntervalTimeout: 1}, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	reader, pipeWriter := io.Pipe()
	defer func() { _ = reader.Close() }()
	resp := &http.Response{StatusCode: http.StatusOK, Body: reader}
	done := make(chan error, 1)
	go func() {
		_, err := svc.handleAntigravityCompatStreamWithKeepaliveInterval(c, resp, time.Now(), "gemini-3.1-pro",
			newAntigravityResponsesStreamAdapter("gemini-3.1-pro"), "test", 10*time.Millisecond, 100*time.Millisecond)
		done <- err
	}()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if _, err := io.WriteString(pipeWriter, ": upstream ping\n\n"); err != nil {
					return
				}
			}
		}
	}()
	select {
	case err := <-done:
		require.ErrorContains(t, err, "pre-content stream timeout")
	case <-time.After(time.Second):
		t.Fatal("comment-only stream exceeded pre-content deadline")
	}
	require.Contains(t, recorder.Body.String(), ": ping\n\n")
	require.Contains(t, recorder.Body.String(), "event: error")
	require.True(t, IsResponseCommitted(c))
	_ = pipeWriter.Close()
}

func TestAntigravityCompatExpiredPreContentDeadlineDoesNotCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	reader, pipeWriter := io.Pipe()
	defer func() { _ = reader.Close() }()
	defer func() { _ = pipeWriter.Close() }()
	resp := &http.Response{StatusCode: http.StatusOK, Body: reader}
	result, err := svc.handleAntigravityCompatStreamWithKeepaliveInterval(c, resp,
		time.Now().Add(-time.Minute), "gemini-3.1-pro",
		newAntigravityResponsesStreamAdapter("gemini-3.1-pro"), "test", 10*time.Millisecond, 20*time.Millisecond)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, recorder.Body.String())
	require.False(t, IsResponseCommitted(c))
}

func TestAntigravityCompatHandlerErrorsAfterPreContentPing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name string
		wait time.Duration
		want string
	}{
		{"read error", time.Second, "stream_read_error"},
		{"timeout", 25 * time.Millisecond, "stream_timeout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
			reader, pipeWriter := io.Pipe()
			defer func() { _ = reader.Close() }()
			defer func() { _ = pipeWriter.Close() }()
			if tc.name == "read error" {
				go func() {
					time.Sleep(15 * time.Millisecond)
					_ = pipeWriter.CloseWithError(io.ErrUnexpectedEOF)
				}()
			}
			resp := &http.Response{StatusCode: http.StatusOK, Body: reader}
			_, err := svc.handleAntigravityCompatStreamWithKeepaliveInterval(c, resp,
				time.Now().Add(-20*time.Millisecond), "gemini-3.1-pro",
				newAntigravityResponsesStreamAdapter("gemini-3.1-pro"), "test", 10*time.Millisecond, tc.wait)
			require.Error(t, err)
			require.Contains(t, recorder.Body.String(), ": ping\n\n")
			require.Contains(t, recorder.Body.String(), tc.want)
			require.True(t, IsResponseCommitted(c))
		})
	}
}

func TestAntigravityCompatEmptyAfterKeepaliveReportsStreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name    string
		adapter antigravityCompatStreamAdapter
		want    string
	}{
		{"chat completions", newAntigravityChatStreamAdapter("gemini-3.1-pro", false), `"upstream_error"`},
		{"responses", newAntigravityResponsesStreamAdapter("gemini-3.1-pro"), "event: error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/", nil)
			writer := newAntigravityClientWriter(c.Writer, c.Writer, "test")
			writer.beforeFirstWrite = func() { c.Header("Content-Type", "text/event-stream") }
			start := time.Now().Add(-20 * time.Second)
			session := newAntigravityCompatStreamSession("gemini-3.1-pro", start, tt.adapter, writer)
			session.consumeClaudeData("message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant"}}`)
			session.writePreContentKeepalive(start.Add(15 * time.Second))
			result, err := handleAntigravityCompatEmptyStream(c, session)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.NotErrorAs(t, err, &failoverErr)
			require.NotNil(t, result)
			require.True(t, IsResponseCommitted(c))
			require.Contains(t, recorder.Body.String(), tt.want)
		})
	}
}

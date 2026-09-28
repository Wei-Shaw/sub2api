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

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardCommandCodeResponsesLowersToolSearchClientTools(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"gpt-5.4",
		"stream":false,
		"tools":[
			{"type":"tool_search"},
			{"type":"function","name":"deferred_func","description":"deferred","parameters":{"type":"object","properties":{}},"defer_loading":true}
		],
		"input":[
			{"type":"tool_search_call","id":"tsc_search","call_id":"call_search","arguments":{"query":"repo"},"execution":"client","status":"completed"},
			{"type":"tool_search_output","id":"tso_search","call_id":"call_search","output":{"groups":["repo"]},"execution":"client","status":"completed"}
		]
	}`)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"resp_commandcode_search",
			"status":"completed",
			"output":[{"type":"function_call","id":"fc_search_upstream","call_id":"call_search","name":"tool_search","arguments":"{\"query\":\"repo\"}","status":"completed"}],
			"usage":{"input_tokens":10,"output_tokens":5}
		}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          1,
		Name:        "command-code",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "test-api-key",
			"base_url": "https://api.commandcode.ai/provider/v1",
		},
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://api.commandcode.ai/provider/v1/responses", upstream.lastReq.URL.String())

	require.Equal(t, "function", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
	require.Equal(t, "tool_search", gjson.GetBytes(upstream.lastBody, "tools.0.name").String())
	require.Equal(t, "function", gjson.GetBytes(upstream.lastBody, "tools.1.type").String())
	require.Equal(t, "deferred_func", gjson.GetBytes(upstream.lastBody, "tools.1.name").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tools.1.defer_loading").Exists())

	require.Equal(t, "function_call", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
	require.Equal(t, "tool_search", gjson.GetBytes(upstream.lastBody, "input.0.name").String())
	require.Equal(t, "call_search", gjson.GetBytes(upstream.lastBody, "input.0.call_id").String())
	require.JSONEq(t, `{"query":"repo"}`, gjson.GetBytes(upstream.lastBody, "input.0.arguments").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
	require.Equal(t, "call_search", gjson.GetBytes(upstream.lastBody, "input.1.call_id").String())
	require.JSONEq(t, `{"groups":["repo"]}`, gjson.GetBytes(upstream.lastBody, "input.1.output").String())

	require.Equal(t, "tool_search_call", gjson.Get(recorder.Body.String(), "output.0.type").String())
	require.Equal(t, "call_search", gjson.Get(recorder.Body.String(), "output.0.call_id").String())
	require.Equal(t, "client", gjson.Get(recorder.Body.String(), "output.0.execution").String())
	require.JSONEq(t, `{"query":"repo"}`, gjson.Get(recorder.Body.String(), "output.0.arguments").Raw)
	require.False(t, gjson.Get(recorder.Body.String(), "output.0.name").Exists())
}

func TestForwardCommandCodeResponsesLowersToolSearchHistoryWithoutTools(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"gpt-5.4",
		"stream":false,
		"input":[
			{"type":"tool_search_call","id":"tsc_search","call_id":"call_search","arguments":{"query":"repo"},"execution":"client","status":"completed"},
			{"type":"tool_search_output","id":"tso_search","call_id":"call_search","output":{"groups":["repo"]},"execution":"client","status":"completed"}
		]
	}`)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"resp_commandcode_history",
			"status":"completed",
			"output":[],
			"usage":{"input_tokens":8,"output_tokens":2}
		}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          1,
		Name:        "command-code",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "test-api-key",
			"base_url": "https://api.commandcode.ai/provider/v1",
		},
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://api.commandcode.ai/provider/v1/responses", upstream.lastReq.URL.String())

	require.Equal(t, "function_call", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
	require.Equal(t, "tool_search", gjson.GetBytes(upstream.lastBody, "input.0.name").String())
	require.Equal(t, "call_search", gjson.GetBytes(upstream.lastBody, "input.0.call_id").String())
	require.JSONEq(t, `{"query":"repo"}`, gjson.GetBytes(upstream.lastBody, "input.0.arguments").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
	require.Equal(t, "call_search", gjson.GetBytes(upstream.lastBody, "input.1.call_id").String())
	require.JSONEq(t, `{"groups":["repo"]}`, gjson.GetBytes(upstream.lastBody, "input.1.output").String())
}

func TestForwardCommandCodeResponsesRestoresToolSearchStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","stream":true,"tools":[{"type":"tool_search"}],"input":"find tools"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"type":"function_call","id":"search_1","call_id":"call_search","name":"tool_search","status":"in_progress"}}`,
		`data: {"type":"response.function_call_arguments.done","sequence_number":1,"item_id":"search_1","call_id":"call_search","name":"tool_search","arguments":"{\"query\":\"repo\"}"}`,
		`data: {"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"function_call","id":"search_1","call_id":"call_search","name":"tool_search","arguments":"{\"query\":\"repo\"}","status":"completed"}}`,
		`data: {"type":"response.completed","sequence_number":3,"response":{"id":"resp_commandcode_stream","status":"completed","output":[{"type":"function_call","id":"search_1","call_id":"call_search","name":"tool_search","arguments":"{\"query\":\"repo\"}"}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
	}, "\n\n") + "\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}}
	svc := openAIClientToolsTestService(upstream)
	account := &Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-api-key", "base_url": "https://api.commandcode.ai/provider/v1"},
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "function", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
	output := recorder.Body.String()
	require.Contains(t, output, `"type":"tool_search_call"`)
	require.Contains(t, output, `"execution":"client"`)
	require.Contains(t, output, `"query":"repo"`)
	require.NotContains(t, output, `"name":"tool_search"`)
}

func TestForwardOpenAIAPIKeyResponsesKeepsNativeToolSearchShape(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"gpt-5.4",
		"stream":false,
		"tools":[{"type":"tool_search"}],
		"input":[
			{"type":"tool_search_call","id":"tsc_search","call_id":"call_search","arguments":{"query":"repo"},"execution":"client","status":"completed"},
			{"type":"tool_search_output","id":"tso_search","call_id":"call_search","output":{"groups":["repo"]},"execution":"client","status":"completed"}
		]
	}`)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"resp_openai_native",
			"status":"completed",
			"output":[],
			"usage":{"input_tokens":8,"output_tokens":2}
		}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          2,
		Name:        "openai-api-key",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "test-api-key",
		},
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://api.openai.com/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "tool_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
	require.Equal(t, "tool_search_call", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
	require.Equal(t, "tool_search_output", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
}

func TestForwardCommandCodeResponsesForceChatCompletionsBypassesNativeAdaptation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"gpt-5.4",
		"stream":false,
		"tools":[{"type":"tool_search"}],
		"input":[
			{"type":"tool_search_call","id":"tsc_search","call_id":"call_search","arguments":{"query":"repo"},"execution":"client","status":"completed"},
			{"type":"tool_search_output","id":"tso_search","call_id":"call_search","output":{"groups":["repo"]},"execution":"client","status":"completed"}
		]
	}`)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl_commandcode_force",
			"object":"chat.completion",
			"model":"gpt-5.4",
			"choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_search","type":"function","function":{"name":"tool_search","arguments":"{\"query\":\"repo\"}"}}]},"finish_reason":"tool_calls"}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
		}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          1,
		Name:        "command-code",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
		},
		Credentials: map[string]any{
			"api_key":  "test-api-key",
			"base_url": "https://api.commandcode.ai/provider/v1",
		},
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)

	// force_chat_completions keeps the request on the chat-completions bridge,
	// so the native Responses client-tool adapter must not run.
	require.Equal(t, "https://api.commandcode.ai/provider/v1/chat/completions", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "messages").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.Equal(t, "function", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
	require.Equal(t, "tool_search", gjson.GetBytes(upstream.lastBody, "tools.0.function.name").String())

	require.Equal(t, "tool_search_call", gjson.Get(recorder.Body.String(), "output.0.type").String())
	require.Equal(t, "call_search", gjson.Get(recorder.Body.String(), "output.0.call_id").String())
	require.Equal(t, "client", gjson.Get(recorder.Body.String(), "output.0.execution").String())
	require.JSONEq(t, `{"query":"repo"}`, gjson.Get(recorder.Body.String(), "output.0.arguments").Raw)
	require.False(t, gjson.Get(recorder.Body.String(), "output.0.name").Exists())
}

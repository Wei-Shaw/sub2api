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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const bufferedHostedItemsMessagesBody = `{"model":"gpt-5.5","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"search the web"}]}`

const bufferedHostedItemsThinkingBody = `{"model":"gpt-5.5","max_tokens":4096,"stream":false,"thinking":{"type":"enabled","budget_tokens":2048},"messages":[{"role":"user","content":"search the web"}]}`

const bufferedHostedItemsChatBody = `{"model":"gpt-5.5","stream":false,"messages":[{"role":"user","content":"search the web"}]}`

const bufferedHostedItemsCreated = `{"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5"}}`

const bufferedHostedItemsCompleted = `{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`

const (
	bufferedHostedItemsWebSearchDone = `{"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"q","sources":[{"type":"url","url":"https://a.example/1"}]}}}`
	bufferedHostedItemsImageDone     = `{"type":"response.output_item.done","output_index":1,"item":{"type":"image_generation_call","id":"ig_1","status":"completed","result":"aGVsbG8="}}`
	bufferedHostedItemsTextDelta     = `{"type":"response.output_text.delta","output_index":2,"delta":"found it"}`
	bufferedHostedItemsMessageDone   = `{"type":"response.output_item.done","output_index":2,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"found it"}]}}`
)

// bufferedItemsSSE joins JSON events into an SSE body.
func bufferedItemsSSE(events ...string) string {
	var b strings.Builder
	for _, event := range events {
		b.WriteString("data: " + event + "\n\n")
	}
	return b.String()
}

// The upstream reports every output item on output_item.done but ends with an
// empty terminal output.
var bufferedHostedItemsSSE = bufferedItemsSSE(
	bufferedHostedItemsCreated,
	`{"type":"response.output_item.added","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"in_progress"}}`,
	bufferedHostedItemsWebSearchDone,
	bufferedHostedItemsImageDone,
	bufferedHostedItemsTextDelta,
	bufferedHostedItemsMessageDone,
	bufferedHostedItemsCompleted,
)

func bufferedHostedItemsOAuthAccount() *Account {
	return &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
	}
}

func bufferedHostedItemsSSEResponse(sse string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}
}

func forwardBufferedHostedItems(t *testing.T, path, requestBody, upstreamSSE string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(requestBody)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	svc := &OpenAIGatewayService{
		httpUpstream: &httpUpstreamRecorder{resp: bufferedHostedItemsSSEResponse(upstreamSSE)},
		cfg:          rawChatCompletionsTestConfig(),
	}
	account := bufferedHostedItemsOAuthAccount()
	var err error
	if path == "/v1/chat/completions" {
		_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	} else {
		_, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
	}
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return rec.Body.String()
}

func TestForwardAsAnthropic_CodexOAuthNonStreamingKeepsHostedOutputItems(t *testing.T) {
	out := forwardBufferedHostedItems(t, "/v1/messages", bufferedHostedItemsMessagesBody, bufferedHostedItemsSSE)

	content := gjson.Get(out, "content").Array()
	require.Len(t, content, 3, out)
	require.Equal(t, "server_tool_use", content[0].Get("type").String())
	require.Equal(t, "web_search", content[0].Get("name").String())
	require.Equal(t, "q", content[0].Get("input.query").String())
	require.Equal(t, "web_search_tool_result", content[1].Get("type").String())
	require.Equal(t, content[0].Get("id").String(), content[1].Get("tool_use_id").String())
	require.Equal(t, "text", content[2].Get("type").String())
	require.Equal(t, "found it", content[2].Get("text").String())
}

func TestForwardAsAnthropic_BufferedHostedItemsFollowReasoning(t *testing.T) {
	sse := bufferedItemsSSE(
		bufferedHostedItemsCreated,
		`{"type":"response.reasoning_summary_text.delta","output_index":0,"summary_index":0,"delta":"plan"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[]}}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"q"}}}`,
		`{"type":"response.output_text.delta","output_index":2,"delta":"found it"}`,
		bufferedHostedItemsCompleted,
	)
	out := forwardBufferedHostedItems(t, "/v1/messages", bufferedHostedItemsThinkingBody, sse)

	var types []string
	for _, block := range gjson.Get(out, "content").Array() {
		types = append(types, block.Get("type").String())
	}
	require.Equal(t, []string{"thinking", "server_tool_use", "web_search_tool_result", "text"}, types, out)
}

func TestReadOpenAICompatBufferedTerminal_EmptyTerminalOutputGainsHostedItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}

	response, _, acc, _, err := (&OpenAIGatewayService{}).readOpenAICompatBufferedTerminal(
		bufferedHostedItemsSSEResponse(bufferedHostedItemsSSE), c, account, "openai chat buffered", "req-1")
	require.NoError(t, err)
	require.NotNil(t, response)

	var types []string
	for _, item := range response.Output {
		types = append(types, item.Type)
	}
	require.Equal(t, []string{"web_search_call", "image_generation_call", "message"}, types)

	// The handler supplements the output afterwards; the text must not be added twice.
	acc.SupplementResponseOutput(response)
	require.Len(t, response.Output, 3)
}

func TestReadOpenAICompatBufferedTerminal_HostedItemsWithoutOutputIndexKeepArrivalOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}
	sse := bufferedItemsSSE(
		bufferedHostedItemsCreated,
		`{"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws_1","status":"completed"}}`,
		`{"type":"response.output_item.done","item":{"type":"image_generation_call","id":"ig_1","status":"completed"}}`,
		bufferedHostedItemsCompleted,
	)

	response, _, _, _, err := (&OpenAIGatewayService{}).readOpenAICompatBufferedTerminal(
		bufferedHostedItemsSSEResponse(sse), c, account, "openai chat buffered", "req-1")
	require.NoError(t, err)

	var ids []string
	for _, item := range response.Output {
		ids = append(ids, item.ID)
	}
	require.Equal(t, []string{"ws_1", "ig_1"}, ids)
}

// With an empty terminal output the accumulator keeps building reasoning, text
// and function calls from deltas. Only the types it does not know are added.
func TestBufferedCompat_EmptyTerminalOutputKeepsAccumulatedItems(t *testing.T) {
	fnTool := `"tools":[{"type":"function","function":{"name":"get","parameters":{"type":"object"}}}]`
	customTool := `"tools":[{"type":"custom","custom":{"name":"exec","description":"run"}}]`
	chatBody := func(extra string) string {
		return `{"model":"gpt-5.5","stream":false,` + extra + `,"messages":[{"role":"user","content":"hi"}]}`
	}
	toolMessagesBody := `{"model":"gpt-5.5","max_tokens":64,"stream":false,"tools":[{"name":"get","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hi"}]}`

	// The reasoning done item has an empty summary; its text only streamed as deltas.
	reasoningSSE := bufferedItemsSSE(
		bufferedHostedItemsCreated,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[]}}`,
		`{"type":"response.reasoning_summary_text.delta","output_index":0,"summary_index":0,"delta":"thinking hard"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[]}}`,
		`{"type":"response.output_text.delta","output_index":1,"delta":"answer"}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"answer"}]}}`,
		bufferedHostedItemsCompleted,
	)
	// Item events without output_index, two calls.
	noIndexSSE := bufferedItemsSSE(
		bufferedHostedItemsCreated,
		`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"get","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"a\":1}"}`,
		`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"get","arguments":"{\"a\":1}","status":"completed"}}`,
		`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"get","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_2","delta":"{\"b\":2}"}`,
		`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"get","arguments":"{\"b\":2}","status":"completed"}}`,
		bufferedHostedItemsCompleted,
	)
	customSSE := bufferedItemsSSE(
		bufferedHostedItemsCreated,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"custom_tool_call","id":"ctc_1","call_id":"call_1","name":"exec","input":""}}`,
		`{"type":"response.custom_tool_call_input.delta","output_index":0,"item_id":"ctc_1","delta":"dir"}`,
		`{"type":"response.custom_tool_call_input.done","output_index":0,"item_id":"ctc_1","input":"dir"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"custom_tool_call","id":"ctc_1","call_id":"call_1","name":"exec","input":"dir","status":"completed"}}`,
		bufferedHostedItemsCompleted,
	)

	t.Run("reasoning text from deltas, chat", func(t *testing.T) {
		out := forwardBufferedHostedItems(t, "/v1/chat/completions", chatBody(`"reasoning_effort":"high"`), reasoningSSE)
		require.Equal(t, "thinking hard", gjson.Get(out, "choices.0.message.reasoning_content").String(), out)
		require.Equal(t, "answer", gjson.Get(out, "choices.0.message.content").String(), out)
	})
	t.Run("reasoning text from deltas, messages", func(t *testing.T) {
		out := forwardBufferedHostedItems(t, "/v1/messages", bufferedHostedItemsThinkingBody, reasoningSSE)
		require.Equal(t, "thinking", gjson.Get(out, "content.0.type").String(), out)
		require.Equal(t, "thinking hard", gjson.Get(out, "content.0.thinking").String(), out)
		require.Equal(t, "answer", gjson.Get(out, "content.1.text").String(), out)
	})
	t.Run("two function calls without output_index, chat", func(t *testing.T) {
		out := forwardBufferedHostedItems(t, "/v1/chat/completions", chatBody(fnTool), noIndexSSE)
		calls := gjson.Get(out, "choices.0.message.tool_calls").Array()
		require.Len(t, calls, 2, out)
		require.Equal(t, "call_1", calls[0].Get("id").String())
		require.Equal(t, "call_2", calls[1].Get("id").String())
	})
	t.Run("two function calls without output_index, messages", func(t *testing.T) {
		out := forwardBufferedHostedItems(t, "/v1/messages", toolMessagesBody, noIndexSSE)
		content := gjson.Get(out, "content").Array()
		require.Len(t, content, 2, out)
		require.Equal(t, "call_1", content[0].Get("id").String())
		require.Equal(t, "call_2", content[1].Get("id").String())
	})
	t.Run("custom tool call, chat", func(t *testing.T) {
		out := forwardBufferedHostedItems(t, "/v1/chat/completions", chatBody(customTool), customSSE)
		calls := gjson.Get(out, "choices.0.message.tool_calls").Array()
		require.Len(t, calls, 1, out)
		require.Equal(t, "exec", calls[0].Get("function.name").String())
		require.Equal(t, "dir", calls[0].Get("function.arguments").String())
	})
}

// Native search calls are billed from the terminal Responses body, so an empty
// terminal output that is rebuilt with hosted items must count them, and a
// terminal output that already has items must keep the count it reports.
func TestBufferedCompat_GrokSearchCountFromRebuiltHostedItems(t *testing.T) {
	const searchDone = `{"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"q"}}}`
	const xSearchDone = `{"type":"response.output_item.done","output_index":1,"item":{"type":"x_search_call","id":"xs_1","status":"completed"}}`
	const messageDone = `{"type":"response.output_item.done","output_index":2,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"found it"}]}}`
	const usage = `"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}`
	completed := func(output string) string {
		return `{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":` + output + `,` + usage + `}}`
	}
	stream := func(terminalOutput string) string {
		return bufferedItemsSSE(bufferedHostedItemsCreated, searchDone, xSearchDone, bufferedHostedItemsTextDelta, messageDone, completed(terminalOutput))
	}

	cases := []struct {
		name           string
		terminalOutput string
		wantSearches   int
	}{
		{"empty terminal output", `[]`, 2},
		{"terminal output with the search items", `[{"type":"web_search_call","id":"ws_1","status":"completed"},{"type":"x_search_call","id":"xs_1","status":"completed"},{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"found it"}]}]`, 2},
		{"terminal output without search items", `[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"found it"}]}]`, 0},
	}
	for _, tc := range cases {
		for _, path := range []string{"/v1/messages", "/v1/chat/completions"} {
			t.Run(tc.name+" "+path, func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				body := []byte(`{"model":"grok-4.7","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
				c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set("api_key", &APIKey{ID: 88000})
				account := healthyGrokOAuthGatewayTestAccount(4242, "access-token")
				svc := grokAuditOAuthService(&httpUpstreamRecorder{resp: bufferedHostedItemsSSEResponse(stream(tc.terminalOutput))}, account)

				var result *OpenAIForwardResult
				var err error
				if path == "/v1/chat/completions" {
					result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				} else {
					result, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
				}
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.NotNil(t, result)
				require.Equal(t, tc.wantSearches, result.SearchCount)
			})
		}
	}
}

// The output of a failed terminal only feeds the error payload, so hosted items
// reported before the failure must not change the payload recorded for ops.
func TestBufferedCompat_FailedTerminalKeepsErrorPayload(t *testing.T) {
	const failed = `{"type":"response.failed","response":{"id":"resp_err","object":"response","status":"failed","error":{"type":"rate_limit_error","message":"Rate limit reached"},"output":[],"usage":{"input_tokens":10,"output_tokens":0,"total_tokens":10}}}`
	plain := bufferedItemsSSE(bufferedHostedItemsCreated, failed)
	withHosted := bufferedItemsSSE(bufferedHostedItemsCreated, bufferedHostedItemsWebSearchDone, bufferedHostedItemsImageDone, failed)

	recordedPayload := func(t *testing.T, path, requestBody, sse string) string {
		t.Helper()
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		body := []byte(requestBody)
		c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		cfg := rawChatCompletionsTestConfig()
		cfg.Gateway.LogUpstreamErrorBody = true
		cfg.Gateway.LogUpstreamErrorBodyMaxBytes = 4096
		svc := &OpenAIGatewayService{
			httpUpstream: &httpUpstreamRecorder{resp: bufferedHostedItemsSSEResponse(sse)},
			cfg:          cfg,
		}
		account := bufferedHostedItemsOAuthAccount()
		var err error
		if path == "/v1/chat/completions" {
			_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
		} else {
			_, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
		}
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		events, _ := c.Get(OpsUpstreamErrorsKey)
		recorded, _ := events.([]*OpsUpstreamErrorEvent)
		require.NotEmpty(t, recorded)
		return recorded[0].Detail
	}

	for path, requestBody := range map[string]string{
		"/v1/messages":         bufferedHostedItemsMessagesBody,
		"/v1/chat/completions": bufferedHostedItemsChatBody,
	} {
		t.Run(path, func(t *testing.T) {
			want := recordedPayload(t, path, requestBody, plain)
			require.Contains(t, want, `"output":[]`)
			require.Equal(t, want, recordedPayload(t, path, requestBody, withHosted))
		})
	}
}

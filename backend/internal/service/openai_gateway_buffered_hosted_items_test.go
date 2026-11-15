//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func readBufferedOutputItems(t *testing.T, sse string) []string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}
	response, _, _, _, err := (&OpenAIGatewayService{}).readOpenAICompatBufferedTerminal(
		bufferedHostedItemsSSEResponse(sse), c, account, "openai chat buffered", "req-1")
	require.NoError(t, err)
	require.NotNil(t, response)
	var items []string
	for _, item := range response.Output {
		items = append(items, item.Type+":"+item.ID+":"+item.Arguments+":"+item.Input)
	}
	return items
}

// Items that share an output_index stay apart; the same item reported again
// replaces its earlier copy; equal indexes keep arrival order.
func TestReadOpenAICompatBufferedTerminal_DoneItemsMergeByItemID(t *testing.T) {
	done := func(index int, id string) string {
		return `{"type":"response.output_item.done","output_index":` + strconv.Itoa(index) + `,"item":{"type":"web_search_call","id":"` + id + `","status":"completed"}}`
	}
	items := readBufferedOutputItems(t, bufferedItemsSSE(
		bufferedHostedItemsCreated,
		done(0, "ws_a"), done(0, "ws_b"), done(1, "ws_c"), done(0, "ws_a"),
		bufferedHostedItemsCompleted,
	))
	require.Equal(t, []string{"web_search_call:ws_a::", "web_search_call:ws_b::", "web_search_call:ws_c::"}, items)
}

// The done item is the upstream's final form of an item; deltas only complete a
// text, refusal or arguments field that the done item left empty, and rebuild an
// item that never got a done event.
func TestReadOpenAICompatBufferedTerminal_DoneItemsWinOverDeltas(t *testing.T) {
	items := readBufferedOutputItems(t, bufferedItemsSSE(
		bufferedHostedItemsCreated,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"get","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"stale\":1}"}`,
		`{"type":"response.function_call_arguments.done","output_index":0,"arguments":"{\"a\":1}"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"get","arguments":""}}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"get","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"b\":2}"}`,
		`{"type":"response.output_item.done","output_index":2,"item":{"type":"function_call","id":"fc_3","call_id":"call_3","name":"get","arguments":"{\"c\":3}"}}`,
		bufferedHostedItemsCompleted,
	))
	require.Equal(t, []string{
		`function_call:fc_1:{"a":1}:`,
		`function_call:fc_2:{"b":2}:`,
		`function_call:fc_3:{"c":3}:`,
	}, items)
}

func TestForwardAsAnthropic_CancelledTerminalKeepsHostedOutputItems(t *testing.T) {
	cancelled := `{"type":"response.cancelled","response":{"id":"resp_1","status":"cancelled","output":[],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`
	out := forwardBufferedHostedItems(t, "/v1/messages", bufferedHostedItemsMessagesBody,
		bufferedItemsSSE(bufferedHostedItemsCreated, bufferedHostedItemsWebSearchDone, cancelled))
	require.Equal(t, "server_tool_use", gjson.Get(out, "content.0.type").String(), out)
}

// A tool call whose arguments are not a JSON object must not empty the response:
// a truncated function call gets {}, a custom tool call its input wrapped.
func TestForwardAsAnthropic_BufferedNonObjectToolInput(t *testing.T) {
	toolBody := `{"model":"gpt-5.5","max_tokens":64,"stream":false,"tools":[{"name":"get","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hi"}]}`
	truncated := bufferedItemsSSE(
		bufferedHostedItemsCreated,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"plan"}]}}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"get","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"a\":1"}`,
		`{"type":"response.incomplete","response":{"id":"resp_1","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}}`,
	)
	out := forwardBufferedHostedItems(t, "/v1/messages", toolBody, truncated)
	require.Equal(t, "thinking", gjson.Get(out, "content.0.type").String(), out)
	require.Equal(t, "plan", gjson.Get(out, "content.0.thinking").String(), out)
	require.Equal(t, "tool_use", gjson.Get(out, "content.1.type").String(), out)
	require.Equal(t, "{}", gjson.Get(out, "content.1.input").Raw, out)
	require.Equal(t, "max_tokens", gjson.Get(out, "stop_reason").String(), out)

	custom := bufferedItemsSSE(
		bufferedHostedItemsCreated,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"custom_tool_call","id":"ctc_1","call_id":"call_1","name":"get","input":""}}`,
		`{"type":"response.custom_tool_call_input.delta","output_index":0,"delta":"dir"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"custom_tool_call","id":"ctc_1","call_id":"call_1","name":"get","input":"dir"}}`,
		bufferedHostedItemsCompleted,
	)
	out = forwardBufferedHostedItems(t, "/v1/messages", toolBody, custom)
	require.Equal(t, "tool_use", gjson.Get(out, "content.0.type").String(), out)
	require.Equal(t, `{"input":"dir"}`, gjson.Get(out, "content.0.input").Raw, out)
	require.Equal(t, "tool_use", gjson.Get(out, "stop_reason").String(), out)
}

// A reasoning item with a summary and raw reasoning_text content: the converters render one of the two,
// so the deltas that carried both supply the text, as the streaming path shows it.
func TestForwardAsAnthropic_BufferedReasoningSummaryAndContentKeepBoth(t *testing.T) {
	sse := bufferedItemsSSE(
		bufferedHostedItemsCreated,
		`{"type":"response.reasoning_text.delta","output_index":0,"content_index":0,"delta":"raw thoughts"}`,
		`{"type":"response.reasoning_summary_text.delta","output_index":0,"summary_index":0,"delta":"summary"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"summary"}],"content":[{"type":"reasoning_text","text":"raw thoughts"}]}}`,
		bufferedHostedItemsCompleted,
	)
	out := forwardBufferedHostedItems(t, "/v1/messages", bufferedHostedItemsThinkingBody, sse)
	require.Equal(t, "raw thoughtssummary", gjson.Get(out, "content.0.thinking").String(), out)
}

// bufferedPartsEntries are the two buffered entries, each with the request that
// makes it show the field under test.
var bufferedPartsEntries = []struct{ name, path, body string }{
	{"messages", "/v1/messages", bufferedHostedItemsThinkingBody},
	{"chat", "/v1/chat/completions", `{"model":"gpt-5.5","stream":false,"reasoning_effort":"high","messages":[{"role":"user","content":"search the web"}]}`},
}

// bufferedPartsVisible is the text of one kind ("text", "thinking" or "refusal")
// the client is shown: the text or thinking blocks joined for /v1/messages, the
// content, reasoning_content or refusal of the message for /v1/chat/completions.
func bufferedPartsVisible(out, path, kind string) string {
	if path == "/v1/chat/completions" {
		return gjson.Get(out, "choices.0.message."+map[string]string{"text": "content", "thinking": "reasoning_content", "refusal": "refusal"}[kind]).String()
	}
	var text strings.Builder
	for _, block := range gjson.Get(out, "content").Array() {
		if block.Get("type").String() == kind {
			text.WriteString(block.Get(kind).String())
		}
	}
	return text.String()
}

// One item with several parts whose done item leaves only some of them empty, and
// an empty terminal output: a part is completed with the deltas of its own part
// index, a part that has text and the order of the parts stay as the done item
// has them.
func TestBufferedCompat_PartialTextParts(t *testing.T) {
	text := func(s string) string { return `{"type":"output_text","text":"` + s + `"}` }
	cases := []struct {
		name, parts, want string
		deltas            []string
	}{
		{"first empty", "[" + text("") + "," + text("second") + "]", "first second", []string{"first ", "second"}},
		{"second empty", "[" + text("first ") + "," + text("") + "]", "first second", []string{"first ", "second"}},
		{"both empty", "[" + text("") + "," + text("") + "]", "first second", []string{"first ", "second"}},
		{"both with text, the done item wins", "[" + text("one ") + "," + text("two") + "]", "one two", []string{"first ", "second"}},
		{"middle with text of three", "[" + text("") + "," + text("b ") + "," + text("") + "]", "a b c", []string{"a ", "b ", "c"}},
	}
	for _, entry := range bufferedPartsEntries {
		for _, tc := range cases {
			t.Run(entry.name+" "+tc.name, func(t *testing.T) {
				events := []string{
					bufferedHostedItemsCreated,
					`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","content":[]}}`,
				}
				for index, delta := range tc.deltas {
					events = append(events, `{"type":"response.output_text.delta","output_index":0,"content_index":`+strconv.Itoa(index)+`,"delta":"`+delta+`"}`)
				}
				events = append(events,
					`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","content":`+tc.parts+`}}`,
					bufferedHostedItemsCompleted)
				sse := bufferedItemsSSE(events...)
				out := forwardBufferedHostedItems(t, entry.path, entry.body, sse)
				require.Equal(t, tc.want, bufferedPartsVisible(out, entry.path, "text"), out)
			})
		}
	}
}

// The same for the refusal parts of a message, which only /v1/chat/completions renders.
func TestBufferedCompat_PartialRefusalParts(t *testing.T) {
	refusal := func(s string) string { return `{"type":"refusal","refusal":"` + s + `"}` }
	cases := []struct{ name, parts, want string }{
		{"first empty", "[" + refusal("") + "," + refusal("second") + "]", "first second"},
		{"second empty", "[" + refusal("first ") + "," + refusal("") + "]", "first second"},
		{"both empty", "[" + refusal("") + "," + refusal("") + "]", "first second"},
		{"both with text, the done item wins", "[" + refusal("one ") + "," + refusal("two") + "]", "one two"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sse := bufferedItemsSSE(
				bufferedHostedItemsCreated,
				`{"type":"response.refusal.delta","output_index":0,"content_index":0,"delta":"first "}`,
				`{"type":"response.refusal.delta","output_index":0,"content_index":1,"delta":"second"}`,
				`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","content":`+tc.parts+`}}`,
				bufferedHostedItemsCompleted,
			)
			out := forwardBufferedHostedItems(t, "/v1/chat/completions", bufferedHostedItemsChatBody, sse)
			require.Equal(t, tc.want, bufferedPartsVisible(out, "/v1/chat/completions", "refusal"), out)
		})
	}
}

// A refusal part and a text part of one message keep their own deltas.
func TestBufferedCompat_RefusalAndTextPartsKeepTheirOwnDeltas(t *testing.T) {
	sse := bufferedItemsSSE(
		bufferedHostedItemsCreated,
		`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"answer"}`,
		`{"type":"response.refusal.delta","output_index":0,"content_index":1,"delta":"no"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":""},{"type":"refusal","refusal":""}]}}`,
		bufferedHostedItemsCompleted,
	)
	out := forwardBufferedHostedItems(t, "/v1/chat/completions", bufferedHostedItemsChatBody, sse)
	require.Equal(t, "answer", bufferedPartsVisible(out, "/v1/chat/completions", "text"), out)
	require.Equal(t, "no", bufferedPartsVisible(out, "/v1/chat/completions", "refusal"), out)
}

// Reasoning with several summary parts, or several reasoning_text parts, whose
// done item leaves only some of them empty.
func TestBufferedCompat_PartialReasoningParts(t *testing.T) {
	summary := func(s string) string { return `{"type":"summary_text","text":"` + s + `"}` }
	raw := func(s string) string { return `{"type":"reasoning_text","text":"` + s + `"}` }
	summaryDeltas := []string{
		`{"type":"response.reasoning_summary_text.delta","output_index":0,"summary_index":0,"delta":"first "}`,
		`{"type":"response.reasoning_summary_text.delta","output_index":0,"summary_index":1,"delta":"second"}`,
	}
	rawDeltas := []string{
		`{"type":"response.reasoning_text.delta","output_index":0,"content_index":0,"delta":"first "}`,
		`{"type":"response.reasoning_text.delta","output_index":0,"content_index":1,"delta":"second"}`,
	}
	cases := []struct {
		name             string
		deltas           []string
		summary, content string
		want             string
	}{
		{"summary, first empty", summaryDeltas, "[" + summary("") + "," + summary("second") + "]", "[]", "first second"},
		{"summary, second empty", summaryDeltas, "[" + summary("first ") + "," + summary("") + "]", "[]", "first second"},
		{"summary, both empty", summaryDeltas, "[" + summary("") + "," + summary("") + "]", "[]", "first second"},
		{"summary, both with text, the done item wins", summaryDeltas, "[" + summary("one ") + "," + summary("two") + "]", "[]", "one two"},
		{"reasoning_text, first empty", rawDeltas, "[]", "[" + raw("") + "," + raw("second") + "]", "first second"},
		{"reasoning_text, second empty", rawDeltas, "[]", "[" + raw("first ") + "," + raw("") + "]", "first second"},
		{"reasoning_text, both empty", rawDeltas, "[]", "[" + raw("") + "," + raw("") + "]", "first second"},
		{"reasoning_text, both with text, the done item wins", rawDeltas, "[]", "[" + raw("one ") + "," + raw("two") + "]", "one two"},
	}
	for _, entry := range bufferedPartsEntries {
		for _, tc := range cases {
			t.Run(entry.name+" "+tc.name, func(t *testing.T) {
				events := append([]string{bufferedHostedItemsCreated}, tc.deltas...)
				events = append(events,
					`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":`+tc.summary+`,"content":`+tc.content+`}}`,
					bufferedHostedItemsCompleted)
				out := forwardBufferedHostedItems(t, entry.path, entry.body, bufferedItemsSSE(events...))
				require.Equal(t, tc.want, bufferedPartsVisible(out, entry.path, "thinking"), out)
			})
		}
	}
}

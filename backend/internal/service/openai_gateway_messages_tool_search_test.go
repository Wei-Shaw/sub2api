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

const messagesToolSearchBody = `{"model":"gpt-6-astra","max_tokens":64,"stream":true,
 "tools":[
  {"name":"Bash","description":"run","input_schema":{"type":"object","properties":{"command":{"type":"string"}}}},
  {"name":"ToolSearch","description":"search deferred tools","input_schema":{"type":"object","properties":{"query":{"type":"string"},"max_results":{"type":"number"}},"required":["query","max_results"]}},
  {"name":"WebFetch","description":"fetch a url","input_schema":{"type":"object","properties":{"url":{"type":"string"}}},"defer_loading":true}],
 "messages":[
  {"role":"user","content":"fetch the page"},
  {"role":"assistant","content":[{"type":"tool_use","id":"toolu_search","name":"ToolSearch","input":{"query":"select:WebFetch","max_results":5}}]},
  {"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_search","content":[{"type":"tool_reference","tool_name":"WebFetch"}]}]}]}`

func forwardMessagesToolSearch(t *testing.T, account *Account, requestBody, upstreamSSE, mappedModel string) (*httpUpstreamRecorder, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(requestBody)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          rawChatCompletionsTestConfig(),
	}
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", mappedModel)
	require.NoError(t, err)
	return upstream, rec
}

const toolSearchCallSSE = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_ts\",\"model\":\"gpt-6-astra\"}}\n\n" +
	"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"tool_search_call\",\"call_id\":\"call_search_2\",\"execution\":\"client\",\"status\":\"completed\",\"arguments\":{\"query\":\"notebook\",\"max_results\":5}}}\n\n" +
	"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_ts\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":5,\"output_tokens\":2,\"total_tokens\":7}}}\n\n"

func TestForwardAsAnthropic_CodexOAuthUsesNativeToolSearch(t *testing.T) {
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
	}
	upstream, rec := forwardMessagesToolSearch(t, account, messagesToolSearchBody, toolSearchCallSSE, "gpt-6-astra")

	tools := gjson.GetBytes(upstream.lastBody, "tools").Array()
	require.Len(t, tools, 2, "deferred WebFetch stays out of the top-level tools")
	require.Equal(t, "Bash", tools[0].Get("name").String())
	require.Equal(t, "tool_search", tools[1].Get("type").String())
	require.Equal(t, "client", tools[1].Get("execution").String())

	var types []string
	for _, item := range gjson.GetBytes(upstream.lastBody, "input").Array() {
		types = append(types, item.Get("type").String())
	}
	require.Contains(t, types, "tool_search_call")
	require.Contains(t, types, "tool_search_output")
	require.NotContains(t, types, "function_call")

	out := rec.Body.String()
	require.Contains(t, out, `"name":"ToolSearch"`)
	require.Contains(t, out, `"id":"call_search_2"`)
	require.Contains(t, out, `"stop_reason":"tool_use"`)
}

func TestForwardAsAnthropic_NonCodexAccountKeepsFunctionToolSearch(t *testing.T) {
	completed := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_fn\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":5,\"output_tokens\":2,\"total_tokens\":7}}}\n\n"
	upstream, _ := forwardMessagesToolSearch(t, rawChatCompletionsTestAccount(), messagesToolSearchBody, completed, "gpt-6-astra")

	var names []string
	for _, tool := range gjson.GetBytes(upstream.lastBody, "tools").Array() {
		require.Equal(t, "function", tool.Get("type").String())
		names = append(names, tool.Get("name").String())
	}
	require.Equal(t, []string{"Bash", "ToolSearch", "WebFetch"}, names)
	require.NotContains(t, string(upstream.lastBody), "tool_search_call")
}

// A Codex OAuth model without supports_search_tool keeps ToolSearch and the
// deferred tools as plain function tools.
func TestForwardAsAnthropic_CodexOAuthModelWithoutSearchSupportKeepsFunctionToolSearch(t *testing.T) {
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
	}
	completed := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_fn\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":5,\"output_tokens\":2,\"total_tokens\":7}}}\n\n"
	body := strings.Replace(messagesToolSearchBody, `"model":"gpt-6-astra"`, `"model":"gpt-5.5"`, 1)
	upstream, _ := forwardMessagesToolSearch(t, account, body, completed, "gpt-5.5")

	var names []string
	for _, tool := range gjson.GetBytes(upstream.lastBody, "tools").Array() {
		require.Equal(t, "function", tool.Get("type").String())
		names = append(names, tool.Get("name").String())
	}
	require.Equal(t, []string{"Bash", "ToolSearch", "WebFetch"}, names)
	require.NotContains(t, string(upstream.lastBody), "tool_search_call")
}

func codexOAuthToolSearchAccount() *Account {
	return &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
	}
}

const toolSearchNoopSSE = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_noop\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":5,\"output_tokens\":2,\"total_tokens\":7}}}\n\n"

// A revealed tool is declared in tool_search_output and replayed as a
// function_call; both must carry the same upstream name.
func TestForwardAsAnthropic_CodexOAuthRevealedToolNameMatchesReplay(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"python", "python__sub2api"},
		{"Python", "python__sub2api"},
		{"Echo", "Echo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.ReplaceAll(messagesToolSearchBody, "WebFetch", tc.name)
			body = strings.TrimSuffix(body, "]}") + `,
  {"role":"assistant","content":[{"type":"tool_use","id":"toolu_call","name":"` + tc.name + `","input":{"url":"https://example.com"}}]},
  {"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_call","content":"ok"}]}]}`
			upstream, _ := forwardMessagesToolSearch(t, codexOAuthToolSearchAccount(), body, toolSearchNoopSSE, "gpt-6-astra")

			var declared, replayed []string
			for _, item := range gjson.GetBytes(upstream.lastBody, "input").Array() {
				switch item.Get("type").String() {
				case "tool_search_output":
					for _, tool := range item.Get("tools").Array() {
						declared = append(declared, tool.Get("name").String())
					}
				case "function_call":
					replayed = append(replayed, item.Get("name").String())
				}
			}
			require.Equal(t, []string{tc.want}, declared)
			require.Equal(t, []string{tc.want}, replayed)
		})
	}
}

// Selections that name a top-level function or carry no name are forwarded
// as the bridge produced them.
func TestForwardAsAnthropic_CodexOAuthToolChoiceControls(t *testing.T) {
	for choice, want := range map[string]string{
		`{"type":"tool","name":"Bash"}`: `{"type":"function","name":"Bash"}`,
		`{"type":"auto"}`:               `"auto"`,
		`{"type":"any"}`:                `"required"`,
	} {
		t.Run(choice, func(t *testing.T) {
			body := strings.Replace(messagesToolSearchBody, `"stream":true,`, `"stream":true,"tool_choice":`+choice+`,`, 1)
			upstream, _ := forwardMessagesToolSearch(t, codexOAuthToolSearchAccount(), body, toolSearchNoopSSE, "gpt-6-astra")
			require.JSONEq(t, want, gjson.GetBytes(upstream.lastBody, "tool_choice").Raw)
		})
	}
}

func TestForwardAsAnthropic_CodexOAuthForcedHiddenToolKeepsFunctionTools(t *testing.T) {
	for _, name := range []string{"ToolSearch", "WebFetch"} {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(messagesToolSearchBody, `"stream":true,`, `"stream":true,"tool_choice":{"type":"tool","name":"`+name+`"},`, 1)
			upstream, _ := forwardMessagesToolSearch(t, codexOAuthToolSearchAccount(), body, toolSearchNoopSSE, "gpt-6-astra")

			var names []string
			for _, tool := range gjson.GetBytes(upstream.lastBody, "tools").Array() {
				require.Equal(t, "function", tool.Get("type").String())
				names = append(names, tool.Get("name").String())
			}
			require.Equal(t, []string{"Bash", "ToolSearch", "WebFetch"}, names)
			require.JSONEq(t, `{"type":"function","name":"`+name+`"}`, gjson.GetBytes(upstream.lastBody, "tool_choice").Raw)
			require.NotContains(t, string(upstream.lastBody), "tool_search_call")
			require.NotContains(t, string(upstream.lastBody), "tool_search_output")
		})
	}
}

// Claude Code also sends stream:false turns; their upstream stream can end
// with an empty terminal output, rebuilt from the buffered output items.
func TestForwardAsAnthropic_CodexOAuthNonStreamingRestoresToolSearchCall(t *testing.T) {
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
	}
	body := strings.Replace(messagesToolSearchBody, `"stream":true`, `"stream":false`, 1)
	_, rec := forwardMessagesToolSearch(t, account, body, toolSearchCallSSE, "gpt-6-astra")

	out := rec.Body.String()
	require.Contains(t, out, `"name":"ToolSearch"`)
	require.Contains(t, out, `"id":"call_search_2"`)
	require.Contains(t, out, `"stop_reason":"tool_use"`)
	require.Equal(t, "notebook", gjson.Get(out, "content.0.input.query").String())
}

// Grok OAuth accounts also speak the Codex protocol, but xAI has no native
// tool_search: ToolSearch and deferred tools stay function tools there.
func TestForwardAsAnthropic_GrokOAuthKeepsFunctionToolSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(messagesToolSearchBody)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	account := healthyGrokOAuthGatewayTestAccount(71, "access-token")
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{71: account}}}
	upstream := &httpUpstreamRecorder{resp: grokMessagesSSECompletedResponse("resp_grok_ts", 0)}
	svc := &OpenAIGatewayService{httpUpstream: upstream, grokTokenProvider: NewGrokTokenProvider(repo, nil), accountRepo: repo}

	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
	require.NoError(t, err)

	var functions []string
	for _, tool := range gjson.GetBytes(upstream.lastBody, "tools").Array() {
		require.NotEqual(t, "tool_search", tool.Get("type").String())
		if tool.Get("type").String() == "function" {
			functions = append(functions, tool.Get("name").String())
		}
	}
	require.Contains(t, functions, "ToolSearch")
	require.Contains(t, functions, "WebFetch")
	require.NotContains(t, string(upstream.lastBody), "tool_search_call")
	require.NotContains(t, string(upstream.lastBody), "tool_search_output")
}

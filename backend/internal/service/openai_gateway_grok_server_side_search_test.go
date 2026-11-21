//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// X Search runs on the xAI server and arrives as a custom_tool_call whose call_id
// starts with xs_ (attachment search: as_). The item shape follows the CustomToolCall
// schema in https://api.x.ai/api-docs/openapi.json.
const (
	grokXSearchItemJSON = `{"type":"custom_tool_call","id":"ctc_1","call_id":"xs_123","name":"x_keyword_search","input":"{\"query\":\"q\"}","status":"completed"}`
	grokASearchItemJSON = `{"type":"custom_tool_call","id":"ctc_2","call_id":"as_456","name":"attachment_search","input":"{\"query\":\"q\"}","status":"completed"}`
	grokClientToolJSON  = `{"type":"custom_tool_call","id":"ctc_3","call_id":"call_9","name":"exec","input":"{\"cmd\":\"dir\"}","status":"completed"}`
	grokMessageItemJSON = `{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"found it"}]}`
)

func grokSSEFrame(eventType, payload string) string {
	if payload == "" {
		return fmt.Sprintf("data: {\"type\":%q}\n\n", eventType)
	}
	return fmt.Sprintf("data: {\"type\":%q,%s}\n\n", eventType, payload)
}

// grokCustomToolCallStream streams the given custom_tool_call items in order and then a text
// answer. terminalOutput is the output array of the terminal event.
func grokCustomToolCallStream(items []string, terminalOutput string) string {
	var sb strings.Builder
	sb.WriteString(grokSSEFrame("response.created", `"response":{"id":"resp_1","model":"grok-4.7"}`))
	for index, item := range items {
		callID := gjson.Get(item, "call_id").String()
		name := gjson.Get(item, "name").String()
		input := gjson.Get(item, "input").String()
		sb.WriteString(grokSSEFrame("response.output_item.added", fmt.Sprintf(`"output_index":%d,"item":{"type":"custom_tool_call","id":"ctc_%d","call_id":%q,"name":%q,"input":""}`, index, index, callID, name)))
		sb.WriteString(grokSSEFrame("response.custom_tool_call_input.delta", fmt.Sprintf(`"output_index":%d,"item_id":"ctc_%d","delta":%q`, index, index, input)))
		sb.WriteString(grokSSEFrame("response.custom_tool_call_input.done", fmt.Sprintf(`"output_index":%d,"item_id":"ctc_%d","input":%q`, index, index, input)))
		sb.WriteString(grokSSEFrame("response.output_item.done", fmt.Sprintf(`"output_index":%d,"item":%s`, index, item)))
	}
	textIndex := len(items)
	sb.WriteString(grokSSEFrame("response.output_text.delta", fmt.Sprintf(`"output_index":%d,"delta":"found it"`, textIndex)))
	sb.WriteString(grokSSEFrame("response.output_item.done", fmt.Sprintf(`"output_index":%d,"item":%s`, textIndex, grokMessageItemJSON)))
	sb.WriteString(grokSSEFrame("response.completed", `"response":{"id":"resp_1","status":"completed","output":`+terminalOutput+`,"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`))
	return sb.String()
}

type grokSearchForwardOutcome struct {
	body        string
	searchCount int
}

func forwardGrokCustomToolCallStream(t *testing.T, path string, streaming bool, account *Account, sse string) grokSearchForwardOutcome {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	model := "grok-4.7"
	if !account.IsGrok() {
		model = "gpt-5.5"
	}
	body := []byte(fmt.Sprintf(`{"model":%q,"stream":%t,"messages":[{"role":"user","content":"hi"}]}`, model, streaming))
	if path == "/v1/messages" {
		body, _ = sjson.SetBytes(body, "max_tokens", 32)
	}
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("api_key", &APIKey{ID: 88000})
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}}
	var svc *OpenAIGatewayService
	if account.IsGrok() {
		svc = grokAuditOAuthService(upstream, account)
	} else {
		svc = &OpenAIGatewayService{httpUpstream: upstream, cfg: rawChatCompletionsTestConfig()}
	}
	var result *OpenAIForwardResult
	var err error
	if path == "/v1/chat/completions" {
		result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	} else {
		result, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
	}
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return grokSearchForwardOutcome{body: rec.Body.String(), searchCount: result.SearchCount}
}

func TestGrokServerSideSearchCustomToolCallIsNotAClientToolCall(t *testing.T) {
	// A Grok API-key account sends /v1/chat/completions to xAI as Chat Completions, not
	// through Responses, so only /v1/messages applies to it.
	accounts := map[string]*Account{
		"oauth":  healthyGrokOAuthGatewayTestAccount(4242, "access-token"),
		"apikey": grokAuditAPIKeyAccount(4243),
	}
	terminals := map[string]string{
		"empty terminal":          `[]`,
		"terminal with the items": `[` + grokXSearchItemJSON + `,` + grokMessageItemJSON + `]`,
	}
	for accountName, account := range accounts {
		for terminalName, terminalOutput := range terminals {
			for _, path := range []string{"/v1/messages", "/v1/chat/completions"} {
				if path == "/v1/chat/completions" && account.Type == AccountTypeAPIKey {
					continue
				}
				for _, streaming := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s %s %s stream=%v", accountName, terminalName, path, streaming), func(t *testing.T) {
						sse := grokCustomToolCallStream([]string{grokXSearchItemJSON}, terminalOutput)
						got := forwardGrokCustomToolCallStream(t, path, streaming, account, sse)

						require.Equal(t, 1, got.searchCount)
						require.NotContains(t, got.body, "tool_use", got.body)
						require.NotContains(t, got.body, "tool_calls", got.body)
						require.NotContains(t, got.body, "x_keyword_search", got.body)
						require.Contains(t, got.body, "found it", got.body)
						if path == "/v1/messages" {
							require.Contains(t, got.body, `"stop_reason":"end_turn"`, got.body)
						} else {
							require.Contains(t, got.body, `"finish_reason":"stop"`, got.body)
						}
					})
				}
			}
		}
	}
}

func TestGrokServerSideSearchCountsEveryPrefixAndKeepsClientTools(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(4242, "access-token")
	items := []string{grokXSearchItemJSON, grokASearchItemJSON, grokClientToolJSON}
	for _, path := range []string{"/v1/messages", "/v1/chat/completions"} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s stream=%v", path, streaming), func(t *testing.T) {
				got := forwardGrokCustomToolCallStream(t, path, streaming, account, grokCustomToolCallStream(items, `[]`))

				require.Equal(t, 2, got.searchCount, "xs_ and as_ are searches, call_ is not")
				require.NotContains(t, got.body, "x_keyword_search", got.body)
				require.NotContains(t, got.body, "attachment_search", got.body)
				require.Contains(t, got.body, "exec", "the client-declared custom tool is still a tool call")
				if path == "/v1/messages" {
					require.Contains(t, got.body, "tool_use", got.body)
				} else {
					require.Contains(t, got.body, "tool_calls", got.body)
				}
			})
		}
	}
}

// The xs_/as_ rule is xAI's. The same call_id on an OpenAI account is an ordinary custom tool
// call: it is emitted as a tool call and is not a Grok search to bill.
func TestServerSideSearchRuleAppliesOnlyToGrokAccounts(t *testing.T) {
	openAIAccounts := map[string]*Account{
		"openai oauth": {ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
			Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"}},
		"openai apikey": rawChatCompletionsTestAccount(),
	}
	t.Run("grok oauth, same stream", func(t *testing.T) {
		sse := grokCustomToolCallStream([]string{grokXSearchItemJSON, grokASearchItemJSON}, `[]`)
		got := forwardGrokCustomToolCallStream(t, "/v1/messages", true, healthyGrokOAuthGatewayTestAccount(4242, "access-token"), sse)
		require.Equal(t, 2, got.searchCount)
		require.NotContains(t, got.body, "tool_use", got.body)
	})
	for accountName, account := range openAIAccounts {
		for _, path := range []string{"/v1/messages", "/v1/chat/completions"} {
			for _, terminalOutput := range []string{`[]`, `[` + grokXSearchItemJSON + `,` + grokMessageItemJSON + `]`} {
				for _, streaming := range []bool{false, true} {
					if !streaming && terminalOutput != `[]` {
						continue // the nonstreaming converters never emit a custom_tool_call from a terminal output
					}
					t.Run(fmt.Sprintf("%s %s terminal=%s stream=%v", accountName, path, terminalOutput[:2], streaming), func(t *testing.T) {
						sse := grokCustomToolCallStream([]string{grokXSearchItemJSON, grokASearchItemJSON}, terminalOutput)
						got := forwardGrokCustomToolCallStream(t, path, streaming, account, sse)

						require.Equal(t, 0, got.searchCount)
						require.Contains(t, got.body, "x_keyword_search", got.body)
						if path == "/v1/messages" {
							require.Contains(t, got.body, "tool_use", got.body)
						} else {
							require.Contains(t, got.body, "tool_calls", got.body)
						}
					})
				}
			}
		}
	}
}

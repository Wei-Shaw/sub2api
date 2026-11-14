//go:build unit

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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const bufferedDoneReasoning = `{"type":"reasoning","id":"rs_buffered","summary":[{"type":"summary_text","text":"plan"}],"encrypted_content":"signed-thinking"}`
const bufferedDoneSearch = `{"type":"web_search_call","id":"ws_buffered","status":"completed","action":{"type":"search","query":"release notes"}}`
const bufferedDoneMessage = `{"type":"message","id":"msg_buffered","role":"assistant","status":"completed","content":[{"type":"output_text","text":"found"}]}`

func bufferedDoneItemsStream(terminalType, terminalOutput string, eventTypesOnly, unterminated bool) string {
	frames := []string{
		`{"type":"response.created","response":{"id":"resp_buffered_items","model":"gpt-5.5","status":"in_progress","output":[]}}`,
		`{"type":"response.output_item.done","output_index":2,"item":` + bufferedDoneMessage + `}`,
		`{"type":"response.output_item.done","output_index":0,"item":` + bufferedDoneReasoning + `}`,
		`{"type":"response.output_item.done","output_index":1,"item":` + bufferedDoneSearch + `}`,
		`{"type":"response.output_item.done","output_index":1,"item":` + bufferedDoneSearch + `}`,
		`{"type":"` + terminalType + `","response":{"id":"resp_buffered_items","model":"gpt-5.5","status":"completed","output":` + terminalOutput + `,"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5,"input_tokens_details":{"cached_tokens":1}}}}`,
	}
	var stream strings.Builder
	for _, frame := range frames {
		if eventTypesOnly {
			typ := gjson.Get(frame, "type").String()
			// The parser supplies the effective type when the JSON omits it.
			var body map[string]json.RawMessage
			_ = json.Unmarshal([]byte(frame), &body)
			delete(body, "type")
			raw, _ := json.Marshal(body)
			stream.WriteString("event: " + typ + "\n")
			frame = string(raw)
		}
		stream.WriteString("data: " + frame + "\n\n")
	}
	result := stream.String()
	if unterminated {
		result = strings.TrimSuffix(result, "\n\n")
	}
	return result
}

func TestReadOpenAICompatBufferedTerminal_RetainsDoneItems(t *testing.T) {
	for _, tc := range []struct {
		name, terminal               string
		eventTypesOnly, unterminated bool
	}{
		{"completed", "response.completed", false, false},
		{"done_alias", "response.done", false, false},
		{"event_types_only", "response.completed", true, false},
		{"final_frame_at_eof", "response.completed", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			response := &http.Response{Body: io.NopCloser(strings.NewReader(bufferedDoneItemsStream(tc.terminal, "[]", tc.eventTypesOnly, tc.unterminated)))}
			defer response.Body.Close()
			final, usage, acc, _, err := (&OpenAIGatewayService{}).readOpenAICompatBufferedTerminal(response, c, nil, "test", "request")
			require.NoError(t, err)
			require.NotNil(t, final)
			acc.SupplementResponseOutput(final)
			require.Len(t, final.Output, 3, "completed items must survive an empty terminal output, in output_index order and without duplicates")
			require.Equal(t, "reasoning", final.Output[0].Type)
			require.Equal(t, "signed-thinking", final.Output[0].EncryptedContent)
			require.Equal(t, "web_search_call", final.Output[1].Type)
			require.Equal(t, "ws_buffered", final.Output[1].ID)
			require.NotNil(t, final.Output[1].Action)
			require.Equal(t, "release notes", final.Output[1].Action.Query)
			require.Equal(t, "message", final.Output[2].Type)
			require.Equal(t, "found", final.Output[2].Content[0].Text)
			require.Equal(t, 3, usage.InputTokens)
			require.Equal(t, 2, usage.OutputTokens)
			require.Equal(t, 1, usage.CacheReadInputTokens)
		})
	}
}

func TestReadOpenAICompatBufferedTerminal_KeepsNonemptyTerminalOutput(t *testing.T) {
	response := &http.Response{Body: io.NopCloser(strings.NewReader(bufferedDoneItemsStream("response.completed", `[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"terminal wins"}]}]`, false, false)))}
	defer response.Body.Close()
	final, _, _, _, err := (&OpenAIGatewayService{}).readOpenAICompatBufferedTerminal(response, nil, nil, "test", "request")
	require.NoError(t, err)
	require.NotNil(t, final)
	require.Len(t, final.Output, 1)
	require.Equal(t, "terminal wins", final.Output[0].Content[0].Text)
}

func TestForwardAsAnthropic_BufferedDoneItemsPreserveWebSearch(t *testing.T) {
	for _, apiKey := range []bool{false, true} {
		name := "oauth"
		account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}}
		if apiKey {
			name = "api_key"
			account = rawChatCompletionsTestAccount()
		}
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.5","max_tokens":64,"stream":false,"tools":[{"type":"web_search_20250305","name":"web_search"}],"messages":[{"role":"user","content":"find release notes"}]}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(bufferedDoneItemsStream("response.completed", "[]", false, false)))}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.5")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, rec.Code)
			blocks := gjson.Get(rec.Body.String(), "content").Array()
			var types []string
			for _, block := range blocks {
				types = append(types, block.Get("type").String())
			}
			require.Equal(t, []string{"thinking", "server_tool_use", "web_search_tool_result", "text"}, types)
			require.Equal(t, "signed-thinking", blocks[0].Get("signature").String())
			require.Equal(t, "srvtoolu_ws_buffered", blocks[1].Get("id").String())
			require.Equal(t, "release notes", blocks[1].Get("input.query").String())
			require.Equal(t, blocks[1].Get("id").String(), blocks[2].Get("tool_use_id").String())
			require.Equal(t, "found", blocks[3].Get("text").String())
			require.Equal(t, 3, result.Usage.InputTokens)
			require.Equal(t, 1, result.Usage.CacheReadInputTokens)
		})
	}
}

func TestForwardAsChatCompletions_BufferedDoneItemsPreserveText(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"messages":[{"role":"user","content":"find release notes"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(bufferedDoneItemsStream("response.completed", "[]", false, false)))}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}}
	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.5")
	require.NoError(t, err)
	require.Equal(t, "found", gjson.Get(rec.Body.String(), "choices.0.message.content").String())
	require.Equal(t, "plan", gjson.Get(rec.Body.String(), "choices.0.message.reasoning_content").String())
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
}

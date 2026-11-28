//go:build unit

package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNativeWebSearchBufferedHostedOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, bridge := range []string{"messages", "chat"} {
		for _, terminalOnly := range []bool{false, true} {
			for _, finishAtEOF := range []bool{false, true} {
				name := bridge + "/stream_items"
				if terminalOnly {
					name = bridge + "/terminal_items"
				}
				if finishAtEOF {
					name += "/eof"
				}
				t.Run(name, func(t *testing.T) {
					searchItem := `{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"q"}}`
					messageItem := `{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"found it"}]}`
					events := []string{
						bufferedHostedItemsCreated,
						`{"type":"response.output_item.added","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"in_progress"}}`,
					}
					terminalOutput := "[]"
					if terminalOnly {
						terminalOutput = "[" + searchItem + "," + messageItem + "]"
					} else {
						events = append(events,
							`{"type":"response.web_search_call.completed","item_id":"ws_1"}`,
							`{"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","status":"completed","action":{"type":"search","query":"q"}}}`,
							`{"type":"response.output_item.done","output_index":1,"item":`+messageItem+`}`,
							`{"type":"response.output_item.added","output_index":2,"item":{"type":"web_search_call","id":"ws_pending","status":"in_progress"}}`,
						)
					}
					events = append(events, `{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","status":"completed","output":`+terminalOutput+`,"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`)
					body := bufferedItemsSSE(events...)
					if finishAtEOF {
						body = strings.TrimRight(body, "\n")
					}
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/"+bridge, nil)
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
					response := bufferedHostedItemsSSEResponse(body)
					account := bufferedHostedItemsOAuthAccount()
					var result *OpenAIForwardResult
					var err error
					if bridge == "messages" {
						result, err = svc.handleAnthropicBufferedStreamingResponse(response, ctx, account, "gpt-5.5", "gpt-5.5", "gpt-5.5", time.Now())
					} else {
						result, err = svc.handleChatBufferedStreamingResponse(response, ctx, account, "gpt-5.5", "gpt-5.5", "gpt-5.5", time.Now())
					}
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, 1, result.NativeWebSearchCalls)
					require.Zero(t, result.SearchCount)
					require.Zero(t, result.WebSearchCalls)
					require.Equal(t, 5, result.Usage.InputTokens)
					require.Equal(t, 2, result.Usage.OutputTokens)
					require.Equal(t, http.StatusOK, recorder.Code)
					output := recorder.Body.String()
					if bridge == "messages" {
						content := gjson.Get(output, "content").Array()
						require.Len(t, content, 3, output)
						require.Equal(t, "server_tool_use", content[0].Get("type").String())
						require.Equal(t, "web_search", content[0].Get("name").String())
						require.Equal(t, "web_search_tool_result", content[1].Get("type").String())
						require.Equal(t, "found it", content[2].Get("text").String())
					} else {
						require.Equal(t, "found it", gjson.Get(output, "choices.0.message.content").String(), output)
					}
				})
			}
		}
	}
}

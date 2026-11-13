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

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestFilterCodexInput_AdditionalToolsDeclarationID(t *testing.T) {
	for _, preserveReferences := range []bool{false, true} {
		t.Run(fmt.Sprintf("references=%t", preserveReferences), func(t *testing.T) {
			input := []any{
				map[string]any{
					"type": "additional_tools", "id": "at_discovery", "role": "developer",
					"tools": []any{map[string]any{"type": "tool_search", "execution": "client"}},
				},
				map[string]any{"type": "message", "id": "msg_history", "role": "user", "content": "find a tool"},
				map[string]any{"type": "reasoning", "id": "rs_history", "encrypted_content": "sealed", "summary": []any{}},
				map[string]any{"type": "item_reference", "id": "msg_reference"},
			}
			original, err := json.Marshal(input)
			require.NoError(t, err)

			filtered := filterCodexInput(input, preserveReferences)
			encoded, err := json.Marshal(filtered)
			require.NoError(t, err)
			require.Equal(t, "at_discovery", gjson.GetBytes(encoded, "0.id").String())
			require.Equal(t, "tool_search", gjson.GetBytes(encoded, "0.tools.0.type").String())
			require.Equal(t, preserveReferences, gjson.GetBytes(encoded, "1.id").Exists())
			require.False(t, gjson.GetBytes(encoded, "2.id").Exists())
			require.Equal(t, "sealed", gjson.GetBytes(encoded, "2.encrypted_content").String())
			require.Equal(t, preserveReferences, gjson.GetBytes(encoded, "3.id").Exists())
			after, err := json.Marshal(input)
			require.NoError(t, err)
			require.JSONEq(t, string(original), string(after), "filtering must not mutate the caller's input")
		})
	}
}

func TestOpenAIGatewayService_OAuthAdditionalToolsDeclarationID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, lite := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("passthrough=%t/lite=%t/stream=%t", passthrough, lite, stream), func(t *testing.T) {
					body := []byte(fmt.Sprintf(`{
						"model":"gpt-6.1-sol","stream":%t,"store":false,"instructions":"test instructions",
						"input":[
							{"type":"additional_tools","id":"at_discovery","role":"developer","tools":[
								{"type":"tool_search","execution":"client"},
								{"type":"namespace","name":"multi_agent_v1","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"},"defer_loading":true}]}
							]},
							{"type":"message","id":"msg_history","role":"user","content":[{"type":"input_text","text":"find spawn_agent"}]},
							{"type":"reasoning","id":"rs_history","encrypted_content":"sealed","summary":[]}
						]
					}`, stream))
					upstreamSSE := strings.Join([]string{
						`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"tool_search_call","id":"tsc_discovery","call_id":"tsc_discovery","execution":"client","arguments":"{\"query\":\"spawn_agent\"}"}}`,
						"",
						`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"tool_search_call","id":"tsc_discovery","call_id":"tsc_discovery","execution":"client","arguments":"{\"query\":\"spawn_agent\"}"}}`,
						"",
						`data: {"type":"response.completed","response":{"id":"resp_discovery","status":"completed","model":"gpt-6.1-sol","output":[{"type":"tool_search_call","id":"tsc_discovery","call_id":"tsc_discovery","execution":"client","arguments":"{\"query\":\"spawn_agent\"}"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
						"", "data: [DONE]", "",
					}, "\n")
					upstream := &httpUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
						Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
					}}
					svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
					account := &Account{
						ID: 8752, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
						Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"},
						Extra:       map[string]any{"openai_passthrough": passthrough},
						Status:      StatusActive, Schedulable: true,
					}
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
					c.Request.Header.Set("User-Agent", "codex_cli_rs/0.167.0")
					if lite {
						c.Request.Header.Set(responsesLiteHeader, "true")
					}

					result, err := svc.Forward(context.Background(), c, account, body)

					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, "https://chatgpt.com/backend-api/codex/responses", upstream.lastReq.URL.String())
					require.Equal(t, "at_discovery", gjson.GetBytes(upstream.lastBody, "input.0.id").String())
					require.JSONEq(t, gjson.GetBytes(body, "input.0").Raw, gjson.GetBytes(upstream.lastBody, "input.0").Raw)
					require.Equal(t, passthrough, gjson.GetBytes(upstream.lastBody, "input.1.id").Exists())
					require.Equal(t, passthrough, gjson.GetBytes(upstream.lastBody, "input.2.id").Exists())
					require.Equal(t, "sealed", gjson.GetBytes(upstream.lastBody, "input.2.encrypted_content").String())
					require.Contains(t, recorder.Body.String(), `"type":"tool_search_call"`)
					require.Contains(t, recorder.Body.String(), `"call_id":"tsc_discovery"`)
					require.NotContains(t, recorder.Body.String(), `"type":"function_call"`)
				})
			}
		}
	}
}

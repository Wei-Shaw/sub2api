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

func TestFilterCodexInput_AdditionalToolsUUID(t *testing.T) {
	const uuidID = "at_00112233-4455-4677-8899-aabbccddeeff"
	const compactID = "at_00112233445546778899aabbccddeeff"
	for _, tc := range []struct {
		name string
		id   any
		want any
	}{
		{"hyphenated", uuidID, compactID},
		{"uppercase", "at_00112233-4455-4677-8899-AABBCCDDEEFF", "at_00112233445546778899AABBCCDDEEFF"},
		{"compact", compactID, compactID},
		{"opaque", "at_discovery", "at_discovery"},
		{"wrong_prefix", "xx_00112233-4455-4677-8899-aabbccddeeff", "xx_00112233-4455-4677-8899-aabbccddeeff"},
		{"non_hex", "at_z0112233-4455-4677-8899-aabbccddeeff", "at_z0112233-4455-4677-8899-aabbccddeeff"},
		{"misplaced_hyphen", "at_0011223-34455-4677-8899-aabbccddeeff", "at_0011223-34455-4677-8899-aabbccddeeff"},
		{"extra_hyphens", "at_0011223--4455-4677-8899-aabbccddeef-", "at_0011223--4455-4677-8899-aabbccddeef-"},
		{"short", "at_00112233-4455-4677-8899-aabbccddee", "at_00112233-4455-4677-8899-aabbccddee"},
		{"whitespace", " " + uuidID, " " + uuidID},
		{"numeric", 42, 42},
		{"null", nil, nil},
	} {
		for _, preserveReferences := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/references=%t", tc.name, preserveReferences), func(t *testing.T) {
				declaration := map[string]any{
					"type": "additional_tools", "id": tc.id, "role": "developer",
					"tools": []any{map[string]any{"type": "tool_search", "execution": "client"}},
				}
				reference := map[string]any{"type": "item_reference", "id": tc.id}
				input := []any{declaration, reference}
				filtered := filterCodexInput(input, preserveReferences)
				want := map[string]any{
					"type": "additional_tools", "id": tc.want, "role": "developer", "tools": declaration["tools"],
				}
				require.Equal(t, want, filtered[0])
				if preserveReferences {
					require.Equal(t, map[string]any{"type": "item_reference", "id": tc.want}, filtered[1])
				} else {
					require.Len(t, filtered, 1)
				}
				require.Equal(t, tc.id, declaration["id"], "normalization must not mutate the declaration")
				require.Equal(t, tc.id, reference["id"], "normalization must not mutate references")
				require.Equal(t, filtered, filterCodexInput(filtered, preserveReferences), "normalization must be idempotent")
			})
		}
	}

	t.Run("only_declared_tools", func(t *testing.T) {
		input := []any{
			map[string]any{"type": "additional_tools", "tools": []any{}},
			map[string]any{"type": "message", "id": "msg_00112233-4455-4677-8899-aabbccddeeff", "role": "user", "content": "hello"},
			map[string]any{"type": "item_reference", "id": uuidID},
			map[string]any{"type": "item_reference", "id": "at_11112233-4455-4677-8899-aabbccddeeff"},
		}
		require.Equal(t, input, filterCodexInput(input, true), "do not invent IDs or rewrite unrelated history/references")
	})
}

func TestOpenAIResponsesCompatibility_AdditionalToolsUUID(t *testing.T) {
	const uuidID = "at_00112233-4455-4677-8899-aabbccddeeff"
	const compactID = "at_00112233445546778899aabbccddeeff"
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		t.Run(accountType, func(t *testing.T) {
			body := []byte(`{"model":"gpt-6.1-sol","marker":"\u0061","number":9007199254740993,"input":[` +
				`{"type":"item_reference","id":"` + uuidID + `"},` +
				`{"type":"additional_tools","id":"` + uuidID + `","role":"developer","tools":[{"type":"tool_search","execution":"client"}]},` +
				`{"type":"future_item","id":"` + uuidID + `"},` +
				`{"type":"additional_tools","id":"at_discovery","tools":[]},` +
				`{"type":"additional_tools","id":"at_z0112233-4455-4677-8899-aabbccddeeff","tools":[]},` +
				`{"type":"message","id":"msg_test","role":"user","content":"` + uuidID + `"}]}`)
			original := bytes.Clone(body)
			account := &Account{Platform: PlatformOpenAI, Type: accountType}
			normalized, changed, err := normalizeOpenAIResponsesCompatibilityBodyWithOptions(body, account, openAIResponsesCompatibilityOptions{})
			require.NoError(t, err)
			require.Equal(t, accountType == AccountTypeOAuth, changed)
			wantID := uuidID
			if accountType == AccountTypeOAuth {
				wantID = compactID
			}
			require.Equal(t, wantID, gjson.GetBytes(normalized, "input.0.id").String(), "references can precede declarations")
			require.Equal(t, wantID, gjson.GetBytes(normalized, "input.1.id").String())
			for _, index := range []int{2, 3, 4, 5} {
				path := fmt.Sprintf("input.%d", index)
				require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(normalized, path).Raw)
			}
			require.Equal(t, `"\u0061"`, gjson.GetBytes(normalized, "marker").Raw)
			require.Equal(t, "9007199254740993", gjson.GetBytes(normalized, "number").Raw)
			require.Equal(t, original, body)
			again, changed, err := normalizeOpenAIResponsesCompatibilityBodyWithOptions(normalized, account, openAIResponsesCompatibilityOptions{})
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, normalized, again)
		})
	}
	t.Run("undeclared_reference", func(t *testing.T) {
		body := []byte(`{"model":"gpt-6.1-sol","input":[{"type":"item_reference","id":"` + uuidID + `"}]}`)
		normalized, changed, err := normalizeOpenAIResponsesCompatibilityBodyWithOptions(body, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, openAIResponsesCompatibilityOptions{})
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, normalized)
	})
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
							{"type":"additional_tools","id":"at_00112233-4455-4677-8899-aabbccddeeff","role":"developer","tools":[
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
					wantID := "at_00112233445546778899aabbccddeeff"
					require.Equal(t, wantID, gjson.GetBytes(upstream.lastBody, "input.0.id").String())
					require.Equal(t, "at_00112233-4455-4677-8899-aabbccddeeff", gjson.GetBytes(body, "input.0.id").String())
					require.JSONEq(t, gjson.GetBytes(body, "input.0.tools").Raw, gjson.GetBytes(upstream.lastBody, "input.0.tools").Raw)
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

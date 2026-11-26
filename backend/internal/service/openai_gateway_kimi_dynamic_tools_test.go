//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestKimiChatDynamicToolsAcrossProtocols(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"kimi-k3.1-pro",
		"tools":[{"type":"function","function":{"name":"Lookup","parameters":{"type":"object"}}}],
		"tool_choice":"required",
		"messages":[
			{"role":"system","content":"Answer briefly."},
			{"role":"user","content":"Calculate 23 * 47.","tools":[{"type":"function","function":{"name":"IgnoreMe"}}]},
			{"role":"system","tools":[{"type":"function","function":{"name":"Calculator","parameters":{"type":"object","properties":{"expr":{"type":"string"}},"required":["expr"]}}}]}
		]
	}`)
	for _, protocol := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
		t.Run(protocol, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			account := adaptiveProtocolTestAccount(PlatformKimi, map[string]any{protocol: "http://kimi.example"})
			account.Credentials["api_protocol"] = protocol
			c := adaptiveProtocolTestContext("/v1/chat/completions", body)
			_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			require.Error(t, err)
			require.NotNil(t, upstream.lastReq)
			out := gjson.ParseBytes(upstream.lastBody)
			schema := gjson.GetBytes(body, "messages.2.tools.0.function.parameters").Raw
			switch protocol {
			case APIProtocolChatCompletions:
				require.JSONEq(t, string(body), string(upstream.lastBody))
			case APIProtocolResponses:
				require.JSONEq(t, `["Lookup","Calculator"]`, out.Get("tools.#.name").Raw)
				require.JSONEq(t, schema, out.Get("tools.1.parameters").Raw)
				require.Equal(t, "required", out.Get("tool_choice").String())
				require.Len(t, out.Get("input").Array(), 2)
			case APIProtocolAnthropic:
				require.JSONEq(t, `["Lookup","Calculator"]`, out.Get("tools.#.name").Raw)
				require.JSONEq(t, schema, out.Get("tools.1.input_schema").Raw)
				require.Equal(t, "any", out.Get("tool_choice.type").String())
				require.Equal(t, "Answer briefly.", out.Get("system").String())
			}
		})
	}
}

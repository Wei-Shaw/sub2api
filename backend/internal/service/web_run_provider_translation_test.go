//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func codexWebRunResponsesBody(model string) []byte {
	return []byte(`{
		"model":` + mustMarshalString(model) + `,
		"stream":true,
		"tools":[{
			"type":"namespace",
			"name":"web",
			"description":"Search and open current web sources.",
			"tools":[{
				"type":"function",
				"name":"run",
				"description":"Execute a standalone web search request.",
				"parameters":{"type":"object","properties":{"search_query":{"type":"array"}}}
			}]
		}],
		"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Search the web"}]}]
	}`)
}

func mustMarshalString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func TestGrokResponsesPreservesCodexWebRunAsFunctionTool(t *testing.T) {
	body := codexWebRunResponsesBody("grok-4.6")
	patched, mapping, err := patchGrokResponsesBodyWithClientTools(body, "grok-4.6")
	require.NoError(t, err)
	require.Equal(t,
		apicompat.ResponsesNamespaceName{Namespace: "web", Name: "run"},
		mapping.NamespaceTools["web__run"],
	)
	require.Equal(t, "function", gjson.GetBytes(patched, `tools.#(name=="web__run").type`).String())
	require.False(t, gjson.GetBytes(patched, `tools.#(type=="namespace")`).Exists())
}

func TestChatFallbackPreservesCodexWebRunAsFunctionTool(t *testing.T) {
	var request apicompat.ResponsesRequest
	require.NoError(t, json.Unmarshal(codexWebRunResponsesBody("glm-5.2"), &request))

	effective, err := apicompat.EffectiveResponsesTools(&request)
	require.NoError(t, err)
	require.Equal(t,
		apicompat.NamespacedToolName{Namespace: "web", Name: "run"},
		apicompat.NamespaceToolNames(effective)["web__run"],
	)

	chatRequest, err := apicompat.ResponsesToChatCompletionsRequest(&request)
	require.NoError(t, err)
	require.Len(t, chatRequest.Tools, 1)
	require.NotNil(t, chatRequest.Tools[0].Function)
	require.Equal(t, "web__run", chatRequest.Tools[0].Function.Name)
}

func TestGeminiResponsesPreservesCodexWebRunAsFunctionDeclaration(t *testing.T) {
	adapted, mapping, err := adaptResponsesClientToolsForAnthropic(codexWebRunResponsesBody("gemini-3.7-flash"))
	require.NoError(t, err)
	require.Equal(t,
		apicompat.ResponsesNamespaceName{Namespace: "web", Name: "run"},
		mapping.NamespaceTools["web__run"],
	)

	var request apicompat.ResponsesRequest
	require.NoError(t, json.Unmarshal(adapted, &request))
	anthropicRequest, err := apicompat.ResponsesToAnthropicRequest(&request)
	require.NoError(t, err)

	encoded, err := json.Marshal(anthropicRequest.Tools)
	require.NoError(t, err)
	var genericTools any
	require.NoError(t, json.Unmarshal(encoded, &genericTools))
	geminiTools := convertClaudeToolsToGeminiTools(genericTools)
	require.Len(t, geminiTools, 1)

	encodedGemini, err := json.Marshal(geminiTools)
	require.NoError(t, err)
	require.Equal(t, "web__run", gjson.GetBytes(encodedGemini, "0.functionDeclarations.0.name").String())
}

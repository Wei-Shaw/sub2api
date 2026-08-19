package service

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIsCursorClientRequestUsesOnlyClientIdentity(t *testing.T) {
	newContext := func(userAgent, originator string, cursorHeader bool) *gin.Context {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		ctx.Request.Header.Set("User-Agent", userAgent)
		ctx.Request.Header.Set("Originator", originator)
		if cursorHeader {
			ctx.Request.Header.Set("X-Cursor-Client-Version", "2.4.0")
		}
		return ctx
	}

	require.True(t, isCursorClientRequest(newContext("Cursor/2.4.0", "", false)))
	require.True(t, isCursorClientRequest(newContext("", "cursor", false)))
	require.True(t, isCursorClientRequest(newContext("OpenAI", "", true)))
	require.False(t, isCursorClientRequest(newContext("OpenAI", "", false)))
}

func TestAdaptRawChatCompletionsClientToolsLowersCursorApplyPatch(t *testing.T) {
	body := []byte(`{
		"model":"gpt-test",
		"messages":[{"role":"user","content":"edit a file"}],
		"tools":[
			{"type":"function","name":"Shell","description":"Run a command","parameters":{"type":"object"}},
			{"type":"custom","name":"ApplyPatch","description":"Apply a unified diff","format":{"type":"grammar"}}
		]
	}`)

	adapted, changed, err := adaptRawChatCompletionsClientTools(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "function", gjson.GetBytes(adapted, "tools.1.type").String())
	require.Equal(t, "ApplyPatch", gjson.GetBytes(adapted, "tools.1.name").String())
	require.Equal(t, "string", gjson.GetBytes(adapted, "tools.1.parameters.properties.input.type").String())
	require.False(t, gjson.GetBytes(adapted, "tools.1.format").Exists())
	// Existing flat Cursor function declarations must survive unchanged.
	require.Equal(t, "Shell", gjson.GetBytes(adapted, "tools.0.name").String())
}

func TestExtractFlatCursorCustomToolsAppendsWithoutReplacingChatFunctions(t *testing.T) {
	body := []byte(`{
		"model":"gpt-test",
		"messages":[{"role":"user","content":"edit a file"}],
		"tools":[
			{"type":"function","function":{"name":"Shell","description":"Run a command","parameters":{"type":"object"}}},
			{"type":"custom","name":"ApplyPatch","description":"Apply a unified diff","format":{"type":"grammar"}}
		]
	}`)

	tools, found, err := extractFlatCursorCustomToolsFromChatBody(body)
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, tools, 1)
	require.Equal(t, "custom", tools[0].Type)
	require.Equal(t, "ApplyPatch", tools[0].Name)
	encodedCustomTool, err := json.Marshal(tools[0])
	require.NoError(t, err)
	require.Equal(t, "grammar", gjson.GetBytes(encodedCustomTool, "format.type").String())

	var chatReq apicompat.ChatCompletionsRequest
	require.NoError(t, json.Unmarshal(body, &chatReq))
	responsesReq, err := apicompat.ChatCompletionsToResponses(&chatReq)
	require.NoError(t, err)
	// The typed Chat parser converts the standard nested function tool. The
	// gateway must append, rather than replace it with the flat custom tool.
	require.Len(t, responsesReq.Tools, 1)
	require.Equal(t, "Shell", responsesReq.Tools[0].Name)
	responsesReq.Tools = append(responsesReq.Tools, tools...)
	require.Len(t, responsesReq.Tools, 2)
	require.Equal(t, "ApplyPatch", responsesReq.Tools[1].Name)
}

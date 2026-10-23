package service

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func summaryRequestAccount(contextWindow int64) *Account {
	account := &Account{
		ID:       901,
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{OpenAICompactSummaryEnabledExtraKey: true},
	}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"deepseek-chat": {ID: "deepseek-chat", ContextWindow: contextWindow, MaxOutputTokens: 16384},
	}})
	return account
}

func TestBuildOpenAICompactionSummaryRequestBodyRemovesToolsAndTrigger(t *testing.T) {
	body := []byte(`{"model":"deepseek-chat","stream":true,"truncation":"auto","tools":[{"type":"function","name":"shell"}],"tool_choice":"auto","parallel_tool_calls":true,"input":[{"type":"message","role":"user","content":"inspect the repository"},{"type":"compaction_trigger"}]}`)
	summaryBody, err := buildOpenAICompactionSummaryRequestBody(body)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(summaryBody, "tools").Exists())
	require.False(t, gjson.GetBytes(summaryBody, "tool_choice").Exists())
	require.False(t, gjson.GetBytes(summaryBody, "parallel_tool_calls").Exists())
	require.False(t, gjson.GetBytes(summaryBody, "truncation").Exists())
	require.False(t, gjson.GetBytes(summaryBody, `input.#(type=="compaction_trigger")`).Exists())
	require.Equal(t, gjson.False, gjson.GetBytes(summaryBody, "stream").Type)
	require.Equal(t, int64(8192), gjson.GetBytes(summaryBody, "max_output_tokens").Int())
	require.Contains(t, gjson.GetBytes(summaryBody, "input.1.content.0.text").String(), "produce a faithful, concise summary")
}

func TestBuildOpenAICompactionSummaryRequestBodyRejectsHiddenOrUnrecoverableHistory(t *testing.T) {
	cases := []string{
		`{"model":"deepseek-chat","previous_response_id":"resp_hidden","input":[{"type":"message","role":"user","content":"incremental"}]}`,
		`{"model":"deepseek-chat","input":[{"type":"compaction","encrypted_content":"native"},{"type":"compaction_trigger"}]}`,
		`{"model":"deepseek-chat","input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"https://example.test/a.png"}]},{"type":"compaction_trigger"}]}`,
	}
	for _, testBody := range cases {
		_, err := buildOpenAICompactionSummaryRequestBody([]byte(testBody))
		require.Error(t, err, testBody)
	}
}

func TestValidateOpenAICompactionSummaryContext(t *testing.T) {
	account := summaryRequestAccount(10_000)
	body := []byte(`{"model":"deepseek-chat","instructions":"continue the task","input":[{"type":"message","role":"user","content":"small history"}],"max_output_tokens":8192}`)
	require.NoError(t, validateOpenAICompactionSummaryContext(account, body, "deepseek-chat"))

	largeBody := []byte(`{"model":"deepseek-chat","input":[{"type":"message","role":"user","content":"` + strings.Repeat("x", 120_000) + `"}],"max_output_tokens":8192}`)
	require.Error(t, validateOpenAICompactionSummaryContext(account, largeBody, "deepseek-chat"))

	account.Extra = map[string]any{OpenAICompactSummaryEnabledExtraKey: true}
	require.Error(t, validateOpenAICompactionSummaryContext(account, body, "deepseek-chat"), "unknown context window must fail closed")
}

func TestValidateOpenAICompactionSummaryContextRequiresFinalMappedModelMetadata(t *testing.T) {
	account := summaryRequestAccount(128_000)
	account.Credentials = map[string]any{"model_mapping": map[string]any{"public-summary": "mapped-summary"}}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"public-summary": {ID: "public-summary", ContextWindow: 128_000},
	}})
	body := []byte(`{"model":"public-summary","input":[{"type":"message","role":"user","content":"history"}],"max_output_tokens":8192}`)

	require.Error(t, validateOpenAICompactionSummaryContext(account, body, "mapped-summary"), "the original alias must not satisfy the final mapped model window check")
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"mapped-summary": {ID: "mapped-summary", ContextWindow: 128_000},
	}})
	require.NoError(t, validateOpenAICompactionSummaryContext(account, body, "mapped-summary"))
}

func TestOpenAICompactionSummaryExecutionMarkerHasSummaryMode(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(openAINativeCompactionV2Key, true)
	markOpenAICompactionSummaryExecution(c, openAICompactionKindV2)
	execution, ok := openAICompactionSummaryExecutionFromContext(c)
	require.True(t, ok)
	require.Equal(t, openAICompactionKindV2, execution.protocol)
	require.Equal(t, openAICompactionExecutionSummary, execution.mode)

	svc := &OpenAIGatewayService{}
	account := summaryRequestAccount(128_000)
	require.Equal(t, openAICompactionExecutionSummary, svc.openAICompactionExecutionForAccount(c, account, "deepseek-chat", APIProtocolResponses))
}

func TestBeginOpenAICompactionAttemptRestoresIngressKind(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	MarkOpenAINativeCompactionV2(c)
	markOpenAICompactionSummaryExecution(c, openAICompactionKindV2)
	require.True(t, IsOpenAICompactionSummaryExecution(c))
	beginOpenAICompactionAttempt(c)
	require.False(t, IsOpenAICompactionSummaryExecution(c))
	require.True(t, IsOpenAINativeCompactionV2(c))

	markOpenAICompactionSummaryExecution(c, openAICompactionKindLegacy)
	beginOpenAICompactionAttempt(c)
	require.False(t, IsOpenAICompactionSummaryExecution(c))
	require.True(t, IsOpenAINativeCompactionV2(c))
}

func TestFinalizeOpenAICompactionSummaryResponseRequiresCompletedText(t *testing.T) {
	key := testOpenAICompactionSummaryKey()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set("api_key", &APIKey{UserID: 42})
	markOpenAICompactionSummaryExecution(c, openAICompactionKindV2)
	svc := &OpenAIGatewayService{compactionSummaryKey: append([]byte(nil), key...)}

	completed := []byte(`{"id":"resp_1","status":"completed","usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"summary text"}]}]}`)
	sealed, err := svc.finalizeOpenAICompactionSummaryResponse(c, 200, completed)
	require.NoError(t, err)
	require.Equal(t, 1, len(gjson.GetBytes(sealed, "output").Array()))
	require.Equal(t, "compaction", gjson.GetBytes(sealed, "output.0.type").String())
	require.Equal(t, int64(15), gjson.GetBytes(sealed, "usage.total_tokens").Int())
	scope, err := NewOpenAICompactionOwnerScope(42, nil)
	require.NoError(t, err)
	summary, err := OpenOpenAICompactionSummary(key, scope, gjson.GetBytes(sealed, "output.0.encrypted_content").String())
	require.NoError(t, err)
	require.Equal(t, "summary text", summary)

	for _, invalid := range [][]byte{
		[]byte(`{"status":"queued","output":[{"type":"message","content":[{"type":"output_text","text":"x"}]}]}`),
		[]byte(`{"status":"in_progress","output":[{"type":"message","content":[{"type":"output_text","text":"x"}]}]}`),
		[]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"x"}]}]}`),
		[]byte(`{"status":"completed","output":[{"type":"message","content":[]}]}`),
		[]byte(`{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","content":[{"type":"output_text","text":"x"}]}]}`),
	} {
		_, err := svc.finalizeOpenAICompactionSummaryResponse(c, 200, invalid)
		require.ErrorIs(t, err, ErrOpenAICompactionSummaryGeneration)
	}
}

func TestOpenAICompactionSummaryLegacyAndV2WireOutputs(t *testing.T) {
	item, err := marshalOpenAIUpstreamJSON(map[string]any{
		"id": "cmp_test", "type": "compaction", "status": "completed", "encrypted_content": "sealed",
	})
	require.NoError(t, err)
	response, err := marshalOpenAIUpstreamJSON(map[string]any{
		"id": "resp_test", "object": "response", "status": "completed", "output": []any{},
	})
	require.NoError(t, err)
	response, err = sjson.SetRawBytes(response, "output", []byte("["+string(item)+"]"))
	require.NoError(t, err)
	require.True(t, responsesOutputHasCompactionItem(response))

	sse, ok := buildOpenAICompactSSEPayload(response)
	require.True(t, ok)
	text := string(sse)
	require.Contains(t, text, "event: response.output_item.done")
	require.Contains(t, text, `"type":"compaction"`)
	require.Contains(t, text, "event: response.completed")
	require.Equal(t, 1, strings.Count(text, "event: response.output_item.done"))

}

func TestOpenAICompactionSummaryRequiresProtocolTerminalBeforeConversion(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	markOpenAICompactionSummaryExecution(c, openAICompactionKindV2)

	stopReason := apicompat.AnthropicStopReasonPtr("end_turn")
	require.NoError(t, validateOpenAICompactionSummaryAnthropicTerminal(c, &apicompat.AnthropicResponse{StopReason: stopReason}))
	require.ErrorIs(t, validateOpenAICompactionSummaryAnthropicTerminal(c, &apicompat.AnthropicResponse{}), ErrOpenAICompactionSummaryGeneration)
	maxTokens := apicompat.AnthropicStopReasonPtr("max_tokens")
	require.ErrorIs(t, validateOpenAICompactionSummaryAnthropicTerminal(c, &apicompat.AnthropicResponse{StopReason: maxTokens}), ErrOpenAICompactionSummaryGeneration)

	require.NoError(t, validateOpenAICompactionSummaryChatTerminal(c, &apicompat.ChatCompletionsResponse{Choices: []apicompat.ChatChoice{{FinishReason: "stop"}}}))
	require.ErrorIs(t, validateOpenAICompactionSummaryChatTerminal(c, &apicompat.ChatCompletionsResponse{Choices: []apicompat.ChatChoice{{}}}), ErrOpenAICompactionSummaryGeneration)
	require.ErrorIs(t, validateOpenAICompactionSummaryChatTerminal(c, &apicompat.ChatCompletionsResponse{Choices: []apicompat.ChatChoice{{FinishReason: "length"}}}), ErrOpenAICompactionSummaryGeneration)
}

package service

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// A replayed Codex conversation carries one reasoning item per assistant turn,
// and long sessions accumulate hundreds of them. The upstream names only the
// first offending index per response, so clearing a single index per round trip
// needs one retry per item and exhausts the bounded retry budget long before the
// request can succeed. Every reasoning item must be cleared in one pass.
func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyClearsEveryReasoningContentInOnePass(t *testing.T) {
	items := make([]string, 0, 80)
	for i := 0; i < 40; i++ {
		items = append(items, fmt.Sprintf(
			`{"type":"reasoning","id":"rs_%d","summary":[],"content":[{"type":"reasoning_text","text":"thought %d"}],"encrypted_content":"enc-%d"}`,
			i, i, i))
		items = append(items, fmt.Sprintf(
			`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer %d"}]}`, i))
	}
	body := []byte(`{"model":"gpt-5.6-sol","input":[` + strings.Join(items, ",") + `]}`)
	responseBody := []byte(`{"error":{"code":"array_above_max_length","message":"Invalid 'input[32].content': array too long. Expected an array with maximum length 0, but got an array with length 1 instead.","param":"input[32].content","type":"invalid_request_error"}}`)

	retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "indexed reasoning content maximum-length rejection", reason)

	input := gjson.GetBytes(retryBody, "input")
	require.True(t, input.IsArray())
	require.Len(t, input.Array(), 80, "no input item may be dropped")
	for i, item := range input.Array() {
		switch item.Get("type").String() {
		case "reasoning":
			require.False(t, item.Get("content").Exists(),
				"reasoning content must be cleared at input[%d]", i)
			require.True(t, item.Get("encrypted_content").Exists(),
				"encrypted_content must survive at input[%d]", i)
		case "message":
			require.True(t, item.Get("content").IsArray(),
				"message content must survive at input[%d]", i)
		}
	}
}

// The upstream rejection proves only that reasoning items reject content. A
// single failure must not strip content from message or tool-call items.
func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyKeepsNonReasoningContentWhenClearingAll(t *testing.T) {
	body := []byte(`{"input":[` +
		`{"type":"reasoning","content":[{"type":"reasoning_text","text":"drop"}]},` +
		`{"type":"custom_tool_call","call_id":"ctc_1","name":"shell","input":"ls","content":[{"type":"input_text","text":"keep"}]},` +
		`{"type":"reasoning","content":[{"type":"reasoning_text","text":"drop too"}]}` +
		`]}`)
	responseBody := []byte(`{"error":{"code":"array_above_max_length","message":"Invalid 'input[0].content': array too long. Expected an array with maximum length 0, but got an array with length 1 instead.","param":"input[0].content","type":"invalid_request_error"}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(retryBody, "input.0.content").Exists())
	require.Equal(t, "keep", gjson.GetBytes(retryBody, "input.1.content.0.text").String())
	require.False(t, gjson.GetBytes(retryBody, "input.2.content").Exists())
}

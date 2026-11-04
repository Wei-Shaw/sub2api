package service

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func legacyOpenAIWSEventEnvelope(message []byte) (string, string, gjson.Result) {
	if len(message) == 0 {
		return "", "", gjson.Result{}
	}
	values := gjson.GetManyBytes(message, "type", "response.id", "id", "response")
	id := strings.TrimSpace(values[1].String())
	if id == "" {
		id = strings.TrimSpace(values[2].String())
	}
	return strings.TrimSpace(values[0].String()), id, values[3]
}

func TestOpenAIWSEventEnvelopeRetainedStringsOwnTheirBytes(t *testing.T) {
	body := []byte(`{"type":"response.completed","response":{"id":"resp_borrowed","output":[]}}`)
	eventType, responseID, _ := parseOpenAIWSEventEnvelope(body)
	for i := range body {
		body[i] = 'x'
	}
	require.Equal(t, "response.completed", eventType)
	require.Equal(t, "resp_borrowed", responseID)
}

func FuzzOpenAIWSEventEnvelope(f *testing.F) {
	for _, payload := range []string{
		`{"type":"response.completed","response":{"id":"resp_a"}}`,
		`{"id":"root","response":{"id":" "},"type":"response.created"}`,
		`{"response":{"id":"resp_a"},"response":{"id":"resp_b"},"type":" event "}`,
		`{"type":"response.delta","response":null}`, `{"type":"x"}tail`, `not json`,
	} {
		f.Add(payload)
	}
	f.Fuzz(func(t *testing.T, payload string) {
		if len(payload) > 64<<10 {
			return
		}
		body := []byte(payload)
		wantType, wantID, wantResponse := legacyOpenAIWSEventEnvelope(body)
		gotType, gotID, gotResponse := parseOpenAIWSEventEnvelope(body)
		require.Equal(t, wantType, gotType)
		require.Equal(t, wantID, gotID)
		require.Equal(t, wantResponse.Raw, gotResponse.Raw)
		require.Equal(t, wantResponse.Type, gotResponse.Type)
		require.Equal(t, wantResponse.Index, gotResponse.Index)
		for i := range body {
			body[i] = 'x'
		}
		require.Equal(t, wantType, gotType)
		require.Equal(t, wantID, gotID)
	})
}

func TestOpenAIWSEventEnvelopeLargeResponseIsReadOnly(t *testing.T) {
	body := []byte(`{"type":"response.completed","response":{"id":"resp_large","text":"` + strings.Repeat("x", 1<<20) + `"}}`)
	original := bytes.Clone(body)
	_, id, response := parseOpenAIWSEventEnvelope(body)
	require.Equal(t, "resp_large", id)
	require.Equal(t, 1<<20, len(response.Get("text").String()))
	require.Equal(t, original, body)
}

func TestParseOpenAIWSEventEnvelope(t *testing.T) {
	eventType, responseID, response := parseOpenAIWSEventEnvelope([]byte(`{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.1"}}`))
	require.Equal(t, "response.completed", eventType)
	require.Equal(t, "resp_1", responseID)
	require.True(t, response.Exists())
	require.Equal(t, `{"id":"resp_1","model":"gpt-5.1"}`, response.Raw)

	eventType, responseID, response = parseOpenAIWSEventEnvelope([]byte(`{"type":"response.delta","id":"evt_1"}`))
	require.Equal(t, "response.delta", eventType)
	require.Equal(t, "evt_1", responseID)
	require.False(t, response.Exists())
}

func TestParseOpenAIWSResponseUsageFromCompletedEvent(t *testing.T) {
	usage := &OpenAIUsage{}
	parseOpenAIWSResponseUsageFromCompletedEvent(
		[]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":11,"output_tokens":7,"input_tokens_details":{"cached_tokens":3}}}}`),
		usage,
	)
	require.Equal(t, 11, usage.InputTokens)
	require.Equal(t, 7, usage.OutputTokens)
	require.Equal(t, 3, usage.CacheReadInputTokens)

	parseOpenAIWSResponseUsageFromCompletedEvent(
		[]byte(`{"type":"response.completed","response":{"usage":{"prompt_tokens":19,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":4}}}}`),
		usage,
	)
	require.Equal(t, 19, usage.InputTokens)
	require.Equal(t, 5, usage.OutputTokens)
	require.Equal(t, 4, usage.CacheReadInputTokens)
}

func TestOpenAIWSEventShouldParseUsageTerminalEvents(t *testing.T) {
	t.Parallel()

	for _, eventType := range []string{
		"response.completed",
		"response.done",
		"response.failed",
		"response.incomplete",
		"response.cancelled",
		"response.canceled",
	} {
		require.True(t, openAIWSEventShouldParseUsage(eventType), eventType)
		require.True(t, openAIWSEventShouldParseUsage("  "+eventType+"  "), eventType)
	}
	require.False(t, openAIWSEventShouldParseUsage("response.output_text.delta"))
	require.False(t, openAIWSEventShouldParseUsage(""))
}

func TestOpenAIWSErrorEventHelpers_ConsistentWithWrapper(t *testing.T) {
	message := []byte(`{"type":"error","error":{"type":"invalid_request_error","code":"invalid_request","message":"invalid input"}}`)
	codeRaw, errTypeRaw, errMsgRaw := parseOpenAIWSErrorEventFields(message)

	wrappedReason, wrappedRecoverable := classifyOpenAIWSErrorEvent(message)
	rawReason, rawRecoverable := classifyOpenAIWSErrorEventFromRaw(codeRaw, errTypeRaw, errMsgRaw)
	require.Equal(t, wrappedReason, rawReason)
	require.Equal(t, wrappedRecoverable, rawRecoverable)

	wrappedStatus := openAIWSErrorHTTPStatus(message)
	rawStatus := openAIWSErrorHTTPStatusFromRaw(codeRaw, errTypeRaw)
	require.Equal(t, wrappedStatus, rawStatus)
	require.Equal(t, http.StatusBadRequest, rawStatus)

	wrappedCode, wrappedType, wrappedMsg := summarizeOpenAIWSErrorEventFields(message)
	rawCode, rawType, rawMsg := summarizeOpenAIWSErrorEventFieldsFromRaw(codeRaw, errTypeRaw, errMsgRaw)
	require.Equal(t, wrappedCode, rawCode)
	require.Equal(t, wrappedType, rawType)
	require.Equal(t, wrappedMsg, rawMsg)
}

func TestOpenAIWSMessageLikelyContainsToolCalls(t *testing.T) {
	require.False(t, openAIWSMessageLikelyContainsToolCalls([]byte(`{"type":"response.output_text.delta","delta":"hello"}`)))
	require.True(t, openAIWSMessageLikelyContainsToolCalls([]byte(`{"type":"response.output_item.added","item":{"tool_calls":[{"id":"tc1"}]}}`)))
	require.True(t, openAIWSMessageLikelyContainsToolCalls([]byte(`{"type":"response.output_item.added","item":{"type":"function_call"}}`)))
}

func TestReplaceOpenAIWSMessageModel_OptimizedStillCorrect(t *testing.T) {
	noModel := []byte(`{"type":"response.output_text.delta","delta":"hello"}`)
	require.Equal(t, string(noModel), string(replaceOpenAIWSMessageModel(noModel, "gpt-5.1", "custom-model")))

	rootOnly := []byte(`{"type":"response.created","model":"gpt-5.1"}`)
	require.Equal(t, `{"type":"response.created","model":"custom-model"}`, string(replaceOpenAIWSMessageModel(rootOnly, "gpt-5.1", "custom-model")))

	responseOnly := []byte(`{"type":"response.completed","response":{"model":"gpt-5.1"}}`)
	require.Equal(t, `{"type":"response.completed","response":{"model":"custom-model"}}`, string(replaceOpenAIWSMessageModel(responseOnly, "gpt-5.1", "custom-model")))

	both := []byte(`{"model":"gpt-5.1","response":{"model":"gpt-5.1"}}`)
	require.Equal(t, `{"model":"custom-model","response":{"model":"custom-model"}}`, string(replaceOpenAIWSMessageModel(both, "gpt-5.1", "custom-model")))
}

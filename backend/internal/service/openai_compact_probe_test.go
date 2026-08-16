package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeAccountTestMode(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
	}{
		{"", AccountTestModeDefault},
		{"default", AccountTestModeDefault},
		{" compact ", AccountTestModeCompact},
		{"COMPACT", AccountTestModeCompact},
		{"unknown", AccountTestModeDefault},
	} {
		require.Equal(t, tt.want, normalizeAccountTestMode(tt.input))
	}
}

func TestCreateOpenAICompactProbePayload_NativeV2Shape(t *testing.T) {
	payload := createOpenAICompactProbePayload("gpt-5.6-sol", true)
	require.Equal(t, true, payload["stream"])
	require.Equal(t, false, payload["store"])
	require.Equal(t, true, payload["parallel_tool_calls"])
	require.Empty(t, payload["tools"])
	input, ok := payload["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)
	trigger, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "compaction_trigger", trigger["type"])
	clientMetadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	metadata, ok := clientMetadata["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	require.Equal(t, "compaction", gjson.Get(metadata, "request_kind").String())
	require.Equal(t, "manual", gjson.Get(metadata, "compaction.trigger").String())
	require.Equal(t, "user_requested", gjson.Get(metadata, "compaction.reason").String())
	require.Equal(t, "responses_compaction_v2", gjson.Get(metadata, "compaction.implementation").String())
	require.Equal(t, "standalone_turn", gjson.Get(metadata, "compaction.phase").String())
	require.Equal(t, "memento", gjson.Get(metadata, "compaction.strategy").String())

	apiKeyPayload := createOpenAICompactProbePayload("gpt-5.6-sol", false)
	_, hasStore := apiKeyPayload["store"]
	_, hasMetadata := apiKeyPayload["client_metadata"]
	require.False(t, hasStore)
	require.False(t, hasMetadata, "API-key probes must not synthesize Codex metadata")
	require.Equal(t, true, apiKeyPayload["parallel_tool_calls"])
	require.Empty(t, apiKeyPayload["tools"])
}

func TestEvaluateOpenAICompactProbeSSE_StateMachine(t *testing.T) {
	doneCompaction := "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction\",\"id\":\"cmp_1\"}}\n\n"
	doneMessage := "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"id\":\"msg_1\"}}\n\n"
	completed := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\"}}\n\n"
	largeDoneCompaction := "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction\"},\"padding\":\"" +
		strings.Repeat("x", 70*1024) + "\"}\n\n"

	for _, tt := range []struct {
		name        string
		body        string
		wantVerdict openAIProbeVerdict
	}{
		{"official_success", doneCompaction + completed, openAIProbeVerdictSupported},
		{"other_done_items_allowed", doneMessage + doneCompaction + completed, openAIProbeVerdictSupported},
		{"other_done_item_after_compaction", doneCompaction + doneMessage + completed, openAIProbeVerdictSupported},
		{"comments_and_done_marker", ": keepalive\n\n" + doneCompaction + completed + "data: [DONE]\n\n", openAIProbeVerdictSupported},
		{"crlf_framing", strings.ReplaceAll(doneCompaction+completed, "\n", "\r\n"), openAIProbeVerdictSupported},
		{"lone_cr_framing", strings.ReplaceAll(doneCompaction+completed, "\n", "\r"), openAIProbeVerdictSupported},
		{"mixed_line_endings", strings.Replace(doneCompaction, "\n\n", "\r\r", 1) + completed, openAIProbeVerdictSupported},
		{"multi_data_line_json", "data: {\"type\":\"response.output_item.done\",\n" +
			"data: \"item\":{\"type\":\"compaction\"}}\n\n" + completed, openAIProbeVerdictSupported},
		{"multi_data_line_with_comment", "data: {\"type\":\"response.output_item.done\",\n" +
			": keepalive\n" +
			"data: \"item\":{\"type\":\"compaction\"}}\n\n" + completed, openAIProbeVerdictSupported},
		{"sse_fields_and_eof_terminated_event", "event: response.output_item.done\nid: event-1\n" + doneCompaction +
			"event: response.completed\ndata: {\"type\":\"response.completed\"}", openAIProbeVerdictSupported},
		{"field_without_colon_is_legal", "event\ndata\n\nretry\n\n" + doneCompaction + completed, openAIProbeVerdictSupported},
		{"utf8_bom_is_ignored", "\ufeff" + doneCompaction + completed, openAIProbeVerdictSupported},
		{"empty_data_line_before_payload", "data:\ndata: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction\"}}\n\n" + completed, openAIProbeVerdictSupported},
		{"comments_after_done_marker", doneCompaction + completed + "data: [DONE]\n\n: stream closed\n", openAIProbeVerdictSupported},
		{"large_single_line_event", largeDoneCompaction + completed, openAIProbeVerdictSupported},
		{"completed_without_compaction_is_unsupported", doneMessage + completed, openAIProbeVerdictUnsupported},
		{"official_compaction_summary_alias", "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction_summary\"}}\n\n" + completed, openAIProbeVerdictSupported},
		{"added_is_not_done", "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"compaction\"}}\n\n" + completed, openAIProbeVerdictUnsupported},
		{"missing_terminal", doneCompaction, openAIProbeVerdictUnknown},
		{"completed_without_any_done", completed, openAIProbeVerdictUnsupported},
		{"bare_json", `{"status":"completed","output":[{"type":"compaction"}]}`, openAIProbeVerdictUnknown},
		{"invalid_json", "data: not-json\n\n", openAIProbeVerdictUnknown},
		{"failed_terminal", doneCompaction + "data: {\"type\":\"response.failed\"}\n\n", openAIProbeVerdictUnknown},
		{"incomplete_terminal", doneCompaction + "data: {\"type\":\"response.incomplete\"}\n\n", openAIProbeVerdictUnknown},
		{"cancelled_terminal", doneCompaction + "data: {\"type\":\"response.cancelled\"}\n\n", openAIProbeVerdictUnknown},
		{"duplicate_compaction", doneCompaction + doneCompaction + completed, openAIProbeVerdictUnknown},
		{"duplicate_terminal_after_completed_is_ignored", doneCompaction + completed + completed, openAIProbeVerdictSupported},
		{"output_after_completed_is_ignored", doneCompaction + completed + doneCompaction, openAIProbeVerdictSupported},
		{"failure_after_completed_is_ignored", doneCompaction + completed + "data: {\"type\":\"response.failed\"}\n\n", openAIProbeVerdictSupported},
		{"invalid_bytes_after_completed_are_ignored", doneCompaction + completed + "not-an-sse-field\n", openAIProbeVerdictSupported},
		{"completed_before_compaction_stays_unsupported", completed + doneCompaction, openAIProbeVerdictUnsupported},
		{"completed_after_done_marker", doneCompaction + "data: [DONE]\n\n" + completed, openAIProbeVerdictUnknown},
		{"empty", "", openAIProbeVerdictUnknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := evaluateOpenAICompactProbeSSE([]byte(tt.body))
			require.Equal(t, tt.wantVerdict, got)
		})
	}
}

func TestEvaluateOpenAICompactProbeHTTP(t *testing.T) {
	unsupportedBody := []byte(`{"error":{"message":"remote compact is unsupported"}}`)
	validSSE := []byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction\"}}\n\n" +
		"data: {\"type\":\"response.completed\"}\n\n")
	for _, tt := range []struct {
		name   string
		status int
		body   []byte
		want   openAIProbeVerdict
	}{
		{"not_found", http.StatusNotFound, nil, openAIProbeVerdictUnsupported},
		{"method_not_allowed", http.StatusMethodNotAllowed, nil, openAIProbeVerdictUnsupported},
		{"not_implemented", http.StatusNotImplemented, nil, openAIProbeVerdictUnsupported},
		{"bad_request_explicit_unsupported", http.StatusBadRequest, unsupportedBody, openAIProbeVerdictUnsupported},
		{"forbidden_explicit_unsupported", http.StatusForbidden, unsupportedBody, openAIProbeVerdictUnsupported},
		{"unprocessable_explicit_unsupported", http.StatusUnprocessableEntity, unsupportedBody, openAIProbeVerdictUnsupported},
		{"bad_request_without_evidence", http.StatusBadRequest, []byte(`{"error":{"message":"invalid model"}}`), openAIProbeVerdictUnknown},
		{"auth_failure", http.StatusUnauthorized, nil, openAIProbeVerdictUnknown},
		{"forbidden_without_evidence", http.StatusForbidden, []byte(`{"error":{"message":"access denied"}}`), openAIProbeVerdictUnknown},
		{"rate_limit", http.StatusTooManyRequests, nil, openAIProbeVerdictUnknown},
		{"internal_server_error", http.StatusInternalServerError, nil, openAIProbeVerdictUnknown},
		{"bad_gateway", http.StatusBadGateway, nil, openAIProbeVerdictUnknown},
		{"created_with_complete_stream", http.StatusCreated, validSSE, openAIProbeVerdictSupported},
		{"accepted_with_complete_stream", http.StatusAccepted, validSSE, openAIProbeVerdictSupported},
		{"partial_content_with_complete_stream", http.StatusPartialContent, validSSE, openAIProbeVerdictSupported},
		{"status_299_with_complete_stream", 299, validSSE, openAIProbeVerdictSupported},
		{"no_content_without_stream", http.StatusNoContent, nil, openAIProbeVerdictUnknown},
		{"accepted_with_incomplete_stream", http.StatusAccepted, validSSE[:len(validSSE)/2], openAIProbeVerdictUnknown},
		{"partial_content_with_incomplete_stream", http.StatusPartialContent, []byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction\"}}\n\n"), openAIProbeVerdictUnknown},
		{"two_xx_invalid_json", http.StatusOK, []byte("not-json"), openAIProbeVerdictUnknown},
		{"two_xx_without_terminal", http.StatusOK, []byte("data: [DONE]\n\n"), openAIProbeVerdictUnknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := evaluateOpenAICompactProbeHTTP(&http.Response{StatusCode: tt.status}, tt.body)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_TriStateAndDiagnostics(t *testing.T) {
	started := time.Date(2026, 8, 16, 1, 0, 0, 123, time.UTC)
	checked := started.Add(2 * time.Second)
	for _, tt := range []struct {
		name           string
		verdict        openAIProbeVerdict
		reason         string
		wantValue      any
		wantConclusive bool
		wantError      string
	}{
		{"supported", openAIProbeVerdictSupported, "", true, true, ""},
		{"unsupported", openAIProbeVerdictUnsupported, "no compaction item", false, true, "no compaction item"},
		{"unknown", openAIProbeVerdictUnknown, "missing terminal", nil, false, "missing terminal"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusOK}, nil, nil, tt.verdict, tt.reason, started, checked)
			if tt.wantConclusive {
				require.Equal(t, tt.wantValue, updates[openAICompactProbeSupportedExtraKey])
				require.Equal(t, openAICompactProbeProtocolVersion, updates[openAICompactProbeVersionExtraKey])
				require.Equal(t, checked.Format(time.RFC3339Nano), updates[openAICompactProbeCheckedAtExtraKey])
				require.Equal(t, started.UnixNano(), updates[OpenAICompactProbeObservedAtUnixNanoExtraKey])
			} else {
				require.NotContains(t, updates, openAICompactProbeSupportedExtraKey)
				require.NotContains(t, updates, openAICompactProbeVersionExtraKey)
				require.NotContains(t, updates, openAICompactProbeCheckedAtExtraKey)
			}
			require.Equal(t, started.UnixNano(), updates[OpenAICompactProbeObservedAtUnixNanoExtraKey])
			require.Equal(t, http.StatusOK, updates[openAICompactProbeLastStatusExtraKey])
			require.Equal(t, tt.wantError, updates[openAICompactProbeLastErrorExtraKey])
		})
	}
	requestErr := buildOpenAICompactProbeExtraUpdates(nil, nil, errors.New("dial timeout"), openAIProbeVerdictUnknown, "", started, checked)
	require.NotContains(t, requestErr, openAICompactProbeSupportedExtraKey)
	require.NotContains(t, requestErr, openAICompactProbeVersionExtraKey)
	require.NotContains(t, requestErr, openAICompactProbeCheckedAtExtraKey)
	require.Equal(t, started.UnixNano(), requestErr[OpenAICompactProbeObservedAtUnixNanoExtraKey])
	require.Nil(t, requestErr[openAICompactProbeLastStatusExtraKey])
	require.Contains(t, requestErr[openAICompactProbeLastErrorExtraKey], "dial timeout")
}

type failingProbeReader struct {
	data []byte
	err  error
}

func (r *failingProbeReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	return 0, r.err
}

func TestReadOpenAIProbeBody_DetectsTruncationAndReadErrors(t *testing.T) {
	data, err := readOpenAIProbeBody(strings.NewReader("1234"), 4)
	require.NoError(t, err)
	require.Equal(t, "1234", string(data))
	data, err = readOpenAIProbeBody(strings.NewReader("12345"), 4)
	require.ErrorIs(t, err, errOpenAIProbeBodyTooLarge)
	require.Equal(t, "1234", string(data))
	wantErr := errors.New("partial transport failure")
	data, err = readOpenAIProbeBody(&failingProbeReader{data: []byte("partial"), err: wantErr}, 100)
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, "partial", string(data))
	data, err = readOpenAIProbeBody(io.LimitReader(strings.NewReader("abcdef"), 3), 10)
	require.NoError(t, err)
	require.Equal(t, "abc", string(data))
}

func TestOpenAICompactProbeSnapshotFresh(t *testing.T) {
	now := time.Date(2026, 8, 16, 2, 0, 0, 0, time.UTC)
	valid := map[string]any{
		openAICompactProbeVersionExtraKey:   float64(openAICompactProbeProtocolVersion),
		openAICompactProbeCheckedAtExtraKey: now.Add(-time.Hour).Format(time.RFC3339Nano),
	}
	require.True(t, openAICompactProbeSnapshotFresh(valid, now))
	require.False(t, openAICompactProbeSnapshotFresh(map[string]any{
		openAICompactProbeVersionExtraKey:   openAICompactProbeProtocolVersion - 1,
		openAICompactProbeCheckedAtExtraKey: now.Format(time.RFC3339Nano),
	}, now))
	require.False(t, openAICompactProbeSnapshotFresh(map[string]any{
		openAICompactProbeVersionExtraKey:   openAICompactProbeProtocolVersion,
		openAICompactProbeCheckedAtExtraKey: now.Add(-openAICompactProbeMaxAge - time.Second).Format(time.RFC3339Nano),
	}, now))
	require.False(t, openAICompactProbeSnapshotFresh(map[string]any{}, now), "unversioned snapshots must never survive an upgrade")

	for _, tt := range []struct {
		name      string
		checkedAt string
		want      bool
	}{
		{"exact max age", now.Add(-openAICompactProbeMaxAge).Format(time.RFC3339Nano), true},
		{"past max age", now.Add(-openAICompactProbeMaxAge - time.Nanosecond).Format(time.RFC3339Nano), false},
		{"exact future skew", now.Add(5 * time.Minute).Format(time.RFC3339Nano), true},
		{"beyond future skew", now.Add(5*time.Minute + time.Nanosecond).Format(time.RFC3339Nano), false},
		{"blank", " ", false},
		{"invalid", "not-a-time", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			extra := map[string]any{
				openAICompactProbeVersionExtraKey:   openAICompactProbeProtocolVersion,
				openAICompactProbeCheckedAtExtraKey: tt.checkedAt,
			}
			require.Equal(t, tt.want, openAICompactProbeSnapshotFresh(extra, now))
		})
	}
}

func TestNumericExtraEquals(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value any
		want  bool
	}{
		{"int", int(openAICompactProbeProtocolVersion), true},
		{"int64", int64(openAICompactProbeProtocolVersion), true},
		{"float64", float64(openAICompactProbeProtocolVersion), true},
		{"json_number", json.Number(strconv.Itoa(openAICompactProbeProtocolVersion)), true},
		{"different", openAICompactProbeProtocolVersion + 1, false},
		{"fractional", float64(openAICompactProbeProtocolVersion) + 0.5, false},
		{"invalid_json_number", json.Number("not-a-number"), false},
		{"string", strconv.Itoa(openAICompactProbeProtocolVersion), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, numericExtraEquals(tt.value, openAICompactProbeProtocolVersion))
		})
	}
}

func TestCompactProbeSessionID_IsUUIDShapedAndStable(t *testing.T) {
	for _, accountID := range []int64{0, 1, 987654} {
		_, err := uuid.Parse(compactProbeSessionID(accountID))
		require.NoError(t, err)
	}
	require.Equal(t, compactProbeSessionID(7), compactProbeSessionID(7))
	require.NotEqual(t, compactProbeSessionID(7), compactProbeSessionID(8))
}

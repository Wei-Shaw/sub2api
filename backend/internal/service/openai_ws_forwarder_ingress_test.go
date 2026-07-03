package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIsOpenAIWSClientDisconnectError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
REDACTED{
		{name: "nil", err: nil, want: falseREDACTED,
		{name: "io_eof", err: io.EOF, want: trueREDACTED,
		{name: "net_closed", err: net.ErrClosed, want: trueREDACTED,
		{name: "context_canceled", err: context.Canceled, want: trueREDACTED,
		{name: "ws_normal_closure", err: coderws.CloseError{Code: coderws.StatusNormalClosureREDACTED, want: trueREDACTED,
		{name: "ws_going_away", err: coderws.CloseError{Code: coderws.StatusGoingAwayREDACTED, want: trueREDACTED,
		{name: "ws_no_status", err: coderws.CloseError{Code: coderws.StatusNoStatusRcvdREDACTED, want: trueREDACTED,
		{name: "ws_abnormal_1006", err: coderws.CloseError{Code: coderws.StatusAbnormalClosureREDACTED, want: trueREDACTED,
		{name: "ws_policy_violation", err: coderws.CloseError{Code: coderws.StatusPolicyViolationREDACTED, want: falseREDACTED,
		{name: "wrapped_eof_message", err: errors.New("failed to get reader: failed to read frame header: EOF"), want: trueREDACTED,
		{name: "connection_reset_by_peer", err: errors.New("failed to read frame header: read tcp 127.0.0.1:1234->127.0.0.1:5678: read: connection reset by peer"), want: trueREDACTED,
		{name: "broken_pipe", err: errors.New("write tcp 127.0.0.1:1234->127.0.0.1:5678: write: broken pipe"), want: trueREDACTED,
REDACTED

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isOpenAIWSClientDisconnectError(tt.err))
	REDACTED)
REDACTED
REDACTED

func TestIsOpenAIWSIngressPreviousResponseNotFound(t *testing.T) {
	t.Parallel()

	require.False(t, isOpenAIWSIngressPreviousResponseNotFound(nil))
	require.False(t, isOpenAIWSIngressPreviousResponseNotFound(errors.New("plain error")))
	require.False(t, isOpenAIWSIngressPreviousResponseNotFound(
		wrapOpenAIWSIngressTurnError("read_upstream", errors.New("upstream read failed"), false),
	))
	require.False(t, isOpenAIWSIngressPreviousResponseNotFound(
		wrapOpenAIWSIngressTurnError(openAIWSIngressStagePreviousResponseNotFound, errors.New("previous response not found"), true),
	))
	require.True(t, isOpenAIWSIngressPreviousResponseNotFound(
		wrapOpenAIWSIngressTurnError(openAIWSIngressStagePreviousResponseNotFound, errors.New("previous response not found"), false),
	))
REDACTED

func TestOpenAIWSIngressPreviousResponseRecoveryEnabled(t *testing.T) {
	t.Parallel()

	var nilService *OpenAIGatewayService
	require.True(t, nilService.openAIWSIngressPreviousResponseRecoveryEnabled(), "nil service should default to enabled")

	svcWithNilCfg := &OpenAIGatewayService{REDACTED
	require.True(t, svcWithNilCfg.openAIWSIngressPreviousResponseRecoveryEnabled(), "nil config should default to enabled")

	svc := &OpenAIGatewayService{
		cfg: &config.Config{REDACTED,
REDACTED
	require.False(t, svc.openAIWSIngressPreviousResponseRecoveryEnabled(), "explicit config default should be false")

	svc.cfg.Gateway.OpenAIWS.IngressPreviousResponseRecoveryEnabled = true
	require.True(t, svc.openAIWSIngressPreviousResponseRecoveryEnabled())
REDACTED

func TestDropPreviousResponseIDFromRawPayload(t *testing.T) {
	t.Parallel()

	t.Run("empty_payload", func(t *testing.T) {
		updated, removed, err := dropPreviousResponseIDFromRawPayload(nil)
	REDACTED
		require.False(t, removed)
		require.Empty(t, updated)
REDACTED)

	t.Run("payload_without_previous_response_id", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1"REDACTED`)
		updated, removed, err := dropPreviousResponseIDFromRawPayload(payload)
	REDACTED
		require.False(t, removed)
		require.Equal(t, string(payload), string(updated))
REDACTED)

	t.Run("normal_delete_success", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_abc"REDACTED`)
		updated, removed, err := dropPreviousResponseIDFromRawPayload(payload)
	REDACTED
		require.True(t, removed)
		require.False(t, gjson.GetBytes(updated, "previous_response_id").Exists())
REDACTED)

	t.Run("duplicate_keys_are_removed", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","previous_response_id":"resp_a","input":[],"previous_response_id":"resp_b"REDACTED`)
		updated, removed, err := dropPreviousResponseIDFromRawPayload(payload)
	REDACTED
		require.True(t, removed)
		require.False(t, gjson.GetBytes(updated, "previous_response_id").Exists())
REDACTED)

	t.Run("nil_delete_fn_uses_default_delete_logic", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_abc"REDACTED`)
		updated, removed, err := dropPreviousResponseIDFromRawPayloadWithDeleteFn(payload, nil)
	REDACTED
		require.True(t, removed)
		require.False(t, gjson.GetBytes(updated, "previous_response_id").Exists())
REDACTED)

	t.Run("delete_error", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_abc"REDACTED`)
		updated, removed, err := dropPreviousResponseIDFromRawPayloadWithDeleteFn(payload, func(_ []byte, _ string) ([]byte, error) {
			return nil, errors.New("delete failed")
	REDACTED)
	REDACTED
		require.False(t, removed)
		require.Equal(t, string(payload), string(updated))
REDACTED)

	t.Run("malformed_json_is_still_best_effort_deleted", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","previous_response_id":"resp_abc"`)
		require.True(t, gjson.GetBytes(payload, "previous_response_id").Exists())

		updated, removed, err := dropPreviousResponseIDFromRawPayload(payload)
	REDACTED
		require.True(t, removed)
		require.False(t, gjson.GetBytes(updated, "previous_response_id").Exists())
REDACTED)
REDACTED

func TestStripCodexSparkImageGenerationToolFromRawPayload(t *testing.T) {
	t.Run("strips_image_generation_for_spark", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.3-codex-spark","tools":[{"type":"function","name":"shell"REDACTED,{"type":"image_generation","output_format":"png"REDACTED]REDACTED`)
		updated, changed, err := stripCodexSparkImageGenerationToolFromRawPayload(payload, "gpt-5.3-codex-spark")
	REDACTED
		require.True(t, changed)
		require.False(t, gjson.GetBytes(updated, `tools.#(type=="image_generation")`).Exists())
		require.True(t, gjson.GetBytes(updated, `tools.#(type=="function")`).Exists())
REDACTED)

	t.Run("keeps_image_generation_for_non_spark", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.3-codex","tools":[{"type":"image_generation","output_format":"png"REDACTED]REDACTED`)
		updated, changed, err := stripCodexSparkImageGenerationToolFromRawPayload(payload, "gpt-5.3-codex")
	REDACTED
		require.False(t, changed)
		require.Equal(t, string(payload), string(updated))
REDACTED)

	t.Run("noop_when_no_image_tool", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.3-codex-spark","tools":[{"type":"function","name":"shell"REDACTED]REDACTED`)
		updated, changed, err := stripCodexSparkImageGenerationToolFromRawPayload(payload, "gpt-5.3-codex-spark")
	REDACTED
		require.False(t, changed)
		require.Equal(t, string(payload), string(updated))
REDACTED)
REDACTED

func TestStripOpenAIImageGenerationToolFromRawPayload(t *testing.T) {
	payload := []byte(`{
		"type":"response.create",
		"model":"gpt-5.4",
		"tools":[
			{"type":"function","name":"shell"REDACTED,
			{"type":"image_generation","output_format":"png"REDACTED
		],
		"tool_choice":{"type":"image_generation"REDACTED
REDACTED`)

	updated, changed, err := stripOpenAIImageGenerationToolFromRawPayload(payload)

REDACTED
	require.True(t, changed)
	require.False(t, gjson.GetBytes(updated, `tools.#(type=="image_generation")`).Exists())
	require.True(t, gjson.GetBytes(updated, `tools.#(type=="function")`).Exists())
	require.False(t, gjson.GetBytes(updated, "tool_choice").Exists())
REDACTED

func TestAlignStoreDisabledPreviousResponseID(t *testing.T) {
	t.Parallel()

	t.Run("empty_payload", func(t *testing.T) {
		updated, changed, err := alignStoreDisabledPreviousResponseID(nil, "resp_target")
	REDACTED
		require.False(t, changed)
		require.Empty(t, updated)
REDACTED)

	t.Run("empty_expected", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","previous_response_id":"resp_old"REDACTED`)
		updated, changed, err := alignStoreDisabledPreviousResponseID(payload, "")
	REDACTED
		require.False(t, changed)
		require.Equal(t, string(payload), string(updated))
REDACTED)

	t.Run("missing_previous_response_id", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1"REDACTED`)
		updated, changed, err := alignStoreDisabledPreviousResponseID(payload, "resp_target")
	REDACTED
		require.False(t, changed)
		require.Equal(t, string(payload), string(updated))
REDACTED)

	t.Run("already_aligned", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","previous_response_id":"resp_target"REDACTED`)
		updated, changed, err := alignStoreDisabledPreviousResponseID(payload, "resp_target")
	REDACTED
		require.False(t, changed)
		require.Equal(t, "resp_target", gjson.GetBytes(updated, "previous_response_id").String())
REDACTED)

	t.Run("mismatch_rewrites_to_expected", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","previous_response_id":"resp_old","input":[]REDACTED`)
		updated, changed, err := alignStoreDisabledPreviousResponseID(payload, "resp_target")
	REDACTED
		require.True(t, changed)
		require.Equal(t, "resp_target", gjson.GetBytes(updated, "previous_response_id").String())
REDACTED)

	t.Run("duplicate_keys_rewrites_to_single_expected", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","previous_response_id":"resp_old_1","input":[],"previous_response_id":"resp_old_2"REDACTED`)
		updated, changed, err := alignStoreDisabledPreviousResponseID(payload, "resp_target")
	REDACTED
		require.True(t, changed)
		require.Equal(t, "resp_target", gjson.GetBytes(updated, "previous_response_id").String())
REDACTED)
REDACTED

func TestSetPreviousResponseIDToRawPayload(t *testing.T) {
	t.Parallel()

	t.Run("empty_payload", func(t *testing.T) {
		updated, err := setPreviousResponseIDToRawPayload(nil, "resp_target")
	REDACTED
		require.Empty(t, updated)
REDACTED)

	t.Run("empty_previous_response_id", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1"REDACTED`)
		updated, err := setPreviousResponseIDToRawPayload(payload, "")
	REDACTED
		require.Equal(t, string(payload), string(updated))
REDACTED)

	t.Run("set_previous_response_id_when_missing", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1"REDACTED`)
		updated, err := setPreviousResponseIDToRawPayload(payload, "resp_target")
	REDACTED
		require.Equal(t, "resp_target", gjson.GetBytes(updated, "previous_response_id").String())
		require.Equal(t, "gpt-5.1", gjson.GetBytes(updated, "model").String())
REDACTED)

	t.Run("overwrite_existing_previous_response_id", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_old"REDACTED`)
		updated, err := setPreviousResponseIDToRawPayload(payload, "resp_new")
	REDACTED
		require.Equal(t, "resp_new", gjson.GetBytes(updated, "previous_response_id").String())
REDACTED)
REDACTED

func TestShouldInferIngressFunctionCallOutputPreviousResponseID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		storeDisabled           bool
		turn                    int
		signals                 ToolContinuationSignals
		currentPreviousResponse string
		expectedPrevious        string
		want                    bool
REDACTED{
		{
			name:             "infer_when_all_conditions_match",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: trueREDACTED,
			expectedPrevious: "resp_1",
			want:             true,
	REDACTED,
		{
			name:             "skip_when_store_enabled",
			storeDisabled:    false,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: trueREDACTED,
			expectedPrevious: "resp_1",
			want:             false,
	REDACTED,
		{
			name:             "skip_on_first_turn",
			storeDisabled:    true,
			turn:             1,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: trueREDACTED,
			expectedPrevious: "resp_1",
			want:             false,
	REDACTED,
		{
			name:             "skip_without_function_call_output",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{REDACTED,
			expectedPrevious: "resp_1",
			want:             false,
	REDACTED,
		{
			name:                    "skip_when_request_already_has_previous_response_id",
			storeDisabled:           true,
			turn:                    2,
			signals:                 ToolContinuationSignals{HasFunctionCallOutput: trueREDACTED,
			currentPreviousResponse: "resp_client",
			expectedPrevious:        "resp_1",
			want:                    false,
	REDACTED,
		{
			name:             "skip_when_last_turn_response_id_missing",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: trueREDACTED,
			expectedPrevious: "",
			want:             false,
	REDACTED,
		{
			name:             "trim_whitespace_before_judgement",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: trueREDACTED,
			expectedPrevious: "   resp_2   ",
			want:             true,
	REDACTED,
		{
			name:             "skip_when_tool_call_context_already_present",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: true, HasToolCallContext: trueREDACTED,
			expectedPrevious: "resp_2",
			want:             false,
	REDACTED,
		{
			name:             "infer_when_only_item_reference_covers_call_ids",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: true, HasItemReferenceForAllCallIDs: trueREDACTED,
			expectedPrevious: "resp_2",
			want:             true,
	REDACTED,
		{
			name:             "skip_when_function_call_output_missing_call_id",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: true, HasFunctionCallOutputMissingCallID: trueREDACTED,
			expectedPrevious: "resp_2",
			want:             false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := shouldInferIngressFunctionCallOutputPreviousResponseID(
				tt.storeDisabled,
				tt.turn,
				tt.signals,
				tt.currentPreviousResponse,
				tt.expectedPrevious,
			)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIWSInputIsPrefixExtended(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		previous  []byte
		current   []byte
		want      bool
		expectErr bool
REDACTED{
		{
			name:     "both_missing_input",
			previous: []byte(`{"type":"response.create","model":"gpt-5.1"REDACTED`),
			current:  []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_1"REDACTED`),
			want:     true,
	REDACTED,
		{
			name:     "previous_missing_current_empty_array",
			previous: []byte(`{"type":"response.create","model":"gpt-5.1"REDACTED`),
			current:  []byte(`{"type":"response.create","model":"gpt-5.1","input":[]REDACTED`),
			want:     true,
	REDACTED,
		{
			name:     "previous_missing_current_non_empty_array",
			previous: []byte(`{"type":"response.create","model":"gpt-5.1"REDACTED`),
			current:  []byte(`{"type":"response.create","model":"gpt-5.1","input":[{"type":"input_text","text":"hello"REDACTED]REDACTED`),
			want:     false,
	REDACTED,
		{
			name:     "array_prefix_match",
			previous: []byte(`{"input":[{"type":"input_text","text":"hello"REDACTED]REDACTED`),
			current:  []byte(`{"input":[{"text":"hello","type":"input_text"REDACTED,{"type":"input_text","text":"world"REDACTED]REDACTED`),
			want:     true,
	REDACTED,
		{
			name:     "array_prefix_mismatch",
			previous: []byte(`{"input":[{"type":"input_text","text":"hello"REDACTED]REDACTED`),
			current:  []byte(`{"input":[{"type":"input_text","text":"different"REDACTED]REDACTED`),
			want:     false,
	REDACTED,
		{
			name:     "current_shorter_than_previous",
			previous: []byte(`{"input":[{"type":"input_text","text":"a"REDACTED,{"type":"input_text","text":"b"REDACTED]REDACTED`),
			current:  []byte(`{"input":[{"type":"input_text","text":"a"REDACTED]REDACTED`),
			want:     false,
	REDACTED,
		{
			name:     "previous_has_input_current_missing",
			previous: []byte(`{"input":[{"type":"input_text","text":"a"REDACTED]REDACTED`),
			current:  []byte(`{"model":"gpt-5.1"REDACTED`),
			want:     false,
	REDACTED,
		{
			name:     "input_string_treated_as_single_item",
			previous: []byte(`{"input":"hello"REDACTED`),
			current:  []byte(`{"input":"hello"REDACTED`),
			want:     true,
	REDACTED,
		{
			name:      "current_invalid_input_json",
			previous:  []byte(`{"input":[]REDACTED`),
			current:   []byte(`{"input":[REDACTED`),
			expectErr: true,
	REDACTED,
		{
			name:      "invalid_input_json",
			previous:  []byte(`{"input":[REDACTED`),
			current:   []byte(`{"input":[]REDACTED`),
			expectErr: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := openAIWSInputIsPrefixExtended(tt.previous, tt.current)
			if tt.expectErr {
			REDACTED
				return
		REDACTED
		REDACTED
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

func TestNormalizeOpenAIWSJSONForCompare(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeOpenAIWSJSONForCompare([]byte(`{"b":2,"a":1REDACTED`))
REDACTED
	require.Equal(t, `{"a":1,"b":2REDACTED`, string(normalized))

	_, err = normalizeOpenAIWSJSONForCompare([]byte("   "))
REDACTED

	_, err = normalizeOpenAIWSJSONForCompare([]byte(`{"a":`))
REDACTED
REDACTED

func TestNormalizeOpenAIWSJSONForCompareOrRaw(t *testing.T) {
	t.Parallel()

	require.Equal(t, `{"a":1,"b":2REDACTED`, string(normalizeOpenAIWSJSONForCompareOrRaw([]byte(`{"b":2,"a":1REDACTED`))))
	require.Equal(t, `{"a":`, string(normalizeOpenAIWSJSONForCompareOrRaw([]byte(`{"a":`))))
REDACTED

func TestNormalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID(
		[]byte(`{"model":"gpt-5.1","input":[1],"previous_response_id":"resp_x","metadata":{"b":2,"a":1REDACTEDREDACTED`),
	)
REDACTED
	require.False(t, gjson.GetBytes(normalized, "input").Exists())
	require.False(t, gjson.GetBytes(normalized, "previous_response_id").Exists())
	require.Equal(t, float64(1), gjson.GetBytes(normalized, "metadata.a").Float())

	_, err = normalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID(nil)
REDACTED

	_, err = normalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID([]byte(`[]`))
REDACTED
REDACTED

func TestOpenAIWSExtractNormalizedInputSequence(t *testing.T) {
	t.Parallel()

	t.Run("empty_payload", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence(nil)
	REDACTED
		require.False(t, exists)
		require.Nil(t, items)
REDACTED)

	t.Run("input_missing", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"type":"response.create"REDACTED`))
	REDACTED
		require.False(t, exists)
		require.Nil(t, items)
REDACTED)

	t.Run("input_array", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":[{"type":"input_text","text":"hello"REDACTED]REDACTED`))
	REDACTED
		require.True(t, exists)
		require.Len(t, items, 1)
REDACTED)

	t.Run("input_object", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":{"type":"input_text","text":"hello"REDACTEDREDACTED`))
	REDACTED
		require.True(t, exists)
		require.Len(t, items, 1)
REDACTED)

	t.Run("input_string", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":"hello"REDACTED`))
	REDACTED
		require.True(t, exists)
		require.Len(t, items, 1)
		require.Equal(t, `"hello"`, string(items[0]))
REDACTED)

	t.Run("input_number", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":42REDACTED`))
	REDACTED
		require.True(t, exists)
		require.Len(t, items, 1)
		require.Equal(t, "42", string(items[0]))
REDACTED)

	t.Run("input_bool", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":trueREDACTED`))
	REDACTED
		require.True(t, exists)
		require.Len(t, items, 1)
		require.Equal(t, "true", string(items[0]))
REDACTED)

	t.Run("input_null", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":nullREDACTED`))
	REDACTED
		require.True(t, exists)
		require.Len(t, items, 1)
		require.Equal(t, "null", string(items[0]))
REDACTED)

	t.Run("input_invalid_array_json", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":[REDACTED`))
	REDACTED
		require.True(t, exists)
		require.Nil(t, items)
REDACTED)
REDACTED

func TestShouldKeepIngressPreviousResponseID(t *testing.T) {
	t.Parallel()

	previousPayload := []byte(`{
		"type":"response.create",
		"model":"gpt-5.1",
		"store":false,
		"tools":[{"type":"function","name":"tool_a"REDACTED],
		"input":[{"type":"input_text","text":"hello"REDACTED]
REDACTED`)
	currentStrictPayload := []byte(`{
		"type":"response.create",
		"model":"gpt-5.1",
		"store":false,
		"tools":[{"name":"tool_a","type":"function"REDACTED],
		"previous_response_id":"resp_turn_1",
		"input":[{"text":"hello","type":"input_text"REDACTED,{"type":"input_text","text":"world"REDACTED]
REDACTED`)

	t.Run("strict_incremental_keep", func(t *testing.T) {
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, currentStrictPayload, "resp_turn_1", false)
	REDACTED
		require.True(t, keep)
		require.Equal(t, "strict_incremental_ok", reason)
REDACTED)

	t.Run("missing_previous_response_id", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1","input":[]REDACTED`)
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, payload, "resp_turn_1", false)
	REDACTED
		require.False(t, keep)
		require.Equal(t, "missing_previous_response_id", reason)
REDACTED)

	t.Run("missing_last_turn_response_id", func(t *testing.T) {
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, currentStrictPayload, "", false)
	REDACTED
		require.False(t, keep)
		require.Equal(t, "missing_last_turn_response_id", reason)
REDACTED)

	t.Run("previous_response_id_mismatch", func(t *testing.T) {
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, currentStrictPayload, "resp_turn_other", false)
	REDACTED
		require.False(t, keep)
		require.Equal(t, "previous_response_id_mismatch", reason)
REDACTED)

	t.Run("missing_previous_turn_payload", func(t *testing.T) {
		keep, reason, err := shouldKeepIngressPreviousResponseID(nil, currentStrictPayload, "resp_turn_1", false)
	REDACTED
		require.False(t, keep)
		require.Equal(t, "missing_previous_turn_payload", reason)
REDACTED)

	t.Run("non_input_changed", func(t *testing.T) {
		payload := []byte(`{
			"type":"response.create",
			"model":"gpt-5.1-mini",
			"store":false,
			"tools":[{"type":"function","name":"tool_a"REDACTED],
			"previous_response_id":"resp_turn_1",
			"input":[{"type":"input_text","text":"hello"REDACTED,{"type":"input_text","text":"world"REDACTED]
	REDACTED`)
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, payload, "resp_turn_1", false)
	REDACTED
		require.False(t, keep)
		require.Equal(t, "non_input_changed", reason)
REDACTED)

	t.Run("delta_input_keeps_previous_response_id", func(t *testing.T) {
		payload := []byte(`{
			"type":"response.create",
			"model":"gpt-5.1",
			"store":false,
			"tools":[{"type":"function","name":"tool_a"REDACTED],
			"previous_response_id":"resp_turn_1",
			"input":[{"type":"input_text","text":"different"REDACTED]
	REDACTED`)
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, payload, "resp_turn_1", false)
	REDACTED
		require.True(t, keep)
		require.Equal(t, "strict_incremental_ok", reason)
REDACTED)

	t.Run("function_call_output_keeps_previous_response_id", func(t *testing.T) {
		payload := []byte(`{
			"type":"response.create",
			"model":"gpt-5.1",
			"store":false,
			"previous_response_id":"resp_external",
			"input":[{"type":"function_call_output","call_id":"call_1","output":"ok"REDACTED]
	REDACTED`)
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, payload, "resp_turn_1", true)
	REDACTED
		require.True(t, keep)
		require.Equal(t, "has_function_call_output", reason)
REDACTED)

	t.Run("non_input_compare_error", func(t *testing.T) {
		keep, reason, err := shouldKeepIngressPreviousResponseID([]byte(`[]`), currentStrictPayload, "resp_turn_1", false)
	REDACTED
		require.False(t, keep)
		require.Equal(t, "non_input_compare_error", reason)
REDACTED)

	t.Run("current_payload_compare_error", func(t *testing.T) {
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, []byte(`{"previous_response_id":"resp_turn_1","input":[REDACTED`), "resp_turn_1", false)
	REDACTED
		require.False(t, keep)
		require.Equal(t, "non_input_compare_error", reason)
REDACTED)
REDACTED

func TestBuildOpenAIWSReplayInputSequence(t *testing.T) {
	t.Parallel()

	lastFull := []json.RawMessage{
		json.RawMessage(`{"type":"input_text","text":"hello"REDACTED`),
REDACTED

	t.Run("no_previous_response_id_use_current", func(t *testing.T) {
		items, exists, err := buildOpenAIWSReplayInputSequence(
			lastFull,
			true,
			[]byte(`{"input":[{"type":"input_text","text":"new"REDACTED]REDACTED`),
			false,
		)
	REDACTED
		require.True(t, exists)
		require.Len(t, items, 1)
		require.Equal(t, "new", gjson.GetBytes(items[0], "text").String())
REDACTED)

	t.Run("previous_response_id_delta_append", func(t *testing.T) {
		items, exists, err := buildOpenAIWSReplayInputSequence(
			lastFull,
			true,
			[]byte(`{"previous_response_id":"resp_1","input":[{"type":"input_text","text":"world"REDACTED]REDACTED`),
			true,
		)
	REDACTED
		require.True(t, exists)
		require.Len(t, items, 2)
		require.Equal(t, "hello", gjson.GetBytes(items[0], "text").String())
		require.Equal(t, "world", gjson.GetBytes(items[1], "text").String())
REDACTED)

	t.Run("previous_response_id_full_input_replace", func(t *testing.T) {
		items, exists, err := buildOpenAIWSReplayInputSequence(
			lastFull,
			true,
			[]byte(`{"previous_response_id":"resp_1","input":[{"type":"input_text","text":"hello"REDACTED,{"type":"input_text","text":"world"REDACTED]REDACTED`),
			true,
		)
	REDACTED
		require.True(t, exists)
		require.Len(t, items, 2)
		require.Equal(t, "hello", gjson.GetBytes(items[0], "text").String())
		require.Equal(t, "world", gjson.GetBytes(items[1], "text").String())
REDACTED)
REDACTED

func TestOpenAIWSRawPayloadHasToolCallOutput(t *testing.T) {
	t.Parallel()

	for _, typ := range []string{
		"function_call_output",
		"tool_search_output",
		"custom_tool_call_output",
		"mcp_tool_call_output",
REDACTED {
		typ := typ
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			payload := []byte(`{"input":[{"type":"` + typ + `","call_id":"call_1","output":"ok"REDACTED]REDACTED`)
			require.True(t, openAIWSRawPayloadHasToolCallOutput(payload))
	REDACTED)
REDACTED

	t.Run("object_input", func(t *testing.T) {
		t.Parallel()
		payload := []byte(`{"input":{"type":"tool_search_output","call_id":"call_1","output":"ok"REDACTEDREDACTED`)
		require.True(t, openAIWSRawPayloadHasToolCallOutput(payload))
REDACTED)

	t.Run("non_tool_output", func(t *testing.T) {
		t.Parallel()
		payload := []byte(`{"input":[{"type":"input_text","text":"hello"REDACTED]REDACTED`)
		require.False(t, openAIWSRawPayloadHasToolCallOutput(payload))
REDACTED)
REDACTED

func TestSetOpenAIWSPayloadInputSequence(t *testing.T) {
	t.Parallel()

	t.Run("set_items", func(t *testing.T) {
		original := []byte(`{"type":"response.create","previous_response_id":"resp_1"REDACTED`)
		items := []json.RawMessage{
			json.RawMessage(`{"type":"input_text","text":"hello"REDACTED`),
			json.RawMessage(`{"type":"input_text","text":"world"REDACTED`),
	REDACTED
		updated, err := setOpenAIWSPayloadInputSequence(original, items, true)
	REDACTED
		require.Equal(t, "hello", gjson.GetBytes(updated, "input.0.text").String())
		require.Equal(t, "world", gjson.GetBytes(updated, "input.1.text").String())
REDACTED)

	t.Run("preserve_empty_array_not_null", func(t *testing.T) {
		original := []byte(`{"type":"response.create","previous_response_id":"resp_1"REDACTED`)
		updated, err := setOpenAIWSPayloadInputSequence(original, nil, true)
	REDACTED
		require.True(t, gjson.GetBytes(updated, "input").IsArray())
		require.Len(t, gjson.GetBytes(updated, "input").Array(), 0)
		require.False(t, gjson.GetBytes(updated, "input").Type == gjson.Null)
REDACTED)
REDACTED

func TestCloneOpenAIWSRawMessages(t *testing.T) {
	t.Parallel()

	t.Run("nil_slice", func(t *testing.T) {
		cloned := cloneOpenAIWSRawMessages(nil)
		require.Nil(t, cloned)
REDACTED)

	t.Run("empty_slice", func(t *testing.T) {
		items := make([]json.RawMessage, 0)
		cloned := cloneOpenAIWSRawMessages(items)
		require.NotNil(t, cloned)
		require.Len(t, cloned, 0)
REDACTED)
REDACTED

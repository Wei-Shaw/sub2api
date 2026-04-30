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
	}{
		{name: "nil", err: nil, want: false},
		{name: "io_eof", err: io.EOF, want: true},
		{name: "net_closed", err: net.ErrClosed, want: true},
		{name: "context_canceled", err: context.Canceled, want: true},
		{name: "ws_normal_closure", err: coderws.CloseError{Code: coderws.StatusNormalClosure}, want: true},
		{name: "ws_going_away", err: coderws.CloseError{Code: coderws.StatusGoingAway}, want: true},
		{name: "ws_no_status", err: coderws.CloseError{Code: coderws.StatusNoStatusRcvd}, want: true},
		{name: "ws_abnormal_1006", err: coderws.CloseError{Code: coderws.StatusAbnormalClosure}, want: true},
		{name: "ws_policy_violation", err: coderws.CloseError{Code: coderws.StatusPolicyViolation}, want: false},
		{name: "wrapped_eof_message", err: errors.New("failed to get reader: failed to read frame header: EOF"), want: true},
		{name: "connection_reset_by_peer", err: errors.New("failed to read frame header: read tcp 127.0.0.1:1234->127.0.0.1:5678: read: connection reset by peer"), want: true},
		{name: "broken_pipe", err: errors.New("write tcp 127.0.0.1:1234->127.0.0.1:5678: write: broken pipe"), want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isOpenAIWSClientDisconnectError(tt.err))
		})
	}
}

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
}

func TestOpenAIWSIngressPreviousResponseRecoveryEnabled(t *testing.T) {
	t.Parallel()

	var nilService *OpenAIGatewayService
	require.True(t, nilService.openAIWSIngressPreviousResponseRecoveryEnabled(), "nil service should default to enabled")

	svcWithNilCfg := &OpenAIGatewayService{}
	require.True(t, svcWithNilCfg.openAIWSIngressPreviousResponseRecoveryEnabled(), "nil config should default to enabled")

	svc := &OpenAIGatewayService{
		cfg: &config.Config{},
	}
	require.False(t, svc.openAIWSIngressPreviousResponseRecoveryEnabled(), "explicit config default should be false")

	svc.cfg.Gateway.OpenAIWS.IngressPreviousResponseRecoveryEnabled = true
	require.True(t, svc.openAIWSIngressPreviousResponseRecoveryEnabled())
}

func TestDropPreviousResponseIDFromRawPayload(t *testing.T) {
	t.Parallel()

	t.Run("empty_payload", func(t *testing.T) {
		updated, removed, err := dropPreviousResponseIDFromRawPayload(nil)
		require.NoError(t, err)
		require.False(t, removed)
		require.Empty(t, updated)
	})

	t.Run("payload_without_previous_response_id", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1"}`)
		updated, removed, err := dropPreviousResponseIDFromRawPayload(payload)
		require.NoError(t, err)
		require.False(t, removed)
		require.Equal(t, string(payload), string(updated))
	})

	t.Run("normal_delete_success", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_abc"}`)
		updated, removed, err := dropPreviousResponseIDFromRawPayload(payload)
		require.NoError(t, err)
		require.True(t, removed)
		require.False(t, gjson.GetBytes(updated, "previous_response_id").Exists())
	})

	t.Run("duplicate_keys_are_removed", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","previous_response_id":"resp_a","input":[],"previous_response_id":"resp_b"}`)
		updated, removed, err := dropPreviousResponseIDFromRawPayload(payload)
		require.NoError(t, err)
		require.True(t, removed)
		require.False(t, gjson.GetBytes(updated, "previous_response_id").Exists())
	})

	t.Run("nil_delete_fn_uses_default_delete_logic", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_abc"}`)
		updated, removed, err := dropPreviousResponseIDFromRawPayloadWithDeleteFn(payload, nil)
		require.NoError(t, err)
		require.True(t, removed)
		require.False(t, gjson.GetBytes(updated, "previous_response_id").Exists())
	})

	t.Run("delete_error", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_abc"}`)
		updated, removed, err := dropPreviousResponseIDFromRawPayloadWithDeleteFn(payload, func(_ []byte, _ string) ([]byte, error) {
			return nil, errors.New("delete failed")
		})
		require.Error(t, err)
		require.False(t, removed)
		require.Equal(t, string(payload), string(updated))
	})

	t.Run("malformed_json_is_still_best_effort_deleted", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","previous_response_id":"resp_abc"`)
		require.True(t, gjson.GetBytes(payload, "previous_response_id").Exists())

		updated, removed, err := dropPreviousResponseIDFromRawPayload(payload)
		require.NoError(t, err)
		require.True(t, removed)
		require.False(t, gjson.GetBytes(updated, "previous_response_id").Exists())
	})
}

func TestAlignStoreDisabledPreviousResponseID(t *testing.T) {
	t.Parallel()

	t.Run("empty_payload", func(t *testing.T) {
		updated, changed, err := alignStoreDisabledPreviousResponseID(nil, "resp_target")
		require.NoError(t, err)
		require.False(t, changed)
		require.Empty(t, updated)
	})

	t.Run("empty_expected", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","previous_response_id":"resp_old"}`)
		updated, changed, err := alignStoreDisabledPreviousResponseID(payload, "")
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, string(payload), string(updated))
	})

	t.Run("missing_previous_response_id", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1"}`)
		updated, changed, err := alignStoreDisabledPreviousResponseID(payload, "resp_target")
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, string(payload), string(updated))
	})

	t.Run("already_aligned", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","previous_response_id":"resp_target"}`)
		updated, changed, err := alignStoreDisabledPreviousResponseID(payload, "resp_target")
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, "resp_target", gjson.GetBytes(updated, "previous_response_id").String())
	})

	t.Run("mismatch_rewrites_to_expected", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","previous_response_id":"resp_old","input":[]}`)
		updated, changed, err := alignStoreDisabledPreviousResponseID(payload, "resp_target")
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, "resp_target", gjson.GetBytes(updated, "previous_response_id").String())
	})

	t.Run("duplicate_keys_rewrites_to_single_expected", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","previous_response_id":"resp_old_1","input":[],"previous_response_id":"resp_old_2"}`)
		updated, changed, err := alignStoreDisabledPreviousResponseID(payload, "resp_target")
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, "resp_target", gjson.GetBytes(updated, "previous_response_id").String())
	})
}

func TestSetPreviousResponseIDToRawPayload(t *testing.T) {
	t.Parallel()

	t.Run("empty_payload", func(t *testing.T) {
		updated, err := setPreviousResponseIDToRawPayload(nil, "resp_target")
		require.NoError(t, err)
		require.Empty(t, updated)
	})

	t.Run("empty_previous_response_id", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1"}`)
		updated, err := setPreviousResponseIDToRawPayload(payload, "")
		require.NoError(t, err)
		require.Equal(t, string(payload), string(updated))
	})

	t.Run("set_previous_response_id_when_missing", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1"}`)
		updated, err := setPreviousResponseIDToRawPayload(payload, "resp_target")
		require.NoError(t, err)
		require.Equal(t, "resp_target", gjson.GetBytes(updated, "previous_response_id").String())
		require.Equal(t, "gpt-5.1", gjson.GetBytes(updated, "model").String())
	})

	t.Run("overwrite_existing_previous_response_id", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_old"}`)
		updated, err := setPreviousResponseIDToRawPayload(payload, "resp_new")
		require.NoError(t, err)
		require.Equal(t, "resp_new", gjson.GetBytes(updated, "previous_response_id").String())
	})
}

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
	}{
		{
			name:             "infer_when_all_conditions_match",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: true},
			expectedPrevious: "resp_1",
			want:             true,
		},
		{
			name:             "skip_when_store_enabled",
			storeDisabled:    false,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: true},
			expectedPrevious: "resp_1",
			want:             false,
		},
		{
			name:             "skip_on_first_turn",
			storeDisabled:    true,
			turn:             1,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: true},
			expectedPrevious: "resp_1",
			want:             false,
		},
		{
			name:             "skip_without_function_call_output",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{},
			expectedPrevious: "resp_1",
			want:             false,
		},
		{
			name:                    "skip_when_request_already_has_previous_response_id",
			storeDisabled:           true,
			turn:                    2,
			signals:                 ToolContinuationSignals{HasFunctionCallOutput: true},
			currentPreviousResponse: "resp_client",
			expectedPrevious:        "resp_1",
			want:                    false,
		},
		{
			name:             "skip_when_last_turn_response_id_missing",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: true},
			expectedPrevious: "",
			want:             false,
		},
		{
			name:             "trim_whitespace_before_judgement",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: true},
			expectedPrevious: "   resp_2   ",
			want:             true,
		},
		{
			name:             "skip_when_tool_call_context_already_present",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: true, HasToolCallContext: true},
			expectedPrevious: "resp_2",
			want:             false,
		},
		{
			name:             "skip_when_item_reference_already_covers_all_call_ids",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: true, HasItemReferenceForAllCallIDs: true},
			expectedPrevious: "resp_2",
			want:             false,
		},
		{
			name:             "skip_when_function_call_output_missing_call_id",
			storeDisabled:    true,
			turn:             2,
			signals:          ToolContinuationSignals{HasFunctionCallOutput: true, HasFunctionCallOutputMissingCallID: true},
			expectedPrevious: "resp_2",
			want:             false,
		},
	}

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
		})
	}
}

func TestOpenAIWSInputIsPrefixExtended(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		previous  []byte
		current   []byte
		want      bool
		expectErr bool
	}{
		{
			name:     "both_missing_input",
			previous: []byte(`{"type":"response.create","model":"gpt-5.1"}`),
			current:  []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_1"}`),
			want:     true,
		},
		{
			name:     "previous_missing_current_empty_array",
			previous: []byte(`{"type":"response.create","model":"gpt-5.1"}`),
			current:  []byte(`{"type":"response.create","model":"gpt-5.1","input":[]}`),
			want:     true,
		},
		{
			name:     "previous_missing_current_non_empty_array",
			previous: []byte(`{"type":"response.create","model":"gpt-5.1"}`),
			current:  []byte(`{"type":"response.create","model":"gpt-5.1","input":[{"type":"input_text","text":"hello"}]}`),
			want:     false,
		},
		{
			name:     "array_prefix_match",
			previous: []byte(`{"input":[{"type":"input_text","text":"hello"}]}`),
			current:  []byte(`{"input":[{"text":"hello","type":"input_text"},{"type":"input_text","text":"world"}]}`),
			want:     true,
		},
		{
			name:     "array_prefix_mismatch",
			previous: []byte(`{"input":[{"type":"input_text","text":"hello"}]}`),
			current:  []byte(`{"input":[{"type":"input_text","text":"different"}]}`),
			want:     false,
		},
		{
			name:     "current_shorter_than_previous",
			previous: []byte(`{"input":[{"type":"input_text","text":"a"},{"type":"input_text","text":"b"}]}`),
			current:  []byte(`{"input":[{"type":"input_text","text":"a"}]}`),
			want:     false,
		},
		{
			name:     "previous_has_input_current_missing",
			previous: []byte(`{"input":[{"type":"input_text","text":"a"}]}`),
			current:  []byte(`{"model":"gpt-5.1"}`),
			want:     false,
		},
		{
			name:     "input_string_treated_as_single_item",
			previous: []byte(`{"input":"hello"}`),
			current:  []byte(`{"input":"hello"}`),
			want:     true,
		},
		{
			name:      "current_invalid_input_json",
			previous:  []byte(`{"input":[]}`),
			current:   []byte(`{"input":[}`),
			expectErr: true,
		},
		{
			name:      "invalid_input_json",
			previous:  []byte(`{"input":[}`),
			current:   []byte(`{"input":[]}`),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := openAIWSInputIsPrefixExtended(tt.previous, tt.current)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeOpenAIWSJSONForCompare(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeOpenAIWSJSONForCompare([]byte(`{"b":2,"a":1}`))
	require.NoError(t, err)
	require.Equal(t, `{"a":1,"b":2}`, string(normalized))

	_, err = normalizeOpenAIWSJSONForCompare([]byte("   "))
	require.Error(t, err)

	_, err = normalizeOpenAIWSJSONForCompare([]byte(`{"a":`))
	require.Error(t, err)
}

func TestNormalizeOpenAIWSJSONForCompareOrRaw(t *testing.T) {
	t.Parallel()

	require.Equal(t, `{"a":1,"b":2}`, string(normalizeOpenAIWSJSONForCompareOrRaw([]byte(`{"b":2,"a":1}`))))
	require.Equal(t, `{"a":`, string(normalizeOpenAIWSJSONForCompareOrRaw([]byte(`{"a":`))))
}

func TestNormalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID(
		[]byte(`{"model":"gpt-5.1","input":[1],"previous_response_id":"resp_x","metadata":{"b":2,"a":1}}`),
	)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(normalized, "input").Exists())
	require.False(t, gjson.GetBytes(normalized, "previous_response_id").Exists())
	require.Equal(t, float64(1), gjson.GetBytes(normalized, "metadata.a").Float())

	_, err = normalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID(nil)
	require.Error(t, err)

	_, err = normalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID([]byte(`[]`))
	require.Error(t, err)
}

func TestOpenAIWSExtractNormalizedInputSequence(t *testing.T) {
	t.Parallel()

	t.Run("empty_payload", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence(nil)
		require.NoError(t, err)
		require.False(t, exists)
		require.Nil(t, items)
	})

	t.Run("input_missing", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"type":"response.create"}`))
		require.NoError(t, err)
		require.False(t, exists)
		require.Nil(t, items)
	})

	t.Run("input_array", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":[{"type":"input_text","text":"hello"}]}`))
		require.NoError(t, err)
		require.True(t, exists)
		require.Len(t, items, 1)
	})

	t.Run("input_object", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":{"type":"input_text","text":"hello"}}`))
		require.NoError(t, err)
		require.True(t, exists)
		require.Len(t, items, 1)
	})

	t.Run("input_string", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":"hello"}`))
		require.NoError(t, err)
		require.True(t, exists)
		require.Len(t, items, 1)
		require.Equal(t, `"hello"`, string(items[0]))
	})

	t.Run("input_number", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":42}`))
		require.NoError(t, err)
		require.True(t, exists)
		require.Len(t, items, 1)
		require.Equal(t, "42", string(items[0]))
	})

	t.Run("input_bool", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":true}`))
		require.NoError(t, err)
		require.True(t, exists)
		require.Len(t, items, 1)
		require.Equal(t, "true", string(items[0]))
	})

	t.Run("input_null", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":null}`))
		require.NoError(t, err)
		require.True(t, exists)
		require.Len(t, items, 1)
		require.Equal(t, "null", string(items[0]))
	})

	t.Run("input_invalid_array_json", func(t *testing.T) {
		items, exists, err := openAIWSExtractNormalizedInputSequence([]byte(`{"input":[}`))
		require.Error(t, err)
		require.True(t, exists)
		require.Nil(t, items)
	})
}

func TestShouldKeepIngressPreviousResponseID(t *testing.T) {
	t.Parallel()

	previousPayload := []byte(`{
		"type":"response.create",
		"model":"gpt-5.1",
		"store":false,
		"tools":[{"type":"function","name":"tool_a"}],
		"input":[{"type":"input_text","text":"hello"}]
	}`)
	currentStrictPayload := []byte(`{
		"type":"response.create",
		"model":"gpt-5.1",
		"store":false,
		"tools":[{"name":"tool_a","type":"function"}],
		"previous_response_id":"resp_turn_1",
		"input":[{"text":"hello","type":"input_text"},{"type":"input_text","text":"world"}]
	}`)

	t.Run("strict_incremental_keep", func(t *testing.T) {
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, currentStrictPayload, "resp_turn_1", false)
		require.NoError(t, err)
		require.True(t, keep)
		require.Equal(t, "strict_incremental_ok", reason)
	})

	t.Run("missing_previous_response_id", func(t *testing.T) {
		payload := []byte(`{"type":"response.create","model":"gpt-5.1","input":[]}`)
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, payload, "resp_turn_1", false)
		require.NoError(t, err)
		require.False(t, keep)
		require.Equal(t, "missing_previous_response_id", reason)
	})

	t.Run("missing_last_turn_response_id", func(t *testing.T) {
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, currentStrictPayload, "", false)
		require.NoError(t, err)
		require.False(t, keep)
		require.Equal(t, "missing_last_turn_response_id", reason)
	})

	t.Run("previous_response_id_mismatch", func(t *testing.T) {
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, currentStrictPayload, "resp_turn_other", false)
		require.NoError(t, err)
		require.False(t, keep)
		require.Equal(t, "previous_response_id_mismatch", reason)
	})

	t.Run("missing_previous_turn_payload", func(t *testing.T) {
		keep, reason, err := shouldKeepIngressPreviousResponseID(nil, currentStrictPayload, "resp_turn_1", false)
		require.NoError(t, err)
		require.False(t, keep)
		require.Equal(t, "missing_previous_turn_payload", reason)
	})

	t.Run("non_input_changed", func(t *testing.T) {
		payload := []byte(`{
			"type":"response.create",
			"model":"gpt-5.1-mini",
			"store":false,
			"tools":[{"type":"function","name":"tool_a"}],
			"previous_response_id":"resp_turn_1",
			"input":[{"type":"input_text","text":"hello"},{"type":"input_text","text":"world"}]
		}`)
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, payload, "resp_turn_1", false)
		require.NoError(t, err)
		require.False(t, keep)
		require.Equal(t, "non_input_changed", reason)
	})

	t.Run("delta_input_keeps_previous_response_id", func(t *testing.T) {
		payload := []byte(`{
			"type":"response.create",
			"model":"gpt-5.1",
			"store":false,
			"tools":[{"type":"function","name":"tool_a"}],
			"previous_response_id":"resp_turn_1",
			"input":[{"type":"input_text","text":"different"}]
		}`)
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, payload, "resp_turn_1", false)
		require.NoError(t, err)
		require.True(t, keep)
		require.Equal(t, "strict_incremental_ok", reason)
	})

	t.Run("function_call_output_keeps_previous_response_id", func(t *testing.T) {
		payload := []byte(`{
			"type":"response.create",
			"model":"gpt-5.1",
			"store":false,
			"previous_response_id":"resp_external",
			"input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]
		}`)
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, payload, "resp_turn_1", true)
		require.NoError(t, err)
		require.True(t, keep)
		require.Equal(t, "has_function_call_output", reason)
	})

	t.Run("non_input_compare_error", func(t *testing.T) {
		keep, reason, err := shouldKeepIngressPreviousResponseID([]byte(`[]`), currentStrictPayload, "resp_turn_1", false)
		require.Error(t, err)
		require.False(t, keep)
		require.Equal(t, "non_input_compare_error", reason)
	})

	t.Run("current_payload_compare_error", func(t *testing.T) {
		keep, reason, err := shouldKeepIngressPreviousResponseID(previousPayload, []byte(`{"previous_response_id":"resp_turn_1","input":[}`), "resp_turn_1", false)
		require.Error(t, err)
		require.False(t, keep)
		require.Equal(t, "non_input_compare_error", reason)
	})
}

func TestBuildOpenAIWSReplayInputSequence(t *testing.T) {
	t.Parallel()

	lastFull := []json.RawMessage{
		json.RawMessage(`{"type":"input_text","text":"hello"}`),
	}

	t.Run("no_previous_response_id_use_current", func(t *testing.T) {
		items, exists, err := buildOpenAIWSReplayInputSequence(
			lastFull,
			true,
			[]byte(`{"input":[{"type":"input_text","text":"new"}]}`),
			false,
		)
		require.NoError(t, err)
		require.True(t, exists)
		require.Len(t, items, 1)
		require.Equal(t, "new", gjson.GetBytes(items[0], "text").String())
	})

	t.Run("previous_response_id_delta_append", func(t *testing.T) {
		items, exists, err := buildOpenAIWSReplayInputSequence(
			lastFull,
			true,
			[]byte(`{"previous_response_id":"resp_1","input":[{"type":"input_text","text":"world"}]}`),
			true,
		)
		require.NoError(t, err)
		require.True(t, exists)
		require.Len(t, items, 2)
		require.Equal(t, "hello", gjson.GetBytes(items[0], "text").String())
		require.Equal(t, "world", gjson.GetBytes(items[1], "text").String())
	})

	t.Run("previous_response_id_full_input_replace", func(t *testing.T) {
		items, exists, err := buildOpenAIWSReplayInputSequence(
			lastFull,
			true,
			[]byte(`{"previous_response_id":"resp_1","input":[{"type":"input_text","text":"hello"},{"type":"input_text","text":"world"}]}`),
			true,
		)
		require.NoError(t, err)
		require.True(t, exists)
		require.Len(t, items, 2)
		require.Equal(t, "hello", gjson.GetBytes(items[0], "text").String())
		require.Equal(t, "world", gjson.GetBytes(items[1], "text").String())
	})
}

func TestSetOpenAIWSPayloadInputSequence(t *testing.T) {
	t.Parallel()

	t.Run("set_items", func(t *testing.T) {
		original := []byte(`{"type":"response.create","previous_response_id":"resp_1"}`)
		items := []json.RawMessage{
			json.RawMessage(`{"type":"input_text","text":"hello"}`),
			json.RawMessage(`{"type":"input_text","text":"world"}`),
		}
		updated, err := setOpenAIWSPayloadInputSequence(original, items, true)
		require.NoError(t, err)
		require.Equal(t, "hello", gjson.GetBytes(updated, "input.0.text").String())
		require.Equal(t, "world", gjson.GetBytes(updated, "input.1.text").String())
	})

	t.Run("preserve_empty_array_not_null", func(t *testing.T) {
		original := []byte(`{"type":"response.create","previous_response_id":"resp_1"}`)
		updated, err := setOpenAIWSPayloadInputSequence(original, nil, true)
		require.NoError(t, err)
		require.True(t, gjson.GetBytes(updated, "input").IsArray())
		require.Len(t, gjson.GetBytes(updated, "input").Array(), 0)
		require.False(t, gjson.GetBytes(updated, "input").Type == gjson.Null)
	})
}

func TestCloneOpenAIWSRawMessages(t *testing.T) {
	t.Parallel()

	t.Run("nil_slice", func(t *testing.T) {
		cloned := cloneOpenAIWSRawMessages(nil)
		require.Nil(t, cloned)
	})

	t.Run("empty_slice", func(t *testing.T) {
		items := make([]json.RawMessage, 0)
		cloned := cloneOpenAIWSRawMessages(items)
		require.NotNil(t, cloned)
		require.Len(t, cloned, 0)
	})
}

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNeedsToolContinuationSignals(t *testing.T) {
	// 覆盖所有触发续链的信号来源，确保判定逻辑完整。
	cases := []struct {
		name string
		body map[string]any
		want bool
REDACTED{
		{name: "nil", body: nil, want: falseREDACTED,
		{name: "previous_response_id", body: map[string]any{"previous_response_id": "resp_1"REDACTED, want: trueREDACTED,
		{name: "previous_response_id_blank", body: map[string]any{"previous_response_id": "  "REDACTED, want: falseREDACTED,
		{name: "function_call_output", body: map[string]any{"input": []any{map[string]any{"type": "function_call_output"REDACTEDREDACTEDREDACTED, want: trueREDACTED,
		{name: "tool_search_output", body: map[string]any{"input": []any{map[string]any{"type": "tool_search_output"REDACTEDREDACTEDREDACTED, want: trueREDACTED,
		{name: "custom_tool_call_output", body: map[string]any{"input": []any{map[string]any{"type": "custom_tool_call_output"REDACTEDREDACTEDREDACTED, want: trueREDACTED,
		{name: "mcp_tool_call_output", body: map[string]any{"input": []any{map[string]any{"type": "mcp_tool_call_output"REDACTEDREDACTEDREDACTED, want: trueREDACTED,
		{name: "item_reference", body: map[string]any{"input": []any{map[string]any{"type": "item_reference"REDACTEDREDACTEDREDACTED, want: trueREDACTED,
		{name: "tools", body: map[string]any{"tools": []any{map[string]any{"type": "function"REDACTEDREDACTEDREDACTED, want: trueREDACTED,
		{name: "tools_empty", body: map[string]any{"tools": []any{REDACTEDREDACTED, want: falseREDACTED,
		{name: "tools_invalid", body: map[string]any{"tools": "bad"REDACTED, want: falseREDACTED,
		{name: "tool_choice", body: map[string]any{"tool_choice": "auto"REDACTED, want: trueREDACTED,
		{name: "tool_choice_object", body: map[string]any{"tool_choice": map[string]any{"type": "function"REDACTEDREDACTED, want: trueREDACTED,
		{name: "tool_choice_empty_object", body: map[string]any{"tool_choice": map[string]any{REDACTEDREDACTED, want: falseREDACTED,
		{name: "none", body: map[string]any{"input": []any{map[string]any{"type": "text", "text": "hi"REDACTEDREDACTEDREDACTED, want: falseREDACTED,
REDACTED

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NeedsToolContinuation(tt.body))
	REDACTED)
REDACTED
REDACTED

func TestHasFunctionCallOutput(t *testing.T) {
	// 仅当 input 中存在 function_call_output 才视为续链输出。
	require.False(t, HasFunctionCallOutput(nil))
	require.True(t, HasFunctionCallOutput(map[string]any{
		"input": []any{map[string]any{"type": "function_call_output"REDACTEDREDACTED,
REDACTED))
	require.False(t, HasFunctionCallOutput(map[string]any{
		"input": "text",
REDACTED))
REDACTED

func TestHasToolCallContext(t *testing.T) {
	// tool_call/function_call 必须包含 call_id，才能作为可关联上下文。
	require.False(t, HasToolCallContext(nil))
	require.True(t, HasToolCallContext(map[string]any{
		"input": []any{map[string]any{"type": "tool_call", "call_id": "call_1"REDACTEDREDACTED,
REDACTED))
	require.True(t, HasToolCallContext(map[string]any{
		"input": []any{map[string]any{"type": "function_call", "call_id": "call_2"REDACTEDREDACTED,
REDACTED))
	require.False(t, HasToolCallContext(map[string]any{
		"input": []any{map[string]any{"type": "tool_call"REDACTEDREDACTED,
REDACTED))
REDACTED

func TestFunctionCallOutputCallIDs(t *testing.T) {
	// 仅提取非空 call_id，去重后返回。
	require.Empty(t, FunctionCallOutputCallIDs(nil))
	callIDs := FunctionCallOutputCallIDs(map[string]any{
		"input": []any{
			map[string]any{"type": "function_call_output", "call_id": "call_1"REDACTED,
			map[string]any{"type": "function_call_output", "call_id": ""REDACTED,
			map[string]any{"type": "function_call_output", "call_id": "call_1"REDACTED,
	REDACTED,
REDACTED)
	require.ElementsMatch(t, []string{"call_1"REDACTED, callIDs)
REDACTED

func TestHasFunctionCallOutputMissingCallID(t *testing.T) {
	require.False(t, HasFunctionCallOutputMissingCallID(nil))
	require.True(t, HasFunctionCallOutputMissingCallID(map[string]any{
		"input": []any{map[string]any{"type": "function_call_output"REDACTEDREDACTED,
REDACTED))
	require.False(t, HasFunctionCallOutputMissingCallID(map[string]any{
		"input": []any{map[string]any{"type": "function_call_output", "call_id": "call_1"REDACTEDREDACTED,
REDACTED))
REDACTED

func TestHasItemReferenceForCallIDs(t *testing.T) {
	// item_reference 需要覆盖所有 call_id 才视为可关联上下文。
	require.False(t, HasItemReferenceForCallIDs(nil, []string{"call_1"REDACTED))
	require.False(t, HasItemReferenceForCallIDs(map[string]any{REDACTED, []string{"call_1"REDACTED))
	req := map[string]any{
		"input": []any{
			map[string]any{"type": "item_reference", "id": "call_1"REDACTED,
			map[string]any{"type": "item_reference", "id": "call_2"REDACTED,
	REDACTED,
REDACTED
	require.True(t, HasItemReferenceForCallIDs(req, []string{"call_1"REDACTED))
	require.True(t, HasItemReferenceForCallIDs(req, []string{"call_1", "call_2"REDACTED))
	require.False(t, HasItemReferenceForCallIDs(req, []string{"call_1", "call_3"REDACTED))
REDACTED

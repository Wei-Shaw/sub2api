package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAIResponsesLegacyIngressMessagesOnly(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","stream":false,"messages":[{"role":"system","content":"repo policy"REDACTED,{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"hello\"REDACTED"REDACTEDREDACTED]REDACTED,{"role":"tool","tool_call_id":"call_1","content":"ok"REDACTED],"previous_response_id":"resp_stale"REDACTED`)

	normalized, changed, err := normalizeOpenAIResponsesLegacyIngress(body)
REDACTED
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "messages").Exists())
	require.False(t, gjson.GetBytes(normalized, "previous_response_id").Exists())
	require.Equal(t, "system", gjson.GetBytes(normalized, "input.0.role").String())
	require.Equal(t, "repo policy", gjson.GetBytes(normalized, "input.0.content").String())
	require.Equal(t, "function_call", gjson.GetBytes(normalized, "input.1.type").String())
	require.Equal(t, "call_1", gjson.GetBytes(normalized, "input.1.call_id").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(normalized, "input.2.type").String())
	require.Equal(t, "call_1", gjson.GetBytes(normalized, "input.2.call_id").String())
	require.Equal(t, gjson.False, gjson.GetBytes(normalized, "stream").Type)
REDACTED

func TestNormalizeOpenAIResponsesLegacyIngressConvertsChatTopLevelFields(t *testing.T) {
	body := []byte(`{"model":"gpt-4.1","messages":[{"role":"user","content":"weather?"REDACTED],"tools":[{"type":"function","function":{"name":"lookup","description":"Lookup weather","parameters":{"type":"object"REDACTEDREDACTEDREDACTED],"tool_choice":{"type":"function","function":{"name":"lookup"REDACTEDREDACTED,"max_tokens":64,"reasoning_effort":"high","service_tier":"flex","temperature":0.2,"top_p":0.7REDACTED`)

	normalized, changed, err := normalizeOpenAIResponsesLegacyIngress(body)
REDACTED
	require.True(t, changed)
	require.Equal(t, "user", gjson.GetBytes(normalized, "input.0.role").String())
	require.Equal(t, "function", gjson.GetBytes(normalized, "tools.0.type").String())
	require.Equal(t, "lookup", gjson.GetBytes(normalized, "tools.0.name").String())
	require.False(t, gjson.GetBytes(normalized, "tools.0.function").Exists())
	require.Equal(t, "function", gjson.GetBytes(normalized, "tool_choice.type").String())
	require.Equal(t, "lookup", gjson.GetBytes(normalized, "tool_choice.name").String())
	require.False(t, gjson.GetBytes(normalized, "tool_choice.function").Exists())
	require.Equal(t, int64(128), gjson.GetBytes(normalized, "max_output_tokens").Int())
	require.Equal(t, "high", gjson.GetBytes(normalized, "reasoning.effort").String())
	require.Equal(t, "auto", gjson.GetBytes(normalized, "reasoning.summary").String())
	require.Equal(t, "flex", gjson.GetBytes(normalized, "service_tier").String())
	require.False(t, gjson.GetBytes(normalized, "max_tokens").Exists())
	require.False(t, gjson.GetBytes(normalized, "reasoning_effort").Exists())
REDACTED

func TestNormalizeOpenAIResponsesLegacyIngressNativeInputRemainsAuthoritative(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":[{"id":"msg_native","type":"message","role":"user","content":[{"type":"input_text","text":"keep me"REDACTED]REDACTED],"messages":[{"role":"user","content":"do not merge"REDACTED],"tools":[{"type":"function","function":{"name":"native_shape"REDACTEDREDACTED],"tool_choice":{"type":"function","function":{"name":"native_shape"REDACTEDREDACTED,"previous_response_id":"resp_keep"REDACTED`)

	normalized, changed, err := normalizeOpenAIResponsesLegacyIngress(body)
REDACTED
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "messages").Exists())
	require.Equal(t, "msg_native", gjson.GetBytes(normalized, "input.0.id").String())
	require.Equal(t, int64(1), gjson.GetBytes(normalized, "input.#").Int())
	require.Equal(t, "native_shape", gjson.GetBytes(normalized, "tools.0.function.name").String())
	require.False(t, gjson.GetBytes(normalized, "tools.0.name").Exists())
	require.Equal(t, "native_shape", gjson.GetBytes(normalized, "tool_choice.function.name").String())
	require.Equal(t, "resp_keep", gjson.GetBytes(normalized, "previous_response_id").String())
REDACTED

func TestNormalizeOpenAIResponsesLegacyIngressKeepsPromptAliasAndDropsCommands(t *testing.T) {
	body := []byte(`{"model":"gpt-4.1","prompt":"hello","commands":[{"name":"legacy"REDACTED]REDACTED`)

	normalized, changed, err := normalizeOpenAIResponsesLegacyIngress(body)
REDACTED
	require.True(t, changed)
	require.Equal(t, "hello", gjson.GetBytes(normalized, "input").String())
	require.False(t, gjson.GetBytes(normalized, "prompt").Exists())
	require.False(t, gjson.GetBytes(normalized, "commands").Exists())
REDACTED

func TestNormalizeOpenAIResponsesLegacyIngressPreservesNativePromptTemplate(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","prompt":{"id":"pmpt_abc","version":"7","variables":{"topic":"ownership"REDACTEDREDACTEDREDACTED`)

	normalized, changed, err := normalizeOpenAIResponsesLegacyIngress(body)
REDACTED
	require.False(t, changed)
	require.JSONEq(t, string(body), string(normalized))
	require.Equal(t, "pmpt_abc", gjson.GetBytes(normalized, "prompt.id").String())
	require.False(t, gjson.GetBytes(normalized, "input").Exists())
REDACTED

func TestNormalizeOpenAIResponsesLegacyIngressPreservesUnknownPromptShapeWhileDroppingCommands(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","prompt":["one","two"],"commands":[{"name":"legacy"REDACTED]REDACTED`)

	normalized, changed, err := normalizeOpenAIResponsesLegacyIngress(body)
REDACTED
	require.True(t, changed)
	require.Equal(t, int64(2), gjson.GetBytes(normalized, "prompt.#").Int())
	require.False(t, gjson.GetBytes(normalized, "input").Exists())
	require.False(t, gjson.GetBytes(normalized, "commands").Exists())
REDACTED

//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestStripEmptyChatToolCallIdentity_FirstChunkIdentityUntouched 首包带合法
// id/name 的 delta 必须原样保留（changed=false）。
func TestStripEmptyChatToolCallIdentity_FirstChunkIdentityUntouched(t *testing.T) {
	payload := []byte(`{"id":"chatcmpl_tool","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_example","type":"function","function":{"name":"web_search","arguments":""REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`)
	rewritten, changed := stripEmptyChatToolCallIdentity(payload)
	require.False(t, changed)
	require.Equal(t, string(payload), string(rewritten))
	require.Equal(t, "call_example", gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.id").String())
	require.Equal(t, "web_search", gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.name").String())
	// 首包 arguments 为空串也不应被删除。
	require.True(t, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.arguments").Exists())
	require.Equal(t, "", gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.arguments").String())
REDACTED

// TestStripEmptyChatToolCallIdentity_FollowUpDelta 后续参数 delta 的
// `"id":""` 与 `"function":{"name":""REDACTED` 应被删除；arguments 碎片、
// index、type 保留。
func TestStripEmptyChatToolCallIdentity_FollowingDelta(t *testing.T) {
	payload := []byte(`{"id":"chatcmpl_tool","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"","type":"function","function":{"name":"","arguments":"{\"query\":"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`)
	rewritten, changed := stripEmptyChatToolCallIdentity(payload)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.id").Exists())
	require.False(t, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.name").Exists())
	require.Equal(t, `{"query":`, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.arguments").String())
	require.Equal(t, int64(0), gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.index").Int())
	require.Equal(t, "function", gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.type").String())
	// 空串字段不得再出现在 payload 里。
	require.NotContains(t, string(rewritten), `"id":""`)
	require.NotContains(t, string(rewritten), `"name":""`)
REDACTED

// TestStripEmptyChatToolCallIdentity_OnlyEmptyName / _OnlyEmptyID 只删
// 空的那一个，非空字段必须保留。
func TestStripEmptyChatToolCallIdentity_OnlyEmptyName(t *testing.T) {
	payload := []byte(`{"id":"chatcmpl_tool","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"","arguments":"{REDACTED"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`)
	rewritten, changed := stripEmptyChatToolCallIdentity(payload)
	require.True(t, changed)
	require.Equal(t, "call_1", gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.id").String())
	require.False(t, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.name").Exists())
	require.Equal(t, "{REDACTED", gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.arguments").String())
REDACTED

// TestStripEmptyChatToolCallIdentity_OnlyEmptyID 覆盖 `"id": ""` 带空格的
// JSON 形式，确认 gjson/sjson 均能识别。
func TestStripEmptyChatToolCallIdentity_OnlyEmptyID(t *testing.T) {
	payload := []byte(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id" : "" , "type" : "function","function":{"name":"get_weather","arguments":"{REDACTED"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`)
	rewritten, changed := stripEmptyChatToolCallIdentity(payload)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.id").Exists())
	require.Equal(t, "get_weather", gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.name").String())
	require.Equal(t, "{REDACTED", gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.arguments").String())
REDACTED

// TestStripEmptyChatToolCallIdentity_EmptyArgumentsKept 空 arguments 不删，
// 只有 id/name 空串被剔除。
func TestStripEmptyChatToolCallIdentity_EmptyArgumentsKept(t *testing.T) {
	payload := []byte(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"","type":"function","function":{"name":"","arguments":""REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`)
	rewritten, changed := stripEmptyChatToolCallIdentity(payload)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.id").Exists())
	require.False(t, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.name").Exists())
	require.True(t, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.arguments").Exists())
	require.Equal(t, "", gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.arguments").String())
REDACTED

// TestStripEmptyChatToolCallIdentity_TwoParallelToolCalls 两个并行 index 都要
// 处理：合法的 index 0 保留，后续参数 delta 的 index 1 剔除空 id/name。
func TestStripEmptyChatToolCallIdentity_TwoParallelToolCalls(t *testing.T) {
	payload := []byte(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"tool_a","arguments":"{\"x\":"REDACTEDREDACTED,{"index":1,"id":"","type":"function","function":{"name":"","arguments":"{\"y\":"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`)
	rewritten, changed := stripEmptyChatToolCallIdentity(payload)
	require.True(t, changed)
	require.Equal(t, "call_a", gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.id").String())
	require.Equal(t, "tool_a", gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.name").String())
	require.Equal(t, `{"x":`, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.arguments").String())
	require.False(t, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.1.id").Exists())
	require.False(t, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.1.function.name").Exists())
	require.Equal(t, `{"y":`, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.1.function.arguments").String())
REDACTED

// TestStripEmptyChatToolCallIdentity_Passthrough 无 tool_calls、无 choices、
// 非数组 tool_calls、非法 JSON 一律原样返回。
func TestStripEmptyChatToolCallIdentity_Passthrough(t *testing.T) {
	tests := []struct {
		name    string
		payload string
REDACTED{
		{"no tool_calls", `{"choices":[{"index":0,"delta":{"content":"hi"REDACTEDREDACTED]REDACTED`REDACTED,
		{"no choices", `{"id":"chatcmpl_x"REDACTED`REDACTED,
		{"empty choices", `{"choices":[]REDACTED`REDACTED,
		{"tool_calls not array", `{"choices":[{"index":0,"delta":{"tool_calls":{"foo":1REDACTEDREDACTEDREDACTED]REDACTED`REDACTED,
		{"invalid JSON", `{"choices":[{`REDACTED,
		{"empty string", ``REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rewritten, changed := stripEmptyChatToolCallIdentity([]byte(tt.payload))
			require.False(t, changed)
			require.Equal(t, tt.payload, string(rewritten))
	REDACTED)
REDACTED
REDACTED

// TestStripEmptyChatToolCallIdentityFromSSELine_Passthrough SSE 行级：非
// data 行、[DONE]、空行原样；data 行保留 `data: ` 前缀。
func TestStripEmptyChatToolCallIdentityFromSSELine_Passthrough(t *testing.T) {
	tests := []struct {
		name string
		line string
REDACTED{
		{"done", "data: [DONE]"REDACTED,
		{"non-data line", ": keep-alive"REDACTED,
		{"empty line", ""REDACTED,
		{"comment line", ":"REDACTED,
		{"content chunk", `data: {"choices":[{"index":0,"delta":{"content":"hi"REDACTEDREDACTED]REDACTED`REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.line, stripEmptyChatToolCallIdentityFromSSELine(tt.line))
	REDACTED)
REDACTED
REDACTED

// TestStripEmptyChatToolCallIdentityFromSSELine_KeepsDataPrefix 改写后的
// SSE 行必须保留 `data: ` 前缀。
func TestStripEmptyChatToolCallIdentityFromSSELine_KeepsDataPrefix(t *testing.T) {
	line := `data: {"id":"chatcmpl_tool","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"","type":"function","function":{"name":"","arguments":"{REDACTED"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`
	got := stripEmptyChatToolCallIdentityFromSSELine(line)
	require.True(t, strings.HasPrefix(got, "data: "))
	payload, ok := extractOpenAISSEDataLine(got)
	require.True(t, ok)
	require.False(t, gjson.Get(payload, "choices.0.delta.tool_calls.0.id").Exists())
	require.False(t, gjson.Get(payload, "choices.0.delta.tool_calls.0.function.name").Exists())
	require.Equal(t, "{REDACTED", gjson.Get(payload, "choices.0.delta.tool_calls.0.function.arguments").String())
REDACTED

// TestStripEmptyChatToolCallIdentity_DshClientMerge 模拟 dsh rc.2 adapter 的
// 合并逻辑（字段存在——含空串——才覆盖）：sanitize 后合并，最终 id/name 必须
// 仍是首包合法值，arguments 为各碎片拼接。
func TestStripEmptyChatToolCallIdentity_DshClientMerge(t *testing.T) {
	lines := []string{
		`data: {"id":"chatcmpl_tool","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_example","type":"function","function":{"name":"web_search","arguments":""REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`,
		`data: {"id":"chatcmpl_tool","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"","type":"function","function":{"name":"","arguments":"{\"query\":"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`,
		`data: {"id":"chatcmpl_tool","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"","type":"function","function":{"name":"","arguments":"\"example\"REDACTED"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`,
REDACTED

	var mergedID, mergedName, mergedArgs string
	for _, line := range lines {
		sanitized := stripEmptyChatToolCallIdentityFromSSELine(line)
		payload, ok := extractOpenAISSEDataLine(sanitized)
		require.True(t, ok)
		for _, tc := range gjson.Get(payload, "choices.0.delta.tool_calls").Array() {
			if v := tc.Get("id"); v.Exists() {
				mergedID = v.String()
		REDACTED
			if v := tc.Get("function.name"); v.Exists() {
				mergedName = v.String()
		REDACTED
			if v := tc.Get("function.arguments"); v.Exists() {
				mergedArgs += v.String()
		REDACTED
	REDACTED
REDACTED

	require.Equal(t, "call_example", mergedID)
	require.Equal(t, "web_search", mergedName)
	require.Equal(t, `{"query":"example"REDACTED`, mergedArgs)
REDACTED

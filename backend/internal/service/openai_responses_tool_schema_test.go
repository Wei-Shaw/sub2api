package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// issue #5364 的最小复现体：Codex Desktop 内置 automation_update 带
// parameters.type = null，upstream 回 400 invalid_function_parameters。
func TestSanitizeOpenAIResponsesToolParameterTypes_TopLevelFunctionTool(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.6-sol",
		"input": "Reply with OK.",
		"stream": false,
		"tools": [
			{
				"type": "function",
				"name": "automation_update",
				"description": "Update an automation.",
				"parameters": {"type": null, "properties": {REDACTEDREDACTED
		REDACTED
		]
REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, "object", gjson.GetBytes(sanitized, "tools.0.parameters.type").String())
	// 只改 type，工具其余定义原样保留。
	require.Equal(t, "automation_update", gjson.GetBytes(sanitized, "tools.0.name").String())
	require.Equal(t, "Update an automation.", gjson.GetBytes(sanitized, "tools.0.description").String())
	require.True(t, gjson.GetBytes(sanitized, "tools.0.parameters.properties").IsObject())
	// 请求体其余字段不受影响。
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(sanitized, "model").String())
	require.Equal(t, "Reply with OK.", gjson.GetBytes(sanitized, "input").String())
REDACTED

// 合法 Schema 必须原样返回：changed=false 且字节不变，避免无谓重写打散
// prompt cache 前缀。
func TestSanitizeOpenAIResponsesToolParameterTypes_ValidSchemaUntouched(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","name":"ok","parameters":{"type":"object","properties":{REDACTEDREDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.False(t, changed)
	require.Equal(t, string(body), string(sanitized))
REDACTED

// 缺失 type 的 Schema 本身合法（等价于不约束），不得补写——补写会收窄客户端语义。
func TestSanitizeOpenAIResponsesToolParameterTypes_MissingTypeNotInvented(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","name":"ok","parameters":{"properties":{REDACTEDREDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.False(t, changed)
	require.False(t, gjson.GetBytes(sanitized, "tools.0.parameters.type").Exists())
REDACTED

// 多轮历史：工具定义沉进 input 后，upstream 报错路径形如
// input[N].tools[i].tools[j].parameters，两层都要修。
func TestSanitizeOpenAIResponsesToolParameterTypes_NestedHistoryTools(t *testing.T) {
	body := []byte(`{
		"input": [
			{"type": "message", "role": "user", "content": "hi"REDACTED,
			{
				"type": "additional_tools",
				"role": "developer",
				"tools": [
					{
						"type": "namespace",
						"name": "codex_app",
						"tools": [
							{"type": "function", "name": "noop", "parameters": {"type": "object"REDACTEDREDACTED,
							{"type": "function", "name": "automation_update", "parameters": {"type": nullREDACTEDREDACTED
						]
				REDACTED,
					{"type": "function", "name": "outer", "parameters": {"type": nullREDACTEDREDACTED
				]
		REDACTED
		]
REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, "object", gjson.GetBytes(sanitized, "input.1.tools.0.tools.1.parameters.type").String())
	require.Equal(t, "object", gjson.GetBytes(sanitized, "input.1.tools.1.parameters.type").String())
	// 原本合法的兄弟条目保持不变。
	require.Equal(t, "object", gjson.GetBytes(sanitized, "input.1.tools.0.tools.0.parameters.type").String())
	require.Equal(t, "hi", gjson.GetBytes(sanitized, "input.0.content").String())
REDACTED

// ChatCompletions 形态的工具（{type:"function", function:{...REDACTEDREDACTED）同样可能出现在
// Responses 请求里，见 normalizeCodexTools。
func TestSanitizeOpenAIResponsesToolParameterTypes_ChatCompletionsShape(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","function":{"name":"legacy","parameters":{"type":nullREDACTEDREDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, "object", gjson.GetBytes(sanitized, "tools.0.function.parameters.type").String())
	require.Equal(t, "legacy", gjson.GetBytes(sanitized, "tools.0.function.name").String())
REDACTED

// 索引映射：只有坏条目被改，前后兄弟条目按原下标保持不变。
func TestSanitizeOpenAIResponsesToolParameterTypes_OnlyOffendingIndexRewritten(t *testing.T) {
	body := []byte(`{"tools":[
		{"type":"function","name":"a","parameters":{"type":"object"REDACTEDREDACTED,
		{"type":"function","name":"b","parameters":{"type":nullREDACTEDREDACTED,
		{"type":"function","name":"c","parameters":{"type":"object"REDACTEDREDACTED
	]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, "a", gjson.GetBytes(sanitized, "tools.0.name").String())
	require.Equal(t, "b", gjson.GetBytes(sanitized, "tools.1.name").String())
	require.Equal(t, "c", gjson.GetBytes(sanitized, "tools.2.name").String())
	require.Equal(t, "object", gjson.GetBytes(sanitized, "tools.1.parameters.type").String())
	require.Equal(t, 3, int(gjson.GetBytes(sanitized, "tools.#").Int()))
REDACTED

// 畸形/非常规形态不得 panic，且一律按不变处理。
func TestSanitizeOpenAIResponsesToolParameterTypes_MalformedShapesAreNoOps(t *testing.T) {
	cases := []struct {
		name string
		body string
REDACTED{
		{"empty_body", ``REDACTED,
		{"no_tools", `{"model":"gpt-5.6-sol","input":"hi"REDACTED`REDACTED,
		{"tools_null", `{"tools":nullREDACTED`REDACTED,
		{"tools_object", `{"tools":{"type":"function"REDACTEDREDACTED`REDACTED,
		{"tool_is_string", `{"tools":["freeform"]REDACTED`REDACTED,
		{"parameters_is_string", `{"tools":[{"type":"function","parameters":"nope"REDACTED]REDACTED`REDACTED,
		{"parameters_null", `{"tools":[{"type":"function","parameters":nullREDACTED]REDACTED`REDACTED,
		{"input_string", `{"input":"hi","tools":[]REDACTED`REDACTED,
		{"input_item_not_object", `{"input":["hi"]REDACTED`REDACTED,
		{"type_already_array", `{"tools":[{"type":"function","parameters":{"type":["object","null"]REDACTEDREDACTED]REDACTED`REDACTED,
REDACTED

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes([]byte(tc.body))
		REDACTED
			require.False(t, changed)
			require.Equal(t, tc.body, string(sanitized))
	REDACTED)
REDACTED
REDACTED

// 递归深度守卫：超深嵌套只做截断，不递归到栈溢出，也不报错。
func TestSanitizeOpenAIResponsesToolParameterTypes_DepthGuard(t *testing.T) {
	tool := map[string]any{"type": "function", "name": "deep", "parameters": map[string]any{"type": nilREDACTEDREDACTED
	for i := 0; i < 12; i++ {
		tool = map[string]any{"type": "namespace", "tools": []any{toolREDACTEDREDACTED
REDACTED
	body, err := json.Marshal(map[string]any{"tools": []any{toolREDACTEDREDACTED)
REDACTED

	require.NotPanics(t, func() {
		_, _, sanitizeErr := sanitizeOpenAIResponsesToolParameterTypes(body)
		require.NoError(t, sanitizeErr)
REDACTED)
REDACTED

// 输出必须是合法 JSON，且除目标字段外与输入等价。
func TestSanitizeOpenAIResponsesToolParameterTypes_OutputStaysValidJSON(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","tool_choice":"none","store":false,"tools":[{"type":"function","name":"automation_update","parameters":{"type":null,"properties":{REDACTEDREDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)
REDACTED
	require.True(t, changed)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(sanitized, &decoded))
	require.Equal(t, "gpt-5.5", decoded["model"])
	require.Equal(t, "none", decoded["tool_choice"])
	require.Equal(t, false, decoded["store"])
REDACTED

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
				"parameters": {"type": null, "properties": {}}
			}
		]
	}`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "object", gjson.GetBytes(sanitized, "tools.0.parameters.type").String())
	// 只改 type，工具其余定义原样保留。
	require.Equal(t, "automation_update", gjson.GetBytes(sanitized, "tools.0.name").String())
	require.Equal(t, "Update an automation.", gjson.GetBytes(sanitized, "tools.0.description").String())
	require.True(t, gjson.GetBytes(sanitized, "tools.0.parameters.properties").IsObject())
	// 请求体其余字段不受影响。
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(sanitized, "model").String())
	require.Equal(t, "Reply with OK.", gjson.GetBytes(sanitized, "input").String())
}

// 合法 Schema 必须原样返回：changed=false 且字节不变，避免无谓重写打散
// prompt cache 前缀。
func TestSanitizeOpenAIResponsesToolParameterTypes_ValidSchemaUntouched(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","name":"ok","parameters":{"type":"object","properties":{}}}]}`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, string(body), string(sanitized))
}

// 缺失 type 的 Schema 本身合法（等价于不约束），不得补写——补写会收窄客户端语义。
func TestSanitizeOpenAIResponsesToolParameterTypes_MissingTypeNotInvented(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","name":"ok","parameters":{"properties":{}}}]}`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

	require.NoError(t, err)
	require.False(t, changed)
	require.False(t, gjson.GetBytes(sanitized, "tools.0.parameters.type").Exists())
}

// 多轮历史：工具定义沉进 input 后，upstream 报错路径形如
// input[N].tools[i].tools[j].parameters，两层都要修。
func TestSanitizeOpenAIResponsesToolParameterTypes_NestedHistoryTools(t *testing.T) {
	body := []byte(`{
		"input": [
			{"type": "message", "role": "user", "content": "hi"},
			{
				"type": "additional_tools",
				"role": "developer",
				"tools": [
					{
						"type": "namespace",
						"name": "codex_app",
						"tools": [
							{"type": "function", "name": "noop", "parameters": {"type": "object"}},
							{"type": "function", "name": "automation_update", "parameters": {"type": null}}
						]
					},
					{"type": "function", "name": "outer", "parameters": {"type": null}}
				]
			}
		]
	}`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "object", gjson.GetBytes(sanitized, "input.1.tools.0.tools.1.parameters.type").String())
	require.Equal(t, "object", gjson.GetBytes(sanitized, "input.1.tools.1.parameters.type").String())
	// 原本合法的兄弟条目保持不变。
	require.Equal(t, "object", gjson.GetBytes(sanitized, "input.1.tools.0.tools.0.parameters.type").String())
	require.Equal(t, "hi", gjson.GetBytes(sanitized, "input.0.content").String())
}

// ChatCompletions 形态的工具（{type:"function", function:{...}}）同样可能出现在
// Responses 请求里，见 normalizeCodexTools。
func TestSanitizeOpenAIResponsesToolParameterTypes_ChatCompletionsShape(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","function":{"name":"legacy","parameters":{"type":null}}}]}`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "object", gjson.GetBytes(sanitized, "tools.0.function.parameters.type").String())
	require.Equal(t, "legacy", gjson.GetBytes(sanitized, "tools.0.function.name").String())
}

// 索引映射：只有坏条目被改，前后兄弟条目按原下标保持不变。
func TestSanitizeOpenAIResponsesToolParameterTypes_OnlyOffendingIndexRewritten(t *testing.T) {
	body := []byte(`{"tools":[
		{"type":"function","name":"a","parameters":{"type":"object"}},
		{"type":"function","name":"b","parameters":{"type":null}},
		{"type":"function","name":"c","parameters":{"type":"object"}}
	]}`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "a", gjson.GetBytes(sanitized, "tools.0.name").String())
	require.Equal(t, "b", gjson.GetBytes(sanitized, "tools.1.name").String())
	require.Equal(t, "c", gjson.GetBytes(sanitized, "tools.2.name").String())
	require.Equal(t, "object", gjson.GetBytes(sanitized, "tools.1.parameters.type").String())
	require.Equal(t, 3, int(gjson.GetBytes(sanitized, "tools.#").Int()))
}

// 畸形/非常规形态不得 panic，且一律按不变处理。
func TestSanitizeOpenAIResponsesToolParameterTypes_MalformedShapesAreNoOps(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty_body", ``},
		{"no_tools", `{"model":"gpt-5.6-sol","input":"hi"}`},
		{"tools_null", `{"tools":null}`},
		{"tools_object", `{"tools":{"type":"function"}}`},
		{"tool_is_string", `{"tools":["freeform"]}`},
		{"parameters_is_string", `{"tools":[{"type":"function","parameters":"nope"}]}`},
		{"parameters_null", `{"tools":[{"type":"function","parameters":null}]}`},
		{"input_string", `{"input":"hi","tools":[]}`},
		{"input_item_not_object", `{"input":["hi"]}`},
		{"type_already_array", `{"tools":[{"type":"function","parameters":{"type":["object","null"]}}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes([]byte(tc.body))
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, tc.body, string(sanitized))
		})
	}
}

// 递归深度守卫：超深嵌套只做截断，不递归到栈溢出，也不报错。
func TestSanitizeOpenAIResponsesToolParameterTypes_DepthGuard(t *testing.T) {
	tool := map[string]any{"type": "function", "name": "deep", "parameters": map[string]any{"type": nil}}
	for i := 0; i < 12; i++ {
		tool = map[string]any{"type": "namespace", "tools": []any{tool}}
	}
	body, err := json.Marshal(map[string]any{"tools": []any{tool}})
	require.NoError(t, err)

	require.NotPanics(t, func() {
		_, _, sanitizeErr := sanitizeOpenAIResponsesToolParameterTypes(body)
		require.NoError(t, sanitizeErr)
	})
}

// 输出必须是合法 JSON，且除目标字段外与输入等价。
func TestSanitizeOpenAIResponsesToolParameterTypes_OutputStaysValidJSON(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","tool_choice":"none","store":false,"tools":[{"type":"function","name":"automation_update","parameters":{"type":null,"properties":{}}}]}`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)
	require.NoError(t, err)
	require.True(t, changed)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(sanitized, &decoded))
	require.Equal(t, "gpt-5.5", decoded["model"])
	require.Equal(t, "none", decoded["tool_choice"])
	require.Equal(t, false, decoded["store"])
}

// 输入 body 是调用方持有的缓冲区（Forward 里 canonicalImageIntentBody 与它同源），
// 净化必须返回新切片，绝不能就地改写。
func TestSanitizeOpenAIResponsesToolParameterTypes_DoesNotMutateInputBody(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","tools":[{"type":"function","name":"a","parameters":{"type":null}}]}`)
	original := append([]byte(nil), body...)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, string(original), string(body), "调用方的 body 不得被就地改写")
	require.NotEqual(t, string(original), string(sanitized))
}

func buildToolSchemaNullTypeBody(t *testing.T, hits int) []byte {
	t.Helper()
	tools := make([]any, 0, hits)
	for i := 0; i < hits; i++ {
		tools = append(tools, map[string]any{
			"type":       "function",
			"name":       "automation_update",
			"parameters": map[string]any{"type": nil, "properties": map[string]any{}},
		})
	}
	body, err := json.Marshal(map[string]any{"model": "gpt-5.6-sol", "tools": tools})
	require.NoError(t, err)
	return body
}

// 复杂度守卫：重写次数必须与命中数无关。
//
// 逐个 sjson.SetBytes 的写法每命中一处就重扫并全量拷贝一次文档，命中 N 处即 N 次
// 全量拷贝；/v1/responses 的 body 上限是 gateway.max_body_size（默认 256MB），
// 构造请求可以塞进百万级命中，会被放大成 TB 级 memcpy。这里用分配次数锁死该行为：
// 命中数放大 500 倍，分配次数不得随之增长。
func TestSanitizeOpenAIResponsesToolParameterTypes_RewriteCountIndependentOfHits(t *testing.T) {
	small := buildToolSchemaNullTypeBody(t, 4)
	large := buildToolSchemaNullTypeBody(t, 2000)

	smallAllocs := testing.AllocsPerRun(2, func() {
		_, _, _ = sanitizeOpenAIResponsesToolParameterTypes(small)
	})
	largeAllocs := testing.AllocsPerRun(2, func() {
		_, _, _ = sanitizeOpenAIResponsesToolParameterTypes(large)
	})

	// 命中切片扩容是对数级，留出充裕余量；线性写法在这里会是 2000 量级。
	require.Less(t, largeAllocs, smallAllocs+40,
		"分配次数随命中数线性增长，说明退回了逐路径全量重写 (small=%v large=%v)", smallAllocs, largeAllocs)

	// 同时确认大 body 的结果确实全部修好了。
	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(large)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 2000, int(gjson.GetBytes(sanitized, "tools.#").Int()))
	gjson.GetBytes(sanitized, "tools").ForEach(func(_, tool gjson.Result) bool {
		require.Equal(t, "object", tool.Get("parameters.type").String())
		return true
	})
}

package service

import (
	"encoding/json"
	"strconv"
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

func TestSanitizeOpenAIResponsesToolParameterTypes_ObjectOnlyRootUnion(t *testing.T) {
	body := []byte(`{"tools":[
		{"type":"function","name":"automation_update","parameters":{"oneOf":[{"type":"object","properties":{"id":{"type":"string"REDACTEDREDACTEDREDACTED,{"anyOf":[{"type":"object"REDACTED,{"type":"object","properties":{REDACTEDREDACTED]REDACTED]REDACTEDREDACTED,
		{"type":"function","name":"codex_app__automation_update","parameters":{"anyOf":[{"type":"object"REDACTED]REDACTEDREDACTED,
		{"type":"function","name":"unrelated_tool","parameters":{"oneOf":[{"type":"object"REDACTED]REDACTEDREDACTED
	]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.True(t, changed)
	for i := 0; i < 3; i++ {
		require.Equal(t, "object", gjson.GetBytes(sanitized, "tools."+strconv.Itoa(i)+".parameters.type").String())
REDACTED
	require.Equal(t, "string", gjson.GetBytes(sanitized, "tools.0.parameters.oneOf.0.properties.id.type").String())
REDACTED

func TestSanitizeOpenAIResponsesToolParameterTypes_EscapedUnionKeyword(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","name":"other","parameters":{"one\u004ff":[{"type":"object"REDACTED]REDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, "object", gjson.GetBytes(sanitized, "tools.0.parameters.type").String())
REDACTED

func TestSanitizeOpenAIResponsesToolParameterTypes_ObjectUnionMissingTypeSafetyBoundary(t *testing.T) {
	tests := []struct {
		name string
		body string
REDACTED{
		{name: "mixed union", body: `{"tools":[{"type":"function","name":"automation_update","parameters":{"oneOf":[{"type":"object"REDACTED,{"type":"string"REDACTED]REDACTEDREDACTED]REDACTED`REDACTED,
		{name: "unproven branch", body: `{"tools":[{"type":"function","name":"automation_update","parameters":{"oneOf":[{"properties":{REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`REDACTED,
		{name: "empty union", body: `{"tools":[{"type":"function","name":"automation_update","parameters":{"oneOf":[]REDACTEDREDACTED]REDACTED`REDACTED,
		{name: "existing null handled by existing repair", body: `{"tools":[{"type":"function","name":"automation_update","parameters":{"type":null,"oneOf":[{"type":"object"REDACTED]REDACTEDREDACTED]REDACTED`REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes([]byte(tt.body))
		REDACTED
			if tt.name == "existing null handled by existing repair" {
				require.True(t, changed)
				require.Equal(t, "object", gjson.GetBytes(sanitized, "tools.0.parameters.type").String())
				return
		REDACTED
			require.False(t, changed)
			require.Equal(t, tt.body, string(sanitized))
	REDACTED)
REDACTED
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
				"parameters": {"type": nullREDACTED,
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
	require.Equal(t, "null", gjson.GetBytes(sanitized, "input.1.parameters.type").Raw)
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

func TestSanitizeOpenAIResponsesToolParameterTypes_InputItemShapes(t *testing.T) {
	body := []byte(`{"input":[{"parameters":{"type":nullREDACTED,"name":"direct","type":"function"REDACTED,{"function":{"name":"wrapped","parameters":{"type":nullREDACTEDREDACTED,"type":"function"REDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, "object", gjson.GetBytes(sanitized, "input.0.parameters.type").String())
	require.Equal(t, "object", gjson.GetBytes(sanitized, "input.1.function.parameters.type").String())
REDACTED

func TestSanitizeOpenAIResponsesToolParameterTypes_DoesNotTreatMessagesAsTools(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","role":"user","parameters":{"type":nullREDACTED,"function":{"parameters":{"type":nullREDACTEDREDACTED,"metadata":{"parameters":{"type":nullREDACTEDREDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.False(t, changed)
	require.Equal(t, string(body), string(sanitized))
REDACTED

func TestSanitizeOpenAIResponsesToolParameterTypes_PreservesUntouchedBytes(t *testing.T) {
	body := []byte("{\n  \"z\":1e+999, \"integer\":900719925474099312345, \"escaped\":\"\\u003chtml\\u003e\", \"tools\":[{\"parameters\":{\"properties\":{REDACTED, \"type\" : nullREDACTED,\"name\":\"x\"REDACTED], \"a\":-0.00\nREDACTED")
	want := []byte("{\n  \"z\":1e+999, \"integer\":900719925474099312345, \"escaped\":\"\\u003chtml\\u003e\", \"tools\":[{\"parameters\":{\"properties\":{REDACTED, \"type\" : \"object\"REDACTED,\"name\":\"x\"REDACTED], \"a\":-0.00\nREDACTED")

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, string(want), string(sanitized))
REDACTED

func TestSanitizeOpenAIResponsesToolParameterTypes_EscapedKey(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","parameters":{"ty\u0070e":nullREDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, `{"tools":[{"type":"function","parameters":{"ty\u0070e":"object"REDACTEDREDACTED]REDACTED`, string(sanitized))
REDACTED

func TestSanitizeOpenAIResponsesToolParameterTypes_DuplicateTypeIsAmbiguousNoOp(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{"tools":[{"type":"function","parameters":{"type":null,"type":"string"REDACTEDREDACTED]REDACTED`),
		[]byte(`{"tools":[{"type":"function","parameters":{"type":"string","ty\u0070e":nullREDACTEDREDACTED]REDACTED`),
		[]byte(`{"tools":[{"type":"function","parameters":{"type":null,"type":nullREDACTEDREDACTED]REDACTED`),
REDACTED {
		sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)
	REDACTED
		require.False(t, changed)
		require.Equal(t, string(body), string(sanitized))
REDACTED
REDACTED

func TestSanitizeOpenAIResponsesToolSchemas_InvalidAndTrailingJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
		call func([]byte) ([]byte, bool, error)
REDACTED{
		{"null_type_trailing", []byte(`{"tools":[{"parameters":{"type":nullREDACTEDREDACTED]REDACTED trailing`), sanitizeOpenAIResponsesToolParameterTypesREDACTED,
		{"pattern_trailing", []byte(`{"tools":[{"parameters":{"pattern":"(?=x)"REDACTEDREDACTED]REDACTED trailing`), sanitizeOpenAIResponsesToolSchemaPatternsREDACTED,
		{"invalid_utf8", append([]byte(`{"tools":[{"parameters":{"pattern":"(?=x)"REDACTEDREDACTED],"bad":"`), 0xff, '"', 'REDACTED'), sanitizeOpenAIResponsesToolSchemaPatternsREDACTED,
REDACTED {
		t.Run(tc.name, func(t *testing.T) {
			_, changed, err := tc.call(tc.body)
		REDACTED
			require.False(t, changed)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIResponsesToolSchemaCapabilities_PlatformBoundary(t *testing.T) {
	tests := []struct {
		platform         string
		repairNullType   bool
		removeLookaround bool
REDACTED{
		{PlatformOpenAI, true, trueREDACTED,
		{PlatformAnthropic, true, falseREDACTED,
		{PlatformKimi, true, falseREDACTED,
		{PlatformZhipu, true, falseREDACTED,
		{PlatformDeepseek, true, falseREDACTED,
		{PlatformGrok, false, falseREDACTED,
		{PlatformGemini, false, falseREDACTED,
		{PlatformAntigravity, false, falseREDACTED,
		{PlatformComposite, false, falseREDACTED,
		{"", false, falseREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			require.Equal(t, tt.repairNullType, shouldRepairOpenAIResponsesNullToolSchemaType(tt.platform))
			require.Equal(t, tt.removeLookaround, shouldSanitizeOpenAIResponsesToolSchemaPatterns(tt.platform))
	REDACTED)
REDACTED
REDACTED

func TestSanitizeOpenAIResponsesToolSchemasForPlatform_ReplayBoundary(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","parameters":{"type":null,"properties":{"query":{"type":"string","pattern":"(?=keep)"REDACTEDREDACTEDREDACTEDREDACTED]REDACTED`)

	// A malformed tool definition may be replayed after account failover. Every
	// compatible account must repair it, while non-OpenAI providers retain their
	// supported regex semantics.
	for _, platform := range []string{PlatformAnthropic, PlatformKimi, PlatformZhipu, PlatformDeepseekREDACTED {
		t.Run(platform, func(t *testing.T) {
			for attempt := 0; attempt < 2; attempt++ {
				normalized, changed, err := sanitizeOpenAIResponsesToolSchemasForPlatform(body, platform)
			REDACTED
				require.True(t, changed)
				require.Equal(t, "object", gjson.GetBytes(normalized, "tools.0.parameters.type").String())
				require.Equal(t, "(?=keep)", gjson.GetBytes(normalized, "tools.0.parameters.properties.query.pattern").String())
		REDACTED
	REDACTED)
REDACTED

	openAI, changed, err := sanitizeOpenAIResponsesToolSchemasForPlatform(body, PlatformOpenAI)
REDACTED
	require.True(t, changed)
	require.Equal(t, "object", gjson.GetBytes(openAI, "tools.0.parameters.type").String())
	require.False(t, gjson.GetBytes(openAI, "tools.0.parameters.properties.query.pattern").Exists())

	unsupported, changed, err := sanitizeOpenAIResponsesToolSchemasForPlatform(body, PlatformGrok)
REDACTED
	require.False(t, changed)
	require.Equal(t, string(body), string(unsupported))
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

// 输入 body 是调用方持有的缓冲区（Forward 里 canonicalImageIntentBody 与它同源），
// 净化必须返回新切片，绝不能就地改写。
func TestSanitizeOpenAIResponsesToolParameterTypes_DoesNotMutateInputBody(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","tools":[{"type":"function","name":"a","parameters":{"type":nullREDACTEDREDACTED]REDACTED`)
	original := append([]byte(nil), body...)

	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, string(original), string(body), "调用方的 body 不得被就地改写")
	require.NotEqual(t, string(original), string(sanitized))
REDACTED

func TestSanitizeOpenAIResponsesToolSchemaPatterns_RemovesLookaroundOnly(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","name":"search","parameters":{"type":"object","properties":{"q":{"type":"string","pattern":"^(?=.*foo)[a-z]+$"REDACTED,"id":{"type":"string","pattern":"^[a-z]+$"REDACTED,"z":{"type":"string","pattern":"(?<!bad)ok"REDACTEDREDACTEDREDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)
REDACTED
	require.True(t, changed)
	require.False(t, gjson.GetBytes(sanitized, "tools.0.parameters.properties.q.pattern").Exists())
	require.False(t, gjson.GetBytes(sanitized, "tools.0.parameters.properties.z.pattern").Exists())
	require.Equal(t, "^[a-z]+$", gjson.GetBytes(sanitized, "tools.0.parameters.properties.id.pattern").String())
REDACTED

func TestSanitizeOpenAIResponsesToolSchemaPatterns_PreservesRawDocumentRepresentation(t *testing.T) {
	body := []byte("{\n \"number\":1e+999,\n \"tools\":[{\"parameters\":{\"pattern\":\"(?=drop)\", \"description\":\"\\u003ctag\\u003e\", \"type\":\"string\"REDACTEDREDACTED],\n \"tail\":-0.00\nREDACTED")
	want := []byte("{\n \"number\":1e+999,\n \"tools\":[{\"parameters\":{ \"description\":\"\\u003ctag\\u003e\", \"type\":\"string\"REDACTEDREDACTED],\n \"tail\":-0.00\nREDACTED")

	sanitized, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, string(want), string(sanitized))
REDACTED

func TestSanitizeOpenAIResponsesToolSchemaPatterns_AllMemberPositions(t *testing.T) {
	body := []byte(`{"tools":[{"parameters":{"properties":{"first":{"pattern":"(?=a)","type":"string"REDACTED,"middle":{"title":"x","pattern":"(?!b)","type":"string"REDACTED,"last":{"type":"string","pattern":"(?<=c)"REDACTED,"only":{"pattern":"(?<!d)"REDACTEDREDACTEDREDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)

REDACTED
	require.True(t, changed)
	for _, name := range []string{"first", "middle", "last", "only"REDACTED {
		require.False(t, gjson.GetBytes(sanitized, "tools.0.parameters.properties."+name+".pattern").Exists(), name)
		require.True(t, gjson.GetBytes(sanitized, "tools.0.parameters.properties."+name).IsObject(), name)
REDACTED
REDACTED

func TestSanitizeOpenAIResponsesToolSchemaPatterns_TraversesConservativeSchemaKeywords(t *testing.T) {
	body := []byte(`{"input":[{"type":"function","name":"direct","parameters":{"type":"object","properties":{"p":{"anyOf":[{"pattern":"(?=any)"REDACTED],"items":{"pattern":"(?!item)"REDACTEDREDACTEDREDACTED,"$defs":{"d":{"pattern":"(?<=def)"REDACTEDREDACTED,"additionalProperties":{"pattern":"(?<!additional)"REDACTEDREDACTEDREDACTED,{"type":"function","function":{"name":"wrapped","parameters":{"oneOf":[{"pattern":"(?=wrapped)"REDACTED]REDACTEDREDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)

REDACTED
	require.True(t, changed)
	for _, path := range []string{
		"input.0.parameters.properties.p.anyOf.0.pattern",
		"input.0.parameters.properties.p.items.pattern",
		"input.0.parameters.$defs.d.pattern",
		"input.0.parameters.additionalProperties.pattern",
		"input.1.function.parameters.oneOf.0.pattern",
REDACTED {
		require.False(t, gjson.GetBytes(sanitized, path).Exists(), path)
REDACTED
REDACTED

func TestSanitizeOpenAIResponsesToolSchemaPatterns_DoesNotTraverseMessageMetadata(t *testing.T) {
	body := []byte(`{"input":[{"parameters":{"pattern":"(?=missing-type)"REDACTEDREDACTED,{"type":"message","parameters":{"pattern":"(?=parameters)"REDACTED,"function":{"parameters":{"pattern":"(?=function)"REDACTEDREDACTED,"metadata":{"tools":[{"type":"function","parameters":{"pattern":"(?=metadata)"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)

REDACTED
	require.False(t, changed)
	require.Equal(t, string(body), string(sanitized))
REDACTED

func TestSanitizeOpenAIResponsesToolSchemaPatterns_DecodesEscapedLookaround(t *testing.T) {
	for _, pattern := range []string{`\u0028?\u003dfoo`, `(\u003f\u003dfoo)`REDACTED {
		body := []byte(`{"tools":[{"parameters":{"type":"string","pattern":"` + pattern + `"REDACTEDREDACTED]REDACTED`)
		sanitized, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)
	REDACTED
		require.True(t, changed)
		require.False(t, gjson.GetBytes(sanitized, "tools.0.parameters.pattern").Exists())
REDACTED
REDACTED

func TestSanitizeOpenAIResponsesToolSchemaPatterns_EscapedKey(t *testing.T) {
	body := []byte(`{"to\u006fls":[{"par\u0061meters":{"type":"string","pat\u0074ern":"(?=drop)"REDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, `{"to\u006fls":[{"par\u0061meters":{"type":"string"REDACTEDREDACTED]REDACTED`, string(sanitized))
REDACTED

func TestSanitizeOpenAIResponsesToolSchemaPatterns_DuplicateKeysStayValid(t *testing.T) {
	for _, tc := range []struct {
		body string
		want string
REDACTED{
		{
			`{"tools":[{"parameters":{"pattern":"(?=drop)","pattern":"^[a-z]+$","type":"string"REDACTEDREDACTED]REDACTED`,
			`{"tools":[{"parameters":{"pattern":"^[a-z]+$","type":"string"REDACTEDREDACTED]REDACTED`,
	REDACTED,
		{
			`{"tools":[{"parameters":{"type":"string","pattern":"(?=first)","pattern":"(?!second)"REDACTEDREDACTED]REDACTED`,
			`{"tools":[{"parameters":{"type":"string"REDACTEDREDACTED]REDACTED`,
	REDACTED,
REDACTED {
		sanitized, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns([]byte(tc.body))
	REDACTED
		require.True(t, changed)
		require.True(t, json.Valid(sanitized))
		require.Equal(t, tc.want, string(sanitized))
REDACTED
REDACTED

func TestOpenAIResponsesToolSchemaPlatformGate_APIKeyAndOAuth(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","parameters":{"type":null,"pattern":"(?=drop)"REDACTEDREDACTED]REDACTED`)
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuthREDACTED {
		t.Run(accountType, func(t *testing.T) {
			normalized, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{
				Platform: PlatformOpenAI,
				Type:     accountType,
		REDACTED)
		REDACTED
			require.True(t, changed)
			require.Equal(t, "object", gjson.GetBytes(normalized, "tools.0.parameters.type").String())
			require.False(t, gjson.GetBytes(normalized, "tools.0.parameters.pattern").Exists())
	REDACTED)
REDACTED

	normalized, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
REDACTED)
REDACTED
	require.False(t, changed)
	require.Equal(t, string(body), string(normalized))
REDACTED

func TestSanitizeOpenAIResponsesToolSchemas_DepthLimitReturnsOriginal(t *testing.T) {
	body := []byte(`{"tools":[{"parameters":`)
	for i := 0; i < openAIResponsesToolSchemaMaxDepth+2; i++ {
		body = append(body, []byte(`{"allOf":[`)...)
REDACTED
	body = append(body, []byte(`{"pattern":"(?=too-deep)"REDACTED`)...)
	for i := 0; i < openAIResponsesToolSchemaMaxDepth+2; i++ {
		body = append(body, []byte(`]REDACTED`)...)
REDACTED
	body = append(body, []byte(`REDACTED]REDACTED`)...)
	require.True(t, json.Valid(body))

	sanitized, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)

REDACTED
	require.False(t, changed)
	require.Equal(t, string(body), string(sanitized))
REDACTED

func TestSanitizeOpenAIResponsesToolSchemaPatterns_DoesNotTouchUserInputPattern(t *testing.T) {
	body := []byte(`{"input":{"pattern":"(?=keep)"REDACTED,"metadata":{"pattern":"(?!keep)"REDACTEDREDACTED`)
	sanitized, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)
REDACTED
	require.False(t, changed)
	require.Equal(t, string(body), string(sanitized))
REDACTED

func TestSanitizeOpenAIResponsesToolSchemaPatterns_DoesNotTraverseInstanceData(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","parameters":{"type":"object","properties":{"config":{"type":"object","pattern":"(?=remove)","default":{"pattern":"(?=keep-default)"REDACTED,"examples":[{"pattern":"(?=keep-example)"REDACTED],"const":{"pattern":"(?=keep-const)"REDACTED,"enum":[{"pattern":"(?=keep-enum)"REDACTED]REDACTEDREDACTEDREDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)
REDACTED
	require.True(t, changed)
	require.False(t, gjson.GetBytes(sanitized, "tools.0.parameters.properties.config.pattern").Exists())
	require.Equal(t, "(?=keep-default)", gjson.GetBytes(sanitized, "tools.0.parameters.properties.config.default.pattern").String())
	require.Equal(t, "(?=keep-example)", gjson.GetBytes(sanitized, "tools.0.parameters.properties.config.examples.0.pattern").String())
	require.Equal(t, "(?=keep-const)", gjson.GetBytes(sanitized, "tools.0.parameters.properties.config.const.pattern").String())
	require.Equal(t, "(?=keep-enum)", gjson.GetBytes(sanitized, "tools.0.parameters.properties.config.enum.0.pattern").String())
REDACTED

func buildToolSchemaNullTypeBody(t *testing.T, hits int) []byte {
REDACTED
	tools := make([]any, 0, hits)
	for i := 0; i < hits; i++ {
		tools = append(tools, map[string]any{
			"type":       "function",
			"name":       "automation_update",
			"parameters": map[string]any{"type": nil, "properties": map[string]any{REDACTEDREDACTED,
	REDACTED)
REDACTED
	body, err := json.Marshal(map[string]any{"model": "gpt-5.6-sol", "tools": toolsREDACTED)
REDACTED
	return body
REDACTED

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
REDACTED)
	largeAllocs := testing.AllocsPerRun(2, func() {
		_, _, _ = sanitizeOpenAIResponsesToolParameterTypes(large)
REDACTED)

	// 命中切片扩容是对数级，留出充裕余量；线性写法在这里会是 2000 量级。
	// 干净环境实测 large 约 17 allocs，200 是 10 倍余量，同时容忍 CI 慢 pod 上
	// 包内后台 goroutine（日志/ticker）对进程级 Mallocs 的噪声污染。
	require.Less(t, largeAllocs, 200.0,
		"分配次数随命中数线性增长，说明退回了逐路径全量重写 (small=%v large=%v)", smallAllocs, largeAllocs)

	// 同时确认大 body 的结果确实全部修好了。
	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(large)
REDACTED
	require.True(t, changed)
	require.Equal(t, 2000, int(gjson.GetBytes(sanitized, "tools.#").Int()))
	gjson.GetBytes(sanitized, "tools").ForEach(func(_, tool gjson.Result) bool {
		require.Equal(t, "object", tool.Get("parameters.type").String())
		return true
REDACTED)
REDACTED

func BenchmarkSanitizeOpenAIResponsesToolSchemaPatterns_ByteSpanPatch(b *testing.B) {
	var tools []any
	for i := 0; i < 2000; i++ {
		tools = append(tools, map[string]any{
			"type": "function",
			"name": "search",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string", "pattern": "(?=foo).*"REDACTEDREDACTED,
		REDACTED,
	REDACTED)
REDACTED
	body, _ := json.Marshal(map[string]any{"model": "gpt-5.6", "tools": toolsREDACTED)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = sanitizeOpenAIResponsesToolSchemaPatterns(body)
REDACTED
REDACTED

func BenchmarkSanitizeOpenAIResponsesToolParameterTypes_ByteSpanPatch(b *testing.B) {
	body := make([]byte, 0, 256*2000)
	body = append(body, []byte(`{"tools":[`)...)
	for i := 0; i < 2000; i++ {
		if i > 0 {
			body = append(body, ',')
	REDACTED
		body = append(body, []byte(`{"type":"function","name":"automation_update","parameters":{"properties":{REDACTED,"type":nullREDACTEDREDACTED`)...)
REDACTED
	body = append(body, []byte(`]REDACTED`)...)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = sanitizeOpenAIResponsesToolParameterTypes(body)
REDACTED
REDACTED

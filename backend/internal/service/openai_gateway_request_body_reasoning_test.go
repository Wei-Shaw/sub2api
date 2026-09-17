package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTrimOpenAIEncryptedReasoningItems_ContentNull(t *testing.T) {
	reqBody := map[string]any{
		"model": "grok-4.5",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "hi"},
			map[string]any{
				"type":              "reasoning",
				"summary":           []any{map[string]any{"type": "summary_text", "text": "thinking..."}},
				"content":           nil,
				"encrypted_content": nil,
			},
			map[string]any{"type": "message", "role": "assistant", "content": "Hello!"},
		},
	}

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	require.True(t, changed)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 3)

	reasoning, ok := input[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "reasoning", reasoning["type"])
	assert.NotNil(t, reasoning["summary"])
	_, hasContent := reasoning["content"]
	assert.False(t, hasContent, "content: null should be stripped")
	_, hasEncrypted := reasoning["encrypted_content"]
	assert.False(t, hasEncrypted, "encrypted_content should be stripped")
}

func TestTrimOpenAIEncryptedReasoningItems_ContentNullOnly(t *testing.T) {
	reqBody := map[string]any{
		"model": "grok-4.5",
		"input": []any{
			map[string]any{
				"type":    "reasoning",
				"summary": []any{map[string]any{"type": "summary_text", "text": "ok"}},
				"content": nil,
			},
		},
	}

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	require.True(t, changed)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 1)

	reasoning, ok := input[0].(map[string]any)
	require.True(t, ok)
	_, hasContent := reasoning["content"]
	assert.False(t, hasContent, "content: null should be stripped even without encrypted_content")
}

func TestTrimOpenAIEncryptedReasoningItems_ContentNonNull(t *testing.T) {
	reqBody := map[string]any{
		"model": "grok-4.5",
		"input": []any{
			map[string]any{
				"type":    "reasoning",
				"summary": []any{map[string]any{"type": "summary_text", "text": "ok"}},
				"content": "some actual content",
			},
		},
	}

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	assert.False(t, changed, "non-null content should not be stripped")

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	reasoning, ok := input[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "some actual content", reasoning["content"])
}

func TestTrimOpenAIEncryptedReasoningItems_NoReasoningItems(t *testing.T) {
	reqBody := map[string]any{
		"model": "grok-4.5",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "hi"},
		},
	}

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	assert.False(t, changed)
}

func TestTrimOpenAIEncryptedReasoningItems_Compaction(t *testing.T) {
	tests := []struct {
		name      string
		itemType  string
		encrypted bool
		changed   bool
	}{
		{name: "compaction", itemType: "compaction", encrypted: true, changed: true},
		{name: "compaction summary", itemType: "compaction_summary", encrypted: true, changed: true},
		{name: "unencrypted compaction", itemType: "compaction", encrypted: false, changed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := map[string]any{"type": tt.itemType, "id": "cmp_stale"}
			if tt.encrypted {
				item["encrypted_content"] = "gAAA"
			}
			reqBody := map[string]any{"input": []any{
				item,
				map[string]any{"type": "message", "content": "hi"},
			}}

			changed := trimOpenAIEncryptedReasoningItems(reqBody)
			assert.Equal(t, tt.changed, changed)
			input, ok := reqBody["input"].([]any)
			require.True(t, ok)
			require.NotEmpty(t, input)
			first, ok := input[0].(map[string]any)
			require.True(t, ok)
			if tt.changed {
				require.Len(t, input, 1)
				assert.Equal(t, "message", first["type"])
				return
			}
			require.Len(t, input, 2)
			assert.Equal(t, tt.itemType, first["type"])
		})
	}
}

func TestSanitizeOpenAICrossModeFailoverReasoning_DropsWholeEncryptedItem(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1","input":[` +
		`{"type":"message","role":"user","content":"hi"},` +
		`{"type":"reasoning","id":"rs_kiro_1","encrypted_content":"ENC","summary":[{"type":"summary_text","text":"t"}]},` +
		`{"type":"message","role":"assistant","content":"yo"}` +
		`]}`)

	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
	require.NoError(t, err)
	require.True(t, changed)
	// The whole reasoning item is gone — id and summary go with encrypted_content,
	// unlike trimOpenAIEncryptedReasoningItems which keeps the skeleton.
	require.NotContains(t, string(sanitized), "reasoning")
	require.NotContains(t, string(sanitized), "rs_kiro_1")
	require.NotContains(t, string(sanitized), "summary_text")
	require.Equal(t, int64(2), gjson.GetBytes(sanitized, "input.#").Int())
}

func TestSanitizeOpenAICrossModeFailoverReasoning_NoEncryptedIsNoop(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1","input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"t"}]}]}`)
	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
	require.NoError(t, err)
	require.False(t, changed, "reasoning without encrypted_content must be preserved")
	require.Equal(t, string(body), string(sanitized))
}

func TestSanitizeOpenAICrossModeFailoverReasoning_NoInputIsNoop(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1"}`)
	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, string(body), string(sanitized))
}

func TestSanitizeOpenAICrossModeFailoverReasoning_PreservesLargeIntegers(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1","input":[` +
		`{"type":"reasoning","id":"rs_kiro_1","encrypted_content":"ENC"},` +
		`{"type":"message","role":"user","content":"hi"}` +
		`],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"id":{"const":9007199254740993}}}}}]}`)

	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, string(sanitized), `"const":9007199254740993`,
		"sanitization must not round JSON integers through float64")
}

func TestTrimOpenAIEncryptedReasoningItems_ContentNullDropsBareSkeleton(t *testing.T) {
	reqBody := map[string]any{
		"input": []any{
			map[string]any{"type": "reasoning", "content": nil},
		},
	}

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	require.True(t, changed)
	_, hasInput := reqBody["input"]
	assert.False(t, hasInput, "bare reasoning skeleton should be dropped, emptying input")
}

func TestNormalizeOpenAIAPIKeyStoreFalseReasoningReplay(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","store":false,"input":[` +
		`{"type":"reasoning","id":"rs_encrypted","call_id":"remove","encrypted_content":"cipher","summary":null,"opaque":9007199254740993},` +
		`{"type":"reasoning","id":"rs_server_only","summary":[{"type":"summary_text","text":"drop"}]},` +
		`{"type":"item_reference","id":"rs_server_only"},` +
		`{"type":"item_reference","id":"msg_keep"},` +
		`{"type":"message","id":"msg_keep","role":"user","content":"continue"}` +
		`]}`)

	normalized, changed, err := normalizeOpenAIAPIKeyStoreFalseReasoningReplay(body, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, int64(3), gjson.GetBytes(normalized, "input.#").Int())
	require.Equal(t, "reasoning", gjson.GetBytes(normalized, "input.0.type").String())
	require.False(t, gjson.GetBytes(normalized, "input.0.id").Exists())
	require.False(t, gjson.GetBytes(normalized, "input.0.call_id").Exists())
	require.Equal(t, "cipher", gjson.GetBytes(normalized, "input.0.encrypted_content").String())
	require.True(t, gjson.GetBytes(normalized, "input.0.summary").IsArray())
	require.Equal(t, "9007199254740993", gjson.GetBytes(normalized, "input.0.opaque").Raw)
	require.Equal(t, "msg_keep", gjson.GetBytes(normalized, "input.1.id").String())
	require.Equal(t, "message", gjson.GetBytes(normalized, "input.2.type").String())
}

func TestNormalizeOpenAIAPIKeyStoreFalseReasoningReplayRequiresExplicitStoreFalse(t *testing.T) {
	for _, body := range []string{
		`{"input":[{"type":"reasoning","id":"rs_keep"}]}`,
		`{"store":true,"input":[{"type":"reasoning","id":"rs_keep"}]}`,
	} {
		normalized, changed, err := normalizeOpenAIAPIKeyStoreFalseReasoningReplay([]byte(body), false)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, string(normalized))
	}
}

func TestNormalizeOpenAIAPIKeyStoreFalseReasoningReplayKnownCompactMode(t *testing.T) {
	body := []byte(`{"input":[{"type":"reasoning","id":"rs_drop","summary":[]},{"type":"message","content":"continue"}]}`)

	normalized, changed, err := normalizeOpenAIAPIKeyStoreFalseReasoningReplay(body, true)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, int64(1), gjson.GetBytes(normalized, "input.#").Int())
	require.Equal(t, "message", gjson.GetBytes(normalized, "input.0.type").String())
}

func TestNormalizeOpenAIAPIKeyStoreFalseReasoningReplayRejectsEmptyEncryptedContent(t *testing.T) {
	for _, encrypted := range []string{"null", `""`, `"   "`, "123"} {
		body := []byte(`{"store":false,"input":[{"type":"reasoning","id":"rs_drop","encrypted_content":` + encrypted + `},{"type":"message","content":"continue"}]}`)
		normalized, changed, err := normalizeOpenAIAPIKeyStoreFalseReasoningReplay(body, false)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, int64(1), gjson.GetBytes(normalized, "input.#").Int())
		require.Equal(t, "message", gjson.GetBytes(normalized, "input.0.type").String())
	}
}

func TestNormalizeOpenAIParallelToolCallsWithoutTools(t *testing.T) {
	withTools := []byte(`{"tools":[{"type":"function","name":"lookup"}],"parallel_tool_calls":false}`)
	normalized, changed, err := normalizeOpenAIParallelToolCallsWithoutTools(withTools, false)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, string(withTools), string(normalized))

	withoutTools := []byte(`{"input":"hi","parallel_tool_calls":true}`)
	normalized, changed, err = normalizeOpenAIParallelToolCallsWithoutTools(withoutTools, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "parallel_tool_calls").Exists())
}

func TestFilterOpenAIResponsesNoneReasoningEffortForAccount(t *testing.T) {
	tests := []struct {
		name          string
		account       *Account
		body          string
		wantNested    bool
		wantFlat      bool
		wantSummary   bool
		wantReasoning bool
	}{
		{
			name:          "custom compatible endpoint strips none placeholders",
			account:       &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://compat.example/v1"}},
			body:          `{"reasoning":{"effort":"none"},"reasoning_effort":"NONE"}`,
			wantReasoning: false,
		},
		{
			name:          "third-party platform keeps other reasoning members",
			account:       &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey},
			body:          `{"reasoning":{"effort":" none ","summary":"auto"}}`,
			wantSummary:   true,
			wantReasoning: true,
		},
		{
			name:          "non-none effort is unchanged",
			account:       &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://compat.example/v1"}},
			body:          `{"reasoning":{"effort":"high"},"reasoning_effort":"low"}`,
			wantNested:    true,
			wantFlat:      true,
			wantReasoning: true,
		},
		{
			name:          "official OpenAI API key preserves none",
			account:       &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			body:          `{"reasoning":{"effort":"none"},"reasoning_effort":"none"}`,
			wantNested:    true,
			wantFlat:      true,
			wantReasoning: true,
		},
		{
			name:          "OpenAI OAuth preserves none",
			account:       &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			body:          `{"reasoning":{"effort":"none"}}`,
			wantNested:    true,
			wantReasoning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filterOpenAIResponsesNoneReasoningEffortForAccount(tt.account, []byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.wantNested, gjson.GetBytes(got, "reasoning.effort").Exists())
			require.Equal(t, tt.wantFlat, gjson.GetBytes(got, "reasoning_effort").Exists())
			require.Equal(t, tt.wantSummary, gjson.GetBytes(got, "reasoning.summary").Exists())
			require.Equal(t, tt.wantReasoning, gjson.GetBytes(got, "reasoning").Exists())
		})
	}
}

func TestFilterOpenAIResponsesNoneReasoningEffortForAccount_APIKeyAutomaticPassthroughPreservesRequest(t *testing.T) {
	body := []byte(`{"model":"qwen3.8-27b","input":"hi","max_output_tokens":20,"reasoning":{"effort":"none"},"presence_penalty":1.5}`)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://compat.example/v1",
		},
		Extra: map[string]any{"openai_passthrough": true},
	}

	got, err := filterOpenAIResponsesNoneReasoningEffortForAccount(account, body)

	require.NoError(t, err)
	require.JSONEq(t, string(body), string(got))
}

// Lite 工具迁移到 input[].additional_tools 后，仍应按有工具请求处理。
func TestNormalizeOpenAIParallelToolCallsWithoutTools_KeepsResponsesLiteAdditionalTools(t *testing.T) {
	liteBody := []byte(`{"input":[{"type":"message","role":"user","content":"hi"},{"type":"additional_tools","tools":[{"type":"function","name":"spawn_agent"}]}],"parallel_tool_calls":false}`)
	normalized, changed, err := normalizeOpenAIParallelToolCallsWithoutTools(liteBody, false)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, gjson.False, gjson.GetBytes(normalized, "parallel_tool_calls").Type)

	// 非 Lite 请求的空 additional_tools 不构成有效工具声明，字段仍需删除。
	emptyLiteBody := []byte(`{"input":[{"type":"additional_tools","tools":[]}],"parallel_tool_calls":true}`)
	normalized, changed, err = normalizeOpenAIParallelToolCallsWithoutTools(emptyLiteBody, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "parallel_tool_calls").Exists())

	// Lite 请求即使没有工具，也必须保留已经固定的 false。
	toolLessLiteBody := []byte(`{"input":"hi","parallel_tool_calls":false}`)
	normalized, changed, err = normalizeOpenAIParallelToolCallsWithoutTools(toolLessLiteBody, true)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, gjson.False, gjson.GetBytes(normalized, "parallel_tool_calls").Type)
}

func TestNormalizeOpenAIResponsesReasoningContentReplayStripsCrossProviderArray(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[` +
		`{"type":"message","role":"user","content":"one"},` +
		`{"type":"message","role":"assistant","content":"two"},` +
		`{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"call_1","output":"ok"},` +
		`{"type":"message","role":"user","content":"five"},` +
		`{"type":"reasoning","id":"rs_provider","summary":[{"type":"summary_text","text":"portable"}],"content":[{"type":"reasoning_text","text":"visible reasoning"}],"opaque":9007199254740993},` +
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}` +
		`]}`)

	normalized, changed, err := normalizeOpenAIResponsesReasoningContentReplay(body)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "reasoning", gjson.GetBytes(normalized, "input.5.type").String())
	require.False(t, gjson.GetBytes(normalized, "input.5.content").Exists())
	require.Equal(t, "portable", gjson.GetBytes(normalized, "input.5.summary.0.text").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(normalized, "input.5.opaque").Raw)
	require.Equal(t, "answer", gjson.GetBytes(normalized, "input.6.content.0.text").String())
}

func TestNormalizeOpenAIResponsesReasoningContentReplayKeepsPortableShapes(t *testing.T) {
	for _, body := range []string{
		`{"input":[{"type":"reasoning","summary":[]}]}`,
		`{"input":[{"type":"reasoning","content":[],"summary":[]}]}`,
		`{"input":[{"type":"message","content":[{"type":"input_text","text":"keep"}]}]}`,
	} {
		normalized, changed, err := normalizeOpenAIResponsesReasoningContentReplay([]byte(body))
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, body, string(normalized))
	}
}

func TestNormalizeOpenAIResponsesWebSocketCompatibilityBodyStripsReasoningContentOnlyForOpenAI(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"gpt-5.6-sol","store":true,"input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"keep"}],"content":[{"type":"reasoning_text","text":"remove"}]}]}`)
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		normalized, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{
			Platform: PlatformOpenAI,
			Type:     accountType,
		}, false)
		require.NoError(t, err)
		require.True(t, changed)
		require.False(t, gjson.GetBytes(normalized, "input.0.content").Exists())
		require.Equal(t, "keep", gjson.GetBytes(normalized, "input.0.summary.0.text").String())
	}

	normalized, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{
		Platform: PlatformZhipu,
		Type:     AccountTypeAPIKey,
	}, false)
	require.NoError(t, err)
	require.False(t, changed)
	require.JSONEq(t, string(body), string(normalized))
}

// deepSeekResponsesToolsFragment 是 DeepSeek 原生 Responses 请求的最小 tools 片段。
const deepSeekResponsesToolsFragment = `"tools":[{"type":"function","name":"shell","description":"run",` +
	`"parameters":{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"]}}]`

// assertAssistantMessagesGuarded 断言 input 里每条 assistant 消息前面都紧邻一条带非空明文
// 的 reasoning item（DeepSeek 原生 Responses 在携带 tools 时的硬性契约）。
func assertAssistantMessagesGuarded(t *testing.T, body []byte) {
	t.Helper()
	items := gjson.GetBytes(body, "input").Array()
	require.NotEmpty(t, items)
	// 补齐后每条 reasoning item 都必须带非空明文：DeepSeek 对 no-plain 的 reasoning item
	// 同样报 "The `reasoning_text` in the thinking mode must be passed back to the API."
	for i, item := range items {
		if strings.TrimSpace(item.Get("type").String()) != "reasoning" {
			continue
		}
		text := item.Get("content.0.text").String()
		require.NotEmptyf(t, text, "input.%d 的 reasoning 缺明文", i)
	}
	for i, item := range items {
		if responsesInputItemTypeFromJSON(item) != "message" || item.Get("role").String() != "assistant" {
			continue
		}
		require.Greaterf(t, i, 0, "assistant 消息不应位于 input 首位")
		prev := items[i-1]
		require.Equalf(t, "reasoning", prev.Get("type").String(), "input.%d 前应是 reasoning item", i)
		require.NotEmptyf(t, prev.Get("content.0.text").String(), "input.%d 的 reasoning 明文不得为空", i-1)
	}
}

func TestEnsureDeepSeekResponsesReasoningPlaceholders(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		hasTools    bool
		wantChanged bool
	}{
		{
			name: "缺失 reasoning 的 assistant 消息被补齐",
			body: `{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
				`{"type":"message","role":"user","content":"go"},` +
				`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":""}]},` +
				`{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},` +
				`{"type":"function_call_output","call_id":"c1","output":"ok"}]}`,
			hasTools:    true,
			wantChanged: true,
		},
		{
			name: "已有非空明文 reasoning 不改写",
			body: `{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
				`{"type":"message","role":"user","content":"go"},` +
				`{"type":"reasoning","id":"rs_1","summary":[],"content":[{"type":"reasoning_text","text":"think"}]},` +
				`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}`,
			hasTools:    true,
			wantChanged: false,
		},
		{
			name: "明文为空串视为缺失",
			body: `{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
				`{"type":"message","role":"user","content":"go"},` +
				`{"type":"reasoning","id":"rs_1","summary":[],"content":[{"type":"reasoning_text","text":""}]},` +
				`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}`,
			hasTools:    true,
			wantChanged: true,
		},
		{
			name: "只有 summary 视为缺失",
			body: `{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
				`{"type":"message","role":"user","content":"go"},` +
				`{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"portable"}]},` +
				`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}`,
			hasTools:    true,
			wantChanged: true,
		},
		{
			name: "只有 encrypted_content 视为缺失",
			body: `{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
				`{"type":"message","role":"user","content":"go"},` +
				`{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"opaque"},` +
				`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}`,
			hasTools:    true,
			wantChanged: true,
		},
		{
			// 线上第二种形态：no-plain 的 reasoning 后面直接跟工具调用，没有 assistant 消息
			// 可以承载新插入的占位 reasoning，必须给该 reasoning item 本身补明文。
			name: "无明文的 reasoning 后直接跟工具调用",
			body: `{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
				`{"type":"message","role":"user","content":"go"},` +
				`{"type":"reasoning","id":"rs_np","summary":[],"encrypted_content":"opaque"},` +
				`{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},` +
				`{"type":"function_call_output","call_id":"c1","output":"ok"}]}`,
			hasTools:    true,
			wantChanged: true,
		},
		{
			name: "同一轮第二条 assistant 消息也要补齐",
			body: `{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
				`{"type":"message","role":"user","content":"go"},` +
				`{"type":"reasoning","id":"rs_1","summary":[],"content":[{"type":"reasoning_text","text":"think"}]},` +
				`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"hi"}]},` +
				`{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},` +
				`{"type":"function_call_output","call_id":"c1","output":"ok"},` +
				`{"type":"message","id":"msg_2","role":"assistant","content":[{"type":"output_text","text":"again"}]}]}`,
			hasTools:    true,
			wantChanged: true,
		},
		{
			name: "user 开启新段后 assistant 消息重新需要 reasoning",
			body: `{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
				`{"type":"message","role":"user","content":"go"},` +
				`{"type":"reasoning","id":"rs_1","summary":[],"content":[{"type":"reasoning_text","text":"think"}]},` +
				`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"hi"}]},` +
				`{"type":"message","role":"user","content":"again"},` +
				`{"type":"message","id":"msg_2","role":"assistant","content":[{"type":"output_text","text":"second"}]}]}`,
			hasTools:    true,
			wantChanged: true,
		},
		{
			// Responses 允许 message 省略 type 字段；DeepSeek 按 role 识别这类 item，
			// 线上 400 就是这种形态：补齐必须同样认得出来。
			name: "assistant 消息省略 type 字段也要补齐",
			body: `{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
				`{"role":"user","content":"go"},` +
				`{"role":"assistant","content":[{"type":"output_text","text":""}]},` +
				`{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},` +
				`{"type":"function_call_output","call_id":"c1","output":"ok"}]}`,
			hasTools:    true,
			wantChanged: true,
		},
		{
			name: "省略 type 的 user 消息要能结束上一段的 reasoning 复用",
			body: `{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
				`{"type":"message","role":"user","content":"go"},` +
				`{"type":"reasoning","id":"rs_1","summary":[],"content":[{"type":"reasoning_text","text":"think"}]},` +
				`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"hi"}]},` +
				`{"role":"user","content":"again"},` +
				`{"role":"assistant","id":"msg_2","content":[{"type":"output_text","text":"second"}]}]}`,
			hasTools:    true,
			wantChanged: true,
		},
		{
			name: "没有 assistant 消息不改写",
			body: `{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
				`{"type":"message","role":"user","content":"go"}]}`,
			hasTools:    true,
			wantChanged: false,
		},
		{
			name: "请求不带 tools 时不补齐",
			body: `{"model":"deepseek-flash","input":[` +
				`{"type":"message","role":"user","content":"go"},` +
				`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}`,
			hasTools:    false,
			wantChanged: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rewritten, changed := ensureDeepSeekResponsesReasoningPlaceholders([]byte(tc.body), tc.hasTools)
			require.Equal(t, tc.wantChanged, changed)
			if tc.wantChanged {
				assertAssistantMessagesGuarded(t, rewritten)
				return
			}
			require.JSONEq(t, tc.body, string(rewritten))
		})
	}
}

func TestEnsureDeepSeekResponsesReasoningPlaceholdersIsIdempotent(t *testing.T) {
	body := []byte(`{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
		`{"type":"message","role":"user","content":"go"},` +
		`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":""}]},` +
		`{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"c1","output":"ok"},` +
		`{"type":"message","id":"msg_2","role":"assistant","content":[{"type":"output_text","text":"again"}]}]}`)

	first, changed := ensureDeepSeekResponsesReasoningPlaceholders(body, true)
	require.True(t, changed)
	// 占位 id 由被保护消息的 id 派生，因此重复改写同一份历史必须得到逐字节一致的结果：
	// 否则每轮 id 变化会让 DeepSeek 的上下文缓存前缀失效。
	again, changedAgain := ensureDeepSeekResponsesReasoningPlaceholders(body, true)
	require.True(t, changedAgain)
	require.JSONEq(t, string(first), string(again))
	require.Equal(t, "rs_ph_msg_1", gjson.GetBytes(first, "input.1.id").String())
	require.Equal(t, "rs_ph_msg_2", gjson.GetBytes(first, "input.5.id").String())

	// 已经补齐过的历史再走一遍必须判定为无改动，避免请求体在重试链路上不断膨胀。
	replayed, replayedChanged := ensureDeepSeekResponsesReasoningPlaceholders(first, true)
	require.False(t, replayedChanged)
	require.JSONEq(t, string(first), string(replayed))
}

func TestEnsureDeepSeekResponsesReasoningPlaceholdersFallsBackToIndexID(t *testing.T) {
	body := []byte(`{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
		`{"type":"message","role":"user","content":"go"},` +
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":""}]}]}`)

	rewritten, changed := ensureDeepSeekResponsesReasoningPlaceholders(body, true)
	require.True(t, changed)
	require.Equal(t, "rs_ph_1", gjson.GetBytes(rewritten, "input.1.id").String())
	require.Equal(t, " ", gjson.GetBytes(rewritten, "input.1.content.0.text").String())
	assertAssistantMessagesGuarded(t, rewritten)
}

// TestEnsureDeepSeekResponsesReasoningPlaceholdersPromotesSummary 覆盖跨模型切换后的真实形态：
// xAI 等上游只给 summary 不给 content，DeepSeek 不认 summary，导致
// "The `reasoning_text` in the thinking mode must be passed back to the API."。
// summary 里就是真实推理，提升为明文既满足校验又保住上下文。
func TestEnsureDeepSeekResponsesReasoningPlaceholdersPromotesSummary(t *testing.T) {
	withSummary := []byte(`{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
		`{"type":"message","role":"user","content":"go"},` +
		`{"type":"reasoning","id":"rs_s","summary":[{"type":"summary_text","text":"真实推理"}],"encrypted_content":"opaque"},` +
		`{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"c1","output":"ok"}]}`)

	rewritten, changed := ensureDeepSeekResponsesReasoningPlaceholders(withSummary, true)
	require.True(t, changed)
	require.Equal(t, "真实推理", gjson.GetBytes(rewritten, "input.1.content.0.text").String())
	require.False(t, gjson.GetBytes(rewritten, "input.1.summary").Exists(), "summary 对 DeepSeek 无意义，提升后应移除")

	// 没有 summary 时退回空格占位，仍然满足「每条 reasoning 都带非空明文」。
	withoutSummary := []byte(`{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
		`{"type":"message","role":"user","content":"go"},` +
		`{"type":"reasoning","id":"rs_e","summary":[],"encrypted_content":"opaque"},` +
		`{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"c1","output":"ok"}]}`)

	rewritten, changed = ensureDeepSeekResponsesReasoningPlaceholders(withoutSummary, true)
	require.True(t, changed)
	require.Equal(t, " ", gjson.GetBytes(rewritten, "input.1.content.0.text").String())

	// 幂等：提升过的历史再走一遍不再改写。
	again, changedAgain := ensureDeepSeekResponsesReasoningPlaceholders(rewritten, true)
	require.False(t, changedAgain)
	require.JSONEq(t, string(rewritten), string(again))
}

func TestNormalizeDeepSeekResponsesToolCallBlocks(t *testing.T) {
	callA := `{"type":"function_call","call_id":"call-a","name":"exec_command","arguments":"{}"}`
	callB := `{"type":"function_call","call_id":"call-b","name":"exec_command","arguments":"{}"}`
	outA := `{"type":"function_call_output","call_id":"call-a","output":"ok"}`
	outB := `{"type":"function_call_output","call_id":"call-b","output":"ok"}`
	user := `{"type":"message","role":"user","content":"go"}`
	commentary := `{"type":"message","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":"先执行"}]}`

	cases := []struct {
		name        string
		input       string
		wantChanged bool
	}{
		{
			// 线上形态：assistant message 夹在两个并行调用之间，DeepSeek 报
			// "No tool output found for tool call"，必须把插入项移到调用块之前。
			name:        "调用块内夹着 assistant 消息",
			input:       `[` + user + `,` + callA + `,` + commentary + `,` + callB + `,` + outA + `,` + outB + `]`,
			wantChanged: true,
		},
		{
			name:        "调用块内夹着 reasoning",
			input:       `[` + user + `,` + callA + `,` + `{"type":"reasoning","id":"rs_1","summary":[],"content":[{"type":"reasoning_text","text":"think"}]}` + `,` + callB + `,` + outA + `,` + outB + `]`,
			wantChanged: true,
		},
		{
			name:        "正常的并行调用块不改写",
			input:       `[` + user + `,` + callA + `,` + callB + `,` + outA + `,` + outB + `]`,
			wantChanged: false,
		},
		{
			name:        "串行调用块不改写",
			input:       `[` + user + `,` + callA + `,` + outA + `,` + callB + `,` + outB + `]`,
			wantChanged: false,
		},
		{
			name:        "孤立输出不改写",
			input:       `[` + user + `,` + outA + `]`,
			wantChanged: false,
		},
		{
			name:        "没有工具调用不改写",
			input:       `[` + user + `,` + commentary + `]`,
			wantChanged: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"deepseek-flash","input":` + tc.input + `}`)
			rewritten, changed := normalizeDeepSeekResponsesToolCallBlocks(body)
			require.Equal(t, tc.wantChanged, changed)
			if !tc.wantChanged {
				require.JSONEq(t, string(body), string(rewritten))
				return
			}
			// 改写后检查 input 顺序，并确认没有 item 丢失。
			items := gjson.GetBytes(rewritten, "input").Array()
			require.Len(t, items, len(gjson.GetBytes(body, "input").Array()))
			assertToolBlocksContiguous(t, rewritten)
		})
	}
}

// assertToolBlocksContiguous 断言 input 里每个工具调用块内部只含调用与输出：
// 块一旦开始，出现非调用/输出 item 即视为仍被夹住。
func assertToolBlocksContiguous(t *testing.T, body []byte) {
	t.Helper()
	items := gjson.GetBytes(body, "input").Array()
	for index := 0; index < len(items); {
		if !isOpenAIResponsesToolCallType(strings.TrimSpace(items[index].Get("type").String())) {
			index++
			continue
		}
		callIDs := make(map[string]struct{}, 4)
		pending := make(map[string]struct{}, 4)
		for index < len(items) {
			itemType := strings.TrimSpace(items[index].Get("type").String())
			callID := strings.TrimSpace(items[index].Get("call_id").String())
			switch {
			case isOpenAIResponsesToolCallType(itemType):
				callIDs[callID] = struct{}{}
				pending[callID] = struct{}{}
				index++
			case isOpenAIResponsesToolOutputType(itemType):
				if _, ok := callIDs[callID]; !ok {
					return
				}
				delete(pending, callID)
				index++
				if len(pending) == 0 {
					goto nextBlock
				}
			default:
				if len(pending) > 0 {
					t.Fatalf("调用块内仍夹着 %q（下标 %d）", itemType, index)
				}
				goto nextBlock
			}
		}
	nextBlock:
	}
}

func TestNormalizeDeepSeekResponsesToolCallBlocksNoItemLoss(t *testing.T) {
	// 用「调用块内插入 + 块外消息」的混合形态确认重排只换位置、不增删 item。
	input := `[
		{"type":"message","role":"user","content":"go"},
		{"type":"function_call","call_id":"call-a","name":"exec_command","arguments":"{}"},
		{"type":"reasoning","id":"rs_1","summary":[],"content":[{"type":"reasoning_text","text":"think"}]},
		{"type":"function_call","call_id":"call-b","name":"exec_command","arguments":"{}"},
		{"type":"function_call_output","call_id":"call-a","output":"ok"},
		{"type":"function_call_output","call_id":"call-b","output":"ok"},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}
	]`
	body := []byte(`{"model":"deepseek-flash","input":` + input + `}`)

	rewritten, changed := normalizeDeepSeekResponsesToolCallBlocks(body)
	require.True(t, changed)
	items := gjson.GetBytes(rewritten, "input").Array()
	require.Len(t, items, 7)
	require.Equal(t, "user", items[0].Get("role").String())
	// 插入项被移到调用块之前，调用与输出连续。
	require.Equal(t, "reasoning", items[1].Get("type").String())
	require.Equal(t, "call-a", items[2].Get("call_id").String())
	require.Equal(t, "call-b", items[3].Get("call_id").String())
	require.Equal(t, "call-a", items[4].Get("call_id").String())
	require.Equal(t, "call-b", items[5].Get("call_id").String())
}

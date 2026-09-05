package service

import (
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

package securityaudit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPresetFilterEnvelopesAndInteraction(t *testing.T) {
	for _, tc := range []struct{ name, protocol, body, want string }{
		{"responses", "responses", `{"instructions":"preset","tools":[{"description":"preset"}],"input":[{"role":"developer","content":"preset"},{"role":"user","id":"u","content":"keep","internal_chat_message_metadata_passthrough":{"secret":"drop"}},{"type":"function_call_output","output":"drop tool data"}],"seed":9007199254740993}`, `{"input":[{"role":"user","content":"keep"}]}`},
		{"websocket", "responses_websocket", `{"type":"response.create","client_metadata":{"drop":true},"response":{"instructions":"preset","input":[{"role":"system","content":"preset"},{"role":"user","content":"keep"}]}}`, `{"type":"response.create","response":{"input":[{"role":"user","content":"keep"}]}}`},
		{"claude", "anthropic_messages", `{"system":"You are Claude Code","messages":[{"role":"user","content":[{"type":"text","text":"<system-reminder>preset</system-reminder>\nkeep","cache_control":{"type":"ephemeral"}},{"type":"tool_result","tool_use_id":"x","content":"drop tool data"}]},{"role":"assistant","content":"keep reply","id":"a"}]}`, `{"messages":[{"role":"user","content":[{"type":"text","text":"keep"}]},{"role":"assistant","content":"keep reply"}]}`},
		{"codex", "responses", `{"input":[{"role":"developer","content":"You are Codex"},{"role":"user","content":"# AGENTS.md instructions\n<INSTRUCTIONS>preset</INSTRUCTIONS>\n<environment_context>preset</environment_context>\nkeep"},{"type":"reasoning","encrypted_content":"drop"}]}`, `{"input":[{"role":"user","content":"keep"}]}`},
		{"gemini batch", "gemini", `{"requests":[{"systemInstruction":{"parts":[{"text":"preset"}]},"contents":[{"role":"model","parts":[{"text":"reply","thoughtSignature":"drop"}]},{"role":"user","parts":[{"text":"keep","metadata":"drop"}]}]}]}`, `{"requests":[{"contents":[{"role":"model","parts":[{"text":"reply"}]},{"role":"user","parts":[{"text":"keep"}]}]}]}`},
		{"unknown agent", "openai_chat", `{"messages":[{"role":"system","content":"preset"},{"role":"user","content":"<system-reminder>actual user data</system-reminder>"}]}`, `{"messages":[{"role":"user","content":"<system-reminder>actual user data</system-reminder>"}]}`},
		{"quoted", "anthropic_messages", `{"system":"You are Claude Code","messages":[{"role":"user","content":"Explain this: <system-reminder>keep</system-reminder>"}]}`, `{"messages":[{"role":"user","content":"Explain this: <system-reminder>keep</system-reminder>"}]}`},
		{"multimodal", "openai_chat", `{"model":"drop","messages":[{"role":"system","content":"preset"},{"role":"user","content":[{"type":"input_text","text":"keep","metadata":"drop"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abcd"}}]}]}`, `{"messages":[{"role":"user","content":[{"type":"input_text","text":"keep"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abcd"}}]}]}`},
		{"media prompts", "grok_media", `{"model":"drop","prompt":"draw a lighthouse","image":"data:image/png;base64,IMAGE","input":{"negative_prompt":"no fog","image_prompt":"https://example.test/input.png"},"request":{"lyrics":"ocean song","seed":42}}`, `{"input":{"negative_prompt":"no fog"},"prompt":"draw a lighthouse","request":{"lyrics":"ocean song"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			require.JSONEq(t, tc.want, string(filterPresetRequestBody(tc.protocol, body, true)))
			require.Equal(t, tc.body, string(body))
			require.Equal(t, tc.body, string(filterPresetRequestBody(tc.protocol, body, false)))
		})
	}
	for _, body := range []string{"invalid", "null", `"plain text"`} {
		require.Equal(t, body, string(filterPresetRequestBody("", []byte(body), true)))
	}
}

func TestPresetFilterProductionRequestFixtures(t *testing.T) {
	preset := strings.Repeat("preset instruction ", 4000)
	mustMarshal := func(value any) []byte {
		body, err := json.Marshal(value)
		require.NoError(t, err)
		return body
	}
	for _, tc := range []struct {
		name            string
		body            []byte
		mustContain     []string
		maxPercentOfRaw int
		wantRoot        string
		wantRoles       []string
	}{
		{
			name: "codex responses",
			body: mustMarshal(map[string]any{
				"instructions": "You are Codex\n" + preset,
				"tools":        []any{map[string]any{"type": "function", "description": preset}},
				"input": []any{
					map[string]any{"role": "developer", "content": "You are Codex\n" + preset},
					map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "# AGENTS.md instructions\n<INSTRUCTIONS>" + preset + "</INSTRUCTIONS>\n<environment_context>" + preset + "</environment_context>\n当前环境"}}, "internal_chat_message_metadata_passthrough": map[string]any{"secret": "drop"}},
					map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "你是谁"}}},
					map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "当前运行环境"}}},
					map[string]any{"type": "reasoning", "encrypted_content": preset},
				},
				"client_metadata":  map[string]any{"drop": true},
				"prompt_cache_key": "drop",
			}),
			mustContain: []string{"当前环境", "你是谁", "当前运行环境"}, maxPercentOfRaw: 5, wantRoot: "input",
			wantRoles: []string{"user", "assistant", "user"},
		},
		{
			name: "claude messages",
			body: mustMarshal(map[string]any{
				"system": "You are Claude Code\n" + preset,
				"tools":  []any{map[string]any{"name": "tool", "description": preset}},
				"messages": []any{
					map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "<system-reminder>" + preset + "</system-reminder>\n当前环境", "cache_control": map[string]any{"type": "ephemeral"}}}},
					map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "name": "drop", "input": preset}}},
				},
			}),
			mustContain: []string{"当前环境"}, maxPercentOfRaw: 10, wantRoot: "messages", wantRoles: []string{"user"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filtered := filterPresetRequestBody("openai_responses", tc.body, true)
			for _, expected := range tc.mustContain {
				require.Contains(t, string(filtered), expected)
			}
			for _, omitted := range []string{"additional_tools", `"role":"developer"`, `"type":"reasoning"`, "encrypted_content", "client_metadata", "internal_chat_message_metadata_passthrough", "prompt_cache_key", "# AGENTS.md instructions"} {
				require.NotContains(t, string(filtered), omitted)
			}
			require.LessOrEqual(t, len(filtered)*100, len(tc.body)*tc.maxPercentOfRaw)
			t.Logf("%s: %d bytes -> %d bytes (%.2f%%)", tc.name, len(tc.body), len(filtered), float64(len(filtered))*100/float64(len(tc.body)))
			var root map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(filtered, &root))
			require.Len(t, root, 1)
			require.Contains(t, root, tc.wantRoot)
			var messages []map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(root[tc.wantRoot], &messages))
			require.Len(t, messages, len(tc.wantRoles))
			for index, message := range messages {
				require.Len(t, message, 2)
				require.Contains(t, message, "role")
				require.Contains(t, message, "content")
				role, ok := rawString(message["role"])
				require.True(t, ok)
				require.Equal(t, tc.wantRoles[index], role)
			}
		})
	}
}

func TestPresetRecordingKeepsResponseIdentityAndAuditInput(t *testing.T) {
	settings := &promptRecordingSettingRepository{}
	manager := NewConfigManager(nil, settings, nil, prefixEncryptor{}, testTotpKeyConfig())
	ctx := context.Background()
	require.NoError(t, manager.Reload(ctx))
	require.False(t, manager.PromptRecordingFilterPreset())
	enabled := true
	require.NoError(t, manager.SavePromptRecordingSettings(ctx, nil, nil, nil, &enabled))
	require.NoError(t, manager.Reload(ctx))
	require.True(t, manager.PromptRecordingFilterPreset())
	for _, body := range []string{`{"messages":[{"role":"system","content":"preset"},{"role":"user","content":"keep"}]}`, `{"system":"preset"}`} {
		repo := &capturedRequestRepository{records: make(chan *PromptRecord, 1)}
		records := newPromptRecordService(repo, 1, 1, 1)
		service := &PromptService{config: manager, records: records}
		req := Request{RequestID: "filter-response", Protocol: "anthropic_messages", Body: []byte(body)}
		service.RecordPrompt(ctx, req)
		select {
		case record := <-repo.records:
			require.NotContains(t, record.RequestBody, "preset")
			require.NotContains(t, record.PromptText, "preset")
			require.Contains(t, promptRecordSnapshot(req).FullPrompt, "preset")
			records.persistResponse(req, PromptResponse{Text: "response", CapturedAt: time.Now()})
			require.Equal(t, record.PromptHash, repo.responseHash)
		case <-time.After(3 * time.Second):
			t.Fatal("record not persisted")
		}
	}
}

func TestPresetRecordingEndpoint(t *testing.T) {
	service := &fakePromptAdminService{recording: PromptRecordingConfig{Enabled: true, HeadersEnabled: true, PromptEnabled: true}}
	router := promptAdminRouter(service)
	for _, value := range []bool{true, false} {
		result := promptAdminRequest(t, router, "PUT", "/admin/prompt-records/recording", map[string]any{"filter_preset": value})
		require.Equal(t, 200, result.Code)
		require.Equal(t, value, service.recording.FilterPreset)
		require.True(t, service.recording.HeadersEnabled)
		require.True(t, service.recording.PromptEnabled)
	}
}

func TestPresetFilterSingleInputs(t *testing.T) {
	for _, input := range []string{`"<environment_context>preset</environment_context>\nkeep"`, `{"role":"user","content":"<environment_context>preset</environment_context>\nkeep"}`} {
		filtered := filterPresetRequestBody("responses", []byte(`{"instructions":"You are Codex","input":`+input+`}`), true)
		require.NotContains(t, string(filtered), "preset")
		require.Contains(t, string(filtered), "keep")
	}
}

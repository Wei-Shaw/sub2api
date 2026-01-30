package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeOpenCodeText_RewritesCanonicalSentence(t *testing.T) {
	in := "You are OpenCode, the best coding agent on the planet."
	got := sanitizeSystemText(in)
	require.Equal(t, strings.TrimSpace(claudeCodeSystemPrompt), got)
REDACTED

func TestSanitizeToolDescription_DoesNotRewriteKeywords(t *testing.T) {
	in := "OpenCode and opencode are mentioned."
	got := sanitizeToolDescription(in)
	// We no longer rewrite tool descriptions; only redact obvious path leaks.
	require.Equal(t, in, got)
REDACTED

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeOpenCodeText_RewritesCanonicalSentence(t *testing.T) {
	in := "You are OpenCode, the best coding agent on the planet."
	got := sanitizeOpenCodeText(in)
	require.Equal(t, strings.TrimSpace(claudeCodeSystemPrompt), got)
REDACTED

func TestSanitizeOpenCodeText_RewritesOpenCodeKeywords(t *testing.T) {
	in := "OpenCode and opencode are mentioned."
	got := sanitizeOpenCodeText(in)
	require.Equal(t, "Claude Code and Claude are mentioned.", got)
REDACTED

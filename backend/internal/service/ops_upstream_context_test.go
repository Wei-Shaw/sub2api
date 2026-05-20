package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeUpstreamURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
REDACTED{
		{"strips query", "https://api.anthropic.com/v1/messages?beta=true", "https://api.anthropic.com/v1/messages"REDACTED,
		{"strips fragment", "https://api.openai.com/v1/responses#frag", "https://api.openai.com/v1/responses"REDACTED,
		{"strips both", "https://host/path?token=secret#x", "https://host/path"REDACTED,
		{"no query or fragment", "https://host/path", "https://host/path"REDACTED,
		{"empty string", "", ""REDACTED,
		{"whitespace only", "  ", ""REDACTED,
		{"query before fragment", "https://h/p?a=1#f", "https://h/p"REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, safeUpstreamURL(tt.input))
	REDACTED)
REDACTED
REDACTED

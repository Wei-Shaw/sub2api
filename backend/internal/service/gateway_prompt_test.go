package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsClaudeCodeClient(t *testing.T) {
	tests := []struct {
		name           string
		userAgent      string
		metadataUserID string
		want           bool
REDACTED{
		{
			name:           "Claude Code client",
			userAgent:      "claude-cli/1.0.62 (darwin; arm64)",
			metadataUserID: "session_123e4567-e89b-12d3-a456-426614174000",
			want:           true,
	REDACTED,
		{
			name:           "Claude Code without version suffix",
			userAgent:      "claude-cli/2.0.0",
			metadataUserID: "session_abc",
			want:           true,
	REDACTED,
		{
			name:           "Missing metadata user_id",
			userAgent:      "claude-cli/1.0.0",
			metadataUserID: "",
			want:           false,
	REDACTED,
		{
			name:           "Different user agent",
			userAgent:      "curl/7.68.0",
			metadataUserID: "user123",
			want:           false,
	REDACTED,
		{
			name:           "Empty user agent",
			userAgent:      "",
			metadataUserID: "user123",
			want:           false,
	REDACTED,
		{
			name:           "Similar but not Claude CLI",
			userAgent:      "claude-api/1.0.0",
			metadataUserID: "user123",
			want:           false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isClaudeCodeClient(tt.userAgent, tt.metadataUserID)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

func TestSystemIncludesClaudeCodePrompt(t *testing.T) {
	tests := []struct {
		name   string
		system any
		want   bool
REDACTED{
		{
			name:   "nil system",
			system: nil,
			want:   false,
	REDACTED,
		{
			name:   "empty string",
			system: "",
			want:   false,
	REDACTED,
		{
			name:   "string with Claude Code prompt",
			system: claudeCodeSystemPrompt,
			want:   true,
	REDACTED,
		{
			name:   "string with different content",
			system: "You are a helpful assistant.",
			want:   false,
	REDACTED,
		{
			name:   "empty array",
			system: []any{REDACTED,
			want:   false,
	REDACTED,
		{
			name: "array with Claude Code prompt",
			system: []any{
				map[string]any{
					"type": "text",
					"text": claudeCodeSystemPrompt,
			REDACTED,
		REDACTED,
			want: true,
	REDACTED,
		{
			name: "array with Claude Code prompt in second position",
			system: []any{
				map[string]any{"type": "text", "text": "First prompt"REDACTED,
				map[string]any{"type": "text", "text": claudeCodeSystemPromptREDACTED,
		REDACTED,
			want: true,
	REDACTED,
		{
			name: "array without Claude Code prompt",
			system: []any{
				map[string]any{"type": "text", "text": "Custom prompt"REDACTED,
		REDACTED,
			want: false,
	REDACTED,
		{
			name: "array with partial match (should not match)",
			system: []any{
				map[string]any{"type": "text", "text": "You are Claude"REDACTED,
		REDACTED,
			want: false,
	REDACTED,
		// json.RawMessage cases (conversion path: ForwardAsResponses / ForwardAsChatCompletions)
		{
			name:   "json.RawMessage string with Claude Code prompt",
			system: json.RawMessage(`"` + claudeCodeSystemPrompt + `"`),
			want:   true,
	REDACTED,
		{
			name:   "json.RawMessage string without Claude Code prompt",
			system: json.RawMessage(`"You are a helpful assistant"`),
			want:   false,
	REDACTED,
		{
			name:   "json.RawMessage nil (empty)",
			system: json.RawMessage(nil),
			want:   false,
	REDACTED,
		{
			name:   "json.RawMessage empty string",
			system: json.RawMessage(`""`),
			want:   false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := systemIncludesClaudeCodePrompt(tt.system)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

func TestInjectClaudeCodePrompt(t *testing.T) {
	claudePrefix := strings.TrimSpace(claudeCodeSystemPrompt)

	tests := []struct {
		name           string
		body           string
		system         any
		wantSystemLen  int
		wantFirstText  string
		wantSecondText string
REDACTED{
		{
			name:          "nil system",
			body:          `{"model":"claude-3"REDACTED`,
			system:        nil,
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
	REDACTED,
		{
			name:          "empty string system",
			body:          `{"model":"claude-3"REDACTED`,
			system:        "",
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
	REDACTED,
		{
			name:           "string system",
			body:           `{"model":"claude-3"REDACTED`,
			system:         "Custom prompt",
			wantSystemLen:  2,
			wantFirstText:  claudeCodeSystemPrompt,
			wantSecondText: claudePrefix + "\n\nCustom prompt",
	REDACTED,
		{
			name:          "string system equals Claude Code prompt",
			body:          `{"model":"claude-3"REDACTED`,
			system:        claudeCodeSystemPrompt,
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
	REDACTED,
		{
			name:   "array system",
			body:   `{"model":"claude-3"REDACTED`,
			system: []any{map[string]any{"type": "text", "text": "Custom"REDACTEDREDACTED,
			// Claude Code + Custom = 2
			wantSystemLen:  2,
			wantFirstText:  claudeCodeSystemPrompt,
			wantSecondText: claudePrefix + "\n\nCustom",
	REDACTED,
		{
			name: "array system with existing Claude Code prompt (should dedupe)",
			body: `{"model":"claude-3"REDACTED`,
			system: []any{
				map[string]any{"type": "text", "text": claudeCodeSystemPromptREDACTED,
				map[string]any{"type": "text", "text": "Other"REDACTED,
		REDACTED,
			// Claude Code at start + Other = 2 (deduped)
			wantSystemLen:  2,
			wantFirstText:  claudeCodeSystemPrompt,
			wantSecondText: claudePrefix + "\n\nOther",
	REDACTED,
		{
			name:          "empty array",
			body:          `{"model":"claude-3"REDACTED`,
			system:        []any{REDACTED,
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
	REDACTED,
		// json.RawMessage cases (conversion path: ForwardAsResponses / ForwardAsChatCompletions)
		{
			name:           "json.RawMessage string system",
			body:           `{"model":"claude-3","system":"Custom prompt"REDACTED`,
			system:         json.RawMessage(`"Custom prompt"`),
			wantSystemLen:  2,
			wantFirstText:  claudeCodeSystemPrompt,
			wantSecondText: claudePrefix + "\n\nCustom prompt",
	REDACTED,
		{
			name:          "json.RawMessage nil system",
			body:          `{"model":"claude-3"REDACTED`,
			system:        json.RawMessage(nil),
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
	REDACTED,
		{
			name:          "json.RawMessage Claude Code prompt (should not duplicate)",
			body:          `{"model":"claude-3","system":"` + claudeCodeSystemPrompt + `"REDACTED`,
			system:        json.RawMessage(`"` + claudeCodeSystemPrompt + `"`),
			wantSystemLen: 1,
			wantFirstText: claudeCodeSystemPrompt,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injectClaudeCodePrompt([]byte(tt.body), tt.system)

			var parsed map[string]any
			err := json.Unmarshal(result, &parsed)
		REDACTED

			system, ok := parsed["system"].([]any)
			require.True(t, ok, "system should be an array")
			require.Len(t, system, tt.wantSystemLen)

			first, ok := system[0].(map[string]any)
			require.True(t, ok)
			require.Equal(t, tt.wantFirstText, first["text"])
			require.Equal(t, "text", first["type"])

			// Check cache_control
			cc, ok := first["cache_control"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "ephemeral", cc["type"])

			if tt.wantSecondText != "" && len(system) > 1 {
				second, ok := system[1].(map[string]any)
				require.True(t, ok)
				require.Equal(t, tt.wantSecondText, second["text"])
		REDACTED
	REDACTED)
REDACTED
REDACTED

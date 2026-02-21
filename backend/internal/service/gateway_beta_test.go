package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeAnthropicBeta(t *testing.T) {
	got := mergeAnthropicBeta(
		[]string{"oauth-2025-04-20", "REDACTED"REDACTED,
		"foo, oauth-2025-04-20,bar, foo",
	)
	require.Equal(t, "oauth-2025-04-20,REDACTED,foo,bar", got)
REDACTED

func TestMergeAnthropicBeta_EmptyIncoming(t *testing.T) {
	got := mergeAnthropicBeta(
		[]string{"oauth-2025-04-20", "REDACTED"REDACTED,
		"",
	)
	require.Equal(t, "oauth-2025-04-20,REDACTED", got)
REDACTED

func TestStripBetaToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		token  string
		want   string
REDACTED{
		{
			name:   "token in middle",
			header: "oauth-2025-04-20,context-1m-2025-08-07,REDACTED",
			token:  "context-1m-2025-08-07",
			want:   "oauth-2025-04-20,REDACTED",
	REDACTED,
		{
			name:   "token at start",
			header: "context-1m-2025-08-07,oauth-2025-04-20,REDACTED",
			token:  "context-1m-2025-08-07",
			want:   "oauth-2025-04-20,REDACTED",
	REDACTED,
		{
			name:   "token at end",
			header: "oauth-2025-04-20,REDACTED,context-1m-2025-08-07",
			token:  "context-1m-2025-08-07",
			want:   "oauth-2025-04-20,REDACTED",
	REDACTED,
		{
			name:   "token not present",
			header: "oauth-2025-04-20,REDACTED",
			token:  "context-1m-2025-08-07",
			want:   "oauth-2025-04-20,REDACTED",
	REDACTED,
		{
			name:   "empty header",
			header: "",
			token:  "context-1m-2025-08-07",
			want:   "",
	REDACTED,
		{
			name:   "with spaces",
			header: "oauth-2025-04-20, context-1m-2025-08-07 , REDACTED",
			token:  "context-1m-2025-08-07",
			want:   "oauth-2025-04-20,REDACTED",
	REDACTED,
		{
			name:   "only token",
			header: "context-1m-2025-08-07",
			token:  "context-1m-2025-08-07",
			want:   "",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripBetaToken(tt.header, tt.token)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

func TestMergeAnthropicBetaDropping_Context1M(t *testing.T) {
	required := []string{"oauth-2025-04-20", "REDACTED"REDACTED
	incoming := "context-1m-2025-08-07,foo-beta,oauth-2025-04-20"
	drop := map[string]struct{REDACTED{"context-1m-2025-08-07": {REDACTEDREDACTED

	got := mergeAnthropicBetaDropping(required, incoming, drop)
	require.Equal(t, "oauth-2025-04-20,REDACTED,foo-beta", got)
	require.NotContains(t, got, "context-1m-2025-08-07")
REDACTED

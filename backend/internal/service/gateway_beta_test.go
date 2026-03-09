package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"

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

func TestStripBetaTokens(t *testing.T) {
	tests := []struct {
		name   string
		header string
		tokens []string
		want   string
REDACTED{
		{
			name:   "single token in middle",
			header: "oauth-2025-04-20,context-1m-2025-08-07,REDACTED",
			tokens: []string{"context-1m-2025-08-07"REDACTED,
			want:   "oauth-2025-04-20,REDACTED",
	REDACTED,
		{
			name:   "single token at start",
			header: "context-1m-2025-08-07,oauth-2025-04-20,REDACTED",
			tokens: []string{"context-1m-2025-08-07"REDACTED,
			want:   "oauth-2025-04-20,REDACTED",
	REDACTED,
		{
			name:   "single token at end",
			header: "oauth-2025-04-20,REDACTED,context-1m-2025-08-07",
			tokens: []string{"context-1m-2025-08-07"REDACTED,
			want:   "oauth-2025-04-20,REDACTED",
	REDACTED,
		{
			name:   "token not present",
			header: "oauth-2025-04-20,REDACTED",
			tokens: []string{"context-1m-2025-08-07"REDACTED,
			want:   "oauth-2025-04-20,REDACTED",
	REDACTED,
		{
			name:   "empty header",
			header: "",
			tokens: []string{"context-1m-2025-08-07"REDACTED,
			want:   "",
	REDACTED,
		{
			name:   "with spaces",
			header: "oauth-2025-04-20, context-1m-2025-08-07 , REDACTED",
			tokens: []string{"context-1m-2025-08-07"REDACTED,
			want:   "oauth-2025-04-20,REDACTED",
	REDACTED,
		{
			name:   "only token",
			header: "context-1m-2025-08-07",
			tokens: []string{"context-1m-2025-08-07"REDACTED,
			want:   "",
	REDACTED,
		{
			name:   "nil tokens",
			header: "oauth-2025-04-20,REDACTED",
			tokens: nil,
			want:   "oauth-2025-04-20,REDACTED",
	REDACTED,
		{
			name:   "multiple tokens removed",
			header: "oauth-2025-04-20,context-1m-2025-08-07,REDACTED,fast-mode-2026-02-01",
			tokens: []string{"context-1m-2025-08-07", "fast-mode-2026-02-01"REDACTED,
			want:   "oauth-2025-04-20,REDACTED",
	REDACTED,
		{
			name:   "DroppedBetas removes fast-mode only",
			header: "oauth-2025-04-20,context-1m-2025-08-07,fast-mode-2026-02-01,REDACTED",
			tokens: claude.DroppedBetas,
			want:   "oauth-2025-04-20,context-1m-2025-08-07,REDACTED",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripBetaTokens(tt.header, tt.tokens)
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

func TestMergeAnthropicBetaDropping_DroppedBetas(t *testing.T) {
	required := []string{"oauth-2025-04-20", "REDACTED"REDACTED
	incoming := "context-1m-2025-08-07,fast-mode-2026-02-01,foo-beta,oauth-2025-04-20"
	drop := droppedBetaSet()

	got := mergeAnthropicBetaDropping(required, incoming, drop)
	require.Equal(t, "oauth-2025-04-20,REDACTED,context-1m-2025-08-07,foo-beta", got)
	require.Contains(t, got, "context-1m-2025-08-07")
	require.NotContains(t, got, "fast-mode-2026-02-01")
REDACTED

func TestDroppedBetaSet(t *testing.T) {
	// Base set contains DroppedBetas
	base := droppedBetaSet()
	require.NotContains(t, base, claude.BetaContext1M)
	require.Contains(t, base, claude.BetaFastMode)
	require.Len(t, base, len(claude.DroppedBetas))

	// With extra tokens
	extended := droppedBetaSet(claude.BetaClaudeCode)
	require.NotContains(t, extended, claude.BetaContext1M)
	require.Contains(t, extended, claude.BetaFastMode)
	require.Contains(t, extended, claude.BetaClaudeCode)
	require.Len(t, extended, len(claude.DroppedBetas)+1)
REDACTED

func TestBuildBetaTokenSet(t *testing.T) {
	got := buildBetaTokenSet([]string{"foo", "", "bar", "foo"REDACTED)
	require.Len(t, got, 2)
	require.Contains(t, got, "foo")
	require.Contains(t, got, "bar")
	require.NotContains(t, got, "")

	empty := buildBetaTokenSet(nil)
	require.Empty(t, empty)
REDACTED

func TestStripBetaTokensWithSet_EmptyDropSet(t *testing.T) {
	header := "oauth-2025-04-20,REDACTED"
	got := stripBetaTokensWithSet(header, map[string]struct{REDACTED{REDACTED)
	require.Equal(t, header, got)
REDACTED

func TestIsCountTokensUnsupported404(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       bool
REDACTED{
		{
			name:       "exact endpoint not found",
			statusCode: 404,
			body:       `{"error":{"message":"Not found: /v1/messages/count_tokens","type":"not_found_error"REDACTEDREDACTED`,
			want:       true,
	REDACTED,
		{
			name:       "contains count_tokens and not found",
			statusCode: 404,
			body:       `{"error":{"message":"count_tokens route not found","type":"not_found_error"REDACTEDREDACTED`,
			want:       true,
	REDACTED,
		{
			name:       "generic 404",
			statusCode: 404,
			body:       `{"error":{"message":"resource not found","type":"not_found_error"REDACTEDREDACTED`,
			want:       false,
	REDACTED,
		{
			name:       "404 with empty error message",
			statusCode: 404,
			body:       `{"error":{"message":"","type":"not_found_error"REDACTEDREDACTED`,
			want:       false,
	REDACTED,
		{
			name:       "non-404 status",
			statusCode: 400,
			body:       `{"error":{"message":"Not found: /v1/messages/count_tokens","type":"invalid_request_error"REDACTEDREDACTED`,
			want:       false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCountTokensUnsupported404(tt.statusCode, []byte(tt.body))
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

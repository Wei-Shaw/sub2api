package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
)

func assertJSONTokenOrder(t *testing.T, body string, tokens ...string) {
REDACTED

	last := -1
	for _, token := range tokens {
		pos := strings.Index(body, token)
		require.NotEqualf(t, -1, pos, "missing token %s in body %s", token, body)
		require.Greaterf(t, pos, last, "token %s should appear after previous tokens in body %s", token, body)
		last = pos
REDACTED
REDACTED

func TestReplaceModelInBody_PreservesTopLevelFieldOrder(t *testing.T) {
	svc := &GatewayService{REDACTED
	body := []byte(`{"alpha":1,"model":"claude-3-5-sonnet-latest","messages":[],"omega":2REDACTED`)

	result := svc.replaceModelInBody(body, "claude-3-5-sonnet-20241022")
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"model"`, `"messages"`, `"omega"`)
	require.Contains(t, resultStr, `"model":"claude-3-5-sonnet-20241022"`)
REDACTED

func TestNormalizeClaudeOAuthRequestBody_PreservesTopLevelFieldOrder(t *testing.T) {
	body := []byte(`{"alpha":1,"model":"claude-3-5-sonnet-latest","temperature":0.2,"system":"You are OpenCode, the best coding agent on the planet.","messages":[],"tool_choice":{"type":"auto"REDACTED,"omega":2REDACTED`)

	result, modelID := normalizeClaudeOAuthRequestBody(body, "claude-3-5-sonnet-latest", claudeOAuthNormalizeOptions{
		injectMetadata: true,
		metadataUserID: "user-1",
REDACTED)
	resultStr := string(result)

	require.Equal(t, claude.NormalizeModelID("claude-3-5-sonnet-latest"), modelID)
	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"model"`, `"system"`, `"messages"`, `"omega"`, `"tools"`, `"metadata"`)
	require.NotContains(t, resultStr, `"temperature"`)
	require.NotContains(t, resultStr, `"tool_choice"`)
	require.Contains(t, resultStr, `"system":"`+claudeCodeSystemPrompt+`"`)
	require.Contains(t, resultStr, `"tools":[]`)
	require.Contains(t, resultStr, `"metadata":{"user_id":"user-1"REDACTED`)
REDACTED

func TestInjectClaudeCodePrompt_PreservesFieldOrder(t *testing.T) {
	body := []byte(`{"alpha":1,"system":[{"id":"block-1","type":"text","text":"Custom"REDACTED],"messages":[],"omega":2REDACTED`)

	result := injectClaudeCodePrompt(body, []any{
		map[string]any{"id": "block-1", "type": "text", "text": "Custom"REDACTED,
REDACTED)
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"system"`, `"messages"`, `"omega"`)
	require.Contains(t, resultStr, `{"id":"block-1","type":"text","text":"`+claudeCodeSystemPrompt+`\n\nCustom"REDACTED`)
REDACTED

func TestEnforceCacheControlLimit_PreservesTopLevelFieldOrder(t *testing.T) {
	body := []byte(`{"alpha":1,"system":[{"type":"text","text":"s1","cache_control":{"type":"ephemeral"REDACTEDREDACTED,{"type":"text","text":"s2","cache_control":{"type":"ephemeral"REDACTEDREDACTED],"messages":[{"role":"user","content":[{"type":"text","text":"m1","cache_control":{"type":"ephemeral"REDACTEDREDACTED,{"type":"text","text":"m2","cache_control":{"type":"ephemeral"REDACTEDREDACTED,{"type":"text","text":"m3","cache_control":{"type":"ephemeral"REDACTEDREDACTED]REDACTED],"omega":2REDACTED`)

	result := enforceCacheControlLimit(body)
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"system"`, `"messages"`, `"omega"`)
	require.Equal(t, 4, strings.Count(resultStr, `"cache_control"`))
REDACTED

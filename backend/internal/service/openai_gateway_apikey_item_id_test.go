//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_APIKeyPassthrough_StripsInvalidInputItemIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_test","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2REDACTEDREDACTED`,
		)),
REDACTEDREDACTED
	svc := newOpenAIImageGenerationControlTestService(upstream)
	c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
	account := newOpenAIImageGenerationControlTestAccount()
	account.Extra = map[string]any{"openai_passthrough": trueREDACTED

	body := []byte(`{
		"model":"gpt-5.6-sol",
		"stream":false,
		"input":[
			{"type":"message","id":"item_bad_message","role":"assistant","content":[{"type":"output_text","text":"hello"REDACTED]REDACTED,
			{"type":"function_call","id":"item_bad_call","call_id":"call_123","name":"exec_command","arguments":"{REDACTED"REDACTED,
			{"type":"message","id":"msg_valid","role":"user","content":[{"type":"input_text","text":"continue"REDACTED]REDACTED,
			{"type":"function_call","id":"fc_valid","call_id":"call_456","name":"apply_patch","arguments":"{REDACTED"REDACTED,
			{"type":"function_call_output","id":"item_output","call_id":"call_123","output":"done"REDACTED,
			{"type":"web_search_call","id":"item_unconstrained"REDACTED
		]
REDACTED`)

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)

	forwarded := upstream.lastBody
	require.False(t, gjson.GetBytes(forwarded, "input.0.id").Exists())
	require.Equal(t, "hello", gjson.GetBytes(forwarded, "input.0.content.0.text").String())
	require.False(t, gjson.GetBytes(forwarded, "input.1.id").Exists())
	require.Equal(t, "call_123", gjson.GetBytes(forwarded, "input.1.call_id").String())
	require.Equal(t, "exec_command", gjson.GetBytes(forwarded, "input.1.name").String())
	require.Equal(t, "{REDACTED", gjson.GetBytes(forwarded, "input.1.arguments").String())
	require.Equal(t, "msg_valid", gjson.GetBytes(forwarded, "input.2.id").String())
	require.Equal(t, "fc_valid", gjson.GetBytes(forwarded, "input.3.id").String())
	require.Equal(t, "item_output", gjson.GetBytes(forwarded, "input.4.id").String())
	require.Equal(t, "call_123", gjson.GetBytes(forwarded, "input.4.call_id").String())
	require.Equal(t, "item_unconstrained", gjson.GetBytes(forwarded, "input.5.id").String())
REDACTED

// TestOpenAIGatewayService_APIKeyPassthrough_StripsInvalidReasoningItemIDs
// verifies that reasoning items with a non-rs id (e.g. item_*) are stripped
// before forwarding. OpenAI upstream requires reasoning ids to begin with
// "rs" and rejects item_* with 400:
// "Expected an ID that begins with 'rs'." (#5410)
func TestOpenAIGatewayService_APIKeyPassthrough_StripsInvalidReasoningItemIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_test","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2REDACTEDREDACTED`,
		)),
REDACTEDREDACTED
	svc := newOpenAIImageGenerationControlTestService(upstream)
	c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
	account := newOpenAIImageGenerationControlTestAccount()
	account.Extra = map[string]any{"openai_passthrough": trueREDACTED

	body := []byte(`{
		"model":"gpt-5.6-sol",
		"stream":false,
		"input":[
			{"type":"reasoning","id":"item_bad_reasoning","summary":[]REDACTED,
			{"type":"reasoning","id":"rs_valid","summary":[]REDACTED,
			{"type":"message","id":"msg_valid","role":"user","content":[{"type":"input_text","text":"continue"REDACTED]REDACTED
		]
REDACTED`)

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)

	forwarded := upstream.lastBody
	require.False(t, gjson.GetBytes(forwarded, "input.0.id").Exists(),
		"item_* id should be stripped from reasoning")
	require.Equal(t, "rs_valid", gjson.GetBytes(forwarded, "input.1.id").String(),
		"valid rs* id must be preserved")
	require.Equal(t, "msg_valid", gjson.GetBytes(forwarded, "input.2.id").String())
REDACTED

func TestShouldStripOpenAIResponsesInputItemID_Reasoning(t *testing.T) {
	cases := []struct {
		name     string
		itemType string
		id       string
		want     bool
REDACTED{
		{"reasoning item_* id", "reasoning", "item_bad_reasoning", trueREDACTED,
		{"reasoning rs id", "reasoning", "rs_abc123", falseREDACTED,
		{"reasoning empty id", "reasoning", "", falseREDACTED,
		{"message msg id", "message", "msg_abc", falseREDACTED,
		{"message item id", "message", "item_x", trueREDACTED,
		{"function_call fc id", "function_call", "fc_abc", falseREDACTED,
		{"function_call item id", "function_call", "item_x", trueREDACTED,
		{"unconstrained type", "web_search_call", "ws_001", falseREDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, shouldStripOpenAIResponsesInputItemID(tc.itemType, tc.id))
	REDACTED)
REDACTED
REDACTED

func TestSanitizeOpenAIResponsesInputItemIDs_AllocationGrowthIsLinear(t *testing.T) {
	makeBody := func(itemCount int) []byte {
		items := make([]string, itemCount)
		for i := range items {
			items[i] = fmt.Sprintf(`{"type":"message","id":"item_%d","role":"user","content":[{"type":"input_text","text":"hello"REDACTED]REDACTED`, i)
	REDACTED
		return []byte(`{"model":"gpt-5.6-sol","input":[` + strings.Join(items, ",") + `]REDACTED`)
REDACTED
	allocatedBytes := func(body []byte) uint64 {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)
		runtime.ReadMemStats(&after)
	REDACTED
		require.True(t, changed)
		require.NotEmpty(t, sanitized)
		return after.TotalAlloc - before.TotalAlloc
REDACTED

	smallAllocated := allocatedBytes(makeBody(20))
	largeAllocated := allocatedBytes(makeBody(200))
	require.Less(t, largeAllocated, smallAllocated*30,
		"10x more input items must not cause quadratic whole-body allocation growth")
REDACTED

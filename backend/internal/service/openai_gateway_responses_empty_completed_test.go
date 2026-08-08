//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestOpenAIResponsesEmptyCompletedFailsOver verifies that a Responses stream
// ending with an empty response.completed (no output, no usage, no error) is
// turned into a failover error instead of a successful empty reply (issue
// #5009).
func TestOpenAIResponsesEmptyCompletedFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_empty\",\"object\":\"response\",\"status\":\"in_progress\"REDACTEDREDACTED\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_empty\",\"object\":\"response\",\"status\":\"completed\"REDACTEDREDACTED\n\n",
		)),
REDACTEDREDACTED
	svc := newOpenAIImageGenerationControlTestService(upstream)
	c, recorder := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
	account := newOpenAIImageGenerationControlTestAccount()
	account.Extra = map[string]any{"openai_passthrough": trueREDACTED

	body := []byte(`{
		"model":"gpt-5.6-sol",
		"stream":true,
		"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"REDACTED]REDACTED]
REDACTED`)

	_, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "empty completed must produce UpstreamFailoverError, got: %v", err)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Empty(t, recorder.Body.String(), "no empty success stream may reach the client")
REDACTED

// TestOpenAIResponsesEmptyCompletedWithOutputSucceeds ensures streams with real
// semantic output are untouched.
func TestOpenAIResponsesEmptyCompletedWithOutputSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_ok\",\"object\":\"response\",\"status\":\"in_progress\"REDACTEDREDACTED\n\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"REDACTED\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_ok\",\"object\":\"response\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15REDACTEDREDACTEDREDACTED\n\n",
		)),
REDACTEDREDACTED
	svc := newOpenAIImageGenerationControlTestService(upstream)
	c, recorder := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
	account := newOpenAIImageGenerationControlTestAccount()
	account.Extra = map[string]any{"openai_passthrough": trueREDACTED

	body := []byte(`{
		"model":"gpt-5.6-sol",
		"stream":true,
		"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"REDACTED]REDACTED]
REDACTED`)

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.Contains(t, recorder.Body.String(), "hello")
	require.NotNil(t, result.Usage)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
REDACTED

// TestOpenAIResponsesEmptyCompletedWithUsageSucceeds ensures a completed event
// carrying usage is not mistaken for a silent refusal even without output.
func TestOpenAIResponsesEmptyCompletedWithUsageSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_usage\",\"object\":\"response\",\"status\":\"in_progress\"REDACTEDREDACTED\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_usage\",\"object\":\"response\",\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":0,\"total_tokens\":3REDACTEDREDACTEDREDACTED\n\n",
		)),
REDACTEDREDACTED
	svc := newOpenAIImageGenerationControlTestService(upstream)
	c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
	account := newOpenAIImageGenerationControlTestAccount()
	account.Extra = map[string]any{"openai_passthrough": trueREDACTED

	body := []byte(`{
		"model":"gpt-5.6-sol",
		"stream":true,
		"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"REDACTED]REDACTED]
REDACTED`)

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.Usage)
	require.Equal(t, 3, result.Usage.InputTokens)
REDACTED

func TestOpenAIResponsesCompletedEventIsEmpty(t *testing.T) {
	cases := []struct {
		name  string
		data  string
		usage *OpenAIUsage
		want  bool
REDACTED{
		{
			name: "bare completed",
			data: `{"type":"response.completed"REDACTED`,
			want: true,
	REDACTED,
		{
			name: "completed with empty output array",
			data: `{"type":"response.completed","response":{"id":"r1","status":"completed","output":[]REDACTEDREDACTED`,
			want: true,
	REDACTED,
		{
			name: "completed with usage",
			data: `{"type":"response.completed","response":{"id":"r1","status":"completed","usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTEDREDACTED`,
			want: false,
	REDACTED,
		{
			name: "completed with error",
			data: `{"type":"response.completed","response":{"id":"r1","status":"completed","error":{"code":"x"REDACTEDREDACTEDREDACTED`,
			want: false,
	REDACTED,
		{
			name: "completed with output item",
			data: `{"type":"response.completed","response":{"id":"r1","status":"completed","output":[{"type":"message","id":"msg_1"REDACTED]REDACTEDREDACTED`,
			want: false,
	REDACTED,
		{
			name: "accumulated usage",
			data: `{"type":"response.completed"REDACTED`,
			usage: &OpenAIUsage{
				InputTokens:  7,
				OutputTokens: 2,
		REDACTED,
			want: false,
	REDACTED,
		{
			name: "invalid json",
			data: `{"type":`,
			want: false,
	REDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, openAIResponsesCompletedEventIsEmpty([]byte(tc.data), tc.usage))
	REDACTED)
REDACTED
REDACTED

package service

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpstreamResponseModelObserverTerminalWinsAndRecordsConflict(t *testing.T) {
	observer := &upstreamResponseModelObserver{REDACTED

	observer.ObserveOpenAI([]byte(`{"type":"response.created","response":{"model":"gpt-5.5"REDACTEDREDACTED`), "response.created")
	observer.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"model":"gpt-5.4"REDACTEDREDACTED`), "response.completed")

	require.Equal(t, "gpt-5.4", observer.Model())
	require.True(t, observer.Conflict())
REDACTED

func TestUpstreamResponseModelObserverSupportsAnthropicAndGeminiShapes(t *testing.T) {
	t.Run("anthropic", func(t *testing.T) {
		observer := &upstreamResponseModelObserver{REDACTED
		observer.ObserveAnthropic([]byte(`{"type":"message_start","message":{"model":"claude-sonnet-4-20250514"REDACTEDREDACTED`))
		require.Equal(t, "claude-sonnet-4-20250514", observer.Model())
REDACTED)

	t.Run("gemini outer and nested", func(t *testing.T) {
		observer := &upstreamResponseModelObserver{REDACTED
		observer.ObserveGemini([]byte(`{"response":{"modelVersion":"gemini-2.5-pro"REDACTEDREDACTED`))
		observer.ObserveGemini([]byte(`{"modelVersion":"gemini-2.5-pro-latest"REDACTED`))
		require.Equal(t, "gemini-2.5-pro-latest", observer.Model())
		require.True(t, observer.Conflict())
REDACTED)
REDACTED

func TestUpstreamResponseModelObservationAttemptReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	first := beginUpstreamResponseModelObservation(c)
	first.Observe("failed-attempt-model", false)
	second := beginUpstreamResponseModelObservation(c)
	second.Observe("successful-attempt-model", false)

	require.Equal(t, "successful-attempt-model", observedUpstreamResponseModel(c))
	require.False(t, observedUpstreamResponseModelConflict(c))
REDACTED

func TestUpstreamModelMismatchThreeStateAndCaseInsensitiveComparison(t *testing.T) {
	require.Nil(t, upstreamModelMismatch("gpt-5.5", ""))

	matched := upstreamModelMismatch("gpt-5.5", "GPT-5.5")
	require.NotNil(t, matched)
	require.False(t, *matched)

	mismatched := upstreamModelMismatch("gpt-5.5", "gpt-5.4")
	require.NotNil(t, mismatched)
	require.True(t, *mismatched)
REDACTED

func TestObserveOpenAISSEBodyIgnoresMalformedPayload(t *testing.T) {
	observer := &upstreamResponseModelObserver{REDACTED
	observeOpenAISSEBody(observer, "data: not-json\n\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-5.4\"REDACTEDREDACTED\n\n")

	require.Equal(t, "gpt-5.4", observer.Model())
	require.False(t, observer.Conflict())
REDACTED

func TestUpstreamResponseModelObserverBoundsUntrustedModelName(t *testing.T) {
	observer := &upstreamResponseModelObserver{REDACTED
	observer.Observe("  "+strings.Repeat("模", upstreamResponseModelMaxLength+1)+"  ", false)

	require.Len(t, []rune(observer.Model()), upstreamResponseModelMaxLength)
REDACTED

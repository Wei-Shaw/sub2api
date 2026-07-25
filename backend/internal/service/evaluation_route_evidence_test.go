//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRouteTraceRecordsRedactedAttempts(t *testing.T) {
	hashKey := []byte(strings.Repeat("h", 32))
	trace := NewRouteTrace(EvaluationContext{RouteTraceID: "server-generated-trace"}, RouteTraceConfig{HashKey: hashKey})

	trace.RecordAttempt(RouteAttempt{Provider: "openai", AccountID: 12, ChannelID: 4, ResolvedModel: "gpt-5.4", Region: "cn-east", ErrorCode: "429"})
	trace.RecordAttempt(RouteAttempt{Provider: "openai", AccountID: 13, ChannelID: 4, ResolvedModel: "gpt-5.4", Region: "cn-east"})

	got := trace.Snapshot()
	require.Equal(t, 2, got.Attempts)
	require.Len(t, got.FallbackChain, 2)
	require.Equal(t, 1, got.FallbackChain[0].Ordinal)
	require.Equal(t, "openai", got.FallbackChain[0].Provider)
	require.Equal(t, RedactedResourceRef("account", 12, hashKey), got.FallbackChain[0].AccountPoolRef)
	require.Equal(t, RedactedResourceRef("channel", 4, hashKey), got.FallbackChain[0].ChannelRef)
	require.Equal(t, "429", got.FallbackChain[0].ErrorCode)

	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"account_id":12`)
	require.NotContains(t, string(encoded), `"channel_id":4`)
}

func TestRouteTraceUpdatesLatestAttemptError(t *testing.T) {
	trace := NewRouteTrace(EvaluationContext{}, RouteTraceConfig{HashKey: []byte(strings.Repeat("h", 32))})
	trace.RecordAttempt(RouteAttempt{Provider: "gemini", AccountID: 12, ResolvedModel: "gemini-2.5-pro", Region: "cn-east"})

	trace.RecordLatestAttemptError("503")
	got := trace.Snapshot()

	require.Equal(t, 1, got.Attempts)
	require.Len(t, got.FallbackChain, 1)
	require.Equal(t, "503", got.FallbackChain[0].ErrorCode)
}

func TestRouteTraceContextRoundTrip(t *testing.T) {
	trace := NewRouteTrace(EvaluationContext{}, RouteTraceConfig{HashKey: []byte(strings.Repeat("h", 32))})
	ctx := WithRouteTrace(context.Background(), trace)

	got, ok := RouteTraceFromContext(ctx)
	require.True(t, ok)
	require.Same(t, trace, got)
}

func TestRouteTraceRecordsAttemptsConcurrently(t *testing.T) {
	const attempts = 32
	trace := NewRouteTrace(EvaluationContext{}, RouteTraceConfig{HashKey: []byte(strings.Repeat("h", 32))})

	var group sync.WaitGroup
	group.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func(accountID int64) {
			defer group.Done()
			trace.RecordAttempt(RouteAttempt{Provider: "openai", AccountID: accountID, ResolvedModel: "gpt-5.4", Region: "cn-east"})
		}(int64(i + 1))
	}
	group.Wait()

	got := trace.Snapshot()
	require.Equal(t, attempts, got.Attempts)
	require.Len(t, got.FallbackChain, attempts)
}

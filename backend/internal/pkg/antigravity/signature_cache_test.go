package antigravity

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestThoughtSignatureCacheSetGet(t *testing.T) {
	cache := NewThoughtSignatureCache(time.Minute, 8)
	scope := ThoughtSignatureScope{APIKeyID: 1, Model: "gemini-3.8-flash-high"}
	cache.Set(scope, "call_1", "sig-real")
	require.Equal(t, "sig-real", cache.Get(scope, "call_1"))
	require.Empty(t, cache.Get(scope, "missing"))
	require.Empty(t, cache.Get(ThoughtSignatureScope{APIKeyID: 2, Model: scope.Model}, "call_1"))
	require.Empty(t, cache.Get(ThoughtSignatureScope{APIKeyID: 1, Model: "gemini-3.8-flash-low"}, "call_1"))

	cache.Set(scope, "call_sentinel", DummyThoughtSignature)
	require.Empty(t, cache.Get(scope, "call_sentinel"))
	cache.Set(scope, "call_oversize", strings.Repeat("a", maxCachedThoughtSignatureBytes+1))
	require.Empty(t, cache.Get(scope, "call_oversize"))
	cache.Set(ThoughtSignatureScope{}, "unscoped", "not-cached")
	cache.Set(scope, "", "sig-empty-id")
	require.Len(t, cache.entries, 1)
}

func TestThoughtSignatureCacheExpires(t *testing.T) {
	cache := NewThoughtSignatureCache(time.Minute, 8)
	scope := ThoughtSignatureScope{APIKeyID: 1, Model: "gemini-3.8-flash-high"}
	cache.Set(scope, "call_1", "sig-real")
	require.Equal(t, "sig-real", cache.Get(scope, "call_1"))
	cache.entries[thoughtSignatureKey{scope: scope, toolID: "call_1"}] = thoughtSignatureEntry{
		signature: "sig-real", storedAt: time.Now().Add(-2 * time.Minute),
	}
	require.Empty(t, cache.Get(scope, "call_1"))
}

func TestThoughtSignatureCacheEvictsBeyondCapacity(t *testing.T) {
	cache := NewThoughtSignatureCache(time.Hour, 2)
	scope := ThoughtSignatureScope{APIKeyID: 1, Model: "gemini-3.8-flash-high"}
	cache.Set(scope, "a", "sig-a")
	cache.Set(scope, "b", "sig-b")
	cache.Set(scope, "c", "sig-c")
	require.Equal(t, "sig-c", cache.Get(scope, "c"))
	require.Len(t, cache.entries, 2)
}

func TestStreamProcessorEmitsToolSignature(t *testing.T) {
	processor := NewStreamingProcessor("gemini-3.8-flash-high")
	events := processor.ProcessLine(`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"bash","args":{"cmd":"ls"},"id":"call-stream-1"},"thoughtSignature":"sig-stream"}]}}]}}`)
	require.Contains(t, string(events), `"signature":"sig-stream"`)
	require.True(t, processor.SawUpstreamResponse())
	require.False(t, processor.SawThinking())
}

func TestStreamProcessorSignatureOnlyFrameCountsAsThinking(t *testing.T) {
	processor := NewStreamingProcessor("gemini-3.8-flash-high")
	processor.ProcessLine(`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"thoughtSignature":"sig-only"}]},"finishReason":"STOP"}]}}`)
	require.True(t, processor.SawUpstreamResponse())
	require.True(t, processor.SawThinking())
	require.False(t, processor.HasContent())
	require.Equal(t, "STOP", processor.LastFinishReason())
}

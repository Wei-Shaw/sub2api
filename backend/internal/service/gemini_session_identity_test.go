//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeminiUpstreamSessionIdentity(t *testing.T) {
	first := []byte(`{"contents":[{"role":"user","parts":[{"text":"Original prompt"}]}],"generationConfig":{"temperature":0.2}}`)
	compressed := []byte(`{"contents":[{"role":"user","parts":[{"text":"Compressed summary"}]}],"generationConfig":{"temperature":0.2}}`)
	for _, body := range [][]byte{first, compressed} {
		out := EnsureGeminiRequestSessionIDBytes(body, "group:account:key:session")
		require.Equal(t, gjson.GetBytes(EnsureGeminiRequestSessionIDBytes(first, "group:account:key:session"), "sessionId").String(), gjson.GetBytes(out, "sessionId").String())
		require.Equal(t, gjson.GetBytes(body, "contents").Raw, gjson.GetBytes(out, "contents").Raw)
		require.Equal(t, gjson.GetBytes(body, "generationConfig").Raw, gjson.GetBytes(out, "generationConfig").Raw)
	}
	one := EnsureGeminiRequestSessionIDBytes(first, "group:account:key1:session")
	two := EnsureGeminiRequestSessionIDBytes(first, "group:account:key2:session")
	require.NotEqual(t, gjson.GetBytes(one, "sessionId").String(), gjson.GetBytes(two, "sessionId").String())
	// Preserve both complete partition and conversation identifiers.
	require.NotEqual(t, GenerateGeminiDigestSessionKey("abcdefgh-key1", "same"), GenerateGeminiDigestSessionKey("abcdefgh-key2", "same"))
	require.NotEqual(t, GenerateGeminiDigestSessionKey("key", "abcdefgh-one"), GenerateGeminiDigestSessionKey("key", "abcdefgh-two"))
}

func TestDigestSessionStore_BoundsRetainedKeyBytes(t *testing.T) {
	store := NewDigestSessionStore()
	t.Cleanup(store.Close)
	chain := strings.Repeat("a", maxDigestSessionKeyBytes)
	store.Save(1, "prefix", chain, "uuid", 1, "")
	require.Zero(t, store.cache.ItemCount())
	_, _, _, found := store.Find(1, "prefix", chain)
	require.False(t, found)
}

func TestGeminiExplicitBodySessionCannotBypassUpstreamIsolation(t *testing.T) {
	plain := []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
	for _, body := range [][]byte{
		[]byte(`{"sessionId":"shared-client-id","contents":[{"role":"user","parts":[{"text":"hello"}]}]}`),
		[]byte(`{"session_id":"shared-client-id","contents":[{"role":"user","parts":[{"text":"hello"}]}]}`),
		[]byte(`{"sessionId":"first","contents":[{"role":"user","parts":[{"text":"hello"}]}],"sessionId":"last"}`),
		[]byte(`{"session_id":"first","session_id":"last","contents":[{"role":"user","parts":[{"text":"hello"}]}]}`),
		[]byte(`{"sessionId":"first","session_id":"second","sessionId":"last","contents":[{"role":"user","parts":[{"text":"hello"}]}]}`),
		[]byte(`{"session\u0049d":"escaped","contents":[{"role":"user","parts":[{"text":"hello"}]}]}`),
		[]byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"sessionId":"first","session\u0049d":"last"}`),
	} {
		first := EnsureGeminiRequestSessionIDBytes(body, "group:account:key1:session")
		second := EnsureGeminiRequestSessionIDBytes(body, "group:account:key2:session")
		id := gjson.GetBytes(first, "sessionId").String()
		require.NotEqual(t, "shared-client-id", id)
		require.False(t, gjson.GetBytes(first, "session_id").Exists())
		require.NotEqual(t, id, gjson.GetBytes(second, "sessionId").String())
		require.Equal(t, id, gjson.GetBytes(EnsureGeminiRequestSessionIDBytes(plain, "group:account:key1:session"), "sessionId").String(), "header and body forms resolve to the same authenticated session")
		require.Equal(t, gjson.GetBytes(body, "contents").Raw, gjson.GetBytes(first, "contents").Raw)
		require.Equal(t, first, EnsureGeminiRequestSessionIDBytes(first, "group:account:key1:session"), "repeated normalization is stable")
		fields := 0
		gjson.ParseBytes(first).ForEach(func(key, _ gjson.Result) bool {
			if key.Str == "sessionId" {
				fields++
			}
			return true
		})
		require.Equal(t, 1, fields, "only one canonical session identity may survive")
		wrapped, err := (&AntigravityGatewayService{}).wrapV1InternalRequest("test-project", "test-model", first)
		require.NoError(t, err)
		require.Equal(t, id, gjson.GetBytes(wrapped, "request.sessionId").String(), "encoding/json must forward the isolated identity too")
	}
}

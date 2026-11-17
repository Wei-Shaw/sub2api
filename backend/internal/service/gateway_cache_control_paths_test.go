package service

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type cacheControlPathsSnapshot struct {
	invalidThinking []cacheControlPath
	messages        []string
	tools           []string
	system          []string
}

func snapshotCacheControlPaths(body []byte) cacheControlPathsSnapshot {
	invalid, messages, tools, system := collectCacheControlPaths(body)
	return cacheControlPathsSnapshot{invalid, messages, tools, system}
}

func TestCollectCacheControlPaths_BorrowedViewPreservesSelectionAndOwnership(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want cacheControlPathsSnapshot
	}{
		{name: "empty"},
		{
			name: "array order and invalid thinking",
			body: `{"system":[{"type":"text","cache_control":{}},{"type":"thinking","cache_control":{}}],"messages":[{"content":[{"type":"text"},{"type":"thinking","cache_control":{}},{"type":"text","cache_control":{}}]},{"content":"plain text"}],"tools":[{"cache_control":{}},{},{"cache_control":{}}]}`,
			want: cacheControlPathsSnapshot{
				invalidThinking: []cacheControlPath{
					{path: "system.1.cache_control", log: "[Warning] Removed illegal cache_control from thinking block in system"},
					{path: "messages.0.content.1.cache_control", log: "[Warning] Removed illegal cache_control from thinking block in messages[0].content[1]"},
				},
				messages: []string{"messages.0.content.2.cache_control"},
				tools:    []string{"tools.0.cache_control", "tools.2.cache_control"},
				system:   []string{"system.0.cache_control"},
			},
		},
		{
			name: "first duplicate parent and content keys",
			body: `{"system":[{"type":"text","cache_control":null}],"system":[{"type":"thinking","cache_control":{}}],"messages":[{"content":[{"type":"text","cache_control":{}}],"content":[{"type":"thinking","cache_control":{}}]}],"messages":[],"tools":[],"tools":[{"cache_control":{}}]}`,
			want: cacheControlPathsSnapshot{
				messages: []string{"messages.0.content.0.cache_control"},
				system:   []string{"system.0.cache_control"},
			},
		},
		{
			name: "escaped field names",
			body: ` {"\u0073ystem":[{"type":"text","cache_control":{}}],"\u006dessages":[{"content":[{"type":"text","cache_control":{}}]}],"\u0074ools":[{"cache_control":{}}]} `,
			want: cacheControlPathsSnapshot{
				messages: []string{"messages.0.content.0.cache_control"},
				tools:    []string{"tools.0.cache_control"},
				system:   []string{"system.0.cache_control"},
			},
		},
		{
			name: "garbage prefix retains GetBytes lookup behavior",
			body: `junk{"system":[{"type":"text","cache_control":{}}],"messages":[{"content":[{"type":"text","cache_control":{}}]}],"tools":[{"cache_control":{}}]}`,
			want: cacheControlPathsSnapshot{
				messages: []string{"messages.0.content.0.cache_control"},
				tools:    []string{"tools.0.cache_control"},
				system:   []string{"system.0.cache_control"},
			},
		},
		{
			name: "nonarray fields and content stay ignored",
			body: `{"system":"plain text","messages":[{"content":"plain text"},{"content":null}],"tools":{"cache_control":{}}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			got := snapshotCacheControlPaths(body)
			require.Equal(t, tt.body, string(body), "collection must not mutate borrowed input")
			// The caller can reuse the source after collection. Returned metadata must
			// not retain strings borrowed from the body.
			for i := range body {
				body[i] = 'x'
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCollectCacheControlPaths_ConcurrentImmutableInput(t *testing.T) {
	body := cacheControlPathsBenchmarkBody(4096)
	original := bytes.Clone(body)
	want := cacheControlPathsSnapshot{
		messages: []string{"messages.0.content.0.cache_control"},
		tools:    []string{"tools.0.cache_control"},
		system:   []string{"system.0.cache_control"},
	}
	t.Cleanup(func() { require.Equal(t, original, body) })
	for worker := 0; worker < 8; worker++ {
		t.Run(fmt.Sprintf("worker_%d", worker), func(t *testing.T) {
			t.Parallel()
			for i := 0; i < 8; i++ {
				require.Equal(t, want, snapshotCacheControlPaths(body))
			}
		})
	}
}

func TestEnforceCacheControlLimit_BorrowedCollectionKeepsDuplicateWireFields(t *testing.T) {
	body := `{"opaque":9007199254740993,"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral","ttl":"5m"}}],"system":[{"type":"text","text":"untouched duplicate","cache_control":{"type":"ephemeral","ttl":"5m"}}],"messages":[{"role":"user","content":[{"type":"text","text":"m","cache_control":{"type":"ephemeral","ttl":"1h"}}]}],"unknown":{"decimal":1.2300e+04,"text":"\u4f60\u597d"}}`
	want := strings.Replace(body, `"ttl":"5m"`, `"ttl":"1h"`, 1)
	out := enforceCacheControlLimit([]byte(body))
	require.Equal(t, want, string(out), "both passes must keep first-key semantics and preserve unrelated bytes")
	require.Equal(t, out, enforceCacheControlLimit(out), "a valid TTL order must remain byte-for-byte unchanged")
}

func cacheControlPathsBenchmarkBody(payloadBytes int) []byte {
	return []byte(`{"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral","ttl":"5m"}}],"messages":[{"role":"user","content":[{"type":"text","text":"` + strings.Repeat("x", payloadBytes) + `","cache_control":{"type":"ephemeral","ttl":"5m"}}]}],"tools":[{"name":"probe","cache_control":{"type":"ephemeral","ttl":"5m"}}]}`)
}

var benchmarkCacheControlPathsSnapshotSink cacheControlPathsSnapshot
var benchmarkCacheControlPathsBodySink []byte

// Fixed breakpoint count, increasing opaque message content: cumulative
// allocation bytes should stay bounded instead of copying messages per scan.
func BenchmarkCacheControlPathsLargeContent(b *testing.B) {
	for _, size := range []struct {
		name  string
		bytes int
	}{
		{name: "4KiB", bytes: 4 << 10},
		{name: "1MiB", bytes: 1 << 20},
		{name: "8MiB", bytes: 8 << 20},
	} {
		body := cacheControlPathsBenchmarkBody(size.bytes)
		b.Run(size.name+"/Collect", func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkCacheControlPathsSnapshotSink = snapshotCacheControlPaths(body)
			}
		})
		b.Run(size.name+"/EnforceUnchanged", func(b *testing.B) {
			if got := enforceCacheControlLimit(body); !bytes.Equal(body, got) {
				b.Fatal("fixture should not require cache-control changes")
			}
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkCacheControlPathsBodySink = enforceCacheControlLimit(body)
			}
		})
	}
}

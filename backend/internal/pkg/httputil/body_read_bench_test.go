package httputil

import (
	"bytes"
	"context"
	"testing"
)

// BenchmarkReadRequestBodyKnownLength compares the two initial-capacity
// strategies for a request that carries a trustworthy Content-Length.
//
// legacy_1MiB_cap_hint is the previous policy: never reserve more than 1 MiB
// up front, so a 4 MiB body is read into four chunks and then copied once more
// into the assembled result. single_buffer is the current policy: reserve the
// announced size and return that one chunk without an assembly copy.
func BenchmarkReadRequestBodyKnownLength(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 4<<20)
	contentLength := int64(len(body))

	cases := []struct {
		name    string
		capHint int
	}{
		{name: "legacy_1MiB_cap_hint", capHint: requestBodyReadMaxInitCap},
		{name: "single_buffer", capHint: int(contentLength)},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(contentLength)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				buf, err := readRequestBodyChunksWithBudget(
					context.Background(),
					bytes.NewReader(body),
					tc.capHint,
					contentLength,
				)
				if err != nil {
					b.Fatalf("read body: %v", err)
				}
				if int64(len(buf.data)) != contentLength {
					b.Fatalf("read %d bytes, want %d", len(buf.data), contentLength)
				}
			}
		})
	}
}

package httputil

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// zstdImageRequestBody builds a JSON body of about size bytes that, like a
// request carrying base64 images, compresses only moderately.
func zstdImageRequestBody(size int) []byte {
	raw := make([]byte, size*3/4)
	rng := rand.New(rand.NewSource(int64(size)))
	for i := range raw {
		raw[i] = byte(rng.Intn(64))
	}
	image := base64.StdEncoding.EncodeToString(raw)
	return []byte(`{"model":"gpt-5.5","input":[{"type":"message","content":[{"type":"input_image","image_url":"data:image/png;base64,` + image + `"}]}]}`)
}

func BenchmarkReadZstdRequestBody(b *testing.B) {
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = enc.Close() }()
	for _, size := range []int{256 << 10, 3 << 20, 24 << 20} {
		body := zstdImageRequestBody(size)
		compressed := enc.EncodeAll(body, nil)
		b.Run(fmt.Sprintf("bytes_%d", len(body)), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := &http.Request{Body: io.NopCloser(bodyReaderOnly{bytes.NewReader(compressed)}), ContentLength: int64(len(compressed)), Header: http.Header{"Content-Encoding": {"zstd"}}}
				got, err := ReadRequestBodyWithPrealloc(req)
				if err != nil || len(got) != len(body) {
					b.Fatalf("zstd body was not decoded: len=%d err=%v", len(got), err)
				}
			}
		})
	}
}

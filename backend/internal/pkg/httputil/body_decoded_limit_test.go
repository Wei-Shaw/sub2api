package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func compressDecodedLimitFixture(t *testing.T, encoding string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	var writer io.WriteCloser
	switch encoding {
	case "gzip", "x-gzip":
		writer = gzip.NewWriter(&buf)
	case "deflate":
		writer = zlib.NewWriter(&buf)
	case "zstd":
		var err error
		writer, err = zstd.NewWriter(&buf)
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unsupported fixture encoding: %s", encoding)
	}
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecompressRequestBodyRejectsOversizeWithoutTruncation(t *testing.T) {
	const limit = 128
	for _, encoding := range []string{"gzip", "x-gzip", "deflate", "zstd"} {
		for _, tc := range []struct {
			name string
			size int
		}{
			{"empty", 0}, {"below", limit - 1}, {"exact", limit}, {"over", limit + 1},
		} {
			t.Run(encoding+"/"+tc.name, func(t *testing.T) {
				body := bytes.Repeat([]byte("x"), tc.size)
				raw := compressDecodedLimitFixture(t, encoding, body)
				got, err := decompressRequestBody(encoding, raw, limit)
				if tc.size > limit {
					var maxErr *http.MaxBytesError
					if !errors.As(err, &maxErr) || maxErr.Limit != limit || got != nil {
						t.Fatalf("expected size error with no partial body, len=%d err=%v", len(got), err)
					}
					return
				}
				if err != nil || !bytes.Equal(got, body) {
					t.Fatalf("valid body rejected or altered: len=%d err=%v", len(got), err)
				}
			})
		}
	}
}

func TestReadRequestBodyWithPreallocRejectsDecodedOversize(t *testing.T) {
	// The first 64 MiB is valid JSON plus whitespace; truncation would silently
	// discard the final byte and could allow the altered request to be billed.
	body := bytes.Repeat([]byte(" "), maxDecompressedBodySize+1)
	copy(body, []byte(samplePayload))
	body[len(body)-1] = '!'
	raw := compressDecodedLimitFixture(t, "gzip", body)
	req := newRequestWithBody(t, raw, "gzip")
	req.Header.Set("Content-Length", "encoded-length-marker")
	got, err := ReadRequestBodyWithPrealloc(req)
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) || maxErr.Limit != maxDecompressedBodySize || got != nil {
		t.Fatalf("expected wrapped size error and no body, len=%d err=%v", len(got), err)
	}
	if req.Header.Get("Content-Encoding") != "gzip" || req.ContentLength != int64(len(raw)) || req.Header.Get("Content-Length") != "encoded-length-marker" {
		t.Fatal("failed decoding must not rewrite request metadata")
	}
}

func TestDecompressRequestBodyRejectsCorruptCompressedTrailer(t *testing.T) {
	for _, encoding := range []string{"gzip", "deflate"} {
		t.Run(encoding, func(t *testing.T) {
			raw := compressDecodedLimitFixture(t, encoding, []byte(samplePayload))
			raw[len(raw)-1] ^= 0xff
			if _, err := decompressRequestBody(encoding, raw, int64(len(samplePayload))); err == nil {
				t.Fatal("exact-size decoded body must still validate the compressed trailer")
			}
		})
	}
}

package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func compressedBodyWriter(t *testing.T, encoding string, dst io.Writer) io.WriteCloser {
	t.Helper()
	switch encoding {
	case "gzip", "x-gzip":
		return gzip.NewWriter(dst)
	case "deflate":
		return zlib.NewWriter(dst)
	case "zstd":
		writer, err := zstd.NewWriter(dst, zstd.WithEncoderConcurrency(1))
		require.NoError(t, err)
		return writer
	default:
		t.Fatalf("unsupported test encoding %q", encoding)
		return nil
	}
}

func TestRequestDecompressionLimitBoundaries(t *testing.T) {
	for _, encoding := range []string{"gzip", "x-gzip", "zstd", "deflate"} {
		t.Run(encoding, func(t *testing.T) {
			t.Parallel()
			for _, size := range []int{127, 128, 129} {
				t.Run(fmt.Sprint(size), func(t *testing.T) {
					payload := append([]byte(`{"data":"`), bytes.Repeat([]byte("x"), size-11)...)
					payload = append(payload, []byte(`"}`)...)
					// JSON 总大小必须精确覆盖上限前一字节、等于上限和超限一字节。
					require.Len(t, payload, size)
					var compressed bytes.Buffer
					writer := compressedBodyWriter(t, encoding, &compressed)
					_, err := writer.Write(payload)
					require.NoError(t, err)
					require.NoError(t, writer.Close())
					req := newRequestWithBody(t, compressed.Bytes(), encoding)
					req = req.WithContext(WithMaxDecompressedBodySize(req.Context(), 128))
					body, err := ReadRequestBodyWithPrealloc(req)
					if size > 128 {
						var maxErr *http.MaxBytesError
						require.ErrorAs(t, err, &maxErr)
						require.Equal(t, int64(128), maxErr.Limit)
						require.Nil(t, body, "超限不能返回被截断的正文")
						require.Equal(t, encoding, req.Header.Get("Content-Encoding"))
						require.Equal(t, int64(compressed.Len()), req.ContentLength)
						return
					}
					require.NoError(t, err)
					require.Equal(t, payload, body)
					require.True(t, json.Valid(body))
					require.Empty(t, req.Header.Get("Content-Encoding"))
					require.Equal(t, int64(size), req.ContentLength)
				})
			}
		})
	}
}

func TestRequestDecompressionLimitIdentityUnchanged(t *testing.T) {
	for _, encoding := range []string{"", "identity"} {
		req := newRequestWithBody(t, []byte(samplePayload), encoding)
		req = req.WithContext(WithMaxDecompressedBodySize(req.Context(), 1))
		body, err := ReadRequestBodyWithPrealloc(req)
		require.NoError(t, err)
		require.Equal(t, samplePayload, string(body))
	}
}

func TestRequestDecompressionLimitFallback(t *testing.T) {
	req := newRequestWithBody(t, nil, "")
	require.Equal(t, int64(64<<20), requestMaxDecompressedBodySize(req))
	for _, limit := range []int64{0, -1} {
		withLimit := req.WithContext(WithMaxDecompressedBodySize(req.Context(), limit))
		require.Equal(t, int64(64<<20), requestMaxDecompressedBodySize(withLimit))
	}
}

func TestRequestDecompressionLimitRejectsDamagedTrailer(t *testing.T) {
	for _, encoding := range []string{"gzip", "zstd", "deflate"} {
		t.Run(encoding, func(t *testing.T) {
			var compressed bytes.Buffer
			writer := compressedBodyWriter(t, encoding, &compressed)
			_, err := writer.Write([]byte(samplePayload))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			for _, corrupt := range []bool{false, true} {
				data := bytes.Clone(compressed.Bytes())
				if corrupt {
					data[len(data)-1] ^= 0xff
				} else {
					data = data[:len(data)-1]
				}
				req := newRequestWithBody(t, data, encoding)
				req = req.WithContext(WithMaxDecompressedBodySize(req.Context(), int64(len(samplePayload))))
				body, err := ReadRequestBodyWithPrealloc(req)
				require.Error(t, err)
				require.Nil(t, body)
			}
		})
	}
}

func TestRequestDecompressionLimit130MiB(t *testing.T) {
	if testing.Short() {
		t.Skip("130 MiB 无损解压回归在非 short 测试中运行")
	}
	const size = 130 << 20
	for _, encoding := range []string{"gzip", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			var compressed bytes.Buffer
			encoder := compressedBodyWriter(t, encoding, &compressed)
			expectedHash := sha256.New()
			writer := io.MultiWriter(encoder, expectedHash)
			_, err := writer.Write([]byte(`{"data":"`))
			require.NoError(t, err)
			chunk := bytes.Repeat([]byte("x"), 1<<20)
			for remaining := size - 11; remaining > 0; {
				n := min(remaining, len(chunk))
				_, err = writer.Write(chunk[:n])
				require.NoError(t, err)
				remaining -= n
			}
			_, err = writer.Write([]byte(`"}`))
			require.NoError(t, err)
			require.NoError(t, encoder.Close())

			// 默认配置明确拒绝，不再返回 64 MiB 的半截 JSON。
			body, err := ReadRequestBodyWithPrealloc(newRequestWithBody(t, compressed.Bytes(), encoding))
			var maxErr *http.MaxBytesError
			require.True(t, errors.As(err, &maxErr))
			require.Equal(t, int64(64<<20), maxErr.Limit)
			require.Nil(t, body)

			// 提高到 160 MiB 后，130 MiB 完整 JSON 的长度与 SHA-256 均保持一致。
			req := newRequestWithBody(t, compressed.Bytes(), encoding)
			req = req.WithContext(WithMaxDecompressedBodySize(req.Context(), 160<<20))
			body, err = ReadRequestBodyWithPrealloc(req)
			require.NoError(t, err)
			require.Len(t, body, size)
			actualHash := sha256.Sum256(body)
			require.Equal(t, expectedHash.Sum(nil), actualHash[:])
			require.True(t, json.Valid(body))
		})
	}
}

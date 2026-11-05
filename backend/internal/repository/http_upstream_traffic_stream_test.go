package repository

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

// Gateway SSE readers stop at a protocol terminal event rather than waiting for
// HTTP EOF. The response bytes already consumed must survive closing the body.
func TestUpstreamTrafficSSETerminalBeforeEOF(t *testing.T) {
	payload := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n")
	for _, h2 := range []bool{false, true} {
		for _, compressed := range []bool{false, true} {
			name := map[bool]string{false: "http1", true: "http2"}[h2] + "/" + map[bool]string{false: "identity", true: "gzip"}[compressed]
			t.Run(name, func(t *testing.T) {
				encoded := payload
				if compressed {
					var out bytes.Buffer
					writer := gzip.NewWriter(&out)
					_, err := writer.Write(payload)
					require.NoError(t, err)
					require.NoError(t, writer.Flush())
					// A flushed live gzip stream, not a completed representation.
					encoded = bytes.Clone(out.Bytes())
					require.NoError(t, writer.Close())
				}
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", "text/event-stream")
					if compressed {
						w.Header().Set("Content-Encoding", "gzip")
					}
					_, _ = w.Write(encoded)
					flusher, ok := w.(http.Flusher)
					if !ok {
						t.Error("response writer does not support flushing")
						return
					}
					flusher.Flush()
					// Do not send EOF; only client cancellation ends this request.
					<-r.Context().Done()
				}))
				server.EnableHTTP2 = h2
				if h2 {
					server.StartTLS()
				} else {
					server.Start()
				}
				defer server.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				collector := traffic.NewCollector()
				req, err := http.NewRequestWithContext(traffic.WithCollector(ctx, collector), http.MethodPost, server.URL, strings.NewReader("request"))
				require.NoError(t, err)
				resp, err := doUpstreamRequest(server.Client(), req)
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()
				require.Equal(t, map[bool]int{false: 1, true: 2}[h2], resp.ProtoMajor)
				scanner := bufio.NewScanner(resp.Body)
				terminal := false
				for scanner.Scan() {
					if scanner.Text() == "data: [DONE]" {
						terminal = true
						break
					}
				}
				require.NoError(t, scanner.Err())
				require.True(t, terminal)
				started := time.Now()
				require.NoError(t, resp.Body.Close())
				require.Less(t, time.Since(started), time.Second, "accounting must not drain a stream or wait for EOF")
				collector.Finalize()
				measured := collector.Snapshot().Upstream.Response
				require.NotNil(t, measured.ActualBytes, "consumed SSE response bytes disappeared because HTTP EOF was not read")
				require.EqualValues(t, len(encoded), *measured.ActualBytes)
				require.Equal(t, compressed, measured.Compressed)
				if compressed {
					// Decoder read-ahead prevents a trustworthy whole-body ratio
					// until the decoded representation completes.
					require.Nil(t, measured.OriginalBytes)
					require.Nil(t, measured.Ratio())
				} else {
					require.NotNil(t, measured.OriginalBytes)
					require.EqualValues(t, len(payload), *measured.OriginalBytes)
				}
			})
		}
	}
}

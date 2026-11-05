package repository

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

// Receiving headers is not permission to read a possibly stalled body. This
// also covers callers that reject a 503/redirect and immediately close it.
func TestUpstreamTrafficLazyHeadersOnlyClose(t *testing.T) {
	for _, observed := range []bool{false, true} {
		for _, encoding := range []string{"gzip", "br", "deflate", "zstd"} {
			for _, status := range []int{http.StatusServiceUnavailable, http.StatusFound} {
				t.Run(fmt.Sprintf("collector=%v/%s/%d", observed, encoding, status), func(t *testing.T) {
					headersSent := make(chan struct{})
					releaseBody := make(chan struct{})
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Encoding", encoding)
						w.Header().Set("Content-Length", "100")
						if status == http.StatusFound {
							w.Header().Set("Location", "/redirect")
						}
						w.WriteHeader(status)
						flusher, ok := w.(http.Flusher)
						if !ok {
							t.Error("response writer does not support flushing")
							return
						}
						flusher.Flush()
						close(headersSent)
						select {
						case <-releaseBody:
						case <-r.Context().Done():
						}
					}))
					defer server.Close()
					defer close(releaseBody)
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					collector := traffic.NewCollector()
					if observed {
						ctx = traffic.WithCollector(ctx, collector)
					}
					req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
					// Leave implicit gzip negotiation enabled in both variants.
					if encoding != "gzip" {
						req.Header.Set("Accept-Encoding", encoding)
					}
					client := *server.Client()
					client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
					type answer struct {
						response *http.Response
						err      error
					}
					result := make(chan answer, 1)
					go func() { response, err := doUpstreamRequest(&client, req); result <- answer{response, err} }()
					select {
					case <-headersSent:
					case <-ctx.Done():
						t.Fatal("server did not send headers")
					}
					var response *http.Response
					select {
					case got := <-result:
						require.NoError(t, got.err)
						response = got.response
					case <-time.After(time.Second):
						cancel()
						got := <-result
						if got.response != nil {
							_ = got.response.Body.Close()
						}
						t.Fatal("Do blocked on body after receiving headers")
					}
					require.Empty(t, response.Header.Get("Content-Encoding"))
					require.Empty(t, response.Header.Get("Content-Length"))
					require.EqualValues(t, -1, response.ContentLength)
					closed := make(chan error, 1)
					go func() { closed <- response.Body.Close() }()
					select {
					case err := <-closed:
						require.NoError(t, err)
					case <-time.After(time.Second):
						cancel()
						t.Fatal("Close initialized/drained an unread decoder")
					}
					if observed {
						measurement := collector.Snapshot().Upstream.Response
						require.Nil(t, measurement.ActualBytes)
						require.Nil(t, measurement.OriginalBytes)
					}
				})
			}
		}
	}
}

func TestUpstreamTrafficLazyReadCancellation(t *testing.T) {
	for _, observed := range []bool{false, true} {
		for _, encoding := range []string{"gzip", "br", "deflate", "zstd"} {
			for _, closeDuringRead := range []bool{false, true} {
				t.Run(fmt.Sprintf("collector=%v/%s/close=%v", observed, encoding, closeDuringRead), func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Encoding", encoding)
						w.WriteHeader(http.StatusOK)
						flusher, ok := w.(http.Flusher)
						if !ok {
							t.Error("response writer does not support flushing")
							return
						}
						flusher.Flush()
						<-r.Context().Done()
					}))
					defer server.Close()
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					collector := traffic.NewCollector()
					if observed {
						ctx = traffic.WithCollector(ctx, collector)
					}
					req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
					req.Header.Set("Accept-Encoding", encoding)
					resp, err := doUpstreamRequest(server.Client(), req)
					require.NoError(t, err)
					started := make(chan struct{})
					read := make(chan error, 1)
					go func() { close(started); _, err := io.ReadAll(resp.Body); read <- err }()
					<-started
					closed := make(chan error, 1)
					if closeDuringRead {
						go func() { closed <- resp.Body.Close() }()
					} else {
						cancel()
					}
					select {
					case err := <-read:
						require.Error(t, err)
						if !closeDuringRead {
							require.ErrorIs(t, err, context.Canceled)
						}
					case <-time.After(time.Second):
						cancel()
						t.Fatal("Read did not unblock on cancel/Close")
					}
					if closeDuringRead {
						select {
						case err := <-closed:
							require.NoError(t, err)
						case <-time.After(time.Second):
							t.Fatal("concurrent Close blocked")
						}
					} else {
						require.NoError(t, resp.Body.Close())
					}
					if observed {
						measurement := collector.Snapshot().Upstream.Response
						require.Nil(t, measurement.OriginalBytes)
						require.Nil(t, measurement.ActualBytes)
					}
				})
			}
		}
	}
}

func TestUpstreamTrafficLazyMetadataBeforeReadAndCounts(t *testing.T) {
	for _, observed := range []bool{false, true} {
		for _, encoding := range []string{"gzip", "br", "deflate", "zstd"} {
			t.Run(fmt.Sprintf("collector=%v/%s", observed, encoding), func(t *testing.T) {
				plain := []byte(strings.Repeat("payload before transformation\n", 40))
				var encoded []byte
				switch encoding {
				case "gzip":
					encoded = compressGzip(t, plain)
				case "br":
					encoded = compressBrotli(t, plain)
				case "zstd":
					encoded = compressZstd(t, plain)
				case "deflate":
					var buffer bytes.Buffer
					writer := zlib.NewWriter(&buffer)
					_, err := writer.Write(plain)
					require.NoError(t, err)
					require.NoError(t, writer.Close())
					encoded = buffer.Bytes()
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Encoding", encoding)
					w.Header().Set("Content-Length", strconv.Itoa(len(encoded)))
					_, _ = w.Write(encoded)
				}))
				defer server.Close()
				collector := traffic.NewCollector()
				ctx := context.Background()
				if observed {
					ctx = traffic.WithCollector(ctx, collector)
				}
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
				req.Header.Set("Accept-Encoding", encoding)
				resp, err := doUpstreamRequest(server.Client(), req)
				require.NoError(t, err)
				// Handlers often copy these headers BEFORE io.Copy/first SSE Read.
				require.Empty(t, resp.Header.Get("Content-Encoding"))
				require.Empty(t, resp.Header.Get("Content-Length"))
				require.EqualValues(t, -1, resp.ContentLength)
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, plain, body)
				require.NoError(t, resp.Body.Close())
				if observed {
					measurement := collector.Snapshot().Upstream.Response
					require.EqualValues(t, len(encoded), *measurement.ActualBytes)
					require.EqualValues(t, len(plain), *measurement.OriginalBytes)
				}
			})
		}
	}
}

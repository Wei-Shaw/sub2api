package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

func TestUpstreamTrafficMultipleEncodingFieldsStayOpaque(t *testing.T) {
	for _, fields := range [][]string{{"gzip", "br"}, {"identity", "gzip"}} {
		encoded := compressGzip(t, []byte("payload"))
		if fields[0] == "gzip" {
			encoded = compressBrotli(t, encoded)
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			for _, encoding := range fields {
				w.Header().Add("Content-Encoding", encoding)
			}
			_, _ = w.Write(encoded)
		}))
		collector := traffic.NewCollector()
		ctx := traffic.WithCollector(context.Background(), collector)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(encoded))
		for _, encoding := range fields {
			req.Header.Add("Content-Encoding", encoding)
		}
		resp, err := doUpstreamRequest(server.Client(), req)
		require.NoError(t, err)
		require.Equal(t, fields, resp.Header.Values("Content-Encoding"))
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, encoded, body)
		require.NoError(t, resp.Body.Close())
		usage := collector.Snapshot()
		for _, measurement := range []*traffic.Measurement{usage.Upstream.Request, usage.Upstream.Response} {
			require.True(t, measurement.Compressed)
			require.Nil(t, measurement.OriginalBytes)
			require.EqualValues(t, len(encoded), *measurement.ActualBytes)
		}
		server.Close()
	}
}

func TestUpstreamTrafficExplicitAcceptEncodingIsNotReplaced(t *testing.T) {
	for _, test := range []struct {
		fields []string
		want   string
	}{
		{nil, "gzip"},
		{[]string{"", "br"}, ",br"},
		{[]string{""}, "identity"},
		{[]string{"deflate", "br;q=0.8"}, "deflate,br;q=0.8"},
	} {
		t.Run(fmt.Sprint(test.fields), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, test.want, strings.Join(r.Header.Values("Accept-Encoding"), ","))
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			ctx := traffic.WithCollector(context.Background(), traffic.NewCollector())
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			for _, value := range test.fields {
				req.Header.Add("Accept-Encoding", value)
			}
			resp, err := doUpstreamRequest(server.Client(), req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, test.fields, req.Header.Values("Accept-Encoding")) // caller headers never mutated
		})
	}
}

type trafficFailureBody struct{ cause error }

func (b *trafficFailureBody) Read([]byte) (int, error) { return 0, b.cause }
func (b *trafficFailureBody) Close() error             { return nil }

func TestUpstreamTrafficLazyPreservesReadFailureCause(t *testing.T) {
	for _, observed := range []bool{false, true} {
		for _, encoding := range []string{"gzip", "br", "deflate", "zstd"} {
			for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("original network read failure")} {
				t.Run(fmt.Sprintf("collector=%v/%s/%v", observed, encoding, cause), func(t *testing.T) {
					collector := traffic.NewCollector()
					ctx := context.Background()
					if observed {
						ctx = traffic.WithCollector(ctx, collector)
					}
					client := &http.Client{Transport: trafficTestRoundTripper(func(*http.Request) (*http.Response, error) {
						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Encoding": []string{encoding}}, ContentLength: -1, Body: &trafficFailureBody{cause: cause}}, nil
					})}
					req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unused.test", nil)
					resp, err := doUpstreamRequest(client, req)
					require.NoError(t, err)
					_, err = io.ReadAll(resp.Body)
					require.ErrorIs(t, err, cause)
					require.NoError(t, resp.Body.Close())
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

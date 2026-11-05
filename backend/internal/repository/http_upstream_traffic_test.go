package repository

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

func TestUpstreamTrafficContentCodingsAndImplicitGzip(t *testing.T) {
	payload := []byte(strings.Repeat("upstream original before protocol conversion\n", 60))
	zlibEncode := func(t *testing.T, p []byte) []byte {
		var out bytes.Buffer
		writer := zlib.NewWriter(&out)
		_, err := writer.Write(p)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		return out.Bytes()
	}
	for _, test := range []struct {
		name, encoding string
		encode         func(*testing.T, []byte) []byte
	}{
		{"identity", "", func(_ *testing.T, p []byte) []byte { return p }},
		{"implicit-gzip", "gzip", compressGzip},
		{"br", "br", compressBrotli},
		{"deflate-zlib", "deflate", zlibEncode},
		{"deflate-raw", "deflate", compressDeflate},
		{"zstd", "zstd", compressZstd},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded := test.encode(t, payload)
			upload := "outgoing request after model/protocol conversion"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.name == "implicit-gzip" {
					require.Equal(t, "gzip", r.Header.Get("Accept-Encoding"))
				}
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Equal(t, upload, string(body))
				w.Header().Set("Content-Encoding", test.encoding)
				_, _ = w.Write(encoded)
			}))
			defer server.Close()
			collector := traffic.NewCollector()
			ctx := traffic.WithCollector(context.Background(), collector)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(upload))
			require.NoError(t, err)
			resp, err := doUpstreamRequest(server.Client(), req)
			require.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, payload, body)
			require.NoError(t, resp.Body.Close())
			collector.Finalize()
			u := collector.Snapshot()
			require.EqualValues(t, len(upload), *u.Upstream.Request.ActualBytes)
			require.EqualValues(t, len(upload), *u.Upstream.Request.OriginalBytes)
			require.EqualValues(t, len(encoded), *u.Upstream.Response.ActualBytes)
			require.EqualValues(t, len(payload), *u.Upstream.Response.OriginalBytes)
			require.Equal(t, test.encoding != "", u.Upstream.Response.Compressed)
		})
	}
}

func TestUpstreamTrafficRetriesAccumulateAllAttempts(t *testing.T) {
	collector := traffic.NewCollector()
	ctx := traffic.WithCollector(context.Background(), collector)
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("retry body"))
			return
		}
		_, _ = w.Write([]byte("success body"))
	}))
	defer server.Close()
	for range 2 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader("request"))
		resp, err := doUpstreamRequest(server.Client(), req)
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	u := collector.Snapshot()
	require.EqualValues(t, 2*len("request"), *u.Upstream.Request.ActualBytes)
	require.EqualValues(t, len("retry body")+len("success body"), *u.Upstream.Response.ActualBytes)
}

type trafficTestRoundTripper func(*http.Request) (*http.Response, error)

func (f trafficTestRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUpstreamTrafficGrokInternalFallback(t *testing.T) {
	collector := traffic.NewCollector()
	ctx := traffic.WithCollector(context.Background(), collector)
	transport := trafficTestRoundTripper(func(req *http.Request) (*http.Response, error) {
		_, err := io.Copy(io.Discard, req.Body)
		require.NoError(t, err)
		status, body := http.StatusOK, "final payload"
		if req.URL.Hostname() == grokCLIProxyHost {
			status, body = http.StatusForbidden, "Access denied"
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: req}, nil
	})
	client := httpClientWithGrokAccessDeniedFallback(&http.Client{Transport: transport})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+grokCLIProxyHost+"/v1/chat/completions", strings.NewReader("upload"))
	req.Header.Set("X-XAI-Token-Auth", "xai-grok-cli")
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := doUpstreamRequest(client, req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "final payload", string(body))
	require.NoError(t, resp.Body.Close())
	u := collector.Snapshot()
	require.EqualValues(t, 2*len("upload"), *u.Upstream.Request.ActualBytes)
	require.EqualValues(t, len("Access denied")+len("final payload"), *u.Upstream.Response.ActualBytes)
}

func TestUpstreamTrafficUnknownAndPartial(t *testing.T) {
	for _, test := range []struct {
		name, encoding string
		body           string
		uncompressed   bool
	}{
		{"unsupported", "unknown", "opaque payload", false},
		{"stacked", "gzip, br", "opaque stacked payload", false},
		{"malformed-gzip", "gzip", "not a gzip frame", false},
		{"already-decoded", "", "decoded by custom transport", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			collector := traffic.NewCollector()
			ctx := traffic.WithCollector(context.Background(), collector)
			client := &http.Client{Transport: trafficTestRoundTripper(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Encoding": []string{test.encoding}}, Body: io.NopCloser(strings.NewReader(test.body)), ContentLength: int64(len(test.body)), Uncompressed: test.uncompressed}, nil
			})}
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unused.test", nil)
			resp, err := doUpstreamRequest(client, req)
			require.NoError(t, err)
			if test.name == "unsupported" || test.name == "stacked" {
				require.Equal(t, test.encoding, resp.Header.Get("Content-Encoding"))
				require.EqualValues(t, len(test.body), resp.ContentLength)
			}
			body, err := io.ReadAll(resp.Body)
			if test.name == "malformed-gzip" {
				require.Error(t, err)
				require.Empty(t, body) // never raw encoded bytes under decoded headers
			} else {
				require.NoError(t, err)
				require.Equal(t, test.body, string(body))
			}
			require.NoError(t, resp.Body.Close())
			measurement := collector.Snapshot().Upstream.Response
			if test.uncompressed {
				require.Nil(t, measurement.ActualBytes)
				require.EqualValues(t, len(test.body), *measurement.OriginalBytes)
			} else {
				require.Nil(t, measurement.OriginalBytes)
				require.EqualValues(t, len(test.body), *measurement.ActualBytes)
			}
		})
	}
	t.Run("early close", func(t *testing.T) {
		collector := traffic.NewCollector()
		ctx := traffic.WithCollector(context.Background(), collector)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", 8192))) }))
		defer server.Close()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		resp, err := doUpstreamRequest(server.Client(), req)
		require.NoError(t, err)
		n, err := resp.Body.Read(make([]byte, 1))
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.NoError(t, resp.Body.Close())
		measurement := collector.Snapshot().Upstream.Response
		require.NotNil(t, measurement.ActualBytes)
		require.NotNil(t, measurement.OriginalBytes)
		require.EqualValues(t, n, *measurement.ActualBytes)
		require.EqualValues(t, n, *measurement.OriginalBytes)
	})
	t.Run("read before failed socket write", func(t *testing.T) {
		collector := traffic.NewCollector()
		ctx := traffic.WithCollector(context.Background(), collector)
		failure := errors.New("socket write failed")
		client := &http.Client{Transport: trafficTestRoundTripper(func(req *http.Request) (*http.Response, error) {
			_, _ = io.Copy(io.Discard, req.Body)
			httptrace.ContextClientTrace(req.Context()).WroteRequest(httptrace.WroteRequestInfo{Err: failure})
			return nil, failure
		})}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unused.test", strings.NewReader("not proven sent"))
		_, err := doUpstreamRequest(client, req)
		require.Error(t, err)
		measurement := collector.Snapshot().Upstream.Request
		require.Nil(t, measurement.ActualBytes)
		require.Nil(t, measurement.OriginalBytes)
	})
}

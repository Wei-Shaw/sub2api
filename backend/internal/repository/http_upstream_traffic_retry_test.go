package repository

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

func TestUpstreamTrafficGzipGrokFallbackAndDiscardedFailure(t *testing.T) {
	for _, fallbackStatus := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		collector := traffic.NewCollector()
		ctx := traffic.WithCollector(context.Background(), collector)
		denied := compressGzip(t, []byte("Access denied"))
		success := compressGzip(t, []byte("success"))
		transport := trafficTestRoundTripper(func(req *http.Request) (*http.Response, error) {
			_, _ = io.Copy(io.Discard, req.Body)
			body, encoding, status := denied, "gzip", http.StatusForbidden
			if req.URL.Hostname() != grokCLIProxyHost {
				body, status = success, fallbackStatus
				if status != http.StatusOK {
					body, encoding = []byte("not consumed fallback body"), ""
				}
			}
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Encoding": []string{encoding}}, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: req}, nil
		})
		client := httpClientWithGrokAccessDeniedFallback(&http.Client{Transport: transport})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+grokCLIProxyHost+"/v1/chat/completions", strings.NewReader("request"))
		req.Header.Set("X-XAI-Token-Auth", "xai-grok-cli")
		req.Header.Set("Authorization", "Bearer test")
		resp, err := doUpstreamRequest(client, req)
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		usage := collector.Snapshot()
		require.EqualValues(t, 2*len("request"), *usage.Upstream.Request.ActualBytes)
		if fallbackStatus == http.StatusOK {
			require.EqualValues(t, len(denied)+len(success), *usage.Upstream.Response.ActualBytes)
			require.EqualValues(t, len("Access denied")+len("success"), *usage.Upstream.Response.OriginalBytes)
		} else {
			// Don't silently drop the non-2xx attempt whose body was closed unread.
			require.Nil(t, usage.Upstream.Response.ActualBytes)
			require.Nil(t, usage.Upstream.Response.OriginalBytes)
		}
	}
}

func TestUpstreamTrafficTransportReplayAndResponseFailure(t *testing.T) {
	t.Run("transport GetBody replay counts request twice", func(t *testing.T) {
		collector := traffic.NewCollector()
		ctx := traffic.WithCollector(context.Background(), collector)
		client := &http.Client{Transport: trafficTestRoundTripper(func(req *http.Request) (*http.Response, error) {
			trace := httptrace.ContextClientTrace(req.Context())
			_, _ = io.Copy(io.Discard, req.Body)
			trace.WroteRequest(httptrace.WroteRequestInfo{})
			body, err := req.GetBody()
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, body)
			_ = body.Close()
			trace.WroteRequest(httptrace.WroteRequestInfo{})
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: http.NoBody}, nil
		})}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unused.test", strings.NewReader("replayed"))
		resp, err := doUpstreamRequest(client, req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		u := collector.Snapshot()
		require.EqualValues(t, 2*len("replayed"), *u.Upstream.Request.ActualBytes)
		require.Nil(t, u.Upstream.Response.ActualBytes) // first hidden retry never completed a response
	})
	t.Run("sent request retained when waiting for headers fails", func(t *testing.T) {
		collector := traffic.NewCollector()
		ctx := traffic.WithCollector(context.Background(), collector)
		client := &http.Client{Transport: trafficTestRoundTripper(func(req *http.Request) (*http.Response, error) {
			_, _ = io.Copy(io.Discard, req.Body)
			httptrace.ContextClientTrace(req.Context()).WroteRequest(httptrace.WroteRequestInfo{})
			return nil, errors.New("response headers timeout")
		})}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unused.test", strings.NewReader("fully sent"))
		_, err := doUpstreamRequest(client, req)
		require.Error(t, err)
		u := collector.Snapshot()
		require.EqualValues(t, len("fully sent"), *u.Upstream.Request.ActualBytes)
		require.Nil(t, u.Upstream.Response.ActualBytes)
	})
}

func TestUpstreamTrafficTruncatedGzipKeepsActualButUnknownOriginal(t *testing.T) {
	collector := traffic.NewCollector()
	ctx := traffic.WithCollector(context.Background(), collector)
	encoded := compressGzip(t, []byte(strings.Repeat("body", 100)))
	encoded = encoded[:len(encoded)-4] // missing checksum/trailer
	client := &http.Client{Transport: trafficTestRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Encoding": []string{"gzip"}}, Body: io.NopCloser(bytes.NewReader(encoded)), ContentLength: int64(len(encoded))}, nil
	})}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unused.test", nil)
	resp, err := doUpstreamRequest(client, req)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
	require.NoError(t, resp.Body.Close())
	measurement := collector.Snapshot().Upstream.Response
	require.EqualValues(t, len(encoded), *measurement.ActualBytes)
	require.Nil(t, measurement.OriginalBytes)
}

package repository

import (
	"bytes"
	"compress/zlib"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/reqcompress"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
)

// TestUpstreamRequestCompressionNegotiatesAndMeasuresTraffic exercises the
// whole repository-side path: trial order, identity fallback for rejected
// codings, and traffic accounting for compressed request entities.
func TestUpstreamRequestCompressionNegotiatesAndMeasuresTraffic(t *testing.T) {
	const accountID int64 = 990000001
	original := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"` +
		strings.Repeat("measure compressed request traffic ", 60) + `"}]}`)
	require.Greater(t, len(original), int(reqcompress.MinCompressibleBodyBytes))

	var mu sync.Mutex
	var attempts []string
	var compressedSizes []int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoding := strings.ToLower(r.Header.Get("Content-Encoding"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		mu.Lock()
		attempts = append(attempts, encoding)
		compressedSizes = append(compressedSizes, int64(len(body)))
		mu.Unlock()
		switch reqcompress.Algorithm(encoding) {
		case reqcompress.AlgorithmBrotli, reqcompress.AlgorithmZstd:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"unsupported content encoding"}}`))
			return
		case reqcompress.AlgorithmDeflate:
			reader, err := zlib.NewReader(bytes.NewReader(body))
			require.NoError(t, err)
			decoded, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.NoError(t, reader.Close())
			require.Equal(t, original, decoded)
		default:
			require.Equal(t, original, body)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	reqcompress.SyncAccount(accountID, true, time.Time{})
	t.Cleanup(func() { reqcompress.SyncAccount(accountID, false, time.Time{}) })

	collector := traffic.NewCollector()
	ctx := traffic.WithCollector(context.Background(), collector)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(original))
	require.NoError(t, err)

	resp, err := doUpstreamRequestWithCompression(server.Client(), req, accountID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, err = io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, []string{"br", "zstd", "deflate"}, attempts)
	require.Len(t, compressedSizes, 3)
	collector.Finalize()
	usage := collector.Snapshot()
	require.NotNil(t, usage)
	require.NotNil(t, usage.Upstream)
	require.NotNil(t, usage.Upstream.Request)
	// Rejected probes are measured apart from the request, so the usage record
	// only describes the deflate attempt that was actually delivered.
	require.True(t, usage.Upstream.Request.Compressed)
	require.Equal(t, int64(len(original)), *usage.Upstream.Request.OriginalBytes)
	require.Equal(t, compressedSizes[len(compressedSizes)-1], *usage.Upstream.Request.ActualBytes)
	require.NotNil(t, usage.Upstream.Request.Ratio())
	require.Greater(t, *usage.Upstream.Request.Ratio(), 0.0)
}

func TestUpstreamRequestCompressionDisabledForwardsIdentity(t *testing.T) {
	const accountID int64 = 990000002
	reqcompress.SyncAccount(accountID, false, time.Time{})

	var attempts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts = append(attempts, r.Header.Get("Content-Encoding"))
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"` +
		strings.Repeat("identity request ", 100) + `"}]}`)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, bytes.NewReader(body))
	require.NoError(t, err)

	resp, err := doUpstreamRequestWithCompression(server.Client(), req, accountID)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []string{""}, attempts)
}

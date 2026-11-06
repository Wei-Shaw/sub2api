package reqcompress

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
)

func TestMain(m *testing.M) {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	code := m.Run()
	slog.SetDefault(previous)
	os.Exit(code)
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &logs
}

func TestExecuteLogsObservabilityEvents(t *testing.T) {
	logs := captureLogs(t)
	body := testBody()
	registry := NewRegistry()
	registry.Sync(16, true, time.Time{})

	var sent []string
	resp, err := registry.Execute(16, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		encoding := req.Header.Get("Content-Encoding")
		sent = append(sent, encoding)
		if encoding == "br" {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"X-Request-Id": []string{"req-abc"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":"rejected request"}`)),
			}, nil
		}
		return okResponse("zstd"), nil
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []string{"br", "zstd"}, sent)

	out := logs.String()
	require.Contains(t, out, `msg="reqcompress attempt"`)
	require.Contains(t, out, "content_encoding=br")
	require.Contains(t, out, `level=WARN msg="reqcompress coding unverified"`)
	require.Contains(t, out, "algorithm=br")
	require.Contains(t, out, "status=403")
	require.Contains(t, out, "upstream_request_id=req-abc")
	require.Contains(t, out, `level=INFO msg="reqcompress coding settled"`)
	require.Contains(t, out, "algorithm=zstd")
}

func TestExecuteLogsSettledIdentityFallback(t *testing.T) {
	logs := captureLogs(t)
	body := testBody()
	registry := NewRegistry()
	registry.Sync(18, true, time.Time{})

	// Settle on brotli first.
	_, err := registry.Execute(18, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	logs.Reset()

	var sent []string
	resp, err := registry.Execute(18, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		encoding := req.Header.Get("Content-Encoding")
		sent = append(sent, encoding)
		if encoding == "br" {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"X-Request-Id": []string{"req-abc"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":"rejected request"}`)),
			}, nil
		}
		return okResponse("identity"), nil
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []string{"br", ""}, sent)

	out := logs.String()
	require.Contains(t, out, `level=WARN msg="reqcompress identity fallback"`)
	require.Contains(t, out, "algorithm=br")
	require.Contains(t, out, "status=403")
	require.Contains(t, out, "upstream_request_id=req-abc")
	require.Contains(t, out, `level=WARN msg="reqcompress pinned identity"`)
	require.Contains(t, out, "reason=fallback_succeeded")
}

func TestExecuteLogsDiscardedProbeBytes(t *testing.T) {
	logs := captureLogs(t)
	body := testBody()
	registry := NewRegistry()
	registry.Sync(21, true, time.Time{})

	_, err := registry.Execute(21, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		encoding := req.Header.Get("Content-Encoding")
		if encoding == "" {
			require.Nil(t, traffic.FromContext(req.Context()))
			return okResponse("identity"), nil
		}
		probe := traffic.FromContext(req.Context())
		require.NotNil(t, probe)
		counter := probe.Begin(traffic.UpstreamRequest, true)
		counter.SetOriginal(int64(len(body)))
		counter.AddActual(len(body) / 2)
		counter.CompleteActual()
		return rejectionResponse("unsupported content encoding"), nil
	})
	require.NoError(t, err)

	out := logs.String()
	require.Contains(t, out, `level=WARN msg="reqcompress coding rejected"`)
	require.Contains(t, out, fmt.Sprintf("original_bytes=%d", len(body)))
	require.Contains(t, out, fmt.Sprintf("sent_bytes=%d", len(body)/2))
	require.Contains(t, out, "ratio_percent=50")
}

func TestExecuteLogsSettledCodingAtInfo(t *testing.T) {
	logs := captureLogs(t)
	registry := NewRegistry()
	registry.Sync(17, true, time.Time{})

	_, err := registry.Execute(17, newTestRequest(t, testBody()), func(req *http.Request) (*http.Response, error) {
		return okResponse("ok"), nil
	})
	require.NoError(t, err)

	out := logs.String()
	require.Contains(t, out, `level=INFO msg="reqcompress coding settled"`)
	require.Contains(t, out, "algorithm=br")
	require.Contains(t, out, "original_bytes=")
	require.Contains(t, out, "compressed_bytes=")
	require.Contains(t, out, "ratio_percent=")
}

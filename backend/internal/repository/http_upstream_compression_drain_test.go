package repository

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/reqcompress"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

const compressionDrainAttemptBudget = 40 * time.Millisecond

type compressionDrainAttempt struct {
	encoding  string
	bodyBytes int64
}

// startCompressionDrainServer records each attempt's Content-Encoding and body
// size. Coded attempts listed in reject answer 415, which is authoritative for
// a content-coding rejection; the accepted attempt streams SSE until the client
// disconnects so a regression cannot silently wait for the default drain
// timeout.
func startCompressionDrainServer(t *testing.T, reject map[string]bool) (*httptest.Server, <-chan compressionDrainAttempt) {
	t.Helper()
	attempts := make(chan compressionDrainAttempt, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		encoding := req.Header.Get("Content-Encoding")
		n, _ := io.Copy(io.Discard, req.Body)
		attempts <- compressionDrainAttempt{encoding: encoding, bodyBytes: n}
		if reject[encoding] {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			_, _ = io.WriteString(w, `{"error":{"message":"unsupported content encoding"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ": ping\n\n")
		_ = http.NewResponseController(w).Flush()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-req.Context().Done():
				return
			case <-ticker.C:
				_, _ = io.WriteString(w, ": ping\n\n")
				_ = http.NewResponseController(w).Flush()
			}
		}
	}))
	t.Cleanup(server.Close)
	return server, attempts
}

func awaitCompressionDrainAttempt(t *testing.T, attempts <-chan compressionDrainAttempt) compressionDrainAttempt {
	t.Helper()
	select {
	case attempt := <-attempts:
		return attempt
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for an upstream compression attempt")
		return compressionDrainAttempt{}
	}
}

// Compression negotiation must not bypass the current branch's bounded drain
// context, whether a rejected coding leads to another HTTP attempt or to an
// identity fallback. Discarded probes are measured apart and must not be
// adopted into the request's traffic.
func TestUpstreamRequestCompressionPreservesDrainBudget(t *testing.T) {
	tests := []struct {
		name          string
		accountID     int64
		reject        map[string]bool
		wantEncodings []string
		compressed    bool
	}{
		{
			name:          "brotli_rejected_then_zstd_delivered",
			accountID:     990000011,
			reject:        map[string]bool{"br": true},
			wantEncodings: []string{"br", "zstd"},
			compressed:    true,
		},
		{
			name:          "all_codings_rejected_then_identity_delivered",
			accountID:     990000012,
			reject:        map[string]bool{"br": true, "zstd": true, "deflate": true},
			wantEncodings: []string{"br", "zstd", "deflate", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqcompress.SyncAccount(tt.accountID, true, time.Time{})
			t.Cleanup(func() { reqcompress.SyncAccount(tt.accountID, false, time.Time{}) })
			server, attempts := startCompressionDrainServer(t, tt.reject)

			clientCtx, cancelClient := context.WithCancel(context.Background())
			defer cancelClient()
			collector := traffic.NewCollector()
			ctx := traffic.WithCollector(pkghttputil.DetachUpstreamContext(clientCtx), collector)
			body := bytes.Repeat([]byte("compressed request "), 100)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(body))
			require.NoError(t, err)
			require.True(t, reqcompress.Eligible(req), "fixture must be eligible for compression")

			resp, err := doUpstreamRequestWithCompression(server.Client(), req, tt.accountID, compressionDrainAttemptBudget)
			require.NoError(t, err)
			require.NotNil(t, resp)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			recorded := make([]compressionDrainAttempt, 0, len(tt.wantEncodings))
			for range tt.wantEncodings {
				recorded = append(recorded, awaitCompressionDrainAttempt(t, attempts))
			}
			encodings := make([]string, 0, len(recorded))
			for _, attempt := range recorded {
				encodings = append(encodings, attempt.encoding)
			}
			require.Equal(t, tt.wantEncodings, encodings)

			delivered := recorded[len(recorded)-1]
			rejectedBytes := int64(0)
			for _, attempt := range recorded[:len(recorded)-1] {
				require.Positive(t, attempt.bodyBytes)
				rejectedBytes += attempt.bodyBytes
			}
			require.Positive(t, delivered.bodyBytes)

			// A failed regression must finish instead of waiting for the default
			// drain timeout; the guard closes the response only if it does not.
			guard := time.AfterFunc(time.Second, func() { _ = resp.Body.Close() })
			defer guard.Stop()
			cancelClient()
			started := time.Now()
			got, readErr := io.ReadAll(resp.Body)
			elapsed := time.Since(started)
			require.Error(t, readErr)
			require.Contains(t, string(got), ": ping")
			require.Less(t, elapsed, 500*time.Millisecond)

			collector.Finalize()
			usage := collector.Snapshot()
			require.NotNil(t, usage)
			require.NotNil(t, usage.Upstream)
			require.NotNil(t, usage.Upstream.Request)
			measurement := usage.Upstream.Request
			require.Equal(t, tt.compressed, measurement.Compressed)
			require.NotNil(t, measurement.OriginalBytes)
			require.Equal(t, int64(len(body)), *measurement.OriginalBytes)
			require.NotNil(t, measurement.ActualBytes)
			require.Equal(t, delivered.bodyBytes, *measurement.ActualBytes)
			require.NotEqual(t, delivered.bodyBytes+rejectedBytes, *measurement.ActualBytes,
				"discarded compression probes must not be adopted into the request collector")
		})
	}
}

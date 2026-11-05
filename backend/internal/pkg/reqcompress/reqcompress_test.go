package reqcompress

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
)

func testBody() []byte {
	payload := `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"` +
		strings.Repeat("compress me ", 200) + `"}]}`
	return []byte(payload)
}

func newTestRequest(t *testing.T, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://upstream.test/v1/messages", bytes.NewReader(body))
	require.NoError(t, err)
	return req
}

func okResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func rejectionResponse(message string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"` + message + `"}}`)),
	}
}

func decodeRequest(t *testing.T, req *http.Request) []byte {
	t.Helper()
	raw, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.NoError(t, req.Body.Close())

	var reader io.Reader
	switch Algorithm(strings.ToLower(req.Header.Get("Content-Encoding"))) {
	case AlgorithmBrotli:
		reader = brotli.NewReader(bytes.NewReader(raw))
	case AlgorithmZstd:
		decoder, err := zstd.NewReader(bytes.NewReader(raw))
		require.NoError(t, err)
		defer decoder.Close()
		reader = decoder
	case AlgorithmDeflate:
		deflate, err := zlib.NewReader(bytes.NewReader(raw))
		require.NoError(t, err)
		defer func() { require.NoError(t, deflate.Close()) }()
		reader = deflate
	default:
		t.Fatalf("unexpected Content-Encoding %q", req.Header.Get("Content-Encoding"))
	}
	decoded, err := io.ReadAll(reader)
	require.NoError(t, err)
	return decoded
}

func TestEligible(t *testing.T) {
	body := testBody()

	t.Run("post with replayable body", func(t *testing.T) {
		require.True(t, Eligible(newTestRequest(t, body)))
	})

	t.Run("get without body", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://upstream.test/v1/models", nil)
		require.NoError(t, err)
		require.False(t, Eligible(req))
	})

	t.Run("small body", func(t *testing.T) {
		require.False(t, Eligible(newTestRequest(t, []byte(`{"tiny":true}`))))
	})

	t.Run("already coded", func(t *testing.T) {
		req := newTestRequest(t, body)
		req.Header.Set("Content-Encoding", "gzip")
		require.False(t, Eligible(req))
	})

	t.Run("raw cased content encoding", func(t *testing.T) {
		req := newTestRequest(t, body)
		req.Header["content-encoding"] = []string{"gzip"}
		require.False(t, Eligible(req))
	})

	t.Run("one shot body", func(t *testing.T) {
		req := newTestRequest(t, body)
		req.GetBody = nil
		require.False(t, Eligible(req))
	})

	t.Run("signed request", func(t *testing.T) {
		req := newTestRequest(t, body)
		req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=test")
		require.False(t, Eligible(req))
	})

	t.Run("upgrade request", func(t *testing.T) {
		req := newTestRequest(t, body)
		req.Header.Set("Upgrade", "websocket")
		require.False(t, Eligible(req))
	})
}

func TestExecuteNegotiatesInPreferenceOrder(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(1, true, time.Time{})

	var sent []string
	send := func(req *http.Request) (*http.Response, error) {
		encoding := req.Header.Get("Content-Encoding")
		sent = append(sent, encoding)
		require.Equal(t, body, decodeRequest(t, req))
		require.Equal(t, req.ContentLength, int64(len(mustCompress(t, body, Algorithm(encoding)))))
		require.Equal(t, int64(len(body)), originalBodyBytes(t, req))
		switch Algorithm(encoding) {
		case AlgorithmBrotli, AlgorithmZstd:
			return rejectionResponse("unsupported content encoding"), nil
		default:
			return okResponse("ok"), nil
		}
	}

	resp, err := registry.Execute(1, newTestRequest(t, body), send)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []string{"br", "zstd", "deflate"}, sent)

	// The settled coding is reused without probing the rejected ones again.
	sent = nil
	resp, err = registry.Execute(1, newTestRequest(t, body), send)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []string{"deflate"}, sent)
}

func TestExecuteFallsBackToIdentityWhenEveryCodingIsRejected(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(2, true, time.Time{})

	var sent []string
	send := func(req *http.Request) (*http.Response, error) {
		encoding := req.Header.Get("Content-Encoding")
		sent = append(sent, encoding)
		if encoding == "" {
			require.Equal(t, string(body), readAll(t, req.Body))
			return okResponse("identity"), nil
		}
		require.NotEmpty(t, encoding)
		return rejectionResponse("unsupported content coding"), nil
	}

	resp, err := registry.Execute(2, newTestRequest(t, body), send)
	require.NoError(t, err)
	require.Equal(t, "identity", readAll(t, resp.Body))
	require.Equal(t, []string{"br", "zstd", "deflate", ""}, sent)

	// Identity is pinned for the process lifetime after exhausting the trial.
	sent = nil
	resp, err = registry.Execute(2, newTestRequest(t, body), send)
	require.NoError(t, err)
	require.Equal(t, "identity", readAll(t, resp.Body))
	require.Equal(t, []string{""}, sent)
}

func TestExecuteRenegotiatesAfterSettledCodingBreaks(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(3, true, time.Time{})

	// Settle on brotli first.
	sent := 0
	_, err := registry.Execute(3, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		sent++
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, sent)

	var seen []string
	resp, err := registry.Execute(3, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		encoding := req.Header.Get("Content-Encoding")
		seen = append(seen, encoding)
		if Algorithm(encoding) == AlgorithmBrotli {
			return rejectionResponse("failed to decompress request body"), nil
		}
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []string{"br", "zstd"}, seen)

	// zstd is the new settled coding; brotli is never attempted again.
	seen = nil
	_, err = registry.Execute(3, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		seen = append(seen, req.Header.Get("Content-Encoding"))
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"zstd"}, seen)
}

func TestExecuteProbesRemainingCodingsOnAmbiguousError(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(4, true, time.Time{})

	var sent []string
	resp, err := registry.Execute(4, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		encoding := req.Header.Get("Content-Encoding")
		sent = append(sent, encoding)
		require.NotEmpty(t, encoding)
		switch Algorithm(encoding) {
		case AlgorithmBrotli:
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Body:       io.NopCloser(strings.NewReader(`{"error":"rejected request"}`)),
			}, nil
		default:
			return okResponse(encoding), nil
		}
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "zstd", readAll(t, resp.Body))
	require.Equal(t, []string{"br", "zstd"}, sent)

	// The accepted coding settles for subsequent requests.
	sent = nil
	_, err = registry.Execute(4, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		sent = append(sent, req.Header.Get("Content-Encoding"))
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"zstd"}, sent)
}

func TestExecutePinsIdentityWhenEveryCodingFails(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(13, true, time.Time{})

	var sent []string
	send := func(req *http.Request) (*http.Response, error) {
		sent = append(sent, req.Header.Get("Content-Encoding"))
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"messages: invalid role"}}`)),
		}, nil
	}
	resp, err := registry.Execute(13, newTestRequest(t, body), send)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Equal(t, "messages: invalid role", errorMessage(t, resp.Body))
	require.Equal(t, []string{"br", "zstd", "deflate", ""}, sent)

	// The trial must not keep probing on every request after all codings failed;
	// identity is pinned and the error is returned directly.
	sent = nil
	resp, err = registry.Execute(13, newTestRequest(t, body), send)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Equal(t, []string{""}, sent)
}

func TestExecuteDoesNotFallbackOnRateLimit(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(14, true, time.Time{})

	var sent []string
	send := func(req *http.Request) (*http.Response, error) {
		sent = append(sent, req.Header.Get("Content-Encoding"))
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: http.NoBody}, nil
	}
	resp, err := registry.Execute(14, newTestRequest(t, body), send)
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.Equal(t, []string{"br"}, sent)

	// The trial stays open; the next request probes again.
	sent = nil
	_, err = registry.Execute(14, newTestRequest(t, body), send)
	require.NoError(t, err)
	require.Equal(t, []string{"br"}, sent)
}

func TestExecuteSettledCodingFallsBackToIdentity(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(15, true, time.Time{})

	// Settle on brotli first.
	_, err := registry.Execute(15, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		return okResponse("ok"), nil
	})
	require.NoError(t, err)

	var sent []string
	resp, err := registry.Execute(15, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		encoding := req.Header.Get("Content-Encoding")
		sent = append(sent, encoding)
		if encoding == "br" {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Body:       io.NopCloser(strings.NewReader(`{"error":"rejected request"}`)),
			}, nil
		}
		return okResponse("identity"), nil
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []string{"br", ""}, sent)

	// Identity succeeded where the coding failed, so compression is pinned off.
	sent = nil
	_, err = registry.Execute(15, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		sent = append(sent, req.Header.Get("Content-Encoding"))
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{""}, sent)
}

func TestExecuteTreatsEmptyUnsupportedMediaTypeAsRejection(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(12, true, time.Time{})

	var sent []string
	resp, err := registry.Execute(12, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		encoding := req.Header.Get("Content-Encoding")
		sent = append(sent, encoding)
		if Algorithm(encoding) == AlgorithmBrotli {
			return &http.Response{StatusCode: http.StatusUnsupportedMediaType, Body: http.NoBody}, nil
		}
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []string{"br", "zstd"}, sent)

	sent = nil
	_, err = registry.Execute(12, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		sent = append(sent, req.Header.Get("Content-Encoding"))
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"zstd"}, sent)
}

func TestExecuteWhileTrialIsInFlightUsesIdentity(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(5, true, time.Time{})

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	trialDone := make(chan struct{})

	go func() {
		defer close(trialDone)
		_, _ = registry.Execute(5, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
			once.Do(func() { close(entered) })
			<-release
			return okResponse("trial"), nil
		})
	}()
	<-entered

	var identityCalls int
	resp, err := registry.Execute(5, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		require.Empty(t, req.Header.Get("Content-Encoding"))
		identityCalls++
		return okResponse("identity"), nil
	})
	require.NoError(t, err)
	require.Equal(t, "identity", readAll(t, resp.Body))
	require.Equal(t, 1, identityCalls)

	close(release)
	<-trialDone
}

func TestRegistrySyncOrdersSnapshots(t *testing.T) {
	registry := NewRegistry()
	older := time.Now().Add(-time.Minute)
	newer := time.Now()

	registry.Sync(6, true, newer)
	require.True(t, registry.Enabled(6))

	// A lagging scheduler-cache payload must not undo a newer admin edit.
	registry.Sync(6, false, older)
	require.True(t, registry.Enabled(6))

	registry.Sync(6, false, newer.Add(time.Second))
	require.False(t, registry.Enabled(6))

	// Re-enabling starts a fresh negotiation.
	registry.Sync(6, true, newer.Add(2*time.Second))
	require.True(t, registry.Enabled(6))
}

// TestRegistrySyncUnknownKeepsNegotiatedState pins that an observation without
// a switch value (a scheduler-cache payload written before the switch existed)
// refreshes ordering metadata but leaves the negotiated coding alone. Treating
// that absence as "off" dropped the state on every cache read and made every
// request probe the upstream again.
func TestRegistrySyncUnknownKeepsNegotiatedState(t *testing.T) {
	registry := NewRegistry()
	newer := time.Now()

	registry.Sync(23, true, newer)
	slot := registry.existingSlot(23)
	require.NotNil(t, slot)
	settled := slot.currentState()
	require.NotNil(t, settled)
	settled.mu.Lock()
	settled.current = AlgorithmZstd
	settled.tested = true
	settled.remaining = nil
	settled.mu.Unlock()

	registry.SyncUnknown(23, newer.Add(time.Second))
	require.True(t, registry.Enabled(23))
	require.Same(t, settled, slot.currentState())
	require.Equal(t, AlgorithmZstd, slot.currentState().current)

	// An explicitly disabled snapshot still drops the negotiated state.
	registry.Sync(23, false, newer.Add(2*time.Second))
	require.False(t, registry.Enabled(23))

	// A lagging unknown observation cannot resurrect it.
	registry.SyncUnknown(23, newer)
	require.False(t, registry.Enabled(23))
}

func TestExecuteDisabledAccountForwardsUnchanged(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(7, false, time.Time{})

	calls := 0
	_, err := registry.Execute(7, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		calls++
		require.Empty(t, req.Header.Get("Content-Encoding"))
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.False(t, registry.Enabled(7))
}

func TestExecuteDoesNotLeakClosedRejectionBody(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(8, true, time.Time{})

	closed := 0
	closer := func(body io.ReadCloser) io.ReadCloser {
		return &closeCounter{ReadCloser: body, closed: &closed}
	}
	_, err := registry.Execute(8, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		resp := rejectionResponse("unsupported content encoding")
		resp.Body = closer(resp.Body)
		return resp, nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, closed, "each rejected attempt must be closed before the identity fallback")
}

func TestExecuteRejectsNilSend(t *testing.T) {
	registry := NewRegistry()
	registry.Sync(9, true, time.Time{})
	_, err := registry.Execute(9, newTestRequest(t, testBody()), nil)
	require.Error(t, err)
}

func TestExecuteUsesInjectedHTTPClient(t *testing.T) {
	// End-to-end sanity check: a real upstream that only accepts zstd.
	body := testBody()
	var encodings []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoding := strings.ToLower(r.Header.Get("Content-Encoding"))
		encodings = append(encodings, encoding)
		if encoding == "br" || encoding == "deflate" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			_, _ = w.Write([]byte(`{"error":"unsupported content encoding"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	registry := NewRegistry()
	registry.Sync(10, true, time.Time{})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, bytes.NewReader(body))
	require.NoError(t, err)

	resp, err := registry.Execute(10, req, func(attempt *http.Request) (*http.Response, error) {
		return http.DefaultClient.Do(attempt)
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, _ = io.Copy(io.Discard, resp.Body)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []string{"br", "zstd"}, encodings)
}

func TestOriginalRequestBodyAnnotationRetained(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(11, true, time.Time{})
	_, err := registry.Execute(11, newTestRequest(t, body), func(req *http.Request) (*http.Response, error) {
		original, ok := traffic.OriginalRequestBodyBytes(req.Context())
		require.True(t, ok)
		require.Equal(t, int64(len(body)), original)
		require.NotEmpty(t, req.Header.Get("Content-Encoding"))
		require.NotEmpty(t, req.GetBody)
		require.Equal(t, int64(len(mustCompress(t, body, Algorithm(req.Header.Get("Content-Encoding"))))), req.ContentLength)
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
}

// TestExecuteKeepsDiscardedProbeTrafficOutOfUsage verifies that probing codings
// the upstream rejects never reaches the request's usage record: a trial that
// falls back to identity must be recorded as one plaintext request.
func TestExecuteKeepsDiscardedProbeTrafficOutOfUsage(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(19, true, time.Time{})

	collector := traffic.NewCollector()
	req := newTestRequest(t, body)
	req = req.WithContext(traffic.WithCollector(req.Context(), collector))

	var probes []*traffic.Collector
	send := func(attempt *http.Request) (*http.Response, error) {
		probe := traffic.FromContext(attempt.Context())
		require.NotNil(t, probe)
		encoding := attempt.Header.Get("Content-Encoding")
		if encoding == "" {
			require.Same(t, collector, probe, "the identity fallback is the delivered request")
			counter := probe.Begin(traffic.UpstreamRequest, false)
			counter.AddOriginal(len(body))
			counter.AddActual(len(body))
			counter.CompleteOriginal()
			counter.CompleteActual()
			return okResponse("identity"), nil
		}
		require.NotSame(t, collector, probe, "a coded attempt must be measured apart")
		probes = append(probes, probe)
		counter := probe.Begin(traffic.UpstreamRequest, true)
		counter.SetOriginal(int64(len(body)))
		counter.AddActual(len(body) / 4)
		counter.CompleteActual()
		return rejectionResponse("unsupported content encoding"), nil
	}

	resp, err := registry.Execute(19, req, send)
	require.NoError(t, err)
	require.Equal(t, "identity", readAll(t, resp.Body))
	require.Len(t, probes, 3)

	collector.Finalize()
	usage := collector.Snapshot()
	require.NotNil(t, usage.Upstream)
	require.NotNil(t, usage.Upstream.Request)
	require.False(t, usage.Upstream.Request.Compressed)
	require.EqualValues(t, len(body), *usage.Upstream.Request.OriginalBytes)
	require.EqualValues(t, len(body), *usage.Upstream.Request.ActualBytes)
	require.NotNil(t, usage.Upstream.Request.Ratio())
	require.Zero(t, *usage.Upstream.Request.Ratio(), "a plaintext request saves nothing")
}

// TestExecuteAdoptsDeliveredCodingTraffic verifies that the accepted attempt is
// still recorded: its probe counters move into the request's collector, and
// bodies wrapped by the transport keep updating them after the move.
func TestExecuteAdoptsDeliveredCodingTraffic(t *testing.T) {
	body := testBody()
	registry := NewRegistry()
	registry.Sync(20, true, time.Time{})

	collector := traffic.NewCollector()
	req := newTestRequest(t, body)
	req = req.WithContext(traffic.WithCollector(req.Context(), collector))

	const sentBytes = 128
	send := func(attempt *http.Request) (*http.Response, error) {
		probe := traffic.FromContext(attempt.Context())
		require.NotNil(t, probe)
		require.NotSame(t, collector, probe)
		counter := probe.Begin(traffic.UpstreamRequest, true)
		counter.SetOriginal(int64(len(body)))
		counter.AddActual(sentBytes)
		counter.CompleteActual()
		responseCounter := probe.Begin(traffic.UpstreamResponse, false)
		responseCounter.AddActual(16)
		responseCounter.AddOriginal(16)
		responseCounter.CompleteActual()
		responseCounter.CompleteOriginal()
		return okResponse("compressed"), nil
	}

	resp, err := registry.Execute(20, req, send)
	require.NoError(t, err)
	require.Equal(t, "compressed", readAll(t, resp.Body))

	collector.Finalize()
	usage := collector.Snapshot()
	require.NotNil(t, usage.Upstream)
	require.NotNil(t, usage.Upstream.Request)
	require.True(t, usage.Upstream.Request.Compressed)
	require.EqualValues(t, len(body), *usage.Upstream.Request.OriginalBytes)
	require.EqualValues(t, sentBytes, *usage.Upstream.Request.ActualBytes)
	require.NotNil(t, usage.Upstream.Request.Ratio())
	require.NotNil(t, usage.Upstream.Response)
	require.EqualValues(t, 16, *usage.Upstream.Response.ActualBytes)
}

func mustCompress(t *testing.T, body []byte, algorithm Algorithm) []byte {
	t.Helper()
	encoded, err := encodeBody(algorithm, body)
	require.NoError(t, err)
	return encoded
}

func originalBodyBytes(t *testing.T, req *http.Request) int64 {
	t.Helper()
	original, ok := traffic.OriginalRequestBodyBytes(req.Context())
	require.True(t, ok)
	return original
}

func readAll(t *testing.T, reader io.Reader) string {
	t.Helper()
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)
	return string(raw)
}

func errorMessage(t *testing.T, body io.ReadCloser) string {
	t.Helper()
	raw := readAll(t, body)
	require.NoError(t, body.Close())
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	require.Contains(t, raw, "message")
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	return payload.Error.Message
}

type closeCounter struct {
	io.ReadCloser
	closed *int
}

func (c *closeCounter) Close() error {
	*c.closed = *c.closed + 1
	return c.ReadCloser.Close()
}

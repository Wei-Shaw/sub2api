package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestPrepareOpenAIRequestWireBodyEligibility(t *testing.T) {
	body := []byte(`{"model":"gpt-5","stream":true,"input":"hello"}`)
	tests := []struct {
		name       string
		cfgEnabled bool
		allow      bool
		account    *Account
		targetURL  string
		body       []byte
		want       bool
		wantReason string
	}{
		{
			name:       "eligible oauth codex responses",
			cfgEnabled: true,
			allow:      true,
			account:    openAIRequestCompressionOAuthAccount(1, nil),
			targetURL:  chatgptCodexURL,
			body:       body,
			want:       true,
		},
		{
			name:       "callsite out of scope",
			cfgEnabled: true,
			account:    openAIRequestCompressionOAuthAccount(1, nil),
			targetURL:  chatgptCodexURL,
			body:       body,
			wantReason: openAIRequestCompressionSkipCallsite,
		},
		{
			name:       "global hard gate",
			allow:      true,
			account:    openAIRequestCompressionOAuthAccount(1, map[string]any{"openai_request_compression": true}),
			targetURL:  chatgptCodexURL,
			body:       body,
			wantReason: openAIRequestCompressionSkipConfig,
		},
		{
			name:       "account opt out",
			cfgEnabled: true,
			allow:      true,
			account:    openAIRequestCompressionOAuthAccount(1, map[string]any{"openai_request_compression": false}),
			targetURL:  chatgptCodexURL,
			body:       body,
			wantReason: openAIRequestCompressionSkipAccount,
		},
		{
			name:       "api key",
			cfgEnabled: true,
			allow:      true,
			account:    &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			targetURL:  openaiPlatformAPIURL,
			body:       body,
			wantReason: openAIRequestCompressionSkipNotOAuth,
		},
		{
			name:       "compact endpoint",
			cfgEnabled: true,
			allow:      true,
			account:    openAIRequestCompressionOAuthAccount(1, nil),
			targetURL:  chatgptCodexURL + "/compact",
			body:       body,
			wantReason: openAIRequestCompressionSkipEndpoint,
		},
		{
			name:       "final body not streaming",
			cfgEnabled: true,
			allow:      true,
			account:    openAIRequestCompressionOAuthAccount(1, nil),
			targetURL:  chatgptCodexURL,
			body:       []byte(`{"stream":false}`),
			wantReason: openAIRequestCompressionSkipNotStreaming,
		},
		{
			name:       "string true is not boolean true",
			cfgEnabled: true,
			allow:      true,
			account:    openAIRequestCompressionOAuthAccount(1, nil),
			targetURL:  chatgptCodexURL,
			body:       []byte(`{"stream":"true"}`),
			wantReason: openAIRequestCompressionSkipNotStreaming,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIRequestCompression.Enabled = tt.cfgEnabled
			svc := &OpenAIGatewayService{cfg: cfg}
			wire := svc.prepareOpenAIRequestWireBody(
				context.Background(),
				tt.account,
				tt.targetURL,
				tt.body,
				openAIUpstreamRequestBuildOptions{
					AllowRequestCompression: tt.allow,
					CompressionScope:        newOpenAIRequestCompressionScope(),
				},
			)
			require.Equal(t, tt.want, wire.Compressed)
			require.Equal(t, tt.wantReason, wire.SkipReason)
			if tt.want {
				require.Equal(t, tt.body, decodeOpenAIRequestZstdBody(t, wire.Body))
			} else {
				require.Equal(t, tt.body, wire.Body)
			}
		})
	}
}

func TestOpenAIRequestCompressionCacheReusesAcrossAccountsAndReplacesImmutably(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIRequestCompression.Enabled = true
	svc := &OpenAIGatewayService{cfg: cfg}
	scope := newOpenAIRequestCompressionScope()
	opts := openAIUpstreamRequestBuildOptions{AllowRequestCompression: true, CompressionScope: scope}
	body1 := []byte(`{"model":"gpt-5","stream":true,"input":"first"}`)
	body2 := []byte(`{"model":"gpt-5","stream":true,"input":"second"}`)

	wire1 := svc.prepareOpenAIRequestWireBody(context.Background(), openAIRequestCompressionOAuthAccount(1, nil), chatgptCodexURL, body1, opts)
	require.True(t, wire1.Compressed)
	require.False(t, wire1.Reused)
	req1, err := newOpenAIRequestWithWireBody(context.Background(), http.MethodPost, chatgptCodexURL, wire1.Body)
	require.NoError(t, err)
	t.Cleanup(func() { _ = req1.Body.Close() })

	wire1Again := svc.prepareOpenAIRequestWireBody(context.Background(), openAIRequestCompressionOAuthAccount(2, nil), chatgptCodexURL, body1, opts)
	require.True(t, wire1Again.Reused)
	require.Equal(t, wire1.Body, wire1Again.Body)

	wire2 := svc.prepareOpenAIRequestWireBody(context.Background(), openAIRequestCompressionOAuthAccount(2, nil), chatgptCodexURL, body2, opts)
	require.False(t, wire2.Reused)
	require.Equal(t, body2, decodeOpenAIRequestZstdBody(t, wire2.Body))

	replayed, err := req1.GetBody()
	require.NoError(t, err)
	replayedBytes, err := io.ReadAll(replayed)
	require.NoError(t, err)
	require.NoError(t, replayed.Close())
	require.Equal(t, body1, decodeOpenAIRequestZstdBody(t, replayedBytes))
}

func TestOpenAIRequestCompressionFallbackStateSurvivesBodyChanges(t *testing.T) {
	scope := newOpenAIRequestCompressionScope()
	require.True(t, scope.TryConsumeFallback())
	require.False(t, scope.TryConsumeFallback())
	require.True(t, scope.IsForceUncompressed())

	cfg := &config.Config{}
	cfg.Gateway.OpenAIRequestCompression.Enabled = true
	svc := &OpenAIGatewayService{cfg: cfg}
	body := []byte(`{"stream":true,"input":"rewritten"}`)
	wire := svc.prepareOpenAIRequestWireBody(
		context.Background(),
		openAIRequestCompressionOAuthAccount(1, nil),
		chatgptCodexURL,
		body,
		openAIUpstreamRequestBuildOptions{AllowRequestCompression: true, CompressionScope: scope},
	)
	require.False(t, wire.Compressed)
	require.Equal(t, openAIRequestCompressionSkipForcedIdentity, wire.SkipReason)
	require.Equal(t, body, wire.Body)
}

func TestOpenAIRequestWireHeadersAndReplay(t *testing.T) {
	body := []byte("compressed-wire-body")
	req, err := newOpenAIRequestWithWireBody(context.Background(), http.MethodPost, chatgptCodexURL, body)
	require.NoError(t, err)
	require.Equal(t, int64(len(body)), req.ContentLength)
	req.Header.Set("Content-Encoding", "gzip")
	applyOpenAIRequestWireHeaders(req, openAIRequestWireBody{Body: body, Compressed: true})
	require.Equal(t, "zstd", req.Header.Get("Content-Encoding"))
	require.Equal(t, "application/json", req.Header.Get("Content-Type"))

	replayed, err := req.GetBody()
	require.NoError(t, err)
	replayedBytes, err := io.ReadAll(replayed)
	require.NoError(t, err)
	require.Equal(t, body, replayedBytes)
	require.NoError(t, replayed.Close())

	resp := &http.Response{Request: req}
	releaseOpenAIRequestReplayReferences(req, resp)
	require.Nil(t, req.GetBody)
	require.NoError(t, req.Body.Close())

	plainReq, err := newOpenAIRequestWithWireBody(context.Background(), http.MethodPost, chatgptCodexURL, body)
	require.NoError(t, err)
	plainReq.Header.Set("Content-Encoding", "gzip")
	applyOpenAIRequestWireHeaders(plainReq, openAIRequestWireBody{Body: body, ManageContentEncoding: true})
	require.Empty(t, plainReq.Header.Get("Content-Encoding"))
	require.NoError(t, plainReq.Body.Close())

	unmanagedReq, err := newOpenAIRequestWithWireBody(context.Background(), http.MethodPost, openaiPlatformAPIURL, body)
	require.NoError(t, err)
	unmanagedReq.Header.Set("Content-Encoding", "br")
	applyOpenAIRequestWireHeaders(unmanagedReq, openAIRequestWireBody{Body: body})
	require.Equal(t, "br", unmanagedReq.Header.Get("Content-Encoding"))
	require.NoError(t, unmanagedReq.Body.Close())
}

func TestShouldFallbackOpenAIRequestCompression(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		code    string
		message string
		want    bool
	}{
		{name: "415", status: http.StatusUnsupportedMediaType, want: true},
		{name: "explicit code", status: http.StatusBadRequest, code: "unsupported-content-encoding", want: true},
		{name: "explicit phrase", status: http.StatusUnprocessableEntity, message: "Failed to decompress zstd request body", want: true},
		{name: "business code wins", status: http.StatusBadRequest, code: "unsupported_parameter", message: "unsupported content-encoding: zstd", want: false},
		{name: "generic json error", status: http.StatusBadRequest, message: "failed to parse request body", want: false},
		{name: "transport-like 500", status: http.StatusInternalServerError, message: "invalid zstd frame", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldFallbackOpenAIRequestCompression(tt.status, tt.code, tt.message)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestClaimOpenAIRequestCompressionMetricsLogRateLimitsByTime(t *testing.T) {
	var lastLogAt atomic.Int64
	startedAt := time.Unix(1000, 0)

	require.True(t, claimOpenAIRequestCompressionMetricsLog(&lastLogAt, startedAt))
	require.False(t, claimOpenAIRequestCompressionMetricsLog(&lastLogAt, startedAt.Add(time.Second)))
	require.False(t, claimOpenAIRequestCompressionMetricsLog(&lastLogAt, startedAt.Add(openAIRequestCompressionMetricsLogInterval-time.Nanosecond)))
	require.True(t, claimOpenAIRequestCompressionMetricsLog(&lastLogAt, startedAt.Add(openAIRequestCompressionMetricsLogInterval)))
	require.False(t, claimOpenAIRequestCompressionMetricsLog(&lastLogAt, startedAt.Add(openAIRequestCompressionMetricsLogInterval)))
	require.False(t, claimOpenAIRequestCompressionMetricsLog(nil, startedAt))
}

func TestOpenAIRequestCompressionMetricsCountOnlyEnabledEvaluations(t *testing.T) {
	body := []byte(`{"stream":true,"input":"hello"}`)
	before := SnapshotOpenAIRequestCompressionMetrics()

	disabled := &OpenAIGatewayService{cfg: &config.Config{}}
	disabled.prepareOpenAIRequestWireBody(context.Background(), openAIRequestCompressionOAuthAccount(1, nil), chatgptCodexURL, body, openAIUpstreamRequestBuildOptions{AllowRequestCompression: true})

	enabledCfg := &config.Config{}
	enabledCfg.Gateway.OpenAIRequestCompression.Enabled = true
	enabled := &OpenAIGatewayService{cfg: enabledCfg}
	enabled.prepareOpenAIRequestWireBody(context.Background(), openAIRequestCompressionOAuthAccount(1, nil), chatgptCodexURL, body, openAIUpstreamRequestBuildOptions{})

	afterDisabled := SnapshotOpenAIRequestCompressionMetrics()
	require.Equal(t, before.EvaluatedAttemptsTotal, afterDisabled.EvaluatedAttemptsTotal)

	enabled.prepareOpenAIRequestWireBody(context.Background(), &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, openaiPlatformAPIURL, body, openAIUpstreamRequestBuildOptions{AllowRequestCompression: true})
	enabled.prepareOpenAIRequestWireBody(context.Background(), openAIRequestCompressionOAuthAccount(2, map[string]any{openAIRequestCompressionExtraKey: false}), chatgptCodexURL, body, openAIUpstreamRequestBuildOptions{AllowRequestCompression: true})
	enabled.prepareOpenAIRequestWireBody(context.Background(), openAIRequestCompressionOAuthAccount(3, nil), chatgptCodexURL+"/compact", body, openAIUpstreamRequestBuildOptions{AllowRequestCompression: true})
	enabled.prepareOpenAIRequestWireBody(context.Background(), openAIRequestCompressionOAuthAccount(4, nil), chatgptCodexURL, []byte(`{"stream":false}`), openAIUpstreamRequestBuildOptions{AllowRequestCompression: true})
	forcedScope := newOpenAIRequestCompressionScope()
	require.True(t, forcedScope.TryConsumeFallback())
	enabled.prepareOpenAIRequestWireBody(context.Background(), openAIRequestCompressionOAuthAccount(5, nil), chatgptCodexURL, body, openAIUpstreamRequestBuildOptions{AllowRequestCompression: true, CompressionScope: forcedScope})

	after := SnapshotOpenAIRequestCompressionMetrics()
	require.Equal(t, afterDisabled.EvaluatedAttemptsTotal+5, after.EvaluatedAttemptsTotal)
	require.Equal(t, afterDisabled.SkipNotOAuthTotal+1, after.SkipNotOAuthTotal)
	require.Equal(t, afterDisabled.SkipAccountOverrideTotal+1, after.SkipAccountOverrideTotal)
	require.Equal(t, afterDisabled.SkipEndpointTotal+1, after.SkipEndpointTotal)
	require.Equal(t, afterDisabled.SkipNotStreamingTotal+1, after.SkipNotStreamingTotal)
	require.Equal(t, afterDisabled.SkipForcedUncompressedTotal+1, after.SkipForcedUncompressedTotal)
}

func TestLogOpenAIRequestCompressionMetricsIsStructuredAndBodyFree(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	logOpenAIRequestCompressionMetrics(zap.New(core), OpenAIRequestCompressionMetricsSnapshot{
		EvaluatedAttemptsTotal:      7,
		CompressedAttemptsTotal:     5,
		CompressionOperationsTotal:  4,
		CompressionCacheHitsTotal:   1,
		FallbackUncompressedTotal:   1,
		CompressionErrorsTotal:      0,
		PreCompressionBytesTotal:    1000,
		PostCompressionBytesTotal:   400,
		CompressionDurationNsTotal:  uint64(2500 * time.Microsecond),
		SkipNotOAuthTotal:           1,
		SkipAccountOverrideTotal:    2,
		SkipEndpointTotal:           3,
		SkipNotStreamingTotal:       4,
		SkipForcedUncompressedTotal: 5,
	})

	entries := observed.FilterMessage("openai.request_compression_metrics").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.EqualValues(t, 7, fields["evaluated_attempts_total"])
	require.EqualValues(t, 5, fields["compressed_attempts_total"])
	require.EqualValues(t, 2500, fields["compression_duration_us_total"])
	require.InDelta(t, 2.5, fields["compression_ratio"], 0.001)
	require.InDelta(t, 60, fields["savings_percent"], 0.001)
	require.Equal(t, "service.openai_gateway.request_compression", fields["component"])
	require.NotContains(t, fields, "body")
	require.NotContains(t, fields, "request_id")
	require.NotContains(t, fields, "path")
}

func TestLogOpenAIRequestCompressionFallbackIsStructuredAndBodyFree(t *testing.T) {
	for _, flow := range []string{"converted", "passthrough", "ws_http_bridge"} {
		t.Run(flow, func(t *testing.T) {
			core, observed := observer.New(zap.InfoLevel)
			ctx := logger.IntoContext(context.Background(), zap.New(core).With(zap.String("request_id", "rid-test")))
			logOpenAIRequestCompressionFallback(ctx, flow, &Account{ID: 42}, http.StatusUnsupportedMediaType, "unsupported-content-encoding")

			entries := observed.FilterMessage("openai.request_compression_fallback").All()
			require.Len(t, entries, 1)
			fields := entries[0].ContextMap()
			require.Equal(t, flow, fields["flow"])
			require.EqualValues(t, 42, fields["account_id"])
			require.EqualValues(t, http.StatusUnsupportedMediaType, fields["status_code"])
			require.Equal(t, "unsupported_content_encoding", fields["upstream_error_code"])
			require.Equal(t, true, fields["fallback_uncompressed"])
			require.Equal(t, "rid-test", fields["request_id"])
			require.NotContains(t, fields, "body")
		})
	}
}

func TestIsOpenAIRequestZstdCompressedRequiresOAuthCodexEndpoint(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, chatgptCodexURL, nil)
	require.NoError(t, err)
	req.Header.Set("Content-Encoding", "zstd")
	require.True(t, isOpenAIRequestZstdCompressed(req, openAIRequestCompressionOAuthAccount(1, nil)))
	require.False(t, isOpenAIRequestZstdCompressed(req, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}))

	compactReq, err := http.NewRequest(http.MethodPost, chatgptCodexURL+"/compact", nil)
	require.NoError(t, err)
	compactReq.Header.Set("Content-Encoding", "zstd")
	require.False(t, isOpenAIRequestZstdCompressed(compactReq, openAIRequestCompressionOAuthAccount(1, nil)))
}

func openAIRequestCompressionOAuthAccount(id int64, extra map[string]any) *Account {
	return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
}

func decodeOpenAIRequestZstdBody(t *testing.T, body []byte) []byte {
	t.Helper()
	decoder, err := zstd.NewReader(nil)
	require.NoError(t, err)
	t.Cleanup(decoder.Close)
	decoded, err := decoder.DecodeAll(body, nil)
	require.NoError(t, err)
	return decoded
}

func TestOpenAIReleaseOnCloseReaderDropsPayload(t *testing.T) {
	payload := []byte("payload")
	reader := &openAIReleaseOnCloseReader{reader: bytes.NewReader(payload)}
	require.NoError(t, reader.Close())
	buffer := make([]byte, 1)
	n, err := reader.Read(buffer)
	require.Zero(t, n)
	require.ErrorIs(t, err, io.EOF)
}

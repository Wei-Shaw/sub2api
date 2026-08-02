package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	openAIRequestCompressionScopeContextKey = "openai_request_compression_scope"
	openAIRequestCompressionProfile         = "zstd-v1-level3"
	openAIRequestCompressionMaxConcurrency  = 4
)

const (
	openAIRequestCompressionSkipCallsite         = "callsite_out_of_scope"
	openAIRequestCompressionSkipConfig           = "config_disabled"
	openAIRequestCompressionSkipAccount          = "account_override_disabled"
	openAIRequestCompressionSkipNotOAuth         = "account_not_openai_oauth"
	openAIRequestCompressionSkipEndpoint         = "endpoint_not_codex_responses"
	openAIRequestCompressionSkipNotStreaming     = "final_body_not_streaming"
	openAIRequestCompressionSkipForcedIdentity   = "fallback_uncompressed"
	openAIRequestCompressionSkipEncoderInitError = "encoder_init_error"
)

type openAIUpstreamRequestBuildOptions struct {
	AllowRequestCompression bool
	CompressionScope        *openAIRequestCompressionScope
}

type openAIRequestWireBody struct {
	Body                  []byte
	Compressed            bool
	ManageContentEncoding bool
	Reused                bool
	SkipReason            string
}

type openAICompressedEntry struct {
	profile string
	digest  [sha256.Size]byte
	body    []byte
}

type openAIRequestCompressionScope struct {
	mu sync.Mutex

	fallbackConsumed  bool
	forceUncompressed bool
	entry             *openAICompressedEntry
}

func newOpenAIRequestCompressionScope() *openAIRequestCompressionScope {
	return &openAIRequestCompressionScope{}
}

func ensureOpenAIRequestCompressionScope(c *gin.Context) *openAIRequestCompressionScope {
	if c == nil {
		return newOpenAIRequestCompressionScope()
	}
	if value, ok := c.Get(openAIRequestCompressionScopeContextKey); ok {
		if scope, ok := value.(*openAIRequestCompressionScope); ok && scope != nil {
			return scope
		}
	}
	scope := newOpenAIRequestCompressionScope()
	c.Set(openAIRequestCompressionScopeContextKey, scope)
	return scope
}

func releaseOpenAIRequestCompressionPayload(c *gin.Context) {
	if c == nil {
		return
	}
	value, ok := c.Get(openAIRequestCompressionScopeContextKey)
	if !ok {
		return
	}
	if scope, ok := value.(*openAIRequestCompressionScope); ok && scope != nil {
		scope.ReleasePayload()
	}
}

func (s *openAIRequestCompressionScope) IsForceUncompressed() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.forceUncompressed
}

func (s *openAIRequestCompressionScope) TryConsumeFallback() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallbackConsumed {
		return false
	}
	s.fallbackConsumed = true
	s.forceUncompressed = true
	s.entry = nil
	openAIRequestCompressionStats.fallbackUncompressedTotal.Add(1)
	return true
}

func (s *openAIRequestCompressionScope) ReleasePayload() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.entry = nil
	s.mu.Unlock()
}

func (s *openAIRequestCompressionScope) compressedBody(
	body []byte,
	encoder *zstd.Encoder,
) (compressed []byte, reused bool) {
	digest := sha256.Sum256(body)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.forceUncompressed {
		return nil, false
	}
	if s.entry != nil && s.entry.profile == openAIRequestCompressionProfile && s.entry.digest == digest {
		return s.entry.body, true
	}

	// The result is immutable. Never reuse an older entry's backing array: an
	// already-built request or its GetBody closure may still reference it.
	compressed = encoder.EncodeAll(body, nil)
	s.entry = &openAICompressedEntry{
		profile: openAIRequestCompressionProfile,
		digest:  digest,
		body:    compressed,
	}
	return compressed, false
}

var openAIRequestCompressionEncoder struct {
	once    sync.Once
	encoder *zstd.Encoder
	err     error
}

func getOpenAIRequestCompressionEncoder() (*zstd.Encoder, error) {
	openAIRequestCompressionEncoder.once.Do(func() {
		concurrency := runtime.GOMAXPROCS(0)
		if concurrency < 1 {
			concurrency = 1
		}
		if concurrency > openAIRequestCompressionMaxConcurrency {
			concurrency = openAIRequestCompressionMaxConcurrency
		}
		openAIRequestCompressionEncoder.encoder, openAIRequestCompressionEncoder.err = zstd.NewWriter(
			nil,
			zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)),
			zstd.WithEncoderConcurrency(concurrency),
		)
	})
	return openAIRequestCompressionEncoder.encoder, openAIRequestCompressionEncoder.err
}

func (s *OpenAIGatewayService) prepareOpenAIRequestWireBody(
	ctx context.Context,
	account *Account,
	targetURL string,
	body []byte,
	options openAIUpstreamRequestBuildOptions,
) openAIRequestWireBody {
	wire := openAIRequestWireBody{
		Body: body,
		ManageContentEncoding: options.AllowRequestCompression &&
			account != nil && account.IsOpenAIOAuth() && targetURL == chatgptCodexURL,
	}
	skip := func(reason string) openAIRequestWireBody {
		wire.SkipReason = reason
		logger.FromContext(ctx).Debug("openai request compression skipped", zap.String("skip_reason", reason))
		return wire
	}

	if !options.AllowRequestCompression {
		return skip(openAIRequestCompressionSkipCallsite)
	}
	if !s.isOpenAIRequestCompressionEnabled() {
		return skip(openAIRequestCompressionSkipConfig)
	}
	if account == nil || !account.IsOpenAIOAuth() {
		return skip(openAIRequestCompressionSkipNotOAuth)
	}
	if override := account.GetOpenAIRequestCompressionOverride(); override != nil && !*override {
		return skip(openAIRequestCompressionSkipAccount)
	}
	if targetURL != chatgptCodexURL {
		return skip(openAIRequestCompressionSkipEndpoint)
	}
	stream := gjson.GetBytes(body, "stream")
	if stream.Type != gjson.True {
		return skip(openAIRequestCompressionSkipNotStreaming)
	}
	scope := options.CompressionScope
	if scope == nil {
		scope = newOpenAIRequestCompressionScope()
	}
	if scope.IsForceUncompressed() {
		return skip(openAIRequestCompressionSkipForcedIdentity)
	}
	encoder, err := getOpenAIRequestCompressionEncoder()
	if err != nil {
		openAIRequestCompressionStats.compressionErrorsTotal.Add(1)
		logger.FromContext(ctx).Warn("openai request compression encoder initialization failed", zap.Error(err))
		return skip(openAIRequestCompressionSkipEncoderInitError)
	}

	startedAt := time.Now()
	compressed, reused := scope.compressedBody(body, encoder)
	if compressed == nil {
		return skip(openAIRequestCompressionSkipForcedIdentity)
	}
	duration := time.Since(startedAt)
	wire.Body = compressed
	wire.Compressed = true
	wire.Reused = reused
	openAIRequestCompressionStats.compressedRequestsTotal.Add(1)
	openAIRequestCompressionStats.preCompressionBytesTotal.Add(uint64(len(body)))
	openAIRequestCompressionStats.postCompressionBytesTotal.Add(uint64(len(compressed)))
	if reused {
		openAIRequestCompressionStats.cacheHitsTotal.Add(1)
	} else {
		openAIRequestCompressionStats.compressionOperationsTotal.Add(1)
		openAIRequestCompressionStats.compressionDurationNsTotal.Add(uint64(duration.Nanoseconds()))
	}

	ratio := float64(0)
	savings := float64(0)
	if len(compressed) > 0 {
		ratio = float64(len(body)) / float64(len(compressed))
	}
	if len(body) > 0 {
		savings = (1 - float64(len(compressed))/float64(len(body))) * 100
	}
	logger.FromContext(ctx).Debug("openai request body compressed",
		zap.Int64("account_id", account.ID),
		zap.Int("pre_compression_bytes", len(body)),
		zap.Int("post_compression_bytes", len(compressed)),
		zap.Float64("compression_ratio", ratio),
		zap.Float64("savings_percent", savings),
		zap.Int64("compression_duration_us", duration.Microseconds()),
		zap.Bool("reused", reused),
	)
	return wire
}

func applyOpenAIRequestWireHeaders(req *http.Request, wire openAIRequestWireBody) {
	if req == nil {
		return
	}
	if !wire.ManageContentEncoding && !wire.Compressed {
		return
	}
	req.Header.Del("Content-Encoding")
	if wire.Compressed {
		req.Header.Set("Content-Encoding", "zstd")
		req.Header.Set("Content-Type", "application/json")
	}
}

func isOpenAIRequestZstdCompressed(req *http.Request, account *Account) bool {
	return req != nil && req.URL != nil && account != nil && account.IsOpenAIOAuth() &&
		req.URL.String() == chatgptCodexURL && req.Header.Get("Content-Encoding") == "zstd"
}

type openAIReleaseOnCloseReader struct {
	mu     sync.Mutex
	reader *bytes.Reader
}

func (r *openAIReleaseOnCloseReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reader == nil {
		return 0, io.EOF
	}
	return r.reader.Read(p)
}

func (r *openAIReleaseOnCloseReader) Close() error {
	r.mu.Lock()
	r.reader = nil
	r.mu.Unlock()
	return nil
}

func newOpenAIRequestWithWireBody(
	ctx context.Context,
	method string,
	targetURL string,
	body []byte,
) (*http.Request, error) {
	reader := bytes.NewReader(body)
	req, err := http.NewRequestWithContext(ctx, method, targetURL, reader)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return req, nil
	}
	req.Body = &openAIReleaseOnCloseReader{reader: reader}
	// Preserve replayability during Client.Do redirects/retries, while ensuring
	// each temporary reader drops its large backing slice when closed.
	req.GetBody = func() (io.ReadCloser, error) {
		return &openAIReleaseOnCloseReader{reader: bytes.NewReader(body)}, nil
	}
	return req, nil
}

// releaseOpenAIRequestReplayReferences is called immediately after Do returns.
// Redirects/replays have completed by then, so keeping GetBody would only make
// a long-lived streaming response retain the request payload.
func releaseOpenAIRequestReplayReferences(req *http.Request, resp *http.Response) {
	if req != nil {
		req.GetBody = nil
	}
	if resp != nil && resp.Request != nil {
		resp.Request.GetBody = nil
	}
}

func (s *OpenAIGatewayService) isOpenAIRequestCompressionFallbackEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIRequestCompression.FallbackUncompressed
}

func (s *OpenAIGatewayService) isOpenAIRequestCompressionEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIRequestCompression.Enabled
}

var openAIRequestCompressionBusinessErrorCodes = map[string]struct{}{
	"invalid_encrypted_content": {},
	"unknown_parameter":         {},
	"unsupported_parameter":     {},
}

var openAIRequestCompressionErrorCodes = map[string]struct{}{
	"content_encoding_error":       {},
	"invalid_content_encoding":     {},
	"unsupported_content_encoding": {},
	"zstd_decode_error":            {},
	"zstd_decompression_error":     {},
}

var openAIRequestCompressionErrorPhrases = []string{
	"unsupported content-encoding: zstd",
	"unsupported content encoding: zstd",
	"unsupported content-encoding 'zstd'",
	"unsupported content encoding 'zstd'",
	"content-encoding zstd is not supported",
	"content encoding zstd is not supported",
	"failed to decompress zstd request body",
	"failed to decode zstd request body",
	"invalid zstd frame",
	"zstd decompression failed",
}

func normalizeOpenAIRequestCompressionErrorCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	code = strings.NewReplacer("-", "_", " ", "_").Replace(code)
	return code
}

func shouldFallbackOpenAIRequestCompression(statusCode int, code, message string) bool {
	if statusCode == http.StatusUnsupportedMediaType {
		return true
	}
	if statusCode != http.StatusBadRequest && statusCode != http.StatusUnprocessableEntity {
		return false
	}
	code = normalizeOpenAIRequestCompressionErrorCode(code)
	if _, blocked := openAIRequestCompressionBusinessErrorCodes[code]; blocked {
		return false
	}
	if _, allowed := openAIRequestCompressionErrorCodes[code]; allowed {
		return true
	}
	message = strings.ToLower(strings.TrimSpace(message))
	for _, phrase := range openAIRequestCompressionErrorPhrases {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

type OpenAIRequestCompressionMetricsSnapshot struct {
	CompressedRequestsTotal    uint64
	CompressionOperationsTotal uint64
	CompressionCacheHitsTotal  uint64
	FallbackUncompressedTotal  uint64
	CompressionErrorsTotal     uint64
	PreCompressionBytesTotal   uint64
	PostCompressionBytesTotal  uint64
	CompressionDurationNsTotal uint64
}

var openAIRequestCompressionStats struct {
	compressedRequestsTotal    atomic.Uint64
	compressionOperationsTotal atomic.Uint64
	cacheHitsTotal             atomic.Uint64
	fallbackUncompressedTotal  atomic.Uint64
	compressionErrorsTotal     atomic.Uint64
	preCompressionBytesTotal   atomic.Uint64
	postCompressionBytesTotal  atomic.Uint64
	compressionDurationNsTotal atomic.Uint64
}

func SnapshotOpenAIRequestCompressionMetrics() OpenAIRequestCompressionMetricsSnapshot {
	return OpenAIRequestCompressionMetricsSnapshot{
		CompressedRequestsTotal:    openAIRequestCompressionStats.compressedRequestsTotal.Load(),
		CompressionOperationsTotal: openAIRequestCompressionStats.compressionOperationsTotal.Load(),
		CompressionCacheHitsTotal:  openAIRequestCompressionStats.cacheHitsTotal.Load(),
		FallbackUncompressedTotal:  openAIRequestCompressionStats.fallbackUncompressedTotal.Load(),
		CompressionErrorsTotal:     openAIRequestCompressionStats.compressionErrorsTotal.Load(),
		PreCompressionBytesTotal:   openAIRequestCompressionStats.preCompressionBytesTotal.Load(),
		PostCompressionBytesTotal:  openAIRequestCompressionStats.postCompressionBytesTotal.Load(),
		CompressionDurationNsTotal: openAIRequestCompressionStats.compressionDurationNsTotal.Load(),
	}
}

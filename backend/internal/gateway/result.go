package gateway

import (
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ServiceResultToForwardResult maps a service.ForwardResult (used by
// Anthropic and Antigravity providers) to the protocol-agnostic
// gateway.ForwardResult. Both providers share the same service result
// type, so this single function replaces the per-provider copies.
func ServiceResultToForwardResult(r *service.ForwardResult) *ForwardResult {
	return &ForwardResult{
		RequestID:           r.RequestID,
		Model:               r.Model,
		UpstreamModel:       r.UpstreamModel,
		Stream:              r.Stream,
		Duration:            r.Duration,
		FirstTokenMs:        r.FirstTokenMs,
		InputTokens:         int64(r.Usage.InputTokens),
		OutputTokens:        int64(r.Usage.OutputTokens),
		CacheCreationTokens: int64(r.Usage.CacheCreationInputTokens),
		CacheReadTokens:     int64(r.Usage.CacheReadInputTokens),
		ImageOutputTokens:   int64(r.Usage.ImageOutputTokens),
		ClientDisconnect:    r.ClientDisconnect,
		ImageCount:          r.ImageCount,
		ImageSize:           r.ImageSize,
	}
}

// ForwardResult carries usage and timing data produced by
// GatewayProvider.Forward. The Pipeline passes it to the host-side
// RecordUsage callback for billing and logging.
//
// Token fields use int64 to avoid truncation on high-volume models.
// Platform-specific usage structs (ClaudeUsage, OpenAIUsage) remain in
// the service package; providers map their native result into this
// common representation before returning.
type ForwardResult struct {
	RequestID     string
	Model         string
	UpstreamModel string
	Stream        bool
	Duration      time.Duration
	FirstTokenMs  *int

	// Token counts
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	ImageOutputTokens   int64

	// Client behaviour
	ClientDisconnect bool

	// Image generation billing fields
	ImageCount int
	ImageSize  string // "1K" / "2K" / "4K"

	// ResponseHeaders carries upstream response headers for post-forward
	// processing (e.g. Codex usage snapshot extraction). Only populated
	// by the OpenAI provider.
	ResponseHeaders http.Header
}

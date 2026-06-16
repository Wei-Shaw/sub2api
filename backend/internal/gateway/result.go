package gateway

import (
	"net/http"
	"time"
)

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

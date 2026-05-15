package pluginsdk

import (
	"context"
	"encoding/json"
	"time"
)

// GatewayProviderExtension is the Go interface gateway plugins implement
// to handle request forwarding. This is the plugin-side equivalent of the
// host's gateway.GatewayProvider interface, adapted for gRPC streaming.
//
// Plugins that declare Manifest.Platforms and want to handle forwarding
// SHOULD implement this interface (via GRPCServiceRegistrar) so the host
// can delegate upstream API calls to the plugin.
type GatewayProviderExtension interface {
	// Forward handles a single gateway request. The plugin receives the full
	// request context and streams back the upstream response via the stream
	// interface. The plugin MUST send a Done chunk as the last message.
	Forward(ctx context.Context, req *GatewayForwardReq, stream GatewayForwardStream) error

	// ShouldFailover asks the plugin whether a failed forward should be
	// retried with a different account. Returns true to indicate the error
	// is transient and another account should be tried.
	ShouldFailover(ctx context.Context, req *GatewayFailoverReq) (bool, error)
}

// GatewayForwardReq is the input for GatewayProviderExtension.Forward.
// It is the gRPC-friendly equivalent of gateway.ForwardRequest, minus
// non-serializable fields (http.ResponseWriter, *gin.Context).
type GatewayForwardReq struct {
	// Request identity
	RequestID string
	Model     string
	Stream    bool
	Protocol  string // "anthropic" / "openai" / "gemini"

	// Request body
	RawBody []byte

	// Account info
	Account GatewayAccountInfo

	// Request metadata
	Headers map[string]string
	Method  string
	Path    string

	// Scheduling context
	GroupID        int64
	SessionHash    string
	MetadataUserID string
	UserID         int64
	SwitchCount    int

	// Platform-specific
	GeminiAction    string
	IsStickySession bool
}

// GatewayAccountInfo carries the account credentials and metadata the
// plugin needs to make upstream API calls.
type GatewayAccountInfo struct {
	AccountID   int64
	Platform    string
	AccountType string
	Credentials json.RawMessage
	Extra       json.RawMessage
	Name        string
}

// GatewayForwardStream is the server-side stream interface for sending
// response chunks back to the host. The plugin sends headers first, then
// body chunks, and finally a Done message.
type GatewayForwardStream interface {
	// SendHeaders sends the HTTP response status and headers. Must be
	// called exactly once before any SendBody calls.
	SendHeaders(headers *GatewayResponseHeaders) error

	// SendBody sends a chunk of the response body.
	SendBody(data []byte) error

	// SendTrailers sends optional HTTP trailers after the body.
	SendTrailers(trailers map[string]string) error

	// SendDone signals the end of the stream with optional result data.
	// Must be called exactly once as the last message.
	SendDone(result *GatewayForwardResult, err error) error
}

// GatewayResponseHeaders carries the HTTP response status and headers
// that the host writes to the downstream client.
type GatewayResponseHeaders struct {
	StatusCode int
	Headers    map[string]string
}

// GatewayForwardResult carries usage and timing data produced by the
// plugin's forward operation. This maps to gateway.ForwardResult on the
// host side.
type GatewayForwardResult struct {
	RequestID     string
	Model         string
	UpstreamModel string
	Stream        bool
	Duration      time.Duration
	FirstTokenMs  int // 0 = not measured

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

	// ResponseHeaders carries selected upstream response headers for
	// post-forward processing by the host (e.g. x-codex-* for usage
	// snapshot extraction, x-ratelimit-* for rate limit tracking).
	ResponseHeaders map[string]string
}

// GatewayFailoverReq is the input for ShouldFailover. It carries the
// original request context plus error details so the plugin can decide
// whether to retry with another account.
type GatewayFailoverReq struct {
	Request      *GatewayForwardReq
	ErrorMessage string
	ErrorType    string // Go type name (e.g. "UpstreamFailoverError")
}

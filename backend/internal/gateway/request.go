package gateway

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ForwardRequest is the protocol-agnostic request context passed to
// GatewayProvider.Forward. It is built by the Pipeline from the raw HTTP
// body and enriched with scheduling / billing state at each pipeline stage.
//
// Providers treat all fields as read-only except UpstreamAccepted.
type ForwardRequest struct {
	// --- immutable: set once by Pipeline.parseRequest ---

	RawBody        []byte
	Model          string
	Stream         bool
	GroupID        *int64
	RequestID      string
	Protocol       string // "anthropic" / "openai" / "gemini"
	MetadataUserID string
	SessionHash    string

	// ForcePlatform constrains account selection to a single platform.
	// Empty means the scheduler picks across all platforms in the group.
	ForcePlatform string

	// --- auth context: set by Pipeline.Execute from gin context ---

	APIKey       *service.APIKey
	User         *service.User
	Subscription *service.UserSubscription
	UserID       int64
	Concurrency  int // user max concurrency (from AuthSubject)

	// --- updated per select-account iteration ---

	Account        *service.Account
	ChannelMapping *service.ChannelMappingResult
	BillingTicket  *service.BillingTicket
	SwitchCount    int // failover counter

	// --- Phase 1 adapter bridge fields ---

	// GinContext is the original *gin.Context from the HTTP handler.
	// Phase 1 adapters need it because service-layer Forward methods
	// read headers, write responses, and store gin-level metadata via
	// *gin.Context. Phase 2+ providers (e.g. gRPC plugins) will not
	// use this field; it will be nil for out-of-process providers.
	GinContext *gin.Context

	// GeminiAction is the Gemini API action extracted from the URL path
	// (e.g. "generateContent", "streamGenerateContent"). Only set when
	// Protocol == "gemini".
	GeminiAction string

	// IsStickySession indicates whether the current request matched an
	// existing sticky-session binding. Antigravity uses this to decide
	// cache billing on session switches.
	IsStickySession bool

	// --- writable by provider ---

	// UpstreamAccepted is set to true by the provider once the upstream
	// has begun streaming a response. After this point the Pipeline must
	// not attempt failover to another account.
	UpstreamAccepted bool
}

package gateway

import "github.com/Wei-Shaw/sub2api/internal/service"

// ForwardRequest is the protocol-agnostic request context passed to
// GatewayProvider.Forward. It is built by the Pipeline from the raw HTTP
// body and enriched with scheduling / billing state at each pipeline stage.
//
// Providers treat all fields as read-only except UpstreamAccepted.
type ForwardRequest struct {
	// --- immutable: set once by Pipeline.parseRequest ---

	RawBody     []byte
	Model       string
	Stream      bool
	GroupID     *int64
	RequestID   string
	Protocol    string // "anthropic" / "openai" / "gemini"
	MetadataUserID string
	SessionHash    string

	// ForcePlatform constrains account selection to a single platform.
	// Empty means the scheduler picks across all platforms in the group.
	ForcePlatform string

	// --- updated per select-account iteration ---

	Account        *service.Account
	ChannelMapping *service.ChannelMappingResult
	BillingTicket  *service.BillingTicket
	SwitchCount    int // failover counter

	// --- writable by provider ---

	// UpstreamAccepted is set to true by the provider once the upstream
	// has begun streaming a response. After this point the Pipeline must
	// not attempt failover to another account.
	UpstreamAccepted bool
}

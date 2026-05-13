package gateway

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Protocol constants for pipeline dispatch.
const (
	ProtocolAnthropic       = "anthropic"
	ProtocolChatCompletions = "chat_completions"
	ProtocolResponses       = "responses"
	ProtocolOpenAI          = "openai"
	ProtocolGemini          = "gemini"

	// ProtocolAnthropicViaOpenAI dispatches Anthropic-format Messages
	// requests through OpenAI accounts (ForwardAsAnthropic).
	ProtocolAnthropicViaOpenAI = "anthropic_via_openai"

	// ProtocolImages dispatches OpenAI Images API requests
	// (ForwardImages for /v1/images/generations and /v1/images/edits).
	ProtocolImages = "images"

	// ProtocolCountTokens dispatches Anthropic count_tokens requests.
	// No usage recording or failover; only billing eligibility and
	// account selection are required.
	ProtocolCountTokens = "count_tokens"

	// ProtocolResponsesWS dispatches OpenAI Responses WebSocket v2
	// sessions through the WS pipeline (ExecuteWS). Unlike HTTP
	// protocols, no SSE ping or failover loop is used.
	ProtocolResponsesWS = "responses_ws"
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

	Account           *service.Account
	ChannelMapping    *service.ChannelMappingResult
	BillingTicket     *service.BillingTicket
	SwitchCount       int  // failover counter
	ForceCacheBilling bool // set when failover occurs after billing was consumed

	// --- Phase 1 adapter bridge fields ---

	// GinContext is the original *gin.Context from the HTTP handler.
	// Phase 1 adapters need it because service-layer Forward methods
	// read headers, write responses, and store gin-level metadata via
	// *gin.Context. Phase 2+ providers (e.g. gRPC plugins) will not
	// use this field; it will be nil for out-of-process providers.
	GinContext *gin.Context

	// PromptCacheKey is the OpenAI prompt cache key extracted from the
	// request body or headers. Used by the OpenAI ChatCompletions path
	// (ForwardAsChatCompletions) for session affinity.
	PromptCacheKey string

	// DefaultMappedModel is the fallback model resolved from the group's
	// default_mapped_model setting. Used by the OpenAI ChatCompletions
	// path when the requested model is unavailable.
	DefaultMappedModel string

	// GeminiAction is the Gemini API action extracted from the URL path
	// (e.g. "generateContent", "streamGenerateContent"). Only set when
	// Protocol == "gemini".
	GeminiAction string

	// IsStickySession indicates whether the current request matched an
	// existing sticky-session binding. Antigravity uses this to decide
	// cache billing on session switches.
	IsStickySession bool

	// ImagesRequest carries the parsed OpenAI Images request metadata
	// (endpoint, model, multipart data, capability). Only set when
	// Protocol == ProtocolImages.
	ImagesRequest *service.OpenAIImagesRequest

	// --- detection / scheduling context (set by handler pipeline) ---

	// IsClaudeCodeClient is true when the request originates from a
	// Claude Code CLI session. Set by Messages handler pipeline via
	// SetClaudeCodeClientContext detection.
	IsClaudeCodeClient bool

	// ThinkingEnabled indicates whether extended thinking is enabled
	// for this request. Set by Messages handler pipeline from parsed
	// request body.
	ThinkingEnabled bool

	// ChannelMappedModel is the model name after channel mapping. Empty
	// when no mapping is active or before channel mapping runs.
	ChannelMappedModel string

	// --- writable by provider ---

	// UpstreamAccepted is set to true by the provider once the upstream
	// has begun streaming a response. After this point the Pipeline must
	// not attempt failover to another account.
	UpstreamAccepted bool
}

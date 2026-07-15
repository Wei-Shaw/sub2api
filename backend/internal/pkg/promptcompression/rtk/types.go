// Package rtk contains the protocol-neutral, fail-open RTK prompt compression
// engine.  The package deliberately has no dependency on handlers, storage, or
// network clients so it can be used from HTTP and websocket gateways alike.
package rtk

import "time"

type Protocol string

const (
	ProtocolAnthropic Protocol = "anthropic"
	ProtocolChat      Protocol = "chat_completions"
	ProtocolResponses Protocol = "responses"
	ProtocolGemini    Protocol = "gemini"
)

type Mode string

const (
	ModeOff     Mode = "off"
	ModeObserve Mode = "observe"
	ModeEnforce Mode = "enforce"
)

type Intensity string

const (
	IntensitySafe       Intensity = "safe"
	IntensityBalanced   Intensity = "balanced"
	IntensityAggressive Intensity = "aggressive"
)

// Config contains only immutable engine settings. Runtime policy (group,
// rollout, emergency stop) is intentionally resolved by the service layer.
type Config struct {
	Mode               Mode
	Intensity          Intensity
	Model              string
	MinCandidateBytes  int
	MinCandidateTokens int
	MinSavedTokens     int
	MaxBodyBytes       int
	MaxResultBytes     int
	MaxDuration        time.Duration
	EnableGrouping     bool
	EnableRenderers    bool
}

type Options struct {
	Protocol  Protocol
	Model     string
	Mode      Mode
	Intensity Intensity
	// ProtectedJSONPaths are paths whose string values must never be modified.
	// Paths use gjson/sjson notation (for example messages.0.content.1.text).
	ProtectedJSONPaths map[string]bool
	// Filter overrides the engine's compiled filters for this call. A nil value
	// uses the filters captured by NewEngine.
	Filters []Filter
}

type Target struct {
	Protocol       Protocol
	JSONPath       string
	ToolCallID     string
	ToolName       string
	OutputType     string
	Confidence     float64
	Command        string
	Text           string
	IsError        bool
	CacheProtected bool
	Eligible       bool
	SkipReason     string
}

type Result struct {
	Body            []byte
	Applied         bool
	Mode            Mode
	Outcome         string
	SkipReason      string
	ProfileRevision string
	BeforeBytes     int
	AfterBytes      int
	BeforeTokens    int
	AfterTokens     int
	ChangedTargets  int
	EligibleTargets int
	AppliedFilters  []string
	Duration        time.Duration
	Targets         []Target
}

type Engine struct {
	config  Config
	filters []compiledFilter
	tokens  TokenCounter
}

type TokenCounter interface {
	Count(model, text string) (int, error)
	// ConservativeCount returns both common OpenAI encodings. Implementations
	// should return an error if either encoding cannot be computed.
	ConservativeCount(model, text string) (int, int, error)
}

// StaticTokenCounter is useful in tests and for applications that provide a
// model-specific tokenizer. The production default is TiktokenCounter.
type StaticTokenCounter struct{}

func (StaticTokenCounter) Count(_, text string) (int, error) { return len([]rune(text)), nil }
func (StaticTokenCounter) ConservativeCount(_, text string) (int, int, error) {
	n := len([]rune(text))
	return n, n, nil
}

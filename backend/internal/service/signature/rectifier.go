// Package signature defines the thinking-signature rectifier strategies used
// by the gateway retry loops when upstream rejects a request with a
// signature-related 400 error.
//
// Strategies:
//   - StripClaudeRectifier / StripAntigravityRectifier: the classic two-stage
//     behavior (drop thinking blocks, optionally drop tool blocks).
//   - (Phase 3) PoolClaudeRectifier / PoolAntigravityRectifier: replace the
//     invalid signatures with valid ones fetched from a Redis-backed pool.
//
// The interfaces are deliberately narrow so that a future factory can swap
// strategies at runtime based on RectifierSettings without touching the
// retry-loop scaffolding.
package signature

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// Stage enumerates the retry stages the gateway loops iterate over.
// Strip strategy uses StageThinkingOnly → StageThinkingAndTools.
// Pool strategy uses StageThinkingOnly only (one-shot).
type Stage int

const (
	StageThinkingOnly     Stage = 0 // drop thinking blocks / replace their signatures
	StageThinkingAndTools Stage = 1 // additionally drop tool blocks
)

// Name returns the stage label used in ops-event Kind and logs.
func (s Stage) Name() string {
	switch s {
	case StageThinkingOnly:
		return "thinking-only"
	case StageThinkingAndTools:
		return "thinking+tools"
	}
	return "unknown"
}

// ClaudeInput is the input to the Claude-native body rectifier.
// AccountType / AccountID are passed as primitives so the signature package
// does not import the service package (avoids an import cycle).
type ClaudeInput struct {
	AccountType string
	AccountID   int64
	Platform    string
	Body        []byte
	LastErrMsg  string // error message from the previous stage's 400 response; empty on first stage
}

// ClaudeRectifier transforms a Claude-format request body before a retry.
type ClaudeRectifier interface {
	// Apply returns the body to use for this retry stage.
	// proceed=false signals "skip this stage and abort the retry loop"
	//   - for Strip stage-2 this means the error did not look tool-related
	//   - for Pool this means the pool is empty or replacement produced no changes
	Apply(ctx context.Context, in ClaudeInput, stage Stage) (body []byte, proceed bool)
}

// AntigravityInput is the input to the Antigravity rectifier (mutates the
// intermediate Claude-format struct before it is re-transformed to Gemini).
type AntigravityInput struct {
	AccountType string
	AccountID   int64
	Platform    string
	Request     *antigravity.ClaudeRequest
	LastErrMsg  string
}

// AntigravityRectifier mutates a Claude request in-place for the next
// Antigravity retry.
type AntigravityRectifier interface {
	// Apply mutates the request for the given stage.
	// applied=false means the strip/replace found nothing to do — outer loop
	//   should skip this stage and try the next one (matches legacy behavior).
	// proceed=false means abort further stages entirely (e.g. Pool exhausted).
	Apply(ctx context.Context, in AntigravityInput, stage Stage) (applied bool, proceed bool, err error)

	// Stages returns the ordered list of stages this strategy wants the loop
	// to iterate. Strip returns [ThinkingOnly, ThinkingAndTools]; Pool
	// returns [ThinkingOnly] only.
	Stages() []Stage
}

// LooksLikeToolSignatureError is the predicate that gates Strip stage-2.
// Extracted from the anonymous closure previously inlined in gateway_service.go.
func LooksLikeToolSignatureError(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "tool_use") ||
		strings.Contains(m, "tool_result") ||
		strings.Contains(m, "functioncall") ||
		strings.Contains(m, "function_call") ||
		strings.Contains(m, "functionresponse") ||
		strings.Contains(m, "function_response")
}

// StripClaudeRectifier wraps the legacy FilterThinkingBlocksForRetry /
// FilterSignatureSensitiveBlocksForRetry functions as a ClaudeRectifier.
// The filter functions are injected to avoid an import cycle with the
// service package where they live.
type StripClaudeRectifier struct {
	FilterStage1 func([]byte) []byte // FilterThinkingBlocksForRetry
	FilterStage2 func([]byte) []byte // FilterSignatureSensitiveBlocksForRetry
}

// Apply implements ClaudeRectifier.
func (r *StripClaudeRectifier) Apply(_ context.Context, in ClaudeInput, stage Stage) ([]byte, bool) {
	switch stage {
	case StageThinkingOnly:
		return r.FilterStage1(in.Body), true
	case StageThinkingAndTools:
		if !LooksLikeToolSignatureError(in.LastErrMsg) {
			return nil, false
		}
		return r.FilterStage2(in.Body), true
	}
	return nil, false
}

// StripAntigravityRectifier wraps the legacy stripThinkingFromClaudeRequest /
// stripSignatureSensitiveBlocksFromClaudeRequest functions as an
// AntigravityRectifier.
type StripAntigravityRectifier struct {
	StripStage1 func(*antigravity.ClaudeRequest) (bool, error) // stripThinkingFromClaudeRequest
	StripStage2 func(*antigravity.ClaudeRequest) (bool, error) // stripSignatureSensitiveBlocksFromClaudeRequest
}

// Apply implements AntigravityRectifier.
func (r *StripAntigravityRectifier) Apply(_ context.Context, in AntigravityInput, stage Stage) (bool, bool, error) {
	switch stage {
	case StageThinkingOnly:
		applied, err := r.StripStage1(in.Request)
		return applied, true, err
	case StageThinkingAndTools:
		applied, err := r.StripStage2(in.Request)
		return applied, true, err
	}
	return false, false, nil
}

// Stages returns the two-stage order used by the legacy retry loop.
func (r *StripAntigravityRectifier) Stages() []Stage {
	return []Stage{StageThinkingOnly, StageThinkingAndTools}
}

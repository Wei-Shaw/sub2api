package apicompat

import "strings"

// ReasoningReplayPolicy is selected from the actual destination account, not the
// client-visible model. These legacy prefix checks are compatibility heuristics,
// NOT authenticated provenance or a guarantee of account/key compatibility.
// An upstream may still reject an allowed ciphertext; callers can then retry
// once without encrypted reasoning before committing any downstream output.
type ReasoningReplayPolicy uint8

const (
	ReasoningReplayDisabled ReasoningReplayPolicy = iota
	ReasoningReplayOpenAILegacy
	ReasoningReplayGrokLegacy
)

// AnthropicToResponsesOptions controls destination-specific request conversion.
// The zero value disables opaque reasoning replay and uses the default token floor.
type AnthropicToResponsesOptions struct {
	MinOutputTokens       int
	ReasoningReplayPolicy ReasoningReplayPolicy
}

func (p ReasoningReplayPolicy) allowsSignature(sig string) bool {
	if sig == "" || strings.HasPrefix(sig, "CAQS") || strings.HasPrefix(sig, anthropicThinkingEnvelopePrefix) {
		return false
	}
	switch p {
	case ReasoningReplayOpenAILegacy:
		// OpenAI's historical Fernet-style payloads, never native Claude or
		// arbitrary unknown signatures. This does not validate the ciphertext.
		return strings.HasPrefix(sig, "gAAAA")
	case ReasoningReplayGrokLegacy:
		// Preserve xAI's existing opaque replay behavior (including enc-rs-1
		// fixtures), while excluding known foreign OpenAI/Claude signatures.
		return !strings.HasPrefix(sig, "gAAAA")
	default:
		return false
	}
}

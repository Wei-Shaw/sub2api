package kiro

import "strings"

// modelMapping is one entry in the prefix-priority model map. The list is
// ordered: longer/more-specific prefixes appear first so we don't match
// "claude-sonnet-4" inside "claude-sonnet-4.5".
type modelMapping struct {
	prefix string
	target string
}

// modelMapOrdered translates inbound Anthropic-style model IDs into the
// short canonical names Kiro accepts (e.g. "claude-sonnet-4.6"). Ported
// from Kiro-Go/proxy/translator.go to keep behaviour identical.
//
// Callers should also consult the per-account / per-group model_mapping
// override (DefaultKiroModelMapping in domain/constants.go) before
// falling back to this list.
var modelMapOrdered = []modelMapping{
	{"claude-sonnet-4-20250514", "claude-sonnet-4"},
	{"claude-sonnet-4-5", "claude-sonnet-4.5"},
	{"claude-sonnet-4.5", "claude-sonnet-4.5"},
	{"claude-sonnet-4-6", "claude-sonnet-4.6"},
	{"claude-sonnet-4.6", "claude-sonnet-4.6"},
	{"claude-opus-4-7", "claude-opus-4.7"},
	{"claude-opus-4.7", "claude-opus-4.7"},
	{"claude-haiku-4-5", "claude-haiku-4.5"},
	{"claude-haiku-4.5", "claude-haiku-4.5"},
	{"claude-opus-4-6", "claude-opus-4.6"},
	{"claude-opus-4.6", "claude-opus-4.6"},
	{"claude-sonnet-4", "claude-sonnet-4"},
	// Backward-compat aliases for older model names.
	{"claude-3-5-sonnet", "claude-sonnet-4.5"},
	{"claude-3-opus", "claude-sonnet-4.5"},
	{"claude-3-sonnet", "claude-sonnet-4"},
	{"claude-3-haiku", "claude-haiku-4.5"},
}

// ThinkingSuffix is the conventional suffix that turns on extended
// thinking mode. Stripped from the model name and tracked separately.
const ThinkingSuffix = "-thinking"

// ThinkingModePrompt is prepended to the system prompt when thinking is
// requested. Mirrors Kiro-Go's behaviour exactly.
const ThinkingModePrompt = `<thinking_mode>enabled</thinking_mode>
<max_thinking_length>200000</max_thinking_length>`

// ParseModelAndThinking inspects a requested model id, strips the
// -thinking suffix if present, and returns the mapped Kiro model id
// plus a flag indicating whether thinking should be enabled. Unknown
// "claude-*" inputs are returned unchanged; non-Claude inputs likewise.
func ParseModelAndThinking(model, suffix string) (string, bool) {
	if suffix == "" {
		suffix = ThinkingSuffix
	}
	lower := strings.ToLower(model)
	thinking := false

	if strings.HasSuffix(lower, strings.ToLower(suffix)) {
		thinking = true
		model = model[:len(model)-len(suffix)]
		lower = strings.ToLower(model)
	}

	for _, m := range modelMapOrdered {
		if strings.Contains(lower, m.prefix) {
			return m.target, thinking
		}
	}

	// Pass-through for any other claude-* identifier (forward-compat for
	// model IDs that we haven't explicitly mapped yet).
	if strings.HasPrefix(lower, "claude-") {
		return model, thinking
	}
	return model, thinking
}

// MapModel is a convenience wrapper that returns only the mapped model id.
func MapModel(model string) string {
	out, _ := ParseModelAndThinking(model, ThinkingSuffix)
	return out
}

package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// NormalizeAdaptiveThinkingForMappedModel fixes the thinking config of an
// already converted Anthropic request when a model mapping turns the
// client-requested model into an adaptive-only model. The CC/Responses →
// Anthropic converters build thinking.type="enabled" for ordinary Claude models;
// adaptive-only models (Fable 5, Opus 4.7+, Sonnet 5+) reject "enabled" and a
// manual budget_tokens with a 400 and require "adaptive" instead.
func NormalizeAdaptiveThinkingForMappedModel(req *apicompat.AnthropicRequest, mappedModel string) {
	if req == nil || req.Thinking == nil {
		return
	}
	if !claude.RequiresAdaptiveThinking(mappedModel) || req.Thinking.Type != "enabled" {
		return
	}
	req.Thinking = &apicompat.AnthropicThinking{Type: "adaptive"}
}

// NormalizeAdaptiveOnlyThinking rewrites the legacy thinking.type="enabled"
// form in a native Anthropic Messages body for models that only accept
// adaptive thinking. Some clients (for example MiMo-Code) only special-case
// older model IDs and still send "enabled" + budget_tokens to newer models,
// which the upstream rejects with
// "thinking.type.enabled is not supported for this model".
//
// The rewrite is model-gated so bodies for models that still accept "enabled"
// stay byte-identical (important for the API-key passthrough path).
func NormalizeAdaptiveOnlyThinking(body []byte, mappedModel string) ([]byte, bool) {
	if !claude.RequiresAdaptiveThinking(mappedModel) {
		return body, false
	}
	if gjson.GetBytes(body, "thinking.type").String() != "enabled" {
		return body, false
	}
	rewritten, err := sjson.SetBytes(body, "thinking.type", "adaptive")
	if err != nil {
		return body, false
	}
	if withoutBudget, err := sjson.DeleteBytes(rewritten, "thinking.budget_tokens"); err == nil {
		rewritten = withoutBudget
	}
	return rewritten, true
}

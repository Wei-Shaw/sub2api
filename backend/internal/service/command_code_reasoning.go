package service

import (
	"net/url"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// normalizeCommandCodeChatReasoningEffort preserves "thinking disabled" on the
// Command Code Provider API. Its DeepSeek models spell disable
// reasoning_effort "off"; a client's "none" is refused as an unknown value and
// the whole request fails with 400.
//
// Scope is deliberately narrow, because "none" is valid for every other member
// of a mixed priority pool (OpenCode Go accepts it, official OpenAI accepts it):
//
//   - only the official Command Code host and its Chat Completions path are
//     touched, so Go / official / third-party base URLs are never rewritten
//     even when they carry an identical model name;
//   - only the models verified to accept "off" are touched. Notably
//     deepseek/deepseek-v4-flash-fast refuses "off" too, so it stays verbatim;
//   - only "none" is touched. Every other level (low/medium/high/max/...) is
//     forwarded unchanged, and no global ceiling or mapping is applied.
//
// It runs at the shared Chat Completions send boundary, after model mapping and
// group policy, so native Chat Completions and the Responses→Chat and
// Anthropic→Chat bridges all get the same wire adaptation from one place.
//
// The rewrite applies to the outbound copy only: sjson returns a new slice and
// the caller's body stays "none", so an account retry or failover to another
// pool member cannot inherit Command Code's vocabulary. Usage and billing also
// keep the canonical "none", which is the disable bucket the reasoning-effort
// multipliers are keyed on.
//
// Not fixable by group reasoning_effort_mappings: those match group+model, not
// the selected account, so a rule would rewrite Go and official traffic too.
// Nor does the pool fail over on its own — shouldFailoverOpenAIUpstreamResponse
// treats upstream 400 as a deterministic client error.
//
// Evidence: https://github.com/magpie-community/plugins/issues/334 (GOAT key:
// 0 reasoning tokens with "off" on each model below; "none" refused).
func normalizeCommandCodeChatReasoningEffort(targetURL string, body []byte) ([]byte, error) {
	if len(body) == 0 || !isOfficialCommandCodeHost(targetURL) {
		return body, nil
	}
	parsed, err := url.Parse(targetURL)
	if err != nil || !strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/chat/completions") {
		return body, nil
	}
	if !commandCodeReasonsWithOff(gjson.GetBytes(body, "model").String()) {
		return body, nil
	}
	effort := gjson.GetBytes(body, "reasoning_effort")
	if effort.Type != gjson.String || !strings.EqualFold(strings.TrimSpace(effort.String()), "none") {
		return body, nil
	}
	return sjson.SetBytes(body, "reasoning_effort", "off")
}

// commandCodeReasoningOffModels lists the Command Code upstream models whose
// Provider API turns thinking off at reasoning_effort "off". Keys are bare
// model names; the wire name carries a vendor prefix
// ("deepseek/deepseek-v4.1-flash") that is matched separately.
var commandCodeReasoningOffModels = map[string]struct{}{
	"deepseek-v4-pro":              {},
	"deepseek-v4-flash":            {},
	"deepseek-v4-flash-vision-exp": {},
	"deepseek-v4.1-flash":          {},
	"deepseek-v4.1-flash-fast":     {},
}

// commandCodeReasonsWithOff reports whether model is one of the Command Code
// models that accepts reasoning_effort "off". A vendor prefix other than
// deepseek never matches, and deepseek/deepseek-v4-flash-fast is intentionally
// absent because that model rejects "off".
func commandCodeReasonsWithOff(model string) bool {
	name := strings.ToLower(strings.TrimSpace(model))
	if name == "" {
		return false
	}
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		if name[:idx] != "deepseek" {
			return false
		}
		name = name[idx+1:]
	}
	_, ok := commandCodeReasoningOffModels[name]
	return ok
}

package basispoints

import (
	"strings"

	"github.com/tidwall/gjson"
)

// NativeFallbackReason reports declarations requiring the native Codex channel
// under the default fallback policy. This describes the bridge's capabilities,
// not whether the BPS upstream could support them through another protocol.
func NativeFallbackReason(body []byte) string {
	if !gjson.ValidBytes(body) {
		return ""
	}
	choice := gjson.GetBytes(body, "tool_choice")
	if choice.Type == gjson.String && choice.String() == "none" {
		return ""
	}
	switch choice.Get("type").String() {
	case "web_search", "web_search_preview", "web_search_preview_2025_03_11", "web_search_2025_08_26", "image_generation":
		return "tool_choice"
	}
	if tools := gjson.GetBytes(body, "tools"); tools.IsArray() {
		fallback := ""
		tools.ForEach(func(_, tool gjson.Result) bool {
			kind := strings.ToLower(strings.TrimSpace(tool.Get("type").String()))
			switch kind {
			case "image_generation":
				fallback = "image_generation"
				return false
			case "web_search", "web_search_preview", "web_search_preview_2025_03_11", "web_search_2025_08_26":
				if tool.Get("external_web_access").Bool() || tool.Get("search_context_size").String() == "high" {
					fallback = "web_search"
					return false
				}
			}
			return true
		})
		if fallback != "" {
			return fallback
		}
	}
	// Inline data images are handled by Sub2API's local relay before Prepare;
	// leave them on BPS so the relay can rewrite them to signed HTTPS URLs.
	return ""
}

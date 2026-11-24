package service

import (
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

// Codex delivers every additional context source (browser_tabs, terminal_info,
// codex_apps_open_page, ...) as a function_call_output in the reserved "agent"
// namespace without call_id. The name comes from an opaque client key: ASCII
// letters, digits and underscores, at most 128 characters
// (openai/codex codex-rs/context-fragments/src/additional_context.rs).
//
// Such an item is model input, not the result of a model-issued call: it never
// pairs with a function_call and must not count toward tool-continuation checks.
// Codex sends it as a tool output on purpose so the model treats ambient state
// (for example web page content) as untrusted data rather than user
// instructions; it is forwarded unchanged to Responses upstreams.
var codexAgentContextNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,128}$`)

const codexAgentContextNamespace = "agent"

func isCodexAgentContextOutput(item gjson.Result) bool {
	if item.Get("type").String() != "function_call_output" || item.Get("namespace").String() != codexAgentContextNamespace {
		return false
	}
	if callID := item.Get("call_id"); callID.Exists() && (callID.Type != gjson.String || strings.TrimSpace(callID.String()) != "") {
		return false
	}
	name := item.Get("name")
	return name.Type == gjson.String && codexAgentContextNamePattern.MatchString(name.String()) && item.Get("output").Type == gjson.String
}

func isCodexAgentContextOutputMap(item map[string]any) bool {
	if typ, _ := item["type"].(string); typ != "function_call_output" {
		return false
	}
	if namespace, _ := item["namespace"].(string); namespace != codexAgentContextNamespace {
		return false
	}
	if callID, exists := item["call_id"]; exists {
		value, ok := callID.(string)
		if !ok || strings.TrimSpace(value) != "" {
			return false
		}
	}
	name, _ := item["name"].(string)
	_, outputIsText := item["output"].(string)
	return codexAgentContextNamePattern.MatchString(name) && outputIsText
}

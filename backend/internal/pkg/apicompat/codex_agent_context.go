package apicompat

import (
	"regexp"
	"strings"
)

// Codex sends ambient context (browser_tabs, terminal_info, ...) as a
// function_call_output in the reserved "agent" namespace without call_id; the
// name is derived from an opaque client key. Responses upstreams take it
// natively. Chat Completions and Anthropic have no standalone tool output, so
// the converters render it the way Codex itself does when tool-output context
// is disabled: a user text <external_{name}>{output}</external_{name}>.
var codexAgentContextNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,128}$`)

func isCodexAgentContextOutput(itemType, namespace, callID, name string) bool {
	return itemType == "function_call_output" && namespace == "agent" &&
		strings.TrimSpace(callID) == "" && codexAgentContextNamePattern.MatchString(name)
}

func codexAgentContextText(name, output string) string {
	return "<external_" + name + ">" + output + "</external_" + name + ">"
}

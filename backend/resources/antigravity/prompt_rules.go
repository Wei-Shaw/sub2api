package antigravity

import _ "embed"

//go:embed prompt-sanitize-rules.json
var promptSanitizeRules string

// PromptSanitizeRules returns the bundled system prompt replacement rules as JSON.
func PromptSanitizeRules() string {
	return promptSanitizeRules
}

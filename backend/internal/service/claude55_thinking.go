package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// adaptThinkingToClaude55 replaces a thinking mode that a Claude 5.5 model
// rejects with the nearest mode it accepts: enabled becomes adaptive without
// budget_tokens, and on Sonnet 5.5 disabled becomes between_tools. Other modes
// are left for validateClaude55Request to judge.
func adaptThinkingToClaude55(body []byte, model string) []byte {
	if !isClaude55SignedThinkingModel(model) {
		return body
	}
	switch gjson.GetBytes(body, "thinking.type").String() {
	case "enabled":
		body, _ = sjson.SetBytes(body, "thinking.type", "adaptive")
		body, _ = sjson.DeleteBytes(body, "thinking.budget_tokens")
	case "disabled":
		if claude.IsSonnet55(model) {
			body, _ = sjson.SetBytes(body, "thinking.type", "between_tools")
		}
	}
	return body
}

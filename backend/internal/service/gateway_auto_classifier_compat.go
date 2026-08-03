package service

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// prepareClaudeAutoClassifierOAuthBody upgrades the legacy classifier request
// emitted by Claude Code when ANTHROPIC_BASE_URL points at a relay to the wire
// contract used by Claude Code's first-party subscription path. It deliberately
// preserves the classifier prompt, session context, messages, and cache breaks.
func prepareClaudeAutoClassifierOAuthBody(body []byte) ([]byte, bool, error) {
	if !isClaudeCodeAutoClassifierBody(body) {
		return body, false, nil
	}

	next, err := sjson.SetBytes(body, "model", claude.AutoModeClassifierModel)
	if err != nil {
		return nil, false, fmt.Errorf("set Claude Auto classifier model: %w", err)
	}
	if systemHasBillingAttributionBlock(next) {
		return next, true, nil
	}

	billingText, err := buildBillingAttributionText(next, claude.CLICurrentVersion)
	if err != nil {
		return nil, false, fmt.Errorf("build Claude Auto classifier billing block: %w", err)
	}
	billingBlock, err := marshalAnthropicSystemTextBlock(billingText, false)
	if err != nil {
		return nil, false, fmt.Errorf("marshal Claude Auto classifier billing block: %w", err)
	}

	system := gjson.GetBytes(next, "system")
	if !system.IsArray() {
		return nil, false, fmt.Errorf("Claude Auto classifier system must be an array")
	}
	items := make([][]byte, 0, len(system.Array())+1)
	items = append(items, billingBlock)
	system.ForEach(func(_, item gjson.Result) bool {
		items = append(items, []byte(item.Raw))
		return true
	})

	next, ok := setJSONRawBytes(next, "system", buildJSONArrayRaw(items))
	if !ok {
		return nil, false, fmt.Errorf("prepend Claude Auto classifier billing block")
	}
	return next, true, nil
}

func isClaudeCodeAutoClassifierBody(body []byte) bool {
	system := gjson.GetBytes(body, "system")
	if !system.IsArray() {
		return false
	}
	found := false
	system.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "text" &&
			isClaudeCodeSecurityMonitorPromptText(item.Get("text").String()) {
			found = true
			return false
		}
		return true
	})
	return found
}

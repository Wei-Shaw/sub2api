package service

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const claudeAutoClassifierCompatKey = "claudeAutoClassifierCompat"

// prepareClaudeAutoClassifierOAuthBody upgrades the legacy classifier request
// emitted by Claude Code when ANTHROPIC_BASE_URL points at a relay to the wire
// contract used by Claude Code's first-party subscription path. It deliberately
// preserves the classifier prompt, session context, messages, and cache breaks.
func prepareClaudeAutoClassifierOAuthBody(body []byte, model string) ([]byte, bool, error) {
	if !isClaudeCodeAutoClassifierBody(body) {
		return body, false, nil
	}

	next, err := sjson.SetBytes(body, "model", model)
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
		return nil, false, fmt.Errorf("auto classifier system must be an array")
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

// resolveClaudeAutoClassifierOAuthModel returns the final allowed upstream
// model for the compatibility request. A copied classifier prompt alone is not
// sufficient: the handler must have verified the request as Claude Code, the
// account must use the Anthropic OAuth wire path, and the forced model must pass
// the same channel and account-model gates used during scheduling.
func (s *GatewayService) resolveClaudeAutoClassifierOAuthModel(
	ctx context.Context,
	account *Account,
	groupID *int64,
	body []byte,
) (string, bool) {
	if account == nil || account.Platform != PlatformAnthropic ||
		(account.Type != AccountTypeOAuth && account.Type != AccountTypeSetupToken) ||
		!IsClaudeCodeClient(ctx) || !isClaudeCodeAutoClassifierBody(body) {
		return "", false
	}

	model := claude.AutoModeClassifierModel
	if s.checkChannelPricingRestriction(ctx, groupID, model) {
		return "", false
	}
	if groupID != nil {
		mapping := s.ResolveChannelMapping(ctx, *groupID, model)
		if mapping.Mapped {
			model = mapping.MappedModel
		}
	}
	if !s.isModelSupportedByAccountWithContext(ctx, account, model) {
		return "", false
	}
	if !s.isAccountSchedulableForModelSelection(ctx, account, model) {
		return "", false
	}
	if groupID != nil && s.needsUpstreamChannelRestrictionCheck(ctx, groupID) &&
		s.isUpstreamModelRestrictedByChannel(ctx, *groupID, account, model) {
		return "", false
	}
	if mapped, matched := account.ResolveMappedModel(model); matched {
		model = mapped
	}
	model = claude.NormalizeModelID(model)
	return model, model != ""
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

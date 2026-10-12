package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

func responsesToAnthropicForAccount(req *apicompat.ResponsesRequest, account *Account) (*apicompat.AnthropicRequest, error) {
	// Claude 5.x and later support inline system messages per Anthropic API specification.
	// Earlier versions (3.x, 4.x) require system content to be hoisted to the top-level parameter.
	// Reference: https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#mid-conversation-system-messages
	//
	// Individual accounts can opt out via account.Extra["disable_mid_conversation_system"] if needed.
	supported := claude.SupportsMidConversationSystem(req.Model)
	disabled, _ := account.Extra["disable_mid_conversation_system"].(bool)

	return apicompat.ResponsesToAnthropicRequestWithOptions(req, apicompat.ResponsesAnthropicOptions{
		MidConversationSystem: account.Type == AccountTypeAPIKey && supported && !disabled,
	})
}

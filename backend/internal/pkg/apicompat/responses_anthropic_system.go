package apicompat

import "fmt"

// placeAnthropicSystemMessages preserves instruction priority and the assistant
// turn at which an instruction takes effect. Anthropic accepts content-bearing
// system turns only after user/tool results, before assistant or end of input.
// Delay instructions past intervening user input to that boundary, never past an
// assistant response. Run after tool pairing so instructions cannot split a call
// from its result. No text is stripped and no instruction becomes user content.
func placeAnthropicSystemMessages(messages []AnthropicMessage) ([]AnthropicMessage, error) {
	out := make([]AnthropicMessage, 0, len(messages))
	var pending []AnthropicContentBlock
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		if len(out) == 0 || out[len(out)-1].Role != "user" {
			return fmt.Errorf("mid-conversation system instruction requires user input or tool results before the next assistant turn; disable_mid_conversation_system can restore legacy hoisting for this provider")
		}
		out = append(out, anthropicMessageFromBlocks("system", pending))
		pending = nil
		return nil
	}
	for _, message := range messages {
		switch message.Role {
		case "system":
			pending = append(pending, parseContentBlocks(message.Content)...)
		case "assistant":
			if err := flush(); err != nil {
				return nil, err
			}
			out = append(out, message)
		default:
			out = append(out, message)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}

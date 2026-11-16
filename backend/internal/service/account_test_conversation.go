package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// Playground conversations ride the admin account-test endpoint: the request
// may replace the probe's fixed "hi" with a multi-turn conversation and a few
// sampling parameters. The limits below bound what an admin can push through a
// single test request.
const (
	accountTestConversationMaxMessages     = 100
	accountTestConversationMaxContentBytes = 512 << 10
	accountTestConversationMaxSystemBytes  = 64 << 10
	accountTestConversationMaxTokensLimit  = 128000

	accountTestConversationContextKey = "account_test_conversation"

	accountTestRoleUser      = "user"
	accountTestRoleAssistant = "assistant"

	// AccountTestConversationNotAppliedNotice is sent as a status event when the
	// tested account's probe path cannot carry a conversation.
	AccountTestConversationNotAppliedNotice = "This account type does not support multi-turn conversations here: only the latest message was sent, without the system prompt or parameters."
)

// AccountTestMessage is one turn of a playground conversation.
type AccountTestMessage struct {
	Role    string
	Content string
}

// AccountTestConversation replaces the fixed probe prompt of an account test.
// MaxTokens == 0 and Temperature == nil keep each protocol's probe default.
type AccountTestConversation struct {
	Messages    []AccountTestMessage
	System      string
	MaxTokens   int
	Temperature *float64

	// applied records that a payload builder consumed the conversation.
	applied bool
}

// Validate checks the shape and size of a conversation before anything is sent
// upstream.
func (conv *AccountTestConversation) Validate() error {
	if conv == nil {
		return nil
	}
	if len(conv.Messages) == 0 {
		return errors.New("messages must not be empty")
	}
	if len(conv.Messages) > accountTestConversationMaxMessages {
		return fmt.Errorf("too many messages (max %d)", accountTestConversationMaxMessages)
	}
	total := 0
	for i, message := range conv.Messages {
		if message.Role != accountTestRoleUser && message.Role != accountTestRoleAssistant {
			return fmt.Errorf("messages[%d]: role must be %q or %q", i, accountTestRoleUser, accountTestRoleAssistant)
		}
		if strings.TrimSpace(message.Content) == "" {
			return fmt.Errorf("messages[%d]: content must not be empty", i)
		}
		if !utf8.ValidString(message.Content) {
			return fmt.Errorf("messages[%d]: content must be valid UTF-8", i)
		}
		total += len(message.Content)
	}
	if total > accountTestConversationMaxContentBytes {
		return fmt.Errorf("conversation is too large (max %d bytes)", accountTestConversationMaxContentBytes)
	}
	if conv.Messages[0].Role != accountTestRoleUser {
		return errors.New("the first message must come from the user")
	}
	if conv.Messages[len(conv.Messages)-1].Role != accountTestRoleUser {
		return errors.New("the last message must come from the user")
	}
	if len(conv.System) > accountTestConversationMaxSystemBytes {
		return fmt.Errorf("system prompt is too large (max %d bytes)", accountTestConversationMaxSystemBytes)
	}
	if !utf8.ValidString(conv.System) {
		return errors.New("system prompt must be valid UTF-8")
	}
	if conv.MaxTokens < 0 || conv.MaxTokens > accountTestConversationMaxTokensLimit {
		return fmt.Errorf("max_tokens must be between 0 and %d", accountTestConversationMaxTokensLimit)
	}
	if conv.Temperature != nil && (*conv.Temperature < 0 || *conv.Temperature > 2) {
		return errors.New("temperature must be between 0 and 2")
	}
	return nil
}

// lastUserMessage is what probe paths without conversation support receive as
// their single prompt.
func (conv *AccountTestConversation) lastUserMessage() string {
	if conv == nil || len(conv.Messages) == 0 {
		return ""
	}
	return conv.Messages[len(conv.Messages)-1].Content
}

func (conv *AccountTestConversation) systemPrompt() string {
	if conv == nil {
		return ""
	}
	return strings.TrimSpace(conv.System)
}

func bindAccountTestConversation(c *gin.Context, conv *AccountTestConversation) {
	if c == nil || conv == nil {
		return
	}
	c.Set(accountTestConversationContextKey, conv)
}

func accountTestConversationFrom(c *gin.Context) *AccountTestConversation {
	if c == nil {
		return nil
	}
	value, ok := c.Get(accountTestConversationContextKey)
	if !ok {
		return nil
	}
	conv, _ := value.(*AccountTestConversation)
	return conv
}

// accountTestConversationSkipped reports a bound conversation that no payload
// builder consumed, i.e. the probe path only saw the latest message.
func accountTestConversationSkipped(c *gin.Context) bool {
	conv := accountTestConversationFrom(c)
	return conv != nil && !conv.applied
}

// applyAnthropicTestConversation rewrites a Messages API probe payload (also the
// Bedrock and Vertex variants). The probe's own system block stays first: OAuth
// accounts require it, so a playground system prompt is appended after it.
func applyAnthropicTestConversation(c *gin.Context, payload map[string]any) {
	conv := accountTestConversationFrom(c)
	if conv == nil || payload == nil {
		return
	}
	messages := make([]map[string]any, 0, len(conv.Messages))
	for _, message := range conv.Messages {
		messages = append(messages, map[string]any{
			"role": message.Role,
			"content": []map[string]any{
				{"type": "text", "text": message.Content},
			},
		})
	}
	payload["messages"] = messages

	if system := conv.systemPrompt(); system != "" {
		block := map[string]any{"type": "text", "text": system}
		switch existing := payload["system"].(type) {
		case []map[string]any:
			payload["system"] = append(append([]map[string]any(nil), existing...), block)
		default:
			payload["system"] = []map[string]any{block}
		}
	}
	if conv.MaxTokens > 0 {
		payload["max_tokens"] = conv.MaxTokens
	}
	if conv.Temperature != nil {
		payload["temperature"] = *conv.Temperature
	}
	conv.applied = true
}

// applyResponsesTestConversation rewrites a Responses API probe payload.
// systemRole is the input role that carries the system prompt ("developer" on
// OpenAI, "system" on other Responses-compatible upstreams). allowSampling is
// false for ChatGPT OAuth accounts, whose upstream rejects max_output_tokens
// and temperature.
func applyResponsesTestConversation(c *gin.Context, payload map[string]any, systemRole string, allowSampling bool) {
	conv := accountTestConversationFrom(c)
	if conv == nil || payload == nil {
		return
	}
	input := make([]map[string]any, 0, len(conv.Messages)+1)
	if system := conv.systemPrompt(); system != "" {
		input = append(input, map[string]any{
			"role": systemRole,
			"content": []map[string]any{
				{"type": "input_text", "text": system},
			},
		})
	}
	for _, message := range conv.Messages {
		contentType := "input_text"
		if message.Role == accountTestRoleAssistant {
			contentType = "output_text"
		}
		input = append(input, map[string]any{
			"role": message.Role,
			"content": []map[string]any{
				{"type": contentType, "text": message.Content},
			},
		})
	}
	payload["input"] = input

	if allowSampling {
		if conv.MaxTokens > 0 {
			payload["max_output_tokens"] = conv.MaxTokens
		}
		if conv.Temperature != nil {
			payload["temperature"] = *conv.Temperature
		}
	}
	conv.applied = true
}

// applyChatCompletionsTestConversation rewrites a Chat Completions probe payload.
func applyChatCompletionsTestConversation(c *gin.Context, payload map[string]any) {
	conv := accountTestConversationFrom(c)
	if conv == nil || payload == nil {
		return
	}
	messages := make([]map[string]any, 0, len(conv.Messages)+1)
	if system := conv.systemPrompt(); system != "" {
		messages = append(messages, map[string]any{"role": "system", "content": system})
	}
	for _, message := range conv.Messages {
		messages = append(messages, map[string]any{"role": message.Role, "content": message.Content})
	}
	payload["messages"] = messages

	if conv.MaxTokens > 0 {
		payload["max_tokens"] = conv.MaxTokens
	}
	if conv.Temperature != nil {
		payload["temperature"] = *conv.Temperature
	}
	conv.applied = true
}

// applyGeminiTestConversation rewrites a generateContent probe payload. Image
// generation probes keep their single prompt and are returned unchanged.
func applyGeminiTestConversation(c *gin.Context, modelID string, payload []byte) []byte {
	conv := accountTestConversationFrom(c)
	if conv == nil || isImageGenerationModel(modelID) {
		return payload
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return payload
	}

	contents := make([]map[string]any, 0, len(conv.Messages))
	for _, message := range conv.Messages {
		role := "user"
		if message.Role == accountTestRoleAssistant {
			role = "model"
		}
		contents = append(contents, map[string]any{
			"role":  role,
			"parts": []map[string]any{{"text": message.Content}},
		})
	}
	body["contents"] = contents

	if system := conv.systemPrompt(); system != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": system}},
		}
	}
	if conv.MaxTokens > 0 || conv.Temperature != nil {
		generationConfig, _ := body["generationConfig"].(map[string]any)
		if generationConfig == nil {
			generationConfig = map[string]any{}
		}
		if conv.MaxTokens > 0 {
			generationConfig["maxOutputTokens"] = conv.MaxTokens
		}
		if conv.Temperature != nil {
			generationConfig["temperature"] = *conv.Temperature
		}
		body["generationConfig"] = generationConfig
	}

	rewritten, err := json.Marshal(body)
	if err != nil {
		return payload
	}
	conv.applied = true
	return rewritten
}

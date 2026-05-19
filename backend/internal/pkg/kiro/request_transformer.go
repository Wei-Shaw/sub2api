package kiro

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// AnthropicRequest is the inbound Anthropic Messages API request shape.
// Only the fields the transformer needs are extracted; the original body
// can still be JSON-decoded into a full structure elsewhere if needed.
type AnthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []AnthropicMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
	TopP        float64            `json:"top_p,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
	System      any                `json:"system,omitempty"` // string or []block
	Thinking    *AnthropicThinking `json:"thinking,omitempty"`
	Tools       []AnthropicTool    `json:"tools,omitempty"`
}

// AnthropicThinking is the extended-thinking config block.
type AnthropicThinking struct {
	Type         string `json:"type,omitempty"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// AnthropicMessage is one message in the conversation. Content is
// untyped because Anthropic allows either a string or a list of content
// blocks; the transformer normalises that on the way through.
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// AnthropicTool defines a tool the assistant can call.
type AnthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

// minimalFallbackUserContent is the body we send when the inbound has no
// user-visible text (e.g. tool-result-only continuation). Kiro rejects
// empty content strings.
const minimalFallbackUserContent = "."

const toolResultsContinuationPrefix = "Tool results:"

// TransformAnthropicRequest converts an inbound Anthropic Messages
// request into the Kiro Payload shape. profileARN is the per-account
// CodeWhisperer profile ARN; thinking forces thinking mode on (the
// transformer also honours a `-thinking` suffix on the model id).
//
// Returns the payload plus a tool-name map so the response transformer
// can restore original names in tool_use events.
func TransformAnthropicRequest(req *AnthropicRequest, profileARN string) (*Payload, error) {
	if req == nil {
		return nil, nil
	}

	modelID, thinking := ParseModelAndThinking(req.Model, ThinkingSuffix)
	if isThinkingRequested(req.Thinking) {
		thinking = true
	}

	systemPrompt := buildSystemPrompt(req.System, thinking)
	origin := "AI_EDITOR"

	// Walk the messages, splitting off the trailing user turn (which
	// becomes currentMessage) from everything else (which becomes history).
	history := make([]HistoryMessage, 0, len(req.Messages))
	var currentContent string
	var currentImages []Image
	var currentToolResults []ToolResult

	for i, msg := range req.Messages {
		isLast := i == len(req.Messages)-1
		switch msg.Role {
		case "user":
			content, images, toolResults := extractAnthropicUserContent(msg.Content)
			content = normalizeUserContent(content, len(images) > 0)

			if isLast {
				currentContent = content
				currentImages = images
				currentToolResults = toolResults
				continue
			}

			userMsg := UserInputMessage{
				Content: content,
				ModelID: modelID,
				Origin:  origin,
			}
			if len(images) > 0 {
				userMsg.Images = images
			}
			if len(toolResults) > 0 {
				userMsg.UserInputMessageContext = &UserInputMessageContext{ToolResults: toolResults}
			}
			history = append(history, HistoryMessage{UserInputMessage: &userMsg})

		case "assistant":
			content, toolUses := extractAnthropicAssistantContent(msg.Content)
			history = append(history, HistoryMessage{
				AssistantResponseMessage: &AssistantResponseMessage{
					Content:  content,
					ToolUses: toolUses,
				},
			})
		}
	}

	history = trimLeadingAssistantHistory(history)

	finalContent := ""
	if systemPrompt != "" {
		finalContent = "--- SYSTEM PROMPT ---\n" + systemPrompt + "\n--- END SYSTEM PROMPT ---\n\n"
	}
	switch {
	case currentContent != "":
		finalContent += currentContent
	case len(currentImages) > 0:
		finalContent += normalizeUserContent("", true)
	case len(currentToolResults) > 0:
		finalContent += buildToolResultsContinuation(currentToolResults)
	default:
		finalContent += minimalFallbackUserContent
	}

	kiroTools, toolNameMap := convertAnthropicTools(req.Tools)

	p := &Payload{}
	p.ToolNameMap = toolNameMap
	p.ConversationState.ChatTriggerType = "MANUAL"
	p.ConversationState.AgentTaskType = "vibe"
	p.ConversationState.AgentContinuationID = uuid.New().String()
	p.ConversationState.ConversationID = buildConversationID(modelID, systemPrompt, firstAnthropicConversationAnchor(req.Messages))
	p.ConversationState.CurrentMessage.UserInputMessage = UserInputMessage{
		Content: finalContent,
		ModelID: modelID,
		Origin:  origin,
		Images:  currentImages,
	}

	if len(kiroTools) > 0 || len(currentToolResults) > 0 {
		p.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext = &UserInputMessageContext{
			Tools:       kiroTools,
			ToolResults: currentToolResults,
		}
	}

	if len(history) > 0 {
		p.ConversationState.History = history
	}

	if req.MaxTokens > 0 || req.Temperature > 0 || req.TopP > 0 {
		p.InferenceConfig = &InferenceConfig{
			MaxTokens:   req.MaxTokens,
			Temperature: req.Temperature,
			TopP:        req.TopP,
		}
	}

	p.ProfileARN = profileARN
	return p, nil
}

func isThinkingRequested(t *AnthropicThinking) bool {
	if t == nil {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(t.Type))
	return kind == "enabled" || kind == "adaptive"
}

func buildSystemPrompt(system any, thinking bool) string {
	prompt := strings.TrimSpace(extractSystemString(system))
	if !thinking {
		return prompt
	}
	if prompt == "" {
		return ThinkingModePrompt
	}
	return ThinkingModePrompt + "\n\n" + prompt
}

func extractSystemString(system any) string {
	switch v := system.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		var parts []string
		for _, b := range v {
			if block, ok := b.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// normalizeUserContent ensures the message body is non-empty when an
// image is present (Kiro requires a text body alongside images).
func normalizeUserContent(content string, hasImages bool) string {
	if content == "" && hasImages {
		return "Please analyze the attached image."
	}
	return content
}

// extractAnthropicUserContent walks the user message content (string or
// list of blocks) and returns text, attached images, and tool results.
func extractAnthropicUserContent(content any) (string, []Image, []ToolResult) {
	if s, ok := content.(string); ok {
		return s, nil, nil
	}

	var text strings.Builder
	var images []Image
	var toolResults []ToolResult

	blocks, ok := content.([]any)
	if !ok {
		return "", nil, nil
	}
	for _, b := range blocks {
		block, ok := b.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := block["type"].(string)
		switch blockType {
		case "text", "input_text":
			if t, ok := block["text"].(string); ok {
				text.WriteString(t)
			}
		case "image", "input_image":
			if img := extractImageFromAnthropicBlock(block); img != nil {
				images = append(images, *img)
			}
		case "tool_result":
			toolUseID, _ := block["tool_use_id"].(string)
			toolResults = append(toolResults, ToolResult{
				ToolUseID: toolUseID,
				Content:   []ResultText{{Text: extractToolResultContent(block["content"])}},
				Status:    "success",
			})
		}
	}
	return text.String(), images, toolResults
}

func extractImageFromAnthropicBlock(block map[string]any) *Image {
	source, ok := block["source"].(map[string]any)
	if !ok {
		return nil
	}
	data, _ := source["data"].(string)
	if data == "" {
		return nil
	}
	mediaType, _ := source["media_type"].(string)
	if mediaType == "" {
		mediaType, _ = source["mediaType"].(string)
	}
	format := strings.TrimPrefix(strings.ToLower(mediaType), "image/")
	if format == "" {
		format = "png"
	}
	return &Image{Format: format, Source: ImageSource{Bytes: data}}
}

func extractToolResultContent(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	if blocks, ok := content.([]any); ok {
		var parts []string
		for _, b := range blocks {
			if block, ok := b.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}

// extractAnthropicAssistantContent collects assistant text + tool_use
// blocks from a prior turn.
func extractAnthropicAssistantContent(content any) (string, []ToolUse) {
	if s, ok := content.(string); ok {
		return s, nil
	}
	var text strings.Builder
	var toolUses []ToolUse
	blocks, ok := content.([]any)
	if !ok {
		return "", nil
	}
	for _, b := range blocks {
		block, ok := b.(map[string]any)
		if !ok {
			continue
		}
		switch block["type"] {
		case "text":
			if t, ok := block["text"].(string); ok {
				text.WriteString(t)
			}
		case "tool_use":
			id, _ := block["id"].(string)
			name, _ := block["name"].(string)
			input, _ := block["input"].(map[string]any)
			if input == nil {
				input = map[string]any{}
			}
			toolUses = append(toolUses, ToolUse{ToolUseID: id, Name: name, Input: input})
		}
	}
	return text.String(), toolUses
}

// convertAnthropicTools wraps Anthropic tools in Kiro's ToolWrapper shape,
// sanitizing names and capturing a reverse map.
func convertAnthropicTools(tools []AnthropicTool) ([]ToolWrapper, map[string]string) {
	if len(tools) == 0 {
		return nil, nil
	}
	wrapped := make([]ToolWrapper, 0, len(tools))
	nameMap := make(map[string]string)
	for _, tool := range tools {
		sanitized := ShortenToolName(SanitizeToolName(tool.Name))
		if sanitized != tool.Name {
			nameMap[sanitized] = tool.Name
		}
		wrapped = append(wrapped, ToolWrapper{
			ToolSpecification: ToolSpecification{
				Name:        sanitized,
				Description: TruncateToolDescription(tool.Description),
				InputSchema: InputSchema{JSON: EnsureObjectSchema(tool.InputSchema)},
			},
		})
	}
	return wrapped, nameMap
}

// trimLeadingAssistantHistory removes assistant messages at the start of
// history — Kiro rejects a history that doesn't begin with a user turn.
func trimLeadingAssistantHistory(h []HistoryMessage) []HistoryMessage {
	for i, m := range h {
		if m.UserInputMessage != nil {
			return h[i:]
		}
	}
	return nil
}

// buildToolResultsContinuation is the textual stub we send to Kiro when
// the only content of the trailing user turn is tool results.
func buildToolResultsContinuation(results []ToolResult) string {
	var b strings.Builder
	b.WriteString(toolResultsContinuationPrefix)
	for _, r := range results {
		b.WriteString("\n- ")
		b.WriteString(r.ToolUseID)
		if len(r.Content) > 0 {
			b.WriteString(": ")
			b.WriteString(r.Content[0].Text)
		}
	}
	return b.String()
}

// buildConversationID derives a stable conversation id for Kiro. It uses
// the same anchor scheme as the reference (model + system + first user
// turn) so repeated requests with the same prefix share an id.
func buildConversationID(modelID, systemPrompt, firstUserContent string) string {
	seed := modelID + "\x00" + systemPrompt + "\x00" + firstUserContent
	hash := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(seed))
	return hash.String()
}

// firstAnthropicConversationAnchor returns a stable string drawn from
// the first user message body — used in conversation id derivation.
func firstAnthropicConversationAnchor(msgs []AnthropicMessage) string {
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		content, _, _ := extractAnthropicUserContent(m.Content)
		return content
	}
	return ""
}

// MarshalPayload is a thin convenience that JSON-encodes a Payload while
// excluding the ToolNameMap (it has json:"-").
func MarshalPayload(p *Payload) ([]byte, error) {
	return json.Marshal(p)
}

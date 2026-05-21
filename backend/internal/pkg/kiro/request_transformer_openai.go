package kiro

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// OpenAIRequest is the inbound /v1/chat/completions request shape.
type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
}

// OpenAIMessage covers system / user / assistant / tool roles. Content
// may be a string or a list of typed parts (vision / image_url).
type OpenAIMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// OpenAITool is the OpenAI Chat Completions tool definition.
// Function is the only supported tool type today.
type OpenAITool struct {
	Type     string             `json:"type"`
	Function OpenAIToolFunction `json:"function"`
}

// OpenAIToolFunction is the function-shaped tool body.
type OpenAIToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// OpenAIToolCall is an assistant-produced tool invocation from a previous
// turn (we use it when reconstructing history).
type OpenAIToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function OpenAIToolCallFunction `json:"function"`
}

// OpenAIToolCallFunction is the function payload inside a tool call.
type OpenAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// TransformOpenAIRequest converts an inbound OpenAI Chat Completions
// request into Kiro's Payload shape. System messages are concatenated
// and prepended to the first user message (Kiro has no separate system
// channel — same convention Kiro-Go uses).
//
// thinking can be forced on by appending "-thinking" to the model name.
// OpenAI clients don't have an `thinking` field in the request body.
func TransformOpenAIRequest(req *OpenAIRequest, profileARN string) (*Payload, error) {
	if req == nil {
		return nil, nil
	}

	modelID, thinking := ParseModelAndThinking(req.Model, ThinkingSuffix)
	origin := "AI_EDITOR"

	// Split system messages from the rest.
	var systemPrompt strings.Builder
	nonSystem := make([]OpenAIMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "system" {
			if t := extractOpenAIMessageText(m.Content); t != "" {
				if systemPrompt.Len() > 0 {
					systemPrompt.WriteString("\n")
				}
				systemPrompt.WriteString(t)
			}
			continue
		}
		nonSystem = append(nonSystem, m)
	}
	system := strings.TrimSpace(systemPrompt.String())
	if thinking {
		if system == "" {
			system = ThinkingModePrompt
		} else {
			system = ThinkingModePrompt + "\n\n" + system
		}
	}

	history := make([]HistoryMessage, 0)
	var currentContent string
	var currentImages []Image
	var currentToolResults []ToolResult
	systemMerged := false

	for i, msg := range nonSystem {
		isLast := i == len(nonSystem)-1
		switch msg.Role {
		case "user":
			content, images := extractOpenAIUserContent(msg.Content)
			content = normalizeUserContent(content, len(images) > 0)
			if !systemMerged && system != "" {
				content = system + "\n" + content
				systemMerged = true
			}
			if isLast {
				currentContent = content
				currentImages = images
				continue
			}
			history = append(history, HistoryMessage{
				UserInputMessage: &UserInputMessage{
					Content: content,
					ModelID: modelID,
					Origin:  origin,
					Images:  images,
				},
			})

		case "assistant":
			content := extractOpenAIMessageText(msg.Content)
			var toolUses []ToolUse
			for _, tc := range msg.ToolCalls {
				var input map[string]any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
				if input == nil {
					input = map[string]any{}
				}
				toolUses = append(toolUses, ToolUse{
					ToolUseID: tc.ID,
					Name:      tc.Function.Name,
					Input:     input,
				})
			}
			history = append(history, HistoryMessage{
				AssistantResponseMessage: &AssistantResponseMessage{
					Content:  content,
					ToolUses: toolUses,
				},
			})

		case "tool":
			text := extractOpenAIMessageText(msg.Content)
			currentToolResults = append(currentToolResults, ToolResult{
				ToolUseID: msg.ToolCallID,
				Content:   []ResultText{{Text: text}},
				Status:    "success",
			})
			// Flush tool results into a synthetic user history turn when
			// the next message is not also a tool result.
			nextIdx := i + 1
			if nextIdx >= len(nonSystem) || nonSystem[nextIdx].Role != "tool" {
				if !isLast {
					history = append(history, HistoryMessage{
						UserInputMessage: &UserInputMessage{
							Content: buildToolResultsContinuation(currentToolResults),
							ModelID: modelID,
							Origin:  origin,
							UserInputMessageContext: &UserInputMessageContext{
								ToolResults: currentToolResults,
							},
						},
					})
					currentToolResults = nil
				}
			}
		}
	}

	finalContent := currentContent
	if finalContent == "" {
		switch {
		case len(currentImages) > 0:
			finalContent = normalizeUserContent("", true)
		case len(currentToolResults) > 0:
			finalContent = buildToolResultsContinuation(currentToolResults)
		default:
			finalContent = minimalFallbackUserContent
		}
	}
	if !systemMerged && system != "" {
		finalContent = system + "\n" + finalContent
	}

	tools, toolNameMap := convertOpenAITools(req.Tools)

	p := &Payload{}
	p.ToolNameMap = toolNameMap
	p.ConversationState.ChatTriggerType = "MANUAL"
	p.ConversationState.AgentTaskType = "vibe"
	p.ConversationState.AgentContinuationID = uuid.New().String()
	p.ConversationState.ConversationID = buildConversationID(modelID, system, firstOpenAIConversationAnchor(nonSystem))
	p.ConversationState.CurrentMessage.UserInputMessage = UserInputMessage{
		Content: finalContent,
		ModelID: modelID,
		Origin:  origin,
		Images:  currentImages,
	}
	if len(tools) > 0 || len(currentToolResults) > 0 {
		p.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext = &UserInputMessageContext{
			Tools:       tools,
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

func extractOpenAIUserContent(content any) (string, []Image) {
	if s, ok := content.(string); ok {
		return s, nil
	}
	var text strings.Builder
	var images []Image
	parts, ok := content.([]any)
	if !ok {
		return "", nil
	}
	for _, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			continue
		}
		partType, _ := part["type"].(string)
		switch partType {
		case "text", "input_text":
			if t, ok := part["text"].(string); ok {
				text.WriteString(t)
			}
		case "image_url", "input_image":
			if img := extractImageFromOpenAIPart(part); img != nil {
				images = append(images, *img)
			}
		}
	}
	return text.String(), images
}

func extractOpenAIMessageText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	if parts, ok := content.([]any); ok {
		var sb strings.Builder
		for _, p := range parts {
			if part, ok := p.(map[string]any); ok {
				if t, ok := part["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
	}
	return ""
}

// extractImageFromOpenAIPart handles both data: URL and bare base64 in
// the OpenAI image_url shape, plus the newer Responses-API input_image.
func extractImageFromOpenAIPart(part map[string]any) *Image {
	if iu, ok := part["image_url"].(map[string]any); ok {
		if url, ok := iu["url"].(string); ok {
			if img := parseDataURL(url); img != nil {
				return img
			}
		}
	}
	if data, ok := part["image"].(string); ok {
		if img := parseDataURL(data); img != nil {
			return img
		}
	}
	if data, ok := part["data"].(string); ok {
		if img := parseDataURL(data); img != nil {
			return img
		}
	}
	return nil
}

// parseDataURL splits a "data:image/png;base64,..." URL into an Image.
// Returns nil for non-data URLs.
func parseDataURL(s string) *Image {
	if !strings.HasPrefix(s, "data:") {
		return nil
	}
	semi := strings.Index(s, ";")
	comma := strings.Index(s, ",")
	if semi < 0 || comma < 0 || semi > comma {
		return nil
	}
	mediaType := s[len("data:"):semi]
	data := s[comma+1:]
	format := strings.TrimPrefix(strings.ToLower(mediaType), "image/")
	if format == "" {
		format = "png"
	}
	return &Image{Format: format, Source: ImageSource{Bytes: data}}
}

func firstOpenAIConversationAnchor(messages []OpenAIMessage) string {
	for _, m := range messages {
		if m.Role != "user" {
			continue
		}
		text, _ := extractOpenAIUserContent(m.Content)
		return text
	}
	return ""
}

// convertOpenAITools wraps OpenAI function-call tools in Kiro's
// ToolWrapper shape. Name sanitization keeps the reverse map so the
// response transformer can restore original names.
func convertOpenAITools(tools []OpenAITool) ([]ToolWrapper, map[string]string) {
	if len(tools) == 0 {
		return nil, nil
	}
	wrapped := make([]ToolWrapper, 0, len(tools))
	nameMap := make(map[string]string)
	for _, tool := range tools {
		if tool.Type != "" && tool.Type != "function" {
			continue
		}
		sanitized := ShortenToolName(SanitizeToolName(tool.Function.Name))
		if sanitized != tool.Function.Name {
			nameMap[sanitized] = tool.Function.Name
		}
		wrapped = append(wrapped, ToolWrapper{
			ToolSpecification: ToolSpecification{
				Name:        sanitized,
				Description: TruncateToolDescription(tool.Function.Description),
				InputSchema: InputSchema{JSON: EnsureObjectSchema(tool.Function.Parameters)},
			},
		})
	}
	return wrapped, nameMap
}

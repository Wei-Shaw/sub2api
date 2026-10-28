package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/tidwall/gjson"
)

const geminiToolProgressReminder = "For this tool-assisted task, output one brief, user-visible progress sentence before each batch of function calls. This sentence must be normal assistant output text, not reasoning or function arguments. After tool results, briefly state what you learned and the purpose of the next batch. Respect an explicit user request for silence or a strict output-only format. Do not reveal private reasoning."

// Gemini 3.8 can omit visible progress after a tool result when the caller's
// instruction is buried in a long system prompt. Reinforce only that existing
// request; do not impose narration on callers that did not ask for it.
func reinforceGeminiToolProgress(model string, body any) {
	model = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "models/")
	if model != "gemini-3.8-flash" && !strings.HasPrefix(model, "gemini-3.8-flash-") {
		return
	}
	request, ok := body.(map[string]any)
	if !ok {
		return
	}
	if config, ok := request["toolConfig"].(map[string]any); ok {
		if calling, ok := config["functionCallingConfig"].(map[string]any); ok {
			mode, _ := calling["mode"].(string)
			if strings.EqualFold(mode, "ANY") || strings.EqualFold(mode, "NONE") {
				return
			}
		}
	}
	tools, _ := request["tools"].([]any)
	hasFunctions := false
	for _, tool := range tools {
		if tool, ok := tool.(map[string]any); ok {
			if functions, ok := tool["functionDeclarations"].([]any); ok && len(functions) > 0 {
				hasFunctions = true
				break
			}
		}
	}
	if !hasFunctions {
		return
	}
	system, _ := request["systemInstruction"].(map[string]any)
	parts, _ := system["parts"].([]any)
	requested := false
	for _, part := range parts {
		if part, ok := part.(map[string]any); ok {
			text, _ := part["text"].(string)
			if strings.Contains(text, geminiToolProgressReminder) {
				return
			}
			if strings.Contains(strings.ToLower(text), "before each tool batch") {
				requested = true
			}
		}
	}
	if requested {
		system["parts"] = append(parts, map[string]any{"text": geminiToolProgressReminder})
	}
}

// Only the explicitly requested progress contract may enable gateway notices.
// Tool-result text is not a fresh user instruction, so skip those blocks.
func allowGeminiToolProgressFallback(originalBody, geminiBody []byte, messages []antigravity.ClaudeMessage) bool {
	model := strings.ToLower(gjson.GetBytes(geminiBody, "model").String())
	if model != "gemini-3.8-flash" && !strings.HasPrefix(model, "gemini-3.8-flash-") {
		return false
	}
	if !strings.Contains(gjson.GetBytes(geminiBody, "request.systemInstruction.parts").Raw, geminiToolProgressReminder) {
		return false
	}
	// Compatibility conversion can omit toolConfig, so the original API
	// request remains authoritative for forced/disabled tool selection.
	choice := gjson.GetBytes(originalBody, "tool_choice")
	kind := choice.String()
	if choice.IsObject() {
		kind = choice.Get("type").String()
	}
	switch strings.ToLower(kind) {
	case "required", "any", "none", "function", "tool":
		return false
	}
	mode := gjson.GetBytes(geminiBody, "request.toolConfig.functionCallingConfig.mode").String()
	if strings.EqualFold(mode, "ANY") || strings.EqualFold(mode, "NONE") {
		return false
	}
	for _, path := range []string{"response_format.type", "text.format.type", "output_format.type"} {
		format := gjson.GetBytes(originalBody, path).String()
		if format == "json_schema" || format == "json_object" {
			return false
		}
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		var text string
		if json.Unmarshal(messages[i].Content, &text) != nil {
			var blocks []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal(messages[i].Content, &blocks) != nil {
				continue
			}
			for _, block := range blocks {
				if block.Type == "text" {
					text += block.Text + "\n"
				}
			}
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		text = strings.ToLower(text)
		for _, quiet := range []string{"不要过程", "不用过程", "不需要过程", "不要解释", "只输出", "仅输出", "只给结果", "保持静默", "do not narrate", "no commentary", "output only", "only output", "reply only", "do not explain"} {
			if strings.Contains(text, quiet) {
				return false
			}
		}
		break
	}
	return true
}

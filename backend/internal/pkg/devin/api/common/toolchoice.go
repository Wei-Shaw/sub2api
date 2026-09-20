// 本文件把 OpenAI / Anthropic 的 tool_choice 与 parallel_tool_calls 请求字段
// 解析为中间模型的 llm.ToolChoice。
package common

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// ParseOpenAIToolChoice 解析 OpenAI 风格的 tool_choice 字段：
// 字符串 "auto"/"none"/"required"，或对象 {"type":"function","function":{"name":X}}
// （Responses API 的扁平形态 {"type":"function","name":X} 同样接受）。
// 空输入与 "auto" 返回 nil（模型自选，与缺省一致）。
func ParseOpenAIToolChoice(raw json.RawMessage) (*llm.ToolChoice, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		switch name {
		case "", "auto":
			return nil, nil
		case "none":
			return &llm.ToolChoice{Mode: llm.ToolChoiceNone}, nil
		case "required":
			return &llm.ToolChoice{Mode: llm.ToolChoiceRequired}, nil
		default:
			return nil, fmt.Errorf("unsupported tool_choice %q", name)
		}
	}
	var object struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("decode tool_choice: %w", err)
	}
	toolName := object.Name
	if toolName == "" {
		toolName = object.Function.Name
	}
	if toolName == "" {
		return nil, fmt.Errorf("tool_choice object requires a function name")
	}
	return &llm.ToolChoice{Mode: llm.ToolChoiceNamed, ToolName: toolName}, nil
}

// ParseAnthropicToolChoice 解析 Anthropic 风格的 tool_choice 对象：
// {"type":"auto"|"any"|"tool"|"none", "name":X, "disable_parallel_tool_calls":bool}。
// Anthropic 的 "any"（任一工具必须调用）归一为 ToolChoiceRequired——
// Devin 上游 option_name 不接受 "any"（实测 invalid_argument）。
// 第二个返回值是 disable_parallel_tool_calls。
func ParseAnthropicToolChoice(raw json.RawMessage) (*llm.ToolChoice, bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, false, nil
	}
	var object struct {
		Type                     string `json:"type"`
		Name                     string `json:"name"`
		DisableParallelToolCalls bool   `json:"disable_parallel_tool_calls"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, false, fmt.Errorf("decode tool_choice: %w", err)
	}
	var choice *llm.ToolChoice
	switch object.Type {
	case "", "auto":
	case "any":
		choice = &llm.ToolChoice{Mode: llm.ToolChoiceRequired}
	case "tool":
		if object.Name == "" {
			return nil, false, fmt.Errorf("tool_choice type=tool requires a name")
		}
		choice = &llm.ToolChoice{Mode: llm.ToolChoiceNamed, ToolName: object.Name}
	case "none":
		choice = &llm.ToolChoice{Mode: llm.ToolChoiceNone}
	default:
		return nil, false, fmt.Errorf("unsupported tool_choice type %q", object.Type)
	}
	return choice, object.DisableParallelToolCalls, nil
}

// 本文件定义 OpenAI Chat Completions 请求 JSON 到中间 LLM 模型的转换。
package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/api/common"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// Request 是 OpenAI Chat Completions 请求中本适配器支持的字段集合。
type Request struct {
	Model               string          `json:"model"`
	Messages            []Message       `json:"messages"`
	Tools               []Tool          `json:"tools,omitempty"`
	ToolChoice          json.RawMessage `json:"tool_choice,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
	StreamOptions       *StreamOptions  `json:"stream_options,omitempty"`
	MaxTokens           *int            `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int            `json:"max_completion_tokens,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	Stop                json.RawMessage `json:"stop,omitempty"`
	ResponseFormat      json.RawMessage `json:"response_format,omitempty"`
	TopK                *int            `json:"top_k,omitempty"`
	Seed                *int64          `json:"seed,omitempty"`
	User                string          `json:"user,omitempty"`
	PromptCacheKey      string          `json:"prompt_cache_key,omitempty"`
	ParallelToolCalls   *bool           `json:"parallel_tool_calls,omitempty"`
	N                   *int            `json:"n,omitempty"`
}

// Message 是 Chat Completions 消息条目。
type Message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Name       string          `json:"name,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	// ReasoningContent 是 DeepSeek 系/部分代理回传思考文本的约定字段；
	// 解码进 ThinkingContent，客户端回灌历史时思考不会静默丢失。
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// ToolCall 是助手消息中的工具调用（也用于流式增量）。
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall 是工具调用的函数部分。
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool 是 OpenAI Chat function 工具定义。
type Tool struct {
	Type     string       `json:"type"`
	Function FunctionTool `json:"function"`
}

// FunctionTool 是 function 工具详情。
type FunctionTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// StreamOptions 是流式额外选项。
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// chatRequestFields 是 DecodeRequest 已消费的顶层字段；其余字段
// （reasoning_effort/store/service_tier/metadata/response_format 等）
// 上游没有对应物，记入 Dropped 透出而不是静默吞掉。
var chatRequestFields = map[string]bool{
	"model": true, "messages": true, "tools": true, "tool_choice": true,
	"stream": true, "stream_options": true, "max_tokens": true,
	"max_completion_tokens": true, "temperature": true, "top_p": true,
	"stop": true, "top_k": true, "seed": true, "user": true,
	"prompt_cache_key": true, "parallel_tool_calls": true, "n": true,
}

// AdaptedRequest 是 Chat 请求转换后的中间请求和生成选项。
type AdaptedRequest struct {
	Context llm.RequestMessages
	Options RequestOptions
}

// RequestOptions 保存不属于对话历史的生成控制参数。
type RequestOptions struct {
	Stream          bool
	IncludeUsage    bool
	MaxOutputTokens *int
	Temperature     *float64
}

// DecodeRequest 将 OpenAI Chat Completions JSON 请求转换为中间请求。
func DecodeRequest(data []byte) (AdaptedRequest, error) {
	var request Request
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&request); err != nil {
		return AdaptedRequest{}, fmt.Errorf("decode chat request: %w", err)
	}
	if request.Model == "" {
		return AdaptedRequest{}, errors.New("chat request model is required")
	}
	if len(request.Messages) == 0 {
		return AdaptedRequest{}, errors.New("chat request messages are required")
	}

	context := llm.RequestMessages{Model: request.Model}
	context.Dropped = append(context.Dropped, common.UnconsumedFields(data, chatRequestFields)...)
	maxTokensValue := request.MaxCompletionTokens
	if maxTokensValue == nil {
		maxTokensValue = request.MaxTokens
	}
	if maxTokensValue != nil && *maxTokensValue > 0 {
		context.MaxTokens = maxTokensValue
	}
	context.Temperature = request.Temperature
	context.TopP = request.TopP
	if request.TopK != nil && *request.TopK > 0 {
		context.TopK = request.TopK
	}
	context.Seed = request.Seed
	// 上游 CASCADE 通道只支持单次补全：num_completions>1 会中途崩流，
	// 本地尽早拒绝比打到上游更可读。
	if request.N != nil && *request.N > 1 {
		return AdaptedRequest{}, errors.New("chat request n > 1 is not supported by this provider")
	}
	toolChoice, err := common.ParseOpenAIToolChoice(request.ToolChoice)
	if err != nil {
		return AdaptedRequest{}, err
	}
	context.ToolChoice = toolChoice
	if request.ParallelToolCalls != nil && !*request.ParallelToolCalls {
		context.DisableParallelToolCalls = true
	}
	if len(bytes.TrimSpace(request.Stop)) > 0 && !bytes.Equal(bytes.TrimSpace(request.Stop), []byte("null")) {
		var stops []string
		if err := json.Unmarshal(request.Stop, &stops); err != nil {
			var single string
			if json.Unmarshal(request.Stop, &single) == nil && single != "" {
				stops = []string{single}
			}
		}
		context.StopSequences = stops
	}
	context.SessionKey = request.PromptCacheKey
	if context.SessionKey == "" {
		context.SessionKey = request.User
	}
	// toolNames 随解码增量登记 assistant tool_call 的 id→name，
	// tool 消息按 id 直查，替代逐条 findToolName 全历史回扫。
	toolNames := make(map[string]string)
	if err := appendMessages(&context, request.Messages, toolNames); err != nil {
		return AdaptedRequest{}, err
	}
	for _, tool := range request.Tools {
		if tool.Type != "function" {
			context.Dropped = append(context.Dropped, "tool:"+tool.Type)
			continue
		}
		schema := tool.Function.Parameters
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		context.Tools = append(context.Tools, llm.ToolDefinition{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: schema,
		})
	}
	if err := context.Validate(); err != nil {
		return AdaptedRequest{}, fmt.Errorf("validate adapted request: %w", err)
	}

	return AdaptedRequest{
		Context: context,
		Options: RequestOptions{
			Stream:          request.Stream,
			IncludeUsage:    request.StreamOptions != nil && request.StreamOptions.IncludeUsage,
			MaxOutputTokens: maxTokensValue,
			Temperature:     request.Temperature,
		},
	}, nil
}

func appendMessages(context *llm.RequestMessages, messages []Message, toolNames map[string]string) error {
	for index, message := range messages {
		if err := appendMessage(context, message, toolNames); err != nil {
			return fmt.Errorf("message[%d]: %w", index, err)
		}
	}
	return nil
}

func appendMessage(context *llm.RequestMessages, message Message, toolNames map[string]string) error {
	switch message.Role {
	case "system", "developer":
		content, err := common.DecodeContent(message.Content)
		if err != nil {
			return err
		}
		text := common.ContentText(content)
		if context.SystemPrompt != "" && text != "" {
			context.SystemPrompt += "\n"
		}
		context.SystemPrompt += text
	case "user":
		content, err := decodeUserContent(message.Content)
		if err != nil {
			return err
		}
		context.Messages = append(context.Messages, llm.UserMessage{
			Content:     content,
			TimestampMS: time.Now().UnixMilli(),
		})
	case "assistant":
		content, err := decodeAssistantContent(context, message, toolNames)
		if err != nil {
			return err
		}
		context.Messages = append(context.Messages, llm.AssistantMessage{
			Content:     content,
			TimestampMS: time.Now().UnixMilli(),
		})
	case "tool":
		if message.ToolCallID == "" {
			return errors.New("tool message requires tool_call_id")
		}
		content, err := common.DecodeContent(message.Content)
		if err != nil {
			return err
		}
		name := toolNames[message.ToolCallID]
		if name == "" {
			// 压缩后的历史可能丢掉对应的 assistant tool_call；兜底名交给
			// wire 层的 demoteOrphanToolResults 降级，避免整请求 400。
			context.Dropped = append(context.Dropped, "unmatched_tool_call_id:"+message.ToolCallID)
			name = "tool"
		}
		context.Messages = append(context.Messages, llm.ToolResultMessage{
			ToolCallID:  message.ToolCallID,
			ToolName:    name,
			Content:     content,
			TimestampMS: time.Now().UnixMilli(),
		})
	default:
		context.Dropped = append(context.Dropped, "role:"+message.Role)
	}
	return nil
}

func decodeUserContent(raw json.RawMessage) ([]llm.Content, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return []llm.Content{llm.TextContent{Text: ""}}, nil
	}
	return common.DecodeContent(raw)
}

func decodeAssistantContent(context *llm.RequestMessages, message Message, toolNames map[string]string) ([]llm.Content, error) {
	var content []llm.Content
	if len(bytes.TrimSpace(message.Content)) > 0 && !bytes.Equal(bytes.TrimSpace(message.Content), []byte("null")) {
		decoded, err := common.DecodeContent(message.Content)
		if err != nil {
			return nil, err
		}
		content = append(content, decoded...)
	}
	if message.ReasoningContent != "" {
		content = append(content, llm.ThinkingContent{Thinking: message.ReasoningContent})
	}
	for _, call := range message.ToolCalls {
		if call.Type != "" && call.Type != "function" {
			context.Dropped = append(context.Dropped, "tool_call:"+call.Type)
			continue
		}
		args := json.RawMessage(call.Function.Arguments)
		custom := false
		if len(bytes.TrimSpace(args)) == 0 {
			args = json.RawMessage(`{}`)
		} else if !llm.IsJSONObject(args) {
			// 客户端回灌的畸形/非 JSON 参数原文按 custom 通道保留，
			// 吞成 {} 会让上游看到的调用语义悄悄变空。
			custom = true
		}
		toolNames[call.ID] = call.Function.Name
		content = append(content, llm.ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: args,
			Custom:    custom,
		})
	}
	return content, nil
}

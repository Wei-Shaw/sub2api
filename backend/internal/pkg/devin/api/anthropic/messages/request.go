// 本文件定义 Anthropic Messages API 请求 JSON 到中间 LLM 模型的转换。
package messages

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/api/common"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// Request 是 Anthropic Messages 请求中本适配器支持的字段集合。
type Request struct {
	Model         string          `json:"model"`
	Messages      []Message       `json:"messages"`
	System        json.RawMessage `json:"system,omitempty"`
	MaxTokens     int             `json:"max_tokens"`
	Tools         []Tool          `json:"tools,omitempty"`
	ToolChoice    json.RawMessage `json:"tool_choice,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	TopK          *int            `json:"top_k,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	// Thinking 是 Anthropic 扩展思考开关；enabled/adaptive → Devin 默认 high 档，
	// disabled → off 档（经 thinkingLevelMap 解析为上游 uid）。
	Thinking *struct {
		Type string `json:"type"`
	} `json:"thinking,omitempty"`
	// OutputConfig 携带新版 effort 参数（Claude Code 2.1+ 的推理档位选择器
	// 走 output_config.effort，取值为 low/medium/high/xhigh/max）。
	OutputConfig *struct {
		Effort string `json:"effort,omitempty"`
	} `json:"output_config,omitempty"`
}

// Message 是 Anthropic 消息条目。
type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// Tool 是 Anthropic 工具定义。
type Tool struct {
	// Type 缺省/为 "custom" 时是客户端 function 工具；web_search_* 等
	// server tool 没有 input_schema 语义，本代理不转发。
	Type        string          `json:"type,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ToolResult 是 Anthropic 工具结果内容块。
type ToolResult struct {
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error,omitempty"`
}

// ToolUse 是 Anthropic 助手历史中的工具调用。
type ToolUse struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// anthropicRequestFields 是 DecodeRequest 已消费的顶层字段；其余字段
// （thinking/service_tier/context_management/mcp_servers 等）上游没有
// 对应物，记入 Dropped 透出而不是静默吞掉。
var anthropicRequestFields = map[string]bool{
	"model": true, "messages": true, "system": true, "max_tokens": true,
	"tools": true, "tool_choice": true, "stream": true, "temperature": true,
	"top_p": true, "top_k": true, "stop_sequences": true, "metadata": true,
	"thinking": true, "output_config": true,
}

// AdaptedRequest 是 Anthropic 请求转换后的中间请求和生成选项。
type AdaptedRequest struct {
	Context llm.RequestMessages
	Options RequestOptions
}

// RequestOptions 保存不属于对话历史的生成控制参数。
type RequestOptions struct {
	Stream          bool
	MaxOutputTokens int
	Temperature     *float64
}

// DecodeRequest 将 Anthropic Messages JSON 请求转换为中间请求。
func DecodeRequest(data []byte) (AdaptedRequest, error) {
	var request Request
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&request); err != nil {
		return AdaptedRequest{}, fmt.Errorf("decode anthropic request: %w", err)
	}
	if request.Model == "" {
		return AdaptedRequest{}, errors.New("anthropic request model is required")
	}
	if len(request.Messages) == 0 {
		return AdaptedRequest{}, errors.New("anthropic request messages are required")
	}

	context := llm.RequestMessages{Model: request.Model}
	context.Dropped = append(context.Dropped, common.UnconsumedFields(data, anthropicRequestFields)...)
	// thinking 开关 → thinking 档位；budget_tokens 不做档位量化，
	// 由上游模型自身容量决定（与插件行为一致）。
	// adaptive 是新版客户端的默认形态（自适应预算），按开启处理。
	if request.Thinking != nil {
		switch request.Thinking.Type {
		case "enabled", "adaptive":
			context.Reasoning = "high"
		case "disabled":
			context.Reasoning = "off"
		}
	}
	// output_config.effort 是更具体的信号：用户显式选了档位时优先于
	// thinking.type 的粗粒度开关（effort 存在本身就隐含开启思考）。
	if request.OutputConfig != nil {
		if effort := strings.TrimSpace(request.OutputConfig.Effort); effort != "" {
			context.Reasoning = effort
		}
	}
	maxTokens := request.MaxTokens
	if maxTokens > 0 {
		context.MaxTokens = &maxTokens
	}
	context.Temperature = request.Temperature
	context.TopP = request.TopP
	if request.TopK != nil && *request.TopK > 0 {
		context.TopK = request.TopK
	}
	context.StopSequences = request.StopSequences
	toolChoice, disableParallel, err := common.ParseAnthropicToolChoice(request.ToolChoice)
	if err != nil {
		return AdaptedRequest{}, err
	}
	context.ToolChoice = toolChoice
	context.DisableParallelToolCalls = disableParallel
	if len(bytes.TrimSpace(request.Metadata)) > 0 {
		var metadata struct {
			UserID string `json:"user_id"`
		}
		if json.Unmarshal(request.Metadata, &metadata) == nil {
			context.SessionKey = metadata.UserID
		}
	}
	if len(bytes.TrimSpace(request.System)) > 0 && !bytes.Equal(bytes.TrimSpace(request.System), []byte("null")) {
		if err := appendSystem(&context, request.System); err != nil {
			return AdaptedRequest{}, err
		}
	}
	if err := appendMessages(&context, request.Messages); err != nil {
		return AdaptedRequest{}, err
	}
	for _, tool := range request.Tools {
		// 只转客户端自定义工具：server tool（web_search_*/advisor_* 等）
		// 由供应商托管执行，上游 Devin 无对应物，转发只会制造废工具。
		if tool.Type != "" && tool.Type != "custom" {
			context.Dropped = append(context.Dropped, "tool:"+tool.Type)
			continue
		}
		schema := tool.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		context.Tools = append(context.Tools, llm.ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
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
			MaxOutputTokens: request.MaxTokens,
			Temperature:     request.Temperature,
		},
	}, nil
}

func appendSystem(context *llm.RequestMessages, raw json.RawMessage) error {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		context.SystemPrompt = text
		return nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return fmt.Errorf("decode anthropic system: %w", err)
	}
	for _, part := range parts {
		var block struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(part, &block); err != nil {
			return err
		}
		if block.Type != "text" {
			context.Dropped = append(context.Dropped, "system_block:"+block.Type)
			continue
		}
		if context.SystemPrompt != "" && block.Text != "" {
			context.SystemPrompt += "\n"
		}
		context.SystemPrompt += block.Text
	}
	return nil
}

// appendMessages 顺序解码消息流；toolNames 随 assistant tool_use 增量登记
// id→name，后续 tool_result 直查，替代逐条 findToolNameByToolUseID 回扫。
func appendMessages(context *llm.RequestMessages, messages []Message) error {
	toolNames := make(map[string]string)
	for index, message := range messages {
		if err := appendMessage(context, message, toolNames); err != nil {
			return fmt.Errorf("message[%d]: %w", index, err)
		}
	}
	return nil
}

func appendMessage(context *llm.RequestMessages, message Message, toolNames map[string]string) error {
	switch message.Role {
	case "user":
		messages, err := decodeAnthropicUserMessages(context, message.Content, toolNames)
		if err != nil {
			return err
		}
		context.Messages = append(context.Messages, messages...)
	case "assistant":
		content, err := decodeAssistantContent(context, message.Content, toolNames)
		if err != nil {
			return err
		}
		context.Messages = append(context.Messages, llm.AssistantMessage{
			Content:     content,
			TimestampMS: time.Now().UnixMilli(),
		})
	case "system":
		// Claude Code 在消息流中间插入 role:system 的途中注入（agent 列表、
		// task reminder、system notification）。内容位置敏感——解码为
		// UserMessage 保持时序，不能折叠进系统提示词。
		messages, err := decodeAnthropicUserMessages(context, message.Content, toolNames)
		if err != nil {
			return err
		}
		context.Messages = append(context.Messages, messages...)
	default:
		context.Dropped = append(context.Dropped, "role:"+message.Role)
	}
	return nil
}

// decodeAnthropicUserMessages 把 Anthropic user 消息 content 拆分为一个或多个中间消息。
// tool_result 内容块会生成独立的 llm.ToolResultMessage。
func decodeAnthropicUserMessages(context *llm.RequestMessages, raw json.RawMessage, toolNames map[string]string) ([]llm.Message, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return []llm.Message{llm.UserMessage{
			Content:     []llm.Content{llm.TextContent{Text: ""}},
			TimestampMS: time.Now().UnixMilli(),
		}}, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []llm.Message{llm.UserMessage{
			Content:     []llm.Content{llm.TextContent{Text: text}},
			TimestampMS: time.Now().UnixMilli(),
		}}, nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("decode user content: %w", err)
	}

	var result []llm.Message
	var currentUserContent []llm.Content
	flushUser := func() {
		if len(currentUserContent) == 0 {
			return
		}
		result = append(result, llm.UserMessage{
			Content:     currentUserContent,
			TimestampMS: time.Now().UnixMilli(),
		})
		currentUserContent = nil
	}

	for index, part := range parts {
		var header struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			ToolUseID string          `json:"tool_use_id"`
			Content   json.RawMessage `json:"content"`
			IsError   bool            `json:"is_error"`
		}
		if err := json.Unmarshal(part, &header); err != nil {
			return nil, fmt.Errorf("content[%d]: %w", index, err)
		}
		switch header.Type {
		case "text":
			currentUserContent = append(currentUserContent, llm.TextContent{Text: header.Text})
		case "image":
			image, err := common.DecodeImagePart(part)
			if err != nil {
				return nil, fmt.Errorf("content[%d]: %w", index, err)
			}
			currentUserContent = append(currentUserContent, image)
		case "tool_result":
			flushUser()
			tool, err := decodeToolResult(context, header.ToolUseID, header.Content, header.IsError, toolNames)
			if err != nil {
				return nil, fmt.Errorf("content[%d]: %w", index, err)
			}
			result = append(result, tool)
		default:
			context.Dropped = append(context.Dropped, "user_block:"+header.Type)
		}
	}
	flushUser()
	return result, nil
}

func decodeAssistantContent(context *llm.RequestMessages, raw json.RawMessage, toolNames map[string]string) ([]llm.Content, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return []llm.Content{llm.TextContent{Text: ""}}, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []llm.Content{llm.TextContent{Text: text}}, nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("decode assistant content: %w", err)
	}
	content := make([]llm.Content, 0, len(parts))
	for index, part := range parts {
		var header struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			Thinking  string          `json:"thinking"`
			Signature string          `json:"signature"`
			Data      string          `json:"data"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
		}
		if err := json.Unmarshal(part, &header); err != nil {
			return nil, fmt.Errorf("content[%d]: %w", index, err)
		}
		switch header.Type {
		case "text":
			content = append(content, llm.TextContent{Text: header.Text})
		case "thinking":
			content = append(content, llm.ThinkingContent{
				Thinking:          header.Thinking,
				ThinkingSignature: header.Signature,
				SignatureType:     guessSignatureType(header.Signature),
			})
		case "redacted_thinking":
			// redacted 块的 data 是密封思考体；在 Devin wire 上对应 signature+redacted 标记。
			content = append(content, llm.ThinkingContent{
				ThinkingSignature: header.Data,
				SignatureType:     guessSignatureType(header.Data),
				Redacted:          true,
			})
		case "tool_use":
			args := header.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			toolNames[header.ID] = header.Name
			content = append(content, llm.ToolCall{ID: header.ID, Name: header.Name, Arguments: args})
		default:
			context.Dropped = append(context.Dropped, "assistant_block:"+header.Type)
		}
	}
	return content, nil
}

func decodeToolResult(context *llm.RequestMessages, toolUseID string, raw json.RawMessage, isError bool, toolNames map[string]string) (llm.ToolResultMessage, error) {
	if toolUseID == "" {
		return llm.ToolResultMessage{}, errors.New("tool_result requires tool_use_id")
	}
	name := toolNames[toolUseID]
	if name == "" {
		context.Dropped = append(context.Dropped, "unmatched_tool_use_id:"+toolUseID)
		name = "tool"
	}
	content, err := decodeAnthropicContent(context, raw)
	if err != nil {
		return llm.ToolResultMessage{}, err
	}
	if len(content) == 0 {
		content = []llm.Content{llm.TextContent{Text: ""}}
	}
	return llm.ToolResultMessage{
		ToolCallID:  toolUseID,
		ToolName:    name,
		Content:     content,
		IsError:     isError,
		TimestampMS: time.Now().UnixMilli(),
	}, nil
}

// decodeAnthropicContent 把原始 JSON 解码为 text / image 内容块。
func decodeAnthropicContent(context *llm.RequestMessages, raw json.RawMessage) ([]llm.Content, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return []llm.Content{llm.TextContent{Text: ""}}, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []llm.Content{llm.TextContent{Text: text}}, nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("decode content: %w", err)
	}
	content := make([]llm.Content, 0, len(parts))
	for index, part := range parts {
		var header struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Resource *struct {
				URI      string `json:"uri"`
				MIMEType string `json:"mimeType"`
				Text     string `json:"text"`
				Blob     string `json:"blob"`
			} `json:"resource"`
		}
		if err := json.Unmarshal(part, &header); err != nil {
			return nil, fmt.Errorf("content[%d]: %w", index, err)
		}
		switch header.Type {
		case "text":
			content = append(content, llm.TextContent{Text: header.Text})
		case "image":
			image, err := common.DecodeImagePart(part)
			if err != nil {
				return nil, fmt.Errorf("content[%d]: %w", index, err)
			}
			content = append(content, image)
		case "resource":
			// MCP tool_result 的 resource 块：text 直接展开；blob 按图片或占位降级。
			if header.Resource == nil {
				continue
			}
			switch {
			case header.Resource.Text != "":
				content = append(content, llm.TextContent{Text: header.Resource.Text})
			case header.Resource.Blob != "" && strings.HasPrefix(header.Resource.MIMEType, "image/"):
				content = append(content, llm.ImageContent{Data: header.Resource.Blob, MIMEType: header.Resource.MIMEType})
			default:
				content = append(content, llm.TextContent{Text: "[resource: " + header.Resource.URI + "]"})
			}
		default:
			context.Dropped = append(context.Dropped, "content_block:"+header.Type)
		}
	}
	return content, nil
}

// guessSignatureType 给回放的思考签名标注上游 signature_type：
// sealed.* 是本代理下发过的密封格式；其余不透明 blob 按 anthropic
// 体制标注（走 Anthropic 协议的签名要么来自本代理的 claude 模型，
// 要么来自真实 Anthropic API，两边都是 anthropic 体制）。
// 缺类型实测被上游容忍，标错类型才会 invalid_argument。
func guessSignatureType(signature string) string {
	if strings.HasPrefix(signature, "sealed.") {
		return "sealed"
	}
	if signature == "" {
		return ""
	}
	return "anthropic"
}

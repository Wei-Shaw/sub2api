// Package llm 定义与具体模型供应商无关的请求消息、响应事件与流接口。
//
// 移植自 WncFht/devin2api 的 internal/llm（MIT），作为三个入站协议
// （Anthropic Messages / OpenAI Chat Completions / OpenAI Responses）
// 与 Devin 上游适配层之间的中间表示：agent loop 语义等价，而非
// Provider 请求结构等价。
package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// MessageRole 标识一条中间消息在对话中的角色。
type MessageRole string

const (
	MessageRoleUser       MessageRole = "user"
	MessageRoleAssistant  MessageRole = "assistant"
	MessageRoleToolResult MessageRole = "toolResult"
)

// ContentType 标识一个消息内容块的种类。
type ContentType string

const (
	ContentTypeText     ContentType = "text"
	ContentTypeThinking ContentType = "thinking"
	ContentTypeImage    ContentType = "image"
	ContentTypeToolCall ContentType = "toolCall"
)

// RequestMessages 是发送给任意供应商适配器的完整请求上下文。
type RequestMessages struct {
	// Model 是调用方指定的模型标识；空表示由适配器使用默认配置。
	Model string
	// SystemPrompt 是独立于普通消息历史的系统提示词。
	SystemPrompt string
	// Messages 是按时间顺序排列、可跨供应商重放的完整对话历史。
	Messages []Message
	// Tools 是本次请求允许模型调用的工具定义。
	Tools []ToolDefinition
	// MaxTokens 是可选的输出 token 上限；nil 表示使用供应商默认。
	MaxTokens *int
	// Temperature 是可选的采样温度；nil 表示使用供应商默认。
	Temperature *float64
	// TopP 是可选的 nucleus 采样参数；nil 表示使用供应商默认。
	TopP *float64
	// TopK 是可选的 top-k 采样参数；nil 表示使用供应商默认。
	TopK *int
	// StopSequences 是可选的停止序列列表。
	StopSequences []string
	// ToolChoice 是调用方对工具调用行为的偏好；nil 表示交给模型自选。
	ToolChoice *ToolChoice
	// DisableParallelToolCalls 为 true 时要求模型不要在一轮内发起多个并行工具调用。
	// Devin 上游接受但会忽略该标记（实测并行调用照常发出），适配器仅做形状透传。
	DisableParallelToolCalls bool
	// Seed 是可选的采样种子；nil 表示由供应商随机。
	Seed *int64
	// SessionKey 是调用方提供的会话标识（如 user / prompt_cache_key /
	// metadata.user_id），适配器可据此为同一对话派生稳定的上游会话 ID。
	// 空表示调用方未提供。
	SessionKey string
	// Dropped 记录请求解码时被丢弃/降级的下游字段（"kind:detail"），
	// 供调试日志透出——「解码即过滤」的静默面需要可观测。
	Dropped []string
}

// RequestRepairs 是一次请求投影为上游 wire 格式时发生的静默修复计数，
// 与响应方向的 AssistantMessageDiagnostic 同族（诊断，不改变主结果）。
// 协议翻译层的修复（调用-结果配对重排、孤儿结果降级、空助手消息丢弃、
// 历史图片剥离、策略指纹改写）本身合法，但上游协议漂移排障必须能回答
// 「代理对这次请求动过什么」。全零时整个字段不落盘。
type RequestRepairs struct {
	// ReorderedPrompts 是 call→result 配对重排中改变位置的 prompt 数。
	ReorderedPrompts int `json:"reordered_prompts,omitempty"`
	// DemotedOrphanResults 是被降级为 USER 文本的孤儿 TOOL 结果数。
	DemotedOrphanResults int `json:"demoted_orphan_results,omitempty"`
	// DroppedEmptyAssistant 是被跳过的空助手消息数（上游见空回复会退化）。
	DroppedEmptyAssistant int `json:"dropped_empty_assistant,omitempty"`
	// OmittedHistoryImages 是被改写为文本占位的历史图片数（上游只收当前轮图片）。
	OmittedHistoryImages int `json:"omitted_history_images,omitempty"`
	// SanitizeHits 是上游内容策略指纹改写按规则 id 的命中计数。
	SanitizeHits map[string]int `json:"sanitize_hits,omitempty"`
}

// Total 返回全部修复动作的合计次数，供日志索引汇总成单字段。
func (repairs RequestRepairs) Total() int {
	total := repairs.ReorderedPrompts + repairs.DemotedOrphanResults +
		repairs.DroppedEmptyAssistant + repairs.OmittedHistoryImages
	for _, hits := range repairs.SanitizeHits {
		total += hits
	}
	return total
}

// ToolChoiceMode 标识客户端要求的工具调用模式。
type ToolChoiceMode string

const (
	// ToolChoiceAuto 由模型自行决定是否调用工具（默认行为）。
	ToolChoiceAuto ToolChoiceMode = "auto"
	// ToolChoiceNone 禁止模型调用工具。
	ToolChoiceNone ToolChoiceMode = "none"
	// ToolChoiceRequired 强制模型本轮必须调用工具（任一工具）。
	ToolChoiceRequired ToolChoiceMode = "required"
	// ToolChoiceNamed 强制模型调用 ToolName 指定的工具。
	ToolChoiceNamed ToolChoiceMode = "named"
)

// ToolChoice 是供应商无关的工具调用偏好。
// Anthropic 的 {"type":"any"} 在本层归一为 ToolChoiceRequired——
// Devin 上游的 option_name 合法值是 none/auto/required，"any" 会被拒绝。
type ToolChoice struct {
	// Mode 是归一化后的调用模式。
	Mode ToolChoiceMode
	// ToolName 是 ToolChoiceNamed 模式下要求调用的工具名。
	ToolName string
}

// Message 是用户、助手或工具结果消息的统一接口。
type Message interface {
	// Role 返回消息在对话中的角色。
	Role() MessageRole
	// Validate 检查消息是否满足中间层约束。
	Validate() error
}

// Content 是文字、思考、图片或工具调用内容块的统一接口。
type Content interface {
	// ContentType 返回内容块的种类。
	ContentType() ContentType
	// Validate 检查内容块是否满足中间层约束。
	Validate() error
}

// TextContent 表示普通文字内容块。
type TextContent struct {
	// Text 是向用户展示或作为上下文重放的文字。
	Text string
	// TextSignature 是供应商返回的文字签名或序列化签名，重放时应原样保留。
	// 当前无生产者：Devin 上游不签发文字签名，为 Anthropic 系签名形态预留。
	TextSignature string
}

// ContentType 返回文字内容类型。
func (TextContent) ContentType() ContentType { return ContentTypeText }

// Validate 检查文字内容块。
func (TextContent) Validate() error { return nil }

// ThinkingContent 表示模型的思考或推理内容块。
type ThinkingContent struct {
	// Thinking 是可见的思考内容；加密思考场景下可以为空。
	Thinking string
	// ThinkingSignature 是供应商签名或加密后的不透明载荷，重放时应原样保留。
	ThinkingSignature string
	// SignatureType 是签名载荷的格式标识（Devin 上游 signature_type：
	// sealed/anthropic/openai）。签名的解析规则由它决定——openai 型签名
	// 是序列化的 Responses reasoning item，其余是不透明 blob。重放时必须
	// 随签名原样回传，实测错配触发上游 invalid_argument。
	SignatureType string
	// Redacted 表示思考正文已被供应商隐藏，签名中可能保存可重放载荷。
	Redacted bool
}

// ContentType 返回思考内容类型。
func (ThinkingContent) ContentType() ContentType { return ContentTypeThinking }

// Validate 检查思考内容块。
func (content ThinkingContent) Validate() error {
	if content.Redacted && content.ThinkingSignature == "" {
		return errors.New("redacted thinking content requires a signature")
	}
	return nil
}

// ImageContent 表示以 base64 编码传递的图片附件。
type ImageContent struct {
	// Data 是不含 data URL 前缀的 base64 图片数据。
	Data string
	// MIMEType 是图片的媒体类型，例如 image/png。
	MIMEType string
}

// ContentType 返回图片内容类型。
func (ImageContent) ContentType() ContentType { return ContentTypeImage }

// Validate 检查图片内容块。
func (content ImageContent) Validate() error {
	if content.Data == "" {
		return errors.New("image data is required")
	}
	if content.MIMEType == "" {
		return errors.New("image MIME type is required")
	}
	return nil
}

// ToolCall 表示助手发起的一次工具调用。
type ToolCall struct {
	// ID 是供应商分配的调用标识，用于关联后续工具结果。
	ID string
	// Name 是要调用的工具名称。
	Name string
	// Arguments 是模型增量拼接完成后的 JSON 参数对象。
	Arguments json.RawMessage
	// Custom 为 true 时 Arguments 不是 JSON 对象而是供应商原文
	//（Devin invalid_json_str/is_custom_tool_call：custom/freeform 工具
	// 的参数体本来就不是 JSON，如 apply_patch 的补丁文本）。请求方向
	// 客户端回灌的畸形 JSON 参数也按此保留原文，不吞成 {}。
	Custom bool
	// ThoughtSignature 是部分供应商附加到工具调用上的思考签名，重放时应原样保留。
	// 当前无生产者：Devin 上游不在工具调用上带签名，为 Gemini/Anthropic
	// 的 tool_use 签名形态预留。
	ThoughtSignature string
}

// ContentType 返回工具调用内容类型。
func (ToolCall) ContentType() ContentType { return ContentTypeToolCall }

// Validate 检查工具调用。
func (call ToolCall) Validate() error {
	if call.ID == "" {
		return errors.New("tool call ID is required")
	}
	if call.Name == "" {
		return errors.New("tool call name is required")
	}
	if call.Custom {
		return nil
	}
	if !IsJSONObject(call.Arguments) {
		return errors.New("tool call arguments must be a JSON object")
	}
	return nil
}

// UserMessage 表示一条用户消息。
type UserMessage struct {
	// Content 是用户提交的文字和图片内容块。
	Content []Content
	// TimestampMS 是创建消息时的 Unix 毫秒时间戳。
	TimestampMS int64
}

// Role 返回用户角色。
func (UserMessage) Role() MessageRole { return MessageRoleUser }

// Validate 检查用户消息。
func (message UserMessage) Validate() error {
	return validateContent(message.Content, ContentTypeText, ContentTypeImage)
}

// ToolResultMessage 表示一次工具调用的执行结果。
type ToolResultMessage struct {
	// ToolCallID 是本结果所对应的工具调用标识。
	ToolCallID string
	// ToolName 是被执行的工具名称。
	ToolName string
	// Content 是返回给模型的文字和图片内容块。
	Content []Content
	// Details 是仅供应用层保存和展示、不一定发送给模型的结构化详情。
	// 当前无生产者：三个协议解码器都不填，为携带结构化结果的工具协议预留。
	Details json.RawMessage
	// Usage 是执行工具本身产生的可选用量，不计入助手响应主用量。
	// 当前无生产者：解码器不填。
	Usage *Usage
	// AddedToolNames 是本次结果触发延迟加载后新增的工具名称。
	// 当前无生产者：解码器不填。
	AddedToolNames []string
	// IsError 表示工具执行是否失败。
	IsError bool
	// TimestampMS 是创建消息时的 Unix 毫秒时间戳。
	TimestampMS int64
}

// Role 返回工具结果角色。
func (ToolResultMessage) Role() MessageRole { return MessageRoleToolResult }

// Validate 检查工具结果消息。
func (message ToolResultMessage) Validate() error {
	if message.ToolCallID == "" {
		return errors.New("tool result call ID is required")
	}
	if message.ToolName == "" {
		return errors.New("tool result name is required")
	}
	if err := validateContent(message.Content, ContentTypeText, ContentTypeImage); err != nil {
		return err
	}
	if len(message.Details) > 0 && !json.Valid(message.Details) {
		return errors.New("tool result details must be valid JSON")
	}
	if message.Usage != nil {
		return message.Usage.Validate()
	}
	return nil
}

// ToolDefinition 定义模型可以调用的一个工具。
type ToolDefinition struct {
	// Name 是工具的稳定名称。
	Name string
	// Description 是提供给模型的工具用途说明。
	Description string
	// InputSchema 是描述工具输入对象的 JSON Schema。
	InputSchema json.RawMessage
	// Custom 表示客户端按 freeform/custom 语义声明的工具（Codex apply_patch）：
	// 参数体是原文而非 JSON。上游 is_custom_tool 声明通道实测确定性 unknown，
	// 这类工具在 wire 上包装成单字符串参数的 function 声明（InputSchema 即
	// 包装 schema），响应侧按此标记把 {"input":"<原文>"} 解包回原文。
	Custom bool
}

// toolNameCharset 是上游实测接受的工具名字符集（a.b、mcp::x、中文名
// 均被拒且只回模糊 internal error）。本地校验把这类失败变成可读的 400。
var toolNameCharset = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Validate 检查工具定义。
func (tool ToolDefinition) Validate() error {
	if tool.Name == "" {
		return errors.New("tool name is required")
	}
	if !toolNameCharset.MatchString(tool.Name) {
		return fmt.Errorf("tool name %q contains characters outside [A-Za-z0-9_-] which upstream rejects", tool.Name)
	}
	if !IsJSONObject(tool.InputSchema) {
		return errors.New("tool input schema must be a JSON object")
	}
	return nil
}

// Validate 检查完整请求上下文。
func (request RequestMessages) Validate() error {
	for index, message := range request.Messages {
		if message == nil {
			return fmt.Errorf("message %d is nil", index)
		}
		if err := message.Validate(); err != nil {
			return fmt.Errorf("message %d (%s): %w", index, message.Role(), err)
		}
	}
	for index, tool := range request.Tools {
		if err := tool.Validate(); err != nil {
			return fmt.Errorf("tool %d: %w", index, err)
		}
	}
	if request.ToolChoice != nil {
		switch request.ToolChoice.Mode {
		case ToolChoiceAuto, ToolChoiceNone, ToolChoiceRequired:
		case ToolChoiceNamed:
			if request.ToolChoice.ToolName == "" {
				return errors.New("tool_choice named mode requires a tool name")
			}
		default:
			return fmt.Errorf("invalid tool_choice mode %q", request.ToolChoice.Mode)
		}
	}
	return nil
}

func validateContent(content []Content, allowed ...ContentType) error {
	allowedTypes := make(map[ContentType]struct{}, len(allowed))
	for _, contentType := range allowed {
		allowedTypes[contentType] = struct{}{}
	}
	for index, block := range content {
		if block == nil {
			return fmt.Errorf("content block %d is nil", index)
		}
		if _, ok := allowedTypes[block.ContentType()]; !ok {
			return fmt.Errorf("content block %d has disallowed type %q", index, block.ContentType())
		}
		if err := block.Validate(); err != nil {
			return fmt.Errorf("content block %d (%s): %w", index, block.ContentType(), err)
		}
	}
	return nil
}

// IsJSONObject 判定 value 是否为 JSON 对象（{} 含）。Unmarshal 自带完整
// 语法校验，无需前置 json.Valid；非对象/非法文本/null 均返回 false。
// 各协议前端与适配器共用的参数体检定。
func IsJSONObject(value json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(value, &object) == nil && object != nil
}

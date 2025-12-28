package antigravity

import "encoding/json"

// Claude 请求/响应类型定义

// ClaudeRequest Claude Messages API 请求
type ClaudeRequest struct {
	Model       string          `json:"model"`
	Messages    []ClaudeMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	System      json.RawMessage `json:"system,omitempty"` // string 或 []SystemBlock
	Stream      bool            `json:"stream,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	TopK        *int            `json:"top_k,omitempty"`
	Tools       []ClaudeTool    `json:"tools,omitempty"`
	Thinking    *ThinkingConfig `json:"thinking,omitempty"`
	Metadata    *ClaudeMetadata `json:"metadata,omitempty"`
REDACTED

// ClaudeMessage Claude 消息
type ClaudeMessage struct {
	Role    string          `json:"role"` // user, assistant
	Content json.RawMessage `json:"content"`
REDACTED

// ThinkingConfig Thinking 配置
type ThinkingConfig struct {
	Type         string `json:"type"`                    // "enabled" or "disabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // thinking budget
REDACTED

// ClaudeMetadata 请求元数据
type ClaudeMetadata struct {
	UserID string `json:"user_id,omitempty"`
REDACTED

// ClaudeTool Claude 工具定义
type ClaudeTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{REDACTED `json:"input_schema"`
REDACTED

// SystemBlock system prompt 数组形式的元素
type SystemBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
REDACTED

// ContentBlock Claude 消息内容块（解析后）
type ContentBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// thinking
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	// tool_use
	ID    string      `json:"id,omitempty"`
	Name  string      `json:"name,omitempty"`
	Input interface{REDACTED `json:"input,omitempty"`
	// tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	// image
	Source *ImageSource `json:"source,omitempty"`
REDACTED

// ImageSource Claude 图片来源
type ImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png", "image/jpeg" 等
	Data      string `json:"data"`
REDACTED

// ClaudeResponse Claude Messages API 响应
type ClaudeResponse struct {
	ID           string              `json:"id"`
	Type         string              `json:"type"` // "message"
	Role         string              `json:"role"` // "assistant"
	Model        string              `json:"model"`
	Content      []ClaudeContentItem `json:"content"`
	StopReason   string              `json:"stop_reason,omitempty"`   // end_turn, tool_use, max_tokens
	StopSequence *string             `json:"stop_sequence,omitempty"` // null 或具体值
	Usage        ClaudeUsage         `json:"usage"`
REDACTED

// ClaudeContentItem Claude 响应内容项
type ClaudeContentItem struct {
	Type string `json:"type"` // text, thinking, tool_use

	// text
	Text string `json:"text,omitempty"`

	// thinking
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`

	// tool_use
	ID    string      `json:"id,omitempty"`
	Name  string      `json:"name,omitempty"`
	Input interface{REDACTED `json:"input,omitempty"`
REDACTED

// ClaudeUsage Claude 用量统计
type ClaudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
REDACTED

// ClaudeError Claude 错误响应
type ClaudeError struct {
	Type  string      `json:"type"` // "error"
	Error ErrorDetail `json:"error"`
REDACTED

// ErrorDetail 错误详情
type ErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
REDACTED

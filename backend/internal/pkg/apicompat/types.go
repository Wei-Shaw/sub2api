// Package apicompat provides type definitions and conversion utilities for
// translating between Anthropic Messages, OpenAI Chat Completions, and OpenAI
// Responses API formats. It enables multi-protocol support so that clients
// using different API formats can be served through a unified gateway.
package apicompat

import "encoding/json"

// ---------------------------------------------------------------------------
// Anthropic Messages API types
// ---------------------------------------------------------------------------

// AnthropicRequest is the request body for POST /v1/messages.
type AnthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      json.RawMessage    `json:"system,omitempty"` // string or []AnthropicContentBlock
	Messages    []AnthropicMessage `json:"messages"`
	Tools       []AnthropicTool    `json:"tools,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	StopSeqs    []string           `json:"stop_sequences,omitempty"`
REDACTED

// AnthropicMessage is a single message in the Anthropic conversation.
type AnthropicMessage struct {
	Role    string          `json:"role"` // "user" | "assistant"
	Content json.RawMessage `json:"content"`
REDACTED

// AnthropicContentBlock is one block inside a message's content array.
type AnthropicContentBlock struct {
	Type string `json:"type"`

	// type=text
	Text string `json:"text,omitempty"`

	// type=thinking
	Thinking string `json:"thinking,omitempty"`

	// type=tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// type=tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // string or []AnthropicContentBlock
	IsError   bool            `json:"is_error,omitempty"`
REDACTED

// AnthropicTool describes a tool available to the model.
type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema object
REDACTED

// AnthropicResponse is the non-streaming response from POST /v1/messages.
type AnthropicResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"` // "message"
	Role         string                  `json:"role"` // "assistant"
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason"`
	StopSequence *string                 `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage          `json:"usage"`
REDACTED

// AnthropicUsage holds token counts in Anthropic format.
type AnthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
REDACTED

// ---------------------------------------------------------------------------
// Anthropic SSE event types
// ---------------------------------------------------------------------------

// AnthropicStreamEvent is a single SSE event in the Anthropic streaming protocol.
type AnthropicStreamEvent struct {
	Type string `json:"type"`

	// message_start
	Message *AnthropicResponse `json:"message,omitempty"`

	// content_block_start
	Index        *int                   `json:"index,omitempty"`
	ContentBlock *AnthropicContentBlock `json:"content_block,omitempty"`

	// content_block_delta
	Delta *AnthropicDelta `json:"delta,omitempty"`

	// message_delta
	Usage *AnthropicUsage `json:"usage,omitempty"`
REDACTED

// AnthropicDelta carries incremental content in streaming events.
type AnthropicDelta struct {
	Type string `json:"type,omitempty"` // "text_delta" | "input_json_delta" | "thinking_delta" | "signature_delta"

	// text_delta
	Text string `json:"text,omitempty"`

	// input_json_delta
	PartialJSON string `json:"partial_json,omitempty"`

	// thinking_delta
	Thinking string `json:"thinking,omitempty"`

	// signature_delta
	Signature string `json:"signature,omitempty"`

	// message_delta fields
	StopReason   string  `json:"stop_reason,omitempty"`
	StopSequence *string `json:"stop_sequence,omitempty"`
REDACTED

// ---------------------------------------------------------------------------
// OpenAI Chat Completions API types
// ---------------------------------------------------------------------------

// ChatRequest is the request body for POST /v1/chat/completions.
type ChatRequest struct {
	Model       string          `json:"model"`
	Messages    []ChatMessage   `json:"messages"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Tools       []ChatTool      `json:"tools,omitempty"`
	Stop        json.RawMessage `json:"stop,omitempty"` // string or []string
REDACTED

// ChatMessage is a single message in the Chat Completions conversation.
type ChatMessage struct {
	Role    string          `json:"role"`              // "system" | "user" | "assistant" | "tool"
	Content json.RawMessage `json:"content,omitempty"` // string or []ChatContentPart

	// assistant fields
	ToolCalls []ChatToolCall `json:"tool_calls,omitempty"`

	// tool fields
	ToolCallID string `json:"tool_call_id,omitempty"`

	// Copilot-specific reasoning passthrough
	ReasoningText   string `json:"reasoning_text,omitempty"`
	ReasoningOpaque string `json:"reasoning_opaque,omitempty"`
REDACTED

// ChatContentPart is a typed content part in a multi-part message.
type ChatContentPart struct {
	Type string `json:"type"` // "text" | "image_url"
	Text string `json:"text,omitempty"`
REDACTED

// ChatToolCall represents a tool invocation in an assistant message.
// In streaming deltas, Index identifies which tool call is being updated.
type ChatToolCall struct {
	Index    int              `json:"index"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"` // "function"
	Function ChatFunctionCall `json:"function"`
REDACTED

// ChatFunctionCall holds the function name and arguments.
type ChatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
REDACTED

// ChatTool describes a tool available to the model.
type ChatTool struct {
	Type     string       `json:"type"` // "function"
	Function ChatFunction `json:"function"`
REDACTED

// ChatFunction is the function definition inside a ChatTool.
type ChatFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema
REDACTED

// ChatResponse is the non-streaming response from POST /v1/chat/completions.
type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"` // "chat.completion"
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   *ChatUsage   `json:"usage,omitempty"`
REDACTED

// ChatChoice is one completion choice.
type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
REDACTED

// ChatUsage holds token counts in Chat Completions format.
type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
REDACTED

// ---------------------------------------------------------------------------
// Chat Completions SSE types
// ---------------------------------------------------------------------------

// ChatStreamChunk is a single SSE chunk in the Chat Completions streaming protocol.
type ChatStreamChunk struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"` // "chat.completion.chunk"
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []ChatStreamChoice `json:"choices"`
	Usage   *ChatUsage         `json:"usage,omitempty"`
REDACTED

// ChatStreamChoice is one choice inside a streaming chunk.
type ChatStreamChoice struct {
	Index        int             `json:"index"`
	Delta        ChatStreamDelta `json:"delta"`
	FinishReason *string         `json:"finish_reason"`
REDACTED

// ChatStreamDelta carries incremental content in a streaming chunk.
type ChatStreamDelta struct {
	Role      string         `json:"role,omitempty"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []ChatToolCall `json:"tool_calls,omitempty"`

	// Copilot-specific reasoning passthrough (streaming)
	ReasoningText   string `json:"reasoning_text,omitempty"`
	ReasoningOpaque string `json:"reasoning_opaque,omitempty"`
REDACTED

// ---------------------------------------------------------------------------
// OpenAI Responses API types
// ---------------------------------------------------------------------------

// ResponsesRequest is the request body for POST /v1/responses.
type ResponsesRequest struct {
	Model           string          `json:"model"`
	Input           json.RawMessage `json:"input"` // string or []ResponsesInputItem
	MaxOutputTokens *int            `json:"max_output_tokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
	Tools           []ResponsesTool `json:"tools,omitempty"`
	Include         []string        `json:"include,omitempty"`
	Store           *bool           `json:"store,omitempty"`
REDACTED

// ResponsesInputItem is one item in the Responses API input array.
// The Type field determines which other fields are populated.
type ResponsesInputItem struct {
	// Common
	Type string `json:"type,omitempty"` // "" for role-based messages

	// Role-based messages (system/user/assistant)
	Role    string          `json:"role,omitempty"`
	Content json.RawMessage `json:"content,omitempty"` // string or []ResponsesContentPart

	// type=function_call
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	ID        string `json:"id,omitempty"`

	// type=function_call_output
	Output string `json:"output,omitempty"`
REDACTED

// ResponsesContentPart is a typed content part in a Responses message.
type ResponsesContentPart struct {
	Type string `json:"type"` // "input_text" | "output_text" | "input_image"
	Text string `json:"text,omitempty"`
REDACTED

// ResponsesTool describes a tool in the Responses API.
type ResponsesTool struct {
	Type        string          `json:"type"` // "function" | "web_search" | "local_shell" etc.
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
REDACTED

// ResponsesResponse is the non-streaming response from POST /v1/responses.
type ResponsesResponse struct {
	ID     string            `json:"id"`
	Object string            `json:"object"` // "response"
	Model  string            `json:"model"`
	Status string            `json:"status"` // "completed" | "incomplete" | "failed"
	Output []ResponsesOutput `json:"output"`
	Usage  *ResponsesUsage   `json:"usage,omitempty"`

	// incomplete_details is present when status="incomplete"
	IncompleteDetails *ResponsesIncompleteDetails `json:"incomplete_details,omitempty"`
REDACTED

// ResponsesIncompleteDetails explains why a response is incomplete.
type ResponsesIncompleteDetails struct {
	Reason string `json:"reason"` // "max_output_tokens" | "content_filter"
REDACTED

// ResponsesOutput is one output item in a Responses API response.
type ResponsesOutput struct {
	Type string `json:"type"` // "message" | "reasoning" | "function_call"

	// type=message
	ID      string                 `json:"id,omitempty"`
	Role    string                 `json:"role,omitempty"`
	Content []ResponsesContentPart `json:"content,omitempty"`
	Status  string                 `json:"status,omitempty"`

	// type=reasoning
	EncryptedContent string             `json:"encrypted_content,omitempty"`
	Summary          []ResponsesSummary `json:"summary,omitempty"`

	// type=function_call
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
REDACTED

// ResponsesSummary is a summary text block inside a reasoning output.
type ResponsesSummary struct {
	Type string `json:"type"` // "summary_text"
	Text string `json:"text"`
REDACTED

// ResponsesUsage holds token counts in Responses API format.
type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`

	// Optional detailed breakdown
	InputTokensDetails  *ResponsesInputTokensDetails  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *ResponsesOutputTokensDetails `json:"output_tokens_details,omitempty"`
REDACTED

// ---------------------------------------------------------------------------
// Responses SSE event types
// ---------------------------------------------------------------------------

// ResponsesStreamEvent is a single SSE event in the Responses streaming protocol.
// The Type field corresponds to the "type" in the JSON payload.
type ResponsesStreamEvent struct {
	Type string `json:"type"`

	// response.created / response.completed / response.failed / response.incomplete
	Response *ResponsesResponse `json:"response,omitempty"`

	// response.output_item.added / response.output_item.done
	Item *ResponsesOutput `json:"item,omitempty"`

	// response.output_text.delta / response.output_text.done
	OutputIndex  int    `json:"output_index,omitempty"`
	ContentIndex int    `json:"content_index,omitempty"`
	Delta        string `json:"delta,omitempty"`
	Text         string `json:"text,omitempty"`
	ItemID       string `json:"item_id,omitempty"`

	// response.function_call_arguments.delta / done
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// response.reasoning_summary_text.delta / done
	// Reuses Text/Delta fields above, SummaryIndex identifies which summary part
	SummaryIndex int `json:"summary_index,omitempty"`

	// error event fields
	Code  string `json:"code,omitempty"`
	Param string `json:"param,omitempty"`

	// Sequence number for ordering events
	SequenceNumber int `json:"sequence_number,omitempty"`
REDACTED

// ResponsesOutputReasoning is a reasoning output item in the Responses API.
// This type represents the "type":"reasoning" output item that contains
// extended thinking from the model.
type ResponsesOutputReasoning struct {
	ID               string                      `json:"id,omitempty"`
	Type             string                      `json:"type"`             // "reasoning"
	Status           string                      `json:"status,omitempty"` // "in_progress" | "completed" | "incomplete"
	EncryptedContent string                      `json:"encrypted_content,omitempty"`
	Summary          []ResponsesReasoningSummary `json:"summary,omitempty"`
REDACTED

// ResponsesReasoningSummary is a summary text block inside a reasoning output.
type ResponsesReasoningSummary struct {
	Type string `json:"type"` // "summary_text"
	Text string `json:"text"`
REDACTED

// ResponsesStreamState maintains the state for converting Responses streaming
// events to Chat Completions format. It tracks content blocks, tool calls,
// reasoning blocks, and other streaming artifacts.
type ResponsesStreamState struct {
	// Response metadata
	ID      string
	Model   string
	Created int64

	// Content tracking
	ContentIndex  int
	CurrentText   string
	CurrentItemID string
	PendingText   []string // Text to accumulate before emitting

	// Tool call tracking
	ToolCalls       []ResponsesToolCallState
	CurrentToolCall *ResponsesToolCallState

	// Reasoning tracking
	ReasoningBlocks  []ResponsesReasoningState
	CurrentReasoning *ResponsesReasoningState

	// Usage tracking
	InputTokens  int
	OutputTokens int

	// Status tracking
	Status       string
	FinishReason string
REDACTED

// ResponsesToolCallState tracks a single tool call during streaming.
type ResponsesToolCallState struct {
	Index      int
	ItemID     string
	CallID     string
	Name       string
	Arguments  string
	Status     string
	IsComplete bool
REDACTED

// ResponsesReasoningState tracks a reasoning block during streaming.
type ResponsesReasoningState struct {
	ItemID       string
	SummaryIndex int
	SummaryText  string
	Status       string
	IsComplete   bool
REDACTED

// ResponsesUsageDetail provides additional token usage details in Responses format.
type ResponsesUsageDetail struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`

	// Optional detailed breakdown
	InputTokensDetails  *ResponsesInputTokensDetails  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *ResponsesOutputTokensDetails `json:"output_tokens_details,omitempty"`
REDACTED

// ResponsesInputTokensDetails breaks down input token usage.
type ResponsesInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
REDACTED

// ResponsesOutputTokensDetails breaks down output token usage.
type ResponsesOutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
REDACTED

// ---------------------------------------------------------------------------
// Finish reason mapping helpers
// ---------------------------------------------------------------------------

// ChatFinishToAnthropic maps a Chat Completions finish_reason to an Anthropic stop_reason.
func ChatFinishToAnthropic(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return "end_turn"
REDACTED
REDACTED

// AnthropicStopToChat maps an Anthropic stop_reason to a Chat Completions finish_reason.
func AnthropicStopToChat(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	default:
		return "stop"
REDACTED
REDACTED

// ResponsesStatusToChat maps a Responses API status to a Chat Completions finish_reason.
func ResponsesStatusToChat(status string, details *ResponsesIncompleteDetails) string {
	switch status {
	case "completed":
		return "stop"
	case "incomplete":
		if details != nil && details.Reason == "max_output_tokens" {
			return "length"
	REDACTED
		return "stop"
	default:
		return "stop"
REDACTED
REDACTED

// ChatFinishToResponsesStatus maps a Chat Completions finish_reason to a Responses status.
func ChatFinishToResponsesStatus(reason string) string {
	switch reason {
	case "length":
		return "incomplete"
	default:
		return "completed"
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// Shared constants
// ---------------------------------------------------------------------------

// minMaxOutputTokens is the floor for max_output_tokens in a Responses request.
// Very small values may cause upstream API errors, so we enforce a minimum.
const minMaxOutputTokens = 128

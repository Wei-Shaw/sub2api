// Package apicompat provides type definitions and conversion utilities for
// translating between Anthropic Messages and OpenAI Responses API formats.
// It enables multi-protocol support so that clients using different API
// formats can be served through a unified gateway.
package apicompat

import "encoding/json"

// ---------------------------------------------------------------------------
// Anthropic Messages API types
// ---------------------------------------------------------------------------

// AnthropicRequest is the request body for POST /v1/messages.
type AnthropicRequest struct {
	Model        string                 `json:"model"`
	MaxTokens    int                    `json:"max_tokens"`
	System       json.RawMessage        `json:"system,omitempty"` // string or []AnthropicContentBlock
	Messages     []AnthropicMessage     `json:"messages"`
	Tools        []AnthropicTool        `json:"tools,omitempty"`
	Stream       bool                   `json:"stream,omitempty"`
	Temperature  *float64               `json:"temperature,omitempty"`
	TopP         *float64               `json:"top_p,omitempty"`
	StopSeqs     []string               `json:"stop_sequences,omitempty"`
	Thinking     *AnthropicThinking     `json:"thinking,omitempty"`
	ToolChoice   json.RawMessage        `json:"tool_choice,omitempty"`
	OutputConfig *AnthropicOutputConfig `json:"output_config,omitempty"`
}

// AnthropicOutputConfig controls output generation parameters.
type AnthropicOutputConfig struct {
	Effort string `json:"effort,omitempty"` // "low" | "medium" | "high" | "max"
}

// AnthropicThinking configures extended thinking in the Anthropic API.
type AnthropicThinking struct {
	Type         string `json:"type"`                    // "enabled" | "adaptive" | "disabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // max thinking tokens
}

// AnthropicMessage is a single message in the Anthropic conversation.
type AnthropicMessage struct {
	Role    string          `json:"role"` // "user" | "assistant"
	Content json.RawMessage `json:"content"`
}

// AnthropicContentBlock is one block inside a message's content array.
type AnthropicContentBlock struct {
	Type string `json:"type"`

	// type=text
	Text string `json:"text,omitempty"`

	// type=thinking
	Thinking string `json:"thinking,omitempty"`

	// type=image
	Source *AnthropicImageSource `json:"source,omitempty"`

	// type=tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// type=tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // string or []AnthropicContentBlock
	IsError   bool            `json:"is_error,omitempty"`
}

// AnthropicImageSource describes the source data for an image content block.
type AnthropicImageSource struct {
	Type      string `json:"type"` // "base64"
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// AnthropicTool describes a tool available to the model.
type AnthropicTool struct {
	Type        string          `json:"type,omitempty"` // e.g. "web_search_20250305" for server tools
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema object
}

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
}

// AnthropicUsage holds token counts in Anthropic format.
type AnthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

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
}

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
}

// ---------------------------------------------------------------------------
// OpenAI Responses API types
// ---------------------------------------------------------------------------

// ResponsesRequest is the request body for POST /v1/responses.
type ResponsesRequest struct {
	Model           string              `json:"model"`
	Instructions    string              `json:"instructions,omitempty"`
	Input           json.RawMessage     `json:"input"` // string or []ResponsesInputItem
	MaxOutputTokens *int                `json:"max_output_tokens,omitempty"`
	Temperature     *float64            `json:"temperature,omitempty"`
	TopP            *float64            `json:"top_p,omitempty"`
	Stream          bool                `json:"stream,omitempty"`
	Tools           []ResponsesTool     `json:"tools,omitempty"`
	Include         []string            `json:"include,omitempty"`
	Store           *bool               `json:"store,omitempty"`
	Reasoning       *ResponsesReasoning `json:"reasoning,omitempty"`
	ToolChoice      json.RawMessage     `json:"tool_choice,omitempty"`
	ServiceTier     string              `json:"service_tier,omitempty"`
}

// ResponsesReasoning configures reasoning effort in the Responses API.
type ResponsesReasoning struct {
	Effort  string `json:"effort"`            // "low" | "medium" | "high" | "xhigh"
	Summary string `json:"summary,omitempty"` // "auto" | "concise" | "detailed"
}

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
}

// ResponsesContentPart is a typed content part in a Responses message.
type ResponsesContentPart struct {
	Type     string `json:"type"` // "input_text" | "output_text" | "input_image"
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"` // data URI for input_image
}

// ResponsesTool describes a tool in the Responses API.
type ResponsesTool struct {
	Type        string          `json:"type"` // "function" | "web_search" | "local_shell" etc.
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

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

	// Error is present when status="failed"
	Error *ResponsesError `json:"error,omitempty"`
}

// ResponsesError describes an error in a failed response.
type ResponsesError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ResponsesIncompleteDetails explains why a response is incomplete.
type ResponsesIncompleteDetails struct {
	Reason string `json:"reason"` // "max_output_tokens" | "content_filter"
}

// ResponsesOutput is one output item in a Responses API response.
type ResponsesOutput struct {
	Type string `json:"type"` // "message" | "reasoning" | "function_call" | "web_search_call"

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

	// type=web_search_call
	Action *WebSearchAction `json:"action,omitempty"`
}

// WebSearchAction describes the search action in a web_search_call output item.
type WebSearchAction struct {
	Type  string `json:"type,omitempty"`  // "search"
	Query string `json:"query,omitempty"` // primary search query
}

// ResponsesSummary is a summary text block inside a reasoning output.
type ResponsesSummary struct {
	Type string `json:"type"` // "summary_text"
	Text string `json:"text"`
}

// ResponsesUsage holds token counts in Responses API format.
type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`

	// Optional detailed breakdown
	InputTokensDetails  *ResponsesInputTokensDetails  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *ResponsesOutputTokensDetails `json:"output_tokens_details,omitempty"`
}

// ResponsesInputTokensDetails breaks down input token usage.
type ResponsesInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// ResponsesOutputTokensDetails breaks down output token usage.
type ResponsesOutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

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
}

// ---------------------------------------------------------------------------
// OpenAI Chat Completions API types
// ---------------------------------------------------------------------------

// ChatCompletionsRequest is the request body for POST /v1/chat/completions.
type ChatCompletionsRequest struct {
	Model               string             `json:"model"`
	Messages            []ChatMessage      `json:"messages"`
	Instructions        string             `json:"instructions,omitempty"` // OpenAI Responses API compat
	MaxTokens           *int               `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int               `json:"max_completion_tokens,omitempty"`
	Temperature         *float64           `json:"temperature,omitempty"`
	TopP                *float64           `json:"top_p,omitempty"`
	Stream              bool               `json:"stream,omitempty"`
	StreamOptions       *ChatStreamOptions `json:"stream_options,omitempty"`
	Tools               []ChatTool         `json:"tools,omitempty"`
	ToolChoice          json.RawMessage    `json:"tool_choice,omitempty"`
	ReasoningEffort     string             `json:"reasoning_effort,omitempty"` // "low" | "medium" | "high" | "xhigh"
	ServiceTier         string             `json:"service_tier,omitempty"`
	Stop                json.RawMessage    `json:"stop,omitempty"` // string or []string

	// Legacy function calling (deprecated but still supported)
	Functions    []ChatFunction  `json:"functions,omitempty"`
	FunctionCall json.RawMessage `json:"function_call,omitempty"`
}

// ChatStreamOptions configures streaming behavior.
type ChatStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ChatMessage is a single message in the Chat Completions conversation.
type ChatMessage struct {
	Role             string          `json:"role"` // "system" | "user" | "assistant" | "tool" | "function"
	Content          json.RawMessage `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	Name             string          `json:"name,omitempty"`
	ToolCalls        []ChatToolCall  `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`

	// Legacy function calling
	FunctionCall *ChatFunctionCall `json:"function_call,omitempty"`
}

// ChatContentPart is a typed content part in a multi-modal message.
type ChatContentPart struct {
	Type     string        `json:"type"` // "text" | "image_url"
	Text     string        `json:"text,omitempty"`
	ImageURL *ChatImageURL `json:"image_url,omitempty"`
}

// ChatImageURL contains the URL for an image content part.
type ChatImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto" | "low" | "high"
}

// ChatTool describes a tool available to the model.
type ChatTool struct {
	Type     string        `json:"type"` // "function"
	Function *ChatFunction `json:"function,omitempty"`
}

// ChatFunction describes a function tool definition.
type ChatFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

// ChatToolCall represents a tool call made by the assistant.
// Index is only populated in streaming chunks (omitted in non-streaming responses).
type ChatToolCall struct {
	Index    *int             `json:"index,omitempty"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"` // "function"
	Function ChatFunctionCall `json:"function"`
}

// ChatFunctionCall contains the function name and arguments.
type ChatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatCompletionsResponse is the non-streaming response from POST /v1/chat/completions.
type ChatCompletionsResponse struct {
	ID                string       `json:"id"`
	Object            string       `json:"object"` // "chat.completion"
	Created           int64        `json:"created"`
	Model             string       `json:"model"`
	Choices           []ChatChoice `json:"choices"`
	Usage             *ChatUsage   `json:"usage,omitempty"`
	SystemFingerprint string       `json:"system_fingerprint,omitempty"`
	ServiceTier       string       `json:"service_tier,omitempty"`
}

// ChatChoice is a single completion choice.
type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"` // "stop" | "length" | "tool_calls" | "content_filter"
}

// ChatUsage holds token counts in Chat Completions format.
type ChatUsage struct {
	PromptTokens        int               `json:"prompt_tokens"`
	CompletionTokens    int               `json:"completion_tokens"`
	TotalTokens         int               `json:"total_tokens"`
	PromptTokensDetails *ChatTokenDetails `json:"prompt_tokens_details,omitempty"`
}

// ChatTokenDetails provides a breakdown of token usage.
type ChatTokenDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// ChatCompletionsChunk is a single streaming chunk from POST /v1/chat/completions.
type ChatCompletionsChunk struct {
	ID                string            `json:"id"`
	Object            string            `json:"object"` // "chat.completion.chunk"
	Created           int64             `json:"created"`
	Model             string            `json:"model"`
	Choices           []ChatChunkChoice `json:"choices"`
	Usage             *ChatUsage        `json:"usage,omitempty"`
	SystemFingerprint string            `json:"system_fingerprint,omitempty"`
	ServiceTier       string            `json:"service_tier,omitempty"`
}

// ChatChunkChoice is a single choice in a streaming chunk.
type ChatChunkChoice struct {
	Index        int       `json:"index"`
	Delta        ChatDelta `json:"delta"`
	FinishReason *string   `json:"finish_reason"` // pointer: null when not final
}

// ChatDelta carries incremental content in a streaming chunk.
type ChatDelta struct {
	Role             string         `json:"role,omitempty"`
	Content          *string        `json:"content,omitempty"` // pointer: omit when not present, null vs "" matters
	ReasoningContent *string        `json:"reasoning_content,omitempty"`
	ToolCalls        []ChatToolCall `json:"tool_calls,omitempty"`
}

// ---------------------------------------------------------------------------
// Shared constants
// ---------------------------------------------------------------------------

// minMaxOutputTokens is the floor for max_output_tokens in a Responses request.
// Very small values may cause upstream API errors, so we enforce a minimum.
const minMaxOutputTokens = 128

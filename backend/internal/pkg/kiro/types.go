// Package kiro contains provider-agnostic helpers for interacting with the
// Kiro / CodeWhisperer upstream. Mirrors the layout of internal/pkg/antigravity.
//
// This package is the lowest layer of the Kiro integration: it knows about
// HTTP, Kiro's OAuth endpoints, and the upstream wire format, but nothing
// about the DB, Gin, or our internal Account model. The internal/service
// layer wires this up to the rest of the system.
package kiro

// AuthMethod identifies how a Kiro account was authenticated.
// Stored on account.credentials.auth_method and used by RefreshToken to
// pick the correct refresh endpoint.
type AuthMethod string

const (
	AuthMethodSocial    AuthMethod = "social"
	AuthMethodIdC       AuthMethod = "idc"
	AuthMethodBuilderID AuthMethod = "builderid"
)

// TokenInfo is the canonical token shape returned by all three auth flows.
// All times are Unix seconds.
type TokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	ProfileARN   string `json:"profile_arn,omitempty"`

	// AuthMethod identifies which refresh endpoint to use later.
	AuthMethod AuthMethod `json:"auth_method"`

	// IdC / BuilderID extras (empty for Social). Populated by phase 3.
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	Region       string `json:"region,omitempty"`
	StartURL     string `json:"start_url,omitempty"`

	// Profile data, populated post-token for naming the account.
	Email  string `json:"email,omitempty"`
	UserID string `json:"user_id,omitempty"`
}

// RefreshableAccount is the minimal account shape the refresh helpers need.
// Decouples pkg/kiro from internal/service.Account so this package can be
// imported by tests and tools without pulling in Ent.
type RefreshableAccount struct {
	AuthMethod   AuthMethod
	RefreshToken string
	ClientID     string
	ClientSecret string
	Region       string
	ProxyURL     string
}

// ==================== Upstream wire-format types ====================
// These structs marshal to the JSON body Kiro's generateAssistantResponse
// endpoint expects. Field names and shape are dictated by upstream — do
// not reorder or rename without verifying against the reference impl.

// Payload is the top-level request body posted to Kiro.
type Payload struct {
	ConversationState struct {
		AgentContinuationID string             `json:"agentContinuationId,omitempty"`
		AgentTaskType       string             `json:"agentTaskType,omitempty"`
		ChatTriggerType     string             `json:"chatTriggerType"`
		ConversationID      string             `json:"conversationId"`
		CurrentMessage      CurrentMessage     `json:"currentMessage"`
		History             []HistoryMessage   `json:"history,omitempty"`
	} `json:"conversationState"`
	ProfileARN      string           `json:"profileArn,omitempty"`
	InferenceConfig *InferenceConfig `json:"inferenceConfig,omitempty"`

	// ToolNameMap maps sanitized tool names (sent to Kiro) back to the
	// original names supplied by the client. Used to restore the original
	// names in tool_use responses so the client can match them against its
	// own registry. Not serialized to the Kiro request body.
	ToolNameMap map[string]string `json:"-"`
}

// CurrentMessage wraps the user's most-recent message.
type CurrentMessage struct {
	UserInputMessage UserInputMessage `json:"userInputMessage"`
}

// UserInputMessage is the message body sent to Kiro.
type UserInputMessage struct {
	Content                 string                   `json:"content"`
	ModelID                 string                   `json:"modelId,omitempty"`
	Origin                  string                   `json:"origin"`
	Images                  []Image                  `json:"images,omitempty"`
	UserInputMessageContext *UserInputMessageContext `json:"userInputMessageContext,omitempty"`
}

// UserInputMessageContext carries tools and tool results alongside a message.
type UserInputMessageContext struct {
	Tools       []ToolWrapper `json:"tools,omitempty"`
	ToolResults []ToolResult  `json:"toolResults,omitempty"`
}

// ToolWrapper is Kiro's outer shape for a tool definition.
type ToolWrapper struct {
	ToolSpecification ToolSpecification `json:"toolSpecification"`
}

// ToolSpecification is the tool's name, description, and input schema.
type ToolSpecification struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema wraps a JSON-Schema-like input definition in Kiro's
// expected envelope.
type InputSchema struct {
	JSON any `json:"json"`
}

// ToolResult is the result of a previous tool call sent back as context.
type ToolResult struct {
	ToolUseID string         `json:"toolUseId"`
	Content   []ResultText   `json:"content"`
	Status    string         `json:"status"`
}

// ResultText is one chunk of textual tool-result content.
type ResultText struct {
	Text string `json:"text"`
}

// Image is an inline image attached to a message.
type Image struct {
	Format string `json:"format"`
	Source ImageSource `json:"source"`
}

// ImageSource wraps the base64-encoded image bytes.
type ImageSource struct {
	Bytes string `json:"bytes"`
}

// HistoryMessage is one entry in the conversation history. Exactly one
// of UserInputMessage / AssistantResponseMessage is set.
type HistoryMessage struct {
	UserInputMessage         *UserInputMessage         `json:"userInputMessage,omitempty"`
	AssistantResponseMessage *AssistantResponseMessage `json:"assistantResponseMessage,omitempty"`
}

// AssistantResponseMessage is the assistant's prior turn in history.
type AssistantResponseMessage struct {
	Content  string    `json:"content"`
	ToolUses []ToolUse `json:"toolUses,omitempty"`
}

// ToolUse is a structured tool invocation returned by the assistant.
type ToolUse struct {
	ToolUseID string         `json:"toolUseId"`
	Name      string         `json:"name"`
	Input     map[string]any `json:"input"`
}

// InferenceConfig carries the per-request sampling parameters.
type InferenceConfig struct {
	MaxTokens   int     `json:"maxTokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"topP,omitempty"`
}

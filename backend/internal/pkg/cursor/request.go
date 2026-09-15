package cursor

import (
	"runtime"
	"time"

	"github.com/google/uuid"
)

// Official aiserver.v1.StreamUnifiedChatRequest field numbers from Cursor 3.16.
const (
	fieldConversation    = 1
	fieldModelDetails    = 5
	fieldIsChat          = 22
	fieldConversationID  = 23
	fieldEnvironment     = 26
	fieldIsAgentic       = 27
	fieldIsHeadless      = 45
	fieldUnifiedMode     = 46
	fieldDisableTools    = 48
	fieldThinkingLevel   = 49
	fieldUnifiedModeName = 54

	fieldMsgText     = 1
	fieldMsgType     = 2
	fieldMsgBubbleID = 13

	fieldModelName = 1

	fieldEnvPlatform = 1
	fieldEnvArch     = 2
	fieldEnvRelease  = 3
	fieldEnvVersion  = 7
	fieldEnvOSType   = 9
	fieldEnvTimezone = 11

	fieldWithToolsChat = 1 // StreamUnifiedChatRequestWithTools.stream_unified_chat_request
)

// ConversationMessage.MessageType
const (
	MessageTypeHuman = 1
	MessageTypeAI    = 2
)

// StreamUnifiedChatRequest.UnifiedMode
const (
	UnifiedModeChat = 1
)

// ThinkingLevel constants.
const (
	ThinkingLevelUnspecified = 0
	ThinkingLevelMedium      = 1
	ThinkingLevelHigh        = 2
)

// ChatMessage is an OpenAI-compatible message to be translated to Cursor format.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// BuildChatRequest encodes StreamUnifiedChatRequest (used by StreamUnifiedChat).
func BuildChatRequest(messages []ChatMessage, model string, thinkingLevel int) []byte {
	return buildStreamUnifiedChatRequest(messages, model, thinkingLevel)
}

// BuildChatRequestWithTools wraps StreamUnifiedChatRequest for StreamUnifiedChatWithTools.
func BuildChatRequestWithTools(messages []ChatMessage, model string, thinkingLevel int) []byte {
	inner := buildStreamUnifiedChatRequest(messages, model, thinkingLevel)
	var outer ProtobufWriter
	outer.Bytes(fieldWithToolsChat, inner)
	return outer.Result()
}

func buildStreamUnifiedChatRequest(messages []ChatMessage, model string, thinkingLevel int) []byte {
	var req ProtobufWriter

	for _, m := range messages {
		msgType := MessageTypeHuman
		if m.Role == "assistant" {
			msgType = MessageTypeAI
		}
		var msg ProtobufWriter
		msg.String(fieldMsgText, m.Content)
		msg.Varint(fieldMsgType, msgType)
		msg.String(fieldMsgBubbleID, uuid.New().String())
		req.Bytes(fieldConversation, msg.Result())
	}

	var modelDetails ProtobufWriter
	modelDetails.String(fieldModelName, model)
	req.Bytes(fieldModelDetails, modelDetails.Result())

	req.Bool(fieldIsChat, true)
	req.String(fieldConversationID, uuid.New().String())

	var env ProtobufWriter
	env.String(fieldEnvPlatform, runtime.GOOS)
	env.String(fieldEnvArch, runtime.GOARCH)
	env.String(5, time.Now().Format(time.RFC3339))
	env.String(fieldEnvVersion, DefaultClientVersion)
	env.String(fieldEnvOSType, nodeOS())
	env.String(fieldEnvTimezone, clientTimezone())
	req.Bytes(fieldEnvironment, env.Result())

	req.Varint(fieldUnifiedMode, UnifiedModeChat)
	req.Bool(fieldDisableTools, true)
	if thinkingLevel != ThinkingLevelUnspecified {
		req.Varint(fieldThinkingLevel, thinkingLevel)
	}
	req.String(fieldUnifiedModeName, "Chat")

	return req.Result()
}

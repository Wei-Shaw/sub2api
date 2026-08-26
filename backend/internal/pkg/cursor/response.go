package cursor

import (
	"io"
	"unicode/utf8"
)

// Response field numbers.
const (
	fieldRespToolCall = 1
	fieldRespResponse = 2

	fieldChatRespText     = 1
	fieldChatRespThinking = 25

	fieldThinkingText = 1

	fieldToolCallID      = 3
	fieldToolCallName    = 9
	fieldToolCallRawArgs = 10
	fieldToolCallIsLast  = 11
)

// TokenUsage is agent.v1.TurnEndedUpdate / TokenDeltaUpdate usage.
// Turn-ended counts are Anthropic-style: input is uncached; cache read/write
// are separate. Token deltas are incremental output tokens used as a fallback
// when turn_ended omits totals.
type TokenUsage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	ReasoningTokens  int
}

// Empty reports whether every count is zero.
func (u TokenUsage) Empty() bool {
	return u.InputTokens == 0 &&
		u.OutputTokens == 0 &&
		u.CacheReadTokens == 0 &&
		u.CacheWriteTokens == 0 &&
		u.ReasoningTokens == 0
}

// UsageAccumulator prefers turn_ended totals and falls back to summed token_delta.
type UsageAccumulator struct {
	usage         TokenUsage
	fromTurnEnded bool
}

// Observe records usage from a parsed stream event.
func (a *UsageAccumulator) Observe(ev StreamEvent) {
	if a == nil || ev.Usage == nil {
		return
	}
	switch ev.Type {
	case "turn_ended":
		if !ev.Usage.Empty() {
			a.usage = *ev.Usage
			a.fromTurnEnded = true
		}
	case "token_delta":
		if !a.fromTurnEnded {
			a.usage.OutputTokens += ev.Usage.OutputTokens
		}
	}
}

// Result returns the accumulated usage.
func (a *UsageAccumulator) Result() TokenUsage {
	if a == nil {
		return TokenUsage{}
	}
	return a.usage
}

// StreamEvent represents a parsed piece of a Cursor streaming response.
type StreamEvent struct {
	Type      string // "text", "thinking", "tool_call", "error", "turn_ended", "token_delta"
	Text      string
	ToolCall  *ToolCallEvent
	ErrorText string
	Usage     *TokenUsage
}

// ToolCallEvent represents a parsed tool call from a Cursor response.
type ToolCallEvent struct {
	ID      string
	Name    string
	RawArgs string
	IsLast  bool
}

// ParseResponseFrame extracts StreamEvents from a Connect-RPC protobuf payload.
// Handles AgentServerMessage (NAL), StreamUnifiedChatResponse, and
// StreamUnifiedChatResponseWithTools.
func ParseResponseFrame(data []byte) []StreamEvent {
	if events, ok := parseAgentServerMessage(data); ok {
		return events
	}

	var events []StreamEvent
	pr := NewProtobufReader(data)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}

		switch f.Num {
		case fieldRespResponse:
			events = append(events, parseChatResponse(f.Data)...)
		case fieldRespToolCall:
			if f.WireType != WireBytes {
				continue
			}
			if isPrintableUTF8(f.Data) {
				events = append(events, StreamEvent{Type: "text", Text: string(f.Data)})
				continue
			}
			if tc := parseToolCall(f.Data); tc != nil {
				events = append(events, StreamEvent{Type: "tool_call", ToolCall: tc})
			} else {
				events = append(events, parseChatResponse(f.Data)...)
			}
		}
	}

	return events
}

func parseAgentServerMessage(data []byte) ([]StreamEvent, bool) {
	var events []StreamEvent
	handled := false
	pr := NewProtobufReader(data)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.WireType != WireBytes {
			continue
		}
		switch f.Num {
		case fieldAgentServerInteraction:
			evs, ok := parseInteractionUpdate(f.Data)
			if ok {
				handled = true
				events = append(events, evs...)
			}
		case fieldAgentServerKV:
			handled = true
		}
	}
	return events, handled
}

func parseInteractionUpdate(data []byte) ([]StreamEvent, bool) {
	var events []StreamEvent
	handled := false
	pr := NewProtobufReader(data)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		switch f.Num {
		case fieldInteractionTextDelta:
			handled = true
			if f.WireType == WireBytes {
				notice := false
				inner := NewProtobufReader(f.Data)
				var text string
				for {
					sf, serr := inner.Next()
					if sf == nil || serr != nil {
						break
					}
					switch sf.Num {
					case fieldTextDeltaText:
						text = string(sf.Data)
					case fieldTextDeltaServerNotice:
						notice = sf.Varint != 0
					}
				}
				if text != "" && !notice {
					events = append(events, StreamEvent{Type: "text", Text: text})
				}
			}
		case fieldInteractionThinkingDelta:
			handled = true
			if f.WireType == WireBytes {
				text := GetString(f.Data, fieldThinkingDeltaText)
				if text != "" {
					events = append(events, StreamEvent{Type: "thinking", Text: text})
				}
			}
		case fieldInteractionTokenDelta:
			handled = true
			if f.WireType == WireBytes {
				if ev := parseTokenDelta(f.Data); ev != nil {
					events = append(events, *ev)
				}
			}
		case fieldInteractionHeartbeat:
			handled = true
		case fieldInteractionTurnEnded:
			handled = true
			events = append(events, parseTurnEnded(f.Data))
		default:
			if f.WireType == WireBytes && f.Num >= 2 && f.Num <= 24 {
				handled = true
			}
		}
	}
	return events, handled
}

func parseChatResponse(data []byte) []StreamEvent {
	var events []StreamEvent
	pr := NewProtobufReader(data)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		switch f.Num {
		case fieldChatRespText:
			if f.WireType == WireBytes && len(f.Data) > 0 {
				events = append(events, StreamEvent{Type: "text", Text: string(f.Data)})
			}
		case fieldChatRespThinking:
			if f.WireType == WireBytes {
				text := GetString(f.Data, fieldThinkingText)
				if text != "" {
					events = append(events, StreamEvent{Type: "thinking", Text: text})
				}
			}
		}
	}
	return events
}

func parseToolCall(data []byte) *ToolCallEvent {
	tc := &ToolCallEvent{}
	pr := NewProtobufReader(data)
	found := false
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		switch f.Num {
		case fieldToolCallID:
			tc.ID = string(f.Data)
			found = true
		case fieldToolCallName:
			tc.Name = string(f.Data)
		case fieldToolCallRawArgs:
			tc.RawArgs = string(f.Data)
		case fieldToolCallIsLast:
			tc.IsLast = f.Varint != 0
		}
	}
	if !found {
		return nil
	}
	return tc
}

func parseTurnEnded(data []byte) StreamEvent {
	ev := StreamEvent{Type: "turn_ended"}
	if len(data) == 0 {
		return ev
	}
	u := parseTokenUsage(data)
	if !u.Empty() {
		ev.Usage = &u
	}
	return ev
}

func parseTokenDelta(data []byte) *StreamEvent {
	tokens := int(getVarint(data, fieldTokenDeltaTokens))
	if tokens <= 0 {
		return nil
	}
	return &StreamEvent{
		Type:  "token_delta",
		Usage: &TokenUsage{OutputTokens: tokens},
	}
}

func parseTokenUsage(data []byte) TokenUsage {
	var u TokenUsage
	pr := NewProtobufReader(data)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.WireType != WireVarint {
			continue
		}
		n := int(f.Varint)
		switch f.Num {
		case fieldTurnEndedInputTokens:
			u.InputTokens = n
		case fieldTurnEndedOutputTokens:
			u.OutputTokens = n
		case fieldTurnEndedCacheReadTokens:
			u.CacheReadTokens = n
		case fieldTurnEndedCacheWriteTokens:
			u.CacheWriteTokens = n
		case fieldTurnEndedReasoningTokens:
			u.ReasoningTokens = n
		}
	}
	return u
}

func getVarint(data []byte, fieldNum uint32) uint64 {
	pr := NewProtobufReader(data)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			return 0
		}
		if f.Num == fieldNum && f.WireType == WireVarint {
			return f.Varint
		}
	}
}

// ConsumeAssistantStream reads Connect-RPC AgentService/Run frames until the
// turn ends. emit is invoked for text and thinking deltas; usage is accumulated
// from turn_ended (preferred) or token_delta fallbacks.
func ConsumeAssistantStream(body io.Reader, emit func(StreamEvent) error) (TokenUsage, string) {
	var acc UsageAccumulator
	var connectErr string
	for {
		frame, err := DecodeFrame(body)
		if err != nil {
			return acc.Result(), connectErr
		}
		if msg := ConnectErrorJSON(frame); msg != "" {
			connectErr = msg
			break
		}
		events := ParseResponseFrame(frame.Payload)
		for _, ev := range events {
			acc.Observe(ev)
			switch ev.Type {
			case "text", "thinking":
				if emit != nil {
					if err := emit(ev); err != nil {
						return acc.Result(), connectErr
					}
				}
			case "turn_ended":
				return acc.Result(), connectErr
			}
		}
		if frame.Flags&FrameFlagEndStream != 0 {
			break
		}
	}
	return acc.Result(), connectErr
}

func isPrintableUTF8(b []byte) bool {
	if len(b) == 0 || !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		if r == 0 {
			return false
		}
	}
	return true
}

package apicompat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// Direct Anthropic Messages <-> OpenAI Chat Completions bridge. This bypasses
// the Responses API hub entirely:
//
//	request  : /v1/messages --AnthropicToChatCompletions--> /v1/chat/completions
//	response : chat SSE --ChatCompletionsChunkToAnthropicEvents--> Anthropic SSE
//
// The streaming response side is a single state machine that opens/closes
// Anthropic content blocks straight from chat deltas (reasoning_content ->
// thinking block, content -> text block, tool_calls -> tool_use block), so
// reasoning can never become an "orphan" the way it can when routed through the
// Responses item lifecycle (chat -> responses -> anthropic).

// ===========================================================================
// Request side: Anthropic request -> Chat Completions request
// ===========================================================================

// AnthropicToChatCompletions converts a /v1/messages request body into a
// /v1/chat/completions request for upstreams that only speak Chat Completions.
func AnthropicToChatCompletions(req *AnthropicRequest) (*ChatCompletionsRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}

	var messages []ChatMessage
	if sys := anthropicSystemText(req.System); sys != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: mustJSONString(sys)})
	}
	for _, m := range req.Messages {
		msgs, err := anthropicMessageToChat(m)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msgs...)
	}

	out := &ChatCompletionsRequest{
		Model:       req.Model,
		Messages:    messages,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}
	if req.MaxTokens > 0 {
		v := req.MaxTokens
		out.MaxCompletionTokens = &v
	}
	if len(req.StopSeqs) > 0 {
		if raw, err := json.Marshal(req.StopSeqs); err == nil {
			out.Stop = raw
		}
	}
	if len(req.Tools) > 0 {
		out.Tools = anthropicToolsToChat(req.Tools)
	}
	if len(req.ToolChoice) > 0 {
		if tc, err := anthropicToolChoiceToChat(req.ToolChoice); err == nil {
			out.ToolChoice = tc
		}
	}
	// When the client explicitly disables thinking, forward {type:"disabled"} to
	// the upstream (GLM/DeepSeek/Qwen/... honor it natively) and drop
	// reasoning_effort, so we never send a "disable thinking" signal together with
	// an effort hint that strict upstreams may reject. All other cases keep the
	// existing reasoning_effort mapping (enabled / output_config.effort).
	if req.Thinking != nil && req.Thinking.Type == "disabled" {
		out.Thinking = &ChatThinking{Type: "disabled"}
	} else if effort := anthropicReasoningEffort(req); effort != "" {
		out.ReasoningEffort = effort
	}
	return out, nil
}

// anthropicSystemText flattens the Anthropic system field (string or block
// array) into a single string.
func anthropicSystemText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// anthropicMessageToChat converts one Anthropic message into one or more chat
// messages. A user turn carrying tool_result blocks fans out into separate
// role:"tool" messages (which must follow the assistant tool_calls message).
func anthropicMessageToChat(m AnthropicMessage) ([]ChatMessage, error) {
	// Plain string content.
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		role := m.Role
		if role != "user" && role != "assistant" {
			role = "user"
		}
		return []ChatMessage{{Role: role, Content: mustJSONString(s)}}, nil
	}

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return nil, err
	}

	if m.Role == "assistant" {
		return anthropicAssistantBlocksToChat(blocks), nil
	}
	return anthropicUserBlocksToChat(blocks), nil
}

func anthropicUserBlocksToChat(blocks []AnthropicContentBlock) []ChatMessage {
	var out []ChatMessage
	var toolMsgs []ChatMessage
	var parts []ChatContentPart

	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				parts = append(parts, ChatContentPart{Type: "text", Text: b.Text})
			}
		case "image":
			if uri := anthropicImageDataURI(b.Source); uri != "" {
				parts = append(parts, ChatContentPart{Type: "image_url", ImageURL: &ChatImageURL{URL: uri}})
			}
		case "tool_result":
			toolMsgs = append(toolMsgs, ChatMessage{
				Role:       "tool",
				ToolCallID: b.ToolUseID,
				Content:    mustJSONString(anthropicToolResultText(b)),
			})
		}
	}

	// tool_result replies must come first: Chat Completions requires the tool
	// messages to immediately follow the assistant's tool_calls message. Any new
	// user text/image in the same Anthropic turn is a fresh user turn and follows.
	out = append(out, toolMsgs...)
	if len(parts) > 0 {
		out = append(out, ChatMessage{Role: "user", Content: chatContentFromParts(parts)})
	}
	return out
}

func anthropicAssistantBlocksToChat(blocks []AnthropicContentBlock) []ChatMessage {
	msg := ChatMessage{Role: "assistant"}
	var textParts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				textParts = append(textParts, b.Text)
			}
		case "tool_use":
			args := "{}"
			if len(b.Input) > 0 {
				args = string(b.Input)
			}
			msg.ToolCalls = append(msg.ToolCalls, ChatToolCall{
				ID:       b.ID,
				Type:     "function",
				Function: ChatFunctionCall{Name: b.Name, Arguments: args},
			})
			// thinking blocks: dropped (chat upstreams reject them as input)
		}
	}
	if text := strings.Join(textParts, "\n\n"); text != "" {
		msg.Content = mustJSONString(text)
	}
	return []ChatMessage{msg}
}

func anthropicToolResultText(b AnthropicContentBlock) string {
	if len(b.Content) == 0 {
		return "(empty)"
	}
	var s string
	if err := json.Unmarshal(b.Content, &s); err == nil {
		if s == "" {
			return "(empty)"
		}
		return s
	}
	var inner []AnthropicContentBlock
	if err := json.Unmarshal(b.Content, &inner); err != nil {
		return "(empty)"
	}
	var parts []string
	for _, ib := range inner {
		if ib.Type == "text" && ib.Text != "" {
			parts = append(parts, ib.Text)
		}
	}
	if text := strings.Join(parts, "\n\n"); text != "" {
		return text
	}
	return "(empty)"
}

func anthropicImageDataURI(src *AnthropicImageSource) string {
	if src == nil || src.Data == "" {
		return ""
	}
	mediaType := src.MediaType
	if mediaType == "" {
		mediaType = "image/png"
	}
	return "data:" + mediaType + ";base64," + src.Data
}

// chatContentFromParts emits a plain string when the content is a single text
// part (the common case), else the multi-modal parts array.
func chatContentFromParts(parts []ChatContentPart) json.RawMessage {
	if len(parts) == 1 && parts[0].Type == "text" {
		return mustJSONString(parts[0].Text)
	}
	raw, _ := json.Marshal(parts)
	return raw
}

func anthropicToolsToChat(tools []AnthropicTool) []ChatTool {
	var out []ChatTool
	for _, t := range tools {
		// keep only custom tools (type empty); any typed tool is an Anthropic built-in the chat upstream can't use
		if t.Type != "" {
			continue
		}
		strict := false
		out = append(out, ChatTool{
			Type: "function",
			Function: &ChatFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  normalizeToolParams(t.InputSchema),
				Strict:      &strict,
			},
		})
	}
	return out
}

func anthropicToolChoiceToChat(raw json.RawMessage) (json.RawMessage, error) {
	var tc struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &tc); err != nil {
		return nil, err
	}
	switch tc.Type {
	case "auto":
		return json.Marshal("auto")
	case "any":
		return json.Marshal("required")
	case "none":
		return json.Marshal("none")
	case "tool":
		return json.Marshal(map[string]any{
			"type":     "function",
			"function": map[string]any{"name": tc.Name},
		})
	default:
		return raw, nil
	}
}

// anthropicReasoningEffort mirrors the validated chain: output_config.effort
// wins; otherwise enabled thinking defaults to "medium"; else omitted.
func anthropicReasoningEffort(req *AnthropicRequest) string {
	effort := ""
	if req.OutputConfig != nil && req.OutputConfig.Effort != "" {
		effort = req.OutputConfig.Effort
	} else if req.Thinking != nil && req.Thinking.Type == "enabled" {
		effort = "medium"
	}
	if effort == "max" {
		return "xhigh"
	}
	return effort
}

func normalizeToolParams(schema json.RawMessage) json.RawMessage {
	if len(schema) == 0 || string(schema) == "null" {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(schema, &m); err != nil {
		return schema
	}
	if string(m["type"]) != `"object"` {
		return schema
	}
	if _, ok := m["properties"]; ok {
		return schema
	}
	m["properties"] = json.RawMessage(`{}`)
	if out, err := json.Marshal(m); err == nil {
		return out
	}
	return schema
}

func mustJSONString(s string) json.RawMessage {
	raw, _ := json.Marshal(s)
	return raw
}

// ===========================================================================
// Response side: streaming Chat Completions -> Anthropic SSE
// ===========================================================================

// ChatCompletionsToAnthropicStreamState carries the cross-chunk state needed to
// reconstruct Anthropic content blocks from a Chat Completions SSE stream.
type ChatCompletionsToAnthropicStreamState struct {
	MessageID string
	Model     string

	started bool
	stopped bool

	blockOpen  bool
	blockType  string // "thinking" | "text" | "tool_use"
	blockIndex int
	nextIndex  int

	// Current tool_use block. Blocks are serialized (one open at a time), so a
	// new tool_calls index closes the previous tool block before opening.
	curToolChatIdx  int // -1 when no tool block is open
	curToolName     string
	curToolArgs     strings.Builder
	curToolBuffered bool // "Read": buffer args, emit one sanitized delta at close
	curToolStreamed bool

	reasoning   strings.Builder // accumulated for the reasoning-only fallback
	textEmitted bool

	hasTool bool
	finish  string
	usage   *ChatUsage
}

// NewChatCompletionsToAnthropicStreamState builds an empty stream state.
func NewChatCompletionsToAnthropicStreamState(model string) *ChatCompletionsToAnthropicStreamState {
	return &ChatCompletionsToAnthropicStreamState{Model: model, curToolChatIdx: -1}
}

// ChatCompletionsChunkToAnthropicEvents converts one Chat Completions stream
// chunk into zero or more Anthropic SSE events, mutating state.
func ChatCompletionsChunkToAnthropicEvents(chunk *ChatCompletionsChunk, s *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if chunk == nil || s == nil {
		return nil
	}
	if chunk.ID != "" {
		s.MessageID = chunk.ID
	}
	if s.Model == "" && chunk.Model != "" {
		s.Model = chunk.Model
	}
	if chunk.Usage != nil {
		s.usage = chunk.Usage
	}

	var events []AnthropicStreamEvent
	events = append(events, s.ensureStart()...)

	for _, choice := range chunk.Choices {
		d := choice.Delta

		if d.ReasoningContent != nil && *d.ReasoningContent != "" {
			events = append(events, s.openBlock("thinking")...)
			_, _ = s.reasoning.WriteString(*d.ReasoningContent)
			events = append(events, contentBlockDelta(s.blockIndex, AnthropicDelta{
				Type: "thinking_delta", Thinking: *d.ReasoningContent,
			}))
		}

		if d.Content != nil && *d.Content != "" {
			events = append(events, s.openBlock("text")...)
			s.textEmitted = true
			events = append(events, contentBlockDelta(s.blockIndex, AnthropicDelta{
				Type: "text_delta", Text: *d.Content,
			}))
		}

		for _, tc := range d.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			if !s.blockOpen || s.blockType != "tool_use" || s.curToolChatIdx != idx {
				toolID := tc.ID
				if toolID == "" {
					toolID = generateToolUseID()
				}
				events = append(events, s.openToolBlock(idx, toolID, tc.Function.Name)...)
			} else if tc.Function.Name != "" && s.curToolName == "" {
				s.curToolName = tc.Function.Name
				s.curToolBuffered = tc.Function.Name == "Read"
			}
			if tc.Function.Arguments != "" {
				_, _ = s.curToolArgs.WriteString(tc.Function.Arguments)
				if !s.curToolBuffered {
					s.curToolStreamed = true
					events = append(events, contentBlockDelta(s.blockIndex, AnthropicDelta{
						Type: "input_json_delta", PartialJSON: tc.Function.Arguments,
					}))
				}
			}
		}

		if choice.FinishReason != nil && *choice.FinishReason != "" {
			s.finish = *choice.FinishReason
		}
	}
	return events
}

// FinalizeChatCompletionsAnthropicStream closes any open block and emits the
// terminal message_delta + message_stop events. Safe to call once.
func FinalizeChatCompletionsAnthropicStream(s *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if s == nil || !s.started || s.stopped {
		return nil
	}
	var events []AnthropicStreamEvent
	events = append(events, s.closeBlock()...)

	// Reasoning-only completion (reasoning present, no text, no tools): echo the
	// reasoning as the answer so the client never gets a thinking block with no
	// message. Mirrors sub2api's synthesizeChatReasoningFallbackMessage.
	if !s.textEmitted && !s.hasTool {
		if r := s.reasoning.String(); strings.TrimSpace(r) != "" {
			events = append(events, s.openBlock("text")...)
			s.textEmitted = true
			events = append(events, contentBlockDelta(s.blockIndex, AnthropicDelta{
				Type: "text_delta", Text: r,
			}))
			events = append(events, s.closeBlock()...)
		}
	}

	usage := anthropicUsageFromChatUsage(s.usage)
	events = append(events,
		AnthropicStreamEvent{
			Type:  "message_delta",
			Delta: &AnthropicDelta{StopReason: chatFinishToAnthropicStopReason(s.finish, s.hasTool)},
			Usage: &usage,
		},
		AnthropicStreamEvent{Type: "message_stop"},
	)
	s.stopped = true
	return events
}

func (s *ChatCompletionsToAnthropicStreamState) ensureStart() []AnthropicStreamEvent {
	if s.started {
		return nil
	}
	s.started = true
	return []AnthropicStreamEvent{{
		Type: "message_start",
		Message: &AnthropicResponse{
			ID:      s.MessageID,
			Type:    "message",
			Role:    "assistant",
			Content: []AnthropicContentBlock{},
			Model:   s.Model,
			Usage:   AnthropicUsage{},
		},
	}}
}

// openBlock ensures a thinking/text block of the given type is open, closing
// any other open block first. A no-op if that exact block is already open.
func (s *ChatCompletionsToAnthropicStreamState) openBlock(blockType string) []AnthropicStreamEvent {
	if s.blockOpen && s.blockType == blockType {
		return nil
	}
	var events []AnthropicStreamEvent
	events = append(events, s.closeBlock()...)

	idx := s.nextIndex
	s.nextIndex++
	s.blockOpen = true
	s.blockType = blockType
	s.blockIndex = idx

	bi := idx
	events = append(events, AnthropicStreamEvent{
		Type:         "content_block_start",
		Index:        &bi,
		ContentBlock: &AnthropicContentBlock{Type: blockType},
	})
	return events
}

// openToolBlock closes any open block and opens a tool_use block for a new
// chat-completions tool_calls index.
func (s *ChatCompletionsToAnthropicStreamState) openToolBlock(chatIdx int, id, name string) []AnthropicStreamEvent {
	var events []AnthropicStreamEvent
	events = append(events, s.closeBlock()...)

	idx := s.nextIndex
	s.nextIndex++
	s.blockOpen = true
	s.blockType = "tool_use"
	s.blockIndex = idx
	s.hasTool = true
	s.curToolChatIdx = chatIdx
	s.curToolName = name
	s.curToolBuffered = name == "Read"
	s.curToolStreamed = false
	s.curToolArgs.Reset()

	bi := idx
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_start",
		Index: &bi,
		ContentBlock: &AnthropicContentBlock{
			Type:  "tool_use",
			ID:    id,
			Name:  name,
			Input: json.RawMessage("{}"),
		},
	})
	return events
}

func (s *ChatCompletionsToAnthropicStreamState) closeBlock() []AnthropicStreamEvent {
	if !s.blockOpen {
		return nil
	}
	var events []AnthropicStreamEvent
	idx := s.blockIndex

	// A tool whose arguments were never streamed (a buffered "Read" tool, or a
	// call that produced no argument fragments) emits them as a single, sanitized
	// input_json_delta right before the block closes. Mirrors sub2api's
	// resToAnthHandleFuncArgsDone / closeChatToolItems.
	if s.blockType == "tool_use" && !s.curToolStreamed {
		args := s.curToolArgs.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		events = append(events, contentBlockDelta(idx, AnthropicDelta{
			Type: "input_json_delta", PartialJSON: string(sanitizeAnthropicToolUseInput(s.curToolName, args)),
		}))
	}

	s.blockOpen = false
	s.blockType = ""
	s.curToolChatIdx = -1
	s.curToolName = ""
	s.curToolBuffered = false
	s.curToolStreamed = false
	s.curToolArgs.Reset()

	events = append(events, AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	return events
}

func contentBlockDelta(index int, delta AnthropicDelta) AnthropicStreamEvent {
	i := index
	return AnthropicStreamEvent{Type: "content_block_delta", Index: &i, Delta: &delta}
}

func chatFinishToAnthropicStopReason(finish string, hasTool bool) string {
	switch finish {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "end_turn"
	default:
		if hasTool {
			return "tool_use"
		}
		return "end_turn"
	}
}

// anthropicUsageFromChatUsage maps Chat Completions usage to Anthropic usage.
// Anthropic's input_tokens excludes cached tokens (reported separately as
// cache_read_input_tokens), so cached is subtracted out. Mirrors sub2api's
// anthropicUsageFromResponsesUsage.
func anthropicUsageFromChatUsage(u *ChatUsage) AnthropicUsage {
	if u == nil {
		return AnthropicUsage{}
	}
	cached := 0
	if u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	input := u.PromptTokens - cached
	if input < 0 {
		input = 0
	}
	return AnthropicUsage{
		InputTokens:          input,
		OutputTokens:         u.CompletionTokens,
		CacheReadInputTokens: cached,
	}
}

// ChatCompletionsStreamToAnthropicResponse is the non-streaming (sync) path: it
// buffers the whole chat stream and assembles a single Anthropic Messages JSON
// response, matching what the gateway does when the client requested
// stream=false (upstream is always streamed, then collapsed). Block order
// mirrors ResponsesToAnthropic: thinking, then text, then tool_use.
func ChatCompletionsStreamToAnthropicResponse(chunks []*ChatCompletionsChunk, model string) *AnthropicResponse {
	id := ""
	var reasoning, text strings.Builder
	type toolAgg struct {
		id, name string
		args     strings.Builder
	}
	tools := map[int]*toolAgg{}
	maxToolIdx := -1
	finish := ""
	var usage *ChatUsage

	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		if chunk.ID != "" {
			id = chunk.ID
		}
		if model == "" && chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			d := choice.Delta
			if d.ReasoningContent != nil {
				_, _ = reasoning.WriteString(*d.ReasoningContent)
			}
			if d.Content != nil {
				_, _ = text.WriteString(*d.Content)
			}
			for _, tc := range d.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				agg, ok := tools[idx]
				if !ok {
					agg = &toolAgg{}
					tools[idx] = agg
					if idx > maxToolIdx {
						maxToolIdx = idx
					}
				}
				if tc.ID != "" {
					agg.id = tc.ID
				}
				if tc.Function.Name != "" {
					agg.name = tc.Function.Name
				}
				_, _ = agg.args.WriteString(tc.Function.Arguments)
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finish = *choice.FinishReason
			}
		}
	}

	hasTool := maxToolIdx >= 0
	var blocks []AnthropicContentBlock
	if reasoning.Len() > 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "thinking", Thinking: reasoning.String()})
	}
	finalText := text.String()
	if finalText == "" && !hasTool && strings.TrimSpace(reasoning.String()) != "" {
		finalText = reasoning.String() // reasoning-only fallback
	}
	if finalText != "" {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: finalText})
	}
	for i := 0; i <= maxToolIdx; i++ {
		agg, ok := tools[i]
		if !ok {
			continue
		}
		args := agg.args.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		toolID := agg.id
		if toolID == "" {
			toolID = generateToolUseID()
		}
		blocks = append(blocks, AnthropicContentBlock{
			Type:  "tool_use",
			ID:    toolID,
			Name:  agg.name,
			Input: sanitizeAnthropicToolUseInput(agg.name, args),
		})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: ""})
	}

	return &AnthropicResponse{
		ID:         id,
		Type:       "message",
		Role:       "assistant",
		Model:      model,
		Content:    blocks,
		StopReason: chatFinishToAnthropicStopReason(finish, hasTool),
		Usage:      anthropicUsageFromChatUsage(usage),
	}
}

func generateToolUseID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "toolu_" + hex.EncodeToString(b)
}

package kiro

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	kiroMaxToolDescLen         = 10237
	kiroMaxToolNameLen         = 63
	thinkingStartTag           = "<thinking>"
	thinkingEndTag             = "</thinking>"
	embeddedToolCallPrefix     = "[Called "
	minFrameSize               = 16
	maxEventMsgSize            = 10 << 20
	writeToolDescriptionSuffix = "IMPORTANT: If the content to write exceeds 150 lines, write only the first 50 lines with this tool, then append the remaining content using Edit calls in chunks of no more than 50 lines. Use a unique placeholder if needed. Do not write the whole file in one call."
	editToolDescriptionSuffix  = "IMPORTANT: If new content exceeds 50 lines, split it into multiple Edit calls, replacing or appending no more than 50 lines per call. If appending, use a unique placeholder and remove it in the final chunk."
	systemChunkedWritePolicy   = "When Write or Edit tools include chunking limits, comply silently and complete the operation through multiple tool calls when needed."
)

var (
	trailingCommaPattern = regexp.MustCompile(`,\s*([}\]])`)
	requiredToolFields   = map[string][][]string{
		"write":              {{"file_path", "path"}, {"content"}},
		"write_to_file":      {{"path"}, {"content"}},
		"fswrite":            {{"path"}, {"content"}},
		"create_file":        {{"path"}, {"content"}},
		"edit_file":          {{"path"}},
		"apply_diff":         {{"path"}, {"diff"}},
		"str_replace_editor": {{"path"}, {"old_str"}, {"new_str"}},
		"bash":               {{"cmd", "command"}},
		"execute":            {{"command"}},
		"run_command":        {{"command"}},
	}
)

type Usage struct {
	InputTokens          int
	OutputTokens         int
	TotalTokens          int
	CacheReadInputTokens int
}

type StreamResult struct {
	Usage         Usage
	StopReason    string
	FirstDeltaDur *time.Duration
}

type ParseResult struct {
	ResponseBody []byte
	Usage        Usage
	StopReason   string
}

type KiroRequestContext struct {
	ToolNameMap     map[string]string
	ThinkingEnabled bool
}

type KiroBuildResult struct {
	Payload []byte
	Context KiroRequestContext
}

type KiroPayload struct {
	ConversationState KiroConversationState `json:"conversationState"`
	ProfileArn        string                `json:"profileArn,omitempty"`
	InferenceConfig   *KiroInferenceConfig  `json:"inferenceConfig,omitempty"`
}

type KiroInferenceConfig struct {
	MaxTokens   int     `json:"maxTokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"topP,omitempty"`
}

type thinkingDirective struct {
	Mode         string
	BudgetTokens int
	Effort       string
}

type KiroConversationState struct {
	AgentContinuationID string               `json:"agentContinuationId,omitempty"`
	AgentTaskType       string               `json:"agentTaskType,omitempty"`
	ChatTriggerType     string               `json:"chatTriggerType"`
	ConversationID      string               `json:"conversationId"`
	CurrentMessage      KiroCurrentMessage   `json:"currentMessage"`
	History             []KiroHistoryMessage `json:"history,omitempty"`
}

type KiroCurrentMessage struct {
	UserInputMessage KiroUserInputMessage `json:"userInputMessage"`
}

type KiroHistoryMessage struct {
	UserInputMessage         *KiroUserInputMessage         `json:"userInputMessage,omitempty"`
	AssistantResponseMessage *KiroAssistantResponseMessage `json:"assistantResponseMessage,omitempty"`
}

type KiroImage struct {
	Format string          `json:"format"`
	Source KiroImageSource `json:"source"`
}

type KiroImageSource struct {
	Bytes string `json:"bytes"`
}

type KiroUserInputMessage struct {
	Content                 string                       `json:"content"`
	ModelID                 string                       `json:"modelId"`
	Origin                  string                       `json:"origin"`
	Images                  []KiroImage                  `json:"images,omitempty"`
	UserInputMessageContext *KiroUserInputMessageContext `json:"userInputMessageContext,omitempty"`
}

type KiroUserInputMessageContext struct {
	ToolResults []KiroToolResult  `json:"toolResults,omitempty"`
	Tools       []KiroToolWrapper `json:"tools,omitempty"`
}

type KiroToolResult struct {
	Content   []KiroTextContent `json:"content"`
	Status    string            `json:"status"`
	ToolUseID string            `json:"toolUseId"`
}

type KiroTextContent struct {
	Text string `json:"text"`
}

type KiroToolWrapper struct {
	ToolSpecification KiroToolSpecification `json:"toolSpecification"`
}

type KiroToolSpecification struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema KiroInputSchema `json:"inputSchema"`
}

type KiroInputSchema struct {
	JSON any `json:"json"`
}

type KiroAssistantResponseMessage struct {
	Content  string        `json:"content"`
	ToolUses []KiroToolUse `json:"toolUses,omitempty"`
}

type KiroToolUse struct {
	ToolUseID    string         `json:"toolUseId"`
	Name         string         `json:"name"`
	Input        map[string]any `json:"input"`
	IsTruncated  bool           `json:"-"`
	TruncatedRaw string         `json:"-"`
}

type toolUseState struct {
	ToolUseID   string
	Name        string
	InputBuffer strings.Builder
}

type eventStreamMessage struct {
	EventType string
	Payload   []byte
}

func MapModel(model string) string {
	switch strings.TrimSpace(strings.ToLower(model)) {
	case "claude-opus-4-6", "claude-opus-4-6-thinking", "claude-opus-4.6":
		return "claude-opus-4.6"
	case "claude-sonnet-4-6", "claude-sonnet-4-6-thinking", "claude-sonnet-4.6":
		return "claude-sonnet-4.6"
	case "claude-opus-4-5-20251101", "claude-opus-4-5-20251101-thinking", "claude-opus-4.5":
		return "claude-opus-4.5"
	case "claude-sonnet-4-5-20250929", "claude-sonnet-4-5-20250929-thinking", "claude-sonnet-4.5":
		return "claude-sonnet-4.5"
	case "claude-haiku-4-5-20251001", "claude-haiku-4-5-20251001-thinking", "claude-haiku-4.5":
		return "claude-haiku-4.5"
	default:
		return ""
	}
}

func normalizeModelAlias(model string) string {
	base := strings.TrimSpace(strings.ToLower(model))
	for {
		next := strings.TrimSuffix(base, "-thinking")
		if next == base {
			return next
		}
		base = next
	}
}

func BuildKiroPayload(claudeBody []byte, modelID, profileArn, origin string, headers http.Header) ([]byte, error) {
	result, err := BuildKiroPayloadWithContext(claudeBody, modelID, profileArn, origin, headers)
	if err != nil {
		return nil, err
	}
	return result.Payload, nil
}

func BuildKiroPayloadWithContext(claudeBody []byte, modelID, profileArn, origin string, headers http.Header) (*KiroBuildResult, error) {
	const kiroMaxOutputTokens = 32000
	requestCtx := KiroRequestContext{ToolNameMap: map[string]string{}}
	var maxTokens int64
	if mt := gjson.GetBytes(claudeBody, "max_tokens"); mt.Exists() {
		maxTokens = mt.Int()
		if maxTokens == -1 {
			maxTokens = kiroMaxOutputTokens
		}
	}

	var temperature float64
	var hasTemperature bool
	if temp := gjson.GetBytes(claudeBody, "temperature"); temp.Exists() {
		temperature = temp.Float()
		hasTemperature = true
	}

	var topP float64
	var hasTopP bool
	if tp := gjson.GetBytes(claudeBody, "top_p"); tp.Exists() {
		topP = tp.Float()
		hasTopP = true
	}

	messages := gjson.GetBytes(claudeBody, "messages")
	thinking := deriveThinkingDirective(claudeBody, headers)
	requestCtx.ThinkingEnabled = thinking != nil
	toolChoiceHint := extractClaudeToolChoiceHint(claudeBody, &requestCtx)
	systemPrompt := buildInjectedSystemPrompt(extractSystemPrompt(claudeBody), thinking, toolChoiceHint)

	history, currentUserMsg, currentToolResults := processMessages(messages, modelID, normalizeOrigin(origin), &requestCtx)
	history = prependSystemHistory(history, systemPrompt, modelID, normalizeOrigin(origin))
	var tools gjson.Result
	if !isToolChoiceNone(claudeBody) {
		tools = gjson.GetBytes(claudeBody, "tools")
	}
	kiroTools := convertClaudeToolsToKiro(tools, &requestCtx)
	currentToolResults, orphanedToolUseIDs := validateToolPairing(history, currentToolResults)
	removeOrphanedToolUses(history, orphanedToolUseIDs)
	kiroTools = appendMissingPlaceholderTools(kiroTools, collectHistoryToolNames(history))
	if currentUserMsg != nil {
		currentUserMsg.Content = buildFinalContent(currentUserMsg.Content, currentToolResults)
		currentToolResults = deduplicateToolResults(currentToolResults)
		if len(kiroTools) > 0 || len(currentToolResults) > 0 {
			currentUserMsg.UserInputMessageContext = &KiroUserInputMessageContext{
				Tools:       kiroTools,
				ToolResults: currentToolResults,
			}
		}
	}

	var currentMessage KiroCurrentMessage
	if currentUserMsg != nil {
		currentMessage = KiroCurrentMessage{UserInputMessage: *currentUserMsg}
	} else {
		currentMessage = KiroCurrentMessage{UserInputMessage: KiroUserInputMessage{
			Content: buildFinalContent("", nil),
			ModelID: modelID,
			Origin:  normalizeOrigin(origin),
		}}
	}

	var inferenceConfig *KiroInferenceConfig
	if maxTokens > 0 || hasTemperature || hasTopP {
		inferenceConfig = &KiroInferenceConfig{}
		if maxTokens > 0 {
			inferenceConfig.MaxTokens = int(maxTokens)
		}
		if hasTemperature {
			inferenceConfig.Temperature = temperature
		}
		if hasTopP {
			inferenceConfig.TopP = topP
		}
	}

	conversationID := extractMetadataFromMessages(messages, "conversationId")
	continuationID := extractMetadataFromMessages(messages, "continuationId")
	if conversationID == "" {
		conversationID = uuid.NewString()
	}

	payload := KiroPayload{
		ConversationState: KiroConversationState{
			AgentTaskType:   "vibe",
			ChatTriggerType: "MANUAL",
			ConversationID:  conversationID,
			CurrentMessage:  currentMessage,
			History:         history,
		},
		ProfileArn:      profileArn,
		InferenceConfig: inferenceConfig,
	}
	if continuationID != "" {
		payload.ConversationState.AgentContinuationID = continuationID
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &KiroBuildResult{Payload: payloadBytes, Context: requestCtx}, nil
}

func ParseNonStreamingEventStream(body io.Reader, model string) (*ParseResult, error) {
	return ParseNonStreamingEventStreamWithContext(body, model, KiroRequestContext{})
}

func ParseNonStreamingEventStreamWithContext(body io.Reader, model string, requestCtx KiroRequestContext) (*ParseResult, error) {
	content, toolUses, usage, stopReason, err := parseEventStream(body)
	if err != nil {
		return nil, err
	}
	return &ParseResult{
		ResponseBody: buildClaudeResponse(content, toolUses, model, usage, stopReason, requestCtx),
		Usage:        usage,
		StopReason:   stopReason,
	}, nil
}

func StreamEventStreamAsAnthropic(ctx context.Context, body io.Reader, w io.Writer, model string, inputTokens int) (*StreamResult, error) {
	return StreamEventStreamAsAnthropicWithContext(ctx, body, w, model, inputTokens, KiroRequestContext{})
}

func StreamEventStreamAsAnthropicWithContext(ctx context.Context, body io.Reader, w io.Writer, model string, inputTokens int, requestCtx KiroRequestContext) (*StreamResult, error) {
	reader := bufio.NewReader(body)
	start := time.Now()
	var firstDelta *time.Duration
	usage := Usage{InputTokens: inputTokens}
	contentBlockIndex := -1
	thinkingBlockIndex := -1
	messageStartSent := false
	textBlockOpen := false
	thinkingBlockOpen := false
	processedIDs := make(map[string]bool)
	emittedToolContents := make(map[string]bool)
	streamingToolBlockIndices := make(map[string]int)
	streamingToolStarted := make(map[string]bool)
	streamingToolStopped := make(map[string]bool)
	currentStreamingToolID := ""
	pendingAssistantText := ""
	pendingLeadingWhitespace := ""
	stopReason := ""
	thinkingBuffer := ""
	inThinkingBlock := false
	stripThinkingLeadingNewline := false
	sawNonThinkingBlock := false

	writeEvent := func(event string, data any) error {
		payload, err := json.Marshal(data)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, "event: "+event+"\ndata: "+string(payload)+"\n\n")
		return err
	}
	ensureMessageStart := func() error {
		if messageStartSent {
			return nil
		}
		if err := writeEvent("message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":            "msg_" + uuid.NewString()[:24],
				"type":          "message",
				"role":          "assistant",
				"content":       []any{},
				"model":         model,
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": map[string]any{
					"input_tokens":  usage.InputTokens,
					"output_tokens": 0,
				},
			},
		}); err != nil {
			return err
		}
		messageStartSent = true
		return nil
	}

	closeText := func() error {
		if !textBlockOpen {
			return nil
		}
		textBlockOpen = false
		return writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": contentBlockIndex})
	}
	closeThinking := func() error {
		if !thinkingBlockOpen {
			return nil
		}
		thinkingBlockOpen = false
		return writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": thinkingBlockIndex})
	}
	closeStreamingTool := func(toolUseID string) error {
		if toolUseID == "" || !streamingToolStarted[toolUseID] || streamingToolStopped[toolUseID] {
			return nil
		}
		streamingToolStopped[toolUseID] = true
		if currentStreamingToolID == toolUseID {
			currentStreamingToolID = ""
		}
		return writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": streamingToolBlockIndices[toolUseID]})
	}
	closeOpenStreamingTool := func() error {
		return closeStreamingTool(currentStreamingToolID)
	}
	startStreamingToolUse := func(toolUseID, name string) error {
		if toolUseID == "" || name == "" || streamingToolStopped[toolUseID] {
			return nil
		}
		sawNonThinkingBlock = true
		if currentStreamingToolID != "" && currentStreamingToolID != toolUseID {
			if err := closeOpenStreamingTool(); err != nil {
				return err
			}
		}
		if stopReason == "" {
			stopReason = "tool_use"
		}
		if err := ensureMessageStart(); err != nil {
			return err
		}
		if firstDelta == nil {
			delta := time.Since(start)
			firstDelta = &delta
		}
		if err := closeThinking(); err != nil {
			return err
		}
		if err := closeText(); err != nil {
			return err
		}
		blockIndex, ok := streamingToolBlockIndices[toolUseID]
		if !ok {
			contentBlockIndex++
			blockIndex = contentBlockIndex
			streamingToolBlockIndices[toolUseID] = blockIndex
		}
		currentStreamingToolID = toolUseID
		if streamingToolStarted[toolUseID] {
			return nil
		}
		streamingToolStarted[toolUseID] = true
		return writeEvent("content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": blockIndex,
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    toolUseID,
				"name":  restoreResponseToolName(name, requestCtx),
				"input": map[string]any{},
			},
		})
	}
	emitStreamingToolInput := func(toolUseID, name, fragment string) error {
		if fragment == "" {
			return nil
		}
		if err := startStreamingToolUse(toolUseID, name); err != nil {
			return err
		}
		if toolUseID == "" || !streamingToolStarted[toolUseID] || streamingToolStopped[toolUseID] {
			return nil
		}
		return writeEvent("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": streamingToolBlockIndices[toolUseID],
			"delta": map[string]any{
				"type":         "input_json_delta",
				"partial_json": fragment,
			},
		})
	}
	processStreamingToolUseEvent := func(event map[string]any) error {
		tu := nestedEvent(event, "toolUseEvent")
		toolUseID := getString(tu, "toolUseId")
		name := getString(tu, "name")
		if err := startStreamingToolUse(toolUseID, name); err != nil {
			return err
		}
		if inputRaw, ok := tu["input"]; ok {
			switch v := inputRaw.(type) {
			case string:
				if err := emitStreamingToolInput(toolUseID, name, v); err != nil {
					return err
				}
			case map[string]any:
				encoded, err := json.Marshal(v)
				if err != nil {
					return err
				}
				if err := emitStreamingToolInput(toolUseID, name, string(encoded)); err != nil {
					return err
				}
			}
		}
		isStop, _ := tu["stop"].(bool)
		if isStop {
			processedIDs[toolUseID] = true
			if stopReason == "" {
				stopReason = "tool_use"
			}
			return closeStreamingTool(toolUseID)
		}
		return nil
	}
	emitTextDelta := func(text string, allowWhitespace bool) error {
		if text == "" || (!allowWhitespace && strings.TrimSpace(text) == "") {
			return nil
		}
		if err := closeOpenStreamingTool(); err != nil {
			return err
		}
		if !textBlockOpen && !allowWhitespace {
			if pendingLeadingWhitespace != "" {
				text = strings.TrimLeftFunc(pendingLeadingWhitespace+text, unicode.IsSpace)
				pendingLeadingWhitespace = ""
				if text == "" {
					return nil
				}
			}
		}
		if err := ensureMessageStart(); err != nil {
			return err
		}
		sawNonThinkingBlock = true
		if firstDelta == nil {
			delta := time.Since(start)
			firstDelta = &delta
		}
		if err := closeThinking(); err != nil {
			return err
		}
		if !textBlockOpen {
			contentBlockIndex++
			textBlockOpen = true
			if err := writeEvent("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": contentBlockIndex,
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			}); err != nil {
				return err
			}
		}
		return writeEvent("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": contentBlockIndex,
			"delta": map[string]any{
				"type": "text_delta",
				"text": text,
			},
		})
	}
	emitToolUse := func(tool KiroToolUse) error {
		if !shouldEmitToolUse(tool, emittedToolContents) {
			return nil
		}
		sawNonThinkingBlock = true
		if err := closeOpenStreamingTool(); err != nil {
			return err
		}
		if err := ensureMessageStart(); err != nil {
			return err
		}
		if err := closeText(); err != nil {
			return err
		}
		if err := closeThinking(); err != nil {
			return err
		}
		contentBlockIndex++
		if err := writeEvent("content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": contentBlockIndex,
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    tool.ToolUseID,
				"name":  restoreResponseToolName(tool.Name, requestCtx),
				"input": map[string]any{},
			},
		}); err != nil {
			return err
		}
		inputJSON, _ := json.Marshal(tool.Input)
		if err := writeEvent("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": contentBlockIndex,
			"delta": map[string]any{
				"type":         "input_json_delta",
				"partial_json": string(inputJSON),
			},
		}); err != nil {
			return err
		}
		return writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": contentBlockIndex})
	}
	flushPendingAssistantText := func() error {
		text, embeddedTools, pending := drainEmbeddedToolText(pendingAssistantText)
		pendingAssistantText = pending
		if err := emitTextDelta(text, false); err != nil {
			return err
		}
		for _, tool := range embeddedTools {
			if err := emitToolUse(tool); err != nil {
				return err
			}
		}
		return nil
	}
	emitPlainAssistantText := func(text string) error {
		if text == "" {
			return nil
		}
		pendingAssistantText += text
		return flushPendingAssistantText()
	}
	startThinkingBlock := func() error {
		if err := closeOpenStreamingTool(); err != nil {
			return err
		}
		if err := closeText(); err != nil {
			return err
		}
		if err := ensureMessageStart(); err != nil {
			return err
		}
		if firstDelta == nil {
			delta := time.Since(start)
			firstDelta = &delta
		}
		if thinkingBlockOpen {
			return nil
		}
		contentBlockIndex++
		thinkingBlockIndex = contentBlockIndex
		thinkingBlockOpen = true
		return writeEvent("content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": thinkingBlockIndex,
			"content_block": map[string]any{
				"type":     "thinking",
				"thinking": "",
			},
		})
	}
	emitThinkingDelta := func(text string) error {
		if !thinkingBlockOpen {
			if err := startThinkingBlock(); err != nil {
				return err
			}
		}
		return writeEvent("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": thinkingBlockIndex,
			"delta": map[string]any{
				"type":     "thinking_delta",
				"thinking": text,
			},
		})
	}
	finishThinkingBlock := func() error {
		if err := emitThinkingDelta(""); err != nil {
			return err
		}
		return closeThinking()
	}
	processThinkingTaggedText := func(text string) error {
		if text == "" {
			return nil
		}
		thinkingBuffer += text
		for {
			if !inThinkingBlock {
				startPos := findRealThinkingStartTag(thinkingBuffer, 0)
				if startPos != -1 {
					before := thinkingBuffer[:startPos]
					if strings.TrimSpace(before) != "" {
						if err := emitPlainAssistantText(before); err != nil {
							return err
						}
					}
					inThinkingBlock = true
					stripThinkingLeadingNewline = true
					thinkingBuffer = thinkingBuffer[startPos+len(thinkingStartTag):]
					if err := startThinkingBlock(); err != nil {
						return err
					}
					continue
				}
				safeLen := safeThinkingStreamFlushLen(thinkingBuffer, len(thinkingStartTag))
				if safeLen > 0 {
					safeText := thinkingBuffer[:safeLen]
					if strings.TrimSpace(safeText) != "" {
						if err := emitPlainAssistantText(safeText); err != nil {
							return err
						}
						thinkingBuffer = thinkingBuffer[safeLen:]
					}
				}
				break
			}
			if stripThinkingLeadingNewline {
				if strings.HasPrefix(thinkingBuffer, "\n") {
					thinkingBuffer = thinkingBuffer[1:]
					stripThinkingLeadingNewline = false
				} else if thinkingBuffer != "" {
					stripThinkingLeadingNewline = false
				}
			}
			endPos := findStreamThinkingEndTagStrict(thinkingBuffer, 0)
			if endPos != -1 {
				if thinkingText := thinkingBuffer[:endPos]; thinkingText != "" {
					if err := emitThinkingDelta(thinkingText); err != nil {
						return err
					}
				}
				inThinkingBlock = false
				if err := finishThinkingBlock(); err != nil {
					return err
				}
				thinkingBuffer = thinkingBuffer[endPos+len(thinkingEndTag)+len("\n\n"):]
				continue
			}
			safeLen := safeThinkingStreamFlushLen(thinkingBuffer, len(thinkingEndTag)+len("\n\n"))
			if safeLen > 0 {
				if err := emitThinkingDelta(thinkingBuffer[:safeLen]); err != nil {
					return err
				}
				thinkingBuffer = thinkingBuffer[safeLen:]
			}
			break
		}
		return nil
	}
	flushThinkingAtBoundary := func() error {
		if !requestCtx.ThinkingEnabled || thinkingBuffer == "" {
			return nil
		}
		if inThinkingBlock {
			endPos := findStreamThinkingEndTagAtBufferEnd(thinkingBuffer, 0)
			if endPos != -1 {
				if thinkingText := thinkingBuffer[:endPos]; thinkingText != "" {
					if err := emitThinkingDelta(thinkingText); err != nil {
						return err
					}
				}
				afterPos := endPos + len(thinkingEndTag)
				remaining := strings.TrimLeftFunc(thinkingBuffer[afterPos:], unicode.IsSpace)
				thinkingBuffer = ""
				inThinkingBlock = false
				if err := finishThinkingBlock(); err != nil {
					return err
				}
				return emitPlainAssistantText(remaining)
			}
			if err := emitThinkingDelta(thinkingBuffer); err != nil {
				return err
			}
			thinkingBuffer = ""
			inThinkingBlock = false
			return finishThinkingBlock()
		}
		remaining := thinkingBuffer
		thinkingBuffer = ""
		return emitPlainAssistantText(remaining)
	}
	flushThinkingAtEOF := func() error {
		if !requestCtx.ThinkingEnabled {
			return nil
		}
		return flushThinkingAtBoundary()
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		msg, err := readEventStreamMessage(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if msg == nil || len(msg.Payload) == 0 {
			continue
		}

		var event map[string]any
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			continue
		}

		if sr := readStopReason(event); sr != "" {
			stopReason = sr
		}

		switch msg.EventType {
		case "assistantResponseEvent":
			assistant := nestedEvent(event, "assistantResponseEvent")
			if sr := readStopReason(assistant); sr != "" {
				stopReason = sr
			}
			text := getString(assistant, "content")
			if text == "" {
				text = getString(event, "content")
			}
			if text != "" {
				if requestCtx.ThinkingEnabled {
					if err := processThinkingTaggedText(text); err != nil {
						return nil, err
					}
				} else {
					pendingAssistantText += text
					if err := flushPendingAssistantText(); err != nil {
						return nil, err
					}
				}
			}
			for _, tool := range readToolUses(assistant, event) {
				if processedIDs[tool.ToolUseID] {
					continue
				}
				processedIDs[tool.ToolUseID] = true
				if err := flushThinkingAtBoundary(); err != nil {
					return nil, err
				}
				if err := emitToolUse(tool); err != nil {
					return nil, err
				}
			}
		case "reasoningContentEvent":
			reasoning := nestedEvent(event, "reasoningContentEvent")
			text := getString(reasoning, "text")
			if text == "" {
				text = getString(event, "text")
			}
			if text == "" {
				continue
			}
			if requestCtx.ThinkingEnabled {
				wrapped := thinkingStartTag + text + thinkingEndTag + "\n\n"
				if err := processThinkingTaggedText(wrapped); err != nil {
					return nil, err
				}
			}
		case "toolUseEvent":
			if err := flushThinkingAtBoundary(); err != nil {
				return nil, err
			}
			if err := processStreamingToolUseEvent(event); err != nil {
				return nil, err
			}
		case "messageMetadataEvent", "metadataEvent", "supplementaryWebLinksEvent", "usageEvent", "messageStopEvent", "message_stop":
			updateUsageFromEvent(&usage, msg.EventType, event)
		default:
			updateUsageFromEvent(&usage, msg.EventType, event)
		}
	}

	if err := closeOpenStreamingTool(); err != nil {
		return nil, err
	}
	if err := flushThinkingAtEOF(); err != nil {
		return nil, err
	}
	if err := flushPendingAssistantText(); err != nil {
		return nil, err
	}
	if requestCtx.ThinkingEnabled && thinkingBlockIndex != -1 && !sawNonThinkingBlock {
		stopReason = "max_tokens"
		if err := emitTextDelta(" ", true); err != nil {
			return nil, err
		}
	}

	if err := closeText(); err != nil {
		return nil, err
	}
	if err := closeThinking(); err != nil {
		return nil, err
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	if stopReason == "" {
		if len(emittedToolContents) > 0 {
			stopReason = "tool_use"
		} else {
			stopReason = "end_turn"
		}
	}
	if err := ensureMessageStart(); err != nil {
		return nil, err
	}
	if err := writeEvent("message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"input_tokens":                usage.InputTokens,
			"output_tokens":               usage.OutputTokens,
			"cache_read_input_tokens":     usage.CacheReadInputTokens,
			"cache_creation_input_tokens": 0,
		},
	}); err != nil {
		return nil, err
	}
	if err := writeEvent("message_stop", map[string]any{"type": "message_stop"}); err != nil {
		return nil, err
	}

	return &StreamResult{
		Usage:         usage,
		StopReason:    stopReason,
		FirstDeltaDur: firstDelta,
	}, nil
}

func extractSystemPrompt(claudeBody []byte) string {
	systemField := gjson.GetBytes(claudeBody, "system")
	if systemField.IsArray() {
		var sb strings.Builder
		for _, block := range systemField.Array() {
			if block.Get("type").String() == "text" {
				_, _ = sb.WriteString(block.Get("text").String())
			} else if block.Type == gjson.String {
				_, _ = sb.WriteString(block.String())
			}
		}
		return sb.String()
	}
	return systemField.String()
}

//nolint:unused // exercised from same-package tests and kept as a focused helper for thinking-header behavior.
func isThinkingEnabledWithHeaders(body []byte, headers http.Header) bool {
	return deriveThinkingDirective(body, headers) != nil
}

func deriveThinkingDirective(body []byte, headers http.Header) *thinkingDirective {
	if override := thinkingDirectiveFromModel(gjson.GetBytes(body, "model").String()); override != nil {
		return override
	}
	switch thinkingType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "thinking.type").String())); thinkingType {
	case "adaptive":
		effort := strings.TrimSpace(gjson.GetBytes(body, "output_config.effort").String())
		if effort == "" {
			effort = "high"
		}
		budget := int(gjson.GetBytes(body, "thinking.budget_tokens").Int())
		if budget <= 0 {
			budget = 20000
		}
		return &thinkingDirective{Mode: "adaptive", BudgetTokens: budget, Effort: effort}
	case "enabled":
		budget := int(gjson.GetBytes(body, "thinking.budget_tokens").Int())
		if budget <= 0 {
			budget = 16000
		}
		return &thinkingDirective{Mode: "enabled", BudgetTokens: budget}
	}
	if headers != nil {
		if beta := headers.Get("Anthropic-Beta"); strings.Contains(beta, "interleaved-thinking") {
			return &thinkingDirective{Mode: "enabled", BudgetTokens: 16000}
		}
	}
	if effort := gjson.GetBytes(body, "reasoning_effort").String(); effort != "" && effort != "none" {
		return &thinkingDirective{Mode: "enabled", BudgetTokens: 16000}
	}
	model := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "model").String()))
	if strings.Contains(model, "-reason") {
		return &thinkingDirective{Mode: "enabled", BudgetTokens: 16000}
	}
	return nil
}

func thinkingDirectiveFromModel(model string) *thinkingDirective {
	model = strings.ToLower(strings.TrimSpace(model))
	if !strings.Contains(model, "thinking") {
		return nil
	}

	switch normalizeModelAlias(model) {
	case "claude-opus-4-6", "claude-opus-4.6":
		return &thinkingDirective{
			Mode:         "adaptive",
			BudgetTokens: 20000,
			Effort:       "high",
		}
	default:
		return &thinkingDirective{
			Mode:         "enabled",
			BudgetTokens: 20000,
		}
	}
}

func buildInjectedSystemPrompt(systemPrompt string, thinking *thinkingDirective, toolChoiceHint string) string {
	systemPrompt = strings.TrimSpace(systemPrompt)
	timestampContext := fmt.Sprintf("[Context: Current time is %s]", time.Now().Format("2006-01-02 15:04:05 MST"))
	if systemPrompt == "" {
		systemPrompt = timestampContext
	} else {
		systemPrompt = timestampContext + "\n\n" + systemPrompt
	}
	if toolChoiceHint != "" {
		if systemPrompt != "" {
			systemPrompt += "\n"
		}
		systemPrompt += toolChoiceHint
	}
	if !strings.Contains(systemPrompt, systemChunkedWritePolicy) {
		systemPrompt += "\n" + systemChunkedWritePolicy
	}
	if thinking != nil {
		switch thinking.Mode {
		case "adaptive":
			effort := strings.TrimSpace(thinking.Effort)
			if effort == "" {
				effort = "high"
			}
			thinkingPrefix := "<thinking_mode>adaptive</thinking_mode>\n<thinking_effort>" + effort + "</thinking_effort>"
			return thinkingPrefix + "\n\n" + systemPrompt
		default:
			budget := thinking.BudgetTokens
			if budget <= 0 {
				budget = 16000
			}
			thinkingPrefix := "<thinking_mode>enabled</thinking_mode>\n<max_thinking_length>" + strconv.Itoa(budget) + "</max_thinking_length>"
			return thinkingPrefix + "\n\n" + systemPrompt
		}
	}
	return systemPrompt
}

func extractClaudeToolChoiceHint(claudeBody []byte, requestCtx *KiroRequestContext) string {
	toolChoice := gjson.GetBytes(claudeBody, "tool_choice")
	if !toolChoice.Exists() {
		return ""
	}

	if toolChoice.Type == gjson.String {
		switch strings.ToLower(strings.TrimSpace(toolChoice.String())) {
		case "none":
			return "[INSTRUCTION: Do not use any tools. Respond with text only.]"
		case "auto", "":
			return ""
		}
	}

	switch strings.ToLower(strings.TrimSpace(toolChoice.Get("type").String())) {
	case "any":
		return "[INSTRUCTION: You MUST use at least one of the available tools to respond. Do not respond with text only - always make a tool call.]"
	case "tool":
		toolName := mapKiroToolName(toolChoice.Get("name").String(), requestCtx)
		if toolName != "" {
			return fmt.Sprintf("[INSTRUCTION: You MUST use the tool named '%s' to respond. Do not use any other tool or respond with text only.]", toolName)
		}
	case "none":
		return "[INSTRUCTION: Do not use any tools. Respond with text only.]"
	}

	return ""
}

func isToolChoiceNone(claudeBody []byte) bool {
	toolChoice := gjson.GetBytes(claudeBody, "tool_choice")
	if !toolChoice.Exists() {
		return false
	}
	if toolChoice.Type == gjson.String {
		return strings.EqualFold(strings.TrimSpace(toolChoice.String()), "none")
	}
	return strings.EqualFold(strings.TrimSpace(toolChoice.Get("type").String()), "none")
}

//nolint:unused // exposed for focused tool-alias regression tests in this package.
func kiroToolNameAlias(name string) string {
	return mapKiroToolName(name, nil)
}

func prependSystemHistory(history []KiroHistoryMessage, systemPrompt, modelID, origin string) []KiroHistoryMessage {
	systemPrompt = strings.TrimSpace(systemPrompt)
	if systemPrompt == "" {
		return history
	}

	prefix := []KiroHistoryMessage{
		{
			UserInputMessage: &KiroUserInputMessage{
				Content: systemPrompt,
				ModelID: modelID,
				Origin:  origin,
			},
		},
		{
			AssistantResponseMessage: &KiroAssistantResponseMessage{
				Content: "I will follow these instructions.",
			},
		},
	}

	return append(prefix, history...)
}

func normalizeOrigin(origin string) string {
	switch origin {
	case "KIRO_CLI", "AMAZON_Q":
		return "CLI"
	case "KIRO_AI_EDITOR", "KIRO_IDE", "":
		return "AI_EDITOR"
	default:
		return origin
	}
}

func extractMetadataFromMessages(messages gjson.Result, key string) string {
	arr := messages.Array()
	for i := len(arr) - 1; i >= 0; i-- {
		if val := arr[i].Get("additional_kwargs." + key); val.Exists() && val.String() != "" {
			return val.String()
		}
	}
	return ""
}

func convertClaudeToolsToKiro(tools gjson.Result, requestCtx *KiroRequestContext) []KiroToolWrapper {
	if !tools.IsArray() {
		return nil
	}
	var out []KiroToolWrapper
	for _, tool := range tools.Array() {
		originalName := tool.Get("name").String()
		if strings.TrimSpace(originalName) == "" {
			originalName = tool.Get("type").String()
		}
		isWebSearch := strings.TrimSpace(originalName) == "web_search"
		name := mapKiroToolName(originalName, requestCtx)
		description := strings.TrimSpace(tool.Get("description").String())
		if isWebSearch {
			if cached := GetCachedWebSearchDescription(); cached != "" {
				description = cached
			} else {
				description = remoteWebSearchDescription
			}
		}
		if description == "" {
			description = "Tool: " + name
		}
		description = appendChunkedToolDescription(originalName, description)
		description = truncateKiroToolDescription(description)
		inputSchema := normalizeKiroJSONSchema(tool.Get("input_schema").Value())
		out = append(out, KiroToolWrapper{
			ToolSpecification: KiroToolSpecification{
				Name:        name,
				Description: description,
				InputSchema: KiroInputSchema{JSON: inputSchema},
			},
		})
	}
	return out
}

func appendChunkedToolDescription(name, description string) string {
	suffix := chunkedToolDescriptionSuffix(name)
	if suffix == "" {
		return description
	}
	description = strings.Replace(description, suffix, "", 1)
	if strings.TrimSpace(description) == "" {
		return suffix
	}
	base := strings.TrimRight(description, "\n")
	joined := base + "\n" + suffix
	if len(joined) <= kiroMaxToolDescLen {
		return joined
	}
	const truncationMarker = "... (description truncated)"
	baseLimit := kiroMaxToolDescLen - len(suffix) - 1 - len(truncationMarker)
	if baseLimit <= 0 {
		return truncateKiroToolDescription(joined)
	}
	return truncateUTF8(base, baseLimit) + truncationMarker + "\n" + suffix
}

func chunkedToolDescriptionSuffix(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "write", "write_to_file", "fswrite", "create_file":
		return writeToolDescriptionSuffix
	case "edit", "edit_file", "str_replace_editor", "apply_diff":
		return editToolDescriptionSuffix
	default:
		return ""
	}
}

func truncateKiroToolDescription(description string) string {
	if len(description) <= kiroMaxToolDescLen {
		return description
	}
	return truncateUTF8(description, kiroMaxToolDescLen-30) + "... (description truncated)"
}

func truncateUTF8(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit]
}

func shortenToolNameIfNeeded(name string) string {
	name = strings.TrimSpace(name)
	if len(name) <= kiroMaxToolNameLen {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	suffix := fmt.Sprintf("%x", sum[:])[:8]
	prefixLen := kiroMaxToolNameLen - 1 - len(suffix)
	prefix := name
	if len(prefix) > prefixLen {
		prefix = prefix[:prefixLen]
		for len(prefix) > 0 && !utf8.ValidString(prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix + "_" + suffix
}

func mapKiroToolName(name string, requestCtx *KiroRequestContext) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if name == "web_search" {
		return "remote_web_search"
	}
	short := shortenToolNameIfNeeded(name)
	if short != name && requestCtx != nil {
		if requestCtx.ToolNameMap == nil {
			requestCtx.ToolNameMap = make(map[string]string)
		}
		requestCtx.ToolNameMap[short] = name
	}
	return short
}

func normalizeKiroJSONSchema(schema any) any {
	return normalizeKiroJSONSchemaValue(schema, true)
}

func normalizeKiroJSONSchemaValue(schema any, enforceObjectKeywords bool) any {
	obj, ok := schema.(map[string]any)
	if !ok || obj == nil {
		return defaultKiroJSONSchema()
	}
	normalized := make(map[string]any, len(obj)+4)
	for key, value := range obj {
		normalized[key] = normalizeSchemaChild(key, value)
	}
	if typ, ok := normalized["type"].(string); !ok || strings.TrimSpace(typ) == "" {
		normalized["type"] = "object"
	}
	typ, _ := normalized["type"].(string)
	needsObjectKeywords := enforceObjectKeywords ||
		strings.TrimSpace(typ) == "object" ||
		hasSchemaKey(normalized, "properties") ||
		hasSchemaKey(normalized, "required") ||
		hasSchemaKey(normalized, "additionalProperties")
	if needsObjectKeywords {
		properties, ok := normalized["properties"].(map[string]any)
		if !ok || properties == nil {
			normalized["properties"] = map[string]any{}
		} else {
			for key, value := range properties {
				properties[key] = normalizeKiroJSONSchemaValue(value, false)
			}
			normalized["properties"] = properties
		}
		normalized["required"] = normalizeSchemaRequired(normalized["required"])
		switch additional := normalized["additionalProperties"].(type) {
		case bool:
		case map[string]any:
			normalized["additionalProperties"] = normalizeKiroJSONSchemaValue(additional, false)
		default:
			normalized["additionalProperties"] = true
		}
	}
	return normalized
}

func hasSchemaKey(schema map[string]any, key string) bool {
	_, ok := schema[key]
	return ok
}

func defaultKiroJSONSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"required":             []any{},
		"additionalProperties": true,
	}
}

func normalizeSchemaRequired(value any) []any {
	arr, ok := value.([]any)
	if !ok {
		return []any{}
	}
	out := make([]any, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func normalizeSchemaChild(key string, value any) any {
	switch key {
	case "items", "not":
		if obj, ok := value.(map[string]any); ok {
			return normalizeKiroJSONSchemaValue(obj, false)
		}
		if arr, ok := value.([]any); ok {
			out := make([]any, 0, len(arr))
			for _, item := range arr {
				out = append(out, normalizeKiroJSONSchemaValue(item, false))
			}
			return out
		}
	case "oneOf", "anyOf", "allOf":
		if arr, ok := value.([]any); ok {
			out := make([]any, 0, len(arr))
			for _, item := range arr {
				out = append(out, normalizeKiroJSONSchemaValue(item, false))
			}
			return out
		}
	}
	return value
}

func processMessages(messages gjson.Result, modelID, origin string, requestCtx *KiroRequestContext) ([]KiroHistoryMessage, *KiroUserInputMessage, []KiroToolResult) {
	messagesArray := mergeAdjacentMessages(messages.Array())
	if len(messagesArray) > 0 && messagesArray[0].Get("role").String() == "assistant" {
		messagesArray = append([]gjson.Result{gjson.Parse(`{"role":"user","content":"."}`)}, messagesArray...)
	}

	var history []KiroHistoryMessage
	var currentUserMsg *KiroUserInputMessage
	var currentToolResults []KiroToolResult

	for i, msg := range messagesArray {
		role := msg.Get("role").String()
		last := i == len(messagesArray)-1
		switch role {
		case "user":
			userMsg, toolResults := buildUserMessageStruct(msg, modelID, origin)
			if strings.TrimSpace(userMsg.Content) == "" {
				if len(toolResults) > 0 {
					userMsg.Content = "Tool results provided."
				} else {
					userMsg.Content = "Continue"
				}
			}
			if last {
				currentUserMsg = &userMsg
				currentToolResults = toolResults
			} else {
				if len(toolResults) > 0 {
					userMsg.UserInputMessageContext = &KiroUserInputMessageContext{ToolResults: toolResults}
				}
				history = append(history, KiroHistoryMessage{UserInputMessage: &userMsg})
			}
		case "assistant":
			assistantMsg := buildAssistantMessageStruct(msg, requestCtx)
			if last {
				history = append(history, KiroHistoryMessage{AssistantResponseMessage: &assistantMsg})
				currentUserMsg = &KiroUserInputMessage{
					Content: "Continue",
					ModelID: modelID,
					Origin:  origin,
				}
			} else {
				history = append(history, KiroHistoryMessage{AssistantResponseMessage: &assistantMsg})
			}
		}
	}

	return history, currentUserMsg, currentToolResults
}

func validateToolPairing(history []KiroHistoryMessage, currentToolResults []KiroToolResult) ([]KiroToolResult, map[string]bool) {
	allToolUseIDs := make(map[string]bool)
	pairedToolUseIDs := make(map[string]bool)
	for _, h := range history {
		if h.AssistantResponseMessage != nil {
			for _, tu := range h.AssistantResponseMessage.ToolUses {
				allToolUseIDs[tu.ToolUseID] = true
			}
		}
		if h.UserInputMessage != nil && h.UserInputMessage.UserInputMessageContext != nil {
			for _, tr := range h.UserInputMessage.UserInputMessageContext.ToolResults {
				pairedToolUseIDs[tr.ToolUseID] = true
			}
		}
	}

	filtered := currentToolResults[:0]
	for _, tr := range currentToolResults {
		if allToolUseIDs[tr.ToolUseID] && !pairedToolUseIDs[tr.ToolUseID] {
			filtered = append(filtered, tr)
			pairedToolUseIDs[tr.ToolUseID] = true
		}
	}
	orphaned := make(map[string]bool)
	for toolUseID := range allToolUseIDs {
		if !pairedToolUseIDs[toolUseID] {
			orphaned[toolUseID] = true
		}
	}
	return filtered, orphaned
}

func removeOrphanedToolUses(history []KiroHistoryMessage, orphaned map[string]bool) {
	if len(orphaned) == 0 {
		return
	}
	for i := range history {
		msg := history[i].AssistantResponseMessage
		if msg == nil || len(msg.ToolUses) == 0 {
			continue
		}
		filtered := msg.ToolUses[:0]
		for _, toolUse := range msg.ToolUses {
			if !orphaned[toolUse.ToolUseID] {
				filtered = append(filtered, toolUse)
			}
		}
		msg.ToolUses = filtered
	}
}

func collectHistoryToolNames(history []KiroHistoryMessage) []string {
	seen := make(map[string]bool)
	var names []string
	for _, h := range history {
		if h.AssistantResponseMessage == nil {
			continue
		}
		for _, tu := range h.AssistantResponseMessage.ToolUses {
			name := strings.TrimSpace(tu.Name)
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			if seen[key] {
				continue
			}
			seen[key] = true
			names = append(names, name)
		}
	}
	return names
}

func appendMissingPlaceholderTools(tools []KiroToolWrapper, historyToolNames []string) []KiroToolWrapper {
	if len(historyToolNames) == 0 {
		return tools
	}
	seen := make(map[string]bool)
	for _, tool := range tools {
		seen[strings.ToLower(strings.TrimSpace(tool.ToolSpecification.Name))] = true
	}
	for _, name := range historyToolNames {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		tools = append(tools, KiroToolWrapper{
			ToolSpecification: KiroToolSpecification{
				Name:        name,
				Description: "Tool used in conversation history",
				InputSchema: KiroInputSchema{JSON: normalizeKiroJSONSchema(nil)},
			},
		})
	}
	return tools
}

func buildFinalContent(content string, toolResults []KiroToolResult) string {
	if strings.TrimSpace(content) == "" {
		if len(toolResults) > 0 {
			return "Tool results provided."
		}
		return "Continue"
	}
	return content
}

func deduplicateToolResults(toolResults []KiroToolResult) []KiroToolResult {
	seen := make(map[string]bool)
	out := make([]KiroToolResult, 0, len(toolResults))
	for _, tr := range toolResults {
		if seen[tr.ToolUseID] {
			continue
		}
		seen[tr.ToolUseID] = true
		out = append(out, tr)
	}
	return out
}

func buildUserMessageStruct(msg gjson.Result, modelID, origin string) (KiroUserInputMessage, []KiroToolResult) {
	content := msg.Get("content")
	var contentBuilder strings.Builder
	var toolResults []KiroToolResult
	var images []KiroImage
	seenToolUseIDs := make(map[string]bool)

	if content.IsArray() {
		for _, part := range content.Array() {
			switch part.Get("type").String() {
			case "text":
				_, _ = contentBuilder.WriteString(part.Get("text").String())
			case "image":
				mediaType := part.Get("source.media_type").String()
				data := part.Get("source.data").String()
				format := ""
				if idx := strings.LastIndex(mediaType, "/"); idx != -1 {
					format = mediaType[idx+1:]
				}
				if format != "" && data != "" {
					images = append(images, KiroImage{
						Format: format,
						Source: KiroImageSource{Bytes: data},
					})
				}
			case "tool_result":
				toolUseID := part.Get("tool_use_id").String()
				if toolUseID == "" || seenToolUseIDs[toolUseID] {
					continue
				}
				seenToolUseIDs[toolUseID] = true
				status := "success"
				if part.Get("is_error").Bool() {
					status = "error"
				}
				textContents := []KiroTextContent{{Text: "Tool use was cancelled by the user"}}
				resultContent := part.Get("content")
				if resultContent.IsArray() {
					textContents = textContents[:0]
					for _, item := range resultContent.Array() {
						if item.Get("type").String() == "text" {
							textContents = append(textContents, KiroTextContent{Text: item.Get("text").String()})
						} else if item.Type == gjson.String {
							textContents = append(textContents, KiroTextContent{Text: item.String()})
						}
					}
				} else if resultContent.Type == gjson.String {
					textContents = []KiroTextContent{{Text: resultContent.String()}}
				}
				toolResults = append(toolResults, KiroToolResult{
					ToolUseID: toolUseID,
					Content:   textContents,
					Status:    status,
				})
			}
		}
	} else {
		_, _ = contentBuilder.WriteString(content.String())
	}

	userMsg := KiroUserInputMessage{
		Content: contentBuilder.String(),
		ModelID: modelID,
		Origin:  origin,
	}
	if len(images) > 0 {
		userMsg.Images = images
	}
	return userMsg, toolResults
}

func buildAssistantMessageStruct(msg gjson.Result, requestCtx *KiroRequestContext) KiroAssistantResponseMessage {
	content := msg.Get("content")
	var contentBuilder strings.Builder
	var toolUses []KiroToolUse

	if content.IsArray() {
		for _, part := range content.Array() {
			switch part.Get("type").String() {
			case "text":
				_, _ = contentBuilder.WriteString(part.Get("text").String())
			case "tool_use":
				toolName := mapKiroToolName(part.Get("name").String(), requestCtx)
				input := map[string]any{}
				toolInput := part.Get("input")
				if toolInput.IsObject() {
					toolInput.ForEach(func(key, value gjson.Result) bool {
						input[key.String()] = value.Value()
						return true
					})
				}
				toolUses = append(toolUses, KiroToolUse{
					ToolUseID: part.Get("id").String(),
					Name:      toolName,
					Input:     input,
				})
			}
		}
	} else {
		_, _ = contentBuilder.WriteString(content.String())
	}

	finalContent := contentBuilder.String()
	if strings.TrimSpace(finalContent) == "" {
		finalContent = " "
	}
	return KiroAssistantResponseMessage{
		Content:  finalContent,
		ToolUses: toolUses,
	}
}

func mergeAdjacentMessages(messages []gjson.Result) []gjson.Result {
	if len(messages) <= 1 {
		return messages
	}
	var merged []gjson.Result
	for _, msg := range messages {
		if len(merged) == 0 {
			merged = append(merged, msg)
			continue
		}
		lastMsg := merged[len(merged)-1]
		role := msg.Get("role").String()
		lastRole := lastMsg.Get("role").String()
		if role == "tool" || lastRole == "tool" || role != lastRole {
			merged = append(merged, msg)
			continue
		}
		mergedMsg := map[string]any{
			"role":    role,
			"content": json.RawMessage(mergeMessageContent(lastMsg, msg)),
		}
		encoded, _ := json.Marshal(mergedMsg)
		merged[len(merged)-1] = gjson.ParseBytes(encoded)
	}
	return merged
}

func mergeMessageContent(msg1, msg2 gjson.Result) string {
	var blocks1, blocks2 []map[string]any
	content1 := msg1.Get("content")
	content2 := msg2.Get("content")
	if content1.IsArray() {
		for _, block := range content1.Array() {
			blocks1 = append(blocks1, blockToMap(block))
		}
	} else if content1.Type == gjson.String {
		blocks1 = append(blocks1, map[string]any{"type": "text", "text": content1.String()})
	}
	if content2.IsArray() {
		for _, block := range content2.Array() {
			blocks2 = append(blocks2, blockToMap(block))
		}
	} else if content2.Type == gjson.String {
		blocks2 = append(blocks2, map[string]any{"type": "text", "text": content2.String()})
	}
	if len(blocks1) > 0 && len(blocks2) > 0 && blocks1[len(blocks1)-1]["type"] == "text" && blocks2[0]["type"] == "text" {
		leftText, leftOK := blocks1[len(blocks1)-1]["text"].(string)
		rightText, rightOK := blocks2[0]["text"].(string)
		if leftOK && rightOK {
			blocks1[len(blocks1)-1]["text"] = leftText + "\n\n" + rightText
			blocks2 = blocks2[1:]
		}
	}
	allBlocks := append(blocks1, blocks2...)
	result, _ := json.Marshal(allBlocks)
	return string(result)
}

func blockToMap(block gjson.Result) map[string]any {
	result := make(map[string]any)
	block.ForEach(func(key, value gjson.Result) bool {
		if value.IsObject() {
			result[key.String()] = blockToMap(value)
		} else if value.IsArray() {
			var arr []any
			for _, item := range value.Array() {
				if item.IsObject() {
					arr = append(arr, blockToMap(item))
				} else {
					arr = append(arr, item.Value())
				}
			}
			result[key.String()] = arr
		} else {
			result[key.String()] = value.Value()
		}
		return true
	})
	return result
}

func parseEventStream(body io.Reader) (string, []KiroToolUse, Usage, string, error) {
	reader := bufio.NewReader(body)
	var content strings.Builder
	var toolUses []KiroToolUse
	var usage Usage
	stopReason := ""
	processedIDs := make(map[string]bool)
	var currentTool *toolUseState

	for {
		msg, err := readEventStreamMessage(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, usage, stopReason, err
		}
		if msg == nil || len(msg.Payload) == 0 {
			continue
		}

		var event map[string]any
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			continue
		}
		if sr := readStopReason(event); sr != "" {
			stopReason = sr
		}
		switch msg.EventType {
		case "assistantResponseEvent":
			assistant := nestedEvent(event, "assistantResponseEvent")
			if text := getString(assistant, "content"); text != "" {
				_, _ = content.WriteString(text)
			} else if text := getString(event, "content"); text != "" {
				_, _ = content.WriteString(text)
			}
			if sr := readStopReason(assistant); sr != "" {
				stopReason = sr
			}
			for _, tool := range readToolUses(assistant, event) {
				if processedIDs[tool.ToolUseID] {
					continue
				}
				processedIDs[tool.ToolUseID] = true
				toolUses = append(toolUses, tool)
			}
		case "toolUseEvent":
			completed, next := processToolUseEvent(event, currentTool, processedIDs)
			currentTool = next
			toolUses = append(toolUses, completed...)
		case "reasoningContentEvent":
			reasoning := nestedEvent(event, "reasoningContentEvent")
			text := getString(reasoning, "text")
			if text == "" {
				text = getString(event, "text")
			}
			if text != "" {
				_, _ = content.WriteString(thinkingStartTag)
				_, _ = content.WriteString(text)
				_, _ = content.WriteString(thinkingEndTag)
			}
		default:
			updateUsageFromEvent(&usage, msg.EventType, event)
		}
	}

	if currentTool != nil && currentTool.ToolUseID != "" && !processedIDs[currentTool.ToolUseID] {
		completed, _ := processToolUseEvent(map[string]any{
			"toolUseEvent": map[string]any{
				"toolUseId": currentTool.ToolUseID,
				"name":      currentTool.Name,
				"stop":      true,
				"input":     currentTool.InputBuffer.String(),
			},
		}, currentTool, processedIDs)
		toolUses = append(toolUses, completed...)
	}
	cleanText, embeddedToolUses, _ := drainEmbeddedToolText(content.String())
	toolUses = append(toolUses, embeddedToolUses...)
	toolUses = deduplicateToolUses(toolUses)

	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	if stopReason == "" {
		if hasUsableToolUses(toolUses) {
			stopReason = "tool_use"
		} else {
			stopReason = "end_turn"
		}
	}
	return cleanText, toolUses, usage, stopReason, nil
}

func buildClaudeResponse(content string, toolUses []KiroToolUse, model string, usage Usage, stopReason string, requestCtx KiroRequestContext) []byte {
	var blocks []map[string]any
	blocks = append(blocks, extractThinkingBlocks(content)...)
	usableTools := 0
	for _, tool := range toolUses {
		if tool.IsTruncated {
			continue
		}
		usableTools++
		blocks = append(blocks, map[string]any{
			"type":  "tool_use",
			"id":    tool.ToolUseID,
			"name":  restoreResponseToolName(tool.Name, requestCtx),
			"input": tool.Input,
		})
	}
	pureThinking := hasThinkingBlocksOnly(blocks) && usableTools == 0
	if pureThinking {
		blocks = append(blocks, map[string]any{"type": "text", "text": ""})
		stopReason = "max_tokens"
	}
	if len(blocks) == 0 {
		blocks = append(blocks, map[string]any{"type": "text", "text": ""})
	}
	if stopReason == "" {
		if usableTools > 0 {
			stopReason = "tool_use"
		} else {
			stopReason = "end_turn"
		}
	}
	response := map[string]any{
		"id":          "msg_" + uuid.NewString()[:24],
		"type":        "message",
		"role":        "assistant",
		"model":       model,
		"content":     blocks,
		"stop_reason": stopReason,
		"usage": map[string]any{
			"input_tokens":            usage.InputTokens,
			"output_tokens":           usage.OutputTokens,
			"cache_read_input_tokens": usage.CacheReadInputTokens,
		},
	}
	result, _ := json.Marshal(response)
	return result
}

func restoreResponseToolName(name string, requestCtx KiroRequestContext) string {
	name = strings.TrimSpace(name)
	if requestCtx.ToolNameMap == nil {
		return name
	}
	if original := strings.TrimSpace(requestCtx.ToolNameMap[name]); original != "" {
		return original
	}
	return name
}

func hasThinkingBlocksOnly(blocks []map[string]any) bool {
	if len(blocks) == 0 {
		return false
	}
	hasThinking := false
	for _, block := range blocks {
		blockType, _ := block["type"].(string)
		switch blockType {
		case "thinking":
			hasThinking = true
		case "text":
			return false
		default:
			return false
		}
	}
	return hasThinking
}

func extractThinkingBlocks(content string) []map[string]any {
	if content == "" {
		return nil
	}
	if findRealThinkingStartTag(content, 0) == -1 {
		return []map[string]any{{"type": "text", "text": content}}
	}
	var blocks []map[string]any
	pos := 0
	for pos < len(content) {
		start := findRealThinkingStartTag(content, pos)
		if start == -1 {
			if text := content[pos:]; strings.TrimSpace(text) != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": text})
			}
			break
		}
		end := findRealThinkingEndTag(content, start+len(thinkingStartTag))
		if end == -1 {
			if text := content[pos:]; strings.TrimSpace(text) != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": text})
			}
			break
		}
		if text := content[pos:start]; strings.TrimSpace(text) != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": text})
		}
		thinking := strings.TrimPrefix(content[start+len(thinkingStartTag):end], "\n")
		if strings.TrimSpace(thinking) != "" {
			blocks = append(blocks, map[string]any{
				"type":      "thinking",
				"thinking":  thinking,
				"signature": thinkingSignature(thinking),
			})
		}
		pos = end + len(thinkingEndTag)
		if strings.HasPrefix(content[pos:], "\n\n") {
			pos += len("\n\n")
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, map[string]any{"type": "text", "text": ""})
	}
	return blocks
}

func findRealThinkingStartTag(content string, from int) int {
	return findRealThinkingTag(content, thinkingStartTag, from, false)
}

func findRealThinkingEndTag(content string, from int) int {
	searchFrom := from
	for {
		pos := findRealThinkingTag(content, thinkingEndTag, searchFrom, true)
		if pos == -1 {
			return -1
		}
		after := pos + len(thinkingEndTag)
		if strings.HasPrefix(content[after:], "\n\n") || strings.TrimSpace(content[after:]) == "" {
			return pos
		}
		searchFrom = pos + 1
	}
}

func findStreamThinkingEndTagStrict(content string, from int) int {
	searchFrom := from
	for {
		pos := findRealThinkingTag(content, thinkingEndTag, searchFrom, true)
		if pos == -1 {
			return -1
		}
		after := pos + len(thinkingEndTag)
		if strings.HasPrefix(content[after:], "\n\n") {
			return pos
		}
		searchFrom = pos + 1
	}
}

func findStreamThinkingEndTagAtBufferEnd(content string, from int) int {
	searchFrom := from
	for {
		pos := findRealThinkingTag(content, thinkingEndTag, searchFrom, true)
		if pos == -1 {
			return -1
		}
		after := pos + len(thinkingEndTag)
		if strings.TrimSpace(content[after:]) == "" {
			return pos
		}
		searchFrom = pos + 1
	}
}

func safeThinkingStreamFlushLen(content string, keepBytes int) int {
	if keepBytes <= 0 || len(content) <= keepBytes {
		return 0
	}
	pos := len(content) - keepBytes
	for pos > 0 && !utf8.ValidString(content[:pos]) {
		pos--
	}
	for pos > 0 && !utf8.RuneStart(content[pos]) {
		pos--
	}
	return pos
}

func findRealThinkingTag(content, tag string, from int, allowEndBoundary bool) int {
	if from < 0 {
		from = 0
	}
	searchFrom := from
	for searchFrom < len(content) {
		rel := strings.Index(content[searchFrom:], tag)
		if rel == -1 {
			return -1
		}
		pos := searchFrom + rel
		after := pos + len(tag)
		if !isThinkingTagQuoted(content, pos, after) &&
			!isInsideMarkdownFence(content, pos) &&
			!isLineBlockQuote(content, pos) &&
			(!allowEndBoundary || after <= len(content)) {
			return pos
		}
		searchFrom = pos + 1
	}
	return -1
}

func isThinkingTagQuoted(content string, start, after int) bool {
	if start > 0 && isThinkingQuoteChar(content[start-1]) {
		return true
	}
	return after < len(content) && isThinkingQuoteChar(content[after])
}

func isThinkingQuoteChar(ch byte) bool {
	switch ch {
	case '`', '"', '\'', '\\':
		return true
	default:
		return false
	}
}

func isInsideMarkdownFence(content string, pos int) bool {
	inFence := false
	lineStart := 0
	for lineStart < pos {
		lineEnd := strings.IndexByte(content[lineStart:], '\n')
		if lineEnd == -1 {
			lineEnd = len(content)
		} else {
			lineEnd += lineStart
		}
		line := strings.TrimSpace(content[lineStart:lineEnd])
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
		}
		lineStart = lineEnd + 1
	}
	return inFence
}

func isLineBlockQuote(content string, pos int) bool {
	lineStart := strings.LastIndexByte(content[:pos], '\n') + 1
	return strings.HasPrefix(strings.TrimLeftFunc(content[lineStart:pos], unicode.IsSpace), ">")
}

func thinkingSignature(content string) string {
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func readEventStreamMessage(reader *bufio.Reader) (*eventStreamMessage, error) {
	prelude := make([]byte, 12)
	_, err := io.ReadFull(reader, prelude)
	if err != nil {
		return nil, err
	}
	totalLength := binary.BigEndian.Uint32(prelude[0:4])
	headersLength := binary.BigEndian.Uint32(prelude[4:8])
	if totalLength < minFrameSize || totalLength > maxEventMsgSize {
		return nil, fmt.Errorf("invalid kiro eventstream frame length: %d", totalLength)
	}
	if headersLength > totalLength-16 {
		return nil, fmt.Errorf("invalid kiro eventstream headers length: %d", headersLength)
	}
	remaining := make([]byte, totalLength-12)
	if _, err := io.ReadFull(reader, remaining); err != nil {
		return nil, err
	}
	eventType := extractEventType(remaining[:headersLength])
	payloadStart := headersLength
	payloadEnd := uint32(len(remaining)) - 4
	if payloadStart >= payloadEnd {
		return &eventStreamMessage{EventType: eventType}, nil
	}
	return &eventStreamMessage{
		EventType: eventType,
		Payload:   remaining[payloadStart:payloadEnd],
	}, nil
}

func extractEventType(headers []byte) string {
	offset := 0
	for offset < len(headers) {
		nameLen := int(headers[offset])
		offset++
		if offset+nameLen > len(headers) {
			break
		}
		name := string(headers[offset : offset+nameLen])
		offset += nameLen
		if offset >= len(headers) {
			break
		}
		valueType := headers[offset]
		offset++
		if valueType == 7 {
			if offset+2 > len(headers) {
				break
			}
			valueLen := int(binary.BigEndian.Uint16(headers[offset : offset+2]))
			offset += 2
			if offset+valueLen > len(headers) {
				break
			}
			value := string(headers[offset : offset+valueLen])
			offset += valueLen
			if name == ":event-type" {
				return value
			}
			continue
		}
		next, ok := skipHeaderValue(headers, offset, valueType)
		if !ok {
			break
		}
		offset = next
	}
	return ""
}

func skipHeaderValue(headers []byte, offset int, valueType byte) (int, bool) {
	switch valueType {
	case 0, 1:
		return offset, true
	case 2:
		if offset+1 > len(headers) {
			return offset, false
		}
		return offset + 1, true
	case 3:
		if offset+2 > len(headers) {
			return offset, false
		}
		return offset + 2, true
	case 4:
		if offset+4 > len(headers) {
			return offset, false
		}
		return offset + 4, true
	case 5, 8:
		if offset+8 > len(headers) {
			return offset, false
		}
		return offset + 8, true
	case 6:
		if offset+2 > len(headers) {
			return offset, false
		}
		length := int(binary.BigEndian.Uint16(headers[offset : offset+2]))
		offset += 2
		if offset+length > len(headers) {
			return offset, false
		}
		return offset + length, true
	case 9:
		if offset+16 > len(headers) {
			return offset, false
		}
		return offset + 16, true
	default:
		return offset, false
	}
}

func processToolUseEvent(event map[string]any, currentTool *toolUseState, processedIDs map[string]bool) ([]KiroToolUse, *toolUseState) {
	tu := nestedEvent(event, "toolUseEvent")
	toolUseID := getString(tu, "toolUseId")
	name := getString(tu, "name")
	isStop, _ := tu["stop"].(bool)

	var inputFragment string
	var inputMap map[string]any
	if inputRaw, ok := tu["input"]; ok {
		switch v := inputRaw.(type) {
		case string:
			inputFragment = v
		case map[string]any:
			inputMap = v
		}
	}

	if toolUseID != "" && name != "" {
		if currentTool == nil || currentTool.ToolUseID != toolUseID {
			if processedIDs[toolUseID] {
				return nil, currentTool
			}
			currentTool = &toolUseState{ToolUseID: toolUseID, Name: name}
		}
	}
	if currentTool != nil && inputFragment != "" {
		_, _ = currentTool.InputBuffer.WriteString(inputFragment)
	}
	if currentTool != nil && inputMap != nil {
		currentTool.InputBuffer.Reset()
		encoded, _ := json.Marshal(inputMap)
		_, _ = currentTool.InputBuffer.Write(encoded)
	}
	if !isStop || currentTool == nil {
		return nil, currentTool
	}
	processedIDs[currentTool.ToolUseID] = true
	return []KiroToolUse{finalizeRawToolUse(currentTool.ToolUseID, currentTool.Name, currentTool.InputBuffer.String())}, nil
}

func repairJSON(input string) string {
	str := strings.TrimSpace(input)
	if str == "" {
		return "{}"
	}
	var parsed any
	if err := json.Unmarshal([]byte(str), &parsed); err == nil {
		return str
	}
	str = escapeControlCharsInStrings(str)
	str = trailingCommaPattern.ReplaceAllString(str, "$1")
	openBraces, openBrackets, inString := jsonBalance(str)
	if inString {
		str += `"`
		openBraces, openBrackets, _ = jsonBalance(str)
	}
	if openBraces > 0 {
		str += strings.Repeat("}", openBraces)
	}
	if openBrackets > 0 {
		str += strings.Repeat("]", openBrackets)
	}
	if err := json.Unmarshal([]byte(str), &parsed); err != nil {
		return strings.TrimSpace(input)
	}
	return str
}

func escapeControlCharsInStrings(input string) string {
	var out strings.Builder
	inString := false
	escape := false
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if escape {
			_ = out.WriteByte(ch)
			escape = false
			continue
		}
		if ch == '\\' {
			_ = out.WriteByte(ch)
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			_ = out.WriteByte(ch)
			continue
		}
		if inString {
			switch ch {
			case '\n':
				_, _ = out.WriteString("\\n")
				continue
			case '\r':
				_, _ = out.WriteString("\\r")
				continue
			case '\t':
				_, _ = out.WriteString("\\t")
				continue
			}
		}
		_ = out.WriteByte(ch)
	}
	return out.String()
}

func jsonBalance(input string) (openBraces int, openBrackets int, inString bool) {
	escape := false
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if escape {
			escape = false
			continue
		}
		if ch == '\\' {
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			openBraces++
		case '}':
			openBraces--
		case '[':
			openBrackets++
		case ']':
			openBrackets--
		}
	}
	return openBraces, openBrackets, inString
}

func finalizeRawToolUse(toolUseID, name, rawInput string) KiroToolUse {
	tool := KiroToolUse{
		ToolUseID: toolUseID,
		Name:      normalizeResponseToolName(name),
		Input:     map[string]any{},
	}
	rawInput = strings.TrimSpace(rawInput)
	tool.TruncatedRaw = rawInput
	repaired := repairJSON(rawInput)
	if strings.TrimSpace(repaired) != "" {
		_ = json.Unmarshal([]byte(repaired), &tool.Input)
	}
	tool.IsTruncated = isTruncatedToolUse(tool.Name, rawInput, tool.Input)
	return tool
}

func finalizeStructuredToolUse(toolUseID, name string, input map[string]any) KiroToolUse {
	if input == nil {
		input = map[string]any{}
	}
	tool := KiroToolUse{
		ToolUseID: toolUseID,
		Name:      normalizeResponseToolName(name),
		Input:     input,
	}
	tool.IsTruncated = hasMissingRequiredFields(tool.Name, tool.Input)
	return tool
}

func normalizeResponseToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "web_search" {
		return "remote_web_search"
	}
	return name
}

func shouldEmitToolUse(tool KiroToolUse, emittedToolContents map[string]bool) bool {
	if tool.IsTruncated {
		return false
	}
	key := toolUseContentKey(tool)
	if key == "" {
		return false
	}
	if emittedToolContents[key] {
		return false
	}
	emittedToolContents[key] = true
	return true
}

func hasUsableToolUses(toolUses []KiroToolUse) bool {
	for _, tool := range toolUses {
		if !tool.IsTruncated {
			return true
		}
	}
	return false
}

func deduplicateToolUses(toolUses []KiroToolUse) []KiroToolUse {
	seenIDs := make(map[string]bool)
	seenContent := make(map[string]bool)
	out := make([]KiroToolUse, 0, len(toolUses))
	for _, tool := range toolUses {
		if tool.ToolUseID != "" {
			if seenIDs[tool.ToolUseID] {
				continue
			}
			seenIDs[tool.ToolUseID] = true
		}
		key := toolUseContentKey(tool)
		if key != "" && seenContent[key] {
			continue
		}
		if key != "" {
			seenContent[key] = true
		}
		out = append(out, tool)
	}
	return out
}

func toolUseContentKey(tool KiroToolUse) string {
	name := strings.TrimSpace(tool.Name)
	if name == "" {
		return ""
	}
	inputJSON, _ := json.Marshal(tool.Input)
	return name + ":" + string(inputJSON)
}

func drainEmbeddedToolText(text string) (cleanText string, toolUses []KiroToolUse, pending string) {
	complete, pending := splitCompleteEmbeddedToolText(text)
	if strings.TrimSpace(complete) == "" {
		return "", nil, pending
	}
	cleanText, toolUses = parseEmbeddedToolCalls(complete)
	return cleanText, deduplicateToolUses(toolUses), pending
}

func splitCompleteEmbeddedToolText(text string) (complete string, pending string) {
	searchFrom := 0
	for {
		idx := strings.Index(text[searchFrom:], embeddedToolCallPrefix)
		if idx == -1 {
			return text, ""
		}
		idx += searchFrom
		_, _, end, ok := parseEmbeddedToolCallAt(text, idx)
		if !ok {
			return text[:idx], text[idx:]
		}
		searchFrom = end
	}
}

func parseEmbeddedToolCalls(text string) (string, []KiroToolUse) {
	if !strings.Contains(text, embeddedToolCallPrefix) {
		return text, nil
	}
	var (
		builder  strings.Builder
		toolUses []KiroToolUse
		index    int
	)
	for index < len(text) {
		start := strings.Index(text[index:], embeddedToolCallPrefix)
		if start == -1 {
			_, _ = builder.WriteString(text[index:])
			break
		}
		start += index
		_, _ = builder.WriteString(text[index:start])
		tool, _, end, ok := parseEmbeddedToolCallAt(text, start)
		if !ok {
			_, _ = builder.WriteString(text[start:])
			break
		}
		toolUses = append(toolUses, tool)
		index = end
	}
	return builder.String(), toolUses
}

func parseEmbeddedToolCallAt(text string, start int) (KiroToolUse, int, int, bool) {
	if start < 0 || start >= len(text) || !strings.HasPrefix(text[start:], embeddedToolCallPrefix) {
		return KiroToolUse{}, 0, 0, false
	}
	pos := start + len(embeddedToolCallPrefix)
	argsMarker := " with args:"
	argsIndex := strings.Index(text[pos:], argsMarker)
	if argsIndex == -1 {
		return KiroToolUse{}, 0, 0, false
	}
	argsIndex += pos
	toolName := strings.TrimSpace(text[pos:argsIndex])
	if toolName == "" {
		return KiroToolUse{}, 0, 0, false
	}
	jsonStart := argsIndex + len(argsMarker)
	for jsonStart < len(text) && (text[jsonStart] == ' ' || text[jsonStart] == '\t' || text[jsonStart] == '\n') {
		jsonStart++
	}
	if jsonStart >= len(text) || text[jsonStart] != '{' {
		return KiroToolUse{}, 0, 0, false
	}
	jsonEnd := findMatchingJSONBracket(text, jsonStart)
	if jsonEnd == -1 {
		return KiroToolUse{}, 0, 0, false
	}
	end := jsonEnd + 1
	for end < len(text) && text[end] != ']' {
		end++
	}
	if end >= len(text) {
		return KiroToolUse{}, 0, 0, false
	}
	rawJSON := text[jsonStart : jsonEnd+1]
	tool := finalizeRawToolUse("toolu_"+GenerateToolUseID(), toolName, rawJSON)
	return tool, start, end + 1, true
}

func findMatchingJSONBracket(text string, start int) int {
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(text); i++ {
		ch := text[i]
		if escape {
			escape = false
			continue
		}
		if ch == '\\' {
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func isTruncatedToolUse(name, rawInput string, input map[string]any) bool {
	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		return hasToolRequirements(name)
	}
	if looksLikeTruncatedJSON(rawInput) {
		return true
	}
	return hasMissingRequiredFields(name, input)
}

func looksLikeTruncatedJSON(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] != '{' {
		return false
	}
	openBraces, openBrackets, inString := jsonBalance(raw)
	if openBraces > 0 || openBrackets > 0 || inString {
		return true
	}
	last := raw[len(raw)-1]
	return last == ':' || last == ','
}

func hasToolRequirements(name string) bool {
	_, ok := requiredToolFields[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func hasMissingRequiredFields(name string, input map[string]any) bool {
	groups, ok := requiredToolFields[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return false
	}
	for _, group := range groups {
		matched := false
		for _, candidate := range group {
			if _, exists := input[candidate]; exists {
				matched = true
				break
			}
		}
		if !matched {
			return true
		}
	}
	return false
}

func updateUsageFromEvent(usage *Usage, eventType string, event map[string]any) {
	if usage == nil {
		return
	}
	meta := nestedEvent(event, eventType)
	if len(meta) == 0 {
		meta = event
	}
	if tokenUsage, ok := meta["tokenUsage"].(map[string]any); ok {
		if value, ok := toInt(tokenUsage["uncachedInputTokens"]); ok {
			usage.InputTokens = value
		}
		if value, ok := toInt(tokenUsage["outputTokens"]); ok {
			usage.OutputTokens = value
		}
		if value, ok := toInt(tokenUsage["totalTokens"]); ok {
			usage.TotalTokens = value
		}
		if value, ok := toInt(tokenUsage["cacheReadInputTokens"]); ok {
			usage.CacheReadInputTokens = value
			if usage.InputTokens == 0 {
				usage.InputTokens = value
			} else {
				usage.InputTokens += value
			}
		}
	}
	if value, ok := toInt(event["inputTokens"]); ok && value > 0 {
		usage.InputTokens = value
	}
	if value, ok := toInt(event["outputTokens"]); ok && value > 0 {
		usage.OutputTokens = value
	}
	if value, ok := toInt(event["totalTokens"]); ok && value > 0 {
		usage.TotalTokens = value
	}
	if value, ok := toInt(meta["inputTokens"]); ok && value > 0 {
		usage.InputTokens = value
	}
	if value, ok := toInt(meta["outputTokens"]); ok && value > 0 {
		usage.OutputTokens = value
	}
	if value, ok := toInt(meta["totalTokens"]); ok && value > 0 {
		usage.TotalTokens = value
	}
}

func readToolUses(primary, fallback map[string]any) []KiroToolUse {
	var raw []any
	if value, ok := primary["toolUses"].([]any); ok {
		raw = value
	} else if value, ok := fallback["toolUses"].([]any); ok {
		raw = value
	}
	if len(raw) == 0 {
		return nil
	}
	out := make([]KiroToolUse, 0, len(raw))
	for _, item := range raw {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		input := map[string]any{}
		if value, ok := tool["input"].(map[string]any); ok {
			input = value
		}
		out = append(out, finalizeStructuredToolUse(getString(tool, "toolUseId"), getString(tool, "name"), input))
	}
	return out
}

func nestedEvent(event map[string]any, key string) map[string]any {
	if nested, ok := event[key].(map[string]any); ok {
		return nested
	}
	return event
}

func getString(m map[string]any, key string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return ""
}

func readStopReason(m map[string]any) string {
	if stop := getString(m, "stop_reason"); stop != "" {
		return stop
	}
	return getString(m, "stopReason")
}

func toInt(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	default:
		return 0, false
	}
}

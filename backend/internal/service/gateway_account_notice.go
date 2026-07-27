package service

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
)

const gatewayAccountNoticePrefix = "[Corgi AI Gateway]"

const (
	gatewayAccountNoticeColorGateway = "\x1b[93m"
	gatewayAccountNoticeColorText    = "\x1b[97m"
	gatewayAccountNoticeColorValue   = "\x1b[96m"
	gatewayAccountNoticeColorReset   = "\x1b[0m"
)

const gatewayAccountNoticeContextKey = "gateway_account_notice_state"

// GatewayAccountNoticeMode identifies the client wire format being written.
// The writer only edits protocol-defined text fields for the selected mode.
type GatewayAccountNoticeMode string

const (
	GatewayAccountNoticeOpenAIResponses GatewayAccountNoticeMode = "openai_responses"
	GatewayAccountNoticeOpenAIChat      GatewayAccountNoticeMode = "openai_chat"
	GatewayAccountNoticeAnthropic       GatewayAccountNoticeMode = "anthropic"
	GatewayAccountNoticeGemini          GatewayAccountNoticeMode = "gemini"
)

type gatewayAccountNoticeState struct {
	text     string
	mode     GatewayAccountNoticeMode
	injected bool
}

// GatewayAccountNoticeTransformer applies a pending notice to response JSON
// frames that bypass gin's HTTP ResponseWriter, such as Responses WebSocket.
type GatewayAccountNoticeTransformer struct {
	state *gatewayAccountNoticeState
}

// NewGatewayAccountNoticeTransformer creates a protocol-aware transformer
// when a session is new or has switched accounts. A nil result is a no-op.
func NewGatewayAccountNoticeTransformer(mode GatewayAccountNoticeMode, previousAccountID int64, account *Account) *GatewayAccountNoticeTransformer {
	if account == nil || (previousAccountID > 0 && previousAccountID == account.ID) {
		return nil
	}
	text := GatewayAccountNoticeText(account)
	if text == "" {
		return nil
	}
	return &GatewayAccountNoticeTransformer{state: &gatewayAccountNoticeState{text: text, mode: mode}}
}

// TransformJSON prepends the notice to the first protocol-defined assistant
// text field. Non-JSON, error, tool-only, and terminal frames pass through.
func (t *GatewayAccountNoticeTransformer) TransformJSON(data []byte) []byte {
	if t == nil || t.state == nil || t.state.injected {
		return data
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return data
	}
	if !prependGatewayNoticeToPayload(payload, t.state) {
		return data
	}
	transformed, err := json.Marshal(payload)
	if err != nil {
		return data
	}
	return transformed
}

// GatewayAccountNoticeText returns the stable user-facing account banner.
func GatewayAccountNoticeText(account *Account) string {
	if account == nil {
		return ""
	}
	name := strings.Join(strings.FieldsFunc(strings.TrimSpace(account.Name), unicode.IsControl), " ")
	name = strings.Join(strings.Fields(name), " ")
	rate := strconv.FormatFloat(account.BillingRateMultiplier(), 'f', -1, 64)
	return gatewayAccountNoticeColorGateway + gatewayAccountNoticePrefix +
		gatewayAccountNoticeColorText + " 当前正在使用 " +
		gatewayAccountNoticeColorValue + "#" + strconv.FormatInt(account.ID, 10) + " " + name +
		gatewayAccountNoticeColorText + " 账号，计费倍率 " +
		gatewayAccountNoticeColorValue + rate + gatewayAccountNoticeColorReset + "\n"
}

// SetGatewayAccountNotice schedules a banner for the first real assistant text
// emitted by this response. It intentionally does nothing when routing remains
// on the bound account.
func SetGatewayAccountNotice(c *gin.Context, mode GatewayAccountNoticeMode, previousAccountID int64, account *Account) {
	if c == nil || account == nil || (previousAccountID > 0 && previousAccountID == account.ID) {
		return
	}
	text := GatewayAccountNoticeText(account)
	if text == "" {
		return
	}

	state, _ := c.Get(gatewayAccountNoticeContextKey)
	if notice, ok := state.(*gatewayAccountNoticeState); ok && notice != nil {
		notice.text = text
		notice.mode = mode
		notice.injected = false
		return
	}

	notice := &gatewayAccountNoticeState{text: text, mode: mode}
	c.Set(gatewayAccountNoticeContextKey, notice)
	c.Writer = &gatewayAccountNoticeWriter{ResponseWriter: c.Writer, state: notice}
}

// StripGatewayAccountNoticeFromBody removes gateway-generated notices from
// assistant/model history before the payload reaches an upstream provider.
// User, system, developer, and tool content are deliberately left untouched.
func StripGatewayAccountNoticeFromBody(body []byte) ([]byte, bool) {
	if !bytes.Contains(body, []byte(gatewayAccountNoticePrefix)) {
		return body, false
	}
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return body, false
	}
	changed := false
	for _, field := range []string{"messages", "input", "contents"} {
		value, exists := request[field]
		if !exists {
			continue
		}
		cleaned, fieldChanged := stripGatewayNoticeEntries(value)
		if fieldChanged {
			request[field] = cleaned
			changed = true
		}
	}
	if !changed {
		return body, false
	}
	cleaned, err := json.Marshal(request)
	if err != nil {
		return body, false
	}
	return cleaned, true
}

func stripGatewayNoticeEntries(value any) (any, bool) {
	entries, ok := value.([]any)
	if !ok {
		return value, false
	}
	changed := false
	cleaned := make([]any, 0, len(entries))
	for _, entry := range entries {
		message, ok := entry.(map[string]any)
		if !ok || !gatewayNoticeAssistantRole(message) {
			cleaned = append(cleaned, entry)
			continue
		}
		keep, messageChanged := stripGatewayNoticeMessage(message)
		if !messageChanged {
			cleaned = append(cleaned, entry)
			continue
		}
		changed = true
		if keep {
			cleaned = append(cleaned, message)
		}
	}
	if !changed {
		return value, false
	}
	return cleaned, true
}

func gatewayNoticeAssistantRole(message map[string]any) bool {
	role, _ := message["role"].(string)
	return strings.EqualFold(strings.TrimSpace(role), "assistant") || strings.EqualFold(strings.TrimSpace(role), "model")
}

func stripGatewayNoticeMessage(message map[string]any) (keep bool, changed bool) {
	for _, field := range []string{"content", "parts"} {
		value, exists := message[field]
		if !exists {
			continue
		}
		cleaned, fieldChanged, hasContent := stripGatewayNoticeContent(value)
		if !fieldChanged {
			continue
		}
		changed = true
		if hasContent {
			message[field] = cleaned
		} else {
			delete(message, field)
		}
	}
	if !changed {
		return true, false
	}
	if gatewayNoticeMessageHasNonTextContent(message) {
		return true, true
	}
	return false, true
}

func stripGatewayNoticeContent(value any) (cleaned any, changed bool, hasContent bool) {
	switch content := value.(type) {
	case string:
		text, stripped := stripGatewayNoticePrefix(content)
		if !stripped {
			return value, false, strings.TrimSpace(content) != ""
		}
		return text, true, strings.TrimSpace(text) != ""
	case []any:
		result := make([]any, 0, len(content))
		for _, block := range content {
			blockMap, ok := block.(map[string]any)
			if !ok {
				result = append(result, block)
				continue
			}
			blockType, _ := blockMap["type"].(string)
			text, isText := blockMap["text"].(string)
			if !isText || !gatewayNoticeTextBlockType(blockType) {
				result = append(result, block)
				continue
			}
			stripped, didStrip := stripGatewayNoticePrefix(text)
			if !didStrip {
				result = append(result, block)
				continue
			}
			changed = true
			if strings.TrimSpace(stripped) == "" {
				continue
			}
			blockMap["text"] = stripped
			result = append(result, blockMap)
		}
		if !changed {
			return value, false, len(content) > 0
		}
		return result, true, len(result) > 0
	default:
		return value, false, value != nil
	}
}

func gatewayNoticeTextBlockType(blockType string) bool {
	switch strings.TrimSpace(blockType) {
	case "", "text", "output_text", "input_text":
		return true
	default:
		return false
	}
}

func gatewayNoticeMessageHasNonTextContent(message map[string]any) bool {
	for _, key := range []string{"tool_calls", "tool_call_id", "tool_use_id", "function_call", "call_id"} {
		if value, ok := message[key]; ok && value != nil {
			return true
		}
	}
	for _, field := range []string{"content", "parts"} {
		if value, ok := message[field]; ok {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return true
				}
			case []any:
				if len(typed) > 0 {
					return true
				}
			}
		}
	}
	return false
}

func stripGatewayNoticePrefix(text string) (string, bool) {
	if strings.HasPrefix(text, gatewayAccountNoticeColorGateway+gatewayAccountNoticePrefix) {
		if newline := strings.IndexByte(text, '\n'); newline >= 0 {
			return text[newline+1:], true
		}
		return "", true
	}
	if !strings.HasPrefix(text, gatewayAccountNoticePrefix) {
		return text, false
	}
	if newline := strings.IndexByte(text, '\n'); newline >= 0 {
		return text[newline+1:], true
	}
	return "", true
}

type gatewayAccountNoticeWriter struct {
	gin.ResponseWriter
	state   *gatewayAccountNoticeState
	pending []byte
}

func (w *gatewayAccountNoticeWriter) Write(data []byte) (int, error) {
	if w == nil || w.ResponseWriter == nil || w.state == nil || w.state.injected || len(data) == 0 {
		return w.ResponseWriter.Write(data)
	}
	if strings.Contains(strings.ToLower(w.Header().Get("Content-Type")), "text/event-stream") {
		w.pending = append(w.pending, data...)
		if err := w.writeCompleteSSEFrames(); err != nil {
			return 0, err
		}
		return len(data), nil
	}
	return w.ResponseWriter.Write(w.transformJSON(data))
}

func (w *gatewayAccountNoticeWriter) WriteString(data string) (int, error) {
	_, err := w.Write([]byte(data))
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func (w *gatewayAccountNoticeWriter) Flush() {
	if w != nil && w.ResponseWriter != nil {
		_ = w.writeCompleteSSEFrames()
		w.ResponseWriter.Flush()
	}
}

func (w *gatewayAccountNoticeWriter) writeCompleteSSEFrames() error {
	for {
		end, separatorLen := nextGatewayNoticeSSEFrame(w.pending)
		if end < 0 {
			return nil
		}
		frame := w.transformSSEFrame(w.pending[:end])
		if _, err := w.ResponseWriter.Write(frame); err != nil {
			return err
		}
		if _, err := w.ResponseWriter.Write(w.pending[end : end+separatorLen]); err != nil {
			return err
		}
		w.pending = w.pending[end+separatorLen:]
	}
}

func nextGatewayNoticeSSEFrame(data []byte) (int, int) {
	if index := bytes.Index(data, []byte("\r\n\r\n")); index >= 0 {
		return index, 4
	}
	if index := bytes.Index(data, []byte("\n\n")); index >= 0 {
		return index, 2
	}
	return -1, 0
}

func (w *gatewayAccountNoticeWriter) transformSSEFrame(frame []byte) []byte {
	lines := strings.Split(string(frame), "\n")
	changed := false
	for index, line := range lines {
		trimmed := strings.TrimSuffix(line, "\r")
		if !strings.HasPrefix(trimmed, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		transformed := w.transformJSON([]byte(payload))
		if string(transformed) == payload {
			continue
		}
		ending := ""
		if strings.HasSuffix(line, "\r") {
			ending = "\r"
		}
		lines[index] = "data: " + string(transformed) + ending
		changed = true
	}
	if !changed {
		return frame
	}
	return []byte(strings.Join(lines, "\n"))
}

func (w *gatewayAccountNoticeWriter) transformJSON(data []byte) []byte {
	if w == nil || w.state == nil || w.state.injected {
		return data
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return data
	}
	if !prependGatewayNoticeToPayload(payload, w.state) {
		return data
	}
	transformed, err := json.Marshal(payload)
	if err != nil {
		return data
	}
	return transformed
}

func prependGatewayNoticeToPayload(payload any, state *gatewayAccountNoticeState) bool {
	root, ok := payload.(map[string]any)
	if !ok || state == nil || state.injected {
		return false
	}
	switch state.mode {
	case GatewayAccountNoticeOpenAIChat:
		return prependGatewayNoticeToOpenAIChat(root, state)
	case GatewayAccountNoticeAnthropic:
		return prependGatewayNoticeToAnthropic(root, state)
	case GatewayAccountNoticeGemini:
		return prependGatewayNoticeToGemini(root, state)
	case GatewayAccountNoticeOpenAIResponses:
		return prependGatewayNoticeToResponses(root, state)
	default:
		return false
	}
}

func prependGatewayNoticeText(state *gatewayAccountNoticeState, text string) (string, bool) {
	if state == nil || state.injected || text == "" || state.text == "" {
		return text, false
	}
	state.injected = true
	return state.text + text, true
}

func prependGatewayNoticeToOpenAIChat(root map[string]any, state *gatewayAccountNoticeState) bool {
	choices, ok := root["choices"].([]any)
	if !ok {
		return false
	}
	for _, choice := range choices {
		choiceMap, ok := choice.(map[string]any)
		if !ok {
			continue
		}
		for _, field := range []string{"delta", "message"} {
			content, ok := choiceMap[field].(map[string]any)
			if !ok {
				continue
			}
			text, ok := content["content"].(string)
			if !ok {
				continue
			}
			if prefixed, changed := prependGatewayNoticeText(state, text); changed {
				content["content"] = prefixed
				return true
			}
		}
	}
	return false
}

func prependGatewayNoticeToAnthropic(root map[string]any, state *gatewayAccountNoticeState) bool {
	if delta, ok := root["delta"].(map[string]any); ok {
		if text, ok := delta["text"].(string); ok {
			if prefixed, changed := prependGatewayNoticeText(state, text); changed {
				delta["text"] = prefixed
				return true
			}
		}
	}
	content, ok := root["content"].([]any)
	if !ok {
		return false
	}
	for _, block := range content {
		blockMap, ok := block.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := blockMap["text"].(string); ok {
			if prefixed, changed := prependGatewayNoticeText(state, text); changed {
				blockMap["text"] = prefixed
				return true
			}
		}
	}
	return false
}

func prependGatewayNoticeToGemini(root map[string]any, state *gatewayAccountNoticeState) bool {
	candidates, ok := root["candidates"].([]any)
	if !ok {
		return false
	}
	for _, candidate := range candidates {
		candidateMap, ok := candidate.(map[string]any)
		if !ok {
			continue
		}
		content, ok := candidateMap["content"].(map[string]any)
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}
		for _, part := range parts {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := partMap["text"].(string); ok {
				if prefixed, changed := prependGatewayNoticeText(state, text); changed {
					partMap["text"] = prefixed
					return true
				}
			}
		}
	}
	return false
}

func prependGatewayNoticeToResponses(root map[string]any, state *gatewayAccountNoticeState) bool {
	if kind, _ := root["type"].(string); kind == "response.output_text.delta" {
		if delta, ok := root["delta"].(string); ok {
			if prefixed, changed := prependGatewayNoticeText(state, delta); changed {
				root["delta"] = prefixed
				return true
			}
		}
	}
	for _, field := range []string{"response", "output"} {
		value, exists := root[field]
		if !exists {
			continue
		}
		if prependGatewayNoticeToResponseOutput(value, state) {
			return true
		}
	}
	return false
}

func prependGatewayNoticeToResponseOutput(value any, state *gatewayAccountNoticeState) bool {
	if response, ok := value.(map[string]any); ok {
		if output, exists := response["output"]; exists {
			return prependGatewayNoticeToResponseOutput(output, state)
		}
	}
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok || !gatewayNoticeAssistantRole(itemMap) {
			continue
		}
		content, ok := itemMap["content"].([]any)
		if !ok {
			continue
		}
		for _, block := range content {
			blockMap, ok := block.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := blockMap["text"].(string); ok {
				if prefixed, changed := prependGatewayNoticeText(state, text); changed {
					blockMap["text"] = prefixed
					return true
				}
			}
		}
	}
	return false
}

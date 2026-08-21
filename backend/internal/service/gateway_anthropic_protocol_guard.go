package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const rawAnthropicToolCallMarker = `<invoke name="`

var rawAnthropicToolCallNamePattern = regexp.MustCompile(`<invoke\s+name="([^"]+)">`)

type anthropicToolCallProtocolError struct {
	reason string
}

func (e *anthropicToolCallProtocolError) Error() string {
	return "upstream returned malformed tool-call protocol: " + e.reason
}

func rawAnthropicToolCallNames(text string) ([]string, bool) {
	matches := rawAnthropicToolCallNamePattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 || strings.Count(text, "</invoke>") < len(matches) {
		return nil, false
	}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 || strings.TrimSpace(match[1]) == "" {
			return nil, false
		}
		names = append(names, strings.TrimSpace(match[1]))
	}
	return names, true
}

func splitRawAnthropicToolCallText(text string) (visible string, names []string, found, complete bool) {
	markerIndex := strings.Index(text, rawAnthropicToolCallMarker)
	if markerIndex < 0 {
		return text, nil, false, true
	}
	rawNames, isComplete := rawAnthropicToolCallNames(text[markerIndex:])
	if !isComplete {
		return text[:markerIndex], nil, true, false
	}
	return text[:markerIndex], rawNames, true, true
}

func sameToolNameMultiset(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]string(nil), left...)
	rightCopy := append([]string(nil), right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	for index := range leftCopy {
		if leftCopy[index] != rightCopy[index] {
			return false
		}
	}
	return true
}

func validateAnthropicToolCallProtocol(stopReason string, rawToolNames, structuredToolNames []string) error {
	if len(rawToolNames) > 0 {
		if len(structuredToolNames) == 0 {
			return &anthropicToolCallProtocolError{reason: "raw tool-call markup had no structured tool_use block"}
		}
		if !sameToolNameMultiset(rawToolNames, structuredToolNames) {
			return &anthropicToolCallProtocolError{reason: "raw and structured tool names disagree"}
		}
	}
	if stopReason == "tool_use" && len(structuredToolNames) == 0 {
		return &anthropicToolCallProtocolError{reason: "stop_reason=tool_use had no structured tool_use block"}
	}
	return nil
}

func normalizeAnthropicPassthroughResponseBody(body []byte) ([]byte, error) {
	var envelope struct {
		Content    []json.RawMessage `json:"content"`
		StopReason string            `json:"stop_reason"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}

	normalizedContent := make([]json.RawMessage, 0, len(envelope.Content))
	rawToolNames := make([]string, 0)
	structuredToolNames := make([]string, 0)
	for _, rawBlock := range envelope.Content {
		var block struct {
			Type string `json:"type"`
			Text string `json:"text"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(rawBlock, &block); err != nil {
			return nil, err
		}
		switch block.Type {
		case "text":
			visible, names, found, complete := splitRawAnthropicToolCallText(block.Text)
			if found {
				if !complete {
					return nil, &anthropicToolCallProtocolError{reason: "raw tool-call markup was incomplete"}
				}
				rawToolNames = append(rawToolNames, names...)
				if visible == "" {
					continue
				}
				normalizedBlock, err := sjson.SetBytes(rawBlock, "text", visible)
				if err != nil {
					return nil, err
				}
				normalizedContent = append(normalizedContent, normalizedBlock)
				continue
			}
		case "tool_use":
			if strings.TrimSpace(block.Name) != "" {
				structuredToolNames = append(structuredToolNames, strings.TrimSpace(block.Name))
			}
		}
		normalizedContent = append(normalizedContent, rawBlock)
	}

	if err := validateAnthropicToolCallProtocol(envelope.StopReason, rawToolNames, structuredToolNames); err != nil {
		return nil, err
	}
	if len(rawToolNames) == 0 {
		return body, nil
	}
	contentJSON, err := json.Marshal(normalizedContent)
	if err != nil {
		return nil, err
	}
	return sjson.SetRawBytes(body, "content", contentJSON)
}

func anthropicProtocolFailoverError(resp *http.Response, account *Account, protocolErr error) error {
	retryableOnSameAccount := false
	if account != nil {
		retryableOnSameAccount = account.IsPoolMode() && account.IsPoolModeRetryableStatus(http.StatusBadGateway)
	}
	responseBody, _ := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    "protocol_error",
			"message": "Upstream returned malformed tool-call content",
		},
	})
	return &UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		ResponseBody:           responseBody,
		ResponseHeaders:        resp.Header,
		RetryableOnSameAccount: retryableOnSameAccount,
	}
}

type anthropicPassthroughTextBlockState struct {
	pending      string
	rawCandidate strings.Builder
	readingRaw   bool
}

type anthropicPassthroughSSEGuard struct {
	eventLines          []string
	textBlocks          map[int]*anthropicPassthroughTextBlockState
	rawToolNames        []string
	structuredToolNames []string
}

func newAnthropicPassthroughSSEGuard() *anthropicPassthroughSSEGuard {
	return &anthropicPassthroughSSEGuard{
		textBlocks: make(map[int]*anthropicPassthroughTextBlockState),
	}
}

func (g *anthropicPassthroughSSEGuard) PushLine(line string) ([]string, error) {
	if line != "" {
		g.eventLines = append(g.eventLines, line)
		if g.canEagerEmitEvent(line) {
			lines := append([]string(nil), g.eventLines...)
			g.eventLines = nil
			return lines, nil
		}
		return nil, nil
	}
	if len(g.eventLines) == 0 {
		return []string{""}, nil
	}
	lines, err := g.transformEvent(g.eventLines)
	g.eventLines = nil
	return append(lines, ""), err
}

func (g *anthropicPassthroughSSEGuard) canEagerEmitEvent(line string) bool {
	data, ok := extractAnthropicSSEDataLine(line)
	if !ok || !gjson.Valid(data) {
		return false
	}
	switch gjson.Get(data, "type").String() {
	case "message_start", "ping":
		return true
	default:
		return false
	}
}

func (g *anthropicPassthroughSSEGuard) Flush() ([]string, error) {
	if len(g.eventLines) == 0 {
		return nil, nil
	}
	lines, err := g.transformEvent(g.eventLines)
	g.eventLines = nil
	return lines, err
}

func (g *anthropicPassthroughSSEGuard) transformEvent(lines []string) ([]string, error) {
	dataLineIndex := -1
	data := ""
	for index, line := range lines {
		if extracted, ok := extractAnthropicSSEDataLine(line); ok {
			dataLineIndex = index
			data = extracted
			break
		}
	}
	if dataLineIndex < 0 || data == "" || data == "[DONE]" || !gjson.Valid(data) {
		return append([]string(nil), lines...), nil
	}

	eventType := gjson.Get(data, "type").String()
	switch eventType {
	case "content_block_start":
		index := int(gjson.Get(data, "index").Int())
		blockType := gjson.Get(data, "content_block.type").String()
		if blockType == "text" {
			state := &anthropicPassthroughTextBlockState{}
			g.textBlocks[index] = state
			initialText := gjson.Get(data, "content_block.text").String()
			if initialText != "" {
				visible := g.consumeTextDelta(state, initialText)
				updated, err := sjson.Set(data, "content_block.text", visible)
				if err != nil {
					return nil, err
				}
				updatedLines := append([]string(nil), lines...)
				updatedLines[dataLineIndex] = "data: " + updated
				return updatedLines, nil
			}
		}
		if blockType == "tool_use" {
			name := strings.TrimSpace(gjson.Get(data, "content_block.name").String())
			if name != "" {
				g.structuredToolNames = append(g.structuredToolNames, name)
			}
		}
	case "content_block_delta":
		if gjson.Get(data, "delta.type").String() != "text_delta" {
			return append([]string(nil), lines...), nil
		}
		index := int(gjson.Get(data, "index").Int())
		state := g.textBlocks[index]
		if state == nil {
			state = &anthropicPassthroughTextBlockState{}
			g.textBlocks[index] = state
		}
		visible := g.consumeTextDelta(state, gjson.Get(data, "delta.text").String())
		updated, err := sjson.Set(data, "delta.text", visible)
		if err != nil {
			return nil, err
		}
		updatedLines := append([]string(nil), lines...)
		updatedLines[dataLineIndex] = "data: " + updated
		return updatedLines, nil
	case "content_block_stop":
		index := int(gjson.Get(data, "index").Int())
		state := g.textBlocks[index]
		if state == nil {
			return append([]string(nil), lines...), nil
		}
		delete(g.textBlocks, index)
		flushText := state.pending
		if state.readingRaw {
			candidate := state.rawCandidate.String()
			if names, complete := rawAnthropicToolCallNames(candidate); complete {
				g.rawToolNames = append(g.rawToolNames, names...)
				flushText = ""
			} else {
				return anthropicProtocolSSEErrorLines(), &anthropicToolCallProtocolError{reason: "raw tool-call markup was incomplete"}
			}
		}
		if flushText == "" {
			return append([]string(nil), lines...), nil
		}
		return append(g.syntheticTextDelta(index, flushText), append([]string(nil), lines...)...), nil
	case "message_delta":
		stopReason := gjson.Get(data, "delta.stop_reason").String()
		if err := validateAnthropicToolCallProtocol(stopReason, g.rawToolNames, g.structuredToolNames); err != nil {
			return anthropicProtocolSSEErrorLines(), err
		}
	case "message_stop":
		if err := validateAnthropicToolCallProtocol("", g.rawToolNames, g.structuredToolNames); err != nil {
			return anthropicProtocolSSEErrorLines(), err
		}
	}
	return append([]string(nil), lines...), nil
}

func (g *anthropicPassthroughSSEGuard) consumeTextDelta(state *anthropicPassthroughTextBlockState, text string) string {
	if state.readingRaw {
		state.rawCandidate.WriteString(text)
		return ""
	}
	combined := state.pending + text
	markerIndex := strings.Index(combined, rawAnthropicToolCallMarker)
	if markerIndex >= 0 {
		state.pending = ""
		state.readingRaw = true
		state.rawCandidate.WriteString(combined[markerIndex:])
		return combined[:markerIndex]
	}
	pendingLength := longestRawToolCallMarkerPrefixSuffix(combined)
	if pendingLength == 0 {
		state.pending = ""
		return combined
	}
	state.pending = combined[len(combined)-pendingLength:]
	return combined[:len(combined)-pendingLength]
}

func longestRawToolCallMarkerPrefixSuffix(text string) int {
	maxLength := len(rawAnthropicToolCallMarker) - 1
	if len(text) < maxLength {
		maxLength = len(text)
	}
	for length := maxLength; length > 0; length-- {
		if strings.HasSuffix(text, rawAnthropicToolCallMarker[:length]) {
			return length
		}
	}
	return 0
}

func (g *anthropicPassthroughSSEGuard) syntheticTextDelta(index int, text string) []string {
	payload, _ := json.Marshal(map[string]any{
		"type":  "content_block_delta",
		"index": index,
		"delta": map[string]string{
			"type": "text_delta",
			"text": text,
		},
	})
	return []string{"event: content_block_delta", "data: " + string(payload), ""}
}

func anthropicProtocolSSEErrorLines() []string {
	payload, _ := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    "protocol_error",
			"message": "Upstream returned malformed tool-call content",
		},
	})
	return []string{"event: error", "data: " + string(payload)}
}

func writeAnthropicGuardedLines(w http.ResponseWriter, c interface {
	Get(string) (any, bool)
}, flusher http.Flusher, lines []string) (clientDisconnected bool, completedEvent bool) {
	for _, line := range lines {
		restored := string(reverseToolNamesIfPresent(c, []byte(line)))
		if _, err := fmt.Fprintln(w, restored); err != nil {
			return true, completedEvent
		}
		if line == "" {
			flusher.Flush()
			completedEvent = true
		}
	}
	return false, completedEvent
}

package service

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	rawAnthropicToolCallMarker             = `<invoke`
	maxAnthropicToolCallCandidateBytes     = 64 * 1024
	maxAnthropicToolCallDeferredEventBytes = 128 * 1024
)

type anthropicToolCallProtocolError struct {
	reason string
}

func (e *anthropicToolCallProtocolError) Error() string {
	return "upstream returned malformed tool-call protocol: " + e.reason
}

func rawAnthropicToolCallNames(text string) ([]string, bool) {
	if len(text) == 0 || len(text) > maxAnthropicToolCallCandidateBytes {
		return nil, false
	}
	decoder := xml.NewDecoder(strings.NewReader("<raw-tool-calls>" + text + "</raw-tool-calls>"))
	names := make([]string, 0, 1)
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return names, len(names) > 0
		}
		if err != nil {
			return nil, false
		}
		switch value := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				if value.Name.Local != "raw-tool-calls" {
					return nil, false
				}
				continue
			}
			if depth != 2 {
				continue
			}
			if value.Name.Local != "invoke" {
				return nil, false
			}
			name := ""
			for _, attribute := range value.Attr {
				if attribute.Name.Local == "name" {
					name = strings.TrimSpace(attribute.Value)
					break
				}
			}
			if name == "" {
				return nil, false
			}
			names = append(names, name)
		case xml.EndElement:
			depth--
			if depth < 0 {
				return nil, false
			}
		case xml.CharData:
			if depth == 1 && strings.TrimSpace(string(value)) != "" {
				return nil, false
			}
		case xml.Comment:
			if depth == 1 {
				return nil, false
			}
		case xml.Directive, xml.ProcInst:
			return nil, false
		}
	}
}

func splitRawAnthropicToolCallText(text string) (visible string, names []string, found bool) {
	markerIndex := strings.Index(text, rawAnthropicToolCallMarker)
	if markerIndex < 0 {
		return text, nil, false
	}
	rawNames, isComplete := rawAnthropicToolCallNames(text[markerIndex:])
	if !isComplete {
		return text, nil, false
	}
	return text[:markerIndex], rawNames, true
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

func validateAnthropicToolCallProtocol(stopReason string, structuredToolNames []string) error {
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

	type rawCandidate struct {
		blockIndex int
		visible    string
	}
	candidates := make([]rawCandidate, 0, 1)
	rawToolNames := make([]string, 0)
	structuredToolNames := make([]string, 0)
	for blockIndex, rawBlock := range envelope.Content {
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
			visible, names, found := splitRawAnthropicToolCallText(block.Text)
			if found {
				rawToolNames = append(rawToolNames, names...)
				candidates = append(candidates, rawCandidate{blockIndex: blockIndex, visible: visible})
			}
		case "tool_use":
			if strings.TrimSpace(block.Name) != "" {
				structuredToolNames = append(structuredToolNames, strings.TrimSpace(block.Name))
			}
		}
	}

	if err := validateAnthropicToolCallProtocol(envelope.StopReason, structuredToolNames); err != nil {
		return nil, err
	}
	if len(candidates) == 0 || envelope.StopReason != "tool_use" || !sameToolNameMultiset(rawToolNames, structuredToolNames) {
		return body, nil
	}
	candidateByBlock := make(map[int]rawCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateByBlock[candidate.blockIndex] = candidate
	}
	normalizedContent := make([]json.RawMessage, 0, len(envelope.Content))
	for blockIndex, rawBlock := range envelope.Content {
		candidate, ok := candidateByBlock[blockIndex]
		if !ok {
			normalizedContent = append(normalizedContent, rawBlock)
			continue
		}
		if candidate.visible == "" {
			continue
		}
		normalizedBlock, err := sjson.SetBytes(rawBlock, "text", candidate.visible)
		if err != nil {
			return nil, err
		}
		normalizedContent = append(normalizedContent, normalizedBlock)
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
	pending           string
	rawCandidate      strings.Builder
	readingRaw        bool
	detectionDisabled bool
}

type anthropicPassthroughSSEGuard struct {
	eventLines          []string
	textBlocks          map[int]*anthropicPassthroughTextBlockState
	structuredToolNames []string
	deferredLines       []string
	deferredBytes       int
	deferredCandidate   string
	deferredBlockIndex  int
	deferring           bool
	passthrough         bool
}

func newAnthropicPassthroughSSEGuard() *anthropicPassthroughSSEGuard {
	return &anthropicPassthroughSSEGuard{
		textBlocks: make(map[int]*anthropicPassthroughTextBlockState),
	}
}

func (g *anthropicPassthroughSSEGuard) PushLine(line string) ([]string, error) {
	if g.passthrough {
		return []string{line}, nil
	}
	if line != "" {
		g.eventLines = append(g.eventLines, line)
		if !g.deferring && g.canEagerEmitEvent(line) {
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
	if err != nil {
		return append(lines, ""), err
	}
	if g.deferring {
		deferredEventLines := append(lines, "")
		if !g.deferLines(deferredEventLines) {
			resolved := g.resolveDeferred(false)
			resolved = append(resolved, deferredEventLines...)
			g.passthrough = true
			return resolved, nil
		}
		return nil, nil
	}
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
	if g.passthrough {
		lines := append([]string(nil), g.eventLines...)
		g.eventLines = nil
		return lines, nil
	}
	output := make([]string, 0)
	if len(g.eventLines) > 0 {
		lines, err := g.transformEvent(g.eventLines)
		g.eventLines = nil
		if err != nil {
			return lines, err
		}
		if g.deferring {
			if !g.deferLines(lines) {
				output = append(output, g.resolveDeferred(false)...)
				output = append(output, lines...)
			}
		} else {
			output = append(output, lines...)
		}
	}
	if g.deferring {
		output = append(output, g.resolveDeferred(false)...)
	}
	for index, state := range g.textBlocks {
		text := state.pending
		if state.readingRaw {
			text += state.rawCandidate.String()
		}
		if text != "" {
			output = append(output, g.syntheticTextDelta(index, text)...)
		}
	}
	return output, nil
}

func (g *anthropicPassthroughSSEGuard) deferLines(lines []string) bool {
	additionalBytes := 0
	for _, line := range lines {
		additionalBytes += len(line) + 1
	}
	if additionalBytes > maxAnthropicToolCallDeferredEventBytes-g.deferredBytes {
		return false
	}
	g.deferredLines = append(g.deferredLines, lines...)
	g.deferredBytes += additionalBytes
	return true
}

func (g *anthropicPassthroughSSEGuard) beginDeferredCandidate(index int, candidate string) {
	g.deferring = true
	g.deferredBlockIndex = index
	g.deferredCandidate = candidate
	g.deferredBytes = len(candidate)
}

func (g *anthropicPassthroughSSEGuard) resolveDeferred(stripCandidate bool) []string {
	lines := make([]string, 0, len(g.deferredLines)+3)
	if !stripCandidate && g.deferredCandidate != "" {
		lines = append(lines, g.syntheticTextDelta(g.deferredBlockIndex, g.deferredCandidate)...)
	}
	lines = append(lines, g.deferredLines...)
	g.deferredLines = nil
	g.deferredBytes = 0
	g.deferredCandidate = ""
	g.deferredBlockIndex = 0
	g.deferring = false
	return lines
}

func (g *anthropicPassthroughSSEGuard) resolveDeferredForStopReason(stopReason string) []string {
	rawToolNames, complete := rawAnthropicToolCallNames(g.deferredCandidate)
	stripCandidate := complete && stopReason == "tool_use" && sameToolNameMultiset(rawToolNames, g.structuredToolNames)
	return g.resolveDeferred(stripCandidate)
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
			g.beginDeferredCandidate(index, state.rawCandidate.String())
			return append([]string(nil), lines...), nil
		}
		if flushText == "" {
			return append([]string(nil), lines...), nil
		}
		return append(g.syntheticTextDelta(index, flushText), append([]string(nil), lines...)...), nil
	case "message_delta":
		stopReason := gjson.Get(data, "delta.stop_reason").String()
		if err := validateAnthropicToolCallProtocol(stopReason, g.structuredToolNames); err != nil {
			g.resolveDeferred(true)
			return anthropicProtocolSSEErrorLines(), err
		}
		if g.deferring {
			resolved := g.resolveDeferredForStopReason(stopReason)
			return append(resolved, lines...), nil
		}
	case "message_stop":
		if g.deferring {
			resolved := g.resolveDeferred(false)
			return append(resolved, lines...), nil
		}
	}
	return append([]string(nil), lines...), nil
}

func (g *anthropicPassthroughSSEGuard) consumeTextDelta(state *anthropicPassthroughTextBlockState, text string) string {
	if state.detectionDisabled {
		return text
	}
	if state.readingRaw {
		if state.rawCandidate.Len()+len(text) > maxAnthropicToolCallCandidateBytes {
			restored := state.rawCandidate.String() + text
			state.rawCandidate.Reset()
			state.readingRaw = false
			state.detectionDisabled = true
			return restored
		}
		_, _ = state.rawCandidate.WriteString(text)
		return ""
	}
	combined := state.pending + text
	markerIndex := strings.Index(combined, rawAnthropicToolCallMarker)
	if markerIndex >= 0 {
		candidate := combined[markerIndex:]
		state.pending = ""
		if len(candidate) > maxAnthropicToolCallCandidateBytes {
			state.detectionDisabled = true
			return combined
		}
		state.readingRaw = true
		_, _ = state.rawCandidate.WriteString(candidate)
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

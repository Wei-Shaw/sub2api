package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync/atomic"

	"github.com/tidwall/gjson"
)

const (
	openAIWSNativeHistorySessionDefault = 64 << 20
	openAIWSNativeHistoryItemsDefault   = 32768
	// Reserve space for the item descriptors as well as their owned JSON bodies.
	openAIWSNativeHistoryItemOverhead = 64
)

// History belongs to one native ingress attempt. Only a complete, current
// request may cross accounts; unknown chains and exhausted budgets fail closed.
type openAIWSNativeHistory struct {
	used         *atomic.Int64
	maxSession   int64
	maxItems     int
	reserved     int64
	items        []json.RawMessage
	exists       bool
	complete     bool
	ready        bool
	lastResponse string
	reason       string
}

func (s *OpenAIGatewayService) newOpenAIWSNativeHistory() *openAIWSNativeHistory {
	h := &openAIWSNativeHistory{
		used:       &s.openaiWSNativeHistoryBytes,
		maxSession: openAIWSNativeHistorySessionDefault,
		maxItems:   openAIWSNativeHistoryItemsDefault,
	}
	if s.cfg != nil {
		c := s.cfg.Gateway.OpenAIWS
		h.maxSession = c.NativeReplayMaxSessionBytes
		h.maxItems = c.NativeReplayMaxItems
	}
	return h
}

func (h *openAIWSNativeHistory) clear(reason string) {
	if h.reserved != 0 {
		h.used.Add(-h.reserved)
	}
	h.reserved = 0
	h.items = nil
	h.exists = false
	h.complete = false
	h.ready = false
	h.lastResponse = ""
	h.reason = reason
}

func (h *openAIWSNativeHistory) close() { h.clear("closed") }

func (h *openAIWSNativeHistory) resize(n int64) bool {
	if n > h.maxSession || h.maxSession <= 0 || h.maxItems <= 0 {
		h.clear("session_budget")
		return false
	}
	// Aggregate usage is accounting only; admission is bounded per connection.
	h.used.Add(n - h.reserved)
	h.reserved = n
	return true
}

func (h *openAIWSNativeHistory) replace(next []json.RawMessage, detach bool) bool {
	if len(next) > h.maxItems {
		h.clear("item_budget")
		return false
	}
	n := int64(len(next)) * openAIWSNativeHistoryItemOverhead
	for _, item := range next {
		n += int64(len(item))
	}
	if !h.resize(n) {
		return false
	}
	// Detach newly received items once, so small slices cannot pin a much larger
	// request buffer. Previously owned bodies are immutable and remain shared.
	if detach {
		for i, item := range next {
			if i < len(h.items) && len(item) > 0 && len(h.items[i]) > 0 && &item[0] == &h.items[i][0] {
				continue
			}
			next[i] = bytes.Clone(item)
		}
	}
	h.items = next
	return true
}

func (h *openAIWSNativeHistory) prepare(items []json.RawMessage, exists bool, previous string) {
	h.ready = false
	if h.maxSession <= 0 || h.maxItems <= 0 {
		h.clear("disabled")
		return
	}
	previous = strings.TrimSpace(previous)
	if previous != "" && (!h.complete || previous != h.lastResponse) {
		reason := h.reason
		if h.complete || reason == "" {
			reason = "incomplete_chain"
		}
		h.clear(reason)
		return
	}
	if previous == "" {
		h.clear("")
	}
	if !exists {
		h.clear("missing_input")
		return
	}
	for _, item := range items {
		value := gjson.Parse(openAIWSPayloadStringView(item))
		if !value.IsObject() {
			h.clear("unsupported_input_shape")
			return
		}
		if value.Get("type").String() == "item_reference" {
			h.clear("unresolved_item_reference")
			return
		}
	}
	next := items
	if previous != "" && !openAIWSRawItemsHasPrefix(items, h.items) {
		// Unlike tool-only transport recovery, full history must not discard
		// assistant calls whose results have not arrived yet.
		next = combineOpenAIWSReplayItems(h.items, items)
	}
	// Never mutate the caller's item descriptors when detaching input bodies.
	next = append([]json.RawMessage(nil), next...)
	if !h.replace(next, true) {
		return
	}
	h.exists = true
	h.complete = true
	h.ready = true
	h.reason = ""
}

// The terminal output array is authoritative for ordering and final item bodies.
// A missing array is not evidence of an empty response: do not migrate it.
func openAIWSNativeCompletedOutput(payload []byte) []json.RawMessage {
	output := gjson.Get(openAIWSPayloadStringView(payload), "response.output")
	if !output.IsArray() || !json.Valid(openAIWSRawMessageFromResult(payload, output)) {
		return nil
	}
	values := output.Array()
	items := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		itemType := value.Get("type").String()
		if !value.IsObject() || itemType == "" || itemType == "item_reference" {
			return nil
		}
		items = append(items, json.RawMessage(value.Raw))
	}
	return items
}

func (h *openAIWSNativeHistory) commit(result *OpenAIForwardResult) {
	if !h.ready {
		return
	}
	if result == nil || result.RequestID == "" || result.wsAccountFailoverReplayInput == nil ||
		(result.UpstreamTerminalEvent != "response.completed" && result.UpstreamTerminalEvent != "response.done") {
		h.clear("incomplete_response")
		return
	}
	// Completed output bodies are independently owned; do not copy them again.
	next := combineOpenAIWSReplayItems(h.items, result.wsAccountFailoverReplayInput)
	if h.replace(next, false) {
		h.lastResponse = strings.Clone(result.RequestID)
	}
	h.ready = false
}

func (h *openAIWSNativeHistory) stripInvalid(digests map[string]struct{}) {
	if !h.complete {
		return
	}
	next, changed := stripOpenAIInvalidEncryptedContentFromReplayItems(h.items, digests)
	if changed > 0 {
		h.replace(next, true)
	}
}

func (h *openAIWSNativeHistory) retryPayload(source []byte, model string) ([]byte, bool, error) {
	if !h.ready || !h.complete {
		return nil, false, nil
	}
	payload, safe, err := buildOpenAIWSCurrentTurnRetryPayload(source, h.items, h.exists, model)
	if err != nil {
		h.reason = "retry_payload_build"
	} else if !safe {
		h.reason = "unpaired_tool_output"
	}
	return payload, safe, err
}

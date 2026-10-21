package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

const (
	maxOpenAIConcatenatedJSONDocuments = 16
	maxOpenAIConcatenatedJSONBytes     = 16 * 1024 * 1024
	maxOpenAIDeferredSSEFieldLines     = 256
	maxOpenAIDeferredSSEFieldBytes     = maxOpenAIConcatenatedJSONBytes
)

var errOpenAIDeferredSSEFieldsLimit = errors.New("OpenAI deferred SSE field group exceeds repair limit")

// splitOpenAIConcatenatedJSONDocuments recognizes the narrow corruption shape
// produced when multiple complete Responses events arrive in one transport
// message. Other malformed payloads are left untouched for normal error paths.
func splitOpenAIConcatenatedJSONDocuments(payload []byte) ([][]byte, bool) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || len(payload) > maxOpenAIConcatenatedJSONBytes || json.Valid(payload) {
		return nil, false
	}

	documents := make([][]byte, 0, 2)
	remaining := payload
	for len(remaining) > 0 {
		if len(documents) > 0 {
			remaining = trimOpenAIEmbeddedSSEPrefix(remaining)
			if len(remaining) == 0 {
				return nil, false
			}
		}
		decoder := json.NewDecoder(bytes.NewReader(remaining))
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, false
		}
		consumed := decoder.InputOffset()
		if consumed <= 0 || consumed > int64(len(remaining)) {
			return nil, false
		}
		raw = bytes.TrimSpace(raw)
		if _, ok := openAITypedEventJSON(raw); !ok {
			return nil, false
		}
		if len(documents) == maxOpenAIConcatenatedJSONDocuments {
			return nil, false
		}
		documents = append(documents, raw)
		remaining = bytes.TrimSpace(remaining[consumed:])
	}
	if len(documents) > 1 {
		return documents, true
	}
	return nil, false
}

func trimOpenAIEmbeddedSSEPrefix(payload []byte) []byte {
	payload = bytes.TrimSpace(payload)
	if bytes.HasPrefix(payload, []byte("data:")) {
		return bytes.TrimLeft(payload[len("data:"):], " \t")
	}
	if !bytes.HasPrefix(payload, []byte("event:")) {
		return payload
	}
	dataIndex := bytes.Index(payload[len("event:"):], []byte("data:"))
	if dataIndex < 0 {
		return payload
	}
	dataIndex += len("event:")
	eventType := strings.TrimSpace(string(payload[len("event:"):dataIndex]))
	if eventType == "" || strings.ContainsAny(eventType, "\r\n{}[]") {
		return payload
	}
	return bytes.TrimLeft(payload[dataIndex+len("data:"):], " \t")
}

func openAITypedEventJSON(payload []byte) (string, bool) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", false
	}
	eventType := strings.TrimSpace(envelope.Type)
	if eventType == "" || strings.ContainsAny(eventType, "\r\n") {
		return "", false
	}
	return eventType, true
}

func splitOpenAITypedJSONAndDoneMarker(payload []byte) ([]byte, bool) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || len(payload) > maxOpenAIConcatenatedJSONBytes || json.Valid(payload) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, false
	}
	raw = bytes.TrimSpace(raw)
	if _, ok := openAITypedEventJSON(raw); !ok {
		return nil, false
	}
	consumed := decoder.InputOffset()
	if consumed <= 0 || consumed > int64(len(payload)) {
		return nil, false
	}
	tail := bytes.TrimSpace(trimOpenAIEmbeddedSSEPrefix(payload[consumed:]))
	if !bytes.Equal(tail, []byte("[DONE]")) {
		return nil, false
	}
	return raw, true
}

type openAISSEJSONDocumentScanner struct {
	scanner                   *bufio.Scanner
	pending                   []string
	current                   string
	deferredFieldLines        []string
	deferredFieldBytes        int
	frameHasExplicitEvent     bool
	frameHasCompleteTypedData bool
	frameRepairDisabled       bool
	err                       error
}

func newOpenAISSEJSONDocumentScanner(scanner *bufio.Scanner) *openAISSEJSONDocumentScanner {
	return &openAISSEJSONDocumentScanner{scanner: scanner}
}

func (s *openAISSEJSONDocumentScanner) Scan() bool {
	if len(s.pending) > 0 {
		s.current = s.pending[0]
		s.pending = s.pending[1:]
		return true
	}

	for {
		if s.scanner == nil || !s.scanner.Scan() {
			if s.scanner != nil && s.scanner.Err() != nil {
				return false
			}
			if len(s.deferredFieldLines) > 0 {
				return s.emitDeferredFieldLines()
			}
			return false
		}

		line := s.scanner.Text()
		if line == "" {
			s.frameHasExplicitEvent = false
			s.frameHasCompleteTypedData = false
			s.frameRepairDisabled = false
			if len(s.deferredFieldLines) > 0 {
				return s.emitDeferredFieldLines(line)
			}
			s.current = line
			return true
		}
		data, ok := extractOpenAISSEDataLine(line)
		if !ok {
			if s.frameHasCompleteTypedData || len(s.deferredFieldLines) > 0 {
				if !s.deferSSEFieldLine(line) {
					return false
				}
				continue
			}
			if _, isEvent := extractOpenAISSEEventLine(line); isEvent {
				s.frameHasExplicitEvent = true
			}
			s.current = line
			return true
		}
		if strings.TrimSpace(data) == "[DONE]" {
			if !s.frameHasCompleteTypedData {
				if len(s.deferredFieldLines) > 0 {
					s.frameHasCompleteTypedData = false
					s.frameRepairDisabled = true
					return s.emitDeferredFieldLines(line)
				}
				s.current = line
				return true
			}
			expanded := make([]string, 0, 2+len(s.deferredFieldLines))
			expanded = append(expanded, "")
			expanded = append(expanded, s.deferredFieldLines...)
			expanded = append(expanded, line)
			s.clearDeferredFieldLines()
			s.frameHasExplicitEvent = false
			s.frameHasCompleteTypedData = false
			s.frameRepairDisabled = false
			s.current = expanded[0]
			s.pending = append(s.pending, expanded[1:]...)
			return true
		}

		dataBytes := []byte(data)
		if eventType, typed := openAITypedEventJSON(dataBytes); typed {
			if s.frameRepairDisabled {
				s.current = line
				return true
			}
			if s.frameHasCompleteTypedData {
				expanded := make([]string, 0, 3+len(s.deferredFieldLines))
				expanded = append(expanded, "")
				expanded = append(expanded, s.deferredFieldLines...)
				if !s.deferredFieldsHaveEvent() {
					expanded = append(expanded, "event: "+eventType)
				}
				expanded = append(expanded, "data: "+data)
				s.clearDeferredFieldLines()
				s.current = expanded[0]
				s.pending = append(s.pending, expanded[1:]...)
				s.frameHasExplicitEvent = true
				s.frameHasCompleteTypedData = true
				s.frameRepairDisabled = false
				return true
			}
			s.frameHasCompleteTypedData = s.frameHasExplicitEvent
			s.current = line
			return true
		}
		if len(data) > maxOpenAIConcatenatedJSONBytes {
			if len(s.deferredFieldLines) > 0 {
				s.frameHasCompleteTypedData = false
				s.frameRepairDisabled = true
				return s.emitDeferredFieldLines(line)
			}
			s.current = line
			return true
		}
		if document, repaired := splitOpenAITypedJSONAndDoneMarker(dataBytes); repaired {
			eventType, _ := openAITypedEventJSON(document)
			expanded := make([]string, 0, 6+len(s.deferredFieldLines))
			if s.frameHasCompleteTypedData {
				expanded = append(expanded, "")
				expanded = append(expanded, s.deferredFieldLines...)
				if !s.deferredFieldsHaveEvent() {
					expanded = append(expanded, "event: "+eventType)
				}
			}
			expanded = append(expanded, "data: "+string(document), "", "data: [DONE]")
			s.clearDeferredFieldLines()
			s.frameHasExplicitEvent = false
			s.frameHasCompleteTypedData = false
			s.frameRepairDisabled = false
			s.current = expanded[0]
			s.pending = expanded[1:]
			return true
		}
		documents, repaired := splitOpenAIConcatenatedJSONDocuments(dataBytes)
		if !repaired {
			if len(s.deferredFieldLines) > 0 {
				s.frameHasCompleteTypedData = false
				s.frameRepairDisabled = true
				return s.emitDeferredFieldLines(line)
			}
			s.current = line
			return true
		}

		expanded := make([]string, 0, len(documents)*3+1+len(s.deferredFieldLines))
		if s.frameHasCompleteTypedData {
			expanded = append(expanded, "")
			expanded = append(expanded, s.deferredFieldLines...)
		}
		for i, document := range documents {
			eventType, _ := openAITypedEventJSON(document)
			if i > 0 || (s.frameHasCompleteTypedData && !s.deferredFieldsHaveEvent()) {
				expanded = append(expanded, "event: "+eventType)
			}
			expanded = append(expanded, "data: "+string(document), "")
		}
		s.clearDeferredFieldLines()
		s.frameHasExplicitEvent = false
		s.frameHasCompleteTypedData = false
		s.frameRepairDisabled = false
		s.current = expanded[0]
		s.pending = expanded[1:]
		return true
	}
}

func (s *openAISSEJSONDocumentScanner) deferSSEFieldLine(line string) bool {
	lineBytes := len(line) + 1
	if len(s.deferredFieldLines) >= maxOpenAIDeferredSSEFieldLines || lineBytes > maxOpenAIDeferredSSEFieldBytes-s.deferredFieldBytes {
		s.err = errOpenAIDeferredSSEFieldsLimit
		return false
	}
	s.deferredFieldLines = append(s.deferredFieldLines, line)
	s.deferredFieldBytes += lineBytes
	return true
}

func (s *openAISSEJSONDocumentScanner) deferredFieldsHaveEvent() bool {
	for _, line := range s.deferredFieldLines {
		if _, ok := extractOpenAISSEEventLine(line); ok {
			return true
		}
	}
	return false
}

func (s *openAISSEJSONDocumentScanner) clearDeferredFieldLines() {
	s.deferredFieldLines = nil
	s.deferredFieldBytes = 0
}

func (s *openAISSEJSONDocumentScanner) emitDeferredFieldLines(lines ...string) bool {
	expanded := make([]string, 0, len(s.deferredFieldLines)+len(lines))
	expanded = append(expanded, s.deferredFieldLines...)
	expanded = append(expanded, lines...)
	s.clearDeferredFieldLines()
	s.current = expanded[0]
	s.pending = append(s.pending, expanded[1:]...)
	return true
}

func (s *openAISSEJSONDocumentScanner) Text() string {
	return s.current
}

func (s *openAISSEJSONDocumentScanner) Err() error {
	if s.err != nil {
		return s.err
	}
	if s.scanner == nil {
		return nil
	}
	return s.scanner.Err()
}

package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

const (
	maxOpenAIConcatenatedJSONDocuments = 16
	maxOpenAIConcatenatedJSONBytes     = 16 * 1024 * 1024
)

// splitOpenAIConcatenatedJSONDocuments recognizes the narrow corruption shape
// produced when multiple complete Responses events arrive in one transport
// message. Other malformed payloads are left untouched for normal error paths.
func splitOpenAIConcatenatedJSONDocuments(payload []byte) ([][]byte, bool) {
	documents, repaired, _ := classifyOpenAIConcatenatedJSONDocuments(payload)
	return documents, repaired
}

// WS accepts large valid single frames even when they exceed the repair limit.
// A repaired result contains independently decoded, valid documents. Reuse that
// proof for the first document and its queued siblings in the forwarding loop.
func prepareOpenAIWSJSONDocuments(payload []byte) (documents [][]byte, repaired, valid bool) {
	documents, repaired, valid = classifyOpenAIConcatenatedJSONDocuments(payload)
	if valid {
		// Classification validates the broadly trimmed payload. A length check
		// against JSON-only trimming proves its outer bytes were also JSON ws,
		// without rescanning the document (form-feed and NBSP must still fail).
		valid = len(bytes.Trim(payload, " \t\r\n")) == len(bytes.TrimSpace(payload))
	} else if !repaired && len(payload) > maxOpenAIConcatenatedJSONBytes {
		// The repair classifier skips oversized payloads, but WS historically
		// accepted any single frame that passes strict JSON validation.
		valid = json.Valid(payload)
	}
	return documents, repaired, valid || repaired
}

func classifyOpenAIConcatenatedJSONDocuments(payload []byte) (documents [][]byte, repaired, valid bool) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || len(payload) > maxOpenAIConcatenatedJSONBytes {
		return nil, false, false
	}
	if json.Valid(payload) {
		return nil, false, true
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	documents = make([][]byte, 0, 2)
	for {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if err != nil {
			if err == io.EOF && len(documents) > 1 {
				return documents, true, false
			}
			return nil, false, false
		}
		raw = bytes.TrimSpace(raw)
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, false, false
		}
		eventType := strings.TrimSpace(envelope.Type)
		if eventType == "" || strings.ContainsAny(eventType, "\r\n") {
			return nil, false, false
		}
		if len(documents) == maxOpenAIConcatenatedJSONDocuments {
			return nil, false, false
		}
		documents = append(documents, raw)
	}
}

type openAISSEJSONDocumentScanner struct {
	scanner          *bufio.Scanner
	pending          []string
	current          string
	currentDataValid bool
}

func newOpenAISSEJSONDocumentScanner(scanner *bufio.Scanner) *openAISSEJSONDocumentScanner {
	return &openAISSEJSONDocumentScanner{scanner: scanner}
}

func (s *openAISSEJSONDocumentScanner) Scan() bool {
	if len(s.pending) > 0 {
		s.current = s.pending[0]
		s.pending = s.pending[1:]
		s.currentDataValid = true
		return true
	}
	if s.scanner == nil || !s.scanner.Scan() {
		s.currentDataValid = false
		return false
	}

	line := s.scanner.Text()
	data, ok := extractOpenAISSEDataLine(line)
	if !ok {
		s.current = line
		s.currentDataValid = true
		return true
	}
	if len(data) > maxOpenAIConcatenatedJSONBytes {
		s.current = line
		s.currentDataValid = false
		return true
	}
	documents, repaired, valid := classifyOpenAIConcatenatedJSONDocuments([]byte(data))
	if !repaired {
		s.current = line
		s.currentDataValid = valid
		return true
	}

	expanded := make([]string, 0, len(documents)*3)
	for i, document := range documents {
		if i > 0 {
			var envelope struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(document, &envelope)
			expanded = append(expanded, "event: "+strings.TrimSpace(envelope.Type))
		}
		expanded = append(expanded, "data: "+string(document), "")
	}
	s.current = expanded[0]
	s.currentDataValid = true
	s.pending = expanded[1:]
	return true
}

func (s *openAISSEJSONDocumentScanner) Text() string {
	return s.current
}

func (s *openAISSEJSONDocumentScanner) DataValid() bool {
	return s.currentDataValid
}

func (s *openAISSEJSONDocumentScanner) Err() error {
	if s.scanner == nil {
		return nil
	}
	return s.scanner.Err()
}

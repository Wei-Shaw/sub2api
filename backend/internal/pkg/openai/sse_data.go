package openai

import (
	"strings"

	"github.com/tidwall/gjson"
)

// ExtractSSEDataLine extracts the payload from a SSE `data:` line.
// Compatible with both `data: xxx` and `data:xxx`.
func ExtractSSEDataLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	start := len("data:")
	for start < len(line) {
		if line[start] != ' ' && line[start] != '\t' {
			break
		}
		start++
	}
	return line[start:], true
}

// ExtractSSEEventLine extracts the event type from a SSE `event:` line.
func ExtractSSEEventLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "event:") {
		return "", false
	}
	start := len("event:")
	for start < len(line) {
		if line[start] != ' ' && line[start] != '\t' {
			break
		}
		start++
	}
	return strings.TrimSpace(line[start:]), true
}

// SSEDataAccumulator accumulates multi-line SSE data: frames and emits payloads.
type SSEDataAccumulator struct {
	lines []string
}

// AddLine processes one SSE line. When a blank line ends a frame, payloads are
// flushed via fn.
func (a *SSEDataAccumulator) AddLine(line string, fn func([]byte)) {
	if fn == nil {
		return
	}
	trimmedLine := strings.TrimRight(line, "\r\n")
	if data, ok := ExtractSSEDataLine(trimmedLine); ok {
		a.lines = append(a.lines, data)
		return
	}
	if strings.TrimSpace(trimmedLine) == "" {
		a.Flush(fn)
	}
}

// Flush emits any buffered multi-line data payload.
func (a *SSEDataAccumulator) Flush(fn func([]byte)) {
	if fn == nil || len(a.lines) == 0 {
		return
	}
	emitSSEDataPayloads(a.lines, fn)
	a.lines = a.lines[:0]
}

// ForEachSSEDataPayload walks body and invokes fn for each data payload.
func ForEachSSEDataPayload(body string, fn func([]byte)) {
	if fn == nil || strings.TrimSpace(body) == "" {
		return
	}
	var acc SSEDataAccumulator
	for _, line := range strings.Split(body, "\n") {
		acc.AddLine(line, fn)
	}
	acc.Flush(fn)
}

func emitSSEDataPayloads(lines []string, fn func([]byte)) {
	if fn == nil || len(lines) == 0 {
		return
	}
	if len(lines) == 1 {
		emitSSEDataPayload(lines[0], fn)
		return
	}
	joined := strings.Join(lines, "\n")
	if gjson.Valid(joined) {
		emitSSEDataPayload(joined, fn)
		return
	}
	for _, line := range lines {
		emitSSEDataPayload(line, fn)
	}
}

func emitSSEDataPayload(data string, fn func([]byte)) {
	data = strings.TrimSpace(data)
	if data == "" || data == "[DONE]" {
		return
	}
	fn([]byte(data))
}

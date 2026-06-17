package deepseek

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// StreamConverter converts DeepSeek SSE events to OpenAI SSE format.
type StreamConverter struct {
	reader   *bufio.Reader
	model     string
	created   int64
	chunkID   string
	closed    bool
	finished  bool
}

// NewStreamConverter creates a new stream converter that reads from the given reader.
func NewStreamConverter(reader io.Reader, model string, completionID string) *StreamConverter {
	return &StreamConverter{
		reader:  bufio.NewReaderSize(reader, 64*1024),
		model:   model,
		created: time.Now().Unix(),
		chunkID: completionID,
	}
}

// NextEvent reads the next DeepSeek SSE event and returns the OpenAI SSE formatted string.
// Returns (event, done, error). When done is true, the stream has ended.
func (sc *StreamConverter) NextEvent() (string, bool, error) {
	if sc.closed || sc.finished {
		return "", true, nil
	}

	for {
		line, err := sc.reader.ReadBytes('\n')
		if len(line) > 0 {
			lineStr := strings.TrimSpace(string(line))

			// Skip empty lines
			if lineStr == "" {
				continue
			}

			// Skip SSE event name lines (e.g., "event: update_session", "event: title", "event: close")
			if strings.HasPrefix(lineStr, "event:") {
				continue
			}

			// Handle [DONE]
			if lineStr == "data: [DONE]" || lineStr == "[DONE]" {
				sc.finished = true
				return "data: [DONE]\n\n", true, nil
			}

			// Strip "data: " prefix
			data := lineStr
			if strings.HasPrefix(data, "data: ") {
				data = data[6:]
			}

			// Parse DeepSeek JSON Patch event
			var event map[string]any
			if jsonErr := json.Unmarshal([]byte(data), &event); jsonErr != nil {
				continue // Skip unparseable events
			}

			chunk := sc.convertEvent(event)
			if chunk != "" {
				return chunk, false, nil
			}
			continue
		}
		if err != nil {
			if err == io.EOF {
				sc.finished = true
				return "data: [DONE]\n\n", true, nil
			}
			return "", true, err
		}
	}
}

// Close marks the converter as closed.
func (sc *StreamConverter) Close() {
	sc.closed = true
}

// convertEvent converts a single DeepSeek event to OpenAI SSE chunk.
func (sc *StreamConverter) convertEvent(event map[string]any) string {
	path, _ := event["p"].(string)
	op, _ := event["o"].(string)
	value := event["v"]

	// New format: {"v":"text"} — content token without p/o fields
	if path == "" && op == "" {
		if text, ok := value.(string); ok && text != "" {
			return sc.buildTextChunk(text)
		}
		return ""
	}

	// Handle fragment content: {"p":"response/fragments/0/content","o":"REPLACE","v":"text"}
	if strings.Contains(path, "fragments") && strings.HasSuffix(path, "/content") && op == "REPLACE" {
		text, ok := value.(string)
		if !ok || text == "" {
			return ""
		}
		return sc.buildTextChunk(text)
	}

	// Handle fragment append: {"p":"response/fragments","o":"APPEND","v":[{...}]}
	if path == "response/fragments" && op == "APPEND" {
		fragments, ok := value.([]any)
		if !ok {
			return ""
		}
		var result strings.Builder
		for _, f := range fragments {
			frag, ok := f.(map[string]any)
			if !ok {
				continue
			}
			fragType, _ := frag["type"].(string)
			content, _ := frag["content"].(string)
			if content == "" {
				continue
			}
			if fragType == "THINKING" {
				result.WriteString(sc.buildThinkingChunk(content))
			} else {
				result.WriteString(sc.buildTextChunk(content))
			}
		}
		return result.String()
	}

	// Handle status events for finish reason
	if strings.Contains(path, "status") {
		if statusStr, ok := value.(string); ok && statusStr == "FINISHED" {
			return sc.buildFinishChunk()
		}
	}

	// Handle BATCH events (e.g., {"p":"response","o":"BATCH","v":[...]})
	if op == "BATCH" {
		if items, ok := value.([]any); ok {
			for _, item := range items {
				if itemMap, ok := item.(map[string]any); ok {
					// Check for status in batch
					if p, _ := itemMap["p"].(string); strings.Contains(p, "status") {
						if v, _ := itemMap["v"].(string); v == "FINISHED" {
							return sc.buildFinishChunk()
						}
					}
					// Check for quasi_status
					if p, _ := itemMap["p"].(string); p == "quasi_status" {
						if v, _ := itemMap["v"].(string); v == "FINISHED" {
							return sc.buildFinishChunk()
						}
					}
				}
			}
		}
	}

	return ""
}

func (sc *StreamConverter) buildTextChunk(text string) string {
	chunk := map[string]any{
		"id":      sc.chunkID,
		"object":  "chat.completion.chunk",
		"created": sc.created,
		"model":   sc.model,
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"content": text,
				},
				"finish_reason": nil,
			},
		},
	}
	b, _ := json.Marshal(chunk)
	return "data: " + string(b) + "\n\n"
}

func (sc *StreamConverter) buildThinkingChunk(text string) string {
	chunk := map[string]any{
		"id":      sc.chunkID,
		"object":  "chat.completion.chunk",
		"created": sc.created,
		"model":   sc.model,
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"reasoning_content": text,
				},
				"finish_reason": nil,
			},
		},
	}
	b, _ := json.Marshal(chunk)
	return "data: " + string(b) + "\n\n"
}

func (sc *StreamConverter) buildFinishChunk() string {
	chunk := map[string]any{
		"id":      sc.chunkID,
		"object":  "chat.completion.chunk",
		"created": sc.created,
		"model":   sc.model,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "stop",
			},
		},
	}
	b, _ := json.Marshal(chunk)
	return "data: " + string(b) + "\n\n"
}

package kiro

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
)

// AnthropicSSEWriter adapts a StreamCallback to Anthropic Messages SSE
// output. Each callback invocation immediately writes the matching event
// frame to the underlying writer; nothing is buffered.
//
// Anthropic's streaming protocol is documented here:
//
//	https://docs.anthropic.com/en/api/messages-streaming
//
// The contract this writer implements:
//
//	message_start              (once at the top)
//	content_block_start (text) (once before the first text/thinking delta)
//	content_block_delta...     (per text/thinking chunk)
//	content_block_stop         (after each content block)
//	content_block_start (tool_use)
//	content_block_delta (input_json_delta)
//	content_block_stop
//	message_delta              (with stop_reason + usage)
//	message_stop
//
// Flusher is called after each write so the chunk is pushed to the client.
type AnthropicSSEWriter struct {
	w           io.Writer
	flusher     func()
	model       string
	messageID   string
	textOpened  bool
	thinkOpened bool
	blockIdx    int
	stopReason  string
}

// NewAnthropicSSEWriter constructs the writer and emits message_start.
// The caller is responsible for setting the SSE response headers
// (Content-Type: text/event-stream, etc.) before invoking this.
//
// flusher is the gin.ResponseWriter Flush() (or equivalent) — empty
// function is fine for tests / non-streaming consumers.
func NewAnthropicSSEWriter(w io.Writer, flusher func(), model string) *AnthropicSSEWriter {
	if flusher == nil {
		flusher = func() {}
	}
	id := "msg_" + uuid.New().String()
	out := &AnthropicSSEWriter{
		w:         w,
		flusher:   flusher,
		model:     model,
		messageID: id,
		blockIdx:  0,
	}
	out.writeEvent("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            id,
			"type":          "message",
			"role":          "assistant",
			"content":       []any{},
			"model":         model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	})
	return out
}

// Callback returns a StreamCallback whose methods wire into this writer.
// Suitable for ProcessEventsFromCallback.
func (w *AnthropicSSEWriter) Callback() *StreamCallback {
	return &StreamCallback{
		OnText: func(text string, isThinking bool) {
			w.WriteText(text, isThinking)
		},
		OnToolUse:  w.WriteToolUse,
		OnComplete: w.WriteFinal,
	}
}

// WriteText writes a content_block_delta for either text or thinking.
// Opens the corresponding block on first use.
func (w *AnthropicSSEWriter) WriteText(text string, isThinking bool) {
	if text == "" {
		return
	}
	if isThinking {
		if !w.thinkOpened {
			w.openBlock("thinking", map[string]any{
				"type":     "thinking",
				"thinking": "",
			})
			w.thinkOpened = true
		}
		w.writeEvent("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": w.blockIdx,
			"delta": map[string]any{
				"type":     "thinking_delta",
				"thinking": text,
			},
		})
		return
	}
	if w.thinkOpened {
		w.closeBlock()
		w.thinkOpened = false
	}
	if !w.textOpened {
		w.openBlock("text", map[string]any{
			"type": "text",
			"text": "",
		})
		w.textOpened = true
	}
	w.writeEvent("content_block_delta", map[string]any{
		"type":  "content_block_delta",
		"index": w.blockIdx,
		"delta": map[string]any{
			"type": "text_delta",
			"text": text,
		},
	})
}

// WriteToolUse writes a complete tool_use block (start + input_json_delta + stop).
// The tool input is fully materialised by the time the callback fires, so
// we emit it as a single JSON chunk.
func (w *AnthropicSSEWriter) WriteToolUse(tu ToolUse) {
	if w.textOpened {
		w.closeBlock()
		w.textOpened = false
	}
	if w.thinkOpened {
		w.closeBlock()
		w.thinkOpened = false
	}
	w.openBlock("tool_use", map[string]any{
		"type":  "tool_use",
		"id":    tu.ToolUseID,
		"name":  tu.Name,
		"input": map[string]any{},
	})
	inputJSON, _ := json.Marshal(tu.Input)
	w.writeEvent("content_block_delta", map[string]any{
		"type":  "content_block_delta",
		"index": w.blockIdx,
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": string(inputJSON),
		},
	})
	w.closeBlock()
	w.stopReason = "tool_use"
}

// WriteFinal closes any open blocks and emits message_delta / message_stop
// with the final usage figures. Idempotent — safe to call once at end.
func (w *AnthropicSSEWriter) WriteFinal(inputTokens, outputTokens int) {
	if w.textOpened {
		w.closeBlock()
		w.textOpened = false
	}
	if w.thinkOpened {
		w.closeBlock()
		w.thinkOpened = false
	}
	stop := w.stopReason
	if stop == "" {
		stop = "end_turn"
	}
	w.writeEvent("message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stop,
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	})
	w.writeEvent("message_stop", map[string]any{
		"type": "message_stop",
	})
}

func (w *AnthropicSSEWriter) openBlock(blockType string, contentBlock map[string]any) {
	w.writeEvent("content_block_start", map[string]any{
		"type":          "content_block_start",
		"index":         w.blockIdx,
		"content_block": contentBlock,
	})
}

func (w *AnthropicSSEWriter) closeBlock() {
	w.writeEvent("content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": w.blockIdx,
	})
	w.blockIdx++
}

func (w *AnthropicSSEWriter) writeEvent(event string, data map[string]any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	var sb strings.Builder
	sb.WriteString("event: ")
	sb.WriteString(event)
	sb.WriteString("\ndata: ")
	sb.Write(payload)
	sb.WriteString("\n\n")
	_, _ = io.WriteString(w.w, sb.String())
	w.flusher()
}

// BuildAnthropicNonStreamingResponse aggregates events into the
// non-streaming Messages API response shape. Useful when the inbound
// request had `stream: false`.
type AnthropicNonStreamingResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Model        string                  `json:"model"`
	Content      []map[string]any        `json:"content"`
	StopReason   string                  `json:"stop_reason"`
	StopSequence *string                 `json:"stop_sequence"`
	Usage        map[string]int          `json:"usage"`
}

// BuildAnthropicNonStreamingResponse drives the response transformer
// over a captured event sequence and returns the resulting full body.
func BuildAnthropicNonStreamingResponse(events []Event, model string, toolNameMap map[string]string) *AnthropicNonStreamingResponse {
	var textBuf strings.Builder
	var thinkBuf strings.Builder
	var tools []ToolUse
	var inputTokens, outputTokens int

	cb := &StreamCallback{
		OnText: func(text string, isThinking bool) {
			if isThinking {
				thinkBuf.WriteString(text)
			} else {
				textBuf.WriteString(text)
			}
		},
		OnToolUse: func(tu ToolUse) {
			tools = append(tools, tu)
		},
		OnComplete: func(inT, outT int) {
			inputTokens = inT
			outputTokens = outT
		},
	}
	ProcessEvents(events, toolNameMap, cb)

	blocks := make([]map[string]any, 0, 2+len(tools))
	if thinkBuf.Len() > 0 {
		blocks = append(blocks, map[string]any{
			"type":     "thinking",
			"thinking": thinkBuf.String(),
		})
	}
	if textBuf.Len() > 0 {
		blocks = append(blocks, map[string]any{
			"type": "text",
			"text": textBuf.String(),
		})
	}
	stop := "end_turn"
	for _, tu := range tools {
		blocks = append(blocks, map[string]any{
			"type":  "tool_use",
			"id":    tu.ToolUseID,
			"name":  tu.Name,
			"input": tu.Input,
		})
		stop = "tool_use"
	}

	return &AnthropicNonStreamingResponse{
		ID:         "msg_" + uuid.New().String(),
		Type:       "message",
		Role:       "assistant",
		Model:      model,
		Content:    blocks,
		StopReason: stop,
		Usage: map[string]int{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}
}

// HTTPErrorf is a sentinel-shaped error so callers can distinguish
// upstream HTTP failures from local ones.
type HTTPError struct {
	StatusCode int
	Body       []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("kiro upstream HTTP %d: %s", e.StatusCode, string(e.Body))
}

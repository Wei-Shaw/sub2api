package kiro

import (
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

// OpenAISSEWriter adapts a StreamCallback to OpenAI Chat Completions
// SSE output.
//
// OpenAI's streaming protocol emits one JSON chunk per delta, formatted as:
//
//	data: {"id":"chatcmpl-…","object":"chat.completion.chunk","created":…,
//	       "model":"…","choices":[{"index":0,"delta":{"content":"…"},
//	                               "finish_reason":null}]}
//
// followed by a terminating `data: [DONE]`. Each chunk is one SSE event
// (no `event:` line — OpenAI uses bare `data:`). Tool calls stream as a
// single delta with tool_calls populated.
type OpenAISSEWriter struct {
	w        io.Writer
	flusher  func()
	id       string
	model    string
	created  int64
	finished bool
}

// NewOpenAISSEWriter constructs the writer. No header chunk is emitted
// up front — OpenAI's protocol starts with the first content delta.
func NewOpenAISSEWriter(w io.Writer, flusher func(), model string) *OpenAISSEWriter {
	if flusher == nil {
		flusher = func() {}
	}
	return &OpenAISSEWriter{
		w:       w,
		flusher: flusher,
		id:      "chatcmpl-" + uuid.New().String(),
		model:   model,
		created: time.Now().Unix(),
	}
}

// Callback returns a StreamCallback wired into this writer.
func (w *OpenAISSEWriter) Callback() *StreamCallback {
	return &StreamCallback{
		OnText:     w.WriteText,
		OnToolUse:  w.WriteToolUse,
		OnComplete: w.WriteFinal,
	}
}

// WriteText emits a text delta. isThinking is reported as a separate
// reasoning_content field (matches Kiro-Go's OpenAI thinking shape).
func (w *OpenAISSEWriter) WriteText(text string, isThinking bool) {
	if text == "" {
		return
	}
	delta := map[string]any{}
	if isThinking {
		delta["reasoning_content"] = text
	} else {
		delta["content"] = text
	}
	w.writeChunk(map[string]any{
		"id":      w.id,
		"object":  "chat.completion.chunk",
		"created": w.created,
		"model":   w.model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         delta,
			"finish_reason": nil,
		}},
	})
}

// WriteToolUse emits a tool_calls delta. OpenAI tool calls stream
// arguments incrementally, but our upstream materialises the full input
// before invoking OnToolUse, so we emit a single chunk with the
// complete arguments string.
func (w *OpenAISSEWriter) WriteToolUse(tu ToolUse) {
	args, _ := json.Marshal(tu.Input)
	w.writeChunk(map[string]any{
		"id":      w.id,
		"object":  "chat.completion.chunk",
		"created": w.created,
		"model":   w.model,
		"choices": []map[string]any{{
			"index": 0,
			"delta": map[string]any{
				"tool_calls": []map[string]any{{
					"index": 0,
					"id":    tu.ToolUseID,
					"type":  "function",
					"function": map[string]any{
						"name":      tu.Name,
						"arguments": string(args),
					},
				}},
			},
			"finish_reason": nil,
		}},
	})
}

// WriteFinal emits the final chunk (finish_reason="stop" / "tool_calls")
// followed by `data: [DONE]`. Idempotent — additional calls are no-ops.
func (w *OpenAISSEWriter) WriteFinal(inputTokens, outputTokens int) {
	if w.finished {
		return
	}
	w.finished = true

	w.writeChunk(map[string]any{
		"id":      w.id,
		"object":  "chat.completion.chunk",
		"created": w.created,
		"model":   w.model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         map[string]any{},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{
			"prompt_tokens":     inputTokens,
			"completion_tokens": outputTokens,
			"total_tokens":      inputTokens + outputTokens,
		},
	})
	// `data: [DONE]` is the canonical end-of-stream marker for OpenAI.
	_, _ = io.WriteString(w.w, "data: [DONE]\n\n")
	w.flusher()
}

func (w *OpenAISSEWriter) writeChunk(payload map[string]any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	var sb strings.Builder
	sb.WriteString("data: ")
	sb.Write(body)
	sb.WriteString("\n\n")
	_, _ = io.WriteString(w.w, sb.String())
	w.flusher()
}

// OpenAINonStreamingResponse is the shape returned for stream=false
// Chat Completions requests.
type OpenAINonStreamingResponse struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Choices []OpenAIChoice    `json:"choices"`
	Usage   map[string]int    `json:"usage"`
}

// OpenAIChoice is one item in the non-streaming choices array.
type OpenAIChoice struct {
	Index        int            `json:"index"`
	Message      OpenAIMsg      `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

// OpenAIMsg is the assistant message body inside a Choice.
type OpenAIMsg struct {
	Role             string                  `json:"role"`
	Content          string                  `json:"content,omitempty"`
	ReasoningContent string                  `json:"reasoning_content,omitempty"`
	ToolCalls        []OpenAIChoiceToolCall  `json:"tool_calls,omitempty"`
}

// OpenAIChoiceToolCall is a fully materialised tool call in a
// non-streaming response.
type OpenAIChoiceToolCall struct {
	ID       string                       `json:"id"`
	Type     string                       `json:"type"`
	Function OpenAIChoiceToolCallFunction `json:"function"`
}

// OpenAIChoiceToolCallFunction carries the tool name + arguments string.
type OpenAIChoiceToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// BuildOpenAINonStreamingResponse drives ProcessEvents over a captured
// event sequence and returns the resulting Chat Completions response.
func BuildOpenAINonStreamingResponse(events []Event, model string, toolNameMap map[string]string) *OpenAINonStreamingResponse {
	var textBuf strings.Builder
	var reasonBuf strings.Builder
	var tools []ToolUse
	var inputTokens, outputTokens int

	cb := &StreamCallback{
		OnText: func(text string, isThinking bool) {
			if isThinking {
				reasonBuf.WriteString(text)
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

	msg := OpenAIMsg{Role: "assistant"}
	if textBuf.Len() > 0 {
		msg.Content = textBuf.String()
	}
	if reasonBuf.Len() > 0 {
		msg.ReasoningContent = reasonBuf.String()
	}
	finish := "stop"
	for _, tu := range tools {
		args, _ := json.Marshal(tu.Input)
		msg.ToolCalls = append(msg.ToolCalls, OpenAIChoiceToolCall{
			ID:   tu.ToolUseID,
			Type: "function",
			Function: OpenAIChoiceToolCallFunction{
				Name:      tu.Name,
				Arguments: string(args),
			},
		})
		finish = "tool_calls"
	}

	return &OpenAINonStreamingResponse{
		ID:      "chatcmpl-" + uuid.New().String(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []OpenAIChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: finish,
		}},
		Usage: map[string]int{
			"prompt_tokens":     inputTokens,
			"completion_tokens": outputTokens,
			"total_tokens":      inputTokens + outputTokens,
		},
	}
}

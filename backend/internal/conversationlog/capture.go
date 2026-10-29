package conversationlog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

type Identity struct {
	UserID, KeyID      int64
	UserLabel, KeyName string
}
type IdentityResolver func(*gin.Context) (Identity, bool)

type boundedBuffer struct {
	data  []byte
	limit int
	total int64
}

func (b *boundedBuffer) append(p []byte) {
	b.total += int64(len(p))
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.data = append(b.data, p...)
	}
}
func (b *boundedBuffer) truncated() bool { return b.total > int64(b.limit) }

type bodyCapture struct {
	io.ReadCloser
	buffer *boundedBuffer
}

func (b *bodyCapture) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.buffer.append(p[:n])
	return n, err
}

type captureWriter struct {
	gin.ResponseWriter
	buffer   *boundedBuffer
	writeErr error
}

func (w *captureWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.buffer.append(p[:n])
	if err != nil {
		w.writeErr = err
	}
	return n, err
}
func (w *captureWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *captureWriter) Unwrap() http.ResponseWriter       { return w.ResponseWriter }

func supportedPath(path string) bool {
	switch path {
	case "/v1/chat/completions", "/chat/completions", "/v1/responses", "/responses", "/backend-api/codex/responses", "/v1/messages", "/messages", "/antigravity/v1/messages":
		return true
	}
	return false
}

// Middleware captures the downstream wire payload after protocol conversion.
// POST-only means WebSocket upgrades, images, and unrelated admin traffic are
// deliberately excluded. Flushing and all other writer interfaces are preserved.
func (r *Recorder) Middleware(identity IdentityResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := *r.config.Load()
		if !cfg.Enabled || c.Request.Method != http.MethodPost || !supportedPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		select {
		case r.slots <- struct{}{}:
		default:
			r.dropped.Add(1)
			c.Next()
			return
		}
		defer func() { <-r.slots }()
		start := time.Now()
		requestBuffer := &boundedBuffer{limit: cfg.MaxBodyBytes}
		responseBuffer := &boundedBuffer{limit: cfg.MaxBodyBytes}
		if c.Request.Body != nil {
			c.Request.Body = &bodyCapture{ReadCloser: c.Request.Body, buffer: requestBuffer}
		}
		original := c.Writer
		writer := &captureWriter{ResponseWriter: original, buffer: responseBuffer}
		c.Writer = writer
		defer func() { c.Writer = original }()
		c.Next()
		who, ok := identity(c)
		if !ok || who.UserID <= 0 || who.KeyID <= 0 {
			return
		}
		requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
		if requestID == "" {
			requestID = uuid.NewString()
		}
		stream := gjson.GetBytes(requestBuffer.data, "stream").Bool() || strings.Contains(writer.Header().Get("Content-Type"), "text/event-stream")
		answer, tools, terminal, failed := parseResponse(responseBuffer.data, stream, cfg.RedactSensitive)
		outcome := "completed"
		if writer.Status() >= 400 || failed {
			outcome = "failed"
		} else if writer.writeErr != nil || c.Request.Context().Err() != nil {
			outcome = "interrupted"
		} else if stream && !terminal {
			outcome = "incomplete"
		}
		if responseBuffer.truncated() && outcome == "completed" {
			outcome = "capture_truncated"
		}
		requestBody := sanitizeJSON(requestBuffer.data, cfg.RedactSensitive)
		r.enqueue(Record{
			RequestID: clip(requestID, 128), CreatedAt: start, UserID: who.UserID, APIKeyID: who.KeyID,
			UserLabel: clip(who.UserLabel, 320), KeyName: clip(who.KeyName, 256), Endpoint: c.Request.URL.Path,
			Model: clip(gjson.GetBytes(requestBuffer.data, "model").String(), 256), Stream: stream, StatusCode: writer.Status(),
			Outcome: outcome, DurationMS: time.Since(start).Milliseconds(), Prompt: latestPrompt(requestBody), Answer: postgresText(answer),
			RequestBody: requestBody, ResponseBody: sanitizeResponse(responseBuffer.data, stream, cfg.RedactSensitive), ToolCalls: tools,
			RequestTruncated: requestBuffer.truncated(), ResponseTruncated: responseBuffer.truncated(),
			RequestBytes: requestBuffer.total, ResponseBytes: responseBuffer.total,
			Redacted: cfg.RedactSensitive,
		})
	}
}

func clip(s string, max int) string {
	s = postgresText(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// PostgreSQL text and JSONB reject NUL even when valid JSON encodes it as \u0000.
// Normalize only the saved copy; never modify the request or response on the wire.
func postgresText(s string) string {
	return strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", "\uFFFD"), "\uFFFD")
}

func redact(v any, mask bool) any {
	switch x := v.(type) {
	case map[string]any:
		for k, item := range x {
			key := postgresText(k)
			if key != k {
				delete(x, k)
				x[key] = item
			}
			switch strings.ToLower(strings.ReplaceAll(key, "-", "_")) {
			case "authorization", "api_key", "access_token", "refresh_token", "password", "client_secret":
				if mask {
					x[key] = "[redacted]"
				} else {
					x[key] = redact(item, false)
				}
			case "data":
				if _, ok := item.(string); ok {
					x[key] = "[binary data omitted]"
				} else {
					x[key] = redact(item, mask)
				}
			default:
				x[key] = redact(item, mask)
			}
		}
	case []any:
		for i, item := range x {
			x[i] = redact(item, mask)
		}
	case string:
		if strings.HasPrefix(x, "data:") {
			return "[data URL omitted]"
		}
		return postgresText(x)
	}
	return v
}

func sanitizeJSON(b []byte, mask bool) string {
	var value any
	if json.Unmarshal(b, &value) != nil {
		return `{"capture_note":"Payload was truncated, empty or not valid JSON; raw bytes omitted."}`
	}
	out, _ := json.Marshal(redact(value, mask))
	return string(out)
}

func sanitizeResponse(b []byte, stream, mask bool) string {
	if !stream {
		return sanitizeJSON(b, mask)
	}
	var result strings.Builder
	for _, event := range sseData(b) {
		if event == "[DONE]" {
			result.WriteString("data: [DONE]\n\n")
			continue
		}
		result.WriteString("data: ")
		result.WriteString(sanitizeJSON([]byte(event), mask))
		result.WriteString("\n\n")
	}
	return result.String()
}

func sseData(b []byte) []string {
	scanner := bufio.NewScanner(bytes.NewReader(b))
	scanner.Buffer(make([]byte, 4096), 2<<20+4096)
	events := []string{}
	lines := []string{}
	flush := func() {
		if len(lines) > 0 {
			events = append(events, strings.Join(lines, "\n"))
			lines = nil
		}
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			flush()
		} else if strings.HasPrefix(line, "data:") {
			lines = append(lines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	// The last event may be partial on client disconnect or capture truncation.
	flush()
	return events
}

func contentText(v gjson.Result) string {
	if v.Type == gjson.String {
		return v.String()
	}
	var text strings.Builder
	for _, part := range v.Array() {
		switch part.Get("type").String() {
		case "text", "input_text", "output_text":
			text.WriteString(part.Get("text").String())
		case "refusal":
			text.WriteString(part.Get("refusal").String())
		case "image_url", "input_image", "image":
			text.WriteString("[image]")
		case "tool_result":
			text.WriteString(contentText(part.Get("content")))
		}
	}
	return text.String()
}

func latestPrompt(body string) string {
	j := gjson.Parse(body)
	if j.Get("input").Type == gjson.String {
		return j.Get("input").String()
	}
	messages := j.Get("messages").Array()
	if len(messages) == 0 {
		messages = j.Get("input").Array()
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Get("role").String() == "user" {
			return contentText(messages[i].Get("content"))
		}
	}
	return ""
}

func responseParts(j gjson.Result) (string, []any) {
	var text strings.Builder
	tools := []any{}
	if j.Get("choices").Exists() {
		message := j.Get("choices.0.message")
		text.WriteString(contentText(message.Get("content")))
		text.WriteString(message.Get("refusal").String())
		for _, v := range message.Get("tool_calls").Array() {
			tools = append(tools, v.Value())
		}
	} else if j.Get("output").Exists() {
		for _, item := range j.Get("output").Array() {
			if item.Get("type").String() == "function_call" {
				tools = append(tools, item.Value())
			} else {
				text.WriteString(contentText(item.Get("content")))
			}
		}
	} else {
		text.WriteString(contentText(j.Get("content")))
		for _, v := range j.Get("content").Array() {
			if v.Get("type").String() == "tool_use" {
				tools = append(tools, v.Value())
			}
		}
	}
	return text.String(), tools
}

// Responses includes "error": null on successful replies. Only a non-null
// error payload is an error; mere field presence must not mark a reply failed.
func hasResponseError(j gjson.Result) bool {
	err := j.Get("error")
	return err.Exists() && err.Type != gjson.Null
}

// parseResponse joins Chat Completions, Responses and Anthropic SSE deltas.
// Only client-visible text and tool calls are synthesized; sanitized wire events
// remain available in the detail view for fields that are not normalized.
func parseResponse(body []byte, stream, mask bool) (string, json.RawMessage, bool, bool) {
	if !stream {
		j := gjson.ParseBytes(body)
		text, tools := responseParts(j)
		out, _ := json.Marshal(redact(tools, mask))
		return text, out, true, hasResponseError(j)
	}
	var answer strings.Builder
	tools := map[int]map[string]any{}
	order := []int{}
	terminal, failed := false, false
	for _, data := range sseData(body) {
		if data == "[DONE]" {
			terminal = true
			continue
		}
		if !gjson.Valid(data) {
			continue
		}
		j := gjson.Parse(data)
		event := j.Get("type").String()
		if hasResponseError(j) || event == "error" || event == "response.failed" {
			failed = true
			terminal = true
		}
		if event == "message_stop" || event == "response.completed" || event == "response.incomplete" {
			terminal = true
		}
		if event == "response.incomplete" {
			failed = true
		}
		if event == "response.completed" {
			text, finalTools := responseParts(j.Get("response"))
			if text != "" || len(finalTools) > 0 {
				out, _ := json.Marshal(redact(finalTools, mask))
				return text, out, true, failed
			}
		}
		answer.WriteString(j.Get("choices.0.delta.content").String())
		answer.WriteString(j.Get("choices.0.delta.refusal").String())
		if f := j.Get("choices.0.finish_reason"); f.Exists() && f.Type != gjson.Null {
			terminal = true
		}
		for _, call := range j.Get("choices.0.delta.tool_calls").Array() {
			idx := int(call.Get("index").Int())
			if tools[idx] == nil {
				tools[idx] = map[string]any{"id": "", "type": "function", "function": map[string]any{"name": "", "arguments": ""}}
				order = append(order, idx)
			}
			t := tools[idx]
			fn := t["function"].(map[string]any)
			t["id"] = t["id"].(string) + call.Get("id").String()
			fn["name"] = fn["name"].(string) + call.Get("function.name").String()
			fn["arguments"] = fn["arguments"].(string) + call.Get("function.arguments").String()
		}
		if event == "response.output_text.delta" || event == "response.refusal.delta" {
			answer.WriteString(j.Get("delta").String())
		}
		if event == "response.output_item.added" && j.Get("item.type").String() == "function_call" {
			idx := int(j.Get("output_index").Int())
			if tools[idx] == nil {
				order = append(order, idx)
			}
			tools[idx] = j.Get("item").Value().(map[string]any)
		}
		if event == "response.function_call_arguments.delta" {
			idx := int(j.Get("output_index").Int())
			if t := tools[idx]; t != nil {
				old, _ := t["arguments"].(string)
				t["arguments"] = old + j.Get("delta").String()
			}
		}
		if event == "content_block_start" && j.Get("content_block.type").String() == "tool_use" {
			idx := int(j.Get("index").Int())
			if tools[idx] == nil {
				order = append(order, idx)
			}
			tools[idx] = j.Get("content_block").Value().(map[string]any)
		}
		if event == "content_block_delta" {
			answer.WriteString(j.Get("delta.text").String())
			if j.Get("delta.type").String() == "input_json_delta" {
				if t := tools[int(j.Get("index").Int())]; t != nil {
					old, _ := t["partial_json"].(string)
					t["partial_json"] = old + j.Get("delta.partial_json").String()
				}
			}
		}
	}
	list := []any{}
	for _, idx := range order {
		list = append(list, tools[idx])
	}
	out, _ := json.Marshal(redact(list, mask))
	return answer.String(), out, terminal, failed
}

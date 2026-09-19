package keyprotection

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"net"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const maxProtectedResponse = 16 << 20
const maxProtectedEvent = 4 << 20

// ErrResponseProtection deliberately carries no body, candidate, or mapping data.
var ErrResponseProtection = errors.New("automatic key protection: unsafe or incomplete response")

// ResponseWriter is the final client-bound transformation, after protocol conversion.
// It is request-local and must not be wrapped by a logger that captures its output.
type ResponseWriter struct {
	gin.ResponseWriter
	state                                           *State
	protocol                                        string
	status, size                                    int
	started, prepared, streaming, finished, aborted bool
	err                                             error
	body                                            bytes.Buffer
	stream                                          *responseStream
}

func NewResponseWriter(dst gin.ResponseWriter, state *State, protocol string) *ResponseWriter {
	w := &ResponseWriter{ResponseWriter: dst, state: state, protocol: protocol, status: http.StatusOK, size: -1}
	w.stream = newResponseStream(w)
	return w
}

func (w *ResponseWriter) Status() int   { return w.status }
func (w *ResponseWriter) Size() int     { return w.size }
func (w *ResponseWriter) Written() bool { return w.started }
func (w *ResponseWriter) Err() error    { return w.err }
func (w *ResponseWriter) WriteHeader(code int) {
	if !w.started && code > 0 {
		w.status = code
	}
}
func (w *ResponseWriter) WriteHeaderNow() {
	if !w.started {
		w.started = true
		w.size = 0
	}
}
func (w *ResponseWriter) prepare() error {
	if w.prepared {
		return w.err
	}
	w.prepared = true
	if w.state == nil || (w.protocol != "chat" && w.protocol != "responses" && w.protocol != "messages") {
		return w.fail()
	}
	encoding := strings.TrimSpace(w.Header().Get("Content-Encoding"))
	if encoding != "" && !strings.EqualFold(encoding, "identity") {
		return w.fail()
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(w.Header().Get("Content-Type"), ";")[0]))
	w.streaming = contentType == "text/event-stream"
	if contentType != "" && contentType != "application/json" && !strings.HasSuffix(contentType, "+json") && !w.streaming {
		return w.fail()
	}
	w.Header().Del("Content-Length")
	w.Header().Del("ETag")
	w.Header().Set("Cache-Control", "private, no-store")
	return nil
}
func (w *ResponseWriter) Write(p []byte) (int, error) {
	if w.finished || w.aborted || w.err != nil {
		return 0, ErrResponseProtection
	}
	if err := w.prepare(); err != nil {
		return 0, err
	}
	w.WriteHeaderNow()
	if w.streaming {
		if err := w.stream.write(p); err != nil {
			return 0, w.fail()
		}
	} else {
		if w.body.Len()+len(p) > maxProtectedResponse {
			return 0, w.fail()
		}
		_, _ = w.body.Write(p)
	}
	w.size += len(p)
	return len(p), nil
}
func (w *ResponseWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *ResponseWriter) Flush() {
	if w.finished || w.aborted || w.err != nil {
		return
	}
	if w.prepare() != nil {
		return
	}
	// Incomplete frames and JSON bodies stay private until validated.
	if w.streaming && w.ResponseWriter.Written() {
		w.ResponseWriter.Flush()
	}
}

// Hijacking would bypass the checked HTTP/SSE boundary.
func (w *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, ErrResponseProtection
}

func (w *ResponseWriter) emit(p []byte) error {
	if w.aborted || w.err != nil {
		return ErrResponseProtection
	}
	w.Header().Del("Content-Length")
	w.Header().Set("Cache-Control", "private, no-store")
	w.ResponseWriter.WriteHeader(w.status)
	_, err := w.ResponseWriter.Write(p)
	return err
}

// Abort discards unchecked tails on cancellation; it never flushes buffered data.
func (w *ResponseWriter) Abort() {
	w.aborted = true
	w.body.Reset()
	w.stream.discard()
}

// Finish must be called after the gateway handler returns, unless canceled.
func (w *ResponseWriter) Finish() error {
	if w.finished {
		return w.err
	}
	if w.aborted {
		return ErrResponseProtection
	}
	if w.err != nil {
		return w.err
	}
	if w.prepare() != nil {
		return w.err
	}
	if w.streaming {
		if w.stream.finish() != nil {
			return w.fail()
		}
	} else if w.body.Len() > 0 {
		body := w.body.Bytes()
		object, decodeErr := DecodeObject(body)
		if decodeErr != nil || !w.boundedRestoration(object) {
			return w.fail()
		}
		out, err := w.state.RestoreJSON(body, w.protocol)
		if err != nil || len(out) > maxProtectedResponse {
			return w.fail()
		}
		if w.emit(out) != nil {
			return w.fail()
		}
	} else {
		w.ResponseWriter.WriteHeader(w.status)
		w.ResponseWriter.WriteHeaderNow()
	}
	w.body.Reset()
	w.stream.discard()
	w.finished = true
	return nil
}

func (w *ResponseWriter) fail() error {
	if w.err != nil {
		return w.err
	}
	w.err = ErrResponseProtection
	w.Header().Set("Cache-Control", "private, no-store")
	w.body.Reset()
	w.stream.discard()
	if w.aborted {
		return w.err
	}
	// Emit a fixed protocol error; never serialize an underlying parser error.
	message := `{"error":{"type":"key_protection_failed","code":"key_protection_failed","message":"Automatic key protection could not safely process the response."}}`
	if !w.ResponseWriter.Written() {
		w.Header().Del("Content-Encoding")
		w.Header().Del("Content-Length")
		w.Header().Set("Content-Type", "application/json")
		w.status = http.StatusBadGateway
		w.ResponseWriter.WriteHeader(w.status)
		_, _ = w.ResponseWriter.Write([]byte(message))
	} else if w.streaming {
		switch w.protocol {
		case "responses":
			message = "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"key_protection_failed\",\"message\":\"Automatic key protection failed.\"}}}\n\n"
		case "messages":
			message = "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"key_protection_failed\",\"message\":\"Automatic key protection failed.\"}}\n\n"
		default:
			message = "data: " + message + "\n\ndata: [DONE]\n\n"
		}
		_, _ = w.ResponseWriter.Write([]byte(message))
		w.ResponseWriter.Flush()
	}
	return w.err
}

var _ gin.ResponseWriter = (*ResponseWriter)(nil)

// Check expansion before allocating restored strings. The estimate includes
// JSON's worst-case escaping for replacements, including inner tool JSON.
func (w *ResponseWriter) boundedRestoration(value any) bool {
	budget := maxProtectedResponse
	var walk func(any) bool
	walk = func(value any) bool {
		switch value := value.(type) {
		case string:
			budget -= len(value)
			for remaining := value; len(remaining) >= PlaceholderLength; {
				at := strings.Index(remaining, "keyx_")
				if at < 0 || at+PlaceholderLength > len(remaining) {
					break
				}
				if secret, ok := w.state.entries[remaining[at:at+PlaceholderLength]]; ok {
					budget -= 12 * len(secret)
				}
				if budget < 0 {
					return false
				}
				remaining = remaining[at+PlaceholderLength:]
			}
			// Serialized tool arguments may encode the token itself with \u
			// escapes. Estimate their decoded strings before core restores them.
			trimmed := strings.TrimSpace(value)
			if len(trimmed) > 1 && (trimmed[0] == '{' || trimmed[0] == '[' || trimmed[0] == '"') && json.Valid([]byte(trimmed)) {
				inner, err := decodeJSON([]byte(trimmed))
				if err != nil || !walk(inner) {
					return false
				}
			}
		case map[string]any:
			for key, child := range value {
				budget -= len(key) + 4
				if budget < 0 || !walk(child) {
					return false
				}
			}
		case []any:
			for _, child := range value {
				budget--
				if budget < 0 || !walk(child) {
					return false
				}
			}
		}
		return budget >= 0
	}
	return walk(value)
}

const maxProtectedStreams = 4096
const maxProtectedToolBytes = 8 << 20

type sseEvent struct {
	name string
	head []string
	obj  map[string]any
}
type outputChannel struct {
	key      string
	tool     bool
	pending  string
	previous byte
	template *sseEvent
	path     []any
}
type responseStream struct {
	w              *ResponseWriter
	frame          []byte
	channels       map[string]*outputChannel
	order          []string
	toolBytes      int
	terminal       bool
	previousCR     bool
	sequenceNumber int64
	hasSequence    bool
	sources        map[string]hash.Hash
}

func newResponseStream(w *ResponseWriter) *responseStream {
	return &responseStream{w: w, channels: make(map[string]*outputChannel), sources: make(map[string]hash.Hash)}
}
func (s *responseStream) discard() {
	s.frame = nil
	s.channels = make(map[string]*outputChannel)
	s.order = nil
	s.toolBytes = 0
	s.sources = make(map[string]hash.Hash)
}
func (s *responseStream) write(p []byte) error {
	// SSE permits LF, CRLF and CR line endings, even split across network writes.
	for _, b := range p {
		if b == '\n' && s.previousCR {
			s.previousCR = false
			continue
		}
		s.previousCR = b == '\r'
		if b == '\r' {
			b = '\n'
		}
		s.frame = append(s.frame, b)
		if len(s.frame) > maxProtectedEvent {
			return ErrResponseProtection
		}
		n := len(s.frame)
		if b == '\n' && n >= 2 && s.frame[n-2] == '\n' {
			if err := s.processFrame(s.frame); err != nil {
				return err
			}
			s.frame = s.frame[:0]
		}
	}
	return nil
}
func (s *responseStream) finish() error {
	if len(bytes.TrimSpace(s.frame)) != 0 || !s.terminal {
		return ErrResponseProtection
	}
	return nil
}
func decodeObject(data []byte) (map[string]any, error) {
	obj, err := DecodeObject(data)
	if err != nil {
		return nil, ErrResponseProtection
	}
	return obj, nil
}
func (s *responseStream) processFrame(frame []byte) error {
	if !utf8.Valid(frame) {
		return ErrResponseProtection
	}
	event := &sseEvent{}
	var data []string
	for _, line := range strings.Split(strings.ReplaceAll(string(frame), "\r\n", "\n"), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ":") {
			event.head = append(event.head, line)
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "data":
			data = append(data, value)
		case "event":
			event.name = value
			event.head = append(event.head, line)
		case "id", "retry":
			event.head = append(event.head, line)
		default:
			return ErrResponseProtection
		}
	}
	if len(data) == 0 {
		if s.terminal {
			return nil
		}
		return s.w.emit([]byte(strings.Join(event.head, "\n") + "\n\n"))
	}
	joined := strings.Join(data, "\n")
	if joined == "[DONE]" {
		if s.w.protocol == "responses" && s.terminal {
			return s.w.emit([]byte("data: [DONE]\n\n"))
		}
		if s.w.protocol != "chat" {
			return ErrResponseProtection
		}
		if err := s.flushPrefix(""); err != nil {
			return err
		}
		s.terminal = true
		return s.w.emit([]byte("data: [DONE]\n\n"))
	}
	if s.terminal {
		return ErrResponseProtection
	}
	var err error
	event.obj, err = decodeObject([]byte(joined))
	if err != nil {
		return err
	}
	if !s.w.boundedRestoration(event.obj) {
		return ErrResponseProtection
	}
	if !validStreamIndices(event.obj, s.w.protocol) {
		return ErrResponseProtection
	}
	if typ := str(event.obj["type"]); s.w.protocol != "chat" && event.name != "" && typ != "" && typ != event.name {
		return ErrResponseProtection
	}
	switch s.w.protocol {
	case "chat":
		err = s.chat(event)
	case "responses":
		err = s.responses(event)
	case "messages":
		err = s.messages(event)
	default:
		return ErrResponseProtection
	}
	if err != nil {
		return err
	}
	if event.obj == nil {
		return nil
	}
	return s.emitEvent(event)
}
func str(value any) string { v, _ := value.(string); return v }
func idx(value any) string {
	if value == nil {
		return "0"
	}
	if number, ok := value.(json.Number); ok {
		return string(number)
	}
	return str(value)
}

func validStreamIndices(obj map[string]any, protocol string) bool {
	valid := func(value any) bool {
		if value == nil {
			return true
		}
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		index, err := number.Int64()
		return err == nil && index >= 0 && index <= 1<<20
	}
	for _, field := range []string{"index", "output_index", "content_index", "summary_index"} {
		if !valid(obj[field]) {
			return false
		}
	}
	if protocol == "chat" {
		if choices, ok := obj["choices"].([]any); ok {
			for _, raw := range choices {
				choice, ok := raw.(map[string]any)
				if !ok || !valid(choice["index"]) {
					return false
				}
				if delta, ok := choice["delta"].(map[string]any); ok {
					for _, field := range []string{"tool_calls", "content"} {
						if parts, ok := delta[field].([]any); ok {
							for _, rawPart := range parts {
								part, ok := rawPart.(map[string]any)
								if !ok || !valid(part["index"]) {
									return false
								}
							}
						}
					}
				}
			}
		}
	}
	return true
}
func cloneMap(obj map[string]any) map[string]any {
	data, _ := json.Marshal(obj)
	out, _ := decodeObject(data)
	return out
}
func cloneEvent(e *sseEvent) *sseEvent {
	return &sseEvent{name: e.name, head: append([]string(nil), e.head...), obj: cloneMap(e.obj)}
}
func setPath(obj map[string]any, path []any, value string) {
	var target any = obj
	for _, part := range path[:len(path)-1] {
		switch key := part.(type) {
		case string:
			target = target.(map[string]any)[key]
		case int:
			target = target.([]any)[key]
		}
	}
	target.(map[string]any)[path[len(path)-1].(string)] = value
}
func (s *responseStream) emitEvent(e *sseEvent) error {
	if s.w.protocol == "responses" {
		if original, ok := e.obj["sequence_number"].(json.Number); ok && !s.hasSequence {
			parsed, err := original.Int64()
			if err != nil || parsed < 0 {
				return ErrResponseProtection
			}
			s.sequenceNumber = parsed
			s.hasSequence = true
		}
		if s.hasSequence {
			e.obj["sequence_number"] = s.sequenceNumber
			s.sequenceNumber++
		}
	}
	data, err := json.Marshal(e.obj)
	if err != nil || len(data) > maxProtectedResponse {
		return ErrResponseProtection
	}
	var out bytes.Buffer
	for _, line := range e.head {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	out.WriteString("data: ")
	out.Write(data)
	out.WriteString("\n\n")
	return s.w.emit(out.Bytes())
}

func tokenBoundaryByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-'
}
func placeholderPrefix(text string) bool {
	const prefix = "keyx_"
	if len(text) <= len(prefix) {
		return strings.HasPrefix(prefix, text)
	}
	if !strings.HasPrefix(text, prefix) || len(text) > PlaceholderLength {
		return false
	}
	for i := len(prefix); i < len(text); i++ {
		if !(text[i] >= '0' && text[i] <= '9' || text[i] >= 'a' && text[i] <= 'f') {
			return false
		}
	}
	return true
}

// scan retains at most one complete placeholder, to inspect the right
// boundary in the next delta. It also remembers the prior source character so
// splitting an identifier cannot turn a non-token substring into a valid token.
func (s *responseStream) scan(channel *outputChannel, delta string, final bool) string {
	source := channel.pending + delta
	channel.pending = ""
	var out strings.Builder
	for i := 0; i < len(source); {
		before := channel.previous
		if i > 0 {
			before = source[i-1]
		}
		if source[i] == 'k' && !tokenBoundaryByte(before) {
			remaining := source[i:]
			if len(remaining) <= PlaceholderLength && placeholderPrefix(remaining) && !final {
				channel.pending = remaining
				if i > 0 {
					channel.previous = source[i-1]
				}
				return out.String()
			}
			if len(remaining) >= PlaceholderLength && IsPlaceholderToken(remaining[:PlaceholderLength]) && (len(remaining) == PlaceholderLength || !tokenBoundaryByte(remaining[PlaceholderLength])) {
				out.WriteString(s.w.state.RestoreText(remaining[:PlaceholderLength]))
				i += PlaceholderLength
				continue
			}
		}
		out.WriteByte(source[i])
		i++
	}
	if len(source) > 0 {
		channel.previous = source[len(source)-1]
	}
	return out.String()
}
func (s *responseStream) feed(key string, delta string, tool bool, template *sseEvent, path []any, rawTool ...bool) (string, error) {
	channel := s.channels[key]
	bufferTool := tool && !(len(rawTool) > 0 && rawTool[0])
	if channel == nil {
		if len(s.order) >= maxProtectedStreams {
			return "", ErrResponseProtection
		}
		channel = &outputChannel{key: key, tool: bufferTool}
		s.channels[key] = channel
		s.order = append(s.order, key)
	}
	channel.template = template
	channel.path = path
	delete(template.obj, "usage")
	if s.sources[key] == nil {
		s.sources[key] = sha256.New()
	}
	_, _ = s.sources[key].Write([]byte(delta))
	if bufferTool {
		if len(channel.pending)+len(delta) > maxProtectedEvent || s.toolBytes+len(delta) > maxProtectedToolBytes {
			return "", ErrResponseProtection
		}
		channel.pending += delta
		s.toolBytes += len(delta)
		return "", nil
	}
	if !s.w.boundedRestoration(channel.pending + delta) {
		return "", ErrResponseProtection
	}
	return s.scan(channel, delta, false), nil
}
func (s *responseStream) restoreArguments(arguments string) (string, error) {
	if arguments == "" {
		return "", nil
	}
	value, err := decodeJSON([]byte(arguments))
	if err != nil || !s.w.boundedRestoration(value) {
		return "", ErrResponseProtection
	}
	// Reuse the core's inner-JSON handling and exact authorization rules.
	out, err := s.w.state.RestoreArguments(arguments)
	if err != nil {
		return "", ErrResponseProtection
	}
	return out, nil
}
func (s *responseStream) flushPrefix(prefix string) error {
	for _, key := range s.order {
		channel := s.channels[key]
		if channel == nil || !strings.HasPrefix(key, prefix) {
			continue
		}
		var value string
		var err error
		if channel.tool {
			value, err = s.restoreArguments(channel.pending)
			s.toolBytes -= len(channel.pending)
		} else {
			value = s.scan(channel, "", true)
		}
		if err != nil {
			return err
		}
		if value != "" {
			setPath(channel.template.obj, channel.path, value)
			if err = s.emitEvent(channel.template); err != nil {
				return err
			}
		}
		delete(s.channels, key)
	}
	return nil
}
func (s *responseStream) restoreEnvelope(value any, protocol string) (any, error) {
	if !s.w.boundedRestoration(value) {
		return nil, ErrResponseProtection
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, ErrResponseProtection
	}
	out, err := s.w.state.RestoreJSON(encoded, protocol)
	if err != nil {
		return nil, ErrResponseProtection
	}
	return decodeObject(out)
}

func (s *responseStream) chat(event *sseEvent) error {
	if _, ok := event.obj["error"]; ok {
		s.discard()
		s.terminal = true
		return nil
	}
	choices, ok := event.obj["choices"].([]any)
	if !ok {
		return nil
	}
	var endingChoices []any
	var endingPrefixes []string
	for _, raw := range choices {
		choice, ok := raw.(map[string]any)
		if !ok {
			return ErrResponseProtection
		}
		prefix := "chat:" + idx(choice["index"]) + ":"
		delta, ok := choice["delta"].(map[string]any)
		if ok {
			for _, field := range []string{"content", "refusal", "reasoning_content", "reasoning"} {
				if text, ok := delta[field].(string); ok {
					template := cloneEvent(event)
					template.obj["choices"] = []any{map[string]any{"index": choice["index"], "delta": map[string]any{field: ""}, "finish_reason": nil}}
					out, err := s.feed(prefix+field, text, false, template, []any{"choices", 0, "delta", field})
					if err != nil {
						return err
					}
					delta[field] = out
				}
			}
			if parts, ok := delta["content"].([]any); ok {
				for position, rawPart := range parts {
					part, ok := rawPart.(map[string]any)
					if !ok {
						return ErrResponseProtection
					}
					partIndex := strconv.Itoa(position)
					if explicit, ok := part["index"]; ok {
						partIndex = idx(explicit)
					}
					for _, field := range []string{"text", "refusal"} {
						if value, ok := part[field].(string); ok {
							one := map[string]any{"type": part["type"], field: ""}
							if explicit, ok := part["index"]; ok {
								one["index"] = explicit
							}
							template := cloneEvent(event)
							template.obj["choices"] = []any{map[string]any{"index": choice["index"], "delta": map[string]any{"content": []any{one}}, "finish_reason": nil}}
							out, err := s.feed(prefix+"content:"+partIndex+":"+field, value, false, template, []any{"choices", 0, "delta", "content", 0, field})
							if err != nil {
								return err
							}
							part[field] = out
						}
					}
				}
			}
			if calls, ok := delta["tool_calls"].([]any); ok {
				for _, rawCall := range calls {
					call, ok := rawCall.(map[string]any)
					if !ok {
						return ErrResponseProtection
					}
					if custom, ok := call["custom"].(map[string]any); ok {
						if input, ok := custom["input"].(string); ok {
							template := cloneEvent(event)
							template.obj["choices"] = []any{map[string]any{"index": choice["index"], "delta": map[string]any{"tool_calls": []any{map[string]any{"index": call["index"], "custom": map[string]any{"input": ""}}}}, "finish_reason": nil}}
							out, err := s.feed(prefix+"custom:"+idx(call["index"]), input, true, template, []any{"choices", 0, "delta", "tool_calls", 0, "custom", "input"}, true)
							if err != nil {
								return err
							}
							custom["input"] = out
						}
					}
					function, ok := call["function"].(map[string]any)
					if !ok {
						continue
					}
					arguments, ok := function["arguments"].(string)
					if !ok {
						continue
					}
					template := cloneEvent(event)
					template.obj["choices"] = []any{map[string]any{"index": choice["index"], "delta": map[string]any{"tool_calls": []any{map[string]any{"index": call["index"], "function": map[string]any{"arguments": ""}}}}, "finish_reason": nil}}
					out, err := s.feed(prefix+"tool:"+idx(call["index"]), arguments, true, template, []any{"choices", 0, "delta", "tool_calls", 0, "function", "arguments"})
					if err != nil {
						return err
					}
					function["arguments"] = out
				}
			}
			if function, ok := delta["function_call"].(map[string]any); ok {
				if arguments, ok := function["arguments"].(string); ok {
					template := cloneEvent(event)
					template.obj["choices"] = []any{map[string]any{"index": choice["index"], "delta": map[string]any{"function_call": map[string]any{"arguments": ""}}, "finish_reason": nil}}
					out, err := s.feed(prefix+"function", arguments, true, template, []any{"choices", 0, "delta", "function_call", "arguments"})
					if err != nil {
						return err
					}
					function["arguments"] = out
				}
			}
		}
		if choice["finish_reason"] != nil {
			endingChoices = append(endingChoices, map[string]any{"index": choice["index"], "delta": map[string]any{}, "finish_reason": choice["finish_reason"]})
			endingPrefixes = append(endingPrefixes, prefix)
			choice["finish_reason"] = nil
		}
	}
	if len(endingChoices) > 0 {
		// A terminal chunk can itself contain text. Emit that text before its
		// buffered suffix, and expose finish_reason only after both are written.
		if err := s.emitEvent(event); err != nil {
			return err
		}
		for _, prefix := range endingPrefixes {
			if err := s.flushPrefix(prefix); err != nil {
				return err
			}
		}
		ending := cloneEvent(event)
		ending.obj["choices"] = endingChoices
		delete(ending.obj, "usage")
		if err := s.emitEvent(ending); err != nil {
			return err
		}
		event.obj = nil
	}
	return nil
}

func (s *responseStream) responses(event *sseEvent) error {
	typ := str(event.obj["type"])
	if typ == "" {
		typ = event.name
	}
	prefix := "responses:" + idx(event.obj["output_index"]) + ":"
	contentKey := prefix + idx(event.obj["content_index"]) + ":"
	if summaryIndex, ok := event.obj["summary_index"]; ok {
		contentKey = prefix + "summary:" + idx(summaryIndex) + ":"
	}
	switch typ {
	case "response.output_text.delta", "response.refusal.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		text, ok := event.obj["delta"].(string)
		if !ok {
			return ErrResponseProtection
		}
		key := contentKey + strings.TrimSuffix(typ, ".delta")
		out, err := s.feed(key, text, false, cloneEvent(event), []any{"delta"})
		if err != nil {
			return err
		}
		event.obj["delta"] = out
	case "response.function_call_arguments.delta":
		text, ok := event.obj["delta"].(string)
		if !ok {
			return ErrResponseProtection
		}
		out, err := s.feed(prefix+"tool", text, true, cloneEvent(event), []any{"delta"})
		if err != nil {
			return err
		}
		event.obj["delta"] = out
	case "response.custom_tool_call_input.delta":
		text, ok := event.obj["delta"].(string)
		if !ok {
			return ErrResponseProtection
		}
		out, err := s.feed(prefix+"custom_tool", text, true, cloneEvent(event), []any{"delta"}, true)
		if err != nil {
			return err
		}
		event.obj["delta"] = out
	case "response.custom_tool_call_input.done":
		if !s.sameSource(prefix+"custom_tool", str(event.obj["input"])) {
			return ErrResponseProtection
		}
		if err := s.flushPrefix(prefix + "custom_tool"); err != nil {
			return err
		}
		text, ok := event.obj["input"].(string)
		if !ok {
			return ErrResponseProtection
		}
		event.obj["input"] = s.w.state.RestoreText(text)
	case "response.function_call_arguments.done":
		if !s.sameSource(prefix+"tool", str(event.obj["arguments"])) {
			return ErrResponseProtection
		}
		if err := s.flushPrefix(prefix + "tool"); err != nil {
			return err
		}
		out, err := s.restoreArguments(str(event.obj["arguments"]))
		if err != nil {
			return err
		}
		event.obj["arguments"] = out
	case "response.output_text.done", "response.refusal.done", "response.reasoning_summary_text.done", "response.reasoning_text.done":
		field := "text"
		if typ == "response.refusal.done" {
			field = "refusal"
		}
		if !s.sameSource(contentKey+strings.TrimSuffix(typ, ".done"), str(event.obj[field])) {
			return ErrResponseProtection
		}
		if err := s.flushPrefix(contentKey + strings.TrimSuffix(typ, ".done")); err != nil {
			return err
		}
		for _, field := range []string{"text", "refusal"} {
			if text, ok := event.obj[field].(string); ok {
				event.obj[field] = s.w.state.RestoreText(text)
			}
		}
	case "response.output_item.done", "response.output_item.added":
		if strings.HasSuffix(typ, ".done") {
			if item, ok := event.obj["item"].(map[string]any); ok && !s.sameItem(idx(event.obj["output_index"]), item) {
				return ErrResponseProtection
			}
		}
		if strings.HasSuffix(typ, ".done") {
			if err := s.flushPrefix(prefix); err != nil {
				return err
			}
		}
		if item, ok := event.obj["item"].(map[string]any); ok {
			if typ == "response.output_item.added" && str(item["type"]) == "function_call" && str(item["arguments"]) == "" {
				return nil
			}
			out, err := s.restoreEnvelope(map[string]any{"output": []any{item}}, "responses")
			if err != nil {
				return err
			}
			event.obj["item"] = out.(map[string]any)["output"].([]any)[0]
		}
	case "response.content_part.done", "response.reasoning_summary_part.done":
		if part, ok := event.obj["part"].(map[string]any); ok {
			kind := "response.output_text"
			field := "text"
			if strings.Contains(typ, "reasoning_summary_part") {
				kind = "response.reasoning_summary_text"
			} else if str(part["type"]) == "refusal" {
				kind = "response.refusal"
				field = "refusal"
			}
			if !s.sameSource(contentKey+kind, str(part[field])) {
				return ErrResponseProtection
			}
		}
		if err := s.flushPrefix(contentKey); err != nil {
			return err
		}
		fallthrough
	case "response.content_part.added", "response.reasoning_summary_part.added":
		if part, ok := event.obj["part"].(map[string]any); ok {
			for _, field := range []string{"text", "refusal"} {
				if text, ok := part[field].(string); ok {
					if strings.HasSuffix(typ, ".added") {
						deltaType := "response.output_text.delta"
						if strings.Contains(typ, "reasoning_summary_part") {
							deltaType = "response.reasoning_summary_text.delta"
						}
						if field == "refusal" {
							deltaType = "response.refusal.delta"
						}
						template := cloneEvent(event)
						template.name = deltaType
						template.head = []string{"event: " + deltaType}
						delete(template.obj, "part")
						template.obj["type"] = deltaType
						template.obj["delta"] = ""
						out, err := s.feed(contentKey+strings.TrimSuffix(deltaType, ".delta"), text, false, template, []any{"delta"})
						if err != nil {
							return err
						}
						part[field] = out
					} else {
						part[field] = s.w.state.RestoreText(text)
					}
				}
			}
		}
	case "response.completed", "response.incomplete":
		if response, ok := event.obj["response"].(map[string]any); ok {
			if output, ok := response["output"].([]any); ok {
				for index, raw := range output {
					if item, ok := raw.(map[string]any); ok && !s.sameItem(strconv.Itoa(index), item) {
						return ErrResponseProtection
					}
				}
			}
		}
		if err := s.flushPrefix(""); err != nil {
			return err
		}
		if response, ok := event.obj["response"].(map[string]any); ok {
			out, err := s.restoreEnvelope(response, "responses")
			if err != nil {
				return err
			}
			event.obj["response"] = out
		}
		s.terminal = true
	case "response.failed", "error":
		s.discard()
		s.terminal = true
	case "response.created", "response.in_progress":
		if response, ok := event.obj["response"].(map[string]any); ok {
			out, err := s.restoreEnvelope(response, "responses")
			if err != nil {
				return err
			}
			event.obj["response"] = out
		}
	default:
		if strings.HasSuffix(typ, ".delta") && mappedOutput(s.w.state, event.obj) {
			return ErrResponseProtection
		}
	}
	return nil
}

func mappedOutput(state *State, value any) bool {
	switch value := value.(type) {
	case string:
		return state.RestoreText(value) != value
	case map[string]any:
		for _, child := range value {
			if mappedOutput(state, child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if mappedOutput(state, child) {
				return true
			}
		}
	}
	return false
}

func (s *responseStream) sameSource(key, text string) bool {
	previous := s.sources[key]
	if previous == nil {
		return true
	}
	current := sha256.Sum256([]byte(text))
	return bytes.Equal(previous.Sum(nil), current[:])
}
func (s *responseStream) sameItem(index string, item map[string]any) bool {
	prefix := "responses:" + index + ":"
	switch str(item["type"]) {
	case "function_call":
		return s.sameSource(prefix+"tool", str(item["arguments"]))
	case "custom_tool_call":
		return s.sameSource(prefix+"custom_tool", str(item["input"]))
	case "message", "reasoning":
		for _, field := range []string{"content", "summary"} {
			if parts, ok := item[field].([]any); ok {
				for index, raw := range parts {
					if part, ok := raw.(map[string]any); ok {
						kind := "response.output_text"
						textField := "text"
						partPrefix := prefix + strconv.Itoa(index) + ":"
						if field == "summary" {
							partPrefix = prefix + "summary:" + strconv.Itoa(index) + ":"
							kind = "response.reasoning_summary_text"
						}
						if str(part["type"]) == "refusal" {
							kind = "response.refusal"
							textField = "refusal"
						}
						if !s.sameSource(partPrefix+kind, str(part[textField])) {
							return false
						}
					}
				}
			}
		}
	}
	return true
}

func (s *responseStream) messages(event *sseEvent) error {
	typ := str(event.obj["type"])
	if typ == "" {
		typ = event.name
	}
	prefix := "messages:" + idx(event.obj["index"]) + ":"
	switch typ {
	case "message_start":
		if message, ok := event.obj["message"].(map[string]any); ok {
			out, err := s.restoreEnvelope(message, "messages")
			if err != nil {
				return err
			}
			event.obj["message"] = out
		}
	case "content_block_start":
		if block, ok := event.obj["content_block"].(map[string]any); ok {
			if str(block["type"]) == "text" {
				template := &sseEvent{name: "content_block_delta", head: []string{"event: content_block_delta"}, obj: map[string]any{"type": "content_block_delta", "index": event.obj["index"], "delta": map[string]any{"type": "text_delta", "text": ""}}}
				out, err := s.feed(prefix+"text", str(block["text"]), false, template, []any{"delta", "text"})
				if err != nil {
					return err
				}
				block["text"] = out
				return nil
			}
			out, err := s.restoreEnvelope(map[string]any{"content": []any{block}}, "messages")
			if err != nil {
				return err
			}
			event.obj["content_block"] = out.(map[string]any)["content"].([]any)[0]
		}
	case "content_block_delta":
		delta, ok := event.obj["delta"].(map[string]any)
		if !ok {
			return ErrResponseProtection
		}
		switch str(delta["type"]) {
		case "text_delta":
			out, err := s.feed(prefix+"text", str(delta["text"]), false, cloneEvent(event), []any{"delta", "text"})
			if err != nil {
				return err
			}
			delta["text"] = out
		case "input_json_delta":
			out, err := s.feed(prefix+"tool", str(delta["partial_json"]), true, cloneEvent(event), []any{"delta", "partial_json"})
			if err != nil {
				return err
			}
			delta["partial_json"] = out
		}
	case "content_block_stop":
		return s.flushPrefix(prefix)
	case "message_stop":
		if err := s.flushPrefix(""); err != nil {
			return err
		}
		s.terminal = true
	case "error":
		s.discard()
		s.terminal = true
	}
	return nil
}

// String intentionally excludes source fragments and authorized mapping values.
func (s *responseStream) String() string {
	return fmt.Sprintf("key protection stream (%s, %s channels)", s.w.protocol, strconv.Itoa(len(s.channels)))
}

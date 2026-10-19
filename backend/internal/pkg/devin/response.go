// Adapted from Oh My Pi (MIT), commit 4049c62fbb0a7f5068954d01f11a73112c942a11,
// packages/ai/src/providers/devin.ts (stream and response mapping).
// See the repository root THIRD_PARTY_NOTICES.md for license and attribution.
package devin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// Chat always invokes the upstream Connect streaming RPC, translating it into
// CC JSON or SSE. The caller must close the returned body, even after read errors.
// The first Connect message is read before returning so immediate trailers are
// surfaced as typed HTTP errors. Later errors are returned by Body.Read.
func (c *Client) Chat(ctx context.Context, token, cascadeID string, model Model, body []byte) (*http.Response, error) {
	if err := checkToken(token); err != nil {
		return nil, err
	}
	if cascadeID == "" {
		cascadeID = randomID()
	}
	if len(cascadeID) > 1024 {
		return nil, inputError("Cascade ID is too long")
	}
	uid := model.UID
	if uid == "" {
		uid = model.ID
	}
	if strings.TrimSpace(uid) == "" {
		return nil, inputError("A Devin model is required")
	}
	if isFusionModelID(model.ID) || isFusionModelID(uid) {
		return nil, inputError("Devin Fusion lead/sidekick orchestration is not supported by this adapter; select a standalone model")
	}
	mapped, err := mapRequest(body, cascadeID, model)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	keep := false
	defer func() {
		if !keep {
			cancel()
		}
	}()
	jwt, base, err := c.auth(ctx, token)
	if err != nil {
		return nil, err
	}
	if model.Router || uid == "adaptive" {
		if mapped.routerPrompt == nil {
			return nil, inputError("Router models require a user or developer message")
		}
		request := pb(nil, 1, metadata(token, "", false))
		request = ps(request, 2, uid)
		request = ps(request, 3, cascadeID)
		request = pb(request, 5, mapped.routerPrompt)
		response, e := c.unary(ctx, base, assignPath, request)
		if e != nil {
			return nil, e
		}
		if e = response.check([]int32{1}, nil); e != nil {
			return nil, e
		}
		assignment, e := parseProto(response.data(1))
		if e != nil {
			return nil, e
		}
		if e = assignment.check([]int32{1, 2}, nil); e != nil {
			return nil, e
		}
		assigned, assignmentJWT := assignment.text(2), assignment.text(1)
		if strings.TrimSpace(assigned) == "" || strings.TrimSpace(assignmentJWT) == "" || assigned == uid {
			return nil, &Error{502, "invalid_assignment", "Devin router returned no usable assignment"}
		}
		uid = assigned
		mapped.wire = ps(mapped.wire, 26, assignmentJWT)
	}
	wire := pb(mapped.wire, 1, metadata(token, jwt, false))
	wire = ps(wire, 21, uid)
	frame, err := gzipFrame(wire)
	if err != nil {
		return nil, err
	}
	response, err := c.request(ctx, base, ChatPath, frame, true)
	if err != nil {
		return nil, err
	}
	owned := &ownedBody{ReadCloser: response.Body, ctx: ctx, cancel: cancel}
	// The callback only closes the transport, avoiding a race with initialization
	// of stop if the context was canceled before registration returned.
	owned.stop = context.AfterFunc(ctx, func() { owned.closeUpstream() })
	cr := &connectReader{r: owned}
	first, err := cr.next()
	if err != nil && err != io.EOF {
		contextErr := ctx.Err()
		owned.Close()
		if contextErr != nil {
			return nil, contextErr
		}
		return nil, err
	}
	state := newCompletion(uid)
	state.streaming = mapped.req.Stream
	if first != nil {
		if _, err = state.apply(first); err != nil {
			owned.Close()
			return nil, err
		}
	}
	headers := make(http.Header)
	if !mapped.req.Stream {
		defer owned.Close()
		for !cr.terminal {
			message, e := cr.next()
			if e == io.EOF {
				break
			}
			if e != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, e
			}
			if _, e = state.apply(message); e != nil {
				return nil, e
			}
		}
		if err = state.validate(); err != nil {
			return nil, err
		}
		payload, err := json.Marshal(state.response())
		if err != nil {
			return nil, protocolError()
		}
		headers.Set("Content-Type", "application/json")
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: headers, Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: int64(len(payload))}, nil
	}
	// apply was used to validate the prefetch; reset before emitting its deltas.
	state = newCompletion(uid)
	state.streaming = true
	stream := &completionReader{upstream: owned, connect: cr, state: state, first: first}
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	keep = true
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: headers, Body: stream, ContentLength: -1}, nil
}

const (
	maxCompletionBytes   = 32 << 20
	maxToolArgumentBytes = 4 << 20
	maxCompletionTools   = 1024
	maxToolIdentityBytes = 1024
)

type completion struct {
	id, model                 string
	idObserved, modelObserved bool
	idFrozen, streaming       bool
	outputBytes               int
	created                   int64
	text, thinking            strings.Builder
	tools                     []apicompat.ChatToolCall
	toolIndex                 map[string]int
	active                    string
	usage                     *apicompat.ChatUsage
	stop                      uint64
}

func newCompletion(model string) *completion {
	return &completion{id: "chatcmpl-" + randomID(), model: model, created: time.Now().Unix(), toolIndex: map[string]int{}}
}
func fragment(previous, incoming string) (delta, accumulated string) {
	if strings.HasPrefix(incoming, previous) {
		return incoming[len(previous):], incoming
	}
	return incoming, previous + incoming
}
func (s *completion) accountBytes(n int) error {
	if n < 0 || n > maxCompletionBytes-s.outputBytes {
		return &Error{502, "response_too_large", "Devin completion exceeds the output limit"}
	}
	s.outputBytes += n
	return nil
}

// Bound both retained state and transient concatenations before allocating.
func (s *completion) toolFragment(previous, incoming string, limit int) (string, string, error) {
	snapshot := strings.HasPrefix(incoming, previous)
	size := len(incoming)
	if !snapshot {
		size += len(previous)
	}
	if size > limit {
		return "", "", &Error{502, "response_too_large", "Devin tool output exceeds the limit"}
	}
	if err := s.accountBytes(size - len(previous)); err != nil {
		return "", "", err
	}
	delta, accumulated := fragment(previous, incoming)
	return delta, accumulated, nil
}

func (s *completion) apply(m protoMessage) (apicompat.ChatDelta, error) {
	var delta apicompat.ChatDelta
	if err := m.check([]int32{1, 3, 6, 7, 9, 23}, []int32{5, 8, 11}); err != nil {
		return delta, err
	}
	if m.num(8) != 0 || m.num(11) != 0 {
		return delta, &Error{502, "unsupported_response", "Devin returned redacted content"}
	}
	if id := m.text(1); id != "" {
		if !s.idFrozen {
			s.id = id
		}
		s.idObserved = true
	}
	if model := m.text(23); model != "" {
		s.model = model
		s.modelObserved = true
	}
	if text := m.text(3); text != "" {
		if err := s.accountBytes(len(text)); err != nil {
			return delta, err
		}
		if !s.streaming {
			s.text.WriteString(text)
		}
		delta.Content = &text
	}
	if thinking := m.text(9); thinking != "" {
		if err := s.accountBytes(len(thinking)); err != nil {
			return delta, err
		}
		if !s.streaming {
			s.thinking.WriteString(thinking)
		}
		delta.ReasoningContent = &thinking
	}
	for _, field := range m[6] {
		tc, err := parseProto(field.b)
		if err != nil {
			return delta, err
		}
		if err = tc.check([]int32{1, 2, 3, 4, 5}, []int32{6}); err != nil {
			return delta, err
		}
		if tc.text(4) != "" || tc.text(5) != "" || tc.num(6) != 0 {
			return delta, protocolError()
		}
		id := tc.text(1)
		if id == "" {
			id = s.active
		}
		if id == "" {
			return delta, protocolError()
		}
		index, exists := s.toolIndex[id]
		out := apicompat.ChatToolCall{}
		if !exists {
			if len(s.tools) >= maxCompletionTools || len(id) > maxToolIdentityBytes {
				return delta, &Error{502, "response_too_large", "Devin tool output exceeds the limit"}
			}
			if err := s.accountBytes(len(id)); err != nil {
				return delta, err
			}
			index = len(s.tools)
			s.toolIndex[id] = index
			s.tools = append(s.tools, apicompat.ChatToolCall{ID: id, Type: "function"})
			out.ID = id
			out.Type = "function"
		}
		s.active = id
		out.Index = &index
		tool := &s.tools[index]
		if name := tc.text(2); name != "" {
			var err error
			out.Function.Name, tool.Function.Name, err = s.toolFragment(tool.Function.Name, name, maxToolIdentityBytes)
			if err != nil {
				return delta, err
			}
		}
		if args := tc.text(3); args != "" {
			var err error
			out.Function.Arguments, tool.Function.Arguments, err = s.toolFragment(tool.Function.Arguments, args, maxToolArgumentBytes)
			if err != nil {
				return delta, err
			}
		}
		if !exists || out.Function.Name != "" || out.Function.Arguments != "" {
			delta.ToolCalls = append(delta.ToolCalls, out)
		}
	}
	if stop := m.num(5); stop != 0 {
		if stop == 13 || stop > 13 {
			return delta, &Error{502, "generation_failed", "Devin generation failed"}
		}
		s.stop = stop
	}
	if len(m[7]) > 0 {
		usage, err := parseProto(m.data(7))
		if err != nil {
			return delta, err
		}
		if err = usage.check([]int32{7, 9}, []int32{2, 3, 4, 5}); err != nil {
			return delta, err
		}
		input, output, write, read := usage.num(2), usage.num(3), usage.num(4), usage.num(5)
		// Validate before addition or conversion to machine-sized CC integers.
		cap := uint64(^uint(0) >> 1)
		total := uint64(0)
		for _, n := range []uint64{input, output, write, read} {
			if n > cap-total {
				return delta, protocolError()
			}
			total += n
		}
		s.usage = &apicompat.ChatUsage{PromptTokens: int(input + write + read), CompletionTokens: int(output), TotalTokens: int(total),
			PromptTokensDetails: &apicompat.ChatTokenDetails{CachedTokens: int(read), CacheCreationTokens: int(write)}}
		if id := usage.text(7); id != "" && !s.idObserved && !s.idFrozen {
			s.id = id
		}
		if model := usage.text(9); model != "" && !s.modelObserved {
			s.model = model
		}
	}
	return delta, nil
}
func (s *completion) validate() error {
	for _, tool := range s.tools {
		if tool.Function.Name == "" || !json.Valid([]byte(tool.Function.Arguments)) {
			return protocolError()
		}
	}
	return nil
}
func (s *completion) finish() string {
	if s.stop == 11 {
		return "content_filter"
	}
	if len(s.tools) > 0 || s.stop == 10 {
		return "tool_calls"
	}
	if s.stop == 3 || s.stop == 5 {
		return "length"
	}
	return "stop"
}
func (s *completion) response() apicompat.ChatCompletionsResponse {
	text, _ := json.Marshal(s.text.String())
	return apicompat.ChatCompletionsResponse{ID: s.id, Object: "chat.completion", Created: s.created, Model: s.model, Usage: s.usage,
		Choices: []apicompat.ChatChoice{{Index: 0, Message: apicompat.ChatMessage{Role: "assistant", Content: text, ReasoningContent: s.thinking.String(), ToolCalls: s.tools}, FinishReason: s.finish()}}}
}
func (s *completion) chunk(delta apicompat.ChatDelta, finish *string, usage *apicompat.ChatUsage) apicompat.ChatCompletionsChunk {
	s.idFrozen = true
	return apicompat.ChatCompletionsChunk{ID: s.id, Object: "chat.completion.chunk", Created: s.created, Model: s.model, Usage: usage,
		Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: delta, FinishReason: finish}}}
}

type completionReader struct {
	upstream      *ownedBody
	connect       *connectReader
	state         *completion
	first         protoMessage
	pending       []byte
	started, done bool
	failure       error
}

func (r *completionReader) Close() error { return r.upstream.Close() }
func sse(value any) []byte {
	b, _ := json.Marshal(value)
	return append(append([]byte("data: "), b...), []byte("\n\n")...)
}
func (r *completionReader) advance() error {
	var message protoMessage
	if r.first != nil {
		message = r.first
		r.first = nil
	} else if !r.connect.terminal {
		var err error
		message, err = r.connect.next()
		if err != nil && err != io.EOF {
			if r.upstream.ctx.Err() != nil {
				return r.upstream.ctx.Err()
			}
			return err
		}
	}
	if message != nil {
		delta, err := r.state.apply(message)
		if err != nil {
			return err
		}
		if !r.started {
			delta.Role = "assistant"
			r.started = true
		}
		// Empty upstream messages can carry authoritative ID/model observations.
		r.pending = sse(r.state.chunk(delta, nil, nil))
		return nil
	}
	if err := r.state.validate(); err != nil {
		return err
	}
	if !r.started {
		r.pending = append(r.pending, sse(r.state.chunk(apicompat.ChatDelta{Role: "assistant"}, nil, nil))...)
		r.started = true
	}
	finish := r.state.finish()
	r.pending = append(r.pending, sse(r.state.chunk(apicompat.ChatDelta{}, &finish, r.state.usage))...)
	r.pending = append(r.pending, []byte("data: [DONE]\n\n")...)
	r.done = true
	r.upstream.Close()
	return nil
}
func (r *completionReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.failure != nil {
		return 0, r.failure
	}
	if !r.done && r.upstream.ctx.Err() != nil {
		r.failure = r.upstream.ctx.Err()
		r.upstream.Close()
		return 0, r.failure
	}
	for len(r.pending) == 0 {
		if r.done {
			return 0, io.EOF
		}
		if err := r.advance(); err != nil {
			r.failure = err
			r.upstream.Close()
			return 0, err
		}
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

//go:build unit

package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func feedThreadSSE(t *testing.T, parts ...string) ([]threadSSEEvent, *threadSSEDecoder) {
	t.Helper()
	decoder := newThreadSSEDecoder(1<<20, 1024)
	var events []threadSSEEvent
	for _, part := range parts {
		var err error
		events, err = decoder.Feed([]byte(part), events)
		require.NoError(t, err)
	}
	return events, decoder
}

// The decoder against the rules of the format, each written out by hand.
func TestThreadSSEDecoder(t *testing.T) {
	named := func(name, data string) threadSSEEvent { return threadSSEEvent{Name: name, Named: true, Data: data} }
	for name, tc := range map[string]struct {
		stream string
		want   []threadSSEEvent
	}{
		"one event":               {"event: a\ndata: 1\n\n", []threadSSEEvent{named("a", "1")}},
		"crlf":                    {"event: a\r\ndata: 1\r\n\r\n", []threadSSEEvent{named("a", "1")}},
		"cr":                      {"event: a\rdata: 1\r\r", []threadSSEEvent{named("a", "1")}},
		"mixed terminators":       {"event: a\rdata: 1\r\ndata: 2\n\r\n", []threadSSEEvent{named("a", "1\n2")}},
		"data lines are joined":   {"data: {\ndata: \"k\":1\ndata: }\n\n", []threadSSEEvent{{Data: "{\n\"k\":1\n}"}}},
		"no event field":          {"data: 1\n\n", []threadSSEEvent{{Data: "1"}}},
		"empty event field":       {"event:\ndata: 1\n\n", []threadSSEEvent{named("", "1")}},
		"last event field wins":   {"event: a\nevent: b\ndata: 1\n\n", []threadSSEEvent{named("b", "1")}},
		"no space after colon":    {"event:a\ndata:1\n\n", []threadSSEEvent{named("a", "1")}},
		"one space is removed":    {"data:  1 \n\n", []threadSSEEvent{{Data: " 1 "}}},
		"empty data line":         {"data\ndata:\ndata: x\n\n", []threadSSEEvent{{Data: "\n\nx"}}},
		"data that is empty":      {"data:\n\n", []threadSSEEvent{{Data: ""}}},
		"comment":                 {": hello\ndata: 1\n:data: 2\n\n", []threadSSEEvent{{Data: "1"}}},
		"comment only":            {": hello\n\n", nil},
		"event without data":      {"event: a\n\ndata: 1\n\n", []threadSSEEvent{{Data: "1"}}},
		"other fields":            {"id: 3\nretry: 5\nfuture: x\nnocolon\ndata: 1\n\n", []threadSSEEvent{{Data: "1"}}},
		"field names are exact":   {"Data: 0\n data: 0\ndata : 0\nEVENT: x\ndata: 1\n\n", []threadSSEEvent{{Data: "1"}}},
		"blank lines in a row":    {"\n\n\ndata: 1\n\n\n\n", []threadSSEEvent{{Data: "1"}}},
		"a space is not blank":    {"data: 1\n \ndata: 2\n\n", []threadSSEEvent{{Data: "1\n2"}}},
		"a tab is not blank":      {"data: 1\n\t\n", nil},
		"unfinished at the end":   {"data: 1\n\ndata: 2\n", []threadSSEEvent{{Data: "1"}}},
		"unfinished line":         {"data: 1\n\ndata: 2", []threadSSEEvent{{Data: "1"}}},
		"only a space line ends":  {"event: message_stop\ndata: {}\n \n", nil},
		"byte order mark":         {"\xEF\xBB\xBFdata: 1\n\n", []threadSSEEvent{{Data: "1"}}},
		"mark only at the start":  {"data: 1\n\n\xEF\xBB\xBFdata: 2\n\n", []threadSSEEvent{{Data: "1"}}},
		"name does not carry on":  {"event: a\ndata: 1\n\ndata: 2\n\n", []threadSSEEvent{named("a", "1"), {Data: "2"}}},
		"colon in the value":      {"data: a: b\n\n", []threadSSEEvent{{Data: "a: b"}}},
		"cr then lf across lines": {"data: 1\r\n\r\ndata: 2\r\r\n", []threadSSEEvent{{Data: "1"}, {Data: "2"}}},
	} {
		whole, _ := feedThreadSSE(t, tc.stream)
		require.Equal(t, tc.want, whole, name)
		// However the bytes are cut up, the events are the same.
		for cut := 1; cut < len(tc.stream); cut++ {
			got, _ := feedThreadSSE(t, tc.stream[:cut], tc.stream[cut:])
			require.Equal(t, tc.want, got, "%s cut at %d", name, cut)
		}
		got, _ := feedThreadSSE(t, strings.Split(tc.stream, "")...)
		require.Equal(t, tc.want, got, "%s byte by byte", name)
	}

	// What the stream stopped in is not an event; its data is kept apart.
	_, decoder := feedThreadSSE(t, "data: 1\n\nevent: message_delta\ndata: {\"a\":\ndata: 2}")
	require.Equal(t, "{\"a\":\n2}", decoder.unfinishedData())
	_, decoder = feedThreadSSE(t, "data: 1\n\n")
	require.Empty(t, decoder.unfinishedData())

	// One event is bounded in bytes and in lines; a blank line starts over.
	decoder = newThreadSSEDecoder(32, 4)
	_, err := decoder.Feed([]byte("data: 1234567890\n\ndata: 1234567890\n\n: comment\n\n"), nil)
	require.NoError(t, err)
	_, err = decoder.Feed([]byte("data: 1234567890\ndata: 1234567890\n"), nil)
	require.ErrorIs(t, err, errThreadSSEEventTooLarge)
	require.ErrorIs(t, err, bufio.ErrTooLong)
	decoder = newThreadSSEDecoder(32, 4)
	_, err = decoder.Feed([]byte(strings.Repeat("x", 33)), nil)
	require.ErrorIs(t, err, errThreadSSEEventTooLarge, "a line that never ends")
	decoder = newThreadSSEDecoder(1024, 4)
	_, err = decoder.Feed([]byte(":\n:\n:\n:\n"), nil)
	require.NoError(t, err)
	_, err = decoder.Feed([]byte(":\n"), nil)
	require.ErrorIs(t, err, errThreadSSEEventTooLarge, "an event that never ends")
}

type threadWireEvent struct{ name, data string }

// threadWire writes events as a stream. split puts each top-level member of a
// JSON payload on a data line of its own.
func threadWire(events []threadWireEvent, newline string, split bool) string {
	var b strings.Builder
	for _, event := range events {
		if event.name != "" {
			b.WriteString("event: " + event.name + newline)
		}
		lines := []string{event.data}
		if split {
			lines = threadSplitJSON(event.data)
		}
		for _, line := range lines {
			b.WriteString("data: " + line + newline)
		}
		b.WriteString(newline)
	}
	return b.String()
}

// threadSplitJSON breaks a JSON object where JSON allows a line break: after
// its opening brace and after each of its top-level commas.
func threadSplitJSON(data string) []string {
	var lines []string
	depth, inString, escaped, start := 0, false, false, 0
	for i := 0; i < len(data); i++ {
		c := data[i]
		switch {
		case escaped:
			escaped = false
		case inString:
			escaped = c == '\\'
			inString = c != '"'
		case c == '"':
			inString = true
		case c == '{' || c == '[':
			depth++
			if depth == 1 {
				lines, start = append(lines, data[start:i+1]), i+1
			}
		case c == '}' || c == ']':
			depth--
		case c == ',' && depth == 1:
			lines, start = append(lines, data[start:i+1]), i+1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// threadTestStream is one complete Messages stream: 100 input, 900 cache read,
// 50 cache creation and, cumulatively, 20 output tokens.
func threadTestStream(stopReason, block string) []threadWireEvent {
	return []threadWireEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_new","type":"message","model":"claude-sonnet-4-5","usage":{"input_tokens":100,"cache_read_input_tokens":900,"cache_creation_input_tokens":50,"output_tokens":3}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"` + block + `"}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"x"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"ping", `{"type":"ping"}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"` + stopReason + `"},"usage":{"output_tokens":20}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}
}

// threadSlicedReader hands out a stream in the given pieces, one per Read.
type threadSlicedReader struct {
	parts []string
	err   error
}

func (r *threadSlicedReader) Read(p []byte) (int, error) {
	for len(r.parts) > 0 && r.parts[0] == "" {
		r.parts = r.parts[1:]
	}
	if len(r.parts) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		return 0, io.EOF
	}
	n := copy(p, r.parts[0])
	r.parts[0] = r.parts[0][n:]
	return n, nil
}

// threadStreamClient records every write to the client together with the
// number of bindings stored at that moment.
type threadStreamClient struct {
	*httptest.ResponseRecorder
	store  *threadStoreStub
	binds  []int
	failAt int
}

func (w *threadStreamClient) Write(p []byte) (int, error) {
	w.binds = append(w.binds, w.store.binds)
	if w.failAt > 0 && len(w.binds) >= w.failAt {
		return 0, errors.New("client connection closed")
	}
	return w.ResponseRecorder.Write(p)
}

func (w *threadStreamClient) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

type threadStreamRun struct {
	opts    threadStreamOptions
	account *Account
	prepare func(*MessageThread, *gin.Context)
	ctx     context.Context
	failAt  int
	cfg     *config.Config
}

type threadStreamOutcome struct {
	result *streamingResult
	err    error
	body   string
	store  *threadStoreStub
	client *threadStreamClient
}

func (o threadStreamOutcome) usage() [4]int {
	if o.result == nil {
		return [4]int{}
	}
	u := o.result.usage
	return [4]int{u.InputTokens, u.CacheReadInputTokens, u.CacheCreationInputTokens, u.OutputTokens}
}

func runThreadStream(t *testing.T, body io.Reader, run threadStreamRun) threadStreamOutcome {
	t.Helper()
	store := &threadStoreStub{}
	thread := &MessageThread{store: store, scope: "1:2:3"}
	thread.attempt.sealed, thread.attempt.next = true, threadTestBinding()
	thread.attempt.next.ContextTokens = 0
	client := &threadStreamClient{ResponseRecorder: httptest.NewRecorder(), store: store, failAt: run.failAt}
	c, _ := gin.CreateTestContext(client)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if run.ctx != nil {
		c.Request = c.Request.WithContext(run.ctx)
	}
	if run.prepare != nil {
		run.prepare(thread, c)
	}
	account := run.account
	if account == nil {
		account = &Account{ID: 7, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k"}}
	}
	cfg := run.cfg
	if cfg == nil {
		cfg = &config.Config{}
	}
	svc := &GatewayService{cfg: cfg}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(body)}
	result, err := svc.streamMessageThreadResponse(thread.Context(context.Background()), thread, resp, c, account, time.Now(), run.opts)
	return threadStreamOutcome{result: result, err: err, body: client.Body.String(), store: store, client: client}
}

// One complete stream: billed as metered, bound before its end is written,
// and relayed event for event.
func TestMessageThreadStreamComplete(t *testing.T) {
	for _, reason := range []string{"end_turn", "tool_use", "pause_turn", "stop_sequence", "max_tokens"} {
		for _, passthrough := range []bool{false, true} {
			stream := threadWire(threadTestStream(reason, "text"), "\n", false)
			out := runThreadStream(t, strings.NewReader(stream), threadStreamRun{opts: threadStreamOptions{passthrough: passthrough}})
			require.NoError(t, out.err, reason)
			require.Equal(t, [4]int{100, 900, 50, 20}, out.usage())
			require.Equal(t, 1, out.store.binds)
			require.Equal(t, 1070, out.store.bindings["1:2:3/msg_new"].ContextTokens)
			require.Equal(t, stream, out.body, "an unchanged stream in its plain form is relayed as it is")
			// Seven events, seven writes; the binding exists exactly from the
			// write that carries message_stop on.
			require.Equal(t, []int{0, 0, 0, 0, 0, 0, 1}, out.client.binds)
		}
	}
}

// A stream is a complete response that can be continued only if every event a
// binding depends on is named as its payload type, parses, and comes in the
// order of a Messages stream with each block closed under its own index.
// Whatever the stream is, what it meters is billed and its events are relayed.
func TestMessageThreadStreamNotContinuable(t *testing.T) {
	remove := func(i int) func([]threadWireEvent) []threadWireEvent {
		return func(e []threadWireEvent) []threadWireEvent { return append(e[:i:i], e[i+1:]...) }
	}
	set := func(i int, name, data string) func([]threadWireEvent) []threadWireEvent {
		return func(e []threadWireEvent) []threadWireEvent { e[i] = threadWireEvent{name, data}; return e }
	}
	insert := func(i int, events ...threadWireEvent) func([]threadWireEvent) []threadWireEvent {
		return func(e []threadWireEvent) []threadWireEvent {
			return append(append(append([]threadWireEvent{}, e[:i]...), events...), e[i:]...)
		}
	}
	start := func(index string) threadWireEvent {
		return threadWireEvent{"content_block_start", `{"type":"content_block_start","index":` + index + `,"content_block":{"type":"text"}}`}
	}
	stop := func(index string) threadWireEvent {
		return threadWireEvent{"content_block_stop", `{"type":"content_block_stop","index":` + index + `}`}
	}
	for name, tc := range map[string]struct {
		reason, block string
		mutate        func([]threadWireEvent) []threadWireEvent
		output        int
	}{
		"refusal":             {reason: "refusal"},
		"truncated tool call": {reason: "max_tokens", block: "tool_use"},
		"truncated mcp call":  {reason: "max_tokens", block: "mcp_tool_use"},
		"fallback model": {mutate: set(5, "message_delta",
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":20,"iterations":[{"type":"message"},{"type":"fallback_message","model":"x"}]}}`)},
		"no stop reason": {mutate: set(5, "message_delta", `{"type":"message_delta","delta":{},"usage":{"output_tokens":20}}`)},
		"two message_start": {mutate: insert(1, threadWireEvent{"message_start",
			`{"type":"message_start","message":{"id":"msg_other","type":"message","usage":{"input_tokens":100,"cache_read_input_tokens":900,"cache_creation_input_tokens":50}}}`})},
		"id with a space": {mutate: set(0, "message_start",
			`{"type":"message_start","message":{"id":"msg_new ","type":"message","usage":{"input_tokens":100,"cache_read_input_tokens":900,"cache_creation_input_tokens":50}}}`)},

		// Name and payload type: both, and the same.
		"stop named, other payload": {mutate: set(6, "message_stop", `{"type":"ping"}`)},
		"stop payload, other name":  {mutate: set(6, "ping", `{"type":"message_stop"}`)},
		"stop named, no type":       {mutate: set(6, "message_stop", `{}`)},
		"stop named, type not text": {mutate: set(6, "message_stop", `{"type":7}`)},
		"stop payload, no name":     {mutate: set(6, "", `{"type":"message_stop"}`)},
		"stop that is not json":     {mutate: set(6, "message_stop", `{"type":"message_stop"`)},
		"stop with trailing data":   {mutate: set(6, "message_stop", `{"type":"message_stop"} {}`)},
		"stop that is an array":     {mutate: set(6, "message_stop", `[{"type":"message_stop"}]`)},
		// A member named twice: which one counts depends on who reads it.
		"stop with two types":      {mutate: set(6, "message_stop", `{"type":"ping","type":"message_stop"}`)},
		"stop with the type twice": {mutate: set(6, "message_stop", `{"type":"message_stop","type":"message_stop"}`)},
		"two stop reasons": {mutate: set(5, "message_delta",
			`{"type":"message_delta","delta":{"stop_reason":"refusal","stop_reason":"end_turn"},"usage":{"output_tokens":20}}`)},
		"two ids": {mutate: set(0, "message_start",
			`{"type":"message_start","message":{"id":"msg_other","id":"msg_new","type":"message","usage":{"input_tokens":100,"cache_read_input_tokens":900,"cache_creation_input_tokens":50}}}`)},
		"two indexes":              {mutate: set(3, "content_block_stop", `{"type":"content_block_stop","index":1,"index":0}`)},
		"delta without name":       {mutate: set(5, "", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":20}}`)},
		"delta named as something": {mutate: set(5, "ping", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":20}}`)},
		"block stop without name":  {mutate: set(3, "", `{"type":"content_block_stop","index":0}`)},
		"start named as delta": {mutate: set(0, "message_delta",
			`{"type":"message_start","message":{"id":"msg_new","type":"message","usage":{"input_tokens":100,"cache_read_input_tokens":900,"cache_creation_input_tokens":50}}}`)},

		// Order, and each block under its own index.
		"block never closed":         {mutate: remove(3)},
		"block never opened":         {mutate: remove(1)},
		"stop without any delta":     {mutate: remove(5), output: 3},
		"block closed as another":    {mutate: set(3, "content_block_stop", `{"type":"content_block_stop","index":1}`)},
		"delta for another block":    {mutate: set(2, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"x"}}`)},
		"block opened twice":         {mutate: insert(2, start("0"))},
		"index used again":           {mutate: insert(4, start("0"), stop("0"))},
		"block closed twice":         {mutate: insert(4, stop("0"))},
		"block after the delta":      {mutate: insert(6, start("1"), stop("1"))},
		"delta inside a block":       {mutate: func(e []threadWireEvent) []threadWireEvent { e[3], e[5] = e[5], e[3]; return e }},
		"stop without an index":      {mutate: set(3, "content_block_stop", `{"type":"content_block_stop"}`)},
		"index that is a string":     {mutate: set(3, "content_block_stop", `{"type":"content_block_stop","index":"0"}`)},
		"index that is no integer":   {mutate: set(3, "content_block_stop", `{"type":"content_block_stop","index":0.5}`)},
		"index spelled as a decimal": {mutate: set(3, "content_block_stop", `{"type":"content_block_stop","index":0.0}`)},
		"negative index": {mutate: func(e []threadWireEvent) []threadWireEvent {
			e[1], e[2], e[3] = start("-1"), threadWireEvent{"ping", `{"type":"ping"}`}, stop("-1")
			return e
		}},
	} {
		t.Run(name, func(t *testing.T) {
			reason, block := tc.reason, tc.block
			if reason == "" {
				reason = "end_turn"
			}
			if block == "" {
				block = "text"
			}
			events := threadTestStream(reason, block)
			if tc.mutate != nil {
				events = tc.mutate(events)
			}
			stream := threadWire(events, "\n", false)
			output := 20
			if tc.output != 0 {
				output = tc.output
			}
			for _, passthrough := range []bool{false, true} {
				out := runThreadStream(t, strings.NewReader(stream), threadStreamRun{opts: threadStreamOptions{passthrough: passthrough}})
				require.Zero(t, out.store.binds)
				require.Equal(t, [4]int{100, 900, 50, output}, out.usage(), "what the upstream metered is billed")
				switch name {
				case "refusal", "truncated tool call", "truncated mcp call", "fallback model":
					require.NoError(t, out.err)
					require.Equal(t, stream, out.body, "valid even though it cannot be continued")
				default:
					require.Error(t, out.err)
					// Do not repair or drop earlier frames. Replace only the bad
					// termination with a single explicit error on both routes.
					prefix := threadWire(events[:len(events)-1], "\n", false)
					require.True(t, strings.HasPrefix(out.body, prefix))
					tail := strings.TrimPrefix(out.body, prefix)
					require.True(t, strings.HasPrefix(tail, "event: error\ndata: "), tail)
					require.Equal(t, 1, strings.Count(tail, "\n\n"))
					require.NotContains(t, tail, "message_stop")
				}
			}
		})
	}

	// Reasons outside the stream.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	stream := threadWire(threadTestStream("end_turn", "text"), "\n", false)
	for name, run := range map[string]threadStreamRun{
		"client gone":       {failAt: 3},
		"request canceled":  {ctx: canceled},
		"not sealed":        {prepare: func(t *MessageThread, _ *gin.Context) { t.attempt.sealed = false }},
		"request rewritten": {prepare: func(t *MessageThread, _ *gin.Context) { t.markRewritten() }},
	} {
		out := runThreadStream(t, strings.NewReader(stream), run)
		require.Zero(t, out.store.binds, name)
		require.Equal(t, [4]int{100, 900, 50, 20}, out.usage(), name)
	}
	out := runThreadStream(t, strings.NewReader(threadWire(func() []threadWireEvent {
		events := threadTestStream("end_turn", "text")
		events[0].data = `{"type":"message_start","message":{"id":"msg_new","type":"message"}}`
		events[5].data = `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`
		return events
	}(), "\n", false)), threadStreamRun{})
	require.Zero(t, out.store.binds, "a response without usage is not one to reserve for")

	// A failed write of the binding never fails the response.
	out = runThreadStream(t, strings.NewReader(stream), threadStreamRun{prepare: func(t *MessageThread, _ *gin.Context) {
		t.store = &threadStoreStub{bindErr: errors.New("redis down")}
	}})
	require.NoError(t, out.err)
	require.Equal(t, stream, out.body)
}

// Several blocks, blocks open at the same time, unknown events and events
// without JSON are all part of a stream that can be continued.
func TestMessageThreadStreamLegalVariety(t *testing.T) {
	events := []threadWireEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_new","type":"message","usage":{"input_tokens":100,"cache_read_input_tokens":900,"cache_creation_input_tokens":50,"output_tokens":1}}}`},
		{"future_event", `not json`},
		{"future_event", `{"type":"something_else","index":9}`},
		{"", `[DONE]`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`},
		{"content_block_start", `{"type":"content_block_start","index":2,"content_block":{"type":"tool_use"}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"type\":\"message_stop\""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"event: message_stop"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"content_block_stop", `{"type":"content_block_stop","index":2}`},
		{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":1}`},
		{"message_delta", `{"type":"message_delta","delta":{},"usage":{"output_tokens":12}}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":20}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}
	stream := threadWire(events, "\n", false)
	out := runThreadStream(t, strings.NewReader(stream), threadStreamRun{})
	require.NoError(t, out.err)
	require.Equal(t, 1, out.store.binds)
	require.Equal(t, [4]int{100, 900, 50, 20}, out.usage(), "usage is cumulative: the last report counts, nothing is added up")
	require.Equal(t, stream, out.body)

	// Nothing after the terminal event is interpreted, relayed or bound.
	more := stream + threadWire(threadTestStream("end_turn", "text"), "\n", false) +
		threadWire([]threadWireEvent{{"message_delta", `{"type":"message_delta","usage":{"output_tokens":9999}}`}}, "\n", false)
	out = runThreadStream(t, strings.NewReader(more), threadStreamRun{})
	require.NoError(t, out.err)
	require.Equal(t, 1, out.store.binds)
	require.Equal(t, [4]int{100, 900, 50, 20}, out.usage())
	require.Equal(t, stream, out.body)
}

// The same events mean the same in every representation the format allows and
// however the bytes are cut up on the way: the same usage, the same binding
// at the same point, the same events for the client.
func TestMessageThreadStreamRepresentationInvariance(t *testing.T) {
	events := threadTestStream("tool_use", "tool_use")
	plain := threadWire(events, "\n", false)
	reference := runThreadStream(t, strings.NewReader(plain), threadStreamRun{})
	require.NoError(t, reference.err)
	require.Equal(t, [4]int{100, 900, 50, 20}, reference.usage())
	binding := reference.store.bindings["1:2:3/msg_new"]

	same := func(t *testing.T, out threadStreamOutcome, wantBody string, context string) {
		t.Helper()
		require.NoError(t, out.err, context)
		require.Equal(t, reference.usage(), out.usage(), context)
		require.Equal(t, 1, out.store.binds, context)
		require.Equal(t, binding, out.store.bindings["1:2:3/msg_new"], context)
		require.Equal(t, wantBody, out.body, context)
		require.Equal(t, []int{0, 0, 0, 0, 0, 0, 1}, out.client.binds, context)
	}
	for _, newline := range []string{"\n", "\r\n", "\r"} {
		for _, split := range []bool{false, true} {
			stream := threadWire(events, newline, split)
			name := fmt.Sprintf("newline=%q split=%t", newline, split)
			whole := runThreadStream(t, strings.NewReader(stream), threadStreamRun{})
			// The client is handed the same events. Data that arrived on
			// several lines is relayed on several lines, in the canonical form.
			wantBody := plain
			if split {
				wantBody = threadWire(events, "\n", true)
			}
			same(t, whole, wantBody, name)
			if split {
				// The usage sat on a data line of its own, not on the first.
				require.Contains(t, stream, "data: \"usage\":{\"output_tokens\":20}}")
			}
			for cut := 1; cut < len(stream); cut++ {
				out := runThreadStream(t, &threadSlicedReader{parts: []string{stream[:cut], stream[cut:]}}, threadStreamRun{})
				same(t, out, wantBody, fmt.Sprintf("%s cut at %d", name, cut))
			}
			for _, size := range []int{1, 2, 3, 7, 64} {
				var parts []string
				for rest := stream; rest != ""; {
					n := min(size, len(rest))
					parts, rest = append(parts, rest[:n]), rest[n:]
				}
				out := runThreadStream(t, &threadSlicedReader{parts: parts}, threadStreamRun{})
				same(t, out, wantBody, fmt.Sprintf("%s in pieces of %d", name, size))
			}
		}
	}
	// Comments, other fields and a byte order mark change nothing either.
	decorated := "\xEF\xBB\xBF: hello\n\nid: 1\nretry: 3\n" + strings.ReplaceAll(plain, "\n\n", "\n\n: between\n\n")
	same(t, runThreadStream(t, strings.NewReader(decorated), threadStreamRun{}), plain, "decorated")
}

// A stream that stops inside an event did not send that event. It completes
// nothing and is not relayed, on any route; usage it had already written out
// is still billed.
func TestMessageThreadStreamEndsInsideAnEvent(t *testing.T) {
	events := threadTestStream("end_turn", "text")
	whole := threadWire(events, "\n", false)
	untilDelta := threadWire(events[:5], "\n", false)
	untilStop := threadWire(events[:6], "\n", false)
	delta := "event: message_delta\ndata: " + events[5].data
	for name, tc := range map[string]struct {
		stream     string
		wantBody   string
		wantOutput int
	}{
		"stop without its blank line":      {strings.TrimSuffix(whole, "\n"), untilStop, 20},
		"stop followed by a space line":    {strings.TrimSuffix(whole, "\n") + " \n", untilStop, 20},
		"stop followed by a tab line":      {strings.TrimSuffix(whole, "\n") + "\t\n", untilStop, 20},
		"stop without its line end":        {strings.TrimSuffix(whole, "\n\n"), untilStop, 20},
		"stop without data":                {untilStop + "event: message_stop\n\n", untilStop, 20},
		"stop cut in its data":             {untilStop + "event: message_stop\ndata: {\"type\":\"mess", untilStop, 20},
		"delta without its blank line":     {untilDelta + delta + "\n", untilDelta, 20},
		"delta without its line end":       {untilDelta + delta, untilDelta, 20},
		"delta on two lines, cut after":    {untilDelta + "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{},\ndata: \"usage\":{\"output_tokens\":20}}\n", untilDelta, 20},
		"delta cut before its usage ended": {untilDelta + "event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":2", untilDelta, 3},
		"right after message_start":        {threadWire(events[:1], "\n", false), threadWire(events[:1], "\n", false), 3},
	} {
		for _, passthrough := range []bool{false, true} {
			for _, failure := range []error{nil, io.ErrUnexpectedEOF} {
				out := runThreadStream(t, &threadSlicedReader{parts: []string{tc.stream}, err: failure},
					threadStreamRun{opts: threadStreamOptions{passthrough: passthrough}})
				context := fmt.Sprintf("%s passthrough=%t failure=%v", name, passthrough, failure)
				require.Error(t, out.err, context)
				require.Zero(t, out.store.binds, context)
				require.NotNil(t, out.result, context)
				require.Equal(t, [4]int{100, 900, 50, tc.wantOutput}, out.usage(), context)
				prefix, tail, sentError := strings.Cut(out.body, "event: error\ndata: ")
				require.True(t, sentError, context)
				require.Equal(t, tc.wantBody, prefix, context)
				require.Equal(t, 1, strings.Count(tail, "\n\n"), context)
				require.NotContains(t, out.body, "message_stop", context)
			}
		}
	}
}

// A false stop permits accounting only for the current message in the
// already-framed batch. A later Read or another message cannot add a charge.
func TestMessageThreadFramedUsageBoundary(t *testing.T) {
	events := threadTestStream("end_turn", "text")
	events[3].name = "message_stop" // payload still closes block 0
	wire := func(e []threadWireEvent) string { return threadWire(e, "\n", false) }
	prefix := wire(events[:4])
	finalUsage, realStop := wire(events[5:6]), wire(events[6:])
	extra := strings.Replace(finalUsage, `"output_tokens":20`, `"output_tokens":999`, 1)
	for _, tc := range []struct {
		name   string
		reads  []string
		output int
	}{
		{"stop inside this read", []string{prefix + finalUsage + realStop + extra}, 20},
		{"usage in the next read", []string{prefix, finalUsage + realStop}, 3},
		{"another message", []string{prefix + wire(events[:1]) + extra + realStop}, 3},
		{"delta not dispatched", []string{prefix + strings.TrimSuffix(finalUsage, "\n\n")}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := runThreadStream(t, &threadSlicedReader{parts: tc.reads}, threadStreamRun{})
			require.Error(t, out.err)
			require.Zero(t, out.store.binds)
			require.Equal(t, [4]int{100, 900, 50, tc.output}, out.usage())
			require.Equal(t, 1, strings.Count(out.body, "event: error\n"))
			require.NotContains(t, out.body, "event: message_stop\n")
			require.NotContains(t, out.body, "event: message_delta\n")
		})
	}
}

func TestMessageThreadStreamErrorsAndLimits(t *testing.T) {
	events := threadTestStream("end_turn", "text")
	failed := threadWire(append(append([]threadWireEvent{}, events[:2]...),
		threadWireEvent{"error", `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`}), "\n", false)

	// The route that builds its own error response hands the event back with
	// what was metered before it.
	out := runThreadStream(t, strings.NewReader(failed), threadStreamRun{})
	var streamErr *sseStreamErrorEventError
	require.ErrorAs(t, out.err, &streamErr)
	require.JSONEq(t, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`, streamErr.RawData)
	require.Equal(t, [4]int{100, 900, 50, 3}, out.usage())
	require.Zero(t, out.store.binds)
	require.Equal(t, threadWire(events[:2], "\n", false), out.body)

	// The passthrough route relays it like any event.
	out = runThreadStream(t, strings.NewReader(failed), threadStreamRun{opts: threadStreamOptions{passthrough: true}})
	require.Error(t, out.err)
	require.Equal(t, failed, out.body)
	require.Equal(t, [4]int{100, 900, 50, 3}, out.usage())
	require.Zero(t, out.store.binds)

	// A complete stream that had an error event in it is not continued either.
	withError := threadWire(append(append(append([]threadWireEvent{}, events[:4]...),
		threadWireEvent{"error", `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`}), events[4:]...), "\n", false)
	out = runThreadStream(t, strings.NewReader(withError), threadStreamRun{opts: threadStreamOptions{passthrough: true}})
	require.Error(t, out.err)
	require.Zero(t, out.store.binds)
	require.Equal(t, [4]int{100, 900, 50, 20}, out.usage(), "drain for billing even after the error")
	require.Equal(t, strings.SplitAfterN(withError, `"message":"Overloaded"}}`+"\n\n", 2)[0], out.body, "the upstream error, but no success after it")

	// An event larger than the configured size ends the stream.
	cfg := &config.Config{}
	cfg.Gateway.SSEEventMaxBytes = 256
	huge := threadWire(events[:1], "\n", false) + "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"x\":\"" + strings.Repeat("a", 1024)
	for _, passthrough := range []bool{false, true} {
		out = runThreadStream(t, strings.NewReader(huge+threadWire(events[5:], "\n", false)), threadStreamRun{cfg: cfg, opts: threadStreamOptions{passthrough: passthrough}})
		require.ErrorIs(t, out.err, bufio.ErrTooLong)
		require.Zero(t, out.store.binds)
		require.Equal(t, [4]int{100, 900, 50, 3}, out.usage())
		require.NotContains(t, out.body, "message_stop")
	}

	// Nothing arrived at all: reported as an upstream failure with no output.
	out = runThreadStream(t, &threadSlicedReader{err: io.ErrUnexpectedEOF}, threadStreamRun{})
	var failover *UpstreamFailoverError
	require.ErrorAs(t, out.err, &failover)
	require.Empty(t, out.body)
	require.Nil(t, out.result)

	// The connection broke inside the very first event, after the upstream
	// had written what it metered. No event reached the client, and the
	// request is still not handed on as if it had cost nothing.
	broken := strings.TrimSuffix(threadWire(events[:1], "\n", false), "\n\n")
	for _, passthrough := range []bool{false, true} {
		out = runThreadStream(t, &threadSlicedReader{parts: []string{broken}, err: io.ErrUnexpectedEOF}, threadStreamRun{opts: threadStreamOptions{passthrough: passthrough}})
		require.Error(t, out.err)
		require.False(t, errors.As(out.err, &failover))
		require.NotNil(t, out.result)
		require.Equal(t, [4]int{100, 900, 50, 3}, out.usage())
		require.NotContains(t, out.body, "message_start")
		require.Zero(t, out.store.binds)
	}
}

// The transforms of a route work on the decoded event and leave the question
// of completion alone: it is answered from what the upstream sent.
func TestMessageThreadStreamTransforms(t *testing.T) {
	events := []threadWireEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_new","type":"message","model":"claude-upstream","content":[],"usage":{"input_tokens":100,"cache_read_input_tokens":900,"cache_creation_input_tokens":50,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":50},"output_tokens":3}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hm"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"cc_sess_read","input":{}}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"name\":\"cc_sess_read\"}"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":1}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":20}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}
	// Model and cache usage transforms must preserve all content blocks.
	account := &Account{ID: 7, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "k"},
		Extra: map[string]any{"custom_base_url_enabled": true, "custom_base_url": "https://relay.example.com",
			"cache_ttl_override_enabled": true, "cache_ttl_override_target": "5m"}}

	var reference string
	for _, split := range []bool{false, true} {
		for _, newline := range []string{"\n", "\r\n"} {
			stream := threadWire(events, newline, split)
			out := runThreadStream(t, &threadSlicedReader{parts: strings.SplitAfter(stream, ":")}, threadStreamRun{
				account: account,
				opts:    threadStreamOptions{originalModel: "claude-public", mappedModel: "claude-upstream"},
			})
			require.NoError(t, out.err)
			// Completion and billing use the upstream events.
			require.Equal(t, 1, out.store.binds)
			require.Equal(t, [4]int{100, 900, 50, 20}, out.usage())

			var got []string
			for _, block := range strings.Split(strings.TrimSuffix(out.body, "\n\n"), "\n\n") {
				name, data, _ := strings.Cut(block, "\n")
				var decoded any
				require.NoError(t, json.Unmarshal([]byte(strings.ReplaceAll(strings.TrimPrefix(data, "data: "), "\ndata: ", "\n")), &decoded), block)
				compact, _ := json.Marshal(decoded)
				got = append(got, name+" "+string(compact))
			}
			require.Len(t, got, 9, "both content blocks retain their original indexes")
			require.Contains(t, got[0], `"model":"claude-public"`)
			require.Contains(t, got[0], `"ephemeral_5m_input_tokens":50`)
			require.Contains(t, got[0], `"ephemeral_1h_input_tokens":0`)
			require.Equal(t, `event: content_block_start {"content_block":{"id":"toolu_1","input":{},"name":"cc_sess_read","type":"tool_use"},"index":1,"type":"content_block_start"}`, got[4],
				"the exact tool name and index")
			require.Contains(t, got[5], `"index":1`)
			require.Contains(t, got[5], `cc_sess_read`, "arguments are opaque")
			require.Equal(t, `event: content_block_stop {"index":1,"type":"content_block_stop"}`, got[6])
			require.Equal(t, `event: message_stop {"type":"message_stop"}`, got[8])
			if reference == "" {
				reference = strings.Join(got, "\n")
			}
			require.Equal(t, reference, strings.Join(got, "\n"), "the same for every representation")
		}
	}

	// The passthrough route keeps the upstream model, cache counters and thinking blocks.
	out := runThreadStream(t, strings.NewReader(threadWire(events, "\n", false)), threadStreamRun{
		account: account, opts: threadStreamOptions{passthrough: true, originalModel: "claude-public", mappedModel: "claude-upstream"},
	})
	require.NoError(t, out.err)
	require.Equal(t, 1, out.store.binds)
	require.Contains(t, out.body, `"model":"claude-upstream"`)
	require.Contains(t, out.body, `"ephemeral_1h_input_tokens":50`)
	require.Contains(t, out.body, "thinking_delta")
}

func TestMessageThreadObserveJSON(t *testing.T) {
	usage := &ClaudeUsage{InputTokens: 100, CacheReadInputTokens: 900, CacheCreationInputTokens: 50, OutputTokens: 20}
	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"complete":            {`{"id":"msg_json","type":"message","stop_reason":"tool_use","content":[{"type":"text","text":"x"},{"type":"tool_use","id":"t","name":"n","input":{}}]}`, 1},
		"paused":              {`{"id":"msg_json","type":"message","stop_reason":"pause_turn","content":[]}`, 1},
		"refusal":             {`{"id":"msg_json","type":"message","stop_reason":"refusal","content":[]}`, 0},
		"truncated tool call": {`{"id":"msg_json","type":"message","stop_reason":"max_tokens","content":[{"type":"text","text":"x"},{"type":"tool_use","id":"t","name":"n","input":{}}]}`, 0},
		"truncated text":      {`{"id":"msg_json","type":"message","stop_reason":"max_tokens","content":[{"type":"tool_use","id":"t","name":"n","input":{}},{"type":"text","text":"x"}]}`, 1},
		"fallback model":      {`{"id":"msg_json","type":"message","stop_reason":"end_turn","content":[],"usage":{"iterations":[{"type":"fallback_message"}]}}`, 0},
		"error body":          {`{"type":"error","error":{"type":"api_error"}}`, 0},
		"no stop reason":      {`{"id":"msg_json","type":"message","content":[]}`, 0},
		"no id":               {`{"type":"message","stop_reason":"end_turn","content":[]}`, 0},
		"id that is a number": {`{"id":7,"type":"message","stop_reason":"end_turn","content":[]}`, 0},
		"two documents":       {`{"id":"msg_json","type":"message","stop_reason":"end_turn","content":[]} {}`, 0},
		"two ids":             {`{"id":"msg_other","id":"msg_json","type":"message","stop_reason":"end_turn","content":[]}`, 0},
		"two stop reasons":    {`{"id":"msg_json","type":"message","stop_reason":"refusal","stop_reason":"end_turn","content":[]}`, 0},
		"twice, far inside":   {`{"id":"msg_json","type":"message","stop_reason":"end_turn","content":[{"type":"text","text":"a","text":"b"}]}`, 0},
		"cut off":             {`{"id":"msg_json","type":"message","stop_reason":"end_turn","content":[`, 0},
	} {
		store := &threadStoreStub{}
		thread := &MessageThread{store: store, scope: "1:2:3"}
		thread.attempt.sealed, thread.attempt.next = true, threadTestBinding()
		err := thread.observeJSON(context.Background(), []byte(tc.body), usage)
		if name == "error body" || name == "cut off" || name == "two documents" {
			require.Error(t, err, name)
		} else {
			require.NoError(t, err, name)
		}
		require.Equal(t, tc.want, store.binds, name)
		// A response is judged once.
		require.NoError(t, thread.observeJSON(context.Background(), []byte(`{"id":"msg_again","type":"message","stop_reason":"end_turn","content":[]}`), usage))
		require.Equal(t, tc.want, store.binds, name)
	}

	// Nothing here runs for a request without a thread.
	var none *MessageThread
	none.beginAttempt("m")
	none.markRewritten()
	none.SetRequestContext("s", "m", true)
	require.NoError(t, none.observeJSON(context.Background(), []byte(`{}`), usage))
	require.False(t, none.InheritsClaudeCode())
	require.False(t, none.AdmissionFault())
	require.Equal(t, "computed", none.SessionHash("computed"))
	require.Zero(t, none.BoundAccountID())
}

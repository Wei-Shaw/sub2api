package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// The response stream of a message thread is read here, by one decoder, for
// every account type a thread can live on.
//
//	upstream bytes ─► threadSSEDecoder ─► threadSSEEvent ─► one JSON decode
//	                                                          │
//	              ┌───────────────────────────────────────────┼─────────────────────┐
//	              ▼                                           ▼                     ▼
//	   threadStreamState                              usage (billing)        output to client
//	   (is this a complete response                   from that payload      that event, with the
//	    that can be continued?)                                              route's transforms
//
// Three questions are asked of every event: does it complete the response,
// what did the upstream meter, and what does the client receive. They are
// answered from the same decoded event, so they cannot disagree about where an
// event ends, what its data is, or whether it arrived at all. Requests without
// a thread do not come here.

// threadSSEEvent is one event of a server-sent events stream as the format
// defines it: dispatched by a blank line, with the data fields joined by a
// line feed. Named tells an event that carries an event field from one that
// does not; nothing is filled in for the upstream.
type threadSSEEvent struct {
	Name  string
	Named bool
	Data  string
}

var errThreadSSEEventTooLarge = fmt.Errorf("%w: %w: message thread stream event exceeds the configured size", errSSETransportLimit, bufio.ErrTooLong)

var threadSSEByteOrderMark = []byte{0xEF, 0xBB, 0xBF}

// threadSSEDecoder turns the bytes of an event stream into events, in whatever
// pieces the bytes arrive. It follows the format's own rules: a line ends in
// CRLF, LF or CR; only an empty line ends an event (a line of spaces is a
// field with an odd name); every data field counts; a line that starts with a
// colon is a comment; an event without data is not dispatched; and whatever
// is still open when the stream ends was never an event. One event may not
// exceed maxBytes or maxLines, so an upstream cannot make it hold more.
type threadSSEDecoder struct {
	maxBytes int64
	maxLines int

	line    []byte
	afterCR bool
	started bool

	name    string
	named   bool
	data    []byte
	hasData bool
	bytes   int64
	lines   int
}

func newThreadSSEDecoder(maxBytes int64, maxLines int) *threadSSEDecoder {
	return &threadSSEDecoder{maxBytes: maxBytes, maxLines: maxLines}
}

// Feed consumes chunk and appends the events it completes to events.
func (d *threadSSEDecoder) Feed(chunk []byte, events []threadSSEEvent) ([]threadSSEEvent, error) {
	for len(chunk) > 0 {
		if d.afterCR {
			d.afterCR = false
			if chunk[0] == '\n' {
				chunk = chunk[1:]
				continue
			}
		}
		end := bytes.IndexAny(chunk, "\r\n")
		if end < 0 {
			if err := d.grow(len(chunk)); err != nil {
				return events, err
			}
			d.line = append(d.line, chunk...)
			return events, nil
		}
		if err := d.grow(end + 1); err != nil {
			return events, err
		}
		d.line = append(d.line, chunk[:end]...)
		d.afterCR = chunk[end] == '\r'
		chunk = chunk[end+1:]
		event, dispatched, err := d.endLine()
		if err != nil {
			return events, err
		}
		if dispatched {
			events = append(events, event)
		}
	}
	return events, nil
}

func (d *threadSSEDecoder) grow(n int) error {
	d.bytes += int64(n)
	if d.bytes > d.maxBytes {
		return errThreadSSEEventTooLarge
	}
	return nil
}

func (d *threadSSEDecoder) endLine() (threadSSEEvent, bool, error) {
	line := d.line
	if cap(d.line) > 64<<10 {
		d.line = nil
	} else {
		d.line = d.line[:0]
	}
	if !d.started {
		d.started = true
		line = bytes.TrimPrefix(line, threadSSEByteOrderMark)
	}
	if len(line) == 0 {
		event, dispatched := threadSSEEvent{}, d.hasData
		if dispatched {
			// The data buffer holds every data field followed by a line feed;
			// the last one is not part of the data.
			event = threadSSEEvent{Name: d.name, Named: d.named, Data: string(d.data[:len(d.data)-1])}
		}
		d.name, d.named, d.hasData, d.bytes, d.lines = "", false, false, 0, 0
		if cap(d.data) > 64<<10 {
			d.data = nil
		} else {
			d.data = d.data[:0]
		}
		return event, dispatched, nil
	}
	d.lines++
	if d.lines > d.maxLines {
		return threadSSEEvent{}, false, errThreadSSEEventTooLarge
	}
	d.field(line)
	return threadSSEEvent{}, false, nil
}

func (d *threadSSEDecoder) field(line []byte) {
	if line[0] == ':' {
		return
	}
	name, value := line, []byte(nil)
	if colon := bytes.IndexByte(line, ':'); colon >= 0 {
		name, value = line[:colon], bytes.TrimPrefix(line[colon+1:], []byte(" "))
	}
	switch string(name) {
	case "event":
		d.name, d.named = string(value), true
	case "data":
		d.data = append(append(d.data, value...), '\n')
		d.hasData = true
	}
}

// unfinishedData is the data of the event the stream stopped in, as far as it
// arrived. It was never dispatched and is not an event. It is offered for one
// purpose only: usage the upstream had already written out is still usage.
func (d *threadSSEDecoder) unfinishedData() string {
	data := append([]byte(nil), d.data...)
	if value, ok := bytes.CutPrefix(d.line, []byte("data:")); ok {
		data = append(append(data, bytes.TrimPrefix(value, []byte(" "))...), '\n')
	}
	return strings.TrimSuffix(string(data), "\n")
}

// decodeThreadEventPayload decodes the data of an event once. Everything that
// is concluded from an event, or changed in it, works on this value. Data that
// is not exactly one JSON object has no payload.
//
// ambiguous reports an object that names a member twice. JSON parsers differ
// on which of the two counts, so the client may read such an event differently
// from this code. It is relayed as it is and its usage is billed; nothing
// about the completeness of the response is concluded from it.
func decodeThreadEventPayload(data string) (payload map[string]any, ambiguous bool) {
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.UseNumber()
	value, err := decodeThreadJSONValue(decoder, &ambiguous, 0)
	if err != nil {
		return nil, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	payload, _ = value.(map[string]any)
	return payload, ambiguous && payload != nil
}

const threadJSONMaxDepth = 512

func decodeThreadJSONValue(decoder *json.Decoder, duplicate *bool, depth int) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, nested := token.(json.Delim)
	if !nested {
		return token, nil
	}
	if depth >= threadJSONMaxDepth {
		return nil, errors.New("JSON nested too deeply")
	}
	switch delim {
	case '{':
		object := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, errors.New("JSON object key is not a string")
			}
			value, err := decodeThreadJSONValue(decoder, duplicate, depth+1)
			if err != nil {
				return nil, err
			}
			if _, seen := object[name]; seen {
				*duplicate = true
			}
			object[name] = value
		}
		_, err := decoder.Token()
		return object, err
	case '[':
		list := []any{}
		for decoder.More() {
			value, err := decodeThreadJSONValue(decoder, duplicate, depth+1)
			if err != nil {
				return nil, err
			}
			list = append(list, value)
		}
		_, err := decoder.Token()
		return list, err
	}
	return nil, errors.New("unexpected JSON delimiter")
}

// threadStreamEvents are the Messages stream events a binding depends on.
// Other event types (ping, and any added later) are passed on and not judged.
var threadStreamEvents = map[string]bool{
	"message_start": true, "message_delta": true, "message_stop": true, "error": true,
	"content_block_start": true, "content_block_delta": true, "content_block_stop": true,
}

const threadStreamMaxBlocks = 4096

// threadResponseFacts is what a response says about itself that decides
// whether it can be continued.
type threadResponseFacts struct {
	id         string
	stopReason string
	lastBlock  string
	fallback   bool
	// invalid names the first thing that keeps this from being one complete
	// response; empty when there is none.
	invalid string
}

// threadStreamState follows the events of one Messages stream: message_start,
// content blocks that are each opened and closed under their own index,
// message_delta, message_stop. Every one of these events must carry its name
// both as the event field and as the payload type. Keep metering a malformed
// stream, but never give it a successful terminal event or a binding.
type threadStreamState struct {
	threadResponseFacts
	blocks   map[int]bool // index → still open
	open     int
	maxIndex int // largest final content position, not the most recent start
	sawDelta bool
}

func (st *threadStreamState) fail(reason string) {
	if st.invalid == "" {
		st.invalid = reason
	}
}

// observe takes one dispatched event with its decoded payload and reports
// whether it is the terminal event of the stream.
func (st *threadStreamState) observe(event threadSSEEvent, payload map[string]any, ambiguous bool) (terminal bool) {
	kind, _ := payload["type"].(string)
	if !threadStreamEvents[event.Name] && !threadStreamEvents[kind] {
		return false
	}
	terminal = event.Name == "message_stop" || kind == "message_stop"
	if payload == nil || !event.Named || event.Name != kind {
		st.fail("event_name_and_type_differ")
		return terminal
	}
	if ambiguous {
		st.fail("ambiguous_json")
	}
	if kind != "message_start" && st.id == "" {
		st.fail("no_message_start")
		return terminal
	}
	switch kind {
	case "message_start":
		message, _ := payload["message"].(map[string]any)
		id, _ := message["id"].(string)
		if st.id != "" || id == "" || len(id) > 256 || strings.TrimSpace(id) != id {
			st.fail("message_start")
			return false
		}
		st.id = id
	case "content_block_start":
		index, ok := threadStreamBlockIndex(payload)
		if _, used := st.blocks[index]; !ok || used || st.sawDelta || len(st.blocks) >= threadStreamMaxBlocks {
			st.fail("content_block_start")
			return false
		}
		if st.blocks == nil {
			st.blocks = map[int]bool{}
		}
		st.blocks[index] = true
		st.open++
		st.maxIndex = max(st.maxIndex, index)
		if index == st.maxIndex {
			block, _ := payload["content_block"].(map[string]any)
			st.lastBlock, _ = block["type"].(string)
		}
	case "content_block_delta":
		if index, ok := threadStreamBlockIndex(payload); !ok || !st.blocks[index] {
			st.fail("content_block_delta")
		}
	case "content_block_stop":
		index, ok := threadStreamBlockIndex(payload)
		if !ok || !st.blocks[index] {
			st.fail("content_block_stop")
			return false
		}
		st.blocks[index] = false
		st.open--
	case "message_delta":
		if st.open > 0 {
			st.fail("message_delta_inside_block")
			return false
		}
		st.sawDelta = true
		delta, _ := payload["delta"].(map[string]any)
		if reason, ok := delta["stop_reason"].(string); ok {
			st.stopReason = reason
		}
		usage, _ := payload["usage"].(map[string]any)
		if threadUsageHasFallbackMessage(usage) {
			st.fallback = true
		}
	case "error":
		st.fail("error_event")
	case "message_stop":
		if st.open > 0 || !st.sawDelta || st.stopReason == "" {
			st.fail("message_stop_out_of_order")
		}
		// Distinct nonnegative indexes form a dense list iff max == count-1.
		// No scan or allocation proportional to an upstream-provided index.
		if len(st.blocks) > 0 && st.maxIndex != len(st.blocks)-1 {
			st.fail("missing_content_block")
		}
	}
	return terminal
}

func threadStreamBlockIndex(payload map[string]any) (int, bool) {
	number, ok := payload["index"].(json.Number)
	if !ok {
		return 0, false
	}
	index, err := strconv.Atoi(number.String())
	return index, err == nil && index >= 0
}

// A response written by a fallback model is not stored by the upstream.
func threadUsageHasFallbackMessage(usage map[string]any) bool {
	iterations, _ := usage["iterations"].([]any)
	for _, iteration := range iterations {
		if fields, ok := iteration.(map[string]any); ok && fields["type"] == "fallback_message" {
			return true
		}
	}
	return false
}

// formatThreadSSEEvent writes an event in one canonical representation: its
// event field if it has one, a data field per line of data, a blank line.
func formatThreadSSEEvent(event threadSSEEvent, data string) string {
	var b strings.Builder
	b.Grow(len(event.Name) + len(data) + 16)
	if event.Named {
		_, _ = b.WriteString("event: " + event.Name + "\n")
	}
	for {
		line, rest, more := strings.Cut(data, "\n")
		_, _ = b.WriteString("data: " + line + "\n")
		if !more {
			break
		}
		data = rest
	}
	_ = b.WriteByte('\n')
	return b.String()
}

// threadStreamOptions are what differs between the routes a thread stream is
// served on. None of them changes how the stream is read.
type threadStreamOptions struct {
	// passthrough is the API-key passthrough route: an upstream error event is
	// relayed as sent, and the client-visible usage is left as sent. A broken
	// Messages stream still ends in an explicit error on this route.
	passthrough bool
	// originalModel replaces mappedModel in what the client sees, when they
	// differ. timeoutModel is the model a stream timeout is recorded against.
	originalModel, mappedModel, timeoutModel string
}

const (
	threadStreamReadSize = 32 << 10
	// After the terminal event nothing more is interpreted or relayed. The
	// rest of the body is read for a moment so the connection can be reused.
	threadStreamDrainGrace = time.Second
	threadStreamDrainBytes = 64 << 10
)

// streamMessageThreadResponse relays the response stream of a thread request.
// Response headers have been written by the caller.
func (s *GatewayService) streamMessageThreadResponse(ctx context.Context, thread *MessageThread, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, opts threadStreamOptions) (*streamingResult, error) {
	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}

	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	limits := newSSEEventLimits(s.cfg, maxLineSize)
	decoder := newThreadSSEDecoder(limits.maxBytes, limits.maxLines)
	state := &threadStreamState{}
	usage := &ClaudeUsage{}
	var firstTokenMs *int
	clientDisconnected := false
	result := func() *streamingResult {
		return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}
	}

	// One goroutine reads the body so that a stalled upstream cannot keep the
	// timeout and keepalive below from running. Its queue is a few chunks.
	type chunk struct {
		data []byte
		err  error
	}
	chunks := make(chan chunk, 4)
	done := make(chan struct{})
	defer close(done)
	var lastReadAt atomic.Int64
	lastReadAt.Store(time.Now().UnixNano())
	go func() {
		defer close(chunks)
		buffer := make([]byte, threadStreamReadSize)
		for {
			n, err := resp.Body.Read(buffer)
			var next chunk
			if n > 0 {
				lastReadAt.Store(time.Now().UnixNano())
				next.data = append([]byte(nil), buffer[:n]...)
			}
			if err != nil && !errors.Is(err, io.EOF) {
				next.err = err
			}
			if n > 0 || next.err != nil {
				select {
				case chunks <- next:
				case <-done:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	streamInterval, keepaliveInterval := time.Duration(0), time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	if s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		keepaliveInterval = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}
	var intervalCh, keepaliveCh <-chan time.Time
	if streamInterval > 0 {
		ticker := time.NewTicker(streamInterval)
		defer ticker.Stop()
		intervalCh = ticker.C
	}
	if keepaliveInterval > 0 {
		ticker := time.NewTicker(keepaliveInterval)
		defer ticker.Stop()
		keepaliveCh = ticker.C
	}
	lastDataAt := time.Now()

	errorEventSent := false
	sendErrorEvent := func(reason, message string) {
		if errorEventSent || clientDisconnected {
			return
		}
		errorEventSent = true
		body, err := json.Marshal(map[string]any{"type": "error", "error": map[string]string{"type": reason, "message": message}})
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", body)
		flusher.Flush()
		MarkResponseCommitted(c) // no second, generic terminal error from the handler
	}
	write := func(block string) {
		if clientDisconnected || block == "" {
			return
		}
		if _, err := io.WriteString(w, block); err != nil {
			// The upstream is still read to the end: what it meters is billed.
			clientDisconnected = true
			logger.LegacyPrintf("service.gateway", "Client disconnected during a message thread stream, continuing to drain upstream for billing: account=%d", account.ID)
			return
		}
		flusher.Flush()
		lastDataAt = time.Now()
	}

	transform := newThreadStreamTransform(ctx, s, c, account, opts)
	// meter records what the upstream reports as metered. Usage fields are
	// cumulative: a later report replaces an earlier one, nothing is added up.
	meter := func(payload map[string]any) {
		if patch := s.extractSSEUsagePatch(payload); patch != nil {
			mergeSSEUsagePatch(usage, patch)
		}
		if payload["type"] != "message_start" || usage.OutputTokens != 0 {
			return
		}
		// message_start already reports output tokens: the count so far. A
		// response that ends before any message_delta is still billed for them.
		message, _ := payload["message"].(map[string]any)
		started, _ := message["usage"].(map[string]any)
		if reported, ok := parseSSEUsageInt(started["output_tokens"]); ok && reported > 0 && reported <= messageThreadMaxTokens {
			usage.OutputTokens = reported
		}
	}
	// meterUnfinished keeps usage that had already arrived in an event the
	// stream then broke off in. It changes nothing else: that event did not
	// happen, is not relayed, and completes nothing.
	meterUnfinished := func() {
		if payload, _ := decodeThreadEventPayload(decoder.unfinishedData()); payload != nil {
			meter(payload)
		}
	}

	// A false end closes client output immediately. Only complete deltas in
	// the rest of this Feed can still affect its bill; never fetch another
	// chunk or cross a message boundary to look for usage.
	meterFramedRemainder := func(frames []threadSSEEvent) {
		for _, frame := range frames {
			payload, ambiguous := decodeThreadEventPayload(frame.Data)
			kind, _ := payload["type"].(string)
			if kind == "message_stop" || kind == "message_start" || kind == "error" {
				return
			}
			if ambiguous || kind != "message_delta" || frame.Name != kind {
				continue
			}
			if transform.override {
				reported, _ := payload["usage"].(map[string]any)
				rewriteCacheCreationJSON(reported, transform.overrideTarget)
			}
			meter(payload)
		}
	}

	handle := func(event threadSSEEvent, remaining []threadSSEEvent) (terminal bool, err error) {
		payload, ambiguous := decodeThreadEventPayload(event.Data)
		terminal = state.observe(event, payload, ambiguous)
		if payload != nil {
			observer.ObserveAnthropic([]byte(event.Data))
		}
		kind, _ := payload["type"].(string)
		if !terminal && (event.Name == "error" || kind == "error") && !opts.passthrough {
			return false, &sseStreamErrorEventError{RawData: event.Data}
		}
		blocks := transform.apply(event, payload) // retain the route's cache TTL billing classification
		if payload != nil {
			meter(payload)
		}
		if terminal && state.invalid != "" {
			sendErrorEvent("api_error", "The upstream sent an incomplete or invalid message stream.")
			if event.Name == "message_stop" && !ambiguous && state.id != "" {
				// A named stop on an identified, still-in-message payload is
				// not the upstream's actual stop or the start of another bill.
				switch kind {
				case "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "ping":
					meterFramedRemainder(remaining)
				}
			}
			return false, fmt.Errorf("invalid thread stream: %s", state.invalid)
		}
		if errorEventSent {
			return false, nil // drain for billing after a relayed upstream error
		}
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		if terminal {
			// Bound before the client can read the end of the response, from
			// the same events the client is being handed.
			thread.bind(c.Request.Context(), state.threadResponseFacts, usage, clientDisconnected)
		}
		for _, block := range blocks {
			write(block)
		}
		if opts.passthrough && event.Name == "error" && payload["type"] == "error" {
			errorEventSent = true
			MarkResponseCommitted(c)
		}
		return terminal, nil
	}

	var events []threadSSEEvent
	finished := false
	var drainCh <-chan time.Time
	drained := 0
	for {
		select {
		case next, ok := <-chunks:
			if finished {
				drained += len(next.data)
				if !ok || next.err != nil || drained > threadStreamDrainBytes {
					return result(), nil
				}
				continue
			}
			if !ok {
				meterUnfinished()
				sendErrorEvent("api_error", "The upstream message stream ended before its terminal event.")
				return result(), fmt.Errorf("stream usage incomplete: missing terminal event")
			}
			var decodeErr error
			events, decodeErr = decoder.Feed(next.data, events[:0])
			for i, event := range events {
				terminal, err := handle(event, events[i+1:])
				if err != nil {
					if clientDisconnected {
						return result(), nil
					}
					// What the upstream metered before its error goes back with it.
					return result(), err
				}
				if terminal {
					finished = true
					drainCh = time.After(threadStreamDrainGrace)
					break
				}
			}
			if finished {
				continue
			}
			readErr := next.err
			if decodeErr != nil {
				readErr = decodeErr
			}
			if readErr == nil {
				continue
			}
			meterUnfinished()
			if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
				clientDisconnected = true
				return result(), fmt.Errorf("stream usage incomplete: %w", readErr)
			}
			if clientDisconnected {
				return result(), fmt.Errorf("stream usage incomplete after disconnect: %w", readErr)
			}
			if errors.Is(readErr, bufio.ErrTooLong) {
				logger.LegacyPrintf("service.gateway", "Message thread stream event too large: account=%d error=%v", account.ID, readErr)
				sendErrorEvent("response_too_large", "upstream SSE event exceeded the configured size")
				return result(), readErr
			}
			if !opts.passthrough && !c.Writer.Written() && !usage.hasObservedTokens() {
				// Nothing reached the client and nothing was metered. Reported
				// as an upstream failure; the handler decides, and never
				// resends a continue request. Once the upstream has reported
				// usage the request is settled here instead, with its error.
				logger.LegacyPrintf("service.gateway", "Message thread stream read error before any client output (account=%d): %v", account.ID, readErr)
				body, _ := json.Marshal(map[string]any{"type": "error", "error": map[string]string{
					"type": "upstream_disconnected", "message": "upstream stream disconnected: " + sanitizeStreamError(readErr),
				}})
				return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: body, RetryableOnSameAccount: true}
			}
			sendErrorEvent("stream_read_error", "upstream stream disconnected: "+sanitizeStreamError(readErr))
			return result(), fmt.Errorf("stream read error: %w", readErr)

		case <-drainCh:
			return result(), nil

		case <-intervalCh:
			if finished || time.Since(time.Unix(0, lastReadAt.Load())) < streamInterval {
				continue
			}
			meterUnfinished()
			if clientDisconnected {
				return result(), fmt.Errorf("stream usage incomplete after timeout")
			}
			logger.LegacyPrintf("service.gateway", "Message thread stream data interval timeout: account=%d model=%s interval=%s", account.ID, opts.timeoutModel, streamInterval)
			if s.rateLimitService != nil {
				s.rateLimitService.HandleStreamTimeout(ctx, account, opts.timeoutModel)
			}
			sendErrorEvent("stream_timeout", fmt.Sprintf("upstream stream idle for %s", streamInterval))
			return result(), fmt.Errorf("stream data interval timeout")

		case <-keepaliveCh:
			if finished || clientDisconnected || time.Since(lastDataAt) < keepaliveInterval {
				continue
			}
			// Only whole events are ever written, so a keepalive never lands
			// inside one.
			write(transform.keepalive())
		}
	}
}

// threadStreamTransform is what a route does to an event on its way to the
// client: the transforms the ordinary stream readers apply, as functions of a
// decoded event. It reads nothing from the wire and concludes nothing about
// the response.
type threadStreamTransform struct {
	opts threadStreamOptions

	overrideTarget string
	override       bool
	clientModel    string

	noopKeepalive bool
	noopIndex     int
	noopDeltaType string
}

func newThreadStreamTransform(ctx context.Context, s *GatewayService, c *gin.Context, account *Account, opts threadStreamOptions) *threadStreamTransform {
	t := &threadStreamTransform{
		opts: opts,

		clientModel: compositeClientModel(ctx, ""),
		noopIndex:   -1,
	}
	if !opts.passthrough {
		t.overrideTarget, t.override = s.resolveCacheTTLUsageOverrideTarget(ctx, account)
		t.noopKeepalive = c != nil && c.Request != nil && shouldUseClaudeCodeNoopDeltaKeepalive(c.GetHeader("User-Agent"))
	}
	return t
}

// apply returns what is written to the client for one upstream event. An
// event that is not changed keeps its own name, or its lack of one, and its
// own data.
func (t *threadStreamTransform) apply(event threadSSEEvent, payload map[string]any) []string {
	if payload == nil {
		return []string{formatThreadSSEEvent(event, event.Data)}
	}
	kind, _ := payload["type"].(string)
	changed := false
	t.trackKeepalive(kind, payload)

	usage, _ := payload["usage"].(map[string]any)
	message, _ := payload["message"].(map[string]any)
	if kind == "message_start" {
		usage, _ = message["usage"].(map[string]any)
	}
	if kind == "message_start" || kind == "message_delta" {
		if !t.opts.passthrough {
			changed = reconcileCachedTokens(usage)
		}
		if t.override && usage != nil {
			changed = rewriteCacheCreationJSON(usage, t.overrideTarget) || changed
		}
	}
	if model, ok := message["model"].(string); ok {
		if !t.opts.passthrough && t.opts.originalModel != t.opts.mappedModel && model == t.opts.mappedModel {
			message["model"], changed = t.opts.originalModel, true
		}
		if t.clientModel != "" && kind == "message_start" {
			message["model"], changed = t.clientModel, true
		}
	}

	data := event.Data
	if changed {
		if encoded, err := json.Marshal(payload); err == nil {
			data = string(encoded)
		}
	}
	return []string{formatThreadSSEEvent(event, data)}
}

// trackKeepalive remembers the content block a Claude Code client is in the
// middle of, for the empty delta that keeps such a client from timing out.
func (t *threadStreamTransform) trackKeepalive(kind string, payload map[string]any) {
	if !t.noopKeepalive {
		return
	}
	index, hasIndex := sseEventIndex(payload)
	switch kind {
	case "content_block_start":
		if !hasIndex {
			return
		}
		t.noopIndex, t.noopDeltaType = -1, ""
		block, _ := payload["content_block"].(map[string]any)
		blockType, _ := block["type"].(string)
		if deltaType := claudeCodeKeepaliveDeltaTypeForContentBlock(blockType); deltaType != "" {
			t.noopIndex, t.noopDeltaType = index, deltaType
		}
	case "content_block_delta":
		delta, _ := payload["delta"].(map[string]any)
		deltaType, _ := delta["type"].(string)
		if hasIndex && claudeCodeKeepaliveFieldForDeltaType(deltaType) != "" {
			t.noopIndex, t.noopDeltaType = index, deltaType
		}
	case "content_block_stop":
		if hasIndex && index == t.noopIndex {
			t.noopIndex, t.noopDeltaType = -1, ""
		}
	case "message_stop":
		t.noopIndex, t.noopDeltaType = -1, ""
	}
}

func (t *threadStreamTransform) keepalive() string {
	if t.noopKeepalive && t.noopIndex >= 0 {
		if block, ok := buildClaudeCodeNoopDeltaKeepalive(t.noopIndex, t.noopDeltaType); ok {
			return block
		}
	}
	return "event: ping\ndata: {\"type\": \"ping\"}\n\n"
}

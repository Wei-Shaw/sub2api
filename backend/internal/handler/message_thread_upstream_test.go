//go:build unit

package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// threadFakeUpstream is a local Messages endpoint that implements message
// threads the way the public documentation describes them, independently of
// the gateway's own bookkeeping. It is not a model of the whole API; it checks
// what the gateway's tests rely on:
//
//   - a thread belongs to the caller that created it;
//   - a response can be continued once it is complete, not while it is still
//     being generated, and not at all when the API did not keep it;
//   - only one continue request per response may run, and only one may succeed;
//   - a paused response is resumed with an empty message list and nothing else;
//   - system and tools are create-only, rejected on a continue request even as
//     null;
//   - model, tool_choice, thinking, mcp_servers (apart from each server's
//     authorization token), context_management, output_config, the skills in
//     container, the top-level cache_control and the set of anthropic-beta
//     values are fixed: a continue request must send the same JSON values, an
//     explicit null being a value; fallbacks is present on every request or none;
//   - at most four cache breakpoints, counting the top-level cache_control and,
//     on every continue request, those on the create request's system and tools;
//   - a one-hour breakpoint may not follow a shorter one, in the order tools,
//     system, messages, across everything the thread holds;
//   - a few rules of features a thread request may carry: a manual thinking
//     budget fits in max_tokens and allows no forced tool; an MCP server, a
//     skill and context management each need their beta and their tools;
//   - context editing that clears tool results is not available on a thread;
//   - a stream is message_start, content blocks each opened and closed under
//     their own index, message_delta, message_stop, every event named as its
//     payload type and terminated by a blank line. How an event is written on
//     the wire (line terminators, one or several data lines, comments, the
//     size of each write) is a matter of representation, see threadFakeWire.
type threadFakeUpstream struct {
	t    *testing.T
	name string
	srv  *httptest.Server

	mu         sync.Mutex
	principals map[string]string // credential → caller identity
	threads    map[string]*threadFakeState
	requests   []threadFakeRequest
	seq        int
	mode       string
	wire       threadFakeWire
	redirectTo string
	status     int
	// hold, when set, is closed to let a response in mode "hold" go on after
	// its message_start.
	hold chan struct{}
}

type threadFakeRequest struct {
	header http.Header
	body   []byte
}

type threadFakeState struct {
	principal string
	fixed     map[string]string // canonical JSON per fixed field, "" when absent
	beta      string
	fallbacks bool
	tools     []string // names, or types for tools without a name
	reserved  int      // breakpoints on the create request's system and tools
	shortTTL  bool     // the thread already holds a breakpoint shorter than one hour
	status    string   // threadFakePending, threadFakeComplete or threadFakePaused
	inFlight  bool     // a continue request from this response is running
	continued bool     // a continue request from this response succeeded
}

const (
	threadFakePending  = "pending"
	threadFakeComplete = "complete"
	threadFakePaused   = "paused"
)

// threadFakeFixedFields is the documented list of fixed fields besides the
// anthropic-beta set.
var threadFakeFixedFields = []string{
	"model", "tool_choice", "thinking", "mcp_servers", "context_management", "output_config", "container.skills", "cache_control",
}

type threadFakeRejection struct {
	status      int
	errType     string
	code        string
	message     string
	unsupported []string
	raw         string
}

const threadFakeStartOutputTokens = 3

func newThreadFakeUpstream(t *testing.T, name string) *threadFakeUpstream {
	u := &threadFakeUpstream{t: t, name: name, principals: map[string]string{}, threads: map[string]*threadFakeState{}}
	u.srv = httptest.NewServer(http.HandlerFunc(u.serve))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *threadFakeUpstream) setMode(mode string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.mode = mode
}

func (u *threadFakeUpstream) setWire(wire threadFakeWire) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.wire = wire
}

// threadFakeWire is how the fake writes a stream. The server-sent events
// format allows all of these for the same events: a client that reads the
// format reads the same response from each.
type threadFakeWire struct {
	newline  string // line terminator; "\n" when empty, or "\r\n", "\r"
	split    bool   // a JSON payload is spread over several data lines
	tight    bool   // no space after the colon of a field
	comments bool   // comment lines, and blocks that hold nothing but a comment
	fields   bool   // id and retry fields, and a field no client knows
	chunk    int    // bytes per write, each flushed; 0 writes an event at once
}

// threadFakeSplitJSON breaks a JSON object where JSON allows a line break:
// after its opening brace and after each of its top-level commas.
func threadFakeSplitJSON(data string) []string {
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

// render writes one event the way this wire does.
func (wire threadFakeWire) render(name, data string) string {
	newline, space := wire.newline, " "
	if newline == "" {
		newline = "\n"
	}
	if wire.tight {
		space = ""
	}
	var b strings.Builder
	line := func(text string) { b.WriteString(text + newline) }
	if wire.comments {
		line(": a comment is not an event")
		line("")
		line(":")
	}
	if wire.fields {
		line("id: 7")
		line("retry:" + space + "1000")
		line("future-field:" + space + "ignored")
		line("field-without-a-colon")
	}
	if name != "" {
		line("event:" + space + name)
	}
	lines := []string{data}
	if wire.split {
		lines = threadFakeSplitJSON(data)
	}
	for _, part := range lines {
		line("data:" + space + part)
	}
	line("")
	return b.String()
}

func (u *threadFakeUpstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.requests)
}

func (u *threadFakeUpstream) last() threadFakeRequest {
	u.mu.Lock()
	defer u.mu.Unlock()
	require.NotEmpty(u.t, u.requests)
	return u.requests[len(u.requests)-1]
}

func threadTestBetaTokens(h http.Header) []string {
	var tokens []string
	for _, value := range h.Values("Anthropic-Beta") {
		for _, token := range strings.Split(value, ",") {
			if token = strings.TrimSpace(token); token != "" {
				tokens = append(tokens, token)
			}
		}
	}
	slices.Sort(tokens)
	return slices.Compact(tokens)
}

func threadTestCanonicalBeta(h http.Header) string {
	return strings.Join(threadTestBetaTokens(h), ",")
}

// threadFakeCanonical compares JSON values, not bytes: key order, whitespace
// and the spelling of a number do not matter, different numbers always differ,
// and null is a value. An absent field is the empty string.
func threadFakeCanonical(field string, value gjson.Result) string {
	if !value.Exists() {
		return ""
	}
	decoder := json.NewDecoder(strings.NewReader(value.Raw))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return value.Raw
	}
	if field == "mcp_servers" {
		if servers, ok := decoded.([]any); ok {
			for _, server := range servers {
				if object, ok := server.(map[string]any); ok {
					delete(object, "authorization_token")
				}
			}
		}
	}
	return threadFakeRender(decoded)
}

func threadFakeRender(value any) string {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, key := range keys {
			parts[i] = fmt.Sprintf("%q:%s", key, threadFakeRender(v[key]))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = threadFakeRender(item)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case json.Number:
		return threadFakeNumber(v.String())
	default:
		encoded, _ := json.Marshal(v)
		return string(encoded)
	}
}

// threadFakeNumber writes a JSON number as sign, significant digits and a
// power of ten, by moving the decimal point in the text: no arithmetic, so no
// rounding. 1024, 1024.0 and 1.024e3 all become 1024e0.
func threadFakeNumber(literal string) string {
	sign := ""
	if strings.HasPrefix(literal, "-") {
		sign, literal = "-", literal[1:]
	}
	exponent := 0
	if i := strings.IndexAny(literal, "eE"); i >= 0 {
		if _, err := fmt.Sscanf(literal[i+1:], "%d", &exponent); err != nil {
			return sign + literal
		}
		literal = literal[:i]
	}
	if i := strings.IndexByte(literal, '.'); i >= 0 {
		exponent -= len(literal) - i - 1
		literal = literal[:i] + literal[i+1:]
	}
	digits := strings.TrimLeft(literal, "0")
	for strings.HasSuffix(digits, "0") {
		digits, exponent = digits[:len(digits)-1], exponent+1
	}
	if digits == "" {
		return "0"
	}
	return fmt.Sprintf("%s%se%d", sign, digits, exponent)
}

type threadFakeBreakpoints struct {
	tools, system, messages int
	topLevel                bool
	order                   []bool // true for a one-hour breakpoint, in evaluation order
}

func threadFakeCountBreakpoints(body []byte) threadFakeBreakpoints {
	var out threadFakeBreakpoints
	add := func(cc gjson.Result, counter *int) {
		if !cc.Exists() {
			return
		}
		*counter++
		out.order = append(out.order, cc.Get("ttl").Str == "1h")
	}
	gjson.GetBytes(body, "tools").ForEach(func(_, tool gjson.Result) bool {
		add(tool.Get("cache_control"), &out.tools)
		return true
	})
	if system := gjson.GetBytes(body, "system"); system.IsArray() {
		system.ForEach(func(_, block gjson.Result) bool {
			add(block.Get("cache_control"), &out.system)
			return true
		})
	}
	gjson.GetBytes(body, "messages").ForEach(func(_, message gjson.Result) bool {
		if content := message.Get("content"); content.IsArray() {
			content.ForEach(func(_, block gjson.Result) bool {
				add(block.Get("cache_control"), &out.messages)
				return true
			})
		}
		return true
	})
	if top := gjson.GetBytes(body, "cache_control"); top.Exists() {
		out.topLevel = true
		out.order = append(out.order, top.Get("ttl").Str == "1h")
	}
	return out
}

func threadFakeHasBeta(tokens []string, prefix string) bool {
	return slices.ContainsFunc(tokens, func(token string) bool { return strings.HasPrefix(token, prefix) })
}

// threadFakeFeatureRules are the rules of features a request may combine with
// a thread. tools are the tools of the thread: declared by this request, or
// kept from its create request.
func threadFakeFeatureRules(body []byte, betas, tools []string) string {
	if thinking := gjson.GetBytes(body, "thinking"); thinking.Get("type").Str == "enabled" {
		budget, maxTokens := thinking.Get("budget_tokens").Int(), gjson.GetBytes(body, "max_tokens").Int()
		if budget < 1024 {
			return "thinking.budget_tokens: must be at least 1024"
		}
		if budget >= maxTokens && !threadFakeHasBeta(betas, "interleaved-thinking-") {
			return "max_tokens: must be greater than thinking.budget_tokens"
		}
		if kind := gjson.GetBytes(body, "tool_choice.type").Str; kind == "any" || kind == "tool" {
			return "tool_choice: thinking may not be enabled when tool use is forced"
		}
	}
	if servers := gjson.GetBytes(body, "mcp_servers").Array(); len(servers) > 0 {
		if !threadFakeHasBeta(betas, "mcp-client-") {
			return "mcp_servers: Extra inputs are not permitted"
		}
		for _, server := range servers {
			if !slices.Contains(tools, "mcp_toolset:"+server.Get("name").Str) {
				return "mcp_servers: server " + server.Get("name").Str + " is not referenced by an mcp_toolset tool"
			}
		}
	}
	if len(gjson.GetBytes(body, "container.skills").Array()) > 0 {
		if !threadFakeHasBeta(betas, "skills-") || !threadFakeHasBeta(betas, "code-execution-") {
			return "container.skills: Extra inputs are not permitted"
		}
		if !slices.ContainsFunc(tools, func(tool string) bool { return strings.HasPrefix(tool, "code_execution") }) {
			return "container.skills: skills require the code execution tool"
		}
	}
	if gjson.GetBytes(body, "context_management").Exists() && !threadFakeHasBeta(betas, "context-management-") {
		return "context_management: Extra inputs are not permitted"
	}
	return ""
}

// threadFakeUnsupportedWithThread names a feature of the request that cannot
// be combined with a thread: the API keeps the conversation, so an edit that
// clears tool results from it is not available there.
func threadFakeUnsupportedWithThread(body []byte) string {
	unsupported := ""
	gjson.GetBytes(body, "context_management.edits").ForEach(func(_, edit gjson.Result) bool {
		if strings.HasPrefix(edit.Get("type").Str, "clear_tool_uses_") {
			unsupported = "context_management"
		}
		return unsupported == ""
	})
	return unsupported
}

// check applies the protocol to one request. It returns the state the response
// would be stored under and, for a continue request, the response it continues;
// or the rejection. It changes nothing.
func (u *threadFakeUpstream) check(principal string, header http.Header, body []byte) (*threadFakeState, *threadFakeState, *threadFakeRejection) {
	invalid := func(message string) *threadFakeRejection {
		return &threadFakeRejection{status: http.StatusBadRequest, errType: "invalid_request_error", message: message}
	}
	coded := func(code, message string, unsupported ...string) *threadFakeRejection {
		return &threadFakeRejection{status: http.StatusBadRequest, errType: "invalid_request_error", code: code, message: message, unsupported: unsupported}
	}
	thread := gjson.GetBytes(body, "thread")
	betas := threadTestBetaTokens(header)
	state := &threadFakeState{principal: principal, beta: strings.Join(betas, ","), fixed: map[string]string{},
		fallbacks: gjson.GetBytes(body, "fallbacks").Exists(), status: threadFakePending}
	for _, field := range threadFakeFixedFields {
		state.fixed[field] = threadFakeCanonical(field, gjson.GetBytes(body, field))
	}
	gjson.GetBytes(body, "tools").ForEach(func(_, tool gjson.Result) bool {
		switch {
		case tool.Get("type").Str == "mcp_toolset":
			state.tools = append(state.tools, "mcp_toolset:"+tool.Get("mcp_server_name").Str)
		case tool.Get("name").Str != "":
			state.tools = append(state.tools, tool.Get("name").Str)
		default:
			state.tools = append(state.tools, tool.Get("type").Str)
		}
		return true
	})
	breakpoints := threadFakeCountBreakpoints(body)
	if !thread.Exists() {
		if message := threadFakeFeatureRules(body, betas, state.tools); message != "" {
			return nil, nil, invalid(message)
		}
		return state, nil, nil
	}
	if !thread.IsObject() {
		return nil, nil, invalid("thread: must be an object")
	}
	if !slices.Contains(betas, threadTestBeta) {
		return nil, nil, invalid("thread: the " + threadTestBeta + " beta header is required")
	}
	total := breakpoints.tools + breakpoints.system + breakpoints.messages
	if breakpoints.topLevel {
		total++
	}
	var previous *threadFakeState
	switch thread.Get("type").Str {
	case "create":
		if thread.Get("previous_message_id").Exists() {
			return nil, nil, invalid("thread: previous_message_id is not allowed on a create request")
		}
		state.reserved = breakpoints.tools + breakpoints.system
	case "continue":
		id := thread.Get("previous_message_id")
		if id.Type != gjson.String || id.Str == "" {
			return nil, nil, invalid("thread: previous_message_id is required")
		}
		for _, field := range []string{"system", "tools"} {
			if gjson.GetBytes(body, field).Exists() {
				return nil, nil, invalid("thread: " + field + " is not allowed on a continue request")
			}
		}
		previous = u.threads[id.Str]
		// Unknown, someone else's, or still being generated.
		if u.mode == "thread404" || previous == nil || previous.principal != principal || previous.status == threadFakePending {
			return nil, nil, &threadFakeRejection{status: http.StatusNotFound, raw: threadTestNotFound}
		}
		var differing []string
		for _, field := range threadFakeFixedFields {
			if previous.fixed[field] != state.fixed[field] {
				differing = append(differing, field)
			}
		}
		if previous.beta != state.beta {
			differing = append(differing, "anthropic-beta")
		}
		if previous.fallbacks != state.fallbacks {
			differing = append(differing, "fallbacks")
		}
		if len(differing) > 0 {
			return nil, nil, coded("thread_fingerprint_mismatch", "thread: these fields differ from the stored thread: "+strings.Join(differing, ", "))
		}
		if previous.inFlight {
			return nil, nil, coded("thread_continue_in_flight", "thread: another continue request from this response is still running")
		}
		if previous.continued {
			return nil, nil, coded("thread_already_continued", "thread: this response was already continued")
		}
		messages := len(gjson.GetBytes(body, "messages").Array())
		if previous.status == threadFakePaused && messages > 0 {
			return nil, nil, invalid("messages: a paused turn is resumed with an empty list of messages")
		}
		if previous.status != threadFakePaused && messages == 0 {
			return nil, nil, invalid("messages: at least one message is required")
		}
		state.tools, state.reserved = previous.tools, previous.reserved
		total += previous.reserved
	default:
		return nil, nil, invalid("thread: type must be create or continue")
	}
	if choice := gjson.GetBytes(body, "tool_choice"); choice.Get("type").Str == "tool" && !slices.Contains(state.tools, choice.Get("name").Str) {
		return nil, nil, invalid("tool_choice: unknown tool " + choice.Get("name").Str)
	}
	if message := threadFakeFeatureRules(body, betas, state.tools); message != "" {
		return nil, nil, invalid(message)
	}
	if feature := threadFakeUnsupportedWithThread(body); feature != "" {
		return nil, nil, coded("thread_unsupported_request", "thread: "+feature+" is not supported when `thread` is set.", feature)
	}
	if total > 4 {
		return nil, nil, coded("thread_unsupported_request", fmt.Sprintf("thread: a maximum of 4 blocks with cache_control may be provided when `thread` is set. Found %d.", total), "cache_control")
	}
	short := previous != nil && previous.shortTTL
	for _, long := range breakpoints.order {
		if long && short {
			return nil, nil, invalid("cache_control: a ttl of 1h must not come after a shorter ttl")
		}
		short = short || !long
	}
	state.shortTTL = short
	return state, previous, nil
}

func (r *threadFakeRejection) write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(r.status)
	if r.raw != "" {
		_, _ = io.WriteString(w, r.raw)
		return
	}
	details := ""
	if r.code != "" {
		details = fmt.Sprintf(`,"details":{"error_code":%q`, r.code)
		if len(r.unsupported) > 0 {
			list, _ := json.Marshal(r.unsupported)
			details += `,"unsupported":` + string(list)
		}
		details += "}"
	}
	fmt.Fprintf(w, `{"type":"error","error":{"type":%q,"message":%q%s},"request_id":"req_fake"}`, r.errType, r.message, details)
}

func threadFakeError(w http.ResponseWriter, status int, errType, code, message string) {
	(&threadFakeRejection{status: status, errType: errType, code: code, message: message}).write(w)
}

// threadFakeUnstored are the responses the API serves but does not keep, and
// the ones this fake deliberately leaves unfinished or malformed.
var threadFakeUnstored = map[string]bool{
	"fallback": true, "truncated": true, "cut": true, "error-after-start": true, "eof-after-start": true,
	"unterminated-stop": true, "hang-unterminated": true, "mismatched-stop": true, "misnamed-stop": true,
	"untyped-stop": true, "unnamed-stop": true, "unclosed-block": true,
	"space-line-stop": true, "wrong-block-stop": true, "reopened-block": true, "stop-without-data": true,
	"block-gap": true, "truncated-reordered": true, "block-stop-named-ping": true, "block-stop-named-end": true,
	"no-content": true, "content-null": true, "content-object": true,
}

// threadFakeNonStreamModes change a non-streaming response only. In the first
// three the body is not a Message: content, a required list, is missing or is
// something else. empty-content is a Message with nothing in it, which is one.
// The last three are about the transport: json-read-error and json-cut break
// the connection before the declared end of the body, after the whole JSON
// document or in the middle of it; json-trailing-newline ends normally.
var threadFakeNonStreamModes = map[string]bool{
	"no-content": true, "content-null": true, "content-object": true, "empty-content": true,
	"json-read-error": true, "json-cut": true, "json-trailing-newline": true,
}

// threadFakeStreamOnlyModes change a streaming response only.
var threadFakeStreamOnlyModes = map[string]bool{
	"cut": true, "error-after-start": true, "eof-after-start": true, "unterminated-stop": true, "hang-unterminated": true,
	"mismatched-stop": true, "misnamed-stop": true, "untyped-stop": true, "unnamed-stop": true, "unclosed-block": true,
	"space-line-stop": true, "wrong-block-stop": true, "reopened-block": true, "stop-without-data": true,
	"block-gap": true, "truncated-reordered": true, "reordered": true, "block-stop-named-ping": true, "block-stop-named-end": true,
}

func (u *threadFakeUpstream) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	u.mu.Lock()
	u.requests = append(u.requests, threadFakeRequest{header: r.Header.Clone(), body: body})
	u.seq++
	id := fmt.Sprintf("msg_%s_%d", u.name, u.seq)
	mode, wire := u.mode, u.wire

	credential := r.Header.Get("X-Api-Key")
	if credential == "" {
		credential = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	principal, known := u.principals[credential]
	w.Header().Set("Request-Id", "req_"+id)
	reject := func(status int, errType, message string) {
		u.mu.Unlock()
		threadFakeError(w, status, errType, "", message)
	}
	switch {
	case mode == "redirect":
		target, status := u.redirectTo, u.status
		u.mu.Unlock()
		w.Header().Set("Location", target)
		w.WriteHeader(status)
		return
	case !known:
		reject(http.StatusUnauthorized, "authentication_error", "invalid credential")
		return
	case mode == "model404":
		reject(http.StatusNotFound, "not_found_error", "model: no-such-model")
		return
	case mode == "ratelimit":
		w.Header().Set("Retry-After", "30")
		reject(http.StatusTooManyRequests, "rate_limit_error", "slow down")
		return
	case mode == "overloaded":
		reject(529, "overloaded_error", "overloaded")
		return
	case mode == "forbidden":
		reject(http.StatusForbidden, "permission_error", "forbidden")
		return
	case mode == "server-error":
		reject(http.StatusInternalServerError, "api_error", "internal error")
		return
	case mode == "signature-once", mode == "signature-then-thread-error":
		u.mode = ""
		if mode == "signature-then-thread-error" {
			u.mode = "thread-error-raw"
		}
		reject(http.StatusBadRequest, "invalid_request_error", "messages.1.content.0: Invalid `signature` in `thinking` block")
		return
	case mode == "thread-error-raw":
		u.mode = ""
		u.mu.Unlock()
		(&threadFakeRejection{status: http.StatusBadRequest, raw: threadTestUnsupported}).write(w)
		return
	}

	state, previous, rejection := u.check(principal, r.Header, body)
	if rejection != nil {
		u.mu.Unlock()
		rejection.write(w)
		return
	}
	threaded := gjson.GetBytes(body, "thread").Exists()
	stream := gjson.GetBytes(body, "stream").Bool()
	model := gjson.GetBytes(body, "model").Str
	hold := u.hold
	if threaded {
		// The response has an id from its first event on, and is not a thread
		// to continue from until it is complete.
		u.threads[id] = state
		if previous != nil {
			previous.inFlight = true
		}
	}
	u.mu.Unlock()

	// finish settles the response: published as complete or paused, or dropped.
	finish := func(stored bool, status string) {
		u.mu.Lock()
		defer u.mu.Unlock()
		if !threaded {
			return
		}
		if previous != nil {
			previous.inFlight = false
			previous.continued = previous.continued || stored
		}
		if stored {
			state.status = status
		} else {
			delete(u.threads, id)
		}
	}
	stored, status := !threadFakeUnstored[mode], threadFakeComplete

	stopReason, block := "end_turn", `{"type":"text","text":"ok"}`
	if tool := threadFakeClientTool(state.tools); tool != "" {
		stopReason, block = "tool_use", fmt.Sprintf(`{"type":"tool_use","id":"toolu_1","name":%q,"input":{}}`, tool)
	}
	iterations := ""
	switch mode {
	case "refusal":
		stopReason, block = "refusal", `{"type":"text","text":""}`
	case "truncated":
		stopReason, block = "max_tokens", `{"type":"tool_use","id":"toolu_cut","name":"lookup","input":{}}`
	case "pause":
		stopReason, status = "pause_turn", threadFakePaused
	case "fallback":
		iterations = `,"iterations":[{"type":"fallback_message","model":"claude-haiku-4-5"}]`
	case "truncated-reordered":
		// Two blocks, the last of them a tool call the token limit cut off.
		stopReason = "max_tokens"
	}
	if !stream {
		content := `"content":[` + block + `],`
		switch mode {
		case "no-content":
			content = ""
		case "content-null":
			content = `"content":null,`
		case "content-object":
			content = `"content":` + block + `,`
		case "empty-content":
			content, stopReason = `"content":[],`, "end_turn"
		}
		finish(stored, status)
		payload := fmt.Sprintf(`{"id":%q,"type":"message","role":"assistant","model":%q,%s"stop_reason":%q,"usage":{"input_tokens":10000,"cache_read_input_tokens":80000,"cache_creation_input_tokens":1000,"output_tokens":20%s}}`,
			id, model, content, stopReason, iterations)
		w.Header().Set("Content-Type", "application/json")
		switch mode {
		case "json-read-error":
			// The whole document, and a body declared one byte longer than
			// what is sent: the connection ends before the body does.
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)+1))
		case "json-cut":
			// The connection ends in the middle of the document.
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			payload = payload[:len(payload)-30]
		case "json-trailing-newline":
			payload += "\n"
		}
		_, _ = io.WriteString(w, payload)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	flusher := w.(http.Flusher)
	write := func(text string) {
		step := wire.chunk
		if step <= 0 {
			step = len(text)
		}
		for len(text) > 0 {
			n := min(step, len(text))
			_, _ = io.WriteString(w, text[:n])
			flusher.Flush()
			text = text[n:]
		}
	}
	event := func(name, data string) { write(wire.render(name, data)) }
	event("message_start", fmt.Sprintf(`{"type":"message_start","message":{"id":%q,"type":"message","role":"assistant","content":[],"model":%q,"usage":{"input_tokens":10000,"cache_read_input_tokens":80000,"cache_creation_input_tokens":1000,"output_tokens":%d}}}`, id, model, threadFakeStartOutputTokens))
	if mode == "hold" && hold != nil {
		select {
		case <-hold:
		case <-r.Context().Done():
			finish(false, "")
			return
		}
	}
	switch mode {
	case "eof-after-start":
		finish(false, "")
		return
	case "error-after-start":
		event("error", `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
		finish(false, "")
		return
	}
	event("ping", `{"type":"ping"}`)
	switch mode {
	case "block-gap":
		// One whole block under index 1. Index 0 of the content never comes.
		event("content_block_start", fmt.Sprintf(`{"type":"content_block_start","index":1,"content_block":%s}`, block))
	case "truncated-reordered", "reordered":
		// The content is [text, tool call]; the tool call arrives first.
		event("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_late","name":"lookup","input":{}}}`)
		event("content_block_stop", `{"type":"content_block_stop","index":1}`)
		event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	default:
		event("content_block_start", fmt.Sprintf(`{"type":"content_block_start","index":0,"content_block":%s}`, block))
	}
	if mode == "cut" {
		finish(false, "")
		return
	}
	switch mode {
	case "block-gap":
		event("content_block_stop", `{"type":"content_block_stop","index":1}`)
	case "unclosed-block":
	case "block-stop-named-ping", "block-stop-named-end":
		name := "ping"
		if mode == "block-stop-named-end" {
			name = "message_stop"
		}
		event(name, `{"type":"content_block_stop","index":0}`)
	case "wrong-block-stop":
		// Block 0 is open; the stop names a block that never was.
		event("content_block_stop", `{"type":"content_block_stop","index":1}`)
	case "reopened-block":
		event("content_block_stop", `{"type":"content_block_stop","index":0}`)
		event("content_block_start", fmt.Sprintf(`{"type":"content_block_start","index":0,"content_block":%s}`, block))
		event("content_block_stop", `{"type":"content_block_stop","index":0}`)
	default:
		event("content_block_stop", `{"type":"content_block_stop","index":0}`)
	}
	event("message_delta", fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q},"usage":{"output_tokens":20%s}}`, stopReason, iterations))
	// Complete before the client can read message_stop.
	finish(stored, status)
	switch mode {
	case "unterminated-stop", "hang-unterminated":
		// The data line arrives, the blank line that ends the event does not.
		fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n")
		flusher.Flush()
		if mode == "hang-unterminated" {
			<-r.Context().Done()
		}
	case "mismatched-stop":
		event("message_stop", `{"type":"ping"}`)
	case "misnamed-stop":
		event("ping", `{"type":"message_stop"}`)
	case "untyped-stop":
		event("message_stop", `{}`)
	case "unnamed-stop":
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	case "space-line-stop":
		// A line holding one space is not the blank line that ends an event,
		// and the stream ends there.
		fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n \n")
		flusher.Flush()
	case "stop-without-data":
		// An event without a data field is not dispatched at all.
		fmt.Fprint(w, "event: message_stop\n\n")
		flusher.Flush()
	default:
		event("message_stop", `{"type":"message_stop"}`)
	}
}

// threadFakeClientTool is the first tool the model could call by name.
func threadFakeClientTool(tools []string) string {
	for _, tool := range tools {
		if !strings.HasPrefix(tool, "mcp_toolset:") && !strings.HasPrefix(tool, "code_execution") && !strings.HasPrefix(tool, "computer") {
			return tool
		}
	}
	return ""
}

// The fake has to be stricter than the gateway for the gateway's tests to mean
// anything. These tests drive it directly, with no gateway in between.

type threadFakeClient struct {
	t *testing.T
	u *threadFakeUpstream
}

func (c threadFakeClient) post(key, beta, body string) (int, string) {
	c.t.Helper()
	resp := c.open(key, beta, body)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func (c threadFakeClient) open(key, beta, body string) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.u.srv.URL+"/v1/messages", bytes.NewBufferString(body))
	require.NoError(c.t, err)
	req.Header.Set("X-Api-Key", key)
	if beta != "" {
		req.Header.Set("Anthropic-Beta", beta)
	}
	resp, err := c.u.srv.Client().Do(req)
	require.NoError(c.t, err)
	return resp
}

func (c threadFakeClient) ok(key, beta, body string) string {
	c.t.Helper()
	status, raw := c.post(key, beta, body)
	require.Equal(c.t, http.StatusOK, status, raw)
	return gjson.Get(raw, "id").Str
}

func threadFakeContinue(id, fields, messages string) string {
	if fields != "" {
		fields += ","
	}
	return `{` + fields + `"messages":` + messages + `,"thread":{"type":"continue","previous_message_id":"` + id + `"}}`
}

const threadFakeUser = `[{"role":"user","content":"more"}]`

func TestThreadFakeUpstreamFixedFields(t *testing.T) {
	u := newThreadFakeUpstream(t, "p")
	u.principals["key-1"], u.principals["key-2"] = "caller-1", "caller-2"
	c := threadFakeClient{t, u}
	beta := threadTestBeta + ",other-beta,context-management-2025-06-27"
	fixed := `"model":"m","max_tokens":4096,"tool_choice":{"type":"auto"},"thinking":{"type":"enabled","budget_tokens":1024},` +
		`"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},` +
		`"output_config":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"n":{"enum":[9007199254740993]}}}}},` +
		`"cache_control":{"type":"ephemeral"}`
	create := `{` + fixed + `,"system":"s","tools":[{"name":"lookup","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hi"}],"thread":{"type":"create"}}`
	first := c.ok("key-1", beta, create)

	// The same values in another spelling, and the documented free parts.
	respelled := `"cache_control":{"type":"ephemeral"},` +
		`"output_config":{"format":{"schema":{"properties":{"n":{"enum":[9.007199254740993e15]}},"type":"object"},"type":"json_schema"}},` +
		`"context_management":{"edits":[{"keep":"all","type":"clear_thinking_20251015"}]},` +
		`"thinking":{"budget_tokens":1024.0,"type":"enabled"},"tool_choice":{"type":"auto"},"model":"m","max_tokens":8192,"stop_sequences":["x"],"metadata":{"user_id":"u"}`
	second := c.ok("key-1", "other-beta, context-management-2025-06-27,"+threadTestBeta, threadFakeContinue(first, respelled, threadFakeUser))

	// Every fixed field on its own, a neighbouring large integer included.
	for field, changed := range map[string]string{
		"model":              strings.Replace(fixed, `"model":"m"`, `"model":"n"`, 1),
		"tool_choice":        strings.Replace(fixed, `{"type":"auto"}`, `{"type":"none"}`, 1),
		"thinking":           strings.Replace(fixed, `1024`, `2048`, 1),
		"context_management": strings.Replace(fixed, `"keep":"all"`, `"keep":"none"`, 1),
		"output_config":      strings.Replace(fixed, `9007199254740993`, `9007199254740992`, 1),
		"cache_control":      strings.Replace(fixed, `"cache_control":{"type":"ephemeral"}`, `"cache_control":{"type":"ephemeral","ttl":"1h"}`, 1),
	} {
		status, body := c.post("key-1", beta, threadFakeContinue(second, changed, threadFakeUser))
		require.Equal(t, http.StatusBadRequest, status, field)
		require.Equal(t, "thread_fingerprint_mismatch", gjson.Get(body, "error.details.error_code").Str, field+": "+body)
		require.Contains(t, gjson.Get(body, "error.message").Str, field)
	}
	// Dropping a field is a change, null is not the same as absent, and so is
	// a different beta set or the presence of fallbacks.
	for field, changed := range map[string]string{
		"tool_choice":   strings.Replace(fixed, `"tool_choice":{"type":"auto"},`, ``, 1),
		"output_config": strings.Replace(fixed, `{"format":{"type":"json_schema","schema":{"type":"object","properties":{"n":{"enum":[9007199254740993]}}}}}`, `null`, 1),
		"fallbacks":     fixed + `,"fallbacks":"default"`,
	} {
		_, body := c.post("key-1", beta, threadFakeContinue(second, changed, threadFakeUser))
		require.Contains(t, gjson.Get(body, "error.message").Str, field, body)
	}
	_, body := c.post("key-1", threadTestBeta+",context-management-2025-06-27", threadFakeContinue(second, fixed, threadFakeUser))
	require.Contains(t, gjson.Get(body, "error.message").Str, "anthropic-beta")

	// Create-only fields, even as null.
	for _, extra := range []string{`"system":"s"`, `"system":null`, `"tools":[]`, `"tools":null`} {
		status, body := c.post("key-1", beta, threadFakeContinue(second, fixed+","+extra, threadFakeUser))
		require.Equal(t, http.StatusBadRequest, status, extra)
		require.False(t, gjson.Get(body, "error.details").Exists(), extra)
	}

	// Another caller, an unknown id, a missing beta.
	status, body := c.post("key-2", beta, threadFakeContinue(second, fixed, threadFakeUser))
	require.Equal(t, http.StatusNotFound, status)
	require.Equal(t, threadTestNotFound, body)
	_, body = c.post("key-1", beta, threadFakeContinue("msg_unknown", fixed, threadFakeUser))
	require.Equal(t, threadTestNotFound, body)
	status, _ = c.post("key-1", "other-beta", threadFakeContinue(second, fixed, threadFakeUser))
	require.Equal(t, http.StatusBadRequest, status)

	// None of the rejected requests used up the response.
	third := c.ok("key-1", beta, threadFakeContinue(second, fixed, threadFakeUser))
	for _, used := range []string{first, second} {
		status, body = c.post("key-1", beta, threadFakeContinue(used, fixed, threadFakeUser))
		require.Equal(t, http.StatusBadRequest, status)
		require.Equal(t, "thread_already_continued", gjson.Get(body, "error.details.error_code").Str)
	}
	c.ok("key-1", beta, threadFakeContinue(third, fixed, threadFakeUser))
}

// Requests that are not valid on their own are not accepted just because they
// carry a thread.
func TestThreadFakeUpstreamFeatureRules(t *testing.T) {
	u := newThreadFakeUpstream(t, "p")
	u.principals["key-1"] = "caller-1"
	c := threadFakeClient{t, u}
	create := func(fields, tools string) string {
		return `{"model":"m",` + fields + `,"tools":` + tools + `,"messages":[{"role":"user","content":"hi"}],"thread":{"type":"create"}}`
	}
	lookup := `[{"name":"lookup","input_schema":{"type":"object"}}]`
	rejected := func(beta, body, want string) {
		t.Helper()
		status, raw := c.post("key-1", beta, body)
		require.Equal(t, http.StatusBadRequest, status, raw)
		require.Contains(t, gjson.Get(raw, "error.message").Str, want)
		require.False(t, gjson.Get(raw, "error.details").Exists())
	}

	// A manual thinking budget.
	thinking := `"thinking":{"type":"enabled","budget_tokens":2048}`
	rejected(threadTestBeta, create(`"max_tokens":64,`+thinking, lookup), "max_tokens")
	rejected(threadTestBeta, create(`"max_tokens":2048,`+thinking, lookup), "max_tokens")
	rejected(threadTestBeta, create(`"max_tokens":4096,"thinking":{"type":"enabled","budget_tokens":512}`, lookup), "budget_tokens")
	rejected(threadTestBeta, create(`"max_tokens":4096,"tool_choice":{"type":"any"},`+thinking, lookup), "tool_choice")
	id := c.ok("key-1", threadTestBeta, create(`"max_tokens":4096,`+thinking, lookup))
	rejected(threadTestBeta, threadFakeContinue(id, `"model":"m","max_tokens":2048,`+thinking, threadFakeUser), "max_tokens")
	c.ok("key-1", threadTestBeta, threadFakeContinue(id, `"model":"m","max_tokens":2049,`+thinking, threadFakeUser))
	c.ok("key-1", threadTestBeta+",interleaved-thinking-2025-05-14", create(`"max_tokens":64,`+thinking, lookup))

	// An MCP server needs its beta and a toolset that refers to it.
	mcp := `"max_tokens":64,"mcp_servers":[{"type":"url","url":"https://mcp.example.com","name":"docs","authorization_token":"t1"}]`
	toolset := `[{"type":"mcp_toolset","mcp_server_name":"docs"}]`
	mcpBeta := threadTestBeta + ",mcp-client-2025-11-20"
	rejected(threadTestBeta, create(mcp, toolset), "mcp_servers")
	rejected(mcpBeta, create(mcp, lookup), "mcp_toolset")
	id = c.ok("key-1", mcpBeta, create(mcp, toolset))
	c.ok("key-1", mcpBeta, threadFakeContinue(id, `"model":"m",`+strings.Replace(mcp, "t1", "t2", 1), threadFakeUser))
	// A tool that only has MCP in its name is an ordinary tool.
	c.ok("key-1", threadTestBeta, create(`"max_tokens":64`, `[{"name":"mcp__files__read","input_schema":{"type":"object"}}]`))

	// A skill needs its betas and the code execution tool.
	skills := `"max_tokens":64,"container":{"skills":[{"type":"anthropic","skill_id":"pdf","version":"latest"}]}`
	execution := `[{"type":"code_execution_20250825","name":"code_execution"}]`
	skillsBeta := threadTestBeta + ",skills-2025-10-02,code-execution-2025-08-25"
	rejected(threadTestBeta, create(skills, execution), "container.skills")
	rejected(skillsBeta, create(skills, lookup), "code execution")
	id = c.ok("key-1", skillsBeta, create(skills, execution))
	c.ok("key-1", skillsBeta, threadFakeContinue(id, `"model":"m",`+skills, threadFakeUser))

	// Context management needs its beta, and an edit that clears tool results
	// is not available on a thread at all.
	management := `"max_tokens":64,"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}`
	managementBeta := threadTestBeta + ",context-management-2025-06-27"
	rejected(threadTestBeta, create(management, lookup), "context_management")
	c.ok("key-1", managementBeta, create(management, lookup))
	clearing := `"max_tokens":64,"context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]}`
	status, body := c.post("key-1", managementBeta, create(clearing, lookup))
	require.Equal(t, http.StatusBadRequest, status, body)
	require.Equal(t, "thread_unsupported_request", gjson.Get(body, "error.details.error_code").Str)
	require.Equal(t, "context_management", gjson.Get(body, "error.details.unsupported.0").Str)
	// Without a thread it is an ordinary request.
	c.ok("key-1", "context-management-2025-06-27", strings.Replace(create(clearing, lookup), `,"thread":{"type":"create"}`, "", 1))
}

// The fake writes one stream in every representation the tests use; read by
// the format's rules they are the same events. This checks the two pieces the
// gateway's stream tests stand on against each other, with no gateway involved.
func TestThreadFakeUpstreamWire(t *testing.T) {
	u := newThreadFakeUpstream(t, "p")
	u.principals["key-1"] = "caller-1"
	c := threadFakeClient{t, u}
	body := `{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	read := func(wire threadFakeWire) (string, []string) {
		t.Helper()
		u.setWire(wire)
		status, raw := c.post("key-1", "", body)
		require.Equal(t, http.StatusOK, status)
		events := threadCompactEvents(t, threadReadClientEvents(raw))
		for i := range events {
			// The response id counts up; everything else is the same response.
			events[i] = strings.Replace(events[i], fmt.Sprintf("msg_p_%d", u.count()), "msg", 1)
		}
		return raw, events
	}
	raw, plain := read(threadFakeWire{})
	require.Len(t, plain, 6)
	require.True(t, strings.HasPrefix(plain[4], `message_delta {"delta":{"stop_reason":"end_turn"},"type":"message_delta","usage":{"output_tokens":20}}`), plain[4])
	require.Equal(t, `message_stop {"type":"message_stop"}`, plain[5])
	require.NotContains(t, raw, "\r")
	for name, wire := range threadTestWires {
		raw, events := read(wire)
		require.Equal(t, plain, events, name)
		if wire.newline != "" {
			require.NotContains(t, strings.ReplaceAll(raw, wire.newline, ""), "\n", name)
		}
		if wire.split {
			// The usage of the final delta is not on the event's first data line.
			_, delta, found := strings.Cut(raw, "message_delta")
			require.True(t, found)
			newline := wire.newline
			if newline == "" {
				newline = "\n"
			}
			first := strings.SplitN(delta, newline, 3)[1]
			require.True(t, strings.HasPrefix(first, "data:"), name)
			require.NotContains(t, first, "usage", name)
		}
	}
	// What only looks like the end of a stream is not one.
	for _, mode := range []string{"space-line-stop", "stop-without-data", "unterminated-stop"} {
		u.setWire(threadFakeWire{})
		u.setMode(mode)
		status, raw := c.post("key-1", "", body)
		u.setMode("")
		require.Equal(t, http.StatusOK, status)
		require.Contains(t, raw, "message_stop", mode)
		events := threadReadClientEvents(raw)
		require.Equal(t, "message_delta", events[len(events)-1].name, mode)
	}
}

func TestThreadFakeUpstreamCacheBreakpoints(t *testing.T) {
	u := newThreadFakeUpstream(t, "p")
	u.principals["key-1"] = "caller-1"
	c := threadFakeClient{t, u}
	cached := func(n int, ttl string) string {
		blocks := make([]string, n)
		for i := range blocks {
			blocks[i] = fmt.Sprintf(`{"type":"text","text":"b%d","cache_control":{"type":"ephemeral"%s}}`, i, ttl)
		}
		return `[{"role":"user","content":[` + strings.Join(blocks, ",") + `]}]`
	}
	thread := func(kind, id, top, system, messages string) string {
		body := `{"model":"m"` + top
		if system != "" {
			body += `,"system":` + system
		}
		body += `,"messages":` + messages + `,"thread":{"type":"` + kind + `"`
		if id != "" {
			body += `,"previous_message_id":"` + id + `"`
		}
		return body + "}}"
	}
	// Four breakpoints, counting the top-level one and, on a continue request,
	// those of the create request's system and tools.
	top := `,"cache_control":{"type":"ephemeral"}`
	system := `[{"type":"text","text":"s","cache_control":{"type":"ephemeral"}}]`
	_, body := c.post("key-1", threadTestBeta, thread("create", "", top, system, cached(3, "")))
	require.Equal(t, "thread_unsupported_request", gjson.Get(body, "error.details.error_code").Str)
	require.Equal(t, "cache_control", gjson.Get(body, "error.details.unsupported.0").Str)
	limited := c.ok("key-1", threadTestBeta, thread("create", "", top, system, cached(2, "")))
	_, body = c.post("key-1", threadTestBeta, thread("continue", limited, top, "", cached(3, "")))
	require.Equal(t, "thread_unsupported_request", gjson.Get(body, "error.details.error_code").Str)
	c.ok("key-1", threadTestBeta, thread("continue", limited, top, "", cached(2, "")))

	// One hour after something shorter, within a request and across the thread.
	hour := `,"ttl":"1h"`
	status, _ := c.post("key-1", threadTestBeta, thread("create", "", "", system, cached(1, hour)))
	require.Equal(t, http.StatusBadRequest, status)
	short := c.ok("key-1", threadTestBeta, thread("create", "", "", system, cached(1, "")))
	status, body = c.post("key-1", threadTestBeta, thread("continue", short, "", "", cached(1, hour)))
	require.Equal(t, http.StatusBadRequest, status, body)
	require.False(t, gjson.Get(body, "error.details").Exists())
	longSystem := `[{"type":"text","text":"s","cache_control":{"type":"ephemeral","ttl":"1h"}}]`
	long := c.ok("key-1", threadTestBeta, thread("create", "", "", longSystem, cached(1, hour)))
	c.ok("key-1", threadTestBeta, thread("continue", long, "", "", cached(1, hour)))
}

// A response is a thread to continue from once it is complete: not while it is
// being generated, not when it was never finished or not kept, and only once.
func TestThreadFakeUpstreamResponseLifecycle(t *testing.T) {
	u := newThreadFakeUpstream(t, "p")
	u.principals["key-1"] = "caller-1"
	c := threadFakeClient{t, u}
	create := func(stream bool) string {
		return fmt.Sprintf(`{"model":"m","stream":%t,"messages":[{"role":"user","content":"hi"}],"thread":{"type":"create"}}`, stream)
	}
	next := func(id string, stream bool) string {
		return threadFakeContinue(id, fmt.Sprintf(`"model":"m","stream":%t`, stream), threadFakeUser)
	}
	notFound := func(id string) {
		t.Helper()
		status, body := c.post("key-1", threadTestBeta, next(id, false))
		require.Equal(t, http.StatusNotFound, status, body)
		require.Equal(t, threadTestNotFound, body)
	}
	startedID := func(resp *http.Response) (string, *bufio.Reader) {
		t.Helper()
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			require.NoError(t, err)
			if data, ok := strings.CutPrefix(line, "data: "); ok && gjson.Get(data, "type").Str == "message_start" {
				return gjson.Get(data, "message.id").Str, reader
			}
		}
	}

	// A complete stream: every event is named as its payload type and ends
	// with a blank line, in the order of a Messages stream.
	status, body := c.post("key-1", threadTestBeta, create(true))
	require.Equal(t, http.StatusOK, status)
	var names []string
	for _, frame := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
		lines := strings.Split(frame, "\n")
		require.Len(t, lines, 2, frame)
		name := strings.TrimPrefix(lines[0], "event: ")
		require.Equal(t, name, gjson.Get(strings.TrimPrefix(lines[1], "data: "), "type").Str)
		names = append(names, name)
	}
	require.Equal(t, []string{"message_start", "ping", "content_block_start", "content_block_stop", "message_delta", "message_stop"}, names)
	parent := gjson.Get(strings.TrimPrefix(strings.Split(body, "\n")[1], "data: "), "message.id").Str
	require.NotEmpty(t, parent)

	// While a response is being generated: its id is known from message_start
	// and cannot be continued yet, and the response it continues is taken.
	u.mu.Lock()
	u.mode, u.hold = "hold", make(chan struct{})
	u.mu.Unlock()
	running := c.open("key-1", threadTestBeta, next(parent, true))
	defer func() { _ = running.Body.Close() }()
	child, rest := startedID(running)
	u.setMode("")
	notFound(child)
	status, body = c.post("key-1", threadTestBeta, next(parent, false))
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "thread_continue_in_flight", gjson.Get(body, "error.details.error_code").Str)
	close(u.hold)
	finished, err := io.ReadAll(rest)
	require.NoError(t, err)
	require.Contains(t, string(finished), "event: message_stop")
	// Complete: the child can be continued, the parent was continued.
	status, body = c.post("key-1", threadTestBeta, next(parent, false))
	require.Equal(t, "thread_already_continued", gjson.Get(body, "error.details.error_code").Str, status)
	grandchild := c.ok("key-1", threadTestBeta, next(child, false))

	// Responses that were not finished, or that the API does not keep, are
	// never a thread; the response they tried to continue is free again.
	for mode := range threadFakeUnstored {
		if mode == "hang-unterminated" {
			continue
		}
		for _, stream := range []bool{true, false} {
			if (stream && threadFakeNonStreamModes[mode]) || (!stream && threadFakeStreamOnlyModes[mode]) {
				continue
			}
			u.setMode(mode)
			resp := c.open("key-1", threadTestBeta, next(grandchild, stream))
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			u.setMode("")
			require.Equal(t, http.StatusOK, resp.StatusCode, mode)
			lost := gjson.Get(string(raw), "id").Str
			if stream {
				lost = gjson.Get(strings.TrimPrefix(strings.Split(string(raw), "\n")[1], "data: "), "message.id").Str
			}
			require.NotEmpty(t, lost, mode)
			notFound(lost)
		}
	}
	status, body = c.post("key-1", threadTestBeta, next(grandchild, true))
	require.Equal(t, http.StatusOK, status, body)

	// A paused turn is resumed with no messages, and with nothing else.
	u.setMode("pause")
	paused := c.ok("key-1", threadTestBeta, create(false))
	u.setMode("")
	status, body = c.post("key-1", threadTestBeta, next(paused, false))
	require.Equal(t, http.StatusBadRequest, status, body)
	require.Contains(t, gjson.Get(body, "error.message").Str, "paused")
	resumed := c.ok("key-1", threadTestBeta, threadFakeContinue(paused, `"model":"m"`, `[]`))
	// A complete response is not continued with an empty list.
	status, _ = c.post("key-1", threadTestBeta, threadFakeContinue(resumed, `"model":"m"`, `[]`))
	require.Equal(t, http.StatusBadRequest, status)
	c.ok("key-1", threadTestBeta, next(resumed, false))
}

// The response bodies the gateway's tests about broken responses stand on,
// read here by a plain HTTP client with no gateway in between.
func TestThreadFakeUpstreamBrokenResponses(t *testing.T) {
	u := newThreadFakeUpstream(t, "p")
	u.principals["key-1"] = "caller-1"
	c := threadFakeClient{t, u}
	create := func(stream bool) string {
		return fmt.Sprintf(`{"model":"m","stream":%t,"tools":[{"name":"lookup","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hi"}],"thread":{"type":"create"}}`, stream)
	}
	next := func(id string) string { return threadFakeContinue(id, `"model":"m"`, threadFakeUser) }
	read := func(mode string, stream bool) (string, error) {
		t.Helper()
		u.setMode(mode)
		resp := c.open("key-1", threadTestBeta, create(stream))
		u.setMode("")
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode, mode)
		raw, err := io.ReadAll(resp.Body)
		return string(raw), err
	}
	published := func(id string, want bool) {
		t.Helper()
		status, body := c.post("key-1", threadTestBeta, next(id))
		if want {
			require.Equal(t, http.StatusOK, status, body)
		} else {
			require.Equal(t, http.StatusNotFound, status, body)
		}
	}

	// The connection ends one byte before the declared end of the body. What
	// did arrive is the whole Message, with its usage; the API did its work.
	raw, err := read("json-read-error", false)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.True(t, json.Valid([]byte(raw)), raw)
	require.Equal(t, int64(20), gjson.Get(raw, "usage.output_tokens").Int())
	published(gjson.Get(raw, "id").Str, true)
	// The connection ends inside the document: nothing to read a number from.
	raw, err = read("json-cut", false)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.False(t, json.Valid([]byte(raw)))
	// A newline after the document is an ordinary response.
	raw, err = read("json-trailing-newline", false)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(raw, "}\n"))
	published(gjson.Get(raw, "id").Str, true)

	// A body without the content list is not a Message, and no thread.
	for mode, content := range map[string]string{"no-content": "", "content-null": "null", "content-object": "{"} {
		raw, err = read(mode, false)
		require.NoError(t, err)
		require.True(t, json.Valid([]byte(raw)), mode)
		require.Equal(t, content != "", gjson.Get(raw, "content").Exists(), mode)
		require.False(t, gjson.Get(raw, "content").IsArray(), mode)
		require.True(t, strings.HasPrefix(gjson.Get(raw, "content").Raw, content), mode)
		require.Equal(t, int64(20), gjson.Get(raw, "usage.output_tokens").Int(), mode)
		published(gjson.Get(raw, "id").Str, false)
	}
	// An empty list is content.
	raw, err = read("empty-content", false)
	require.NoError(t, err)
	require.Equal(t, `[]`, gjson.Get(raw, "content").Raw)
	published(gjson.Get(raw, "id").Str, true)

	// Streams. The index of a block is its place in the content list.
	indexes := func(raw string) (started []int64, id string) {
		for _, event := range threadReadClientEvents(raw) {
			switch event.name {
			case "message_start":
				id = gjson.Get(event.data, "message.id").Str
			case "content_block_start":
				started = append(started, gjson.Get(event.data, "index").Int())
			}
		}
		return started, id
	}
	raw, err = read("block-gap", true)
	require.NoError(t, err)
	started, id := indexes(raw)
	require.Equal(t, []int64{1}, started, "index 0 of the content never comes")
	require.Contains(t, raw, "event: message_stop")
	published(id, false)

	raw, err = read("reordered", true)
	require.NoError(t, err)
	started, id = indexes(raw)
	require.Equal(t, []int64{1, 0}, started, "a whole content list, its blocks in another order")
	published(id, true)

	raw, err = read("truncated-reordered", true)
	require.NoError(t, err)
	started, id = indexes(raw)
	require.Equal(t, []int64{1, 0}, started)
	require.Contains(t, raw, `"stop_reason":"max_tokens"`)
	require.Contains(t, raw, `"index":1,"content_block":{"type":"tool_use"`, "the last block of the content is the tool call")
	published(id, false)
}

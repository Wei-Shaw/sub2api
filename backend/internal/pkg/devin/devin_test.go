// Tests for the Oh My Pi (MIT) adaptation, source commit
// 4049c62fbb0a7f5068954d01f11a73112c942a11. See root THIRD_PARTY_NOTICES.md.
package devin

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func mustProto(t *testing.T, b []byte) protoMessage {
	t.Helper()
	m, err := parseProto(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func readRequest(t *testing.T, r *http.Request) protoMessage {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "POST" || r.Header.Get("Connect-Protocol-Version") != "1" || r.Header.Get("Authorization") != "" {
		t.Fatalf("unexpected HTTP request: %s %v", r.Method, r.Header)
	}
	if r.URL.Path == ChatPath {
		if r.Header.Get("Content-Type") != "application/connect+proto" || r.Header.Get("Connect-Content-Encoding") != "gzip" || r.Header.Get("Connect-Accept-Encoding") != "gzip" || r.Header.Get("Accept-Encoding") != "identity" {
			t.Fatalf("bad Connect headers: %v", r.Header)
		}
		if len(b) < 5 || b[0] != 1 || int(binary.BigEndian.Uint32(b[1:5])) != len(b)-5 {
			t.Fatal("request not gzip Connect framed")
		}
		b, err = inflate(b[5:])
		if err != nil {
			t.Fatal(err)
		}
	} else if r.Header.Get("Content-Type") != "application/proto" {
		t.Fatal("unary request should be unframed protobuf")
	}
	return mustProto(t, b)
}
func responseBytes(status int, b []byte) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(b))}
}
func successFrames(messages ...[]byte) []byte {
	var out []byte
	for _, m := range messages {
		out = append(out, connectFrame(0, m)...)
	}
	return append(out, connectFrame(2, []byte(`{}`))...)
}
func zipped(t *testing.T, b []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	if _, err := w.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func assertStatus(t *testing.T, err error, status int) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.StatusCode != status {
		t.Fatalf("got error %v, want typed status %d", err, status)
	}
	return e
}
func baseClient(t *testing.T, chat func(*http.Request) (*http.Response, error)) *Client {
	t.Helper()
	return &Client{Do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == authPath {
			readRequest(t, r)
			return responseBytes(200, ps(nil, 1, "user-jwt-secret")), nil
		}
		return chat(r)
	}}
}

const simpleBody = `{"model":"swe-1-6","messages":[{"role":"user","content":"hello"}]}`
const streamBody = `{"model":"swe-1-6","stream":true,"messages":[{"role":"user","content":"hello"}]}`

func TestMetadataAndRequestMapping(t *testing.T) {
	body := []byte(`{"model":"public-model","instructions":"instructions","max_completion_tokens":123,"temperature":0.7,"top_p":0.9,"stop":["END"],"parallel_tool_calls":false,
 "messages":[{"role":"system","content":[{"type":"text","text":"system"}]},{"role":"developer","content":"developer"},{"role":"user","content":"user"},
 {"role":"assistant","content":"answer","reasoning_content":"reasoning","tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{\"q\":1}"}}]},
 {"role":"tool","tool_call_id":"call-1","content":[{"type":"text","text":"result"}]}],
 "tools":[{"type":"function","function":{"name":"lookup","description":"Find data","parameters":{"type":"object","properties":{"q":{"type":"integer"}}},"strict":true}}],"tool_choice":{"type":"function","function":{"name":"lookup"}}}`)
	calls := 0
	c := &Client{Do: func(r *http.Request) (*http.Response, error) {
		calls++
		m := readRequest(t, r)
		meta := mustProto(t, m.data(1))
		if meta.text(1) != "devin-cli" || meta.text(28) != "chisel" || meta.text(7) != "3000.6.2" || meta.text(12) != "chisel" || meta.text(2) != "3000.6.2" || meta.text(4) != "en" || meta.text(3) != "devin-session-token$session-secret" {
			t.Fatalf("bad metadata: %#v", meta)
		}
		if r.URL.String() != DefaultBaseURL+r.URL.Path {
			t.Fatalf("nondefault auth/chat base: %s", r.URL)
		}
		if r.URL.Path == authPath {
			if meta.text(21) != "" {
				t.Fatal("JWT must not be present in auth request")
			}
			// Literal fixture locks GetUserJwtResponse.user_jwt to field 1.
			b, _ := hex.DecodeString("0a036a7774")
			return responseBytes(200, b), nil
		}
		if r.URL.Path != ChatPath || meta.text(21) != "jwt" {
			t.Fatal("chat lacks both session token and JWT")
		}
		if m.text(2) != "instructions\n\nsystem" || m.text(21) != "wire-model" || m.text(16) != "cascade" || m.text(22) == "" || m.num(7) != 5 || m.num(20) != 1 || m.num(11) != 1 {
			t.Fatalf("bad chat request: %#v", m)
		}
		cfg := mustProto(t, m.data(8))
		if cfg.num(1) != 1 || cfg.num(2) != 123 || cfg.num(3) != 200 || cfg.num(7) != 50 || math.Float64frombits(cfg.num(5)) != 0.7 || math.Float64frombits(cfg.num(6)) != 0.7 || math.Float64frombits(cfg.num(8)) != 0.9 || math.Float64frombits(cfg.num(11)) != 1 {
			t.Fatal("configuration field mapping changed")
		}
		var stops []string
		for _, f := range cfg[9] {
			stops = append(stops, string(f.b))
		}
		want := []string{"<|user|>", "<|bot|>", "<|context_request|>", "<|endoftext|>", "<|end_of_turn|>", "END"}
		if !reflect.DeepEqual(stops, want) {
			t.Fatalf("stop patterns: %v", stops)
		}
		if mustProto(t, m.data(12)).text(2) != "lookup" || mustProto(t, m.data(13)).num(1) != 1 {
			t.Fatal("tool choice/cache mapping changed")
		}
		if len(m[3]) != 4 {
			t.Fatalf("got %d history entries", len(m[3]))
		}
		sources := []uint64{1, 1, 2, 4}
		texts := []string{"developer", "user", "answer", "result"}
		for i, f := range m[3] {
			p := mustProto(t, f.b)
			if p.num(2) != sources[i] || p.text(3) != texts[i] || p.text(1) == "" {
				t.Fatalf("history[%d] incorrect", i)
			}
		}
		assistant := mustProto(t, m[3][2].b)
		if assistant.text(11) != "reasoning" {
			t.Fatal("reasoning history omitted")
		}
		tc := mustProto(t, assistant.data(6))
		if tc.text(1) != "call-1" || tc.text(2) != "lookup" || tc.text(3) != `{"q":1}` {
			t.Fatal("tool history lost")
		}
		if mustProto(t, m[3][3].b).text(7) != "call-1" {
			t.Fatal("tool result not linked")
		}
		tool := mustProto(t, m.data(10))
		if tool.text(1) != "lookup" || tool.text(2) != "Find data" || tool.text(3) != `{"type":"object","properties":{"q":{"type":"integer"}}}` || tool.num(12) != 1 {
			t.Fatal("tool schema string mapping changed")
		}
		return responseBytes(200, successFrames(ps(nil, 3, "ok"))), nil
	}}
	resp, err := c.Chat(context.Background(), "session-secret", "cascade", Model{ID: "public-model", UID: "wire-model"}, body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestStableHistoryAndDefaultConfig(t *testing.T) {
	one, err := mapRequest([]byte(simpleBody), "cascade-1", DefaultModels()[0])
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(simpleBody, "hello", "edited", 1)
	two, err := mapRequest([]byte(changed), "cascade-1", DefaultModels()[0])
	if err != nil {
		t.Fatal(err)
	}
	three, err := mapRequest([]byte(simpleBody), "cascade-2", DefaultModels()[0])
	if err != nil {
		t.Fatal(err)
	}
	id := func(m *mappedRequest) string { return mustProto(t, mustProto(t, m.wire).data(3)).text(1) }
	if id(one) != id(two) || id(one) == id(three) {
		t.Fatal("history IDs must be stable across edits, scoped to cascade")
	}
	m := mustProto(t, one.wire)
	cfg := mustProto(t, m.data(8))
	if cfg.num(2) != 64000 || math.Float64frombits(cfg.num(5)) != 0.4 || math.Float64frombits(cfg.num(8)) != 1 || m.num(11) != 1 || mustProto(t, m.data(12)).text(1) != "auto" {
		t.Fatal("incorrect default configuration")
	}
	// Prefixing must be idempotent.
	if !bytes.Equal(metadata("token", "", false), metadata("devin-session-token$token", "", false)) {
		t.Fatal("token double-prefix")
	}
	defaults := DefaultModels()
	defaults[0].ID = "changed"
	if DefaultModels()[0].ID != "swe-1-6" {
		t.Fatal("defaults shared mutable slice")
	}
}

func modelFixture(uid, label string, disabled bool, info []byte) []byte {
	b := ps(ps(nil, 22, uid), 1, label)
	if disabled {
		b = pv(b, 4, 1)
	}
	b = pv(b, 5, 1)
	return pb(b, 23, info)
}
func TestDiscovery(t *testing.T) {
	features := pv(pv(pv(nil, 11, 1), 12, 1), 21, 1)
	info := pb(nil, 6, features)
	info = pv(info, 13, 8192)
	var catalog []byte
	for _, cfg := range [][]byte{
		modelFixture("swe-1-6", "SWE", false, info), modelFixture("swe-1-6-fast", "SWE Fast", false, info),
		modelFixture("frontier-high", "Frontier High", false, info), modelFixture("frontier-low", "Frontier Low", false, info),
		modelFixture("adaptive", "Adaptive", false, pv(nil, 22, 3)), modelFixture("also-router", "", false, pv(nil, 25, 1)),
		modelFixture("disabled", "Disabled", true, info), modelFixture("internal", "Internal", false, pv(nil, 2, 1)),
		modelFixture("review", "Review", false, pv(nil, 22, 4)), modelFixture("internal-slot", "Slot", false, pv(nil, 22, 6)),
		modelFixture("frontier-high", "Duplicate", false, info), modelFixture(" ", "Blank", false, info),
	} {
		catalog = pb(catalog, 1, cfg)
	}
	c := &Client{Do: func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != DefaultBaseURL+modelsPath {
			t.Fatal(r.URL)
		}
		m := readRequest(t, r)
		meta := mustProto(t, m.data(1))
		if meta.text(1) != "chisel" || meta.text(2) != "0.0.0-dev" || meta.text(7) != "0.0.0-dev" || meta.text(28) != "" || meta.text(21) != "" || meta.text(3) != "devin-session-token$token" || !bytes.Equal(meta.data(30), []byte{3, 4, 6, 7, 8}) {
			t.Fatal("wrong native discovery identity")
		}
		return responseBytes(200, zipped(t, catalog)), nil
	}}
	models, err := c.Models(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 7 {
		t.Fatalf("models: %#v", models)
	}
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
		if m.UID != m.ID {
			t.Fatal("wire UID lost")
		}
		if strings.HasPrefix(m.ID, "swe-") && m.SupportsImages {
			t.Fatal("SWE-1.6 incorrectly advertises vision")
		}
		if strings.HasPrefix(m.ID, "frontier-") && (!m.SupportsImages || !m.SupportsParallelTools || m.MaxTokens != 8192) {
			t.Fatal("features lost")
		}
	}
	want := []string{"adaptive", "also-router", "frontier-high", "frontier-low", "internal", "swe-1-6", "swe-1-6-fast"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("IDs: %v", ids)
	}
	if !models[0].Router || !models[1].Router || models[1].DisplayName != "also-router" {
		t.Fatal("router detection failed")
	}
	serialized, _ := json.Marshal(models[2])
	if strings.Contains(string(serialized), "UID") || strings.Contains(string(serialized), "supports") {
		t.Fatal("private wire capabilities leaked")
	}
}

func TestRouterAndValidatedCustomEndpoint(t *testing.T) {
	paths := []string{}
	validated := false
	c := &Client{ValidateBaseURL: func(base string) (string, error) {
		if base != "https://approved.example/" {
			t.Fatal(base)
		}
		validated = true
		return base, nil
	}}
	c.Do = func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		m := readRequest(t, r)
		if r.URL.Path == authPath {
			if r.URL.Host != "server.codeium.com" {
				t.Fatal(r.URL)
			}
			return responseBytes(200, ps(ps(nil, 1, "jwt"), 2, "https://approved.example/")), nil
		}
		if !validated || r.URL.Host != "approved.example" {
			t.Fatal("custom endpoint was not validated first")
		}
		if r.URL.Path == assignPath {
			meta := mustProto(t, m.data(1))
			if meta.text(21) != "" || meta.text(3) != "devin-session-token$token" || m.text(2) != "adaptive" || m.text(3) != "cascade" {
				t.Fatal("assignment metadata incorrect")
			}
			p := mustProto(t, m.data(5))
			if p.text(3) != "last developer" || p.text(1) != "" || p.num(2) != 1 {
				t.Fatal("router must score last user/developer with empty message ID")
			}
			return responseBytes(200, pb(nil, 1, ps(ps(nil, 1, "assignment-secret"), 2, "concrete"))), nil
		}
		if m.text(21) != "concrete" || m.text(26) != "assignment-secret" || m.text(16) != "cascade" || mustProto(t, m.data(1)).text(21) != "jwt" {
			t.Fatal("assignment not carried into chat")
		}
		return responseBytes(200, successFrames(ps(ps(nil, 1, "actual-id"), 23, "actual-fallback"))), nil
	}
	body := `{"messages":[{"role":"user","content":"old"},{"role":"developer","content":"last developer"},{"role":"assistant","content":"history"}]}`
	resp, err := c.Chat(context.Background(), "token", "cascade", Model{ID: "adaptive", Router: true}, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	var out apicompat.ChatCompletionsResponse
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if out.ID != "actual-id" || out.Model != "actual-fallback" {
		t.Fatal("actual server observations lost")
	}
	if !reflect.DeepEqual(paths, []string{authPath, assignPath, ChatPath}) {
		t.Fatal(paths)
	}
}

func TestRouterFailsClosed(t *testing.T) {
	for _, assignment := range [][]byte{nil, pb(nil, 1, ps(nil, 1, "jwt-only")), pb(nil, 1, ps(nil, 2, "model-only")), pb(nil, 1, ps(ps(nil, 1, "jwt"), 2, "adaptive"))} {
		t.Run(hex.EncodeToString(assignment), func(t *testing.T) {
			calls := 0
			c := baseClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path != assignPath {
					t.Fatal("chat called after invalid assignment")
				}
				return responseBytes(200, assignment), nil
			})
			_, err := c.Chat(context.Background(), "token", "cascade", Model{ID: "adaptive", Router: true}, []byte(simpleBody))
			assertStatus(t, err, 502)
			if calls != 1 {
				t.Fatal(calls)
			}
		})
	}
}

func TestCustomEndpointFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		validate func(string) (string, error)
	}{
		{"missing validator", nil}, {"rejected", func(string) (string, error) { return "", errors.New("secret rejected url") }},
		{"insecure", func(s string) (string, error) { return "http://localhost", nil }},
		{"userinfo", func(s string) (string, error) { return "https://secret@example.com", nil }},
		{"query", func(s string) (string, error) { return "https://example.com?secret=1", nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := &Client{ValidateBaseURL: tc.validate, Do: func(r *http.Request) (*http.Response, error) {
				calls++
				return responseBytes(200, ps(ps(nil, 1, "jwt"), 2, "https://untrusted.example")), nil
			}}
			_, err := c.Chat(context.Background(), "token", "cascade", DefaultModels()[0], []byte(simpleBody))
			assertStatus(t, err, 502)
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "untrusted") || calls != 1 {
				t.Fatalf("unsafe error or request: %v calls=%d", err, calls)
			}
		})
	}
}

type chunkBody struct {
	r      *bytes.Reader
	size   int
	closed bool
}

func (b *chunkBody) Read(p []byte) (int, error) {
	if len(p) > b.size {
		p = p[:b.size]
	}
	return b.r.Read(p)
}
func (b *chunkBody) Close() error { b.closed = true; return nil }
func outputFixture(t *testing.T) []byte {
	t.Helper()
	first := ps(ps(ps(ps(nil, 1, "response-id"), 23, "observed-model"), 9, "think"), 3, "hello")
	call := ps(ps(ps(nil, 1, "tool-1"), 2, "lookup"), 3, `{"q":`)
	second := pb(nil, 6, call)
	snapshot := ps(ps(nil, 1, "tool-1"), 3, `{"q":1`)
	third := pb(nil, 6, snapshot)
	incremental := pb(nil, 6, ps(nil, 3, `}`)) // no ID must attach to the active call
	usage := pv(pv(pv(pv(nil, 2, 2), 3, 3), 4, 5), 5, 7)
	last := pv(pb(nil, 7, usage), 5, 10)
	var out []byte
	for i, m := range [][]byte{first, second, third, incremental, last} {
		if i%2 == 0 {
			out = append(out, connectFrame(1, zipped(t, m))...)
		} else {
			out = append(out, connectFrame(0, m)...)
		}
	}
	return append(out, connectFrame(3, zipped(t, []byte(`{"metadata":{}}`)))...)
}
func TestOutputJSONAndSSE(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, chunkSize := range []int{1, 2, 7, 4096} {
			t.Run(string(rune('0'+chunkSize))+fmtBool(stream), func(t *testing.T) {
				fixture := outputFixture(t)
				upstream := &chunkBody{r: bytes.NewReader(fixture), size: chunkSize}
				c := baseClient(t, func(r *http.Request) (*http.Response, error) {
					readRequest(t, r)
					return &http.Response{StatusCode: 200, Body: upstream}, nil
				})
				body := simpleBody
				if stream {
					body = streamBody
				}
				resp, err := c.Chat(context.Background(), "token", "cascade", DefaultModels()[0], []byte(body))
				if err != nil {
					t.Fatal(err)
				}
				b, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if !upstream.closed {
					t.Fatal("upstream body leaked")
				}
				var usage *apicompat.ChatUsage
				if !stream {
					if resp.Header.Get("Content-Type") != "application/json" {
						t.Fatal(resp.Header)
					}
					var out apicompat.ChatCompletionsResponse
					if err = json.Unmarshal(b, &out); err != nil {
						t.Fatal(err)
					}
					msg := out.Choices[0].Message
					if out.ID != "response-id" || out.Model != "observed-model" || string(msg.Content) != `"hello"` || msg.ReasoningContent != "think" || out.Choices[0].FinishReason != "tool_calls" || len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "tool-1" || msg.ToolCalls[0].Function.Name != "lookup" || msg.ToolCalls[0].Function.Arguments != `{"q":1}` {
						t.Fatalf("incorrect output: %s", b)
					}
					usage = out.Usage
				} else {
					if resp.Header.Get("Content-Type") != "text/event-stream" || !strings.HasSuffix(string(b), "data: [DONE]\n\n") {
						t.Fatalf("not successful SSE: %s", b)
					}
					var text, reasoning, args, names string
					finish := ""
					tools := 0
					for _, line := range strings.Split(string(b), "\n") {
						if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
							continue
						}
						var out apicompat.ChatCompletionsChunk
						if err = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &out); err != nil {
							t.Fatal(err)
						}
						if out.ID != "response-id" || out.Model != "observed-model" {
							t.Fatal("observations lost in SSE")
						}
						d := out.Choices[0].Delta
						if d.Content != nil {
							text += *d.Content
						}
						if d.ReasoningContent != nil {
							reasoning += *d.ReasoningContent
						}
						for _, tc := range d.ToolCalls {
							if tc.Index == nil || *tc.Index != 0 {
								t.Fatal("tool index lost")
							}
							if tc.ID != "" {
								tools++
							}
							args += tc.Function.Arguments
							names += tc.Function.Name
						}
						if out.Choices[0].FinishReason != nil {
							finish = *out.Choices[0].FinishReason
						}
						if out.Usage != nil {
							usage = out.Usage
						}
					}
					if text != "hello" || reasoning != "think" || args != `{"q":1}` || names != "lookup" || tools != 1 || finish != "tool_calls" {
						t.Fatalf("incorrect deltas: %s", b)
					}
				}
				if usage == nil || usage.PromptTokens != 14 || usage.CompletionTokens != 3 || usage.TotalTokens != 17 || usage.PromptTokensDetails.CachedTokens != 7 || usage.PromptTokensDetails.CacheCreationTokens != 5 {
					t.Fatalf("cache-inclusive usage incorrect: %#v", usage)
				}
			})
		}
	}
}
func fmtBool(b bool) string {
	if b {
		return "/stream"
	}
	return "/json"
}

func TestInputRejectionsBeforeHTTP(t *testing.T) {
	inputs := []string{
		`null`, `{}`, `{"messages":[]}`, simpleBody + ` {}`,
		`{"messages":[{"role":"alien","content":"x"}]}`,
		`{"n":2,"messages":[{"role":"user","content":"x"}]}`,
		`{"audio":{},"messages":[{"role":"user","content":"x"}]}`,
		`{"response_format":{"type":"json_object"},"messages":[{"role":"user","content":"x"}]}`,
		`{"reasoning_effort":"high","messages":[{"role":"user","content":"x"}]}`,
		`{"seed":2,"messages":[{"role":"user","content":"x"}]}`,
		`{"max_tokens":0,"messages":[{"role":"user","content":"x"}]}`,
		`{"max_tokens":5,"max_completion_tokens":6,"messages":[{"role":"user","content":"x"}]}`,
		`{"temperature":3,"messages":[{"role":"user","content":"x"}]}`,
		`{"top_p":-1,"messages":[{"role":"user","content":"x"}]}`,
		`{"parallel_tool_calls":true,"messages":[{"role":"user","content":"x"}]}`,
		`{"messages":[{"role":"assistant","content":"x","signature":"private"}]}`,
		`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"x","signature":"private"}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://remote.example/image.png"}}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"AA=="}}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"file","file":{"file_id":"file-1"}}]}]}`,
		`{"messages":[{"role":"tool","content":"result"}]}`,
		`{"messages":[{"role":"assistant","reasoning":"a","reasoning_content":"b"}]}`,
		`{"messages":[{"role":"user","reasoning_content":"not assistant"}]}`,
		`{"messages":[{"role":"user","content":"x"}],"tools":[{"type":"web_search"}]}`,
		`{"messages":[{"role":"user","content":"x"}],"tools":[{"type":"function","function":{"name":"f","parameters":"not schema"}}]}`,
		`{"messages":[{"role":"user","content":"x"}],"tool_choice":{"type":"function","function":{"name":"missing"}}}`,
		`{"messages":[{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"f","arguments":"invalid"}}]}]}`,
	}
	c := &Client{Do: func(r *http.Request) (*http.Response, error) { t.Fatal("invalid input reached HTTP"); return nil, nil }}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			_, err := c.Chat(context.Background(), "token", "cascade", DefaultModels()[0], []byte(input))
			assertStatus(t, err, 400)
		})
	}
	for _, token := range []string{"", " ", "devin-session-token$"} {
		_, err := c.Chat(context.Background(), token, "cascade", DefaultModels()[0], []byte(simpleBody))
		assertStatus(t, err, 401)
		_, err = c.Models(context.Background(), token)
		assertStatus(t, err, 401)
	}
}

func TestInlineImages(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AQID","detail":"auto"}}]}]}`
	m, err := mapRequest([]byte(body), "cascade", Model{ID: "vision", SupportsImages: true})
	if err != nil {
		t.Fatal(err)
	}
	p := mustProto(t, mustProto(t, m.wire).data(3))
	image := mustProto(t, p.data(10))
	if p.text(3) != "describe" || image.text(1) != "AQID" || image.text(2) != "image/png" {
		t.Fatal("inline image mapping incorrect")
	}
	for _, model := range []Model{{ID: "text"}, {ID: "swe-1-6", SupportsImages: true}, {ID: "swe-1-6-fast", SupportsImages: true}} {
		_, err = mapRequest([]byte(body), "cascade", model)
		assertStatus(t, err, 400)
	}
	for _, url := range []string{"https://remote.example/image.png", "data:image/png;base64,not-base64", "data:image/bmp;base64,AQID", "data:image/png,raw"} {
		bad := strings.Replace(body, "data:image/png;base64,AQID", url, 1)
		_, err = mapRequest([]byte(bad), "cascade", Model{ID: "vision", SupportsImages: true})
		assertStatus(t, err, 400)
	}
}

func TestSafeHTTPAndCredentialErrors(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500, 503} {
		c := &Client{Do: func(r *http.Request) (*http.Response, error) {
			return responseBytes(status, []byte("token-secret upstream body")), nil
		}}
		_, err := c.Chat(context.Background(), "token-secret", "cascade", DefaultModels()[0], []byte(simpleBody))
		assertStatus(t, err, status)
		if strings.Contains(err.Error(), "token-secret") || strings.Contains(err.Error(), "upstream body") {
			t.Fatal("unsafe error")
		}
		_, err = c.Models(context.Background(), "token-secret")
		assertStatus(t, err, status)
	}
	c := &Client{Do: func(r *http.Request) (*http.Response, error) { return responseBytes(200, nil), nil }}
	_, err := c.Chat(context.Background(), "token", "cascade", DefaultModels()[0], []byte(simpleBody))
	assertStatus(t, err, 401)
	c = &Client{}
	_, err = c.Models(context.Background(), "token")
	assertStatus(t, err, 500)
}

func TestConnectTrailerErrors(t *testing.T) {
	statuses := map[string]int{"unauthenticated": 401, "permission_denied": 403, "resource_exhausted": 429, "unavailable": 503, "invalid_argument": 400, "internal": 502, "unknown-token-secret": 502}
	for code, status := range statuses {
		for _, delayed := range []bool{false, true} {
			t.Run(code+fmtBool(delayed), func(t *testing.T) {
				trailer, _ := json.Marshal(map[string]any{"error": map[string]string{"code": code, "message": "token-secret must not leak"}})
				fixture := connectFrame(2, trailer)
				if delayed {
					fixture = append(connectFrame(0, ps(nil, 3, "first")), fixture...)
				}
				c := baseClient(t, func(r *http.Request) (*http.Response, error) { return responseBytes(200, fixture), nil })
				resp, err := c.Chat(context.Background(), "token-secret", "cascade", DefaultModels()[0], []byte(streamBody))
				var data []byte
				if delayed {
					if err != nil {
						t.Fatal(err)
					}
					data, err = io.ReadAll(resp.Body)
					resp.Body.Close()
				}
				assertStatus(t, err, status)
				if strings.Contains(err.Error(), "token-secret") || bytes.Contains(data, []byte("[DONE]")) {
					t.Fatalf("unsafe error or success after failure: %v %s", err, data)
				}
			})
		}
	}
}

func TestConnectMalformedTruncatedAndLimits(t *testing.T) {
	oversized := []byte{0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(oversized[1:], maxPayload+1)
	expanded := connectFrame(1, zipped(t, bytes.Repeat([]byte{'a'}, maxPayload+1)))
	cases := map[string][]byte{
		"empty": nil, "partial header": {0, 0}, "partial payload": {0, 0, 0, 0, 2, 1}, "missing trailer": connectFrame(0, ps(nil, 3, "text")),
		"unknown flags": connectFrame(4, nil), "invalid gzip": connectFrame(1, []byte("invalid")), "compressed cap": oversized, "expanded cap": expanded,
		"malformed proto": connectFrame(0, []byte{0x0a, 0x05, 0x01}), "wrong known wire type": connectFrame(0, pv(nil, 3, 1)),
		"invalid trailer JSON": connectFrame(2, []byte(`{`)), "empty trailer": connectFrame(2, nil), "array trailer": connectFrame(2, []byte(`[]`)),
		"bad trailer error":             connectFrame(2, []byte(`{"error":"failure"}`)),
		"bytes after terminal trailer":  append(connectFrame(2, []byte(`{}`)), 0),
		"frames after terminal trailer": append(connectFrame(2, []byte(`{}`)), connectFrame(0, ps(nil, 3, "ignored"))...),
	}
	for name, fixture := range cases {
		t.Run(name, func(t *testing.T) {
			c := baseClient(t, func(r *http.Request) (*http.Response, error) { return responseBytes(200, fixture), nil })
			resp, err := c.Chat(context.Background(), "token", "cascade", DefaultModels()[0], []byte(streamBody))
			var data []byte
			if err == nil {
				data, err = io.ReadAll(resp.Body)
				resp.Body.Close()
			}
			assertStatus(t, err, 502)
			if bytes.Contains(data, []byte("[DONE]")) {
				t.Fatal("success emitted after truncation/error")
			}
		})
	}
	// Both compressed and expanded unary responses are bounded too.
	for _, payload := range [][]byte{bytes.Repeat([]byte{0}, maxPayload+1), zipped(t, bytes.Repeat([]byte{'a'}, maxPayload+1))} {
		c := &Client{Do: func(r *http.Request) (*http.Response, error) { return responseBytes(200, payload), nil }}
		_, err := c.Models(context.Background(), "token")
		assertStatus(t, err, 502)
	}
}

type blockingBody struct {
	prefix               *bytes.Reader
	started              chan struct{}
	closed               chan struct{}
	startOnce, closeOnce sync.Once
}

func newBlockingBody(prefix []byte) *blockingBody {
	return &blockingBody{prefix: bytes.NewReader(prefix), started: make(chan struct{}), closed: make(chan struct{})}
}
func (b *blockingBody) Read(p []byte) (int, error) {
	if b.prefix.Len() > 0 {
		return b.prefix.Read(p)
	}
	b.startOnce.Do(func() { close(b.started) })
	<-b.closed
	return 0, io.ErrClosedPipe
}
func (b *blockingBody) Close() error { b.closeOnce.Do(func() { close(b.closed) }); return nil }
func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation/close did not finish")
	}
}

func TestCancelDuringPrefetch(t *testing.T) {
	body := newBlockingBody(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := baseClient(t, func(r *http.Request) (*http.Response, error) { return &http.Response{StatusCode: 200, Body: body}, nil })
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		_, err = c.Chat(ctx, "token", "cascade", DefaultModels()[0], []byte(streamBody))
	}()
	await(t, body.started)
	cancel()
	await(t, done)
	await(t, body.closed)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context cancellation, got %v", err)
	}
}
func TestCancelAndCloseAfterReturn(t *testing.T) {
	for _, cancelOnly := range []bool{false, true} {
		t.Run(fmtBool(cancelOnly), func(t *testing.T) {
			body := newBlockingBody(connectFrame(0, ps(nil, 3, "first")))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var requestCtx context.Context
			c := baseClient(t, func(r *http.Request) (*http.Response, error) {
				requestCtx = r.Context()
				return &http.Response{StatusCode: 200, Body: body}, nil
			})
			resp, err := c.Chat(ctx, "token", "cascade", DefaultModels()[0], []byte(streamBody))
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			var data []byte
			var readErr error
			go func() { defer close(done); data, readErr = io.ReadAll(resp.Body) }()
			await(t, body.started)
			if cancelOnly {
				cancel()
			} else {
				resp.Body.Close()
			}
			await(t, body.closed)
			await(t, done)
			await(t, requestCtx.Done())
			resp.Body.Close()
			if !errors.Is(readErr, context.Canceled) || bytes.Contains(data, []byte("[DONE]")) {
				t.Fatalf("cancel result: %v %s", readErr, data)
			}
		})
	}
}

func TestSuccessfulEmptyTrailerAndFinishReasons(t *testing.T) {
	for stop, finish := range map[uint64]string{0: "stop", 3: "length", 5: "length", 11: "content_filter"} {
		c := baseClient(t, func(r *http.Request) (*http.Response, error) {
			return responseBytes(200, successFrames(pv(nil, 5, stop))), nil
		})
		resp, err := c.Chat(context.Background(), "token", "cascade", DefaultModels()[0], []byte(simpleBody))
		if err != nil {
			t.Fatal(err)
		}
		var out apicompat.ChatCompletionsResponse
		if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if out.Choices[0].FinishReason != finish {
			t.Fatalf("stop %d => %s", stop, out.Choices[0].FinishReason)
		}
	}
	c := baseClient(t, func(r *http.Request) (*http.Response, error) {
		return responseBytes(200, connectFrame(2, []byte(`{}`))), nil
	})
	resp, err := c.Chat(context.Background(), "token", "cascade", DefaultModels()[0], []byte(streamBody))
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || !bytes.Contains(b, []byte("[DONE]")) {
		t.Fatalf("empty successful response: %s %v", b, err)
	}
}

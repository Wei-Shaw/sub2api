package responses

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// mustNormalize 规范化一帧；失败即 t.Fatal。
func mustNormalize(t *testing.T, s *WSSession, payload string) json.RawMessage {
	t.Helper()
	out, err := s.NormalizeRequest(json.RawMessage(payload))
	if err != nil {
		t.Fatalf("NormalizeRequest(%s) err = %v", payload, err)
	}
	return out
}

// mustCommit 把一次完成的回合记入会话。
func mustCommit(t *testing.T, s *WSSession, outputJSON, responseID string) {
	t.Helper()
	s.Commit(WSTurnResult{
		CompletedOutput:     json.RawMessage(outputJSON),
		CompletedResponseID: responseID,
	})
}

func topField(t *testing.T, body json.RawMessage, key string) json.RawMessage {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		t.Fatalf("unmarshal normalized: %v", err)
	}
	return top[key]
}

func TestWSSessionInitialCreate(t *testing.T) {
	s := NewWSSession()
	out := mustNormalize(t, s, `{"type":"response.create","model":"gpt-test","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
	var top map[string]json.RawMessage
	if err := json.Unmarshal(out, &top); err != nil {
		t.Fatal(err)
	}
	if _, has := top["type"]; has {
		t.Fatal("type envelope field should be stripped")
	}
	if string(top["stream"]) != "true" {
		t.Fatalf("stream = %s, want true", top["stream"])
	}
	if string(top["model"]) != `"gpt-test"` {
		t.Fatalf("model = %s", top["model"])
	}
}

func TestWSSessionInitialRequiresModel(t *testing.T) {
	s := NewWSSession()
	if _, err := s.NormalizeRequest(json.RawMessage(`{"type":"response.create","input":[]}`)); err == nil {
		t.Fatal("missing model should fail")
	}
}

func TestWSSessionAppendBeforeCreateFails(t *testing.T) {
	s := NewWSSession()
	_, err := s.NormalizeRequest(json.RawMessage(`{"type":"response.append","input":[]}`))
	if err == nil || !strings.Contains(err.Error(), "before response.create") {
		t.Fatalf("err = %v", err)
	}
}

func TestWSSessionUnknownTypeFails(t *testing.T) {
	s := NewWSSession()
	_, err := s.NormalizeRequest(json.RawMessage(`{"type":"response.bogus","model":"m"}`))
	if err == nil || !errors.Is(err, ErrWSUnsupportedRequestType) {
		t.Fatalf("err = %v", err)
	}
}

func TestWSSessionNonRespPreviousID(t *testing.T) {
	s := NewWSSession()
	_, err := s.NormalizeRequest(json.RawMessage(`{"type":"response.create","previous_response_id":"msg_123","input":[]}`))
	if err == nil || !strings.Contains(err.Error(), "not a response id") {
		t.Fatalf("err = %v", err)
	}
}

func TestWSSessionMergeContinuation(t *testing.T) {
	s := NewWSSession()
	mustNormalize(t, s, `{"type":"response.create","model":"gpt-test","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"q1"}]}]}`)
	// 首轮完成：assistant 输出进入回放。
	mustCommit(t, s, `[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"a1"}]}]`, "resp_1")

	out := mustNormalize(t, s, `{"type":"response.create","previous_response_id":"resp_1","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"q2"}]}]}`)
	var top map[string]json.RawMessage
	if err := json.Unmarshal(out, &top); err != nil {
		t.Fatal(err)
	}
	// model 继承首轮。
	if string(top["model"]) != `"gpt-test"` {
		t.Fatalf("model = %s", top["model"])
	}
	var input []map[string]json.RawMessage
	if err := json.Unmarshal(top["input"], &input); err != nil {
		t.Fatal(err)
	}
	if len(input) != 3 {
		t.Fatalf("merged input len = %d, want 3 (user+assistant+user)", len(input))
	}
	if string(input[1]["role"]) != `"assistant"` {
		t.Fatalf("input[1].role = %s, want assistant (replayed output)", input[1]["role"])
	}
}

func TestWSSessionPreviousIDMismatch(t *testing.T) {
	s := NewWSSession()
	mustNormalize(t, s, `{"type":"response.create","model":"m","input":[]}`)
	mustCommit(t, s, `[]`, "resp_1")
	_, err := s.NormalizeRequest(json.RawMessage(`{"type":"response.create","previous_response_id":"resp_other","input":[]}`))
	if err == nil || !errors.Is(err, ErrWSPreviousResponseNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestWSSessionPendingToolCallGate(t *testing.T) {
	s := NewWSSession()
	mustNormalize(t, s, `{"type":"response.create","model":"m","input":[{"type":"message","role":"user","content":"run"}]}`)
	// 上轮输出含一个完整 function_call，pending 等待客户端回送 output。
	s.Commit(WSTurnResult{
		CompletedOutput:     json.RawMessage(`[{"type":"function_call","call_id":"call_1","name":"f","arguments":"{}"}]`),
		CompletedResponseID: "resp_1",
		PendingToolCallIDs:  []string{"call_1"},
	})
	// 续帧不带对应 output → 报错。
	_, err := s.NormalizeRequest(json.RawMessage(`{"type":"response.create","previous_response_id":"resp_1","input":[{"type":"message","role":"user","content":"no output"}]}`))
	if err == nil || !strings.Contains(err.Error(), "pending tool call") {
		t.Fatalf("err = %v", err)
	}
	// 带上 output → 放行并合并。
	out := mustNormalize(t, s, `{"type":"response.create","previous_response_id":"resp_1","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	if !strings.Contains(string(topField(t, out, "input")), "call_1") {
		t.Fatal("merged input should contain call output")
	}
}

func TestWSSessionOrphanOutputRejected(t *testing.T) {
	s := NewWSSession()
	_, err := s.NormalizeRequest(json.RawMessage(`{"type":"response.create","model":"m","input":[{"type":"function_call_output","call_id":"ghost","output":"x"}]}`))
	if err == nil || !strings.Contains(err.Error(), "unknown call_id") {
		t.Fatalf("err = %v", err)
	}
}

func TestWSSessionReplacementReplay(t *testing.T) {
	s := NewWSSession()
	mustNormalize(t, s, `{"type":"response.create","model":"m","input":[{"type":"message","role":"user","content":"q"}]}`)
	mustCommit(t, s, `[{"type":"message","role":"assistant","content":"a"}]`, "resp_1")
	s.RequireReplacementReplay()
	// 无 prev_id 的完整回放应走替换语义（不合并历史）。
	out := mustNormalize(t, s, `{"type":"response.create","model":"m","input":[{"type":"message","role":"user","content":"fresh"}]}`)
	var input []map[string]json.RawMessage
	if err := json.Unmarshal(topField(t, out, "input"), &input); err != nil {
		t.Fatal(err)
	}
	if len(input) != 1 {
		t.Fatalf("replacement input len = %d, want 1 (no merge)", len(input))
	}
}

func TestWSSessionCompletedTranscriptDetection(t *testing.T) {
	s := NewWSSession()
	mustNormalize(t, s, `{"type":"response.create","model":"m","input":[{"type":"message","role":"user","content":"q"}]}`)
	mustCommit(t, s, `[{"type":"message","role":"assistant","content":"a"}]`, "resp_1")
	// 客户端自带含 function_call 的完整历史 → 替换而非合并。
	out := mustNormalize(t, s, `{"type":"response.create","input":[{"type":"function_call","call_id":"c1","name":"f","arguments":"{}"},{"type":"function_call_output","call_id":"c1","output":"o"}]}`)
	var input []map[string]json.RawMessage
	if err := json.Unmarshal(topField(t, out, "input"), &input); err != nil {
		t.Fatal(err)
	}
	if len(input) != 2 {
		t.Fatalf("input len = %d, want 2 (replacement, not merged)", len(input))
	}
}

func TestWSGenerateDisabled(t *testing.T) {
	if !WSGenerateDisabled(json.RawMessage(`{"type":"response.create","generate":false,"model":"m"}`)) {
		t.Fatal("generate:false should be detected")
	}
	if WSGenerateDisabled(json.RawMessage(`{"type":"response.create","model":"m"}`)) {
		t.Fatal("absent generate should not trigger prewarm")
	}
	if WSGenerateDisabled(json.RawMessage(`{"type":"response.create","generate":true}`)) {
		t.Fatal("generate:true should not trigger prewarm")
	}
}

func TestWSPendingToolCallIDs(t *testing.T) {
	out := json.RawMessage(`[
		{"type":"function_call","call_id":"a","name":"f","arguments":"{}"},
		{"type":"function_call","call_id":"b","name":"g","arguments":123},
		{"type":"message","role":"assistant","content":"hi"}
	]`)
	ids := WSPendingToolCallIDs(out)
	if len(ids) != 1 || ids[0] != "a" {
		t.Fatalf("pending = %v, want [a]", ids)
	}
}

package cursor

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestParseConnectError(t *testing.T) {
	raw := `{"error":{"code":"invalid_argument","message":"ERROR_BAD_MODEL_NAME: Model name is not valid: \"grok-4.6\""}}`
	got, ok := ParseConnectError(raw)
	if !ok {
		t.Fatal("expected connect error")
	}
	if !got.IsBadModelName() {
		t.Fatalf("expected bad model name: %+v", got)
	}
	if !strings.Contains(got.Message, "grok-4.6") {
		t.Fatalf("message=%q", got.Message)
	}
}

func TestEncodeDecodeFrame(t *testing.T) {
	payload := []byte("hello world")

	// Uncompressed
	frame, err := EncodeFrame(payload, false)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeFrame(bytes.NewReader(frame))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.Payload, payload) {
		t.Fatalf("payload mismatch: got %q, want %q", decoded.Payload, payload)
	}

	// Compressed
	frame, err = EncodeFrame(payload, true)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = DecodeFrame(bytes.NewReader(frame))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.Payload, payload) {
		t.Fatalf("gzip payload mismatch: got %q, want %q", decoded.Payload, payload)
	}
}

func TestProtobufRoundTrip(t *testing.T) {
	var w ProtobufWriter
	w.Varint(1, 42)
	w.String(2, "hello")
	w.Bytes(3, []byte{0xDE, 0xAD})

	r := NewProtobufReader(w.Result())

	f1, err := r.Next()
	if err != nil || f1.Num != 1 || f1.Varint != 42 {
		t.Fatalf("field 1: %+v, err=%v", f1, err)
	}
	f2, err := r.Next()
	if err != nil || f2.Num != 2 || string(f2.Data) != "hello" {
		t.Fatalf("field 2: %+v, err=%v", f2, err)
	}
	f3, err := r.Next()
	if err != nil || f3.Num != 3 || !bytes.Equal(f3.Data, []byte{0xDE, 0xAD}) {
		t.Fatalf("field 3: %+v, err=%v", f3, err)
	}
}

func TestGenerateChecksum(t *testing.T) {
	machineID := "test-machine-id-1234"
	cs := GenerateChecksumAt(machineID, "", time.Unix(1700000000, 0))
	if cs == "" {
		t.Fatal("empty checksum")
	}
	if len(cs) <= len(machineID) {
		t.Fatalf("checksum too short: %q", cs)
	}
	// Should end with machine ID
	if cs[len(cs)-len(machineID):] != machineID {
		t.Fatalf("checksum should end with machine ID: %q", cs)
	}
}

func TestBuildChatRequest(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "Hello"},
	}
	data := BuildChatRequest(msgs, "default", ThinkingLevelUnspecified)
	if len(data) == 0 {
		t.Fatal("empty request")
	}
	// Should be parseable as protobuf (outer field 1)
	r := NewProtobufReader(data)
	f, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if f.Num != 1 || f.WireType != WireBytes {
		t.Fatalf("expected conversation field 1 bytes, got %+v", f)
	}
	inner := NewProtobufReader(f.Data)
	text, err := inner.Next()
	if err != nil || text == nil || string(text.Data) != "Hello" {
		t.Fatalf("expected message text Hello, got %+v err=%v", text, err)
	}
}

func TestBuildHeaders(t *testing.T) {
	creds := Credentials{
		AccessToken:  "test-token",
		MachineID:    "test-machine",
		MacMachineID: "test-mac-machine",
	}
	h := BuildHeaders(creds)
	if h["authorization"] != "Bearer test-token" {
		t.Fatalf("bad auth: %q", h["authorization"])
	}
	if h["x-cursor-client-version"] != DefaultClientVersion {
		t.Fatalf("bad version: %q", h["x-cursor-client-version"])
	}
	if h["x-cursor-checksum"] == "" {
		t.Fatal("empty checksum")
	}
	if !strings.Contains(h["x-cursor-checksum"], "test-machine/test-mac-machine") {
		t.Fatalf("checksum should include machineId/macMachineId: %q", h["x-cursor-checksum"])
	}
	if _, ok := h["x-cursor-client-commit"]; ok {
		t.Fatal("regular clients must not send x-cursor-client-commit")
	}
	if h["x-cursor-client-os"] != nodeOS() {
		t.Fatalf("os: got %q want %q", h["x-cursor-client-os"], nodeOS())
	}
	if h["x-cursor-client-arch"] != nodeArch() {
		t.Fatalf("arch: got %q want %q", h["x-cursor-client-arch"], nodeArch())
	}
	if h["x-cursor-client-layout"] != "editor" {
		t.Fatalf("layout: got %q", h["x-cursor-client-layout"])
	}
}

func TestBuildAgentClientMessage(t *testing.T) {
	payload, convID, runID := BuildAgentClientMessage([]ChatMessage{
		{Role: "user", Content: "Hello"},
	}, "default")
	if convID == "" || runID == "" {
		t.Fatal("missing ids")
	}
	run := GetNested(payload, fieldAgentClientRunRequest)
	if run == nil {
		t.Fatal("missing run_request")
	}
	if GetString(run, fieldRunConversationID) != convID {
		t.Fatalf("conversation id mismatch")
	}
	action := GetNested(run, fieldRunAction)
	userAction := GetNested(action, fieldActionUserMessage)
	userMsg := GetNested(userAction, fieldUserMsgActionMessage)
	if GetString(userMsg, fieldUserMsgText) != "Hello" {
		t.Fatalf("user text: %q", GetString(userMsg, fieldUserMsgText))
	}
}

func TestParseAgentTextDelta(t *testing.T) {
	var delta ProtobufWriter
	delta.String(fieldTextDeltaText, "Hi there")
	var update ProtobufWriter
	update.Bytes(fieldInteractionTextDelta, delta.Result())
	var server ProtobufWriter
	server.Bytes(fieldAgentServerInteraction, update.Result())

	events := ParseResponseFrame(server.Result())
	if len(events) != 1 || events[0].Type != "text" || events[0].Text != "Hi there" {
		t.Fatalf("events=%+v", events)
	}
}

func TestParseAgentTurnEnded(t *testing.T) {
	var update ProtobufWriter
	update.Bytes(fieldInteractionTurnEnded, nil)
	var server ProtobufWriter
	server.Bytes(fieldAgentServerInteraction, update.Result())
	events := ParseResponseFrame(server.Result())
	if len(events) != 1 || events[0].Type != "turn_ended" {
		t.Fatalf("events=%+v", events)
	}
	if events[0].Usage != nil {
		t.Fatalf("empty turn_ended should not carry usage: %+v", events[0].Usage)
	}
}

func TestParseAgentTurnEndedUsage(t *testing.T) {
	var ended ProtobufWriter
	ended.Varint(fieldTurnEndedInputTokens, 12)
	ended.Varint(fieldTurnEndedOutputTokens, 7)
	ended.Varint(fieldTurnEndedCacheReadTokens, 4)
	ended.Varint(fieldTurnEndedCacheWriteTokens, 3)
	ended.Varint(fieldTurnEndedReasoningTokens, 2)
	var update ProtobufWriter
	update.Bytes(fieldInteractionTurnEnded, ended.Result())
	var server ProtobufWriter
	server.Bytes(fieldAgentServerInteraction, update.Result())

	events := ParseResponseFrame(server.Result())
	if len(events) != 1 || events[0].Type != "turn_ended" || events[0].Usage == nil {
		t.Fatalf("events=%+v", events)
	}
	got := *events[0].Usage
	want := TokenUsage{InputTokens: 12, OutputTokens: 7, CacheReadTokens: 4, CacheWriteTokens: 3, ReasoningTokens: 2}
	if got != want {
		t.Fatalf("usage=%+v want %+v", got, want)
	}
}

func TestParseAgentTokenDelta(t *testing.T) {
	var delta ProtobufWriter
	delta.Varint(fieldTokenDeltaTokens, 5)
	var update ProtobufWriter
	update.Bytes(fieldInteractionTokenDelta, delta.Result())
	var server ProtobufWriter
	server.Bytes(fieldAgentServerInteraction, update.Result())

	events := ParseResponseFrame(server.Result())
	if len(events) != 1 || events[0].Type != "token_delta" || events[0].Usage == nil {
		t.Fatalf("events=%+v", events)
	}
	if events[0].Usage.OutputTokens != 5 {
		t.Fatalf("delta=%+v", events[0].Usage)
	}
}

func TestUsageAccumulatorPrefersTurnEnded(t *testing.T) {
	var acc UsageAccumulator
	acc.Observe(StreamEvent{Type: "token_delta", Usage: &TokenUsage{OutputTokens: 3}})
	acc.Observe(StreamEvent{Type: "token_delta", Usage: &TokenUsage{OutputTokens: 2}})
	if acc.Result().OutputTokens != 5 {
		t.Fatalf("delta sum=%+v", acc.Result())
	}
	acc.Observe(StreamEvent{Type: "turn_ended", Usage: &TokenUsage{InputTokens: 9, OutputTokens: 4}})
	got := acc.Result()
	if got.InputTokens != 9 || got.OutputTokens != 4 {
		t.Fatalf("turn_ended should replace deltas: %+v", got)
	}
}

func TestConsumeAssistantStreamCollectsUsage(t *testing.T) {
	var text ProtobufWriter
	text.String(fieldTextDeltaText, "Hi")
	var textUpdate ProtobufWriter
	textUpdate.Bytes(fieldInteractionTextDelta, text.Result())
	var textServer ProtobufWriter
	textServer.Bytes(fieldAgentServerInteraction, textUpdate.Result())

	var ended ProtobufWriter
	ended.Varint(fieldTurnEndedInputTokens, 8)
	ended.Varint(fieldTurnEndedOutputTokens, 1)
	var endUpdate ProtobufWriter
	endUpdate.Bytes(fieldInteractionTurnEnded, ended.Result())
	var endServer ProtobufWriter
	endServer.Bytes(fieldAgentServerInteraction, endUpdate.Result())

	textFrame, err := EncodeFrame(textServer.Result(), false)
	if err != nil {
		t.Fatal(err)
	}
	endFrame, err := EncodeFrame(endServer.Result(), false)
	if err != nil {
		t.Fatal(err)
	}

	var gotText strings.Builder
	usage, connectErr := ConsumeAssistantStream(bytes.NewReader(append(textFrame, endFrame...)), func(ev StreamEvent) error {
		if ev.Type == "text" {
			gotText.WriteString(ev.Text)
		}
		return nil
	})
	if connectErr != "" {
		t.Fatalf("connectErr=%s", connectErr)
	}
	if gotText.String() != "Hi" {
		t.Fatalf("text=%q", gotText.String())
	}
	if usage.InputTokens != 8 || usage.OutputTokens != 1 {
		t.Fatalf("usage=%+v", usage)
	}
}

func TestReadRawFrameRoundTrip(t *testing.T) {
	payload := []byte{0x0a, 0x05, 'h', 'e', 'l', 'l', 'o'}
	enc, err := EncodeFrame(payload, true)
	if err != nil {
		t.Fatal(err)
	}
	raw, frame, err := ReadRawFrame(bytes.NewReader(enc))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, enc) {
		t.Fatal("raw mismatch")
	}
	if !bytes.Equal(frame.Payload, payload) {
		t.Fatalf("payload mismatch %x vs %x", frame.Payload, payload)
	}
}

func TestParseChatTextStillWorks(t *testing.T) {
	var w ProtobufWriter
	w.String(1, "Hello")
	events := ParseResponseFrame(w.Result())
	if len(events) != 1 || events[0].Text != "Hello" {
		t.Fatalf("events=%+v", events)
	}
}

package kiro

import (
	"bytes"
	"testing"
)

func TestDecodeEventStream_Roundtrip(t *testing.T) {
	events := []Event{
		{Type: "assistantResponseEvent", Payload: map[string]any{"content": "hello"}},
		{Type: "assistantResponseEvent", Payload: map[string]any{"content": " world"}},
		{Type: "meteringEvent", Payload: map[string]any{"usage": 1.5}},
	}
	var buf bytes.Buffer
	if err := EncodeEventStream(&buf, events); err != nil {
		t.Fatal(err)
	}

	var got []Event
	err := DecodeEventStream(&buf, func(e Event) {
		got = append(got, e)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if got[0].Type != "assistantResponseEvent" || got[0].Payload["content"] != "hello" {
		t.Fatalf("event[0] wrong: %+v", got[0])
	}
	if got[2].Type != "meteringEvent" {
		t.Fatalf("event[2] type = %q", got[2].Type)
	}
}

func TestDecodeEventStream_EmptyInput(t *testing.T) {
	err := DecodeEventStream(bytes.NewReader(nil), func(Event) {
		t.Fatal("onEvent should not be called for empty input")
	})
	if err != nil {
		t.Fatalf("expected nil for empty input, got %v", err)
	}
}

func TestDecodeEventStream_TruncatedPrelude(t *testing.T) {
	// Only 5 bytes — truncated prelude (need 12). ReadFull yields
	// io.ErrUnexpectedEOF which we treat as a clean exit.
	err := DecodeEventStream(bytes.NewReader([]byte{1, 2, 3, 4, 5}), func(Event) {})
	if err != nil {
		t.Fatalf("expected nil on truncated prelude, got %v", err)
	}
}

func TestDecodeEventStream_SkipsMalformedJSON(t *testing.T) {
	// Construct one frame with valid framing but invalid JSON payload,
	// followed by a valid event.
	var buf bytes.Buffer

	// Bad frame: type=foo, payload=`not-json`
	header := encodeHeader(":event-type", "foo")
	payload := []byte("not-json")
	headersLen := len(header)
	totalLen := 12 + headersLen + len(payload) + 4
	prelude := make([]byte, 12)
	prelude[0] = byte(totalLen >> 24)
	prelude[1] = byte(totalLen >> 16)
	prelude[2] = byte(totalLen >> 8)
	prelude[3] = byte(totalLen)
	prelude[4] = byte(headersLen >> 24)
	prelude[5] = byte(headersLen >> 16)
	prelude[6] = byte(headersLen >> 8)
	prelude[7] = byte(headersLen)
	buf.Write(prelude)
	buf.Write(header)
	buf.Write(payload)
	buf.Write([]byte{0, 0, 0, 0})

	// Good frame after it.
	_ = EncodeEventStream(&buf, []Event{{Type: "ok", Payload: map[string]any{"a": 1}}})

	var got []Event
	if err := DecodeEventStream(&buf, func(e Event) {
		got = append(got, e)
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event (malformed skipped), got %d", len(got))
	}
	if got[0].Type != "ok" {
		t.Fatalf("event type = %q", got[0].Type)
	}
}

func TestExtractEventType_Direct(t *testing.T) {
	h := encodeHeader(":event-type", "assistantResponseEvent")
	if got := extractEventType(h); got != "assistantResponseEvent" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractEventType_MissingTypeHeader(t *testing.T) {
	h := encodeHeader(":content-type", "application/json")
	if got := extractEventType(h); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

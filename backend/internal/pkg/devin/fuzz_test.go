package devin

import (
	"bytes"
	"io"
	"testing"
)

func FuzzDevinUnaryProto(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x0a, 0x03, 'j', 'w', 't'})
	f.Add([]byte{0x1f, 0x8b, 0x08})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			t.Skip()
		}
		message, err := decodeUnary(data)
		if err == nil {
			// Exercise the accessors/type guards with adversarial field encodings.
			_ = message.text(1)
			_ = message.num(5)
			_ = message.check([]int32{1, 3, 6, 7, 9, 23}, []int32{5, 8, 11})
		}
	})
}

func FuzzDevinConnectFrames(f *testing.F) {
	f.Add(connectFrame(2, []byte(`{}`)))
	f.Add(successFrames(ps(nil, 3, "hello")))
	f.Add(connectFrame(2, []byte(`{"error":{"code":"resource_exhausted"}}`)))
	f.Add([]byte{0, 0xff, 0xff, 0xff, 0xff})
	if zipped, err := gzipFrame(ps(nil, 3, "compressed")); err == nil {
		f.Add(append(zipped, connectFrame(2, []byte(`{}`))...))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			t.Skip()
		}
		reader := &connectReader{r: bytes.NewReader(data)}
		state := newCompletion("fuzz-model")
		state.streaming = true
		for i := 0; i <= len(data)/5+1; i++ {
			message, err := reader.next()
			if err != nil {
				if err == io.EOF && !reader.terminal {
					t.Fatal("EOF without a successful terminal trailer")
				}
				return
			}
			if _, err := state.apply(message); err != nil {
				return
			}
		}
		t.Fatal("Connect parser failed to consume its bounded input")
	})
}

func FuzzDevinRequestMapping(f *testing.F) {
	f.Add([]byte(simpleBody))
	f.Add([]byte(streamBody))
	f.Add([]byte(`{"messages":[{"role":"tool","content":"failed","tool_call_id":"c","is_error":true}]}`))
	f.Add([]byte(`{"messages":[{"role":"assistant","tool_calls":[{"type":"function","id":"c","function":{"name":"f","arguments":"{}"}}]}]}`))
	f.Add([]byte(`{"messages":null}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 64<<10 {
			t.Skip()
		}
		mapped, err := mapRequest(body, "fuzz-cascade", DefaultModels()[0])
		if err != nil {
			return
		}
		wire, err := parseProto(mapped.wire)
		if err != nil {
			t.Fatal("accepted request produced malformed protobuf")
		}
		if wire.text(16) != "fuzz-cascade" || wire.text(22) == "" {
			t.Fatal("accepted request lost required turn identity")
		}
	})
}

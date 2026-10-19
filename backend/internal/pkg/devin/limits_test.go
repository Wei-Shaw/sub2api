package devin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProtoFieldCountBoundsDecodedMemory(t *testing.T) {
	// The payload is only ~128 KiB (well below 16 MiB) but would otherwise
	// allocate one Go struct per repeated scalar; gzip can amplify it further.
	payload := bytes.Repeat([]byte{0x08, 0x01}, maxProtoFields+1)
	_, err := parseProto(payload)
	assertStatus(t, err, 502)
	frame, err := gzipFrame(payload)
	if err != nil {
		t.Fatal(err)
	}
	reader := &connectReader{r: bytes.NewReader(frame)}
	_, err = reader.next()
	assertStatus(t, err, 502)
}

func TestCompletionAggregateOutputLimit(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			state := newCompletion("model")
			state.streaming = streaming
			// Every individual protobuf message is far below the frame limit.
			message := mustProto(t, ps(ps(nil, 3, strings.Repeat("x", 512<<10)), 9, strings.Repeat("y", 512<<10)))
			for i := 0; i < maxCompletionBytes/(1<<20); i++ {
				if _, err := state.apply(message); err != nil {
					t.Fatal(err)
				}
			}
			if streaming && (state.text.Len() != 0 || state.thinking.Len() != 0) {
				t.Fatal("SSE retained already-emitted text/reasoning")
			}
			_, err := state.apply(mustProto(t, ps(nil, 3, "overflow")))
			assertStatus(t, err, 502)
			if state.outputBytes != maxCompletionBytes {
				t.Fatal("output accounting exceeded limit")
			}
		})
	}
}

func TestCompletionToolLimits(t *testing.T) {
	state := newCompletion("model")
	call := ps(ps(ps(nil, 1, "call"), 2, "tool"), 3, strings.Repeat("x", maxToolArgumentBytes))
	if _, err := state.apply(mustProto(t, pb(nil, 6, call))); err != nil {
		t.Fatal(err)
	}
	// A repeated cumulative snapshot consumes no additional output budget.
	before := state.outputBytes
	if _, err := state.apply(mustProto(t, pb(nil, 6, call))); err != nil {
		t.Fatal(err)
	}
	if state.outputBytes != before {
		t.Fatal("duplicate snapshot counted twice")
	}
	_, err := state.apply(mustProto(t, pb(nil, 6, ps(nil, 3, "y"))))
	assertStatus(t, err, 502)

	state = newCompletion("model")
	for i := 0; i < maxCompletionTools; i++ {
		call = ps(ps(ps(nil, 1, fmt.Sprint(i)), 2, "f"), 3, `{}`)
		if _, err = state.apply(mustProto(t, pb(nil, 6, call))); err != nil {
			t.Fatal(err)
		}
	}
	_, err = state.apply(mustProto(t, pb(nil, 6, ps(ps(ps(nil, 1, "extra"), 2, "f"), 3, `{}`))))
	assertStatus(t, err, 502)
	if len(state.tools) != maxCompletionTools {
		t.Fatal("tool count exceeded bound")
	}
}

func TestStreamIDStableWithLateUpstreamID(t *testing.T) {
	for _, late := range [][]byte{ps(nil, 1, "authoritative-id"), pb(nil, 7, ps(nil, 7, "usage-id"))} {
		c := baseClient(t, func(*http.Request) (*http.Response, error) {
			return responseBytes(200, successFrames(ps(nil, 3, "first"), late, ps(nil, 3, "last"))), nil
		})
		resp, err := c.Chat(context.Background(), "token", "cascade", DefaultModels()[0], []byte(streamBody))
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		id := ""
		count := 0
		for _, line := range strings.Split(string(body), "\n") {
			if !strings.HasPrefix(line, "data: {") {
				continue
			}
			var chunk struct {
				ID string `json:"id"`
			}
			if err = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
				t.Fatal(err)
			}
			if id == "" {
				id = chunk.ID
			}
			if chunk.ID != id {
				t.Fatalf("completion ID changed from %q to %q", id, chunk.ID)
			}
			count++
		}
		if count < 3 || id == "" {
			t.Fatal("did not inspect a full stream")
		}
	}
}

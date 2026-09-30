package basispoints

import (
	"bufio"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUnknownToolRegenerationStopsAfterVisibleContent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		event   object
		visible bool
	}{
		{"text_delta", object{"type": "response.output_text.delta", "delta": "Working."}, true},
		{"summary_delta", object{"type": "response.reasoning_summary_text.delta", "delta": "Checking tools."}, true},
		{"whitespace_delta", object{"type": "response.output_text.delta", "delta": " "}, true},
		{"empty_summary_delta", object{"type": "response.reasoning_summary_text.delta", "delta": ""}, false},
		{"text_done", object{"type": "response.output_text.done", "text": "Working."}, true},
		{"summary_done", object{"type": "response.reasoning_summary_text.done", "text": "Checking tools."}, true},
		{"summary_part", object{"type": "response.reasoning_summary_part.done", "part": object{"type": "summary_text", "text": "Checking tools."}}, true},
		{"message_item", object{"type": "response.output_item.done", "item": object{"type": "message", "content": []any{object{"type": "output_text", "text": "Working."}}}}, true},
		{"reasoning_item", object{"type": "response.output_item.done", "item": object{"type": "reasoning", "summary": []any{object{"type": "summary_text", "text": "Checking tools."}}}}, true},
		{"empty_reasoning_item", object{"type": "response.output_item.added", "item": object{"type": "reasoning", "summary": []any{}, "encrypted_content": "opaque-test-value"}}, false},
		{"empty_content_part", object{"type": "response.content_part.added", "part": object{"type": "output_text", "text": ""}}, false},
		{"refusal_delta", object{"type": "response.refusal.delta", "delta": "Cannot do that."}, true},
		{"refusal_part", object{"type": "response.content_part.done", "part": object{"type": "refusal", "refusal": "Cannot do that."}}, true},
		{"metadata", object{"type": "response.created", "response": object{"id": "resp_original", "output": []any{}}}, false},
	} {
		for _, stream := range []bool{true, false} {
			name := tc.name + "/buffered"
			if stream {
				name = tc.name + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				source := testSource()
				source["stream"] = stream
				source["tools"] = []any{object{"type": "function", "name": "shell"}}
				cache := new(ReplayCache)
				_, bridge := mustPrepare(t, source, "boundary", cache)
				bad := nativeCall(object{"name": "unknown", "arguments": object{}})
				original := repairResponse("resp_original", 10, 2, bad)
				wire := sse(tc.event) + sse(object{"type": "response.completed", "response": original})
				regenerations, corrections := 0, 0
				body := bridge.StreamWithRepairs(context.Background(), io.NopCloser(strings.NewReader(wire)), func(context.Context, object, error) (object, error) {
					corrections++
					return nil, nil
				}, func(context.Context) (io.ReadCloser, error) {
					regenerations++
					good := nativeCall(object{"name": "shell", "arguments": object{"cmd": "pwd"}})
					good["id"], good["call_id"] = "fc_fixed", "call_fixed"
					return io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": repairResponse("resp_fixed", 7, 1, good)}))), nil
				})
				events := repairEvents(t, body)
				require.Equal(t, tc.event["type"], events[0]["type"])
				for key, value := range tc.event {
					if key == "response" { // Response metadata is normalized by the bridge.
						continue
					}
					require.Equal(t, value, events[0][key], "forwarded event changed: %s", key)
				}
				require.Zero(t, corrections, "must not fall through to the formatting correction path")
				last := events[len(events)-1]
				response := repairValue[object](t, last["response"])
				require.Equal(t, "resp_original", response["id"])
				if stream && tc.visible {
					require.Zero(t, regenerations)
					require.Equal(t, "response.failed", last["type"])
					require.Equal(t, "basispoints_protocol_error", repairValue[object](t, response["error"])["code"])
					require.Equal(t, `{"input_tokens":10,"input_tokens_details":{"cached_tokens":2},"output_tokens":2,"output_tokens_details":{"reasoning_tokens":1},"total_tokens":12}`, quoted(response["usage"]))
					for _, event := range events {
						require.NotEqual(t, "response.completed", event["type"])
						require.False(t, isToolEvent(text(event["type"])))
						item, _ := event["item"].(object)
						require.False(t, isTool(item))
					}
					require.Nil(t, cache.get("boundary", "call_fixed"))
				} else {
					require.Equal(t, 1, regenerations)
					require.Equal(t, "response.completed", last["type"])
					require.NotNil(t, cache.get("boundary", "call_fixed"))
				}
				require.Nil(t, cache.get("boundary", "call_native"))
			})
		}
	}
}

func TestReasoningSummaryArrivesBeforeTerminal(t *testing.T) {
	source := testSource()
	source["stream"] = true
	_, bridge := mustPrepare(t, source, "", nil)
	upstream, producer := io.Pipe()
	body := bridge.Stream(upstream)
	t.Cleanup(func() { _ = body.Close(); _ = producer.Close() })
	written := make(chan error, 1)
	go func() {
		_, err := io.WriteString(producer, sse(object{"type": "response.reasoning_summary_text.delta", "item_id": "rs_test", "output_index": 0, "summary_index": 0, "delta": "Checking tools."}))
		written <- err
	}()
	// The producer remains open without a terminal event. The summary must
	// already be readable, not buffered until the upstream finishes.
	read := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(body).ReadString('\n')
		read <- line
	}()
	select {
	case line := <-read:
		require.Equal(t, "event: response.reasoning_summary_text.delta\n", line)
	case <-time.After(3 * time.Second):
		t.Fatal("reasoning summary was withheld until the terminal event")
	}
	require.NoError(t, <-written)
}

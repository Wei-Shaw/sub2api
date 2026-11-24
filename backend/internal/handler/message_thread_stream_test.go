//go:build unit

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// threadClientEvent is one event as a client reading the server-sent events
// format receives it.
type threadClientEvent struct {
	name string
	data string
}

// threadReadClientEvents reads a response body the way the format defines it:
// lines end in CRLF, LF or CR; a blank line dispatches the event collected so
// far, provided it has data; data lines are joined by a line feed; a line that
// starts with a colon is a comment; whatever is left when the body ends without
// a blank line is discarded. It is written for these tests and shares no code
// with the gateway.
func threadReadClientEvents(body string) []threadClientEvent {
	body = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(body)
	var events []threadClientEvent
	name, data, hasData := "", "", false
	for {
		line, rest, terminated := strings.Cut(body, "\n")
		if !terminated {
			return events
		}
		body = rest
		if line == "" {
			if hasData {
				events = append(events, threadClientEvent{name: name, data: strings.TrimSuffix(data, "\n")})
			}
			name, data, hasData = "", "", false
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			name = value
		case "data":
			data, hasData = data+value+"\n", true
		}
	}
}

// threadCompactEvents renders events with their JSON compacted, so that two
// streams compare by what they say.
func threadCompactEvents(t *testing.T, events []threadClientEvent) []string {
	t.Helper()
	out := make([]string, len(events))
	for i, event := range events {
		var decoded any
		require.NoError(t, json.Unmarshal([]byte(event.data), &decoded), event.data)
		compact, err := json.Marshal(decoded)
		require.NoError(t, err)
		out[i] = event.name + " " + string(compact)
	}
	return out
}

var threadTestWires = map[string]threadFakeWire{
	"crlf":               {newline: "\r\n"},
	"cr":                 {newline: "\r"},
	"several data lines": {split: true},
	"no space":           {tight: true},
	"comments":           {comments: true},
	"other fields":       {fields: true},
	"one byte a write":   {chunk: 1},
	"seven bytes":        {chunk: 7},
	"everything, crlf":   {newline: "\r\n", split: true, tight: true, comments: true, fields: true, chunk: 5},
	"everything, cr":     {newline: "\r", split: true, comments: true, fields: true, chunk: 3},
}

// The server-sent events format lets an upstream write the same events in
// many ways. Whatever the representation and however the bytes are cut up on
// the way, a thread response means one thing: the same usage is billed, the
// same binding is written at the same moment, and the client is handed the
// same events.
func TestMessageThreadStreamRepresentations(t *testing.T) {
	type outcome struct {
		bodies   []string
		commands [][4]int
	}
	run := func(t *testing.T, kind string, wire threadFakeWire) outcome {
		t.Helper()
		h := newThreadHarness(t, kind)
		h.a.setWire(wire)
		var out outcome
		previous := ""
		for turn := 1; turn <= 3; turn++ {
			writer := &threadWriter{}
			rec := h.send(threadTurn{previous: previous, stream: true, writer: writer})
			previous = threadResponseID(t, rec)
			require.Equal(t, fmt.Sprintf("msg_a_%d", turn), previous)
			out.bodies = append(out.bodies, rec.Body.String())

			// Bound when the terminal event was received, before it was written.
			sawStop := false
			for _, write := range writer.writes {
				if strings.Contains(write.data, "message_stop") {
					require.Equal(t, turn, write.bindings, "bound before message_stop is written")
					sawStop = true
				} else {
					require.Equal(t, turn-1, write.bindings, "not bound before the response is complete: %q", write.data)
				}
			}
			require.True(t, sawStop)
		}
		// The upstream published each response: a fourth request continues.
		threadResponseID(t, h.send(threadTurn{previous: previous}))
		require.Equal(t, 4, h.a.count())
		require.Zero(t, h.b.count())
		for _, command := range h.requireCommands(4) {
			out.commands = append(out.commands, [4]int{command.InputTokens, command.CacheReadTokens, command.CacheCreationTokens, command.OutputTokens})
		}
		return out
	}
	for _, kind := range []string{"apikey", "passthrough", "oauth", "oauth-cli", "setup-token"} {
		t.Run(kind, func(t *testing.T) {
			plain := run(t, kind, threadFakeWire{})
			for _, command := range plain.commands {
				require.Equal(t, [4]int{10000, 80000, 1000, 20}, command, "what the upstream metered, once per request")
			}
			for _, body := range plain.bodies {
				events := threadReadClientEvents(body)
				require.Equal(t, "message_start", events[0].name)
				require.Equal(t, "message_stop", events[len(events)-1].name)
				require.Equal(t, int64(20), gjson.Get(events[len(events)-2].data, "usage.output_tokens").Int())
			}
			for name, wire := range threadTestWires {
				t.Run(name, func(t *testing.T) {
					got := run(t, kind, wire)
					require.Equal(t, plain.commands, got.commands, "the same usage is billed")
					for i := range plain.bodies {
						require.Equal(t, threadCompactEvents(t, threadReadClientEvents(plain.bodies[i])),
							threadCompactEvents(t, threadReadClientEvents(got.bodies[i])), "the client receives the same events")
					}
				})
			}
		})
	}
}

// What ends an event is a blank line, and a content block is closed by the
// stop event that carries its index. A stream that only looks finished is not
// a response to continue from, on any path, and an event the upstream never
// dispatched is not handed to the client.
func TestMessageThreadStreamThatOnlyLooksComplete(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth"} {
		for _, mode := range []string{"space-line-stop", "stop-without-data", "wrong-block-stop", "reopened-block"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				h := newThreadHarness(t, kind)
				first := threadResponseID(t, h.send(threadTurn{stream: true}))
				h.requireCommands(1)

				h.a.setMode(mode)
				rec := h.send(threadTurn{previous: first, stream: true})
				require.Equal(t, http.StatusOK, rec.Code)
				h.a.setMode("")

				require.Equal(t, 1, threadBindingCount(h.db), "the upstream did not publish this response, and neither does the gateway")
				command := h.requireCommands(2)[1]
				require.Equal(t, [4]int{10000, 80000, 1000, 20},
					[4]int{command.InputTokens, command.CacheReadTokens, command.CacheCreationTokens, command.OutputTokens}, "what did arrive is billed")
				events := threadReadClientEvents(rec.Body.String())
				switch mode {
				case "space-line-stop", "stop-without-data":
					for _, event := range events {
						require.NotEqual(t, "message_stop", event.name)
						require.NotEqual(t, "message_stop", gjson.Get(event.data, "type").Str)
					}
					require.NotContains(t, rec.Body.String(), "message_stop", "the client is not handed an event the upstream did not finish")
				}
				h.requireNotFound(threadTurn{previous: "msg_a_2"})
				require.Equal(t, 2, h.a.count())
				require.Zero(t, h.b.count())
			})
		}
	}
}

//go:build unit

package handler

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func threadCommandTokens(command *service.UsageBillingCommand) [4]int {
	return [4]int{command.InputTokens, command.CacheReadTokens, command.CacheCreationTokens, command.OutputTokens}
}

// requireBadGateway checks a local failure that is neither a missing thread
// nor a client error, answered with the status of an upstream failure.
func threadRequireBadGateway(t *testing.T, code int, body string) {
	t.Helper()
	require.Equal(t, http.StatusBadGateway, code, body)
	require.Equal(t, "error", gjson.Get(body, "type").Str, body)
	require.Equal(t, "api_error", gjson.Get(body, "error.type").Str, body)
	require.False(t, gjson.Get(body, "error.details").Exists(), body)
	require.False(t, gjson.Get(body, "id").Exists(), "the response itself is not delivered")
}

// A non-streaming response arrived whole, with its usage, and then the
// connection failed before the body ended. The upstream did the work and
// reported what it metered: that is settled once. The response is not
// delivered and nothing is bound.
func TestMessageThreadNonStreamReadErrorAfterWholeResponse(t *testing.T) {
	for _, kind := range threadTestKinds() {
		for _, start := range []string{"create", "continue"} {
			t.Run(kind+"/"+start, func(t *testing.T) {
				h := newThreadHarness(t, kind)
				first := threadResponseID(t, h.send(threadTurn{}))
				h.requireCommands(1)
				turn := threadTurn{previous: first}
				if start == "create" {
					turn = threadTurn{}
				}

				// Control: the same response with the newline that ends the body.
				control := newThreadHarness(t, kind)
				controlFirst := threadResponseID(t, control.send(threadTurn{}))
				control.a.setMode("json-trailing-newline")
				controlTurn := threadTurn{previous: controlFirst}
				if start == "create" {
					controlTurn = threadTurn{}
				}
				threadResponseID(t, control.send(controlTurn))
				require.Equal(t, [4]int{10000, 80000, 1000, 20}, threadCommandTokens(control.requireCommands(2)[1]))
				require.Equal(t, 2, threadBindingCount(control.db))

				h.a.setMode("json-read-error")
				rec := h.send(turn)
				h.a.setMode("")
				require.Equal(t, http.StatusBadGateway, rec.Code)
				require.Equal(t, 2, h.a.count(), "sent once")
				require.Zero(t, h.b.count(), "and tried nowhere else")
				require.Equal(t, 1, threadBindingCount(h.db), "nothing is bound")
				command := h.requireCommands(2)[1]
				require.Equal(t, [4]int{10000, 80000, 1000, 20}, threadCommandTokens(command), "what the upstream reported is settled, once")
				threadRequireBadGateway(t, rec.Code, rec.Body.String())
				h.requireNotFound(threadTurn{previous: "msg_a_2"})

				if start == "continue" {
					// The upstream did continue the thread. Its own answer to a
					// second attempt reaches the client, who then replays.
					rec = h.send(threadTurn{previous: first})
					require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
					require.Equal(t, "thread_already_continued", gjson.Get(rec.Body.String(), "error.details.error_code").Str)
				}
				require.Zero(t, h.b.count())
				h.requireCommands(2)
			})
		}
		t.Run(kind+"/cut inside the document", func(t *testing.T) {
			// Half a document says nothing that can be billed: no number is
			// guessed from it.
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{}))
			h.a.setMode("json-cut")
			rec := h.send(threadTurn{previous: first})
			h.a.setMode("")
			require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
			require.Equal(t, 2, h.a.count())
			require.Zero(t, h.b.count())
			require.Equal(t, 1, threadBindingCount(h.db))
			h.requireCommands(1)
		})
		t.Run(kind+"/over the read limit", func(t *testing.T) {
			h := newThreadHarness(t, kind)
			// The fixture is a real Message, but the bytes within the limit
			// alone do not establish its usage.
			h.cfg.Gateway.UpstreamResponseReadMaxBytes = 40
			h.a.setMode("json-read-error")
			rec := h.send(threadTurn{})
			require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
			require.Equal(t, 1, h.a.count())
			require.Zero(t, h.b.count())
			require.Zero(t, threadBindingCount(h.db))
			h.requireCommands(0)
		})
		t.Run(kind+"/without a thread", func(t *testing.T) {
			// A request without a thread is handled as it always was.
			h := newThreadHarness(t, kind)
			h.a.setMode("json-read-error")
			rec := h.send(threadTurn{noThread: true, omitDefs: true})
			h.a.setMode("")
			require.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError, rec.Body.String())
			require.Zero(t, threadBindingCount(h.db))
			h.requireCommands(0)
		})
	}
}

// A non-streaming response without the content list is not a Message. It is
// not handed to the client as a success and not bound; what it reports as
// metered is settled. An empty list is content.
func TestMessageThreadNonStreamResponseWithoutContent(t *testing.T) {
	for _, kind := range threadTestKinds() {
		for _, mode := range []string{"no-content", "content-null", "content-object"} {
			for _, start := range []string{"create", "continue"} {
				t.Run(kind+"/"+mode+"/"+start, func(t *testing.T) {
					h := newThreadHarness(t, kind)
					first := threadResponseID(t, h.send(threadTurn{}))
					h.requireCommands(1)
					turn := threadTurn{previous: first}
					if start == "create" {
						turn = threadTurn{}
					}
					h.a.setMode(mode)
					rec := h.send(turn)
					h.a.setMode("")
					threadRequireBadGateway(t, rec.Code, rec.Body.String())
					require.Equal(t, 2, h.a.count(), "sent once")
					require.Zero(t, h.b.count(), "and tried nowhere else")
					require.Equal(t, 1, threadBindingCount(h.db), "the upstream did not publish this response, and neither does the gateway")
					require.Equal(t, [4]int{10000, 80000, 1000, 20}, threadCommandTokens(h.requireCommands(2)[1]), "what it reported is settled")
					h.requireNotFound(threadTurn{previous: "msg_a_2"})
				})
			}
		}
		t.Run(kind+"/ordinary response is unchanged", func(t *testing.T) {
			h := newThreadHarness(t, kind)
			h.a.setMode("no-content")
			rec := h.send(threadTurn{noThread: true, omitDefs: true})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.False(t, gjson.Get(rec.Body.String(), "content").Exists())
			require.Zero(t, threadBindingCount(h.db))
			require.Equal(t, [4]int{10000, 80000, 1000, 20}, threadCommandTokens(h.requireCommands(1)[0]))
		})
		t.Run(kind+"/an empty list is content", func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{omitDefs: true}))
			h.a.setMode("empty-content")
			rec := h.send(threadTurn{previous: first, messages: `[{"role":"user","content":"and now?"}]`})
			h.a.setMode("")
			second := threadResponseID(t, rec)
			require.Equal(t, `[]`, gjson.Get(rec.Body.String(), "content").Raw)
			require.Equal(t, 2, threadBindingCount(h.db))
			threadResponseID(t, h.send(threadTurn{previous: second, stream: true, messages: `[{"role":"user","content":"go on"}]`}))
			require.Equal(t, 3, h.a.count())
			require.Zero(t, h.b.count())
		})
	}
}

type threadFixtureRead func([]byte) (int, error)

func (f threadFixtureRead) Read(p []byte) (int, error) { return f(p) }

type threadFixtureTransport func(*http.Request) (*http.Response, error)

func (f threadFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Changing only the name of the block close must not erase a cumulative
// usage report already in that very Read. Do not depend on HTTP write sizes.
func TestMessageThreadMeterAlreadyFramedUsage(t *testing.T) {
	for _, kind := range threadTestKinds() {
		for _, tc := range []struct{ mode, closeName string }{
			{"", "content_block_stop"}, {"block-stop-named-ping", "ping"}, {"block-stop-named-end", "message_stop"},
		} {
			t.Run(kind+"/"+tc.closeName, func(t *testing.T) {
				h := newThreadHarness(t, kind)
				parent := threadResponseID(t, h.send(threadTurn{}))
				h.requireCommands(1)
				h.a.setMode(tc.mode)
				firstRead := make(chan string, 1)
				client := h.a.srv.Client()
				transport := client.Transport
				client.Transport = threadFixtureTransport(func(req *http.Request) (*http.Response, error) {
					resp, err := transport.RoundTrip(req)
					if err != nil {
						return nil, err
					}
					raw, err := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if err != nil {
						return nil, err
					}
					resp.Body = io.NopCloser(threadFixtureRead(func(p []byte) (int, error) {
						if raw == nil {
							return 0, io.EOF
						}
						if len(p) < len(raw) {
							return 0, io.ErrShortBuffer
						}
						n := copy(p, raw)
						select {
						case firstRead <- string(p[:n]):
						default:
						}
						raw = nil
						return n, io.EOF
					}))
					return resp, nil
				})
				t.Cleanup(func() { client.Transport = transport })
				rec := h.send(threadTurn{previous: parent, stream: true})
				var frames []threadClientEvent
				select {
				case raw := <-firstRead:
					frames = threadReadClientEvents(raw)
				default:
					t.Fatal("gateway did not read the fixture")
				}
				require.GreaterOrEqual(t, len(frames), 3)
				tail := frames[len(frames)-3:]
				require.Equal(t, tc.closeName, tail[0].name)
				require.Equal(t, "content_block_stop", gjson.Get(tail[0].data, "type").Str)
				require.Equal(t, "message_delta", tail[1].name)
				require.Equal(t, int64(20), gjson.Get(tail[1].data, "usage.output_tokens").Int())
				require.Equal(t, "message_stop", tail[2].name)
				require.Equal(t, "message_stop", gjson.Get(tail[2].data, "type").Str)

				require.Equal(t, http.StatusOK, rec.Code)
				stopped, failed := threadStreamEnd(rec.Body.String())
				valid := tc.mode == ""
				require.Equal(t, valid, stopped)
				require.Equal(t, !valid, failed)
				if valid {
					require.Equal(t, 2, threadBindingCount(h.db))
				} else {
					require.Equal(t, 1, threadBindingCount(h.db))
					client := threadReadClientEvents(rec.Body.String())
					require.Equal(t, "error", client[len(client)-1].name)
				}
				require.Equal(t, 2, h.a.count(), "one create + one continue")
				require.Zero(t, h.b.count())
				require.Equal(t, [4]int{10000, 80000, 1000, 20}, threadCommandTokens(h.requireCommands(2)[1]))
			})
		}
	}
}

// threadStreamOutcome is what a client that reads the events sees at the end.
func threadStreamEnd(body string) (stopped, failed bool) {
	for _, event := range threadReadClientEvents(body) {
		kind := gjson.Get(event.data, "type").Str
		if event.name == "message_stop" || kind == "message_stop" {
			stopped = true
		}
		if event.name == "error" && kind == "error" {
			failed = true
		}
	}
	return stopped, failed
}

// The index of a content block is its place in the content list. A stream
// whose blocks leave a place empty is not a whole response: it is not bound
// and it does not end as a success either, the client is told with an error
// event. A legal response that merely cannot be continued, such as a tool call
// the token limit cut off, ends as the success it is, whatever order its
// blocks arrived in.
func TestMessageThreadStreamContentByIndex(t *testing.T) {
	for _, kind := range threadTestKinds() {
		run := func(t *testing.T, mode string) (*threadHarness, string) {
			t.Helper()
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{stream: true}))
			h.requireCommands(1)
			h.a.setMode(mode)
			rec := h.send(threadTurn{previous: first, stream: true})
			h.a.setMode("")
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, [4]int{10000, 80000, 1000, 20}, threadCommandTokens(h.requireCommands(2)[1]), "what arrived is billed")
			require.Equal(t, 2, h.a.count())
			require.Zero(t, h.b.count())
			return h, rec.Body.String()
		}
		t.Run(kind+"/a place in the content is empty", func(t *testing.T) {
			h, body := run(t, "block-gap")
			require.Equal(t, 1, threadBindingCount(h.db), "the upstream did not publish this response, and neither does the gateway")
			stopped, failed := threadStreamEnd(body)
			require.False(t, stopped, "not ended as a success: %s", body)
			require.True(t, failed, "the client is told: %s", body)
			h.requireNotFound(threadTurn{previous: "msg_a_2"})
		})
		t.Run(kind+"/the last block is a cut-off tool call that arrived first", func(t *testing.T) {
			h, body := run(t, "truncated-reordered")
			require.Equal(t, 1, threadBindingCount(h.db), "a response that ends in a cut-off tool call is not continued")
			stopped, failed := threadStreamEnd(body)
			require.True(t, stopped, "a legal response ends as one: %s", body)
			require.False(t, failed, body)
			h.requireNotFound(threadTurn{previous: "msg_a_2"})
		})
		t.Run(kind+"/a whole content list in another order", func(t *testing.T) {
			h, body := run(t, "reordered")
			require.Equal(t, 2, threadBindingCount(h.db))
			stopped, failed := threadStreamEnd(body)
			require.True(t, stopped)
			require.False(t, failed)
			threadResponseID(t, h.send(threadTurn{previous: "msg_a_2"}))
		})
	}
}

// A stream whose structure is not that of a Messages stream does not end as a
// success. Wherever the upstream claims the end of such a stream, the client
// gets an error event in its place; the usage that arrived is billed.
func TestMessageThreadMalformedStreamEndsWithAnError(t *testing.T) {
	for _, kind := range threadTestKinds() {
		for _, mode := range []string{"wrong-block-stop", "reopened-block", "unclosed-block", "mismatched-stop", "misnamed-stop", "untyped-stop", "unnamed-stop", "block-gap"} {
			t.Run(fmt.Sprintf("%s/%s", kind, mode), func(t *testing.T) {
				h := newThreadHarness(t, kind)
				first := threadResponseID(t, h.send(threadTurn{stream: true}))
				h.requireCommands(1)
				h.a.setMode(mode)
				rec := h.send(threadTurn{previous: first, stream: true})
				h.a.setMode("")
				require.Equal(t, http.StatusOK, rec.Code)
				stopped, failed := threadStreamEnd(rec.Body.String())
				require.False(t, stopped, "not ended as a success: %s", rec.Body.String())
				require.True(t, failed, "the client is told: %s", rec.Body.String())
				events := threadReadClientEvents(rec.Body.String())
				require.Equal(t, "error", events[len(events)-1].name)
				errors := 0
				for _, ev := range events {
					if ev.name == "error" || gjson.Get(ev.data, "type").Str == "error" {
						errors++
					}
				}
				require.Equal(t, 1, errors, "one terminal error, no generic error appended by the handler")
				require.Equal(t, 1, threadBindingCount(h.db))
				require.Equal(t, [4]int{10000, 80000, 1000, 20}, threadCommandTokens(h.requireCommands(2)[1]))
				require.Equal(t, 2, h.a.count())
				require.Zero(t, h.b.count())
			})
		}
	}
}

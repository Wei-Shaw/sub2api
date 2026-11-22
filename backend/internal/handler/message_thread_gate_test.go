//go:build unit

package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// A thread request goes to the origin its account was checked for, once. The
// gateway's real HTTP client would follow a redirect and send the request,
// body and credential handling included, to wherever it points; for a thread
// request it must not, and the 3xx it gets back is a failure, not a response.
func TestMessageThreadDoesNotFollowRedirects(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth"} {
		for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect, http.StatusFound} {
			t.Run(fmt.Sprintf("%s/%d", kind, status), func(t *testing.T) {
				h := newThreadHarnessWith(t, kind, threadHarnessOptions{productionHTTP: true})
				// The other origin would accept what the first one was sent.
				h.b.principals["secret-a"] = "caller-a"
				first := threadResponseID(t, h.send(threadTurn{}))
				require.Equal(t, 1, h.a.count())

				h.a.mu.Lock()
				h.a.mode, h.a.redirectTo, h.a.status = "redirect", h.b.srv.URL+"/v1/messages", status
				h.a.mu.Unlock()

				// Control: this client does follow redirects. An ordinary request
				// ends up at the other origin.
				rec := h.send(threadTurn{noThread: true, omitDefs: true})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, 1, h.b.count(), "the production client followed the redirect")
				require.Equal(t, 2, h.a.count())

				for i, turn := range []threadTurn{{previous: first}, {previous: first, stream: true}, {}, {stream: true}} {
					rec := h.send(turn)
					require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
					require.Equal(t, "api_error", gjson.Get(rec.Body.String(), "error.type").Str)
					require.Contains(t, gjson.Get(rec.Body.String(), "error.message").Str, "redirect")
					require.Equal(t, 3+i, h.a.count(), "sent once, to the origin of the account")
					require.Equal(t, 1, h.b.count(), "and carried nowhere else")
				}
				require.Equal(t, 1, threadBindingCount(h.db), "a redirect binds nothing")
				h.requireCommands(2) // the first create and the ordinary request

				// The thread itself was not touched.
				h.a.setMode("")
				threadResponseID(t, h.send(threadTurn{previous: first}))
				require.Equal(t, 1, h.b.count())
			})
		}
	}
}

// A beta policy that cannot be read is not a policy that allows. A thread
// request rebuilds its outbound beta set from stored state, so it is not sent
// on rules that could not be read; an unconfigured policy is something else.
func TestMessageThreadPolicyReadFailureIsNotPermission(t *testing.T) {
	const injected = "interleaved-thinking-2025-05-14"
	for _, kind := range []string{"apikey", "passthrough", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{}))
			calls := h.a.count()

			h.settings.policyReads, h.settings.policyFailFrom = 0, 1
			for _, turn := range []threadTurn{{}, {stream: true}, {previous: first}, {previous: first, stream: true}} {
				rec := h.send(turn)
				require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
				require.Equal(t, "api_error", gjson.Get(rec.Body.String(), "error.type").Str)
				require.False(t, gjson.Get(rec.Body.String(), "error.details").Exists(), "neither a missing thread nor a client error")
			}
			require.Equal(t, calls, h.a.count(), "nothing was sent")
			require.Zero(t, h.b.count())
			require.Equal(t, 1, threadBindingCount(h.db))

			// Requests without a thread keep their behaviour on that failure.
			rec := h.send(threadTurn{noThread: true, omitDefs: true})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			calls = h.a.count()

			// No policy configured at all: nothing to fail.
			h.settings.policyFailFrom, h.settings.policyMissing = 0, true
			second := threadResponseID(t, h.send(threadTurn{previous: first}))
			require.Equal(t, calls+1, h.a.count())
			h.settings.policyMissing = false

			if kind != "oauth" {
				return
			}
			// A rule that only matches a token the gateway adds itself, and a
			// store that answers once and then fails. Every decision of the
			// request comes from that one answer, the outbound one included.
			h.settings.policy = `{"rules":[{"beta_token":"` + injected + `","action":"block","scope":"oauth","error_message":"that beta is not allowed here"}]}`
			for _, turn := range []threadTurn{{previous: second}, {}} {
				h.settings.policyReads, h.settings.policyFailFrom = 0, 2
				rec := h.send(turn)
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "that beta is not allowed here")
				require.Equal(t, 1, h.settings.policyReads, "one snapshot per attempt")
			}
			require.Equal(t, calls+1, h.a.count(), "the blocked token was not sent")
		})
	}
}

// An account header override and the beta policy decide the beta set of a
// thread together, once: the same for the create request and its continue
// requests, with nothing the policy filters and with the client's own opt-in.
func TestMessageThreadHeaderOverrideAndPolicy(t *testing.T) {
	override := func(h *threadHarness, value string) {
		for _, account := range []*service.Account{h.accountA, h.accountB} {
			account.Credentials["header_override_enabled"] = true
			account.Credentials["header_overrides"] = map[string]any{"anthropic-beta": value}
		}
	}
	for _, kind := range []string{"apikey", "passthrough"} {
		t.Run(kind+"/override carries a filtered token", func(t *testing.T) {
			h := newThreadHarness(t, kind)
			override(h, threadTestBeta+",filtered-beta-2026-01-01,operator-beta-2026-03-03")
			h.settings.policy = `{"rules":[{"beta_token":"filtered-beta-2026-01-01","action":"filter","scope":"apikey"}]}`
			first := threadResponseID(t, h.send(threadTurn{}))
			want := "message-threads-2026-08-12,operator-beta-2026-03-03"
			require.Equal(t, want, threadTestCanonicalBeta(h.a.last().header), "the filtered token is not sent, whoever configured it")
			second := threadResponseID(t, h.send(threadTurn{previous: first, stream: true}))
			require.Equal(t, want, threadTestCanonicalBeta(h.a.last().header))
			threadResponseID(t, h.send(threadTurn{previous: second}))
			require.Equal(t, want, threadTestCanonicalBeta(h.a.last().header))
			require.Equal(t, 3, h.a.count(), "the thread its create request made can be continued")
			require.Zero(t, h.b.count())
		})
		t.Run(kind+"/override without the thread beta", func(t *testing.T) {
			h := newThreadHarness(t, kind)
			override(h, "operator-beta-2026-03-03")
			first := threadResponseID(t, h.send(threadTurn{}))
			want := "message-threads-2026-08-12,operator-beta-2026-03-03"
			require.Equal(t, want, threadTestCanonicalBeta(h.a.last().header), "the client's opt-in is kept next to the override")
			threadResponseID(t, h.send(threadTurn{previous: first, stream: true}))
			require.Equal(t, want, threadTestCanonicalBeta(h.a.last().header))
			// Without a thread the override is the whole value, as before.
			rec := h.send(threadTurn{noThread: true, omitDefs: true})
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "operator-beta-2026-03-03", threadTestCanonicalBeta(h.a.last().header))
		})
		t.Run(kind+"/override does not opt a client in", func(t *testing.T) {
			h := newThreadHarness(t, kind)
			override(h, threadTestBeta+",operator-beta-2026-03-03")
			rec := h.send(threadTurn{beta: threadTestPtr("custom-beta-2026-01-01")})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), threadTestBeta)
			rec = h.send(threadTurn{beta: threadTestPtr("")})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Zero(t, h.a.count()+h.b.count())
			require.Zero(t, threadBindingCount(h.db))
		})
	}
}

// A thread that lives on an account is not a new session there. Between a
// limit and its hard ceiling an account keeps serving the sessions it already
// has; that includes its threads, and not past the ceiling.
func TestMessageThreadKeepsItsReservedZone(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy_scheduler=%t", legacy), func(t *testing.T) {
			h := newThreadHarnessWith(t, "oauth-cli", threadHarnessOptions{legacyScheduler: legacy})
			h.accountA.Extra["base_rpm"], h.accountA.Extra["rpm_sticky_buffer"] = 1, 5
			first := threadResponseID(t, h.send(threadTurn{}))
			require.Equal(t, 1, h.a.count())

			// At the limit: only sessions the account already has.
			h.rpm.counts.Store(int64(71), 1)
			rec := h.send(threadTurn{noThread: true})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Equal(t, 2, h.a.count(), "the ordinary request of that session stays on its account")
			second := threadResponseID(t, h.send(threadTurn{previous: first, stream: true}))
			require.Equal(t, 3, h.a.count(), "and so does its thread")
			require.Zero(t, h.b.count())

			// Past the ceiling nothing is served there, the thread included.
			h.rpm.counts.Store(int64(71), 6)
			h.requireNotFound(threadTurn{previous: second})
			rec = h.send(threadTurn{noThread: true})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Equal(t, 3, h.a.count())
			require.Equal(t, 1, h.b.count(), "an ordinary request can move; a thread cannot")

			h.rpm.counts.Store(int64(71), 0)
			threadResponseID(t, h.send(threadTurn{previous: second}))
			require.Equal(t, 4, h.a.count())
		})
	}
}

// A continue request registers its session on the bound account before it
// waits for a slot. When it is never served, a registration it created is
// given back at once; one that was already there is not its to release.
func TestMessageThreadReleasesItsSessionSlotWhenNotServed(t *testing.T) {
	h := newThreadHarnessWith(t, "oauth-cli", threadHarnessOptions{productionSessions: true})
	h.accountA.Extra["max_sessions"], h.accountA.Extra["session_idle_timeout_minutes"] = 1, 1
	active := func() int {
		t.Helper()
		count, err := h.realSessions.GetActiveSessionCount(context.Background(), 71)
		require.NoError(t, err)
		return count
	}
	first := threadResponseID(t, h.send(threadTurn{}))
	require.Equal(t, 1, active())

	// The account is busy and its queue is full: the request is not served.
	h.slots.busy.Store(int64(71), true)
	h.slots.queueFull.Store(true)
	notServed := func() {
		t.Helper()
		before := h.a.count()
		rec := h.send(threadTurn{previous: first, stream: true})
		require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
		require.Equal(t, before, h.a.count())
	}

	// The session is still registered from the create request, which the
	// upstream did serve. This request did not create it and leaves it.
	notServed()
	require.Equal(t, 1, active())

	// The registration has idled out while the thread is still alive. Now the
	// request registers the session itself, and gives it back.
	h.db.SetTime(time.Now().Add(2 * time.Minute))
	require.Zero(t, active())
	notServed()
	require.Zero(t, active(), "a request that was never served does not hold a session slot")

	// Served, it keeps the slot like any session.
	h.slots.busy.Delete(int64(71))
	h.slots.queueFull.Store(false)
	threadResponseID(t, h.send(threadTurn{previous: first}))
	require.Equal(t, 1, active())
	require.True(t, strings.HasPrefix(first, "msg_a_"))
}

// A stored beta policy that cannot be understood is not an empty policy. The
// rules an operator configured are unknown then, and a thread request is not
// sent on the default ones; a policy that is absent or empty is a policy.
func TestMessageThreadPolicyThatCannotBeDecoded(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			previous := threadResponseID(t, h.send(threadTurn{}))
			calls := h.a.count()
			for _, stored := range []string{
				`{"rules":[{"beta_token":"` + threadTestBeta + `","action":"blo`, // cut off
				`{"rules":"all"}`,
				`[]`,
				`not json`,
				`{"rules":[]} trailing`,
				// Decodes, and is not something the settings API can have written.
				`{"rules":[{"beta_token":"some-beta","action":"deny","scope":"all"}]}`,
				`{"rules":[{"beta_token":"some-beta","action":"block","scope":"everyone"}]}`,
				`{"rules":[{"beta_token":"","action":"block","scope":"all"}]}`,
			} {
				h.settings.policy = stored
				for _, turn := range []threadTurn{{}, {stream: true}, {previous: previous}, {previous: previous, stream: true}} {
					rec := h.send(turn)
					require.Equal(t, http.StatusServiceUnavailable, rec.Code, "%s: %s", stored, rec.Body.String())
					require.Equal(t, "api_error", gjson.Get(rec.Body.String(), "error.type").Str)
					require.False(t, gjson.Get(rec.Body.String(), "error.details").Exists())
				}
				require.Equal(t, calls, h.a.count(), "nothing was sent: %s", stored)
				require.Zero(t, h.b.count())
				// Requests without a thread keep their behaviour.
				rec := h.send(threadTurn{noThread: true, omitDefs: true})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				calls = h.a.count()
			}
			require.Equal(t, 1, threadBindingCount(h.db))

			// Policies that say nothing: absent, empty, or without rules.
			for _, stored := range []string{"", `{}`, `{"rules":[]}`, `{"rules":null}`, ` {"rules":[]}` + "\n"} {
				h.settings.policy = stored
				previous = threadResponseID(t, h.send(threadTurn{previous: previous}))
			}
			h.settings.policyMissing = true
			previous = threadResponseID(t, h.send(threadTurn{previous: previous, stream: true}))
			h.settings.policyMissing = false
			// And one that says something is applied.
			h.settings.policy = `{"rules":[{"beta_token":"` + threadTestBeta + `","action":"block","scope":"all","error_message":"threads are switched off"}]}`
			rec := h.send(threadTurn{previous: previous})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "threads are switched off")
			require.Zero(t, h.b.count())
		})
	}
}

// The session limit of a subscription account is admission. For a thread
// request its four answers are four outcomes: no limit configured and room
// under the limit let the request through, no room is a thread that cannot be
// continued here, and a store that cannot say is a failure, not room.
func TestMessageThreadSessionLimitAdmission(t *testing.T) {
	for _, kind := range []string{"oauth", "oauth-cli", "setup-token"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{}))
			unavailable := func(turn threadTurn) {
				t.Helper()
				a, b := h.a.count(), h.b.count()
				rec := h.send(turn)
				require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
				require.Equal(t, "api_error", gjson.Get(rec.Body.String(), "error.type").Str)
				require.False(t, gjson.Get(rec.Body.String(), "error.details").Exists())
				require.Equal(t, a, h.a.count(), "nothing was sent")
				require.Equal(t, b, h.b.count())
			}

			// The store cannot be asked.
			h.sessions.fail.Store(true)
			for _, turn := range []threadTurn{{previous: first}, {previous: first, stream: true}, {}, {stream: true}} {
				unavailable(turn)
			}
			require.Equal(t, 1, threadBindingCount(h.db))
			// A request without a thread keeps its behaviour on that failure.
			rec := h.send(threadTurn{noThread: true})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			h.sessions.fail.Store(false)

			// The store answers: no room. The thread cannot go on here, and its
			// messages go nowhere else.
			h.sessions.full.Store(true)
			calls := h.a.count()
			h.requireNotFound(threadTurn{previous: first})
			require.Equal(t, calls, h.a.count())
			require.Zero(t, h.b.count())
			h.sessions.full.Store(false)

			// Room: the thread goes on.
			second := threadResponseID(t, h.send(threadTurn{previous: first, stream: true}))

			// No limit configured: the store is not asked, so it cannot fail.
			h.accountA.Extra["max_sessions"] = 0
			h.sessions.fail.Store(true)
			threadResponseID(t, h.send(threadTurn{previous: second}))
			require.Zero(t, h.b.count())
		})
	}
	t.Run("a limit without a store", func(t *testing.T) {
		h := newThreadHarnessWith(t, "oauth", threadHarnessOptions{noSessionStore: true})
		for _, turn := range []threadTurn{{}, {stream: true}} {
			rec := h.send(turn)
			require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
		}
		require.Zero(t, h.a.count()+h.b.count())
		rec := h.send(threadTurn{noThread: true})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	})
	t.Run("an API key has no session limit", func(t *testing.T) {
		h := newThreadHarness(t, "apikey")
		h.sessions.fail.Store(true)
		first := threadResponseID(t, h.send(threadTurn{}))
		threadResponseID(t, h.send(threadTurn{previous: first, stream: true}))
	})
}

//go:build unit

package handler

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// threadLimit is one of the two soft limits of a subscription account that
// the scheduler evaluates from stored counters: requests per minute, and the
// cost of the current usage window. Both have a zone below the limit, a zone
// above it that is kept for sessions already on the account, and a ceiling.
type threadLimit struct {
	name   string
	enable func(account *service.Account)
	// set puts an account in a zone: "empty" (nothing stored at all),
	// "green", "yellow" (kept for existing sessions) or "red".
	set func(h *threadHarness, accountID int64, zone string)
	// faults are ways the limit's data cannot be read. Each returns its undo.
	faults map[string]func(h *threadHarness) func()
	// reads is how often the limit's data was read for a single account.
	reads func(h *threadHarness) int32
}

var threadLimits = []threadLimit{
	{
		name: "rpm",
		enable: func(account *service.Account) {
			account.Extra["base_rpm"], account.Extra["rpm_sticky_buffer"] = 2, 3
		},
		set: func(h *threadHarness, accountID int64, zone string) {
			h.rpm.counts.Store(accountID, map[string]int{"empty": 0, "green": 1, "yellow": 2, "red": 5}[zone])
			if zone == "empty" {
				h.rpm.counts.Delete(accountID)
			}
		},
		faults: map[string]func(h *threadHarness) func(){
			// The scheduler's batched read still answers, with zero.
			"the read fails": func(h *threadHarness) func() {
				h.rpm.failRead.Store(true)
				return func() { h.rpm.failRead.Store(false) }
			},
			"the read and the batched read fail": func(h *threadHarness) func() {
				h.rpm.failRead.Store(true)
				h.rpm.failBatch.Store(true)
				return func() { h.rpm.failRead.Store(false); h.rpm.failBatch.Store(false) }
			},
		},
		reads: func(h *threadHarness) int32 { return h.rpm.reads.Load() },
	},
	{
		name: "window cost",
		enable: func(account *service.Account) {
			account.Extra["window_cost_limit"], account.Extra["window_cost_sticky_reserve"] = 100.0, 10.0
		},
		set: func(h *threadHarness, accountID int64, zone string) {
			if zone == "empty" {
				h.sessions.costs.Delete(accountID)
				h.costs.costs.Delete(accountID)
				return
			}
			cost := map[string]float64{"green": 50, "yellow": 105, "red": 120}[zone]
			h.sessions.costs.Store(accountID, cost)
			h.costs.costs.Store(accountID, cost)
		},
		faults: map[string]func(h *threadHarness) func(){
			"the cache misses and the state behind it fails": func(h *threadHarness) func() {
				h.sessions.costs.Delete(int64(71))
				h.sessions.costs.Delete(int64(72))
				h.costs.fail.Store(true)
				return func() { h.costs.fail.Store(false) }
			},
			"the cache and the state behind it fail": func(h *threadHarness) func() {
				h.sessions.costFail.Store(true)
				h.costs.fail.Store(true)
				return func() { h.sessions.costFail.Store(false); h.costs.fail.Store(false) }
			},
		},
		reads: func(h *threadHarness) int32 { return h.sessions.costReads.Load() + h.costs.reads.Load() },
	},
}

func (h *threadHarness) requireUnavailable(turn threadTurn) {
	h.t.Helper()
	a, b := h.a.count(), h.b.count()
	rec := h.send(turn)
	require.Equal(h.t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	require.Equal(h.t, "api_error", gjson.Get(rec.Body.String(), "error.type").Str)
	require.False(h.t, gjson.Get(rec.Body.String(), "error.details").Exists(), "neither a missing thread nor a client error")
	require.Equal(h.t, a, h.a.count(), "nothing was sent")
	require.Equal(h.t, b, h.b.count(), "and nothing was tried elsewhere")
}

// servedBy sends a request that must succeed and reports which upstream got it.
func (h *threadHarness) servedBy(turn threadTurn) (id, upstream string) {
	h.t.Helper()
	a, b := h.a.count(), h.b.count()
	id = threadResponseID(h.t, h.send(turn))
	switch {
	case h.a.count() == a+1 && h.b.count() == b:
		return id, "a"
	case h.b.count() == b+1 && h.a.count() == a:
		return id, "b"
	}
	h.t.Fatalf("one request reached the upstreams %d and %d times", h.a.count()-a, h.b.count()-b)
	return "", ""
}

// The zones of a limit, for a thread request, from data that can be read.
func TestMessageThreadRateAndCostLimitZones(t *testing.T) {
	for _, limit := range threadLimits {
		t.Run(limit.name, func(t *testing.T) {
			h := newThreadHarness(t, "oauth-cli")
			limit.enable(h.accountA)

			// Nothing stored at all is an account that has used nothing.
			limit.set(h, 71, "empty")
			first, on := h.servedBy(threadTurn{})
			require.Equal(t, "a", on)
			second, on := h.servedBy(threadTurn{previous: first, stream: true})
			require.Equal(t, "a", on)

			limit.set(h, 71, "green")
			third, on := h.servedBy(threadTurn{previous: second})
			require.Equal(t, "a", on)

			// Above the limit the account keeps what it already has: the thread
			// that lives on it, and a thread created by a session bound to it.
			limit.set(h, 71, "yellow")
			fourth, on := h.servedBy(threadTurn{previous: third, stream: true})
			require.Equal(t, "a", on, "the thread stays in the zone kept for existing sessions")
			_, on = h.servedBy(threadTurn{noThread: true})
			require.Equal(t, "a", on, "like the ordinary request of that session")
			_, on = h.servedBy(threadTurn{})
			require.Equal(t, "a", on, "and a thread that session creates")
			// A thread created by anyone else goes elsewhere.
			_, on = h.servedBy(threadTurn{plain: true})
			require.Equal(t, "b", on)

			// At the ceiling nothing is served there. The thread's messages are
			// not offered to another account.
			limit.set(h, 71, "red")
			h.requireNotFound(threadTurn{previous: fourth})
			h.requireNotFound(threadTurn{previous: fourth, stream: true})
			_, on = h.servedBy(threadTurn{})
			require.Equal(t, "b", on)

			limit.set(h, 71, "empty")
			_, on = h.servedBy(threadTurn{previous: fourth})
			require.Equal(t, "a", on)
		})
	}
}

// A limit that is configured is evaluated from its data. When the data cannot
// be read there is no count to compare, and zero is a guess: a continue
// request fails locally and is tried nowhere, a create request passes the
// account over. Requests without a thread keep what they always did.
func TestMessageThreadRateAndCostLimitFaults(t *testing.T) {
	for _, limit := range threadLimits {
		for fault, inject := range limit.faults {
			t.Run(limit.name+"/"+fault, func(t *testing.T) {
				h := newThreadHarness(t, "oauth-cli")
				limit.enable(h.accountA)
				limit.set(h, 71, "green")
				first, on := h.servedBy(threadTurn{})
				require.Equal(t, "a", on)

				restore := inject(h)
				h.requireUnavailable(threadTurn{previous: first})
				h.requireUnavailable(threadTurn{previous: first, stream: true})
				require.Equal(t, 1, threadBindingCount(h.db))

				// A create request is not sent to the account whose limit is
				// unknown. Another account, one without this limit, serves it.
				created, on := h.servedBy(threadTurn{plain: true})
				require.Equal(t, "b", on)
				_, on = h.servedBy(threadTurn{previous: created, plain: true})
				require.Equal(t, "b", on, "an account without the limit does not depend on its data")

				// The ordinary request of the session: served as before.
				_, on = h.servedBy(threadTurn{noThread: true})
				require.Equal(t, "a", on, "requests without a thread keep their behaviour on that failure")

				// With the limit on every account there is none to create on.
				limit.enable(h.accountB)
				h.requireUnavailable(threadTurn{plain: true})
				h.requireUnavailable(threadTurn{stream: true})
				delete(h.accountB.Extra, "base_rpm")
				delete(h.accountB.Extra, "window_cost_limit")

				restore()
				limit.set(h, 71, "green")
				_, on = h.servedBy(threadTurn{previous: first})
				require.Equal(t, "a", on, "the thread itself was not touched")
			})
		}
	}

	t.Run("rpm/no store", func(t *testing.T) {
		h := newThreadHarnessWith(t, "oauth", threadHarnessOptions{noRPMStore: true})
		// Without the limit the store is not needed.
		first, on := h.servedBy(threadTurn{})
		require.Equal(t, "a", on)
		threadLimits[0].enable(h.accountA)
		h.requireUnavailable(threadTurn{previous: first})
		_, on = h.servedBy(threadTurn{noThread: true})
		require.Equal(t, "a", on, "a request without a thread is served as before")
		_, on = h.servedBy(threadTurn{})
		require.Equal(t, "b", on)
	})
	t.Run("window cost/no state behind the cache", func(t *testing.T) {
		h := newThreadHarnessWith(t, "oauth", threadHarnessOptions{noWindowCostState: true})
		first, on := h.servedBy(threadTurn{})
		require.Equal(t, "a", on)
		threadLimits[1].enable(h.accountA)
		// A cached value is an answer.
		h.sessions.costs.Store(int64(71), 50.0)
		second, on := h.servedBy(threadTurn{previous: first})
		require.Equal(t, "a", on)
		// A miss with nothing to ask is not zero.
		h.sessions.costs.Delete(int64(71))
		h.requireUnavailable(threadTurn{previous: second})
		_, on = h.servedBy(threadTurn{noThread: true})
		require.Equal(t, "a", on, "a request without a thread is served as before")
		_, on = h.servedBy(threadTurn{})
		require.Equal(t, "b", on)
	})
}

// What the scheduler prefetched is not the answer for a thread request: the
// account that was selected is read again, by itself.
func TestMessageThreadLimitIsReadForTheSelectedAccount(t *testing.T) {
	t.Run("rpm: the batched read fails, the single read answers", func(t *testing.T) {
		h := newThreadHarness(t, "oauth-cli")
		threadLimits[0].enable(h.accountA)
		first, _ := h.servedBy(threadTurn{})
		// A failed prefetch is not a failed limit: the counter can be read.
		h.rpm.failBatch.Store(true)
		threadLimits[0].set(h, 71, "yellow")
		reads := h.rpm.reads.Load()
		second, on := h.servedBy(threadTurn{previous: first})
		require.Equal(t, "a", on, "the zone kept for existing sessions holds")
		require.Greater(t, h.rpm.reads.Load(), reads)
		threadLimits[0].set(h, 71, "red")
		h.requireNotFound(threadTurn{previous: second})
		_, on = h.servedBy(threadTurn{plain: true})
		require.Equal(t, "b", on, "a create request is not sent to an account at its ceiling")
	})
	t.Run("rpm: the prefetch is older than the counter", func(t *testing.T) {
		h := newThreadHarness(t, "oauth-cli")
		threadLimits[0].enable(h.accountA)
		first, _ := h.servedBy(threadTurn{})
		// The scheduler works from a prefetch that still says zero and selects
		// the account. Only the read for the selected account sees the ceiling.
		h.rpm.staleBatch.Store(true)
		threadLimits[0].set(h, 71, "red")
		_, on := h.servedBy(threadTurn{noThread: true, plain: true})
		require.Equal(t, "a", on, "the scheduler alone lets a request through")
		h.requireNotFound(threadTurn{previous: first})
		_, on = h.servedBy(threadTurn{plain: true})
		require.Equal(t, "b", on, "a create request is not sent to an account at its ceiling")
		threadLimits[0].set(h, 71, "yellow")
		_, on = h.servedBy(threadTurn{plain: true})
		require.Equal(t, "b", on, "nor into the zone kept for the account's own sessions")
		_, on = h.servedBy(threadTurn{previous: first})
		require.Equal(t, "a", on)
	})
	t.Run("window cost: the cache fails, the state behind it answers", func(t *testing.T) {
		h := newThreadHarness(t, "oauth-cli")
		threadLimits[1].enable(h.accountA)
		first, _ := h.servedBy(threadTurn{})
		h.sessions.costFail.Store(true)
		// The authoritative state says: used little.
		h.costs.costs.Store(int64(71), 50.0)
		reads := h.costs.reads.Load()
		second, on := h.servedBy(threadTurn{previous: first})
		require.Equal(t, "a", on)
		require.Greater(t, h.costs.reads.Load(), reads, "answered by the state behind the cache")
		// And now: at the ceiling.
		h.costs.costs.Store(int64(71), 120.0)
		h.requireNotFound(threadTurn{previous: second})
		_, on = h.servedBy(threadTurn{plain: true})
		require.Equal(t, "b", on)
	})
	t.Run("window cost: a cache miss is answered by the state behind it", func(t *testing.T) {
		h := newThreadHarness(t, "oauth-cli")
		threadLimits[1].enable(h.accountA)
		first, _ := h.servedBy(threadTurn{})
		h.sessions.costs.Delete(int64(71))
		h.costs.costs.Store(int64(71), 120.0)
		h.requireNotFound(threadTurn{previous: first})
		h.sessions.costs.Delete(int64(71))
		h.costs.costs.Store(int64(71), 105.0)
		_, on := h.servedBy(threadTurn{previous: first})
		require.Equal(t, "a", on)
		cached, ok := h.sessions.costs.Load(int64(71))
		require.True(t, ok)
		require.Equal(t, 105.0, cached)
	})
}

// A limit that is not configured reads nothing, so nothing can fail.
// The scheduler snapshot can predate a newly enabled or tightened limit.
// Only the final row reload sees the new configuration in these cases.
func TestMessageThreadFinalRowLimitAdmission(t *testing.T) {
	for _, limit := range threadLimits {
		for _, state := range []string{"enabled", "tightened", "unreadable", "allowed"} {
			t.Run(limit.name+"/"+state, func(t *testing.T) {
				h := newThreadHarness(t, "oauth-cli")
				if state == "tightened" {
					if limit.name == "rpm" {
						h.accountA.Extra["base_rpm"] = 1000
					} else {
						h.accountA.Extra["window_cost_limit"] = 1000.0
					}
				}
				id := threadResponseID(t, h.send(threadTurn{}))
				h.repo.set(71, h.copyOfA(limit.enable))
				limit.set(h, 71, "red")
				turn := threadTurn{previous: id, stream: true}
				switch state {
				case "unreadable":
					if limit.name == "rpm" {
						h.rpm.failRead.Store(true)
					} else {
						h.sessions.costFail.Store(true)
						h.costs.fail.Store(true)
					}
					h.requireUnavailable(turn)
				case "allowed":
					limit.set(h, 71, "green")
					_, upstream := h.servedBy(turn)
					require.Equal(t, "a", upstream)
					h.requireCommands(2)
					return
				default:
					h.requireNotFound(turn)
				}
				h.requireCommands(1) // Only the successful create is metered.
			})
		}
	}
}

func TestMessageThreadLimitsNotConfigured(t *testing.T) {
	for _, kind := range []string{"oauth", "setup-token", "apikey"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			h.rpm.failRead.Store(true)
			h.rpm.failBatch.Store(true)
			h.sessions.costFail.Store(true)
			h.costs.fail.Store(true)
			if kind == "apikey" {
				// The limits belong to subscription accounts; on an API key the
				// same settings mean nothing.
				for _, limit := range threadLimits {
					limit.enable(h.accountA)
				}
			}
			first, on := h.servedBy(threadTurn{})
			require.Equal(t, "a", on)
			second, on := h.servedBy(threadTurn{previous: first, stream: true})
			require.Equal(t, "a", on)
			_, on = h.servedBy(threadTurn{previous: second})
			require.Equal(t, "a", on)
			for _, limit := range threadLimits {
				require.Zero(t, limit.reads(h), limit.name)
			}
		})
	}
}

//go:build unit

package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Use the repository's session adapter and Lua scripts against miniredis.
// A reproducible clock tests the session's idle deadline, not the binding TTL.
func newThreadSessionHarness(t *testing.T) (*threadHarness, time.Time) {
	t.Helper()
	h := newThreadHarnessWith(t, "oauth-cli", threadHarnessOptions{productionSessions: true})
	start := time.Unix(1_800_000_000, 0)
	h.db.SetTime(start)
	h.accountA.Extra["max_sessions"], h.accountA.Extra["session_idle_timeout_minutes"] = 1, 1
	return h, start
}

func requireThreadActiveSessions(t *testing.T, h *threadHarness, want int) {
	t.Helper()
	count, err := h.realSessions.GetActiveSessionCount(context.Background(), h.accountA.ID)
	require.NoError(t, err)
	require.Equal(t, want, count)
}

func TestMessageThreadSharedSessionSurvivesFailedTurn(t *testing.T) {
	for _, kind := range []string{"create", "continue"} {
		t.Run(kind, func(t *testing.T) {
			h, start := newThreadSessionHarness(t)
			id := threadResponseID(t, h.send(threadTurn{}))
			record, err := h.db.Get(threadBindingKeys(h.db)[0])
			require.NoError(t, err)
			session := gjson.Get(record, "s").String()
			require.NotEmpty(t, session)
			h.db.SetTime(start.Add(2 * time.Minute))
			requireThreadActiveSessions(t, h, 0)

			// Interleave another holder's refresh with this request: after
			// registration, but before the failing response reaches the handler.
			client := h.a.srv.Client()
			transport := client.Transport
			t.Cleanup(func() { client.Transport = transport })
			client.Transport = threadFixtureTransport(func(req *http.Request) (*http.Response, error) {
				requireThreadActiveSessions(t, h, 1)
				active, err := h.realSessions.IsSessionActive(context.Background(), h.accountA.ID, session)
				require.NoError(t, err)
				require.True(t, active)
				ok, err := h.realSessions.RegisterSession(context.Background(), h.accountA.ID, session, 1, time.Minute)
				require.NoError(t, err)
				require.True(t, ok)
				return transport.RoundTrip(req)
			})
			h.a.setMode("thread-error-raw")
			turn := threadTurn{stream: true}
			if kind == "continue" {
				turn.previous = id
			}
			rec := h.send(turn)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Equal(t, 2, h.a.count())
			require.Zero(t, h.b.count())
			requireThreadActiveSessions(t, h, 1)
		})
	}
}

func TestMessageThreadUnsentRegistrationExpires(t *testing.T) {
	h, start := newThreadSessionHarness(t)
	id := threadResponseID(t, h.send(threadTurn{}))
	start = start.Add(2 * time.Minute)
	h.db.SetTime(start)
	requireThreadActiveSessions(t, h, 0)

	client := h.a.srv.Client()
	original := client.Transport
	t.Cleanup(func() { client.Transport = original })
	calls := 0
	client.Transport = threadFixtureTransport(func(*http.Request) (*http.Response, error) {
		calls++
		requireThreadActiveSessions(t, h, 1)
		return nil, errors.New("local transport closed before sending")
	})
	rec := h.send(threadTurn{previous: id})
	require.GreaterOrEqual(t, rec.Code, 400)
	require.Equal(t, 1, calls)
	require.Equal(t, 1, h.a.count(), "only the create reached the fake upstream")
	require.Zero(t, h.b.count())
	h.requireCommands(1)
	h.db.SetTime(start.Add(59 * time.Second))
	requireThreadActiveSessions(t, h, 1)
	h.db.SetTime(start.Add(time.Minute))
	requireThreadActiveSessions(t, h, 0)
	client.Transport = original
	threadResponseID(t, h.send(threadTurn{previous: id}))
	requireThreadActiveSessions(t, h, 1)
}

func TestMessageThreadCreatePassesOverFullSessionAccount(t *testing.T) {
	h, _ := newThreadSessionHarness(t)
	ok, err := h.realSessions.RegisterSession(context.Background(), h.accountA.ID, "another-session", 1, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	_, upstream := h.servedBy(threadTurn{})
	require.Equal(t, "b", upstream)
	require.Zero(t, h.a.count())
	active, err := h.realSessions.IsSessionActive(context.Background(), h.accountA.ID, "another-session")
	require.NoError(t, err)
	require.True(t, active)
}

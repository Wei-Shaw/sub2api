//go:build unit

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMessageThreadBetaRepresentation(t *testing.T) {
	const feature = "context-management-2025-06-27"
	const field = `{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}`
	for _, kind := range []string{"apikey", "passthrough", "oauth-cli"} {
		firstLine := threadTestBeta
		if kind == "oauth-cli" {
			firstLine = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14," + firstLine
		}
		joined := firstLine + "," + feature
		turn := func(split bool) threadTurn {
			if split {
				return threadTurn{beta: &firstLine, betaLines: []string{feature}}
			}
			return threadTurn{beta: &joined}
		}
		for _, split := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/split=%t", kind, split), func(t *testing.T) {
				h := newThreadHarness(t, kind)
				h.fixed = `"context_management":` + field
				id, where := h.servedBy(turn(split))
				require.Equal(t, "a", where)
				check := func() string {
					t.Helper()
					sent := h.a.last()
					tokens := threadTestBetaTokens(sent.header)
					require.Contains(t, tokens, feature)
					require.ElementsMatch(t, strings.Split(joined, ","), tokens)
					require.JSONEq(t, field, gjson.GetBytes(sent.body, "context_management").Raw)
					return strings.Join(tokens, ",")
				}
				initial := check()
				next := turn(!split)
				next.previous = id
				_, where = h.servedBy(next)
				require.Equal(t, "a", where)
				require.Equal(t, initial, check())
				h.requireCommands(2)
			})
		}
		for _, policy := range []string{"filter", "override"} {
			t.Run(kind+"/"+policy, func(t *testing.T) {
				h := newThreadHarness(t, kind)
				h.fixed = `"context_management":` + field
				if policy == "filter" {
					h.settings.policy = `{"rules":[{"beta_token":"` + feature + `","action":"filter","scope":"all"}]}`
				} else {
					h.accountA.Credentials["header_override_enabled"] = true
					h.accountA.Credentials["header_overrides"] = map[string]any{"anthropic-beta": firstLine}
				}
				create := turn(true)
				id, where := h.servedBy(create)
				require.Equal(t, "a", where)
				check := func() {
					t.Helper()
					sent := h.a.last()
					if policy == "override" && kind == "oauth-cli" {
						// OAuth accounts are not eligible for API-key header overrides.
						require.Contains(t, threadTestBetaTokens(sent.header), feature)
						require.JSONEq(t, field, gjson.GetBytes(sent.body, "context_management").Raw)
						return
					}
					require.NotContains(t, threadTestBetaTokens(sent.header), feature)
					require.False(t, gjson.GetBytes(sent.body, "context_management").Exists())
				}
				check()
				next := turn(false)
				next.previous = id
				_, where = h.servedBy(next)
				require.Equal(t, "a", where)
				check()
			})
		}
	}
}

// Read the same production RPM adapter the gateway writes, in a fixed Redis minute.
func threadRPMReader(t *testing.T, h *threadHarness) func(int64) int {
	t.Helper()
	h.db.SetTime(time.Unix(1_800_000_000, 0))
	kv := redis.NewClient(&redis.Options{Addr: h.db.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = kv.Close() })
	counter := repository.NewRPMCache(kv)
	return func(id int64) int {
		t.Helper()
		n, err := counter.GetRPM(context.Background(), id)
		require.NoError(t, err)
		return n
	}
}

func TestMessageThreadSuccessfulRPM(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		snapshot, latest, success bool
		inc                       int
	}{
		{"enabled", true, true, true, 1}, {"enabled_at_final_check", false, true, true, 1},
		{"disabled_at_final_check", true, false, true, 0}, {"upstream_refused", false, true, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newThreadHarnessWith(t, "oauth-cli", threadHarnessOptions{productionRPM: true})
			rpm := threadRPMReader(t, h)
			if tc.snapshot {
				threadLimits[0].enable(h.accountA)
			}
			id, where := h.servedBy(threadTurn{})
			require.Equal(t, "a", where)
			h.repo.set(h.accountA.ID, h.copyOfA(func(row *service.Account) {
				if tc.latest {
					threadLimits[0].enable(row)
				} else {
					delete(row.Extra, "base_rpm")
				}
			}))
			before := rpm(h.accountA.ID)
			if tc.snapshot {
				require.Equal(t, 1, before)
			} else {
				require.Zero(t, before)
			}
			snapshotRPM := h.accountA.GetBaseRPM()
			if tc.success {
				_, where = h.servedBy(threadTurn{previous: id})
				require.Equal(t, "a", where)
				h.requireCommands(2)
			} else {
				h.a.setMode("forbidden")
				rec := h.send(threadTurn{previous: id})
				require.GreaterOrEqual(t, rec.Code, 400, rec.Body.String())
				require.Equal(t, 2, h.a.count())
				require.Zero(t, h.b.count())
				h.requireCommands(1)
			}
			require.Equal(t, before+tc.inc, rpm(h.accountA.ID), "RPM follows the admitted row only on success")
			require.Equal(t, snapshotRPM, h.accountA.GetBaseRPM(), "the scheduling snapshot stays unchanged")
		})
	}
	for _, enabledOnB := range []bool{false, true} {
		t.Run(fmt.Sprintf("create_failover/enabled_on_b=%t", enabledOnB), func(t *testing.T) {
			h := newThreadHarnessWith(t, "oauth-cli", threadHarnessOptions{productionRPM: true})
			rpm := threadRPMReader(t, h)
			if enabledOnB {
				threadLimits[0].enable(h.accountB)
			} else {
				threadLimits[0].enable(h.accountA)
			}
			h.a.setMode("overloaded")
			threadResponseID(t, h.send(threadTurn{}))
			require.Positive(t, h.a.count())
			require.Equal(t, 1, h.b.count())
			h.requireCommands(1)
			require.Zero(t, rpm(h.accountA.ID))
			want := 0
			if enabledOnB {
				want = 1
			}
			require.Equal(t, want, rpm(h.accountB.ID))
		})
	}
}

func TestMessageThreadMixedCapacity(t *testing.T) {
	incapable := func(a *service.Account) {
		a.Extra["custom_base_url_enabled"] = true
		a.Extra["custom_base_url"] = "https://unused.invalid"
	}
	for _, kind := range []string{"full", "full_and_incapable", "only_incapable"} {
		t.Run(kind, func(t *testing.T) {
			h, _ := newThreadSessionHarness(t)
			if kind == "full" {
				h.accountB.Status = service.StatusDisabled
			} else {
				incapable(h.accountB)
			}
			if kind == "only_incapable" {
				incapable(h.accountA)
			} else {
				ok, err := h.realSessions.RegisterSession(context.Background(), h.accountA.ID, "another-session", 1, time.Minute)
				require.NoError(t, err)
				require.True(t, ok)
			}
			rec := h.send(threadTurn{})
			require.Zero(t, h.a.count())
			require.Zero(t, h.b.count())
			h.requireCommands(0)
			if kind == "only_incapable" {
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Equal(t, "thread_unsupported_request", gjson.Get(rec.Body.String(), "error.details.error_code").Str)
			} else {
				require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "no account has room")
			}
		})
	}
	t.Run("unreadable_and_incapable", func(t *testing.T) {
		h := newThreadHarnessWith(t, "oauth-cli", threadHarnessOptions{noSessionStore: true})
		incapable(h.accountB)
		rec := h.send(threadTurn{})
		require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
		require.NotContains(t, rec.Body.String(), "no account has room")
		require.Zero(t, h.a.count()+h.b.count())
		h.requireCommands(0)
	})
	t.Run("upstream_failure", func(t *testing.T) {
		// An actual upstream error still takes the normal failover path.
		h, _ := newThreadSessionHarness(t)
		incapable(h.accountB)
		h.a.setMode("overloaded")
		rec := h.send(threadTurn{})
		require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
		require.Equal(t, "overloaded_error", gjson.Get(rec.Body.String(), "error.type").Str)
		require.NotContains(t, rec.Body.String(), "no account has room")
		require.NotContains(t, rec.Body.String(), "thread_unsupported_request")
		require.Positive(t, h.a.count())
		require.Zero(t, h.b.count())
		h.requireCommands(0)
	})
}

// The same compatible origin reports usage on message_start or message_delta.
// Compare the client's fields independently of the one settled usage command.
func TestMessageThreadClientCacheUsage(t *testing.T) {
	for _, tc := range []struct {
		name, kind           string
		ordinary, delta      bool
		standard             string
		wantClient, wantBill int64
	}{
		{"ordinary", "apikey", true, false, "", 80000, 80000},
		{"thread", "apikey", false, false, "", 80000, 80000},
		{"thread_delta", "apikey", false, true, "", 80000, 80000},
		{"standard_wins", "apikey", false, false, `"cache_read_input_tokens":17000,`, 17000, 17000},
		{"zero_standard", "apikey", false, false, `"cache_read_input_tokens":0,`, 80000, 80000},
		{"passthrough", "passthrough", false, false, "", 0, 80000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newThreadHarness(t, tc.kind)
			h.accountA.Credentials["base_url"] = h.a.srv.URL
			h.cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			h.accountA.Extra[service.MessageThreadAccountFlag] = true
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
				text := string(raw)
				alias := tc.standard + `"cached_tokens":80000,`
				if tc.delta {
					text = strings.Replace(text, `"cache_read_input_tokens":80000,`, "", 1)
					text = strings.Replace(text, `"usage":{"output_tokens":20`, `"usage":{`+alias+`"output_tokens":20`, 1)
				} else {
					text = strings.Replace(text, `"cache_read_input_tokens":80000,`, alias, 1)
				}
				resp.Body = io.NopCloser(strings.NewReader(text))
				return resp, nil
			})
			t.Cleanup(func() { client.Transport = transport })
			rec := h.send(threadTurn{stream: true, noThread: tc.ordinary})
			threadResponseID(t, rec)
			cmd := h.requireCommands(1)[0]
			require.EqualValues(t, tc.wantBill, cmd.CacheReadTokens)
			event, path := "message_start", "message.usage"
			if tc.delta {
				event, path = "message_delta", "usage"
			}
			for _, ev := range threadReadClientEvents(rec.Body.String()) {
				if ev.name == event {
					u := gjson.Get(ev.data, path)
					require.EqualValues(t, 80000, u.Get("cached_tokens").Int())
					standard := u.Get("cache_read_input_tokens")
					if tc.kind == "passthrough" {
						require.False(t, standard.Exists())
					} else {
						require.Equal(t, gjson.Number, standard.Type)
						require.Equal(t, tc.wantClient, standard.Int())
					}
					return
				}
			}
			t.Fatalf("missing %s", event)
		})
	}
}

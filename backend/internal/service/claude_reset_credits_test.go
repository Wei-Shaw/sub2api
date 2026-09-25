package service

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type resetAccountStub struct{ account *Account }

func (s resetAccountStub) GetByID(context.Context, int64) (*Account, error) { return s.account, nil }

type resetTokenStub struct{}

func (resetTokenStub) GetAccessToken(context.Context, *Account) (string, error) {
	return "synthetic-token", nil
}
func TestClaudeResetStatusNativeContract(t *testing.T) {
	now := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	s := &ClaudeResetCreditService{accounts: resetAccountStub{&Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"scope": "user:profile user:inference"}}}, tokens: resetTokenStub{}, now: func() time.Time { return now }}
	s.do = func(r *http.Request, p string) (*http.Response, error) {
		require.Equal(t, claudeResetUsageURL, r.URL.String())
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "Bearer synthetic-token", r.Header.Get("Authorization"))
		require.Contains(t, r.Header.Get("User-Agent"), "claude-cli/")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"cedar_ember":{"eligible":true,"at_limit":false,"next_grant_id":"launch","grants":[{"id":"launch","resets_left":2,"usable_now":true,"use_requires_limit":false,"ends_at":"2026-10-22T00:00:00Z","clears":["five_hour"],"percent_used":{"five_hour":65},"blocking":[]},{"id":"later","clears":["five_hour"],"resets_left":1,"usable_now":true,"use_requires_limit":false}]}}`))}, nil
	}
	r, e := s.Query(context.Background(), 1)
	require.NoError(t, e)
	require.Equal(t, 2, r.AvailableCount)
	require.True(t, r.Credits[0].Redeemable)
	require.False(t, r.Credits[1].Redeemable)
	b, e := json.Marshal(r)
	require.NoError(t, e)
	require.NotContains(t, string(b), `"id"`)
	require.NotContains(t, string(b), "launch")
	require.NotContains(t, string(b), "synthetic-token")
}
func TestClaudeResetEligibilityFailClosed(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	no := false
	base := claudeResetGrant{ID: "grant", ResetsLeft: 1, UsableNow: true, UseRequiresLimit: &no, Clears: []string{"five_hour"}}
	for _, name := range []string{"paused", "expired", "future", "cooldown", "requires-limit", "blocking", "ineligible", "spent", "not-next"} {
		t.Run(name, func(t *testing.T) {
			g := base
			b := &claudeResetBlock{Eligible: true, NextGrantID: g.ID}
			switch name {
			case "paused":
				g.Paused = true
			case "expired":
				g.EndsAt = &past
			case "future":
				g.StartsAt = &future
			case "cooldown":
				b.CooldownUntil = &future
			case "requires-limit":
				g.UseRequiresLimit = nil
			case "blocking":
				g.Blocking = []string{"seven_day"}
			case "ineligible":
				b.Eligible = false
			case "spent":
				g.ResetsLeft = 0
			case "not-next":
				b.NextGrantID = "other"
			}
			b.Grants = []claudeResetGrant{g}
			r := projectClaudeResetCredits(b, now)
			require.Zero(t, r.AvailableCount)
		})
	}
}
func TestClaudeResetStatusRejectsMissingScopeBeforeNetwork(t *testing.T) {
	s := &ClaudeResetCreditService{accounts: resetAccountStub{&Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}}, do: func(*http.Request, string) (*http.Response, error) { t.Fatal("network called"); return nil, nil }}
	_, e := s.Query(context.Background(), 1)
	require.Error(t, e)
}
func TestClaudeResetMalformedAndAbsent(t *testing.T) {
	for _, body := range []string{`{}`, `{"five_hour":{},"cedar_ember":null}`, `{"cedar_ember":{"eligible":true}}`, `{"error":{"message":"private upstream data"}}`, `not json`} {
		t.Run(body, func(t *testing.T) {
			s := &ClaudeResetCreditService{accounts: resetAccountStub{&Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"scope": "user:profile"}}}, tokens: resetTokenStub{}, now: time.Now, do: func(*http.Request, string) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			}}
			r, e := s.Query(context.Background(), 1)
			if body == `{}` || strings.Contains(body, `"cedar_ember":null`) {
				require.NoError(t, e)
				require.Empty(t, r.Credits)
			} else {
				require.Error(t, e)
				require.NotContains(t, e.Error(), "private upstream data")
			}
		})
	}
}

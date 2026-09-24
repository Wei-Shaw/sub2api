package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type resetWriterStub struct{ account *Account }

func (s *resetWriterStub) GetByID(context.Context, int64) (*Account, error)         { return s.account, nil }
func (s *resetWriterStub) UpdateExtra(context.Context, int64, map[string]any) error { return nil }

type resetLeaseStub struct {
	keys map[string]bool
	held bool
	fail bool
}

func (s *resetLeaseStub) TryAcquireLeaderLock(_ context.Context, key, owner string, _ time.Duration) (bool, error) {
	if s.fail {
		return false, errors.New("unavailable")
	}
	if s.keys == nil {
		s.keys = map[string]bool{}
	}
	if s.held || s.keys[key] {
		return false, nil
	}
	s.keys[key] = true
	return true, nil
}
func (s *resetLeaseStub) ReleaseLeaderLock(_ context.Context, key, owner string) error {
	delete(s.keys, key)
	return nil
}
func resetTestService(t *testing.T, claim string) (*ClaudeResetCreditService, *int) {
	repo := &resetWriterStub{&Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"scope": "user:profile"}}}
	cfg := DefaultIdempotencyConfig()
	cfg.FailedRetryBackoff = 0
	s := &ClaudeResetCreditService{accounts: repo, writer: repo, tokens: resetTokenStub{}, now: time.Now, idempotency: NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), cfg), locks: &resetLeaseStub{}}
	count := new(int)
	s.do = func(r *http.Request, _ string) (*http.Response, error) {
		body := `{"cedar_ember":{"eligible":true,"next_grant_id":"grant","grants":[{"id":"grant","resets_left":1,"usable_now":true,"use_requires_limit":false,"clears":["five_hour"]}]}}`
		if strings.HasSuffix(r.URL.Path, "/profile") {
			body = `{"organization":{"uuid":"11111111-1111-4111-8111-111111111111"}}`
		}
		if r.Method == "POST" {
			*count++
			require.Equal(t, "https://api.anthropic.com/api/organizations/11111111-1111-4111-8111-111111111111/reset_rate_limits", r.URL.String())
			var payload map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, "cedar_ember", payload["program"])
			require.Equal(t, "grant", payload["grant_id"])
			require.NotEmpty(t, payload["request_id"])
			if claim == "network-error" {
				return nil, errors.New("timeout private upstream details")
			}
			body = claim
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	return s, count
}
func TestClaudeResetRedemptionReplayAndUnknownFence(t *testing.T) {
	for _, wire := range []string{`{"result":"reset"}`, `{"result":"unavailable","reason":"stamp_indeterminate"}`, `{"result":"not_limited"}`, `{"result":"already_used"}`, `{broken`, "network-error"} {
		t.Run(wire, func(t *testing.T) {
			s, count := resetTestService(t, wire)
			status, e := s.Query(context.Background(), 1)
			require.NoError(t, e)
			selection := status.Credits[0].SelectionToken
			out, e := s.Redeem(context.Background(), 1, selection, "operator-one")
			require.NoError(t, e)
			require.Equal(t, 1, *count)
			again, e := s.Redeem(context.Background(), 1, selection, "operator-one")
			require.NoError(t, e)
			require.True(t, again.Replayed)
			require.Equal(t, out.Outcome, again.Outcome)
			require.Equal(t, 1, *count)
			if out.Outcome == "unknown" {
				// Simulated process restart retains database only; no extra marker or memory required.
				s.locks = &resetLeaseStub{}
				other, e := s.Redeem(context.Background(), 1, selection, "operator-two")
				require.NoError(t, e)
				require.Equal(t, "unknown", other.Outcome)
				require.True(t, other.Replayed)
				require.Equal(t, 1, *count)
				_, e = s.Redeem(context.Background(), 1, strings.Repeat("a", 64), "operator-three")
				require.Error(t, e)
				require.Equal(t, 1, *count)
			}
		})
	}
}
func TestClaudeResetRedemptionFailsClosed(t *testing.T) {
	s, count := resetTestService(t, `{"result":"reset"}`)
	status, e := s.Query(context.Background(), 1)
	require.NoError(t, e)
	sel := status.Credits[0].SelectionToken
	_, e = s.Redeem(context.Background(), 1, sel, "")
	require.ErrorIs(t, e, ErrIdempotencyKeyRequired)
	s.locks = &resetLeaseStub{fail: true}
	_, e = s.Redeem(context.Background(), 1, sel, "valid-key")
	require.Error(t, e)
	require.Zero(t, *count)
	s.locks = &resetLeaseStub{}
	_, e = s.Redeem(context.Background(), 1, strings.Repeat("a", 64), "stale-key")
	require.Error(t, e)
	require.Zero(t, *count)
}

func TestClaudeResetNoopNewOperationUsesNewRequestID(t *testing.T) {
	s, count := resetTestService(t, `{"result":"cooldown"}`)
	original := s.do
	var ids []string
	s.do = func(r *http.Request, p string) (*http.Response, error) {
		if r.Method == "POST" {
			raw, e := io.ReadAll(r.Body)
			require.NoError(t, e)
			r.Body = io.NopCloser(strings.NewReader(string(raw)))
			var body map[string]string
			require.NoError(t, json.Unmarshal(raw, &body))
			ids = append(ids, body["request_id"])
		}
		return original(r, p)
	}
	status, e := s.Query(context.Background(), 1)
	require.NoError(t, e)
	sel := status.Credits[0].SelectionToken
	_, e = s.Redeem(context.Background(), 1, sel, "first-op")
	require.NoError(t, e)
	_, e = s.Redeem(context.Background(), 1, sel, "first-op")
	require.NoError(t, e)
	require.Len(t, ids, 1)
	_, e = s.Redeem(context.Background(), 1, sel, "new-op")
	require.NoError(t, e)
	require.Equal(t, 2, *count)
	require.Len(t, ids, 2)
	require.NotEqual(t, ids[0], ids[1])
}

func TestClaudeResetDuplicateLocalAccountsShareOrganizationFence(t *testing.T) {
	s, count := resetTestService(t, "network-error")
	status, e := s.Query(context.Background(), 1)
	require.NoError(t, e)
	sel := status.Credits[0].SelectionToken
	out, e := s.Redeem(context.Background(), 1, sel, "first-account")
	require.NoError(t, e)
	require.Equal(t, "unknown", out.Outcome)
	out, e = s.Redeem(context.Background(), 2, sel, "duplicate-row")
	require.NoError(t, e)
	require.Equal(t, "unknown", out.Outcome)
	require.True(t, out.Replayed)
	require.Equal(t, 1, *count)
}

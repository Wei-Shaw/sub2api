package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

const redeemTestOrg = "11111111-1111-4111-8111-111111111111"

type redeemAccountsStub struct{}

func (redeemAccountsStub) GetByID(_ context.Context, id int64) (*Account, error) {
	return &Account{ID: id, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"scope": "user:profile"}}, nil
}

type redeemLeaseStub struct {
	mu   sync.Mutex
	keys map[string]bool
	fail bool
}

func (s *redeemLeaseStub) TryAcquireLeaderLock(_ context.Context, key, _ string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return false, errors.New("unavailable")
	}
	if s.keys == nil {
		s.keys = map[string]bool{}
	}
	if s.keys[key] {
		return false, nil
	}
	s.keys[key] = true
	return true, nil
}

func (s *redeemLeaseStub) ReleaseLeaderLock(_ context.Context, key, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keys, key)
	return nil
}

type redeemFake struct {
	mu        sync.Mutex
	status    string // usage body
	claim     string // POST body, or "network-error"
	claimHTTP int
	posts     []map[string]string
	gate      chan struct{} // when set, POST blocks until closed
	entered   chan struct{}
}

func (f *redeemFake) postCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.posts)
}

const redeemableStatus = `{"cedar_ember":{"eligible":true,"at_limit":true,"next_grant_id":"grant_next","grants":[` +
	`{"id":"grant_other","resets_left":1,"usable_now":true,"use_requires_limit":false,"clears":["five_hour"]},` +
	`{"id":"grant_next","resets_left":2,"usable_now":true,"use_requires_limit":true,"clears":["five_hour","seven_day"]}]}}`

func newRedeemService(t *testing.T, f *redeemFake) (*ClaudeResetCreditService, *inMemoryIdempotencyRepo, *redeemLeaseStub) {
	t.Helper()
	repo := newInMemoryIdempotencyRepo()
	cfg := DefaultIdempotencyConfig()
	cfg.FailedRetryBackoff = 0
	locks := &redeemLeaseStub{}
	s := &ClaudeResetCreditService{accounts: redeemAccountsStub{}, tokens: resetTokenStub{}, now: time.Now}
	s.ConfigureRedemption(NewIdempotencyCoordinator(repo, cfg), locks)
	if f.status == "" {
		f.status = redeemableStatus
	}
	s.do = func(r *http.Request, _ string) (*http.Response, error) {
		require.Equal(t, "Bearer synthetic-token", r.Header.Get("Authorization"))
		code := http.StatusOK
		var body string
		switch {
		case r.Method == http.MethodGet && r.URL.String() == claudeResetProfileURL:
			body = `{"organization":{"uuid":"` + redeemTestOrg + `"}}`
		case r.Method == http.MethodGet && r.URL.String() == claudeResetUsageURL:
			f.mu.Lock()
			body = f.status
			f.mu.Unlock()
		case r.Method == http.MethodPost:
			require.Equal(t, "https://api.anthropic.com/api/organizations/"+redeemTestOrg+"/reset_rate_limits", r.URL.String())
			var payload map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Regexp(t, `^[A-Za-z0-9_-]{1,64}$`, payload["request_id"])
			f.mu.Lock()
			f.posts = append(f.posts, payload)
			f.mu.Unlock()
			if f.entered != nil {
				f.entered <- struct{}{}
			}
			if f.gate != nil {
				<-f.gate
			}
			if f.claim == "network-error" {
				return nil, errors.New("timeout private upstream details")
			}
			body = f.claim
			if f.claimHTTP != 0 {
				code = f.claimHTTP
			}
		default:
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	return s, repo, locks
}

func TestClaudeResetRedeemHappyPathServerPicksNextGrant(t *testing.T) {
	f := &redeemFake{claim: `{"result":"reset","resets_left":1,"cleared":["five_hour","seven_day"]}`}
	s, _, _ := newRedeemService(t, f)
	out, err := s.Redeem(context.Background(), 1, "op-1")
	require.NoError(t, err)
	require.Equal(t, ClaudeResetOutcomeReset, out.Outcome)
	require.False(t, out.Replayed)
	require.Equal(t, []string{"five_hour", "seven_day"}, out.Cleared)
	require.NotNil(t, out.Credits)
	require.Equal(t, 1, f.postCount())
	require.Equal(t, "grant_next", f.posts[0]["grant_id"])
	require.Equal(t, "cedar_ember", f.posts[0]["program"])

	raw, err := json.Marshal(out)
	require.NoError(t, err)
	for _, secret := range []string{"grant_next", "grant_other", redeemTestOrg, "synthetic-token", f.posts[0]["request_id"], `"id"`} {
		require.NotContains(t, string(raw), secret)
	}
}

func TestClaudeResetRedeemNonRedeemableSendsNoPost(t *testing.T) {
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	cases := map[string]string{
		"not at limit":   strings.Replace(redeemableStatus, `"at_limit":true`, `"at_limit":false`, 1),
		"ineligible":     strings.Replace(redeemableStatus, `"eligible":true`, `"eligible":false`, 1),
		"cooldown":       strings.Replace(redeemableStatus, `"eligible":true,`, `"eligible":true,"cooldown_until":"`+future+`",`, 1),
		"no next grant":  strings.Replace(redeemableStatus, `"next_grant_id":"grant_next"`, `"next_grant_id":"missing"`, 1),
		"blocked":        strings.Replace(redeemableStatus, `"clears":["five_hour","seven_day"]`, `"clears":["five_hour","seven_day"],"blocking":["x"]`, 1),
		"paused":         strings.Replace(redeemableStatus, `"id":"grant_next",`, `"id":"grant_next","paused":true,`, 1),
		"not usable now": strings.Replace(redeemableStatus, `"id":"grant_next","resets_left":2,"usable_now":true`, `"id":"grant_next","resets_left":2,"usable_now":false`, 1),
		"expired":        strings.Replace(redeemableStatus, `"id":"grant_next",`, `"id":"grant_next","ends_at":"2000-01-01T00:00:00Z",`, 1),
		"not started":    strings.Replace(redeemableStatus, `"id":"grant_next",`, `"id":"grant_next","starts_at":"`+future+`",`, 1),
		"no resets left": strings.Replace(redeemableStatus, `"id":"grant_next","resets_left":2`, `"id":"grant_next","resets_left":0`, 1),
		"no program":     `{"cedar_ember":null}`,
	}
	for name, status := range cases {
		t.Run(name, func(t *testing.T) {
			require.NotEqual(t, redeemableStatus, status)
			f := &redeemFake{status: status, claim: `{"result":"reset"}`}
			s, _, _ := newRedeemService(t, f)
			_, err := s.Redeem(context.Background(), 1, "op-1")
			require.Error(t, err)
			require.Equal(t, "CLAUDE_RESET_NOT_AVAILABLE", infraerrors.Reason(err))
			require.Zero(t, f.postCount())
		})
	}
}

func TestClaudeResetRedeemRequiresKeyAndStores(t *testing.T) {
	f := &redeemFake{claim: `{"result":"reset"}`}
	s, _, locks := newRedeemService(t, f)
	_, err := s.Redeem(context.Background(), 1, "  ")
	require.ErrorIs(t, err, ErrIdempotencyKeyRequired)
	locks.fail = true
	_, err = s.Redeem(context.Background(), 1, "op-1")
	require.Error(t, err)
	unconfigured := &ClaudeResetCreditService{accounts: redeemAccountsStub{}, tokens: resetTokenStub{}, now: time.Now}
	_, err = unconfigured.Redeem(context.Background(), 1, "op-2")
	require.ErrorIs(t, err, ErrIdempotencyStoreUnavail)
	require.Zero(t, f.postCount())
}

func TestClaudeResetRedeemSameKeyReplaysWithoutSecondPost(t *testing.T) {
	f := &redeemFake{claim: `{"result":"reset"}`}
	s, _, _ := newRedeemService(t, f)
	first, err := s.Redeem(context.Background(), 1, "op-1")
	require.NoError(t, err)
	again, err := s.Redeem(context.Background(), 1, "op-1")
	require.NoError(t, err)
	require.True(t, again.Replayed)
	require.Equal(t, first.Outcome, again.Outcome)
	require.Equal(t, 1, f.postCount())

	// A new confirmation is a new operation with a new upstream request_id.
	_, err = s.Redeem(context.Background(), 1, "op-2")
	require.NoError(t, err)
	require.Equal(t, 2, f.postCount())
	require.NotEqual(t, f.posts[0]["request_id"], f.posts[1]["request_id"])
}

func TestClaudeResetRedeemDuplicateOrgAccountsConcurrentOnlyOnePost(t *testing.T) {
	f := &redeemFake{claim: `{"result":"reset"}`, gate: make(chan struct{}), entered: make(chan struct{}, 2)}
	s, _, _ := newRedeemService(t, f)
	type res struct {
		out *ClaudeResetOutcome
		err error
	}
	first := make(chan res, 1)
	go func() {
		out, err := s.Redeem(context.Background(), 1, "account-one")
		first <- res{out, err}
	}()
	<-f.entered // account 1 holds the organization lease and is mid-claim
	_, err := s.Redeem(context.Background(), 2, "account-two")
	require.Error(t, err)
	require.Equal(t, "CLAUDE_RESET_BUSY", infraerrors.Reason(err))
	close(f.gate)
	r := <-first
	require.NoError(t, r.err)
	require.Equal(t, ClaudeResetOutcomeReset, r.out.Outcome)
	require.Equal(t, 1, f.postCount())
}

func TestClaudeResetRedeemUnknownOutcomeFencesOrganization(t *testing.T) {
	for _, claim := range []string{"network-error", `{broken`, `{"result":"weird"}`, `{"result":"unavailable","reason":"stamp_indeterminate"}`, `{"result":"reset","reason":"reset_unconfirmed"}`, "http-500"} {
		t.Run(claim, func(t *testing.T) {
			f := &redeemFake{claim: claim}
			if claim == "http-500" {
				f.claim, f.claimHTTP = `{"result":"reset"}`, http.StatusInternalServerError
			}
			s, _, _ := newRedeemService(t, f)
			out, err := s.Redeem(context.Background(), 1, "op-1")
			require.NoError(t, err)
			require.Equal(t, ClaudeResetOutcomeUnknown, out.Outcome)
			require.Nil(t, out.Credits)
			require.NotContains(t, out.Reason, "private")

			// Same confirmation replays, no resend.
			again, err := s.Redeem(context.Background(), 1, "op-1")
			require.NoError(t, err)
			require.True(t, again.Replayed)
			require.Equal(t, ClaudeResetOutcomeUnknown, again.Outcome)

			// Simulated restart (fresh leases); a new confirmation on this or a
			// duplicate account of the same organization is blocked by the fence.
			s.locks = &redeemLeaseStub{}
			for _, id := range []int64{1, 2} {
				_, err = s.Redeem(context.Background(), id, "op-new")
				require.Error(t, err)
				require.Equal(t, "CLAUDE_RESET_UNRESOLVED", infraerrors.Reason(err))
			}
			require.Equal(t, 1, f.postCount())

			// Still blocked past the short unavailable fence.
			s.now = func() time.Time { return time.Now().Add(claudeResetUnavailableFenceTTL + time.Minute) }
			_, err = s.Redeem(context.Background(), 1, "op-new")
			require.Equal(t, "CLAUDE_RESET_UNRESOLVED", infraerrors.Reason(err))

			// Once the fence has settled, a fresh query is authoritative again.
			s.now = func() time.Time { return time.Now().Add(claudeResetUnknownFenceTTL + time.Minute) }
			f.claim, f.claimHTTP = `{"result":"reset"}`, 0
			out, err = s.Redeem(context.Background(), 1, "op-later")
			require.NoError(t, err)
			require.Equal(t, ClaudeResetOutcomeReset, out.Outcome)
			require.Equal(t, 2, f.postCount())
		})
	}
}

func TestClaudeResetRedeemCrashAfterMarkerNeverResends(t *testing.T) {
	f := &redeemFake{claim: `{"result":"reset"}`}
	s, _, _ := newRedeemService(t, f)
	// Simulate a process that persisted the marker for op-1 and died mid-claim.
	orgHash := HashIdempotencyKey("claude-org:" + redeemTestOrg)
	fence, err := s.loadFence(context.Background(), orgHash)
	require.NoError(t, err)
	op := HashIdempotencyKey("claude-reset:1:op-1")
	require.NoError(t, s.persistFence(context.Background(), fence.ID, claudeResetFence{Operation: op, Outcome: ClaudeResetOutcomeUnknown, Reason: "claim_unconfirmed", At: time.Now()}))
	out, err := s.Redeem(context.Background(), 1, "op-1")
	require.NoError(t, err)
	require.Equal(t, ClaudeResetOutcomeUnknown, out.Outcome)
	require.True(t, out.Replayed)
	require.Zero(t, f.postCount())
}

func TestClaudeResetRedeemMapsUpstreamResults(t *testing.T) {
	cases := []struct {
		claim, outcome string
		http           int
	}{
		{`{"result":"reset"}`, ClaudeResetOutcomeReset, 0},
		{`{"result":"already_used"}`, ClaudeResetOutcomeAlreadyUsed, 0},
		{`{"result":"not_limited"}`, ClaudeResetOutcomeNotLimited, 0},
		{`{"result":"cooldown","cooldown_until":"2099-01-01T00:00:00Z"}`, ClaudeResetOutcomeCooldown, 0},
		{`{"result":"ineligible","reason":"tenure"}`, ClaudeResetOutcomeIneligible, 0},
		{`{"result":"unavailable","reason":"stamp_indeterminate"}`, ClaudeResetOutcomeUnknown, 0},
		{`{}`, ClaudeResetOutcomeIneligible, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.claim, func(t *testing.T) {
			f := &redeemFake{claim: tc.claim, claimHTTP: tc.http}
			s, _, _ := newRedeemService(t, f)
			out, err := s.Redeem(context.Background(), 1, "op-1")
			require.NoError(t, err)
			require.Equal(t, tc.outcome, out.Outcome)
			if tc.outcome == ClaudeResetOutcomeCooldown {
				require.NotNil(t, out.CooldownUntil)
			}
			if tc.outcome == ClaudeResetOutcomeIneligible && tc.http == 0 {
				require.Equal(t, "tenure", out.Reason)
			}
			// Definite outcomes never block a later confirmation.
			if tc.outcome != ClaudeResetOutcomeUnknown {
				_, err = s.Redeem(context.Background(), 1, "op-2")
				require.NoError(t, err)
				require.Equal(t, 2, f.postCount())
			}
		})
	}
}

func TestClaudeResetRedeemSanitizesUpstreamReason(t *testing.T) {
	f := &redeemFake{claim: `{"result":"ineligible","reason":"Bearer synthetic-token leaked <script>","cleared":["five_hour","<bad>"]}`}
	s, _, _ := newRedeemService(t, f)
	out, err := s.Redeem(context.Background(), 1, "op-1")
	require.NoError(t, err)
	require.Empty(t, out.Reason)
	require.Equal(t, []string{"five_hour"}, out.Cleared)
}

func TestClaudeResetRedeemExplicitUnavailableFencesBriefly(t *testing.T) {
	f := &redeemFake{claim: `{"result":"unavailable","reason":"grant_next"}`}
	s, _, _ := newRedeemService(t, f)
	out, err := s.Redeem(context.Background(), 1, "op-1")
	require.NoError(t, err)
	require.Equal(t, ClaudeResetOutcomeUnknown, out.Outcome)
	require.Equal(t, claudeResetReasonUnavailable, out.Reason)

	s.locks = &redeemLeaseStub{}
	_, err = s.Redeem(context.Background(), 2, "op-new")
	require.Equal(t, "CLAUDE_RESET_UPSTREAM_UNAVAILABLE", infraerrors.Reason(err))
	require.Equal(t, 1, f.postCount())

	s.now = func() time.Time { return time.Now().Add(claudeResetUnavailableFenceTTL + time.Minute) }
	f.claim = `{"result":"reset"}`
	out, err = s.Redeem(context.Background(), 1, "op-later")
	require.NoError(t, err)
	require.Equal(t, ClaudeResetOutcomeReset, out.Outcome)
	require.Equal(t, 2, f.postCount())
}

func TestClaudeResetRedeemNeverEchoesGrantIDs(t *testing.T) {
	f := &redeemFake{claim: `{"result":"reset","reason":"grant_next","cleared":["five_hour","grant_next","launch","seven_day_overage_included"]}`}
	s, _, _ := newRedeemService(t, f)
	out, err := s.Redeem(context.Background(), 1, "op-1")
	require.NoError(t, err)
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "grant_next")
	require.NotContains(t, string(raw), "launch")
	require.Empty(t, out.Reason)
	require.Equal(t, []string{"five_hour", "seven_day_overage_included"}, out.Cleared)
}

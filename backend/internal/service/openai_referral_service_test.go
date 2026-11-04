package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/imroc/req/v3"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type referralClientStub struct {
	eligibility       *OpenAIReferralEligibility
	queryErr, sendErr error
	calls             []OpenAIReferralCall
	emails            []string
}

func (s *referralClientStub) QueryEligibility(_ context.Context, call OpenAIReferralCall) (*OpenAIReferralEligibility, error) {
	s.calls = append(s.calls, call)
	return s.eligibility, s.queryErr
}
func (s *referralClientStub) SendInvite(_ context.Context, call OpenAIReferralCall, email string) error {
	s.calls = append(s.calls, call)
	s.emails = append(s.emails, email)
	return s.sendErr
}

func referralTestService(t *testing.T, plan string, client OpenAIReferralClient) (*OpenAIQuotaService, *stubQuotaAccountRepo) {
	t.Helper()
	a := &Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"chatgpt_account_id": "workspace-test", "plan_type": plan}}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{100: a}}
	tokens := &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(a): "test-token"}}
	factory := func(string) (*req.Client, error) {
		t.Fatal("referral service must use the business interface, not the HTTP factory")
		return nil, nil
	}
	return NewOpenAIQuotaService(repo, nil, NewOpenAITokenProvider(repo, tokens, nil), factory, client), repo
}

func TestOpenAIReferralSend(t *testing.T) {
	for _, tc := range []struct{ plan, program string }{
		{"plus", openAIReferralConsumer}, {"team", openAIReferralWorkspace},
		{"self_serve_business_usage_based", openAIReferralWorkspace},
	} {
		t.Run(tc.plan, func(t *testing.T) {
			send, reward := 8, 3
			client := &referralClientStub{eligibility: &OpenAIReferralEligibility{
				ShouldShow: true, RemainingSendCapacity: &send, RemainingRewardCapacity: &reward,
				Grants: []OpenAIReferralGrant{{GrantType: "rate_limit_reset_credit", Amount: 1, Recipient: "referrer"}},
				Rules:  []string{"Offer rule"},
			}}
			svc, repo := referralTestService(t, tc.plan, client)
			eligibility, err := svc.QueryReferralEligibility(context.Background(), 100)
			require.NoError(t, err)
			require.Equal(t, 3, *eligibility.AvailableInvites)
			require.Equal(t, []string{"Offer rule"}, eligibility.Rules)
			require.Positive(t, eligibility.FetchedAt)
			require.NoError(t, svc.CacheReferralSnapshot(context.Background(), 100, eligibility))
			require.Equal(t, eligibility, repo.extraUpdates[100][openAIReferralSnapshotKey])
			result, err := svc.SendReferralInvite(context.Background(), 100, OpenAIReferralSendRequest{
				Email: " friend@example.com ", ProgramID: tc.program, Confirmed: true,
			})
			require.NoError(t, err)
			require.True(t, result.Sent)
			require.Equal(t, "friend@example.com", result.Email)
			require.Equal(t, []string{"friend@example.com"}, client.emails)
			require.Len(t, client.calls, 3, "send must recheck eligibility")
			for _, call := range client.calls {
				require.Equal(t, tc.program, call.ProgramID)
				headers := make(http.Header)
				for key, value := range call.Headers {
					headers.Set(key, value)
				}
				require.Equal(t, "Bearer test-token", headers.Get("Authorization"))
				require.Equal(t, "workspace-test", headers.Get("ChatGPT-Account-ID"))
			}
		})
	}
}

func TestOpenAIReferralSendGuards(t *testing.T) {
	for _, tc := range []struct {
		name, email, body, reason string
		confirmed, shadow         bool
	}{
		{"invalid email", "bad-email", `{}`, "OPENAI_REFERRAL_INVALID_EMAIL", true, false},
		{"multiple emails", "a@example.com,b@example.com", `{}`, "OPENAI_REFERRAL_INVALID_EMAIL", true, false},
		{"display name", "User <a@example.com>", `{}`, "OPENAI_REFERRAL_INVALID_EMAIL", true, false},
		{"header injection", "a@example.com\r\nBcc: b@example.com", `{}`, "OPENAI_REFERRAL_INVALID_EMAIL", true, false},
		{"no consent", "a@example.com", `{"should_show":true,"remaining_send_capacity":2}`, "OPENAI_REFERRAL_CONFIRMATION_REQUIRED", false, false},
		{"ineligible", "a@example.com", `{"should_show":false,"remaining_send_capacity":2}`, "OPENAI_REFERRAL_UNAVAILABLE", true, false},
		{"exhausted", "a@example.com", `{"should_show":true,"remaining_send_capacity":0}`, "OPENAI_REFERRAL_UNAVAILABLE", true, false},
		{"unknown capacity", "a@example.com", `{"should_show":true}`, "OPENAI_REFERRAL_UNAVAILABLE", true, false},
		{"reward exhausted", "a@example.com", `{"should_show":true,"remaining_send_capacity":3,"offer_id":"credits_250","remaining_reward_capacity":0}`, "OPENAI_REFERRAL_UNAVAILABLE", true, false},
		{"unknown reward capacity", "a@example.com", `{"should_show":true,"remaining_send_capacity":3,"grants":[{}]}`, "OPENAI_REFERRAL_UNAVAILABLE", true, false},
		{"shadow", "a@example.com", `{}`, "OPENAI_REFERRAL_SHADOW_ACCOUNT", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &referralClientStub{}
			require.NoError(t, json.Unmarshal([]byte(tc.body), &client.eligibility))
			svc, repo := referralTestService(t, "plus", client)
			if tc.shadow {
				parentID := int64(200)
				repo.accounts[100].ParentAccountID = &parentID
			}
			_, err := svc.SendReferralInvite(context.Background(), 100, OpenAIReferralSendRequest{Email: tc.email, ProgramID: openAIReferralConsumer, Confirmed: tc.confirmed})
			require.Equal(t, tc.reason, infraerrors.Reason(err))
			require.Empty(t, client.emails)
		})
	}
}

func TestOpenAIReferralSendStopsOnQueryError(t *testing.T) {
	client := &referralClientStub{queryErr: errors.New("eligibility unavailable")}
	svc, _ := referralTestService(t, "plus", client)
	_, err := svc.SendReferralInvite(context.Background(), 100, OpenAIReferralSendRequest{Email: "friend@example.com", ProgramID: openAIReferralConsumer, Confirmed: true})
	require.ErrorIs(t, err, client.queryErr)
	require.Empty(t, client.emails)
}

func TestOpenAIReferralSendPreservesUnknownOutcomeWithoutRetry(t *testing.T) {
	count := 2
	client := &referralClientStub{
		eligibility: &OpenAIReferralEligibility{ShouldShow: true, RemainingSendCapacity: &count},
		sendErr:     infraerrors.New(502, "OPENAI_REFERRAL_SEND_UNKNOWN", "outcome unknown"),
	}
	svc, _ := referralTestService(t, "plus", client)
	_, err := svc.SendReferralInvite(context.Background(), 100, OpenAIReferralSendRequest{Email: "friend@example.com", ProgramID: openAIReferralConsumer, Confirmed: true})
	require.ErrorIs(t, err, client.sendErr)
	require.Len(t, client.emails, 1)
}

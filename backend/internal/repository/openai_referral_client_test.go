package repository

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func referralTestClient(t *testing.T, handler http.HandlerFunc) *openAIReferralClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	adapter := NewOpenAIReferralClient(func(proxy string) (*req.Client, error) {
		require.Equal(t, "test-proxy", proxy)
		// A shared factory may have retries; the adapter must override them.
		return req.C().SetCommonRetryCount(2).SetCommonRetryCondition(func(_ *req.Response, _ error) bool { return true }), nil
	})
	client, ok := adapter.(*openAIReferralClient)
	require.True(t, ok)
	client.baseURL = srv.URL + "/backend-api/referrals/invite"
	return client
}

func referralTestCall() service.OpenAIReferralCall {
	return service.OpenAIReferralCall{ProxyURL: "test-proxy", ProgramID: "codex_referral_consumer",
		Headers: map[string]string{"Authorization": "Bearer test-token", "ChatGPT-Account-ID": "workspace-test"}}
}

func TestOpenAIReferralClientProtocol(t *testing.T) {
	var gets, posts atomic.Int32
	client := referralTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		require.Equal(t, "workspace-test", r.Header.Get("ChatGPT-Account-ID"))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			gets.Add(1)
			require.Equal(t, "/backend-api/referrals/invite/eligibility", r.URL.Path)
			require.Equal(t, "codex_referral_consumer", r.URL.Query().Get("program_id"))
			require.Equal(t, "persistent", r.URL.Query().Get("entrypoint"))
			_, _ = w.Write([]byte(`{"should_show":true,"remaining_send_capacity":8,"remaining_reward_capacity":3,"grants":[{"grant_type":"rate_limit_reset_credit","amount":1,"recipient":"referrer"}],"rules":["Offer rule"]}`))
			return
		}
		posts.Add(1)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/backend-api/referrals/invite", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, map[string]any{"program_id": "codex_referral_consumer", "entrypoint": "persistent", "emails": []any{"friend@example.com"}}, body)
		_, _ = w.Write([]byte(`{"invites":[{"referral_id":"test-invite"}]}`))
	})
	result, err := client.QueryEligibility(context.Background(), referralTestCall())
	require.NoError(t, err)
	require.True(t, result.ShouldShow)
	require.Equal(t, 8, *result.RemainingSendCapacity)
	require.Equal(t, 3, *result.RemainingRewardCapacity)
	require.Equal(t, "rate_limit_reset_credit", result.Grants[0].GrantType)
	require.Equal(t, []string{"Offer rule"}, result.Rules)
	require.NoError(t, client.SendInvite(context.Background(), referralTestCall(), "friend@example.com"))
	require.EqualValues(t, 1, gets.Load())
	require.EqualValues(t, 1, posts.Load())
}

func TestOpenAIReferralClientSanitizesErrorsWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		status int
		reason string
	}{
		{400, "OPENAI_REFERRAL_REJECTED"}, {422, "OPENAI_REFERRAL_REJECTED"},
		{401, "OPENAI_REFERRAL_AUTH_ERROR"}, {403, "OPENAI_REFERRAL_FORBIDDEN"},
		{409, "OPENAI_REFERRAL_ALREADY_EXISTS"}, {429, "OPENAI_REFERRAL_RATE_LIMITED"},
		{500, "OPENAI_REFERRAL_UPSTREAM_ERROR"},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			var requests atomic.Int32
			client := referralTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"detail":"secret-token someone@example.com"}`))
			})
			_, queryErr := client.QueryEligibility(context.Background(), referralTestCall())
			sendErr := client.SendInvite(context.Background(), referralTestCall(), "friend@example.com")
			for _, err := range []error{queryErr, sendErr} {
				require.Equal(t, tc.reason, infraerrors.Reason(err))
				require.NotContains(t, err.Error(), "secret-token")
				require.NotContains(t, err.Error(), "someone@example.com")
			}
			require.EqualValues(t, 2, requests.Load(), "one query and one send, no retries")
		})
	}
}

func TestOpenAIReferralClientInvalidResponses(t *testing.T) {
	for _, body := range []string{"null", "invalid json", `{"invites":[]}`, `{"invites":[null]}`, `{"invites":[{},{}]}`} {
		t.Run(body, func(t *testing.T) {
			var requests atomic.Int32
			client := referralTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				_, _ = w.Write([]byte(body))
			})
			err := client.SendInvite(context.Background(), referralTestCall(), "friend@example.com")
			require.Equal(t, "OPENAI_REFERRAL_SEND_UNKNOWN", infraerrors.Reason(err))
			require.EqualValues(t, 1, requests.Load())
			if body == "null" || body == "invalid json" {
				_, err := client.QueryEligibility(context.Background(), referralTestCall())
				require.Equal(t, "OPENAI_REFERRAL_INVALID_RESPONSE", infraerrors.Reason(err))
			}
		})
	}
}

func TestOpenAIReferralClientLostResponseIsUnknownWithoutRetry(t *testing.T) {
	var requests atomic.Int32
	client := referralTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		hijacker, ok := w.(http.Hijacker)
		require.True(t, ok)
		conn, _, err := hijacker.Hijack()
		require.NoError(t, err)
		_ = conn.Close() // Upstream received the request but its response was lost.
	})
	err := client.SendInvite(context.Background(), referralTestCall(), "friend@example.com")
	require.Equal(t, "OPENAI_REFERRAL_SEND_UNKNOWN", infraerrors.Reason(err))
	require.EqualValues(t, 1, requests.Load())
}

func TestOpenAIReferralClientFactoryErrorIsSanitized(t *testing.T) {
	client := NewOpenAIReferralClient(func(string) (*req.Client, error) { return nil, errors.New("secret proxy credentials") })
	err := client.SendInvite(context.Background(), referralTestCall(), "friend@example.com")
	require.Equal(t, "OPENAI_REFERRAL_CLIENT_ERROR", infraerrors.Reason(err))
	require.NotContains(t, err.Error(), "secret")
}

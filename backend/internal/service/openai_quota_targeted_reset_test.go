package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func newTargetedResetTestService(t *testing.T, handler http.HandlerFunc) *OpenAIQuotaService {
	t.Helper()
	account := &Account{
		ID:       100,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"chatgpt_account_id": "account-targeted-reset",
		},
	}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(account): "fake-token",
	}}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewOpenAIQuotaService(repo, nil, NewOpenAITokenProvider(repo, tokenCache, nil), newQuotaRedirectingFactory(server))
}

func TestQueryResetCreditsReturnsAdminDetails(t *testing.T) {
	service := newTargetedResetTestService(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/wham/rate-limit-reset-credits", r.URL.Path)
		require.Equal(t, "account-targeted-reset", r.Header.Get("ChatGPT-Account-ID"))
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"available_count":2,
			"credits":[
				{"id":"credit-earliest","reset_type":"codex_rate_limits","status":"available","granted_at":"2026-06-17T00:00:00Z","expires_at":"2026-07-17T00:00:00Z","title":"Full reset","description":"Ready"},
				{"id":"credit-no-expiry","reset_type":"codex_rate_limits","status":"available","granted_at":"2026-06-18T00:00:00Z","expires_at":null}
			]
		}`))
	})

	result, err := service.QueryResetCredits(context.Background(), 100)
	require.NoError(t, err)
	require.Equal(t, 2, result.AvailableCount)
	require.True(t, result.DetailsComplete)
	require.Positive(t, result.FetchedAt)
	require.Len(t, result.Credits, 2)
	require.Equal(t, "credit-earliest", result.Credits[0].ID)
	require.Equal(t, "available", result.Credits[0].Status)
	require.Equal(t, "2026-07-17T00:00:00Z", *result.Credits[0].ExpiresAt)
	require.Equal(t, "Full reset", result.Credits[0].Title)
	require.Nil(t, result.Credits[1].ExpiresAt)
}

func TestQueryResetCreditsMarksIncompleteDetails(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "upstream caps detail rows",
			body: `{"available_count":2,"credits":[{"id":"credit-1","status":"available"}]}`,
		},
		{
			name: "available row lacks id",
			body: `{"available_count":1,"credits":[{"status":"available","expires_at":"2026-07-17T00:00:00Z"}]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := newTargetedResetTestService(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("content-type", "application/json")
				_, _ = w.Write([]byte(test.body))
			})
			result, err := service.QueryResetCredits(context.Background(), 100)
			require.NoError(t, err)
			require.False(t, result.DetailsComplete)
		})
	}
}

func TestQueryResetCreditsFailsClosedWithoutDetailList(t *testing.T) {
	service := newTargetedResetTestService(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"available_count":1}`))
	})

	result, err := service.QueryResetCredits(context.Background(), 100)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, infraerrors.Code(err))
}

func TestResetCreditByIDForwardsStableIdempotencyKey(t *testing.T) {
	var bodies []map[string]string
	service := newTargetedResetTestService(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/wham/rate-limit-reset-credits/consume", r.URL.Path)
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		bodies = append(bodies, body)
		w.Header().Set("content-type", "application/json")
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"temporary"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":"reset","windows_reset":2}`))
	})

	const idempotencyKey = "550e8400-e29b-41d4-a716-446655440000"
	_, err := service.ResetCreditByID(context.Background(), 100, "credit-earliest", idempotencyKey)
	require.Error(t, err)
	result, err := service.ResetCreditByID(context.Background(), 100, "credit-earliest", idempotencyKey)
	require.NoError(t, err)
	require.Equal(t, "reset", result.Code)
	require.Equal(t, 2, result.WindowsReset)
	require.Equal(t, []map[string]string{
		{"redeem_request_id": idempotencyKey, "credit_id": "credit-earliest"},
		{"redeem_request_id": idempotencyKey, "credit_id": "credit-earliest"},
	}, bodies)
}

func TestResetCreditLegacyRequestOmitsCreditID(t *testing.T) {
	var body map[string]string
	service := newTargetedResetTestService(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"code":"reset","windows_reset":1}`))
	})

	_, err := service.ResetCredit(context.Background(), 100)
	require.NoError(t, err)
	require.NotEmpty(t, body["redeem_request_id"])
	require.NotContains(t, body, "credit_id")
}

func TestResetCreditByIDValidatesInputs(t *testing.T) {
	tests := []struct {
		name           string
		creditID       string
		idempotencyKey string
	}{
		{name: "missing credit id", idempotencyKey: "key"},
		{name: "missing idempotency key", creditID: "credit"},
		{name: "idempotency key too long", creditID: "credit", idempotencyKey: strings.Repeat("k", 129)},
		{name: "idempotency key contains whitespace", creditID: "credit", idempotencyKey: "key\nvalue"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &OpenAIQuotaService{}
			_, err := service.ResetCreditByID(context.Background(), 100, test.creditID, test.idempotencyKey)
			require.Error(t, err)
			require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
		})
	}
}

func TestTargetedResetOperationsRejectShadowAccount(t *testing.T) {
	parentID := int64(100)
	shadow := &Account{
		ID:              200,
		ParentAccountID: &parentID,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		QuotaDimension:  QuotaDimensionSpark,
	}
	service := &OpenAIQuotaService{accountRepo: &stubQuotaAccountRepo{accounts: map[int64]*Account{shadow.ID: shadow}}}

	_, err := service.QueryResetCredits(context.Background(), shadow.ID)
	require.ErrorIs(t, err, ErrSparkShadowResetNotSupported)
	_, err = service.ResetCreditByID(context.Background(), shadow.ID, "credit", "key")
	require.ErrorIs(t, err, ErrSparkShadowResetNotSupported)
}

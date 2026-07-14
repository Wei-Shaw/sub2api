package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOpenAIRateLimitResetCreditDetails_PreservesAvailableCreditOrder(t *testing.T) {
	body := []byte(`{
		"availableCount":"2",
		"credits":[
			{"reset_type":"codex_rate_limits","status":"redeemed","expires_at":"2026-07-01T04:05:06Z"REDACTED,
			{"reset_type":"codex_rate_limits","status":"available","expires_at":"2026-07-04T04:05:06Z"REDACTED,
			{"resetType":"codex_rate_limits","status":"available","expiresAt":"2026-07-03T04:05:06Z"REDACTED,
			{"reset_type":"other","status":"available","expires_at":"2026-07-02T04:05:06Z"REDACTED
		]
REDACTED`)

	details, err := parseOpenAIRateLimitResetCreditDetails(body)
REDACTED
	require.NotNil(t, details.AvailableCount)
	require.Equal(t, 2, *details.AvailableCount)
	require.Equal(t, []OpenAIRateLimitResetCreditDetail{
		{ExpiresAt: "2026-07-04T04:05:06Z"REDACTED,
		{ExpiresAt: "2026-07-03T04:05:06Z"REDACTED,
REDACTED, details.Credits)
REDACTED

func TestQueryUsageResetCreditCountPrecedence(t *testing.T) {
	tests := []struct {
		name        string
		usageBody   string
		detailBody  string
		wantCount   int
		wantCredits int
		wantNil     bool
REDACTED{
		{
			name:       "detail count creates missing usage credits",
			usageBody:  `{REDACTED`,
			detailBody: `{"available_count":3,"credits":[{"expires_at":"2026-07-03T04:05:06Z"REDACTED]REDACTED`,
			wantCount:  3, wantCredits: 1,
	REDACTED,
		{
			name:       "explicit detail zero overrides usage and records",
			usageBody:  `{"rate_limit_reset_credits":{"available_count":4REDACTEDREDACTED`,
			detailBody: `{"available_count":0,"credits":[{"expires_at":"2026-07-03T04:05:06Z"REDACTED]REDACTED`,
			wantCount:  0, wantCredits: 1,
	REDACTED,
		{
			name:       "available records override usage when detail count is absent",
			usageBody:  `{"rate_limit_reset_credits":{"available_count":7REDACTEDREDACTED`,
			detailBody: `{"credits":[{"expires_at":"2026-07-03T04:05:06Z"REDACTED,{"expiresAt":"2026-07-04T04:05:06Z"REDACTED]REDACTED`,
			wantCount:  2, wantCredits: 2,
	REDACTED,
		{
			name:       "empty detail list overrides usage with zero",
			usageBody:  `{"rate_limit_reset_credits":{"available_count":7REDACTEDREDACTED`,
			detailBody: `{"credits":[]REDACTED`,
			wantCount:  0,
	REDACTED,
		{
			name:       "fully filtered list overrides usage with zero",
			usageBody:  `{"rate_limit_reset_credits":{"available_count":7REDACTEDREDACTED`,
			detailBody: `{"credits":[{"reset_type":"codex_rate_limits","status":"redeemed","expires_at":"2026-07-03T04:05:06Z"REDACTED,{"reset_type":"other","status":"available","expires_at":"2026-07-04T04:05:06Z"REDACTED]REDACTED`,
			wantCount:  0,
	REDACTED,
		{
			name:       "available records without expiry still count",
			usageBody:  `{"rate_limit_reset_credits":{"available_count":7REDACTEDREDACTED`,
			detailBody: `{"credits":[{"status":"available"REDACTED,{"status":"available","expires_at":"2026-07-04T04:05:06Z"REDACTED]REDACTED`,
			wantCount:  2, wantCredits: 1,
	REDACTED,
		{
			name:        "shape without count or list preserves usage details",
			usageBody:   `{"rate_limit_reset_credits":{"available_count":5,"credits":[{"expires_at":"usage-expiry"REDACTED]REDACTEDREDACTED`,
			detailBody:  `{REDACTED`,
			wantCount:   5,
			wantCredits: 1,
	REDACTED,
		{
			name:       "negative detail count without list preserves usage",
			usageBody:  `{"rate_limit_reset_credits":{"available_count":4REDACTEDREDACTED`,
			detailBody: `{"available_count":-1REDACTED`,
			wantCount:  4,
	REDACTED,
		{
			name:       "negative detail count falls back to available records",
			usageBody:  `{"rate_limit_reset_credits":{"available_count":4REDACTEDREDACTED`,
			detailBody: `{"available_count":-1,"credits":[{"status":"available","expires_at":"2026-07-04T04:05:06Z"REDACTED]REDACTED`,
			wantCount:  1, wantCredits: 1,
	REDACTED,
		{
			name:       "empty object preserves missing usage credits",
			usageBody:  `{REDACTED`,
			detailBody: `{REDACTED`,
			wantNil:    true,
	REDACTED,
		{
			name:       "null body preserves missing usage credits",
			usageBody:  `{REDACTED`,
			detailBody: `null`,
			wantNil:    true,
	REDACTED,
		{
			name:       "empty body preserves missing usage credits",
			usageBody:  `{REDACTED`,
			detailBody: ``,
			wantNil:    true,
	REDACTED,
		{
			name:       "null object record is not counted",
			usageBody:  `{"rate_limit_reset_credits":{"available_count":7REDACTEDREDACTED`,
			detailBody: `{"credits":[null]REDACTED`,
			wantCount:  0,
	REDACTED,
		{
			name:       "null top level record is not counted",
			usageBody:  `{"rate_limit_reset_credits":{"available_count":7REDACTEDREDACTED`,
			detailBody: `[null]`,
			wantCount:  0,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				ID:       100,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Status:   StatusActive,
		REDACTED
					"chatgpt_account_id": "org-parent123",
			REDACTED,
		REDACTED
			repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{100: accountREDACTEDREDACTED
			tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
				OpenAITokenCacheKey(account): "fake-token",
		REDACTEDREDACTED
			tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)

			var detailCalls int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("content-type", "application/json")
				switch r.URL.Path {
				case "/backend-api/wham/usage":
					_, _ = w.Write([]byte(tt.usageBody))
				case "/backend-api/wham/rate-limit-reset-credits":
					detailCalls++
					_, _ = w.Write([]byte(tt.detailBody))
				default:
					http.NotFound(w, r)
			REDACTED
		REDACTED))
			defer srv.Close()

			svc := NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv))
			usage, err := svc.QueryUsage(context.Background(), 100)
		REDACTED
			require.NotNil(t, usage)
			require.Equal(t, 1, detailCalls)
			if tt.wantNil {
				require.Nil(t, usage.RateLimitResetCredits)
				return
		REDACTED
			require.NotNil(t, usage.RateLimitResetCredits)
			require.Equal(t, tt.wantCount, usage.RateLimitResetCredits.AvailableCount)
			require.Len(t, usage.RateLimitResetCredits.Credits, tt.wantCredits)
	REDACTED)
REDACTED
REDACTED

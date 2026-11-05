//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// A model refusal (the account's plan or key cannot serve the requested model)
// fails over and cools only that (account, model) pair down. It says nothing
// about how the account serves other models, so it must not count against the
// account's scheduler error rate. The error rate is an EWMA that only moves
// when a result is reported: an idle account that only ever sees other
// callers' refused models would otherwise climb towards 1 and lose every
// selection, for every model.

type openAIModelRefusalScheduleCase struct {
	name    string
	account *Account
	err     error
}

func newOpenAIModelRefusalSchedulerService(t *testing.T) *OpenAIGatewayService {
	t.Helper()
	svc := &OpenAIGatewayService{
		cfg:              &config.Config{},
		rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"),
	}
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	require.NotNil(t, svc.getOpenAIAccountScheduler(context.Background()), "the error rate only exists while the advanced scheduler is on")
	return svc
}

func openAIModelRefusalOAuthAccount() *Account {
	return &Account{
		ID:          7101,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"access_token": "token"},
	}
}

func openAIModelRefusalAPIKeyAccount() *Account {
	return &Account{
		ID:          7102,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"api_key": "test-key"},
	}
}

func openAIModelRefusalFailover(statusCode int, body string) *UpstreamFailoverError {
	return &UpstreamFailoverError{StatusCode: statusCode, ResponseBody: []byte(body)}
}

func openAIModelRefusalCases() []openAIModelRefusalScheduleCase {
	return []openAIModelRefusalScheduleCase{
		{
			name:    "codex plan-gated 400",
			account: openAIModelRefusalOAuthAccount(),
			err:     openAIModelRefusalFailover(http.StatusBadRequest, `{"detail":"The 'gpt-5.1' model is not supported when using Codex with a ChatGPT account."}`),
		},
		{
			name:    "404 model not found",
			account: openAIModelRefusalOAuthAccount(),
			err:     openAIModelRefusalFailover(http.StatusNotFound, `{"error":{"code":"model_not_found","message":"The model gpt-5.1 does not exist or you do not have access to it."}}`),
		},
		{
			name:    "api key 401 model not found",
			account: openAIModelRefusalAPIKeyAccount(),
			err:     openAIModelRefusalFailover(http.StatusUnauthorized, `{"error":{"code":"model_not_found","message":"model not found"}}`),
		},
		{
			name:    "wrapped 404 model not found",
			account: openAIModelRefusalOAuthAccount(),
			err:     fmt.Errorf("forward: %w", openAIModelRefusalFailover(http.StatusNotFound, `{"error":{"code":"model_not_found","message":"model not found"}}`)),
		},
	}
}

// Failures that fail over the same way but get no (account, model) cooldown
// keep counting: the error rate is then the only signal that steers the
// scheduler away from the account.
func openAIAccountFailureCases() []openAIModelRefusalScheduleCase {
	apiKeyWithout404 := openAIModelRefusalAPIKeyAccount()
	apiKeyWithout404.Credentials["custom_error_codes_enabled"] = true
	apiKeyWithout404.Credentials["custom_error_codes"] = []any{float64(http.StatusTooManyRequests)}
	return []openAIModelRefusalScheduleCase{
		{name: "502", account: openAIModelRefusalOAuthAccount(), err: openAIModelRefusalFailover(http.StatusBadGateway, `{"error":{"message":"bad gateway"}}`)},
		{name: "500", account: openAIModelRefusalOAuthAccount(), err: openAIModelRefusalFailover(http.StatusInternalServerError, `{"error":{"message":"internal error"}}`)},
		{name: "404 without a model", account: openAIModelRefusalOAuthAccount(), err: openAIModelRefusalFailover(http.StatusNotFound, `{"error":{"message":"route not found"}}`)},
		{name: "400 model refusal outside codex", account: openAIModelRefusalOAuthAccount(), err: openAIModelRefusalFailover(http.StatusBadRequest, `{"error":{"code":"model_not_found","message":"The model is not supported."}}`)},
		{name: "oauth 401 model not found", account: openAIModelRefusalOAuthAccount(), err: openAIModelRefusalFailover(http.StatusUnauthorized, `{"error":{"code":"model_not_found","message":"model not found"}}`)},
		{name: "404 model not found outside custom error codes", account: apiKeyWithout404, err: openAIModelRefusalFailover(http.StatusNotFound, `{"error":{"code":"model_not_found","message":"model not found"}}`)},
		{name: "transport timeout", account: openAIModelRefusalOAuthAccount(), err: &url.Error{Op: "Post", URL: "https://example.invalid", Err: context.DeadlineExceeded}},
		{name: "no observed error", account: openAIModelRefusalOAuthAccount()},
	}
}

func TestReportOpenAIAccountScheduleResult_ModelRefusalKeepsErrorRate(t *testing.T) {
	for _, tc := range openAIModelRefusalCases() {
		t.Run(tc.name, func(t *testing.T) {
			svc := newOpenAIModelRefusalSchedulerService(t)

			for range 5 {
				svc.ReportOpenAIAccountScheduleResult(tc.account, "gpt-5.1", false, nil, tc.err)
			}

			errorRate, _, _ := svc.openaiAccountStats.snapshot(tc.account.ID)
			require.Zero(t, errorRate, "a model refusal must not raise the account's error rate")
			require.Zero(t, svc.openaiAccountStats.size(), "a model refusal must not record a scheduler sample")
		})
	}
}

func TestReportOpenAIAccountScheduleResult_AccountFailureStillRaisesErrorRate(t *testing.T) {
	for _, tc := range openAIAccountFailureCases() {
		t.Run(tc.name, func(t *testing.T) {
			svc := newOpenAIModelRefusalSchedulerService(t)

			if tc.err == nil {
				svc.ReportOpenAIAccountScheduleResult(tc.account, "gpt-5.1", false, nil)
			} else {
				svc.ReportOpenAIAccountScheduleResult(tc.account, "gpt-5.1", false, nil, tc.err)
			}

			errorRate, _, _ := svc.openaiAccountStats.snapshot(tc.account.ID)
			require.InDelta(t, 0.2, errorRate, 1e-9)
		})
	}
}

// The scheduler exemption and the cooldown share one classifier, so an
// attempt is exempted exactly when HandleUpstreamModelNotFound takes it.
func TestUpstreamModelUnavailableCooldown_MatchesHandleUpstreamModelNotFound(t *testing.T) {
	cases := append(openAIModelRefusalCases(), openAIAccountFailureCases()...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var failoverErr *UpstreamFailoverError
			if tc.err == nil || !errors.As(tc.err, &failoverErr) {
				require.False(t, isUpstreamModelUnavailableFailure(tc.account, tc.err))
				return
			}
			repo := &modelNotFoundAccountRepoStub{}
			rates := &RateLimitService{accountRepo: repo}

			handled := rates.HandleUpstreamModelNotFound(context.Background(), tc.account, "gpt-5.1", failoverErr.StatusCode, failoverErr.ResponseBody)

			require.Equal(t, handled, isUpstreamModelUnavailableFailure(tc.account, tc.err))
			require.Equal(t, handled, len(repo.modelRateLimitCalls) == 1)
		})
	}
}

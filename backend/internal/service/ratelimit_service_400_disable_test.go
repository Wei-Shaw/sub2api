//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// A 400 whose message only echoes client-supplied text (unknown field names,
// header values, enum tags) must not permanently disable the account; the
// request would otherwise fail over and disable every account it reaches.
func TestRateLimitService_HandleUpstreamError_400ClientEchoDoesNotDisableAccount(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		body     string
	}{
		{
			name:     "anthropic extra top-level field named after the disabled-org message",
			platform: PlatformAnthropic,
			body:     `{"type":"error","error":{"type":"invalid_request_error","message":"organization has been disabled: Extra inputs are not permitted"}}`,
		},
		{
			name:     "anthropic extra nested field named after the credit message",
			platform: PlatformAnthropic,
			body:     `{"type":"error","error":{"type":"invalid_request_error","message":"metadata.your credit balance is too low: Extra inputs are not permitted"}}`,
		},
		{
			name:     "anthropic beta header value echo",
			platform: PlatformAnthropic,
			body:     "{\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Unexpected value(s) `identity verification is required` for the `anthropic-beta` header. Please consult our documentation at docs.anthropic.com or try again without the header.\"}}",
		},
		{
			name:     "anthropic enum tag echo",
			platform: PlatformAnthropic,
			body:     `{"type":"error","error":{"type":"invalid_request_error","message":"messages.0.content.0: Input tag 'organization has been disabled' found using 'type' does not match any of the expected tags"}}`,
		},
		{
			name:     "openai unknown parameter echo",
			platform: PlatformOpenAI,
			body:     `{"error":{"message":"Unknown parameter: 'identity verification is required'.","type":"invalid_request_error","param":"identity verification is required","code":"unknown_parameter"}}`,
		},
		{
			name:     "openai unsupported parameter echo",
			platform: PlatformOpenAI,
			body:     `{"error":{"message":"Unsupported parameter: 'organization has been disabled' is not supported with this model.","type":"invalid_request_error","code":"unsupported_parameter"}}`,
		},
		{
			name:     "openai unrecognized argument echo",
			platform: PlatformOpenAI,
			body:     `{"error":{"message":"Unrecognized request argument supplied: organization has been disabled","type":"invalid_request_error"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{}
			svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			account := &Account{ID: 42, Platform: tc.platform, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}

			shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusBadRequest, http.Header{}, []byte(tc.body))

			require.False(t, shouldDisable)
			require.Equal(t, 0, repo.setErrorCalls)
		})
	}
}

// Genuine account-state 400s keep disabling the account.
func TestRateLimitService_HandleUpstreamError_400GenuineAccountStateStillDisables(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		body     string
	}{
		{
			name:     "anthropic organization disabled",
			platform: PlatformAnthropic,
			body:     `{"type":"error","error":{"type":"invalid_request_error","message":"This organization has been disabled."}}`,
		},
		{
			name:     "anthropic credit balance too low",
			platform: PlatformAnthropic,
			body:     `{"type":"error","error":{"type":"invalid_request_error","message":"Your credit balance is too low to access the Anthropic API. Please go to Plans & Billing to upgrade or purchase credits."}}`,
		},
		{
			name:     "identity verification required",
			platform: PlatformOpenAI,
			body:     `{"error":{"message":"Identity verification is required to continue using this organization.","type":"invalid_request_error"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{}
			svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			account := &Account{ID: 43, Platform: tc.platform, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}

			shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusBadRequest, http.Header{}, []byte(tc.body))

			require.True(t, shouldDisable)
			require.Equal(t, 1, repo.setErrorCalls)
			require.Equal(t, int64(43), repo.lastErrorID)
		})
	}
}

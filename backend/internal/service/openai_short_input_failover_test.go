package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIShortInputPolicyClassification(t *testing.T) {
	policyBody := []byte(`{"error":{"code":"short_input_rejected","message":"Upstream rejected illegal short-input distillation or heartbeat probing.","type":"invalid_request_error"}}`)
	streamBody := []byte(`{"type":"response.failed","response":{"error":{"code":"short_input_rejected","message":"request rejected","type":"invalid_request_error"}}}`)
	svc := &OpenAIGatewayService{}

	require.True(t, isOpenAIShortInputPolicyError(http.StatusBadRequest, policyBody))
	require.True(t, svc.shouldFailoverOpenAIUpstreamResponse(http.StatusBadRequest, "request rejected", policyBody))
	require.True(t, shouldFailoverOpenAIPassthroughResponse(&Account{Type: AccountTypeAPIKey}, http.StatusBadRequest, policyBody))
	require.True(t, openAIStreamFailedEventShouldFailover(streamBody, "request rejected"))
	require.True(t, openAIStreamErrorEventShouldFailover(streamBody, "request rejected"))

	failoverErr := newOpenAIUpstreamFailoverError(http.StatusBadRequest, nil, policyBody, "request rejected", true)
	require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
	require.Equal(t, openAIShortInputPolicyReason, failoverErr.Reason)
	require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
	require.False(t, failoverErr.RetryableOnSameAccount)
	require.False(t, failoverErr.RequestScopedTransient)
	require.Equal(t, http.StatusBadGateway, failoverErr.ClientStatusCode)
	require.Equal(t, openAIShortInputPolicyClientMessage, failoverErr.ClientMessage)
}

func TestOpenAIShortInputPolicyClassificationRequiresStructuredCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "message only",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"code":"invalid_request_error","message":"short_input_rejected"}}`,
		},
		{
			name:       "echoed user input",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"code":"unknown_parameter","message":"invalid input"},"echo":{"error":{"code":"short_input_rejected"}}}`,
		},
		{
			name:       "wrong status",
			statusCode: http.StatusServiceUnavailable,
			body:       `{"error":{"code":"short_input_rejected"}}`,
		},
		{
			name:       "plain text",
			statusCode: http.StatusBadRequest,
			body:       `short_input_rejected`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			require.False(t, isOpenAIShortInputPolicyError(tt.statusCode, body))
			if tt.statusCode == http.StatusBadRequest {
				require.False(t, (&OpenAIGatewayService{}).shouldFailoverOpenAIUpstreamResponse(tt.statusCode, "", body))
				require.False(t, shouldFailoverOpenAIPassthroughResponse(&Account{Type: AccountTypeAPIKey}, tt.statusCode, body))
			}
		})
	}
}

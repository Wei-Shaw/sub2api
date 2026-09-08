package service

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestClassifyOpenAIRateLimitReason(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       OpenAIRateLimitReason
	}{
		{"quota without reset", `{"error":{"code":"insufficient_quota"}}`, OpenAIRateLimitQuota},
		{"usage window", `{"error":{"type":"usage_limit_reached"}}`, OpenAIRateLimitQuota},
		{"concurrency", `{"error":{"code":"too_many_concurrent_requests","type":"rate_limit_error"}}`, OpenAIRateLimitConcurrency},
		{"rate", `{"error":{"code":"rate_limit_exceeded"}}`, OpenAIRateLimitRate},
		{"message is not evidence", `{"error":{"message":"insufficient_quota too_many_concurrent_requests"}}`, OpenAIRateLimitUnknown},
		{"empty", "", OpenAIRateLimitUnknown},
		{"malformed", "{", OpenAIRateLimitUnknown},
		{"quota wins", `{"error":{"code":"too_many_concurrent_requests","type":"insufficient_quota"}}`, OpenAIRateLimitQuota},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.want, ClassifyOpenAIRateLimitReason(nil, []byte(tc.body))) })
	}
}

func TestCodexRateLimitClassificationUsesFailureEvidence(t *testing.T) {
	headers := http.Header{"X-Codex-Primary-Used-Percent": {"100"}, "X-Codex-Primary-Window-Minutes": {"300"}}
	require.Equal(t, OpenAIRateLimitQuota, ClassifyOpenAIRateLimitReason(headers, []byte(`{"error":{"code":"too_many_concurrent_requests"}}`)))
	failure := &UpstreamFailoverError{StatusCode: 429, Reason: "openai_429_unknown", ResponseHeaders: headers, ResponseBody: []byte(`{"error":{"message":"busy"}}`)}
	require.Equal(t, OpenAIRateLimitUnknown, OpenAIFailoverRateLimitReason(failure), "successful handshake quota data must not override the failure classification")
	failure.Reason = ""
	require.Equal(t, OpenAIRateLimitUnknown, OpenAIFailoverRateLimitReason(failure))
}

func TestExplicitQuotaDoesNotEnterSameAccountRetryWindow(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"error":{"code":"insufficient_quota"}}`)
	require.False(t, svc.shouldRetryOpenAIOAuth429OnSameAccountWithResponse(account, http.StatusTooManyRequests, false, nil, body))
	disposition, reset := classifyOpenAIOAuth429(nil, body)
	require.NotEqual(t, openAIOAuth429Transient, disposition)
	require.Nil(t, reset)
	failure := svc.newOpenAIAccountFailoverError(account, http.StatusTooManyRequests, nil, body, "", false, false)
	require.Equal(t, GatewayFailureReason("openai_429_quota_exhausted"), failure.Reason)
	require.False(t, failure.RetryableOnSameAccount)
	failure = svc.newOpenAIAccountFailoverError(account, http.StatusTooManyRequests, nil, body, "", true, false)
	require.False(t, failure.RetryableOnSameAccount, "an explicit quota failure overrides generic retry hints")
}

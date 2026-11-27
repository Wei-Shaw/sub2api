//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHandleSmartRetry_LastRetryInfoControlsCooldown(t *testing.T) {
	for _, delay := range []time.Duration{5 * time.Second, 60 * time.Second} {
		t.Run(delay.String(), func(t *testing.T) {
			makeBody := func(d string) string {
				return fmt.Sprintf(`{"error":{"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"RATE_LIMIT_EXCEEDED","metadata":{"model":"gemini-3-flash"}},{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":%q}]}}`, d)
			}
			first := []byte(makeBody("1s"))
			upstream := &mockSmartRetryUpstream{
				responses: []*http.Response{{StatusCode: http.StatusTooManyRequests, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(makeBody(delay.String())))}},
				errors:    []error{nil},
			}
			repo := &stubAntigravityAccountRepo{}
			account := &Account{ID: 2, Type: AccountTypeOAuth, Platform: PlatformAntigravity}
			params := antigravityRetryLoopParams{
				ctx: context.Background(), prefix: "[test]", account: account,
				accessToken: "synthetic-test-token", action: "generateContent", body: []byte(`{"input":"test"}`),
				httpUpstream: upstream, accountRepo: repo,
			}
			resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(first))}
			start := time.Now()
			result := (&AntigravityGatewayService{}).handleSmartRetry(params, resp, first, "https://ag-1.test", 0, []string{"https://ag-1.test"})
			end := time.Now()
			require.NotNil(t, result)
			require.NotNil(t, result.switchError)
			require.Len(t, upstream.calls, 1, "the final reset must not cause another retry")
			require.Len(t, repo.modelRateLimitCalls, 2)
			for _, call := range repo.modelRateLimitCalls {
				require.False(t, call.resetAt.Before(start.Add(delay)), "cooldown must honor the final response")
				require.False(t, call.resetAt.After(end.Add(delay)), "cooldown must not exceed the final delay")
			}
		})
	}
}

//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A capacity 503 on one account must not hold the request for ~60 s when other accounts can serve
// the model (2026-11-13: 80 s of retries on one account, then 1.9 s on another).

func capacityBody(model string) []byte {
	return []byte(`{
		"error": {
			"code": 503,
			"status": "UNAVAILABLE",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "` + model + `"}, "reason": "MODEL_CAPACITY_EXHAUSTED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "39s"}
			],
			"message": "No capacity available for model ` + model + ` on the server"
		}
	}`)
}

func capacityParams(ctx context.Context, account *Account, repo *stubAntigravityAccountRepo, upstream *mockSmartRetryUpstream) antigravityRetryLoopParams {
	return antigravityRetryLoopParams{
		ctx:          ctx,
		prefix:       "[test]",
		account:      account,
		accessToken:  "token",
		action:       "generateContent",
		body:         []byte(`{"input":"test"}`),
		accountRepo:  repo,
		httpUpstream: upstream,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}
}

func TestHandleSmartRetry_503_ModelCapacityExhausted_SwitchesAccountAfterFewRetries(t *testing.T) {
	const model = "gemini-3.8-flash-high"
	body := capacityBody(model)
	repo := &stubAntigravityAccountRepo{}
	account := &Account{ID: 13, Name: "AG Lin 20x", Type: AccountTypeOAuth, Platform: PlatformAntigravity}
	upstream := &mockSmartRetryUpstream{
		responses:  []*http.Response{{StatusCode: http.StatusServiceUnavailable, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body))}},
		errors:     []error{nil},
		repeatLast: true,
	}
	resp := &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body))}

	started := time.Now()
	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(capacityParams(context.Background(), account, repo, upstream), resp, body, "https://ag-1.test", 0, []string{"https://ag-1.test"})

	require.NotNil(t, result)
	require.NotNil(t, result.switchError, "capacity exhausted on one account should switch to another account")
	require.Equal(t, int64(13), result.switchError.OriginalAccountID)
	require.Len(t, upstream.calls, antigravityModelCapacitySwitchAttempts, "only a few retries on the same account")
	require.Less(t, time.Since(started), 10*time.Second)

	// Only this account and this exact model cool down, briefly; not the Gemini family scope.
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, int64(13), repo.modelRateLimitCalls[0].accountID)
	require.Equal(t, model, repo.modelRateLimitCalls[0].modelKey)
	require.WithinDuration(t, time.Now().Add(antigravityModelCapacityAccountCooldown), repo.modelRateLimitCalls[0].resetAt, 5*time.Second)
}

func TestHandleSmartRetry_503_ModelCapacityExhausted_OtherAccountNotBlockedByGlobalDedup(t *testing.T) {
	const model = "gemini-3.8-flash-medium"
	modelCapacityExhaustedMu.Lock()
	modelCapacityExhaustedUntil[model] = time.Now().Add(time.Minute) // set by another account's failure
	modelCapacityExhaustedMu.Unlock()
	t.Cleanup(func() {
		modelCapacityExhaustedMu.Lock()
		delete(modelCapacityExhaustedUntil, model)
		modelCapacityExhaustedMu.Unlock()
	})

	body := capacityBody(model)
	repo := &stubAntigravityAccountRepo{}
	account := &Account{ID: 2, Name: "AG William 20x", Type: AccountTypeOAuth, Platform: PlatformAntigravity}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader([]byte(`{"ok":true}`)))}},
		errors:    []error{nil},
	}
	resp := &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body))}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(capacityParams(context.Background(), account, repo, upstream), resp, body, "https://ag-1.test", 0, []string{"https://ag-1.test"})

	require.NotNil(t, result)
	require.NotNil(t, result.resp)
	require.Equal(t, http.StatusOK, result.resp.StatusCode, "another account's capacity failure must not fail this account's request")
	require.Len(t, upstream.calls, 1)
}

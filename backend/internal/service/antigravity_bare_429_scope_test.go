//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The upstream's bare 429 has no quotaResetDelay / RetryInfo (#8824).
const antigravityBare429Body = `{"error":{"code":429,"message":"Resource has been exhausted (e.g. check quota).","status":"RESOURCE_EXHAUSTED"}}`

func newAntigravityScopeTestAccount(planType string) *Account {
	return &Account{
		ID: 901, Name: "acc-bare-429", Platform: PlatformAntigravity, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{
			"plan_type": planType,
			"model_mapping": map[string]any{
				"gemini-3.8-flash":    "gemini-3.8-flash",
				"gemini-2.5-pro":      "gemini-2.5-pro",
				"gemini-4-argon-high": "gemini-4-argon-high",
				"gemini-4-argon-low":  "gemini-4-argon-low",
				"claude-sonnet-4-6":   "claude-sonnet-4-6",
			},
		},
	}
}

// schedulerSnapshotAfter rebuilds the persisted limits and reads the account back
// through JSON, as the scheduler does.
func schedulerSnapshotAfter(t *testing.T, account *Account, calls []modelRateLimitCall) Account {
	t.Helper()
	limits := make(map[string]any)
	for _, call := range calls {
		require.Equal(t, account.ID, call.accountID)
		limits[call.modelKey] = map[string]any{"rate_limit_reset_at": call.resetAt.Format(time.RFC3339)}
	}
	account.Extra = map[string]any{modelRateLimitsKey: limits}
	data, err := json.Marshal(account)
	require.NoError(t, err)
	var snapshot Account
	require.NoError(t, json.Unmarshal(data, &snapshot))
	return snapshot
}

func modelRateLimitKeysOf(calls []modelRateLimitCall) []string {
	keys := make([]string, 0, len(calls))
	for _, call := range calls {
		keys = append(keys, call.modelKey)
	}
	return keys
}

func TestHandleUpstreamError_Bare429CoolsOnlyFailedGeminiModel(t *testing.T) {
	t.Setenv(antigravityFallbackSecondsEnv, "")
	cases := []struct {
		name   string
		plan   string
		failed string
		other  []string
	}{
		{name: "shared Gemini scope", plan: "Ultra", failed: "gemini-3.8-flash", other: []string{"gemini-2.5-pro", "gemini-4-argon-high"}},
		{name: "Pro Argon scope", plan: "Pro", failed: "gemini-4-argon-high", other: []string{"gemini-4-argon-low", "gemini-3.8-flash"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubAntigravityAccountRepo{}
			svc := &AntigravityGatewayService{accountRepo: repo}
			account := newAntigravityScopeTestAccount(tc.plan)

			before := time.Now()
			result := svc.handleUpstreamError(context.Background(), "[test]", account, http.StatusTooManyRequests, http.Header{}, []byte(antigravityBare429Body), tc.failed, 0, "", false)

			require.Nil(t, result)
			require.Empty(t, repo.rateCalls, "a bare 429 must not rate limit the whole account")
			require.Equal(t, []string{tc.failed}, modelRateLimitKeysOf(repo.modelRateLimitCalls))
			require.WithinDuration(t, before.Add(antigravityDefaultRateLimitDuration), repo.modelRateLimitCalls[0].resetAt, 2*time.Second)

			snapshot := schedulerSnapshotAfter(t, account, repo.modelRateLimitCalls)
			require.False(t, snapshot.IsSchedulableForModel(tc.failed))
			for _, model := range tc.other {
				require.True(t, snapshot.IsSchedulableForModel(model), "other model: %s", model)
			}
		})
	}
}

func TestHandleUpstreamError_429WithResetDelayKeepsGeminiFamilyScope(t *testing.T) {
	bodies := map[string]string{
		"quotaResetDelay": string(buildGeminiRateLimitBody("30s")),
		"RetryInfo":       `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"30s"}]}}`,
		"retry in text":   `{"error":{"code":429,"message":"Too many requests. Please retry in 30s.","status":"RESOURCE_EXHAUSTED"}}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			repo := &stubAntigravityAccountRepo{}
			svc := &AntigravityGatewayService{accountRepo: repo}
			account := newAntigravityScopeTestAccount("Ultra")

			result := svc.handleUpstreamError(context.Background(), "[test]", account, http.StatusTooManyRequests, http.Header{}, []byte(body), "gemini-3.8-flash", 0, "", false)

			require.Nil(t, result)
			require.Equal(t, []string{"gemini-3.8-flash", antigravityGeminiModelRateLimitKey}, modelRateLimitKeysOf(repo.modelRateLimitCalls))
			snapshot := schedulerSnapshotAfter(t, account, repo.modelRateLimitCalls)
			require.False(t, snapshot.IsSchedulableForModel("gemini-3.8-flash"))
			require.False(t, snapshot.IsSchedulableForModel("gemini-2.5-pro"), "an explicit reset keeps the shared Gemini cooldown")
			require.True(t, snapshot.IsSchedulableForModel("claude-sonnet-4-6"))
		})
	}
}

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

func TestAntigravityModelRateLimits_ProArgonQuotaIsolation(t *testing.T) {
	plans := []struct {
		name     string
		value    any
		separate bool
	}{
		{name: "Pro", value: "Pro", separate: true},
		{name: "normalized Pro", value: " pro ", separate: true},
		{name: "Pro tier ID", value: "g1-pro-tier", separate: true},
		{name: "Ultra", value: "Ultra"},
		{name: "Ultra tier ID", value: "g1-ultra-tier"},
		{name: "Ultra lite tier ID", value: "g1-ultra-lite-tier"},
		{name: "free", value: "Free"},
		{name: "unknown", value: "enterprise"},
		{name: "missing"},
		{name: "malformed", value: 42},
	}
	pools := []struct {
		name      string
		exhausted string
		same      []string
		other     []string
	}{
		{
			name:      "Argon exhausted",
			exhausted: "gemini-4-argon-high",
			same:      []string{"gemini-4-argon-high", "gemini-4-argon-low", "gemini-4-argon-medium", "client-argon"},
			other:     []string{"gemini-3.8-flash", "gemini-2.5-pro"},
		},
		{
			name:      "other Gemini exhausted",
			exhausted: "gemini-3.8-flash",
			same:      []string{"gemini-3.8-flash", "gemini-2.5-pro"},
			other:     []string{"gemini-4-argon-high", "gemini-4-argon-low", "gemini-4-argon-medium", "client-argon"},
		},
	}
	for _, plan := range plans {
		for _, pool := range pools {
			t.Run(plan.name+"/"+pool.name, func(t *testing.T) {
				account := &Account{
					ID: 789, Platform: PlatformAntigravity, Status: StatusActive, Schedulable: true,
					Credentials: map[string]any{
						"plan_type": plan.value,
						"model_mapping": map[string]any{
							"gemini-4-argon-high":   "gemini-4-argon-high",
							"gemini-4-argon-low":    "gemini-4-argon-low",
							"gemini-4-argon-medium": "gemini-4-argon-medium",
							"client-argon":          "gemini-4-argon-high",
							"gemini-3.8-flash":      "gemini-3.8-flash",
							"gemini-2.5-pro":        "gemini-2.5-pro",
							"gemini-3-pro-image":    "gemini-3-pro-image",
							"claude-sonnet-4-6":     "claude-sonnet-4-6",
						},
					},
				}
				repo := &stubAntigravityAccountRepo{}
				svc := &AntigravityGatewayService{}
				resetAt := time.Now().Add(time.Hour)
				require.True(t, svc.setAntigravityModelRateLimits(context.Background(), repo, account,
					pool.exhausted, "[test]", http.StatusTooManyRequests, resetAt, false))

				// Recreate the persisted limits, then read a fresh account through JSON,
				// as the scheduler does instead of using the forwarding account object.
				limits := make(map[string]any)
				for _, call := range repo.modelRateLimitCalls {
					require.Equal(t, account.ID, call.accountID)
					limits[call.modelKey] = map[string]any{"rate_limit_reset_at": call.resetAt.Format(time.RFC3339)}
				}
				account.Extra = map[string]any{modelRateLimitsKey: limits}
				data, err := json.Marshal(account)
				require.NoError(t, err)
				var snapshot Account
				require.NoError(t, json.Unmarshal(data, &snapshot))

				for _, model := range pool.same {
					require.False(t, snapshot.IsSchedulableForModel(model), "exhausted pool: %s", model)
					require.Positive(t, snapshot.GetRateLimitRemainingTime(model), "exhausted pool: %s", model)
				}
				for _, model := range pool.other {
					require.Equal(t, plan.separate, snapshot.IsSchedulableForModel(model), "other pool: %s", model)
					require.Equal(t, !plan.separate, snapshot.GetRateLimitRemainingTime(model) > 0, "other pool: %s", model)
				}
				for _, model := range []string{"gemini-3-pro-image", "claude-sonnet-4-6"} {
					require.True(t, snapshot.IsSchedulableForModel(model), "unrelated model: %s", model)
					require.Zero(t, snapshot.GetRateLimitRemainingTime(model))
				}
			})
		}
	}
}

//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

func TestFetchQuotaUsesForwardEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mode      string
		plan      string
		wantDaily bool
	}{
		{name: "free defaults to production", plan: "free"},
		{name: "paid defaults to daily", plan: "pro", wantDaily: true},
		{name: "daily override", mode: " daily ", plan: "free", wantDaily: true},
		{name: "sandbox override", mode: "sandbox", plan: "free", wantDaily: true},
		{name: "production override", mode: "prod", plan: "pro"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(antigravityForwardBaseURLEnv, tc.mode)
			var productionCalls, dailyCalls atomic.Int32
			handler := func(calls *atomic.Int32, remaining float64) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Path == "/v1internal:loadCodeAssist" {
						_, _ = w.Write([]byte(`{"currentTier":{"id":"free-tier"}}`))
						return
					}
					calls.Add(1)
					assert.Equal(t, "/v1internal:fetchAvailableModels", r.URL.Path)
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
					var body antigravity.FetchAvailableModelsRequest
					if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
						http.Error(w, "invalid request", http.StatusBadRequest)
						return
					}
					assert.Equal(t, "project", body.Project)
					assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"models": map[string]any{"gemini-test": map[string]any{
							"quotaInfo": map[string]any{"remainingFraction": remaining},
						}},
					}))
				}
			}
			production := httptest.NewServer(handler(&productionCalls, 1))
			defer production.Close()
			daily := httptest.NewServer(handler(&dailyCalls, 0.75))
			defer daily.Close()
			setQuotaTestEndpoints(t, production.URL, daily.URL)

			result, err := NewAntigravityQuotaFetcher(nil, nil).FetchQuota(context.Background(), &Account{
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"access_token": "token", "project_id": "project", "plan_type": tc.plan,
				},
			}, "")
			require.NoError(t, err)
			if tc.wantDaily {
				require.EqualValues(t, 0, productionCalls.Load())
				require.EqualValues(t, 1, dailyCalls.Load())
				require.Equal(t, 25, result.UsageInfo.AntigravityQuota["gemini-test"].Utilization)
			} else {
				require.EqualValues(t, 1, productionCalls.Load())
				require.EqualValues(t, 0, dailyCalls.Load())
				require.Equal(t, 0, result.UsageInfo.AntigravityQuota["gemini-test"].Utilization)
			}
		})
	}
}

func TestFetchQuotaDoesNotUseAnotherEndpointOnFailure(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Setenv(antigravityForwardBaseURLEnv, "daily")
			var productionCalls atomic.Int32
			production := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				productionCalls.Add(1)
				_, _ = w.Write([]byte(`{"models":{}}`))
			}))
			defer production.Close()
			daily := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"denied"}`))
			}))
			defer daily.Close()
			setQuotaTestEndpoints(t, production.URL, daily.URL)

			result, err := NewAntigravityQuotaFetcher(nil, nil).FetchQuota(context.Background(), &Account{
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"access_token": "token"},
			}, "")
			if status == http.StatusForbidden {
				require.NoError(t, err)
				require.True(t, result.UsageInfo.IsForbidden)
			} else {
				require.ErrorContains(t, err, fmt.Sprintf("HTTP %d", status))
				require.Nil(t, result)
			}
			require.EqualValues(t, 0, productionCalls.Load())
		})
	}
}

func setQuotaTestEndpoints(t *testing.T, urls ...string) {
	t.Helper()
	oldURLs := antigravity.BaseURLs
	oldAvailability := antigravity.DefaultURLAvailability
	t.Cleanup(func() {
		antigravity.BaseURLs = oldURLs
		antigravity.DefaultURLAvailability = oldAvailability
	})
	antigravity.BaseURLs = urls
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)
}

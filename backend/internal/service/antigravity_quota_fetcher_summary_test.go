//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestAntigravityQuotaFetcher_SummaryFallback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		summary   string
		forbidden bool
		groups    int
	}{
		{"quota subscription unavailable", 403, `{"error":{"details":[{"reason":"SUBSCRIPTION_REQUIRED"}]}}`, false, 0},
		{"account validation required", 403, `{"error":{"message":"VALIDATION_REQUIRED"}}`, true, 0},
		{"real grouped quota", 200, `{"groups":[{"displayName":"Gemini 4 Argon","buckets":[{"bucketId":"gemini-argon-wtu-weekly","window":"weekly","remainingFraction":0.6}]}]}`, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1internal:retrieveUserQuotaSummary":
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.JSONEq(t, `{"project":"test-project"}`, string(body))
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, tc.summary)
				case "/v1internal:fetchAvailableModels":
					_, _ = io.WriteString(w, `{"models":{"gemini-4-argon":{"quotaInfo":{"remainingFraction":0.6}}}}`)
				case "/v1internal:loadCodeAssist":
					_, _ = io.WriteString(w, `{}`)
				default:
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			oldURLs, oldAvailability := antigravity.BaseURLs, antigravity.DefaultURLAvailability
			antigravity.BaseURLs = []string{server.URL}
			antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)
			t.Cleanup(func() {
				antigravity.BaseURLs = oldURLs
				antigravity.DefaultURLAvailability = oldAvailability
			})
			fetcher := NewAntigravityQuotaFetcher(nil, nil)
			result, err := fetcher.FetchQuota(context.Background(), &Account{Credentials: map[string]any{
				"access_token": "test-token", "project_id": "test-project",
			}}, "")
			require.NoError(t, err)
			require.Equal(t, tc.forbidden, result.UsageInfo.IsForbidden)
			require.Len(t, result.UsageInfo.AntigravityQuotaGroups, tc.groups)
			if !tc.forbidden && tc.groups == 0 {
				require.Equal(t, "models", result.UsageInfo.AntigravityQuotaSource)
				require.NotEmpty(t, result.UsageInfo.AntigravityQuotaError)
				require.Contains(t, result.UsageInfo.AntigravityQuota, "gemini-4-argon")
				require.Nil(t, result.UsageInfo.FiveHour)
				require.Nil(t, result.UsageInfo.SevenDay)
			}
		})
	}
}

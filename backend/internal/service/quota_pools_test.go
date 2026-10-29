package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

func TestQuotaPoolsFromSummary(t *testing.T) {
	var summary antigravity.QuotaSummary
	err := json.Unmarshal([]byte(`{"groups":[{"displayName":"Gemini Models","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.74},{"bucketId":"gemini-5h","window":"5h","remainingFraction":0},{"bucketId":"gemini-5h","window":"5h","remainingFraction":1}]},{"displayName":"Claude and GPT models","buckets":[{"bucketId":"3p-weekly","window":"weekly"}]}]}`), &summary)
	if err != nil {
		t.Fatal(err)
	}
	pools := quotaPoolsFromSummary(&summary)
	if len(pools) != 2 || len(pools[0].Windows) != 2 {
		t.Fatalf("unexpected pools: %+v", pools)
	}
	if *pools[0].Windows[0].Remaining != 0.74 || *pools[0].Windows[1].Remaining != 0 {
		t.Fatal("lost measured fraction")
	}
	if pools[1].Windows[0].Remaining != nil {
		t.Fatal("missing fraction treated as full/zero")
	}
}

func TestQuotaPoolsFromSummaryAdditionalWindow(t *testing.T) {
	remaining := 0.6
	invalid := 1.5
	summary := &antigravity.QuotaSummary{Groups: []antigravity.QuotaSummaryGroup{{
		DisplayName: "New Pool",
		Buckets: []antigravity.QuotaSummaryBucket{
			{ID: "pool-daily", Window: "daily", RemainingFraction: &remaining, ResetTime: "2026-11-01T00:00:00Z"},
			{ID: "pool-weekly", Window: "weekly", RemainingFraction: &invalid},
			{ID: "unrelated", Window: "monthly", RemainingFraction: &remaining},
		},
	}}}
	pools := quotaPoolsFromSummary(summary)
	if len(pools) != 1 || len(pools[0].Windows) != 2 {
		t.Fatalf("unexpected pools: %+v", pools)
	}
	daily := pools[0].Windows[0]
	if daily.ID != "daily" || daily.Remaining == nil || *daily.Remaining != 0.6 || daily.ResetsAt != "2026-11-01T00:00:00Z" {
		t.Fatalf("lost reported window: %+v", daily)
	}
	if pools[0].Windows[1].Remaining != nil {
		t.Fatal("invalid fraction should be unknown")
	}
}

func TestAntigravityQuotaSummaryPreservesExistingUsage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantPools int
	}{
		{"success", 200, `{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.7}]}]}`, 1},
		{"forbidden", 403, `{}`, 0},
		{"rate_limited", 429, `{}`, 0},
		{"invalid_json", 200, `{`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls[r.URL.Path]++
				switch r.URL.Path {
				case "/v1internal:fetchAvailableModels":
					_, _ = w.Write([]byte(`{"models":{"gemini-2.5-pro":{"quotaInfo":{"remainingFraction":0.8}}}}`))
				case "/v1internal:loadCodeAssist":
					_, _ = w.Write([]byte(`{"paidTier":{"id":"g1-pro-tier"}}`))
				case "/v1internal:retrieveUserQuotaSummary":
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				default:
					t.Errorf("unexpected path: %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			original := antigravity.BaseURLs
			antigravity.BaseURLs = []string{server.URL}
			t.Cleanup(func() { antigravity.BaseURLs = original })
			account := &Account{Platform: PlatformAntigravity, Status: "active", Credentials: map[string]any{"access_token": "test", "project_id": "test-project"}}
			result, err := NewAntigravityQuotaFetcher(nil, nil).FetchQuota(context.Background(), account, "")
			if err != nil {
				t.Fatal(err)
			}
			usage := result.UsageInfo
			if len(usage.QuotaPools) != tc.wantPools || usage.AntigravityQuota["gemini-2.5-pro"] == nil || usage.SubscriptionTier != "PRO" {
				t.Fatalf("existing usage or summary lost: %+v", usage)
			}
			if usage.IsForbidden || usage.IsBanned || usage.NeedsVerify || account.Status != "active" {
				t.Fatal("summary failure changed account status")
			}
			for _, path := range []string{"fetchAvailableModels", "loadCodeAssist", "retrieveUserQuotaSummary"} {
				if calls["/v1internal:"+path] != 1 {
					t.Fatalf("unexpected calls: %+v", calls)
				}
			}
		})
	}
}

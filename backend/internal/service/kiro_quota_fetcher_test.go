package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

func TestKiroQuotaFetcher_CanFetch(t *testing.T) {
	f := NewKiroQuotaFetcher(nil)

	if f.CanFetch(nil) {
		t.Fatal("CanFetch(nil) should be false")
	}
	if f.CanFetch(&Account{Platform: PlatformAnthropic}) {
		t.Fatal("non-kiro platform should not be fetchable")
	}
	if f.CanFetch(&Account{Platform: PlatformKiro}) {
		t.Fatal("kiro with no access token should not be fetchable")
	}
	acc := &Account{
		Platform:    PlatformKiro,
		Credentials: map[string]any{"access_token": "tok"},
	}
	if !f.CanFetch(acc) {
		t.Fatal("kiro with access token should be fetchable")
	}
}

func TestKiroQuotaFetcher_FetchQuota_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"userInfo": {"email": "u@e", "userId": "u-1"},
			"subscriptionInfo": {
				"subscriptionType": "PRO",
				"subscriptionTitle": "Pro Plan",
				"status": "ACTIVE"
			},
			"nextDateReset": "1735689600",
			"usageBreakdownList": [{
				"resourceType": "AGENTIC_REQUEST",
				"currentUsage": 250,
				"usageLimit": 1000,
				"freeTrialInfo": {
					"currentUsage": 10,
					"usageLimit": 50,
					"freeTrialStatus": "ACTIVE"
				}
			}]
		}`))
	}))
	defer srv.Close()

	// Inject the fake upstream by swapping the package URL through the
	// pkg/kiro test seam.
	kiro.OverrideProfileURLForTest(t, srv.URL)

	f := NewKiroQuotaFetcher(nil)
	acc := &Account{
		Platform:    PlatformKiro,
		Credentials: map[string]any{"access_token": "tok-x"},
	}
	result, err := f.FetchQuota(context.Background(), acc, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.UsageInfo == nil {
		t.Fatal("UsageInfo nil")
	}
	if result.UsageInfo.SubscriptionTier != "PRO" {
		t.Fatalf("tier = %q", result.UsageInfo.SubscriptionTier)
	}
	subRaw, _ := result.Raw["subscription"].(map[string]any)
	if subRaw["type"] != "PRO" || subRaw["title"] != "Pro Plan" {
		t.Fatalf("subscription raw wrong: %+v", subRaw)
	}
	usageRaw, _ := result.Raw["usage"].(map[string]any)
	if usageRaw["current"].(float64) != 250 || usageRaw["limit"].(float64) != 1000 {
		t.Fatalf("usage raw wrong: %+v", usageRaw)
	}
	if usageRaw["trial_status"] != "ACTIVE" {
		t.Fatalf("trial missing: %+v", usageRaw)
	}
	if usageRaw["percent"].(float64) != 0.25 {
		t.Fatalf("percent = %v", usageRaw["percent"])
	}
}

func TestKiroQuotaFetcher_FetchQuota_NoAccessToken(t *testing.T) {
	f := NewKiroQuotaFetcher(nil)
	_, err := f.FetchQuota(context.Background(), &Account{Platform: PlatformKiro}, "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestKiroQuotaFetcher_FetchQuota_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "unauthorized"})
	}))
	defer srv.Close()
	kiro.OverrideProfileURLForTest(t, srv.URL)

	f := NewKiroQuotaFetcher(nil)
	acc := &Account{
		Platform:    PlatformKiro,
		Credentials: map[string]any{"access_token": "tok"},
	}
	_, err := f.FetchQuota(context.Background(), acc, "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestKiroSubscriptionTier(t *testing.T) {
	cases := map[string]string{
		"FREE":     "FREE",
		"PRO":      "PRO",
		"PRO_PLUS": "PRO",
		"POWER":    "ULTRA",
		"":         "UNKNOWN",
		"unknown":  "UNKNOWN",
	}
	for raw, want := range cases {
		if got := kiroSubscriptionTier(raw); got != want {
			t.Errorf("kiroSubscriptionTier(%q) = %q, want %q", raw, got, want)
		}
	}
}

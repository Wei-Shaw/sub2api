package kiro

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchProfile_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer at-x" {
			t.Fatalf("missing/invalid bearer: %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userInfo": map[string]any{
				"email":  "u@e.com",
				"userId": "uid-1",
			},
		})
	}))
	defer srv.Close()

	p, err := fetchProfileAt(srv.URL, "at-x", http.DefaultClient)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if p.Email != "u@e.com" || p.UserID != "uid-1" {
		t.Fatalf("unexpected profile: %+v", p)
	}
}

func TestFetchProfile_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"unauthorized"}`))
	}))
	defer srv.Close()
	_, err := fetchProfileAt(srv.URL, "at", http.DefaultClient)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}

func TestFetchUsageLimits_FullResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"userInfo": {"email": "u@e", "userId": "u-1"},
			"subscriptionInfo": {
				"subscriptionName": "Pro",
				"subscriptionTitle": "Pro Plan",
				"subscriptionType": "PRO",
				"status": "ACTIVE"
			},
			"nextDateReset": "1735689600",
			"usageBreakdownList": [
				{
					"resourceType": "AGENTIC_REQUEST",
					"currentUsage": 123.45,
					"usageLimit": 1000,
					"currency": "USD",
					"unit": "credits"
				},
				{
					"resourceType": "OTHER",
					"currentUsage": 0,
					"usageLimit": 100
				}
			]
		}`))
	}))
	defer srv.Close()

	u, err := fetchUsageLimitsAt(srv.URL, "at", http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if u.UserInfo == nil || u.UserInfo.Email != "u@e" {
		t.Fatalf("user info wrong: %+v", u.UserInfo)
	}
	if u.SubscriptionInfo == nil || u.SubscriptionInfo.SubscriptionType != "PRO" {
		t.Fatalf("subscription wrong: %+v", u.SubscriptionInfo)
	}
	agentic := u.AgenticUsage()
	if agentic == nil {
		t.Fatal("agentic usage missing")
	}
	if agentic.CurrentUsage != 123.45 || agentic.UsageLimit != 1000 {
		t.Fatalf("agentic usage wrong: %+v", agentic)
	}
}

func TestFetchUsageLimits_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := fetchUsageLimitsAt(srv.URL, "at", http.DefaultClient)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 error, got %v", err)
	}
}

func TestUsageLimits_AgenticUsage_NoMatch(t *testing.T) {
	u := &UsageLimits{
		UsageBreakdown: []UsageBreakdown{{ResourceType: "OTHER"}},
	}
	if u.AgenticUsage() != nil {
		t.Fatal("expected nil when no AGENTIC_REQUEST row")
	}
}

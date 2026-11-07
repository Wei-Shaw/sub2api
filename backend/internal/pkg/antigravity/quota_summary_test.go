//go:build unit

package antigravity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The shape Google returned for a consumer Ultra account on 2026-11-04.
const quotaSummaryBody = `{
  "groups": [
    {"buckets": [
       {"bucketId": "gemini-weekly", "displayName": "Weekly Limit Remaining", "window": "weekly",
        "resetTime": "2026-11-11T15:52:48Z", "remainingFraction": 0.75},
       {"bucketId": "gemini-5h", "displayName": "Five Hour Limit Remaining", "window": "5h",
        "resetTime": "2026-11-04T20:52:48Z", "remainingFraction": 0.4}],
     "displayName": "Gemini Models", "description": "Models within this group: Gemini Flash, Gemini Pro"},
    {"buckets": [
       {"bucketId": "3p-weekly", "window": "weekly", "resetTime": "2026-11-11T15:52:48Z", "remainingFraction": 1},
       {"bucketId": "3p-5h", "window": "5h", "remainingFraction": 1}],
     "displayName": "Claude and GPT models"}
  ],
  "description": "Within each group, models share a weekly limit and a 5-hour limit."
}`

func TestClient_RetrieveUserQuotaSummaryFromURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:retrieveUserQuotaSummary" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("unexpected authorization header")
		}
		var body FetchAvailableModelsRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Project != "proj" {
			t.Errorf("unexpected project: %+v, %v", body, err)
		}
		_, _ = w.Write([]byte(quotaSummaryBody))
	}))
	defer server.Close()

	client := mustNewClient(t, "")
	summary, err := client.RetrieveUserQuotaSummaryFromURL(context.Background(), server.URL, "tok", "proj")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(summary.Groups) != 2 || summary.Groups[0].DisplayName != "Gemini Models" {
		t.Fatalf("groups: %+v", summary.Groups)
	}
	weekly := summary.Groups[0].Buckets[0]
	if weekly.BucketID != "gemini-weekly" || weekly.Window != "weekly" || weekly.RemainingFraction != 0.75 ||
		weekly.ResetTime != "2026-11-11T15:52:48Z" {
		t.Fatalf("weekly bucket: %+v", weekly)
	}
	if summary.Groups[1].Buckets[1].ResetTime != "" {
		t.Fatalf("a bucket without resetTime keeps it empty")
	}
}

func TestClient_RetrieveUserQuotaSummaryFromURL_Errors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{http.StatusForbidden, `{"error":{"code":403}}`}, {http.StatusInternalServerError, `{}`}, {http.StatusOK, `not json`}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		client := mustNewClient(t, "")
		if _, err := client.RetrieveUserQuotaSummaryFromURL(context.Background(), server.URL, "tok", "proj"); err == nil {
			t.Errorf("status %d body %q must be an error", tc.status, tc.body)
		}
		server.Close()
	}
}

func TestClient_RetrieveUserQuotaSummary_403RetriesPlainUserAgent(t *testing.T) {
	var gotUserAgents []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgents = append(gotUserAgents, r.Header.Get("User-Agent"))
		if len(gotUserAgents) == 1 {
			// The configured hub UA is rejected by the summary endpoint only.
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":403,"message":"Verify your account to continue."}}`))
			return
		}
		_, _ = w.Write([]byte(quotaSummaryBody))
	}))
	defer server.Close()

	previous := BaseURLs
	BaseURLs = []string{server.URL}
	t.Cleanup(func() { BaseURLs = previous })

	client := mustNewClient(t, "")
	summary, err := client.RetrieveUserQuotaSummary(context.Background(), "tok", "proj")
	if err != nil {
		t.Fatalf("summary after UA retry: %v", err)
	}
	if len(summary.Groups) != 2 {
		t.Fatalf("groups: %+v", summary.Groups)
	}
	if len(gotUserAgents) != 2 {
		t.Fatalf("expected a single retry, got %d requests", len(gotUserAgents))
	}
	if gotUserAgents[1] != quotaSummaryFallbackUserAgent {
		t.Fatalf("retry used %q, want %q", gotUserAgents[1], quotaSummaryFallbackUserAgent)
	}
}

func TestClient_RetrieveUserQuotaSummary_403BothUserAgentsFails(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403}}`))
	}))
	defer server.Close()

	previous := BaseURLs
	BaseURLs = []string{server.URL}
	t.Cleanup(func() { BaseURLs = previous })

	client := mustNewClient(t, "")
	if _, err := client.RetrieveUserQuotaSummary(context.Background(), "tok", "proj"); err == nil {
		t.Fatal("both UAs failing must surface an error")
	}
	if requests != 2 {
		t.Fatalf("expected the plain-UA retry, got %d requests", requests)
	}
}

func TestClient_RetrieveUserQuotaSummary_Non403NoUserAgentRetry(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	previous := BaseURLs
	BaseURLs = []string{server.URL}
	t.Cleanup(func() { BaseURLs = previous })

	client := mustNewClient(t, "")
	if _, err := client.RetrieveUserQuotaSummary(context.Background(), "tok", "proj"); err == nil {
		t.Fatal("500 must surface an error")
	}
	if requests != 1 {
		t.Fatalf("500 must not trigger a UA retry, got %d requests", requests)
	}
}

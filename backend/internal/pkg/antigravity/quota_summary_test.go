package antigravity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchQuotaSummaryAccountIsolation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1internal:retrieveUserQuotaSummary" || r.Header.Get("User-Agent") == "" {
			t.Error("invalid quota request")
		}
		var body FetchAvailableModelsRequest
		json.NewDecoder(r.Body).Decode(&body)
		if body.Project != "account-project" {
			t.Error("wrong project")
		}
		if r.Header.Get("Authorization") == "Bearer account-a" {
			w.Write([]byte(`{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.7}]}]}`))
		} else {
			w.WriteHeader(429)
		}
	}))
	defer server.Close()
	original := BaseURLs
	BaseURLs = []string{server.URL}
	t.Cleanup(func() { BaseURLs = original })
	client, err := NewClient("")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.FetchQuotaSummary(context.Background(), "account-a", "account-project")
	if err != nil || len(result.Groups) != 1 {
		t.Fatalf("missing result %v", err)
	}
	if _, err = client.FetchQuotaSummary(context.Background(), "account-b", "account-project"); err == nil {
		t.Fatal("429 accepted")
	}
	if calls != 2 {
		t.Fatalf("unexpected retries: %d", calls)
	}
}

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

func TestFetchProfile_EmptyUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Some accounts return an empty userInfo block; we tolerate it.
		_, _ = w.Write([]byte(`{"userInfo":{}}`))
	}))
	defer srv.Close()
	p, err := fetchProfileAt(srv.URL, "at", http.DefaultClient)
	if err != nil {
		t.Fatalf("expected nil error on empty userInfo, got %v", err)
	}
	if p.Email != "" || p.UserID != "" {
		t.Fatalf("expected zero profile, got %+v", p)
	}
}

package kiro

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRefreshOIDCAt_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grantType"] != "refresh_token" {
			t.Fatalf("grantType=%q", body["grantType"])
		}
		if body["clientId"] != "cid" || body["clientSecret"] != "cs" {
			t.Fatalf("client creds wrong: %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accessToken":  "at-new",
			"refreshToken": "rt-new",
			"expiresIn":    3600,
			"profileArn":   "arn:cw",
		})
	}))
	defer srv.Close()

	info, err := refreshOIDCAt(srv.URL, "cid", "cs", "rt-old", AuthMethodIdC, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if info.AccessToken != "at-new" || info.RefreshToken != "rt-new" ||
		info.AuthMethod != AuthMethodIdC || info.ProfileARN != "arn:cw" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestRefreshOIDCAt_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()
	_, err := refreshOIDCAt(srv.URL, "cid", "cs", "rt", AuthMethodBuilderID, http.DefaultClient)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected 400 error, got %v", err)
	}
}

func TestRefreshOIDCAt_EmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "", "expiresIn": 3600})
	}))
	defer srv.Close()
	_, err := refreshOIDCAt(srv.URL, "cid", "cs", "rt", AuthMethodIdC, http.DefaultClient)
	if err == nil || !strings.Contains(err.Error(), "empty access token") {
		t.Fatalf("expected empty-token error, got %v", err)
	}
}

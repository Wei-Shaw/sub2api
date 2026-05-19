package kiro

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRefreshSocial_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/refreshToken" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["refreshToken"] != "rt-old" {
			t.Fatalf("got refreshToken=%q", body["refreshToken"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accessToken":  "at-new",
			"refreshToken": "rt-new",
			"expiresIn":    3600,
			"profileArn":   "arn:aws:cw:foo",
		})
	}))
	defer srv.Close()

	info, err := refreshSocialAt(srv.URL+"/refreshToken", "rt-old", http.DefaultClient)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if info.AccessToken != "at-new" {
		t.Fatalf("access_token = %q", info.AccessToken)
	}
	if info.RefreshToken != "rt-new" {
		t.Fatalf("refresh_token = %q", info.RefreshToken)
	}
	if info.ProfileARN != "arn:aws:cw:foo" {
		t.Fatalf("profile_arn = %q", info.ProfileARN)
	}
	if info.ExpiresAt == 0 {
		t.Fatalf("expires_at not set")
	}
	if info.AuthMethod != AuthMethodSocial {
		t.Fatalf("auth_method = %v", info.AuthMethod)
	}
}

func TestRefreshSocial_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()
	_, err := refreshSocialAt(srv.URL+"/refreshToken", "rt-bad", http.DefaultClient)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected 400 error, got %v", err)
	}
}

func TestRefreshSocial_EmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accessToken":  "",
			"refreshToken": "rt-new",
			"expiresIn":    3600,
		})
	}))
	defer srv.Close()
	_, err := refreshSocialAt(srv.URL+"/refreshToken", "rt", http.DefaultClient)
	if err == nil || !strings.Contains(err.Error(), "empty access token") {
		t.Fatalf("expected empty-token error, got %v", err)
	}
}

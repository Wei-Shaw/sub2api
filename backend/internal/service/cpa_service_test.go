//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newCpaTestService(t *testing.T, baseURL string, enabled bool, key string) *CpaService {
	t.Helper()
	repo := newMockSettingRepo()
	repo.data[SettingKeyCpaManagementEnabled] = boolToString(enabled)
	repo.data[SettingKeyCpaManagementBaseURL] = baseURL
	repo.data[SettingKeyCpaManagementKey] = key
	return NewCpaService(&SettingService{settingRepo: repo})
}

func TestNormalizeCpaManagementBaseURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"http://host.docker.internal:8317", "http://host.docker.internal:8317"},
		{"http://host.docker.internal:8317/", "http://host.docker.internal:8317"},
		{"  http://127.0.0.1:8317///  ", "http://127.0.0.1:8317"},
		{"127.0.0.1:8317", "http://127.0.0.1:8317"},
		{"https://cpa.example.com", "https://cpa.example.com"},
	}
	for _, tc := range cases {
		if got := NormalizeCpaManagementBaseURL(tc.in); got != tc.want {
			t.Fatalf("NormalizeCpaManagementBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCpaConfigViewNeverExposesManagementKey(t *testing.T) {
	cfg := &CpaConfig{Enabled: true, BaseURL: "http://cpa:8317", ManagementKey: "lIlI-secret-value-1234"}
	view := cfg.View()

	if !view.Enabled || view.BaseURL != "http://cpa:8317" {
		t.Fatalf("unexpected view: %#v", view)
	}
	if !view.ManagementKeyConfigured {
		t.Fatal("expected management_key_configured=true")
	}
	if strings.Contains(view.ManagementKeyHint, "secret-value") {
		t.Fatalf("hint leaks the key: %q", view.ManagementKeyHint)
	}
	if view.ManagementKeyHint != "lI" + strings.Repeat("*", len("lIlI-secret-value-1234")-4) + "34" {
		t.Fatalf("unexpected hint: %q", view.ManagementKeyHint)
	}

	empty := (&CpaConfig{BaseURL: "http://cpa:8317"}).View()
	if empty.ManagementKeyConfigured || empty.ManagementKeyHint != "" {
		t.Fatalf("expected no key configured, got %#v", empty)
	}
	if (&CpaConfig{BaseURL: "http://cpa:8317"}).Configured() {
		t.Fatal("config without key must not report Configured()")
	}
	if !(&CpaConfig{BaseURL: "http://cpa:8317", ManagementKey: "k"}).Configured() {
		t.Fatal("config with url+key must report Configured()")
	}
}

func TestMaskCpaManagementKey(t *testing.T) {
	if got := maskCpaManagementKey(""); got != "" {
		t.Fatalf("empty key mask = %q", got)
	}
	if got := maskCpaManagementKey("abc"); got != "***" {
		t.Fatalf("short key mask = %q", got)
	}
	if got := maskCpaManagementKey("abcdefghij"); got != "ab******ij" {
		t.Fatalf("long key mask = %q", got)
	}
}

func TestCpaServiceListAuthFilesSendsManagementKey(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v0/management/auth-files" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"files":[{"name":"codex-pro.json","auth_index":"idx-1"}]}`))
	}))
	defer server.Close()

	svc := newCpaTestService(t, server.URL, true, "test-management-key")
	raw, err := svc.ListAuthFiles(context.Background())
	if err != nil {
		t.Fatalf("ListAuthFiles error: %v", err)
	}
	if gotAuth != "Bearer test-management-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if !strings.Contains(string(raw), "codex-pro.json") {
		t.Fatalf("unexpected payload: %s", raw)
	}
}

func TestCpaServiceRejectsInvalidManagementKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid management key"}`))
	}))
	defer server.Close()

	svc := newCpaTestService(t, server.URL, true, "bad-key")
	if _, err := svc.ListAuthFiles(context.Background()); !errors.Is(err, ErrCpaKeyInvalid) {
		t.Fatalf("expected ErrCpaKeyInvalid, got %v", err)
	}
}

func TestCpaServiceRequiresEnabledConfig(t *testing.T) {
	svc := newCpaTestService(t, "http://127.0.0.1:1", false, "key")
	if _, err := svc.ListAuthFiles(context.Background()); !errors.Is(err, ErrCpaNotConfigured) {
		t.Fatalf("expected ErrCpaNotConfigured when disabled, got %v", err)
	}

	svcNoKey := newCpaTestService(t, "http://127.0.0.1:1", true, "")
	if _, err := svcNoKey.ListAuthFiles(context.Background()); !errors.Is(err, ErrCpaNotConfigured) {
		t.Fatalf("expected ErrCpaNotConfigured without key, got %v", err)
	}
}

func TestCpaServiceAuthFileQuotaUsesAPICallEnvelope(t *testing.T) {
	var sentBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/management/api-call" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		sentBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status_code":200,"header":{},"body":"{\"plan_type\":\"pro\",\"rate_limit\":{\"primary_window\":{\"used_percent\":42}}}"}`))
	}))
	defer server.Close()

	svc := newCpaTestService(t, server.URL, true, "test-management-key")
	raw, err := svc.AuthFileQuota(context.Background(), "idx-1")
	if err != nil {
		t.Fatalf("AuthFileQuota error: %v", err)
	}

	var sent map[string]any
	if err := json.Unmarshal(sentBody, &sent); err != nil {
		t.Fatalf("request body is not JSON: %v (%s)", err, sentBody)
	}
	if sent["auth_index"] != "idx-1" {
		t.Fatalf("auth_index = %v", sent["auth_index"])
	}
	if sent["url"] != cpaCodexUsageURL {
		t.Fatalf("url = %v", sent["url"])
	}
	headers, _ := sent["header"].(map[string]any)
	if headers["Authorization"] != "Bearer $TOKEN$" {
		t.Fatalf("Authorization header = %v", headers["Authorization"])
	}

	var payload struct {
		PlanType string `json:"plan_type"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("quota payload is not JSON: %v (%s)", err, raw)
	}
	if payload.PlanType != "pro" {
		t.Fatalf("plan_type = %q, want pro", payload.PlanType)
	}
}

func TestCpaServiceAuthFileQuotaSurfacesUpstreamStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status_code":401,"header":{},"body":"unauthorized"}`))
	}))
	defer server.Close()

	svc := newCpaTestService(t, server.URL, true, "test-management-key")
	if _, err := svc.AuthFileQuota(context.Background(), "idx-1"); err == nil {
		t.Fatal("expected an error for upstream 401")
	}
}

func TestCpaServiceAuthFileQuotaRequiresAuthIndex(t *testing.T) {
	svc := newCpaTestService(t, "http://127.0.0.1:1", true, "key")
	if _, err := svc.AuthFileQuota(context.Background(), "   "); err == nil {
		t.Fatal("expected an error for empty auth_index")
	}
}

func TestCpaServiceVersionReadsHeadersOnUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("X-CPA-Version", "7.2.147")
		w.Header().Set("X-CPA-Commit", "17a65ee5")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"missing management key"}`))
	}))
	defer server.Close()

	svc := newCpaTestService(t, server.URL, true, "wrong-key")
	info, err := svc.Version(context.Background())
	if err != nil {
		t.Fatalf("Version error: %v", err)
	}
	if !info.Reachable || !info.HealthOK {
		t.Fatalf("expected reachable+healthy, got %#v", info)
	}
	if info.Version != "7.2.147" || info.Commit != "17a65ee5" {
		t.Fatalf("unexpected version info: %#v", info)
	}
	if info.KeyAccepted {
		t.Fatalf("expected key_accepted=false, got %#v", info)
	}
	if info.Message == "" {
		t.Fatal("expected an error message for the rejected key")
	}
}

func TestCpaServiceRedactsManagementKeyFromErrors(t *testing.T) {
	secret := "super-secret-key"
	err := cpaRedactError(errors.New("dial tcp: auth "+secret+" failed"), secret)
	if strings.Contains(err, secret) {
		t.Fatalf("error leaks the management key: %s", err)
	}
	if !strings.Contains(err, "***") {
		t.Fatalf("expected redaction marker: %s", err)
	}
}

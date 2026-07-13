package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newCodexModelsTestAccount() *Account {
	return &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acc-123",
		},
	}
}

func newSchedulableCodexModelsTestAccount(id int64, priority int, accessToken string) Account {
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    priority,
		Credentials: map[string]any{
			"access_token":       accessToken,
			"chatgpt_account_id": "acc-123",
		},
	}
}

func useCodexModelsTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	t.Cleanup(func() {
		chatgptCodexModelsURL = original
		server.Close()
	})
}

func TestFetchCodexModelsManifestWithFailoverRetriesAnotherAccountInGroup(t *testing.T) {
	const manifestBody = `{"models":[{"slug":"gpt-5.6-sol"}]}`
	groupID := int64(42)
	first := newSchedulableCodexModelsTestAccount(1, 0, "first-token")
	first.AccountGroups = []AccountGroup{{GroupID: groupID}}
	outsideGroup := newSchedulableCodexModelsTestAccount(2, 0, "outside-token")
	backup := newSchedulableCodexModelsTestAccount(3, 1, "backup-token")
	backup.AccountGroups = []AccountGroup{{GroupID: groupID}}

	var gotAuth []string
	useCodexModelsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		switch r.Header.Get("Authorization") {
		case "Bearer first-token":
			http.Error(w, `{"detail":"first account failed"}`, http.StatusBadGateway)
		case "Bearer backup-token":
			w.Header().Set("ETag", `W/"backup"`)
			_, _ = w.Write([]byte(manifestBody))
		default:
			http.Error(w, `{"detail":"unexpected account"}`, http.StatusBadGateway)
		}
	})

	repo := groupAwareStubOpenAIAccountRepo{stubOpenAIAccountRepo{accounts: []Account{first, outsideGroup, backup}}}
	s := &OpenAIGatewayService{accountRepo: repo}
	manifest, err := s.FetchCodexModelsManifestWithFailover(context.Background(), &groupID, &first, "0.144.1", "")
	if err != nil {
		t.Fatalf("FetchCodexModelsManifestWithFailover returned error: %v", err)
	}
	if string(manifest.Body) != manifestBody {
		t.Errorf("body not passed through from backup account: got %q", manifest.Body)
	}
	if manifest.ETag != `W/"backup"` {
		t.Errorf("etag not passed through from backup account: got %q", manifest.ETag)
	}
	wantAuth := []string{"Bearer first-token", "Bearer backup-token"}
	if len(gotAuth) != len(wantAuth) {
		t.Fatalf("request count: got %d, want %d (%v)", len(gotAuth), len(wantAuth), gotAuth)
	}
	for i := range wantAuth {
		if gotAuth[i] != wantAuth[i] {
			t.Errorf("request %d authorization: got %q, want %q", i+1, gotAuth[i], wantAuth[i])
		}
	}
}

func TestFetchCodexModelsManifestWithFailoverStopsAfterFirstSuccess(t *testing.T) {
	requests := 0
	useCodexModelsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"models":[]}`))
	})

	s := &OpenAIGatewayService{}
	if _, err := s.FetchCodexModelsManifestWithFailover(context.Background(), nil, newCodexModelsTestAccount(), "0.144.1", ""); err != nil {
		t.Fatalf("FetchCodexModelsManifestWithFailover returned error: %v", err)
	}
	if requests != 1 {
		t.Fatalf("request count: got %d, want 1", requests)
	}
}

func TestFetchCodexModelsManifestWithFailoverSingleAccountReturnsFetchError(t *testing.T) {
	requests := 0
	useCodexModelsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, `{"detail":"only account failed"}`, http.StatusBadGateway)
	})

	account := newSchedulableCodexModelsTestAccount(1, 0, "only-token")
	s := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
	_, err := s.FetchCodexModelsManifestWithFailover(context.Background(), nil, &account, "0.144.1", "")
	if err == nil || !strings.Contains(err.Error(), "only account failed") {
		t.Fatalf("expected original manifest error, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("request count: got %d, want 1", requests)
	}
}

func TestFetchCodexModelsManifestWithFailoverReturnsSecondFetchError(t *testing.T) {
	first := newSchedulableCodexModelsTestAccount(1, 0, "first-token")
	second := newSchedulableCodexModelsTestAccount(2, 1, "second-token")

	var gotAuth []string
	useCodexModelsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") == "Bearer first-token" {
			http.Error(w, `{"detail":"first account failed"}`, http.StatusBadGateway)
			return
		}
		http.Error(w, `{"detail":"second account failed"}`, http.StatusBadGateway)
	})

	s := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: []Account{first, second}}}
	_, err := s.FetchCodexModelsManifestWithFailover(context.Background(), nil, &first, "0.144.1", "")
	if err == nil || !strings.Contains(err.Error(), "second account failed") {
		t.Fatalf("expected second manifest error, got %v", err)
	}
	wantAuth := []string{"Bearer first-token", "Bearer second-token"}
	if len(gotAuth) != len(wantAuth) {
		t.Fatalf("request count: got %d, want %d (%v)", len(gotAuth), len(wantAuth), gotAuth)
	}
	for i := range wantAuth {
		if gotAuth[i] != wantAuth[i] {
			t.Errorf("request %d authorization: got %q, want %q", i+1, gotAuth[i], wantAuth[i])
		}
	}
}

func TestFetchCodexModelsManifestWithFailoverDoesNotRetryCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requests := 0
	useCodexModelsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		cancel()
		http.Error(w, `{"detail":"first account failed"}`, http.StatusBadGateway)
	})

	first := newSchedulableCodexModelsTestAccount(1, 0, "first-token")
	second := newSchedulableCodexModelsTestAccount(2, 1, "second-token")
	s := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: []Account{first, second}}}
	if _, err := s.FetchCodexModelsManifestWithFailover(ctx, nil, &first, "0.144.1", ""); err == nil {
		t.Fatal("expected error for canceled manifest request, got nil")
	}
	if requests != 1 {
		t.Fatalf("request count: got %d, want 1", requests)
	}
}

func TestFetchCodexModelsManifestWithFailoverPreservesNotModified(t *testing.T) {
	requests := 0
	useCodexModelsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("ETag", `W/"abc123"`)
		w.WriteHeader(http.StatusNotModified)
	})

	s := &OpenAIGatewayService{}
	manifest, err := s.FetchCodexModelsManifestWithFailover(context.Background(), nil, newCodexModelsTestAccount(), "0.144.1", `W/"abc123"`)
	if err != nil {
		t.Fatalf("FetchCodexModelsManifestWithFailover returned error: %v", err)
	}
	if !manifest.NotModified || manifest.ETag != `W/"abc123"` {
		t.Fatalf("not-modified metadata: got %+v", manifest)
	}
	if requests != 1 {
		t.Fatalf("request count: got %d, want 1", requests)
	}
}

func TestFetchCodexModelsManifestPassthrough(t *testing.T) {
	manifestBody := `{"models":[{"slug":"gpt-5.5","display_name":"GPT-5.5"}]}`

	var gotAuth, gotAccountID, gotOriginator, gotClientVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccountID = r.Header.Get("chatgpt-account-id")
		gotOriginator = r.Header.Get("Originator")
		gotClientVersion = r.URL.Query().Get("client_version")
		w.Header().Set("ETag", `W/"abc123"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(manifestBody))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	s := &OpenAIGatewayService{}
	manifest, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.137.0", "")
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}

	if string(manifest.Body) != manifestBody {
		t.Errorf("body not passed through verbatim: got %q", manifest.Body)
	}
	if manifest.ETag != `W/"abc123"` {
		t.Errorf("etag not passed through: got %q", manifest.ETag)
	}
	if gotAuth != "Bearer test-access-token" {
		t.Errorf("authorization header: got %q", gotAuth)
	}
	if gotAccountID != "acc-123" {
		t.Errorf("chatgpt-account-id header: got %q", gotAccountID)
	}
	if gotOriginator != "codex_cli_rs" {
		t.Errorf("originator header: got %q", gotOriginator)
	}
	if gotClientVersion != "0.137.0" {
		t.Errorf("client_version query: got %q", gotClientVersion)
	}
}

func TestFetchCodexModelsManifestDefaultClientVersion(t *testing.T) {
	var gotClientVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClientVersion = r.URL.Query().Get("client_version")
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	s := &OpenAIGatewayService{}
	if _, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "", ""); err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}
	if gotClientVersion != openAICodexProbeVersion {
		t.Errorf("default client_version: got %q, want %q", gotClientVersion, openAICodexProbeVersion)
	}
}

func TestFetchCodexModelsManifestNotModified(t *testing.T) {
	var gotIfNoneMatch string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `W/"abc123"`)
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	s := &OpenAIGatewayService{}
	manifest, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.137.0", `W/"abc123"`)
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}
	if !manifest.NotModified {
		t.Error("expected NotModified to be true")
	}
	if gotIfNoneMatch != `W/"abc123"` {
		t.Errorf("if-none-match header: got %q", gotIfNoneMatch)
	}
}

func TestFetchCodexModelsManifestUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"boom"}`, http.StatusInternalServerError)
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	s := &OpenAIGatewayService{}
	if _, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.137.0", ""); err == nil {
		t.Fatal("expected error for upstream 500, got nil")
	}
}

func TestFetchCodexModelsManifestMissingToken(t *testing.T) {
	account := newCodexModelsTestAccount()
	delete(account.Credentials, "access_token")

	s := &OpenAIGatewayService{}
	if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", ""); err == nil {
		t.Fatal("expected error for missing access token, got nil")
	}
}

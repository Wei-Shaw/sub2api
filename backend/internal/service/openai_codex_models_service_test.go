package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type codexModelsTestHTTPUpstream struct {
	HTTPUpstream
	do func(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error)
}

type codexModelsTokenCache struct {
	OpenAITokenCache
	token string
}

func (c codexModelsTokenCache) GetAccessToken(context.Context, string) (string, error) {
	return c.token, nil
}

func (u *codexModelsTestHTTPUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	if u.do != nil {
		return u.do(req, proxyURL, accountID, accountConcurrency)
	}
	return http.DefaultClient.Do(req)
}

func newCodexModelsTestService() *OpenAIGatewayService {
	return &OpenAIGatewayService{httpUpstream: &codexModelsTestHTTPUpstream{}}
}

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

	s := newCodexModelsTestService()
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

	s := newCodexModelsTestService()
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

	s := newCodexModelsTestService()
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

	s := newCodexModelsTestService()
	if _, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.137.0", ""); err == nil {
		t.Fatal("expected error for upstream 500, got nil")
	}
}

func TestFetchCodexModelsManifestMissingToken(t *testing.T) {
	account := newCodexModelsTestAccount()
	delete(account.Credentials, "access_token")

	s := newCodexModelsTestService()
	if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", ""); err == nil {
		t.Fatal("expected error for missing access token, got nil")
	}
}

func TestFetchCodexModelsManifestUsesOpenAIHTTPUpstreamProfile(t *testing.T) {
	account := newCodexModelsTestAccount()
	account.Concurrency = 7
	account.Proxy = &Proxy{Host: "127.0.0.1", Port: 7890, Protocol: "http"}

	var gotProfile HTTPUpstreamProfile
	var gotProxyURL string
	var gotAccountID int64
	var gotConcurrency int
	var hadDeadline bool
	upstream := &codexModelsTestHTTPUpstream{do: func(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
		gotProfile = HTTPUpstreamProfileFromContext(req.Context())
		gotProxyURL = proxyURL
		gotAccountID = accountID
		gotConcurrency = accountConcurrency
		_, hadDeadline = req.Context().Deadline()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       http.NoBody,
		}, nil
	}}
	s := &OpenAIGatewayService{httpUpstream: upstream}

	_, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")

	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}
	if gotProfile != HTTPUpstreamProfileOpenAI {
		t.Fatalf("HTTP upstream profile: got %q, want %q", gotProfile, HTTPUpstreamProfileOpenAI)
	}
	if gotProxyURL != account.Proxy.URL() {
		t.Fatalf("proxy URL: got %q, want %q", gotProxyURL, account.Proxy.URL())
	}
	if gotAccountID != account.ID || gotConcurrency != account.Concurrency {
		t.Fatalf("account transport identity: got id=%d concurrency=%d", gotAccountID, gotConcurrency)
	}
	if hadDeadline {
		t.Fatal("models request unexpectedly installed a service-local deadline")
	}
}

func TestFetchCodexModelsManifestTransportErrorIsRetryable(t *testing.T) {
	upstream := &codexModelsTestHTTPUpstream{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	}}
	s := &OpenAIGatewayService{httpUpstream: upstream}

	_, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.144.0", "")

	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
	if !IsCodexModelsManifestRetryable(err) {
		t.Fatalf("expected retryable transport error, got %v", err)
	}
}

func TestFetchCodexModelsManifestCanceledRequestIsNotRetryable(t *testing.T) {
	upstream := &codexModelsTestHTTPUpstream{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return nil, context.Canceled
	}}
	s := &OpenAIGatewayService{httpUpstream: upstream}

	_, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.144.0", "")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if IsCodexModelsManifestRetryable(err) {
		t.Fatalf("canceled request must not be retryable: %v", err)
	}
}

func TestFetchCodexModelsManifestUsesTokenProvider(t *testing.T) {
	account := newCodexModelsTestAccount()
	account.Credentials["access_token"] = "stale-account-token"
	var gotAuthorization string
	upstream := &codexModelsTestHTTPUpstream{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		gotAuthorization = req.Header.Get("Authorization")
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
	}}
	s := &OpenAIGatewayService{
		httpUpstream:        upstream,
		openAITokenProvider: NewOpenAITokenProvider(nil, codexModelsTokenCache{token: "cached-provider-token"}, nil),
	}

	_, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")

	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}
	if gotAuthorization != "Bearer cached-provider-token" {
		t.Fatalf("authorization header: got %q", gotAuthorization)
	}
}

func TestFetchCodexModelsManifestUsesShadowCredentialsAndSelectedTransportIdentity(t *testing.T) {
	parent := newCodexModelsTestAccount()
	parent.ID = 100
	parent.Credentials["access_token"] = "parent-token"
	parent.Credentials["chatgpt_account_id"] = "parent-chatgpt-account"
	parentID := parent.ID
	shadow := newCodexModelsTestAccount()
	shadow.ID = 200
	shadow.ParentAccountID = &parentID
	shadow.Credentials = nil

	var gotAuthorization, gotChatGPTAccountID string
	var gotTransportAccountID int64
	upstream := &codexModelsTestHTTPUpstream{do: func(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
		gotAuthorization = req.Header.Get("Authorization")
		gotChatGPTAccountID = req.Header.Get("chatgpt-account-id")
		gotTransportAccountID = accountID
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
	}}
	s := &OpenAIGatewayService{accountRepo: newStubCredRepo(parent), httpUpstream: upstream}

	_, err := s.FetchCodexModelsManifest(context.Background(), shadow, "0.144.0", "")

	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}
	if gotAuthorization != "Bearer parent-token" || gotChatGPTAccountID != "parent-chatgpt-account" {
		t.Fatalf("shadow credentials not resolved: auth=%q account=%q", gotAuthorization, gotChatGPTAccountID)
	}
	if gotTransportAccountID != shadow.ID {
		t.Fatalf("transport account ID: got %d, want shadow ID %d", gotTransportAccountID, shadow.ID)
	}
}

func TestFetchCodexModelsManifestHTTPRetryability(t *testing.T) {
	tests := []struct {
		status    int
		retryable bool
	}{
		{status: http.StatusBadRequest, retryable: false},
		{status: http.StatusUnauthorized, retryable: true},
		{status: http.StatusForbidden, retryable: true},
		{status: http.StatusTooManyRequests, retryable: true},
		{status: http.StatusInternalServerError, retryable: true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			upstream := &codexModelsTestHTTPUpstream{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.status,
					Status:     http.StatusText(tt.status),
					Header:     make(http.Header),
					Body:       http.NoBody,
				}, nil
			}}
			s := &OpenAIGatewayService{httpUpstream: upstream}

			_, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.144.0", "")

			if err == nil {
				t.Fatal("expected upstream error, got nil")
			}
			if got := IsCodexModelsManifestRetryable(err); got != tt.retryable {
				t.Fatalf("retryable=%v, want %v: %v", got, tt.retryable, err)
			}
		})
	}
}

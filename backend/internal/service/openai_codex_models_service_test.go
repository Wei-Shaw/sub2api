package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

type codexModelsHTTPUpstreamStub struct {
	do func(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error)
}

type codexModelsBlockingBody struct {
	ctx         context.Context
	readStarted chan struct{}
	startedOnce *sync.Once
	release     <-chan struct{}
	body        *strings.Reader
}

func (b *codexModelsBlockingBody) Read(p []byte) (int, error) {
	b.startedOnce.Do(func() { close(b.readStarted) })
	select {
	case <-b.release:
		return b.body.Read(p)
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
}

func (b *codexModelsBlockingBody) Close() error { return nil }

func (s *codexModelsHTTPUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return s.do(req, proxyURL, accountID, accountConcurrency)
}

func (s *codexModelsHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestIsRetryableCodexModelsManifestTransportError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{name: "nil", err: nil},
		{name: "configuration error", err: errors.New("invalid proxy URL")},
		{name: "upstream configuration error", err: errors.New("upstream error: invalid proxy")},
		{name: "proxy connection configuration error", err: errors.New("proxy connection error: invalid configuration")},
		{name: "canceled request", err: context.Canceled},
		{
			name: "redirect policy error",
			err: &url.Error{
				Op:  "Get",
				URL: "https://upstream.example/v1/models",
				Err: errors.New("stopped after 10 redirects"),
			},
		},
		{name: "deadline exceeded", err: context.DeadlineExceeded, retryable: true},
		{name: "unexpected EOF", err: io.ErrUnexpectedEOF, retryable: true},
		{name: "closed connection", err: net.ErrClosed, retryable: true},
		{
			name: "network operation",
			err: &net.OpError{
				Op:  "read",
				Net: "tcp",
				Err: errors.New("connection reset"),
			},
			retryable: true,
		},
		{
			name:      "DNS error",
			err:       &net.DNSError{Err: "temporary failure", Name: "upstream.example"},
			retryable: true,
		},
		{
			name:      "typed HTTP2 GOAWAY",
			err:       http2.GoAwayError{ErrCode: http2.ErrCodeNo},
			retryable: true,
		},
		{
			name:      "stdlib HTTP2 GOAWAY",
			err:       errors.New("http2: server sent GOAWAY and closed the connection; LastStreamID=1, ErrCode=NO_ERROR"),
			retryable: true,
		},
		{
			name:      "stdlib HTTP2 refused stream",
			err:       errors.New("stream error: stream ID 3; REFUSED_STREAM"),
			retryable: true,
		},
		{
			name:      "stdlib HTTP2 connection error",
			err:       errors.New(`Get "https://upstream.example/v1/models": connection error: PROTOCOL_ERROR`),
			retryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableCodexModelsManifestTransportError(tt.err); got != tt.retryable {
				t.Fatalf("retryable = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func newCodexModelsAPIKeyTestService(upstream HTTPUpstream) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled: false,
		}}},
		httpUpstream: upstream,
	}
}

func newCodexModelsAPIKeyTestAccount(baseURL string) *Account {
	credentials := map[string]any{"api_key": "sk-upstream"}
	if baseURL != "" {
		credentials["base_url"] = baseURL
	}
	return &Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: credentials,
		Concurrency: 3,
	}
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

func TestFetchCodexModelsManifestAgentIdentityUsesAssertionWithoutOAuthToken(t *testing.T) {
	key, privateKey := newTestAgentIdentityKey(t)
	account := &Account{
		ID:       3,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode":          OpenAIAuthModeAgentIdentity,
			"agent_runtime_id":   key.runtimeID,
			"agent_private_key":  privateKey,
			"task_id":            key.taskID,
			"chatgpt_account_id": "acc-agent",
		},
	}

	var gotAuth, gotAccountID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccountID = r.Header.Get("chatgpt-account-id")
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	s := &OpenAIGatewayService{}
	manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}
	if string(manifest.Body) != `{"models":[]}` {
		t.Fatalf("unexpected manifest body: %q", manifest.Body)
	}
	if !strings.HasPrefix(gotAuth, "AgentAssertion ") {
		t.Fatalf("authorization scheme: got %q", strings.SplitN(gotAuth, " ", 2)[0])
	}
	if gotAccountID != "acc-agent" {
		t.Fatalf("chatgpt-account-id header: got %q", gotAccountID)
	}
}

func TestFetchCodexModelsManifestAgentIdentityRecoversInvalidTaskOnce(t *testing.T) {
	key, privateKey := newTestAgentIdentityKey(t)
	account := &Account{
		ID:       4,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode":          OpenAIAuthModeAgentIdentity,
			"agent_runtime_id":   key.runtimeID,
			"agent_private_key":  privateKey,
			"task_id":            "task-models-old",
			"chatgpt_account_id": "acc-agent-recovery",
		},
	}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	modelsCalls := 0
	registerCalls := 0
	var assertions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		if strings.Contains(r.URL.Path, "/task/register") {
			registerCalls++
			_, _ = w.Write([]byte(`{"task_id":"task-models-new"}`))
			return
		}
		modelsCalls++
		assertions = append(assertions, r.Header.Get("Authorization"))
		if modelsCalls == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_task_id"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer server.Close()

	originalModelsURL := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	t.Cleanup(func() { chatgptCodexModelsURL = originalModelsURL })
	originalAuthBase := openAIAgentIdentityAuthAPIBaseURL
	openAIAgentIdentityAuthAPIBaseURL = server.URL
	t.Cleanup(func() { openAIAgentIdentityAuthAPIBaseURL = originalAuthBase })

	s := &OpenAIGatewayService{accountRepo: repo}
	manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")
	require.NoError(t, err)
	require.Equal(t, `{"models":[]}`, string(manifest.Body))
	require.Equal(t, 2, modelsCalls)
	require.Equal(t, 1, registerCalls)
	require.Len(t, assertions, 2)
	require.Equal(t, "task-models-old", decodeAgentAssertionTask(t, assertions[0]))
	require.Equal(t, "task-models-new", decodeAgentAssertionTask(t, assertions[1]))
}

func TestFetchCodexModelsManifestAgentIdentityRedactsUpstreamErrors(t *testing.T) {
	key, privateKey := newTestAgentIdentityKey(t)
	account := &Account{
		ID:       5,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode":          OpenAIAuthModeAgentIdentity,
			"agent_runtime_id":   key.runtimeID,
			"agent_private_key":  privateKey,
			"task_id":            key.taskID,
			"chatgpt_account_id": "acc-agent-redaction",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"error":"%s %s %s AgentAssertion leaked"}`, key.runtimeID, key.taskID, privateKey)
	}))
	defer server.Close()
	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	t.Cleanup(func() { chatgptCodexModelsURL = original })

	s := &OpenAIGatewayService{}
	_, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")
	require.Error(t, err)
	require.NotContains(t, err.Error(), key.runtimeID)
	require.NotContains(t, err.Error(), key.taskID)
	require.NotContains(t, err.Error(), privateKey)
	require.NotContains(t, err.Error(), "AgentAssertion leaked")
	require.Contains(t, err.Error(), "[redacted]")
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

func TestFetchCodexModelsManifestAPIKeyCustomUpstream(t *testing.T) {
	manifestBody := `{"models":[{"slug":"gpt-5.6"}]}`
	var gotRequest *http.Request
	var gotProxyURL string
	var gotAccountID int64
	var gotConcurrency int
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
		gotRequest = req
		gotProxyURL = proxyURL
		gotAccountID = accountID
		gotConcurrency = accountConcurrency
		header := make(http.Header)
		header.Set("ETag", `W/"api-key-manifest"`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(manifestBody)),
		}, nil
	}}

	s := newCodexModelsAPIKeyTestService(upstream)
	manifest, err := s.FetchCodexModelsManifest(
		context.Background(),
		newCodexModelsAPIKeyTestAccount("https://upstream.example/v1"),
		"0.144.0",
		"",
	)
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}

	if gotRequest == nil {
		t.Fatal("expected request to custom API key upstream")
	}
	if gotRequest.Method != http.MethodGet {
		t.Errorf("method: got %q", gotRequest.Method)
	}
	if gotRequest.URL.String() != "https://upstream.example/v1/models?client_version=0.144.0" {
		t.Errorf("request URL: got %q", gotRequest.URL.String())
	}
	if gotRequest.Header.Get("Authorization") != "Bearer sk-upstream" {
		t.Errorf("authorization header: got %q", gotRequest.Header.Get("Authorization"))
	}
	if gotRequest.Header.Get("Originator") != "codex_cli_rs" {
		t.Errorf("originator header: got %q", gotRequest.Header.Get("Originator"))
	}
	if gotRequest.Header.Get("Version") != "0.144.0" {
		t.Errorf("version header: got %q", gotRequest.Header.Get("Version"))
	}
	if gotRequest.Header.Get("User-Agent") != codexCLIUserAgent {
		t.Errorf("user-agent header: got %q", gotRequest.Header.Get("User-Agent"))
	}
	if gotRequest.Header.Get("chatgpt-account-id") != "" {
		t.Errorf("chatgpt-account-id must not be sent to API key upstream: got %q", gotRequest.Header.Get("chatgpt-account-id"))
	}
	if gotProxyURL != "" || gotAccountID != 2 || gotConcurrency != 3 {
		t.Errorf("upstream routing metadata: proxy=%q account_id=%d concurrency=%d", gotProxyURL, gotAccountID, gotConcurrency)
	}
	if string(manifest.Body) != manifestBody {
		t.Errorf("body not passed through verbatim: got %q", manifest.Body)
	}
	if manifest.ETag != `W/"api-key-manifest"` {
		t.Errorf("etag not passed through: got %q", manifest.ETag)
	}
}

func TestFetchCodexModelsManifestAPIKeyConvertsStandardOpenAIModelList(t *testing.T) {
	upstreamBody := `{"object":"list","data":[{"id":"gpt-5.6","object":"model"},{"id":"  ","object":"model"},{"id":"gpt-5.6-codex","object":"model"}]}`
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		header := make(http.Header)
		header.Set("ETag", `W/"openai-list"`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(upstreamBody)),
		}, nil
	}}

	s := newCodexModelsAPIKeyTestService(upstream)
	manifest, err := s.FetchCodexModelsManifest(
		context.Background(),
		newCodexModelsAPIKeyTestAccount("https://upstream.example/v1"),
		"0.144.0",
		"",
	)
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}
	if got, want := string(manifest.Body), `{"models":[{"slug":"gpt-5.6"},{"slug":"gpt-5.6-codex"}]}`; got != want {
		t.Errorf("converted body: got %q, want %q", got, want)
	}
	if manifest.ETag != `W/"openai-list"` {
		t.Errorf("etag not passed through: got %q", manifest.ETag)
	}
}

func TestConvertOpenAIModelListToCodexManifest(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "standard list",
			body: `{"object":"list","data":[{"id":"m-1"},{"id":"m-2"}]}`,
			want: `{"models":[{"slug":"m-1"},{"slug":"m-2"}]}`,
		},
		{
			name: "codex manifest unchanged",
			body: `{"models":[{"slug":"m-1"}]}`,
			want: `{"models":[{"slug":"m-1"}]}`,
		},
		{
			name: "empty data unchanged",
			body: `{"object":"list","data":[]}`,
			want: `{"object":"list","data":[]}`,
		},
		{
			name: "data not an array unchanged",
			body: `{"object":"list","data":{"id":"m-1"}}`,
			want: `{"object":"list","data":{"id":"m-1"}}`,
		},
		{
			name: "entries without usable IDs unchanged",
			body: `{"object":"list","data":[{"id":""},{"object":"model"}]}`,
			want: `{"object":"list","data":[{"id":""},{"object":"model"}]}`,
		},
		{
			name: "invalid JSON unchanged",
			body: `{"data":`,
			want: `{"data":`,
		},
		{
			name: "non-object unchanged",
			body: `[]`,
			want: `[]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(convertOpenAIModelListToCodexManifest([]byte(tt.body))); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFetchCodexModelsManifestRejectsInvalidEnvelope(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "OpenAI models list", body: `{"object":"list","data":[]}`},
		{name: "invalid JSON", body: `{"models":`},
		{name: "non-object", body: `[]`},
		{name: "null object", body: `null`},
		{name: "missing models", body: `{}`},
		{name: "models object", body: `{"models":{}}`},
		{name: "models null", body: `{"models":null}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(tt.body)),
				}, nil
			}}

			s := newCodexModelsAPIKeyTestService(upstream)
			_, err := s.FetchCodexModelsManifest(
				context.Background(),
				newCodexModelsAPIKeyTestAccount("https://upstream.example"),
				"0.144.0",
				"",
			)
			if err == nil {
				t.Fatal("expected invalid manifest error, got nil")
			}
			if infraerrors.Reason(err) != "OPENAI_CODEX_MODELS_UPSTREAM_INVALID_MANIFEST" {
				t.Errorf("error reason: got %q", infraerrors.Reason(err))
			}
			if !IsRetryableCodexModelsManifestError(err) {
				t.Error("invalid upstream manifest must be retryable")
			}
		})
	}
}

func TestFetchCodexModelsManifestAPIKeyDoesNotCacheInvalidEnvelope(t *testing.T) {
	var calls atomic.Int32
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		body := `{"object":"list","data":[]}`
		if calls.Add(1) > 1 {
			body = `{"models":[{"slug":"gpt-5.6"}]}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}}

	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", ""); err == nil {
		t.Fatal("expected invalid manifest error on first fetch")
	}
	manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
	if err != nil {
		t.Fatalf("second fetch returned error: %v", err)
	}
	if got, want := string(manifest.Body), `{"models":[{"slug":"gpt-5.6"}]}`; got != want {
		t.Errorf("body: got %q, want %q", got, want)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream calls: got %d, want 2", got)
	}
}

func TestFetchCodexModelsManifestAPIKeySharedRefreshSurvivesCallerCancellation(t *testing.T) {
	const manifestBody = `{"models":[{"slug":"gpt-5.6"}]}`
	var calls atomic.Int32
	var readStartedOnce sync.Once
	readStarted := make(chan struct{})
	deadlineRemaining := make(chan time.Duration, 1)
	release := make(chan struct{})
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		deadline, ok := req.Context().Deadline()
		if !ok {
			deadlineRemaining <- 0
		} else {
			deadlineRemaining <- time.Until(deadline)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Etag": []string{`W/"shared"`}},
			Body: &codexModelsBlockingBody{
				ctx:         req.Context(),
				readStarted: readStarted,
				startedOnce: &readStartedOnce,
				release:     release,
				body:        strings.NewReader(manifestBody),
			},
		}, nil
	}}

	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstErr := make(chan error, 1)
	go func() {
		_, err := s.FetchCodexModelsManifest(firstCtx, account, "0.144.0", "")
		firstErr <- err
	}()

	select {
	case <-readStarted:
	case <-time.After(time.Second):
		t.Fatal("upstream body read did not start")
	}
	remaining := <-deadlineRemaining
	if remaining < 14*time.Second || remaining > codexModelsManifestRequestTimeout {
		t.Errorf("detached refresh deadline: got %s, want approximately %s", remaining, codexModelsManifestRequestTimeout)
	}
	cancelFirst()
	select {
	case err := <-firstErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first caller error: got %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled caller did not return promptly")
	}

	secondResult := make(chan struct {
		manifest *CodexModelsManifest
		err      error
	}, 1)
	go func() {
		manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
		secondResult <- struct {
			manifest *CodexModelsManifest
			err      error
		}{manifest: manifest, err: err}
	}()

	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("upstream calls before shared refresh completed: got %d, want 1", got)
	}
	close(release)
	select {
	case result := <-secondResult:
		if result.err != nil {
			t.Fatalf("second caller returned error: %v", result.err)
		}
		if string(result.manifest.Body) != manifestBody {
			t.Errorf("second caller body: got %q", result.manifest.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("second caller did not receive shared refresh result")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("total upstream calls: got %d, want 1", got)
	}
}

func TestFetchCodexModelsManifestAPIKeyConcurrentRequestsShareRefresh(t *testing.T) {
	const callers = 8
	var calls atomic.Int32
	started := make(chan struct{})
	var startedOnce sync.Once
	release := make(chan struct{})
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		startedOnce.Do(func() { close(started) })
		<-release
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"models":[]}`)),
		}, nil
	}}

	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	begin := make(chan struct{})
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			<-begin
			_, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
			errs <- err
		}()
	}
	close(begin)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upstream request did not start")
	}
	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("concurrent upstream calls: got %d, want 1", got)
	}
	close(release)
	for i := 0; i < callers; i++ {
		if err := <-errs; err != nil {
			t.Errorf("caller %d returned error: %v", i, err)
		}
	}
}

func TestFetchCodexModelsManifestAPIKeyFreshCacheHandlesETagLocally(t *testing.T) {
	var calls atomic.Int32
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		if got := req.Header.Get("If-None-Match"); got != "" {
			t.Errorf("cache refresh must not inherit a caller's If-None-Match: got %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Etag": []string{`W/"cached"`}},
			Body:       io.NopCloser(strings.NewReader(`{"models":[]}`)),
		}, nil
	}}

	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", ""); err != nil {
		t.Fatalf("initial fetch returned error: %v", err)
	}
	manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", `W/"cached"`)
	if err != nil {
		t.Fatalf("cached fetch returned error: %v", err)
	}
	if !manifest.NotModified {
		t.Fatal("matching cached ETag must return NotModified")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("upstream calls: got %d, want 1", got)
	}
}

func TestFetchCodexModelsManifestAPIKeyCacheKeyIsolatesRequestIdentity(t *testing.T) {
	var calls atomic.Int32
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"models":[]}`)),
		}, nil
	}}
	s := newCodexModelsAPIKeyTestService(upstream)

	base := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	fetch := func(account *Account, version string) {
		t.Helper()
		if _, err := s.FetchCodexModelsManifest(context.Background(), account, version, ""); err != nil {
			t.Fatalf("fetch returned error: %v", err)
		}
	}
	fetch(base, "0.144.0")
	fetch(base, "0.144.0")

	differentAccount := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	differentAccount.ID = 3
	fetch(differentAccount, "0.144.0")

	differentToken := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	differentToken.Credentials["api_key"] = "sk-other"
	fetch(differentToken, "0.144.0")

	differentUpstream := newCodexModelsAPIKeyTestAccount("https://other-upstream.example")
	fetch(differentUpstream, "0.144.0")
	fetch(base, "0.145.0")

	differentHeaders := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	differentHeaders.Credentials[credKeyHeaderOverrideEnabled] = true
	differentHeaders.Credentials[credKeyHeaderOverrides] = map[string]any{"x-tenant": "other"}
	fetch(differentHeaders, "0.144.0")

	proxyID := int64(9)
	differentProxy := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	differentProxy.ProxyID = &proxyID
	differentProxy.Proxy = &Proxy{Protocol: "http", Host: "127.0.0.1", Port: 8080}
	fetch(differentProxy, "0.144.0")
	fetch(differentProxy, "0.144.0")

	if got := calls.Load(); got != 7 {
		t.Errorf("isolated upstream calls: got %d, want 7", got)
	}
}

func TestFetchCodexModelsManifestAPIKeyCacheBoundsEntriesAndBodySize(t *testing.T) {
	var calls atomic.Int32
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		body := `{"models":[]}`
		if strings.Contains(req.URL.Host, "large") {
			body = `{"models":[],"padding":"` + strings.Repeat("x", (1<<20)+1) + `"}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}}
	s := newCodexModelsAPIKeyTestService(upstream)
	fetch := func(account *Account) {
		t.Helper()
		if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", ""); err != nil {
			t.Fatalf("fetch returned error: %v", err)
		}
	}

	small := newCodexModelsAPIKeyTestAccount("https://small.example")
	fetch(small)
	fetch(small)
	large := newCodexModelsAPIKeyTestAccount("https://large.example")
	large.ID = 3
	fetch(large)
	fetch(large)
	if got := calls.Load(); got != 3 {
		t.Fatalf("body-size bounded cache calls: got %d, want 3", got)
	}

	for i := int64(10); i < 75; i++ {
		account := newCodexModelsAPIKeyTestAccount("https://bounded.example")
		account.ID = i
		fetch(account)
	}
	last := newCodexModelsAPIKeyTestAccount("https://bounded.example")
	last.ID = 74
	fetch(last)
	if got := calls.Load(); got != 68 {
		t.Fatalf("most recent cache entry was not retained: calls=%d, want 68", got)
	}
	first := newCodexModelsAPIKeyTestAccount("https://bounded.example")
	first.ID = 10
	fetch(first)
	if got := calls.Load(); got != 69 {
		t.Errorf("oldest cache entry was not evicted: calls=%d, want 69", got)
	}
}

func TestFetchCodexModelsManifestAPIKeyServesStaleWhileRefreshing(t *testing.T) {
	var calls atomic.Int32
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		call := calls.Add(1)
		body := `{"models":[{"slug":"old"}]}`
		if call > 1 {
			if call == 2 {
				close(refreshStarted)
			}
			<-releaseRefresh
			body = `{"models":[{"slug":"new"}]}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}}
	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", ""); err != nil {
		t.Fatalf("initial fetch returned error: %v", err)
	}

	s.codexModelsManifestCache.mu.Lock()
	for key, entry := range s.codexModelsManifestCache.entries {
		entry.expiresAt = time.Now().Add(-time.Second)
		s.codexModelsManifestCache.entries[key] = entry
	}
	s.codexModelsManifestCache.mu.Unlock()

	resultCh := make(chan struct {
		manifest *CodexModelsManifest
		err      error
	}, 1)
	go func() {
		manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
		resultCh <- struct {
			manifest *CodexModelsManifest
			err      error
		}{manifest: manifest, err: err}
	}()
	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}

	var staleResult struct {
		manifest *CodexModelsManifest
		err      error
	}
	select {
	case staleResult = <-resultCh:
	case <-time.After(100 * time.Millisecond):
		t.Error("stale manifest was not returned while refresh was blocked")
		close(releaseRefresh)
		staleResult = <-resultCh
	}
	if staleResult.err != nil {
		t.Fatalf("stale fetch returned error: %v", staleResult.err)
	}
	if got := string(staleResult.manifest.Body); got != `{"models":[{"slug":"old"}]}` {
		t.Errorf("stale body: got %q", got)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream calls during stale refresh: got %d, want 2", got)
	}

	select {
	case <-releaseRefresh:
	default:
		close(releaseRefresh)
	}
	deadline := time.Now().Add(time.Second)
	for {
		manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
		if err == nil && string(manifest.Body) == `{"models":[{"slug":"new"}]}` {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("refreshed manifest was not cached: manifest=%v err=%v", manifest, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("stale refresh was not deduplicated: calls=%d, want 2", got)
	}
}

func TestFetchCodexModelsManifestAPIKeyRevalidatesStaleETag(t *testing.T) {
	var calls atomic.Int32
	refreshDone := make(chan struct{})
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		call := calls.Add(1)
		if call == 1 {
			header := make(http.Header)
			header.Set("ETag", `W/"cached"`)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(`{"models":[{"slug":"cached"}]}`)),
			}, nil
		}
		if got := req.Header.Get("If-None-Match"); got != `W/"cached"` {
			t.Errorf("background revalidation If-None-Match: got %q", got)
		}
		close(refreshDone)
		header := make(http.Header)
		header.Set("ETag", `W/"cached"`)
		return &http.Response{StatusCode: http.StatusNotModified, Header: header, Body: http.NoBody}, nil
	}}
	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", ""); err != nil {
		t.Fatalf("initial fetch returned error: %v", err)
	}
	s.codexModelsManifestCache.mu.Lock()
	for key, entry := range s.codexModelsManifestCache.entries {
		entry.expiresAt = time.Now().Add(-time.Second)
		s.codexModelsManifestCache.entries[key] = entry
	}
	s.codexModelsManifestCache.mu.Unlock()

	manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
	if err != nil {
		t.Fatalf("stale fetch returned error: %v", err)
	}
	if got := string(manifest.Body); got != `{"models":[{"slug":"cached"}]}` {
		t.Fatalf("stale body: got %q", got)
	}
	select {
	case <-refreshDone:
	case <-time.After(time.Second):
		t.Fatal("ETag revalidation did not complete")
	}

	deadline := time.Now().Add(time.Second)
	for {
		s.codexModelsManifestCache.mu.Lock()
		fresh := false
		for _, entry := range s.codexModelsManifestCache.entries {
			fresh = time.Now().Before(entry.expiresAt)
		}
		s.codexModelsManifestCache.mu.Unlock()
		if fresh {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("304 revalidation did not renew the cached manifest")
		}
		time.Sleep(10 * time.Millisecond)
	}
	manifest, err = s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
	if err != nil || string(manifest.Body) != `{"models":[{"slug":"cached"}]}` {
		t.Fatalf("renewed cached manifest: body=%q err=%v", manifest.Body, err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream calls: got %d, want 2", got)
	}
}

func TestFetchCodexModelsManifestAPIKeyColdCacheHandlesNotModifiedLocally(t *testing.T) {
	var gotIfNoneMatch string
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		gotIfNoneMatch = req.Header.Get("If-None-Match")
		header := make(http.Header)
		header.Set("ETag", `W/"api-key-manifest"`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(`{"models":[]}`)),
		}, nil
	}}

	s := newCodexModelsAPIKeyTestService(upstream)
	manifest, err := s.FetchCodexModelsManifest(
		context.Background(),
		newCodexModelsAPIKeyTestAccount("https://upstream.example"),
		"0.144.0",
		`W/"api-key-manifest"`,
	)
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}
	if !manifest.NotModified {
		t.Error("expected NotModified to be true")
	}
	if manifest.ETag != `W/"api-key-manifest"` {
		t.Errorf("etag not passed through: got %q", manifest.ETag)
	}
	if gotIfNoneMatch != "" {
		t.Errorf("cold shared refresh must not inherit caller if-none-match: got %q", gotIfNoneMatch)
	}
}

func TestFetchCodexModelsManifestAPIKeyDoesNotCacheUnexpectedColdNotModified(t *testing.T) {
	var calls atomic.Int32
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		if got := req.Header.Get("If-None-Match"); got != "" {
			t.Errorf("cold shared refresh If-None-Match: got %q", got)
		}
		header := make(http.Header)
		header.Set("ETag", `W/"unexpected"`)
		return &http.Response{StatusCode: http.StatusNotModified, Header: header, Body: http.NoBody}, nil
	}}
	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	for i := 0; i < 2; i++ {
		manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
		if err != nil {
			t.Fatalf("fetch %d returned error: %v", i, err)
		}
		if !manifest.NotModified {
			t.Fatalf("fetch %d: expected upstream NotModified response", i)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("unexpected cold 304 was cached: upstream calls=%d, want 2", got)
	}
}

func TestFetchCodexModelsManifestAPIKeyPreservesBaseURLQuery(t *testing.T) {
	var gotURL string
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		gotURL = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"models":[]}`)),
		}, nil
	}}

	s := newCodexModelsAPIKeyTestService(upstream)
	_, err := s.FetchCodexModelsManifest(
		context.Background(),
		newCodexModelsAPIKeyTestAccount("https://upstream.example/v1?tenant=acme"),
		"0.144.0",
		"",
	)
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}
	if gotURL != "https://upstream.example/v1/models?client_version=0.144.0&tenant=acme" {
		t.Errorf("request URL: got %q", gotURL)
	}
}

func TestFetchCodexModelsManifestAPIKeyRejectsBaseURLFragment(t *testing.T) {
	called := false
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		called = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"models":[]}`)),
		}, nil
	}}

	s := newCodexModelsAPIKeyTestService(upstream)
	_, err := s.FetchCodexModelsManifest(
		context.Background(),
		newCodexModelsAPIKeyTestAccount("https://upstream.example/v1#models"),
		"0.144.0",
		"",
	)
	if err == nil {
		t.Fatal("expected invalid upstream base URL error, got nil")
	}
	if infraerrors.Reason(err) != "OPENAI_CODEX_MODELS_API_KEY_UPSTREAM_INVALID" {
		t.Errorf("error reason: got %q", infraerrors.Reason(err))
	}
	if called {
		t.Fatal("fragment-bearing base URL must be rejected before the upstream request")
	}
}

// codexModelsAccountStateRepo records account state transitions triggered by
// manifest upstream errors (#4544).
type codexModelsAccountStateRepo struct {
	AccountRepository
	mu                  sync.Mutex
	setErrorCalls       int
	lastErrorMsg        string
	setTempUnschedCalls int
	lastTempReason      string
}

func (r *codexModelsAccountStateRepo) SetError(_ context.Context, _ int64, errorMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setErrorCalls++
	r.lastErrorMsg = errorMsg
	return nil
}

func (r *codexModelsAccountStateRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setTempUnschedCalls++
	r.lastTempReason = reason
	return nil
}

func newCodexModels401TestService(repo AccountRepository) *OpenAIGatewayService {
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	s := &OpenAIGatewayService{rateLimitService: rateLimitService}
	rateLimitService.SetAccountRuntimeBlocker(s)
	return s
}

func TestFetchCodexModelsManifestOAuth401MarksAccountUnschedulable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":{"message":"invalid token"}}`))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	repo := &codexModelsAccountStateRepo{}
	s := newCodexModels401TestService(repo)
	account := newCodexModelsTestAccount()
	account.Credentials["refresh_token"] = "test-refresh-token"

	_, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")
	require.Error(t, err)
	require.True(t, IsRetryableCodexModelsManifestError(err), "manifest 401 should allow account failover")
	require.Equal(t, 1, repo.setTempUnschedCalls, "OAuth 401 should temp-unschedule the account")
	require.Equal(t, 0, repo.setErrorCalls)
	require.True(t, s.isOpenAIAccountRuntimeBlocked(account), "account should be runtime-blocked after manifest 401")
}

func TestFetchCodexModelsManifestOAuth401TokenRevokedDisablesAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"token_revoked","message":"token has been revoked"}}`))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	repo := &codexModelsAccountStateRepo{}
	s := newCodexModels401TestService(repo)
	account := newCodexModelsTestAccount()
	account.Credentials["refresh_token"] = "test-refresh-token"

	_, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")
	require.Error(t, err)
	require.True(t, IsRetryableCodexModelsManifestError(err))
	require.Equal(t, 1, repo.setErrorCalls, "revoked token should permanently disable the account")
	require.Contains(t, repo.lastErrorMsg, "Token revoked")
	require.Equal(t, 0, repo.setTempUnschedCalls)
}

func TestFetchCodexModelsManifestAgentIdentity401DoesNotDisableAccount(t *testing.T) {
	key, privateKey := newTestAgentIdentityKey(t)
	account := &Account{
		ID:       6,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode":          OpenAIAuthModeAgentIdentity,
			"agent_runtime_id":   key.runtimeID,
			"agent_private_key":  privateKey,
			"task_id":            key.taskID,
			"chatgpt_account_id": "acc-agent-401",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"some non-task 401"}`))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	repo := &codexModelsAccountStateRepo{}
	s := newCodexModels401TestService(repo)

	_, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")
	require.Error(t, err)
	require.Equal(t, 0, repo.setErrorCalls, "agent identity 401s must not disable the account")
	require.Equal(t, 0, repo.setTempUnschedCalls)
}

func TestFetchCodexModelsManifestAPIKey401KeepsNoFailoverAndNoDisable(t *testing.T) {
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Status:     "401 Unauthorized",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":"invalid api key"}`)),
		}, nil
	}}

	repo := &codexModelsAccountStateRepo{}
	s := newCodexModelsAPIKeyTestService(upstream)
	s.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	_, err := s.FetchCodexModelsManifest(
		context.Background(),
		newCodexModelsAPIKeyTestAccount("https://upstream.example"),
		"0.144.0",
		"",
	)
	require.Error(t, err)
	require.False(t, IsRetryableCodexModelsManifestError(err), "custom upstream manifest 401 keeps the no-failover behavior")
	require.Equal(t, 0, repo.setErrorCalls, "custom upstream manifest 401 must not disable the account")
	require.Equal(t, 0, repo.setTempUnschedCalls)
}

func TestFetchCodexModelsManifestAPIKeyUpstreamError(t *testing.T) {
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Status:     "429 Too Many Requests",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":"rate limited"}`)),
		}, nil
	}}

	s := newCodexModelsAPIKeyTestService(upstream)
	_, err := s.FetchCodexModelsManifest(
		context.Background(),
		newCodexModelsAPIKeyTestAccount("https://upstream.example"),
		"0.144.0",
		"",
	)
	if err == nil {
		t.Fatal("expected error for upstream 429, got nil")
	}
	if infraerrors.Code(err) != http.StatusBadGateway {
		t.Errorf("error status: got %d, want %d", infraerrors.Code(err), http.StatusBadGateway)
	}
	if infraerrors.Reason(err) != "OPENAI_CODEX_MODELS_UPSTREAM_FAILED" {
		t.Errorf("error reason: got %q", infraerrors.Reason(err))
	}
}

func TestFetchCodexModelsManifestAPIKeyRejectsOfficialOpenAIBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "missing base URL"},
		{name: "official host", baseURL: "https://api.openai.com"},
		{name: "official versioned URL", baseURL: "https://API.OPENAI.COM:443/v1/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
				t.Fatal("official OpenAI API key must not be used as a Codex manifest upstream")
				return nil, nil
			}})

			_, err := s.FetchCodexModelsManifest(
				context.Background(),
				newCodexModelsAPIKeyTestAccount(tt.baseURL),
				"0.144.0",
				"",
			)
			if err == nil {
				t.Fatal("expected unsupported API key upstream error, got nil")
			}
			if infraerrors.Reason(err) != "OPENAI_CODEX_MODELS_API_KEY_UPSTREAM_UNSUPPORTED" {
				t.Errorf("error reason: got %q", infraerrors.Reason(err))
			}
		})
	}
}

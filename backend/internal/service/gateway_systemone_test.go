package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type systemOneHTTPUpstream struct {
	do func(*http.Request) (*http.Response, error)
}

type systemOnePolicyAccountRepo struct {
	AccountRepository
	account          *Account
	rateLimitedCalls int
	overloadedCalls  int
	errorCalls       int
}

func (r *systemOnePolicyAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *systemOnePolicyAccountRepo) SetRateLimited(context.Context, int64, time.Time) error {
	r.rateLimitedCalls++
	return nil
}

func (r *systemOnePolicyAccountRepo) SetOverloaded(context.Context, int64, time.Time) error {
	r.overloadedCalls++
	return nil
}

func (r *systemOnePolicyAccountRepo) SetError(context.Context, int64, string) error {
	r.errorCalls++
	return nil
}

func (u *systemOneHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.do(req)
}

func (u *systemOneHTTPUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.do(req)
}

func newSystemOneTestService(upstream HTTPUpstream) *GatewayService {
	return &GatewayService{
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{AllowInsecureHTTP: true}}},
		httpUpstream: upstream,
	}
}

func newSystemOneTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)
	return c
}

func TestForwardSystemOneForwardsNativeProtocolAndUsage(t *testing.T) {
	requestBody := []byte(`{"model":"jev-latest","state":{"text":"sample"},"questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`)
	responseBody := []byte(`{"model":"jev-1.13.0","answers":{"q":{"type":"choice","choice":"a"}},"usage":{"input_tokens":123,"output_tokens":7},"provider_extension":{"kept":true}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, typesafeSystemOnePathForTest, r.URL.Path)
		require.Equal(t, "Bearer ts-secret", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		got, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, requestBody, got)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("x-request-id", "req-jev")
		_, err = w.Write(responseBody)
		require.NoError(t, err)
	}))
	defer server.Close()

	svc := newSystemOneTestService(&systemOneHTTPUpstream{do: server.Client().Do})
	account := &Account{ID: 7, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "ts-secret"}}
	result, err := svc.ForwardSystemOne(context.Background(), newSystemOneTestContext(), account, requestBody)
	require.NoError(t, err)
	require.Equal(t, responseBody, result.Body)
	require.Equal(t, http.StatusOK, result.StatusCode)
	require.Equal(t, "application/json; charset=utf-8", result.ContentType)
	require.Equal(t, "req-jev", result.RequestID)
	require.Equal(t, "jev-latest", result.Model)
	require.Equal(t, "jev-1.13.0", result.UpstreamResponseModel)
	require.Equal(t, 123, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
}

func TestForwardSystemOneAllowsSuccessfulResponseWithoutModel(t *testing.T) {
	responseBody := []byte(`{"answers":{"q":{"type":"noul","answer":"ok"}},"usage":{"input_tokens":12}}`)
	upstream := &systemOneHTTPUpstream{do: func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(responseBody)))}, nil
	}}
	svc := newSystemOneTestService(upstream)
	account := &Account{ID: 11, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "http://typesafe.test", "api_key": "ts-secret"}}

	result, err := svc.ForwardSystemOne(context.Background(), newSystemOneTestContext(), account, []byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, responseBody, result.Body)
	require.Empty(t, result.UpstreamResponseModel)
	require.Equal(t, 12, result.Usage.InputTokens)
}

const typesafeSystemOnePathForTest = "/v1/systemone"

func TestForwardSystemOneSchemaConformancePassthrough(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"omitted noul instructions", `{"model":"jev-latest","state":"sample","questions":{"q":{"type":"noul","extension":{"kept":true}}}}`},
		{"nullable noul fields", `{"model":"jev-latest","state":"sample","questions":{"q":{"type":"noul","instructions":null,"criteria":null}}}`},
		{"structured noul descriptions", `{"model":"jev-latest","state":"sample","questions":{"q":{"type":"noul","criteria":{"true":{"reason":"Yes"},"false":["No",null]}}}}`},
		{"object choice description", `{"model":"jev-latest","state":"sample","questions":{"q":{"type":"choice","criteria":{"a":{"description":"A","extra":null}}}}}`},
		{"array choice description", `{"model":"jev-latest","state":"sample","questions":{"q":{"type":"choice","instructions":null,"criteria":{"a":["A",null],"b":null}}}}`},
		{"empty choice criteria", `{"model":"jev-latest","state":"sample","questions":{"q":{"type":"choice","criteria":{}}}}`},
		{"one-level score", `{"model":"jev-latest","state":"sample","questions":{"q":{"type":"score","criteria":["only"]}}}`},
		{"object score level", `{"model":"jev-latest","state":"sample","questions":{"q":{"type":"score","instructions":null,"criteria":[{"description":"only","extra":null}]}}}`},
		{"array score level", `{"model":"jev-latest","state":"sample","questions":{"q":{"type":"score","criteria":[["only",null]]}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestBody := []byte(tc.body)
			_, err := typesafe.ValidateSystemOneRequest(requestBody)
			require.NoError(t, err)
			responseBody := []byte(`{"answers":{},"usage":{"input_tokens":12}}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/v1/systemone", r.URL.Path)
				require.Equal(t, "Bearer ts-mock-key", r.Header.Get("Authorization"))
				got, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Equal(t, requestBody, got)
				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(responseBody)
				require.NoError(t, err)
			}))
			defer server.Close()

			svc := newSystemOneTestService(&systemOneHTTPUpstream{do: server.Client().Do})
			account := &Account{ID: 12, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "ts-mock-key"}}
			result, err := svc.ForwardSystemOne(context.Background(), newSystemOneTestContext(), account, requestBody)
			require.NoError(t, err)
			require.Equal(t, responseBody, result.Body)
			require.Equal(t, 12, result.Usage.InputTokens)
		})
	}
}

func TestForwardSystemOneErrorPolicy(t *testing.T) {
	for _, status := range []int{400, 422, 401, 429, 529, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := &systemOneHTTPUpstream{do: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("private upstream body ts-secret"))}, nil
			}}
			svc := newSystemOneTestService(upstream)
			account := &Account{ID: 8, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "http://typesafe.test", "api_key": "ts-secret"}}
			result, err := svc.ForwardSystemOne(context.Background(), newSystemOneTestContext(), account, []byte(`{"private":"request"}`))
			require.Nil(t, result)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private")
			require.NotContains(t, err.Error(), "ts-secret")
			if status == 400 || status == 422 {
				var upstreamErr *SystemOneUpstreamError
				require.ErrorAs(t, err, &upstreamErr)
				require.Equal(t, status, upstreamErr.StatusCode)
				return
			}
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.True(t, failoverErr.ShouldRetryNextAccount())
			if status == http.StatusUnauthorized {
				require.Equal(t, GatewayFailureStageAccountAuth, failoverErr.Stage)
				require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
				require.Equal(t, TypeSafeCredentialRejectedReason, failoverErr.Reason)
			}
		})
	}
}

func TestForwardSystemOneAppliesExistingAccountStatePolicy(t *testing.T) {
	for _, tc := range []struct {
		name            string
		status          int
		wantRateLimited int
		wantOverloaded  int
		wantError       int
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantError: 1},
		{name: "rate limited", status: http.StatusTooManyRequests, wantRateLimited: 1},
		{name: "overloaded", status: 529, wantOverloaded: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{ID: 10, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "http://typesafe.test", "api_key": "ts-secret"}}
			repo := &systemOnePolicyAccountRepo{account: account}
			upstream := &systemOneHTTPUpstream{do: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"private"}`))}, nil
			}}
			svc := newSystemOneTestService(upstream)
			svc.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

			_, err := svc.ForwardSystemOne(context.Background(), newSystemOneTestContext(), account, []byte(`{}`))
			require.Error(t, err)
			require.Equal(t, tc.wantRateLimited, repo.rateLimitedCalls)
			require.Equal(t, tc.wantOverloaded, repo.overloadedCalls)
			require.Equal(t, tc.wantError, repo.errorCalls)
		})
	}
}

func TestForwardSystemOneTransportAndTimeoutFailOver(t *testing.T) {
	for _, transportErr := range []error{errors.New("network unavailable"), context.DeadlineExceeded} {
		upstream := &systemOneHTTPUpstream{do: func(*http.Request) (*http.Response, error) { return nil, transportErr }}
		svc := newSystemOneTestService(upstream)
		account := &Account{ID: 9, Name: "jev", Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "http://typesafe.test", "api_key": "ts-secret"}}
		_, err := svc.ForwardSystemOne(context.Background(), newSystemOneTestContext(), account, []byte(`{}`))
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
	}
}

func newSystemOneStatusUpstream(status int, body string) *systemOneHTTPUpstream {
	return &systemOneHTTPUpstream{do: func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: http.Header{"X-Request-Id": []string{"req-err"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}}
}

func systemOneOpsEvents(t *testing.T, c *gin.Context) []*OpsUpstreamErrorEvent {
	t.Helper()
	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := raw.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	return events
}

func TestForwardSystemOneRequestErrorsNeverTouchAccountState(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			// Even a custom error-code rule must not let client input disable the account.
			account := &Account{ID: 21, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"base_url": "http://typesafe.test", "api_key": "ts-secret",
				"custom_error_codes_enabled": true, "custom_error_codes": []any{float64(status)},
			}}
			repo := &systemOnePolicyAccountRepo{account: account}
			svc := newSystemOneTestService(newSystemOneStatusUpstream(status, `{"detail":"bad question"}`))
			svc.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			c := newSystemOneTestContext()

			_, err := svc.ForwardSystemOne(context.Background(), c, account, []byte(`{}`))
			var upstreamErr *SystemOneUpstreamError
			require.ErrorAs(t, err, &upstreamErr)
			require.Equal(t, status, upstreamErr.StatusCode)
			require.Zero(t, repo.errorCalls+repo.rateLimitedCalls+repo.overloadedCalls)
			events := systemOneOpsEvents(t, c)
			require.Len(t, events, 1)
			require.Equal(t, "http_error", events[0].Kind)
			require.Equal(t, status, events[0].UpstreamStatusCode)
			require.Equal(t, "req-err", events[0].UpstreamRequestID)
		})
	}
}

func TestForwardSystemOneAccountLevelFailuresFailOver(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		wantError int
	}{
		{name: "payment required", status: http.StatusPaymentRequired, wantError: 1},
		{name: "forbidden", status: http.StatusForbidden, wantError: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{ID: 22, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "http://typesafe.test", "api_key": "ts-secret"}}
			repo := &systemOnePolicyAccountRepo{account: account}
			svc := newSystemOneTestService(newSystemOneStatusUpstream(tc.status, `{"detail":"account problem"}`))
			svc.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			c := newSystemOneTestContext()

			_, err := svc.ForwardSystemOne(context.Background(), c, account, []byte(`{}`))
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.True(t, failoverErr.ShouldRetryNextAccount())
			require.Equal(t, tc.status, failoverErr.StatusCode)
			require.Equal(t, tc.wantError, repo.errorCalls)
			require.Equal(t, "failover", systemOneOpsEvents(t, c)[0].Kind)
		})
	}
}

func TestForwardSystemOneHonorsCustomErrorCodes(t *testing.T) {
	account := &Account{ID: 23, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url": "http://typesafe.test", "api_key": "ts-secret",
		"custom_error_codes_enabled": true, "custom_error_codes": []any{float64(http.StatusNotFound)},
	}}
	repo := &systemOnePolicyAccountRepo{account: account}
	svc := newSystemOneTestService(newSystemOneStatusUpstream(http.StatusNotFound, `{"detail":"missing"}`))
	svc.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	_, err := svc.ForwardSystemOne(context.Background(), newSystemOneTestContext(), account, []byte(`{}`))
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, 1, repo.errorCalls)
}

func TestForwardSystemOneUnhandledStatusDoesNotFailOver(t *testing.T) {
	account := &Account{ID: 24, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "http://typesafe.test", "api_key": "ts-secret"}}
	repo := &systemOnePolicyAccountRepo{account: account}
	svc := newSystemOneTestService(newSystemOneStatusUpstream(http.StatusConflict, `{"detail":"conflict"}`))
	svc.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	_, err := svc.ForwardSystemOne(context.Background(), newSystemOneTestContext(), account, []byte(`{}`))
	var upstreamErr *SystemOneUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusConflict, upstreamErr.StatusCode)
	require.Zero(t, repo.errorCalls)
}

func TestForwardSystemOneNormalizesResponseContentType(t *testing.T) {
	for _, tc := range []struct{ upstream, want string }{
		{"application/json; charset=utf-8", "application/json; charset=utf-8"},
		{"application/problem+json", "application/problem+json"},
		{"text/html; charset=utf-8", "application/json"},
		{"", "application/json"},
		{"not a media type;;", "application/json"},
	} {
		upstream := &systemOneHTTPUpstream{do: func(*http.Request) (*http.Response, error) {
			header := make(http.Header)
			if tc.upstream != "" {
				header.Set("Content-Type", tc.upstream)
			}
			return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(`{"answers":{}}`))}, nil
		}}
		account := &Account{ID: 25, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "http://typesafe.test", "api_key": "ts-secret"}}
		result, err := newSystemOneTestService(upstream).ForwardSystemOne(context.Background(), newSystemOneTestContext(), account, []byte(`{}`))
		require.NoError(t, err)
		require.Equal(t, tc.want, result.ContentType, tc.upstream)
	}
}

func TestForwardSystemOneRejectsOversizedResponse(t *testing.T) {
	oversized := `{"pad":"` + strings.Repeat("a", typesafe.MaxSystemOneResponseBytes) + `"}`
	upstream := &systemOneHTTPUpstream{do: func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(oversized))}, nil
	}}
	account := &Account{ID: 26, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "http://typesafe.test", "api_key": "ts-secret"}}
	c := newSystemOneTestContext()
	_, err := newSystemOneTestService(upstream).ForwardSystemOne(context.Background(), c, account, []byte(`{}`))
	require.ErrorIs(t, err, typesafe.ErrSystemOneResponseTooLarge)
	events := systemOneOpsEvents(t, c)
	require.Len(t, events, 1)
	require.Equal(t, "response_error", events[0].Kind)
	require.Equal(t, http.StatusOK, events[0].UpstreamStatusCode)
}

func TestForwardSystemOneBillsLenientUsageShapes(t *testing.T) {
	upstream := &systemOneHTTPUpstream{do: func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"answers":{},"usage":{"input_tokens":"21","output_tokens":1.0}}`))}, nil
	}}
	account := &Account{ID: 27, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "http://typesafe.test", "api_key": "ts-secret"}}
	result, err := newSystemOneTestService(upstream).ForwardSystemOne(context.Background(), newSystemOneTestContext(), account, []byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, 21, result.Usage.InputTokens)
	require.Equal(t, 1, result.Usage.OutputTokens)
}

func TestTypeSafeAccountBaseURLNeverFallsBackToAnthropic(t *testing.T) {
	account := &Account{Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "ts-secret"}}
	require.Equal(t, typesafe.DefaultBaseURL, account.GetBaseURL())
	require.Equal(t, typesafe.DefaultBaseURL, account.GetTypeSafeBaseURL())
	for raw, want := range map[string]string{
		"https://api.typesafe.ai/v1":      "https://api.typesafe.ai",
		"https://api.typesafe.ai/V1/":     "https://api.typesafe.ai",
		"https://proxy.example/typesafe/": "https://proxy.example/typesafe",
		"https://proxy.example/apiv1":     "https://proxy.example/apiv1",
		" https://proxy.example/x/v1/ ":   "https://proxy.example/x",
		"/v1":                             typesafe.DefaultBaseURL,
	} {
		withBase := &Account{Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": raw}}
		require.Equal(t, want, withBase.GetTypeSafeBaseURL(), raw)
	}
	anthropic := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk"}}
	require.Equal(t, "https://api.anthropic.com", anthropic.GetBaseURL())
}

func TestTypeSafeModelsListCandidates(t *testing.T) {
	require.Equal(t, []string{typesafe.JevLatestModel}, defaultModelsListCandidateIDs(PlatformTypeSafe))
	require.NotContains(t, compositeDefaultModelsListCandidateIDs(), typesafe.JevLatestModel)
}

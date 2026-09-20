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

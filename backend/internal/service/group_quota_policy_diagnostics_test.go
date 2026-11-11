//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type quotaDiagnosticAccounts struct {
	AccountRepository
	accounts []Account
}

func (r *quotaDiagnosticAccounts) ListByGroup(context.Context, int64) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}

func newQuotaDiagnosticService(t *testing.T, count int) (*GroupQuotaPolicyService, *quotaDiagnosticAccounts) {
	t.Helper()
	repo := &quotaDiagnosticAccounts{}
	for i := 1; i <= count; i++ {
		repo.accounts = append(repo.accounts, Account{ID: int64(i), Name: fmt.Sprintf("account-%d", i), Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"access_token": "private", "model_mapping": map[string]any{"target": "gpt-4.1", "image-alias": "gpt-image-1"}}})
	}
	s := NewGroupQuotaPolicyService(nil, quotaPolicyGroups{}, repo, nil, nil)
	t.Cleanup(s.cancel)
	s.diagnostics = newGroupQuotaPolicyDiagnostics()
	s.diagnostics.discover = func(context.Context, *Account) ([]openai.Model, error) { return []openai.Model{{ID: "target"}}, nil }
	return s, repo
}

func TestGroupQuotaPolicyDiagnosticsExecutionPoolAndCache(t *testing.T) {
	s, repo := newQuotaDiagnosticService(t, 2)
	var calls atomic.Int32
	s.diagnostics.probe = func(_ context.Context, _ *Account, upstream string) GroupQuotaMappingAccountResult {
		calls.Add(1)
		if upstream != "gpt-4.1" {
			return GroupQuotaMappingAccountResult{Status: "failed", ErrorCode: "WRONG_ALIAS"}
		}
		return GroupQuotaMappingAccountResult{Status: "passed", Supported: true}
	}
	req := GroupQuotaMappingRequest{Source: "source", Target: "target", AccountIDs: []int64{1}}
	result, err := s.ValidateMapping(context.Background(), 7, req)
	require.NoError(t, err)
	require.Equal(t, "passed", result.Status)
	require.Len(t, result.Accounts, 2, "monitor selection must not restrict execution accounts")
	require.EqualValues(t, 2, calls.Load())
	req.Source = "another-source"
	result, err = s.ValidateMapping(context.Background(), 7, req)
	require.NoError(t, err)
	require.True(t, result.Cached)
	require.EqualValues(t, 2, calls.Load(), "source changes reuse the same target check")
	req.Force = true
	_, err = s.ValidateMapping(context.Background(), 7, req)
	require.NoError(t, err)
	require.EqualValues(t, 4, calls.Load())
	req.Force = false
	repo.accounts[0].Credentials["access_token"] = "rotated-private"
	_, err = s.ValidateMapping(context.Background(), 7, req)
	require.NoError(t, err)
	require.EqualValues(t, 6, calls.Load(), "credential changes invalidate cached diagnostics")
	repo.accounts[0].Proxy = &Proxy{Host: "proxy.example", Port: 8080}
	_, err = s.ValidateMapping(context.Background(), 7, req)
	require.NoError(t, err)
	require.EqualValues(t, 8, calls.Load(), "proxy changes invalidate cached diagnostics")
	repo.accounts = repo.accounts[:1]
	_, err = s.ValidateMapping(context.Background(), 7, req)
	require.NoError(t, err)
	require.EqualValues(t, 9, calls.Load(), "group membership changes invalidate cached diagnostics")
}

func TestGroupQuotaPolicyDiagnosticsValidationAndCoverage(t *testing.T) {
	s, repo := newQuotaDiagnosticService(t, 2)
	var calls atomic.Int32
	s.diagnostics.probe = func(_ context.Context, a *Account, _ string) GroupQuotaMappingAccountResult {
		calls.Add(1)
		if a.ID == 2 {
			return GroupQuotaMappingAccountResult{Status: "failed", Supported: true, ErrorCode: "UPSTREAM_HTTP_429"}
		}
		return GroupQuotaMappingAccountResult{Status: "passed", Supported: true}
	}
	for _, req := range []GroupQuotaMappingRequest{
		{Source: "x", Target: "target", AccountIDs: []int64{3}},
		{Source: "x", Target: "target", AccountIDs: []int64{1, 1}},
		{Source: "x", Target: "target"},
		{Source: "x", Target: "x", AccountIDs: []int64{1}},
		{Source: "*", Target: "target", AccountIDs: []int64{1}},
	} {
		_, err := s.ValidateMapping(context.Background(), 7, req)
		require.Error(t, err)
	}
	require.Zero(t, calls.Load())
	req := GroupQuotaMappingRequest{Source: "source", Target: "target", AccountIDs: []int64{1}}
	result, err := s.ValidateMapping(context.Background(), 7, req)
	require.NoError(t, err)
	require.Equal(t, "partial", result.Status)
	repo.accounts[1].Schedulable = false
	result, err = s.ValidateMapping(context.Background(), 7, req)
	require.NoError(t, err)
	require.Equal(t, "partial", result.Status)
	require.Equal(t, "skipped", result.Accounts[1].Status)
	req.Target = "image-alias"
	before := calls.Load()
	result, err = s.ValidateMapping(context.Background(), 7, req)
	require.NoError(t, err)
	require.Equal(t, "failed", result.Status)
	require.Equal(t, "NON_TEXT_TARGET", result.Accounts[0].ErrorCode)
	require.Equal(t, before, calls.Load(), "image aliases must never issue paid generation probes")
}

func TestGroupQuotaPolicyDiagnosticsConcurrency(t *testing.T) {
	s, _ := newQuotaDiagnosticService(t, 5)
	var running, maximum atomic.Int32
	s.diagnostics.probe = func(context.Context, *Account, string) GroupQuotaMappingAccountResult {
		n := running.Add(1)
		defer running.Add(-1)
		for old := maximum.Load(); n > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		return GroupQuotaMappingAccountResult{Status: "passed", Supported: true}
	}
	result, err := s.ValidateMapping(context.Background(), 7, GroupQuotaMappingRequest{Source: "source", Target: "target", AccountIDs: []int64{1}})
	require.NoError(t, err)
	require.Equal(t, "passed", result.Status)
	require.EqualValues(t, 2, maximum.Load())
}

func TestGroupQuotaPolicyDiagnosticsCatalogFallback(t *testing.T) {
	s, _ := newQuotaDiagnosticService(t, 2)
	s.diagnostics.probe = func(context.Context, *Account, string) GroupQuotaMappingAccountResult {
		panic("catalog must not infer")
	}
	s.diagnostics.discover = func(_ context.Context, a *Account) ([]openai.Model, error) {
		if a.ID == 2 {
			return nil, errors.New("private upstream body")
		}
		return []openai.Model{{ID: "target"}}, nil
	}
	result, err := s.Models(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "configured", result.Accounts[0].CatalogStatus)
	require.Equal(t, "configured", result.Accounts[1].CatalogStatus)
	require.Equal(t, "CATALOG_UNAVAILABLE", result.Accounts[1].ErrorCode)
	require.Equal(t, "gpt-4.1", result.Accounts[0].Models[0].UpstreamModel)
	for _, m := range result.Models {
		if m.ID == "target" {
			require.Equal(t, []int64{1, 2}, m.AccountIDs)
		}
	}
}

func TestGroupQuotaPolicyDiagnosticCompletionRequiresTerminalSuccess(t *testing.T) {
	for _, tc := range []struct{ name, body, code string }{
		{"completed", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n", ""},
		{"done completed", "data: {\"type\":\"response.done\",\"response\":{\"status\":\"completed\"}}\n", ""},
		{"delta EOF", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n", "STREAM_INCOMPLETE"},
		{"DONE only", "data: [DONE]\n", "STREAM_INCOMPLETE"},
		{"incomplete done", "data: {\"type\":\"response.done\",\"response\":{\"status\":\"incomplete\"}}\n", "RESPONSE_NOT_COMPLETED"},
		{"error", "data: {\"type\":\"error\",\"message\":\"private\"}\n", "UPSTREAM_RESPONSE_FAILED"},
		{"oversize", "data: " + strings.Repeat("x", 128<<10) + "\n", "STREAM_READ_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.code, readQuotaDiagnosticCompletion(strings.NewReader(tc.body)))
		})
	}
}

func TestGroupQuotaPolicyDiagnosticProbeNoSchedulingWrites(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		for _, status := range []int{200, 401, 429} {
			t.Run(fmt.Sprintf("%s/%d", accountType, status), func(t *testing.T) {
				body := "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n"
				if status != 200 {
					body = "private-token-and-upstream-error"
				}
				upstream := &queuedHTTPUpstream{responses: []*http.Response{newJSONResponse(status, body)}}
				// The nil embedded repository panics if any scheduling write is attempted.
				repo := &quotaDiagnosticAccounts{}
				s := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: &config.Config{}, openaiGatewayService: &OpenAIGatewayService{accountRepo: repo}}
				a := &Account{ID: 1, Platform: PlatformOpenAI, Type: accountType, Credentials: map[string]any{"access_token": "private", "api_key": "private"}}
				result := s.quotaTargetProbe(context.Background(), a, "gpt-4.1")
				require.Len(t, upstream.requests, 1, "no automatic retry")
				request := upstream.requests[0]
				payload, err := io.ReadAll(request.Body)
				require.NoError(t, err)
				require.Equal(t, "gpt-4.1", gjson.GetBytes(payload, "model").String())
				require.True(t, gjson.GetBytes(payload, "input").IsArray())
				if accountType == AccountTypeOAuth {
					require.Equal(t, chatgptCodexAPIURL, request.URL.String())
					require.False(t, gjson.GetBytes(payload, "max_output_tokens").Exists())
				} else {
					require.Equal(t, "https://api.openai.com/v1/responses", request.URL.String())
					require.False(t, gjson.GetBytes(payload, "max_output_tokens").Exists())
				}
				if status == 200 {
					require.Equal(t, "passed", result.Status)
				} else {
					require.Equal(t, fmt.Sprintf("UPSTREAM_HTTP_%d", status), result.ErrorCode)
				}
				require.NotContains(t, result.Message, "private")
			})
		}
	}
}

func TestGroupQuotaPolicyDiagnosticExpiredAuthAndCatalogAreReadOnly(t *testing.T) {
	repo := &quotaDiagnosticAccounts{}
	upstream := &queuedHTTPUpstream{}
	gateway := &OpenAIGatewayService{accountRepo: repo, openAITokenProvider: NewOpenAITokenProvider(repo, nil, nil)}
	tester := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: &config.Config{}, openaiGatewayService: gateway}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "expired", "expires_at": time.Now().Add(-time.Hour).Format(time.RFC3339), "model_mapping": map[string]any{"alias": "gpt-4.1"}}}
	result := tester.quotaTargetProbe(context.Background(), account, "gpt-4.1")
	require.Equal(t, "AUTH_UNAVAILABLE", result.ErrorCode)
	require.Empty(t, upstream.requests)
	s := NewGroupQuotaPolicyService(nil, quotaPolicyGroups{}, repo, nil, nil)
	t.Cleanup(s.cancel)
	s.SetModelDiagnostics(tester)
	models, err := s.diagnostics.discover(context.Background(), account)
	require.NoError(t, err)
	require.Contains(t, models, openai.Model{ID: "alias"})
	require.Empty(t, upstream.requests, "model candidate reads never authenticate upstream accounts")
}

func TestGroupQuotaPolicyDiagnosticUnrefreshableOAuthBypassesProvider(t *testing.T) {
	for _, tc := range []struct {
		name         string
		expiresIn    time.Duration
		refreshToken string
		wantProvider bool
	}{
		{name: "valid without refresh", expiresIn: time.Hour},
		{name: "within refresh skew without refresh", expiresIn: time.Minute},
		{name: "unknown expiry without refresh"},
		{name: "refreshable retains provider", expiresIn: time.Minute, refreshToken: "refresh", wantProvider: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &quotaDiagnosticAccounts{}
			cache := newOpenAITokenCacheStub()
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "stored", "refresh_token": tc.refreshToken}}
			if tc.expiresIn != 0 {
				account.Credentials["expires_at"] = time.Now().Add(tc.expiresIn).Format(time.RFC3339)
			}
			cache.tokens[OpenAITokenCacheKey(account)] = "cached"
			gateway := &OpenAIGatewayService{accountRepo: repo, openAITokenProvider: NewOpenAITokenProvider(repo, cache, nil)}
			upstream := &queuedHTTPUpstream{responses: []*http.Response{newJSONResponse(200, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n")}}
			tester := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: &config.Config{}, openaiGatewayService: gateway}
			result := tester.quotaTargetProbe(context.Background(), account, "gpt-4.1")
			require.Equal(t, "passed", result.Status)
			require.Len(t, upstream.requests, 1)
			if tc.wantProvider {
				require.EqualValues(t, 1, atomic.LoadInt32(&cache.getCalled))
				require.Equal(t, "Bearer cached", upstream.requests[0].Header.Get("Authorization"))
			} else {
				require.Zero(t, atomic.LoadInt32(&cache.getCalled), "no provider lookup that could race with token expiry")
				require.Equal(t, "Bearer stored", upstream.requests[0].Header.Get("Authorization"))
			}
		})
	}
}

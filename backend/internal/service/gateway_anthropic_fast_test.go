//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const fastTestBody = `{"model":"claude-opus-5","speed":"fast","max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`
const fastTestRejection = `{"type":"error","error":{"type":"rate_limit_error","message":"Usage credits are required for fast mode."}}`
const fastTestSuccess = `{"id":"msg_test","type":"message","role":"assistant","model":"claude-opus-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`

func fastTestService(t *testing.T, rules ...BetaPolicyRule) *GatewayService {
	t.Helper()
	raw, err := json.Marshal(BetaPolicySettings{Rules: rules})
	require.NoError(t, err)
	cfg := &config.Config{}
	return &GatewayService{
		cfg: cfg,
		settingService: NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{
			SettingKeyBetaPolicySettings: string(raw),
		}}, cfg),
		rateLimitService: &RateLimitService{},
	}
}

func fastTestRule(action string) BetaPolicyRule {
	return BetaPolicyRule{BetaToken: claude.BetaFastMode, Action: action, Scope: BetaPolicyScopeAll, ErrorMessage: "Fast disabled by policy"}
}

func fastTestAccount(kind string) *Account {
	return &Account{ID: 1, Platform: PlatformAnthropic, Type: kind, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "synthetic-key", "access_token": "synthetic-token", "project_id": "test-project", "location": "us-east5"}}
}

func fastTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c
}

func TestAnthropicFastPolicyOutboundBuilders(t *testing.T) {
	for _, route := range []string{"apikey", "oauth", "setup-token", "passthrough", "count-apikey", "count-oauth", "count-passthrough", "vertex"} {
		for _, action := range []string{BetaPolicyActionFilter, BetaPolicyActionBlock, BetaPolicyActionPass, "no-rule"} {
			for _, beta := range []string{"", claude.BetaFastMode} {
				t.Run(route+"/"+action+"/"+beta, func(t *testing.T) {
					svc := fastTestService(t, fastTestRule(action))
					if action == "no-rule" {
						svc = fastTestService(t)
					}
					c := fastTestContext()
					c.Request.Header.Set("anthropic-beta", beta)
					body := []byte(fastTestBody)
					account := fastTestAccount(AccountTypeAPIKey)
					tokenType := "apikey"
					if strings.Contains(route, "oauth") || route == "setup-token" {
						account.Type, tokenType = AccountTypeOAuth, "oauth"
					}
					if route == "setup-token" {
						account.Type = AccountTypeSetupToken
					}
					if route == "vertex" {
						account.Type = AccountTypeServiceAccount
					}
					var req *http.Request
					var wire []byte
					var err error
					switch route {
					case "passthrough":
						req, wire, err = svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "synthetic")
					case "count-passthrough":
						req, err = svc.buildCountTokensRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "synthetic")
					case "count-apikey", "count-oauth":
						req, wire, err = svc.buildCountTokensRequest(context.Background(), c, account, body, "synthetic", tokenType, "claude-opus-5", true)
					default:
						req, wire, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "synthetic", tokenType, "claude-opus-5", false, true)
					}
					require.Equal(t, fastTestBody, string(body), "the caller's bytes must remain immutable")
					require.Equal(t, beta, c.GetHeader("anthropic-beta"), "never mutate shared incoming headers")
					if action == BetaPolicyActionBlock {
						var blocked *BetaBlockedError
						require.ErrorAs(t, err, &blocked)
						require.Equal(t, "Fast disabled by policy", blocked.Message)
						require.Nil(t, req)
						return
					}
					require.NoError(t, err)
					actual := readRequestBodyForTest(t, req)
					if wire != nil {
						require.Equal(t, wire, actual, "billing metadata must see the actual wire body")
					}
					require.Equal(t, action != BetaPolicyActionFilter, gjson.GetBytes(actual, "speed").Exists())
					if action == BetaPolicyActionFilter {
						require.False(t, containsBetaToken(getHeaderRaw(req.Header, "anthropic-beta"), claude.BetaFastMode))
					} else if beta != "" && route != "vertex" {
						require.True(t, containsBetaToken(getHeaderRaw(req.Header, "anthropic-beta"), claude.BetaFastMode))
					}
					require.Equal(t, "hello", gjson.GetBytes(actual, "messages.0.content").String())
				})
			}
		}
	}
}

func TestAnthropicFastPolicyScopeAndWhitelist(t *testing.T) {
	for _, tc := range []struct {
		name, scope, kind, action, fallback string
		whitelist                           []string
		wantFilter, wantBlock               bool
	}{
		{"oauth", BetaPolicyScopeOAuth, AccountTypeOAuth, BetaPolicyActionFilter, "", nil, true, false},
		{"setup", BetaPolicyScopeOAuth, AccountTypeSetupToken, BetaPolicyActionBlock, "", nil, false, true},
		{"oauth excludes key", BetaPolicyScopeOAuth, AccountTypeAPIKey, BetaPolicyActionBlock, "", nil, false, false},
		{"key excludes oauth", BetaPolicyScopeAPIKey, AccountTypeOAuth, BetaPolicyActionFilter, "", nil, false, false},
		{"key excludes bedrock", BetaPolicyScopeAPIKey, AccountTypeBedrock, BetaPolicyActionFilter, "", nil, false, false},
		{"bedrock", BetaPolicyScopeBedrock, AccountTypeBedrock, BetaPolicyActionFilter, "", nil, true, false},
		{"whitelist hit", BetaPolicyScopeAll, AccountTypeAPIKey, BetaPolicyActionFilter, "", []string{"claude-opus-*"}, true, false},
		{"whitelist miss defaults pass", BetaPolicyScopeAll, AccountTypeAPIKey, BetaPolicyActionBlock, "", []string{"claude-sonnet-*"}, false, false},
		{"fallback filter", BetaPolicyScopeAll, AccountTypeAPIKey, BetaPolicyActionPass, BetaPolicyActionFilter, []string{"claude-sonnet-*"}, true, false},
		{"fallback block", BetaPolicyScopeAll, AccountTypeAPIKey, BetaPolicyActionPass, BetaPolicyActionBlock, []string{"claude-sonnet-*"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := fastTestRule(tc.action)
			rule.Scope, rule.ModelWhitelist, rule.FallbackAction = tc.scope, tc.whitelist, tc.fallback
			rule.FallbackErrorMessage = "Fast fallback denied"
			svc := fastTestService(t, rule)
			policy := svc.evaluateBetaPolicy(context.Background(), "", fastTestAccount(tc.kind), "claude-opus-5", []byte(fastTestBody))
			_, filtered := policy.filterSet[claude.BetaFastMode]
			require.Equal(t, tc.wantFilter, filtered)
			require.Equal(t, tc.wantBlock, policy.blockErr != nil)
			if tc.fallback == BetaPolicyActionBlock {
				require.Equal(t, "Fast fallback denied", policy.blockErr.Message)
			}
		})
	}
}

func TestAnthropicFastPolicyAttemptIsolationAndOverrides(t *testing.T) {
	rule := fastTestRule(BetaPolicyActionFilter)
	rule.Scope = BetaPolicyScopeOAuth
	svc := fastTestService(t, rule)
	c := fastTestContext()
	c.Request.Header.Set("anthropic-beta", claude.BetaFastMode)
	oauth, key := fastTestAccount(AccountTypeOAuth), fastTestAccount(AccountTypeAPIKey)
	policy := svc.requestBetaPolicy(context.Background(), c, oauth, "claude-opus-5", []byte(fastTestBody))
	ctx := withBetaPolicy(context.Background(), oauth, policy)
	_, filtered, err := svc.buildUpstreamRequest(ctx, c, oauth, []byte(fastTestBody), "synthetic", "oauth", "claude-opus-5", false, false)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(filtered, "speed").Exists())
	_, passed, err := svc.buildUpstreamRequest(ctx, c, key, []byte(fastTestBody), "synthetic", "apikey", "claude-opus-5", false, false)
	require.NoError(t, err)
	require.Equal(t, "fast", gjson.GetBytes(passed, "speed").String(), "a previous account's cached decision must not leak")

	for _, passthrough := range []bool{false, true} {
		svc = fastTestService(t, fastTestRule(BetaPolicyActionFilter))
		key.Credentials["header_override_enabled"] = true
		key.Credentials["header_overrides"] = map[string]any{"anthropic-beta": claude.BetaFastMode + ",unrelated-beta"}
		var req *http.Request
		if passthrough {
			req, _, err = svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, key, []byte(fastTestBody), "synthetic")
		} else {
			req, _, err = svc.buildUpstreamRequest(context.Background(), c, key, []byte(fastTestBody), "synthetic", "apikey", "claude-opus-5", false, false)
		}
		require.NoError(t, err)
		require.False(t, containsBetaToken(getHeaderRaw(req.Header, "anthropic-beta"), claude.BetaFastMode))
		require.True(t, containsBetaToken(getHeaderRaw(req.Header, "anthropic-beta"), "unrelated-beta"))
		require.False(t, gjson.GetBytes(readRequestBodyForTest(t, req), "speed").Exists())
	}
}

type fastTestUpstream struct {
	anthropicHTTPUpstreamRecorder
	calls   int
	status  int
	payload string
	headers http.Header
}

func (u *fastTestUpstream) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	u.calls++
	u.resp = &http.Response{StatusCode: u.status, Header: u.headers.Clone(), Body: io.NopCloser(strings.NewReader(u.payload))}
	return u.anthropicHTTPUpstreamRecorder.Do(req, proxyURL, accountID, concurrency)
}

func (u *fastTestUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestAnthropicFastPolicyForwardBillingAndFailoverInputs(t *testing.T) {
	for _, kind := range []string{AccountTypeAPIKey, AccountTypeOAuth, AccountTypeSetupToken, "passthrough"} {
		for _, action := range []string{BetaPolicyActionPass, BetaPolicyActionFilter, BetaPolicyActionBlock} {
			t.Run(kind+"/"+action, func(t *testing.T) {
				svc := fastTestService(t, fastTestRule(action))
				account := fastTestAccount(kind)
				if kind == "passthrough" {
					account.Type, account.Extra = AccountTypeAPIKey, map[string]any{"anthropic_passthrough": true}
				}
				upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(fastTestSuccess))}}
				svc.httpUpstream = upstream
				original, err := ParseGatewayRequest(NewRequestBodyRef([]byte(fastTestBody)), PlatformAnthropic)
				require.NoError(t, err)
				attempt, err := original.CloneForBody(original.Body.Bytes())
				require.NoError(t, err)
				result, err := svc.Forward(context.Background(), fastTestContext(), account, attempt)
				require.Equal(t, fastTestBody, string(original.Body.Bytes()))
				require.Equal(t, "fast", original.Speed)
				if action == BetaPolicyActionBlock {
					var blocked *BetaBlockedError
					require.ErrorAs(t, err, &blocked)
					require.Nil(t, upstream.lastReq, "block must happen before transport")
					return
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, action == BetaPolicyActionPass, gjson.GetBytes(upstream.lastBody, "speed").Exists())
				if action == BetaPolicyActionPass {
					require.NotNil(t, result.ServiceTier)
					require.Equal(t, "fast", *result.ServiceTier)
				} else {
					require.Nil(t, result.ServiceTier, "filtered requests must not be charged at the Fast tier")
					require.Empty(t, attempt.Speed)
				}
			})
		}
	}
}

func TestAnthropicFastCreditsClassification(t *testing.T) {
	for _, tc := range []struct {
		name, platform, kind string
		status               int
		body                 string
		want                 bool
	}{
		{"exact", PlatformAnthropic, AccountTypeOAuth, 429, fastTestRejection, true},
		{"key", PlatformAnthropic, AccountTypeAPIKey, 429, fastTestRejection, true},
		{"setup", PlatformAnthropic, AccountTypeSetupToken, 429, fastTestRejection, true},
		{"other provider", PlatformOpenAI, AccountTypeAPIKey, 429, fastTestRejection, false},
		{"vertex", PlatformAnthropic, AccountTypeServiceAccount, 429, fastTestRejection, false},
		{"bedrock", PlatformAnthropic, AccountTypeBedrock, 429, fastTestRejection, false},
		{"auth status", PlatformAnthropic, AccountTypeOAuth, 401, fastTestRejection, false},
		{"overload status", PlatformAnthropic, AccountTypeOAuth, 529, fastTestRejection, false},
		{"wrong type", PlatformAnthropic, AccountTypeOAuth, 429, strings.Replace(fastTestRejection, "rate_limit_error", "authentication_error", 1), false},
		{"substring", PlatformAnthropic, AccountTypeOAuth, 429, strings.Replace(fastTestRejection, "Usage credits", "Example: Usage credits", 1), false},
		{"ordinary", PlatformAnthropic, AccountTypeOAuth, 429, `{"type":"error","error":{"type":"rate_limit_error","message":"Rate limit exceeded"}}`, false},
		{"invalid JSON", PlatformAnthropic, AccountTypeOAuth, 429, fastTestRejection + "x", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := fastTestAccount(tc.kind)
			account.Platform = tc.platform
			require.Equal(t, tc.want, isAnthropicFastCreditsRequired(account, tc.status, []byte(tc.body)))
		})
	}
}

func TestAnthropicFastCreditsNeverPersistMonthlyLimit(t *testing.T) {
	for _, existing := range []bool{false, true} {
		repo := &anthropicWindowLimitRepo{}
		svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		account := fastTestAccount(AccountTypeOAuth)
		account.Credentials["temp_unschedulable_enabled"] = true
		account.Credentials["temp_unschedulable_rules"] = []any{map[string]any{"error_code": float64(429), "keywords": []any{"credits"}, "duration_minutes": float64(30)}}
		existingReset := time.Now().Add(3 * time.Hour)
		if existing {
			account.RateLimitResetAt = &existingReset
		}
		headers := http.Header{}
		headers.Set("anthropic-ratelimit-unified-reset", strconv.FormatInt(time.Now().Add(30*24*time.Hour).Unix(), 10))
		require.False(t, svc.HandleUpstreamError(context.Background(), account, 429, headers, []byte(fastTestRejection)))
		require.Equal(t, ErrorPolicySkipped, svc.CheckErrorPolicy(context.Background(), account, 429, []byte(fastTestRejection)))
		require.Zero(t, repo.rateLimitCalls)
		require.Zero(t, repo.tempUnschedCalls)
		require.Zero(t, repo.modelRateLimitCalls)
		require.Zero(t, repo.sessionWindowCalls)
		require.Nil(t, repo.lastExtraUpdates)
		if existing {
			require.Equal(t, &existingReset, account.RateLimitResetAt)
		} else {
			require.Nil(t, account.RateLimitResetAt)
		}
	}
	for _, window := range []string{"5h", "7d"} {
		repo := &anthropicWindowLimitRepo{}
		svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		headers := http.Header{}
		reset := time.Now().Add(2 * time.Hour).Truncate(time.Second)
		headers.Set("anthropic-ratelimit-unified-"+window+"-status", "rejected")
		headers.Set("anthropic-ratelimit-unified-"+window+"-utilization", "1.0")
		headers.Set("anthropic-ratelimit-unified-"+window+"-reset", strconv.FormatInt(reset.Unix(), 10))
		headers.Set("anthropic-ratelimit-unified-reset", strconv.FormatInt(time.Now().Add(30*24*time.Hour).Unix(), 10))
		svc.HandleUpstreamError(context.Background(), fastTestAccount(AccountTypeOAuth), 429, headers, []byte(fastTestRejection))
		require.Equal(t, 1, repo.rateLimitCalls, "independent genuine exhaustion remains actionable")
		require.Equal(t, reset, repo.lastRateLimitReset)
	}
}

func TestAnthropicFastResponseRejectionPreservesOrdinary429(t *testing.T) {
	repo := &anthropicWindowLimitRepo{}
	svc := &GatewayService{rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
	account := fastTestAccount(AccountTypeOAuth)
	const ordinary = `{"type":"error","error":{"type":"rate_limit_error","message":"Account window exhausted"}}`
	reset := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-status", "rejected")
	headers.Set("anthropic-ratelimit-unified-5h-reset", strconv.FormatInt(reset.Unix(), 10))
	resp := &http.Response{StatusCode: 429, Header: headers, Body: io.NopCloser(strings.NewReader(ordinary))}
	require.Nil(t, svc.anthropicFastResponseRejection(context.Background(), fastTestContext(), account, resp))
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, ordinary, string(body))
	svc.rateLimitService.HandleUpstreamError(context.Background(), account, 429, headers, body)
	require.Equal(t, 1, repo.rateLimitCalls)
	require.Equal(t, reset, repo.lastRateLimitReset)
}

func TestAnthropicFastPolicyPreservesStandardAndBlocksHeaderOnly(t *testing.T) {
	account := fastTestAccount(AccountTypeAPIKey)
	body := []byte(strings.Replace(fastTestBody, `"speed":"fast"`, `"speed":"standard"`, 1))
	svc := fastTestService(t, fastTestRule(BetaPolicyActionFilter))
	policy := svc.evaluateBetaPolicy(context.Background(), "", account, "claude-opus-5", body)
	out, err := policy.applyBody(body)
	require.NoError(t, err)
	require.Equal(t, body, out)
	svc = fastTestService(t, fastTestRule(BetaPolicyActionBlock))
	require.Nil(t, svc.evaluateBetaPolicy(context.Background(), "", account, "claude-opus-5", body).blockErr)
	require.NotNil(t, svc.evaluateBetaPolicy(context.Background(), claude.BetaFastMode, account, "claude-opus-5", body).blockErr)
}

func TestAnthropicFastPolicyForwardKeepsRequestedModelWhitelist(t *testing.T) {
	rule := fastTestRule(BetaPolicyActionFilter)
	rule.ModelWhitelist = []string{"client-alias"}
	svc := fastTestService(t, rule)
	account := fastTestAccount(AccountTypeAPIKey)
	account.Credentials["model_mapping"] = map[string]any{"client-alias": "claude-opus-5"}
	upstream := &fastTestUpstream{status: 200, headers: http.Header{}, payload: fastTestSuccess}
	svc.httpUpstream = upstream
	parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(strings.Replace(fastTestBody, "claude-opus-5", "client-alias", 1))), PlatformAnthropic)
	require.NoError(t, err)
	result, err := svc.Forward(context.Background(), fastTestContext(), account, parsed)
	require.NoError(t, err)
	require.Equal(t, "claude-opus-5", gjson.GetBytes(upstream.lastBody, "model").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "speed").Exists())
	require.Nil(t, result.ServiceTier)
}

func TestAnthropicFastPolicyBedrockWire(t *testing.T) {
	for _, action := range []string{BetaPolicyActionFilter, BetaPolicyActionBlock} {
		svc := fastTestService(t, fastTestRule(action))
		account := fastTestAccount(AccountTypeBedrock)
		account.Credentials["auth_mode"] = "apikey"
		account.Credentials["region"] = "us-east-1"
		account.Credentials["model_mapping"] = map[string]any{"claude-opus-5": "us.anthropic.claude-opus-5-v1"}
		upstream := &fastTestUpstream{status: 200, headers: http.Header{}, payload: fastTestSuccess}
		svc.httpUpstream = upstream
		parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(fastTestBody)), PlatformAnthropic)
		require.NoError(t, err)
		_, err = svc.Forward(context.Background(), fastTestContext(), account, parsed)
		if action == BetaPolicyActionBlock {
			var blocked *BetaBlockedError
			require.ErrorAs(t, err, &blocked)
			require.Zero(t, upstream.calls)
		} else {
			require.NoError(t, err)
			require.Equal(t, 1, upstream.calls)
			require.False(t, gjson.GetBytes(upstream.lastBody, "speed").Exists())
			require.Equal(t, "bedrock-2023-05-31", gjson.GetBytes(upstream.lastBody, "anthropic_version").String())
		}
	}
}

func TestAnthropicFastCreditsCountTokensAndSSE(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		repo := &anthropicWindowLimitRepo{}
		svc := fastTestService(t)
		svc.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		account := fastTestAccount(AccountTypeAPIKey)
		account.Extra = map[string]any{"anthropic_passthrough": passthrough}
		headers := http.Header{}
		headers.Set("anthropic-ratelimit-unified-reset", strconv.FormatInt(time.Now().Add(30*24*time.Hour).Unix(), 10))
		upstream := &fastTestUpstream{status: 429, headers: headers, payload: fastTestRejection}
		svc.httpUpstream = upstream
		parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(fastTestBody)), PlatformAnthropic)
		require.NoError(t, err)
		err = svc.ForwardCountTokens(context.Background(), fastTestContext(), account, parsed)
		require.Error(t, err)
		require.Equal(t, 1, upstream.calls)
		require.Zero(t, repo.rateLimitCalls)
		require.Zero(t, repo.tempUnschedCalls)
		for _, outputStarted := range []bool{false, true} {
			failure := svc.sseStreamErrorEventFailover(context.Background(), fastTestContext(), account, &http.Response{Header: headers}, fastTestRejection, "claude-opus-5", outputStarted)
			require.True(t, failure.IsAnthropicFastCreditsRequired())
			require.False(t, failure.ShouldRetryNextAccount())
			require.Equal(t, 429, failure.StatusCode)
			require.Zero(t, repo.rateLimitCalls)
		}
	}
}

func TestAnthropicFastCreditsNonAnthropicStillRateLimited(t *testing.T) {
	repo := &anthropicWindowLimitRepo{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := fastTestAccount(AccountTypeAPIKey)
	account.Platform = PlatformOpenAI
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-window-minutes", "300")
	headers.Set("x-codex-primary-reset-after-seconds", "7200")
	svc.HandleUpstreamError(context.Background(), account, 429, headers, []byte(fastTestRejection))
	require.Equal(t, 1, repo.rateLimitCalls, "coincidental text from another provider must not bypass its rate limits")
}

func TestAnthropicFastCreditsForwardStopsWithoutAccountPenalty(t *testing.T) {
	for _, route := range []string{"native", "passthrough", "chat", "responses"} {
		t.Run(route, func(t *testing.T) {
			repo := &anthropicWindowLimitRepo{}
			svc := fastTestService(t)
			svc.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			headers := http.Header{}
			headers.Set("anthropic-ratelimit-unified-reset", strconv.FormatInt(time.Now().Add(30*24*time.Hour).Unix(), 10))
			upstream := &fastTestUpstream{status: 429, headers: headers, payload: fastTestRejection}
			svc.httpUpstream = upstream
			account := fastTestAccount(AccountTypeAPIKey)
			// Excluding 429 normally triggers internal same-account retries.
			account.Credentials["custom_error_codes_enabled"] = true
			account.Credentials["custom_error_codes"] = []any{float64(500)}
			c := fastTestContext()
			var err error
			switch route {
			case "chat":
				_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(`{"model":"claude-opus-5","messages":[{"role":"user","content":"hi"}]}`), nil)
			case "responses":
				_, err = svc.ForwardAsResponses(context.Background(), c, account, []byte(`{"model":"claude-opus-5","input":"hi"}`), nil)
			default:
				if route == "passthrough" {
					account.Extra = map[string]any{"anthropic_passthrough": true}
				}
				parsed, parseErr := ParseGatewayRequest(NewRequestBodyRef([]byte(fastTestBody)), PlatformAnthropic)
				require.NoError(t, parseErr)
				_, err = svc.Forward(context.Background(), c, account, parsed)
			}
			var failover *UpstreamFailoverError
			require.True(t, errors.As(err, &failover), "%v", err)
			require.Equal(t, GatewayFailureScopeRequest, failover.Scope)
			require.Equal(t, GatewayFailureStageInference, failover.Stage)
			require.True(t, failover.IsAnthropicFastCreditsRequired())
			require.False(t, failover.ShouldRetryNextAccount())
			require.False(t, failover.RetryableOnSameAccount)
			require.False(t, failover.ShouldReportAccountScheduleFailure())
			require.Equal(t, fastTestRejection, string(failover.ResponseBody))
			require.Zero(t, repo.rateLimitCalls)
			require.Zero(t, repo.tempUnschedCalls)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, 1, upstream.calls, "do not retry the unchanged entitlement failure")
		})
	}
}

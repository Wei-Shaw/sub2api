package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openaidecisions "github.com/Wei-Shaw/sub2api/internal/pkg/openai_decisions"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type decisionsUpstreamStub struct {
	request *http.Request
	body    string
	status  int
	err     error
	calls   int
}

func (s *decisionsUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	s.request = req.Clone(req.Context())
	data, _ := io.ReadAll(req.Body)
	s.request.Body = io.NopCloser(strings.NewReader(string(data)))
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"dec-1"}}, Body: io.NopCloser(strings.NewReader(s.body))}, nil
}

func (s *decisionsUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestForwardDecisionsUsesDedicatedEndpointAndAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(`{"model":"gpt-6-luna"}`))
	stub := &decisionsUpstreamStub{body: `{"model":"gpt-6-luna","answers":[{"type":"predicate","probability":0.9}],"usage":{"input_tokens":4,"output_tokens":0,"input_tokens_details":{"cached_tokens":1}}}`}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://api.openai.com"}}
	body := []byte(`{"model":"gpt-6-luna","input":"hello","questions":[{"type":"predicate","instructions":"Is this valid?"}]}`)
	result, err := svc.ForwardDecisions(c, c, account, body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, result.StatusCode)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 1, result.Usage.CacheReadInputTokens)
	require.Equal(t, "/v1/decisions", stub.request.URL.Path)
	require.Equal(t, "Bearer sk-test", stub.request.Header.Get("Authorization"))
	require.Equal(t, string(body), string(mustReadBody(t, stub.request.Body)))
}

func mustReadBody(t *testing.T, body io.Reader) []byte {
	t.Helper()
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	return data
}

const decisionsTestBody = `{"model":"gpt-6-luna","input":"hello","questions":[{"type":"predicate","instructions":"Is this valid?"}]}`
const openrouterTestReply = `{"model":"openai/gpt-6-luna-decisions","answers":{"q0":{"type":"noul","noul":0.9}},"usage":{"input_tokens":4,"output_tokens":0,"cost":900,"input_tokens_details":{"cached_tokens":1}}}`

func decisionsTestContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/decisions", nil)
	return c
}
func TestDecisionsOpenRouterRoundTripAndProtocolIsolation(t *testing.T) {
	stub := &decisionsUpstreamStub{body: openrouterTestReply}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeUpstream, Credentials: map[string]any{"api_key": "test", "openai_decisions_protocol": "openrouter", "base_url": "https://relay.example/api/v1"}}
	result, err := svc.ForwardDecisions(context.Background(), decisionsTestContext(), account, []byte(decisionsTestBody))
	require.NoError(t, err)
	require.Equal(t, "/api/alpha/decisions", stub.request.URL.Path)
	wire := mustReadBody(t, stub.request.Body)
	require.Equal(t, openaidecisions.OpenRouterModel, gjson.GetBytes(wire, "model").String())
	require.Equal(t, "hello", gjson.GetBytes(wire, "state").String())
	require.False(t, gjson.GetBytes(wire, "input").Exists())
	require.Equal(t, 1, result.Usage.CacheReadInputTokens)
	require.Equal(t, "gpt-6-luna", gjson.GetBytes(result.Body, "model").String())
	require.True(t, gjson.GetBytes(result.Body, "answers").IsArray())
	require.Equal(t, openaidecisions.OpenRouterModel, result.BillingModel)
	require.Equal(t, openaidecisions.OpenRouterPath, result.UpstreamEndpoint)
	account.Credentials["openai_decisions_protocol"] = "openai"
	account.Credentials["base_url"] = "https://api.openai.com/v1"
	stub.body = `{"model":"gpt-6-luna","answers":[{"type":"predicate","probability":0.9}],"usage":{"input_tokens":4,"output_tokens":0}}`
	_, err = svc.ForwardDecisions(context.Background(), decisionsTestContext(), account, []byte(decisionsTestBody))
	require.NoError(t, err)
	require.Equal(t, "/v1/decisions", stub.request.URL.Path)
	require.Equal(t, decisionsTestBody, string(mustReadBody(t, stub.request.Body)))
}
func TestDecisionsErrorsAndCancel(t *testing.T) {
	for _, status := range []int{400, 413, 422, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			stub := &decisionsUpstreamStub{status: status, body: `{"error":{"message":"error"}}`}
			svc := &OpenAIGatewayService{httpUpstream: stub}
			account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "openai_decisions_protocol": "openrouter"}}
			result, err := svc.ForwardDecisions(context.Background(), decisionsTestContext(), account, []byte(decisionsTestBody))
			if status == 429 || status == 503 {
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
			} else {
				require.NoError(t, err)
				require.Equal(t, status, result.StatusCode)
			}
			require.Equal(t, 1, stub.calls)
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stub := &decisionsUpstreamStub{err: context.Canceled}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}}
	_, err := svc.ForwardDecisions(ctx, decisionsTestContext(), account, []byte(decisionsTestBody))
	require.True(t, errors.Is(err, context.Canceled))
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	stub.err = nil
	stub.body = `{"model":"wrong"}`
	_, err = svc.ForwardDecisions(context.Background(), decisionsTestContext(), account, []byte(decisionsTestBody))
	require.Error(t, err)
	require.False(t, errors.As(err, &failover))
	require.Equal(t, 2, stub.calls)
}
func TestDecisionsURLConfiguration(t *testing.T) {
	for _, base := range []string{"https://openrouter.ai", "https://openrouter.ai/api", "https://openrouter.ai/api/v1/", "https://openrouter.ai/v1", "https://openrouter.ai/api/alpha/decisions"} {
		got, err := buildOpenRouterDecisionsURL(base)
		require.NoError(t, err)
		require.Equal(t, "https://openrouter.ai/api/alpha/decisions", got)
	}
	got, err := buildOpenRouterDecisionsURL("https://relay.example/prefix/api/v1")
	require.NoError(t, err)
	require.Equal(t, "https://relay.example/prefix/api/alpha/decisions", got)
}
func TestDecisionsOpenRouterMultiImageCapability(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "openai_decisions_protocol": "openrouter"}}
	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDecisions))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDecisionsMultiImage))
	account.Credentials["openai_decisions_protocol"] = "openai"
	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDecisionsMultiImage))
}

func TestDecisionsCredentialsRejectUnsupportedTargets(t *testing.T) {
	for _, tc := range []struct {
		platform, typ string
		raw           any
		valid         bool
	}{{PlatformOpenAI, AccountTypeAPIKey, "openrouter", true}, {PlatformOpenAI, AccountTypeUpstream, "openai", true}, {PlatformOpenAI, AccountTypeOAuth, "openrouter", false}, {PlatformAnthropic, AccountTypeAPIKey, "openrouter", false}, {PlatformOpenAI, AccountTypeAPIKey, "invalid", false}, {PlatformOpenAI, AccountTypeAPIKey, true, false}} {
		err := validateOpenAIDecisionsCredentials(tc.platform, tc.typ, map[string]any{"openai_decisions_protocol": tc.raw})
		if tc.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

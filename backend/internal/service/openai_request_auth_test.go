package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func testOpenAIAgentIdentityAccount(t *testing.T, id int64) *Account {
	t.Helper()
	seed := sha256.Sum256([]byte("agent-identity-test-key"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	return &Account{
		ID:       id,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"auth_mode":                        OpenAIAuthModeAgentIdentity,
			OpenAIAgentRuntimeIDCredentialKey:  "runtime-test",
			OpenAIAgentPrivateKeyCredentialKey: base64.StdEncoding.EncodeToString(der),
			OpenAIAgentTaskIDCredentialKey:     "task-test",
			"chatgpt_account_id":               "account-test",
			"chatgpt_user_id":                  "user-test",
			"chatgpt_account_is_fedramp":       false,
		},
	}
}

func TestOpenAIRequestAuthProviderBuildsVerifiableAgentAssertion(t *testing.T) {
	account := testOpenAIAgentIdentityAccount(t, 1)
	fixedNow := time.Date(2026, 7, 14, 12, 34, 56, 789, time.UTC)
	provider := NewOpenAIRequestAuthProvider(nil, nil)
	provider.now = func() time.Time { return fixedNow }

	result, err := provider.Build(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, OpenAIAuthModeAgentIdentity, result.Mode)
	require.Equal(t, "account-test", result.Headers.Get("ChatGPT-Account-ID"))

	authorization := result.Headers.Get("Authorization")
	require.True(t, strings.HasPrefix(authorization, "AgentAssertion "))
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(authorization, "AgentAssertion "))
	require.NoError(t, err)
	var envelope struct {
		AgentRuntimeID string `json:"agent_runtime_id"`
		TaskID         string `json:"task_id"`
		Timestamp      string `json:"timestamp"`
		Signature      string `json:"signature"`
	}
	require.NoError(t, json.Unmarshal(payload, &envelope))
	require.Equal(t, "runtime-test", envelope.AgentRuntimeID)
	require.Equal(t, "task-test", envelope.TaskID)
	require.Equal(t, "2026-07-14T12:34:56Z", envelope.Timestamp)

	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	require.NoError(t, err)
	privateKey, err := parseOpenAIAgentIdentityPrivateKey(credentialString(account.Credentials, OpenAIAgentPrivateKeyCredentialKey))
	require.NoError(t, err)
	require.True(t, ed25519.Verify(privateKey.Public().(ed25519.PublicKey), []byte("runtime-test:task-test:2026-07-14T12:34:56Z"), signature))
}
func TestOpenAIAgentIdentityBuildsHTTPAndWebSocketHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := testOpenAIAgentIdentityAccount(t, 2)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4"}`))
	svc := &OpenAIGatewayService{}

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5.4"}`), true, "", true)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(req.Header.Get("Authorization"), "AgentAssertion "))
	require.Equal(t, "account-test", req.Header.Get("ChatGPT-Account-ID"))

	headers, _, err := svc.buildOpenAIWSHeaders(
		c.Request.Context(),
		c,
		account,
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		true,
		"",
		"",
		"",
	)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(headers.Get("Authorization"), "AgentAssertion "))
	require.Equal(t, "account-test", headers.Get("ChatGPT-Account-ID"))
}

func TestGetOpenAIUsageHTTPAgentIdentityRefreshesAgainAfterTTL(t *testing.T) {
	account := testOpenAIAgentIdentityAccount(t, 44)
	repo := &sparkShadowUsageTestRepo{
		accounts: map[int64]*Account{44: account},
	}
	tokenProvider := NewOpenAITokenProvider(repo, nil, nil)
	var usageCalls atomic.Int32
	var capturedAuthorization atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/wham/usage") {
			usageCalls.Add(1)
			capturedAuthorization.Store(r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{
				PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: 12, ResetAfterSeconds: 1200, LimitWindowSeconds: 18000},
				SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 34, ResetAfterSeconds: 7200, LimitWindowSeconds: 604800},
			}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"available_count": 0})
	}))
	defer srv.Close()

	quotaService := NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv))
	svc := &AccountUsageService{accountRepo: repo, openAIQuotaService: quotaService}
	usage, err := svc.getOpenAIUsage(context.Background(), account, false)
	require.NoError(t, err)
	require.NotNil(t, usage.FiveHour)
	require.NotNil(t, usage.SevenDay)
	require.Equal(t, int32(1), usageCalls.Load())
	require.True(t, strings.HasPrefix(capturedAuthorization.Load().(string), "AgentAssertion "))

	account.Extra["codex_usage_updated_at"] = time.Now().Add(-openAIProbeCacheTTL - time.Minute).Format(time.RFC3339)
	_, err = svc.getOpenAIUsage(context.Background(), account, false)
	require.NoError(t, err)
	require.Equal(t, int32(2), usageCalls.Load(), "HTTP-only Agent Identity must query /wham/usage again after TTL")
}

func TestOpenAICodexSnapshotStaleKeepsOrdinaryHTTPBearerBehavior(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token",
		},
		Extra: map[string]any{
			"codex_usage_updated_at": time.Now().Add(-openAIProbeCacheTTL - time.Minute).Format(time.RFC3339),
		},
	}
	require.False(t, isOpenAICodexSnapshotStale(account, time.Now()))
}

//go:build unit

package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 换取 SA access token 时 SA 已不存在（token 端点 400 invalid_grant
// "Invalid grant: account not found"）的契约，三条转发路径一致：
//   - 账号置为 error 并停止调度，错误信息带 token 端点的 error_description；
//   - 不发出上游请求，返回 *UpstreamFailoverError(401) 交给 handler 换号，service 不写响应；
//   - Ops 事件记为凭据获取阶段（stage=account_auth）；
//   - 其他换 token 失败不改账号状态、不换号，直接回写 502。
// ---------------------------------------------------------------------------

const vertexAccountNotFoundTokenBody = `{"error":"invalid_grant","error_description":"Invalid grant: account not found"}`

func newVertexTokenTestAccount(t *testing.T) *Account {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	saJSON, err := json.Marshal(map[string]string{
		"type":           "service_account",
		"project_id":     "proj-1",
		"private_key_id": "kid-1",
		"private_key":    string(pemBytes),
		"client_email":   "svc@proj-1.iam.gserviceaccount.com",
	})
	require.NoError(t, err)
	return &Account{
		ID:       820,
		Platform: PlatformGemini,
		Type:     AccountTypeServiceAccount,
		Credentials: map[string]any{
			"service_account_json": string(saJSON),
			"project_id":           "proj-1",
			"location":             "global",
		},
	}
}

func useVertexTokenEndpoint(t *testing.T, status int, body string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	original := vertexServiceAccountTokenURL
	vertexServiceAccountTokenURL = server.URL
	t.Cleanup(func() { vertexServiceAccountTokenURL = original })
}

func newVertexTokenErrorService() (*GeminiMessagesCompatService, *geminiCompatHTTPUpstreamStub, *errorPolicyRepoStub) {
	repo := &errorPolicyRepoStub{}
	httpStub := &geminiCompatHTTPUpstreamStub{}
	svc := &GeminiMessagesCompatService{
		accountRepo:      repo,
		httpUpstream:     httpStub,
		tokenProvider:    NewGeminiTokenProvider(repo, nil, nil),
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
		cfg:              &config.Config{},
	}
	return svc, httpStub, repo
}

func requireVertexAccountNotFoundHandled(t *testing.T, c *gin.Context, err error, httpStub *geminiCompatHTTPUpstreamStub, repo *errorPolicyRepoStub) {
	t.Helper()
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "SA 不存在应换号，got %T: %v", err, err)
	require.Equal(t, http.StatusUnauthorized, failoverErr.StatusCode)
	require.Equal(t, string(geminiVertexCredentialFailoverBody), string(failoverErr.ResponseBody))
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.False(t, failoverErr.RetryableOnSameAccount)

	require.Equal(t, 1, repo.setErrCalls, "SA 不存在应把账号置为 error")
	require.Equal(t, "Service account token rejected (400): Invalid grant: account not found", repo.lastErrorMsg)
	require.Zero(t, httpStub.calls, "换不到 token 时不应发出上游请求")
	require.False(t, c.Writer.Written(), "换号场景不应写客户端响应")

	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok, "应记录 ops 上游错误事件")
	events := raw.([]*OpsUpstreamErrorEvent)
	require.Len(t, events, 1)
	require.Equal(t, "failover", events[0].Kind)
	require.Equal(t, int64(820), events[0].AccountID)
	require.Equal(t, string(GatewayFailureStageAccountAuth), events[0].Stage)
	require.Equal(t, string(GatewayFailureScopeAccount), events[0].Scope)
	require.Equal(t, string(vertexServiceAccountNotFoundReason), events[0].Reason)
	require.Zero(t, events[0].UpstreamStatusCode, "换 token 失败不占用上游状态码")
	require.Contains(t, events[0].Message, "service account token request returned 400: Invalid grant: account not found")
}

func TestExchangeVertexServiceAccountToken_AccountNotFoundIsRecognized(t *testing.T) {
	useVertexTokenEndpoint(t, http.StatusBadRequest, vertexAccountNotFoundTokenBody)
	key, err := parseVertexServiceAccountKey(newVertexTokenTestAccount(t))
	require.NoError(t, err)

	_, _, err = exchangeVertexServiceAccountToken(contextWithTestTimeout(t), key, "")

	require.EqualError(t, err, "service account token request returned 400: Invalid grant: account not found")
	tokenErr, ok := vertexServiceAccountNotFoundError(fmt.Errorf("build request: %w", err))
	require.True(t, ok)
	require.Equal(t, "invalid_grant", tokenErr.Code)
	require.Equal(t, "Invalid grant: account not found", tokenErr.Description)
}

func TestVertexServiceAccountNotFoundError_OnlyMatchesAccountNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"other_invalid_grant", &vertexTokenRequestError{StatusCode: 400, Code: "invalid_grant", Description: "Invalid JWT Signature."}},
		{"not_400", &vertexTokenRequestError{StatusCode: 401, Code: "invalid_grant", Description: "Invalid grant: account not found"}},
		{"other_code", &vertexTokenRequestError{StatusCode: 400, Code: "invalid_request", Description: "Invalid grant: account not found"}},
		{"server_error", &vertexTokenRequestError{StatusCode: 500, Message: "backend error"}},
		{"plain_error", errors.New("service account token request returned 400: Invalid grant: account not found")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := vertexServiceAccountNotFoundError(tt.err)
			require.False(t, ok)
		})
	}
}

func TestGeminiForwardNative_VertexServiceAccountNotFoundDisablesAndFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useVertexTokenEndpoint(t, http.StatusBadRequest, vertexAccountNotFoundTokenBody)
	svc, httpStub, repo := newVertexTokenErrorService()
	c, _ := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, newVertexTokenTestAccount(t),
		"gemini-3.8-flash", "streamGenerateContent", true, []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))

	require.Nil(t, result)
	requireVertexAccountNotFoundHandled(t, c, err, httpStub, repo)
}

func TestGeminiForward_VertexServiceAccountNotFoundDisablesAndFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useVertexTokenEndpoint(t, http.StatusBadRequest, vertexAccountNotFoundTokenBody)
	svc, httpStub, repo := newVertexTokenErrorService()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-3.8-flash","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	result, err := svc.Forward(context.Background(), c, newVertexTokenTestAccount(t), body)

	require.Nil(t, result)
	requireVertexAccountNotFoundHandled(t, c, err, httpStub, repo)
}

func TestGeminiForwardAsChatCompletions_VertexServiceAccountNotFoundDisablesAndFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useVertexTokenEndpoint(t, http.StatusBadRequest, vertexAccountNotFoundTokenBody)
	svc, httpStub, repo := newVertexTokenErrorService()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-3.8-flash","messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, newVertexTokenTestAccount(t), body)

	require.Nil(t, result)
	requireVertexAccountNotFoundHandled(t, c, err, httpStub, repo)
}

func TestGeminiForwardNative_OtherVertexTokenErrorKeepsAccountState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useVertexTokenEndpoint(t, http.StatusBadRequest, `{"error":"invalid_grant","error_description":"Invalid JWT Signature."}`)
	svc, httpStub, repo := newVertexTokenErrorService()
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, newVertexTokenTestAccount(t),
		"gemini-3.8-flash", "generateContent", false, []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))

	require.Nil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "未识别的换 token 失败不换号")
	require.Zero(t, repo.setErrCalls, "未识别的换 token 失败不改账号状态")
	require.Zero(t, httpStub.calls)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.True(t, strings.Contains(rec.Body.String(), "Invalid JWT Signature."))
}

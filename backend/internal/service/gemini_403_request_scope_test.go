//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	gemini403ScopeUpstreamBody  = `{"error":{"code":403,"message":"The caller does not have permission for project-123","status":"PERMISSION_DENIED"}}`
	gemini403ScopeSuccessBody   = `{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1,"totalTokenCount":4}}`
	gemini403ScopeNativeBody    = `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`
	gemini403ScopeClaudeBody    = `{"model":"gemini-2.5-flash","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`
	gemini403ScopeChatBody      = `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`
	gemini403ScopeNativePath    = "/v1beta/models/gemini-2.5-flash:generateContent"
	gemini403ScopeClaudePath    = "/v1/messages"
	gemini403ScopeChatPath      = "/v1/chat/completions"
	gemini403ScopeProjectMarker = "project-123"
)

func newGemini403ScopeService(responses ...geminiSequenceUpstreamResponse) (*GeminiMessagesCompatService, *geminiSequenceUpstreamStub, *errorPolicyRepoStub) {
	repo := &errorPolicyRepoStub{}
	upstream := &geminiSequenceUpstreamStub{responses: responses}
	svc := &GeminiMessagesCompatService{
		accountRepo:      repo,
		httpUpstream:     upstream,
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
		cfg:              &config.Config{},
	}
	return svc, upstream, repo
}

func gemini403ScopeAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Platform:    PlatformGemini,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}
}

func newGemini403ScopeContext(t *testing.T, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	return c, rec
}

type gemini403ScopeForwarder struct {
	name    string
	path    string
	body    []byte
	forward func(t *testing.T, svc *GeminiMessagesCompatService, c *gin.Context, account *Account) (*ForwardResult, error)
}

var gemini403ScopeForwarders = []gemini403ScopeForwarder{
	{
		name: "native",
		path: gemini403ScopeNativePath,
		body: []byte(gemini403ScopeNativeBody),
		forward: func(t *testing.T, svc *GeminiMessagesCompatService, c *gin.Context, account *Account) (*ForwardResult, error) {
			t.Helper()
			return svc.ForwardNative(context.Background(), c, account, "gemini-2.5-flash", "generateContent", false, []byte(gemini403ScopeNativeBody))
		},
	},
	{
		name: "claude_compat",
		path: gemini403ScopeClaudePath,
		body: []byte(gemini403ScopeClaudeBody),
		forward: func(t *testing.T, svc *GeminiMessagesCompatService, c *gin.Context, account *Account) (*ForwardResult, error) {
			t.Helper()
			return svc.Forward(context.Background(), c, account, []byte(gemini403ScopeClaudeBody))
		},
	},
	{
		name: "chat_completions_compat",
		path: gemini403ScopeChatPath,
		body: []byte(gemini403ScopeChatBody),
		forward: func(t *testing.T, svc *GeminiMessagesCompatService, c *gin.Context, account *Account) (*ForwardResult, error) {
			t.Helper()
			return svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(gemini403ScopeChatBody))
		},
	},
}

func TestGemini403_FirstAccountIsDeferredWithoutAccountPenalty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fw := range gemini403ScopeForwarders {
		t.Run(fw.name, func(t *testing.T) {
			svc, upstream, repo := newGemini403ScopeService(geminiSequenceUpstreamResponse{
				status: http.StatusForbidden,
				body:   gemini403ScopeUpstreamBody,
			})
			c, rec := newGemini403ScopeContext(t, fw.path, fw.body)

			result, err := fw.forward(t, svc, c, gemini403ScopeAccount(910))

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr), "first 403 should fail over")
			require.Equal(t, http.StatusForbidden, failoverErr.StatusCode)
			require.Equal(t, 1, upstream.calls)
			require.Zero(t, repo.setErrCalls, "first 403 must not disable the account")
			require.Zero(t, repo.tempCalls, "first 403 must not cool down the account")
			require.Zero(t, rec.Body.Len(), "failover attempt must not write a client response")
		})
	}
}

func TestGemini403_SecondDifferentAccountStopsAsRequestScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fw := range gemini403ScopeForwarders {
		t.Run(fw.name, func(t *testing.T) {
			svc, upstream, repo := newGemini403ScopeService(
				geminiSequenceUpstreamResponse{status: http.StatusForbidden, body: gemini403ScopeUpstreamBody},
				geminiSequenceUpstreamResponse{status: http.StatusForbidden, body: gemini403ScopeUpstreamBody},
			)
			c, rec := newGemini403ScopeContext(t, fw.path, fw.body)

			_, err := fw.forward(t, svc, c, gemini403ScopeAccount(920))
			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr))

			result, err := fw.forward(t, svc, c, gemini403ScopeAccount(921))

			require.Nil(t, result)
			require.Error(t, err)
			require.False(t, errors.As(err, &failoverErr), "request-scoped 403 must stop failover")
			require.Equal(t, 2, upstream.calls)
			require.Zero(t, repo.setErrCalls, "neither account may be disabled")
			require.Zero(t, repo.tempCalls, "neither account may be cooled down")
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), GeminiRequestScoped403ClientMessage)
			require.NotContains(t, rec.Body.String(), gemini403ScopeProjectMarker, "upstream body must not leak to the client")
			if fw.name == "native" {
				var payload struct {
					Error struct {
						Code   int    `json:"code"`
						Status string `json:"status"`
					} `json:"error"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
				require.Equal(t, http.StatusBadRequest, payload.Error.Code)
				require.Equal(t, "INVALID_ARGUMENT", payload.Error.Status)
			}
		})
	}
}

func TestGemini403_AnotherAccountSuccessConfirmsAndPenalizesDeferredAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fw := range gemini403ScopeForwarders {
		t.Run(fw.name, func(t *testing.T) {
			svc, upstream, repo := newGemini403ScopeService(
				geminiSequenceUpstreamResponse{status: http.StatusForbidden, body: gemini403ScopeUpstreamBody},
				geminiSequenceUpstreamResponse{status: http.StatusOK, body: gemini403ScopeSuccessBody},
			)
			c, rec := newGemini403ScopeContext(t, fw.path, fw.body)

			_, err := fw.forward(t, svc, c, gemini403ScopeAccount(930))
			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr))
			require.Zero(t, repo.setErrCalls)

			result, err := fw.forward(t, svc, c, gemini403ScopeAccount(931))

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 2, upstream.calls)
			require.Equal(t, 1, repo.setErrCalls, "confirmed 403 should disable the deferred account")
			require.Contains(t, repo.lastErrorMsg, "Access forbidden (403)")
			require.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestGemini403_FirstAccountBypassesTempUnschedulablePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fw := range gemini403ScopeForwarders {
		t.Run(fw.name, func(t *testing.T) {
			svc, _, repo := newGemini403ScopeService(geminiSequenceUpstreamResponse{
				status: http.StatusForbidden,
				body:   gemini403ScopeUpstreamBody,
			})
			account := gemini403ScopeAccount(940)
			account.Credentials["temp_unschedulable_enabled"] = true
			account.Credentials["temp_unschedulable_rules"] = []any{
				map[string]any{
					"error_code":       float64(http.StatusForbidden),
					"keywords":         []any{"permission"},
					"duration_minutes": float64(10),
				},
			}
			c, _ := newGemini403ScopeContext(t, fw.path, fw.body)

			_, err := fw.forward(t, svc, c, account)

			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr))
			require.Zero(t, repo.tempCalls, "first 403 must bypass temp-unschedulable rules")
			require.Zero(t, repo.setErrCalls)
		})
	}
}

func TestGemini403_ConfirmedTempRuleAppliesPolicyOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fw := range gemini403ScopeForwarders {
		t.Run(fw.name, func(t *testing.T) {
			svc, _, repo := newGemini403ScopeService(
				geminiSequenceUpstreamResponse{status: http.StatusForbidden, body: gemini403ScopeUpstreamBody},
				geminiSequenceUpstreamResponse{status: http.StatusOK, body: gemini403ScopeSuccessBody},
			)
			account := gemini403ScopeAccount(950)
			account.Credentials["temp_unschedulable_enabled"] = true
			account.Credentials["temp_unschedulable_rules"] = []any{
				map[string]any{
					"error_code":       float64(http.StatusForbidden),
					"keywords":         []any{"permission"},
					"duration_minutes": float64(10),
					"scope":            TempUnschedScopeAccount,
				},
			}
			c, _ := newGemini403ScopeContext(t, fw.path, fw.body)

			_, err := fw.forward(t, svc, c, account)
			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr))
			require.Zero(t, repo.tempCalls)

			_, err = fw.forward(t, svc, c, gemini403ScopeAccount(951))

			require.NoError(t, err)
			require.Equal(t, 1, repo.tempCalls, "confirmed 403 should apply the configured policy once")
			require.Zero(t, repo.setErrCalls)
		})
	}
}

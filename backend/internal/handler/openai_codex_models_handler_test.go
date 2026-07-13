package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type codexModelsHandlerAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r codexModelsHandlerAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			account := r.accounts[i]
			return &account, nil
		}
	}
	return nil, service.ErrNoAvailableAccounts
}

func (r codexModelsHandlerAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, platform string) ([]service.Account, error) {
	return r.accountsForPlatform(platform), nil
}

func (r codexModelsHandlerAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.accountsForPlatform(platform), nil
}

func (r codexModelsHandlerAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.accountsForPlatform(platform), nil
}

func (r codexModelsHandlerAccountRepo) accountsForPlatform(platform string) []service.Account {
	out := make([]service.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform {
			out = append(out, account)
		}
	}
	return out
}

type codexModelsHandlerUpstreamCall struct {
	accountID   int64
	ifNoneMatch string
}

type codexModelsHandlerUpstream struct {
	service.HTTPUpstream
	do    func(req *http.Request, accountID int64) (*http.Response, error)
	calls []codexModelsHandlerUpstreamCall
}

func (u *codexModelsHandlerUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.calls = append(u.calls, codexModelsHandlerUpstreamCall{
		accountID:   accountID,
		ifNoneMatch: req.Header.Get("If-None-Match"),
	})
	return u.do(req, accountID)
}

func newCodexModelsHandlerTestAccount(id int64, accountType string, priority int) service.Account {
	credentials := map[string]any{"access_token": "token-" + strconv.FormatInt(id, 10)}
	if accountType == service.AccountTypeAPIKey {
		credentials = map[string]any{"api_key": "api-key"}
	}
	return service.Account{
		ID:          id,
		Name:        "account",
		Platform:    service.PlatformOpenAI,
		Type:        accountType,
		Status:      service.StatusActive,
		Schedulable: true,
		Priority:    priority,
		Credentials: credentials,
	}
}

func newCodexModelsHandlerForTest(accounts []service.Account, upstream service.HTTPUpstream) *OpenAIGatewayHandler {
	cfg := &config.Config{RunMode: config.RunModeSimple}
	gatewayService := service.NewOpenAIGatewayService(
		codexModelsHandlerAccountRepo{accounts: accounts},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		nil,
		nil,
		nil,
		nil,
		upstream,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	return &OpenAIGatewayHandler{gatewayService: gatewayService, maxAccountSwitches: 3}
}

func newCodexModelsHandlerContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	groupID := int64(3131)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, EndpointModels+"?client_version=0.144.0", nil)
	c.Request.Header.Set("If-None-Match", `W/"cached-account-2"`)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      99,
		GroupID: &groupID,
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
		},
	})
	return c, rec
}

func TestOpenAIGatewayHandlerCodexModels_FailsOverAndClearsETag(t *testing.T) {
	tests := []struct {
		name       string
		firstError func() (*http.Response, error)
	}{
		{
			name: "response header timeout",
			firstError: func() (*http.Response, error) {
				return nil, context.DeadlineExceeded
			},
		},
		{
			name: "upstream 500",
			firstError: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Status:     "500 Internal Server Error",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporary"}}`)),
				}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := []service.Account{
				newCodexModelsHandlerTestAccount(1, service.AccountTypeAPIKey, 0),
				newCodexModelsHandlerTestAccount(2, service.AccountTypeOAuth, 1),
				newCodexModelsHandlerTestAccount(3, service.AccountTypeOAuth, 2),
			}
			upstream := &codexModelsHandlerUpstream{do: func(_ *http.Request, accountID int64) (*http.Response, error) {
				if accountID == 2 {
					return tt.firstError()
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"ETag": []string{`W/"account-3"`}},
					Body:       io.NopCloser(strings.NewReader(`{"models":[{"slug":"gpt-5.5"}]}`)),
				}, nil
			}}
			handler := newCodexModelsHandlerForTest(accounts, upstream)
			c, rec := newCodexModelsHandlerContext(t)

			handler.CodexModels(c)

			require.Equal(t, http.StatusOK, rec.Code)
			require.JSONEq(t, `{"models":[{"slug":"gpt-5.5"}]}`, rec.Body.String())
			require.Equal(t, []codexModelsHandlerUpstreamCall{
				{accountID: 2, ifNoneMatch: `W/"cached-account-2"`},
				{accountID: 3, ifNoneMatch: ""},
			}, upstream.calls)
			require.Equal(t, EndpointModels, GetInboundEndpoint(c))
			require.Equal(t, EndpointCodexModels, GetUpstreamEndpoint(c, service.PlatformOpenAI))
			require.Equal(t, int64(3), mustContextInt64(t, c, opsAccountIDKey))
			require.Equal(t, int16(service.RequestTypeSync), mustContextInt16(t, c, opsRequestTypeKey))

			rawEvents, ok := c.Get(service.OpsUpstreamErrorsKey)
			require.True(t, ok)
			events, ok := rawEvents.([]*service.OpsUpstreamErrorEvent)
			require.True(t, ok)
			require.Len(t, events, 1)
			require.Equal(t, int64(2), events[0].AccountID)
		})
	}
}

func TestOpenAIGatewayHandlerCodexModels_DoesNotFailOverCanceledRequest(t *testing.T) {
	accounts := []service.Account{
		newCodexModelsHandlerTestAccount(1, service.AccountTypeOAuth, 0),
		newCodexModelsHandlerTestAccount(2, service.AccountTypeOAuth, 1),
	}
	upstream := &codexModelsHandlerUpstream{do: func(_ *http.Request, _ int64) (*http.Response, error) {
		return nil, context.Canceled
	}}
	handler := newCodexModelsHandlerForTest(accounts, upstream)
	c, _ := newCodexModelsHandlerContext(t)

	handler.CodexModels(c)

	require.Len(t, upstream.calls, 1)
	require.Equal(t, int64(1), upstream.calls[0].accountID)
}

func TestOpenAIGatewayHandlerCodexModels_StopsAtConfiguredSwitchLimit(t *testing.T) {
	accounts := []service.Account{
		newCodexModelsHandlerTestAccount(1, service.AccountTypeOAuth, 0),
		newCodexModelsHandlerTestAccount(2, service.AccountTypeOAuth, 1),
		newCodexModelsHandlerTestAccount(3, service.AccountTypeOAuth, 2),
	}
	upstream := &codexModelsHandlerUpstream{do: func(_ *http.Request, _ int64) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	}}
	handler := newCodexModelsHandlerForTest(accounts, upstream)
	handler.maxAccountSwitches = 1
	c, rec := newCodexModelsHandlerContext(t)

	handler.CodexModels(c)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Len(t, upstream.calls, 2)
	require.Equal(t, int64(1), upstream.calls[0].accountID)
	require.Equal(t, int64(2), upstream.calls[1].accountID)
	require.Equal(t, int64(2), mustContextInt64(t, c, opsAccountIDKey))
	rawEvents, ok := c.Get(service.OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*service.OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 2)
}

func mustContextInt64(t *testing.T, c *gin.Context, key string) int64 {
	t.Helper()
	value, ok := c.Get(key)
	require.True(t, ok)
	result, ok := value.(int64)
	require.True(t, ok)
	return result
}

func mustContextInt16(t *testing.T, c *gin.Context, key string) int16 {
	t.Helper()
	value, ok := c.Get(key)
	require.True(t, ok)
	result, ok := value.(int16)
	require.True(t, ok)
	return result
}

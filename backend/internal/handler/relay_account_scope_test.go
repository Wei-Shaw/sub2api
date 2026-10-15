package handler

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type relayScopeAccountRepo struct {
	service.AccountRepository
	account *service.Account
}

func (r relayScopeAccountRepo) GetByID(context.Context, int64) (*service.Account, error) {
	return r.account, nil
}

func TestRelayAccountScopeRejectsPlatformProxiesAndKeepsBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{`{"proxy_id":3}`, `{"proxy_id":"3"}`, `[]`, `{broken`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Request = httptest.NewRequest("POST", "/relay/openai/generate-auth-url", strings.NewReader(body))
		(&RelayHandler{}).AccountScope(c)
		require.True(t, c.IsAborted(), body)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
	c.Request = httptest.NewRequest("POST", "/relay/openai/exchange-code", strings.NewReader(`{"proxy_id":null,"session_id":"session","code":"secret"}`))
	(&RelayHandler{}).AccountScope(c)
	require.False(t, c.IsAborted())
	require.Equal(t, int64(7), service.RelayOwnerFromContext(c.Request.Context()))
	body, err := io.ReadAll(c.Request.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"session_id":"session","code":"secret"}`, string(body))
}

func TestRelayOwnedAccountGateStopsSharedHandlerBeforeReadingCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	owner := int64(7)
	relay := service.NewRelayService(nil, relayScopeAccountRepo{account: &service.Account{ID: 9, OwnerUserID: &owner}}, nil, nil, nil, nil, nil)
	h := NewRelayHandler(relay, nil)
	for _, caller := range []int64{7, 8} {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: caller}) })
		called := false
		r.GET("/relay/accounts/:id", h.RequireOwnedAccount, func(c *gin.Context) { called = true; c.Status(200) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/relay/accounts/9", nil))
		require.Equal(t, caller == owner, called)
		if caller != owner {
			require.Equal(t, 404, w.Code)
		}
	}
}

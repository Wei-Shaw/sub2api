package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type zhipuResetHandlerRepo struct{ service.AccountRepository }

func (zhipuResetHandlerRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	return &service.Account{ID: id, Type: service.AccountTypeAPIKey, Platform: service.PlatformZhipu,
		Credentials: map[string]any{"account_mode": service.AccountModeCoding}}, nil
}

func (zhipuResetHandlerRepo) CompareAndSwapZhipuResetCards(context.Context, *service.Account, json.RawMessage) (bool, error) {
	panic("missing idempotency must not write")
}

func TestZhipuResetCardsAdminSessionAndIdempotency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Totp.EncryptionKeyConfigured = true
	h := &AccountHandler{zhipuResetCards: service.NewZhipuResetCardsService(
		zhipuResetHandlerRepo{}, &openDesignTestUpstream{t: t}, openDesignTestEncryptor{}, cfg)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_method", c.GetHeader("Test-Auth-Method"))
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
	})
	router.GET("/accounts/:id/cards", h.GetZhipuResetCards)
	router.POST("/accounts/:id/cards/use", h.UseZhipuResetCard)
	call := func(method, url, auth, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Test-Auth-Method", auth)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	require.Equal(t, http.StatusForbidden, call(http.MethodGet, "/accounts/1/cards", "admin_api_key", "").Code)
	unbound := call(http.MethodGet, "/accounts/1/cards", "jwt", "")
	require.Equal(t, http.StatusOK, unbound.Code)
	require.Equal(t, "no-store", unbound.Header().Get("Cache-Control"))
	require.Contains(t, unbound.Body.String(), `"status":"unbound"`)
	bad := call(http.MethodPost, "/accounts/1/cards/use", "jwt", `{"binding_id":"test","reset_type":"WEEK"}`)
	require.Equal(t, http.StatusBadRequest, bad.Code)
	require.Contains(t, bad.Body.String(), "IDEMPOTENCY_KEY_REQUIRED")
}

func TestZhipuResetCardsBindingNotExported(t *testing.T) {
	extra := map[string]any{service.ZhipuResetCardsExtraKey: "encrypted-fixture", "other": "preserved"}
	require.Equal(t, map[string]any{"other": "preserved"}, exportAccountExtra(extra))
	require.Contains(t, extra, service.ZhipuResetCardsExtraKey)
}

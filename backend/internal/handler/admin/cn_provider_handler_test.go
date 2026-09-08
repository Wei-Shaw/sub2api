package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type cnProviderHandlerAccountRepo struct {
	service.AccountRepository
	account *service.Account
}

func (r *cnProviderHandlerAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if r.account != nil && r.account.ID == id {
		return r.account, nil
	}
	return nil, service.ErrAccountNotFound
}

type cnProviderHandlerUpstream struct {
	calls atomic.Int64
}

func (u *cnProviderHandlerUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	return nil, context.Canceled
}

func (u *cnProviderHandlerUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestCNProviderHandlerQueryQuotaRejectsQwenTokenPlanWithoutUpstreamCall(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &cnProviderHandlerAccountRepo{account: &service.Account{
		ID:       88,
		Platform: service.PlatformQwen,
		Type:     service.AccountTypeAPIKey,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"account_mode": service.AccountModeTokenPlan,
			"api_key":      "qwen-token-plan-key",
		},
	}}
	upstream := &cnProviderHandlerUpstream{}
	quotaService := service.NewCNProviderQuotaService(repo, nil, upstream, nil)
	handler := NewCNProviderHandler(quotaService, nil)

	router := gin.New()
	router.GET("/admin/cn-providers/accounts/:id/quota", handler.QueryQuota)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/cn-providers/accounts/88/quota", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, int64(0), upstream.calls.Load(), "Qwen Token Plan must not send a usage request")

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "CN_QUOTA_NOT_SUPPORTED", body["reason"])
	require.Contains(t, body["message"], "does not support usage queries")
}

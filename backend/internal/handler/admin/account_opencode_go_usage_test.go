package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openCodeGoUsageHandlerTestRepo struct {
	service.AccountRepository
	account  *service.Account
	accounts []*service.Account
}

func (r *openCodeGoUsageHandlerTestRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if r.account != nil && r.account.ID == id {
		return r.account, nil
	}
	for _, account := range r.accounts {
		if account.ID == id {
			return account, nil
		}
	}
	return nil, service.ErrAccountNotFound
}

func (r *openCodeGoUsageHandlerTestRepo) ListOpenCodeGoUsageGroupAccounts(_ context.Context, anchors []*service.Account) ([]service.Account, error) {
	wanted := make(map[string]struct{}, len(anchors))
	for _, anchor := range anchors {
		if apiKey, ok := openCodeGoUsageHandlerTestAPIKey(anchor); ok {
			wanted[apiKey] = struct{}{}
		}
	}
	result := make([]service.Account, 0, len(r.accounts)+1)
	if r.account != nil {
		if apiKey, ok := openCodeGoUsageHandlerTestAPIKey(r.account); ok {
			if _, match := wanted[apiKey]; match {
				result = append(result, *r.account)
			}
		}
	}
	for _, account := range r.accounts {
		if apiKey, ok := openCodeGoUsageHandlerTestAPIKey(account); ok {
			if _, match := wanted[apiKey]; match {
				result = append(result, *account)
			}
		}
	}
	return result, nil
}

func openCodeGoUsageHandlerTestAPIKey(account *service.Account) (string, bool) {
	if account == nil || account.Credentials == nil {
		return "", false
	}
	apiKey, ok := account.Credentials["api_key"].(string)
	return apiKey, ok && apiKey != ""
}

func (r *openCodeGoUsageHandlerTestRepo) SetOpenCodeGoUsageAutoRefresh(context.Context, *service.Account, bool) error {
	return nil
}
func (r *openCodeGoUsageHandlerTestRepo) UpdateOpenCodeGoUsageSnapshot(context.Context, *service.Account, *service.OpenCodeGoUsageSnapshot) error {
	return nil
}
func (r *openCodeGoUsageHandlerTestRepo) DisableOpenCodeGoUsageAutoRefresh(context.Context, *service.Account) error {
	return nil
}
func (r *openCodeGoUsageHandlerTestRepo) ListDueOpenCodeGoUsageAccounts(context.Context, time.Time, time.Duration, time.Duration, int) ([]service.Account, error) {
	return nil, nil
}

func newOpenCodeGoUsageHandlerTestService(t *testing.T) *service.OpenCodeGoUsageService {
	t.Helper()
	svc := service.NewOpenCodeGoUsageService(nil, nil, nil)
	t.Cleanup(svc.Stop)
	return svc
}

func newOpenCodeGoUsageHandlerContext(method, target, body, id string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	if id != "" {
		ctx.Params = gin.Params{{Key: "id", Value: id}}
	}
	return ctx, recorder
}

func TestOpenCodeGoUsageHandlersValidateRequestsAndDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newOpenCodeGoUsageHandlerTestService(t)

	t.Run("invalid account id", func(t *testing.T) {
		ctx, recorder := newOpenCodeGoUsageHandlerContext(http.MethodGet, "/admin/accounts/not-an-id/opencode-go-usage", "", "not-an-id")
		(&AccountHandler{opencodeGoUsage: svc}).GetOpenCodeGoUsage(ctx)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("missing enabled", func(t *testing.T) {
		ctx, recorder := newOpenCodeGoUsageHandlerContext(http.MethodPut, "/admin/accounts/7/opencode-go-usage/auto-refresh", `{}`, "7")
		(&AccountHandler{opencodeGoUsage: svc}).SetOpenCodeGoUsageAutoRefresh(ctx)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("service unavailable", func(t *testing.T) {
		ctx, recorder := newOpenCodeGoUsageHandlerContext(http.MethodGet, "/admin/accounts/7/opencode-go-usage", "", "7")
		(&AccountHandler{}).GetOpenCodeGoUsage(ctx)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Contains(t, recorder.Body.String(), "OPENCODE_GO_USAGE_UNAVAILABLE")
	})
}

func TestGetOpenCodeGoUsageSettingsHandlerSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newOpenCodeGoUsageHandlerContext(http.MethodGet, "/admin/accounts/opencode-go-usage/settings", "", "")
	handler := &AccountHandler{opencodeGoUsage: newOpenCodeGoUsageHandlerTestService(t)}

	handler.GetOpenCodeGoUsageSettings(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"enabled":false`)
	require.Contains(t, recorder.Body.String(), `"interval_minutes":15`)
}

func TestOpenCodeGoUsageStateEmbeddedInListAndDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	account := &service.Account{
		ID: 7, Name: "opencode", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://opencode.ai/zen/go/v1", "api_key": "test-key"},
		Extra: map[string]any{
			service.OpenCodeGoUsageAutoRefreshExtraKey: true,
			service.OpenCodeGoUsageSnapshotExtraKey: &service.OpenCodeGoUsageSnapshot{
				Status: service.OpenCodeGoUsageStatusOK, Data: &service.OpenCodeGoUsageData{
					Rolling: service.OpenCodeGoUsageWindow{Status: "ok", Percent: 6},
				},
				LastAttemptAt: now, NextRefreshAt: now.Add(time.Hour),
			},
		},
		Status: service.StatusActive,
	}
	repo := &openCodeGoUsageHandlerTestRepo{account: account}
	adminService := newStubAdminService()
	adminService.accounts = []service.Account{*account}
	adminService.getAccountResult = account
	usageService := service.NewOpenCodeGoUsageService(repo, nil, nil)
	t.Cleanup(usageService.Stop)
	handler := NewAccountHandler(adminService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.SetOpenCodeGoUsageService(usageService)
	router := gin.New()
	router.GET("/accounts", handler.List)
	router.GET("/accounts/:id", handler.GetByID)
	router.GET("/accounts/:id/opencode-go-usage", handler.GetOpenCodeGoUsage)

	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/accounts?page=1&page_size=20", nil))
	require.Equal(t, http.StatusOK, listRecorder.Code)
	var listPayload struct {
		Data struct {
			Items []struct {
				OpenCodeGoUsage *service.OpenCodeGoUsageState `json:"opencode_go_usage"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listRecorder.Body.Bytes(), &listPayload))
	require.Len(t, listPayload.Data.Items, 1)
	require.NotNil(t, listPayload.Data.Items[0].OpenCodeGoUsage)
	require.True(t, listPayload.Data.Items[0].OpenCodeGoUsage.AutoRefreshEnabled)
	require.Equal(t, 6.0, listPayload.Data.Items[0].OpenCodeGoUsage.Snapshot.Data.Rolling.Percent)

	detailRecorder := httptest.NewRecorder()
	router.ServeHTTP(detailRecorder, httptest.NewRequest(http.MethodGet, "/accounts/7", nil))
	require.Equal(t, http.StatusOK, detailRecorder.Code)
	var detailPayload struct {
		Data struct {
			OpenCodeGoUsage *service.OpenCodeGoUsageState `json:"opencode_go_usage"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(detailRecorder.Body.Bytes(), &detailPayload))
	require.NotNil(t, detailPayload.Data.OpenCodeGoUsage)
	require.Equal(t, 6.0, detailPayload.Data.OpenCodeGoUsage.Snapshot.Data.Rolling.Percent)

	stateRecorder := httptest.NewRecorder()
	router.ServeHTTP(stateRecorder, httptest.NewRequest(http.MethodGet, "/accounts/7/opencode-go-usage", nil))
	require.Equal(t, http.StatusOK, stateRecorder.Code)
	var statePayload struct {
		Data service.OpenCodeGoUsageState `json:"data"`
	}
	require.NoError(t, json.Unmarshal(stateRecorder.Body.Bytes(), &statePayload))
	require.Equal(t, statePayload.Data.Snapshot, detailPayload.Data.OpenCodeGoUsage.Snapshot)

	for _, body := range []string{listRecorder.Body.String(), detailRecorder.Body.String(), stateRecorder.Body.String()} {
		require.NotContains(t, body, "test-key")
	}
}

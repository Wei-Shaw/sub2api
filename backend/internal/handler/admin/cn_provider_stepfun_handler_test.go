package admin

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stepPlanHandlerTestRepo struct {
	service.AccountRepository
	account *service.Account
	updates map[string]any
}

func (r *stepPlanHandlerTestRepo) GetByID(context.Context, int64) (*service.Account, error) {
	return r.account, nil
}

func (r *stepPlanHandlerTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = updates
	return nil
}

func (r *stepPlanHandlerTestRepo) NextStepPlanUsageAttemptSeq(context.Context) (int64, error) {
	return 1, nil
}

func (r *stepPlanHandlerTestRepo) UpdateStepPlanExtraIfIdentityUnchanged(
	_ context.Context,
	_ int64,
	_ string,
	_ string,
	updates map[string]any,
	_ int64,
) error {
	r.updates = updates
	if r.account.Extra == nil {
		r.account.Extra = make(map[string]any)
	}
	for key, value := range updates {
		r.account.Extra[key] = value
	}
	return nil
}

func stepPlanHandlerTestAccount() *service.Account {
	return &service.Account{
		ID: 31, Platform: service.PlatformStepfun, Type: service.AccountTypeAPIKey, Status: service.StatusActive,
		Credentials: map[string]any{
			"account_mode": service.AccountModeCoding,
			"api_key":      "sk-stepfun",
			"base_url":     service.DefaultStepFunCodingBaseURL,
		},
		Extra: map[string]any{
			service.StepFunUsageSnapshotExtraKey: map[string]any{
				"status":           service.StepFunUsageStatusOK,
				"credential_valid": true,
				"plan_level":       "Plus",
				"tiers": []any{
					map[string]any{"window": "monthly", "used_percent": float64(30)},
				},
				"last_attempt_at": "2026-10-21T03:00:00Z",
			},
		},
	}
}

func TestStepPlanUsageHandlerReturnsState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &stepPlanHandlerTestRepo{account: stepPlanHandlerTestAccount()}
	serviceLayer := service.NewCNProviderQuotaService(repo, nil, nil, nil)
	handler := NewCNProviderHandler(serviceLayer, nil)
	ctx, recorder := newOpenCodeGoUsageHandlerContext(http.MethodGet, "/admin/cn-providers/accounts/31/quota/state", "", "31")

	handler.GetStepPlanUsageState(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"eligible":true`)
	require.Contains(t, recorder.Body.String(), `"auto_refresh_enabled":true`)
	require.Contains(t, recorder.Body.String(), `"plan_level":"Plus"`)
	require.NotContains(t, recorder.Body.String(), "sk-stepfun")
}

func TestStepPlanUsageAutoRefreshHandlerPersistsFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &stepPlanHandlerTestRepo{account: stepPlanHandlerTestAccount()}
	serviceLayer := service.NewCNProviderQuotaService(repo, nil, nil, nil)
	handler := NewCNProviderHandler(serviceLayer, nil)
	ctx, recorder := newOpenCodeGoUsageHandlerContext(http.MethodPut, "/admin/cn-providers/accounts/31/quota/auto-refresh", `{"enabled":false}`, "31")

	handler.SetStepPlanUsageAutoRefresh(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, false, repo.updates[service.StepFunUsageAutoRefreshExtraKey])
	require.Contains(t, recorder.Body.String(), `"auto_refresh_enabled":false`)
}

func TestStepPlanUsageAutoRefreshHandlerRequiresEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &stepPlanHandlerTestRepo{account: stepPlanHandlerTestAccount()}
	serviceLayer := service.NewCNProviderQuotaService(repo, nil, nil, nil)
	handler := NewCNProviderHandler(serviceLayer, nil)
	ctx, recorder := newOpenCodeGoUsageHandlerContext(http.MethodPut, "/admin/cn-providers/accounts/31/quota/auto-refresh", `{}`, "31")

	handler.SetStepPlanUsageAutoRefresh(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

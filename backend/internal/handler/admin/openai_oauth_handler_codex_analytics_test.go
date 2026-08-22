//go:build unit

package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func performCodexAnalyticsRequest(handler *OpenAIOAuthHandler, query string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/admin/accounts/:id/codex-analytics", handler.QueryCodexAnalytics)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/42/codex-analytics"+query, nil)
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestQueryCodexAnalyticsValidatesAccountBeforeServiceInvocation(t *testing.T) {
	tests := []struct {
		name    string
		account *service.Account
	}{
		{
			name: "platform",
			account: &service.Account{
				ID:       42,
				Platform: service.PlatformAnthropic,
				Type:     service.AccountTypeOAuth,
			},
		},
		{
			name: "type",
			account: &service.Account{
				ID:       42,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeSetupToken,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			quota := &openAIQuotaWorkflowStub{}
			handler := &OpenAIOAuthHandler{
				adminService: &openAIResetAdminServiceStub{account: test.account},
				quotaService: quota,
			}

			response := performCodexAnalyticsRequest(handler, "?period=recent&days=7")

			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Zero(t, quota.analyticsCalls)
		})
	}
}

func TestQueryCodexAnalyticsDefaultsToCurrentSevenDayCycle(t *testing.T) {
	account := &service.Account{ID: 42, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	quota := &openAIQuotaWorkflowStub{analyticsResult: &service.OpenAICodexAnalytics{AccountID: 42, PeriodDays: 7}}
	handler := &OpenAIOAuthHandler{adminService: &openAIResetAdminServiceStub{account: account}, quotaService: quota}

	response := performCodexAnalyticsRequest(handler, "")

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, 1, quota.analyticsCalls)
	require.Equal(t, int64(42), quota.analyticsID)
	require.Equal(t, service.OpenAICodexAnalyticsQuery{Period: service.OpenAICodexAnalyticsCurrent7Days}, quota.analyticsQuery)
}

func TestQueryCodexAnalyticsParsesRecentDays(t *testing.T) {
	account := &service.Account{ID: 42, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	quota := &openAIQuotaWorkflowStub{analyticsResult: &service.OpenAICodexAnalytics{AccountID: 42, PeriodDays: 14}}
	handler := &OpenAIOAuthHandler{adminService: &openAIResetAdminServiceStub{account: account}, quotaService: quota}

	response := performCodexAnalyticsRequest(handler, "?period=recent&days=14")

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, service.OpenAICodexAnalyticsQuery{Period: service.OpenAICodexAnalyticsRecent, Days: 14}, quota.analyticsQuery)
}

func TestQueryCodexAnalyticsValidatesPeriodQuery(t *testing.T) {
	account := &service.Account{ID: 42, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	queries := []string{
		"?period=recent",
		"?period=recent&days=0",
		"?period=recent&days=31",
		"?period=recent&days=invalid",
		"?period=current_7d&days=7",
		"?period=invalid&days=7",
	}
	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			quota := &openAIQuotaWorkflowStub{}
			handler := &OpenAIOAuthHandler{adminService: &openAIResetAdminServiceStub{account: account}, quotaService: quota}

			response := performCodexAnalyticsRequest(handler, query)

			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Zero(t, quota.analyticsCalls)
		})
	}
}

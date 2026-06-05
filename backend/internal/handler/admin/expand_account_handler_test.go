package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type expandAccountServiceStub struct {
	item *service.ExpandAccount
	err  error
}

func (s *expandAccountServiceStub) ListExpandAccounts(ctx context.Context, page, pageSize int, filters service.ExpandAccountListFilters) ([]service.ExpandAccount, int64, error) {
	panic("unexpected call")
}

func (s *expandAccountServiceStub) GetExpandAccount(ctx context.Context, id int64) (*service.ExpandAccount, error) {
	panic("unexpected call")
}

func (s *expandAccountServiceStub) CreateExpandAccount(ctx context.Context, input *service.ExpandAccountCreateInput) (*service.ExpandAccount, error) {
	panic("unexpected call")
}

func (s *expandAccountServiceStub) UpdateExpandAccount(ctx context.Context, id int64, input *service.ExpandAccountUpdateInput) (*service.ExpandAccount, error) {
	panic("unexpected call")
}

func (s *expandAccountServiceStub) DeleteExpandAccount(ctx context.Context, id int64) error {
	panic("unexpected call")
}

func (s *expandAccountServiceStub) MarkExpandAccountUsed(ctx context.Context, id int64) (*service.ExpandAccount, error) {
	panic("unexpected call")
}

func (s *expandAccountServiceStub) GetAndMarkExpandAccountByPlatform(ctx context.Context, platform string) (*service.ExpandAccount, error) {
	return s.item, s.err
}

func (s *expandAccountServiceStub) ReportExpandAccountLogin(ctx context.Context, input *service.ExpandAccountReportInput) (*service.ExpandAccount, error) {
	panic("unexpected call")
}

func TestExpandAccountHandlerGetByPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 6, 2, 3, 3, 20, 0, time.UTC)
	expandSvc := &expandAccountServiceStub{
		item: &service.ExpandAccount{
			ID:               3,
			Email:            "test1@example.com",
			Platform:         "openai",
			SubscriptionType: "pro",
			Country:          "US",
			SessionKey:       "test-session-key-001",
			ProxyID:          ptrInt64(40),
			ProxyInfo: &service.ProxyInfo{
				Protocol: "socks5",
				Host:     "154.63.48.107",
				Port:     7778,
				Username: "a3p3p1Q0o5j8",
				Password: "m5N7v5T9s9h4",
			},
			Used:      true,
			AccountID: ptrInt64(77),
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	latencyMs := int64(147)
	qualityScore := 80
	qualityChecked := int64(1780327184)
	adminSvc := newStubAdminService()
	adminSvc.proxyCounts = []service.ProxyWithAccountCount{
		{
			Proxy: service.Proxy{
				ID:        40,
				Name:      "rw-纯净渠道-美国3",
				Protocol:  "socks5",
				Host:      "154.63.48.107",
				Port:      7778,
				Username:  "a3p3p1Q0o5j8",
				Password:  "m5N7v5T9s9h4",
				Status:    service.StatusActive,
				CreatedAt: now,
				UpdatedAt: now,
			},
			AccountCount:   10,
			LatencyMs:      &latencyMs,
			LatencyStatus:  "success",
			LatencyMessage: "通过 2 项，告警 2 项，失败 0 项，挑战 0 项",
			IPAddress:      "154.63.48.107",
			Country:        "美国",
			CountryCode:    "US",
			Region:         "伊利诺伊州",
			City:           "芝加哥",
			QualityStatus:  "warn",
			QualityScore:   &qualityScore,
			QualityGrade:   "B",
			QualitySummary: "通过 2 项，告警 2 项，失败 0 项，挑战 0 项",
			QualityChecked: &qualityChecked,
		},
	}

	h := NewExpandAccountHandler(expandSvc, adminSvc, nil)
	router := gin.New()
	router.POST("/api/v1/expand-accounts/get", h.GetByPlatform)

	reqBody := []byte(`{"platform":"Anthropic"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/expand-accounts/get", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			ID               int64                           `json:"id"`
			Email            string                          `json:"email"`
			Platform         string                          `json:"platform"`
			SubscriptionType string                          `json:"subscription_type"`
			Country          string                          `json:"country"`
			SessionKey       string                          `json:"session_key"`
			ProxyID          *int64                          `json:"proxy_id"`
			ProxyInfo        *service.ProxyInfo              `json:"proxy_info"`
			Proxy            *dto.AdminProxyWithAccountCount `json:"proxy"`
			AccountID        *int64                          `json:"account_id"`
			CreatedAt        string                          `json:"created_at"`
			UpdatedAt        string                          `json:"updated_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "success", resp.Message)
	require.Equal(t, int64(3), resp.Data.ID)
	require.Equal(t, "test1@example.com", resp.Data.Email)
	require.Equal(t, "openai", resp.Data.Platform)
	require.NotNil(t, resp.Data.ProxyID)
	require.Equal(t, int64(40), *resp.Data.ProxyID)
	require.NotNil(t, resp.Data.ProxyInfo)
	require.Equal(t, "socks5", resp.Data.ProxyInfo.Protocol)
	require.NotNil(t, resp.Data.Proxy)
	require.Equal(t, int64(40), resp.Data.Proxy.ID)
	require.Equal(t, "rw-纯净渠道-美国3", resp.Data.Proxy.Name)
	require.Equal(t, "m5N7v5T9s9h4", resp.Data.Proxy.Password)
	require.Equal(t, int64(10), resp.Data.Proxy.AccountCount)
	require.Equal(t, "success", resp.Data.Proxy.LatencyStatus)
	require.Equal(t, "warn", resp.Data.Proxy.QualityStatus)
	require.NotNil(t, resp.Data.AccountID)
	require.Equal(t, int64(77), *resp.Data.AccountID)
	require.Equal(t, now.Format(time.RFC3339), resp.Data.CreatedAt)
	require.Equal(t, now.Format(time.RFC3339), resp.Data.UpdatedAt)
}

func ptrInt64(v int64) *int64 {
	return &v
}

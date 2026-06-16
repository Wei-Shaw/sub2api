package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// spyUserMonitorSvc 捕获 Snapshot 调用入参，便于断言 user scope 注入正确。
type spyUserMonitorSvc struct {
	lastFilter service.MonitorSnapshotFilter
	calls      int
}

func (s *spyUserMonitorSvc) Snapshot(_ context.Context, filter service.MonitorSnapshotFilter) (*service.MonitorSnapshot, error) {
	s.calls++
	s.lastFilter = filter
	return &service.MonitorSnapshot{Enabled: true, Items: []service.LimiterRuntime{}}, nil
}

func setupUserHandler(svc service.ServiceQuotaMonitorService, subject *middleware.AuthSubject) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	h := NewUserServiceQuotaHandler(svc)
	r := gin.New()
	r.GET("/my", func(c *gin.Context) {
		if subject != nil {
			c.Set(string(middleware.ContextKeyUser), *subject)
		}
		h.MyQuota(c)
	})
	return r, httptest.NewRecorder()
}

func TestMyQuota_NoAuthSubject_401(t *testing.T) {
	spy := &spyUserMonitorSvc{}
	r, rec := setupUserHandler(spy, nil)
	req := httptest.NewRequest(http.MethodGet, "/my", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, 0, spy.calls)
}

func TestMyQuota_InjectsUserID(t *testing.T) {
	spy := &spyUserMonitorSvc{}
	r, rec := setupUserHandler(spy, &middleware.AuthSubject{UserID: 7})
	req := httptest.NewRequest(http.MethodGet, "/my", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, spy.calls)
	require.NotNil(t, spy.lastFilter.UserScope)
	require.Equal(t, int64(7), spy.lastFilter.UserScope.UserID)
}

func TestMyQuota_IgnoresAdminQueryParams(t *testing.T) {
	spy := &spyUserMonitorSvc{}
	r, rec := setupUserHandler(spy, &middleware.AuthSubject{UserID: 7})
	req := httptest.NewRequest(http.MethodGet, "/my?channel_id=999&platform=openai", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	// 验证 admin 字段未被透传
	require.Nil(t, spy.lastFilter.ChannelID)
	require.Nil(t, spy.lastFilter.Platform)
	require.NotNil(t, spy.lastFilter.UserScope)
}

func TestMyQuota_NilSvc_404(t *testing.T) {
	r, rec := setupUserHandler(nil, &middleware.AuthSubject{UserID: 7})
	req := httptest.NewRequest(http.MethodGet, "/my", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
	// nil svc 由于响应 body 不是固定结构，仅断言状态码即可
	_ = json.NewDecoder(rec.Body).Decode(&map[string]any{})
}

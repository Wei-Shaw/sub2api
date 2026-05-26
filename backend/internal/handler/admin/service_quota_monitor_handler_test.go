package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// spyMonitorSvc 捕获 Snapshot 调用入参，便于断言 query 解析正确。
type spyMonitorSvc struct {
	lastFilter service.MonitorSnapshotFilter
	calls      int
	resp       *service.MonitorSnapshot
	err        error
}

func (s *spyMonitorSvc) Snapshot(_ context.Context, filter service.MonitorSnapshotFilter) (*service.MonitorSnapshot, error) {
	s.calls++
	s.lastFilter = filter
	if s.resp == nil && s.err == nil {
		return &service.MonitorSnapshot{Enabled: true, Items: []service.LimiterRuntime{}}, nil
	}
	return s.resp, s.err
}

func setupMonitorHandler(svc service.ServiceQuotaMonitorService) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	h := NewServiceQuotaMonitorHandler(svc)
	r := gin.New()
	r.GET("/monitor", h.Snapshot)
	return r, httptest.NewRecorder()
}

func TestSnapshot_QueryParseSuccess(t *testing.T) {
	spy := &spyMonitorSvc{}
	r, rec := setupMonitorHandler(spy)
	req := httptest.NewRequest(http.MethodGet,
		"/monitor?rule_id=10&user_id=20&channel_id=30&group_id=40&account_id=50&platform=openai", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, spy.calls)
	require.NotNil(t, spy.lastFilter.RuleID)
	require.Equal(t, int64(10), *spy.lastFilter.RuleID)
	require.NotNil(t, spy.lastFilter.UserID)
	require.Equal(t, int64(20), *spy.lastFilter.UserID)
	require.NotNil(t, spy.lastFilter.ChannelID)
	require.Equal(t, int64(30), *spy.lastFilter.ChannelID)
	require.NotNil(t, spy.lastFilter.GroupID)
	require.Equal(t, int64(40), *spy.lastFilter.GroupID)
	require.NotNil(t, spy.lastFilter.AccountID)
	require.Equal(t, int64(50), *spy.lastFilter.AccountID)
	require.NotNil(t, spy.lastFilter.Platform)
	require.Equal(t, "openai", *spy.lastFilter.Platform)
}

func TestSnapshot_InvalidRuleID_400(t *testing.T) {
	spy := &spyMonitorSvc{}
	r, rec := setupMonitorHandler(spy)
	req := httptest.NewRequest(http.MethodGet, "/monitor?rule_id=abc", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, spy.calls) // service 不应被调用
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "INVALID_QUERY_PARAM", body["reason"])
}

func TestSnapshot_NilSvc_404(t *testing.T) {
	r, rec := setupMonitorHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/monitor", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSnapshot_EmptyResult_OK(t *testing.T) {
	spy := &spyMonitorSvc{
		resp: &service.MonitorSnapshot{Enabled: true, Items: []service.LimiterRuntime{}},
	}
	r, rec := setupMonitorHandler(spy)
	req := httptest.NewRequest(http.MethodGet, "/monitor", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, data["enabled"])
	items, ok := data["items"].([]any)
	require.True(t, ok)
	require.Empty(t, items)
}

func TestSnapshot_SvcError_500(t *testing.T) {
	spy := &spyMonitorSvc{err: errors.New("boom")}
	r, rec := setupMonitorHandler(spy)
	req := httptest.NewRequest(http.MethodGet, "/monitor", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

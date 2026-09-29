package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dashboardAccountWindowsProbe struct {
	service.UsageLogRepository
	calls   int
	windows []usagestats.AccountStatsWindow
	err     error
	ctx     context.Context
}

func (r *dashboardAccountWindowsProbe) GetAccountStatsByWindows(ctx context.Context, windows []usagestats.AccountStatsWindow) (map[int64]*usagestats.AccountStats, error) {
	r.calls++
	r.windows = windows
	r.ctx = ctx
	if r.err != nil {
		return nil, r.err
	}
	result := make(map[int64]*usagestats.AccountStats, len(windows))
	for _, window := range windows {
		result[window.AccountID] = &usagestats.AccountStats{Requests: 2, Tokens: 300, Cost: 1.5, StandardCost: 3, UserCost: 4}
	}
	return result, nil
}

func accountWindowsTestRouter(repo *dashboardAccountWindowsProbe) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewDashboardHandler(service.NewDashboardService(repo, nil, nil, nil), nil)
	r := gin.New()
	r.POST("/admin/dashboard/account-stats/batch", h.GetBatchAccountWindowStats)
	return r
}

const validAccountWindowsBody = `{"windows":[
	{"account_id":18,"start_at":"2026-09-21T12:34:56.123456Z","end_at":"2026-09-28T12:34:56.123456Z"},
	{"account_id":19,"start_at":"2026-09-24T08:30:00+08:00","end_at":"2026-09-28T12:34:56.123456Z"}
]}`

func TestDashboardHandlerBatchAccountWindows(t *testing.T) {
	repo := &dashboardAccountWindowsProbe{}
	router := accountWindowsTestRouter(repo)
	req := httptest.NewRequest(http.MethodPost, "/admin/dashboard/account-stats/batch", strings.NewReader(validAccountWindowsBody))
	req.Header.Set("Content-Type", "application/json")
	// The endpoint must retain a caller deadline shorter than its own cap.
	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(ctx))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Equal(t, 1, repo.calls)
	require.Len(t, repo.windows, 2)
	require.Equal(t, 123456000, repo.windows[0].StartAt.Nanosecond())
	require.Equal(t, time.Date(2026, 9, 24, 0, 30, 0, 0, time.UTC), repo.windows[1].StartAt.UTC())
	callerDeadline, _ := ctx.Deadline()
	queryDeadline, ok := repo.ctx.Deadline()
	require.True(t, ok)
	require.Equal(t, callerDeadline, queryDeadline)
	var body struct {
		Code int `json:"code"`
		Data struct {
			Stats map[int64]*usagestats.AccountStats `json:"stats"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Zero(t, body.Code)
	require.Equal(t, &usagestats.AccountStats{Requests: 2, Tokens: 300, Cost: 1.5, StandardCost: 3, UserCost: 4}, body.Data.Stats[18])
	require.Len(t, body.Data.Stats, 2)
}

func TestDashboardHandlerBatchAccountWindowsRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		code int
	}{
		{"missing", `{}`, 400},
		{"empty", `{"windows":[]}`, 400},
		{"null", `null`, 400},
		{"malformed", `{`, 400},
		{"trailing JSON", validAccountWindowsBody + `{}`, 400},
		{"unknown field", `{"window":[]}`, 400},
		{"wrong nested field", strings.Replace(validAccountWindowsBody, `"start_at"`, `"start"`, 1), 400},
		{"bad timestamp", strings.Replace(validAccountWindowsBody, "2026-09-21T12:34:56.123456Z", "yesterday", 1), 400},
		{"bad ID", strings.Replace(validAccountWindowsBody, `"account_id":18`, `"account_id":0`, 1), 400},
		{"duplicate ID", strings.Replace(validAccountWindowsBody, `"account_id":19`, `"account_id":18`, 1), 400},
		{"missing start", `{"windows":[{"account_id":1,"end_at":"2026-09-28T00:00:00Z"}]}`, 400},
		{"reversed", `{"windows":[{"account_id":1,"start_at":"2026-09-28T00:00:00Z","end_at":"2026-09-27T00:00:00Z"}]}`, 400},
		{"too long", `{"windows":[{"account_id":1,"start_at":"2026-07-01T00:00:00Z","end_at":"2026-09-28T00:00:00Z"}]}`, 400},
		{"oversized", strings.Repeat(" ", accountStatsBatchBodyLimit) + validAccountWindowsBody, 413},
		{"oversized trailing", validAccountWindowsBody + strings.Repeat(" ", accountStatsBatchBodyLimit), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &dashboardAccountWindowsProbe{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/admin/dashboard/account-stats/batch", strings.NewReader(tc.body))
			accountWindowsTestRouter(repo).ServeHTTP(rec, req)
			require.Equal(t, tc.code, rec.Code, rec.Body.String())
			require.Zero(t, repo.calls)
		})
	}
}

func TestDashboardHandlerBatchAccountWindowsQueryFailure(t *testing.T) {
	repo := &dashboardAccountWindowsProbe{err: errors.New("database diagnostic")}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/dashboard/account-stats/batch", strings.NewReader(validAccountWindowsBody))
	start := time.Now()
	accountWindowsTestRouter(repo).ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "database diagnostic")
	require.NotContains(t, rec.Body.String(), `"stats"`)
	deadline, ok := repo.ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, start.Add(30*time.Second), deadline, time.Second)
}

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type adminVideoUsageRepo struct {
	service.UsageLogRepository
	log *service.UsageLog
}

func (r *adminVideoUsageRepo) GetByID(context.Context, int64) (*service.UsageLog, error) {
	return r.log, nil
}

func TestAdminVideoPreviewCookieAndRetention(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := &service.UsageLog{ID: 12, RequestID: "grok-video:wan_123", CreatedAt: time.Now().Add(-time.Hour), VideoCount: 1}
	repo := &adminVideoUsageRepo{log: log}
	h := NewUsageHandler(service.NewUsageService(repo, nil, nil, nil), nil, nil, nil, nil)
	r := gin.New()
	r.GET("/api/v1/admin/usage/:id/videos/:index", h.Video)
	for _, tc := range []struct {
		path    string
		expired bool
		status  int
	}{
		{"/api/v1/admin/usage/12/videos/0", false, 200},
		{"/api/v1/admin/usage/12/videos/1", false, 404},
		{"/api/v1/admin/usage/12/videos/-1", false, 404},
		{"/api/v1/admin/usage/bad/videos/0", false, 404},
		{"/api/v1/admin/usage/12/videos/0", true, 404},
	} {
		log.CreatedAt = time.Now().Add(-time.Hour)
		if tc.expired {
			log.CreatedAt = time.Now().Add(-8 * 24 * time.Hour)
		}
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Authorization", "Bearer test-admin-session")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code)
		if tc.status == 200 {
			require.Equal(t, "/api/v1/admin/usage/12/videos/0/content", w.Body.String())
			cookies := w.Result().Cookies()
			require.Len(t, cookies, 1)
			require.True(t, cookies[0].HttpOnly)
			require.True(t, cookies[0].Secure)
			require.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)
			require.Equal(t, 300, cookies[0].MaxAge)
			require.Equal(t, w.Body.String(), cookies[0].Path)
		} else {
			require.Empty(t, w.Result().Cookies())
		}
	}
}

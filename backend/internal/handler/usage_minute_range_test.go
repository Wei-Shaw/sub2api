package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMinuteRangeUserMetadataAndValidation(t *testing.T) {
	repo := &userUsageRepoCapture{}
	router := newUserUsageRequestTypeTestRouter(repo)
	h := NewUsageHandler(service.NewUsageService(repo, nil, nil, nil), nil, nil, nil)
	router.GET("/usage/dashboard/trend", h.DashboardTrend)
	for _, route := range []string{"/usage/stats", "/usage/dashboard/trend", "/usage/dashboard/models", "/usage/dashboard/snapshot-v2"} {
		for _, tc := range []struct{ query, start, end, date string }{
			{"start_date=2026-09-10T01:02:03.123456789Z&end_date=2026-09-10T01:03:03.123456789Z", "2026-09-10T01:02:03.123456789Z", "2026-09-10T01:03:03.123456789Z", "2026-09-10"},
			{"start_date=2026-03-08&end_date=2026-03-08&timezone=America%2FNew_York", "2026-03-08T00:00:00-05:00", "2026-03-09T00:00:00-04:00", "2026-03-08"},
		} {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("GET", route+"?"+tc.query+"&include_group_stats=true", nil))
			require.Equal(t, 200, rec.Code, rec.Body.String())
			var body struct{ Data map[string]any }
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, tc.start, body.Data["start_time"])
			require.Equal(t, tc.end, body.Data["end_time"])
			if route != "/usage/stats" {
				require.Equal(t, tc.date, body.Data["end_date"])
			}
		}
	}
	for _, route := range []string{"/usage", "/usage/stats", "/usage/dashboard/models", "/usage/dashboard/snapshot-v2"} {
		for _, query := range []string{"start_date=bad", "end_date=bad", "start_date=2026-09-10&end_date=2026-09-08", "start_date=2026-09-10T00:00:00Z&end_date=2026-09-10T00:00:00Z"} {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("GET", route+"?"+query, nil))
			require.Equal(t, 400, rec.Code, route+"?"+query)
		}
	}
}

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseUserTimeRangeLast24Hours(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?period=last24hours&timezone=UTC", nil)

	before := time.Now().UTC()
	start, end := parseUserTimeRange(c)
	after := time.Now().UTC()

	require.WithinDuration(t, before.Add(-24*time.Hour), start, 2*time.Second)
	require.WithinDuration(t, after, end, 2*time.Second)
	require.Equal(t, 24*time.Hour, end.Sub(start))
}

func TestUserTimeRangeResponseMetadataLast24Hours(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?period=24h&timezone=UTC", nil)

	before := time.Now().UTC()
	start, end := parseUserTimeRange(c)
	after := time.Now().UTC()
	metadata := newTimeRangeResponseMetadata(c, start, end)

	require.Equal(t, "last24hours", metadata.Period)
	require.WithinDuration(t, before.Add(-24*time.Hour), start, 2*time.Second)
	require.WithinDuration(t, after, end, 2*time.Second)
	require.Equal(t, start.Format(time.RFC3339), metadata.StartTime)
	require.Equal(t, end.Format(time.RFC3339), metadata.EndTime)
	require.Equal(t, end.Format("2006-01-02"), metadata.EndDate)
}

func TestUserTimeRangeResponseMetadataDateRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?start_date=2024-01-01&end_date=2024-01-02&timezone=UTC", nil)

	start, end := parseUserTimeRange(c)
	metadata := newTimeRangeResponseMetadata(c, start, end)

	require.Equal(t, "2024-01-01", metadata.StartDate)
	require.Equal(t, "2024-01-02", metadata.EndDate)
	require.Empty(t, metadata.Period)
	require.Equal(t, "2024-01-01T00:00:00Z", metadata.StartTime)
	require.Equal(t, "2024-01-03T00:00:00Z", metadata.EndTime)
}

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

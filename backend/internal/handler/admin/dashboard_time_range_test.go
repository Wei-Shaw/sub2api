package admin

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestDashboardRollingTimeRangePreservesBoundaries(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?start_date=2026-09-09T06:30:00Z&end_date=2026-09-10T06:30:00Z&timezone=Asia%2FTokyo", nil)
	start, end := parseTimeRange(c)
	if start.Format(time.RFC3339) != "2026-09-09T06:30:00Z" || end.Sub(start) != 24*time.Hour {
		t.Fatalf("rolling range expanded: %v to %v", start, end)
	}
	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?start_date=2026-03-08&end_date=2026-03-08&timezone=America%2FNew_York", nil)
	start, end = parseTimeRange(c)
	if end.Sub(start) != 23*time.Hour {
		t.Fatalf("calendar range must respect DST: %v to %v", start, end)
	}
}

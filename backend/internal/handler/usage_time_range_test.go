package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func TestUserUsageRollingRangeAcrossQueryModes(t *testing.T) {
	for _, required := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Request = httptest.NewRequest("GET", "/?start_date=2026-09-09T06:30:00Z&end_date=2026-09-10T06:30:00Z", nil)
		parsed, ok := (&UsageHandler{}).parseUserUsageFilters(c, required)
		if !ok {
			t.Fatal("precise timestamps rejected")
		}
		if parsed.EndTime.Sub(parsed.StartTime) != 24*time.Hour || parsed.StartTime.Hour() != 6 {
			t.Fatalf("range expanded: %v to %v", parsed.StartTime, parsed.EndTime)
		}
	}
}

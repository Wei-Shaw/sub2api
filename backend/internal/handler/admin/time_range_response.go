package admin

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/gin-gonic/gin"
)

type timeRangeResponseMetadata struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	StartTime string `json:"start_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`
	Period    string `json:"period,omitempty"`
}

func newTimeRangeResponseMetadata(c *gin.Context, startTime, endTime time.Time) timeRangeResponseMetadata {
	metadata := timeRangeResponseMetadata{
		StartDate: startTime.Format("2006-01-02"),
		EndDate:   formatResponseEndDate(c, endTime),
		StartTime: startTime.Format(time.RFC3339),
		EndTime:   endTime.Format(time.RFC3339),
	}

	if period := normalizeResponsePeriod(c); period != "" {
		metadata.Period = period
	}

	return metadata
}

func addTimeRangeResponseMetadata(payload gin.H, c *gin.Context, startTime, endTime time.Time) gin.H {
	metadata := newTimeRangeResponseMetadata(c, startTime, endTime)
	payload["start_date"] = metadata.StartDate
	payload["end_date"] = metadata.EndDate
	payload["start_time"] = metadata.StartTime
	payload["end_time"] = metadata.EndTime
	if metadata.Period != "" {
		payload["period"] = metadata.Period
	}
	return payload
}

func formatResponseEndDate(c *gin.Context, endTime time.Time) string {
	if c != nil && c.Query("start_date") == "" && c.Query("end_date") == "" && timezone.IsLast24HoursPeriod(c.Query("period")) {
		return endTime.Format("2006-01-02")
	}
	return endTime.Add(-time.Nanosecond).Format("2006-01-02")
}

func normalizeResponsePeriod(c *gin.Context) string {
	if c == nil {
		return ""
	}
	period := strings.ToLower(strings.TrimSpace(c.Query("period")))
	if period == "" {
		return ""
	}
	if timezone.IsLast24HoursPeriod(period) {
		return "last24hours"
	}
	return period
}

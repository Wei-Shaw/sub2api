package admin

import (
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

func parseUsageViewerAccountSummaryRange(c *gin.Context) (time.Time, time.Time, bool) {
	userTZ := c.Query("timezone")
	now := timezone.NowInUserLocation(userTZ)
	startTime := timezone.StartOfDayInUserLocation(now.AddDate(0, 0, -1), userTZ)
	endTime := timezone.StartOfDayInUserLocation(now.AddDate(0, 0, 1), userTZ)

	if rawStart := strings.TrimSpace(c.Query("start_date")); rawStart != "" {
		parsed, err := timezone.ParseInUserLocation("2006-01-02", rawStart, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid start_date format, use YYYY-MM-DD")
			return time.Time{}, time.Time{}, false
		}
		startTime = parsed
	}
	if rawEnd := strings.TrimSpace(c.Query("end_date")); rawEnd != "" {
		parsed, err := timezone.ParseInUserLocation("2006-01-02", rawEnd, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
			return time.Time{}, time.Time{}, false
		}
		endTime = parsed.AddDate(0, 0, 1)
	}
	return startTime, endTime, true
}

func parseUsageViewerAccountSummaryGranularity(c *gin.Context) (string, bool) {
	granularity := strings.TrimSpace(c.DefaultQuery("granularity", "hour"))
	if granularity == "day" || granularity == "hour" {
		return granularity, true
	}
	response.BadRequest(c, "Invalid granularity, use day or hour")
	return "", false
}

func parseUsageViewerAccountSummaryModelSource(c *gin.Context) (string, bool) {
	source := strings.TrimSpace(c.DefaultQuery("model_source", usagestats.ModelSourceRequested))
	if !usagestats.IsValidModelSource(source) {
		response.BadRequest(c, "Invalid model_source, use requested/upstream/mapping")
		return "", false
	}
	return usagestats.NormalizeModelSource(source), true
}

// GetUsageViewerSummary returns aggregated usage charts for accounts assigned to a usage_viewer.
// GET /api/v1/admin/accounts/usage-viewer-summary
func (h *AccountHandler) GetUsageViewerSummary(c *gin.Context) {
	allowedIDs, scoped, err := h.usageViewerAllowedAccountIDs(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !scoped {
		response.ErrorFrom(c, infraerrors.Forbidden("USAGE_VIEWER_ONLY", "usage viewer account is required"))
		return
	}
	if h.accountUsageService == nil {
		response.InternalError(c, "Account usage service is not configured")
		return
	}

	startTime, endTime, ok := parseUsageViewerAccountSummaryRange(c)
	if !ok {
		return
	}
	granularity, ok := parseUsageViewerAccountSummaryGranularity(c)
	if !ok {
		return
	}
	modelSource, ok := parseUsageViewerAccountSummaryModelSource(c)
	if !ok {
		return
	}

	if len(allowedIDs) == 0 {
		response.Success(c, gin.H{
			"models":      []usagestats.ModelStat{},
			"trend":       []usagestats.TrendDataPoint{},
			"start_date":  startTime.Format("2006-01-02"),
			"end_date":    endTime.AddDate(0, 0, -1).Format("2006-01-02"),
			"granularity": granularity,
		})
		return
	}

	g, gctx := errgroup.WithContext(c.Request.Context())
	var models []usagestats.ModelStat
	var trend []usagestats.TrendDataPoint
	g.Go(func() error {
		var modelErr error
		models, modelErr = h.accountUsageService.GetAccountsModelStats(gctx, allowedIDs, startTime, endTime, modelSource)
		return modelErr
	})
	g.Go(func() error {
		var trendErr error
		trend, trendErr = h.accountUsageService.GetAccountsUsageTrend(gctx, allowedIDs, startTime, endTime, granularity)
		return trendErr
	})
	if err := g.Wait(); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"models":      models,
		"trend":       trend,
		"start_date":  startTime.Format("2006-01-02"),
		"end_date":    endTime.AddDate(0, 0, -1).Format("2006-01-02"),
		"granularity": granularity,
	})
}

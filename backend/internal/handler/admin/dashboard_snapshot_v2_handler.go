package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

var dashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)

type dashboardSnapshotV2Stats struct {
	usagestats.DashboardStats
	Uptime int64 `json:"uptime"`
}

type dashboardSnapshotV2Response struct {
	GeneratedAt string `json:"generated_at"`

	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Granularity string `json:"granularity"`

	Stats                  *dashboardSnapshotV2Stats            `json:"stats,omitempty"`
	Trend                  []usagestats.TrendDataPoint          `json:"trend,omitempty"`
	Models                 []usagestats.ModelStat               `json:"models,omitempty"`
	Groups                 []usagestats.GroupStat               `json:"groups,omitempty"`
	UsersTrend             []usagestats.UserUsageTrendPoint     `json:"users_trend,omitempty"`
	Ranking                []usagestats.UserSpendingRankingItem `json:"ranking,omitempty"`
	RankingTotalActualCost float64                              `json:"ranking_total_actual_cost,omitempty"`
	RankingTotalRequests   int64                                `json:"ranking_total_requests,omitempty"`
	RankingTotalTokens     int64                                `json:"ranking_total_tokens,omitempty"`
}

type dashboardSnapshotV2CacheKey struct {
	StartTime         string `json:"start_time"`
	EndTime           string `json:"end_time"`
	Granularity       string `json:"granularity"`
	UserID            int64  `json:"user_id"`
	APIKeyID          int64  `json:"api_key_id"`
	AccountID         int64  `json:"account_id"`
	GroupID           int64  `json:"group_id"`
	Model             string `json:"model"`
	RequestType       *int16 `json:"request_type"`
	Stream            *bool  `json:"stream"`
	BillingType       *int8  `json:"billing_type"`
	IncludeStats      bool   `json:"include_stats"`
	IncludeTrend      bool   `json:"include_trend"`
	IncludeModels     bool   `json:"include_models"`
	IncludeGroups     bool   `json:"include_groups"`
	IncludeUsersTrend bool   `json:"include_users_trend"`
	UsersTrendLimit   int    `json:"users_trend_limit"`
	IncludeRanking    bool   `json:"include_ranking"`
	RankingLimit      int    `json:"ranking_limit"`
}

func (h *DashboardHandler) GetSnapshotV2(c *gin.Context) {
	startTime, endTime := parseTimeRange(c)
	granularity := strings.TrimSpace(c.DefaultQuery("granularity", "day"))
	if granularity != "hour" {
		granularity = "day"
	}

	includeStats := parseBoolQueryWithDefault(c.Query("include_stats"), true)
	includeTrend := parseBoolQueryWithDefault(c.Query("include_trend"), true)
	includeModels := parseBoolQueryWithDefault(c.Query("include_model_stats"), true)
	includeGroups := parseBoolQueryWithDefault(c.Query("include_group_stats"), false)
	includeUsersTrend := parseBoolQueryWithDefault(c.Query("include_users_trend"), false)
	includeRanking := parseBoolQueryWithDefault(c.Query("include_user_ranking"), false)
	usersTrendLimit := 12
	if raw := strings.TrimSpace(c.Query("users_trend_limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 50 {
			usersTrendLimit = parsed
		}
	}
	rankingLimit := 12
	if raw := strings.TrimSpace(c.Query("user_ranking_limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 50 {
			rankingLimit = parsed
		}
	}

	filters, err := parseDashboardSnapshotV2Filters(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	keyRaw, _ := json.Marshal(dashboardSnapshotV2CacheKey{
		StartTime:         startTime.UTC().Format(time.RFC3339),
		EndTime:           endTime.UTC().Format(time.RFC3339),
		Granularity:       granularity,
		UserID:            filters.UserID,
		APIKeyID:          filters.APIKeyID,
		AccountID:         filters.AccountID,
		GroupID:           filters.GroupID,
		Model:             filters.Model,
		RequestType:       filters.RequestType,
		Stream:            filters.Stream,
		BillingType:       filters.BillingType,
		IncludeStats:      includeStats,
		IncludeTrend:      includeTrend,
		IncludeModels:     includeModels,
		IncludeGroups:     includeGroups,
		IncludeUsersTrend: includeUsersTrend,
		UsersTrendLimit:   usersTrendLimit,
		IncludeRanking:    includeRanking,
		RankingLimit:      rankingLimit,
	})
	cacheKey := string(keyRaw)
	query := service.DashboardSnapshotQuery{
		StartTime:         startTime,
		EndTime:           endTime,
		Granularity:       granularity,
		Filters:           filters,
		IncludeStats:      includeStats,
		IncludeTrend:      includeTrend,
		IncludeModels:     includeModels,
		IncludeGroups:     includeGroups,
		IncludeUsersTrend: includeUsersTrend,
		IncludeRanking:    includeRanking,
		UsersTrendLimit:   usersTrendLimit,
		RankingLimit:      rankingLimit,
	}

	cached, hit, err := dashboardSnapshotV2Cache.GetOrLoad(cacheKey, func() (any, error) {
		return h.buildSnapshotV2Response(c.Request.Context(), query)
	})
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if cached.ETag != "" {
		c.Header("ETag", cached.ETag)
		c.Header("Vary", "If-None-Match")
		if ifNoneMatchMatched(c.GetHeader("If-None-Match"), cached.ETag) {
			c.Status(http.StatusNotModified)
			return
		}
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))
	response.Success(c, cached.Payload)
}

func (h *DashboardHandler) buildSnapshotV2Response(
	ctx context.Context,
	query service.DashboardSnapshotQuery,
) (*dashboardSnapshotV2Response, error) {
	snapshot, err := h.dashboardService.GetDashboardSnapshot(ctx, query)
	if err != nil {
		return nil, err
	}
	resp := &dashboardSnapshotV2Response{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		StartDate:   query.StartTime.Format("2006-01-02"),
		EndDate:     query.EndTime.Add(-24 * time.Hour).Format("2006-01-02"),
		Granularity: query.Granularity,
		Trend:       snapshot.Trend,
		Models:      snapshot.Models,
		Groups:      snapshot.Groups,
	}
	if snapshot.Stats != nil {
		resp.Stats = &dashboardSnapshotV2Stats{
			DashboardStats: *snapshot.Stats,
			Uptime:         int64(time.Since(h.startTime).Seconds()),
		}
	}
	if snapshot.UserInsights != nil {
		if query.IncludeUsersTrend {
			resp.UsersTrend = snapshot.UserInsights.Trend
		}
		if query.IncludeRanking {
			resp.Ranking = snapshot.UserInsights.Ranking.Ranking
			resp.RankingTotalActualCost = snapshot.UserInsights.Ranking.TotalActualCost
			resp.RankingTotalRequests = snapshot.UserInsights.Ranking.TotalRequests
			resp.RankingTotalTokens = snapshot.UserInsights.Ranking.TotalTokens
		}
	}
	return resp, nil
}

func parseDashboardSnapshotV2Filters(c *gin.Context) (service.DashboardSnapshotFilters, error) {
	filters := service.DashboardSnapshotFilters{
		Model: strings.TrimSpace(c.Query("model")),
	}

	if userIDStr := strings.TrimSpace(c.Query("user_id")); userIDStr != "" {
		id, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			return service.DashboardSnapshotFilters{}, err
		}
		filters.UserID = id
	}
	if apiKeyIDStr := strings.TrimSpace(c.Query("api_key_id")); apiKeyIDStr != "" {
		id, err := strconv.ParseInt(apiKeyIDStr, 10, 64)
		if err != nil {
			return service.DashboardSnapshotFilters{}, err
		}
		filters.APIKeyID = id
	}
	if accountIDStr := strings.TrimSpace(c.Query("account_id")); accountIDStr != "" {
		id, err := strconv.ParseInt(accountIDStr, 10, 64)
		if err != nil {
			return service.DashboardSnapshotFilters{}, err
		}
		filters.AccountID = id
	}
	if groupIDStr := strings.TrimSpace(c.Query("group_id")); groupIDStr != "" {
		id, err := strconv.ParseInt(groupIDStr, 10, 64)
		if err != nil {
			return service.DashboardSnapshotFilters{}, err
		}
		filters.GroupID = id
	}

	if requestTypeStr := strings.TrimSpace(c.Query("request_type")); requestTypeStr != "" {
		parsed, err := service.ParseUsageRequestType(requestTypeStr)
		if err != nil {
			return service.DashboardSnapshotFilters{}, err
		}
		value := int16(parsed)
		filters.RequestType = &value
	} else if streamStr := strings.TrimSpace(c.Query("stream")); streamStr != "" {
		streamVal, err := strconv.ParseBool(streamStr)
		if err != nil {
			return service.DashboardSnapshotFilters{}, err
		}
		filters.Stream = &streamVal
	}

	if billingTypeStr := strings.TrimSpace(c.Query("billing_type")); billingTypeStr != "" {
		v, err := strconv.ParseInt(billingTypeStr, 10, 8)
		if err != nil {
			return service.DashboardSnapshotFilters{}, err
		}
		bt := int8(v)
		filters.BillingType = &bt
	}

	return filters, nil
}

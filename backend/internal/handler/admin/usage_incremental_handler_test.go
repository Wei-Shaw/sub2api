package admin

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type incrementalUsageStub struct {
	service.UsageLogRepository
	cursor service.UsageCursor
	limit  int
}

func (s *incrementalUsageStub) ListIncremental(_ context.Context, cursor service.UsageCursor, limit int) (*service.IncrementalUsagePage, error) {
	s.cursor = cursor
	s.limit = limit
	return &service.IncrementalUsagePage{Records: []service.UsageLog{{ID: 9, UserID: 7}}, NextCursor: service.UsageCursor{TransactionID: 42, ID: 9}, HasMore: true}, nil
}
func TestUsageIncrementalHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &incrementalUsageStub{}
	h := NewUsageHandler(service.NewUsageService(repo, nil, nil, nil), nil, nil, nil)
	router := gin.New()
	router.GET("/usage/incremental", h.Incremental)
	good := userKeyRequest(router, http.MethodGet, "/usage/incremental?cursor=v1:41:8&limit=2", "", "")
	require.Equal(t, http.StatusOK, good.Code)
	require.Equal(t, service.UsageCursor{TransactionID: 41, ID: 8}, repo.cursor)
	require.Equal(t, 2, repo.limit)
	require.Contains(t, good.Body.String(), `"next_cursor":"v1:42:9"`)
	require.Contains(t, good.Body.String(), `"has_more":true`)
	require.Contains(t, good.Body.String(), `"items":[`)
	for _, query := range []string{"cursor=invalid", "limit=0", "limit=1001", "limit=abc", "user_id=7", "start_date=2026-01-01"} {
		rec := userKeyRequest(router, http.MethodGet, "/usage/incremental?"+query, "", "")
		require.Equal(t, http.StatusBadRequest, rec.Code, query)
	}
}

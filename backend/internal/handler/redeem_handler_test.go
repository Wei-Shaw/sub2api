package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type historyRepo struct {
	service.RedeemCodeRepository
	userID      int64
	params      pagination.PaginationParams
	legacyLimit int
}

func (r *historyRepo) ListByUser(_ context.Context, userID int64, limit int) ([]service.RedeemCode, error) {
	r.userID, r.legacyLimit = userID, limit
	return []service.RedeemCode{}, nil
}
func (r *historyRepo) ListByUserPaginated(_ context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	r.userID, r.params = userID, params
	return []service.RedeemCode{}, &pagination.PaginationResult{Total: 101}, nil
}
func TestRedeemHistory(t *testing.T) {
	for _, tt := range []struct {
		name, query        string
		userID             int64
		status, page, size int
	}{
		{"legacy", "", 7, 200, 0, 0},
		{"default", "?page=1", 7, 200, 1, 20},
		{"size only", "?page_size=50", 7, 200, 1, 50},
		{"second user", "?page=2&page_size=100&user_id=7", 8, 200, 2, 100},
		{"cap", "?page_size=101", 7, 200, 1, 100},
		{"beyond last", "?page=100&page_size=20", 7, 200, 100, 20},
		{"zero", "?page=0", 7, 400, 0, 0},
		{"negative", "?page_size=-1", 7, 400, 0, 0},
		{"invalid", "?page=abc", 7, 400, 0, 0},
		{"overflow", "?page=9223372036854775807", 7, 400, 0, 0},
		{"unauthenticated", "?page=1", 0, 401, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &historyRepo{}
			h := NewRedeemHandler(service.NewRedeemService(repo, nil, nil, nil, nil, nil, nil, nil))
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/v1/redeem/history"+tt.query, nil)
			if tt.userID != 0 {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: tt.userID})
			}
			h.GetHistory(c)
			require.Equal(t, tt.status, w.Code)
			if tt.status != 200 {
				require.Zero(t, repo.userID)
				return
			}
			require.Equal(t, tt.userID, repo.userID)
			var body struct {
				Data json.RawMessage `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			if tt.query == "" {
				require.Equal(t, 25, repo.legacyLimit)
				require.JSONEq(t, "[]", string(body.Data))
				return
			}
			require.Equal(t, tt.page, repo.params.Page)
			require.Equal(t, tt.size, repo.params.PageSize)
			var data struct {
				Items []service.RedeemCode `json:"items"`
				Total int                  `json:"total"`
				Page  int                  `json:"page"`
				Size  int                  `json:"page_size"`
			}
			require.NoError(t, json.Unmarshal(body.Data, &data))
			require.NotNil(t, data.Items)
			require.Equal(t, 101, data.Total)
			require.Equal(t, tt.page, data.Page)
			require.Equal(t, tt.size, data.Size)
		})
	}
}

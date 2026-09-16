package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type accountPerformanceRepoStub struct {
	service.UsageLogRepository
	read func(context.Context, []int64, time.Time, time.Time) (map[int64]*service.AccountPerformanceStats, error)
}

func (r *accountPerformanceRepoStub) GetAccountPerformanceStatsBatch(ctx context.Context, ids []int64, start, end time.Time) (map[int64]*service.AccountPerformanceStats, error) {
	return r.read(ctx, ids, start, end)
}

func setupAccountPerformanceRouter(t *testing.T, repo service.UsageLogRepository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	originalCache := accountPerformanceBatchCache
	accountPerformanceBatchCache = newSnapshotCache(30 * time.Second)
	t.Cleanup(func() { accountPerformanceBatchCache = originalCache })
	svc := service.NewAccountUsageService(nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &AccountHandler{accountUsageService: svc}
	router := gin.New()
	router.POST("/api/v1/admin/accounts/performance/batch", h.GetBatchPerformance)
	return router
}

func requestAccountPerformance(router *gin.Engine, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/performance/batch", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAccountPerformanceHandlerValidation(t *testing.T) {
	router := setupAccountPerformanceRouter(t, nil)
	for _, body := range []string{
		`{`, `{}`, `null`, `{"account_ids":null}`, `{"account_ids":"1"}`,
		`{"account_ids":[0]}`, `{"account_ids":[-1]}`, `{"account_ids":[1.5]}`,
		`{"account_ids":[9223372036854775808]}`, `{"account_ids":[1,null]}`,
		`{"account_ids":[` + strings.Repeat("1,", service.MaxAccountPerformanceBatchSize) + `1]}`,
	} {
		t.Run(body[:min(len(body), 60)], func(t *testing.T) {
			require.Equal(t, http.StatusBadRequest, requestAccountPerformance(router, body).Code)
		})
	}
	recorder := requestAccountPerformance(router, `{"account_ids":[]}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Data service.AccountPerformanceBatchStats `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.NotNil(t, payload.Data.Stats)
	require.Empty(t, payload.Data.Stats)
	require.Equal(t, time.Hour, payload.Data.WindowEnd.Sub(payload.Data.WindowStart))
}

func TestAccountPerformanceHandlerCacheSnapshot(t *testing.T) {
	calls := 0
	var queryStart, queryEnd time.Time
	router := setupAccountPerformanceRouter(t, &accountPerformanceRepoStub{
		read: func(_ context.Context, ids []int64, start, end time.Time) (map[int64]*service.AccountPerformanceStats, error) {
			calls++
			require.Equal(t, []int64{2, 9}, ids)
			queryStart, queryEnd = start, end
			return map[int64]*service.AccountPerformanceStats{
				2: {RequestCount: 3, LastRequestAt: &start},
			}, nil
		},
	})
	first := requestAccountPerformance(router, `{"account_ids":[9,2,9]}`)
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, "miss", first.Header().Get("X-Snapshot-Cache"))
	var payload struct {
		Data service.AccountPerformanceBatchStats `json:"data"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &payload))
	require.Equal(t, queryStart, payload.Data.WindowStart)
	require.Equal(t, queryEnd, payload.Data.WindowEnd)
	require.Equal(t, int64(3), payload.Data.Stats[2].RequestCount)
	require.Equal(t, &service.AccountPerformanceStats{}, payload.Data.Stats[9])
	require.Contains(t, first.Body.String(), `"ttft_ms":null`)
	require.Contains(t, first.Body.String(), `"tps":null`)
	require.Contains(t, first.Body.String(), `"cache_rate":null`)
	require.Contains(t, first.Body.String(), `"last_request_at":null`)

	second := requestAccountPerformance(router, `{"account_ids":[2,9]}`)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, "hit", second.Header().Get("X-Snapshot-Cache"))
	require.JSONEq(t, first.Body.String(), second.Body.String())
	require.Equal(t, 1, calls)
	require.Equal(t, 30*time.Second, accountPerformanceBatchCache.ttl)

	accountPerformanceBatchCache.mu.Lock()
	for key, entry := range accountPerformanceBatchCache.items {
		entry.ExpiresAt = time.Now().Add(-time.Second)
		accountPerformanceBatchCache.items[key] = entry
	}
	accountPerformanceBatchCache.mu.Unlock()
	third := requestAccountPerformance(router, `{"account_ids":[2,9]}`)
	require.Equal(t, http.StatusOK, third.Code)
	require.Equal(t, "miss", third.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, 2, calls)
}

func TestAccountPerformanceHandlerMaximumBatch(t *testing.T) {
	calls := 0
	router := setupAccountPerformanceRouter(t, &accountPerformanceRepoStub{
		read: func(_ context.Context, ids []int64, _, _ time.Time) (map[int64]*service.AccountPerformanceStats, error) {
			calls++
			require.Len(t, ids, 1000)
			return nil, nil
		},
	})
	ids := make([]int64, 1000)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	body, err := json.Marshal(BatchAccountPerformanceRequest{AccountIDs: ids})
	require.NoError(t, err)
	recorder := requestAccountPerformance(router, string(body))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, calls)
	var payload struct {
		Data service.AccountPerformanceBatchStats `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Stats, 1000)
}

func TestAccountPerformanceHandlerDoesNotCacheErrors(t *testing.T) {
	calls := 0
	router := setupAccountPerformanceRouter(t, &accountPerformanceRepoStub{
		read: func(context.Context, []int64, time.Time, time.Time) (map[int64]*service.AccountPerformanceStats, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("query failed")
			}
			return nil, nil
		},
	})
	require.Equal(t, http.StatusInternalServerError, requestAccountPerformance(router, `{"account_ids":[1]}`).Code)
	require.Equal(t, http.StatusOK, requestAccountPerformance(router, `{"account_ids":[1]}`).Code)
	require.Equal(t, 2, calls)
}

func TestAccountPerformanceHandlerSingleflight(t *testing.T) {
	var calls atomic.Int32
	router := setupAccountPerformanceRouter(t, &accountPerformanceRepoStub{
		read: func(context.Context, []int64, time.Time, time.Time) (map[int64]*service.AccountPerformanceStats, error) {
			calls.Add(1)
			time.Sleep(20 * time.Millisecond)
			return nil, nil
		},
	})
	var wg sync.WaitGroup
	results := make([]*httptest.ResponseRecorder, 12)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := `{"account_ids":[2,1]}`
			if i%2 == 0 {
				body = `{"account_ids":[1,2,1]}`
			}
			results[i] = requestAccountPerformance(router, body)
		}()
	}
	wg.Wait()
	for _, result := range results {
		require.Equal(t, http.StatusOK, result.Code)
		require.JSONEq(t, results[0].Body.String(), result.Body.String())
	}
	require.Equal(t, int32(1), calls.Load())
}

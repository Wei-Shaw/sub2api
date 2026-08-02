package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type batchDeleteAdminService struct {
	*stubAdminService

	mu               sync.Mutex
	active           int
	maxActive        int
	deletedIDs       []int64
	deleteErrorsByID map[int64]error
	accountsByID     map[int64]*service.Account
}

func (s *batchDeleteAdminService) GetAccountsByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	accounts := make([]*service.Account, 0, len(ids))
	for _, id := range ids {
		if s.accountsByID != nil {
			if account, ok := s.accountsByID[id]; ok {
				accounts = append(accounts, account)
			}
			continue
		}
		accounts = append(accounts, &service.Account{ID: id})
	}
	return accounts, nil
}

func (s *batchDeleteAdminService) DeleteAccount(ctx context.Context, id int64) error {
	s.mu.Lock()
	s.active++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		s.mu.Lock()
		s.active--
		s.mu.Unlock()
		return ctx.Err()
	case <-time.After(10 * time.Millisecond):
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.active--
	s.deletedIDs = append(s.deletedIDs, id)
	return s.deleteErrorsByID[id]
}

func setupAccountBatchDeleteRouter(adminSvc *batchDeleteAdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/batch-delete", handler.BatchDelete)
	return router
}

func TestAccountHandlerBatchDeleteReturnsStablePerAccountResults(t *testing.T) {
	adminSvc := &batchDeleteAdminService{
		stubAdminService: newStubAdminService(),
		deleteErrorsByID: map[int64]error{
			3: errors.New("delete failed"),
		},
	}
	router := setupAccountBatchDeleteRouter(adminSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/accounts/batch-delete",
		bytes.NewBufferString(`{"account_ids":[5,4,3,2,1,2,0,-1]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var payload struct {
		Data struct {
			Total      int     `json:"total"`
			Success    int     `json:"success"`
			Failed     int     `json:"failed"`
			SuccessIDs []int64 `json:"success_ids"`
			FailedIDs  []int64 `json:"failed_ids"`
			Errors     []struct {
				AccountID int64  `json:"account_id"`
				Error     string `json:"error"`
			} `json:"errors"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, 5, payload.Data.Total)
	require.Equal(t, 4, payload.Data.Success)
	require.Equal(t, 1, payload.Data.Failed)
	require.Equal(t, []int64{1, 2, 4, 5}, payload.Data.SuccessIDs)
	require.Equal(t, []int64{3}, payload.Data.FailedIDs)
	require.Equal(t, int64(3), payload.Data.Errors[0].AccountID)
	require.Equal(t, "delete failed", payload.Data.Errors[0].Error)
	require.LessOrEqual(t, adminSvc.maxActive, 5)
	require.Greater(t, adminSvc.maxActive, 1)
}

func TestAccountHandlerBatchDeleteDoesNotRaceSelectedShadowWithParent(t *testing.T) {
	parentID := int64(1)
	adminSvc := &batchDeleteAdminService{
		stubAdminService: newStubAdminService(),
		accountsByID: map[int64]*service.Account{
			1: {ID: 1},
			2: {ID: 2, ParentAccountID: &parentID},
			3: {ID: 3},
		},
	}
	router := setupAccountBatchDeleteRouter(adminSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/accounts/batch-delete",
		bytes.NewBufferString(`{"account_ids":[1,2,3]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var payload struct {
		Data struct {
			SuccessIDs []int64 `json:"success_ids"`
			FailedIDs  []int64 `json:"failed_ids"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, []int64{1, 2, 3}, payload.Data.SuccessIDs)
	require.Empty(t, payload.Data.FailedIDs)
	require.ElementsMatch(t, []int64{1, 3}, adminSvc.deletedIDs)
}

func TestAccountHandlerBatchDeleteRejectsEmptyNormalizedIDs(t *testing.T) {
	adminSvc := &batchDeleteAdminService{
		stubAdminService: newStubAdminService(),
	}
	router := setupAccountBatchDeleteRouter(adminSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/accounts/batch-delete",
		bytes.NewBufferString(`{"account_ids":[0,-1]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

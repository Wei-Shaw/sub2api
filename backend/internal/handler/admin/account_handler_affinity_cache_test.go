package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type affinityCacheAdminService struct {
	*stubAdminService
	accounts        map[int64]*service.Account
	bulkResultIDs   []int64
	bulkFailedCount int
}

func (s *affinityCacheAdminService) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	if acc, ok := s.accounts[id]; ok {
		cloned := *acc
		return &cloned, nil
	}
	return s.stubAdminService.GetAccount(context.Background(), id)
}

func (s *affinityCacheAdminService) UpdateAccount(_ context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	acc := s.accounts[id]
	if acc == nil {
		return s.stubAdminService.UpdateAccount(context.Background(), id, input)
	}
	if input.Extra != nil {
		acc.Extra = input.Extra
	}
	if input.GroupIDs != nil {
		acc.GroupIDs = *input.GroupIDs
	}
	cloned := *acc
	return &cloned, nil
}

func (s *affinityCacheAdminService) BulkUpdateAccounts(_ context.Context, input *service.BulkUpdateAccountsInput) (*service.BulkUpdateAccountsResult, error) {
	for _, accountID := range input.AccountIDs {
		acc := s.accounts[accountID]
		if acc == nil {
			continue
		}
		if input.Extra != nil {
			acc.Extra = input.Extra
		}
		if input.GroupIDs != nil {
			acc.GroupIDs = *input.GroupIDs
		}
	}
	successIDs := append([]int64(nil), input.AccountIDs...)
	failed := 0
	if len(s.bulkResultIDs) > 0 {
		successIDs = append([]int64(nil), s.bulkResultIDs...)
		failed = s.bulkFailedCount
	}
	return &service.BulkUpdateAccountsResult{
		Success:    len(successIDs),
		Failed:     failed,
		SuccessIDs: successIDs,
		Results:    []service.BulkUpdateAccountResult{},
	}, nil
}

type affinityCacheClearRecorder struct {
	clearCalls []struct {
		accountID int64
		groupIDs  []int64
	}
}

func (r *affinityCacheClearRecorder) GetSessionAccountID(_ context.Context, _ int64, _ string) (int64, error) {
	return 0, nil
}
func (r *affinityCacheClearRecorder) SetSessionAccountID(_ context.Context, _ int64, _ string, _ int64, _ time.Duration) error {
	return nil
}
func (r *affinityCacheClearRecorder) RefreshSessionTTL(_ context.Context, _ int64, _ string, _ time.Duration) error {
	return nil
}
func (r *affinityCacheClearRecorder) DeleteSessionAccountID(_ context.Context, _ int64, _ string) error {
	return nil
}
func (r *affinityCacheClearRecorder) GetAffinityAccounts(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
	return nil, nil
}
func (r *affinityCacheClearRecorder) UpdateAffinity(_ context.Context, _ int64, _ int64, _ string, _ int64, _ time.Duration) error {
	return nil
}
func (r *affinityCacheClearRecorder) GetAccountAffinityCountBatch(_ context.Context, _ int64, _ []int64, _ time.Duration) (map[int64]int64, error) {
	return map[int64]int64{}, nil
}
func (r *affinityCacheClearRecorder) GetAccountAffinityClientsBatch(_ context.Context, _ map[int64][]int64, _ time.Duration) (map[int64][]string, error) {
	return map[int64][]string{}, nil
}
func (r *affinityCacheClearRecorder) GetAccountAffinityClientsWithScores(_ context.Context, _ int64, _ []int64, _ time.Duration) ([]service.AffinityClient, error) {
	return nil, nil
}
func (r *affinityCacheClearRecorder) ClearAccountAffinity(_ context.Context, accountID int64, groupIDs []int64) error {
	r.clearCalls = append(r.clearCalls, struct {
		accountID int64
		groupIDs  []int64
	}{
		accountID: accountID,
		groupIDs:  append([]int64(nil), groupIDs...),
	})
	return nil
}
func (r *affinityCacheClearRecorder) GetAffinityMultiCount(_ context.Context, _ int64, _ int64, _ int64, _ time.Duration) (users, clients, perUser int64, err error) {
	return 0, 0, 0, nil
}

func setupAffinityCacheRouter(adminSvc service.AdminService, gatewayCache service.GatewayCache) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, gatewayCache)
	router.PUT("/api/v1/admin/accounts/:id", handler.Update)
	router.POST("/api/v1/admin/accounts/bulk-update", handler.BulkUpdate)
	return router
}

func TestAccountHandlerUpdate_DisablingAffinityClearsCache(t *testing.T) {
	adminSvc := &affinityCacheAdminService{
		stubAdminService: newStubAdminService(),
		accounts: map[int64]*service.Account{
			3: {
				ID:       3,
				Platform: service.PlatformAnthropic,
				Type:     service.AccountTypeOAuth,
				Status:   service.StatusActive,
				GroupIDs: []int64{1, 2},
				Extra: map[string]any{
					"client_affinity_enabled": true,
				},
			},
		},
	}
	cache := &affinityCacheClearRecorder{}
	router := setupAffinityCacheRouter(adminSvc, cache)

	body, _ := json.Marshal(map[string]any{
		"extra": map[string]any{},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/accounts/3", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, cache.clearCalls, 1)
	require.Equal(t, int64(3), cache.clearCalls[0].accountID)
	require.Equal(t, []int64{1, 2}, cache.clearCalls[0].groupIDs)
}

func TestAccountHandlerBulkUpdate_DisablingAffinityClearsCache(t *testing.T) {
	adminSvc := &affinityCacheAdminService{
		stubAdminService: newStubAdminService(),
		accounts: map[int64]*service.Account{
			1: {
				ID:       1,
				Platform: service.PlatformAnthropic,
				Type:     service.AccountTypeOAuth,
				Status:   service.StatusActive,
				GroupIDs: []int64{11},
				Extra: map[string]any{
					"client_affinity_enabled": true,
				},
			},
			2: {
				ID:       2,
				Platform: service.PlatformAnthropic,
				Type:     service.AccountTypeSetupToken,
				Status:   service.StatusActive,
				GroupIDs: []int64{22},
				Extra: map[string]any{
					"client_affinity_enabled": true,
				},
			},
		},
	}
	cache := &affinityCacheClearRecorder{}
	router := setupAffinityCacheRouter(adminSvc, cache)

	body, _ := json.Marshal(map[string]any{
		"account_ids": []int64{1, 2},
		"extra": map[string]any{
			"client_affinity_enabled": false,
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, cache.clearCalls, 2)
	require.Equal(t, int64(1), cache.clearCalls[0].accountID)
	require.Equal(t, []int64{11}, cache.clearCalls[0].groupIDs)
	require.Equal(t, int64(2), cache.clearCalls[1].accountID)
	require.Equal(t, []int64{22}, cache.clearCalls[1].groupIDs)
}

func TestAccountHandlerBulkUpdate_DisablingAffinityWithNewFlagClearsCache(t *testing.T) {
	adminSvc := &affinityCacheAdminService{
		stubAdminService: newStubAdminService(),
		accounts: map[int64]*service.Account{
			1: {
				ID:       1,
				Platform: service.PlatformAnthropic,
				Type:     service.AccountTypeOAuth,
				Status:   service.StatusActive,
				GroupIDs: []int64{33},
				Extra: map[string]any{
					"affinity_enabled": true,
				},
			},
		},
	}
	cache := &affinityCacheClearRecorder{}
	router := setupAffinityCacheRouter(adminSvc, cache)

	body, _ := json.Marshal(map[string]any{
		"account_ids": []int64{1},
		"extra": map[string]any{
			"affinity_enabled": false,
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, cache.clearCalls, 1)
	require.Equal(t, int64(1), cache.clearCalls[0].accountID)
	require.Equal(t, []int64{33}, cache.clearCalls[0].groupIDs)
}

func TestAccountHandlerBulkUpdate_PartialFailureStillClearsDisabledAffinityCache(t *testing.T) {
	adminSvc := &affinityCacheAdminService{
		stubAdminService: newStubAdminService(),
		accounts: map[int64]*service.Account{
			1: {
				ID:       1,
				Platform: service.PlatformAnthropic,
				Type:     service.AccountTypeOAuth,
				Status:   service.StatusActive,
				GroupIDs: []int64{44},
				Extra: map[string]any{
					"affinity_enabled": true,
				},
			},
			2: {
				ID:       2,
				Platform: service.PlatformAnthropic,
				Type:     service.AccountTypeSetupToken,
				Status:   service.StatusActive,
				GroupIDs: []int64{55},
				Extra: map[string]any{
					"client_affinity_enabled": true,
				},
			},
		},
		bulkResultIDs:   []int64{1},
		bulkFailedCount: 1,
	}
	cache := &affinityCacheClearRecorder{}
	router := setupAffinityCacheRouter(adminSvc, cache)

	body, _ := json.Marshal(map[string]any{
		"account_ids": []int64{1, 2},
		"group_ids":   []int64{101},
		"extra": map[string]any{
			"affinity_enabled":        false,
			"client_affinity_enabled": false,
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, cache.clearCalls, 2)
	require.Equal(t, int64(1), cache.clearCalls[0].accountID)
	require.Equal(t, []int64{44, 101}, cache.clearCalls[0].groupIDs)
	require.Equal(t, int64(2), cache.clearCalls[1].accountID)
	require.Equal(t, []int64{55, 101}, cache.clearCalls[1].groupIDs)
}

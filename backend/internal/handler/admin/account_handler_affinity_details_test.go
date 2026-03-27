package admin

import (
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

type affinityDetailsAdminService struct {
	*stubAdminService
	account   service.Account
	usersByID map[int64]service.User
}

func (s *affinityDetailsAdminService) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	if s.account.ID == id {
		acc := s.account
		return &acc, nil
	}
	return s.stubAdminService.GetAccount(context.Background(), id)
}

func (s *affinityDetailsAdminService) GetUser(_ context.Context, id int64) (*service.User, error) {
	if u, ok := s.usersByID[id]; ok {
		user := u
		return &user, nil
	}
	return s.stubAdminService.GetUser(context.Background(), id)
}

type affinityDetailsGatewayCacheStub struct {
	clients []service.AffinityClient
}

func (s *affinityDetailsGatewayCacheStub) GetSessionAccountID(_ context.Context, _ int64, _ string) (int64, error) {
	return 0, nil
}
func (s *affinityDetailsGatewayCacheStub) SetSessionAccountID(_ context.Context, _ int64, _ string, _ int64, _ time.Duration) error {
	return nil
}
func (s *affinityDetailsGatewayCacheStub) RefreshSessionTTL(_ context.Context, _ int64, _ string, _ time.Duration) error {
	return nil
}
func (s *affinityDetailsGatewayCacheStub) DeleteSessionAccountID(_ context.Context, _ int64, _ string) error {
	return nil
}
func (s *affinityDetailsGatewayCacheStub) GetAffinityAccounts(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
	return nil, nil
}
func (s *affinityDetailsGatewayCacheStub) UpdateAffinity(_ context.Context, _ int64, _ int64, _ string, _ int64, _ time.Duration) error {
	return nil
}
func (s *affinityDetailsGatewayCacheStub) GetAccountAffinityCountBatch(_ context.Context, _ int64, _ []int64, _ time.Duration) (map[int64]int64, error) {
	return map[int64]int64{}, nil
}
func (s *affinityDetailsGatewayCacheStub) GetAccountAffinityClientsBatch(_ context.Context, _ map[int64][]int64, _ time.Duration) (map[int64][]string, error) {
	return map[int64][]string{}, nil
}
func (s *affinityDetailsGatewayCacheStub) GetAccountAffinityClientsWithScores(_ context.Context, _ int64, _ []int64, _ time.Duration) ([]service.AffinityClient, error) {
	return s.clients, nil
}
func (s *affinityDetailsGatewayCacheStub) ClearAccountAffinity(_ context.Context, _ int64, _ []int64) error {
	return nil
}
func (s *affinityDetailsGatewayCacheStub) GetAffinityMultiCount(_ context.Context, _ int64, _ int64, _ int64, _ time.Duration) (users, clients, perUser int64, err error) {
	return 0, 0, 0, nil
}

func setupAffinityDetailsRouter(adminSvc service.AdminService, gatewayCache service.GatewayCache) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, gatewayCache)
	router.GET("/api/v1/admin/accounts/:id/affinity-details", handler.GetAffinityDetails)
	return router
}

func TestAccountHandlerGetAffinityDetails_ContractFields(t *testing.T) {
	adminSvc := &affinityDetailsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       88,
			Platform: service.PlatformAnthropic,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			GroupIDs: []int64{1},
			Extra: map[string]any{
				"client_affinity_enabled": true,
				"pinned_users":            []any{float64(42), float64(99)},
			},
		},
		usersByID: map[int64]service.User{
			42: {ID: 42, Email: "user42@example.com"},
			7:  {ID: 7, Username: "user7"},
		},
	}
	cache := &affinityDetailsGatewayCacheStub{
		clients: []service.AffinityClient{
			{UserID: 42, ClientID: "client-a", LastActive: time.Now().Add(-time.Minute)},
			{UserID: 7, ClientID: "client-b", LastActive: time.Now().Add(-2 * time.Minute)},
			{UserID: 42, ClientID: "client-c", LastActive: time.Now().Add(-3 * time.Minute)},
		},
	}
	router := setupAffinityDetailsRouter(adminSvc, cache)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/88/affinity-details", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Users []struct {
				UserID      int64  `json:"user_id"`
				UserEmail   string `json:"user_email"`
				ClientCount int    `json:"client_count"`
				IsPinned    bool   `json:"is_pinned"`
				Clients     []struct {
					ClientID string `json:"client_id"`
				} `json:"clients"`
			} `json:"users"`
			TotalUsers   int     `json:"total_users"`
			TotalClients int     `json:"total_clients"`
			PinnedUsers  []int64 `json:"pinned_users"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 2, resp.Data.TotalUsers)
	require.Equal(t, 3, resp.Data.TotalClients)
	require.Equal(t, []int64{42, 99}, resp.Data.PinnedUsers)
	require.Len(t, resp.Data.Users, 2)

	usersByID := make(map[int64]struct {
		UserEmail   string
		ClientCount int
		IsPinned    bool
		ClientLen   int
	}, len(resp.Data.Users))
	for _, u := range resp.Data.Users {
		usersByID[u.UserID] = struct {
			UserEmail   string
			ClientCount int
			IsPinned    bool
			ClientLen   int
		}{
			UserEmail:   u.UserEmail,
			ClientCount: u.ClientCount,
			IsPinned:    u.IsPinned,
			ClientLen:   len(u.Clients),
		}
	}
	require.Equal(t, "user42@example.com", usersByID[42].UserEmail)
	require.Equal(t, 2, usersByID[42].ClientCount)
	require.True(t, usersByID[42].IsPinned)
	require.Equal(t, 2, usersByID[42].ClientLen)

	require.Equal(t, "user7", usersByID[7].UserEmail)
	require.Equal(t, 1, usersByID[7].ClientCount)
	require.False(t, usersByID[7].IsPinned)
	require.Equal(t, 1, usersByID[7].ClientLen)
}

func TestAccountHandlerGetAffinityDetails_DisabledReturnsEmptyContract(t *testing.T) {
	adminSvc := &affinityDetailsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       89,
			Platform: service.PlatformAnthropic,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			GroupIDs: []int64{1},
		},
		usersByID: map[int64]service.User{},
	}
	router := setupAffinityDetailsRouter(adminSvc, &affinityDetailsGatewayCacheStub{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/89/affinity-details", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Users        []any   `json:"users"`
			TotalUsers   int     `json:"total_users"`
			TotalClients int     `json:"total_clients"`
			PinnedUsers  []int64 `json:"pinned_users"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Empty(t, resp.Data.Users)
	require.Equal(t, 0, resp.Data.TotalUsers)
	require.Equal(t, 0, resp.Data.TotalClients)
	require.Empty(t, resp.Data.PinnedUsers)
}

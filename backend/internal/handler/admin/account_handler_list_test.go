package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAccountListRouter() (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.GET("/api/v1/admin/accounts", handler.List)
	return router, adminSvc
}

func TestAccountHandlerListIncludesCreatedAt(t *testing.T) {
	router, adminSvc := setupAccountListRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?page=1&page_size=20&sort_by=created_at&sort_order=desc", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "created_at", adminSvc.lastListAccounts.sortBy)

	var payload struct {
		Data struct {
			Items []struct {
				ID        int64  `json:"id"`
				CreatedAt string `json:"created_at"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 1)

	createdAt := payload.Data.Items[0].CreatedAt
	require.NotEmpty(t, createdAt)
	require.True(t, strings.HasSuffix(createdAt, "Z"), "created_at should be serialized as UTC")
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	require.NoError(t, err)
	_, offset := parsed.Zone()
	require.Equal(t, 0, offset)
}

func TestAccountHandlerListReturnsSchedulerScoresPerGroup(t *testing.T) {
	router, adminSvc := setupAccountListRouter()
	now := time.Now().UTC()
	groupID := int64(41)
	adminSvc.accounts = []service.Account{
		{
			ID:          101,
			Name:        "account-high-priority",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Concurrency: 10,
			Priority:    1,
			AccountGroups: []service.AccountGroup{
				{AccountID: 101, GroupID: groupID, Priority: 100, Group: &service.Group{ID: groupID, Name: "openai"}},
			},
			GroupIDs:  []int64{groupID},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          102,
			Name:        "account-low-priority",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Concurrency: 10,
			Priority:    100000,
			AccountGroups: []service.AccountGroup{
				{AccountID: 102, GroupID: groupID, Priority: 1, Group: &service.Group{ID: groupID, Name: "openai"}},
			},
			GroupIDs:  []int64{groupID},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?page=1&page_size=20&platform=openai", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var payload struct {
		Data struct {
			Items []struct {
				ID             int64 `json:"id"`
				SchedulerScore struct {
					BaseScore float64 `json:"base_score"`
				} `json:"scheduler_score"`
				SchedulerScores []struct {
					GroupID       *int64  `json:"group_id"`
					GroupName     string  `json:"group_name"`
					GroupPriority *int    `json:"group_priority"`
					BaseScore     float64 `json:"base_score"`
				} `json:"scheduler_scores"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 2)

	var high, low *struct {
		ID             int64 `json:"id"`
		SchedulerScore struct {
			BaseScore float64 `json:"base_score"`
		} `json:"scheduler_score"`
		SchedulerScores []struct {
			GroupID       *int64  `json:"group_id"`
			GroupName     string  `json:"group_name"`
			GroupPriority *int    `json:"group_priority"`
			BaseScore     float64 `json:"base_score"`
		} `json:"scheduler_scores"`
	}
	for i := range payload.Data.Items {
		item := &payload.Data.Items[i]
		switch item.ID {
		case 101:
			high = item
		case 102:
			low = item
		}
	}
	require.NotNil(t, high)
	require.NotNil(t, low)
	require.Len(t, high.SchedulerScores, 1)
	require.Len(t, low.SchedulerScores, 1)
	require.Equal(t, groupID, *high.SchedulerScores[0].GroupID)
	require.Equal(t, "openai", high.SchedulerScores[0].GroupName)
	require.Equal(t, 100, *high.SchedulerScores[0].GroupPriority)
	require.Equal(t, 1, *low.SchedulerScores[0].GroupPriority)
	require.Greater(t, high.SchedulerScores[0].BaseScore, low.SchedulerScores[0].BaseScore)
}

func TestAccountHandlerListKeepsSchedulerScoreScopedToFilter(t *testing.T) {
	router, adminSvc := setupAccountListRouter()
	now := time.Now().UTC()
	groupID := int64(42)
	visibleAccount := service.Account{
		ID:          201,
		Name:        "visible-low-priority",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 10,
		Priority:    100000,
		AccountGroups: []service.AccountGroup{
			{AccountID: 201, GroupID: groupID, Priority: 1, Group: &service.Group{ID: groupID, Name: "openai"}},
		},
		GroupIDs:  []int64{groupID},
		CreatedAt: now,
		UpdatedAt: now,
	}
	hiddenGroupPeer := service.Account{
		ID:          202,
		Name:        "hidden-high-priority",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 10,
		Priority:    1,
		AccountGroups: []service.AccountGroup{
			{AccountID: 202, GroupID: groupID, Priority: 2, Group: &service.Group{ID: groupID, Name: "openai"}},
		},
		GroupIDs:  []int64{groupID},
		CreatedAt: now,
		UpdatedAt: now,
	}
	adminSvc.accounts = []service.Account{visibleAccount}
	adminSvc.accountSchedulerScoreFilterAccounts = []service.Account{visibleAccount, hiddenGroupPeer}
	adminSvc.openAISchedulerScorePoolAccounts = []service.Account{visibleAccount, hiddenGroupPeer}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?page=1&page_size=1&platform=openai", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var payload struct {
		Data struct {
			Items []struct {
				ID             int64 `json:"id"`
				SchedulerScore struct {
					BaseScore float64 `json:"base_score"`
				} `json:"scheduler_score"`
				SchedulerScores []struct {
					GroupID   *int64  `json:"group_id"`
					BaseScore float64 `json:"base_score"`
				} `json:"scheduler_scores"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 1)
	item := payload.Data.Items[0]
	require.Equal(t, int64(201), item.ID)
	require.Len(t, item.SchedulerScores, 1)
	require.Equal(t, groupID, *item.SchedulerScores[0].GroupID)
	require.Equal(t, item.SchedulerScores[0].BaseScore, item.SchedulerScore.BaseScore)
}

func TestAccountHandlerListSchedulerScoreIgnoresPagination(t *testing.T) {
	router, adminSvc := setupAccountListRouter()
	now := time.Now().UTC()
	visibleAccount := service.Account{
		ID:          301,
		Name:        "visible-low-priority",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 10,
		Priority:    100000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	hiddenFilterPeer := service.Account{
		ID:          302,
		Name:        "hidden-high-priority",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 10,
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	adminSvc.accounts = []service.Account{visibleAccount}
	adminSvc.accountSchedulerScoreFilterAccounts = []service.Account{visibleAccount, hiddenFilterPeer}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?page=1&page_size=1&platform=openai", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var payload struct {
		Data struct {
			Items []struct {
				ID             int64 `json:"id"`
				SchedulerScore struct {
					BaseScore float64 `json:"base_score"`
				} `json:"scheduler_score"`
				SchedulerScores []struct {
					GroupID   *int64  `json:"group_id"`
					BaseScore float64 `json:"base_score"`
				} `json:"scheduler_scores"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, int64(301), payload.Data.Items[0].ID)
	require.Less(t, payload.Data.Items[0].SchedulerScore.BaseScore, 3.75)
	require.Empty(t, payload.Data.Items[0].SchedulerScores)
}

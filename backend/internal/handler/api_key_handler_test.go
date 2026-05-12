//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ─── Stubs ────────────────────────────────────────────────────────────────────

// availGroupsUserRepoStub satisfies the subset of UserRepository that
// GetAvailableGroups / GetAvailableGroupsProfile exercises.
type availGroupsUserRepoStub struct {
	service.UserRepository
}

func (s *availGroupsUserRepoStub) GetByID(_ context.Context, id int64) (*service.User, error) {
	return &service.User{ID: id, Status: service.StatusActive}, nil
}

func (s *availGroupsUserRepoStub) GetEffectiveAllowedGroupSources(_ context.Context, _ []int64, _ service.EffectiveAllowedGroupsOptions) (map[int64][]service.EffectiveAllowedGroupSource, error) {
	return map[int64][]service.EffectiveAllowedGroupSource{}, nil
}

// availGroupsGroupRepoStub satisfies GroupRepository.ListActive.
type availGroupsGroupRepoStub struct {
	service.GroupRepository
	groups []service.Group
}

func (s *availGroupsGroupRepoStub) ListActive(_ context.Context) ([]service.Group, error) {
	return s.groups, nil
}

// availGroupsSubRepoStub satisfies UserSubscriptionRepository.
type availGroupsSubRepoStub struct {
	service.UserSubscriptionRepository
}

func (s *availGroupsSubRepoStub) ListActiveByUserID(_ context.Context, _ int64) ([]service.UserSubscription, error) {
	return nil, nil
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

// newHandlerWithStubSvc creates an APIKeyHandler backed by a real APIKeyService
// wired with the provided user repository and a minimal group/sub stub.
func newHandlerWithUserRepo(
	userRepo service.UserRepository,
	groupRepo service.GroupRepository,
	subRepo service.UserSubscriptionRepository,
) *APIKeyHandler {
	cfg := &config.Config{}
	svc := service.NewAPIKeyService(nil, userRepo, groupRepo, subRepo, nil, nil, cfg)
	return &APIKeyHandler{apiKeyService: svc}
}

// setAuthSubject injects the auth subject into a gin context (replicates the
// middleware behaviour used in production).
func setAuthSubject(c *gin.Context, userID int64) {
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userID})
}

// ─── GetAvailableGroups tests ─────────────────────────────────────────────────

// TestGetAvailableGroups_ReturnsDataAsArray asserts that GET /groups/available
// returns {"code":0,"data":[...]}, where each item has "id" and "name" fields.
func TestGetAvailableGroups_ReturnsDataAsArray(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Static user repo that returns a plain active user (no AllowedGroups).
	userRepo := &availGroupsUserRepoStub{}

	groupRepo := &availGroupsGroupRepoStub{
		groups: []service.Group{
			{ID: 1, Name: "public-group", Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, IsExclusive: false},
		},
	}
	subRepo := &availGroupsSubRepoStub{}

	handler := newHandlerWithUserRepo(userRepo, groupRepo, subRepo)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/groups/available", nil)
	setAuthSubject(c, 42)

	handler.GetAvailableGroups(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int              `json:"code"`
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.IsType(t, []map[string]any{}, resp.Data, "data must be a flat array")
}

// TestGetAvailableGroups_ItemHasIDAndNameFields asserts that items in the data
// array have "id" and "name" fields, matching the legacy Group DTO contract.
func TestGetAvailableGroups_ItemHasIDAndNameFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userRepo := &availGroupsUserRepoStub{}
	groupRepo := &availGroupsGroupRepoStub{
		groups: []service.Group{
			{ID: 7, Name: "my-group", Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, IsExclusive: false},
		},
	}
	subRepo := &availGroupsSubRepoStub{}

	handler := newHandlerWithUserRepo(userRepo, groupRepo, subRepo)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/groups/available", nil)
	setAuthSubject(c, 5)

	handler.GetAvailableGroups(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int              `json:"code"`
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	item := resp.Data[0]
	require.Contains(t, item, "id", "item must have 'id' field")
	require.Contains(t, item, "name", "item must have 'name' field")
}

// ─── GetAvailableGroupsProfile tests ─────────────────────────────────────────

// TestGetAvailableGroupsProfile_ReturnsObjectStructure asserts that the profile
// endpoint returns an object with "bindable_groups" and "grant_effective_groups"
// keys (not a flat array).
func TestGetAvailableGroupsProfile_ReturnsObjectStructure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userRepo := &availGroupsUserRepoStub{}
	groupRepo := &availGroupsGroupRepoStub{
		groups: []service.Group{
			{ID: 3, Name: "pool-group", Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, IsExclusive: false},
		},
	}
	subRepo := &availGroupsSubRepoStub{}

	// flag=false → no pool queries but structure must still be the profile object.
	handler := newHandlerWithUserRepo(userRepo, groupRepo, subRepo)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/groups/available/profile", nil)
	setAuthSubject(c, 9)

	handler.GetAvailableGroupsProfile(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			BindableGroups       []map[string]any `json:"bindable_groups"`
			GrantEffectiveGroups []int64          `json:"grant_effective_groups"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	// data must have both top-level keys (even if empty).
	var raw struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &raw))
	require.Contains(t, raw.Data, "bindable_groups")
	require.Contains(t, raw.Data, "grant_effective_groups")
}

// TestGetAvailableGroupsProfile_BindableGroupsItemHasIDAndName asserts that
// items in bindable_groups have "id" and "name" fields.
func TestGetAvailableGroupsProfile_BindableGroupsItemHasIDAndName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userRepo := &availGroupsUserRepoStub{}
	groupRepo := &availGroupsGroupRepoStub{
		groups: []service.Group{
			{ID: 11, Name: "bindable", Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, IsExclusive: false},
		},
	}
	subRepo := &availGroupsSubRepoStub{}

	handler := newHandlerWithUserRepo(userRepo, groupRepo, subRepo)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/groups/available/profile", nil)
	setAuthSubject(c, 13)

	handler.GetAvailableGroupsProfile(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			BindableGroups []map[string]any `json:"bindable_groups"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Len(t, resp.Data.BindableGroups, 1)
	item := resp.Data.BindableGroups[0]
	require.Contains(t, item, "id", "bindable_groups item must have 'id' field")
	require.Contains(t, item, "name", "bindable_groups item must have 'name' field")
}

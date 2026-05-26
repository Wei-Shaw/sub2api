//go:build unit

package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type modelMarketplaceSettingRepoStub struct {
	service.SettingRepository
	values map[string]string
	err    error
}

type modelMarketplaceHandlerChannelRepoStub struct {
	service.ChannelRepository
	channels   []service.Channel
	listAllErr error
}

func (s *modelMarketplaceHandlerChannelRepoStub) ListAll(context.Context) ([]service.Channel, error) {
	if s.listAllErr != nil {
		return nil, s.listAllErr
	}
	return append([]service.Channel(nil), s.channels...), nil
}

type modelMarketplaceHandlerGroupRepoStub struct {
	service.GroupRepository
	groups []service.Group
}

func (s *modelMarketplaceHandlerGroupRepoStub) ListActive(context.Context) ([]service.Group, error) {
	return append([]service.Group(nil), s.groups...), nil
}

type modelMarketplaceHandlerUserRepoStub struct {
	service.UserRepository
	user       *service.User
	getByIDErr error
}

func (s *modelMarketplaceHandlerUserRepoStub) GetByID(_ context.Context, id int64) (*service.User, error) {
	if s.getByIDErr != nil {
		return nil, s.getByIDErr
	}
	if s.user == nil || s.user.ID != id {
		return nil, errors.New("user not found")
	}
	copyUser := *s.user
	copyUser.AllowedGroups = append([]int64(nil), s.user.AllowedGroups...)
	return &copyUser, nil
}

type modelMarketplaceHandlerUserSubscriptionRepoStub struct {
	service.UserSubscriptionRepository
	activeByUser  map[int64][]service.UserSubscription
	listActiveErr error
}

func (s *modelMarketplaceHandlerUserSubscriptionRepoStub) ListActiveByUserID(_ context.Context, userID int64) ([]service.UserSubscription, error) {
	if s.listActiveErr != nil {
		return nil, s.listActiveErr
	}
	items := s.activeByUser[userID]
	out := make([]service.UserSubscription, len(items))
	copy(out, items)
	return out, nil
}

type modelMarketplaceHandlerUserGroupRateRepoStub struct {
	service.UserGroupRateRepository
	ratesByUser  map[int64]map[int64]float64
	getByUserErr error
}

func (s *modelMarketplaceHandlerUserGroupRateRepoStub) GetByUserID(_ context.Context, userID int64) (map[int64]float64, error) {
	if s.getByUserErr != nil {
		return nil, s.getByUserErr
	}
	items := s.ratesByUser[userID]
	out := make(map[int64]float64, len(items))
	for groupID, rate := range items {
		out[groupID] = rate
	}
	return out, nil
}

func (s *modelMarketplaceSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func newModelMarketplaceSettingService(enabled, public bool) *service.SettingService {
	return service.NewSettingService(&modelMarketplaceSettingRepoStub{
		values: map[string]string{
			service.SettingKeyAvailableChannelsEnabled:      boolString(enabled),
			service.SettingKeyModelMarketplacePublicEnabled: boolString(public),
		},
	}, &config.Config{})
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func newModelMarketplacePublicChannelService() *service.ChannelService {
	return service.NewChannelService(&modelMarketplaceHandlerChannelRepoStub{channels: []service.Channel{{
		ID:          1,
		Name:        "demo-channel",
		Description: "demo",
		Status:      service.StatusActive,
		GroupIDs:    []int64{1},
		ModelPricing: []service.ChannelModelPricing{{
			Platform: service.PlatformAnthropic,
			Models:   []string{"claude-sonnet-4-6"},
		}},
	}}}, &modelMarketplaceHandlerGroupRepoStub{groups: []service.Group{{
		ID:               1,
		Name:             "public-standard",
		Platform:         service.PlatformAnthropic,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		RateMultiplier:   1.2,
	}}}, nil, nil)
}

func newModelMarketplaceAuthenticatedUserRepo() *modelMarketplaceHandlerUserRepoStub {
	return &modelMarketplaceHandlerUserRepoStub{user: &service.User{
		ID:            42,
		Email:         "user@example.com",
		Role:          service.RoleUser,
		Status:        service.StatusActive,
		AllowedGroups: []int64{},
	}}
}

func newModelMarketplaceHandlerEntTestClient(t *testing.T) *dbent.Client {
	t.Helper()

	dbName := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestModelMarketplaceList_MissingChannelServiceFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelMarketplaceHandler(nil, nil, nil, nil, newModelMarketplaceSettingService(true, true))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)

	h.List(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Code)
	require.Equal(t, "MODEL_MARKETPLACE_DEPENDENCY_MISSING", resp.Reason)
	require.Equal(t, "model marketplace dependencies are not configured", resp.Message)
}

func TestModelMarketplaceList_MissingAuthenticatedServicesFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelMarketplaceHandler(&service.ChannelService{}, nil, nil, nil, newModelMarketplaceSettingService(true, true))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})

	h.List(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp struct {
		Code   int    `json:"code"`
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Code)
	require.Equal(t, "MODEL_MARKETPLACE_DEPENDENCY_MISSING", resp.Reason)
}

func TestModelMarketplaceList_MisconfiguredPaymentConfigFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	channelService := newModelMarketplacePublicChannelService()
	h := NewModelMarketplaceHandler(channelService, nil, nil, service.NewPaymentConfigService(nil, nil, nil), newModelMarketplaceSettingService(true, true))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)

	h.List(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp struct {
		Code   int    `json:"code"`
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Code)
	require.Equal(t, "MODEL_MARKETPLACE_DEPENDENCY_MISSING", resp.Reason)
}

func TestModelMarketplaceList_RuntimeSettingErrorFailsClosedToEmptyList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelMarketplaceHandler(nil, nil, nil, nil, service.NewSettingService(&modelMarketplaceSettingRepoStub{err: errors.New("settings unavailable")}, &config.Config{}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)

	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Models []map[string]any `json:"models"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Models, 0)
}

func TestModelMarketplaceList_ChannelServiceErrorReturns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	channelService := service.NewChannelService(&modelMarketplaceHandlerChannelRepoStub{listAllErr: errors.New("list channels failed")}, nil, nil, nil)
	h := NewModelMarketplaceHandler(channelService, nil, nil, nil, newModelMarketplaceSettingService(true, true))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)

	h.List(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Code)
}

func TestModelMarketplaceList_ApiKeyServiceErrorReturns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	channelService := newModelMarketplacePublicChannelService()
	apiKeyService := service.NewAPIKeyService(nil, &modelMarketplaceHandlerUserRepoStub{getByIDErr: errors.New("user lookup failed")}, nil, nil, nil, nil, &config.Config{})
	h := NewModelMarketplaceHandler(channelService, apiKeyService, &service.SubscriptionService{}, nil, newModelMarketplaceSettingService(true, true))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})

	h.List(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Code)
}

func TestModelMarketplaceList_GetUserGroupRatesErrorReturns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	channelService := newModelMarketplacePublicChannelService()
	userRepo := newModelMarketplaceAuthenticatedUserRepo()
	groupRepo := &modelMarketplaceHandlerGroupRepoStub{groups: []service.Group{{
		ID:               1,
		Name:             "public-standard",
		Platform:         service.PlatformAnthropic,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		RateMultiplier:   1.2,
	}}}
	userSubRepo := &modelMarketplaceHandlerUserSubscriptionRepoStub{}
	userGroupRateRepo := &modelMarketplaceHandlerUserGroupRateRepoStub{getByUserErr: errors.New("get rates failed")}
	apiKeyService := service.NewAPIKeyService(nil, userRepo, groupRepo, userSubRepo, userGroupRateRepo, nil, &config.Config{})
	subscriptionService := service.NewSubscriptionService(groupRepo, &modelMarketplaceHandlerUserSubscriptionRepoStub{}, nil, nil, &config.Config{})
	h := NewModelMarketplaceHandler(channelService, apiKeyService, subscriptionService, nil, newModelMarketplaceSettingService(true, true))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})

	h.List(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Code)
}

func TestModelMarketplaceList_ListActiveUserSubscriptionsErrorReturns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	channelService := newModelMarketplacePublicChannelService()
	userRepo := newModelMarketplaceAuthenticatedUserRepo()
	groupRepo := &modelMarketplaceHandlerGroupRepoStub{groups: []service.Group{{
		ID:               1,
		Name:             "public-standard",
		Platform:         service.PlatformAnthropic,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		RateMultiplier:   1.2,
	}}}
	apiKeyUserSubRepo := &modelMarketplaceHandlerUserSubscriptionRepoStub{}
	apiKeyService := service.NewAPIKeyService(nil, userRepo, groupRepo, apiKeyUserSubRepo, &modelMarketplaceHandlerUserGroupRateRepoStub{}, nil, &config.Config{})
	subscriptionService := service.NewSubscriptionService(groupRepo, &modelMarketplaceHandlerUserSubscriptionRepoStub{listActiveErr: errors.New("list subs failed")}, nil, nil, &config.Config{})
	h := NewModelMarketplaceHandler(channelService, apiKeyService, subscriptionService, nil, newModelMarketplaceSettingService(true, true))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})

	h.List(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Code)
}

func TestModelMarketplaceList_PaymentConfigQueryErrorReturns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	channelService := newModelMarketplacePublicChannelService()
	entClient := newModelMarketplaceHandlerEntTestClient(t)
	require.NoError(t, entClient.Close())
	h := NewModelMarketplaceHandler(channelService, nil, nil, service.NewPaymentConfigService(entClient, nil, nil), newModelMarketplaceSettingService(true, true))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)

	h.List(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp struct {
		Code   int    `json:"code"`
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Code)
	require.Empty(t, resp.Reason)
}

func TestModelMarketplaceGroupsByPlatform_AnonymousDoesNotLeakPrivateFields(t *testing.T) {
	h := &ModelMarketplaceHandler{}
	groupsByPlatform := h.marketplaceGroupsByPlatform(
		[]service.AvailableGroupRef{{
			ID:               1,
			Name:             "public-standard",
			Platform:         service.PlatformAnthropic,
			SubscriptionType: service.SubscriptionTypeStandard,
			RateMultiplier:   1.2,
			IsExclusive:      false,
		}},
		nil,
		map[int64]float64{1: 1.9},
		nil,
		map[int64][]marketplacePlan{},
		false,
	)

	require.Len(t, groupsByPlatform[service.PlatformAnthropic], 1)
	group := groupsByPlatform[service.PlatformAnthropic][0]
	require.Equal(t, marketplaceAccessAvailable, group.AccessState)
	require.Nil(t, group.UserRateMultiplier)
	require.Nil(t, group.ActiveSubscription)

	raw, err := json.Marshal(group)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	_, hasUserRate := decoded["user_rate_multiplier"]
	_, hasActiveSub := decoded["active_subscription"]
	require.False(t, hasUserRate)
	require.False(t, hasActiveSub)
	for _, key := range []string{"id", "name", "platform", "subscription_type", "rate_multiplier", "is_exclusive", "access_state", "plans"} {
		_, ok := decoded[key]
		require.Truef(t, ok, "anonymous marketplace group must expose %q", key)
	}
}

func TestModelMarketplaceGroupsByPlatform_AuthenticatedAccessStateCoverage(t *testing.T) {
	h := &ModelMarketplaceHandler{}
	now := time.Now().UTC()
	userRate := 1.8
	groupsByPlatform := h.marketplaceGroupsByPlatform(
		[]service.AvailableGroupRef{
			{
				ID:               1,
				Name:             "available-standard",
				Platform:         service.PlatformAnthropic,
				SubscriptionType: service.SubscriptionTypeStandard,
				RateMultiplier:   1.1,
				IsExclusive:      false,
			},
			{
				ID:               2,
				Name:             "subscribed-plan",
				Platform:         service.PlatformAnthropic,
				SubscriptionType: service.SubscriptionTypeSubscription,
				RateMultiplier:   1.3,
				IsExclusive:      true,
			},
			{
				ID:               3,
				Name:             "purchasable-plan",
				Platform:         service.PlatformAnthropic,
				SubscriptionType: service.SubscriptionTypeSubscription,
				RateMultiplier:   1.5,
				IsExclusive:      true,
			},
			{
				ID:               4,
				Name:             "hidden-exclusive",
				Platform:         service.PlatformAnthropic,
				SubscriptionType: service.SubscriptionTypeStandard,
				RateMultiplier:   2.0,
				IsExclusive:      true,
			},
		},
		map[int64]service.Group{
			1: {ID: 1},
		},
		map[int64]float64{
			1: userRate,
		},
		map[int64]service.UserSubscription{
			2: {
				ID:              22,
				GroupID:         2,
				Status:          service.SubscriptionStatusActive,
				StartsAt:        now.Add(-24 * time.Hour),
				ExpiresAt:       now.Add(24 * time.Hour),
				DailyUsageUSD:   1.5,
				WeeklyUsageUSD:  3.5,
				MonthlyUsageUSD: 7.5,
			},
		},
		map[int64][]marketplacePlan{
			3: {{ID: 33, GroupID: 3, Name: "Starter"}},
		},
		true,
	)

	groups := groupsByPlatform[service.PlatformAnthropic]
	require.Len(t, groups, 3)
	byID := make(map[int64]marketplaceGroup, len(groups))
	for _, group := range groups {
		byID[group.ID] = group
	}

	require.Equal(t, marketplaceAccessAvailable, byID[1].AccessState)
	require.NotNil(t, byID[1].UserRateMultiplier)
	require.InDelta(t, userRate, *byID[1].UserRateMultiplier, 0.0001)
	require.Nil(t, byID[1].ActiveSubscription)

	require.Equal(t, marketplaceAccessSubscribed, byID[2].AccessState)
	require.NotNil(t, byID[2].ActiveSubscription)
	require.Equal(t, int64(22), byID[2].ActiveSubscription.ID)

	require.Equal(t, marketplaceAccessPurchasable, byID[3].AccessState)
	require.Len(t, byID[3].Plans, 1)
	require.Nil(t, byID[3].ActiveSubscription)

	_, exists := byID[4]
	require.False(t, exists)

	model := &marketplaceModel{Groups: groups}
	sortMarketplaceModel(model)
	require.Equal(t, marketplaceAccessSubscribed, highestMarketplaceAccess(model.Groups))
	require.Equal(t, marketplaceAccessSubscribed, model.Groups[0].AccessState)
}

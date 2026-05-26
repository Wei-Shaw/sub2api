package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const provideRouterModelMarketplaceJWTSecret = "test-jwt-secret-32bytes-long!!!"

type provideRouterModelMarketplaceSettingRepoStub struct {
	service.SettingRepository
	values map[string]string
}

func (s *provideRouterModelMarketplaceSettingRepoStub) Get(_ context.Context, key string) (*service.Setting, error) {
	if value, ok := s.values[key]; ok {
		return &service.Setting{Key: key, Value: value}, nil
	}
	return nil, nil
}

func (s *provideRouterModelMarketplaceSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s *provideRouterModelMarketplaceSettingRepoStub) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func (s *provideRouterModelMarketplaceSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *provideRouterModelMarketplaceSettingRepoStub) SetMultiple(_ context.Context, values map[string]string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	for key, value := range values {
		s.values[key] = value
	}
	return nil
}

func (s *provideRouterModelMarketplaceSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *provideRouterModelMarketplaceSettingRepoStub) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

type provideRouterModelMarketplaceChannelRepoStub struct {
	service.ChannelRepository
	channels []service.Channel
}

func (s *provideRouterModelMarketplaceChannelRepoStub) ListAll(context.Context) ([]service.Channel, error) {
	return append([]service.Channel(nil), s.channels...), nil
}

type provideRouterModelMarketplaceGroupRepoStub struct {
	service.GroupRepository
	groups []service.Group
}

func (s *provideRouterModelMarketplaceGroupRepoStub) ListActive(context.Context) ([]service.Group, error) {
	return append([]service.Group(nil), s.groups...), nil
}

type provideRouterModelMarketplaceUserRepoStub struct {
	service.UserRepository
	user *service.User
}

func (s *provideRouterModelMarketplaceUserRepoStub) GetByID(_ context.Context, id int64) (*service.User, error) {
	if s.user == nil || s.user.ID != id {
		return nil, service.ErrUserNotFound
	}
	copyUser := *s.user
	copyUser.AllowedGroups = append([]int64(nil), s.user.AllowedGroups...)
	return &copyUser, nil
}

func (s *provideRouterModelMarketplaceUserRepoStub) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

func (s *provideRouterModelMarketplaceUserRepoStub) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	return nil
}

type provideRouterModelMarketplaceUserSubscriptionRepoStub struct {
	service.UserSubscriptionRepository
	activeByUser map[int64][]service.UserSubscription
}

func (s *provideRouterModelMarketplaceUserSubscriptionRepoStub) ListActiveByUserID(_ context.Context, userID int64) ([]service.UserSubscription, error) {
	items := s.activeByUser[userID]
	out := make([]service.UserSubscription, len(items))
	copy(out, items)
	return out, nil
}

type provideRouterModelMarketplaceUserGroupRateRepoStub struct {
	service.UserGroupRateRepository
	ratesByUser map[int64]map[int64]float64
}

func (s *provideRouterModelMarketplaceUserGroupRateRepoStub) GetByUserID(_ context.Context, userID int64) (map[int64]float64, error) {
	items := s.ratesByUser[userID]
	out := make(map[int64]float64, len(items))
	for groupID, rate := range items {
		out[groupID] = rate
	}
	return out, nil
}

type provideRouterModelMarketplaceFixture struct {
	UserID             int64
	PublicGroupID      int64
	SubscribedGroupID  int64
	UserRateMultiplier float64
	SubscriptionID     int64
}

type provideRouterModelMarketplaceScenario struct {
	router  *gin.Engine
	token   string
	fixture provideRouterModelMarketplaceFixture
	user    *service.User
}

func newProvideRouterModelMarketplaceScenario(t *testing.T, public, backendMode bool) *provideRouterModelMarketplaceScenario {
	t.Helper()
	gin.SetMode(gin.TestMode)

	userID := int64(42)
	publicGroupID := int64(1)
	subscribedGroupID := int64(2)
	subscriptionID := int64(9001)
	userRate := 1.27

	user := &service.User{
		ID:           userID,
		Email:        "user@example.com",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		TokenVersion: 1,
	}
	userRepo := &provideRouterModelMarketplaceUserRepoStub{user: user}
	groupRepo := &provideRouterModelMarketplaceGroupRepoStub{groups: []service.Group{
		{ID: publicGroupID, Name: "public-standard", Platform: service.PlatformAnthropic, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 1.1, IsExclusive: false},
		{ID: subscribedGroupID, Name: "subscriber-pro", Platform: service.PlatformAnthropic, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeSubscription, RateMultiplier: 1.3, IsExclusive: true},
	}}
	userSubRepo := &provideRouterModelMarketplaceUserSubscriptionRepoStub{activeByUser: map[int64][]service.UserSubscription{
		userID: {{
			ID:              subscriptionID,
			UserID:          userID,
			GroupID:         subscribedGroupID,
			Status:          service.SubscriptionStatusActive,
			StartsAt:        time.Now().Add(-2 * time.Hour),
			ExpiresAt:       time.Now().Add(48 * time.Hour),
			DailyUsageUSD:   1.25,
			WeeklyUsageUSD:  2.5,
			MonthlyUsageUSD: 3.75,
		}},
	}}
	userGroupRateRepo := &provideRouterModelMarketplaceUserGroupRateRepoStub{ratesByUser: map[int64]map[int64]float64{
		userID: {publicGroupID: userRate},
	}}

	cfg := &config.Config{}
	cfg.Server.Mode = "release"
	cfg.RunMode = config.RunModeStandard
	cfg.JWT.Secret = provideRouterModelMarketplaceJWTSecret
	cfg.JWT.AccessTokenExpireMinutes = 60

	settingService := service.NewSettingService(&provideRouterModelMarketplaceSettingRepoStub{values: map[string]string{
		service.SettingKeyAvailableChannelsEnabled:      provideRouterBoolString(true),
		service.SettingKeyModelMarketplacePublicEnabled: provideRouterBoolString(public),
		service.SettingKeyBackendModeEnabled:            provideRouterBoolString(backendMode),
	}}, cfg)
	channelService := service.NewChannelService(&provideRouterModelMarketplaceChannelRepoStub{channels: []service.Channel{{
		ID:          1,
		Name:        "alpha",
		Description: "alpha description",
		Status:      service.StatusActive,
		GroupIDs:    []int64{publicGroupID, subscribedGroupID},
		ModelPricing: []service.ChannelModelPricing{{
			Platform:    service.PlatformAnthropic,
			Models:      []string{"claude-sonnet-4-6"},
			BillingMode: service.BillingModeToken,
			InputPrice:  provideRouterFloat64Ptr(0.0012),
			OutputPrice: provideRouterFloat64Ptr(0.0045),
		}},
	}}}, groupRepo, nil, nil)
	apiKeyService := service.NewAPIKeyService(nil, userRepo, groupRepo, userSubRepo, userGroupRateRepo, nil, cfg)
	subscriptionService := service.NewSubscriptionService(groupRepo, userSubRepo, nil, nil, cfg)
	modelMarketplaceHandler := handler.NewModelMarketplaceHandler(channelService, apiKeyService, subscriptionService, nil, settingService)

	handlers := &handler.Handlers{
		Auth:             &handler.AuthHandler{},
		User:             &handler.UserHandler{},
		APIKey:           &handler.APIKeyHandler{},
		Usage:            &handler.UsageHandler{},
		Redeem:           &handler.RedeemHandler{},
		Subscription:     &handler.SubscriptionHandler{},
		Announcement:     &handler.AnnouncementHandler{},
		ChannelMonitor:   &handler.ChannelMonitorUserHandler{},
		Admin:            &handler.AdminHandlers{},
		Gateway:          &handler.GatewayHandler{},
		OpenAIGateway:    &handler.OpenAIGatewayHandler{},
		Setting:          &handler.SettingHandler{},
		Totp:             &handler.TotpHandler{},
		Payment:          &handler.PaymentHandler{},
		PaymentWebhook:   &handler.PaymentWebhookHandler{},
		AvailableChannel: &handler.AvailableChannelHandler{},
		ModelMarketplace: modelMarketplaceHandler,
	}

	authService := service.NewAuthService(nil, userRepo, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil)
	userService := service.NewUserService(userRepo, nil, nil, nil)
	router := ProvideRouter(
		cfg,
		handlers,
		servermiddleware.NewJWTAuthMiddleware(authService, userService),
		servermiddleware.NewOptionalJWTAuthMiddleware(authService, userService),
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) { c.Next() }),
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) { c.Next() }),
		apiKeyService,
		subscriptionService,
		nil,
		settingService,
		nil,
	)
	token, err := authService.GenerateToken(user)
	require.NoError(t, err)

	return &provideRouterModelMarketplaceScenario{
		router: router,
		token:  token,
		fixture: provideRouterModelMarketplaceFixture{
			UserID:             userID,
			PublicGroupID:      publicGroupID,
			SubscribedGroupID:  subscribedGroupID,
			UserRateMultiplier: userRate,
			SubscriptionID:     subscriptionID,
		},
		user: user,
	}
}

func provideRouterBoolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func provideRouterFloat64Ptr(v float64) *float64 { return &v }

func TestProvideRouterModelMarketplace_PublicAnonymousAccessibleEvenInBackendMode(t *testing.T) {
	scenario := newProvideRouterModelMarketplaceScenario(t, true, true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)

	scenario.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Auth struct {
				Authenticated bool `json:"authenticated"`
			} `json:"auth"`
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.False(t, resp.Data.Auth.Authenticated)
	require.Len(t, resp.Data.Models, 1)
	require.Equal(t, "claude-sonnet-4-6", resp.Data.Models[0].Name)
}

func TestProvideRouterModelMarketplace_PrivateRequiresLogin(t *testing.T) {
	scenario := newProvideRouterModelMarketplaceScenario(t, false, true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)

	scenario.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestProvideRouterModelMarketplace_ValidJWTGetsEnhancedView(t *testing.T) {
	scenario := newProvideRouterModelMarketplaceScenario(t, true, true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)
	req.Header.Set("Authorization", "Bearer "+scenario.token)

	scenario.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Auth struct {
				Authenticated bool  `json:"authenticated"`
				UserID        int64 `json:"user_id"`
			} `json:"auth"`
			Models []struct {
				Name        string `json:"name"`
				AccessState string `json:"access_state"`
				Groups      []struct {
					ID                 int64    `json:"id"`
					UserRateMultiplier *float64 `json:"user_rate_multiplier"`
					ActiveSubscription *struct {
						ID int64 `json:"id"`
					} `json:"active_subscription"`
				} `json:"groups"`
			} `json:"models"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.Auth.Authenticated)
	require.Equal(t, scenario.fixture.UserID, resp.Data.Auth.UserID)
	require.Len(t, resp.Data.Models, 1)
	require.Equal(t, "subscribed", resp.Data.Models[0].AccessState)

	var publicGroupFound bool
	var subscribedGroupFound bool
	for _, group := range resp.Data.Models[0].Groups {
		if group.ID == scenario.fixture.PublicGroupID {
			publicGroupFound = true
			require.NotNil(t, group.UserRateMultiplier)
			require.InDelta(t, scenario.fixture.UserRateMultiplier, *group.UserRateMultiplier, 0.0001)
		}
		if group.ID == scenario.fixture.SubscribedGroupID {
			subscribedGroupFound = true
			require.NotNil(t, group.ActiveSubscription)
			require.Equal(t, scenario.fixture.SubscriptionID, group.ActiveSubscription.ID)
		}
	}
	require.True(t, publicGroupFound)
	require.True(t, subscribedGroupFound)
}

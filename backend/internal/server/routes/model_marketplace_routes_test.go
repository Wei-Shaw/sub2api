package routes

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
	userhandler "github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

const modelMarketplaceRouteJWTSecret = "test-jwt-secret-32bytes-long!!!"

type modelMarketplaceRouteSettingRepoStub struct {
	service.SettingRepository
	values map[string]string
}

func (s *modelMarketplaceRouteSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

type modelMarketplaceRouteChannelRepoStub struct {
	service.ChannelRepository
	channels []service.Channel
}

func (s *modelMarketplaceRouteChannelRepoStub) ListAll(context.Context) ([]service.Channel, error) {
	return append([]service.Channel(nil), s.channels...), nil
}

type modelMarketplaceRouteGroupRepoStub struct {
	service.GroupRepository
	groups []service.Group
}

func (s *modelMarketplaceRouteGroupRepoStub) ListActive(context.Context) ([]service.Group, error) {
	return append([]service.Group(nil), s.groups...), nil
}

type modelMarketplaceRouteUserRepoStub struct {
	service.UserRepository
	user *service.User
}

func (s *modelMarketplaceRouteUserRepoStub) GetByID(_ context.Context, id int64) (*service.User, error) {
	if s.user == nil || s.user.ID != id {
		return nil, errors.New("user not found")
	}
	copyUser := *s.user
	copyUser.AllowedGroups = append([]int64(nil), s.user.AllowedGroups...)
	return &copyUser, nil
}

func (s *modelMarketplaceRouteUserRepoStub) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

func (s *modelMarketplaceRouteUserRepoStub) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	return nil
}

type modelMarketplaceRouteUserSubscriptionRepoStub struct {
	service.UserSubscriptionRepository
	activeByUser map[int64][]service.UserSubscription
}

func (s *modelMarketplaceRouteUserSubscriptionRepoStub) ListActiveByUserID(_ context.Context, userID int64) ([]service.UserSubscription, error) {
	items := s.activeByUser[userID]
	out := make([]service.UserSubscription, len(items))
	copy(out, items)
	return out, nil
}

type modelMarketplaceRouteUserGroupRateRepoStub struct {
	service.UserGroupRateRepository
	ratesByUser map[int64]map[int64]float64
}

func (s *modelMarketplaceRouteUserGroupRateRepoStub) GetByUserID(_ context.Context, userID int64) (map[int64]float64, error) {
	items := s.ratesByUser[userID]
	out := make(map[int64]float64, len(items))
	for groupID, rate := range items {
		out[groupID] = rate
	}
	return out, nil
}

type modelMarketplaceRouteFixture struct {
	UserID                   int64
	AnthropicPublicGroupID   int64
	AnthropicSubGroupID      int64
	AnthropicSecondGroupID   int64
	AnthropicHiddenGroupID   int64
	OpenAIPurchasableGroupID int64
	OpenAIPlanID             int64
	UserRateMultiplier       float64
	SubscriptionID           int64
}

type modelMarketplaceRouteScenario struct {
	settingService *service.SettingService
	handlers       *userhandler.Handlers
	fixture        modelMarketplaceRouteFixture
	user           *service.User
	userRepo       *modelMarketplaceRouteUserRepoStub
}

type modelMarketplaceRouteResponse struct {
	Code int `json:"code"`
	Data struct {
		Auth struct {
			Authenticated bool  `json:"authenticated"`
			UserID        int64 `json:"user_id"`
		} `json:"auth"`
		Models []modelMarketplaceRouteModel `json:"models"`
	} `json:"data"`
}

type modelMarketplaceRouteModel struct {
	Name        string                          `json:"name"`
	Platform    string                          `json:"platform"`
	AccessState string                          `json:"access_state"`
	Pricing     *modelMarketplaceRoutePricing   `json:"pricing"`
	Channels    []modelMarketplaceRouteChannel  `json:"channels"`
	Groups      []modelMarketplaceRouteGroupDTO `json:"groups"`
}

type modelMarketplaceRoutePricing struct {
	BillingMode string                                 `json:"billing_mode"`
	InputPrice  *float64                               `json:"input_price"`
	OutputPrice *float64                               `json:"output_price"`
	Intervals   []modelMarketplaceRoutePricingInterval `json:"intervals"`
}

type modelMarketplaceRoutePricingInterval struct {
	MinTokens int    `json:"min_tokens"`
	MaxTokens *int   `json:"max_tokens"`
	TierLabel string `json:"tier_label"`
}

type modelMarketplaceRouteChannel struct {
	Name string `json:"name"`
}

type modelMarketplaceRouteGroupDTO struct {
	ID                 int64                              `json:"id"`
	Name               string                             `json:"name"`
	Platform           string                             `json:"platform"`
	SubscriptionType   string                             `json:"subscription_type"`
	RateMultiplier     float64                            `json:"rate_multiplier"`
	UserRateMultiplier *float64                           `json:"user_rate_multiplier"`
	IsExclusive        bool                               `json:"is_exclusive"`
	AccessState        string                             `json:"access_state"`
	ActiveSubscription *modelMarketplaceRouteSubscription `json:"active_subscription"`
	Plans              []modelMarketplaceRoutePlan        `json:"plans"`
}

type modelMarketplaceRouteSubscription struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type modelMarketplaceRoutePlan struct {
	ID           int64    `json:"id"`
	GroupID      int64    `json:"group_id"`
	Name         string   `json:"name"`
	Price        float64  `json:"price"`
	ValidityDays int      `json:"validity_days"`
	ValidityUnit string   `json:"validity_unit"`
	Features     []string `json:"features"`
	ProductName  string   `json:"product_name"`
}

func newModelMarketplaceRoutesTestRouter(enabled, public bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	settingService := service.NewSettingService(&modelMarketplaceRouteSettingRepoStub{
		values: map[string]string{
			service.SettingKeyAvailableChannelsEnabled:      routeBoolString(enabled),
			service.SettingKeyModelMarketplacePublicEnabled: routeBoolString(public),
		},
	}, &config.Config{})
	groupRepo := &modelMarketplaceRouteGroupRepoStub{groups: []service.Group{
		{
			ID:               1,
			Name:             "public-standard",
			Platform:         service.PlatformAnthropic,
			Status:           service.StatusActive,
			SubscriptionType: service.SubscriptionTypeStandard,
			RateMultiplier:   1.1,
		},
	}}
	channelService := service.NewChannelService(&modelMarketplaceRouteChannelRepoStub{channels: []service.Channel{
		{
			ID:          1,
			Name:        "demo-channel",
			Description: "demo description",
			Status:      service.StatusActive,
			GroupIDs:    []int64{1},
			ModelPricing: []service.ChannelModelPricing{
				{
					Platform: service.PlatformAnthropic,
					Models:   []string{"claude-sonnet-4-6"},
				},
			},
		},
	}}, groupRepo, nil, nil)
	modelMarketplaceHandler := userhandler.NewModelMarketplaceHandler(channelService, nil, nil, nil, settingService)

	router := gin.New()
	v1 := router.Group("/api/v1")
	RegisterUserRoutes(v1, &userhandler.Handlers{
		User:             &userhandler.UserHandler{},
		APIKey:           &userhandler.APIKeyHandler{},
		Usage:            &userhandler.UsageHandler{},
		Announcement:     &userhandler.AnnouncementHandler{},
		Redeem:           &userhandler.RedeemHandler{},
		Subscription:     &userhandler.SubscriptionHandler{},
		ChannelMonitor:   &userhandler.ChannelMonitorUserHandler{},
		Totp:             &userhandler.TotpHandler{},
		AvailableChannel: &userhandler.AvailableChannelHandler{},
		ModelMarketplace: modelMarketplaceHandler,
	},
		servermiddleware.JWTAuthMiddleware(func(c *gin.Context) {
			c.Next()
		}),
		servermiddleware.OptionalJWTAuthMiddleware(func(c *gin.Context) {
			c.Next()
		}),
		settingService,
	)
	return router
}

func routeBoolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func newModelMarketplaceRouteTestRouter(
	settingService *service.SettingService,
	handlers *userhandler.Handlers,
	jwtAuth servermiddleware.JWTAuthMiddleware,
	optionalJWTAuth servermiddleware.OptionalJWTAuthMiddleware,
) *gin.Engine {
	router := gin.New()
	v1 := router.Group("/api/v1")
	RegisterUserRoutes(v1, handlers, jwtAuth, optionalJWTAuth, settingService)
	return router
}

func newRichModelMarketplaceRouteScenario(t *testing.T, public bool) *modelMarketplaceRouteScenario {
	t.Helper()
	gin.SetMode(gin.TestMode)

	settingService := service.NewSettingService(&modelMarketplaceRouteSettingRepoStub{
		values: map[string]string{
			service.SettingKeyAvailableChannelsEnabled:      routeBoolString(true),
			service.SettingKeyModelMarketplacePublicEnabled: routeBoolString(public),
		},
	}, &config.Config{})

	entClient := newModelMarketplaceEntTestClient(t)
	ctx := context.Background()

	publicAnthropicGroupID := mustCreateModelMarketplaceEntGroup(t, ctx, entClient, "public-anthropic", service.PlatformAnthropic, service.SubscriptionTypeStandard, 1.1, false)
	subscribedAnthropicGroupID := mustCreateModelMarketplaceEntGroup(t, ctx, entClient, "subscriber-anthropic", service.PlatformAnthropic, service.SubscriptionTypeSubscription, 1.3, true)
	purchasableOpenAIGroupID := mustCreateModelMarketplaceEntGroup(t, ctx, entClient, "openai-pro", service.PlatformOpenAI, service.SubscriptionTypeSubscription, 1.6, true)
	secondAnthropicGroupID := mustCreateModelMarketplaceEntGroup(t, ctx, entClient, "public-anthropic-plus", service.PlatformAnthropic, service.SubscriptionTypeStandard, 1.05, false)
	hiddenAnthropicGroupID := mustCreateModelMarketplaceEntGroup(t, ctx, entClient, "hidden-exclusive", service.PlatformAnthropic, service.SubscriptionTypeStandard, 2.0, true)

	planID := mustCreateModelMarketplacePlan(t, ctx, entClient, purchasableOpenAIGroupID)

	groups := []service.Group{
		{ID: publicAnthropicGroupID, Name: "public-anthropic", Platform: service.PlatformAnthropic, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 1.1, IsExclusive: false},
		{ID: subscribedAnthropicGroupID, Name: "subscriber-anthropic", Platform: service.PlatformAnthropic, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeSubscription, RateMultiplier: 1.3, IsExclusive: true},
		{ID: purchasableOpenAIGroupID, Name: "openai-pro", Platform: service.PlatformOpenAI, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeSubscription, RateMultiplier: 1.6, IsExclusive: true},
		{ID: secondAnthropicGroupID, Name: "public-anthropic-plus", Platform: service.PlatformAnthropic, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 1.05, IsExclusive: false},
		{ID: hiddenAnthropicGroupID, Name: "hidden-exclusive", Platform: service.PlatformAnthropic, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 2.0, IsExclusive: true},
	}

	channelService := service.NewChannelService(&modelMarketplaceRouteChannelRepoStub{channels: []service.Channel{
		{
			ID:          1,
			Name:        "alpha",
			Description: "alpha description",
			Status:      service.StatusActive,
			GroupIDs:    []int64{publicAnthropicGroupID, subscribedAnthropicGroupID, purchasableOpenAIGroupID, hiddenAnthropicGroupID},
			ModelPricing: []service.ChannelModelPricing{
				{
					Platform:    service.PlatformAnthropic,
					Models:      []string{"claude-sonnet-4-6"},
					BillingMode: service.BillingModeToken,
					InputPrice:  modelMarketplaceRouteFloat64Ptr(0.0012),
					OutputPrice: modelMarketplaceRouteFloat64Ptr(0.0045),
					Intervals: []service.PricingInterval{{
						MinTokens:   0,
						MaxTokens:   modelMarketplaceRouteIntPtr(200000),
						InputPrice:  modelMarketplaceRouteFloat64Ptr(0.0011),
						OutputPrice: modelMarketplaceRouteFloat64Ptr(0.004),
					}},
				},
				{
					Platform:        service.PlatformOpenAI,
					Models:          []string{"gpt-4o"},
					BillingMode:     service.BillingModePerRequest,
					PerRequestPrice: modelMarketplaceRouteFloat64Ptr(0.42),
					Intervals: []service.PricingInterval{{
						TierLabel:       "128k",
						PerRequestPrice: modelMarketplaceRouteFloat64Ptr(0.42),
					}},
				},
			},
		},
		{
			ID:          2,
			Name:        "beta",
			Description: "beta description",
			Status:      service.StatusActive,
			GroupIDs:    []int64{publicAnthropicGroupID, secondAnthropicGroupID},
			ModelPricing: []service.ChannelModelPricing{{
				Platform: service.PlatformAnthropic,
				Models:   []string{"claude-sonnet-4-6"},
			}},
		},
	}}, &modelMarketplaceRouteGroupRepoStub{groups: groups}, nil, nil)

	userID := int64(42)
	userRepo := &modelMarketplaceRouteUserRepoStub{user: &service.User{
		ID:           userID,
		Email:        "user@example.com",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		TokenVersion: 1,
	}}
	subscriptionID := int64(9001)
	userSubRepo := &modelMarketplaceRouteUserSubscriptionRepoStub{activeByUser: map[int64][]service.UserSubscription{
		userID: {{
			ID:              subscriptionID,
			UserID:          userID,
			GroupID:         subscribedAnthropicGroupID,
			Status:          service.SubscriptionStatusActive,
			StartsAt:        time.Now().Add(-2 * time.Hour),
			ExpiresAt:       time.Now().Add(48 * time.Hour),
			DailyUsageUSD:   1.25,
			WeeklyUsageUSD:  2.5,
			MonthlyUsageUSD: 3.75,
		}},
	}}
	userRate := 1.27
	userGroupRateRepo := &modelMarketplaceRouteUserGroupRateRepoStub{ratesByUser: map[int64]map[int64]float64{
		userID: {
			publicAnthropicGroupID: userRate,
		},
	}}
	apiKeyService := service.NewAPIKeyService(nil, userRepo, &modelMarketplaceRouteGroupRepoStub{groups: groups}, userSubRepo, userGroupRateRepo, nil, &config.Config{})
	subscriptionService := service.NewSubscriptionService(&modelMarketplaceRouteGroupRepoStub{groups: groups}, userSubRepo, nil, nil, &config.Config{})
	paymentConfigService := service.NewPaymentConfigService(entClient, nil, nil)
	modelMarketplaceHandler := userhandler.NewModelMarketplaceHandler(channelService, apiKeyService, subscriptionService, paymentConfigService, settingService)
	handlers := &userhandler.Handlers{
		User:             &userhandler.UserHandler{},
		APIKey:           &userhandler.APIKeyHandler{},
		Usage:            &userhandler.UsageHandler{},
		Announcement:     &userhandler.AnnouncementHandler{},
		Redeem:           &userhandler.RedeemHandler{},
		Subscription:     &userhandler.SubscriptionHandler{},
		ChannelMonitor:   &userhandler.ChannelMonitorUserHandler{},
		Totp:             &userhandler.TotpHandler{},
		AvailableChannel: &userhandler.AvailableChannelHandler{},
		ModelMarketplace: modelMarketplaceHandler,
	}

	return &modelMarketplaceRouteScenario{
		settingService: settingService,
		handlers:       handlers,
		user:           userRepo.user,
		userRepo:       userRepo,
		fixture: modelMarketplaceRouteFixture{
			UserID:                   userID,
			AnthropicPublicGroupID:   publicAnthropicGroupID,
			AnthropicSubGroupID:      subscribedAnthropicGroupID,
			AnthropicSecondGroupID:   secondAnthropicGroupID,
			AnthropicHiddenGroupID:   hiddenAnthropicGroupID,
			OpenAIPurchasableGroupID: purchasableOpenAIGroupID,
			OpenAIPlanID:             planID,
			UserRateMultiplier:       userRate,
			SubscriptionID:           subscriptionID,
		},
	}
}

func newRichModelMarketplaceRoutesTestRouter(t *testing.T, public, authenticated bool) (*gin.Engine, modelMarketplaceRouteFixture) {
	t.Helper()
	scenario := newRichModelMarketplaceRouteScenario(t, public)
	router := newModelMarketplaceRouteTestRouter(
		scenario.settingService,
		scenario.handlers,
		servermiddleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }),
		servermiddleware.OptionalJWTAuthMiddleware(func(c *gin.Context) {
			if authenticated {
				c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: scenario.fixture.UserID})
			}
			c.Next()
		}),
	)
	return router, scenario.fixture
}

func newModelMarketplaceRouteAuthService(userRepo *modelMarketplaceRouteUserRepoStub, accessTokenExpireMinutes, expireHour int) *service.AuthService {
	cfg := &config.Config{}
	cfg.JWT.Secret = modelMarketplaceRouteJWTSecret
	cfg.JWT.AccessTokenExpireMinutes = accessTokenExpireMinutes
	cfg.JWT.ExpireHour = expireHour
	return service.NewAuthService(nil, userRepo, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil)
}

func newRichModelMarketplaceRoutesTestRouterWithRealOptionalJWTScenario(t *testing.T, public bool) (*gin.Engine, *modelMarketplaceRouteScenario, string) {
	t.Helper()
	scenario := newRichModelMarketplaceRouteScenario(t, public)
	authService := newModelMarketplaceRouteAuthService(scenario.userRepo, 60, 0)
	userService := service.NewUserService(scenario.userRepo, nil, nil, nil)
	optionalJWTAuth := servermiddleware.NewOptionalJWTAuthMiddleware(authService, userService)
	token, err := authService.GenerateToken(scenario.user)
	require.NoError(t, err)
	router := newModelMarketplaceRouteTestRouter(
		scenario.settingService,
		scenario.handlers,
		servermiddleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }),
		optionalJWTAuth,
	)
	return router, scenario, token
}

func newRichModelMarketplaceRoutesTestRouterWithRealOptionalJWT(t *testing.T, public bool) (*gin.Engine, modelMarketplaceRouteFixture, string) {
	t.Helper()
	router, scenario, token := newRichModelMarketplaceRoutesTestRouterWithRealOptionalJWTScenario(t, public)
	return router, scenario.fixture, token
}

func newModelMarketplaceEntTestClient(t *testing.T) *dbent.Client {
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

func mustCreateModelMarketplaceEntGroup(t *testing.T, ctx context.Context, client *dbent.Client, name, platform, subscriptionType string, rate float64, exclusive bool) int64 {
	t.Helper()
	groupEnt, err := client.Group.Create().
		SetName(name).
		SetPlatform(platform).
		SetStatus(service.StatusActive).
		SetSubscriptionType(subscriptionType).
		SetRateMultiplier(rate).
		SetIsExclusive(exclusive).
		Save(ctx)
	require.NoError(t, err)
	return groupEnt.ID
}

func mustCreateModelMarketplacePlan(t *testing.T, ctx context.Context, client *dbent.Client, groupID int64) int64 {
	t.Helper()
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(groupID).
		SetName("OpenAI Pro 30D").
		SetDescription("OpenAI access plan").
		SetPrice(19.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetFeatures("Priority access\nOpenAI models").
		SetProductName("OpenAI Pro Subscription").
		SetForSale(true).
		SetSortOrder(10).
		Save(ctx)
	require.NoError(t, err)
	return int64(plan.ID)
}

func modelMarketplaceRouteFloat64Ptr(v float64) *float64 { return &v }

func modelMarketplaceRouteIntPtr(v int) *int { return &v }

func findMarketplaceRouteModel(models []modelMarketplaceRouteModel, platform, name string) *modelMarketplaceRouteModel {
	for i := range models {
		if models[i].Platform == platform && models[i].Name == name {
			return &models[i]
		}
	}
	return nil
}

func findMarketplaceRouteGroup(groups []modelMarketplaceRouteGroupDTO, groupID int64) *modelMarketplaceRouteGroupDTO {
	for i := range groups {
		if groups[i].ID == groupID {
			return &groups[i]
		}
	}
	return nil
}

func assertMarketplaceRouteGuestView(t *testing.T, body []byte, fixture modelMarketplaceRouteFixture) {
	t.Helper()
	var raw struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &raw))
	require.Equal(t, 0, raw.Code)

	auth, ok := raw.Data["auth"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, auth["authenticated"])
	_, hasUserID := auth["user_id"]
	require.False(t, hasUserID)

	models, ok := raw.Data["models"].([]any)
	require.True(t, ok)
	var anthropicModel map[string]any
	for _, item := range models {
		modelMap, ok := item.(map[string]any)
		require.True(t, ok)
		if modelMap["platform"] == service.PlatformAnthropic && modelMap["name"] == "claude-sonnet-4-6" {
			anthropicModel = modelMap
			break
		}
	}
	require.NotNil(t, anthropicModel)
	groups, ok := anthropicModel["groups"].([]any)
	require.True(t, ok)
	for _, item := range groups {
		groupMap, ok := item.(map[string]any)
		require.True(t, ok)
		require.NotEqual(t, float64(fixture.AnthropicHiddenGroupID), groupMap["id"])
		_, hasUserRate := groupMap["user_rate_multiplier"]
		_, hasActiveSub := groupMap["active_subscription"]
		require.False(t, hasUserRate)
		require.False(t, hasActiveSub)
	}
}

func TestUserRoutesModelMarketplace_FeatureDisabledReturnsEmptyList(t *testing.T) {
	router := newModelMarketplaceRoutesTestRouter(false, true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)

	router.ServeHTTP(w, req)

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

func TestUserRoutesModelMarketplace_PrivateAnonymousReturns401(t *testing.T) {
	router := newModelMarketplaceRoutesTestRouter(true, false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestUserRoutesModelMarketplace_PublicAnonymousReturnsSuccess(t *testing.T) {
	router := newModelMarketplaceRoutesTestRouter(true, true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Auth struct {
				Authenticated bool `json:"authenticated"`
			} `json:"auth"`
			Models []struct {
				Name        string `json:"name"`
				AccessState string `json:"access_state"`
				Channels    []struct {
					Name string `json:"name"`
				} `json:"channels"`
			} `json:"models"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.False(t, resp.Data.Auth.Authenticated)
	require.Len(t, resp.Data.Models, 1)
	require.Equal(t, "claude-sonnet-4-6", resp.Data.Models[0].Name)
	require.Equal(t, "available", resp.Data.Models[0].AccessState)
	require.Len(t, resp.Data.Models[0].Channels, 1)
	require.Equal(t, "demo-channel", resp.Data.Models[0].Channels[0].Name)
}

func TestUserRoutesModelMarketplace_AuthenticatedResponseIncludesPricingPlansAndAggregatedChannels(t *testing.T) {
	router, fixture := newRichModelMarketplaceRoutesTestRouter(t, true, true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp modelMarketplaceRouteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.Auth.Authenticated)
	require.Equal(t, fixture.UserID, resp.Data.Auth.UserID)
	require.Len(t, resp.Data.Models, 2)

	anthropic := findMarketplaceRouteModel(resp.Data.Models, service.PlatformAnthropic, "claude-sonnet-4-6")
	require.NotNil(t, anthropic)
	require.Equal(t, "subscribed", anthropic.AccessState)
	require.NotNil(t, anthropic.Pricing)
	require.Equal(t, string(service.BillingModeToken), anthropic.Pricing.BillingMode)
	require.NotNil(t, anthropic.Pricing.InputPrice)
	require.NotNil(t, anthropic.Pricing.OutputPrice)
	require.Len(t, anthropic.Pricing.Intervals, 1)
	require.Equal(t, 0, anthropic.Pricing.Intervals[0].MinTokens)
	require.NotNil(t, anthropic.Pricing.Intervals[0].MaxTokens)
	require.Equal(t, 200000, *anthropic.Pricing.Intervals[0].MaxTokens)
	require.Len(t, anthropic.Channels, 2)
	require.ElementsMatch(t, []string{"alpha", "beta"}, []string{anthropic.Channels[0].Name, anthropic.Channels[1].Name})
	require.Len(t, anthropic.Groups, 3)
	require.NotNil(t, findMarketplaceRouteGroup(anthropic.Groups, fixture.AnthropicPublicGroupID))
	require.NotNil(t, findMarketplaceRouteGroup(anthropic.Groups, fixture.AnthropicSubGroupID))
	require.NotNil(t, findMarketplaceRouteGroup(anthropic.Groups, fixture.AnthropicSecondGroupID))
	require.Nil(t, findMarketplaceRouteGroup(anthropic.Groups, fixture.AnthropicHiddenGroupID))

	publicGroup := findMarketplaceRouteGroup(anthropic.Groups, fixture.AnthropicPublicGroupID)
	require.NotNil(t, publicGroup)
	require.Equal(t, "available", publicGroup.AccessState)
	require.NotNil(t, publicGroup.UserRateMultiplier)
	require.InDelta(t, fixture.UserRateMultiplier, *publicGroup.UserRateMultiplier, 0.0001)
	require.Nil(t, publicGroup.ActiveSubscription)

	subscribedGroup := findMarketplaceRouteGroup(anthropic.Groups, fixture.AnthropicSubGroupID)
	require.NotNil(t, subscribedGroup)
	require.Equal(t, "subscribed", subscribedGroup.AccessState)
	require.NotNil(t, subscribedGroup.ActiveSubscription)
	require.Equal(t, fixture.SubscriptionID, subscribedGroup.ActiveSubscription.ID)
	require.Equal(t, service.SubscriptionStatusActive, subscribedGroup.ActiveSubscription.Status)

	openAI := findMarketplaceRouteModel(resp.Data.Models, service.PlatformOpenAI, "gpt-4o")
	require.NotNil(t, openAI)
	require.Equal(t, "purchasable", openAI.AccessState)
	require.Len(t, openAI.Channels, 1)
	require.Equal(t, "alpha", openAI.Channels[0].Name)
	require.Len(t, openAI.Groups, 1)
	purchasableGroup := findMarketplaceRouteGroup(openAI.Groups, fixture.OpenAIPurchasableGroupID)
	require.NotNil(t, purchasableGroup)
	require.Equal(t, service.PlatformOpenAI, purchasableGroup.Platform)
	require.Equal(t, "purchasable", purchasableGroup.AccessState)
	require.Len(t, purchasableGroup.Plans, 1)
	require.Equal(t, fixture.OpenAIPlanID, purchasableGroup.Plans[0].ID)
	require.Equal(t, []string{"Priority access", "OpenAI models"}, purchasableGroup.Plans[0].Features)
	require.Equal(t, "OpenAI Pro Subscription", purchasableGroup.Plans[0].ProductName)
}

func TestUserRoutesModelMarketplace_PublicAnonymousOmitsPrivateFieldsAndKeepsPlatformScopedPlans(t *testing.T) {
	router, fixture := newRichModelMarketplaceRoutesTestRouter(t, true, false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var raw struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	require.Equal(t, 0, raw.Code)

	auth, ok := raw.Data["auth"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, auth["authenticated"])
	_, hasUserID := auth["user_id"]
	require.False(t, hasUserID)

	models, ok := raw.Data["models"].([]any)
	require.True(t, ok)
	require.Len(t, models, 2)

	var anthropicModel map[string]any
	var openAIModel map[string]any
	for _, item := range models {
		modelMap, ok := item.(map[string]any)
		require.True(t, ok)
		if modelMap["platform"] == service.PlatformAnthropic && modelMap["name"] == "claude-sonnet-4-6" {
			anthropicModel = modelMap
		}
		if modelMap["platform"] == service.PlatformOpenAI && modelMap["name"] == "gpt-4o" {
			openAIModel = modelMap
		}
	}
	require.NotNil(t, anthropicModel)
	require.NotNil(t, openAIModel)
	require.Equal(t, "available", anthropicModel["access_state"])
	require.Equal(t, "purchasable", openAIModel["access_state"])

	anthropicGroups, ok := anthropicModel["groups"].([]any)
	require.True(t, ok)
	require.Len(t, anthropicGroups, 2)
	for _, item := range anthropicGroups {
		groupMap, ok := item.(map[string]any)
		require.True(t, ok)
		require.Equal(t, service.PlatformAnthropic, groupMap["platform"])
		_, hasUserRate := groupMap["user_rate_multiplier"]
		_, hasActiveSub := groupMap["active_subscription"]
		require.False(t, hasUserRate)
		require.False(t, hasActiveSub)
		require.NotEqual(t, float64(fixture.AnthropicHiddenGroupID), groupMap["id"])
	}

	openAIGroups, ok := openAIModel["groups"].([]any)
	require.True(t, ok)
	require.Len(t, openAIGroups, 1)
	groupMap, ok := openAIGroups[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(fixture.OpenAIPurchasableGroupID), groupMap["id"])
	require.Equal(t, service.PlatformOpenAI, groupMap["platform"])
	plans, ok := groupMap["plans"].([]any)
	require.True(t, ok)
	require.Len(t, plans, 1)
	planMap, ok := plans[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(fixture.OpenAIPlanID), planMap["id"])
	features, ok := planMap["features"].([]any)
	require.True(t, ok)
	require.Equal(t, []any{"Priority access", "OpenAI models"}, features)
	_, hasUserRate := groupMap["user_rate_multiplier"]
	_, hasActiveSub := groupMap["active_subscription"]
	require.False(t, hasUserRate)
	require.False(t, hasActiveSub)
}

func TestUserRoutesModelMarketplace_RealOptionalJWTValidTokenGetsEnhancedView(t *testing.T) {
	router, fixture, token := newRichModelMarketplaceRoutesTestRouterWithRealOptionalJWT(t, true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp modelMarketplaceRouteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.Auth.Authenticated)
	require.Equal(t, fixture.UserID, resp.Data.Auth.UserID)

	anthropic := findMarketplaceRouteModel(resp.Data.Models, service.PlatformAnthropic, "claude-sonnet-4-6")
	require.NotNil(t, anthropic)
	publicGroup := findMarketplaceRouteGroup(anthropic.Groups, fixture.AnthropicPublicGroupID)
	require.NotNil(t, publicGroup)
	require.NotNil(t, publicGroup.UserRateMultiplier)
	require.InDelta(t, fixture.UserRateMultiplier, *publicGroup.UserRateMultiplier, 0.0001)

	subscribedGroup := findMarketplaceRouteGroup(anthropic.Groups, fixture.AnthropicSubGroupID)
	require.NotNil(t, subscribedGroup)
	require.NotNil(t, subscribedGroup.ActiveSubscription)
	require.Equal(t, fixture.SubscriptionID, subscribedGroup.ActiveSubscription.ID)
}

func TestUserRoutesModelMarketplace_RealOptionalJWTMissingOrInvalidTokenFallsBackToGuestView(t *testing.T) {
	router, fixture, _ := newRichModelMarketplaceRoutesTestRouterWithRealOptionalJWT(t, true)

	tests := []struct {
		name   string
		header string
	}{
		{name: "missing token"},
		{name: "invalid token", header: "Bearer invalid-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}

			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			assertMarketplaceRouteGuestView(t, w.Body.Bytes(), fixture)
		})
	}
}

func TestUserRoutesModelMarketplace_RealOptionalJWTExpiredTokenFallsBackToGuestView(t *testing.T) {
	router, scenario, _ := newRichModelMarketplaceRoutesTestRouterWithRealOptionalJWTScenario(t, true)
	expiredToken, err := newModelMarketplaceRouteAuthService(scenario.userRepo, 0, -1).GenerateToken(scenario.user)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assertMarketplaceRouteGuestView(t, w.Body.Bytes(), scenario.fixture)
}

func TestUserRoutesModelMarketplace_RealOptionalJWTVersionMismatchFallsBackToGuestView(t *testing.T) {
	router, scenario, token := newRichModelMarketplaceRoutesTestRouterWithRealOptionalJWTScenario(t, true)
	scenario.user.TokenVersion = 2

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assertMarketplaceRouteGuestView(t, w.Body.Bytes(), scenario.fixture)
}

func TestUserRoutesModelMarketplace_RealOptionalJWTInactiveUserFallsBackToGuestView(t *testing.T) {
	router, scenario, token := newRichModelMarketplaceRoutesTestRouterWithRealOptionalJWTScenario(t, true)
	scenario.user.Status = service.StatusDisabled

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/models", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assertMarketplaceRouteGuestView(t, w.Body.Bytes(), scenario.fixture)
}

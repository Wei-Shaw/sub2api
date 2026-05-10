package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type providerPriceGroupRepoStub struct {
	groups []service.Group
}

func (s *providerPriceGroupRepoStub) Create(context.Context, *service.Group) error { panic("unexpected call") }
func (s *providerPriceGroupRepoStub) GetByID(context.Context, int64) (*service.Group, error) {
	panic("unexpected call")
}
func (s *providerPriceGroupRepoStub) GetByIDLite(context.Context, int64) (*service.Group, error) {
	panic("unexpected call")
}
func (s *providerPriceGroupRepoStub) Update(context.Context, *service.Group) error { panic("unexpected call") }
func (s *providerPriceGroupRepoStub) Delete(context.Context, int64) error          { panic("unexpected call") }
func (s *providerPriceGroupRepoStub) DeleteCascade(context.Context, int64) ([]int64, error) {
	panic("unexpected call")
}
func (s *providerPriceGroupRepoStub) List(context.Context, pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	panic("unexpected call")
}
func (s *providerPriceGroupRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]service.Group, *pagination.PaginationResult, error) {
	panic("unexpected call")
}
func (s *providerPriceGroupRepoStub) ListActive(context.Context) ([]service.Group, error) {
	return s.groups, nil
}
func (s *providerPriceGroupRepoStub) ListActiveByPlatform(context.Context, string) ([]service.Group, error) {
	panic("unexpected call")
}
func (s *providerPriceGroupRepoStub) ExistsByName(context.Context, string) (bool, error) { panic("unexpected call") }
func (s *providerPriceGroupRepoStub) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("unexpected call")
}
func (s *providerPriceGroupRepoStub) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected call")
}
func (s *providerPriceGroupRepoStub) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("unexpected call")
}
func (s *providerPriceGroupRepoStub) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("unexpected call")
}
func (s *providerPriceGroupRepoStub) UpdateSortOrders(context.Context, []service.GroupSortOrderUpdate) error {
	panic("unexpected call")
}

type providerPriceChannelRepoStub struct {
	channel *service.Channel
}

func (s *providerPriceChannelRepoStub) Create(context.Context, *service.Channel) error { panic("unexpected call") }
func (s *providerPriceChannelRepoStub) GetByID(context.Context, int64) (*service.Channel, error) {
	return s.channel, nil
}
func (s *providerPriceChannelRepoStub) Update(context.Context, *service.Channel) error { panic("unexpected call") }
func (s *providerPriceChannelRepoStub) Delete(context.Context, int64) error            { panic("unexpected call") }
func (s *providerPriceChannelRepoStub) List(context.Context, pagination.PaginationParams, string, string) ([]service.Channel, *pagination.PaginationResult, error) {
	panic("unexpected call")
}
func (s *providerPriceChannelRepoStub) ListAll(context.Context) ([]service.Channel, error) {
	if s.channel == nil {
		return nil, nil
	}
	return []service.Channel{*s.channel}, nil
}
func (s *providerPriceChannelRepoStub) ExistsByName(context.Context, string) (bool, error) {
	panic("unexpected call")
}
func (s *providerPriceChannelRepoStub) ExistsByNameExcluding(context.Context, string, int64) (bool, error) {
	panic("unexpected call")
}
func (s *providerPriceChannelRepoStub) GetGroupIDs(context.Context, int64) ([]int64, error) {
	panic("unexpected call")
}
func (s *providerPriceChannelRepoStub) SetGroupIDs(context.Context, int64, []int64) error {
	panic("unexpected call")
}
func (s *providerPriceChannelRepoStub) GetChannelIDByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected call")
}
func (s *providerPriceChannelRepoStub) GetGroupsInOtherChannels(context.Context, int64, []int64) ([]int64, error) {
	panic("unexpected call")
}
func (s *providerPriceChannelRepoStub) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	if s.channel == nil {
		return nil, nil
	}
	out := make(map[int64]string, len(s.channel.GroupIDs))
	for _, groupID := range s.channel.GroupIDs {
		out[groupID] = service.PlatformOpenAI
	}
	return out, nil
}
func (s *providerPriceChannelRepoStub) ListModelPricing(context.Context, int64) ([]service.ChannelModelPricing, error) {
	panic("unexpected call")
}
func (s *providerPriceChannelRepoStub) CreateModelPricing(context.Context, *service.ChannelModelPricing) error {
	panic("unexpected call")
}
func (s *providerPriceChannelRepoStub) UpdateModelPricing(context.Context, *service.ChannelModelPricing) error {
	panic("unexpected call")
}
func (s *providerPriceChannelRepoStub) DeleteModelPricing(context.Context, int64) error {
	panic("unexpected call")
}
func (s *providerPriceChannelRepoStub) ReplaceModelPricing(context.Context, int64, []service.ChannelModelPricing) error {
	panic("unexpected call")
}

type providerPriceSettingRepoStub struct {
	values map[string]string
}

func (s *providerPriceSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}
func (s *providerPriceSettingRepoStub) Get(_ context.Context, key string) (*service.Setting, error) {
	return &service.Setting{Key: key, Value: s.values[key]}, nil
}
func (s *providerPriceSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = s.values[key]
	}
	return out, nil
}
func (s *providerPriceSettingRepoStub) Set(context.Context, string, string) error {
	return nil
}
func (s *providerPriceSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected call")
}
func (s *providerPriceSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected call")
}
func (s *providerPriceSettingRepoStub) Delete(context.Context, string) error { panic("unexpected call") }

func TestProviderPriceHandler_OpenAIGroupExportsExpectedCNYPrices(t *testing.T) {
	t.Helper()

	groupRepo := &providerPriceGroupRepoStub{
		groups: []service.Group{
			{
				ID:               1,
				Name:             "default",
				Platform:         service.PlatformOpenAI,
				RateMultiplier:   1,
				Status:           service.StatusActive,
				SubscriptionType: service.SubscriptionTypeStandard,
			},
		},
	}
	groupService := service.NewGroupService(groupRepo, nil)

	settingRepo := &providerPriceSettingRepoStub{
		values: map[string]string{
			service.SettingBalanceRechargeMult: "2",
			service.SettingKeySiteName:         "Test Site",
			service.SettingKeyFrontendURL:      "https://pricing.example.com",
		},
	}
	cfg := &config.Config{}
	paymentConfigService := service.NewPaymentConfigService(nil, settingRepo, nil)
	settingService := service.NewSettingService(settingRepo, cfg)
	billingService := service.NewBillingService(cfg, nil)
	resolver := service.NewModelPricingResolver(&service.ChannelService{}, billingService)

	h := NewProviderPriceHandler(
		groupService,
		paymentConfigService,
		billingService,
		nil,
		settingService,
		nil,
		resolver,
	)
	h.modelLister = func(context.Context, *service.Group) []string {
		return []string{"gpt-5.4", "gpt-5.5"}
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/provider/pricing", h.GetProviderPricing)

	req := httptest.NewRequest(http.MethodGet, "/api/provider/pricing", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp providerPricingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if !resp.Success || resp.Data == nil {
		t.Fatalf("expected success response, got %+v", resp)
	}
	if resp.Data.SiteName != "Test Site" {
		t.Fatalf("site name mismatch: got %q", resp.Data.SiteName)
	}
	if resp.Data.SiteDomain != "pricing.example.com" {
		t.Fatalf("site domain mismatch: got %q", resp.Data.SiteDomain)
	}

	var got54 *providerPricingModel
	var got55 *providerPricingModel
	for i := range resp.Data.Models {
		switch resp.Data.Models[i].ModelName {
		case "gpt-5.4":
			got54 = &resp.Data.Models[i]
		case "gpt-5.5":
			got55 = &resp.Data.Models[i]
		}
	}

	if got54 == nil {
		t.Fatal("expected gpt-5.4 in response")
	}
	if got54.InputPrice == nil || *got54.InputPrice != 1.25 {
		t.Fatalf("gpt-5.4 input mismatch: got %+v", got54.InputPrice)
	}
	if got54.OutputPrice == nil || *got54.OutputPrice != 7.5 {
		t.Fatalf("gpt-5.4 output mismatch: got %+v", got54.OutputPrice)
	}
	if got54.CacheCreatePrice == nil || *got54.CacheCreatePrice != 1.25 {
		t.Fatalf("gpt-5.4 cache_create_price mismatch: got %+v", got54.CacheCreatePrice)
	}
	if got54.CacheCreatePrice1h != nil {
		t.Fatalf("gpt-5.4 cache_create_price_1h should be nil, got %+v", got54.CacheCreatePrice1h)
	}

	if got55 == nil {
		t.Fatal("expected gpt-5.5 in response")
	}
	if got55.InputPrice == nil || *got55.InputPrice != 2.5 {
		t.Fatalf("gpt-5.5 input mismatch: got %+v", got55.InputPrice)
	}
	if got55.OutputPrice == nil || *got55.OutputPrice != 15 {
		t.Fatalf("gpt-5.5 output mismatch: got %+v", got55.OutputPrice)
	}
	if got55.CacheCreatePrice == nil || *got55.CacheCreatePrice != 2.5 {
		t.Fatalf("gpt-5.5 cache_create_price mismatch: got %+v", got55.CacheCreatePrice)
	}
}

func TestProviderPriceHandler_SkipsTokenIntervalPricingWithoutStablePublicPrice(t *testing.T) {
	group := service.Group{
		ID:               1,
		Name:             "default",
		Platform:         service.PlatformOpenAI,
		RateMultiplier:   1,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}

	h := NewProviderPriceHandler(
		service.NewGroupService(&providerPriceGroupRepoStub{groups: []service.Group{group}}, nil),
		service.NewPaymentConfigService(nil, &providerPriceSettingRepoStub{values: map[string]string{service.SettingBalanceRechargeMult: "2"}}, nil),
		service.NewBillingService(&config.Config{}, nil),
		nil,
		service.NewSettingService(&providerPriceSettingRepoStub{values: map[string]string{}}, &config.Config{}),
		nil,
		nil,
	)
	h.modelLister = func(context.Context, *service.Group) []string { return []string{"gpt-5.4"} }
	h.resolver = &service.ModelPricingResolver{}

	resolved := &service.ResolvedPricing{
		Mode: service.BillingModeToken,
		BasePricing: &service.ModelPricing{
			InputPricePerToken: 2.5e-6,
		},
		Intervals: []service.PricingInterval{
			{
				MinTokens:  0,
				InputPrice: floatPtr(1e-6),
			},
		},
	}
	if price, ok := h.exportPriceForResolved(resolved); ok {
		t.Fatalf("expected interval-priced model to be skipped, got %+v", price)
	}
}

func TestProviderPriceHandler_UsesChannelSupportedModelsWhenGatewayModelsAreEmpty(t *testing.T) {
	groupRepo := &providerPriceGroupRepoStub{
		groups: []service.Group{
			{
				ID:               1,
				Name:             "default",
				Platform:         service.PlatformOpenAI,
				RateMultiplier:   1,
				Status:           service.StatusActive,
				SubscriptionType: service.SubscriptionTypeStandard,
			},
		},
	}
	channelRepo := &providerPriceChannelRepoStub{
		channel: &service.Channel{
			ID:     10,
			Name:   "test-channel",
			Status: service.StatusActive,
			GroupIDs: []int64{1},
			ModelPricing: []service.ChannelModelPricing{
				{
					Platform:    service.PlatformOpenAI,
					Models:      []string{"gpt-5.4"},
					BillingMode: service.BillingModeToken,
					InputPrice:  floatPtr(2.5e-6),
					OutputPrice: floatPtr(15e-6),
				},
			},
			UpdatedAt: time.Now(),
		},
	}

	groupService := service.NewGroupService(groupRepo, nil)
	settingRepo := &providerPriceSettingRepoStub{
		values: map[string]string{
			service.SettingBalanceRechargeMult: "2",
			service.SettingKeySiteName:         "Test Site",
		},
	}
	cfg := &config.Config{}
	billingService := service.NewBillingService(cfg, nil)
	channelService := service.NewChannelService(channelRepo, groupRepo, nil, nil)
	resolver := service.NewModelPricingResolver(channelService, billingService)

	h := NewProviderPriceHandler(
		groupService,
		service.NewPaymentConfigService(nil, settingRepo, nil),
		billingService,
		channelService,
		service.NewSettingService(settingRepo, cfg),
		nil,
		resolver,
	)

	models := h.availableModelsForGroup(context.Background(), &groupRepo.groups[0])
	if len(models) != 1 || models[0] != "gpt-5.4" {
		t.Fatalf("unexpected models: %+v", models)
	}
}

func TestFilterModelsByGroupScope_RespectsImageGenerationGate(t *testing.T) {
	group := &service.Group{
		Platform:             service.PlatformGemini,
		AllowImageGeneration: false,
		SupportedModelScopes: []string{"gemini_text", "gemini_image"},
	}

	models := filterModelsByGroupScope(group, []string{
		"gemini-2.5-flash",
		"gemini-2.5-flash-image",
	})

	if len(models) != 1 || models[0] != "gemini-2.5-flash" {
		t.Fatalf("unexpected filtered models: %+v", models)
	}
}

func TestProviderPriceHandler_PrefersOverridePrices(t *testing.T) {
	groupRepo := &providerPriceGroupRepoStub{
		groups: []service.Group{
			{
				ID:               1,
				Name:             "default",
				Platform:         service.PlatformOpenAI,
				RateMultiplier:   1,
				Status:           service.StatusActive,
				SubscriptionType: service.SubscriptionTypeStandard,
			},
		},
	}

	settingRepo := &providerPriceSettingRepoStub{
		values: map[string]string{
			service.SettingBalanceRechargeMult: "2",
			service.SettingKeySiteName:         "Test Site",
			service.SettingKeyFrontendURL:      "https://pricing.example.com",
			service.SettingKeyProviderPriceOverrides: `[{"id":"ovr-1","group_name":"公开组","model_name":"gpt-5.4","input_price":99.5,"output_price":199.5,"enabled":true,"sort_order":0}]`,
		},
	}
	cfg := &config.Config{}
	h := NewProviderPriceHandler(
		service.NewGroupService(groupRepo, nil),
		service.NewPaymentConfigService(nil, settingRepo, nil),
		service.NewBillingService(cfg, nil),
		nil,
		service.NewSettingService(settingRepo, cfg),
		nil,
		service.NewModelPricingResolver(&service.ChannelService{}, service.NewBillingService(cfg, nil)),
	)
	h.modelLister = func(context.Context, *service.Group) []string {
		return []string{"gpt-5.4"}
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/provider/pricing", h.GetProviderPricing)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/provider/pricing", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var resp providerPricingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Data == nil || len(resp.Data.Models) != 1 {
		t.Fatalf("unexpected data: %+v", resp)
	}
	got := resp.Data.Models[0]
	if got.GroupName != "公开组" || got.ModelName != "gpt-5.4" {
		t.Fatalf("unexpected model identity: %+v", got)
	}
	if got.InputPrice == nil || *got.InputPrice != 99.5 {
		t.Fatalf("unexpected input price: %+v", got.InputPrice)
	}
	if got.OutputPrice == nil || *got.OutputPrice != 199.5 {
		t.Fatalf("unexpected output price: %+v", got.OutputPrice)
	}
}

func TestProviderPriceHandler_FallbacksToAutoWhenOverrideEmpty(t *testing.T) {
	groupRepo := &providerPriceGroupRepoStub{
		groups: []service.Group{
			{
				ID:               1,
				Name:             "default",
				Platform:         service.PlatformOpenAI,
				RateMultiplier:   1,
				Status:           service.StatusActive,
				SubscriptionType: service.SubscriptionTypeStandard,
			},
		},
	}
	settingRepo := &providerPriceSettingRepoStub{
		values: map[string]string{
			service.SettingBalanceRechargeMult: "2",
			service.SettingKeyProviderPriceOverrides: `[]`,
		},
	}
	cfg := &config.Config{}
	billingService := service.NewBillingService(cfg, nil)
	h := NewProviderPriceHandler(
		service.NewGroupService(groupRepo, nil),
		service.NewPaymentConfigService(nil, settingRepo, nil),
		billingService,
		nil,
		service.NewSettingService(settingRepo, cfg),
		nil,
		service.NewModelPricingResolver(&service.ChannelService{}, billingService),
	)
	h.modelLister = func(context.Context, *service.Group) []string {
		return []string{"gpt-5.4"}
	}

	entries := h.buildProviderPriceEntries(context.Background(), groupRepo.groups, 2)
	if len(entries) == 0 {
		t.Fatal("expected auto pricing entries")
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

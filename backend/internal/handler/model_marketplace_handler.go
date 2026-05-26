package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	marketplaceAccessAvailable   = "available"
	marketplaceAccessSubscribed  = "subscribed"
	marketplaceAccessPurchasable = "purchasable"
)

var errModelMarketplaceDependencyMissing = infraerrors.InternalServer(
	"MODEL_MARKETPLACE_DEPENDENCY_MISSING",
	"model marketplace dependencies are not configured",
)

type marketplaceAuthStatus struct {
	Authenticated bool  `json:"authenticated"`
	UserID        int64 `json:"user_id,omitempty"`
}

type marketplacePlan struct {
	ID            int64    `json:"id"`
	GroupID       int64    `json:"group_id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Price         float64  `json:"price"`
	OriginalPrice *float64 `json:"original_price,omitempty"`
	ValidityDays  int      `json:"validity_days"`
	ValidityUnit  string   `json:"validity_unit"`
	Features      []string `json:"features"`
	ProductName   string   `json:"product_name"`
	SortOrder     int      `json:"sort_order"`
}

type marketplaceSubscription struct {
	ID              int64     `json:"id"`
	Status          string    `json:"status"`
	StartsAt        time.Time `json:"starts_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	DailyUsageUSD   float64   `json:"daily_usage_usd"`
	WeeklyUsageUSD  float64   `json:"weekly_usage_usd"`
	MonthlyUsageUSD float64   `json:"monthly_usage_usd"`
}

type marketplaceGroup struct {
	ID                 int64                    `json:"id"`
	Name               string                   `json:"name"`
	Platform           string                   `json:"platform"`
	SubscriptionType   string                   `json:"subscription_type"`
	RateMultiplier     float64                  `json:"rate_multiplier"`
	UserRateMultiplier *float64                 `json:"user_rate_multiplier,omitempty"`
	IsExclusive        bool                     `json:"is_exclusive"`
	AccessState        string                   `json:"access_state"`
	ActiveSubscription *marketplaceSubscription `json:"active_subscription,omitempty"`
	Plans              []marketplacePlan        `json:"plans"`
}

type marketplaceChannel struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type marketplaceModel struct {
	Name        string                     `json:"name"`
	Platform    string                     `json:"platform"`
	Pricing     *userSupportedModelPricing `json:"pricing"`
	Channels    []marketplaceChannel       `json:"channels"`
	Groups      []marketplaceGroup         `json:"groups"`
	AccessState string                     `json:"access_state"`
}

type marketplaceResponse struct {
	Auth   marketplaceAuthStatus `json:"auth"`
	Models []marketplaceModel    `json:"models"`
}

// ModelMarketplaceHandler handles the user/public model marketplace view.
type ModelMarketplaceHandler struct {
	channelService      *service.ChannelService
	apiKeyService       *service.APIKeyService
	subscriptionService *service.SubscriptionService
	paymentConfig       *service.PaymentConfigService
	settingService      *service.SettingService
}

func NewModelMarketplaceHandler(
	channelService *service.ChannelService,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	paymentConfig *service.PaymentConfigService,
	settingService *service.SettingService,
) *ModelMarketplaceHandler {
	return &ModelMarketplaceHandler{
		channelService:      channelService,
		apiKeyService:       apiKeyService,
		subscriptionService: subscriptionService,
		paymentConfig:       paymentConfig,
		settingService:      settingService,
	}
}

// List returns a model-centric marketplace aggregated from channels, groups,
// subscription plans, and the current user's access state when authenticated.
// GET /api/v1/marketplace/models
func (h *ModelMarketplaceHandler) List(c *gin.Context) {
	runtime := h.marketplaceRuntime(c)
	if !runtime.Enabled {
		response.Success(c, marketplaceResponse{Models: []marketplaceModel{}})
		return
	}

	subject, authenticated := middleware.GetAuthSubjectFromContext(c)
	if !authenticated && !runtime.ModelMarketplacePublic {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	view, err := h.buildMarketplace(c, subject.UserID, authenticated)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

func (h *ModelMarketplaceHandler) marketplaceRuntime(c *gin.Context) service.AvailableChannelsRuntime {
	if h.settingService == nil {
		return service.AvailableChannelsRuntime{}
	}
	return h.settingService.GetAvailableChannelsRuntime(c.Request.Context())
}

func (h *ModelMarketplaceHandler) buildMarketplace(c *gin.Context, userID int64, authenticated bool) (marketplaceResponse, error) {
	ctx := c.Request.Context()
	if h.channelService == nil {
		return marketplaceResponse{}, errModelMarketplaceDependencyMissing
	}

	availableGroups := map[int64]service.Group{}
	userRates := map[int64]float64{}
	activeSubs := map[int64]service.UserSubscription{}
	if authenticated {
		if h.apiKeyService == nil || h.subscriptionService == nil {
			return marketplaceResponse{}, errModelMarketplaceDependencyMissing
		}
		groups, err := h.apiKeyService.GetAvailableGroups(ctx, userID)
		if err != nil {
			return marketplaceResponse{}, err
		}
		for i := range groups {
			availableGroups[groups[i].ID] = groups[i]
		}
		rates, err := h.apiKeyService.GetUserGroupRates(ctx, userID)
		if err != nil {
			return marketplaceResponse{}, err
		}
		if rates != nil {
			userRates = rates
		}
		subs, err := h.subscriptionService.ListActiveUserSubscriptions(ctx, userID)
		if err != nil {
			return marketplaceResponse{}, err
		}
		for i := range subs {
			activeSubs[subs[i].GroupID] = subs[i]
		}
	}

	plansByGroup, err := h.salePlansByGroup(ctx)
	if err != nil {
		return marketplaceResponse{}, err
	}

	channels, err := h.channelService.ListAvailable(ctx)
	if err != nil {
		return marketplaceResponse{}, err
	}

	models := make(map[string]*marketplaceModel)
	for _, ch := range channels {
		if ch.Status != service.StatusActive {
			continue
		}
		groupsByPlatform := h.marketplaceGroupsByPlatform(ch.Groups, availableGroups, userRates, activeSubs, plansByGroup, authenticated)
		if len(groupsByPlatform) == 0 {
			continue
		}
		for _, m := range ch.SupportedModels {
			platformGroups := groupsByPlatform[m.Platform]
			if len(platformGroups) == 0 {
				continue
			}
			key := marketplaceModelKey(m.Platform, m.Name)
			entry := models[key]
			if entry == nil {
				entry = &marketplaceModel{
					Name:     m.Name,
					Platform: m.Platform,
					Pricing:  toUserPricing(m.Pricing),
					Channels: []marketplaceChannel{},
					Groups:   []marketplaceGroup{},
				}
				models[key] = entry
			}
			if entry.Pricing == nil && m.Pricing != nil {
				entry.Pricing = toUserPricing(m.Pricing)
			}
			entry.Channels = appendMarketplaceChannel(entry.Channels, marketplaceChannel{Name: ch.Name, Description: ch.Description})
			for _, g := range platformGroups {
				entry.Groups = appendMarketplaceGroup(entry.Groups, g)
			}
		}
	}

	out := make([]marketplaceModel, 0, len(models))
	for _, model := range models {
		sortMarketplaceModel(model)
		model.AccessState = highestMarketplaceAccess(model.Groups)
		out = append(out, *model)
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi := strings.ToLower(out[i].Platform)
		pj := strings.ToLower(out[j].Platform)
		if pi != pj {
			return pi < pj
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})

	return marketplaceResponse{
		Auth: marketplaceAuthStatus{
			Authenticated: authenticated,
			UserID:        userID,
		},
		Models: out,
	}, nil
}

func (h *ModelMarketplaceHandler) salePlansByGroup(ctx context.Context) (map[int64][]marketplacePlan, error) {
	out := make(map[int64][]marketplacePlan)
	if h.paymentConfig == nil {
		return out, nil
	}
	var (
		plans []*dbent.SubscriptionPlan
		err   error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = errModelMarketplaceDependencyMissing.WithCause(fmt.Errorf("list marketplace plans: %v", r))
			}
		}()
		plans, err = h.paymentConfig.ListPlansForSale(ctx)
	}()
	if err != nil {
		return nil, err
	}
	for _, p := range plans {
		if p == nil || p.GroupID <= 0 {
			continue
		}
		out[p.GroupID] = append(out[p.GroupID], marketplacePlan{
			ID:            int64(p.ID),
			GroupID:       p.GroupID,
			Name:          p.Name,
			Description:   p.Description,
			Price:         p.Price,
			OriginalPrice: p.OriginalPrice,
			ValidityDays:  p.ValidityDays,
			ValidityUnit:  p.ValidityUnit,
			Features:      parseFeatures(p.Features),
			ProductName:   p.ProductName,
			SortOrder:     p.SortOrder,
		})
	}
	for groupID := range out {
		plans := out[groupID]
		sort.SliceStable(plans, func(i, j int) bool {
			if plans[i].SortOrder != plans[j].SortOrder {
				return plans[i].SortOrder < plans[j].SortOrder
			}
			return plans[i].Price < plans[j].Price
		})
		out[groupID] = plans
	}
	return out, nil
}

func (h *ModelMarketplaceHandler) marketplaceGroupsByPlatform(
	groups []service.AvailableGroupRef,
	availableGroups map[int64]service.Group,
	userRates map[int64]float64,
	activeSubs map[int64]service.UserSubscription,
	plansByGroup map[int64][]marketplacePlan,
	authenticated bool,
) map[string][]marketplaceGroup {
	out := make(map[string][]marketplaceGroup)
	for _, g := range groups {
		if g.ID <= 0 || g.Platform == "" {
			continue
		}
		plans := plansByGroup[g.ID]
		_, available := availableGroups[g.ID]
		sub, subscribed := activeSubs[g.ID]

		state := ""
		switch {
		case subscribed:
			state = marketplaceAccessSubscribed
		case available:
			state = marketplaceAccessAvailable
		case g.SubscriptionType == service.SubscriptionTypeSubscription && len(plans) > 0:
			state = marketplaceAccessPurchasable
		case !authenticated && !g.IsExclusive && (g.SubscriptionType == "" || g.SubscriptionType == service.SubscriptionTypeStandard):
			state = marketplaceAccessAvailable
		}
		if state == "" {
			continue
		}

		mg := marketplaceGroup{
			ID:               g.ID,
			Name:             g.Name,
			Platform:         g.Platform,
			SubscriptionType: g.SubscriptionType,
			RateMultiplier:   g.RateMultiplier,
			IsExclusive:      g.IsExclusive,
			AccessState:      state,
			Plans:            cloneMarketplacePlans(plans),
		}
		if authenticated {
			if rate, ok := userRates[g.ID]; ok {
				v := rate
				mg.UserRateMultiplier = &v
			}
			if subscribed {
				mg.ActiveSubscription = toMarketplaceSubscription(sub)
			}
		}
		out[g.Platform] = append(out[g.Platform], mg)
	}
	return out
}

func marketplaceModelKey(platform, name string) string {
	return strings.ToLower(strings.TrimSpace(platform)) + "\x00" + strings.ToLower(strings.TrimSpace(name))
}

func appendMarketplaceChannel(items []marketplaceChannel, item marketplaceChannel) []marketplaceChannel {
	for _, existing := range items {
		if strings.EqualFold(existing.Name, item.Name) {
			return items
		}
	}
	return append(items, item)
}

func appendMarketplaceGroup(items []marketplaceGroup, item marketplaceGroup) []marketplaceGroup {
	for _, existing := range items {
		if existing.ID == item.ID {
			return items
		}
	}
	return append(items, item)
}

func sortMarketplaceModel(model *marketplaceModel) {
	sort.SliceStable(model.Channels, func(i, j int) bool {
		return strings.ToLower(model.Channels[i].Name) < strings.ToLower(model.Channels[j].Name)
	})
	sort.SliceStable(model.Groups, func(i, j int) bool {
		if model.Groups[i].AccessState != model.Groups[j].AccessState {
			return marketplaceAccessRank(model.Groups[i].AccessState) < marketplaceAccessRank(model.Groups[j].AccessState)
		}
		return strings.ToLower(model.Groups[i].Name) < strings.ToLower(model.Groups[j].Name)
	})
}

func highestMarketplaceAccess(groups []marketplaceGroup) string {
	state := ""
	best := 99
	for _, g := range groups {
		if rank := marketplaceAccessRank(g.AccessState); rank < best {
			best = rank
			state = g.AccessState
		}
	}
	return state
}

func marketplaceAccessRank(state string) int {
	switch state {
	case marketplaceAccessSubscribed:
		return 0
	case marketplaceAccessAvailable:
		return 1
	case marketplaceAccessPurchasable:
		return 2
	default:
		return 9
	}
}

func toMarketplaceSubscription(sub service.UserSubscription) *marketplaceSubscription {
	return &marketplaceSubscription{
		ID:              sub.ID,
		Status:          sub.Status,
		StartsAt:        sub.StartsAt,
		ExpiresAt:       sub.ExpiresAt,
		DailyUsageUSD:   sub.DailyUsageUSD,
		WeeklyUsageUSD:  sub.WeeklyUsageUSD,
		MonthlyUsageUSD: sub.MonthlyUsageUSD,
	}
}

func cloneMarketplacePlans(plans []marketplacePlan) []marketplacePlan {
	if len(plans) == 0 {
		return []marketplacePlan{}
	}
	out := make([]marketplacePlan, len(plans))
	copy(out, plans)
	return out
}

package upstreamstation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	coreservice "github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	ManagedAccountOwnerKey    = "managed_by"
	ManagedAccountOwner       = "upstream_station_pool"
	ManagedStationIDKey       = "upstream_station_id"
	ManagedRouteIDKey         = "upstream_route_id"
	ManagedEffectiveRateKey   = "upstream_effective_rate"
	ManagedRemoteGroupKey     = "upstream_remote_group"
	managedAccountConcurrency = 5
	managedAccountPriority    = 100
)

type ManagedAccountAdmin interface {
	GetAllGroups(ctx context.Context) ([]coreservice.Group, error)
	CreateGroup(ctx context.Context, input *coreservice.CreateGroupInput) (*coreservice.Group, error)
	GetAccount(ctx context.Context, id int64) (*coreservice.Account, error)
	CreateAccount(ctx context.Context, input *coreservice.CreateAccountInput) (*coreservice.Account, error)
	UpdateAccount(ctx context.Context, id int64, input *coreservice.UpdateAccountInput) (*coreservice.Account, error)
	SetAccountSchedulable(ctx context.Context, id int64, schedulable bool) (*coreservice.Account, error)
}

type ManagedAccountLookup interface {
	FindByExtraField(ctx context.Context, key string, value any) ([]coreservice.Account, error)
}

type ManagedAccountMaterializer struct {
	admin    ManagedAccountAdmin
	accounts ManagedAccountLookup
}

func NewManagedAccountMaterializer(admin ManagedAccountAdmin, accounts ManagedAccountLookup) *ManagedAccountMaterializer {
	return &ManagedAccountMaterializer{admin: admin, accounts: accounts}
}

func (m *ManagedAccountMaterializer) Materialize(ctx context.Context, station *Station, route *Route, apiKey string) (int64, error) {
	if m == nil || m.admin == nil || m.accounts == nil {
		return 0, errors.New("managed account dependencies are not configured")
	}
	if station == nil || route == nil || station.ID <= 0 || route.ID <= 0 {
		return 0, errors.New("station and route are required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return 0, errors.New("managed account API key is required")
	}

	group, err := m.ensureGroup(ctx, route.Platform)
	if err != nil {
		return 0, err
	}
	existing, err := m.findExistingAccount(ctx, route)
	if err != nil {
		return 0, err
	}

	credentials := map[string]any{
		"api_key":       apiKey,
		"base_url":      normalizeBaseURL(station.BaseURL),
		"model_mapping": buildModelMapping(route.Models),
	}
	extra := managedAccountExtra(nil, station, route)
	name := managedAccountName(station, route)

	if existing == nil {
		created, createErr := m.admin.CreateAccount(ctx, &coreservice.CreateAccountInput{
			Name:                  name,
			Platform:              normalizeRoutePlatform(route.Platform),
			Type:                  coreservice.AccountTypeAPIKey,
			Credentials:           credentials,
			Extra:                 extra,
			Concurrency:           managedAccountConcurrency,
			Priority:              managedAccountPriority,
			GroupIDs:              []int64{group.ID},
			SkipDefaultGroupBind:  true,
			SkipMixedChannelCheck: true,
		})
		if createErr != nil {
			return 0, createErr
		}
		return created.ID, nil
	}
	if !isOwnedManagedAccount(existing, route.ID) {
		return 0, fmt.Errorf("account %d is not owned by upstream station pool", existing.ID)
	}
	extra = managedAccountExtra(existing.Extra, station, route)
	concurrency := existing.Concurrency
	if concurrency <= 0 {
		concurrency = managedAccountConcurrency
	}
	priority := existing.Priority
	groupIDs := []int64{group.ID}
	_, err = m.admin.UpdateAccount(ctx, existing.ID, &coreservice.UpdateAccountInput{
		Name:                  name,
		Type:                  coreservice.AccountTypeAPIKey,
		Credentials:           credentials,
		Extra:                 extra,
		Concurrency:           &concurrency,
		Priority:              &priority,
		Status:                coreservice.StatusActive,
		GroupIDs:              &groupIDs,
		SkipMixedChannelCheck: true,
	})
	if err != nil {
		return 0, err
	}
	if _, err := m.admin.SetAccountSchedulable(ctx, existing.ID, route.Schedulable && route.HealthStatus != HealthStatusError); err != nil {
		return 0, err
	}
	return existing.ID, nil
}

func (m *ManagedAccountMaterializer) ensureGroup(ctx context.Context, platform string) (*coreservice.Group, error) {
	platform = normalizeRoutePlatform(platform)
	name := managedGroupName(platform)
	groups, err := m.admin.GetAllGroups(ctx)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].Name != name {
			continue
		}
		if groups[i].Platform != platform {
			return nil, fmt.Errorf("managed group %q exists with platform %q", name, groups[i].Platform)
		}
		return &groups[i], nil
	}
	return m.admin.CreateGroup(ctx, &coreservice.CreateGroupInput{
		Name:             name,
		Description:      "上游中转站资源池自动管理分组",
		Platform:         platform,
		RateMultiplier:   1,
		SubscriptionType: coreservice.SubscriptionTypeStandard,
	})
}

func (m *ManagedAccountMaterializer) findExistingAccount(ctx context.Context, route *Route) (*coreservice.Account, error) {
	if route.ManagedAccountID != nil && *route.ManagedAccountID > 0 {
		account, err := m.admin.GetAccount(ctx, *route.ManagedAccountID)
		if err == nil && account != nil {
			return account, nil
		}
	}
	accounts, err := m.accounts.FindByExtraField(ctx, ManagedRouteIDKey, route.ID)
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		if isOwnedManagedAccount(&accounts[i], route.ID) {
			return &accounts[i], nil
		}
	}
	return nil, nil
}

func managedGroupName(platform string) string {
	switch normalizeRoutePlatform(platform) {
	case coreservice.PlatformAnthropic:
		return "自动上游-Claude"
	case coreservice.PlatformGemini:
		return "自动上游-Gemini"
	case coreservice.PlatformGrok:
		return "自动上游-Grok"
	default:
		return "自动上游-OpenAI"
	}
}

func managedAccountName(station *Station, route *Route) string {
	stationName := firstNonEmpty(station.Name, fmt.Sprintf("station-%d", station.ID))
	routeName := firstNonEmpty(route.RemoteGroupName, route.RemoteGroupKey, fmt.Sprintf("route-%d", route.ID))
	return fmt.Sprintf("自动上游/%s/%s", stationName, routeName)
}

func buildModelMapping(models []string) map[string]any {
	mapping := make(map[string]any, len(models))
	for _, model := range normalizeModels(models) {
		mapping[model] = model
	}
	return mapping
}

func managedAccountExtra(existing map[string]any, station *Station, route *Route) map[string]any {
	extra := make(map[string]any, len(existing)+5)
	for key, value := range existing {
		extra[key] = value
	}
	extra[ManagedAccountOwnerKey] = ManagedAccountOwner
	extra[ManagedStationIDKey] = station.ID
	extra[ManagedRouteIDKey] = route.ID
	extra[ManagedEffectiveRateKey] = route.EffectiveRate
	extra[ManagedRemoteGroupKey] = route.RemoteGroupKey
	return extra
}

func isOwnedManagedAccount(account *coreservice.Account, routeID int64) bool {
	if account == nil || account.Extra == nil {
		return false
	}
	owner, _ := account.Extra[ManagedAccountOwnerKey].(string)
	return owner == ManagedAccountOwner && int64(coreservice.ParseExtraInt(account.Extra[ManagedRouteIDKey])) == routeID
}

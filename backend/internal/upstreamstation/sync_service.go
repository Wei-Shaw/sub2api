package upstreamstation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type StationStore interface {
	GetStation(ctx context.Context, id int64) (*Station, error)
	ListRoutes(ctx context.Context, stationID int64) ([]Route, error)
	UpsertRoute(ctx context.Context, route *Route) (*Route, error)
	AppendRateSnapshot(ctx context.Context, snapshot RateSnapshot) error
	AppendSyncLog(ctx context.Context, item SyncLog) error
	UpdateStationObservation(ctx context.Context, id int64, balance *float64, rechargeMultiplier float64, health, lastError string, tested, synced bool) error
	UpdateRouteManagedAccount(ctx context.Context, id, accountID int64) error
}

type SecretEncryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

type RouteMaterializer interface {
	Materialize(ctx context.Context, station *Station, route *Route, apiKey string) (int64, error)
}

type SyncResult struct {
	StationID    int64    `json:"station_id"`
	SyncedRoutes int      `json:"synced_routes"`
	CreatedKeys  int      `json:"created_keys"`
	Errors       []string `json:"errors"`
}

type SyncService struct {
	store        StationStore
	credentials  *CredentialCodec
	registry     *ConnectorRegistry
	encryptor    SecretEncryptor
	materializer RouteMaterializer
}

func NewSyncService(store StationStore, credentials *CredentialCodec, registry *ConnectorRegistry, encryptor SecretEncryptor, materializer RouteMaterializer) *SyncService {
	return &SyncService{
		store:        store,
		credentials:  credentials,
		registry:     registry,
		encryptor:    encryptor,
		materializer: materializer,
	}
}

func (s *SyncService) SyncStation(ctx context.Context, stationID int64) (*SyncResult, error) {
	result := &SyncResult{StationID: stationID, Errors: []string{}}
	station, err := s.store.GetStation(ctx, stationID)
	if err != nil {
		return result, err
	}
	if !station.Enabled {
		return result, errors.New("upstream station is disabled")
	}
	credentials, err := s.credentials.Decrypt(station.CredentialCipher)
	if err != nil {
		s.recordFailure(ctx, station, err)
		return result, err
	}
	connector, err := s.registry.Resolve(ctx, station)
	if err != nil && station.CredentialMode == CredentialModeAPIKey {
		connector = s.registry.FirstModelDiscoverer()
		if connector != nil {
			err = nil
		}
	}
	if err != nil {
		s.recordFailure(ctx, station, err)
		return result, err
	}

	if station.CredentialMode == CredentialModeAPIKey {
		err = s.syncFixedRoutes(ctx, station, credentials, connector, result)
	} else {
		err = s.syncManagedRoutes(ctx, station, credentials, connector, result)
	}
	if err != nil {
		s.recordFailure(ctx, station, err)
		return result, err
	}

	_ = s.store.AppendSyncLog(ctx, SyncLog{
		StationID: station.ID,
		Action:    "sync",
		Success:   true,
		Message:   fmt.Sprintf("synced %d routes", result.SyncedRoutes),
	})
	return result, nil
}

func (s *SyncService) syncManagedRoutes(ctx context.Context, station *Station, credentials Credentials, connector Connector, result *SyncResult) error {
	session, err := connector.Authenticate(ctx, station, credentials)
	if err != nil {
		return fmt.Errorf("authenticate upstream station: %w", err)
	}

	var balance *float64
	if value, balanceErr := connector.GetBalance(ctx, station.BaseURL, session); balanceErr == nil {
		balance = &value
	} else {
		result.Errors = append(result.Errors, "balance: "+balanceErr.Error())
	}

	rechargeMultiplier := station.RechargeMultiplier
	if rechargeMultiplier <= 0 {
		rechargeMultiplier = 1
	}
	if station.RechargeSource == RechargeSourceAuto {
		if value, multiplierErr := connector.GetRechargeMultiplier(ctx, station.BaseURL, session); multiplierErr == nil && value > 0 {
			rechargeMultiplier = value
		} else if multiplierErr != nil {
			result.Errors = append(result.Errors, "recharge multiplier: "+multiplierErr.Error())
		}
	}

	groups, err := connector.ListGroups(ctx, station.BaseURL, session)
	if err != nil {
		return fmt.Errorf("list upstream groups: %w", err)
	}
	if len(groups) == 0 {
		return errors.New("upstream returned no groups; previous routes were kept")
	}

	keyManager, ok := connector.(APIKeyManager)
	if !ok {
		return errors.New("upstream connector does not support managed API keys")
	}
	modelDiscoverer, ok := connector.(ModelDiscoverer)
	if !ok {
		return errors.New("upstream connector does not support model discovery")
	}
	keys, err := keyManager.ListAPIKeys(ctx, station.BaseURL, session)
	if err != nil {
		return fmt.Errorf("list upstream API keys: %w", err)
	}
	existingRoutes, err := s.store.ListRoutes(ctx, station.ID)
	if err != nil {
		return err
	}
	routeByKey := indexRoutes(existingRoutes)
	seenRouteKeys := make(map[string]struct{}, len(groups))

	for _, group := range groups {
		platform := normalizeRoutePlatform(group.Platform)
		group.Platform = platform
		key := routeIndexKey(group.Key, platform)
		seenRouteKeys[key] = struct{}{}
		previous, existed := routeByKey[key]
		managedName := ManagedAPIKeyName(station.ID, group.Key)
		managedKey := findManagedKey(keys, managedName, group.Key)
		if managedKey == nil {
			created, createErr := keyManager.CreateAPIKey(ctx, station.BaseURL, session, managedName, group)
			if createErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("group %s: create key: %v", group.Name, createErr))
				continue
			}
			managedKey = created
			keys = append(keys, *created)
			result.CreatedKeys++
		}
		apiKey := strings.TrimSpace(managedKey.Key)
		if apiKey == "" || strings.Contains(apiKey, "*") {
			apiKey, err = keyManager.RevealAPIKey(ctx, station.BaseURL, session, managedKey.ID)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("group %s: reveal key: %v", group.Name, err))
				continue
			}
		}
		models, modelErr := modelDiscoverer.ListModels(ctx, station.BaseURL, apiKey, platform)
		models = normalizeModels(models)
		if modelErr != nil || len(models) == 0 {
			if modelErr == nil {
				modelErr = errors.New("upstream returned no models")
			}
			result.Errors = append(result.Errors, fmt.Sprintf("group %s: models: %v", group.Name, modelErr))
			if existed {
				if markErr := s.markRouteUnhealthy(ctx, station, previous, apiKey, modelErr); markErr != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("group %s: mark unhealthy: %v", group.Name, markErr))
				}
			}
			continue
		}
		cipher, cipherErr := s.encryptor.Encrypt(apiKey)
		if cipherErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("group %s: encrypt API key: %v", group.Name, cipherErr))
			continue
		}
		route := &Route{
			StationID:          station.ID,
			RemoteGroupKey:     group.Key,
			RemoteGroupName:    firstNonEmpty(group.Name, group.Key),
			Platform:           platform,
			Models:             models,
			GroupRate:          group.Rate,
			RechargeMultiplier: rechargeMultiplier,
			FixedRoute:         false,
			RemoteAPIKeyID:     managedKey.ID,
			APIKeyCipher:       cipher,
			Schedulable:        true,
			HealthStatus:       HealthStatusHealthy,
		}
		if existed {
			route.ManagedAccountID = previous.ManagedAccountID
			route.Schedulable = previous.Schedulable
		}
		if err := s.persistRoute(ctx, station, route, apiKey, previous, existed); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("group %s: %v", group.Name, err))
			continue
		}
		result.SyncedRoutes++
	}
	for _, route := range existingRoutes {
		if route.FixedRoute {
			continue
		}
		if _, exists := seenRouteKeys[routeIndexKey(route.RemoteGroupKey, route.Platform)]; exists {
			continue
		}
		apiKey, decryptErr := s.encryptor.Decrypt(route.APIKeyCipher)
		if decryptErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("group %s: decrypt previous API key: %v", route.RemoteGroupName, decryptErr))
			continue
		}
		missingErr := errors.New("upstream group is no longer available")
		if markErr := s.markRouteUnhealthy(ctx, station, route, apiKey, missingErr); markErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("group %s: mark unavailable: %v", route.RemoteGroupName, markErr))
		}
	}

	if result.SyncedRoutes == 0 {
		return errors.New("no upstream routes were synchronized; previous routes were kept")
	}
	if err := s.store.UpdateStationObservation(ctx, station.ID, balance, rechargeMultiplier, HealthStatusHealthy, "", true, true); err != nil {
		return err
	}
	return nil
}

func (s *SyncService) syncFixedRoutes(ctx context.Context, station *Station, credentials Credentials, connector Connector, result *SyncResult) error {
	apiKey := strings.TrimSpace(credentials.APIKey)
	if apiKey == "" {
		return errors.New("fixed route API key is required")
	}
	discoverer, ok := connector.(ModelDiscoverer)
	if !ok {
		return errors.New("upstream connector does not support model discovery")
	}
	routes, err := s.store.ListRoutes(ctx, station.ID)
	if err != nil {
		return err
	}
	if len(routes) == 0 {
		return errors.New("fixed route requires at least one manually configured route")
	}
	rechargeMultiplier := station.RechargeMultiplier
	if rechargeMultiplier <= 0 {
		rechargeMultiplier = 1
	}
	for i := range routes {
		route := routes[i]
		if !route.FixedRoute {
			continue
		}
		models, modelErr := discoverer.ListModels(ctx, station.BaseURL, apiKey, route.Platform)
		models = normalizeModels(models)
		if modelErr != nil || len(models) == 0 {
			if modelErr == nil {
				modelErr = errors.New("upstream returned no models")
			}
			result.Errors = append(result.Errors, fmt.Sprintf("route %s: model discovery failed: %v", route.RemoteGroupName, modelErr))
			if markErr := s.markRouteUnhealthy(ctx, station, route, apiKey, modelErr); markErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("route %s: mark unhealthy: %v", route.RemoteGroupName, markErr))
			}
			continue
		}
		cipher, cipherErr := s.encryptor.Encrypt(apiKey)
		if cipherErr != nil {
			return cipherErr
		}
		previous := route
		route.Models = models
		route.RechargeMultiplier = rechargeMultiplier
		route.APIKeyCipher = cipher
		route.HealthStatus = HealthStatusHealthy
		route.LastError = ""
		if err := s.persistRoute(ctx, station, &route, apiKey, previous, true); err != nil {
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		result.SyncedRoutes++
	}
	if result.SyncedRoutes == 0 {
		return errors.New("no fixed routes were synchronized; previous routes were kept")
	}
	return s.store.UpdateStationObservation(ctx, station.ID, nil, rechargeMultiplier, HealthStatusHealthy, "", true, true)
}

func (s *SyncService) markRouteUnhealthy(ctx context.Context, station *Station, route Route, apiKey string, cause error) error {
	previous := route
	route.HealthStatus = HealthStatusError
	route.LastError = cause.Error()
	return s.persistRoute(ctx, station, &route, apiKey, previous, true)
}

func (s *SyncService) persistRoute(ctx context.Context, station *Station, route *Route, apiKey string, previous Route, existed bool) error {
	route.EffectiveRate = EffectiveRate(route.GroupRate, route.RechargeMultiplier)
	persisted, err := s.store.UpsertRoute(ctx, route)
	if err != nil {
		return err
	}
	if !existed || previous.GroupRate != persisted.GroupRate || previous.RechargeMultiplier != persisted.RechargeMultiplier || previous.EffectiveRate != persisted.EffectiveRate {
		if err := s.store.AppendRateSnapshot(ctx, RateSnapshot{
			RouteID:            persisted.ID,
			GroupRate:          persisted.GroupRate,
			RechargeMultiplier: persisted.RechargeMultiplier,
			EffectiveRate:      persisted.EffectiveRate,
		}); err != nil {
			return err
		}
	}
	if s.materializer == nil {
		return nil
	}
	accountID, err := s.materializer.Materialize(ctx, station, persisted, apiKey)
	if err != nil {
		return fmt.Errorf("materialize managed account: %w", err)
	}
	if accountID > 0 {
		if err := s.store.UpdateRouteManagedAccount(ctx, persisted.ID, accountID); err != nil {
			return err
		}
		persisted.ManagedAccountID = &accountID
	}
	return nil
}

func (s *SyncService) recordFailure(ctx context.Context, station *Station, syncErr error) {
	if station == nil {
		return
	}
	message := syncErr.Error()
	_ = s.store.UpdateStationObservation(ctx, station.ID, nil, 0, HealthStatusError, message, true, false)
	_ = s.store.AppendSyncLog(ctx, SyncLog{StationID: station.ID, Action: "sync", Success: false, Message: message})
}

func findManagedKey(keys []ManagedKey, name, groupKey string) *ManagedKey {
	for i := range keys {
		if keys[i].Name == name && (keys[i].GroupKey == "" || keys[i].GroupKey == groupKey) {
			copy := keys[i]
			return &copy
		}
	}
	return nil
}

func indexRoutes(routes []Route) map[string]Route {
	indexed := make(map[string]Route, len(routes))
	for _, route := range routes {
		indexed[routeIndexKey(route.RemoteGroupKey, route.Platform)] = route
	}
	return indexed
}

func routeIndexKey(groupKey, platform string) string {
	return groupKey + ":" + normalizeRoutePlatform(platform)
}

func normalizeRoutePlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "anthropic", "claude":
		return "anthropic"
	case "gemini", "google":
		return "gemini"
	case "grok", "xai":
		return "grok"
	default:
		return "openai"
	}
}

func normalizeModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(strings.TrimPrefix(model, "models/"))
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	sort.Strings(result)
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return "route-" + time.Now().UTC().Format("20060102150405")
}

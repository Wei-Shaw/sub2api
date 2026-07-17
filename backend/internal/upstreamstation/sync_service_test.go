package upstreamstation

import (
	"context"
	"errors"
	"testing"

	coreservice "github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type syncTestStore struct {
	station     *Station
	routes      map[string]Route
	snapshots   []RateSnapshot
	logs        []SyncLog
	observation struct {
		balance  *float64
		recharge float64
		health   string
	}
}

func newSyncTestStore(station *Station) *syncTestStore {
	return &syncTestStore{station: station, routes: make(map[string]Route)}
}

func (s *syncTestStore) GetStation(context.Context, int64) (*Station, error) {
	copy := *s.station
	return &copy, nil
}
func (s *syncTestStore) ListRoutes(context.Context, int64) ([]Route, error) {
	out := make([]Route, 0, len(s.routes))
	for _, route := range s.routes {
		out = append(out, route)
	}
	return out, nil
}
func (s *syncTestStore) UpsertRoute(_ context.Context, route *Route) (*Route, error) {
	key := route.RemoteGroupKey + ":" + route.Platform
	if existing, ok := s.routes[key]; ok {
		route.ID = existing.ID
	} else {
		route.ID = int64(len(s.routes) + 1)
	}
	copy := *route
	copy.Models = append([]string(nil), route.Models...)
	s.routes[key] = copy
	return route, nil
}
func (s *syncTestStore) AppendRateSnapshot(_ context.Context, snapshot RateSnapshot) error {
	s.snapshots = append(s.snapshots, snapshot)
	return nil
}
func (s *syncTestStore) AppendSyncLog(_ context.Context, item SyncLog) error {
	s.logs = append(s.logs, item)
	return nil
}
func (s *syncTestStore) UpdateStationObservation(_ context.Context, _ int64, balance *float64, recharge float64, health, _ string, _, _ bool) error {
	s.observation.balance = balance
	s.observation.recharge = recharge
	s.observation.health = health
	return nil
}
func (s *syncTestStore) UpdateRouteManagedAccount(_ context.Context, id, accountID int64) error {
	for key, route := range s.routes {
		if route.ID == id {
			route.ManagedAccountID = &accountID
			s.routes[key] = route
		}
	}
	return nil
}

type syncTestConnector struct {
	siteType        string
	groups          []RemoteGroup
	keys            []ManagedKey
	models          []string
	modelErr        error
	revealedKey     string
	revealCount     int
	modelAPIKeys    []string
	createdKeyCount int
}

func (c *syncTestConnector) Type() string {
	if c.siteType != "" {
		return c.siteType
	}
	return SiteTypeSub2API
}
func (*syncTestConnector) Detect(context.Context, string) (bool, error) { return true, nil }
func (*syncTestConnector) Authenticate(context.Context, *Station, Credentials) (*Session, error) {
	return &Session{AccessToken: "session"}, nil
}
func (*syncTestConnector) GetBalance(context.Context, string, *Session) (float64, error) {
	return 12.5, nil
}
func (c *syncTestConnector) ListGroups(context.Context, string, *Session) ([]RemoteGroup, error) {
	return append([]RemoteGroup(nil), c.groups...), nil
}
func (*syncTestConnector) GetRechargeMultiplier(context.Context, string, *Session) (float64, error) {
	return 2, nil
}
func (c *syncTestConnector) ListAPIKeys(context.Context, string, *Session) ([]ManagedKey, error) {
	return append([]ManagedKey(nil), c.keys...), nil
}
func (c *syncTestConnector) CreateAPIKey(_ context.Context, _ string, _ *Session, name string, group RemoteGroup) (*ManagedKey, error) {
	c.createdKeyCount++
	created := ManagedKey{ID: "9", Name: name, GroupKey: group.Key, Key: "sk-managed", Status: "active"}
	c.keys = append(c.keys, created)
	return &created, nil
}
func (c *syncTestConnector) RevealAPIKey(context.Context, string, *Session, string) (string, error) {
	c.revealCount++
	if c.revealedKey != "" {
		return c.revealedKey, nil
	}
	return "sk-managed", nil
}
func (c *syncTestConnector) ListModels(_ context.Context, _, apiKey, _ string) ([]string, error) {
	c.modelAPIKeys = append(c.modelAPIKeys, apiKey)
	return append([]string(nil), c.models...), c.modelErr
}

type syncTestMaterializer struct {
	calls      int
	lastRoutes []Route
}

type fixedSyncTestConnector struct {
	*syncTestConnector
}

func (*fixedSyncTestConnector) Detect(context.Context, string) (bool, error) { return false, nil }

func (m *syncTestMaterializer) Materialize(_ context.Context, _ *Station, route *Route, apiKey string) (int64, error) {
	m.calls++
	m.lastRoutes = append(m.lastRoutes, *route)
	if apiKey == "" {
		return 0, errors.New("missing api key")
	}
	return 42, nil
}

func TestSyncStationReusesManagedKeyAndRoute(t *testing.T) {
	t.Parallel()

	codec := NewCredentialCodec(testEncryptor{})
	cipher, err := codec.Encrypt(Credentials{Username: "boss", Password: "secret"})
	require.NoError(t, err)
	store := newSyncTestStore(&Station{
		ID: 7, Name: "station", SiteType: SiteTypeSub2API, BaseURL: "https://station.example",
		CredentialMode: CredentialModePassword, CredentialCipher: cipher,
		RechargeMultiplier: 1, RechargeSource: RechargeSourceAuto, Enabled: true,
	})
	connector := &syncTestConnector{
		groups: []RemoteGroup{{Key: "3", Name: "pro", Rate: 0.8, Platform: coreservice.PlatformOpenAI}},
		models: []string{"gpt-5", "o3"},
	}
	materializer := &syncTestMaterializer{}
	svc := NewSyncService(store, codec, NewConnectorRegistry(connector), testEncryptor{}, materializer)

	first, err := svc.SyncStation(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, 1, first.CreatedKeys)
	require.Equal(t, 1, first.SyncedRoutes)

	second, err := svc.SyncStation(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, 0, second.CreatedKeys)
	require.Equal(t, 1, second.SyncedRoutes)

	require.Equal(t, 1, connector.createdKeyCount)
	require.Len(t, store.routes, 1)
	route := store.routes["3:openai"]
	require.Equal(t, 0.4, route.EffectiveRate)
	require.Equal(t, []string{"gpt-5", "o3"}, route.Models)
	require.Equal(t, int64(42), *route.ManagedAccountID)
	require.Len(t, store.snapshots, 1)
	require.Equal(t, 2, materializer.calls)
	require.Equal(t, HealthStatusHealthy, store.observation.health)
}

func TestSyncStationRevealsMaskedManagedKeyBeforeModelDiscovery(t *testing.T) {
	t.Parallel()

	codec := NewCredentialCodec(testEncryptor{})
	cipher, err := codec.Encrypt(Credentials{Username: "boss", Password: "secret"})
	require.NoError(t, err)
	store := newSyncTestStore(&Station{
		ID: 13, Name: "station", SiteType: SiteTypeNewAPI, BaseURL: "https://station.example",
		CredentialMode: CredentialModePassword, CredentialCipher: cipher,
		RechargeMultiplier: 1, Enabled: true,
	})
	connector := &syncTestConnector{
		siteType:    SiteTypeNewAPI,
		groups:      []RemoteGroup{{Key: "vip", Name: "vip", Rate: 0.2, Platform: coreservice.PlatformOpenAI}},
		keys:        []ManagedKey{{ID: "19", Name: ManagedAPIKeyName(13, "vip"), GroupKey: "vip", Key: "sk-masked********key"}},
		models:      []string{"gpt-5"},
		revealedKey: "sk-full-key",
	}
	svc := NewSyncService(store, codec, NewConnectorRegistry(connector), testEncryptor{}, &syncTestMaterializer{})

	result, err := svc.SyncStation(context.Background(), 13)
	require.NoError(t, err)
	require.Equal(t, 1, result.SyncedRoutes)
	require.Equal(t, 1, connector.revealCount)
	require.Equal(t, []string{"sk-full-key"}, connector.modelAPIKeys)
}

func TestSyncStationKeepsPreviousRoutesWhenGroupListIsEmpty(t *testing.T) {
	t.Parallel()

	codec := NewCredentialCodec(testEncryptor{})
	cipher, err := codec.Encrypt(Credentials{AccessToken: "token"})
	require.NoError(t, err)
	store := newSyncTestStore(&Station{
		ID: 8, SiteType: SiteTypeSub2API, BaseURL: "https://station.example",
		CredentialMode: CredentialModeToken, CredentialCipher: cipher, RechargeMultiplier: 1, Enabled: true,
	})
	store.routes["old:openai"] = Route{ID: 10, StationID: 8, RemoteGroupKey: "old", Platform: "openai", Models: []string{"gpt-4"}}
	connector := &syncTestConnector{}
	svc := NewSyncService(store, codec, NewConnectorRegistry(connector), testEncryptor{}, &syncTestMaterializer{})

	_, err = svc.SyncStation(context.Background(), 8)
	require.ErrorContains(t, err, "no groups")
	require.Len(t, store.routes, 1)
	require.Equal(t, []string{"gpt-4"}, store.routes["old:openai"].Models)
	require.Equal(t, HealthStatusError, store.observation.health)
}

func TestSyncFixedRouteUsesModelDiscoveryWithoutControlPlaneDetection(t *testing.T) {
	t.Parallel()

	codec := NewCredentialCodec(testEncryptor{})
	cipher, err := codec.Encrypt(Credentials{APIKey: "sk-fixed"})
	require.NoError(t, err)
	store := newSyncTestStore(&Station{
		ID: 9, SiteType: SiteTypeAuto, BaseURL: "https://compatible.example",
		CredentialMode: CredentialModeAPIKey, CredentialCipher: cipher,
		RechargeMultiplier: 2, Enabled: true,
	})
	store.routes["fixed:openai"] = Route{
		ID: 12, StationID: 9, RemoteGroupKey: "fixed", RemoteGroupName: "Fixed",
		Platform: "openai", GroupRate: 0.8, RechargeMultiplier: 2,
		EffectiveRate: 0.4, FixedRoute: true, Schedulable: true,
	}
	connector := &fixedSyncTestConnector{syncTestConnector: &syncTestConnector{models: []string{"gpt-5"}}}
	svc := NewSyncService(store, codec, NewConnectorRegistry(connector), testEncryptor{}, &syncTestMaterializer{})

	result, err := svc.SyncStation(context.Background(), 9)
	require.NoError(t, err)
	require.Equal(t, 1, result.SyncedRoutes)
	require.Equal(t, []string{"gpt-5"}, store.routes["fixed:openai"].Models)
}

func TestSyncStationPreservesManuallyDisabledRoute(t *testing.T) {
	t.Parallel()

	codec := NewCredentialCodec(testEncryptor{})
	cipher, err := codec.Encrypt(Credentials{AccessToken: "token"})
	require.NoError(t, err)
	store := newSyncTestStore(&Station{
		ID: 10, Name: "station", SiteType: SiteTypeSub2API, BaseURL: "https://station.example",
		CredentialMode: CredentialModeToken, CredentialCipher: cipher,
		RechargeMultiplier: 1, Enabled: true,
	})
	store.routes["3:openai"] = Route{
		ID: 13, StationID: 10, RemoteGroupKey: "3", RemoteGroupName: "pro", Platform: "openai",
		Models: []string{"gpt-4"}, GroupRate: 0.8, RechargeMultiplier: 1,
		EffectiveRate: 0.8, Schedulable: false, HealthStatus: HealthStatusHealthy,
	}
	connector := &syncTestConnector{
		groups: []RemoteGroup{{Key: "3", Name: "pro", Rate: 0.8, Platform: "openai"}},
		keys:   []ManagedKey{{ID: "9", Name: ManagedAPIKeyName(10, "3"), GroupKey: "3", Key: "sk-managed"}},
		models: []string{"gpt-5"},
	}
	materializer := &syncTestMaterializer{}
	svc := NewSyncService(store, codec, NewConnectorRegistry(connector), testEncryptor{}, materializer)

	_, err = svc.SyncStation(context.Background(), 10)
	require.NoError(t, err)
	route := store.routes["3:openai"]
	require.False(t, route.Schedulable)
	require.Equal(t, HealthStatusHealthy, route.HealthStatus)
	require.False(t, materializer.lastRoutes[len(materializer.lastRoutes)-1].Schedulable)
}

func TestSyncStationMarksExistingRouteUnhealthyWhenModelDiscoveryFails(t *testing.T) {
	t.Parallel()

	codec := NewCredentialCodec(testEncryptor{})
	cipher, err := codec.Encrypt(Credentials{AccessToken: "token"})
	require.NoError(t, err)
	store := newSyncTestStore(&Station{
		ID: 11, Name: "station", SiteType: SiteTypeSub2API, BaseURL: "https://station.example",
		CredentialMode: CredentialModeToken, CredentialCipher: cipher,
		RechargeMultiplier: 1, Enabled: true,
	})
	store.routes["3:openai"] = Route{
		ID: 14, StationID: 11, RemoteGroupKey: "3", RemoteGroupName: "pro", Platform: "openai",
		Models: []string{"gpt-4"}, GroupRate: 0.8, RechargeMultiplier: 1,
		EffectiveRate: 0.8, Schedulable: true, HealthStatus: HealthStatusHealthy,
	}
	connector := &syncTestConnector{
		groups:   []RemoteGroup{{Key: "3", Name: "pro", Rate: 0.8, Platform: "openai"}},
		keys:     []ManagedKey{{ID: "9", Name: ManagedAPIKeyName(11, "3"), GroupKey: "3", Key: "sk-managed"}},
		modelErr: errors.New("model endpoint unavailable"),
	}
	materializer := &syncTestMaterializer{}
	svc := NewSyncService(store, codec, NewConnectorRegistry(connector), testEncryptor{}, materializer)

	_, err = svc.SyncStation(context.Background(), 11)
	require.ErrorContains(t, err, "no upstream routes")
	route := store.routes["3:openai"]
	require.Equal(t, []string{"gpt-4"}, route.Models)
	require.True(t, route.Schedulable)
	require.Equal(t, HealthStatusError, route.HealthStatus)
	require.ErrorContains(t, errors.New(route.LastError), "model endpoint unavailable")
	require.Equal(t, HealthStatusError, materializer.lastRoutes[len(materializer.lastRoutes)-1].HealthStatus)
}

func TestSyncStationMarksMissingManagedRouteUnhealthy(t *testing.T) {
	t.Parallel()

	codec := NewCredentialCodec(testEncryptor{})
	credentialCipher, err := codec.Encrypt(Credentials{AccessToken: "token"})
	require.NoError(t, err)
	oldAPIKeyCipher, err := testEncryptor{}.Encrypt("sk-old")
	require.NoError(t, err)
	store := newSyncTestStore(&Station{
		ID: 12, Name: "station", SiteType: SiteTypeSub2API, BaseURL: "https://station.example",
		CredentialMode: CredentialModeToken, CredentialCipher: credentialCipher,
		RechargeMultiplier: 1, Enabled: true,
	})
	store.routes["old:openai"] = Route{
		ID: 15, StationID: 12, RemoteGroupKey: "old", RemoteGroupName: "old", Platform: "openai",
		Models: []string{"gpt-4"}, GroupRate: 0.5, RechargeMultiplier: 1,
		EffectiveRate: 0.5, APIKeyCipher: oldAPIKeyCipher, Schedulable: true, HealthStatus: HealthStatusHealthy,
	}
	connector := &syncTestConnector{
		groups: []RemoteGroup{{Key: "new", Name: "new", Rate: 0.8, Platform: "openai"}},
		keys:   []ManagedKey{{ID: "9", Name: ManagedAPIKeyName(12, "new"), GroupKey: "new", Key: "sk-new"}},
		models: []string{"gpt-5"},
	}
	materializer := &syncTestMaterializer{}
	svc := NewSyncService(store, codec, NewConnectorRegistry(connector), testEncryptor{}, materializer)

	result, err := svc.SyncStation(context.Background(), 12)
	require.NoError(t, err)
	require.Equal(t, 1, result.SyncedRoutes)
	oldRoute := store.routes["old:openai"]
	require.Equal(t, []string{"gpt-4"}, oldRoute.Models)
	require.Equal(t, HealthStatusError, oldRoute.HealthStatus)
	require.ErrorContains(t, errors.New(oldRoute.LastError), "no longer available")
}

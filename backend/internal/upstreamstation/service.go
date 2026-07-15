package upstreamstation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	coreservice "github.com/Wei-Shaw/sub2api/internal/service"
)

type StationInput struct {
	Name               string
	SiteType           string
	BaseURL            string
	CredentialMode     string
	Credentials        *Credentials
	RechargeMultiplier float64
	RechargeSource     string
	Enabled            bool
	AutoSync           bool
	FixedRoutes        []FixedRouteInput
}

type StationUpdateInput struct {
	Name               *string
	SiteType           *string
	BaseURL            *string
	CredentialMode     *string
	Credentials        *Credentials
	RechargeMultiplier *float64
	RechargeSource     *string
	Enabled            *bool
	AutoSync           *bool
}

type FixedRouteInput struct {
	RemoteGroupKey  string   `json:"remote_group_key"`
	RemoteGroupName string   `json:"remote_group_name"`
	Platform        string   `json:"platform"`
	Models          []string `json:"models"`
	GroupRate       float64  `json:"group_rate"`
	Schedulable     *bool    `json:"schedulable"`
}

type RouteUpdateInput struct {
	RemoteGroupName    *string
	Models             *[]string
	GroupRate          *float64
	RechargeMultiplier *float64
	Schedulable        *bool
}

type ConnectionTestResult struct {
	SiteType   string   `json:"site_type"`
	Balance    *float64 `json:"balance,omitempty"`
	GroupCount int      `json:"group_count"`
	RouteCount int      `json:"route_count"`
}

type Service struct {
	repository   *Repository
	codec        *CredentialCodec
	registry     *ConnectorRegistry
	syncer       *SyncService
	admin        ManagedAccountAdmin
	encryptor    SecretEncryptor
	materializer *ManagedAccountMaterializer
	workerCancel context.CancelFunc
	workerWG     sync.WaitGroup
}

func NewService(db *sql.DB, encryptor coreservice.SecretEncryptor, admin coreservice.AdminService, accounts coreservice.AccountRepository) *Service {
	repository := NewRepository(db)
	codec := NewCredentialCodec(encryptor)
	registry := NewConnectorRegistry(NewNewAPIConnector(nil), NewSub2APIConnector(nil))
	materializer := NewManagedAccountMaterializer(admin, accounts)
	service := &Service{
		repository:   repository,
		codec:        codec,
		registry:     registry,
		syncer:       NewSyncService(repository, codec, registry, encryptor, materializer),
		admin:        admin,
		encryptor:    encryptor,
		materializer: materializer,
	}
	service.Start()
	return service
}

func (s *Service) ListStations(ctx context.Context) ([]Station, error) {
	return s.repository.ListStations(ctx)
}

func (s *Service) GetStation(ctx context.Context, id int64) (*Station, error) {
	return s.repository.GetStation(ctx, id)
}

func (s *Service) CreateStation(ctx context.Context, input StationInput) (*Station, error) {
	if err := validateStationInput(input.Name, input.SiteType, input.BaseURL, input.CredentialMode, input.RechargeMultiplier); err != nil {
		return nil, err
	}
	if input.Credentials == nil {
		return nil, errors.New("upstream credentials are required")
	}
	if err := validateCredentials(input.SiteType, input.CredentialMode, *input.Credentials); err != nil {
		return nil, err
	}
	if input.CredentialMode == CredentialModeAPIKey && len(input.FixedRoutes) == 0 {
		return nil, errors.New("fixed route mode requires at least one route")
	}
	ciphertext, err := s.codec.Encrypt(*input.Credentials)
	if err != nil {
		return nil, err
	}
	station := &Station{
		Name: input.Name, SiteType: normalizeSiteType(input.SiteType), BaseURL: normalizeBaseURL(input.BaseURL),
		CredentialMode: input.CredentialMode, CredentialCipher: ciphertext,
		RechargeMultiplier: input.RechargeMultiplier, RechargeSource: normalizeRechargeSource(input.RechargeSource),
		Enabled: input.Enabled, AutoSync: input.AutoSync, HealthStatus: HealthStatusUnknown,
	}
	created, err := s.repository.CreateStation(ctx, station)
	if err != nil {
		return nil, err
	}
	for _, route := range input.FixedRoutes {
		if _, err := s.CreateFixedRoute(ctx, created.ID, route); err != nil {
			return created, err
		}
	}
	return created, nil
}

func (s *Service) UpdateStation(ctx context.Context, id int64, input StationUpdateInput) (*Station, error) {
	station, err := s.repository.GetStation(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.Name != nil {
		station.Name = strings.TrimSpace(*input.Name)
	}
	if input.SiteType != nil {
		station.SiteType = normalizeSiteType(*input.SiteType)
	}
	if input.BaseURL != nil {
		station.BaseURL = normalizeBaseURL(*input.BaseURL)
	}
	credentials, err := s.codec.Decrypt(station.CredentialCipher)
	if err != nil {
		return nil, err
	}
	if input.CredentialMode != nil {
		station.CredentialMode = strings.TrimSpace(*input.CredentialMode)
	}
	if input.Credentials != nil {
		credentials = mergeCredentials(credentials, *input.Credentials)
	}
	rechargeMultiplierChanged := input.RechargeMultiplier != nil && station.RechargeMultiplier != *input.RechargeMultiplier
	if input.RechargeMultiplier != nil {
		station.RechargeMultiplier = *input.RechargeMultiplier
	}
	if input.RechargeSource != nil {
		station.RechargeSource = normalizeRechargeSource(*input.RechargeSource)
	}
	if input.Enabled != nil {
		station.Enabled = *input.Enabled
	}
	if input.AutoSync != nil {
		station.AutoSync = *input.AutoSync
	}
	if err := validateStationInput(station.Name, station.SiteType, station.BaseURL, station.CredentialMode, station.RechargeMultiplier); err != nil {
		return nil, err
	}
	if err := validateCredentials(station.SiteType, station.CredentialMode, credentials); err != nil {
		return nil, err
	}
	if input.Credentials != nil {
		station.CredentialCipher, err = s.codec.Encrypt(credentials)
		if err != nil {
			return nil, err
		}
	}
	if err := s.repository.UpdateStation(ctx, station); err != nil {
		return nil, err
	}
	if rechargeMultiplierChanged {
		routes, listErr := s.repository.ListRoutes(ctx, station.ID)
		if listErr != nil {
			return nil, listErr
		}
		for _, route := range routes {
			if route.RechargeMultiplier == station.RechargeMultiplier {
				continue
			}
			if _, updateErr := s.UpdateRoute(ctx, route.ID, RouteUpdateInput{RechargeMultiplier: &station.RechargeMultiplier}); updateErr != nil {
				return nil, updateErr
			}
		}
	}
	return s.repository.GetStation(ctx, id)
}

func (s *Service) DeleteStation(ctx context.Context, id int64) error {
	routes, err := s.repository.ListRoutes(ctx, id)
	if err != nil {
		return err
	}
	for _, route := range routes {
		if route.ManagedAccountID != nil && *route.ManagedAccountID > 0 {
			_, _ = s.admin.SetAccountSchedulable(ctx, *route.ManagedAccountID, false)
		}
	}
	return s.repository.DeleteStation(ctx, id)
}

func (s *Service) TestStation(ctx context.Context, id int64) (*ConnectionTestResult, error) {
	station, err := s.repository.GetStation(ctx, id)
	if err != nil {
		return nil, err
	}
	credentials, err := s.codec.Decrypt(station.CredentialCipher)
	if err != nil {
		return nil, err
	}
	connector, err := s.registry.Resolve(ctx, station)
	if err != nil {
		return nil, err
	}
	result := &ConnectionTestResult{SiteType: connector.Type()}
	if station.CredentialMode == CredentialModeAPIKey {
		routes, listErr := s.repository.ListRoutes(ctx, id)
		if listErr != nil {
			return nil, listErr
		}
		discoverer, ok := connector.(ModelDiscoverer)
		if !ok {
			return nil, errors.New("upstream connector does not support model discovery")
		}
		for _, route := range routes {
			if _, modelErr := discoverer.ListModels(ctx, station.BaseURL, credentials.APIKey, route.Platform); modelErr != nil {
				return nil, modelErr
			}
			result.RouteCount++
		}
		_ = s.repository.UpdateStationObservation(ctx, id, nil, 0, HealthStatusHealthy, "", true, false)
		return result, nil
	}
	session, err := connector.Authenticate(ctx, station, credentials)
	if err != nil {
		return nil, err
	}
	if balance, balanceErr := connector.GetBalance(ctx, station.BaseURL, session); balanceErr == nil {
		result.Balance = &balance
	}
	groups, err := connector.ListGroups(ctx, station.BaseURL, session)
	if err != nil {
		return nil, err
	}
	result.GroupCount = len(groups)
	_ = s.repository.UpdateStationObservation(ctx, id, result.Balance, 0, HealthStatusHealthy, "", true, false)
	return result, nil
}

func (s *Service) SyncStation(ctx context.Context, id int64) (*SyncResult, error) {
	return s.syncer.SyncStation(ctx, id)
}

func (s *Service) SyncAll(ctx context.Context) []SyncResult {
	stations, err := s.repository.ListStations(ctx)
	if err != nil {
		return []SyncResult{{Errors: []string{err.Error()}}}
	}
	results := make([]SyncResult, 0, len(stations))
	for _, station := range stations {
		if !station.Enabled {
			continue
		}
		result, syncErr := s.syncer.SyncStation(ctx, station.ID)
		if syncErr != nil {
			result.Errors = append(result.Errors, syncErr.Error())
		}
		results = append(results, *result)
	}
	return results
}

func (s *Service) ListRoutes(ctx context.Context, stationID int64) ([]Route, error) {
	return s.repository.ListRoutes(ctx, stationID)
}

func (s *Service) CreateFixedRoute(ctx context.Context, stationID int64, input FixedRouteInput) (*Route, error) {
	station, err := s.repository.GetStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	if station.CredentialMode != CredentialModeAPIKey {
		return nil, errors.New("manual fixed routes require api_key credential mode")
	}
	credentials, err := s.codec.Decrypt(station.CredentialCipher)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.RemoteGroupKey) == "" {
		input.RemoteGroupKey = "fixed"
	}
	if input.GroupRate < 0 {
		return nil, errors.New("group rate must be >= 0")
	}
	cipher, err := s.encryptor.Encrypt(credentials.APIKey)
	if err != nil {
		return nil, err
	}
	route := &Route{
		StationID: stationID, RemoteGroupKey: input.RemoteGroupKey,
		RemoteGroupName: firstNonEmpty(input.RemoteGroupName, input.RemoteGroupKey),
		Platform:        normalizeRoutePlatform(input.Platform), Models: normalizeModels(input.Models),
		GroupRate: input.GroupRate, RechargeMultiplier: station.RechargeMultiplier,
		FixedRoute: true, APIKeyCipher: cipher, Schedulable: boolDefault(input.Schedulable, true), HealthStatus: HealthStatusUnknown,
	}
	return s.repository.UpsertRoute(ctx, route)
}

func (s *Service) UpdateRoute(ctx context.Context, id int64, input RouteUpdateInput) (*Route, error) {
	route, err := s.repository.GetRoute(ctx, id)
	if err != nil {
		return nil, err
	}
	previous := *route
	if input.RemoteGroupName != nil {
		route.RemoteGroupName = strings.TrimSpace(*input.RemoteGroupName)
	}
	if input.Models != nil {
		route.Models = normalizeModels(*input.Models)
	}
	if input.GroupRate != nil {
		if *input.GroupRate < 0 {
			return nil, errors.New("group rate must be >= 0")
		}
		route.GroupRate = *input.GroupRate
	}
	if input.RechargeMultiplier != nil {
		if *input.RechargeMultiplier <= 0 {
			return nil, errors.New("recharge multiplier must be > 0")
		}
		route.RechargeMultiplier = *input.RechargeMultiplier
	}
	if input.Schedulable != nil {
		route.Schedulable = *input.Schedulable
	}
	persisted, err := s.repository.UpsertRoute(ctx, route)
	if err != nil {
		return nil, err
	}
	if previous.GroupRate != persisted.GroupRate || previous.RechargeMultiplier != persisted.RechargeMultiplier || previous.EffectiveRate != persisted.EffectiveRate {
		if err := s.repository.AppendRateSnapshot(ctx, RateSnapshot{
			RouteID: persisted.ID, GroupRate: persisted.GroupRate,
			RechargeMultiplier: persisted.RechargeMultiplier, EffectiveRate: persisted.EffectiveRate,
		}); err != nil {
			return nil, err
		}
	}
	if persisted.ManagedAccountID != nil {
		apiKey, decryptErr := s.encryptor.Decrypt(persisted.APIKeyCipher)
		if decryptErr != nil {
			return nil, decryptErr
		}
		station, stationErr := s.repository.GetStation(ctx, persisted.StationID)
		if stationErr != nil {
			return nil, stationErr
		}
		accountID, materializeErr := s.materializer.Materialize(ctx, station, persisted, apiKey)
		if materializeErr != nil {
			return nil, materializeErr
		}
		if err := s.repository.UpdateRouteManagedAccount(ctx, persisted.ID, accountID); err != nil {
			return nil, err
		}
	}
	return persisted, nil
}

func (s *Service) SetRouteSchedulable(ctx context.Context, id int64, schedulable bool) error {
	route, err := s.repository.GetRoute(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.SetRouteSchedulable(ctx, id, schedulable); err != nil {
		return err
	}
	if route.ManagedAccountID != nil && *route.ManagedAccountID > 0 {
		_, err = s.admin.SetAccountSchedulable(ctx, *route.ManagedAccountID, schedulable && route.HealthStatus != HealthStatusError)
	}
	return err
}

func (s *Service) ListSyncLogs(ctx context.Context, stationID int64, limit int) ([]SyncLog, error) {
	return s.repository.ListSyncLogs(ctx, stationID, limit)
}

func validateStationInput(name, siteType, baseURL, credentialMode string, rechargeMultiplier float64) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("station name is required")
	}
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("station base_url must be an absolute HTTP(S) URL")
	}
	switch normalizeSiteType(siteType) {
	case SiteTypeAuto, SiteTypeNewAPI, SiteTypeSub2API:
	default:
		return fmt.Errorf("unsupported station type %q", siteType)
	}
	switch credentialMode {
	case CredentialModePassword, CredentialModeToken, CredentialModeAPIKey:
	default:
		return fmt.Errorf("unsupported credential mode %q", credentialMode)
	}
	if rechargeMultiplier <= 0 {
		return errors.New("recharge multiplier must be > 0")
	}
	return nil
}

func validateCredentials(siteType, credentialMode string, credentials Credentials) error {
	switch credentialMode {
	case CredentialModePassword:
		if strings.TrimSpace(credentials.Username) == "" {
			return errors.New("username is required for password credentials")
		}
		if strings.TrimSpace(credentials.Password) == "" {
			return errors.New("password is required for password credentials")
		}
	case CredentialModeToken:
		if strings.TrimSpace(credentials.AccessToken) == "" && strings.TrimSpace(credentials.Cookie) == "" {
			return errors.New("access token or cookie is required for token credentials")
		}
		if normalizeSiteType(siteType) == SiteTypeNewAPI && strings.TrimSpace(credentials.UserID) == "" {
			return errors.New("user_id is required for newapi token credentials")
		}
	case CredentialModeAPIKey:
		if strings.TrimSpace(credentials.APIKey) == "" {
			return errors.New("fixed route API key is required")
		}
	}
	return nil
}

func mergeCredentials(current, update Credentials) Credentials {
	mergeString := func(target *string, value string) {
		if strings.TrimSpace(value) != "" {
			*target = strings.TrimSpace(value)
		}
	}
	mergeString(&current.Username, update.Username)
	mergeString(&current.Password, update.Password)
	mergeString(&current.AccessToken, update.AccessToken)
	mergeString(&current.RefreshToken, update.RefreshToken)
	mergeString(&current.Cookie, update.Cookie)
	mergeString(&current.UserID, update.UserID)
	mergeString(&current.APIKey, update.APIKey)
	if len(update.Extra) > 0 {
		if current.Extra == nil {
			current.Extra = make(map[string]any, len(update.Extra))
		}
		for key, value := range update.Extra {
			current.Extra[key] = value
		}
	}
	return current
}

func normalizeSiteType(siteType string) string {
	siteType = strings.ToLower(strings.TrimSpace(siteType))
	if siteType == "" {
		return SiteTypeAuto
	}
	return siteType
}

func normalizeRechargeSource(source string) string {
	if strings.EqualFold(strings.TrimSpace(source), RechargeSourceAuto) {
		return RechargeSourceAuto
	}
	return RechargeSourceManual
}

package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

var errAutoProvisionStateBusy = errors.New("openai auto-provision state is busy")

type openAIAutoProvisionAccountReader interface {
	GetByID(context.Context, int64) (*Account, error)
	ListSchedulableByPlatform(context.Context, string) ([]Account, error)
	ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error)
}

type openAIAutoProvisionAccountWriter interface {
	UpdateAccount(context.Context, int64, *UpdateAccountInput) (*Account, error)
	ClearAccountError(context.Context, int64) (*Account, error)
}

type OpenAIAutoProvisionService struct {
	accounts    openAIAutoProvisionAccountReader
	admin       openAIAutoProvisionAccountWriter
	settings    *SettingService
	settingRepo SettingRepository
	oauth       *OpenAIOAuthService
	client      openAIProvisionClient
	lockCache   LeaderLockCache
	db          *sql.DB
	instanceID  string
	stateMu     sync.Mutex
	lifecycleMu sync.Mutex
	stop        context.CancelFunc
	started     bool
}

func NewOpenAIAutoProvisionService(
	accounts openAIAutoProvisionAccountReader,
	admin openAIAutoProvisionAccountWriter,
	settings *SettingService,
	settingRepo SettingRepository,
	oauth *OpenAIOAuthService,
	client openAIProvisionClient,
	lockCache LeaderLockCache,
	db *sql.DB,
) *OpenAIAutoProvisionService {
	if client == nil {
		client = newTurbOpenAIProvisionClient(nil)
	}
	return &OpenAIAutoProvisionService{
		accounts:    accounts,
		admin:       admin,
		settings:    settings,
		settingRepo: settingRepo,
		oauth:       oauth,
		client:      client,
		lockCache:   lockCache,
		db:          db,
		instanceID:  automationInstanceID(),
	}
}

type openAIAutoProvisionConfig struct {
	enabled         bool
	target          int
	interval        time.Duration
	turbURL         string
	turbAuthCode    string
	callbackURL     string
	callbackSecret  string
	emailSource     string
	workers         int
	reauthorization bool
}

func (s *OpenAIAutoProvisionService) config(ctx context.Context) (openAIAutoProvisionConfig, error) {
	if s == nil || s.settings == nil {
		return openAIAutoProvisionConfig{}, errors.New("openai auto-provision settings are unavailable")
	}
	settings, err := s.settings.GetAllSettings(ctx)
	if err != nil {
		return openAIAutoProvisionConfig{}, fmt.Errorf("load openai auto-provision settings: %w", err)
	}
	interval := time.Duration(settings.OpenAIAutoProvisionIntervalSeconds) * time.Second
	if interval < 15*time.Second {
		interval = 15 * time.Second
	}
	workers := settings.OpenAIAutoProvisionWorkers
	if workers < 1 {
		workers = 1
	}
	if workers > 16 {
		workers = 16
	}
	return openAIAutoProvisionConfig{
		enabled:         settings.OpenAIAutoProvisionEnabled,
		target:          settings.OpenAIAutoProvisionTarget,
		interval:        interval,
		turbURL:         strings.TrimSpace(settings.OpenAIAutoProvisionTurbURL),
		turbAuthCode:    strings.TrimSpace(settings.OpenAIAutoProvisionTurbAuthCode),
		callbackURL:     strings.TrimSpace(settings.OpenAIAutoProvisionCallbackURL),
		callbackSecret:  strings.TrimSpace(settings.OpenAIAutoProvisionCallbackSecret),
		emailSource:     strings.TrimSpace(settings.OpenAIAutoProvisionEmailSource),
		workers:         workers,
		reauthorization: settings.OpenAIReauthorizationEnabled,
	}, nil
}

func automationInstanceID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (s *OpenAIAutoProvisionService) Start() {
	if s == nil {
		return
	}
	s.lifecycleMu.Lock()
	if s.started {
		s.lifecycleMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.stop = cancel
	s.started = true
	s.lifecycleMu.Unlock()
	go s.runLoop(ctx)
}

func (s *OpenAIAutoProvisionService) runLoop(ctx context.Context) {
	for {
		_ = s.RunOnce(ctx)
		interval := time.Minute
		if cfg, err := s.config(ctx); err == nil {
			interval = cfg.interval
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func (s *OpenAIAutoProvisionService) Stop() {
	if s == nil {
		return
	}
	s.lifecycleMu.Lock()
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
	s.lifecycleMu.Unlock()
}

func (s *OpenAIAutoProvisionService) RunOnce(ctx context.Context) error {
	cfg, err := s.config(ctx)
	if err != nil {
		return err
	}
	if !cfg.enabled || cfg.target <= 0 {
		return nil
	}
	if err := validateAutomationDispatchConfig(cfg); err != nil {
		return err
	}
	release, ok := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, autoProvisionRunLockKey, s.instanceID, 5*time.Minute)
	if !ok {
		return nil
	}
	defer release()

	healthyAccounts, err := s.accounts.ListSchedulableByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return fmt.Errorf("list healthy OpenAI accounts: %w", err)
	}

	var provision *openAIProvisionCommand
	err = s.updateState(ctx, func(state *autoProvisionState) error {
		pruneAutoProvisionState(state, time.Now().UTC())
		if cfg.enabled && state.Provision == nil {
			deficit := openAIProvisionDeficit(countHealthyOpenAIOAuthAccounts(healthyAccounts), cfg.target)
			if deficit > 0 {
				if deficit > maxAutoProvisionBatch {
					deficit = maxAutoProvisionBatch
				}
				pending := &autoProvisionPending{
					RequestID:      newAutoProvisionRequestID(),
					RequestedCount: deficit,
					CreatedAt:      time.Now().UTC(),
				}
				state.Provision = pending
				provision = &openAIProvisionCommand{
					RequestID: pending.RequestID, Count: deficit, Workers: cfg.workers,
					EmailSource: cfg.emailSource, CallbackURL: cfg.callbackURL,
				}
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errAutoProvisionStateBusy) {
		return err
	}
	if err == nil && provision != nil {
		if dispatchErr := s.client.Provision(ctx, *provision, cfg.turbURL, cfg.turbAuthCode); dispatchErr != nil {
			_ = s.removeProvision(ctx, provision.RequestID)
			return fmt.Errorf("dispatch OpenAI provision request: %w", dispatchErr)
		}
	}
	return nil
}

func validateAutomationDispatchConfig(cfg openAIAutoProvisionConfig) error {
	if cfg.turbURL == "" || cfg.turbAuthCode == "" || cfg.callbackURL == "" || cfg.callbackSecret == "" {
		return errors.New("openai automation requires turb URL, turb auth code, callback URL, and callback secret")
	}
	if _, err := validatedAutomationBaseURL(cfg.callbackURL); err != nil {
		return fmt.Errorf("invalid automation callback URL: %w", err)
	}
	return nil
}

// ReauthorizeErroredOpenAIOAuthAccounts is called by TokenRefreshService's
// existing OAuth refresh cycle. It reconciles already errored accounts without
// starting a second reauthorization scheduler.
func (s *OpenAIAutoProvisionService) ReauthorizeErroredOpenAIOAuthAccounts(ctx context.Context) error {
	cfg, err := s.config(ctx)
	if err != nil {
		return err
	}
	if !cfg.reauthorization {
		return nil
	}
	if err := validateAutomationDispatchConfig(cfg); err != nil {
		return err
	}
	release, ok := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, autoProvisionRunLockKey, s.instanceID, 5*time.Minute)
	if !ok {
		return nil
	}
	defer release()

	errorAccounts, err := s.accounts.ListAllWithFilters(ctx, PlatformOpenAI, AccountTypeOAuth, StatusError, "", 0, "")
	if err != nil {
		return fmt.Errorf("list errored OpenAI OAuth accounts: %w", err)
	}

	var reauthorizations []openAIReauthorizeCommand
	err = s.updateState(ctx, func(state *autoProvisionState) error {
		pruneAutoProvisionState(state, time.Now().UTC())
		for _, account := range errorAccounts {
			if len(reauthorizations) >= maxAutoReauthorization || account.IsCredentialShadow() || !account.IsOpenAIOAuth() {
				continue
			}
			if autoReauthorizationPendingForAccount(state, account.ID) {
				continue
			}
			email := strings.TrimSpace(account.GetCredential("email"))
			if email == "" {
				continue
			}
			requestID := newAutoProvisionRequestID()
			state.Reauthorizations[requestID] = autoReauthorizationPending{
				RequestID: requestID, AccountID: account.ID, Email: email, CreatedAt: time.Now().UTC(),
			}
			reauthorizations = append(reauthorizations, openAIReauthorizeCommand{
				RequestID: requestID, AccountID: account.ID, Email: email, CallbackURL: cfg.callbackURL,
			})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, command := range reauthorizations {
		if dispatchErr := s.client.Reauthorize(ctx, command, cfg.turbURL, cfg.turbAuthCode); dispatchErr != nil {
			_ = s.removeReauthorization(ctx, command.RequestID)
			slog.Error("openai auto-reauthorization dispatch failed", "request_id", command.RequestID, "account_id", command.AccountID, "error", dispatchErr)
		}
	}
	return nil
}

func newAutoProvisionRequestID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err == nil {
		return "openai-auto-" + hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("openai-auto-%d", time.Now().UnixNano())
}

func autoReauthorizationPendingForAccount(state *autoProvisionState, accountID int64) bool {
	for _, pending := range state.Reauthorizations {
		if pending.AccountID == accountID {
			return true
		}
	}
	return false
}

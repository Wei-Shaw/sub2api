package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const (
	openAIModelAutoSyncCycle       = time.Minute
	openAIModelAutoSyncTimeout     = 30 * time.Second
	openAIModelAutoSyncConcurrency = 4
	openAIModelAutoSyncBatchSize   = 20
	openAIModelAutoSyncRunTimeout  = 3 * time.Minute
	openAIModelAutoSyncLockTTL     = 4 * time.Minute
)

// OpenAIModelSyncRepository requires a bounded scan and atomic conditional writes
// without extending the read-only repositories used by gateway callers.
type OpenAIModelSyncRepository interface {
	GetByID(context.Context, int64) (*Account, error)
	ListDueOpenAIModelSyncAccounts(context.Context, time.Time, int) ([]Account, error)
	CommitOpenAIModelSync(context.Context, *Account, map[string]string, *OpenAIModelAutoSyncState) error
}

type OpenAIModelAutoSyncService struct {
	repo         OpenAIModelSyncRepository
	fetchModels  func(context.Context, *Account) ([]string, error)
	refreshToken func(context.Context, *Account) (string, error)
	lockCache    LeaderLockCache
	db           *sql.DB
	instanceID   string
	now          func() time.Time
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	started      bool
	stopped      bool
	wg           sync.WaitGroup
	cycleMu      sync.Mutex
}

func NewOpenAIModelAutoSyncService(repo OpenAIModelSyncRepository, testService *AccountTestService, tokenProvider *OpenAITokenProvider) *OpenAIModelAutoSyncService {
	ctx, cancel := context.WithCancel(context.Background())
	s := &OpenAIModelAutoSyncService{
		repo: repo, ctx: ctx, cancel: cancel, now: time.Now, instanceID: uuid.NewString(),
	}
	if testService != nil {
		s.fetchModels = testService.fetchOpenAIModelsForAutoSync
	}
	if tokenProvider != nil {
		s.refreshToken = tokenProvider.GetAccessToken
	}
	return s
}

func ProvideOpenAIModelAutoSyncService(accountRepo AccountRepository, testService *AccountTestService, tokenProvider *OpenAITokenProvider, lockCache LeaderLockCache, db *sql.DB) (*OpenAIModelAutoSyncService, error) {
	repo, ok := accountRepo.(OpenAIModelSyncRepository)
	if !ok {
		return nil, errors.New("account repository does not support automatic model sync")
	}
	svc := NewOpenAIModelAutoSyncService(repo, testService, tokenProvider)
	svc.lockCache, svc.db = lockCache, db
	svc.Start()
	return svc, nil
}

func (s *OpenAIModelAutoSyncService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.stopped || s.repo == nil || s.fetchModels == nil {
		return
	}
	s.started = true
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runCycle()
		ticker := time.NewTicker(openAIModelAutoSyncCycle)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.runCycle()
			}
		}
	}()
}

func (s *OpenAIModelAutoSyncService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.stopped = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *OpenAIModelAutoSyncService) runCycle() {
	if err := s.RunDue(s.ctx); err != nil && s.ctx.Err() == nil {
		// Upstream bodies/URLs/credentials must never enter logs or saved state.
		slog.Warn("openai_model_auto_sync_cycle_failed")
	}
}

func (s *OpenAIModelAutoSyncService) RunDue(ctx context.Context) error {
	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, openAIModelAutoSyncRunTimeout)
	defer cancel()
	release, acquired := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db,
		"openai-model-auto-sync", s.instanceID, openAIModelAutoSyncLockTTL)
	if !acquired {
		return nil
	}
	defer release()
	accounts, err := s.repo.ListDueOpenAIModelSyncAccounts(ctx, s.now(), openAIModelAutoSyncBatchSize)
	if err != nil {
		return fmt.Errorf("list accounts for model sync: %w", err)
	}
	var group errgroup.Group
	group.SetLimit(openAIModelAutoSyncConcurrency)
	for i := range accounts {
		if i >= openAIModelAutoSyncBatchSize || ctx.Err() != nil {
			break
		}
		account := accounts[i]
		group.Go(func() error {
			if err := s.syncAccount(ctx, account.ID); err != nil && !errors.Is(err, ErrOpenAIModelAutoSyncChanged) && ctx.Err() == nil {
				slog.Warn("openai_model_auto_sync_write_failed", "account_id", account.ID)
			}
			return nil
		})
	}
	return group.Wait()
}

func (s *OpenAIModelAutoSyncService) syncAccount(parent context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(parent, openAIModelAutoSyncTimeout)
	account, err := s.repo.GetByID(ctx, id)
	if err != nil {
		cancel()
		return err
	}
	if !openAIModelAutoSyncDue(account, s.now()) {
		cancel()
		return nil
	}
	if account.Type == AccountTypeOAuth && !account.IsOpenAIAgentIdentity() && s.refreshToken != nil {
		_, err = s.refreshToken(ctx, account)
		if err == nil {
			// Token refresh may have written a rotated credential. Re-read before
			// discovery and CAS rather than persisting a pre-refresh snapshot.
			account, err = s.repo.GetByID(ctx, id)
			if err == nil && !openAIModelAutoSyncDue(account, s.now()) {
				cancel()
				return nil
			}
		}
	}
	if account == nil {
		cancel()
		return err
	}
	var models []string
	if err == nil {
		models, err = s.fetchModels(ctx, account)
	}
	cancel()
	if parent.Err() != nil {
		return parent.Err()
	}
	now := s.now()
	previous := decodeOpenAIModelAutoSyncState(account.Extra)
	var mapping map[string]string
	var state OpenAIModelAutoSyncState
	if err != nil {
		state = failedOpenAIModelSync(previous, now, "upstream_error")
	} else {
		mapping, state = reconcileOpenAIModels(account, models, previous, now)
	}
	// A timed-out network request still needs to record its next attempt, using
	// a separate bounded context. Application shutdown cancels both phases.
	writeCtx, writeCancel := context.WithTimeout(parent, 5*time.Second)
	defer writeCancel()
	return s.repo.CommitOpenAIModelSync(writeCtx, account, mapping, &state)
}

func openAIModelAutoSyncDue(account *Account, now time.Time) bool {
	return openAIModelAutoSyncEnabled(account) && account.IsActive() &&
		(account.ExpiresAt == nil || now.Before(*account.ExpiresAt)) &&
		decodeOpenAIModelAutoSyncState(account.Extra).NextSyncUnix <= now.Unix()
}

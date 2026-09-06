package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

const (
	OpenCodeGoUsageAutoRefreshExtraKey = "opencode_go_usage_auto_refresh"
	OpenCodeGoUsageSnapshotExtraKey    = "opencode_go_usage_snapshot"

	// OpenCodeGoUsageMinFetchInterval is the hard floor between two successful
	// fetches of the same group, mirroring the floor nextOpenCodeGoUsageDelay
	// applies to next_refresh_at. Activity may bring a refresh forward to this
	// bound but never past it. Exported so the repository can apply the same
	// floor inside the SQL due filter.
	OpenCodeGoUsageMinFetchInterval = opencodeGoUsageMinIntervalMinutes * time.Minute

	opencodeGoUsageAPIURL                 = "https://opencode.ai/zen/go/v1/usage"
	opencodeGoUsageDefaultIntervalMinutes = 15
	opencodeGoUsageMinIntervalMinutes     = 5
	opencodeGoUsageMaxIntervalMinutes     = 24 * 60
	opencodeGoUsageDefaultDebounceMinutes = 1
	opencodeGoUsageMinDebounceMinutes     = 1
	opencodeGoUsageMaxDebounceMinutes     = 60
	opencodeGoUsageCycleInterval          = time.Minute
	opencodeGoUsageManualRefreshInterval  = 30 * time.Second
	opencodeGoUsageRequestTimeout         = 15 * time.Second
	opencodeGoUsageMaxBodyBytes           = 512 * 1024
	opencodeGoUsageMaxPerCycle            = 20
	opencodeGoUsageConcurrency            = 4
	opencodeGoUsageMaxDelay               = 24 * time.Hour
	opencodeGoUsageLeaderLockKey          = "opencode:go:usage:leader"
	opencodeGoUsageLeaderLockTTL          = 2 * time.Minute
)

var (
	ErrOpenCodeGoUsageUnavailable = infraerrors.ServiceUnavailable(
		"OPENCODE_GO_USAGE_UNAVAILABLE", "OpenCode Go usage is unavailable",
	)
	ErrOpenCodeGoUsageAccountInvalid = infraerrors.BadRequest(
		"OPENCODE_GO_USAGE_ACCOUNT_INVALID", "account must be an OpenAI API key account using https://opencode.ai/zen/go/v1",
	)
	ErrOpenCodeGoUsageIdentityChanged = infraerrors.Conflict(
		"OPENCODE_GO_USAGE_IDENTITY_CHANGED", "account identity or proxy changed during refresh; retry",
	)
	ErrOpenCodeGoUsageRefreshRateLimited = infraerrors.TooManyRequests(
		"OPENCODE_GO_USAGE_REFRESH_RATE_LIMITED", "OpenCode Go usage can be refreshed manually once every 30 seconds",
	)
)

const (
	OpenCodeGoUsageStatusOK           = "ok"
	OpenCodeGoUsageStatusUnauthorized = "unauthorized"
	OpenCodeGoUsageStatusFailed       = "failed"
)

// OpenCodeGoUsageSettings controls the opt-in request-driven refresh runner.
//
// IntervalMinutes is the max-wait bound: when model requests keep arriving and
// the trailing debounce keeps sliding, a refresh is forced after this long.
// DebounceMinutes is the quiet period after the latest request in a group.
type OpenCodeGoUsageSettings struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"interval_minutes"` // max wait while requests continue
	DebounceMinutes int  `json:"debounce_minutes"` // trailing quiet period after last request
}

// OpenCodeGoUsageWindow is a narrow, sanitized view of one official usage window.
type OpenCodeGoUsageWindow struct {
	Status   string    `json:"status"`
	Percent  float64   `json:"percent"`
	ResetsAt time.Time `json:"resets_at"`
}

// OpenCodeGoUsageData intentionally excludes raw upstream payload details.
type OpenCodeGoUsageData struct {
	Rolling OpenCodeGoUsageWindow `json:"rolling"`
	Weekly  OpenCodeGoUsageWindow `json:"weekly"`
	Monthly OpenCodeGoUsageWindow `json:"monthly"`
}

// OpenCodeGoUsageSnapshot is the only usage observation persisted in account extra.
//
// NextRefreshAt remains a persisted compatibility field. For status=ok it is a
// max-wait horizon marker only; automatic success refreshes are driven by model
// request activity (group last_used_at + debounce/max-wait), not by this field
// alone. For failed/unauthorized snapshots it is the failure not-before time
// (Retry-After / exponential backoff) and is enforced as max(activityDue, NextRefreshAt).
type OpenCodeGoUsageSnapshot struct {
	Status        string               `json:"status"`
	Data          *OpenCodeGoUsageData `json:"data,omitempty"`
	FetchedAt     *time.Time           `json:"fetched_at,omitempty"`
	LastAttemptAt time.Time            `json:"last_attempt_at"`
	NextRefreshAt time.Time            `json:"next_refresh_at"`
	FailureCount  int                  `json:"failure_count,omitempty"`
	HTTPStatus    int                  `json:"http_status,omitempty"`
	LastError     string               `json:"last_error,omitempty"`
}

// OpenCodeGoUsageState is the dedicated DTO exposed to administrators.
type OpenCodeGoUsageState struct {
	AccountID          int64                    `json:"account_id"`
	Eligible           bool                     `json:"eligible"`
	AutoRefreshEnabled bool                     `json:"auto_refresh_enabled"`
	Snapshot           *OpenCodeGoUsageSnapshot `json:"snapshot,omitempty"`
}

type openCodeGoUsageRepository interface {
	ListOpenCodeGoUsageGroupAccounts(context.Context, []*Account) ([]Account, error)
	SetOpenCodeGoUsageAutoRefresh(context.Context, *Account, bool) error
	UpdateOpenCodeGoUsageSnapshot(context.Context, *Account, *OpenCodeGoUsageSnapshot) error
	DisableOpenCodeGoUsageAutoRefresh(context.Context, *Account) error
	ListDueOpenCodeGoUsageAccounts(context.Context, time.Time, time.Duration, time.Duration, int) ([]Account, error)
}

// GetOpenCodeGoUsageSettings returns fail-safe defaults when the setting is absent.
func (s *SettingService) GetOpenCodeGoUsageSettings(ctx context.Context) (*OpenCodeGoUsageSettings, error) {
	defaults := defaultOpenCodeGoUsageSettings()
	if s == nil || s.settingRepo == nil {
		return defaults, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpenCodeGoUsageSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return defaults, nil
		}
		return nil, fmt.Errorf("get OpenCode Go usage settings: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return defaults, nil
	}
	settings := *defaults
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, fmt.Errorf("parse OpenCode Go usage settings: %w", err)
	}
	if settings.IntervalMinutes == 0 {
		settings.IntervalMinutes = defaults.IntervalMinutes
	}
	if settings.DebounceMinutes == 0 {
		settings.DebounceMinutes = defaults.DebounceMinutes
	}
	normalizeOpenCodeGoUsageSettings(&settings)
	return &settings, nil
}

func (s *SettingService) SetOpenCodeGoUsageSettings(ctx context.Context, settings *OpenCodeGoUsageSettings) error {
	if s == nil || s.settingRepo == nil {
		return ErrOpenCodeGoUsageUnavailable
	}
	if settings == nil {
		return infraerrors.BadRequest("INVALID_OPENCODE_GO_USAGE_SETTINGS", "settings cannot be nil")
	}
	if settings.DebounceMinutes == 0 {
		// Legacy clients that omit debounce_minutes keep the fail-safe default.
		settings.DebounceMinutes = opencodeGoUsageDefaultDebounceMinutes
	}
	if settings.IntervalMinutes < opencodeGoUsageMinIntervalMinutes || settings.IntervalMinutes > opencodeGoUsageMaxIntervalMinutes {
		return infraerrors.BadRequest(
			"INVALID_OPENCODE_GO_USAGE_INTERVAL",
			fmt.Sprintf("interval_minutes must be between %d and %d", opencodeGoUsageMinIntervalMinutes, opencodeGoUsageMaxIntervalMinutes),
		)
	}
	if settings.DebounceMinutes < opencodeGoUsageMinDebounceMinutes || settings.DebounceMinutes > opencodeGoUsageMaxDebounceMinutes {
		return infraerrors.BadRequest(
			"INVALID_OPENCODE_GO_USAGE_DEBOUNCE",
			fmt.Sprintf("debounce_minutes must be between %d and %d", opencodeGoUsageMinDebounceMinutes, opencodeGoUsageMaxDebounceMinutes),
		)
	}
	// The due time is min(lastUsed+debounce, fetchedAt+maxWait). Once the debounce
	// reaches the max wait the debounce term can never win, so the knob would be
	// silently inert instead of doing what the operator asked for.
	if settings.DebounceMinutes >= settings.IntervalMinutes {
		return infraerrors.BadRequest(
			"INVALID_OPENCODE_GO_USAGE_DEBOUNCE",
			fmt.Sprintf("debounce_minutes (%d) must be less than interval_minutes (%d)", settings.DebounceMinutes, settings.IntervalMinutes),
		)
	}
	normalizeOpenCodeGoUsageSettings(settings)
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal OpenCode Go usage settings: %w", err)
	}
	return s.settingRepo.Set(ctx, SettingKeyOpenCodeGoUsageSettings, string(data))
}

func defaultOpenCodeGoUsageSettings() *OpenCodeGoUsageSettings {
	return &OpenCodeGoUsageSettings{
		Enabled:         false,
		IntervalMinutes: opencodeGoUsageDefaultIntervalMinutes,
		DebounceMinutes: opencodeGoUsageDefaultDebounceMinutes,
	}
}

func normalizeOpenCodeGoUsageSettings(settings *OpenCodeGoUsageSettings) {
	if settings.IntervalMinutes < opencodeGoUsageMinIntervalMinutes {
		settings.IntervalMinutes = opencodeGoUsageMinIntervalMinutes
	}
	if settings.IntervalMinutes > opencodeGoUsageMaxIntervalMinutes {
		settings.IntervalMinutes = opencodeGoUsageMaxIntervalMinutes
	}
	if settings.DebounceMinutes <= 0 {
		settings.DebounceMinutes = opencodeGoUsageDefaultDebounceMinutes
	}
	if settings.DebounceMinutes < opencodeGoUsageMinDebounceMinutes {
		settings.DebounceMinutes = opencodeGoUsageMinDebounceMinutes
	}
	if settings.DebounceMinutes > opencodeGoUsageMaxDebounceMinutes {
		settings.DebounceMinutes = opencodeGoUsageMaxDebounceMinutes
	}
}

func openCodeGoUsageDurations(settings *OpenCodeGoUsageSettings) (debounce, maxWait time.Duration) {
	normalized := defaultOpenCodeGoUsageSettings()
	if settings != nil {
		*normalized = *settings
	}
	normalizeOpenCodeGoUsageSettings(normalized)
	return time.Duration(normalized.DebounceMinutes) * time.Minute,
		time.Duration(normalized.IntervalMinutes) * time.Minute
}

// openCodeGoUsageIsAutoRefreshDue decides whether a configured auto-refresh
// group should fetch now. groupLastUsedAt must be MAX(last_used_at) across the
// exact api_key group so shared multi-account groups do not miss activity.
//
// Success: a request must be newer than fetched_at; dueAt = min(lastUsed+debounce, fetchedAt+maxWait).
// Failure: a request must be newer than last_attempt_at; activity due uses the same min formula,
// then dueAt = max(activityDue, next_refresh_at) so Retry-After / exponential backoff win.
// Missing or invalid snapshots fail open to a first fetch.
func openCodeGoUsageIsAutoRefreshDue(
	snapshot *OpenCodeGoUsageSnapshot,
	groupLastUsedAt *time.Time,
	now time.Time,
	debounce, maxWait time.Duration,
) bool {
	dueAt, ok := openCodeGoUsageAutoRefreshDueAt(snapshot, groupLastUsedAt, debounce, maxWait)
	if !ok {
		return false
	}
	return !now.Before(dueAt)
}

func openCodeGoUsageAutoRefreshDueAt(
	snapshot *OpenCodeGoUsageSnapshot,
	groupLastUsedAt *time.Time,
	debounce, maxWait time.Duration,
) (time.Time, bool) {
	if debounce <= 0 {
		debounce = time.Duration(opencodeGoUsageDefaultDebounceMinutes) * time.Minute
	}
	if maxWait <= 0 {
		maxWait = time.Duration(opencodeGoUsageDefaultIntervalMinutes) * time.Minute
	}
	if snapshot == nil {
		return time.Time{}, true
	}
	switch snapshot.Status {
	case OpenCodeGoUsageStatusOK:
		if snapshot.FetchedAt == nil || snapshot.FetchedAt.IsZero() {
			return time.Time{}, true
		}
		fetchedAt := snapshot.FetchedAt.UTC()
		if groupLastUsedAt == nil || !groupLastUsedAt.After(fetchedAt) {
			return time.Time{}, false
		}
		lastUsed := groupLastUsedAt.UTC()
		dueAt := minTime(lastUsed.Add(debounce), fetchedAt.Add(maxWait))
		// Keep the pre-existing hard floor between successful fetches. The success
		// path no longer consults next_refresh_at, which is where
		// nextOpenCodeGoUsageDelay used to apply opencodeGoUsageMinIntervalMinutes;
		// without this, request traffic spaced slightly wider than the debounce
		// drives the group's outbound rate far above the previous minimum.
		if floor := fetchedAt.Add(OpenCodeGoUsageMinFetchInterval); dueAt.Before(floor) {
			return floor, true
		}
		return dueAt, true
	case OpenCodeGoUsageStatusFailed, OpenCodeGoUsageStatusUnauthorized:
		if snapshot.LastAttemptAt.IsZero() {
			return time.Time{}, true
		}
		lastAttempt := snapshot.LastAttemptAt.UTC()
		if groupLastUsedAt == nil || !groupLastUsedAt.After(lastAttempt) {
			return time.Time{}, false
		}
		lastUsed := groupLastUsedAt.UTC()
		activityDue := minTime(lastUsed.Add(debounce), lastAttempt.Add(maxWait))
		if !snapshot.NextRefreshAt.IsZero() && snapshot.NextRefreshAt.UTC().After(activityDue) {
			return snapshot.NextRefreshAt.UTC(), true
		}
		return activityDue, true
	default:
		return time.Time{}, true
	}
}

// maxOpenCodeGoUsageGroupLastUsed returns the newest last_used_at among group members.
func maxOpenCodeGoUsageGroupLastUsed(accounts []Account) *time.Time {
	var latest *time.Time
	for i := range accounts {
		candidate := accounts[i].LastUsedAt
		if candidate == nil || candidate.IsZero() {
			continue
		}
		if latest == nil || candidate.After(*latest) {
			ts := candidate.UTC()
			latest = &ts
		}
	}
	return latest
}

// scheduleOpenCodeGoUsageActivity records that an OpenCode Go API-key account
// actually attempted an upstream model request (including 429/5xx/transport errors).
// Local auth/validation failures must not call this. DeferredService dedupes writes.
func scheduleOpenCodeGoUsageActivity(deferred *DeferredService, account *Account) {
	if deferred == nil || account == nil || !IsOpenCodeGoUsageAccount(account) {
		return
	}
	deferred.ScheduleLastUsedUpdate(account.ID)
}

// OpenCodeGoUsageService refreshes the official usage JSON without affecting routing state.
type OpenCodeGoUsageService struct {
	accountRepo    AccountRepository
	httpUpstream   HTTPUpstream
	settingService *SettingService

	parentCtx    context.Context
	parentCancel context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
	started      bool
	stopped      bool
	cycleMu      sync.Mutex
	refreshGroup singleflight.Group
	refreshSlots chan struct{}
	now          func() time.Time
	lockCache    LeaderLockCache
	db           *sql.DB
	instanceID   string
}

func NewOpenCodeGoUsageService(
	accountRepo AccountRepository,
	httpUpstream HTTPUpstream,
	settingService *SettingService,
) *OpenCodeGoUsageService {
	ctx, cancel := context.WithCancel(context.Background())
	return &OpenCodeGoUsageService{
		accountRepo:    accountRepo,
		httpUpstream:   httpUpstream,
		settingService: settingService,
		parentCtx:      ctx,
		parentCancel:   cancel,
		refreshSlots:   make(chan struct{}, opencodeGoUsageConcurrency),
		now:            time.Now,
		instanceID:     uuid.NewString(),
	}
}

func ProvideOpenCodeGoUsageService(
	accountRepo AccountRepository,
	httpUpstream HTTPUpstream,
	settingService *SettingService,
	lockCache LeaderLockCache,
	db *sql.DB,
) *OpenCodeGoUsageService {
	svc := NewOpenCodeGoUsageService(accountRepo, httpUpstream, settingService)
	svc.lockCache = lockCache
	svc.db = db
	svc.Start()
	return svc
}

func (s *OpenCodeGoUsageService) Start() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go s.runLoop()
}

func (s *OpenCodeGoUsageService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.parentCancel()
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *OpenCodeGoUsageService) runLoop() {
	defer s.wg.Done()
	_ = s.RunDue(s.parentCtx)
	ticker := time.NewTicker(opencodeGoUsageCycleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case <-ticker.C:
			if err := s.RunDue(s.parentCtx); err != nil {
				logger.LegacyPrintf("service.opencode_go_usage", "run_due_failed: err=%v", err)
			}
		}
	}
}

func (s *OpenCodeGoUsageService) GetSettings(ctx context.Context) (*OpenCodeGoUsageSettings, error) {
	if s == nil || s.settingService == nil {
		return defaultOpenCodeGoUsageSettings(), nil
	}
	return s.settingService.GetOpenCodeGoUsageSettings(ctx)
}

func (s *OpenCodeGoUsageService) UpdateSettings(ctx context.Context, settings *OpenCodeGoUsageSettings) error {
	if s == nil || s.settingService == nil {
		return ErrOpenCodeGoUsageUnavailable
	}
	return s.settingService.SetOpenCodeGoUsageSettings(ctx, settings)
}

func (s *OpenCodeGoUsageService) GetState(ctx context.Context, accountID int64) (*OpenCodeGoUsageState, error) {
	if s == nil || s.accountRepo == nil {
		return nil, ErrOpenCodeGoUsageUnavailable
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if err := s.ResolveOpenCodeGoUsageAccounts(ctx, []*Account{account}); err != nil {
		return nil, err
	}
	return OpenCodeGoUsageStateFromAccount(account), nil
}

// ResolveOpenCodeGoUsageAccounts overlays group-owned managed state onto the
// supplied account objects. The repository resolves all matching siblings in one
// bounded query, so account-list responses do not issue one query per row.
func (s *OpenCodeGoUsageService) ResolveOpenCodeGoUsageAccounts(ctx context.Context, accounts []*Account) error {
	if s == nil || s.accountRepo == nil || len(accounts) == 0 {
		return nil
	}
	writer, ok := s.accountRepo.(openCodeGoUsageRepository)
	if !ok {
		return nil
	}
	eligible := make([]*Account, 0, len(accounts))
	for _, account := range accounts {
		if _, ok := openCodeGoUsageGroupFingerprint(account); ok {
			eligible = append(eligible, account)
		}
	}
	if len(eligible) == 0 {
		return nil
	}
	siblings, err := writer.ListOpenCodeGoUsageGroupAccounts(ctx, eligible)
	if err != nil {
		return fmt.Errorf("resolve OpenCode Go usage groups: %w", err)
	}
	sources := make(map[string]*Account)
	for index := range siblings {
		candidate := &siblings[index]
		fingerprint, valid := openCodeGoUsageGroupFingerprint(candidate)
		if !valid {
			continue
		}
		current := sources[fingerprint]
		if current == nil || candidate.UpdatedAt.After(current.UpdatedAt) ||
			(candidate.UpdatedAt.Equal(current.UpdatedAt) && candidate.ID < current.ID) {
			sources[fingerprint] = candidate
		}
	}
	resolvedSources := make(map[string]*Account, len(sources))
	for fingerprint, source := range sources {
		clone := *source
		clone.Extra = make(map[string]any, len(source.Extra))
		maps.Copy(clone.Extra, source.Extra)
		resolvedSources[fingerprint] = &clone
	}
	for index := range siblings {
		candidate := &siblings[index]
		fingerprint, valid := openCodeGoUsageGroupFingerprint(candidate)
		source := resolvedSources[fingerprint]
		if !valid || source == nil {
			continue
		}
		candidateSnapshot := decodeOpenCodeGoUsageSnapshot(candidate.Extra)
		currentSnapshot := decodeOpenCodeGoUsageSnapshot(source.Extra)
		if candidateSnapshot != nil && (currentSnapshot == nil || candidateSnapshot.LastAttemptAt.After(currentSnapshot.LastAttemptAt)) {
			source.Extra[OpenCodeGoUsageSnapshotExtraKey] = candidate.Extra[OpenCodeGoUsageSnapshotExtraKey]
		}
	}
	for _, account := range eligible {
		fingerprint, _ := openCodeGoUsageGroupFingerprint(account)
		applyOpenCodeGoUsageManagedExtra(account, resolvedSources[fingerprint])
	}
	return nil
}

func applyOpenCodeGoUsageManagedExtra(target, source *Account) {
	if target == nil {
		return
	}
	if target.Extra == nil {
		target.Extra = make(map[string]any)
	}
	for _, key := range []string{
		OpenCodeGoUsageAutoRefreshExtraKey,
		OpenCodeGoUsageSnapshotExtraKey,
	} {
		delete(target.Extra, key)
		if source != nil && source.Extra != nil {
			if value, ok := source.Extra[key]; ok {
				target.Extra[key] = value
			}
		}
	}
}

func (s *OpenCodeGoUsageService) SetAutoRefresh(ctx context.Context, accountID int64, enabled bool) (*OpenCodeGoUsageState, error) {
	if s == nil || s.accountRepo == nil {
		return nil, ErrOpenCodeGoUsageUnavailable
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !IsOpenCodeGoUsageAccount(account) {
		return nil, ErrOpenCodeGoUsageAccountInvalid
	}
	if err := s.ResolveOpenCodeGoUsageAccounts(ctx, []*Account{account}); err != nil {
		return nil, err
	}
	writer, ok := s.accountRepo.(openCodeGoUsageRepository)
	if !ok {
		return nil, ErrOpenCodeGoUsageUnavailable
	}
	if err := writer.SetOpenCodeGoUsageAutoRefresh(ctx, account, enabled); err != nil {
		return nil, err
	}
	return s.GetState(ctx, accountID)
}

func (s *OpenCodeGoUsageService) Refresh(ctx context.Context, accountID int64) (*OpenCodeGoUsageState, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.refreshAccount(ctx, accountID, settings, false); err != nil {
		return nil, err
	}
	return s.GetState(ctx, accountID)
}

func (s *OpenCodeGoUsageService) RunDue(ctx context.Context) error {
	if s == nil || s.accountRepo == nil {
		return nil
	}
	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return nil
	}
	release, acquired := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, opencodeGoUsageLeaderLockKey, s.instanceID, opencodeGoUsageLeaderLockTTL)
	if !acquired {
		return nil
	}
	defer release()

	writer, ok := s.accountRepo.(openCodeGoUsageRepository)
	if !ok {
		return ErrOpenCodeGoUsageUnavailable
	}
	now := s.currentTime()
	debounce, maxWait := openCodeGoUsageDurations(settings)
	accounts, err := writer.ListDueOpenCodeGoUsageAccounts(ctx, now, debounce, maxWait, opencodeGoUsageMaxPerCycle)
	if err != nil {
		return fmt.Errorf("list due OpenCode Go usage accounts: %w", err)
	}
	var group errgroup.Group
	seenGroups := make(map[string]struct{}, len(accounts))
	for index := range accounts {
		account := accounts[index]
		fingerprint, valid := openCodeGoUsageGroupFingerprint(&account)
		if !valid || !account.IsActive() || !openCodeGoUsageAutoRefreshEnabled(&account) {
			continue
		}
		if _, duplicate := seenGroups[fingerprint]; duplicate {
			continue
		}
		seenGroups[fingerprint] = struct{}{}
		snapshot := decodeOpenCodeGoUsageSnapshot(account.Extra)
		// ListDue stamps Account.LastUsedAt with the api_key group MAX(last_used_at).
		if !openCodeGoUsageIsAutoRefreshDue(snapshot, account.LastUsedAt, now, debounce, maxWait) {
			continue
		}
		accountID := account.ID
		expected := account
		group.Go(func() error {
			if _, refreshErr := s.refreshAccount(ctx, accountID, settings, true); refreshErr != nil {
				if errors.Is(refreshErr, ErrOpenCodeGoUsageIdentityChanged) {
					if disableErr := writer.DisableOpenCodeGoUsageAutoRefresh(ctx, &expected); disableErr != nil {
						logger.LegacyPrintf("service.opencode_go_usage", "disable_auto_refresh_failed: account_id=%d err=%v", accountID, disableErr)
					}
					return nil
				}
				logger.LegacyPrintf("service.opencode_go_usage", "refresh_due_failed: account_id=%d err=%v", accountID, refreshErr)
			}
			return nil
		})
	}
	return group.Wait()
}

func (s *OpenCodeGoUsageService) refreshAccount(ctx context.Context, accountID int64, settings *OpenCodeGoUsageSettings, requireEnabled bool) (*OpenCodeGoUsageSnapshot, error) {
	if s == nil || s.accountRepo == nil {
		return nil, ErrOpenCodeGoUsageUnavailable
	}
	if settings == nil {
		settings = defaultOpenCodeGoUsageSettings()
	}
	intervalMinutes := settings.IntervalMinutes
	debounce, maxWait := openCodeGoUsageDurations(settings)
	anchor, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	key, valid := openCodeGoUsageGroupFingerprint(anchor)
	if !valid {
		return nil, ErrOpenCodeGoUsageAccountInvalid
	}
	value, err, _ := s.refreshGroup.Do(key, func() (any, error) {
		select {
		case s.refreshSlots <- struct{}{}:
			defer func() { <-s.refreshSlots }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		account, loadErr := s.accountRepo.GetByID(ctx, accountID)
		if loadErr != nil {
			return nil, loadErr
		}
		currentKey, currentValid := openCodeGoUsageGroupFingerprint(account)
		if !currentValid {
			return nil, ErrOpenCodeGoUsageAccountInvalid
		}
		if currentKey != key {
			return nil, ErrOpenCodeGoUsageIdentityChanged
		}
		if err := s.ResolveOpenCodeGoUsageAccounts(ctx, []*Account{account}); err != nil {
			return nil, err
		}
		if !requireEnabled {
			if snapshot := decodeOpenCodeGoUsageSnapshot(account.Extra); snapshot != nil && !snapshot.LastAttemptAt.IsZero() {
				retryAt := snapshot.LastAttemptAt.Add(opencodeGoUsageManualRefreshInterval)
				if now := s.currentTime(); now.Before(retryAt) {
					remaining := retryAt.Sub(now)
					seconds := int((remaining + time.Second - 1) / time.Second)
					return nil, ErrOpenCodeGoUsageRefreshRateLimited.WithMetadata(map[string]string{
						"retry_after_seconds": strconv.Itoa(seconds),
					})
				}
			}
		}
		if requireEnabled {
			if !account.IsActive() || !openCodeGoUsageAutoRefreshEnabled(account) {
				return nil, nil
			}
			groupLastUsed := account.LastUsedAt
			if writer, ok := s.accountRepo.(openCodeGoUsageRepository); ok {
				siblings, listErr := writer.ListOpenCodeGoUsageGroupAccounts(ctx, []*Account{account})
				if listErr != nil {
					// Fall back to this account's own last_used_at. That is a narrower
					// activity signal than the group maximum, so the due check may skip a
					// refresh it would otherwise have run; surface it rather than
					// silently changing the due semantics.
					logger.LegacyPrintf("service.opencode_go_usage",
						"group_last_used_lookup_failed: account_id=%d err=%v", account.ID, listErr)
				} else {
					groupLastUsed = maxOpenCodeGoUsageGroupLastUsed(siblings)
				}
			}
			if !openCodeGoUsageIsAutoRefreshDue(decodeOpenCodeGoUsageSnapshot(account.Extra), groupLastUsed, s.currentTime(), debounce, maxWait) {
				return nil, nil
			}
		}
		return s.refreshLoadedAccount(ctx, account, intervalMinutes)
	})
	if err != nil || value == nil {
		return nil, err
	}
	snapshot, ok := value.(*OpenCodeGoUsageSnapshot)
	if !ok {
		return nil, fmt.Errorf("invalid OpenCode Go usage refresh result")
	}
	return snapshot, nil
}

func (s *OpenCodeGoUsageService) refreshLoadedAccount(ctx context.Context, account *Account, intervalMinutes int) (*OpenCodeGoUsageSnapshot, error) {
	now := s.currentTime().UTC()
	apiKey, _ := account.Credentials["api_key"].(string)
	if apiKey == "" {
		return nil, ErrOpenCodeGoUsageAccountInvalid
	}
	if s.httpUpstream == nil {
		return nil, ErrOpenCodeGoUsageUnavailable
	}
	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy == nil || account.Proxy.ID != *account.ProxyID {
			return nil, ErrOpenCodeGoUsageIdentityChanged
		}
		proxyURL = account.Proxy.URL()
	}
	requestCtx, cancel := context.WithTimeout(WithHTTPUpstreamRedirectsDisabled(ctx), opencodeGoUsageRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, opencodeGoUsageAPIURL, nil)
	if err != nil || !isExactOpenCodeGoUsageURL(req.URL) {
		return nil, ErrOpenCodeGoUsageUnavailable
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "sub2api-opencode-go-usage/1")
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return s.persistFailure(ctx, account, intervalMinutes, now, 0, "request_failed", 0, false)
	}
	if resp == nil || resp.Body == nil {
		return s.persistFailure(ctx, account, intervalMinutes, now, 0, "empty_response", 0, false)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Request != nil && !isExactOpenCodeGoUsageURL(resp.Request.URL) {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "response_host_mismatch", 0, false)
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "redirect_blocked", retryAfter(resp.Header, now), false)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "unauthorized", retryAfter(resp.Header, now), true)
	}
	if resp.StatusCode == http.StatusForbidden {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "OpenCode Go subscription required (403)", retryAfter(resp.Header, now), false)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "http_error", retryAfter(resp.Header, now), false)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, opencodeGoUsageMaxBodyBytes+1))
	if readErr != nil {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "response_read_failed", 0, false)
	}
	if len(body) > opencodeGoUsageMaxBodyBytes {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "response_too_large", 0, false)
	}
	data, parseErr := parseOpenCodeGoUsageJSON(body)
	if parseErr != nil {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "invalid_json", 0, false)
	}
	snapshot := &OpenCodeGoUsageSnapshot{
		Status:        OpenCodeGoUsageStatusOK,
		Data:          data,
		FetchedAt:     &now,
		LastAttemptAt: now,
		NextRefreshAt: now.Add(nextOpenCodeGoUsageDelay(intervalMinutes, 0, 0)),
		HTTPStatus:    resp.StatusCode,
	}
	if err := s.updateSnapshot(ctx, account, snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *OpenCodeGoUsageService) persistFailure(
	ctx context.Context,
	account *Account,
	intervalMinutes int,
	now time.Time,
	httpStatus int,
	reason string,
	retryAfterDuration time.Duration,
	unauthorized bool,
) (*OpenCodeGoUsageSnapshot, error) {
	previous := decodeOpenCodeGoUsageSnapshot(account.Extra)
	failureCount := 1
	if previous != nil {
		failureCount = previous.FailureCount + 1
	}
	status := OpenCodeGoUsageStatusFailed
	if unauthorized {
		status = OpenCodeGoUsageStatusUnauthorized
	}
	snapshot := &OpenCodeGoUsageSnapshot{
		Status:        status,
		LastAttemptAt: now,
		NextRefreshAt: now.Add(nextOpenCodeGoUsageDelay(intervalMinutes, failureCount, retryAfterDuration)),
		FailureCount:  failureCount,
		HTTPStatus:    httpStatus,
		LastError:     reason,
	}
	if previous != nil {
		snapshot.Data = previous.Data
		snapshot.FetchedAt = previous.FetchedAt
	}
	if err := s.updateSnapshot(ctx, account, snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *OpenCodeGoUsageService) updateSnapshot(ctx context.Context, account *Account, snapshot *OpenCodeGoUsageSnapshot) error {
	writer, ok := s.accountRepo.(openCodeGoUsageRepository)
	if !ok {
		return ErrOpenCodeGoUsageUnavailable
	}
	return writer.UpdateOpenCodeGoUsageSnapshot(ctx, account, snapshot)
}

func OpenCodeGoUsageStateFromAccount(account *Account) *OpenCodeGoUsageState {
	state := &OpenCodeGoUsageState{}
	if account == nil {
		return state
	}
	state.AccountID = account.ID
	state.Eligible = IsOpenCodeGoUsageAccount(account)
	if !state.Eligible {
		return state
	}
	state.AutoRefreshEnabled = openCodeGoUsageAutoRefreshEnabled(account)
	state.Snapshot = decodeOpenCodeGoUsageSnapshot(account.Extra)
	return state
}

func IsOpenCodeGoUsageAccount(account *Account) bool {
	if account == nil || account.Type != AccountTypeAPIKey || account.Platform != PlatformOpenAI {
		return false
	}
	baseURL, _ := account.Credentials["base_url"].(string)
	return isOpenCodeGoBaseURL(baseURL)
}

func isOpenCodeGoBaseURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "?#") {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawFragment != "" {
		return false
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname != "opencode.ai" {
		return false
	}
	authority := strings.ToLower(parsed.Host)
	if authority != hostname && authority != hostname+":443" {
		return false
	}
	if parsed.RawPath != "" {
		return false
	}
	return strings.EqualFold(strings.TrimSuffix(parsed.Path, "/"), "/zen/go/v1")
}

func openCodeGoUsageIdentity(account *Account) map[string]any {
	if !IsOpenCodeGoUsageAccount(account) {
		return nil
	}
	apiKey, ok := account.Credentials["api_key"].(string)
	if !ok || apiKey == "" {
		return nil
	}
	return map[string]any{"host": "opencode.ai", "api_key": apiKey}
}

func openCodeGoUsageGroupFingerprint(account *Account) (string, bool) {
	identity := openCodeGoUsageIdentity(account)
	if identity == nil {
		return "", false
	}
	apiKey, _ := identity["api_key"].(string)
	sum := sha256.Sum256([]byte("opencode.ai\x00" + apiKey))
	return hex.EncodeToString(sum[:]), true
}

func isExactOpenCodeGoUsageURL(parsed *url.URL) bool {
	return parsed != nil && parsed.Scheme == "https" && parsed.Host == "opencode.ai" && parsed.Path == "/zen/go/v1/usage" &&
		parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && parsed.RawPath == ""
}

func openCodeGoUsageAutoRefreshEnabled(account *Account) bool {
	if account == nil || account.Extra == nil {
		return false
	}
	enabled, ok := account.Extra[OpenCodeGoUsageAutoRefreshExtraKey].(bool)
	return ok && enabled
}

func decodeOpenCodeGoUsageSnapshot(extra map[string]any) *OpenCodeGoUsageSnapshot {
	if extra == nil {
		return nil
	}
	value, ok := extra[OpenCodeGoUsageSnapshotExtraKey]
	if !ok || value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var snapshot OpenCodeGoUsageSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil
	}
	if snapshot.Status != OpenCodeGoUsageStatusOK && snapshot.Status != OpenCodeGoUsageStatusUnauthorized && snapshot.Status != OpenCodeGoUsageStatusFailed {
		return nil
	}
	return &snapshot
}

// nextOpenCodeGoUsageDelay computes the not-before delay for the next refresh:
// interval * 2^min(failureCount-1, 6) capped at 24h, ±10% jitter capped at 5min,
// never below a Retry-After hint or one minute.
func nextOpenCodeGoUsageDelay(intervalMinutes, failureCount int, retryAfterDuration time.Duration) time.Duration {
	minimumDelay := retryAfterDuration
	base := time.Duration(intervalMinutes) * time.Minute
	if base < opencodeGoUsageMinIntervalMinutes*time.Minute {
		base = opencodeGoUsageMinIntervalMinutes * time.Minute
	}
	if failureCount > 0 {
		shift := min(failureCount-1, 6)
		base *= time.Duration(1 << shift)
	}
	if base > opencodeGoUsageMaxDelay {
		base = opencodeGoUsageMaxDelay
	}
	if retryAfterDuration > base {
		base = retryAfterDuration
	}
	jitterRange := base / 10
	if jitterRange > 5*time.Minute {
		jitterRange = 5 * time.Minute
	}
	if jitterRange > 0 {
		base += time.Duration(rand.Int64N(int64(jitterRange)*2+1)) - jitterRange
	}
	if base < minimumDelay {
		return minimumDelay
	}
	if base < time.Minute {
		return time.Minute
	}
	return base
}

func (s *OpenCodeGoUsageService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

type openCodeGoUsageAPIWindow struct {
	Status   string  `json:"status"`
	Percent  float64 `json:"percent"`
	ResetsAt string  `json:"resetsAt"`
}

type openCodeGoUsageAPIUsage struct {
	Rolling *openCodeGoUsageAPIWindow `json:"rolling"`
	Weekly  *openCodeGoUsageAPIWindow `json:"weekly"`
	Monthly *openCodeGoUsageAPIWindow `json:"monthly"`
}

type openCodeGoUsageAPIResponse struct {
	Usage *openCodeGoUsageAPIUsage `json:"usage"`
}

// parseOpenCodeGoUsageJSON parses the upstream usage payload leniently: the top
// level may wrap the windows in a "usage" object or expose them directly, and
// missing windows/fields degrade to zero values instead of failing the whole
// parse. Only structurally invalid JSON is an error.
func parseOpenCodeGoUsageJSON(body []byte) (*OpenCodeGoUsageData, error) {
	var wrapped openCodeGoUsageAPIResponse
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, err
	}
	usage := wrapped.Usage
	if usage == nil {
		var direct openCodeGoUsageAPIUsage
		if err := json.Unmarshal(body, &direct); err != nil {
			return nil, err
		}
		usage = &direct
	}
	data := &OpenCodeGoUsageData{}
	if usage.Rolling != nil {
		data.Rolling = openCodeGoUsageWindowFromAPI(usage.Rolling)
	}
	if usage.Weekly != nil {
		data.Weekly = openCodeGoUsageWindowFromAPI(usage.Weekly)
	}
	if usage.Monthly != nil {
		data.Monthly = openCodeGoUsageWindowFromAPI(usage.Monthly)
	}
	return data, nil
}

func openCodeGoUsageWindowFromAPI(window *openCodeGoUsageAPIWindow) OpenCodeGoUsageWindow {
	out := OpenCodeGoUsageWindow{Status: window.Status, Percent: window.Percent}
	if resetsAt, err := time.Parse(time.RFC3339, window.ResetsAt); err == nil {
		out.ResetsAt = resetsAt.UTC()
	}
	return out
}

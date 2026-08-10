package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

const (
	// These values live in accounts.extra so PR2 does not require a schema migration.
	UpstreamBillingProbeExtraKey              = "upstream_billing_probe"
	UpstreamBillingProbeEnabledExtraKey       = "upstream_billing_probe_enabled"
	UpstreamBillingRateSyncEnabledExtraKey    = "upstream_billing_rate_sync_enabled"
	UpstreamBillingRechargeMultiplierExtraKey = "upstream_billing_recharge_multiplier"
	UpstreamBillingNewAPIGroupExtraKey        = "upstream_billing_newapi_group"

	upstreamBillingProbeDefaultIntervalMinutes = 30
	upstreamBillingProbeMinIntervalMinutes     = 5
	upstreamBillingProbeMaxIntervalMinutes     = 24 * 60
	upstreamBillingProbeCycleInterval          = time.Minute
	upstreamBillingProbeRequestTimeout         = 10 * time.Second
	upstreamBillingProbeMaxBodyBytes           = 64 * 1024
	// New API's /api/pricing response includes its complete model catalog before
	// group_ratio, so it needs the same order of magnitude as New API's own
	// ratio-sync endpoint while the Sub2API response remains tightly bounded.
	upstreamBillingProbeMaxNewAPIBodyBytes = 10 << 20
	upstreamBillingProbeMaxPerCycle        = 20
	upstreamBillingProbeConcurrency        = 4
	upstreamBillingProbeMaxDelay           = 24 * time.Hour
	// unsupported 账号的重探间隔倍数：上游不是 sub2api 中转就不会突然长出
	// /v1/sub2api/billing，按常规 interval 重排只会持续占满每周期
	// upstreamBillingProbeMaxPerCycle 个名额。
	upstreamBillingProbeUnsupportedDelayFactor = 8
	upstreamBillingProbeAccountRateScale       = 10000.0
	upstreamBillingProbeLeaderLockKey          = "upstream:billing:probe:leader"
	upstreamBillingProbeLeaderLockTTL          = 2 * time.Minute
	upstreamBillingRechargeMultiplierDefault   = 1.0
	upstreamBillingNewAPIGroupMaxRunes         = 100
)

// UpstreamBillingProbeMaxBatchSize limits one manual batch and one runner cycle.
const UpstreamBillingProbeMaxBatchSize = upstreamBillingProbeMaxPerCycle

// upstreamBillingRateSyncMaxMultiplier bounds the value the automatic
// write-back may push into accounts.rate_multiplier.
//
// No other code path bounds that column from above — admins may type any
// non-negative number and the only ceiling is the DECIMAL(10,4) column itself
// (999999.9999). That ceiling is meaningless as a guard: rate_multiplier
// scales the per-request account cost that feeds quota_used, so a single
// declared 999999 would exhaust any account quota on the first request and
// poison cost reporting. 100 is picked as a deliberately generous bound: it is
// two orders of magnitude above the 1.0 default and far above any plausible
// upstream resale markup, so no legitimate declaration is rejected while an
// absurd or hostile one cannot reach the quota control plane unattended.
// It only constrains the automatic path; manual edits keep their old range.
const upstreamBillingRateSyncMaxMultiplier = 100.0

var (
	ErrUpstreamBillingProbeUnavailable = infraerrors.ServiceUnavailable(
		"UPSTREAM_BILLING_PROBE_UNAVAILABLE", "upstream billing probe is unavailable",
	)
	ErrUpstreamBillingProbeAccountInvalid = infraerrors.BadRequest(
		"UPSTREAM_BILLING_PROBE_ACCOUNT_INVALID", "account is not an API key account",
	)
	ErrUpstreamBillingProbeIdentityChanged = infraerrors.Conflict(
		"UPSTREAM_BILLING_PROBE_IDENTITY_CHANGED", "account identity changed during upstream billing probe; retry the probe",
	)
	ErrUpstreamBillingRateSyncBulkConflict = infraerrors.Conflict(
		"UPSTREAM_BILLING_RATE_SYNC_BULK_CONFLICT",
		"account rate multiplier cannot be changed in bulk while upstream billing rate sync is enabled",
	)
	ErrUpstreamBillingRateSyncConflict = infraerrors.Conflict(
		"UPSTREAM_BILLING_RATE_SYNC_CONFLICT",
		"account rate multiplier cannot be changed while upstream billing rate sync is enabled",
	)
)

const (
	UpstreamBillingProbeStatusOK          = "ok"
	UpstreamBillingProbeStatusUnsupported = "unsupported"
	UpstreamBillingProbeStatusFailed      = "failed"
)

// UpstreamBillingProbeSettings controls the periodic probe runner.
type UpstreamBillingProbeSettings struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"interval_minutes"`
}

// UpstreamBillingProbeSnapshot is persisted in accounts.extra. Data is kept as
// a sanitized map so future response fields do not require a database change.
type UpstreamBillingProbeSnapshot struct {
	Status        string         `json:"status"`
	Data          map[string]any `json:"data,omitempty"`
	ReceivedAt    *time.Time     `json:"received_at,omitempty"`
	FreshUntil    *time.Time     `json:"fresh_until,omitempty"`
	LastAttemptAt time.Time      `json:"last_attempt_at"`
	NextProbeAt   time.Time      `json:"next_probe_at"`
	FailureCount  int            `json:"failure_count,omitempty"`
	HTTPStatus    int            `json:"http_status,omitempty"`
	LastError     string         `json:"last_error,omitempty"`
	// SyncedRateMultiplier records the value this probe wrote into
	// accounts.rate_multiplier. It is only set when the account opted into rate
	// sync and the declared value passed the write-back range check, so the
	// stored snapshot always answers "did this probe move the account rate, and
	// to what" without a separate history table.
	SyncedRateMultiplier *float64 `json:"synced_rate_multiplier,omitempty"`
}

// UpstreamBillingProbeResult is returned by manual probe endpoints.
type UpstreamBillingProbeResult struct {
	AccountID int64                         `json:"account_id"`
	Snapshot  *UpstreamBillingProbeSnapshot `json:"snapshot,omitempty"`
	Error     string                        `json:"error,omitempty"`
}

type upstreamBillingProbeResponse struct {
	Object                  string   `json:"object"`
	SchemaVersion           int      `json:"schema_version"`
	BillingScope            string   `json:"billing_scope"`
	GroupRateMultiplier     *float64 `json:"group_rate_multiplier"`
	UserRateMultiplier      *float64 `json:"user_rate_multiplier"`
	ResolvedRateMultiplier  *float64 `json:"resolved_rate_multiplier"`
	PeakRateEnabled         *bool    `json:"peak_rate_enabled"`
	PeakStart               *string  `json:"peak_start"`
	PeakEnd                 *string  `json:"peak_end"`
	PeakRateMultiplier      *float64 `json:"peak_rate_multiplier"`
	AppliedPeakMultiplier   *float64 `json:"applied_peak_multiplier"`
	EffectiveRateMultiplier *float64 `json:"effective_rate_multiplier"`
	Timezone                *string  `json:"timezone"`
	ObservedAt              string   `json:"observed_at"`
}

type newAPIPricingResponse struct {
	Success    bool                       `json:"success"`
	GroupRatio map[string]json.RawMessage `json:"group_ratio"`
}

type upstreamBillingProbeHTTPResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

type upstreamBillingProbeRequestOptions struct {
	authorizationHeader         string
	applyAccountHeaderOverrides bool
}

var (
	errNewAPIPricingGroupRequired = errors.New("new api pricing requires an explicitly configured group")
	errNewAPIPricingGroupMissing  = errors.New("configured new api pricing group is missing")
)

// GetUpstreamBillingProbeSettings returns defaults when the setting is absent.
func (s *SettingService) GetUpstreamBillingProbeSettings(ctx context.Context) (*UpstreamBillingProbeSettings, error) {
	defaults := defaultUpstreamBillingProbeSettings()
	if s == nil || s.settingRepo == nil {
		return defaults, nil
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyUpstreamBillingProbeSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return defaults, nil
		}
		return nil, fmt.Errorf("get upstream billing probe settings: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return defaults, nil
	}
	settings := *defaults
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return nil, fmt.Errorf("parse upstream billing probe settings: %w", err)
	}
	if settings.IntervalMinutes == 0 {
		settings.IntervalMinutes = defaults.IntervalMinutes
	}
	normalizeUpstreamBillingProbeSettings(&settings)
	return &settings, nil
}

// SetUpstreamBillingProbeSettings validates and persists the runner settings.
func (s *SettingService) SetUpstreamBillingProbeSettings(ctx context.Context, settings *UpstreamBillingProbeSettings) error {
	if s == nil || s.settingRepo == nil {
		return fmt.Errorf("setting repository is unavailable")
	}
	if settings == nil {
		return infraerrors.BadRequest("INVALID_UPSTREAM_BILLING_PROBE_SETTINGS", "settings cannot be nil")
	}
	if settings.IntervalMinutes < upstreamBillingProbeMinIntervalMinutes || settings.IntervalMinutes > upstreamBillingProbeMaxIntervalMinutes {
		return infraerrors.BadRequest(
			"INVALID_UPSTREAM_BILLING_PROBE_INTERVAL",
			fmt.Sprintf("interval_minutes must be between %d and %d", upstreamBillingProbeMinIntervalMinutes, upstreamBillingProbeMaxIntervalMinutes),
		)
	}
	normalizeUpstreamBillingProbeSettings(settings)
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal upstream billing probe settings: %w", err)
	}
	return s.settingRepo.Set(ctx, SettingKeyUpstreamBillingProbeSettings, string(data))
}

func defaultUpstreamBillingProbeSettings() *UpstreamBillingProbeSettings {
	return &UpstreamBillingProbeSettings{Enabled: true, IntervalMinutes: upstreamBillingProbeDefaultIntervalMinutes}
}

func normalizeUpstreamBillingProbeSettings(settings *UpstreamBillingProbeSettings) {
	if settings.IntervalMinutes < upstreamBillingProbeMinIntervalMinutes {
		settings.IntervalMinutes = upstreamBillingProbeMinIntervalMinutes
	}
	if settings.IntervalMinutes > upstreamBillingProbeMaxIntervalMinutes {
		settings.IntervalMinutes = upstreamBillingProbeMaxIntervalMinutes
	}
}

// UpstreamBillingProbeService discovers a remote Sub2API billing snapshot.
type UpstreamBillingProbeService struct {
	accountRepo        AccountRepository
	accountTestService *AccountTestService
	settingService     *SettingService

	parentCtx    context.Context
	parentCancel context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
	started      bool
	stopped      bool
	cycleMu      sync.Mutex
	probeGroup   singleflight.Group
	probeSlots   chan struct{}
	now          func() time.Time
	lockCache    LeaderLockCache
	db           *sql.DB
	instanceID   string
}

type upstreamBillingProbeSnapshotWriter interface {
	UpdateUpstreamBillingProbeSnapshot(context.Context, *Account, *UpstreamBillingProbeSnapshot, *float64) error
}

type upstreamBillingProbeDueAccountLister interface {
	ListDueUpstreamBillingProbeAccounts(context.Context, time.Time, int) ([]Account, error)
}

func NewUpstreamBillingProbeService(
	accountRepo AccountRepository,
	accountTestService *AccountTestService,
	settingService *SettingService,
) *UpstreamBillingProbeService {
	ctx, cancel := context.WithCancel(context.Background())
	return &UpstreamBillingProbeService{
		accountRepo:        accountRepo,
		accountTestService: accountTestService,
		settingService:     settingService,
		parentCtx:          ctx,
		parentCancel:       cancel,
		probeSlots:         make(chan struct{}, upstreamBillingProbeConcurrency),
		now:                time.Now,
		instanceID:         uuid.NewString(),
	}
}

func (s *UpstreamBillingProbeService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

// ProvideUpstreamBillingProbeService starts the process-wide periodic runner.
func ProvideUpstreamBillingProbeService(
	accountRepo AccountRepository,
	accountTestService *AccountTestService,
	settingService *SettingService,
	lockCache LeaderLockCache,
	db *sql.DB,
) *UpstreamBillingProbeService {
	svc := NewUpstreamBillingProbeService(accountRepo, accountTestService, settingService)
	svc.SetLeaderLock(lockCache, db)
	svc.Start()
	return svc
}

func (s *UpstreamBillingProbeService) Start() {
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

func (s *UpstreamBillingProbeService) Stop() {
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

func (s *UpstreamBillingProbeService) runLoop() {
	defer s.wg.Done()
	_ = s.RunDue(s.parentCtx)
	ticker := time.NewTicker(upstreamBillingProbeCycleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case <-ticker.C:
			if err := s.RunDue(s.parentCtx); err != nil {
				logger.LegacyPrintf("service.upstream_billing_probe", "run_due_failed: err=%v", err)
			}
		}
	}
}

// RunDue executes at most one bounded batch of due accounts.
func (s *UpstreamBillingProbeService) RunDue(ctx context.Context) error {
	if s == nil || s.accountRepo == nil {
		return nil
	}
	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()

	settings, err := s.getSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return nil
	}
	runRelease, acquired, lockErr := s.tryAcquireLeaderLock(ctx, upstreamBillingProbeLeaderLockKey)
	if lockErr != nil {
		return fmt.Errorf("acquire upstream billing probe leader lock: %w", lockErr)
	}
	if !acquired {
		return nil
	}
	defer runRelease()

	lockNow := time.Now()
	cadenceRelease, acquired, lockErr := s.tryAcquireLeaderLock(ctx, upstreamBillingProbeLeaderLockKeyAt(lockNow))
	if lockErr != nil {
		return fmt.Errorf("acquire upstream billing probe cadence lock: %w", lockErr)
	}
	if !acquired {
		return nil
	}
	defer releaseUpstreamBillingProbeLeaderLock(cadenceRelease, lockNow.Truncate(upstreamBillingProbeCycleInterval).Add(upstreamBillingProbeCycleInterval))

	now := s.currentTime()
	accounts, err := s.listDueAccounts(ctx, now)
	if err != nil {
		return fmt.Errorf("list enabled upstream billing probes: %w", err)
	}
	due := make([]Account, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		if !isUpstreamBillingProbeAccount(&account) || !account.IsActive() || !upstreamBillingProbeEnabled(&account) {
			continue
		}
		snapshot := decodeUpstreamBillingProbeSnapshot(account.Extra)
		if snapshot != nil && !snapshot.NextProbeAt.IsZero() && now.Before(snapshot.NextProbeAt) {
			continue
		}
		due = append(due, account)
	}
	sort.SliceStable(due, func(i, j int) bool {
		left := decodeUpstreamBillingProbeSnapshot(due[i].Extra)
		right := decodeUpstreamBillingProbeSnapshot(due[j].Extra)
		leftUnset := left == nil || left.NextProbeAt.IsZero()
		rightUnset := right == nil || right.NextProbeAt.IsZero()
		if leftUnset && rightUnset {
			return due[i].ID < due[j].ID
		}
		if leftUnset {
			return true
		}
		if rightUnset {
			return false
		}
		return left.NextProbeAt.Before(right.NextProbeAt)
	})
	if len(due) > upstreamBillingProbeMaxPerCycle {
		due = due[:upstreamBillingProbeMaxPerCycle]
	}

	var group errgroup.Group
	for i := range due {
		accountID := due[i].ID
		group.Go(func() error {
			if _, probeErr := s.probeScheduledAccount(ctx, accountID, settings.IntervalMinutes); probeErr != nil {
				logger.LegacyPrintf("service.upstream_billing_probe", "probe_due_failed: account_id=%d err=%v", accountID, probeErr)
			}
			return nil
		})
	}
	return group.Wait()
}

func (s *UpstreamBillingProbeService) listDueAccounts(ctx context.Context, now time.Time) ([]Account, error) {
	if lister, ok := s.accountRepo.(upstreamBillingProbeDueAccountLister); ok {
		return lister.ListDueUpstreamBillingProbeAccounts(ctx, now, upstreamBillingProbeMaxPerCycle)
	}
	// Non-production repositories and older adapters keep the generic path. The
	// runner still truncates before issuing network requests.
	return s.accountRepo.FindByExtraField(ctx, UpstreamBillingProbeEnabledExtraKey, true)
}

func (s *UpstreamBillingProbeService) getSettings(ctx context.Context) (*UpstreamBillingProbeSettings, error) {
	if s.settingService == nil {
		return defaultUpstreamBillingProbeSettings(), nil
	}
	return s.settingService.GetUpstreamBillingProbeSettings(ctx)
}

func (s *UpstreamBillingProbeService) GetSettings(ctx context.Context) (*UpstreamBillingProbeSettings, error) {
	return s.getSettings(ctx)
}

func (s *UpstreamBillingProbeService) UpdateSettings(ctx context.Context, settings *UpstreamBillingProbeSettings) error {
	if s == nil || s.settingService == nil {
		return ErrUpstreamBillingProbeUnavailable
	}
	return s.settingService.SetUpstreamBillingProbeSettings(ctx, settings)
}

// ProbeAccount performs one manual or scheduled probe. Manual calls ignore both switches.
func (s *UpstreamBillingProbeService) ProbeAccount(ctx context.Context, accountID int64) (*UpstreamBillingProbeSnapshot, error) {
	if s == nil || s.accountRepo == nil {
		return nil, ErrUpstreamBillingProbeUnavailable
	}
	settings, err := s.getSettings(ctx)
	if err != nil {
		return nil, err
	}
	return s.probeAccount(ctx, accountID, settings.IntervalMinutes)
}

func (s *UpstreamBillingProbeService) probeAccount(ctx context.Context, accountID int64, intervalMinutes int) (*UpstreamBillingProbeSnapshot, error) {
	return s.probeAccountWithMode(ctx, accountID, intervalMinutes, false)
}

func (s *UpstreamBillingProbeService) probeScheduledAccount(ctx context.Context, accountID int64, intervalMinutes int) (*UpstreamBillingProbeSnapshot, error) {
	return s.probeAccountWithMode(ctx, accountID, intervalMinutes, true)
}

func (s *UpstreamBillingProbeService) probeAccountWithMode(ctx context.Context, accountID int64, intervalMinutes int, requireEnabled bool) (*UpstreamBillingProbeSnapshot, error) {
	key := strconv.FormatInt(accountID, 10)
	value, err, _ := s.probeGroup.Do(key, func() (any, error) {
		select {
		case s.probeSlots <- struct{}{}:
			defer func() { <-s.probeSlots }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		account, loadErr := s.accountRepo.GetByID(ctx, accountID)
		if loadErr != nil {
			return nil, loadErr
		}
		if !isUpstreamBillingProbeAccount(account) {
			return nil, ErrUpstreamBillingProbeAccountInvalid
		}
		if requireEnabled {
			if !account.IsActive() || !upstreamBillingProbeEnabled(account) {
				return nil, nil
			}
			if snapshot := decodeUpstreamBillingProbeSnapshot(account.Extra); snapshot != nil &&
				!snapshot.NextProbeAt.IsZero() && s.currentTime().Before(snapshot.NextProbeAt) {
				return nil, nil
			}
		}
		return s.probeLoadedAccount(ctx, account, intervalMinutes)
	})
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}
	snapshot, ok := value.(*UpstreamBillingProbeSnapshot)
	if !ok {
		return nil, fmt.Errorf("invalid upstream billing probe result")
	}
	return snapshot, nil
}

// ProbeAccounts performs a bounded manual batch with the same concurrency limit as the runner.
func (s *UpstreamBillingProbeService) ProbeAccounts(ctx context.Context, accountIDs []int64) []UpstreamBillingProbeResult {
	if len(accountIDs) > upstreamBillingProbeMaxPerCycle {
		accountIDs = accountIDs[:upstreamBillingProbeMaxPerCycle]
	}
	results := make([]UpstreamBillingProbeResult, len(accountIDs))
	if s == nil || s.accountRepo == nil {
		for i, accountID := range accountIDs {
			results[i] = UpstreamBillingProbeResult{AccountID: accountID, Error: ErrUpstreamBillingProbeUnavailable.Error()}
		}
		return results
	}
	settings, settingsErr := s.getSettings(ctx)
	if settingsErr != nil {
		for i, accountID := range accountIDs {
			results[i] = UpstreamBillingProbeResult{AccountID: accountID, Error: safeProbeError(settingsErr)}
		}
		return results
	}
	var group errgroup.Group
	for i, accountID := range accountIDs {
		i, accountID := i, accountID
		results[i].AccountID = accountID
		group.Go(func() error {
			snapshot, err := s.probeAccount(ctx, accountID, settings.IntervalMinutes)
			if err != nil {
				results[i].Error = safeProbeError(err)
				return nil
			}
			results[i].Snapshot = snapshot
			return nil
		})
	}
	_ = group.Wait()
	return results
}

func upstreamBillingProbeLeaderLockKeyAt(now time.Time) string {
	return fmt.Sprintf("%s:%d", upstreamBillingProbeLeaderLockKey, now.Unix()/int64(upstreamBillingProbeCycleInterval/time.Second))
}

func (s *UpstreamBillingProbeService) tryAcquireLeaderLock(ctx context.Context, key string) (func(), bool, error) {
	lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if s.lockCache != nil {
		acquired, err := s.lockCache.TryAcquireLeaderLock(lockCtx, key, s.instanceID, upstreamBillingProbeLeaderLockTTL)
		if err != nil {
			return nil, false, err
		}
		if !acquired {
			return nil, false, nil
		}
		return func() {
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer releaseCancel()
			_ = s.lockCache.ReleaseLeaderLock(releaseCtx, key, s.instanceID)
		}, true, nil
	}
	if s.db != nil {
		return tryAcquireDBAdvisoryLockWithError(lockCtx, s.db, hashAdvisoryLockID(key))
	}
	return func() {}, true, nil
}

func releaseUpstreamBillingProbeLeaderLock(release func(), releaseAt time.Time) {
	delay := time.Until(releaseAt)
	if delay <= 0 {
		release()
		return
	}
	time.AfterFunc(delay, release)
}

func (s *UpstreamBillingProbeService) SetAccountEnabled(ctx context.Context, accountID int64, enabled bool) error {
	if s == nil || s.accountRepo == nil {
		return ErrUpstreamBillingProbeUnavailable
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if !isUpstreamBillingProbeAccount(account) {
		return ErrUpstreamBillingProbeAccountInvalid
	}
	updates := map[string]any{UpstreamBillingProbeEnabledExtraKey: enabled}
	if !enabled {
		updates[UpstreamBillingRateSyncEnabledExtraKey] = false
	}
	return s.accountRepo.UpdateExtra(ctx, accountID, updates)
}

func (s *UpstreamBillingProbeService) probeLoadedAccount(ctx context.Context, account *Account, intervalMinutes int) (*UpstreamBillingProbeSnapshot, error) {
	now := s.currentTime().UTC()
	if s.accountTestService == nil || s.accountTestService.httpUpstream == nil {
		return s.persistProbeFailure(ctx, account, intervalMinutes, now, 0, "transport_unavailable", 0)
	}
	rechargeMultiplier, validRechargeMultiplier := upstreamBillingRechargeMultiplier(account)
	if !validRechargeMultiplier {
		return s.persistProbeFailure(ctx, account, intervalMinutes, now, 0, "invalid_recharge_multiplier", 0)
	}
	// 平台放宽后取数直读 credentials：所有 API-key 平台的密钥与自定义上游
	// 统一存放在 credentials.api_key / credentials.base_url。
	apiKey := account.GetCredential("api_key")
	if apiKey == "" {
		return s.persistProbeFailure(ctx, account, intervalMinutes, now, 0, "missing_api_key", 0)
	}
	baseURL := account.GetCredential("base_url")
	if account.Platform == PlatformOpenAI {
		if baseURL == "" {
			// 保持官方语义：OpenAI 账号无自定义 base 时探官方域（404 → unsupported）。
			baseURL = "https://api.openai.com"
		}
	} else if upstreamBillingProbeTargetIsOfficialAPI(baseURL) {
		// 其他平台 base_url 为空或指向官方 API 根域（前端创建时会把空值
		// 填成官方默认域，且提供 us-east-1.api.x.ai 等官方区域预设）⇒
		// 必无 /v1/sub2api/billing；不发请求，直接记 unsupported，避免
		// 拿账号 Key 周期性请求官方域的不存在路径。
		return s.persistProbeFailure(ctx, account, intervalMinutes, now, 0, "unsupported", 0)
	}
	normalizedBaseURL, err := s.accountTestService.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return s.persistProbeFailure(ctx, account, intervalMinutes, now, 0, "invalid_base_url", 0)
	}
	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy == nil {
			return s.persistProbeFailure(ctx, account, intervalMinutes, now, 0, "proxy_unavailable", 0)
		}
		if account.Proxy.ID != *account.ProxyID {
			return nil, ErrUpstreamBillingProbeIdentityChanged
		}
		proxyURL = account.Proxy.URL()
	}
	probeCtx, cancel := context.WithTimeout(ctx, upstreamBillingProbeRequestTimeout)
	defer cancel()
	var tlsProfile *tlsfingerprint.Profile
	if s.accountTestService.tlsFPProfileService != nil {
		tlsProfile = s.accountTestService.tlsFPProfileService.ResolveTLSProfile(account)
	}

	result, failureReason := s.requestUpstreamBillingProbe(
		probeCtx,
		account,
		buildOpenAIEndpointURL(normalizedBaseURL, "/v1/sub2api/billing"),
		proxyURL,
		tlsProfile,
		upstreamBillingProbeMaxBodyBytes,
		upstreamBillingProbeRequestOptions{
			authorizationHeader:         "Bearer " + apiKey,
			applyAccountHeaderOverrides: true,
		},
	)
	if failureReason != "" && !upstreamBillingCompatibilityFallbackStatus(result) {
		return s.persistUpstreamBillingRequestFailure(ctx, account, intervalMinutes, now, result, failureReason)
	}

	var data map[string]any
	if upstreamBillingCompatibilityFallbackStatus(result) {
		newAPIGroup, validGroup := upstreamBillingNewAPIGroup(account)
		if !validGroup {
			return s.persistProbeFailure(ctx, account, intervalMinutes, now, result.StatusCode, "invalid_newapi_group", retryAfter(result.Header, now))
		}
		if newAPIGroup == "" {
			// /api/pricing is a dashboard catalogue, not a relay-key introspection
			// endpoint. Without an explicitly confirmed group there is no safe value
			// to read or write back, so do not make the second request at all.
			return s.persistProbeFailure(ctx, account, intervalMinutes, now, result.StatusCode, "newapi_group_required", retryAfter(result.Header, now))
		}
		result, failureReason = s.requestUpstreamBillingProbe(
			probeCtx,
			account,
			buildUpstreamSiteEndpointURL(normalizedBaseURL, "/api/pricing"),
			proxyURL,
			tlsProfile,
			upstreamBillingProbeMaxNewAPIBodyBytes,
			// A relay API key is not a New API dashboard session/PAT. Keep the
			// compatibility request anonymous and never carry account-specific
			// headers into the dashboard route.
			upstreamBillingProbeRequestOptions{},
		)
		if upstreamBillingCompatibilityFallbackStatus(result) {
			return s.persistProbeFailure(ctx, account, intervalMinutes, now, result.StatusCode, "unsupported", retryAfter(result.Header, now))
		}
		if failureReason != "" {
			return s.persistUpstreamBillingRequestFailure(ctx, account, intervalMinutes, now, result, failureReason)
		}
		if newAPIPricingAuthRequired(result) {
			return s.persistProbeFailure(ctx, account, intervalMinutes, now, result.StatusCode, "newapi_pricing_auth_required", retryAfter(result.Header, now))
		}
		if result.StatusCode < 200 || result.StatusCode >= 300 {
			return s.persistProbeFailure(ctx, account, intervalMinutes, now, result.StatusCode, "http_error", retryAfter(result.Header, now))
		}
		data, err = parseNewAPIUpstreamBillingResponse(result.Body, newAPIGroup, now)
		if err != nil {
			reason := "invalid_response"
			switch {
			case errors.Is(err, errNewAPIPricingGroupRequired):
				reason = "newapi_group_required"
			case errors.Is(err, errNewAPIPricingGroupMissing):
				reason = "newapi_group_not_found"
			}
			return s.persistProbeFailure(ctx, account, intervalMinutes, now, result.StatusCode, reason, retryAfter(result.Header, now))
		}
	} else {
		if result.StatusCode < 200 || result.StatusCode >= 300 {
			return s.persistProbeFailure(ctx, account, intervalMinutes, now, result.StatusCode, "http_error", retryAfter(result.Header, now))
		}
		data, err = parseUpstreamBillingProbeResponse(result.Body)
		if err != nil {
			return s.persistProbeFailure(ctx, account, intervalMinutes, now, result.StatusCode, "invalid_response", retryAfter(result.Header, now))
		}
	}
	if err := applyUpstreamBillingRechargeMultiplier(data, rechargeMultiplier); err != nil {
		return s.persistProbeFailure(ctx, account, intervalMinutes, now, result.StatusCode, "invalid_recharge_multiplier", retryAfter(result.Header, now))
	}
	snapshot := &UpstreamBillingProbeSnapshot{
		Status:        UpstreamBillingProbeStatusOK,
		Data:          data,
		ReceivedAt:    probeTimePtr(now),
		FreshUntil:    probeTimePtr(now.Add(2 * time.Duration(intervalMinutes) * time.Minute)),
		LastAttemptAt: now,
		NextProbeAt:   now.Add(nextProbeDelay(intervalMinutes, 0)),
		HTTPStatus:    result.StatusCode,
	}
	// 账号级值域与精度只在真要写回时才有影响：只观察上游声明、未开启同步的
	// 账号不因声明值不适配 accounts.rate_multiplier 而被记成探测失败并进入
	// 指数退避——探测本身成功了，原始声明照常存进快照供展示。
	var syncRate *float64
	previousRate := account.BillingRateMultiplier()
	if upstreamBillingRateSyncEnabled(account) {
		if value, valid := upstreamBillingProbeSyncRateForAccount(account, data); valid {
			syncRate = &value
			snapshot.SyncedRateMultiplier = &value
		} else {
			declared, _ := resolveAccountExtraNumber(data, "resolved_rate_multiplier")
			slog.Warn("upstream_billing_rate_sync_rejected",
				"source", "upstream_billing_probe",
				"account_id", account.ID,
				"declared_resolved_rate_multiplier", declared,
				"max_rate_multiplier", upstreamBillingRateSyncMaxMultiplier,
				"current_rate_multiplier", previousRate,
			)
		}
	}
	if err := s.updateSnapshot(ctx, account, snapshot, syncRate); err != nil {
		return nil, err
	}
	if syncRate != nil {
		// 写回是后台任务的裸 SQL，不经过管理端路由，因此不会产生 audit_logs 行。
		// old_rate_multiplier 是本次探测开始时读到的值（写回的 CAS 不比对该列）。
		slog.Info("upstream_billing_rate_sync_applied",
			"source", "upstream_billing_probe",
			"account_id", account.ID,
			"old_rate_multiplier", previousRate,
			"new_rate_multiplier", *syncRate,
		)
	}
	return snapshot, nil
}

func upstreamBillingCompatibilityFallbackStatus(result *upstreamBillingProbeHTTPResult) bool {
	return result != nil && (result.StatusCode == http.StatusNotFound || result.StatusCode == http.StatusMethodNotAllowed)
}

func newAPIPricingAuthRequired(result *upstreamBillingProbeHTTPResult) bool {
	if result == nil {
		return false
	}
	if result.StatusCode == http.StatusUnauthorized || result.StatusCode == http.StatusForbidden {
		return true
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return false
	}

	var response struct {
		Success *bool           `json:"success"`
		Message json.RawMessage `json:"message"`
		Error   json.RawMessage `json:"error"`
		Code    json.RawMessage `json:"code"`
	}
	if err := json.Unmarshal(result.Body, &response); err != nil || response.Success == nil || *response.Success {
		return false
	}
	authText := strings.ToLower(string(response.Message) + " " + string(response.Error) + " " + string(response.Code))
	for _, marker := range []string{
		"unauthorized",
		"invalid access token",
		"authentication",
		"access token",
		"login required",
		"not logged in",
	} {
		if strings.Contains(authText, marker) {
			return true
		}
	}
	return false
}

func (s *UpstreamBillingProbeService) requestUpstreamBillingProbe(
	ctx context.Context,
	account *Account,
	endpointURL string,
	proxyURL string,
	tlsProfile *tlsfingerprint.Profile,
	maxBodyBytes int,
	options upstreamBillingProbeRequestOptions,
) (*upstreamBillingProbeHTTPResult, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL, bytes.NewReader(nil))
	if err != nil {
		return nil, "request_build_failed"
	}
	profile := HTTPUpstreamProfileDefault
	if account.Platform == PlatformOpenAI {
		profile = HTTPUpstreamProfileOpenAI
	}
	reqCtx := WithHTTPUpstreamProfile(req.Context(), profile)
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(reqCtx))
	req.Header.Set("Accept", "application/json")
	if options.authorizationHeader != "" {
		req.Header.Set("Authorization", options.authorizationHeader)
	}
	if options.applyAccountHeaderOverrides {
		account.ApplyHeaderOverrides(req.Header)
	}

	resp, err := s.accountTestService.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
	if err != nil {
		return nil, "request_failed"
	}
	if resp == nil || resp.Body == nil {
		return nil, "empty_response"
	}
	defer func() { _ = resp.Body.Close() }()
	result := &upstreamBillingProbeHTTPResult{StatusCode: resp.StatusCode, Header: resp.Header}
	result.Body, err = io.ReadAll(io.LimitReader(resp.Body, int64(maxBodyBytes)+1))
	if err != nil {
		return result, "response_read_failed"
	}
	if len(result.Body) > maxBodyBytes {
		return result, "response_too_large"
	}
	return result, ""
}

func (s *UpstreamBillingProbeService) persistUpstreamBillingRequestFailure(
	ctx context.Context,
	account *Account,
	intervalMinutes int,
	now time.Time,
	result *upstreamBillingProbeHTTPResult,
	reason string,
) (*UpstreamBillingProbeSnapshot, error) {
	if result == nil {
		return s.persistProbeFailure(ctx, account, intervalMinutes, now, 0, reason, 0)
	}
	return s.persistProbeFailure(ctx, account, intervalMinutes, now, result.StatusCode, reason, retryAfter(result.Header, now))
}

func (s *UpstreamBillingProbeService) persistProbeFailure(
	ctx context.Context,
	account *Account,
	intervalMinutes int,
	now time.Time,
	statusCode int,
	reason string,
	retryAfterDuration time.Duration,
) (*UpstreamBillingProbeSnapshot, error) {
	previous := decodeUpstreamBillingProbeSnapshot(account.Extra)
	failureCount := 1
	if previous != nil {
		failureCount = previous.FailureCount + 1
	}
	status := UpstreamBillingProbeStatusFailed
	delay := nextProbeDelay(intervalMinutes, retryAfterDuration)
	if upstreamBillingProbeReasonIsUnsupported(reason) {
		status = UpstreamBillingProbeStatusUnsupported
		delay = unsupportedProbeDelay(intervalMinutes, retryAfterDuration)
	}
	snapshot := &UpstreamBillingProbeSnapshot{
		Status:        status,
		LastAttemptAt: now,
		NextProbeAt:   now.Add(delay),
		FailureCount:  failureCount,
		HTTPStatus:    statusCode,
		LastError:     reason,
	}
	if previous != nil {
		snapshot.Data = previous.Data
		snapshot.ReceivedAt = previous.ReceivedAt
		snapshot.FreshUntil = previous.FreshUntil
		if snapshot.FreshUntil == nil && previous.Status == UpstreamBillingProbeStatusOK && previous.ReceivedAt != nil {
			snapshot.FreshUntil = probeTimePtr(previous.ReceivedAt.Add(2 * time.Duration(intervalMinutes) * time.Minute))
		}
	}
	if err := s.updateSnapshot(ctx, account, snapshot, nil); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func upstreamBillingProbeReasonIsUnsupported(reason string) bool {
	return reason == "unsupported" || reason == "newapi_pricing_auth_required"
}

func (s *UpstreamBillingProbeService) updateSnapshot(
	ctx context.Context,
	account *Account,
	snapshot *UpstreamBillingProbeSnapshot,
	rateMultiplier *float64,
) error {
	writer, ok := s.accountRepo.(upstreamBillingProbeSnapshotWriter)
	if !ok {
		return ErrUpstreamBillingProbeUnavailable
	}
	return writer.UpdateUpstreamBillingProbeSnapshot(ctx, account, snapshot, rateMultiplier)
}

func parseUpstreamBillingProbeResponse(body []byte) (map[string]any, error) {
	var response upstreamBillingProbeResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if response.Object != "sub2api.key_billing" || response.SchemaVersion != 1 || response.BillingScope != "token" {
		return nil, fmt.Errorf("unexpected billing response schema")
	}
	if response.GroupRateMultiplier == nil || response.ResolvedRateMultiplier == nil ||
		response.PeakRateEnabled == nil || response.EffectiveRateMultiplier == nil {
		return nil, fmt.Errorf("incomplete billing response")
	}
	for _, value := range []float64{
		*response.GroupRateMultiplier,
		*response.ResolvedRateMultiplier,
		*response.EffectiveRateMultiplier,
	} {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("invalid billing multiplier")
		}
	}
	if response.UserRateMultiplier != nil && (*response.UserRateMultiplier < 0 || math.IsNaN(*response.UserRateMultiplier) || math.IsInf(*response.UserRateMultiplier, 0)) {
		return nil, fmt.Errorf("invalid user billing multiplier")
	}
	expectedResolved := *response.GroupRateMultiplier
	if response.UserRateMultiplier != nil {
		expectedResolved = *response.UserRateMultiplier
	}
	if !equalBillingMultiplier(*response.ResolvedRateMultiplier, expectedResolved) {
		return nil, fmt.Errorf("inconsistent resolved billing multiplier")
	}
	observedAt, err := time.Parse(time.RFC3339Nano, response.ObservedAt)
	if err != nil || observedAt.IsZero() {
		return nil, fmt.Errorf("invalid observed_at")
	}
	data := map[string]any{
		"object":                    response.Object,
		"provider":                  "sub2api",
		"schema_version":            response.SchemaVersion,
		"billing_scope":             response.BillingScope,
		"group_rate_multiplier":     *response.GroupRateMultiplier,
		"resolved_rate_multiplier":  *response.ResolvedRateMultiplier,
		"peak_rate_enabled":         *response.PeakRateEnabled,
		"effective_rate_multiplier": *response.EffectiveRateMultiplier,
		"observed_at":               observedAt.UTC().Format(time.RFC3339Nano),
	}
	if response.UserRateMultiplier != nil {
		data["user_rate_multiplier"] = *response.UserRateMultiplier
	}
	if *response.PeakRateEnabled {
		if response.PeakStart == nil || response.PeakEnd == nil || response.Timezone == nil ||
			response.PeakRateMultiplier == nil || response.AppliedPeakMultiplier == nil ||
			*response.PeakStart == "" || *response.PeakEnd == "" || *response.Timezone == "" ||
			*response.PeakRateMultiplier < 0 || *response.AppliedPeakMultiplier < 0 ||
			math.IsNaN(*response.PeakRateMultiplier) || math.IsInf(*response.PeakRateMultiplier, 0) ||
			math.IsNaN(*response.AppliedPeakMultiplier) || math.IsInf(*response.AppliedPeakMultiplier, 0) {
			return nil, fmt.Errorf("incomplete peak billing response")
		}
		data["peak_start"] = *response.PeakStart
		data["peak_end"] = *response.PeakEnd
		data["peak_rate_multiplier"] = *response.PeakRateMultiplier
		data["applied_peak_multiplier"] = *response.AppliedPeakMultiplier
		data["timezone"] = *response.Timezone
	}
	appliedPeak, ok := upstreamBillingPeakMultiplierAt(data, observedAt)
	if !ok {
		return nil, fmt.Errorf("invalid peak billing response")
	}
	if response.PeakRateEnabled != nil && *response.PeakRateEnabled {
		if !equalBillingMultiplier(*response.AppliedPeakMultiplier, appliedPeak) {
			return nil, fmt.Errorf("inconsistent applied peak multiplier")
		}
	} else if response.AppliedPeakMultiplier != nil && !equalBillingMultiplier(*response.AppliedPeakMultiplier, 1) {
		return nil, fmt.Errorf("inconsistent applied peak multiplier")
	}
	if !equalBillingMultiplier(*response.EffectiveRateMultiplier, *response.ResolvedRateMultiplier*appliedPeak) {
		return nil, fmt.Errorf("inconsistent effective billing multiplier")
	}
	return data, nil
}

func parseNewAPIUpstreamBillingResponse(body []byte, configuredGroup string, observedAt time.Time) (map[string]any, error) {
	var response newAPIPricingResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if !response.Success || len(response.GroupRatio) == 0 {
		return nil, fmt.Errorf("unexpected new api pricing response schema")
	}
	group := strings.TrimSpace(configuredGroup)
	if group == "" {
		return nil, errNewAPIPricingGroupRequired
	}
	rawRatio, ok := response.GroupRatio[group]
	if !ok {
		return nil, errNewAPIPricingGroupMissing
	}
	ratio, err := parseStrictNewAPIGroupRatio(rawRatio)
	if err != nil {
		return nil, fmt.Errorf("invalid new api group multiplier for %q: %w", group, err)
	}
	if ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return nil, fmt.Errorf("invalid new api group multiplier")
	}
	return map[string]any{
		"object":                    "newapi.group_ratio",
		"provider":                  "newapi",
		"schema_version":            1,
		"billing_scope":             "token",
		"upstream_group":            group,
		"group_rate_multiplier":     ratio,
		"resolved_rate_multiplier":  ratio,
		"peak_rate_enabled":         false,
		"effective_rate_multiplier": ratio,
		"observed_at":               observedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

func parseStrictNewAPIGroupRatio(raw json.RawMessage) (float64, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, fmt.Errorf("ratio must be a JSON number")
	}
	var ratio float64
	if err := json.Unmarshal(trimmed, &ratio); err != nil {
		return 0, fmt.Errorf("ratio must be a JSON number: %w", err)
	}
	if ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return 0, fmt.Errorf("ratio must be finite and non-negative")
	}
	return ratio, nil
}

func applyUpstreamBillingRechargeMultiplier(data map[string]any, rechargeMultiplier float64) error {
	if rechargeMultiplier <= 0 || math.IsNaN(rechargeMultiplier) || math.IsInf(rechargeMultiplier, 0) {
		return fmt.Errorf("invalid recharge multiplier")
	}
	rawResolved, ok := resolveAccountExtraNumber(data, "resolved_rate_multiplier")
	if !ok {
		return fmt.Errorf("missing resolved billing multiplier")
	}
	rawEffective, ok := resolveAccountExtraNumber(data, "effective_rate_multiplier")
	if !ok {
		return fmt.Errorf("missing effective billing multiplier")
	}
	for _, key := range []string{
		"group_rate_multiplier",
		"user_rate_multiplier",
		"resolved_rate_multiplier",
		"effective_rate_multiplier",
	} {
		value, exists := resolveAccountExtraNumber(data, key)
		if !exists {
			if key == "user_rate_multiplier" {
				continue
			}
			return fmt.Errorf("missing %s", key)
		}
		corrected := value * rechargeMultiplier
		if corrected < 0 || math.IsNaN(corrected) || math.IsInf(corrected, 0) {
			return fmt.Errorf("corrected %s is not finite", key)
		}
		data[key] = corrected
	}
	data["raw_resolved_rate_multiplier"] = rawResolved
	data["raw_effective_rate_multiplier"] = rawEffective
	data["recharge_multiplier"] = rechargeMultiplier
	return nil
}

func upstreamBillingRateAt(data map[string]any, now time.Time) (float64, bool) {
	if scope, _ := data["billing_scope"].(string); scope != "token" {
		return 0, false
	}
	base, ok := resolveAccountExtraNumber(data, "resolved_rate_multiplier")
	if !ok || base < 0 || math.IsNaN(base) || math.IsInf(base, 0) {
		return 0, false
	}
	appliedPeak, ok := upstreamBillingPeakMultiplierAt(data, now)
	if !ok {
		return 0, false
	}
	base *= appliedPeak
	if math.IsNaN(base) || math.IsInf(base, 0) {
		return 0, false
	}
	return base, true
}

// upstreamBillingProbeSyncRate converts the declared multiplier into the value
// the automatic write-back may store in accounts.rate_multiplier, at the
// precision that column supports (DECIMAL(10,4)).
//
// It reads resolved_rate_multiplier, not effective_rate_multiplier: the
// effective value folds in the peak coefficient that happened to apply at the
// instant of the probe, so writing it would freeze one probe cycle's peak (or
// off-peak) factor into a static column, while display and scheduling
// recompute the peak factor for the current time through upstreamBillingRateAt.
//
// The accepted range is deliberately narrower than the column:
//   - 0 is rejected. accountCost multiplies the request cost by this value, so
//     an upstream-declared 0 would stop quota_used from ever growing and every
//     admin-configured account quota and cost alert would silently stop
//     working. Admins may still set 0 by hand; only the automatic path refuses.
//   - anything above upstreamBillingRateSyncMaxMultiplier is rejected.
//
// A rejected declaration leaves the current multiplier untouched; the probe
// still records an OK snapshot carrying the raw declaration for display.
func upstreamBillingProbeSyncRate(data map[string]any) (float64, bool) {
	value, ok := resolveAccountExtraNumber(data, "resolved_rate_multiplier")
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	rounded := math.Round(value*upstreamBillingProbeAccountRateScale) / upstreamBillingProbeAccountRateScale
	if rounded <= 0 || rounded > upstreamBillingRateSyncMaxMultiplier {
		return 0, false
	}
	return rounded, true
}

// New API pricing is a group catalogue, so automatic write-back additionally
// requires the observed catalogue entry to match the account's explicit group
// confirmation. The repository CAS separately rejects a concurrent group edit.
func upstreamBillingProbeSyncRateForAccount(account *Account, data map[string]any) (float64, bool) {
	provider, _ := data["provider"].(string)
	object, _ := data["object"].(string)
	if provider == "newapi" || object == "newapi.group_ratio" {
		configuredGroup, valid := upstreamBillingNewAPIGroup(account)
		observedGroup, observed := data["upstream_group"].(string)
		if !valid || configuredGroup == "" || !observed || observedGroup != configuredGroup {
			return 0, false
		}
	}
	return upstreamBillingProbeSyncRate(data)
}

func upstreamBillingPeakMultiplierAt(data map[string]any, now time.Time) (float64, bool) {
	peakEnabled, ok := data["peak_rate_enabled"].(bool)
	if !ok {
		return 0, false
	}
	if !peakEnabled {
		return 1, true
	}

	start, startOK := data["peak_start"].(string)
	end, endOK := data["peak_end"].(string)
	timezoneName, timezoneOK := data["timezone"].(string)
	peakMultiplier, multiplierOK := resolveAccountExtraNumber(data, "peak_rate_multiplier")
	startMinute, validStart := parseMinutes(start)
	endMinute, validEnd := parseMinutes(end)
	if !startOK || !endOK || !timezoneOK || !multiplierOK || !validStart || !validEnd ||
		startMinute >= endMinute || peakMultiplier < 0 || math.IsNaN(peakMultiplier) || math.IsInf(peakMultiplier, 0) {
		return 0, false
	}
	location, err := time.LoadLocation(timezoneName)
	if err != nil {
		return 0, false
	}

	local := now.In(location)
	minute := local.Hour()*60 + local.Minute()
	if minute >= startMinute && minute < endMinute {
		return peakMultiplier, true
	}
	return 1, true
}

func equalBillingMultiplier(left, right float64) bool {
	if math.IsNaN(left) || math.IsNaN(right) || math.IsInf(left, 0) || math.IsInf(right, 0) {
		return false
	}
	scale := math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
	return math.Abs(left-right) <= 1e-9*scale
}

func validateUpstreamBillingRechargeMultiplier(value float64) error {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return infraerrors.BadRequest(
			"INVALID_UPSTREAM_BILLING_RECHARGE_MULTIPLIER",
			"upstream_billing_recharge_multiplier must be a finite number greater than 0",
		)
	}
	return nil
}

func normalizeUpstreamBillingNewAPIGroup(value string) (string, error) {
	// Validate before trimming so a value made entirely of control characters
	// cannot be silently reinterpreted as the empty (clear) value.
	for _, char := range value {
		if unicode.IsControl(char) {
			return "", infraerrors.BadRequest(
				"INVALID_UPSTREAM_BILLING_NEWAPI_GROUP",
				"upstream_billing_newapi_group cannot contain control characters",
			)
		}
	}
	normalized := strings.TrimSpace(value)
	if utf8.RuneCountInString(normalized) > upstreamBillingNewAPIGroupMaxRunes {
		return "", infraerrors.BadRequest(
			"INVALID_UPSTREAM_BILLING_NEWAPI_GROUP",
			fmt.Sprintf("upstream_billing_newapi_group must be at most %d characters", upstreamBillingNewAPIGroupMaxRunes),
		)
	}
	return normalized, nil
}

func upstreamBillingRechargeMultiplier(account *Account) (float64, bool) {
	if account == nil || account.Extra == nil {
		return upstreamBillingRechargeMultiplierDefault, true
	}
	if _, exists := account.Extra[UpstreamBillingRechargeMultiplierExtraKey]; !exists {
		return upstreamBillingRechargeMultiplierDefault, true
	}
	value, ok := resolveAccountExtraNumber(account.Extra, UpstreamBillingRechargeMultiplierExtraKey)
	if !ok || validateUpstreamBillingRechargeMultiplier(value) != nil {
		return 0, false
	}
	return value, true
}

func upstreamBillingNewAPIGroup(account *Account) (string, bool) {
	if account == nil || account.Extra == nil {
		return "", true
	}
	raw, exists := account.Extra[UpstreamBillingNewAPIGroupExtraKey]
	if !exists || raw == nil {
		return "", true
	}
	value, ok := raw.(string)
	if !ok {
		return "", false
	}
	normalized, err := normalizeUpstreamBillingNewAPIGroup(value)
	return normalized, err == nil
}

func buildUpstreamSiteEndpointURL(base string, endpoint string) string {
	normalized := strings.TrimSpace(base)
	endpoint = "/" + strings.TrimLeft(strings.TrimSpace(endpoint), "/")
	parsed, err := url.Parse(normalized)
	if err != nil {
		return strings.TrimRight(normalized, "/") + endpoint
	}
	path := strings.TrimRight(parsed.Path, "/")
	if openAIBaseURLHasVersionSuffix(path) {
		if slash := strings.LastIndex(path, "/"); slash >= 0 {
			path = path[:slash]
		} else {
			path = ""
		}
	}
	parsed.Path = strings.TrimRight(path, "/") + endpoint
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func decodeUpstreamBillingProbeSnapshot(extra map[string]any) *UpstreamBillingProbeSnapshot {
	if extra == nil {
		return nil
	}
	value, ok := extra[UpstreamBillingProbeExtraKey]
	if !ok {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var snapshot UpstreamBillingProbeSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot.Status == "" {
		return nil
	}
	if snapshot.Status != UpstreamBillingProbeStatusOK &&
		snapshot.Status != UpstreamBillingProbeStatusUnsupported &&
		snapshot.Status != UpstreamBillingProbeStatusFailed {
		return nil
	}
	return &snapshot
}

// IsUpstreamBillingProbeIdentity reports whether an account identity may opt
// in to the upstream billing probe. `/v1/sub2api/billing` is a key-scoped
// sub2api convention shared by the five supported API-key platforms.
// Non-sub2api upstreams return 404 and the snapshot records "unsupported".
// Only AccountTypeAPIKey is in scope. OAuth/Bedrock hold no static API key to
// present at all; AccountTypeUpstream (antigravity relay accounts) does carry
// a base_url plus a static api_key, but it is deliberately left out of the
// current supported set. New antigravity relay accounts are created with
// type=apikey by the admin form, so only pre-existing type=upstream rows
// cannot turn the probe on.
func IsUpstreamBillingProbeIdentity(platform, accountType string) bool {
	if accountType != AccountTypeAPIKey {
		return false
	}
	switch platform {
	case PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrok:
		return true
	default:
		return false
	}
}

func isUpstreamBillingProbeAccount(account *Account) bool {
	return account != nil && IsUpstreamBillingProbeIdentity(account.Platform, account.Type)
}

// upstreamBillingProbeOfficialAPIDomains lists the root domains of official
// provider APIs. The create form fills empty base_url values with official
// defaults (and offers official regional presets like us-east-1.api.x.ai),
// so probing them would send the account key to an official API path that
// cannot exist. Matching is by registrable root domain — exact host or any
// subdomain, after stripping the port and a trailing DNS dot — because no
// third-party sub2api relay can live under these domains, while custom
// relays (the only targets that can answer /v1/sub2api/billing) always do
// probe. OpenAI-platform accounts never reach this check: they keep the
// upstream-official behavior of probing api.openai.com.
// ollama.com is a first-class configuration here (Ollama Cloud accounts are
// platform openai/anthropic with base_url https://ollama.com/v1), and it is
// an official provider API just like the rest, so it belongs on this list.
var upstreamBillingProbeOfficialAPIDomains = []string{
	"anthropic.com",
	"googleapis.com",
	"x.ai",
	"grok.com",
	"openai.com",
	"ollama.com",
}

func upstreamBillingProbeTargetIsOfficialAPI(baseURL string) bool {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return true
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" {
		return true
	}
	for _, domain := range upstreamBillingProbeOfficialAPIDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func upstreamBillingProbeEnabled(account *Account) bool {
	if account == nil || account.Extra == nil {
		return false
	}
	enabled, ok := account.Extra[UpstreamBillingProbeEnabledExtraKey].(bool)
	return ok && enabled
}

// upstreamBillingRateSyncEnabled is the probe-side pre-filter deciding whether
// a rate is even proposed for write-back. It is a necessary condition, not the
// authority: the repository CAS re-checks both switches against the row it
// updates, so a switch flipped between load and write can never sneak a rate in.
func upstreamBillingRateSyncEnabled(account *Account) bool {
	if account == nil || account.Extra == nil {
		return false
	}
	enabled, ok := account.Extra[UpstreamBillingRateSyncEnabledExtraKey].(bool)
	return ok && enabled && upstreamBillingProbeEnabled(account)
}

func (s *UpstreamBillingProbeService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func nextProbeDelay(intervalMinutes int, retryAfterDuration time.Duration) time.Duration {
	interval := time.Duration(intervalMinutes) * time.Minute
	if interval < upstreamBillingProbeMinIntervalMinutes*time.Minute {
		interval = upstreamBillingProbeMinIntervalMinutes * time.Minute
	}
	if interval > upstreamBillingProbeMaxDelay {
		interval = upstreamBillingProbeMaxDelay
	}
	jitterRange := interval / 5
	if jitterRange > 5*time.Minute {
		jitterRange = 5 * time.Minute
	}
	if jitterRange > 0 {
		interval += time.Duration(rand.Int64N(int64(jitterRange)*2+1)) - jitterRange
	}
	if retryAfterDuration > interval {
		// Retry-After is an explicit upstream instruction; do not shorten it
		// with the local maximum delay.
		return retryAfterDuration
	}
	if interval > upstreamBillingProbeMaxDelay {
		return upstreamBillingProbeMaxDelay
	}
	return interval
}

// unsupportedProbeDelay 拉长 unsupported 账号的重探间隔，让无效候选自然退出
// 热队列，不再和真正接入 sub2api 的中转账号抢每周期的探测名额。
// 仍按 upstreamBillingProbeMaxDelay 封顶，保证上游后来接入 sub2api 时最迟一天
// 内会被重新发现；base 本身已达上限（例如 Retry-After 明确要求更久）时原样返回，
// 不缩短上游指令。
func unsupportedProbeDelay(intervalMinutes int, retryAfterDuration time.Duration) time.Duration {
	base := nextProbeDelay(intervalMinutes, retryAfterDuration)
	if base >= upstreamBillingProbeMaxDelay {
		return base
	}
	stretched := base * upstreamBillingProbeUnsupportedDelayFactor
	if stretched > upstreamBillingProbeMaxDelay {
		return upstreamBillingProbeMaxDelay
	}
	return stretched
}

func retryAfter(header http.Header, now time.Time) time.Duration {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if delay := at.Sub(now); delay > 0 {
			return delay
		}
	}
	return 0
}

func probeTimePtr(value time.Time) *time.Time {
	return &value
}

func safeProbeError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrUpstreamBillingProbeAccountInvalid) {
		return ErrUpstreamBillingProbeAccountInvalid.Error()
	}
	if errors.Is(err, ErrUpstreamBillingProbeUnavailable) {
		return ErrUpstreamBillingProbeUnavailable.Error()
	}
	return "probe_failed"
}

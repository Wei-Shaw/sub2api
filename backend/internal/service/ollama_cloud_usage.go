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

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"golang.org/x/net/http/httpguts"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

const (
	OllamaCloudUsageSessionExtraKey     = "ollama_cloud_usage_session"
	OllamaCloudUsageAutoRefreshExtraKey = "ollama_cloud_usage_auto_refresh"
	OllamaCloudUsageSnapshotExtraKey    = "ollama_cloud_usage_snapshot"

	// OllamaCloudUsageMinFetchInterval is the hard floor between two successful
	// fetches of the same group, mirroring the floor nextOllamaCloudUsageDelay
	// applies to next_refresh_at. Activity may bring a refresh forward to this
	// bound but never past it. Exported so the repository can apply the same
	// floor inside the SQL due filter.
	OllamaCloudUsageMinFetchInterval = ollamaCloudUsageMinIntervalMinutes * time.Minute

	ollamaCloudUsageSettingsURL            = "https://ollama.com/settings"
	ollamaCloudUsageDefaultIntervalMinutes = 60
	ollamaCloudUsageMinIntervalMinutes     = 15
	ollamaCloudUsageMaxIntervalMinutes     = 24 * 60
	ollamaCloudUsageDefaultDebounceMinutes = 1
	ollamaCloudUsageMinDebounceMinutes     = 1
	ollamaCloudUsageMaxDebounceMinutes     = 60
	ollamaCloudUsageCycleInterval          = time.Minute
	ollamaCloudUsageManualRefreshInterval  = 30 * time.Second
	ollamaCloudUsageRequestTimeout         = 15 * time.Second
	ollamaCloudUsageMaxBodyBytes           = 512 * 1024
	ollamaCloudUsageMaxSessionBytes        = 16 * 1024
	ollamaCloudUsageMaxPerCycle            = 20
	ollamaCloudUsageConcurrency            = 4
	ollamaCloudUsageMaxDelay               = 24 * time.Hour
	ollamaCloudUsageLeaderLockKey          = "ollama:cloud:usage:leader"
	ollamaCloudUsageLeaderLockTTL          = 2 * time.Minute
)

var (
	ErrOllamaCloudUsageUnavailable = infraerrors.ServiceUnavailable(
		"OLLAMA_CLOUD_USAGE_UNAVAILABLE", "Ollama Cloud usage is unavailable",
	)
	ErrOllamaCloudUsageAccountInvalid = infraerrors.BadRequest(
		"OLLAMA_CLOUD_USAGE_ACCOUNT_INVALID", "account must be an OpenAI or Anthropic API key account using https://ollama.com",
	)
	ErrOllamaCloudUsageSessionRequired = infraerrors.BadRequest(
		"OLLAMA_CLOUD_USAGE_SESSION_REQUIRED", "an Ollama web session must be configured first",
	)
	ErrOllamaCloudUsageEncryptionKey = infraerrors.BadRequest(
		"OLLAMA_CLOUD_USAGE_ENCRYPTION_KEY_NOT_CONFIGURED", "cannot store an Ollama web session without a fixed TOTP_ENCRYPTION_KEY",
	)
	ErrOllamaCloudUsageIdentityChanged = infraerrors.Conflict(
		"OLLAMA_CLOUD_USAGE_IDENTITY_CHANGED", "account identity or Ollama web session changed during refresh; retry",
	)
	ErrOllamaCloudUsageRefreshRateLimited = infraerrors.TooManyRequests(
		"OLLAMA_CLOUD_USAGE_REFRESH_RATE_LIMITED", "Ollama Cloud usage can be refreshed manually once every 30 seconds",
	)
	errOllamaCloudUsageUnauthorizedHTML = errors.New("settings HTML is a sign-in page")
)

const (
	OllamaCloudUsageStatusOK           = "ok"
	OllamaCloudUsageStatusUnauthorized = "unauthorized"
	OllamaCloudUsageStatusFailed       = "failed"
)

// OllamaCloudUsageSettings controls the opt-in request-driven refresh runner.
//
// IntervalMinutes is the max-wait bound: when model requests keep arriving and
// the trailing debounce keeps sliding, a refresh is forced after this long.
// DebounceMinutes is the quiet period after the latest request in a group.
type OllamaCloudUsageSettings struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"interval_minutes"` // max wait while requests continue
	DebounceMinutes int  `json:"debounce_minutes"` // trailing quiet period after last request
REDACTED

// OllamaCloudUsageWindow is a narrow, sanitized view of one official usage window.
type OllamaCloudUsageWindow struct {
	UsedPercent float64    `json:"used_percent"`
	ResetAt     *time.Time `json:"reset_at,omitempty"`
	ResetText   string     `json:"reset_text,omitempty"`
REDACTED

// OllamaCloudUsageModelWindow identifies the official window for a model count.
type OllamaCloudUsageModelWindow string

const (
	OllamaCloudUsageModelWindowFiveHour OllamaCloudUsageModelWindow = "five_hour"
	OllamaCloudUsageModelWindowSevenDay OllamaCloudUsageModelWindow = "seven_day"
)

// OllamaCloudUsageModel is the window-scoped model/request pair exposed by Ollama's usage DOM.
type OllamaCloudUsageModel struct {
	Model    string                      `json:"model"`
	Window   OllamaCloudUsageModelWindow `json:"window"`
	Requests int64                       `json:"requests"`
REDACTED

// OllamaCloudUsageData intentionally excludes raw HTML and browser-session data.
type OllamaCloudUsageData struct {
	Plan     string                  `json:"plan,omitempty"`
	FiveHour *OllamaCloudUsageWindow `json:"five_hour,omitempty"`
	SevenDay *OllamaCloudUsageWindow `json:"seven_day,omitempty"`
	Balance  string                  `json:"balance,omitempty"`
	Models   []OllamaCloudUsageModel `json:"models,omitempty"`
REDACTED

// OllamaCloudUsageSnapshot is the only usage observation persisted in account extra.
//
// NextRefreshAt remains a persisted compatibility field. For status=ok it is a
// max-wait horizon marker only; automatic success refreshes are driven by model
// request activity (group last_used_at + debounce/max-wait), not by this field
// alone. For failed/unauthorized snapshots it is the failure not-before time
// (Retry-After / exponential backoff) and is enforced as max(activityDue, NextRefreshAt).
type OllamaCloudUsageSnapshot struct {
	Status        string                `json:"status"`
	Data          *OllamaCloudUsageData `json:"data,omitempty"`
	FetchedAt     *time.Time            `json:"fetched_at,omitempty"`
	LastAttemptAt time.Time             `json:"last_attempt_at"`
	NextRefreshAt time.Time             `json:"next_refresh_at"`
	FailureCount  int                   `json:"failure_count,omitempty"`
	HTTPStatus    int                   `json:"http_status,omitempty"`
	LastError     string                `json:"last_error,omitempty"`
REDACTED

// OllamaCloudUsageState is the dedicated DTO exposed to administrators.
type OllamaCloudUsageState struct {
	AccountID               int64                     `json:"account_id"`
	Eligible                bool                      `json:"eligible"`
	Configured              bool                      `json:"configured"`
	AutoRefreshEnabled      bool                      `json:"auto_refresh_enabled"`
	EncryptionKeyConfigured bool                      `json:"encryption_key_configured"`
	Snapshot                *OllamaCloudUsageSnapshot `json:"snapshot,omitempty"`
REDACTED

type ollamaCloudUsageRepository interface {
	ListOllamaCloudUsageGroupAccounts(context.Context, []*Account) ([]Account, error)
	SaveOllamaCloudUsageSession(context.Context, *Account, string, bool) error
	DeleteOllamaCloudUsageSession(context.Context, *Account) error
	SetOllamaCloudUsageAutoRefresh(context.Context, *Account, bool) error
	UpdateOllamaCloudUsageSnapshot(context.Context, *Account, *OllamaCloudUsageSnapshot) error
	DisableOllamaCloudUsageAutoRefresh(context.Context, *Account) error
	ListDueOllamaCloudUsageAccounts(context.Context, time.Time, time.Duration, time.Duration, int) ([]Account, error)
REDACTED

// GetOllamaCloudUsageSettings returns fail-safe defaults when the setting is absent.
func (s *SettingService) GetOllamaCloudUsageSettings(ctx context.Context) (*OllamaCloudUsageSettings, error) {
	defaults := defaultOllamaCloudUsageSettings()
	if s == nil || s.settingRepo == nil {
		return defaults, nil
REDACTED
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOllamaCloudUsageSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return defaults, nil
	REDACTED
		return nil, fmt.Errorf("get Ollama Cloud usage settings: %w", err)
REDACTED
	if strings.TrimSpace(raw) == "" {
		return defaults, nil
REDACTED
	settings := *defaults
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, fmt.Errorf("parse Ollama Cloud usage settings: %w", err)
REDACTED
	if settings.IntervalMinutes == 0 {
		settings.IntervalMinutes = defaults.IntervalMinutes
REDACTED
	if settings.DebounceMinutes == 0 {
		settings.DebounceMinutes = defaults.DebounceMinutes
REDACTED
	normalizeOllamaCloudUsageSettings(&settings)
	return &settings, nil
REDACTED

func (s *SettingService) SetOllamaCloudUsageSettings(ctx context.Context, settings *OllamaCloudUsageSettings) error {
	if s == nil || s.settingRepo == nil {
		return ErrOllamaCloudUsageUnavailable
REDACTED
	if settings == nil {
		return infraerrors.BadRequest("INVALID_OLLAMA_CLOUD_USAGE_SETTINGS", "settings cannot be nil")
REDACTED
	if settings.DebounceMinutes == 0 {
		// Legacy clients that omit debounce_minutes keep the fail-safe default.
		settings.DebounceMinutes = ollamaCloudUsageDefaultDebounceMinutes
REDACTED
	if settings.IntervalMinutes < ollamaCloudUsageMinIntervalMinutes || settings.IntervalMinutes > ollamaCloudUsageMaxIntervalMinutes {
		return infraerrors.BadRequest(
			"INVALID_OLLAMA_CLOUD_USAGE_INTERVAL",
			fmt.Sprintf("interval_minutes must be between %d and %d", ollamaCloudUsageMinIntervalMinutes, ollamaCloudUsageMaxIntervalMinutes),
		)
REDACTED
	if settings.DebounceMinutes < ollamaCloudUsageMinDebounceMinutes || settings.DebounceMinutes > ollamaCloudUsageMaxDebounceMinutes {
		return infraerrors.BadRequest(
			"INVALID_OLLAMA_CLOUD_USAGE_DEBOUNCE",
			fmt.Sprintf("debounce_minutes must be between %d and %d", ollamaCloudUsageMinDebounceMinutes, ollamaCloudUsageMaxDebounceMinutes),
		)
REDACTED
	// The due time is min(lastUsed+debounce, fetchedAt+maxWait). Once the debounce
	// reaches the max wait the debounce term can never win, so the knob would be
	// silently inert instead of doing what the operator asked for.
	if settings.DebounceMinutes >= settings.IntervalMinutes {
		return infraerrors.BadRequest(
			"INVALID_OLLAMA_CLOUD_USAGE_DEBOUNCE",
			fmt.Sprintf("debounce_minutes (%d) must be less than interval_minutes (%d)", settings.DebounceMinutes, settings.IntervalMinutes),
		)
REDACTED
	normalizeOllamaCloudUsageSettings(settings)
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal Ollama Cloud usage settings: %w", err)
REDACTED
	return s.settingRepo.Set(ctx, SettingKeyOllamaCloudUsageSettings, string(data))
REDACTED

func defaultOllamaCloudUsageSettings() *OllamaCloudUsageSettings {
	return &OllamaCloudUsageSettings{
		Enabled:         false,
		IntervalMinutes: ollamaCloudUsageDefaultIntervalMinutes,
		DebounceMinutes: ollamaCloudUsageDefaultDebounceMinutes,
REDACTED
REDACTED

func normalizeOllamaCloudUsageSettings(settings *OllamaCloudUsageSettings) {
	if settings.IntervalMinutes < ollamaCloudUsageMinIntervalMinutes {
		settings.IntervalMinutes = ollamaCloudUsageMinIntervalMinutes
REDACTED
	if settings.IntervalMinutes > ollamaCloudUsageMaxIntervalMinutes {
		settings.IntervalMinutes = ollamaCloudUsageMaxIntervalMinutes
REDACTED
	if settings.DebounceMinutes <= 0 {
		settings.DebounceMinutes = ollamaCloudUsageDefaultDebounceMinutes
REDACTED
	if settings.DebounceMinutes < ollamaCloudUsageMinDebounceMinutes {
		settings.DebounceMinutes = ollamaCloudUsageMinDebounceMinutes
REDACTED
	if settings.DebounceMinutes > ollamaCloudUsageMaxDebounceMinutes {
		settings.DebounceMinutes = ollamaCloudUsageMaxDebounceMinutes
REDACTED
REDACTED

func ollamaCloudUsageDurations(settings *OllamaCloudUsageSettings) (debounce, maxWait time.Duration) {
	normalized := defaultOllamaCloudUsageSettings()
	if settings != nil {
		*normalized = *settings
REDACTED
	normalizeOllamaCloudUsageSettings(normalized)
	return time.Duration(normalized.DebounceMinutes) * time.Minute,
		time.Duration(normalized.IntervalMinutes) * time.Minute
REDACTED

// ollamaCloudUsageIsAutoRefreshDue decides whether a configured auto-refresh
// group should fetch now. groupLastUsedAt must be MAX(last_used_at) across the
// exact api_key group so shared multi-platform accounts do not miss activity.
//
// Success: a request must be newer than fetched_at; dueAt = min(lastUsed+debounce, fetchedAt+maxWait).
// Failure: a request must be newer than last_attempt_at; activity due uses the same min formula,
// then dueAt = max(activityDue, next_refresh_at) so Retry-After / exponential backoff win.
// Missing or invalid snapshots fail open to a first fetch.
func ollamaCloudUsageIsAutoRefreshDue(
	snapshot *OllamaCloudUsageSnapshot,
	groupLastUsedAt *time.Time,
	now time.Time,
	debounce, maxWait time.Duration,
) bool {
	dueAt, ok := ollamaCloudUsageAutoRefreshDueAt(snapshot, groupLastUsedAt, debounce, maxWait)
	if !ok {
		return false
REDACTED
	return !now.Before(dueAt)
REDACTED

func ollamaCloudUsageAutoRefreshDueAt(
	snapshot *OllamaCloudUsageSnapshot,
	groupLastUsedAt *time.Time,
	debounce, maxWait time.Duration,
) (time.Time, bool) {
	if debounce <= 0 {
		debounce = time.Duration(ollamaCloudUsageDefaultDebounceMinutes) * time.Minute
REDACTED
	if maxWait <= 0 {
		maxWait = time.Duration(ollamaCloudUsageDefaultIntervalMinutes) * time.Minute
REDACTED
	if snapshot == nil {
		return time.Time{REDACTED, true
REDACTED
	switch snapshot.Status {
	case OllamaCloudUsageStatusOK:
		if snapshot.FetchedAt == nil || snapshot.FetchedAt.IsZero() {
			return time.Time{REDACTED, true
	REDACTED
		fetchedAt := snapshot.FetchedAt.UTC()
		if groupLastUsedAt == nil || !groupLastUsedAt.After(fetchedAt) {
			return time.Time{REDACTED, false
	REDACTED
		lastUsed := groupLastUsedAt.UTC()
		dueAt := minTime(lastUsed.Add(debounce), fetchedAt.Add(maxWait))
		// Keep the pre-existing hard floor between successful fetches. The success
		// path no longer consults next_refresh_at, which is where
		// nextOllamaCloudUsageDelay used to apply ollamaCloudUsageMinIntervalMinutes;
		// without this, request traffic spaced slightly wider than the debounce
		// drives the group's outbound rate far above the previous minimum.
		if floor := fetchedAt.Add(OllamaCloudUsageMinFetchInterval); dueAt.Before(floor) {
			return floor, true
	REDACTED
		return dueAt, true
	case OllamaCloudUsageStatusFailed, OllamaCloudUsageStatusUnauthorized:
		if snapshot.LastAttemptAt.IsZero() {
			return time.Time{REDACTED, true
	REDACTED
		lastAttempt := snapshot.LastAttemptAt.UTC()
		if groupLastUsedAt == nil || !groupLastUsedAt.After(lastAttempt) {
			return time.Time{REDACTED, false
	REDACTED
		lastUsed := groupLastUsedAt.UTC()
		activityDue := minTime(lastUsed.Add(debounce), lastAttempt.Add(maxWait))
		if !snapshot.NextRefreshAt.IsZero() && snapshot.NextRefreshAt.UTC().After(activityDue) {
			return snapshot.NextRefreshAt.UTC(), true
	REDACTED
		return activityDue, true
	default:
		return time.Time{REDACTED, true
REDACTED
REDACTED

// maxOllamaCloudUsageGroupLastUsed returns the newest last_used_at among group members.
func maxOllamaCloudUsageGroupLastUsed(accounts []Account) *time.Time {
	var latest *time.Time
	for i := range accounts {
		candidate := accounts[i].LastUsedAt
		if candidate == nil || candidate.IsZero() {
			continue
	REDACTED
		if latest == nil || candidate.After(*latest) {
			ts := candidate.UTC()
			latest = &ts
	REDACTED
REDACTED
	return latest
REDACTED

// scheduleOllamaCloudUsageActivity records that an Ollama Cloud API-key account
// actually attempted an upstream model request (including 429/5xx/transport errors).
// Local auth/validation failures must not call this. DeferredService dedupes writes.
func scheduleOllamaCloudUsageActivity(deferred *DeferredService, account *Account) {
	if deferred == nil || account == nil || !IsOllamaCloudUsageAccount(account) {
		return
REDACTED
	deferred.ScheduleLastUsedUpdate(account.ID)
REDACTED

// OllamaCloudUsageService refreshes the official settings HTML without affecting routing state.
type OllamaCloudUsageService struct {
	accountRepo             AccountRepository
	httpUpstream            HTTPUpstream
	settingService          *SettingService
	encryptor               SecretEncryptor
	encryptionKeyConfigured bool

	parentCtx    context.Context
	parentCancel context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
	started      bool
	stopped      bool
	cycleMu      sync.Mutex
	refreshGroup singleflight.Group
	refreshSlots chan struct{REDACTED
	now          func() time.Time
	lockCache    LeaderLockCache
	db           *sql.DB
	instanceID   string
REDACTED

func NewOllamaCloudUsageService(
	accountRepo AccountRepository,
	httpUpstream HTTPUpstream,
	settingService *SettingService,
	encryptor SecretEncryptor,
	encryptionKeyConfigured bool,
) *OllamaCloudUsageService {
	ctx, cancel := context.WithCancel(context.Background())
	return &OllamaCloudUsageService{
		accountRepo:             accountRepo,
		httpUpstream:            httpUpstream,
		settingService:          settingService,
		encryptor:               encryptor,
		encryptionKeyConfigured: encryptionKeyConfigured,
		parentCtx:               ctx,
		parentCancel:            cancel,
		refreshSlots:            make(chan struct{REDACTED, ollamaCloudUsageConcurrency),
		now:                     time.Now,
		instanceID:              uuid.NewString(),
REDACTED
REDACTED

func ProvideOllamaCloudUsageService(
	accountRepo AccountRepository,
	httpUpstream HTTPUpstream,
	settingService *SettingService,
	encryptor SecretEncryptor,
	cfg *config.Config,
	lockCache LeaderLockCache,
	db *sql.DB,
) *OllamaCloudUsageService {
	keyConfigured := cfg != nil && cfg.Totp.EncryptionKeyConfigured
	svc := NewOllamaCloudUsageService(accountRepo, httpUpstream, settingService, encryptor, keyConfigured)
	svc.lockCache = lockCache
	svc.db = db
	svc.Start()
	return svc
REDACTED

func (s *OllamaCloudUsageService) Start() {
	if s == nil {
		return
REDACTED
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return
REDACTED
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go s.runLoop()
REDACTED

func (s *OllamaCloudUsageService) Stop() {
	if s == nil {
		return
REDACTED
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
REDACTED
	s.stopped = true
	s.parentCancel()
	s.mu.Unlock()
	s.wg.Wait()
REDACTED

func (s *OllamaCloudUsageService) runLoop() {
	defer s.wg.Done()
	_ = s.RunDue(s.parentCtx)
	ticker := time.NewTicker(ollamaCloudUsageCycleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case <-ticker.C:
			if err := s.RunDue(s.parentCtx); err != nil {
				logger.LegacyPrintf("service.ollama_cloud_usage", "run_due_failed: err=%v", err)
		REDACTED
	REDACTED
REDACTED
REDACTED

func (s *OllamaCloudUsageService) GetSettings(ctx context.Context) (*OllamaCloudUsageSettings, error) {
	if s == nil || s.settingService == nil {
		return defaultOllamaCloudUsageSettings(), nil
REDACTED
	return s.settingService.GetOllamaCloudUsageSettings(ctx)
REDACTED

func (s *OllamaCloudUsageService) UpdateSettings(ctx context.Context, settings *OllamaCloudUsageSettings) error {
	if s == nil || s.settingService == nil {
		return ErrOllamaCloudUsageUnavailable
REDACTED
	return s.settingService.SetOllamaCloudUsageSettings(ctx, settings)
REDACTED

func (s *OllamaCloudUsageService) GetState(ctx context.Context, accountID int64) (*OllamaCloudUsageState, error) {
	if s == nil || s.accountRepo == nil {
		return nil, ErrOllamaCloudUsageUnavailable
REDACTED
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
REDACTED
	if err := s.ResolveAccounts(ctx, []*Account{accountREDACTED); err != nil {
		return nil, err
REDACTED
	state := OllamaCloudUsageStateFromAccount(account)
	s.EnrichState(state)
	return state, nil
REDACTED

// ResolveAccounts overlays group-owned managed state onto the supplied account
// objects. The repository resolves all matching siblings in one bounded query,
// so account-list responses do not issue one query per row.
func (s *OllamaCloudUsageService) ResolveAccounts(ctx context.Context, accounts []*Account) error {
	if s == nil || s.accountRepo == nil || len(accounts) == 0 {
		return nil
REDACTED
	writer, ok := s.accountRepo.(ollamaCloudUsageRepository)
	if !ok {
		return nil
REDACTED
	eligible := make([]*Account, 0, len(accounts))
	for _, account := range accounts {
		if _, ok := ollamaCloudUsageGroupFingerprint(account); ok {
			eligible = append(eligible, account)
	REDACTED
REDACTED
	if len(eligible) == 0 {
		return nil
REDACTED
	siblings, err := writer.ListOllamaCloudUsageGroupAccounts(ctx, eligible)
	if err != nil {
		return fmt.Errorf("resolve Ollama Cloud usage groups: %w", err)
REDACTED
	sources := make(map[string]*Account)
	for index := range siblings {
		candidate := &siblings[index]
		fingerprint, valid := ollamaCloudUsageGroupFingerprint(candidate)
		if !valid || !ollamaCloudUsageConfigured(candidate) {
			continue
	REDACTED
		current := sources[fingerprint]
		if current == nil || candidate.UpdatedAt.After(current.UpdatedAt) ||
			(candidate.UpdatedAt.Equal(current.UpdatedAt) && candidate.ID < current.ID) {
			sources[fingerprint] = candidate
	REDACTED
REDACTED
	resolvedSources := make(map[string]*Account, len(sources))
	for fingerprint, source := range sources {
		clone := *source
		clone.Extra = make(map[string]any, len(source.Extra))
		maps.Copy(clone.Extra, source.Extra)
		resolvedSources[fingerprint] = &clone
REDACTED
	for index := range siblings {
		candidate := &siblings[index]
		fingerprint, valid := ollamaCloudUsageGroupFingerprint(candidate)
		source := resolvedSources[fingerprint]
		if !valid || source == nil || !sameOllamaCloudUsageSession(source, candidate) {
			continue
	REDACTED
		candidateSnapshot := decodeOllamaCloudUsageSnapshot(candidate.Extra)
		currentSnapshot := decodeOllamaCloudUsageSnapshot(source.Extra)
		if candidateSnapshot != nil && (currentSnapshot == nil || candidateSnapshot.LastAttemptAt.After(currentSnapshot.LastAttemptAt)) {
			source.Extra[OllamaCloudUsageSnapshotExtraKey] = candidate.Extra[OllamaCloudUsageSnapshotExtraKey]
	REDACTED
REDACTED
	for _, account := range eligible {
		fingerprint, _ := ollamaCloudUsageGroupFingerprint(account)
		applyOllamaCloudUsageManagedExtra(account, resolvedSources[fingerprint])
REDACTED
	return nil
REDACTED

func sameOllamaCloudUsageSession(left, right *Account) bool {
	if left == nil || right == nil || left.Extra == nil || right.Extra == nil {
		return false
REDACTED
	leftSession, leftOK := left.Extra[OllamaCloudUsageSessionExtraKey].(string)
	rightSession, rightOK := right.Extra[OllamaCloudUsageSessionExtraKey].(string)
	return leftOK && rightOK && leftSession != "" && leftSession == rightSession
REDACTED

func applyOllamaCloudUsageManagedExtra(target, source *Account) {
	if target == nil {
		return
REDACTED
	if target.Extra == nil {
		target.Extra = make(map[string]any)
REDACTED
	for _, key := range []string{
		OllamaCloudUsageSessionExtraKey,
		OllamaCloudUsageAutoRefreshExtraKey,
		OllamaCloudUsageSnapshotExtraKey,
REDACTED {
		delete(target.Extra, key)
		if source != nil && source.Extra != nil {
			if value, ok := source.Extra[key]; ok {
				target.Extra[key] = value
		REDACTED
	REDACTED
REDACTED
REDACTED

func (s *OllamaCloudUsageService) SaveSession(ctx context.Context, accountID int64, session string) (*OllamaCloudUsageState, error) {
	if s == nil || s.accountRepo == nil || s.encryptor == nil {
		return nil, ErrOllamaCloudUsageUnavailable
REDACTED
	if !s.encryptionKeyConfigured {
		return nil, ErrOllamaCloudUsageEncryptionKey
REDACTED
	normalized, err := normalizeOllamaCloudUsageCookie(session)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_OLLAMA_CLOUD_USAGE_SESSION", err.Error())
REDACTED
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
REDACTED
	if !IsOllamaCloudUsageAccount(account) {
		return nil, ErrOllamaCloudUsageAccountInvalid
REDACTED
	if err := s.ResolveAccounts(ctx, []*Account{accountREDACTED); err != nil {
		return nil, err
REDACTED
	ciphertext, err := s.encryptor.Encrypt(normalized)
	if err != nil {
		return nil, fmt.Errorf("encrypt Ollama web session: %w", err)
REDACTED
	writer, ok := s.accountRepo.(ollamaCloudUsageRepository)
	if !ok {
		return nil, ErrOllamaCloudUsageUnavailable
REDACTED
	preserveAutoRefresh := ollamaCloudUsageConfigured(account) && ollamaCloudUsageAutoRefreshEnabled(account)
	if err := writer.SaveOllamaCloudUsageSession(ctx, account, ciphertext, preserveAutoRefresh); err != nil {
		return nil, err
REDACTED
	return s.GetState(ctx, accountID)
REDACTED

func (s *OllamaCloudUsageService) DeleteSession(ctx context.Context, accountID int64) (*OllamaCloudUsageState, error) {
	if s == nil || s.accountRepo == nil {
		return nil, ErrOllamaCloudUsageUnavailable
REDACTED
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
REDACTED
	if !IsOllamaCloudUsageAccount(account) {
		return nil, ErrOllamaCloudUsageAccountInvalid
REDACTED
	if err := s.ResolveAccounts(ctx, []*Account{accountREDACTED); err != nil {
		return nil, err
REDACTED
	writer, ok := s.accountRepo.(ollamaCloudUsageRepository)
	if !ok {
		return nil, ErrOllamaCloudUsageUnavailable
REDACTED
	if err := writer.DeleteOllamaCloudUsageSession(ctx, account); err != nil {
		return nil, err
REDACTED
	return s.GetState(ctx, accountID)
REDACTED

func (s *OllamaCloudUsageService) SetAutoRefresh(ctx context.Context, accountID int64, enabled bool) (*OllamaCloudUsageState, error) {
	if s == nil || s.accountRepo == nil {
		return nil, ErrOllamaCloudUsageUnavailable
REDACTED
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
REDACTED
	if !IsOllamaCloudUsageAccount(account) {
		return nil, ErrOllamaCloudUsageAccountInvalid
REDACTED
	if err := s.ResolveAccounts(ctx, []*Account{accountREDACTED); err != nil {
		return nil, err
REDACTED
	if enabled && !ollamaCloudUsageConfigured(account) {
		return nil, ErrOllamaCloudUsageSessionRequired
REDACTED
	writer, ok := s.accountRepo.(ollamaCloudUsageRepository)
	if !ok {
		return nil, ErrOllamaCloudUsageUnavailable
REDACTED
	if err := writer.SetOllamaCloudUsageAutoRefresh(ctx, account, enabled); err != nil {
		return nil, err
REDACTED
	return s.GetState(ctx, accountID)
REDACTED

func (s *OllamaCloudUsageService) Refresh(ctx context.Context, accountID int64) (*OllamaCloudUsageState, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
REDACTED
	if _, err := s.refreshAccount(ctx, accountID, settings, false); err != nil {
		return nil, err
REDACTED
	return s.GetState(ctx, accountID)
REDACTED

func (s *OllamaCloudUsageService) RunDue(ctx context.Context) error {
	if s == nil || s.accountRepo == nil {
		return nil
REDACTED
	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return err
REDACTED
	if !settings.Enabled {
		return nil
REDACTED
	release, acquired := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, ollamaCloudUsageLeaderLockKey, s.instanceID, ollamaCloudUsageLeaderLockTTL)
	if !acquired {
		return nil
REDACTED
	defer release()

	writer, ok := s.accountRepo.(ollamaCloudUsageRepository)
	if !ok {
		return ErrOllamaCloudUsageUnavailable
REDACTED
	now := s.currentTime()
	debounce, maxWait := ollamaCloudUsageDurations(settings)
	accounts, err := writer.ListDueOllamaCloudUsageAccounts(ctx, now, debounce, maxWait, ollamaCloudUsageMaxPerCycle)
	if err != nil {
		return fmt.Errorf("list due Ollama Cloud usage accounts: %w", err)
REDACTED
	var group errgroup.Group
	seenGroups := make(map[string]struct{REDACTED, len(accounts))
	for index := range accounts {
		account := accounts[index]
		fingerprint, valid := ollamaCloudUsageGroupFingerprint(&account)
		if !valid || !account.IsActive() || !ollamaCloudUsageConfigured(&account) || !ollamaCloudUsageAutoRefreshEnabled(&account) {
			continue
	REDACTED
		if _, duplicate := seenGroups[fingerprint]; duplicate {
			continue
	REDACTED
		seenGroups[fingerprint] = struct{REDACTED{REDACTED
		snapshot := decodeOllamaCloudUsageSnapshot(account.Extra)
		// ListDue stamps Account.LastUsedAt with the api_key group MAX(last_used_at).
		if !ollamaCloudUsageIsAutoRefreshDue(snapshot, account.LastUsedAt, now, debounce, maxWait) {
			continue
	REDACTED
		accountID := account.ID
		expected := account
		group.Go(func() error {
			if _, refreshErr := s.refreshAccount(ctx, accountID, settings, true); refreshErr != nil {
				if errors.Is(refreshErr, ErrOllamaCloudUsageIdentityChanged) {
					if disableErr := writer.DisableOllamaCloudUsageAutoRefresh(ctx, &expected); disableErr != nil {
						logger.LegacyPrintf("service.ollama_cloud_usage", "disable_auto_refresh_failed: account_id=%d err=%v", accountID, disableErr)
				REDACTED
					return nil
			REDACTED
				logger.LegacyPrintf("service.ollama_cloud_usage", "refresh_due_failed: account_id=%d err=%v", accountID, refreshErr)
		REDACTED
			return nil
	REDACTED)
REDACTED
	return group.Wait()
REDACTED

func (s *OllamaCloudUsageService) refreshAccount(ctx context.Context, accountID int64, settings *OllamaCloudUsageSettings, requireEnabled bool) (*OllamaCloudUsageSnapshot, error) {
	if s == nil || s.accountRepo == nil {
		return nil, ErrOllamaCloudUsageUnavailable
REDACTED
	if settings == nil {
		settings = defaultOllamaCloudUsageSettings()
REDACTED
	intervalMinutes := settings.IntervalMinutes
	debounce, maxWait := ollamaCloudUsageDurations(settings)
	anchor, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
REDACTED
	key, valid := ollamaCloudUsageGroupFingerprint(anchor)
	if !valid {
		return nil, ErrOllamaCloudUsageAccountInvalid
REDACTED
	value, err, _ := s.refreshGroup.Do(key, func() (any, error) {
		select {
		case s.refreshSlots <- struct{REDACTED{REDACTED:
			defer func() { <-s.refreshSlots REDACTED()
		case <-ctx.Done():
			return nil, ctx.Err()
	REDACTED
		account, loadErr := s.accountRepo.GetByID(ctx, accountID)
		if loadErr != nil {
			return nil, loadErr
	REDACTED
		currentKey, currentValid := ollamaCloudUsageGroupFingerprint(account)
		if !currentValid {
			return nil, ErrOllamaCloudUsageAccountInvalid
	REDACTED
		if currentKey != key {
			return nil, ErrOllamaCloudUsageIdentityChanged
	REDACTED
		if err := s.ResolveAccounts(ctx, []*Account{accountREDACTED); err != nil {
			return nil, err
	REDACTED
		if !ollamaCloudUsageConfigured(account) {
			return nil, ErrOllamaCloudUsageSessionRequired
	REDACTED
		if !requireEnabled {
			if snapshot := decodeOllamaCloudUsageSnapshot(account.Extra); snapshot != nil && !snapshot.LastAttemptAt.IsZero() {
				retryAt := snapshot.LastAttemptAt.Add(ollamaCloudUsageManualRefreshInterval)
				if now := s.currentTime(); now.Before(retryAt) {
					remaining := retryAt.Sub(now)
					seconds := int((remaining + time.Second - 1) / time.Second)
					return nil, ErrOllamaCloudUsageRefreshRateLimited.WithMetadata(map[string]string{
						"retry_after_seconds": strconv.Itoa(seconds),
				REDACTED)
			REDACTED
		REDACTED
	REDACTED
		if requireEnabled {
			if !account.IsActive() || !ollamaCloudUsageAutoRefreshEnabled(account) {
				return nil, nil
		REDACTED
			groupLastUsed := account.LastUsedAt
			if writer, ok := s.accountRepo.(ollamaCloudUsageRepository); ok {
				siblings, listErr := writer.ListOllamaCloudUsageGroupAccounts(ctx, []*Account{accountREDACTED)
				if listErr != nil {
					// Fall back to this account's own last_used_at. That is a narrower
					// activity signal than the group maximum, so the due check may skip a
					// refresh it would otherwise have run; surface it rather than
					// silently changing the due semantics.
					logger.LegacyPrintf("service.ollama_cloud_usage",
						"group_last_used_lookup_failed: account_id=%d err=%v", account.ID, listErr)
			REDACTED else {
					groupLastUsed = maxOllamaCloudUsageGroupLastUsed(siblings)
			REDACTED
		REDACTED
			if !ollamaCloudUsageIsAutoRefreshDue(decodeOllamaCloudUsageSnapshot(account.Extra), groupLastUsed, s.currentTime(), debounce, maxWait) {
				return nil, nil
		REDACTED
	REDACTED
		return s.refreshLoadedAccount(ctx, account, intervalMinutes)
REDACTED)
	if err != nil || value == nil {
		return nil, err
REDACTED
	snapshot, ok := value.(*OllamaCloudUsageSnapshot)
	if !ok {
		return nil, fmt.Errorf("invalid Ollama Cloud usage refresh result")
REDACTED
	return snapshot, nil
REDACTED

func (s *OllamaCloudUsageService) refreshLoadedAccount(ctx context.Context, account *Account, intervalMinutes int) (*OllamaCloudUsageSnapshot, error) {
	now := s.currentTime().UTC()
	ciphertext, _ := account.Extra[OllamaCloudUsageSessionExtraKey].(string)
	if ciphertext == "" {
		return nil, ErrOllamaCloudUsageSessionRequired
REDACTED
	if !s.encryptionKeyConfigured || s.encryptor == nil {
		return nil, ErrOllamaCloudUsageEncryptionKey
REDACTED
	cookie, err := s.encryptor.Decrypt(ciphertext)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("OLLAMA_CLOUD_USAGE_SESSION_DECRYPT_FAILED", "stored Ollama web session cannot be decrypted")
REDACTED
	cookie, err = normalizeOllamaCloudUsageCookie(cookie)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("OLLAMA_CLOUD_USAGE_SESSION_INVALID", "stored Ollama web session is invalid")
REDACTED
	if s.httpUpstream == nil {
		return nil, ErrOllamaCloudUsageUnavailable
REDACTED
	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy == nil || account.Proxy.ID != *account.ProxyID {
			return nil, ErrOllamaCloudUsageIdentityChanged
	REDACTED
		proxyURL = account.Proxy.URL()
REDACTED
	requestCtx, cancel := context.WithTimeout(WithHTTPUpstreamRedirectsDisabled(ctx), ollamaCloudUsageRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, ollamaCloudUsageSettingsURL, nil)
	if err != nil || !isExactOllamaCloudSettingsURL(req.URL) {
		return nil, ErrOllamaCloudUsageUnavailable
REDACTED
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "sub2api-ollama-usage/1")
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return s.persistFailure(ctx, account, intervalMinutes, now, 0, "request_failed", 0, false)
REDACTED
	if resp == nil || resp.Body == nil {
		return s.persistFailure(ctx, account, intervalMinutes, now, 0, "empty_response", 0, false)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	if resp.Request != nil && !isExactOllamaCloudSettingsURL(resp.Request.URL) {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "response_host_mismatch", 0, false)
REDACTED
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "redirect_blocked", retryAfter(resp.Header, now), false)
REDACTED
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "unauthorized", retryAfter(resp.Header, now), true)
REDACTED
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "http_error", retryAfter(resp.Header, now), false)
REDACTED
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, ollamaCloudUsageMaxBodyBytes+1))
	if readErr != nil {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "response_read_failed", 0, false)
REDACTED
	if len(body) > ollamaCloudUsageMaxBodyBytes {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "response_too_large", 0, false)
REDACTED
	data, parseErr := parseOllamaCloudUsageHTML(body)
	if errors.Is(parseErr, errOllamaCloudUsageUnauthorizedHTML) {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "unauthorized", 0, true)
REDACTED
	if parseErr != nil {
		return s.persistFailure(ctx, account, intervalMinutes, now, resp.StatusCode, "invalid_html", 0, false)
REDACTED
	snapshot := &OllamaCloudUsageSnapshot{
		Status:        OllamaCloudUsageStatusOK,
		Data:          data,
		FetchedAt:     &now,
		LastAttemptAt: now,
		NextRefreshAt: now.Add(nextOllamaCloudUsageDelay(intervalMinutes, 0, 0)),
		HTTPStatus:    resp.StatusCode,
REDACTED
	if err := s.updateSnapshot(ctx, account, snapshot); err != nil {
		return nil, err
REDACTED
	return snapshot, nil
REDACTED

func (s *OllamaCloudUsageService) persistFailure(
	ctx context.Context,
	account *Account,
	intervalMinutes int,
	now time.Time,
	httpStatus int,
	reason string,
	retryAfterDuration time.Duration,
	unauthorized bool,
) (*OllamaCloudUsageSnapshot, error) {
	previous := decodeOllamaCloudUsageSnapshot(account.Extra)
	failureCount := 1
	if previous != nil {
		failureCount = previous.FailureCount + 1
REDACTED
	status := OllamaCloudUsageStatusFailed
	if unauthorized {
		status = OllamaCloudUsageStatusUnauthorized
REDACTED
	snapshot := &OllamaCloudUsageSnapshot{
		Status:        status,
		LastAttemptAt: now,
		NextRefreshAt: now.Add(nextOllamaCloudUsageDelay(intervalMinutes, failureCount, retryAfterDuration)),
		FailureCount:  failureCount,
		HTTPStatus:    httpStatus,
		LastError:     reason,
REDACTED
	if previous != nil {
		snapshot.Data = previous.Data
		snapshot.FetchedAt = previous.FetchedAt
REDACTED
	if err := s.updateSnapshot(ctx, account, snapshot); err != nil {
		return nil, err
REDACTED
	return snapshot, nil
REDACTED

func (s *OllamaCloudUsageService) updateSnapshot(ctx context.Context, account *Account, snapshot *OllamaCloudUsageSnapshot) error {
	writer, ok := s.accountRepo.(ollamaCloudUsageRepository)
	if !ok {
		return ErrOllamaCloudUsageUnavailable
REDACTED
	return writer.UpdateOllamaCloudUsageSnapshot(ctx, account, snapshot)
REDACTED

// EnrichState adds service-owned runtime configuration to an account-derived state.
func (s *OllamaCloudUsageService) EnrichState(state *OllamaCloudUsageState) {
	if state == nil {
		return
REDACTED
	state.EncryptionKeyConfigured = s != nil && s.encryptionKeyConfigured
REDACTED

func OllamaCloudUsageStateFromAccount(account *Account) *OllamaCloudUsageState {
	state := &OllamaCloudUsageState{REDACTED
	if account == nil {
		return state
REDACTED
	state.AccountID = account.ID
	state.Eligible = IsOllamaCloudUsageAccount(account)
	if !state.Eligible {
		return state
REDACTED
	state.Configured = ollamaCloudUsageConfigured(account)
	state.AutoRefreshEnabled = state.Configured && ollamaCloudUsageAutoRefreshEnabled(account)
	state.Snapshot = decodeOllamaCloudUsageSnapshot(account.Extra)
	return state
REDACTED

func IsOllamaCloudUsageAccount(account *Account) bool {
	if account == nil || account.Type != AccountTypeAPIKey || (account.Platform != PlatformOpenAI && account.Platform != PlatformAnthropic) {
		return false
REDACTED
	baseURL, _ := account.Credentials["base_url"].(string)
	return isOllamaCloudBaseURL(baseURL)
REDACTED

func isOllamaCloudBaseURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "?#") {
		return false
REDACTED
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawFragment != "" {
		return false
REDACTED
	hostname := strings.ToLower(parsed.Hostname())
	if hostname != "ollama.com" && hostname != "www.ollama.com" {
		return false
REDACTED
	authority := strings.ToLower(parsed.Host)
	if authority != hostname && authority != hostname+":443" {
		return false
REDACTED
	if parsed.RawPath != "" {
		return false
REDACTED
	return parsed.Path == "" || parsed.Path == "/v1"
REDACTED

func ollamaCloudUsageIdentity(account *Account) map[string]any {
	if !IsOllamaCloudUsageAccount(account) {
		return nil
REDACTED
	apiKey, ok := account.Credentials["api_key"].(string)
	if !ok || apiKey == "" {
		return nil
REDACTED
	return map[string]any{"host": "ollama.com", "api_key": apiKeyREDACTED
REDACTED

func ollamaCloudUsageGroupFingerprint(account *Account) (string, bool) {
	identity := ollamaCloudUsageIdentity(account)
	if identity == nil {
		return "", false
REDACTED
	apiKey, _ := identity["api_key"].(string)
	sum := sha256.Sum256([]byte("ollama.com\x00" + apiKey))
	return hex.EncodeToString(sum[:]), true
REDACTED

func isExactOllamaCloudSettingsURL(parsed *url.URL) bool {
	return parsed != nil && parsed.Scheme == "https" && parsed.Host == "ollama.com" && parsed.Path == "/settings" &&
		parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && parsed.RawPath == ""
REDACTED

func normalizeOllamaCloudUsageCookie(raw string) (string, error) {
	if len(raw) > ollamaCloudUsageMaxSessionBytes {
		return "", errors.New("session is too large")
REDACTED
	raw = strings.TrimSpace(raw)
	if strings.ContainsAny(raw, "\r\n") {
		return "", errors.New("session contains invalid header characters")
REDACTED
	if raw == "" {
		return "", errors.New("session cannot be empty")
REDACTED
	if !httpguts.ValidHeaderFieldValue(raw) {
		return "", errors.New("session contains invalid header characters")
REDACTED
	blockedAttributes := map[string]struct{REDACTED{
		"domain": {REDACTED, "path": {REDACTED, "expires": {REDACTED, "max-age": {REDACTED, "samesite": {REDACTED, "secure": {REDACTED, "httponly": {REDACTED, "partitioned": {REDACTED,
REDACTED
	parts := strings.Split(raw, ";")
	normalized := make([]string, 0, len(parts))
	seen := make(map[string]struct{REDACTED, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		name, value, ok := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if !ok || name == "" || value == "" || !httpguts.ValidHeaderFieldName(name) || strings.HasPrefix(name, "$") {
			return "", errors.New("session must be a Cookie header containing name=value pairs")
	REDACTED
		lowerName := strings.ToLower(name)
		if _, blocked := blockedAttributes[lowerName]; blocked {
			return "", errors.New("paste a Cookie header, not a Set-Cookie value with attributes")
	REDACTED
		if _, duplicate := seen[lowerName]; duplicate {
			return "", errors.New("session contains duplicate cookie names")
	REDACTED
		if strings.ContainsAny(value, ";\r\n") {
			return "", errors.New("session contains an invalid cookie value")
	REDACTED
		seen[lowerName] = struct{REDACTED{REDACTED
		if isAllowedOllamaCloudSessionCookie(name) {
			normalized = append(normalized, name+"="+value)
	REDACTED
REDACTED
	if len(normalized) == 0 {
		return "", errors.New("session does not contain an allowed Ollama session cookie")
REDACTED
	return strings.Join(normalized, "; "), nil
REDACTED

func isAllowedOllamaCloudSessionCookie(name string) bool {
	switch name {
	case "wos-session", "__Secure-session", "session", "ollama_session", "__Host-ollama_session":
		return true
REDACTED
	for _, base := range []string{
		"next-auth.session-token",
		"__Secure-next-auth.session-token",
		"authjs.session-token",
		"__Secure-authjs.session-token",
REDACTED {
		if name == base {
			return true
	REDACTED
		if suffix, ok := strings.CutPrefix(name, base+"."); ok && suffix != "" {
			validShard := true
			for _, char := range suffix {
				if char < '0' || char > '9' {
					validShard = false
					break
			REDACTED
		REDACTED
			if validShard {
				return true
		REDACTED
	REDACTED
REDACTED
	return false
REDACTED

func ollamaCloudUsageConfigured(account *Account) bool {
	if account == nil || account.Extra == nil {
		return false
REDACTED
	value, ok := account.Extra[OllamaCloudUsageSessionExtraKey].(string)
	return ok && strings.TrimSpace(value) != ""
REDACTED

func ollamaCloudUsageAutoRefreshEnabled(account *Account) bool {
	if account == nil || account.Extra == nil {
		return false
REDACTED
	enabled, ok := account.Extra[OllamaCloudUsageAutoRefreshExtraKey].(bool)
	return ok && enabled
REDACTED

func decodeOllamaCloudUsageSnapshot(extra map[string]any) *OllamaCloudUsageSnapshot {
	if extra == nil {
		return nil
REDACTED
	value, ok := extra[OllamaCloudUsageSnapshotExtraKey]
	if !ok || value == nil {
		return nil
REDACTED
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
REDACTED
	var snapshot OllamaCloudUsageSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil
REDACTED
	if snapshot.Status != OllamaCloudUsageStatusOK && snapshot.Status != OllamaCloudUsageStatusUnauthorized && snapshot.Status != OllamaCloudUsageStatusFailed {
		return nil
REDACTED
	return &snapshot
REDACTED

func nextOllamaCloudUsageDelay(intervalMinutes, failureCount int, retryAfterDuration time.Duration) time.Duration {
	minimumDelay := retryAfterDuration
	base := time.Duration(intervalMinutes) * time.Minute
	if base < ollamaCloudUsageMinIntervalMinutes*time.Minute {
		base = ollamaCloudUsageMinIntervalMinutes * time.Minute
REDACTED
	if failureCount > 0 {
		shift := min(failureCount-1, 6)
		base *= time.Duration(1 << shift)
REDACTED
	if base > ollamaCloudUsageMaxDelay {
		base = ollamaCloudUsageMaxDelay
REDACTED
	if retryAfterDuration > base {
		base = retryAfterDuration
REDACTED
	jitterRange := base / 10
	if jitterRange > 5*time.Minute {
		jitterRange = 5 * time.Minute
REDACTED
	if jitterRange > 0 {
		base += time.Duration(rand.Int64N(int64(jitterRange)*2+1)) - jitterRange
REDACTED
	if base < minimumDelay {
		return minimumDelay
REDACTED
	if base < time.Minute {
		return time.Minute
REDACTED
	return base
REDACTED

func (s *OllamaCloudUsageService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
REDACTED
	return time.Now()
REDACTED

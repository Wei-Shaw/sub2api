package service

import (
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type UpstreamPriceMonitorConfig = domain.UpstreamPriceMonitorConfig
type UpstreamPriceMonitorRuntime = domain.UpstreamPriceMonitorRuntime
type UpstreamPriceMonitorRun = domain.UpstreamPriceMonitorRun
type UpstreamPriceMonitorRunPage = domain.UpstreamPriceMonitorRunPage
type UpstreamPriceEvidence = domain.UpstreamPriceEvidence
type UpstreamPriceVector = domain.UpstreamPriceVector
type UpstreamPriceUsageCounters = domain.UpstreamPriceUsageCounters
type UpstreamPriceRemoteUsageSnapshot = domain.UpstreamPriceRemoteUsageSnapshot
type UpstreamPriceBillingSnapshot = domain.UpstreamPriceBillingSnapshot

const (
	UpstreamPriceMonitorModeObserve   = domain.UpstreamPriceMonitorModeObserve
	UpstreamPriceMonitorModeAutoApply = domain.UpstreamPriceMonitorModeAutoApply

	UpstreamPriceEvidenceStatusTrusted  = domain.UpstreamPriceEvidenceStatusTrusted
	UpstreamPriceEvidenceStatusPending  = domain.UpstreamPriceEvidenceStatusPending
	UpstreamPriceEvidenceStatusMismatch = domain.UpstreamPriceEvidenceStatusMismatch
)

var upstreamPriceProbeLedgerPollInterval = 1500 * time.Millisecond

var (
	ErrUpstreamPriceMonitorUnavailable = infraerrors.ServiceUnavailable(
		"UPSTREAM_PRICE_MONITOR_UNAVAILABLE", "upstream price monitor is unavailable",
	)
	ErrUpstreamPriceMonitorInvalidConfig = infraerrors.BadRequest(
		"UPSTREAM_PRICE_MONITOR_INVALID_CONFIG", "invalid upstream price monitor configuration",
	)
	ErrUpstreamPriceMonitorRunConflict = infraerrors.Conflict(
		"UPSTREAM_PRICE_MONITOR_RUN_CONFLICT", "an upstream price monitor run is already active",
	)
	ErrUpstreamPriceCheckpointConflict = infraerrors.Conflict(
		"UPSTREAM_PRICE_CHECKPOINT_CONFLICT", "upstream price usage checkpoint changed concurrently",
	)
	ErrUpstreamPriceInsufficientRank = errors.New("insufficient independent usage observations")
	ErrUpstreamPriceRunNotApplicable = infraerrors.Conflict(
		"UPSTREAM_PRICE_RUN_NOT_APPLICABLE", "upstream price monitor run cannot be applied",
	)
	ErrUpstreamPriceSnapshotMismatch = infraerrors.Conflict(
		"UPSTREAM_PRICE_SNAPSHOT_MISMATCH", "upstream price monitor snapshot hash no longer matches",
	)
	ErrUpstreamPriceModelDiscoveryIncomplete = infraerrors.ServiceUnavailable(
		"UPSTREAM_PRICE_MODEL_DISCOVERY_INCOMPLETE", "not every production account returned a complete model catalogue",
	)
)

// UpstreamPriceMonitorRepository is deliberately independent from channel and
// display-pricing repositories. A future applier can consume a completed run
// without allowing the passive reconciler to mutate production pricing.
type UpstreamPriceMonitorRepository interface {
	GetConfig(context.Context) (*domain.UpstreamPriceMonitorConfig, error)
	UpdateConfig(context.Context, *domain.UpstreamPriceMonitorConfig) error
	GetRuntime(context.Context) (*domain.UpstreamPriceMonitorRuntime, error)
	CreateRun(context.Context, *domain.UpstreamPriceMonitorRun) error
	FinishRun(context.Context, *domain.UpstreamPriceMonitorRun) error
	GetRun(context.Context, int64) (*domain.UpstreamPriceMonitorRun, error)
	ListRuns(context.Context, int, int, ...domain.UpstreamPriceMonitorRunStatus) (*domain.UpstreamPriceMonitorRunPage, error)
	ListEvidenceByRun(context.Context, int64) ([]domain.UpstreamPriceEvidence, error)
	GetCheckpoints(context.Context, int64, []string) (map[string]domain.UpstreamPriceUsageCheckpoint, error)
	CurrentLocalUsageLogID(context.Context, []int64) (int64, error)
	AggregateLocalUsage(context.Context, []int64, map[string]int64, int64) (map[string]domain.UpstreamPriceLocalAggregate, error)
	ListMatchedObservations(context.Context, int64, string, string, time.Time, int) ([]domain.UpstreamPriceObservation, error)
	SaveReconciliation(context.Context, *domain.UpstreamPriceUsageCheckpoint, *int64, *domain.UpstreamPriceEvidence) error
	ApplyRun(context.Context, int64, string, []int64, []int64, int, int, time.Time, int64) error
	RollbackRun(context.Context, int64, string) error
	MarkApplyFailure(context.Context, int64, string) error
	ReconcileModelCatalog(context.Context, []domain.UpstreamPriceDiscoveredModel, int, bool) (int64, error)
	ListModelCatalog(context.Context) ([]domain.UpstreamPriceModelCatalogEntry, error)
	SetModelCatalogStatus(context.Context, string, domain.UpstreamPriceModelStatus) (*domain.UpstreamPriceModelCatalogEntry, error)
}

type upstreamPriceMonitorAccountReader interface {
	GetByID(context.Context, int64) (*Account, error)
}

// UpstreamPriceRemoteFetcher reads only API-key endpoints. Implementations must
// never use an x5m5x browser/JWT session.
type UpstreamPriceRemoteFetcher interface {
	FetchUsage(context.Context, *Account) (*domain.UpstreamPriceRemoteUsageSnapshot, error)
	FetchBilling(context.Context, *Account) (*domain.UpstreamPriceBillingSnapshot, error)
	FetchModels(context.Context, *Account) ([]string, error)
}

type UpstreamPriceActiveProbeRequest struct {
	Model         string
	SystemPrompt  string
	UserPrompt    string
	MaxTokens     int
	SessionID     string
	ExplicitCache bool
}

// UpstreamPriceActiveProber sends one synthetic request through the selected
// production account credential. The monitor itself performs before/after
// ledger attribution and advances the passive checkpoint so probes never
// become unexplained remote traffic.
type UpstreamPriceActiveProber interface {
	Probe(context.Context, *Account, UpstreamPriceActiveProbeRequest) error
}

type UpstreamPriceRunOptions struct {
	Trigger domain.UpstreamPriceMonitorRunTrigger
	DryRun  bool
}

type UpstreamPriceMonitorService struct {
	repo              UpstreamPriceMonitorRepository
	accounts          upstreamPriceMonitorAccountReader
	remote            UpstreamPriceRemoteFetcher
	pricePage         UpstreamPricePageFetcher
	prober            UpstreamPriceActiveProber
	concurrency       *ConcurrencyService
	probeSlotAcquirer func(context.Context, *Account) (func(), bool, error)
	now               func() time.Time
	runMu             sync.Mutex
	cacheInvalidator  interface{ InvalidatePricingCache() }
}

func NewUpstreamPriceMonitorService(
	repo UpstreamPriceMonitorRepository,
	accounts upstreamPriceMonitorAccountReader,
	remote UpstreamPriceRemoteFetcher,
) *UpstreamPriceMonitorService {
	return &UpstreamPriceMonitorService{repo: repo, accounts: accounts, remote: remote, now: time.Now}
}

func (s *UpstreamPriceMonitorService) SetActiveProber(prober UpstreamPriceActiveProber) {
	if s != nil {
		s.prober = prober
	}
}

func (s *UpstreamPriceMonitorService) SetProbeConcurrencyService(concurrency *ConcurrencyService) {
	if s != nil {
		s.concurrency = concurrency
	}
}

func (s *UpstreamPriceMonitorService) SetPricePageFetcher(fetcher UpstreamPricePageFetcher) {
	if s != nil {
		s.pricePage = fetcher
	}
}

func (s *UpstreamPriceMonitorService) SetPricingCacheInvalidator(invalidator interface{ InvalidatePricingCache() }) {
	if s != nil {
		s.cacheInvalidator = invalidator
	}
}

func (s *UpstreamPriceMonitorService) GetConfig(ctx context.Context) (*domain.UpstreamPriceMonitorConfig, error) {
	if s == nil || s.repo == nil {
		cfg := domain.DefaultUpstreamPriceMonitorConfig()
		return &cfg, nil
	}
	return s.repo.GetConfig(ctx)
}

func (s *UpstreamPriceMonitorService) UpdateConfig(ctx context.Context, cfg *domain.UpstreamPriceMonitorConfig) error {
	if s == nil || s.repo == nil {
		return ErrUpstreamPriceMonitorUnavailable
	}
	if cfg != nil && cfg.ActiveProbeEnabled && s.prober == nil {
		return ErrUpstreamPriceMonitorInvalidConfig
	}
	if cfg != nil {
		catalog, err := s.repo.ListModelCatalog(ctx)
		if err != nil {
			return err
		}
		cfg.DomesticModels = cfg.DomesticModels[:0]
		for _, model := range catalog {
			if model.Status == domain.UpstreamPriceModelStatusManaged {
				cfg.DomesticModels = append(cfg.DomesticModels, model.ModelName)
			}
		}
	}
	if err := normalizeAndValidateUpstreamPriceMonitorConfig(cfg); err != nil {
		return err
	}
	if err := s.validateUpstreamPriceMonitorAccounts(ctx, cfg.AccountIDs); err != nil {
		return err
	}
	return s.repo.UpdateConfig(ctx, cfg)
}

func (s *UpstreamPriceMonitorService) validateUpstreamPriceMonitorAccounts(ctx context.Context, accountIDs []int64) error {
	_, err := s.loadDistinctUpstreamPriceMonitorAccounts(ctx, accountIDs)
	return err
}

func (s *UpstreamPriceMonitorService) loadDistinctUpstreamPriceMonitorAccounts(ctx context.Context, accountIDs []int64) (map[int64]*Account, error) {
	if len(accountIDs) == 0 {
		return map[int64]*Account{}, nil
	}
	if s.accounts == nil {
		return nil, ErrUpstreamPriceMonitorUnavailable
	}
	seenLedgers := make(map[string]int64, len(accountIDs))
	loaded := make(map[int64]*Account, len(accountIDs))
	for _, accountID := range accountIDs {
		account, err := s.accounts.GetByID(ctx, accountID)
		if err != nil || account == nil || account.Type != AccountTypeAPIKey || strings.TrimSpace(account.GetCredential("api_key")) == "" {
			return nil, fmt.Errorf("%w: account %d is not a usable API-key account", ErrUpstreamPriceMonitorInvalidConfig, accountID)
		}
		ledgerHash := UpstreamPriceCredentialLedgerHash(account)
		if previousID, duplicate := seenLedgers[ledgerHash]; duplicate {
			return nil, fmt.Errorf("%w: accounts %d and %d share one upstream usage ledger", ErrUpstreamPriceMonitorInvalidConfig, previousID, accountID)
		}
		seenLedgers[ledgerHash] = accountID
		loaded[accountID] = account
	}
	return loaded, nil
}

func (s *UpstreamPriceMonitorService) GetRuntime(ctx context.Context) (*domain.UpstreamPriceMonitorRuntime, error) {
	if s == nil || s.repo == nil {
		return nil, ErrUpstreamPriceMonitorUnavailable
	}
	return s.repo.GetRuntime(ctx)
}

func (s *UpstreamPriceMonitorService) ListModelCatalog(ctx context.Context) ([]domain.UpstreamPriceModelCatalogEntry, error) {
	if s == nil || s.repo == nil {
		return nil, ErrUpstreamPriceMonitorUnavailable
	}
	return s.repo.ListModelCatalog(ctx)
}

func (s *UpstreamPriceMonitorService) SetModelCatalogStatus(
	ctx context.Context,
	model string,
	status domain.UpstreamPriceModelStatus,
) (*domain.UpstreamPriceModelCatalogEntry, error) {
	if s == nil || s.repo == nil {
		return nil, ErrUpstreamPriceMonitorUnavailable
	}
	switch status {
	case domain.UpstreamPriceModelStatusManaged, domain.UpstreamPriceModelStatusDiscovered,
		domain.UpstreamPriceModelStatusIgnored, domain.UpstreamPriceModelStatusRetired:
	default:
		return nil, ErrUpstreamPriceMonitorInvalidConfig
	}
	return s.repo.SetModelCatalogStatus(ctx, strings.TrimSpace(model), status)
}

func (s *UpstreamPriceMonitorService) DiscoverModelCatalog(ctx context.Context) ([]domain.UpstreamPriceModelCatalogEntry, error) {
	if s == nil || s.repo == nil || s.accounts == nil || s.remote == nil {
		return nil, ErrUpstreamPriceMonitorUnavailable
	}
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	accounts, err := s.loadDistinctUpstreamPriceMonitorAccounts(ctx, cfg.AccountIDs)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, ErrUpstreamPriceMonitorInvalidConfig
	}
	if _, complete, _, err := s.refreshUpstreamPriceModelCatalog(ctx, accounts); err != nil {
		return nil, err
	} else if !complete {
		return nil, ErrUpstreamPriceModelDiscoveryIncomplete
	}
	return s.repo.ListModelCatalog(ctx)
}

func (s *UpstreamPriceMonitorService) GetRun(ctx context.Context, id int64) (*domain.UpstreamPriceMonitorRun, error) {
	if s == nil || s.repo == nil || id <= 0 {
		return nil, ErrUpstreamPriceMonitorUnavailable
	}
	return s.repo.GetRun(ctx, id)
}

func (s *UpstreamPriceMonitorService) ListRuns(ctx context.Context, limit, offset int, statuses ...domain.UpstreamPriceMonitorRunStatus) (*domain.UpstreamPriceMonitorRunPage, error) {
	if s == nil || s.repo == nil {
		return nil, ErrUpstreamPriceMonitorUnavailable
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	if len(statuses) > 0 && statuses[0] != "" {
		switch statuses[0] {
		case domain.UpstreamPriceMonitorRunStatusRunning, domain.UpstreamPriceMonitorRunStatusCompleted,
			domain.UpstreamPriceMonitorRunStatusPartial, domain.UpstreamPriceMonitorRunStatusFailed:
		default:
			return nil, ErrUpstreamPriceMonitorInvalidConfig
		}
	}
	return s.repo.ListRuns(ctx, limit, offset, statuses...)
}

func (s *UpstreamPriceMonitorService) ListRunEvidence(ctx context.Context, runID int64) ([]domain.UpstreamPriceEvidence, error) {
	if s == nil || s.repo == nil || runID <= 0 {
		return nil, ErrUpstreamPriceMonitorUnavailable
	}
	items, err := s.repo.ListEvidenceByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.UpstreamPriceEvidence, 0, len(items))
	for _, item := range items {
		if item.Source == domain.UpstreamPriceEvidenceSourceActiveProbe && strings.Contains(item.ContextKey, "-sample-") {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *UpstreamPriceMonitorService) ApplyRun(ctx context.Context, runID int64, snapshotHash string) (*domain.UpstreamPriceMonitorRun, error) {
	if s == nil || s.repo == nil || runID <= 0 || strings.TrimSpace(snapshotHash) == "" {
		return nil, ErrUpstreamPriceRunNotApplicable
	}
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	scope, ok := ReadUpstreamPriceRunApplyScope(run)
	if !ok {
		return nil, ErrUpstreamPriceRunNotApplicable
	}
	currentAccounts, err := s.loadDistinctUpstreamPriceMonitorAccounts(ctx, scope.AccountIDs)
	if err != nil {
		return nil, err
	}
	for _, accountID := range scope.AccountIDs {
		if UpstreamPriceCredentialLedgerHash(currentAccounts[accountID]) != scope.AccountLedgerHashes[accountID] ||
			UpstreamPriceAccountIdentityHash(currentAccounts[accountID]) != scope.AccountIdentityHashes[accountID] {
			return nil, ErrUpstreamPriceSnapshotMismatch
		}
	}
	if err := s.repo.ApplyRun(ctx, runID, strings.TrimSpace(snapshotHash), scope.ChannelIDs, scope.AccountIDs,
		scope.DisplayMultiplierDecimals, scope.MaxAgeMinutes, scope.ConfigUpdatedAt, scope.ModelCatalogRevision); err != nil {
		return nil, err
	}
	applied, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	appliedModels, _ := upstreamPriceSummaryInt(applied.Summary["applied_models"])
	if appliedModels > 0 && s.cacheInvalidator != nil {
		s.cacheInvalidator.InvalidatePricingCache()
	}
	return applied, nil
}

func (s *UpstreamPriceMonitorService) RollbackRun(ctx context.Context, runID int64, snapshotHash string) (*domain.UpstreamPriceMonitorRun, error) {
	if s == nil || s.repo == nil || runID <= 0 || strings.TrimSpace(snapshotHash) == "" {
		return nil, ErrUpstreamPriceRunNotApplicable
	}
	if err := s.repo.RollbackRun(ctx, runID, strings.TrimSpace(snapshotHash)); err != nil {
		return nil, err
	}
	if s.cacheInvalidator != nil {
		s.cacheInvalidator.InvalidatePricingCache()
	}
	return s.repo.GetRun(ctx, runID)
}

func normalizeAndValidateUpstreamPriceMonitorConfig(cfg *domain.UpstreamPriceMonitorConfig) error {
	if cfg == nil {
		return ErrUpstreamPriceMonitorInvalidConfig
	}
	if cfg.Mode == "" {
		cfg.Mode = domain.UpstreamPriceMonitorModeObserve
	}
	if cfg.Mode != domain.UpstreamPriceMonitorModeObserve && cfg.Mode != domain.UpstreamPriceMonitorModeAutoApply {
		return ErrUpstreamPriceMonitorInvalidConfig
	}
	if cfg.IntervalMinutes < 5 || cfg.IntervalMinutes > 1440 || cfg.Markup < 1 || cfg.Markup > 100 ||
		math.IsNaN(cfg.Markup) || math.IsInf(cfg.Markup, 0) || cfg.DisplayMultiplierDecimals < 0 ||
		cfg.DisplayMultiplierDecimals > 6 || cfg.PassiveSampleMaxAgeMinutes < 15 || cfg.PassiveSampleMaxAgeMinutes > 10080 {
		return ErrUpstreamPriceMonitorInvalidConfig
	}
	if cfg.ActiveProbeEnabled && cfg.IntervalMinutes < 15 {
		return ErrUpstreamPriceMonitorInvalidConfig
	}
	cfg.AccountIDs = uniquePositiveInt64s(cfg.AccountIDs)
	cfg.ChannelIDs = uniquePositiveInt64s(cfg.ChannelIDs)
	if cfg.Mode == domain.UpstreamPriceMonitorModeAutoApply && (len(cfg.AccountIDs) == 0 || len(cfg.ChannelIDs) == 0) {
		return ErrUpstreamPriceMonitorInvalidConfig
	}
	models, err := normalizeDomesticModelAllowlist(cfg.DomesticModels)
	if err != nil {
		return err
	}
	cfg.DomesticModels = models
	return nil
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func normalizeDomesticModelAllowlist(models []string) ([]string, error) {
	allowed := make(map[string]string, len(domain.DefaultX5M5XDomesticModels))
	for _, model := range domain.DefaultX5M5XDomesticModels {
		allowed[strings.ToLower(model)] = model
	}
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, raw := range models {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || len(trimmed) > 255 || strings.ContainsAny(trimmed, " \t\r\n,") {
			return nil, ErrUpstreamPriceMonitorInvalidConfig
		}
		canonical := trimmed
		if known, ok := allowed[strings.ToLower(trimmed)]; ok {
			canonical = known
		}
		key := strings.ToLower(canonical)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, canonical)
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out, nil
}

type upstreamPriceModelDiscoveryResult struct {
	accountID int64
	models    []string
	err       error
}

func (s *UpstreamPriceMonitorService) refreshUpstreamPriceModelCatalog(
	ctx context.Context,
	accounts map[int64]*Account,
) (map[int64]map[string]struct{}, bool, int64, error) {
	availability := make(map[int64]map[string]struct{}, len(accounts))
	if len(accounts) == 0 {
		return availability, false, 0, nil
	}
	results := make(chan upstreamPriceModelDiscoveryResult, len(accounts))
	var wg sync.WaitGroup
	for accountID, account := range accounts {
		_ = account.GetHeaderOverrides()
		wg.Add(1)
		go func(id int64, value *Account) {
			defer wg.Done()
			models, err := s.remote.FetchModels(ctx, value)
			results <- upstreamPriceModelDiscoveryResult{accountID: id, models: models, err: err}
		}(accountID, account)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	type observedModel struct {
		name     string
		accounts int
	}
	observed := make(map[string]observedModel)
	complete := true
	for result := range results {
		if result.err != nil {
			complete = false
			continue
		}
		set := make(map[string]struct{}, len(result.models))
		for _, model := range result.models {
			key := strings.ToLower(strings.TrimSpace(model))
			if key == "" {
				continue
			}
			if _, duplicate := set[key]; duplicate {
				continue
			}
			set[key] = struct{}{}
			item := observed[key]
			if item.name == "" {
				item.name = model
			}
			item.accounts++
			observed[key] = item
		}
		availability[result.accountID] = set
	}
	discovered := make([]domain.UpstreamPriceDiscoveredModel, 0, len(observed))
	for _, item := range observed {
		discovered = append(discovered, domain.UpstreamPriceDiscoveredModel{
			ModelName: item.name, SeenAccountCount: item.accounts,
			DomesticCandidate: isLikelyDomesticUpstreamModel(item.name),
		})
	}
	revision, err := s.repo.ReconcileModelCatalog(ctx, discovered, len(accounts), complete)
	if err != nil {
		return availability, complete, 0, err
	}
	return availability, complete, revision, nil
}

func isLikelyDomesticUpstreamModel(model string) bool {
	value := strings.ToLower(strings.TrimSpace(model))
	for _, known := range domain.DefaultX5M5XDomesticModels {
		if strings.EqualFold(value, known) {
			return true
		}
	}
	for _, prefix := range []string{
		"deepseek", "kimi", "moonshot", "glm", "minimax", "qwen", "mimo", "hunyuan", "hy-", "hy3",
		"baichuan", "yi-", "doubao", "seed-", "step-", "abab", "ernie", "wenxin",
	} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func intersectUpstreamPriceModels(models []string, available map[string]struct{}) []string {
	if available == nil {
		return append([]string(nil), models...)
	}
	out := make([]string, 0, len(models))
	for _, model := range models {
		if _, ok := available[strings.ToLower(model)]; ok {
			out = append(out, model)
		}
	}
	return out
}

// RunOnce records a durable run even on partial failure. Scheduled callers
// should check config.Enabled before invoking; manual dry-runs remain available
// while the scheduler is disabled.
func (s *UpstreamPriceMonitorService) RunOnce(ctx context.Context, options UpstreamPriceRunOptions) (*domain.UpstreamPriceMonitorRun, error) {
	if s == nil || s.repo == nil || s.accounts == nil || s.remote == nil {
		return nil, ErrUpstreamPriceMonitorUnavailable
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	ctx, cancelRun := context.WithTimeout(ctx, 20*time.Minute)
	defer cancelRun()

	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get upstream price monitor config: %w", err)
	}
	if options.Trigger == "" {
		options.Trigger = domain.UpstreamPriceMonitorRunTriggerManual
	}
	if options.Trigger == domain.UpstreamPriceMonitorRunTriggerScheduled && !cfg.Enabled {
		return nil, nil
	}
	if err := normalizeAndValidateUpstreamPriceMonitorConfig(cfg); err != nil {
		return nil, err
	}
	if cfg.ActiveProbeEnabled && s.prober == nil {
		return nil, ErrUpstreamPriceMonitorInvalidConfig
	}
	run := &domain.UpstreamPriceMonitorRun{
		Trigger:   options.Trigger,
		Status:    domain.UpstreamPriceMonitorRunStatusRunning,
		Mode:      cfg.Mode,
		DryRun:    options.DryRun || cfg.Mode == domain.UpstreamPriceMonitorModeObserve,
		StartedAt: s.now().UTC(),
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	loadedAccounts, accountValidationErr := s.loadDistinctUpstreamPriceMonitorAccounts(ctx, cfg.AccountIDs)
	if accountValidationErr != nil {
		finished := s.now().UTC()
		run.Status = domain.UpstreamPriceMonitorRunStatusFailed
		run.FinishedAt = &finished
		run.Error = accountValidationErr.Error()
		run.Summary = map[string]any{"accounts": len(cfg.AccountIDs), "models": len(cfg.DomesticModels), "observe_only": true}
		finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		finishErr := s.repo.FinishRun(finalizeCtx, run)
		cancel()
		if finishErr != nil {
			return run, errors.Join(accountValidationErr, finishErr)
		}
		return run, accountValidationErr
	}

	var runErrors []string
	modelAvailability, _, modelCatalogRevision, discoveryErr := s.refreshUpstreamPriceModelCatalog(ctx, loadedAccounts)
	if discoveryErr != nil {
		runErrors = append(runErrors, "refresh upstream model catalogue: "+discoveryErr.Error())
	} else if refreshedConfig, refreshErr := s.repo.GetConfig(ctx); refreshErr != nil {
		runErrors = append(runErrors, "reload model probe scope: "+refreshErr.Error())
	} else if validateErr := normalizeAndValidateUpstreamPriceMonitorConfig(refreshedConfig); validateErr != nil {
		runErrors = append(runErrors, "validate discovered model probe scope: "+validateErr.Error())
	} else if !sameUpstreamPriceNonModelConfig(cfg, refreshedConfig) {
		runErrors = append(runErrors, "monitor configuration changed while refreshing the upstream model catalogue")
	} else {
		cfg.DomesticModels = refreshedConfig.DomesticModels
		cfg.UpdatedAt = refreshedConfig.UpdatedAt
	}
	if modelCatalogRevision <= 0 {
		runErrors = append(runErrors, "model catalogue has no scan revision")
	}
	type accountRunResult struct {
		matched, mismatched, probed int
		probeCost                   float64
		errors                      []string
	}
	processAccount := func(accountID int64) accountRunResult {
		result := accountRunResult{}
		account := loadedAccounts[accountID]
		if account == nil {
			result.errors = append(result.errors, fmt.Sprintf("account %d: validated account snapshot is unavailable", accountID))
			return result
		}
		accountCfg := *cfg
		availableModels, availabilityKnown := modelAvailability[accountID]
		accountCfg.DomesticModels = intersectUpstreamPriceModels(cfg.DomesticModels, availableModels)
		usage, fetchErr := s.remote.FetchUsage(ctx, account)
		if fetchErr != nil {
			result.errors = append(result.errors, fmt.Sprintf("account %d: usage: %v", accountID, fetchErr))
			return result
		}
		billing, billingErr := s.remote.FetchBilling(ctx, account)
		if billingErr != nil {
			result.errors = append(result.errors, fmt.Sprintf("account %d: billing context: %v", accountID, billingErr))
		}
		matched, mismatched, reconcileErr := s.reconcileAccount(ctx, run.ID, &accountCfg, account, usage, billing)
		result.matched, result.mismatched = matched, mismatched
		if reconcileErr != nil {
			result.errors = append(result.errors, fmt.Sprintf("account %d: reconcile: %v", accountID, reconcileErr))
		}
		if cfg.ActiveProbeEnabled && s.prober != nil && billing != nil {
			runEvidence, listErr := s.repo.ListEvidenceByRun(ctx, run.ID)
			if listErr != nil {
				result.errors = append(result.errors, fmt.Sprintf("account %d: list passive evidence: %v", accountID, listErr))
			} else {
				activeModels := accountCfg.DomesticModels
				if !availabilityKnown {
					activeModels = nil
				}
				missing := missingUpstreamPriceProbeModels(runEvidence, account.ID, activeModels)
				probed, probeCost, probeErr := s.probeMissingUpstreamPriceModels(ctx, run.ID, &accountCfg, account, billing, missing)
				result.probed, result.probeCost = probed, probeCost
				if probeErr != nil {
					result.errors = append(result.errors, fmt.Sprintf("account %d: active probe: %v", accountID, probeErr))
				}
			}
		}
		return result
	}
	accountWorkers := 3
	if accountWorkers > len(cfg.AccountIDs) {
		accountWorkers = len(cfg.AccountIDs)
	}
	accountJobs := make(chan int64)
	accountResults := make(chan accountRunResult, len(cfg.AccountIDs))
	var accountWG sync.WaitGroup
	for worker := 0; worker < accountWorkers; worker++ {
		accountWG.Add(1)
		go func() {
			defer accountWG.Done()
			for accountID := range accountJobs {
				accountResults <- processAccount(accountID)
			}
		}()
	}
	go func() {
		defer close(accountJobs)
		for _, accountID := range cfg.AccountIDs {
			select {
			case <-ctx.Done():
				return
			case accountJobs <- accountID:
			}
		}
	}()
	go func() {
		accountWG.Wait()
		close(accountResults)
	}()
	processedAccounts := 0
	for result := range accountResults {
		processedAccounts++
		run.MatchedModels += result.matched
		run.MismatchedModels += result.mismatched
		run.ProbedModels += result.probed
		run.ProbeCost += result.probeCost
		runErrors = append(runErrors, result.errors...)
	}
	if processedAccounts != len(cfg.AccountIDs) {
		runErrors = append(runErrors, "account processing stopped before every configured production account completed")
	}
	if s.pricePage != nil {
		prices, pageErr := s.pricePage.FetchPerRequestPrices(ctx)
		if pageErr != nil {
			runErrors = append(runErrors, "per-request pricing page: "+pageErr.Error())
		} else {
			allowedModels := make(map[string]struct{}, len(cfg.DomesticModels))
			for _, model := range cfg.DomesticModels {
				allowedModels[strings.ToLower(model)] = struct{}{}
			}
			for model, price := range prices {
				if _, allowed := allowedModels[strings.ToLower(strings.TrimSpace(model))]; !allowed {
					continue
				}
				evidence := &domain.UpstreamPriceEvidence{
					RunID: run.ID, AccountID: 0, ModelName: model, BillingMode: DisplayBillingModePerRequest,
					Status: domain.UpstreamPriceEvidenceStatusTrusted, Source: domain.UpstreamPriceEvidenceSourcePricePage,
					ReconciliationStatus: domain.UpstreamPriceReconciliationMatched, ContextKey: "price-page",
					ObservedAt: s.now().UTC(), SampleCount: 1, Prices: price,
					SuggestedPrices: multiplyUpstreamPriceVector(price, cfg.Markup),
				}
				if saveErr := s.repo.SaveReconciliation(ctx, nil, nil, evidence); saveErr != nil {
					runErrors = append(runErrors, "per-request "+model+": "+saveErr.Error())
					continue
				}
				run.MatchedModels++
			}
		}
	}

	finalizeCtx, cancelFinalize := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelFinalize()
	evidence, listErr := s.repo.ListEvidenceByRun(finalizeCtx, run.ID)
	if listErr != nil {
		runErrors = append(runErrors, "list evidence: "+listErr.Error())
	} else {
		run.SnapshotHash = upstreamPriceEvidenceHash(evidence)
		evidenceProbeCost := 0.0
		for _, item := range evidence {
			if item.Source == domain.UpstreamPriceEvidenceSourceActiveProbe {
				evidenceProbeCost += math.Max(0, item.RemoteDelta.ActualCost)
			}
		}
		run.ProbeCost = evidenceProbeCost
	}
	finished := s.now().UTC()
	run.FinishedAt = &finished
	switch {
	case len(runErrors) == 0:
		run.Status = domain.UpstreamPriceMonitorRunStatusCompleted
	case len(evidence) > 0:
		run.Status = domain.UpstreamPriceMonitorRunStatusPartial
	default:
		run.Status = domain.UpstreamPriceMonitorRunStatusFailed
	}
	run.Error = strings.Join(runErrors, "; ")
	accountLedgerHashes := make(map[string]string, len(loadedAccounts))
	accountIdentityHashes := make(map[string]string, len(loadedAccounts))
	for accountID, account := range loadedAccounts {
		key := strconv.FormatInt(accountID, 10)
		accountLedgerHashes[key] = UpstreamPriceCredentialLedgerHash(account)
		accountIdentityHashes[key] = UpstreamPriceAccountIdentityHash(account)
	}
	run.Summary = map[string]any{
		"accounts": len(cfg.AccountIDs), "models": len(cfg.DomesticModels), "observe_only": run.DryRun,
		"account_ids":                 append([]int64(nil), cfg.AccountIDs...),
		"channel_ids":                 append([]int64(nil), cfg.ChannelIDs...),
		"display_multiplier_decimals": cfg.DisplayMultiplierDecimals,
		"snapshot_max_age_minutes":    cfg.PassiveSampleMaxAgeMinutes,
		"config_updated_at":           cfg.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"account_ledger_hashes":       accountLedgerHashes,
		"account_identity_hashes":     accountIdentityHashes,
		"model_catalog_revision":      modelCatalogRevision,
	}
	if finishErr := s.repo.FinishRun(finalizeCtx, run); finishErr != nil {
		return run, fmt.Errorf("finish upstream price monitor run: %w", finishErr)
	}
	if run.Status == domain.UpstreamPriceMonitorRunStatusCompleted && !run.DryRun && cfg.Mode == domain.UpstreamPriceMonitorModeAutoApply {
		applied, applyErr := s.ApplyRun(ctx, run.ID, run.SnapshotHash)
		if applyErr != nil {
			message := "automatic price apply failed: " + applyErr.Error()
			failureCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.repo.MarkApplyFailure(failureCtx, run.ID, message)
			cancel()
			run.Status = domain.UpstreamPriceMonitorRunStatusPartial
			run.Error = message
			return run, applyErr
		}
		return applied, nil
	}
	if run.Status == domain.UpstreamPriceMonitorRunStatusFailed {
		return run, errors.New(run.Error)
	}
	return run, nil
}

func sameUpstreamPriceNonModelConfig(a, b *domain.UpstreamPriceMonitorConfig) bool {
	if a == nil || b == nil || a.Enabled != b.Enabled || a.Mode != b.Mode ||
		a.IntervalMinutes != b.IntervalMinutes || a.Markup != b.Markup ||
		a.DisplayMultiplierDecimals != b.DisplayMultiplierDecimals ||
		a.PassiveSampleMaxAgeMinutes != b.PassiveSampleMaxAgeMinutes ||
		a.ActiveProbeEnabled != b.ActiveProbeEnabled || len(a.AccountIDs) != len(b.AccountIDs) ||
		len(a.ChannelIDs) != len(b.ChannelIDs) {
		return false
	}
	for i := range a.AccountIDs {
		if a.AccountIDs[i] != b.AccountIDs[i] {
			return false
		}
	}
	for i := range a.ChannelIDs {
		if a.ChannelIDs[i] != b.ChannelIDs[i] {
			return false
		}
	}
	return true
}

func (s *UpstreamPriceMonitorService) reconcileAccount(
	ctx context.Context,
	runID int64,
	cfg *domain.UpstreamPriceMonitorConfig,
	account *Account,
	usage *domain.UpstreamPriceRemoteUsageSnapshot,
	billing *domain.UpstreamPriceBillingSnapshot,
) (int, int, error) {
	if usage == nil || strings.TrimSpace(usage.LedgerDate) == "" {
		return 0, 0, errors.New("remote usage snapshot has no fixed ledger_date")
	}
	usage.AccountID = account.ID
	identityHash := UpstreamPriceAccountIdentityHash(account)
	billingHash, contextKey := upstreamPriceBillingContext(billing)
	identityKey := identityHash
	if len(identityKey) > 16 {
		identityKey = identityKey[:16]
	}
	contextKey += ";ledger=" + usage.LedgerDate + ";identity=" + identityKey
	checkpoints, err := s.repo.GetCheckpoints(ctx, account.ID, cfg.DomesticModels)
	if err != nil {
		return 0, 0, err
	}
	highWatermark, err := s.repo.CurrentLocalUsageLogID(ctx, []int64{account.ID})
	if err != nil {
		return 0, 0, err
	}
	after := make(map[string]int64, len(cfg.DomesticModels))
	for _, model := range cfg.DomesticModels {
		cp, ok := checkpointForModel(checkpoints, model)
		if !ok || cp.AccountIdentityHash != identityHash || cp.LedgerDate != usage.LedgerDate {
			after[model] = highWatermark
		} else {
			after[model] = cp.LocalUsageLogID
		}
	}
	locals, err := s.repo.AggregateLocalUsage(ctx, []int64{account.ID}, after, highWatermark)
	if err != nil {
		return 0, 0, err
	}

	remoteModels := canonicalUsageMap(usage.Models)
	matched, mismatched := 0, 0
	var errs []string
	for _, model := range cfg.DomesticModels {
		currentRemote := remoteModels[strings.ToLower(model)]
		previous, exists := checkpointForModel(checkpoints, model)
		local := locals[strings.ToLower(model)]
		evidence := &domain.UpstreamPriceEvidence{
			RunID: runID, AccountID: account.ID, ModelName: model, BillingMode: DisplayBillingModeToken,
			Status: domain.UpstreamPriceEvidenceStatusPending, Source: domain.UpstreamPriceEvidenceSourceUserRequest,
			ContextKey: contextKey, ObservedAt: usage.CapturedAt.UTC(), LocalDelta: local.Counters,
		}
		checkpoint := &domain.UpstreamPriceUsageCheckpoint{
			AccountID: account.ID, ModelName: model, AccountIdentityHash: identityHash, Remote: currentRemote,
			LedgerDate: usage.LedgerDate, BillingContextHash: billingHash, LocalUsageLogID: highWatermark,
			CapturedAt: usage.CapturedAt.UTC(),
		}

		if !exists || previous.AccountIdentityHash != identityHash || previous.LedgerDate != usage.LedgerDate {
			evidence.ReconciliationStatus = domain.UpstreamPriceReconciliationBaseline
			evidence.LastError = "checkpoint initialized; no price inference from pre-baseline traffic"
			var expectedRevision *int64
			if exists {
				revision := previous.Revision
				expectedRevision = &revision
			}
			if err := s.repo.SaveReconciliation(ctx, checkpoint, expectedRevision, evidence); err != nil {
				errs = append(errs, model+": "+err.Error())
			}
			continue
		}
		if previous.ActiveProbePending {
			evidence.Source = domain.UpstreamPriceEvidenceSourceActiveProbe
			evidence.Status = domain.UpstreamPriceEvidenceStatusUnobservable
			evidence.ContextKey = "active-recovery-" + strconv.FormatInt(previous.Revision, 10)
			if previous.ActiveProbeStartedAt != nil && usage.CapturedAt.Sub(*previous.ActiveProbeStartedAt) < 2*time.Minute {
				evidence.ReconciliationStatus = domain.UpstreamPriceReconciliationNoActivity
				evidence.LastError = "active probe settlement is still inside its grace period; checkpoint left pending"
				if err := s.repo.SaveReconciliation(ctx, nil, nil, evidence); err != nil {
					errs = append(errs, model+": "+err.Error())
				}
				continue
			}
			evidence.ReconciliationStatus = domain.UpstreamPriceReconciliationRemoteReset
			evidence.LastError = "recovered an interrupted active probe by re-baselining its remote and local ledgers"
			if recoveredDelta, ok := subtractUpstreamPriceCounters(previous.Remote, currentRemote); ok {
				evidence.RemoteDelta = recoveredDelta
			}
			expectedRevision := previous.Revision
			if err := s.repo.SaveReconciliation(ctx, checkpoint, &expectedRevision, evidence); err != nil {
				errs = append(errs, model+": "+err.Error())
			}
			continue
		}

		expectedRevision := previous.Revision
		remoteDelta, deltaOK := subtractUpstreamPriceCounters(previous.Remote, currentRemote)
		evidence.RemoteDelta = remoteDelta
		if !deltaOK {
			evidence.Status = domain.UpstreamPriceEvidenceStatusMismatch
			evidence.ReconciliationStatus = domain.UpstreamPriceReconciliationRemoteReset
			evidence.LastError = "remote cumulative counters moved backwards; checkpoint re-baselined"
			mismatched++
			if err := s.repo.SaveReconciliation(ctx, checkpoint, &expectedRevision, evidence); err != nil {
				errs = append(errs, model+": "+err.Error())
			}
			continue
		}
		if previous.BillingContextHash != billingHash {
			evidence.ReconciliationStatus = domain.UpstreamPriceReconciliationMixedContext
			evidence.LastError = "billing multiplier or peak context changed inside the sample window"
			if err := s.repo.SaveReconciliation(ctx, checkpoint, &expectedRevision, evidence); err != nil {
				errs = append(errs, model+": "+err.Error())
			}
			continue
		}
		if local.HasSpecialContext || local.DistinctServiceTiers > 1 {
			evidence.ReconciliationStatus = domain.UpstreamPriceReconciliationMixedContext
			evidence.LastError = "priority/flex/long-context traffic cannot infer the default token price"
			if err := s.repo.SaveReconciliation(ctx, checkpoint, &expectedRevision, evidence); err != nil {
				errs = append(errs, model+": "+err.Error())
			}
			continue
		}
		if upstreamPriceCountersEmpty(remoteDelta) && upstreamPriceCountersEmpty(local.Counters) {
			evidence.ReconciliationStatus = domain.UpstreamPriceReconciliationNoActivity
			if err := s.repo.SaveReconciliation(ctx, checkpoint, &expectedRevision, evidence); err != nil {
				errs = append(errs, model+": "+err.Error())
			}
			continue
		}
		if !equalUpstreamPriceAccountingCounters(remoteDelta, local.Counters) {
			evidence.Status = domain.UpstreamPriceEvidenceStatusMismatch
			evidence.ReconciliationStatus = domain.UpstreamPriceReconciliationMismatch
			evidence.LastError = "local request/token totals do not close against the production key ledger"
			mismatched++
			var saveCheckpoint *domain.UpstreamPriceUsageCheckpoint
			var saveRevision *int64
			if usage.CapturedAt.Sub(previous.CapturedAt) >= 30*time.Minute {
				saveCheckpoint = checkpoint
				revision := previous.Revision
				saveRevision = &revision
				evidence.LastError += "; polluted window exceeded 30 minutes and was explicitly re-baselined"
			}
			if err := s.repo.SaveReconciliation(ctx, saveCheckpoint, saveRevision, evidence); err != nil {
				errs = append(errs, model+": "+err.Error())
			}
			continue
		}

		evidence.ReconciliationStatus = domain.UpstreamPriceReconciliationMatched
		cutoff := usage.CapturedAt.Add(-time.Duration(cfg.PassiveSampleMaxAgeMinutes) * time.Minute)
		observations, obsErr := s.repo.ListMatchedObservations(ctx, account.ID, model, contextKey, cutoff, 32)
		if obsErr != nil {
			errs = append(errs, model+": observations: "+obsErr.Error())
		} else {
			observations = append(observations, domain.UpstreamPriceObservation{
				InputTokens: remoteDelta.InputTokens, OutputTokens: remoteDelta.OutputTokens,
				CacheCreationTokens: remoteDelta.CacheCreationTokens, CacheReadTokens: remoteDelta.CacheReadTokens,
				ActualCost: remoteDelta.ActualCost,
			})
			prices, sampleCount, solveErr := SolveUpstreamTokenPrices(observations)
			evidence.SampleCount = sampleCount
			if solveErr == nil && billing != nil {
				evidence.Status = domain.UpstreamPriceEvidenceStatusTrusted
				evidence.Prices = prices
				evidence.SuggestedPrices = multiplyUpstreamPriceVector(prices, cfg.Markup)
			} else if solveErr != nil {
				evidence.LastError = solveErr.Error()
			} else {
				evidence.LastError = "billing context unavailable; matched sample retained but not trusted for auto-apply"
			}
		}
		matched++
		if err := s.repo.SaveReconciliation(ctx, checkpoint, &expectedRevision, evidence); err != nil {
			errs = append(errs, model+": "+err.Error())
		}
	}
	return matched, mismatched, errors.Join(stringErrors(errs)...)
}

type upstreamPriceActiveProbeResult struct {
	probed bool
	cost   float64
	err    error
}

func missingUpstreamPriceProbeModels(
	evidence []domain.UpstreamPriceEvidence,
	accountID int64,
	models []string,
) []string {
	trusted := make(map[string]struct{}, len(models))
	for _, item := range evidence {
		if item.AccountID == accountID && item.BillingMode == DisplayBillingModeToken &&
			item.Status == domain.UpstreamPriceEvidenceStatusTrusted &&
			item.Prices.InputPerMillion != nil && item.Prices.OutputPerMillion != nil &&
			item.Prices.CacheWritePerMillion != nil && item.Prices.CacheReadPerMillion != nil {
			trusted[strings.ToLower(item.ModelName)] = struct{}{}
		}
	}
	missing := make([]string, 0, len(models))
	for _, model := range models {
		if _, ok := trusted[strings.ToLower(model)]; !ok {
			missing = append(missing, model)
		}
	}
	return missing
}

func (s *UpstreamPriceMonitorService) probeMissingUpstreamPriceModels(
	ctx context.Context,
	runID int64,
	cfg *domain.UpstreamPriceMonitorConfig,
	account *Account,
	billing *domain.UpstreamPriceBillingSnapshot,
	models []string,
) (int, float64, error) {
	if len(models) == 0 || s.prober == nil {
		return 0, 0, nil
	}
	if len(models) > 1 {
		offset := int((runID * 7) % int64(len(models)))
		rotated := append([]string(nil), models[offset:]...)
		models = append(rotated, models[:offset]...)
	}
	// Resolve and cache normalized overrides before sharing the immutable
	// account configuration between probe workers.
	_ = account.GetHeaderOverrides()
	workerCount := 1
	if workerCount > len(models) {
		workerCount = len(models)
	}
	jobs := make(chan string)
	results := make(chan upstreamPriceActiveProbeResult, len(models))
	var workers sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for model := range jobs {
				probed, cost, err := s.probeOneUpstreamPriceModel(ctx, runID, cfg, account, billing, model)
				results <- upstreamPriceActiveProbeResult{probed: probed, cost: cost, err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, model := range models {
			select {
			case <-ctx.Done():
				return
			case jobs <- model:
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	probedModels := 0
	probeCost := 0.0
	var probeErrors []error
	for result := range results {
		if result.probed {
			probedModels++
		}
		probeCost += result.cost
		if result.err != nil {
			probeErrors = append(probeErrors, result.err)
		}
	}
	return probedModels, probeCost, errors.Join(probeErrors...)
}

func (s *UpstreamPriceMonitorService) probeOneUpstreamPriceModel(
	ctx context.Context,
	runID int64,
	cfg *domain.UpstreamPriceMonitorConfig,
	account *Account,
	billing *domain.UpstreamPriceBillingSnapshot,
	model string,
) (bool, float64, error) {
	checkpoints, err := s.repo.GetCheckpoints(ctx, account.ID, []string{model})
	if err != nil {
		return false, 0, err
	}
	checkpoint, exists := checkpointForModel(checkpoints, model)
	if !exists || checkpoint.ActiveProbePending {
		return false, 0, nil
	}
	identityHash := UpstreamPriceAccountIdentityHash(account)
	billingHash, contextKey := upstreamPriceBillingContext(billing)
	identityKey := identityHash
	if len(identityKey) > 16 {
		identityKey = identityKey[:16]
	}
	contextKey += ";ledger=" + checkpoint.LedgerDate + ";identity=" + identityKey
	activeContextHash := sha256.Sum256([]byte(contextKey))
	activeContextKey := "active-" + hex.EncodeToString(activeContextHash[:16])
	evidence := &domain.UpstreamPriceEvidence{
		RunID: runID, AccountID: account.ID, ModelName: model, BillingMode: DisplayBillingModeToken,
		Status: domain.UpstreamPriceEvidenceStatusUnobservable, Source: domain.UpstreamPriceEvidenceSourceActiveProbe,
		ReconciliationStatus: domain.UpstreamPriceReconciliationNoActivity, ContextKey: activeContextKey,
		ObservedAt: s.now().UTC(),
	}

	initialUsage, err := s.remote.FetchUsage(ctx, account)
	if err != nil {
		evidence.LastError = "active probe baseline usage unavailable: " + err.Error()
		return false, 0, s.repo.SaveReconciliation(ctx, nil, nil, evidence)
	}
	initialRemote := canonicalUsageMap(initialUsage.Models)[strings.ToLower(model)]
	initialLocalID, err := s.repo.CurrentLocalUsageLogID(ctx, []int64{account.ID})
	if err != nil {
		return false, 0, err
	}
	initialLocal, err := s.repo.AggregateLocalUsage(ctx, []int64{account.ID}, map[string]int64{model: checkpoint.LocalUsageLogID}, initialLocalID)
	if err != nil {
		return false, 0, err
	}
	if initialUsage.LedgerDate != checkpoint.LedgerDate || checkpoint.AccountIdentityHash != identityHash ||
		checkpoint.BillingContextHash != billingHash || !equalUpstreamPriceFullCounters(initialRemote, checkpoint.Remote) ||
		!upstreamPriceCountersEmpty(initialLocal[strings.ToLower(model)].Counters) {
		evidence.LastError = "new natural traffic arrived before active attribution; deferred to passive reconciliation"
		return false, 0, s.repo.SaveReconciliation(ctx, nil, nil, evidence)
	}

	specs := upstreamPriceActiveProbeSpecs(model)
	observations := make([]domain.UpstreamPriceObservation, 0, len(specs))
	checkpoint.Remote = initialRemote
	checkpoint.LocalUsageLogID = initialLocalID
	checkpoint.CapturedAt = initialUsage.CapturedAt.UTC()
	checkpointCapturedAt := initialUsage.CapturedAt.UTC()
	contaminated := false
	var notes []string
	probeCost := 0.0
	explicitFallbackStarted := false
	for specIndex, spec := range specs {
		if spec.ExplicitCache && !explicitFallbackStarted {
			if upstreamPriceObservationsHaveCache(observations) {
				break
			}
			explicitFallbackStarted = true
		}
		beforeUsage, fetchErr := s.remote.FetchUsage(ctx, account)
		if fetchErr != nil {
			notes = append(notes, "usage baseline failed: "+fetchErr.Error())
			break
		}
		beforeRemote := canonicalUsageMap(beforeUsage.Models)[strings.ToLower(model)]
		beforeLocalID, localIDErr := s.repo.CurrentLocalUsageLogID(ctx, []int64{account.ID})
		if localIDErr != nil {
			return false, probeCost, localIDErr
		}
		localBefore, localErr := s.repo.AggregateLocalUsage(ctx, []int64{account.ID}, map[string]int64{model: checkpoint.LocalUsageLogID}, beforeLocalID)
		if localErr != nil {
			return false, probeCost, localErr
		}
		if beforeUsage.LedgerDate != checkpoint.LedgerDate || !equalUpstreamPriceFullCounters(beforeRemote, checkpoint.Remote) ||
			!upstreamPriceCountersEmpty(localBefore[strings.ToLower(model)].Counters) {
			notes = append(notes, "natural traffic arrived between active samples; remaining probes deferred")
			break
		}

		releaseProbeSlot, acquired, acquireErr := s.acquireUpstreamPriceProbeSlot(ctx, account)
		if acquireErr != nil {
			notes = append(notes, "active probe concurrency check failed: "+acquireErr.Error())
			break
		}
		if !acquired {
			notes = append(notes, "active probe deferred to preserve a production-user concurrency slot")
			break
		}
		probeStartedAt := s.now().UTC()
		pendingCheckpoint := checkpoint
		pendingCheckpoint.Remote = beforeRemote
		pendingCheckpoint.LocalUsageLogID = beforeLocalID
		pendingCheckpoint.CapturedAt = beforeUsage.CapturedAt.UTC()
		pendingCheckpoint.ActiveProbePending = true
		pendingCheckpoint.ActiveProbeStartedAt = &probeStartedAt
		expectedRevision := checkpoint.Revision
		if err := s.repo.SaveReconciliation(ctx, &pendingCheckpoint, &expectedRevision, nil); err != nil {
			releaseProbeSlot()
			return false, probeCost, err
		}
		checkpoint = pendingCheckpoint
		sendErr := s.prober.Probe(ctx, account, spec)
		ledgerWait := 15 * time.Second
		if sendErr != nil {
			ledgerWait = 8 * time.Second
		}
		afterUsage, afterRemote, waitErr := s.waitForUpstreamPriceProbeLedger(
			ctx, account, model, beforeRemote, checkpoint.LedgerDate, ledgerWait,
		)
		if waitErr != nil {
			if sendErr != nil {
				notes = append(notes, sendErr.Error())
			}
			notes = append(notes, waitErr.Error())
			var httpErr *upstreamPriceActiveProbeHTTPError
			if errors.As(sendErr, &httpErr) && httpErr.DefinitiveNoCharge() {
				clearedCheckpoint := checkpoint
				clearedCheckpoint.ActiveProbePending = false
				clearedCheckpoint.ActiveProbeStartedAt = nil
				settleCtx, cancelSettle := upstreamPriceProbeSettlementContext(ctx)
				expectedRevision = checkpoint.Revision
				settleErr := s.repo.SaveReconciliation(settleCtx, &clearedCheckpoint, &expectedRevision, nil)
				cancelSettle()
				if settleErr != nil {
					releaseProbeSlot()
					return false, probeCost, settleErr
				}
				checkpoint = clearedCheckpoint
			}
			releaseProbeSlot()
			break
		}
		afterLocalID, localIDErr := s.repo.CurrentLocalUsageLogID(ctx, []int64{account.ID})
		if localIDErr != nil {
			releaseProbeSlot()
			return false, probeCost, localIDErr
		}
		localDuring, localErr := s.repo.AggregateLocalUsage(ctx, []int64{account.ID}, map[string]int64{model: beforeLocalID}, afterLocalID)
		if localErr != nil {
			releaseProbeSlot()
			return false, probeCost, localErr
		}
		remoteDelta, deltaOK := subtractUpstreamPriceCounters(beforeRemote, afterRemote)
		if !deltaOK {
			notes = append(notes, "active probe ledger reset while sampling")
			releaseProbeSlot()
			break
		}
		settledCheckpoint := checkpoint
		settledCheckpoint.Remote = afterRemote
		settledCheckpoint.LocalUsageLogID = afterLocalID
		settledCheckpoint.CapturedAt = afterUsage.CapturedAt.UTC()
		settledCheckpoint.ActiveProbePending = false
		settledCheckpoint.ActiveProbeStartedAt = nil
		checkpointCapturedAt = afterUsage.CapturedAt.UTC()
		localDelta := localDuring[strings.ToLower(model)].Counters
		if remoteDelta.Requests != 1 || !upstreamPriceCountersEmpty(localDelta) {
			contaminated = true
			notes = append(notes, "active probe overlapped natural or external traffic; durable pending state retained for delayed recovery")
			releaseProbeSlot()
			break
		}
		sampleEvidence := &domain.UpstreamPriceEvidence{
			RunID: runID, AccountID: account.ID, ModelName: model, BillingMode: DisplayBillingModeToken,
			Status: domain.UpstreamPriceEvidenceStatusPending, Source: domain.UpstreamPriceEvidenceSourceActiveProbe,
			ReconciliationStatus: domain.UpstreamPriceReconciliationMatched,
			ContextKey:           activeContextKey + "-sample-" + strconv.Itoa(specIndex),
			ObservedAt:           afterUsage.CapturedAt.UTC(), SampleCount: 1, RemoteDelta: remoteDelta,
		}
		settleCtx, cancelSettle := upstreamPriceProbeSettlementContext(ctx)
		expectedRevision = checkpoint.Revision
		settleErr := s.repo.SaveReconciliation(settleCtx, &settledCheckpoint, &expectedRevision, sampleEvidence)
		cancelSettle()
		if settleErr != nil {
			releaseProbeSlot()
			return false, probeCost, settleErr
		}
		checkpoint = settledCheckpoint
		releaseProbeSlot()
		probeCost += math.Max(0, remoteDelta.ActualCost)
		observations = append(observations, domain.UpstreamPriceObservation{
			InputTokens: remoteDelta.InputTokens, OutputTokens: remoteDelta.OutputTokens,
			CacheCreationTokens: remoteDelta.CacheCreationTokens, CacheReadTokens: remoteDelta.CacheReadTokens,
			ActualCost: remoteDelta.ActualCost,
		})
		if sendErr != nil {
			notes = append(notes, "probe response failed after one attributable ledger charge: "+sendErr.Error())
		}
	}

	billingStable := true
	latestBilling, latestBillingErr := s.remote.FetchBilling(ctx, account)
	if latestBillingErr != nil || latestBilling == nil {
		billingStable = false
		notes = append(notes, "could not verify billing context after active probes")
	} else {
		latestBillingHash, _ := upstreamPriceBillingContext(latestBilling)
		if latestBillingHash != billingHash {
			billingStable = false
			notes = append(notes, "billing multiplier or peak context changed during active probes")
		}
	}
	evidence.ObservedAt = checkpointCapturedAt
	evidence.SampleCount = len(observations)
	if checkpoint.ActiveProbePending {
		evidence.Status = domain.UpstreamPriceEvidenceStatusUnobservable
		evidence.ReconciliationStatus = domain.UpstreamPriceReconciliationNoActivity
		notes = append(notes, "active probe settlement remains pending and will be recovered before later passive inference")
	} else if contaminated || !billingStable {
		evidence.Status = domain.UpstreamPriceEvidenceStatusMismatch
		evidence.ReconciliationStatus = domain.UpstreamPriceReconciliationMixedContext
	} else if len(observations) > 0 {
		evidence.Status = domain.UpstreamPriceEvidenceStatusPending
		evidence.ReconciliationStatus = domain.UpstreamPriceReconciliationMatched
		prices, sampleCount, solveErr := SolveUpstreamTokenPrices(observations)
		evidence.SampleCount = sampleCount
		if solveErr == nil {
			evidence.Status = domain.UpstreamPriceEvidenceStatusTrusted
			evidence.Prices = prices
			evidence.SuggestedPrices = multiplyUpstreamPriceVector(prices, cfg.Markup)
		} else {
			notes = append(notes, solveErr.Error())
		}
	}
	evidence.LastError = strings.Join(notes, "; ")
	saveCtx, cancelSave := upstreamPriceProbeSettlementContext(ctx)
	defer cancelSave()
	if err := s.repo.SaveReconciliation(saveCtx, nil, nil, evidence); err != nil {
		return len(observations) > 0, probeCost, err
	}
	return len(observations) > 0, probeCost, nil
}

func upstreamPriceActiveProbeSpecs(model string) []UpstreamPriceActiveProbeRequest {
	nonce := upstreamPriceProbeNonce()
	cacheSession := "pricing-probe-cache-" + nonce
	explicitCacheSession := "pricing-probe-cache-explicit-" + nonce
	cachePrefix := "pricing probe " + nonce + " " + strings.Repeat("stable calibration knowledge alpha beta gamma delta epsilon ", 220)
	return []UpstreamPriceActiveProbeRequest{
		{Model: model, UserPrompt: "Probe " + nonce + ". Return exactly the single letter A.", MaxTokens: 1, SessionID: "pricing-probe-short-" + nonce},
		{Model: model, UserPrompt: "Probe " + nonce + ". " + strings.Repeat("calibrationword ", 96) + " Return exactly the single letter A.", MaxTokens: 1, SessionID: "pricing-probe-input-" + nonce},
		{Model: model, UserPrompt: "Probe " + nonce + ". Write the integers 1 through 40 separated by commas, with no other text.", MaxTokens: 64, SessionID: "pricing-probe-output-" + nonce},
		{Model: model, SystemPrompt: cachePrefix, UserPrompt: "Reply exactly A.", MaxTokens: 1, SessionID: cacheSession},
		{Model: model, SystemPrompt: cachePrefix, UserPrompt: "Reply exactly A.", MaxTokens: 1, SessionID: cacheSession},
		{Model: model, SystemPrompt: cachePrefix, UserPrompt: "Reply exactly A.", MaxTokens: 1, SessionID: explicitCacheSession, ExplicitCache: true},
		{Model: model, SystemPrompt: cachePrefix, UserPrompt: "Reply exactly A.", MaxTokens: 1, SessionID: explicitCacheSession, ExplicitCache: true},
	}
}

func upstreamPriceObservationsHaveCache(values []domain.UpstreamPriceObservation) bool {
	hasWrite, hasRead := false, false
	for _, value := range values {
		hasWrite = hasWrite || value.CacheCreationTokens > 0
		hasRead = hasRead || value.CacheReadTokens > 0
	}
	return hasWrite && hasRead
}

func upstreamPriceProbeNonce() string {
	var raw [8]byte
	if _, err := crand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func (s *UpstreamPriceMonitorService) acquireUpstreamPriceProbeSlot(
	ctx context.Context,
	account *Account,
) (func(), bool, error) {
	if s.probeSlotAcquirer != nil {
		return s.probeSlotAcquirer(ctx, account)
	}
	if account == nil {
		return func() {}, false, nil
	}
	if s.concurrency != nil {
		loads, err := s.concurrency.GetAccountsLoadBatchFresh(ctx, []AccountWithConcurrency{{
			ID: account.ID, MaxConcurrency: account.Concurrency,
		}})
		if err != nil {
			return func() {}, false, err
		}
		if load := loads[account.ID]; load != nil &&
			(load.CurrentConcurrency > 0 || load.WaitingCount > 0) {
			return func() {}, false, nil
		}
	}
	if account.Concurrency <= 0 || s.concurrency == nil {
		return func() {}, false, nil
	}
	releases := make([]func(), 0, account.Concurrency)
	for i := 0; i < account.Concurrency; i++ {
		result, err := s.concurrency.AcquireAccountSlot(ctx, account.ID, account.Concurrency)
		if err != nil {
			for _, release := range releases {
				release()
			}
			return func() {}, false, err
		}
		if result == nil || !result.Acquired || result.ReleaseFunc == nil {
			for _, release := range releases {
				release()
			}
			return func() {}, false, nil
		}
		releases = append(releases, result.ReleaseFunc)
	}
	return func() {
		for index := len(releases) - 1; index >= 0; index-- {
			releases[index]()
		}
	}, true, nil
}

func upstreamPriceProbeSettlementContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, 5*time.Second)
}

func (s *UpstreamPriceMonitorService) waitForUpstreamPriceProbeLedger(
	ctx context.Context,
	account *Account,
	model string,
	before domain.UpstreamPriceUsageCounters,
	ledgerDate string,
	maxWait time.Duration,
) (*domain.UpstreamPriceRemoteUsageSnapshot, domain.UpstreamPriceUsageCounters, error) {
	deadline := time.Now().Add(maxWait)
	stableReads := 0
	var candidate domain.UpstreamPriceUsageCounters
	var candidateSnapshot *domain.UpstreamPriceRemoteUsageSnapshot
	for {
		snapshot, err := s.remote.FetchUsage(ctx, account)
		if err == nil {
			current := canonicalUsageMap(snapshot.Models)[strings.ToLower(model)]
			if snapshot.LedgerDate != ledgerDate {
				return snapshot, current, errors.New("active probe crossed the upstream ledger day boundary")
			}
			if current.Requests > before.Requests {
				if stableReads > 0 && equalUpstreamPriceFullCounters(current, candidate) {
					stableReads++
				} else {
					candidate = current
					candidateSnapshot = snapshot
					stableReads = 1
				}
				if stableReads >= 3 {
					return candidateSnapshot, candidate, nil
				}
			} else {
				stableReads = 0
				candidateSnapshot = nil
			}
		}
		if time.Now().After(deadline) {
			return nil, before, errors.New("active probe charge did not appear in /v1/usage before timeout")
		}
		timer := time.NewTimer(upstreamPriceProbeLedgerPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, before, ctx.Err()
		case <-timer.C:
		}
	}
}

func equalUpstreamPriceFullCounters(a, b domain.UpstreamPriceUsageCounters) bool {
	return equalUpstreamPriceAccountingCounters(a, b) && math.Abs(a.ActualCost-b.ActualCost) <= 1e-9
}

func addUpstreamPriceCounters(a, b domain.UpstreamPriceUsageCounters) domain.UpstreamPriceUsageCounters {
	return domain.UpstreamPriceUsageCounters{
		Requests: a.Requests + b.Requests, InputTokens: a.InputTokens + b.InputTokens,
		OutputTokens:        a.OutputTokens + b.OutputTokens,
		CacheCreationTokens: a.CacheCreationTokens + b.CacheCreationTokens,
		CacheReadTokens:     a.CacheReadTokens + b.CacheReadTokens, ActualCost: a.ActualCost + b.ActualCost,
	}
}

func checkpointForModel(values map[string]domain.UpstreamPriceUsageCheckpoint, model string) (domain.UpstreamPriceUsageCheckpoint, bool) {
	if value, ok := values[model]; ok {
		return value, true
	}
	value, ok := values[strings.ToLower(model)]
	return value, ok
}

func canonicalUsageMap(values map[string]domain.UpstreamPriceUsageCounters) map[string]domain.UpstreamPriceUsageCounters {
	out := make(map[string]domain.UpstreamPriceUsageCounters, len(values))
	for model, counters := range values {
		out[strings.ToLower(strings.TrimSpace(model))] = counters
	}
	return out
}

func subtractUpstreamPriceCounters(before, after domain.UpstreamPriceUsageCounters) (domain.UpstreamPriceUsageCounters, bool) {
	delta := domain.UpstreamPriceUsageCounters{
		Requests:            after.Requests - before.Requests,
		InputTokens:         after.InputTokens - before.InputTokens,
		OutputTokens:        after.OutputTokens - before.OutputTokens,
		CacheCreationTokens: after.CacheCreationTokens - before.CacheCreationTokens,
		CacheReadTokens:     after.CacheReadTokens - before.CacheReadTokens,
		ActualCost:          after.ActualCost - before.ActualCost,
	}
	ok := delta.Requests >= 0 && delta.InputTokens >= 0 && delta.OutputTokens >= 0 &&
		delta.CacheCreationTokens >= 0 && delta.CacheReadTokens >= 0 && delta.ActualCost >= -1e-9
	if delta.ActualCost < 0 && delta.ActualCost >= -1e-9 {
		delta.ActualCost = 0
	}
	return delta, ok
}

func equalUpstreamPriceAccountingCounters(a, b domain.UpstreamPriceUsageCounters) bool {
	return a.Requests == b.Requests && a.InputTokens == b.InputTokens && a.OutputTokens == b.OutputTokens &&
		a.CacheCreationTokens == b.CacheCreationTokens && a.CacheReadTokens == b.CacheReadTokens
}

func upstreamPriceCountersEmpty(v domain.UpstreamPriceUsageCounters) bool {
	return v.Requests == 0 && v.InputTokens == 0 && v.OutputTokens == 0 && v.CacheCreationTokens == 0 && v.CacheReadTokens == 0
}

func upstreamPriceBillingContext(billing *domain.UpstreamPriceBillingSnapshot) (string, string) {
	if billing == nil {
		return "unknown", "billing-unknown"
	}
	// observed_at identifies when the response was fetched, not a price
	// dimension. Including it would make every adjacent checkpoint appear to
	// have a mixed billing context and permanently prevent trusted samples.
	stable := struct {
		ResolvedRateMultiplier  float64
		EffectiveRateMultiplier float64
		PeakRateEnabled         bool
		PeakStart               string
		PeakEnd                 string
		PeakRateMultiplier      *float64
		AppliedPeakMultiplier   float64
		Timezone                string
	}{
		ResolvedRateMultiplier: billing.ResolvedRateMultiplier, EffectiveRateMultiplier: billing.EffectiveRateMultiplier,
		PeakRateEnabled: billing.PeakRateEnabled, PeakStart: billing.PeakStart, PeakEnd: billing.PeakEnd,
		PeakRateMultiplier: billing.PeakRateMultiplier, AppliedPeakMultiplier: billing.AppliedPeakMultiplier,
		Timezone: billing.Timezone,
	}
	raw, _ := json.Marshal(stable)
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	key := "resolved=" + strconv.FormatFloat(billing.ResolvedRateMultiplier, 'g', 10, 64) +
		";effective=" + strconv.FormatFloat(billing.EffectiveRateMultiplier, 'g', 10, 64) +
		";peak=" + strconv.FormatFloat(billing.AppliedPeakMultiplier, 'g', 10, 64)
	return hash, key
}

// UpstreamPriceAccountIdentityHash invalidates checkpoints on credential,
// endpoint, proxy, or header-override changes without persisting the secret.
func UpstreamPriceAccountIdentityHash(account *Account) string {
	if account == nil {
		return ""
	}
	proxyID := int64(0)
	proxyURL := ""
	if account.ProxyID != nil {
		proxyID = *account.ProxyID
	}
	if account.Proxy != nil {
		proxyURL = strings.TrimSpace(account.Proxy.URL())
	}
	payload := struct {
		ID         int64
		LedgerHash string
		BaseURL    string
		ProxyID    int64
		ProxyURL   string
		Headers    any
	}{
		account.ID, UpstreamPriceCredentialLedgerHash(account),
		strings.TrimRight(strings.ToLower(strings.TrimSpace(account.GetOpenAIBaseURL())), "/"),
		proxyID, proxyURL, account.GetHeaderOverrides(),
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// UpstreamPriceCredentialLedgerHash identifies accounts that read the same
// cumulative upstream /v1/usage ledger without persisting the credential.
// Account ID is intentionally excluded so duplicate production-key entries
// can be rejected before they create permanently mismatched local windows.
func UpstreamPriceCredentialLedgerHash(account *Account) string {
	if account == nil {
		return ""
	}
	payload := struct{ APIKey string }{APIKey: strings.TrimSpace(account.GetCredential("api_key"))}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// SolveUpstreamTokenPrices solves C = Xp using full-rank matched windows. Token
// columns that have never been observed are omitted and remain nil. Prices are
// returned in USD per million tokens.
func SolveUpstreamTokenPrices(observations []domain.UpstreamPriceObservation) (domain.UpstreamPriceVector, int, error) {
	valid := make([]domain.UpstreamPriceObservation, 0, len(observations))
	active := [4]bool{}
	for _, row := range observations {
		if row.ActualCost < 0 || math.IsNaN(row.ActualCost) || math.IsInf(row.ActualCost, 0) {
			continue
		}
		tokens := [4]int64{row.InputTokens, row.OutputTokens, row.CacheCreationTokens, row.CacheReadTokens}
		invalid := false
		for i, value := range tokens {
			if value < 0 {
				invalid = true
			}
			if value > 0 {
				active[i] = true
			}
		}
		if !invalid {
			valid = append(valid, row)
		}
	}
	columns := make([]int, 0, 4)
	for i, enabled := range active {
		if enabled {
			columns = append(columns, i)
		}
	}
	if len(columns) == 0 || len(valid) < len(columns) {
		return domain.UpstreamPriceVector{}, len(valid), ErrUpstreamPriceInsufficientRank
	}

	n := len(columns)
	a := make([][]float64, n)
	b := make([]float64, n)
	for i := range a {
		a[i] = make([]float64, n)
	}
	for _, row := range valid {
		tokens := [4]int64{row.InputTokens, row.OutputTokens, row.CacheCreationTokens, row.CacheReadTokens}
		x := make([]float64, n)
		for i, column := range columns {
			x[i] = float64(tokens[column]) / 1_000_000
		}
		for i := 0; i < n; i++ {
			b[i] += x[i] * row.ActualCost
			for j := 0; j < n; j++ {
				a[i][j] += x[i] * x[j]
			}
		}
	}
	solution, ok := solveUpstreamPriceLinearSystem(a, b)
	if !ok {
		return domain.UpstreamPriceVector{}, len(valid), ErrUpstreamPriceInsufficientRank
	}
	for i, value := range solution {
		if value < -1e-8 || math.IsNaN(value) || math.IsInf(value, 0) {
			return domain.UpstreamPriceVector{}, len(valid), errors.New("price solution is negative or non-finite")
		}
		if value < 0 {
			solution[i] = 0
		}
	}
	maxCost, maxResidual := 0.0, 0.0
	for _, row := range valid {
		tokens := [4]int64{row.InputTokens, row.OutputTokens, row.CacheCreationTokens, row.CacheReadTokens}
		predicted := 0.0
		for i, column := range columns {
			predicted += float64(tokens[column]) / 1_000_000 * solution[i]
		}
		maxCost = math.Max(maxCost, row.ActualCost)
		maxResidual = math.Max(maxResidual, math.Abs(predicted-row.ActualCost))
	}
	if maxResidual > math.Max(5e-8, maxCost*.01) {
		return domain.UpstreamPriceVector{}, len(valid), fmt.Errorf("price fit residual %.12g exceeds tolerance", maxResidual)
	}
	var out domain.UpstreamPriceVector
	for i, column := range columns {
		value := solution[i]
		switch column {
		case 0:
			out.InputPerMillion = &value
		case 1:
			out.OutputPerMillion = &value
		case 2:
			out.CacheWritePerMillion = &value
		case 3:
			out.CacheReadPerMillion = &value
		}
	}
	return out, len(valid), nil
}

func solveUpstreamPriceLinearSystem(a [][]float64, b []float64) ([]float64, bool) {
	n := len(b)
	matrixScale := 0.0
	for row := range a {
		for col := range a[row] {
			matrixScale = math.Max(matrixScale, math.Abs(a[row][col]))
		}
	}
	if matrixScale == 0 || math.IsNaN(matrixScale) || math.IsInf(matrixScale, 0) {
		return nil, false
	}
	minPivot, maxPivot := math.Inf(1), 0.0
	for col := 0; col < n; col++ {
		pivot := col
		for row := col + 1; row < n; row++ {
			if math.Abs(a[row][col]) > math.Abs(a[pivot][col]) {
				pivot = row
			}
		}
		pivotSize := math.Abs(a[pivot][col])
		if pivotSize < math.Max(matrixScale*1e-10, 1e-24) {
			return nil, false
		}
		minPivot = math.Min(minPivot, pivotSize)
		maxPivot = math.Max(maxPivot, pivotSize)
		a[col], a[pivot] = a[pivot], a[col]
		b[col], b[pivot] = b[pivot], b[col]
		divisor := a[col][col]
		for j := col; j < n; j++ {
			a[col][j] /= divisor
		}
		b[col] /= divisor
		for row := 0; row < n; row++ {
			if row == col {
				continue
			}
			factor := a[row][col]
			for j := col; j < n; j++ {
				a[row][j] -= factor * a[col][j]
			}
			b[row] -= factor * b[col]
		}
	}
	// Normal equations square the original matrix condition number. Reject a
	// weakly independent natural-traffic matrix rather than turning tiny token
	// ratio noise into an extreme but low-residual positive price.
	if minPivot <= 0 || maxPivot/minPivot > 1e10 {
		return nil, false
	}
	return b, true
}

func multiplyUpstreamPriceVector(value domain.UpstreamPriceVector, multiplier float64) domain.UpstreamPriceVector {
	return domain.UpstreamPriceVector{
		InputPerMillion:      multiplyPricePtr(value.InputPerMillion, multiplier),
		OutputPerMillion:     multiplyPricePtr(value.OutputPerMillion, multiplier),
		CacheWritePerMillion: multiplyPricePtr(value.CacheWritePerMillion, multiplier),
		CacheReadPerMillion:  multiplyPricePtr(value.CacheReadPerMillion, multiplier),
		PerRequestLTE256K:    multiplyPricePtr(value.PerRequestLTE256K, multiplier),
		PerRequest256K512K:   multiplyPricePtr(value.PerRequest256K512K, multiplier),
		PerRequestGT512K:     multiplyPricePtr(value.PerRequestGT512K, multiplier),
	}
}

func multiplyPricePtr(value *float64, multiplier float64) *float64 {
	if value == nil {
		return nil
	}
	out := *value * multiplier
	return &out
}

type UpstreamPriceRunApplyScope struct {
	AccountIDs                []int64
	AccountLedgerHashes       map[int64]string
	AccountIdentityHashes     map[int64]string
	ChannelIDs                []int64
	DisplayMultiplierDecimals int
	MaxAgeMinutes             int
	ConfigUpdatedAt           time.Time
	ModelCatalogRevision      int64
}

func ReadUpstreamPriceRunApplyScope(run *domain.UpstreamPriceMonitorRun) (UpstreamPriceRunApplyScope, bool) {
	if run == nil || run.Summary == nil {
		return UpstreamPriceRunApplyScope{}, false
	}
	accountIDs, ok := upstreamPriceSummaryInt64Slice(run.Summary["account_ids"])
	if !ok {
		return UpstreamPriceRunApplyScope{}, false
	}
	ledgerHashes, ok := upstreamPriceSummaryLedgerHashes(run.Summary["account_ledger_hashes"])
	if !ok {
		return UpstreamPriceRunApplyScope{}, false
	}
	identityHashes, ok := upstreamPriceSummaryLedgerHashes(run.Summary["account_identity_hashes"])
	if !ok {
		return UpstreamPriceRunApplyScope{}, false
	}
	channelIDs, ok := upstreamPriceSummaryInt64Slice(run.Summary["channel_ids"])
	if !ok {
		return UpstreamPriceRunApplyScope{}, false
	}
	decimals, ok := upstreamPriceSummaryInt(run.Summary["display_multiplier_decimals"])
	if !ok || decimals < 0 || decimals > 6 {
		return UpstreamPriceRunApplyScope{}, false
	}
	maxAgeMinutes, ok := upstreamPriceSummaryInt(run.Summary["snapshot_max_age_minutes"])
	if !ok || maxAgeMinutes <= 0 {
		return UpstreamPriceRunApplyScope{}, false
	}
	configUpdatedRaw, ok := run.Summary["config_updated_at"].(string)
	if !ok {
		return UpstreamPriceRunApplyScope{}, false
	}
	configUpdatedAt, err := time.Parse(time.RFC3339Nano, configUpdatedRaw)
	if err != nil || configUpdatedAt.IsZero() {
		return UpstreamPriceRunApplyScope{}, false
	}
	catalogRevision, ok := upstreamPriceSummaryInt(run.Summary["model_catalog_revision"])
	if !ok || catalogRevision <= 0 {
		return UpstreamPriceRunApplyScope{}, false
	}
	accountIDs = uniquePositiveInt64s(accountIDs)
	channelIDs = uniquePositiveInt64s(channelIDs)
	for _, accountID := range accountIDs {
		if strings.TrimSpace(ledgerHashes[accountID]) == "" || strings.TrimSpace(identityHashes[accountID]) == "" {
			return UpstreamPriceRunApplyScope{}, false
		}
	}
	return UpstreamPriceRunApplyScope{
		AccountIDs: accountIDs, AccountLedgerHashes: ledgerHashes, AccountIdentityHashes: identityHashes,
		ChannelIDs: channelIDs, DisplayMultiplierDecimals: decimals,
		MaxAgeMinutes: maxAgeMinutes, ConfigUpdatedAt: configUpdatedAt,
		ModelCatalogRevision: int64(catalogRevision),
	}, len(accountIDs) > 0 && len(channelIDs) > 0
}

func upstreamPriceSummaryLedgerHashes(value any) (map[int64]string, bool) {
	out := make(map[int64]string)
	switch typed := value.(type) {
	case map[string]string:
		for key, hash := range typed {
			accountID, err := strconv.ParseInt(key, 10, 64)
			if err != nil || accountID <= 0 {
				return nil, false
			}
			out[accountID] = strings.TrimSpace(hash)
		}
	case map[string]any:
		for key, raw := range typed {
			accountID, err := strconv.ParseInt(key, 10, 64)
			hash, ok := raw.(string)
			if err != nil || accountID <= 0 || !ok {
				return nil, false
			}
			out[accountID] = strings.TrimSpace(hash)
		}
	default:
		return nil, false
	}
	return out, true
}

func upstreamPriceSummaryInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		converted := int(typed)
		return converted, int64(converted) == typed
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) {
			return 0, false
		}
		converted := int(typed)
		return converted, float64(converted) == typed
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		return parsed, err == nil
	default:
		return 0, false
	}
}

func upstreamPriceSummaryInt64Slice(value any) ([]int64, bool) {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...), true
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			parsed, ok := upstreamPriceSummaryInt(item)
			if !ok {
				return nil, false
			}
			out = append(out, int64(parsed))
		}
		return out, true
	default:
		return nil, false
	}
}

func upstreamPriceEvidenceHash(evidence []domain.UpstreamPriceEvidence) string {
	sort.Slice(evidence, func(i, j int) bool {
		if evidence[i].AccountID != evidence[j].AccountID {
			return evidence[i].AccountID < evidence[j].AccountID
		}
		if evidence[i].ModelName != evidence[j].ModelName {
			return evidence[i].ModelName < evidence[j].ModelName
		}
		return evidence[i].ContextKey < evidence[j].ContextKey
	})
	raw, _ := json.Marshal(evidence)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func stringErrors(values []string) []error {
	out := make([]error, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, errors.New(value))
		}
	}
	return out
}

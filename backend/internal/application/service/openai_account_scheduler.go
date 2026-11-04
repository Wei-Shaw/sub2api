package service

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"golang.org/x/sync/singleflight"
)

const (
	openAIAccountScheduleLayerPreviousResponse = "previous_response_id"
	openAIAccountScheduleLayerSessionSticky    = "session_hash"
	openAIAccountScheduleLayerContentBurst     = "content_session_burst"
	openAIAccountScheduleLayerLoadBalance      = "load_balance"
	openAIAdvancedSchedulerSettingKey          = "openai_advanced_scheduler_enabled"
)

const (
	openAIAdvancedSchedulerSettingCacheTTL  = 5 * time.Second
	openAIAdvancedSchedulerSettingDBTimeout = 2 * time.Second
	// ponytail: cap probes added when cost ordering expands configured Top-K;
	// use bulk acquisition if a measured workload needs a higher ceiling.
	openAIAccountSelectionProbeLimit = 64
)

const (
	openAIQuotaHeadroomNeutralFactor           = 0.5
	openAIQuotaHeadroomSecondaryLowRemain      = 0.10
	openAIQuotaHeadroomSnapshotStaleAfter      = 8 * time.Hour
	openAIUpstreamCostNeutralFactor            = 0.5
	defaultOpenAIOAuthSchedulingRateMultiplier = 1.0
)

const (
	openAIAccountRuntimeStatsMaxEntries = 10000
	openAIAccountRuntimeStatsTrimBatch  = 1000
	openAIAccountRuntimeStatsStaleTTL   = 24 * time.Hour
	openAIAccountRuntimeStatsSweepEvery = time.Minute
)

type cachedOpenAIAdvancedSchedulerSetting struct {
	lowUpstreamRatePriorityEnabled bool
	oauthSchedulingRateMultiplier  float64
	contentSessionBurstBalance     bool
	sessionIDRateLimitEnabled      bool
	sessionIDRateLimitPerMinute    int
	oauthGatewayRateLimitEnabled   bool
	oauthGatewayRateLimitRPM       int
	oauthGatewayRateLimitBurst     int
	requestIntegrityObserveEnabled bool
	enabled                        bool
	stickyWeightedEnabled          bool
	subscriptionPriorityEnabled    bool
	lbTopKOverride                 int
	weightOverrides                map[string]float64
	expiresAt                      int64
}

type openAIAdvancedSchedulerRuntimeSettings struct {
	lowUpstreamRatePriorityEnabled bool
	oauthSchedulingRateMultiplier  float64
	contentSessionBurstBalance     bool
	sessionIDRateLimitEnabled      bool
	sessionIDRateLimitPerMinute    int
	oauthGatewayRateLimitEnabled   bool
	oauthGatewayRateLimitRPM       int
	oauthGatewayRateLimitBurst     int
	requestIntegrityObserveEnabled bool
	enabled                        bool
	stickyWeightedEnabled          bool
	subscriptionPriorityEnabled    bool
	lbTopKOverride                 int
	weightOverrides                map[string]float64
}

var openAIAdvancedSchedulerSettingCache atomic.Value // *cachedOpenAIAdvancedSchedulerSetting
var openAIAdvancedSchedulerSettingSF singleflight.Group

type OpenAIAccountScheduleRequest struct {
	GroupID                 *int64
	Platform                string
	SessionHash             string
	StickyAccountID         int64
	EphemeralStickyAccount  bool
	StickyPreviousAccountID int64
	StickyWeighted          bool
	SubscriptionPriority    bool
	PreserveStickyBinding   bool
	// DisableStickyEscape keeps task-owner lookups on their bound account even
	// when generic sticky health/concurrency heuristics would prefer another.
	DisableStickyEscape      bool
	ContentSessionConcurrent bool
	PreviousResponseID       string
	PreviousResponseCanMove  bool
	UseUpstreamTokenCost     bool
	RequestedModel           string
	RequiredTransport        OpenAIUpstreamTransport
	RequiredCapability       OpenAIEndpointCapability
	RequiredImageCapability  OpenAIImagesCapability
	RequireCompact           bool
	IsStreaming              bool
	ExcludedIDs              map[int64]struct{}
}

type OpenAIAccountScheduleDecision struct {
	Layer               string
	StickyPreviousHit   bool
	StickySessionHit    bool
	CandidateCount      int
	TopK                int
	LatencyMs           int64
	LoadSkew            float64
	SelectedAccountID   int64
	SelectedAccountType string
}

type OpenAIAccountSchedulerMetricsSnapshot struct {
	SelectTotal              int64
	StickyPreviousHitTotal   int64
	StickySessionHitTotal    int64
	LoadBalanceSelectTotal   int64
	AccountSwitchTotal       int64
	SchedulerLatencyMsTotal  int64
	SchedulerLatencyMsAvg    float64
	StickyHitRatio           float64
	AccountSwitchRate        float64
	LoadSkewAvg              float64
	RuntimeStatsAccountCount int
}

type OpenAIAccountScheduler interface {
	Select(ctx context.Context, req OpenAIAccountScheduleRequest) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error)
	ReportResult(accountID int64, success bool, firstTokenMs *int)
	ReportSwitch()
	SnapshotMetrics() OpenAIAccountSchedulerMetricsSnapshot
}

type openAIAccountSchedulerMetrics struct {
	selectTotal            atomic.Int64
	stickyPreviousHitTotal atomic.Int64
	stickySessionHitTotal  atomic.Int64
	stickyHitTotal         atomic.Int64
	loadBalanceSelectTotal atomic.Int64
	accountSwitchTotal     atomic.Int64
	latencyMsTotal         atomic.Int64
	loadSkewMilliTotal     atomic.Int64
}

type openAIAccountLoadPlan struct {
	allCandidates             []openAIAccountCandidateScore
	candidates                []openAIAccountCandidateScore
	staleSnapshotCompactRetry []openAIAccountCandidateScore
	selectionOrder            []openAIAccountCandidateScore
	candidateCount            int
	topK                      int
	loadSkew                  float64
	includeOverflowFallback   bool
}

type openAIAccountLoadSelectionAttempt struct {
	result              *AccountSelectionResult
	selectionOrder      []openAIAccountCandidateScore
	candidateCount      int
	topK                int
	loadSkew            float64
	compactBlocked      bool
	noCompactCandidates bool
	err                 error
}

func (m *openAIAccountSchedulerMetrics) recordSelect(decision OpenAIAccountScheduleDecision) {
	if m == nil {
		return
	}
	m.selectTotal.Add(1)
	m.latencyMsTotal.Add(decision.LatencyMs)
	m.loadSkewMilliTotal.Add(int64(math.Round(decision.LoadSkew * 1000)))
	if decision.StickyPreviousHit {
		m.stickyPreviousHitTotal.Add(1)
	}
	if decision.StickySessionHit {
		m.stickySessionHitTotal.Add(1)
	}
	if decision.StickyPreviousHit || decision.StickySessionHit {
		m.stickyHitTotal.Add(1)
	}
	if decision.Layer == openAIAccountScheduleLayerLoadBalance {
		m.loadBalanceSelectTotal.Add(1)
	}
}

func (m *openAIAccountSchedulerMetrics) recordSwitch() {
	if m == nil {
		return
	}
	m.accountSwitchTotal.Add(1)
}

type openAIAccountRuntimeStats struct {
	accounts     sync.Map
	accountCount atomic.Int64
	lastSweep    atomic.Int64
	sweepMu      sync.Mutex
}

type openAIAccountRuntimeStat struct {
	errorRateEWMABits atomic.Uint64
	ttftEWMABits      atomic.Uint64
	lastUsedUnixNano  atomic.Int64
}

func newOpenAIAccountRuntimeStats() *openAIAccountRuntimeStats {
	return &openAIAccountRuntimeStats{}
}

func (s *openAIAccountRuntimeStats) loadOrCreate(accountID int64) *openAIAccountRuntimeStat {
	now := time.Now()
	if value, ok := s.accounts.Load(accountID); ok {
		stat, _ := value.(*openAIAccountRuntimeStat)
		if stat != nil {
			stat.lastUsedUnixNano.Store(now.UnixNano())
			return stat
		}
	}

	stat := &openAIAccountRuntimeStat{}
	stat.ttftEWMABits.Store(math.Float64bits(math.NaN()))
	stat.lastUsedUnixNano.Store(now.UnixNano())
	actual, loaded := s.accounts.LoadOrStore(accountID, stat)
	if !loaded {
		s.accountCount.Add(1)
		s.sweep(now, s.accountCount.Load() > openAIAccountRuntimeStatsMaxEntries)
		return stat
	}
	existing, _ := actual.(*openAIAccountRuntimeStat)
	if existing != nil {
		existing.lastUsedUnixNano.Store(now.UnixNano())
		return existing
	}
	return stat
}

func updateEWMAAtomic(target *atomic.Uint64, sample float64, alpha float64) {
	for {
		oldBits := target.Load()
		oldValue := math.Float64frombits(oldBits)
		newValue := alpha*sample + (1-alpha)*oldValue
		if target.CompareAndSwap(oldBits, math.Float64bits(newValue)) {
			return
		}
	}
}

func (s *openAIAccountRuntimeStats) report(accountID int64, success bool, firstTokenMs *int) {
	if s == nil || accountID <= 0 {
		return
	}
	const alpha = 0.2
	stat := s.loadOrCreate(accountID)

	errorSample := 1.0
	if success {
		errorSample = 0.0
	}
	updateEWMAAtomic(&stat.errorRateEWMABits, errorSample, alpha)

	if firstTokenMs != nil && *firstTokenMs > 0 {
		ttft := float64(*firstTokenMs)
		ttftBits := math.Float64bits(ttft)
		for {
			oldBits := stat.ttftEWMABits.Load()
			oldValue := math.Float64frombits(oldBits)
			if math.IsNaN(oldValue) {
				if stat.ttftEWMABits.CompareAndSwap(oldBits, ttftBits) {
					break
				}
				continue
			}
			newValue := alpha*ttft + (1-alpha)*oldValue
			if stat.ttftEWMABits.CompareAndSwap(oldBits, math.Float64bits(newValue)) {
				break
			}
		}
	}
}

func (s *openAIAccountRuntimeStats) snapshot(accountID int64) (errorRate float64, ttft float64, hasTTFT bool) {
	if s == nil || accountID <= 0 {
		return 0, 0, false
	}
	value, ok := s.accounts.Load(accountID)
	if !ok {
		return 0, 0, false
	}
	stat, _ := value.(*openAIAccountRuntimeStat)
	if stat == nil {
		return 0, 0, false
	}
	stat.lastUsedUnixNano.Store(time.Now().UnixNano())
	errorRate = clamp01(math.Float64frombits(stat.errorRateEWMABits.Load()))
	ttftValue := math.Float64frombits(stat.ttftEWMABits.Load())
	if math.IsNaN(ttftValue) {
		return errorRate, 0, false
	}
	return errorRate, ttftValue, true
}

func (s *openAIAccountRuntimeStats) delete(accountID int64) {
	if s == nil || accountID <= 0 {
		return
	}
	if _, loaded := s.accounts.LoadAndDelete(accountID); loaded {
		s.accountCount.Add(-1)
	}
}

func (s *openAIAccountRuntimeStats) sweep(now time.Time, force bool) {
	if s == nil {
		return
	}
	lastSweep := time.Unix(0, s.lastSweep.Load())
	if !force && !lastSweep.IsZero() && now.Sub(lastSweep) < openAIAccountRuntimeStatsSweepEvery {
		return
	}
	s.sweepMu.Lock()
	defer s.sweepMu.Unlock()
	lastSweep = time.Unix(0, s.lastSweep.Load())
	if !force && !lastSweep.IsZero() && now.Sub(lastSweep) < openAIAccountRuntimeStatsSweepEvery {
		return
	}
	s.lastSweep.Store(now.UnixNano())

	type candidate struct {
		accountID int64
		lastUsed  int64
	}
	candidates := make([]candidate, 0, s.size())
	cutoff := now.Add(-openAIAccountRuntimeStatsStaleTTL).UnixNano()
	s.accounts.Range(func(key, value any) bool {
		accountID, idOK := key.(int64)
		stat, statOK := value.(*openAIAccountRuntimeStat)
		if !idOK || !statOK || stat == nil {
			if idOK {
				s.delete(accountID)
			}
			return true
		}
		lastUsed := stat.lastUsedUnixNano.Load()
		if lastUsed <= cutoff {
			s.delete(accountID)
			return true
		}
		candidates = append(candidates, candidate{accountID: accountID, lastUsed: lastUsed})
		return true
	})
	if s.size() <= openAIAccountRuntimeStatsMaxEntries {
		return
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].lastUsed < candidates[j].lastUsed })
	targetSize := openAIAccountRuntimeStatsMaxEntries - openAIAccountRuntimeStatsTrimBatch
	for _, candidate := range candidates {
		if s.size() <= targetSize {
			break
		}
		s.delete(candidate.accountID)
	}
}

func (s *openAIAccountRuntimeStats) size() int {
	if s == nil {
		return 0
	}
	return int(s.accountCount.Load())
}

type defaultOpenAIAccountScheduler struct {
	service *OpenAIGatewayService
	metrics openAIAccountSchedulerMetrics
	stats   *openAIAccountRuntimeStats
}

type openAISelectionProbeBudget struct {
	acquires  int
	rechecks  int
	attempted map[int64]struct{}
	limited   bool
}

func newOpenAISelectionProbeBudget() *openAISelectionProbeBudget {
	return &openAISelectionProbeBudget{attempted: make(map[int64]struct{})}
}

func (b *openAISelectionProbeBudget) enableLimit() {
	if b != nil {
		b.limited = true
	}
}

func (b *openAISelectionProbeBudget) recordAcquire(accountID int64) bool {
	if b == nil {
		return false
	}
	if !b.limited {
		return true
	}
	if b.acquires >= openAIAccountSelectionProbeLimit {
		return false
	}
	if b.attempted == nil {
		b.attempted = make(map[int64]struct{})
	}
	b.acquires++
	b.attempted[accountID] = struct{}{}
	return true
}

func (b *openAISelectionProbeBudget) recordRecheck() bool {
	if b == nil {
		return false
	}
	if !b.limited {
		return true
	}
	if b.rechecks >= openAIAccountSelectionProbeLimit {
		return false
	}
	b.rechecks++
	return true
}

func (b *openAISelectionProbeBudget) acquireExhausted() bool {
	return b != nil && b.limited && b.acquires >= openAIAccountSelectionProbeLimit
}

func (b *openAISelectionProbeBudget) wasAttempted(accountID int64) bool {
	if b == nil {
		return false
	}
	_, ok := b.attempted[accountID]
	return ok
}

type openAIStickyEscapeConfig struct {
	enabled   bool
	ttftMs    float64
	errorRate float64
}

func newDefaultOpenAIAccountScheduler(service *OpenAIGatewayService, stats *openAIAccountRuntimeStats) OpenAIAccountScheduler {
	if stats == nil {
		stats = newOpenAIAccountRuntimeStats()
	}
	return &defaultOpenAIAccountScheduler{
		service: service,
		stats:   stats,
	}
}

func (s *defaultOpenAIAccountScheduler) Select(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (selection *AccountSelectionResult, decision OpenAIAccountScheduleDecision, err error) {
	ctx = withOpenAIImageSizeStickyAccount(withOpenAIImageSizeStickyAccount(ctx, req.StickyAccountID), req.StickyPreviousAccountID)
	start := time.Now()
	defer func() {
		decision.LatencyMs = time.Since(start).Milliseconds()
		s.metrics.recordSelect(decision)
	}()

	previousResponseID := strings.TrimSpace(req.PreviousResponseID)
	if previousResponseID != "" && normalizeOpenAICompatiblePlatform(req.Platform) == PlatformOpenAI &&
		(!req.StickyWeighted || !req.PreviousResponseCanMove) {
		selection, err := s.service.selectAccountByPreviousResponseIDForCapability(
			ctx,
			req.GroupID,
			previousResponseID,
			req.RequestedModel,
			req.ExcludedIDs,
			req.RequiredCapability,
			req.RequireCompact,
		)
		if err != nil {
			return nil, decision, err
		}
		if selection != nil && selection.Account != nil {
			vetoed, _ := openAIImageSizeVetoReason(withOpenAIImageSizeStickyAccount(ctx, selection.Account.ID), selection.Account)
			if vetoed || !s.isAccountTransportCompatible(selection.Account, req.RequiredTransport) {
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
				selection = nil
			}
		}
		if selection != nil && selection.Account != nil {
			escaped := false
			if req.PreviousResponseCanMove {
				escapeCfg := s.service.openAIStickyEscapeConfig()
				if reason, errorRate, ttft, shouldEscape := s.shouldEscapeStickyAccount(selection.Account.ID, escapeCfg); shouldEscape {
					if selection.ReleaseFunc != nil {
						selection.ReleaseFunc()
					}
					slog.Info("sticky_escape_triggered",
						"account_id", selection.Account.ID,
						"reason", "previous_response_"+reason,
						"error_rate", errorRate,
						"ttft", ttft,
					)
					req.PreserveStickyBinding = true
					req.ExcludedIDs = cloneExcludedAccountIDs(req.ExcludedIDs)
					if req.ExcludedIDs == nil {
						req.ExcludedIDs = make(map[int64]struct{}, 1)
					}
					req.ExcludedIDs[selection.Account.ID] = struct{}{}
					escaped = true
				}
			}
			if !escaped {
				decision.Layer = openAIAccountScheduleLayerPreviousResponse
				decision.StickyPreviousHit = true
				decision.SelectedAccountID = selection.Account.ID
				decision.SelectedAccountType = selection.Account.Type
				if req.SessionHash != "" && !req.PreserveStickyBinding {
					_ = s.service.bindOpenAIStickySessionDuringSelection(ctx, req.GroupID, req.SessionHash, selection.Account.ID)
				}
				return selection, decision, nil
			}
		}
	}

	contentSessionOverflow := openAIContentSessionRequestOverflow(ctx)
	if !req.StickyWeighted && !req.ContentSessionConcurrent && !contentSessionOverflow {
		selection, escapedStickyAccountID, err := s.selectBySessionHash(ctx, req)
		if err != nil {
			return nil, decision, err
		}
		if selection != nil && selection.Account != nil {
			decision.Layer = openAIAccountScheduleLayerSessionSticky
			decision.StickySessionHit = true
			decision.SelectedAccountID = selection.Account.ID
			decision.SelectedAccountType = selection.Account.Type
			return selection, decision, nil
		}
		if escapedStickyAccountID > 0 {
			req.PreserveStickyBinding = true
			req.ExcludedIDs = cloneExcludedAccountIDs(req.ExcludedIDs)
			if req.ExcludedIDs == nil {
				req.ExcludedIDs = make(map[int64]struct{}, 1)
			}
			req.ExcludedIDs[escapedStickyAccountID] = struct{}{}
		}
	}

	if contentSessionOverflow {
		selection, candidateErr := s.selectByContentSessionBurstCandidates(ctx, req)
		if candidateErr != nil {
			return nil, decision, candidateErr
		}
		if selection != nil && selection.Account != nil {
			decision.Layer = openAIAccountScheduleLayerContentBurst
			decision.SelectedAccountID = selection.Account.ID
			decision.SelectedAccountType = selection.Account.Type
			return selection, decision, nil
		}
	}

	if openAIContentSessionRequestTracked(ctx) {
		releaseLoadBalance, acquireErr := s.service.acquireOpenAIContentSessionLoadBalance(ctx)
		if acquireErr != nil {
			return nil, decision, acquireErr
		}
		defer releaseLoadBalance()

		// Overflow requests may have observed an empty binding before waiting for
		// admission. Re-read it now so only the first admitted overflow scans.
		if contentSessionOverflow {
			selection, candidateErr := s.selectByContentSessionBurstCandidates(ctx, req)
			if candidateErr != nil {
				return nil, decision, candidateErr
			}
			if selection != nil && selection.Account != nil {
				decision.Layer = openAIAccountScheduleLayerContentBurst
				decision.SelectedAccountID = selection.Account.ID
				decision.SelectedAccountType = selection.Account.Type
				return selection, decision, nil
			}

			stickyReq := req
			stickyReq.StickyAccountID = 0
			stickyReq.StickyWeighted = false
			selection, _, stickyErr := s.selectBySessionHash(ctx, stickyReq)
			if stickyErr != nil {
				return nil, decision, stickyErr
			}
			if selection != nil && selection.Account != nil {
				decision.Layer = openAIAccountScheduleLayerSessionSticky
				decision.StickySessionHit = true
				decision.SelectedAccountID = selection.Account.ID
				decision.SelectedAccountType = selection.Account.Type
				return selection, decision, nil
			}
		}
	}

	selection, candidateCount, topK, loadSkew, err := s.selectByLoadBalance(ctx, req)
	decision.Layer = openAIAccountScheduleLayerLoadBalance
	decision.CandidateCount = candidateCount
	decision.TopK = topK
	decision.LoadSkew = loadSkew
	if err != nil {
		return nil, decision, err
	}
	if selection != nil && selection.Account != nil {
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
		if req.StickyWeighted {
			if req.StickyPreviousAccountID > 0 && selection.Account.ID == req.StickyPreviousAccountID {
				decision.StickyPreviousHit = true
			}
			if req.StickyAccountID > 0 && selection.Account.ID == req.StickyAccountID {
				decision.StickySessionHit = true
			}
		}
	}
	return selection, decision, nil
}

func (s *defaultOpenAIAccountScheduler) selectBySessionHash(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, int64, error) {
	sessionHash := strings.TrimSpace(req.SessionHash)
	if sessionHash == "" || s == nil || s.service == nil {
		return nil, 0, nil
	}

	accountID := req.StickyAccountID
	if accountID <= 0 {
		if s.service.cache == nil {
			return nil, 0, nil
		}
		var err error
		accountID, err = s.service.getStickySessionAccountID(ctx, req.GroupID, sessionHash)
		if err != nil || accountID <= 0 {
			return nil, 0, nil
		}
	}
	if accountID <= 0 {
		return nil, 0, nil
	}
	if req.ExcludedIDs != nil {
		if _, excluded := req.ExcludedIDs[accountID]; excluded {
			return nil, 0, nil
		}
	}

	ctx = withOpenAIImageSizeStickyAccount(ctx, accountID)

	account, err := s.service.getSchedulableAccount(ctx, accountID)
	if err != nil {
		return nil, 0, nil
	}
	if account == nil {
		if req.EphemeralStickyAccount {
			s.service.dropOpenAIContentSessionCandidate(ctx, req.GroupID, sessionHash, accountID)
		} else {
			_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		}
		return nil, 0, nil
	}
	if shouldClearStickySession(account, req.RequestedModel) || account.Platform != normalizeOpenAICompatiblePlatform(req.Platform) || !account.IsOpenAICompatible() || !account.IsSchedulable() {
		if req.EphemeralStickyAccount {
			s.service.dropOpenAIContentSessionCandidate(ctx, req.GroupID, sessionHash, accountID)
		} else {
			_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		}
		return nil, 0, nil
	}
	if !s.isAccountRequestCompatible(ctx, account, req) ||
		!accountSupportsOpenAICapabilities(account, req.RequiredCapability, req.RequiredImageCapability) {
		return nil, 0, nil
	}
	if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
		if !req.EphemeralStickyAccount {
			_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		}
		return nil, 0, nil
	}
	account = s.service.recheckSelectedOpenAIAccountFromDB(ctx, account, req.GroupID, req.Platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability)
	if account == nil || !s.service.openAIAccountMatchesSchedulingGroup(account, req.GroupID) {
		if req.EphemeralStickyAccount {
			s.service.dropOpenAIContentSessionCandidate(ctx, req.GroupID, sessionHash, accountID)
		} else {
			_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		}
		return nil, 0, nil
	}
	if !s.isAccountTransportCompatible(account, req.RequiredTransport) ||
		!accountSupportsOpenAICapabilities(account, req.RequiredCapability, req.RequiredImageCapability) {
		if !req.EphemeralStickyAccount {
			_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		}
		return nil, 0, nil
	}
	if req.IsStreaming {
		if _, degraded := s.service.openAIStreamCandidateTier(account.ID, time.Now()); degraded {
			return nil, 0, nil
		}
	}
	escapeCfg := s.service.openAIStickyEscapeConfig()
	contentSessionOverflow := openAIContentSessionRequestOverflow(ctx)
	if reason, errorRate, ttft, shouldEscape := s.shouldEscapeStickyAccount(accountID, escapeCfg); !contentSessionOverflow && shouldEscape && !req.DisableStickyEscape {
		slog.Info("sticky_escape_triggered",
			"account_id", accountID,
			"reason", reason,
			"error_rate", errorRate,
			"ttft", ttft,
		)
		return nil, accountID, nil
	}
	result, acquireErr := s.service.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
	if errors.Is(acquireErr, ErrPriorityAdmissionUnavailable) {
		return nil, 0, acquireErr
	}
	if acquireErr != nil && req.DisableStickyEscape {
		return nil, 0, acquireErr
	}
	if acquireErr == nil && result != nil && result.Acquired {
		if !req.EphemeralStickyAccount {
			_ = s.service.refreshStickySessionTTL(ctx, req.GroupID, sessionHash, s.service.openAIWSSessionStickyTTL())
		}
		return &AccountSelectionResult{
			Account:     account,
			Acquired:    true,
			ReleaseFunc: result.ReleaseFunc,
		}, 0, nil
	}

	cfg := s.service.schedulingConfig()
	// Keep a healthy sticky conversation on its account while the bounded wait
	// queue still has room. Escaping on the first full-slot observation made
	// short bursts migrate to a cache-cold account even though the original
	// account could have admitted the request shortly afterwards. Only spill
	// over once the queue is saturated (or when no queue is configured).
	// WaitPlan.MaxConcurrency 使用 Concurrency（非 EffectiveLoadFactor），因为 WaitPlan 控制的是 Redis 实际并发槽位等待。
	if s.service.concurrencyService != nil {
		if escapeCfg.enabled && !req.DisableStickyEscape && !contentSessionOverflow && acquireErr == nil && result != nil && !result.Acquired {
			waitingCount, waitErr := s.service.concurrencyService.GetAccountWaitingCount(ctx, accountID)
			queueSaturated := cfg.StickySessionMaxWaiting <= 0 || (waitErr == nil && waitingCount >= cfg.StickySessionMaxWaiting)
			if queueSaturated {
				errorRate, ttft := 0.0, 0.0
				if s.stats != nil {
					errorRate, ttft, _ = s.stats.snapshot(accountID)
				}
				slog.Info("sticky_escape_triggered",
					"account_id", accountID,
					"reason", "concurrency_full_queue_saturated",
					"waiting_count", waitingCount,
					"max_waiting", cfg.StickySessionMaxWaiting,
					"error_rate", errorRate,
					"ttft", ttft,
				)
				return nil, accountID, nil
			}
		}
		return &AccountSelectionResult{
			Account: account,
			WaitPlan: &AccountWaitPlan{
				AccountID:      accountID,
				MaxConcurrency: account.Concurrency,
				Timeout:        cfg.StickySessionWaitTimeout,
				MaxWaiting:     cfg.StickySessionMaxWaiting,
			},
		}, 0, nil
	}
	return nil, 0, nil
}

// selectByContentSessionBurstCandidates keeps overflow scheduling on a bounded
// per-session candidate set. A busy candidate yields a normal WaitPlan rather
// than falling through to another full account-pool scan.
func (s *defaultOpenAIAccountScheduler) selectByContentSessionBurstCandidates(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, error) {
	candidates, count := s.service.nextOpenAIContentSessionBurstCandidates(ctx, req.GroupID, req.SessionHash)
	var waitSelection *AccountSelectionResult
	for i := 0; i < count; i++ {
		candidateReq := req
		candidateReq.StickyAccountID = candidates[i]
		candidateReq.StickyWeighted = false
		candidateReq.EphemeralStickyAccount = true
		selection, _, err := s.selectBySessionHash(ctx, candidateReq)
		if err != nil {
			return nil, err
		}
		if selection == nil || selection.Account == nil {
			continue
		}
		if selection.Acquired {
			return selection, nil
		}
		if selection.PriorityAdmissionTerminal {
			return selection, nil
		}
		if waitSelection == nil {
			waitSelection = selection
		}
	}
	return waitSelection, nil
}

func openAIStickyAccountMatchesGroup(account *Account, groupID *int64) bool {
	if account == nil {
		return false
	}
	if groupID == nil {
		return len(account.AccountGroups) == 0 && len(account.GroupIDs) == 0
	}
	for _, accountGroupID := range account.GroupIDs {
		if accountGroupID == *groupID {
			return true
		}
	}
	for _, accountGroup := range account.AccountGroups {
		if accountGroup.GroupID == *groupID {
			return true
		}
	}
	return false
}

func openAIAccountSchedulingPriority(account *Account) int {
	if account == nil {
		return 0
	}
	return account.Priority
}

func openAIAccountCandidatePoolOAuthOnly(candidates []*Account) bool {
	if len(candidates) == 0 {
		return false
	}
	for _, candidate := range candidates {
		if candidate == nil || !candidate.IsOpenAIOAuth() {
			return false
		}
	}
	return true
}

func (s *defaultOpenAIAccountScheduler) shouldEscapeStickyAccount(accountID int64, cfg openAIStickyEscapeConfig) (reason string, errorRate float64, ttft float64, shouldEscape bool) {
	if !cfg.enabled || s == nil || s.stats == nil || accountID <= 0 {
		return "", 0, 0, false
	}
	errorRate, ttft, hasTTFT := s.stats.snapshot(accountID)
	if hasTTFT && ttft > cfg.ttftMs {
		return "ttft", errorRate, ttft, true
	}
	if errorRate > cfg.errorRate {
		return "error_rate", errorRate, ttft, true
	}
	return "", errorRate, ttft, false
}

type openAIAccountCandidateScore struct {
	account            *Account
	loadInfo           *AccountLoadInfo
	loadKnown          bool
	channelKey         string
	selectionTieBreak  uint64
	score              float64
	upstreamCostFactor float64
	priority           int
	errorRate          float64
	ttft               float64
	hasTTFT            bool
	streamTier         int
	streamDegraded     bool
}

type openAIAccountCandidateHeap []openAIAccountCandidateScore

func (h openAIAccountCandidateHeap) Len() int {
	return len(h)
}

func (h openAIAccountCandidateHeap) Less(i, j int) bool {
	// 最小堆根节点保存“最差”候选，便于 O(log k) 维护 topK。
	return isOpenAIAccountCandidateBetter(h[j], h[i])
}

func (h openAIAccountCandidateHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *openAIAccountCandidateHeap) Push(x any) {
	candidate, ok := x.(openAIAccountCandidateScore)
	if !ok {
		panic("openAIAccountCandidateHeap: invalid element type")
	}
	*h = append(*h, candidate)
}

func (h *openAIAccountCandidateHeap) Pop() any {
	old := *h
	n := len(old)
	last := old[n-1]
	*h = old[:n-1]
	return last
}

func isOpenAIAccountCandidateBetter(left openAIAccountCandidateScore, right openAIAccountCandidateScore) bool {
	leftStreamTier := openAICandidateStreamTier(left)
	rightStreamTier := openAICandidateStreamTier(right)
	if leftStreamTier != rightStreamTier {
		return leftStreamTier < rightStreamTier
	}
	leftPriority := openAIAccountSchedulingPriority(left.account)
	rightPriority := openAIAccountSchedulingPriority(right.account)
	if leftPriority != rightPriority {
		return leftPriority < rightPriority
	}
	if left.score != right.score {
		return left.score > right.score
	}
	if left.loadInfo.LoadRate != right.loadInfo.LoadRate {
		return left.loadInfo.LoadRate < right.loadInfo.LoadRate
	}
	if left.loadInfo.WaitingCount != right.loadInfo.WaitingCount {
		return left.loadInfo.WaitingCount < right.loadInfo.WaitingCount
	}
	leftShare := 1
	if left.account != nil {
		leftShare = left.account.EffectiveLoadFactor()
	}
	rightShare := 1
	if right.account != nil {
		rightShare = right.account.EffectiveLoadFactor()
	}
	if leftShare != rightShare {
		return leftShare > rightShare
	}
	if left.selectionTieBreak != right.selectionTieBreak {
		return left.selectionTieBreak < right.selectionTieBreak
	}
	return left.account.ID < right.account.ID
}

func openAICandidateStreamTier(candidate openAIAccountCandidateScore) int {
	if !candidate.streamDegraded {
		return openAIStreamCandidateTierHealthy
	}
	return candidate.streamTier
}

func compareOpenAIAccountCandidates(left, right openAIAccountCandidateScore) int {
	switch {
	case isOpenAIAccountCandidateBetter(left, right):
		return -1
	case isOpenAIAccountCandidateBetter(right, left):
		return 1
	default:
		return 0
	}
}

func isOpenAIAccountCandidateWorse(left, right openAIAccountCandidateScore) bool {
	return isOpenAIAccountCandidateBetter(right, left)
}

func siftDownOpenAIAccountCandidateHeap(items []openAIAccountCandidateScore, root int) {
	for {
		child := root*2 + 1
		if child >= len(items) {
			return
		}
		if right := child + 1; right < len(items) && isOpenAIAccountCandidateWorse(items[right], items[child]) {
			child = right
		}
		if !isOpenAIAccountCandidateWorse(items[child], items[root]) {
			return
		}
		items[root], items[child] = items[child], items[root]
		root = child
	}
}

func initOpenAIAccountCandidateHeap(items []openAIAccountCandidateScore) {
	for root := len(items)/2 - 1; root >= 0; root-- {
		siftDownOpenAIAccountCandidateHeap(items, root)
	}
}

func selectSmallTopKOpenAICandidates(candidates []openAIAccountCandidateScore, topK int) []openAIAccountCandidateScore {
	best := make([]openAIAccountCandidateScore, 0, topK)
	for _, candidate := range candidates {
		insertAt := 0
		for insertAt < len(best) && !isOpenAIAccountCandidateBetter(candidate, best[insertAt]) {
			insertAt++
		}
		if len(best) < topK {
			best = append(best, openAIAccountCandidateScore{})
			copy(best[insertAt+1:], best[insertAt:len(best)-1])
			best[insertAt] = candidate
			continue
		}
		if insertAt < topK {
			copy(best[insertAt+1:], best[insertAt:topK-1])
			best[insertAt] = candidate
		}
	}
	return best
}

func selectTopKOpenAICandidates(candidates []openAIAccountCandidateScore, topK int) []openAIAccountCandidateScore {
	if len(candidates) == 0 {
		return nil
	}
	if topK <= 0 {
		topK = 1
	}
	if topK >= len(candidates) {
		ranked := append([]openAIAccountCandidateScore(nil), candidates...)
		slices.SortFunc(ranked, compareOpenAIAccountCandidates)
		return ranked
	}
	// The configured scheduler pool is normally small (default 7). Maintaining
	// a sorted bounded slice avoids interface boxing and a second result
	// allocation on this request hot path.
	if topK <= 16 && len(candidates) <= 64 {
		return selectSmallTopKOpenAICandidates(candidates, topK)
	}

	best := make(openAIAccountCandidateHeap, topK)
	copy(best, candidates[:topK])
	initOpenAIAccountCandidateHeap(best)
	for _, candidate := range candidates[topK:] {
		if isOpenAIAccountCandidateBetter(candidate, best[0]) {
			best[0] = candidate
			siftDownOpenAIAccountCandidateHeap(best, 0)
		}
	}

	ranked := make([]openAIAccountCandidateScore, len(best))
	copy(ranked, best)
	slices.SortFunc(ranked, compareOpenAIAccountCandidates)
	return ranked
}

type openAISelectionRNG struct {
	state uint64
}

func newOpenAISelectionRNG(seed uint64) openAISelectionRNG {
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	return openAISelectionRNG{state: seed}
}

func (r *openAISelectionRNG) nextUint64() uint64 {
	// xorshift64*
	x := r.state
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	r.state = x
	return x * 2685821657736338717
}

func (r *openAISelectionRNG) nextFloat64() float64 {
	// [0,1)
	return float64(r.nextUint64()>>11) / (1 << 53)
}

func deriveOpenAISelectionSeed(req OpenAIAccountScheduleRequest) uint64 {
	hasher := fnv.New64a()
	writeValue := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		_, _ = hasher.Write([]byte(trimmed))
		_, _ = hasher.Write([]byte{0})
	}

	writeValue(req.SessionHash)
	writeValue(req.PreviousResponseID)
	writeValue(req.RequestedModel)
	if req.GroupID != nil {
		_, _ = hasher.Write([]byte(strconv.FormatInt(*req.GroupID, 10)))
	}

	seed := hasher.Sum64()
	// 对“无会话锚点”的纯负载均衡请求，以及已从粘性账号溢出的请求
	// 引入时间熵，避免固定命中同一账号。保留粘性绑定时仍不改写绑定，
	// 这里只让备用候选在相同负载下拥有不同的平局顺序。
	if req.PreserveStickyBinding || ((strings.TrimSpace(req.SessionHash) == "" || req.ContentSessionConcurrent) && strings.TrimSpace(req.PreviousResponseID) == "") {
		seed ^= uint64(time.Now().UnixNano())
	}
	if seed == 0 {
		seed = uint64(time.Now().UnixNano()) ^ 0x9e3779b97f4a7c15
	}
	return seed
}

func buildOpenAIWeightedSelectionOrder(
	candidates []openAIAccountCandidateScore,
	req OpenAIAccountScheduleRequest,
) []openAIAccountCandidateScore {
	return buildOpenAIChannelAwareWeightedSelectionOrder(candidates, req)
}

func (s *defaultOpenAIAccountScheduler) buildOpenAIAccountLoadPlan(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	filtered []*Account,
	loadMap map[int64]*AccountLoadInfo,
) openAIAccountLoadPlan {
	allCandidates := make([]openAIAccountCandidateScore, 0, len(filtered))
	for _, account := range filtered {
		loadInfo, loadKnown := loadMap[account.ID]
		if !loadKnown || loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
			loadKnown = false
		}
		errorRate, ttft, hasTTFT := 0.0, 0.0, false
		if s.stats != nil {
			errorRate, ttft, hasTTFT = s.stats.snapshot(account.ID)
		}
		streamTier, streamDegraded := openAIStreamCandidateTierHealthy, false
		if req.IsStreaming && s.service != nil {
			streamTier, streamDegraded = s.service.openAIStreamCandidateTier(account.ID, time.Now())
		}
		allCandidates = append(allCandidates, openAIAccountCandidateScore{
			account:        account,
			loadInfo:       loadInfo,
			loadKnown:      loadKnown,
			channelKey:     openAIUpstreamChannelKey(account),
			errorRate:      errorRate,
			ttft:           ttft,
			hasTTFT:        hasTTFT,
			streamTier:     streamTier,
			streamDegraded: streamDegraded,
		})
	}
	// A bounded Top-K is important for large pools, but an ID-based final
	// tiebreak would permanently favor the first few OAuth accounts whenever
	// Redis reports the same load for all of them. Use request-scoped entropy
	// only for exact scheduling ties; real load, queue, priority and health
	// signals continue to dominate the ordering.
	if openAIAccountCandidatePoolOAuthOnly(filtered) {
		tieBreakRNG := newOpenAISelectionRNG(deriveOpenAISelectionSeed(req))
		for i := range allCandidates {
			allCandidates[i].selectionTieBreak = tieBreakRNG.nextUint64()
		}
	}

	candidates := allCandidates
	staleSnapshotCompactRetry := make([]openAIAccountCandidateScore, 0, len(allCandidates))
	if req.RequireCompact {
		candidates = make([]openAIAccountCandidateScore, 0, len(allCandidates))
		for _, candidate := range allCandidates {
			if openAICompactSupportTier(candidate.account) == 0 {
				staleSnapshotCompactRetry = append(staleSnapshotCompactRetry, candidate)
				continue
			}
			candidates = append(candidates, candidate)
		}
	}

	plan := openAIAccountLoadPlan{
		allCandidates:             allCandidates,
		candidates:                candidates,
		staleSnapshotCompactRetry: staleSnapshotCompactRetry,
		candidateCount:            len(candidates),
	}
	if len(candidates) == 0 {
		plan.selectionOrder = s.buildOpenAISelectionOrder(req, plan)
		return plan
	}

	minPriority, maxPriority := openAIAccountSchedulingPriority(candidates[0].account), openAIAccountSchedulingPriority(candidates[0].account)
	maxWaiting := 1
	loadRateSum := 0.0
	loadRateSumSquares := 0.0
	minTTFT, maxTTFT := 0.0, 0.0
	hasTTFTSample := false
	for i := range candidates {
		candidate := &candidates[i]
		candidate.priority = openAIAccountSchedulingPriority(candidate.account)
		if candidate.priority < minPriority {
			minPriority = candidate.priority
		}
		if candidate.priority > maxPriority {
			maxPriority = candidate.priority
		}
		if candidate.loadInfo.WaitingCount > maxWaiting {
			maxWaiting = candidate.loadInfo.WaitingCount
		}
		if candidate.hasTTFT && candidate.ttft > 0 {
			if !hasTTFTSample {
				minTTFT, maxTTFT = candidate.ttft, candidate.ttft
				hasTTFTSample = true
			} else {
				if candidate.ttft < minTTFT {
					minTTFT = candidate.ttft
				}
				if candidate.ttft > maxTTFT {
					maxTTFT = candidate.ttft
				}
			}
		}
		loadRate := float64(candidate.loadInfo.LoadRate)
		loadRateSum += loadRate
		loadRateSumSquares += loadRate * loadRate
	}
	plan.loadSkew = calcLoadSkewByMoments(loadRateSum, loadRateSumSquares, len(candidates))

	weights := s.service.openAIWSSchedulerWeightsForRequest(ctx)
	now := time.Now()
	if req.UseUpstreamTokenCost && weights.UpstreamCost > 0 {
		plan.includeOverflowFallback = applyOpenAIUpstreamCostFactors(candidates, now, s.service.openAIOAuthSchedulingRateMultiplier(ctx))
	}

	// Reset 因子（use-it-or-lose-it）：优先读取 Codex 5h 重置时间，回退到会话窗口。
	// 剩余时间越短 → 因子越接近 1（越早重置越优先用尽）。无活跃窗口的账号因子为 0。
	// 仅在 weights.Reset > 0 时计算，默认关闭不影响原有行为。
	minResetRemaining, maxResetRemaining := 0.0, 0.0
	hasResetSample := false
	if weights.Reset > 0 {
		for _, candidate := range candidates {
			end, ok := openAISchedulingResetWindowEnd(candidate.account, now)
			if !ok {
				continue
			}
			remaining := end.Sub(now).Seconds()
			if !hasResetSample {
				minResetRemaining, maxResetRemaining = remaining, remaining
				hasResetSample = true
				continue
			}
			if remaining < minResetRemaining {
				minResetRemaining = remaining
			}
			if remaining > maxResetRemaining {
				maxResetRemaining = remaining
			}
		}
	}

	for i := range candidates {
		item := &candidates[i]
		priorityFactor := 1.0
		if maxPriority > minPriority {
			priorityFactor = 1 - float64(item.priority-minPriority)/float64(maxPriority-minPriority)
		}
		loadFactor := 1 - clamp01(float64(item.loadInfo.LoadRate)/100.0)
		queueFactor := 1 - clamp01(float64(item.loadInfo.WaitingCount)/float64(maxWaiting))
		errorFactor := 1 - clamp01(item.errorRate)
		ttftFactor := 0.5
		if item.hasTTFT && hasTTFTSample && maxTTFT > minTTFT {
			ttftFactor = 1 - clamp01((item.ttft-minTTFT)/(maxTTFT-minTTFT))
		}
		resetFactor := 0.0
		if weights.Reset > 0 && hasResetSample {
			if end, ok := openAISchedulingResetWindowEnd(item.account, now); ok {
				if maxResetRemaining > minResetRemaining {
					resetFactor = 1 - clamp01((end.Sub(now).Seconds()-minResetRemaining)/(maxResetRemaining-minResetRemaining))
				} else {
					// 所有有窗口的账号剩余时间相同：一律给满分，让其优于无窗口账号。
					resetFactor = 1
				}
			}
		}
		quotaHeadroomFactor := 0.0
		if weights.QuotaHeadroom > 0 {
			quotaHeadroomFactor = openAIQuotaHeadroomFactor(item.account, now)
		}
		upstreamCostFactor := openAIUpstreamCostNeutralFactor
		if req.UseUpstreamTokenCost && weights.UpstreamCost > 0 {
			upstreamCostFactor = item.upstreamCostFactor
		}

		item.score = weights.Priority*priorityFactor +
			weights.Load*loadFactor +
			weights.Queue*queueFactor +
			weights.ErrorRate*errorFactor +
			weights.TTFT*ttftFactor +
			weights.Reset*resetFactor +
			weights.QuotaHeadroom*quotaHeadroomFactor +
			weights.UpstreamCost*(upstreamCostFactor-openAIUpstreamCostNeutralFactor)
		if req.StickyWeighted {
			if req.PreviousResponseCanMove && req.StickyPreviousAccountID > 0 && item.account.ID == req.StickyPreviousAccountID {
				item.score += weights.Previous
			}
			if !req.ContentSessionConcurrent && req.StickyAccountID > 0 && item.account.ID == req.StickyAccountID {
				item.score += weights.SessionSticky
			}
		}
	}
	plan.candidates = candidates

	plan.topK = s.service.openAIWSLBTopKForRequest(ctx)
	if plan.topK > len(candidates) {
		plan.topK = len(candidates)
	}
	if plan.topK <= 0 {
		plan.topK = 1
	}

	plan.selectionOrder = s.buildOpenAISelectionOrder(req, plan)
	return plan
}

func (s *defaultOpenAIAccountScheduler) buildOpenAISelectionOrder(
	req OpenAIAccountScheduleRequest,
	plan openAIAccountLoadPlan,
) []openAIAccountCandidateScore {
	buildPrioritySelectionOrder := func(pool []openAIAccountCandidateScore, includeOverflow bool) []openAIAccountCandidateScore {
		if len(pool) == 0 || plan.topK <= 0 {
			return nil
		}
		groupTopK := plan.topK
		if groupTopK > len(pool) {
			groupTopK = len(pool)
		}
		ranked := selectTopKOpenAICandidatesByChannel(pool, groupTopK)
		var primary []openAIAccountCandidateScore
		if req.StickyWeighted {
			sessionStickyID := req.StickyAccountID
			if req.ContentSessionConcurrent {
				sessionStickyID = 0
			}
			for _, stickyID := range []int64{req.StickyPreviousAccountID, sessionStickyID} {
				if stickyID <= 0 {
					continue
				}
				for i, candidate := range ranked {
					if candidate.account != nil && candidate.account.ID == stickyID {
						primary = append([]openAIAccountCandidateScore{candidate}, ranked[:i]...)
						primary = append(primary, ranked[i+1:]...)
						break
					}
				}
				if len(primary) > 0 {
					break
				}
			}
		}
		if len(primary) == 0 {
			primary = buildOpenAIWeightedSelectionOrder(ranked, req)
		}
		if (!plan.includeOverflowFallback && !includeOverflow) || groupTopK >= len(pool) {
			return primary
		}

		selected := make(map[int64]struct{}, len(primary))
		for _, candidate := range primary {
			selected[candidate.account.ID] = struct{}{}
		}
		overflow := make([]openAIAccountCandidateScore, 0, len(pool)-len(primary))
		for _, candidate := range pool {
			if _, ok := selected[candidate.account.ID]; !ok {
				overflow = append(overflow, candidate)
			}
		}
		sort.Slice(overflow, func(i, j int) bool {
			return isOpenAIAccountCandidateBetter(overflow[i], overflow[j])
		})
		return append(primary, overflow...)
	}

	buildSelectionOrder := func(pool []openAIAccountCandidateScore) []openAIAccountCandidateScore {
		priorityLevels := openAICandidatePriorityLevels(pool)
		selectionOrder := make([]openAIAccountCandidateScore, 0, len(pool))
		for i, priority := range priorityLevels {
			priorityCandidates := make([]openAIAccountCandidateScore, 0, len(pool))
			for _, candidate := range pool {
				if openAICandidatePriorityLevelFor(candidate) == priority {
					priorityCandidates = append(priorityCandidates, candidate)
				}
			}
			selectionOrder = append(selectionOrder, buildPrioritySelectionOrder(priorityCandidates, i < len(priorityLevels)-1)...)
		}
		if req.StickyWeighted {
			globalTopK := plan.topK
			if globalTopK > len(pool) {
				globalTopK = len(pool)
			}
			ranked := selectTopKOpenAICandidatesByChannel(pool, globalTopK)
			sessionStickyID := req.StickyAccountID
			if req.ContentSessionConcurrent {
				sessionStickyID = 0
			}
			for _, stickyID := range []int64{req.StickyPreviousAccountID, sessionStickyID} {
				eligible := false
				for _, candidate := range ranked {
					if candidate.account != nil && candidate.account.ID == stickyID {
						eligible = true
						break
					}
				}
				if !eligible {
					continue
				}
				for i, candidate := range selectionOrder {
					if candidate.account != nil && candidate.account.ID == stickyID {
						if req.IsStreaming && candidate.streamDegraded {
							continue
						}
						copy(selectionOrder[1:i+1], selectionOrder[:i])
						selectionOrder[0] = candidate
						return selectionOrder
					}
				}
			}
		}
		return selectionOrder
	}

	if req.RequireCompact {
		supported := make([]openAIAccountCandidateScore, 0, len(plan.candidates))
		unknown := make([]openAIAccountCandidateScore, 0, len(plan.candidates))
		for _, candidate := range plan.candidates {
			switch openAICompactSupportTier(candidate.account) {
			case 2:
				supported = append(supported, candidate)
			case 1:
				unknown = append(unknown, candidate)
			}
		}
		selectionOrder := make([]openAIAccountCandidateScore, 0, len(plan.allCandidates))
		selectionOrder = append(selectionOrder, buildSelectionOrder(supported)...)
		selectionOrder = append(selectionOrder, buildSelectionOrder(unknown)...)
		if len(plan.staleSnapshotCompactRetry) > 0 && s.service.schedulerSnapshot != nil {
			selectionOrder = append(selectionOrder, sortOpenAICompactRetryCandidates(plan.staleSnapshotCompactRetry)...)
		}
		return selectionOrder
	}

	return buildSelectionOrder(plan.candidates)
}

func sortOpenAICompactRetryCandidates(pool []openAIAccountCandidateScore) []openAIAccountCandidateScore {
	if len(pool) == 0 {
		return nil
	}
	ordered := append([]openAIAccountCandidateScore(nil), pool...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if openAICandidateStreamTier(a) != openAICandidateStreamTier(b) {
			return openAICandidateStreamTier(a) < openAICandidateStreamTier(b)
		}
		if a.account.Priority != b.account.Priority {
			return a.account.Priority < b.account.Priority
		}
		if a.loadInfo.LoadRate != b.loadInfo.LoadRate {
			return a.loadInfo.LoadRate < b.loadInfo.LoadRate
		}
		if a.loadInfo.WaitingCount != b.loadInfo.WaitingCount {
			return a.loadInfo.WaitingCount < b.loadInfo.WaitingCount
		}
		switch {
		case a.account.LastUsedAt == nil && b.account.LastUsedAt != nil:
			return true
		case a.account.LastUsedAt != nil && b.account.LastUsedAt == nil:
			return false
		case a.account.LastUsedAt == nil && b.account.LastUsedAt == nil:
			return false
		default:
			return a.account.LastUsedAt.Before(*b.account.LastUsedAt)
		}
	})
	return ordered
}

func (s *defaultOpenAIAccountScheduler) tryAcquireOpenAISelectionOrder(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	selectionOrder []openAIAccountCandidateScore,
) (*AccountSelectionResult, bool, error) {
	budget := newOpenAISelectionProbeBudget()
	budget.enableLimit()
	return s.tryAcquireOpenAISelectionOrderWithBudget(ctx, req, selectionOrder, budget)
}

func (s *defaultOpenAIAccountScheduler) tryAcquireOpenAISelectionOrderWithBudget(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	selectionOrder []openAIAccountCandidateScore,
	budget *openAISelectionProbeBudget,
) (*AccountSelectionResult, bool, error) {
	compactBlocked := false
	hasHealthyCandidate := false
	if req.IsStreaming {
		for _, candidate := range selectionOrder {
			if candidate.account != nil && !candidate.streamDegraded {
				hasHealthyCandidate = true
				break
			}
		}
	}
	release := func(result *AcquireResult) {
		if result != nil && result.ReleaseFunc != nil {
			result.ReleaseFunc()
		}
	}
	for i := 0; i < len(selectionOrder); i++ {
		candidate := selectionOrder[i]
		if candidate.account == nil {
			continue
		}
		if candidate.loadKnown && candidate.account.Concurrency > 0 &&
			candidate.loadInfo.CurrentConcurrency >= candidate.account.Concurrency {
			continue
		}

		result, attempted, acquireErr := s.tryAcquireOpenAIAccountSlot(ctx, candidate.account.ID, candidate.account.Concurrency, budget)
		if !attempted {
			break
		}
		if acquireErr != nil {
			return nil, compactBlocked, acquireErr
		}
		if result == nil || !result.Acquired {
			if result != nil && result.PriorityAdmissionTerminal {
				cfg := s.service.schedulingConfig()
				return priorityAdmissionTerminalSelection(candidate.account, cfg.FallbackWaitTimeout, cfg.FallbackMaxWaiting), compactBlocked, nil
			}
			continue
		}

		fresh := s.service.resolveFreshSchedulableOpenAIAccount(ctx, candidate.account, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.isAccountRequestCompatible(ctx, fresh, req) {
			release(result)
			continue
		}
		if !s.consumeOpenAISelectionDBRecheck(budget) {
			release(result)
			break
		}
		fresh = s.service.recheckSelectedOpenAIAccountFromDB(ctx, fresh, req.GroupID, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.isAccountRequestCompatible(ctx, fresh, req) {
			release(result)
			continue
		}
		if req.RequireCompact && openAICompactSupportTier(fresh) == 0 {
			compactBlocked = true
			release(result)
			continue
		}

		if fresh.Concurrency != candidate.account.Concurrency {
			release(result)
			result, attempted, acquireErr = s.tryAcquireOpenAIAccountSlot(ctx, fresh.ID, fresh.Concurrency, budget)
			if !attempted {
				continue
			}
			if acquireErr != nil {
				return nil, compactBlocked, acquireErr
			}
			if result == nil || !result.Acquired {
				continue
			}
		}
		if req.IsStreaming {
			streamTier, degraded := s.service.openAIStreamCandidateTier(fresh.ID, time.Now())
			if degraded && hasHealthyCandidate {
				if streamTier != openAIStreamCandidateTierProbe || !s.service.claimOpenAIStreamRecoveryProbe(fresh.ID, time.Now()) {
					release(result)
					continue
				}
			}
		}
		if req.SessionHash != "" && !req.PreserveStickyBinding {
			_ = s.service.bindOpenAIStickySessionDuringSelection(ctx, req.GroupID, req.SessionHash, fresh.ID)
		}
		return &AccountSelectionResult{
			Account:     fresh,
			Acquired:    true,
			ReleaseFunc: result.ReleaseFunc,
		}, compactBlocked, nil
	}
	return nil, compactBlocked, nil
}

func (s *defaultOpenAIAccountScheduler) tryAcquireOpenAIAccountSlot(
	ctx context.Context,
	accountID int64,
	maxConcurrency int,
	budget *openAISelectionProbeBudget,
) (*AcquireResult, bool, error) {
	if s.service.concurrencyService != nil && maxConcurrency > 0 && !budget.recordAcquire(accountID) {
		return nil, false, nil
	}
	result, err := s.service.tryAcquireAccountSlot(ctx, accountID, maxConcurrency)
	return result, true, err
}

func (s *defaultOpenAIAccountScheduler) consumeOpenAISelectionDBRecheck(budget *openAISelectionProbeBudget) bool {
	if s.service.schedulerSnapshot == nil || s.service.accountRepo == nil {
		return true
	}
	return budget.recordRecheck()
}

func (s *defaultOpenAIAccountScheduler) tryFallbackToWeightedSticky(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, error) {
	if !req.StickyWeighted {
		return nil, nil
	}
	sessionStickyID := req.StickyAccountID
	if req.ContentSessionConcurrent {
		sessionStickyID = 0
	}
	for _, accountID := range []int64{req.StickyPreviousAccountID, sessionStickyID} {
		if accountID <= 0 {
			continue
		}
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[accountID]; excluded {
				continue
			}
		}
		account, err := s.service.getSchedulableAccount(ctx, accountID)
		if err != nil || account == nil {
			continue
		}
		if !s.isAccountRequestCompatible(ctx, account, req) || !s.isAccountTransportCompatible(account, req.RequiredTransport) {
			continue
		}
		account = s.service.recheckSelectedOpenAIAccountFromDB(ctx, account, req.GroupID, req.Platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability)
		if account == nil {
			if accountID == req.StickyAccountID && strings.TrimSpace(req.SessionHash) != "" {
				_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, req.SessionHash)
			}
			continue
		}
		if !s.service.openAIAccountMatchesSchedulingGroup(account, req.GroupID) {
			if accountID == req.StickyAccountID && strings.TrimSpace(req.SessionHash) != "" {
				_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, req.SessionHash)
			}
			continue
		}
		if !s.isAccountRequestCompatible(ctx, account, req) || !s.isAccountTransportCompatible(account, req.RequiredTransport) {
			continue
		}
		if req.RequireCompact && openAICompactSupportTier(account) == 0 {
			continue
		}
		if req.IsStreaming {
			if _, degraded := s.service.openAIStreamCandidateTier(account.ID, time.Now()); degraded {
				continue
			}
		}
		result, acquireErr := s.service.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
		if acquireErr != nil {
			return nil, acquireErr
		}
		if result != nil && result.Acquired {
			if req.SessionHash != "" && !req.PreserveStickyBinding {
				_ = s.service.bindOpenAIStickySessionDuringSelection(ctx, req.GroupID, req.SessionHash, account.ID)
			}
			return &AccountSelectionResult{
				Account:     account,
				Acquired:    true,
				ReleaseFunc: result.ReleaseFunc,
			}, nil
		}
		if s.service.concurrencyService != nil {
			cfg := s.service.schedulingConfig()
			return &AccountSelectionResult{
				Account: account,
				WaitPlan: &AccountWaitPlan{
					AccountID:      account.ID,
					MaxConcurrency: account.Concurrency,
					Timeout:        cfg.StickySessionWaitTimeout,
					MaxWaiting:     cfg.StickySessionMaxWaiting,
				},
			}, nil
		}
	}
	return nil, nil
}

// openAISelectionFilterStats counts why candidates were dropped by the
// selectByLoadBalance initial filter. Historically these exclusions were
// silent (debug logs at best), so a "no available accounts" failure with
// excluded_account_count=0 was undiagnosable from the error alone (#4599).
// The reasons map is lazily allocated: on the happy path (nothing filtered
// out, or an account is eventually selected) no extra allocation happens.
type openAISelectionFilterStats struct {
	pool    int
	reasons map[string]int
}

func (s *openAISelectionFilterStats) exclude(reason string) {
	if s.reasons == nil {
		s.reasons = make(map[string]int, 4)
	}
	s.reasons[reason]++
}

// summary renders deterministic exclusion statistics for scheduling error
// messages, e.g. "pool=3, filtered: model_not_supported=2 quota_auto_pause_7d=1".
// Reasons are sorted lexicographically so the output is stable for tests and
// log aggregation. extra, when non-empty, is appended as a trailing marker.
func (s openAISelectionFilterStats) summary(extra string) string {
	var b strings.Builder
	_, _ = b.WriteString("pool=")
	_, _ = b.WriteString(strconv.Itoa(s.pool))
	if len(s.reasons) > 0 {
		reasons := make([]string, 0, len(s.reasons))
		for reason := range s.reasons {
			reasons = append(reasons, reason)
		}
		sort.Strings(reasons)
		_, _ = b.WriteString(", filtered:")
		for _, reason := range reasons {
			_, _ = b.WriteString(" ")
			_, _ = b.WriteString(reason)
			_, _ = b.WriteString("=")
			_, _ = b.WriteString(strconv.Itoa(s.reasons[reason]))
		}
	}
	if extra != "" {
		_, _ = b.WriteString(", ")
		_, _ = b.WriteString(extra)
	}
	return b.String()
}

// selectionFailureDetails computes exclusion statistics only after selection
// has failed. The normal path keeps the request-aware snapshot predicate and
// avoids allocating a reason map or sorting reason keys.
func (s *defaultOpenAIAccountScheduler) selectionFailureDetails(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	extra string,
) string {
	if ctx == nil {
		return openAISelectionFilterStats{}.summary(extra)
	}
	accounts, err := s.service.listSchedulableAccounts(ctx, req.GroupID, req.Platform)
	if err != nil {
		return openAISelectionFilterStats{}.summary(extra)
	}

	var schedGroup *Group
	if req.GroupID != nil && s.service.schedulerSnapshot != nil {
		schedGroup, _ = s.service.schedulerSnapshot.GetGroupByIDLite(ctx, *req.GroupID)
	}

	stats := openAISelectionFilterStats{pool: len(accounts)}
	for i := range accounts {
		account := &accounts[i]
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[account.ID]; excluded {
				stats.exclude("excluded")
				continue
			}
		}
		if !account.IsSchedulable() {
			stats.exclude("not_schedulable")
			continue
		}
		if account.Platform != normalizeOpenAICompatiblePlatform(req.Platform) || !account.IsOpenAICompatible() {
			stats.exclude("platform_mismatch")
			continue
		}
		if s.service.isOpenAIAccountRequestRuntimeBlocked(account, req.RequestedModel) {
			stats.exclude("runtime_blocked")
			continue
		}
		if schedGroup != nil && schedGroup.RequirePrivacySet && !account.IsPrivacySet() {
			stats.exclude("privacy_not_set")
			continue
		}
		if compatible, reason := s.isAccountRequestCompatibleReason(ctx, account, req); !compatible {
			stats.exclude(reason)
			continue
		}
		if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
			stats.exclude("transport_incompatible")
		}
	}
	return stats.summary(extra)
}

func (s *defaultOpenAIAccountScheduler) selectByLoadBalance(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, int, int, float64, error) {
	budget := newOpenAISelectionProbeBudget()
	diagnosticCtx := ctx
	ctx = withSchedulerCandidateExclusions(ctx, req.ExcludedIDs)
	ctx = s.service.withOpenAISchedulerCandidateFilter(ctx, req.GroupID, req.Platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability)
	requestCtx := ctx
	ctx = withSchedulerCandidatePredicate(ctx, func(account *Account) bool {
		return s.isAccountRequestCompatible(requestCtx, account, req) &&
			s.isAccountTransportCompatible(account, req.RequiredTransport)
	})
	accounts, err := s.service.listSchedulableAccounts(ctx, req.GroupID, req.Platform)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	if len(accounts) == 0 {
		details := s.selectionFailureDetails(diagnosticCtx, req, "")
		return nil, 0, 0, 0, noAvailableOpenAISelectionError(req.RequestedModel, false, details)
	}

	// require_privacy_set: 获取分组信息
	var schedGroup *Group
	if req.GroupID != nil && s.service.schedulerSnapshot != nil {
		schedGroup, _ = s.service.schedulerSnapshot.GetGroupByIDLite(ctx, *req.GroupID)
	}

	filtered := make([]*Account, 0, len(accounts))
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[account.ID]; excluded {
				continue
			}
		}
		if !account.IsSchedulable() {
			continue
		}
		if account.Platform != normalizeOpenAICompatiblePlatform(req.Platform) || !account.IsOpenAICompatible() {
			continue
		}
		if s.service.isOpenAIAccountRequestRuntimeBlocked(account, req.RequestedModel) {
			continue
		}
		// require_privacy_set: 跳过 privacy 未设置的账号并标记异常
		if schedGroup != nil && schedGroup.RequirePrivacySet && !account.IsPrivacySet() {
			s.service.BlockAccountScheduling(account, time.Time{}, "privacy_not_set")
			_ = s.service.accountRepo.SetError(ctx, account.ID,
				fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
			continue
		}
		if compatible, _ := s.isAccountRequestCompatibleReason(ctx, account, req); !compatible {
			continue
		}
		if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
			continue
		}
		filtered = append(filtered, account)
		loadReq = append(loadReq, AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: account.EffectiveLoadFactor(),
		})
	}
	if len(filtered) == 0 {
		details := s.selectionFailureDetails(diagnosticCtx, req, "")
		return nil, 0, 0, 0, noAvailableOpenAISelectionError(req.RequestedModel, false, details)
	}

	loadMap := map[int64]*AccountLoadInfo{}
	if s.service.concurrencyService != nil {
		if batchLoad, loadErr := s.service.concurrencyService.GetAccountsLoadBatch(ctx, loadReq); loadErr == nil {
			loadMap = batchLoad
		}
	}

	if req.SubscriptionPriority {
		subscriptionAccounts, regularAccounts := partitionOpenAIChatGPTSubscriptionAccounts(filtered)
		if len(subscriptionAccounts) > 0 {
			attempt := s.trySelectByLoadBalancePool(ctx, req, subscriptionAccounts, loadMap, budget)
			if attempt.err != nil && (!attempt.noCompactCandidates || len(regularAccounts) <= 0) {
				return nil, attempt.candidateCount, attempt.topK, attempt.loadSkew, attempt.err
			}
			if attempt.result != nil {
				return attempt.result, attempt.candidateCount, attempt.topK, attempt.loadSkew, nil
			}
			if len(regularAccounts) > 0 {
				regularAttempt := s.trySelectByLoadBalancePool(ctx, req, regularAccounts, loadMap, budget)
				if regularAttempt.err != nil && !regularAttempt.noCompactCandidates {
					return nil, regularAttempt.candidateCount, regularAttempt.topK, regularAttempt.loadSkew, regularAttempt.err
				}
				if regularAttempt.result != nil {
					return regularAttempt.result, regularAttempt.candidateCount, regularAttempt.topK, regularAttempt.loadSkew, nil
				}
				var result *AccountSelectionResult
				candidateCount, topK, loadSkew := regularAttempt.candidateCount, regularAttempt.topK, regularAttempt.loadSkew
				fallbackErr := regularAttempt.err
				if regularAttempt.err == nil {
					result, candidateCount, topK, loadSkew, fallbackErr = s.finishLoadBalanceSelectionFallback(ctx, diagnosticCtx, req, regularAttempt, budget)
					if fallbackErr == nil && result != nil {
						return result, candidateCount, topK, loadSkew, nil
					}
				}
				// 常规池既无法获取也无法排队（含仅剩不支持 compact 的候选）时，
				// 回退到订阅池的等待计划：busy-but-waitable 的订阅账号不应因常规池存在
				// 而被丢弃，否则开启订阅优先反而让本可排队成功的请求硬失败。
				subResult, subCandidateCount, subTopK, subLoadSkew, subErr := s.finishLoadBalanceSelectionFallback(ctx, diagnosticCtx, req, attempt, budget)
				if subErr == nil && subResult != nil {
					return subResult, subCandidateCount, subTopK, subLoadSkew, nil
				}
				return result, candidateCount, topK, loadSkew, fallbackErr
			}
			return s.finishLoadBalanceSelectionFallback(ctx, diagnosticCtx, req, attempt, budget)
		}
	}

	attempt := s.trySelectByLoadBalancePool(ctx, req, filtered, loadMap, budget)
	if attempt.err != nil {
		return nil, attempt.candidateCount, attempt.topK, attempt.loadSkew, attempt.err
	}
	if attempt.result != nil {
		return attempt.result, attempt.candidateCount, attempt.topK, attempt.loadSkew, nil
	}
	return s.finishLoadBalanceSelectionFallback(ctx, diagnosticCtx, req, attempt, budget)
}

func partitionOpenAIChatGPTSubscriptionAccounts(accounts []*Account) ([]*Account, []*Account) {
	subscriptionAccounts := make([]*Account, 0, len(accounts))
	regularAccounts := make([]*Account, 0, len(accounts))
	for _, account := range accounts {
		if account != nil && account.IsOpenAIChatGPTSubscription() {
			subscriptionAccounts = append(subscriptionAccounts, account)
			continue
		}
		regularAccounts = append(regularAccounts, account)
	}
	return subscriptionAccounts, regularAccounts
}

func (s *defaultOpenAIAccountScheduler) trySelectByLoadBalancePool(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	filtered []*Account,
	loadMap map[int64]*AccountLoadInfo,
	budget *openAISelectionProbeBudget,
) openAIAccountLoadSelectionAttempt {
	plan := s.buildOpenAIAccountLoadPlan(ctx, req, filtered, loadMap)
	if openAICostOverflowExpanded(req, plan) {
		budget.enableLimit()
	}
	attempt := openAIAccountLoadSelectionAttempt{
		selectionOrder: plan.selectionOrder,
		candidateCount: plan.candidateCount,
		topK:           plan.topK,
		loadSkew:       plan.loadSkew,
	}
	if req.RequireCompact && len(plan.candidates) == 0 && len(plan.staleSnapshotCompactRetry) == 0 {
		attempt.noCompactCandidates = true
		attempt.err = ErrNoAvailableCompactAccounts
		return attempt
	}
	if req.RequireCompact && len(attempt.selectionOrder) == 0 && s.service.schedulerSnapshot == nil {
		attempt.noCompactCandidates = true
		attempt.err = ErrNoAvailableCompactAccounts
		return attempt
	}
	if len(attempt.selectionOrder) == 0 {
		attempt.compactBlocked = req.RequireCompact && len(plan.allCandidates) > 0
		return attempt
	}

	result, compactBlocked, acquireErr := s.tryAcquireOpenAISelectionOrderWithBudget(ctx, req, attempt.selectionOrder, budget)
	attempt.compactBlocked = compactBlocked
	if acquireErr != nil {
		attempt.err = acquireErr
		return attempt
	}
	if result != nil {
		attempt.result = result
		return attempt
	}

	if s.service.concurrencyService != nil && !budget.acquireExhausted() {
		loadReq := buildOpenAIAccountLoadRequest(filtered)
		if freshLoadMap, loadErr := s.service.concurrencyService.GetAccountsLoadBatchFresh(ctx, loadReq); loadErr == nil {
			freshPlan := s.buildOpenAIAccountLoadPlan(ctx, req, filtered, freshLoadMap)
			if openAICostOverflowExpanded(req, freshPlan) {
				budget.enableLimit()
			}
			if len(freshPlan.selectionOrder) > 0 {
				freshResult, freshCompactBlocked, freshAcquireErr := s.tryAcquireOpenAISelectionOrderWithBudget(ctx, req, freshPlan.selectionOrder, budget)
				if freshAcquireErr != nil {
					attempt.err = freshAcquireErr
					return attempt
				}
				if freshResult != nil {
					attempt.result = freshResult
					attempt.selectionOrder = freshPlan.selectionOrder
					attempt.candidateCount = freshPlan.candidateCount
					attempt.topK = freshPlan.topK
					attempt.loadSkew = freshPlan.loadSkew
					return attempt
				}
				attempt.compactBlocked = attempt.compactBlocked || freshCompactBlocked
				attempt.selectionOrder = freshPlan.selectionOrder
				attempt.candidateCount = freshPlan.candidateCount
				attempt.topK = freshPlan.topK
				attempt.loadSkew = freshPlan.loadSkew
			}
		}
	}

	return attempt
}

func openAICostOverflowExpanded(req OpenAIAccountScheduleRequest, plan openAIAccountLoadPlan) bool {
	if !plan.includeOverflowFallback || plan.topK <= 0 {
		return false
	}
	if !req.RequireCompact {
		return len(plan.candidates) > plan.topK
	}
	supported, unknown := 0, 0
	for _, candidate := range plan.candidates {
		switch openAICompactSupportTier(candidate.account) {
		case 2:
			supported++
		case 1:
			unknown++
		}
	}
	return supported > plan.topK || unknown > plan.topK
}

func buildOpenAIAccountLoadRequest(accounts []*Account) []AccountWithConcurrency {
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		loadReq = append(loadReq, AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: account.EffectiveLoadFactor(),
		})
	}
	return loadReq
}

func (s *defaultOpenAIAccountScheduler) finishLoadBalanceSelectionFallback(
	ctx context.Context,
	diagnosticCtx context.Context,
	req OpenAIAccountScheduleRequest,
	attempt openAIAccountLoadSelectionAttempt,
	budget *openAISelectionProbeBudget,
) (*AccountSelectionResult, int, int, float64, error) {
	candidateCount := attempt.candidateCount
	topK := attempt.topK
	loadSkew := attempt.loadSkew

	if len(attempt.selectionOrder) == 0 {
		details := s.selectionFailureDetails(diagnosticCtx, req, "selection_order_empty")
		return nil, candidateCount, topK, loadSkew, noAvailableOpenAISelectionError(req.RequestedModel, attempt.compactBlocked, details)
	}

	if stickyFallback, stickyErr := s.tryFallbackToWeightedSticky(ctx, req); stickyErr != nil {
		return nil, candidateCount, topK, loadSkew, stickyErr
	} else if stickyFallback != nil {
		return stickyFallback, candidateCount, topK, loadSkew, nil
	}

	cfg := s.service.schedulingConfig()
	compactBlocked := attempt.compactBlocked
	hasHealthyCandidate := false
	if req.IsStreaming {
		for _, candidate := range attempt.selectionOrder {
			if candidate.account != nil && !candidate.streamDegraded {
				hasHealthyCandidate = true
				break
			}
		}
	}
	// WaitPlan.MaxConcurrency 使用 Concurrency（非 EffectiveLoadFactor），因为 WaitPlan 控制的是 Redis 实际并发槽位等待。
	passes := 1
	if budget != nil && budget.limited {
		passes = 4
	}
	for pass := 0; pass < passes; pass++ {
		wantAttempted := pass == 1 || pass == 3
		wantKnownFull := pass >= 2
		for _, candidate := range attempt.selectionOrder {
			if candidate.account == nil {
				continue
			}
			if req.IsStreaming && hasHealthyCandidate && candidate.streamDegraded &&
				openAICandidateStreamTier(candidate) == openAIStreamCandidateTierProbe {
				continue
			}
			if budget != nil && budget.limited {
				knownFull := candidate.loadKnown && candidate.account.Concurrency > 0 &&
					candidate.loadInfo.CurrentConcurrency >= candidate.account.Concurrency
				if budget.wasAttempted(candidate.account.ID) != wantAttempted || knownFull != wantKnownFull {
					continue
				}
			}
			fresh := s.service.resolveFreshSchedulableOpenAIAccount(ctx, candidate.account, req.Platform, req.RequestedModel, false, req.RequiredCapability)
			if fresh == nil || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.isAccountRequestCompatible(ctx, fresh, req) {
				continue
			}
			if !s.consumeOpenAISelectionDBRecheck(budget) {
				details := s.selectionFailureDetails(diagnosticCtx, req, "selection_order_exhausted")
				return nil, candidateCount, topK, loadSkew, noAvailableOpenAISelectionError(req.RequestedModel, compactBlocked, details)
			}
			fresh = s.service.recheckSelectedOpenAIAccountFromDB(ctx, fresh, req.GroupID, req.Platform, req.RequestedModel, false, req.RequiredCapability)
			if fresh == nil || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.isAccountRequestCompatible(ctx, fresh, req) {
				continue
			}
			if req.RequireCompact && openAICompactSupportTier(fresh) == 0 {
				compactBlocked = true
				continue
			}
			return &AccountSelectionResult{
				Account: fresh,
				WaitPlan: &AccountWaitPlan{
					AccountID:      fresh.ID,
					MaxConcurrency: fresh.Concurrency,
					Timeout:        cfg.FallbackWaitTimeout,
					MaxWaiting:     cfg.FallbackMaxWaiting,
				},
			}, candidateCount, topK, loadSkew, nil
		}
	}

	details := s.selectionFailureDetails(diagnosticCtx, req, "selection_order_exhausted")
	return nil, candidateCount, topK, loadSkew, noAvailableOpenAISelectionError(req.RequestedModel, compactBlocked, details)
}

func (s *defaultOpenAIAccountScheduler) isAccountTransportCompatible(account *Account, requiredTransport OpenAIUpstreamTransport) bool {
	if requiredTransport == OpenAIUpstreamTransportAny || requiredTransport == OpenAIUpstreamTransportHTTPSSE {
		return true
	}
	if s == nil || s.service == nil {
		return false
	}
	return s.service.isOpenAIAccountTransportCompatible(account, requiredTransport)
}

func (s *defaultOpenAIAccountScheduler) lookupShadowParentAccount(ctx context.Context, id int64) *Account {
	if s == nil || s.service == nil {
		return nil
	}
	if s.service.schedulerSnapshot != nil {
		if account, err := s.service.schedulerSnapshot.GetAccount(ctx, id); err == nil && account != nil {
			return account
		}
	}
	if s.service.accountRepo == nil {
		return nil
	}
	account, _ := s.service.accountRepo.GetByID(ctx, id)
	return account
}

func (s *defaultOpenAIAccountScheduler) isAccountRequestCompatible(ctx context.Context, account *Account, req OpenAIAccountScheduleRequest) bool {
	compatible, _ := s.isAccountRequestCompatibleReason(ctx, account, req)
	return compatible
}

// isAccountRequestCompatibleReason reports whether the account can serve the
// request, and when it cannot, names the veto point. The reason feeds
// openAISelectionFilterStats so that "no available accounts" errors state why
// each candidate was dropped instead of failing silently (#4599).
func (s *defaultOpenAIAccountScheduler) isAccountRequestCompatibleReason(ctx context.Context, account *Account, req OpenAIAccountScheduleRequest) (bool, string) {
	if account == nil {
		return false, "account_nil"
	}
	if s != nil && s.service != nil && !s.service.codexContinuationCandidateMatches(ctx, account) {
		return false, "codex_continuation_principal_mismatch"
	}
	if s != nil && s.service != nil && s.service.isOpenAIAccountRequestRuntimeBlocked(account, req.RequestedModel) {
		return false, "runtime_blocked"
	}
	if s != nil && s.service != nil && s.service.isOpenAIProxyStreamQuarantined(ctx, account) {
		return false, "proxy_stream_quarantined"
	}
	// Quota auto-pause must be evaluated during the initial filter too. Without it the
	// TopK candidate pool can be filled with paused accounts and the later fresh/DB
	// rechecks won't reach healthy accounts that fell outside TopK — manifesting as
	// "no available accounts" even though healthy ones exist.
	if paused, decision := shouldAutoPauseOpenAIAccountByQuota(ctx, account); paused {
		reason := "quota_auto_pause"
		if decision.window != "" {
			reason += "_" + decision.window
		}
		return false, reason
	}
	// 母账号健康联动：影子账号的凭据来自母账号，母账号不可调度时影子也不应被选中。
	// Parent-health gate: shadow borrows the parent's credentials; an unschedulable
	// parent must block the shadow across all scheduler paths.
	if !parentHealthyForShadow(account, func(id int64) *Account {
		return s.lookupShadowParentAccount(ctx, id)
	}) {
		return false, "shadow_parent_unhealthy"
	}
	if req.RequestedModel != "" && !supportsOpenAIRequestModel(account, req.RequestedModel, openAIRequestModelForSupport(ctx, req.RequestedModel), req.RequireCompact) {
		return false, "model_not_supported"
	}
	if req.GroupID != nil && s != nil && s.service != nil &&
		s.service.needsUpstreamChannelRestrictionCheck(ctx, req.GroupID) &&
		s.service.isUpstreamModelRestrictedByChannel(ctx, *req.GroupID, account, req.RequestedModel, req.RequireCompact) {
		return false, "channel_upstream_restricted"
	}
	if !accountSupportsOpenAICapabilities(account, req.RequiredCapability, req.RequiredImageCapability) {
		return false, "capability_mismatch"
	}
	if vetoed, reason := openAIImageSizeVetoReason(ctx, account); vetoed {
		return false, reason
	}
	if vetoed, reason := profitControlVetoReason(ctx, account); vetoed {
		return false, reason
	}
	return true, ""
}

func (s *defaultOpenAIAccountScheduler) ReportResult(accountID int64, success bool, firstTokenMs *int) {
	if s == nil || s.stats == nil {
		return
	}
	s.stats.report(accountID, success, firstTokenMs)
}

func (s *defaultOpenAIAccountScheduler) ReportSwitch() {
	if s == nil {
		return
	}
	s.metrics.recordSwitch()
}

func (s *defaultOpenAIAccountScheduler) SnapshotMetrics() OpenAIAccountSchedulerMetricsSnapshot {
	if s == nil {
		return OpenAIAccountSchedulerMetricsSnapshot{}
	}

	selectTotal := s.metrics.selectTotal.Load()
	prevHit := s.metrics.stickyPreviousHitTotal.Load()
	sessionHit := s.metrics.stickySessionHitTotal.Load()
	stickyHit := s.metrics.stickyHitTotal.Load()
	switchTotal := s.metrics.accountSwitchTotal.Load()
	latencyTotal := s.metrics.latencyMsTotal.Load()
	loadSkewTotal := s.metrics.loadSkewMilliTotal.Load()

	snapshot := OpenAIAccountSchedulerMetricsSnapshot{
		SelectTotal:              selectTotal,
		StickyPreviousHitTotal:   prevHit,
		StickySessionHitTotal:    sessionHit,
		LoadBalanceSelectTotal:   s.metrics.loadBalanceSelectTotal.Load(),
		AccountSwitchTotal:       switchTotal,
		SchedulerLatencyMsTotal:  latencyTotal,
		RuntimeStatsAccountCount: s.stats.size(),
	}
	if selectTotal > 0 {
		snapshot.SchedulerLatencyMsAvg = float64(latencyTotal) / float64(selectTotal)
		snapshot.StickyHitRatio = float64(stickyHit) / float64(selectTotal)
		snapshot.AccountSwitchRate = float64(switchTotal) / float64(selectTotal)
		snapshot.LoadSkewAvg = float64(loadSkewTotal) / 1000 / float64(selectTotal)
	}
	return snapshot
}

func (s *OpenAIGatewayService) openAIAdvancedSchedulerSettingRepo() SettingRepository {
	if s == nil || s.rateLimitService == nil || s.rateLimitService.settingService == nil {
		return nil
	}
	return s.rateLimitService.settingService.settingRepo
}

func (s *OpenAIGatewayService) openAIAdvancedSchedulerRuntimeSettings(ctx context.Context) openAIAdvancedSchedulerRuntimeSettings {
	if cached, ok := openAIAdvancedSchedulerSettingCache.Load().(*cachedOpenAIAdvancedSchedulerSetting); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return openAIAdvancedSchedulerRuntimeSettings{
				lowUpstreamRatePriorityEnabled: cached.lowUpstreamRatePriorityEnabled,
				oauthSchedulingRateMultiplier:  cached.oauthSchedulingRateMultiplier,
				contentSessionBurstBalance:     cached.contentSessionBurstBalance,
				sessionIDRateLimitEnabled:      cached.sessionIDRateLimitEnabled,
				sessionIDRateLimitPerMinute:    cached.sessionIDRateLimitPerMinute,
				oauthGatewayRateLimitEnabled:   cached.oauthGatewayRateLimitEnabled,
				oauthGatewayRateLimitRPM:       cached.oauthGatewayRateLimitRPM,
				oauthGatewayRateLimitBurst:     cached.oauthGatewayRateLimitBurst,
				requestIntegrityObserveEnabled: cached.requestIntegrityObserveEnabled,
				enabled:                        cached.enabled,
				stickyWeightedEnabled:          cached.stickyWeightedEnabled,
				subscriptionPriorityEnabled:    cached.subscriptionPriorityEnabled,
				lbTopKOverride:                 cached.lbTopKOverride,
				weightOverrides:                cloneOpenAIAdvancedSchedulerWeightOverrides(cached.weightOverrides),
			}
		}
	}

	result, _, _ := openAIAdvancedSchedulerSettingSF.Do(openAIAdvancedSchedulerSettingKey, func() (any, error) {
		if cached, ok := openAIAdvancedSchedulerSettingCache.Load().(*cachedOpenAIAdvancedSchedulerSetting); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return openAIAdvancedSchedulerRuntimeSettings{
					lowUpstreamRatePriorityEnabled: cached.lowUpstreamRatePriorityEnabled,
					oauthSchedulingRateMultiplier:  cached.oauthSchedulingRateMultiplier,
					contentSessionBurstBalance:     cached.contentSessionBurstBalance,
					sessionIDRateLimitEnabled:      cached.sessionIDRateLimitEnabled,
					sessionIDRateLimitPerMinute:    cached.sessionIDRateLimitPerMinute,
					oauthGatewayRateLimitEnabled:   cached.oauthGatewayRateLimitEnabled,
					oauthGatewayRateLimitRPM:       cached.oauthGatewayRateLimitRPM,
					oauthGatewayRateLimitBurst:     cached.oauthGatewayRateLimitBurst,
					requestIntegrityObserveEnabled: cached.requestIntegrityObserveEnabled,
					enabled:                        cached.enabled,
					stickyWeightedEnabled:          cached.stickyWeightedEnabled,
					subscriptionPriorityEnabled:    cached.subscriptionPriorityEnabled,
					lbTopKOverride:                 cached.lbTopKOverride,
					weightOverrides:                cloneOpenAIAdvancedSchedulerWeightOverrides(cached.weightOverrides),
				}, nil
			}
		}

		lowUpstreamRatePriorityEnabled := false
		oauthSchedulingRateMultiplier := defaultOpenAIOAuthSchedulingRateMultiplier
		contentSessionBurstBalance := false
		sessionIDRateLimitEnabled := false
		sessionIDRateLimitPerMinute := 0
		oauthGatewayRateLimitEnabled := false
		oauthGatewayRateLimitRPM := 60
		oauthGatewayRateLimitBurst := 5
		requestIntegrityObserveEnabled := false
		if stale, ok := openAIAdvancedSchedulerSettingCache.Load().(*cachedOpenAIAdvancedSchedulerSetting); ok && stale != nil {
			oauthGatewayRateLimitEnabled = stale.oauthGatewayRateLimitEnabled
			oauthGatewayRateLimitRPM = stale.oauthGatewayRateLimitRPM
			oauthGatewayRateLimitBurst = stale.oauthGatewayRateLimitBurst
			requestIntegrityObserveEnabled = stale.requestIntegrityObserveEnabled
		}
		enabled := false
		stickyWeightedEnabled := false
		subscriptionPriorityEnabled := false
		lbTopKOverride := 0
		weightOverrides := map[string]float64{}
		if repo := s.openAIAdvancedSchedulerSettingRepo(); repo != nil {
			dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAdvancedSchedulerSettingDBTimeout)
			defer cancel()

			if values, err := repo.GetMultiple(dbCtx, openAIAdvancedSchedulerRuntimeSettingKeys()); err == nil {
				lowUpstreamRatePriorityEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAILowUpstreamRatePriorityEnabled]), "true")
				oauthSchedulingRateMultiplier = parseOpenAIOAuthSchedulingRateMultiplier(values[SettingKeyOpenAIOAuthSchedulingRateMultiplier])
				contentSessionBurstBalance = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAIContentSessionBurstBalanceEnabled]), "true")
				sessionIDRateLimitEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAISessionIDRateLimitEnabled]), "true")
				sessionIDRateLimitPerMinute = parseOpenAISessionIDRateLimitPerMinute(values[SettingKeyOpenAISessionIDRateLimitPerMinute])
				oauthGatewayRateLimitEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAIOAuthGatewayRateLimitEnabled]), "true")
				oauthGatewayRateLimitRPM = parseOpenAIOAuthGatewayRateLimitValue(values[SettingKeyOpenAIOAuthGatewayRateLimitRPM], 60)
				oauthGatewayRateLimitBurst = parseOpenAIOAuthGatewayRateLimitValue(values[SettingKeyOpenAIOAuthGatewayRateLimitBurst], 5)
				requestIntegrityObserveEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAIRequestIntegrityObserveEnabled]), "true")
				enabled = strings.EqualFold(strings.TrimSpace(values[openAIAdvancedSchedulerSettingKey]), "true")
				stickyWeightedEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled]), "true")
				subscriptionPriorityEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled]), "true")
				lbTopKOverride = parsePositiveIntOverride(values[SettingKeyOpenAIAdvancedSchedulerLBTopK])
				weightOverrides = parseOpenAIAdvancedSchedulerWeightOverrides(values)
			} else {
				// 批量读取失败时逐键降级，覆盖全部键（含 TopK/权重），避免只加载布尔开关
				// 而静默丢弃管理员配置的覆盖值；降级状态会被缓存一个 TTL，必须留痕。
				slog.Warn("openai_advanced_scheduler_settings_batch_load_failed", "error", err)
				fallbackValues := make(map[string]string)
				for _, key := range openAIAdvancedSchedulerRuntimeSettingKeys() {
					if value, valueErr := repo.GetValue(dbCtx, key); valueErr == nil {
						fallbackValues[key] = value
					}
				}
				lowUpstreamRatePriorityEnabled = strings.EqualFold(strings.TrimSpace(fallbackValues[SettingKeyOpenAILowUpstreamRatePriorityEnabled]), "true")
				oauthSchedulingRateMultiplier = parseOpenAIOAuthSchedulingRateMultiplier(fallbackValues[SettingKeyOpenAIOAuthSchedulingRateMultiplier])
				contentSessionBurstBalance = strings.EqualFold(strings.TrimSpace(fallbackValues[SettingKeyOpenAIContentSessionBurstBalanceEnabled]), "true")
				sessionIDRateLimitEnabled = strings.EqualFold(strings.TrimSpace(fallbackValues[SettingKeyOpenAISessionIDRateLimitEnabled]), "true")
				sessionIDRateLimitPerMinute = parseOpenAISessionIDRateLimitPerMinute(fallbackValues[SettingKeyOpenAISessionIDRateLimitPerMinute])
				if raw, ok := fallbackValues[SettingKeyOpenAIOAuthGatewayRateLimitEnabled]; ok {
					oauthGatewayRateLimitEnabled = strings.EqualFold(strings.TrimSpace(raw), "true")
				}
				if raw, ok := fallbackValues[SettingKeyOpenAIOAuthGatewayRateLimitRPM]; ok {
					oauthGatewayRateLimitRPM = parseOpenAIOAuthGatewayRateLimitValue(raw, 60)
				}
				if raw, ok := fallbackValues[SettingKeyOpenAIOAuthGatewayRateLimitBurst]; ok {
					oauthGatewayRateLimitBurst = parseOpenAIOAuthGatewayRateLimitValue(raw, 5)
				}
				if raw, ok := fallbackValues[SettingKeyOpenAIRequestIntegrityObserveEnabled]; ok {
					requestIntegrityObserveEnabled = strings.EqualFold(strings.TrimSpace(raw), "true")
				}
				enabled = strings.EqualFold(strings.TrimSpace(fallbackValues[openAIAdvancedSchedulerSettingKey]), "true")
				stickyWeightedEnabled = strings.EqualFold(strings.TrimSpace(fallbackValues[SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled]), "true")
				subscriptionPriorityEnabled = strings.EqualFold(strings.TrimSpace(fallbackValues[SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled]), "true")
				lbTopKOverride = parsePositiveIntOverride(fallbackValues[SettingKeyOpenAIAdvancedSchedulerLBTopK])
				weightOverrides = parseOpenAIAdvancedSchedulerWeightOverrides(fallbackValues)
			}
		}

		openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{
			lowUpstreamRatePriorityEnabled: lowUpstreamRatePriorityEnabled,
			oauthSchedulingRateMultiplier:  oauthSchedulingRateMultiplier,
			contentSessionBurstBalance:     contentSessionBurstBalance,
			sessionIDRateLimitEnabled:      sessionIDRateLimitEnabled,
			sessionIDRateLimitPerMinute:    sessionIDRateLimitPerMinute,
			oauthGatewayRateLimitEnabled:   oauthGatewayRateLimitEnabled,
			oauthGatewayRateLimitRPM:       oauthGatewayRateLimitRPM,
			oauthGatewayRateLimitBurst:     oauthGatewayRateLimitBurst,
			requestIntegrityObserveEnabled: requestIntegrityObserveEnabled,
			enabled:                        enabled,
			stickyWeightedEnabled:          stickyWeightedEnabled,
			subscriptionPriorityEnabled:    subscriptionPriorityEnabled,
			lbTopKOverride:                 lbTopKOverride,
			weightOverrides:                cloneOpenAIAdvancedSchedulerWeightOverrides(weightOverrides),
			expiresAt:                      time.Now().Add(openAIAdvancedSchedulerSettingCacheTTL).UnixNano(),
		})
		return openAIAdvancedSchedulerRuntimeSettings{
			lowUpstreamRatePriorityEnabled: lowUpstreamRatePriorityEnabled,
			oauthSchedulingRateMultiplier:  oauthSchedulingRateMultiplier,
			contentSessionBurstBalance:     contentSessionBurstBalance,
			sessionIDRateLimitEnabled:      sessionIDRateLimitEnabled,
			sessionIDRateLimitPerMinute:    sessionIDRateLimitPerMinute,
			oauthGatewayRateLimitEnabled:   oauthGatewayRateLimitEnabled,
			oauthGatewayRateLimitRPM:       oauthGatewayRateLimitRPM,
			oauthGatewayRateLimitBurst:     oauthGatewayRateLimitBurst,
			requestIntegrityObserveEnabled: requestIntegrityObserveEnabled,
			enabled:                        enabled,
			stickyWeightedEnabled:          stickyWeightedEnabled,
			subscriptionPriorityEnabled:    subscriptionPriorityEnabled,
			lbTopKOverride:                 lbTopKOverride,
			weightOverrides:                weightOverrides,
		}, nil
	})

	settings, _ := result.(openAIAdvancedSchedulerRuntimeSettings)
	return settings
}

func (s *OpenAIGatewayService) isOpenAIAdvancedSchedulerEnabled(ctx context.Context) bool {
	return s.openAIAdvancedSchedulerRuntimeSettings(ctx).enabled
}

func (s *OpenAIGatewayService) isOpenAILowUpstreamRatePriorityEnabled(ctx context.Context) bool {
	settings := s.openAIAdvancedSchedulerRuntimeSettings(ctx)
	return !settings.enabled && settings.lowUpstreamRatePriorityEnabled
}

func (s *OpenAIGatewayService) openAIOAuthSchedulingRateMultiplier(ctx context.Context) float64 {
	return s.openAIAdvancedSchedulerRuntimeSettings(ctx).oauthSchedulingRateMultiplier
}

func (s *OpenAIGatewayService) isOpenAIContentSessionBurstBalanceEnabled(ctx context.Context) bool {
	if cached, ok := openAIAdvancedSchedulerSettingCache.Load().(*cachedOpenAIAdvancedSchedulerSetting); ok && cached != nil &&
		time.Now().UnixNano() < cached.expiresAt {
		return cached.contentSessionBurstBalance
	}
	return s.openAIAdvancedSchedulerRuntimeSettings(ctx).contentSessionBurstBalance
}

func (s *OpenAIGatewayService) isOpenAIAdvancedSchedulerStickyWeightedEnabled(ctx context.Context) bool {
	settings := s.openAIAdvancedSchedulerRuntimeSettings(ctx)
	return settings.enabled && settings.stickyWeightedEnabled
}

func (s *OpenAIGatewayService) isOpenAIAdvancedSchedulerSubscriptionPriorityEnabled(ctx context.Context) bool {
	settings := s.openAIAdvancedSchedulerRuntimeSettings(ctx)
	return settings.enabled && settings.subscriptionPriorityEnabled
}

func openAIAdvancedSchedulerRuntimeSettingKeys() []string {
	keys := []string{
		SettingKeyOpenAILowUpstreamRatePriorityEnabled,
		SettingKeyOpenAIOAuthSchedulingRateMultiplier,
		SettingKeyOpenAIContentSessionBurstBalanceEnabled,
		SettingKeyOpenAISessionIDRateLimitEnabled,
		SettingKeyOpenAISessionIDRateLimitPerMinute,
		SettingKeyOpenAIOAuthGatewayRateLimitEnabled,
		SettingKeyOpenAIOAuthGatewayRateLimitRPM,
		SettingKeyOpenAIOAuthGatewayRateLimitBurst,
		SettingKeyOpenAIRequestIntegrityObserveEnabled,
		openAIAdvancedSchedulerSettingKey,
		SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled,
		SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled,
		SettingKeyOpenAIAdvancedSchedulerLBTopK,
	}
	for _, spec := range openAIAdvancedSchedulerWeightOverrideSpecs() {
		keys = append(keys, spec.key)
	}
	return keys
}

type openAIAdvancedSchedulerWeightOverrideSpec struct {
	key  string
	name string
}

func openAIAdvancedSchedulerWeightOverrideSpecs() []openAIAdvancedSchedulerWeightOverrideSpec {
	return []openAIAdvancedSchedulerWeightOverrideSpec{
		{key: SettingKeyOpenAIAdvancedSchedulerWeightPriority, name: "priority"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightLoad, name: "load"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightQueue, name: "queue"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightErrorRate, name: "error_rate"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightTTFT, name: "ttft"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightReset, name: "reset"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom, name: "quota_headroom"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightUpstreamCost, name: "upstream_cost"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse, name: "previous_response"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky, name: "session_sticky"},
	}
}

func parsePositiveIntOverride(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func parseOpenAIAdvancedSchedulerWeightOverrides(values map[string]string) map[string]float64 {
	overrides := map[string]float64{}
	for _, spec := range openAIAdvancedSchedulerWeightOverrideSpecs() {
		raw := strings.TrimSpace(values[spec.key])
		if raw == "" {
			continue
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		overrides[spec.name] = value
	}
	return overrides
}

func cloneOpenAIAdvancedSchedulerWeightOverrides(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (s *OpenAIGatewayService) getOpenAIAccountScheduler(ctx context.Context) OpenAIAccountScheduler {
	if s == nil {
		return nil
	}
	if !s.isOpenAIAdvancedSchedulerEnabled(ctx) {
		return nil
	}
	s.openaiSchedulerOnce.Do(func() {
		if s.openaiAccountStats == nil {
			s.openaiAccountStats = newOpenAIAccountRuntimeStats()
		}
		if s.openaiScheduler == nil {
			s.openaiScheduler = newDefaultOpenAIAccountScheduler(s, s.openaiAccountStats)
		}
	})
	return s.openaiScheduler
}

func resetOpenAIAdvancedSchedulerSettingCacheForTest() {
	openAIAdvancedSchedulerSettingCache = atomic.Value{}
	openAIAdvancedSchedulerSettingSF = singleflight.Group{}
}

func (s *OpenAIGatewayService) SelectAccountWithScheduler(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requireCompact bool,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	selection, decision, err := s.selectAccountWithScheduler(ctx, groupID, previousResponseID, sessionHash, requestedModel, excludedIDs, requiredTransport, "", "", requireCompact, PlatformOpenAI, false, true)
	if selection != nil && selection.Account != nil {
		s.recordOpenAIContentSessionCandidate(ctx, groupID, sessionHash, selection.Account.ID)
		s.recordOpenAISessionIDGrowth(ctx, sessionHash, selection.Account)
	}
	return selection, decision, err
}

// SelectAccountWithSchedulerForCapability 按能力要求调度账号。
// previousResponseCanMove 表示首包 input 可自行重建工具续链，previous_response_id 允许跨账号迁移
// （粘性加权模式下改为加权偏好而非硬粘连）。
func (s *OpenAIGatewayService) SelectAccountWithSchedulerForCapability(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requiredCapability OpenAIEndpointCapability,
	requireCompact bool,
	previousResponseCanMove bool,
	useUpstreamTokenCost bool,
	platformOverride ...string,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	platform := PlatformOpenAI
	if len(platformOverride) > 0 {
		platform = platformOverride[0]
	}
	selection, decision, err := s.selectAccountWithScheduler(ctx, groupID, previousResponseID, sessionHash, requestedModel, excludedIDs, requiredTransport, requiredCapability, "", requireCompact, platform, previousResponseCanMove, useUpstreamTokenCost)
	if selection != nil && selection.Account != nil {
		s.recordOpenAIContentSessionCandidate(ctx, groupID, sessionHash, selection.Account.ID)
		s.recordOpenAISessionIDGrowth(ctx, sessionHash, selection.Account)
	}
	return selection, decision, err
}

func (s *OpenAIGatewayService) SelectAccountWithSchedulerForImages(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredCapability OpenAIImagesCapability,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	selection, decision, err := s.selectAccountWithScheduler(ctx, groupID, "", sessionHash, requestedModel, excludedIDs, OpenAIUpstreamTransportHTTPSSE, "", requiredCapability, false, PlatformOpenAI, false, false)
	if err == nil && selection != nil && selection.Account != nil {
		s.recordOpenAIContentSessionCandidate(ctx, groupID, sessionHash, selection.Account.ID)
		s.recordOpenAISessionIDGrowth(ctx, sessionHash, selection.Account)
		return selection, decision, nil
	}
	// Forced API capability must never fall back to OAuth/basic accounts.
	// The group-level Responses bridge relies on the account's configured
	// /v1/images endpoint and records image-only billing against that account.
	if requiredCapability == OpenAIImagesCapabilityNative {
		selection, decision, err = s.selectAccountWithScheduler(ctx, groupID, "", sessionHash, requestedModel, excludedIDs, OpenAIUpstreamTransportHTTPSSE, "", OpenAIImagesCapabilityBasic, false, PlatformOpenAI, false, false)
	}
	if selection != nil && selection.Account != nil {
		s.recordOpenAIContentSessionCandidate(ctx, groupID, sessionHash, selection.Account.ID)
		s.recordOpenAISessionIDGrowth(ctx, sessionHash, selection.Account)
	}
	return selection, decision, err
}

func (s *OpenAIGatewayService) recordOpenAISessionIDGrowth(ctx context.Context, sessionHash string, account *Account) {
	if s == nil || account == nil || account.Platform != PlatformOpenAI || strings.TrimSpace(sessionHash) == "" {
		return
	}
	if openAISessionHashMetadataFromContext(ctx).contentDerived {
		return
	}
	metrics := s.sessionIDRateMetrics
	if metrics == nil {
		metrics = DefaultOpenAISessionIDRateMetrics()
	}
	metrics.Record(account.ID, sessionHash, time.Now())
}

func (s *OpenAIGatewayService) selectAccountWithScheduler(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requiredCapability OpenAIEndpointCapability,
	requiredImageCapability OpenAIImagesCapability,
	requireCompact bool,
	platform string,
	previousResponseCanMove bool,
	useUpstreamTokenCost bool,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	candidates, routed := apiKeyGroupRoutingCandidates(ctx, groupID)
	if !routed {
		return s.selectAccountWithSchedulerInGroup(ctx, groupID, previousResponseID, sessionHash, requestedModel, excludedIDs, requiredTransport, requiredCapability, requiredImageCapability, requireCompact, platform, previousResponseCanMove, useUpstreamTokenCost)
	}
	var (
		lastDecision OpenAIAccountScheduleDecision
		lastErr      = ErrNoAvailableAccounts
	)
	for i := range candidates {
		candidateID := candidates[i].GroupID
		selection, decision, err := s.selectAccountWithSchedulerInGroup(
			withAPIKeyGroupRoutingAttempt(ctx, candidates[i]),
			&candidateID,
			previousResponseID,
			sessionHash,
			requestedModel,
			excludedIDs,
			requiredTransport,
			requiredCapability,
			requiredImageCapability,
			requireCompact,
			platform,
			previousResponseCanMove,
			useUpstreamTokenCost,
		)
		lastDecision = decision
		if err == nil && selection != nil && selection.Account != nil {
			markAPIKeyGroupRoutingSelected(ctx, candidateID)
			return selection, decision, nil
		}
		if err != nil {
			lastErr = err
			if !errors.Is(err, ErrNoAvailableAccounts) && !errors.Is(err, ErrNoAvailableCompactAccounts) {
				return selection, decision, err
			}
		}
	}
	return nil, lastDecision, lastErr
}

func (s *OpenAIGatewayService) selectAccountWithSchedulerInGroup(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requiredCapability OpenAIEndpointCapability,
	requiredImageCapability OpenAIImagesCapability,
	requireCompact bool,
	platform string,
	previousResponseCanMove bool,
	useUpstreamTokenCost bool,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	enabled, limit := s.openAISessionIDRateLimitSettings(ctx)
	if !enabled || limit <= 0 || strings.TrimSpace(sessionHash) == "" || openAISessionHashMetadataFromContext(ctx).contentDerived || normalizeOpenAICompatiblePlatform(platform) != PlatformOpenAI || (strings.TrimSpace(previousResponseID) != "" && !previousResponseCanMove) || codexContinuationSchedulingAffinityActive(ctx) {
		return s.selectAccountWithSchedulerInGroupRaw(ctx, groupID, previousResponseID, sessionHash, requestedModel, excludedIDs, requiredTransport, requiredCapability, requiredImageCapability, requireCompact, platform, previousResponseCanMove, useUpstreamTokenCost)
	}
	effectiveExcludedIDs := cloneExcludedAccountIDs(excludedIDs)
	var lastDecision OpenAIAccountScheduleDecision
	for attempt := 0; attempt < openAIAccountSelectionProbeLimit; attempt++ {
		selection, decision, err := s.selectAccountWithSchedulerInGroupRaw(ctx, groupID, previousResponseID, sessionHash, requestedModel, effectiveExcludedIDs, requiredTransport, requiredCapability, requiredImageCapability, requireCompact, platform, previousResponseCanMove, useUpstreamTokenCost)
		lastDecision = decision
		if selection == nil || selection.Account == nil || err != nil {
			return selection, decision, err
		}
		if !openAISessionIDRateLimitApplies(selection.Account) {
			return selection, decision, nil
		}
		admission, admissionErr := s.admitOpenAISessionID(ctx, selection.Account.ID, sessionHash, limit)
		if admissionErr != nil {
			// Preserve the existing fail-open behavior when Redis is unavailable.
			s.recordOpenAISessionIDGrowth(ctx, sessionHash, selection.Account)
			return selection, decision, nil
		}
		if admission.Allowed {
			return selection, decision, nil
		}
		if strings.TrimSpace(previousResponseID) == "" && s.cache != nil {
			_ = s.deleteStickySessionAccountID(ctx, groupID, sessionHash)
		}
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		if effectiveExcludedIDs == nil {
			effectiveExcludedIDs = make(map[int64]struct{})
		}
		effectiveExcludedIDs[selection.Account.ID] = struct{}{}
	}
	return nil, lastDecision, ErrNoAvailableAccounts
}

func (s *OpenAIGatewayService) selectAccountWithSchedulerInGroupRaw(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requiredCapability OpenAIEndpointCapability,
	requiredImageCapability OpenAIImagesCapability,
	requireCompact bool,
	platform string,
	previousResponseCanMove bool,
	useUpstreamTokenCost bool,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	ctx = s.withOpenAIRoutingModelContext(ctx, groupID, requestedModel, requireCompact)
	if requiredImageCapability == "" {
		ctx = s.withOpenAIProfitControlGate(ctx, groupID)
	}
	selection, decision, err := s.selectAccountWithSchedulerBySize(ctx, groupID, previousResponseID, sessionHash, requestedModel, excludedIDs, requiredTransport, requiredCapability, requiredImageCapability, requireCompact, platform, previousResponseCanMove, useUpstreamTokenCost)
	selection = attachSelectionProfitGate(ctx, selection)
	if err == nil || openAIProxyStreamQuarantineBypassed(ctx) {
		return selection, decision, err
	}
	if !errors.Is(err, ErrNoAvailableAccounts) && !errors.Is(err, ErrNoAvailableCompactAccounts) {
		return selection, decision, err
	}
	if normalizeOpenAICompatiblePlatform(platform) != PlatformOpenAI {
		return selection, decision, err
	}
	circuit := s.getOpenAIProxyStreamCircuit()
	blocked := circuit.activeBlockCount(time.Now())
	if blocked == 0 {
		return selection, decision, err
	}
	s.logOpenAIProxyStreamQuarantineFailOpen(requestedModel, blocked)
	selection, decision, err = s.selectAccountWithSchedulerBySize(withOpenAIProxyStreamQuarantineBypass(ctx), groupID, previousResponseID, sessionHash, requestedModel, excludedIDs, requiredTransport, requiredCapability, requiredImageCapability, requireCompact, platform, previousResponseCanMove, useUpstreamTokenCost)
	return attachSelectionProfitGate(ctx, selection), decision, err
}

func (s *OpenAIGatewayService) selectAccountWithSchedulerOnce(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requiredCapability OpenAIEndpointCapability,
	requiredImageCapability OpenAIImagesCapability,
	requireCompact bool,
	platform string,
	previousResponseCanMove bool,
	useUpstreamTokenCost bool,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	ctx = s.withOpenAIQuotaAutoPauseContext(ctx)
	platform = normalizeOpenAICompatiblePlatform(platform)
	contentSessionConcurrent := openAIContentSessionRequestConcurrent(ctx)
	contentSessionOverflow := openAIContentSessionRequestOverflow(ctx)
	contentSessionBurst := contentSessionConcurrent || contentSessionOverflow
	preserveStickyBinding := contentSessionBurst || openAIStickyFailoverPreservationEnabled(ctx) || openAIImageSizeSpilloverPass(ctx)
	decision := OpenAIAccountScheduleDecision{}
	scheduler := s.getOpenAIAccountScheduler(ctx)
	if scheduler == nil {
		decision.Layer = openAIAccountScheduleLayerLoadBalance
		if contentSessionOverflow {
			selection, candidateErr := s.selectOpenAIContentSessionBurstCandidates(
				ctx, groupID, platform, sessionHash, requestedModel, excludedIDs, requireCompact,
				requiredTransport, requiredCapability, requiredImageCapability,
			)
			if candidateErr != nil {
				return nil, decision, candidateErr
			}
			if selection != nil && selection.Account != nil {
				decision.Layer = openAIAccountScheduleLayerContentBurst
				decision.SelectedAccountID = selection.Account.ID
				decision.SelectedAccountType = selection.Account.Type
				return selection, decision, nil
			}
		}
		if requiredTransport == OpenAIUpstreamTransportAny || requiredTransport == OpenAIUpstreamTransportHTTPSSE {
			effectiveExcludedIDs := cloneExcludedAccountIDs(excludedIDs)
			for {
				selection, err := s.selectAccountWithLoadAwareness(ctx, groupID, platform, sessionHash, requestedModel, effectiveExcludedIDs, requireCompact, requiredCapability, useUpstreamTokenCost, contentSessionBurst)
				if err != nil {
					return nil, decision, err
				}
				if selection == nil || selection.Account == nil {
					return selection, decision, nil
				}
				if accountSupportsOpenAICapabilities(selection.Account, requiredCapability, requiredImageCapability) {
					return selection, decision, nil
				}
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
				if effectiveExcludedIDs == nil {
					effectiveExcludedIDs = make(map[int64]struct{})
				}
				if _, exists := effectiveExcludedIDs[selection.Account.ID]; exists {
					return nil, decision, ErrNoAvailableAccounts
				}
				effectiveExcludedIDs[selection.Account.ID] = struct{}{}
			}
		}

		effectiveExcludedIDs := cloneExcludedAccountIDs(excludedIDs)
		for {
			selection, err := s.selectAccountWithLoadAwareness(ctx, groupID, platform, sessionHash, requestedModel, effectiveExcludedIDs, requireCompact, requiredCapability, useUpstreamTokenCost, contentSessionBurst)
			if err != nil {
				return nil, decision, err
			}
			if selection == nil || selection.Account == nil {
				return selection, decision, nil
			}
			if s.isOpenAIAccountTransportCompatible(selection.Account, requiredTransport) &&
				accountSupportsOpenAICapabilities(selection.Account, requiredCapability, requiredImageCapability) {
				return selection, decision, nil
			}
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			if effectiveExcludedIDs == nil {
				effectiveExcludedIDs = make(map[int64]struct{})
			}
			if _, exists := effectiveExcludedIDs[selection.Account.ID]; exists {
				return nil, decision, ErrNoAvailableAccounts
			}
			effectiveExcludedIDs[selection.Account.ID] = struct{}{}
		}
	}

	if s.checkChannelPricingRestriction(ctx, groupID, requestedModel) {
		slog.Warn("channel pricing restriction blocked request",
			"group_id", derefGroupID(groupID),
			"model", requestedModel)
		return nil, decision, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, requestedModel)
	}

	var stickyAccountID int64
	if sessionHash != "" && s.cache != nil {
		if accountID, err := s.getStickySessionAccountID(ctx, groupID, sessionHash); err == nil && accountID > 0 {
			stickyAccountID = accountID
		}
	}
	// Overflow requests must return to the sticky account after admission. Letting
	// weighted sticky run here would still scan the full pool for every overflow.
	stickyWeighted := !contentSessionOverflow && s.isOpenAIAdvancedSchedulerStickyWeightedEnabled(ctx)
	subscriptionPriority := s.isOpenAIAdvancedSchedulerSubscriptionPriorityEnabled(ctx)
	stickyPreviousAccountID := int64(0)
	if stickyWeighted && previousResponseCanMove && strings.TrimSpace(previousResponseID) != "" && platform == PlatformOpenAI {
		stickyPreviousAccountID = s.ResolveAccountIDByPreviousResponseIDForScheduler(ctx, groupID, previousResponseID, requestedModel, excludedIDs, requiredCapability, requireCompact)
	}

	return scheduler.Select(ctx, OpenAIAccountScheduleRequest{
		GroupID:                  groupID,
		Platform:                 platform,
		SessionHash:              sessionHash,
		StickyAccountID:          stickyAccountID,
		StickyPreviousAccountID:  stickyPreviousAccountID,
		StickyWeighted:           stickyWeighted,
		SubscriptionPriority:     subscriptionPriority,
		PreserveStickyBinding:    preserveStickyBinding,
		ContentSessionConcurrent: contentSessionBurst,
		PreviousResponseID:       previousResponseID,
		PreviousResponseCanMove:  previousResponseCanMove,
		UseUpstreamTokenCost:     useUpstreamTokenCost,
		RequestedModel:           requestedModel,
		RequiredTransport:        requiredTransport,
		RequiredCapability:       requiredCapability,
		RequiredImageCapability:  requiredImageCapability,
		RequireCompact:           requireCompact,
		IsStreaming:              isOpenAIStreamScheduling(ctx),
		ExcludedIDs:              excludedIDs,
	})
}

func accountSupportsOpenAICapabilities(account *Account, requiredCapability OpenAIEndpointCapability, requiredImageCapability OpenAIImagesCapability) bool {
	if account == nil {
		return false
	}
	return account.SupportsOpenAIEndpointCapability(requiredCapability) &&
		account.SupportsOpenAIImageCapability(requiredImageCapability)
}

func cloneExcludedAccountIDs(excludedIDs map[int64]struct{}) map[int64]struct{} {
	if len(excludedIDs) == 0 {
		return nil
	}
	cloned := make(map[int64]struct{}, len(excludedIDs))
	for id := range excludedIDs {
		cloned[id] = struct{}{}
	}
	return cloned
}

func (s *OpenAIGatewayService) isOpenAIAccountTransportCompatible(account *Account, requiredTransport OpenAIUpstreamTransport) bool {
	if requiredTransport == OpenAIUpstreamTransportAny || requiredTransport == OpenAIUpstreamTransportHTTPSSE {
		return true
	}
	if s == nil || account == nil {
		return false
	}
	if requiredTransport == OpenAIUpstreamTransportResponsesWebsocketV2Ingress {
		if !s.isOpenAIWSModeRouterV2Enabled(context.Background()) {
			return s.getOpenAIWSProtocolResolver().Resolve(account).Transport == OpenAIUpstreamTransportResponsesWebsocketV2
		}
		if s.cfg == nil {
			return false
		}
		mode := account.ResolveOpenAIResponsesWebSocketV2Mode(s.cfg.Gateway.OpenAIWS.IngressModeDefault)
		switch mode {
		case OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough, OpenAIWSIngressModeHTTPBridge, OpenAIWSIngressModeShared, OpenAIWSIngressModeDedicated:
			return true
		default:
			return false
		}
	}
	return s.getOpenAIWSProtocolResolver().Resolve(account).Transport == requiredTransport
}

func (s *OpenAIGatewayService) ReportOpenAIAccountScheduleResult(accountID int64, model string, success bool, firstTokenMs *int) {
	if success {
		s.clearOpenAIAccountModelTransientState(accountID, normalizeOpenAIAccountModelTransientModel(model))
		if s != nil && s.rateLimitService != nil {
			s.rateLimitService.resetOpenAIFailureCounterAfterSuccess(context.Background(), accountID)
		}
	}
	scheduler := s.getOpenAIAccountScheduler(context.Background())
	if scheduler == nil {
		return
	}
	scheduler.ReportResult(accountID, success, firstTokenMs)
}

// ReportOpenAIAccountStreamScheduleResult adds stream degradation recovery to
// the normal scheduler metrics hook. A successful stream clears degradation;
// other results release any claimed recovery probe for the next backoff cycle.
func (s *OpenAIGatewayService) ReportOpenAIAccountStreamScheduleResult(accountID int64, model string, success bool, firstTokenMs *int, stream bool) {
	s.ReportOpenAIAccountScheduleResult(accountID, model, success, firstTokenMs)
	if !stream {
		return
	}
	if success {
		s.clearOpenAIStreamDegradation(accountID)
		return
	}
	s.releaseOpenAIStreamRecoveryProbe(accountID)
}

func (s *OpenAIGatewayService) RecordOpenAIAccountSwitch() {
	scheduler := s.getOpenAIAccountScheduler(context.Background())
	if scheduler == nil {
		return
	}
	scheduler.ReportSwitch()
}

func (s *OpenAIGatewayService) SnapshotOpenAIAccountSchedulerMetrics() OpenAIAccountSchedulerMetricsSnapshot {
	scheduler := s.getOpenAIAccountScheduler(context.Background())
	if scheduler == nil {
		return OpenAIAccountSchedulerMetricsSnapshot{}
	}
	return scheduler.SnapshotMetrics()
}

func (s *OpenAIGatewayService) openAIWSSessionStickyTTL() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		return time.Duration(s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second
	}
	return openaiStickySessionTTL
}

func (s *OpenAIGatewayService) openAIWSLBTopK() int {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.LBTopK > 0 {
		return s.cfg.Gateway.OpenAIWS.LBTopK
	}
	return 7
}

func (s *OpenAIGatewayService) openAIWSLBTopKForRequest(ctx context.Context) int {
	base := s.openAIWSLBTopK()
	settings := s.openAIAdvancedSchedulerRuntimeSettings(ctx)
	// DB 覆盖值与 stickyWeighted/subscriptionPriority 一样受总开关门控：
	// 关闭高级调度器后所有调用方（含管理页分数快照）都应回到配置/默认行为。
	if !settings.enabled {
		return base
	}
	if settings.lbTopKOverride > 0 {
		return settings.lbTopKOverride
	}
	return base
}

func (s *OpenAIGatewayService) openAIStickyEscapeConfig() openAIStickyEscapeConfig {
	if s != nil && s.cfg != nil {
		cfg := s.cfg.Gateway.OpenAIScheduler
		enabled := cfg.StickyEscapeEnabled
		if !enabled && cfg.StickyEscapeTTFTMs == 0 && cfg.StickyEscapeErrorRate == 0 {
			enabled = true
		}
		ttftMs := float64(cfg.StickyEscapeTTFTMs)
		if ttftMs <= 0 {
			ttftMs = 15000
		}
		errorRate := cfg.StickyEscapeErrorRate
		if errorRate < 0 || errorRate > 1 {
			errorRate = 0.5
		}
		if errorRate == 0 && cfg.StickyEscapeTTFTMs == 0 && cfg.StickyEscapeErrorRate == 0 {
			errorRate = 0.5
		}
		return openAIStickyEscapeConfig{
			enabled:   enabled,
			ttftMs:    ttftMs,
			errorRate: errorRate,
		}
	}
	return openAIStickyEscapeConfig{
		enabled:   true,
		ttftMs:    15000,
		errorRate: 0.5,
	}
}

func (s *OpenAIGatewayService) openAIWSSchedulerWeights() GatewayOpenAIWSSchedulerScoreWeightsView {
	if s != nil && s.cfg != nil {
		return GatewayOpenAIWSSchedulerScoreWeightsView{
			Priority:      s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority,
			Load:          s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load,
			Queue:         s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue,
			ErrorRate:     s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate,
			TTFT:          s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT,
			Reset:         s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Reset,
			QuotaHeadroom: s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.QuotaHeadroom,
			UpstreamCost:  s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.UpstreamCost,
			Previous:      s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.PreviousResponse,
			SessionSticky: s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.SessionSticky,
		}
	}
	return GatewayOpenAIWSSchedulerScoreWeightsView{
		Priority:      1.0,
		Load:          1.0,
		Queue:         0.7,
		ErrorRate:     0.8,
		TTFT:          0.5,
		Reset:         0.0,
		QuotaHeadroom: 0.0,
		UpstreamCost:  0.0,
		Previous:      5.0,
		SessionSticky: 3.0,
	}
}

func (s *OpenAIGatewayService) openAIWSSchedulerWeightsForRequest(ctx context.Context) GatewayOpenAIWSSchedulerScoreWeightsView {
	weights := s.openAIWSSchedulerWeights()
	settings := s.openAIAdvancedSchedulerRuntimeSettings(ctx)
	// 同 openAIWSLBTopKForRequest：总开关关闭时不应用 DB 覆盖值。
	if !settings.enabled {
		return weights
	}
	overridden := applyOpenAIAdvancedSchedulerWeightOverrides(weights, settings.weightOverrides)
	if !overridden.configWeights().IsValid() {
		return weights
	}
	return overridden
}

func applyOpenAIAdvancedSchedulerWeightOverrides(
	weights GatewayOpenAIWSSchedulerScoreWeightsView,
	overrides map[string]float64,
) GatewayOpenAIWSSchedulerScoreWeightsView {
	for key, value := range overrides {
		switch key {
		case "priority":
			weights.Priority = value
		case "load":
			weights.Load = value
		case "queue":
			weights.Queue = value
		case "error_rate":
			weights.ErrorRate = value
		case "ttft":
			weights.TTFT = value
		case "reset":
			weights.Reset = value
		case "quota_headroom":
			weights.QuotaHeadroom = value
		case "upstream_cost":
			weights.UpstreamCost = value
		case "previous_response":
			weights.Previous = value
		case "session_sticky":
			weights.SessionSticky = value
		}
	}
	return weights
}

type GatewayOpenAIWSSchedulerScoreWeightsView struct {
	Priority  float64
	Load      float64
	Queue     float64
	ErrorRate float64
	TTFT      float64
	// Reset 倾向「会话窗口最早重置」的账号；0 表示关闭（默认）。
	Reset         float64
	QuotaHeadroom float64
	UpstreamCost  float64
	Previous      float64
	SessionSticky float64
}

func (w GatewayOpenAIWSSchedulerScoreWeightsView) configWeights() config.GatewayOpenAIWSSchedulerScoreWeights {
	return config.GatewayOpenAIWSSchedulerScoreWeights{
		Priority:         w.Priority,
		Load:             w.Load,
		Queue:            w.Queue,
		ErrorRate:        w.ErrorRate,
		TTFT:             w.TTFT,
		Reset:            w.Reset,
		QuotaHeadroom:    w.QuotaHeadroom,
		UpstreamCost:     w.UpstreamCost,
		PreviousResponse: w.Previous,
		SessionSticky:    w.SessionSticky,
	}
}

type OpenAIAccountSchedulerScoreSnapshot struct {
	BaseScore             float64
	StickyScore           float64
	StickyScoreInfinity   bool
	StickyWeightedEnabled bool
}

func (s *RateLimitService) BuildOpenAIAccountSchedulerScoreSnapshot(
	ctx context.Context,
	accounts []*Account,
	loadMap map[int64]*AccountLoadInfo,
) map[int64]OpenAIAccountSchedulerScoreSnapshot {
	gateway := &OpenAIGatewayService{cfg: nil, rateLimitService: s}
	if s != nil {
		gateway.cfg = s.cfg
	}
	return buildOpenAIAccountSchedulerScoreSnapshot(
		accounts,
		loadMap,
		gateway.openAIWSSchedulerWeightsForRequest(ctx),
		gateway.isOpenAIAdvancedSchedulerStickyWeightedEnabled(ctx),
		gateway.openAIOAuthSchedulingRateMultiplier(ctx),
	)
}

func BuildOpenAIAccountSchedulerScoreSnapshot(
	accounts []*Account,
	loadMap map[int64]*AccountLoadInfo,
) map[int64]OpenAIAccountSchedulerScoreSnapshot {
	gateway := &OpenAIGatewayService{}
	return buildOpenAIAccountSchedulerScoreSnapshot(accounts, loadMap, gateway.openAIWSSchedulerWeights(), false, defaultOpenAIOAuthSchedulingRateMultiplier)
}

func buildOpenAIAccountSchedulerScoreSnapshot(
	accounts []*Account,
	loadMap map[int64]*AccountLoadInfo,
	weights GatewayOpenAIWSSchedulerScoreWeightsView,
	stickyWeightedEnabled bool,
	oauthSchedulingRateMultiplier float64,
) map[int64]OpenAIAccountSchedulerScoreSnapshot {
	if len(accounts) == 0 {
		return nil
	}
	candidates := make([]openAIAccountCandidateScore, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		candidates = append(candidates, openAIAccountCandidateScore{
			account:   account,
			loadInfo:  loadInfo,
			errorRate: 0,
			ttft:      0,
			hasTTFT:   false,
		})
	}
	if len(candidates) == 0 {
		return nil
	}

	minPriority, maxPriority := openAIAccountSchedulingPriority(candidates[0].account), openAIAccountSchedulingPriority(candidates[0].account)
	maxWaiting := 1
	for i := range candidates {
		candidate := &candidates[i]
		candidate.priority = openAIAccountSchedulingPriority(candidate.account)
		if candidate.priority < minPriority {
			minPriority = candidate.priority
		}
		if candidate.priority > maxPriority {
			maxPriority = candidate.priority
		}
		if candidate.loadInfo.WaitingCount > maxWaiting {
			maxWaiting = candidate.loadInfo.WaitingCount
		}
	}

	minResetRemaining, maxResetRemaining := 0.0, 0.0
	hasResetSample := false
	now := time.Now()
	if weights.UpstreamCost > 0 {
		applyOpenAIUpstreamCostFactors(candidates, now, oauthSchedulingRateMultiplier)
	}
	if weights.Reset > 0 {
		for _, candidate := range candidates {
			end, ok := openAISchedulingResetWindowEnd(candidate.account, now)
			if !ok {
				continue
			}
			remaining := end.Sub(now).Seconds()
			if !hasResetSample {
				minResetRemaining, maxResetRemaining = remaining, remaining
				hasResetSample = true
				continue
			}
			if remaining < minResetRemaining {
				minResetRemaining = remaining
			}
			if remaining > maxResetRemaining {
				maxResetRemaining = remaining
			}
		}
	}

	result := make(map[int64]OpenAIAccountSchedulerScoreSnapshot, len(candidates))
	for _, candidate := range candidates {
		priorityFactor := 1.0
		if maxPriority > minPriority {
			priorityFactor = 1 - float64(candidate.priority-minPriority)/float64(maxPriority-minPriority)
		}
		loadFactor := 1 - clamp01(float64(candidate.loadInfo.LoadRate)/100.0)
		queueFactor := 1 - clamp01(float64(candidate.loadInfo.WaitingCount)/float64(maxWaiting))
		errorFactor := 1.0
		ttftFactor := 0.5
		resetFactor := 0.0
		if weights.Reset > 0 && hasResetSample {
			if end, ok := openAISchedulingResetWindowEnd(candidate.account, now); ok {
				if maxResetRemaining > minResetRemaining {
					resetFactor = 1 - clamp01((end.Sub(now).Seconds()-minResetRemaining)/(maxResetRemaining-minResetRemaining))
				} else {
					resetFactor = 1
				}
			}
		}
		quotaHeadroomFactor := 0.0
		if weights.QuotaHeadroom > 0 {
			quotaHeadroomFactor = openAIQuotaHeadroomFactor(candidate.account, now)
		}
		upstreamCostFactor := openAIUpstreamCostNeutralFactor
		if weights.UpstreamCost > 0 {
			upstreamCostFactor = candidate.upstreamCostFactor
		}
		baseScore := weights.Priority*priorityFactor +
			weights.Load*loadFactor +
			weights.Queue*queueFactor +
			weights.ErrorRate*errorFactor +
			weights.TTFT*ttftFactor +
			weights.Reset*resetFactor +
			weights.QuotaHeadroom*quotaHeadroomFactor +
			weights.UpstreamCost*(upstreamCostFactor-openAIUpstreamCostNeutralFactor)
		score := OpenAIAccountSchedulerScoreSnapshot{
			BaseScore:             baseScore,
			StickyWeightedEnabled: stickyWeightedEnabled,
			StickyScoreInfinity:   !stickyWeightedEnabled,
		}
		if stickyWeightedEnabled {
			score.StickyScore = baseScore + weights.Previous + weights.SessionSticky
		}
		result[candidate.account.ID] = score
	}
	return result
}

type openAIUpstreamCostSample struct {
	target    int
	accountID int64
	rate      float64
	factor    float64
}

func openAIUpstreamCostFactors(accounts []*Account, now time.Time, oauthSchedulingRateMultiplier float64) map[int64]float64 {
	factors := make(map[int64]float64, len(accounts))
	samples := make([]openAIUpstreamCostSample, 0, len(accounts))
	eligibleCount := 0
	for _, account := range accounts {
		if account == nil {
			continue
		}
		factors[account.ID] = openAIUpstreamCostNeutralFactor
		if !account.IsOpenAIApiKey() && !account.IsOpenAIOAuth() {
			continue
		}
		eligibleCount++
		if rate, ok := openAISchedulingRate(account, now, oauthSchedulingRateMultiplier); ok {
			samples = append(samples, openAIUpstreamCostSample{accountID: account.ID, rate: rate})
		}
	}
	if normalizeOpenAIUpstreamCostSamples(samples, eligibleCount) {
		for _, sample := range samples {
			factors[sample.accountID] = sample.factor
		}
	}
	return factors
}

func applyOpenAIUpstreamCostFactors(candidates []openAIAccountCandidateScore, now time.Time, oauthSchedulingRateMultiplier float64) bool {
	samples := make([]openAIUpstreamCostSample, 0, len(candidates))
	eligibleCount := 0
	for i := range candidates {
		candidate := &candidates[i]
		candidate.upstreamCostFactor = openAIUpstreamCostNeutralFactor
		account := candidate.account
		if account == nil || (!account.IsOpenAIApiKey() && !account.IsOpenAIOAuth()) {
			continue
		}
		eligibleCount++
		if rate, ok := openAISchedulingRate(account, now, oauthSchedulingRateMultiplier); ok {
			samples = append(samples, openAIUpstreamCostSample{target: i, rate: rate})
		}
	}
	if !normalizeOpenAIUpstreamCostSamples(samples, eligibleCount) {
		return false
	}
	for _, sample := range samples {
		candidates[sample.target].upstreamCostFactor = sample.factor
	}
	return true
}

func normalizeOpenAIUpstreamCostSamples(samples []openAIUpstreamCostSample, eligibleCount int) bool {
	if len(samples) < 2 || eligibleCount == 0 {
		return false
	}
	allEqual := true
	for i, sample := range samples {
		if i > 0 && sample.rate != samples[0].rate {
			allEqual = false
		}
	}
	if allEqual {
		return false
	}

	slices.SortFunc(samples, func(a, b openAIUpstreamCostSample) int {
		switch {
		case a.rate < b.rate:
			return -1
		case a.rate > b.rate:
			return 1
		default:
			return 0
		}
	})
	positiveStart := 0
	for positiveStart < len(samples) && samples[positiveStart].rate <= 0 {
		positiveStart++
	}
	positiveCount := len(samples) - positiveStart
	if positiveCount == 0 {
		return false
	}
	middle := positiveStart + positiveCount/2
	center := samples[middle].rate
	if positiveCount%2 == 0 {
		center = math.Exp((math.Log(samples[middle-1].rate) + math.Log(samples[middle].rate)) / 2)
	}
	if center <= 0 || math.IsNaN(center) || math.IsInf(center, 0) {
		return false
	}

	coverage := float64(len(samples)) / float64(eligibleCount)
	hasSignal := false
	for i := range samples {
		sample := &samples[i]
		rawFactor := 1.0
		if sample.rate > 0 {
			rawFactor = 1 / (1 + sample.rate/center)
		}
		sample.factor = clamp01(openAIUpstreamCostNeutralFactor + coverage*(rawFactor-openAIUpstreamCostNeutralFactor))
		hasSignal = hasSignal || sample.factor != openAIUpstreamCostNeutralFactor
	}
	return hasSignal
}

type openAILegacyUpstreamRateOrder struct {
	enabled bool
	rates   map[int64]float64
}

func newOpenAILegacyUpstreamRateOrder(accounts []*Account, now time.Time, oauthSchedulingRateMultiplier float64) openAILegacyUpstreamRateOrder {
	rates := make(map[int64]float64, len(accounts))
	var first float64
	distinct := false
	for _, account := range accounts {
		if account == nil {
			continue
		}
		if !account.IsOpenAIApiKey() && !account.IsOpenAIOAuth() {
			continue
		}
		rate, ok := openAISchedulingRate(account, now, oauthSchedulingRateMultiplier)
		if !ok {
			continue
		}
		if len(rates) == 0 {
			first = rate
		} else if rate != first {
			distinct = true
		}
		rates[account.ID] = rate
	}
	return openAILegacyUpstreamRateOrder{enabled: len(rates) >= 2 && distinct, rates: rates}
}

func openAISchedulingRate(account *Account, now time.Time, oauthSchedulingRateMultiplier float64) (float64, bool) {
	if account != nil && account.IsOpenAIOAuth() {
		return oauthSchedulingRateMultiplier, true
	}
	return openAIFreshUpstreamBillingRate(account, now)
}

// compare returns -1 when a should be selected before b, 1 when b should be
// selected first, and 0 when the rate signal does not distinguish them.
func (o openAILegacyUpstreamRateOrder) compare(a, b *Account) int {
	if !o.enabled || a == nil || b == nil {
		return 0
	}
	aRate, aKnown := o.rates[a.ID]
	bRate, bKnown := o.rates[b.ID]
	if aKnown != bKnown {
		if aKnown {
			return -1
		}
		return 1
	}
	if !aKnown || aRate == bRate {
		return 0
	}
	if aRate < bRate {
		return -1
	}
	return 1
}

func openAIFreshUpstreamBillingRate(account *Account, now time.Time) (float64, bool) {
	if !isUpstreamBillingProbeAccount(account) {
		return 0, false
	}
	snapshot, ok := readOpenAIUpstreamBillingScheduleSnapshot(account.Extra)
	if !ok || (snapshot.status != UpstreamBillingProbeStatusOK && snapshot.status != UpstreamBillingProbeStatusFailed) ||
		!snapshot.hasReceivedAt || snapshot.receivedAt.IsZero() {
		return 0, false
	}
	receivedAt := snapshot.receivedAt
	freshUntil := snapshot.freshUntil
	hasFreshUntil := snapshot.hasFreshUntil
	if !hasFreshUntil && snapshot.status == UpstreamBillingProbeStatusOK {
		interval := snapshot.nextProbeAt.Sub(receivedAt)
		if interval > 0 {
			freshUntil = receivedAt.Add(2 * interval)
			hasFreshUntil = true
		}
	}
	if !hasFreshUntil || !freshUntil.After(receivedAt) || now.Before(receivedAt) || now.After(freshUntil) {
		return 0, false
	}
	return upstreamBillingRateAt(snapshot.data, now)
}

type openAIUpstreamBillingScheduleSnapshot struct {
	status        string
	data          map[string]any
	receivedAt    time.Time
	freshUntil    time.Time
	nextProbeAt   time.Time
	hasReceivedAt bool
	hasFreshUntil bool
}

func readOpenAIUpstreamBillingScheduleSnapshot(extra map[string]any) (openAIUpstreamBillingScheduleSnapshot, bool) {
	if extra == nil {
		return openAIUpstreamBillingScheduleSnapshot{}, false
	}
	value, ok := extra[UpstreamBillingProbeExtraKey]
	if !ok || value == nil {
		return openAIUpstreamBillingScheduleSnapshot{}, false
	}

	switch snapshot := value.(type) {
	case *UpstreamBillingProbeSnapshot:
		return readTypedOpenAIUpstreamBillingScheduleSnapshot(snapshot)
	case UpstreamBillingProbeSnapshot:
		return readTypedOpenAIUpstreamBillingScheduleSnapshot(&snapshot)
	case map[string]any:
		status, _ := snapshot["status"].(string)
		if status == "" {
			return openAIUpstreamBillingScheduleSnapshot{}, false
		}
		data, _ := snapshot["data"].(map[string]any)
		result := openAIUpstreamBillingScheduleSnapshot{status: status, data: data}
		var valid bool
		result.receivedAt, result.hasReceivedAt, valid = parseOpenAIUpstreamBillingScheduleTime(snapshot["received_at"])
		if !valid {
			return openAIUpstreamBillingScheduleSnapshot{}, false
		}
		result.freshUntil, result.hasFreshUntil, valid = parseOpenAIUpstreamBillingScheduleTime(snapshot["fresh_until"])
		if !valid {
			return openAIUpstreamBillingScheduleSnapshot{}, false
		}
		result.nextProbeAt, _, valid = parseOpenAIUpstreamBillingScheduleTime(snapshot["next_probe_at"])
		if !valid {
			return openAIUpstreamBillingScheduleSnapshot{}, false
		}
		return result, true
	default:
		return openAIUpstreamBillingScheduleSnapshot{}, false
	}
}

func readTypedOpenAIUpstreamBillingScheduleSnapshot(snapshot *UpstreamBillingProbeSnapshot) (openAIUpstreamBillingScheduleSnapshot, bool) {
	if snapshot == nil || snapshot.Status == "" {
		return openAIUpstreamBillingScheduleSnapshot{}, false
	}
	result := openAIUpstreamBillingScheduleSnapshot{
		status:      snapshot.Status,
		data:        snapshot.Data,
		nextProbeAt: snapshot.NextProbeAt,
	}
	if snapshot.ReceivedAt != nil {
		result.receivedAt = *snapshot.ReceivedAt
		result.hasReceivedAt = true
	}
	if snapshot.FreshUntil != nil {
		result.freshUntil = *snapshot.FreshUntil
		result.hasFreshUntil = true
	}
	return result, true
}

func parseOpenAIUpstreamBillingScheduleTime(value any) (parsedTime time.Time, present bool, valid bool) {
	switch parsed := value.(type) {
	case nil:
		return time.Time{}, false, true
	case time.Time:
		return parsed, true, true
	case *time.Time:
		if parsed == nil {
			return time.Time{}, false, true
		}
		return *parsed, true, true
	case string:
		result, err := time.Parse(time.RFC3339Nano, parsed)
		return result, true, err == nil
	default:
		return time.Time{}, true, false
	}
}

func openAIQuotaHeadroomFactor(account *Account, now time.Time) float64 {
	if account == nil || len(account.Extra) == 0 || openAIQuotaHeadroomSnapshotStale(account.Extra, now) {
		return openAIQuotaHeadroomNeutralFactor
	}
	window5h, window7d := openAICanonicalQuotaWindows(account.Extra, now)
	if !window7d.hasUsed || window7d.reset {
		return openAIQuotaHeadroomNeutralFactor
	}

	factor := 1 - clamp01(window7d.usedPercent/100)
	if window5h.hasUsed && !window5h.reset {
		remaining := 1 - clamp01(window5h.usedPercent/100)
		if remaining < openAIQuotaHeadroomSecondaryLowRemain {
			factor *= openAIQuotaHeadroomNeutralFactor
		}
	}
	return factor
}

type openAICanonicalQuotaWindow struct {
	usedPercent float64
	hasUsed     bool
	reset       bool
}

func openAICanonicalQuotaWindows(extra map[string]any, now time.Time) (window5h, window7d openAICanonicalQuotaWindow) {
	if used, ok := resolveAccountExtraNumber(extra, "codex_5h_used_percent"); ok {
		window5h = openAICanonicalQuotaWindow{usedPercent: used, hasUsed: true, reset: openAIQuotaWindowReset(extra, "5h", now)}
	}
	if used, ok := resolveAccountExtraNumber(extra, "codex_7d_used_percent"); ok {
		window7d = openAICanonicalQuotaWindow{usedPercent: used, hasUsed: true, reset: openAIQuotaWindowReset(extra, "7d", now)}
	}
	if window5h.hasUsed && window7d.hasUsed {
		return window5h, window7d
	}

	snapshot := &OpenAICodexUsageSnapshot{}
	if used, ok := resolveAccountExtraNumber(extra, "codex_primary_used_percent"); ok {
		snapshot.PrimaryUsedPercent = &used
	}
	if used, ok := resolveAccountExtraNumber(extra, "codex_secondary_used_percent"); ok {
		snapshot.SecondaryUsedPercent = &used
	}
	if minutes := parseExtraInt(extra["codex_primary_window_minutes"]); minutes > 0 {
		snapshot.PrimaryWindowMinutes = &minutes
	}
	if minutes := parseExtraInt(extra["codex_secondary_window_minutes"]); minutes > 0 {
		snapshot.SecondaryWindowMinutes = &minutes
	}
	normalized := snapshot.Normalize()
	if normalized == nil {
		return window5h, window7d
	}
	fromRaw := func(used *float64) openAICanonicalQuotaWindow {
		if used == nil {
			return openAICanonicalQuotaWindow{}
		}
		window := "secondary"
		if used == snapshot.PrimaryUsedPercent {
			window = "primary"
		}
		return openAICanonicalQuotaWindow{usedPercent: *used, hasUsed: true, reset: openAIQuotaWindowReset(extra, window, now)}
	}
	if !window5h.hasUsed {
		window5h = fromRaw(normalized.Used5hPercent)
	}
	if !window7d.hasUsed {
		window7d = fromRaw(normalized.Used7dPercent)
	}
	return window5h, window7d
}

func openAISchedulingResetWindowEnd(account *Account, now time.Time) (time.Time, bool) {
	if account == nil {
		return time.Time{}, false
	}
	if end, ok := openAICodexWindowResetAt(account.Extra, "5h"); ok && now.Before(end) {
		return end, true
	}
	if end := account.SessionWindowEnd; end != nil && now.Before(*end) {
		return *end, true
	}
	return time.Time{}, false
}

func openAIQuotaHeadroomSnapshotStale(extra map[string]any, now time.Time) bool {
	updatedRaw, ok := extra["codex_usage_updated_at"]
	if !ok {
		return true
	}
	updatedAt, err := parseTime(fmt.Sprint(updatedRaw))
	if err != nil {
		return true
	}
	return now.Sub(updatedAt) >= openAIQuotaHeadroomSnapshotStaleAfter
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func calcLoadSkewByMoments(sum float64, sumSquares float64, count int) float64 {
	if count <= 1 {
		return 0
	}
	mean := sum / float64(count)
	variance := sumSquares/float64(count) - mean*mean
	if variance < 0 {
		variance = 0
	}
	return math.Sqrt(variance)
}

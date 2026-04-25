package service

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	openAIAccountScheduleLayerPreviousResponse = "previous_response_id"
	openAIAccountScheduleLayerSessionSticky    = "session_hash"
	openAIAccountScheduleLayerLoadBalance      = "load_balance"
	openAISelectedGroupReserve                 = "reserve"
	openAIAffinityDomainActive                 = "active"
	openAIAffinityDomainExhausted              = "exhausted"
)

const (
	openAIStickyEvalResultHit                      = "hit"
	openAIStickyEvalResultMissNoBinding            = "miss_no_binding"
	openAIStickyEvalResultMissBindingInvalid       = "miss_binding_invalid"
	openAIStickyEvalResultMissBindingRestricted    = "miss_binding_restricted"
	openAIStickyEvalResultMissBindingExcluded      = "miss_binding_excluded"
	openAIStickyEvalResultBypassedPreviousResponse = "bypassed_previous_response_id"
	openAIStickyEvalResultNoSessionSignal          = "no_session_signal"
	openAIStickySessionSourceNone                  = "none"
)

type AccountTargetGroup string

const (
	TargetGroupAny       AccountTargetGroup = ""
	TargetGroupActive    AccountTargetGroup = "active"
	TargetGroupExhausted AccountTargetGroup = "exhausted"
)

func normalizeTargetGroup(group AccountTargetGroup) AccountTargetGroup {
	switch strings.ToLower(strings.TrimSpace(string(group))) {
	case string(TargetGroupActive):
		return TargetGroupActive
	case string(TargetGroupExhausted):
		return TargetGroupExhausted
	default:
		return TargetGroupAny
	}
}

type OpenAIAccountScheduleRequest struct {
	GroupID              *int64
	TargetGroup          AccountTargetGroup
	SessionHash          string
	SessionSource        string
	StickyAccountID      int64
	ParentSessionPresent bool
	ParentSessionKey     string
	PreviousResponseID   string
	RequestedModel       string
	RequiredTransport    OpenAIUpstreamTransport
	ExcludedIDs          map[int64]struct{}
}

type openAIAffinityBinding struct {
	BoundAccountID     int64      `json:"bound_account_id"`
	AffinityDomain     string     `json:"affinity_domain"`
	SelectedGroup      string     `json:"selected_group"`
	ProjectionVersion  int64      `json:"projection_version,omitempty"`
	ProjectionModelKey string     `json:"projection_model_key,omitempty"`
	ProjectionBuiltAt  *time.Time `json:"projection_built_at,omitempty"`
}

type openAIStickyEval struct {
	SessionSource          string
	SessionHashPresent     bool
	EvalResult             string
	SelectedAccountChanged bool
	ParentSessionPresent   bool
	ParentSessionKey       string
	BoundAccountID         *int64
	AffinityBinding        *openAIAffinityBinding
}

func cloneOpenAIAffinityBinding(binding *openAIAffinityBinding) *openAIAffinityBinding {
	if binding == nil {
		return nil
	}
	cloned := *binding
	cloned.AffinityDomain = normalizeOpenAIAffinityDomain(cloned.AffinityDomain)
	cloned.SelectedGroup = normalizeOpenAISelectedGroup(cloned.SelectedGroup)
	cloned.ProjectionModelKey = NormalizeOpenAIProjectionModelKey(cloned.ProjectionModelKey)
	cloned.ProjectionBuiltAt = cloneOpenAIProjectionBuiltAt(binding.ProjectionBuiltAt)
	return &cloned
}

func normalizeOpenAIProjectionBuiltAt(builtAt time.Time) time.Time {
	if builtAt.IsZero() {
		return time.Time{}
	}
	return builtAt.UTC()
}

func cloneOpenAIProjectionBuiltAt(builtAt *time.Time) *time.Time {
	if builtAt == nil || builtAt.IsZero() {
		return nil
	}
	normalized := builtAt.UTC()
	return &normalized
}

func derefOpenAIProjectionBuiltAt(builtAt *time.Time) time.Time {
	if builtAt == nil {
		return time.Time{}
	}
	return normalizeOpenAIProjectionBuiltAt(*builtAt)
}

func normalizeOpenAISelectedGroup(group string) string {
	switch strings.ToLower(strings.TrimSpace(group)) {
	case "":
		return ""
	case string(TargetGroupActive):
		return string(TargetGroupActive)
	case string(TargetGroupExhausted):
		return string(TargetGroupExhausted)
	case openAISelectedGroupReserve:
		return openAISelectedGroupReserve
	default:
		return string(TargetGroupActive)
	}
}

func normalizeOpenAIAffinityDomain(domain string) string {
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case openAIAffinityDomainExhausted:
		return openAIAffinityDomainExhausted
	default:
		return openAIAffinityDomainActive
	}
}

func newOpenAIAffinityBinding(accountID int64, selectedGroup string) *openAIAffinityBinding {
	selectedGroup = normalizeOpenAISelectedGroup(selectedGroup)
	if accountID <= 0 || selectedGroup == "" {
		return nil
	}
	binding := &openAIAffinityBinding{
		BoundAccountID: accountID,
		SelectedGroup:  selectedGroup,
		AffinityDomain: openAIAffinityDomainActive,
	}
	if selectedGroup == string(TargetGroupExhausted) || selectedGroup == openAISelectedGroupReserve {
		binding.AffinityDomain = openAIAffinityDomainExhausted
	}
	return binding
}

func isOpenAIReserveAffinityBinding(binding *openAIAffinityBinding) bool {
	if binding == nil {
		return false
	}
	return binding.BoundAccountID > 0 &&
		normalizeOpenAIAffinityDomain(binding.AffinityDomain) == openAIAffinityDomainExhausted &&
		normalizeOpenAISelectedGroup(binding.SelectedGroup) == openAISelectedGroupReserve
}

func resolveOpenAISelectedGroupFromBindingOrAccount(binding *openAIAffinityBinding, account *Account) string {
	if binding != nil {
		if selectedGroup := normalizeOpenAISelectedGroup(binding.SelectedGroup); selectedGroup != "" {
			if selectedGroup == openAISelectedGroupReserve && account != nil && account.MatchesTargetGroup(TargetGroupExhausted) {
				return string(TargetGroupExhausted)
			}
			return selectedGroup
		}
	}
	return resolveOpenAISelectedGroupFromAccount(account)
}

type OpenAIAccountScheduleDecision struct {
	Layer               string
	StickyPreviousHit   bool
	StickySessionHit    bool
	Sticky              *openAIStickyEval
	CandidateCount      int
	TopK                int
	LatencyMs           int64
	LoadSkew            float64
	SelectedAccountID   int64
	SelectedAccountType string
	SelectedGroup       string
	ProjectionVersion   int64
	ProjectionModelKey  string
	ProjectionBuiltAt   time.Time
}

func (d *OpenAIAccountScheduleDecision) setProjectionMetadata(version int64, modelKey string, builtAt time.Time) {
	if d == nil {
		return
	}
	if version > 0 {
		d.ProjectionVersion = version
	}
	if normalizedModelKey := NormalizeOpenAIProjectionModelKey(modelKey); normalizedModelKey != "" {
		d.ProjectionModelKey = normalizedModelKey
	}
	if normalizedBuiltAt := normalizeOpenAIProjectionBuiltAt(builtAt); !normalizedBuiltAt.IsZero() {
		d.ProjectionBuiltAt = normalizedBuiltAt
	}
}

func (d *OpenAIAccountScheduleDecision) setProjectionMetadataFromBinding(binding *openAIAffinityBinding) {
	if binding == nil {
		return
	}
	d.setProjectionMetadata(binding.ProjectionVersion, binding.ProjectionModelKey, derefOpenAIProjectionBuiltAt(binding.ProjectionBuiltAt))
}

func (d *OpenAIAccountScheduleDecision) setProjectionMetadataFromView(view *openAIProjectionViewResult) {
	if view == nil || view.state == nil {
		return
	}
	d.setProjectionMetadata(view.state.ProjectionVersion, view.canonicalModel, view.state.BuiltAt)
}

func normalizeOpenAIStickySessionSource(source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return openAIStickySessionSourceNone
	}
	return trimmed
}

func newOpenAIStickyEval(req OpenAIAccountScheduleRequest) *openAIStickyEval {
	eval := &openAIStickyEval{
		SessionSource:        normalizeOpenAIStickySessionSource(req.SessionSource),
		SessionHashPresent:   strings.TrimSpace(req.SessionHash) != "",
		ParentSessionPresent: req.ParentSessionPresent,
		ParentSessionKey:     strings.TrimSpace(req.ParentSessionKey),
	}
	eval.setBoundAccountID(req.StickyAccountID)
	if eval.ParentSessionKey != "" {
		eval.ParentSessionPresent = true
	}
	if eval.SessionSource == openAIStickySessionSourceNone {
		eval.EvalResult = openAIStickyEvalResultNoSessionSignal
	}
	return eval
}

func cloneOpenAIStickyEval(eval *openAIStickyEval) *openAIStickyEval {
	if eval == nil {
		return nil
	}
	cloned := *eval
	if eval.BoundAccountID != nil {
		boundAccountID := *eval.BoundAccountID
		cloned.BoundAccountID = &boundAccountID
	}
	cloned.AffinityBinding = cloneOpenAIAffinityBinding(eval.AffinityBinding)
	return &cloned
}

func (e *openAIStickyEval) setBoundAccountID(accountID int64) {
	if e == nil {
		return
	}
	if accountID <= 0 {
		e.BoundAccountID = nil
		return
	}
	boundAccountID := accountID
	e.BoundAccountID = &boundAccountID
}

func (e *openAIStickyEval) setAffinityBinding(binding *openAIAffinityBinding) {
	if e == nil {
		return
	}
	e.AffinityBinding = cloneOpenAIAffinityBinding(binding)
	if e.AffinityBinding != nil {
		e.setBoundAccountID(e.AffinityBinding.BoundAccountID)
	}
}

func (e *openAIStickyEval) setEvalResult(result string) {
	if e == nil {
		return
	}
	result = strings.TrimSpace(result)
	if result == "" {
		return
	}
	if e.EvalResult == openAIStickyEvalResultNoSessionSignal {
		switch result {
		case openAIStickyEvalResultHit, openAIStickyEvalResultBypassedPreviousResponse:
			// 实际命中 sticky / previous_response_id 时，应该覆盖初始 no_session_signal。
		case openAIStickyEvalResultMissBindingInvalid, openAIStickyEvalResultMissBindingRestricted, openAIStickyEvalResultMissBindingExcluded:
			if e.BoundAccountID == nil {
				return
			}
		default:
			return
		}
	}
	e.EvalResult = result
}

func (e *openAIStickyEval) finalizeSelectedAccount(selectedAccountID int64) {
	if e == nil || e.BoundAccountID == nil || selectedAccountID <= 0 {
		e.SelectedAccountChanged = false
		return
	}
	e.SelectedAccountChanged = *e.BoundAccountID != selectedAccountID
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
	loadBalanceSelectTotal atomic.Int64
	accountSwitchTotal     atomic.Int64
	latencyMsTotal         atomic.Int64
	loadSkewMilliTotal     atomic.Int64
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
}

type openAIAccountRuntimeStat struct {
	errorRateEWMABits atomic.Uint64
	ttftEWMABits      atomic.Uint64
}

func newOpenAIAccountRuntimeStats() *openAIAccountRuntimeStats {
	return &openAIAccountRuntimeStats{}
}

func (s *openAIAccountRuntimeStats) loadOrCreate(accountID int64) *openAIAccountRuntimeStat {
	if value, ok := s.accounts.Load(accountID); ok {
		stat, _ := value.(*openAIAccountRuntimeStat)
		if stat != nil {
			return stat
		}
	}

	stat := &openAIAccountRuntimeStat{}
	stat.ttftEWMABits.Store(math.Float64bits(math.NaN()))
	actual, loaded := s.accounts.LoadOrStore(accountID, stat)
	if !loaded {
		s.accountCount.Add(1)
		return stat
	}
	existing, _ := actual.(*openAIAccountRuntimeStat)
	if existing != nil {
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
	errorRate = clamp01(math.Float64frombits(stat.errorRateEWMABits.Load()))
	ttftValue := math.Float64frombits(stat.ttftEWMABits.Load())
	if math.IsNaN(ttftValue) {
		return errorRate, 0, false
	}
	return errorRate, ttftValue, true
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
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	req.TargetGroup = normalizeTargetGroup(req.TargetGroup)
	req.SessionHash = strings.TrimSpace(req.SessionHash)
	req.SessionSource = normalizeOpenAIStickySessionSource(req.SessionSource)
	req.PreviousResponseID = strings.TrimSpace(req.PreviousResponseID)
	req.ParentSessionKey = strings.TrimSpace(req.ParentSessionKey)
	if req.ParentSessionKey != "" {
		req.ParentSessionPresent = true
	}
	decision := OpenAIAccountScheduleDecision{Sticky: newOpenAIStickyEval(req)}
	if req.StickyAccountID <= 0 && req.SessionHash != "" && s != nil && s.service != nil && s.service.cache != nil {
		if accountID, err := s.service.getStickySessionAccountID(ctx, req.GroupID, req.SessionHash); err == nil && accountID > 0 {
			req.StickyAccountID = accountID
			if decision.Sticky != nil {
				decision.Sticky.setBoundAccountID(accountID)
			}
		}
	}
	start := time.Now()
	defer func() {
		decision.LatencyMs = time.Since(start).Milliseconds()
		if decision.Sticky != nil {
			decision.Sticky.finalizeSelectedAccount(decision.SelectedAccountID)
		}
		s.metrics.recordSelect(decision)
	}()

	previousResponseID := req.PreviousResponseID
	if previousResponseID != "" {
		selection, affinityBinding, err := s.service.SelectAccountByPreviousResponseID(
			ctx,
			req.GroupID,
			previousResponseID,
			req.RequestedModel,
			req.TargetGroup,
			req.ExcludedIDs,
		)
		if err != nil {
			return nil, decision, err
		}
		if selection != nil && selection.Account != nil {
			if !s.isAccountTransportCompatible(selection.Account, req.RequiredTransport) {
				selection = nil
			}
		}
		if selection != nil && selection.Account != nil {
			decision.Layer = openAIAccountScheduleLayerPreviousResponse
			decision.StickyPreviousHit = true
			if decision.Sticky != nil {
				var previousBoundAccountID int64
				if decision.Sticky.BoundAccountID != nil {
					previousBoundAccountID = *decision.Sticky.BoundAccountID
				}
				decision.Sticky.setAffinityBinding(affinityBinding)
				if previousBoundAccountID > 0 {
					decision.Sticky.setBoundAccountID(previousBoundAccountID)
				}
				decision.Sticky.setEvalResult(openAIStickyEvalResultBypassedPreviousResponse)
			}
			decision.SelectedAccountID = selection.Account.ID
			decision.SelectedAccountType = selection.Account.Type
			decision.SelectedGroup = normalizeOpenAISelectedGroup(selection.SelectedGroup)
			if decision.SelectedGroup == "" {
				decision.SelectedGroup = resolveOpenAISelectedGroupFromBindingOrAccount(affinityBinding, selection.Account)
			}
			if decision.Sticky != nil {
				decision.Sticky.SelectedAccountChanged = decision.Sticky.BoundAccountID != nil && *decision.Sticky.BoundAccountID != decision.SelectedAccountID
				decision.Sticky.finalizeSelectedAccount(decision.SelectedAccountID)
			}
			decision.setProjectionMetadataFromBinding(affinityBinding)
			if req.SessionHash != "" {
				_ = s.service.bindOpenAIStickySessionAffinity(ctx, req.GroupID, req.SessionHash, affinityBinding)
			}
			return selection, decision, nil
		}
	}

	selection, err := s.selectBySessionHash(ctx, req, decision.Sticky)
	if err != nil {
		return nil, decision, err
	}
	if selection != nil && selection.Account != nil {
		decision.Layer = openAIAccountScheduleLayerSessionSticky
		decision.StickySessionHit = true
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
		decision.SelectedGroup = normalizeOpenAISelectedGroup(selection.SelectedGroup)
		if decision.SelectedGroup == "" {
			decision.SelectedGroup = resolveOpenAISelectedGroupFromBindingOrAccount(decision.Sticky.AffinityBinding, selection.Account)
		}
		decision.setProjectionMetadataFromBinding(decision.Sticky.AffinityBinding)
		return selection, decision, nil
	}

	selection, selectedGroup, candidateCount, topK, loadSkew, projectionView, err := s.selectByLoadBalance(ctx, req)
	decision.Layer = openAIAccountScheduleLayerLoadBalance
	decision.CandidateCount = candidateCount
	decision.TopK = topK
	decision.LoadSkew = loadSkew
	decision.SelectedGroup = selectedGroup
	decision.setProjectionMetadataFromView(projectionView)
	if err != nil {
		return nil, decision, err
	}
	if selection != nil && selection.Account != nil {
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
	}
	return selection, decision, nil
}

func (s *defaultOpenAIAccountScheduler) selectBySessionHash(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	sticky *openAIStickyEval,
) (*AccountSelectionResult, error) {
	sessionHash := strings.TrimSpace(req.SessionHash)
	if sessionHash == "" {
		if sticky != nil {
			if sticky.SessionSource == openAIStickySessionSourceNone {
				sticky.setEvalResult(openAIStickyEvalResultNoSessionSignal)
			} else {
				sticky.setEvalResult(openAIStickyEvalResultMissNoBinding)
			}
		}
		return nil, nil
	}
	if s == nil || s.service == nil {
		if sticky != nil {
			sticky.setEvalResult(openAIStickyEvalResultMissNoBinding)
		}
		return nil, nil
	}

	affinityBinding := s.service.getOpenAIStickyAffinityBinding(ctx, req.GroupID, sessionHash)
	accountID := req.StickyAccountID
	if accountID <= 0 {
		var err error
		accountID, err = s.service.getStickySessionAccountID(ctx, req.GroupID, sessionHash)
		if (err != nil || accountID <= 0) && affinityBinding != nil {
			accountID = affinityBinding.BoundAccountID
			err = nil
		}
		if err != nil || accountID <= 0 {
			if sticky != nil {
				sticky.setEvalResult(openAIStickyEvalResultMissNoBinding)
			}
			return nil, nil
		}
	}
	if affinityBinding != nil && affinityBinding.BoundAccountID > 0 && affinityBinding.BoundAccountID != accountID {
		_ = s.service.deleteOpenAIStickyAffinityBinding(ctx, req.GroupID, sessionHash)
		affinityBinding = nil
	}
	if sticky != nil {
		sticky.setBoundAccountID(accountID)
		sticky.setAffinityBinding(affinityBinding)
	}
	if accountID <= 0 {
		if sticky != nil {
			sticky.setEvalResult(openAIStickyEvalResultMissNoBinding)
		}
		return nil, nil
	}
	if req.ExcludedIDs != nil {
		if _, excluded := req.ExcludedIDs[accountID]; excluded {
			if sticky != nil {
				sticky.setEvalResult(openAIStickyEvalResultMissBindingExcluded)
			}
			return nil, nil
		}
	}

	account, err := s.service.getSchedulableAccount(ctx, accountID)
	if err != nil || account == nil {
		if sticky != nil {
			sticky.setEvalResult(openAIStickyEvalResultMissBindingInvalid)
		}
		_ = s.service.clearOpenAIStickySessionBindings(ctx, req.GroupID, sessionHash)
		return nil, nil
	}
	if !account.IsOpenAI() {
		if sticky != nil {
			sticky.setEvalResult(openAIStickyEvalResultMissBindingInvalid)
		}
		_ = s.service.clearOpenAIStickySessionBindings(ctx, req.GroupID, sessionHash)
		return nil, nil
	}
	var projectionView *openAIProjectionViewResult
	selectedGroup := resolveOpenAISelectedGroupFromBindingOrAccount(affinityBinding, account)
	legacyReserveAffinityHit := false
	if req.RequestedModel != "" && s.service != nil && s.service.schedulerSnapshot != nil {
		projectionView, _, err = s.service.getOpenAIProjectionViewWithLegacyFallback(ctx, req.GroupID, req.RequestedModel)
		if err != nil {
			return nil, err
		}
	}
	if projectionView != nil {
		if affinityBinding != nil && !projectionView.bindingMatches(affinityBinding) {
			_ = s.service.deleteOpenAIStickyAffinityBinding(ctx, req.GroupID, sessionHash)
			affinityBinding = nil
			if sticky != nil {
				sticky.setAffinityBinding(nil)
			}
		}
		selectedGroup = projectionView.selectedGroupForTarget(account.ID, req.TargetGroup)
	}
	if isOpenAIReserveAffinityBinding(affinityBinding) && normalizeTargetGroup(req.TargetGroup) != TargetGroupExhausted {
		if sticky != nil {
			sticky.setEvalResult(openAIStickyEvalResultMissBindingInvalid)
		}
		_ = s.service.clearOpenAIStickySessionBindings(ctx, req.GroupID, sessionHash)
		return nil, nil
	}
	if projectionView != nil {
		switch normalizeTargetGroup(req.TargetGroup) {
		case TargetGroupExhausted:
			if selectedGroup != openAISelectedGroupReserve && selectedGroup != string(TargetGroupExhausted) {
				if sticky != nil {
					sticky.setEvalResult(openAIStickyEvalResultMissBindingInvalid)
				}
				_ = s.service.clearOpenAIStickySessionBindings(ctx, req.GroupID, sessionHash)
				return nil, nil
			}
		default:
			if selectedGroup == openAISelectedGroupReserve {
				if sticky != nil {
					sticky.setEvalResult(openAIStickyEvalResultMissBindingInvalid)
				}
				_ = s.service.clearOpenAIStickySessionBindings(ctx, req.GroupID, sessionHash)
				return nil, nil
			}
		}
	} else {
		reserveAffinityBinding := isOpenAIReserveAffinityBinding(affinityBinding)
		legacyReserveAffinityHit = reserveAffinityBinding && account.IsOpenAIReserveCandidate()
		if reserveAffinityBinding && !account.IsOpenAIReserveCandidate() && !account.MatchesTargetGroup(TargetGroupExhausted) {
			if sticky != nil {
				sticky.setEvalResult(openAIStickyEvalResultMissBindingInvalid)
			}
			_ = s.service.clearOpenAIStickySessionBindings(ctx, req.GroupID, sessionHash)
			return nil, nil
		}
		if (req.TargetGroup == TargetGroupAny || req.TargetGroup == TargetGroupActive) && s.service.isCurrentOpenAIReserveOverlayAccount(ctx, req.GroupID, req.RequestedModel, account) {
			if sticky != nil {
				sticky.setEvalResult(openAIStickyEvalResultMissBindingInvalid)
			}
			_ = s.service.clearOpenAIStickySessionBindings(ctx, req.GroupID, sessionHash)
			return nil, nil
		}
		if !account.MatchesTargetGroup(req.TargetGroup) && !legacyReserveAffinityHit {
			if sticky != nil {
				sticky.setEvalResult(openAIStickyEvalResultMissBindingRestricted)
			}
			return nil, nil
		}
	}
	if shouldClearStickySessionForTargetGroup(account, req.RequestedModel, req.TargetGroup) {
		if sticky != nil {
			sticky.setEvalResult(openAIStickyEvalResultMissBindingInvalid)
		}
		_ = s.service.clearOpenAIStickySessionBindings(ctx, req.GroupID, sessionHash)
		return nil, nil
	}
	if projectionView == nil && !account.IsSchedulableForTargetGroup(req.TargetGroup) && !legacyReserveAffinityHit {
		if sticky != nil {
			sticky.setEvalResult(openAIStickyEvalResultMissBindingInvalid)
		}
		_ = s.service.clearOpenAIStickySessionBindings(ctx, req.GroupID, sessionHash)
		return nil, nil
	}
	if req.RequestedModel != "" && (projectionView == nil || selectedGroup == string(TargetGroupActive)) && !account.IsModelSupported(req.RequestedModel) {
		if sticky != nil {
			sticky.setEvalResult(openAIStickyEvalResultMissBindingRestricted)
		}
		return nil, nil
	}
	if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
		if sticky != nil {
			sticky.setEvalResult(openAIStickyEvalResultMissBindingInvalid)
		}
		_ = s.service.clearOpenAIStickySessionBindings(ctx, req.GroupID, sessionHash)
		return nil, nil
	}
	account = s.service.recheckSelectedProjectedOpenAIAccountFromDB(ctx, req.GroupID, account, req.RequestedModel, selectedGroup, req.TargetGroup)
	if account == nil {
		if sticky != nil {
			sticky.setEvalResult(openAIStickyEvalResultMissBindingInvalid)
		}
		_ = s.service.clearOpenAIStickySessionBindings(ctx, req.GroupID, sessionHash)
		return nil, nil
	}

	result, acquireErr := s.service.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
	if acquireErr == nil && result.Acquired {
		if sticky != nil {
			sticky.setEvalResult(openAIStickyEvalResultHit)
		}
		if req.SessionHash != "" {
			binding := newOpenAIAffinityBinding(account.ID, selectedGroup)
			if projectionView != nil {
				binding = projectionView.newAffinityBinding(account.ID, selectedGroup)
			}
			if sticky != nil {
				sticky.setAffinityBinding(binding)
			}
			_ = s.service.bindOpenAIStickySessionAffinity(ctx, req.GroupID, sessionHash, binding)
		}
		return &AccountSelectionResult{
			Account:       account,
			Acquired:      true,
			ReleaseFunc:   result.ReleaseFunc,
			SelectedGroup: selectedGroup,
		}, nil
	}

	cfg := s.service.schedulingConfig()
	// WaitPlan.MaxConcurrency 使用 Concurrency（非 EffectiveLoadFactor），因为 WaitPlan 控制的是 Redis 实际并发槽位等待。
	if s.service.concurrencyService != nil {
		if sticky != nil {
			sticky.setEvalResult(openAIStickyEvalResultHit)
			binding := newOpenAIAffinityBinding(account.ID, selectedGroup)
			if projectionView != nil {
				binding = projectionView.newAffinityBinding(account.ID, selectedGroup)
			}
			sticky.setAffinityBinding(binding)
		}
		return &AccountSelectionResult{
			Account: account,
			WaitPlan: &AccountWaitPlan{
				AccountID:      accountID,
				MaxConcurrency: account.Concurrency,
				Timeout:        cfg.StickySessionWaitTimeout,
				MaxWaiting:     cfg.StickySessionMaxWaiting,
			},
			SelectedGroup: selectedGroup,
		}, nil
	}
	if sticky != nil && sticky.EvalResult == "" {
		sticky.setEvalResult(openAIStickyEvalResultMissBindingInvalid)
	}
	return nil, nil
}

type openAIAccountCandidateScore struct {
	account       *Account
	loadInfo      *AccountLoadInfo
	score         float64
	errorRate     float64
	ttft          float64
	hasTTFT       bool
	selectedGroup string
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
	if left.score != right.score {
		return left.score > right.score
	}
	if left.account.Priority != right.account.Priority {
		return left.account.Priority < right.account.Priority
	}
	if left.loadInfo.LoadRate != right.loadInfo.LoadRate {
		return left.loadInfo.LoadRate < right.loadInfo.LoadRate
	}
	if left.loadInfo.WaitingCount != right.loadInfo.WaitingCount {
		return left.loadInfo.WaitingCount < right.loadInfo.WaitingCount
	}
	return left.account.ID < right.account.ID
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
		sort.Slice(ranked, func(i, j int) bool {
			return isOpenAIAccountCandidateBetter(ranked[i], ranked[j])
		})
		return ranked
	}

	best := make(openAIAccountCandidateHeap, 0, topK)
	for _, candidate := range candidates {
		if len(best) < topK {
			heap.Push(&best, candidate)
			continue
		}
		if isOpenAIAccountCandidateBetter(candidate, best[0]) {
			best[0] = candidate
			heap.Fix(&best, 0)
		}
	}

	ranked := make([]openAIAccountCandidateScore, len(best))
	copy(ranked, best)
	sort.Slice(ranked, func(i, j int) bool {
		return isOpenAIAccountCandidateBetter(ranked[i], ranked[j])
	})
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
	// 对“无会话锚点”的纯负载均衡请求引入时间熵，避免固定命中同一账号。
	if strings.TrimSpace(req.SessionHash) == "" && strings.TrimSpace(req.PreviousResponseID) == "" {
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
	if len(candidates) <= 1 {
		return append([]openAIAccountCandidateScore(nil), candidates...)
	}

	pool := append([]openAIAccountCandidateScore(nil), candidates...)
	weights := make([]float64, len(pool))
	minScore := pool[0].score
	for i := 1; i < len(pool); i++ {
		if pool[i].score < minScore {
			minScore = pool[i].score
		}
	}
	for i := range pool {
		// 将 top-K 分值平移到正区间，避免“单一最高分账号”长期垄断。
		weight := (pool[i].score - minScore) + 1.0
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 {
			weight = 1.0
		}
		weights[i] = weight
	}

	order := make([]openAIAccountCandidateScore, 0, len(pool))
	rng := newOpenAISelectionRNG(deriveOpenAISelectionSeed(req))
	for len(pool) > 0 {
		total := 0.0
		for _, w := range weights {
			total += w
		}

		selectedIdx := 0
		if total > 0 {
			r := rng.nextFloat64() * total
			acc := 0.0
			for i, w := range weights {
				acc += w
				if r <= acc {
					selectedIdx = i
					break
				}
			}
		} else {
			selectedIdx = int(rng.nextUint64() % uint64(len(pool)))
		}

		order = append(order, pool[selectedIdx])
		pool = append(pool[:selectedIdx], pool[selectedIdx+1:]...)
		weights = append(weights[:selectedIdx], weights[selectedIdx+1:]...)
	}
	return order
}

func (s *defaultOpenAIAccountScheduler) selectByLoadBalance(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, string, int, int, float64, *openAIProjectionViewResult, error) {
	var (
		accounts        []Account
		reserveAccounts []Account
		projectionView  *openAIProjectionViewResult
		err             error
	)
	allowLegacyFallback := true
	if req.RequestedModel != "" && s.service != nil && s.service.schedulerSnapshot != nil {
		projectionView, allowLegacyFallback, err = s.service.getOpenAIProjectionViewWithLegacyFallback(ctx, req.GroupID, req.RequestedModel)
		if err != nil {
			return nil, "", 0, 0, 0, nil, err
		}
	}
	if req.TargetGroup == TargetGroupExhausted {
		if projectionView != nil {
			accounts, reserveAccounts, err = s.service.listOpenAIExhaustedWithReserveOverlay(ctx, req.GroupID, req.RequestedModel)
		} else if allowLegacyFallback && req.RequestedModel != "" && s.service != nil && s.service.schedulerSnapshot != nil {
			accounts, err = s.service.listSchedulableAccounts(ctx, req.GroupID, req.TargetGroup)
			if err == nil {
				reserveAccounts, err = s.service.listCurrentOpenAILegacyReserveOverlay(ctx, req.GroupID, req.RequestedModel)
			}
		} else {
			accounts, reserveAccounts, err = s.service.listOpenAIExhaustedWithReserveOverlay(ctx, req.GroupID, req.RequestedModel)
		}
	} else {
		accounts, err = s.service.listSchedulableAccounts(ctx, req.GroupID, req.TargetGroup)
		if err == nil && (req.TargetGroup == TargetGroupAny || req.TargetGroup == TargetGroupActive) {
			if projectionView != nil {
				_, reserveAccounts, err = s.service.listOpenAIExhaustedWithReserveOverlay(ctx, req.GroupID, req.RequestedModel)
			} else {
				reserveAccounts, err = s.service.listCurrentOpenAILegacyReserveOverlay(ctx, req.GroupID, req.RequestedModel)
			}
		}
	}
	if err != nil {
		return nil, "", 0, 0, 0, projectionView, err
	}
	if len(accounts) == 0 && !(req.TargetGroup == TargetGroupExhausted && len(reserveAccounts) > 0) {
		return nil, "", 0, 0, 0, projectionView, errors.New("no available OpenAI accounts")
	}

	// require_privacy_set: 获取分组信息
	var schedGroup *Group
	if req.GroupID != nil && s.service.schedulerSnapshot != nil {
		schedGroup, _ = s.service.schedulerSnapshot.GetGroupByID(ctx, *req.GroupID)
	}

	filtered := make([]*Account, 0, len(accounts))
	filteredValues := make([]Account, 0, len(accounts))
	reserveFiltered := make([]*Account, 0, len(reserveAccounts))
	reserveFilteredValues := make([]Account, 0, len(reserveAccounts))
	loadReq := make([]AccountWithConcurrency, 0, len(accounts)+len(reserveAccounts))
	reserveOverlayIDs := make(map[int64]struct{}, len(reserveAccounts))
	for _, account := range reserveAccounts {
		reserveOverlayIDs[account.ID] = struct{}{}
	}
	canUseReserveForTarget := projectionView != nil && (req.TargetGroup == TargetGroupAny || req.TargetGroup == TargetGroupActive)
	rebuildLoadReq := func(primary []Account, reserve []Account) []AccountWithConcurrency {
		rebuilt := make([]AccountWithConcurrency, 0, len(primary)+len(reserve))
		for _, account := range primary {
			rebuilt = append(rebuilt, AccountWithConcurrency{ID: account.ID, MaxConcurrency: account.EffectiveLoadFactor()})
		}
		for _, account := range reserve {
			rebuilt = append(rebuilt, AccountWithConcurrency{ID: account.ID, MaxConcurrency: account.EffectiveLoadFactor()})
		}
		return rebuilt
	}
	appendFilteredAccount := func(account *Account, selectedGroup string, out *[]*Account, outValues *[]Account) {
		if account == nil {
			return
		}
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[account.ID]; excluded {
				return
			}
		}
		if !account.IsOpenAI() {
			return
		}
		if selectedGroup == openAISelectedGroupReserve {
			if projectionView == nil && !account.IsOpenAIReserveCandidate() {
				return
			}
		} else {
			if req.TargetGroup == TargetGroupAny || req.TargetGroup == TargetGroupActive {
				if _, reserved := reserveOverlayIDs[account.ID]; reserved {
					return
				}
			}
			if !account.MatchesTargetGroup(req.TargetGroup) {
				return
			}
			if !account.IsSchedulableForTargetGroup(req.TargetGroup) {
				return
			}
		}
		if schedGroup != nil && schedGroup.RequirePrivacySet && !account.IsPrivacySet() {
			_ = s.service.accountRepo.SetError(ctx, account.ID,
				fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
			return
		}
		if req.RequestedModel != "" && !account.IsModelSupported(req.RequestedModel) {
			return
		}
		if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
			return
		}
		*out = append(*out, account)
		*outValues = append(*outValues, *account)
		loadReq = append(loadReq, AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: account.EffectiveLoadFactor(),
		})
	}
	for i := range accounts {
		appendFilteredAccount(&accounts[i], string(TargetGroupExhausted), &filtered, &filteredValues)
	}
	for i := range reserveAccounts {
		appendFilteredAccount(&reserveAccounts[i], openAISelectedGroupReserve, &reserveFiltered, &reserveFilteredValues)
	}
	if (req.TargetGroup == TargetGroupAny || req.TargetGroup == TargetGroupActive) && projectionView != nil && len(projectionView.view.ExhaustedBaseIDs) == 0 && len(filteredValues) > 0 {
		hasNonReserveActive := false
		for _, account := range filteredValues {
			if _, reserved := reserveOverlayIDs[account.ID]; !reserved {
				hasNonReserveActive = true
				break
			}
		}
		if hasNonReserveActive {
			pruned := filtered[:0]
			prunedValues := filteredValues[:0]
			for i, account := range filteredValues {
				if _, reserved := reserveOverlayIDs[account.ID]; reserved {
					continue
				}
				pruned = append(pruned, filtered[i])
				prunedValues = append(prunedValues, account)
			}
			filtered = pruned
			filteredValues = prunedValues
			loadReq = rebuildLoadReq(filteredValues, reserveFilteredValues)
		}
	}
	if len(filtered) == 0 && !(req.TargetGroup == TargetGroupExhausted && len(reserveFiltered) > 0) && !(canUseReserveForTarget && len(reserveFiltered) > 0) {
		return nil, "", 0, 0, 0, projectionView, errors.New("no available OpenAI accounts")
	}

	loadMap := map[int64]*AccountLoadInfo{}
	if s.service.concurrencyService != nil {
		if batchLoad, loadErr := s.service.concurrencyService.GetAccountsLoadBatch(ctx, loadReq); loadErr == nil {
			loadMap = batchLoad
		}
	}

	participantGroups := make(map[int64]string, len(filtered)+len(reserveFiltered))
	participants := filtered
	fallbackSelectedGroup := ""
	if req.TargetGroup == TargetGroupExhausted && shouldRouteExhaustedOverflowToReserve(filteredValues, reserveFilteredValues, loadMap) {
		reserveUsage := calculateOpenAIConcurrentUsagePercent(reserveFilteredValues, loadMap)
		if reserveUsage < 60 {
			participants = reserveFiltered
			fallbackSelectedGroup = openAISelectedGroupReserve
			participantGroups = make(map[int64]string, len(reserveFiltered))
			for _, account := range reserveFiltered {
				participantGroups[account.ID] = openAISelectedGroupReserve
			}
		} else {
			participants = append(append([]*Account{}, filtered...), reserveFiltered...)
			for _, account := range filtered {
				participantGroups[account.ID] = string(TargetGroupExhausted)
			}
			for _, account := range reserveFiltered {
				participantGroups[account.ID] = openAISelectedGroupReserve
			}
		}
	}
	if canUseReserveForTarget && len(reserveFiltered) > 0 {
		participants = append(append([]*Account{}, filtered...), reserveFiltered...)
		participantGroups = make(map[int64]string, len(participants))
		for _, account := range filtered {
			participantGroups[account.ID] = resolveOpenAISelectedGroupFromAccount(account)
		}
		for _, account := range reserveFiltered {
			participantGroups[account.ID] = openAISelectedGroupReserve
		}
	}
	if len(participantGroups) == 0 {
		for _, account := range participants {
			participantGroups[account.ID] = resolveOpenAISelectedGroupFromAccount(account)
		}
	}
	if len(participants) == 0 {
		return nil, "", 0, 0, 0, projectionView, errors.New("no available OpenAI accounts")
	}

	minPriority, maxPriority := participants[0].Priority, participants[0].Priority
	maxWaiting := 1
	loadRateSum := 0.0
	loadRateSumSquares := 0.0
	minTTFT, maxTTFT := 0.0, 0.0
	hasTTFTSample := false
	candidates := make([]openAIAccountCandidateScore, 0, len(participants))
	for _, account := range participants {
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		if account.Priority < minPriority {
			minPriority = account.Priority
		}
		if account.Priority > maxPriority {
			maxPriority = account.Priority
		}
		if loadInfo.WaitingCount > maxWaiting {
			maxWaiting = loadInfo.WaitingCount
		}
		errorRate, ttft, hasTTFT := s.stats.snapshot(account.ID)
		if hasTTFT && ttft > 0 {
			if !hasTTFTSample {
				minTTFT, maxTTFT = ttft, ttft
				hasTTFTSample = true
			} else {
				if ttft < minTTFT {
					minTTFT = ttft
				}
				if ttft > maxTTFT {
					maxTTFT = ttft
				}
			}
		}
		loadRate := float64(loadInfo.LoadRate)
		loadRateSum += loadRate
		loadRateSumSquares += loadRate * loadRate
		candidates = append(candidates, openAIAccountCandidateScore{
			account:       account,
			loadInfo:      loadInfo,
			errorRate:     errorRate,
			ttft:          ttft,
			hasTTFT:       hasTTFT,
			selectedGroup: participantGroups[account.ID],
		})
	}
	loadSkew := calcLoadSkewByMoments(loadRateSum, loadRateSumSquares, len(candidates))

	weights := s.service.openAIWSSchedulerWeights()
	for i := range candidates {
		item := &candidates[i]
		priorityFactor := 1.0
		if maxPriority > minPriority {
			priorityFactor = 1 - float64(item.account.Priority-minPriority)/float64(maxPriority-minPriority)
		}
		loadFactor := 1 - clamp01(float64(item.loadInfo.LoadRate)/100.0)
		queueFactor := 1 - clamp01(float64(item.loadInfo.WaitingCount)/float64(maxWaiting))
		errorFactor := 1 - clamp01(item.errorRate)
		ttftFactor := 0.5
		if item.hasTTFT && hasTTFTSample && maxTTFT > minTTFT {
			ttftFactor = 1 - clamp01((item.ttft-minTTFT)/(maxTTFT-minTTFT))
		}

		item.score = weights.Priority*priorityFactor +
			weights.Load*loadFactor +
			weights.Queue*queueFactor +
			weights.ErrorRate*errorFactor +
			weights.TTFT*ttftFactor
	}

	topK := s.service.openAIWSLBTopK()
	if topK > len(candidates) {
		topK = len(candidates)
	}
	if topK <= 0 {
		topK = 1
	}
	rankedCandidates := selectTopKOpenAICandidates(candidates, topK)
	selectionOrder := buildOpenAIWeightedSelectionOrder(rankedCandidates, req)
	prepareCandidate := func(candidate openAIAccountCandidateScore) *Account {
		fresh := s.service.resolveFreshProjectedOpenAIAccount(ctx, req.GroupID, candidate.account, req.RequestedModel, candidate.selectedGroup, req.TargetGroup)
		if fresh == nil || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) {
			return nil
		}
		fresh = s.service.recheckSelectedProjectedOpenAIAccountFromDB(ctx, req.GroupID, fresh, req.RequestedModel, candidate.selectedGroup, req.TargetGroup)
		if fresh == nil || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) {
			return nil
		}
		return fresh
	}

	for i := 0; i < len(selectionOrder); i++ {
		candidate := selectionOrder[i]
		fresh := prepareCandidate(candidate)
		if fresh == nil {
			continue
		}
		result, acquireErr := s.service.tryAcquireAccountSlot(ctx, fresh.ID, fresh.Concurrency)
		if acquireErr != nil {
			return nil, fallbackSelectedGroup, len(candidates), topK, loadSkew, projectionView, acquireErr
		}
		if result != nil && result.Acquired {
			if req.SessionHash != "" {
				binding := newOpenAIAffinityBinding(fresh.ID, candidate.selectedGroup)
				if projectionView != nil {
					binding = projectionView.newAffinityBinding(fresh.ID, candidate.selectedGroup)
				}
				_ = s.service.bindOpenAIStickySessionAffinity(ctx, req.GroupID, req.SessionHash, binding)
			}
			return &AccountSelectionResult{
				Account:       fresh,
				Acquired:      true,
				ReleaseFunc:   result.ReleaseFunc,
				SelectedGroup: candidate.selectedGroup,
			}, candidate.selectedGroup, len(candidates), topK, loadSkew, projectionView, nil
		}
	}

	cfg := s.service.schedulingConfig()
	// WaitPlan.MaxConcurrency 使用 Concurrency（非 EffectiveLoadFactor），因为 WaitPlan 控制的是 Redis 实际并发槽位等待。
	for _, candidate := range selectionOrder {
		fresh := prepareCandidate(candidate)
		if fresh == nil {
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
			SelectedGroup: candidate.selectedGroup,
		}, candidate.selectedGroup, len(candidates), topK, loadSkew, projectionView, nil
	}

	return nil, fallbackSelectedGroup, len(candidates), topK, loadSkew, projectionView, ErrNoAvailableAccounts
}

func resolveOpenAISelectedGroupFromAccount(account *Account) string {
	if account == nil {
		return ""
	}
	if account.MatchesTargetGroup(TargetGroupExhausted) {
		return string(TargetGroupExhausted)
	}
	return string(TargetGroupActive)
}

func shouldBindOpenAISharedSticky(selectedGroup string) bool {
	return strings.TrimSpace(selectedGroup) != openAISelectedGroupReserve
}

func (s *defaultOpenAIAccountScheduler) isAccountTransportCompatible(account *Account, requiredTransport OpenAIUpstreamTransport) bool {
	// HTTP 入站可回退到 HTTP 线路，不需要在账号选择阶段做传输协议强过滤。
	if requiredTransport == OpenAIUpstreamTransportAny || requiredTransport == OpenAIUpstreamTransportHTTPSSE {
		return true
	}
	if s == nil || s.service == nil || account == nil {
		return false
	}
	return s.service.getOpenAIWSProtocolResolver().Resolve(account).Transport == requiredTransport
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
		snapshot.StickyHitRatio = float64(prevHit+sessionHit) / float64(selectTotal)
		snapshot.AccountSwitchRate = float64(switchTotal) / float64(selectTotal)
		snapshot.LoadSkewAvg = float64(loadSkewTotal) / 1000 / float64(selectTotal)
	}
	return snapshot
}

func (s *OpenAIGatewayService) getOpenAIAccountScheduler() OpenAIAccountScheduler {
	if s == nil {
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

func (s *OpenAIGatewayService) SelectAccountWithScheduler(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	targetGroup AccountTargetGroup,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	return s.SelectAccountWithSchedulerRequest(ctx, OpenAIAccountScheduleRequest{
		GroupID:              groupID,
		TargetGroup:          targetGroup,
		SessionHash:          sessionHash,
		PreviousResponseID:   previousResponseID,
		RequestedModel:       requestedModel,
		RequiredTransport:    requiredTransport,
		ExcludedIDs:          excludedIDs,
		ParentSessionPresent: strings.TrimSpace(previousResponseID) != "",
		ParentSessionKey:     strings.TrimSpace(previousResponseID),
	})
}

func (s *OpenAIGatewayService) SelectAccountWithSchedulerRequest(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{Sticky: newOpenAIStickyEval(req)}
	req.TargetGroup = normalizeTargetGroup(req.TargetGroup)
	req.SessionHash = strings.TrimSpace(req.SessionHash)
	req.SessionSource = normalizeOpenAIStickySessionSource(req.SessionSource)
	req.PreviousResponseID = strings.TrimSpace(req.PreviousResponseID)
	req.ParentSessionKey = strings.TrimSpace(req.ParentSessionKey)
	if req.ParentSessionKey != "" {
		req.ParentSessionPresent = true
	}
	scheduler := s.getOpenAIAccountScheduler()
	if scheduler == nil {
		selection, err := s.SelectAccountWithLoadAwareness(ctx, req.GroupID, req.SessionHash, req.RequestedModel, req.ExcludedIDs)
		decision.Layer = openAIAccountScheduleLayerLoadBalance
		if decision.Sticky != nil {
			if decision.Sticky.EvalResult == "" {
				if decision.Sticky.SessionHashPresent {
					decision.Sticky.setEvalResult(openAIStickyEvalResultMissNoBinding)
				} else {
					decision.Sticky.setEvalResult(openAIStickyEvalResultNoSessionSignal)
				}
			}
			if selection != nil && selection.Account != nil {
				decision.SelectedAccountID = selection.Account.ID
				decision.SelectedAccountType = selection.Account.Type
				decision.SelectedGroup = resolveOpenAISelectedGroupFromAccount(selection.Account)
			}
			decision.Sticky.finalizeSelectedAccount(decision.SelectedAccountID)
		}
		return selection, decision, err
	}

	var stickyAccountID int64
	if req.StickyAccountID > 0 {
		stickyAccountID = req.StickyAccountID
	}
	if stickyAccountID <= 0 && req.SessionHash != "" && s.cache != nil {
		if accountID, err := s.getStickySessionAccountID(ctx, req.GroupID, req.SessionHash); err == nil && accountID > 0 {
			stickyAccountID = accountID
		}
	}
	req.StickyAccountID = stickyAccountID
	if decision.Sticky != nil {
		decision.Sticky.setBoundAccountID(stickyAccountID)
	}

	return scheduler.Select(ctx, req)
}

func (s *OpenAIGatewayService) ReportOpenAIAccountScheduleResult(accountID int64, success bool, firstTokenMs *int) {
	scheduler := s.getOpenAIAccountScheduler()
	if scheduler == nil {
		return
	}
	scheduler.ReportResult(accountID, success, firstTokenMs)
}

func (s *OpenAIGatewayService) RecordOpenAIAccountSwitch() {
	scheduler := s.getOpenAIAccountScheduler()
	if scheduler == nil {
		return
	}
	scheduler.ReportSwitch()
}

func (s *OpenAIGatewayService) SnapshotOpenAIAccountSchedulerMetrics() OpenAIAccountSchedulerMetricsSnapshot {
	scheduler := s.getOpenAIAccountScheduler()
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

func (s *OpenAIGatewayService) openAIWSSchedulerWeights() GatewayOpenAIWSSchedulerScoreWeightsView {
	if s != nil && s.cfg != nil {
		return GatewayOpenAIWSSchedulerScoreWeightsView{
			Priority:  s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority,
			Load:      s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load,
			Queue:     s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue,
			ErrorRate: s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate,
			TTFT:      s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT,
		}
	}
	return GatewayOpenAIWSSchedulerScoreWeightsView{
		Priority:  1.0,
		Load:      1.0,
		Queue:     0.7,
		ErrorRate: 0.8,
		TTFT:      0.5,
	}
}

type GatewayOpenAIWSSchedulerScoreWeightsView struct {
	Priority  float64
	Load      float64
	Queue     float64
	ErrorRate float64
	TTFT      float64
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

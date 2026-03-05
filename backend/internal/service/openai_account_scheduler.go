package service

import (
	"container/heap"
	"context"
	"errors"
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
)

type OpenAIAccountScheduleRequest struct {
	GroupID            *int64
	SessionHash        string
	StickyAccountID    int64
	PreviousResponseID string
	RequestedModel     string
	RequiredTransport  OpenAIUpstreamTransport
	ExcludedIDs        map[int64]struct{REDACTED
REDACTED

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
REDACTED

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
REDACTED

type OpenAIAccountScheduler interface {
	Select(ctx context.Context, req OpenAIAccountScheduleRequest) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error)
	ReportResult(accountID int64, success bool, firstTokenMs *int)
	ReportSwitch()
	SnapshotMetrics() OpenAIAccountSchedulerMetricsSnapshot
REDACTED

type openAIAccountSchedulerMetrics struct {
	selectTotal            atomic.Int64
	stickyPreviousHitTotal atomic.Int64
	stickySessionHitTotal  atomic.Int64
	loadBalanceSelectTotal atomic.Int64
	accountSwitchTotal     atomic.Int64
	latencyMsTotal         atomic.Int64
	loadSkewMilliTotal     atomic.Int64
REDACTED

func (m *openAIAccountSchedulerMetrics) recordSelect(decision OpenAIAccountScheduleDecision) {
	if m == nil {
		return
REDACTED
	m.selectTotal.Add(1)
	m.latencyMsTotal.Add(decision.LatencyMs)
	m.loadSkewMilliTotal.Add(int64(math.Round(decision.LoadSkew * 1000)))
	if decision.StickyPreviousHit {
		m.stickyPreviousHitTotal.Add(1)
REDACTED
	if decision.StickySessionHit {
		m.stickySessionHitTotal.Add(1)
REDACTED
	if decision.Layer == openAIAccountScheduleLayerLoadBalance {
		m.loadBalanceSelectTotal.Add(1)
REDACTED
REDACTED

func (m *openAIAccountSchedulerMetrics) recordSwitch() {
	if m == nil {
		return
REDACTED
	m.accountSwitchTotal.Add(1)
REDACTED

type openAIAccountRuntimeStats struct {
	accounts     sync.Map
	accountCount atomic.Int64
REDACTED

type openAIAccountRuntimeStat struct {
	errorRateEWMABits atomic.Uint64
	ttftEWMABits      atomic.Uint64
REDACTED

func newOpenAIAccountRuntimeStats() *openAIAccountRuntimeStats {
	return &openAIAccountRuntimeStats{REDACTED
REDACTED

func (s *openAIAccountRuntimeStats) loadOrCreate(accountID int64) *openAIAccountRuntimeStat {
	if value, ok := s.accounts.Load(accountID); ok {
		stat, _ := value.(*openAIAccountRuntimeStat)
		if stat != nil {
			return stat
	REDACTED
REDACTED

	stat := &openAIAccountRuntimeStat{REDACTED
	stat.ttftEWMABits.Store(math.Float64bits(math.NaN()))
	actual, loaded := s.accounts.LoadOrStore(accountID, stat)
	if !loaded {
		s.accountCount.Add(1)
		return stat
REDACTED
	existing, _ := actual.(*openAIAccountRuntimeStat)
	if existing != nil {
		return existing
REDACTED
	return stat
REDACTED

func updateEWMAAtomic(target *atomic.Uint64, sample float64, alpha float64) {
	for {
		oldBits := target.Load()
		oldValue := math.Float64frombits(oldBits)
		newValue := alpha*sample + (1-alpha)*oldValue
		if target.CompareAndSwap(oldBits, math.Float64bits(newValue)) {
			return
	REDACTED
REDACTED
REDACTED

func (s *openAIAccountRuntimeStats) report(accountID int64, success bool, firstTokenMs *int) {
	if s == nil || accountID <= 0 {
		return
REDACTED
	const alpha = 0.2
	stat := s.loadOrCreate(accountID)

	errorSample := 1.0
	if success {
		errorSample = 0.0
REDACTED
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
			REDACTED
				continue
		REDACTED
			newValue := alpha*ttft + (1-alpha)*oldValue
			if stat.ttftEWMABits.CompareAndSwap(oldBits, math.Float64bits(newValue)) {
				break
		REDACTED
	REDACTED
REDACTED
REDACTED

func (s *openAIAccountRuntimeStats) snapshot(accountID int64) (errorRate float64, ttft float64, hasTTFT bool) {
	if s == nil || accountID <= 0 {
		return 0, 0, false
REDACTED
	value, ok := s.accounts.Load(accountID)
	if !ok {
		return 0, 0, false
REDACTED
	stat, _ := value.(*openAIAccountRuntimeStat)
	if stat == nil {
		return 0, 0, false
REDACTED
	errorRate = clamp01(math.Float64frombits(stat.errorRateEWMABits.Load()))
	ttftValue := math.Float64frombits(stat.ttftEWMABits.Load())
	if math.IsNaN(ttftValue) {
		return errorRate, 0, false
REDACTED
	return errorRate, ttftValue, true
REDACTED

func (s *openAIAccountRuntimeStats) size() int {
	if s == nil {
		return 0
REDACTED
	return int(s.accountCount.Load())
REDACTED

type defaultOpenAIAccountScheduler struct {
	service *OpenAIGatewayService
	metrics openAIAccountSchedulerMetrics
	stats   *openAIAccountRuntimeStats
REDACTED

func newDefaultOpenAIAccountScheduler(service *OpenAIGatewayService, stats *openAIAccountRuntimeStats) OpenAIAccountScheduler {
	if stats == nil {
		stats = newOpenAIAccountRuntimeStats()
REDACTED
	return &defaultOpenAIAccountScheduler{
		service: service,
		stats:   stats,
REDACTED
REDACTED

func (s *defaultOpenAIAccountScheduler) Select(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{REDACTED
	start := time.Now()
	defer func() {
		decision.LatencyMs = time.Since(start).Milliseconds()
		s.metrics.recordSelect(decision)
REDACTED()

	previousResponseID := strings.TrimSpace(req.PreviousResponseID)
	if previousResponseID != "" {
		selection, err := s.service.SelectAccountByPreviousResponseID(
			ctx,
			req.GroupID,
			previousResponseID,
			req.RequestedModel,
			req.ExcludedIDs,
		)
		if err != nil {
			return nil, decision, err
	REDACTED
		if selection != nil && selection.Account != nil {
			if !s.isAccountTransportCompatible(selection.Account, req.RequiredTransport) {
				selection = nil
		REDACTED
	REDACTED
		if selection != nil && selection.Account != nil {
			decision.Layer = openAIAccountScheduleLayerPreviousResponse
			decision.StickyPreviousHit = true
			decision.SelectedAccountID = selection.Account.ID
			decision.SelectedAccountType = selection.Account.Type
			if req.SessionHash != "" {
				_ = s.service.BindStickySession(ctx, req.GroupID, req.SessionHash, selection.Account.ID)
		REDACTED
			return selection, decision, nil
	REDACTED
REDACTED

	selection, err := s.selectBySessionHash(ctx, req)
	if err != nil {
		return nil, decision, err
REDACTED
	if selection != nil && selection.Account != nil {
		decision.Layer = openAIAccountScheduleLayerSessionSticky
		decision.StickySessionHit = true
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
		return selection, decision, nil
REDACTED

	selection, candidateCount, topK, loadSkew, err := s.selectByLoadBalance(ctx, req)
	decision.Layer = openAIAccountScheduleLayerLoadBalance
	decision.CandidateCount = candidateCount
	decision.TopK = topK
	decision.LoadSkew = loadSkew
	if err != nil {
		return nil, decision, err
REDACTED
	if selection != nil && selection.Account != nil {
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
REDACTED
	return selection, decision, nil
REDACTED

func (s *defaultOpenAIAccountScheduler) selectBySessionHash(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, error) {
	sessionHash := strings.TrimSpace(req.SessionHash)
	if sessionHash == "" || s == nil || s.service == nil || s.service.cache == nil {
		return nil, nil
REDACTED

	accountID := req.StickyAccountID
	if accountID <= 0 {
		var err error
		accountID, err = s.service.getStickySessionAccountID(ctx, req.GroupID, sessionHash)
		if err != nil || accountID <= 0 {
			return nil, nil
	REDACTED
REDACTED
	if accountID <= 0 {
		return nil, nil
REDACTED
	if req.ExcludedIDs != nil {
		if _, excluded := req.ExcludedIDs[accountID]; excluded {
			return nil, nil
	REDACTED
REDACTED

	account, err := s.service.getSchedulableAccount(ctx, accountID)
	if err != nil || account == nil {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, nil
REDACTED
	if shouldClearStickySession(account, req.RequestedModel) || !account.IsOpenAI() {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, nil
REDACTED
	if req.RequestedModel != "" && !account.IsModelSupported(req.RequestedModel) {
		return nil, nil
REDACTED
	if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, nil
REDACTED

	result, acquireErr := s.service.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
	if acquireErr == nil && result.Acquired {
		_ = s.service.refreshStickySessionTTL(ctx, req.GroupID, sessionHash, s.service.openAIWSSessionStickyTTL())
		return &AccountSelectionResult{
			Account:     account,
			Acquired:    true,
			ReleaseFunc: result.ReleaseFunc,
	REDACTED, nil
REDACTED

	cfg := s.service.schedulingConfig()
	// WaitPlan.MaxConcurrency 使用 Concurrency（非 EffectiveLoadFactor），因为 WaitPlan 控制的是 Redis 实际并发槽位等待。
	if s.service.concurrencyService != nil {
		return &AccountSelectionResult{
			Account: account,
			WaitPlan: &AccountWaitPlan{
				AccountID:      accountID,
				MaxConcurrency: account.Concurrency,
				Timeout:        cfg.StickySessionWaitTimeout,
				MaxWaiting:     cfg.StickySessionMaxWaiting,
		REDACTED,
	REDACTED, nil
REDACTED
	return nil, nil
REDACTED

type openAIAccountCandidateScore struct {
	account   *Account
	loadInfo  *AccountLoadInfo
	score     float64
	errorRate float64
	ttft      float64
	hasTTFT   bool
REDACTED

type openAIAccountCandidateHeap []openAIAccountCandidateScore

func (h openAIAccountCandidateHeap) Len() int {
	return len(h)
REDACTED

func (h openAIAccountCandidateHeap) Less(i, j int) bool {
	// 最小堆根节点保存“最差”候选，便于 O(log k) 维护 topK。
	return isOpenAIAccountCandidateBetter(h[j], h[i])
REDACTED

func (h openAIAccountCandidateHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
REDACTED

func (h *openAIAccountCandidateHeap) Push(x any) {
	candidate, ok := x.(openAIAccountCandidateScore)
	if !ok {
		panic("openAIAccountCandidateHeap: invalid element type")
REDACTED
	*h = append(*h, candidate)
REDACTED

func (h *openAIAccountCandidateHeap) Pop() any {
	old := *h
	n := len(old)
	last := old[n-1]
	*h = old[:n-1]
	return last
REDACTED

func isOpenAIAccountCandidateBetter(left openAIAccountCandidateScore, right openAIAccountCandidateScore) bool {
	if left.score != right.score {
		return left.score > right.score
REDACTED
	if left.account.Priority != right.account.Priority {
		return left.account.Priority < right.account.Priority
REDACTED
	if left.loadInfo.LoadRate != right.loadInfo.LoadRate {
		return left.loadInfo.LoadRate < right.loadInfo.LoadRate
REDACTED
	if left.loadInfo.WaitingCount != right.loadInfo.WaitingCount {
		return left.loadInfo.WaitingCount < right.loadInfo.WaitingCount
REDACTED
	return left.account.ID < right.account.ID
REDACTED

func selectTopKOpenAICandidates(candidates []openAIAccountCandidateScore, topK int) []openAIAccountCandidateScore {
	if len(candidates) == 0 {
		return nil
REDACTED
	if topK <= 0 {
		topK = 1
REDACTED
	if topK >= len(candidates) {
		ranked := append([]openAIAccountCandidateScore(nil), candidates...)
		sort.Slice(ranked, func(i, j int) bool {
			return isOpenAIAccountCandidateBetter(ranked[i], ranked[j])
	REDACTED)
		return ranked
REDACTED

	best := make(openAIAccountCandidateHeap, 0, topK)
	for _, candidate := range candidates {
		if len(best) < topK {
			heap.Push(&best, candidate)
			continue
	REDACTED
		if isOpenAIAccountCandidateBetter(candidate, best[0]) {
			best[0] = candidate
			heap.Fix(&best, 0)
	REDACTED
REDACTED

	ranked := make([]openAIAccountCandidateScore, len(best))
	copy(ranked, best)
	sort.Slice(ranked, func(i, j int) bool {
		return isOpenAIAccountCandidateBetter(ranked[i], ranked[j])
REDACTED)
	return ranked
REDACTED

type openAISelectionRNG struct {
	state uint64
REDACTED

func newOpenAISelectionRNG(seed uint64) openAISelectionRNG {
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
REDACTED
	return openAISelectionRNG{state: seedREDACTED
REDACTED

func (r *openAISelectionRNG) nextUint64() uint64 {
	// xorshift64*
	x := r.state
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	r.state = x
	return x * 2685821657736338717
REDACTED

func (r *openAISelectionRNG) nextFloat64() float64 {
	// [0,1)
	return float64(r.nextUint64()>>11) / (1 << 53)
REDACTED

func deriveOpenAISelectionSeed(req OpenAIAccountScheduleRequest) uint64 {
	hasher := fnv.New64a()
	writeValue := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
	REDACTED
		_, _ = hasher.Write([]byte(trimmed))
		_, _ = hasher.Write([]byte{0REDACTED)
REDACTED

	writeValue(req.SessionHash)
	writeValue(req.PreviousResponseID)
	writeValue(req.RequestedModel)
	if req.GroupID != nil {
		_, _ = hasher.Write([]byte(strconv.FormatInt(*req.GroupID, 10)))
REDACTED

	seed := hasher.Sum64()
	// 对“无会话锚点”的纯负载均衡请求引入时间熵，避免固定命中同一账号。
	if strings.TrimSpace(req.SessionHash) == "" && strings.TrimSpace(req.PreviousResponseID) == "" {
		seed ^= uint64(time.Now().UnixNano())
REDACTED
	if seed == 0 {
		seed = uint64(time.Now().UnixNano()) ^ 0x9e3779b97f4a7c15
REDACTED
	return seed
REDACTED

func buildOpenAIWeightedSelectionOrder(
	candidates []openAIAccountCandidateScore,
	req OpenAIAccountScheduleRequest,
) []openAIAccountCandidateScore {
	if len(candidates) <= 1 {
		return append([]openAIAccountCandidateScore(nil), candidates...)
REDACTED

	pool := append([]openAIAccountCandidateScore(nil), candidates...)
	weights := make([]float64, len(pool))
	minScore := pool[0].score
	for i := 1; i < len(pool); i++ {
		if pool[i].score < minScore {
			minScore = pool[i].score
	REDACTED
REDACTED
	for i := range pool {
		// 将 top-K 分值平移到正区间，避免“单一最高分账号”长期垄断。
		weight := (pool[i].score - minScore) + 1.0
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 {
			weight = 1.0
	REDACTED
		weights[i] = weight
REDACTED

	order := make([]openAIAccountCandidateScore, 0, len(pool))
	rng := newOpenAISelectionRNG(deriveOpenAISelectionSeed(req))
	for len(pool) > 0 {
		total := 0.0
		for _, w := range weights {
			total += w
	REDACTED

		selectedIdx := 0
		if total > 0 {
			r := rng.nextFloat64() * total
			acc := 0.0
			for i, w := range weights {
				acc += w
				if r <= acc {
					selectedIdx = i
					break
			REDACTED
		REDACTED
	REDACTED else {
			selectedIdx = int(rng.nextUint64() % uint64(len(pool)))
	REDACTED

		order = append(order, pool[selectedIdx])
		pool = append(pool[:selectedIdx], pool[selectedIdx+1:]...)
		weights = append(weights[:selectedIdx], weights[selectedIdx+1:]...)
REDACTED
	return order
REDACTED

func (s *defaultOpenAIAccountScheduler) selectByLoadBalance(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, int, int, float64, error) {
	accounts, err := s.service.listSchedulableAccounts(ctx, req.GroupID)
	if err != nil {
		return nil, 0, 0, 0, err
REDACTED
	if len(accounts) == 0 {
		return nil, 0, 0, 0, errors.New("no available OpenAI accounts")
REDACTED

	filtered := make([]*Account, 0, len(accounts))
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[account.ID]; excluded {
				continue
		REDACTED
	REDACTED
		if !account.IsSchedulable() || !account.IsOpenAI() {
			continue
	REDACTED
		if req.RequestedModel != "" && !account.IsModelSupported(req.RequestedModel) {
			continue
	REDACTED
		if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
			continue
	REDACTED
		filtered = append(filtered, account)
		loadReq = append(loadReq, AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: account.EffectiveLoadFactor(),
	REDACTED)
REDACTED
	if len(filtered) == 0 {
		return nil, 0, 0, 0, errors.New("no available OpenAI accounts")
REDACTED

	loadMap := map[int64]*AccountLoadInfo{REDACTED
	if s.service.concurrencyService != nil {
		if batchLoad, loadErr := s.service.concurrencyService.GetAccountsLoadBatch(ctx, loadReq); loadErr == nil {
			loadMap = batchLoad
	REDACTED
REDACTED

	minPriority, maxPriority := filtered[0].Priority, filtered[0].Priority
	maxWaiting := 1
	loadRateSum := 0.0
	loadRateSumSquares := 0.0
	minTTFT, maxTTFT := 0.0, 0.0
	hasTTFTSample := false
	candidates := make([]openAIAccountCandidateScore, 0, len(filtered))
	for _, account := range filtered {
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.IDREDACTED
	REDACTED
		if account.Priority < minPriority {
			minPriority = account.Priority
	REDACTED
		if account.Priority > maxPriority {
			maxPriority = account.Priority
	REDACTED
		if loadInfo.WaitingCount > maxWaiting {
			maxWaiting = loadInfo.WaitingCount
	REDACTED
		errorRate, ttft, hasTTFT := s.stats.snapshot(account.ID)
		if hasTTFT && ttft > 0 {
			if !hasTTFTSample {
				minTTFT, maxTTFT = ttft, ttft
				hasTTFTSample = true
		REDACTED else {
				if ttft < minTTFT {
					minTTFT = ttft
			REDACTED
				if ttft > maxTTFT {
					maxTTFT = ttft
			REDACTED
		REDACTED
	REDACTED
		loadRate := float64(loadInfo.LoadRate)
		loadRateSum += loadRate
		loadRateSumSquares += loadRate * loadRate
		candidates = append(candidates, openAIAccountCandidateScore{
			account:   account,
			loadInfo:  loadInfo,
			errorRate: errorRate,
			ttft:      ttft,
			hasTTFT:   hasTTFT,
	REDACTED)
REDACTED
	loadSkew := calcLoadSkewByMoments(loadRateSum, loadRateSumSquares, len(candidates))

	weights := s.service.openAIWSSchedulerWeights()
	for i := range candidates {
		item := &candidates[i]
		priorityFactor := 1.0
		if maxPriority > minPriority {
			priorityFactor = 1 - float64(item.account.Priority-minPriority)/float64(maxPriority-minPriority)
	REDACTED
		loadFactor := 1 - clamp01(float64(item.loadInfo.LoadRate)/100.0)
		queueFactor := 1 - clamp01(float64(item.loadInfo.WaitingCount)/float64(maxWaiting))
		errorFactor := 1 - clamp01(item.errorRate)
		ttftFactor := 0.5
		if item.hasTTFT && hasTTFTSample && maxTTFT > minTTFT {
			ttftFactor = 1 - clamp01((item.ttft-minTTFT)/(maxTTFT-minTTFT))
	REDACTED

		item.score = weights.Priority*priorityFactor +
			weights.Load*loadFactor +
			weights.Queue*queueFactor +
			weights.ErrorRate*errorFactor +
			weights.TTFT*ttftFactor
REDACTED

	topK := s.service.openAIWSLBTopK()
	if topK > len(candidates) {
		topK = len(candidates)
REDACTED
	if topK <= 0 {
		topK = 1
REDACTED
	rankedCandidates := selectTopKOpenAICandidates(candidates, topK)
	selectionOrder := buildOpenAIWeightedSelectionOrder(rankedCandidates, req)

	for i := 0; i < len(selectionOrder); i++ {
		candidate := selectionOrder[i]
		result, acquireErr := s.service.tryAcquireAccountSlot(ctx, candidate.account.ID, candidate.account.Concurrency)
		if acquireErr != nil {
			return nil, len(candidates), topK, loadSkew, acquireErr
	REDACTED
		if result != nil && result.Acquired {
			if req.SessionHash != "" {
				_ = s.service.BindStickySession(ctx, req.GroupID, req.SessionHash, candidate.account.ID)
		REDACTED
			return &AccountSelectionResult{
				Account:     candidate.account,
				Acquired:    true,
				ReleaseFunc: result.ReleaseFunc,
		REDACTED, len(candidates), topK, loadSkew, nil
	REDACTED
REDACTED

	cfg := s.service.schedulingConfig()
	// WaitPlan.MaxConcurrency 使用 Concurrency（非 EffectiveLoadFactor），因为 WaitPlan 控制的是 Redis 实际并发槽位等待。
	candidate := selectionOrder[0]
	return &AccountSelectionResult{
		Account: candidate.account,
		WaitPlan: &AccountWaitPlan{
			AccountID:      candidate.account.ID,
			MaxConcurrency: candidate.account.Concurrency,
			Timeout:        cfg.FallbackWaitTimeout,
			MaxWaiting:     cfg.FallbackMaxWaiting,
	REDACTED,
REDACTED, len(candidates), topK, loadSkew, nil
REDACTED

func (s *defaultOpenAIAccountScheduler) isAccountTransportCompatible(account *Account, requiredTransport OpenAIUpstreamTransport) bool {
	// HTTP 入站可回退到 HTTP 线路，不需要在账号选择阶段做传输协议强过滤。
	if requiredTransport == OpenAIUpstreamTransportAny || requiredTransport == OpenAIUpstreamTransportHTTPSSE {
		return true
REDACTED
	if s == nil || s.service == nil || account == nil {
		return false
REDACTED
	return s.service.getOpenAIWSProtocolResolver().Resolve(account).Transport == requiredTransport
REDACTED

func (s *defaultOpenAIAccountScheduler) ReportResult(accountID int64, success bool, firstTokenMs *int) {
	if s == nil || s.stats == nil {
		return
REDACTED
	s.stats.report(accountID, success, firstTokenMs)
REDACTED

func (s *defaultOpenAIAccountScheduler) ReportSwitch() {
	if s == nil {
		return
REDACTED
	s.metrics.recordSwitch()
REDACTED

func (s *defaultOpenAIAccountScheduler) SnapshotMetrics() OpenAIAccountSchedulerMetricsSnapshot {
	if s == nil {
		return OpenAIAccountSchedulerMetricsSnapshot{REDACTED
REDACTED

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
REDACTED
	if selectTotal > 0 {
		snapshot.SchedulerLatencyMsAvg = float64(latencyTotal) / float64(selectTotal)
		snapshot.StickyHitRatio = float64(prevHit+sessionHit) / float64(selectTotal)
		snapshot.AccountSwitchRate = float64(switchTotal) / float64(selectTotal)
		snapshot.LoadSkewAvg = float64(loadSkewTotal) / 1000 / float64(selectTotal)
REDACTED
	return snapshot
REDACTED

func (s *OpenAIGatewayService) getOpenAIAccountScheduler() OpenAIAccountScheduler {
	if s == nil {
		return nil
REDACTED
	s.openaiSchedulerOnce.Do(func() {
		if s.openaiAccountStats == nil {
			s.openaiAccountStats = newOpenAIAccountRuntimeStats()
	REDACTED
		if s.openaiScheduler == nil {
			s.openaiScheduler = newDefaultOpenAIAccountScheduler(s, s.openaiAccountStats)
	REDACTED
REDACTED)
	return s.openaiScheduler
REDACTED

func (s *OpenAIGatewayService) SelectAccountWithScheduler(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{REDACTED,
	requiredTransport OpenAIUpstreamTransport,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{REDACTED
	scheduler := s.getOpenAIAccountScheduler()
	if scheduler == nil {
		selection, err := s.SelectAccountWithLoadAwareness(ctx, groupID, sessionHash, requestedModel, excludedIDs)
		decision.Layer = openAIAccountScheduleLayerLoadBalance
		return selection, decision, err
REDACTED

	var stickyAccountID int64
	if sessionHash != "" && s.cache != nil {
		if accountID, err := s.getStickySessionAccountID(ctx, groupID, sessionHash); err == nil && accountID > 0 {
			stickyAccountID = accountID
	REDACTED
REDACTED

	return scheduler.Select(ctx, OpenAIAccountScheduleRequest{
		GroupID:            groupID,
		SessionHash:        sessionHash,
		StickyAccountID:    stickyAccountID,
		PreviousResponseID: previousResponseID,
		RequestedModel:     requestedModel,
		RequiredTransport:  requiredTransport,
		ExcludedIDs:        excludedIDs,
REDACTED)
REDACTED

func (s *OpenAIGatewayService) ReportOpenAIAccountScheduleResult(accountID int64, success bool, firstTokenMs *int) {
	scheduler := s.getOpenAIAccountScheduler()
	if scheduler == nil {
		return
REDACTED
	scheduler.ReportResult(accountID, success, firstTokenMs)
REDACTED

func (s *OpenAIGatewayService) RecordOpenAIAccountSwitch() {
	scheduler := s.getOpenAIAccountScheduler()
	if scheduler == nil {
		return
REDACTED
	scheduler.ReportSwitch()
REDACTED

func (s *OpenAIGatewayService) SnapshotOpenAIAccountSchedulerMetrics() OpenAIAccountSchedulerMetricsSnapshot {
	scheduler := s.getOpenAIAccountScheduler()
	if scheduler == nil {
		return OpenAIAccountSchedulerMetricsSnapshot{REDACTED
REDACTED
	return scheduler.SnapshotMetrics()
REDACTED

func (s *OpenAIGatewayService) openAIWSSessionStickyTTL() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		return time.Duration(s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second
REDACTED
	return openaiStickySessionTTL
REDACTED

func (s *OpenAIGatewayService) openAIWSLBTopK() int {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.LBTopK > 0 {
		return s.cfg.Gateway.OpenAIWS.LBTopK
REDACTED
	return 7
REDACTED

func (s *OpenAIGatewayService) openAIWSSchedulerWeights() GatewayOpenAIWSSchedulerScoreWeightsView {
	if s != nil && s.cfg != nil {
		return GatewayOpenAIWSSchedulerScoreWeightsView{
			Priority:  s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority,
			Load:      s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load,
			Queue:     s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue,
			ErrorRate: s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate,
			TTFT:      s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT,
	REDACTED
REDACTED
	return GatewayOpenAIWSSchedulerScoreWeightsView{
		Priority:  1.0,
		Load:      1.0,
		Queue:     0.7,
		ErrorRate: 0.8,
		TTFT:      0.5,
REDACTED
REDACTED

type GatewayOpenAIWSSchedulerScoreWeightsView struct {
	Priority  float64
	Load      float64
	Queue     float64
	ErrorRate float64
	TTFT      float64
REDACTED

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
REDACTED
REDACTED

func calcLoadSkewByMoments(sum float64, sumSquares float64, count int) float64 {
	if count <= 1 {
		return 0
REDACTED
	mean := sum / float64(count)
	variance := sumSquares/float64(count) - mean*mean
	if variance < 0 {
		variance = 0
REDACTED
	return math.Sqrt(variance)
REDACTED

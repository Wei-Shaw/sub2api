package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// cnQuotaProber 抽象额度探测（*CNProviderQuotaService 实现，测试可替换）。
type cnQuotaProber interface {
	QueryUsage(ctx context.Context, accountID int64) (*CNProviderQuotaProbeResult, error)
REDACTED

// cnQuotaProbeConcurrency 周期任务并发探测额度账号的并发度。
const cnQuotaProbeConcurrency = 4

// CNProviderBalanceCheckService 周期性探测国产供应商账号：
//   - payg（按量付费）：余额低于阈值则临时停调，恢复则清除（仅清除本服务写入的停调）；
//   - coding plan：调用 CNProviderQuotaService 探测 5h/weekly 滚动窗口并落 extra 快照，
//     调度阈值评估（cnProviderThresholdCandidates）据此自动停调/恢复。
//
// 克隆自 AccountExpiryService 的 Start/Stop/runOnce + ticker 骨架。
// 余额探测仅覆盖有公开余额端点的 kimi / deepseek；智谱无余额端点，仅靠响应式 429/402。
// 额度探测覆盖 kimi / zhipu 的 coding plan 账号（deepseek 无 coding 套餐）。
type CNProviderBalanceCheckService struct {
	accountRepo    AccountRepository
	balanceService *CNProviderBalanceService
	quotaService   cnQuotaProber
	cfg            *config.Config
	interval       time.Duration
	stopCh         chan struct{REDACTED
	stopOnce       sync.Once
	wg             sync.WaitGroup
REDACTED

// NewCNProviderBalanceCheckService 构造周期余额/额度检测服务。
// interval <= 0 时 Start() 直接返回（不启动），便于通过配置关闭。
func NewCNProviderBalanceCheckService(
	accountRepo AccountRepository,
	balanceService *CNProviderBalanceService,
	quotaService *CNProviderQuotaService,
	cfg *config.Config,
	interval time.Duration,
) *CNProviderBalanceCheckService {
	return &CNProviderBalanceCheckService{
		accountRepo:    accountRepo,
		balanceService: balanceService,
		quotaService:   quotaService,
		cfg:            cfg,
		interval:       interval,
		stopCh:         make(chan struct{REDACTED),
REDACTED
REDACTED

func (s *CNProviderBalanceCheckService) Start() {
	if s == nil || s.accountRepo == nil || s.balanceService == nil || s.cfg == nil {
		return
REDACTED
	if !s.cfg.Gateway.CNProviders.BalanceCheckEnabled {
		return
REDACTED
	if s.interval <= 0 {
		return
REDACTED
	log.Printf("[CNBalance] started (interval=%s threshold=%.2f)", s.interval, s.cfg.Gateway.CNProviders.BalanceThreshold)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		// 启动后先等待一个周期再首次探测，避免与进程启动峰重叠。
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
		REDACTED
	REDACTED
REDACTED()
REDACTED

func (s *CNProviderBalanceCheckService) Stop() {
	if s == nil {
		return
REDACTED
	s.stopOnce.Do(func() {
		close(s.stopCh)
REDACTED)
	s.wg.Wait()
REDACTED

func (s *CNProviderBalanceCheckService) runOnce() {
	// 收集 coding 探测目标（kimi/deepseek + 智谱）与 payg 检查队列。
	// coding 探测统一在收集完成后按 4 并发执行：单账号探测 15-20s，串行 ×
	// 多账号会耗尽整体预算（120s 上限），排在后面的账号快照会饥饿，
	// 连锁影响阈值停调的新鲜度判定。
	type quotaTarget struct {
		id       int64
		platform string
REDACTED
	var quotaTargets []quotaTarget
	var paygTargets []*Account
	collect := func(platform string, accounts []Account) {
		for i := range accounts {
			account := &accounts[i]
			if !account.IsActive() {
				continue
		REDACTED
			// coding 账号：探测滚动窗口并落快照（不要求 Schedulable——已被
			// 阈值停调的账号也需要新鲜快照决定是否续停）。
			if account.IsCodingPlan() {
				quotaTargets = append(quotaTargets, quotaTarget{id: account.ID, platform: account.PlatformREDACTED)
				continue
		REDACTED
			// payg 余额探测仅 kimi/deepseek（智谱无公开余额端点，payg 账号
			// 依赖响应式 402/429 处理）。
			if platform != PlatformZhipu && account.Schedulable {
				paygTargets = append(paygTargets, account)
		REDACTED
	REDACTED
REDACTED
	for _, platform := range s.platforms() {
		accounts, err := s.accountRepo.ListByPlatform(context.Background(), platform)
		if err != nil {
			log.Printf("[CNBalance] list %s accounts failed: %v", platform, err)
			continue
	REDACTED
		collect(platform, accounts)
REDACTED
	// 智谱无余额端点，仅进额度探测。
	if s.quotaService != nil {
		accounts, err := s.accountRepo.ListByPlatform(context.Background(), PlatformZhipu)
		if err != nil {
			log.Printf("[CNBalance] list %s accounts failed: %v", PlatformZhipu, err)
	REDACTED else {
			collect(PlatformZhipu, accounts)
	REDACTED
REDACTED

	// 预算按工作量放大：4 并发 × 15s/批 + payg 每账号 5s，下限 30s 上限 300s。
	batches := (len(quotaTargets) + cnQuotaProbeConcurrency - 1) / cnQuotaProbeConcurrency
	timeout := 30*time.Second + time.Duration(batches)*15*time.Second + time.Duration(len(paygTargets))*5*time.Second
	if timeout > 300*time.Second {
		timeout = 300 * time.Second
REDACTED
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	threshold := s.cfg.Gateway.CNProviders.BalanceThreshold
	paused, cleared := 0, 0
	for _, account := range paygTargets {
		switch s.checkOne(ctx, account, threshold) {
		case cnBalancePaused:
			paused++
		case cnBalanceCleared:
			cleared++
	REDACTED
REDACTED

	if len(quotaTargets) > 0 && s.quotaService != nil {
		sem := make(chan struct{REDACTED, cnQuotaProbeConcurrency)
		var wg sync.WaitGroup
		for _, target := range quotaTargets {
			wg.Add(1)
			go func(t quotaTarget) {
				defer wg.Done()
				sem <- struct{REDACTED{REDACTED
				defer func() { <-sem REDACTED()
				s.probeQuota(ctx, t.id, t.platform)
		REDACTED(target)
	REDACTED
		wg.Wait()
REDACTED

	if paused > 0 || cleared > 0 {
		log.Printf("[CNBalance] paused=%d cleared=%d (threshold=%.2f)", paused, cleared, threshold)
REDACTED
REDACTED

// probeQuota 探测单个 coding plan 账号的滚动窗口用量并落 extra 快照。
// 不在此处做停调/恢复决策：调度阈值评估读取快照统一判定（含暂停账号的续停）。
func (s *CNProviderBalanceCheckService) probeQuota(ctx context.Context, accountID int64, platform string) {
	if s.quotaService == nil {
		return
REDACTED
	result, err := s.quotaService.QueryUsage(ctx, accountID)
	if err != nil {
		log.Printf("[CNBalance] quota probe account %d (%s) failed: %v", accountID, platform, err)
		return
REDACTED
	if result != nil && !result.Success && result.Error != "" {
		log.Printf("[CNBalance] quota probe account %d (%s) error: %s", accountID, platform, result.Error)
REDACTED
REDACTED

type cnBalanceCheckOutcome int

const (
	cnBalanceNoChange cnBalanceCheckOutcome = iota
	cnBalancePaused
	cnBalanceCleared
)

// checkOne 探测单账号余额并决定停调/恢复。探测失败时不动现状（避免瞬时网络抖动
// 误解除或误停调）。
func (s *CNProviderBalanceCheckService) checkOne(ctx context.Context, account *Account, threshold float64) cnBalanceCheckOutcome {
	result, err := s.balanceService.QueryBalance(ctx, account.ID)
	if err != nil || result == nil || !result.Success {
		return cnBalanceNoChange
REDACTED

	// 双币种（deepseek CNY+USD）任一币种余额达标即可继续调度；仅当全部低于
	// 阈值（或不可用）才停调。
	low := !result.Available || allCNBalancesBelowThreshold(result, threshold)
	if low {
		// 已被（任何来源）停调时不覆盖其 reason。
		if !account.IsSchedulable() {
			return cnBalanceNoChange
	REDACTED
		reason := cnBalanceLowReason(fmt.Sprintf("余额 %.4g %s 低于阈值 %.2f", result.Balance, result.Currency, threshold))
		if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, time.Now().Add(s.cooldown()), reason); err != nil {
			log.Printf("[CNBalance] pause account %d failed: %v", account.ID, err)
			return cnBalanceNoChange
	REDACTED
		log.Printf("[CNBalance] paused account %d (%s): balance=%.4g %s", account.ID, account.Platform, result.Balance, result.Currency)
		return cnBalancePaused
REDACTED

	// 余额健康：仅清除「本服务写入」的临时停调（reason 前缀匹配），不触碰其他子系统。
	if account.TempUnschedulableUntil != nil && strings.HasPrefix(account.TempUnschedulableReason, cnBalanceLowReasonPrefix) {
		if err := s.accountRepo.ClearTempUnschedulable(ctx, account.ID); err != nil {
			log.Printf("[CNBalance] clear account %d failed: %v", account.ID, err)
			return cnBalanceNoChange
	REDACTED
		log.Printf("[CNBalance] reactivated account %d (%s): balance=%.4g %s", account.ID, account.Platform, result.Balance, result.Currency)
		return cnBalanceCleared
REDACTED
	return cnBalanceNoChange
REDACTED

func (s *CNProviderBalanceCheckService) platforms() []string {
	return []string{PlatformKimi, PlatformDeepseekREDACTED
REDACTED

// allCNBalancesBelowThreshold 判断全部币种余额是否均低于阈值。
// 无明细时退回主币种判定（与旧行为一致）。
func allCNBalancesBelowThreshold(result *CNProviderBalanceResult, threshold float64) bool {
	if len(result.Balances) == 0 {
		return result.Balance < threshold
REDACTED
	for _, entry := range result.Balances {
		if entry.Balance >= threshold {
			return false
	REDACTED
REDACTED
	return true
REDACTED

// cooldown 返回临时停调持续时长（= 2× 检测周期），与响应式 402/429 路径一致。
func (s *CNProviderBalanceCheckService) cooldown() time.Duration {
	minutes := 10
	if s.cfg != nil {
		if cfgMin := s.cfg.Gateway.CNProviders.BalanceCheckIntervalMinutes; cfgMin > 0 {
			minutes = cfgMin
	REDACTED
REDACTED
	cooldown := time.Duration(minutes) * time.Minute * 2
	if cooldown < time.Minute {
		cooldown = 10 * time.Minute
REDACTED
	return cooldown
REDACTED

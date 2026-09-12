package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/google/uuid"
)

const (
	openAIAutoResetScanInterval  = time.Minute
	openAIAutoResetSnapshotTTL   = openAIProbeCacheTTL
	openAIAutoResetBatchSize     = 100
	openAIAutoResetWorkerCount   = 4
	openAIAutoResetQueueCapacity = 1024
	openAIAutoResetAttemptTTL    = 8 * 24 * time.Hour
	openAIAutoResetLeaderLockKey = "jobs:openai-auto-reset-credit"
)

const (
	openAIAutoResetCreditRefreshInterval = 24 * time.Hour
	openAIAutoResetMaxClockSkew          = 5 * time.Minute
	openAIAutoResetStartupDelay          = 5 * time.Second
	openAIAutoResetSchedulerLockKey      = "jobs:openai-auto-reset-credit:scheduler"
	openAIAutoResetSchedulerLeaseTTL     = 3 * time.Minute
)

// 启动后每 6 秒取一个账号的卡信息，n 个账号共摊 n/10 分钟，避免对上游并发过大。
var openAIAutoResetInitialFetchInterval = 6 * time.Second

const (
	OpenAIAutoResetStatusChecking  = "checking"
	OpenAIAutoResetStatusAvailable = "available"
	OpenAIAutoResetStatusResetting = "resetting"
	OpenAIAutoResetStatusSuccess   = "success"
	OpenAIAutoResetStatusNoCredit  = "no_credit"
	OpenAIAutoResetStatusFailed    = "failed"
)

// OpenAIAutoResetCreditState 是可返回管理端的脱敏运行态。Attempt* 仅保存不可逆
// 指纹，用于重启后拒绝切换到另一张卡；不会保存卡 ID 或兑换 ID。
type OpenAIAutoResetCreditState struct {
	Status            string `json:"status"`
	TriggerWindow     string `json:"trigger_window,omitempty"`
	AvailableCount    int    `json:"available_count"`
	CheckedAt         string `json:"checked_at,omitempty"`
	LastResultAt      string `json:"last_result_at,omitempty"`
	ErrorCode         string `json:"error_code,omitempty"`
	AttemptCycleHash  string `json:"attempt_cycle_hash,omitempty"`
	AttemptCreditHash string `json:"attempt_credit_hash,omitempty"`
}

type openAIAutoResetQuota interface {
	QueryUsage(ctx context.Context, accountID int64) (*OpenAIQuotaUsage, error)
	CacheResetCreditsSnapshot(ctx context.Context, accountID int64, credits *OpenAIRateLimitResetCredits) error
	CachePostResetSnapshot(ctx context.Context, accountID int64, usage *OpenAIQuotaUsage) error
	ResetCreditTargeted(ctx context.Context, accountID int64, creditID, redeemRequestID string) (*OpenAIQuotaResetResult, error)
}

type openAIAutoResetContextKey struct{}

func withOpenAIAutoResetContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIAutoResetContextKey{}, true)
}

func isOpenAIAutoResetContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	value, _ := ctx.Value(openAIAutoResetContextKey{}).(bool)
	return value
}

type openAIAutoResetRecovery interface {
	RecoverAccountState(ctx context.Context, accountID int64, options AccountRecoveryOptions) (*SuccessfulTestRecoveryResult, error)
}

// OpenAIQuotaAutoResetService 通过小型去重队列承接实时信号，并用分钟扫描补偿
// 重启、漏事件和多实例读取；真正消费仍由 PostgreSQL 幂等记录串行化。
type OpenAIQuotaAutoResetService struct {
	accountRepo AccountRepository
	quota       openAIAutoResetQuota
	recoverer   openAIAutoResetRecovery
	idempotency *IdempotencyCoordinator
	audit       *AuditLogService
	settings    *SettingService
	leaderLock  LeaderLockCache

	ctx     context.Context
	cancel  context.CancelFunc
	queue   chan int64
	pending sync.Map
	owner   string
	start   sync.Once
	stop    sync.Once
	wg      sync.WaitGroup

	fetchStates     sync.Map
	expiryDue       sync.Map
	stateDue        sync.Map
	expiryTimers    sync.Map
	schedulerLeader atomic.Bool
	// 上游 Date 减本机时钟的偏移，进程内记录：它是实例属性而非账号属性，多实例各自的本机
	// 时钟不同，落库共享会互相污染。没有实查结果时估算上游当前时间用它折算。
	upstreamClockOffset atomic.Int64
	upstreamClockKnown  atomic.Bool
}

// openAIAutoResetFetchState 只存进程内：nextAt 带单调时钟读数，重启后靠启动错峰重建。
type openAIAutoResetFetchState struct {
	nextAt  time.Time
	fetched bool
	config  OpenAIAutoResetCreditConfig
}

func NewOpenAIQuotaAutoResetService(
	accountRepo AccountRepository,
	quota openAIAutoResetQuota,
	recoverer openAIAutoResetRecovery,
	idempotency *IdempotencyCoordinator,
	audit *AuditLogService,
	settings *SettingService,
	leaderLock LeaderLockCache,
) *OpenAIQuotaAutoResetService {
	ctx, cancel := context.WithCancel(context.Background())
	return &OpenAIQuotaAutoResetService{
		accountRepo: accountRepo,
		quota:       quota,
		recoverer:   recoverer,
		idempotency: idempotency,
		audit:       audit,
		settings:    settings,
		leaderLock:  leaderLock,
		ctx:         ctx,
		cancel:      cancel,
		queue:       make(chan int64, openAIAutoResetQueueCapacity),
		owner:       uuid.NewString(),
	}
}

func (s *OpenAIQuotaAutoResetService) Start() {
	if s == nil || s.accountRepo == nil || s.quota == nil || s.idempotency == nil {
		return
	}
	s.start.Do(func() {
		setOpenAIAutoResetNotifier(s)
		for range openAIAutoResetWorkerCount {
			s.wg.Add(1)
			go s.runWorker()
		}
		s.wg.Add(1)
		go s.runScanner()
	})
}

func (s *OpenAIQuotaAutoResetService) Stop() {
	if s == nil {
		return
	}
	s.stop.Do(func() {
		clearOpenAIAutoResetNotifier(s)
		s.cancel()
		s.wg.Wait()
		if s.leaderLock != nil && s.schedulerLeader.Load() {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = s.leaderLock.ReleaseLeaderLock(releaseCtx, openAIAutoResetSchedulerLockKey, s.owner)
			cancel()
		}
		s.resignSchedulerLeader()
	})
}

// Notify 是请求热路径的非阻塞入口。同一账号尚在队列时只保留一个任务；队列
// 满时丢弃本次信号，分钟扫描仍会补偿，因此不会反向拖慢网关请求。
func (s *OpenAIQuotaAutoResetService) Notify(accountID int64) {
	if s == nil || accountID <= 0 {
		return
	}
	if _, loaded := s.pending.LoadOrStore(accountID, struct{}{}); loaded {
		return
	}
	select {
	case <-s.ctx.Done():
		s.pending.Delete(accountID)
	case s.queue <- accountID:
	default:
		s.pending.Delete(accountID)
		slog.Warn("openai_auto_reset_queue_full", "account_id", accountID)
	}
}

func (s *OpenAIQuotaAutoResetService) runWorker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case accountID := <-s.queue:
			ctx, cancel := context.WithTimeout(s.ctx, 50*time.Second)
			if err := s.evaluateAccount(ctx, accountID); err != nil && !errors.Is(err, context.Canceled) {
				slog.Warn("openai_auto_reset_evaluate_failed", "account_id", accountID, "error_code", infraerrors.Reason(err))
			}
			cancel()
			s.pending.Delete(accountID)
		}
	}
}

func (s *OpenAIQuotaAutoResetService) runScanner() {
	defer s.wg.Done()
	timer := time.NewTimer(openAIAutoResetStartupDelay)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return
	case <-timer.C:
		s.refreshSchedulerLease(s.ctx)
		s.scanEnabledAccounts(s.ctx)
	}
	ticker := time.NewTicker(openAIAutoResetScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.refreshSchedulerLease(s.ctx)
			s.scanEnabledAccounts(s.ctx)
		}
	}
}

func (s *OpenAIQuotaAutoResetService) scanEnabledAccounts(ctx context.Context) {
	release, scan := s.tryAcquireScanLock(ctx)
	if !scan {
		return
	}
	if release != nil {
		defer release()
	}
	s.forEachEnabledAccount(ctx, s.Notify)
}

func (s *OpenAIQuotaAutoResetService) scheduleInitialFetch(ctx context.Context) {
	var ids []int64
	s.forEachEnabledAccount(ctx, func(accountID int64) { ids = append(ids, accountID) })
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	start := time.Now()
	for i, id := range ids {
		s.fetchStates.Store(id, openAIAutoResetFetchState{nextAt: start.Add(time.Duration(i) * openAIAutoResetInitialFetchInterval)})
	}
	s.wg.Add(1)
	go s.runInitialFetch(ctx, ids)
}

func (s *OpenAIQuotaAutoResetService) runInitialFetch(ctx context.Context, ids []int64) {
	defer s.wg.Done()
	ticker := time.NewTicker(openAIAutoResetInitialFetchInterval)
	defer ticker.Stop()
	for i, id := range ids {
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		s.Notify(id)
	}
}

func (s *OpenAIQuotaAutoResetService) forEachEnabledAccount(ctx context.Context, fn func(accountID int64)) {
	for page := 1; ; page++ {
		accounts, pageInfo, err := s.accountRepo.ListWithFilters(ctx, pagination.PaginationParams{
			Page: page, PageSize: openAIAutoResetBatchSize,
		}, PlatformOpenAI, AccountTypeOAuth, StatusActive, "", 0, "")
		if err != nil {
			slog.Warn("openai_auto_reset_scan_failed", "page", page, "error", err)
			return
		}
		for i := range accounts {
			account := &accounts[i]
			if account.Schedulable && ResolveOpenAIAutoResetCreditConfig(account).Active() {
				fn(account.ID)
			}
		}
		if len(accounts) < openAIAutoResetBatchSize || pageInfo == nil || page >= pageInfo.Pages {
			return
		}
	}
}

// 调度角色（启动错峰、每日取卡、到期定时器）只由持有租约的实例承担，租约随每分钟扫描续期；
// 没有 Redis 时退化为单实例，每个进程都是领导者。
func (s *OpenAIQuotaAutoResetService) isSchedulerLeader() bool {
	return s.leaderLock == nil || s.schedulerLeader.Load()
}

func (s *OpenAIQuotaAutoResetService) refreshSchedulerLease(ctx context.Context) {
	if s.leaderLock == nil {
		s.becomeSchedulerLeader(ctx)
		return
	}
	lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var held bool
	var err error
	if s.schedulerLeader.Load() {
		held, err = s.leaderLock.ExtendLeaderLock(lockCtx, openAIAutoResetSchedulerLockKey, s.owner, openAIAutoResetSchedulerLeaseTTL)
	} else {
		held, err = s.leaderLock.TryAcquireLeaderLock(lockCtx, openAIAutoResetSchedulerLockKey, s.owner, openAIAutoResetSchedulerLeaseTTL)
	}
	if err != nil {
		// 与扫描锁一致：协调设施故障时放行，宁可重复取卡也不让所有实例同时停摆。
		slog.Warn("openai_auto_reset_scheduler_lease_unavailable", "error", err)
		s.becomeSchedulerLeader(ctx)
		return
	}
	if held {
		s.becomeSchedulerLeader(ctx)
		return
	}
	s.resignSchedulerLeader()
}

func (s *OpenAIQuotaAutoResetService) becomeSchedulerLeader(ctx context.Context) {
	if s.schedulerLeader.Swap(true) {
		return
	}
	slog.Info("openai_auto_reset_scheduler_leader_acquired", "owner", s.owner)
	s.scheduleInitialFetch(ctx)
}

func (s *OpenAIQuotaAutoResetService) resignSchedulerLeader() {
	if !s.schedulerLeader.Swap(false) {
		return
	}
	slog.Info("openai_auto_reset_scheduler_leader_released", "owner", s.owner)
	s.expiryTimers.Range(func(key, value any) bool {
		if timer, ok := value.(*time.Timer); ok {
			timer.Stop()
		}
		s.expiryTimers.Delete(key)
		return true
	})
	s.fetchStates.Range(func(key, _ any) bool {
		s.fetchStates.Delete(key)
		return true
	})
}

// Redis 锁异常时允许重复扫描，避免协调设施故障导致所有实例同时停止补偿；
// 消费唯一性由数据库幂等记录负责，扫描锁只用于削减重复查询。
func (s *OpenAIQuotaAutoResetService) tryAcquireScanLock(ctx context.Context) (func(), bool) {
	if s.leaderLock == nil {
		return func() {}, true
	}
	ok, err := s.leaderLock.TryAcquireLeaderLock(ctx, openAIAutoResetLeaderLockKey, s.owner, 55*time.Second)
	if err != nil {
		slog.Warn("openai_auto_reset_leader_lock_unavailable", "error", err)
		return func() {}, true
	}
	if !ok {
		return nil, false
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.leaderLock.ReleaseLeaderLock(releaseCtx, openAIAutoResetLeaderLockKey, s.owner)
	}, true
}

type openAIAutoResetAssessment struct {
	triggerWindow    string
	resetReached     bool
	thresholdReached bool
	pauseReached     bool
	utilization5h    float64
	utilization7d    float64
	threshold5h      float64
	threshold7d      float64
}

func (s *OpenAIQuotaAutoResetService) evaluateAccount(ctx context.Context, accountID int64) error {
	ctx = withOpenAIAutoResetContext(ctx)
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return err
	}
	if account.IsShadow() {
		if account.ParentAccountID != nil {
			s.Notify(*account.ParentAccountID)
		}
		return nil
	}
	config := ResolveOpenAIAutoResetCreditConfig(account)
	if !config.ExpiryEnabled {
		if s.disarmExpiryTimer(accountID) || account.Extra[OpenAIAutoResetCreditExpiryAtExtraKey] != nil {
			s.persistExpiryAt(ctx, accountID, nil)
		}
	}
	if !account.IsActive() || !account.Schedulable {
		return nil
	}

	now := time.Now()
	leader := s.isSchedulerLeader()
	fetch, scheduled := s.fetchStates.Load(accountID)
	fetchState, _ := fetch.(openAIAutoResetFetchState)
	// 开关或提前量变化时立即重取一次，刷新缓存并按新配置重排定时器。
	configChanged := fetchState.fetched && fetchState.config != config
	refreshDue := leader && (!scheduled || !now.Before(fetchState.nextAt) || configChanged)
	if !config.Active() {
		return nil
	}
	assessment := s.assessExtra(account, config, now)
	state := openAIAutoResetStateFromExtra(account.Extra)
	_, expiryDue := s.expiryDue.LoadAndDelete(accountID)
	_, stateDue := s.stateDue.LoadAndDelete(accountID)
	// 到期触发的评估失败后，卡已在提前窗口内排不出定时器，用量快照也可能刚刷新，
	// 只能靠落库的失败状态在下一分钟扫描重试（重启后同样有效）；按上游时钟折算后
	// 缓存快照里没有未过期的卡时停止，避免明细长期缺失的账号每分钟打上游。
	expiryRetryDue := config.ExpiryEnabled && state != nil && state.Status == OpenAIAutoResetStatusFailed &&
		openAIAutoResetWindowHasExpiry(state.TriggerWindow) &&
		s.expiryRetryWithinCachedCredit(account.Extra, config.ExpiryLead, now)
	expiryPending := expiryDue || expiryRetryDue
	// 用量快照过期只驱动阈值路径重查；只开到期用卡的账号按 24 小时计划与定时器取卡。
	// 计划内取卡只由领导实例做；错峰未轮到的账号也不因用量快照过期而提前实查，
	// 避免重启时集中打上游。用量阈值触发不受此限。
	needsQuery := (openAIAutoResetSnapshotStale(account.Extra, now) && fetchState.fetched && config.Enabled) ||
		assessment.thresholdReached || expiryPending || stateDue || refreshDue
	if assessment.pauseReached && !assessment.resetReached {
		needsQuery = needsQuery || state == nil || state.Status == OpenAIAutoResetStatusChecking || state.Status == OpenAIAutoResetStatusFailed || openAIAutoResetStateStale(state, now)
	}
	if !needsQuery {
		// 稳态整理只清理触发窗口残留，失败记录保留到下一次实查解决。
		if !assessment.pauseReached && state != nil && state.Status != OpenAIAutoResetStatusFailed && state.TriggerWindow != "" {
			state.TriggerWindow = ""
			state.ErrorCode = ""
			state.CheckedAt = now.UTC().Format(time.RFC3339)
			if state.AvailableCount > 0 {
				state.Status = OpenAIAutoResetStatusAvailable
			} else {
				state.Status = OpenAIAutoResetStatusNoCredit
			}
			return s.persistState(ctx, accountID, state)
		}
		return nil
	}

	checkingWindow := assessment.triggerWindow
	if expiryPending {
		// 到期待处理时的查询失败也要按到期语义重试，把窗口写进状态供下一次扫描判定。
		checkingWindow = openAIAutoResetWindowWithExpiry(checkingWindow)
	}
	checking := &OpenAIAutoResetCreditState{
		Status:         OpenAIAutoResetStatusChecking,
		TriggerWindow:  checkingWindow,
		AvailableCount: stateAvailableCount(state),
		CheckedAt:      now.UTC().Format(time.RFC3339),
	}
	copyOpenAIAutoResetAttempt(checking, state)
	if err := s.persistState(ctx, accountID, checking); err != nil {
		return err
	}

	usage, err := s.quota.QueryUsage(ctx, accountID)
	if err != nil || usage == nil {
		if leader {
			s.fetchStates.Store(accountID, openAIAutoResetFetchState{nextAt: now.Add(openAIAutoResetSnapshotTTL), fetched: fetchState.fetched, config: fetchState.config})
		}
		return s.failState(ctx, accountID, checking, "RESET_CREDIT_QUERY_FAILED", err)
	}
	if leader {
		nextAt := now.Add(openAIAutoResetCreditRefreshInterval)
		if config.ExpiryEnabled && usage.upstreamTime.IsZero() {
			// 没有上游时间就排不了定时器，按失败重试节奏继续取，直到拿到 Date 头。
			nextAt = now.Add(openAIAutoResetSnapshotTTL)
		}
		s.fetchStates.Store(accountID, openAIAutoResetFetchState{nextAt: nextAt, fetched: true, config: config})
		s.armExpiryTimer(ctx, accountID, config, usage)
	}
	if err := s.persistFreshUsage(ctx, accountID, usage, now); err != nil {
		code := "USAGE_SNAPSHOT_WRITE_FAILED"
		if errors.Is(err, errOpenAIQuotaResetCreditsRefreshFailed) {
			code = "RESET_CREDIT_DETAILS_INCOMPLETE"
		}
		return s.failState(ctx, accountID, checking, code, err)
	}
	if usage.RateLimitResetCredits == nil {
		return s.failState(ctx, accountID, checking, "RESET_CREDIT_DETAILS_UNAVAILABLE", nil)
	}

	// 查询期间管理员可能关闭开关；消费前重新读取账号，确保尚未发出的任务可取消。
	account, err = s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return err
	}
	config = ResolveOpenAIAutoResetCreditConfig(account)
	if !config.Active() {
		return nil
	}
	assessment = s.assessUsage(usage, account, config, now)
	if config.ExpiryEnabled && !usage.upstreamTime.IsZero() && now.Sub(usage.upstreamTime).Abs() > openAIAutoResetMaxClockSkew {
		slog.Warn("openai_auto_reset_clock_skew", "account_id", accountID, "skew", now.Sub(usage.upstreamTime).String())
	}
	available := usage.RateLimitResetCredits.AvailableCount
	if !assessment.resetReached {
		status := OpenAIAutoResetStatusNoCredit
		if available > 0 {
			status = OpenAIAutoResetStatusAvailable
		}
		return s.persistState(ctx, accountID, &OpenAIAutoResetCreditState{
			Status:         status,
			TriggerWindow:  assessment.triggerWindow,
			AvailableCount: available,
			CheckedAt:      now.UTC().Format(time.RFC3339),
		})
	}
	if available <= 0 {
		return s.persistState(ctx, accountID, &OpenAIAutoResetCreditState{
			Status:         OpenAIAutoResetStatusNoCredit,
			TriggerWindow:  assessment.triggerWindow,
			AvailableCount: 0,
			CheckedAt:      now.UTC().Format(time.RFC3339),
			LastResultAt:   now.UTC().Format(time.RFC3339),
			ErrorCode:      "NO_RESET_CREDIT",
		})
	}

	cycleSeed := openAIAutoResetCycleSeed(usage)
	cycleHash := shortOpenAIAutoResetHash(cycleSeed)
	candidate, selectErr := selectOpenAIAutoResetCandidate(usage.autoResetCandidates, available, state, cycleHash)
	if selectErr != nil {
		failed := checking
		failed.AvailableCount = available
		failed.TriggerWindow = assessment.triggerWindow
		failed.AttemptCycleHash = cycleHash
		return s.failState(ctx, accountID, failed, infraerrors.Reason(selectErr), selectErr)
	}
	creditHash := shortOpenAIAutoResetHash(candidate.ID)
	stableKey := fmt.Sprintf("oarc:%d:%s:%s", accountID, creditHash, cycleHash)
	redeemRequestID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(stableKey)).String()
	resetting := &OpenAIAutoResetCreditState{
		Status:            OpenAIAutoResetStatusResetting,
		TriggerWindow:     assessment.triggerWindow,
		AvailableCount:    available,
		CheckedAt:         now.UTC().Format(time.RFC3339),
		AttemptCycleHash:  cycleHash,
		AttemptCreditHash: creditHash,
	}
	if err := s.persistState(ctx, accountID, resetting); err != nil {
		return err
	}

	account, err = s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil || !ResolveOpenAIAutoResetCreditConfig(account).Active() {
		return err
	}
	result, err := s.idempotency.Execute(ctx, IdempotencyExecuteOptions{
		Scope:          "openai_auto_reset_credit",
		ActorScope:     fmt.Sprintf("account:%d", accountID),
		Method:         http.MethodPost,
		Route:          "/system/openai/reset-credit/auto",
		IdempotencyKey: stableKey,
		Payload: map[string]any{
			"account_id":  accountID,
			"credit_hash": creditHash,
			"cycle_hash":  cycleHash,
		},
		TTL:        openAIAutoResetAttemptTTL,
		RequireKey: true,
	}, func(execCtx context.Context) (any, error) {
		resetResult, resetErr := s.quota.ResetCreditTargeted(execCtx, accountID, candidate.ID, redeemRequestID)
		if resetErr != nil {
			return nil, resetErr
		}
		if resetResult == nil {
			return nil, infraerrors.InternalServer("OPENAI_AUTO_RESET_EMPTY_RESULT", "automatic reset returned an empty result")
		}
		// 幂等表只保存脱敏结果，避免上游返回的卡 ID 被持久化到响应体列。
		return openAIAutoResetConsumeResult{Code: resetResult.Code, WindowsReset: resetResult.WindowsReset}, nil
	})
	if err != nil {
		// 另一个实例已持有同一周期的兑换时保持 resetting，等待下一轮读取同一
		// 幂等结果；不能把并发冲突误报成上游消费失败，更不能改选下一张卡。
		reason := infraerrors.Reason(err)
		if reason == infraerrors.Reason(ErrIdempotencyInProgress) || reason == infraerrors.Reason(ErrIdempotencyRetryBackoff) {
			return nil
		}
		s.recordAudit(accountID, assessment, available, "failed", 0, infraerrors.Reason(err))
		return s.failState(ctx, accountID, resetting, infraerrors.Reason(err), err)
	}

	consumeResult := decodeOpenAIAutoResetConsumeResult(result.Data)
	if strings.EqualFold(strings.TrimSpace(consumeResult.Code), "no_credit") {
		noCreditAt := time.Now().UTC().Format(time.RFC3339)
		noCredit := &OpenAIAutoResetCreditState{
			Status:            OpenAIAutoResetStatusNoCredit,
			TriggerWindow:     assessment.triggerWindow,
			AvailableCount:    0,
			CheckedAt:         noCreditAt,
			LastResultAt:      noCreditAt,
			ErrorCode:         "NO_RESET_CREDIT",
			AttemptCycleHash:  cycleHash,
			AttemptCreditHash: creditHash,
		}
		s.recordAudit(accountID, assessment, available, "no_credit", 0, noCredit.ErrorCode)
		return s.persistState(ctx, accountID, noCredit)
	}
	postCtx, cancelPost := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	post := RunOpenAIQuotaResetPostProcess(postCtx, accountID, s.quota, s.recoverer, s.accountRepo.GetByID, nil)
	cancelPost()
	if !post.AccountStateRecovered || post.WarningCode != "" {
		code := post.WarningCode
		if code == "" {
			code = OpenAIQuotaResetWarningAccountRecoveryFailed
		}
		s.recordAudit(accountID, assessment, available, "recovery_failed", consumeResult.WindowsReset, code)
		return s.failState(ctx, accountID, resetting, code, nil)
	}

	successAt := time.Now().UTC().Format(time.RFC3339)
	success := &OpenAIAutoResetCreditState{
		Status:            OpenAIAutoResetStatusSuccess,
		TriggerWindow:     assessment.triggerWindow,
		AvailableCount:    max(0, available-1),
		CheckedAt:         successAt,
		LastResultAt:      successAt,
		AttemptCycleHash:  cycleHash,
		AttemptCreditHash: creditHash,
	}
	if post.Quota != nil && post.Quota.RateLimitResetCredits != nil {
		success.AvailableCount = post.Quota.RateLimitResetCredits.AvailableCount
	}
	if leader && post.Quota != nil {
		s.armExpiryTimer(ctx, accountID, config, post.Quota)
	}
	if err := s.persistState(ctx, accountID, success); err != nil {
		return err
	}
	// 下一张卡已在提前窗口内时排不出定时器，打标记让后续扫描继续实查。
	if s.markExpiryDueIfExpiring(accountID, config, post.Quota) {
		s.Notify(accountID)
	}
	s.recordAudit(accountID, assessment, available, "success", consumeResult.WindowsReset, "")
	slog.Info("openai_auto_reset_credit_success",
		"account_id", accountID,
		"trigger_window", assessment.triggerWindow,
		"threshold_5h", assessment.threshold5h,
		"threshold_7d", assessment.threshold7d,
		"utilization_5h", assessment.utilization5h,
		"utilization_7d", assessment.utilization7d,
		"windows_reset", consumeResult.WindowsReset,
	)
	return nil
}

type openAIAutoResetConsumeResult struct {
	Code         string `json:"code"`
	WindowsReset int    `json:"windows_reset"`
}

func decodeOpenAIAutoResetConsumeResult(value any) openAIAutoResetConsumeResult {
	if typed, ok := value.(openAIAutoResetConsumeResult); ok {
		return typed
	}
	raw, _ := json.Marshal(value)
	var decoded openAIAutoResetConsumeResult
	_ = json.Unmarshal(raw, &decoded)
	return decoded
}

func (s *OpenAIQuotaAutoResetService) assessExtra(account *Account, config OpenAIAutoResetCreditConfig, now time.Time) openAIAutoResetAssessment {
	utilization5h, _ := resolveOpenAIQuotaUtilization(account.Extra, "5h", now)
	utilization7d, _ := resolveOpenAIQuotaUtilization(account.Extra, "7d", now)
	return s.buildAssessment(account, config, utilization5h, utilization7d, false)
}

func (s *OpenAIQuotaAutoResetService) assessUsage(usage *OpenAIQuotaUsage, account *Account, config OpenAIAutoResetCreditConfig, now time.Time) openAIAutoResetAssessment {
	updates := buildOpenAIAutoResetUsageUpdates(usage, now)
	utilization5h := readOpenAIQuotaUsedPercent(updates, "5h") / 100
	utilization7d := readOpenAIQuotaUsedPercent(updates, "7d") / 100
	// 到期判定只信上游 Date 头，本机时钟被改过时不得据此花卡。
	expiring := false
	if config.ExpiryEnabled && config.ExpiryLead > 0 {
		if usage.upstreamTime.IsZero() {
			slog.Warn("openai_auto_reset_upstream_time_missing", "account_id", account.ID)
		} else {
			expiring = openAIAutoResetCreditExpiring(openAIAutoResetCreditExpirations(usage.RateLimitResetCredits), config.ExpiryLead, usage.upstreamTime)
		}
	}
	return s.buildAssessment(account, config, utilization5h, utilization7d, expiring)
}

func (s *OpenAIQuotaAutoResetService) buildAssessment(account *Account, config OpenAIAutoResetCreditConfig, utilization5h, utilization7d float64, expiryReached bool) openAIAutoResetAssessment {
	assessment := openAIAutoResetAssessment{
		utilization5h: utilization5h,
		utilization7d: utilization7d,
		threshold5h:   config.Threshold5h,
		threshold7d:   config.Threshold7d,
	}
	reset5h := config.Enabled && utilization5h >= config.Threshold5h
	reset7d := config.Enabled && utilization7d >= config.Threshold7d
	assessment.thresholdReached = reset5h || reset7d
	assessment.resetReached = assessment.thresholdReached || expiryReached
	assessment.triggerWindow = joinOpenAIAutoResetWindows(reset5h, reset7d)
	if expiryReached {
		assessment.triggerWindow = openAIAutoResetWindowWithExpiry(assessment.triggerWindow)
	}

	pause5h, pause7d := resolveOpenAIQuotaAutoPauseThresholds(context.Background(), account)
	if s.settings != nil {
		pause5h, pause7d = resolveOpenAIQuotaAutoPauseThresholds(
			withOpenAIQuotaAutoPauseSettings(context.Background(), s.settings.GetOpenAIQuotaAutoPauseSettings(context.Background())),
			account,
		)
	}
	pauseReached5h := !resolveAccountExtraBool(account.Extra, "auto_pause_5h_disabled") && pause5h > 0 && utilization5h >= pause5h
	pauseReached7d := !resolveAccountExtraBool(account.Extra, "auto_pause_7d_disabled") && pause7d > 0 && utilization7d >= pause7d
	assessment.pauseReached = pauseReached5h || pauseReached7d || assessment.resetReached
	if assessment.triggerWindow == "" {
		assessment.triggerWindow = joinOpenAIAutoResetWindows(pauseReached5h, pauseReached7d)
	}
	return assessment
}

const openAIAutoResetExpiryWindow = "expiry"

func openAIAutoResetWindowWithExpiry(window string) string {
	if openAIAutoResetWindowHasExpiry(window) {
		return window
	}
	if window == "" {
		return openAIAutoResetExpiryWindow
	}
	return window + "+" + openAIAutoResetExpiryWindow
}

func openAIAutoResetWindowHasExpiry(window string) bool {
	for _, part := range strings.Split(window, "+") {
		if part == openAIAutoResetExpiryWindow {
			return true
		}
	}
	return false
}

func joinOpenAIAutoResetWindows(fiveHour, sevenDay bool) string {
	switch {
	case fiveHour && sevenDay:
		return "5h+7d"
	case fiveHour:
		return "5h"
	case sevenDay:
		return "7d"
	default:
		return ""
	}
}

func buildOpenAIAutoResetUsageUpdates(usage *OpenAIQuotaUsage, now time.Time) map[string]any {
	if usage == nil || usage.RateLimit == nil {
		return nil
	}
	rateLimit := usage.RateLimit
	snapshot := &OpenAICodexUsageSnapshot{UpdatedAt: now.UTC().Format(time.RFC3339)}
	applyWindow := func(window *OpenAIRateLimitWindow, primary bool) {
		if window == nil {
			return
		}
		used := window.UsedPercent
		resetAfter := int(window.ResetAfterSeconds)
		windowMinutes := int(window.LimitWindowSeconds / 60)
		if primary {
			snapshot.PrimaryUsedPercent = &used
			snapshot.PrimaryResetAfterSeconds = &resetAfter
			snapshot.PrimaryWindowMinutes = &windowMinutes
		} else {
			snapshot.SecondaryUsedPercent = &used
			snapshot.SecondaryResetAfterSeconds = &resetAfter
			snapshot.SecondaryWindowMinutes = &windowMinutes
		}
	}
	applyWindow(rateLimit.PrimaryWindow, true)
	applyWindow(rateLimit.SecondaryWindow, false)
	return buildCodexUsageExtraUpdates(snapshot, now)
}

func (s *OpenAIQuotaAutoResetService) persistFreshUsage(ctx context.Context, accountID int64, usage *OpenAIQuotaUsage, now time.Time) error {
	s.noteUpstreamClock(usage.upstreamTime, now)
	updates := buildOpenAIAutoResetUsageUpdates(usage, now)
	if len(updates) > 0 {
		if err := s.accountRepo.UpdateExtra(ctx, accountID, updates); err != nil {
			return err
		}
	}
	return s.quota.CacheResetCreditsSnapshot(ctx, accountID, usage.RateLimitResetCredits)
}

func selectOpenAIAutoResetCandidate(candidates []openAIAutoResetCreditCandidate, available int, previous *OpenAIAutoResetCreditState, cycleHash string) (openAIAutoResetCreditCandidate, error) {
	if available <= 0 {
		return openAIAutoResetCreditCandidate{}, infraerrors.Conflict("OPENAI_AUTO_RESET_NO_CREDIT", "no reset credit is available")
	}
	if len(candidates) < available {
		return openAIAutoResetCreditCandidate{}, infraerrors.Conflict("OPENAI_AUTO_RESET_CREDIT_DETAILS_INCOMPLETE", "reset credit details are incomplete")
	}
	for _, candidate := range candidates {
		if _, err := time.Parse(time.RFC3339, candidate.ExpiresAt); err != nil {
			return openAIAutoResetCreditCandidate{}, infraerrors.Conflict("OPENAI_AUTO_RESET_CREDIT_EXPIRY_INVALID", "reset credit expiration is invalid")
		}
	}
	sorted := append([]openAIAutoResetCreditCandidate(nil), candidates...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339, sorted[i].ExpiresAt)
		right, rightErr := time.Parse(time.RFC3339, sorted[j].ExpiresAt)
		if leftErr != nil {
			return false
		}
		if rightErr != nil {
			return true
		}
		return left.Before(right)
	})
	if previous != nil && previous.AttemptCycleHash == cycleHash && previous.AttemptCreditHash != "" {
		for _, candidate := range sorted {
			if shortOpenAIAutoResetHash(candidate.ID) == previous.AttemptCreditHash {
				if strings.TrimSpace(candidate.ID) == "" {
					break
				}
				return candidate, nil
			}
		}
		return openAIAutoResetCreditCandidate{}, infraerrors.Conflict("OPENAI_AUTO_RESET_ORIGINAL_CREDIT_UNAVAILABLE", "the original reset credit cannot be confirmed; refusing to switch credits")
	}
	if len(sorted) == 0 || strings.TrimSpace(sorted[0].ID) == "" {
		return openAIAutoResetCreditCandidate{}, infraerrors.Conflict("OPENAI_AUTO_RESET_CREDIT_ID_MISSING", "the earliest reset credit has no official id")
	}
	return sorted[0], nil
}

func openAIAutoResetCycleSeed(usage *OpenAIQuotaUsage) string {
	if usage == nil || usage.RateLimit == nil {
		return "5h:0|7d:0"
	}
	var fiveHour, sevenDay int64
	for _, window := range []*OpenAIRateLimitWindow{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow} {
		if window == nil {
			continue
		}
		resetAt := window.ResetAt
		if resetAt <= 0 {
			resetAt = usage.FetchedAt + window.ResetAfterSeconds
		}
		if window.LimitWindowSeconds <= 6*60*60 {
			fiveHour = resetAt
		} else {
			sevenDay = resetAt
		}
	}
	return fmt.Sprintf("5h:%d|7d:%d", fiveHour, sevenDay)
}

func shortOpenAIAutoResetHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:12])
}

func openAIAutoResetSnapshotStale(extra map[string]any, now time.Time) bool {
	if len(extra) == 0 {
		return true
	}
	raw, ok := extra["codex_usage_updated_at"]
	if !ok {
		return true
	}
	updatedAt, err := parseTime(fmt.Sprint(raw))
	return err != nil || now.Sub(updatedAt) >= openAIAutoResetSnapshotTTL
}

// noteUpstreamClock 记录上游 Date 与本机时钟的偏移；手动同步与调度实查都会调用。
func (s *OpenAIQuotaAutoResetService) noteUpstreamClock(upstreamTime, now time.Time) {
	if upstreamTime.IsZero() {
		return
	}
	s.upstreamClockOffset.Store(int64(upstreamTime.Sub(now)))
	s.upstreamClockKnown.Store(true)
}

func (s *OpenAIQuotaAutoResetService) upstreamNow(now time.Time) (time.Time, bool) {
	if !s.upstreamClockKnown.Load() {
		return time.Time{}, false
	}
	return now.Add(time.Duration(s.upstreamClockOffset.Load())), true
}

// expiryRetryWithinCachedCredit 判断缓存快照里是否还有值得重试的未过期卡。判定按上游时钟
// 折算；尚无可靠的上游参考时间时不以本机时钟终止重试，交给实查裁决。
func (s *OpenAIQuotaAutoResetService) expiryRetryWithinCachedCredit(extra map[string]any, lead time.Duration, now time.Time) bool {
	upstreamNow, ok := s.upstreamNow(now)
	if !ok {
		return true
	}
	return openAIAutoResetCreditExpiring(openAIAutoResetCachedCreditExpirations(extra), lead, upstreamNow)
}

// openAIAutoResetCachedCreditExpirations 读取账号 extra 里缓存的卡明细快照；
// 明细缺失时快照不会被覆盖，因此它能反映最后一次成功查询到的卡。
func openAIAutoResetCachedCreditExpirations(extra map[string]any) []string {
	raw, ok := extra[openaiQuotaResetCreditsKey]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var credits OpenAIRateLimitResetCredits
	if err := json.Unmarshal(encoded, &credits); err != nil {
		return nil
	}
	return openAIAutoResetCreditExpirations(&credits)
}

func openAIAutoResetCreditExpirations(credits *OpenAIRateLimitResetCredits) []string {
	if credits == nil {
		return nil
	}
	expirations := make([]string, 0, len(credits.Credits))
	for _, credit := range credits.Credits {
		expirations = append(expirations, credit.ExpiresAt)
	}
	return expirations
}

func openAIAutoResetEarliestExpiryDelay(expirations []string, lead time.Duration, now time.Time) (time.Duration, bool) {
	if lead <= 0 {
		return 0, false
	}
	var earliest time.Time
	for _, expiresAt := range expirations {
		expiry, err := time.Parse(time.RFC3339, strings.TrimSpace(expiresAt))
		if err != nil || !expiry.After(now) {
			continue
		}
		if earliest.IsZero() || expiry.Before(earliest) {
			earliest = expiry
		}
	}
	if earliest.IsZero() {
		return 0, false
	}
	return earliest.Sub(now) - lead, true
}

func openAIAutoResetCreditExpiring(expirations []string, lead time.Duration, now time.Time) bool {
	delay, ok := openAIAutoResetEarliestExpiryDelay(expirations, lead, now)
	return ok && delay <= 0
}

// 定时器到点只打标记并入队，是否用卡由实查上游后的最终校验决定。
// 计划触发时刻同时写入账号 extra 供列表显示；没有定时器时清空。
func (s *OpenAIQuotaAutoResetService) armExpiryTimer(ctx context.Context, accountID int64, config OpenAIAutoResetCreditConfig, usage *OpenAIQuotaUsage) {
	s.disarmExpiryTimer(accountID)
	var expiryAt any
	if config.ExpiryEnabled && config.ExpiryLead > 0 && usage != nil && !usage.upstreamTime.IsZero() {
		delay, ok := openAIAutoResetEarliestExpiryDelay(openAIAutoResetCreditExpirations(usage.RateLimitResetCredits), config.ExpiryLead, usage.upstreamTime)
		if ok && delay > 0 {
			s.expiryTimers.Store(accountID, time.AfterFunc(delay, func() {
				s.expiryDue.Store(accountID, struct{}{})
				s.Notify(accountID)
			}))
			expiryAt = usage.upstreamTime.Add(delay).UTC().Format(time.RFC3339)
		}
	}
	s.persistExpiryAt(ctx, accountID, expiryAt)
}

// markExpiryDueIfExpiring 处理已进入提前窗口、排不出定时器的卡：按到点语义打标记，
// 是否用卡仍由实查后的最终校验决定。
func (s *OpenAIQuotaAutoResetService) markExpiryDueIfExpiring(accountID int64, config OpenAIAutoResetCreditConfig, usage *OpenAIQuotaUsage) bool {
	if usage == nil || usage.RateLimitResetCredits == nil || usage.upstreamTime.IsZero() || !config.ExpiryEnabled || config.ExpiryLead <= 0 {
		return false
	}
	if !openAIAutoResetCreditExpiring(openAIAutoResetCreditExpirations(usage.RateLimitResetCredits), config.ExpiryLead, usage.upstreamTime) {
		return false
	}
	s.expiryDue.Store(accountID, struct{}{})
	return true
}

func (s *OpenAIQuotaAutoResetService) disarmExpiryTimer(accountID int64) bool {
	value, ok := s.expiryTimers.LoadAndDelete(accountID)
	if !ok {
		return false
	}
	if timer, ok := value.(*time.Timer); ok {
		timer.Stop()
	}
	return true
}

func (s *OpenAIQuotaAutoResetService) persistExpiryAt(ctx context.Context, accountID int64, expiryAt any) {
	if err := s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{OpenAIAutoResetCreditExpiryAtExtraKey: expiryAt}); err != nil {
		slog.Warn("openai_auto_reset_expiry_at_persist_failed", "account_id", accountID, "error", err)
	}
}

func openAIAutoResetStateFromExtra(extra map[string]any) *OpenAIAutoResetCreditState {
	if len(extra) == 0 {
		return nil
	}
	raw, ok := extra[OpenAIAutoResetCreditStateExtraKey]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var state OpenAIAutoResetCreditState
	if err := json.Unmarshal(encoded, &state); err != nil || state.Status == "" {
		return nil
	}
	return &state
}

func openAIAutoResetStateStale(state *OpenAIAutoResetCreditState, now time.Time) bool {
	if state == nil || state.CheckedAt == "" {
		return true
	}
	checkedAt, err := time.Parse(time.RFC3339, state.CheckedAt)
	return err != nil || now.Sub(checkedAt) >= openAIAutoResetSnapshotTTL
}

func stateAvailableCount(state *OpenAIAutoResetCreditState) int {
	if state == nil {
		return 0
	}
	return state.AvailableCount
}

func copyOpenAIAutoResetAttempt(target, source *OpenAIAutoResetCreditState) {
	if target == nil || source == nil {
		return
	}
	target.AttemptCycleHash = source.AttemptCycleHash
	target.AttemptCreditHash = source.AttemptCreditHash
}

func (s *OpenAIQuotaAutoResetService) persistState(ctx context.Context, accountID int64, state *OpenAIAutoResetCreditState) error {
	if state == nil {
		return nil
	}
	return s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{OpenAIAutoResetCreditStateExtraKey: state})
}

func (s *OpenAIQuotaAutoResetService) failState(ctx context.Context, accountID int64, state *OpenAIAutoResetCreditState, code string, cause error) error {
	if state == nil {
		state = &OpenAIAutoResetCreditState{}
	}
	if strings.TrimSpace(code) == "" {
		code = "OPENAI_AUTO_RESET_FAILED"
	}
	state.Status = OpenAIAutoResetStatusFailed
	state.ErrorCode = code
	state.LastResultAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.persistState(ctx, accountID, state); err != nil {
		return err
	}
	slog.Warn("openai_auto_reset_credit_failed",
		"account_id", accountID,
		"trigger_window", state.TriggerWindow,
		"available_count", state.AvailableCount,
		"error_code", code,
	)
	if cause != nil {
		return cause
	}
	return infraerrors.Conflict(code, "automatic reset credit operation failed")
}

func (s *OpenAIQuotaAutoResetService) recordAudit(accountID int64, assessment openAIAutoResetAssessment, available int, resultCode string, windowsReset int, errorCode string) {
	if s.audit == nil {
		return
	}
	statusCode := http.StatusOK
	if resultCode != "success" {
		statusCode = http.StatusConflict
	}
	s.audit.Record(&AuditLog{
		ActorEmail: "system",
		ActorRole:  "system",
		AuthMethod: "system",
		Action:     "system.openai.reset_credit.auto",
		Method:     "SYSTEM",
		Path:       fmt.Sprintf("/system/openai/accounts/%d/auto-reset-credit", accountID),
		StatusCode: statusCode,
		Extra: map[string]any{
			"account_id":      accountID,
			"trigger_window":  assessment.triggerWindow,
			"threshold_5h":    assessment.threshold5h,
			"threshold_7d":    assessment.threshold7d,
			"utilization_5h":  assessment.utilization5h,
			"utilization_7d":  assessment.utilization7d,
			"available_count": available,
			"result_code":     resultCode,
			"windows_reset":   windowsReset,
			"error_code":      errorCode,
		},
	})
}

var openAIAutoResetNotifierRegistry struct {
	sync.RWMutex
	service *OpenAIQuotaAutoResetService
}

func setOpenAIAutoResetNotifier(service *OpenAIQuotaAutoResetService) {
	openAIAutoResetNotifierRegistry.Lock()
	openAIAutoResetNotifierRegistry.service = service
	openAIAutoResetNotifierRegistry.Unlock()
}

func clearOpenAIAutoResetNotifier(service *OpenAIQuotaAutoResetService) {
	openAIAutoResetNotifierRegistry.Lock()
	if openAIAutoResetNotifierRegistry.service == service {
		openAIAutoResetNotifierRegistry.service = nil
	}
	openAIAutoResetNotifierRegistry.Unlock()
}

func notifyOpenAIAutoReset(accountID int64) {
	openAIAutoResetNotifierRegistry.RLock()
	service := openAIAutoResetNotifierRegistry.service
	openAIAutoResetNotifierRegistry.RUnlock()
	if service != nil {
		service.Notify(accountID)
	}
}

// NotifyOpenAIAutoResetCredit 供额度查询入口发送轻量信号；不执行同步上游请求。
func NotifyOpenAIAutoResetCredit(accountID int64) {
	notifyOpenAIAutoReset(accountID)
}

// SyncOpenAIAutoResetCredit 把手动查询或手动用卡刚取到的上游结果交给调度器，不再额外请求上游。
func SyncOpenAIAutoResetCredit(ctx context.Context, accountID int64, usage *OpenAIQuotaUsage) {
	openAIAutoResetNotifierRegistry.RLock()
	service := openAIAutoResetNotifierRegistry.service
	openAIAutoResetNotifierRegistry.RUnlock()
	if service != nil {
		service.syncCreditUsage(ctx, accountID, usage)
	}
}

// syncCreditUsage 让手动路径的实查结果顶替本轮计划取卡：领导实例据此重排到期定时器，
// 避免列表继续显示已消耗卡的计划时刻；非领导实例不设定时器，留给领导实例下一次
// 计划取卡重排。运行态一律不写：它与后台评估的读改写会互相覆盖尝试指纹，
// 而张数与到期时刻已有独立标签展示，留给调度器实查后统一落库。
func (s *OpenAIQuotaAutoResetService) syncCreditUsage(ctx context.Context, accountID int64, usage *OpenAIQuotaUsage) {
	if s == nil || usage == nil || usage.RateLimitResetCredits == nil || usage.upstreamTime.IsZero() {
		return
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil || account.IsShadow() {
		return
	}
	config := ResolveOpenAIAutoResetCreditConfig(account)
	if !config.Active() {
		return
	}
	s.noteUpstreamClock(usage.upstreamTime, time.Now())
	if s.isSchedulerLeader() {
		s.fetchStates.Store(accountID, openAIAutoResetFetchState{nextAt: time.Now().Add(openAIAutoResetCreditRefreshInterval), fetched: true, config: config})
		s.armExpiryTimer(ctx, accountID, config, usage)
	}
	notify := s.markExpiryDueIfExpiring(accountID, config, usage)
	// 网关暂停决策读运行态里的卡数；与刚查到的不一致时让调度器立即实查校正，而不是等 24 小时。
	if state := openAIAutoResetStateFromExtra(account.Extra); state != nil && state.AvailableCount != usage.RateLimitResetCredits.AvailableCount {
		s.stateDue.Store(accountID, struct{}{})
		notify = true
	}
	if notify {
		s.Notify(accountID)
	}
}

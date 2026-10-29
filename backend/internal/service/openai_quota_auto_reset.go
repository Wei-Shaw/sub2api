package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
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
	// 查询阶段失败后的最短重试间隔。调度热路径和扫描都会触发评估，不设间隔时上游
	// 故障期间同一账号会被十几秒重查一次；用卡阶段的失败不受此限，超时后仍立即用
	// 同一幂等键重试。
	openAIAutoResetQueryFailureRetryAfter = time.Minute
	// 调度热路径每次过滤候选都会评估暂停，对同一账号的通知按此冷却合并；
	// 后台每分钟全量扫描兜底，冷却不会让账号漏检。
	openAIAutoResetSchedulerNotifyCooldown = 30 * time.Second
)

const (
	OpenAIAutoResetStatusChecking    = "checking"
	OpenAIAutoResetStatusAvailable   = "available"
	OpenAIAutoResetStatusResetting   = "resetting"
	OpenAIAutoResetStatusSuccess     = "success"
	OpenAIAutoResetStatusNoCredit    = "no_credit"
	OpenAIAutoResetStatusFailed      = "failed"
	OpenAIAutoResetStatusNotConsumed = "not_consumed"
)

// OpenAIAutoResetCreditState 是可返回管理端的脱敏运行态。Attempt* 仅保存不可逆
// 指纹，用于重启后拒绝切换到另一张卡；不会保存卡 ID 或兑换 ID。
type OpenAIAutoResetCreditState struct {
	Status              string                         `json:"status"`
	TriggerWindow       string                         `json:"trigger_window,omitempty"`
	AvailableCount      int                            `json:"available_count"`
	CheckedAt           string                         `json:"checked_at,omitempty"`
	LastResultAt        string                         `json:"last_result_at,omitempty"`
	ErrorCode           string                         `json:"error_code,omitempty"`
	AttemptCycleHash    string                         `json:"attempt_cycle_hash,omitempty"`
	AttemptCreditHash   string                         `json:"attempt_credit_hash,omitempty"`
	TriggerReason       string                         `json:"trigger_reason,omitempty"`
	CreditExpiresAt     string                         `json:"credit_expires_at,omitempty"`
	CreditsCheckedAt    string                         `json:"credits_checked_at,omitempty"`
	NextCreditExpiresAt string                         `json:"next_credit_expires_at,omitempty"`
	AttemptKeyVersion   string                         `json:"attempt_key_version,omitempty"`
	Attempts            []OpenAIAutoResetCreditAttempt `json:"attempts,omitempty"`
	PostProcessPending  bool                           `json:"post_process_pending,omitempty"`
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
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return
	case <-timer.C:
		s.scanEnabledAccounts(s.ctx)
	}
	ticker := time.NewTicker(openAIAutoResetScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
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
	for page := 1; ; page++ {
		accounts, pageInfo, err := s.accountRepo.ListWithFilters(ctx, pagination.PaginationParams{
			Page: page, PageSize: openAIAutoResetBatchSize,
		}, PlatformOpenAI, AccountTypeOAuth, "", "", 0, "")
		if err != nil {
			slog.Warn("openai_auto_reset_scan_failed", "page", page, "error", err)
			return
		}
		for i := range accounts {
			account := &accounts[i]
			// 到期兜底属于账号用卡，与管理端状态和调度开关无关；旧用量路径仍保留原条件。
			if openAIAutoResetEnabledForAccount(account, ResolveOpenAIAutoResetCreditConfig(account)) {
				s.Notify(account.ID)
			}
		}
		if len(accounts) < openAIAutoResetBatchSize || pageInfo == nil || page >= pageInfo.Pages {
			return
		}
	}
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
	triggerWindow   string
	triggerReason   string
	creditExpiresAt string
	resetReached    bool
	pauseReached    bool
	utilization5h   float64
	utilization7d   float64
	threshold5h     float64
	threshold7d     float64
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
	if !openAIAutoResetEnabledForAccount(account, config) {
		return nil
	}

	now := time.Now()
	assessment := s.assessExtra(account, config, now)
	usageEnabled := openAIAutoResetUsageEnabled(account, config)
	assessment.resetReached = usageEnabled && assessment.resetReached
	state := openAIAutoResetStateFromExtra(account.Extra)
	if state == nil {
		state = &OpenAIAutoResetCreditState{}
	}
	openAIAutoResetMigrateAttempt(state)
	pruneOpenAIAutoResetAttempts(state, now)
	// 已确认消费的本地恢复不依赖上游当前可查询；共享流程会先解除本地限流，再刷新额度。
	if resolved, resolveErr := s.resolveStoredAttempt(ctx, accountID, state, nil, assessment); resolved || resolveErr != nil {
		return resolveErr
	}
	needsQuery := (usageEnabled && (openAIAutoResetSnapshotStale(account.Extra, now) ||
		(assessment.resetReached && !openAIAutoResetNoCreditConfirmed(state, now)))) ||
		openAIAutoResetExpiryQueryDue(state, config, now) || openAIAutoResetHasPending(state)
	if usageEnabled && assessment.pauseReached && !assessment.resetReached {
		needsQuery = needsQuery || state.Status == "" || state.Status == OpenAIAutoResetStatusChecking || state.Status == OpenAIAutoResetStatusFailed || openAIAutoResetStateStale(state, now)
	}
	if needsQuery && openAIAutoResetQueryFailureBackoffActive(state, now) {
		needsQuery = false
	}
	if !needsQuery {
		if usageEnabled && !assessment.pauseReached && state.TriggerWindow != "" && !openAIAutoResetHasPending(state) {
			state.TriggerWindow = ""
			state.ErrorCode = ""
			state.CheckedAt = now.UTC().Format(time.RFC3339)
			state.Status = OpenAIAutoResetStatusNoCredit
			if state.AvailableCount > 0 {
				state.Status = OpenAIAutoResetStatusAvailable
			}
			return s.persistState(ctx, accountID, state)
		}
		return nil
	}

	state.Status = OpenAIAutoResetStatusChecking
	state.CheckedAt = now.UTC().Format(time.RFC3339)
	if err := s.persistState(ctx, accountID, state); err != nil {
		return err
	}
	usage, err := s.quota.QueryUsage(ctx, accountID)
	if err != nil || usage == nil {
		return s.failState(ctx, accountID, state, "RESET_CREDIT_QUERY_FAILED", err)
	}
	if err := s.persistFreshUsage(ctx, accountID, usage, now); err != nil {
		return s.failState(ctx, accountID, state, "USAGE_SNAPSHOT_WRITE_FAILED", err)
	}
	if usage.RateLimitResetCredits == nil {
		return s.failState(ctx, accountID, state, "RESET_CREDIT_DETAILS_UNAVAILABLE", nil)
	}
	state.CreditsCheckedAt = time.Now().UTC().Format(time.RFC3339)
	state.AvailableCount = usage.RateLimitResetCredits.AvailableCount
	openAIAutoResetSetNextExpiry(state, usage.autoResetCandidates, now)

	// 关闭到期兜底、缩短提前时间或转换账号类型都必须在真实消费前生效。
	account, err = s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return err
	}
	config = ResolveOpenAIAutoResetCreditConfig(account)
	if !openAIAutoResetEnabledForAccount(account, config) {
		return nil
	}
	assessment = s.assessUsage(usage, account, config, time.Now())
	assessment.resetReached = openAIAutoResetUsageEnabled(account, config) && assessment.resetReached
	if resolved, resolveErr := s.resolveStoredAttempt(ctx, accountID, state, usage, assessment); resolved || resolveErr != nil {
		return resolveErr
	}
	candidate, reason, selectErr := selectOpenAIAutoResetEligibleCandidate(usage, state, config, assessment.resetReached, time.Now())
	if selectErr != nil {
		return s.failState(ctx, accountID, state, infraerrors.Reason(selectErr), selectErr)
	}
	if candidate == nil {
		if openAIAutoResetHasPending(state) {
			return s.failState(ctx, accountID, state, "RESET_RESULT_UNCONFIRMED", nil)
		}
		// 已确认未消费结果在后台扫描后仍保留，不能降级显示成一次成功重置。
		if state.Status == OpenAIAutoResetStatusChecking {
			state.Status = OpenAIAutoResetStatusAvailable
			state.ErrorCode = ""
			if state.AvailableCount <= 0 {
				state.Status = OpenAIAutoResetStatusNoCredit
				if assessment.resetReached {
					state.ErrorCode = "NO_RESET_CREDIT"
				}
			}
			if len(state.Attempts) > 0 && state.Attempts[len(state.Attempts)-1].Outcome == openAIAutoResetAttemptNotConsumed {
				code := strings.ToLower(strings.TrimSpace(state.Attempts[len(state.Attempts)-1].Code))
				if code == "nothing_to_reset" {
					state.Status = OpenAIAutoResetStatusNotConsumed
					state.ErrorCode = "NOTHING_TO_RESET"
				} else if code == "no_credit" && state.AvailableCount <= 0 {
					state.ErrorCode = "NO_RESET_CREDIT"
				}
			}
		}
		return s.persistState(ctx, accountID, state)
	}
	assessment.triggerReason = reason
	assessment.creditExpiresAt = candidate.ExpiresAt
	if reason == openAIAutoResetTriggerExpiry {
		assessment.triggerWindow = ""
	}
	creditHash := shortOpenAIAutoResetHash(candidate.ID)
	attempt := openAIAutoResetFindAttempt(state, creditHash)
	if attempt == nil {
		state.Attempts = append(state.Attempts, OpenAIAutoResetCreditAttempt{CreditHash: creditHash})
		attempt = &state.Attempts[len(state.Attempts)-1]
	}
	cycleHash := shortOpenAIAutoResetHash(openAIAutoResetCycleSeed(usage))
	if attempt.Outcome == openAIAutoResetAttemptNotConsumed {
		// 只有明确的 nothing_to_reset 才允许后续真实达阈值开启确定性新代次。
		attempt.Generation = "usage:" + cycleHash
		attempt.LegacyCycleHash = ""
	}
	attempt.Outcome = openAIAutoResetAttemptPending
	attempt.CreditExpiresAt = candidate.ExpiresAt
	attempt.TriggerReason = reason
	attempt.QuotaCycleHash = cycleHash
	state.Status = OpenAIAutoResetStatusResetting
	state.TriggerReason = reason
	state.TriggerWindow = assessment.triggerWindow
	state.CreditExpiresAt = candidate.ExpiresAt
	state.AttemptCreditHash = creditHash
	state.AttemptCycleHash = attempt.LegacyCycleHash
	state.AttemptKeyVersion = openAIAutoResetCardKeyVersion
	state.ErrorCode = ""
	if err := s.persistState(ctx, accountID, state); err != nil {
		return err
	}

	account, err = s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return err
	}
	config = ResolveOpenAIAutoResetCreditConfig(account)
	if !openAIAutoResetEnabledForAccount(account, config) {
		return nil
	}
	finalAssessment := s.assessUsage(usage, account, config, time.Now())
	usageReached := openAIAutoResetUsageEnabled(account, config) && finalAssessment.resetReached
	if attempt.Generation != "" && !usageReached {
		return nil
	}
	if openAIAutoResetCandidateReason(*candidate, config, usageReached, time.Now()) == "" {
		return nil
	}
	stableKey := openAIAutoResetAttemptKey(accountID, *attempt)
	redeemRequestID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(stableKey)).String()
	payload := map[string]any{"account_id": accountID, "credit_hash": creditHash, "generation": attempt.Generation}
	if attempt.LegacyCycleHash != "" {
		payload = map[string]any{"account_id": accountID, "credit_hash": creditHash, "cycle_hash": attempt.LegacyCycleHash}
	}
	ttl := openAIAutoResetAttemptTTL
	if expiry, parseErr := time.Parse(time.RFC3339, candidate.ExpiresAt); parseErr == nil {
		remaining := time.Until(expiry)
		creditTTL := time.Duration(1<<63 - 1)
		if remaining <= creditTTL-24*time.Hour {
			creditTTL = remaining + 24*time.Hour
		}
		if creditTTL > ttl {
			ttl = creditTTL
		}
	}
	result, err := s.idempotency.Execute(ctx, IdempotencyExecuteOptions{
		Scope: "openai_auto_reset_credit", ActorScope: fmt.Sprintf("account:%d", accountID),
		Method: http.MethodPost, Route: "/system/openai/reset-credit/auto",
		IdempotencyKey: stableKey, Payload: payload, TTL: ttl, RequireKey: true,
		// 原卡与兑换 ID 都固定，进程在请求后退出时可回收过期租约并按原身份确认结果。
		ReclaimExpiredProcessing: true,
	}, func(execCtx context.Context) (any, error) {
		resetResult, resetErr := s.quota.ResetCreditTargeted(execCtx, accountID, candidate.ID, redeemRequestID)
		if resetErr != nil {
			return nil, resetErr
		}
		if resetResult == nil {
			return nil, infraerrors.InternalServer("OPENAI_AUTO_RESET_EMPTY_RESULT", "automatic reset returned an empty result")
		}
		consumeResult := openAIAutoResetConsumeResult{Code: resetResult.Code, WindowsReset: resetResult.WindowsReset}
		attempt.Code = resetResult.Code
		if openAIAutoResetConsumeOutcome(consumeResult) == openAIAutoResetAttemptPending {
			// 未确认的 HTTP 200 也应保持可重试；保存为成功会永久回放旧未知结果，无法原 ID 再确认。
			return nil, infraerrors.Conflict("RESET_RESULT_UNCONFIRMED", "automatic reset result is not confirmed")
		}
		// 幂等表只保存脱敏结果，卡 ID 不进入响应体列或运行态。
		return consumeResult, nil
	})
	if err != nil {
		reason := infraerrors.Reason(err)
		if reason == infraerrors.Reason(ErrIdempotencyInProgress) || reason == infraerrors.Reason(ErrIdempotencyRetryBackoff) {
			return nil
		}
		s.recordAudit(accountID, assessment, state.AvailableCount, "failed", 0, reason)
		return s.failState(ctx, accountID, state, reason, err)
	}
	return s.finishAttempt(ctx, accountID, state, usage, assessment, decodeOpenAIAutoResetConsumeResult(result.Data))
}

func (s *OpenAIQuotaAutoResetService) finishAttempt(ctx context.Context, accountID int64, state *OpenAIAutoResetCreditState, usage *OpenAIQuotaUsage, assessment openAIAutoResetAssessment, result openAIAutoResetConsumeResult) error {
	attempt := openAIAutoResetFindAttempt(state, state.AttemptCreditHash)
	if attempt == nil {
		return s.failState(ctx, accountID, state, "RESET_ATTEMPT_STATE_MISSING", nil)
	}
	attempt.Outcome = openAIAutoResetConsumeOutcome(result)
	attempt.Code = result.Code
	state.LastResultAt = time.Now().UTC().Format(time.RFC3339)
	if attempt.Outcome == openAIAutoResetAttemptPending {
		return s.failState(ctx, accountID, state, "RESET_RESULT_UNCONFIRMED", nil)
	}
	if usage != nil {
		openAIAutoResetSetNextExpiry(state, usage.autoResetCandidates, time.Now())
	}
	if attempt.Outcome == openAIAutoResetAttemptNotConsumed {
		state.Status = OpenAIAutoResetStatusNotConsumed
		state.ErrorCode = strings.ToUpper(strings.TrimSpace(result.Code))
		if strings.EqualFold(strings.TrimSpace(result.Code), "no_credit") {
			// no_credit 是上游确认没有可用卡，不能沿用消费前缓存数量或声称原卡仍保留。
			state.Status = OpenAIAutoResetStatusNoCredit
			state.AvailableCount = 0
			state.NextCreditExpiresAt = ""
			state.ErrorCode = "NO_RESET_CREDIT"
		}
		// 上游 nothing_to_reset 表示卡仍可用；只刷新卡缓存，不恢复账号调度状态。
		if err := s.persistState(ctx, accountID, state); err != nil {
			return err
		}
		s.recordAudit(accountID, assessment, state.AvailableCount, "not_consumed", result.WindowsReset, state.ErrorCode)
		slog.Info("openai_auto_reset_credit_not_consumed", "account_id", accountID, "trigger_reason", state.TriggerReason, "code", result.Code)
		return nil
	}
	// 先持久化已消费事实，再做可失败的本地恢复，防止刷新失败被当成可以另用一张卡。
	state.Status = OpenAIAutoResetStatusSuccess
	// 成功消费与本地恢复之间可能进程退出，必须先把待恢复事实一起落盘。
	state.PostProcessPending = true
	if err := s.persistState(ctx, accountID, state); err != nil {
		return err
	}
	postCtx, cancelPost := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	post := RunOpenAIQuotaResetPostProcess(postCtx, accountID, s.quota, s.recoverer, s.accountRepo.GetByID)
	cancelPost()
	if !post.AccountStateRecovered || post.WarningCode != "" {
		code := post.WarningCode
		if code == "" {
			code = OpenAIQuotaResetWarningAccountRecoveryFailed
		}
		s.recordAudit(accountID, assessment, state.AvailableCount, "recovery_failed", result.WindowsReset, code)
		return s.failState(ctx, accountID, state, code, nil)
	}
	if post.Quota != nil && post.Quota.RateLimitResetCredits != nil {
		state.AvailableCount = post.Quota.RateLimitResetCredits.AvailableCount
		openAIAutoResetSetNextExpiry(state, post.Quota.autoResetCandidates, time.Now())
	}
	state.ErrorCode = ""
	state.PostProcessPending = false
	if err := s.persistState(ctx, accountID, state); err != nil {
		return err
	}
	s.recordAudit(accountID, assessment, state.AvailableCount, "success", result.WindowsReset, "")
	slog.Info("openai_auto_reset_credit_success", "account_id", accountID, "trigger_reason", state.TriggerReason, "credit_expires_at", state.CreditExpiresAt, "windows_reset", result.WindowsReset)
	return nil
}

// resolveStoredAttempt 处理进程在消费成功后、保存账号运行态前退出的情况。即使原卡已
// 从可用列表消失，也先读取原幂等结果完成恢复，不能凭“找不到卡”就切换下一张。
func (s *OpenAIQuotaAutoResetService) resolveStoredAttempt(ctx context.Context, accountID int64, state *OpenAIAutoResetCreditState, usage *OpenAIQuotaUsage, assessment openAIAutoResetAssessment) (bool, error) {
	if s.idempotency == nil || s.idempotency.repo == nil {
		return false, nil
	}
	for _, attempt := range state.Attempts {
		recoveryPending := attempt.Outcome == openAIAutoResetAttemptConsumed && attempt.CreditHash == state.AttemptCreditHash &&
			(state.PostProcessPending || state.ErrorCode == OpenAIQuotaResetWarningCacheRefreshFailed || state.ErrorCode == OpenAIQuotaResetWarningAccountRecoveryFailed || state.ErrorCode == OpenAIQuotaResetWarningAccountRefreshFailed)
		if attempt.Outcome != openAIAutoResetAttemptPending && !recoveryPending {
			continue
		}
		record, err := s.idempotency.repo.GetByScopeAndKeyHash(ctx, "openai_auto_reset_credit", HashIdempotencyKey(openAIAutoResetAttemptKey(accountID, attempt)))
		if err != nil {
			return false, err
		}
		if record == nil || record.Status != IdempotencyStatusSucceeded {
			return false, nil
		}
		data, err := s.idempotency.decodeStoredResponse(record.ResponseBody)
		if err != nil {
			return false, err
		}
		consumeResult := decodeOpenAIAutoResetConsumeResult(data)
		if usage == nil && openAIAutoResetConsumeOutcome(consumeResult) != openAIAutoResetAttemptConsumed {
			// 查询前只补已确认消费的恢复；未确认或未消费结果仍需走原来的实时查询流程。
			return false, nil
		}
		state.AttemptCreditHash = attempt.CreditHash
		state.CreditExpiresAt = attempt.CreditExpiresAt
		state.TriggerReason = attempt.TriggerReason
		assessment.triggerReason = attempt.TriggerReason
		assessment.creditExpiresAt = attempt.CreditExpiresAt
		return true, s.finishAttempt(ctx, accountID, state, usage, assessment, consumeResult)
	}
	return false, nil
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
	return s.buildAssessment(account, config, utilization5h, utilization7d)
}

func (s *OpenAIQuotaAutoResetService) assessUsage(usage *OpenAIQuotaUsage, account *Account, config OpenAIAutoResetCreditConfig, now time.Time) openAIAutoResetAssessment {
	updates := buildOpenAIAutoResetUsageUpdates(usage, now)
	utilization5h := readOpenAIQuotaUsedPercent(updates, "5h") / 100
	utilization7d := readOpenAIQuotaUsedPercent(updates, "7d") / 100
	return s.buildAssessment(account, config, utilization5h, utilization7d)
}

func (s *OpenAIQuotaAutoResetService) buildAssessment(account *Account, config OpenAIAutoResetCreditConfig, utilization5h, utilization7d float64) openAIAutoResetAssessment {
	assessment := openAIAutoResetAssessment{
		utilization5h: utilization5h,
		utilization7d: utilization7d,
		threshold5h:   config.Threshold5h,
		threshold7d:   config.Threshold7d,
	}
	reset5h := utilization5h >= config.Threshold5h
	reset7d := utilization7d >= config.Threshold7d
	assessment.resetReached = reset5h || reset7d
	assessment.triggerWindow = joinOpenAIAutoResetWindows(reset5h, reset7d)

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
	sorted, err := orderOpenAIAutoResetCandidates(candidates, available)
	if err != nil {
		return openAIAutoResetCreditCandidate{}, err
	}
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

// orderOpenAIAutoResetCandidates 保持现有阈值入口与到期入口使用相同的完整性与排序规则。
func orderOpenAIAutoResetCandidates(candidates []openAIAutoResetCreditCandidate, available int) ([]openAIAutoResetCreditCandidate, error) {
	if len(candidates) < available {
		return nil, infraerrors.Conflict("OPENAI_AUTO_RESET_CREDIT_DETAILS_INCOMPLETE", "reset credit details are incomplete")
	}
	for _, candidate := range candidates {
		if _, err := time.Parse(time.RFC3339, candidate.ExpiresAt); err != nil {
			return nil, infraerrors.Conflict("OPENAI_AUTO_RESET_CREDIT_EXPIRY_INVALID", "reset credit expiration is invalid")
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
	return sorted, nil
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

// openAIAutoResetNoCreditConfirmed 报告账号在状态有效期内已被确认无卡。
func openAIAutoResetNoCreditConfirmed(state *OpenAIAutoResetCreditState, now time.Time) bool {
	return state != nil && state.Status == OpenAIAutoResetStatusNoCredit && !openAIAutoResetStateStale(state, now)
}

// openAIAutoResetQueryFailureBackoffActive 报告查询阶段刚失败、仍在重试间隔内。
// 只覆盖查询、快照写入、卡明细缺失三类失败；用卡阶段的失败需要立即按原幂等键重试。
func openAIAutoResetQueryFailureBackoffActive(state *OpenAIAutoResetCreditState, now time.Time) bool {
	if state == nil || state.Status != OpenAIAutoResetStatusFailed {
		return false
	}
	switch state.ErrorCode {
	case "RESET_CREDIT_QUERY_FAILED", "USAGE_SNAPSHOT_WRITE_FAILED", "RESET_CREDIT_DETAILS_UNAVAILABLE":
	default:
		return false
	}
	failedAt, err := time.Parse(time.RFC3339, state.LastResultAt)
	if err != nil {
		return false
	}
	elapsed := now.Sub(failedAt)
	return elapsed >= 0 && elapsed < openAIAutoResetQueryFailureRetryAfter
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
		"trigger_reason", state.TriggerReason,
		"credit_expires_at", state.CreditExpiresAt,
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
			"account_id":        accountID,
			"trigger_window":    assessment.triggerWindow,
			"trigger_reason":    assessment.triggerReason,
			"credit_expires_at": assessment.creditExpiresAt,
			"threshold_5h":      assessment.threshold5h,
			"threshold_7d":      assessment.threshold7d,
			"utilization_5h":    assessment.utilization5h,
			"utilization_7d":    assessment.utilization7d,
			"available_count":   available,
			"result_code":       resultCode,
			"windows_reset":     windowsReset,
			"error_code":        errorCode,
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

// openAIAutoResetSchedulerNotifiedAt 记录调度热路径最近一次为某账号发出通知的时间。
var openAIAutoResetSchedulerNotifiedAt sync.Map // accountID(int64) -> time.Time

// notifyOpenAIAutoResetFromScheduler 供调度候选过滤使用。候选过滤按请求逐账号执行，
// 每条通知都会让后台读一次账号，不做冷却时无卡或待用卡的账号会持续占满后台协程。
func notifyOpenAIAutoResetFromScheduler(accountID int64) {
	notifyOpenAIAutoResetFromSchedulerAt(accountID, time.Now())
}

func notifyOpenAIAutoResetFromSchedulerAt(accountID int64, now time.Time) bool {
	if accountID <= 0 {
		return false
	}
	if last, ok := openAIAutoResetSchedulerNotifiedAt.Load(accountID); ok {
		if lastAt, ok := last.(time.Time); ok {
			elapsed := now.Sub(lastAt)
			if elapsed >= 0 && elapsed < openAIAutoResetSchedulerNotifyCooldown {
				return false
			}
		}
	}
	openAIAutoResetSchedulerNotifiedAt.Store(accountID, now)
	notifyOpenAIAutoReset(accountID)
	return true
}

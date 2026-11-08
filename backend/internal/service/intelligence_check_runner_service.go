package service

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
)

const (
	// 每分钟 tick 一次：真正的到期判定在 RunDue 里按账号级/全局间隔计算，
	// 所以 tick 频率只决定「最多迟一分钟」，不决定实际跑测密度。
	intelligenceCheckRunnerCronSpec = "* * * * *"
	intelligenceCheckLeaderLockKey  = "intelligence:check:leader"
	intelligenceCheckLeaderLockTTL  = 5 * time.Minute
	// 单周期最多跑多少个账号：防止一次批量开开关把上百个账号同时点着。
	intelligenceCheckRunnerMaxPerCycle = 20
	// 单周期总超时：宁可让本轮被截断，也不要让一个卡死的上游拖住下一轮。
	intelligenceCheckRunnerCycleTimeout = 30 * time.Minute
)

// intelligenceCheckDueAccount 是本轮判定为到期的账号，连同它解析后的配置与最近一次记录。
type intelligenceCheckDueAccount struct {
	account Account
	config  IntelligenceCheckAccountConfig
	last    *IntelligenceCheckRun
}

// IntelligenceCheckRunnerService 定时驱动智力检测跑测。
//
// 设计要点：不引入任何额外的持久化状态。到期判定与账号冷却全部从
// intelligence_check_runs 推导（最近一条记录的 created_at / status / attempt），
// 因此进程重启不会丢失冷却进度，也不会出现「重启即重跑」。
type IntelligenceCheckRunnerService struct {
	checkSvc    *IntelligenceCheckService
	accountRepo AccountRepository
	settingSvc  *SettingService
	cfg         *config.Config

	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string

	mu      sync.Mutex
	started bool
	stopped bool
	cron    *cron.Cron

	// cycleMu 保证单实例内同一时刻只有一个调度周期在跑。
	cycleMu sync.Mutex
}

// NewIntelligenceCheckRunnerService 构造定时跑测服务（不自动启动）。
func NewIntelligenceCheckRunnerService(
	checkSvc *IntelligenceCheckService,
	accountRepo AccountRepository,
	settingSvc *SettingService,
	cfg *config.Config,
) *IntelligenceCheckRunnerService {
	return &IntelligenceCheckRunnerService{
		checkSvc:    checkSvc,
		accountRepo: accountRepo,
		settingSvc:  settingSvc,
		cfg:         cfg,
		instanceID:  uuid.NewString(),
	}
}

// SetLeaderLock 注入多实例选主所需依赖（与上游账单探针同一套机制）。
func (s *IntelligenceCheckRunnerService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

// ProvideIntelligenceCheckRunnerService 在进程启动时拉起定时跑测。
func ProvideIntelligenceCheckRunnerService(
	checkSvc *IntelligenceCheckService,
	accountRepo AccountRepository,
	settingSvc *SettingService,
	cfg *config.Config,
	lockCache LeaderLockCache,
	db *sql.DB,
) *IntelligenceCheckRunnerService {
	svc := NewIntelligenceCheckRunnerService(checkSvc, accountRepo, settingSvc, cfg)
	svc.SetLeaderLock(lockCache, db)
	svc.Start()
	return svc
}

// Start 启动每分钟一次的 cron tick。
func (s *IntelligenceCheckRunnerService) Start() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.stopped {
		return
	}
	loc := time.Local
	if s.cfg != nil {
		if parsed, err := time.LoadLocation(s.cfg.Timezone); err == nil && parsed != nil {
			loc = parsed
		}
	}
	c := cron.New(cron.WithParser(scheduledTestCronParser), cron.WithLocation(loc))
	if _, err := c.AddFunc(intelligenceCheckRunnerCronSpec, func() { s.runScheduled() }); err != nil {
		logger.LegacyPrintf("service.intelligence_check_runner", "[IntelligenceCheckRunner] not started (invalid schedule): %v", err)
		return
	}
	s.cron = c
	s.cron.Start()
	s.started = true
	logger.LegacyPrintf("service.intelligence_check_runner", "[IntelligenceCheckRunner] started (tick=every minute)")
}

// Stop 停止 cron（进程退出时调用）。
func (s *IntelligenceCheckRunnerService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	if s.cron != nil {
		stopCtx := s.cron.Stop()
		select {
		case <-stopCtx.Done():
		case <-time.After(5 * time.Second):
		}
		s.cron = nil
	}
}

func (s *IntelligenceCheckRunnerService) runScheduled() {
	ctx, cancel := context.WithTimeout(context.Background(), intelligenceCheckRunnerCycleTimeout)
	defer cancel()
	if err := s.RunDue(ctx); err != nil {
		logger.LegacyPrintf("service.intelligence_check_runner", "[IntelligenceCheckRunner] cycle failed: %v", err)
	}
}

// recoverStaleRuns 回收被中断的僵尸跑测记录。
//
// 阈值取「跑测超时 × 3」：正常跑测最长就是超时时间，留三倍余量既不会把一条
// 真在跑的记录误判成僵尸，又能让卡死的记录在十几分钟内自愈，无需人工清库。
func (s *IntelligenceCheckRunnerService) recoverStaleRuns(ctx context.Context, settings IntelligenceCheckGlobalSettings) {
	if s == nil || s.checkSvc == nil {
		return
	}
	deadline := time.Now().Add(-settings.Timeout() * 3)
	recovered, err := s.checkSvc.MarkStaleRunsInterrupted(ctx, deadline)
	if err != nil {
		logger.LegacyPrintf("service.intelligence_check_runner", "[IntelligenceCheckRunner] recover stale runs failed: %v", err)
		return
	}
	if recovered > 0 {
		logger.LegacyPrintf("service.intelligence_check_runner", "[IntelligenceCheckRunner] recovered %d stale run(s) as interrupted", recovered)
	}
}

// RunDue 执行一个调度周期：挑出到期账号，按并发上限跑测，最后裁剪历史。
func (s *IntelligenceCheckRunnerService) RunDue(ctx context.Context) error {
	if s == nil || s.checkSvc == nil || s.accountRepo == nil || s.settingSvc == nil {
		return nil
	}
	// 单实例内串行：上一轮没跑完就不要再叠一轮。
	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()

	settings, err := s.settingSvc.GetIntelligenceCheckGlobalSettings(ctx)
	if err != nil {
		return fmt.Errorf("load intelligence check settings: %w", err)
	}
	// 先回收僵尸记录再判断能不能跑：总开关关闭时同样要清理，
	// 否则一关开关，上一次遗留下来的 running 记录就会永久卡住界面。
	s.recoverStaleRuns(ctx, settings)

	if !settings.Runnable() {
		// 总开关关闭或没配模型：一条上游请求都不发。
		return nil
	}

	release, acquired, err := s.tryAcquireLeaderLock(ctx, intelligenceCheckLeaderLockKey)
	if err != nil {
		return fmt.Errorf("acquire intelligence check leader lock: %w", err)
	}
	if !acquired {
		return nil
	}
	defer release()

	accounts, err := s.accountRepo.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list active accounts: %w", err)
	}
	due, err := s.collectDueAccounts(ctx, accounts, settings, time.Now())
	if err != nil {
		return err
	}
	if len(due) == 0 {
		return nil
	}

	group := errgroup.Group{}
	group.SetLimit(settings.MaxConcurrency)
	for i := range due {
		item := due[i]
		group.Go(func() error {
			// 单个账号失败绝不能中断整轮，所以这里始终返回 nil。
			s.runDueAccount(ctx, item, settings)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}

	// 裁剪放在最后：即使本轮有账号失败，历史也不会无限增长。
	if pruned, err := s.checkSvc.PruneRuns(ctx, settings.MaxRunsPerAccount, time.Time{}); err != nil {
		logger.LegacyPrintf("service.intelligence_check_runner", "prune_runs_failed: err=%v", err)
	} else if pruned > 0 {
		logger.LegacyPrintf("service.intelligence_check_runner", "pruned_runs: count=%d", pruned)
	}
	return nil
}

// collectDueAccounts 过滤出启用了智力检测、且本轮到期的账号。
func (s *IntelligenceCheckRunnerService) collectDueAccounts(
	ctx context.Context,
	accounts []Account,
	settings IntelligenceCheckGlobalSettings,
	now time.Time,
) ([]intelligenceCheckDueAccount, error) {
	candidates := make([]intelligenceCheckDueAccount, 0, len(accounts))
	ids := make([]int64, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		config := ReadIntelligenceCheckAccountConfig(account.Extra)
		if !config.Enabled {
			continue
		}
		// 账号覆盖与全局模型都为空时直接跳过：没人配过模型就不该发请求。
		if strings.TrimSpace(config.ModelID) == "" && strings.TrimSpace(settings.ModelID) == "" {
			continue
		}
		candidates = append(candidates, intelligenceCheckDueAccount{account: account, config: config})
		ids = append(ids, account.ID)
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	latest, err := s.checkSvc.ListLatestByAccount(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load latest intelligence check runs: %w", err)
	}

	due := make([]intelligenceCheckDueAccount, 0, len(candidates))
	for i := range candidates {
		item := candidates[i]
		item.last = latest[item.account.ID]
		if !intelligenceCheckAccountDue(now, item, settings) {
			continue
		}
		due = append(due, item)
	}
	sort.SliceStable(due, func(i, j int) bool { return due[i].account.ID < due[j].account.ID })
	if len(due) > intelligenceCheckRunnerMaxPerCycle {
		due = due[:intelligenceCheckRunnerMaxPerCycle]
	}
	return due, nil
}

// intelligenceCheckAccountDue 判断账号本轮是否该跑。
//
// 规则：从没跑过→立即跑；正在跑→跳过（不叠加）；上次失败→按「账号自动恢复间隔」
// 重试，恢复次数用尽→暂停该账号；上次成功→按间隔到期（账号覆盖优先于全局）。
func intelligenceCheckAccountDue(now time.Time, item intelligenceCheckDueAccount, settings IntelligenceCheckGlobalSettings) bool {
	last := item.last
	if last == nil {
		return true
	}
	switch last.Status {
	case IntelligenceCheckStatusQueued, IntelligenceCheckStatusRunning:
		return false
	}
	if last.Status == IntelligenceCheckStatusFailed {
		if last.Attempt > settings.AccountRetryCount {
			return false
		}
		return !now.Before(last.CreatedAt.Add(settings.AccountRetryInterval()))
	}
	interval := settings.Interval()
	if item.config.IntervalMinutes > 0 {
		interval = time.Duration(item.config.IntervalMinutes) * time.Minute
	}
	return !now.Before(last.CreatedAt.Add(interval))
}

// runDueAccount 跑一个账号：入队一条记录，然后在同一条记录上做单次跑测重试。
func (s *IntelligenceCheckRunnerService) runDueAccount(ctx context.Context, item intelligenceCheckDueAccount, settings IntelligenceCheckGlobalSettings) {
	modelID := strings.TrimSpace(item.config.ModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(settings.ModelID)
	}
	if modelID == "" {
		return
	}

	// 思考强度同样支持账号级覆盖，缺省跟随全局；与模型覆盖同一套回落语义。
	reasoningEffort := strings.TrimSpace(item.config.ReasoningEffort)
	if reasoningEffort == "" {
		reasoningEffort = strings.TrimSpace(settings.ReasoningEffort)
	}

	req := IntelligenceCheckRequest{
		AccountID:       item.account.ID,
		ModelID:         modelID,
		ReasoningEffort: reasoningEffort,
		MaxTokens:       settings.MaxTokens,
		Timeout:         settings.Timeout(),
		TriggerSource:   IntelligenceCheckTriggerSchedule,
		Attempt:         intelligenceCheckNextAttempt(item.last),
		// 与全局流式开关保持一致；resolveRequestDefaults 也会兜一次，
		// 这里显式带上是为了让调度器路径的意图自解释。
		DisableStream: !settings.StreamEnabled,
	}

	run, err := s.checkSvc.StartRun(ctx, req)
	if err != nil {
		logger.LegacyPrintf("service.intelligence_check_runner", "start_run_failed: account_id=%d err=%v", item.account.ID, err)
		return
	}

	// 单次跑测重试：网络抖动 / 上游 5xx 时在同一条记录上重跑，所以历史里
	// 不会留下 N 条「重试碎片」，Attempt 语义只留给账号自动恢复使用。
	for round := 0; round <= settings.RunRetryCount; round++ {
		if round > 0 {
			if !intelligenceCheckSleep(ctx, settings.RunRetryInterval()) {
				return
			}
		}
		if err := s.checkSvc.ExecuteRun(ctx, run.ID, req); err != nil {
			logger.LegacyPrintf("service.intelligence_check_runner",
				"execute_run_failed: run_id=%d account_id=%d err=%v", run.ID, item.account.ID, err)
			return
		}
		updated, err := s.checkSvc.GetRun(ctx, run.ID)
		if err != nil || updated == nil {
			return
		}
		if updated.Status == IntelligenceCheckStatusCompleted {
			return
		}
		if round < settings.RunRetryCount {
			logger.LegacyPrintf("service.intelligence_check_runner",
				"run_retry: run_id=%d account_id=%d retry=%d/%d error_code=%s",
				run.ID, item.account.ID, round+1, settings.RunRetryCount, updated.ErrorCode)
		}
	}
}

// intelligenceCheckNextAttempt 推导本次尝试序号：上次失败说明正在走账号自动恢复，
// 序号 +1；否则是新的一轮，从 1 开始。
func intelligenceCheckNextAttempt(last *IntelligenceCheckRun) int {
	if last == nil || last.Status != IntelligenceCheckStatusFailed {
		return 1
	}
	if last.Attempt <= 0 {
		return 2
	}
	return last.Attempt + 1
}

func (s *IntelligenceCheckRunnerService) tryAcquireLeaderLock(ctx context.Context, key string) (func(), bool, error) {
	lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if s.lockCache != nil {
		acquired, err := s.lockCache.TryAcquireLeaderLock(lockCtx, key, s.instanceID, intelligenceCheckLeaderLockTTL)
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

// intelligenceCheckSleep 可取消的等待，返回 false 表示 ctx 已结束。
func intelligenceCheckSleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

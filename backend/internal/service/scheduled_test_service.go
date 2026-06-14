package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/robfig/cron/v3"
)

var scheduledTestCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// HealthSnapshotUpdater 写入账号健康快照(最近检测原始状态 + 耗时 + 时间)。
// 由 AccountRepository 实现;用接口隔离,避免 ScheduledTestService 依赖整个账号仓储。
type HealthSnapshotUpdater interface {
	UpdateHealthSnapshot(ctx context.Context, id int64, status string, latencyMs int64, checkedAt time.Time) error
}

// ScheduledTestService provides CRUD operations for scheduled test plans and results.
type ScheduledTestService struct {
	planRepo      ScheduledTestPlanRepository
	resultRepo    ScheduledTestResultRepository
	healthUpdater HealthSnapshotUpdater
}

// NewScheduledTestService creates a new ScheduledTestService.
func NewScheduledTestService(
	planRepo ScheduledTestPlanRepository,
	resultRepo ScheduledTestResultRepository,
) *ScheduledTestService {
	return &ScheduledTestService{
		planRepo:   planRepo,
		resultRepo: resultRepo,
	}
}

// SetHealthSnapshotUpdater 注入健康快照更新器(可选)。
// 通过 setter 注入而非构造参数,避免改动 Wire provider 签名及既有调用点。
func (s *ScheduledTestService) SetHealthSnapshotUpdater(u HealthSnapshotUpdater) {
	s.healthUpdater = u
}

// updateHealthSnapshot 尽力更新账号健康快照;失败仅记录,不影响检测结果落库。
func (s *ScheduledTestService) updateHealthSnapshot(ctx context.Context, accountID int64, result *ScheduledTestResult) {
	if s.healthUpdater == nil || result == nil {
		return
	}
	checkedAt := result.FinishedAt
	if checkedAt.IsZero() {
		checkedAt = time.Now()
	}
	if err := s.healthUpdater.UpdateHealthSnapshot(ctx, accountID, result.Status, result.LatencyMs, checkedAt); err != nil {
		logger.LegacyPrintf("service.scheduled_test", "[ScheduledTest] update health snapshot failed: account=%d err=%v", accountID, err)
	}
}

// CreatePlan validates the cron expression, computes next_run_at, and persists the plan.
func (s *ScheduledTestService) CreatePlan(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	plan.NextRunAt = &nextRun

	if plan.MaxResults <= 0 {
		plan.MaxResults = 50
	}

	return s.planRepo.Create(ctx, plan)
}

// BatchPlanConflictStrategy 批量创建定时检测计划时,对已存在 (account_id, model_id) 计划的处理策略。
type BatchPlanConflictStrategy string

const (
	BatchConflictOverwrite BatchPlanConflictStrategy = "overwrite" // 默认:用新配置替换旧计划
	BatchConflictSkip      BatchPlanConflictStrategy = "skip"       // 跳过已有同模型计划的账号
	BatchConflictAdd       BatchPlanConflictStrategy = "add"        // 仍新增(允许同账号同模型多计划)
)

// BatchCreatePlansInput 批量创建定时检测计划的输入(需求 §7.4)。
type BatchCreatePlansInput struct {
	AccountIDs     []int64
	ModelID        string
	CronExpression string
	Enabled        bool
	MaxResults     int
	AutoRecover    bool
	Conflict       BatchPlanConflictStrategy
}

// BatchPlanItemResult 单个账号的批量处理结果。
type BatchPlanItemResult struct {
	AccountID int64  `json:"account_id"`
	Action    string `json:"action"` // created/overwritten/skipped/failed
	PlanID    int64  `json:"plan_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// BatchCreatePlansResult 批量处理汇总(需求 §7.4.4)。
type BatchCreatePlansResult struct {
	Success int                   `json:"success"`
	Failed  int                   `json:"failed"`
	Skipped int                   `json:"skipped"`
	Items   []BatchPlanItemResult `json:"items"`
}

// BatchCreatePlans 为多个账号统一创建定时检测计划(需求 §7.4)。
// 冲突键为 (account_id, model_id):
//   - overwrite(默认):更新已有计划为新配置
//   - skip:跳过已有同模型计划的账号
//   - add:无视冲突直接新增
//
// 调用方应在传入前过滤掉不存在/不可用账号(§7.4.5)。单个账号失败不影响其他账号。
func (s *ScheduledTestService) BatchCreatePlans(ctx context.Context, in BatchCreatePlansInput) (*BatchCreatePlansResult, error) {
	if in.ModelID == "" {
		return nil, fmt.Errorf("model_id is required")
	}
	// 预校验 cron,避免逐账号重复报同一错误。
	if _, err := computeNextRun(in.CronExpression, time.Now()); err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	strategy := in.Conflict
	if strategy == "" {
		strategy = BatchConflictOverwrite
	}
	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}

	result := &BatchCreatePlansResult{Items: make([]BatchPlanItemResult, 0, len(in.AccountIDs))}
	for _, accountID := range in.AccountIDs {
		item := BatchPlanItemResult{AccountID: accountID}

		// 查已有计划,按 model_id 判定冲突。
		existing, err := s.planRepo.ListByAccountID(ctx, accountID)
		if err != nil {
			item.Action = "failed"
			item.Error = err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		var conflict *ScheduledTestPlan
		for _, p := range existing {
			if p.ModelID == in.ModelID {
				conflict = p
				break
			}
		}

		if conflict != nil && strategy == BatchConflictSkip {
			item.Action = "skipped"
			item.PlanID = conflict.ID
			result.Skipped++
			result.Items = append(result.Items, item)
			continue
		}

		if conflict != nil && strategy == BatchConflictOverwrite {
			conflict.CronExpression = in.CronExpression
			conflict.Enabled = in.Enabled
			conflict.MaxResults = maxResults
			conflict.AutoRecover = in.AutoRecover
			updated, uerr := s.UpdatePlan(ctx, conflict)
			if uerr != nil {
				item.Action = "failed"
				item.Error = uerr.Error()
				result.Failed++
			} else {
				item.Action = "overwritten"
				item.PlanID = updated.ID
				result.Success++
			}
			result.Items = append(result.Items, item)
			continue
		}

		// 无冲突,或 strategy=add:新增。
		plan := &ScheduledTestPlan{
			AccountID:      accountID,
			ModelID:        in.ModelID,
			CronExpression: in.CronExpression,
			Enabled:        in.Enabled,
			MaxResults:     maxResults,
			AutoRecover:    in.AutoRecover,
		}
		created, cerr := s.CreatePlan(ctx, plan)
		if cerr != nil {
			item.Action = "failed"
			item.Error = cerr.Error()
			result.Failed++
		} else {
			item.Action = "created"
			item.PlanID = created.ID
			result.Success++
		}
		result.Items = append(result.Items, item)
	}

	return result, nil
}

// GetPlan retrieves a plan by ID.
func (s *ScheduledTestService) GetPlan(ctx context.Context, id int64) (*ScheduledTestPlan, error) {
	return s.planRepo.GetByID(ctx, id)
}

// ListPlansByAccount returns all plans for a given account.
func (s *ScheduledTestService) ListPlansByAccount(ctx context.Context, accountID int64) ([]*ScheduledTestPlan, error) {
	return s.planRepo.ListByAccountID(ctx, accountID)
}

// UpdatePlan validates cron and updates the plan.
func (s *ScheduledTestService) UpdatePlan(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	plan.NextRunAt = &nextRun

	return s.planRepo.Update(ctx, plan)
}

// DeletePlan removes a plan and its results (via CASCADE).
func (s *ScheduledTestService) DeletePlan(ctx context.Context, id int64) error {
	return s.planRepo.Delete(ctx, id)
}

// ListResults returns the most recent results for a plan.
func (s *ScheduledTestService) ListResults(ctx context.Context, planID int64, limit int) ([]*ScheduledTestResult, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.resultRepo.ListByPlanID(ctx, planID, limit)
}

// SaveResult inserts a scheduled result and prunes old entries beyond maxResults.
// 调用方(runner)负责传入 accountID(plan.AccountID);本方法标记 source=scheduled。
func (s *ScheduledTestService) SaveResult(ctx context.Context, planID int64, accountID int64, maxResults int, result *ScheduledTestResult) error {
	pid := planID
	result.PlanID = &pid
	result.AccountID = accountID
	result.Source = "scheduled"
	if _, err := s.resultRepo.Create(ctx, result); err != nil {
		return err
	}
	s.updateHealthSnapshot(ctx, accountID, result)
	return s.resultRepo.PruneOldResults(ctx, planID, maxResults)
}

// defaultManualResultKeep 手动测试结果每账号默认保留条数(与定时计划 max_results 默认一致)。
const defaultManualResultKeep = 50

// SaveManualResult 写入一条手动测试结果(source=manual, plan_id=NULL),
// 然后裁剪该账号的旧手动结果,防止无限增长。
func (s *ScheduledTestService) SaveManualResult(ctx context.Context, accountID int64, result *ScheduledTestResult) error {
	result.PlanID = nil
	result.AccountID = accountID
	result.Source = "manual"
	if _, err := s.resultRepo.Create(ctx, result); err != nil {
		return err
	}
	s.updateHealthSnapshot(ctx, accountID, result)
	return s.resultRepo.PruneOldManualResults(ctx, accountID, defaultManualResultKeep)
}

// GetLatestResultsByAccounts 批量获取每个账号最近一条检测结果(避免 N+1)。
func (s *ScheduledTestService) GetLatestResultsByAccounts(ctx context.Context, accountIDs []int64) (map[int64]*ScheduledTestResult, error) {
	return s.resultRepo.ListLatestByAccountIDs(ctx, accountIDs)
}

// ListResultsByAccount 返回账号最近 limit 条检测结果(含手动 + 定时),供健康详情展示(需求 §7.5)。
func (s *ScheduledTestService) ListResultsByAccount(ctx context.Context, accountID int64, limit int) ([]*ScheduledTestResult, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	return s.resultRepo.ListByAccountID(ctx, accountID, limit)
}

func computeNextRun(cronExpr string, from time.Time) (time.Time, error) {
	sched, err := scheduledTestCronParser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(from), nil
}

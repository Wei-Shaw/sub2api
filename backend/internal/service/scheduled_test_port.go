package service

import (
	"context"
	"time"
)

// ScheduledTestPlan represents a scheduled test plan domain model.
type ScheduledTestPlan struct {
	ID             int64      `json:"id"`
	AccountID      int64      `json:"account_id"`
	ModelID        string     `json:"model_id"`
	CronExpression string     `json:"cron_expression"`
	Enabled        bool       `json:"enabled"`
	MaxResults     int        `json:"max_results"`
	AutoRecover    bool       `json:"auto_recover"`
	LastRunAt      *time.Time `json:"last_run_at"`
	NextRunAt      *time.Time `json:"next_run_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ScheduledTestResult represents a single test execution result.
type ScheduledTestResult struct {
	ID           int64  `json:"id"`
	PlanID       *int64 `json:"plan_id"`   // 指针:手动测试为 nil,定时测试为所属 plan
	AccountID    int64  `json:"account_id"`
	Source       string `json:"source"` // "manual" | "scheduled"
	ModelID      string `json:"model_id"`
	Status       string `json:"status"`
	ResponseText string `json:"response_text"`
	ErrorMessage string `json:"error_message"`
	LatencyMs    int64  `json:"latency_ms"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// ScheduledTestPlanRepository defines the data access interface for test plans.
type ScheduledTestPlanRepository interface {
	Create(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error)
	GetByID(ctx context.Context, id int64) (*ScheduledTestPlan, error)
	ListByAccountID(ctx context.Context, accountID int64) ([]*ScheduledTestPlan, error)
	ListDue(ctx context.Context, now time.Time) ([]*ScheduledTestPlan, error)
	Update(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error)
	Delete(ctx context.Context, id int64) error
	UpdateAfterRun(ctx context.Context, id int64, lastRunAt time.Time, nextRunAt time.Time) error
}

// ScheduledTestResultRepository defines the data access interface for test results.
type ScheduledTestResultRepository interface {
	Create(ctx context.Context, result *ScheduledTestResult) (*ScheduledTestResult, error)
	ListByPlanID(ctx context.Context, planID int64, limit int) ([]*ScheduledTestResult, error)
	PruneOldResults(ctx context.Context, planID int64, keepCount int) error
	// ListLatestByAccountIDs 一次查出每个账号最近一条结果(避免 N+1)。
	ListLatestByAccountIDs(ctx context.Context, accountIDs []int64) (map[int64]*ScheduledTestResult, error)
	// ListByAccountID 按账号查最近 limit 条检测结果(含手动+定时,按时间倒序),供健康详情展示。
	ListByAccountID(ctx context.Context, accountID int64, limit int) ([]*ScheduledTestResult, error)
	// PruneOldManualResults 按 account_id 且 plan_id IS NULL 保留最近 N 条手动结果。
	PruneOldManualResults(ctx context.Context, accountID int64, keepCount int) error
}

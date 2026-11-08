package service

import (
	"context"
	"time"
)

// 智力检测（鹈鹕测试）状态机。
const (
	IntelligenceCheckStatusQueued      = "queued"
	IntelligenceCheckStatusRunning     = "running"
	IntelligenceCheckStatusCompleted   = "completed"
	IntelligenceCheckStatusFailed      = "failed"
	IntelligenceCheckStatusCancelled   = "cancelled"
	IntelligenceCheckStatusInterrupted = "interrupted"
)

// 判定结论。跑测本身不再自动判定成败，只负责产出作品：
//   - unknown = 已产出作品、等待管理员人工评审（也用于尚未跑出结果的记录）
//   - pass    = 管理员评审为「通过」
//   - fail    = 管理员评审为「不通过」
//
// 注意与 Status 区分：Status=failed 表示跑测执行失败（超时/未配模型/没抽出 HTML），
// 那是「没评的资格」，不是「评审不通过」。
const (
	IntelligenceCheckVerdictPass    = "pass"
	IntelligenceCheckVerdictFail    = "fail"
	IntelligenceCheckVerdictUnknown = "unknown"
)

// 失败原因分类。写入 error_code 列，便于前端按类型聚合。
const (
	IntelligenceCheckErrTimeout          = "PELICAN_TIMEOUT"
	IntelligenceCheckErrStreamIncomplete = "PELICAN_STREAM_INCOMPLETE"
	IntelligenceCheckErrResponseTooLarge = "PELICAN_RESPONSE_TOO_LARGE"
	IntelligenceCheckErrUnsupportedAcct  = "PELICAN_UNSUPPORTED_ACCOUNT"
	IntelligenceCheckErrNoModel          = "PELICAN_NO_MODEL"
	IntelligenceCheckErrNoHTML           = "PELICAN_NO_HTML"
	// IntelligenceCheckErrInterrupted 表示这条记录在跑测途中被中断：进程重启 / 容器重建
	// 会直接杀掉执行 goroutine，记录就此停在 running，既没有终态也不会再有人推进。
	// 它不是上游失败，也不代表模型答得不好，所以与 PELICAN_TIMEOUT 等分开。
	IntelligenceCheckErrInterrupted = "PELICAN_INTERRUPTED"
)

// 触发来源。
const (
	IntelligenceCheckTriggerManual   = "manual"
	IntelligenceCheckTriggerSchedule = "schedule"
)

// 默认题面变体标识。
const IntelligenceCheckPromptVariantClassic = "classic"

// IntelligenceCheckRun 是一次跑测的完整记录。
type IntelligenceCheckRun struct {
	ID              int64
	AccountID       int64
	BatchID         string
	TriggerSource   string
	ModelID         string
	UpstreamModel   string
	ReasoningEffort string
	PromptVariant   string
	Status          string
	Verdict         string
	HasHTML         bool
	HTMLBytes       int
	LatencyMs       int64
	Attempt         int
	ErrorCode       string
	ErrorMessage    string
	StartedAt       time.Time
	FinishedAt      *time.Time
	CreatedAt       time.Time
	// ReviewedBy / ReviewedAt 记录人工评审。ReviewedAt 为 nil 即「待评审」。
	// 跑测执行过程只写 verdict=unknown，这两个字段仅由管理员评审动作填充。
	ReviewedBy int64
	ReviewedAt *time.Time
}

// IntelligenceCheckArtifact 是跑测产出的作品原文，与跑测记录一对一。
type IntelligenceCheckArtifact struct {
	RunID     int64
	HTMLText  string
	ByteLen   int
	SHA256    string
	CreatedAt time.Time
}

// IntelligenceCheckRunFilter 是列表查询条件。零值表示不过滤。
type IntelligenceCheckRunFilter struct {
	AccountIDs []int64
	Status     string
	Verdict    string
	ModelID    string
	Since      *time.Time
	Limit      int
	Offset     int
}

// IntelligenceCheckRunRepository 是跑测记录与作品原文的仓储接口。
// 约定：接口声明在 service 包，实现放在 repository 包。
type IntelligenceCheckRunRepository interface {
	// Create 写入一条跑测记录并回填 ID/CreatedAt。
	Create(ctx context.Context, run *IntelligenceCheckRun) (*IntelligenceCheckRun, error)
	// UpdateResult 回写跑测终态（状态、耗时、错误、完成时间）。
	// 它把 verdict 重置为「待评审」，但绝不写 ReviewedBy / ReviewedAt ——
	// 重跑一次等于作废上一次评审结论，需要管理员重新评。
	UpdateResult(ctx context.Context, run *IntelligenceCheckRun) error
	// UpdateReview 回写人工评审结论（verdict、评审人、评审时间）。
	UpdateReview(ctx context.Context, run *IntelligenceCheckRun) error
	// GetByID 按主键取单条记录。
	GetByID(ctx context.Context, id int64) (*IntelligenceCheckRun, error)
	// List 分页返回记录与总数，按 created_at 倒序。
	List(ctx context.Context, filter IntelligenceCheckRunFilter) ([]*IntelligenceCheckRun, int64, error)
	// ListLatestByAccount 返回每个账号最近一次跑测记录，供卡片墙使用。
	ListLatestByAccount(ctx context.Context, accountIDs []int64) (map[int64]*IntelligenceCheckRun, error)
	// SaveArtifact 保存作品原文（同一 run 重复保存时覆盖）。
	SaveArtifact(ctx context.Context, artifact *IntelligenceCheckArtifact) error
	// GetArtifact 取出作品原文。
	GetArtifact(ctx context.Context, runID int64) (*IntelligenceCheckArtifact, error)
	// PruneRuns 按「每账号保留最近 keepPerAccount 条」与「早于 before 的一律删除」两个条件剪枝，
	// 返回删除条数。
	PruneRuns(ctx context.Context, keepPerAccount int, before time.Time) (int64, error)
	// ListLatestPerAccount 返回每个账号最近一次跑测（按 account_id 升序），供公开作品墙脱敏展示。
	ListLatestPerAccount(ctx context.Context, limit int) ([]*IntelligenceCheckRun, error)
	// MarkStaleRunsInterrupted 把「开始时间早于 deadline 且仍停在 queued/running」的记录
	// 落成 interrupted 终态，返回处理条数。用于回收进程重启后遗留的僵尸跑测。
	MarkStaleRunsInterrupted(ctx context.Context, deadline time.Time) (int64, error)
}

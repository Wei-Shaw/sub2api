package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

var (
	ErrEvaluationReportNotFound = infraerrors.NotFound("EVALUATION_REPORT_NOT_FOUND", "评测报告不存在，请刷新历史记录")
	ErrEvaluationRoundConflict  = infraerrors.Conflict("EVALUATION_ROUND_CONFLICT", "该轮正在运行或前一轮未完成，请刷新历史记录；中断后请新建评测")
)

type ModelEvaluationCreate struct {
	ModelEvaluationRequest
	Rounds int `json:"rounds"`
}

type ModelEvaluationReport struct {
	ID         string                  `json:"id"`
	TargetType string                  `json:"target_type"`
	TargetID   int64                   `json:"target_id"`
	TargetName string                  `json:"target_name"`
	Model      string                  `json:"model"`
	Effort     string                  `json:"effort"`
	Rounds     int                     `json:"rounds"`
	Benchmark  string                  `json:"benchmark"`
	Prompt     string                  `json:"prompt,omitempty"`
	CreatedBy  int64                   `json:"created_by,omitempty"`
	Visibility string                  `json:"visibility"`
	CreatedAt  time.Time               `json:"created_at"`
	Completed  int                     `json:"completed"`
	Correct    int                     `json:"correct"`
	Graded     int                     `json:"graded"`
	Results    []*ModelEvaluationRound `json:"results,omitempty"`
}

type ModelEvaluationRound struct {
	Round     int                    `json:"round"`
	Result    *ModelEvaluationResult `json:"result"`
	Saved     bool                   `json:"saved"`
	SaveError string                 `json:"save_error,omitempty"`
}

type ModelEvaluationHistory struct {
	Items    []*ModelEvaluationReport `json:"items"`
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

type ModelEvaluationRepository interface {
	Create(context.Context, *ModelEvaluationReport) error
	List(context.Context, string, int64, int, int) ([]*ModelEvaluationReport, int64, error)
	Get(context.Context, string) (*ModelEvaluationReport, error)
	Reserve(context.Context, string, int, *ModelEvaluationResult) (*ModelEvaluationRound, bool, error)
	Finish(context.Context, string, *ModelEvaluationRound) error
}

// The report is persisted before any upstream call. Each round is reserved once
// and finished independently of the browser's request lifetime.
type ModelEvaluationHistoryService struct {
	evaluator *ModelEvaluationService
	repo      ModelEvaluationRepository
}

func NewModelEvaluationHistoryService(evaluator *ModelEvaluationService, repo ModelEvaluationRepository) *ModelEvaluationHistoryService {
	return &ModelEvaluationHistoryService{evaluator: evaluator, repo: repo}
}

func (s *ModelEvaluationHistoryService) Create(ctx context.Context, req ModelEvaluationCreate, createdBy int64) (*ModelEvaluationReport, error) {
	req.Model = strings.TrimSpace(req.Model)
	if err := validateModelEvaluationRequest(req.ModelEvaluationRequest); err != nil {
		return nil, err
	}
	if req.Rounds < 1 || req.Rounds > 20 {
		return nil, infraerrors.BadRequest("INVALID_EVALUATION_ROUNDS", "评测轮数必须为 1–20 的整数")
	}
	var name string
	if req.TargetType == "account" {
		account, err := s.evaluator.accounts.GetByID(ctx, req.TargetID)
		if err != nil || account == nil {
			return nil, infraerrors.BadRequest("EVALUATION_ACCOUNT_UNAVAILABLE", "账号不存在或读取失败，请刷新账号列表后重试")
		}
		name = account.Name
	} else {
		group, err := s.evaluator.groups.GetByID(ctx, req.TargetID)
		if err != nil || group == nil {
			return nil, infraerrors.BadRequest("EVALUATION_GROUP_UNAVAILABLE", "分组不存在或读取失败，请刷新分组列表后重试")
		}
		name = group.Name
	}
	report := &ModelEvaluationReport{
		ID: uuid.NewString(), TargetType: req.TargetType, TargetID: req.TargetID, TargetName: name,
		Model: req.Model, Effort: req.Effort, Rounds: req.Rounds, Benchmark: modelEvaluationBenchmark,
		Prompt: modelEvaluationPrompt, CreatedBy: createdBy, Visibility: "private", CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, report); err != nil {
		return nil, infraerrors.ServiceUnavailable("EVALUATION_REPORT_SAVE_FAILED", "无法保存评测报告，尚未调用模型；请检查数据库后重试")
	}
	return report, nil
}

func (s *ModelEvaluationHistoryService) List(ctx context.Context, targetType string, targetID int64, page int) (*ModelEvaluationHistory, error) {
	if (targetType != "account" && targetType != "group") || targetID <= 0 || page < 1 || page > 1000000 {
		return nil, infraerrors.BadRequest("INVALID_EVALUATION_HISTORY", "历史记录查询参数无效，请重新打开评测")
	}
	items, total, err := s.repo.List(ctx, targetType, targetID, page, 20)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("EVALUATION_HISTORY_UNAVAILABLE", "无法读取评测历史，请稍后刷新重试")
	}
	return &ModelEvaluationHistory{Items: items, Total: total, Page: page, PageSize: 20}, nil
}

func (s *ModelEvaluationHistoryService) Get(ctx context.Context, id string) (*ModelEvaluationReport, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, infraerrors.BadRequest("INVALID_EVALUATION_REPORT", "评测报告编号无效，请刷新历史记录")
	}
	report, err := s.repo.Get(ctx, id)
	if errors.Is(err, ErrEvaluationReportNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("EVALUATION_HISTORY_UNAVAILABLE", "无法读取评测报告，请稍后刷新重试")
	}
	for _, row := range report.Results {
		if row.Result.Status == "running" && time.Since(row.Result.StartedAt) > 4*time.Minute {
			row.Result.Status = "interrupted"
			row.Result.Error = "该轮未保存最终结果，可能已中断；已完成轮次仍保留，请新建评测复测"
		}
	}
	return report, nil
}

func (s *ModelEvaluationHistoryService) Run(ctx context.Context, id string, number int) (*ModelEvaluationRound, error) {
	report, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if number < 1 || number > report.Rounds {
		return nil, infraerrors.BadRequest("INVALID_EVALUATION_ROUND", "轮次超出报告范围，请新建评测")
	}
	if report.Benchmark != modelEvaluationBenchmark {
		return nil, infraerrors.BadRequest("EVALUATION_BENCHMARK_CHANGED", "题目版本已更新，请新建评测；历史结果仍可查看")
	}
	initial := &ModelEvaluationResult{
		ID: uuid.NewString(), StartedAt: time.Now().UTC(), Benchmark: report.Benchmark,
		TargetType: report.TargetType, TargetID: report.TargetID, TargetName: report.TargetName,
		RequestedModel: report.Model, RequestedEffort: report.Effort, Status: "running",
	}
	row, created, err := s.repo.Reserve(ctx, id, number, initial)
	if errors.Is(err, ErrEvaluationRoundConflict) {
		return nil, err
	}
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("EVALUATION_ROUND_SAVE_FAILED", "无法保存评测轮次，尚未调用模型；请检查数据库并刷新历史记录")
	}
	if !created {
		if row.Result.Status == "running" {
			return nil, ErrEvaluationRoundConflict
		}
		return row, nil
	}
	result, runErr := s.evaluator.Run(ctx, ModelEvaluationRequest{
		TargetType: report.TargetType, TargetID: report.TargetID, Model: report.Model, Effort: report.Effort,
	})
	if runErr != nil {
		result = initial
		result.Status = "error"
		result.Error = infraerrors.Message(runErr)
		result.DurationMs = time.Since(initial.StartedAt).Milliseconds()
	}
	result.ID = initial.ID
	row.Result = result
	// Canceling the evaluation must not cancel saving the completed/canceled round.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.repo.Finish(saveCtx, id, row); err != nil {
		row.Saved = false
		row.SaveError = fmt.Sprintf("报告 %s 第 %d 轮结果保存失败；请先导出当前报告，再检查数据库并刷新历史记录，不要重复发送本轮", id, number)
		return row, nil
	}
	row.Saved = true
	return row, nil
}

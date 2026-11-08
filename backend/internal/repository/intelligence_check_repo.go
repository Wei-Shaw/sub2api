package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 智力检测跑测记录仓储。沿用 scheduled_test_repo 的原生 database/sql 写法：
// 这张表是展示型低频数据，不值得为它引入 ent schema 与 codegen。
const intelligenceCheckRunColumns = `
	id, account_id, COALESCE(batch_id::text, ''), trigger_source, model_id, upstream_model,
	reasoning_effort, prompt_variant, status, verdict, has_html, html_bytes, latency_ms,
	attempt, error_code, error_message, started_at, finished_at, created_at,
	COALESCE(reviewed_by, 0), reviewed_at`

const (
	intelligenceCheckListDefaultLimit = 50
	intelligenceCheckListMaxLimit     = 200
)

type intelligenceCheckRunRepository struct {
	db *sql.DB
}

// NewIntelligenceCheckRunRepository 构造智力检测跑测记录仓储。
func NewIntelligenceCheckRunRepository(db *sql.DB) service.IntelligenceCheckRunRepository {
	return &intelligenceCheckRunRepository{db: db}
}

func (r *intelligenceCheckRunRepository) Create(ctx context.Context, run *service.IntelligenceCheckRun) (*service.IntelligenceCheckRun, error) {
	startedAt := run.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	// batch_id 是 UUID 列，空串会被 Postgres 判为非法输入，必须落成 NULL。
	var batchID any
	if strings.TrimSpace(run.BatchID) != "" {
		batchID = run.BatchID
	}

	row := r.db.QueryRowContext(ctx, `
		INSERT INTO intelligence_check_runs (
			account_id, batch_id, trigger_source, model_id, upstream_model, reasoning_effort,
			prompt_variant, status, verdict, has_html, html_bytes, latency_ms, attempt,
			error_code, error_message, started_at, finished_at, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
		RETURNING`+intelligenceCheckRunColumns,
		run.AccountID, batchID, run.TriggerSource, run.ModelID, run.UpstreamModel, run.ReasoningEffort,
		run.PromptVariant, run.Status, run.Verdict, run.HasHTML, run.HTMLBytes, run.LatencyMs, run.Attempt,
		run.ErrorCode, run.ErrorMessage, startedAt, run.FinishedAt,
	)
	return scanIntelligenceCheckRun(row)
}

func (r *intelligenceCheckRunRepository) UpdateResult(ctx context.Context, run *service.IntelligenceCheckRun) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE intelligence_check_runs
		SET status = $2, verdict = $3, has_html = $4, html_bytes = $5, latency_ms = $6,
		    model_id = $7, upstream_model = $8, reasoning_effort = $9,
		    error_code = $10, error_message = $11, finished_at = $12,
		    reviewed_by = NULL, reviewed_at = NULL
		WHERE id = $1
	`, run.ID, run.Status, run.Verdict, run.HasHTML, run.HTMLBytes, run.LatencyMs,
		run.ModelID, run.UpstreamModel, run.ReasoningEffort,
		run.ErrorCode, run.ErrorMessage, run.FinishedAt)
	return err
}

// UpdateReview 只回写评审三件套，绝不碰 status / has_html / latency 等跑测结果字段：
// 评审结论与跑测执行结果是两个独立维度，互不覆盖。
func (r *intelligenceCheckRunRepository) UpdateReview(ctx context.Context, run *service.IntelligenceCheckRun) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE intelligence_check_runs
		SET verdict = $2, reviewed_by = NULLIF($3, 0), reviewed_at = $4
		WHERE id = $1
	`, run.ID, run.Verdict, run.ReviewedBy, run.ReviewedAt)
	return err
}

// MarkStaleRunsInterrupted 回收僵尸跑测记录。
//
// 背景：跑测在后台 goroutine 里执行，进程退出（容器重建、部署、OOM）会直接杀掉它，
// 记录便永远停在 queued/running —— 既没有终态，也不会再有人推进它。前端看到这种
// 记录会一直显示「跑测中」，用户无论怎么刷新都不会变，而它其实早就不跑了。
//
// 判定只用「开始时间早于 deadline」这一个与进程无关的条件，所以重启后第一轮调度
// 就能自愈；重复执行是幂等的。verdict 一并重置为待评审：中断的记录没有作品，
// 本来就不可评审，也避免残留的上一次结论被误读成人工评审过。
func (r *intelligenceCheckRunRepository) MarkStaleRunsInterrupted(ctx context.Context, deadline time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE intelligence_check_runs
		SET status = $1,
		    verdict = $2,
		    error_code = $3,
		    error_message = $4,
		    finished_at = NOW()
		WHERE status IN ($5, $6)
		  AND started_at < $7
	`, service.IntelligenceCheckStatusInterrupted,
		service.IntelligenceCheckVerdictUnknown,
		service.IntelligenceCheckErrInterrupted,
		"跑测被中断（服务重启或部署），本次未产出作品，请重新跑测",
		service.IntelligenceCheckStatusQueued,
		service.IntelligenceCheckStatusRunning,
		deadline,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *intelligenceCheckRunRepository) GetByID(ctx context.Context, id int64) (*service.IntelligenceCheckRun, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT`+intelligenceCheckRunColumns+`
		FROM intelligence_check_runs WHERE id = $1
	`, id)
	return scanIntelligenceCheckRun(row)
}

func (r *intelligenceCheckRunRepository) List(ctx context.Context, filter service.IntelligenceCheckRunFilter) ([]*service.IntelligenceCheckRun, int64, error) {
	where, args := buildIntelligenceCheckRunWhere(filter)

	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM intelligence_check_runs`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = intelligenceCheckListDefaultLimit
	}
	if limit > intelligenceCheckListMaxLimit {
		limit = intelligenceCheckListMaxLimit
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT`+intelligenceCheckRunColumns+`
		FROM intelligence_check_runs`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	runs, err := scanIntelligenceCheckRuns(rows)
	if err != nil {
		return nil, 0, err
	}
	return runs, total, nil
}

func (r *intelligenceCheckRunRepository) ListLatestByAccount(ctx context.Context, accountIDs []int64) (map[int64]*service.IntelligenceCheckRun, error) {
	out := make(map[int64]*service.IntelligenceCheckRun)
	if len(accountIDs) == 0 {
		return out, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT ON (account_id)`+intelligenceCheckRunColumns+`
		FROM intelligence_check_runs
		WHERE account_id = ANY($1)
		ORDER BY account_id, created_at DESC, id DESC
	`, pq.Array(accountIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	runs, err := scanIntelligenceCheckRuns(rows)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		out[run.AccountID] = run
	}
	return out, nil
}

// ListLatestPerAccount 返回每个账号最近一次跑测（按 account_id 升序）。
// 供公开作品墙使用：调用方据此生成展示序号，全程不需要暴露 account_id。
func (r *intelligenceCheckRunRepository) ListLatestPerAccount(ctx context.Context, limit int) ([]*service.IntelligenceCheckRun, error) {
	query := `
		SELECT DISTINCT ON (account_id)` + intelligenceCheckRunColumns + `
		FROM intelligence_check_runs
		ORDER BY account_id ASC, created_at DESC, id DESC`
	args := []any{}
	if limit > 0 {
		query += `
		LIMIT $1`
		args = append(args, limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanIntelligenceCheckRuns(rows)
}

func (r *intelligenceCheckRunRepository) SaveArtifact(ctx context.Context, artifact *service.IntelligenceCheckArtifact) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO intelligence_check_artifacts (run_id, html_text, byte_len, sha256, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (run_id) DO UPDATE
		SET html_text = EXCLUDED.html_text, byte_len = EXCLUDED.byte_len, sha256 = EXCLUDED.sha256
	`, artifact.RunID, artifact.HTMLText, artifact.ByteLen, artifact.SHA256)
	return err
}

func (r *intelligenceCheckRunRepository) GetArtifact(ctx context.Context, runID int64) (*service.IntelligenceCheckArtifact, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT run_id, html_text, byte_len, sha256, created_at
		FROM intelligence_check_artifacts WHERE run_id = $1
	`, runID)

	out := &service.IntelligenceCheckArtifact{}
	if err := row.Scan(&out.RunID, &out.HTMLText, &out.ByteLen, &out.SHA256, &out.CreatedAt); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *intelligenceCheckRunRepository) PruneRuns(ctx context.Context, keepPerAccount int, before time.Time) (int64, error) {
	if keepPerAccount <= 0 {
		keepPerAccount = 100
	}
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM intelligence_check_runs
		WHERE id IN (
			SELECT id FROM (
				SELECT id, created_at,
				       ROW_NUMBER() OVER (PARTITION BY account_id ORDER BY created_at DESC, id DESC) AS rn
				FROM intelligence_check_runs
			) ranked
			WHERE rn > $1 OR created_at < $2
		)
	`, keepPerAccount, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- helpers ---

func buildIntelligenceCheckRunWhere(filter service.IntelligenceCheckRunFilter) (string, []any) {
	clauses := []string{"TRUE"}
	args := []any{}

	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}

	if len(filter.AccountIDs) > 0 {
		add("account_id = ANY($%d)", pq.Array(filter.AccountIDs))
	}
	if v := strings.TrimSpace(filter.Status); v != "" {
		add("status = $%d", v)
	}
	if v := strings.TrimSpace(filter.Verdict); v != "" {
		add("verdict = $%d", v)
	}
	if v := strings.TrimSpace(filter.ModelID); v != "" {
		add("model_id = $%d", v)
	}
	if filter.Since != nil {
		add("created_at >= $%d", *filter.Since)
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanIntelligenceCheckRun(row scannable) (*service.IntelligenceCheckRun, error) {
	run := &service.IntelligenceCheckRun{}
	if err := row.Scan(
		&run.ID, &run.AccountID, &run.BatchID, &run.TriggerSource, &run.ModelID, &run.UpstreamModel,
		&run.ReasoningEffort, &run.PromptVariant, &run.Status, &run.Verdict, &run.HasHTML, &run.HTMLBytes,
		&run.LatencyMs, &run.Attempt, &run.ErrorCode, &run.ErrorMessage, &run.StartedAt, &run.FinishedAt,
		&run.CreatedAt, &run.ReviewedBy, &run.ReviewedAt,
	); err != nil {
		return nil, err
	}
	return run, nil
}

func scanIntelligenceCheckRuns(rows *sql.Rows) ([]*service.IntelligenceCheckRun, error) {
	var runs []*service.IntelligenceCheckRun
	for rows.Next() {
		run, err := scanIntelligenceCheckRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

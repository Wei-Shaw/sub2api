package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type generationContentRepository struct {
	db *sql.DB
}

// NewGenerationContentRepository builds the raw-SQL repository for the M1 采集口
// side table ai_generation_content (mirrors the content_moderation_logs precedent:
// append-only, no ent schema/codegen).
func NewGenerationContentRepository(db *sql.DB) service.GenerationContentRepository {
	return &generationContentRepository{db: db}
}

func (r *generationContentRepository) Create(ctx context.Context, content *service.GenerationContent) error {
	if r == nil || r.db == nil || content == nil {
		return nil
	}
	var apiKeyID, userID, groupID, accountID any
	if content.APIKeyID != nil {
		apiKeyID = *content.APIKeyID
	}
	if content.UserID != nil {
		userID = *content.UserID
	}
	if content.GroupID != nil {
		groupID = *content.GroupID
	}
	if content.AccountID != nil {
		accountID = *content.AccountID
	}
	err := r.db.QueryRowContext(ctx, `
INSERT INTO ai_generation_content (
    request_id, api_key_id, user_id, group_id, account_id, model, request_payload_hash,
    prompt_redacted, response_redacted, prompt_bytes, response_bytes, response_truncated, redaction_version
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13
)
ON CONFLICT (api_key_id, request_id) WHERE request_id <> '' DO NOTHING
RETURNING id, created_at`,
		content.RequestID, apiKeyID, userID, groupID, accountID, content.Model, content.RequestPayloadHash,
		content.PromptRedacted, content.ResponseRedacted, content.PromptBytes, content.ResponseBytes, content.ResponseTruncated, content.RedactionVersion,
	).Scan(&content.ID, &content.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// 命中 (api_key_id, request_id) 唯一索引 → 该请求已采集，幂等 no-op。
		return nil
	}
	if err != nil {
		return fmt.Errorf("insert ai generation content: %w", err)
	}
	return nil
}

func (r *generationContentRepository) CreateVideoTaskContent(ctx context.Context, content *service.GenerationContent) error {
	if r == nil || r.db == nil || content == nil || content.TaskID == nil || *content.TaskID <= 0 {
		return nil
	}
	var apiKeyID, userID, groupID, accountID any
	if content.APIKeyID != nil {
		apiKeyID = *content.APIKeyID
	}
	if content.UserID != nil {
		userID = *content.UserID
	}
	if content.GroupID != nil {
		groupID = *content.GroupID
	}
	if content.AccountID != nil {
		accountID = *content.AccountID
	}
	err := r.db.QueryRowContext(ctx, `
INSERT INTO ai_generation_content (
    request_id, api_key_id, user_id, group_id, account_id, task_id, model, request_payload_hash,
    prompt_redacted, response_redacted, prompt_bytes, response_bytes, response_truncated, redaction_version
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14
)
ON CONFLICT (task_id) WHERE task_id IS NOT NULL DO NOTHING
RETURNING id, created_at`,
		content.RequestID, apiKeyID, userID, groupID, accountID, *content.TaskID, content.Model, content.RequestPayloadHash,
		content.PromptRedacted, content.ResponseRedacted, content.PromptBytes, content.ResponseBytes, content.ResponseTruncated, content.RedactionVersion,
	).Scan(&content.ID, &content.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// 命中 task_id 唯一索引 → 该视频任务已采集，幂等 no-op。
		return nil
	}
	if err != nil {
		return fmt.Errorf("insert video ai generation content: %w", err)
	}
	return nil
}

func (r *generationContentRepository) UpdateVideoTaskAdoption(ctx context.Context, input service.GenerationContentAdoptionInput) (*service.GenerationContentAdoption, error) {
	out := &service.GenerationContentAdoption{
		TaskID:         input.TaskID,
		AdoptionStatus: input.AdoptionStatus,
		QualityScore:   input.QualityScore,
		Notes:          input.Notes,
		Saved:          false,
	}
	if r == nil || r.db == nil || input.TaskID <= 0 {
		return out, nil
	}

	const q = `
UPDATE ai_generation_content
SET adoption_status = $2,
    quality_score = $3,
    adoption_notes = $4
WHERE task_id = $1
RETURNING task_id, adoption_status, quality_score, adoption_notes`
	var score sql.NullFloat64
	var notes sql.NullString
	err := r.db.QueryRowContext(ctx, q, input.TaskID, input.AdoptionStatus, input.QualityScore, input.Notes).
		Scan(&out.TaskID, &out.AdoptionStatus, &score, &notes)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update video task adoption: %w", err)
	}
	if score.Valid {
		out.QualityScore = &score.Float64
	} else {
		out.QualityScore = nil
	}
	if notes.Valid {
		out.Notes = notes.String
	}
	out.Saved = true
	return out, nil
}

// GetCaptureStats 聚合 ai_generation_content：计数/去重/体量 + 近 7 日每日序列。
// 全程只读；空表返回零值快照（非错误）。镜像 usage_log_repo 的 COUNT/SUM/COALESCE/TO_CHAR 套路。
func (r *generationContentRepository) GetCaptureStats(ctx context.Context) (*service.GenerationContentStats, error) {
	if r == nil || r.db == nil {
		return &service.GenerationContentStats{}, nil
	}
	stats := &service.GenerationContentStats{}

	const aggQuery = `
SELECT
    COUNT(*)                                                                          AS total,
    COUNT(*) FILTER (WHERE created_at >= date_trunc('day', NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC') AS captured_today,
    COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '7 days')                   AS captured_week,
    COUNT(DISTINCT user_id)                                                           AS distinct_employees,
    COUNT(DISTINCT group_id)                                                          AS distinct_teams,
    COUNT(DISTINCT NULLIF(model, ''))                                                 AS distinct_models,
    COALESCE(SUM(prompt_bytes + response_bytes), 0)                                   AS total_bytes
FROM ai_generation_content`
	if err := scanSingleRow(ctx, r.db, aggQuery, nil,
		&stats.Total,
		&stats.CapturedToday,
		&stats.CapturedWeek,
		&stats.DistinctEmployees,
		&stats.DistinctTeams,
		&stats.DistinctModels,
		&stats.TotalBytes,
	); err != nil {
		return nil, fmt.Errorf("generation content stats agg: %w", err)
	}

	const seriesQuery = `
SELECT
    TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS d,
    COUNT(*)                                             AS c
FROM ai_generation_content
WHERE created_at >= (date_trunc('day', NOW() AT TIME ZONE 'UTC') - INTERVAL '6 days') AT TIME ZONE 'UTC'
GROUP BY d
ORDER BY d ASC`
	rows, err := r.db.QueryContext(ctx, seriesQuery)
	if err != nil {
		return nil, fmt.Errorf("generation content daily series: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var p service.GenerationContentDailyPoint
		if err := rows.Scan(&p.Date, &p.Count); err != nil {
			return nil, fmt.Errorf("scan generation content daily: %w", err)
		}
		stats.DailySeries = append(stats.DailySeries, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate generation content daily: %w", err)
	}
	return stats, nil
}

func (r *generationContentRepository) GetWeeklyReport(ctx context.Context, start, end time.Time) (*service.GenerationContentWeeklyReport, error) {
	report := &service.GenerationContentWeeklyReport{
		PeriodStart: start,
		PeriodEnd:   end,
	}
	if r == nil || r.db == nil {
		return report, nil
	}
	q := dashboardUSDCNYRateCTE + `
SELECT
    COUNT(*) AS entries,
    COUNT(c.task_id) FILTER (WHERE c.task_id IS NOT NULL) AS video_tasks,
    COALESCE(SUM(` + dashboardVideoCostUSDExpr("vt") + `), 0)::float8 AS total_cost_estimate,
    COUNT(*) FILTER (WHERE c.adoption_status = 'adopted') AS adopted_count,
    COUNT(*) FILTER (WHERE c.adoption_status = 'rejected') AS rejected_count,
    COUNT(*) FILTER (WHERE c.adoption_status = 'pending') AS pending_count,
    COUNT(*) FILTER (WHERE COALESCE(c.adoption_status, '') = '') AS unreviewed_count,
    COUNT(*) FILTER (WHERE vt.status = 'failed') AS failed_task_count,
    COUNT(*) FILTER (WHERE c.task_id IS NOT NULL AND vt.id IS NULL) AS missing_task_join_count,
    COUNT(*) FILTER (WHERE c.response_truncated = TRUE) AS truncated_count
FROM ai_generation_content c
LEFT JOIN video_tasks vt ON vt.id = c.task_id
CROSS JOIN billing_rate
WHERE c.created_at >= $1 AND c.created_at < $2`
	if err := scanSingleRow(ctx, r.db, q, []any{start, end},
		&report.Entries,
		&report.VideoTasks,
		&report.TotalCostEstimate,
		&report.AdoptedCount,
		&report.RejectedCount,
		&report.PendingCount,
		&report.UnreviewedCount,
		&report.Anomalies.FailedTasks,
		&report.Anomalies.MissingTaskJoins,
		&report.Anomalies.TruncatedRows,
	); err != nil {
		return nil, fmt.Errorf("generation content weekly report: %w", err)
	}
	if report.Entries > 0 {
		report.AdoptionRate = float64(report.AdoptedCount) / float64(report.Entries)
	}
	return report, nil
}

// PurgeExpiredContent 保留期清理（NULL-OUT）：把 created_at < cutoff 且仍有内容的行的
// prompt_redacted/response_redacted 置空——保留行与计数（看板全时段指标持续累计），仅抹内容。
// 谓词含 (prompt_redacted <> ” OR response_redacted <> ”) 命中 partial index
// idx_ai_generation_content_unpurged_created_at，使已清空行不被重复扫描。
// 单批最多 batch 行；dryRun=true 只 COUNT（封顶 batch，与真实清理语义一致），零副作用。
// 镜像 idempotency_repo.go::DeleteExpired 的 CTE 批处理形，但用 UPDATE 而非 DELETE。
func (r *generationContentRepository) PurgeExpiredContent(ctx context.Context, cutoff time.Time, batch int, dryRun bool) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	if batch <= 0 {
		batch = 500
	}

	if dryRun {
		const countQuery = `
SELECT COUNT(*) FROM (
    SELECT 1
    FROM ai_generation_content
    WHERE created_at < $1 AND (prompt_redacted <> '' OR response_redacted <> '')
    ORDER BY created_at ASC
    LIMIT $2
) victims`
		var n int64
		if err := r.db.QueryRowContext(ctx, countQuery, cutoff, batch).Scan(&n); err != nil {
			return 0, fmt.Errorf("count ai generation content older than cutoff: %w", err)
		}
		return n, nil
	}

	const updateQuery = `
WITH victims AS (
    SELECT id
    FROM ai_generation_content
    WHERE created_at < $1 AND (prompt_redacted <> '' OR response_redacted <> '')
    ORDER BY created_at ASC
    LIMIT $2
)
UPDATE ai_generation_content
SET prompt_redacted = '', response_redacted = ''
WHERE id IN (SELECT id FROM victims)`
	res, err := r.db.ExecContext(ctx, updateQuery, cutoff, batch)
	if err != nil {
		return 0, fmt.Errorf("purge expired ai generation content: %w", err)
	}
	return res.RowsAffected()
}

// GetRecent 返回最近 limit 条采集样本，LEFT JOIN users/groups 取归因展示名（脱敏文本原样返回，截断由 handler 做）。
func (r *generationContentRepository) GetRecent(ctx context.Context, limit int) ([]service.GenerationContentSample, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	const query = `
SELECT
    c.task_id,
    c.model,
    c.created_at,
    c.prompt_redacted,
    c.response_redacted,
    c.prompt_bytes,
    c.response_bytes,
    c.response_truncated,
    COALESCE(u.username, ''),
    COALESCE(u.email, ''),
    COALESCE(g.name, ''),
    c.adoption_status,
    c.quality_score,
    c.adoption_notes,
    COALESCE(vt.status, ''),
    COALESCE(vt.cost_estimate, 0)::float8,
    COALESCE(NULLIF(vul.currency, ''), NULLIF(vt.currency, ''), 'USD')
FROM ai_generation_content c
LEFT JOIN users  u ON u.id = c.user_id
LEFT JOIN groups g ON g.id = c.group_id
LEFT JOIN video_tasks vt ON vt.id = c.task_id
LEFT JOIN video_usage_logs vul ON vul.video_task_id = c.task_id
ORDER BY c.created_at DESC
LIMIT $1`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("generation content recent: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []service.GenerationContentSample
	for rows.Next() {
		var s service.GenerationContentSample
		var taskID sql.NullInt64
		var score sql.NullFloat64
		if err := rows.Scan(
			&taskID,
			&s.Model,
			&s.CreatedAt,
			&s.PromptRedacted,
			&s.ResponseRedacted,
			&s.PromptBytes,
			&s.ResponseBytes,
			&s.ResponseTruncated,
			&s.Username,
			&s.Email,
			&s.GroupName,
			&s.AdoptionStatus,
			&score,
			&s.AdoptionNotes,
			&s.VideoStatus,
			&s.CostEstimate,
			&s.Currency,
		); err != nil {
			return nil, fmt.Errorf("scan generation content recent: %w", err)
		}
		if taskID.Valid {
			s.TaskID = &taskID.Int64
		}
		if score.Valid {
			s.QualityScore = &score.Float64
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate generation content recent: %w", err)
	}
	return out, nil
}

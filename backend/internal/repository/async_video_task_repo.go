package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type asyncVideoTaskRepository struct {
	sql sqlExecutor
}

// NewAsyncVideoTaskRepository 创建视频异步任务仓储。
func NewAsyncVideoTaskRepository(_ *dbent.Client, sqlDB *sql.DB) service.AsyncVideoTaskRepository {
	return &asyncVideoTaskRepository{sql: sqlDB}
}

const asyncVideoTaskColumns = `
	id, internal_request_id, upstream_request_id, status_url, response_url,
	account_id, api_key_id, user_id, organization_id, payer_user_id, balance_source, authz_generation, group_id, channel_id,
	facade, requested_model, upstream_model,
	resolution, duration_seconds, aspect_ratio,
	status, held_cost, final_cost, rate_multiplier, unit_price_snapshot, upstream_cost,
	request_payload, result_payload, video_urls, cos_urls,
	error_reason, fail_deadline_at, finished_at,
	client_ip, user_agent, inbound_endpoint, upstream_endpoint,
	created_at, updated_at`

func (r *asyncVideoTaskRepository) Create(ctx context.Context, task *service.AsyncVideoTask) error {
	if task == nil {
		return errors.New("nil async video task")
	}
	if task.Status == "" {
		task.Status = service.AsyncVideoStatusPending
	}
	if task.Facade == "" {
		task.Facade = service.AsyncVideoFacadeFal
	}
	if task.RateMultiplier == 0 {
		task.RateMultiplier = 1
	}
	requestJSON, err := marshalAnyMap(task.RequestPayload)
	if err != nil {
		return fmt.Errorf("marshal request_payload: %w", err)
	}
	resultJSON, err := marshalAnyMap(task.ResultPayload)
	if err != nil {
		return fmt.Errorf("marshal result_payload: %w", err)
	}
	videoURLsJSON, err := marshalStringSlice(task.VideoURLs)
	if err != nil {
		return fmt.Errorf("marshal video_urls: %w", err)
	}
	cosURLsJSON, err := marshalStringSlice(task.CosURLs)
	if err != nil {
		return fmt.Errorf("marshal cos_urls: %w", err)
	}

	query := `
		INSERT INTO async_video_tasks (
			internal_request_id, upstream_request_id, status_url, response_url,
			account_id, api_key_id, user_id, organization_id, payer_user_id, balance_source, authz_generation, group_id, channel_id,
			facade, requested_model, upstream_model,
			resolution, duration_seconds, aspect_ratio,
			status, held_cost, final_cost, rate_multiplier, unit_price_snapshot, upstream_cost,
			request_payload, result_payload, video_urls, cos_urls,
			error_reason, fail_deadline_at, finished_at,
			client_ip, user_agent, inbound_endpoint, upstream_endpoint
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9, $10, $11, $12, $13,
			$14, $15, $16,
			$17, $18, $19,
			$20, $21, $22, $23, $24, $25,
			$26, $27, $28, $29,
			$30, $31, $32,
			$33, $34, $35, $36
		) RETURNING id, created_at, updated_at`

	return scanSingleRow(ctx, r.sql, query, []any{
		task.InternalRequestID, task.UpstreamRequestID, task.StatusURL, task.ResponseURL,
		task.AccountID, task.APIKeyID, task.UserID, task.OrganizationID, task.PayerUserID, task.BalanceSource, task.AuthzGeneration, task.GroupID, task.ChannelID,
		task.Facade, task.RequestedModel, task.UpstreamModel,
		task.Resolution, task.DurationSeconds, task.AspectRatio,
		task.Status, task.HeldCost, task.FinalCost, task.RateMultiplier, task.UnitPriceSnapshot, task.UpstreamCost,
		requestJSON, resultJSON, videoURLsJSON, cosURLsJSON,
		task.ErrorReason, task.FailDeadlineAt, task.FinishedAt,
		task.ClientIP, task.UserAgent, task.InboundEndpoint, task.UpstreamEndpoint,
	}, &task.ID, &task.CreatedAt, &task.UpdatedAt)
}

func (r *asyncVideoTaskRepository) GetByID(ctx context.Context, id int64) (*service.AsyncVideoTask, error) {
	return r.queryOne(ctx, `SELECT `+asyncVideoTaskColumns+` FROM async_video_tasks WHERE id = $1`, id)
}

func (r *asyncVideoTaskRepository) GetByInternalRequestID(ctx context.Context, internalRequestID string) (*service.AsyncVideoTask, error) {
	return r.queryOne(ctx, `SELECT `+asyncVideoTaskColumns+` FROM async_video_tasks WHERE internal_request_id = $1`, internalRequestID)
}

func (r *asyncVideoTaskRepository) GetByUpstreamRequestID(ctx context.Context, upstreamRequestID string) (*service.AsyncVideoTask, error) {
	return r.queryOne(ctx, `SELECT `+asyncVideoTaskColumns+` FROM async_video_tasks WHERE upstream_request_id = $1 ORDER BY id DESC LIMIT 1`, upstreamRequestID)
}

func (r *asyncVideoTaskRepository) UpdateUpstreamRef(ctx context.Context, id int64, upstreamRequestID, statusURL, responseURL string) error {
	query := `
		UPDATE async_video_tasks
		SET upstream_request_id = $2,
			status_url = $3,
			response_url = $4,
			status = CASE WHEN status = $5 THEN $6 ELSE status END,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.sql.ExecContext(ctx, query,
		id,
		nullIfEmpty(upstreamRequestID),
		nullIfEmpty(statusURL),
		nullIfEmpty(responseURL),
		service.AsyncVideoStatusPending,
		service.AsyncVideoStatusRunning,
	)
	return err
}

func (r *asyncVideoTaskRepository) MarkSucceeded(ctx context.Context, id int64, videoURLs, cosURLs []string, resultPayload map[string]any, finalCost float64, durationSeconds int, upstreamCost float64) (bool, error) {
	videoURLsJSON, err := marshalStringSlice(videoURLs)
	if err != nil {
		return false, fmt.Errorf("marshal video_urls: %w", err)
	}
	cosURLsJSON, err := marshalStringSlice(cosURLs)
	if err != nil {
		return false, fmt.Errorf("marshal cos_urls: %w", err)
	}
	resultJSON, err := marshalAnyMap(resultPayload)
	if err != nil {
		return false, fmt.Errorf("marshal result_payload: %w", err)
	}
	// duration_seconds<=0 时保留原库值（例如上游没返回时长），否则以实际值覆盖。
	// upstream_cost<=0 时保留原库值（例如 fal/atlascloud 未回传真实成本），
	// 只有 apiz 这类回传 price 的平台才会 >0 覆盖。
	query := `
		UPDATE async_video_tasks
		SET status = $2,
			video_urls = $3,
			cos_urls = $4,
			result_payload = $5,
			final_cost = $6,
			duration_seconds = CASE WHEN $10::int > 0 THEN $10 ELSE duration_seconds END,
			upstream_cost = CASE WHEN $11::numeric > 0 THEN $11 ELSE upstream_cost END,
			error_reason = NULL,
			finished_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
			AND status NOT IN ($7, $8, $9)`
	res, err := r.sql.ExecContext(ctx, query,
		id,
		service.AsyncVideoStatusSucceeded,
		videoURLsJSON,
		cosURLsJSON,
		resultJSON,
		finalCost,
		service.AsyncVideoStatusSucceeded,
		service.AsyncVideoStatusRefunded,
		service.AsyncVideoStatusExpired,
		durationSeconds,
		upstreamCost,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *asyncVideoTaskRepository) MarkRefunded(ctx context.Context, id int64, status, errorReason string) (bool, error) {
	if status == "" {
		status = service.AsyncVideoStatusRefunded
	}
	query := `
		UPDATE async_video_tasks
		SET status = $2,
			final_cost = 0,
			error_reason = $3,
			finished_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
			AND status NOT IN ($4, $5, $6)`
	res, err := r.sql.ExecContext(ctx, query,
		id,
		status,
		nullIfEmpty(errorReason),
		service.AsyncVideoStatusSucceeded,
		service.AsyncVideoStatusRefunded,
		service.AsyncVideoStatusExpired,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *asyncVideoTaskRepository) ListUnfinished(ctx context.Context, limit int) ([]*service.AsyncVideoTask, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `SELECT ` + asyncVideoTaskColumns + `
		FROM async_video_tasks
		WHERE status IN ($1, $2)
		ORDER BY created_at ASC
		LIMIT $3`
	rows, err := r.sql.QueryContext(ctx, query,
		service.AsyncVideoStatusPending,
		service.AsyncVideoStatusRunning,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*service.AsyncVideoTask
	for rows.Next() {
		task, err := scanAsyncVideoTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ListByUserAndSlug 分页列出某用户在指定模型 slug 下的历史任务。
// slug 为空时列出该用户所有视频任务。按 created_at DESC 排序。
func (r *asyncVideoTaskRepository) ListByUserAndSlug(ctx context.Context, userID int64, slug string, offset, limit int) ([]*service.AsyncVideoTask, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	whereClauses := []string{"user_id = $1"}
	args := []any{userID}
	if s := strings.TrimSpace(slug); s != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("requested_model = $%d", len(args)+1))
		args = append(args, s)
	}
	whereSQL := "WHERE " + strings.Join(whereClauses, " AND ")

	// 总数（用于分页）— 用 QueryContext + Scan 是因为 sqlExecutor 接口不含 QueryRowContext。
	var total int64
	countRows, err := r.sql.QueryContext(ctx, `SELECT COUNT(*) FROM async_video_tasks `+whereSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			_ = countRows.Close()
			return nil, 0, err
		}
	}
	if err := countRows.Err(); err != nil {
		_ = countRows.Close()
		return nil, 0, err
	}
	_ = countRows.Close()
	if total == 0 {
		return nil, 0, nil
	}

	listArgs := append([]any(nil), args...)
	listArgs = append(listArgs, limit, offset)
	query := `SELECT ` + asyncVideoTaskColumns + ` FROM async_video_tasks ` + whereSQL +
		fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", len(listArgs)-1, len(listArgs))

	rows, err := r.sql.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]*service.AsyncVideoTask, 0, limit)
	for rows.Next() {
		task, err := scanAsyncVideoTask(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, task)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

func (r *asyncVideoTaskRepository) InsertTerminalUsageLog(ctx context.Context, in *service.VideoTerminalUsageLogInput) (bool, error) {
	if in == nil {
		return false, errors.New("nil video terminal usage log input")
	}
	if in.RateMultiplier == 0 {
		in.RateMultiplier = 1
	}
	model := in.Model
	if model == "" {
		model = in.UpstreamModel
	}
	if model == "" {
		model = in.RequestedModel
	}

	videoURLsJSON, err := marshalStringSlice(in.VideoURLs)
	if err != nil {
		return false, fmt.Errorf("marshal video_urls: %w", err)
	}
	cosURLsJSON, err := marshalStringSlice(in.CosURLs)
	if err != nil {
		return false, fmt.Errorf("marshal cos_url: %w", err)
	}

	var taskID any
	if in.TaskID > 0 {
		taskID = in.TaskID
	}
	var durationMs any
	if in.DurationMs > 0 {
		durationMs = in.DurationMs
	}

	// 视频复用 usage_logs 表，写入语义化字段：
	//   - billing_mode = "video"（前端据此走视频分支，不会被 isImageUsage 误判为图片）
	//   - video_count = 1
	//   - video_resolution = <resolution>（例如 720p）
	//   - video_duration_seconds = <duration>
	//   - billing_tier = <resolution>（与 image 一致的层级 tag 语义）
	// image_urls / cos_url 继续复用为"媒体 URL 列表"存视频地址，避免为视频再加一列。
	var videoResolution any
	if r := nullIfEmpty(in.Resolution); r != nil {
		videoResolution = r
	}
	var videoDuration any
	if in.DurationSeconds > 0 {
		videoDuration = in.DurationSeconds
	}
	billingTier := nullIfEmpty(in.Resolution)

	const query = `
		INSERT INTO usage_logs (
			user_id, api_key_id, account_id, request_id,
			model, requested_model, upstream_model,
			group_id, channel_id,
			total_cost, actual_cost, rate_multiplier,
			billing_type, request_type,
			video_count, video_resolution, video_duration_seconds,
			billing_mode, billing_tier,
			task_id, image_urls, cos_url, billing_status,
			inbound_endpoint, upstream_endpoint, duration_ms, ip_address, user_agent,
			organization_id, payer_user_id, balance_source, authz_generation,
			created_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9,
			$10, $11, $12,
			$13, $14,
			$15, $16, $17,
			$18, $19,
			$20, $21, $22, $23,
			$24, $25, $26, $27, $28,
			$29, $30, $31, $32,
			NOW()
		)
		ON CONFLICT (request_id, api_key_id) DO NOTHING`

	res, err := r.sql.ExecContext(ctx, query,
		in.UserID, in.APIKeyID, in.AccountID, nullIfEmpty(in.RequestID),
		model, nullIfEmpty(in.RequestedModel), nullIfEmpty(in.UpstreamModel),
		in.GroupID, in.ChannelID,
		in.TotalCost, in.ActualCost, in.RateMultiplier,
		int16(in.BillingType), in.RequestType,
		1, videoResolution, videoDuration,
		string(service.BillingModeVideo), billingTier,
		taskID, videoURLsJSON, cosURLsJSON, nullIfEmpty(in.BillingStatus),
		nullIfEmpty(in.InboundEndpoint), nullIfEmpty(in.UpstreamEndpoint), durationMs, nullIfEmpty(in.ClientIP), nullIfEmpty(in.UserAgent),
		in.OrganizationID, in.PayerUserID, in.BalanceSource, in.AuthzGeneration,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *asyncVideoTaskRepository) queryOne(ctx context.Context, query string, args ...any) (*service.AsyncVideoTask, error) {
	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	return scanAsyncVideoTask(rows)
}

func scanAsyncVideoTask(rows *sql.Rows) (*service.AsyncVideoTask, error) {
	task := &service.AsyncVideoTask{}
	var requestJSON, resultJSON, videoURLsJSON, cosURLsJSON []byte
	if err := rows.Scan(
		&task.ID, &task.InternalRequestID, &task.UpstreamRequestID, &task.StatusURL, &task.ResponseURL,
		&task.AccountID, &task.APIKeyID, &task.UserID, &task.OrganizationID, &task.PayerUserID, &task.BalanceSource, &task.AuthzGeneration, &task.GroupID, &task.ChannelID,
		&task.Facade, &task.RequestedModel, &task.UpstreamModel,
		&task.Resolution, &task.DurationSeconds, &task.AspectRatio,
		&task.Status, &task.HeldCost, &task.FinalCost, &task.RateMultiplier, &task.UnitPriceSnapshot, &task.UpstreamCost,
		&requestJSON, &resultJSON, &videoURLsJSON, &cosURLsJSON,
		&task.ErrorReason, &task.FailDeadlineAt, &task.FinishedAt,
		&task.ClientIP, &task.UserAgent, &task.InboundEndpoint, &task.UpstreamEndpoint,
		&task.CreatedAt, &task.UpdatedAt,
	); err != nil {
		return nil, err
	}
	task.RequestPayload = unmarshalAnyMap(requestJSON)
	task.ResultPayload = unmarshalAnyMap(resultJSON)
	task.VideoURLs = unmarshalStringSlice(videoURLsJSON)
	task.CosURLs = unmarshalStringSlice(cosURLsJSON)
	return task, nil
}

// marshalAnyMap 序列化任意 map；空值返回 nil（写入 NULL/'{}'）。
func marshalAnyMap(m map[string]any) (any, error) {
	if len(m) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func unmarshalAnyMap(b []byte) map[string]any {
	if len(b) == 0 {
		return nil
	}
	out := make(map[string]any)
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

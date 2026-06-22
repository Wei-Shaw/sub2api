package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type asyncMediaTaskRepository struct {
	sql sqlExecutor
}

// NewAsyncMediaTaskRepository 创建异步媒体任务仓储。
func NewAsyncMediaTaskRepository(_ *dbent.Client, sqlDB *sql.DB) service.AsyncMediaTaskRepository {
	return &asyncMediaTaskRepository{sql: sqlDB}
}

const asyncMediaTaskColumns = `
	id, internal_request_id, upstream_request_id, status_url, response_url,
	account_id, api_key_id, user_id, group_id, channel_id,
	facade, requested_model, upstream_model,
	image_size, quality, num_images,
	status, held_cost, final_cost, rate_multiplier, size_tier,
	image_urls, cos_urls,
	error_reason, fail_deadline_at, finished_at,
	client_ip, user_agent, inbound_endpoint, upstream_endpoint,
	created_at, updated_at`

func (r *asyncMediaTaskRepository) Create(ctx context.Context, task *service.AsyncMediaTask) error {
	if task == nil {
		return errors.New("nil async media task")
	}
	if task.Status == "" {
		task.Status = service.AsyncMediaStatusPending
	}
	if task.Facade == "" {
		task.Facade = service.AsyncMediaFacadeOpenAI
	}
	if task.NumImages <= 0 {
		task.NumImages = 1
	}
	if task.RateMultiplier == 0 {
		task.RateMultiplier = 1
	}

	imageURLsJSON, err := marshalStringSlice(task.ImageURLs)
	if err != nil {
		return fmt.Errorf("marshal image_urls: %w", err)
	}
	cosURLsJSON, err := marshalStringSlice(task.CosURLs)
	if err != nil {
		return fmt.Errorf("marshal cos_urls: %w", err)
	}

	query := `
		INSERT INTO async_media_tasks (
			internal_request_id, upstream_request_id, status_url, response_url,
			account_id, api_key_id, user_id, group_id, channel_id,
			facade, requested_model, upstream_model,
			image_size, quality, num_images,
			status, held_cost, final_cost, rate_multiplier, size_tier,
			image_urls, cos_urls,
			error_reason, fail_deadline_at, finished_at,
			client_ip, user_agent, inbound_endpoint, upstream_endpoint
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9,
			$10, $11, $12,
			$13, $14, $15,
			$16, $17, $18, $19, $20,
			$21, $22,
			$23, $24, $25,
			$26, $27, $28, $29
		) RETURNING id, created_at, updated_at`

	return scanSingleRow(ctx, r.sql, query, []any{
		task.InternalRequestID, task.UpstreamRequestID, task.StatusURL, task.ResponseURL,
		task.AccountID, task.APIKeyID, task.UserID, task.GroupID, task.ChannelID,
		task.Facade, task.RequestedModel, task.UpstreamModel,
		task.ImageSize, task.Quality, task.NumImages,
		task.Status, task.HeldCost, task.FinalCost, task.RateMultiplier, task.SizeTier,
		imageURLsJSON, cosURLsJSON,
		task.ErrorReason, task.FailDeadlineAt, task.FinishedAt,
		task.ClientIP, task.UserAgent, task.InboundEndpoint, task.UpstreamEndpoint,
	}, &task.ID, &task.CreatedAt, &task.UpdatedAt)
}

func (r *asyncMediaTaskRepository) GetByID(ctx context.Context, id int64) (*service.AsyncMediaTask, error) {
	return r.queryOne(ctx, `SELECT `+asyncMediaTaskColumns+` FROM async_media_tasks WHERE id = $1`, id)
}

func (r *asyncMediaTaskRepository) GetByInternalRequestID(ctx context.Context, internalRequestID string) (*service.AsyncMediaTask, error) {
	return r.queryOne(ctx, `SELECT `+asyncMediaTaskColumns+` FROM async_media_tasks WHERE internal_request_id = $1`, internalRequestID)
}

func (r *asyncMediaTaskRepository) GetByUpstreamRequestID(ctx context.Context, upstreamRequestID string) (*service.AsyncMediaTask, error) {
	return r.queryOne(ctx, `SELECT `+asyncMediaTaskColumns+` FROM async_media_tasks WHERE upstream_request_id = $1 ORDER BY id DESC LIMIT 1`, upstreamRequestID)
}

func (r *asyncMediaTaskRepository) UpdateUpstreamRef(ctx context.Context, id int64, upstreamRequestID, statusURL, responseURL string) error {
	query := `
		UPDATE async_media_tasks
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
		service.AsyncMediaStatusPending,
		service.AsyncMediaStatusRunning,
	)
	return err
}

func (r *asyncMediaTaskRepository) MarkSucceeded(ctx context.Context, id int64, imageURLs, cosURLs []string, finalCost float64) (bool, error) {
	imageURLsJSON, err := marshalStringSlice(imageURLs)
	if err != nil {
		return false, fmt.Errorf("marshal image_urls: %w", err)
	}
	cosURLsJSON, err := marshalStringSlice(cosURLs)
	if err != nil {
		return false, fmt.Errorf("marshal cos_urls: %w", err)
	}
	query := `
		UPDATE async_media_tasks
		SET status = $2,
			image_urls = $3,
			cos_urls = $4,
			final_cost = $5,
			error_reason = NULL,
			finished_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
			AND status NOT IN ($6, $7, $8)`
	res, err := r.sql.ExecContext(ctx, query,
		id,
		service.AsyncMediaStatusSucceeded,
		imageURLsJSON,
		cosURLsJSON,
		finalCost,
		service.AsyncMediaStatusSucceeded,
		service.AsyncMediaStatusRefunded,
		service.AsyncMediaStatusExpired,
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

func (r *asyncMediaTaskRepository) MarkRefunded(ctx context.Context, id int64, status, errorReason string) (bool, error) {
	if status == "" {
		status = service.AsyncMediaStatusRefunded
	}
	query := `
		UPDATE async_media_tasks
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
		service.AsyncMediaStatusSucceeded,
		service.AsyncMediaStatusRefunded,
		service.AsyncMediaStatusExpired,
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

func (r *asyncMediaTaskRepository) ListUnfinished(ctx context.Context, limit int) ([]*service.AsyncMediaTask, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `SELECT ` + asyncMediaTaskColumns + `
		FROM async_media_tasks
		WHERE status IN ($1, $2)
		ORDER BY created_at ASC
		LIMIT $3`
	rows, err := r.sql.QueryContext(ctx, query,
		service.AsyncMediaStatusPending,
		service.AsyncMediaStatusRunning,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*service.AsyncMediaTask
	for rows.Next() {
		task, err := scanAsyncMediaTask(rows)
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

// InsertTerminalUsageLog 终态追加写一条 usage_log。
// 仅写入异步图片任务所需的核心计费列与新增异步列（task_id/image_urls/cos_url/billing_status），
// 其余列依赖 usage_logs 的默认值。该 INSERT 与高并发批处理写入路径完全隔离。
func (r *asyncMediaTaskRepository) InsertTerminalUsageLog(ctx context.Context, in *service.TerminalUsageLogInput) (bool, error) {
	if in == nil {
		return false, errors.New("nil terminal usage log input")
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

	imageURLsJSON, err := marshalStringSlice(in.ImageURLs)
	if err != nil {
		return false, fmt.Errorf("marshal image_urls: %w", err)
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

	const query = `
		INSERT INTO usage_logs (
			user_id, api_key_id, account_id, request_id,
			model, requested_model, upstream_model,
			group_id, channel_id,
			total_cost, actual_cost, rate_multiplier,
			billing_type, request_type,
			image_count, image_size,
			billing_mode, billing_tier,
			task_id, image_urls, cos_url, billing_status,
			inbound_endpoint, upstream_endpoint, duration_ms, ip_address, user_agent,
			created_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9,
			$10, $11, $12,
			$13, $14,
			$15, $16,
			$17, $18,
			$19, $20, $21, $22,
			$23, $24, $25, $26, $27,
			NOW()
		)
		ON CONFLICT (request_id, api_key_id) DO NOTHING`

	res, err := r.sql.ExecContext(ctx, query,
		in.UserID, in.APIKeyID, in.AccountID, nullIfEmpty(in.RequestID),
		model, nullIfEmpty(in.RequestedModel), nullIfEmpty(in.UpstreamModel),
		in.GroupID, in.ChannelID,
		in.TotalCost, in.ActualCost, in.RateMultiplier,
		int16(in.BillingType), in.RequestType,
		in.ImageCount, nullIfEmpty(in.ImageSize),
		string(service.BillingModeImage), nullIfEmpty(in.BillingTier),
		taskID, imageURLsJSON, cosURLsJSON, nullIfEmpty(in.BillingStatus),
		nullIfEmpty(in.InboundEndpoint), nullIfEmpty(in.UpstreamEndpoint), durationMs, nullIfEmpty(in.ClientIP), nullIfEmpty(in.UserAgent),
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

func (r *asyncMediaTaskRepository) queryOne(ctx context.Context, query string, args ...any) (*service.AsyncMediaTask, error) {
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
	return scanAsyncMediaTask(rows)
}

func scanAsyncMediaTask(rows *sql.Rows) (*service.AsyncMediaTask, error) {
	task := &service.AsyncMediaTask{}
	var imageURLsJSON, cosURLsJSON []byte
	if err := rows.Scan(
		&task.ID, &task.InternalRequestID, &task.UpstreamRequestID, &task.StatusURL, &task.ResponseURL,
		&task.AccountID, &task.APIKeyID, &task.UserID, &task.GroupID, &task.ChannelID,
		&task.Facade, &task.RequestedModel, &task.UpstreamModel,
		&task.ImageSize, &task.Quality, &task.NumImages,
		&task.Status, &task.HeldCost, &task.FinalCost, &task.RateMultiplier, &task.SizeTier,
		&imageURLsJSON, &cosURLsJSON,
		&task.ErrorReason, &task.FailDeadlineAt, &task.FinishedAt,
		&task.ClientIP, &task.UserAgent, &task.InboundEndpoint, &task.UpstreamEndpoint,
		&task.CreatedAt, &task.UpdatedAt,
	); err != nil {
		return nil, err
	}
	task.ImageURLs = unmarshalStringSlice(imageURLsJSON)
	task.CosURLs = unmarshalStringSlice(cosURLsJSON)
	return task, nil
}

// marshalStringSlice 将字符串切片序列化为 JSON；空切片/ nil 返回 nil（写入 NULL）。
func marshalStringSlice(s []string) (any, error) {
	if len(s) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func unmarshalStringSlice(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

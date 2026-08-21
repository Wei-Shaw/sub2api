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
	account_id, api_key_id, user_id, organization_id, payer_user_id, balance_source, authz_generation, group_id, channel_id,
	facade, requested_model, upstream_model,
	image_size, quality, num_images,
	status, held_cost, final_cost, rate_multiplier, size_tier,
	image_urls, cos_urls, image_metadata,
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

	imageURLsJSON, err := marshalStringSlice(task.ImageURLs)
	if err != nil {
		return fmt.Errorf("marshal image_urls: %w", err)
	}
	cosURLsJSON, err := marshalStringSlice(task.CosURLs)
	if err != nil {
		return fmt.Errorf("marshal cos_urls: %w", err)
	}
	imageMetadataJSON, err := json.Marshal(task.ImageMetadata)
	if err != nil {
		return fmt.Errorf("marshal image_metadata: %w", err)
	}

	query := `
		INSERT INTO async_media_tasks (
			internal_request_id, upstream_request_id, status_url, response_url,
			account_id, api_key_id, user_id, organization_id, payer_user_id, balance_source, authz_generation, group_id, channel_id,
			facade, requested_model, upstream_model,
			image_size, quality, num_images,
			status, held_cost, final_cost, rate_multiplier, size_tier,
			image_urls, cos_urls, image_metadata,
			error_reason, fail_deadline_at, finished_at,
			client_ip, user_agent, inbound_endpoint, upstream_endpoint
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9, $10, $11, $12, $13,
			$14, $15, $16,
			$17, $18, $19,
			$20, $21, $22, $23, $24,
			$25, $26, $27,
			$28, $29, $30,
			$31, $32, $33, $34
		) RETURNING id, created_at, updated_at`

	return scanSingleRow(ctx, r.sql, query, []any{
		task.InternalRequestID, task.UpstreamRequestID, task.StatusURL, task.ResponseURL,
		task.AccountID, task.APIKeyID, task.UserID, task.OrganizationID, task.PayerUserID, task.BalanceSource, task.AuthzGeneration, task.GroupID, task.ChannelID,
		task.Facade, task.RequestedModel, task.UpstreamModel,
		task.ImageSize, task.Quality, task.NumImages,
		task.Status, task.HeldCost, task.FinalCost, task.RateMultiplier, task.SizeTier,
		imageURLsJSON, cosURLsJSON, imageMetadataJSON,
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

func (r *asyncMediaTaskRepository) ListByUserAndModel(ctx context.Context, userID int64, requestedModel string, offset, limit int) ([]*service.AsyncMediaTask, int64, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}
	const where = ` FROM async_media_tasks WHERE user_id = $1 AND requested_model = $2`
	countRows, err := r.sql.QueryContext(ctx, `SELECT COUNT(*)`+where, userID, requestedModel)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = countRows.Close() }()
	var total int64
	if !countRows.Next() {
		if err := countRows.Err(); err != nil {
			return nil, 0, err
		}
		return nil, 0, errors.New("count image tasks returned no rows")
	}
	if err := countRows.Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT `+asyncMediaTaskColumns+where+` ORDER BY created_at DESC, id DESC LIMIT $3 OFFSET $4`, userID, requestedModel, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]*service.AsyncMediaTask, 0)
	for rows.Next() {
		task, scanErr := scanAsyncMediaTask(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, task)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
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

func (r *asyncMediaTaskRepository) MarkSucceeded(ctx context.Context, id int64, imageURLs, cosURLs []string, imageMetadata []service.ImageOutputMetadata, finalCost float64) (bool, error) {
	imageURLsJSON, err := marshalStringSlice(imageURLs)
	if err != nil {
		return false, fmt.Errorf("marshal image_urls: %w", err)
	}
	cosURLsJSON, err := marshalStringSlice(cosURLs)
	if err != nil {
		return false, fmt.Errorf("marshal cos_urls: %w", err)
	}
	imageMetadataJSON, err := json.Marshal(imageMetadata)
	if err != nil {
		return false, fmt.Errorf("marshal image_metadata: %w", err)
	}
	query := `
		UPDATE async_media_tasks
		SET status = $2,
			image_urls = $3,
			cos_urls = $4,
			image_metadata = $5,
			final_cost = $6,
			error_reason = NULL,
			finished_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
			AND status NOT IN ($7, $8, $9)`
	res, err := r.sql.ExecContext(ctx, query,
		id,
		service.AsyncMediaStatusSucceeded,
		imageURLsJSON,
		cosURLsJSON,
		imageMetadataJSON,
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
			image_count, image_size, image_input_size, image_quality,
			image_output_size, image_size_source, image_size_breakdown,
			billing_mode, billing_tier,
			task_id, image_urls, cos_url, billing_status,
			inbound_endpoint, upstream_endpoint, duration_ms, ip_address, user_agent,
			organization_id, payer_user_id, balance_source, authz_generation, account_rate_multiplier,
			created_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9,
			$10, $11, $12,
			$13, $14,
			$15, $16, $17, $18,
			$19, $20, $21,
			$22, $23,
			$24, $25, $26, $27,
			$28, $29, $30, $31, $32,
			$33, $34, $35, $36, $37,
			NOW()
		)
		ON CONFLICT (request_id, api_key_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			account_id = EXCLUDED.account_id,
			model = EXCLUDED.model,
			requested_model = EXCLUDED.requested_model,
			upstream_model = EXCLUDED.upstream_model,
			group_id = EXCLUDED.group_id,
			channel_id = EXCLUDED.channel_id,
			total_cost = EXCLUDED.total_cost,
			actual_cost = EXCLUDED.actual_cost,
			rate_multiplier = EXCLUDED.rate_multiplier,
			account_rate_multiplier = EXCLUDED.account_rate_multiplier,
			billing_type = EXCLUDED.billing_type,
			request_type = EXCLUDED.request_type,
			image_count = EXCLUDED.image_count,
			image_size = EXCLUDED.image_size,
			image_input_size = EXCLUDED.image_input_size,
			image_quality = EXCLUDED.image_quality,
			image_output_size = EXCLUDED.image_output_size,
			image_size_source = EXCLUDED.image_size_source,
			image_size_breakdown = EXCLUDED.image_size_breakdown,
			billing_mode = EXCLUDED.billing_mode,
			billing_tier = EXCLUDED.billing_tier,
			task_id = EXCLUDED.task_id,
			image_urls = EXCLUDED.image_urls,
			cos_url = EXCLUDED.cos_url,
			billing_status = EXCLUDED.billing_status,
			inbound_endpoint = EXCLUDED.inbound_endpoint,
			upstream_endpoint = EXCLUDED.upstream_endpoint,
			duration_ms = EXCLUDED.duration_ms,
			ip_address = EXCLUDED.ip_address,
			user_agent = EXCLUDED.user_agent,
			organization_id = EXCLUDED.organization_id,
			payer_user_id = EXCLUDED.payer_user_id,
			balance_source = EXCLUDED.balance_source,
			authz_generation = EXCLUDED.authz_generation
		WHERE usage_logs.task_id = EXCLUDED.task_id
			OR (
				EXCLUDED.billing_status IN ('charged', 'refunded')
				AND COALESCE(usage_logs.actual_cost, 0) = 0
				AND COALESCE(usage_logs.total_cost, 0) = 0
			)`

	res, err := r.sql.ExecContext(ctx, query,
		in.UserID, in.APIKeyID, in.AccountID, nullIfEmpty(in.RequestID),
		model, nullIfEmpty(in.RequestedModel), nullIfEmpty(in.UpstreamModel),
		in.GroupID, in.ChannelID,
		in.TotalCost, in.ActualCost, in.RateMultiplier,
		int16(in.BillingType), in.RequestType,
		in.ImageCount, nullIfEmpty(in.ImageSize), nullIfEmpty(in.ImageInputSize), nullIfEmpty(in.ImageQuality),
		nullIfEmpty(in.ImageOutputSize), nullIfEmpty(in.ImageSizeSource), nullStringIntMapJSON(in.ImageSizeBreakdown),
		string(service.BillingModeImage), nullIfEmpty(in.BillingTier),
		taskID, imageURLsJSON, cosURLsJSON, nullIfEmpty(in.BillingStatus),
		nullIfEmpty(in.InboundEndpoint), nullIfEmpty(in.UpstreamEndpoint), durationMs, nullIfEmpty(in.ClientIP), nullIfEmpty(in.UserAgent),
		in.OrganizationID, in.PayerUserID, in.BalanceSource, in.AuthzGeneration, in.AccountRateMultiplier,
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
	var imageURLsJSON, cosURLsJSON, imageMetadataJSON []byte
	if err := rows.Scan(
		&task.ID, &task.InternalRequestID, &task.UpstreamRequestID, &task.StatusURL, &task.ResponseURL,
		&task.AccountID, &task.APIKeyID, &task.UserID, &task.OrganizationID, &task.PayerUserID, &task.BalanceSource, &task.AuthzGeneration, &task.GroupID, &task.ChannelID,
		&task.Facade, &task.RequestedModel, &task.UpstreamModel,
		&task.ImageSize, &task.Quality, &task.NumImages,
		&task.Status, &task.HeldCost, &task.FinalCost, &task.RateMultiplier, &task.SizeTier,
		&imageURLsJSON, &cosURLsJSON, &imageMetadataJSON,
		&task.ErrorReason, &task.FailDeadlineAt, &task.FinishedAt,
		&task.ClientIP, &task.UserAgent, &task.InboundEndpoint, &task.UpstreamEndpoint,
		&task.CreatedAt, &task.UpdatedAt,
	); err != nil {
		return nil, err
	}
	task.ImageURLs = unmarshalStringSlice(imageURLsJSON)
	task.CosURLs = unmarshalStringSlice(cosURLsJSON)
	if len(imageMetadataJSON) > 0 {
		if err := json.Unmarshal(imageMetadataJSON, &task.ImageMetadata); err != nil {
			return nil, fmt.Errorf("unmarshal image_metadata: %w", err)
		}
	}
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

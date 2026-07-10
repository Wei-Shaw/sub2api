package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type videoGatewayRepository struct {
	db *sql.DB
}

var _ service.VideoDispatchRepository = (*videoGatewayRepository)(nil)
var _ service.VideoTaskPollRepository = (*videoGatewayRepository)(nil)
var _ service.BillingReservationReaperRepository = (*videoGatewayRepository)(nil)

// NewVideoGatewayRepository preserves the legacy CRUD return type. Its concrete
// implementation also satisfies VideoTaskFinalizationRepository (asserted in
// video_task_finalization_repo.go), allowing the worker to narrow the same
// database-backed instance without a second Wire provider.
func NewVideoGatewayRepository(db *sql.DB) service.VideoGatewayRepository {
	return &videoGatewayRepository{db: db}
}

func (r *videoGatewayRepository) CreateWithReservation(ctx context.Context, input service.VideoTaskCreationInput) (*service.VideoTaskCreationResult, error) {
	return (&videoTaskCreationRepository{db: r.db}).CreateWithReservation(ctx, input)
}

func (r *videoGatewayRepository) ReplayExisting(ctx context.Context, input service.VideoTaskCreationReplayInput) (*service.VideoTaskCreationResult, bool, error) {
	return (&videoTaskCreationRepository{db: r.db}).ReplayExisting(ctx, input)
}

func (r *videoGatewayRepository) ReapExpiredVideoReservations(ctx context.Context, now time.Time, limit int) ([]service.BillingReservationReapResult, error) {
	return reapExpiredVideoReservations(ctx, r.db, now, limit)
}

const insertVideoUsageLogSQL = `
	INSERT INTO video_usage_logs (video_task_id, provider, model, status, cost_estimate, duration, currency, pricing_source, pricing_version)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	ON CONFLICT (video_task_id) DO NOTHING
`

const videoTaskClaimLeaseSeconds = 120

const videoTaskSelectColumns = `
		vt.id, vt.provider_account_id, vt.provider, vt.model, vt.task_type, vt.prompt, vt.negative_prompt,
		vt.reference_image_url, vt.reference_video_url, vt.content_json, vt.has_video_input,
		vt.aspect_ratio, vt.duration, vt.resolution,
		vt.generate_audio, vt.watermark, vt.camera_fixed, vt.return_last_frame,
		vt.usage_total_tokens, vt.actual_resolution, vt.actual_duration, vt.last_frame_url,
		vt.status, vt.upstream_task_id, vt.result_url, vt.error_message, vt.cost_estimate, vt.poll_count,
		vt.local_asset_path, vt.local_asset_saved_at,
		vt.created_by, vt.created_at, vt.updated_at, vt.completed_at,
		COALESCE(vpa.display_name, ''), COALESCE(u.email, ''), COALESCE(u.username, '')
`

const videoTaskSelectColumnsWithReliability = `
		vt.id, vt.provider_account_id, vt.provider, vt.model, vt.task_type, vt.prompt, vt.negative_prompt,
		vt.reference_image_url, vt.reference_video_url, vt.content_json, vt.has_video_input,
		vt.aspect_ratio, vt.duration, vt.resolution,
		vt.generate_audio, vt.watermark, vt.camera_fixed, vt.return_last_frame,
		vt.usage_total_tokens, vt.actual_resolution, vt.actual_duration, vt.last_frame_url,
		vt.status, vt.upstream_task_id, vt.result_url, vt.error_message, vt.cost_estimate, vt.poll_count,
		vt.local_asset_path, vt.local_asset_saved_at,
		vt.api_key_id, vt.creation_key, vt.creation_fingerprint, vt.reservation_id,
		vt.version, vt.dispatch_state, vt.settlement_status, vt.archive_status, vt.capture_status,
		vt.balance_charged_at, vt.worker_claimed_at, vt.worker_claimed_until,
		vt.created_by, vt.created_at, vt.updated_at, vt.completed_at,
		COALESCE(vpa.display_name, ''), COALESCE(u.email, ''), COALESCE(u.username, '')
`

const videoTaskJoinSQL = `
		FROM video_tasks vt
		LEFT JOIN video_provider_accounts vpa ON vpa.id = vt.provider_account_id
		LEFT JOIN users u ON u.id = vt.created_by
`

const createVideoTaskSQL = `
	INSERT INTO video_tasks (
		provider_account_id, provider, model, task_type, prompt, negative_prompt,
		reference_image_url, reference_video_url, content_json, has_video_input,
		aspect_ratio, duration, resolution, generate_audio, watermark, camera_fixed,
		return_last_frame, status, created_by
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
	RETURNING id, created_at, updated_at
`

const createVideoTaskWithReliabilitySQL = `
	INSERT INTO video_tasks (
		provider_account_id, provider, model, task_type, prompt, negative_prompt,
		reference_image_url, reference_video_url, content_json, has_video_input,
		aspect_ratio, duration, resolution, generate_audio, watermark, camera_fixed,
		return_last_frame, status, created_by, api_key_id, creation_key,
		creation_fingerprint, reservation_id, version, dispatch_state,
		settlement_status, archive_status, capture_status, cost_estimate
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,
		$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)
	RETURNING id, created_at, updated_at
`

func (r *videoGatewayRepository) CreateProviderAccount(ctx context.Context, account *service.VideoProviderAccount) error {
	metadata, err := marshalJSONMap(account.Metadata)
	if err != nil {
		return err
	}
	const q = `
		INSERT INTO video_provider_accounts (
			provider, display_name, enabled, encrypted_api_key, masked_key,
			base_url, default_model, rate_limit_per_minute, metadata_json
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)
		RETURNING id, created_at, updated_at
	`
	err = r.db.QueryRowContext(ctx, q,
		account.Provider,
		account.DisplayName,
		account.Enabled,
		account.EncryptedAPIKey,
		account.MaskedKey,
		account.BaseURL,
		account.DefaultModel,
		account.RateLimitPerMinute,
		string(metadata),
	).Scan(&account.ID, &account.CreatedAt, &account.UpdatedAt)
	if err != nil {
		return err
	}
	return nil
}

func (r *videoGatewayRepository) GetProviderAccount(ctx context.Context, id int64) (*service.VideoProviderAccount, error) {
	const q = `
		SELECT id, provider, display_name, enabled, encrypted_api_key, masked_key,
		       base_url, default_model, rate_limit_per_minute, metadata_json, created_at, updated_at
		FROM video_provider_accounts
		WHERE id = $1
	`
	account, err := scanVideoProvider(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrVideoProviderNotFound, nil)
	}
	return account, nil
}

func (r *videoGatewayRepository) ListProviderAccounts(ctx context.Context) ([]*service.VideoProviderAccount, error) {
	const q = `
		SELECT id, provider, display_name, enabled, encrypted_api_key, masked_key,
		       base_url, default_model, rate_limit_per_minute, metadata_json, created_at, updated_at
		FROM video_provider_accounts
		ORDER BY
			CASE provider WHEN 'mock' THEN 1 WHEN 'seedance' THEN 2 WHEN 'kling' THEN 3 ELSE 99 END,
			id
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*service.VideoProviderAccount
	for rows.Next() {
		item, err := scanVideoProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *videoGatewayRepository) UpdateProviderAccount(ctx context.Context, account *service.VideoProviderAccount) error {
	metadata, err := marshalJSONMap(account.Metadata)
	if err != nil {
		return err
	}
	const q = `
		UPDATE video_provider_accounts
		SET display_name = $2,
		    enabled = $3,
		    encrypted_api_key = $4,
		    masked_key = $5,
		    base_url = $6,
		    default_model = $7,
		    rate_limit_per_minute = $8,
		    metadata_json = $9::jsonb,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at
	`
	err = r.db.QueryRowContext(ctx, q,
		account.ID,
		account.DisplayName,
		account.Enabled,
		account.EncryptedAPIKey,
		account.MaskedKey,
		account.BaseURL,
		account.DefaultModel,
		account.RateLimitPerMinute,
		string(metadata),
	).Scan(&account.UpdatedAt)
	if err != nil {
		return translatePersistenceError(err, service.ErrVideoProviderNotFound, nil)
	}
	return nil
}

func (r *videoGatewayRepository) CreateTask(ctx context.Context, task *service.VideoTask) error {
	return insertVideoTaskWith(ctx, r.db, task)
}

func insertVideoTaskWith(ctx context.Context, queryer sqlQueryRower, task *service.VideoTask) error {
	contentJSON, err := marshalVideoTaskContent(task.Content)
	if err != nil {
		return err
	}
	legacyArgs := []any{
		task.ProviderAccountID,
		task.Provider,
		task.Model,
		task.TaskType,
		task.Prompt,
		task.NegativePrompt,
		task.ReferenceImageURL,
		task.ReferenceVideoURL,
		string(contentJSON),
		task.HasVideoInput,
		task.AspectRatio,
		task.Duration,
		task.Resolution,
		nullableBoolValue(task.GenerateAudio),
		nullableBoolValue(task.Watermark),
		nullableBoolValue(task.CameraFixed),
		nullableBoolValue(task.ReturnLastFrame),
		task.Status,
		task.CreatedBy,
	}
	if !videoTaskUsesReliabilityColumns(task) {
		return scanCreatedVideoTask(queryer.QueryRowContext(ctx, createVideoTaskSQL, legacyArgs...), task)
	}
	reliabilityArgs := append(legacyArgs,
		task.APIKeyID,
		nullableNonEmptyString(task.CreationKey),
		nullableNonEmptyString(task.CreationFingerprint),
		nullableInt64Ptr(task.ReservationID),
		videoTaskVersion(task.Version),
		videoTaskStateOrDefault(task.DispatchState, service.VideoDispatchStatePending),
		videoTaskStateOrDefault(task.SettlementStatus, service.VideoSettlementStatusPending),
		videoTaskStateOrDefault(task.ArchiveStatus, service.VideoSideEffectStatusPending),
		videoTaskStateOrDefault(task.CaptureStatus, service.VideoSideEffectStatusPending),
		task.CostEstimate,
	)
	return scanCreatedVideoTask(queryer.QueryRowContext(ctx, createVideoTaskWithReliabilitySQL, reliabilityArgs...), task)
}

func videoTaskUsesReliabilityColumns(task *service.VideoTask) bool {
	return task != nil && (task.APIKeyID != nil || task.ReservationID != nil || task.CreationKey != "" || task.CreationFingerprint != "")
}

func (r *videoGatewayRepository) CreateDailyTrialTask(ctx context.Context, task *service.VideoTask, provider string, createdBy int64, trialDate time.Time) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var reservationID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO video_daily_trial_reservations (provider, created_by, trial_date)
		VALUES ($1,$2,$3)
		ON CONFLICT (provider, created_by, trial_date) DO NOTHING
		RETURNING id
	`, provider, createdBy, trialDate).Scan(&reservationID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	if err := insertVideoTaskWith(ctx, tx, task); err != nil {
		return false, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE video_daily_trial_reservations
		SET video_task_id = $2
		WHERE id = $1
	`, reservationID, task.ID); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *videoGatewayRepository) GetTask(ctx context.Context, id int64) (*service.VideoTask, error) {
	q := "SELECT" + videoTaskSelectColumnsWithReliability + videoTaskJoinSQL + " WHERE vt.id = $1"
	task, err := scanVideoTaskWithReliability(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrVideoTaskNotFound, nil)
	}
	return task, nil
}

func (r *videoGatewayRepository) ListTasks(ctx context.Context, params service.VideoTaskListParams) ([]*service.VideoTask, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	if strings.TrimSpace(params.Status) != "" {
		args = append(args, params.Status)
		where = append(where, fmt.Sprintf("vt.status = $%d", len(args)))
	}
	if strings.TrimSpace(params.Provider) != "" {
		args = append(args, params.Provider)
		where = append(where, fmt.Sprintf("vt.provider = $%d", len(args)))
	}
	if !params.IsAdmin {
		args = append(args, params.CreatedBy)
		where = append(where, fmt.Sprintf("vt.created_by = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	countQ := "SELECT COUNT(*) FROM video_tasks vt WHERE " + whereSQL
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	page := params.Page
	if page <= 0 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	args = append(args, pageSize, (page-1)*pageSize)
	q := "SELECT" + videoTaskSelectColumnsWithReliability + videoTaskJoinSQL + " WHERE " + whereSQL + fmt.Sprintf(`
		ORDER BY vt.created_at DESC, vt.id DESC
		LIMIT $%d OFFSET $%d
	`, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out, err := scanVideoTaskRowsWithReliability(rows)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

const dramaContextJoinSQL = `
		INNER JOIN LATERAL (
			SELECT vte_dc.payload_json
			FROM video_task_events vte_dc
			WHERE vte_dc.video_task_id = vt.id AND vte_dc.event_type = 'drama_context'
			ORDER BY vte_dc.created_at DESC, vte_dc.id DESC
			LIMIT 1
		) dc ON TRUE
`

const dramaEngineFamilySQL = `
CASE
	WHEN POSITION('seedance' IN LOWER(COALESCE(dc.payload_json->>'selected_provider', vt.provider))) > 0 THEN 'seedance'
	WHEN POSITION('kling' IN LOWER(COALESCE(dc.payload_json->>'selected_provider', vt.provider))) > 0 THEN 'kling'
	ELSE 'mock'
END`

const dramaSelectedModeSQL = `COALESCE(NULLIF(dc.payload_json->>'selected_mode', ''), REPLACE(vt.task_type, '_', '-'))`

const dramaSelectedModelSQL = `COALESCE(NULLIF(dc.payload_json->>'selected_model', ''), vt.model)`

func appendDramaTaskFilterClauses(filters map[string]string, where *[]string, args *[]any) {
	textFilters := map[string]string{
		"employee_alias": "employee_alias",
		"api_client_id":  "api_client_id",
		"project_id":     "project_id",
		"drama_type":     "drama_type",
		"genre":          "genre",
		"scene_type":     "scene_type",
	}
	for key, jsonKey := range textFilters {
		want := strings.TrimSpace(filters[key])
		if want == "" {
			continue
		}
		*args = append(*args, strings.ToLower(want))
		*where = append(*where, fmt.Sprintf("LOWER(COALESCE(dc.payload_json->>%s, '')) = $%d", quoteSQLLiteral(jsonKey), len(*args)))
	}
	if want := strings.TrimSpace(filters["status"]); want != "" {
		*args = append(*args, want)
		*where = append(*where, fmt.Sprintf("vt.status = $%d", len(*args)))
	}
	if want := strings.TrimSpace(filters["engine"]); want != "" {
		*args = append(*args, strings.ToLower(want))
		*where = append(*where, fmt.Sprintf("LOWER(%s) = $%d", dramaEngineFamilySQL, len(*args)))
	}
	if want := strings.TrimSpace(filters["model"]); want != "" {
		*args = append(*args, strings.ToLower(want))
		*where = append(*where, fmt.Sprintf("LOWER(%s) = $%d", dramaSelectedModelSQL, len(*args)))
	}
	if want := strings.TrimSpace(filters["mode"]); want != "" {
		*args = append(*args, strings.ToLower(want))
		*where = append(*where, fmt.Sprintf("LOWER(%s) = $%d", dramaSelectedModeSQL, len(*args)))
	}
}

func quoteSQLLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func (r *videoGatewayRepository) ListDramaTasks(ctx context.Context, params service.VideoTaskListParams, filters map[string]string) ([]*service.VideoTask, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	if strings.TrimSpace(params.Status) != "" {
		args = append(args, params.Status)
		where = append(where, fmt.Sprintf("vt.status = $%d", len(args)))
	}
	if strings.TrimSpace(params.Provider) != "" {
		args = append(args, params.Provider)
		where = append(where, fmt.Sprintf("vt.provider = $%d", len(args)))
	}
	if !params.IsAdmin {
		args = append(args, params.CreatedBy)
		where = append(where, fmt.Sprintf("vt.created_by = $%d", len(args)))
	}
	appendDramaTaskFilterClauses(filters, &where, &args)
	whereSQL := strings.Join(where, " AND ")
	fromSQL := videoTaskJoinSQL + dramaContextJoinSQL

	var total int64
	countQ := "SELECT COUNT(*) " + fromSQL + " WHERE " + whereSQL
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	page := params.Page
	if page <= 0 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	args = append(args, pageSize, (page-1)*pageSize)
	q := "SELECT" + videoTaskSelectColumnsWithReliability + fromSQL + " WHERE " + whereSQL + fmt.Sprintf(`
		ORDER BY vt.created_at DESC, vt.id DESC
		LIMIT $%d OFFSET $%d
	`, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out, err := scanVideoTaskRowsWithReliability(rows)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *videoGatewayRepository) ListRunnableTasks(ctx context.Context, limit int) ([]*service.VideoTask, error) {
	if limit <= 0 {
		limit = 20
	}
	q := `
		WITH candidate_ids AS (
			SELECT vt.id
			FROM video_tasks vt
			WHERE vt.status IN ('queued', 'submitted', 'running')
			  AND vt.dispatch_state <> 'unknown'
			  AND NOT (vt.reservation_id IS NOT NULL AND vt.settlement_status <> 'pending')
			  AND (vt.worker_claimed_until IS NULL OR vt.worker_claimed_until <= NOW())
			ORDER BY vt.updated_at ASC, vt.id ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		),
		claimed AS (
			UPDATE video_tasks vt
			SET worker_claimed_at = NOW(),
			    worker_claimed_until = NOW() + ($2::int * INTERVAL '1 second')
			FROM candidate_ids c
			WHERE vt.id = c.id
			RETURNING vt.*
		)
		SELECT` + videoTaskSelectColumnsWithReliability + `
		FROM claimed vt
		LEFT JOIN video_provider_accounts vpa ON vpa.id = vt.provider_account_id
		LEFT JOIN users u ON u.id = vt.created_by
		ORDER BY vt.updated_at ASC, vt.id ASC
	`
	rows, err := r.db.QueryContext(ctx, q, limit, videoTaskClaimLeaseSeconds)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanVideoTaskRowsWithReliability(rows)
}

func (r *videoGatewayRepository) ListDispatchUnknownTasks(ctx context.Context, limit int) ([]*service.VideoTask, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT`+videoTaskSelectColumnsWithReliability+videoTaskJoinSQL+`
		WHERE vt.dispatch_state = 'unknown'
		ORDER BY vt.updated_at ASC, vt.id ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanVideoTaskRowsWithReliability(rows)
}

func (r *videoGatewayRepository) MarkDispatchingCAS(ctx context.Context, task *service.VideoTask, event *service.VideoTaskEvent) (bool, error) {
	if task == nil {
		return false, fmt.Errorf("video task is required")
	}
	return r.applyDispatchCASTransaction(ctx, task, event, func(tx *sql.Tx) (int64, time.Time, error) {
		var version int64
		var updatedAt time.Time
		err := tx.QueryRowContext(ctx, `
			UPDATE video_tasks
			SET status = $3,
			    dispatch_state = $4,
			    version = version + 1,
			    updated_at = NOW()
			WHERE id = $1 AND version = $2
			  AND status = 'queued'
			  AND dispatch_state = 'pending'
			  AND (
			      reservation_id IS NULL OR (
			          settlement_status = 'pending'
			          AND EXISTS (
			              SELECT 1 FROM billing_reservations br
			              WHERE br.id = video_tasks.reservation_id
			                AND br.status = 'active'
			                AND br.expires_at > NOW()
			          )
			      )
			  )
			RETURNING version, updated_at
		`, task.ID, task.Version, service.VideoStatusSubmitted, service.VideoDispatchStateDispatching).Scan(&version, &updatedAt)
		return version, updatedAt, err
	}, func() {
		task.Status = service.VideoStatusSubmitted
		task.DispatchState = service.VideoDispatchStateDispatching
	})
}

func (r *videoGatewayRepository) MarkDispatchAcceptedCAS(ctx context.Context, task *service.VideoTask, event *service.VideoTaskEvent) (bool, error) {
	if task == nil {
		return false, fmt.Errorf("video task is required")
	}
	if strings.TrimSpace(task.UpstreamTaskID) == "" {
		return false, fmt.Errorf("accepted dispatch requires upstream task id")
	}
	return r.applyDispatchCASTransaction(ctx, task, event, func(tx *sql.Tx) (int64, time.Time, error) {
		var version int64
		var updatedAt time.Time
		err := tx.QueryRowContext(ctx, `
			UPDATE video_tasks
			SET status = $3,
			    upstream_task_id = $4,
			    dispatch_state = $5,
			    version = version + 1,
			    worker_claimed_at = NULL,
			    worker_claimed_until = NULL,
			    updated_at = NOW()
			WHERE id = $1 AND version = $2
			  AND status = 'submitted'
			  AND dispatch_state = 'dispatching'
			RETURNING version, updated_at
		`, task.ID, task.Version, task.Status, task.UpstreamTaskID, service.VideoDispatchStateAccepted).Scan(&version, &updatedAt)
		return version, updatedAt, err
	}, func() {
		task.DispatchState = service.VideoDispatchStateAccepted
	})
}

func (r *videoGatewayRepository) MarkDispatchUnknownCAS(ctx context.Context, task *service.VideoTask, event *service.VideoTaskEvent) (bool, error) {
	if task == nil {
		return false, fmt.Errorf("video task is required")
	}
	return r.applyDispatchCASTransaction(ctx, task, event, func(tx *sql.Tx) (int64, time.Time, error) {
		var version int64
		var updatedAt time.Time
		err := tx.QueryRowContext(ctx, `
			UPDATE video_tasks
			SET status = $3,
			    error_message = $4,
			    dispatch_state = $5,
			    version = version + 1,
			    worker_claimed_at = NULL,
			    worker_claimed_until = NULL,
			    updated_at = NOW()
			WHERE id = $1 AND version = $2
			  AND status = 'submitted'
			  AND dispatch_state = 'dispatching'
			RETURNING version, updated_at
		`, task.ID, task.Version, service.VideoStatusSubmitted, task.ErrorMessage, service.VideoDispatchStateUnknown).Scan(&version, &updatedAt)
		return version, updatedAt, err
	}, func() {
		task.Status = service.VideoStatusSubmitted
		task.DispatchState = service.VideoDispatchStateUnknown
	})
}

func (r *videoGatewayRepository) applyDispatchCASTransaction(
	ctx context.Context,
	task *service.VideoTask,
	event *service.VideoTaskEvent,
	update func(*sql.Tx) (int64, time.Time, error),
	apply func(),
) (bool, error) {
	if event == nil || event.VideoTaskID != task.ID {
		return false, fmt.Errorf("matching dispatch event is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	version, updatedAt, err := update(tx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	eventCopy := *event
	eventCopy.Payload = cloneJSONMap(event.Payload)
	if err := addVideoTaskEventWith(ctx, tx, &eventCopy); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	apply()
	task.Version = version
	task.UpdatedAt = updatedAt
	*event = eventCopy
	return true, nil
}

func (r *videoGatewayRepository) ListUnchargedSucceededVideoTasks(ctx context.Context, limit int) ([]*service.VideoTask, error) {
	if limit <= 0 {
		limit = 20
	}
	q := `
		SELECT` + videoTaskSelectColumns + videoTaskJoinSQL + `
		WHERE vt.status = 'succeeded'
		  AND vt.balance_charged_at IS NULL
		  AND COALESCE(vt.cost_estimate, 0) > 0
		ORDER BY vt.completed_at ASC NULLS LAST, vt.updated_at ASC, vt.id ASC
		LIMIT $1
	`
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanVideoTaskRows(rows)
}

func (r *videoGatewayRepository) ClaimTaskForSubmit(ctx context.Context, taskID int64) (bool, error) {
	const q = `
		UPDATE video_tasks
		SET status = $2,
		    updated_at = NOW(),
		    worker_claimed_at = COALESCE(worker_claimed_at, NOW()),
		    worker_claimed_until = NOW() + ($4::int * INTERVAL '1 second')
		WHERE id = $1 AND status = $3
		RETURNING id
	`
	var claimedID int64
	err := r.db.QueryRowContext(ctx, q, taskID, service.VideoStatusSubmitted, service.VideoStatusQueued, videoTaskClaimLeaseSeconds).Scan(&claimedID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return claimedID > 0, nil
}

func (r *videoGatewayRepository) UpdateTask(ctx context.Context, task *service.VideoTask) error {
	const q = `
		UPDATE video_tasks
		SET status = $2,
		    upstream_task_id = $3,
		    result_url = $4,
		    error_message = $5,
		    cost_estimate = $6,
		    completed_at = $7,
		    poll_count = $8,
		    usage_total_tokens = $9,
		    actual_resolution = $10,
		    actual_duration = $11,
		    last_frame_url = $12,
		    worker_claimed_at = NULL,
		    worker_claimed_until = NULL,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at
	`
	var completedAt any
	if task.CompletedAt != nil {
		completedAt = *task.CompletedAt
	}
	if err := r.db.QueryRowContext(ctx, q,
		task.ID,
		task.Status,
		task.UpstreamTaskID,
		task.ResultURL,
		task.ErrorMessage,
		task.CostEstimate,
		completedAt,
		task.PollCount,
		nullableInt64Ptr(task.UsageTotalTokens),
		nullableNonEmptyString(task.ActualResolution),
		nullableVideoIntPtr(task.ActualDuration),
		nullableNonEmptyString(task.LastFrameURL),
	).Scan(&task.UpdatedAt); err != nil {
		return translatePersistenceError(err, service.ErrVideoTaskNotFound, nil)
	}
	return nil
}

func (r *videoGatewayRepository) UpdatePolledTaskCAS(ctx context.Context, expectedVersion int64, candidate *service.VideoTask, event *service.VideoTaskEvent) (service.VideoTaskPollUpdateResult, error) {
	if r == nil || r.db == nil {
		return service.VideoTaskPollUpdateResult{}, fmt.Errorf("video poll database is required")
	}
	if candidate == nil || candidate.ID <= 0 {
		return service.VideoTaskPollUpdateResult{}, fmt.Errorf("video poll candidate is required")
	}
	if expectedVersion <= 0 {
		return service.VideoTaskPollUpdateResult{}, fmt.Errorf("video poll expected version must be positive")
	}
	if service.IsTerminalVideoStatus(candidate.Status) {
		return service.VideoTaskPollUpdateResult{}, fmt.Errorf("video poll candidate must be nonterminal")
	}
	if event == nil || event.VideoTaskID != candidate.ID || event.EventType != candidate.Status {
		return service.VideoTaskPollUpdateResult{}, fmt.Errorf("matching poll event is required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return service.VideoTaskPollUpdateResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	row, err := scanVideoFinalizationTaskRow(tx.QueryRowContext(ctx, `
		UPDATE video_tasks
		SET status = $3,
			result_url = $4,
			error_message = $5,
			cost_estimate = $6,
			poll_count = $7,
			usage_total_tokens = $8,
			actual_resolution = $9,
			actual_duration = $10,
			last_frame_url = $11,
			version = version + 1,
			worker_claimed_at = NULL,
			worker_claimed_until = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND version = $2
		  AND status NOT IN ('succeeded', 'failed', 'cancelled')
		RETURNING `+videoFinalizationTaskColumns,
		candidate.ID,
		expectedVersion,
		candidate.Status,
		strings.TrimSpace(candidate.ResultURL),
		strings.TrimSpace(candidate.ErrorMessage),
		candidate.CostEstimate,
		candidate.PollCount,
		nullableInt64Ptr(candidate.UsageTotalTokens),
		nullableNonEmptyString(candidate.ActualResolution),
		nullableVideoIntPtr(candidate.ActualDuration),
		nullableNonEmptyString(candidate.LastFrameURL),
	))
	if errors.Is(err, sql.ErrNoRows) {
		current, readErr := readVideoFinalizationCurrentTask(ctx, tx, candidate.ID)
		if readErr != nil {
			return service.VideoTaskPollUpdateResult{}, readErr
		}
		return videoTaskPollUpdateResultFromRow(current), nil
	}
	if err != nil {
		return service.VideoTaskPollUpdateResult{}, fmt.Errorf("polled video task CAS: %w", err)
	}
	eventCopy := *event
	eventCopy.Payload = cloneJSONMap(event.Payload)
	if err := addVideoTaskEventWith(ctx, tx, &eventCopy); err != nil {
		return service.VideoTaskPollUpdateResult{}, fmt.Errorf("insert polled video task event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return service.VideoTaskPollUpdateResult{}, err
	}
	result := videoTaskPollUpdateResultFromRow(row)
	result.Applied = true
	return result, nil
}

func (r *videoGatewayRepository) SetTaskLocalAsset(ctx context.Context, taskID int64, path string, savedAt time.Time) error {
	const q = `
		UPDATE video_tasks
		SET local_asset_path = $2,
		    local_asset_saved_at = $3,
		    updated_at = NOW()
		WHERE id = $1
	`
	res, err := r.db.ExecContext(ctx, q, taskID, path, savedAt)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return service.ErrVideoTaskNotFound
	}
	return nil
}

// CompleteVideoOutboxSideEffect commits the side-effect CAS and outbox
// completion together. A succeeded side effect is never regressed by a replay.
func (r *videoGatewayRepository) CompleteVideoOutboxSideEffect(ctx context.Context, eventID int64, workerID string, completedAt time.Time, taskID int64, effect string) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("video outbox database is required")
	}
	effectColumn, err := videoSideEffectColumn(effect)
	if err != nil {
		return false, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE domain_outbox
		SET status = $1, completed_at = $2, locked_at = NULL, locked_until = NULL,
		    locked_by = NULL, last_error = NULL
		WHERE id = $3 AND status = $4 AND locked_by = $5 AND locked_until > CURRENT_TIMESTAMP
	`, service.DomainOutboxStatusCompleted, completedAt.Truncate(time.Microsecond), eventID, service.DomainOutboxStatusProcessing, strings.TrimSpace(workerID))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM domain_outbox WHERE id = $1`, eventID).Scan(&status); err != nil {
			return false, err
		}
		if status == service.DomainOutboxStatusCompleted {
			return true, nil
		}
		return false, service.ErrDomainOutboxLeaseConflict
	}
	taskResult, err := tx.ExecContext(ctx, `UPDATE video_tasks SET `+effectColumn+` = 'succeeded', updated_at = NOW() WHERE id = $1`, taskID)
	if err != nil {
		return false, err
	}
	if rows, rowsErr := taskResult.RowsAffected(); rowsErr != nil {
		return false, rowsErr
	} else if rows == 0 {
		return false, service.ErrVideoTaskNotFound
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// DeadVideoOutboxSideEffect commits a dead-letter transition and marks only
// the affected delivery side effect failed. The terminal generation status and
// billing ledger remain untouched.
func (r *videoGatewayRepository) DeadVideoOutboxSideEffect(ctx context.Context, eventID int64, workerID string, nextAttemptAt time.Time, taskID int64, effect string, lastError string) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("video outbox database is required")
	}
	effectColumn, err := videoSideEffectColumn(effect)
	if err != nil {
		return false, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE domain_outbox
		SET status = $1, next_attempt_at = $2, locked_at = NULL, locked_until = NULL,
		    locked_by = NULL, last_error = $3
		WHERE id = $4 AND status = $5 AND locked_by = $6 AND locked_until > CURRENT_TIMESTAMP
	`, service.DomainOutboxStatusDead, nextAttemptAt.Truncate(time.Microsecond), sanitizeOutboxError(lastError), eventID, service.DomainOutboxStatusProcessing, strings.TrimSpace(workerID))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM domain_outbox WHERE id = $1`, eventID).Scan(&status); err != nil {
			return false, err
		}
		if status == service.DomainOutboxStatusDead {
			return true, nil
		}
		return false, service.ErrDomainOutboxLeaseConflict
	}
	taskResult, err := tx.ExecContext(ctx, `UPDATE video_tasks SET `+effectColumn+` = CASE WHEN `+effectColumn+` = 'succeeded' THEN 'succeeded' ELSE 'failed' END, updated_at = NOW() WHERE id = $1`, taskID)
	if err != nil {
		return false, err
	}
	if rows, rowsErr := taskResult.RowsAffected(); rowsErr != nil {
		return false, rowsErr
	} else if rows == 0 {
		return false, service.ErrVideoTaskNotFound
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func videoSideEffectColumn(effect string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(effect)) {
	case "capture", "capture_content":
		return "capture_status", nil
	case "archive", "archive_asset":
		return "archive_status", nil
	default:
		return "", fmt.Errorf("unsupported video side effect %q", effect)
	}
}

func (r *videoGatewayRepository) ClearTaskLocalAsset(ctx context.Context, taskID int64) error {
	const q = `
		UPDATE video_tasks
		SET local_asset_path = NULL,
		    local_asset_saved_at = NULL,
		    updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, q, taskID)
	return err
}

func (r *videoGatewayRepository) ListExpiredLocalAssets(ctx context.Context, olderThan time.Time, limit int) ([]*service.VideoTask, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT ` + videoTaskSelectColumnsWithReliability + `
		` + videoTaskJoinSQL + `
		WHERE vt.local_asset_path IS NOT NULL
		  AND vt.local_asset_path <> ''
		  AND vt.local_asset_saved_at IS NOT NULL
		  AND vt.local_asset_saved_at < $1
		ORDER BY vt.local_asset_saved_at ASC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, q, olderThan, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanVideoTaskRowsWithReliability(rows)
}

func (r *videoGatewayRepository) AddTaskEvent(ctx context.Context, event *service.VideoTaskEvent) error {
	return addVideoTaskEventWith(ctx, r.db, event)
}

func addVideoTaskEventWith(ctx context.Context, queryer sqlQueryRower, event *service.VideoTaskEvent) error {
	payload, err := marshalJSONMap(event.Payload)
	if err != nil {
		return err
	}
	const q = `
		INSERT INTO video_task_events (video_task_id, event_type, message, payload_json)
		VALUES ($1,$2,$3,$4::jsonb)
		RETURNING id, created_at
	`
	return queryer.QueryRowContext(ctx, q,
		event.VideoTaskID,
		event.EventType,
		event.Message,
		string(payload),
	).Scan(&event.ID, &event.CreatedAt)
}

func cloneJSONMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func (r *videoGatewayRepository) ListTaskEvents(ctx context.Context, taskID int64, limit int) ([]*service.VideoTaskEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = `
		SELECT id, video_task_id, event_type, message, payload_json, created_at
		FROM video_task_events
		WHERE video_task_id = $1
		ORDER BY created_at ASC, id ASC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, q, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*service.VideoTaskEvent
	for rows.Next() {
		var payload []byte
		event := &service.VideoTaskEvent{}
		if err := rows.Scan(&event.ID, &event.VideoTaskID, &event.EventType, &event.Message, &payload, &event.CreatedAt); err != nil {
			return nil, err
		}
		event.Payload = unmarshalJSONMap(payload)
		out = append(out, event)
	}
	return out, rows.Err()
}

func (r *videoGatewayRepository) InsertUsageLog(ctx context.Context, task *service.VideoTask) error {
	currency := service.NormalizeBillingCurrency(task.Currency)
	pricingSource := service.NormalizeBillingPricingSource(task.PricingSource)
	pricingVersion := service.NormalizeBillingPricingVersion(task.PricingVersion)
	var pricingVersionArg any
	if pricingVersion != "" {
		pricingVersionArg = pricingVersion
	}
	_, err := r.db.ExecContext(ctx, insertVideoUsageLogSQL,
		task.ID,
		task.Provider,
		task.Model,
		task.Status,
		task.CostEstimate,
		task.Duration,
		currency,
		pricingSource,
		pricingVersionArg,
	)
	return err
}

func (r *videoGatewayRepository) ClaimVideoBalanceCharge(ctx context.Context, taskID int64) (time.Time, bool, error) {
	const q = `
		UPDATE video_tasks
		SET balance_charged_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1 AND balance_charged_at IS NULL
		RETURNING balance_charged_at
	`
	var claimedAt time.Time
	err := r.db.QueryRowContext(ctx, q, taskID).Scan(&claimedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	return claimedAt, true, nil
}

func (r *videoGatewayRepository) ClearVideoBalanceChargeIfClaimedAt(ctx context.Context, taskID int64, claimedAt time.Time) (bool, error) {
	const q = `
		UPDATE video_tasks
		SET balance_charged_at = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND balance_charged_at = $2
	`
	result, err := r.db.ExecContext(ctx, q, taskID, claimedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *videoGatewayRepository) CountTasksSince(ctx context.Context, since time.Time) (map[string]int64, error) {
	const q = `
		SELECT status, COUNT(*)
		FROM video_tasks
		WHERE created_at >= $1
		GROUP BY status
	`
	rows, err := r.db.QueryContext(ctx, q, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int64{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[status] = count
	}
	return out, rows.Err()
}

func (r *videoGatewayRepository) CountProviderTasksSince(ctx context.Context, since time.Time) (map[string]map[string]int64, error) {
	const q = `
		SELECT provider, status, COUNT(*)
		FROM video_tasks
		WHERE created_at >= $1
		GROUP BY provider, status
	`
	rows, err := r.db.QueryContext(ctx, q, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]map[string]int64{}
	for rows.Next() {
		var provider, status string
		var count int64
		if err := rows.Scan(&provider, &status, &count); err != nil {
			return nil, err
		}
		if out[provider] == nil {
			out[provider] = map[string]int64{}
		}
		out[provider][status] = count
	}
	return out, rows.Err()
}

func (r *videoGatewayRepository) ProviderAccountTaskStatsSince(ctx context.Context, since time.Time) (map[int64]service.VideoProviderRuntimeStats, error) {
	const statsQ = `
		SELECT provider_account_id,
		       COUNT(*) FILTER (WHERE created_at >= $1),
		       COUNT(*) FILTER (WHERE status IN ('queued', 'submitted', 'running')),
		       COUNT(*) FILTER (WHERE status = 'failed' AND created_at >= $1)
		FROM video_tasks
		GROUP BY provider_account_id
	`
	rows, err := r.db.QueryContext(ctx, statsQ, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]service.VideoProviderRuntimeStats{}
	for rows.Next() {
		var providerAccountID int64
		var item service.VideoProviderRuntimeStats
		if err := rows.Scan(&providerAccountID, &item.TodayTasks, &item.CurrentInflight, &item.TodayFailures); err != nil {
			return nil, err
		}
		out[providerAccountID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	const errorsQ = `
		SELECT DISTINCT ON (provider_account_id) provider_account_id, error_message, updated_at
		FROM video_tasks
		WHERE status = 'failed' AND COALESCE(error_message, '') <> ''
		ORDER BY provider_account_id, updated_at DESC, id DESC
	`
	errorRows, err := r.db.QueryContext(ctx, errorsQ)
	if err != nil {
		return nil, err
	}
	defer func() { _ = errorRows.Close() }()
	for errorRows.Next() {
		var providerAccountID int64
		var lastError string
		var lastErrorAt time.Time
		if err := errorRows.Scan(&providerAccountID, &lastError, &lastErrorAt); err != nil {
			return nil, err
		}
		item := out[providerAccountID]
		item.LastError = lastError
		item.LastErrorAt = &lastErrorAt
		out[providerAccountID] = item
	}
	return out, errorRows.Err()
}

func (r *videoGatewayRepository) ListRecentTasksByStatus(ctx context.Context, status string, limit int) ([]*service.VideoTask, error) {
	if limit <= 0 {
		limit = 8
	}
	q := "SELECT" + videoTaskSelectColumnsWithReliability + videoTaskJoinSQL + `
		WHERE vt.status = $1
		ORDER BY vt.updated_at DESC, vt.id DESC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, q, status, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanVideoTaskRowsWithReliability(rows)
}

func (r *videoGatewayRepository) UsageSummarySince(ctx context.Context, since time.Time) ([]service.VideoUsageSummary, error) {
	const q = `
		SELECT provider, model, status, COUNT(*), COALESCE(SUM(cost_estimate), 0), COALESCE(SUM(duration), 0)
		FROM video_usage_logs
		WHERE created_at >= $1
		GROUP BY provider, model, status
		ORDER BY COUNT(*) DESC, provider, model
		LIMIT 20
	`
	rows, err := r.db.QueryContext(ctx, q, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []service.VideoUsageSummary
	for rows.Next() {
		var item service.VideoUsageSummary
		if err := rows.Scan(&item.Provider, &item.Model, &item.Status, &item.Count, &item.CostEstimate, &item.Duration); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanCreatedVideoTask(row scanner, task *service.VideoTask) error {
	return row.Scan(&task.ID, &task.CreatedAt, &task.UpdatedAt)
}

func scanVideoProvider(row scanner) (*service.VideoProviderAccount, error) {
	var metadata []byte
	account := &service.VideoProviderAccount{}
	err := row.Scan(
		&account.ID,
		&account.Provider,
		&account.DisplayName,
		&account.Enabled,
		&account.EncryptedAPIKey,
		&account.MaskedKey,
		&account.BaseURL,
		&account.DefaultModel,
		&account.RateLimitPerMinute,
		&metadata,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	account.Metadata = unmarshalJSONMap(metadata)
	account.APIKeyConfigured = strings.TrimSpace(account.EncryptedAPIKey) != "" || strings.TrimSpace(account.MaskedKey) != ""
	return account, nil
}

func scanVideoTaskRows(rows *sql.Rows) ([]*service.VideoTask, error) {
	return scanVideoTaskRowsMode(rows, false)
}

func scanVideoTaskRowsWithReliability(rows *sql.Rows) ([]*service.VideoTask, error) {
	return scanVideoTaskRowsMode(rows, true)
}

func scanVideoTaskRowsMode(rows *sql.Rows, includeReliability bool) ([]*service.VideoTask, error) {
	var out []*service.VideoTask
	for rows.Next() {
		task, err := scanVideoTaskMode(rows, includeReliability)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

func scanVideoTaskWithReliability(row scanner) (*service.VideoTask, error) {
	return scanVideoTaskMode(row, true)
}

func scanVideoTaskMode(row scanner, includeReliability bool) (*service.VideoTask, error) {
	task := &service.VideoTask{}
	var completed sql.NullTime
	var contentJSON []byte
	var generateAudio, watermark, cameraFixed, returnLastFrame sql.NullBool
	var usageTotalTokens sql.NullInt64
	var actualResolution, lastFrameURL sql.NullString
	var actualDuration sql.NullInt64
	var localAssetPath sql.NullString
	var localAssetSavedAt sql.NullTime
	var apiKeyID, reservationID sql.NullInt64
	var creationKey, creationFingerprint sql.NullString
	var balanceChargedAt, workerClaimedAt, workerClaimedUntil sql.NullTime
	destinations := []any{
		&task.ID,
		&task.ProviderAccountID,
		&task.Provider,
		&task.Model,
		&task.TaskType,
		&task.Prompt,
		&task.NegativePrompt,
		&task.ReferenceImageURL,
		&task.ReferenceVideoURL,
		&contentJSON,
		&task.HasVideoInput,
		&task.AspectRatio,
		&task.Duration,
		&task.Resolution,
		&generateAudio,
		&watermark,
		&cameraFixed,
		&returnLastFrame,
		&usageTotalTokens,
		&actualResolution,
		&actualDuration,
		&lastFrameURL,
		&task.Status,
		&task.UpstreamTaskID,
		&task.ResultURL,
		&task.ErrorMessage,
		&task.CostEstimate,
		&task.PollCount,
		&localAssetPath,
		&localAssetSavedAt,
	}
	if includeReliability {
		destinations = append(destinations,
			&apiKeyID,
			&creationKey,
			&creationFingerprint,
			&reservationID,
			&task.Version,
			&task.DispatchState,
			&task.SettlementStatus,
			&task.ArchiveStatus,
			&task.CaptureStatus,
			&balanceChargedAt,
			&workerClaimedAt,
			&workerClaimedUntil,
		)
	}
	destinations = append(destinations,
		&task.CreatedBy,
		&task.CreatedAt,
		&task.UpdatedAt,
		&completed,
		&task.ProviderAccountName,
		&task.CreatedByEmail,
		&task.CreatedByName,
	)
	err := row.Scan(destinations...)
	if err != nil {
		return nil, err
	}
	if completed.Valid {
		task.CompletedAt = &completed.Time
	}
	task.Content = unmarshalVideoTaskContent(contentJSON)
	task.GenerateAudio = boolPtrFromNull(generateAudio)
	task.Watermark = boolPtrFromNull(watermark)
	task.CameraFixed = boolPtrFromNull(cameraFixed)
	task.ReturnLastFrame = boolPtrFromNull(returnLastFrame)
	task.UsageTotalTokens = int64PtrFromNull(usageTotalTokens)
	if actualResolution.Valid {
		task.ActualResolution = actualResolution.String
	}
	task.ActualDuration = intPtrFromNullInt64(actualDuration)
	if lastFrameURL.Valid {
		task.LastFrameURL = lastFrameURL.String
	}
	if localAssetPath.Valid {
		task.LocalAssetPath = localAssetPath.String
	}
	if localAssetSavedAt.Valid {
		t := localAssetSavedAt.Time
		task.LocalAssetSavedAt = &t
	}
	if includeReliability {
		task.APIKeyID = int64PtrFromNull(apiKeyID)
		task.ReservationID = int64PtrFromNull(reservationID)
		task.BalanceChargedAt = timePtrFromNull(balanceChargedAt)
		task.WorkerClaimedAt = timePtrFromNull(workerClaimedAt)
		task.WorkerClaimedUntil = timePtrFromNull(workerClaimedUntil)
		if creationKey.Valid {
			task.CreationKey = creationKey.String
		}
		if creationFingerprint.Valid {
			task.CreationFingerprint = creationFingerprint.String
		}
	}
	return task, nil
}

func marshalVideoTaskContent(in []service.VideoTaskContentItem) ([]byte, error) {
	if in == nil {
		in = []service.VideoTaskContentItem{}
	}
	return json.Marshal(in)
}

func unmarshalVideoTaskContent(data []byte) []service.VideoTaskContentItem {
	if len(data) == 0 {
		return nil
	}
	out := []service.VideoTaskContentItem{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func nullableBoolValue(v *bool) any {
	if v == nil {
		return nil
	}
	return *v
}

func boolPtrFromNull(v sql.NullBool) *bool {
	if !v.Valid {
		return nil
	}
	out := v.Bool
	return &out
}

func nullableInt64Ptr(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableVideoIntPtr(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableNonEmptyString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func videoTaskVersion(version int64) int64 {
	if version <= 0 {
		return 1
	}
	return version
}

func videoTaskStateOrDefault(state, fallback string) string {
	state = strings.TrimSpace(state)
	if state == "" {
		return fallback
	}
	return state
}

func int64PtrFromNull(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	out := v.Int64
	return &out
}

func intPtrFromNullInt64(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	out := int(v.Int64)
	return &out
}

func marshalJSONMap(in map[string]any) ([]byte, error) {
	if in == nil {
		in = map[string]any{}
	}
	return json.Marshal(in)
}

func unmarshalJSONMap(data []byte) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	return out
}

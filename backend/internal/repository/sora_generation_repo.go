package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// soraGenerationRepository 实现 service.SoraGenerationRepository 接口。
// 使用原生 SQL 操作 sora_generations 表。
type soraGenerationRepository struct {
	sql *sql.DB
}

// NewSoraGenerationRepository 创建 Sora 生成记录仓储实例。
func NewSoraGenerationRepository(sqlDB *sql.DB) service.SoraGenerationRepository {
	return &soraGenerationRepository{sql: sqlDB}
}

func (r *soraGenerationRepository) Create(ctx context.Context, gen *service.SoraGeneration) error {
	mediaURLsJSON, _ := json.Marshal(gen.MediaURLs)
	s3KeysJSON, _ := json.Marshal(gen.S3ObjectKeys)

	err := r.sql.QueryRowContext(ctx, `
		INSERT INTO sora_generations (
			user_id, api_key_id, model, prompt, media_type,
			status, media_url, media_urls, file_size_bytes,
			storage_type, s3_object_keys, upstream_task_id, error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at
	`,
		gen.UserID, gen.APIKeyID, gen.Model, gen.Prompt, gen.MediaType,
		gen.Status, gen.MediaURL, mediaURLsJSON, gen.FileSizeBytes,
		gen.StorageType, s3KeysJSON, gen.UpstreamTaskID, gen.ErrorMessage,
	).Scan(&gen.ID, &gen.CreatedAt)
	return err
}

// CreatePendingWithLimit 在单事务内执行“并发上限检查 + 创建”，避免 count+create 竞态。
func (r *soraGenerationRepository) CreatePendingWithLimit(
	ctx context.Context,
	gen *service.SoraGeneration,
	activeStatuses []string,
	maxActive int64,
) error {
	if gen == nil {
		return fmt.Errorf("generation is nil")
	}
	if maxActive <= 0 {
		return r.Create(ctx, gen)
	}
	if len(activeStatuses) == 0 {
		activeStatuses = []string{service.SoraGenStatusPending, service.SoraGenStatusGenerating}
	}

	tx, err := r.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 使用用户级 advisory lock 串行化并发创建，避免超限竞态。
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, gen.UserID); err != nil {
		return err
	}

	placeholders := make([]string, len(activeStatuses))
	args := make([]any, 0, 1+len(activeStatuses))
	args = append(args, gen.UserID)
	for i, s := range activeStatuses {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, s)
	}
	countQuery := fmt.Sprintf(
		`SELECT COUNT(*) FROM sora_generations WHERE user_id = $1 AND status IN (%s)`,
		strings.Join(placeholders, ","),
	)
	var activeCount int64
	if err := tx.QueryRowContext(ctx, countQuery, args...).Scan(&activeCount); err != nil {
		return err
	}
	if activeCount >= maxActive {
		return service.ErrSoraGenerationConcurrencyLimit
	}

	mediaURLsJSON, _ := json.Marshal(gen.MediaURLs)
	s3KeysJSON, _ := json.Marshal(gen.S3ObjectKeys)
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO sora_generations (
			user_id, api_key_id, model, prompt, media_type,
			status, media_url, media_urls, file_size_bytes,
			storage_type, s3_object_keys, upstream_task_id, error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at
	`,
		gen.UserID, gen.APIKeyID, gen.Model, gen.Prompt, gen.MediaType,
		gen.Status, gen.MediaURL, mediaURLsJSON, gen.FileSizeBytes,
		gen.StorageType, s3KeysJSON, gen.UpstreamTaskID, gen.ErrorMessage,
	).Scan(&gen.ID, &gen.CreatedAt); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *soraGenerationRepository) GetByID(ctx context.Context, id int64) (*service.SoraGeneration, error) {
	gen := &service.SoraGeneration{}
	var mediaURLsJSON, s3KeysJSON []byte
	var completedAt sql.NullTime
	var apiKeyID sql.NullInt64

	err := r.sql.QueryRowContext(ctx, `
		SELECT id, user_id, api_key_id, model, prompt, media_type,
			status, media_url, media_urls, file_size_bytes,
			storage_type, s3_object_keys, upstream_task_id, error_message,
			created_at, completed_at
		FROM sora_generations WHERE id = $1
	`, id).Scan(
		&gen.ID, &gen.UserID, &apiKeyID, &gen.Model, &gen.Prompt, &gen.MediaType,
		&gen.Status, &gen.MediaURL, &mediaURLsJSON, &gen.FileSizeBytes,
		&gen.StorageType, &s3KeysJSON, &gen.UpstreamTaskID, &gen.ErrorMessage,
		&gen.CreatedAt, &completedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("生成记录不存在")
		}
		return nil, err
	}

	if apiKeyID.Valid {
		gen.APIKeyID = &apiKeyID.Int64
	}
	if completedAt.Valid {
		gen.CompletedAt = &completedAt.Time
	}
	_ = json.Unmarshal(mediaURLsJSON, &gen.MediaURLs)
	_ = json.Unmarshal(s3KeysJSON, &gen.S3ObjectKeys)
	return gen, nil
}

func (r *soraGenerationRepository) Update(ctx context.Context, gen *service.SoraGeneration) error {
	mediaURLsJSON, _ := json.Marshal(gen.MediaURLs)
	s3KeysJSON, _ := json.Marshal(gen.S3ObjectKeys)

	var completedAt *time.Time
	if gen.CompletedAt != nil {
		completedAt = gen.CompletedAt
	}

	_, err := r.sql.ExecContext(ctx, `
		UPDATE sora_generations SET
			status = $2, media_url = $3, media_urls = $4, file_size_bytes = $5,
			storage_type = $6, s3_object_keys = $7, upstream_task_id = $8,
			error_message = $9, completed_at = $10
		WHERE id = $1
	`,
		gen.ID, gen.Status, gen.MediaURL, mediaURLsJSON, gen.FileSizeBytes,
		gen.StorageType, s3KeysJSON, gen.UpstreamTaskID,
		gen.ErrorMessage, completedAt,
	)
	return err
}

// UpdateGeneratingIfPending 仅当状态为 pending 时更新为 generating。
func (r *soraGenerationRepository) UpdateGeneratingIfPending(ctx context.Context, id int64, upstreamTaskID string) (bool, error) {
	result, err := r.sql.ExecContext(ctx, `
		UPDATE sora_generations
		SET status = $2, upstream_task_id = $3
		WHERE id = $1 AND status = $4
	`,
		id, service.SoraGenStatusGenerating, upstreamTaskID, service.SoraGenStatusPending,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// UpdateCompletedIfActive 仅当状态为 pending/generating 时更新为 completed。
func (r *soraGenerationRepository) UpdateCompletedIfActive(
	ctx context.Context,
	id int64,
	mediaURL string,
	mediaURLs []string,
	storageType string,
	s3Keys []string,
	fileSizeBytes int64,
	completedAt time.Time,
) (bool, error) {
	mediaURLsJSON, _ := json.Marshal(mediaURLs)
	s3KeysJSON, _ := json.Marshal(s3Keys)
	result, err := r.sql.ExecContext(ctx, `
		UPDATE sora_generations
		SET status = $2,
			media_url = $3,
			media_urls = $4,
			file_size_bytes = $5,
			storage_type = $6,
			s3_object_keys = $7,
			error_message = '',
			completed_at = $8
		WHERE id = $1 AND status IN ($9, $10)
	`,
		id, service.SoraGenStatusCompleted, mediaURL, mediaURLsJSON, fileSizeBytes,
		storageType, s3KeysJSON, completedAt, service.SoraGenStatusPending, service.SoraGenStatusGenerating,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// UpdateFailedIfActive 仅当状态为 pending/generating 时更新为 failed。
func (r *soraGenerationRepository) UpdateFailedIfActive(
	ctx context.Context,
	id int64,
	errMsg string,
	completedAt time.Time,
) (bool, error) {
	result, err := r.sql.ExecContext(ctx, `
		UPDATE sora_generations
		SET status = $2,
			error_message = $3,
			completed_at = $4
		WHERE id = $1 AND status IN ($5, $6)
	`,
		id, service.SoraGenStatusFailed, errMsg, completedAt, service.SoraGenStatusPending, service.SoraGenStatusGenerating,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// UpdateCancelledIfActive 仅当状态为 pending/generating 时更新为 cancelled。
func (r *soraGenerationRepository) UpdateCancelledIfActive(ctx context.Context, id int64, completedAt time.Time) (bool, error) {
	result, err := r.sql.ExecContext(ctx, `
		UPDATE sora_generations
		SET status = $2, completed_at = $3
		WHERE id = $1 AND status IN ($4, $5)
	`,
		id, service.SoraGenStatusCancelled, completedAt, service.SoraGenStatusPending, service.SoraGenStatusGenerating,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// UpdateStorageIfCompleted 更新已完成记录的存储信息（用于手动保存，不重置 completed_at）。
func (r *soraGenerationRepository) UpdateStorageIfCompleted(
	ctx context.Context,
	id int64,
	mediaURL string,
	mediaURLs []string,
	storageType string,
	s3Keys []string,
	fileSizeBytes int64,
) (bool, error) {
	mediaURLsJSON, _ := json.Marshal(mediaURLs)
	s3KeysJSON, _ := json.Marshal(s3Keys)
	result, err := r.sql.ExecContext(ctx, `
		UPDATE sora_generations
		SET media_url = $2,
			media_urls = $3,
			file_size_bytes = $4,
			storage_type = $5,
			s3_object_keys = $6
		WHERE id = $1 AND status = $7
	`,
		id, mediaURL, mediaURLsJSON, fileSizeBytes, storageType, s3KeysJSON, service.SoraGenStatusCompleted,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *soraGenerationRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.sql.ExecContext(ctx, `DELETE FROM sora_generations WHERE id = $1`, id)
	return err
}

func (r *soraGenerationRepository) List(ctx context.Context, params service.SoraGenerationListParams) ([]*service.SoraGeneration, int64, error) {
	// 构建 WHERE 条件
	conditions := []string{"user_id = $1"}
	args := []any{params.UserID}
	argIdx := 2

	if params.Status != "" {
		// 支持逗号分隔的多状态
		statuses := strings.Split(params.Status, ",")
		placeholders := make([]string, len(statuses))
		for i, s := range statuses {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, strings.TrimSpace(s))
			argIdx++
		}
		conditions = append(conditions, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ",")))
	}
	if params.StorageType != "" {
		storageTypes := strings.Split(params.StorageType, ",")
		placeholders := make([]string, len(storageTypes))
		for i, s := range storageTypes {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, strings.TrimSpace(s))
			argIdx++
		}
		conditions = append(conditions, fmt.Sprintf("storage_type IN (%s)", strings.Join(placeholders, ",")))
	}
	if params.MediaType != "" {
		conditions = append(conditions, fmt.Sprintf("media_type = $%d", argIdx))
		args = append(args, params.MediaType)
		argIdx++
	}

	whereClause := "WHERE " + strings.Join(conditions, " AND ")

	// 计数
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM sora_generations %s", whereClause)
	if err := r.sql.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (params.Page - 1) * params.PageSize
	listQuery := fmt.Sprintf(`
		SELECT id, user_id, api_key_id, model, prompt, media_type,
			status, media_url, media_urls, file_size_bytes,
			storage_type, s3_object_keys, upstream_task_id, error_message,
			created_at, completed_at
		FROM sora_generations %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)
	args = append(args, params.PageSize, offset)

	rows, err := r.sql.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var results []*service.SoraGeneration
	for rows.Next() {
		gen := &service.SoraGeneration{}
		var mediaURLsJSON, s3KeysJSON []byte
		var completedAt sql.NullTime
		var apiKeyID sql.NullInt64

		if err := rows.Scan(
			&gen.ID, &gen.UserID, &apiKeyID, &gen.Model, &gen.Prompt, &gen.MediaType,
			&gen.Status, &gen.MediaURL, &mediaURLsJSON, &gen.FileSizeBytes,
			&gen.StorageType, &s3KeysJSON, &gen.UpstreamTaskID, &gen.ErrorMessage,
			&gen.CreatedAt, &completedAt,
		); err != nil {
			return nil, 0, err
		}

		if apiKeyID.Valid {
			gen.APIKeyID = &apiKeyID.Int64
		}
		if completedAt.Valid {
			gen.CompletedAt = &completedAt.Time
		}
		_ = json.Unmarshal(mediaURLsJSON, &gen.MediaURLs)
		_ = json.Unmarshal(s3KeysJSON, &gen.S3ObjectKeys)
		results = append(results, gen)
	}

	return results, total, rows.Err()
}

func (r *soraGenerationRepository) CountByUserAndStatus(ctx context.Context, userID int64, statuses []string) (int64, error) {
	if len(statuses) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(statuses))
	args := []any{userID}
	for i, s := range statuses {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, s)
	}

	var count int64
	query := fmt.Sprintf("SELECT COUNT(*) FROM sora_generations WHERE user_id = $1 AND status IN (%s)", strings.Join(placeholders, ","))
	err := r.sql.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

// CountByStorageType 按存储类型和状态统计生成记录数。
func (r *soraGenerationRepository) CountByStorageType(ctx context.Context, storageType string, statuses []string) (int64, error) {
	if len(statuses) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(statuses))
	args := []any{storageType}
	for i, s := range statuses {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, s)
	}

	var count int64
	query := fmt.Sprintf("SELECT COUNT(*) FROM sora_generations WHERE storage_type = $1 AND status IN (%s)", strings.Join(placeholders, ","))
	err := r.sql.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

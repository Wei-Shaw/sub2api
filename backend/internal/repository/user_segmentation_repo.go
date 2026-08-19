package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type userSegmentationRepository struct {
	db *sql.DB
}

func NewUserSegmentationRepository(db *sql.DB) service.UserTagRepository {
	return &userSegmentationRepository{db: db}
}

func (r *userSegmentationRepository) List(ctx context.Context) ([]service.UserTag, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, normalized_name, color, description, created_at, updated_at
		FROM user_tags
		WHERE deleted_at IS NULL
		ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	tags := make([]service.UserTag, 0)
	for rows.Next() {
		var tag service.UserTag
		if err := rows.Scan(
			&tag.ID,
			&tag.Name,
			&tag.NormalizedName,
			&tag.Color,
			&tag.Description,
			&tag.CreatedAt,
			&tag.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tags, nil
}

func (r *userSegmentationRepository) Create(ctx context.Context, tag *service.UserTag) error {
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO user_tags (name, normalized_name, color, description)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`,
		tag.Name, tag.NormalizedName, tag.Color, tag.Description,
	).Scan(&tag.ID, &tag.CreatedAt, &tag.UpdatedAt)
	return translateUserTagWriteError(err)
}

func (r *userSegmentationRepository) Update(ctx context.Context, tag *service.UserTag) error {
	err := r.db.QueryRowContext(ctx, `
		UPDATE user_tags
		SET name = $2,
			normalized_name = $3,
			color = $4,
			description = $5,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, created_at, updated_at`,
		tag.ID, tag.Name, tag.NormalizedName, tag.Color, tag.Description,
	).Scan(&tag.ID, &tag.CreatedAt, &tag.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrUserTagNotFound
	}
	return translateUserTagWriteError(err)
}

func (r *userSegmentationRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE user_tags
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrUserTagNotFound
	}
	return nil
}

func (r *userSegmentationRepository) ListByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]service.UserTag, error) {
	userIDs = uniquePositiveIDs(userIDs)
	result := make(map[int64][]service.UserTag, len(userIDs))
	for _, userID := range userIDs {
		result[userID] = []service.UserTag{}
	}
	if len(userIDs) == 0 {
		return result, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT a.user_id, t.id, t.name, t.normalized_name, t.color, t.description,
			t.created_at, t.updated_at
		FROM user_tag_assignments a
		JOIN user_tags t ON t.id = a.tag_id AND t.deleted_at IS NULL
		WHERE a.user_id = ANY($1)
		ORDER BY a.user_id, t.name, t.id`, pq.Array(userIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var userID int64
		var tag service.UserTag
		if err := rows.Scan(
			&userID,
			&tag.ID,
			&tag.Name,
			&tag.NormalizedName,
			&tag.Color,
			&tag.Description,
			&tag.CreatedAt,
			&tag.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result[userID] = append(result[userID], tag)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *userSegmentationRepository) ReplaceUserTags(ctx context.Context, userID int64, tagIDs []int64) error {
	tagIDs = uniquePositiveIDs(tagIDs)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_tag_assignments WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if len(tagIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_tag_assignments (user_id, tag_id)
			SELECT $1, t.id
			FROM user_tags t
			WHERE t.id = ANY($2) AND t.deleted_at IS NULL
			ON CONFLICT DO NOTHING`, userID, pq.Array(tagIDs)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *userSegmentationRepository) BatchAdd(ctx context.Context, userIDs, tagIDs []int64) (int, error) {
	userIDs = uniquePositiveIDs(userIDs)
	tagIDs = uniquePositiveIDs(tagIDs)
	if len(userIDs) == 0 || len(tagIDs) == 0 {
		return 0, nil
	}

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO user_tag_assignments (user_id, tag_id)
		SELECT u.user_id, t.id
		FROM unnest($1::bigint[]) AS u(user_id)
		CROSS JOIN user_tags t
		WHERE t.id = ANY($2) AND t.deleted_at IS NULL
		ON CONFLICT DO NOTHING`, pq.Array(userIDs), pq.Array(tagIDs))
	if err != nil {
		return 0, err
	}
	return rowsAffectedInt(result)
}

func (r *userSegmentationRepository) BatchRemove(ctx context.Context, userIDs, tagIDs []int64) (int, error) {
	userIDs = uniquePositiveIDs(userIDs)
	tagIDs = uniquePositiveIDs(tagIDs)
	if len(userIDs) == 0 || len(tagIDs) == 0 {
		return 0, nil
	}

	result, err := r.db.ExecContext(ctx, `
		DELETE FROM user_tag_assignments
		WHERE user_id = ANY($1) AND tag_id = ANY($2)`, pq.Array(userIDs), pq.Array(tagIDs))
	if err != nil {
		return 0, err
	}
	return rowsAffectedInt(result)
}

func (r *userSegmentationRepository) BatchReplaceTags(ctx context.Context, userIDs, tagIDs []int64) (int, error) {
	userIDs = uniquePositiveIDs(userIDs)
	tagIDs = uniquePositiveIDs(tagIDs)
	if len(userIDs) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_tag_assignments WHERE user_id = ANY($1)`, pq.Array(userIDs)); err != nil {
		return 0, err
	}
	if len(tagIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_tag_assignments (user_id, tag_id)
			SELECT u.user_id, t.id
			FROM unnest($1::bigint[]) AS u(user_id)
			CROSS JOIN user_tags t
			WHERE t.id = ANY($2) AND t.deleted_at IS NULL
			ON CONFLICT DO NOTHING`, pq.Array(userIDs), pq.Array(tagIDs)); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(userIDs), nil
}

func (r *userSegmentationRepository) FilterUserIDs(ctx context.Context, tagIDs []int64, match string) ([]int64, error) {
	tagIDs = uniquePositiveIDs(tagIDs)
	if len(tagIDs) == 0 {
		return []int64{}, nil
	}

	query := `
		SELECT a.user_id
		FROM user_tag_assignments a
		JOIN user_tags t ON t.id = a.tag_id AND t.deleted_at IS NULL
		WHERE a.tag_id = ANY($1)
		GROUP BY a.user_id`
	args := []any{pq.Array(tagIDs)}
	if match == "all" {
		query += ` HAVING COUNT(DISTINCT a.tag_id) = $2`
		args = append(args, len(tagIDs))
	}
	query += ` ORDER BY a.user_id`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	userIDs := make([]int64, 0)
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return userIDs, nil
}

func (r *userSegmentationRepository) ListHiddenGroupIDsByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]int64, error) {
	userIDs = uniquePositiveIDs(userIDs)
	result := make(map[int64][]int64, len(userIDs))
	for _, userID := range userIDs {
		result[userID] = []int64{}
	}
	if len(userIDs) == 0 {
		return result, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id, group_id
		FROM user_hidden_groups
		WHERE user_id = ANY($1)
		ORDER BY user_id, group_id`, pq.Array(userIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var userID, groupID int64
		if err := rows.Scan(&userID, &groupID); err != nil {
			return nil, err
		}
		result[userID] = append(result[userID], groupID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *userSegmentationRepository) ReplaceHiddenGroups(ctx context.Context, userID int64, groupIDs []int64) error {
	groupIDs = uniquePositiveIDs(groupIDs)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_hidden_groups WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if len(groupIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_hidden_groups (user_id, group_id)
			SELECT $1, group_id
			FROM unnest($2::bigint[]) AS requested(group_id)
			ON CONFLICT DO NOTHING`, userID, pq.Array(groupIDs)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *userSegmentationRepository) BatchReplaceHiddenGroups(ctx context.Context, userIDs, groupIDs []int64) (int, error) {
	userIDs = uniquePositiveIDs(userIDs)
	groupIDs = uniquePositiveIDs(groupIDs)
	if len(userIDs) == 0 {
		return 0, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_hidden_groups WHERE user_id = ANY($1)`, pq.Array(userIDs)); err != nil {
		return 0, err
	}
	if len(groupIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_hidden_groups (user_id, group_id)
			SELECT u.user_id, g.group_id
			FROM unnest($1::bigint[]) AS u(user_id)
			CROSS JOIN unnest($2::bigint[]) AS g(group_id)
			ON CONFLICT DO NOTHING`, pq.Array(userIDs), pq.Array(groupIDs)); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(userIDs), nil
}

func translateUserTagWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return service.ErrUserTagExists
	}
	return err
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func rowsAffectedInt(result sql.Result) (int, error) {
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

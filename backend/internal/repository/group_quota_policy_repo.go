package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type groupQuotaPolicyRepository struct{ db *sql.DB }

func NewGroupQuotaPolicyRepository(db *sql.DB) service.GroupQuotaPolicyRepository {
	return &groupQuotaPolicyRepository{db: db}
}

func scanGroupQuotaPolicy(row interface{ Scan(...any) error }) (*service.GroupQuotaPolicyRecord, error) {
	record := service.EmptyGroupQuotaPolicyRecord(0)
	var config, runtime []byte
	if err := row.Scan(&record.GroupID, &record.Revision, &record.RuntimeVersion, &config, &runtime); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(config, &record.Config); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(runtime, &record.Runtime); err != nil {
		return nil, err
	}
	return record, nil
}

func (r *groupQuotaPolicyRepository) Get(ctx context.Context, groupID int64) (*service.GroupQuotaPolicyRecord, error) {
	record, err := scanGroupQuotaPolicy(r.db.QueryRowContext(ctx, `SELECT p.group_id, p.revision, p.runtime_version, p.config, p.runtime
		FROM group_quota_policies p JOIN groups g ON g.id = p.group_id
		WHERE p.group_id = $1 AND g.deleted_at IS NULL`, groupID))
	if errors.Is(err, sql.ErrNoRows) {
		return service.EmptyGroupQuotaPolicyRecord(groupID), nil
	}
	return record, err
}

func (r *groupQuotaPolicyRepository) List(ctx context.Context) ([]*service.GroupQuotaPolicyRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT p.group_id, p.revision, p.runtime_version, p.config, p.runtime
		FROM group_quota_policies p JOIN groups g ON g.id = p.group_id
		WHERE g.deleted_at IS NULL AND g.platform = 'openai' ORDER BY p.group_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []*service.GroupQuotaPolicyRecord{}
	for rows.Next() {
		record, err := scanGroupQuotaPolicy(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (r *groupQuotaPolicyRepository) SaveConfig(ctx context.Context, record *service.GroupQuotaPolicyRecord, revision, runtimeVersion int64) (bool, error) {
	config, err := json.Marshal(record.Config)
	if err != nil {
		return false, err
	}
	runtime, err := json.Marshal(record.Runtime)
	if err != nil {
		return false, err
	}
	// The SELECT also protects against writing a soft-deleted group. Both versions
	// are checked so a concurrent sample/release cannot be lost during a save.
	result, err := r.db.ExecContext(ctx, `INSERT INTO group_quota_policies (group_id, revision, runtime_version, config, runtime)
		SELECT id, $2 + 1, $3 + 1, $4::jsonb, $5::jsonb FROM groups WHERE id = $1 AND deleted_at IS NULL AND platform = 'openai'
		AND ($2 = 0 OR EXISTS (SELECT 1 FROM group_quota_policies WHERE group_id = $1))
		ON CONFLICT (group_id) DO UPDATE SET revision = EXCLUDED.revision, runtime_version = EXCLUDED.runtime_version,
		config = EXCLUDED.config, runtime = EXCLUDED.runtime, updated_at = NOW()
		WHERE group_quota_policies.revision = $2 AND group_quota_policies.runtime_version = $3`,
		record.GroupID, revision, runtimeVersion, string(config), string(runtime))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (r *groupQuotaPolicyRepository) SaveRuntime(ctx context.Context, record *service.GroupQuotaPolicyRecord, revision, runtimeVersion int64) (bool, error) {
	runtime, err := json.Marshal(record.Runtime)
	if err != nil {
		return false, err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE group_quota_policies SET runtime = $4::jsonb, runtime_version = runtime_version + 1, updated_at = NOW()
		WHERE group_id = $1 AND revision = $2 AND runtime_version = $3
		AND EXISTS (SELECT 1 FROM groups WHERE id = $1 AND deleted_at IS NULL AND platform = 'openai')`,
		record.GroupID, revision, runtimeVersion, string(runtime))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

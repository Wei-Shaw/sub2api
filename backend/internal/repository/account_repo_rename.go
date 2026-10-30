package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *accountRepository) ListAccountNamesForRename(ctx context.Context, ids []int64, prefix, suffix string) ([]service.AccountName, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT id, name FROM accounts WHERE deleted_at IS NULL AND (id = ANY($1) OR (LEFT(name, CHAR_LENGTH($2)) = $2 AND RIGHT(name, CHAR_LENGTH($3)) = $3)) ORDER BY id`, pq.Array(ids), prefix, suffix)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]service.AccountName, 0)
	for rows.Next() {
		var row service.AccountName
		if err := rows.Scan(&row.ID, &row.Name); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *accountRepository) ApplyAccountRenames(ctx context.Context, items []service.AccountRenameItem) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	client := tx.Client()
	// Serialize bulk naming plans, including plans with disjoint selections that
	// chose the same next number. Ordinary account names remain non-unique.
	if _, err = client.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('account_bulk_rename'))`); err != nil {
		return err
	}
	ids := make([]int64, 0, len(items))
	names := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.AccountID)
		names = append(names, item.NewName)
	}
	rows, err := client.QueryContext(ctx, `SELECT id, name FROM accounts WHERE deleted_at IS NULL AND id = ANY($1) ORDER BY id FOR UPDATE`, pq.Array(ids))
	if err != nil {
		return err
	}
	current := make(map[int64]string, len(items))
	for rows.Next() {
		var id int64
		var name string
		if err = rows.Scan(&id, &name); err != nil {
			_ = rows.Close()
			return err
		}
		current[id] = name
	}
	readErr := rows.Err()
	_ = rows.Close()
	if readErr != nil {
		return readErr
	}
	if len(current) != len(items) {
		return service.ErrAccountNotFound
	}
	for _, item := range items {
		if current[item.AccountID] != item.OldName && current[item.AccountID] != item.NewName {
			return service.ErrAccountRenameConflict
		}
	}
	// Target rows may exchange names. Only names held outside the plan conflict.
	rows, err = client.QueryContext(ctx, `SELECT id FROM accounts WHERE deleted_at IS NULL AND name = ANY($1) AND NOT (id = ANY($2)) LIMIT 1`, pq.Array(names), pq.Array(ids))
	if err != nil {
		return err
	}
	conflict := rows.Next()
	readErr = rows.Err()
	_ = rows.Close()
	if readErr != nil {
		return readErr
	}
	if conflict {
		return service.ErrAccountRenameConflict
	}
	for _, item := range items {
		if current[item.AccountID] == item.NewName {
			continue
		}
		if _, err = client.ExecContext(ctx, `UPDATE accounts SET name = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`, item.NewName, item.AccountID); err != nil {
			return err
		}
		if err = enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &item.AccountID, nil, nil); err != nil {
			return err
		}
	}
	return tx.Commit()
}

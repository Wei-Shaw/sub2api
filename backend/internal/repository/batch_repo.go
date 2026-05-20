package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// batchRepository 不依赖 Ent，直接使用底层 sql.DB。
type batchRepository struct {
	db *sql.DB
}

func NewBatchRepository(sqlDB *sql.DB) service.BatchRepository {
	return &batchRepository{db: sqlDB}
}

func (r *batchRepository) Create(ctx context.Context, batch *service.Batch) error {
	existing, err := r.GetByName(ctx, batch.Name)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("batch name already exists: %s", batch.Name)
	}

	row := r.db.QueryRowContext(ctx,
		`INSERT INTO batches (name, description, source, account_count)
		 VALUES ($1, $2, $3, 0)
		 RETURNING id, created_at, updated_at`,
		batch.Name, batch.Description, batch.Source)
	err = row.Scan(&batch.ID, &batch.CreatedAt, &batch.UpdatedAt)
	if err != nil {
		// Check for unique violation
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return fmt.Errorf("batch name already exists: %s", batch.Name)
		}
	}
	return err
}

func (r *batchRepository) GetByID(ctx context.Context, id int64) (*service.Batch, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, source, account_count, created_at, updated_at
		 FROM batches WHERE id = $1`, id)
	var b service.Batch
	if err := row.Scan(&b.ID, &b.Name, &b.Description, &b.Source, &b.AccountCount, &b.CreatedAt, &b.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

func (r *batchRepository) GetByName(ctx context.Context, name string) (*service.Batch, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, source, account_count, created_at, updated_at
		 FROM batches WHERE name = $1`, name)
	var b service.Batch
	if err := row.Scan(&b.ID, &b.Name, &b.Description, &b.Source, &b.AccountCount, &b.CreatedAt, &b.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

func (r *batchRepository) List(ctx context.Context) ([]service.Batch, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT b.id, b.name, b.description, b.source,
		 COALESCE((SELECT COUNT(*) FROM accounts a WHERE a.batch_id = b.id), 0) AS account_count,
		 b.created_at, b.updated_at
		 FROM batches b ORDER BY b.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var batches []service.Batch
	for rows.Next() {
		var b service.Batch
		if err := rows.Scan(&b.ID, &b.Name, &b.Description, &b.Source, &b.AccountCount, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		batches = append(batches, b)
	}
	return batches, rows.Err()
}

func (r *batchRepository) Update(ctx context.Context, batch *service.Batch) error {
	existing, err := r.GetByName(ctx, batch.Name)
	if err != nil {
		return err
	}
	if existing != nil && existing.ID != batch.ID {
		return fmt.Errorf("batch name already exists: %s", batch.Name)
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE batches SET name = $1, description = $2, updated_at = NOW() WHERE id = $3`,
		batch.Name, batch.Description, batch.ID)
	return err
}

func (r *batchRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET batch_id = NULL WHERE batch_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM batches WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *batchRepository) UpdateAccountCount(ctx context.Context, batchID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE batches SET account_count = (
			SELECT COUNT(*) FROM accounts WHERE batch_id = $1
		), updated_at = NOW() WHERE id = $1`, batchID)
	return err
}

// 确保 pq.Array 被引用（account_repo.go 中使用）
var _ = pq.Array

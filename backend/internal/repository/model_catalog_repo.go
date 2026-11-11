package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type modelCatalogRepository struct {
	db *sql.DB
}

// NewModelCatalogRepository 创建模型目录数据访问实例。
func NewModelCatalogRepository(db *sql.DB) service.ModelCatalogRepository {
	return &modelCatalogRepository{db: db}
}

const modelCatalogColumns = `id, model_id, platform, vendor, display_name, description,
	input_price, output_price, cache_write_price, cache_read_price,
	context_window, max_output_tokens, released_at, enabled, sort_order, created_at, updated_at`

type modelCatalogScanner interface {
	Scan(dest ...any) error
}

func scanModelCatalogEntry(row modelCatalogScanner) (*service.ModelCatalogEntry, error) {
	e := &service.ModelCatalogEntry{}
	var inputPrice, outputPrice, cacheWritePrice, cacheReadPrice sql.NullFloat64
	var releasedAt sql.NullTime
	if err := row.Scan(
		&e.ID, &e.ModelID, &e.Platform, &e.Vendor, &e.DisplayName, &e.Description,
		&inputPrice, &outputPrice, &cacheWritePrice, &cacheReadPrice,
		&e.ContextWindow, &e.MaxOutputTokens, &releasedAt, &e.Enabled, &e.SortOrder, &e.CreatedAt, &e.UpdatedAt,
	); err != nil {
		return nil, err
	}
	e.InputPrice = nullFloatPtr(inputPrice)
	e.OutputPrice = nullFloatPtr(outputPrice)
	e.CacheWritePrice = nullFloatPtr(cacheWritePrice)
	e.CacheReadPrice = nullFloatPtr(cacheReadPrice)
	if releasedAt.Valid {
		t := releasedAt.Time
		e.ReleasedAt = &t
	}
	return e, nil
}

func nullFloatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

func (r *modelCatalogRepository) List(ctx context.Context, params service.ModelCatalogListParams) ([]*service.ModelCatalogEntry, error) {
	where := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if p := strings.TrimSpace(params.Platform); p != "" {
		args = append(args, p)
		where = append(where, fmt.Sprintf("platform = $%d", len(args)))
	}
	if v := strings.TrimSpace(params.Vendor); v != "" {
		args = append(args, v)
		where = append(where, fmt.Sprintf("vendor = $%d", len(args)))
	}
	if q := strings.TrimSpace(params.Search); q != "" {
		args = append(args, "%"+escapeLike(q)+"%")
		where = append(where, fmt.Sprintf("(model_id ILIKE $%d OR display_name ILIKE $%d)", len(args), len(args)))
	}
	if params.Enabled != nil {
		args = append(args, *params.Enabled)
		where = append(where, fmt.Sprintf("enabled = $%d", len(args)))
	}

	query := `SELECT ` + modelCatalogColumns + ` FROM model_catalog`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY sort_order ASC, id ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list model catalog: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.ModelCatalogEntry, 0)
	for rows.Next() {
		e, err := scanModelCatalogEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan model catalog: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate model catalog: %w", err)
	}
	return out, nil
}

func (r *modelCatalogRepository) GetByID(ctx context.Context, id int64) (*service.ModelCatalogEntry, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+modelCatalogColumns+` FROM model_catalog WHERE id = $1`, id)
	e, err := scanModelCatalogEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrModelCatalogNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get model catalog: %w", err)
	}
	return e, nil
}

func (r *modelCatalogRepository) Create(ctx context.Context, e *service.ModelCatalogEntry) error {
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO model_catalog (model_id, platform, vendor, display_name, description,
			input_price, output_price, cache_write_price, cache_read_price,
			context_window, max_output_tokens, released_at, enabled, sort_order)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 RETURNING id, created_at, updated_at`,
		e.ModelID, e.Platform, e.Vendor, e.DisplayName, e.Description,
		e.InputPrice, e.OutputPrice, e.CacheWritePrice, e.CacheReadPrice,
		e.ContextWindow, e.MaxOutputTokens, e.ReleasedAt, e.Enabled, e.SortOrder,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return service.ErrModelCatalogExists
		}
		return fmt.Errorf("insert model catalog: %w", err)
	}
	return nil
}

func (r *modelCatalogRepository) Update(ctx context.Context, e *service.ModelCatalogEntry) error {
	err := r.db.QueryRowContext(ctx,
		`UPDATE model_catalog SET model_id = $2, platform = $3, vendor = $4, display_name = $5, description = $6,
			input_price = $7, output_price = $8, cache_write_price = $9, cache_read_price = $10,
			context_window = $11, max_output_tokens = $12, released_at = $13, enabled = $14, sort_order = $15,
			updated_at = NOW()
		 WHERE id = $1
		 RETURNING created_at, updated_at`,
		e.ID, e.ModelID, e.Platform, e.Vendor, e.DisplayName, e.Description,
		e.InputPrice, e.OutputPrice, e.CacheWritePrice, e.CacheReadPrice,
		e.ContextWindow, e.MaxOutputTokens, e.ReleasedAt, e.Enabled, e.SortOrder,
	).Scan(&e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrModelCatalogNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return service.ErrModelCatalogExists
		}
		return fmt.Errorf("update model catalog: %w", err)
	}
	return nil
}

func (r *modelCatalogRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM model_catalog WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete model catalog: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return service.ErrModelCatalogNotFound
	}
	return nil
}
